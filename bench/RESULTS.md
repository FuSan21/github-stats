# Bun vs Go on a Raspberry Pi 5

The same cards service in two implementations, measured side by side on 2026-10-31:

- **Bun:** TypeScript on Bun 1.4 (`oven/bun:1.4-alpine`), `main` at `ca844da`.
- **Go:** Go 1.27, a static binary on `scratch`, from the `go-rewrite` branch.

Both render identical SVGs. 28 of 29 parity cases match byte for byte; the other differs in the last digit of one float.

## Setup

- **Host:** rpi5-01, a Raspberry Pi 5 Model B (4 GB, NVMe), otherwise idle, Docker 28.5.
- **CPUs:** each service was pinned to cores 0–1 (`--cpuset-cpus`), and the load generator (`bench/load`) to cores 2–3, so it didn't take CPU from the service.
- **Data:** no real GitHub. Both talk to `bench/fakegh` with the test fixtures, and every card's data was cached before measuring, so these numbers are rendering and HTTP cost only.
- **Memory:** read from `/proc/<pid>/status` (`VmRSS`, and `VmHWM` for the peak). The Pi boots with `cgroup_disable=memory`, so `docker stats` reports nothing.
- **CPU:** utime + stime from `/proc/<pid>/stat` over each run.
- **Load:** 15 s per card at 32 clients and 30 s mixed, each after a 2 s warm-up. A single run, so read differences under about 10 % as noise.

Raw data: [`results-2026-10-31-rpi5.jsonl`](results-2026-10-31-rpi5.jsonl). To reproduce: `bench/prepare.sh main go-rewrite`, copy `bench/out/` to the Pi, run `./run.sh`, then `go run ./report < results.jsonl`.

## Results

| | Bun | Go | Go vs Bun |
| --- | ---: | ---: | ---: |
| Image size | 115.1 MB | 7.8 MB | 14.8× smaller |
| Build time (no cache, on the Pi) | 1.8 s | 16.6 s | 9× slower |
| Start to first `/healthz` | 162 ms | 145 ms | about the same |
| Memory at idle | 41.7 MiB | 9.0 MiB | 4.6× less |
| Memory at idle, data cached | 49.5 MiB | 9.9 MiB | 5.0× less |
| Peak memory under load | 99.9 MiB | 16.0 MiB | 6.2× less |
| Memory 10 s after load | 61.1 MiB | 14.7 MiB | 4.2× less |

**Per card**, 32 concurrent clients:

| Card | Bun req/s | Go req/s | Go vs Bun | Bun p99 | Go p99 | Bun CPU/req | Go CPU/req |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `stats` | 10056 | 10153 | 1.0× | 6.2 ms | 9.7 ms | 0.12 ms | 0.17 ms |
| `top-langs` | 9416 | 13053 | 1.4× | 6.5 ms | 7.4 ms | 0.12 ms | 0.14 ms |
| `streak` | 2522 | 8079 | 3.2× | 24.9 ms | 13.5 ms | 0.47 ms | 0.24 ms |
| `graph` | 5670 | 6122 | 1.1× | 11.3 ms | 17.6 ms | 0.21 ms | 0.33 ms |
| `oss` | 12239 | 18394 | 1.5× | 5.1 ms | 5.4 ms | 0.09 ms | 0.08 ms |
| `views` | 16963 | 21016 | 1.2× | 4.0 ms | 5.3 ms | 0.07 ms | 0.07 ms |
| `healthz` | 26478 | 26772 | 1.0× | 3.6 ms | 4.2 ms | 0.04 ms | 0.04 ms |

**Mixed**, all cards round-robin:

| Clients | | req/s | p50 | p90 | p99 | max | CPU/req |
| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | Bun | 4074 | 0.18 ms | 0.48 ms | 0.81 ms | 7.2 ms | 0.19 ms |
| 1 | Go | 4526 | 0.20 ms | 0.32 ms | 0.51 ms | 6.1 ms | 0.18 ms |
| 64 | Bun | 5607 | 10.87 ms | 12.64 ms | 22.29 ms | 29.4 ms | 0.19 ms |
| 64 | Go | 9015 | 5.46 ms | 15.03 ms | 29.34 ms | 82.5 ms | 0.20 ms |

No run had errors.

## What it means

- **Memory and image size are the real difference.** Go uses 4–6× less memory at every stage and its image is 15× smaller. On a 4 GB Pi shared with other services, that matters more than anything below.
- **Throughput mostly comes from cores, not speed.** Bun runs the JavaScript on one thread and used about 1.0 core throughout. Go used 1.3–1.8 of its 2 cores. Per CPU-millisecond the two are close: Go is about 2× cheaper on `streak` and slightly cheaper on `oss`, but dearer on `stats` (1.4×) and `graph` (1.6×), where it builds the SVG with many `fmt` calls.
- **Go's tail latency is worse under heavy load.** At 64 clients its p99 is 29 ms and its max 83 ms, against Bun's 22 ms and 29 ms, while it serves 1.6× the requests. At one client Go's tail is better.
- **None of this throughput is used in practice.** nginx caches each card for an hour, so the service sees a few requests an hour, and either version answers one in well under a millisecond once its data is cached. The first request after a cache expiry is dominated by the GitHub API, not by either runtime.
- **Bun builds faster.** It has nothing to compile, while Go compiles on the Pi for about 17 s, once per deploy.
