#!/usr/bin/env python3
"""The camp audit's fixes (MasterVG782's EQOA Frontiers walkthrough on GameFAQs, section XVI "CAMPS & UNIQUE
PLACES"; the audit ~/.cache/campaudit-20261121/report.md), the four groups the owner approved 2026-11-21:

  1. rares    Named rares and respawns. New: Chief Merka and Priestess Thiel (Merkans, Stone of Morthalis: rares on
              the placeholders the guide names, the huts up the ramp), the queen crawler (Spider Mine: a rare on the
              cave crawler at the back of the cave), the Froglok Invader (Burial Mounds: a rarer roamer, red to a 60:
              its own row, 20 min, wandering). Respawns: Boss Hurg 15 min (the guide: "around 10-20 minutes"); Grug
              Ironhead, Slayer Demaeog and the spectre's soul 20 min (each "a rare spawn" with no placeholder named).
              Boss Hurg's stray L44 row at Deathfist Horde removed.
  2. casters  Class templates for the camps' casters (each the roamer's figures and its crit, plus the guide's kit):
              the Taksehir miners by their bodies as the guide tells them apart (Erudite: the AoE wizard; full body
              armour: the AR-nuke cleric; the other body: the paladin with Lay Hands at half health), the froglok
              inquisitor (cleric nukes, one self heal at half), the goblin soothsayer (a ~717 DR nuke), the frosteye
              sage (shaman nukes, a wolf spirit beside each: Kindred Spirit, a companion kind), the treant diviner
              (a fire nuke), the Arctic Spiritcaller (shaman) and Arctic Adept (wizard), the Centaur Valley undead
              (alternate rows: a shadow knight's lifetap or a druid's lightning and fire nukes with a heal at half),
              the spectre's soul (paladin, Lay Hands). AoE criticals (a rampage on a 30 s clock) for the frosteye
              centurions and patrollers, grizz warriors, kobold slayers and tenacious frost giants. The griffons' damage
              shield (23: the guide's 22-25). See-invisible for Voxite mages, ice giant priests and the goblin
              shamans and witch doctors (Moss Mouth).
  3. links    Linked aggro (npcsocial): the aviaks (Bluefeather's Brood, the faction their kill lowers), the Centaur
              Valley undead, the Taksehir miners, the tenacious frost giants and Sothimus the Brigand with his
              bandits (Desert Bandits, theirs).
  4. places   The Merkan villagers to ~32 and shamans to ~33 (the guide: villagers "around level 32"); the 41-50
              ratmen (fighters, scavengers) moved out of The Vastly Deep into South Barren Coast (N), the camp's shape
              kept (part "ratmen": --skip ratmen leaves them); a third tenacious frost giant camp, the Permafrost
              camp's 27 copied NE of it (the guide: "the last one northeast of the one in Permafrost"; centre
              ~(5438, 5094), ~700 NE of the ruins camp, ~1,380 N of Zentar's Keep's gate); Live's "Artic Spiritcaller"
              spelt as Heirloom's "Arctic Spiritcaller" (its loot and class rows too; Heirloom, which has both spellings
              standing together, is left and said).

Every new or moved spawn's height is the ground's (cmd/campground over the box's ground files, worlds 0 and 2, at
the time this was written: SPOTS below): on its floor, not roofed, not embedded, not in water, clear of every other
NPC on both shards. A source row moved since is refused. --campground BIN --ground DIR checks them again first.

Report only by default (every change printed); --apply writes it in one transaction, every row with a gm_audit line
(accountid 1, dab, via "tool") and in the log guide_camps_1121_log; --undo puts back exactly what it wrote (a row
changed since is left and said) and drops the log; --applied lists it. By NAMES and places, never ids: each shard's
rows are found by name within 3 units of where they stand; new ids from 196100 (refused if any is taken).
Shard-aware by its data: what a shard has not got (Live's misspelt Spiritcaller, Heirloom's second one) decides.

WHEN IT TAKES EFFECT: at the next restart (npcs, npcrare, npcspawnonly, templates, social, aggro, move, loot).

    python3 -E -s tools/guide_camps_20261121.py --db eqoa.db [--apply | --applied | --undo] [--skip ratmen]
        [--campground BIN --ground DIR]
"""
import argparse
import datetime
import json
import math
import sqlite3
import subprocess
import sys

LOG = "guide_camps_1121_log"
SRC = "guide-camps 20261121"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
GUIDE = "MasterVG782's GameFAQs walkthrough"


def detail(camp):
    return f"the owner 2026-11-21: camp audit vs MasterVG782's guide: {camp}"


TOL = 3.0          # a row is found by name within this of its place
ID_FROM, ID_TO = 196100, 196400

# ---------------------------------------------------------------- the templates
# Each is the roamer's row (its figures and its Critical Hit from level 10 every 25-35 s) with the guide's kit; the
# percentages are of a player's spell of the creature's level, as jib's wizard (35%) and necromancer (50%) are.
ROAMER = "roamer"
T_PALADIN = "guide camp paladin"
T_CLERIC = "guide camp cleric"
T_AOEWIZ = "guide camp aoe wizard"
T_SHAMAN = "guide camp shaman"
T_SOOTH = "guide camp soothsayer"
T_DRUID = "guide camp druid"
T_WIZARD = "guide camp wizard"
T_UKNIGHT = "guide camp undead knight"
T_UDRUID = "guide camp undead druid"
T_AOECRIT = "guide camp aoe crit"
T_GRIFFON = "guide camp griffon"

