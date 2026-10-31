// GET /api/top-langs: the readme-stats languages card, in all five layouts.

import { cardColors, frameOptions, renderCard } from "../lib/card.ts";
import { cached, graphql } from "../lib/github.ts";
import type { Params } from "../lib/params.ts";
import { chunk, clamp, esc, flex, formatBytes, measureText } from "../lib/svg.ts";

export type RepoLanguages = { repo: string; langs: { name: string; color: string | null; size: number }[] };
export type Lang = { name: string; color: string; size: number };

const QUERY = `
query ($login: String!, $after: String) {
  user(login: $login) {
    repositories(ownerAffiliations: OWNER, isFork: false, first: 100, after: $after) {
      nodes {
        name
        languages(first: 10, orderBy: {field: SIZE, direction: DESC}) {
          edges { size node { color name } }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`;

type Response = {
  user: {
    repositories: {
      nodes: { name: string; languages: { edges: { size: number; node: { name: string; color: string | null } }[] } }[];
      pageInfo: { hasNextPage: boolean; endCursor: string | null };
    };
  };
};

/** Languages of every owned, non-fork repository (all pages). */
export function fetchRepoLanguages(login: string): Promise<RepoLanguages[]> {
  return cached(`langs:${login}`, async () => {
    const repos: RepoLanguages[] = [];
    let after: string | null = null;
    do {
      const { user }: Response = await graphql<Response>(QUERY, { login, after });
      for (const r of user.repositories.nodes)
        repos.push({ repo: r.name, langs: r.languages.edges.map((e) => ({ name: e.node.name, color: e.node.color, size: e.size })) });
      after = user.repositories.pageInfo.hasNextPage ? user.repositories.pageInfo.endCursor : null;
    } while (after);
    return repos;
  });
}

/**
 * Rank languages by bytes^size_weight * repos^count_weight, like readme-stats.
 * The defaults (1, 0) rank purely by bytes.
 */
export function topLanguages(repos: RepoLanguages[], o: { excludeRepos?: string[]; sizeWeight?: number; countWeight?: number } = {}): Lang[] {
  const exclude = new Set(o.excludeRepos ?? []);
  const totals = new Map<string, { color: string | null; size: number; count: number }>();
  for (const r of repos) {
    if (exclude.has(r.repo)) continue;
    for (const l of r.langs) {
      const t = totals.get(l.name) ?? { color: l.color, size: 0, count: 0 };
      t.size += l.size;
      t.count += 1;
      totals.set(l.name, t);
    }
  }
  return [...totals]
    .map(([name, t]) => ({ name, color: t.color ?? "#858585", size: t.size ** (o.sizeWeight ?? 1) * t.count ** (o.countWeight ?? 0) }))
    .sort((a, b) => b.size - a.size);
}

// --------------------------------------------------------------- render

const DEFAULT_WIDTH = 300;
const MIN_WIDTH = 280;
const PADDING = 25;
const COMPACT_BASE_HEIGHT = 90;
const LAYOUTS = ["normal", "compact", "donut", "donut-vertical", "pie"] as const;
type Layout = (typeof LAYOUTS)[number];

const display = (size: number, total: number, format: string) =>
  format === "bytes" ? formatBytes(size) : `${((size / total) * 100).toFixed(2)}%`;

const polar = (cx: number, cy: number, r: number, deg: number) => {
  const rad = (deg * Math.PI) / 180;
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) };
};

function compactLangNode(l: Lang, total: number, i: number, hideProgress: boolean, format: string) {
  return `<g class="stagger" style="animation-delay: ${(i + 3) * 150}ms">
  <circle cx="5" cy="6" r="5" fill="${l.color}"/>
  <text x="15" y="10" class="lang-name">${esc(l.name)} ${hideProgress ? "" : display(l.size, total, format)}</text>
</g>`;
}

/** Two columns of dot + name + value. */
function langColumns(langs: Lang[], total: number, hideProgress: boolean, format: string) {
  const columns = chunk(langs, Math.ceil(langs.length / 2)).map((col) =>
    flex(col.map((l, i) => compactLangNode(l, total, i, hideProgress, format)), 25, "column"),
  );
  const longest = langs.reduce((a, l) => (l.name.length > a.name.length ? l : a), langs[0]!);
  const gap = 20 + measureText(`${longest.name} ${((longest.size / total) * 100).toFixed(2)}%`, 11);
  return flex(columns, Math.max(gap, 150));
}

