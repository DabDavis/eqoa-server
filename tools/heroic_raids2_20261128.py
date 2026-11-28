#!/usr/bin/env python3
"""The heroic raids, phase 2: every other trigger raid of the open world as a heroic dungeon, its encounter a trigger
in each party's copy (Heirloom petition 3396, Tanker: "Please make all raids, yes all, have a heroic dungeon"; the
owner, 2026-11-28: "yes to the heroic dungeons for all raids", the trigger raids first, a raid-gear set a raid,
Heirloom only, no seasonal or Moogie events). The first, Kratas's Deep, is tools/heroic_kratas_20261128.py; this is
its pattern for ten more (RAIDS):

  ct          Cazic Thule and the Trials (world 0): the trial of celerity (Qual'Tin's), the Watchman, the
              phantasms of the lava pool and CazicThule, one run in order, each part waiting for a player near it
  thulite     the Thulite Golem on his anvil
  dynmath     Dynmath the Tyrant out of the Etheneric Portal, his dead called back while he lives
  parathior   Parathior down the mountain to the broken tower
  jarek       Jarek's Creation, which kills its maker (Jarek is not copied: a friendly; the popup tells it)
  zek         the Zek Sergeant, after three of his four troops die (the troops are a quest's seeded rows, so the
              run stands them up; no Portal of Tallon: it leads to world 5)
  jailor      Runnyeye's superior jailor after four pickclaw guards, then the superior head jailor
  valkanith   (world 1) the Tal'thex dead's waves on Envar's island, then Lord Valkanith
  sarek       (world 1) Sarek the Betrayer on Archon Aereth's floor (the party lands there: the tower's portals
              lead to world 1)
  voxraid     Enraged Lady Vox, in the vox dungeon's copies (its box and landing are the dungeon's own)

Portal of Zek is left out: no fight, only world 5's way in (world 5 comes later). So is the Khal hounds chain and
every encounter that is not a raid.

For each, as Kratas's:
  - npcs, world 14: a copy of each creature of the dungeon's box (tools/worlds/dungeons.json) of its kinds (FAMILY)
    or named (a capitalised name), never friendly (npc_type 0x80), spawn-only or a quest's seeded row, levels raised
    as the other heroic dungeons' (trash 60-62, named 63-64); npctemplate "#<row>" -> its tier; npcloot of the
    tier's tables. Vox's raid has none (its dungeon has them).
  - the trigger's creatures: a copy of each as a template OUTSIDE every box (so no instance's opening stands it
    up), its tier's level (boss 65, named 64, trash 61), template and loot; the trigger brings each by "#<row>".
  - gmtriggers 402-411, world 14: an awaitnear start (a player near the encounter's place in a party's copy), its
    lock (held per instance), a clock with an empty time (a wipe ends the run, so it starts again), the
    encounter, then a clock of 4 hours with no empty time and a wait as long: the boss rises once in a copy (its
    creatures never come back, as every heroic dungeon's); a party in one copy that long sees him again. Counts of
    waves are cut for a party of four (the dungeons' cap): the phantasms 27 -> 12 (at most 6 up), Valkanith's
    waves 7 x 6 -> 4 x 3, Dynmath's dead 8 -> 4; Parathior's light waits 60 s (300 in the open world).
  - itempattern 995157-995276: twelve raid-gear records a raid, each a copy of a raid boss's drop the other heroic
    dungeons give (BASES, rotated so each raid covers other classes and slots) under a name of its own (rent 1),
    loottables "dungeon <key> boss|named".
  - a Dungeon Warden twin beside each new dungeon's landing, a copy of Blackburrow's.

Needs tools/dungeons_20261112.py applied (the tier templates and the twins) and tools/heroic_kratas_20261128.py's
engine (build468): a shard without the dungeons (the Live Server) is refused, nothing written. The template rows
are checked by id AND name (Heirloom's ids; the Live Server's differ). The server reads npcs, templates, loot and
triggers at its start: run it in a keep restart's before.sh, with the world file of 24 zones (world 14's recipe of
2026-11-28 phase 2) and its ground in worlds/. Refused when applied already (a second run does nothing) or a row it
would write exists; --undo deletes every row it wrote, refused when one changed since (a GM's edit); exact. Report
only by default; --apply in one transaction, a gm_audit line a row, log heroic_raids2_1128_log.

    python3 -E -s tools/heroic_raids2_20261128.py --db eqoa.db [--apply | --undo] [--recipe tools/worlds/dungeons.json]
"""
import argparse
import importlib.util
import json
import math
import os
import re
import sqlite3
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("dungeons_base", os.path.join(HERE, "dungeons_20261112.py"))
base = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(base)

RUN, LOG = "heroic raids2 20261128", "heroic_raids2_1128_log"
DETAIL = ("Heirloom petition 3396, the owner 2026-11-28: the trigger raids as heroic dungeons (phase 2); "
          "tools/heroic_raids2_20261128.py")
WORLD = 14
ITEM_FIRST = 995157
WARDEN = base.WARDEN
TWIN_OF = (12586.0, 12760.0)  # Blackburrow's twin (world 14), whose row each new twin copies
TIER_LEVEL = {"boss": 65, "named": 64, "trash": 61}
TEMPLATE_OF = {"boss": "dungeon boss", "named": "dungeon named", "trash": "dungeon trash"}

# The raid-gear bases: the raid boss drops the heroic dungeons give (tools/dungeons_*.py, heroic_kratas), chests
# first (each class's), then the rest; raid i takes chests 5i.. and others 7i.. (mod their counts)
CHESTS = [5072, 6509, 716001, 6456, 718002, 6455, 6454, 6480, 718006, 1342, 6472, 6478, 6479, 6453, 6451, 6476,
          6477, 6474, 716005, 6475]
