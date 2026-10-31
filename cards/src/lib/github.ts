// GitHub API access with a small in-memory cache. The gateway caches rendered
// cards per URL; this caches the data underneath, so different themes or
// layouts of the same card cost one API call.

import { env } from "./env.ts";

export class GitHubError extends Error {
  constructor(
    message: string,
    readonly status = 502,
  ) {
    super(message);
  }
}

const headers = () => ({
  Authorization: `bearer ${env.token}`,
  "Content-Type": "application/json",
  "User-Agent": "github-stats-cards",
});

export async function graphql<T>(query: string, variables: Record<string, unknown> = {}): Promise<T> {
  const res = await fetch("https://api.github.com/graphql", {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ query, variables }),
    signal: AbortSignal.timeout(20_000),
  });
  const body = (await res.json().catch(() => ({}))) as { data?: T; errors?: { type?: string; message: string }[]; message?: string };
  if (!res.ok || body.errors?.length || !body.data) {
    const error = body.errors?.[0];
    console.error("graphql failed", res.status, JSON.stringify(body.errors ?? body.message ?? body));
    if (error?.type === "NOT_FOUND") throw new GitHubError("Could not find that user", 404);
    if (error?.type === "RATE_LIMITED" || res.status === 403 || res.status === 429) throw new GitHubError("GitHub API rate limit reached");
    if (res.status === 401) throw new GitHubError("GitHub token is invalid or expired");
    throw new GitHubError("Could not load data from GitHub");
  }
  return body.data;
}

export async function rest<T>(path: string): Promise<T> {
  const res = await fetch(`https://api.github.com${path}`, {
    headers: { ...headers(), Accept: "application/vnd.github+json" },
    signal: AbortSignal.timeout(20_000),
  });
  if (!res.ok) {
    console.error("rest failed", path, res.status, await res.text().catch(() => ""));
    throw new GitHubError(res.status === 401 ? "GitHub token is invalid or expired" : "Could not load data from GitHub");
  }
  return (await res.json()) as T;
}

type Entry = { at: number; value?: unknown; pending?: Promise<unknown> };
const entries = new Map<string, Entry>();

export const DATA_TTL_MS = 15 * 60 * 1000;

/**
 * Memoise `load` under `key` for `ttl` ms. Concurrent callers share one
 * request, and if a refresh fails the previous value keeps being served.
 */
export function cached<T>(key: string, load: () => Promise<T>, ttl = DATA_TTL_MS): Promise<T> {
  const entry = entries.get(key) ?? { at: 0 };
  entries.set(key, entry);
  if ("value" in entry && Date.now() - entry.at < ttl) return Promise.resolve(entry.value as T);
  if (entry.pending) return entry.pending as Promise<T>;

  const pending = load().then(
    (value) => {
      entry.at = Date.now();
      entry.value = value;
      entry.pending = undefined;
      return value;
    },
    (error) => {
      entry.pending = undefined;
      if ("value" in entry) {
        console.error(`refresh of ${key} failed, serving stale data:`, error.message);
        return entry.value as T;
      }
      throw error;
    },
  );
  entry.pending = pending;
  return pending;
}

export const clearCache = () => entries.clear();
