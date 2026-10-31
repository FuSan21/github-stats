# github-stats

Self-hosted GitHub profile cards for a single user, deployed on Dokploy. One Go service (`cards/`) renders every card, using only the standard library. It reimplements these public instances, keeping their query parameters and look:

| Card | Reimplements |
| --- | --- |
| Stats, top languages | [anuraghazra/github-readme-stats](https://github.com/anuraghazra/github-readme-stats) |
| Streak | [DenverCoder1/github-readme-streak-stats](https://github.com/DenverCoder1/github-readme-streak-stats) |
| Activity graph | [Ashutosh00710/github-readme-activity-graph](https://github.com/Ashutosh00710/github-readme-activity-graph) |
| Profile views | [komarev.com/ghpvc](https://github.com/antonkomarev/github-profile-views-counter) |

The service runs with the profile owner's own token, so private repositories and private contributions are counted. Only aggregate numbers show up on the cards; no private repository names do.

## Endpoints

Dokploy’s Traefik routes the domain straight to the `cards` service. Query parameters are the upstream ones, English only (no `locale`).

| Path | Card | Required parameter |
| --- | --- | --- |
| `/api` | stats card | `username` |
| `/api/top-langs` | top languages | `username` |
| `/streak` | streak card | `user` |
| `/graph` | activity graph | `username` |
| `/oss` | third-party PRs card | `username` |
| `/views` | profile view counter | `username` |
| `/healthz` | service health | none |

### `/oss`: open source contributions

Lists your pull requests to other people's public repositories, refreshed from the GitHub API at most every 15 minutes. Each repository appears once, newest first, showing its latest PR; its other PRs are summarised on a line below, like `+2 merged, +1 open`. A repository counts as third-party when it's public, not owned by you, and your token has no push access to it, so employer and client repos you can write to drop out automatically.

| Parameter | Default | |
| --- | --- | --- |
| `limit` | `6` | repositories to show, 1–10 |
| `theme` | `gruvbox` | also `dark`, `default` |
| `show_open` | `true` | `false` lists merged PRs only |
| `hide_border` | `false` | |
| `card_width` | `495` | up to 1200; from 900 the PRs are laid out in two columns, so `990` fills a README row like the activity graph |

Closed-unmerged PRs are never shown. The subtitle counts merged PRs and projects among your 100 most recent PRs.

### `/views`: profile view counter

A drop-in for `komarev.com/ghpvc` with `label` and `color` (named shields color or hex). Every request increments the count, and the response is never cached, so camo refetches on each profile view. The count lives in the `views-data` volume; `INITIAL_VIEWS` seeds it on first start only.

```md
![Stats](https://stats.fusan.me/api?username=FuSan21&theme=gruvbox&show_icons=true)
![Top languages](https://stats.fusan.me/api/top-langs?username=FuSan21&layout=compact&theme=gruvbox)
![Streak](https://stats.fusan.me/streak?user=FuSan21&theme=gruvbox&hide_border=true)
![Activity graph](https://stats.fusan.me/graph?username=FuSan21&theme=gruvbox)
![Open source](https://stats.fusan.me/oss?username=FuSan21&theme=gruvbox)
![Views](https://stats.fusan.me/views?username=FuSan21)
```

## Protections

The token can see private repositories and has a quota, so the service guards both itself:

- **Single user.** Any other username gets `403`, so nobody else can spend the token's quota.
- **Aggregate cards only.** The upstream routes that would expose a single private repository or raw data (`/api/pin`, `/api/gist`, `/api/wakatime`, the graph's `/data`) are not implemented; any other path is `404`.
- **GitHub call budget.** At most 100 GitHub calls in a burst, refilling one every 12 seconds (about 400 an hour, against GitHub's 5,000). Over budget, cards that need fresh data get a `503` error card; cached cards keep working. `commits_year` and `starting_year` outside GitHub's lifetime are ignored, so they can't mint new cache keys.
- **Caching.** GitHub data is kept for 15 minutes, shared across themes and layouts, and the last good data keeps being served if a refresh fails. Cards are sent with `Cache-Control: public, max-age=3600`, which GitHub's image proxy (camo) honours; `/views` is never cached.

Errors are sent with a non-`200` status (`403` user not allowed, `502` GitHub unreachable or token rejected, `503` over the call budget) and `no-store`, so they are never cached. Check the `cards` logs for the underlying GitHub error.

## Token

Create a **classic** personal access token as the profile owner with scopes `repo` and `read:user`, no expiry or a long one. A token from another account only sees public data.

The streak and graph read the contribution calendar, so they match what your profile shows. With **Include private contributions on my profile** turned on (as it is for FuSan21), the public instances already counted private contributions there. The stats and top-languages cards are the ones that gain from the token: private commits, PRs, issues and repository languages.

## Deploy on Dokploy

1. Create a Compose service with source **Git**, pointing at this repository, compose path `./docker-compose.yml`.
2. Environment: `GITHUB_USER`, `GITHUB_TOKEN`, `TZ` (see `.env.example`).
3. Domain: `stats.fusan.me` on service `cards`, port `7000`, HTTPS with Let's Encrypt.
4. Deploy. Check `https://stats.fusan.me/healthz`.

## Run locally

```sh
cp .env.example .env   # fill in GITHUB_TOKEN
docker compose -f docker-compose.yml -f - up -d --build <<'EOF'
services:
  cards:
    ports: ["8089:7000"]
EOF
curl "localhost:8089/api?username=FuSan21"
```

## Development

```sh
cd cards
go test -race ./...  # against a fake GitHub API, no token needed
go vet ./...
GITHUB_USER=FuSan21 GITHUB_TOKEN=... VIEWS_FILE=./views.json go run .
```

Standard library only, no third-party modules. The image is a static binary on `scratch`; zone data (`time/tzdata`) and the themes (`embed`) are compiled in.

Layout:

- `main.go` starts the server and flushes the view count every 5 seconds and on shutdown.
- `internal/app` routes each path to a card and enforces the single user.
- `internal/cards` has one file per card, each with its GitHub query and its SVG renderer, plus `frame.go` for the readme-stats card frame and colours.
- `internal/cards/themes/*.json` holds each upstream project's theme table: 62 readme-stats, 166 streak and 38 graph themes.
- `internal/gh` is the GitHub client and data cache. `internal/render` holds query parameters and SVG helpers.
- `bench/` benchmarks two builds side by side; see [Benchmarks](#benchmarks).

### Differences from upstream

- **No `locale`.** All text is English.
- **Every page of repositories is read.** The top-languages card reads all of them, not just the first 100, and the stats card keeps paging while repositories have stars. With one user, the token has quota to spare.
- **The activity graph is plain SVG.** It is drawn directly as a monotone curve, where upstream rendered it with Chartist in jsdom. The title is an SVG `<text>` rather than an HTML `foreignObject`.
- **The streak card's `date_format` takes PHP `date()` letters.** Only `d j D l m n M F y Y` are supported, and `[...]` hides a part in the current year.
- **Colour parameters must be hex** (or a CSS colour name on the streak card). Anything else falls back to the theme colour, so a parameter can't inject markup.

## Benchmarks

`bench/` runs two builds side by side against a fake GitHub API and measures image size, memory, startup, and throughput and latency per card. [`bench/RESULTS.md`](bench/RESULTS.md) has the Bun vs Go comparison on a Raspberry Pi 5. In short, Go uses 4–6× less memory (9 MiB idle against 42 MiB), its image is 15× smaller (8 MB against 115 MB), and it serves up to 1.6× more requests by using both cores. Bun builds about 15 s faster and has slightly lower tail latency under heavy load.
