// GET /graph: the daily contribution line chart of
// Ashutosh00710/github-readme-activity-graph, drawn directly as SVG
// (upstream ran Chartist inside jsdom for this).

import THEMES from "../themes/graph.json";
import { GitHubError, cached, graphql } from "../lib/github.ts";
import type { Params } from "../lib/params.ts";
import { clamp, esc, isHex } from "../lib/svg.ts";

export type Day = { date: string; count: number };

const QUERY = `
query ($login: String!, $from: DateTime!, $to: DateTime!) {
  user(login: $login) {
    name
    contributionsCollection(from: $from, to: $to) {
      contributionCalendar { weeks { contributionDays { contributionCount date } } }
    }
  }
}`;

type Response = {
  user: { name: string | null; contributionsCollection: { contributionCalendar: { weeks: { contributionDays: { contributionCount: number; date: string }[] }[] } } } | null;
};

const DAY_MS = 86_400_000;
const DATE = /^\d{4}-\d{2}-\d{2}$/;

/** The window to plot: the last `days` days, or an explicit from/to in the past. */
export function graphRange(p: Params, now = Date.now()) {
  const from = p.str("from");
  const to = p.str("to");
  if (from && to && DATE.test(from) && DATE.test(to)) {
    const f = Date.parse(`${from}T00:00:00Z`);
    const t = Date.parse(`${to}T00:00:00Z`);
    if (f < t && t <= now) return { custom: true as const, from: new Date(f).toISOString(), to: new Date(t).toISOString(), days: Math.round((t - f) / DAY_MS) };
  }
  const d = p.int("days");
  const days = d && d > 0 && d <= 90 ? d : 31;
  // One day ahead in case this server is behind GitHub's clock.
  return { custom: false as const, from: new Date(now - days * DAY_MS).toISOString(), to: new Date(now + DAY_MS).toISOString(), days };
}

export function fetchDays(login: string, range: ReturnType<typeof graphRange>): Promise<{ name: string | null; days: Day[] }> {
  // Hour resolution keeps the cache key stable for the TTL.
  const key = range.custom ? `${range.from}:${range.to}` : `${range.days}:${range.to.slice(0, 13)}`;
  return cached(`graph:${login}:${key}`, async () => {
    const { user } = await graphql<Response>(QUERY, { login, from: range.from, to: range.to });
    if (!user) throw new GitHubError("Could not find that user", 404);
    let days = user.contributionsCollection.contributionCalendar.weeks.flatMap((w) =>
      w.contributionDays.map((d) => ({ date: d.date, count: d.contributionCount })),
    );
    if (!range.custom) {
      // An empty last day is today not having started (or no commits yet); drop it.
      if (days.at(-1)?.count === 0) days = days.slice(0, -1);
      days = days.slice(-range.days);
    }
    return { name: user.name, days };
  });
}

// --------------------------------------------------------------- render

type Colors = Record<"areaColor" | "bgColor" | "borderColor" | "color" | "titleColor" | "lineColor" | "pointColor", string>;
const themes: Record<string, Colors> = THEMES;

function colors(p: Params): Colors {
  const t = themes[p.str("theme") ?? "default"] ?? themes.default!;
  const pick = (name: string, fallback: string) => {
    const v = p.str(name);
    return isHex(v) ? v : fallback;
  };
  const color = pick("color", t.color);
  return {
    areaColor: pick("area_color", t.areaColor),
    bgColor: pick("bg_color", t.bgColor),
    borderColor: pick("border_color", p.bool("hide_border") ? "0000" : t.borderColor),
    color,
    titleColor: pick("title_color", isHex(p.str("color")) ? color : t.titleColor),
    lineColor: pick("line", t.lineColor),
    pointColor: pick("point", t.pointColor),
  };
}

/** Round integer ticks with at least `minSpace` px between them, like Chartist's onlyInteger axis. */
export function yTicks(max: number, length: number, minSpace = 20): number[] {
  const top = Math.max(max, 1);
  const maxTicks = Math.max(Math.floor(length / minSpace), 1);
  let step = 1;
  for (let mag = 1; ; mag *= 10) {
    const s = [1, 2, 5].map((m) => m * mag).find((s) => Math.ceil(top / s) <= maxTicks);
    if (s) {
      step = s;
      break;
    }
  }
  const ticks: number[] = [];
  for (let v = 0; v <= Math.ceil(top / step) * step; v += step) ticks.push(v);
  return ticks;
}

const r = (n: number) => Math.round(n * 100) / 100;

/** Monotone cubic interpolation (Fritsch–Carlson): smooth, and never overshoots below zero. */
export function monotonePath(pts: { x: number; y: number }[]): string {
  const n = pts.length;
  if (n === 0) return "";
  if (n === 1) return `M${pts[0]!.x},${pts[0]!.y}`;
  const dx = pts.slice(1).map((p, i) => p.x - pts[i]!.x);
  const slope = pts.slice(1).map((p, i) => (p.y - pts[i]!.y) / dx[i]!);
  const m = pts.map((_, i) => {
    if (i === 0) return slope[0]!;
    if (i === n - 1) return slope[n - 2]!;
    const a = slope[i - 1]!;
    const b = slope[i]!;
    return a * b <= 0 ? 0 : (3 * (dx[i - 1]! + dx[i]!)) / ((2 * dx[i]! + dx[i - 1]!) / a + (dx[i]! + 2 * dx[i - 1]!) / b);
  });
  let d = `M${pts[0]!.x},${pts[0]!.y}`;
  for (let i = 0; i < n - 1; i++) {
    const h = dx[i]! / 3;
    const p0 = pts[i]!;
    const p1 = pts[i + 1]!;
    d += ` C${r(p0.x + h)},${r(p0.y + m[i]! * h)} ${r(p1.x - h)},${r(p1.y - m[i + 1]! * h)} ${p1.x},${p1.y}`;
  }
  return d;
}


