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

    python3 -E -s tools/gen_camp_teles.py --db eqoa.db --eqoa-data ../external/EQOA-Data \\
        --map-out internal/handlers/mapcamps.go --keep-written

writes the maps' other camps (EQOA-Data's un-noted camps, FAQ_CAMPS, CENSUS_CAMPS); --keep-written keeps the camps
the file holds as they are written (keep_written) and adds only new ones, so a run over a database whose creatures
moved since does not move, rename or merge the camps the maps already show.
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
    ("hexbones", ["shrieks"], "Hexbones & Shrieks", None, r"^(hexbone|shriek)$", "the Gramash Ruins' hexbones and shrieks"),
    ("nasehir", [], "Nasehir", [0], r"^nasehir ", "Northern Ro's Nasehir, the first camp"),
    ("nasehir2", [], "Nasehir", [1], r"^nasehir ", "Northern Ro's Nasehir, the second camp"),
    ("hatebone", ["halasorcs"], "Hatebone Orcs", [0], r"^hatebone ", "Halas' Hatebone orcs"),
    ("hatebone2", [], "Hatebone Orcs", [1], r"^hatebone ", "Halas' Hatebone orcs, the second camp"),
    ("hatebonecave", [], "Hatebone Orc Cave", None, r"^hatebone ", "Halas' Hatebone orc cave"),
    ("nroundead", [], "Undead", [0], r"skeleton|zombie|rotting|decaying|grave rat", "Northern Ro's undead"),
    ("nroundead2", [], "Undead", [1], r"skeleton|zombie|rotting|decaying|grave rat", "Northern Ro's undead, the second camp"),
    # gharial, tendora, phantoms, gargoyles, icegiants: with the kinds tools/faq_camps_20261114.py added or
    # restored (the gharial island's ancient gharials and Mudscale, Himlar, the elder spirits and The Ancient One,
    # Garhuuga, the frost drakes) and Tendora-Buki's own (its villagers are "Tendora-Buki ..." now, no kerran)
    ("gharial", [], "Gharial", None, r"^(ancient )?gharial|deep sea ghoul|^mudscale$", "the gharial island south of Moradhim"),
    ("tendora", ["kerrans"], "Tendora-Buki", None, r"^tendora-buki ", "Tendora-Buki, West Toxulia's kerran village"),
    ("kappa", ["slythis"], "Kappa", None, r"^kappa (guard|slave|worker|shaman|brute)", "Slythis Dwell's kappa"),
    ("kappawarriors", [], "Kappa Warrior", None, r"^kappa warrior", "Slythis Dwell's kappa warriors"),
    ("kappaelites", [], "Kappa Elites", None, r"^kappa elite", "Slythis Dwell's kappa elites"),
    ("drudges", [], "Kappa Drudge", None, r"^kappa drudge", "the East Plateau's kappa drudges"),
    ("phantoms", [], "Phantoms", None, r"^(phantom|himlar|elder spirit|the ancient one)$", "the East Plateau's phantoms"),
    ("gargoyles", [], "Gargoyle", None, r"gargoyle|^garhuuga$|^statue guardian$", "the South Barren Coast's gargoyles"),
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
    ("icegiants", [], "Ice Giant Refugees", None, r"^ice giant|^frost drake$", "the ice giant refugees near Northgate"),
    ("ghosts", [], "Ghosts", None, r"^wandering ghost$", "the wandering ghosts"),
    ("xrag", ["pariah"], "Pariah of Xragg", None, r"xrag|^sphere of annihilation", "the Elephant Graveyard's children of Xrag"),
    ("tombrats", [], "Tomb Rats", None, r"^(tomb|grim) rat", "the tomb rats"),
    ("gusts", ["icywind"], "Icy wind & freezing gusts", None, r"^freezing gust|^frost elemental|^icy wind", "the freezing gusts"),
    ("guardianforest", [], "Goblins", None, r"^goblin (assassin|soothsayer)", "Guardian Forest's goblins"),
    ("freezeblood", [], "Freezeblood Goblins", None, r"^freezeblood ", "Everfrost's Freezeblood goblins"),
    ("crocodiles", [], "Saltwater Crocodiles", None, r"crocodile|caiman", "Bobble-by-Water's saltwater crocodiles"),
    ("minotaurs", [], "Rustel Minotaur", None, r"^rustel minotaur", "the Gerntar Mines' Rustel minotaurs"),
]

# Camps EQOA-Data's map has no point for, placed by their creatures here round a centre (world, x, z) within
# reach: (key, aliases, (world, x, z, reach), the creatures (a regex on npcs.npc_name), what the list calls it).
# Heirloom petition 2489 (2026-11-20): "there is no level 34. One of my favorite camps is bogman rancid bone
# trolls" (seven rancidbone trolls, L34, west of Bogman Village).
EXTRA_CAMPS = [
    # Heirloom petition 2671 (2026-11-22): the map's "Hexbones & Shrieks" point is the Gramash Ruins' camp, labelled
    # Zentar's Keep's; the keep's own hexbones stand round its south walls
    ("zkhexbones", ["zkhex", "zentarhexbones"], (0, 5480.0, 6100.0, 300.0), r"^(hexbone|shriek)$", "Zentar's Keep's hexbones and shrieks"),
    ("rancidbones", ["rancidbone"], (0, 5536.0, 8616.0, 250.0), r"^rancidbone troll$", "Bogman Village's rancidbone trolls"),
]


def place_extra(spec, rows):
    key, aliases, (w, cx, cz, reach), pattern, label = spec
    rx = re.compile(pattern, re.I)
    kind = [r for r in rows if r[5] == w and rx.search(r[1].strip())]
    near = [r for r in kind if math.hypot(r[2] - cx, r[4] - cz) <= reach]
    if len(near) < 3:
        return None, f"only {len(near)} of its creatures within {reach:.0f} of its centre"
    anchor = camp_heart(kind, near, True, cx, cz)
    around = [o for o in kind if math.hypot(o[2] - anchor[2], o[4] - anchor[4]) <= LEVELS_WITHIN]
    lv = [o[6] for o in around if o[6]]
    levels = f"{min(lv)}-{max(lv)}" if lv and min(lv) != max(lv) else (str(lv[0]) if lv else "")
    spell = collections.defaultdict(collections.Counter)
    for o in kind:
        if math.hypot(o[2] - anchor[2], o[4] - anchor[4]) <= CREATURE_REACH:
            spell[o[1].strip().lower()][o[1].strip()] += 1
    creatures = [c.most_common(1)[0][0] for _, c in sorted(spell.items(), key=lambda kv: (-sum(kv[1].values()), kv[0]))]
    return {
        "key": key, "aliases": aliases, "label": label, "npc": anchor[1],
        "x": anchor[2], "y": anchor[3], "z": anchor[4], "world": w, "levels": levels,
        "count": len(around), "kinds": collections.Counter(o[1].strip() for o in around).most_common(4), "off": 0.0,
        "point": (cx, cz), "guide": "", "note": "", "creatures": creatures,
    }, ""


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
    # the kinds the FAQ's camps tool added to these camps (tools/faq_camps_20261114.py)
    "Snow Griffon": r"^(snow griffon|elder squeeks)$",
    "Tenacious Frost Giant": r"^(tenacious frost giant|arctic adept|arctic spiritcaller)$",
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