NUKE35 = {"kind": "nuke", "every": 12, "first": 4, "damage_pct": 35, "max_recast": 10, "max_cast": 5}
ONCE_HEAL = {"kind": "heal", "below": 50, "every": 3600, "max_cast": 5}
# a druid's book holds no heal a template picks: a heal of its share, as 'dungeon trash''s druids and shamans have
ONCE_SHARE_HEAL = {"kind": "heal", "below": 50, "every": 3600, "heal_pct": 10, "name": "Healing"}
TEMPLATES = {
    # (class, abilities, overrides, note)
    T_PALADIN: (2, [{"kind": "heal", "heal_pct": 100, "below": 50, "every": 3600, "name": "Lay Hands"}], {},
                "the guide's paladin-type (Taksehir miners: 'uses Lay Hands when it gets to about half health'; "
                "the spectre's soul: 'LH and a crit'): one Lay Hands to full at half health a fight"),
    T_CLERIC: (9, [NUKE35, ONCE_HEAL], {},
               "the guide's cleric-type (froglok inquisitors: 'AR-based nukes and the ability to heal themselves "
               "(always once when they get to half life)'; Taksehir priests, Chief Merka): a cleric nuke of its "
               "level every 12 s at 35%, one cleric heal below half a fight"),
    T_AOEWIZ: (13, [{"kind": "aoe", "line": "column", "every": 10, "first": 3, "damage_pct": 100, "max_recast": 10,
                     "max_cast": 5}], {},
               "the guide's Taksehir wizards: 'every nuke they have is AoE (860 for regular portion plus a 250 or so "
               "AoE portion)': a wizard's Lightning Column (L60: 846 on its target, 360 round it) at 100%, every 10 s "
               "(no other wizard area spell of a level a template can pick is near the guide's figures; a fixed "
               "figure is a nuke's, a lifetap's or a ward's only)"),
    T_SHAMAN: (8, [NUKE35], {},
               "the guide's shaman-type (frosteye sages, Arctic Spiritcaller, Priestess Thiel: 'DR-based nukes'): a "
               "shaman nuke of its level every 12 s at 35% (no shaman nuke's words name disease, so DR is not "
               "picked: its resist is the spell's)"),
    T_SOOTH: (8, [{"kind": "nuke", "every": 15, "first": 5, "damage": 717, "resist": "DR"}], {},
              "the guide's goblin soothsayers: 'a DR-based nuke of around 717', no pet: a fixed 717 cut by DR, its "
              "look a shaman nuke's, every 15 s (no shaman spell of a level reaches 717; the defensive debuff is not "
              "named, so not made)"),
    T_DRUID: (7, [dict(NUKE35, element="fire")], {},
              "the guide's treant diviners: 'a FR-based nuke, plus they hit once for moderate damage': a druid fire "
              "nuke of its level every 12 s at 35%"),
    T_WIZARD: (13, [{"kind": "aoe", "line": "column", "every": 15, "first": 2, "damage_pct": 35, "max_recast": 10, "max_cast": 5},
                    {"kind": "nuke", "chain": True, "damage_pct": 35, "max_recast": 10, "max_cast": 5}], {},
               "the guide's Arctic Adept, 'a wizard-type mob': jib's wizard kit (an area spell, then a single "
               "target's, every 15 s at 35%) on the roamer's figures, its area spell the Lightning Column (jib's "
               "picks the Fingers, 100 for a creature with no DEX)"),
    T_UKNIGHT: (3, [{"kind": "lifetap", "every": 15, "first": 3, "damage_pct": 50, "max_recast": 90}], {},
                "the guide's Centaur Valley undead, one kind: 'normal SK-type mobs, who only melee twice, have a "
                "860-ish crit and have a large lifetap': jib's shadow knight tap (50%, every 15 s)"),
    T_UDRUID: (7, [dict(NUKE35, element="lightning"), dict(NUKE35, element="fire", first=10), ONCE_SHARE_HEAL], {},
               "the guide's Centaur Valley undead, the other kind: 'a druid-type mob, alternating between LR nukes "
               "and FR-based ones, who also heal once half of their life have been taken' (10% of its hit points, once)"),
    T_AOECRIT: (0, [{"kind": "rampage", "every": 30, "first": 25, "damage_pct": 150, "name": "Area Critical"}], {},
                "the guide's 'regular critical and an AoE one' (frosteye centurions/patrollers, grizz warriors, "
                "kobold slayers, tenacious frost giants): the roamer's Critical Hit, and a melee hit of 150% on "
                "everyone within 10 (a rampage) every 30 s from 25 s"),
    T_GRIFFON: (0, [], {"shield_damage": 23},
                "the guide's griffon warriors' camp: 'All of the griffons have a DS that ranges from 22-25, plus "
                "criticals of around 700 or so': the roamer with a damage shield of 23"),
}

# name -> template, and the templates a name may have now (None: no row)
BY_NAME = [
    ("FROGLOKS", "froglok inquisitor", T_CLERIC),
    ("GOBLINS", "goblin soothsayer", T_SOOTH),
    ("FROSTEYE ORCS", "frosteye sage", T_SHAMAN),
    ("TREANTS", "treant diviner", T_DRUID),
    ("TFGS - TENACIOUS FROST GIANTS", "arctic spiritcaller", T_SHAMAN),
    ("TFGS - TENACIOUS FROST GIANTS", "artic spiritcaller", T_SHAMAN),   # where a row is still spelt so (Heirloom)
    ("TFGS - TENACIOUS FROST GIANTS", "arctic adept", T_WIZARD),
    ("CV UNDEAD", "spectre's soul", T_PALADIN),
    ("FROSTEYE ORCS", "frosteye centurion", T_AOECRIT),
    ("FROSTEYE ORCS", "frosteye patroller", T_AOECRIT),
    ("GRIZZES", "grizz warrior", T_AOECRIT),
    ("SLAYERS", "kobold slayer", T_AOECRIT),
    ("TFGS - TENACIOUS FROST GIANTS", "tenacious frost giant", T_AOECRIT),
    ("GRIFFON WARRIORS", "griffon elder", T_GRIFFON),
    ("GRIFFON WARRIORS", "griffon nest protector", T_GRIFFON),
    ("GRIFFON WARRIORS", "griffon warrior", T_GRIFFON),
    ("GRIFFON WARRIORS", "windtuft", T_GRIFFON),
]
REPLACEABLE = {None, ROAMER}

# the Taksehir miners (The Great Waste): by body, as the guide tells them apart ("The casters use an Erudite model,
# the clerics seem to always be in full body armor and the other model used here are the paladins")
MINERS = ("taksehir miner", "taksehir foreman", "taksehir crystal runner")
GREAT_WASTE = (0, 22000, 24000, 24000, 26000)   # world, x0, x1, z0, z1
ERUDITE = {0x320C0B47, 0x4F1A3CBE}
CENTAUR_VALLEY = (0, 14000, 16000, 20000, 22000)
CV_UNDEAD = ("wandering ghast", "wandering ghost")

SEE_INVIS = [("PERMAFROST", "Voxite mage"), ("PERMAFROST", "ice giant priest"),
             ("MOSSMOUTH GOBLINS", "goblin shaman"), ("MOSSMOUTH GOBLINS", "goblin witch doctor")]

