import { Hono } from "hono";
import { z } from "zod";
import type { AppEnv } from "./env";
import type { DepositRow } from "./history";
import { httpError, json, parseId, readJson } from "./http";
import { nowIso, parseYmd, RECURRENCE_PERIODS } from "./time";

const COLUMNS = "id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at";

export const depositJson = (d: DepositRow) => ({
  id: d.id,
  user_id: d.user_id,
  amount: d.amount,
  description: d.description ?? "",
  deposit_date: d.deposit_date,
  is_recurring: Boolean(d.is_recurring),
  recurrence_period: d.recurrence_period,
  end_date: d.end_date,
  created_at: d.created_at,
});

const addSchema = z.object({
  amount: z.number().nullish().transform((v) => v ?? 0),
  description: z.string().nullish().transform((v) => v ?? ""),
  deposit_date: z.string().nullish().transform((v) => v ?? ""),
  is_recurring: z.boolean().nullish().transform((v) => v ?? false),
  recurrence_period: z.string().nullish(),
});

// JSON null means "not provided", like a nil pointer in the Go payload; "" clears dates/periods.
const updateSchema = z.object({
  amount: z.number().nullish(),
  description: z.string().nullish(),
  deposit_date: z.string().nullish(),
  is_recurring: z.boolean().nullish(),
  recurrence_period: z.string().nullish(),
  end_date: z.string().nullish(),
});

async function ownDeposit(db: D1Database, id: number): Promise<DepositRow | null> {
  return db.prepare(`SELECT ${COLUMNS} FROM deposits WHERE id = ?`).bind(id).first<DepositRow>();
}

export const depositRoutes = new Hono<AppEnv>()
  .post("/", async (c) => {
    const p = await readJson(c.req.raw, addSchema);
    if (!p) return httpError(400, "Bad Request: Invalid JSON");
    if (p.amount <= 0) return httpError(400, "Bad Request: Amount must be positive");
    if (!p.description) return httpError(400, "Bad Request: Description is required");
    const date = parseYmd(p.deposit_date);
    if (!date) return httpError(400, "Bad Request: invalid date format");
    let period: string | null = null;
    if (p.is_recurring) {
      if (!p.recurrence_period) return httpError(400, "Bad Request: Recurrence period is required for recurring deposits");
      if (!RECURRENCE_PERIODS.includes(p.recurrence_period)) return httpError(400, "Bad Request: Unsupported recurrence period");
      period = p.recurrence_period;
    }
    const row = await c.env.DB.prepare(
      `INSERT INTO deposits (user_id, amount, description, deposit_date, is_recurring, recurrence_period, created_at)
       VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
    )
      .bind(c.var.userId, p.amount, p.description, date.toISOString(), p.is_recurring ? 1 : 0, period, nowIso())
      .first<{ id: number }>();
    return json({ message: "Deposit added successfully", deposit_id: row?.id }, 201);
  })
  .get("/", async (c) => {
    const { results } = await c.env.DB.prepare(
      `SELECT ${COLUMNS} FROM deposits WHERE user_id = ? ORDER BY deposit_date DESC, created_at DESC`,
    )
      .bind(c.var.userId)
      .all<DepositRow>();
    return json(results.map(depositJson));
  })
  .get("/:deposit_id", async (c) => {
    const id = parseId(c.req.param("deposit_id"));
    if (id === null) return httpError(400, "Invalid deposit ID");
    const d = await ownDeposit(c.env.DB, id);
    if (!d || d.user_id !== c.var.userId) return httpError(404, "Deposit not found");
    return json(depositJson(d));
  })
  .put("/:deposit_id", async (c) => {
    const id = parseId(c.req.param("deposit_id"));
    if (id === null) return httpError(400, "Invalid deposit ID");
    const p = await readJson(c.req.raw, updateSchema);
    if (!p) return httpError(400, "Bad Request: Invalid JSON");
    const current = await ownDeposit(c.env.DB, id);
    if (!current) return httpError(404, "Deposit not found");
    if (current.user_id !== c.var.userId) return httpError(403, "Forbidden: You can only update your own deposits");

    const changes: Partial<Record<keyof DepositRow, string | number | null>> = {};
    const next = { ...current };
    const set = <K extends keyof DepositRow>(key: K, value: DepositRow[K]) => {
      changes[key] = value;
      next[key] = value;
    };

    if (p.amount != null) {
      if (p.amount <= 0) return httpError(400, "Bad Request: Amount must be positive");
      set("amount", p.amount);
    }
    if (p.description != null) {
      if (p.description === "") return httpError(400, "Bad Request: Description cannot be empty");
      set("description", p.description);
    }
    if (p.deposit_date != null) {
      const date = parseYmd(p.deposit_date);
      if (!date) return httpError(400, "Bad Request: invalid date format");
      set("deposit_date", date.toISOString());
    }
    if (p.is_recurring != null) set("is_recurring", p.is_recurring ? 1 : 0);
    if (p.recurrence_period != null) {
      if (p.recurrence_period !== "" && !RECURRENCE_PERIODS.includes(p.recurrence_period)) {
        return httpError(400, "Bad Request: Unsupported recurrence period");
      }
      set("recurrence_period", p.recurrence_period || null);
    }
    if (next.is_recurring && !next.recurrence_period) {
      return httpError(400, "Bad Request: Recurrence period is required for recurring deposits");
    }
    if (!next.is_recurring && next.recurrence_period !== null) set("recurrence_period", null);
    if (p.end_date != null) {
      const date = p.end_date === "" ? null : parseYmd(p.end_date);
      if (p.end_date !== "" && !date) return httpError(400, "Bad Request: invalid date format");
      set("end_date", date ? date.toISOString() : null);
    }
    if (!next.is_recurring && next.end_date !== null) set("end_date", null);

    const keys = Object.keys(changes) as (keyof DepositRow)[];
    if (keys.length === 0) return json({ message: "No changes detected", deposit: depositJson(next) });
    await c.env.DB.prepare(`UPDATE deposits SET ${keys.map((k) => `${k} = ?`).join(", ")} WHERE id = ?`)
      .bind(...keys.map((k) => changes[k] ?? null), id)
      .run();
    return json({ message: "Deposit updated successfully", deposit: depositJson(next) });
  })
  .delete("/:deposit_id", async (c) => {
    const id = parseId(c.req.param("deposit_id"));
    if (id === null) return httpError(400, "Invalid deposit ID");
    const d = await ownDeposit(c.env.DB, id);
    if (!d) return httpError(404, "Deposit not found");
    if (d.user_id !== c.var.userId) return httpError(403, "Forbidden: You can only delete your own deposits");
    await c.env.DB.prepare("DELETE FROM deposits WHERE id = ?").bind(id).run();
    return json({ message: "Deposit deleted successfully" });
  });
