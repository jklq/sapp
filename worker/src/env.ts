import type { Context } from "hono";

export interface Env {
  DB: D1Database;
  CATEGORIZE: Queue<CategorizeMessage>;
  JWT_SECRET_KEY: string;
  OPENROUTER_KEY: string;
  OPENROUTER_MODEL: string;
}

export interface CategorizeMessage {
  jobId: number;
}

export type AppEnv = { Bindings: Env; Variables: { userId: number } };
export type AppContext = Context<AppEnv>;