# the frosteye sages' wolf spirits: a Kindred Spirit (entity/companion.go's companionPetKinds: the pet of the creature
# beside it) with the wolf body the L46-64 shaman pets have, 2 off each sage (campground: floor, dry, clear)
PET_BASE = ("Kindred Spirit", 821616988)
SAGE_PETS = [  # the sage (world, x, z), the pet's place
    ((0, 7516.0, 5750.9), (7514.0, 72.48, 5750.9)),
    ((0, 7517.9, 5622.9), (7515.9, 80.30, 5622.9)),
    ((0, 7662.2, 5568.6), (7660.2, 57.72, 5568.6)),
]

# ---------------------------------------------------------------- group 3: links
LINKS = [
    ("AVIAKS", "aviak", "Bluefeather's Brood"), ("AVIAKS", "aviak scouter", "Bluefeather's Brood"),
    ("CV UNDEAD", "Wandering Ghast", "Centaur Valley Undead"), ("CV UNDEAD", "Wandering Ghost", "Centaur Valley Undead"),
    ("CV UNDEAD", "Spectre's soul", "Centaur Valley Undead"),
    ("MINERS", "Taksehir miner", "Taksehir Miners"), ("MINERS", "Taksehir foreman", "Taksehir Miners"),
    ("MINERS", "Taksehir crystal runner", "Taksehir Miners"),
    ("TFGS - TENACIOUS FROST GIANTS", "tenacious frost giant", "Tenacious Frost Giants"),
    ("TFGS - TENACIOUS FROST GIANTS", "Arctic Spiritcaller", "Tenacious Frost Giants"),
    ("TFGS - TENACIOUS FROST GIANTS", "Arctic Adept", "Tenacious Frost Giants"),
    ("TFGS - TENACIOUS FROST GIANTS", "Artic Spiritcaller", "Tenacious Frost Giants"),   # where a row is still spelt so
    ("DESERT BANDITS", "Sothimus the Brigand", "Desert Bandits"),
]

# ---------------------------------------------------------------- group 1: rares
# a rare on placeholders: its own row stands out of the world (npcspawnonly "#id"), copied from the first
# placeholder's (place) with the body of `body` when given; entity/npcrare.go makes it on a placeholder's spot
RARES = [
    dict(camp="MERKANS", name="Chief Merka", level=37, template=T_CLERIC, like="merkan warrior", body=None, size=None,
         placeholders=[("merkan warrior", 2, 6229.8, 11052.2), ("merkan warrior", 2, 6244.3, 11061.4)], chance=15,
         why="the guide: 'Chief Merka is a cleric (AR-based nukes) and his placeholder is either the shaman/warrior in "
             "the west hut or the shaman/warrior on the eastern side of the north hut' (the huts up the ramp: our "
             "warriors there); level ours (the guards are 36)"),
    dict(camp="MERKANS", name="Priestess Thiel", level=36, template=T_SHAMAN, like="merkan shaman",
         body=("merkan shaman", 2, 6310.3, 11042.1), size=None,
         placeholders=[("merkan warrior", 2, 6224.5, 11023.1)], chance=15,
         why="the guide: 'Priestess Thiel is a shaman-type mob (DR-based nukes and a pet) and her placeholder is "
             "located in the north hut, just behind the table' (the northmost hut row); a shaman's body; level ours"),
    dict(camp="CAVE CRAWLERS", name="queen crawler", level=30, template=ROAMER, like="cave crawler", body=None,
         size=1.5, placeholders=[("cave crawler", 0, 9629.6, 18898.6)], chance=15,
         why="the guide: 'The queen crawler sometimes spawns in place of the one cave crawler near the NPC in the back "
             "of the cave, but it doesn't seem to drop anything other than the normal loot' (the cave's deepest "
             "crawler; no NPC stands in our cave); size and level ours"),
]
CHANCE_WHY = "15%: a placeholder's few minutes a kill make it about every 15-25 min (the guide's 'a bit rarer')"

INVADER = dict(camp="FROGLOKS", name="Froglok Invader", base=("Froglok Inquisitor", 0, 18531.3, 29630.8), level=63,
               at=(0, 18420.0, 54.12, 29592.0), respawn=1200.0, roam_radius=60.0,
               why="the guide: 'a rarer roamer (Froglok Invader) that follows the same model as an Inquisitor and is "
                   "red to a 60': an inquisitor's body and kit at 63, a 20 min respawn, wandering 60 round its spawn")

RESPAWNS = [  # (camp, name, world, x, z, seconds, why)
    ("BLOODFOOT BANDITS", "Boss Hurg", 0, 12170.9, 10981.5, 900.0, "the guide: 'a respawn of maybe around 10-20 minutes'"),
    ("SOUTHWATCH KEEP", "Grug Ironhead", 0, 18231.8, 20888.6, 1200.0, "the guide: 'a rare spawn' (no placeholder named): 20 min"),
    ("SLAYERS", "Slayer Demaeog", 2, None, None, 1200.0, "the guide: 'a rare spawn' at the coastal camp (no placeholder named): 20 min"),
    ("CV UNDEAD", "Spectre's soul", 0, 15426.5, 20805.9, 1200.0, "the guide: 'two rarer mobs' (no placeholder named): 20 min"),
]
STRAY_HURG = ("BLOODFOOT BANDITS", "Boss Hurg", 0, 19289.3, 20306.3)

# ---------------------------------------------------------------- group 4: places
MERKAN_LEVELS = [("merkan villager", 12), ("merkan shaman", 11)]   # 19-20 -> 31-32, 21-22 -> 32-33
STONE_OF_MORTHALIS = (2, 6000, 8000, 10000, 12000)

