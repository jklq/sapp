import { env } from "cloudflare:workers";
import { describe, expect, it } from "vitest";
import { processJob } from "../src/categorize";
import type { Env } from "../src/env";
import { api, db, seedPartners } from "./helpers";

async function createJob(total: number): Promise<number> {
  const row = await db()
    .prepare(
      "INSERT INTO ai_categorization_jobs (buyer, shared_with, prompt, total_amount, transaction_date, status) VALUES (1, 2, 'middag og brød', ?, '2026-03-01T00:00:00.000Z', 'pending') RETURNING id",
    )
    .bind(total)
    .first<{ id: number }>();
  return row!.id;
}

const reply = (spendings: object[], ambiguity_flag = "") => JSON.stringify({ ambiguity_flag, spendings });

describe("AI categorisation", () => {
  it("retries inconsistent answers, then stores the split and shows it in both histories", async () => {
    const { alice, bob } = await seedPartners();
    const jobId = await createJob(100);
    const answers = [
      reply([{ apportion_mode: "shared", category: "Groceries", amount: 90, description: "Middag" }]),
      reply([
        { apportion_mode: "shared", category: "Groceries", amount: 60, description: "Middag" },
        { apportion_mode: "other", category: "Groceries", amount: 40, description: "" },
      ], "uklart"),
    ];
    const prompts: string[] = [];
    await processJob(env as unknown as Env, jobId, async (prompt) => {
      prompts.push(prompt);
      return answers.shift()!;
    });

    expect(prompts).toHaveLength(2);
    expect(prompts[0]).toContain("heter Alice. Kjøpet kan potensielt involvere partneren Bob.");
    expect(prompts[0]).toContain('- Groceries - "dersom bruker');
    expect(prompts[0]).toContain("- Coffee\n");

    const { history } = (await (await api("GET", "/v1/history", alice)).json()) as { history: any[] };
    expect(history).toHaveLength(1);
    expect(history[0]).toMatchObject({
      type: "spending_group",
      job_id: jobId,
      total_amount: 100,
      buyer_name: "Alice",
      is_ambiguity_flagged: true,
      ambiguity_flag_reason: "uklart",
    });
    expect(history[0].spendings.map((s: any) => [s.description, s.sharing_status])).toEqual([
      ["Middag", "Shared with Bob"],
      ["AI Categorized", "Paid by Bob"],
    ]);

    const bobView = (await (await api("GET", "/v1/history", bob)).json()) as { history: any[] };
    expect(bobView.history[0].spendings.map((s: any) => s.sharing_status)).toEqual(["Shared with You (Bob)", "Paid by You (Bob)"]);
    expect(await (await api("GET", "/v1/transfer/status", bob)).json()).toMatchObject({ amount_owed: 70, owed_by: "Bob" });
  });

  it("fails the job without partial writes when a category is unknown", async () => {
    await seedPartners();
    const jobId = await createJob(50);
    await processJob(env as unknown as Env, jobId, async () =>
      reply([
        { apportion_mode: "alone", category: "Coffee", amount: 25, description: "" },
        { apportion_mode: "alone", category: "Snacks", amount: 25, description: "" },
      ]),
    );
    expect(await db().prepare("SELECT status, is_finished, error_message FROM ai_categorization_jobs WHERE id = ?").bind(jobId).first()).toEqual({
      status: "failed",
      is_finished: 1,
      error_message: "category 'Snacks' not found or invalid",
    });
    expect(await db().prepare("SELECT COUNT(*) AS n FROM spendings").first("n")).toBe(0);
  });

  it("deletes a job with its spendings, only for the buyer", async () => {
    const { alice, bob } = await seedPartners();
    const jobId = await createJob(10);
    await processJob(env as unknown as Env, jobId, async () => reply([{ apportion_mode: "alone", category: "Coffee", amount: 10, description: "" }]));
    expect((await api("DELETE", `/v1/jobs/${jobId}`, bob)).status).toBe(403);
    expect((await api("DELETE", `/v1/jobs/${jobId}`, alice)).status).toBe(204);
    expect(await db().prepare("SELECT (SELECT COUNT(*) FROM spendings) + (SELECT COUNT(*) FROM user_spendings) AS n").first("n")).toBe(0);
  });
});
