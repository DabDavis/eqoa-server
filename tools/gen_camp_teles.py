#!/usr/bin/env python3
"""EQOA-Data's XP camps as #tele shortcuts: writes internal/handlers/camps.go.

EQOA-Data (github.com/Jadiction/EQOA-Data) marks 403 camps on its map
(Map/zones.json, the pois of category "camp"); the 69 with a note are its
XP-camp guide ("Deathfist Citadel (DFC) 29-34 ... One of the best XP camps in
the low 30s"). CAMPS below names each of those this server can reach, by the
camp's name on the map and the creatures that make it.

A camp's place: its points on EQOA-Data's map, laid over the game by a fit of
the map to this server's NPCs that the map names (every poi or town whose name
is one NPC's: Tunaria and Odus each their own fit, printed), then the camp's
creatures in this server's npcs near it: the one with the most of its kind
within 60 is the anchor, its spawn the place. Where the map's point has none of
them within reach, the nearest such cluster in the world (said, with how far
off the map was). The table holds the anchor's name and spawn (a server whose
npcs lack it lands on that spawn as written), the world and the levels of the
camp's creatures here; ResolveTeleDest takes the NPC of that name nearest the
spawn, so the shortcut follows a GM's move of the camp.

Beside the shortcuts it writes what the GM panel's map says of each camp
(campNotes, handlers/campinfo.go): EQOA-Data's note on it (its "Needs review"
asides left out), the guide's levels, the creatures that make it here (every
name of its kind within CREATURE_REACH of the anchor, as
tools/seed_camp_loot.py binds their loot), and EQOA-Data's rare mobs within
RARE_REACH of it, each with its level here (an NPC of the name) and the drops
EQOA-Data's items name it the source of.

    python3 -E -s tools/gen_camp_teles.py --db eqoa.db \\
        --eqoa-data ../external/EQOA-Data [--out internal/handlers/camps.go]

Without --out it prints the table it would write and every camp's match.
"""

import argparse
import collections
import glob
import json
import math
import os
import re
import shutil
import sqlite3
import subprocess
import sys

# (key, aliases, the camp's name on the map, which of its points (file order,
# None: all), the creatures (a regex on npcs.npc_name, case-insensitive), what
# the list calls it). The keys are what a GM types: one word.
CAMPS = [
    ("dfc", ["deathfist"], "Deathfist Citadel (DFC)", None, r"^deathfist ", "Deathfist Citadel"),
    ("alfarak", [], "Deathfist Slayers & Reavers", None, r"^deathfist (slayer|reaver)", "Al Farak Ruins' Deathfist slayers and reavers"),
    ("hexbones", ["shrieks"], "Hexbones & Shrieks", None, r"^(hexbone|shriek)$", "Zentar's Keep's hexbones and shrieks"),
    ("nasehir", [], "Nasehir", [0], r"^nasehir ", "Northern Ro's Nasehir, the first camp"),
    ("nasehir2", [], "Nasehir", [1], r"^nasehir ", "Northern Ro's Nasehir, the second camp"),
    ("hatebone", ["halasorcs"], "Hatebone Orcs", [0], r"^hatebone ", "Halas' Hatebone orcs"),
    ("hatebone2", [], "Hatebone Orcs", [1], r"^hatebone ", "Halas' Hatebone orcs, the second camp"),
    ("hatebonecave", [], "Hatebone Orc Cave", None, r"^hatebone ", "Halas' Hatebone orc cave"),
    ("nroundead", [], "Undead", [0], r"skeleton|zombie|rotting|decaying|grave rat", "Northern Ro's undead"),
    ("nroundead2", [], "Undead", [1], r"skeleton|zombie|rotting|decaying|grave rat", "Northern Ro's undead, the second camp"),
    ("gharial", [], "Gharial", None, r"^gharial|deep sea ghoul", "the gharial island south of Moradhim"),
    ("tendora", ["kerrans"], "Tendora-Buki", None, r"^kerran |kolbok", "Tendora-Buki, West Toxulia's kerran village"),
    ("kappa", ["slythis"], "Kappa", None, r"^kappa (guard|slave|worker|shaman|brute)", "Slythis Dwell's kappa"),
    ("kappawarriors", [], "Kappa Warrior", None, r"^kappa warrior", "Slythis Dwell's kappa warriors"),
    ("kappaelites", [], "Kappa Elites", None, r"^kappa elite", "Slythis Dwell's kappa elites"),
    ("drudges", [], "Kappa Drudge", None, r"^kappa drudge", "the East Plateau's kappa drudges"),
    ("phantoms", [], "Phantoms", None, r"^phantom$", "the East Plateau's phantoms"),
    ("gargoyles", [], "Gargoyle", None, r"gargoyle", "the South Barren Coast's gargoyles"),
    ("harpies", [], "Harpy", None, r"harpy", "Erud's Crossing's harpies"),
    ("moots", [], "Moots", None, r"moot", "Erud's Crossing's moots"),
    ("blackburrow", ["bbgnolls"], "Gnolls", None, r"^gnoll ", "Blackburrow's gnolls"),
    ("bloodfoots", [], "Bloodfoots", None, r"^bloodfoot ", "Mt. Hatespike's Bloodfoots"),
    # the map's other two Coldpaw points have no gnolls here
    ("coldpaw", [], "Coldpaw Gnolls", [2], r"^coldpaw ", "Klik'Anon's Coldpaw gnolls"),
    ("hillgiants", [], "Hill Giants", None, r"^hill giant", "Keliner's hill giants"),
    ("serpents", [], "Serpent Guardians", None, r"^serpent (guardian|king)", "the Bandit Hills' serpent guardians"),
    ("cutthroats", ["bandits"], "Cutthroats & Bandits", [0], r"^(cutthroat|bandit)$", "Mulin's Reach's cutthroats and bandits"),
    ("cutthroats2", [], "Cutthroats & Bandits", [1, 2, 3], r"^(cutthroat|bandit)$", "Mulin's Reach's cutthroats and bandits, the second camp"),
    ("plainstriders", [], "Orc Plainstriders", None, r"^orc plains", "Mulin's Reach's orc plainstriders"),
    ("plainsraiders", [], "Orc Plainsraiders", None, r"^orc plains", "Mulin's Reach's orc plainsraiders"),
    ("cracktooth", [], "Cracktooth Orcs", None, r"cracktooth", "Bobble-by-Water's Cracktooth orcs"),
    ("cadavers", [], "Druidic Cadavers", None, r"^druidic cadaver", "the druidic cadavers west of Qeynos"),
    ("slesher", [], "Slesher Gnoll Camp", None, r"^slesher |gnoll", "Surefall Glade's Slesher gnolls"),
    ("aviaks", [], "Aviak Outpost", None, r"^aviak ", "the aviak outpost near Halas"),
    ("orctunnel", ["druk"], "Orc Tunnel", None, r"^orc (brute|henchman|prophet)", "the Druk's orc tunnel"),
    ("gnollfishers", [], "Gnoll Fishermen", None, r"^gnoll fisherman", "the gnoll fishermen"),
    ("scarecrows", [], "Scarecrows", None, r"^scarecrow", "Alseop's Wall's scarecrows"),
    ("goblincitadel", [], "Goblin Citadel", None, r"^goblin ", "the Goblin Citadel near Runneye"),
    ("klickpirates", [], "Pirates of Klick", None, r"pirate|deckhand", "the Klik'Anon coast's pirates"),
    ("cyclops", [], "Cyclops Fortress", None, r"^cyclops", "the cyclops"),
    ("siloths", [], "Siloths", None, r"^siloth ", "the Salt Mine's siloths"),
    ("icegiants", [], "Ice Giant Refugees", None, r"^ice giant", "the ice giant refugees near Northgate"),
    ("ghosts", [], "Ghosts", None, r"^wandering ghost$", "the wandering ghosts"),
    ("xrag", ["pariah"], "Pariah of Xragg", None, r"xrag|^sphere of annihilation", "the Elephant Graveyard's children of Xrag"),
    ("tombrats", [], "Tomb Rats", None, r"^(tomb|grim) rat", "the tomb rats"),
    ("gusts", ["icywind"], "Icy wind & freezing gusts", None, r"^freezing gust|^frost elemental|^icy wind", "the freezing gusts"),
    ("guardianforest", [], "Goblins", None, r"^goblin (assassin|soothsayer)", "Guardian Forest's goblins"),
    ("freezeblood", [], "Freezeblood Goblins", None, r"^freezeblood ", "Everfrost's Freezeblood goblins"),
    ("crocodiles", [], "Saltwater Crocodiles", None, r"crocodile|caiman", "Bobble-by-Water's saltwater crocodiles"),
    ("minotaurs", [], "Rustel Minotaur", None, r"^rustel minotaur", "the Gerntar Mines' Rustel minotaurs"),
]

