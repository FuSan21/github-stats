#!/bin/sh
# Benchmarks two builds of the cards service side by side on one host.
#
# Expects, next to this script: bun-src/ and go-src/ (each a cards/ directory
# whose GitHub base URL can be set with GITHUB_API_URL), and linux binaries
# fakegh and load (built from bench/fakegh and bench/load). See prepare.sh.
#
# Each service gets cores 0-1; the load generator gets cores 2-3. No real
# GitHub token is used: the services talk to fakegh.
#
# Memory and CPU come from /proc/<pid> of the service's process, not docker
# stats, so they work without the cgroup memory controller (which Raspberry Pi
# OS often boots with disabled). VmHWM is the peak resident memory since start.
set -eu
cd "$(dirname "$0")"

NET=cards-bench
PORT=7100
OUT=results.jsonl
DURATION=${DURATION:-15s}
MIXED_DURATION=${MIXED_DURATION:-30s}
TICK=$(getconf CLK_TCK)
: > "$OUT"

ms() { date +%s%3N; }
emit() { echo "$1" | tee -a "$OUT"; }
proc_kb() { awk -v k="$2:" '$1 == k { print $2 }' "/proc/$1/status"; }
mib() { awk -v kb="$1" 'BEGIN { printf "%.1f", kb / 1024 }'; }
# utime + stime of the whole process (all threads), in ms. The command name has no spaces.
cpu_ms() { awk -v t="$TICK" '{ printf "%d", ($14 + $15) * 1000 / t }' "/proc/$1/stat"; }

PIN=""
command -v taskset >/dev/null && PIN="taskset -c 2,3"

echo "== setup"
docker network create "$NET" >/dev/null 2>&1 || true
mkdir -p fakegh-ctx && cp fakegh fakegh-ctx/
printf 'FROM scratch\nCOPY fakegh /fakegh\nENTRYPOINT ["/fakegh"]\n' > fakegh-ctx/Dockerfile
docker build -q -t cards-bench-fakegh fakegh-ctx >/dev/null
docker rm -f fakegh >/dev/null 2>&1 || true
docker run -d --name fakegh --network "$NET" --cpuset-cpus 2,3 cards-bench-fakegh >/dev/null
docker pull -q oven/bun:1.4-alpine >/dev/null
docker pull -q golang:1.27-alpine >/dev/null

U="http://127.0.0.1:$PORT"
CARDS="stats=/api?username=FuSan21&theme=gruvbox&show_icons=true
top-langs=/api/top-langs?username=FuSan21&layout=compact&theme=gruvbox
streak=/streak?user=FuSan21&theme=gruvbox&hide_border=true
graph=/graph?username=FuSan21&theme=gruvbox
oss=/oss?username=FuSan21&theme=gruvbox
views=/views?username=FuSan21
healthz=/healthz"
MIXED=$(echo "$CARDS" | grep -v '^healthz=' | cut -d= -f2- | sed "s#^#$U#")

# load <pid> <concurrency> <duration> <label>  (URLs on stdin) -> JSON with CPU and memory added
load() {
  c0=$(cpu_ms "$1"); t0=$(ms)
  res=$($PIN ./load -c "$2" -d "$3" -w 2s -label "$4")
  c1=$(cpu_ms "$1"); t1=$(ms)
  printf '{"cpu_ms":%d,"wall_ms":%d,"rss_mib":%s,"peak_mib":%s,"load":%s}' \
    $((c1 - c0)) $((t1 - t0)) "$(mib "$(proc_kb "$1" VmRSS)")" "$(mib "$(proc_kb "$1" VmHWM)")" "$res"
}

for impl in bun go; do
  echo "== $impl: build"
  t0=$(ms)
  docker build -q --no-cache -t "cards-bench-$impl" "$impl-src" >/dev/null
  build_s=$(( ($(ms) - t0) / 1000 ))
  size_mb=$(docker image inspect "cards-bench-$impl" --format '{{.Size}}' | awk '{ printf "%.1f", $1 / 1e6 }')

  echo "== $impl: cold start"
  docker rm -f "cards-$impl" >/dev/null 2>&1 || true
  t0=$(ms)
  docker run -d --name "cards-$impl" --network "$NET" -p "127.0.0.1:$PORT:7000" --cpuset-cpus 0,1 \
    -e GITHUB_USER=FuSan21 -e GITHUB_TOKEN=bench -e GITHUB_API_URL=http://fakegh:8080 -e TZ=Asia/Dhaka \
    --tmpfs /data:uid=1000,gid=1000 "cards-bench-$impl" >/dev/null
  until curl -sf "$U/healthz" >/dev/null; do sleep 0.005; done
  ready_ms=$(( $(ms) - t0 ))
  pid=$(docker inspect -f '{{.State.Pid}}' "cards-$impl")
  t1=$(ms)
  curl -sf "$U/api?username=FuSan21" >/dev/null
  first_card_ms=$(( $(ms) - t1 ))
  sleep 3
  idle=$(mib "$(proc_kb "$pid" VmRSS)")

  # Warm every card's data cache, then measure idle memory with data loaded.
  for line in $CARDS; do curl -sf "$U${line#*=}" >/dev/null; done
  sleep 3
  warm=$(mib "$(proc_kb "$pid" VmRSS)")
  emit "{\"impl\":\"$impl\",\"kind\":\"startup\",\"build_s\":$build_s,\"image_mb\":$size_mb,\"ready_ms\":$ready_ms,\"first_card_ms\":$first_card_ms,\"idle_mib\":$idle,\"warm_idle_mib\":$warm}"

  for line in $CARDS; do
    name=${line%%=*}
    echo "== $impl: $name"
    emit "{\"impl\":\"$impl\",\"kind\":\"card\",\"card\":\"$name\",$(echo "$U${line#*=}" | load "$pid" 32 "$DURATION" "$name" | cut -c2-)"
  done

  for c in 1 64; do
    echo "== $impl: mixed c=$c"
    emit "{\"impl\":\"$impl\",\"kind\":\"mixed\",\"concurrency\":$c,$(echo "$MIXED" | load "$pid" "$c" "$MIXED_DURATION" "mixed-c$c" | cut -c2-)"
  done
  sleep 10
  emit "{\"impl\":\"$impl\",\"kind\":\"settled\",\"rss_mib\":$(mib "$(proc_kb "$pid" VmRSS)"),\"peak_mib\":$(mib "$(proc_kb "$pid" VmHWM)")}"

  docker rm -f "cards-$impl" >/dev/null
done

echo "== cleanup"
docker rm -f fakegh >/dev/null
docker network rm "$NET" >/dev/null
docker rmi -f cards-bench-bun cards-bench-go cards-bench-fakegh >/dev/null
echo "done: $OUT"
