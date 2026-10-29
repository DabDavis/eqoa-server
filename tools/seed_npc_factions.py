#!/usr/bin/env python3
"""The seed rows of npcfaction: which faction a town's people and the
creatures among them are of, so that their target face and their aggro
follow a player's standing (entity/aggro.go; 2026-10-29: Neriak's ghoul
servant, a creature's body 52 from the nearest dark elf, outside the town's
40, killed a new dark elf by the coach).

Towns:

- anchors: every start spot of defaultcharacter, named by its nearest map
  town within 1000 (EQOA-Data's Map/zones.json, laid over Tunaria by
  seed_faction_hits.py's fit), else by its race's home city (the ogres'
  Oggok, world 1); and, with --eqoa-data, the map's towns themselves. A start
  spot stands among its city's people, where the map's point may be 900 off
  (Neriak's is in Nektulos, over the cavern the city is in; its 400 reach is
  why seed_faction_hits.py's Neriak is the Nektulos outpost's ten).
- people: NPCs with a players' race's body (charactermodel), chained within
  --link (60, towns.go's) of one another in a world; a chain is the town's of
  the nearest anchor within --reach (400) of one of its people.
- a town's faction is its "Citizens of" (seed_faction_hits.py's citizens_of,
  by its name, then the map's "aka"); its guards' its guard faction where it
  has one (seed_faction_hits.py's guard_factions, the best by
  seed_npc_social.py's order: The Freeport Militia, The Qeynos Guard, The
  Highpass Guard, Klick`Anon Watch). A town with none is left out (said).

Rows, each said by town:

- a town's people (a guard by seed_npc_social.py's guard words to its guard
  faction, else to its Citizens), the NPC-bit ones too (their face; they
  attack no one); one whose name ends in a house's ("... Do`Vexis": House
  Do`Vexis; "Merchant House Nothard") or is a faction's ("Zentar Reese") to
  that faction; a word of a faction of no kind keeps no one from its town
  ("Priest of Bertoxxulous" is Freeport's, whatever Sandstorm's Bertoxxulous);
- the creatures among a town's people: a creature's body, no NPC bit,
  spawned within --near (40, the town's margin) in 3D of one of the town's
  people, or inside the hull of the town's people within --hull (150) of it
  and --height (30) of its height: Neriak's servants, zombies, animations
  and its ghoul servant 53 off between two of the city's chains. Wildlife
  (wildlife.go's words, seed_npc_move.py's) only inside that hull: a city's
  cats and tavern rats, the cave minnows of Neriak's lake (Sandstorm's let a
  level-1 swimmer be), not the outskirts' snakes, nor the forest's bats over
  Neriak's cavern (54 above it).
- left out, said: players' pets and what a GM made peaceful (npcaggro rows
  aggressive 0), the dummies, prisoners, spies and slaves, objects (chests,
  announcements, signposts), and a creature or person whose name names a
  faction of its own kind ("Pickclaw Warrior": Pickclaw Goblins, the gnoll
  raiders by Darvar Manor: Gnoll Raiders, a "Bandit": Bandits of Tunaria):
  all the faction's own words but its kind's and "of", "the", "Tunaria"...;
  or its kind's word when it has no other. --creature-factions gives each
  such name its faction where only one matches, and the creatures named a
  faction's name ("Lord Nagafen"), listed. Off by default: an NPC of a
  faction attacks only a player whose standing with it is below -350.

- the NPCs quests talk to that no town takes in, by name and world (NAMED):
  the Plane of Sky's quest NPCs and essences to Denizens of the Plane of Sky,
  Graumord's citizens to Citizens of Graumord, the Fire Giant Magus to Lord
  Nagafen (never on sight, still attackable); Kithgrin left (UNPLACED).

A name's row is for every NPC of the name (world -1) when all its spawns are
the one town's and of the one faction; else for those of the town, as a place
(world, the middle of their spawns, the radius round them + 5), when no other
spawn of the name stands in it; else the name is left to a GM (said).

The impact, before anything is written: per town, how many of its
attackable members (no NPC bit, no npcaggro row) would attack a new
character of each race (humans by city; a range over the race's classes) on
sight: its starting standing (factionstart, else the factions table; with
--measured, Sandstorm's measured TSV wherever it holds the combo) below -350.
And the start check: a new character of each combo at its start spot must not
be attacked on sight by a member within 60 of it; --apply is refused where it
would be (--force writes anyway): seed the measured starts first
(tools/seed_faction_starts.py).

Rows are written with source "seed" and never over a GM's row for a name
(any place). Every row written is logged in npcfaction_seed; --undo deletes
them, but not one a GM changed since (said), and drops the log; a second
--apply is refused while it is there. Both take a copy of the database first
(<db>.bak-<when>-pre-npcfaction). Report only by default. The running server
reads the table at its start, and again after any GM panel edit of NPC
factions.

    tools/seed_npc_factions.py --db eqoa.db [--eqoa-data ../external/EQOA-Data]
        [--measured starts.tsv] [--apply | --undo] [--force] [--creature-factions]
        [--no-backup] [--quiet]
"""
import argparse
import collections
import datetime
import json
import math
import os
import re
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_faction_hits as fh  # noqa: E402  the map's fit, a town's Citizens and guard factions
import seed_faction_starts as sfs  # noqa: E402  the starts as the server reads them
import seed_npc_move as mv  # noqa: E402  NPCClassKey, wildlife.go's words, the undead
import seed_npc_social as soc  # noqa: E402  a guard's words, the guard factions' order

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"
LOG = "npcfaction_seed"
KOS_BELOW = -350  # entity.FactionKOSBelow: the red faces
START_REACH = 60  # entity.startThreatReach
MAP_REACH = 1000  # a start spot takes the name of a map town this near

