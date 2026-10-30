#!/usr/bin/env python3
"""The re-fit of 2026-10-30 04:20 UTC: Balance by class and level moved from the rows tools/tune_balance.py left to
the rows the simulator fits on eqoaclassic's data of 04:20 (claude-notes/balance-20261029.md).

The first tuning was fitted on the data of 01:30. By 04:20 the kits the classes wear had changed under it:
fix_armor_class.py moved 320 items' armour class out of a stat it had been written into (an "+63 agility" that was
AC), fix_reward_class_masks.py --all-rewards gave 187 quest rewards their quests' classes, and the trash items, the
class quest scrolls and items and Sandstorm's spell visuals went in. A monk of 5 then won a yellow two up 31% of the
time where the first tuning had it at 70% (its kit lost an item of +27 STR), and 31 of the cells the first tuning met
the targets in missed them (a warrior, shadow knight or rogue of 60 lost every yellow; healers and wizards were taken
down less). The same fit as the first tuning's (cmd/balancesim on that data: each class's share of an NPC's melee
and each pet class's pet hit points, at every level 1-60, to both yellows at once: the player's low point, or the
pet's hit points left, averaged over the fights won one and two up near 25%, one up won at least 80% of the time (a
healer 90%, its share then eased 8%), two up 65%; level 1 to one up alone) found 294 of the 720 levels unchanged and
189 moved by more than 7%. Every level takes the re-fit's share; levels 1-12 are each a row of their own (a small
hit rounds: a rogue of 2 won a yellow two up 65% of the time at a share of 42 and 2% at 44, where the hit went from
3 to 4) and each class's curve above 12 is cut to its fewest rows within 4% of every level: RETUNED. The pets' hit
points all fitted within 7% of the first fit's and are left as they were.

It writes only balanceclasslevel: a row the first tuning left and the re-fit does not keep is taken out, a row the
re-fit adds is added where none is, a row both have is changed, each only while it holds what the first tuning
left (a row a GM changed is left, and said). Every change is logged in balance_retune_log (the row's place, the
whole row before and after: NULL for a row added or taken out, why); --undo puts back each row still as this left
it and drops the log; a second --apply only does what is still to do. Both copy the database first
(<db>.bak-<when>-pre-balanceretune) unless --no-backup. Report only by default. tools/tune_balance.py first: on a
database it has not been applied to this does nothing.

No code changes with it: the server reads the balance tables at its start and at once whenever a GM saves a row of
any Balance page in the panel, so it takes these at its next start (or the next save).

    tools/retune_balance.py [--db eqoa.db] [--apply | --undo] [--no-backup]
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
import tune_balance as first  # noqa: E402  the first tuning's rows (as eqoaclassic has them since 2026-10-30)

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "balance_retune_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balanceclasslevel
    place   TEXT NOT NULL,   -- the row's class and level, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT,            -- the whole row after, JSON; NULL: taken out
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
TBL = "balanceclasslevel"
WHY = "the re-fit of 2026-10-30 04:20 (tools/retune_balance.py; claude-notes/balance-20261029.md)"

# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct)}, the re-fit (see the doc)
RETUNED = {
    0: {1: (89, 100), 2: (63, 100), 3: (95, 100), 4: (100, 100), 5: (95, 100), 6: (86, 100), 7: (79, 100), 8: (95, 100), 9: (95, 100), 10: (90, 100), 11: (86, 100), 12: (86, 100), 14: (105, 100), 17: (105, 100), 19: (74, 100), 20: (111, 100), 21: (95, 100), 22: (95, 100), 23: (86, 100), 24: (100, 100), 28: (90, 100), 29: (77, 100), 30: (90, 100), 32: (86, 100), 33: (77, 100), 34: (77, 100), 37: (95, 100), 44: (95, 100), 46: (86, 100), 47: (90, 100), 48: (77, 100), 49: (86, 100), 50: (86, 100), 52: (111, 100), 56: (100, 100), 58: (123, 100), 60: (95, 100)},
    1: {1: (42, 100), 2: (42, 100), 3: (63, 100), 4: (65, 100), 5: (57, 100), 6: (63, 100), 7: (57, 100), 8: (63, 100), 9: (63, 100), 10: (57, 100), 11: (57, 100), 12: (70, 100), 14: (82, 100), 15: (77, 100), 16: (63, 100), 17: (63, 100), 18: (51, 100), 19: (46, 100), 20: (60, 100), 21: (51, 100), 23: (63, 100), 24: (82, 100), 25: (70, 100), 27: (57, 100), 28: (60, 100), 29: (57, 100), 30: (86, 100), 32: (86, 100), 33: (74, 100), 34: (77, 100), 35: (63, 100), 37: (66, 100), 38: (77, 100), 39: (60, 100), 40: (82, 100), 41: (57, 100), 44: (63, 100), 45: (42, 100), 46: (51, 100), 47: (46, 100), 48: (46, 100), 49: (57, 100), 50: (51, 100), 51: (54, 100), 52: (42, 100), 53: (57, 100), 54: (42, 100), 55: (42, 100), 56: (54, 100), 57: (57, 100), 58: (46, 100), 59: (40, 100), 60: (60, 100)},
    2: {1: (89, 100), 2: (66, 100), 3: (86, 100), 4: (95, 100), 5: (136, 100), 6: (136, 100), 7: (143, 100), 8: (138, 100), 9: (140, 100), 10: (129, 100), 11: (114, 100), 12: (117, 100), 14: (117, 100), 17: (95, 100), 18: (74, 100), 19: (63, 100), 20: (90, 100), 23: (82, 100), 24: (95, 100), 25: (100, 100), 26: (90, 100), 27: (105, 100), 28: (111, 100), 29: (90, 100), 31: (108, 100), 33: (95, 100), 34: (95, 100), 35: (82, 100), 36: (77, 100), 38: (90, 100), 41: (95, 100), 42: (90, 100), 43: (100, 100), 46: (90, 100), 47: (93, 100), 48: (77, 100), 49: (95, 100), 50: (95, 100), 51: (111, 100), 52: (100, 100), 54: (103, 100), 56: (90, 100), 58: (117, 100), 59: (100, 100), 60: (159, 100)},
    3: {1: (74, 100), 2: (66, 100), 3: (77, 100), 4: (77, 100), 5: (86, 100), 6: (86, 100), 7: (95, 100), 8: (90, 100), 9: (95, 100), 10: (86, 100), 11: (86, 100), 12: (90, 100), 15: (95, 100), 16: (86, 100), 17: (86, 100), 18: (70, 100), 19: (63, 100), 20: (129, 100), 23: (105, 100), 25: (129, 100), 27: (111, 100), 28: (117, 100), 29: (100, 100), 30: (111, 100), 34: (117, 100), 35: (95, 100), 37: (100, 100), 38: (123, 100), 40: (111, 100), 41: (129, 100), 42: (129, 100), 43: (140, 100), 44: (123, 100), 46: (129, 100), 47: (123, 100), 48: (93, 100), 50: (143, 100), 51: (155, 100), 52: (133, 100), 53: (159, 100), 55: (155, 100), 56: (129, 100), 58: (185, 100), 60: (136, 100)},
    4: {1: (74, 100), 2: (66, 100), 3: (86, 100), 4: (95, 100), 5: (74, 100), 6: (69, 100), 7: (82, 100), 8: (77, 100), 9: (77, 100), 10: (69, 100), 11: (77, 100), 12: (82, 100), 13: (117, 100), 14: (95, 100), 15: (129, 100), 16: (123, 100), 17: (105, 100), 18: (70, 100), 19: (63, 100), 20: (117, 100), 22: (100, 100), 23: (117, 100), 24: (117, 100), 25: (129, 100), 28: (123, 100), 30: (136, 100), 32: (136, 100), 33: (159, 100), 34: (143, 100), 35: (167, 100), 36: (151, 100), 41: (167, 100), 42: (147, 100), 44: (143, 100), 47: (205, 100), 48: (151, 100), 49: (205, 100), 50: (176, 100), 51: (216, 100), 52: (205, 100), 53: (239, 100), 54: (216, 100), 55: (216, 100), 56: (239, 100), 58: (227, 100), 59: (200, 100), 60: (302, 100)},
    5: {1: (42, 100), 2: (42, 100), 3: (51, 100), 4: (95, 100), 5: (86, 100), 6: (77, 100), 7: (86, 100), 8: (77, 100), 9: (86, 100), 10: (70, 100), 11: (74, 100), 12: (79, 100), 14: (86, 100), 15: (82, 100), 16: (68, 100), 17: (74, 100), 18: (54, 100), 19: (49, 100), 20: (70, 100), 21: (63, 100), 22: (66, 100), 24: (86, 100), 25: (86, 100), 27: (70, 100), 28: (77, 100), 29: (77, 100), 30: (86, 100), 31: (86, 100), 32: (82, 100), 33: (66, 100), 35: (77, 100), 39: (77, 100), 41: (90, 100), 42: (77, 100), 43: (86, 100), 44: (74, 100), 45: (77, 100), 46: (70, 100), 47: (77, 100), 48: (74, 100), 49: (86, 100), 50: (74, 100), 51: (74, 100), 52: (66, 100), 53: (74, 100), 54: (54, 100), 55: (54, 100), 56: (63, 100), 57: (57, 100), 58: (60, 100), 59: (38, 100), 60: (60, 100)},
    6: {1: (42, 100), 2: (42, 100), 3: (46, 100), 4: (57, 100), 5: (60, 100), 6: (51, 100), 7: (63, 100), 8: (63, 100), 9: (63, 100), 10: (55, 100), 11: (57, 100), 12: (70, 100), 13: (77, 100), 15: (77, 100), 16: (63, 100), 17: (68, 100), 18: (51, 100), 19: (42, 100), 20: (51, 100), 21: (48, 100), 22: (51, 100), 23: (49, 100), 24: (63, 100), 27: (44, 100), 28: (51, 100), 30: (57, 100), 31: (51, 100), 32: (54, 100), 33: (42, 100), 34: (46, 100), 35: (54, 100), 36: (49, 100), 37: (49, 100), 38: (42, 100), 39: (42, 100), 40: (51, 100), 41: (46, 100), 42: (49, 100), 43: (46, 100), 46: (34, 100), 47: (34, 100), 51: (51, 100), 52: (34, 100), 53: (49, 100), 54: (23, 100), 56: (34, 100), 57: (31, 100), 58: (42, 100), 59: (23, 100), 60: (21, 100)},
    7: {1: (17, 100), 2: (17, 100), 3: (47, 100), 4: (47, 100), 5: (47, 100), 6: (47, 100), 7: (71, 100), 8: (71, 100), 9: (71, 100), 10: (64, 100), 11: (79, 100), 12: (119, 100), 13: (87, 100), 15: (90, 100), 16: (79, 100), 17: (81, 100), 18: (58, 100), 19: (52, 100), 20: (75, 100), 23: (71, 100), 25: (90, 100), 27: (79, 100), 30: (95, 100), 31: (113, 100), 32: (97, 100), 34: (79, 100), 35: (90, 100), 36: (79, 100), 37: (80, 100), 38: (87, 100), 39: (108, 100), 40: (103, 100), 41: (108, 100), 42: (98, 100), 44: (95, 100), 45: (83, 100), 46: (94, 100), 47: (97, 100), 48: (78, 100), 49: (109, 100), 50: (52, 100), 52: (51, 100), 53: (60, 100), 54: (45, 100), 55: (42, 100), 56: (45, 100), 57: (40, 100), 58: (58, 100), 59: (50, 100), 60: (79, 100)},
    8: {1: (17, 78), 2: (17, 78), 3: (47, 78), 4: (58, 78), 5: (58, 78), 6: (64, 78), 7: (71, 78), 8: (97, 78), 9: (92, 78), 10: (75, 78), 11: (92, 78), 12: (108, 78), 13: (119, 91), 15: (97, 116), 17: (90, 165), 18: (79, 258), 19: (58, 352), 20: (83, 90), 21: (74, 105), 22: (73, 120), 23: (79, 135), 24: (80, 150), 25: (81, 178), 26: (71, 205), 28: (75, 260), 29: (99, 100), 30: (97, 105), 31: (86, 111), 32: (90, 116), 33: (82, 154), 34: (84, 192), 35: (87, 206), 36: (71, 220), 37: (68, 233), 38: (87, 247), 39: (97, 95), 40: (79, 103), 41: (68, 111), 44: (61, 134), 45: (68, 142), 46: (58, 149), 47: (55, 157), 48: (68, 165), 49: (97, 86), 50: (79, 92), 51: (162, 98), 52: (146, 105), 53: (179, 111), 54: (108, 117), 56: (146, 130), 57: (119, 136), 58: (125, 142), 59: (119, 142), 60: (162, 90)},
    9: {1: (17, 100), 2: (17, 100), 3: (39, 100), 4: (119, 100), 5: (132, 100), 6: (108, 100), 7: (132, 100), 8: (125, 100), 9: (119, 100), 10: (97, 100), 11: (132, 100), 12: (139, 100), 14: (132, 100), 16: (108, 100), 17: (108, 100), 18: (75, 100), 19: (66, 100), 20: (79, 100), 23: (64, 100), 25: (83, 100), 26: (75, 100), 29: (81, 100), 31: (99, 100), 32: (102, 100), 33: (75, 100), 34: (73, 100), 35: (79, 100), 36: (71, 100), 38: (71, 100), 39: (83, 100), 41: (75, 100), 43: (85, 100), 44: (79, 100), 45: (98, 100), 46: (83, 100), 49: (97, 100), 50: (87, 100), 52: (87, 100), 54: (111, 100), 55: (83, 100), 56: (92, 100), 57: (68, 100), 58: (119, 100), 59: (87, 100), 60: (146, 100)},
    10: {1: (100, 53), 2: (100, 100), 3: (100, 183), 4: (100, 64), 7: (100, 150), 8: (100, 90), 9: (100, 90), 11: (100, 122), 12: (100, 27), 14: (100, 52), 15: (100, 76), 16: (100, 55), 17: (100, 67), 18: (100, 129), 19: (100, 165), 20: (100, 86), 23: (100, 135), 24: (100, 45), 25: (100, 55), 28: (100, 111), 29: (100, 67), 32: (100, 111), 33: (100, 162), 34: (100, 74), 38: (100, 142), 39: (100, 82), 43: (100, 122), 44: (100, 74), 45: (100, 74), 46: (100, 90), 47: (100, 82), 60: (100, 165)},
    11: {1: (100, 90), 2: (100, 135), 3: (100, 202), 4: (100, 70), 7: (100, 165), 8: (100, 95), 9: (100, 95), 11: (100, 129), 12: (100, 78), 15: (100, 116), 16: (100, 74), 17: (100, 86), 19: (100, 183), 20: (100, 86), 23: (100, 135), 24: (100, 90), 25: (100, 90), 28: (100, 135), 29: (100, 95), 30: (100, 95), 32: (100, 116), 33: (100, 145), 35: (100, 100), 38: (100, 129), 39: (100, 95), 43: (100, 122), 44: (100, 95), 48: (100, 129), 49: (100, 95), 50: (100, 93), 54: (100, 122), 55: (100, 95), 59: (100, 116), 60: (100, 95)},
    12: {1: (100, 78), 4: (100, 224), 6: (100, 452), 7: (100, 497), 8: (100, 74), 9: (100, 74), 10: (100, 86), 12: (100, 122), 15: (100, 224), 16: (100, 62), 17: (100, 74), 19: (100, 150), 23: (100, 267), 24: (100, 100), 32: (100, 247), 33: (100, 318), 34: (100, 135), 43: (100, 247), 44: (100, 122), 48: (100, 165), 49: (100, 116), 54: (100, 157), 55: (100, 122), 59: (100, 150), 60: (100, 122)},
    13: {1: (19, 74), 2: (19, 74), 3: (19, 74), 4: (20, 74), 5: (27, 74), 6: (36, 74), 7: (20, 74), 8: (22, 74), 9: (21, 74), 10: (18, 74), 11: (25, 74), 12: (23, 74), 14: (22, 102), 15: (52, 116), 16: (32, 140), 17: (24, 165), 18: (17, 224), 19: (33, 352), 20: (40, 86), 21: (30, 102), 22: (42, 118), 23: (35, 134), 24: (35, 150), 26: (36, 205), 27: (40, 232), 28: (38, 260), 29: (42, 95), 30: (50, 108), 31: (45, 122), 32: (70, 135), 33: (55, 174), 34: (53, 213), 35: (64, 225), 36: (51, 236), 37: (64, 248), 38: (60, 260), 39: (75, 90), 40: (90, 100), 41: (92, 111), 42: (120, 121), 43: (88, 132), 44: (70, 142), 45: (51, 148), 46: (82, 154), 48: (54, 165), 49: (87, 82), 50: (63, 90), 51: (90, 98), 52: (77, 106), 53: (111, 114), 54: (52, 122), 55: (77, 126), 57: (68, 134), 58: (77, 138), 59: (57, 142), 60: (77, 82)},
    14: {1: (17, 100), 2: (17, 100), 3: (17, 100), 4: (17, 100), 5: (17, 100), 6: (17, 100), 7: (27, 100), 8: (21, 100), 9: (27, 100), 10: (35, 100), 11: (54, 100), 12: (46, 100), 13: (43, 100), 14: (54, 100), 16: (52, 100), 17: (65, 100), 18: (36, 100), 19: (34, 100), 20: (52, 100), 21: (61, 100), 22: (55, 100), 23: (83, 100), 24: (100, 100), 25: (84, 100), 26: (91, 100), 28: (75, 100), 29: (74, 100), 30: (98, 100), 31: (79, 100), 32: (118, 100), 33: (57, 100), 34: (57, 100), 35: (81, 100), 36: (63, 100), 38: (91, 100), 39: (63, 100), 41: (63, 100), 43: (106, 100), 44: (82, 100), 45: (86, 100), 46: (239, 100), 47: (178, 100), 48: (215, 100), 50: (176, 100), 51: (143, 100), 52: (143, 100), 53: (181, 100), 54: (133, 100), 55: (179, 100), 56: (205, 100), 57: (169, 100), 59: (205, 100), 60: (123, 100)},
}


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def place_text(row):
    return json.dumps({"class": row["class"], "level": row["level"]}, sort_keys=True, separators=(",", ":"))


def row_of(db, cls, level):
    cur = db.execute(f"SELECT * FROM {TBL} WHERE class = ? AND level = ?", (cls, level))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def values(r):
    return None if r is None else (r["melee_taken_pct"], r["pet_hp_pct"])


def plan(db):
    """The changes [(class, level, row before or None, row after or None)] and the rows left, with why."""
    changes, kept = [], []
    if not has_table(db, TBL) or "pet_hp_pct" not in [r[1] for r in db.execute(f"PRAGMA table_info({TBL})")]:
        return changes, [(TBL, "no such table or no pet_hp_pct: tools/tune_balance.py first, with a server of 2026-10-30")]
    if not db.execute(f"SELECT count(*) FROM {TBL}").fetchone()[0]:
        return changes, [(TBL, "empty: tools/tune_balance.py first")]
    for cls in sorted(set(first.CLASSLEVELS) | set(RETUNED)):
        was, want = first.CLASSLEVELS.get(cls, {}), RETUNED.get(cls, {})
        for level in sorted(set(was) | set(want)):
            old = tuple(map(float, was[level])) if level in was else None
            new = tuple(map(float, want[level])) if level in want else None
            if old == new:
                continue
            r = row_of(db, cls, level)
            have = values(r)
            if have == new:
                continue  # done already
            if have != old:
                kept.append((f"{TBL} {cls} {level}", f"{have if have else 'no row'}, not the first tuning's "
                                                     f"{old if old else 'no row'}: a GM's"))
                continue
            after = None
            if new is not None:
                after = dict(r) if r else {"class": cls, "level": level, "note": "", "source": "seed"}
                after.update(melee_taken_pct=new[0], pet_hp_pct=new[1])
            changes.append((cls, level, r, after))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-balanceretune", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-balanceretune"
    shutil.copy2(path, dst)
    return dst


def write_row(db, cls, level, row, insert):
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {TBL} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    elif row is None:
        db.execute(f"DELETE FROM {TBL} WHERE class = ? AND level = ?", (cls, level))
    else:
        cols = [c for c in row if c not in ("class", "level")]
        db.execute(f"UPDATE {TBL} SET {', '.join(f'{c} = ?' for c in cols)} WHERE class = ? AND level = ?",
                   [row[c] for c in cols] + [cls, level])


def reorder(db):
    """balanceclasslevel rewritten in (class, level) order, as tools/tune_balance.py made it: a row taken out and put
    back would otherwise go to the table's end, and a database's dump would differ by the order alone."""
    cols = ", ".join(r[1] for r in db.execute(f"PRAGMA table_info({TBL})"))
    db.execute(f"CREATE TEMP TABLE retune_order AS SELECT {cols} FROM {TBL} ORDER BY class, level")
    db.execute(f"DELETE FROM {TBL}")
    db.execute(f"INSERT INTO {TBL} ({cols}) SELECT {cols} FROM retune_order ORDER BY class, level")
    db.execute("DROP TABLE retune_order")


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for cls, level, before, after in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (TBL, place_text({"class": cls, "level": level}),
                        None if before is None else json.dumps(before, sort_keys=True),
                        None if after is None else json.dumps(after, sort_keys=True), WHY, when))
            write_row(db, cls, level, after, insert=before is None)
        reorder(db)
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
        for seq, tbl, place, before, after in logged:
            if tbl != TBL:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            p = json.loads(place)
            now = row_of(db, p["class"], p["level"])
            if now != (None if after is None else json.loads(after)):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            b = None if before is None else json.loads(before)
            write_row(db, p["class"], p["level"], b, insert=now is None)
            back += 1
        reorder(db)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start or a Balance page's next save")


def describe(change):
    cls, level, before, after = change
    if before is None:
        return f"{TBL} class {cls} level {level}: added, melee_taken_pct {after['melee_taken_pct']:g}, pet_hp_pct {after['pet_hp_pct']:g}"
    if after is None:
        return f"{TBL} class {cls} level {level}: taken out (was {before['melee_taken_pct']:g}/{before['pet_hp_pct']:g})"
    return (f"{TBL} class {cls} level {level}: melee_taken_pct {before['melee_taken_pct']:g} -> {after['melee_taken_pct']:g}, "
            f"pet_hp_pct {before['pet_hp_pct']:g} -> {after['pet_hp_pct']:g}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the re-fit (logged)")
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
    changes, kept = plan(db)
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
