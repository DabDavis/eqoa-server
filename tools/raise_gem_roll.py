#!/usr/bin/env python3
"""The jewelers' gems rolled on 5% of every creature's corpses, not 3% (the
user, 2026-10-30, of tools/drop_rates.py's report: the four rare gems of level
58 took 13,123 kills for a one-in-two chance). The roll is data: npcloot's row
binding "*" (every creature, handlers/loot.go) to the table "craft gems",
written by tools/seed_tradeskills.py, whose JEWEL_LOOT_CHANCE a new seed
writes as 5. The modifiers' roll ("craft modifiers", 3%) is left.

Measured by tools/drop_rates.py on the live data of 2026-10-30: the gems'
median from 0.118% a kill (586 kills for a one-in-two chance) to 0.197% (352);
the rare level 58 gems from 0.005% (13,123) to 0.008% (7,874).

The change is logged in gem_roll_log; --undo puts the roll back while it is
still as this left it. Report only by default. The server reads npcloot at
every corpse: no restart.

    python3 -E -s tools/raise_gem_roll.py --db eqoa.db [--chance 5] [--apply | --undo]
"""
import argparse
import datetime
import os
import shutil
import sqlite3
import sys

EVERY, TABLE, CHANCE = "*", "craft gems", 5

LOG_DDL = """CREATE TABLE IF NOT EXISTS gem_roll_log (
    npc_name  TEXT NOT NULL,
    tablename TEXT NOT NULL,
    before    INTEGER NOT NULL,
    after     INTEGER NOT NULL,
    applied   TEXT NOT NULL,
    PRIMARY KEY (npc_name, tablename)
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--chance", type=int, default=CHANCE, help=f"the gems' roll, percent (default {CHANCE})")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="raise the roll (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    if not 1 <= a.chance <= 100:
        sys.exit("--chance is 1 to 100")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "gem_roll_log"):
            print("nothing logged: nothing to undo")
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        db.execute("BEGIN IMMEDIATE")
        try:
            row = db.execute("SELECT before, after FROM gem_roll_log WHERE npc_name = ? AND tablename = ?", (EVERY, TABLE)).fetchone()
            cur = db.execute("SELECT chance FROM npcloot WHERE npc_name = ? AND tablename = ?", (EVERY, TABLE)).fetchone()
            if row and cur and cur[0] == row[1]:
                db.execute("UPDATE npcloot SET chance = ? WHERE npc_name = ? AND tablename = ?", (row[0], EVERY, TABLE))
                print(f"the gems' roll back to {row[0]}%")
            else:
                print(f"the gems' roll is {cur[0] if cur else 'gone'}, changed since: left as it is")
            db.execute("DROP TABLE gem_roll_log")
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        return
    if a.apply and has_table(db, "gem_roll_log"):
        sys.exit("gem_roll_log is there: applied already (--undo first)")
    cur = db.execute("SELECT chance FROM npcloot WHERE npc_name = ? AND tablename = ?", (EVERY, TABLE)).fetchone()
    if cur is None:
        sys.exit(f"no npcloot row binds {EVERY!r} to {TABLE!r} (tools/seed_tradeskills.py --craft jewel writes it)")
    print(f"the gems' roll: {cur[0]}% -> {a.chance}%; the modifiers' left at " +
          ", ".join(f"{c}%" for (c,) in db.execute("SELECT chance FROM npcloot WHERE npc_name = ? AND tablename = 'craft modifiers'", (EVERY,))))
    if cur[0] == a.chance:
        print("nothing to change")
        return
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        db.execute("UPDATE npcloot SET chance = ? WHERE npc_name = ? AND tablename = ?", (a.chance, EVERY, TABLE))
        db.execute("INSERT INTO gem_roll_log VALUES (?, ?, ?, ?, ?)", (EVERY, TABLE, cur[0], a.chance, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"the gems' roll is {a.chance}%")


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-gemroll"
    shutil.copy2(path, dst)
    return dst


if __name__ == "__main__":
    main()
