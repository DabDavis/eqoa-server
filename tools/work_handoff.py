"""Print the current next actions from the shared work board. Writes no files."""
import argparse
import contextlib
import json
from pathlib import Path
import sqlite3
import sys


COLUMNS = ("id", "version", "status", "owner", "title", "next", "waiting_on")
GROUPS = ("Needs Jibrish", "Waiting on Dab", "Ready for Codex", "Other waits", "Needs a next action")


def entries_export(data):
    if isinstance(data, list):
        return data
    if "rows" in data and "columns" in data:
        return [dict(zip(data["columns"], row, strict=True)) for row in data["rows"]]
    if "entry" in data:
        return [data["entry"]]
    if data.get("more") or data.get("matching", 0) > len(data["entries"]):
        raise ValueError("incomplete work-board page: request the selected IDs or pipe a complete query")
    return data["entries"]


def database_rows(path, ids):
    uri = Path(path).resolve().as_uri() + "?mode=ro"
    with contextlib.closing(sqlite3.connect(uri, uri=True)) as db:
        actual = {r[1] for r in db.execute("PRAGMA table_info(workboard)")}
        if not set(COLUMNS) <= actual:
            raise ValueError("the work board has not exposed its next-action fields")
        db.row_factory = sqlite3.Row
        where = "status IN ('planned','in_progress','blocked')"
        if ids:
            where += " AND id IN (" + ",".join("?" for _ in ids) + ")"
        fields = ",".join(f'"{column}"' for column in COLUMNS)
        return [dict(r) for r in db.execute(f"SELECT {fields} FROM workboard WHERE {where}", ids)]


def handoff(rows, ids=()):
    latest = {}
    for row in rows:
        if not set(COLUMNS) <= row.keys():
            raise ValueError("expected the current work-board fields: " + ", ".join(COLUMNS))
        ident, version = row["id"], row["version"]
        if (not ids or ident in ids) and (ident not in latest or version > latest[ident]["version"]):
            latest[ident] = row
    grouped = {key: [] for key in GROUPS}
    for ident, row in sorted(latest.items()):
        if row["status"] == "done":
            continue
        wait = (row["waiting_on"] or "").strip().casefold()
        owner = (row["owner"] or "").strip().casefold()
        action = (row["next"] or "").strip()
        if wait in {"jibrish", "owner decision", "jibrish's decision"}:
            group = "Needs Jibrish"
        elif wait in {"dab", "dab's sessions"}:
            group = "Waiting on Dab"
        elif wait in {"codex", "jibrish's codex"}:
            group = "Ready for Codex"
        elif wait or row["status"] == "blocked":
            group = "Other waits"
        elif not action:
            group = "Needs a next action"
        elif owner in {"dab", "dab's sessions"}:
            group = "Waiting on Dab"
        elif owner in {"jibrish", "codex", "jibrish's codex"}:
            group = "Ready for Codex"
        else:
            group = "Other waits"
        # Include the version so the next operator can compare it before acting.
        line = f"#{ident} v{row['version']}  {action or row['title']}"
        if group == "Other waits" and wait:
            line += f"  [waiting on {row['waiting_on']}]"
        grouped[group].append(line)
    return grouped


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", help="read-only shared board database; otherwise read JSON from stdin")
    parser.add_argument("--id", action="append", type=int, default=[], help="limit to this work ID (repeatable)")
    args = parser.parse_args()
    rows = database_rows(args.db, args.id) if args.db else entries_export(json.load(sys.stdin))
    groups = handoff(rows, args.id)
    if not any(groups.values()):
        print("No open work in this selection.")
    for name, lines in groups.items():
        if lines:
            print(name)
            print(*lines, sep="\n")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, OSError, TypeError, ValueError, sqlite3.Error) as exc:
        sys.exit(str(exc))
