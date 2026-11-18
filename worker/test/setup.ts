import { applyD1Migrations, reset, type D1Migration } from "cloudflare:test";
import { env } from "cloudflare:workers";
import { afterEach, beforeEach } from "vitest";

declare global {
  namespace Cloudflare {
    interface Env {
      DB: D1Database;
      TEST_MIGRATIONS: D1Migration[];
    }
  }
}

// Every test starts from an empty, migrated database.
beforeEach(() => applyD1Migrations(env.DB, env.TEST_MIGRATIONS));
afterEach(() => reset());
