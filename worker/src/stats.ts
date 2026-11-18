import { Hono } from "hono";
import type { AppEnv } from "./env";
import { depositOccurrences, type DepositRow } from "./history";
import { httpError, json } from "./http";
import { parseYmd } from "./time";

const round2 = (n: number) => Math.round(n * 100) / 100;

/** Parses ?startDate=&endDate= (YYYY-MM-DD); the range ends at the end of endDate. */
function dateRange(url: URL): { start: Date; end: Date } | Response {
  const parsed: Date[] = [];
  for (const name of ["startDate", "endDate"]) {
    const value = url.searchParams.get(name);
    if (!value) return httpError(400, `missing query parameter: ${name}`);
    const date = parseYmd(value);
    if (!date) return httpError(400, `invalid date format for ${name} (use YYYY-MM-DD)`);
    parsed.push(date);
  }
  const [start, endDay] = parsed;
  if (endDay < start) return httpError(400, "Bad Request: endDate cannot be before startDate");
  return { start, end: new Date(endDay.getTime() + 24 * 60 * 60 * 1000 - 1) };
}

export const statsRoutes = new Hono<AppEnv>()
  .get("/spending", async (c) => {
    const range = dateRange(new URL(c.req.url));
    if (range instanceof Response) return range;
    const { results } = await c.env.DB.prepare(
      `SELECT c.name AS category_name, SUM(
         CASE
           WHEN us.buyer = ?1 AND us.shared_with IS NULL THEN s.amount
           WHEN us.buyer = ?1 AND us.shared_with IS NOT NULL AND us.shared_user_takes_all = 0 THEN s.amount / 2.0
           WHEN us.buyer = ?1 AND us.shared_with IS NOT NULL AND us.shared_user_takes_all = 1 THEN 0.0
           WHEN us.buyer != ?1 AND us.shared_with = ?1 AND us.shared_user_takes_all = 0 THEN s.amount / 2.0
           WHEN us.buyer != ?1 AND us.shared_with = ?1 AND us.shared_user_takes_all = 1 THEN s.amount
           ELSE 0.0
         END) AS total_amount
       FROM spendings s
       JOIN categories c ON s.category = c.id
       JOIN user_spendings us ON s.id = us.spending_id
       WHERE s.spending_date >= ?2 AND s.spending_date <= ?3 AND (us.buyer = ?1 OR us.shared_with = ?1)
       GROUP BY c.name
       HAVING total_amount > 0.001
       ORDER BY total_amount DESC`,
    )
      .bind(c.var.userId, range.start.toISOString(), range.end.toISOString())
      .all<{ category_name: string; total_amount: number }>();
    return json(results.map((r) => ({ category_name: r.category_name, total_amount: round2(r.total_amount) })));
  })
  .get("/deposits", async (c) => {
    const range = dateRange(new URL(c.req.url));
    if (range instanceof Response) return range;
    const { results } = await c.env.DB.prepare(
      "SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at FROM deposits WHERE user_id = ?",
    )
      .bind(c.var.userId)
      .all<DepositRow>();
    let total = 0;
    let count = 0;
    for (const d of results) {
      const n = depositOccurrences(d, range.start, range.end).length;
      total += n * d.amount;
      count += n;
    }
    return json({ total_amount: round2(total), count });
  });
