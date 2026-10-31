#!/usr/bin/env python3
"""Hit points and durability for the worn items that have none: armour, jewellery, shields, held items and
gloves (the user, 2026-10-31: "i made a new delf necro pledging stone has 0/0 HP and 0 DUR"; the weapons were
filled the day before by tools/fill_weapon_durability.py, after "yes fill in the 729 missing records").

Records of maxhp 0 and duration 0 show in the item window as "HP 0/0, DUR 0", whatever they are: the old
dump's blank records and the quest items import_quests.py made from them (the Pledging Stone, the Prayer
Beads). The records that have them follow the weapons' rule, in each wear slot: maxhp 200 times the level
(1: 200, 3: 600, 8: 1600, 13: 2600, 18: 3600), duration 50 nearly always. So a record of no hit points is
given its slot and level's most common maxhp where at least three records of that slot and level carry one,
else 200 times its level (at least 200), and duration 50 unless it has one of its own; a held copy at 0 hit
points is made whole. Wear slots 1-11 (head to feet), 13 (a shield of no damage: the weapons' tool took
those with one), 18 (held) and 19 (hands); not the weapons, the fishing gear or the tools. Every change is
logged in wear_durability_fix_log with the values it replaced; --undo puts back each row still as this left
it. Report only by default. The server reads item records as it goes: no restart (a player sees new numbers
at the next login).

    tools/fill_wear_durability.py [--db eqoa.db] [--apply | --undo]
"""

import argparse
import collections
import sqlite3

WEAR_SLOTS = (1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 18, 19)
DURATION = 50
MIN_SAMPLES = 3

LOG_DDL = """CREATE TABLE IF NOT EXISTS wear_durability_fix_log (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl TEXT NOT NULL,
    rid INTEGER NOT NULL,
    old_maxhp INTEGER,
    old_duration INTEGER,
    new_maxhp INTEGER,
    new_duration INTEGER
)"""

# a shield that hits is a weapon (tools/fill_weapon_durability.py's)
ITEMS = "equipslot IN ({}) AND NOT (equipslot = 13 AND weapondamage > 0)".format(",".join(map(str, WEAR_SLOTS)))


def slot_level_hp(db):
    """Each slot and level's most common maxhp among the records that have one, where at least
    MIN_SAMPLES do."""
    seen = collections.defaultdict(collections.Counter)
    for slot, lv, hp in db.execute(f"SELECT equipslot, levelreq, maxhp FROM itempattern WHERE {ITEMS} AND maxhp > 0"):
        seen[(slot, lv)][hp] += 1
    out = {}
    for k, c in seen.items():
        hp, n = c.most_common(1)[0]
        if n >= MIN_SAMPLES:
            out[k] = hp
    return out


def hp_for(slot, level, table):
    return table.get((slot, level), 200 * max(level or 0, 1))


def plan(db):
    """(patternid, slot, level, maxhp to give, duration now, duration to give) for each worn record of
    no hit points: a duration of its own is kept."""
    table = slot_level_hp(db)
    rows = db.execute(f"SELECT patternid, equipslot, levelreq, duration FROM itempattern WHERE {ITEMS} AND maxhp = 0 "
                      "ORDER BY patternid").fetchall()
    return [(pid, slot, lv, hp_for(slot, lv, table), du, du or DURATION) for pid, slot, lv, du in rows], table


def apply(db, todo):
    db.execute(LOG_DDL)
    records = held = 0
    for pid, _, _, hp, old_du, du in todo:
        cur = db.execute("UPDATE itempattern SET maxhp = ?, duration = ? WHERE patternid = ? AND maxhp = 0 AND duration IS ?",
                         (hp, du, pid, old_du))
        if cur.rowcount:
            db.execute("INSERT INTO wear_durability_fix_log (tbl, rid, old_maxhp, old_duration, new_maxhp, new_duration) "
                       "VALUES ('itempattern', ?, 0, ?, ?, ?)", (pid, old_du, hp, du))
            records += 1
        for (itemid,) in db.execute("SELECT itemid FROM charinventory WHERE patternid = ? AND remainHP = 0", (pid,)).fetchall():
            db.execute("UPDATE charinventory SET remainHP = ? WHERE itemid = ?", (hp, itemid))
            db.execute("INSERT INTO wear_durability_fix_log (tbl, rid, old_maxhp, new_maxhp) "
                       "VALUES ('charinventory', ?, 0, ?)", (itemid, hp))
            held += 1
    return records, held


def undo(db):
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'wear_durability_fix_log'").fetchone()[0] == 0:
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for tbl, rid, old_hp, old_du, new_hp, new_du in db.execute(
            "SELECT tbl, rid, old_maxhp, old_duration, new_maxhp, new_duration FROM wear_durability_fix_log "
            "ORDER BY seq DESC").fetchall():
        if tbl == "itempattern":
            cur = db.execute("UPDATE itempattern SET maxhp = ?, duration = ? WHERE patternid = ? AND maxhp = ? AND duration = ?",
                             (old_hp or 0, old_du, rid, new_hp, new_du))
        else:
            cur = db.execute("UPDATE charinventory SET remainHP = ? WHERE itemid = ? AND remainHP = ?", (old_hp or 0, rid, new_hp))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE wear_durability_fix_log")
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
    by_slot = collections.Counter(slot for _, slot, _, _, _, _ in todo)
    kept = sum(1 for t in todo if t[4])
    print(f"{len(todo)} worn records with no hit points ({kept} with a duration of their own, kept), by wear slot: "
          + ", ".join(f"{s}: {n}" for s, n in sorted(by_slot.items())))
    print(f"{len(table)} slot-and-level values from the records that have them; the rest 200 a level")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    records, held = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {records} records, {held} held copies made whole")


if __name__ == "__main__":
    main()