# the moved and copied camps: offsets, and each new place's floor height by its source row's place (x,z to 0.1)
RATMEN = dict(names=("ratman fighter", "ratman scavenger"), area=(2, 10000, 12000, 6000, 8000), dx=-231.2, dz=1803.4,
              y={"10539.6,6694.8": 57.61, "10689.4,6721.2": 54.15, "10692.8,6726.0": 54.56, "10686.8,6726.0": 54.32,
                 "10695.7,6742.2": 57.88, "10684.7,6745.8": 57.72, "10744.1,6706.9": 54.07, "10668.3,6638.8": 52.14,
                 "10677.5,6622.5": 51.35, "10672.9,6653.8": 52.42, "10735.0,6626.6": 46.69, "10758.5,6573.2": 46.27,
                 "10764.6,6550.4": 50.09, "10782.5,6540.4": 53.2, "10706.8,6522.3": 51.98, "10698.6,6549.2": 50.08,
                 "10673.0,6529.6": 54.53, "10723.3,6517.9": 52.15, "10585.4,6731.4": 54.09, "10585.4,6756.3": 54.05,
                 "10571.5,6757.6": 53.92, "10585.5,6742.4": 54.09, "10746.2,6497.4": 53.21})
TFG = dict(name="tenacious frost giant", area=(0, 4000, 6000, 4000, 6000), dx=500.0, dz=-500.0,
           y={"4861.7,5607.9": 60.18, "4765.7,5572.8": 65.37, "4826.9,5572.0": 60.87, "4830.1,5475.6": 57.4,
              "4884.1,5528.6": 59.27, "4874.9,5623.4": 59.75, "4906.6,5601.2": 60.11, "4796.5,5653.8": 63.94,
              "4855.5,5680.2": 62.87, "4829.4,5716.8": 64.46, "4920.0,5642.6": 59.38, "4906.4,5751.1": 60.48,
              "4943.2,5717.2": 58.0, "4999.1,5758.6": 54.16, "5042.5,5709.9": 59.46, "5015.7,5655.8": 59.51,
              "5094.3,5650.1": 68.92, "5067.4,5602.3": 63.76, "5008.5,5571.4": 51.37, "5092.0,5517.5": 57.81,
              "5042.4,5443.4": 58.89, "5006.2,5490.2": 64.06, "4972.5,5418.4": 60.22, "4983.4,5617.4": 63.76,
              "4958.8,5552.6": 51.38, "4927.2,5476.0": 61.79, "4902.4,5428.8": 57.45, "4779.4,5566.8": 63.3,
              "4830.5,5580.2": 61.09, "4831.6,5476.2": 57.42, "4871.5,5528.8": 57.9, "4898.8,5600.9": 58.64,
              "4808.2,5656.7": 63.1, "4851.3,5668.0": 62.79, "4820.7,5730.7": 65.0, "4927.3,5650.6": 59.38,
              "4908.7,5743.3": 60.2, "4943.3,5725.6": 57.8, "5000.0,5759.7": 54.14, "5041.7,5691.9": 60.7,
              "5027.6,5649.4": 60.11, "5104.4,5653.9": 69.8, "5074.2,5604.9": 64.78, "5006.9,5575.5": 51.38,
              "5101.5,5523.5": 58.47, "5056.7,5458.0": 57.7, "5006.5,5499.1": 66.56, "4972.6,5420.0": 60.04,
              "4972.8,5628.3": 61.06, "4942.8,5546.3": 51.38, "4926.8,5465.5": 60.51, "4908.7,5427.3": 57.56})
ARTIC, ARCTIC = "Artic Spiritcaller", "Arctic Spiritcaller"

PARTS = ("rares", "casters", "links", "places", "ratmen")


