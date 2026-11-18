import { Hono } from "hono";
import { z } from "zod";
import { firstName, partnerOf } from "./auth";
import type { AppEnv } from "./env";
import { httpError, json, parseId, readJson } from "./http";
import { nowIso, parseYmd } from "./time";

const str = z.string().nullish().transform((v) => v ?? "");
const num = z.number().nullish().transform((v) => v ?? 0);
const bool = z.boolean().nullish().transform((v) => v ?? false);

const paySchema = z.object({
  shared_status: str,
  amount: num,
  category: str,
  spending_date: z.string().nullish(),
  pre_settled: bool,
});
const categorizeSchema = z.object({
  amount: num,
  prompt: str,
  transaction_date: z.string().nullish(),
  pre_settled: bool,
});
const SHARING_STATUSES = ["Alone", "Shared", "Paid by Partner"];
const updateSpendingSchema = z.object({ description: str, category_name: str, sharing_status: str });

export const spendingRoutes = new Hono<AppEnv>()
  .get("/categories", async (c) => {
    const { results } = await c.env.DB.prepare("SELECT id, name FROM categories ORDER BY name ASC").all<{ id: number; name: string }>();
    return json(results);
  })
  .post("/pay", async (c) => {
    const userId = c.var.userId;
    const db = c.env.DB;
    const payload = await readJson(c.req.raw, paySchema);
    if (!payload) return httpError(400, "Bad Request: Invalid JSON");
    if (payload.amount <= 0) return httpError(400, "Amount must be positive");

    // An unparseable date falls back to now, like the Go handler.
    const date = payload.spending_date ? parseYmd(payload.spending_date) : null;
    const spendingDate = date ? date.toISOString() : nowIso();

    let sharedWith: number | null = null;
    if (payload.shared_status === "shared") {
      sharedWith = await partnerOf(db, userId);
      if (sharedWith === null) return httpError(400, "Cannot share: Partner not found or not configured for this user.");
    } else if (payload.shared_status !== "alone") {
      return httpError(400, "Invalid shared status.");
    }

    const category = await db.prepare("SELECT id FROM categories WHERE name = ? LIMIT 1").bind(payload.category).first<{ id: number }>();
    if (!category) return httpError(400, "Category not found.");

    await db.batch([
      db
        .prepare("INSERT INTO spendings (amount, description, category, made_by, spending_date) VALUES (?, 'Manual Entry', ?, ?, ?)")
        .bind(payload.amount, category.id, userId, spendingDate),
      db
        .prepare(
          "INSERT INTO user_spendings (spending_id, buyer, shared_with, shared_user_takes_all, settled_at) VALUES (last_insert_rowid(), ?, ?, 0, ?)",
        )
        .bind(userId, sharedWith, payload.pre_settled ? nowIso() : null),
    ]);
    return c.body(null, 201);
  })
  .post("/categorize", async (c) => {
    const userId = c.var.userId;
    const db = c.env.DB;
    const payload = await readJson(c.req.raw, categorizeSchema);
    if (!payload) return httpError(400, "Bad Request: Invalid JSON");
    // An unparseable date is ignored (the job gets the current time), like the Go handler.
    const date = payload.transaction_date ? parseYmd(payload.transaction_date) : null;
    if (!payload.prompt || payload.amount <= 0) return httpError(400, "Bad Request: Missing prompt or invalid amount");
    if ((await firstName(db, userId)) === null) return httpError(500, "Internal Server Error: User configuration issue");

    const partner = await partnerOf(db, userId);
    const partnerExists = partner !== null && (await firstName(db, partner)) !== null;
    const job = await db
      .prepare(
        `INSERT INTO ai_categorization_jobs (buyer, shared_with, prompt, total_amount, pre_settled, transaction_date, status)
         VALUES (?, ?, ?, ?, ?, ?, 'pending') RETURNING id`,
      )
      .bind(userId, partnerExists ? partner : null, payload.prompt, payload.amount, payload.pre_settled ? 1 : 0, (date ?? new Date()).toISOString())
      .first<{ id: number }>();
    if (!job) return httpError(500, "Internal Server Error");
    await c.env.CATEGORIZE.send({ jobId: job.id });
    return json({ job_id: job.id }, 202);
  })
  .put("/spendings/:spending_id", async (c) => {
    const userId = c.var.userId;
    const db = c.env.DB;
    const spendingId = parseId(c.req.param("spending_id"));
    if (spendingId === null) return httpError(400, "Invalid spending ID");
    const payload = await readJson(c.req.raw, updateSpendingSchema);
    if (!payload) return httpError(400, "Bad Request: Invalid JSON");
    if (!payload.category_name) return httpError(400, "Bad Request: Category name is required");
    if (!SHARING_STATUSES.includes(payload.sharing_status)) return httpError(400, "Bad Request: Invalid sharing status");

    const owner = await db.prepare("SELECT buyer FROM user_spendings WHERE spending_id = ?").bind(spendingId).first<{ buyer: number }>();
    if (!owner) return httpError(404, "Spending item not found");
    if (owner.buyer !== userId) return httpError(403, "Forbidden: You can only edit your own spending items");
    const category = await db.prepare("SELECT id FROM categories WHERE name = ?").bind(payload.category_name).first<{ id: number }>();
    if (!category) return httpError(400, "Category not found");

    let sharedWith: number | null = null;
    if (payload.sharing_status !== "Alone") {
      sharedWith = await partnerOf(db, userId);
      if (sharedWith === null) {
        return httpError(400, "Cannot set status to 'Shared' or 'Paid by Partner': No partner configured for your user.");
      }
    }
    await db.batch([
      db.prepare("UPDATE spendings SET description = ?, category = ? WHERE id = ?").bind(payload.description, category.id, spendingId),
      db
        .prepare("UPDATE user_spendings SET shared_with = ?, shared_user_takes_all = ? WHERE spending_id = ?")
        .bind(sharedWith, payload.sharing_status === "Paid by Partner" ? 1 : 0, spendingId),
    ]);
    return c.body(null, 200);
  })
  .delete("/jobs/:job_id", async (c) => {
    const db = c.env.DB;
    const jobId = parseId(c.req.param("job_id"));
    if (jobId === null) return httpError(400, "Invalid job ID");
    const job = await db.prepare("SELECT buyer FROM ai_categorization_jobs WHERE id = ?").bind(jobId).first<{ buyer: number }>();
    if (!job) return httpError(404, "Job not found");
    if (job.buyer !== c.var.userId) return httpError(403, "Forbidden: You can only delete your own jobs");

    const ofJob = "SELECT spending_id FROM ai_categorized_spendings WHERE job_id = ?";
    await db.batch([
      db.prepare(`DELETE FROM user_spendings WHERE spending_id IN (${ofJob})`).bind(jobId),
      db.prepare(`DELETE FROM spendings WHERE id IN (${ofJob})`).bind(jobId),
      db.prepare("DELETE FROM ai_categorization_jobs WHERE id = ?").bind(jobId),
    ]);
    return c.body(null, 204);
  });
