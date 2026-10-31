// GET /streak: total contributions, current streak and longest streak,
// following DenverCoder1/github-readme-streak-stats.

import THEMES from "../themes/streak.json";
import { GitHubError, cached, graphql } from "../lib/github.ts";
import type { Params } from "../lib/params.ts";
import { esc, gradientDef, isHex, parseGradient } from "../lib/svg.ts";

/** Contribution count per day, YYYY-MM-DD keys in ascending order. */
export type Calendar = [date: string, count: number][];

const QUERY = `
query ($login: String!, $from: DateTime!, $to: DateTime!) {
  user(login: $login) {
    createdAt
    contributionsCollection(from: $from, to: $to) {
      contributionYears
      contributionCalendar { weeks { contributionDays { contributionCount date } } }
    }
  }
}`;

type Response = {
  user: {
    createdAt: string;
    contributionsCollection: {
      contributionYears: number[];
      contributionCalendar: { weeks: { contributionDays: { contributionCount: number; date: string }[] }[] };
    };
  } | null;
};

const fetchYear = async (login: string, year: number) => {
  const data = await graphql<Response>(QUERY, { login, from: `${year}-01-01T00:00:00Z`, to: `${year}-12-31T23:59:59Z` });
  if (!data.user) throw new GitHubError("Could not find that user", 404);
  return data.user;
};

/** Every day since the account was created (or `startingYear`), all years fetched in parallel. */
export function fetchCalendar(login: string, startingYear?: number): Promise<Calendar> {
  return cached(`calendar:${login}:${startingYear ?? ""}`, async () => {
    const currentYear = Number(today().slice(0, 4));
    const current = await fetchYear(login, currentYear);
    const created = Number(current.createdAt.slice(0, 4));
    const years: number[] = [];
    for (let y = Math.max(startingYear || created, 2005); y < currentYear; y++) years.push(y);
    // Backdated commits (e.g. to 1970) show up as contribution years before Git existed.
    const first = current.contributionsCollection.contributionYears.at(-1) ?? created;
    if (first < 2005) years.unshift(first);

    const all = [...(await Promise.all(years.map((y) => fetchYear(login, y)))), current];
    const days = new Map<string, number>();
    for (const u of all)
      for (const w of u.contributionsCollection.contributionCalendar.weeks)
        for (const d of w.contributionDays) days.set(d.date, d.contributionCount);
    return [...days].sort(([a], [b]) => (a < b ? -1 : 1));
  });
}

// ---------------------------------------------------------------- dates

const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

const parseDate = (d: string) => new Date(`${d}T00:00:00Z`);
const isoDate = (d: Date) => d.toISOString().slice(0, 10);
const addDays = (d: string, n: number) => isoDate(new Date(parseDate(d).getTime() + n * 86_400_000));
const weekday = (d: string) => DAY_NAMES[parseDate(d).getUTCDay()]!;
const previousSunday = (d: string) => addDays(d, -parseDate(d).getUTCDay());

/** Today's date in `timeZone` (default: the process TZ). */
export function today(timeZone?: string, now = new Date()): string {
  try {
    return new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
  } catch {
    throw new GitHubError("Invalid timezone.", 400);
  }
}

/** Days up to today, plus tomorrow if it already has contributions (the server may be behind GitHub). */
export const untilToday = (cal: Calendar, todayDate: string): Calendar => {
  const tomorrow = addDays(todayDate, 1);
  return cal.filter(([d, n]) => d <= todayDate || (d === tomorrow && n > 0));
};

// ---------------------------------------------------------------- stats

type Streak = { start: string; end: string; length: number };
export type StreakStats = {
  mode: "daily" | "weekly";
  totalContributions: number;
  firstContribution: string;
  currentStreak: Streak;
  longestStreak: Streak;
  excludedDays: string[];
};

/** Normalise "monday, Tue" to ["Mon", "Tue"], dropping anything that isn't a weekday. */
export const normalizeDays = (days: string[]) =>
  days.map((d) => d.trim().toLowerCase().replace(/^./, (c) => c.toUpperCase()).slice(0, 3)).filter((d) => DAY_NAMES.includes(d));

