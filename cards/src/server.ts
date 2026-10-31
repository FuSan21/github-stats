// Profile cards for one GitHub user, rendered with that user's own token:
//   /api            stats card              /graph  contribution graph
//   /api/top-langs  top languages           /oss    PRs to other people's projects
//   /streak         contribution streak     /views  profile view counter
// The nginx gateway in front caches responses and enforces the single user too.

import { handle } from "./app.ts";
import { flushViews, loadViews } from "./cards/views.ts";
import { env } from "./lib/env.ts";

if (!env.user || !env.token) {
  console.error("GITHUB_USER and GITHUB_TOKEN are required");
  process.exit(1);
}

await loadViews();
const flush = () => flushViews().catch((e) => console.error("views flush failed", e));
setInterval(flush, 5000).unref();

const server = Bun.serve({ port: env.port, fetch: handle });
console.log(`cards listening on ${server.port}`);

for (const sig of ["SIGTERM", "SIGINT"] as const) {
  process.on(sig, async () => {
    await server.stop();
    await flush();
    process.exit(0);
  });
}
