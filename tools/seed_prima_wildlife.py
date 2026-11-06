#!/usr/bin/env python3
"""The animals of Prima's bestiary this server lacks, or has only a few of,
seeded where the guide's tables put them.

The user, 2026-11-06: check the Prima strategy guides' wildlife against the
live server (spawns, adequate numbers, roaming). Only the Frontiers guide is
at hand (Prima's Official Strategy Guide, EverQuest Online Adventures:
Frontiers, the Bestiary, printed pages 653-672); its tables are by biome, not
by zone: Tunaria's Arctic Beasts, Beach, Desert, Forest, Fresh Water, Global,
Grassland, Jungle, Mountain, Ocean, Savannah, Swamp and Volcanic, then the
Frontiers' Deep Jungle, Marine, Rathe Mountains, Red Desert, Savannah,
Wasteland and Wetlands. The audit of that day found about a quarter of its
animals with no row here and a few more with one to four. cmd/wildspread
fills the land's gaps with Prima's animals too, but only by copying a
namesake row (its Rathe-only made kinds aside), so an animal with no row
never comes; this tool makes them.

What it decides, and from what:

- Which: every row of the bestiary (--bestiary, the operator's own parse of
  the guide: below) that is an animal (seed_npc_move.wildlife_word, not
  undead, or one of READ_ANIMALS, the beasts wildlife.go's words miss), whose
  name, by its letters alone or as ALIASES spells it here, has fewer than
  THIN rows in all worlds. A missing one gets --count spawns, a thin one as
  many more as make --count.
- Where: the guide's table names the land. A Tunaria table of a biome
  (BIOME: Grassland and Savannah are EQOA-Data's Plains, Beach and Ocean its
  Marine) takes the quarters of EQOA-Data's zones (Map/zones.json: each zone
  and town's four quarters' biomes) of that biome, each zone laid on the
  2000-unit cell its point falls in by seed_npc_move.py's fit (cell =
  round((x - 5000) / 2000); Isle of Dread, which the map draws a cell off,
  at 10,-1). Global takes any of those cells. A table of no biome (Jungle,
  Fresh Water, Volcanic, and every Frontiers table) takes the cells where
  its animals this server has spawn (its kin), and spawns within KIN_REACH of
  them: Tunaria's tables in Tunaria and the Rathe, the Frontiers' in the
  Rathe and Odus; Volcanic's are its fire bats and the lava (the
  environment's lava flag on the top floor, sought LAVA_STEP apart), a water
  table's whose animals stand in fewer than MIN_KIN_CELLS cells (the
  Frontiers' Marine) its worlds' sea creatures too (seed_npc_move's WATER
  words). Beach and Ocean take their
  kin's cells too.
- At what levels: a place is ranked by its creatures (no NPC bit, no
  players' race body) of a level within LEVEL_SLACK of the guide's range,
  at least MIN_MATCH of them, the best UNIT_CHOICE drawn from at random
  (fixed seed), PER_UNIT spawns each, one cell a species. A spawn stands
  where the creatures within LEVEL_RADIUS of it are, by their median, within
  twice LEVEL_SLACK of the range (a water spawn with none round it takes the
  place's), and its level is that median, one either way, inside the range,
  else the half of the range nearer it (none round: any of the range).
- On what: the terrain (the operator's ground files, cmd/groundbake: the top
  floor, level enough to step off: each neighbour STEP away within 2.5), in
  the water for a fish (the world's environment file, internal/ground/env.go:
  water at half a unit and WATER_DEPTH over the floor, the sea floor where
  the server keeps a creature that spawned there), out of water, slime, lava
  and mud for a land animal, either for one that goes both ways
  (seed_npc_move's BOTH words: crabs, alligators, turtles, tortoises);
  TOWN_CLEAR from any town's people (seed_wildlife.town_people, as
  entity/towns.go finds them), SPACING from every NPC, and APART from its
  own species' other seeds.
- The body: a missing animal's is the nearest kind of creature this server
  has (body_for): a generic creature (seed_wildlife.creature_kinds: two rows
  or more, no players' race body, no NPC bit, no pet's name) whose head noun
  is of the animal's FAMILY ("frost wolf" a wolf's or a worg's,
  "brazercampus" the twisted hippocampus's, "darkwater skate" a ray's),
  scored by the same head, shared words, young or grown alike, being a kin
  of the table, its levels and its worlds, an undead one only on a shared
  word (the icebone dog an icebone worg's); the row copied is
  seed_wildlife.template's (of
  the kind's most common body, the nearest). An animal whose family this
  server has no creature of (the striders, the chimeras) is left out and
  listed. A thin animal copies its own rows.
- Its row: every column of the body's row but the name (Prima's, lower case,
  as the game writes a creature that is no one in particular; a thin one's as
  its rows spell it), place, facing, world, level, npc_id 0 (no loot list of
  its own) and dt_kill. A made name takes its body's rows in the tables a
  name keys (MADE_TABLES: its class, class template, loot region, loot pool
  and undead tag, as cmd/wildspread's made kinds do), its loot region the
  biome of the quarter most of its spawns stand in (Tunaria) or its world's
  (the Rathe's "rathe mountains", Odus's "odus"), and an npcmove row when
  the server's rules would not move it as an animal: roam 1 for a name
  wildlife.go's words miss (a hydra, a wurm) on a body that is not undead,
  water "both" for one of BOTH's words.

Ours, not measured: THIN, the count, the units' sizes and choice, every
distance, the level slack, the body's choice where no row of the name exists,
ALIASES' readings of the near spellings, and the loot region.

Every row is written in one transaction and logged: the npcs rows in
prima_wildlife_seed_npcs (the species, its table, the unit, missing or thin,
the row copied), the name-keyed rows in prima_wildlife_seed_rows (table,
rowid). --undo deletes exactly those, puts each AUTOINCREMENT counter back
when nothing was added after, and drops the logs, so every other row is as it
was; a second --apply is refused while a seed is in. The same database,
bestiary, map and ground give the same plan (a fixed random seed). Report
only by default. The server reads npcs and the name-keyed tables at its
start: apply with the server stopped (a swap's before.sh), or restart after.

The bestiary file is the guide's tables as JSON, made from the operator's own
copy and never kept here: a list (or {"species": [...]}) of rows with the
name, the table ("region" or "section"), the levels ("lo" and "hi", or
"prima_levels") and whether it is a Frontiers table ("frontiers"; Savannah is
both). The audit's prima_tagged.json and analysis.json are of that shape.

    tools/seed_prima_wildlife.py --bestiary prima_tagged.json --ground worlds/ground
        [--db eqoa.db] [--eqoa-data ../external/EQOA-Data] [--apply | --undo]
        [--count N] [--only "name,..."] [--skip "name,..."] [--seed N] [--verbose]
"""
import argparse
import collections
import datetime
import json
import math
import os
import random
import re
import sqlite3
import statistics
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_npc_move as mv  # noqa: E402
import seed_wildlife as sw  # noqa: E402
from seed_pod import Env  # noqa: E402