function streaks(entries: [string, number][], skip: (date: string) => boolean): Omit<StreakStats, "mode" | "excludedDays"> {
  if (!entries.length) throw new GitHubError("No contributions found.", 404);
  const last = entries.at(-1)![0];
  const first = entries[0]![0];
  const stats = { totalContributions: 0, firstContribution: "", longestStreak: { start: first, end: first, length: 0 }, currentStreak: { start: first, end: first, length: 0 } };
  const cur = stats.currentStreak;
  for (const [date, count] of entries) {
    stats.totalContributions += count;
    if (count > 0 || (cur.length > 0 && skip(date))) {
      cur.length++;
      cur.end = date;
      if (cur.length === 1) cur.start = date;
      if (!stats.firstContribution && count > 0) stats.firstContribution = date;
      if (cur.length > stats.longestStreak.length) stats.longestStreak = { ...cur };
    } else if (date !== last) {
      // The latest day (or week) doesn't break the streak yet; it isn't over.
      Object.assign(cur, { start: last, end: last, length: 0 });
    }
  }
  return stats;
}

export function dailyStats(cal: Calendar, excludedDays: string[] = []): StreakStats {
  const skip = (d: string) => excludedDays.includes(weekday(d));
  return { mode: "daily", excludedDays, ...streaks(cal, skip) };
}

export function weeklyStats(cal: Calendar): StreakStats {
  const weeks = new Map<string, number>();
  for (const [d, n] of cal) weeks.set(previousSunday(d), (weeks.get(previousSunday(d)) ?? 0) + n);
  const stats = streaks([...weeks], () => false);
  // The first contribution is a day, not a week.
  stats.firstContribution = cal.find(([, n]) => n > 0)?.[0] ?? "";
  return { mode: "weekly", excludedDays: [], ...stats };
}

// --------------------------------------------------------------- render

type Theme = Record<"background" | "border" | "stroke" | "ring" | "fire" | "currStreakNum" | "sideNums" | "currStreakLabel" | "sideLabels" | "dates" | "excludeDaysLabel", string>;
const themes: Record<string, Theme> = THEMES;
const CSS_NAME = /^[a-z]{3,20}$/;

function theme(p: Params) {
  const name = (p.str("theme") ?? "default").toLowerCase().replaceAll("_", "-");
  const t: Theme = { ...(themes[name] ?? themes.default!) };
  for (const key of Object.keys(t) as (keyof Theme)[]) {
    const v = p.str(key)?.toLowerCase();
    if (isHex(v)) t[key] = `#${v}`;
    else if (v && CSS_NAME.test(v)) t[key] = v;
    else if (key === "background" && parseGradient(v)) t[key] = v!;
  }
  if (p.bool("hide_border")) t.border = "#0000";
  const gradient = parseGradient(t.background);
  if (gradient) t.background = "url(#gradient)";
  return { ...t, gradient: gradient ? gradientDef("gradient", gradient) : "" };
}

/**
 * Format a date as "Oct 31" in the current year and "Oct 31, 2024" otherwise.
 * A `date_format` uses PHP date() letters (d j D l m n M F y Y); text in
 * [brackets] is only shown for other years, e.g. "M j[, Y]".
 */
