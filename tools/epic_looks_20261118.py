#!/usr/bin/env python3
"""Items wearing an epic weapon's look that are no epic and no heirloom get a regular look (the owner, 2026-11-18: "if
not an epic or heirloom item lets change items that have an epic weapon model but not an epic to a regular model").

THE EPIC MODELS are ITEM.ESF's hand models 44-78, the artist's gallery's 35 epic weapons in its alphabetical order
(claude-notes/epic-weapons-looks-20261106.md; checked against ITEM.ESF's order in ~/.cache/bowlooks/index.txt and the
renders: EPIC_MODELS below, with each one's ITEMICON plate).

AN EPIC is a record that (any one):
  - is a reward of an epic quest: "The <Class>'s Reward, Level 49" (30,40,49/<group>/L49-<class>) or "The <Class>'s
    Epic Reward" (60/<group>/reward-<class>), a step's rewarditems or an option's items;
  - is an Ascended copy of one or the base of one (itemascended);
  - is an heirloom's epic (heirlooms.epic);
  - is named as an epic (EPIC_NAMES: the above off both shards, the 2026-11-06 note's 49s, EQOA-Data's epics_49/60
    item lists), with or without "Ascended ", or an epic's name followed by " (" (the transfer stand-in "Staff of
    Nebula (1H)"), or has "Epic" in its name.
AN HEIRLOOM is a record of table heirlooms, or named "Heirloom ...".
KEPT as the owner made them: Twinkle and Icingdeath (Drizzt's weapons, the owner 2026-11-16, KEEP_NAMES).

THE NEW LOOK, of the item's own kind (hands: wear 15 two-handed, else one; damage: its attack type, slash 1/2, blunt
3/4, pierce 5/6; a held item, wear 18 or not worn, by its name):
  1. a record of the same name (any case) on a regular hand model of the same kind: that model (and its icon);
  2. else what its name says it is (FAMILY_RULES: a dagger, a spear, an axe, a book, an orb ...), the epic's own
     closest regular model where the family is the epic's own (EPIC_OWN), else the family's commonest model;
  3. else the epic's closest regular model of that kind (EPIC_OWN), else the kind's commonest (KIND_DEFAULT).
  Every model chosen is a regular hand model (none of 44-78, nor the Sword of Virtue's 88EC092D), each picked by eye
  from the disc's renders and among the ones comparable records already wear (the report's contact sheet).
  THE ICON follows only where the item's icon is an epic's icon (EPIC_ICONS): it takes the new model's commonest
  icon among the regular records that wear it (MODEL_ICON). Every other column (stats, procs, auras, colour) is kept.

NPC ROWS (npcs.primary_weapon / secondary_weapon / shield) carrying an epic model take the epic's closest regular model
of its own kind (NPC_LOOK). None is an epic's own boss or quest holder (checked against the epic quests' givers and
targets), so none is kept; KEEP_NPC is empty.

One transaction: every row changed has its gm_audit line (as the panel's row edit, via "tool") and a row in
epic_looks_1118_log; --undo puts back exactly what was there (a row changed since is left and said); a second --apply
finds nothing to do. Report only by default; --report FILE writes the item -> old -> new table (TSV).

Live: item records at once (a held copy shows the new look at its holder's next login, or a re-equip); the NPCs'
weapons at the panel's "Reload every NPC" (or a restart).

    python3 -E -s tools/epic_looks_20261118.py --db eqoa.db [--apply | --undo] [--report FILE]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "epic_looks_1118_log"
DETAIL = ("Items wearing an epic weapon's look that are no epic and no heirloom given a regular look of their kind "
          "(the owner, 2026-11-18); tools/epic_looks_20261118.py")
WHO, VIA = "dab", "tool"

# ITEM.ESF hand models 44-78: DictID -> (gallery name, ITEMICON plate)
EPIC_MODELS = {
    0x9EA9384F: ("Bard_MainGauche", -1270313026), 0xF4DD991A: ("Bard_Rapier", 1046403209),
    0x8E85E4A6: ("Bard_Sabre", -1728866781), 0xD8406C85: ("Cleric_Greathammer", -1279264174),
    0xB2116A98: ("Cleric_Hammer", -439584577), 0xD03942DE: ("Druid_Scimitar", 2051918763),
    0xCBF4135A: ("Druid_Staff", -593164599), 0x2EDA5089: ("Enchanter_Rod", -611086336),
    0xDAF955A6: ("Enchanter_Staff", -1368870331), 0x784CC2A6: ("Magician_Rod", 1023374315),
    0x63DCEFAB: ("Magician_Staff", 1473918600), 0x039995A5: ("Monk_Claws", 259449042),
    0xAACB29AF: ("Monk_Knuckles", 1830242566), 0x1D875ECD: ("Monk_Staff", -1010960774),
    0x5FD4AE7C: ("Necromancer_Sickle", None), 0xFD9C3858: ("Necromancer_Totem", -1341171857),
    0x8A82B605: ("Paladin_Greatsword", 913980370), 0xF2877ACE: ("Paladin_Lance", -1100218923),
    0xF841266D: ("Paladin_Warhammer", -971859436), 0x11666A69: ("Ranger_Axe", 1081969366),
    0xF3A41E80: ("Ranger_Longsword", -903203787), 0xAB6B7D29: ("Ranger_Scimitar", 1086674104),
    0xDD61A3D4: ("Rogue_Dagger", 621242233), 0xBD3C3DC8: ("Rogue_Dirk", 1572730341),
    0x0D54F88C: ("Rogue_Stiletto", 571603897), 0x9A1DBA92: ("ShadowKnight_Greataxe", 1719285689),
    0x6EB7224D: ("ShadowKnight_Greatsword", -1604900612), 0x867ED577: ("ShadowKnight_Polearm", 1296932780),
    0xDF034C5A: ("Shaman_Club", 469503945), 0x38107CE9: ("Shaman_Spear", 440403230),
    0x942735D3: ("Warrior_Greataxe", 569415544), 0xB44E7A96: ("Warrior_Greatsword", 445518418),
    0x17F9870B: ("Warrior_Longsword", -1725988846), 0x29B36334: ("Wizard_Rod", 747083025),
    0x7C7DBEA9: ("Wizard_Staff", 1682985694),
}
NOT_REGULAR = set(EPIC_MODELS) | {0x88EC092D}  # the Sword of Virtue's own (the paladin's 60 epic)
# the epics' plates; the necromancer sickle's stand-in 1997486978 is a regular sickle's, so not here
EPIC_ICONS = {i for _, i in EPIC_MODELS.values() if i is not None}

# the regular models used (ITEM.ESF index: what it is)
M = {
    "sword": 0xBE0B682B,      # 146 sword (121 records)
    "longsword": 0xD2FD0C8C,  # 193 longsword (75)
    "scimitar": 0x47D981E0,   # 148 scimitar / sabre (190)
    "katana": 0x6618F8B2,     # 108 katana / shamshir, single-edged (39)
    "broadsword": 0x6C86AF03,  # 80 broad ornate blade (Bloodfyre Broadsword, Great Sword of the Elddar)
    "wavy": 0x8E8DB41F,       # 86 wavy flamberge blade (Prismatic Steel Sword, Rotblade Claymore)
    "rapier": 0x462FE629,     # 145 rapier / foil (48)
    "cutlass": 0x3C2B42A5,    # 199 cutlass / claymore
    "cleaver": 0x60960E46,    # 22 cleaver
    "sickle": 0xCC647CDF,     # 168 sickle
    "kama": 0x0A265AFA,       # 107 kama
    "scythe": 0x66073B90,     # 149 scythe
    "hatchet": 0x20F27B0B,    # 104 flat axe blade (80 one-handed axes)
    "waraxe": 0x024DB2A6,     # 10 double-bitted waraxe (Brutal Headaxe, Waraxe of Noroxus)
    "greataxe": 0xEC05910B,   # 13 greataxe (247)
    "glaive": 0x3694FA44,     # 89 glaive / guisarme (41)
    "claymore": 0x67B40AE4,   # 96 claymore / greatsword (200)
    "mace": 0x0A6AF116,       # 123 flanged mace (188)
    "hammer": 0x1246E528,     # 101 block hammer, wooden haft (145)
    "warhammer": 0x1D1E59E2,  # 195 steel warhammer (105 two-handed)
    "club": 0x0916D39E,       # 23 club (55)
    "greatclub": 0x944F4462,  # 194 spiked greatclub
    "mace2h": 0x4261AF2B,     # 175 two-handed mace / maul
    "staff": 0x026DA0CC,      # 143 plain quarterstaff / bo (60)
    "crook": 0x63003585,      # 176 crook staff (82)
    "crystalcrook": 0xADC8FFDB,  # 178 banded crook with a crystal (Blizzard Staff, Heavy Wizard's Staff)
    "branchstaff": 0x2B340FD4,   # 190 golden forked branch staff (Staff of the Jade Forest)
    "bonestaff": 0x9D67A85B,  # 184 bone staff with a skull (Hallowbone Spiked Staff, Bloodmire Relic)
    "knuckles": 0x11EE9E59,   # 115 bladed knuckles
    "fist": 0x097CF260,       # 84 fist / iron knuckles
    "claws": 0xB4D5ADE9,      # 85 bone claws (Xioshezmon, Slime Covered Bear Paw)
    "dagger": 0xD9069A64,     # 164 dagger (233)
    "stiletto": 0x83BE855A,   # 32 thin dagger / parrying dagger (Ice Needle, Crow's Parrying Dagger)
    "kris": 0x8DD0BA16,       # 33 wavy kris (Elegant Dirk, Nightmorrow Kris)
    "spear": 0xFF96D99F,      # 103 one-handed spear / harpoon
    "partizan": 0x478B7440,   # 173 partizan / ranseur (74)
    "longspear": 0xFEFDC1E9,  # 121 longspear
    "lance": 0x432E88B3,      # 117 lance
    "trident": 0x8C257F7A,    # 189 trident
    "tome": 0xBF7E7DCB,       # 17 book (Mystical Tome)
    "orb": 0x9CCAF5A8,        # 114 orb (Quakestone Orb, Earthflow Orb)
    "totem": 0x699DAD40,      # 185 horned totem (Totem of the Brute, Icebear Fetish)
    "crystalwand": 0x8BB07164,  # 43 crystal-headed rod (Serene Wand of Leaves, Priest's Holy Symbol)
    "orbwand": 0x1EBC7693,    # 3 glass-orb wand (Brushfire Wand, Summoner's Wand)
    "moonwand": 0xDBED3481,   # 4 orb wand with a dark crescent (Turtle Shell Wand, Dark Implement)
    "charm": 0x4A6F1D38,      # 40 crystal charm (Bogwater Crystal, Pine Coated Charm)
}

# a regular model's icon: its commonest among the regular records wearing it (Heirloom shard, 2026-11-18)
MODEL_ICON = {
    M["sword"]: -1261834373, M["longsword"]: 901798929, M["scimitar"]: 506409893, M["katana"]: 724322735,
    M["broadsword"]: 227957592, M["wavy"]: -955781738, M["rapier"]: -1778984458, M["cutlass"]: 1254320386,
    M["cleaver"]: -594224747, M["sickle"]: 47323628, M["kama"]: -1240893553, M["scythe"]: -1601496691,
    M["hatchet"]: 1822970346, M["waraxe"]: 219574349, M["greataxe"]: -1545510330, M["glaive"]: 1612474497,
    M["claymore"]: -921006351, M["mace"]: -1405600909, M["hammer"]: 980933879, M["warhammer"]: -133430711,
    M["club"]: -1405600909, M["greatclub"]: -1877811087, M["mace2h"]: -597854559, M["staff"]: -597854559,
    M["crook"]: 25125364, M["crystalcrook"]: -2060152422, M["branchstaff"]: -2055321559, M["bonestaff"]: 443349912,
    M["knuckles"]: -1657844872, M["fist"]: 960488149, M["claws"]: -1665822312, M["dagger"]: -541133199,
    M["stiletto"]: 1681985143, M["kris"]: 1888840149, M["spear"]: 109831808, M["partizan"]: 109831808,
    M["longspear"]: 109831808, M["lance"]: -907954022, M["trident"]: 1202280489, M["tome"]: 1856243648,
    M["orb"]: -1224776763, M["totem"]: -220702867, M["crystalwand"]: -278482247, M["orbwand"]: -562721,
    M["moonwand"]: -530524322, M["charm"]: -868600977,
}

# a kind: (hands 1/2 or 0 held, damage S/B/P/H or "" held)
KIND_DEFAULT = {
    (1, "S"): "sword", (2, "S"): "claymore", (1, "B"): "mace", (2, "B"): "warhammer", (1, "P"): "dagger",
    (2, "P"): "partizan", (1, "H"): "fist", (2, "H"): "warhammer", (0, ""): "charm",
}
# what a name says the item is: (regex, {hands: family}); first match wins
FAMILY_RULES = [
    (r"\b(book|tome|tomb|journal|manual|writ|grimoire|guide|catalogue|compendium|history|log|izz ta geth)\b", {0: "tome"}),
    (r"\b(orb|eye|sphere|globe|gaze|polyhedron|dice)\b", {0: "orb"}),
    (r"\b(totem|fetish|skull|horn|paw|beads|icon|statue|ward|talisman|deity|diety)\b", {0: "totem"}),
    (r"\b(wand|rod|scepter|sceptre|crosier|focus)\b", {0: "wand", 1: "wand"}),
    (r"\b(club|mace)\b", {0: "wand"}),
    (r"\b(staff|stick|crook|limb|bo|bloodstaff)\b", {0: "wand", 1: "staff", 2: "staff"}),
    (r"\b(claws?)\b", {1: "claws", 2: "claws"}),
    (r"\b(knuckles?|dusters|fists?)\b", {1: "knuckles"}),
    (r"\b(dagger|dirk|knife|shank|kris)\b", {1: "dagger", 2: "dagger"}),
    (r"\b(stiletto|main gauche)\b", {1: "stiletto"}),
    (r"\b(rapier|foil)\b", {1: "rapier"}),
    (r"\b(kama)\b", {1: "kama"}),
    (r"\b(sickle)\b", {1: "sickle"}),
    (r"\b(scythe)\b", {1: "sickle", 2: "scythe"}),
    (r"\b(cleaver)\b", {1: "cleaver", 2: "cleaver"}),
    (r"\b(partizan|ranseur|ahlspiess|pike|halberd)\b", {1: "spear", 2: "partizan"}),
    (r"\b(glaive|guisarme)\b", {2: "glaive"}),
    (r"\b(lance)\b", {1: "spear", 2: "lance"}),
    (r"\b(trident)\b", {2: "trident"}),
    (r"\b(spear|harpoon|spine)\b", {1: "spear", 2: "partizan"}),
    (r"\b(waraxe|headaxe|battle axe)\b", {1: "waraxe", 2: "greataxe"}),
    (r"(axe|hatchet|chopper|reaver|splitter)\b", {1: "hatchet", 2: "greataxe"}),
    (r"\b(scimitar|shamshir|sabre|saber)\b", {1: "scimitar", 2: "claymore"}),
    (r"\b(katana|no dachi)\b", {1: "katana", 2: "katana"}),
    (r"\b(cutlass)\b", {1: "cutlass", 2: "cutlass"}),
    (r"\b(flamberge|beheader|claymore|zweihander|greatsword|warsword)\b", {1: "sword", 2: "claymore"}),
    (r"\b(longsword|longblade|warblade)\b", {1: "longsword", 2: "claymore"}),
    (r"\b(broadsword)\b", {1: "broadsword", 2: "broadsword"}),
    (r"\b(sword|blade|edge|slicer)\b", {1: "sword", 2: "claymore"}),
    (r"\b(warhammer|battlehammer|maul)\b", {1: "warhammer", 2: "warhammer"}),
    (r"\b(hammer|mallet|sunder|glacialsunder)\b", {1: "hammer", 2: "warhammer"}),
    (r"\b(mace|morningstar|star)\b", {1: "mace", 2: "mace2h"}),
    (r"\b(club|greatclub|warclub|cudgel|bludgeoner|crusher|bonecracker|root)\b", {1: "club", 2: "greatclub"}),
]
# the families a sword, staff, wand ... of each epic's own look takes (its closest regular model by eye); a family
# missing here takes M[family]
EPIC_OWN = {
    "Bard_Rapier": {"rapier": "rapier", "sword": "rapier"},
    "Bard_Sabre": {"sword": "katana", "scimitar": "katana"},
    "Bard_MainGauche": {"dagger": "stiletto", "stiletto": "stiletto", "sword": "stiletto"},
    "Druid_Scimitar": {"sword": "scimitar"},
    "Ranger_Scimitar": {"sword": "scimitar"},
    "Ranger_Longsword": {"sword": "longsword"},
    "Paladin_Greatsword": {"sword": "longsword", "claymore": "claymore"},
    "Warrior_Longsword": {"sword": "scimitar"},
    "Warrior_Greatsword": {"sword": "broadsword", "claymore": "broadsword", "longsword": "broadsword"},
    "ShadowKnight_Greatsword": {"sword": "wavy", "claymore": "wavy", "longsword": "wavy", "dagger": "wavy"},
    "Rogue_Dagger": {"sword": "sword", "dagger": "dagger"},
    "Rogue_Dirk": {"dagger": "kris"},
    "Rogue_Stiletto": {"dagger": "stiletto"},
    "Ranger_Axe": {"hatchet": "hatchet", "greataxe": "greataxe", "claymore": "greataxe"},
    "Warrior_Greataxe": {"hatchet": "waraxe", "greataxe": "greataxe", "claymore": "greataxe"},
    "ShadowKnight_Greataxe": {"hatchet": "waraxe", "greataxe": "greataxe", "claymore": "greataxe"},
    "Cleric_Hammer": {"hammer": "hammer", "warhammer": "warhammer", "mace": "hammer"},
    "Paladin_Warhammer": {"hammer": "warhammer", "warhammer": "warhammer", "mace": "warhammer"},
    "Cleric_Greathammer": {"staff": "crystalcrook", "warhammer": "warhammer", "mace": "hammer"},
    "Shaman_Club": {"club": "club", "greatclub": "greatclub", "mace": "club", "warhammer": "greatclub"},
    "Druid_Staff": {"staff": "branchstaff", "warhammer": "branchstaff"},
    "Wizard_Staff": {"staff": "crook", "warhammer": "crook", "mace": "crook"},
    "Monk_Staff": {"staff": "staff", "warhammer": "staff", "mace": "staff", "dagger": "staff"},
    "Enchanter_Staff": {"staff": "crystalcrook", "warhammer": "crystalcrook"},
    "Magician_Staff": {"staff": "crystalcrook", "warhammer": "crystalcrook", "mace": "crystalcrook"},
    "Necromancer_Totem": {"staff": "bonestaff", "warhammer": "bonestaff", "mace": "bonestaff", "wand": "totem",
                          "charm": "totem"},
    "Enchanter_Rod": {"wand": "crystalwand", "mace": "crystalwand"},
    "Magician_Rod": {"wand": "orbwand", "mace": "orbwand", "charm": "orbwand"},
    "Wizard_Rod": {"wand": "moonwand", "mace": "moonwand", "sword": "moonwand", "charm": "moonwand"},
    "Monk_Claws": {"fist": "claws", "knuckles": "claws", "sword": "claws", "mace": "claws"},
    "Monk_Knuckles": {"fist": "knuckles", "sword": "knuckles"},
    "ShadowKnight_Polearm": {"dagger": "spear", "partizan": "partizan"},
    "Shaman_Spear": {"dagger": "spear", "partizan": "partizan"},
    "Paladin_Lance": {"partizan": "lance"},
    "Necromancer_Sickle": {"sword": "sickle"},
}
# an NPC's hand: the epic's closest regular model of its own kind
NPC_LOOK = {
    "Bard_MainGauche": "stiletto", "Bard_Rapier": "rapier", "Bard_Sabre": "katana", "Cleric_Greathammer": "warhammer",
    "Cleric_Hammer": "hammer", "Druid_Scimitar": "scimitar", "Druid_Staff": "branchstaff", "Enchanter_Rod": "crystalwand",
    "Enchanter_Staff": "crystalcrook", "Magician_Rod": "orbwand", "Magician_Staff": "crystalcrook", "Monk_Claws": "claws",
    "Monk_Knuckles": "knuckles", "Monk_Staff": "staff", "Necromancer_Sickle": "sickle", "Necromancer_Totem": "bonestaff",
    "Paladin_Greatsword": "claymore", "Paladin_Lance": "lance", "Paladin_Warhammer": "warhammer", "Ranger_Axe": "hatchet",
    "Ranger_Longsword": "longsword", "Ranger_Scimitar": "scimitar", "Rogue_Dagger": "sword", "Rogue_Dirk": "kris",
    "Rogue_Stiletto": "stiletto", "ShadowKnight_Greataxe": "greataxe", "ShadowKnight_Greatsword": "wavy",
    "ShadowKnight_Polearm": "partizan", "Shaman_Club": "club", "Shaman_Spear": "partizan", "Warrior_Greataxe": "greataxe",
    "Warrior_Greatsword": "broadsword", "Warrior_Longsword": "scimitar", "Wizard_Rod": "moonwand", "Wizard_Staff": "crook",
}
KEEP_NAMES = {"twinkle", "icingdeath"}   # Drizzt's weapons, their look the owner's (2026-11-16)
KEEP_NPC = set()

# epics by name: both shards' epic quest rewards, Ascended copies and heirlooms' epics (2026-11-18), the 49s of
# claude-notes/epic-weapons-looks-20261106.md, and EQOA-Data's Information/epics_49.json + epics_60.json item lists
EPIC_NAMES = """Ascendant staff|Ascendent Staff|Axe of Ancient Melodies|Axe of Bloodletting|Axe of Nature's Innocence|
Axe of Nature's Purity|Ballad of Mithril|Bedlam|Beheader of Isthiak|Blacktongue|Blade of Ancient Melodies|
Blade of Ancient Song|Blade of Lyrical Harmony|Blade of Lyrical Refrain|Blade of Nature's Innocence|
Blade of Nature's Purity|Bow of Sharp Shooting|Braclet of orbital energy|Chestguard of Chain Reaction|
Chestguard of Combustion|Chestguard of Diabolism|Chestguard of Dynamism|Chestguard of Frostwork|
Chestplate of Arcanist Warding|Chestplate of Burning Ashes|Chestplate of Clouds|Chestplate of Tumultuous Storms|
Chestplate of the Icy Whirlwind|Claws of Insurrection|Claws of Mayhem|Conductive Tunic|Crystalmana Crosier|
Cystalmana Crosier|Cudgel of the Spirit Guide|Dagger of Bloodletting|Dagger of Vendetta's Resolved|
Dagger of Vendettas Resolved|Downfall|Elite Staff of the Summoner|Enkindled Tunic|Focus of Anarchic Control|
Focus of Ancient Reanimation|Focus of Chaotic Might|Focus of Mystifying Power|Focus of Runic Currents|
Focus of the Creator|Foereaver|Glory of Elddar|Hammer of Absolution|Hammer of Ethereal Conquest|
Hammer of Mystical Enlightenment|Hammer of mystical enlightnment|Hammer of the Cosmos|Heartsbane|Heroic Ykesha|
Judgement|Mace of Repentence|Metamorphic Robes|Miraculous Chestguard|Mithril Eagle Rod|Oak Staff of Eilerawdl|
Oaken Staff if Eilerawdl|Odan's Bonded Gloves|Pulsaric Rod|Pure Emerald Longsword|Quicksilver longbow|Rod of Ataxia|
Rod of Consuming Hatred|Rod of Elemental Union|Rogue's Epic Dagger|Scimitar of Fury|Scimitar of the Cosmos|
Scintillating Chestplate|Selo's Jeweled Rapier|Shadowknight's Epic Greataxe|Shadowknight's Epic Lance|
Sickle of Avhi Escron|Sickle of Escron|Sickle of the Cosmos|Spear of Fate|Staff of Coercing Energies|
Staff of Divinity|Staff of Dynamic Summoning|Staff of Nature's Blessing|Staff of Nebula|Staff of Spiritual Energies|
Staff of Superior Influence|Staff of the Banshee Wail|Staff of the Enlightened|Steelwind|Stiletto of Bloodshed|
Stiletto of Furious Revenge|Sunsilver Blade|Sword of the Challenger|Sword of the Contender|Sword of Virtue|
The Mountain Path|Totem of 1000 Beckons|Totem of Ancient Reanimation|Triad Scimitar|Wand of Runic Currents|
Warrior's Epic Blade|Warrior's Epic Greatsword"""
RE_EPIC_QUEST = re.compile(r"^EQOA-Data (60/[^/ ]+/reward-[a-z]+|30,40,49/[^/ ]+/L49-[a-z]{3,}) ")
HAND_SLOTS = (12, 14, 15, 18, 20, -1, 0)


def norm(name):
    n = re.sub(r"[`'’]", "", (name or "").strip().lower())
    return n[9:] if n.startswith("ascended ") else n


EPIC_NAME_SET = {norm(n) for n in EPIC_NAMES.replace("\n", "").split("|")}


def signed(v):
    v &= 0xFFFFFFFF
    return v - (1 << 32) if v >= 1 << 31 else v


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


# ---- who is an epic, who an heirloom ----------------------------------------------------------------------------------

def epics_and_heirlooms(db):
    """({patternid: why epic}, {patternid: why heirloom})."""
    names = dict(db.execute("SELECT patternid, itemname FROM itempattern"))
    why = {}
    if table(db, "quests") and table(db, "queststeps"):
        for qid, src in db.execute("SELECT questid, source FROM quests").fetchall():
            if not RE_EPIC_QUEST.match(src or ""):
                continue
            for ri, ch in db.execute("SELECT rewarditems, choices FROM queststeps WHERE questid = ?", (qid,)).fetchall():
                ids = re.findall(r"\d+", ri or "")
                if ch:
                    try:
                        for o in json.loads(ch).get("options") or []:
                            ids += re.findall(r"\d+", str(o.get("items") or ""))
                    except (ValueError, AttributeError):
                        pass
                for i in ids:
                    why.setdefault(int(i), f"epic quest {qid}")
    if table(db, "itemascended"):
        for p, b in db.execute("SELECT patternid, base FROM itemascended").fetchall():
            if b in why:
                why.setdefault(p, f"Ascended copy of epic {b}")
            if p in why:
                why.setdefault(b, f"base of Ascended epic {p}")
    heir = {}
    if table(db, "heirlooms"):
        for p, cls, role, epic in db.execute("SELECT patternid, class, role, epic FROM heirlooms").fetchall():
            heir[p] = f"heirlooms (class {cls}, {role})"
            if epic:
                why.setdefault(epic, f"heirloom {p}'s epic")
    found = {norm(names[p]) for p in why if p in names} | EPIC_NAME_SET
    for p, n in names.items():
        k = norm(n)
        if k in found:
            why.setdefault(p, "an epic's name")
        elif any(k.startswith(e + " (") for e in found):
            why.setdefault(p, "an epic's name, a variant")
        elif re.search(r"\bepic\b", k):
            why.setdefault(p, "'Epic' in its name")
        if (n or "").strip().lower().startswith("heirloom "):
            heir.setdefault(p, "named Heirloom")
    return why, heir


# ---- the new look -------------------------------------------------------------------------------------------------------

def kind_of(slot, attack):
    if slot not in (12, 14, 15):
        if attack in (1, 2, 3, 4, 5, 6, 8) and slot not in (18, 20):
            pass  # a weapon kept outside a hand slot (a wear 6 sword): judged as a one-hander
        else:
            return (0, "")
    hands = 2 if slot == 15 else 1
    dmg = {1: "S", 2: "S", 3: "B", 4: "B", 5: "P", 6: "P", 8: "H"}.get(attack, "")
    return (hands, dmg)


def family_of(name, hands):
    n = (name or "").lower()
    for rx, fams in FAMILY_RULES:
        if re.search(rx, n):
            f = fams.get(hands)
            if f:
                return f
    return None


def family_fits(fam, kind):
    """Whether a family drawn from a name suits the record's attack kind (a 'Steelwind Staff' of blunt is a staff; a
    'Sword of Power' of pierce stays a sword: a name decides the shape, the kind only rules out a wrong one)."""
    hands, dmg = kind
    if hands == 0:
        return True
    blunt = {"staff", "wand", "mace", "hammer", "warhammer", "club", "greatclub", "mace2h", "knuckles", "fist", "claws"}
    if dmg == "B":
        return fam in blunt or fam in ("crook",)
    return fam not in ("wand", "mace", "hammer", "club", "greatclub", "mace2h") or fam == "staff"


def pick(db, rec, epic, twins):
    """(new model, how) for one item record."""
    pid, name, slot, attack = rec["patternid"], rec["itemname"], rec["equipslot"], rec["attacktype"]
    kind = kind_of(slot, attack)
    t = twins.get((norm(name), kind))
    if t:
        return t, "a record of the same name wears it"
    own = EPIC_OWN.get(epic, {})
    fam = family_of(name, kind[0])
    if fam is None and kind[0] == 0:
        fam = family_of(name, 1)  # a sword kept outside the hands (Nature's Sword of Faith, wear 6)
        if fam in ("staff",):
            fam = "wand"
    if fam and family_fits(fam, kind):
        f = own.get(fam, fam)
        return M[f], f"its name: {fam}" + (f" (the epic's own: {f})" if f != fam else "")
    f = KIND_DEFAULT[kind]
    f2 = own.get(f, f)
    return M[f2], f"its kind {kind[0] or 'held'}{kind[1]}: {f2}"


def twin_models(db):
    """((normalized name, kind) -> a regular hand model a record of that name and kind wears, the commonest;
    {(normalized name, model): its icon}; {model: the commonest icon among the regular records wearing it})."""
    out, icons, count, by_model = {}, {}, {}, {}
    for name, slot, attack, model, icon in db.execute(
            "SELECT itemname, equipslot, attacktype, model, itemicon FROM itempattern "
            "WHERE equipslot IN (12, 14, 15, 18) ORDER BY patternid"):
        m = (model or 0) & 0xFFFFFFFF
        if not m or m in NOT_REGULAR or signed(icon) in EPIC_ICONS:
            continue
        key = (norm(name), kind_of(slot, attack), m)
        count[key] = count.get(key, 0) + 1
        icons.setdefault((norm(name), m), signed(icon))
        c = by_model.setdefault(m, {})
        c[signed(icon)] = c.get(signed(icon), 0) + 1
    for (n, k, m), c in sorted(count.items(), key=lambda kv: -kv[1]):
        out.setdefault((n, k), m)
    model_icon = {m: max(c.items(), key=lambda kv: (kv[1], -kv[0]))[0] for m, c in by_model.items()}
    return out, icons, model_icon


def plan(db):
    """(item writes, npc writes, rows for the report, kept, refusals)."""
    refusals = [f"no {t} table here" for t in ("itempattern", "npcs", "gm_audit") if not table(db, t)]
    if refusals:
        return [], [], [], [], refusals
    why, heir = epics_and_heirlooms(db)
    twins, twin_icons, model_icon = twin_models(db)
    items, report, kept = [], [], []
    cur = db.execute("SELECT patternid, itemname, equipslot, attacktype, levelreq, model, itemicon FROM itempattern")
    cols = [d[0] for d in cur.description]
    for r in cur.fetchall():
        rec = dict(zip(cols, r))
        m = (rec["model"] or 0) & 0xFFFFFFFF
        if m not in EPIC_MODELS:
            continue
        epic = EPIC_MODELS[m][0]
        pid = rec["patternid"]
        if pid in why or pid in heir:
            kept.append((pid, rec["itemname"], epic, why.get(pid) or heir.get(pid)))
            continue
        if norm(rec["itemname"]) in KEEP_NAMES:
            kept.append((pid, rec["itemname"], epic, "kept: the owner's own look for it (KEEP_NAMES)"))
            continue
        new, how = pick(db, rec, epic, twins)
        if new in NOT_REGULAR:
            refusals.append(f"itempattern {pid} {rec['itemname']!r}: picked an epic model {new:08X}")
            continue
        icon = rec["itemicon"]
        want = {"model": signed(new)}
        newicon = signed(icon)
        if signed(icon) in EPIC_ICONS:
            newicon = twin_icons.get((norm(rec["itemname"]), new)) if how.startswith("a record of the same name") else None
            if newicon is None:
                newicon = MODEL_ICON.get(new, model_icon.get(new))
            if newicon is None:
                refusals.append(f"itempattern {pid} {rec['itemname']!r}: no regular icon known for {new:08X}")
                continue
            want["itemicon"] = newicon
        items.append(("itempattern", {"patternid": pid}, want))
        report.append(("item", pid, rec["itemname"], rec["equipslot"], rec["attacktype"], rec["levelreq"],
                       f"{m:08X}", epic, f"{new:08X}", how, signed(icon), newicon))
    npcs = []
    for nid, nname, pri, sec, shd in db.execute(
            "SELECT id, npc_name, primary_weapon, secondary_weapon, shield FROM npcs").fetchall():
        want = {}
        for col, v in (("primary_weapon", pri), ("secondary_weapon", sec), ("shield", shd)):
            m = (v or 0) & 0xFFFFFFFF
            if m in EPIC_MODELS and nid not in KEEP_NPC:
                new = M[NPC_LOOK[EPIC_MODELS[m][0]]]
                want[col] = signed(new)
                report.append(("npc", nid, nname, col, "", "", f"{m:08X}", EPIC_MODELS[m][0], f"{new:08X}",
                               f"the epic's closest regular look: {NPC_LOOK[EPIC_MODELS[m][0]]}", "", ""))
        if want:
            npcs.append(("npcs", {"id": nid}, want))
    return items, npcs, report, kept, refusals


# ---- writing ------------------------------------------------------------------------------------------------------------

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT NOT NULL,      -- the columns changed, as they were (JSON)
    after  TEXT NOT NULL,      -- the columns changed, as written (JSON)
    audit  INTEGER NOT NULL
)"""


