// Small cards the upstream apps don't provide:
//   GET /views  profile view counter badge (replaces komarev.com/ghpvc)
//   GET /oss    pull requests to other people's public projects
// No dependencies; Node 22's fetch and http are enough.

import { createServer } from "node:http";
import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { dirname } from "node:path";

const PORT = Number(process.env.PORT || 7000);
const USER = process.env.GITHUB_USER;
const TOKEN = process.env.GITHUB_TOKEN;
const DATA_FILE = process.env.VIEWS_FILE || "/data/views.json";
const INITIAL_VIEWS = Number(process.env.INITIAL_VIEWS || 0);
const OSS_TTL_MS = 15 * 60 * 1000;

if (!USER || !TOKEN) {
  console.error("GITHUB_USER and GITHUB_TOKEN are required");
  process.exit(1);
}

const esc = (s) =>
  String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);

const truncate = (s, max) => (s.length > max ? s.slice(0, max - 1).trimEnd() + "…" : s);

const fmt = (n) => n.toLocaleString("en-US");

// Rough Verdana 11px advance widths, good enough to size a badge.
const textWidth = (s) =>
  [...s].reduce((w, c) => w + (/[il.,:;'|!]/.test(c) ? 3.2 : /[mwMW]/.test(c) ? 9.5 : /[A-Z0-9]/.test(c) ? 7.5 : 6.4), 0);

// ---------------------------------------------------------------- views

let views = INITIAL_VIEWS;
let dirty = false;

try {
  views = JSON.parse(await readFile(DATA_FILE, "utf8")).views;
} catch {
  // First start: seed from INITIAL_VIEWS so the count carries over from komarev.
  dirty = true;
}

async function flush() {
  if (!dirty) return;
  dirty = false;
  await mkdir(dirname(DATA_FILE), { recursive: true });
  const tmp = `${DATA_FILE}.tmp`;
  await writeFile(tmp, JSON.stringify({ views }));
  await rename(tmp, DATA_FILE);
}

setInterval(() => flush().catch((e) => console.error("flush failed", e)), 5000).unref();

const COLORS = {
  brightgreen: "#4c1", green: "#97ca00", yellow: "#dfb317", yellowgreen: "#a4a61d",
  orange: "#fe7d37", red: "#e05d44", blue: "#007ec6", grey: "#555", lightgrey: "#9f9f9f",
};

function viewsBadge(params) {
  const label = params.get("label") || "Profile views";
  const raw = params.get("color") || "blue";
  const color = COLORS[raw] || (/^[0-9a-f]{3,8}$/i.test(raw) ? `#${raw}` : COLORS.blue);
  const value = fmt(views);
  const lw = Math.round(textWidth(label) + 12);
  const vw = Math.round(textWidth(value) + 12);
  const w = lw + vw;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="20" role="img" aria-label="${esc(label)}: ${value}">
<title>${esc(label)}: ${value}</title>
<linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
<clipPath id="r"><rect width="${w}" height="20" rx="3" fill="#fff"/></clipPath>
<g clip-path="url(#r)"><rect width="${lw}" height="20" fill="#555"/><rect x="${lw}" width="${vw}" height="20" fill="${color}"/><rect width="${w}" height="20" fill="url(#s)"/></g>
<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
<text x="${lw / 2}" y="15" fill="#010101" fill-opacity=".3">${esc(label)}</text><text x="${lw / 2}" y="14">${esc(label)}</text>
<text x="${lw + vw / 2}" y="15" fill="#010101" fill-opacity=".3">${value}</text><text x="${lw + vw / 2}" y="14">${value}</text>
</g></svg>`;
}

// ------------------------------------------------------------------ oss

const OSS_QUERY = `
query($q: String!) {
  search(query: $q, type: ISSUE, first: 100) {
    nodes {
      ... on PullRequest {
        title
        url
        state
        createdAt
        mergedAt
        repository {
          nameWithOwner
          isPrivate
          stargazerCount
          viewerPermission
          owner { login }
        }
      }
    }
  }
}`;

let ossCache = { at: 0, prs: null };

async function fetchOssPrs() {
  if (ossCache.prs && Date.now() - ossCache.at < OSS_TTL_MS) return ossCache.prs;
  const res = await fetch("https://api.github.com/graphql", {
    method: "POST",
    headers: { Authorization: `bearer ${TOKEN}`, "Content-Type": "application/json", "User-Agent": "github-stats-extras" },
    body: JSON.stringify({ query: OSS_QUERY, variables: { q: `is:pr author:${USER} is:public -user:${USER} sort:created-desc` } }),
  });
  const body = await res.json();
  if (!res.ok || body.errors) throw new Error(JSON.stringify(body.errors || body));
  const prs = body.data.search.nodes.filter((pr) => {
    const repo = pr.repository;
    if (!repo || repo.isPrivate) return false;
    if (repo.owner.login.toLowerCase() === USER.toLowerCase()) return false;
    // Push access means an employer or client repo, not a third-party project.
    return !["ADMIN", "MAINTAIN", "WRITE"].includes(repo.viewerPermission);
  });
  ossCache = { at: Date.now(), prs };
  return prs;
}

const THEMES = {
  gruvbox: { bg: "#282828", border: "#3c3836", title: "#fabd2f", text: "#ebdbb2", muted: "#a89984", repo: "#8ec07c", merged: "#d3869b", open: "#b8bb26" },
  dark: { bg: "#0d1117", border: "#30363d", title: "#58a6ff", text: "#e6edf3", muted: "#8b949e", repo: "#3fb950", merged: "#a371f7", open: "#3fb950" },
  default: { bg: "#fffefe", border: "#e4e2e2", title: "#2f80ed", text: "#434d58", muted: "#6a737d", repo: "#1a7f37", merged: "#8250df", open: "#1a7f37" },
};

function ossCard(prs, params) {
  const t = THEMES[params.get("theme")] || THEMES.gruvbox;
  const limit = Math.min(Math.max(Number(params.get("limit")) || 6, 1), 10);
  const hideBorder = params.get("hide_border") === "true";
  const showOpen = params.get("show_open") !== "false";

  const shown = prs.filter((pr) => pr.state === "MERGED" || (showOpen && pr.state === "OPEN")).slice(0, limit);
  const merged = prs.filter((pr) => pr.state === "MERGED");
  const projects = new Set(merged.map((pr) => pr.repository.nameWithOwner)).size;

  const W = 495, rowH = 46, top = 78;
  const H = top + Math.max(shown.length, 1) * rowH + 10;
  const rows = shown.length
    ? shown
        .map((pr, i) => {
          const y = top + i * rowH;
          const isMerged = pr.state === "MERGED";
          const color = isMerged ? t.merged : t.open;
          const label = isMerged ? "Merged" : "Open";
          const stars = pr.repository.stargazerCount;
          return `<g transform="translate(25, ${y})">
  <rect x="0" y="-11" width="52" height="16" rx="8" fill="none" stroke="${color}"/>
  <text x="26" y="1" text-anchor="middle" class="pill" fill="${color}">${label}</text>
  <text x="62" y="1" class="repo">${esc(truncate(pr.repository.nameWithOwner, 42))}</text>
  ${stars ? `<text x="445" y="1" text-anchor="end" class="muted">★ ${fmt(stars)}</text>` : ""}
  <text x="62" y="20" class="pr">${esc(truncate(pr.title, 58))}</text>
</g>`;
        })
        .join("\n")
    : `<text x="25" y="${top}" class="muted">No public contributions yet.</text>`;

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" role="img" aria-label="${esc(USER)}'s open source contributions">
<title>${esc(USER)}'s open source contributions</title>
<style>
  .title { font: 600 18px 'Segoe UI', Ubuntu, sans-serif; fill: ${t.title}; }
  .sub { font: 400 12px 'Segoe UI', Ubuntu, sans-serif; fill: ${t.muted}; }
  .repo { font: 600 13px 'Segoe UI', Ubuntu, sans-serif; fill: ${t.repo}; }
  .pr { font: 400 12px 'Segoe UI', Ubuntu, sans-serif; fill: ${t.text}; }
  .muted { font: 400 12px 'Segoe UI', Ubuntu, sans-serif; fill: ${t.muted}; }
  .pill { font: 600 10px 'Segoe UI', Ubuntu, sans-serif; }
</style>
<rect x="0.5" y="0.5" width="${W - 1}" height="${H - 1}" rx="4.5" fill="${t.bg}" stroke="${t.border}" stroke-opacity="${hideBorder ? 0 : 1}"/>
<text x="25" y="35" class="title">Open Source Contributions</text>
<text x="25" y="55" class="sub">${fmt(merged.length)} merged PR${merged.length === 1 ? "" : "s"} across ${fmt(projects)} project${projects === 1 ? "" : "s"}</text>
${rows}
</svg>`;
}

function errorCard(message) {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="495" height="80" viewBox="0 0 495 80"><rect x="0.5" y="0.5" width="494" height="79" rx="4.5" fill="#282828" stroke="#3c3836"/><text x="25" y="45" font-family="Segoe UI, Ubuntu, sans-serif" font-size="14" fill="#fb4934">${esc(message)}</text></svg>`;
}

// --------------------------------------------------------------- server

const server = createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const send = (status, svg, cache) => {
    res.writeHead(status, { "Content-Type": "image/svg+xml; charset=utf-8", "Cache-Control": cache });
    res.end(svg);
  };

  const username = url.searchParams.get("username") || "";
  if (username.toLowerCase() !== USER.toLowerCase()) return send(403, errorCard("This username is not allowed"), "no-store");

  if (url.pathname === "/views") {
    views += 1;
    dirty = true;
    return send(200, viewsBadge(url.searchParams), "max-age=0, no-cache, no-store, must-revalidate");
  }

  if (url.pathname === "/oss") {
    try {
      const prs = await fetchOssPrs();
      return send(200, ossCard(prs, url.searchParams), "public, max-age=3600");
    } catch (e) {
      console.error("oss fetch failed", e);
      return send(502, errorCard("Could not load contributions from GitHub"), "no-store");
    }
  }

  send(404, errorCard("Not found"), "no-store");
});

server.listen(PORT, () => console.log(`extras listening on ${PORT}`));

for (const sig of ["SIGTERM", "SIGINT"]) {
  process.on(sig, async () => {
    server.close();
    await flush().catch((e) => console.error("final flush failed", e));
    process.exit(0);
  });
}
