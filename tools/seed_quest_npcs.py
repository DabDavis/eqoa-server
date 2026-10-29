#!/usr/bin/env python3
"""The NPCs and creatures the class quests need and the database lacks, added.

The user: "add the missing NPCs for the quests". tools/quests/data/*.json
(EQOA-Data's 1-20 city guides made into quests, tools/quests/FORMAT.md) name
in each quest's `unresolved` the NPCs and creatures no `npcs` row has, and a
quest with anything unresolved is imported as a draft. This tool adds them,
one spawn for a named NPC and a few for a kind the quests kill in numbers,
and makes the quests' three kill targets that stand as townsfolk (npc_type
0x80: Squire Janxt, Gamor, Furdle Nimdar) attackable.

What it decides, and from what (SPECS below, one entry a name):

- Where, the first of these that has the name:
  1. FrontiersForge's sightings of it (npcs_ff_import, tools/import_ff_npcs.py:
     the live game's own positions, game coordinates; a sighting marked
     pos_suspect is not used);
  2. EQOA-Data's map (Map/zones.json `pois`, the quest guides' points, tied
     to a guide by questGuideIds), laid over the game: Tunaria by a fit of
     the map's points to the NPCs of the same unique names (x = 2.13 mx -
     3633, z = 2.13 my + 3793; a median 7 units off), each point then moved
     by the mean miss of the nearest points FrontiersForge also saw (the
     sheet is drawn by hand: round Grobb it is 700 off); the Rathe (world 1),
     which the sheet draws as an inset at map (4500-8500, 9000-12800), by
     the same scale anchored on Oggok (the map's "Oggok" to the town's
     people's middle): too few of its points match a world-1 NPC for a fit;
  3. a place the guide names, beside an NPC that stands there (the guide's
     "outside the South Gate", "at the Pilgrim's Inn", "in Bastable").
  Then on the ground: the floor nearest the place's height (the sighting's,
  the anchor's, else the NPCs' round it) in the operator's ground files
  (cmd/groundbake; seed_wildlife.py's reader), or a lower one where the
  guide says the creature is below ("two levels down"), out of the water
  (the zone leaves' flags, `<world>.env`, seed_pod.py's reader) but for a
  fish; a place with no floor is left out and listed.
- Who else: a town's newbie quests' creatures the server has only far off
  (Surefall Glade's bats, badgers and vipers stood by Qeynos, 6,000 away;
  Highbourne's rats and tarantulas 4,000 to 17,000), a few of the same
  name and body outside the town, and the creatures the guides put round
  a quest NPC's house (the old woman's in Jethro's Cast).
- What: a body copied from an NPC the server has, every column of its row
  (so it is drawn, sized and dressed as its model): a person of the race
  the guide's words or the place give (charactermodel's bodies; a named
  NPC's own family where one stands, Haltak V`Ghen for his brothers), a
  creature of the kind its name says (a lizardman's, a froglok's, a bear for
  Trapmaw), the nearest of the most common body of that kind.
- Its row: the spelling the game gives it where FrontiersForge saw it
  (backticks in the Teir'dal names, "Dragoon V`Ghen"), npc_type 0 for what
  a quest kills and 128 for whom it talks to (as the cities' quest NPCs
  are), npc_id 0 (no list), its level the sighting's, the map's, else the
  quest's, facing its anchor or the town.
- How many: SPECS' count, round the place (a kind's spawns 25-120 apart).

The quest files are kept in step by hand (this tool never writes them): their
targets use these spellings and their `unresolved` lists what is still
missing with the seed in. Every row it writes is in one transaction and
logged in quest_npc_seed_rows (the npcs row, the name, its source, the row it
copies), every npc_type it changes in quest_npc_seed_types (the old value);
--undo deletes exactly those rows, puts the types back (a row a GM changed
since is left alone, said) and drops the logs. The same database and ground
give the same plan (a fixed random seed). Report only by default. Stop the
server to apply and start it after (it reads npcs at the start).

Seed sets. SPECS below are the set "1-20" (the city guides'); every
tools/quests/npcs/<set>.json is one more ({"set": "<set>", "npcs": [Spec,
...], "attackable": [names]}: a Spec is S()'s arguments by name, its body
and place as JSON lists, FORMAT.md), written for the later guides. Each set
is applied and taken back on its own (--apply --set X, --undo --set X), its
rows, types and meta logged under its name (quest_npc_seed_*.seedset), so
several are in at once; a set already in is refused, not planned again. A log
made before the sets (one seed, no seedset column) is the set 1-20's, made so
in place the first time a set is applied or undone. Sets applied in one run
go in the order named, each planned against the database with the ones
before it in.

    tools/seed_quest_npcs.py --ground worlds/ground [--db eqoa.db]
        [--data ../external/EQOA-Data] [--specs FILE ...] [--set X ...]
        [--apply | --undo] [--verbose]
"""
import argparse
import collections
import datetime
import importlib.util
import json
import math
import os
import random
import re
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
DEFAULT_GROUND = os.path.join(HERE, "..", "worlds", "ground")
DEFAULT_DATA = os.path.join(HERE, "..", "..", "external", "EQOA-Data")
DEFAULT_SPECS = os.path.join(HERE, "quests", "npcs")
DEFAULT_SEED = 18
BUILTIN = "1-20"   # the set SPECS and ATTACKABLE are

# seed_wildlife.py reads the ground files; its Ground is this tool's too
_spec = importlib.util.spec_from_file_location("seed_wildlife", os.path.join(HERE, "seed_wildlife.py"))
_sw = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_sw)
Ground = _sw.Ground
# and seed_pod.py the water files
_spec = importlib.util.spec_from_file_location("seed_pod", os.path.join(HERE, "seed_pod.py"))
_sp = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_sp)
Env = _sp.Env

FOE, TALKER = "foe", "talker"
TYPE = {FOE: 0, TALKER: 128}
RACE = dict(human=0, elf=1, darkelf=2, gnome=3, dwarf=4, troll=5, barbarian=6, halfling=7, erudite=8, ogre=9)
MALE, FEMALE = 0, 1

# a kind's look, by its name's words (the NPCs a creature's body is copied from)
KIND = {
    "aviak": r"\bav[ai][ai]?k\b", "bear": r"\bbear\b", "alligator": r"\balligator\b", "boar": r"\bboar\b",
    "brownie": r"\bbrownie\b", "centipede": r"\b(centipede|crawler)\b", "clockwork": r"\bclockwork\b",
    "crab": r"\bcrab\b", "dog": r"\b(wolf|dog)\b", "fireflies": r"\bfireflies\b", "fire goblin": r"\bfire goblin\b",
    "freezeblood": r"^freezeblood\b", "frosteye": r"^frosteye\b", "gnoll": r"\bgnoll\b",
    "froglok": r"\bfroglok\b", "ghost": r"\b(ghost|spectre|phantom|wraith)\b", "ghoul": r"\bghoul\b",
    "goblin": r"^goblin\b", "golem": r"\b(stone|granite|clay) golem\b", "hobgoblin": r"\bhobgoblin\b",
    "insect": r"\b(beetle|scarab)\b", "kingsnake": r"\bkingsnake\b", "lion": r"\blion\b",
    "lizardman": r"\blizard ?m[ae]n\b", "mosquito": r"\bmosquito", "orc": r"^orc\b", "pike": r"\bpike\b",
    "piranha": r"\bpiranha\b", "pixie": r"\bpixie\b", "rat": r"^(large |giant )?rat\b", "skeleton": r"\bskeleton\b",
    "snake": r"\bsnake\b", "spider": r"\b(spiderling|spider)\b", "ant": r"\bants?\b", "tae ew": r"\btae ew\b",
}


LAND, WATER, BOTH = "land", "water", "both"


def S(name, role, body, place, count=1, level=None, note="", water=LAND):
    return dict(name=name, role=role, body=body, place=place, count=count, level=level, note=note, water=water)