export function formatDate(date: string, format: string | undefined, currentYear: number): string {
  const d = parseDate(date);
  const sameYear = d.getUTCFullYear() === currentYear;
  if (!format) {
    const month = MONTHS[d.getUTCMonth()]!.slice(0, 3);
    return sameYear ? `${month} ${d.getUTCDate()}` : `${month} ${d.getUTCDate()}, ${d.getUTCFullYear()}`;
  }
  const pattern = sameYear ? format.replace(/\[.*?\]/g, "") : format.replace(/[[\]]/g, "");
  const pad = (n: number) => String(n).padStart(2, "0");
  const parts: Record<string, () => string> = {
    d: () => pad(d.getUTCDate()),
    j: () => String(d.getUTCDate()),
    D: () => DAY_NAMES[d.getUTCDay()]!,
    l: () => ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"][d.getUTCDay()]!,
    m: () => pad(d.getUTCMonth() + 1),
    n: () => String(d.getUTCMonth() + 1),
    M: () => MONTHS[d.getUTCMonth()]!.slice(0, 3),
    F: () => MONTHS[d.getUTCMonth()]!,
    y: () => String(d.getUTCFullYear()).slice(-2),
    Y: () => String(d.getUTCFullYear()),
  };
  return pattern.replace(/\\?./g, (c) => (c.length === 2 ? c[1]! : (parts[c]?.() ?? c)));
}

const range = (start: string, end: string) => (start === end ? start : `${start} - ${end}`);

/** Split text over two <tspan>s when it is longer than `maxChars`, preferring " - ". */
function splitLines(text: string, maxChars: number, line1Offset: number): string {
  let lines = [text];
  if (maxChars > 0 && [...text].length > maxChars) {
    if (text.includes(" - ")) lines = text.split(" - ").map((l, i) => (i ? `- ${l}` : l));
    else {
      const words = text.split(" ");
      lines = [""];
      for (const w of words) {
        const line = lines.at(-1)!;
        if (line && (line + " " + w).length > maxChars) lines.push(w);
        else lines[lines.length - 1] = line ? `${line} ${w}` : w;
      }
    }
  }
  if (lines.length < 2) return esc(text);
  return `<tspan x="0" dy="${line1Offset}">${esc(lines[0]!)}</tspan><tspan x="0" dy="16">${esc(lines.slice(1).join(" "))}</tspan>`;
}

const FONT = `font-family="'Segoe UI', Ubuntu, sans-serif"`;
const FIRE =
  "M 1.5 0.67 C 1.5 0.67 2.24 3.32 2.24 5.47 C 2.24 7.53 0.89 9.2 -1.17 9.2 C -3.23 9.2 -4.79 7.53 -4.79 5.47 L -4.76 5.11 C -6.78 7.51 -8 10.62 -8 13.99 C -8 18.41 -4.42 22 0 22 C 4.42 22 8 18.41 8 13.99 C 8 8.6 5.41 3.79 1.5 0.67 Z M -0.29 19 C -2.07 19 -3.51 17.6 -3.51 15.86 C -3.51 14.24 -2.46 13.1 -0.7 12.74 C 1.07 12.38 2.9 11.53 3.92 10.16 C 4.31 11.45 4.51 12.81 4.51 14.2 C 4.51 16.85 2.36 19 -0.29 19 Z";

function formatNumber(n: number, short: boolean) {
  if (!short) return n.toLocaleString("en-US");
  const units = ["", "K", "M", "B", "T"];
  let i = 0;
  for (; n >= 1000 && i < units.length - 1; i++) n /= 1000;
  return Math.round(n * 10) / 10 + units[i]!;
}

