import { describe, expect, it } from "vitest";
import { api, db, login, seedPartners } from "./helpers";

describe("auth", () => {
  it("logs in, verifies the access token and rejects bad credentials", async () => {
    const { alice } = await seedPartners();
    expect(await (await api("GET", "/v1/verify", alice)).json()).toEqual({ user_id: 1, first_name: "Alice" });

    const bad = await api("POST", "/v1/login", undefined, { username: "alice", password: "nope" });
    expect(bad.status).toBe(401);
    expect(await bad.text()).toBe("Invalid credentials\n");
    expect((await api("GET", "/v1/verify")).status).toBe(401);
    expect((await api("GET", "/v1/verify", "garbage")).status).toBe(401);
  });

  it("accepts refresh tokens stored by the Go backend (padded URL-safe base64 SHA-256)", async () => {
    await seedPartners();
    // base64.URLEncoding.EncodeToString(sha256("abc")) in Go.
    const goHash = "ungWv48Bz-pBQUDeXa4iI7ADYaOWF3qctBD_YfIAFa0=";
    await db()
      .prepare("INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (1, ?, '2999-01-01T00:00:00.000Z')")
      .bind(goHash)
      .run();
    const res = await api("POST", "/v1/refresh", undefined, { refresh_token: "abc" });
    expect(res.status).toBe(200);
    const { access_token } = (await res.json()) as { access_token: string };
    expect((await api("GET", "/v1/verify", access_token)).status).toBe(200);
  });

  it("rejects and deletes expired refresh tokens", async () => {
    await seedPartners();
    await db()
      .prepare("INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (1, 'ungWv48Bz-pBQUDeXa4iI7ADYaOWF3qctBD_YfIAFa0=', '2000-01-01T00:00:00.000Z')")
      .run();
    const res = await api("POST", "/v1/refresh", undefined, { refresh_token: "abc" });
    expect(res.status).toBe(401);
    expect(await res.text()).toBe("refresh token expired\n");
    expect(await db().prepare("SELECT COUNT(*) AS n FROM refresh_tokens WHERE expires_at < '2001'").first("n")).toBe(0);
  });

  it("registers partners once", async () => {
    const body = {
      user1: { username: "carol", password: "secret1", first_name: "Carol" },
      user2: { username: "dave", password: "secret2", first_name: "Dave" },
    };
    const res = await api("POST", "/v1/register/partners", undefined, body);
    expect(res.status).toBe(201);
    const { user1_id, user2_id } = (await res.json()) as { user1_id: number; user2_id: number };
    expect(await db().prepare("SELECT user1_id, user2_id FROM partnerships").first()).toEqual({
      user1_id: Math.min(user1_id, user2_id),
      user2_id: Math.max(user1_id, user2_id),
    });
    expect((await api("POST", "/v1/register/partners", undefined, body)).status).toBe(409);
    expect(await login("dave", "secret2")).toBeTruthy();
  });
});
