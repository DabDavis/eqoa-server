#!/usr/bin/env python3
"""The town dialog's lines (the user, 2026-10-29: "add dialog to all town NPCs
and Guards throughout Norrath, but not wildlife of course", "we can improvise
our own in eqoa flavor", "feel free to vary dialog to make the world
interesting"; 2026-11-10: "Let's add more lines, the NPCs talk about random
eqoa lore (remember this is before EQ1 lorewise)").

What a town's people and the guards say when talked to and nothing else
answers is table npcdialog (database/npcdialog.go; who speaks and which line,
entity/dialog.go). No record of the live game's lines for them exists, and
Sandstorm's own were guesses, so these are ours, in sets, a file and a log
of its own each (--set):

    town   tools/dialog/town_lines.json   npcdialog_seed        source "ours"
           the everyday lines, written in EQOA's voice from the cities' lore
           and the dangers near them (the default set);
    lore   tools/dialog/lore_lines.json   npcdialog_lore_seed   source "lore"
           the people on EQOA's own lore, five hundred years before
           EverQuest: each line carries a note naming its fact, and the
           facts and their sources are tools/dialog/LORE-FACTS.md.

Each block of a file is lines and who says them (town, role, name, mood,
race, sex, speaker_race, away, weight, an absent field any; sex "male" or
"female", the speaker's by its body: the user, 2026-10-29, "most of the city
folk are men so would be nice if we had Male/Female lines"; speaker_race a
race's name or number, the speaker's by its body, its "away" lines said only
by a guest of that race); a line is a string, or {"line": ..., "note": ...}
(the note is the reviewer's: the tool does not write it). Each line becomes a
row of the set's source. A table made before the sex, speaker_race and away
columns has them added.

The report says per town how many lines the set has, names a town the file
gives lines that no faction of the database names (a town is its faction's:
"Citizens of X", or a guard faction or House entity/dialog.go maps), and
how many of the lines are in the table already.

--apply ADDS what is new, and only that: a line the set's log holds was
written once (it is as it was, or as a GM left it, or a GM took it away: the
GM's either way, never written again); a line the table holds unlogged, word
for word and selector for selector, is there already (a database whose logs
were dropped: the open database) and is left alone; every other line is
written and logged in the set's log. So a database that holds the set's
earlier lines gets only the later ones, and a second --apply writes nothing.
A logged line the file no longer has is said, and taken out (while still as
written) only with --prune.

--undo takes back the set's lines, each row still as it was written, leaves
one a GM changed (said), and drops the set's log: the other set's rows and
log are not touched. When nothing else was written to npcdialog since, the
table's counter goes back too, and the database is as it was before the
set's first --apply to the byte of its dump. --list prints the set's lines,
each with who says it and its note.

--apply and --undo copy the database first when they have something to write
(<db>.bak-<when>-pre-dialog, SQLite's own backup, so a live database copies
whole). The server reads npcdialog at most 30 s old, so no restart: the next
talk after that hears the lines.

    tools/seed_town_dialog.py [--db eqoa.db] [--set town|lore] [--lines FILE] [--apply [--prune] | --undo | --list]
"""
import argparse
import collections
import datetime
import json
import os
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
# the sets: a file, the log of what --apply wrote of it, and its rows' source
SETS = {
    "town": (os.path.join(HERE, "dialog", "town_lines.json"), "npcdialog_seed", "ours"),
    "lore": (os.path.join(HERE, "dialog", "lore_lines.json"), "npcdialog_lore_seed", "lore"),
}
DEFAULT_LINES = SETS["town"][0]
SOURCE = SETS["town"][2]
ROLES = {"guard", "trader", "priest", "scholar", "worker", "noble", "citizen"}
MOODS = {"neutral", "friendly", "warm", "wary", "hostile"}
SEXES = {"", "male", "female"}
# the races as the wire numbers them (CLAUDE.md's constants), a speaker_race by name
RACES = {"human": 0, "elf": 1, "dark elf": 2, "darkelf": 2, "gnome": 3, "dwarf": 4, "troll": 5, "barbarian": 6,
         "halfling": 7, "erudite": 8, "ogre": 9}
