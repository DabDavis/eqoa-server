#!/usr/bin/env python3
"""Armor and shields whose armor class sits in another stat: the number moved
to the armor class (the user, 2026-10-30: "Yes, move it to AC").

EQOA-Data's records give hundreds of armor pieces and shields their armor
class under another stat's name, and the dump carried them into itempattern
that way: Tundra Tear, a level 32 shield, has "agi": 289 and no armor class,
so in the game it gives 289 agility and no armor. The stat varies (dexterity,
agility, intelligence, stamina, strength, wisdom), so the number is found by
what it is, against the records of the same slot and level that have their
armor class (PEERS: the same wear slot within LEVEL_SPAN levels, widened to
WIDE_SPAN when fewer than MIN_PEERS):

  - **moved**: a record with no armor class, one of whose attributes or
    resists is in its peers' armor class range (a quarter to twice their
    median: cloth to plate) and far above what an attribute of theirs holds
    (above 1.5 times their attributes' 95th percentile, and above its level +
    ATTR_OVER). Exactly one such stat: its number becomes the armor class and
    the stat 0. Of several, the largest moves when it is twice the next and
    every other is below half the peers' median (an armor class beside an
    attribute-sized number).
  - **copied**: a record that has its armor class and the same number in an
    attribute or resist as well, far above an attribute (Danlon's Iron Wall,
    armor class 343 and agility 343): the stat 0.
  - **undecided**, listed and left: two or more stats that could be the armor
    class, a robe's number (half of Sandstorm's robes have no armor class, and
    a robe's power is the size of its armor class), or a number above an
    attribute's but outside the armor class range.

Measured on the live data of 2026-10-30 02:05 UTC: 320 moved, 15 copies taken
out, 68 undecided. Against Sandstorm's own records (the harvest of
2026-10-21): every one of the 7 moved items it has carries exactly the number
moved as its armor class. The same records show the rest of these records'
stats mislabeled as well (the values are in the game's stat order, labeled
strength, stamina, agility, ... from the first: Incandescent Shield's strength
25, stamina 100 and dexterity 22 are Sandstorm's wisdom 25, power 100 and fire
resist 22); this moves the armor class alone.

Every change is logged in armor_class_fix_log (the item, the stat, its number,
the armor class before); --undo puts back each item still as this left it.
Report only by default. The server reads itempattern when an item is made or
loaded, so a restart is needed for items already in the world to read the new
armor class (characters' worn items are loaded at login).

    python3 -E -s tools/fix_armor_class.py --db eqoa.db [--list] [--apply | --undo]
"""
import argparse
import collections
import datetime
import os
import shutil
import sqlite3
import sys

SLOTS = {1: "helm", 2: "robe", 5: "chest", 7: "bracers", 10: "legs", 11: "boots", 19: "gloves", 13: "shield"}
STATS = ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "FR", "LR", "CR", "PR", "DR", "AR")
ATTRS = STATS[:7]
LEVEL_SPAN, WIDE_SPAN, MIN_PEERS = 3, 6, 5
ATTR_OVER = 10        # an attribute above its item's level + this is no attribute an item of that level holds
LOW, HIGH = 0.25, 2.0  # the armor class range: LOW to HIGH times the peers' median armor class (cloth to plate)
AC_SIZED = 0.5         # of several numbers in the range, the largest is the armor class when it is twice the next
                       # and every other is below this times the peers' median
ROBE = 2               # half of Sandstorm's robes have no armor class: a robe's number is never taken for one
ATTR_MARGIN = 1.5     # and above this times their attributes' 95th percentile

