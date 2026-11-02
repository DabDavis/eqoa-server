#!/usr/bin/env python3
"""The low-level casters' even fights fitted again after the belts and robes went back to their slots (the user,
2026-11-02: "yes retune the low level casters").

tools/fix_belt_robe_slots.py took the robes Return Home's import had filed as belts out of the belt slot. A
caster's kit of 7-9 (entity/balancekit.go: the median item of the last ten levels a slot) had worn one of them, the
Stained Robe, as its belt beside a real robe; no belt a caster may wear is below level 10, so that slot is empty now,
and TestRealLowLevelsEvenAndBelow (every class at 1-10 wins 95% of its even fights and 97% of those one and two
down, by hand and in its own play) failed by hand for the magician at 7-10 (85-92%), the enchanter at 7-8 (85-90%),
the wizard at 6 (88%) and the alchemist at 7 and 9 (82-90%) once the wiki's items were in.

Fitted with the simulator (cmd/balancesim -try-classlevel, the roamer, the kits at 0.5) to the test's own checks at
40 fights and again at 160, the least change that meets them:
- the magician and the enchanter: dealt_pct (the share of the player's own damage that lands; not its pet's), so
  their pet play, where their yellows are fitted, does not move (one and two up the same low points);
- the wizard and the alchemist: melee_taken_pct (the share of a creature's melee they take), which by hand is the
  fight they lose and in their root play barely counts: their rooted yellows' low points move 0-3 points, where
  raising dealt_pct instead (the alchemist of 7 needed 200 -> 285) eased them up to 22.

Each row is written only while it holds the values it had (a GM's change is left, and said). Logged in
balance_low_casters_log (the row's key, the whole row before and after); --undo puts back each row still as this
left it and drops the log. Both copy the database first (<db>.bak-<when>-pre-lowcasters) unless --no-backup.
Report only by default. The server reads the balance tables at its start and at once at a save of any Balance page
in the GM panel. internal/database/balanceseed.go holds the same rows (a table made new).

    tools/refit_low_casters.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import refit_eqoaemu_balance as rf  # noqa: E402  the row helpers
import retune_balance as retune  # noqa: E402  balanceclasslevel's order

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
CL = rf.CL
LOG = "balance_low_casters_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balanceclasslevel
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON
    after   TEXT,            -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
WHY = ("the low-level casters' even fights fitted again once the belt slot's misfiled robes were out (tools/refit_low_casters.py, "
       "2026-11-02)")

# (class, level): ((melee_taken_pct, pet_hp_pct, dealt_pct) before, after)
CHANGES = {
    (10, 7): ((32, 126, 100), (32, 126, 118)),
    (10, 8): ((29, 84, 100), (29, 84, 112)),
    (10, 9): ((39, 100, 100), (39, 100, 106)),
    (10, 10): ((20, 119, 100), (20, 119, 106)),
    (12, 7): ((29, 734, 100), (29, 734, 118)),
    (12, 8): ((24, 71, 100), (24, 71, 118)),
    (13, 6): ((67, 100, 130), (60, 100, 130)),
    (14, 7): ((62, 100, 200), (56, 100, 200)),
    (14, 9): ((67, 100, 175), (60, 100, 175)),
}


def plan(db):
    """The changes [(table, key, row before, row after)] and the rows left, with why."""
    changes, kept = [], []
    if not rf.has_table(db, CL) or "dealt_pct" not in rf.columns(db, CL):
        return changes, [(CL, "no such table or no dealt_pct")]
    for (c, l), (old, new) in sorted(CHANGES.items()):
        r = rf.row_of(db, CL, {"class": c, "level": l})
        have = rf.cl_values(r)
        if rf.same(have, tuple(map(float, new))):
            continue  # done already
        if not rf.same(have, tuple(map(float, old))):
            kept.append((f"{CL} {c} {l}", f"{have if have else 'no row'}, not {old}: a GM's"))
            continue
        after = dict(r)
        after.update({"melee_taken_pct": float(new[0]), "pet_hp_pct": float(new[1]), "dealt_pct": float(new[2])})
        changes.append((CL, {"class": c, "level": l}, r, after))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-lowcasters", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-lowcasters"
    shutil.copy2(path, dst)
    return dst


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), WHY, when))
            rf.write_row(db, tbl, key, after, insert=False)
        retune.reorder(db)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not rf.has_table(db, LOG):
        print("nothing logged: nothing to undo")
        return
    logged = db.execute(f"SELECT seq, tbl, place, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    back = 0
    db.execute("BEGIN IMMEDIATE")
    try:
        for _seq, tbl, place, before, after in logged:
            if tbl != CL:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            if rf.row_of(db, tbl, key) != json.loads(after):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            rf.write_row(db, tbl, key, json.loads(before), insert=False)
            back += 1
        retune.reorder(db)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start or a Balance page's next save")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
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
        if not a.no_backup and rf.has_table(db, LOG):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    changes, kept = plan(db)
    for c in changes:
        print(rf.describe(c))
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