export function renderGraph(name: string | null, days: Day[], p: Params, login: string): string {
  const c = colors(p);
  const width = 1200;
  const height = clamp(p.int("height") ?? 420, 200, 600);
  const radius = clamp(p.num("radius") ?? 0, 0, 16);
  const grid = p.str("grid") !== "false";
  const area = p.bool("area");
  const title = p.bool("hide_title") ? "" : (p.str("custom_title") ?? `${name ?? login}'s Contribution Graph`);

  // Chartist layout: padding (top 80, right 50, bottom 20, left 20) plus axis offsets (y 70, x 50).
  const x1 = 90;
  const x2 = width - 50;
  const y1 = 80;
  const y2 = height - 20 - 50;
  const ticks = yTicks(Math.max(0, ...days.map((d) => d.count)), y2 - y1);
  const top = ticks.at(-1)!;
  const xAt = (i: number) => (days.length > 1 ? x1 + ((x2 - x1) * i) / (days.length - 1) : (x1 + x2) / 2);
  const yAt = (v: number) => y2 - ((y2 - y1) * v) / top;
  const pts = days.map((d, i) => ({ x: r(xAt(i)), y: r(yAt(d.count)) }));
  const line = monotonePath(pts);

  const gridLines = grid
    ? ticks.map((t) => `<line x1="${x1}" x2="${x2}" y1="${r(yAt(t))}" y2="${r(yAt(t))}" class="ct-grid"/>`).join("") +
      pts.map((pt) => `<line x1="${pt.x}" x2="${pt.x}" y1="${y1}" y2="${y2}" class="ct-grid"/>`).join("")
    : "";
  const yLabels = ticks.map((t) => `<text x="${x1 - 10}" y="${r(yAt(t) + 4.5)}" text-anchor="end" class="ct-label">${t}</text>`).join("");
  const xLabels = days
    .map((d, i) => `<text x="${pts[i]!.x}" y="${y2 + 20}" text-anchor="middle" class="ct-label">${Number(d.date.slice(8))}</text>`)
    .join("");
  const areaPath = area && pts.length ? `<path d="${line} L${pts.at(-1)!.x},${y2} L${pts[0]!.x},${y2} Z" class="ct-area"/>` : "";
  const points = pts.map((pt) => `<line x1="${pt.x}" y1="${pt.y}" x2="${r(pt.x + 0.01)}" y2="${pt.y}" class="ct-point"/>`).join("");

  return `<svg width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" fill="none" xmlns="http://www.w3.org/2000/svg" role="img">
<title>${esc(title || `${login}'s contribution graph`)}</title>
<style>
  svg { font: 600 18px 'Segoe UI', Ubuntu, Sans-Serif; user-select: none; }
  .header { font: 600 20px 'Segoe UI', Ubuntu, Sans-Serif; fill: #${c.titleColor}; }
  .ct-label { font: 400 12px 'Segoe UI', Ubuntu, Sans-Serif; fill: #${c.color}; }
  .ct-axis-title { font: 400 14px 'Segoe UI', Ubuntu, Sans-Serif; fill: #${c.color}; }
  .ct-grid { stroke: #${c.color}; stroke-width: 1px; stroke-opacity: 0.3; stroke-dasharray: 2px; }
  .ct-point { stroke-width: 10px; stroke-linecap: round; stroke: #${c.pointColor}; animation: blink 1s ease-in-out forwards; }
  .ct-line { fill: none; stroke-width: 4px; stroke-dasharray: 5000; stroke-dashoffset: 5000; stroke: #${c.lineColor}; animation: dash 5s ease-in-out forwards; }
  .ct-area { stroke: none; fill: #${c.areaColor}; fill-opacity: 0.1; }
  @keyframes blink { from { opacity: 0; transform: translateX(-20px); } to { opacity: 1; transform: translateX(0); } }
  @keyframes dash { to { stroke-dashoffset: 0; } }
</style>
<rect x="0.5" y="0.5" rx="${radius}" width="${width - 1}" height="${height - 1}" fill="#${c.bgColor}" stroke="#${c.borderColor}" stroke-width="1"/>
${title ? `<text x="${width / 2}" y="40" text-anchor="middle" class="header">${esc(title)}</text>` : ""}
<g>${gridLines}</g>
<g>${yLabels}${xLabels}</g>
<text x="${x1 + (x2 - x1) / 2}" y="${height - 15}" text-anchor="middle" class="ct-axis-title">Days</text>
<text transform="translate(30, ${y1 + (y2 - y1) / 2}) rotate(-90)" text-anchor="middle" class="ct-axis-title">Contributions</text>
${areaPath}
<path d="${line}" class="ct-line"/>
${points}
</svg>`;
}
