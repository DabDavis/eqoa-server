#!/usr/bin/env python3
"""NPCs' own lines (the user, 2026-11-01: "give him specific lines talking
about his brother, etc. special dialog and not the pool", "we can have pools
per character if needed, especially for quest npcs").

An NPC with lines of its own name in npcdialog says those alone when talked
to with nothing else to say (entity/dialog.go: its own pool, never the town
lines, wherever it stands and whatever its body). They are ours, kept in
tools/dialog/npc_lines.json (--lines): blocks of a name and its lines, each
with a mood, a race and a weight as town_lines.json's; each line becomes a row
of source "ours" (town, role and sex empty).

The report says per NPC how many lines the file gives it, how many of them
are in the table, and how many spawns of that name npcs has (none: said).
--apply brings the table to the file, so it is run again after an edit: a line
not written before is added and logged in npcdialog_own_seed, a logged line
the file no longer has is taken out while still as written; a line a GM
changed or deleted since stays the GM's (said, and not written again). --undo
takes out every logged line still as written and drops the log. Both copy the
database first (<db>.bak-<when>-pre-npclines, SQLite's own backup, so a live
database copies whole). The server reads npcdialog at most 30 s old: no
restart, the next talk after that hears them.

    tools/seed_npc_lines.py [--db eqoa.db] [--lines tools/dialog/npc_lines.json] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
DEFAULT_LINES = os.path.join(HERE, "dialog", "npc_lines.json")
SOURCE = "ours"
MOODS = {"neutral", "friendly", "warm", "wary", "hostile"}
BOX = 217  # a 0x0046 box's characters (handlers' questBoxChars), the speaker's name before the line
# database/npcdialog.go's npcDialogSchema, word for word (seed_town_dialog.py's)
SCHEMA = """CREATE TABLE IF NOT EXISTS npcdialog (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	town   TEXT NOT NULL DEFAULT '',
	role   TEXT NOT NULL DEFAULT '',
	name   TEXT NOT NULL DEFAULT '',
	mood   TEXT NOT NULL DEFAULT '',
	race   INTEGER NOT NULL DEFAULT -1,
	sex    TEXT NOT NULL DEFAULT '',
	line   TEXT NOT NULL,
	weight INTEGER NOT NULL DEFAULT 1,
	source TEXT NOT NULL DEFAULT 'gm'
)"""
LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS npcdialog_own_seed (
    id   INTEGER PRIMARY KEY,   -- npcdialog.id written
    row  TEXT NOT NULL          -- the row as written, JSON
)"""
COLS = ("town", "role", "name", "mood", "race", "sex", "line", "weight")


def key(name):
    """A name as npcclass keys one (database.NPCClassKey)."""
    n = (name or "").strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def rows_of(path):
    """The rows the file's blocks make (each its JSON), and its problems."""
    with open(path, encoding="utf-8") as f:
        d = json.load(f)
    rows, problems = [], []
    for i, b in enumerate(d.get("blocks") or []):
        name, mood = (b.get("name") or "").strip(), (b.get("mood") or "").strip()
        if not name:
            problems.append(f"block {i}: no name")
            continue
        if mood and any(m.strip() not in MOODS for m in mood.split(",")):
            problems.append(f"block {i} ({name}): mood {mood!r}")
        for line in b.get("say") or []:
            if not isinstance(line, str) or not line.strip():
                problems.append(f"block {i} ({name}): an empty line")
                continue
            if len(f"{name}: {line}") > BOX:
                problems.append(f"block {i} ({name}): {len(name) + 2 + len(line)} characters with the name, past a box's {BOX}: {line[:40]!r}")
            rows.append({"town": "", "role": "", "name": name, "mood": mood, "race": int(b.get("race", -1)),
                         "sex": "", "line": line, "weight": int(b.get("weight", 1))})
    return rows, problems


def as_json(r):
    return json.dumps(r, sort_keys=True, ensure_ascii=False)


