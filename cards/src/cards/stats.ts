// GET /api: the readme-stats stats card (commits, PRs, issues, stars, rank).

import { cardColors, frameOptions, renderCard } from "../lib/card.ts";
import { cached, graphql, rest } from "../lib/github.ts";
import type { Params } from "../lib/params.ts";
import { clamp, esc, flex, kFormat, measureText } from "../lib/svg.ts";
import { GITHUB_MARK, icons } from "./icons.ts";

export type Stats = {
  name: string;
  totalStars: number;
  totalCommits: number;
  totalPRs: number;
  totalPRsMerged: number;
  totalReviews: number;
  totalIssues: number;
  totalDiscussionsStarted: number;
  totalDiscussionsAnswered: number;
  contributedTo: number;
  followers: number;
  /** Stars per owned repository name, so exclude_repo can be applied per request. */
  repoStars: Record<string, number>;
};

const REPOS_FIELD = `
  repositories(first: 100, ownerAffiliations: OWNER, orderBy: {direction: DESC, field: STARGAZERS}, after: $after) {
    nodes { name stargazers { totalCount } }
    pageInfo { hasNextPage endCursor }
  }`;

const STATS_QUERY = `
query ($login: String!, $after: String, $startTime: DateTime) {
  user(login: $login) {
    name
    login
    commits: contributionsCollection(from: $startTime) { totalCommitContributions }
    reviews: contributionsCollection { totalPullRequestReviewContributions }
    repositoriesContributedTo(first: 1, contributionTypes: [COMMIT, ISSUE, PULL_REQUEST, REPOSITORY]) { totalCount }
    pullRequests(first: 1) { totalCount }
    mergedPullRequests: pullRequests(states: MERGED) { totalCount }
    openIssues: issues(states: OPEN) { totalCount }
    closedIssues: issues(states: CLOSED) { totalCount }
    followers { totalCount }
    repositoryDiscussions { totalCount }
    repositoryDiscussionComments(onlyAnswers: true) { totalCount }
    ${REPOS_FIELD}
  }
}`;

const REPOS_QUERY = `query ($login: String!, $after: String) { user(login: $login) { ${REPOS_FIELD} } }`;

type Repos = { nodes: { name: string; stargazers: { totalCount: number } }[]; pageInfo: { hasNextPage: boolean; endCursor: string | null } };
type Count = { totalCount: number };
type StatsResponse = {
  user: {
    name: string | null;
    login: string;
    commits: { totalCommitContributions: number };
    reviews: { totalPullRequestReviewContributions: number };
    repositoriesContributedTo: Count;
    pullRequests: Count;
    mergedPullRequests: Count;
    openIssues: Count;
    closedIssues: Count;
    followers: Count;
    repositoryDiscussions: Count;
    repositoryDiscussionComments: Count;
    repositories: Repos;
  };
};

export function fetchStats(login: string, commitsYear?: number): Promise<Stats> {
  return cached(`stats:${login}:${commitsYear ?? ""}`, async () => {
    const startTime = commitsYear ? `${commitsYear}-01-01T00:00:00Z` : null;
    const { user } = await graphql<StatsResponse>(STATS_QUERY, { login, after: null, startTime });

    // Repositories come sorted by stars; stop at the first page with an unstarred one.
    const repoStars: Record<string, number> = {};
    let page: Repos = user.repositories;
    for (;;) {
      for (const r of page.nodes) repoStars[r.name] = r.stargazers.totalCount;
      if (!page.pageInfo.hasNextPage || page.nodes.some((r) => r.stargazers.totalCount === 0)) break;
      page = (await graphql<{ user: { repositories: Repos } }>(REPOS_QUERY, { login, after: page.pageInfo.endCursor })).user.repositories;
    }

    return {
      name: user.name || user.login,
      totalStars: Object.values(repoStars).reduce((a, b) => a + b, 0),
      totalCommits: user.commits.totalCommitContributions,
      totalPRs: user.pullRequests.totalCount,
      totalPRsMerged: user.mergedPullRequests.totalCount,
      totalReviews: user.reviews.totalPullRequestReviewContributions,
      totalIssues: user.openIssues.totalCount + user.closedIssues.totalCount,
      totalDiscussionsStarted: user.repositoryDiscussions.totalCount,
      totalDiscussionsAnswered: user.repositoryDiscussionComments.totalCount,
      contributedTo: user.repositoriesContributedTo.totalCount,
      followers: user.followers.totalCount,
      repoStars,
    };
  });
}

/** All-time commit count from commit search, for include_all_commits. */
export const fetchAllCommits = (login: string) =>
  cached(`all-commits:${login}`, async () => (await rest<{ total_count: number }>(`/search/commits?q=author:${encodeURIComponent(login)}`)).total_count);

// ----------------------------------------------------------------- rank

const exponentialCdf = (x: number) => 1 - 2 ** -x;
const logNormalCdf = (x: number) => x / (1 + x);

