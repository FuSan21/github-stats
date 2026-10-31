#!/bin/sh
# Assembles bench/out/ for run.sh: both versions' sources, patched so their
# GitHub base URL can point at fakegh, plus fakegh and load built for the target.
#
#   bench/prepare.sh <bun-ref> <go-ref>     e.g. bench/prepare.sh main go-rewrite
#
# A ref of "." takes cards/ from the working tree instead of git.
# GOARCH defaults to arm64 (Raspberry Pi 5). Copy bench/out/ to the host and run run.sh.
set -eu
cd "$(dirname "$0")/.."
BUN_REF=${1:?bun ref}
GO_REF=${2:?go ref}
OUT=bench/out
rm -rf "$OUT" && mkdir -p "$OUT"

extract() { # ref dest
  mkdir -p "$2"
  if [ "$1" = . ]; then cp -r cards/. "$2"; else git archive "$1" cards | tar -x -C "$2" --strip-components=1; fi
  rm -rf "$2/node_modules"
}

extract "$BUN_REF" "$OUT/bun-src"
sed -i 's#"https://api.github.com/graphql"#`${process.env.GITHUB_API_URL ?? "https://api.github.com"}/graphql`#; s#`https://api.github.com${path}`#`${process.env.GITHUB_API_URL ?? "https://api.github.com"}${path}`#' \
  "$OUT/bun-src/src/lib/github.ts"
grep -q GITHUB_API_URL "$OUT/bun-src/src/lib/github.ts"

extract "$GO_REF" "$OUT/go-src"
sed -i 's#GitHub: gh.New(token),#GitHub: func() *gh.Client { c := gh.New(token); c.BaseURL = env("GITHUB_API_URL", c.BaseURL); return c }(),#' \
  "$OUT/go-src/main.go"
grep -q GITHUB_API_URL "$OUT/go-src/main.go"

(cd bench && CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH:-arm64} go build -trimpath -o out/fakegh ./fakegh \
  && CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH:-arm64} go build -trimpath -o out/load ./load)
cp bench/run.sh "$OUT/"
echo "ready: $OUT"
