#!/usr/bin/env python3
"""The balance re-fitted after EQOAEmu's spell workbook went in (2026-11-01; the user, asked whether the workbook's
combat numbers replace ours: "Workbook wins, re-fit"): its spells, effects, masteries and pets in the place of ours
(tools/import_eqoaemu_spells.py, import_eqoaemu_cms.py, import_eqoaemu_pets.py), and the balance tables fitted again
to the user's targets of 2026-10-30, which the four tunings before had met on our own spells
(claude-notes/balance-20261029.md).

What the workbook moved, measured with the fight simulator on eqoaclassic's database of 07:15 UTC 2026-11-01 with
the whole import in it (cmd/balancesim against SOE's roamer, the player's low point averaged over the fights won one
and two up; the tables as the four tunings left them):

  the melee classes and the tanks got stronger (the buff lines stack by the workbook's stack types, its heals and
  procs land): a warrior, a paladin, a shadow knight, a bard and a rogue kept 40-65% of their hit points through a
  yellow where the targets want a quarter, and a paladin of 37-60 and a bard of 13-59 won nearly every red three up
  alone; a monk got weaker (20-29 and 40-60 lose a yellow two up more often than not);
  the priests out-healed their reds: a druid, a shaman and a cleric of 10-50 won 70-100% of their fights three up
  alone, in two to eight minutes;
  the pets went over the top: a workbook pet fights as a player of its kind with the workbook's weapon and armour
  (petstats.go), and balancelevel's pet columns (pet_hp_pct 100-540, pet_damage_pct 225-560) had been fitted for the
  server's own pets, which hit with a creature's roll: a magician's and a necromancer's pet won every fight to three
  up alone keeping 60-99% of its hit points from level 8, in 6-30 s; an enchanter's lost every fight at 6-23;
  a rooting wizard of 45-60 lost every fight: the workbook's roots end at a hit (DisspellOnContact), and the
  simulator's root play recast Arcane Shackles at every nuke (balancesim.go now roots with one that holds, from
  within its reach); and once the simulator's kits carry their hands (integ 835efef), a kit armed by score had a
  rogue's backstabs, a paladin's strikes, a bard's and a ranger's blades and a ranger's arrows refused for the
  wrong weapon (balancekit.go now arms a kit for its arts).

It writes, each change only while the row holds what the four tunings left it (the post-import database has them so;
a row a GM changed is left, and said):

  balancelevel       pet_hp_pct and pet_damage_pct 100 at every row (PET_LEVEL): a workbook pet's hits land as a
                     player's do, its hit points the workbook's times its owner's class and level share (below)
  balanceclasslevel  every class's rows (REFIT), fitted by the simulator at every level 1-60 as the first tunings
                     were (tools/retune_balance.py and tools/tune_pets.py name the method): melee_taken_pct to both
                     yellows at once in the class's own play (by hand, a wizard's and an alchemist's root), the
                     low point averaged over one and two up near 25%, one up won 80% (a healer 90%, then eased 8%),
                     two up 65%; dealt_pct where no share met both win rates; a red three up won alone over 40%
                     made a loss by the share times 1.15^k; pet_hp_pct for every summoner's pet (a shaman's and a
                     wizard's too) on the pet the book gives, near a quarter over both yellows; and levels 1-10
                     against the low-level check (even fights won 95%, blue and green 97%, by hand AND in the
                     class's own play, in a new character's starting gear at 1-5). Levels 1-12 are each a row, above
                     them each curve the fewest rows within 4% of every level's fit (the pets' 7%); dealt_pct exact.
                     Six cells set after, each a row: a shadow knight of 5 at 95 (the user's fire bat fight, Dust's,
                     won with 15% left), a magician of 4 dealing 144 with its pet at 100 (at 216 its nukes pulled the
                     creature off its pet), an alchemist of 6 at 35 (its even fights by hand at the fit's edge), a
                     druid of 5 and 10 at 59 and 39 and a magician of 44's pet at 44 (a red won alone 42-57% at the
                     fit's rows). Not
                     met by any row: a rooting wizard's or alchemist's yellows are won as often as asked but leave it
                     40-60% when won (won with the root held, lost when it breaks), and a shaman of 51-54 takes 4-5x
                     an NPC's melee (Avatar's 1500 a tick out-heals any yellow); an enchanter of 57's pet wins 65-72%
                     of its reds alone at any share that wins its yellows (five-minute fights its owner's spells end).

Every change is logged in balance_eqoaemu_log (the table, the row's key, the whole row before and after: NULL for one
added or taken out, why); --undo puts back each row still as this left it and drops the log; a second --apply only
does what is still to do. Both copy the database first (<db>.bak-<when>-pre-eqoaemurefit) unless --no-backup. Report
only by default. Nothing is done on a database the four tunings have not been applied to (tools/tune_balance.py,
retune_balance.py, tune_pets.py, fit_pet_selfbuffs.py first). The server reads the balance tables at its start and
at once at a save of any Balance page in the GM panel; a pet takes its numbers at its next summoning. Apply it with
the workbook's import (its spells and pets): the rows are fitted on them, and on the server's spells and pets
without them every class is far off its targets again. internal/database/balanceseed.go holds the same rows (a
table made new).

    tools/refit_eqoaemu_balance.py [--db eqoa.db] [--apply | --undo] [--no-backup]
"""
import argparse
import datetime
import json
import math
import os
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fit_pet_selfbuffs as petself  # noqa: E402  the magician's 34-38 as the pets' fit again left them
import retune_balance as retune  # noqa: E402  the re-fit of 2026-10-30, and balanceclasslevel's order
import tune_pets as pets  # noqa: E402  the pet classes' rows over it

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
LOG = "balance_eqoaemu_log"
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,   -- balancelevel | balanceclasslevel
    place   TEXT NOT NULL,   -- the row's key, JSON
    before  TEXT,            -- the whole row before, JSON; NULL: added
    after   TEXT,            -- the whole row after, JSON; NULL: taken out
    why     TEXT NOT NULL,
    applied TEXT NOT NULL
)"""
CL = retune.TBL  # balanceclasslevel
WHY = "the balance re-fitted after EQOAEmu's workbook (tools/refit_eqoaemu_balance.py; 2026-11-01, \"Workbook wins, re-fit\")"

# balancelevel's pet columns as the four tunings left them (tools/tune_balance.py's), level -> (pet_hp_pct,
# pet_damage_pct), and the re-fit's at every row
PET_LEVEL_BEFORE = {
    1: (100.0, 250.0), 2: (100.0, 253.75), 3: (100.0, 257.5), 4: (100.0, 261.25), 5: (100.0, 265.0),
    6: (100.0, 263.3333333333333), 7: (100.0, 261.6666666666667), 8: (100.0, 260.0), 9: (120.0, 242.5),
    10: (140.0, 225.0), 11: (143.0, 236.0), 12: (146.0, 247.0), 13: (149.0, 258.0), 14: (152.0, 269.0),
    15: (155.0, 280.0), 18: (155.0, 300.0), 25: (190.0, 285.0), 30: (205.0, 300.0), 35: (250.0, 340.0),
    45: (375.0, 430.0), 50: (450.0, 475.0), 60: (540.0, 560.0),
}
PET_LEVEL = (100.0, 100.0)

# balanceclasslevel: class -> {level: (melee_taken_pct, pet_hp_pct, dealt_pct)}, the re-fit (see the doc)
REFIT = {
    0: {1: (95, 100, 100), 2: (66, 100, 100), 3: (105, 100, 100), 4: (117, 100, 100), 5: (100, 100, 100), 6: (95, 100, 100), 7: (86, 100, 100), 8: (70, 100, 100), 9: (51, 100, 100), 10: (46, 100, 100), 11: (52, 100, 100), 12: (59, 100, 100), 13: (67, 100, 100), 14: (67, 100, 100), 15: (76, 100, 100), 17: (76, 100, 100), 19: (55, 100, 100), 20: (97, 100, 100), 21: (76, 100, 100), 23: (80, 100, 100), 24: (97, 100, 100), 25: (97, 100, 100), 29: (59, 100, 100), 30: (85, 100, 100), 32: (76, 100, 100), 33: (91, 100, 100), 34: (80, 100, 100), 35: (97, 100, 100), 37: (91, 100, 100), 38: (97, 100, 100), 42: (97, 100, 100), 43: (110, 100, 100), 44: (110, 100, 100), 45: (97, 100, 100), 46: (97, 100, 100), 48: (67, 100, 100), 49: (80, 100, 100), 50: (76, 100, 100), 52: (110, 100, 100), 53: (85, 100, 100), 54: (97, 100, 100), 55: (97, 100, 100), 56: (85, 100, 100), 57: (117, 100, 100), 59: (97, 100, 100), 60: (97, 100, 100)},
    1: {1: (34, 100, 100), 2: (42, 100, 115), 3: (51, 100, 100), 4: (70, 100, 100), 5: (63, 100, 100), 6: (63, 100, 100), 7: (63, 100, 100), 8: (54, 100, 100), 9: (42, 100, 115), 10: (25, 100, 100), 11: (25, 100, 100), 12: (25, 100, 100), 13: (36, 100, 100), 14: (40, 100, 100), 15: (38, 100, 100), 16: (25, 100, 100), 19: (25, 100, 100), 20: (32, 100, 100), 22: (19, 100, 100), 24: (52, 100, 100), 25: (36, 100, 100), 26: (32, 100, 100), 27: (22, 100, 100), 28: (25, 100, 100), 29: (22, 100, 100), 30: (55, 100, 100), 31: (67, 100, 100), 33: (52, 100, 100), 34: (55, 100, 100), 35: (40, 100, 100), 36: (49, 100, 100), 37: (46, 100, 100), 38: (52, 100, 100), 39: (40, 100, 100), 40: (52, 100, 100), 41: (36, 100, 100), 42: (40, 100, 100), 43: (55, 100, 100), 45: (34, 100, 100), 46: (36, 100, 100), 48: (34, 100, 100), 49: (52, 100, 100), 50: (32, 100, 100), 51: (40, 100, 100), 52: (36, 100, 100), 53: (67, 100, 100), 54: (32, 100, 100), 55: (34, 100, 100), 56: (52, 100, 100), 57: (43, 100, 100), 58: (40, 100, 100), 59: (32, 100, 100), 60: (140, 100, 100)},
    2: {1: (78, 100, 100), 2: (63, 100, 100), 3: (94, 100, 100), 4: (78, 100, 100), 5: (123, 100, 100), 6: (117, 100, 100), 7: (151, 100, 100), 8: (149, 100, 100), 9: (105, 100, 100), 10: (93, 100, 100), 11: (100, 100, 100), 12: (124, 100, 100), 13: (113, 100, 100), 15: (117, 100, 100), 16: (91, 100, 100), 17: (90, 100, 100), 18: (88, 100, 115), 19: (67, 100, 100), 20: (110, 100, 100), 21: (91, 100, 100), 22: (100, 100, 100), 23: (85, 100, 100), 24: (117, 100, 100), 25: (97, 100, 100), 26: (97, 100, 100), 27: (110, 100, 100), 29: (97, 100, 100), 30: (110, 100, 100), 34: (110, 100, 100), 35: (91, 100, 100), 36: (91, 100, 100), 37: (113, 100, 100), 38: (124, 100, 100), 39: (117, 100, 100), 41: (132, 100, 100), 42: (120, 100, 100), 44: (140, 100, 100), 45: (140, 100, 100), 46: (124, 100, 100), 47: (132, 100, 100), 48: (110, 100, 100), 49: (140, 100, 100), 50: (140, 100, 100), 52: (159, 100, 100), 53: (140, 100, 100), 54: (140, 100, 100), 55: (159, 100, 100), 56: (132, 100, 100), 57: (159, 100, 100), 58: (169, 100, 100), 59: (204, 100, 100), 60: (214, 100, 100)},
    3: {1: (95, 100, 100), 2: (63, 100, 100), 3: (78, 100, 100), 4: (78, 100, 100), 5: (95, 100, 100), 6: (111, 100, 100), 7: (111, 100, 100), 8: (95, 100, 100), 9: (86, 100, 100), 10: (63, 100, 100), 11: (59, 100, 100), 12: (85, 100, 100), 13: (76, 100, 100), 14: (85, 100, 100), 15: (132, 100, 100), 17: (110, 100, 100), 18: (110, 100, 115), 19: (83, 100, 100), 20: (217, 100, 100), 21: (182, 100, 100), 23: (180, 100, 100), 24: (198, 100, 100), 25: (180, 100, 100), 28: (175, 100, 100), 29: (97, 100, 100), 31: (140, 100, 100), 32: (140, 100, 100), 33: (124, 100, 100), 34: (132, 100, 100), 35: (110, 100, 100), 36: (117, 100, 100), 37: (110, 100, 100), 38: (124, 100, 100), 40: (117, 100, 100), 41: (140, 100, 100), 43: (150, 100, 100), 45: (124, 100, 100), 47: (124, 100, 100), 48: (103, 100, 100), 49: (204, 100, 100), 50: (204, 100, 100), 51: (224, 100, 100), 52: (220, 100, 100), 54: (169, 100, 100), 56: (180, 100, 100), 57: (159, 100, 100), 58: (192, 100, 100), 59: (204, 100, 100), 60: (246, 100, 100)},
    4: {1: (78, 100, 100), 2: (66, 100, 115), 3: (94, 100, 100), 4: (165, 100, 200), 5: (86, 100, 120), 6: (70, 100, 100), 7: (78, 100, 100), 8: (87, 100, 150), 9: (23, 100, 100), 10: (18, 100, 100), 11: (25, 100, 100), 12: (25, 100, 100), 13: (59, 100, 100), 14: (46, 100, 100), 15: (80, 100, 100), 16: (72, 100, 100), 17: (63, 100, 115), 18: (49, 100, 130), 19: (35, 100, 100), 20: (49, 100, 100), 23: (46, 100, 100), 26: (59, 100, 100), 27: (52, 100, 100), 29: (55, 100, 100), 30: (76, 100, 100), 32: (80, 100, 100), 33: (124, 100, 115), 34: (97, 100, 100), 35: (110, 100, 100), 36: (103, 100, 100), 38: (117, 100, 100), 40: (97, 100, 100), 41: (103, 100, 100), 44: (80, 100, 100), 45: (103, 100, 100), 46: (100, 100, 100), 47: (113, 100, 100), 48: (85, 100, 100), 49: (117, 100, 100), 50: (91, 100, 100), 51: (124, 100, 100), 52: (124, 100, 100), 53: (164, 100, 100), 55: (124, 100, 100), 56: (140, 100, 100), 57: (136, 100, 100), 59: (97, 100, 100), 60: (246, 100, 100)},
    5: {1: (34, 100, 100), 2: (51, 100, 100), 3: (51, 100, 100), 4: (95, 100, 115), 5: (74, 100, 100), 6: (86, 100, 100), 7: (100, 100, 100), 8: (60, 100, 100), 9: (34, 100, 100), 10: (23, 100, 100), 11: (52, 100, 100), 12: (40, 100, 100), 13: (54, 100, 100), 14: (67, 100, 100), 15: (67, 100, 100), 16: (91, 100, 100), 17: (88, 100, 100), 19: (52, 100, 100), 20: (97, 100, 100), 21: (85, 100, 100), 22: (85, 100, 100), 23: (110, 100, 100), 25: (140, 100, 100), 26: (117, 100, 100), 27: (110, 100, 100), 29: (110, 100, 100), 30: (124, 100, 100), 32: (124, 100, 100), 33: (97, 100, 100), 35: (85, 100, 100), 36: (85, 100, 100), 37: (103, 100, 100), 38: (110, 100, 100), 40: (110, 100, 100), 41: (124, 100, 100), 42: (103, 100, 100), 43: (117, 100, 100), 44: (97, 100, 100), 45: (97, 100, 100), 46: (110, 100, 100), 47: (103, 100, 100), 48: (110, 100, 100), 52: (110, 100, 100), 53: (97, 100, 100), 54: (97, 100, 100), 55: (78, 100, 100), 56: (97, 100, 100), 57: (80, 100, 100), 58: (91, 100, 100), 59: (71, 100, 100), 60: (76, 100, 100)},
    6: {1: (51, 100, 100), 2: (42, 100, 100), 3: (51, 100, 100), 4: (65, 100, 100), 5: (60, 100, 100), 6: (51, 100, 100), 7: (54, 100, 100), 8: (66, 100, 150), 9: (18, 100, 100), 10: (17, 100, 100), 11: (16, 100, 100), 12: (25, 100, 100), 13: (28, 100, 100), 14: (36, 100, 100), 15: (32, 100, 100), 16: (19, 100, 100), 17: (25, 100, 100), 18: (19, 100, 115), 19: (19, 100, 100), 20: (32, 100, 100), 21: (25, 100, 100), 22: (28, 100, 100), 23: (25, 100, 100), 24: (52, 100, 100), 25: (36, 100, 100), 26: (25, 100, 100), 27: (19, 100, 100), 28: (25, 100, 100), 31: (25, 100, 100), 32: (28, 100, 100), 33: (25, 100, 100), 34: (32, 100, 100), 35: (32, 100, 100), 36: (25, 100, 100), 37: (36, 100, 100), 38: (32, 100, 100), 39: (25, 100, 100), 40: (46, 100, 100), 41: (49, 100, 100), 42: (46, 100, 100), 43: (32, 100, 100), 44: (36, 100, 100), 45: (52, 100, 100), 46: (40, 100, 100), 47: (40, 100, 100), 48: (32, 100, 100), 49: (67, 100, 100), 50: (52, 100, 100), 52: (52, 100, 100), 53: (59, 100, 100), 54: (40, 100, 100), 55: (52, 100, 100), 57: (40, 100, 100), 58: (40, 100, 100), 59: (19, 100, 100), 60: (46, 100, 100)},
    7: {1: (16, 100, 100), 2: (32, 100, 130), 3: (45, 100, 100), 4: (55, 100, 100), 5: (59, 100, 100), 6: (63, 100, 100), 7: (78, 100, 100), 8: (57, 100, 100), 9: (34, 100, 100), 10: (39, 100, 100), 11: (48, 100, 100), 12: (89, 100, 100), 13: (90, 100, 100), 15: (107, 100, 100), 16: (90, 100, 100), 17: (89, 100, 100), 19: (70, 100, 100), 20: (79, 100, 100), 21: (61, 100, 100), 22: (79, 100, 100), 23: (61, 100, 100), 24: (89, 100, 100), 25: (79, 100, 100), 28: (70, 100, 100), 29: (61, 100, 100), 30: (79, 100, 100), 32: (79, 100, 100), 33: (89, 100, 100), 34: (89, 100, 100), 35: (79, 100, 100), 36: (79, 100, 100), 37: (70, 100, 100), 38: (89, 100, 100), 39: (79, 100, 100), 40: (86, 100, 100), 41: (75, 100, 100), 42: (101, 100, 100), 44: (101, 100, 100), 45: (61, 100, 100), 46: (79, 100, 100), 47: (79, 100, 100), 48: (61, 100, 100), 49: (89, 100, 100), 50: (42, 100, 100), 51: (42, 100, 100), 53: (54, 100, 100), 54: (29, 100, 100), 55: (31, 100, 100), 56: (37, 100, 100), 57: (29, 100, 100), 58: (42, 100, 100), 59: (33, 100, 100), 60: (67, 100, 100)},
    8: {1: (17, 284, 100), 2: (31, 284, 130), 3: (63, 284, 100), 4: (63, 284, 100), 5: (78, 284, 100), 6: (78, 284, 100), 7: (95, 284, 100), 8: (117, 284, 100), 9: (78, 284, 100), 10: (78, 284, 100), 11: (79, 284, 100), 12: (107, 284, 100), 13: (101, 328, 100), 14: (114, 372, 100), 16: (95, 460, 100), 17: (101, 504, 100), 19: (70, 794, 100), 20: (84, 401, 100), 22: (79, 476, 100), 23: (95, 546, 100), 24: (86, 616, 100), 26: (78, 755, 100), 27: (74, 772, 100), 28: (79, 789, 100), 29: (57, 142, 100), 30: (81, 100, 100), 31: (89, 110, 100), 32: (89, 119, 100), 33: (89, 140, 100), 34: (95, 160, 100), 35: (95, 180, 100), 36: (74, 201, 100), 37: (79, 206, 100), 38: (101, 212, 100), 39: (101, 126, 100), 40: (70, 84, 100), 41: (79, 90, 100), 44: (79, 106, 100), 45: (79, 118, 100), 46: (61, 130, 100), 47: (61, 142, 100), 48: (89, 142, 100), 49: (102, 100, 100), 50: (114, 106, 100), 51: (476, 113, 100), 52: (421, 106, 100), 53: (418, 126, 100), 54: (301, 126, 100), 55: (114, 151, 100), 56: (114, 177, 100), 57: (101, 202, 100), 58: (114, 228, 100), 59: (101, 253, 100), 60: (129, 126, 100)},
    9: {1: (17, 100, 100), 2: (27, 100, 130), 3: (38, 100, 100), 4: (138, 100, 100), 5: (143, 100, 100), 6: (117, 100, 100), 7: (176, 100, 100), 8: (117, 100, 100), 9: (78, 100, 100), 10: (63, 100, 100), 11: (79, 100, 100), 12: (129, 100, 100), 13: (114, 100, 100), 14: (121, 100, 100), 15: (114, 100, 100), 16: (89, 100, 100), 17: (89, 100, 100), 19: (70, 100, 100), 20: (89, 100, 100), 21: (79, 100, 100), 23: (70, 100, 100), 26: (89, 100, 100), 27: (70, 100, 100), 28: (79, 100, 100), 30: (84, 100, 100), 32: (101, 100, 100), 33: (76, 100, 100), 34: (84, 100, 100), 35: (79, 100, 100), 36: (58, 100, 100), 38: (79, 100, 100), 40: (84, 100, 100), 42: (76, 100, 100), 43: (89, 100, 100), 44: (79, 100, 100), 45: (79, 100, 100), 46: (89, 100, 100), 47: (79, 100, 100), 48: (89, 100, 100), 49: (89, 100, 100), 50: (81, 100, 100), 51: (101, 100, 100), 53: (107, 100, 100), 55: (101, 100, 100), 57: (61, 100, 100), 58: (129, 100, 100), 59: (61, 100, 100), 60: (129, 100, 100)},
    10: {1: (15, 71, 400), 2: (17, 159, 600), 3: (16, 300, 360), 4: (19, 100, 144), 5: (18, 126, 130), 6: (26, 159, 100), 7: (32, 126, 100), 8: (29, 84, 100), 9: (39, 100, 100), 10: (20, 119, 100), 11: (100, 142, 100), 12: (100, 159, 100), 15: (100, 284, 100), 16: (100, 63, 100), 19: (100, 126, 100), 20: (100, 100, 100), 23: (100, 113, 100), 24: (100, 84, 100), 26: (100, 90, 100), 28: (100, 113, 100), 29: (100, 100, 100), 30: (100, 113, 100), 31: (100, 100, 100), 33: (100, 159, 100), 34: (100, 56, 100), 35: (100, 71, 100), 36: (100, 63, 100), 37: (100, 80, 100), 43: (100, 80, 100), 44: (100, 44, 100), 45: (100, 90, 100), 46: (100, 29, 100), 47: (100, 63, 100), 49: (100, 71, 100), 56: (100, 71, 100), 60: (100, 90, 100)},
    11: {1: (15, 284, 600), 2: (15, 327, 500), 3: (22, 450, 300), 4: (17, 169, 180), 5: (18, 253, 108), 6: (28, 318, 100), 7: (32, 401, 100), 8: (32, 126, 100), 9: (40, 159, 100), 10: (17, 201, 100), 11: (100, 253, 100), 12: (100, 142, 100), 15: (100, 225, 100), 16: (100, 142, 100), 17: (100, 159, 100), 19: (100, 284, 100), 20: (100, 142, 100), 23: (100, 212, 100), 24: (100, 95, 100), 25: (100, 80, 100), 28: (100, 113, 100), 30: (100, 80, 100), 31: (100, 80, 100), 33: (100, 113, 100), 35: (100, 80, 100), 38: (100, 95, 100), 40: (100, 71, 100), 41: (100, 71, 100), 43: (100, 84, 100), 44: (100, 63, 100), 48: (100, 80, 100), 49: (100, 56, 100), 50: (100, 53, 100), 54: (100, 71, 100), 55: (100, 63, 100), 59: (100, 100, 100), 60: (100, 75, 100)},
    12: {1: (15, 142, 600), 2: (15, 126, 600), 3: (16, 201, 360), 4: (22, 401, 216), 5: (17, 635, 130), 6: (26, 755, 100), 7: (29, 794, 100), 8: (24, 71, 100), 9: (26, 100, 100), 10: (16, 126, 115), 11: (100, 159, 100), 12: (100, 199, 100), 15: (100, 318, 100), 16: (100, 80, 100), 17: (100, 90, 100), 18: (100, 126, 100), 19: (100, 201, 100), 23: (100, 284, 100), 24: (100, 71, 100), 28: (100, 100, 100), 32: (100, 159, 100), 33: (100, 201, 100), 34: (100, 63, 100), 38: (100, 80, 100), 43: (100, 142, 100), 44: (100, 48, 100), 48: (100, 53, 100), 49: (100, 39, 100), 50: (100, 37, 100), 54: (100, 39, 100), 55: (100, 33, 100), 56: (100, 33, 100), 57: (100, 38, 100), 58: (100, 33, 100), 60: (100, 31, 100)},
    13: {1: (19, 100, 100), 2: (23, 100, 300), 3: (35, 100, 180), 4: (31, 100, 115), 5: (44, 100, 130), 6: (67, 100, 130), 7: (26, 100, 115), 8: (72, 100, 150), 9: (83, 100, 150), 10: (26, 100, 115), 11: (25, 100, 100), 12: (22, 100, 100), 13: (25, 100, 100), 14: (25, 100, 100), 15: (49, 100, 100), 16: (180, 100, 150), 17: (126, 100, 130), 18: (16, 100, 115), 19: (24, 100, 100), 20: (32, 100, 100), 21: (23, 100, 100), 22: (33, 100, 100), 23: (28, 100, 100), 24: (27, 100, 100), 25: (19, 100, 100), 26: (25, 100, 100), 27: (25, 100, 100), 28: (22, 100, 100), 29: (31, 100, 100), 30: (36, 100, 100), 31: (33, 100, 100), 32: (45, 100, 100), 33: (52, 100, 100), 34: (67, 100, 100), 35: (71, 100, 100), 36: (59, 100, 100), 37: (78, 100, 100), 38: (61, 100, 100), 39: (97, 100, 100), 40: (154, 100, 100), 42: (180, 100, 100), 44: (91, 100, 100), 46: (98, 100, 100), 47: (80, 100, 100), 48: (74, 100, 100), 49: (127, 100, 100), 50: (106, 100, 100), 51: (70, 100, 100), 52: (59, 100, 100), 53: (102, 100, 100), 54: (83, 100, 100), 56: (69, 100, 100), 57: (57, 100, 100), 58: (61, 100, 100), 59: (41, 100, 100), 60: (136, 100, 100)},
    14: {1: (17, 100, 300), 2: (15, 100, 300), 3: (16, 100, 300), 4: (23, 100, 180), 5: (25, 100, 130), 6: (35, 100, 130), 7: (62, 100, 200), 8: (18, 100, 130), 9: (67, 100, 175), 10: (15, 100, 115), 11: (20, 100, 100), 12: (28, 100, 100), 13: (25, 100, 100), 15: (37, 100, 100), 16: (39, 100, 100), 17: (47, 100, 100), 18: (28, 100, 100), 19: (27, 100, 100), 20: (55, 100, 100), 22: (49, 100, 100), 24: (92, 100, 100), 25: (71, 100, 100), 26: (74, 100, 100), 27: (60, 100, 100), 28: (67, 100, 100), 29: (61, 100, 100), 30: (93, 100, 100), 31: (66, 100, 100), 32: (97, 100, 100), 33: (58, 100, 100), 34: (55, 100, 100), 35: (78, 100, 100), 36: (62, 100, 100), 37: (63, 100, 100), 38: (76, 100, 100), 39: (55, 100, 100), 40: (55, 100, 100), 41: (62, 100, 100), 42: (82, 100, 100), 43: (86, 100, 100), 45: (68, 100, 100), 46: (180, 100, 100), 47: (180, 100, 100), 48: (204, 100, 100), 50: (189, 100, 100), 52: (140, 100, 100), 53: (172, 100, 100), 54: (136, 100, 100), 56: (210, 100, 100), 57: (157, 100, 100), 58: (173, 100, 100), 59: (214, 100, 100), 60: (169, 100, 100)},
}


def before_rows():
    """balanceclasslevel as the four tunings left it: the re-fit of 2026-10-30, the pet classes' rows of the pets' fit
    over it, the magician's 34-38 of the pets' fit again"""
    out = {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in retune.RETUNED.items()}
    for c, by in pets.PETS.items():
        out[c] = {l: tuple(map(float, v)) for l, v in by.items()}
    for level, (old, new) in petself.CLASS_LEVELS.items():
        out[petself.MAGICIAN][level] = new
    return out


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def columns(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def row_of(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def cl_values(r):
    return None if r is None else (float(r["melee_taken_pct"]), float(r["pet_hp_pct"]), float(r["dealt_pct"]))


def same(a, b):
    """two rows' figures alike (a seed's thirds read back from the database)"""
    if a is None or b is None:
        return a is b
    return len(a) == len(b) and all(math.isclose(x, y, rel_tol=1e-9, abs_tol=1e-9) for x, y in zip(a, b))


def after_rows():
    return {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in REFIT.items()}


def tunings_done(db):
    """Whether the four tunings are what this goes from: tools/fit_pet_selfbuffs.py's log (or this tool's), or the
    magician's rows at 34 and 38 as it left them."""
    if has_table(db, petself.LOG) or has_table(db, LOG):
        return True
    ends = [cl_values(row_of(db, CL, {"class": petself.MAGICIAN, "level": level})) for level in (34, 38)]
    return same(ends[0], petself.CLASS_LEVELS[34][1]) and same(ends[1], petself.CLASS_LEVELS[38][1])