def audit(db, tbl, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def put(db, tbl, key, want):
    """The named columns of tbl's row of key set; (the columns before, after, audit id)."""
    full_before = row(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in want)} WHERE {where}",
               tuple(want.values()) + tuple(key.values()))
    full_after = row(db, tbl, key)
    n = audit(db, tbl, key, full_before, full_after)
    return {k: full_before[k] for k in want}, {k: full_after[k] for k in want}, n


def apply(db, writes):
    db.execute(LOG_DDL)
    for tbl, key, want in writes:
        before, after, n = put(db, tbl, key, want)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, go_json(key), go_json(before), go_json(after), n))
    return len(writes)


def undo(db):
    back, left = 0, []
    if not table(db, LOG):
        return back, left
    for seq, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, b, a = json.loads(rowkey), json.loads(before), json.loads(after)
        now = row(db, tbl, key)
        if now is None or {k: now[k] for k in a} != a:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        put(db, tbl, key, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def write_report(path, report, kept):
    with open(path, "w") as f:
        f.write("what\tid\tname\tslot_or_column\tattacktype\tlevel\told_model\tepic\tnew_model\thow\told_icon\tnew_icon\n")
        for r in report:
            f.write("\t".join(str(x) for x in r) + "\n")
        for pid, name, epic, why in kept:
            f.write(f"kept\t{pid}\t{name}\t\t\t\t\t{epic}\t\t{why}\t\t\n")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--report", help="write the item -> old -> new table here (TSV)")
    ap.add_argument("--verbose", action="store_true", help="list every row")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + "".join(f"\n  left: {w}" for w in left))
            return 0 if not left else 1
        items, npcs, report, kept, refusals = plan(db)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:" + "".join(f"\n  {r}" for r in refusals))
            return 1
        if a.report:
            write_report(a.report, report, kept)
        icons = sum(1 for _, _, w in items if "itemicon" in w)
        print(f"epic-model records kept (epic, heirloom or the owner's): {len(kept)}; items to change: {len(items)} "
              f"({icons} with the icon); NPC rows: {len(npcs)}")
        if a.verbose:
            for r in report:
                print("  " + " | ".join(str(x) for x in r))
        writes = items + npcs
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before and changed since ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(writes)} rows; --apply to write)")
            return 0
        n = apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; items now (held copies at the next login or re-equip), "
              "NPCs at the panel's Reload every NPC")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