# body: ("race", race, sex) | ("kind", KIND key) | ("npc", name: that row's body)
# place: ("ff",) | ("poi", "tunaria" | "rathe", poi name) | ("near", npc, distance, world) |
#        ("at", world, x, z) ; each may be followed by "low" (a lower floor) in the tuple's end
# water: LAND (out of the water), WATER (a fish: in it) or BOTH (an alligator, a crab)
SPECS = [
    # --- Freeport ---
    S("Crim Arikson", TALKER, ("race", "human", MALE), ("ff",), note="the forger in Bastable Village"),
    S("Eliene", FOE, ("race", "human", FEMALE), ("ff",), note="a thief in Bastable"),
    S("Nanik Novandear", FOE, ("npc", "Domnik Novandear"), ("ff",), note="at the broken bridge north of Freeport, with his family"),
    S("Glixin", TALKER, ("race", "gnome", MALE), ("near", "Crim Arikson", 18, 0), level=20,
      note="sells two potions in Bastable Village"),
    S("Korpus", FOE, ("race", "human", MALE), ("ff",), note="a human necromancer, east along Freeport's shore"),
    S("Helthea", FOE, ("race", "human", FEMALE), ("ff",), note="at the Pilgrim's Inn's door"),
    S("Efram", FOE, ("race", "human", MALE), ("near", "Helthea", 9, 0), level=13, note="at the Pilgrim's Inn"),
    S("Thrun", FOE, ("race", "human", MALE), ("near", "Helthea", 12, 0), level=13, note="at the Pilgrim's Inn"),
    S("Croc", FOE, ("race", "human", MALE), ("ff",), note="keeps the hatestone on Croc Island, off Freeport"),
    S("Grebarg", TALKER, ("race", "troll", MALE), ("ff",), note="waits at the whale bones south of Freeport"),
    S("Elder of Hodstock", FOE, ("race", "human", MALE), ("near", "Slick Stevenson", 25, 0), level=20,
      note="Hodstock, the western part of Hodstock and Temby"),
    S("Kithicor Thug", FOE, ("race", "human", MALE), ("ff",), count=2, note="near Saerk's tower"),
    S("Wandering Herbalist", FOE, ("race", "human", MALE), ("ff",), count=3, note="Mu Lin's Reach"),
    S("Orc Gatherer", FOE, ("kind", "orc"), ("ff",), count=3, note="north-western Ro"),
    # --- Neriak and its roads ---
    S("Counselor Du`Klot", TALKER, ("race", "darkelf", MALE), ("ff",)),
    S("Denouncer R`Noxt", TALKER, ("race", "darkelf", MALE), ("ff",)),
    S("Denouncer T`Crit", TALKER, ("race", "darkelf", MALE), ("ff",), note="above the ghoul captains' dock"),
    S("Dragoon V`Ghen", TALKER, ("npc", "Haltak V`Ghen"), ("ff",), note="the outpost north of the city; Haltak's brother"),
    S("Valrean V`Ghen", FOE, ("npc", "Haltak V`Ghen"), ("near", "Dragoon V`Ghen", 55, 0), level=13,
      note="in the tent below the outpost (the map's point, 55 from his brother); the files spelled him Valraen and Valrean"),
    S("Dragoon X`Talin", TALKER, ("race", "darkelf", MALE), ("ff",), note="his tower up the river west of Neriak"),
    S("Arzen", FOE, ("race", "darkelf", MALE), ("ff",), note="on the rocky hill north of X`Talin's tower"),
    S("Xeruin", FOE, ("race", "darkelf", FEMALE), ("ff",), note="camps in the tents up the river with Guryon"),
    S("Grim", FOE, ("kind", "dog"), ("ff",), note="Guryon's dog"),
    S("Zezilbe Veldinox", FOE, ("race", "darkelf", MALE), ("ff",), note="on the road to Klick'Anon"),
    S("Captain Thender", FOE, ("race", "human", MALE), ("ff",), note="at the dock"),
    S("Tailor Soldark", TALKER, ("race", "darkelf", MALE), ("near", "Pazin J`Esal", 25, 0), level=20,
      note="at the bottom of the ramp inside Neriak's entrance, on the city's floor"),
    S("Goblin Alchemist", FOE, ("kind", "goblin"), ("ff",), count=2, note="near the goblin camp at Saerk's Tower"),
    S("Imprisoned Wizard", FOE, ("race", "human", MALE), ("at", 0, 25362.0, 8719.0), level=13,
      note="at the far end of the arena: the map's point moved 24 to where the arena's floor (the city's) lies"),
    S("Elf Paladin", FOE, ("race", "elf", MALE), ("near", "Imprisoned Wizard", 9, 0), level=13,
      note="at the far end of the arena (the map's point is 8 off the wizard's, where no floor of the arena lies)"),
    S("Arena Troll", FOE, ("race", "troll", MALE), ("near", "Imprisoned Wizard", 16, 0), level=13,
      note="the troll at the far end of the arena (the guide names him not)"),
    S("Final Thought", FOE, ("kind", "ghost"), ("at", 0, 22850.0, 8660.0), count=3, level=15,
      note="roam near the Crest of the Duelist in Collonridge Cemetery (Allister's tombstone's reach step)"),
    # --- Grobb and the swamps ---
    S("Smithy Cragis", TALKER, ("race", "troll", MALE), ("ff",)),
    S("Torgot", TALKER, ("race", "troll", MALE), ("ff",), note="the Basher Enclave"),
    S("Krazul", FOE, ("race", "troll", MALE), ("ff",), note="the mushroom patch west along the mountain"),
    S("Traitor", FOE, ("race", "troll", MALE), ("poi", "tunaria", "Traitor"), level=10,
      note="a troll at the mushroom patch"),
    S("Grothmal", FOE, ("kind", "froglok"), ("ff",), note="the mushroom patch west of the war camp, at night"),
    S("Bogscraper", FOE, ("kind", "alligator"), ("ff",), water=BOTH, note="an alligator in the swamps round Clagga Village"),
    S("Bogwater", FOE, ("kind", "alligator"), ("near", "Bogscraper", 320, 0), level=17, water=BOTH,
      note="the other named alligator, in another of the three swamps"),
    S("Froglok Novitiate", FOE, ("kind", "froglok"), ("ff",), count=4, note="the swamp north-west of Grobb"),
    S("Froglok Apprentice", FOE, ("kind", "froglok"), ("ff",), count=2, note="the Hand of Thule"),
    S("Froglok Squire", FOE, ("kind", "froglok"), ("ff",), count=2, note="Godhand, at night"),
    S("Froglok Page", FOE, ("kind", "froglok"), ("ff",), note="between Godhand and the North Crossroads"),
    S("Froglok Tactician", FOE, ("kind", "froglok"), ("ff",), note="between the raider camps north of Grobb"),
    S("Froglok Knight", FOE, ("kind", "froglok"), ("ff",), note="the Northern Crossroads"),
    S("Froglok Tuk", FOE, ("kind", "froglok"), ("ff",), note="the caves south-west of Honjour"),
    S("Vengeful Dead", FOE, ("kind", "skeleton"), ("ff",), count=3, note="the tower, at night"),
    S("Hazinak Troll", TALKER, ("race", "troll", MALE), ("near", "Coachman Droogra", 24, 0), level=20,
      note="evil Hazinak, by its coach"),
    S("Druid Trainee", FOE, ("race", "human", MALE), ("ff",), count=4, note="the outpost north-west of Honjour"),
    S("Grugnik Blackface", TALKER, ("race", "darkelf", MALE), ("near", "Coachman Norial", 80, 0, "north"), level=30,
      note="Honjour, north of the coach"),
    # --- Oggok, the Rathe (world 1) ---
    S("Bouncer Trod", TALKER, ("race", "ogre", MALE), ("at", 1, 8805.0, 6735.0), level=35,
      note="the guard tower north-west of Oggok's north entrance"),
    S("Guard Fraug", TALKER, ("race", "ogre", MALE), ("poi", "rathe", "Moggok's Gate"), level=35,
      note="the outpost near Moggok's Gate"),
    S("Lork", TALKER, ("race", "ogre", MALE), ("near", "Guard Fraug", 30, 1), level=20,
      note="buried by the catapult across the outpost (the guide's spirit stands here)"),
    S("Merchant Zogog", TALKER, ("race", "ogre", MALE), ("at", 1, 9030.0, 7640.0), level=30,
      note="near the South Gate (sells nothing: the quest hands the extract over)"),
    S("Lokar", TALKER, ("race", "ogre", MALE), ("at", 1, 9060.0, 7800.0), level=20, note="outside the South Gate"),
    S("Thodgar", TALKER, ("race", "ogre", MALE), ("near", "Elder Thorg", 25, 1), level=30, note="sells mudblood"),
    S("mosquitos", FOE, ("kind", "mosquito"), ("at", 1, 9050.0, 7960.0), count=4, level=2,
      note="beyond the South Gate (the game's name, as Tunaria's)"),
    S("Ghoul", FOE, ("kind", "ghoul"), ("at", 1, 9055.0, 7880.0, "low"), level=20,
      note="two levels down in the first building south of the south entrance"),
    S("Lizardman Outcast", FOE, ("kind", "lizardman"), ("poi", "rathe", "Lizardman Outcast"), count=2, level=5,
      note="the abandoned tower south-west of Oggok"),
    S("Lizardman Sneak", FOE, ("kind", "lizardman"), ("poi", "rathe", "Lizardman Sneak"), count=3, level=15,
      note="the lake's south shore"),
    S("Wandering Forager", FOE, ("kind", "lizardman"), ("poi", "rathe", "Lizardman Forager"), level=15,
      note="the lizardman camp (the map's Lizardman Forager)"),
    S("Tainted Rat", FOE, ("kind", "rat"), ("poi", "rathe", "Tainted Rat"), count=4, level=7,
      note="the ruins of Amog-Thelg"),
    S("Undead Alchemist", FOE, ("kind", "skeleton"), ("poi", "rathe", "Undead Alchemist"), level=10,
      note="woken by the fire at Amog-Thelg"),
    S("Ogre Magi", FOE, ("race", "ogre", MALE), ("poi", "rathe", "Undead Alchemist", "low"), count=2, level=13,
      note="Amog-Thelg's lower levels"),
    S("Necrapede", FOE, ("kind", "centipede"), ("poi", "rathe", "Necrapede"), level=16, note="south of Oggok"),
    S("Fat Piranha", FOE, ("kind", "piranha"), ("poi", "rathe", "Fat Piranha"), level=13, water=WATER),
    S("Gruk Crab", FOE, ("kind", "crab"), ("poi", "rathe", "Gruk Crab"), count=3, level=15, water=BOTH,
      note="the lake shore north-west of the north gate"),
    S("Burdened Mog Thug", FOE, ("race", "ogre", MALE), ("poi", "rathe", "Burdened Mog Thug"), level=17,
      note="near the Sphinx Pyramids"),
    S("Hobgoblin Initiate", FOE, ("kind", "hobgoblin"), ("poi", "rathe", "Stump"), level=13,
      note="by the stump with a fern"),
    S("Hobgoblin Vandal", FOE, ("kind", "hobgoblin"), ("at", 1, 8350.0, 7450.0), count=3, level=13,
      note="the valley west by south-west of the second tower"),
    S("Ankextet Runner", FOE, ("kind", "insect"), ("at", 1, 8500.0, 6500.0), level=20,
      note="north-west of Oggok (an insect's body: the server has no ankextet)"),
    S("Aviak Windstrider", FOE, ("kind", "aviak"), ("near", "Cawtou ranger avaik", 60, 1), count=3, level=20,
      note="the aviaks' camp north of Oggok"),
    S("Dwarf Thug", FOE, ("race", "dwarf", MALE), ("near", "orge thug", 50, 1), count=2, level=20,
      note="with the ogre thugs that killed Brogar's courier"),
    # the Feerrott's (Tunaria's south, by Kerplunk)
    S("Tae Ew Commander", FOE, ("kind", "tae ew"), ("near", "Tae Ew hierophant", 45, 0), count=2, level=22,
      note="the Tae Ew camps by Kerplunk"),
    S("Tae Ew Weaponsmith", FOE, ("kind", "tae ew"), ("near", "Tae Ew hierophant", 90, 0), count=2, level=18,
      note="the Tae Ew camps by Kerplunk (the map's Weaponmaker)"),
    S("Weary Troll Raider", FOE, ("race", "troll", MALE), ("at", 0, 14400.0, 29241.0), level=15,
      note="on the hill west of Kerplunk"),
    # --- Klick'Anon and round it ---
    S("Engineer Eagglefoot", TALKER, ("race", "gnome", MALE), ("poi", "tunaria", "Engineer Eagglefoot"), level=20,
      note="the lake's lab"),
    S("Hosco Brixwak", TALKER, ("race", "gnome", MALE), ("ff",)),
    S("Stily Hornbar", TALKER, ("race", "gnome", MALE), ("ff",)),
    S("Watchman Prit", TALKER, ("race", "gnome", MALE), ("ff",)),
    S("Gligbin Gearcharmer", TALKER, ("race", "gnome", MALE), ("ff",),
      note="the files called him Gligbin and Gligbin Gearcharmer"),
    S("Blackpaw", FOE, ("kind", "bear"), ("ff",)),
    S("Gorged Rat", FOE, ("kind", "rat"), ("ff",), count=4, note="by the mushroom patch outside the southern gatewall"),
    S("Model III", FOE, ("kind", "clockwork"), ("ff",), count=3),
    S("Strange Clockwork", FOE, ("kind", "clockwork"), ("ff",)),
    S("Fire Goblin Alchemist", FOE, ("kind", "fire goblin"), ("ff",), count=2),
    S("goblin burglar", FOE, ("kind", "goblin"), ("poi", "tunaria", "goblin burglars"), count=4, level=7,
      note="roam Misty Thicket, past the pickclaws"),
    S("Orc Thief", FOE, ("kind", "orc"), ("ff",), count=3),
    S("Orc Wizard", FOE, ("kind", "orc"), ("poi", "tunaria", "Orc Wizard"), level=10,
      note="near the Hatebone orc cave, between the hills"),
    S("Jarek's Creation", FOE, ("kind", "insect"), ("ff",)),
    S("Grizglurg", FOE, ("kind", "goblin"), ("ff",), note="holds a holy symbol"),
    S("Trapmaw", FOE, ("kind", "bear"), ("ff",), note="a bear south of the village along the small river"),
    S("Stoneite Golem", FOE, ("kind", "golem"), ("poi", "tunaria", "Stoneite Golems"), count=3, level=20,
      note="at Trail's End"),
    # --- Fayspire and Tethelin ---
    S("Amelie", TALKER, ("race", "elf", FEMALE), ("near", "Venathorn", 15, 0), level=30,
      note="the alchemists' level 10 giver, beside Venathorn"),
    S("Sunflies", FOE, ("kind", "fireflies"), ("ff",), count=4, note="the game's name; the files said Sunfly"),
    S("Pixie Nettle", FOE, ("kind", "pixie"), ("at", 0, 17300.0, 9300.0), count=3, level=10,
      note="Shon-To (SE), south-east of the monastery"),
    # --- Highbourne and the Qeynos lands ---
    S("Brother Pendle", TALKER, ("race", "human", MALE), ("ff",)),
    S("Duke Geoff Melnorn", TALKER, ("race", "human", MALE), ("ff",)),
    S("Farny the Woodcutter", TALKER, ("race", "human", MALE), ("ff",), note="near Melton"),
    S("Maryan", TALKER, ("race", "human", FEMALE), ("ff",), note="the Stoneclaw ravines"),
    S("Mayor Reively", TALKER, ("race", "human", MALE), ("ff",), note="Wyndhaven"),
    S("Jargan Nicks", TALKER, ("race", "human", MALE), ("ff",), note="Qeynos"),
    S("Fenra", TALKER, ("npc", "Caller Toshei"), ("near", "Caller Toshei", 12, 0), level=30,
      note="at the library (top floor)"),
    S("Blinkbart Bagglebaff", FOE, ("race", "halfling", MALE), ("ff",), note="the hill east of the windmill"),
    S("Rory Redface", FOE, ("race", "human", MALE), ("ff",), note="with the bandits on the hill north of the gate"),
    S("Grend Darkmaw", FOE, ("race", "human", MALE), ("ff",), note="near Hagley"),
    S("Ganron Blackart", FOE, ("npc", "Caller Toshei"), ("poi", "tunaria", "Ganron Blackart"), level=10,
      note="the traitor caller, in a cave in the ravine to the east"),
    S("Goretusk the Boar", FOE, ("kind", "boar"), ("poi", "tunaria", "Goretusk the Boar"), level=20),
    S("Nubja", FOE, ("kind", "goblin"), ("ff",), note="a goblin by the north gate, at night"),
    S("Brownie Thief", FOE, ("kind", "brownie"), ("ff",), note="at Nalai Belzia's boat, at night"),
    S("greyflesh pike", FOE, ("kind", "pike"), ("ff",), count=2, water=WATER, note="under the docks"),
    S("banded kingsnake", FOE, ("kind", "kingsnake"), ("ff",), count=4),
    S("ribbed snake", FOE, ("kind", "snake"), ("ff",), count=3),
    S("rogue lion", FOE, ("kind", "lion"), ("ff",), count=3),
    S("Spotted Lion", FOE, ("kind", "lion"), ("ff",), count=3, note="west of the walled town"),
    S("decaying rat", FOE, ("kind", "rat"), ("ff",), count=4),
    S("tarantula hatchling", FOE, ("kind", "spider"), ("near", "decaying rat", 140, 0), count=4, level=3,
      note="the newbie yard east of Highbourne, beside the decaying rats"),
    S("Elusive Stone Ant", FOE, ("kind", "ant"), ("at", 0, 5900.0, 21200.0), count=3, level=10,
      note="the wilds east of Highbourne"),
    S("Giggen Aviak", FOE, ("kind", "aviak"), ("ff",), count=2, note="a male and a female, east past the Stoneclaw ravines"),
    S("Alchemist of Arcadia", TALKER, ("race", "erudite", MALE), ("near", "Merchant Samoul", 8, 2), level=30,
      note="Arcadia (Odus), in the merchants' building west of the docks; the guide names no one"),
    S("Sage of Gramash", TALKER, ("race", "erudite", MALE), ("at", 0, 10584.0, 8625.0), level=40,
      note="a tower at Gramash Ruins; the guide names no one"),
    S("Guard of the Sage", TALKER, ("race", "human", MALE), ("near", "Sage of Gramash", 20, 0), level=35,
      note="sends you to Kara Village first; the guide names no one"),
    # --- Qeynos, Hagley and the coast (the server has no one in Hagley village or Hagley Keep) ---
    S("Snyde Cragsmear", TALKER, ("race", "human", MALE), ("ff",), note="the rogues' guildmaster"),
    S("Dilana", TALKER, ("race", "human", FEMALE), ("ff",), note="the inn in Hagley"),
    S("Sheriff Lordun", TALKER, ("race", "human", MALE), ("ff",), note="Hagley"),
    S("Tarnina", TALKER, ("race", "human", FEMALE), ("ff",), note="the guard tower near Hagley"),
    S("Duchess Olinda Melnorn", TALKER, ("race", "human", FEMALE), ("ff",), note="Hagley Keep"),
    S("Merchant Dwindel", TALKER, ("race", "human", MALE), ("poi", "tunaria", "Merchant Dwindel"), level=20,
      note="Hagley (sells nothing: the quests hand the glue and the potion over)"),
    S("Grave Digger Joe", TALKER, ("race", "human", MALE), ("ff",), note="Melton"),
    S("Guard Opheena", TALKER, ("race", "human", FEMALE), ("ff",), note="Shoalhome; Guard Filgen's wife"),
    S("Master Nuzaon Sivrendal", TALKER, ("race", "human", MALE), ("ff",), note="the coastal village"),
    S("Foreman Gerald", TALKER, ("race", "human", MALE), ("ff",), note="over the hill from the coastal village"),
    S("Mildred Morath", TALKER, ("race", "human", FEMALE), ("ff",), note="the coast south of Qeynos"),
    S("Smithy Vorgel", TALKER, ("race", "human", MALE), ("ff",), note="Stormhaven"),
    S("Wilwalt", TALKER, ("race", "human", MALE), ("ff",), note="Stormhaven"),
    S("Old Woman", TALKER, ("race", "human", FEMALE), ("ff",),
      note="her house deep in the forest west of Surefall Glade, in Jethro's Cast"),
    S("Garb", FOE, ("race", "human", MALE), ("ff",), note="the little village east of the Silent Fist"),
    S("Gilen the Dark Piper", FOE, ("npc", "Dovre the Unfaithful"), ("ff",),
      note="the lowest level of the Hagley Tomb (Dovre the Unfaithful's look, who is there)"),
    S("Wyna the Hateful", FOE, ("npc", "Dovre the Unfaithful"), ("ff",), note="the lowest level of the Hagley Tomb"),
    S("Minia the Twisted", FOE, ("npc", "Dovre the Unfaithful"), ("near", "Gilen the Dark Piper", 15, 0), level=15,
      note="the lowest level of the Hagley Tomb, with Gilen and Wyna (the map's point lies 160 off the tomb)"),
    S("Gorbrash Strongarm", FOE, ("kind", "gnoll"), ("ff",), note="leads the gnolls by the Slesher fort"),
    S("Vithara Swiftpaw", FOE, ("kind", "gnoll"), ("ff",), note="with the gnoll champions north of the Slesher camp"),
    S("darkpaw sargeant", FOE, ("kind", "gnoll"), ("poi", "tunaria", "darkpaw sargeant"),
      note="leads the Darkpaw gnolls (the guide's spelling)"),
    S("darkpaw seer", FOE, ("npc", "gnoll mystic"), ("ff",), note="north past the small gnoll camp"),
    S("Gnoll Thief", FOE, ("kind", "gnoll"), ("ff",), note="steals from Foreman Gerald's camp"),
    S("Plague Wolf", FOE, ("kind", "dog"), ("ff",), count=3, note="near the temple"),
    S("Thick Coat Wolves", FOE, ("kind", "dog"), ("ff",), count=4,
      note="south of Hagley (the game's name; the files said thick coat wolf)"),
    S("Undead Wolf Pup", FOE, ("kind", "dog"), ("ff",), count=3, note="south of Hagley"),
    S("thornless treant", FOE, ("npc", "treant"), ("ff",), count=2, note="near Jethro's Cast"),
    S("Grave Rat Herald", FOE, ("kind", "rat"), ("ff",), note="near the aviak village"),
    S("Snow Leopard Herald", FOE, ("npc", "snow leopard"), ("ff",), note="near Greyvax's caves"),
    S("Young Bear Herald", FOE, ("kind", "bear"), ("ff",)),
    S("Gugluk", FOE, ("kind", "froglok"), ("ff",),
      note="the froglok shaman, on a hill south-west of the bridge west of Honjour"),
    # --- Surefall Glade ---
    S("Falwyn Breen", TALKER, ("race", "human", MALE), ("ff",), note="the druids' level 4 giver"),
    S("Aieari Scoutmaster", TALKER, ("npc", "Ranger Sexton"), ("ff",),
      note="on the mountain side near the Slesher fort (a ranger's look)"),
    S("Ranger Gaffy", TALKER, ("npc", "Ranger Kjartan"), ("ff",),
      note="the guard tower west of the glade (the game's name and the map's; the guide's Guard Gaffy)"),
    S("Gilthar", FOE, ("race", "human", MALE), ("ff",), note="an outcast ranger north-west of Gaffy's tower"),
    S("Gnoll Champion", FOE, ("kind", "gnoll"), ("poi", "tunaria", "Gnoll Champions"), count=3,
      note="by the stone north-west of the Slesher fort"),
    S("Gnoll Runner", FOE, ("kind", "gnoll"), ("ff",), note="near Aieari (the ranger's quest also spawns one by the player)"),
    S("Gnoll Shaman Forager", FOE, ("npc", "gnoll mystic"), ("ff",),
      note="the valley near the guard tower west of the glade"),
    S("Surefall Shambler", FOE, ("npc", "treant"), ("ff",), count=3, note="round the glade (a walking plant's look)"),
    # --- Halas, Diren's Hold and Moradhim ---
    S("Iron Eagle", TALKER, ("race", "barbarian", MALE), ("ff",), note="Halas, the rogues' giver"),
    S("Pratcher", TALKER, ("race", "barbarian", MALE), ("ff",), note="Diren's Hold, by the banker on the tower's second floor"),
    S("Freezeblood captain", FOE, ("kind", "freezeblood"), ("ff",), count=2, note="the igloo camp west of Halas"),
    S("Freezeblood Hoarder", FOE, ("kind", "freezeblood"), ("ff",), count=2,
      note="with the Icemasters and Icecrushers (the game's name and the map's; the guide's Freezeblood Horde)"),
    S("Freezeblood Icemaster", FOE, ("kind", "freezeblood"), ("poi", "tunaria", "Goblin Icemaster"),
      note="the Freezeblood camp the warriors' level 7 sends them to (the map's Goblin Icemaster)"),
    S("Freezeblood Icecrusher", FOE, ("kind", "freezeblood"), ("poi", "tunaria", "Goblin Icecrusher"),
      note="the same camp (the map's Goblin Icecrusher)"),
    S("Ice Spider Queen", FOE, ("kind", "spider"), ("ff",), note="far to the west of Halas"),
    S("Numla", FOE, ("kind", "freezeblood"), ("ff",),
      note="the assassin, north of the Freezeblood camp by the wooden T at the water (a scavenger stands in for him)"),
    S("Salmon", FOE, ("npc", "small lake trout"), ("ff",), count=3, water=WATER, note="the river by Diren's Hold"),
    S("Amber", TALKER, ("kind", "dog"), ("ff",),
      note="the little wolf the Frosteye marauders hold in the dark tower east of Diren's Hold"),
    S("Guard Reigald", TALKER, ("npc", "Guard Hargin"), ("ff",),
      note="the lone guard past the first river north-north-west of Moradhim (a Moradhim guard's look)"),
    S("Raldir", TALKER, ("race", "dwarf", MALE), ("at", 0, 14320.0, 8720.0), level=10,
      note="by a tree west of Moradhim, just past the first icy river (the guide's way: the gates, then west along the wall)"),
    S("Red-Eyed Monster", FOE, ("kind", "rat"), ("ff",),
      note="a red-eyed rat by the frozen lake behind the lone guard (the game's spelling)"),
    S("frosteye lurker", FOE, ("kind", "frosteye"), ("ff",), count=2,
      note="north-east of Diren's Hold by the small river"),
    S("Frosteye Marauder", FOE, ("kind", "frosteye"), ("ff",), count=3,
      note="the dark tower east of Diren's Hold (three keep Amber's key)"),
    S("Freezeblood Alchemist", FOE, ("kind", "freezeblood"), ("poi", "tunaria", "Freezeblood Alchemist")),
    S("Freezeblood Sapper", FOE, ("kind", "freezeblood"), ("poi", "tunaria", "Freezeblood Sapper"), count=2,
      note="north of Diren's Hold"),
    S("Freezeblood treasurer", FOE, ("kind", "freezeblood"), ("ff",),
      note="King Ickcil's throne room in the Freezeblood caves"),
    # --- Rivervale, Merry-by-Water and Kithicor ---
    S("Barnimus Giggletoes", TALKER, ("race", "halfling", MALE), ("poi", "tunaria", "Barnimus Giggletoes"),
      note="Merry-by-Water"),
    S("Merchant Fretwater", TALKER, ("race", "halfling", MALE), ("poi", "tunaria", "Merchant Fretwater"), level=20,
      note="hands over dice and worms (sells nothing)"),
    S("Stalis Slithertonuge", TALKER, ("race", "halfling", MALE), ("poi", "tunaria", "Stalis Slithertonuge"), level=20,
      note="south of Kithicor, by the fallen tree"),
    S("Goblin Foreman", FOE, ("kind", "goblin"), ("ff",), note="the tunnel camp south-west of the commanders"),
    S("Suspicious Goblin", FOE, ("kind", "goblin"), ("ff",), note="the camp north-east of the wall's gate"),
    S("Goblin Courier", FOE, ("kind", "goblin"), ("ff",), note="by the goblin tower"),
    S("Goblin Digger", FOE, ("kind", "goblin"), ("ff",), count=4, note="the west side of the wall"),
    # --- a town's newbie creatures the server had only far off, outside the town (their bodies the rows') ---
    # Surefall Glade's: on the flat south of the glade's ridge (its bats, badgers and vipers stood by Qeynos)
    S("Bat", FOE, ("npc", "Bat"), ("at", 0, 8820.0, 13180.0), count=4, level=1, note="outside Surefall Glade"),
    S("Badger", FOE, ("npc", "Badger"), ("at", 0, 8980.0, 13180.0), count=4, level=2,
      note="outside Surefall Glade (FrontiersForge saw a level 2 Badger round it)"),
    S("Grass Viper", FOE, ("npc", "Grass Viper"), ("at", 0, 8820.0, 13330.0), count=4, level=3,
      note="outside Surefall Glade"),
    S("Fireflies", FOE, ("npc", "Fireflies"), ("at", 0, 8980.0, 13330.0), count=4, level=2,
      note="outside Surefall Glade (the game's name; the file said firefly)"),
    S("Ant", FOE, ("npc", "Ant"), ("at", 0, 9100.0, 13250.0), count=4, level=2, note="just outside Surefall Glade"),
    S("firebeetle", FOE, ("npc", "firebeetle"), ("at", 0, 8740.0, 13260.0), count=4, level=3,
      note="just outside Surefall Glade (the game's spelling, Qeynos's 62)"),
    # Highbourne's: by the newbie yard east of the city
    S("Rat", FOE, ("npc", "Rat"), ("near", "decaying rat", 60, 0), count=4, level=2, note="outside Highbourne"),
    S("Tarantula", FOE, ("npc", "Tarantula"), ("near", "tarantula hatchling", 60, 0), count=4, level=3,
      note="outside Highbourne"),
    S("Dragonflies", FOE, ("npc", "Dragonflies"), ("near", "decaying rat", 140, 0), count=4, level=3,
      note="outside Highbourne"),
    S("Ant", FOE, ("npc", "Ant"), ("at", 0, 5004.0, 20963.0), count=4, level=1,
      note="north of Highbourne, where FrontiersForge saw a level 1 Ant"),
    # the old woman's, round her house in Jethro's Cast (the map's points; her black widows, which it has none of, by her)
    S("Black Widow", FOE, ("npc", "Black Widow"), ("near", "Old Woman", 70, 0), count=4, level=13,
      note="near the old woman's house (the server's nearest were 2,500 off)"),
    S("Giant Beetle", FOE, ("npc", "Giant Beetle"), ("poi", "tunaria", "Giant Beetle"), count=3, level=13,
      note="near the old woman's house"),
    S("Tree Viper", FOE, ("npc", "Tree Viper"), ("poi", "tunaria", "tree viper"), count=3, level=13,
      note="near the old woman's house"),
    S("Black Wolf Cub", FOE, ("npc", "Black Wolf Cub"), ("poi", "tunaria", "black wolf cub"), count=3, level=13,
      note="near the old woman's house"),
    S("Young Wolf", FOE, ("npc", "Young Wolf"), ("poi", "tunaria", "black wolf cub"), count=3, level=13,
      note="near the old woman's house (the wizards' guide's, at the map's wolf cubs)"),
]