# ---------------------------------------------------------------- helpers
def has(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def rows(db, tbl, where, args=()):
    cur = db.execute(f'SELECT * FROM "{tbl}" WHERE {where}', args)
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def row(db, tbl, where, args=()):
    r = rows(db, tbl, where, args)
    return r[0] if r else None


def js(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True) if v is not None else None


def key_of(x, z):
    return f"{x:.1f},{z:.1f}"


def in_area(r, area):
    w, x0, x1, z0, z1 = area
    return r["world"] == w and x0 <= r["x_coord"] < x1 and z0 <= r["z_coord"] < z1


class Plan:
    """What the tool would write on this database: ops in order, each (op, tbl, key, values, camp, why)."""

    def __init__(self, db, skip=()):
        self.db, self.skip, self.bad, self.ops, self.notes = db, set(skip), [], [], []
        self.ids = self.free_ids()
        self.new_rows = {}   # name -> new npcs id (for rows later ops point at)
        # Live's misspelt Spiritcaller is renamed in the places part: its new name counts as there from the start
        self.renaming = ("places" not in self.skip and bool(rows(db, "npcs", "npc_name = ?", (ARTIC,)))
                         and not rows(db, "npcs", "npc_name = ?", (ARCTIC,)))
        if "rares" not in self.skip:
            self.plan_rares()
        if "casters" not in self.skip:
            self.plan_casters()
        if "links" not in self.skip:
            self.plan_links()
        if "places" not in self.skip:
            self.plan_places()

    # -- lookups
    def exists(self, name):
        if self.renaming and name.lower() == ARCTIC.lower():
            return True
        if self.renaming and name.lower() == ARTIC.lower():
            return False
        return bool(rows(self.db, "npcs", "lower(npc_name) = lower(?)", (name,)))

    def free_ids(self):
        taken = {r[0] for r in self.db.execute("SELECT id FROM npcs WHERE id BETWEEN ? AND ?", (ID_FROM, ID_TO))}
        if taken:
            self.bad.append(f"npcs ids {ID_FROM}-{ID_TO} are not free: {sorted(taken)[:5]}")
        return iter(range(ID_FROM, ID_TO))

    def find(self, name, world, x, z, camp):
        rs = rows(self.db, "npcs", "lower(npc_name) = lower(?) AND world = ?", (name, world))
        if x is None:
            if len(rs) != 1:
                self.bad.append(f"{camp}: {len(rs)} rows of {name!r} in world {world} (one expected)")
                return None
            return rs[0]
        near = sorted(rs, key=lambda r: math.hypot(r["x_coord"] - x, r["z_coord"] - z))
        if not near or math.hypot(near[0]["x_coord"] - x, near[0]["z_coord"] - z) > TOL:
            self.bad.append(f"{camp}: no {name!r} within {TOL} of ({x}, {z}) in world {world}")
            return None
        return near[0]

    # -- op makers
    def insert(self, tbl, values, key, camp, why):
        self.ops.append(("insert", tbl, key, values, camp, why))

    def update(self, tbl, key, values, camp, why):
        where = " AND ".join(f'"{c}" = ?' for c in key)
        before = row(self.db, tbl, where, tuple(key.values()))
        if before is None:
            self.bad.append(f"{camp}: {tbl} {key} is not there to update")
            return
        if all(before[c] == v for c, v in values.items()):
            return
        self.ops.append(("update", tbl, key, values, camp, why))

    def delete(self, tbl, key, camp, why):
        self.ops.append(("delete", tbl, key, None, camp, why))

    def new_npc(self, base, camp, why, **changes):
        nid = next(self.ids)
        r = dict(base)
        r.update(changes)
        r.update(id=nid, npc_id=nid, dt_kill=None)
        self.insert("npcs", r, {"id": nid}, camp, why)
        return nid

    def upsert_template_use(self, name, template, camp):
        cur = row(self.db, "npctemplate", "name = ?", (name,))
        if cur is None:
            self.insert("npctemplate", {"name": name, "template": template, "note": detail(camp), "source": SRC},
                        {"name": name}, camp, f"npctemplate {name} -> {template}")
        elif cur["template"] != template:
            if cur["template"] not in REPLACEABLE:
                self.bad.append(f"{camp}: npctemplate {name!r} is {cur['template']!r} (expected roamer or none)")
                return
            self.update("npctemplate", {"name": name}, {"template": template, "note": detail(camp), "source": SRC},
                        camp, f"npctemplate {name}: {cur['template']} -> {template}")

    def copy_loot(self, like, name, camp):
        """the camp kind's own loot (npcloot) and faction hits, and its loot region, for a new name"""
        for r in rows(self.db, "npcloot", "lower(npc_name) = lower(?)", (like,)):
            if not row(self.db, "npcloot", "npc_name = ? AND tablename = ?", (name.lower(), r["tablename"])):
                v = dict(r, npc_name=name.lower(), source=SRC, note=f"as {like}'s: {r['note']}"[:400])
                self.insert("npcloot", v, {"npc_name": name.lower(), "tablename": r["tablename"]}, camp,
                            f"loot {r['tablename']} as {like}'s")
        lc = row(self.db, "lootcreature", "lower(npc_name) = lower(?)", (like,))
        if lc and not row(self.db, "lootcreature", "lower(npc_name) = lower(?)", (name,)):
            self.insert("lootcreature", {"npc_name": name.lower(), "region": lc["region"], "profile": lc["profile"],
                                         "source": SRC}, {"npc_name": name.lower()}, camp, f"loot region as {like}'s")
        for r in rows(self.db, "factionhits", "lower(npc_name) = lower(?)", (like,)):
            if not row(self.db, "factionhits", "lower(npc_name) = lower(?) AND faction = ?", (name, r["faction"])):
                self.insert("factionhits", {"npc_name": name.lower(), "faction": r["faction"], "delta": r["delta"],
                                            "source": SRC}, {"npc_name": name.lower(), "faction": r["faction"]}, camp,
                            f"faction hit as {like}'s")

    # -- group 1
    def plan_rares(self):
        for rr in RARES:
            camp = rr["camp"]
            if rows(self.db, "npcs", "lower(npc_name) = lower(?)", (rr["name"],)):
                self.bad.append(f"{camp}: a row called {rr['name']!r} is there already")
                continue
            phs = [self.find(n, w, x, z, camp) for n, w, x, z in rr["placeholders"]]
            if None in phs:
                continue
            base = dict(phs[0])
            if rr["body"]:
                b = self.find(*rr["body"], camp)
                if b is None:
                    continue
                for c in ("modelid", "size", "primary_weapon", "secondary_weapon", "shield", "torso", "forearm", "gloves",
                          "legs", "feet", "head", "hair_color", "hair_length", "hair_style", "face", "robe_style", "race"):
                    base[c] = b[c]
            ch = {"npc_name": rr["name"], "npc_level": rr["level"]}
            if rr["size"]:
                ch["size"] = rr["size"]
            nid = self.new_npc(base, camp, rr["why"], **ch)
            self.new_rows[rr["name"]] = nid
            self.insert("npcspawnonly", {"name": f"#{nid}", "note": f"{rr['name']}: a rare on placeholders (npcrare): {SRC}",
                                         "source": SRC}, {"name": f"#{nid}"}, camp, "the rare's own row stands out of the world")
            for p in phs:
                if row(self.db, "npcrare", "npc_row = ?", (p["id"],)):
                    self.bad.append(f"{camp}: placeholder {p['id']} has a rare already")
                    continue
                self.insert("npcrare", {"npc_row": p["id"], "rare": rr["name"], "chance": rr["chance"],
                                        "note": f"{GUIDE}: {rr['why']}; {CHANCE_WHY}"[:600], "source": SRC},
                            {"npc_row": p["id"]}, camp, f"placeholder {p['npc_name']} at ({p['x_coord']:.0f}, {p['z_coord']:.0f})")
            self.upsert_template_use(rr["name"].lower(), rr["template"], camp)
            self.copy_loot(rr["like"], rr["name"], camp)
        # the invader
        iv = INVADER
        camp = iv["camp"]
        if rows(self.db, "npcs", "lower(npc_name) = lower(?)", (iv["name"],)):
            self.bad.append(f"{camp}: a row called {iv['name']!r} is there already")
        else:
            b = self.find(*iv["base"], camp)
            if b:
                w, x, y, z = iv["at"]
                nid = self.new_npc(b, camp, iv["why"], npc_name=iv["name"], npc_level=iv["level"], world=w, x_coord=x,
                                   y_coord=y, z_coord=z)
                self.new_rows[iv["name"]] = nid
                self.insert("npcrespawn", {"npc_row": nid, "seconds": iv["respawn"], "note": detail(camp) + ": a rarer roamer, 20 min",
                                           "source": SRC}, {"npc_row": nid}, camp, "20 min respawn")
                mv = row(self.db, "npcmove", "lower(name) = lower(?)", (iv["name"],))
                if mv is None:
                    self.insert("npcmove", {"name": iv["name"].lower(), "roam": 1, "radius": iv["roam_radius"], "water": "land",
                                            "note": "the guide's 'rarer roamer'", "source": SRC, "speed": 0.0},
                                {"name": iv["name"].lower()}, camp, "wanders 60 round its spawn")
                self.upsert_template_use(iv["name"].lower(), T_CLERIC, camp)
                self.copy_loot("froglok inquisitor", iv["name"], camp)
        # respawns
        for camp, name, w, x, z, secs, why in RESPAWNS:
            r = self.find(name, w, x, z, camp)
            if r is None:
                continue
            cur = row(self.db, "npcrespawn", "npc_row = ?", (r["id"],))
            v = {"seconds": secs, "note": f"{detail(camp)}: {why}", "source": SRC}
            if cur is None:
                self.insert("npcrespawn", dict(v, npc_row=r["id"]), {"npc_row": r["id"]}, camp, f"{name} respawn {secs:.0f} s")
            else:
                self.update("npcrespawn", {"npc_row": r["id"]}, v, camp, f"{name} respawn {cur['seconds']:.0f} -> {secs:.0f} s")
        # the stray Boss Hurg
        camp, name, w, x, z = STRAY_HURG
        r = self.find(name, w, x, z, camp)
        if r:
            if r["npc_level"] != 44:
                self.bad.append(f"{camp}: the stray {name} is L{r['npc_level']}, not 44")
            else:
                for tbl, col in (("npcrespawn", "npc_row"), ("npcwander", "npcid"), ("npcpaths", "npcid"),
                                 ("npcrare", "npc_row"), ("campspawns", "npcid")):
                    if has(self.db, tbl) and row(self.db, tbl, f"{col} = ?", (r["id"],)):
                        self.delete(tbl, {col: r["id"]}, camp, f"the stray {name}'s {tbl} row")
                for tbl in ("npctemplate", "npcspawnonly"):
                    if row(self.db, tbl, "name = ?", (f"#{r['id']}",)):
                        self.delete(tbl, {"name": f"#{r['id']}"}, camp, f"the stray {name}'s {tbl} row")
                self.delete("npcs", {"id": r["id"]}, camp, f"the stray L44 {name} at Deathfist Horde (Bloodfoot's is L33 on Mount Hatespike)")

    # -- group 2
    def plan_casters(self):
        roamer = row(self.db, "classtemplates", "name = ?", (ROAMER,))
        if roamer is None:
            self.bad.append("no roamer template")
            return
        for name, (cls, abilities, over, note) in TEMPLATES.items():
            v = dict(roamer)
            v.update(name=name, **{"class": cls}, abilities=json.dumps(abilities, separators=(",", ":")),
                     note=f"{GUIDE} (the owner 2026-11-21): {note}; on the roamer's figures and crit", source=SRC)
            v.update(over)
            cur = row(self.db, "classtemplates", "name = ?", (name,))
            if cur is None:
                self.insert("classtemplates", v, {"name": name}, "templates", note[:90])
            elif any(cur[k] != v[k] for k in v):
                self.bad.append(f"classtemplates {name!r} is there and differs")
        for camp, name, t in BY_NAME:
            if not self.exists(name):
                if name != "artic spiritcaller":
                    self.notes.append(f"{camp}: no row of {name!r} on this shard (its template use left)")
                continue
            self.upsert_template_use(name, t, camp)
        # the miners by body
        camp = "MINERS"
        for r in rows(self.db, "npcs", "lower(npc_name) IN (?, ?, ?)", MINERS):
            if not in_area(r, GREAT_WASTE):
                continue
            if (r["modelid"] or 0) & 0xFFFFFFFF in ERUDITE:
                t, kind = T_AOEWIZ, "Erudite: the AoE wizard"
            elif (r["torso"] or 0) >= 4:
                t, kind = T_CLERIC, "full body armour: the cleric"
            else:
                t, kind = T_PALADIN, "the other body: the paladin"
            self.row_template(r, t, camp, kind)
        # the Centaur Valley undead: every other row of each name a druid
        camp = "CV UNDEAD"
        for name in CV_UNDEAD:
            rs = sorted((r for r in rows(self.db, "npcs", "lower(npc_name) = ?", (name,)) if in_area(r, CENTAUR_VALLEY)),
                        key=lambda r: r["id"])
            for i, r in enumerate(rs):
                self.row_template(r, T_UKNIGHT if i % 2 == 0 else T_UDRUID, camp,
                                  "SK-type" if i % 2 == 0 else "druid-type")
        # the sages' wolf spirits
        camp = "FROSTEYE ORCS"
        bases = [r for r in rows(self.db, "npcs", "npc_name = ? AND modelid = ?", PET_BASE)]
        if not bases:
            self.bad.append(f"{camp}: no {PET_BASE[0]} row with body {PET_BASE[1]} to copy")
        else:
            base = sorted(bases, key=lambda r: r["id"])[0]
            for (w, sx, sz), (x, y, z) in SAGE_PETS:
                s = self.find("frosteye sage", w, sx, sz, camp)
                if s is None:
                    continue
                if rows(self.db, "npcs", "npc_name = ? AND world = ? AND abs(x_coord - ?) < 4 AND abs(z_coord - ?) < 4",
                        (PET_BASE[0], w, s["x_coord"], s["z_coord"])):
                    self.notes.append(f"{camp}: the sage at ({sx}, {sz}) has a spirit already")
                    continue
                self.new_npc(base, camp, "the guide: 'The sages are shaman-type mobs, complete with DR-nukes and wolf pets' "
                             "(a Kindred Spirit, a companion kind, with the wolf body of the L46-64 shaman pets)",
                             world=w, x_coord=x, y_coord=y, z_coord=z, npc_level=s["npc_level"] - 4, facing=s["facing"],
                             zone=s["zone"])
        # see invisible
        for camp, name in SEE_INVIS:
            if not rows(self.db, "npcs", "lower(npc_name) = lower(?)", (name,)):
                self.bad.append(f"{camp}: no row of {name!r}")
                continue
            cur = row(self.db, "npcaggro", "lower(npc_name) = lower(?)", (name,))
            if cur is None:
                self.insert("npcaggro", {"npc_name": name, "aggressive": -1, "radius": 0.0, "leash": 0.0, "see_invis": 1,
                                         "holds": 0, "note": f"{detail(camp)}: sees invisible", "source": SRC},
                            {"npc_name": name}, camp, f"{name} sees invisible (aggressive -1: the world's rules otherwise)")
            elif not cur["see_invis"]:
                self.update("npcaggro", {"npc_name": cur["npc_name"]}, {"see_invis": 1}, camp, f"{name} sees invisible")

    def row_template(self, r, t, camp, why):
        k = f"#{r['id']}"
        cur = row(self.db, "npctemplate", "name = ?", (k,))
        if cur is None:
            self.insert("npctemplate", {"name": k, "template": t, "note": f"{detail(camp)}: {why}", "source": SRC},
                        {"name": k}, camp, f"{r['npc_name']} {k} ({why}) -> {t}")
        elif cur["template"] != t:
            self.bad.append(f"{camp}: npctemplate {k} is {cur['template']!r} already")

    # -- group 3
    def plan_links(self):
        for camp, name, faction in LINKS:
            if not self.exists(name):
                if name.lower() == ARTIC.lower():
                    continue
                self.bad.append(f"{camp}: no row of {name!r}")
                continue
            cur = row(self.db, "npcsocial", "lower(npc_name) = lower(?)", (name,))
            if cur is None:
                self.insert("npcsocial", {"npc_name": name, "social": 1, "faction": faction, "radius": 0.0,
                                          "note": f"{detail(camp)}: linked", "source": SRC},
                            {"npc_name": name}, camp, f"{name} social, faction {faction}")
            elif cur["social"] != 1 or cur["faction"] != faction:
                self.bad.append(f"{camp}: npcsocial {name!r} is there ({cur['social']}, {cur['faction']!r})")

    # -- group 4
    def plan_places(self):
        camp = "MERKANS"
        for name, up in MERKAN_LEVELS:
            for r in rows(self.db, "npcs", "lower(npc_name) = ?", (name,)):
                if not in_area(r, STONE_OF_MORTHALIS):
                    continue
                if r["npc_level"] > 25:
                    self.bad.append(f"{camp}: {name} {r['id']} is L{r['npc_level']} already")
                    continue
                self.update("npcs", {"id": r["id"]}, {"npc_level": r["npc_level"] + up}, camp,
                            f"{name} L{r['npc_level']} -> {r['npc_level'] + up} (the guide: villagers 'around level 32')")
        if "ratmen" not in self.skip:
            camp = "RATMEN (41-50)"
            rs = [r for r in rows(self.db, "npcs", "lower(npc_name) IN (?, ?)", RATMEN["names"]) if in_area(r, RATMEN["area"])]
            if len(rs) != len(RATMEN["y"]):
                self.bad.append(f"{camp}: {len(rs)} ratmen in The Vastly Deep, {len(RATMEN['y'])} expected")
            for r in rs:
                k = key_of(r["x_coord"], r["z_coord"])
                if k not in RATMEN["y"]:
                    self.bad.append(f"{camp}: {r['npc_name']} {r['id']} stands at {k}, not where its new place was measured")
                    continue
                v = {"x_coord": round(round(r["x_coord"], 1) + RATMEN["dx"], 1), "y_coord": RATMEN["y"][k],
                     "z_coord": round(round(r["z_coord"], 1) + RATMEN["dz"], 1)}
                self.update("npcs", {"id": r["id"]}, v, camp,
                            f"{r['npc_name']} {k} -> {v['x_coord']},{v['z_coord']} (South Barren Coast N, the camp's shape kept)")
        camp = "TFGS - TENACIOUS FROST GIANTS"
        src = [r for r in rows(self.db, "npcs", "lower(npc_name) = ?", (TFG["name"],)) if in_area(r, TFG["area"])]
        if len(src) != 27:
            self.bad.append(f"{camp}: {len(src)} giants in the Permafrost camp, 27 expected")
        tx, tz = TFG["dx"], TFG["dz"]
        if rows(self.db, "npcs", "lower(npc_name) = ? AND world = 0 AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?",
                (TFG["name"], 4700 + tx, 5150 + tx, 5350 + tz, 5800 + tz)):
            self.bad.append(f"{camp}: giants stand at the new camp's place already")
            src = []
        for r in sorted(src, key=lambda r: r["id"]):
            k = key_of(r["x_coord"], r["z_coord"])
            if k not in TFG["y"]:
                self.bad.append(f"{camp}: giant {r['id']} stands at {k}, not where its copy's place was measured")
                continue
            nid = self.new_npc(r, camp, f"the guide: 3 camps, 'the last one northeast of the one in Permafrost': a copy of "
                                        f"giant {r['id']} {tx:+.0f},{tz:+.0f} off",
                               x_coord=round(round(r["x_coord"], 1) + tx, 1), y_coord=TFG["y"][k],
                               z_coord=round(round(r["z_coord"], 1) + tz, 1))
            wd = row(self.db, "npcwander", "npcid = ?", (r["id"],))
            if wd:
                self.insert("npcwander", dict(wd, npcid=nid, source=SRC), {"npcid": nid}, camp, f"wanders as giant {r['id']}")
            rp = row(self.db, "npcrespawn", "npc_row = ?", (r["id"],))
            if rp:
                self.insert("npcrespawn", dict(rp, npc_row=nid, source=SRC), {"npc_row": nid}, camp, f"respawn as giant {r['id']}")
        # Live's misspelt Spiritcaller
        camp = "TFGS - TENACIOUS FROST GIANTS"
        artic = rows(self.db, "npcs", "npc_name = ?", (ARTIC,))
        arctic = rows(self.db, "npcs", "npc_name = ?", (ARCTIC,))
        if artic and arctic:
            self.notes.append(f"{camp}: this shard has both {ARTIC!r} ({[r['id'] for r in artic]}) and {ARCTIC!r} "
                              f"({[r['id'] for r in arctic]}) standing together: left as they are (not this tool's call)")
        elif artic:
            for r in artic:
                self.update("npcs", {"id": r["id"]}, {"npc_name": ARCTIC}, camp, f"{ARTIC} -> {ARCTIC} (Heirloom's spelling)")
            for tbl, col in (("npcloot", "npc_name"), ("npcclass", "name"), ("lootcreature", "npc_name"),
                             ("npctemplate", "name"), ("npcsocial", "npc_name"), ("npcaggro", "npc_name"),
                             ("npcmove", "name"), ("factionhits", "npc_name")):
                for r in rows(self.db, tbl, f"lower({col}) = ?", (ARTIC.lower(),)):
                    pk = {"npcloot": ("npc_name", "tablename"), "factionhits": ("id",)}.get(tbl, (col,))
                    key = {c: r[c] for c in pk}
                    clash = row(self.db, tbl, " AND ".join([f"lower({col}) = ?"] + [f"{c} = ?" for c in pk if c != col]),
                                (ARCTIC.lower(), *[r[c] for c in pk if c != col]))
                    if clash:
                        self.notes.append(f"{camp}: {tbl} has {ARCTIC.lower()!r} already beside {key}: the old row left")
                        continue
                    self.update(tbl, key, {col: ARCTIC.lower()}, camp, f"{tbl} {ARTIC.lower()} -> {ARCTIC.lower()}")


# ---------------------------------------------------------------- report, apply, undo
def show(p):
    for op, tbl, key, values, camp, why in p.ops:
        extra = ""
        if op == "insert" and tbl == "npcs":
            extra = (f" {values['npc_name']} L{values['npc_level']} w{values['world']} "
                     f"({values['x_coord']:.1f}, {values['y_coord']:.2f}, {values['z_coord']:.1f})")
        elif op == "update":
            extra = " " + js(values)
        print(f"  [{camp}] {op} {tbl} {js(key)}{extra}: {why}")
    for n in p.notes:
        print(f"  note: {n}")
    c = {}
    for op in p.ops:
        c[(op[0], op[1])] = c.get((op[0], op[1]), 0) + 1
    print("  totals: " + ", ".join(f"{o} {t} {n}" for (o, t), n in sorted(c.items())))


def check_ground(p, binary, ground):
    """every new or moved npcs place, asked of cmd/campground: on a floor within 0.3 of its height, not roofed, not
    embedded, dry"""
    qs = []
    for i, (op, tbl, key, values, camp, why) in enumerate(p.ops):
        if tbl != "npcs" or op == "delete":
            continue
        if op == "update" and "x_coord" not in values:
            continue
        if op == "insert" and values["npc_name"] in [rr["name"] for rr in RARES]:
            continue  # out of the world: made on its placeholder's spot
        w = values.get("world")
        if w is None:
            w = row(p.db, "npcs", "id = ?", (key["id"],))["world"]
        qs.append((i, w, values["x_coord"], values["y_coord"], values["z_coord"]))
    lines = [json.dumps({"op": "spot", "k": i, "w": w, "x": x, "z": z, "hint": y, "fx": x, "fy": y, "fz": z})
             for i, w, x, y, z in qs]
    out = subprocess.run([binary, "-ground", ground], input="\n".join(lines) + "\n", capture_output=True, text=True,
                         check=True).stdout.splitlines()
    bad = []
    for (i, w, x, y, z), line in zip(qs, out):
        a = json.loads(line)
        if not a.get("knows"):
            bad.append(f"world {w} has no ground files: ({x}, {z}) not checked")
        elif a.get("stand") is None or abs(a["stand"] - y) > 0.3 or a.get("roofed") or a.get("embedded") or a.get("env"):
            bad.append(f"({x}, {y}, {z}) w{w}: {a}")
    return len(qs), bad


def apply(db, p):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, before TEXT, "
               "after TEXT, camp TEXT NOT NULL)")
    n = 0
    for op, tbl, key, values, camp, why in p.ops:
        where = " AND ".join(f'"{c}" = ?' for c in key)
        before = row(db, tbl, where, tuple(key.values()))
        if op == "insert":
            if before is not None:
                raise RuntimeError(f"{tbl} {key} is there already")
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in values)}) VALUES '
                       f'({", ".join("?" * len(values))})', list(values.values()))
        elif op == "update":
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in values)} WHERE {where}',
                       list(values.values()) + list(key.values()))
            key = {c: (values.get(c, v)) for c, v in key.items()}  # a renamed key column
            where = " AND ".join(f'"{c}" = ?' for c in key)
        else:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        after = row(db, tbl, where, tuple(key.values()))
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, camp) VALUES (?, ?, ?, ?, ?)",
                   (tbl, js(key), js(before), js(after), camp))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, ACCOUNT, WHO, VIA, op, tbl, js(key), js(before), js(after), f"{detail(camp)} ({why})"[:1000]))
        n += 1
    return n