# ---- the FAQ's camps (the owner, 2026-11-14: "also add those new camps to the map") ----
#
# tools/faq_camps_20261114.py (Jarrod Garripoli's EQOA Frontiers FAQ, GameFAQs 48152) made four camps EQOA-Data's
# map has no point for, filled others and re-levelled eight. FAQ_CAMPS are the camps of it no camp here was: each
# written to mapcamps.go as a map camp, anchored as one (camp_heart) on its creatures within its reach of the FAQ
# tool's centre, its levels and creatures those of its kind within that reach (the camp whole, not LEVELS_WITHIN of
# the anchor), its MapX/MapY the anchor put back on EQOA-Data's sheet by the world's fit. One whose anchor is within
# SAME of a placed camp of its kind is that camp (said, left out).
# (key, name, world, the FAQ tool's centre (x, z), its creatures (a regex on npcs.npc_name, any case), reach)
FAQ_CAMPS = [
    ("frosteyeexcavation", "Frosteye Orcs (the excavation)", 0, (7571.3, 5687.0), r"^frosteye (centurion|patroller|sage)$", 250.0),
    ("taeewaggressors", "Tae Ew Aggressors", 0, (18330.0, 30560.0), r"^tae ew (aggressor|sacrificer)$", 250.0),
    ("desertbanditstakxiv", "Desert Bandits", 0, (19085.9, 25028.3), r"^desert (henchman|pillager)$", 250.0),
    ("frogloksburialmounds", "Frogloks (Burial Mounds)", 0, (18408.5, 29538.5), r"^froglok (slayer|inquisitor)$", 250.0),
    ("mossmouthgoblins", "Mossmouth Goblins", 0, (19390.0, 12378.0), r"^(goblin (gladiator|brigand|witch doctor|scout)|companion spirit)$", 700.0),
    ("hobgoblinscyclopsfortress", "Hobgoblins (Cyclops' Fortress)", 1, (7114.0, 3346.0), r"^hobgoblin mercenary$", 500.0),
    ("quintashislair", "Quintashi's Lair", 2, (4169.8, 8130.5), r"^(quintashi|quintashi's guardian)$", 200.0),
    ("thehuntbeasts", "The Hunt's Beasts", 2, (5250.0, 10600.0), r"^(plagued insect cloud|sabretooth huntress|vercozian wyrm)$", 450.0),
]

# The camps the FAQ re-levelled (tools/faq_camps_20261114.py's LEVELS): their levels are their kind's in the FAQ
# tool's box (the camp whole) with those about the anchor, so a map shows the FAQ's band, not the anchor's corner of
# it. (the camp's key here, world, box (x0, z0, x1, z1) or None, the names re-levelled). Mossmouth's and the
# Cyclops' Fortress hobgoblins' are FAQ_CAMPS (their reach is the box). A map draws these camps' levels as they
# are here, not the guide's (vanilla's tools/gen_map_markers.py reads this).
FAQ_LEVELS = [
    ("cracktooth", 0, (24000, 11000, 26000, 14000), ["cracktooth matron", "cracktooth orc", "cracktooth warrior", "biter cracktooth"]),
    ("taeewbrokenskullrock", 0, (20000, 30000, 22000, 32000), ["tae ew soldier", "tae ew shaman", "feral spirit"]),
    ("gargoyles", 2, (9900, 7900, 11900, 10000), ["gargoyle soldier", "gargoyle watcher", "leering gargoyle", "statue guardian", "gargoyle"]),
    ("ratmanthevastlydeep", 2, (10400, 6400, 11000, 6800), ["ratman fighter", "ratman scavenger"]),
    ("clankolbokthewarrenssouth", 2, (8000, 7900, 10000, 9900), ["clan kolbok scout", "clan kolbok shaman"]),
    ("tendora", 2, None, ["tendora-buki amir", "tendora-buki fisherman", "tendora-buki ghulam", "tendora-buki ispusar",
                          "tendora-buki mamluk", "tendora-buki pasdar", "tendora-buki puzhal", "tendora-buki spahi"]),
]
FAQ_LEVELED = {k for k, _, _, _ in FAQ_LEVELS} | {"mossmouthgoblins", "hobgoblinscyclopsfortress"}


def faq_levels(placed, rows):
    """FAQ_LEVELS' camps' levels: their kind's rows in the FAQ's box (within CREATURE_REACH*4 of the anchor where
    it gave none) with those the camp has."""
    by = {p["key"]: p for p in placed}
    for key, w, box, names in FAQ_LEVELS:
        p = by.get(key)
        if p is None:
            continue
        lv = [o[6] for o in rows if o[5] == w and o[1].strip().lower() in names and o[6] and (
            box[0] <= o[2] <= box[2] and box[1] <= o[4] <= box[3] if box else
            math.hypot(o[2] - p["x"], o[4] - p["z"]) <= CREATURE_REACH * 4)]
        lo, hi = level_range_of(p["levels"])
        if lv:
            lo, hi = min([lo] + lv) if lo else min(lv), max([hi] + lv)
        old = p["levels"]
        p["levels"] = f"{lo}-{hi}" if lo != hi else str(lo)
        print(f"  faq levels {key:26s} {old} -> {p['levels']}", file=sys.stderr)


def level_range_of(text):
    parts = [int(v) for v in text.split("-") if v.strip()]
    return (parts[0], parts[-1]) if parts else (0, 0)


def faq_camps(zones, rows, fitted, taken):
    """FAQ_CAMPS placed: (placed, left out). taken: the camps placed already (tele and map), each (key, world, x, z,
    kind regex, anchor's name)."""
    placed, left = [], []
    for key, name, w, (cx, cz), pattern, reach in FAQ_CAMPS:
        rx = re.compile(pattern, re.I)
        kind = [r for r in rows if r[5] == w and rx.search(r[1].strip())]
        near = [r for r in kind if math.hypot(r[2] - cx, r[4] - cz) <= reach]
        if len(near) < 3:
            left.append((name, f"only {len(near)} of its creatures within {reach:.0f} of its centre"))
            continue
        anchor = camp_heart(kind, near, True, cx, cz)
        same = [k for k, tw, x, z, trx, an in taken if tw == w and math.hypot(x - anchor[2], z - anchor[4]) <= SAME
                and (trx.search(anchor[1].strip()) or rx.search(an.strip()))]
        if same:
            left.append((name, f"the camp {same[0]}"))
            continue
        spell = collections.defaultdict(collections.Counter)
        for o in near:
            spell[o[1].strip().lower()][o[1].strip()] += 1
        creatures = [cc.most_common(1)[0][0] for _, cc in sorted(spell.items(), key=lambda kv: (-sum(kv[1].values()), kv[0]))]
        (a, b, c, d), _, _, _ = fitted[w]
        mx, my = (anchor[2] - b) / a, (anchor[4] - d) / c
        zone, _ = zone_of(zones, mx, my)
        label = name if zone.lower() in name.lower() else f"{name}, {zone}"
        placed.append({"key": key, "name": name, "map": "", "zone": zone, "label": label, "world": w, "npc": anchor[1],
                       "x": anchor[2], "y": anchor[3], "z": anchor[4], "levels": levels_of(near), "guide": "",
                       "mx": mx, "my": my, "creatures": creatures, "point": (cx, cz), "points": 0,
                       "kinds": collections.Counter(o[1].strip() for o in near).most_common(4)})
    return placed, left


