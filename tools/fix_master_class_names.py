#!/usr/bin/env python3
"""A Master Class's first rank in cmcatalog named as the class itself.

tools/seed_masteries.py wrote every rank as EQOA-Data's lists name it, so the
Class Mastery window offered "Spellbinder 1" for the enchanter's master class
(F3 20261026-135728, jib: "should just be called Spellbinder and not
Spellbinder 1"). The seed names that rank bare now; this renames the rows a
database seeded before it: a row of category 4 (Master Class) and rank 1
whose name is exactly its mastery's and " 1" is named its mastery's. The
ranks after it keep their numbers ("Spellbinder 2"); a first rank named
anything else (a GM's rename) is left as it is.

Every change is logged in mc_name_fix_log (the id, the name before and
after); --undo puts back each name still as this left it (one renamed since
is left, and said) and drops the log. Both take a copy of the database first
(<db>.bak-<when>-pre-mcnames). Report only by default. The server reads the
catalog at its start, and again when a GM edits a cmcatalog row in the
panel: restart it after --apply or --undo (a purchase's line and the
window's list then say the new name; what a character owns is by id, and
stays).

    tools/fix_master_class_names.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import datetime
import os
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

MASTER_CLASS = 4

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS mc_name_fix_log (
    id          INTEGER PRIMARY KEY,
    name_before TEXT NOT NULL,
    name_after  TEXT NOT NULL,
    applied     TEXT NOT NULL
)"""


def plan(db):
    """The renames: [(id, name before, name after)], by id."""
    rows = db.execute("SELECT id, mastery, name FROM cmcatalog WHERE category = ? AND rank = 1 ORDER BY id",
                      (MASTER_CLASS,)).fetchall()
    out = []
    for cid, mastery, name in rows:
        bare = (mastery or "").strip()
        if bare and name == f"{bare} 1":
            out.append((cid, name, bare))
    return out


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-mcnames"
    shutil.copy2(path, dst)
    return dst


def undo(db, path):
    try:
        logged = db.execute("SELECT id, name_before, name_after FROM mc_name_fix_log ORDER BY id").fetchall()
    except sqlite3.OperationalError:
        logged = []
    if not logged:
        print("nothing logged: nothing to undo")
        return
    print(f"backup: {backup(path)}")
    db.execute("BEGIN")
    back = 0
    for cid, before, after in logged:
        now = db.execute("SELECT name FROM cmcatalog WHERE id = ?", (cid,)).fetchone()
        if now is None or now[0] != after:
            print(f"  {cid}: renamed since ({now[0] if now else 'no row'!r}): left as it is")
            continue
        db.execute("UPDATE cmcatalog SET name = ? WHERE id = ?", (before, cid))
        back += 1
    db.execute("DROP TABLE mc_name_fix_log")
    db.execute("COMMIT")
    print(f"put back {back} of {len(logged)} names; restart the server to read them")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)

    if a.undo:
        undo(db, a.db)
        return

    renames = plan(db)
    for cid, before, after in renames:
        print(f"{cid:6d} {before!r} -> {after!r}")
    print(f"{len(renames)} names to change")
    if not a.apply or not renames:
        if not a.apply:
            print("report only: --apply to write")
        return
    print(f"backup: {backup(a.db)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN")
    db.execute(LOG_SCHEMA)
    for cid, before, after in renames:
        # a run after an --apply logs nothing it did not change: a row it
        # renamed before reads bare now and is not in the plan
        db.execute("INSERT OR REPLACE INTO mc_name_fix_log (id, name_before, name_after, applied) VALUES (?, ?, ?, ?)",
                   (cid, before, after, when))
        db.execute("UPDATE cmcatalog SET name = ? WHERE id = ?", (after, cid))
    db.execute("COMMIT")
    print(f"changed {len(renames)} names; restart the server to read them")


if __name__ == "__main__":
    main()
