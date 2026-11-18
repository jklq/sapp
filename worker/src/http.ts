import type { z } from "zod";

/** Plain-text error, like Go's http.Error, which the frontend shows as-is. */
export function httpError(status: number, message: string): Response {
  return new Response(`${message}\n`, {
    status,
    headers: { "Content-Type": "text/plain; charset=utf-8", "X-Content-Type-Options": "nosniff" },
  });
}

export function json(body: unknown, status = 200): Response {
  return Response.json(body, { status });
}

/** Parses a JSON body against a schema; null when the body is not valid JSON of that shape. */
export async function readJson<S extends z.ZodType>(request: Request, schema: S): Promise<z.output<S> | null> {
  let raw: unknown;
  try {
    raw = await request.json();
  } catch {
    return null;
  }
  const parsed = schema.safeParse(raw);
  return parsed.success ? parsed.data : null;
}

/** Parses a positive integer path parameter, like strconv.ParseInt in the Go handlers. */
export function parseId(value: string | undefined): number | null {
  if (value === undefined || !/^[+-]?\d+$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}