OTHERS = [713087, 961106, 715012, 961104, 713096, 5417, 5406, 713079, 5397, 961101, 6117, 715013, 961105, 5424,
          713073, 1060, 5418, 715011, 961103, 715015, 715014]
BOSS_PIECES = {0, 1, 2, 5, 6, 10}  # of a raid's twelve, these drop from the boss table; the rest from named


def bases(i):
    return [CHESTS[(5 * i + j) % len(CHESTS)] for j in range(5)] + [OTHERS[(7 * i + j) % len(OTHERS)] for j in range(7)]


def tail(boss, text):
    """A heroic run's end: the boss's death told, then the copy's rest (4 h, no empty time: he rises once in a copy);
    a wipe's clock (its empty time) goes straight to the end, so the run ends and starts again."""
    return [{"do": "awaitdead", "npc": boss},
            {"do": "broadcast", "text": text},
            {"do": "clock", "seconds": 14400},
            {"do": "wait", "seconds": 14400},
            {"do": "end"},
            {"do": "despawn"}]


def sp(t, x=None, y=None, z=None, **kw):
    s = {"do": "spawn", "npc": t}
    s.update(kw)
    if x is not None:
        s.update({"world": WORLD, "x": x, "y": y, "z": z})
    return s


def steps_ct(T):
    cel = (25690.0, 17.9, 3310.0)
    out = [{"do": "awaitnear", "start": True, "radius": 60},
           {"do": "lock", "lock": "heroic ct"},
           {"do": "clock", "seconds": 7200, "empty": 900},
           {"do": "popup", "text": "The trial of celerity begins. Cazic Thule's servants wake in the hall."}]
    for t, x, z in [("Contagion", 25664, 3307), ("Festering Jack", 25688, 3306), ("Zin the Desiccator", 25695, 3307),
                    ("Bloodletter", 25672, 3312), ("Bloodletter", 25678, 3312), ("Bloodletter", 25684, 3314),
                    ("Impale", 25690, 3316)]:
        out.append(sp(T[t], float(x), cel[1], float(z), spread=0.5, hostile=True, minutes=120))
    out += [sp(T["Qual'Tin"], 25679.0, cel[1], 3329.0, spread=0.5, attackable=True, minutes=120),
            {"do": "awaitdead", "npc": "Qual'Tin"},
            {"do": "broadcast", "text": "Qual'Tin falls. The trial of celerity is passed; the Watchman walks the hall above."},
            {"do": "place", "world": WORLD, "x": 25620.0, "y": 29.0, "z": 3150.0, "radius": 150},
            {"do": "awaitnear", "radius": 150},
            sp(T["The Watchman"], 25640.0, 28.9, 3147.0, spread=0.5, minutes=120, hostile=True,
               patrol=[[25640, 28.9, 3147], [25604, 28.9, 3144], [25604, 29.9, 3110], [25604, 29.9, 3080]],
               pingpong=True, pause=3),
            {"do": "awaitdead", "npc": "The Watchman"},
            {"do": "broadcast", "text": "The Watchman falls. The lava pool's chamber is open."},
            {"do": "place", "world": WORLD, "x": 25497.0, "y": 16.0, "z": 3097.0, "radius": 60},
            {"do": "awaitnear", "radius": 60},
            sp(T["enraged phantasm"], 25497.0, 16.0, 3097.0, count=3, spread=15, minutes=20),
            {"do": "popup", "text": "Enraged phantasms rise out of the lava pool. Their stare holds you where you stand."},
            {"do": "wait", "seconds": 8},
            {"do": "rouse", "npc": "enraged phantasm"}]
    for n in (2, 1, 2, 1, 2, 1):
        out += [{"do": "awaitdead", "npc": "enraged phantasm", "any": True},
                sp(T["enraged phantasm"], 25497.0, 16.0, 3097.0, count=n, spread=15, minutes=20, hostile=True, max=6)]
    out += [{"do": "awaitdead", "npc": "enraged phantasm"},
            {"do": "broadcast", "text": "The last phantasm sinks into the lava. Something stirs in the statue beyond."},
            {"do": "place", "world": WORLD, "x": 25421.0, "y": 18.0, "z": 3096.0, "radius": 80},
            {"do": "awaitnear", "radius": 80},
            sp(T["CazicThule"], 25421.0, 18.0, 3096.0, spread=0.5, hostile=True, minutes=60),
            {"do": "broadcast", "text": "CazicThule wakes in his estate."}]
    return out + tail("CazicThule", "CazicThule has fallen. His estate is silent.")


def steps_thulite(T):
    return [{"do": "awaitnear", "start": True, "radius": 60},
            {"do": "lock", "lock": "heroic thulite"},
            {"do": "clock", "seconds": 3600, "empty": 600},
            {"do": "popup", "text": "The anvil rings. The Thulite Golem stirs from the stone."},
            sp(T["Thulite Golem"], 18467.5, 69.6094, 25426.5, hostile=True, minutes=60, spread=0.5, face="north"),
            {"do": "broadcast", "text": "The Thulite Golem wakes on its anvil!"}] + \
        tail("Thulite Golem", "The Thulite Golem breaks apart on its anvil.")