function normalLayout(langs: Lang[], width: number, total: number, format: string) {
  const barWidth = width - 95;
  return flex(
    langs.map((l, i) => {
      const delay = (i + 3) * 150;
      const pct = (l.size / total) * 100;
      return `<g class="stagger" style="animation-delay: ${delay}ms">
  <text x="2" y="15" class="lang-name">${esc(l.name)}</text>
  <text x="${barWidth + 10}" y="34" class="lang-name">${display(l.size, total, format)}</text>
  <svg width="${barWidth}" x="0" y="25">
    <rect rx="5" ry="5" x="0" y="0" width="${barWidth}" height="8" fill="#ddd"/>
    <svg width="${clamp(pct, 2, 100)}%"><rect height="8" fill="${l.color}" rx="5" ry="5" x="0" y="0" class="lang-progress" style="animation-delay: ${delay + 300}ms;"/></svg>
  </svg>
</g>`;
    }),
    40,
    "column",
  );
}

function compactLayout(langs: Lang[], width: number, total: number, hideProgress: boolean, format: string) {
  const barWidth = width - 50;
  let offset = 0;
  const bar = langs
    .map((l) => {
      const w = parseFloat(((l.size / total) * barWidth).toFixed(2));
      const rect = `<rect mask="url(#rect-mask)" x="${offset}" y="0" width="${w < 10 ? w + 10 : w}" height="8" fill="${l.color}"/>`;
      offset += w;
      return rect;
    })
    .join("");
  return `${hideProgress ? "" : `<mask id="rect-mask"><rect x="0" y="0" width="${barWidth}" height="8" fill="white" rx="5"/></mask>${bar}`}
<g transform="translate(0, ${hideProgress ? 0 : 25})">${langColumns(langs, total, hideProgress, format)}</g>`;
}

function donutLayout(langs: Lang[], width: number, total: number, format: string) {
  const cx = width / 3;
  const r = cx - 60;
  let paths: string;
  if (langs.length === 1) {
    paths = `<circle cx="${cx}" cy="${cx}" r="${r}" stroke="${langs[0]!.color}" fill="none" stroke-width="12"/>`;
  } else {
    const pcts = langs.map((l) => parseFloat(((l.size / total) * 100).toFixed(2)));
    const sum = pcts.reduce((a, b) => a + b, 0);
    let start = 0;
    paths = langs
      .map((l, i) => {
        const end = start + 3.6 * parseFloat(((pcts[i]! / sum) * 100).toFixed(2));
        const a = polar(cx, cx, r, end - 90);
        const b = polar(cx, cx, r, start - 90);
        const large = end - start <= 180 ? 0 : 1;
        start = end;
        return `<g class="stagger" style="animation-delay: ${(i + 3) * 100 + 300}ms"><path d="M ${a.x} ${a.y} A ${r} ${r} 0 ${large} 0 ${b.x} ${b.y}" stroke="${l.color}" fill="none" stroke-width="12"/></g>`;
      })
      .join("");
  }
  return `<g>${flex(langs.map((l, i) => compactLangNode(l, total, i, false, format)), 32, "column")}</g>
<g transform="translate(125, ${-45 + Math.max(langs.length - 5, 0) * 16})"><svg width="${width}" height="${width}">${paths}</svg></g>`;
}

function donutVerticalLayout(langs: Lang[], total: number, format: string) {
  const r = 80;
  const circumference = 2 * Math.PI * r;
  let indent = 0;
  const circles = langs
    .map((l, i) => {
      const len = circumference * (l.size / total);
      const out = `<g class="stagger" style="animation-delay: ${(i + 1) * 100}ms"><circle cx="150" cy="100" r="${r}" fill="transparent" stroke="${l.color}" stroke-width="25" stroke-dasharray="${circumference}" stroke-dashoffset="${indent}"/></g>`;
      indent += len;
      return out;
    })
    .join("");
  return `<svg><g><svg>${circles}</svg></g>
<g transform="translate(0, 220)"><svg x="${PADDING}">${langColumns(langs, total, false, format)}</svg></g></svg>`;
}

