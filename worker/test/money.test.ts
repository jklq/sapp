import { describe, expect, it } from "vitest";
import { api, seedPartners } from "./helpers";

const pay = (token: string, body: object) => api("POST", "/v1/pay", token, { pre_settled: false, ...body });

describe("payments, transfers and stats", () => {
  it("tracks what the partner owes and settles it", async () => {
    const { alice, bob } = await seedPartners();
    expect((await pay(alice, { shared_status: "shared", amount: 100, category: "Groceries" })).status).toBe(201);
    expect((await pay(alice, { shared_status: "alone", amount: 40, category: "Coffee" })).status).toBe(201);

    expect(await (await api("GET", "/v1/transfer/status", bob)).json()).toEqual({
      partner_name: "Alice",
      amount_owed: 50,
      owed_by: "Bob",
      owed_to: "Alice",
    });
    expect((await api("POST", "/v1/transfer/record", bob)).status).toBe(200);
    expect(await (await api("GET", "/v1/transfer/status", alice)).json()).toEqual({
      partner_name: "Bob",
      amount_owed: 0,
      owed_by: null,
      owed_to: null,
    });
  });

  it("validates payments", async () => {
    const { alice } = await seedPartners();
    expect(await (await pay(alice, { shared_status: "alone", amount: 0, category: "Coffee" })).text()).toBe("Amount must be positive\n");
    expect(await (await pay(alice, { shared_status: "both", amount: 5, category: "Coffee" })).text()).toBe("Invalid shared status.\n");
    expect(await (await pay(alice, { shared_status: "alone", amount: 5, category: "Nope" })).text()).toBe("Category not found.\n");
    expect((await pay(alice, { shared_status: "alone", amount: "5", category: "Coffee" })).status).toBe(400);
  });

  it("splits spending stats by share and includes the whole date range", async () => {
    const { alice, bob } = await seedPartners();
    await pay(alice, { shared_status: "shared", amount: 100, category: "Groceries", spending_date: "2026-03-01" });
    await pay(alice, { shared_status: "alone", amount: 30, category: "Coffee", spending_date: "2026-03-31" });
    await pay(alice, { shared_status: "alone", amount: 999, category: "Coffee", spending_date: "2026-04-01" });

    const stats = (token: string) => api("GET", "/v1/stats/spending?startDate=2026-03-01&endDate=2026-03-31", token).then((r) => r.json());
    expect(await stats(alice)).toEqual([
      { category_name: "Groceries", total_amount: 50 },
      { category_name: "Coffee", total_amount: 30 },
    ]);
    expect(await stats(bob)).toEqual([{ category_name: "Groceries", total_amount: 50 }]);
  });

  it("counts recurring deposits like Go's AddDate (Jan 31 + 1 month = Mar 3)", async () => {
    const { alice } = await seedPartners();
    const add = await api("POST", "/v1/deposits", alice, {
      amount: 1000,
      description: "Salary",
      deposit_date: "2026-01-31",
      is_recurring: true,
      recurrence_period: "monthly",
    });
    expect(add.status).toBe(201);
    const stats = await api("GET", "/v1/stats/deposits?startDate=2026-01-01&endDate=2026-03-31", alice);
    expect(await stats.json()).toEqual({ total_amount: 2000, count: 2 });

    const { deposit_id } = (await add.json()) as { deposit_id: number };
    const update = await api("PUT", `/v1/deposits/${deposit_id}`, alice, { is_recurring: false, end_date: "2026-06-01" });
    const { deposit } = (await update.json()) as { deposit: Record<string, unknown> };
    expect(deposit).toMatchObject({ is_recurring: false, recurrence_period: null, end_date: null });
  });
});