export function renderStreak(stats: StreakStats, p: Params, currentYear = new Date().getFullYear()): string {
  const t = theme(p);
  const radius = p.num("border_radius") ?? 4.5;
  const show = {
    total: !p.bool("hide_total_contributions"),
    current: !p.bool("hide_current_streak"),
    longest: !p.bool("hide_longest_streak"),
  };
  const columns = Number(show.total) + Number(show.current) + Number(show.longest);
  const width = Math.max(100 * columns, p.int("card_width") ?? 495);
  const height = Math.max(170, p.int("card_height") ?? 195);
  const colWidth = columns ? width / columns : 0;
  const dy = (height - 195) / 2;
  const short = p.bool("short_numbers");
  const fmtDate = (d: string) => formatDate(d, p.str("date_format"), currentYear);
  const animations = !p.bool("disable_animations");
  const anim = (delay: number) => (animations ? `opacity: 0; animation: fadein 0.5s linear forwards ${delay}s` : "");

  const offsets = Array.from({ length: columns }, (_, i) => colWidth / 2 + colWidth * i);
  let next = 0;
  const x = {
    total: show.total ? offsets[next++]! : -999,
    current: show.current ? offsets[next++]! : -999,
    longest: show.longest ? offsets[next++]! : -999,
  };
  const bars = Array.from({ length: columns - 1 }, (_, i) => colWidth * (i + 1));

  const labelChars = columns ? Math.floor(width / columns / 7.5) : 0;
  const dateChars = columns ? Math.floor(width / columns / 6) : 0;
  const weekly = stats.mode === "weekly";

  const totalRange = splitLines(`${stats.firstContribution ? fmtDate(stats.firstContribution) : ""} - Present`, dateChars, 0);
  const currentRange = splitLines(range(fmtDate(stats.currentStreak.start), fmtDate(stats.currentStreak.end)), dateChars, 0);
  const longestRange = splitLines(range(fmtDate(stats.longestStreak.start), fmtDate(stats.longestStreak.end)), dateChars, 0);

  const text = (tx: number, ty: number, fill: string, weight: number, size: number, style: string, content: string, y = 32) =>
    `<g transform="translate(${tx}, ${ty})"><text x="0" y="${y}" stroke-width="0" text-anchor="middle" fill="${fill}" stroke="none" ${FONT} font-weight="${weight}" font-size="${size}px" font-style="normal" style="${style}">${content}</text></g>`;

  const excluded = stats.excludedDays.length
    ? `<g transform="translate(5,187)"><text stroke-width="0" fill="${t.excludeDaysLabel}" stroke="none" ${FONT} font-weight="400" font-size="10px" font-style="normal" style="${anim(0.9)}">* Excluding ${esc(stats.excludedDays.join(", "))}</text></g>`
    : "";

  return `<svg xmlns="http://www.w3.org/2000/svg" style="isolation: isolate" viewBox="0 0 ${width} ${height}" width="${width}px" height="${height}px">
<title>${esc(`${formatNumber(stats.totalContributions, short)} contributions, current streak ${stats.currentStreak.length}, longest streak ${stats.longestStreak.length}`)}</title>
<style>
  @keyframes currstreak { 0% { font-size: 3px; opacity: 0.2; } 80% { font-size: 34px; opacity: 1; } 100% { font-size: 28px; opacity: 1; } }
  @keyframes fadein { 0% { opacity: 0; } 100% { opacity: 1; } }
</style>
<defs>
  <clipPath id="outer_rectangle"><rect width="${width}" height="${height}" rx="${radius}"/></clipPath>
  <mask id="mask_out_ring_behind_fire"><rect width="${width}" height="${height}" fill="white"/><ellipse cx="${x.current}" cy="32" rx="13" ry="18" fill="black"/></mask>
  ${t.gradient}
</defs>
<g clip-path="url(#outer_rectangle)">
  <rect stroke="${t.border}" fill="${t.background}" rx="${radius}" x="0.5" y="0.5" width="${width - 1}" height="${height - 1}"/>
  ${bars.map((bx) => `<line x1="${bx}" y1="${28 + dy / 2}" x2="${bx}" y2="${170 + dy}" vector-effect="non-scaling-stroke" stroke-width="1" stroke="${t.stroke}" stroke-linejoin="miter" stroke-linecap="square" stroke-miterlimit="3"/>`).join("")}
  ${
    show.total
      ? text(x.total, 48 + dy, t.sideNums, 700, 28, anim(0.6), formatNumber(stats.totalContributions, short)) +
        text(x.total, 84 + dy, t.sideLabels, 400, 14, anim(0.7), splitLines("Total Contributions", labelChars, -9)) +
        text(x.total, 114 + dy, t.dates, 400, 12, anim(0.8), totalRange)
      : ""
  }
  ${
    show.current
      ? text(x.current, 108 + dy, t.currStreakLabel, 700, 14, anim(0.9), splitLines(weekly ? "Week Streak" : "Current Streak", labelChars, -9)) +
        text(x.current, 145 + dy, t.dates, 400, 12, anim(0.9), currentRange, 21) +
        `<g mask="url(#mask_out_ring_behind_fire)"><circle cx="${x.current}" cy="${71 + dy}" r="40" fill="none" stroke="${t.ring}" stroke-width="5" style="${anim(0.4)}"/></g>` +
        `<g transform="translate(${x.current}, ${19.5 + dy})" stroke-opacity="0" style="${anim(0.6)}"><path d="M -12 -0.5 L 15 -0.5 L 15 23.5 L -12 23.5 L -12 -0.5 Z" fill="none"/><path d="${FIRE}" fill="${t.fire}" stroke-opacity="0"/></g>` +
        text(x.current, 48 + dy, t.currStreakNum, 700, 28, animations ? "animation: currstreak 0.6s linear forwards" : "", formatNumber(stats.currentStreak.length, short))
      : ""
  }
  ${
    show.longest
      ? text(x.longest, 48 + dy, t.sideNums, 700, 28, anim(1.2), formatNumber(stats.longestStreak.length, short)) +
        text(x.longest, 84 + dy, t.sideLabels, 400, 14, anim(1.3), splitLines(weekly ? "Longest Week Streak" : "Longest Streak", labelChars, -9)) +
        text(x.longest, 114 + dy, t.dates, 400, 12, anim(1.4), longestRange)
      : ""
  }
  ${excluded}
</g>
</svg>`;
}

