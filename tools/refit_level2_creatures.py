#!/usr/bin/env python3
"""A level 2 creature's melee brought into line with levels 1 and 3, and (asked for) the healers' and casters' 1-12
shares of a creature's melee eased back toward the new players' fit (petition 5287, Live, 2026-11-17: "Level 4 Shaman
too weak, can barely handle a level 2 rat").

balancelevel 2's damage_pct 133 -> 79 (default). Since the new players' curve (tools/refit_new_players.py, 2026-11-04)
a creature of 2 hits at 133% while one of 1 hits at 79% and one of 3 at 61%. The curve fitted each level's figure to the
median class's yellows, and at creature level 2 that median was the same anywhere from 0.25 to 0.45 of the old 317
(79-143): the tanks and melee classes, seven of the fifteen, are hit at the creature's floor of 1 whatever the share.
133 was the top of that step. The healers and casters are not on the floor: an NPC's roll on them carries the
archetype's offset (defence.go: +23.4 a priest, +31.7 a caster, measured at levels 45-55), which against a level 2
creature's top of 1.6 is nearly the whole hit, so for them the share is the hit: a level 2 rat hit a shaman of 4 for
9-11 a blow (11-15% of 82 hit points) and a caster of 4 for 27-31 (41-47%), against the warrior's 1. 79, the bottom of
the same step (0.25 of 317, level 1's figure), keeps the median the fit aimed at (TestRealNewPlayersCurve) and takes a
level 2 creature's hit on them to 5.7-6.4 and 16-19.

--low-shares (the owner's and a GM's call, not the default): balanceclasslevel melee_taken_pct of the druid, shaman,
cleric, magician, necromancer, enchanter, wizard and alchemist at levels 1-12, each the LESSER of its archetype share
(95 a priest, 115 a caster: a GM's panel edit of 2026-11-13 04:40 UTC on Heirloom, synced to Live 2026-11-15) and its
new players' fit of 2026-11-04 (the row before that edit, gm_audit): a priest of 1-3 took 16-63%, a caster of 1-10
15-40%, and since the edit 95% and 115%, under which a caster of 1-10 by hand loses most of its even fights
(TestRealLowLevelsEvenAndBelow). Never more than the archetype share, so nothing is made harder; 13-60 untouched.

Each row is written only while it holds the value it had (a GM's later change is left, and said). Logged in
balance_level2_log (the row's key, the whole row before and after); --undo puts back each row still as this left it
and drops the log. Both copy the database first (<db>.bak-<when>-pre-level2) unless --no-backup. Report only by
default. The server reads the balance tables at its start and at once at a save of any Balance page in the GM panel.

    tools/refit_level2_creatures.py [--db eqoa.db] [--low-shares] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from sqlite_backup import backup_database

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LV, CL = "balancelevel", "balanceclasslevel"
LOG = "balance_level2_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balancelevel or balanceclasslevel
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON
    after   TEXT,            -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
WHY = "petition 5287: a level 2 creature's melee in line with 1 and 3 (tools/refit_level2_creatures.py, 2026-11-17)"
WHY_SHARES = ("petition 5287: the healers' and casters' 1-12 shares the lesser of the archetype's and the new players' "
              "fit (tools/refit_level2_creatures.py --low-shares, 2026-11-17)")

# the creature's level: damage_pct (before, after)
LEVELS = {2: (133.0, 79.0)}

# the archetype share each class holds since 2026-11-13 (the value a row must still have)
FLAT = {7: 95.0, 8: 95.0, 9: 95.0, 10: 115.0, 11: 115.0, 12: 115.0, 13: 115.0, 14: 115.0}
# the new players' fit of 2026-11-04 at 1-12 (gm_audit's rows before 2026-11-13; a level not here was at the flat)
FIT = {
    7: {1: 16, 2: 32, 3: 45, 4: 55, 5: 59, 6: 63, 7: 78, 8: 57, 9: 34, 10: 39, 11: 48, 12: 89},
    8: {1: 17, 2: 31, 3: 63, 4: 78, 6: 117, 7: 143, 8: 143, 11: 99, 12: 114},
    9: {1: 17, 2: 27, 3: 38, 4: 138, 5: 143, 6: 117, 7: 176, 8: 117, 9: 78, 10: 63, 11: 79, 12: 129},
    10: {1: 15, 2: 17, 3: 16, 4: 19, 5: 18, 6: 26, 7: 32, 8: 29, 9: 39, 10: 20, 11: 100, 12: 100},
    11: {1: 15, 2: 15, 3: 22, 4: 17, 5: 18, 6: 28, 7: 32, 8: 32, 9: 40, 10: 17, 11: 100, 12: 100},
    12: {1: 15, 2: 15, 3: 16, 4: 22, 5: 17, 6: 26, 7: 29, 8: 24, 9: 26, 10: 16, 11: 100, 12: 100},
    13: {1: 19, 2: 23, 3: 35, 4: 31, 5: 44, 6: 60, 7: 26, 8: 72, 9: 83, 10: 26, 11: 25, 12: 22},
    14: {1: 17, 2: 15, 3: 16, 4: 23, 5: 25, 6: 35, 7: 56, 8: 18, 9: 60, 10: 15, 11: 20, 12: 28},
}


def shares():
    """(class, level): (melee_taken_pct before, after), only where the fit is under the flat."""
    out = {}
    for c, levels in FIT.items():
        for l, v in levels.items():
            if v < FLAT[c]:
                out[(c, l)] = (FLAT[c], float(v))
    return out


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def close(a, b):
    return a is not None and math.isclose(float(a), float(b), rel_tol=1e-9, abs_tol=1e-6)


def plan(db, low_shares):
    """The changes [(table, key, row before, row after, why)] and the rows left, with why."""
    changes, kept = [], []
    for tbl in (LV, CL):
        if not has_table(db, tbl):
            return [], [(tbl, "no such table")]
    for l, (old, new) in sorted(LEVELS.items()):
        r = row_of(db, LV, {"level": l})
        have = None if r is None else r["damage_pct"]
        if close(have, new):
            continue
        if not close(have, old):
            kept.append((f"{LV} {l}", f"damage_pct {have if r else 'no row'}, not {old:g}: a GM's"))
            continue
        after = dict(r)
        after["damage_pct"] = new
        changes.append((LV, {"level": l}, r, after, WHY))
    if low_shares:
        for (c, l), (old, new) in sorted(shares().items()):
            r = row_of(db, CL, {"class": c, "level": l})
            have = None if r is None else r["melee_taken_pct"]
            if close(have, new):
                continue
            if not close(have, old):
                kept.append((f"{CL} {c} {l}", f"melee_taken_pct {have if r else 'no row'}, not {old:g}: a GM's"))
                continue
            after = dict(r)
            after["melee_taken_pct"] = new
            changes.append((CL, {"class": c, "level": l}, r, after, WHY_SHARES))
    return changes, kept


def describe(change):
    tbl, key, before, after, _why = change
    col = "damage_pct" if tbl == LV else "melee_taken_pct"
    return f"{tbl} {' '.join(str(v) for v in key.values())}: {col} {float(before[col]):g} -> {float(after[col]):g}"


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-level2", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-level2"
    return backup_database(path, dst)


def write_row(db, tbl, key, row):
    where = " AND ".join(f"{k} = ?" for k in key)
    cols = [c for c in row if c not in key]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(key.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after, why in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), why, when))
            write_row(db, tbl, key, after)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute(f"SELECT seq, tbl, place, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for _seq, tbl, place, before, after in logged:
            if tbl not in (LV, CL):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            if row_of(db, tbl, key) != json.loads(after):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            write_row(db, tbl, key, json.loads(before))
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start or a Balance page's next save")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--low-shares", action="store_true",
                    help="also the healers' and casters' 1-12 shares (the owner's and Jibrish's call)")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the rows (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, LOG):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    changes, kept = plan(db, a.low_shares)
    for c in changes:
        print(describe(c))
    for what, why in kept:
        print(f"left as it is: {what} ({why})")
    print(f"{len(changes)} rows to change, {len(kept)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, changes)
    print(f"changed {len(changes)} rows: the server takes them at its next start, or at once at the next save of a "
          "Balance page in the GM panel")


if __name__ == "__main__":
    main()
