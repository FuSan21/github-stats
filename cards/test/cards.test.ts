import { describe, expect, test } from "bun:test";
import { monotonePath, renderGraph, yTicks, graphRange } from "../src/cards/graph.ts";
import { calculateRank, renderStats, type Stats } from "../src/cards/stats.ts";
import { dailyStats, formatDate, normalizeDays, renderStreak, untilToday, weeklyStats, type Calendar } from "../src/cards/streak.ts";
import { renderTopLangs, topLanguages } from "../src/cards/top-langs.ts";
import { Params } from "../src/lib/params.ts";
import { kFormat, measureText } from "../src/lib/svg.ts";
import { assertWellFormed } from "./xml.ts";

const params = (q = "") => new Params(new URLSearchParams(q));

describe("helpers", () => {
  test("kFormat matches readme-stats", () => {
    expect(kFormat(999)).toBe("999");
    expect(kFormat(1234)).toBe("1.2k");
    expect(kFormat(1000)).toBe("1k");
    expect(kFormat(1234, 2)).toBe("1.23k");
  });

  test("measureText uses the upstream width table", () => {
    // "Most Used Languages" at 10px, from readme-stats' table.
    expect(measureText("Most Used Languages")).toBeCloseTo(99.92, 2);
    expect(measureText("é")).toBeCloseTo(5.279, 2);
  });
});

describe("rank", () => {
  test("levels at the extremes", () => {
    const zero = { allCommits: false, commits: 0, prs: 0, issues: 0, reviews: 0, stars: 0, followers: 0 };
    expect(calculateRank(zero)).toEqual({ level: "C", percentile: 100 });
    const huge = { allCommits: false, commits: 1e6, prs: 1e6, issues: 1e6, reviews: 1e6, stars: 1e9, followers: 1e9 };
    expect(calculateRank(huge).level).toBe("S");
  });

  test("matches readme-stats for a typical profile", () => {
    // Worked by hand from the readme-stats formula: weighted CDF sum 10.4975 / 12.
    const r = calculateRank({ allCommits: false, commits: 812, prs: 120, issues: 34, reviews: 14, stars: 1535, followers: 40 });
    expect(r.level).toBe("A");
    expect(r.percentile).toBeCloseTo(12.5207, 3);
  });
});

const stats: Stats = {
  name: "Fu San",
  totalStars: 1535,
  totalCommits: 812,
  totalPRs: 120,
  totalPRsMerged: 100,
  totalReviews: 14,
  totalIssues: 34,
  totalDiscussionsStarted: 2,
  totalDiscussionsAnswered: 1,
  contributedTo: 23,
  followers: 40,
  repoStars: { big: 1500, small: 30, tiny: 5 },
};

describe("stats card", () => {
  test("default card", () => {
    const svg = renderStats(stats, params("theme=gruvbox&show_icons=true"));
    assertWellFormed(svg);
    expect(svg).toContain("Fu San&#39;s GitHub Stats");
    expect(svg).toContain("Total Commits (last year):");
    expect(svg).toContain(">1.5k<");
    expect(svg).toContain("fill: #fabd2f"); // gruvbox title
    expect(svg).toContain('width="467"'); // 450 + icon width
    expect(svg).not.toContain("Total PRs Reviewed");
  });

  test("show, hide, exclude_repo and long numbers", () => {
    const svg = renderStats(stats, params("show=reviews,prs_merged_percentage&hide=issues&exclude_repo=big&number_format=long"));
    assertWellFormed(svg);
    expect(svg).toContain("Total PRs Reviewed:");
    expect(svg).toContain("83.33 %");
    expect(svg).not.toContain("Total Issues");
    expect(svg).toMatch(/>35</); // 1535 - 1500
  });

  test("all-time commits and rank-only card", () => {
    expect(renderStats(stats, params(), 4321)).toContain("Total Commits:");
    const rankOnly = renderStats(stats, params("hide=stars,commits,prs,issues,contribs"));
    expect(rankOnly).toContain("Fu San&#39;s GitHub Rank");
    expect(rankOnly).toContain('width="290"');
  });

  test("escapes custom titles and rejects non-hex colours", () => {
    const svg = renderStats(stats, params(`custom_title=${encodeURIComponent("<b>&")}&title_color=red;}x`));
    assertWellFormed(svg);
    expect(svg).toContain("&lt;b&gt;&amp;");
    expect(svg).not.toContain("red;}x");
  });
});

describe("top languages", () => {
  const repos = [
    { repo: "a", langs: [{ name: "TypeScript", color: "#3178c6", size: 600 }, { name: "CSS", color: "#663399", size: 100 }] },
    { repo: "b", langs: [{ name: "Kotlin", color: null, size: 300 }, { name: "TypeScript", color: "#3178c6", size: 50 }] },
  ];

  test("aggregates bytes across repos", () => {
    expect(topLanguages(repos)).toEqual([
      { name: "TypeScript", color: "#3178c6", size: 650 },
      { name: "Kotlin", color: "#858585", size: 300 },
      { name: "CSS", color: "#663399", size: 100 },
    ]);
    expect(topLanguages(repos, { excludeRepos: ["a"] }).map((l) => l.name)).toEqual(["Kotlin", "TypeScript"]);
    // count_weight=1 multiplies by the number of repos using the language.
    expect(topLanguages(repos, { sizeWeight: 0, countWeight: 1 })[0]).toMatchObject({ name: "TypeScript", size: 2 });
  });

  test.each(["normal", "compact", "donut", "donut-vertical", "pie"])("%s layout renders", (layout) => {
    const svg = renderTopLangs(topLanguages(repos), params(`layout=${layout}&theme=gruvbox`));
    assertWellFormed(svg);
    expect(svg).toContain("Most Used Languages");
    expect(svg).toContain("Kotlin");
  });

  test("percentages, hide and empty data", () => {
    const svg = renderTopLangs(topLanguages(repos), params("layout=compact&hide=css"));
    expect(svg).toContain("TypeScript 68.42%");
    expect(svg).not.toContain("CSS ");
    expect(renderTopLangs([], params())).toContain("No languages data.");
  });
});

