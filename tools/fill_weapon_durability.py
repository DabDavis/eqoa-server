#!/usr/bin/env python3
"""Hit points and durability for the weapons that have none (the user, 2026-10-30: "yes fill in
the 729 missing records", after "the weapons you added are HP 0/0, DUR 0").

The old dump's blank weapon records (claude-notes/dump-weapons-20261029.md) carry maxhp 0 and
duration 0, which the item window shows as "HP 0/0, DUR 0" (a weapon of maxhp 0 already fights as
whole: WeaponCond 1). The weapons that have them follow one rule, measured over every weapon record
here with both: maxhp is 200 times the level (1: 200, 4: 800, 10: 2000, 20: 4000, 30: 6000, 37:
7400), duration 50 nearly always, level 5 the one level whose records say otherwise (800, six of
them). So a blank weapon is given its level's most common maxhp where at least three weapons of that
level carry one, else 200 times its level (at least 200), and duration 50; a held copy at 0 hit
points is made whole. Every change is logged in weapon_durability_fix_log; --undo puts back each row
still as this left it. Report only by default. The server reads item records as it goes: no restart
(a player sees new numbers at the next login).

    tools/fill_weapon_durability.py [--db eqoa.db] [--apply | --undo]
"""

import argparse
import collections
import sqlite3

WEAPON_SLOTS = (12, 13, 14, 15, 16)
DURATION = 50
MIN_SAMPLES = 3

LOG_DDL = """CREATE TABLE IF NOT EXISTS weapon_durability_fix_log (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,
    rid INTEGER NOT NULL,
    old_maxhp INTEGER,
    old_duration INTEGER,
    new_maxhp INTEGER,
    new_duration INTEGER
)"""


def level_hp(db):
    """Each level's most common maxhp among the weapons that have one, where at least
    MIN_SAMPLES do."""
    seen = collections.defaultdict(collections.Counter)
    slots = ",".join(map(str, WEAPON_SLOTS))
    for lv, hp in db.execute(f"SELECT levelreq, maxhp FROM itempattern WHERE equipslot IN ({slots}) "
                             "AND weapondamage > 0 AND maxhp > 0"):
        seen[lv][hp] += 1
    out = {}
    for lv, c in seen.items():
        hp, n = c.most_common(1)[0]
        if n >= MIN_SAMPLES:
            out[lv] = hp
    return out


def hp_for(level, table):
    return table.get(level, 200 * max(level or 0, 1))


def plan(db):
    table = level_hp(db)
    slots = ",".join(map(str, WEAPON_SLOTS))
    rows = db.execute(f"SELECT patternid, levelreq FROM itempattern WHERE equipslot IN ({slots}) "
                      "AND weapondamage > 0 AND maxhp = 0 AND duration = 0 ORDER BY patternid").fetchall()
    return [(pid, lv, hp_for(lv, table)) for pid, lv in rows], table


def apply(db, todo):
    db.execute(LOG_DDL)
    records = held = 0
    for pid, _, hp in todo:
        cur = db.execute("UPDATE itempattern SET maxhp = ?, duration = ? WHERE patternid = ? AND maxhp = 0 AND duration = 0",
                         (hp, DURATION, pid))
        if cur.rowcount:
            db.execute("INSERT INTO weapon_durability_fix_log (tbl, rid, old_maxhp, old_duration, new_maxhp, new_duration) "
                       "VALUES ('itempattern', ?, 0, 0, ?, ?)", (pid, hp, DURATION))
            records += 1
        for (itemid,) in db.execute("SELECT itemid FROM charinventory WHERE patternid = ? AND remainHP = 0", (pid,)).fetchall():
            db.execute("UPDATE charinventory SET remainHP = ? WHERE itemid = ?", (hp, itemid))
            db.execute("INSERT INTO weapon_durability_fix_log (tbl, rid, old_maxhp, new_maxhp) "
                       "VALUES ('charinventory', ?, 0, ?)", (itemid, hp))
            held += 1
    return records, held


def undo(db):
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'weapon_durability_fix_log'").fetchone()[0] == 0:
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for tbl, rid, new_hp, new_du in db.execute(
            "SELECT tbl, rid, new_maxhp, new_duration FROM weapon_durability_fix_log ORDER BY seq DESC").fetchall():
        if tbl == "itempattern":
            cur = db.execute("UPDATE itempattern SET maxhp = 0, duration = 0 WHERE patternid = ? AND maxhp = ? AND duration = ?",
                             (rid, new_hp, new_du))
        else:
            cur = db.execute("UPDATE charinventory SET remainHP = 0 WHERE itemid = ? AND remainHP = ?", (rid, new_hp))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE weapon_durability_fix_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    todo, table = plan(db)
    by_hp = collections.Counter(hp for _, _, hp in todo)
    print(f"{len(todo)} weapons with no hit points and no durability; the levels' own values: "
          + ", ".join(f"{lv}: {hp}" for lv, hp in sorted(table.items())))
    print("to be given: " + ", ".join(f"{hp} x{n}" for hp, n in sorted(by_hp.items())))
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    records, held = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {records} records, {held} held copies made whole")


if __name__ == "__main__":
    main()