# The worlds a camp may be in: Tunaria (0), whose sheet the map is, and Odus
# (2), drawn on the same sheet to its southwest.
FIT_WORLDS = (0, 2)
# How far from the map's point the camp's creatures are looked for, by world
# (Odus's fit is the looser), and the cluster a creature counts in. Where the
# fit is close (Tunaria's half-dozen units) the camp is the dense cluster
# nearest the map's point; where it is loose (Odus's), the densest in reach.
CLOSE_FIT = 50.0
REACH = {0: 400.0, 2: 900.0}
CLUSTER = 60.0
LEVELS_WITHIN = 120.0
# the camp's creatures for its notes (seed_camp_loot.py's CAMP_REACH), and
# how far from the anchor a rare mob counts as the camp's
CREATURE_REACH = 150.0
RARE_REACH = 700.0
# a town's world: the one whose fit puts its point nearest one of this
# server's NPCs, within TOWN_REACH (Odus's fit is ~160 off); Tunaria's towns
# are coachtowns.go's
TOWN_REACH = 400.0


def clean(name):
    return re.sub(r"\s*\([^)]*\)\s*", " ", name).strip().lower()


def fit(points):
    """gx = a*mx + b, gz = c*my + d by least squares, dropping outliers."""
    def line(xs, ys):
        n = len(xs)
        mx, my = sum(xs) / n, sum(ys) / n
        sxx = sum((x - mx) ** 2 for x in xs)
        sxy = sum((x - mx) * (y - my) for x, y in zip(xs, ys))
        a = sxy / sxx
        return a, my - a * mx

    cur = points
    for _ in range(6):
        a, b = line([p[0] for p in cur], [p[2] for p in cur])
        c, d = line([p[1] for p in cur], [p[3] for p in cur])
        res = [math.hypot(a * p[0] + b - p[2], c * p[1] + d - p[3]) for p in cur]
        med = sorted(res)[len(res) // 2]
        keep = [p for p, r in zip(cur, res) if r <= max(3 * med, 150)]
        if len(keep) == len(cur):
            break
        cur = keep
    res = sorted(math.hypot(a * p[0] + b - p[2], c * p[1] + d - p[3]) for p in cur)
    return (a, b, c, d), len(cur), len(points), res[len(res) // 2]


def drops_by_mob(data):
    """EQOA-Data's items by the creature it names their source (not a loot
    table's), lower case: {mob: [item, ...]}."""
    out = collections.defaultdict(list)
    for path in sorted(glob.glob(os.path.join(data, "Information", "databases", "items", "*.json"))):
        doc = json.load(open(path))
        items = doc["items"] if isinstance(doc, dict) and "items" in doc else doc
        for it in items:
            src = it.get("source")
            for s in src if isinstance(src, list) else ([src] if src else []):
                if s.get("type") == "mobDrop" and "loot table" not in s.get("name", "").lower():
                    name = re.sub(r"'s$", "", s["name"].strip()).lower()
                    name = re.sub(r"^the ", "", name)
                    if it["name"] not in out[name]:
                        out[name].append(it["name"])
    return out


def note_of(camps):
    """EQOA-Data's note on the camp, its "Needs review: ..." asides (the
    guide's own editing) left out."""
    note = camps[0].get("note", "")
    note = re.sub(r"\s*Needs review:.*$", "", note).strip()
    return note


def load(db, data):
    zones = json.load(open(os.path.join(data, "Map", "zones.json")))
    rows = db.execute(
        "SELECT id, npc_name, x_coord, y_coord, z_coord, world, npc_level FROM npcs WHERE npc_name IS NOT NULL"
    ).fetchall()
    return zones, rows


def fits(zones, rows):
    byname = collections.defaultdict(list)
    for r in rows:
        byname[r[1].strip().lower()].append(r)
    pairs = collections.defaultdict(list)
    for p in zones["pois"] + zones["towns"]:
        c = byname.get(clean(p["name"]))
        if c and len(c) == 1:
            pairs[c[0][5]].append((p["x"], p["y"], c[0][2], c[0][4]))
    out = {}
    for w in FIT_WORLDS:
        out[w] = fit(pairs[w])
    return out


def camp_heart(kind, cands, close, gx, gz):
    """The heart of a camp nearest the map's point (gx, gz): of the creatures
    cands with at least half the most company any of them has within CLUSTER
    of the camp's kind (and three; the most, where the fit is loose), the
    nearest; then, of the commonest name within CLUSTER of it (not its named
    one), the one of most company."""
    def company(r):
        return sum(1 for o in kind if math.hypot(o[2] - r[2], o[4] - r[4]) <= CLUSTER)
    dense = [(company(r), r) for r in cands]
    top = max(n for n, _ in dense)
    floor = max(3, top // 2) if close else top
    busy = [r for n, r in dense if n >= floor] or [r for _, r in dense]
    near = min(busy, key=lambda r: (math.hypot(r[2] - gx, r[4] - gz), r[0]))
    local = [r for r in cands if math.hypot(r[2] - near[2], r[4] - near[4]) <= CLUSTER]
    common = collections.Counter(r[1].strip().lower() for r in local).most_common(1)[0][0]
    local = [r for r in local if r[1].strip().lower() == common]
    return min(local, key=lambda r: (-company(r), math.hypot(r[2] - near[2], r[4] - near[4]), r[0]))


def place(spec, zones, rows, fitted):
    key, aliases, poi_name, which, pattern, label = spec
    # the guide's own points: the camps of that name with a note (the map has
    # other camps of the same name without one: "Goblins" fourteen times)
    camps = [p for p in zones["pois"] if p["category"] == "camp" and p["name"] == poi_name and p.get("note")]
    if which is not None:
        camps = [camps[i] for i in which if i < len(camps)]
    if not camps:
        return None, "no camp of that name on the map"
    mx = sum(p["x"] for p in camps) / len(camps)
    my = sum(p["y"] for p in camps) / len(camps)
    rx = re.compile(pattern, re.I)
    best = None
    for w in FIT_WORLDS:
        (a, b, c, d), _, _, med = fitted[w]
        gx, gz = a * mx + b, c * my + d
        close = med <= CLOSE_FIT
        kind = [r for r in rows if r[5] == w and rx.search(r[1].strip())]
        if not kind:
            continue

        def heart(cands):
            return camp_heart(kind, cands, close, gx, gz)

        near = [r for r in kind if math.hypot(r[2] - gx, r[4] - gz) <= REACH[w]]
        if near:
            anchor, off = heart(near), 0.0
        else:
            # the map's point is off: the nearest cluster of the kind
            clusters = [r for r in kind if sum(1 for o in kind if math.hypot(o[2] - r[2], o[4] - r[4]) <= CLUSTER) >= 3]
            if not clusters:
                continue
            nearest = min(clusters, key=lambda r: math.hypot(r[2] - gx, r[4] - gz))
            local = [r for r in clusters if math.hypot(r[2] - nearest[2], r[4] - nearest[4]) <= CLUSTER * 2]
            anchor = heart(local)
            off = math.hypot(anchor[2] - gx, anchor[4] - gz)
        around = [o for o in kind if math.hypot(o[2] - anchor[2], o[4] - anchor[4]) <= LEVELS_WITHIN]
        cand = (off, w, anchor, around, (gx, gz))
        if best is None or cand[0] < best[0]:
            best = cand
    if best is None:
        return None, "no creature of the camp in this server's npcs"
    off, w, anchor, around, point = best
    if len(around) < 3:
        return None, f"only {len(around)} of the camp's creatures here"
    lv = [o[6] for o in around if o[6]]
    levels = f"{min(lv)}-{max(lv)}" if lv and min(lv) != max(lv) else (str(lv[0]) if lv else "")
    kinds = collections.Counter(o[1].strip() for o in around)
    # every name of the camp's kind within CREATURE_REACH (seed_camp_loot.py's
    # binding), as the npcs spell it most, the commonest first
    spell = collections.defaultdict(collections.Counter)
    for o in rows:
        if o[5] == w and rx.search(o[1].strip()) and math.hypot(o[2] - anchor[2], o[4] - anchor[4]) <= CREATURE_REACH:
            spell[o[1].strip().lower()][o[1].strip()] += 1
    creatures = [c.most_common(1)[0][0] for _, c in sorted(spell.items(), key=lambda kv: (-sum(kv[1].values()), kv[0]))]
    return {
        # the anchor's name exactly as npcs has it (a trailing space included:
        # "Gharial "), for the World matches it so
        "key": key, "aliases": aliases, "label": label, "npc": anchor[1],
        "x": anchor[2], "y": anchor[3], "z": anchor[4], "world": w, "levels": levels,
        "count": len(around), "kinds": kinds.most_common(4), "off": off, "point": point,
        "guide": camps[0].get("levelRange", ""), "note": note_of(camps), "creatures": creatures,
    }, ""


def other_towns(zones, rows, fitted):
    """EQOA-Data's towns that are not Tunaria's (coachtowns.go's coachTowns
    hold those, their names fixed there): each in the world whose fit puts
    it nearest one of this server's NPCs, by that fit; a name the map gives
    twice told apart by its alignment ("Stone of Morathals (Good)")."""
    names = collections.Counter(t["name"] for t in zones["towns"])
    out = []
    for t in zones["towns"]:
        best = None
        for w in FIT_WORLDS:
            (a, b, c, d), _, _, _ = fitted[w]
            gx, gz = a * t["x"] + b, c * t["y"] + d
            near = min((math.hypot(r[2] - gx, r[4] - gz) for r in rows if r[5] == w), default=math.inf)
            if near <= TOWN_REACH and (best is None or near < best[0]):
                best = (near, w, gx, gz)
        if best is None or best[1] == 0:
            continue
        name = t["name"]
        if names[name] > 1 and t.get("alignment"):
            name = f"{name} ({t['alignment'].capitalize()})"
        out.append({"name": name, "world": best[1], "x": best[2], "z": best[3]})
    return out


def rare_near(placed, zones, rows, fitted, drops):
    """EQOA-Data's rare mobs within RARE_REACH of each camp: where this
    server's npcs hold one of the name (in the camp's world, the nearest),
    its spawn and level, else the map's point by the world's fit; with the
    drops EQOA-Data names it the source of."""
    byname = collections.defaultdict(list)
    for r in rows:
        byname[r[1].strip().lower()].append(r)
    rares = [p for p in zones["pois"] if p["category"] == "rare-mob"]
    for c in placed:
        found = []
        for p in rares:
            name = clean(p["name"])
            (a, b, cc, d), _, _, _ = fitted[c["world"]]
            gx, gz = a * p["x"] + b, cc * p["y"] + d
            ours = [r for r in byname.get(name, []) if r[5] == c["world"]]
            level = 0
            if ours:
                r = min(ours, key=lambda r: math.hypot(r[2] - c["x"], r[4] - c["z"]))
                gx, gz, level = r[2], r[4], r[6] or 0
            dist = math.hypot(gx - c["x"], gz - c["z"])
            if dist <= RARE_REACH:
                shown = ours[0][1].strip() if ours else re.sub(r"\s*\([^)]*\)\s*", " ", p["name"]).strip()
                found.append((dist, shown, level, drops.get(name, [])))
        c["rare"] = [{"name": n, "level": lv, "drops": dr} for _, n, lv, dr in sorted(found)]


# ---- the map's other camps: the map's, not #tele's ----
#
# EQOA-Data's map has 334 camps more, without a note: a name ("Goblins",
# "Taksehir Miners") and a level range that is "1-1" for nearly all (no level
# written: left out). Each this server really has is written to
# internal/handlers/mapcamps.go (--map-out), for the maps alone (the GM panel's
# map, vanilla's): never a #tele shortcut or a Teleporter's camp, and camps.go
# is not rewritten (its 49 stay as they were placed). A camp: the points of a
# name within GROUP of each other on the game's ground (by the fits below) are
# one; its creatures are those MAP_KINDS names for it, else the npcs whose names
# hold every word of its own (plurals made singular, "camp", "cave" and the
# like left out); its world is its nearest zone's on the map (Odus's and
# Rathe's zones carry no biomes: Odus's lie west of 3700); and it is anchored as
# a #tele camp is (camp_heart), but only on creatures within MAP_REACH of its
# point (no search of the whole world: a "Goblins" there is not another
# goblin camp's). A camp left out (none of its kind in reach, under three) is
# printed with why; one whose anchor is within SAME of a #tele camp's (camps.go)
# or of another's and of that camp's kind is that camp (the bigger kept). A name that does not tell
# a camp apart (GENERIC, or one more camps here have) takes its zone on the map
# ("Goblins (Guardian Forest)"), and then a side ("Goblins (Snowfist, north)").

MAP_WORLDS = (0, 1, 2)
# Rathe's fit is ~75 off; Odus's ~160
MAP_REACH = {0: 400.0, 1: 500.0, 2: 900.0}
MAP_CLOSE = {0: True, 1: False, 2: False}
GROUP = 400.0
# a camp is another's (a #tele camp's, or one placed already) when its anchor
# is within SAME of that one's and of that camp's kind
SAME = 300.0
# words of a camp's name that are not its creatures'
STOP = {"camp", "cave", "tunnel", "underground", "outpost", "hive", "and", "the", "of", "village", "tower", "tomb", "fortress", "citadel"}
# names that say what, not where
GENERIC = {"Undead", "Orcs", "Goblins", "Gnolls", "Bandits", "Bandit", "Skeletons", "Spirits", "Treant", "Kobold",
           "Aviak", "Aviaks", "Centaurs", "Faeries", "Frogloks", "Lizardman", "Lizardmen", "Mermen", "Ratman",
           "Cyclops", "Antlion", "Crocodiles", "Ghosts", "Siloths", "Werewolfs", "Nixie"}
# a camp's creatures where its name's words are not their names' (a regex on
# npcs.npc_name, any case), by the camp's name on the map
UNDEAD = r"undead|skelet|zombie|ghoul|mummy|wraith|spectre|ghost|ghast|rotting|decaying|cadaver|bones?$|lich\b"
MAP_KINDS = {
    # the map's word is the race, the npcs' the clan's ranks
    "Frosteye Orcs": r"^frosteye ", "Frosteye orcs": r"^frosteye ",
    "Wktaan Orcs": r"^wktaan ",
    "Freezeblood goblins": r"^freezeblood ", "Freezeblood Goblins": r"^freezeblood ", "Freezeblood goblin cave": r"^freezeblood ",
    "Snowfist Goblins": r"^snowfist ", "Snowfist goblins": r"^snowfist ", "Snowfist goblins cave": r"^snowfist ",
    "Snowfist Guards": r"^snowfist (guard|defender|warrior)",
    "Grimrik Trolls": r"^grimrik ",
    "Glinaldal Druids & Rangers": r"^glinaldal ",
    "Undead Druid & Rangers": r"^undead (druid|ranger)",
    "Lesser ghouls & skeletons": r"^(lesser ghoul|skeleton)$",
    "Treant Defender/Diviner": r"^treant (defender|diviner|elder)",
    "Bone Hunters": r"^(bone hunter|flesh trampler)", "Bone hunter & Flesh Trampler": r"^(bone hunter|flesh trampler)",
    "Flesh Trampler": r"^(bone hunter|flesh trampler)",
    "Deathfist Legionaires": r"^deathfist legionn?aire",
    "Deathfist Slayers": r"^deathfist (slayer|reaver|prophet)",
    "Wasp workerss": r"^wasp ", "Wasp Harvesters": r"^wasp ",
    "Wandering Spectres": r"^wandering (ghost|ghast|spectre)",
    "Frost & ice elementals": r"^(freezing gust|frost elemental|icy wind|guardian wind)", "Wind guardian": r"^(freezing gust|frost elemental|icy wind|guardian wind)",
    "Young Crabs": r"crab$",
    # the Elemental Towers: the titans, and each element's own
    "Elemental Titans": r"titan$|behemoth$",
    "Unstable Elemental (earth)": r"^(rock|earth) elemental|^crag titan", "Unstable Elemental (air)": r"^(wind|air) elemental|^cyclone behemoth",
    "Unstable Elemental (fire)": r"^(flame|fire) elemental|^lava titan", "Unstable Elemental (water)": r"^water elemental",
    # the map spells Regal "Ragel", and Teir'dal three ways
    "Ragel Aviaks": r"^regal (guard|priest)|aviak$", "Ragel High Aviaks": r"^regal high", "Elite Aviak": r"aviak$|^regal ", "Elite Aviaks": r"aviak$|^regal ",
    "Teir'Dal Looter": r"^t(ie|ei)r.dal looter", "Teir'Dal Eliter Dragoon": r"^t(ie|ei)r.dal (elite )?dragoon",
    "Teir'dal": r"^t(ie|ei)r.dal ", "Tier'dal": r"^t(ie|ei)r.dal ",
    "Toltem centaurs": r"centaur",
    "Rotbones": r"^(rotbone|darkbone)",
    "Jaeger Mansion": r"^(charbone|spectre|terror)$",
    "Undead Village": r"^(raised villager|undead farmer|enraged village guard|enraged farmer)$",
    "Desert Bandit": r"bandit|^nasehir (rogue|warrior)",
    # the map's undead: theirs of every kind
    "Undead": UNDEAD, "Undead Tomb": UNDEAD, "Undead Tower": UNDEAD, "Undead Minions": UNDEAD, "Undead Skeletons": r"skelet",
    "Skeletons": r"skelet", "Spirits": r"spirit|spectre|ghost",
}


# the map's typing slips, in the name a camp is shown by (its Map keeps it)
MAP_SPELLING = {"Wasp workerss": "Wasp Workers"}


def singular(w):
    if w.endswith("men"):
        return w[:-3] + "man"
    if w.endswith("ies"):
        return w[:-1]
    if w.endswith("s") and not w.endswith("ss") and len(w) > 3:
        return w[:-1]
    return w


def auto_kind(name):
    """A regex for the npcs a camp's name names: every word of it (singular,
    STOP left out) at a word's start, an apostrophe matching any or none."""
    words = [singular(w) for w in re.findall(r"[a-z'`]+", re.sub(r"\([^)]*\)", " ", name.lower())) if w not in STOP]
    words = [w for w in words if w.strip("'`")]
    if not words:
        return None
    parts = ["(?=.*\\b" + "['`]?".join(re.escape(x) for x in re.split(r"['`]", w.strip("'`"))) + ")" for w in words]
    return "^" + "".join(parts)


def zone_of(zones, mx, my):
    """The map's nearest zone to a point and its world: Odus's and Rathe's
    zones have no biomes, Odus's west of 3700."""
    z = min(zones["zones"], key=lambda q: math.hypot(q["x"] - mx, q["y"] - my))
    if z.get("biomes"):
        return z["name"], 0
    return z["name"], (2 if z["x"] < 3700 else 1)


def map_fits(zones, rows):
    """fits()' fit for every world of MAP_WORLDS (Rathe's too)."""
    byname = collections.defaultdict(list)
    for r in rows:
        byname[r[1].strip().lower()].append(r)
    pairs = collections.defaultdict(list)
    for p in zones["pois"] + zones["towns"]:
        c = byname.get(clean(p["name"]))
        if c and len(c) == 1:
            pairs[c[0][5]].append((p["x"], p["y"], c[0][2], c[0][4]))
    return {w: fit(pairs[w]) for w in MAP_WORLDS}


def tele_camps(path):
    """camps.go's campDests as written: key -> (anchor name, x, z, world)."""
    out = {}
    for m in re.finditer(r'"(\w+)":\s*\{NPC: "([^"]*)", Near: true, X: ([-\d.]+), Y: [-\d.]+, Z: ([-\d.]+), World: (\d+),', open(path).read()):
        out[m.group(1)] = (m.group(2).strip().lower(), float(m.group(3)), float(m.group(4)), int(m.group(5)))
    return out


def levels_of(around):
    lv = [o[6] for o in around if o[6]]
    return f"{min(lv)}-{max(lv)}" if lv and min(lv) != max(lv) else (str(lv[0]) if lv else "")


def map_camps(zones, rows, fitted, tele):
    """The map's camps without a note this server has: (placed, left out)."""
    tele_kind = {spec[0]: re.compile(spec[4], re.I) for spec in CAMPS}
    pts = [p for p in zones["pois"] if p["category"] == "camp" and not p.get("note")]
    # the points of a name (any case) near each other on the ground: one camp
    groups = []
    for low in sorted({p["name"].lower() for p in pts}):
        spelled = [q["name"] for q in pts if q["name"].lower() == low]
        # the spelling most points have, capitals first ("Freezeblood Goblins")
        name = sorted(collections.Counter(spelled).items(), key=lambda kv: (-kv[1], kv[0]))[0][0]
        mine = []
        for p in (q for q in pts if q["name"].lower() == low):
            zone, w = zone_of(zones, p["x"], p["y"])
            (a, b, c, d), _, _, _ = fitted[w]
            mine.append((p, w, zone, a * p["x"] + b, c * p["y"] + d))
        parts = []
        for m in mine:
            hit = [g for g in parts if any(o[1] == m[1] and math.hypot(o[3] - m[3], o[4] - m[4]) <= GROUP for o in g)]
            merged = [m]
            for g in hit:
                merged += g
                parts.remove(g)
            parts.append(merged)
        for g in parts:
            groups.append((name, g))
    cands, left = [], []
    for name, g in groups:
        w = g[0][1]
        mx = sum(m[0]["x"] for m in g) / len(g)
        my = sum(m[0]["y"] for m in g) / len(g)
        zone, _ = zone_of(zones, mx, my)
        (a, b, c, d), _, _, _ = fitted[w]
        gx, gz = a * mx + b, c * my + d
        pattern = MAP_KINDS.get(name) or auto_kind(name)
        where = f"{name} ({zone}, w{w}, {len(g)} point{'s' if len(g) > 1 else ''})"
        if not pattern:
            left.append((where, "no creature named"))
            continue
        rx = re.compile(pattern, re.I)
        kind = [r for r in rows if r[5] == w and rx.search(r[1].strip())]
        if not kind:
            left.append((where, f"no creature of its kind ({pattern}) in world {w}"))
            continue
        near = [r for r in kind if math.hypot(r[2] - gx, r[4] - gz) <= MAP_REACH[w]]
        if not near:
            off = min(math.hypot(r[2] - gx, r[4] - gz) for r in kind)
            left.append((where, f"its kind ({pattern}) is {off:.0f} off its point, past {MAP_REACH[w]:.0f}"))
            continue
        anchor = camp_heart(kind, near, MAP_CLOSE[w], gx, gz)
        around = [o for o in kind if math.hypot(o[2] - anchor[2], o[4] - anchor[4]) <= LEVELS_WITHIN]
        if len(around) < 3:
            left.append((where, f"only {len(around)} of its creatures ({pattern}) here"))
            continue
        same = [k for k, (an, x, z, tw) in tele.items() if tw == w and math.hypot(x - anchor[2], z - anchor[4]) <= SAME
                and k in tele_kind and (tele_kind[k].search(anchor[1].strip()) or rx.search(an))]
        if same:
            left.append((where, f"the #tele camp {same[0]}"))
            continue
        spell = collections.defaultdict(collections.Counter)
        for o in kind:
            if math.hypot(o[2] - anchor[2], o[4] - anchor[4]) <= CREATURE_REACH:
                spell[o[1].strip().lower()][o[1].strip()] += 1
        creatures = [cc.most_common(1)[0][0] for _, cc in sorted(spell.items(), key=lambda kv: (-sum(kv[1].values()), kv[0]))]
        ranges = sorted({m[0]["levelRange"] for m in g if m[0].get("levelRange") and m[0]["levelRange"] != "1-1"})
        cands.append({"name": MAP_SPELLING.get(name, name), "names": [name], "zone": zone, "world": w, "npc": anchor[1], "x": anchor[2], "y": anchor[3],
                      "z": anchor[4], "levels": levels_of(around), "ranges": ranges, "count": len(around), "creatures": creatures,
                      "mx": mx, "my": my, "points": len(g), "point": (gx, gz), "rx": rx, "where": where,
                      "kinds": collections.Counter(o[1].strip() for o in around).most_common(4)})
    # one camp under several names (the map's "Snowfist Guards" in its
    # "Snowfist Goblins"): within SAME, either's anchor of the other's kind;
    # the bigger (more creatures, then nearer its own point) keeps its place,
    # its creatures and its name, the others' names kept as aliases (Map)
    parent = list(range(len(cands)))

    def root(i):
        while parent[i] != i:
            parent[i] = parent[parent[i]]
            i = parent[i]
        return i
    for i, p in enumerate(cands):
        for j in range(i):
            q = cands[j]
            if p["world"] == q["world"] and math.hypot(p["x"] - q["x"], p["z"] - q["z"]) <= SAME \
                    and (p["rx"].search(q["npc"].strip()) or q["rx"].search(p["npc"].strip())):
                parent[root(i)] = root(j)
    sets = collections.defaultdict(list)
    for i, p in enumerate(cands):
        sets[root(i)].append(p)
    placed = []
    for members in sets.values():
        best = min(members, key=lambda p: (-p["count"], math.hypot(p["x"] - p["point"][0], p["z"] - p["point"][1]), p["name"]))
        for p in members:
            if p is not best:
                left.append((p["where"], f"the same camp as {best['name']} ({best['zone']})"))
                for n in p["names"]:
                    if n.lower() not in {x.lower() for x in best["names"]}:
                        best["names"].append(n)
        ranges = sorted({r for p in members for r in p["ranges"]})
        best["guide"] = ""
        if ranges:
            lo = min(int(r.split("-")[0]) for r in ranges)
            hi = max(int(r.split("-")[-1]) for r in ranges)
            best["guide"] = f"{lo}-{hi}" if lo != hi else str(lo)
        best["map"] = " / ".join(best["names"])
        placed.append(best)
    # names: the map's, its zone added where that does not tell the camp
    # apart (a GENERIC name, one more camps here have, a #tele camp's), then a
    # side of the others of its name, +z being south
    taken = {spec[2].lower() for spec in CAMPS}
    count = collections.Counter(p["name"].lower() for p in placed)
    for p in placed:
        if (p["name"] in GENERIC or count[p["name"].lower()] > 1 or p["name"].lower() in taken) and p["zone"].lower() not in p["name"].lower():
            p["name"] = f"{p['name']} ({p['zone']})"
    by = collections.defaultdict(list)
    for p in placed:
        by[p["name"].lower()].append(p)
    for ps in by.values():
        if len(ps) < 2:
            continue
        cx = sum(p["x"] for p in ps) / len(ps)
        cz = sum(p["z"] for p in ps) / len(ps)
        for p in ps:
            dx, dz = p["x"] - cx, p["z"] - cz
            p["side"] = ("east" if dx > 0 else "west") if abs(dx) >= abs(dz) else ("south" if dz > 0 else "north")
        sides = collections.Counter(p["side"] for p in ps)
        n = collections.Counter()
        for p in sorted(ps, key=lambda p: (p["z"], p["x"])):
            tag = p["side"]
            if sides[tag] > 1:
                n[tag] += 1
                tag = f"{tag} {n[tag]}"
            p["name"] = p["name"][:-1] + ", " + tag + ")" if p["name"].endswith(")") else f"{p['name']} ({tag})"
    keys = set(tele) | {a for spec in CAMPS for a in spec[1]}
    for p in sorted(placed, key=lambda p: p["name"]):
        k = re.sub(r"[^a-z0-9]", "", p["name"].lower())
        base, i = k, 2
        while k in keys:
            k, i = f"{base}{i}", i + 1
        keys.add(k)
        p["key"] = k
        p["label"] = p["name"] if p["zone"].lower() in p["name"].lower() else f"{p['name']}, {p['zone']}"
    placed.sort(key=lambda p: p["key"])
    return placed, left


def render_map(placed):
    lines = [
        "// Code written by tools/gen_camp_teles.py --map-out; DO NOT EDIT: change",
        "// the tool and run it again.",
        "",
        "package handlers",
        "",
        "// mapCamps are EQOA-Data's map's other camps (those without a note) that",
        "// this server has, for the maps alone (the GM panel's, vanilla's): never a",
        "// #tele shortcut or a Teleporter camp (campDests, camps.go). Name is the",
        "// map's, its zone added where it would not tell the camp apart; Guide its",
        "// level range where the map writes one; Levels the creatures' here; MapX",
        "// and MapY its point on EQOA-Data's map.",
        "var mapCamps = map[string]mapCamp{",
    ]
    for p in placed:
        rare = ", ".join(
            f'{{Name: {go_string(r["name"])}, Level: {r["level"]}, Drops: []string{{{", ".join(go_string(x) for x in r["drops"])}}}}}'
            for r in p.get("rare", []))
        lines.append(
            f'\t{go_string(p["key"])}: {{Name: {go_string(p["name"])}, Map: {go_string(p["map"])}, Zone: {go_string(p["zone"])}, '
            f'Label: {go_string(p["label"])}, NPC: {go_string(p["npc"])}, '
            f'X: {p["x"]:.1f}, Y: {p["y"]:.1f}, Z: {p["z"]:.1f}, World: {p["world"]}, '
            f'Levels: {go_string(p["levels"])}, Guide: {go_string(p["guide"])}, MapX: {p["mx"]:.0f}, MapY: {p["my"]:.0f}, '
            f'Creatures: []string{{{", ".join(go_string(x) for x in p["creatures"])}}}, Rare: []campRare{{{rare}}}}},'
        )
    lines.append("}")
    return "\n".join(lines) + "\n"


def go_string(s):
    return json.dumps(s, ensure_ascii=False)


def render(placed, towns=()):
    lines = [
        "// Code written by tools/gen_camp_teles.py; DO NOT EDIT: change the tool",
        "// and run it again.",
        "",
        "package handlers",
        "",
        "// campDests are EQOA-Data's XP camps (its map's camps with a note, its",
        "// XP-camp guide), #tele shortcuts anchored on one of each camp's creatures",
        "// in the server's npcs: the one of that name nearest the spawn written here,",
        "// which is where a server without it lands. Levels are the camp's creatures'",
        "// here, not the guide's.",
        "var campDests = map[string]teleDest{",
    ]
    for p in placed:
        lines.append(
            f'\t{go_string(p["key"])}: {{NPC: {go_string(p["npc"])}, Near: true, '
            f'X: {p["x"]:.1f}, Y: {p["y"]:.1f}, Z: {p["z"]:.1f}, World: {p["world"]}, '
            f'Levels: {go_string(p["levels"])}, Label: {go_string(p["label"])}}},'
        )
    lines.append("}")
    lines.append("")
    lines.append("// campNotes are what the GM panel's map says of each camp (campinfo.go):")
    lines.append("// EQOA-Data's note on it, the guide's levels, the creatures that make it")
    lines.append("// here and EQOA-Data's rare mobs near it, with their levels here and the")
    lines.append("// drops EQOA-Data names them the source of.")
    lines.append("var campNotes = map[string]campNote{")
    for p in placed:
        rare = ", ".join(
            f'{{Name: {go_string(r["name"])}, Level: {r["level"]}, Drops: []string{{{", ".join(go_string(x) for x in r["drops"])}}}}}'
            for r in p.get("rare", []))
        lines.append(
            f'\t{go_string(p["key"])}: {{Note: {go_string(p["note"])}, Guide: {go_string(p["guide"])}, '
            f'Creatures: []string{{{", ".join(go_string(x) for x in p["creatures"])}}}, '
            f'Rare: []campRare{{{rare}}}}},'
        )
    lines.append("}")
    lines.append("")
    lines.append("// mapTowns are EQOA-Data's towns outside Tunaria (whose are coachtowns.go's),")
    lines.append("// by its map's fit of their world, for the GM panel's map (campinfo.go).")
    lines.append("var mapTowns = []TownMark{")
    for t in towns:
        lines.append(f'\t{{Name: {go_string(t["name"])}, World: {t["world"]}, X: {t["x"]:.0f}, Z: {t["z"]:.0f}}},')
    lines.append("}")
    lines.append("")
    lines.append("// campAliases are other names a GM may type for a camp.")
    lines.append("var campAliases = map[string]string{")
    for p in placed:
        for a in p["aliases"]:
            lines.append(f'\t{go_string(a)}: {go_string(p["key"])},')
    lines.append("}")
    return "\n".join(lines) + "\n"


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--eqoa-data", required=True)
    ap.add_argument("--out")
    ap.add_argument("--map-out", help="write the map's other camps (mapcamps.go) here; camps.go is read, not written, unless --out")
    ap.add_argument("--camps", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "internal", "handlers", "camps.go"),
                    help="the #tele camps as written (camps.go), which the map's camps leave to them")
    args = ap.parse_args()
    db = sqlite3.connect(f"file:{args.db}?mode=ro", uri=True)
    zones, rows = load(db, args.eqoa_data)
    fitted = fits(zones, rows)
    for w, ((a, b, c, d), kept, n, med) in fitted.items():
        print(f"fit world {w}: x = {a:.5f} mx {b:+.1f}, z = {c:.5f} my {d:+.1f} ({kept} of {n} named points, median {med:.1f})", file=sys.stderr)
    placed, keys = [], set()
    for spec in CAMPS:
        p, why = place(spec, zones, rows, fitted)
        if p is None:
            print(f"  -- {spec[0]:15s} left out: {why}", file=sys.stderr)
            continue
        if p["key"] in keys:
            sys.exit(f"duplicate key {p['key']}")
        keys.add(p["key"])
        placed.append(p)
        off = f", the map {p['off']:.0f} off" if p["off"] else ""
        kinds = ", ".join(f"{k} x{n}" for k, n in p["kinds"])
        print(f"  {p['key']:15s} w{p['world']} ({p['x']:.0f}, {p['y']:.1f}, {p['z']:.0f}) {p['npc']!r} levels {p['levels']} (guide {p['guide']}){off}: {kinds}", file=sys.stderr)
    rare_near(placed, zones, rows, fitted, drops_by_mob(args.eqoa_data))
    for p in placed:
        if p["rare"]:
            print(f"  {p['key']:15s} rare: " + ", ".join(f"{r['name']} ({r['level'] or '?'})" for r in p["rare"]), file=sys.stderr)
    towns = other_towns(zones, rows, fitted)
    print("  towns outside Tunaria: " + ", ".join(f"{t['name']} (w{t['world']})" for t in towns), file=sys.stderr)
    if args.map_out is not None:
        mfit = map_fits(zones, rows)
        for w, ((a, b, c, d), kept, n, med) in mfit.items():
            print(f"map fit world {w}: x = {a:.5f} mx {b:+.1f}, z = {c:.5f} my {d:+.1f} ({kept} of {n} named points, median {med:.1f})", file=sys.stderr)
        mplaced, mleft = map_camps(zones, rows, mfit, tele_camps(args.camps))
        rare_near(mplaced, zones, rows, mfit, drops_by_mob(args.eqoa_data))
        for p in mplaced:
            off = math.hypot(p["x"] - p["point"][0], p["z"] - p["point"][1])
            kinds = ", ".join(f"{k} x{n}" for k, n in p["kinds"])
            print(f"  map {p['key']:34s} w{p['world']} ({p['x']:.0f}, {p['z']:.0f}) {p['npc']!r} levels {p['levels']} guide {p['guide'] or '-'} {off:.0f} off, {p['points']} pts [{p['map']}]: {kinds}", file=sys.stderr)
        for where, why in mleft:
            print(f"  map -- {where}: {why}", file=sys.stderr)
        per = collections.Counter(p["world"] for p in mplaced)
        print(f"map camps: {len(mplaced)} placed ({dict(sorted(per.items()))}), {len(mleft)} left out", file=sys.stderr)
        if args.map_out:
            with open(args.map_out, "w") as f:
                f.write(render_map(mplaced))
            if shutil.which("gofmt"):
                subprocess.run(["gofmt", "-w", args.map_out], check=True)
            print(f"wrote {len(mplaced)} map camps to {args.map_out}", file=sys.stderr)
        if not args.out:
            return
    text = render(placed, towns)
    if args.out:
        with open(args.out, "w") as f:
            f.write(text)
        # as gofmt lays it out, so a rerun's diff is only what changed
        if shutil.which("gofmt"):
            subprocess.run(["gofmt", "-w", args.out], check=True)
        print(f"wrote {len(placed)} camps to {args.out}", file=sys.stderr)
    else:
        sys.stdout.write(text)


if __name__ == "__main__":
    main()
