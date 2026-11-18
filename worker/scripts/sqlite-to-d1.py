"""Convert the Go backend's SQLite database into SQL for the D1 database.

Usage: python3 -I scripts/sqlite-to-d1.py <sapp.db> > import.sql
       wrangler d1 execute sapp --remote --file import.sql

Replaces all rows in D1; rows keep their ids. DATETIME columns are normalised to ISO 8601 UTC with
milliseconds ("2026-03-24T09:18:20.659Z"); the Go backend stored several formats.
Rows that break a foreign key (orphans from before foreign keys were enforced)
are skipped and reported on stderr.
"""
import re
import sqlite3
import sys
from datetime import datetime, timedelta, timezone

# Parent tables first, so foreign keys hold while importing.
TABLES = [
    "users", "categories", "spendings", "user_spendings", "ai_categorization_jobs",
    "ai_categorized_spendings", "transfers", "partnerships", "deposits", "refresh_tokens",
]

GO_TIME = re.compile(r"^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2})(\.\d+)? ([+-])(\d{2})(\d{2}) \S+( m=[+-][\d.]+)?$")
ISO_TIME = re.compile(r"^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(\.\d+)?(Z|([+-])(\d{2}):(\d{2}))?$")


def normalise(value: str) -> str:
    if m := GO_TIME.match(value):
        date, time, frac, sign, hh, mm = m.group(1, 2, 3, 4, 5, 6)
    elif m := ISO_TIME.match(value):
        date, time, frac, zone = m.group(1, 2, 3, 4)
        sign, hh, mm = ("+", "00", "00") if zone in (None, "Z") else m.group(5, 6, 7)
    else:
        raise ValueError(f"unrecognised timestamp: {value!r}")
    micros = int(((frac or ".0")[1:] + "000000")[:6])
    local = datetime.fromisoformat(f"{date}T{time}").replace(microsecond=micros)
    offset = timedelta(hours=int(hh), minutes=int(mm)) * (1 if sign == "+" else -1)
    utc = (local - offset).replace(tzinfo=timezone.utc)
    return utc.strftime("%Y-%m-%dT%H:%M:%S.") + f"{utc.microsecond // 1000:03d}Z"


def literal(value) -> str:
    if value is None:
        return "NULL"
    if isinstance(value, (int, float)):
        return repr(value)
    return "'" + str(value).replace("'", "''") + "'"


def main(path: str) -> None:
    db = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    orphans = {(table, rowid) for table, rowid, *_ in db.execute("PRAGMA foreign_key_check")}
    for table, rowid in sorted(orphans):
        print(f"skipping orphaned {table} row {rowid}", file=sys.stderr)
    # Full replace: also drops the seeded categories, and is safe to re-run for a final sync.
    out = [f"DELETE FROM {table};" for table in reversed(TABLES)]
    for table in TABLES:
        info = db.execute(f"PRAGMA table_info({table})").fetchall()
        columns = [c[1] for c in info]
        datetimes = {c[1] for c in info if c[2].upper() == "DATETIME"}
        for rowid, *row in db.execute(f"SELECT rowid, {', '.join(columns)} FROM {table} ORDER BY rowid"):
            if (table, rowid) in orphans:
                continue
            values = [
                literal(normalise(v) if col in datetimes and isinstance(v, str) else v)
                for col, v in zip(columns, row)
            ]
            out.append(f"INSERT INTO {table} ({', '.join(columns)}) VALUES ({', '.join(values)});")
    print("\n".join(out))


if __name__ == "__main__":
    main(sys.argv[1])