def steps_dynmath(T):
    x, y, z = 4765.39, 58.95, 14028.43
    return [{"do": "awaitnear", "start": True, "radius": 60},
            {"do": "lock", "lock": "heroic dynmath"},
            {"do": "clock", "seconds": 3600, "empty": 600},
            {"do": "broadcast", "text": "The Etheneric Portal tears open. Dynmath the Tyrant strides through, his dead behind him."},
            sp(T["Dynmath the Tyrant"], x, y, z, hostile=True, spread=0.5, minutes=60),
            sp(T["minion of Dynmath"], x, y, z, hostile=True, count=4, spread=12, minutes=60, noloot=True),
            {"do": "resummon", "npc": "minion of Dynmath", "by": "Dynmath the Tyrant", "seconds": 15, "hostile": True,
             "minutes": 60, "text": "Dynmath the Tyrant calls another of his dead out of the portal."},
            {"do": "awaitdead", "npc": "Dynmath the Tyrant"},
            {"do": "despawn", "npc": "minion of Dynmath"}] + \
        tail("Dynmath the Tyrant", "Dynmath the Tyrant falls, and his dead crumble to dust.")[1:]


def steps_parathior(T):
    return [{"do": "awaitnear", "start": True, "radius": 80},
            {"do": "lock", "lock": "heroic parathior"},
            {"do": "clock", "seconds": 3600, "empty": 600},
            {"do": "popup", "text": "A ball of light kindles atop the broken tower. Make ready: in a minute Parathior will come down the mountain."},
            {"do": "wait", "seconds": 60},
            sp(T["Parathior"], 7035.0, 195.5, 4130.0, hostile=True, minutes=60, spread=1, face="south"),
            {"do": "walk", "npc": "Parathior", "radius": 200.0, "pace": 4.0, "x": 7035.4, "y": 76.2, "z": 4281.6},
            {"do": "broadcast", "text": "The light atop the broken tower goes out. Parathior comes walking down the mountain!"}] + \
        tail("Parathior", "Parathior falls at the foot of the broken tower.")


def steps_jarek(T):
    return [{"do": "awaitnear", "start": True, "radius": 45},
            {"do": "lock", "lock": "heroic jarek"},
            {"do": "clock", "seconds": 3600, "empty": 600},
            {"do": "popup", "text": "Jarek N`Toxis's last work stirs by his tent: crab and roach, bound by the hate of Innoruuk. It tore its maker apart, and now it turns to you."},
            {"do": "wait", "seconds": 5},
            sp(T["Jarek's Creation"], 24191.43, 54.13, 9579.13, hostile=True, minutes=60, spread=1)] + \
        tail("Jarek's Creation", "Jarek's Creation falls apart into crab and roach.")


# where the open world's four Zek troops stand (quest 111056's seeded rows, so not copied as the box's: the run
# stands them up)
ZEK_TROOPS = [(20250.0, 89.0, 18250.0), (20296.0, 91.0, 18315.0), (20280.0, 89.0, 18210.0), (20245.0, 67.0, 18108.0)]


def steps_zek(T):
    return [{"do": "awaitnear", "start": True, "radius": 60},
            {"do": "lock", "lock": "heroic zek"},
            {"do": "clock", "seconds": 3600, "empty": 600}] + \
        [sp(T["Zek troop"], x, y, z, spread=0.5, minutes=60) for x, y, z in ZEK_TROOPS] + \
        [{"do": "awaitkill", "npc": "Zek troop", "count": 3, "any": True, "seconds": 1800, "fail": "The Zek troops regroup."},
            sp(T["Zek Sergeant"], hostile=True, spread=2, minutes=60),
            {"do": "broadcast", "text": "A Zek Sergeant storms out to rally his troops!"}] + \
        tail("Zek Sergeant", "The Zek Sergeant falls, and the camp is broken.")


def steps_jailor(T):
    return [{"do": "awaitnear", "start": True, "radius": 60},
            {"do": "lock", "lock": "heroic jailor"},
            {"do": "clock", "seconds": 3600, "empty": 600},
            {"do": "awaitkill", "npc": "superior pickclaw guard", "seconds": 1800, "count": 4, "fail": "The pickclaw guards regroup."},
            {"do": "broadcast", "text": "Keys rattle in the cells: a superior jailor comes out to see to his guards!"},
            sp(T["superior jailor"], 17098.37, 5.03, 13250.46, spread=1, minutes=60, hostile=True),
            {"do": "awaitdead", "npc": "superior jailor", "seconds": 1860, "any": True, "fail": "The superior jailor goes back to his cells."},
            {"do": "wait", "seconds": 20},
            {"do": "broadcast", "text": "The superior head jailor storms out of the cells!"},
            sp(T["superior head jailor"], 17092.31, 5.03, 13246.96, spread=1, minutes=60, hostile=True)] + \
        tail("superior head jailor", "The superior head jailor falls. His keys are yours.")


def steps_valkanith(T):
    out = [{"do": "awaitnear", "start": True, "radius": 60},
           {"do": "lock", "lock": "heroic valkanith"},
           {"do": "clock", "seconds": 4800, "empty": 600},
           {"do": "broadcast", "text": "The Tal'thex wake! Their dead rise on Envar's island."}]
    for _ in range(4):
        out += [sp(T["Tal'thex knight"], hostile=True, minutes=60, spread=15.0),
                sp(T["Tal'thex soldier"], hostile=True, minutes=60, spread=15.0),
                sp(T["Tal'thex sorcerer"], hostile=True, minutes=60, spread=15.0),
                {"do": "awaitdead", "seconds": 240},
                {"do": "wait", "seconds": 20}]
    out += [sp(T["Tal'thex sorcerer"], hostile=True, minutes=60, spread=15.0),
            {"do": "awaitdead", "npc": "Tal'thex sorcerer", "any": True, "seconds": 900,
             "fail": "The Tal'thex dead sink back into the island. Lord Valkanith will not come."},
            {"do": "say", "npc": "Tal'thex sorcerer", "text": "Lord Valkanith... comes..."},
            sp(T["Lord Valkanith"], hostile=True, minutes=60, spread=5),
            {"do": "broadcast", "text": "Lord Valkanith has come to Envar's island."}]
    return out + tail("Lord Valkanith", "Lord Valkanith falls, and the Tal'thex dead lie still.")