DEFAULT_DB = sw.DEFAULT_DB
DEFAULT_GROUND = sw.DEFAULT_GROUND
DEFAULT_DATA = os.path.join(HERE, "..", "..", "external", "EQOA-Data")
DEFAULT_SEED = 41
SOURCE = "prima-wildlife"

THIN = 5               # an animal with fewer rows than this is seeded...
COUNT = 9              # ...up to this many in all
PER_UNIT = 3           # spawns a place takes
UNIT_CHOICE = 6        # the best places a species' are drawn from
MIN_MATCH = 3          # creatures of its levels a place needs
LEVEL_SLACK = 2
LEVEL_RADIUS = 350.0
KIN_REACH = 500.0      # how far from its table's kin a spawn of a table of no biome stands
TOWN_CLEAR = sw.TOWN_CLEAR
SPACING = 25.0         # from any NPC (cmd/wildspread's npcClear)
APART = 120.0          # from its own species' other seeds
STEP = 2.0             # the ground round a spawn: a step each way within MAX_STEP
MAX_STEP = 2.5
WATER_DEPTH = 3.0
INSET = 15.0
TRIES = 300
CELL = 2000.0
ISLE_OF_DREAD = (10, -1)

TUNARIA, FRONTIERS = "Tunaria", "Frontiers"
# a Tunaria table's biome on EQOA-Data's map (cmd/wildspread's biomeSections)
BIOME = {"Arctic Beasts": "Arctic", "Desert": "Desert", "Forest": "Forest", "Grassland": "Plains",
         "Savannah": "Plains", "Mountain": "Mountain", "Swamp": "Swamp", "Beach": "Marine", "Ocean": "Marine"}
KIN_TOO = {"Beach", "Ocean"}       # their biome's quarters and their kin's cells
GLOBAL = "Global"                  # any zone of Tunaria
TABLES = {TUNARIA: ["Arctic Beasts", "Beach", "Desert", "Forest", "Fresh Water", "Global", "Grassland", "Jungle",
                    "Mountain", "Ocean", "Savannah", "Swamp", "Volcanic"],
          FRONTIERS: ["Deep Jungle", "Marine", "Rathe Mountains", "Red Desert", "Savannah", "Wasteland", "Wetlands"]}
KIN_WORLDS = {TUNARIA: (0, 1), FRONTIERS: (1, 2)}
WATER_TABLES = {"Ocean", "Fresh Water", "Marine"}
MIN_KIN_CELLS = 4      # a water table with its animals in fewer cells takes its worlds' sea creatures as kin
VOLCANIC = "Volcanic"  # kin: its fire bats and the lava (the environment's flag 4), sought LAVA_STEP apart
LAVA_STEP = 100.0
WORLD_REGION = {1: "rathe mountains", 2: "odus"}
LAND, WATER, BOTH = "land", "water", "both"
BAD_ENV, WET, LAVA, SOAKED = 1 | 2 | 4 | 8, 1, 4, 2 | 4 | 8   # water, slime, lava, mud (internal/ground/env.go)

# The guide's animals wildlife.go's words miss (the audit's reading of its
# tables: beasts, fish and insects; not people, undead or spirits).
READ_ANIMALS = {
    "ancient chimera", "chimera", "arachnalith", "arclight hydra", "dune hydra", "fetid hydra", "mountain hydra",
    "plague hydra", "whitebolt hydra", "hydra", "blood gulper", "bone gnawer", "brazercampus", "whitemanecampus",
    "darkwater skate", "firepoison skate", "freshwater skate", "dangerous mantaray", "lethal mantaray",
    "deadly manta", "deepmare", "hippocampus", "dire timberwolf", "rabid timberwolf", "frightwolf", "dreadworg",
    "flamecrawler", "giant arachnid", "spitting arachnid", "giant araneae", "huge araneae", "insect swarm",
    "jungle stalker", "midnight strider", "dark strider", "sharpfanged strider", "putrid wurm", "steelfang wyrm",
    "wyrm", "sand midges", "sandshark", "spineback", "sunbeetle", "ursine", "widowmaker", "savannah matriarch",
    "savannah patriarch",
}

# The guide's names this server spells otherwise (read, 2026-11-06: the same
# words, or one changed, at nearby levels): the guide's -> the server's.
ALIASES = {
    "frozen fang spider": "freezefang spider",
    "glacier bear": "glacial bear",
    "shock eel": "shocking eel",
    "widow queen": "queen widow",
    "plague hydra": "plagued hydra",
}

