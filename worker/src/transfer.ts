import { Hono } from "hono";
import { firstName, partnerOf } from "./auth";
import type { AppEnv } from "./env";
import { httpError, json } from "./http";
import { nowIso } from "./time";

// Unqualified columns are unambiguous here: only user_spendings has them.
const UNSETTLED_BETWEEN = "settled_at IS NULL AND ((buyer = ?1 AND shared_with = ?2) OR (buyer = ?2 AND shared_with = ?1))";

export const transferRoutes = new Hono<AppEnv>()
  .get("/status", async (c) => {
    const db = c.env.DB;
    const userId = c.var.userId;
    const partnerId = await partnerOf(db, userId);
    if (partnerId === null) return httpError(400, "Partner not found or not configured for this user.");
    const userName = await firstName(db, userId);
    if (userName === null) return httpError(500, "Internal server error");
    const partnerName = (await firstName(db, partnerId)) ?? "Partner";

    const { results } = await db
      .prepare(
        `SELECT us.buyer, us.shared_user_takes_all, s.amount FROM user_spendings us JOIN spendings s ON us.spending_id = s.id
         WHERE ${UNSETTLED_BETWEEN}`,
      )
      .bind(userId, partnerId)
      .all<{ buyer: number; shared_user_takes_all: number; amount: number }>();

    // Positive: the partner owes the user.
    let balance = 0;
    for (const r of results) {
      const share = r.shared_user_takes_all ? r.amount : r.amount / 2;
      balance += r.buyer === userId ? share : -share;
    }
    const tolerance = 0.001;
    return json({
      partner_name: partnerName,
      amount_owed: Math.abs(balance) > tolerance ? Math.abs(balance) : 0,
      owed_by: balance > tolerance ? partnerName : balance < -tolerance ? userName : null,
      owed_to: balance > tolerance ? userName : balance < -tolerance ? partnerName : null,
    });
  })
  .post("/record", async (c) => {
    const db = c.env.DB;
    const userId = c.var.userId;
    const partnerId = await partnerOf(db, userId);
    if (partnerId === null) return httpError(400, "Partner not found or not configured for this user.");
    const now = nowIso();
    await db.batch([
      db.prepare(`UPDATE user_spendings SET settled_at = ?3 WHERE ${UNSETTLED_BETWEEN}`).bind(userId, partnerId, now),
      db
        .prepare("INSERT INTO transfers (settled_by_user_id, settled_with_user_id, settlement_time) VALUES (?, ?, ?)")
        .bind(userId, partnerId, now),
    ]);
    return c.body(null, 200);
  });
