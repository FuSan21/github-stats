# github-stats

Self-hosted GitHub profile cards for a single user, deployed on Dokploy. It replaces the public instances of:

| Card | Upstream | Pinned commit |
| --- | --- | --- |
| Stats, top languages | [anuraghazra/github-readme-stats](https://github.com/anuraghazra/github-readme-stats) | `54a7985` |
| Streak | [DenverCoder1/github-readme-streak-stats](https://github.com/DenverCoder1/github-readme-streak-stats) | `ae11d32` |
| Activity graph | [Ashutosh00710/github-readme-activity-graph](https://github.com/Ashutosh00710/github-readme-activity-graph) | `0962e94` |

The apps run with the profile owner's own token, so private repositories and private contributions are counted. Only aggregate numbers show up on the cards; no private repository names do.

## Endpoints

Everything goes through the `gateway` (nginx). Query parameters are the upstream ones.

| Path | Upstream route | Required parameter |
| --- | --- | --- |
| `/api` | stats card | `username` |
| `/api/top-langs` | top languages | `username` |
| `/streak` | streak card | `user` |
| `/graph` | activity graph | `username` |
| `/healthz` | gateway health | none |

```md
![Stats](https://stats.fusan.me/api?username=FuSan21&theme=gruvbox&show_icons=true)
![Top languages](https://stats.fusan.me/api/top-langs?username=FuSan21&layout=compact&theme=gruvbox)
![Streak](https://stats.fusan.me/streak?user=FuSan21&theme=gruvbox&hide_border=true)
![Activity graph](https://stats.fusan.me/graph?username=FuSan21&theme=gruvbox)
```

## Why the gateway exists

The token can see private repositories, so the upstream apps must not be reachable directly:

- **Single user.** Any other username gets `403`, so nobody else can spend the token's quota. This is enforced in nginx for all three apps; readme-stats and streak also get `WHITELIST`.
- **Aggregate cards only.** `/api/pin` would render any private repository's name, description and stars, and `/data` returns raw daily counts. They (along with `/api/gist`, `/api/wakatime` and the streak `/demo`) return `404`.
- **Caching.** Successful responses are cached for 1 hour and served stale if GitHub errors. One client is limited to 5 req/s (burst 40).

The upstream apps return their error cards as `200`, so a bad token gets cached for up to an hour. After fixing the token, redeploy with the `gateway-cache` volume cleared.

## Token

Create a **classic** personal access token as the profile owner with scopes `repo` and `read:user`, no expiry or a long one. A token from another account only sees public data.

For private contributions to count on the streak and graph, the profile setting **Include private contributions on my profile** is not required, because the token owner always sees their own.

## Deploy on Dokploy

1. Create a Compose service with source **Git**, pointing at this repository, compose path `./docker-compose.yml`.
2. Environment: `GITHUB_USER`, `GITHUB_TOKEN`, `TZ` (see `.env.example`).
3. Domain: `stats.fusan.me` on service `gateway`, port `80`, HTTPS with Let's Encrypt.
4. Deploy. Check `https://stats.fusan.me/healthz`.

## Run locally

```sh
cp .env.example .env   # fill in GITHUB_TOKEN
docker compose -f docker-compose.yml -f - up -d --build <<'EOF'
services:
  gateway:
    ports: ["8089:80"]
EOF
curl "localhost:8089/api?username=FuSan21"
```

## Updating upstream

Bump the commit in `readme-stats/Dockerfile`, `activity-graph/Dockerfile` or the `streak` build context in `docker-compose.yml`, then redeploy.
