#!/usr/bin/env python3
"""The town dialog's lines (the user, 2026-10-29: "add dialog to all town NPCs
and Guards throughout Norrath, but not wildlife of course", "we can improvise
our own in eqoa flavor", "feel free to vary dialog to make the world
interesting").

What a town's people and the guards say when talked to and nothing else
answers is table npcdialog (database/npcdialog.go; who speaks and which line,
entity/dialog.go). No record of the live game's lines for them exists, and
Sandstorm's own were guesses, so these are ours: tools/dialog/town_lines.json
(--lines), written in EQOA's voice from the cities' lore and the dangers
near them. Each block there is lines and who says them (town, role, name,
mood, race, sex, weight, an absent field any; sex "male" or "female", the
speaker's by its body: the user, 2026-10-29, "most of the city folk are men so
would be nice if we had Male/Female lines"); each line becomes a row of
source "ours". A table made before the sex column has it added.

The report says per town how many lines it has, and names a town the file
gives lines that no faction of the database names (a town is its faction's:
"Citizens of X", or a guard faction or House entity/dialog.go maps).
--apply writes the rows (the table made when the server has not made it yet)
and logs them in npcdialog_seed; a second --apply is refused while the log
is there. --undo deletes each row still as it was written, leaves one a GM
changed (said), and drops the log. Both copy the database first
(<db>.bak-<when>-pre-dialog). The server reads npcdialog at most 30 s old,
so no restart: the next talk after that hears the lines.

    tools/seed_town_dialog.py [--db eqoa.db] [--lines tools/dialog/town_lines.json] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
DEFAULT_LINES = os.path.join(HERE, "dialog", "town_lines.json")
SOURCE = "ours"
ROLES = {"guard", "trader", "priest", "scholar", "worker", "noble", "citizen"}
MOODS = {"neutral", "friendly", "warm", "wary", "hostile"}
SEXES = {"", "male", "female"}
# database/npcdialog.go's npcDialogSchema, word for word
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
LOG_SCHEMA = """CREATE TABLE npcdialog_seed (
    id   INTEGER PRIMARY KEY,   -- npcdialog.id written
    row  TEXT NOT NULL          -- the row as written, JSON
)"""
# entity/dialog.go's townFactions
TOWN_FACTIONS = {"The Freeport Militia": "Freeport", "The Qeynos Guard": "Qeynos", "The Highpass Guard": "Highpass",
                 "Klick`Anon Watch": "Klick`Anon", "House U`Dedne": "Neriak", "House J`Narus": "Neriak",
                 "House Do`Vexis": "Neriak", "House Gal`Saris": "Neriak", "Merchant House Nothard": "Freeport"}
COLS = ("town", "role", "name", "mood", "race", "sex", "line", "weight")


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def rows_of(path):
    """The rows the file's blocks make, and its problems."""
    with open(path) as f:
        d = json.load(f)
    rows, problems = [], []
    for i, b in enumerate(d["blocks"]):
        role, mood = b.get("role", ""), b.get("mood", "")
        if role and role not in ROLES:
            problems.append(f"block {i}: role {role!r}")
        if mood and any(m.strip() not in MOODS for m in mood.split(",")):
            problems.append(f"block {i}: mood {mood!r}")
        sex = b.get("sex", "")
        if sex not in SEXES:
            problems.append(f"block {i}: sex {sex!r}")
        for line in b["say"]:
            if not line.strip():
                problems.append(f"block {i}: an empty line")
                continue
            rows.append({"town": b.get("town", ""), "role": role, "name": b.get("name", ""), "mood": mood,
                         "race": int(b.get("race", -1)), "sex": sex, "line": line, "weight": int(b.get("weight", 1))})
    return rows, problems


def towns_of(db):
    """The towns the database's factions name."""
    if not has_table(db, "npcfaction"):
        return set()
    out = set()
    for (f,) in db.execute("SELECT DISTINCT faction FROM npcfaction"):
        if f.startswith("Citizens of "):
            out.add(f[len("Citizens of "):])
        elif f in TOWN_FACTIONS:
            out.add(TOWN_FACTIONS[f])
    return out


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-dialog"
    shutil.copy2(path, dst)
    return dst


def ensure_sex(db):
    """The sex column, on a table made before it (the server adds it too)."""
    if has_table(db, "npcdialog") and "sex" not in [r[1] for r in db.execute("PRAGMA table_info(npcdialog)")]:
        db.execute("ALTER TABLE npcdialog ADD COLUMN sex TEXT NOT NULL DEFAULT ''")


def apply(db, rows):
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(SCHEMA)
        ensure_sex(db)
        db.execute(LOG_SCHEMA)
        for r in rows:
            cur = db.execute(f"INSERT INTO npcdialog ({', '.join(COLS)}, source) VALUES ({', '.join('?' * len(COLS))}, ?)",
                             [r[c] for c in COLS] + [SOURCE])
            db.execute("INSERT INTO npcdialog_seed VALUES (?, ?)", (cur.lastrowid, json.dumps(r, sort_keys=True)))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "npcdialog_seed"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        gone = kept = 0
        ensure_sex(db)
        for i, row in db.execute("SELECT id, row FROM npcdialog_seed").fetchall():
            r = json.loads(row)
            now = db.execute(f"SELECT {', '.join(COLS)}, source FROM npcdialog WHERE id = ?", (i,)).fetchone()
            if now is None:
                continue
            have = dict(zip(COLS, now[:-1]))
            # a row logged before the sex column has no "sex": it is compared on what it was written with
            if any(have.get(k) != v for k, v in r.items()) or have.get("sex", "") != r.get("sex", "") or now[-1] != SOURCE:
                kept += 1
                continue
            db.execute("DELETE FROM npcdialog WHERE id = ?", (i,))
            gone += 1
        db.execute("DROP TABLE npcdialog_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {gone} lines; {kept} a GM changed since left as they are")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--lines", default=DEFAULT_LINES, help="the lines (default tools/dialog/town_lines.json)")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the lines (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "npcdialog_seed"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "npcdialog_seed"):
        sys.exit("npcdialog_seed is there: seeded already (--undo first)")
    rows, problems = rows_of(a.lines)
    for p in problems:
        print(f"problem: {p}")
    if problems:
        sys.exit("the lines have problems: nothing written")
    towns = towns_of(db)
    per = collections.Counter(r["town"] or "(any town)" for r in rows)
    for t, n in sorted(per.items()):
        mark = "" if t == "(any town)" or t in towns else "   <- no faction of this database names it"
        print(f"  {t}: {n} lines{mark}")
    silent = sorted(towns - set(per))
    if silent:
        print(f"towns with no lines of their own (they say the generic ones): {', '.join(silent)}")
    print(f"{len(rows)} lines, {len(per) - (1 if '(any town)' in per else 0)} towns")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, rows)
    print(f"wrote {len(rows)} lines; the server hears them within 30 s, no restart")


if __name__ == "__main__":
    main()
