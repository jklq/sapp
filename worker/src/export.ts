import { Hono } from "hono";
import { partnerOf } from "./auth";
import type { AppEnv } from "./env";
import { httpError } from "./http";

const apportionMode = (sharedWith: number | null, takesAll: number) =>
  sharedWith === null ? "Alone" : takesAll ? "PaidByPartner" : "Shared";

export const exportRoute = new Hono<AppEnv>().get("/all", async (c) => {
  const db = c.env.DB;
  const userId = c.var.userId;
  const partnerId = (await partnerOf(db, userId)) ?? 0;
  const userQuery = "SELECT username, first_name FROM users WHERE id = ?";

  const [user, partner, categories, jobs, jobSpendings, manual, deposits, transfers] = await db.batch<Record<string, never>>([
    db.prepare(userQuery).bind(userId),
    db.prepare(userQuery).bind(partnerId),
    db.prepare("SELECT name, ai_notes FROM categories ORDER BY name ASC"),
    db
      .prepare(
        `SELECT j.id, j.prompt, j.total_amount, j.transaction_date, j.pre_settled, u.username AS buyer_username,
           j.is_ambiguity_flagged, j.ambiguity_flag_reason
         FROM ai_categorization_jobs j JOIN users u ON j.buyer = u.id
         WHERE j.buyer = ?1 OR j.buyer = ?2
         ORDER BY j.transaction_date DESC, j.created_at DESC`,
      )
      .bind(userId, partnerId),
    db
      .prepare(
        `SELECT acs.job_id, c.name AS category_name, s.amount, s.description, us.shared_with, us.shared_user_takes_all
         FROM spendings s
         JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
         JOIN user_spendings us ON s.id = us.spending_id
         JOIN categories c ON s.category = c.id
         JOIN ai_categorization_jobs j ON j.id = acs.job_id
         WHERE j.buyer = ?1 OR j.buyer = ?2
         ORDER BY s.id ASC`,
      )
      .bind(userId, partnerId),
    db
      .prepare(
        `SELECT s.amount, s.description, c.name AS category_name, s.spending_date, u.username AS buyer_username,
           us.shared_with, us.shared_user_takes_all, us.settled_at
         FROM spendings s
         JOIN user_spendings us ON s.id = us.spending_id
         JOIN categories c ON s.category = c.id
         JOIN users u ON us.buyer = u.id
         LEFT JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
         WHERE acs.job_id IS NULL AND (us.buyer = ?1 OR us.buyer = ?2)
         ORDER BY s.spending_date DESC, s.created_at DESC`,
      )
      .bind(userId, partnerId),
    db
      .prepare(
        `SELECT d.description, d.amount, d.deposit_date, d.is_recurring, d.recurrence_period, d.end_date, u.username AS owner_username
         FROM deposits d JOIN users u ON d.user_id = u.id
         WHERE d.user_id = ?1 OR d.user_id = ?2
         ORDER BY d.deposit_date DESC, d.created_at DESC`,
      )
      .bind(userId, partnerId),
    db
      .prepare(
        `SELECT t.settlement_time, u.username AS settled_by_username
         FROM transfers t JOIN users u ON t.settled_by_user_id = u.id
         WHERE (t.settled_by_user_id = ?1 AND t.settled_with_user_id = ?2) OR (t.settled_by_user_id = ?2 AND t.settled_with_user_id = ?1)
         ORDER BY t.settlement_time DESC`,
      )
      .bind(userId, partnerId),
  ]);
  // Untyped rows from the batch; the column lists above define their shape.
  type Row = Record<string, any>;
  const rows = (r: D1Result<Record<string, never>>) => r.results as Row[];
  const userJson = (r: Row | undefined) => ({ username: r?.username ?? "", first_name: r?.first_name ?? "" });

  if (!user.results[0]) return httpError(404, "Data not found during fetching user details");

  const exportedAt = new Date();
  const body = {
    exported_at: exportedAt.toISOString(),
    user: userJson(user.results[0]),
    partner: userJson(partnerId ? rows(partner)[0] : undefined),
    categories: rows(categories).map((r) => ({ name: r.name, ...(r.ai_notes !== null && { ai_notes: r.ai_notes }) })),
    ai_jobs: rows(jobs).map((j) => ({
      prompt: j.prompt,
      total_amount: j.total_amount,
      transaction_date: j.transaction_date,
      pre_settled: Boolean(j.pre_settled),
      buyer_username: j.buyer_username,
      is_ambiguous: Boolean(j.is_ambiguity_flagged),
      ...(j.ambiguity_flag_reason !== null && { ambiguity_reason: j.ambiguity_flag_reason }),
      spendings: rows(jobSpendings)
        .filter((s) => s.job_id === j.id)
        .map((s) => ({
          category_name: s.category_name,
          amount: s.amount,
          description: s.description ?? "",
          apportion_mode: apportionMode(s.shared_with, s.shared_user_takes_all),
        })),
    })),
    manual_spendings: rows(manual).map((s) => ({
      amount: s.amount,
      description: s.description ?? "",
      category_name: s.category_name,
      spending_date: s.spending_date,
      buyer_username: s.buyer_username,
      shared_status: apportionMode(s.shared_with, s.shared_user_takes_all),
      ...(s.settled_at !== null && { settled_at: s.settled_at }),
    })),
    deposits: rows(deposits).map((d) => ({
      description: d.description ?? "",
      amount: d.amount,
      deposit_date: d.deposit_date,
      is_recurring: Boolean(d.is_recurring),
      ...(d.recurrence_period !== null && { recurrence_period: d.recurrence_period }),
      ...(d.end_date !== null && { end_date: d.end_date }),
      owner_username: d.owner_username,
    })),
    transfers: rows(transfers).map((t) => ({ settlement_time: t.settlement_time, settled_by_username: t.settled_by_username })),
  };

  const stamp = exportedAt.toISOString().replace(/[-:]/g, "").replace("T", "_").slice(0, 15);
  return new Response(JSON.stringify(body, null, 2), {
    headers: { "Content-Type": "application/json", "Content-Disposition": `attachment; filename=sapp_export_${stamp}.json` },
  });
});
