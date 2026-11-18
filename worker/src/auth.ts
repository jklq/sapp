import bcrypt from "bcryptjs";
import { Hono, type MiddlewareHandler } from "hono";
import { sign, verify } from "hono/jwt";
import { JwtTokenExpired } from "hono/utils/jwt/types";
import { z } from "zod";
import type { AppEnv } from "./env";
import { httpError, json, readJson } from "./http";
import { nowIso } from "./time";

const ACCESS_TOKEN_SECONDS = 15 * 60;
const REFRESH_TOKEN_MS = 30 * 24 * 60 * 60 * 1000;
const ISSUER = "sapp-backend";

// Go's base64.URLEncoding: URL-safe alphabet *with* padding. Stored hashes depend on it.
function base64UrlPadded(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_");
}

async function hashToken(token: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(token));
  return base64UrlPadded(new Uint8Array(digest));
}

async function accessToken(userId: number, secret: string): Promise<string> {
  const now = Math.floor(Date.now() / 1000);
  return sign({ user_id: userId, exp: now + ACCESS_TOKEN_SECONDS, iat: now, nbf: now, iss: ISSUER }, secret, "HS256");
}

export const requireAuth: MiddlewareHandler<AppEnv> = async (c, next) => {
  const secret = c.env.JWT_SECRET_KEY;
  if (!secret) return httpError(500, "Internal Server Error: Service configuration incomplete");

  const header = c.req.header("Authorization");
  if (!header) return httpError(401, "Authorization header required");
  const space = header.indexOf(" ");
  if (space < 0 || header.slice(0, space).toLowerCase() !== "bearer") {
    return httpError(401, "Authorization header format must be Bearer {token}");
  }

  let userId: unknown;
  try {
    userId = (await verify(header.slice(space + 1), secret, "HS256")).user_id;
  } catch (err) {
    return httpError(401, err instanceof JwtTokenExpired ? "Token expired" : "Invalid token");
  }
  if (typeof userId !== "number" || !Number.isSafeInteger(userId) || userId <= 0) {
    return httpError(401, "Invalid token claims");
  }
  c.set("userId", userId);
  await next();
};

/** The user's partner, if any. */
export async function partnerOf(db: D1Database, userId: number): Promise<number | null> {
  const row = await db
    .prepare("SELECT user2_id AS id FROM partnerships WHERE user1_id = ?1 UNION SELECT user1_id FROM partnerships WHERE user2_id = ?1")
    .bind(userId)
    .first<{ id: number }>();
  return row?.id ?? null;
}

export async function firstName(db: D1Database, userId: number): Promise<string | null> {
  const row = await db.prepare("SELECT first_name FROM users WHERE id = ?").bind(userId).first<{ first_name: string | null }>();
  return row ? (row.first_name ?? "") : null;
}

const str = z.string().nullish().transform((v) => v ?? "");
const loginSchema = z.object({ username: str, password: str });
const refreshSchema = z.object({ refresh_token: str });
const registrationUser = z
  .object({ username: str, password: str, first_name: str })
  .nullish()
  .transform((v) => v ?? { username: "", password: "", first_name: "" });
const registrationSchema = z.object({ user1: registrationUser, user2: registrationUser });