def steps_sarek(T):
    return [{"do": "awaitnear", "start": True, "radius": 40},
            {"do": "lock", "lock": "heroic sarek"},
            {"do": "clock", "seconds": 3600, "empty": 600},
            {"do": "popup", "text": "The stone of the tower hums. Archon Aereth's floor is not his alone: make ready."},
            {"do": "wait", "seconds": 20},
            {"do": "hide", "npc": "Archon Aereth", "radius": 30, "minutes": 65},
            {"do": "wait", "seconds": 5},
            sp(T["Sarek the Betrayer"], 4700.0, 110.375, 2500.0, hostile=True, spread=0.5, minutes=65),
            {"do": "broadcast", "text": "Sarek the Betrayer: \"Arios sends me another of his pups? Then die in my tower.\""}] + \
        tail("Sarek the Betrayer", "Sarek the Betrayer falls, and the stone of the tower is still.") + \
        [{"do": "hide", "npc": "Archon Aereth", "radius": 30, "minutes": 0}]


def steps_voxraid(T):
    return [{"do": "awaitnear", "start": True, "radius": 60},
            {"do": "lock", "lock": "heroic enraged vox"},
            {"do": "clock", "seconds": 3600, "empty": 600},
            {"do": "popup", "text": "The ice of the lair cracks. Lady Vox's rage wakes."},
            sp(T["Enraged Lady Vox"], 4853.0, 112.7, 4410.3, hostile=True, minutes=60, spread=1),
            {"do": "broadcast", "text": "Enraged Lady Vox rises in her lair!"}] + \
        tail("Enraged Lady Vox", "Enraged Lady Vox falls. Permafrost is still.")


