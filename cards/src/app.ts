// Routes a request to a card. Kept apart from server.ts so tests can call it directly.

import { fetchDays, graphRange, renderGraph } from "./cards/graph.ts";
import { fetchOssPrs, renderOss } from "./cards/oss.ts";
import { fetchAllCommits, fetchStats, renderStats } from "./cards/stats.ts";
import { dailyStats, fetchCalendar, normalizeDays, renderStreak, streakError, today, untilToday, weeklyStats } from "./cards/streak.ts";
import { fetchRepoLanguages, renderTopLangs, topLanguages } from "./cards/top-langs.ts";
import { countView, viewsBadge } from "./cards/views.ts";
import { cardColors, errorCard } from "./lib/card.ts";
import { env } from "./lib/env.ts";
import { GitHubError } from "./lib/github.ts";
import { Params } from "./lib/params.ts";

const CACHE = "public, max-age=3600";
const NO_STORE = "max-age=0, no-cache, no-store, must-revalidate";

const svg = (body: string, status = 200, cache = CACHE) =>
  new Response(body, { status, headers: { "Content-Type": "image/svg+xml; charset=utf-8", "Cache-Control": cache } });

type Route = { userParam: "username" | "user"; render: (p: Params, login: string) => Promise<string> | string; error?: (message: string, p: Params) => string };

const routes: Record<string, Route> = {
  "/api": {
    userParam: "username",
    render: async (p, login) => {
      const commitsYear = p.int("commits_year");
      const [stats, allCommits] = await Promise.all([
        fetchStats(login, commitsYear),
        p.bool("include_all_commits") ? fetchAllCommits(login) : undefined,
      ]);
      return renderStats(stats, p, allCommits);
    },
  },
  "/api/top-langs": {
    userParam: "username",
    render: async (p, login) => {
      const langs = topLanguages(await fetchRepoLanguages(login), {
        excludeRepos: p.list("exclude_repo"),
        sizeWeight: p.num("size_weight"),
        countWeight: p.num("count_weight"),
      });
      return renderTopLangs(langs, p);
    },
  },
  "/streak": {
    userParam: "user",
    render: async (p, login) => {
      const tz = p.str("timezone");
      const now = today(tz);
      const cal = untilToday(await fetchCalendar(login, p.int("starting_year")), now);
      const stats = p.str("mode") === "weekly" ? weeklyStats(cal) : dailyStats(cal, normalizeDays(p.list("exclude_days")));
      return renderStreak(stats, p, Number(now.slice(0, 4)));
    },
    error: streakError,
  },
  "/graph": {
    userParam: "username",
    render: async (p, login) => {
      const { name, days } = await fetchDays(login, graphRange(p));
      return renderGraph(name, days, p, login);
    },
  },
  "/oss": {
    userParam: "username",
    render: async (p, login) => renderOss(await fetchOssPrs(login), p),
  },
};

export async function handle(req: Request): Promise<Response> {
  const url = new URL(req.url);
  if (url.pathname === "/healthz") return new Response("ok\n", { headers: { "Content-Type": "text/plain" } });

  const p = new Params(url.searchParams);
  const route = routes[url.pathname];
  const isViews = url.pathname === "/views";
  if (!route && !isViews) return svg(errorCard("Not found", cardColors(p)), 404, "no-store");

  const fail = (message: string, status: number) =>
    svg(route?.error ? route.error(message, p) : errorCard(message, cardColors(p)), status, "no-store");

  // Only the token owner's cards: any other user would spend its quota, and
  // the token can see private data.
  const login = p.str(route?.userParam ?? "username") ?? "";
  if (login.toLowerCase() !== env.user.toLowerCase()) return fail("This username is not allowed", 403);

  if (isViews) return svg(viewsBadge(countView(), p), 200, NO_STORE);

  try {
    // Query the canonical login so the data cache is shared across spellings.
    return svg(await route!.render(p, env.user));
  } catch (e) {
    if (e instanceof GitHubError) return fail(e.message, e.status);
    console.error(`${url.pathname} failed`, e);
    return fail("Something went wrong rendering this card", 500);
  }
}
