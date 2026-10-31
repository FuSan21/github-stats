// GET /oss: pull requests to other people's public projects.

import { env } from "../lib/env.ts";
import { cached, graphql } from "../lib/github.ts";
import type { Params } from "../lib/params.ts";
import { clamp, esc, fmt, truncate } from "../lib/svg.ts";

export type Pr = {
  title: string;
  url: string;
  state: "OPEN" | "CLOSED" | "MERGED";
  createdAt: string;
  mergedAt: string | null;
  repository: { nameWithOwner: string; isPrivate: boolean; stargazerCount: number; viewerPermission: string | null; owner: { login: string } };
};

const QUERY = `
query ($q: String!) {
  search(query: $q, type: ISSUE, first: 100) {
    nodes {
      ... on PullRequest {
        title url state createdAt mergedAt
        repository { nameWithOwner isPrivate stargazerCount viewerPermission owner { login } }
      }
    }
  }
}`;

export function fetchOssPrs(login: string): Promise<Pr[]> {
  return cached(`oss:${login}`, async () => {
    const q = `is:pr author:${login} is:public -user:${login} sort:created-desc`;
    const data = await graphql<{ search: { nodes: Partial<Pr>[] } }>(QUERY, { q });
    return (data.search.nodes as Pr[]).filter((pr) => {
      const repo = pr.repository;
      if (!repo || repo.isPrivate) return false;
      if (repo.owner.login.toLowerCase() === login.toLowerCase()) return false;
      // Push access means an employer or client repo, not a third-party project.
      return !["ADMIN", "MAINTAIN", "WRITE"].includes(repo.viewerPermission ?? "");
    });
  });
}

const THEMES = {
  gruvbox: { bg: "#282828", border: "#3c3836", title: "#fabd2f", text: "#ebdbb2", muted: "#a89984", repo: "#8ec07c", merged: "#d3869b", open: "#b8bb26" },
  dark: { bg: "#0d1117", border: "#30363d", title: "#58a6ff", text: "#e6edf3", muted: "#8b949e", repo: "#3fb950", merged: "#a371f7", open: "#3fb950" },
  default: { bg: "#fffefe", border: "#e4e2e2", title: "#2f80ed", text: "#434d58", muted: "#6a737d", repo: "#1a7f37", merged: "#8250df", open: "#1a7f37" },
};

export function renderOss(prs: Pr[], p: Params): string {
  const t = THEMES[p.str("theme") as keyof typeof THEMES] ?? THEMES.gruvbox;
  const limit = clamp(p.int("limit") || 6, 1, 10);
  const hideBorder = p.bool("hide_border");
  const showOpen = p.bool("show_open", true);
  const user = env.user;

  const shown = prs.filter((pr) => pr.state === "MERGED" || (showOpen && pr.state === "OPEN")).slice(0, limit);
  const merged = prs.filter((pr) => pr.state === "MERGED");
  const projects = new Set(merged.map((pr) => pr.repository.nameWithOwner)).size;

  const W = 495;
  const rowH = 46;
  const top = 78;
  const H = top + Math.max(shown.length, 1) * rowH + 10;
  const rows = shown.length
    ? shown
        .map((pr, i) => {
          const isMerged = pr.state === "MERGED";
          const color = isMerged ? t.merged : t.open;
          const stars = pr.repository.stargazerCount;
          return `<g transform="translate(25, ${top + i * rowH})">
  <rect x="0" y="-11" width="52" height="16" rx="8" fill="none" stroke="${color}"/>
  <text x="26" y="1" text-anchor="middle" class="pill" fill="${color}">${isMerged ? "Merged" : "Open"}</text>
  <text x="62" y="1" class="repo">${esc(truncate(pr.repository.nameWithOwner, 42))}</text>
  ${stars ? `<text x="445" y="1" text-anchor="end" class="muted">★ ${fmt(stars)}</text>` : ""}
  <text x="62" y="20" class="pr">${esc(truncate(pr.title, 58))}</text>
</g>`;
        })
        .join("\n")
    : `<text x="25" y="${top}" class="muted">No public contributions yet.</text>`;

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" role="img" aria-label="${esc(user)}'s open source contributions">
<title>${esc(user)}'s open source contributions</title>
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