export const authRoutes = new Hono<AppEnv>()
  .post("/login", async (c) => {
    const req = await readJson(c.req.raw, loginSchema);
    if (!req) return httpError(400, "Invalid request body");
    if (!req.username || !req.password) return httpError(400, "Username and password are required");

    const user = await c.env.DB.prepare("SELECT id, password_hash, first_name FROM users WHERE username = ?")
      .bind(req.username)
      .first<{ id: number; password_hash: string; first_name: string | null }>();
    if (!user || !(await bcrypt.compare(req.password, user.password_hash))) {
      return httpError(401, "Invalid credentials");
    }
    if (!c.env.JWT_SECRET_KEY) return httpError(500, "Internal Server Error: Service configuration incomplete");

    const refreshToken = base64UrlPadded(crypto.getRandomValues(new Uint8Array(32)));
    await c.env.DB.batch([
      c.env.DB.prepare("DELETE FROM refresh_tokens WHERE user_id = ?").bind(user.id),
      c.env.DB.prepare("INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (?, ?, ?)").bind(
        user.id,
        await hashToken(refreshToken),
        new Date(Date.now() + REFRESH_TOKEN_MS).toISOString(),
      ),
    ]);
    return json({
      access_token: await accessToken(user.id, c.env.JWT_SECRET_KEY),
      refresh_token: refreshToken,
      user_id: user.id,
      first_name: user.first_name ?? "",
    });
  })
  .post("/refresh", async (c) => {
    const req = await readJson(c.req.raw, refreshSchema);
    if (!req) return httpError(400, "Invalid request body");
    if (!req.refresh_token) return httpError(400, "Refresh token is required");

    const hash = await hashToken(req.refresh_token);
    const row = await c.env.DB.prepare("SELECT user_id, expires_at FROM refresh_tokens WHERE token_hash = ?")
      .bind(hash)
      .first<{ user_id: number; expires_at: string }>();
    if (!row) return httpError(401, "invalid refresh token");
    if (row.expires_at < nowIso()) {
      await c.env.DB.prepare("DELETE FROM refresh_tokens WHERE token_hash = ?").bind(hash).run();
      return httpError(401, "refresh token expired");
    }
    if (!c.env.JWT_SECRET_KEY) return httpError(500, "Internal Server Error: Service configuration incomplete");
    return json({ access_token: await accessToken(row.user_id, c.env.JWT_SECRET_KEY) });
  })
  .post("/register/partners", async (c) => {
    const req = await readJson(c.req.raw, registrationSchema);
    if (!req) return httpError(400, "Invalid request body");
    const { user1: u1, user2: u2 } = req;
    if ([u1, u2].some((u) => !u.username || !u.password || !u.first_name)) {
      return httpError(400, "All fields (username, password, first name) are required for both users");
    }
    if (u1.username === u2.username) return httpError(400, "Usernames must be different");
    const bytes = (s: string) => new TextEncoder().encode(s).length;
    if (bytes(u1.password) < 6 || bytes(u2.password) < 6) {
      return httpError(400, "Password must be at least 6 characters long");
    }

    const db = c.env.DB;
    const existing = await db
      .prepare("SELECT COUNT(*) AS n FROM users WHERE username = ? OR username = ?")
      .bind(u1.username, u2.username)
      .first<{ n: number }>();
    if (existing && existing.n > 0) return httpError(409, "One or both usernames already exist");

    const [hash1, hash2] = await Promise.all([bcrypt.hash(u1.password, 10), bcrypt.hash(u2.password, 10)]);
    const insertUser = "INSERT INTO users (username, password_hash, first_name) VALUES (?, ?, ?)";
    const [r1, r2] = await db.batch([
      db.prepare(insertUser).bind(u1.username, hash1, u1.first_name),
      db.prepare(insertUser).bind(u2.username, hash2, u2.first_name),
      db
        .prepare(
          `INSERT INTO partnerships (user1_id, user2_id)
           SELECT min(a.id, b.id), max(a.id, b.id) FROM users a, users b WHERE a.username = ? AND b.username = ?`,
        )
        .bind(u1.username, u2.username),
    ]);
    return json(
      {
        message: "Users registered and partnered successfully",
        user1_id: r1.meta.last_row_id,
        user2_id: r2.meta.last_row_id,
      },
      201,
    );
  });

export const verifyRoute = new Hono<AppEnv>().get("/", async (c) => {
  const name = await firstName(c.env.DB, c.var.userId);
  if (name === null) return httpError(401, "User not found");
  return json({ user_id: c.var.userId, first_name: name });
});