# The kinds of animal whose bodies may stand in for one another, by head noun.
FAMILIES = {
    "canine": "wolf worg warg dog hound jackal",
    "feline": "lion lioness panther jaguar leopard tiger cat puma cougar",
    "bear": "bear ursine grizzly",
    "boar": "boar hog warthog sawtusk razorback",
    "bat": "bat",
    "bird": "owl crow raven rook magpie eagle hawk vulture buzzard",
    "griffon": "griffon griffin gryphon griffenne griffawn",
    "drake": "drake",
    "spider": "spider spiderling widow recluse tarantula arachnid araneae arachnalith",
    "beetle": "beetle scarab",
    "fly": "fly gnat midge mosquito",
    "wasp": "wasp hornet",
    "ant": "ant",
    "worm": "worm burrower",
    "crawler": "crawler millipede centipede",
    "snake": "snake viper python anaconda cobra copperhead kingsnake rattler serpent moccasin",
    "basilisk": "basilisk",
    "hydra": "hydra",
    "rhino": "rhinoceros rhino",
    "elephant": "elephant mammoth",
    "shell": "tortoise turtle",
    "croc": "alligator crocodile caiman gharial",
    "crab": "crab",
    "eel": "eel gulper",
    "fish": "fish pike piranha barracuda",
    "ray": "ray manta mantaray skate",
    "shark": "shark hammerhead",
    "squid": "squid kraken octopus",
    "angler": "angler",
    "seahorse": "hippocampus campus",
    "wyrm": "wyrm wurm",
    "manticore": "manticore",
    "ooze": "ooze goo slime jelly",
}
FAMILY = {w: f for f, ws in FAMILIES.items() for w in ws.split()}
# heads that are animals though wildlife.go's words miss them
WORDLESS = {"hydra", "seahorse", "wyrm", "ooze", "manticore"}
YOUNG = {"young", "cub", "pup", "kit", "calf", "chick", "small", "weak", "baby", "whelp"}
# words of size or temper, which say less of a body's look than "icebone" or "polar"
GENERIC = {"giant", "huge", "large", "great", "greater", "big", "ancient", "elder", "lesser", "fierce", "deadly",
           "dangerous", "lethal", "strong", "rabid", "raging", "wild", "dire", "savage", "mature"}


# ---- names --------------------------------------------------------------------------

def letters(name):
    return re.sub(r"[^a-z]", "", (name or "").lower())


def key(name):
    """A name as database.NPCClassKey keys it: lower case, trimmed, no '$'."""
    return mv.key(name or "")


def unkern(name):
    """The guide's extraction splits some words: "Yo u ng", "T ortoise"."""
    name = re.sub(r"\bYo u ng\b", "Young", name)
    return re.sub(r"\bT\s+([a-z])", r"T\1", name).strip()


def head(name):
    """(head noun, family, the other words) of a name: the young marks (a cub,
    a small one) dropped off its end and counted as "young", a compound split
    on the family word it ends in ("snowbat": bat and snow). Family None when
    the head is of none."""
    ws = re.findall(r"[a-z]+", name.lower())
    mods = set()
    while len(ws) > 1 and ws[-1] in YOUNG:
        mods.add("young")
        ws = ws[:-1]
    if not ws:
        return "", None, mods
    *rest, last = ws
    mods |= {"young" if w in YOUNG else w for w in rest}
    for s in mv.singulars(last):
        if s in FAMILY:
            return s, FAMILY[s], mods
    if last not in mv.NOT_ANIMALS and last not in mv.PEOPLE:
        for s in mv.singulars(last):
            for w in sorted(FAMILY, key=len, reverse=True):
                if len(w) >= 3 and len(s) > len(w) + 2 and s.endswith(w):
                    return w, FAMILY[w], mods | {s[: -len(w)]}
    return last, None, mods


def is_animal(name):
    k = key(name)
    if k in READ_ANIMALS:
        return True
    return bool(mv.wildlife_word(name)) and not mv.UNDEAD.search(k)


def water_of(name, table):
    k = key(name)
    if mv.BOTH.search(k):
        return BOTH
    if mv.WATER.search(k) or table in WATER_TABLES:
        return WATER
    return LAND


# ---- the bestiary -------------------------------------------------------------------

def read_bestiary(path):
    """The guide's animals: name -> its table entries (part, table, lo, hi,
    page), in the guide's order."""
    with open(path, encoding="utf-8") as f:
        doc = json.load(f)
    rows = doc["species"] if isinstance(doc, dict) else doc
    out = collections.OrderedDict()
    for r in rows:
        name = unkern(r["name"])
        table = (r.get("section") or r.get("region") or "").replace(" (Frontiers table)", "").strip()
        lv = r.get("prima_levels") or [r.get("lo"), r.get("hi")]
        lo, hi = lv[0], lv[1] if lv[1] is not None else lv[0]
        part = FRONTIERS if r.get("frontiers") or (table in TABLES[FRONTIERS] and table not in TABLES[TUNARIA]) \
            else TUNARIA
        if table not in TABLES[part] or lo is None or not is_animal(name):
            continue
        e = dict(part=part, table=table, lo=int(lo), hi=int(hi or lo), page=r.get("pdf_page"))
        entries = out.setdefault(key(name), dict(name=name, entries=[]))["entries"]
        if e not in entries:
            entries.append(e)
    return out


# ---- the database -------------------------------------------------------------------

