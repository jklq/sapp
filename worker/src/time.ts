// Timestamps are stored and returned as ISO 8601 UTC strings.

export const nowIso = (): string => new Date().toISOString();

/** Parses a strict "YYYY-MM-DD" date (Go layout "2006-01-02") to UTC midnight. */
export function parseYmd(value: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!m) return null;
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const date = new Date(Date.UTC(y, mo - 1, d));
  return date.getUTCFullYear() === y && date.getUTCMonth() === mo - 1 && date.getUTCDate() === d ? date : null;
}

export type RecurrencePeriod = "weekly" | "monthly" | "yearly";
export const RECURRENCE_PERIODS: readonly string[] = ["weekly", "monthly", "yearly"];

/** Next occurrence, normalising overflow like Go's time.AddDate (Jan 31 + 1 month = Mar 3). */
export function nextOccurrence(current: Date, period: string): Date | null {
  const add = { weekly: [0, 0, 7], monthly: [0, 1, 0], yearly: [1, 0, 0] }[period];
  if (!add) return null;
  return new Date(
    Date.UTC(
      current.getUTCFullYear() + add[0],
      current.getUTCMonth() + add[1],
      current.getUTCDate() + add[2],
      current.getUTCHours(),
      current.getUTCMinutes(),
      current.getUTCSeconds(),
      current.getUTCMilliseconds(),
    ),
  );
}
