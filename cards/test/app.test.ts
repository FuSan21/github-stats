import { beforeAll, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { calls, installFakeGitHub, setFailNext, USER } from "./fake-github.ts";
import { assertWellFormed } from "./xml.ts";

process.env.GITHUB_USER = USER;
process.env.GITHUB_TOKEN = "test-token";
process.env.VIEWS_FILE = join(await mkdtemp(join(tmpdir(), "cards-")), "views.json");
process.env.INITIAL_VIEWS = "1718";

const { handle } = await import("../src/app.ts");
const { clearCache } = await import("../src/lib/github.ts");
const { flushViews, loadViews } = await import("../src/cards/views.ts");

const get = (path: string) => handle(new Request(`http://cards${path}`));

beforeAll(async () => {
  installFakeGitHub();
  await loadViews();
});
beforeEach(() => {
  clearCache();
  calls.length = 0;
});

describe("routing", () => {
  test.each([
    ["/api?username=FuSan21&theme=gruvbox&show_icons=true", "GitHub Stats"],
    ["/api/top-langs?username=FuSan21&layout=compact&theme=gruvbox", "Most Used Languages"],
    ["/streak?user=FuSan21&theme=gruvbox&hide_border=true", "Current Streak"],
    ["/graph?username=FuSan21&theme=gruvbox", "Contribution Graph"],
    ["/oss?username=FuSan21&theme=gruvbox", "Open Source Contributions"],
    ["/views?username=FuSan21", "Profile views"],
  ])("%s", async (path, text) => {
    const res = await get(path);
    expect(res.status).toBe(200);
    expect(res.headers.get("Content-Type")).toContain("image/svg+xml");
    const body = await res.text();
    assertWellFormed(body);
    expect(body).toContain(text);
  });

  test("usernames are case-insensitive; other users and paths are refused", async () => {
    expect((await get("/api?username=fusan21")).status).toBe(200);
    expect((await get("/api?username=someone")).status).toBe(403);
    expect((await get("/api")).status).toBe(403);
    // /streak takes `user`, not `username`.
    expect((await get("/streak?username=FuSan21")).status).toBe(403);
    expect((await get("/api/pin?username=FuSan21")).status).toBe(404);
    expect(await (await get("/healthz")).text()).toBe("ok\n");
  });
});

describe("data", () => {
  test("stats paginate starred repos and sum their stars", async () => {
    const body = await (await get("/api?username=FuSan21&number_format=long")).text();
    expect(body).toContain(">1535<");
    expect(calls).toHaveLength(2);
  });

  test("include_all_commits uses commit search", async () => {
    const body = await (await get("/api?username=FuSan21&include_all_commits=true&number_format=long")).text();
    expect(body).toContain(">4321<");
    expect(body).toContain("Total Commits:");
  });

  test("top languages read every page", async () => {
    const body = await (await get("/api/top-langs?username=FuSan21&layout=compact&langs_count=10")).text();
    expect(body).toContain("Dockerfile");
    expect(body).toContain("TypeScript 52.00%"); // 6500 of 12500 bytes
  });

  test("oss drops repos with push access and closed PRs", async () => {
    const body = await (await get("/oss?username=FuSan21")).text();
    expect(body).toContain("acme/lib");
    expect(body).toContain("Fix crash on &lt;empty&gt; input");
    expect(body).not.toContain("employer/svc");
    expect(body).not.toContain("Rejected");
    expect(body).toContain("1 merged PR across 1 project");
  });

  test("streak fetches each year since the account was created", async () => {
    const res = await get("/streak?user=FuSan21");
    expect(res.status).toBe(200);
    // Current year first (to learn createdAt), then 2025.
    expect(calls).toHaveLength(2);
  });

  test("themes and layouts of one card share the cached data", async () => {
    await get("/api/top-langs?username=FuSan21&layout=compact");
    await get("/api/top-langs?username=FuSan21&layout=donut&theme=dark");
    expect(calls).toHaveLength(2); // two pages, fetched once
  });
});

describe("failures", () => {
  test("a GitHub outage is a 502 error card, not a cached 200", async () => {
    setFailNext(1);
    const res = await get("/api?username=FuSan21");
    expect(res.status).toBe(502);
    expect(res.headers.get("Cache-Control")).toBe("no-store");
    expect(await res.text()).toContain("Could not load data from GitHub");
  });

  test("streak errors use the streak card's own error style", async () => {
    setFailNext(1);
    const res = await get("/streak?user=FuSan21&theme=gruvbox");
    expect(res.status).toBe(502);
    const body = await res.text();
    assertWellFormed(body);
    expect(body).toContain('fill="#282828"');
  });

  test("an invalid timezone is a 400", async () => {
    expect((await get("/streak?user=FuSan21&timezone=Not/AZone")).status).toBe(400);
  });
});

describe("views", () => {
  test("counts every hit, is never cached, and persists", async () => {
    const first = await get("/views?username=FuSan21");
    expect(first.headers.get("Cache-Control")).toContain("no-store");
    const n = Number((await first.text()).match(/<title>Profile views: ([\d,]+)<\/title>/)![1]!.replace(/,/g, ""));
    expect(n).toBeGreaterThan(1718);
    const second = await (await get("/views?username=FuSan21&label=Visitors&color=ff69b4")).text();
    expect(second).toContain((n + 1).toLocaleString("en-US"));
    expect(second).toContain("Visitors");
    expect(second).toContain("#ff69b4");

    await flushViews();
    expect(await Bun.file(process.env.VIEWS_FILE!).json()).toEqual({ views: n + 1 });
  });
});

describe("data cache", () => {
  test("serves the last good value when a refresh fails", async () => {
    const { cached } = await import("../src/lib/github.ts");
    let n = 0;
    const load = async () => {
      if (++n === 2) throw new Error("boom");
      return n;
    };
    expect(await cached("k", load, 0)).toBe(1);
    expect(await cached("k", load, 0)).toBe(1); // refresh failed, stale value
    expect(await cached("k", load, 0)).toBe(3);
  });

  test("concurrent callers share one request", async () => {
    const { cached } = await import("../src/lib/github.ts");
    let n = 0;
    const load = async () => ++n;
    const all = await Promise.all([cached("c", load), cached("c", load), cached("c", load)]);
    expect(all).toEqual([1, 1, 1]);
  });
});