# a 0x0046 box's characters (handlers' questBoxChars) hold the speaker's name, ": " and the line; the longest
# speaker's name on eqoaclassic is 23 characters, so a line past this would go in two boxes
BOX, NAME = 217, 25
LINE_MAX = BOX - NAME - 2
# what a line's words may be filled with (entity.DialogSay)
FILLS = re.compile(r"%[prctn]")
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
	source TEXT NOT NULL DEFAULT 'gm',
	speaker_race INTEGER NOT NULL DEFAULT -1,
	away   INTEGER NOT NULL DEFAULT 0
)"""
# the columns an older table lacks (the server adds them at its start too)
ADDED = (("sex", "TEXT NOT NULL DEFAULT ''"), ("speaker_race", "INTEGER NOT NULL DEFAULT -1"), ("away", "INTEGER NOT NULL DEFAULT 0"))
LOG_SCHEMA = """CREATE TABLE {log} (
    id   INTEGER PRIMARY KEY,   -- npcdialog.id written
    row  TEXT NOT NULL          -- the row as written, JSON
)"""
# entity/dialog.go's townFactions
TOWN_FACTIONS = {"The Freeport Militia": "Freeport", "The Qeynos Guard": "Qeynos", "The Highpass Guard": "Highpass",
                 "Klick`Anon Watch": "Klick`Anon", "House U`Dedne": "Neriak", "House J`Narus": "Neriak",
                 "House Do`Vexis": "Neriak", "House Gal`Saris": "Neriak", "Merchant House Nothard": "Freeport"}
COLS = ("town", "role", "name", "mood", "race", "sex", "line", "weight", "speaker_race", "away")
# a row logged before those columns reads as these
DEFAULTS = {"sex": "", "speaker_race": -1, "away": 0}


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def race_of(v):
    """A speaker_race as the table holds it: -1 none, None when it names no race."""
    if v in ("", None):
        return -1
    if isinstance(v, int) and not isinstance(v, bool):
        return v if -1 <= v <= 9 else None
    return RACES.get(str(v).strip().lower())


def blocks_of(path):
    """The file's lines as (row, note) pairs, and its problems."""
    with open(path, encoding="utf-8") as f:
        d = json.load(f)
    pairs, problems, seen = [], [], {}
    for i, b in enumerate(d["blocks"]):
        role, mood = b.get("role", ""), b.get("mood", "")
        if role and role not in ROLES:
            problems.append(f"block {i}: role {role!r}")
        if mood and any(m.strip() not in MOODS for m in mood.split(",")):
            problems.append(f"block {i}: mood {mood!r}")
        sex = b.get("sex", "")
        if sex not in SEXES:
            problems.append(f"block {i}: sex {sex!r}")
        speaker = race_of(b.get("speaker_race", ""))
        if speaker is None:
            problems.append(f"block {i}: speaker_race {b.get('speaker_race')!r}")
            speaker = -1
        away = 1 if b.get("away") else 0
        if away and speaker < 0:
            problems.append(f"block {i}: away is a speaker_race's")
        for said in b["say"]:
            line, note = (said.get("line", ""), said.get("note", "")) if isinstance(said, dict) else (said, "")
            if not isinstance(line, str) or not line.strip():
                problems.append(f"block {i}: an empty line")
                continue
            if "|" in line or any(ord(c) < 32 for c in line):
                problems.append(f"block {i}: a '|' or a control character (a PS2's text box reads |x as a code): {line[:40]!r}")
            if "%" in FILLS.sub("", line):
                problems.append(f"block {i}: a '%' that is not %p, %r, %c, %t or %n: {line[:40]!r}")
            if len(line) > LINE_MAX:
                problems.append(f"block {i}: {len(line)} characters, past a box's {BOX} with the speaker's name: {line[:40]!r}")
            row = {"town": b.get("town", ""), "role": role, "name": b.get("name", ""), "mood": mood,
                   "race": int(b.get("race", -1)), "sex": sex, "line": line, "weight": int(b.get("weight", 1)),
                   "speaker_race": speaker, "away": away}
            j = as_json(row)
            if j in seen:
                problems.append(f"block {i}: the same line for the same speakers as block {seen[j]}'s: {line[:40]!r}")
                continue
            seen[j] = i
            pairs.append((row, note))
    return pairs, problems


