#!/usr/bin/env python3
"""The seed row of npctint: Yhafiran, the black and green oriental dragon,
drawn by eqoa-rust-client-vanilla in a tint (entity/npctint.go; the server
owner, 2026-11-10).

Yhafiran wears the disc's only green oriental dragon (CHAR.ESF 0xB5C8A651),
the body the Plane of Disease's twins Uzrathixius and Poxathixius wear, and no
black and green body exists on the disc, so the owner chose: "vanilla draws
Yhafiran tinted toward black and green". The tint is 0x2A7333FF: each of the
body's colours' red times 0.16, green 0.45, blue 0.20, which leaves him nearly
black in the noon light with the green of his scales showing, a deep green on
black in the morning sun, and never the twins' bright green; picked from
vanilla's `eqoa shot CHAR.ESF 0xB5C8A651 <out.png> tint=RRGGBBAA` at four
candidates (0x73B366FF, 0x598C54FF, 0x4D804DFF and this). A PS2 draws him
plain green: only a client that answers the server's offer is told.

The row is keyed by name, so it holds whether or not an NPC of the name stands
in npcs yet (the report says how many do), and every spawn of the name is
tinted. It is written with source "seed" where no row keys to the name (a GM's
row, in any case, is left as it is and listed); a seed row in another colour
(an earlier tint of this tool's) takes this one. Every write is logged in
npc_tint_seed_log (the row before, if any, and the colour written, and whether
this tool made the table), so --undo puts back exactly what was there: the row
deleted or given back its colour and note, the table dropped when this tool
made it and nothing else is in it, the log dropped; a row changed since is
left and listed. Both take a copy of the database first (--no-backup to skip).
Report only by default; a second --apply writes nothing.

The server reads npctint at its start: run this before a server with the table
starts, or after it and then the GM panel's "Reload every NPC" (reload_npcs),
which sends the tints to every client that draws them; a GM's edit of the
panel's NPC body tints goes at once by itself.

    tools/seed_npc_tint.py [--db eqoa.db]            report
    tools/seed_npc_tint.py [--db eqoa.db] --apply    write it
    tools/seed_npc_tint.py [--db eqoa.db] --undo     take it back
"""
import argparse
import datetime
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"
LOG = "npc_tint_seed_log"

