#!/usr/bin/env python3
"""The Harbingers of Khalimet raid line on Zaoran Plateau and Tunaria (work board 682, 684, 699, the GM's approved
research; 768 D for the Khal robes) and the records Hogarth Bartholomew's gem setting needs (Heirloom petition 2614:
a player finished quest "Gnadriin", whose last step says "Khalimet's gems fit the warlords' weapons. For 10,000
tunar I will set one, for 2000 take it out", and nothing answered; the setting is server code,
internal/handlers/gemsetter.go, table gemsetting). Run on each shard's database; every row is found or made by NAME
and place (the shards' ids differ). [ours] in a note marks a figure, a spot or a count no source gives.

WHAT IT WRITES (one set, "harbingers", in tools/seed_guide_gaps.py's log guide_set_log, so --undo is exact):

ITEMS (682, 684, 699; the films of 682's notes)
  - the six Gems of Khalimet (Black, Blue, Green, Red, Yellow, White), Lore, level 55, their augment figures (t288)
    in their words (a gem is no worn item: its figures are what it adds to a Khal weapon);
  - the plain Khal weapons the server lacks: Anarchy (rogue), Rapture (monk), Bloodstaff (one-hand blunt: the
    film's), Mourning Star (cleric, druid, shaman), Warfire Ring (alchemist, a neck piece: "25sta 50wis 25ar") and
    Bloodplague (one-hand slashing: the film's; bard, ranger), each Obliterator 1054's line (700 damage, 20,000 HP,
    80 durability, STA 25, level 55, Lore) and the look of the server's copy of its coloured kin [ours where none];
  - every weapon in each of the six colours, the plain weapon and the gem's figures (its STA 25 the weapon's own),
    the White one's damage 900 (films: Green and Yellow Bloodplague at 700, White Bloodplague at 900): 42 records,
    of which Green, Red and Yellow Obliterator (1021-1023) are kept (their STR 75, the import's, becomes the
    gem's PoT 75 and HoT 75: the films' Yellow Bloodplague carries HoT 75) and White Obliterator (960145 where a
    shard has it) is kept as it is;
  - the Khal robes as 768 D and its films: Robe of Shadows and Robe of Magic made as base records (25,000 HP, 80
    durability; Magic with no AC), Robe of Faith (WIS 40, CHA 40, AC 50, POW 150, NO TRADE); Robe of War 1027 kept;
  - Khal Basher and Khal Battleblade (684, the GM's "Make them": plain 700-damage Khal weapons, the kinds by name);
  - Khalimet's Call, the port stone (684's film: one charge, handed round), NO RENT, a stack of 16; its Use casts
    the gate "Khalimet's Call" (itemspell, spellgates) to the General's isle.
  - table gemsetting: every weapon record of the family (made, kept, and the shard's transfer copies of the same
    names as canonical 0) and the six gems, for Hogarth.

PART II (682, 684; the GM's rulings: "Full quest", then "disable the quest until we choose to enable it")
  - quest "Harbingers of Khalimet: The Viceroy" (tools/quests/data/harbingers.json) written OFF (enabled 0);
  - triggers (slots 300-306, 309): "Khal Hounds" at the portal in Wktaan's 4th Talon (NE) [spot ours]: three hounds at a
    time (each back 10 s after its death: "Khal Hound", npcdeath), the pack leader after ten kills, the post at his
    death ("With a flicker of energy, a post appears to have been activated.", film), an hour's rest; "Khalimet's
    General" (the post's give step): the General untouchable on the free isle of Zaoran Plateau at (4455, 183, 6495)
    for two hours (the SE isle is Razeal's: [ours]); "Khal Viceroy" (his hail, anyone's): waves every 60 s from 12 s
    after it (two Khal Assault Infantry, an Elite Infantry and a Cavalry, a Berserker every third wave [ours]), the
    Khal Assassin standing by him (attacked, not attacking); the Assassin's death: a Khal Dark Templar at 5%
    ("Khal Dark Templar"), a Khal War Templar at 95% ("Khal War Templar", rolled apart: both at 4.75%), who leaves
    unfought after 3 minutes, and another Assassin two minutes on ("Khal Assassin"); the Dark Templar's death ("Khal Viceroy
    Wakes"): the waves gone, the General hidden, the Viceroy hostile for a 15-minute kill; his death: three minutes'
    loot, then the raid to Freeport's docks; a run without a kill ends 15 minutes after the hail and the General
    stands again for a new hail (684 step 13). The wave's Infantry and Templar are named apart from the white camp
    by Freeport's north gate (944: "Khal Infantry", "Khal Templar"), whose templates would otherwise be theirs.
  - the wave mobs, the hounds, the pack leader, the post, the General: spawn-only rows, each a template of 684's
    level and hp_pct; melee from each figure's top hit ("2,500 on a ~7,000 AC warrior") by MELEE_PER below; the
    Berserker's 5,260 crit once (its second hit is a code ask); the Viceroy's root on whom he fights every 20 s, CR 50.
  - loot: Viceroy "khal gems" (one), "khal viceroy" (two: the seven plain weapons and the three coloured Obliterators
    the table held, bound at last), "zp spells" (one), "khal viceroy sword" (Sapphyra's Sword, 5%); every wave
    soldier "khal waves" (Achroite 10%) and "khal wave weapons" (Basher or Battleblade, 5%).

PART III (699)
  - six camps of Khal Conscripts (4) and Khal Garrisons (2) round the Khal Zealots in Permafrost (S), the Khal
    Captain in one [spots ours], 10 minutes' respawn; each camp death 3% to raise the Khal Warlord where it fell
    ("Khal Warlord", one at a time, an hour at most); his loot one robe of four, and a Khal weapon at 50%;
  - the Khal Zealots to level 62, the Khal Reaver to 63, with their templates.

PART V (699: 101280 "Gnadriin", no character on it on either shard when written)
  - The Keeper and Anibael become talkers at level 63 and their steps (2, 5) raise a hostile copy each; step 8
    raises four weary adventurers at 63, and each one killed a Gnadriin where it fell ("Gnadriin", npcdeath),
    Gnadriin's standing row spawn-only; Tobral Arnez and Arliss to 63, Arliss in a goblin's body.

NOT BUILT (code asks, or held): Part IV (a quest of destroyed portals and a roost: code ask, open questions), the
Chiktar King spawn-only (with Part IV), the Berserker's/Warlord's second crit, the Conscripts' boot daggers, the
Viceroy's de-form, the Zealots' nuke, the pouch of stones and the post's riddle, the gems' order, corpse coin,
Ascended copies (tools/ascend_boss_loot.py after this, as 580/623 do).

WHEN IT TAKES EFFECT: items at once; NPCs, templates, triggers, deaths and quests at a start (apply in the keep
restart's before.sh, or with the server stopped); gemsetting at once, with the build that has gemsetter.go.

    python3 -E -s tools/harbingers_20261122.py --db eqoa.db [--apply | --undo | --dump]
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_guide_gaps as sgg  # noqa: E402

SET = "harbingers"
WHO, VIA = "dab", "tool"
DETAIL = "Harbingers of Khalimet II, III and V (board 682, 684, 699) and Hogarth's gem setting (petition 2614): tools/harbingers_20261122.py"
SRC = "harbingers 20261122"
# the tables this set changes beyond the guide sets' own
EXTRA_TABLES = ("gemsetting", "spellgates", "itemspell", "norentvanish")

# --- the figures -----------------------------------------------------------------------------------------------

CLASS = {"WAR": 1, "RAN": 2, "PAL": 4, "SK": 8, "MNK": 16, "BRD": 32, "ROG": 64, "DRD": 128, "SHA": 256, "CL": 512,
         "MAG": 1024, "NEC": 2048, "ENC": 4096, "WIZ": 8192, "ALC": 16384}


def classes(*names):
    return sum(CLASS[n] for n in names)


OBLITERATOR = 1054  # the plain Khal weapon the server has: every made weapon's line
GEM_LIKE = 701411   # Khal Red Gem: a gem's record
ROBE_LIKE = 1027    # Khal Robe of War

# the gems (t288): what each adds to a Khal weapon, the STA 25 of every one the weapon's own already
GEMS = {
    "Black": {"agi": 50, "AC": 200, "DR": 25},
    "Blue": {"intelligence": 50, "POWMAX": 300, "CR": 25},
    "Green": {"dex": 50, "HPMAX": 300, "PR": 25},
    "Red": {"cha": 50, "PoT": 75, "FR": 25},
    "Yellow": {"wis": 50, "HoT": 75, "LR": 25},
    "White": {"AC": 300, "POWMAX": 250, "HPMAX": 250},
}
GEM_WORDS = {"Black": "AGI 50, AC 200, DR 25", "Blue": "INT 50, power 300, CR 25", "Green": "DEX 50, HP 300, PR 25",
             "Red": "CHA 50, PoT 75, FR 25", "Yellow": "WIS 50, HoT 75, LR 25", "White": "AC 300, power 250, HP 250, and its damage to 900"}
WHITE_DAMAGE = 900

# the plain weapons: slot, attack type, classes, look (model, icon) [the server's copies' where it has them]
OBL_LOOK = (-1903315937, -955781738)
WEAPONS = {
    "Obliterator": None,  # 1054, kept
    "Anarchy": dict(equipslot=12, attacktype=1, classuse=classes("ROG"), look=OBL_LOOK),
    "Rapture": dict(equipslot=14, attacktype=5, classuse=classes("MNK"), look=OBL_LOOK),
    "Bloodstaff": dict(equipslot=12, attacktype=3, classuse=classes("MAG", "NEC", "ENC", "WIZ"), look=(1660958085, 25125364)),
    "Mourning Star": dict(equipslot=12, attacktype=3, classuse=classes("CL", "DRD", "SHA"), look=(724832212, -2055321559)),
    "Warfire Ring": dict(equipslot=4, attacktype=0, classuse=classes("ALC"), look=(0, 756343796), weapondamage=0,
                         maxhp=12000, duration=65, extra={"wis": 50, "AR": 25}),
    "Bloodplague": dict(equipslot=12, attacktype=1, classuse=classes("BRD", "RAN"), look=OBL_LOOK),
}
WEAPON_SLOTS = (4, 12, 14)
# the coloured Obliterators the server has (their figures fixed below)
KEPT_COLOURED = {"Green Obliterator": 1021, "Red Obliterator": 1022, "Yellow Obliterator": 1023}
WHITE_OBLITERATOR = 960145
WHITE_OBL_LOOK = (1857495629, -1604900612)

ROBES = {  # 768 D and its films; the colour columns the server's copies carry
    "Khal Robe of Shadows": dict(classuse=114, stats={"str": 40, "dex": 40, "HPMAX": 150, "AC": 50}, color=269488383),
    "Khal Robe of Magic": dict(classuse=31744, stats={"sta": 40, "intelligence": 40, "HPMAX": 150, "PoT": 20}, color=3506438399),
    "Khal Robe of Faith": dict(classuse=896, stats={"wis": 40, "cha": 40, "AC": 50, "POWMAX": 150}, color=3770679551),
}
WAVE_WEAPONS = {"Khal Basher": dict(attacktype=3, look=(724832212, -2055321559)), "Khal Battleblade": dict(attacktype=1, look=OBL_LOOK)}

CALL = "Khalimet's Call"
CALLS = 16
GATE_SPELL_FROM = "Nerius Gate"
GATE_NAME = "Khalimet's Call"
GATE_LINE = "Self, Teleports the caster to the General's isle in Zaoran Plateau"

# --- the places ------------------------------------------------------------------------------------------------

PORTAL = (0, 17880.0, 63.0, 20120.0)      # Wktaan's 4th Talon (NE) [ours]
ISLE = (5, 4455.0, 183.0, 6495.0)          # the free isle of Zaoran Plateau [ours]
GENERAL_AT = (5, 4455.0, 183.0, 6545.0)    # its south end
PAD_AT = (5, 4455.0, 183.0, 6450.0)        # its north end, where Khalimet's Call lands
WAVES_AT = (5, 4530.0, 183.0, 6490.0)      # "from the right of port site and left of Khalimet"
DOCKS = (0, 25390.0, 54.1, 15780.0)        # Freeport's docks, by the dockmasters
ZEALOTS = (4980.0, 5145.0)                 # the Khal Zealots' ground, Permafrost (S)
CAMPS = [(5200, 75.2, 5145), (5090, 53.1, 5336), (4870, 54.1, 5336), (4760, 66.7, 5145), (4870, 60.3, 4954), (5090, 57.2, 4954)]

# --- melee: a template's melee_pct from a top hit on a ~7,000 AC level 60 warrior [ours] ------------------------
# Vallon Zek's template (tools/guidegaps/vallon.json: melee 2558 at 68, 3,520 on a level 60 warrior) is the measure:
# a boosted creature's hit there is its floor ((level + 1) / 2, entity/defence.go) times the con's damage_pct
# (balancecon, its level less 60) times melee_pct, times MELEE_PER.
MELEE_PER = 3520 / (34 * 2.30 * 25.58)


def con_damage(db, diff):
    r = db.execute("SELECT damage_pct FROM balancecon WHERE diff = ?", (diff,)).fetchone() if sgg.has(db, "balancecon") else None
    return (r[0] if r else 100.0) / 100


def melee_for(db, level, top):
    return round(top / (MELEE_PER * ((level + 1) // 2) * con_damage(db, level - 60)) * 100)


# --- the creatures ---------------------------------------------------------------------------------------------
# name: (level, hp_pct, top hit, extras); hp_pct is 684's / 699's (entry 676's measure)
TEMPLATES = {
    "Khal Hound": (63, 316, 1250, {}),
    "Khal Pack Leader": (63, 633, 1250, {}),
    "Khal Assault Infantry": (62, 1305, 2500, {}),
    "Khal Elite Infantry": (64, 1843, 2500, {}),
    "Khal Cavalry": (63, 759, 2500, {}),
    "Khal Assassin": (63, 633, 2500, {}),
    "Khal War Templar": (63, 633, 2500, {}),
    "Khal Dark Templar": (65, 1610, 2500, {}),
    "Khal Berserker": (65, 751, 2500, {"crit": 5260}),
    "Khal Viceroy": (63, 7595, 3527, {"resist_kind": "CR", "resist_pct": 50,
                                      "abilities": [{"kind": "root", "spell_id": 460, "every": 20, "first": 5}]}),
    "Khal Warlord": (65, 1073, 2500, {"crit": 5260}),
    "Khal Conscript": (62, 261, 1250, {}),
    "Khal Garrison": (62, 783, 1500, {}),
    "Khal Captain": (62, 783, 2500, {}),
    "Khal Zealot": (62, 783, 1250, {}),
    "Khal Reaver": (63, 759, 1250, {}),
    "The Keeper": (63, 506, 1250, {}),
    "Anibael": (63, 506, 1250, {}),
    "weary adventurer": (63, 253, 1000, {}),
    "Gnadriin": (63, 633, 1500, {}),
    "Tobral Arnez": (63, 506, 1250, {}),
    "Arliss": (63, 506, 1250, {}),
}
TEMPLATE_NOTES = {
    "Khal Viceroy": "682: L63 (red to a 60), ~1.2M effective (hp_pct 7595), hits of 2,000-3,527 on a 7,066 AC tank (2011 film), roots whoever holds aggro (forum t165; every 20 s ours), CR 50 (\"mid to high CR\"); LR 0.",
    "Khal Berserker": "684: L65 by four kill lines, ~140k, \"about 5260 X 2\" crit (forum): one 5,260 here, its second a code ask.",
    "Khal Warlord": "699: \"similar to the Bezerker\" with \"even more hitpoints\": L65, ~200k, the Berserker's crit [ours].",
}

# the spawn-only rows the triggers copy: name, like (a row to copy) or body, place, size
SPAWN_ROWS = [
    ("Khal Hound", "chaos hound", PORTAL, 1.5, None),
    ("Khal Pack Leader", "chaos hound", PORTAL, 2.0, None),
    ("Khal signpost", "Signpost of Zek", PORTAL, 0.5, 128),
    ("Khal Viceroy", "Khal Zealot", GENERAL_AT, 1.4, None),
    ("Khal Assault Infantry", "Khal Zealot", WAVES_AT, 1.0, None),
    ("Khal Elite Infantry", "Khal Zealot", WAVES_AT, 1.1, None),
    ("Khal Cavalry", "undead stallion", WAVES_AT, 1.6, None),
    ("Khal Assassin", "Assassin of Khal", GENERAL_AT, 1.0, None),
    ("Khal War Templar", "Khal Zealot", WAVES_AT, 1.1, None),
    ("Khal Dark Templar", "Khal Zealot", WAVES_AT, 1.2, None),
    ("Khal Berserker", "Assassin of Khal", WAVES_AT, 1.4, None),
    ("Khal Warlord", "Assassin of Khal", (0, CAMPS[0][0], CAMPS[0][1], CAMPS[0][2]), 1.5, None),
]
WAVE_SOLDIERS = ["Khal Assault Infantry", "Khal Elite Infantry", "Khal Cavalry", "Khal Assassin", "Khal War Templar",
                 "Khal Dark Templar", "Khal Berserker"]
CAMP_MOBS = ("Khal Conscript", "Khal Garrison", "Khal Captain")

SLOTS = {"Khal Hounds": 300, "Khal Hound": 301, "Khalimet's General": 302, "Khal Viceroy": 303,
         "Khal Dark Templar": 304, "Khal War Templar": 305, "Khal Viceroy Wakes": 306, "Khal Warlord": 307, "Gnadriin": 308,
         "Khal Assassin": 309}

PART2_KEY = "Misc/harbingers-of-khalimet/p2"
GNADRIIN_KEY = "Misc/harbingers-of-khalimet/p5"


def at(p, **kw):
    d = {"world": p[0], "x": p[1], "y": p[2], "z": p[3]}
    d.update(kw)
    return d


def triggers():
    """The triggers, as rows of gmtriggers."""
    w = WAVES_AT
    waves = []
    for i in range(14):  # 12 s, then every 60 s, until the 15 minutes run out
        waves.append({"do": "wait", "seconds": 12 if i == 0 else 60})
        waves.append(dict(at(w), do="spawn", npc="Khal Assault Infantry", hostile=True, count=2, spread=15, minutes=20))
        waves.append(dict(at(w), do="spawn", npc="Khal Elite Infantry", hostile=True, count=1, spread=15, minutes=20))
        waves.append(dict(at(w), do="spawn", npc="Khal Cavalry", hostile=True, count=1, spread=15, minutes=20))
        if i % 3 == 2:
            waves.append(dict(at(w), do="spawn", npc="Khal Berserker", hostile=True, count=1, spread=15, minutes=20))
    p, i_ = PORTAL, ISLE
    return [
        dict(slot=SLOTS["Khal Hounds"], name="Khal Hounds", scope="near", radius=150, world=p[0], x=p[1], y=p[2], z=p[3], steps=[
            {"do": "lock", "lock": "khal hounds"},
            {"do": "clock", "seconds": 5400},
            {"do": "broadcast", "text": "The portal in Wktaan's 4th Talon stirs. Khal hounds pour out of it!"},
            dict(at(p), do="spawn", npc="Khal Hound", hostile=True, count=3, spread=15, minutes=90),
            {"do": "awaitkill", "npc": "Khal Hound", "count": 10},
            {"do": "stop", "slot": SLOTS["Khal Hound"]},
            {"do": "despawn", "npc": "Khal Hound"},
            dict(at(p), do="spawn", npc="Khal Pack Leader", hostile=True, count=1, spread=5, minutes=60),
            {"do": "awaitdead", "npc": "Khal Pack Leader"},
            {"do": "broadcast", "text": "With a flicker of energy, a post appears to have been activated."},
            dict(at(p), do="spawn", npc="Khal signpost", untouchable=True, count=1, spread=1, minutes=60),
            {"do": "wait", "seconds": 3600},
            {"do": "end"},
            {"do": "stop", "slot": SLOTS["Khal Hound"]},
            {"do": "despawn"},
        ], notes="684 steps 1-2: a questor with the four gems at the portal raises Khal hounds; kill hounds until a pack leader "
                 "spawns (after the tenth [ours]); \"A post has appeared upon killing the pack leader\"; \"reset timer of about an hour or two\". "
                 "Fired by Part II's reach step (the quest is off)."),
        dict(slot=SLOTS["Khal Hound"], name="Khal Hound", scope="near", radius=150, world=p[0], x=p[1], y=p[2], z=p[3], steps=[
            {"do": "wait", "seconds": 10},
            dict(at(p), do="spawn", npc="Khal Hound", hostile=True, count=1, max=3, spread=15, minutes=30),
            {"do": "awaitdead"},
        ], notes="684: hounds three at a time, each replaced 10 s after its death [ours] (npcdeath khal hound)."),
        dict(slot=SLOTS["Khalimet's General"], name="Khalimet's General", scope="near", radius=200, world=i_[0], x=i_[1], y=i_[2], z=i_[3], steps=[
            {"do": "lock", "lock": "khalimet's general"},
            dict(at(GENERAL_AT), do="spawn", npc="Khal Viceroy", untouchable=True, count=1, spread=1, minutes=120, face="north"),
            {"do": "wait", "seconds": 7200},
        ], notes="684 steps 3-6: the gems given to the post, the General stands non-KoS on his isle for two hours [ours] for the raid's hail. "
                 "The SE isle is Razeal's; this is the plateau's free isle [ours]."),
        dict(slot=SLOTS["Khal Viceroy"], name="Khal Viceroy", scope="near", radius=200, world=i_[0], x=i_[1], y=i_[2], z=i_[3], steps=[
            {"do": "lock", "lock": "khal viceroy"},
            {"do": "awaittalk", "npc": "Khal Viceroy", "start": True,
             "text": "You carry Khalimet's call to me? Then meet his legions."},
            {"do": "clock", "seconds": 900, "empty": 300},
            dict(at(GENERAL_AT, x=GENERAL_AT[1] + 20), do="spawn", npc="Khal Assassin", attackable=True, count=1, spread=2, minutes=20),
        ] + waves + [
            {"do": "wait", "seconds": 120},
            {"do": "end"},
            {"do": "stop", "slot": SLOTS["Khal War Templar"]},
            {"do": "stop", "slot": SLOTS["Khal Dark Templar"]},
            {"do": "stop", "slot": SLOTS["Khal Assassin"]},
            {"do": "despawn"},
            {"do": "broadcast", "text": "The legions of Khalimet fall back. The General stands aloof again."},
        ], notes="684 steps 6-9 and 13: the hail starts timed waves of about five, the first 12 s after it, then one a minute (film 2008-10); "
                 "the Assassin stands by the General unless pulled; a run without a kill ends 15 minutes after the hail [ours] and the General "
                 "stands again for a new hail (\"it has spawn the non kos all over again\")."),
        dict(slot=SLOTS["Khal Dark Templar"], name="Khal Dark Templar", scope="near", radius=200, world=i_[0], x=i_[1], y=i_[2], z=i_[3], steps=[
            {"do": "wait", "seconds": 30},
            dict(at(WAVES_AT), do="spawn", npc="Khal Dark Templar", hostile=True, count=1, spread=10, minutes=20),
        ], notes="684 step 9: the Assassin's death puts \"a Khal Dark Templar at 5%\" in the next wave (npcdeath khal assassin, 5), "
                 "30 s on [ours]."),
        dict(slot=SLOTS["Khal War Templar"], name="Khal War Templar", scope="near", radius=200, world=i_[0], x=i_[1], y=i_[2], z=i_[3], steps=[
            {"do": "wait", "seconds": 30},
            dict(at(WAVES_AT), do="spawn", npc="Khal War Templar", hostile=True, count=1, spread=10, minutes=20, engage=3),
        ], notes="684 step 9: the Assassin's death puts a Templar in the next wave at 95% (npcdeath khal assassin; the Dark Templar's "
                 "5% rolled apart, so at 4.75% both come [ours]), who \"despawns if left\" (3 minutes unfought [ours])."),
        dict(slot=SLOTS["Khal Assassin"], name="Khal Assassin", scope="near", radius=200, world=i_[0], x=i_[1], y=i_[2], z=i_[3], steps=[
            {"do": "wait", "seconds": 120},
            dict(at(GENERAL_AT, x=GENERAL_AT[1] + 20), do="spawn", npc="Khal Assassin", attackable=True, count=1, spread=2, minutes=20),
        ], notes="684 step 9: another Assassin by the General two minutes after one's death [ours], for the next chance at the Dark Templar."),
        dict(slot=SLOTS["Khal Viceroy Wakes"], name="Khal Viceroy Wakes", scope="near", radius=200, world=i_[0], x=i_[1], y=i_[2], z=i_[3], steps=[
            {"do": "lock", "lock": "khal viceroy wakes"},
            {"do": "stop", "slot": SLOTS["Khal Viceroy"]},
            {"do": "stop", "slot": SLOTS["Khal War Templar"]},
            {"do": "stop", "slot": SLOTS["Khal Dark Templar"]},
            {"do": "stop", "slot": SLOTS["Khal Assassin"]},
            {"do": "broadcast", "text": "Khal Dark Templar: Arrrrrrrrrgggggggghhhhhhhhhhhhh!!!"},
            {"do": "hide", "npc": "Khal Viceroy", "radius": 80, "minutes": 20},
            dict(at(GENERAL_AT), do="spawn", npc="Khal Viceroy", hostile=True, count=1, spread=1, minutes=20, face="north"),
            {"do": "clock", "seconds": 900},
            {"do": "awaitdead", "npc": "Khal Viceroy", "any": True},
            {"do": "end"},
            {"do": "broadcast", "text": "The Khal Viceroy has fallen."},
            {"do": "wait", "seconds": 180},
            dict(at(DOCKS), do="teleport"),
            {"do": "stop", "slot": SLOTS["Khalimet's General"]},
            {"do": "despawn"},
            {"do": "hide", "npc": "Khal Viceroy", "radius": 80, "minutes": 0},
        ], notes="684 steps 10-12: the Dark Templar's death makes Viceroy attackable (his face white, then red: film), a timed kill of about "
                 "15 minutes; after the kill the raid is ported to the Freeport docks. Run out, the Viceroy goes and the General stands again. "
                 "The waves are taken away at the Dark Templar's death [ours]."),
        dict(slot=SLOTS["Khal Warlord"], name="Khal Warlord", scope="near", radius=300, world=-1, x=0, y=0, z=0, steps=[
            {"do": "lock", "lock": "khal warlord"},
            {"do": "clock", "seconds": 3600},
            {"do": "spawn", "npc": "Khal Warlord", "hostile": True, "count": 1, "spread": 3, "minutes": 61},
            {"do": "awaitdead", "npc": "Khal Warlord"},
            {"do": "end"},
            {"do": "despawn"},
        ], notes="699: the Warlord among the Permafrost (S) camps, \"completely random and a % chance\": 3% a camp death [ours], one at a time, "
                 "an hour at most [ours]."),
        dict(slot=SLOTS["Gnadriin"], name="Gnadriin", scope="near", radius=100, world=-1, x=0, y=0, z=0, steps=[
            {"do": "spawn", "npc": "Gnadriin", "hostile": True, "count": 1, "spread": 2, "minutes": 15},
        ], notes="699 Part V: \"one spawns in place of each adventurer killed\" (npcdeath weary adventurer)."),
    ]


DEATHS = [
    ("Khal Hound", "Khal Hound", 100, "684: each hound replaced 10 s after its death [ours]"),
    ("Khal Assassin", "Khal Dark Templar", 5, "684: \"5% chance\" of a Dark Templar"),
    ("Khal Assassin", "Khal War Templar", 95, "684: else a Templar (rolled apart from the Dark Templar's 5%)"),
    ("Khal Assassin", "Khal Assassin", 100, "684: another Assassin two minutes on [ours]"),
    ("Khal Dark Templar", "Khal Viceroy Wakes", 100, "684: \"Killing the Dark Templar ... is what will activate the Viceroy\""),
    ("Khal Conscript", "Khal Warlord", 3, "699: 3% a camp death [ours]"),
    ("Khal Garrison", "Khal Warlord", 3, "699: 3% a camp death [ours]"),
    ("Khal Captain", "Khal Warlord", 3, "699: 3% a camp death [ours]"),
    ("weary adventurer", "Gnadriin", 100, "699: a Gnadriin in place of each adventurer killed"),
]


# --- the database's rows ---------------------------------------------------------------------------------------

def one(db, sql, *args):
    return db.execute(sql, args).fetchone()


def item_cols(db):
    return [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]


def record(db, pid):
    cols = item_cols(db)
    r = one(db, "SELECT * FROM itempattern WHERE patternid = ?", pid)
    return dict(zip(cols, r)) if r else None


def insert_item(db, row):
    row = dict(row)
    row.pop("patternid", None)
    names = list(row)
    cur = db.execute(f'INSERT INTO itempattern ({",".join(names)}) VALUES ({",".join("?" * len(names))})', [row[n] for n in names])
    return cur.lastrowid


def zero_stats(row):
    for c in sgg.STATS:
        if c != "weapondamage":
            row[c] = 0
    return row


def by_name(db, name, slots=None):
    """Every record of the exact name (case aside), in id order, of those slots when given."""
    rows = db.execute("SELECT patternid, equipslot FROM itempattern WHERE lower(trim(itemname)) = lower(?) ORDER BY patternid", (name,)).fetchall()
    return [r[0] for r in rows if slots is None or r[1] in slots]


class Refused(Exception):
    pass


def refusals(db):
    out = []
    for pid in (OBLITERATOR, GEM_LIKE, ROBE_LIKE, 701412, 701413, 1253, 701569, 960212):
        if not record(db, pid):
            out.append(f"record {pid} is missing")
    for name, pid in KEPT_COLOURED.items():
        r = record(db, pid)
        if not r or r["itemname"] != name:
            out.append(f"record {pid} is not {name!r}")
    for n in ["Khal Zealot", "Khal Reaver", "Assassin of Khal", "chaos hound", "Signpost of Zek", "undead stallion",
              "The Keeper", "Anibael", "weary adventurer", "Gnadriin", "Tobral Arnez", "Arliss", "goblin veteran"]:
        if not one(db, "SELECT 1 FROM npcs WHERE npc_name = ?", n):
            out.append(f"no NPC {n!r}")
    for t in TEMPLATES:
        k = t.lower()
        if one(db, "SELECT 1 FROM classtemplates WHERE name = ?", k):
            out.append(f"a template {k!r} exists already")
        r = one(db, "SELECT template FROM npctemplate WHERE name = ?", k)
        if r and r[0] != "roamer":  # the roamers' seed row gives way (logged: the undo puts it back)
            out.append(f"npctemplate {k!r} is {r[0]!r} already")
    for name, *_ in SPAWN_ROWS:
        if one(db, "SELECT 1 FROM npcs WHERE npc_name = ?", name):
            out.append(f"an NPC {name!r} exists already")
    for m in CAMP_MOBS:
        if one(db, "SELECT 1 FROM npcs WHERE npc_name = ?", m):
            out.append(f"an NPC {m!r} exists already")
    taken = [r[0] for r in db.execute(f"SELECT slot FROM gmtriggers WHERE slot IN ({','.join('?' * len(SLOTS))})", list(SLOTS.values()))]
    if taken:
        out.append(f"trigger slots taken: {taken}")
    for n in SLOTS:
        if one(db, "SELECT 1 FROM gmtriggers WHERE lower(name) = lower(?)", n):
            out.append(f"a trigger {n!r} exists already")
    for name in [f"{c} Gem of Khalimet" for c in GEMS] + [CALL, "Khal Basher", "Khal Battleblade"]:
        if by_name(db, name):
            out.append(f"a record {name!r} exists already")
    if one(db, "SELECT 1 FROM spellgates WHERE spell = ?", GATE_NAME.lower()):
        out.append(f"a gate {GATE_NAME!r} exists already")
    if not one(db, "SELECT 1 FROM spellpattern WHERE spellname = ?", GATE_SPELL_FROM):
        out.append(f"no spell {GATE_SPELL_FROM!r} to copy")
    q = one(db, "SELECT questid FROM quest_import_quests WHERE key = ?", GNADRIIN_KEY)
    if not q:
        out.append("quest Gnadriin is not imported")
    else:
        busy = one(db, "SELECT count(*) FROM charquests WHERE questid = ? AND step BETWEEN 1 AND 8", q[0])[0]
        if busy:
            out.append(f"{busy} characters are on steps 1-8 of Gnadriin, which this changes")
        ev = one(db, "SELECT events FROM queststeps WHERE questid = ? AND step = 8", q[0])
        if not ev or "Gnadriin" not in ev[0]:
            out.append(f"Gnadriin's step 8 is not as imported: {ev}")
    if one(db, "SELECT 1 FROM quest_import_quests WHERE key = ?", PART2_KEY):
        out.append("Part II's quest is imported already")
    if sgg.has(db, "gemsetting") and one(db, "SELECT 1 FROM gemsetting"):
        out.append("gemsetting has rows already")
    if sgg.has(db, sgg.LOG) and one(db, f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", SET):
        out.append(f"set {SET!r} is in already")
    return out


def make_items(db, say):
    """The records; their ids by name (a weapon's plain and coloured, the gems, the robes, the rest)."""
    ids = {}
    obl = record(db, OBLITERATOR)
    gem_like = record(db, GEM_LIKE)
    # the gems
    for c in GEMS:
        r = dict(gem_like)
        zero_stats(r)
        r.update(itemname=f"{c} Gem of Khalimet", levelreq=55, lore=1, trade=0, rent=1, classuse=32767, raceuse=1023, maxstack=1,
                 itemdesc=f"A gem of Khalimet. Hogarth Bartholomew, south-east of Moradhim, sets it in a Khal weapon: it adds {GEM_WORDS[c]}.")
        ids[r["itemname"]] = insert_item(db, r)
    # the plain weapons
    plain = {"Obliterator": OBLITERATOR}
    for name, spec in WEAPONS.items():
        if spec is None:
            continue
        r = dict(obl)
        zero_stats(r)
        r.update(itemname=name, itemdesc="", equipslot=spec["equipslot"], attacktype=spec["attacktype"], classuse=spec["classuse"],
                 model=spec["look"][0], itemicon=spec["look"][1], sta=25, lore=1, trade=0)
        for k in ("weapondamage", "maxhp", "duration"):
            if k in spec:
                r[k] = spec[k]
        r.update(spec.get("extra", {}))
        plain[name] = insert_item(db, r)
    ids.update(plain)
    # the coloured ones
    for name, pid in plain.items():
        base = record(db, pid)
        for c, adds in GEMS.items():
            cname = f"{c} {name}"
            if cname in KEPT_COLOURED:
                ids[cname] = KEPT_COLOURED[cname]
                continue
            if cname == "White Obliterator" and record(db, WHITE_OBLITERATOR) and record(db, WHITE_OBLITERATOR)["itemname"] == cname:
                ids[cname] = WHITE_OBLITERATOR
                continue
            r = dict(base)
            for k, v in adds.items():
                r[k] = (r.get(k) or 0) + v
            if c == "White" and r["weapondamage"]:
                r["weapondamage"] = WHITE_DAMAGE
            if cname == "White Obliterator":
                r["model"], r["itemicon"] = WHITE_OBL_LOOK
            r.update(itemname=cname, itemdesc="", trade=1, lore=1)
            ids[cname] = insert_item(db, r)
    # the kept coloured Obliterators: the import's STR 75 is the gem's PoT / HoT 75 (t288; the film's Yellow Bloodplague)
    for pid, col in ((1022, "PoT"), (1023, "HoT")):
        r = record(db, pid)
        if r["str"] == 75 and not r[col]:
            db.execute(f'UPDATE itempattern SET str = 0, "{col}" = 75 WHERE patternid = ?', (pid,))
    # the robes
    robe = record(db, ROBE_LIKE)
    for name, spec in ROBES.items():
        r = dict(robe)
        zero_stats(r)
        r.update(spec["stats"])
        r.update(itemname=name, itemdesc="", classuse=spec["classuse"], color=spec["color"], maxhp=25000, duration=80, levelreq=55, trade=1, lore=1)
        ids[name] = insert_item(db, r)
    ids["Khal Robe of War"] = ROBE_LIKE
    # the wave weapons
    for name, spec in WAVE_WEAPONS.items():
        r = dict(obl)
        zero_stats(r)
        r.update(itemname=name, itemdesc="", attacktype=spec["attacktype"], model=spec["look"][0], itemicon=spec["look"][1], lore=1, trade=0)
        ids[name] = insert_item(db, r)
    # Khalimet's Call and its gate
    r = dict(gem_like)
    zero_stats(r)
    r.update(itemname=CALL, itemdesc="A stone that answers Khalimet's call. Use it to be carried to the General's isle in Zaoran Plateau.",
             maxstack=CALLS, lore=0, trade=0, rent=0, levelreq=1, classuse=32767, raceuse=1023)
    ids[CALL] = insert_item(db, r)
    gate_from = one(db, "SELECT spellid FROM spellpattern WHERE spellname = ? ORDER BY spellid LIMIT 1", GATE_SPELL_FROM)[0]
    gate = one(db, "SELECT max(spellid) FROM spellpattern")[0] + 1
    cols = [c[1] for c in db.execute("PRAGMA table_info(spellpattern)")]
    sp = dict(zip(cols, one(db, "SELECT * FROM spellpattern WHERE spellid = ?", gate_from)))
    sp.update(spellid=gate, spellname=GATE_NAME, abilitylvl=1, casttime=0, power=0, recast=0, spelldesc=GATE_LINE)
    db.execute(f'INSERT INTO spellpattern ({",".join(sp)}) VALUES ({",".join("?" * len(sp))})', list(sp.values()))
    fx = one(db, "SELECT fxdict, effects FROM spelleffect WHERE spellid = ?", gate_from)
    eff = json.loads(fx[1]) if fx and fx[1] else {}
    if eff:
        eff["level"] = 1
        for e in eff.get("effects", []):
            if e.get("kind") == "teleport":
                e["location"] = f"Zaoran Plateau,{PAD_AT[1]},{PAD_AT[2]},{PAD_AT[3]}"
    db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?,?,?,?,?)",
               (gate, GATE_LINE, fx[0] if fx else 0, f"{SRC} (visual {GATE_SPELL_FROM}'s)", json.dumps(eff, separators=(",", ":")) if eff else ""))
    db.execute("""INSERT INTO spellgates (spell, kind, place, world, x, y, z, radius, note, source) VALUES (?, 'self', ?, ?, ?, ?, ?, 0, ?, ?)""",
               (GATE_NAME.lower(), "the General's isle in Zaoran Plateau", PAD_AT[0], PAD_AT[1], PAD_AT[2], PAD_AT[3],
                "684's film: Khalimet's Call loads the raid onto a rune pad in sight of Khal Viceroy (the isle [ours])", SRC))
    db.execute("""CREATE TABLE IF NOT EXISTS itemspell (patternid INTEGER PRIMARY KEY, spellid INTEGER NOT NULL,
                  rest_s REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")
    db.execute("INSERT INTO itemspell (patternid, spellid, rest_s, note, source) VALUES (?, ?, 0, ?, ?)",
               (ids[CALL], gate, "Khalimet's Call carries its user to the General's isle", SRC))
    if sgg.has(db, "itemeffect"):
        ie_cols = [c[1] for c in db.execute("PRAGMA table_info(itemeffect)")]
        row = {"patternid": ids[CALL], "line": GATE_LINE, "name": GATE_NAME, "charges": 1, "source": SRC}
        row = {k: v for k, v in row.items() if k in ie_cols}
        db.execute(f'INSERT INTO itemeffect ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
    if sgg.has(db, "norentvanish"):
        nr_cols = [c[1] for c in db.execute("PRAGMA table_info(norentvanish)")]
        row = {"patternid": ids[CALL], "note": "684: the port stones, NO RENT", "source": SRC}
        row = {k: v for k, v in row.items() if k in nr_cols}
        db.execute(f'INSERT INTO norentvanish ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
    ids["_gate"] = gate
    say(f"items: {len([k for k in ids if not k.startswith('_')])} named, gate spell {gate}")
    return ids


def fill_gemsetting(db, ids):
    db.execute("""CREATE TABLE IF NOT EXISTS gemsetting (patternid INTEGER PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'weapon',
        family TEXT NOT NULL DEFAULT '', colour TEXT NOT NULL DEFAULT '', canonical INTEGER NOT NULL DEFAULT 1,
        note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")
    n = 0
    for c in GEMS:
        db.execute("INSERT INTO gemsetting (patternid, kind, family, colour, canonical, note, source) VALUES (?, 'gem', '', ?, 1, '', ?)",
                   (ids[f"{c} Gem of Khalimet"], c.lower(), SRC))
        n += 1
    for name in WEAPONS:
        for c in [""] + list(GEMS):
            full = f"{c} {name}".strip()
            canon = ids[full]
            for pid in by_name(db, full, WEAPON_SLOTS):
                note = "" if pid == canon else "a copy of the record: taken in, never given"
                db.execute("INSERT INTO gemsetting (patternid, kind, family, colour, canonical, note, source) VALUES (?, 'weapon', ?, ?, ?, ?, ?)",
                           (pid, name.lower(), c.lower(), 1 if pid == canon else 0, note, SRC))
                n += 1
    return n


def spec(db):
    """The guide set's part: templates, NPC rows, triggers, deaths and the Part II quest."""
    s = {"set": SET, "quests": "harbingers.json", "templates": [], "npcs": [], "triggers": [], "deaths": []}
    for name, (level, hp_pct, top, extra) in TEMPLATES.items():
        t = {"name": name, "class": 0, "hp_pct": hp_pct, "level": level, "melee": melee_for(db, level, top),
             "note": TEMPLATE_NOTES.get(name, f"684/699: L{level}, hp_pct {hp_pct}, a top hit of ~{top:,} on a ~7,000 AC warrior [ours]")}
        t.update({k: v for k, v in extra.items()})
        s["templates"].append(t)
    for name, like, place, size, ntype in SPAWN_ROWS:
        n = {"name": name, "like": like, "world": place[0], "x": place[1], "y": place[2], "z": place[3], "size": size,
             "level": TEMPLATES[name][0] if name in TEMPLATES else 60, "spawnonly": True, "note": f"{SRC}: spawned by the Harbingers' triggers"}
        if name in TEMPLATES:
            n["template"] = name
        if ntype is not None:
            n["type"] = ntype
        else:
            n["type"] = 0
        s["npcs"].append(n)
    for k, (x, y, z) in enumerate(CAMPS):
        mobs = [("Khal Conscript", 4), ("Khal Garrison", 2)] + ([("Khal Captain", 1)] if k == 0 else [])
        for m, cnt in mobs:
            for j in range(cnt):
                a = 2 * math.pi * j / max(cnt, 1) + k
                r = 8 if m != "Khal Captain" else 0
                s["npcs"].append({"name": m, "like": "Khal Zealot", "world": 0, "x": round(x + r * math.cos(a), 1), "y": y,
                                  "z": round(z + r * math.sin(a), 1), "level": TEMPLATES[m][0], "type": 0,
                                  "size": 1.0 if m == "Khal Conscript" else 1.1, "template": m,
                                  "note": f"{SRC}: camp {k + 1} of six round the Khal Zealots, Permafrost (S) [spot ours]"})
    s["triggers"] = triggers()
    s["deaths"] = [{"name": n, "trigger": t, "chance": c, "note": note} for n, t, c, note in DEATHS]
    s["sql_after"] = [["UPDATE quests SET enabled = 0 WHERE questid = (SELECT questid FROM quest_import_quests WHERE key = ?)", [PART2_KEY]]]
    return s


def npc_edits(db, say):
    """Part III and V's rows: levels, bodies, templates, talkers, spawn-only, respawns."""
    for name in ("Khal Zealot", "Khal Reaver", "The Keeper", "Anibael", "weary adventurer", "Gnadriin", "Tobral Arnez", "Arliss"):
        db.execute("UPDATE npcs SET npc_level = ? WHERE npc_name = ?", (TEMPLATES[name][0], name))
        db.execute("INSERT OR REPLACE INTO npctemplate (name, template, note, source) VALUES (?, ?, ?, ?)",
                   (name.lower(), name.lower(), "699", SRC))
    db.execute("UPDATE npcs SET npc_type = 128 WHERE npc_name IN ('The Keeper', 'Anibael')")
    goblin = one(db, "SELECT modelid FROM npcs WHERE npc_name = 'goblin veteran' LIMIT 1")[0]
    db.execute("UPDATE npcs SET modelid = ? WHERE npc_name = 'Arliss'", (goblin,))
    db.execute("INSERT OR IGNORE INTO npcspawnonly (name, note, source) VALUES ('gnadriin', ?, ?)",
               ("699 Part V: one spawns in place of each weary adventurer killed", SRC))
    for rid, in db.execute(f"SELECT id FROM npcs WHERE npc_name IN ({','.join('?' * len(CAMP_MOBS))})", CAMP_MOBS).fetchall():
        db.execute("INSERT OR REPLACE INTO npcrespawn (npc_row, seconds, note, source) VALUES (?, 600, ?, ?)", (rid, "699: 10 minutes [ours]", SRC))
    # Gnadriin's steps: The Keeper (2) and Anibael (5) raised hostile; four adventurers at step 8, Gnadriin by their deaths
    qid = one(db, "SELECT questid FROM quest_import_quests WHERE key = ?", GNADRIIN_KEY)[0]
    for step, npc, count in ((2, "The Keeper", 1), (5, "Anibael", 1), (8, "weary adventurer", 4)):
        ev = one(db, "SELECT events FROM queststeps WHERE questid = ? AND step = ?", qid, step)[0]
        evs = json.loads(ev) if ev.strip() else []
        evs = [e for e in evs if not (e.get("do") == "spawn" and e.get("npc") in ("weary adventurer", "Gnadriin"))]
        evs.append({"on": "start", "do": "spawn", "npc": npc, "hostile": True, **({"count": count} if count > 1 else {})})
        db.execute("UPDATE queststeps SET events = ? WHERE questid = ? AND step = ?", (json.dumps(evs, separators=(",", ":")), qid, step))
    say(f"Part V: Gnadriin (quest {qid}) steps 2, 5, 8")


def loot(db, ids, say):
    def table(name, items):
        for it in items:
            pid, w = (it, 1) if isinstance(it, int) else it
            db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, ?, 1, 100, ?)",
                       (name, pid, w, SRC))

    def bind(npc, name, chance, rolls, note):
        db.execute("INSERT OR REPLACE INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                   (npc.lower(), name, chance, rolls, note, SRC))

    plain = [ids[w] for w in WEAPONS]
    table("khal gems", [ids[f"{c} Gem of Khalimet"] for c in GEMS])
    table("khal viceroy", plain)  # beside the three coloured Obliterators it holds
    table("khal viceroy sword", [960212])
    table("khal waves", [701569])
    table("khal wave weapons", [ids["Khal Basher"], ids["Khal Battleblade"]])
    table("khal robes", [ids[r] for r in ("Khal Robe of War", "Khal Robe of Shadows", "Khal Robe of Magic", "Khal Robe of Faith")])
    table("khal weapons", plain)
    bind("Khal Viceroy", "khal gems", 100, 1, "682: \"1 spell, 1 gem, 2 khal weps\"")
    bind("Khal Viceroy", "khal viceroy", 100, 2, "682: two Khal weapons (the table's three coloured Obliterators among them)")
    bind("Khal Viceroy", "zp spells", 100, 1, "682: one ZP ability spell")
    bind("Khal Viceroy", "khal viceroy sword", 5, 1, "682: Sapphyra's Sword \"has been known to drop\" (5% ours)")
    for m in WAVE_SOLDIERS:
        bind(m, "khal waves", 10, 1, "684: \"looted achroite\" (10% ours)")
        bind(m, "khal wave weapons", 5, 1, "684: Khal Basher, Khal Battleblade (5% ours)")
    bind("Khal Warlord", "khal robes", 100, 1, "699: one Khal robe a kill")
    bind("Khal Warlord", "khal weapons", 50, 1, "699: a chance at a Khal weapon (50% ours)")
    say("loot: 7 tables, Viceroy, wave soldiers and Warlord bound")


def part2_give(db, ids):
    """The post's give step hands sixteen Khalimet's Calls (the record's id is the shard's)."""
    qid = one(db, "SELECT questid FROM quest_import_quests WHERE key = ?", PART2_KEY)[0]
    ev = one(db, "SELECT events FROM queststeps WHERE questid = ? AND step = 3", qid)[0]
    evs = json.loads(ev) if ev.strip() else []
    evs.insert(0, {"on": "done", "do": "give", "items": f"{ids[CALL]}*{CALLS}"})
    db.execute("UPDATE queststeps SET events = ? WHERE questid = ? AND step = 3", (json.dumps(evs, separators=(",", ":")), qid))
    db.execute("UPDATE quests SET enabled = 0 WHERE questid = ?", (qid,))


def audit(db, action, after):
    acct = one(db, "SELECT accountid FROM accountinfo WHERE username = ?", WHO) if sgg.has(db, "accountinfo") else None
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'guide_set_log', ?, NULL, ?, ?)",
               (when, acct[0] if acct else 0, WHO, VIA, action, json.dumps({"set": SET}), json.dumps(after, sort_keys=True), DETAIL))