# ---- the census's camps (the owner, 2026-11-23: "we are missing some XP camps on the map still", a camp of shrieks
# where a player stood among them) ----
#
# The camps the records hold that no map or #tele camp marks: the hostile spawns of worlds 0-2 (no NPC bit, a body,
# not a player's; levels 5-65; not spawn-only, seasonal, a quest step's own row or a GM trigger's) linked where each is
# within 50 of another (and 40 in height), a cluster of 5 or more with 4 or more of one kind, no camp within 300 of its
# middle (200 of any of its spawns, 700 for a camp anchored on one of its kinds), its kinds coherent (its top two, or
# one name's family, 60% of it), clusters of a kind within 300 one camp; kept when its lowest level is 60 or under and
# MasterVG782's guide (GameFAQs, section XVI) names one of its kinds as a camp's, or it holds 10 or more, or 6 or more
# of one kind. Each is placed as a FAQ camp is (census_camps: anchored by camp_heart on its kinds within its reach of
# its middle, its levels and creatures those of its kinds in reach, its zone EQOA-Data's map's by the world's fit),
# named by its creatures (the commonest kind, plural, and the next where it is 30%; a clan's name where no one kind is
# half) and its zone, a side where a name repeats. One within SAME of a placed camp of its kind is that camp (said).
# The comment after each: the cluster's size and levels, and the guide's camps it matches.
# (key, name, world, the cluster's middle (x, z), its kinds (a regex on npcs.npc_name, any case), reach)
CENSUS_CAMPS = [
    ("alkaradghastsputridskeletonsalkaradruins", "Al'karad Ghasts & Putrid Skeletons (Al Karad Ruins)", 0, (10762.0, 14853.8), r"^(al'karad ghast|putrid skeleton|al'karad ghoul|rotbone skeleton)$", 140.0),  # n 26, 27-29
    ("ancientgharielsserpenthills", "Ancient Ghariels (Serpent Hills)", 0, (16273.5, 22215.4), r"^(ancient ghariel)$", 70.0),  # n 9, 45-45
    ("ancientsawtuskselephantgraveyard", "Ancient Sawtusks (Elephant Graveyard)", 0, (14817.0, 26748.5), r"^(ancient sawtusk)$", 60.0),  # n 10, 37-54
    ("ancientsawtusksfortalliance", "Ancient Sawtusks (Fort Alliance)", 0, (15740.5, 24203.9), r"^(ancient sawtusk|bearwere)$", 80.0),  # n 8, 36-43
    ("ancientsawtuskstakxizeast", "Ancient Sawtusks (Tak 'Xiz, east)", 0, (16696.1, 25043.4), r"^(ancient sawtusk)$", 60.0),  # n 6, 42-44
    ("ancientsawtuskstakxiznorth", "Ancient Sawtusks (Tak 'Xiz, north)", 0, (16362.0, 24322.5), r"^(ancient sawtusk)$", 60.0),  # n 6, 42-44
    ("ancientsawtuskstakxizsouth", "Ancient Sawtusks (Tak 'Xiz, south)", 0, (16124.5, 25327.3), r"^(ancient sawtusk)$", 60.0),  # n 6, 42-44
    ("aviakthrashersbashersgoldfeathereyrie", "Aviak Thrashers & Bashers (Goldfeather Eyrie)", 0, (14660.9, 7441.0), r"^(aviak thrasher|aviak basher)$", 180.0),  # n 14, 20-20
    ("aviakswidowspeak", "Aviaks (Widow's Peak)", 0, (12891.3, 22472.0), r"^(aviak|aviak scouter)$", 240.0),  # n 44, 41-43, guide: AVIAKS
    ("blackantsantcolonies", "Black Ants (Ant Colonies)", 0, (21098.6, 29660.0), r"^(black ant scout|black ant leafcutter|black ant worker)$", 140.0),  # n 13, 25-32
    ("blacktongueferranshope", "Blacktongue (Ferran's Hope)", 0, (18531.3, 16679.5), r"^(blacktongue disciple|blacktongue orc|blacktongue apprenctice)$", 100.0),  # n 12, 37-40
    ("breathstealersdreadrotsisleofdread", "Breath Stealers & Dread Rots (Isle of Dread)", 0, (25398.9, 3194.6), r"^(breath stealer|dread rot)$", 150.0),  # n 13, 53-53
    ("breathstealersisleofdreadnorth", "Breath Stealers (Isle of Dread, north)", 0, (24976.8, 2360.1), r"^(breath stealer|dread rot|virulent blade)$", 170.0),  # n 14, 53-55
    ("breathstealersisleofdreadsouth", "Breath Stealers (Isle of Dread, south)", 0, (24649.9, 3016.6), r"^(breath stealer|dread rot|virulent blade|flesh crawler|dead rider|wicker man)$", 700.0),  # n 234, 53-55
    ("centaurwarderselephantgraveyard", "Centaur Warders (Elephant Graveyard)", 0, (14776.3, 26269.2), r"^(centaur warder|centaur elder)$", 60.0),  # n 17, 42-45
    ("chaoshoundstakishhiznorth", "Chaos Hounds (Takish'Hiz, north)", 0, (19304.5, 26624.3), r"^(chaos hound)$", 60.0),  # n 6, 44-45
    ("chaoshoundstakishhizsouth", "Chaos Hounds (Takish'Hiz, south)", 0, (18961.3, 27252.5), r"^(chaos hound)$", 60.0),  # n 6, 44-46
    ("cryptmummiessslathis", "Crypt Mummies (Sslathis)", 0, (22733.7, 28546.3), r"^(crypt mummy)$", 110.0),  # n 17, 26-28
    ("darkwispsgriffenneselephantgraveyard", "Darkwisps & Griffennes (Elephant Graveyard)", 0, (13650.9, 26272.7), r"^(darkwisp|griffenne|spineback)$", 60.0),  # n 24, 37-38
    ("darkwispsspiritselephantgraveyard", "Darkwisps & Spirits (Elephant Graveyard)", 0, (13623.8, 27712.2), r"^(darkwisp|spirit)$", 60.0),  # n 12, 37-40, guide: PHANTOMS
    ("deathfistdeathfistforge", "Deathfist (Deathfist Forge)", 0, (20890.5, 17469.2), r"^(deathfist myrmidon|deathfist priest|deathfist champion)$", 100.0),  # n 15, 27-33, guide: DFC - DEATHFIST CITADEL; MYRMIDONS
    ("deathfistnorthwesternro", "Deathfist (Northwestern Ro)", 0, (22550.2, 18544.7), r"^(deathfist legionnaire|deathfist healer|deathfist myrmidon|deathfist priest|deathfist oracle)$", 100.0),  # n 17, 23-30, guide: MTG DEATHFISTS; MTG DEATHFISTS/NASEHIRS; MYRMIDONS
    ("deathfistmyrmidonspriestsdeathfistforge", "Deathfist Myrmidons & Priests (Deathfist Forge)", 0, (21350.9, 17548.5), r"^(deathfist myrmidon|deathfist priest)$", 100.0),  # n 15, 27-42, guide: MYRMIDONS
    ("deathfistmyrmidonsdeathfistforge", "Deathfist Myrmidons (Deathfist Forge)", 0, (21664.2, 16516.4), r"^(deathfist myrmidon|deathfist priest)$", 80.0),  # n 14, 27-31, guide: MYRMIDONS
    ("deathfistprophetsreaversserpenthills", "Deathfist Prophets & Reavers (Serpent Hills)", 0, (17885.9, 22152.7), r"^(deathfist prophet|deathfist reaver)$", 100.0),  # n 10, 38-40, guide: DFC - DEATHFIST CITADEL
    ("deathfistreaversprophetsdeathfisthorde", "Deathfist Reavers & Prophets (Deathfist Horde)", 0, (19244.5, 20332.0), r"^(deathfist reaver|deathfist prophet)$", 80.0),  # n 10, 38-40, guide: DFC - DEATHFIST CITADEL
    ("deathfistreaversdeathfisthordeeast", "Deathfist Reavers (Deathfist Horde, east)", 0, (19532.2, 21025.0), r"^(deathfist reaver|deathfist prophet)$", 90.0),  # n 8, 38-40, guide: DFC - DEATHFIST CITADEL
    ("deathfistreaversdeathfisthordenorth", "Deathfist Reavers (Deathfist Horde, north)", 0, (19608.2, 20548.6), r"^(deathfist reaver|deathfist prophet)$", 80.0),  # n 7, 39-40, guide: DFC - DEATHFIST CITADEL
    ("deathfistreaversdeathfisthordesouth", "Deathfist Reavers (Deathfist Horde, south)", 0, (19229.1, 21506.3), r"^(deathfist reaver|deathfist prophet)$", 90.0),  # n 11, 38-40, guide: DFC - DEATHFIST CITADEL
    ("deathfistridersdeathfistcitadel", "Deathfist Riders (Deathfist Citadel)", 0, (21705.8, 18026.9), r"^(deathfist rider)$", 70.0),  # n 7, 31-33
    ("decayingzombiesnorthwesternro", "Decaying Zombies (Northwestern Ro)", 0, (22927.3, 16943.6), r"^(decaying zombie|skeleton|decaying guardian|rotting wolf|grave rat)$", 150.0),  # n 15, 17-22
    ("defiledmummiesfortalliance", "Defiled Mummies (Fort Alliance)", 0, (14484.3, 24563.5), r"^(defiled mummy)$", 80.0),  # n 8, 42-43
    ("direratshagley", "Dire Rats (Hagley)", 0, (7739.3, 17447.5), r"^(dire rat|tomb rat|grave spider)$", 100.0),  # n 17, 15-17
    ("direworgsrabidgreatbearsdshinnsredoubt", "Dire Worgs & Rabid Great Bears (Dshinn's Redoubt)", 0, (16124.8, 18829.3), r"^(dire worg|rabid great bear)$", 190.0),  # n 12, 44-51
    ("dragonworshippersfortalliance", "Dragon Worshippers (Fort Alliance)", 0, (14293.6, 23727.5), r"^(dragon worshipper)$", 60.0),  # n 7, 49-54
    ("elephantsrazorbackswidowspeak", "Elephants & Razorbacks (Widow's Peak)", 0, (13880.6, 22961.4), r"^(elephant|razorback)$", 270.0),  # n 18, 28-38
    ("elitefungusmencentaurvalley", "Elite Fungusmen (Centaur Valley)", 0, (14477.9, 22367.8), r"^(elite fungusman)$", 70.0),  # n 8, 36-40
    ("enthralledminotaursrunnyeye", "Enthralled Minotaurs (Runnyeye)", 0, (17744.0, 13563.5), r"^(enthralled minotaur)$", 100.0),  # n 7, 26-26
    ("fireantsrogueclockworks", "Fire Ants (Rogue Clockworks)", 0, (25005.5, 6157.2), r"^(fire ant)$", 60.0),  # n 6, 8-10
    ("froglokdarguk", "Froglok Dar (Guk)", 0, (24726.2, 29347.0), r"^(froglok dar warrior|froglok dar wizard|froglok dar shaman)$", 90.0),  # n 19, 38-43
    ("froglokguardsguk", "Froglok Guards (Guk)", 0, (25216.7, 30357.8), r"^(froglok guard|froglok fisherman|froglok forager)$", 80.0),  # n 13, 5-6
    ("froglokwarriorsshamansantcolonies", "Froglok Warriors & Shamans (Ant Colonies)", 0, (20233.5, 29344.7), r"^(froglok warrior|froglok shaman)$", 90.0),  # n 15, 31-33
    ("frosthoundspermafrost", "Frost Hounds (Permafrost)", 0, (4503.9, 4415.9), r"^(frost hound)$", 90.0),  # n 9, 47-51, guide: PERMAFROST
    ("funguscockroachescollonridgecemetary", "Fungus Cockroaches (Collonridge Cemetary)", 0, (24027.1, 8161.7), r"^(fungus cockroach)$", 80.0),  # n 8, 6-7
    ("gnollberserkersshamanswidowspeak", "Gnoll Berserkers & Shamans (Widow's Peak)", 0, (13696.4, 22506.5), r"^(gnoll berserker|gnoll shaman)$", 120.0),  # n 12, 33-40
    ("gnollraidersbandithills", "Gnoll Raiders (Bandit Hills)", 0, (13575.2, 14646.9), r"^(gnoll raider|gnoll mystic)$", 100.0),  # n 17, 16-35
    ("gnollsbearcave", "Gnolls (Bear Cave)", 0, (9280.0, 17517.3), r"^(gnoll strongfang|widowmaker|gnoll spiritpaw)$", 130.0),  # n 26, 16-27
    ("gnollsjethroscast", "Gnolls (Jethro's Cast)", 0, (6470.8, 13292.3), r"^(gnoll ranger|gnoll ovate|gnoll warder|gnoll spear fisher|gnoll hierophant)$", 180.0),  # n 28, 17-22
    ("goblinbanditssaerktowers", "Goblin Bandits (Saerk Towers)", 0, (20350.2, 13177.8), r"^(goblin bandit)$", 60.0),  # n 7, 25-25
    ("goldenemberfliesspidermine", "Golden Emberflies (Spider Mine)", 0, (9150.8, 19046.5), r"^(golden emberfly)$", 60.0),  # n 6, 28-28
    ("greyfleshpikesqeynosprison", "Greyflesh Pikes (Qeynos Prison)", 0, (4089.4, 17213.9), r"^(greyflesh pike)$", 90.0),  # n 11, 8-8
    ("griffawnsgriffonssaltmine", "Griffawns & Griffons (Salt Mine)", 0, (15036.7, 18535.8), r"^(griffawn|griffon|rabid great bear)$", 170.0),  # n 12, 32-45
    ("griffonsserpenthills", "Griffons (Serpent Hills)", 0, (17603.5, 22991.3), r"^(griffon warrior|griffon nest protector|griffon elder)$", 180.0),  # n 27, 48-49, guide: GRIFFON WARRIORS
    ("holcieltombofkings", "Holciel (Tomb of Kings)", 0, (21072.5, 14183.0), r"^(holciel guard|holciel commoner|holciel noble|holciel bureaucrat|summoned sentinel|hociel bureaucrat|fallen treasure hunter)$", 310.0),  # n 56, 31-34
    ("icedrakesanuvillage", "Ice Drakes (Anu Village)", 0, (8803.2, 6022.4), r"^(ice drake)$", 160.0),  # n 12, 43-50, guide: SNOW GRIFFONS
    ("icedrakespermafrost", "Ice Drakes (Permafrost)", 0, (4428.9, 4896.1), r"^(ice drake)$", 70.0),  # n 16, 48-52
    ("iceclawbearssnowblindplains", "Iceclaw Bears (Snowblind Plains)", 0, (7664.4, 4150.7), r"^(iceclaw bear)$", 110.0),  # n 19, 46-50
    ("imprisonedsoulsisleofdread", "Imprisoned Souls (Isle of Dread)", 0, (24863.6, 2597.8), r"^(imprisoned soul)$", 90.0),  # n 9, 53-53
    ("mosssnakesbagavillage", "Moss Snakes (Baga Village)", 0, (14634.9, 9748.8), r"^(moss snake|lightning bug)$", 110.0),  # n 12, 8-9
    ("orcthugsspiritiststhedruk", "Orc Thugs & Spiritists (Thedruk)", 0, (18431.6, 8248.0), r"^(orc thug|orc spiritist)$", 60.0),  # n 9, 7-8
    ("orcsthedruknorth", "Orcs (Thedruk, north)", 0, (19298.1, 7893.7), r"^(orc arsonist|orc chemist|orc spiritist)$", 160.0),  # n 19, 7-12
    ("petroleumoozeswhalehill", "Petroleum Oozes (Whale Hill)", 0, (5072.0, 15661.9), r"^(petroleum ooze)$", 170.0),  # n 15, 12-12
    ("prisonratsqeynosprison", "Prison Rats (Qeynos Prison)", 0, (5496.8, 19045.8), r"^(prison rat)$", 80.0),  # n 8, 17-18
    ("redantcarpentersburialmounds", "Red Ant Carpenters (Burial Mounds)", 0, (19051.4, 29769.2), r"^(red ant carpenter)$", 60.0),  # n 6, 34-35
    ("redantcarpentersstonewatchers", "Red Ant Carpenters (Stone Watchers)", 0, (19759.4, 30069.8), r"^(red ant carpenter)$", 60.0),  # n 6, 34-36
    ("redantsantcolonies", "Red Ants (Ant Colonies)", 0, (21489.3, 28535.2), r"^(red ant leafcutter|red ant warrior|red ant carpenter)$", 120.0),  # n 11, 35-40
    ("sacredmonarchsmayflyglade", "Sacred Monarchs (Mayfly Glade)", 0, (10540.5, 18828.9), r"^(sacred monarch)$", 90.0),  # n 11, 30-30
    ("scorchedspiritsseaoflions", "Scorched Spirits (Sea of Lions)", 0, (21045.2, 26410.2), r"^(scorched spirit)$", 60.0),  # n 7, 37-38
    ("shrublingsspirittalkerswood", "Shrublings (Spirit Talker's Wood)", 0, (12132.0, 11518.0), r"^(shrubling)$", 60.0),  # n 8, 30-30
    ("skeletonpiratesalfarakruins", "Skeleton Pirates (Al Farak Ruins)", 0, (23622.9, 19313.7), r"^(skeleton pirate)$", 60.0),  # n 6, 15-18
    ("skeletonsdecayingzombiesalkaradruins", "Skeletons & Decaying Zombies (Al Karad Ruins)", 0, (9089.2, 14882.5), r"^(skeleton|decaying zombie|grave rat)$", 200.0),  # n 33, 16-20
    ("skeletonsbagavillage", "Skeletons (Baga Village)", 0, (14041.8, 11612.1), r"^(skeleton)$", 60.0),  # n 8, 19-21
    ("skeletonsbearcave", "Skeletons (Bear Cave)", 0, (9224.0, 15500.8), r"^(skeleton|decaying zombie)$", 160.0),  # n 16, 16-20
    ("slesherswymondham", "Sleshers (Wymondham)", 0, (10536.2, 13826.7), r"^(slesher hierophant|slesher warrior|slesher scout)$", 100.0),  # n 14, 20-22
    ("slithtarwymondham", "Slith Tar (Wymondham)", 0, (11121.1, 12381.0), r"^(slith tar worker|slith tar servant|slith tar warrior|slith tar grub)$", 110.0),  # n 20, 21-23
    ("smallpiranhasthegreenrift", "Small Piranhas (The Green Rift)", 0, (22708.9, 11322.5), r"^(small piranha)$", 250.0),  # n 25, 18-27
    ("snowgriffonsanuvillage", "Snow Griffons (Anu Village)", 0, (8939.9, 5120.8), r"^(snow griffon)$", 60.0),  # n 6, 46-46, guide: SNOW GRIFFONS
    ("snowgriffonssnowblindplains", "Snow Griffons (Snowblind Plains)", 0, (7436.0, 4583.3), r"^(snow griffon)$", 60.0),  # n 6, 46-47, guide: SNOW GRIFFONS
    ("spinebackslionweressaltmine", "Spinebacks & Lionweres (Salt Mine)", 0, (13507.9, 19644.4), r"^(spineback|lionwere)$", 160.0),  # n 11, 33-42
    ("spinebacksmilasreef", "Spinebacks (Mila's Reef)", 0, (12452.3, 31176.0), r"^(spineback)$", 60.0),  # n 6, 36-38
    ("sunbeetleswhalehill", "Sunbeetles (Whale Hill)", 0, (4360.2, 16696.1), r"^(sunbeetle|giant rat|large spider)$", 240.0),  # n 16, 5-10
    ("sunstarstakishhiz", "Sunstars (Takish'Hiz)", 0, (19508.2, 27554.5), r"^(sunstar|moon wraith|star knight|plague stalker)$", 210.0),  # n 28, 44-52
    ("taeewcazicthule", "Tae Ew (Cazic-Thule)", 0, (14757.1, 30440.1), r"^(tae ew devastator|tae ew templar|tae ew champion|tae ew crusader|tae ew hierophant|tae ew zealot|thulian maneater|tae ew chosen)$", 310.0),  # n 56, 42-49
    ("taeewelephantgraveyard", "Tae Ew (Elephant Graveyard)", 0, (14915.7, 27755.0), r"^(tae ew crusader|tae ew devastator|tae ew hierophant)$", 140.0),  # n 15, 41-43
    ("taeewhierophantslakenoregard", "Tae Ew Hierophants (Lake Noregard)", 0, (17120.7, 28448.2), r"^(tae ew hierophant|tae ew crusader|tae ew devastator)$", 100.0),  # n 12, 41-43
    ("taeewknightsstonewatchers", "Tae Ew Knights (Stone Watchers)", 0, (18237.4, 30294.9), r"^(tae ew knight|tae ew destroyer|tae ew witchdoctor)$", 90.0),  # n 14, 37-39
    ("taintedwolvesmarielvillage", "Tainted Wolves (Mariel Village)", 0, (5501.8, 11317.5), r"^(tainted wolf)$", 80.0),  # n 6, 32-32
    ("tamegriffonsnorthkithicor", "Tame Griffons (North Kithicor)", 0, (21281.9, 10704.0), r"^(tame griffon)$", 70.0),  # n 8, 28-28
    ("tenaciousfrostgiantspermafrosteast", "Tenacious Frost Giants (Permafrost, east)", 0, (5503.0, 5110.8), r"^(tenacious frost giant)$", 290.0),  # n 32, 44-59, guide: TFGS - TENACIOUS FROST GIANTS
    ("tenaciousfrostgiantspermafrostwest", "Tenacious Frost Giants (Permafrost, west)", 0, (4939.8, 5466.0), r"^(tenacious frost giant)$", 70.0),  # n 10, 57-60, guide: TFGS - TENACIOUS FROST GIANTS
    ("thicketvipersmistythicket", "Thicket Vipers (Misty Thicket)", 0, (17911.2, 10551.9), r"^(thicket viper)$", 60.0),  # n 6, 8-8
    ("waterratshodstockandtemby", "Water Rats (Hodstock and Temby)", 0, (24932.6, 13616.6), r"^(water rat)$", 80.0),  # n 12, 10-13
    ("werelionscentaurrangerscentaurvalley", "Werelions & Centaur Rangers (Centaur Valley)", 0, (14511.3, 21470.3), r"^(werelion|centaur ranger|centaur druid)$", 130.0),  # n 14, 33-33
    ("widowspiderlingsrogueclockworks", "Widow Spiderlings (Rogue Clockworks)", 0, (24160.6, 6776.5), r"^(widow spiderling)$", 60.0),  # n 6, 7-8
    ("wildgriffawnsnorthkithicor", "Wild Griffawns (North Kithicor)", 0, (21532.2, 10315.3), r"^(wild griffawn)$", 110.0),  # n 10, 25-25
    ("wolfweresunkemptglade", "Wolfweres (Unkempt Glade)", 0, (6649.3, 8802.4), r"^(wolfwere)$", 100.0),  # n 8, 31-35
    ("woodfaeriesunkemptglade", "Wood Faeries (Unkempt Glade)", 0, (7889.3, 9208.5), r"^(wood faerie|faerie druid)$", 60.0),  # n 10, 43-45
    ("ankextetogreruins", "Ankextet (Ogre Ruins)", 1, (6489.8, 6754.9), r"^(ankextet soldier|ankextet guard|ankextet caster|ankextet priest|ankextet servant|ankextet noble)$", 210.0),  # n 46, 30-32
    ("ateshahlakerathe", "Atesh-Ah (Lake Rathe)", 1, (6347.5, 4768.7), r"^(atesh\-ah scout|atesh\-ah guard|atesh\-ah warrior|atesh\-ah spiritist|atesh\-ah soldier|atesh\-ah fisher|atesh\-ah ritualist|atesh\-ah embalmer)$", 420.0),  # n 66, 40-45
    ("cryptkeepersgeomancerspass", "Crypt Keepers (Geomancer's Pass)", 1, (4605.3, 6202.0), r"^(crypt keeper|malign wisp|toxic mist|revenant)$", 160.0),  # n 12, 46-50
    ("cyclopssentriescyclopsfortresseast", "Cyclops Sentries (Cyclops' Fortress, east)", 1, (6996.3, 2613.5), r"^(cyclops sentry)$", 90.0),  # n 9, 45-53
    ("cyclopssentriescyclopsfortresssouth", "Cyclops Sentries (Cyclops' Fortress, south)", 1, (6730.4, 3087.0), r"^(cyclops sentry)$", 90.0),  # n 10, 51-53
    ("cyclopssentriescyclopsfortresswest", "Cyclops Sentries (Cyclops' Fortress, west)", 1, (6247.2, 2693.2), r"^(cyclops sentry)$", 90.0),  # n 9, 45-53
    ("deadhillsspidersderelictmummiesdeadhills", "Dead Hills Spiders & Derelict Mummies (Dead Hills)", 1, (5113.6, 7521.6), r"^(dead hills spider|derelict mummy)$", 90.0),  # n 9, 54-57
    ("deadlycentipedeslowlandbasiliskssphinxpyramid", "Deadly Centipedes & Lowland Basilisks (Sphinx Pyramid)", 1, (8672.8, 5548.1), r"^(deadly centipede|lowland basilisk|timber wolf cub|giant spider)$", 120.0),  # n 14, 13-15
    ("derelictmummieszombiesdeadhills", "Derelict Mummies & Zombies (Dead Hills)", 1, (4625.8, 7634.3), r"^(derelict mummy|derelict zombie|derelict skeleton)$", 120.0),  # n 15, 53-55
    ("derelictmummiesdeadhills", "Derelict Mummies (Dead Hills)", 1, (4333.8, 6402.7), r"^(derelict mummy|derelict zombie|derelict skeleton)$", 100.0),  # n 18, 58-62
    ("fatpiranhasmoggoksgate", "Fat Piranhas (Moggok's Gate)", 1, (8883.0, 8559.3), r"^(fat piranha)$", 110.0),  # n 6, 13-13
    ("giantspiderssphinxpyramid", "Giant Spiders (Sphinx Pyramid)", 1, (9066.5, 5990.8), r"^(giant spider|raven)$", 130.0),  # n 13, 8-13
    ("greatspidersdirewolvesogreruins", "Great Spiders & Dire Wolves (Ogre Ruins)", 1, (6859.0, 7314.8), r"^(great spider|dire wolf|chaos rat)$", 140.0),  # n 14, 21-25
    ("hobgoblinoracleslakerathe", "Hobgoblin Oracles (Lake Rathe)", 1, (7132.6, 5350.7), r"^(hobgoblin oracle|hobgoblin centurion)$", 120.0),  # n 16, 33-35
    ("hobgoblinskulkssphinxpyramid", "Hobgoblin Skulks (Sphinx Pyramid)", 1, (8779.5, 6442.4), r"^(hobgoblin skulk)$", 90.0),  # n 8, 5-13
    ("hobgoblinwarriorsoracleslakerathe", "Hobgoblin Warriors & Oracles (Lake Rathe)", 1, (7478.8, 5505.7), r"^(hobgoblin warrior|hobgoblin oracle)$", 90.0),  # n 10, 33-35
    ("hobgoblinworkersogreruins", "Hobgoblin Workers (Ogre Ruins)", 1, (7045.5, 6902.6), r"^(hobgoblin worker|hobgoblin guard|hobgoblin footman)$", 100.0),  # n 15, 26-28
    ("lizardmandregsmoggoksgate", "Lizardman Dregs (Moggok's Gate)", 1, (8330.2, 8981.1), r"^(lizardman dreg|cockroach|lowland basilisk|deadly centipede|giant beetle)$", 260.0),  # n 19, 11-15
    ("lizardmenmoggoksgate", "Lizardmen (Moggok's Gate)", 1, (8223.6, 8254.0), r"^(lizardman dreg|lizardman neophyte|lizardman forager)$", 110.0),  # n 11, 11-15
    ("mithrilgargoylesgeomancerscitadeleast", "Mithril Gargoyles (Geomancer's Citadel, east)", 1, (5220.3, 2859.0), r"^(mithril gargoyle)$", 310.0),  # n 34, 51-51
    ("mithrilgargoylesgeomancerscitadelwest", "Mithril Gargoyles (Geomancer's Citadel, west)", 1, (4676.0, 2866.4), r"^(mithril gargoyle)$", 80.0),  # n 9, 51-51
    ("spectralrallosiansgeomancerspasseast", "Spectral Rallosians (Geomancer's Pass, east)", 1, (5267.6, 5165.3), r"^(spectral rallosian|spectral warlord)$", 180.0),  # n 13, 58-62
    ("spectralrallosiansgeomancerspasswest", "Spectral Rallosians (Geomancer's Pass, west)", 1, (4426.6, 5143.7), r"^(spectral rallosian|tomb serpent)$", 150.0),  # n 14, 52-62
    ("taeewwestfeerrotteast", "Tae Ew (West Feerrott, east)", 1, (7273.2, 8946.9), r"^(tae ew soldier|tae ew warder|plague wasp)$", 140.0),  # n 15, 18-24
    ("taeewwestfeerrottnorth", "Tae Ew (West Feerrott, north)", 1, (7251.2, 8128.8), r"^(tae ew soldier|tae ew warder|tae ew shaman)$", 70.0),  # n 12, 22-25
    ("taeewwestfeerrottsouth1", "Tae Ew (West Feerrott, south 1)", 1, (7078.0, 9201.7), r"^(tae ew warder|tae ew shaman|tae ew soldier)$", 70.0),  # n 12, 21-23
    ("taeewwestfeerrottsouth2", "Tae Ew (West Feerrott, south 2)", 1, (7430.4, 9374.2), r"^(tae ew warder|tae ew soldier|tae ew shaman|scythe beetle|large jaguar|slitnose alligator)$", 290.0),  # n 37, 18-25
    ("taeewwestfeerrottsouth3", "Tae Ew (West Feerrott, south 3)", 1, (7022.6, 9716.6), r"^(tae ew warder|tae ew shaman|tae ew soldier)$", 90.0),  # n 13, 23-25
    ("taeewwestfeerrottwest", "Tae Ew (West Feerrott, west)", 1, (6721.3, 8835.9), r"^(tae ew warder|slitnose alligator|tae ew soldier|tae ew shaman|wisp)$", 210.0),  # n 25, 18-25
    ("timberwolfcubssphinxpyramid", "Timber Wolf Cubs (Sphinx Pyramid)", 1, (8098.0, 5053.2), r"^(timber wolf cub|deadly centipede)$", 140.0),  # n 12, 13-18
    ("timberwolvessphinxpyramid", "Timber Wolves (Sphinx Pyramid)", 1, (9270.4, 4770.6), r"^(timber wolf|filth bug)$", 150.0),  # n 14, 19-22
    ("tombserpentsgeomancerspass", "Tomb Serpents (Geomancer's Pass)", 1, (4462.6, 5982.1), r"^(tomb serpent|skeletal giant|pyre ash elemental|mummified ogre|revenant)$", 320.0),  # n 23, 48-55
    ("torturedelementalslakerathe", "Tortured Elementals (Lake Rathe)", 1, (6797.1, 4657.4), r"^(tortured elemental|frenzied protectors|tortured guardian|wandering wisp)$", 100.0),  # n 13, 48-55
    ("toxicmistsgeomancerspass", "Toxic Mists (Geomancer's Pass)", 1, (4954.2, 6220.7), r"^(toxic mist|revenant)$", 150.0),  # n 9, 46-50
    ("treantsdeadhills", "Treants (Dead Hills)", 1, (5494.7, 7755.3), r"^(treant|revenant)$", 130.0),  # n 18, 48-53
    ("widowspiderlingsmoggoksgate", "Widow Spiderlings (Moggok's Gate)", 1, (9471.3, 8852.7), r"^(widow spiderling|giant mosquito|marrow bat|roach|giant centipede)$", 130.0),  # n 26, 7-16
    ("widowmakerswestfeerrott", "Widowmakers (West Feerrott)", 1, (6665.8, 9270.3), r"^(widowmaker|plague wasp|slitnose alligator|scythe beetle)$", 280.0),  # n 19, 21-24
    ("willowispsogreruins", "Will O'wisps (Ogre Ruins)", 1, (7082.7, 6115.3), r"^(will o'wisp|grizzly bear|rabid timberwolf|hobgoblin footman)$", 240.0),  # n 15, 26-30
    ("acolytefaunssatyrseasttoxxulia", "Acolyte Fauns & Satyrs (East Toxxulia)", 2, (7680.0, 4717.0), r"^(acolyte faun|acolyte satyr)$", 220.0),  # n 13, 41-45
    ("acolytesatyrseasttoxxuliaeast", "Acolyte Satyrs (East Toxxulia, east)", 2, (7491.9, 5047.5), r"^(acolyte satyr|acolyte faun|swampfire cobra)$", 170.0),  # n 17, 41-45
    ("acolytesatyrseasttoxxuliawest", "Acolyte Satyrs (East Toxxulia, west)", 2, (6055.7, 4436.7), r"^(acolyte satyr)$", 130.0),  # n 9, 41-44
    ("acolytesatyrssouthtoxxulia", "Acolyte Satyrs (South Toxxulia)", 2, (7089.1, 5982.3), r"^(acolyte satyr|acolyte faun)$", 280.0),  # n 17, 41-54
    ("acolyteseasttoxxulia", "Acolytes (East Toxxulia)", 2, (7248.9, 4657.5), r"^(acolyte faun|acolyte satyr|shadow lurker)$", 140.0),  # n 13, 41-45
    ("alluringseawispsstoneshellseaturtleserudscrossing", "Alluring Sea Wisps & Stoneshell Seaturtles (Erud's Crossing)", 2, (10576.1, 3681.8), r"^(alluring sea wisp|stoneshell seaturtle|volcanic dervish)$", 170.0),  # n 10, 56-60
    ("arisentemplarseasttoxxulia", "Arisen Templars (East Toxxulia)", 2, (8446.1, 5170.8), r"^(arisen templar)$", 140.0),  # n 9, 44-46
    ("clankolbokwesttoxxuliasouth", "Clan Kolbok (West Toxxulia, south)", 2, (5693.3, 5690.0), r"^(clan kolbok shaman|clan kolbok scout|forest dryad|clan kolbok guard|noxious basilisk)$", 310.0),  # n 25, 46-55, guide: KOBOLDS
    ("clankolbokshamansthewarrens", "Clan Kolbok Shamans (The Warrens)", 2, (9688.6, 9099.1), r"^(clan kolbok shaman|clan kolbok guard)$", 140.0),  # n 9, 46-53, guide: KOBOLDS
    ("corrodedclockworkscapedreg", "Corroded Clockworks (Cape Dreg)", 2, (10918.2, 10738.8), r"^(corroded clockwork)$", 70.0),  # n 6, 51-53, guide: CORRODED CLOCKWORKS
    ("elephantswestplateau", "Elephants (West Plateau)", 2, (8636.8, 4092.1), r"^(elephant|young cobra|savannah lioness)$", 130.0),  # n 10, 31-39
    ("enchantingsirenswesttoxxulia", "Enchanting Sirens (West Toxxulia)", 2, (4458.6, 5866.9), r"^(enchanting siren)$", 70.0),  # n 8, 60-60, guide: ENCHANTING SIRENS
    ("faunmaidenssylvanprotectorseasttoxxulia", "Faun Maidens & Sylvan Protectors (East Toxxulia)", 2, (6098.8, 4743.5), r"^(faun maiden|sylvan protector|shadow lurker)$", 150.0),  # n 16, 41-45
    ("giantscarabbeetlessavannahmatriarchswestplateau", "Giant Scarab Beetles & Savannah Matriarchs (West Plateau)", 2, (8103.9, 3139.6), r"^(giant scarab beetle|savannah matriarch|hooded cobra|giant scythe beetle)$", 330.0),  # n 22, 36-40
    ("haliaikerraisle", "Haliai (Kerra Isle)", 2, (4215.1, 6496.9), r"^(haliai guardian|haliai sorceress|haliai warrior|haliai priestess)$", 140.0),  # n 19, 60-63, guide: HALIAL
    ("hamadryadsabyssmalsea", "Hama Dryads (Abyssmal Sea)", 2, (5271.2, 9586.5), r"^(hama dryad)$", 70.0),  # n 8, 51-53
    ("hamadryadskerraisleeast", "Hama Dryads (Kerra Isle, east)", 2, (5084.8, 6827.3), r"^(hama dryad)$", 60.0),  # n 6, 51-53
    ("hamadryadskerraislenorth", "Hama Dryads (Kerra Isle, north)", 2, (4478.4, 6439.6), r"^(hama dryad)$", 60.0),  # n 10, 52-53
    ("hamadryadskerraislesouth", "Hama Dryads (Kerra Isle, south)", 2, (4910.3, 7549.1), r"^(hama dryad)$", 60.0),  # n 6, 52-53
    ("hillgiantssylhilthisdwell", "Hill Giants (Sylhilthis' Dwell)", 2, (5834.2, 3580.3), r"^(hill giant|kappa slave)$", 110.0),  # n 7, 40-53, guide: HILL GIANTS
    ("kappadrudgeswestplateaueast", "Kappa Drudges (West Plateau, east)", 2, (7578.8, 3701.8), r"^(kappa drudge)$", 90.0),  # n 8, 42-44, guide: PHANTOMS
    ("kappadrudgeswestplateauwest", "Kappa Drudges (West Plateau, west)", 2, (6644.8, 3878.2), r"^(kappa drudge)$", 110.0),  # n 8, 42-44, guide: HILL GIANTS; PHANTOMS
    ("kappashillgiantswestplateau", "Kappas & Hill Giants (West Plateau)", 2, (6318.9, 3295.5), r"^(kappa|hill giant)$", 160.0),  # n 11, 41-46, guide: HILL GIANTS
    ("kejekanstonebruntmtns", "Kejekan (Stonebrunt Mtns)", 2, (8621.8, 7684.7), r"^(kejekan warrior|kejekan pasdar|kejekan amir|kejekan ghulam|kejekan ispusar|kejekan ghazi|sabretooth jaguar|sabretooth panther)$", 160.0),  # n 42, 51-62
    ("kerranabyssmalseanorth", "Kerran (Abyssmal Sea, north)", 2, (5588.4, 8662.9), r"^(kerran priest|kerran wizard|kerran fisherman)$", 220.0),  # n 38, 55-57
    ("kerranabyssmalseasouth", "Kerran (Abyssmal Sea, south)", 2, (5530.1, 9690.7), r"^(kerran priest|kerran mage|kerran wizard)$", 110.0),  # n 11, 55-57
    ("kerrankerraisle", "Kerran (Kerra Isle)", 2, (5168.4, 6963.3), r"^(kerran soldier|kerran villager|kerran priest|kerran sentry|kerran fisherman)$", 110.0),  # n 24, 55-61
    ("kerranpriestsabyssmalsea", "Kerran Priests (Abyssmal Sea)", 2, (4930.5, 9794.6), r"^(kerran priest|kerran wizard)$", 110.0),  # n 13, 55-57
    ("kerranslavespaineel", "Kerran Slaves (Paineel)", 2, (6445.3, 9168.4), r"^(kerran slave|ooze|murk bone)$", 270.0),  # n 43, 20-35
    ("kerranwarriorssouthtoxxulia", "Kerran Warriors (South Toxxulia)", 2, (6812.1, 6267.8), r"^(kerran warrior|shambling hulk|noxious millipede)$", 120.0),  # n 10, 52-57
    ("marionettesgulfofuzun", "Marionettes (Gulf of Uzun)", 2, (8901.0, 11139.1), r"^(marionette|vagrant satyr)$", 250.0),  # n 25, 56-58
    ("noxiousmillipedeskerraisle", "Noxious Millipedes (Kerra Isle)", 2, (4363.7, 6748.6), r"^(noxious millipede)$", 60.0),  # n 6, 51-53
    ("oozespaineel", "Oozes (Paineel)", 2, (6916.3, 9088.4), r"^(ooze|murkwood hog|murk bone|murk spirit)$", 230.0),  # n 21, 31-35
    ("rumblingsandsnorthbarrencoast", "Rumbling Sands (North Barren Coast)", 2, (11687.2, 5697.1), r"^(rumbling sand|grotesque gargoyle|ratman wizard|warthog|ratman scout)$", 330.0),  # n 23, 41-45
    ("stoneshellseaturtlesvolcanicdervisheserudscrossing", "Stoneshell Seaturtles & Volcanic Dervishes (Erud's Crossing)", 2, (10253.4, 3089.5), r"^(stoneshell seaturtle|volcanic dervish|giant rockcrusher crab|alluring sea wisp)$", 130.0),  # n 14, 56-60
    ("tendorabukiwesttoxxuliaeast", "Tendora-Buki (West Toxxulia, east)", 2, (4859.5, 4756.5), r"^(tendora\-buki puzhal|tendora\-buki pasdar|tendora\-buki amir|tendora\-buki spahi|tendora\-buki ispusar|sylvan satyr)$", 260.0),  # n 24, 46-50
    ("tendorabukiwesttoxxuliawest", "Tendora-Buki (West Toxxulia, west)", 2, (4336.7, 4947.5), r"^(tendora\-buki fisherman|tendora\-buki mamluk|tendora\-buki spahi|tendora\-buki puzhal)$", 80.0),  # n 15, 46-50
    ("tendorabukiispusarsfishermenwesttoxxulia", "Tendora-Buki Ispusars & Fishermen (West Toxxulia)", 2, (4522.1, 5594.4), r"^(tendora\-buki ispusar|tendora\-buki fisherman|tendora\-buki amir)$", 120.0),  # n 16, 46-50
    ("welahinaierudscrossing", "Welahi-Nai (Erud's Crossing)", 2, (11192.0, 2472.1), r"^(welahi\-nai mujahed|welahi\-nai spahi|welahi\-nai ademzada|stoneshell seaturtle)$", 150.0),  # n 22, 56-63
    ("welahinaikerraisle", "Welahi-Nai (Kerra Isle)", 2, (4742.1, 6645.2), r"^(welahi\-nai myrmidon|welahi\-nai amir|welahi\-nai priestess|welahi\-nai mage|welahi\-nai puzhal|welahi\-nai fisherman)$", 260.0),  # n 50, 59-63
    ("welahinaifishermenkerraisle", "Welahi-Nai Fishermen (Kerra Isle)", 2, (4456.6, 7088.4), r"^(welahi\-nai fisherman)$", 60.0),  # n 6, 59-61
]