LOG_DDL = """CREATE TABLE IF NOT EXISTS armor_class_fix_log (
    patternid INTEGER PRIMARY KEY,
    itemname  TEXT NOT NULL,
    kind      TEXT NOT NULL,     -- moved | copied
    field     TEXT NOT NULL,     -- the stat the number sat in
    number    INTEGER NOT NULL,  -- the number
    ac_before INTEGER,           -- the armor class before, as it was (NULL kept)
    applied   TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def pct(vals, f):
    vals = sorted(vals)
    return vals[int(f * (len(vals) - 1))] if vals else 0


def plan(db):
    """(moved, copied, undecided): [(patternid, name, level, slot, field, number, ac before, why)]."""
    rows = db.execute(f"SELECT patternid, itemname, levelreq, equipslot, AC, {', '.join(STATS)} FROM itempattern "
                      f"WHERE equipslot IN ({', '.join(str(s) for s in SLOTS)}) AND levelreq > 0").fetchall()
    items = [dict(zip(("pid", "name", "lv", "slot", "AC") + STATS, r)) for r in rows]
    for it in items:
        for k in ("AC",) + STATS:
            it[k] = it[k] or 0

    def copy_of(it):
        """The stats that hold the armor class's own number, far above an attribute."""
        return [k for k in STATS if it["AC"] > it["lv"] + ATTR_OVER and it[k] == it["AC"]]
    right = collections.defaultdict(list)  # slot -> records with their armor class (and no copy of it)
    for it in items:
        if it["AC"] > 0 and not copy_of(it):
            right[it["slot"]].append(it)

    def peers(it):
        for span in (LEVEL_SPAN, WIDE_SPAN):
            ps = [p for p in right[it["slot"]] if abs(p["lv"] - it["lv"]) <= span]
            if len(ps) >= MIN_PEERS:
                return ps
        return ps
    moved, copied, undecided = [], [], []
    for it in sorted(items, key=lambda i: i["pid"]):
        base = (it["pid"], it["name"], it["lv"], SLOTS[it["slot"]])
        ps = peers(it)
        if not ps:
            continue
        acs = [p["AC"] for p in ps]
        lo, hi = LOW * pct(acs, 0.5), HIGH * pct(acs, 0.5)
        ceiling = max(ATTR_MARGIN * pct([p[k] for p in ps for k in ATTRS], 0.95), it["lv"] + ATTR_OVER)
        if it["AC"] > 0:
            for k in copy_of(it):
                if it[k] > ceiling:
                    copied.append(base + (k, it[k], it["AC"], f"the armor class {it['AC']} again in {k}"))
            continue
        over = [k for k in STATS if it[k] > ceiling]
        fits = sorted((k for k in over if lo <= it[k] <= hi), key=lambda k: -it[k])
        med = pct(acs, 0.5)
        if fits and it["slot"] == ROBE:
            undecided.append(base + ("/".join(fits), it[fits[0]], 0,
                                     "a robe (half of Sandstorm's robes have no armor class): " +
                                     ", ".join(f"{k} {it[k]}" for k in fits) + " may be power or health"))
        elif len(fits) == 1:
            k = fits[0]
            moved.append(base + (k, it[k], 0, f"{k} {it[k]}: in its peers' armor class {lo:.0f}-{hi:.0f}, "
                                               f"an attribute of theirs at most {ceiling:.0f}"))
        elif len(fits) > 1 and it[fits[0]] >= 2 * it[fits[1]] and all(it[k] < AC_SIZED * med for k in fits[1:]):
            k = fits[0]
            moved.append(base + (k, it[k], 0, f"{k} {it[k]}: the largest of " +
                                 ", ".join(f"{x} {it[x]}" for x in fits) + f" in its peers' armor class "
                                 f"{lo:.0f}-{hi:.0f}, the others below half their median"))
        elif len(fits) > 1:
            undecided.append(base + ("/".join(fits), it[fits[0]], 0,
                                     "several could be the armor class: " + ", ".join(f"{k} {it[k]}" for k in fits) +
                                     f" (peers' armor class {lo:.0f}-{hi:.0f}, attributes at most {ceiling:.0f})"))
        elif over:
            undecided.append(base + ("/".join(over), max(it[k] for k in over), 0,
                                     "above an attribute but outside the armor class range: " +
                                     ", ".join(f"{k} {it[k]}" for k in over) + f" (peers' armor class {lo:.0f}-{hi:.0f})"))
    return moved, copied, undecided


def apply(db, moved, copied):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for kind, rows in (("moved", moved), ("copied", copied)):
            for pid, name, lv, slot, k, n, ac, why in rows:
                (raw_ac,) = db.execute("SELECT AC FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
                if kind == "moved":
                    db.execute(f"UPDATE itempattern SET AC = ?, {k} = 0 WHERE patternid = ?", (n, pid))
                else:
                    db.execute(f"UPDATE itempattern SET {k} = 0 WHERE patternid = ?", (pid,))
                db.execute("INSERT INTO armor_class_fix_log VALUES (?, ?, ?, ?, ?, ?, ?)", (pid, name, kind, k, n, raw_ac, when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "armor_class_fix_log"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back, kept = 0, []
        for pid, kind, k, n, ac in db.execute(
                "SELECT patternid, kind, field, number, ac_before FROM armor_class_fix_log").fetchall():
            if k not in STATS:
                continue
            cur = db.execute(f"SELECT AC, {k} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            want_ac = n if kind == "moved" else (ac or 0)
            if cur is not None and (cur[0] or 0) == want_ac and (cur[1] or 0) == 0:
                db.execute(f"UPDATE itempattern SET AC = ?, {k} = ? WHERE patternid = ?", (ac, n, pid))
                back += 1
            elif cur is not None:
                kept.append(str(pid))
        db.execute("DROP TABLE armor_class_fix_log")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} items" + (f"; {len(kept)} changed since left as they are: " + ", ".join(kept[:20]) if kept else ""))


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-armorclass"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--list", action="store_true", help="every item moved and copied too, not only the undecided")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="move the numbers (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "armor_class_fix_log"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "armor_class_fix_log"):
        sys.exit("armor_class_fix_log is there: applied already (--undo first)")
    moved, copied, undecided = plan(db)
    by = lambda rows: ", ".join(f"{k} {v}" for k, v in collections.Counter(r[4] for r in rows).most_common())
    print(f"moved to the armor class: {len(moved)} ({by(moved)}); by slot: " +
          ", ".join(f"{k} {v}" for k, v in collections.Counter(r[3] for r in moved).most_common()))
    print(f"a copy of the armor class taken out of a stat: {len(copied)} ({by(copied)})")
    print(f"undecided, left: {len(undecided)}")
    for pid, name, lv, slot, k, n, ac, why in (moved + copied if a.list else []) + undecided:
        print(f"  {pid} {name!r} L{lv} {slot}: {why}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not moved and not copied:
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, moved, copied)
    print(f"moved {len(moved)}, copies taken out {len(copied)}")


if __name__ == "__main__":
    main()
