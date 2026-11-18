import bcrypt from "bcryptjs";
import { env, exports } from "cloudflare:workers";

export const db = () => env.DB;

/** Creates two partnered users (ids 1 and 2, password "password") and returns their access tokens. */
export async function seedPartners(): Promise<{ alice: string; bob: string }> {
  const hash = bcrypt.hashSync("password", 4);
  await env.DB.batch([
    env.DB.prepare("INSERT INTO users (id, username, password_hash, first_name) VALUES (1, 'alice', ?, 'Alice'), (2, 'bob', ?, 'Bob')").bind(hash, hash),
    env.DB.prepare("INSERT INTO partnerships (user1_id, user2_id) VALUES (1, 2)"),
  ]);
  return { alice: await login("alice"), bob: await login("bob") };
}

export async function login(username: string, password = "password"): Promise<string> {
  const res = await api("POST", "/v1/login", undefined, { username, password });
  return ((await res.json()) as { access_token: string }).access_token;
}

export function api(method: string, path: string, token?: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  return exports.default.fetch(
    new Request(`https://sapp.test${path}`, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) }),
  );
}