function pieLayout(langs: Lang[], total: number, format: string) {
  const [r, cx, cy] = [90, 150, 100];
  let start = 0;
  const paths =
    langs.length === 1
      ? `<circle cx="${cx}" cy="${cy}" r="${r}" stroke="none" fill="${langs[0]!.color}"/>`
      : langs
          .map((l, i) => {
            const angle = (l.size / total) * 360;
            const a = polar(cx, cy, r, start);
            const b = polar(cx, cy, r, start + angle);
            start += angle;
            return `<g class="stagger" style="animation-delay: ${(i + 1) * 100}ms"><path d="M ${cx} ${cy} L ${a.x} ${a.y} A ${r} ${r} 0 ${angle > 180 ? 1 : 0} 1 ${b.x} ${b.y} Z" fill="${l.color}"/></g>`;
          })
          .join("");
  return `<svg><g><svg>${paths}</svg></g>
<g transform="translate(0, 220)"><svg x="${PADDING}">${langColumns(langs, total, false, format)}</svg></g></svg>`;
}

export function renderTopLangs(all: Lang[], p: Params): string {
  const colors = cardColors(p);
  const frame = frameOptions(p);
  const layoutParam = p.str("layout");
  const layout: Layout = (LAYOUTS as readonly string[]).includes(layoutParam ?? "") ? (layoutParam as Layout) : "normal";
  const hideProgress = p.bool("hide_progress");
  const format = p.str("stats_format") === "bytes" ? "bytes" : "percentages";

  const defaultCount = layout === "compact" || hideProgress ? 6 : layout === "donut" || layout === "normal" ? 5 : 6;
  const hidden = new Set(p.list("hide").map((s) => s.toLowerCase()));
  const langs = all.filter((l) => !hidden.has(l.name.toLowerCase())).slice(0, clamp(p.int("langs_count") ?? defaultCount, 1, 20));
  const total = langs.reduce((a, l) => a + l.size, 0);

  let width = Math.max(p.int("card_width") ?? DEFAULT_WIDTH, MIN_WIDTH);
  let height: number;
  let body: string;
  const half = Math.round(langs.length / 2);

  if (!langs.length) {
    height = COMPACT_BASE_HEIGHT;
    body = `<text x="${layout === "pie" || layout === "donut-vertical" ? PADDING : 0}" y="11" class="stat bold" fill="${colors.textColor}">No languages data.</text>`;
  } else if (layout === "pie") {
    height = 300 + half * 25;
    body = pieLayout(langs, total, format);
  } else if (layout === "donut-vertical") {
    height = 300 + half * 25;
    body = donutVerticalLayout(langs, total, format);
  } else if (layout === "compact" || hideProgress) {
    height = COMPACT_BASE_HEIGHT + half * 25 - (hideProgress ? 25 : 0);
    body = compactLayout(langs, width, total, hideProgress, format);
  } else if (layout === "donut") {
    height = 215 + Math.max(langs.length - 5, 0) * 32;
    width += 50;
    body = donutLayout(langs, width, total, format);
  } else {
    height = 45 + (langs.length + 1) * 40;
    body = normalLayout(langs, width, total, format);
  }

  const css = `
  @keyframes slideInAnimation { from { width: 0; } to { width: calc(100%-100px); } }
  @keyframes growWidthAnimation { from { width: 0; } to { width: 100%; } }
  .stat { font: 600 14px 'Segoe UI', Ubuntu, "Helvetica Neue", Sans-Serif; fill: ${colors.textColor}; }
  @supports(-moz-appearance: auto) { .stat { font-size: 12px; } }
  .bold { font-weight: 700 }
  .lang-name { font: 400 11px "Segoe UI", Ubuntu, Sans-Serif; fill: ${colors.textColor}; }
  .stagger { opacity: 0; animation: fadeInAnimation 0.3s ease-in-out forwards; }
  #rect-mask rect { animation: slideInAnimation 1s ease-in-out forwards; }
  .lang-progress { animation: growWidthAnimation 0.6s ease-in-out forwards; }`;

  const title = frame.customTitle ?? "Most Used Languages";
  const wrapped = layout === "pie" || layout === "donut-vertical" ? body : `<svg x="${PADDING}">${body}</svg>`;
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
      a11yDesc: langs.map((l) => `${l.name}: ${display(l.size, total, format)}`).join(", "),
    },
    wrapped,
  );
}