# Quest targets that stand as townsfolk (npc_type 0x80): made attackable, the
# flag alone cleared (none of them is a merchant, a banker or a trainer: no
# other bit), so the quests that kill them can.
ATTACKABLE = ["Squire Janxt", "Gamor", "Furdle Nimdar"]

# The Tunaria fit's own points are only those that match one NPC there; the
# Rathe inset's anchor is the map's "Oggok" and Oggok's people.
INSET = (4500.0, 8500.0, 9000.0, 12800.0)
SCALE = 2.13
OGGOK_MAP = (6855.0, 10988.0)
SPREAD = (25.0, 120.0)
SPACING = 12.0
CLEAR = 3.0     # and this far from any NPC there is
DECK = 8.0      # a sighting above every floor keeps its height this far round it
TRIES = 300


# ---- names ------------------------------------------------------------------------

def fold(s):
    return (s or "").strip().lower()


def key(s):
    """A name as the map and the sightings are matched: case, apostrophes and
    backticks, a level in brackets, a leading "the" and plurals aside."""
    s = fold(s).replace("`", "'").replace("’", "'")
    s = re.sub(r"\s*\([^)]*\)", "", s)
    s = re.sub(r"^(the|a|an)\s+", "", s.strip())
    s = re.sub(r"[^a-z0-9' ]", " ", s)

    def sing(w):
        if w.endswith("ies"):
            return w[:-3] + "y"
        if w.endswith("ves"):
            return w[:-3] + "f"
        if w.endswith("es") and w[:-2].endswith(("s", "x", "ch", "sh")):
            return w[:-2]
        if w.endswith("s") and not w.endswith("ss"):
            return w[:-1]
        return w
    return " ".join(sing(w) for w in s.split())