/** readme-stats' rank: a weighted percentile against fixed medians. Lower is better. */
export function calculateRank(s: { allCommits: boolean; commits: number; prs: number; issues: number; reviews: number; stars: number; followers: number }) {
  const parts: [weight: number, score: number][] = [
    [2, exponentialCdf(s.commits / (s.allCommits ? 1000 : 250))],
    [3, exponentialCdf(s.prs / 50)],
    [1, exponentialCdf(s.issues / 25)],
    [1, exponentialCdf(s.reviews / 2)],
    [4, logNormalCdf(s.stars / 50)],
    [1, logNormalCdf(s.followers / 10)],
  ];
  const total = parts.reduce((a, [w]) => a + w, 0);
  const rank = 1 - parts.reduce((a, [w, v]) => a + w * v, 0) / total;
  const thresholds = [1, 12.5, 25, 37.5, 50, 62.5, 75, 87.5, 100];
  const levels = ["S", "A+", "A", "A-", "B+", "B", "B-", "C+", "C"];
  return { level: levels[thresholds.findIndex((t) => rank * 100 <= t)] ?? "C", percentile: rank * 100 };
}

// --------------------------------------------------------------- render

const RANK_CARD_MIN_WIDTH = 420;
const RANK_CARD_DEFAULT_WIDTH = 450;
const RANK_ONLY_CARD_WIDTH = 290;
const CARD_MIN_WIDTH = 287;
const CARD_DEFAULT_WIDTH = 287;

const OPTIONAL = ["prs_merged", "prs_merged_percentage", "reviews", "discussions_started", "discussions_answered"] as const;

function rankIcon(kind: string | undefined, level: string, percentile: number) {
  if (kind === "github")
    return `<svg x="-38" y="-30" height="66" width="66" aria-hidden="true" viewBox="0 0 16 16"><path d="${GITHUB_MARK}"/></svg>`;
  if (kind === "percentile")
    return `<text x="-5" y="-12" alignment-baseline="central" dominant-baseline="central" text-anchor="middle" class="rank-percentile-header">Top</text>
<text x="-5" y="12" alignment-baseline="central" dominant-baseline="central" text-anchor="middle" class="rank-percentile-text">${percentile.toFixed(1)}%</text>`;
  return `<text x="-5" y="3" alignment-baseline="central" dominant-baseline="central" text-anchor="middle">${level}</text>`;
}

