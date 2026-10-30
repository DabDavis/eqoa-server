#!/usr/bin/env python3
"""The re-fit of 2026-10-30: Balance by class and level moved from the rows tools/tune_balance.py left (live on
eqoaclassic since 00:27 UTC) to the rows the simulator fits on eqoaclassic's data of 04:20 with SOE's roamers in the
creatures (tools/seed_roamers.py; claude-notes/balance-20261029.md).

Two things moved under the first tuning. The kits: by 04:20 fix_armor_class.py had moved 320 items' armour class out
of a stat it had been written into, fix_reward_class_masks.py --all-rewards had given 187 quest rewards their quests'
classes, and the trash items, the class quest scrolls and items and Sandstorm's spell visuals had gone in; a monk of
5 then won a yellow two up 31% of the time where the first tuning had it at 70% (its kit lost an item of +27 STR).
And the creatures: the user's "SOE had most mobs as warrior class, but stripped down to just a 'crit' attack once
every 30 secs or so ... they get it around level 10 starting": every hostile warrior or classless creature fights by
the roamer template (a Critical Hit of 7.4 a level, fixed and through any armour, every 25-35 s of its time at its
enemy from 10-20 s in, from level 10), and wisps by the wisp template (jib's wizard at 28% of a player's figures).

The fit is the first tuning's (cmd/balancesim on that data, the foe a roamer: each class's share of an NPC's melee at
every level 1-60, to both yellows at once: the player's low point averaged over the fights won one and two up near
25%, one up won at least 80% of the time (a healer 90%, its share then eased 8%), two up 65%; level 1 to one up
alone). Levels 1-7 come out as they were without the crit (a level 7's two up is a level 9: no crit); from 8 the
crit takes the shares down to 0.3-0.9 of what they were. Where no share met both win rates even at the floor (15%),
the class's own damage is let land more (dealt_pct, the column of 2026-10-30): 16 cells, a rooting wizard of 8-18
(115-130%: out of power in a 95-120 s fight, the root off and the crits of its last 40 s in reach finished it), an
alchemist of 4, 5 and 8 (115-130%) and a rogue of 54, 59 and 60 (115%). A share low enough for a healer's yellows
let it out-heal a red three up in a long fight (a druid of 14 won 100% of them in about three minutes): at the 14
levels where a class won a red alone more than 40% of the time (13 a priest's, a ranger of 31) the share is raised
by 15-50% to where it no longer does, both yellows still won. Levels 1-12 are each a row of their own (a small hit
rounds), those 14 too, and each class's curve above 12 is cut to its fewest rows within 4% of every level; dealt_pct
is exact at every level. The pets' hit points (pet_hp_pct, by the owner's class and level) are fitted the same way
against the roamer on the pet's play (the pet's hit points left near a quarter over both yellows, one up won 80%,
two up 65%), the necromancer's with the spectre's instant (pet_special_hits): a pet's hit points dwarf a crit, so
they moved little but for the necromancer's spectre levels (44-54: 15-20% fewer, as its instant kills sooner).

Then levels 1-10 against the coordinator's low-level check (jib: "jibrush got beat up by a level 1 while he was
level 3 alc"; a new alchemist hit a 335-hit-point bat for 2 every 3.5 s): every class wins its even fights about 95%
of the time and blue and green ones about all of it, BY HAND as well as in its own play, in a new character's
starting gear at 1-5 (its start weapon, no armour). Melee classes and tanks already did. Where a class did not, its
fights were too LONG for its hands, so its own damage lands more there, tapering (at most 40% less a level): an
alchemist of 1-5 300/300/200/130/115%, a magician of 1-4 400/400/240/144%, a necromancer of 1-4 300/300/180/108%,
an enchanter of 1-5 400/400/360/216/130% (and 115% at 10), with the share it takes at 15-40% (a pet class's
melee share was 100: it fights with its pet, but a new one has none yet); their pets' hit points re-fitted on top
(an owner whose nukes land four times pulls the creature off its pet: a magician of 1 at 15%). A wizard of 2 takes
15%, a druid of 1 16% and of 3 40%. A pet class's level 11 keeps its old row, so nothing above 10 moves. One cell is
short: an enchanter of 2 by hand wins 92% of its even fights at any dealt (the NPC of 2 hits 317% and its first
blows kill a 25-hit-point caster). The yellows keep the tier's kit (the fire bat's calibration is Dust's own gear),
and every class's own play still wins them, a priest of 1 and the casters before their root aside: RETUNED.

It writes only balanceclasslevel: a row the first tuning left and the re-fit does not keep is taken out, a row the
re-fit adds is added where none is, a row both have is changed, each only while it holds what the first tuning
left (a row a GM changed is left, and said); the dealt_pct column is added when the table lacks it, as a server of
2026-10-30 adds it at its start (a row of the first tuning deals all: 100). Every change is logged in
balance_retune_log (the row's place, the whole row before and after: NULL for a row added or taken out, why);
--undo puts back each row still as this left it (the column stays, at 100 on those rows) and drops the log; a second
--apply only does what is still to do. Both copy the database first (<db>.bak-<when>-pre-balanceretune) unless
--no-backup. Report only by default. tools/tune_balance.py first: on a database it has not been applied to this does
nothing.

The rows are read by a server of 2026-10-30 with dealt_pct (an older one reads the shares and leaves dealt_pct out,
so the 16 cells' classes deal no more); it reads the balance tables at its start and at once whenever a GM saves a
row of any Balance page in the panel. Apply it with tools/seed_roamers.py: the shares from 8 are fitted with the
roamers' crits, so without them a yellow from 8 is easier than the targets.

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
WHY = "the re-fit of 2026-10-30, the 04:20 data with the roamers (tools/retune_balance.py; claude-notes/balance-20261029.md)"

# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct, dealt_pct)}, the re-fit (see the doc)
RETUNED = {
    0: {1: (95, 100, 100), 2: (63, 100, 100), 3: (95, 100, 100), 4: (100, 100, 100), 5: (95, 100, 100), 6: (86, 100, 100), 7: (79, 100, 100), 8: (66, 100, 100), 9: (51, 100, 100), 10: (46, 100, 100), 11: (46, 100, 100), 12: (51, 100, 100), 14: (63, 100, 100), 17: (63, 100, 100), 18: (57, 100, 100), 19: (46, 100, 100), 20: (86, 100, 100), 21: (63, 100, 100), 22: (63, 100, 100), 23: (57, 100, 100), 24: (70, 100, 100), 25: (70, 100, 100), 27: (57, 100, 100), 28: (57, 100, 100), 29: (46, 100, 100), 30: (57, 100, 100), 31: (51, 100, 100), 32: (51, 100, 100), 33: (46, 100, 100), 34: (51, 100, 100), 35: (63, 100, 100), 36: (63, 100, 100), 37: (70, 100, 100), 44: (70, 100, 100), 45: (63, 100, 100), 47: (63, 100, 100), 48: (51, 100, 100), 51: (70, 100, 100), 52: (82, 100, 100), 53: (77, 100, 100), 55: (77, 100, 100), 56: (70, 100, 100), 57: (86, 100, 100), 58: (90, 100, 100), 59: (74, 100, 100), 60: (66, 100, 100)},
    1: {1: (51, 100, 100), 2: (42, 100, 100), 3: (63, 100, 100), 4: (65, 100, 100), 5: (57, 100, 100), 6: (63, 100, 100), 7: (57, 100, 100), 8: (20, 100, 100), 9: (18, 100, 100), 10: (16, 100, 100), 11: (15, 100, 100), 12: (23, 100, 100), 13: (31, 100, 100), 14: (34, 100, 100), 15: (34, 100, 100), 16: (23, 100, 100), 17: (23, 100, 100), 18: (20, 100, 100), 19: (23, 100, 100), 20: (34, 100, 100), 21: (23, 100, 100), 22: (25, 100, 100), 23: (42, 100, 100), 24: (63, 100, 100), 25: (42, 100, 100), 26: (34, 100, 100), 27: (23, 100, 100), 29: (23, 100, 100), 30: (57, 100, 100), 31: (39, 100, 100), 32: (34, 100, 100), 33: (34, 100, 100), 34: (57, 100, 100), 35: (34, 100, 100), 37: (34, 100, 100), 38: (51, 100, 100), 39: (34, 100, 100), 40: (57, 100, 100), 41: (38, 100, 100), 42: (34, 100, 100), 43: (42, 100, 100), 44: (42, 100, 100), 45: (23, 100, 100), 46: (28, 100, 100), 47: (28, 100, 100), 48: (25, 100, 100), 49: (38, 100, 100), 50: (28, 100, 100), 51: (34, 100, 100), 52: (25, 100, 100), 53: (34, 100, 100), 54: (23, 100, 100), 55: (23, 100, 100), 56: (34, 100, 100), 57: (34, 100, 100), 59: (23, 100, 100), 60: (42, 100, 100)},
    2: {1: (95, 100, 100), 2: (66, 100, 100), 3: (86, 100, 100), 4: (95, 100, 100), 5: (136, 100, 100), 6: (136, 100, 100), 7: (143, 100, 100), 8: (111, 100, 100), 9: (95, 100, 100), 10: (86, 100, 100), 11: (70, 100, 100), 12: (79, 100, 100), 14: (77, 100, 100), 15: (63, 100, 100), 19: (38, 100, 100), 20: (63, 100, 100), 21: (57, 100, 100), 23: (57, 100, 100), 24: (70, 100, 100), 25: (70, 100, 100), 26: (57, 100, 100), 27: (77, 100, 100), 28: (77, 100, 100), 29: (63, 100, 100), 30: (63, 100, 100), 31: (77, 100, 100), 34: (70, 100, 100), 35: (57, 100, 100), 36: (51, 100, 100), 38: (63, 100, 100), 39: (66, 100, 100), 42: (63, 100, 100), 43: (70, 100, 100), 46: (63, 100, 100), 47: (66, 100, 100), 48: (51, 100, 100), 49: (70, 100, 100), 50: (66, 100, 100), 51: (86, 100, 100), 52: (70, 100, 100), 54: (74, 100, 100), 56: (63, 100, 100), 57: (77, 100, 100), 58: (82, 100, 100), 59: (70, 100, 100), 60: (129, 100, 100)},
    3: {1: (95, 100, 100), 2: (66, 100, 100), 3: (77, 100, 100), 4: (77, 100, 100), 5: (86, 100, 100), 6: (86, 100, 100), 7: (95, 100, 100), 8: (60, 100, 100), 9: (42, 100, 100), 10: (42, 100, 100), 11: (42, 100, 100), 12: (51, 100, 100), 15: (57, 100, 100), 16: (51, 100, 100), 17: (51, 100, 100), 18: (38, 100, 100), 19: (38, 100, 100), 20: (95, 100, 100), 21: (77, 100, 100), 22: (70, 100, 100), 23: (70, 100, 100), 24: (77, 100, 100), 25: (100, 100, 100), 27: (77, 100, 100), 28: (82, 100, 100), 29: (70, 100, 100), 30: (77, 100, 100), 31: (77, 100, 100), 32: (86, 100, 100), 33: (77, 100, 100), 34: (95, 100, 100), 35: (74, 100, 100), 37: (77, 100, 100), 38: (100, 100, 100), 40: (86, 100, 100), 41: (105, 100, 100), 42: (105, 100, 100), 43: (117, 100, 100), 44: (95, 100, 100), 46: (105, 100, 100), 47: (100, 100, 100), 48: (70, 100, 100), 50: (117, 100, 100), 51: (129, 100, 100), 52: (105, 100, 100), 53: (136, 100, 100), 55: (129, 100, 100), 56: (95, 100, 100), 57: (136, 100, 100), 58: (159, 100, 100), 60: (105, 100, 100)},
    4: {1: (77, 100, 100), 2: (66, 100, 100), 3: (86, 100, 100), 4: (95, 100, 100), 5: (74, 100, 100), 6: (69, 100, 100), 7: (82, 100, 100), 8: (38, 100, 100), 9: (34, 100, 100), 10: (23, 100, 100), 11: (34, 100, 100), 12: (34, 100, 100), 13: (77, 100, 100), 14: (63, 100, 100), 15: (86, 100, 100), 17: (63, 100, 100), 18: (46, 100, 100), 19: (42, 100, 100), 20: (95, 100, 100), 21: (77, 100, 100), 22: (70, 100, 100), 23: (95, 100, 100), 26: (95, 100, 100), 27: (86, 100, 100), 28: (86, 100, 100), 30: (105, 100, 100), 32: (105, 100, 100), 33: (136, 100, 100), 34: (117, 100, 100), 35: (117, 100, 100), 36: (129, 100, 100), 39: (143, 100, 100), 41: (143, 100, 100), 42: (117, 100, 100), 43: (129, 100, 100), 44: (117, 100, 100), 47: (185, 100, 100), 48: (129, 100, 100), 49: (176, 100, 100), 50: (143, 100, 100), 51: (185, 100, 100), 52: (185, 100, 100), 53: (216, 100, 100), 54: (185, 100, 100), 56: (216, 100, 100), 59: (176, 100, 100), 60: (279, 100, 100)},
    5: {1: (51, 100, 100), 2: (42, 100, 100), 3: (51, 100, 100), 4: (95, 100, 100), 5: (86, 100, 100), 6: (77, 100, 100), 7: (86, 100, 100), 8: (54, 100, 100), 9: (42, 100, 100), 10: (34, 100, 100), 11: (15, 100, 100), 12: (34, 100, 100), 15: (34, 100, 100), 16: (23, 100, 100), 17: (28, 100, 100), 18: (18, 100, 100), 19: (20, 100, 100), 20: (38, 100, 100), 21: (34, 100, 100), 22: (34, 100, 100), 23: (51, 100, 100), 24: (51, 100, 100), 25: (57, 100, 100), 26: (51, 100, 100), 27: (34, 100, 100), 28: (42, 100, 100), 29: (42, 100, 100), 30: (51, 100, 100), 32: (51, 100, 100), 33: (42, 100, 100), 35: (42, 100, 100), 36: (51, 100, 100), 38: (51, 100, 100), 39: (46, 100, 100), 40: (51, 100, 100), 41: (63, 100, 100), 42: (51, 100, 100), 44: (51, 100, 100), 45: (57, 100, 100), 46: (51, 100, 100), 47: (54, 100, 100), 48: (49, 100, 100), 49: (60, 100, 100), 50: (51, 100, 100), 51: (51, 100, 100), 52: (42, 100, 100), 53: (49, 100, 100), 54: (34, 100, 100), 55: (34, 100, 100), 56: (42, 100, 100), 57: (34, 100, 100), 58: (34, 100, 100), 59: (20, 100, 100), 60: (42, 100, 100)},
    6: {1: (51, 100, 100), 2: (42, 100, 100), 3: (46, 100, 100), 4: (57, 100, 100), 5: (60, 100, 100), 6: (51, 100, 100), 7: (63, 100, 100), 8: (20, 100, 100), 9: (18, 100, 100), 10: (16, 100, 100), 11: (15, 100, 100), 12: (23, 100, 100), 13: (34, 100, 100), 15: (34, 100, 100), 16: (23, 100, 100), 17: (28, 100, 100), 19: (18, 100, 100), 20: (23, 100, 100), 21: (18, 100, 100), 22: (23, 100, 100), 23: (20, 100, 100), 24: (34, 100, 100), 27: (17, 100, 100), 28: (18, 100, 100), 29: (23, 100, 100), 30: (25, 100, 100), 31: (20, 100, 100), 32: (23, 100, 100), 33: (18, 100, 100), 34: (23, 100, 100), 35: (25, 100, 100), 36: (23, 100, 100), 37: (25, 100, 100), 39: (20, 100, 100), 40: (23, 100, 100), 41: (28, 100, 100), 42: (28, 100, 100), 44: (23, 100, 100), 45: (18, 100, 100), 48: (18, 100, 100), 50: (28, 100, 100), 51: (31, 100, 100), 52: (20, 100, 100), 53: (31, 100, 100), 54: (16, 100, 115), 55: (15, 100, 100), 56: (18, 100, 100), 57: (16, 100, 100), 58: (25, 100, 100), 59: (18, 100, 115), 60: (15, 100, 115)},
    7: {1: (16, 100, 100), 2: (17, 100, 100), 3: (40, 100, 100), 4: (47, 100, 100), 5: (47, 100, 100), 6: (54, 100, 100), 7: (71, 100, 100), 8: (47, 100, 100), 9: (31, 100, 100), 10: (31, 100, 100), 11: (39, 100, 100), 12: (79, 100, 100), 13: (47, 100, 100), 14: (34, 100, 100), 15: (40, 100, 100), 16: (39, 100, 100), 18: (24, 100, 100), 19: (21, 100, 100), 20: (47, 100, 100), 21: (39, 100, 100), 22: (47, 100, 100), 23: (31, 100, 100), 24: (42, 100, 100), 25: (47, 100, 100), 26: (47, 100, 100), 27: (35, 100, 100), 28: (31, 100, 100), 29: (58, 100, 100), 31: (79, 100, 100), 33: (58, 100, 100), 35: (64, 100, 100), 36: (55, 100, 100), 37: (52, 100, 100), 38: (58, 100, 100), 39: (79, 100, 100), 40: (87, 100, 100), 41: (87, 100, 100), 42: (71, 100, 100), 44: (73, 100, 100), 45: (58, 100, 100), 47: (66, 100, 100), 48: (58, 100, 100), 49: (83, 100, 100), 50: (29, 100, 100), 51: (26, 100, 100), 53: (33, 100, 100), 54: (21, 100, 100), 55: (21, 100, 100), 56: (23, 100, 100), 57: (21, 100, 100), 58: (31, 100, 100), 59: (27, 100, 100), 60: (55, 100, 100)},
    8: {1: (17, 113, 100), 2: (17, 113, 100), 3: (47, 113, 100), 4: (58, 113, 100), 5: (58, 113, 100), 6: (64, 113, 100), 7: (71, 113, 100), 8: (64, 113, 100), 9: (52, 113, 100), 10: (47, 113, 100), 11: (52, 113, 100), 12: (71, 113, 100), 14: (71, 144, 100), 15: (36, 159, 100), 17: (61, 225, 100), 18: (36, 300, 100), 19: (39, 424, 100), 20: (58, 113, 100), 21: (47, 132, 100), 22: (44, 151, 100), 23: (47, 170, 100), 24: (47, 189, 100), 26: (47, 249, 100), 28: (31, 309, 100), 29: (64, 119, 100), 30: (64, 127, 100), 31: (58, 134, 100), 32: (58, 142, 100), 34: (58, 206, 100), 35: (64, 238, 100), 36: (47, 250, 100), 37: (47, 263, 100), 38: (59, 275, 100), 39: (71, 106, 100), 40: (47, 114, 100), 43: (48, 136, 100), 44: (39, 144, 100), 45: (47, 151, 100), 47: (39, 166, 100), 48: (50, 174, 100), 49: (79, 95, 100), 50: (71, 101, 100), 51: (146, 107, 100), 52: (132, 113, 100), 53: (154, 119, 100), 54: (97, 126, 100), 55: (119, 132, 100), 56: (125, 138, 100), 57: (108, 144, 100), 58: (108, 150, 100), 59: (108, 150, 100), 60: (146, 95, 100)},
    9: {1: (17, 100, 100), 2: (17, 100, 100), 3: (39, 100, 100), 4: (119, 100, 100), 5: (132, 100, 100), 6: (108, 100, 100), 7: (132, 100, 100), 8: (79, 100, 100), 9: (64, 100, 100), 10: (58, 100, 100), 11: (71, 100, 100), 12: (102, 100, 100), 14: (71, 100, 100), 17: (71, 100, 100), 18: (31, 100, 100), 19: (31, 100, 100), 20: (52, 100, 100), 21: (36, 100, 100), 22: (31, 100, 100), 23: (31, 100, 100), 24: (42, 100, 100), 25: (41, 100, 100), 26: (47, 100, 100), 27: (31, 100, 100), 28: (36, 100, 100), 30: (58, 100, 100), 31: (47, 100, 100), 32: (68, 100, 100), 33: (47, 100, 100), 35: (55, 100, 100), 36: (39, 100, 100), 39: (58, 100, 100), 41: (49, 100, 100), 42: (55, 100, 100), 44: (58, 100, 100), 45: (54, 100, 100), 47: (47, 100, 100), 48: (54, 100, 100), 49: (68, 100, 100), 50: (47, 100, 100), 51: (64, 100, 100), 52: (58, 100, 100), 53: (70, 100, 100), 54: (71, 100, 100), 55: (47, 100, 100), 56: (58, 100, 100), 57: (47, 100, 100), 58: (82, 100, 100), 59: (47, 100, 100), 60: (108, 100, 100)},
    10: {1: (15, 63, 400), 2: (17, 53, 400), 3: (16, 159, 240), 4: (19, 60, 144), 5: (18, 90, 100), 6: (27, 119, 100), 7: (33, 159, 100), 8: (29, 113, 100), 9: (39, 134, 100), 10: (24, 150, 100), 11: (100, 179, 100), 12: (100, 80, 100), 15: (100, 142, 100), 16: (100, 100, 100), 17: (100, 113, 100), 19: (100, 212, 100), 20: (100, 113, 100), 23: (100, 169, 100), 24: (100, 80, 100), 25: (100, 90, 100), 28: (100, 159, 100), 29: (100, 100, 100), 32: (100, 142, 100), 33: (100, 195, 100), 34: (100, 106, 100), 38: (100, 169, 100), 39: (100, 100, 100), 43: (100, 142, 100), 44: (100, 90, 100), 46: (100, 100, 100), 47: (100, 90, 100), 52: (100, 134, 100), 60: (100, 179, 100)},
    11: {1: (18, 40, 300), 2: (15, 113, 300), 3: (22, 179, 180), 4: (17, 71, 108), 5: (18, 100, 100), 6: (28, 126, 100), 7: (32, 164, 100), 8: (32, 113, 100), 9: (40, 134, 100), 10: (17, 150, 100), 11: (100, 179, 100), 12: (100, 113, 100), 15: (100, 164, 100), 16: (100, 106, 100), 17: (100, 126, 100), 19: (100, 225, 100), 20: (100, 106, 100), 23: (100, 159, 100), 24: (100, 106, 100), 26: (100, 126, 100), 28: (100, 169, 100), 29: (100, 119, 100), 31: (100, 126, 100), 33: (100, 169, 100), 35: (100, 113, 100), 38: (100, 142, 100), 39: (100, 106, 100), 43: (100, 138, 100), 44: (100, 90, 100), 48: (100, 119, 100), 49: (100, 84, 100), 54: (100, 110, 100), 55: (100, 100, 100), 59: (100, 126, 100), 60: (100, 103, 100)},
    12: {1: (32, 40, 400), 2: (15, 100, 400), 3: (17, 142, 360), 4: (22, 179, 216), 5: (17, 318, 130), 6: (26, 450, 100), 7: (31, 566, 100), 8: (24, 90, 100), 9: (28, 106, 100), 10: (16, 126, 115), 11: (100, 150, 100), 15: (100, 300, 100), 16: (100, 84, 100), 17: (100, 100, 100), 19: (100, 189, 100), 23: (100, 318, 100), 24: (100, 119, 100), 32: (100, 292, 100), 33: (100, 347, 100), 34: (100, 159, 100), 43: (100, 268, 100), 44: (100, 142, 100), 48: (100, 179, 100), 49: (100, 130, 100), 54: (100, 174, 100), 55: (100, 126, 100), 59: (100, 159, 100), 60: (100, 130, 100)},
    13: {1: (19, 106, 100), 2: (15, 106, 100), 3: (19, 106, 100), 4: (20, 106, 100), 5: (27, 106, 100), 6: (36, 106, 100), 7: (20, 106, 100), 8: (16, 106, 115), 9: (18, 106, 130), 10: (19, 106, 130), 11: (21, 106, 115), 12: (28, 106, 130), 13: (24, 124, 130), 14: (18, 141, 115), 15: (36, 159, 100), 16: (32, 192, 115), 17: (35, 225, 130), 18: (23, 300, 130), 19: (22, 424, 100), 20: (32, 106, 100), 21: (21, 124, 100), 22: (28, 141, 100), 23: (27, 159, 100), 24: (29, 191, 100), 25: (24, 223, 100), 27: (29, 286, 100), 28: (26, 318, 100), 29: (33, 119, 100), 31: (35, 146, 100), 32: (50, 159, 100), 33: (38, 198, 100), 34: (39, 238, 100), 35: (51, 252, 100), 36: (37, 265, 100), 38: (48, 292, 100), 39: (74, 106, 100), 40: (91, 114, 100), 41: (79, 122, 100), 42: (115, 130, 100), 43: (91, 138, 100), 44: (51, 147, 100), 45: (54, 155, 100), 46: (74, 163, 100), 47: (72, 171, 100), 48: (42, 179, 100), 49: (83, 90, 100), 50: (68, 99, 100), 51: (63, 108, 100), 53: (105, 126, 100), 54: (52, 131, 100), 55: (77, 136, 100), 56: (73, 140, 100), 57: (57, 145, 100), 58: (77, 150, 100), 59: (51, 150, 100), 60: (82, 90, 100)},
    14: {1: (17, 100, 300), 2: (16, 100, 300), 3: (17, 100, 200), 4: (20, 100, 130), 5: (20, 100, 115), 6: (17, 100, 100), 7: (27, 100, 100), 8: (29, 100, 115), 9: (15, 100, 100), 10: (17, 100, 100), 11: (41, 100, 100), 12: (31, 100, 100), 13: (29, 100, 100), 14: (39, 100, 100), 16: (39, 100, 100), 17: (46, 100, 100), 18: (28, 100, 100), 19: (26, 100, 100), 20: (41, 100, 100), 21: (44, 100, 100), 22: (41, 100, 100), 24: (88, 100, 100), 25: (66, 100, 100), 27: (68, 100, 100), 28: (59, 100, 100), 29: (62, 100, 100), 30: (77, 100, 100), 31: (61, 100, 100), 32: (96, 100, 100), 33: (46, 100, 100), 34: (49, 100, 100), 35: (70, 100, 100), 36: (55, 100, 100), 37: (57, 100, 100), 38: (86, 100, 100), 39: (49, 100, 100), 41: (52, 100, 100), 42: (68, 100, 100), 43: (91, 100, 100), 44: (72, 100, 100), 45: (76, 100, 100), 46: (233, 100, 100), 47: (239, 100, 100), 48: (205, 100, 100), 50: (189, 100, 100), 52: (129, 100, 100), 53: (178, 100, 100), 54: (134, 100, 100), 55: (169, 100, 100), 56: (220, 100, 100), 57: (154, 100, 100), 59: (205, 100, 100), 60: (140, 100, 100)},
}


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def place_text(row):
    return json.dumps({"class": row["class"], "level": row["level"]}, sort_keys=True, separators=(",", ":"))


def row_of(db, cls, level):
    cur = db.execute(f"SELECT * FROM {TBL} WHERE class = ? AND level = ?", (cls, level))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


DEALT = "dealt_pct"  # added by a server of 2026-10-30 after 00:27 at its start, and by --apply when it is missing
DEALT_DECL = "REAL NOT NULL DEFAULT 100"


def values(r):
    return None if r is None else (r["melee_taken_pct"], r["pet_hp_pct"], r.get(DEALT, 100.0))


def has_dealt(db):
    return DEALT in [r[1] for r in db.execute(f"PRAGMA table_info({TBL})")]


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
            old = tuple(map(float, was[level])) + (100.0,) if level in was else None  # the first tuning: all dealt
            new = tuple(map(float, want[level])) if level in want else None
            if old == new:
                continue
            r = row_of(db, cls, level)
            if r is not None and DEALT not in r:
                r[DEALT] = 100.0  # a table before the column: all of it, as the server's own default
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
                after.update({"melee_taken_pct": new[0], "pet_hp_pct": new[1], DEALT: new[2]})
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
        if not has_dealt(db):  # as the server adds it; --undo leaves it (at 100 it changes nothing)
            db.execute(f"ALTER TABLE {TBL} ADD COLUMN {DEALT} {DEALT_DECL}")
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
        return (f"{TBL} class {cls} level {level}: added, melee_taken_pct {after['melee_taken_pct']:g}, pet_hp_pct "
                f"{after['pet_hp_pct']:g}, {DEALT} {after[DEALT]:g}")
    if after is None:
        return (f"{TBL} class {cls} level {level}: taken out (was {before['melee_taken_pct']:g}/{before['pet_hp_pct']:g}/"
                f"{before[DEALT]:g})")
    out = (f"{TBL} class {cls} level {level}: melee_taken_pct {before['melee_taken_pct']:g} -> {after['melee_taken_pct']:g}, "
           f"pet_hp_pct {before['pet_hp_pct']:g} -> {after['pet_hp_pct']:g}")
    if before[DEALT] != after[DEALT]:
        out += f", {DEALT} {before[DEALT]:g} -> {after[DEALT]:g}"
    return out


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
