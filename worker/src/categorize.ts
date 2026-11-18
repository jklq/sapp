// AI categorisation: a job row is created by POST /v1/categorize and processed
// by the queue consumer, which asks the LLM to split the purchase into spendings.
import { z } from "zod";
import type { Env } from "./env";
import { nowIso } from "./time";

const MAX_TRIES = 3;

/** Sends a prompt to the LLM and returns the reply text. */
export type Complete = (prompt: string) => Promise<string>;

export function openRouter(apiKey: string, model: string): Complete {
  return async (prompt) => {
    const res = await fetch("https://openrouter.ai/api/v1/chat/completions", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${apiKey}` },
      body: JSON.stringify({
        model,
        messages: [
          { role: "system", content: "Du er norsk og tenker på norsk." },
          { role: "user", content: prompt },
        ],
      }),
    });
    const body = await res.text();
    if (!res.ok) throw new Error(`request failed with status code ${res.status}: ${body}`);
    const content = (JSON.parse(body) as { choices?: { message?: { content?: string } }[] }).choices?.[0]?.message?.content;
    if (typeof content !== "string") throw new Error("LLM response had no message content");
    return content;
  };
}

interface Person {
  name: string;
}

interface Params {
  totalAmount: number;
  buyer: Person;
  partner: Person | null;
  prompt: string;
}

const resultSchema = z.object({
  ambiguity_flag: z.string().nullish(),
  spendings: z
    .array(
      z.object({
        category: z.string().nullish().transform((v) => v ?? ""),
        amount: z.number().nullish().transform((v) => v ?? 0),
        apportion_mode: z.string().nullish().transform((v) => v ?? ""),
        description: z.string().nullish().transform((v) => v ?? ""),
      }),
    )
    .nullish()
    .transform((v) => v ?? []),
});
export type JobResult = z.output<typeof resultSchema>;

export function buildPrompt(params: Params, categories: { name: string; ai_notes: string | null }[]): string {
  const categoryList = categories.map((c) => `- ${c.name}${c.ai_notes ? ` - "${c.ai_notes}"` : ""}\n`).join("");
  const buyer = params.buyer.name;
  const partner = params.partner ? params.partner.name || "[Partner Name]" : "[Partner Name]";
  const examplePartner = params.partner?.name || "Partner";
  const userInfo = params.partner
    ? `Personen som har betalt (og oppgitt beskrivelsen) heter ${buyer}. Kjøpet kan potensielt involvere partneren ${partner}.`
    : `Personen som har betalt (og oppgitt beskrivelsen) heter ${buyer}. Det er ingen partner involvert.`;
  const apportionRules = params.partner
    ? `- "alone": Brukes når beskrivelsen indikerer at varen KUN er til kjøperen (${buyer}), ELLER når ingenting om deling/partner er nevnt. Dette er standard antagelse med mindre det er sterke indikasjoner på deling (f.eks. "felles", "oss", "delt", eller kategori som "Groceries").
- "shared": Brukes når beskrivelsen eksplisitt sier at varen er delt ("felles", "oss", "delt"), ELLER når det er en typisk fellesutgift (som "Groceries") OG beskrivelsen ikke indikerer at det er personlig.
- "other": Brukes KUN når beskrivelsen eksplisitt sier at varen er KUN til partneren (${partner}).`
    : `- "alone": Skal brukes for alle deler siden det ikke er noen partner å dele med.`;
  const b = buyer;
  const p = examplePartner;

  return `Du skal nå kategorisere et kjøp ut ifra en liste med kategorier og en beskrivelse på kjøpet. Dette er ET kjøp på EN butikk.
${userInfo}
Totalbeløpet på kjøpet er ${params.totalAmount} kroner.
Beskrivelsen av kjøpet er: "${params.prompt}".

Du skal dele opp kjøpet i en eller flere deler basert på beskrivelsen og totalbeløpet.
For HVER del skal du bestemme 'apportion_mode' basert KUN på beskrivelsen.
Du skal returnere JSON i formatet:
{"ambiguity_flag": "<string>", "spendings":[{"apportion_mode":"shared|alone|other", "category": "<category_name>", "amount": <float>, "description":"<string>"}]}
med en eller flere elementer i "spendings"-listen.

VIKTIGE REGLER FOR 'apportion_mode':
${apportionRules}

- ambiguity_flag: Hvis noe med kjøpet/oppdelingen/beskrivelsen er uklart, fyll strengen med en kort begrunnelse på norsk. Ellers la strengen være tom (""). Ikke bruk den for mye.
- description: Kan være tom string ("") hvis kategorien er beskrivende nok (f.eks. "Dagligvarer").

Eksempler på hvordan 'apportion_mode' skal bestemmes fra beskrivelsen:
1. Beskrivelse: "Redbull til meg for 25kr, og resten er delt middag", Totalbeløp: 100kr, Kjøper: ${b}, Partner: ${p}
   -> [{"apportion_mode":"alone", "category":"...", "amount":25.0, "description":"Redbull"}, {"apportion_mode":"shared", "category":"...", "amount":75.0, "description":"Middag"}]
2. Beskrivelse: "Billetter", Totalbeløp: 500kr, Kjøper: ${b}, Partner: ${p}
   -> [{"apportion_mode":"alone", "category":"...", "amount":500.0, "description":"Billetter"}] (Standard: Anta 'alone' hvis deling ikke er nevnt)
3. Beskrivelse: "Brød til ${p}", Totalbeløp: 40kr, Kjøper: ${b}, Partner: ${p}
   -> [{"apportion_mode":"other", "category":"...", "amount":40.0, "description":"Brød"}] (Fordi det er spesifikt til partneren)
4. Beskrivelse: "Delt lunsj", Totalbeløp: 200kr, Kjøper: ${b}, Partner: ${p}
   -> [{"apportion_mode":"shared", "category":"...", "amount":200.0, "description":"Lunsj"}]
5. Beskrivelse: "Dagligvarer", Totalbeløp: 350kr, Kjøper: ${b}, Partner: ${p}
   -> [{"apportion_mode":"shared", "category":"Groceries", "amount":350.0, "description":"Dagligvarer"}] (Standard for Groceries: Anta 'shared' hvis partner finnes og ikke annet er spesifisert)
6. Beskrivelse: "Genser", Totalbeløp: 600kr, Kjøper: ${b}, Partner: ${p}
   -> [{"apportion_mode":"alone", "category":"Shopping", "amount":600.0, "description":"Genser"}] (Standard for personlige ting: Anta 'alone' hvis deling ikke er nevnt)
7. Beskrivelse: "Flybilletter til oss", Totalbeløp: 2000kr, Kjøper: ${b}, Partner: ${p}
   -> [{"apportion_mode":"shared", "category":"Transport", "amount":2000.0, "description":"Flybilletter"}]

Her er listen av kategorier du kan velge mellom (category_name står først, notater etterpå):
${categoryList}
Bruk den mest spesifikke kategorien. Bruk NØYAKTIG riktig category_name.

Ekskluder sparing og investering fra svaret.
Svaret skal KUN være gyldig JSON, UTEN markdown-formatering. Summen av 'amount' i svaret MÅ være lik totalbeløpet.
`;
}

/** Asks the LLM until the answer is consistent (valid modes, amounts sum to the total). */
export async function categorize(
  params: Params,
  categories: { name: string; ai_notes: string | null }[],
  complete: Complete,
): Promise<JobResult> {
  const prompt = buildPrompt(params, categories);
  for (let attempt = 1; attempt <= MAX_TRIES; attempt++) {
    const parsed = resultSchema.safeParse(JSON.parse(await complete(prompt)));
    if (!parsed.success) throw new Error(`invalid LLM JSON: ${parsed.error.message}`);
    const result = parsed.data;
    const modesOk = result.spendings.every((s) =>
      params.partner ? ["alone", "shared", "other"].includes(s.apportion_mode) : s.apportion_mode === "alone",
    );
    const total = result.spendings.reduce((sum, s) => sum + s.amount, 0);
    if (modesOk && Math.abs(total - params.totalAmount) <= 0.01) return result;
  }
  throw new Error("too many retries");
}

interface JobRow {
  id: number;
  status: string;
  prompt: string;
  buyer: number;
  shared_with: number | null;
  total_amount: number;
  pre_settled: number;
  transaction_date: string;
}

/** Processes one queued job. Failures are recorded on the job, like the Go worker pool. */
export async function processJob(env: Env, jobId: number, complete: Complete): Promise<void> {
  const db = env.DB;
  const job = await db
    .prepare(
      "SELECT id, status, prompt, buyer, shared_with, total_amount, pre_settled, transaction_date FROM ai_categorization_jobs WHERE id = ?",
    )
    .bind(jobId)
    .first<JobRow>();
  if (!job || job.status === "completed") return; // Deleted, or a redelivered message.

  const setStatus = (status: string, error: string | null) =>
    db
      .prepare("UPDATE ai_categorization_jobs SET status = ?, error_message = ?, is_finished = ?, status_updated_at = ? WHERE id = ?")
      .bind(status, error, status === "failed" ? 1 : 0, nowIso(), jobId)
      .run();

  await setStatus("processing", null);
  try {
    const names = await db
      .prepare("SELECT id, first_name FROM users WHERE id IN (?, ?)")
      .bind(job.buyer, job.shared_with ?? -1)
      .all<{ id: number; first_name: string | null }>();
    const nameOf = (id: number) => names.results.find((u) => u.id === id)?.first_name ?? "";
    const { results: categories } = await db
      .prepare("SELECT id, name, ai_notes FROM categories")
      .all<{ id: number; name: string; ai_notes: string | null }>();

    const result = await categorize(
      {
        totalAmount: job.total_amount,
        buyer: { name: nameOf(job.buyer) },
        partner: job.shared_with === null ? null : { name: nameOf(job.shared_with) },
        prompt: job.prompt,
      },
      categories,
      complete,
    );

    const categoryIds = new Map(categories.map((c) => [c.name, c.id]));
    const settledAt = job.pre_settled ? nowIso() : null;
    const statements: D1PreparedStatement[] = [];
    for (const s of result.spendings) {
      const categoryId = categoryIds.get(s.category);
      if (categoryId === undefined) throw new Error(`category '${s.category}' not found or invalid`);
      const sharedWith = s.apportion_mode === "alone" ? null : job.shared_with;
      statements.push(
        db
          .prepare("INSERT INTO spendings (amount, description, category, made_by, spending_date) VALUES (?, ?, ?, ?, ?)")
          .bind(s.amount, s.description || "AI Categorized", categoryId, job.buyer, job.transaction_date),
        db
          .prepare(
            "INSERT INTO user_spendings (spending_id, buyer, shared_with, shared_user_takes_all, settled_at) VALUES (last_insert_rowid(), ?, ?, ?, ?)",
          )
          .bind(job.buyer, sharedWith, s.apportion_mode === "other" ? 1 : 0, settledAt),
        db
          .prepare("INSERT INTO ai_categorized_spendings (job_id, spending_id) SELECT ?, spending_id FROM user_spendings WHERE id = last_insert_rowid()")
          .bind(jobId),
      );
    }
    const reason = result.ambiguity_flag ?? "";
    statements.push(
      db
        .prepare(
          `UPDATE ai_categorization_jobs SET status = 'completed', error_message = NULL, is_finished = 1,
             is_ambiguity_flagged = ?, ambiguity_flag_reason = ?, status_updated_at = ? WHERE id = ?`,
        )
        .bind(reason ? 1 : 0, reason || null, nowIso(), jobId),
    );
    await db.batch(statements);
  } catch (err) {
    console.error(JSON.stringify({ event: "categorize_failed", jobId, error: String(err) }));
    await setStatus("failed", err instanceof Error ? err.message : String(err));
  }
}
