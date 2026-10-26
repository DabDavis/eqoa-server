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

    python3 -E -s tools/gen_camp_teles.py --db eqoa.db \\
        --eqoa-data ../external/EQOA-Data [--out internal/handlers/camps.go]

Without --out it prints the table it would write and every camp's match.
"""

import argparse
import collections
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
            # the heart of the camp nearest the point: of the creatures with
            # at least half the most company any of them has within CLUSTER
            # (and three), the nearest; then the one of most company within
            # CLUSTER of it
            def company(r):
                return sum(1 for o in kind if math.hypot(o[2] - r[2], o[4] - r[4]) <= CLUSTER)
            dense = [(company(r), r) for r in cands]
            top = max(n for n, _ in dense)
            floor = max(3, top // 2) if close else top
            busy = [r for n, r in dense if n >= floor] or [r for _, r in dense]
            near = min(busy, key=lambda r: (math.hypot(r[2] - gx, r[4] - gz), r[0]))
            local = [r for r in cands if math.hypot(r[2] - near[2], r[4] - near[4]) <= CLUSTER]
            # of the camp's commonest kind there (not its named one)
            common = collections.Counter(r[1].strip().lower() for r in local).most_common(1)[0][0]
            local = [r for r in local if r[1].strip().lower() == common]
            return min(local, key=lambda r: (-company(r), math.hypot(r[2] - near[2], r[4] - near[4]), r[0]))

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
    return {
        "key": key, "aliases": aliases, "label": label, "npc": anchor[1].strip(),
        "x": anchor[2], "y": anchor[3], "z": anchor[4], "world": w, "levels": levels,
        "count": len(around), "kinds": kinds.most_common(4), "off": off, "point": point,
        "guide": camps[0].get("levelRange", ""),
    }, ""


def go_string(s):
    return json.dumps(s, ensure_ascii=False)


def render(placed):
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
    text = render(placed)
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