def apply(db, commit, say):
    sgg.TABLES = tuple(dict.fromkeys(sgg.TABLES + EXTRA_TABLES))
    db.execute("BEGIN IMMEDIATE")
    try:
        bad = refusals(db)
        if bad:
            raise Refused("; ".join(bad))
        before = sgg.snapshot(db)
        ids = make_items(db, say)
        s = spec(db)
        sgg.run(db, s, say)
        n = fill_gemsetting(db, ids)
        say(f"gemsetting: {n} rows")
        npc_edits(db, say)
        loot(db, ids, say)
        part2_give(db, ids)
        after = sgg.snapshot(db)
        ch = sgg.changes(before, after)
        sgg.log_changes(db, SET, ch)
        counts = {}
        for t, op, _, _, _ in ch:
            counts[f"{t} {op}"] = counts.get(f"{t} {op}", 0) + 1
        say(json.dumps(counts, sort_keys=True))
        if commit:
            audit(db, "apply", counts)
            db.execute("COMMIT")
            say(f"applied {SET}")
        else:
            db.execute("ROLLBACK")
            say("(a report: nothing written)")
        return counts
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, say):
    sgg.undo(db, SET, say, extra=lambda d, n: audit(d, "undo", {"rows": n}))


def dump():
    """The triggers, deaths and spawn-only names, for the Go test (internal/handlers/harbingers_test.go)."""
    trig = []
    for t in triggers():
        t = dict(t)
        t["steps"] = t["steps"]
        trig.append(t)
    npcs = [{"name": n, "spawnonly": True} for n, *_ in SPAWN_ROWS]
    print(json.dumps({"triggers": trig, "npcs": npcs, "deaths": [{"name": n, "trigger": t, "chance": c} for n, t, c, _ in DEATHS]}))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--dump", action="store_true")
    a = ap.parse_args(argv)
    if a.dump:
        dump()
        return 0
    db = sqlite3.connect(a.db, timeout=5, isolation_level=None)
    try:
        if a.undo:
            undo(db, print)
        else:
            apply(db, a.apply, print)
    except Refused as e:
        print("refused:", e)
        return 1
    finally:
        db.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