def is_refit(db):
    """Whether balanceclasslevel is the re-fit's rows already, every one (a table made new: the seeds are them)."""
    have = {(c, l): (float(m), float(p), float(d)) for c, l, m, p, d in
            db.execute(f"SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM {CL}")}
    want = {(c, l): v for c, by in after_rows().items() for l, v in by.items()}
    return set(have) == set(want) and all(same(have[k], want[k]) for k in want)


def plan(db):
    """The changes [(table, key, row before or None, row after or None)] and the rows left, with why."""
    changes, kept = [], []
    if not REFIT:
        return changes, [("everything", "no re-fit in this copy of the tool (REFIT is empty)")]
    if not has_table(db, CL) or "dealt_pct" not in columns(db, CL) or not has_table(db, "balancelevel"):
        return changes, [(CL, "no such table or no dealt_pct: tools/tune_balance.py and tools/retune_balance.py first")]
    if not tunings_done(db) and not is_refit(db):
        return changes, [("everything", "not the state the four tunings leave: tools/tune_balance.py, "
                                        "retune_balance.py, tune_pets.py and fit_pet_selfbuffs.py first")]
    was, want = before_rows(), after_rows()
    places = [(c, l) for c in sorted(set(was) | set(want)) for l in sorted(set(was.get(c, {})) | set(want.get(c, {})))]
    now = {p: cl_values(row_of(db, CL, {"class": p[0], "level": p[1]})) for p in places}
    for c, l in places:
        old, new = was.get(c, {}).get(l), want.get(c, {}).get(l)
        if same(old, new):
            continue
        have = now[(c, l)]
        if same(have, new):
            continue  # done already
        if not same(have, old):
            kept.append((f"{CL} {c} {l}", f"{have if have else 'no row'}, not the tunings' {old if old else 'no row'}: a GM's"))
            continue
        r = row_of(db, CL, {"class": c, "level": l})
        after = None
        if new is not None:
            after = dict(r) if r else {"class": c, "level": l, "note": "", "source": "seed"}
            after.update({"melee_taken_pct": new[0], "pet_hp_pct": new[1], "dealt_pct": new[2]})
        changes.append((CL, {"class": c, "level": l}, r, after))
    for level, old in sorted(PET_LEVEL_BEFORE.items()):
        r = row_of(db, "balancelevel", {"level": level})
        if r is None:
            kept.append((f"balancelevel {level}", "no such row: a GM took it out"))
            continue
        have = (float(r["pet_hp_pct"]), float(r["pet_damage_pct"]))
        if same(have, PET_LEVEL):
            continue
        if not same(have, old):
            kept.append((f"balancelevel {level}", f"pet_hp_pct/pet_damage_pct {have[0]:g}/{have[1]:g}, not the "
                                                  f"tunings' {old[0]:g}/{old[1]:g}: a GM's"))
            continue
        changes.append(("balancelevel", {"level": level}, r, dict(r, pet_hp_pct=PET_LEVEL[0], pet_damage_pct=PET_LEVEL[1])))
    return changes, kept


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst, n = f"{path}.bak-{when}-pre-eqoaemurefit", 1
    while os.path.exists(dst):
        n += 1
        dst = f"{path}.bak-{when}-{n}-pre-eqoaemurefit"
    shutil.copy2(path, dst)
    return dst