# ---- the map's points laid over the game ------------------------------------------

def line(xs, ys):
    n = len(xs)
    mx, my = sum(xs) / n, sum(ys) / n
    a = sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / sum((x - mx) ** 2 for x in xs)
    return a, my - a * mx


def fit(points):
    """gx = a*mx + b, gz = c*my + d by least squares, the far misses dropped."""
    cur = points
    for _ in range(8):
        a, b = line([p[0] for p in cur], [p[2] for p in cur])
        c, d = line([p[1] for p in cur], [p[3] for p in cur])
        res = [math.hypot(a * p[0] + b - p[2], c * p[1] + d - p[3]) for p in cur]
        med = sorted(res)[len(res) // 2]
        keep = [p for p, r in zip(cur, res) if r <= max(3 * med, 150.0)]
        if len(keep) == len(cur):
            break
        cur = keep
    res = sorted(math.hypot(a * p[0] + b - p[2], c * p[1] + d - p[3]) for p in cur)
    return (a, b, c, d), len(cur), res[len(res) // 2]


class Map:
    """EQOA-Data's sheet over Tunaria (a fit) and the Rathe (Oggok-anchored)."""

    def __init__(self, zones, npcs, ff):
        self.pois = zones["pois"] + zones["zones"] + zones["towns"]
        byname = collections.defaultdict(list)
        for n in npcs:
            byname[(n["world"], key(n["name"]))].append(n)
        pairs = []
        for p in zones["pois"] + zones["towns"]:
            if self.in_inset(p):
                continue
            rs = byname.get((0, key(p["name"])))
            if rs and len(rs) == 1:
                pairs.append((p["x"], p["y"], rs[0]["x"], rs[0]["z"]))
        self.tunaria, self.fit_kept, self.fit_median = fit(pairs)
        # the sheet's misses where FrontiersForge saw the same name once
        seen = collections.defaultdict(list)
        for s in ff:
            if s["world"] == 0 and not s["pos_suspect"]:
                seen[key(s["npc_name"])].append(s)
        self.misses = []
        for p in zones["pois"]:
            if self.in_inset(p):
                continue
            s = seen.get(key(p["name"]))
            if s and len(s) == 1:
                gx, gz = self.tunaria_at(p["x"], p["y"])
                dx, dz = s[0]["x_coord"] - gx, s[0]["z_coord"] - gz
                if math.hypot(dx, dz) < 1200:
                    self.misses.append((p["x"], p["y"], dx, dz))
        town = [n for n in npcs if n["world"] == 1 and re.match(r"(bouncer|merchant) ", n["name"].lower())]
        self.oggok = (sum(n["x"] for n in town) / len(town), sum(n["z"] for n in town) / len(town)) if town else None

    @staticmethod
    def in_inset(p):
        return INSET[0] <= p["x"] <= INSET[1] and INSET[2] <= p["y"] <= INSET[3]

    def tunaria_at(self, mx, my):
        a, b, c, d = self.tunaria
        return a * mx + b, c * my + d

    def corrected(self, mx, my):
        """Tunaria's place for a map point, moved by the mean miss of the
        (up to) four nearest points FrontiersForge also saw, within 400 on
        the sheet."""
        gx, gz = self.tunaria_at(mx, my)
        near = sorted(self.misses, key=lambda m: math.hypot(m[0] - mx, m[1] - my))[:4]
        near = [m for m in near if math.hypot(m[0] - mx, m[1] - my) <= 400]
        if near:
            gx += sum(m[2] for m in near) / len(near)
            gz += sum(m[3] for m in near) / len(near)
        return gx, gz

    def rathe_at(self, mx, my):
        ox, oz = self.oggok
        return ox + SCALE * (mx - OGGOK_MAP[0]), oz + SCALE * (my - OGGOK_MAP[1])

    def poi(self, name, guides):
        """The map's point of that name (one of the quest's guides' first)."""
        k = key(name)
        cands = [p for p in self.pois if key(p["name"]) == k]
        mine = [p for p in cands if set(p.get("questGuideIds", [])) & guides]
        pick = mine or cands
        return pick[0] if pick else None


# ---- the database -----------------------------------------------------------------

def load(db):
    db.row_factory = sqlite3.Row
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    npcs = []
    for r in db.execute("SELECT * FROM npcs ORDER BY id"):
        d = dict(r)
        npcs.append(dict(row=d, id=d["id"], name=(d["npc_name"] or "").strip(), world=d["world"] or 0,
                         x=d["x_coord"] or 0.0, y=d["y_coord"] or 0.0, z=d["z_coord"] or 0.0,
                         model=(d["modelid"] or 0) & 0xFFFFFFFF, type=d["npc_type"] or 0))
    bodies = collections.defaultdict(list)  # (race, sex) -> modelids
    pc = set()
    for sex, model, race in db.execute("SELECT sex, modelid, race FROM charactermodel"):
        bodies[(race, sex)].append(model & 0xFFFFFFFF)
        pc.add(model & 0xFFFFFFFF)
    ff = [dict(r) for r in db.execute("SELECT * FROM npcs_ff_import ORDER BY id")]
    db.row_factory = None
    return cols, npcs, bodies, pc, ff


def guide_of(key):
    """A quest key's guide as EQOA-Data's map ties its points to one
    (questGuideIds): "1-20/freeport/warrior" of "1-20/freeport/warrior/L3",
    and of a later guide's "<folder>/<stem>/<part>" the folder in lower case,
    its commas hyphens ("30,40,49/group-4-evil/p1" is
    "30-40-49/group-4-evil")."""
    parts = key.split("/")
    if parts[0] == "1-20":
        return "/".join(parts[:3])
    return "/".join([parts[0].lower().replace(",", "-")] + parts[1:2])


def guides_of(data_dir):
    """Each name the quest files use (a giver, a step's target, an event's
    NPC, what is unresolved): the guides (1-20/<city>/<class>) it comes from.
    Every name, not the unresolved alone, so a map point is picked the same
    once the files list the name as found."""
    out = collections.defaultdict(set)
    for f in sorted(os.listdir(data_dir)):
        if not f.endswith(".json"):
            continue
        with open(os.path.join(data_dir, f)) as fh:
            quests = json.load(fh)["quests"]
        for q in quests:
            guide = guide_of(q["key"])
            names = [q.get("giver", "")] + [u.replace(" (attackable)", "") for u in q.get("unresolved", [])]
            for st in q.get("steps", []):
                names.append(st.get("target", ""))
                names += [ev.get("npc", "") for ev in st.get("events", [])]
            names += [ev.get("npc", "") for ev in q.get("events", [])]
            for n in names:
                if n:
                    out[key(n)].add(guide)
    return out


# ---- the plan ---------------------------------------------------------------------

def plan(db, ground_dir, data_dir, quest_dir, seed=DEFAULT_SEED, specs=None, attackable=None):
    """The rows a set would write: specs and attackable are its (each not
    given: the set 1-20's, SPECS and ATTACKABLE)."""
    specs = SPECS if specs is None else specs
    attackable = ATTACKABLE if attackable is None else attackable
    cols, npcs, bodies, pc, ff = load(db)
    with open(os.path.join(data_dir, "Map", "zones.json")) as f:
        zones = json.load(f)
    sheet = Map(zones, npcs, ff)
    guides = guides_of(quest_dir) if quest_dir and os.path.isdir(quest_dir) else {}
    rng = None   # each entry's own, below
    grounds, envs = {}, {}
    by_name = collections.defaultdict(list)
    for n in npcs:
        by_name[fold(n["name"])].append(n)
    seen = collections.defaultdict(list)
    for s in ff:
        if not s["pos_suspect"]:
            seen[key(s["npc_name"])].append(s)
    placed = []   # every new spawn: world, x, z (spacing)
    others = collections.defaultdict(list)   # the NPCs there are, by (world, 16-unit cell)
    for n in npcs:
        others[(n["world"], int(n["x"] // 16), int(n["z"] // 16))].append((n["x"], n["z"]))

    def crowded(world, x, z, r=CLEAR):
        cx, cz = int(x // 16), int(z // 16)
        return any(math.hypot(px - x, pz - z) < r
                   for dx in (-1, 0, 1) for dz in (-1, 0, 1) for px, pz in others.get((world, cx + dx, cz + dz), ()))
    rows, left, types = [], [], []

    def ground(world):
        if world not in grounds:
            path = os.path.join(ground_dir, f"{world}.ground")
            grounds[world] = Ground(path) if os.path.exists(path) else None
        return grounds[world]

    def wet(world, x, y, z):
        """Whether a body standing at y is in the water (or slime): half a
        unit above its feet, as the server has it; None without the file."""
        if world not in envs:
            path = os.path.join(ground_dir, f"{world}.env")
            envs[world] = Env(path) if os.path.exists(path) else None
        e = envs[world]
        return None if e is None else e.flags(x, y + 0.5, z) & 3 != 0

    def around_y(world, x, z, r=150.0):
        """The height of the NPC nearest (x, z) within r, one this plan
        placed too; None (the top floor) with none."""
        best, bd = None, r
        for n in npcs:
            if n["world"] == world and abs(n["x"] - x) < r and abs(n["z"] - z) < r:
                d = math.hypot(n["x"] - x, n["z"] - z)
                if d < bd:
                    best, bd = n["y"], d
        for q in rows:
            w = q["row"]
            if w["world"] == world:
                d = math.hypot(w["x_coord"] - x, w["z_coord"] - z)
                if d < bd:
                    best, bd = w["y_coord"], d
        return best

    def floor_at(world, x, z, ref, low):
        g = ground(world)
        if g is None:
            return None
        fl = g.floors(x, z)
        if not fl:
            return None
        if low:
            return fl[-1]
        if ref is None:
            return fl[0]
        return min(fl, key=lambda h: abs(h - ref))

    def walked(world, base, x, z):
        """The floor at (x, z) reached from base (x, z, floor) a step of 2 at a
        time, each no more than a stair (2.5) above or below the last; None if
        it cannot be."""
        g = ground(world)
        bx, bz, h = base
        n = max(1, math.ceil(math.hypot(x - bx, z - bz) / 2.0))
        for k in range(1, n + 1):
            fl = g.floors(bx + (x - bx) * k / n, bz + (z - bz) * k / n) if g else []
            if not fl:
                return None
            f = min(fl, key=lambda v: abs(v - h))
            if abs(f - h) > 2.5:
                return None
            h = f
        return h

    def template(body, world, x, z):
        kind = body[0]
        if kind == "npc":
            cands = [n for n in by_name.get(fold(body[1]), []) if n["model"]]
            return min(cands, key=lambda n: n["id"]) if cands else None
        if kind == "race":
            want = set(bodies.get((RACE[body[1]], body[2]), []))
            pool = [n for n in npcs if n["model"] in want]
        else:
            rx = re.compile(KIND[body[1]], re.I)
            pool = [n for n in npcs if n["model"] and n["model"] not in pc and not (n["type"] & 0x80)
                    and rx.search(n["name"])]
        if not pool:
            return None
        common = collections.Counter(n["model"] for n in pool).most_common(1)[0][0]
        pick = [n for n in pool if n["model"] == common]
        return min(pick, key=lambda n: (n["world"] != world, math.hypot(n["x"] - x, n["z"] - z), n["id"]))

    def anchor_of(spec):
        """Where the spec stands: world, x, z, the height it is near, a lower
        floor, the facing's target, where it came from; or why none."""
        pl = spec["place"]
        low = pl[-1] == "low"
        if pl[0] == "ff":
            ss = seen.get(key(spec["name"]), [])
            ss = [s for s in ss if s["world"] == 0]
            if not ss:
                return None, "no FrontiersForge sighting"
            return [(s["world"], s["x_coord"], s["z_coord"], s["y_coord"], low, s["npc_level"], None) for s in ss], "ff"
        if pl[0] == "poi":
            p = sheet.poi(pl[2], guides.get(key(spec["name"]), set()) | guides.get(key(pl[2]), set()))
            if p is None:
                return None, f"no map point {pl[2]!r}"
            m = re.search(r"\((\d+)", p["name"])
            lv = int(m.group(1)) if m else None
            if pl[1] == "rathe":
                x, z = sheet.rathe_at(p["x"], p["y"])
                return [(1, x, z, around_y(1, x, z), low, lv, None)], "map (the Rathe)"
            x, z = sheet.corrected(p["x"], p["y"])
            return [(0, x, z, around_y(0, x, z), low, lv, None)], "map"
        if pl[0] == "near":
            name, dist, world = pl[1], pl[2], pl[3]
            north = len(pl) > 4 and pl[4] == "north"
            cands = [n for n in by_name.get(fold(name), []) if n["world"] == world]
            cands += [dict(x=r["row"]["x_coord"], y=r["row"]["y_coord"], z=r["row"]["z_coord"])
                      for r in rows if fold(r["row"]["npc_name"]) == fold(name) and r["row"]["world"] == world]
            if not cands:
                return None, f"no {name!r} in world {world} to stand by"
            a = cands[0]
            # where the NPC stands, on its own floor (a talker by it walks there from it)
            af = floor_at(world, a["x"], a["z"], a["y"], False)
            base = (a["x"], a["z"], a["y"] if af is None else af)
            if north:   # smaller z is north
                return [(world, a["x"], a["z"] - dist, a["y"], low, None, base)], f"north of {name}"
            ang = rng.uniform(0, 2 * math.pi)
            return [(world, a["x"] + dist * math.cos(ang), a["z"] + dist * math.sin(ang), a["y"], low, None, base)], \
                f"by {name}"
        if pl[0] == "at":
            world, x, z = pl[1], pl[2], pl[3]
            return [(world, x, z, around_y(world, x, z), low, None, None)], "the guide's place"
        return None, f"place {pl[0]!r}"

    for spec in specs:
        # each entry draws from its own stream, so one entry's tries never move another's spawns
        rng = random.Random(f"{seed}/{spec['name']}/{spec['place']!r}")
        places, src = anchor_of(spec)
        if places is None:
            left.append((spec["name"], src))
            continue
        want = spec["count"]
        made = 0
        for i in range(want):
            world, ax, az, ay, low, lv, base = places[i % len(places)]
            first = i < len(places)
            soaked = False
            for t in range(TRIES):
                if first and t == 0:
                    x, z = ax, az
                else:
                    r = rng.uniform(*SPREAD) if not first else rng.uniform(3.0, 25.0)
                    a = rng.uniform(0, 2 * math.pi)
                    x, z = ax + r * math.cos(a), az + r * math.sin(a)
                if any(w == world and math.hypot(px - x, pz - z) < SPACING for w, px, pz in placed):
                    continue
                if crowded(world, x, z):
                    continue
                y = floor_at(world, x, z, ay, low)
                if first and src == "ff" and math.hypot(x - ax, z - az) <= DECK and y is not None \
                        and ay is not None and ay - y > 3.0 and spec["water"] == LAND and wet(world, x, ay, z) is False:
                    # seen standing on what the ground files lack (a boat's deck, the froglok
                    # raiders' 13 over the slime they stand above): there, out of the water
                    y = ay
                # a talker by a townsperson or where it was seen stands on that floor (not
                # off a dock or a storey up); anything else within 30 of its place's height
                tol = 10.0 if spec["role"] == TALKER and spec["place"][0] in ("near", "ff") else 30.0
                if y is None or (ay is not None and not low and abs(y - ay) > tol):
                    continue
                if spec["role"] == TALKER and base is not None:
                    # a talker by a townsperson stands where one could walk to from it on its
                    # floor (not the slope outside the room the other is in)
                    y = walked(world, base, x, z)
                    if y is None:
                        continue
                w = wet(world, x, y, z)
                if w is not None and spec["water"] != BOTH and w != (spec["water"] == WATER):
                    soaked = True
                    continue
                tpl = template(spec["body"], world, x, z)
                if tpl is None:
                    left.append((spec["name"], f"no body for {spec['body']}"))
                    break
                row = dict(tpl["row"])
                row.pop("id", None)
                level = spec["level"] or lv or row.get("npc_level") or 1
                if src == "ff":
                    level = lv or level
                row.update(npc_name=spec["name"], world=world, x_coord=round(x, 3), y_coord=round(y, 4),
                           z_coord=round(z, 3), facing=rng.randrange(256), npc_level=int(level),
                           npc_type=TYPE[spec["role"]], npc_id=0, dt_kill=None, zone=0)
                rows.append(dict(row=row, name=spec["name"], source=src, template=tpl.get("id"), note=spec["note"]))
                placed.append((world, x, z))
                made += 1
                break
            else:
                what = ("no floor in the water" if spec["water"] == WATER else "no dry floor") if soaked else "no floor"
                left.append((spec["name"], f"{what} round ({ax:.0f}, {az:.0f}) in world {world}"))
        if made and made < want:
            left.append((spec["name"], f"{want - made} of {want} spawns found no place"))
    for name in attackable:
        for n in by_name.get(fold(name), []):
            if n["type"] & 0x80:
                types.append(dict(id=n["id"], name=n["name"], old=n["type"], new=n["type"] & ~0x80))
    return dict(cols=cols, rows=rows, left=left, types=types, fit=(sheet.tunaria, sheet.fit_kept, sheet.fit_median),
                misses=len(sheet.misses), oggok=sheet.oggok)


# ---- the sets: the built-in one and the spec files ---------------------------------

PLACES = ("ff", "poi", "near", "at")


def spec_of(d, where):
    """One of a spec file's npcs as S() makes one (its body and place lists made
    tuples), and what is wrong with it (nothing: [])."""
    errs = []
    if not isinstance(d, dict):
        return None, [f"{where}: not an object"]
    name = d.get("name")
    if not isinstance(name, str) or not name.strip():
        errs.append(f"{where}: no name")
        name = "?"
    where = f"{where} {name!r}"
    role = d.get("role")
    if role not in TYPE:
        errs.append(f"{where}: role {role!r} (foe or talker)")
    body = d.get("body")
    ok = isinstance(body, list) and body and (
        (body[0] == "race" and len(body) == 3 and body[1] in RACE and body[2] in (MALE, FEMALE)) or
        (body[0] == "kind" and len(body) == 2 and body[1] in KIND) or
        (body[0] == "npc" and len(body) == 2 and isinstance(body[1], str) and body[1].strip()))
    if not ok:
        errs.append(f"{where}: body {body!r} (['race', <{'|'.join(RACE)}>, 0|1], ['kind', <a KIND key>] or ['npc', <name>])")
    place = d.get("place")
    pl = list(place) if isinstance(place, list) else []
    low = bool(pl) and pl[-1] == "low"
    core = pl[:-1] if low else pl
    num = (int, float)
    ok = bool(core) and core[0] in PLACES and (
        (core[0] == "ff" and len(core) == 1) or
        (core[0] == "poi" and len(core) == 3 and core[1] in ("tunaria", "rathe") and isinstance(core[2], str)) or
        (core[0] == "near" and len(core) in (4, 5) and isinstance(core[1], str) and isinstance(core[2], num)
         and isinstance(core[3], int) and (len(core) == 4 or core[4] == "north")) or
        (core[0] == "at" and len(core) == 4 and isinstance(core[1], int) and isinstance(core[2], num) and isinstance(core[3], num)))
    if not ok:
        errs.append(f"{where}: place {place!r} (['ff'], ['poi', 'tunaria'|'rathe', <point>], ['near', <npc>, <distance>, "
                    "<world>], ['at', <world>, <x>, <z>], each with a last 'low' or not)")
    count = d.get("count", 1)
    if not isinstance(count, int) or isinstance(count, bool) or count < 1:
        errs.append(f"{where}: count {count!r}")
    level = d.get("level")
    if level is not None and (not isinstance(level, int) or isinstance(level, bool) or not 1 <= level <= 100):
        errs.append(f"{where}: level {level!r}")
    water = d.get("water", LAND)
    if water not in (LAND, WATER, BOTH):
        errs.append(f"{where}: water {water!r} (land, water or both)")
    note = d.get("note", "")
    if not isinstance(note, str):
        errs.append(f"{where}: note is not text")
    if errs:
        return None, errs
    return S(name.strip(), role, tuple(body), tuple(pl), count=count, level=level, note=note, water=water), []


def load_sets(paths):
    """The sets: the built-in one ("1-20": SPECS, ATTACKABLE) and each spec
    file's ({set: dict(specs, attackable, file)} in that order), and what is
    wrong with the files."""
    sets = {BUILTIN: dict(specs=SPECS, attackable=ATTACKABLE, file="(built in)")}
    errs = []
    for path in paths:
        try:
            with open(path) as f:
                d = json.load(f)
        except (OSError, ValueError) as e:
            errs.append(f"{path}: {e}")
            continue
        name = d.get("set") if isinstance(d, dict) else None
        if not isinstance(name, str) or not name.strip() or any(ch.isspace() for ch in name.strip()):
            errs.append(f"{path}: no set name (\"set\": a word)")
            continue
        name = name.strip()
        if name in sets:
            errs.append(f"{path}: set {name!r} is {sets[name]['file']}'s already")
            continue
        specs = []
        for i, e in enumerate(d.get("npcs") or [], 1):
            sp, es = spec_of(e, f"{os.path.basename(path)} npc {i}")
            errs += es
            if sp:
                specs.append(sp)
        att = d.get("attackable") or []
        if not isinstance(att, list) or not all(isinstance(a, str) and a.strip() for a in att):
            errs.append(f"{path}: attackable is not a list of names")
            att = []
        sets[name] = dict(specs=specs, attackable=[a.strip() for a in att], file=path)
    return sets, errs


# ---- writing it, and taking it back out ---------------------------------------------

# seedset last: a log from before the sets gains it at the end (migrate)
LOG_DDL = [
    "CREATE TABLE IF NOT EXISTS quest_npc_seed_meta (seedset TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, "
    "PRIMARY KEY (seedset, key))",
    "CREATE TABLE IF NOT EXISTS quest_npc_seed_rows (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, "
    f"source TEXT NOT NULL, template_row INTEGER, seedset TEXT NOT NULL DEFAULT '{BUILTIN}')",
    "CREATE TABLE IF NOT EXISTS quest_npc_seed_types (npc_row INTEGER PRIMARY KEY, old_type INTEGER NOT NULL, "
    f"new_type INTEGER NOT NULL, seedset TEXT NOT NULL DEFAULT '{BUILTIN}')",
]
LOG_TABLES = ("quest_npc_seed_rows", "quest_npc_seed_types", "quest_npc_seed_meta")


def seeded(db):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'quest_npc_seed_rows'").fetchone()[0] > 0


def log_columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def sets_in(db):
    """The sets in this database, each with its rows (a log from before the
    sets is the set 1-20's, whatever its shape)."""
    if not seeded(db):
        return {}
    if "seedset" not in log_columns(db, "quest_npc_seed_meta"):
        return {BUILTIN: db.execute("SELECT count(*) FROM quest_npc_seed_rows").fetchone()[0]}
    out = {s: 0 for (s,) in db.execute("SELECT DISTINCT seedset FROM quest_npc_seed_meta ORDER BY seedset")}
    for s, n in db.execute("SELECT seedset, count(*) FROM quest_npc_seed_rows GROUP BY seedset"):
        out[s] = n
    return out


def migrate(db):
    """A log made before the sets made the set 1-20's, in place: seedset
    added to its rows and types, its meta keyed by set (in the caller's
    transaction)."""
    if not seeded(db):
        return False
    done = False
    for t in ("quest_npc_seed_rows", "quest_npc_seed_types"):
        if "seedset" not in log_columns(db, t):
            db.execute(f"ALTER TABLE {t} ADD COLUMN seedset TEXT NOT NULL DEFAULT '{BUILTIN}'")
            done = True
    if "seedset" not in log_columns(db, "quest_npc_seed_meta"):
        db.execute("CREATE TABLE quest_npc_seed_meta_sets (seedset TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, "
                   "PRIMARY KEY (seedset, key))")
        db.execute("INSERT INTO quest_npc_seed_meta_sets SELECT ?, key, value FROM quest_npc_seed_meta", (BUILTIN,))
        db.execute("DROP TABLE quest_npc_seed_meta")
        db.execute("ALTER TABLE quest_npc_seed_meta_sets RENAME TO quest_npc_seed_meta")
        done = True
    return done


def npc_seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return None if r is None else r[0]


def gm_edited(db, ids):
    """npcs rows a GM changed (gm_audit), of these ids."""
    if not ids or not db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'gm_audit'").fetchone()[0]:
        return set()
    cols = {r[1] for r in db.execute("PRAGMA table_info(gm_audit)")}
    if not {"tbl", "row_key"} <= cols:
        return set()
    out = set()
    for (rk,) in db.execute("SELECT row_key FROM gm_audit WHERE tbl = 'npcs'"):
        try:
            v = json.loads(rk)
            v = v.get("id") if isinstance(v, dict) else v
            out.add(int(v))
        except (ValueError, TypeError):
            pass
    return out & set(ids)


def apply(db, p, seedset=BUILTIN):
    """Write a set's plan, logged under its name (refused while it is in)."""
    cols = [c for c in p["cols"] if c != "id"]
    db.execute("BEGIN IMMEDIATE")
    try:
        migrate(db)
        if seedset in sets_in(db):
            raise SystemExit(f"the set {seedset!r} is in this database already: --undo --set {seedset} first")
        for stmt in LOG_DDL:
            db.execute(stmt)
        meta = dict(when=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                    seq_before="" if npc_seq(db) is None else str(npc_seq(db)), rows=str(len(p["rows"])))
        for r in p["rows"]:
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r["row"].get(c) for c in cols])
            db.execute("INSERT INTO quest_npc_seed_rows (npc_row, name, source, template_row, seedset) VALUES (?, ?, ?, ?, ?)",
                       (cur.lastrowid, r["name"], r["source"], r["template"], seedset))
        skip = gm_edited(db, [t["id"] for t in p["types"]])
        for t in p["types"]:
            if t["id"] in skip:
                continue
            db.execute("UPDATE npcs SET npc_type = ? WHERE id = ?", (t["new"], t["id"]))
            db.execute("INSERT INTO quest_npc_seed_types (npc_row, old_type, new_type, seedset) VALUES (?, ?, ?, ?)",
                       (t["id"], t["old"], t["new"], seedset))
        db.executemany("INSERT INTO quest_npc_seed_meta (seedset, key, value) VALUES (?, ?, ?)",
                       [(seedset, k, v) for k, v in sorted(meta.items())])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, seedset=BUILTIN):
    """Take a set back out: its rows deleted, its types put back (a row a GM
    changed since is left), the npcs counter put back when nothing past it is
    left, its log rows gone (the logs too, once no set is in)."""
    db.execute("BEGIN IMMEDIATE")
    try:
        migrate(db)
        if seedset not in sets_in(db):
            raise SystemExit(f"the set {seedset!r} is not in this database")
        meta = dict(db.execute("SELECT key, value FROM quest_npc_seed_meta WHERE seedset = ?", (seedset,)))
        n = db.execute("DELETE FROM npcs WHERE id IN (SELECT npc_row FROM quest_npc_seed_rows WHERE seedset = ?)",
                       (seedset,)).rowcount
        kept = []
        for rid, old, new in db.execute("SELECT npc_row, old_type, new_type FROM quest_npc_seed_types WHERE seedset = ?",
                                        (seedset,)).fetchall():
            cur = db.execute("SELECT npc_type FROM npcs WHERE id = ?", (rid,)).fetchone()
            if cur is None or cur[0] != new:
                kept.append(rid)
                continue
            db.execute("UPDATE npcs SET npc_type = ? WHERE id = ?", (old, rid))
        before = meta.get("seq_before", "")
        top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
        if before == "":
            if top == 0:
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
        elif top <= int(before):
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (int(before),))
        # a set applied after this one now counts from this one's counter
        # when no row holds an id between the two: so whichever order the
        # sets go back out in, the last one out puts the counter where it was
        low = int(before or 0)
        for other, b in db.execute("SELECT seedset, value FROM quest_npc_seed_meta WHERE key = 'seq_before' AND seedset != ?",
                                   (seedset,)).fetchall():
            if int(b or 0) > low and not db.execute("SELECT count(*) FROM npcs WHERE id > ? AND id <= ?",
                                                    (low, int(b or 0))).fetchone()[0]:
                db.execute("UPDATE quest_npc_seed_meta SET value = ? WHERE seedset = ? AND key = 'seq_before'", (before, other))
        for t in LOG_TABLES:
            db.execute(f"DELETE FROM {t} WHERE seedset = ?", (seedset,))
        if not db.execute("SELECT count(*) FROM quest_npc_seed_meta").fetchone()[0]:
            for t in LOG_TABLES:
                db.execute(f"DROP TABLE {t}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n, kept


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-questnpcs"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(p, verbose=False):
    (a, b, c, d), kept, med = p["fit"]
    print(f"Tunaria: x = {a:.5f} mx {b:+.1f}, z = {c:.5f} my {d:+.1f} ({kept} points, a median {med:.1f} off); "
          f"{p['misses']} points FrontiersForge also saw move what is near them")
    if p["oggok"]:
        print(f"the Rathe: anchored on Oggok's people at ({p['oggok'][0]:.0f}, {p['oggok'][1]:.0f}), {SCALE} a map unit")
    by = collections.defaultdict(list)
    for r in p["rows"]:
        by[r["name"]].append(r)
    for name, rs in by.items():
        r = rs[0]["row"]
        print(f"  {name!r}: {len(rs)} ({rs[0]['source']}) L{r['npc_level']} type {r['npc_type']} world {r['world']} "
              f"at ({r['x_coord']:.0f}, {r['y_coord']:.1f}, {r['z_coord']:.0f}) body of row {rs[0]['template']}")
        if verbose:
            for x in rs[1:]:
                print(f"      also ({x['row']['x_coord']:.0f}, {x['row']['y_coord']:.1f}, {x['row']['z_coord']:.0f})")
    for t in p["types"]:
        print(f"  {t['name']!r} (row {t['id']}): npc_type {t['old']} -> {t['new']} (attackable)")
    if p["left"]:
        print("left out:")
        for name, why in p["left"]:
            print(f"    {name!r}: {why}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    ap.add_argument("--data", default=DEFAULT_DATA, help="a checkout of EQOA-Data (its Map/zones.json)")
    ap.add_argument("--quests", default=os.path.join(HERE, "quests", "data"), help="the quest files (their guides)")
    ap.add_argument("--specs", nargs="+", metavar="FILE",
                    help="the spec files, a set each (default: every tools/quests/npcs/*.json)")
    ap.add_argument("--set", action="append", default=[], metavar="SET", dest="sets",
                    help=f"a set to plan, apply or undo ({BUILTIN} the built-in one, a spec file's \"set\", or all); "
                         "a report without one plans every set")
    ap.add_argument("--apply", action="store_true", help="write the sets named")
    ap.add_argument("--undo", action="store_true", help="delete every NPC the sets named wrote and put the types back")
    ap.add_argument("--seed", type=int, default=DEFAULT_SEED, help="the placement's random seed")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--verbose", action="store_true", help="every spawn")
    args = ap.parse_args()
    paths = args.specs if args.specs is not None else sorted(
        os.path.join(DEFAULT_SPECS, f) for f in (os.listdir(DEFAULT_SPECS) if os.path.isdir(DEFAULT_SPECS) else [])
        if f.endswith(".json"))
    sets, errs = load_sets(paths)
    if errs:
        for e in errs:
            print("ERROR:", e)
        sys.exit(f"{len(errs)} errors in the spec files: nothing planned")
    writing = args.apply or args.undo
    target = os.path.abspath(args.db) if writing else f"file:{os.path.abspath(args.db)}?mode=ro"
    db = sqlite3.connect(target, isolation_level=None, uri=True)
    db.execute("PRAGMA busy_timeout = 5000")
    have = sets_in(db)
    names = list(sets) if "all" in args.sets else args.sets
    for n in names:
        if n not in sets and not (args.undo and n in have):
            sys.exit(f"no set {n!r} (the sets: {', '.join(sets)})")
    if args.undo:
        if not names:
            if len(have) != 1:
                sys.exit(f"name the set to undo (--set X): in this database {', '.join(have) or 'none'}")
            names = list(have)
        names = [n for n in names if n in have] if "all" in args.sets else names
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        for n in reversed(names):   # the last applied first, so each counter goes back
            got, kept = undo(db, n)
            print(f"set {n}: deleted {got} seeded NPCs")
            for rid in kept:
                print(f"  row {rid}'s npc_type was changed since: left as it is")
        print("restart the server")
        return
    if args.apply and not names:
        sys.exit(f"name the sets to apply: --set {' --set '.join(sets)} (or --set all)")
    todo = names or list(sets)
    if args.apply and not args.no_backup:
        print("backup:", backup(db, args.db))
    for n in todo:
        print(f"== set {n} ({sets[n]['file']}): {len(sets[n]['specs'])} names")
        if n in sets_in(db):
            print(f"   in this database already ({sets_in(db)[n]} NPCs): --undo --set {n} to plan it again")
            continue
        p = plan(db, args.ground, args.data, args.quests, seed=args.seed,
                 specs=sets[n]["specs"], attackable=sets[n]["attackable"])
        report(p, args.verbose)
        if not args.apply:
            print(f"{len(p['rows'])} spawns, {len(p['types'])} made attackable (report only: --apply --set {n} writes them)")
            continue
        apply(db, p, n)
        print(f"wrote {len(p['rows'])} spawns, made {len(p['types'])} attackable")
    if args.apply:
        print("restart the server to read them")


if __name__ == "__main__":
    main()
