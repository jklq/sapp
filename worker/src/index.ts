import { Hono } from "hono";
import { authRoutes, requireAuth, verifyRoute } from "./auth";
import { openRouter, processJob } from "./categorize";
import { depositRoutes } from "./deposits";
import type { AppEnv, CategorizeMessage, Env } from "./env";
import { exportRoute } from "./export";
import { historyRoute } from "./history";
import { httpError } from "./http";
import { spendingRoutes } from "./spendings";
import { statsRoutes } from "./stats";
import { transferRoutes } from "./transfer";

const protectedRoutes = new Hono<AppEnv>()
  .use(requireAuth)
  .route("/verify", verifyRoute)
  .route("/history", historyRoute)
  .route("/transfer", transferRoutes)
  .route("/deposits", depositRoutes)
  .route("/stats", statsRoutes)
  .route("/export", exportRoute)
  .route("/", spendingRoutes);

// Static assets (the Vite SPA) are served before the Worker runs; only /v1/* reaches it.
const app = new Hono<AppEnv>()
  .route("/v1", authRoutes)
  .route("/v1", protectedRoutes)
  .notFound(() => httpError(404, "404 page not found"))
  .onError((err) => {
    console.error(JSON.stringify({ event: "unhandled_error", error: String(err), stack: err.stack }));
    return httpError(500, "Internal server error");
  });

export default {
  fetch: app.fetch,
  async queue(batch, env) {
    for (const message of batch.messages) {
      await processJob(env, message.body.jobId, openRouter(env.OPENROUTER_KEY, env.OPENROUTER_MODEL));
      message.ack();
    }
  },
} satisfies ExportedHandler<Env, CategorizeMessage>;