def census_camps(zones, rows, fitted, taken):
    """CENSUS_CAMPS placed as faq_camps places FAQ_CAMPS: (placed, left out)."""
    placed, left = [], []
    for key, name, w, (cx, cz), pattern, reach in CENSUS_CAMPS:
        rx = re.compile(pattern, re.I)
        kind = [r for r in rows if r[5] == w and rx.search(r[1].strip())]
        near = [r for r in kind if math.hypot(r[2] - cx, r[4] - cz) <= reach]
        if len(near) < 3:
            left.append((name, f"only {len(near)} of its creatures within {reach:.0f} of its middle"))
            continue
        anchor = camp_heart(kind, near, True, cx, cz)
        same = [k for k, tw, x, z, trx, an in taken if tw == w and math.hypot(x - anchor[2], z - anchor[4]) <= SAME
                and (trx.search(anchor[1].strip()) or rx.search(an.strip()))]
        if same:
            left.append((name, f"the camp {same[0]}"))
            continue
        spell = collections.defaultdict(collections.Counter)
        for o in near:
            spell[o[1].strip().lower()][o[1].strip()] += 1
        creatures = [cc.most_common(1)[0][0] for _, cc in sorted(spell.items(), key=lambda kv: (-sum(kv[1].values()), kv[0]))]
        (a, b, c, d), _, _, _ = fitted[w]
        mx, my = (anchor[2] - b) / a, (anchor[4] - d) / c
        zone, _ = zone_of(zones, mx, my)
        label = name if zone.lower() in name.lower() else f"{name}, {zone}"
        placed.append({"key": key, "name": name, "map": "", "zone": zone, "label": label, "world": w, "npc": anchor[1],
                       "x": anchor[2], "y": anchor[3], "z": anchor[4], "levels": levels_of(near), "guide": "",
                       "mx": mx, "my": my, "creatures": creatures, "point": (cx, cz), "points": 0, "rx": rx,
                       "kinds": collections.Counter(o[1].strip() for o in near).most_common(4)})
        taken.append((key, w, anchor[2], anchor[4], rx, anchor[1]))
    return placed, left