def write_row(db, tbl, key, row, insert):
    where = " AND ".join(f"{k} = ?" for k in key)
    if insert:
        cols = list(row)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    elif row is None:
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
    else:
        cols = [c for c in row if c not in key]
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in cols)} WHERE {where}",
                   [row[c] for c in cols] + list(key.values()))


def apply(db, changes):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for tbl, key, before, after in changes:
            db.execute(f"INSERT INTO {LOG} (tbl, place, before, after, why, applied) VALUES (?, ?, ?, ?, ?, ?)",
                       (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                        None if after is None else json.dumps(after, sort_keys=True), WHY, when))
            write_row(db, tbl, key, after, insert=before is None)
        retune.reorder(db)
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
            if tbl not in (CL, "balancelevel"):
                raise SystemExit(f"{LOG} names table {tbl!r}, which this tool never writes")
            key = json.loads(place)
            now = row_of(db, tbl, key)
            if now != (None if after is None else json.loads(after)):
                print(f"  {tbl} {place}: changed since: left as it is")
                continue
            write_row(db, tbl, key, None if before is None else json.loads(before), insert=now is None)
            back += 1
        retune.reorder(db)
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"put back {back} of {len(logged)} rows; the server takes them at its next start or a Balance page's next save")


def describe(change):
    tbl, key, before, after = change
    k = " ".join(str(v) for v in key.values())
    if tbl == "balancelevel":
        return (f"balancelevel {k}: pet_hp_pct {before['pet_hp_pct']:g} -> {after['pet_hp_pct']:g}, pet_damage_pct "
                f"{before['pet_damage_pct']:g} -> {after['pet_damage_pct']:g}")
    if before is None:
        return (f"{tbl} class {key['class']} level {key['level']}: added, melee_taken_pct {after['melee_taken_pct']:g}, "
                f"pet_hp_pct {after['pet_hp_pct']:g}, dealt_pct {after['dealt_pct']:g}")
    if after is None:
        return (f"{tbl} class {key['class']} level {key['level']}: taken out (was {before['melee_taken_pct']:g}/"
                f"{before['pet_hp_pct']:g}/{before['dealt_pct']:g})")
    diff = [f"{c} {before[c]:g} -> {after[c]:g}" for c in ("melee_taken_pct", "pet_hp_pct", "dealt_pct") if before[c] != after[c]]
    return f"{tbl} class {key['class']} level {key['level']}: " + ", ".join(diff)


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
          "Balance page in the GM panel (a pet at its next summoning)")


if __name__ == "__main__":
    main()