describe("streak", () => {
  // Wed 2026-10-21 .. Sat 2026-10-31
  const cal: Calendar = [
    ["2026-10-21", 3],
    ["2026-10-22", 1],
    ["2026-10-23", 4],
    ["2026-10-24", 0], // Saturday
    ["2026-10-25", 2],
    ["2026-10-26", 0],
    ["2026-10-27", 1],
    ["2026-10-28", 1],
    ["2026-10-29", 0],
    ["2026-10-30", 5],
    ["2026-10-31", 0], // today, not yet contributed
  ];

  test("daily streaks; an empty today doesn't break the streak", () => {
    const s = dailyStats(cal);
    expect(s.totalContributions).toBe(17);
    expect(s.firstContribution).toBe("2026-10-21");
    expect(s.longestStreak).toEqual({ start: "2026-10-21", end: "2026-10-23", length: 3 });
    expect(s.currentStreak).toEqual({ start: "2026-10-30", end: "2026-10-30", length: 1 });
  });

  test("excluded days bridge a streak", () => {
    expect(normalizeDays(["saturday", " MON", "xyz"])).toEqual(["Sat", "Mon"]);
    const s = dailyStats(cal, ["Sat", "Mon"]);
    expect(s.longestStreak).toEqual({ start: "2026-10-21", end: "2026-10-28", length: 8 });
  });

  test("weekly streaks", () => {
    const s = weeklyStats(cal);
    expect(s.longestStreak).toEqual({ start: "2026-10-18", end: "2026-10-25", length: 2 });
    expect(s.currentStreak.length).toBe(2);
    expect(s.firstContribution).toBe("2026-10-21");
  });

  test("tomorrow only counts once it has contributions", () => {
    expect(untilToday([["2026-10-31", 1], ["2026-11-01", 0]], "2026-10-31")).toHaveLength(1);
    expect(untilToday([["2026-10-31", 1], ["2026-11-01", 2]], "2026-10-31")).toHaveLength(2);
  });

  test("date formats", () => {
    expect(formatDate("2026-03-05", undefined, 2026)).toBe("Mar 5");
    expect(formatDate("2024-03-05", undefined, 2026)).toBe("Mar 5, 2024");
    expect(formatDate("2024-03-05", "j/n[/Y]", 2026)).toBe("5/3/2024");
    expect(formatDate("2026-03-05", "j/n[/Y]", 2026)).toBe("5/3");
    expect(formatDate("2026-03-05", "D, d.m", 2026)).toBe("Thu, 05.03");
  });

  test("renders with theme, overrides and hidden columns", () => {
    const svg = renderStreak(dailyStats(cal), params("theme=gruvbox&hide_border=true&ring=ff0000"), 2026);
    assertWellFormed(svg);
    expect(svg).toContain("Current Streak");
    expect(svg).toContain("Oct 21 - Oct 23");
    expect(svg).toContain('stroke="#0000"');
    expect(svg).toContain('stroke="#ff0000"');
    const two = renderStreak(dailyStats(cal), params("hide_total_contributions=true"), 2026);
    assertWellFormed(two);
    expect(two).not.toContain("Total Contributions");
  });
});

describe("activity graph", () => {
  test("integer ticks with at least 20px between them", () => {
    expect(yTicks(0, 270)).toEqual([0, 1]);
    expect(yTicks(7, 270)).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
    expect(yTicks(40, 270)).toEqual([0, 5, 10, 15, 20, 25, 30, 35, 40]);
    expect(yTicks(41, 270).at(-1)).toBe(45);
  });

  test("monotone curve never dips below its points", () => {
    const pts = [0, 10, 0, 0, 8].map((v, i) => ({ x: i * 10, y: 100 - v }));
    const ys = [...monotonePath(pts).matchAll(/,(-?[\d.]+)/g)].map((m) => Number(m[1]));
    expect(Math.max(...ys)).toBeLessThanOrEqual(100);
  });

  test("range from days or from/to", () => {
    const now = Date.parse("2026-10-31T12:00:00Z");
    expect(graphRange(params("days=7"), now)).toMatchObject({ custom: false, days: 7 });
    expect(graphRange(params("days=500"), now).days).toBe(31);
    expect(graphRange(params("from=2026-10-01&to=2026-10-11"), now)).toMatchObject({ custom: true, days: 10 });
    expect(graphRange(params("from=2026-10-11&to=2026-10-01"), now).custom).toBe(false);
  });

  test("renders", () => {
    const days = Array.from({ length: 31 }, (_, i) => ({ date: `2026-10-${String(i + 1).padStart(2, "0")}`, count: i % 4 }));
    const svg = renderGraph("Fu San", days, params("theme=gruvbox&area=true&hide_border=true"), "FuSan21");
    assertWellFormed(svg);
    expect(svg).toContain("Fu San&#39;s Contribution Graph");
    expect(svg).toContain('class="ct-area"');
    expect(svg).toContain('stroke="#0000"');
    expect(svg.match(/class="ct-point"/g)).toHaveLength(31);
  });
});