# Each raid: its dungeon (the recipe's key; voxraid's is vox's), source world, trash kinds, the trigger's creatures
# (name -> (Heirloom row, tier)), the trigger, the template spot, the twelve pieces' names.
RAIDS = [
    dict(key="ct", dungeon="ct", src=0, family=r"^(dread rot|virulent blade|dead rider)$", slot=402,
         trigger="Heroic Cazic Thule", at=(25690.0, 17.9, 3310.0), radius=300, steps=steps_ct, tat=(25100.0, 18.0, 2750.0),
         temps={"Contagion": (118877, "trash"), "Festering Jack": (118875, "trash"), "Zin the Desiccator": (118869, "trash"),
                "Bloodletter": (163618, "trash"), "Impale": (163621, "trash"), "Qual'Tin": (118867, "named"),
                "The Watchman": (118868, "named"), "enraged phantasm": (118870, "trash"), "CazicThule": (118882, "boss")},
         items=["Robe of the Fear Lord", "Cazic's Dread Chestplate", "Breastplate of the Trials", "Chainmail of Celerity",
                "Tunic of the Lava Pool", "Ring of Contention", "Cazic Thule's Signet", "Belt of the Watchman",
                "Coil of Qual'Tin", "Leggings of the Phantasm", "Blade of the Dread Estate", "Sword of the Faceless"]),
    dict(key="thulite", dungeon="thulite", src=0, slot=403, trigger="Heroic Thulite Golem", at=(18467.5, 69.6, 25426.5), radius=300,
         family=r"^(chaos hound|singetail scorpion|plague griffon|pox griffon|arachnalith|crag spider|ebon great bear|torrid drake|griffon|stone spider|fire drake)$",
         steps=steps_thulite, tat=(18067.0, 69.6, 25026.0), temps={"Thulite Golem": (122497, "boss")},
         items=["Thulite Breastplate", "Golemheart Breastplate", "Tunic of the Anvil", "Forgeworn Tunic", "Tunic of Thulite Dust",
                "Katana of the Anvil", "Thulite Maul", "Golem Shard Fang", "Thulite Tower Shield", "Anvilstone Shield",
                "Bulwark of the Golem", "Slag Dagger"]),
    dict(key="dynmath", dungeon="dynmath", src=0, slot=404, trigger="Heroic Dynmath", at=(4765.4, 59.0, 14028.4), radius=300,
         family=None, steps=steps_dynmath, tat=(4365.0, 59.0, 13628.0),
         temps={"Dynmath the Tyrant": (122985, "boss"), "minion of Dynmath": (196476, "trash")},
         items=["Tunic of the Tyrant's Shadow", "Etheneric Druid's Tunic", "Tunic of the Portal Seer", "Breastplate of the Tyrant",
                "Breastplate of the Rift", "Halberd of Dynmath", "Tyrant's Edge", "Smasher of the Dead", "Bow of the Etheneric Rift",
                "Eye of the Etheneric Portal", "Dynmath's Black Heart", "Lantern of the Rift"]),
    dict(key="parathior", dungeon="parathior", src=0, slot=405, trigger="Heroic Parathior", at=(7035.4, 76.2, 4281.6), radius=300,
         family=r"^(ice drake|avalanche worm|iceclaw bear|blizzard dervish|ice giant|ancient mammoth)$",
         steps=steps_parathior, tat=(6635.0, 76.2, 3900.0), temps={"Parathior": (114825, "boss")},
         items=["Tunic of the Frozen Summoner", "Robe of the Broken Tower", "Tunic of the Ice Charmer", "Tunic of the Mountain Light",
                "Tunic of the Frost Alchemist", "Ring of Parathior", "Ring of the Broken Tower", "Belt of the Avalanche",
                "Frostwoven Coil", "Leggings of the Glacier", "Blade of the Mountainside", "Sword of Frozen Light"]),
    dict(key="jarek", dungeon="jarek", src=0, slot=406, trigger="Heroic Jarek's Creation", at=(24162.3, 54.1, 9576.8), radius=300,
         family=None, steps=steps_jarek, tat=(23900.0, 54.1, 9227.0), temps={"Jarek's Creation": (122241, "boss")},
         items=["Robe of the Maker's Ruin", "Chestplate of the Creation", "Breastplate of Innoruuk's Hate", "Chainmail of the Crab Shell",
                "Tunic of the Roach Swarm", "Katana of the Unbound", "Jarek's Ruined Scepter", "Fang of the Creation",
                "Carapace Tower Shield", "Shield of Crab and Roach", "Bulwark of the Unmade", "Dagger of N`Toxis"]),
    dict(key="zek", dungeon="zek", src=0, slot=407, trigger="Heroic Zek Sergeant", at=(20250.0, 89.0, 18250.0), radius=300,
         family=None, steps=steps_zek, tat=(20000.0, 89.0, 17930.0),
         temps={"Zek troop": (123157, "trash"), "Zek Sergeant": (123161, "boss")},
         items=["Robe of the Zek Recruit", "Chestplate of the Sergeant", "Breastplate of the Zek Camp", "Chainmail of the Zek Scout",
                "Tunic of the Desert Camp", "Halberd of the Zek Troop", "Sergeant's Rallying Blade", "Smasher of the Warlord",
                "Bow of the Zek Watch", "Eye of Tallon", "Seal of the Zek Legion", "Lantern of the Zek Camp"]),
    dict(key="jailor", dungeon="jailor", src=0, slot=408, trigger="Heroic Superior Head Jailor", at=(17098.4, 5.0, 13250.5),
         radius=300, family=r"^(superior pickclaw guard)$", steps=steps_jailor, tat=(16850.0, 5.0, 13000.0),
         temps={"superior jailor": (198449, "named"), "superior head jailor": (123114, "boss")},
         items=["Tunic of the Goblin Captor", "Robe of the Cells", "Tunic of the Jailor's Charm", "Tunic of the Pickclaw Seer",
                "Tunic of the Prison Brewer", "Katana of the Head Jailor", "Jailor's Cudgel", "Fang of the Pickclaw",
                "Tower Shield of the Cells", "Pickclaw Guard's Shield", "Bulwark of the Jail Gate", "Shiv of the Cells"]),
    dict(key="valkanith", dungeon="valkanith", src=1, slot=409, trigger="Heroic Lord Valkanith", at=(4160.4, 86.5, 9809.5),
         radius=400, family=r"^(spectral slayer|toxic mist|mummified ogre|malign wisp|crypt keeper|revenant)$",
         steps=steps_valkanith, tat=(4560.0, 86.5, 9400.0),
         temps={"Tal'thex knight": (123016, "trash"), "Tal'thex soldier": (123018, "trash"), "Tal'thex sorcerer": (123020, "trash"),
                "Lord Valkanith": (123022, "boss")},
         items=["Breastplate of the Tal'thex", "Valkanith's Black Plate", "Tunic of Envar's Isle", "Tunic of the Risen Dead",
                "Tunic of the Tal'thex Blade", "Halberd of Lord Valkanith", "Sword of the Tal'thex Knight", "Smasher of the Risen",
                "Bow of Envar's Island", "Eye of the Tal'thex", "Gem of the Tal'thex Dead", "Lantern of Envar's Ruin"]),
    dict(key="sarek", dungeon="sarek", src=1, slot=410, trigger="Heroic Sarek the Betrayer", at=(4700.0, 110.4, 2500.0), radius=300,
         family=r"^(mercurial construct|mithril gargoyle|granite guardian|magma guardian|earthen dryad|construct of earth|construct of fire)$",
         steps=steps_sarek, tat=(4450.0, 66.0, 2150.0), temps={"Sarek the Betrayer": (196277, "boss")},
         items=["Tunic of the Betrayer", "Tunic of the Earthen Dryad", "Tunic of the Primeval", "Breastplate of the Archons",
                "Breastplate of Geomancy", "Ring of Sarek", "Ring of the Betrayal", "Belt of the Citadel", "Coil of the Primevals",
                "Leggings of Mercurial Stone", "Blade of the Betrayer", "Sword of the Archon"]),
    dict(key="voxraid", dungeon="vox", src=0, slot=411, trigger="Heroic Enraged Lady Vox", at=(4853.0, 112.7, 4410.3), radius=300,
         family=None, box=False, steps=steps_voxraid, tat=(4370.0, 112.7, 4100.0), temps={"Enraged Lady Vox": (122824, "boss")},
         items=["Breastplate of the Enraged", "Plate of Vox's Fury", "Tunic of the Frozen Lair", "Tunic of the Ice Wyrm",
                "Tunic of Frostbitten Shadows", "Ring of the Enraged Dragon", "Ring of Permafrost", "Belt of the Frozen Wyrm",
                "Coil of Lady Vox", "Leggings of Glacial Scale", "Blade of the Enraged", "Sword of Vox's Lair"]),
]

base.PKS.setdefault("gmtriggers", ["slot"])