def table_exists(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def load(db):
    db.row_factory = sqlite3.Row
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    npcs = []
    for r in db.execute("SELECT * FROM npcs ORDER BY id"):
        d = dict(r)
        npcs.append(dict(row=d, id=d["id"], name=d["npc_name"] or "", key=key(d["npc_name"]),
                         letters=letters(d["npc_name"]), world=d["world"] or 0, x=d["x_coord"] or 0.0,
                         y=d["y_coord"] or 0.0, z=d["z_coord"] or 0.0, model=(d["modelid"] or 0) & 0xFFFFFFFF,
                         level=d["npc_level"] or 1, type=d["npc_type"] or 0))
    db.row_factory = None
    pc = {r[0] & 0xFFFFFFFF for r in db.execute("SELECT modelid FROM charactermodel")}
    spells = {key(r[0]) for r in db.execute("SELECT spellname FROM spellpattern")} \
        if table_exists(db, "spellpattern") else set()
    undead = {key(n): bool(u) for n, u in db.execute("SELECT name, undead FROM npcundead")} \
        if table_exists(db, "npcundead") else {}
    moves = {key(n) for (n,) in db.execute("SELECT name FROM npcmove")} if table_exists(db, "npcmove") else set()
    return cols, npcs, pc, spells, undead, moves


def creature(n, pc):
    return n["model"] and n["model"] not in pc and not (n["type"] & 0x80)


# ---- the map ------------------------------------------------------------------------

def cell_of(x, z):
    return round((x - 5000) / CELL), round((z - 5000) / CELL)


def cell_box(c):
    x0, z0 = 4000 + CELL * c[0], 4000 + CELL * c[1]
    return x0, x0 + CELL, z0, z0 + CELL


def read_zones(data_dir):
    """EQOA-Data's zones and towns on Tunaria's cells: cell -> (name,
    biomes NW NE SW SE)."""
    with open(os.path.join(data_dir, "Map", "zones.json"), encoding="utf-8") as f:
        doc = json.load(f)
    out = {}
    for z in doc.get("zones", []) + doc.get("towns", []):
        b = z.get("biomes") or []
        if len(b) != 4:
            continue
        c = cell_of(mv.AX * z["x"] + mv.BX, mv.AZ * z["y"] + mv.BZ)
        if z["name"] == "Isle of Dread":
            c = ISLE_OF_DREAD
        out.setdefault(c, (z["name"], list(b)))
    return out


def quarter_box(c, q):
    x0, _, z0, _ = cell_box(c)
    x0 += (q % 2) * CELL / 2
    z0 += (q // 2) * CELL / 2
    return x0, x0 + CELL / 2, z0, z0 + CELL / 2


def biome_at(zones, world, x, z):
    if world != 0:
        return None
    c = cell_of(x, z)
    if c not in zones:
        return None
    x0, _, z0, _ = cell_box(c)
    q = (x - x0 >= CELL / 2) + 2 * (z - z0 >= CELL / 2)
    return zones[c][1][q]


# ---- bodies -------------------------------------------------------------------------

def body_kinds(npcs, pc, spells, undead):
    """The kinds a made animal's body may come from: seed_wildlife's generic
    creatures whose head is of a family (and an animal's by wildlife.go's
    words, or of a WORDLESS family, or undead): key -> (rows, head, family,
    words, median level, worlds, undead)."""
    kinds, _ = sw.creature_kinds(npcs, pc, spells)
    out = {}
    for k, rows in kinds.items():
        dead = undead.get(k, bool(mv.UNDEAD.search(k)))
        h, fam, mods = head(k)
        if fam is None or not (mv.wildlife_word(k) or fam in WORDLESS or dead):
            continue
        out[k] = dict(rows=rows, head=h, family=fam, mods=mods, undead=dead,
                      level=statistics.median(n["level"] for n in rows), worlds={n["world"] for n in rows})
    return out


def body_for(name, lo, hi, kin_keys, worlds, kinds):
    """The kind whose body a made animal takes, and why; None, why not. Of
    its family, by: the same head noun (5), the animal's head among the
    kind's words or the other way (4: "young hammerhead", "hammerhead
    shark"), each word shared (7, a GENERIC one 2), young or grown alike (3,
    else -4), a kin of its table (4), its worlds (1), less a third of the
    levels between, and a common kind a little before a rare one. An undead
    kind only on a shared word (the icebone dog an icebone worg's)."""
    h, fam, mods = head(name)
    if fam is None:
        return None, f"no family for its head noun {h!r}"
    young, mods = "young" in mods, mods - {"young"}
    best = None
    for k, kd in kinds.items():
        if kd["family"] != fam:
            continue
        kmods = kd["mods"] - {"young"}
        if kd["undead"] and not mods & kmods:
            continue
        kyoung = "young" in kd["mods"]
        gap = max(0.0, lo - kd["level"], kd["level"] - hi)
        score = (5 if kd["head"] == h else 0) + (4 if h in kmods or kd["head"] in mods else 0) \
            + sum(2 if w in GENERIC else 7 for w in mods & kmods) \
            + (3 if young and kyoung else -4 if young != kyoung else 0) \
            + (4 if k in kin_keys else 0) + (1 if kd["worlds"] & set(worlds) else 0) - gap / 3 \
            + 0.3 * math.log(len(kd["rows"]))
        if best is None or (score, k) > (best[0], best[1]):
            best = (score, k)
    if best is None:
        return None, f"this server has no {fam} to take a body from"
    kd = kinds[best[1]]
    return best[1], f"{best[1]!r} ({fam}, L{kd['level']:g}, {len(kd['rows'])} rows{', undead' if kd['undead'] else ''})"


# ---- the plan -----------------------------------------------------------------------

def plan(db, ground_dir, data_dir, bestiary, count=COUNT, seed=DEFAULT_SEED, only=(), skip=()):
    cols, npcs, pc, spells, undead, moves = load(db)
    zones = read_zones(data_dir)
    by_letters = collections.defaultdict(list)
    for n in npcs:
        if n["letters"]:
            by_letters[n["letters"]].append(n)
    kinds = body_kinds(npcs, pc, spells, undead)
    towns, town_count = sw.town_people(npcs, pc)
    taken = sw.Grid(64.0)
    for n in npcs:
        taken.add(n["world"], n["x"], n["z"])
    folk = sw.Grid(LEVEL_RADIUS)
    in_cell = collections.defaultdict(list)
    for n in npcs:
        if creature(n, pc):
            folk.add(n["world"], n["x"], n["z"], n["level"])
            in_cell[(n["world"],) + cell_of(n["x"], n["z"])].append(n)
    grounds, envs = {}, {}

    def ground(world):
        if world not in grounds:
            path = os.path.join(ground_dir, f"{world}.ground")
            grounds[world] = sw.Ground(path) if os.path.exists(path) else None
        return grounds[world]

    def env(world):
        if world not in envs:
            path = os.path.join(ground_dir, f"{world}.env")
            envs[world] = Env(path) if os.path.exists(path) else None
        return envs[world]

    def rows_of(k):
        """The rows of a name, by its letters, the guide's or ALIASES'."""
        return by_letters.get(letters(ALIASES.get(k, k)), [])

    # each table's kin: its animals this server has; a water table's with its
    # animals in too few places, its worlds' sea creatures too
    kin = collections.defaultdict(list)
    kin_keys = collections.defaultdict(set)
    for k, sp in bestiary.items():
        for e in sp["entries"]:
            t = (e["part"], e["table"])
            for n in rows_of(k):
                if creature(n, pc) and n["world"] in KIN_WORLDS[e["part"]]:
                    kin[t].append(n)
                    kin_keys[t].add(n["key"])
    for part, tables in TABLES.items():
        for table in tables:
            t = (part, table)
            if table in WATER_TABLES and \
                    len({(n["world"],) + cell_of(n["x"], n["z"]) for n in kin[t]}) < MIN_KIN_CELLS:
                for n in npcs:
                    if creature(n, pc) and n["world"] in KIN_WORLDS[part] and mv.WATER.search(n["key"]):
                        kin[t].append(n)
                        kin_keys[t].add(n["key"])
    lavas = {}

    def lava(world):
        """Where the world's top floor is lava, LAVA_STEP apart."""
        if world not in lavas:
            g, ev = ground(world), env(world)
            pts = set()
            if g is not None and ev is not None:
                for e in g.entries:
                    if not e[9]:
                        continue
                    for i in range(int(e[0] // LAVA_STEP), int(e[2] // LAVA_STEP) + 1):
                        for j in range(int(e[1] // LAVA_STEP), int(e[3] // LAVA_STEP) + 1):
                            if (i, j) in pts:
                                continue
                            x, z = i * LAVA_STEP, j * LAVA_STEP
                            fl = g.floors(x, z)
                            if fl and ev.flags(x, fl[0] + 0.5, z) & LAVA:
                                pts.add((i, j))
            lavas[world] = [(i * LAVA_STEP, j * LAVA_STEP) for i, j in sorted(pts)]
        return lavas[world]

    def units(part, table):
        """The places a table's animals go: world, cell, a box, the zone's
        name and biome, and the kin to stand near (None: anywhere in it)."""
        out = []
        if part == TUNARIA and (table in BIOME or table == GLOBAL):
            for c, (zname, biomes) in sorted(zones.items()):
                if table == GLOBAL:
                    out.append(dict(world=0, cell=c, box=cell_box(c), name=zname, biome=None, anchors=None))
                    continue
                for q, b in enumerate(biomes):
                    if b == BIOME[table]:
                        out.append(dict(world=0, cell=c, box=quarter_box(c, q), biome=b, anchors=None,
                                        name=f"{zname} {('NW', 'NE', 'SW', 'SE')[q]}"))
        if part == FRONTIERS or table not in BIOME and table != GLOBAL or table in KIN_TOO:
            spots = collections.defaultdict(list)
            for n in kin[(part, table)]:
                spots[(n["world"],) + cell_of(n["x"], n["z"])].append((n["x"], n["z"]))
            if table == VOLCANIC:
                for w in KIN_WORLDS[part]:
                    for x, z in lava(w):
                        # a lava spot counts as two kin: one is a place
                        spots[(w,) + cell_of(x, z)] += [(x, z), (x, z)]
            for (w, cx, cz), pts in sorted(spots.items()):
                if len(pts) < 2:
                    continue
                c = (cx, cz)
                zname = zones[c][0] if w == 0 and c in zones else f"world {w} cell {cx},{cz}"
                out.append(dict(world=w, cell=c, box=cell_box(c), name=f"{zname} (kin)", biome=None, anchors=pts))
        return out

    def match(u, lo, hi):
        x0, x1, z0, z1 = u["box"]
        return sum(1 for n in in_cell[(u["world"],) + u["cell"]]
                   if x0 <= n["x"] < x1 and z0 <= n["z"] < z1 and lo - LEVEL_SLACK <= n["level"] <= hi + LEVEL_SLACK)

    def neighbours(world, x, z):
        return [lv for px, pz, lv in folk.near(world, x, z, LEVEL_RADIUS)]

    def stands(g, x, y, z):
        for dx, dz in ((STEP, 0), (-STEP, 0), (0, STEP), (0, -STEP)):
            if not any(abs(h - y) <= MAX_STEP for h in g.floors(x + dx, z + dz)):
                return False
        return True

    rows, groups, left, made = [], [], [], []
    placed = []   # every seed: world, x, z

    def place(sp, k, e, u, n, rng, body_rows, how, mine):
        """n spawns of an animal in a unit; the rows planned."""
        world = u["world"]
        g, ev = ground(world), env(world)
        if g is None:
            return 0, f"no ground file for world {world}"
        if ev is None and how != LAND:
            return 0, f"no environment file for world {world} (no water to put it in)"
        lo, hi = e["lo"], e["hi"]
        x0, x1, z0, z1 = u["box"]
        got, why = 0, "no place"
        for _ in range(n):
            for _ in range(TRIES):
                if u["anchors"]:
                    ax, az = rng.choice(u["anchors"])
                    r, a = KIN_REACH * math.sqrt(rng.random()), rng.uniform(0, 2 * math.pi)
                    x, z = ax + r * math.cos(a), az + r * math.sin(a)
                else:
                    x, z = rng.uniform(x0 + INSET, x1 - INSET), rng.uniform(z0 + INSET, z1 - INSET)
                if not (x0 + INSET <= x <= x1 - INSET and z0 + INSET <= z <= z1 - INSET):
                    continue
                if next(towns.near(world, x, z, TOWN_CLEAR), None) is not None:
                    why = "in or by a town"
                    continue
                if next(taken.near(world, x, z, SPACING), None) is not None:
                    continue
                if any(w == world and math.hypot(px - x, pz - z) < APART for w, px, pz in mine):
                    continue
                fl = g.floors(x, z)
                if not fl:
                    why = "no ground"
                    continue
                y = fl[0]
                if ev is not None:
                    f = ev.flags(x, y + 0.5, z)
                    if how == LAND and f & BAD_ENV or how == BOTH and f & SOAKED:
                        why = "no dry ground"
                        continue
                    if how == WATER and not (f & WET and ev.flags(x, y + WATER_DEPTH, z) & WET):
                        why = "no water deep enough"
                        continue
                if not stands(g, x, y, z):
                    why = "no level ground"
                    continue
                near = neighbours(world, x, z)
                med = statistics.median(near) if near else None
                if med is None and how == LAND or med is not None and not \
                        (lo - 2 * LEVEL_SLACK <= med <= hi + 2 * LEVEL_SLACK):
                    why = "nothing of its levels round"
                    continue
                if med is None:
                    lv = rng.randint(lo, hi)
                elif med < lo:
                    lv = rng.randint(lo, (lo + hi) // 2)
                elif med > hi:
                    lv = rng.randint((lo + hi + 1) // 2, hi)
                else:
                    lv = max(lo, min(hi, int(round(med)) + rng.choice((-1, 0, 0, 1))))
                t = sw.template(body_rows, world, x, z)
                row = dict(t["row"])
                row.pop("id", None)
                row.update(npc_name=sp["spelling"], x_coord=round(x, 3), y_coord=round(y, 4), z_coord=round(z, 3),
                           facing=rng.randrange(256), world=world, npc_level=lv, npc_id=0, dt_kill=None, zone=0)
                rows.append(dict(row=row, species=sp["name"], key=k, table=f"{e['part']}/{e['table']}",
                                 unit=u["name"], status=sp["status"], template=t["id"],
                                 biome=biome_at(zones, world, x, z)))
                taken.add(world, x, z)
                placed.append((world, x, z))
                mine.append((world, x, z))
                got += 1
                break
        return got, why

    only = {key(s) for s in only}
    skip = {key(s) for s in skip}
    for k, sp in bestiary.items():
        if only and k not in only and key(ALIASES.get(k, "")) not in only:
            continue
        have = rows_of(k)
        if len(have) >= THIN:
            continue
        status = "thin" if have else "missing"
        e0 = sp["entries"][0]
        if k in skip:
            left.append((sp["name"], e0, "left out by --skip"))
            continue
        # the body: its own rows, else a kind's
        own = [n for n in have if creature(n, pc)]
        if own:
            body_rows, body_key, body_why = own, own[0]["key"], f"its own rows ({len(own)})"
            spelling = sw.spelling(own)
        else:
            worlds = sorted({w for e in sp["entries"] for w in KIN_WORLDS[e["part"]]} |
                            ({0} if any(e["part"] == TUNARIA and e["table"] in BIOME for e in sp["entries"]) else set()))
            keys = set().union(*(kin_keys[(e["part"], e["table"])] for e in sp["entries"]))
            body_key, body_why = body_for(sp["name"], e0["lo"], e0["hi"], keys, worlds, kinds)
            if body_key is None:
                left.append((sp["name"], e0, f"no plausible body: {body_why}"))
                continue
            body_rows = kinds[body_key]["rows"]
            spelling = sp["name"].lower()
        sp = dict(sp, status=status, spelling=spelling)
        want = max(0, count - len(have))
        rng = random.Random(f"{seed}/{k}")
        mine, used, got_all, tried = [], set(), 0, []
        for e in sp["entries"]:
            if got_all >= want:
                break
            how = water_of(sp["name"], e["table"])
            us = [u for u in units(e["part"], e["table"]) if (u["world"],) + u["cell"] not in used]
            scored = [(match(u, e["lo"], e["hi"]), u) for u in us]
            scored = [(m, u) for m, u in scored if m >= MIN_MATCH]
            scored.sort(key=lambda mu: (-mu[0], mu[1]["name"]))
            pool = scored[:UNIT_CHOICE]
            pool.sort(key=lambda mu: -mu[0] * rng.uniform(0.5, 1.0))
            if not pool:
                tried.append(f"{e['table']}: no place of its levels (L{e['lo']}-{e['hi']})")
            for m, u in pool + scored[UNIT_CHOICE:]:
                if got_all >= want:
                    break
                cell = (u["world"],) + u["cell"]
                if cell in used:
                    continue
                n = min(PER_UNIT, want - got_all)
                got, why = place(sp, k, e, u, n, rng, body_rows, how, mine)
                if got:
                    used.add(cell)
                    got_all += got
                    groups.append(dict(species=sp["name"], key=k, status=status, have=len(have),
                                       table=f"{e['part']}/{e['table']}", levels=(e["lo"], e["hi"]), unit=u["name"],
                                       world=u["world"], match=m, got=got, water=how, body=body_why))
                else:
                    tried.append(f"{u['name']}: {why}")
        if got_all == 0:
            left.append((sp["name"], e0, "found no place: " + "; ".join(tried[:3])))
            continue
        if got_all < want:
            left.append((sp["name"], e0, f"{want - got_all} of {want} found no place"))
        if status == "missing":
            made.append(dict(name=spelling, body=body_key, rows=[r for r in rows if r["key"] == k],
                             table=e0["table"], water=water_of(sp["name"], e0["table"]),
                             undead=kinds[body_key]["undead"]))
    extra = made_rows(db, made)
    return dict(cols=cols, rows=rows, groups=groups, left=left, made=made, extra=extra, towns=town_count,
                moves=moves)


# ---- the tables a name keys ---------------------------------------------------------

# the table, its name column, its numbered key ("" for none: the name is it)
MADE_TABLES = (("npcclass", "name", ""), ("npctemplate", "name", ""), ("lootcreature", "npc_name", ""),
               ("lootpool", "npc_name", "id"), ("npcundead", "name", ""))


def loot_region(m):
    """A made animal's loot region: of the world most of its spawns are in,
    in Tunaria "marine" for one of the water (as the pike's and the sharks'
    are), else the biome of the quarter most stand in."""
    worlds = collections.Counter(r["row"]["world"] for r in m["rows"])
    w = worlds.most_common(1)[0][0]
    if w != 0:
        return WORLD_REGION.get(w)
    if m["water"] == WATER:
        return "marine"
    biomes = collections.Counter(r["biome"] for r in m["rows"] if r["row"]["world"] == 0 and r["biome"])
    if biomes:
        return biomes.most_common(1)[0][0].lower()
    return "tunaria"


def made_rows(db, made):
    """The rows a made animal takes in the tables a name keys: its body's,
    renamed (a table holding a row of its name keeps it), and its npcmove row
    when the server's words would not move it as an animal."""
    out = []
    for m in made:
        for table, col, idcol in MADE_TABLES:
            if not table_exists(db, table):
                continue
            if db.execute(f"SELECT count(*) FROM {table} WHERE lower(trim({col})) = ?", (m["name"],)).fetchone()[0]:
                continue
            cur = db.execute(f"SELECT * FROM {table} WHERE lower(trim({col})) = ? ORDER BY rowid", (m["body"],))
            names = [d[0] for d in cur.description]
            for vals in cur.fetchall():
                row = {}
                for c, v in zip(names, vals):
                    if c == idcol:
                        continue
                    if c == col:
                        v = m["name"]
                    elif c == "source":
                        v = SOURCE
                    elif c == "region" and table == "lootcreature":
                        v = loot_region(m) or v
                    row[c] = v
                out.append(dict(table=table, name=m["name"], row=row))
        if table_exists(db, "npcmove") and not db.execute(
                "SELECT count(*) FROM npcmove WHERE lower(trim(name)) = ?", (m["name"],)).fetchone()[0]:
            # an undead body stands as its kind does (entity/movement.go)
            roam = -1 if mv.wildlife_word(m["name"]) or m["undead"] else 1
            water = "both" if m["water"] == BOTH else ""
            if roam == 1 or water:
                why = []
                if roam == 1:
                    why.append(f"wanders: an animal by its body (as {m['body']})")
                if water:
                    why.append("in and out of the water")
                out.append(dict(table="npcmove", name=m["name"], row=dict(
                    name=m["name"], roam=roam, radius=0, water=water, note="; ".join(why), source=SOURCE)))
    return out


# ---- writing it, and taking it back out ---------------------------------------------

LOG_DDL = [
    "CREATE TABLE prima_wildlife_seed_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
    "CREATE TABLE prima_wildlife_seed_npcs (npc_row INTEGER PRIMARY KEY, species TEXT NOT NULL, "
    "prima_table TEXT NOT NULL, unit TEXT NOT NULL, status TEXT NOT NULL, template_row INTEGER NOT NULL)",
    "CREATE TABLE prima_wildlife_seed_rows (tbl TEXT NOT NULL, row INTEGER NOT NULL, name TEXT NOT NULL, "
    "PRIMARY KEY (tbl, row))",
]
SEQ_TABLES = ("npcs", "lootpool")


def seeded(db):
    return table_exists(db, "prima_wildlife_seed_npcs")


def seq(db, table):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (table,)).fetchone()
    return None if r is None else r[0]


def apply(db, p, args):
    if seeded(db):
        sys.exit("this database already holds a Prima wildlife seed (prima_wildlife_seed_npcs): --undo it first")
    cols = [c for c in p["cols"] if c != "id"]
    db.execute("BEGIN IMMEDIATE")
    try:
        for stmt in LOG_DDL:
            db.execute(stmt)
        meta = dict(when=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                    seed=str(args.seed), count=str(args.count), only=args.only, skip=args.skip,
                    rows=str(len(p["rows"])), extra=str(len(p["extra"])))
        for t in SEQ_TABLES:
            meta[f"seq_{t}"] = "" if seq(db, t) is None else str(seq(db, t))
        for r in p["rows"]:
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r["row"].get(c) for c in cols])
            db.execute("INSERT INTO prima_wildlife_seed_npcs VALUES (?, ?, ?, ?, ?, ?)",
                       (cur.lastrowid, r["species"], r["table"], r["unit"], r["status"], r["template"]))
        for x in p["extra"]:
            names = list(x["row"])
            cur = db.execute(f"INSERT INTO {x['table']} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})",
                             [x["row"][c] for c in names])
            db.execute("INSERT INTO prima_wildlife_seed_rows VALUES (?, ?, ?)", (x["table"], cur.lastrowid, x["name"]))
        db.executemany("INSERT INTO prima_wildlife_seed_meta VALUES (?, ?)", sorted(meta.items()))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not seeded(db):
        sys.exit("no Prima wildlife seed to undo here")
    meta = dict(db.execute("SELECT key, value FROM prima_wildlife_seed_meta"))
    db.execute("BEGIN IMMEDIATE")
    try:
        n = db.execute("DELETE FROM npcs WHERE id IN (SELECT npc_row FROM prima_wildlife_seed_npcs)").rowcount
        m = 0
        for tbl, row in db.execute("SELECT tbl, row FROM prima_wildlife_seed_rows").fetchall():
            m += db.execute(f"DELETE FROM {tbl} WHERE rowid = ?", (row,)).rowcount
        # each AUTOINCREMENT counter back, unless a row was added after the seed
        for t in SEQ_TABLES:
            before = meta.get(f"seq_{t}", "")
            if not table_exists(db, t):
                continue
            top = db.execute(f"SELECT max(rowid) FROM {t}").fetchone()[0] or 0
            if before == "":
                if top == 0:
                    db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
            elif top <= int(before):
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (int(before), t))
        for t in ("prima_wildlife_seed_npcs", "prima_wildlife_seed_rows", "prima_wildlife_seed_meta"):
            db.execute(f"DROP TABLE {t}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n, m


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-primawildlife"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(p, verbose=False):
    rows, groups = p["rows"], p["groups"]
    print(f"towns: {p['towns']} townsfolk in towns (entity/towns.go), kept {TOWN_CLEAR:.0f} off")
    worlds = collections.Counter(r["row"]["world"] for r in rows)
    species = {g["key"] for g in groups}
    made = {g["key"] for g in groups if g["status"] == "missing"}
    print(f"{len(rows)} spawns of {len(species)} animals ({len(made)} missing, {len(species - made)} thin), "
          "by world: " + ", ".join(f"{w}: {n}" for w, n in sorted(worlds.items())))
    by = collections.defaultdict(list)
    for g in groups:
        by[g["key"]].append(g)
    for k, gs in by.items():
        g0 = gs[0]
        got = sum(g["got"] for g in gs)
        had = f", had {g0['have']}" if g0["have"] else ""
        print(f"  {g0['species']} [{g0['table']} L{g0['levels'][0]}-{g0['levels'][1]}, {g0['status']}{had}, "
              f"{g0['water']}]: {got}, body {g0['body']}")
        for g in gs:
            print(f"      {g['unit']} (world {g['world']}, {g['match']} creatures of its levels): {g['got']}")
    if p["extra"]:
        tables = collections.Counter(x["table"] for x in p["extra"])
        print("name-keyed rows for the made animals: " + ", ".join(f"{t} {n}" for t, n in sorted(tables.items())))
        for x in p["extra"]:
            if x["table"] == "npcmove":
                print(f"    npcmove {x['name']!r}: roam {x['row']['roam']}, water {x['row']['water'] or '-'}")
    if p["left"]:
        print("left out:")
        for name, e, why in p["left"]:
            print(f"    {name} [{e['part']}/{e['table']} L{e['lo']}-{e['hi']}]: {why}")
    if verbose:
        for r in rows:
            row = r["row"]
            print(f"  {row['npc_name']!r} L{row['npc_level']} world {row['world']} at ({row['x_coord']:.1f}, "
                  f"{row['y_coord']:.2f}, {row['z_coord']:.1f}) in {r['unit']}, copy of {r['template']}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--bestiary", required=True, help="the guide's bestiary as JSON (the operator's own parse)")
    ap.add_argument("--eqoa-data", default=DEFAULT_DATA, help="a checkout of EQOA-Data (its Map/zones.json)")
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground and environment files' folder")
    ap.add_argument("--apply", action="store_true", help="write the spawns")
    ap.add_argument("--undo", action="store_true", help="delete every row a seed wrote")
    ap.add_argument("--count", type=int, default=COUNT, help="spawns an animal is seeded up to")
    ap.add_argument("--only", default="", help="these animals alone, by name, comma-separated")
    ap.add_argument("--skip", default="", help="animals to leave out, by name, comma-separated")
    ap.add_argument("--seed", type=int, default=DEFAULT_SEED, help="the placement's random seed")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--verbose", action="store_true", help="list every spawn")
    args = ap.parse_args()
    writing = args.apply or args.undo
    target = os.path.abspath(args.db) if writing else f"file:{os.path.abspath(args.db)}?mode=ro"
    db = sqlite3.connect(target, isolation_level=None, uri=True)
    db.execute("PRAGMA busy_timeout = 5000")
    if args.undo:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        n, m = undo(db)
        print(f"deleted {n} seeded spawns and {m} name-keyed rows; restart the server")
        return
    if seeded(db):
        n = db.execute("SELECT count(*) FROM prima_wildlife_seed_npcs").fetchone()[0]
        print(f"a Prima wildlife seed is in already ({n} spawns): --undo it to plan again")
        return
    split = lambda s: [x.strip() for x in s.split(",") if x.strip()]  # noqa: E731
    p = plan(db, args.ground, args.eqoa_data, read_bestiary(args.bestiary), count=args.count, seed=args.seed,
             only=split(args.only), skip=split(args.skip))
    report(p, args.verbose)
    if not args.apply:
        print(f"{len(p['rows'])} spawns and {len(p['extra'])} name-keyed rows (report only: --apply writes them)")
        return
    if not args.no_backup:
        print("backup:", backup(db, args.db))
    apply(db, p, args)
    print(f"wrote {len(p['rows'])} spawns and {len(p['extra'])} name-keyed rows; restart the server to read them")


if __name__ == "__main__":
    main()