def written_map_camps(path):
    """mapcamps.go's entries as written: key -> (its line, world, x, z, anchor's name, creatures)."""
    out = {}
    if not os.path.exists(path):
        return out
    for line in open(path):
        m = re.match(r'\t"(\w+)":\s*\{.*NPC: "([^"]*)".*X: ([-\d.]+), Y: [-\d.]+, Z: ([-\d.]+), World: (\d+),.*Creatures: \[\]string\{([^}]*)\}', line)
        if m:
            out[m.group(1)] = (line.rstrip("\n"), int(m.group(5)), float(m.group(3)), float(m.group(4)), m.group(2),
                               [x.lower() for x in re.findall(r'"([^"]*)"', m.group(6))])
    return out


# how far from a written camp its anchor's name may stand (handlers' campAnchorReach)
ANCHOR_REACH = 300.0


def keep_written(written, placed, rows):
    """--keep-written: the camps written stay as they are (key, place, levels, name), but one whose anchor's name no
    longer stands within ANCHOR_REACH of it (its creatures renamed) is written again as placed now; and a placed camp
    is added only when its key is new and no written camp of its kind (either's anchor among the other's creatures)
    is within SAME of it. Returns (the lines, the camps added, the camps a written one already is, those rewritten)."""
    lines = {k: v[0] for k, v in written.items()}
    added, dup, redone = [], [], []
    stands = collections.defaultdict(list)
    for r in rows:
        stands[(r[1].strip().lower(), r[5])].append((r[2], r[4]))
    for p in placed:
        if p["key"] in written:
            _, w, x, z, an, _ = written[p["key"]]
            if not any(math.hypot(a - x, b - z) <= ANCHOR_REACH for a, b in stands[(an.strip().lower(), w)]):
                lines[p["key"]] = None
                redone.append(p)
            continue
        mine = {x.lower() for x in p["creatures"]} | {p["npc"].lower()}
        same = [k for k, (_, w, x, z, an, cr) in written.items() if w == p["world"]
                and math.hypot(x - p["x"], z - p["z"]) <= SAME and (an.lower() in mine or p["npc"].lower() in cr)]
        if same:
            dup.append((p, same[0]))
            continue
        lines[p["key"]] = None
        added.append(p)
    return lines, added, dup, redone


