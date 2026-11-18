import { Hono } from "hono";
import { firstName, partnerOf } from "./auth";
import type { AppEnv } from "./env";
import { json } from "./http";
import { nextOccurrence } from "./time";

export interface DepositRow {
  id: number;
  user_id: number;
  amount: number;
  description: string | null;
  deposit_date: string;
  is_recurring: number;
  recurrence_period: string | null;
  end_date: string | null;
  created_at: string;
}

/** Dates of a deposit template between `from` and `to` (inclusive), honouring its end date. */
export function depositOccurrences(d: DepositRow, from: Date | null, to: Date): Date[] {
  const end = d.end_date && new Date(d.end_date) < to ? new Date(d.end_date) : to;
  const dates: Date[] = [];
  let current: Date | null = new Date(d.deposit_date);
  while (current && current <= end) {
    if (!from || current >= from) dates.push(current);
    if (!d.is_recurring || !d.recurrence_period) break;
    const next = nextOccurrence(current, d.recurrence_period);
    current = next && next > current ? next : null;
  }
  return dates;
}

export function sharingStatus(
  userId: number,
  buyerId: number,
  shared: boolean,
  takesAll: boolean,
  partnerName: string | null,
  userName: string,
): string {
  const partner = partnerName || "Partner";
  if (buyerId === userId) {
    if (!shared) return "Alone";
    return takesAll ? `Paid by ${partner}` : `Shared with ${partner}`;
  }
  if (!shared) return `${partner} Alone`;
  return takesAll ? `Paid by You (${userName})` : `Shared with You (${userName})`;
}

interface JobRow {
  id: number;
  prompt: string;
  total_amount: number;
  date: string;
  is_ambiguity_flagged: number;
  ambiguity_flag_reason: string | null;
  buyer_name: string | null;
}

interface SpendingRow {
  job_id: number;
  id: number;
  amount: number;
  description: string | null;
  category_name: string;
  buyer_name: string | null;
  partner_name: string | null;
  shared_user_takes_all: number;
  shared_with: number | null;
  buyer: number;
}

type HistoryItem = { type: "spending_group" | "deposit"; date: string } & Record<string, unknown>;

export async function history(db: D1Database, userId: number, now: Date): Promise<HistoryItem[]> {
  const partnerId = (await partnerOf(db, userId)) ?? -1;
  const userName = (await firstName(db, userId)) || "You";

  // AI jobs bought by the user, or by the partner with a spending shared with the user.
  const jobFilter = `
    FROM ai_categorization_jobs j
    LEFT JOIN ai_categorized_spendings acs ON j.id = acs.job_id
    LEFT JOIN user_spendings us ON acs.spending_id = us.spending_id
    WHERE j.buyer = ?1 OR (j.buyer = ?2 AND us.shared_with = ?1)`;
  const [jobs, spendings, deposits] = await db.batch<JobRow | SpendingRow | DepositRow>([
    db
      .prepare(
        `SELECT DISTINCT j.id, j.prompt, j.total_amount, j.transaction_date AS date, j.is_ambiguity_flagged,
           j.ambiguity_flag_reason, u.first_name AS buyer_name
         FROM ai_categorization_jobs j JOIN users u ON j.buyer = u.id
         WHERE j.id IN (SELECT j.id ${jobFilter})
         ORDER BY j.transaction_date DESC, j.created_at DESC`,
      )
      .bind(userId, partnerId),
    db
      .prepare(
        `SELECT acs.job_id, s.id, s.amount, s.description, c.name AS category_name, ub.first_name AS buyer_name,
           up.first_name AS partner_name, us.shared_user_takes_all, us.shared_with, us.buyer
         FROM spendings s
         JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
         JOIN user_spendings us ON s.id = us.spending_id
         JOIN categories c ON s.category = c.id
         JOIN users ub ON us.buyer = ub.id
         LEFT JOIN users up ON us.shared_with = up.id
         WHERE acs.job_id IN (SELECT j.id ${jobFilter})
         ORDER BY s.id ASC`,
      )
      .bind(userId, partnerId),
    db
      .prepare(
        `SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at
         FROM deposits WHERE user_id = ? ORDER BY deposit_date DESC, created_at DESC`,
      )
      .bind(userId),
  ]);

  const spendingsByJob = new Map<number, SpendingRow[]>();
  for (const s of spendings.results as SpendingRow[]) {
    spendingsByJob.set(s.job_id, [...(spendingsByJob.get(s.job_id) ?? []), s]);
  }

  const items: HistoryItem[] = (jobs.results as JobRow[]).map((j) => {
    const items = (spendingsByJob.get(j.id) ?? []).map((s) => ({
      id: s.id,
      amount: s.amount,
      description: s.description ?? "",
      category_name: s.category_name,
      spending_date: "0001-01-01T00:00:00Z", // Never populated by the Go backend either.
      buyer_name: s.buyer_name ?? "",
      partner_name: s.partner_name,
      shared_user_takes_all: Boolean(s.shared_user_takes_all),
      sharing_status: sharingStatus(userId, s.buyer, s.shared_with !== null, Boolean(s.shared_user_takes_all), s.partner_name, userName),
    }));
    return {
      type: "spending_group",
      date: j.date,
      job_id: j.id,
      prompt: j.prompt,
      total_amount: j.total_amount,
      buyer_name: j.buyer_name ?? "",
      is_ambiguity_flagged: Boolean(j.is_ambiguity_flagged),
      ...(j.ambiguity_flag_reason !== null && { ambiguity_flag_reason: j.ambiguity_flag_reason }),
      ...(items.length > 0 && { spendings: items }),
    };
  });

  for (const d of deposits.results as DepositRow[]) {
    // One-off deposits are always listed; recurring ones up to today.
    const dates = d.is_recurring && d.recurrence_period ? depositOccurrences(d, null, now) : [new Date(d.deposit_date)];
    for (const date of dates) {
      items.push({
        type: "deposit",
        date: date.toISOString(),
        id: d.id,
        amount: d.amount,
        description: d.description ?? "",
        is_recurring: Boolean(d.is_recurring),
        ...(d.recurrence_period !== null && { recurrence_period: d.recurrence_period }),
        created_at: d.created_at,
      });
    }
  }

  return items.sort((a, b) => Date.parse(b.date) - Date.parse(a.date));
}

export const historyRoute = new Hono<AppEnv>().get("/", async (c) =>
  json({ history: await history(c.env.DB, c.var.userId, new Date()) }),
);