# the table as database/factions.go makes it (npcFactionSchema)
SCHEMA = """CREATE TABLE IF NOT EXISTS npcfaction (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL,
    world   INTEGER NOT NULL DEFAULT -1,
    x       REAL NOT NULL DEFAULT 0,
    z       REAL NOT NULL DEFAULT 0,
    radius  REAL NOT NULL DEFAULT 0,
    faction TEXT NOT NULL,
    note    TEXT NOT NULL DEFAULT '',
    source  TEXT NOT NULL DEFAULT 'gm',
    UNIQUE (name, world, x, z, radius)
)"""
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    rowid   INTEGER NOT NULL,  -- the npcfaction row written
    name    TEXT NOT NULL,
    faction TEXT NOT NULL
)"""

# words of a name that is not a town's own, whatever it stands among
NOT_TOWNS = {"prisoner", "prisoners", "captive", "captives", "spy", "spies", "slave", "slaves"}
OBJECTS = {"chest", "chests", "announcement", "announcements", "signpost", "signposts", "sign", "portal", "statue",
           "crate", "barrel", "fire", "campfire"}  # a fire: Moradhim's "RE PC Fire 1" to 3
DUMMY = re.compile(r"^(?:training|regular|sparring) dummy")

# NPCs quests talk to that no town takes in, by name and world, with the
# faction whose measured start leaves a new character Friendly, or short of
# Aggressive (the coordinator, 2026-10-29: "the Portal quests must TALK to
# them"; until then creatures' bodies attacking on sight). Measured on
# Sandstorm, every combo alike so far: Denizens of the Plane of Sky +200,
# Citizens of Graumord -150, Lord Nagafen -500.
NAMED = [
    # the Plane of Sky's quest NPCs and its essences (EQOA-Data's Plane of Sky
    # chains: "Approach the Essence of Night"); each essence's other rows carry
    # the NPC bit; not its guardians, villagers or the Quintessence, fought
    ("the Plane of Sky", 4, "Denizens of the Plane of Sky",
     ["sartheret", "mark of the majestic", "aviak elder", "essence of day", "essence of desolation",
      "essence of night", "essence of rain", "essence of snow", "essence of sun"]),
    # the Isle of Dread's town ("speak to the graumord citizen in the Inn")
    ("Graumord", 0, "Citizens of Graumord", ["graumord citizen"]),
    # the way to Nagafen's lair (EQOA-Data's Lord Nagafen Portal Quest): never
    # on sight, still attackable; Fire Giants' -1000 would set him on everyone
    ("Solusek's Eye", 3, "Lord Nagafen", ["fire giant magus"]),
]
# and those left, said: no faction their place names
UNPLACED = {"kithgrin": "the Cazic Thule temple's trial giver, among its trial creatures: its place names no faction "
                        "(an npcaggro row of 0, or the NPC bit, would keep it from attacking on sight)"}
# the words of a faction's name that name no one in particular, and the kinds
GENERIC = {"the", "of", "a", "an", "and", "tunaria", "tunarian", "clan", "tribe", "hordes", "horde", "mtns", "ro",
           "karanas", "plains"}
KINDS = {"gnoll", "goblin", "hobgoblin", "orc", "kobold", "aviak", "giant", "minotaur", "lizard", "lizardman",
         "lizardmen", "ant", "bandit", "pirate", "froglok", "ogre", "troll", "centaur", "rat", "were", "elf", "elves",
         "barbarian", "treant", "fungusman", "fungusmen", "kappa"}
# a faction's own words too common to name it alone: its kind's word is wanted
# too ("undead servant" is no Undead Froglok, a "Fire Bat" no Fire Giant), and
# a faction of no kind named by these alone names no one
COMMON = {"undead", "black", "red", "ear", "fire", "pit", "hill", "sand", "ice", "lost", "desert", "dark", "night",
          "stone", "mountain", "gate", "city", "old", "high", "deep", "last", "royal", "silent", "raider", "lurking",
          "eye", "guard", "brood", "cult", "order", "temple", "church", "village", "hive", "den", "watch"}


def words(name):
    return re.findall(r"[a-z]+", name.lower().replace("`", "").replace("'", ""))


def singular(w):
    return mv.singulars(w)[-1] if len(w) > 3 else w


def wordset(name):
    out = set()
    for w in words(name):
        out.update(mv.singulars(w))
    return out


def home_city(race, hum, cls):
    """database.HomeCity, as the map spells the cities."""
    if race in (0, 10):
        if race == 0 and hum == 2:
            return "Surefall Glade" if cls in (1, 7) else "Qeynos"
        return "Freeport"
    if race == 1:
        return "Tethelin" if cls in (1, 5, 6, 7) else "Fayspire"
    return {2: "Neriak, Nektulos", 3: "Klick'Anon", 4: "Moradhim", 5: "Grobb", 6: "Halas", 7: "Rivervale",
            8: "Highbourne", 9: "Oggok"}.get(race)


def read(db):
    models = {(m or 0) & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel")}
    npcs = []
    for r in db.execute("""SELECT id, npc_name, COALESCE(x_coord, 0), COALESCE(y_coord, 0), COALESCE(z_coord, 0),
            COALESCE(modelid, 0), COALESCE(npc_type, 0), COALESCE(world, 0), COALESCE(npc_level, 1) FROM npcs"""):
        name = r[1] or ""
        npcs.append(dict(id=r[0], name=name, key=mv.key(name), x=r[2], y=r[3], z=r[4], model=r[5] & 0xFFFFFFFF,
                         type=r[6], world=r[7], level=r[8], folk=bool(r[5]) and (r[5] & 0xFFFFFFFF) in models))
    try:
        rules = {n: a for n, a in db.execute("SELECT npc_name, aggressive FROM npcaggro")}
    except sqlite3.OperationalError:
        rules = {}
    try:
        undead = {mv.key(n): bool(u) for n, u in db.execute("SELECT name, undead FROM npcundead")}
    except sqlite3.OperationalError:
        undead = {}
    factions = [r[0] for r in db.execute("SELECT name FROM factions")]
    return npcs, models, rules, undead, factions


def anchors(db, eqoa_data, npcs):
    """The places a town is named by: each start spot (with its combo) and,
    with eqoa_data, each map town."""
    maptowns = []
    if eqoa_data:
        with open(os.path.join(eqoa_data, "Map", "zones.json"), encoding="utf-8") as f:
            zones = json.load(f)
        by_name = collections.defaultdict(list)
        for n in npcs:
            if n["world"] == 0:
                by_name[n["name"].strip()].append(dict(x=n["x"], z=n["z"]))
        pois = collections.defaultdict(list)
        for p in zones["pois"]:
            pois[fh.poi_name(p["name"])].append(p)
        (ax, bx, az, bz), *_ = fh.fit_map(pois, by_name)
        for t in zones["towns"]:
            maptowns.append(dict(name=t["name"], aka=t.get("aka"), world=0, x=ax * t["x"] + bx, z=az * t["y"] + bz,
                                 kind="map"))
    out = list(maptowns)
    for race, cls, hum, w, x, y, z in db.execute(
            "SELECT race, tclass, humType, COALESCE(world, 0), x, y, COALESCE(z, 0) FROM defaultcharacter"):
        name, aka = None, None
        if w == 0 and maptowns:
            d, t = min(((math.hypot(t["x"] - x, t["z"] - z), i) for i, t in enumerate(maptowns)))
            if d <= MAP_REACH:
                name, aka = maptowns[t]["name"], maptowns[t]["aka"]
        if name is None:
            name = home_city(race, hum, cls)
        if name:
            out.append(dict(name=name, aka=aka, world=w, x=x, y=y, z=z, kind="start", combo=(race, cls, hum)))
    return out


class Grid:
    """Points by world and cell, for the near ones."""

    def __init__(self, cell=64):
        self.cell, self.g = cell, collections.defaultdict(list)

    def add(self, n, tag=None):
        self.g[(n["world"], int(n["x"] // self.cell), int(n["z"] // self.cell))].append((n, tag))

    def near(self, world, x, z, r):
        k = int(r // self.cell) + 1
        cx, cz = int(x // self.cell), int(z // self.cell)
        for dx in range(-k, k + 1):
            for dz in range(-k, k + 1):
                for n, tag in self.g.get((world, cx + dx, cz + dz), ()):
                    if math.hypot(n["x"] - x, n["z"] - z) <= r:
                        yield n, tag


def towns_of(npcs, places, link, reach):
    """The town of every person (id -> town name), by the chain it is in."""
    town_of = {}
    for w in sorted({n["world"] for n in npcs if n["folk"]}):
        pts = [n for n in npcs if n["folk"] and n["world"] == w]
        here = [a for a in places if a["world"] == w]
        if not here:
            continue
        for g in fh.chains(pts, link):
            members = [pts[i] for i in g]
            best = None
            for a in here:
                d = min(math.hypot(m["x"] - a["x"], m["z"] - a["z"]) for m in members)
                if d <= reach and (best is None or d < best[0]):
                    best = (d, a["name"])
            if best:
                for m in members:
                    town_of[m["id"]] = best[1]
    return town_of


def hull(pts):
    pts = sorted(set(pts))
    if len(pts) < 3:
        return pts

    def cross(o, a, b):
        return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0])
    lo, hi = [], []
    for p in pts:
        while len(lo) >= 2 and cross(lo[-2], lo[-1], p) <= 0:
            lo.pop()
        lo.append(p)
    for p in reversed(pts):
        while len(hi) >= 2 and cross(hi[-2], hi[-1], p) <= 0:
            hi.pop()
        hi.append(p)
    return lo[:-1] + hi[:-1]


def inside(h, p):
    if len(h) < 3:
        return False
    for i in range(len(h)):
        a, b = h[i], h[(i + 1) % len(h)]
        if (b[0] - a[0]) * (p[1] - a[1]) - (b[1] - a[1]) * (p[0] - a[0]) < 0:
            return False
    return True


def town_factions(name, aka, factions):
    """A town's Citizens faction, its guards' faction (or None) and its key."""
    cit, k = fh.citizens_of(name, factions)
    if not cit and aka:
        cit, k = fh.citizens_of(aka, factions)
    guards = fh.guard_factions(k, factions) if k else []
    guard = sorted(guards, key=lambda f: (soc.faction_rank(f), f))[0] if guards else None
    return cit, guard, k


def kind_factions(factions, town_keys):
    """The factions of a kind of their own: neither a city's Citizens nor a
    house, nor naming a town; each with its own words (but GENERIC and the
    kinds') and its kind's word."""
    out = []
    for f in factions:
        k = fh.key(f)
        if f.startswith("Citizens of ") or f.startswith("House ") or "House " in f:
            continue
        if any(t and len(t) >= 5 and t in k for t in town_keys):
            continue
        ws = [singular(w) for w in words(f)]
        kind = [w for w in ws if w in KINDS]
        own = {w for w in ws if w not in GENERIC and w not in KINDS}
        out.append((f, own, kind[0] if kind else None))
    return out


def own_kind(name, kinds):
    """The factions of their own kind a name names (kind_factions): all the
    faction's own words, and its kind's where they are all COMMON; its kind's
    word alone where it has no other."""
    ws = wordset(name)
    hit = []
    for f, own, kind in kinds:
        if own and own <= ws:
            if own <= COMMON and not (kind and kind in ws):
                continue
            hit.append(f)
        elif not own and kind and kind in ws:
            hit.append(f)
    return hit


def houses(factions):
    """A house's name (as fh.key) -> its faction: "House Do`Vexis", "Merchant
    House Nothard"."""
    out = {}
    for f in factions:
        m = re.search(r"\bHouse (.+)$", f)
        if m:
            out[fh.key(m.group(1))] = f
    return out


def plan(db, eqoa_data, link=60, reach=400, near=40, hull_r=150, height=30, creature_factions=False):
    npcs, models, rules, undead, factions = read(db)
    places = anchors(db, eqoa_data, npcs)
    town_of = towns_of(npcs, places, link, reach)
    town_world = {}
    aka = {}
    for a in places:
        aka.setdefault(a["name"], a.get("aka"))
    for n in npcs:
        if n["id"] in town_of:
            town_world.setdefault(town_of[n["id"]], n["world"])
    tfac = {t: town_factions(t, aka.get(t), factions) for t in town_world}
    town_keys = {v[2] for v in tfac.values() if v[2]}
    kinds = kind_factions(factions, town_keys)
    kind_of = {f: kd for f, _, kd in kinds}
    house = houses(factions)
    by_key_faction = {fh.key(f): f for f in factions}

    people = Grid()
    for n in npcs:
        t = town_of.get(n["id"])
        if t and tfac[t][0]:
            people.add(n, t)

    members = collections.defaultdict(list)  # (key, faction, town) -> npcs
    left = collections.defaultdict(list)     # why -> names
    kind_rows = collections.defaultdict(list)  # (key, faction) -> npcs, --creature-factions
    for n in npcs:
        k = n["key"]
        if not k:
            continue
        ws = set(words(n["name"]))
        if n["folk"]:
            t = town_of.get(n["id"])
            if not t:
                continue
            cit, guard, _ = tfac[t]
            if not cit:
                left[f"{t}: no Citizens faction"].append(n["name"])
                continue
            if ws & NOT_TOWNS:
                left["not a town's own (a prisoner, a spy, a slave)"].append(n["name"])
                continue
            last = n["name"].strip().split(" ")[-1] if n["name"].strip() else ""
            f = house.get(fh.key(last)) or by_key_faction.get(fh.key(n["name"]))
            if not f:
                # a person named for a kind's faction ("Bandit") is its kind's; one
                # named for a faction of no kind stays its town's ("Priest of
                # Bertoxxulous" is Freeport's: Sandstorm's Bertoxxulous starts every
                # new character at -1000)
                kinded = [h for h in own_kind(n["name"], kinds) if kind_of[h]]
                if kinded:
                    left["of a kind's own faction"].append(f"{n['name']} ({', '.join(kinded[:2])})")
                    if creature_factions and len(kinded) == 1:
                        kind_rows[(k, kinded[0])].append(n)
                    continue
                f = guard if guard and soc.is_guard(n["name"]) else cit
            members[(k, f, t)].append(n)
            continue
        # a creature's body
        if not n["model"] or n["type"] & 0x80:
            continue
        hits = own_kind(n["name"], kinds)
        named = by_key_faction.get(fh.key(n["name"]))
        if creature_factions and (named or len(hits) == 1):
            kind_rows[(k, named or hits[0])].append(n)
        best = None
        for m, t in people.near(n["world"], n["x"], n["z"], hull_r):
            d = math.dist((m["x"], m["y"], m["z"]), (n["x"], n["y"], n["z"]))
            if best is None or d < best[0]:
                best = (d, t)
        if best is None:
            continue
        d, t = best
        pts = [(m["x"], m["z"]) for m, tt in people.near(n["world"], n["x"], n["z"], hull_r)
               if tt == t and abs(m["y"] - n["y"]) <= height]
        among = inside(hull(pts), (n["x"], n["z"]))
        wild = bool(mv.wildlife_word(n["name"])) and not mv.is_undead(k, undead)
        if not (among if wild else (d <= near or among)):
            continue
        why = None
        if rules.get(n["name"]) == 0:
            why = "a pet or made peaceful (npcaggro)"
        elif DUMMY.match(k):
            why = "a dummy"
        elif ws & NOT_TOWNS:
            why = "not a town's own (a prisoner, a spy, a slave)"
        elif ws & OBJECTS:
            why = "an object"
        elif hits or named:
            why = "of a kind's own faction"
        if why:
            left[why].append(f"{n['name']} ({t})")
            continue
        members[(k, tfac[t][0], t)].append(n)

    # the rows: a name's every NPC, else a place round the town's
    spawns = collections.defaultdict(list)
    for n in npcs:
        spawns[n["key"]].append(n)
    try:
        gm = {k for (k,) in db.execute("SELECT name FROM npcfaction WHERE source != ?", (SOURCE,))}
        gm = {mv.key(k) for k in gm}
    except sqlite3.OperationalError:
        gm = set()
    groups = collections.defaultdict(list)
    for (k, f, t), ms in members.items():
        groups[k].append((f, t, ms))
    if creature_factions:
        for (k, f), ms in kind_rows.items():
            if k not in groups:
                groups[k].append((f, "(its kind)", ms))
    rows = []  # (key, world, x, z, radius, faction, town, npcs)
    for k in sorted(groups):
        if k in gm:
            left["a GM's npcfaction row"].append(k)
            continue
        gs = groups[k]
        every = spawns[k]
        if len(gs) == 1 and len(gs[0][2]) == len(every):
            f, t, ms = gs[0]
            rows.append((k, -1, 0.0, 0.0, 0.0, f, t, ms))
            continue
        for f, t, ms in gs:
            ids = {m["id"] for m in ms}
            w = ms[0]["world"]
            cx = round(sum(m["x"] for m in ms) / len(ms), 1)
            cz = round(sum(m["z"] for m in ms) / len(ms), 1)
            r = math.ceil(max(math.hypot(m["x"] - cx, m["z"] - cz) for m in ms) * 10) / 10 + 5
            others = [o for o in every if o["id"] not in ids and o["world"] == w and math.hypot(o["x"] - cx, o["z"] - cz) <= r]
            if others:
                left["namesakes inside its place"].append(f"{k} ({t}: {len(others)} more)")
                continue
            rows.append((k, w, cx, cz, r, f, t, ms))
    # the named, over any town's rows for their names
    known = set(factions)
    for place, w, f, keys in NAMED:
        for k in keys:
            ms = [n for n in spawns.get(k, []) if n["world"] == w]
            if f not in known:
                left["named, but no such faction"].append(f"{k} ({f})")
                continue
            if not ms:
                left["named, but not in the database"].append(f"{k} (world {w})")
                continue
            if k in gm:
                left["a GM's npcfaction row"].append(k)
                continue
            rows = [r for r in rows if r[0] != k]
            rows.append((k, w, 0.0, 0.0, 0.0, f, place, ms))
    for k, why in UNPLACED.items():
        if spawns.get(k):
            left["left to a GM"].append(f"{k}: {why}")
    combos = sorted({c for c in (a.get("combo") for a in places) if c})
    spots = [a for a in places if a["kind"] == "start"]
    return dict(rows=rows, left=left, towns=tfac, town_world=town_world, people=town_of, npcs=npcs, rules=rules,
                combos=combos, spots=spots, factions=factions)


def npc_faction(rows_by_key, n):
    """The faction an NPC is of by the rows (entity/faction.go's
    npcFactionLocked): its name's row for where it spawned, a place's first
    (the smallest), then a world's, then anywhere's; "" none."""
    best = None
    for w, x, z, r, f in rows_by_key.get(n["key"], ()):
        if w >= 0 and w != n["world"]:
            continue
        if r > 0 and math.hypot(n["x"] - x, n["z"] - z) > r:
            continue
        rank = r if r > 0 else (1e30 if w >= 0 else 1e31)
        if best is None or rank < best[0]:
            best = (rank, f)
    return best[1] if best else ""


class Starts:
    """A new character's starting standings: Sandstorm's measured (with
    --measured) where they hold its combo, else factionstart's, else the
    factions table's (seed_faction_starts.py's rule)."""

    def __init__(self, db, measured=None):
        self.own = dict(db.execute("SELECT name, standing FROM factions"))
        self.rows = collections.defaultdict(list)
        try:
            for r in db.execute("SELECT race, class, humtype, faction, standing FROM factionstart"):
                self.rows[r[3]].append((r[0], r[1], r[2], r[4]))
        except sqlite3.OperationalError:
            pass
        self.measured = measured or {}
        self.combos = {c for m in self.measured.values() for c in m}

    def of(self, combo, faction):
        combo = sfs.combo_of(*combo)
        m = self.measured.get(faction)
        if m and combo in m:
            return m[combo]
        return sfs.resolve(self.rows.get(faction, []), self.own.get(faction, 0), combo)


def attackable(n, rules):
    """entity.factionAggroLocked's NPCs: no NPC bit, no npcaggro row."""
    return not n["type"] & 0x80 and n["name"] not in rules


def impact(p, starts, extra_rows=()):
    """Per town, how many of its attackable members a new character of each
    race group starts KOS to (a range over the group's combos); and the start
    spots where a member within START_REACH would attack the new character."""
    rows_by_key = collections.defaultdict(list)
    for k, w, x, z, r, f, *_ in list(p["rows"]) + list(extra_rows):
        rows_by_key[k].append((w, x, z, r, f))
    groups = collections.OrderedDict()
    for c in p["combos"]:
        race, _, hum = c
        g = ("Human east" if hum == 1 else "Human west") if race == 0 else sfs.name_of(sfs.RACES, race)
        groups.setdefault(g, []).append(c)
    table = collections.defaultdict(dict)
    members = []
    for k, w, x, z, r, f, t, ms in p["rows"]:
        for n in ms:
            if attackable(n, p["rules"]) and npc_faction(rows_by_key, n) == f:
                members.append((t, f, n))
    for g, cs in groups.items():
        if starts.combos:
            cs = [c for c in cs if sfs.combo_of(*c) in starts.combos]  # the measured alone
        for t in sorted({m[0] for m in members}):
            counts = []
            for c in cs:
                counts.append(sum(1 for tt, f, n in members if tt == t and starts.of(c, f) < KOS_BELOW))
            table[t][g] = (min(counts), max(counts)) if counts else None
    unsafe = []
    for s in p["spots"]:
        if starts.combos and sfs.combo_of(*s["combo"]) not in starts.combos:
            continue
        hits = []
        for t, f, n in members:
            if n["world"] != s["world"]:
                continue
            d = math.dist((n["x"], n["y"], n["z"]), (s["x"], s["y"], s["z"]))
            if d <= START_REACH and starts.of(s["combo"], f) < KOS_BELOW:
                hits.append((d, n["name"], f, starts.of(s["combo"], f)))
        if hits:
            unsafe.append((s["combo"], s["name"], sorted(hits)))
    return table, list(groups), unsafe


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-npcfaction"
    n = 2
    while os.path.exists(out):
        out = f"{path}.bak-{when}-{n}-pre-npcfaction"
        n += 1
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def apply(db, p):
    db.execute("BEGIN")
    db.execute(SCHEMA)
    db.execute(LOG_SCHEMA)
    for k, w, x, z, r, f, t, ms in p["rows"]:
        where = "" if r <= 0 else f", spawned within {r:g} of ({x:g}, {z:g})"
        note = f"{t}'s ({len(ms)} NPCs{where})" if not t.startswith("(") else f"a {f} by its name"
        cur = db.execute("""INSERT INTO npcfaction (name, world, x, z, radius, faction, note, source)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)""", (k, w, x, z, r, f, note, SOURCE))
        db.execute(f"INSERT INTO {LOG} (rowid, name, faction) VALUES (?, ?, ?)", (cur.lastrowid, k, f))
    db.execute("COMMIT")


def undo(db):
    done, kept = 0, []
    db.execute("BEGIN")
    for rowid, name, faction in list(db.execute(f"SELECT rowid, name, faction FROM {LOG}")):
        r = db.execute("SELECT name, faction FROM npcfaction WHERE id = ?", (rowid,)).fetchone()
        if r is None:
            continue
        if r != (name, faction):
            kept.append(f"npcfaction row {rowid}: {r[0]} -> {r[1]!r} now, the seed wrote {name} -> {faction!r}")
            continue
        db.execute("DELETE FROM npcfaction WHERE id = ?", (rowid,))
        done += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("COMMIT")
    return done, kept


def report(p, starts, quiet):
    rows = p["rows"]
    per_town = collections.defaultdict(list)
    for row in rows:
        per_town[row[6]].append(row)
    print(f"{len(p['town_world'])} towns of {sum(1 for t in p['people'].values() if t)} people; {len(rows)} rows "
          f"({sum(1 for r in rows if r[1] < 0)} for every NPC of the name, {sum(1 for r in rows if r[1] >= 0)} a place), "
          f"{sum(len(r[7]) for r in rows)} NPCs")
    for t in sorted(p["town_world"], key=lambda t: (p["town_world"][t], t)):
        cit, guard, _ = p["towns"][t]
        rs = per_town.get(t, [])
        folk = sum(len(r[7]) for r in rs if r[7][0]["folk"])
        crit = sum(len(r[7]) for r in rs if not r[7][0]["folk"])
        if not cit:
            print(f"  {t} (world {p['town_world'][t]}): no Citizens faction, left out")
            continue
        print(f"  {t} (world {p['town_world'][t]}): {folk} people, {crit} creatures; {cit}"
              + (f", guards {guard}" if guard else ""))
        if not quiet:
            for k, w, x, z, r, f, _, ms in sorted(rs, key=lambda r: (r[5], r[0])):
                where = "" if w < 0 else f"  [spawned within {r:g} of ({x:g}, {z:g})]"
                kind = "" if ms[0]["folk"] else "  (creature)"
                print(f"      {k!r:34} {f}{kind}{where}")
    named = [r for r in rows if r[6] in {p for p, *_ in NAMED}]
    if named:
        print(f"  named (quests talk to them): {len(named)} names, {sum(len(r[7]) for r in named)} NPCs")
        for k, w, x, z, r, f, t, ms in sorted(named, key=lambda r: (r[6], r[0])):
            print(f"      {k!r:34} {f} (world {w}, {len(ms)}: {t})")
    other = [r for r in rows if r[6].startswith("(")]
    if other:
        print(f"  --creature-factions: {len(other)} names of a kind's own faction")
        if not quiet:
            for k, w, x, z, r, f, _, ms in sorted(other, key=lambda r: (r[5], r[0])):
                print(f"      {k!r:34} {f} ({len(ms)})")
    for why, names in sorted(p["left"].items()):
        names = sorted(set(names))
        print(f"  left ({why}): {len(names)}" + ("" if quiet else ": " + ", ".join(names[:30]) + (" ..." if len(names) > 30 else "")))


def print_impact(table, groups, unsafe, whose):
    print(f"attackable members who attack a new character on sight (standing below -350), by town and race "
          f"(a range over the race's classes; {whose}):")
    w = max([len(t) for t in table] + [10])
    print("  " + " " * w + " " + " ".join(f"{g[:10]:>10}" for g in groups))
    for t in sorted(table):
        cells = []
        for g in groups:
            c = table[t].get(g)
            cells.append(f"{'?' if c is None else c[0] if c[0] == c[1] else f'{c[0]}-{c[1]}':>10}")
        print(f"  {t:<{w}} " + " ".join(cells))
    print_unsafe(unsafe, whose)


def print_unsafe(unsafe, whose):
    if unsafe:
        print(f"{len(unsafe)} start spots where a new character is attacked on sight by a member within "
              f"{START_REACH} ({whose}):")
        for combo, town, hits in unsafe:
            d, name, f, v = hits[0]
            print(f"  {sfs.combo_name(combo)} at {town}: {len(hits)}, the nearest {name.strip()} {d:.0f} off ({f} {v})")
    else:
        print(f"no start spot has a member within {START_REACH} that attacks its new character on sight ({whose})")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="the EQOA-Data checkout (Map/zones.json): the map's towns too")
    ap.add_argument("--measured", help="Sandstorm's measured starts (seed_faction_starts.py's TSV) for the impact")
    ap.add_argument("--apply", action="store_true", help="write the rows")
    ap.add_argument("--undo", action="store_true", help="delete what --apply wrote")
    ap.add_argument("--force", action="store_true", help="--apply where a start spot is in danger")
    ap.add_argument("--creature-factions", action="store_true",
                    help="also give the names of a kind's own faction that faction (they stop attacking on sight)")
    ap.add_argument("--link", type=float, default=60, help="a town's people this near one another are one town")
    ap.add_argument("--reach", type=float, default=400, help="a chain is an anchor's this near it")
    ap.add_argument("--near", type=float, default=40, help="a creature this near (3D) a town's person is among them")
    ap.add_argument("--hull", type=float, default=150, help="the hull of the people this near a creature")
    ap.add_argument("--height", type=float, default=30, help="the hull's people within this of the creature's height")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--quiet", action="store_true", help="the counts, not every name")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    try:
        return run(ap, a, db)
    finally:
        db.close()