export function renderStats(stats: Stats, p: Params, allCommits?: number): string {
  const colors = cardColors(p);
  const frame = frameOptions(p);
  const hide = p.list("hide");
  const show = p.list("show");
  const showIcons = p.bool("show_icons");
  const hideRank = p.bool("hide_rank");
  const bold = p.bool("text_bold", true);
  const lineHeight = p.int("line_height") ?? 25;
  const longNumbers = p.str("number_format")?.toLowerCase() === "long";
  const precisionParam = p.int("number_precision");
  const precision = precisionParam === undefined ? undefined : clamp(precisionParam, 0, 2);
  const commitsYear = p.int("commits_year");

  const exclude = new Set(p.list("exclude_repo"));
  const stars = exclude.size
    ? Object.entries(stats.repoStars).reduce((a, [name, n]) => a + (exclude.has(name) ? 0 : n), 0)
    : stats.totalStars;
  const commits = allCommits ?? stats.totalCommits;
  const rank = calculateRank({
    allCommits: allCommits !== undefined,
    commits,
    prs: stats.totalPRs,
    issues: stats.totalIssues,
    reviews: stats.totalReviews,
    stars,
    followers: stats.followers,
  });

  const commitsLabel = `Total Commits${allCommits !== undefined ? "" : commitsYear ? ` (${commitsYear})` : " (last year)"}`;
  const mergedPct = stats.totalPRs ? (stats.totalPRsMerged / stats.totalPRs) * 100 : 0;

  type Item = { id: string; icon: string; label: string; value: number | string; unit?: string };
  const all: Item[] = [
    { id: "stars", icon: icons.star, label: "Total Stars Earned", value: stars },
    { id: "commits", icon: icons.commits, label: commitsLabel, value: commits },
    { id: "prs", icon: icons.prs, label: "Total PRs", value: stats.totalPRs },
    { id: "prs_merged", icon: icons.prs_merged, label: "Total PRs Merged", value: stats.totalPRsMerged },
    { id: "prs_merged_percentage", icon: icons.prs_merged_percentage, label: "Merged PRs Percentage", value: mergedPct.toFixed(precision ?? 2), unit: "%" },
    { id: "reviews", icon: icons.reviews, label: "Total PRs Reviewed", value: stats.totalReviews },
    { id: "issues", icon: icons.issues, label: "Total Issues", value: stats.totalIssues },
    { id: "discussions_started", icon: icons.discussions_started, label: "Total Discussions Started", value: stats.totalDiscussionsStarted },
    { id: "discussions_answered", icon: icons.discussions_answered, label: "Total Discussions Answered", value: stats.totalDiscussionsAnswered },
    { id: "contribs", icon: icons.contribs, label: "Contributed to (last year)", value: stats.contributedTo },
  ];
  const items = all.filter(
    (s) => !hide.includes(s.id) && (!(OPTIONAL as readonly string[]).includes(s.id) || show.includes(s.id)),
  );

  const nodes = items.map((s, i) => {
    const value = typeof s.value === "string" || longNumbers ? String(s.value) : kFormat(s.value, precision);
    const weight = bold ? "bold" : "not_bold";
    return `<g class="stagger" style="animation-delay: ${(i + 3) * 150}ms" transform="translate(25, 0)">
  ${showIcons ? `<svg class="icon" viewBox="0 0 16 16" version="1.1" width="16" height="16">${s.icon}</svg>` : ""}
  <text class="stat ${weight}" ${showIcons ? `x="25"` : ""} y="12.5">${esc(s.label)}:</text>
  <text class="stat ${weight}" x="${(showIcons ? 140 : 120) + 79.01}" y="12.5">${value}${s.unit ? ` ${s.unit}` : ""}</text>
</g>`;
  });

  const possessive = `${stats.name}'${/s$/i.test(stats.name.trim()) ? "" : "s"}`;
  const title = frame.customTitle ?? `${possessive} GitHub ${items.length ? "Stats" : "Rank"}`;
  const height = Math.max(45 + (items.length + 1) * lineHeight, hideRank ? 0 : items.length ? 150 : 180);

  const iconWidth = showIcons && items.length ? 17 : 0;
  const minWidth =
    (hideRank ? Math.max(50 + measureText(title) * 2, CARD_MIN_WIDTH) : items.length ? RANK_CARD_MIN_WIDTH : RANK_ONLY_CARD_WIDTH) + iconWidth;
  const defaultWidth = (hideRank ? CARD_DEFAULT_WIDTH : items.length ? RANK_CARD_DEFAULT_WIDTH : RANK_ONLY_CARD_WIDTH) + iconWidth;
  const width = Math.max(p.int("card_width") ?? defaultWidth, minWidth);

  let rankX: number;
  if (!items.length) rankX = width / 2 + 10;
  else {
    const minX = RANK_CARD_MIN_WIDTH + iconWidth - 70;
    rankX = width > RANK_CARD_DEFAULT_WIDTH ? minX + (450 - minWidth) / 2 + width - RANK_CARD_DEFAULT_WIDTH : minX + (width - minWidth) / 2;
  }

  const circumference = Math.PI * 80;
  const progress = clamp(100 - rank.percentile, 0, 100);
  const css = `
  .stat { font: 600 14px 'Segoe UI', Ubuntu, "Helvetica Neue", Sans-Serif; fill: ${colors.textColor}; }
  @supports(-moz-appearance: auto) { .stat { font-size: 12px; } }
  .stagger { opacity: 0; animation: fadeInAnimation 0.3s ease-in-out forwards; }
  .rank-text { font: 800 24px 'Segoe UI', Ubuntu, Sans-Serif; fill: ${colors.textColor}; animation: scaleInAnimation 0.3s ease-in-out forwards; }
  .rank-percentile-header { font-size: 14px; }
  .rank-percentile-text { font-size: 16px; }
  .not_bold { font-weight: 400 }
  .bold { font-weight: 700 }
  .icon { fill: ${colors.iconColor}; display: ${showIcons ? "block" : "none"}; }
  .rank-text svg { fill: ${colors.textColor}; }
  .rank-circle-rim { stroke: ${colors.ringColor}; fill: none; stroke-width: 6; opacity: 0.2; }
  .rank-circle { stroke: ${colors.ringColor}; stroke-dasharray: 250; fill: none; stroke-width: 6; stroke-linecap: round; opacity: 0.8; transform-origin: -10px 8px; transform: rotate(-90deg); animation: rankAnimation 1s forwards ease-in-out; }
  @keyframes rankAnimation { from { stroke-dashoffset: ${circumference}; } to { stroke-dashoffset: ${((100 - progress) / 100) * circumference}; } }`;

  const rankCircle = hideRank
    ? ""
    : `<g transform="translate(${rankX}, ${height / 2 - 50})">
  <circle class="rank-circle-rim" cx="-10" cy="8" r="40"/>
  <circle class="rank-circle" cx="-10" cy="8" r="40"/>
  <g class="rank-text">${rankIcon(p.str("rank_icon"), rank.level, rank.percentile)}</g>
</g>`;

  return renderCard(
    {
      width,
      height,
      title,
      colors,
      css,
      hideTitle: frame.hideTitle,
      hideBorder: frame.hideBorder,
      borderRadius: frame.borderRadius,
      animations: frame.animations,
      a11yTitle: `${title}, Rank: ${rank.level}`,
      a11yDesc: items.map((s) => `${s.label}: ${s.value}`).join(", "),
    },
    `${rankCircle}<svg x="0" y="0">${flex(nodes, lineHeight, "column")}</svg>`,
  );
}