def undo(db):
    n, left = 0, 0
    for tbl, rowkey, before, after in db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        where = " AND ".join(f'"{c}" = ?' for c in key)
        now = row(db, tbl, where, tuple(key.values()))
        if js(now) != after:
            print(f"  {tbl} {rowkey}: changed since this tool wrote it, left as it is")
            left += 1
            continue
        if now is not None:
            db.execute(f'DELETE FROM "{tbl}" WHERE {where}', tuple(key.values()))
        if before is not None:
            b = json.loads(before)
            db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in b)}) VALUES ({", ".join("?" * len(b))})',
                       list(b.values()))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, '{}', NULL, NULL, ?)",
               (datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"), ACCOUNT, WHO, VIA, LOG,
                f"undone, {n} rows put back, {left} left: the owner 2026-11-21: camp audit vs MasterVG782's guide"))
    print(f"  {n} rows put back, {left} left")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--applied", action="store_true")
    g.add_argument("--undo", action="store_true")
    ap.add_argument("--skip", action="append", default=[], choices=PARTS)
    ap.add_argument("--campground", help="cmd/campground's binary: check every new or moved place first")
    ap.add_argument("--ground", help="the ground files' directory for --campground")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.applied:
            if not has(db, LOG):
                print("not applied")
                return 1
            for tbl, key, before, after, camp in db.execute(f"SELECT tbl, rowkey, before, after, camp FROM {LOG} ORDER BY seq"):
                print(f"  [{camp}] {tbl} {key}: {'insert' if before is None else 'delete' if after is None else 'update'}")
            return 0
        if a.undo:
            if not has(db, LOG):
                print("nothing to undo")
                return 0
            db.execute("BEGIN IMMEDIATE")
            undo(db)
            db.execute("COMMIT")
            print("undone; takes effect at a restart")
            return 0
        if has(db, LOG):
            print(f"applied already ({LOG} is there): --undo to take it back")
            return 0
        p = Plan(db, a.skip)
        show(p)
        if a.campground:
            n, bad = check_ground(p, a.campground, a.ground or "worlds/ground")
            print(f"  ground: {n} places checked, {len(bad)} wrong")
            p.bad += [f"ground: {b}" for b in bad]
        if p.bad:
            print("refused, nothing done:\n  " + "\n  ".join(p.bad))
            return 1
        if not a.apply:
            print("report only: --apply to write it")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, p)
        except Exception:
            db.execute("ROLLBACK")
            raise
        db.execute("COMMIT")
        print(f"applied, {n} rows; takes effect at the next restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