def current(db, i):
    """npcdialog's row i as a row of the file's shape, and its source; None, None when gone."""
    now = db.execute(f"SELECT {', '.join(COLS)}, source FROM npcdialog WHERE id = ?", (i,)).fetchone()
    if now is None:
        return None, None
    return dict(zip(COLS, now[:-1])), now[-1]


def as_written(db, i, written):
    r, src = current(db, i)
    return r is not None and src == SOURCE and as_json(r) == written


def logged(db):
    if not has_table(db, "npcdialog_own_seed"):
        return {}
    return dict(db.execute("SELECT id, row FROM npcdialog_own_seed"))


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-npclines"
    src = sqlite3.connect(path, timeout=30)
    out = sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def report(db, rows):
    """Per NPC: the file's lines, those in the table, its spawns."""
    have = collections.Counter(json.loads(r)["name"] for r in logged(db).values())
    per = collections.Counter(r["name"] for r in rows)
    spawns = collections.Counter()
    if has_table(db, "npcs"):
        for (n,) in db.execute("SELECT npc_name FROM npcs"):
            spawns[key(n)] += 1
    for name, n in sorted(per.items()):
        s = spawns.get(key(name), 0)
        print(f"  {name}: {n} lines, {have.get(name, 0)} logged in the table, {s} spawns" +
              ("   <- no NPC of this name" if s == 0 else ""))
    return per


def apply(db, rows):
    """The table brought to the file; returns (added, taken out, a GM's kept)."""
    added = gone = kept = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(SCHEMA)
        db.execute(LOG_SCHEMA)
        log = logged(db)
        written = set(log.values())
        wanted = {as_json(r): r for r in rows}
        for j, r in wanted.items():
            if j in written:
                continue  # written once: as it is now, or as a GM left it
            cur = db.execute(f"INSERT INTO npcdialog ({', '.join(COLS)}, source) VALUES ({', '.join('?' * len(COLS))}, ?)",
                             [r[c] for c in COLS] + [SOURCE])
            db.execute("INSERT INTO npcdialog_own_seed VALUES (?, ?)", (cur.lastrowid, j))
            added += 1
        for i, j in log.items():
            if j in wanted:
                continue
            if as_written(db, i, j):
                db.execute("DELETE FROM npcdialog WHERE id = ?", (i,))
                gone += 1
            else:
                kept += 1
                print(f"  left as a GM has it (row {i}): {json.loads(j)['line'][:60]!r}")
            db.execute("DELETE FROM npcdialog_own_seed WHERE id = ?", (i,))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return added, gone, kept


def undo(db):
    log = logged(db)
    if not log:
        print("nothing logged: nothing to undo")
        return 0, 0
    gone = kept = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for i, j in log.items():
            if as_written(db, i, j):
                db.execute("DELETE FROM npcdialog WHERE id = ?", (i,))
                gone += 1
            elif current(db, i)[0] is not None:
                kept += 1
        db.execute("DROP TABLE npcdialog_own_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return gone, kept


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--lines", default=DEFAULT_LINES, help="the lines (default tools/dialog/npc_lines.json)")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="bring the table to the file (logged)")
    g.add_argument("--undo", action="store_true", help="take out every logged line still as written")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if (a.apply or a.undo) and not a.no_backup:
        print(f"copy: {backup(a.db)}")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        gone, kept = undo(db)
        print(f"took out {gone} lines; {kept} a GM changed since left as they are")
        return
    rows, problems = rows_of(a.lines)
    for p in problems:
        print(f"problem: {p}")
    if problems:
        sys.exit("the lines have problems: nothing written")
    per = report(db, rows)
    print(f"{len(rows)} lines for {len(per)} NPCs")
    if a.apply:
        added, gone, kept = apply(db, rows)
        print(f"added {added}, took out {gone}, left {kept} a GM changed; the server hears them within 30 s")
    else:
        print("a report: --apply to write")


if __name__ == "__main__":
    main()