# Kratas's trigger (tools/heroic_kratas_20261128.py, build468) ends "end, wait 14400, despawn": a wipe's clock goes
# to its end too, so a party that wipes waits out 4 hours before he rises again in its copy. Its steps are given
# this tool's end (tail) when they are still as that tool wrote them.
KRATAS_SLOT = 401
KRATAS_OLD_TAIL = [{"do": "end"}, {"do": "wait", "seconds": 14400}, {"do": "despawn"}]
KRATAS_NEW_TAIL = [{"do": "clock", "seconds": 14400}, {"do": "wait", "seconds": 14400}, {"do": "end"}, {"do": "despawn"}]


def wrote(db, now, tbl, key, before=None):
    """Log and audit a row just inserted, or updated from before (key: its primary key's columns)."""
    where = " AND ".join(f"{k} = ?" for k in key)
    after = base.rowd(db, tbl, where, list(key.values()))
    b = None if before is None else json.dumps(before, sort_keys=True)
    db.execute(f"INSERT INTO {LOG} (run, tbl, pk, after, before) VALUES (?, ?, ?, ?, ?)",
               (RUN, tbl, json.dumps(key, sort_keys=True), json.dumps(after, sort_keys=True), b))
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)""",
               (now, "insert" if before is None else "update", tbl, json.dumps(key), b, json.dumps(after, sort_keys=True), DETAIL))


def insert(db, now, tbl, vals):
    cols = list(vals)
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [vals[c] for c in cols])
    key = {k: (cur.lastrowid if k == "id" and k not in vals else vals[k]) for k in base.PKS[tbl]}
    wrote(db, now, tbl, key)


def copy_npc(db, cols, row_id):
    db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (row_id,))
    return db.execute("SELECT last_insert_rowid()").fetchone()[0]


def ensure_log(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   pk TEXT NOT NULL, after TEXT, before TEXT, at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
                   undone TEXT NOT NULL DEFAULT '')""")