/** The streak app's error card: a sad face and the message, in the requested theme. */
export function streakError(message: string, p: Params): string {
  const t = theme(p);
  const width = Math.max(300, p.int("card_width") ?? 495);
  const height = Math.max(170, p.int("card_height") ?? 195);
  const cx = width / 2;
  const radius = p.num("border_radius") ?? 4.5;
  return `<svg xmlns="http://www.w3.org/2000/svg" style="isolation: isolate" viewBox="0 0 ${width} ${height}" width="${width}px" height="${height}px">
<defs><clipPath id="outer_rectangle"><rect width="${width}" height="${height}" rx="${radius}"/></clipPath>${t.gradient}</defs>
<g clip-path="url(#outer_rectangle)">
  <rect stroke="${t.border}" fill="${t.background}" rx="${radius}" x="0.5" y="0.5" width="${width - 1}" height="${height - 1}"/>
  <g transform="translate(${cx}, ${height / 2 + 10.5})"><text x="0" y="50" dy="0.25em" stroke-width="0" text-anchor="middle" fill="${t.sideLabels}" stroke="none" ${FONT} font-weight="400" font-size="14px">${esc(message)}</text></g>
  <g transform="translate(${cx}, ${(height - 195) / 2})">
    <path fill="${t.fire}" d="M0,35.8c-25.2,0-45.7,20.5-45.7,45.7s20.5,45.8,45.7,45.8s45.7-20.5,45.7-45.7S25.2,35.8,0,35.8z M0,122.3c-11.2,0-21.4-4.5-28.8-11.9c-2.9-2.9-5.4-6.3-7.4-10c-3-5.7-4.6-12.1-4.6-18.9c0-22.5,18.3-40.8,40.8-40.8 c10.7,0,20.4,4.1,27.7,10.9c3.8,3.5,6.9,7.7,9.1,12.4c2.6,5.3,4,11.3,4,17.6C40.8,104.1,22.5,122.3,0,122.3z"/>
    <path fill="${t.fire}" d="M4.8,93.8c5.4,1.1,10.3,4.2,13.7,8.6l3.9-3c-4.1-5.3-10-9-16.6-10.4c-10.6-2.2-21.7,1.9-28.3,10.4l3.9,3 C-13.1,95.3-3.9,91.9,4.8,93.8z"/>
    <circle fill="${t.fire}" cx="-15" cy="71" r="4.9"/><circle fill="${t.fire}" cx="15" cy="71" r="4.9"/>
  </g>
</g>
</svg>`;
}
