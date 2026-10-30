#!/usr/bin/env python3
"""The legendary items held out of the pooled loot tables (weight 0), so no
creature drops them at a pooled table's rate (2026-10-30: jib's "aura of
karana, orb of primordial were so rare only 5 on server after 10 years";
tools/loot_tiers.py names the legendary items).

On the live data of 2026-10-30 01:30 UTC three sit in the camp seed's tables:
Aura of Karana in plains (levels 21-31), Grimblade and Mithril Flamberge in
generic. tools/seed_region_loot.py binds the plains table to creatures of Aura
of Karana's levels: 1 in 168 kills at its best creature (a skeletal guard),
about one every four hours of farming it, 1 in 84 with
tools/double_loot_chances.py; the generic table gives the other two 1 in
1,240 (1 in 620 doubled). Five in ten years on this server is about 1 in 4,700
a kill for Aura of Karana's creatures (tools/seed_creature_loot.py).

Held, they drop nowhere until tools/seed_creature_loot.py (or a GM) gives them
their own rate; a boss's table is left. Every row held is logged in
legendary_hold_log; --undo puts back each weight still 0. Report only by
default. The server reads loottables at every corpse: no restart.

    python3 -E -s tools/hold_legendary_loot.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import os
import shutil
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import loot_tiers as lt  # noqa: E402

LOG_DDL = """CREATE TABLE IF NOT EXISTS legendary_hold_log (
    lootrow   INTEGER PRIMARY KEY,
    tablename TEXT NOT NULL,
    patternid INTEGER NOT NULL,
    before    INTEGER NOT NULL,
    applied   TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def plan(db):
    """[(loottables id, table, patternid, weight, item)] for every legendary row to hold."""
    leg = {pid for pid, (t, _) in lt.tiers(db).items() if t == lt.LEGENDARY}
    out = []
    for rid, t, pid, w, name in db.execute(
            "SELECT l.id, l.tablename, l.patternid, l.weight, i.itemname FROM loottables l "
            "JOIN itempattern i ON i.patternid = l.patternid WHERE l.weight > 0 ORDER BY l.id"):
        if pid in leg and not t.startswith("boss "):
            out.append((rid, t, pid, w, name))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="hold the rows (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply held")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "legendary_hold_log"):
            print("nothing logged: nothing to undo")
            return
        if not a.no_backup:
            print(f"copy: {backup(a.db)}")
        db.execute("BEGIN IMMEDIATE")
        try:
            back, kept = 0, []
            for rid, w in db.execute("SELECT lootrow, before FROM legendary_hold_log").fetchall():
                cur = db.execute("SELECT weight FROM loottables WHERE id = ?", (rid,)).fetchone()
                if cur is not None and cur[0] == 0:
                    db.execute("UPDATE loottables SET weight = ? WHERE id = ?", (w, rid))
                    back += 1
                elif cur is not None:
                    kept.append(str(rid))
            db.execute("DROP TABLE legendary_hold_log")
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"put back {back} rows" + (f"; {len(kept)} changed since left as they are: loottables " + ", ".join(kept) if kept else ""))
        return
    if a.apply and has_table(db, "legendary_hold_log"):
        sys.exit("legendary_hold_log is there: applied already (--undo first)")
    rows = plan(db)
    for rid, t, pid, w, name in rows:
        print(f"hold: {name} ({pid}) in {t!r} (loottables {rid}, weight {w})")
    print(f"{len(rows)} legendary rows to hold")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not rows:
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for rid, t, pid, w, name in rows:
            db.execute("UPDATE loottables SET weight = 0 WHERE id = ?", (rid,))
            db.execute("INSERT INTO legendary_hold_log VALUES (?, ?, ?, ?, ?)", (rid, t, pid, w, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"held {len(rows)} rows")


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-legendaryhold"
    shutil.copy2(path, dst)
    return dst


if __name__ == "__main__":
    main()
