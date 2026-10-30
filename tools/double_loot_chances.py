#!/usr/bin/env python3
"""Every creature's loot tables rolled twice as often (the user, 2026-10-30,
of tools/drop_rates.py's report: a generic table's item took 841 kills for a
one-in-two chance): each npcloot row bound to a creature's name has its
chance doubled, at most 100. The tables every creature rolls (npcloot's "*":
the tradeskills' gems and modifiers) are not a creature's and are left
(tools/raise_gem_roll.py raises the gems'); a row at 100 already stays.

Measured by tools/drop_rates.py on the live data of 2026-10-30: the generic
table's median item from 0.082% a kill (841 kills for a one-in-two chance) to
0.165% (420); a biome's from 0.24-0.48% (144-291) to 0.48-0.96% (72-145).

Every row changed is logged in loot_chance_double_log (its chance before and
after); --undo puts back each one still as this left it. Run it after
tools/seed_region_loot.py to double that tool's rows too, and undo in the
other order. Report only by default. The server reads npcloot at every
corpse: no restart.

    python3 -E -s tools/double_loot_chances.py --db eqoa.db [--apply | --undo]
"""
import argparse
import collections
import datetime
import os
import shutil
import sqlite3
import sys

EVERY = "*"  # handlers/loot.go lootEveryCreature: no creature's own
CAP = 100

LOG_DDL = """CREATE TABLE IF NOT EXISTS loot_chance_double_log (
    npc_name  TEXT NOT NULL,
    tablename TEXT NOT NULL,
    before    INTEGER NOT NULL,
    after     INTEGER NOT NULL,
    applied   TEXT NOT NULL,
    PRIMARY KEY (npc_name, tablename)
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def plan(db):
    """[(name, table, chance before, after)] for every creature's row a
    doubling changes."""
    out = []
    for name, table, chance in db.execute("SELECT npc_name, tablename, chance FROM npcloot ORDER BY npc_name, tablename"):
        if name == EVERY or chance is None:
            continue
        new = min(CAP, chance * 2)
        if new != chance:
            out.append((name, table, chance, new))
    return out


def apply(db, rows):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for name, table, old, new in rows:
            db.execute("UPDATE npcloot SET chance = ? WHERE npc_name = ? AND tablename = ? AND chance = ?", (new, name, table, old))
            db.execute("INSERT INTO loot_chance_double_log VALUES (?, ?, ?, ?, ?)", (name, table, old, new, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "loot_chance_double_log"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back, kept = 0, []
        for name, table, old, new in db.execute("SELECT npc_name, tablename, before, after FROM loot_chance_double_log").fetchall():
            cur = db.execute("SELECT chance FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table)).fetchone()
            if cur is not None and cur[0] == new:
                db.execute("UPDATE npcloot SET chance = ? WHERE npc_name = ? AND tablename = ?", (old, name, table))
                back += 1
            elif cur is not None:
                kept.append(f"{name!r} on {table!r}")
        db.execute("DROP TABLE loot_chance_double_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} chances" + (f"; {len(kept)} changed since left as they are: " + ", ".join(kept[:20]) if kept else ""))


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-lootdouble"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="double the chances (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "loot_chance_double_log"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "loot_chance_double_log"):
        sys.exit("loot_chance_double_log is there: applied already (--undo first)")
    rows = plan(db)
    by = collections.Counter(f"{t} {o}% -> {n}%" for _, t, o, n in rows)
    print(f"creature loot rows to double: {len(rows)} ({len({n for n, *_ in rows})} names): " +
          ", ".join(f"{k} x{v}" for k, v in sorted(by.items())))
    capped = db.execute("SELECT count(*) FROM npcloot WHERE npc_name <> ? AND chance >= ?", (EVERY, CAP)).fetchone()[0]
    print(f"rows at {CAP}% already, left: {capped}; the tables of \"*\" left: " +
          ", ".join(f"{t} {c}%" for t, c in db.execute("SELECT tablename, chance FROM npcloot WHERE npc_name = ?", (EVERY,))))
    if not a.apply:
        print("report only: --apply to write")
        return
    if not rows:
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, rows)
    print(f"doubled {len(rows)} rows")


if __name__ == "__main__":
    main()