def rows_of(path):
    """The rows the file's blocks make, and its problems."""
    pairs, problems = blocks_of(path)
    return [r for r, _ in pairs], problems


def as_json(r):
    """A row, the file's or the log's, as the one text both are compared by."""
    if isinstance(r, str):
        r = json.loads(r)
    return json.dumps({**DEFAULTS, **r}, sort_keys=True, ensure_ascii=False)


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
    src = sqlite3.connect(path, timeout=30)
    out = sqlite3.connect(dst)
    try:
        src.backup(out)
    finally:
        out.close()
        src.close()
    return dst


def columns(db):
    return [r[1] for r in db.execute("PRAGMA table_info(npcdialog)")]


def ensure_columns(db):
    """The sex, speaker_race and away columns, on a table made before them (the server adds them too)."""
    have = columns(db)
    for c, d in ADDED:
        if have and c not in have:
            db.execute(f"ALTER TABLE npcdialog ADD COLUMN {c} {d}")


def logged(db, log):
    """The set's log: npcdialog id -> the row as written."""
    if not has_table(db, log):
        return {}
    return dict(db.execute(f"SELECT id, row FROM {log} ORDER BY id"))


def current(db, i):
    """npcdialog's row i as a row of the file's shape, and its source; None, None when gone."""
    have = columns(db)
    cols = [c for c in COLS if c in have]
    now = db.execute(f"SELECT {', '.join(cols)}, source FROM npcdialog WHERE id = ?", (i,)).fetchone()
    if now is None:
        return None, None
    return dict(zip(cols, now[:-1])), now[-1]


def as_written(db, i, written, source):
    r, src = current(db, i)
    return r is not None and src == source and as_json(r) == as_json(written)


def present(db):
    """Every row of npcdialog as its text (as_json), whoever wrote it."""
    if not has_table(db, "npcdialog"):
        return set()
    have = columns(db)
    cols = [c for c in COLS if c in have]
    return {as_json(dict(zip(cols, r))) for r in db.execute(f"SELECT {', '.join(cols)} FROM npcdialog")}


def plan(db, rows, log):
    """What --apply would do with the set: the rows to write, how many its log holds already, how many the
    table holds unlogged, and the logged rows the file no longer has, as (id, row as written)."""
    log_rows = {i: as_json(j) for i, j in logged(db, log).items()}
    written, there = set(log_rows.values()), present(db)
    wanted = [as_json(r) for r in rows]
    new = [r for r, j in zip(rows, wanted) if j not in written and j not in there]
    before = sum(1 for j in wanted if j in written)
    unlogged = sum(1 for j in wanted if j not in written and j in there)
    keep = set(wanted)
    dropped = [(i, j) for i, j in log_rows.items() if j not in keep]
    return new, before, unlogged, dropped


def apply(db, rows, log=SETS["town"][1], source=SOURCE, prune=False):
    """The set's new rows written and logged; returns (written ids, taken out, a GM's left)."""
    ids, gone, kept = [], 0, 0
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(SCHEMA)
        ensure_columns(db)
        new, _, _, dropped = plan(db, rows, log)
        if not has_table(db, log):
            db.execute(LOG_SCHEMA.format(log=log))
        for r in new:
            cur = db.execute(f"INSERT INTO npcdialog ({', '.join(COLS)}, source) VALUES ({', '.join('?' * len(COLS))}, ?)",
                             [r[c] for c in COLS] + [source])
            db.execute(f"INSERT INTO {log} VALUES (?, ?)", (cur.lastrowid, json.dumps(r, sort_keys=True, ensure_ascii=False)))
            ids.append(cur.lastrowid)
        if prune:
            for i, j in dropped:
                if as_written(db, i, j, source):
                    db.execute("DELETE FROM npcdialog WHERE id = ?", (i,))
                    gone += 1
                else:
                    kept += 1
                db.execute(f"DELETE FROM {log} WHERE id = ?", (i,))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return ids, gone, kept