def undo(db):
    if not db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (LOG,)).fetchone():
        sys.exit("nothing applied: nothing done")
    rows = db.execute(f"SELECT seq, tbl, pk, after, before FROM {LOG} WHERE run = ? AND undone = '' ORDER BY seq DESC", (RUN,)).fetchall()
    if not rows:
        sys.exit("nothing applied: nothing done")
    for seq, tbl, pk, after, before in rows:
        key = json.loads(pk)
        cur = base.rowd(db, tbl, " AND ".join(f"{k} = ?" for k in key), list(key.values()))
        if cur is None or json.dumps(cur, sort_keys=True) != after:
            sys.exit(f"{tbl} {pk} changed since (a GM's edit?): nothing done")
    now = base.now_s()
    with db:
        for seq, tbl, pk, after, before in rows:
            key = json.loads(pk)
            where = " AND ".join(f"{k} = ?" for k in key)
            if before is None:
                db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
            else:
                old = json.loads(before)
                db.execute(f"UPDATE {tbl} SET " + ", ".join(f"{c} = ?" for c in old) + f" WHERE {where}",
                           list(old.values()) + list(key.values()))
            db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                          VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)""",
                       (now, "delete" if before is None else "update", tbl, pk, after, before,
                        "undo of tools/heroic_raids2_20261128.py"))
            db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (now, seq))
        for t in ("npcs", "loottables"):
            if db.execute(f"SELECT MAX(id) FROM {t}").fetchone()[0] is None:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
            else:
                db.execute(f"UPDATE sqlite_sequence SET seq = (SELECT MAX(id) FROM {t}) WHERE name = ?", (t,))
    print(f"undone: {len(rows)} rows")


def in_box(b, x, z):
    return min(b[0], b[2]) <= x <= max(b[0], b[2]) and min(b[1], b[3]) <= z <= max(b[1], b[3])


def select(db, R, d, temp_ids):
    """The source world's rows of the dungeon's boxes, each with its tier: its kinds (trash) and named, never the
    trigger's creatures, a friendly, a spawn-only or a quest's seeded row."""
    if R.get("box") is False:
        return []
    spawnonly = {r[0].lower() for r in db.execute("SELECT name FROM npcspawnonly")}
    seeded = {r[0] for r in db.execute("SELECT npc_row FROM quest_npc_seed_rows")}
    names = {n.lower() for n in R["temps"]}
    seen, rows = set(), []
    for b in d["boxes"]:
        x0, z0, x1, z1 = min(b[0], b[2]), min(b[1], b[3]), max(b[0], b[2]), max(b[1], b[3])
        cur = db.execute("""SELECT * FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?
                            ORDER BY id""", (R["src"], x0, x1, z0, z1))
        cols = [c[0] for c in cur.description]
        for r in cur.fetchall():
            r = dict(zip(cols, r))
            n = r["npc_name"]
            if (r["id"] in seen or r["id"] in temp_ids or n.lower() in names or r["npc_type"] & 0x80
                    or f"#{r['id']}" in spawnonly or n.lower() in spawnonly or r["id"] in seeded):
                continue
            named = n[:1].isupper()
            if not (named or (R["family"] and re.match(R["family"], n, re.I))):
                continue
            seen.add(r["id"])
            rows.append((r, "named" if named else "trash"))
    return rows


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--recipe", default=os.path.join(HERE, "worlds", "dungeons.json"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    if a.undo:
        undo(db)
        return
    for t in base.TEMPLATES:
        if not db.execute("SELECT 1 FROM classtemplates WHERE name = ?", (t,)).fetchone():
            sys.exit(f"classtemplates {t!r} is missing (no dungeons on this shard; tools/dungeons_20261112.py first): nothing done")
    if db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (LOG,)).fetchone() and \
            db.execute(f"SELECT 1 FROM {LOG} WHERE run = ? AND undone = '' LIMIT 1", (RUN,)).fetchone():
        print("applied already: nothing done")
        return
    world, dungeons = base.recipe_dungeons(a.recipe)
    if world != WORLD:
        sys.exit(f"the recipe {a.recipe} is world {world}'s, want {WORLD}: nothing done")
    by_key = {d["key"]: d for d in dungeons}
    all_boxes = [b for d in dungeons for b in d["boxes"]]
    last = ITEM_FIRST + 12 * len(RAIDS) - 1
    if db.execute("SELECT count(*) FROM itempattern WHERE patternid BETWEEN ? AND ?", (ITEM_FIRST, last)).fetchone()[0]:
        sys.exit(f"itempattern {ITEM_FIRST}-{last} is taken: nothing done")
    twin = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) < 1 AND abs(z_coord - ?) < 1",
                      (WARDEN, WORLD, *TWIN_OF)).fetchone()
    if twin is None:
        sys.exit(f"no {WARDEN} twin at Blackburrow's landing {TWIN_OF} (tools/dungeons_landing_20261112.py first): nothing done")
    for i, R in enumerate(RAIDS):
        d = by_key.get(R["dungeon"])
        if d is None:
            sys.exit(f"the recipe {a.recipe} has no dungeon {R['dungeon']!r}: nothing done")
        if len(R["items"]) != 12 or any(len(n) > 32 for n in R["items"]):
            sys.exit(f"{R['key']}: twelve names of at most 32 wanted: nothing done")
        if db.execute("SELECT count(*) FROM loottables WHERE tablename LIKE ?", (f"dungeon {R['key']} %",)).fetchone()[0]:
            sys.exit(f"loottables 'dungeon {R['key']} ...' exist already: nothing done")
        if db.execute("SELECT 1 FROM gmtriggers WHERE slot = ? OR name = ?", (R["slot"], R["trigger"])).fetchone():
            sys.exit(f"gmtriggers slot {R['slot']} or {R['trigger']!r} is taken: nothing done")
        if not any(in_box(b, R["at"][0], R["at"][2]) for b in d["boxes"]):
            sys.exit(f"{R['key']}: the encounter's place {R['at']} is outside its dungeon's boxes: nothing done")
        if any(in_box(b, R["tat"][0], R["tat"][2]) for b in all_boxes):
            sys.exit(f"{R['key']}: the template spot {R['tat']} is inside a dungeon's box (every instance would stand them up): nothing done")
        if R.get("box") is not False:
            for b in d["boxes"]:
                n = db.execute("SELECT count(*) FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?",
                               (WORLD, min(b[0], b[2]), max(b[0], b[2]), min(b[1], b[3]), max(b[1], b[3]))).fetchone()[0]
                if n:
                    sys.exit(f"world {WORLD} has {n} npcs rows in {R['key']}'s box {b} already: nothing done")
        for name, (rid, tier) in R["temps"].items():
            r = db.execute("SELECT npc_name, world FROM npcs WHERE id = ?", (rid,)).fetchone()
            if r is None or r[0] != name or r[1] != R["src"]:
                sys.exit(f"{R['key']}: npcs row {rid} is {r}, want {name!r} of world {R['src']} (another shard's ids?): nothing done")
        for bid in bases(i):
            if base.rowd(db, "itempattern", "patternid = ?", (bid,)) is None:
                sys.exit(f"base item {bid} is missing: nothing done")
    ensure_log(db)
    db.commit()
    now = base.now_s()
    t0 = time.time()
    cols = [c[1] for c in db.execute("PRAGMA table_info(npcs)") if c[1] != "id"]
    icols = [c[1] for c in db.execute("PRAGMA table_info(itempattern)")]
    summary = []
    db.execute("BEGIN")
    pid = ITEM_FIRST
    for i, R in enumerate(RAIDS):
        d, key = by_key[R["dungeon"]], R["key"]
        first = pid
        for j, (name, bid) in enumerate(zip(R["items"], bases(i))):
            tier = "boss" if j in BOSS_PIECES else "named"
            b = base.rowd(db, "itempattern", "patternid = ?", (bid,))
            v = dict(b)
            v["patternid"], v["itemname"], v["rent"] = pid, name, 1
            v["itemdesc"] = f"Raid gear of {d['name']}'s instance, as strong as {b['itemname']}."
            insert(db, now, "itempattern", {c: v[c] for c in icols})
            insert(db, now, "loottables", {"tablename": f"dungeon {key} {tier}", "patternid": pid, "weight": 1, "minlevel": 1,
                                           "maxlevel": 100, "source": RUN})
            pid += 1
        temp_ids = {rid for rid, _ in R["temps"].values()}
        rows = select(db, R, d, temp_ids)
        lv = base.raid_levels(rows)
        tiers = {"trash": 0, "named": 0}
        for r, tier in rows:
            nid = copy_npc(db, cols, r["id"])
            db.execute("UPDATE npcs SET world = ?, npc_level = ?, npc_id = 0, dt_kill = NULL WHERE id = ?", (WORLD, lv[r["id"]], nid))
            wrote(db, now, "npcs", {"id": nid})
            insert(db, now, "npctemplate", {"name": f"#{nid}", "template": f"dungeon {tier}",
                                            "note": f"{d['name']}'s {tier}: {r['npc_name']} (world {R['src']} row {r['id']}, level {r['npc_level']} there)",
                                            "source": RUN})
            for suffix, chance, rolls in base.LOOT[tier]:
                insert(db, now, "npcloot", {"npc_name": f"#{nid}", "tablename": f"dungeon {key} {suffix}", "chance": chance,
                                            "rolls": rolls, "note": f"{d['name']}'s {tier} {r['npc_name']}", "source": RUN})
            tiers[tier] += 1
        # the trigger's creatures: templates outside every box, a step apart
        T = {}
        for k, (name, (rid, tier)) in enumerate(R["temps"].items()):
            x, y, z = R["tat"][0] + 3 * k, R["tat"][1], R["tat"][2]
            src_level = db.execute("SELECT npc_level FROM npcs WHERE id = ?", (rid,)).fetchone()[0]
            nid = copy_npc(db, cols, rid)
            db.execute("""UPDATE npcs SET world = ?, npc_level = ?, npc_id = 0, dt_kill = NULL, x_coord = ?, y_coord = ?, z_coord = ?
                          WHERE id = ?""", (WORLD, TIER_LEVEL[tier], x, y, z, nid))
            wrote(db, now, "npcs", {"id": nid})
            insert(db, now, "npctemplate", {"name": f"#{nid}", "template": TEMPLATE_OF[tier],
                                            "note": f"{d['name']}'s {tier}: {name} (world {R['src']} row {rid}, level {src_level} there), raised by trigger {R['slot']}",
                                            "source": RUN})
            for suffix, chance, rolls in base.LOOT[tier]:
                insert(db, now, "npcloot", {"npc_name": f"#{nid}", "tablename": f"dungeon {key} {suffix}", "chance": chance,
                                            "rolls": rolls, "note": f"{d['name']}'s {tier} {name}", "source": RUN})
            T[name] = f"#{nid}"
        steps = R["steps"](T)
        insert(db, now, "gmtriggers", {"slot": R["slot"], "name": R["trigger"], "enabled": 1, "scope": "near", "radius": R["radius"],
                                       "world": WORLD, "x": R["at"][0], "y": R["at"][1], "z": R["at"][2],
                                       "steps": json.dumps(steps, separators=(",", ":")),
                                       "notes": (f"Heirloom petition 3396 (the owner, 2026-11-28): {d['name']}, a heroic raid. A party "
                                                 "near the encounter in its own copy of the dungeon starts it; once its boss is dead "
                                                 "it does not start again in that copy (the run waits out 4 hours), a wipe (the "
                                                 "clock's empty time) lets it start again. tools/heroic_raids2_20261128.py")})
        named = sorted({r["npc_name"] for r, t in rows if t == "named"})
        kinds = {}
        for r, t in rows:
            if t == "trash":
                kinds[r["npc_name"]] = kinds.get(r["npc_name"], 0) + 1
        summary.append(f"{key} ({d['name']}): {len(rows)} creatures ({tiers['trash']} trash, {tiers['named']} named"
                       f"{': ' + ', '.join(named) if named else ''}); trigger {R['slot']} {R['trigger']!r}, {len(steps)} steps; "
                       f"templates {', '.join(f'{n} {T[n]}' for n in T)}; items {first}-{pid - 1}")
        if kinds:
            summary.append("  trash: " + ", ".join(f"{k} {v}" for k, v in sorted(kinds.items())))
        if R.get("box") is not False:
            ex, ey, ez, ef = d["entrance"]
            h = ef * 2 * math.pi / 256
            wx, wz = ex + 4 * math.cos(h), ez - 4 * math.sin(h)
            nid = copy_npc(db, cols, twin[0])
            db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
                       (round(wx, 3), ey, round(wz, 3), base.facing_byte(wx, wz, ex, ez), nid))
            wrote(db, now, "npcs", {"id": nid})
            summary.append(f"  npcs {nid}: {WARDEN} at the landing ({wx:.2f}, {ey}, {wz:.2f}); landing {math.hypot(ex - R['at'][0], ez - R['at'][2]):.0f} from the encounter")
        else:
            ex, ey, ez, ef = d["entrance"]
            summary.append(f"  {d['name']}'s own landing, {math.hypot(ex - R['at'][0], ez - R['at'][2]):.0f} from the encounter")
    kr = base.rowd(db, "gmtriggers", "slot = ?", (KRATAS_SLOT,))
    if kr is not None and kr["name"] == "Heroic Kratas Rises":
        ks = json.loads(kr["steps"])
        if ks[-3:] == KRATAS_OLD_TAIL:
            ks = ks[:-3] + KRATAS_NEW_TAIL
            db.execute("UPDATE gmtriggers SET steps = ? WHERE slot = ?", (json.dumps(ks, separators=(",", ":")), KRATAS_SLOT))
            wrote(db, now, "gmtriggers", {"slot": KRATAS_SLOT}, before=kr)
            summary.append(f"gmtriggers {KRATAS_SLOT} (Heroic Kratas Rises): its end made this tool's (a wipe starts it again)")
        else:
            summary.append(f"gmtriggers {KRATAS_SLOT}: its end is not heroic_kratas's any more (a GM's edit?): left as it is")
    else:
        summary.append(f"gmtriggers {KRATAS_SLOT}: no Heroic Kratas Rises: nothing to mend")
    n = db.execute(f"SELECT count(*) FROM {LOG} WHERE run = ? AND undone = ''", (RUN,)).fetchone()[0]
    print("\n".join(summary))
    print(f"{pid - ITEM_FIRST} items {ITEM_FIRST}-{pid - 1}; {n} rows in {time.time() - t0:.1f} s")
    if a.apply:
        db.commit()
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