# the table as database/npctint.go makes it
SCHEMA = """CREATE TABLE IF NOT EXISTS npctint (
    name   TEXT PRIMARY KEY,
    color  INTEGER NOT NULL DEFAULT 4294967295,
    note   TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm'
)"""

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq         INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    had_row     INTEGER NOT NULL,
    color_was   INTEGER,
    note_was    TEXT,
    source_was  TEXT,
    color       INTEGER NOT NULL,
    note        TEXT NOT NULL,
    made_table  INTEGER NOT NULL,
    at          TEXT NOT NULL
)"""

# the seed's tints: name, 0xRRGGBBAA, note
TINTS = [
    ("Yhafiran", 0x2A7333FF,
     "the black and green oriental dragon: the disc's green one (CHAR.ESF 0xB5C8A651, the Plane of Disease "
     "twins' body) drawn in red x0.16, green x0.45, blue x0.20 by vanilla; a PS2 draws it green"),
]


def key(name):
    """The name as database.NPCClassKey keys it: lower case, trimmed, no
    trailing '$'."""
    n = (name or "").strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-npctintseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def rows_by_key(db):
    """npctint's rows by their names' key: name, color, note, source."""
    if not has_table(db, "npctint"):
        return {}
    out = {}
    for name, color, note, source in db.execute("SELECT name, color, note, source FROM npctint ORDER BY name"):
        out.setdefault(key(name), []).append((name, color, note, source))
    return out


def spawns(db, name):
    """How many npcs rows key to the name (none when there is no npcs)."""
    if not has_table(db, "npcs"):
        return 0
    return sum(1 for (n,) in db.execute("SELECT npc_name FROM npcs") if key(n) == key(name))


def last_written(db):
    """name key -> (colour, note) this tool last wrote, by its log."""
    if not has_table(db, LOG):
        return {}
    return {key(name): (color & 0xFFFFFFFF, note) for name, color, note in
            db.execute(f"SELECT name, color, note FROM {LOG} ORDER BY seq")}


def work(db):
    """What --apply would do: (writes, left), each write (name, the row there
    or None, colour, note) and each left (name, why). A row is this tool's to
    change only while it is what the tool last wrote there."""
    have = rows_by_key(db)
    wrote = last_written(db)
    writes, left = [], []
    for name, color, note in TINTS:
        there = have.get(key(name), [])
        if not there:
            writes.append((name, None, color, note))
            continue
        now = [((r[1] or 0) & 0xFFFFFFFF, r[2]) for r in there]
        if len(there) > 1 or there[0][3] != SOURCE or wrote.get(key(name)) != now[0]:
            left.append((name, "a row a GM set or changed: " +
                         ", ".join(f"{r[0]!r} 0x{c:08X} ({r[3]})" for r, (c, _) in zip(there, now))))
            continue
        if now[0] != (color, note):
            writes.append((name, there[0], color, note))
    return writes, left


def apply(db, writes):
    at = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    try:
        made = 0 if has_table(db, "npctint") else 1
        db.execute(SCHEMA)
        if not has_table(db, LOG):
            db.execute(LOG_DDL)
        for name, row, color, note in writes:
            if row is None:
                db.execute("INSERT INTO npctint (name, color, note, source) VALUES (?, ?, ?, ?)", (name, color, note, SOURCE))
                db.execute(f"INSERT INTO {LOG} (name, had_row, color, note, made_table, at) VALUES (?, 0, ?, ?, ?, ?)",
                           (name, color, note, made, at))
            else:
                db.execute("UPDATE npctint SET color = ?, note = ? WHERE name = ?", (color, note, row[0]))
                db.execute(f"INSERT INTO {LOG} (name, had_row, color_was, note_was, source_was, color, note, made_table, at) "
                           "VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?)", (row[0], row[1], row[2], row[3], color, note, made, at))
            made = 0
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    """Puts back what the log holds, newest first; returns (back, left)."""
    logged = db.execute(f"SELECT seq, name, had_row, color_was, note_was, source_was, color, note, made_table FROM {LOG} "
                        "ORDER BY seq DESC").fetchall()
    back, left, made = 0, [], False
    db.execute("BEGIN IMMEDIATE")
    try:
        for seq, name, had_row, color_was, note_was, source_was, color, note, made_table in logged:
            made = made or bool(made_table)
            now = db.execute("SELECT color, note, source FROM npctint WHERE name = ?", (name,)).fetchone() \
                if has_table(db, "npctint") else None
            if now is None or (now[0] or 0) & 0xFFFFFFFF != color or now[1] != note or now[2] != SOURCE:
                left.append(f"{name!r}: changed since, left as it is")
                continue
            if had_row:
                db.execute("UPDATE npctint SET color = ?, note = ?, source = ? WHERE name = ?",
                           (color_was, note_was, source_was, name))
            else:
                db.execute("DELETE FROM npctint WHERE name = ?", (name,))
            back += 1
        if made and not left and db.execute("SELECT count(*) FROM npctint").fetchone()[0] == 0:
            db.execute("DROP TABLE npctint")
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help=f"write the seed rows (logged in {LOG})")
    g.add_argument("--undo", action="store_true", help="put back everything this tool wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None)

    if a.undo:
        if not has_table(db, LOG):
            sys.exit(f"nothing of this tool's to undo here (no {LOG})")
        if not a.no_backup:
            print("copy:", backup(db, a.db))
        back, left = undo(db)
        for line in left:
            print("  left:", line)
        print(f"put back {back} rows; {len(left)} left as they are now; the GM panel's Reload every NPC (or a restart) "
              "tells the clients")
        return

    writes, left = work(db)
    print("npctint:", "there" if has_table(db, "npctint") else "not made yet (the server makes it at its start; --apply makes it here)")
    for name, color, _ in TINTS:
        print(f"  {name}: 0x{color:08X}, {spawns(db, name)} NPC row(s) of the name in npcs")
    for name, row, color, _ in writes:
        print(f"  to write: {name} 0x{color:08X}" + ("" if row is None else f" (the seed's row was 0x{(row[1] or 0) & 0xFFFFFFFF:08X})"))
    for name, why in left:
        print(f"  left: {name}: {why}")
    if not a.apply:
        print("(report only: --apply writes it)")
        return
    if not writes:
        print("nothing to write")
        return
    if not a.no_backup:
        print("copy:", backup(db, a.db))
    apply(db, writes)
    print(f"wrote {len(writes)} row(s); the GM panel's Reload every NPC (or a restart) tells the clients")


if __name__ == "__main__":
    main()