def run(ap, a, db):
    db.execute("PRAGMA busy_timeout = 5000")
    logged = db.execute("SELECT name FROM sqlite_master WHERE name = ?", (LOG,)).fetchone()
    if a.undo:
        if not logged:
            print(f"{LOG} is not there: nothing to delete")
            return 0
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        done, kept = undo(db)
        print(f"deleted {done} rows")
        for k in kept:
            print("  left (changed since):", k)
        print("restart the server, or edit an NPC factions row in the GM panel, for the world to read it")
        return 0
    if a.apply and logged:
        ap.error(f"{LOG} is there: this database was seeded already (--undo first)")
    if not db.execute("SELECT COUNT(*) FROM factions").fetchone()[0]:
        sys.exit("the factions table is empty or missing: start the server once (it makes and fills it)")
    measured = None
    if a.measured:
        measured, _ = sfs.read_measured(a.measured, {r[0] for r in db.execute("SELECT name FROM factions")})
    p = plan(db, a.eqoa_data, a.link, a.reach, a.near, a.hull, a.height, a.creature_factions)
    report(p, None, a.quiet)
    starts = Starts(db, measured)
    try:
        gm_rows = [(mv.key(r[0]), r[1], r[2], r[3], r[4], r[5]) for r in db.execute(
            "SELECT name, world, x, z, radius, faction FROM npcfaction WHERE source != ?", (SOURCE,))]
    except sqlite3.OperationalError:
        gm_rows = []
    if measured:
        table, groups, _ = impact(p, starts, gm_rows)
        print_impact(table, groups, _, "Sandstorm's measured starts, '?' none measured yet")
    # the server's own view: the starts as the database holds them
    table, groups, unsafe = impact(p, Starts(db), gm_rows)
    if measured:
        print_unsafe(unsafe, "the starts in the database")
    else:
        print_impact(table, groups, unsafe, "the starts in the database")
    if not a.apply:
        print("(report only: --apply writes them)")
        return 0
    if unsafe and not a.force:
        ap.error(f"{len(unsafe)} start spots are in danger: seed the measured starts first "
                 f"(tools/seed_faction_starts.py), or --force")
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    apply(db, p)
    print(f"wrote {len(p['rows'])} rows; restart the server, or edit an NPC factions row in the GM panel, for the "
          f"world to read them")
    return 0


if __name__ == "__main__":
    sys.exit(main())