def undo(db, log=SETS["town"][1], source=SOURCE):
    """The set's rows taken back, each still as written; returns (gone, a GM's left), None with no log."""
    if not has_table(db, log):
        return None
    db.execute("BEGIN IMMEDIATE")
    try:
        gone = kept = 0
        ensure_columns(db)
        rows = logged(db, log)
        for i, row in rows.items():
            if current(db, i)[0] is None:
                continue
            if not as_written(db, i, row, source):
                kept += 1
                continue
            db.execute("DELETE FROM npcdialog WHERE id = ?", (i,))
            gone += 1
        db.execute(f"DROP TABLE {log}")
        rewind(db, rows)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return gone, kept


def rewind(db, rows):
    """npcdialog's counter put back to where it stood before the set's first row, when nothing else has
    been written to the table since: the first row written took the counter's next number, so no row at
    or past it, and the counter at the set's last, mean every number since was the set's."""
    if not rows or not has_table(db, "sqlite_sequence"):
        return
    first, last = min(rows), max(rows)
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcdialog'").fetchone()
    if seq is None or seq[0] != last:
        return
    if db.execute("SELECT count(*) FROM npcdialog WHERE id >= ?", (first,)).fetchone()[0]:
        return
    if first == 1:  # the table's first rows: it had no counter yet
        db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcdialog'")
    else:
        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcdialog'", (first - 1,))


def who(r):
    """Who says a row, as --list names it."""
    def a_race(n):
        name = next((k for k, v in RACES.items() if v == n), f"race {n}")
        return ("an " if name[0] in "aeiou" else "a ") + name
    parts = [r["town"] or "any town"]
    for k in ("role", "name", "sex"):
        if r[k]:
            parts.append(r[k])
    if r["speaker_race"] >= 0:
        parts.append(a_race(r["speaker_race"]) + (" away from home" if r["away"] else ""))
    if r["race"] >= 0:
        parts.append("to " + a_race(r["race"]))
    if r["mood"]:
        parts.append(f"when {r['mood']}")
    if r["weight"] != 1:
        parts.append(f"weight {r['weight']}")
    return ", ".join(parts)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--set", default="town", choices=sorted(SETS), dest="set_",
                    help="which lines: town, the everyday lines (the default), or lore")
    ap.add_argument("--lines", help="the set's lines, in another file than its own (tests)")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    ap.add_argument("--prune", action="store_true", help="with --apply: take out the logged lines the file no longer has")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the set's new lines (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote of the set")
    g.add_argument("--list", action="store_true", help="print the set's lines, who says each and its note")
    a = ap.parse_args()
    path, log, source = SETS[a.set_]
    path = a.lines or path
    if a.list:
        pairs, problems = blocks_of(path)
        for r, note in pairs:
            print(f"[{who(r)}] {r['line']}" + (f"\n    {note}" if note else ""))
        for p in problems:
            print(f"problem: {p}")
        print(f"{len(pairs)} lines")
        return
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, log):
            print(f"nothing logged in {log}: nothing to undo")
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        gone, kept = undo(db, log, source)
        print(f"took back {gone} lines; {kept} a GM changed since left as they are")
        return
    rows, problems = rows_of(path)
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
    new, before, unlogged, dropped = plan(db, rows, log)
    if before or unlogged or dropped:
        print(f"in the table already: {before} written before ({log})" +
              (f", {unlogged} there unlogged (left alone)" if unlogged else "") + f"; {len(new)} new")
    for i, j in dropped:
        print(f"  logged and no longer in the file (row {i}; --apply --prune takes it out): {json.loads(j)['line'][:60]!r}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not new and not (a.prune and dropped):
        print("nothing new: nothing written")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    ids, gone, kept = apply(db, rows, log, source, a.prune)
    if ids:
        print(f"wrote {len(ids)} lines (npcdialog {min(ids)}-{max(ids)}, logged in {log}); the server hears them within 30 s, no restart")
    if a.prune and dropped:
        print(f"took out {gone} lines the file no longer has; {kept} a GM changed since left as they are")


if __name__ == "__main__":
    main()
