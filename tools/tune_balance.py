#!/usr/bin/env python3
"""The balance tables tuned to the user's targets of 2026-10-30 (claude-notes/balance-20261029.md):

  "Yellow (+1/+2): should be a scary fight, at least take you down to less than a third hp" and still
  usually won; "Red (+3 and up): needs a group" (a lone player mostly loses, a fitting group of 2-3 of
  the level wins).

Found with the fight simulator (cmd/balancesim on eqoaclassic's own items, spells and pets), which
reproduces the live log the user called "too easy" (Dust, a level 5 shadow knight of 170 HP, killed a
level 6 fire bat of 786 HP hitting 8 every 3 s in 24 s and kept 65% of his hit points: the simulator's
same fight under the 01:30 tables, 786 HP, 8 a hit, the shield's 2, about 30 s and 65%; under these it
lasts 46 s and ends at 15%, entity/balancesim_real_test.go's TestRealDustAndTheFireBat).

What it writes (every value is a GM's to change later in the panel's World group):

  balancecon      the con's rows: an NPC one up hits 1.47 times as hard and takes 68% of the damage (it was
                  1.12/91%), two up 1.4/70% (1.25/83%), three up 1.9/53%, four up 2.2/45%, five up and past
                  2.3/45%; a root at one up is no likelier resisted or broken than at even (0/5), two up 10/10,
                  three 30/20, four 45/25, five 60/30.
  balancelevel    con_pct 100 at every level (it was 0 at 1-8, 25 at 10 and 70 at 15: a low level's yellow
                  was an even NPC one level up, which is why the fire bat was easy); a summoned pet's hit
                  points (pet_hp_pct) 100 at 1-8 rising to 540 at 60 (300 at 1 and 215-225 at 5-10 let a low
                  level's pet beat its yellows with 92% of its hit points left), so a pet class's yellow takes
                  its pet near a third too; and a row at every level 1-14 whose damage_pct evens out the floor's step (an NPC's
                  least hit is (level+1)/2, defence.go: a step up at each odd level, and at 1-14 most of its
                  hits on a tank are that floor, so an odd level hit up to a third harder than the line through
                  the levels and an even one up to a fifth softer, and a yellow two up was a wall at every odd
                  player level): the 01:30 line's times (2L+1)/(4*ceil(L/2)); its other columns the line's.
                  Hit points, the arts' cooldown and regen are left: an even fight still leaves the median class
                  70-80% of its hit points, as before.
  balanceclass    every class 100 (the ranger's 70, bard's 80 and rogue's 65 move into the next table).
  balanceclasslevel  (the table this branch adds) by a player's class and level: melee_taken_pct, the share
                  of an NPC's melee the class takes (one figure a class cannot hold a class whose strength
                  turns with its level: a monk takes 66-104% at 1-12 and 326% at 60), and pet_hp_pct, its
                  summoned pet's hit points as a share of balancelevel's (each class's pets lag their owner
                  their own way: a magician's last is level 47's Ghilan, an enchanter's come every eight to
                  ten levels, and an NPC's hits on a pet take the con on the PET's level, so a pet three or
                  more below the NPC takes a red's). Both fitted by the simulator at every level 1-60 to both
                  yellows at once: the player's low point (the least hit points it had in a fight won, the
                  user's "take you down to"; a healer ends higher than that), or the pet's hit points left
                  for a pet class, averaged over the fights won one and two up near 25%, one up won at least
                  80% of the time (a healer 90%: it wins or runs out of power, and a few percent more melee
                  takes it from the one to the other) and two up 65%; level 1 to one up alone (two up is a
                  level 3 NPC, a wall at any share); a level at which no share meets both takes the nearest
                  level's that does; a healer's shares eased 8% to stay off that cliff. Each curve is then
                  cut to its fewest rows within 7% of every level's fit (not smoothed: a melee class's swings
                  double at 20 and its yellows' NPCs' at 19-20, so a warrior of 20 needs a share of 111 and one
                  of 21 86, and a pet class's share is a saw that drops at each new summoning). The druid's melee
                  on its hand-to-hand play (its root play wins a yellow at 20-40 with
                  nearly all its hit points in 13-16 minutes), the wizard's and the alchemist's on their root
                  play (a root holds or breaks, so their low point in a yellow won stays high: two up's 65% won
                  is what bounds them).

A row is changed only while it holds what this expects (the tables as eqoaclassic had them at 01:30 UTC
2026-10-30: the seeds of 2026-10-29); a row a GM changed is left, and said. A row it adds is only added
where none is. Every change is logged in balance_tune_log (the table, the row's place, the whole row
before and after, why); --undo puts back each row still as this left it (one changed since is left, and
said), deletes the rows it added (and balanceclasslevel when it made it and it is empty again), and drops
the log; a second --apply only adds what is still to do. Both copy the database first
(<db>.bak-<when>-pre-balancetune) unless --no-backup. Report only by default.

The server reads the balance tables at its start and again whenever a GM saves a row of any Balance page
(any row, as it is). balanceclasslevel is read only by a server of 2026-10-30 or later, and the rest of
the tuning leans on it (the ranger's, bard's and rogue's shares move there, every class's is shaped by
it), so write this with that server: a server of 2026-10-30 or later meeting a database whose balance was
made before makes balanceclasslevel empty and leaves it to this tool, and takes the tuning at its next
start or at once at the next save of a Balance page; under an older server a rogue would take all of an
NPC's melee. The tool makes the table itself if it is missing.

    tools/tune_balance.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import math
import os
import shutil
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
LOG = "balance_tune_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balancelevel | balancecon | balanceclass | balanceclasslevel
    place   TEXT NOT NULL,   -- the row's primary key columns and values, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT NOT NULL,   -- the whole row after, JSON
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
CLASSLEVEL_SCHEMA = """CREATE TABLE IF NOT EXISTS balanceclasslevel (
    class           INTEGER NOT NULL,
    level           INTEGER NOT NULL,
    melee_taken_pct REAL NOT NULL DEFAULT 100,
    pet_hp_pct      REAL NOT NULL DEFAULT 100,
    note            TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (class, level)
)"""
PLACES = {"balancelevel": ("level",), "balancecon": ("diff",), "balanceclass": ("class",),
          "balanceclasslevel": ("class", "level")}
MADE = '{"made":"table"}'  # a log row's place when the tool made the table itself
WHY = "the tuning of 2026-10-30 (tools/tune_balance.py; claude-notes/balance-20261029.md)"

# balancelevel as eqoaclassic had it at 01:30 UTC 2026-10-30 (the seeds of 2026-10-29): level -> its columns
LEVEL_COLS = ("hp_pct", "damage_pct", "con_pct", "pet_hp_pct", "pet_damage_pct", "art_cooldown_ms", "regen_pct")
LEVELS_0130 = {
    1: (300, 250, 0, 300, 250, 1500, 5), 5: (215, 265, 0, 215, 265, 1500, 5), 8: (225, 260, 0, 225, 260, 1500, 5),
    10: (220, 225, 25, 220, 225, 1500, 5), 15: (230, 255, 70, 161, 280, 1500, 5), 18: (230, 250, 100, 161, 300, 1500, 5),
    25: (250, 220, 100, 175, 285, 1500, 5), 30: (300, 200, 100, 210, 300, 1500, 5), 35: (300, 195, 100, 210, 340, 1500, 5),
    45: (330, 190, 100, 287, 430, 1500, 5), 50: (340, 190, 100, 357, 475, 1500, 5), 60: (400, 160, 100, 560, 560, 1500, 5),
}
# what this changes in those rows: con_pct 100 at every level, a summoned pet's hit points (pet_hp_pct)
LEVEL_WANT = {"con_pct": {l: 100 for l in LEVELS_0130},
              "pet_hp_pct": {1: 100, 5: 100, 8: 100, 10: 140, 15: 155, 18: 155, 25: 190, 30: 205, 35: 250, 45: 375,
                             50: 450, 60: 540}}
# and the floor's step evened out (defence.go creatureRoll: an NPC's least hit is (level+1)/2, a step up at
# each odd level, and at levels 1-14 most of its hits on a tank are that floor, so an odd level hit up to a
# third harder than the line through the levels and an even one up to a fifth softer): a row at every level
# 1-14 whose damage_pct is the 01:30 line's there times (2L+1)/(4*ceil(L/2)), the floor's line (L/2 + 1/4)
# over the floor; the rows it adds take every other column from the table's line at their level
FLOOR_LEVELS = range(1, 15)
FLOOR_NOTE = "damage_pct x (2L+1)/(4*ceil(L/2)): the floor's step evened out (tools/tune_balance.py)"


def floor_factor(level):
    return (2 * level + 1) / (4 * math.ceil(level / 2))


def line(rows, level, col):
    """The table's line at level (balance.go's Level): a row's value at its level, the line between two rows,
    the end rows past the ends."""
    ls = sorted(rows)
    if level <= ls[0]:
        return rows[ls[0]][col]
    if level >= ls[-1]:
        return rows[ls[-1]][col]
    for a, b in zip(ls, ls[1:]):
        if a <= level <= b:
            return rows[a][col] + (rows[b][col] - rows[a][col]) * (level - a) / (b - a)


def floor_damage(level):
    """The damage_pct the tuning gives level: the 01:30 line's times the floor's factor, whole."""
    old = {l: dict(zip(LEVEL_COLS, v)) for l, v in LEVELS_0130.items()}
    return round(line(old, level, "damage_pct") * floor_factor(level))


# balancecon: diff -> (damage_pct, taken_pct, root_resist_pct, root_break_pct) now, and wanted
CONS = {
    0: ((100, 100, 0, 5), (100, 100, 0, 5)),
    1: ((112, 91, 5, 10), (147, 68, 0, 5)),
    2: ((125, 83, 15, 15), (140, 70, 10, 10)),
    3: ((135, 79, 35, 25), (190, 53, 30, 20)),
    4: ((142, 76, 50, 30), (220, 45, 45, 25)),
    5: ((150, 72, 65, 35), (230, 45, 60, 30)),
}
# balanceclass: class -> melee_taken_pct now (every class's wanted is 100)
CLASSES = {0: 100, 1: 70, 2: 100, 3: 100, 4: 100, 5: 80, 6: 65, 7: 100, 8: 100, 9: 100, 10: 100, 11: 100,
           12: 100, 13: 100, 14: 100}
# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct)}, the simulator's fit (see the doc)
CLASSLEVELS = {
    0: {1: (89, 100), 2: (66, 100), 3: (95, 100), 7: (79, 100), 9: (100, 100), 12: (86, 100), 14: (105, 100), 16: (105, 100), 17: (100, 100), 18: (79, 100), 19: (77, 100), 20: (111, 100), 21: (86, 100), 23: (86, 100), 24: (105, 100), 25: (105, 100), 29: (77, 100), 32: (86, 100), 34: (74, 100), 38: (95, 100), 47: (90, 100), 48: (77, 100), 50: (86, 100), 52: (111, 100), 54: (105, 100), 55: (117, 100), 56: (105, 100), 57: (111, 100), 58: (136, 100), 59: (105, 100), 60: (143, 100)},
    1: {1: (42, 100), 2: (42, 100), 3: (63, 100), 4: (65, 100), 5: (57, 100), 6: (63, 100), 7: (57, 100), 8: (63, 100), 11: (57, 100), 14: (86, 100), 16: (63, 100), 17: (63, 100), 19: (46, 100), 20: (66, 100), 22: (63, 100), 24: (77, 100), 27: (57, 100), 29: (60, 100), 30: (90, 100), 36: (63, 100), 38: (77, 100), 39: (63, 100), 40: (77, 100), 42: (57, 100), 43: (57, 100), 44: (70, 100), 45: (38, 100), 46: (42, 100), 48: (38, 100), 49: (60, 100), 50: (46, 100), 51: (51, 100), 52: (38, 100), 53: (51, 100), 54: (36, 100), 55: (54, 100), 56: (38, 100), 57: (51, 100), 59: (34, 100), 60: (57, 100)},
    2: {1: (89, 100), 2: (77, 100), 4: (95, 100), 5: (136, 100), 7: (143, 100), 9: (140, 100), 11: (114, 100), 15: (111, 100), 17: (95, 100), 18: (72, 100), 19: (70, 100), 20: (95, 100), 23: (82, 100), 24: (95, 100), 28: (108, 100), 29: (90, 100), 30: (105, 100), 32: (105, 100), 36: (74, 100), 39: (95, 100), 42: (90, 100), 43: (100, 100), 47: (93, 100), 48: (77, 100), 49: (95, 100), 51: (105, 100), 56: (95, 100), 57: (111, 100), 58: (111, 100), 59: (86, 100), 60: (143, 100)},
    3: {1: (89, 100), 2: (66, 100), 7: (95, 100), 10: (86, 100), 15: (95, 100), 17: (86, 100), 18: (70, 100), 19: (70, 100), 20: (129, 100), 23: (105, 100), 25: (129, 100), 28: (111, 100), 29: (86, 100), 30: (111, 100), 31: (117, 100), 32: (111, 100), 33: (123, 100), 35: (95, 100), 36: (95, 100), 37: (100, 100), 38: (123, 100), 40: (114, 100), 43: (143, 100), 44: (123, 100), 47: (117, 100), 48: (95, 100), 51: (159, 100), 52: (143, 100), 53: (167, 100), 56: (151, 100), 57: (159, 100), 58: (195, 100), 59: (117, 100), 60: (185, 100)},
    4: {1: (74, 100), 2: (66, 100), 3: (104, 100), 5: (89, 100), 6: (69, 100), 7: (82, 100), 8: (77, 100), 9: (86, 100), 10: (68, 100), 12: (82, 100), 13: (117, 100), 14: (95, 100), 15: (129, 100), 17: (117, 100), 18: (77, 100), 19: (72, 100), 20: (111, 100), 22: (100, 100), 25: (129, 100), 28: (123, 100), 31: (136, 100), 32: (159, 100), 34: (143, 100), 36: (167, 100), 39: (159, 100), 41: (176, 100), 42: (151, 100), 44: (136, 100), 47: (216, 100), 48: (151, 100), 49: (205, 100), 50: (176, 100), 51: (216, 100), 56: (233, 100), 57: (211, 100), 58: (265, 100), 59: (227, 100), 60: (326, 100)},
    5: {1: (42, 100), 2: (42, 100), 4: (86, 100), 6: (77, 100), 7: (86, 100), 8: (77, 100), 9: (86, 100), 10: (70, 100), 14: (86, 100), 16: (68, 100), 17: (70, 100), 19: (46, 100), 20: (70, 100), 22: (68, 100), 24: (86, 100), 27: (72, 100), 31: (90, 100), 33: (63, 100), 35: (77, 100), 43: (86, 100), 44: (63, 100), 46: (77, 100), 48: (77, 100), 49: (93, 100), 50: (77, 100), 52: (68, 100), 53: (77, 100), 54: (57, 100), 57: (54, 100), 59: (37, 100), 60: (60, 100)},
    6: {1: (42, 100), 2: (42, 100), 4: (57, 100), 6: (51, 100), 7: (63, 100), 11: (57, 100), 13: (77, 100), 17: (63, 100), 19: (42, 100), 20: (54, 100), 21: (46, 100), 22: (53, 100), 23: (51, 100), 24: (63, 100), 27: (46, 100), 30: (57, 100), 32: (51, 100), 33: (42, 100), 34: (42, 100), 35: (57, 100), 38: (42, 100), 39: (42, 100), 40: (51, 100), 41: (46, 100), 42: (49, 100), 44: (34, 100), 46: (42, 100), 48: (42, 100), 51: (57, 100), 52: (38, 100), 53: (54, 100), 54: (24, 100), 55: (34, 100), 56: (36, 100), 57: (26, 100), 58: (31, 100), 59: (31, 100), 60: (42, 100)},
    7: {1: (15, 100), 2: (15, 100), 3: (31, 100), 4: (31, 100), 5: (47, 100), 6: (47, 100), 7: (71, 100), 8: (71, 100), 10: (52, 100), 11: (79, 100), 12: (119, 100), 13: (87, 100), 15: (90, 100), 19: (51, 100), 20: (75, 100), 23: (68, 100), 25: (83, 100), 28: (81, 100), 29: (102, 100), 30: (95, 100), 32: (113, 100), 33: (92, 100), 34: (109, 100), 36: (75, 100), 37: (75, 100), 40: (125, 100), 41: (105, 100), 42: (116, 100), 43: (97, 100), 45: (90, 100), 46: (102, 100), 48: (98, 100), 49: (113, 100), 50: (52, 100), 53: (60, 100), 55: (37, 100), 56: (50, 100), 57: (47, 100), 60: (74, 100)},
    8: {1: (52, 78), 4: (52, 78), 7: (64, 78), 8: (97, 78), 9: (92, 78), 10: (71, 78), 11: (97, 78), 12: (108, 78), 13: (119, 91), 15: (102, 116), 17: (85, 165), 18: (76, 258), 19: (58, 352), 20: (86, 90), 21: (75, 105), 24: (75, 150), 25: (75, 178), 26: (85, 205), 28: (75, 260), 29: (99, 100), 32: (85, 116), 34: (75, 192), 37: (71, 233), 38: (84, 247), 39: (97, 95), 42: (57, 118), 44: (55, 134), 45: (39, 142), 46: (56, 149), 47: (33, 157), 48: (68, 165), 49: (102, 86), 50: (79, 92), 51: (154, 98), 52: (125, 105), 53: (179, 111), 54: (119, 117), 55: (146, 123), 56: (119, 130), 57: (162, 136), 58: (102, 142), 59: (102, 142), 60: (146, 90)},
    9: {1: (47, 100), 3: (47, 100), 4: (125, 100), 5: (132, 100), 6: (108, 100), 9: (119, 100), 10: (97, 100), 11: (132, 100), 14: (132, 100), 17: (108, 100), 18: (79, 100), 19: (66, 100), 20: (79, 100), 21: (79, 100), 23: (64, 100), 24: (77, 100), 27: (75, 100), 32: (97, 100), 33: (75, 100), 34: (82, 100), 36: (58, 100), 39: (83, 100), 40: (71, 100), 45: (97, 100), 46: (87, 100), 49: (98, 100), 50: (87, 100), 51: (94, 100), 53: (87, 100), 54: (110, 100), 55: (79, 100), 56: (79, 100), 58: (106, 100), 59: (87, 100), 60: (148, 100)},
    10: {1: (100, 53), 2: (100, 100), 3: (100, 183), 4: (100, 64), 7: (100, 150), 8: (100, 90), 9: (100, 90), 11: (100, 122), 12: (100, 27), 14: (100, 52), 15: (100, 76), 16: (100, 55), 17: (100, 67), 18: (100, 129), 19: (100, 165), 20: (100, 86), 23: (100, 135), 24: (100, 45), 25: (100, 55), 28: (100, 111), 29: (100, 67), 32: (100, 111), 33: (100, 162), 34: (100, 74), 38: (100, 142), 39: (100, 82), 43: (100, 122), 44: (100, 74), 45: (100, 74), 46: (100, 90), 47: (100, 82), 60: (100, 165)},
    11: {1: (100, 90), 2: (100, 135), 3: (100, 202), 4: (100, 70), 7: (100, 165), 8: (100, 95), 9: (100, 95), 11: (100, 129), 12: (100, 78), 15: (100, 116), 16: (100, 74), 17: (100, 86), 19: (100, 183), 20: (100, 86), 23: (100, 135), 24: (100, 90), 25: (100, 90), 28: (100, 135), 29: (100, 95), 30: (100, 95), 32: (100, 116), 33: (100, 145), 35: (100, 100), 38: (100, 129), 39: (100, 95), 43: (100, 122), 44: (100, 95), 48: (100, 129), 49: (100, 95), 50: (100, 93), 54: (100, 122), 55: (100, 95), 59: (100, 116), 60: (100, 95)},
    12: {1: (100, 78), 4: (100, 224), 6: (100, 452), 7: (100, 497), 8: (100, 74), 9: (100, 74), 10: (100, 86), 12: (100, 122), 15: (100, 224), 16: (100, 62), 17: (100, 74), 19: (100, 150), 23: (100, 267), 24: (100, 100), 32: (100, 247), 33: (100, 318), 34: (100, 135), 43: (100, 247), 44: (100, 122), 48: (100, 165), 49: (100, 116), 54: (100, 157), 55: (100, 122), 59: (100, 150), 60: (100, 122)},
    13: {1: (15, 74), 4: (15, 74), 6: (36, 74), 7: (20, 74), 8: (21, 74), 10: (17, 74), 11: (24, 74), 12: (23, 74), 14: (20, 102), 15: (34, 116), 16: (31, 140), 17: (23, 165), 18: (23, 224), 19: (37, 352), 20: (39, 86), 21: (29, 102), 22: (40, 118), 23: (34, 134), 24: (34, 150), 25: (35, 178), 26: (36, 205), 27: (43, 232), 28: (36, 260), 29: (46, 95), 30: (45, 108), 31: (60, 122), 32: (64, 135), 33: (68, 174), 34: (56, 213), 35: (66, 225), 36: (50, 236), 37: (68, 248), 38: (67, 260), 39: (81, 90), 40: (95, 100), 41: (91, 111), 42: (129, 121), 43: (71, 132), 44: (70, 142), 45: (102, 148), 46: (60, 154), 48: (54, 165), 49: (79, 82), 50: (63, 90), 51: (105, 98), 52: (70, 106), 53: (95, 114), 54: (60, 122), 57: (63, 134), 58: (90, 138), 59: (54, 142), 60: (95, 82)},
    14: {1: (15, 100), 5: (15, 100), 7: (25, 100), 8: (23, 100), 10: (33, 100), 11: (57, 100), 13: (46, 100), 14: (55, 100), 17: (60, 100), 18: (40, 100), 19: (34, 100), 21: (61, 100), 22: (55, 100), 23: (83, 100), 24: (95, 100), 27: (82, 100), 28: (70, 100), 29: (78, 100), 30: (118, 100), 31: (88, 100), 32: (96, 100), 33: (57, 100), 34: (66, 100), 35: (88, 100), 36: (62, 100), 38: (74, 100), 39: (66, 100), 40: (83, 100), 41: (63, 100), 42: (64, 100), 43: (124, 100), 44: (83, 100), 45: (85, 100), 46: (279, 100), 47: (195, 100), 49: (246, 100), 50: (195, 100), 52: (176, 100), 53: (207, 100), 54: (137, 100), 55: (174, 100), 58: (172, 100), 59: (110, 100), 60: (176, 100)},
}


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def place_text(tbl, row):
    return json.dumps({k: row[k] for k in PLACES[tbl]}, sort_keys=True, separators=(",", ":"))


def row_of(db, tbl, place):
    where = " AND ".join(f"{c} = ?" for c in place)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(place.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def plan(db):
    """The changes [(table, place, row before or None, row after, why)] and the rows left, with why."""
    changes, kept = [], []
    for tbl in ("balancelevel", "balancecon", "balanceclass"):
        if not has_table(db, tbl):
            kept.append((tbl, "no such table: a server of 2026-10-29 or later makes it"))
    if kept:
        return changes, kept

    level_cols = [r[1] for r in db.execute("PRAGMA table_info(balancelevel)")]
    missing = [c for c in LEVEL_COLS if c not in level_cols]
    if missing:
        return changes, [("balancelevel", f"no column {', '.join(missing)}: a server of 2026-10-29 or later adds it")]
    have = {r["level"]: r for r in (dict(zip(level_cols, v)) for v in db.execute("SELECT * FROM balancelevel"))}
    if not have:
        return changes, [("balancelevel", "no rows: a server of 2026-10-29 or later seeds them")]
    after = {l: dict(r) for l, r in have.items()}  # the table as this run leaves it, for the rows it adds
    for level, old in LEVELS_0130.items():
        r = have.get(level)
        if r is None:
            kept.append((f"balancelevel {level}", "no such row: a GM took it out"))
            continue
        want = dict(r)
        for col, by in LEVEL_WANT.items():
            want[col] = float(by[level])
        if level in FLOOR_LEVELS:
            want["damage_pct"] = float(floor_damage(level))
            if not r["note"]:
                want["note"] = FLOOR_NOTE
        now = dict(zip(LEVEL_COLS, old))
        if all(r[c] == want[c] for c in LEVEL_COLS + ("note",)):
            continue
        if any(r[c] != now[c] for c in LEVEL_COLS):
            kept.append((f"balancelevel {level}", f"{ {c: r[c] for c in LEVEL_COLS if r[c] != now[c]} }, not the 01:30 seed's: a GM's"))
            continue
        after[level] = want
        changes.append(("balancelevel", {"level": level}, r, want, WHY))
    added = {json.loads(p)["level"] for (p,) in db.execute(f"SELECT place FROM {LOG} WHERE tbl = 'balancelevel' AND before IS NULL")} \
        if has_table(db, LOG) else set()
    for level in FLOOR_LEVELS:
        if level in LEVELS_0130:
            continue
        if level in have:
            if level not in added:
                kept.append((f"balancelevel {level}", "a row is there already: a GM's"))
            continue
        row = {"level": level, **{c: float(line(after, level, c)) for c in level_cols if c not in ("level", "note", "source")}}
        row.update(damage_pct=float(floor_damage(level)), note=FLOOR_NOTE, source="seed")
        changes.append(("balancelevel", {"level": level}, None, row, WHY))

    for diff, (now, want) in CONS.items():
        r = row_of(db, "balancecon", {"diff": diff})
        if r is None:
            kept.append((f"balancecon {diff}", "no such row: a GM took it out"))
            continue
        have = (r["damage_pct"], r["taken_pct"], r["root_resist_pct"], r["root_break_pct"])
        if have == want:
            continue
        if have != now:
            kept.append((f"balancecon {diff}", f"{have}, not {now}: a GM's"))
            continue
        changes.append(("balancecon", {"diff": diff}, r, dict(r, damage_pct=float(want[0]), taken_pct=float(want[1]),
                                                              root_resist_pct=float(want[2]), root_break_pct=float(want[3])), WHY))

    for cls, now in CLASSES.items():
        r = row_of(db, "balanceclass", {"class": cls})
        if r is None:
            kept.append((f"balanceclass {cls}", "no such row: a GM took it out (100 without it)"))
            continue
        if r["melee_taken_pct"] == 100:
            continue
        if r["melee_taken_pct"] != now:
            kept.append((f"balanceclass {cls}", f"{r['melee_taken_pct']}, not {now}: a GM's"))
            continue
        changes.append(("balanceclass", {"class": cls}, r, dict(r, melee_taken_pct=100.0), WHY))

    exists = has_table(db, "balanceclasslevel")
    if exists and "pet_hp_pct" not in [r[1] for r in db.execute("PRAGMA table_info(balanceclasslevel)")]:
        kept.append(("balanceclasslevel", "no column pet_hp_pct: a server of 2026-10-30 or later adds it"))
        return changes, kept
    for cls, by in CLASSLEVELS.items():
        for level, (melee, pet) in by.items():
            r = row_of(db, "balanceclasslevel", {"class": cls, "level": level}) if exists else None
            if r is not None:
                if (r["melee_taken_pct"], r["pet_hp_pct"]) != (melee, pet):
                    kept.append((f"balanceclasslevel {cls} {level}", f"{r['melee_taken_pct']}/{r['pet_hp_pct']}: a GM's"))
                continue
            changes.append(("balanceclasslevel", {"class": cls, "level": level}, None,
                            {"class": cls, "level": level, "melee_taken_pct": float(melee), "pet_hp_pct": float(pet),
                             "note": "", "source": "seed"}, WHY))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-balancetune", 1
    while os.path.exists(dst):  # never over another copy (an --undo the same second as its --apply)
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-balancetune"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, place, row, insert=False):
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
        return
    where = " AND ".join(f"{c} = ?" for c in place)
    if row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(place.values()))
        return
    cols = [c for c in row if c not in place]
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
               [row[c] for c in cols] + list(place.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        if not has_table(db, "balanceclasslevel"):
            # made here: logged, so --undo takes the table out again once it is empty
            db.execute(CLASSLEVEL_SCHEMA)
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, NULL, ?, ?, ?)",
                       ("balanceclasslevel", MADE, MADE, "made the table: a server of 2026-10-30 or later reads it", when))
        for tbl, place, before, after, why in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, place_text(tbl, after), None if before is None else json.dumps(before, sort_keys=True),
                        json.dumps(after, sort_keys=True), why, when))
            write_row(db, tbl, place, after, insert=before is None)
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
            if tbl not in PLACES:
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            if place == MADE:
                if db.execute(f"SELECT count(*) FROM {tbl}").fetchone()[0] == 0:
                    db.execute(f"DROP TABLE {tbl}")
                else:
                    print(f"  {tbl}: rows were added since, so the table this made is kept")
                continue
            p = json.loads(place)
            now = row_of(db, tbl, p)
            if now != json.loads(after):
                print(f"  {tbl} {place}: changed since ({'gone' if now is None else 'not as this left it'}): left as it is")
                continue
            write_row(db, tbl, p, None if before is None else json.loads(before))
            back += 1
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    rows = sum(1 for r in logged if r[2] != MADE)
    print(f"put back {back} of {rows} rows; the server takes them at its next start or a Balance page's next save")


def describe(change):
    tbl, place, before, after, why = change
    if before is None:
        cols = [c for c in after if c not in place and c not in ("note", "source")]
        return f"{tbl} {json.dumps(place, sort_keys=True)}: added, " + ", ".join(f"{c} {after[c]:g}" for c in cols)
    diff = ", ".join(f"{c} {before[c]!r} -> {after[c]!r}" for c in after if after[c] != before[c])
    return f"{tbl} {json.dumps(place, sort_keys=True)}: {diff}"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the tuning (logged)")
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