def render_map(placed, kept=None):
    """mapcamps.go of placed; kept (keep_written's): key -> its line as written, or None for a placed camp's."""
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
        "// and MapY its point on EQOA-Data's map. The FAQ's camps and the census's",
        "// (camps the records hold that nothing marked) are here too.",
        "var mapCamps = map[string]mapCamp{",
    ]
    by = {p["key"]: p for p in placed}
    for key in sorted(kept) if kept is not None else [p["key"] for p in placed]:
        if kept is not None and kept[key] is not None:
            lines.append(kept[key])
            continue
        p = by[key]
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
    ap.add_argument("--keep-written", action="store_true",
                    help="--map-out: keep the camps the file holds as written and add only new camps (keep_written)")
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
    for spec in EXTRA_CAMPS:
        p, why = place_extra(spec, rows)
        if p is None:
            print(f"  -- {spec[0]:15s} left out: {why}", file=sys.stderr)
            continue
        if p["key"] in keys:
            sys.exit(f"duplicate key {p['key']}")
        keys.add(p["key"])
        placed.append(p)
        print(f"  {p['key']:15s} w{p['world']} ({p['x']:.0f}, {p['y']:.1f}, {p['z']:.0f}) {p['npc']!r} levels {p['levels']} (extra)", file=sys.stderr)
    faq_levels(placed, rows)
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
        spec_rx = {spec[0]: re.compile(spec[4], re.I) for spec in CAMPS}
        taken = [(k, tw, x, z, spec_rx.get(k, re.compile(re.escape(an), re.I)), an) for k, (an, x, z, tw) in tele_camps(args.camps).items()]
        taken += [(p["key"], p["world"], p["x"], p["z"], p["rx"], p["npc"]) for p in mplaced]
        fplaced, fleft = faq_camps(zones, rows, mfit, taken)
        taken += [(p["key"], p["world"], p["x"], p["z"], re.compile("^(" + "|".join(re.escape(c) for c in p["creatures"]) + ")$", re.I),
                   p["npc"]) for p in fplaced]
        cplaced, cleft = census_camps(zones, rows, mfit, taken)
        mplaced = sorted(mplaced + fplaced + cplaced, key=lambda p: p["key"])
        if len({p["key"] for p in mplaced}) != len(mplaced):
            sys.exit("duplicate map camp keys")
        mleft += [(f"FAQ {n}", why) for n, why in fleft]
        mleft += [(f"census {n}", why) for n, why in cleft]
        faq_levels(mplaced, rows)
        rare_near(mplaced, zones, rows, mfit, drops_by_mob(args.eqoa_data))
        for p in mplaced:
            off = math.hypot(p["x"] - p["point"][0], p["z"] - p["point"][1])
            kinds = ", ".join(f"{k} x{n}" for k, n in p["kinds"])
            print(f"  map {p['key']:34s} w{p['world']} ({p['x']:.0f}, {p['z']:.0f}) {p['npc']!r} levels {p['levels']} guide {p['guide'] or '-'} {off:.0f} off, {p['points']} pts [{p['map']}]: {kinds}", file=sys.stderr)
        for where, why in mleft:
            print(f"  map -- {where}: {why}", file=sys.stderr)
        per = collections.Counter(p["world"] for p in mplaced)
        print(f"map camps: {len(mplaced)} placed ({dict(sorted(per.items()))}), {len(mleft)} left out", file=sys.stderr)
        kept = None
        if args.keep_written and args.map_out:
            kept, added, dup, redone = keep_written(written_map_camps(args.map_out), mplaced, rows)
            for p, k in dup:
                print(f"  keep: {p['key']} is the written {k}", file=sys.stderr)
            for p in redone:
                print(f"  keep: {p['key']} written again (its anchor's name is gone from its place)", file=sys.stderr)
            print(f"keep: {sum(1 for v in kept.values() if v is not None)} written kept, {len(added)} added: "
                  + ", ".join(p["key"] for p in added), file=sys.stderr)
        if args.map_out:
            with open(args.map_out, "w") as f:
                f.write(render_map(mplaced, kept))
            if shutil.which("gofmt"):
                subprocess.run(["gofmt", "-w", args.map_out], check=True)
            print(f"wrote {len(kept) if kept is not None else len(mplaced)} map camps to {args.map_out}", file=sys.stderr)
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
