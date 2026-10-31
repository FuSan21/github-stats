// A stand-in for api.github.com, answering each query the cards make with fixed data.

import { mock } from "bun:test";

export const USER = "FuSan21";

/** 400 days of contributions ending 2026-10-31: every 3rd day empty, the last 5 days all active. */
export function calendarDays(end = "2026-10-31", count = 400) {
  const days: { date: string; contributionCount: number }[] = [];
  const endMs = Date.parse(`${end}T00:00:00Z`);
  for (let i = count - 1; i >= 0; i--) {
    const date = new Date(endMs - i * 86_400_000).toISOString().slice(0, 10);
    days.push({ date, contributionCount: i < 5 ? 2 + i : i % 3 === 0 ? 0 : (i % 7) + 1 });
  }
  return days;
}

const inRange = (from: string, to: string) => calendarDays().filter((d) => d.date >= from.slice(0, 10) && d.date <= to.slice(0, 10));
const weeks = (days: { date: string; contributionCount: number }[]) => [{ contributionDays: days }];

export const calls: string[] = [];
export let failNext = 0;
export const setFailNext = (n: number) => (failNext = n);

function answer(query: string, v: Record<string, any>): unknown {
  if (query.includes("totalCommitContributions"))
    return {
      user: {
        name: "Fu San",
        login: USER,
        commits: { totalCommitContributions: 812 },
        reviews: { totalPullRequestReviewContributions: 14 },
        repositoriesContributedTo: { totalCount: 23 },
        pullRequests: { totalCount: 120 },
        mergedPullRequests: { totalCount: 100 },
        openIssues: { totalCount: 4 },
        closedIssues: { totalCount: 30 },
        followers: { totalCount: 40 },
        repositoryDiscussions: { totalCount: 2 },
        repositoryDiscussionComments: { totalCount: 1 },
        repositories: {
          nodes: [
            { name: "big", stargazers: { totalCount: 1500 } },
            { name: "small", stargazers: { totalCount: 30 } },
          ],
          pageInfo: { hasNextPage: true, endCursor: "c1" },
        },
      },
    };
  if (query.includes("stargazers") && v.after === "c1")
    return { user: { repositories: { nodes: [{ name: "tiny", stargazers: { totalCount: 5 } }, { name: "none", stargazers: { totalCount: 0 } }], pageInfo: { hasNextPage: true, endCursor: "c2" } } } };
  if (query.includes("languages("))
    return {
      user: {
        repositories: {
          nodes:
            v.after === null
              ? [
                  { name: "big", languages: { edges: [{ size: 6000, node: { name: "TypeScript", color: "#3178c6" } }, { size: 1000, node: { name: "CSS", color: "#663399" } }] } },
                  { name: "small", languages: { edges: [{ size: 3000, node: { name: "Kotlin", color: "#A97BFF" } }, { size: 500, node: { name: "TypeScript", color: "#3178c6" } }] } },
                ]
              : [{ name: "other", languages: { edges: [{ size: 2000, node: { name: "Dockerfile", color: null } }] } }],
          pageInfo: v.after === null ? { hasNextPage: true, endCursor: "p2" } : { hasNextPage: false, endCursor: null },
        },
      },
    };
  if (query.includes("contributionYears"))
    return {
      user: {
        createdAt: "2025-03-01T00:00:00Z",
        contributionsCollection: { contributionYears: [2026, 2025], contributionCalendar: { weeks: weeks(inRange(v.from, v.to)) } },
      },
    };
  if (query.includes("contributionCalendar"))
    return { user: { name: "Fu San", contributionsCollection: { contributionCalendar: { weeks: weeks(inRange(v.from, v.to)) } } } };
  if (query.includes("search("))
    return {
      search: {
        nodes: [
          { title: "Fix crash on <empty> input", url: "u1", state: "MERGED", createdAt: "", mergedAt: "", repository: { nameWithOwner: "acme/lib", isPrivate: false, stargazerCount: 1234, viewerPermission: "READ", owner: { login: "acme" } } },
          { title: "Add feature", url: "u2", state: "OPEN", createdAt: "", mergedAt: null, repository: { nameWithOwner: "acme/app", isPrivate: false, stargazerCount: 0, viewerPermission: "READ", owner: { login: "acme" } } },
          { title: "Work repo", url: "u3", state: "MERGED", createdAt: "", mergedAt: "", repository: { nameWithOwner: "employer/svc", isPrivate: false, stargazerCount: 3, viewerPermission: "WRITE", owner: { login: "employer" } } },
          { title: "Rejected", url: "u4", state: "CLOSED", createdAt: "", mergedAt: null, repository: { nameWithOwner: "acme/lib", isPrivate: false, stargazerCount: 1234, viewerPermission: "READ", owner: { login: "acme" } } },
        ],
      },
    };
  throw new Error(`unexpected query: ${query.slice(0, 80)}`);
}

export function installFakeGitHub() {
  globalThis.fetch = mock(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    calls.push(url);
    if (failNext > 0) {
      failNext--;
      return Response.json({ message: "Server Error" }, { status: 502 });
    }
    if (url.includes("/search/commits")) return Response.json({ total_count: 4321 });
    const { query, variables } = JSON.parse(String(init?.body));
    return Response.json({ data: answer(query, variables) });
  }) as unknown as typeof fetch;
}
