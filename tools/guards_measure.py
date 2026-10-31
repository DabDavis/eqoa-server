#!/usr/bin/env python3
"""What the town guards would fight on a database, by the rule of
internal/entity/guards.go (read-only; F3 pass 25): who is a town guard
(townGuardLocked), what it takes on (guardsPreyLocked), within its radius in
3D in the same world. It lists the creature spawns that stand inside a guard's
radius (residents, which the rule leaves alone: without that they would be
killed at every respawn), the named and quest creatures among them, and the
roaming creatures whose spawn lets them wander into a guard's radius
(wildlife by internal/entity/wildlife.go's lists, npcmove rows), with how many
of those are neutral wildlife.

    tools/guards_measure.py [--db eqoa.db]
"""
import collections
import math
import re
import sqlite3
import sys

import argparse
import os
HERE = os.path.dirname(os.path.abspath(__file__))
ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
ap.add_argument("--db", default=os.path.join(HERE, "..", "eqoa.db"))
args = ap.parse_args()
db = sqlite3.connect(f"file:{args.db}?mode=ro", uri=True)


def key(name):  # database.NPCClassKey
    n = name.strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


TOWN_FACTIONS = {  # dialog.go townFactions
    "The Freeport Militia": "Freeport", "The Qeynos Guard": "Qeynos", "The Highpass Guard": "Highpass",
    "Klick`Anon Watch": "Klick`Anon", "House U`Dedne": "Neriak", "House J`Narus": "Neriak",
    "House Do`Vexis": "Neriak", "House Gal`Saris": "Neriak", "Merchant House Nothard": "Freeport",
}
GUARD_WORD = re.compile(r"(?i)\b(guard|guards|militia|watch|defenders|wardens|warden)\b")


def dialog_town(f):
    if f.startswith("Citizens of "):
        return f[len("Citizens of "):]
    return TOWN_FACTIONS.get(f, "")


def guard_town(f):
    f = f.strip()
    if f.startswith("Guards of "):
        return f[len("Guards of "):].strip()
    if not GUARD_WORD.search(f):
        return ""
    return dialog_town(f)


def guard_faction(f):
    f = f.strip()
    return f != "" and (f.startswith("Guards of ") or bool(GUARD_WORD.search(f)))


SIGN, BLOCKER = 0x6A82352C, 0x6F412342
players = {m & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel")}
social = {}
for name, s, f, r in db.execute("SELECT npc_name, social, faction, radius FROM npcsocial"):
    k = key(name)
    if k in social and not social[k][0] and s:
        continue
    social[k] = (bool(s), f, r)
hits = collections.defaultdict(list)
for name, f, d in db.execute("SELECT npc_name, faction, delta FROM factionhits ORDER BY id"):
    hits[key(name)].append((f, d))
aggro = {key(n): bool(a) for n, a in db.execute("SELECT npc_name, aggressive FROM npcaggro")}
spawnonly = {key(n) for (n,) in db.execute("SELECT name FROM npcspawnonly")}
nf = collections.defaultdict(list)
for name, world, x, z, r, f in db.execute("SELECT name, world, x, z, radius, faction FROM npcfaction"):
    rank = r if r > 0 else (3.4e38 / 2 if world >= 0 else 3.4e38)
    nf[key(name)].append((rank, world, x, z, r, f))
for k in nf:
    nf[k].sort(key=lambda t: t[0])


def npc_faction(n):
    for _, world, x, z, r, f in nf.get(key(n["name"]), []):
        if world >= 0 and n["world"] != world:
            continue
        if r <= 0 or math.hypot(n["x"] - x, n["z"] - z) <= r:
            return f
    return ""


def social_faction(n):
    r = social.get(key(n["name"]))
    if r and r[1]:
        return r[1]
    best, fac = 0, ""
    for f, d in hits.get(key(n["name"]), []):
        if d < best:
            best, fac = d, f
    return fac


def of_faction(n, faction):
    r = social.get(key(n["name"]))
    if r and r[1] == faction:
        return True
    return any(d < 0 and f == faction for f, d in hits.get(key(n["name"]), []))


def town_guard(n):
    r = social.get(key(n["name"]))
    return bool(r and r[0] and guard_town(social_faction(n)))


def dummy(name):
    n = name.strip().lower()
    return n.startswith(("training dummy", "regular dummy", "sparring dummy"))


npcs = []
for i, name, world, x, y, z, model, typ, lvl in db.execute(
        "SELECT id, npc_name, world, x_coord, y_coord, z_coord, modelid, npc_type, npc_level FROM npcs"):
    npcs.append(dict(id=i, name=name or "", world=world, x=x, y=y, z=z, model=(model or 0) & 0xFFFFFFFF,
                     typ=typ or 0, lvl=lvl or 0))


def prey(g, c, gfaction):
    if c is g or c["typ"] & 0x80 or dummy(c["name"]) or c["world"] != g["world"] or key(c["name"]) in spawnonly:
        return False
    if c["model"] in (0, SIGN, BLOCKER) or c["model"] in players:
        return False
    a = aggro.get(key(c["name"]))
    if a is not None and not a:
        return False
    if town_guard(c):
        return False
    own = npc_faction(c)
    if guard_faction(own) or guard_faction(social_faction(c)):
        return False
    if of_faction(c, gfaction):
        return False
    town = guard_town(gfaction)
    return town == "" or dialog_town(own) != town


guards = [n for n in npcs if town_guard(n) and not n["typ"] & 0x80 and not dummy(n["name"]) and key(n["name"]) not in spawnonly]
cells = collections.defaultdict(list)
for n in npcs:
    cells[(n["world"], int(n["x"] // 50), int(n["z"] // 50))].append(n)

quest = set()
for (t,) in db.execute("SELECT target FROM queststeps WHERE kind IN ('kill', 'loot')"):
    quest.add(key(t))
for (t,) in db.execute("SELECT target FROM queststeptargets"):
    quest.add(key(t))

pairs = []
for g in guards:
    r = social.get(key(g["name"]))[2] or 30.0
    gf = social_faction(g)
    cx, cz = int(g["x"] // 50), int(g["z"] // 50)
    reach = int(math.ceil(r / 50))
    for dx in range(-reach, reach + 1):
        for dz in range(-reach, reach + 1):
            for c in cells.get((g["world"], cx + dx, cz + dz), []):
                d = math.dist((g["x"], g["y"], g["z"]), (c["x"], c["y"], c["z"]))
                if d <= r and prey(g, c, gf):
                    pairs.append((g, c, d))

creatures = {c["id"]: c for _, c, _ in pairs}
print(f"town guards: {len(guards)} spawns ({len({key(g['name']) for g in guards})} names); "
      f"creature spawns within a guard's radius: {len(creatures)} (in {len(pairs)} guard-creature pairs)")

by_town = collections.Counter()
for cid, c in creatures.items():
    g = min((p for p in pairs if p[1]["id"] == cid), key=lambda p: p[2])[0]
    by_town[guard_town(social_faction(g))] += 1
print("\nby town:", ", ".join(f"{t} {n}" for t, n in by_town.most_common()))

# places: guards within 60 of each other gathered, the creatures they see counted
place_of = {}
places = []
for g in sorted({p[0]["id"]: p[0] for p in pairs}.values(), key=lambda g: (g["world"], g["x"], g["z"])):
    for pl in places:
        if pl["world"] == g["world"] and any(math.hypot(g["x"] - h["x"], g["z"] - h["z"]) <= 60 for h in pl["guards"]):
            pl["guards"].append(g)
            place_of[g["id"]] = pl
            break
    else:
        pl = {"world": g["world"], "guards": [g], "creatures": {}}
        places.append(pl)
        place_of[g["id"]] = pl
for g, c, d in pairs:
    place_of[g["id"]]["creatures"][c["id"]] = c
places.sort(key=lambda pl: -len(pl["creatures"]))
print("\nthe worst places (world, town, where, guards, creatures):")
for pl in places[:25]:
    gs = pl["guards"]
    x = sum(g["x"] for g in gs) / len(gs)
    z = sum(g["z"] for g in gs) / len(gs)
    town = guard_town(social_faction(gs[0]))
    kinds = collections.Counter(f"{c['name']} L{c['lvl']}" for c in pl["creatures"].values())
    print(f"  world {pl['world']} {town:22s} ({x:7.0f}, {z:7.0f})  {len(gs)} guards ({', '.join(sorted({g['name'] for g in gs})[:3])}{'...' if len(gs) > 3 else ''}): "
          f"{len(pl['creatures'])} - " + ", ".join(f"{k} x{n}" if n > 1 else k for k, n in kinds.most_common(6)))

named = sorted({c["name"] for c in creatures.values() if c["name"][:1].isupper()})
quests = sorted({c["name"] for c in creatures.values() if key(c["name"]) in quest})
print(f"\nnamed (a capital first letter, an individual's name after the name-case fix): {len(named)}")
for n in named:
    print("  ", n, f"(L{next(c['lvl'] for c in creatures.values() if c['name'] == n)})",
          "QUEST TARGET" if key(n) in quest else "")
print(f"\nquest targets (kill/loot steps, queststeptargets): {len(quests)}")
for n in quests:
    cs = [c for c in creatures.values() if c["name"] == n]
    print("  ", n, f"x{len(cs)}", f"L{cs[0]['lvl']}")

# --- creatures that may wander into a guard's radius: they roam (wildlife by
# the server's name rule, or an npcmove row that says so), are no residents
# of any guard, and their spawn lies within their roam radius plus the
# guard's of a post (entity/wildlife.go, movement.go; npcundead's undead
# never roam as wildlife).
src = open(os.path.join(HERE, "..", "internal", "entity", "wildlife.go")).read()
def words_of(block):
    m = re.search(r"var " + block + r" = wordSet\(`(.*?)`\)", src, re.S)
    return set(m.group(1).split()) if m else set()
NOUNS, TITLES, GUARDW = words_of("wildlifeNouns"), words_of("wildlifeTitles"), words_of("wildlifeGuards")
def sing(w):
    out = [w]
    if w.endswith("ies") and len(w) > 4: out.append(w[:-3] + "y")
    if w.endswith("ves") and len(w) > 4: out.append(w[:-3] + "f")
    if w.endswith("es") and len(w) > 3: out.append(w[:-2])
    if w.endswith("s") and len(w) > 3: out.append(w[:-1])
    return out
def wild_word(name):
    ws = re.findall(r"[a-z]+", name.replace("$", "").lower())
    if not ws or any(s in GUARDW for w in ws for s in sing(w)):
        return ""
    while len(ws) > 1 and any(s in TITLES for s in sing(ws[-1])):
        ws = ws[:-1]
    h = ws[-1]
    return h if any(s in NOUNS or (len(s) >= 6 and s.endswith("fly")) for s in sing(h)) else ""
undead = {key(n): bool(u) for n, u in db.execute("SELECT name, undead FROM npcundead")}
move = {key(n): (r, rad) for n, r, rad in db.execute("SELECT name, roam, radius FROM npcmove")}
def roams(c):
    if c["model"] == 0 or c["typ"] & 0x80:
        return 0
    m = move.get(key(c["name"]))
    if m and m[0] >= 0:
        return (m[1] or 75.0) if m[0] == 1 else 0
    if c["model"] in players:
        return 0
    if wild_word(c["name"]) and not undead.get(key(c["name"]), False):
        return 75.0
    return 0
if NOUNS:
    wander = {}
    for g in guards:
        r = social.get(key(g["name"]))[2] or 30.0
        gf = social_faction(g)
        cx, cz = int(g["x"] // 50), int(g["z"] // 50)
        for dx in range(-4, 5):
            for dz in range(-4, 5):
                for c in cells.get((g["world"], cx + dx, cz + dz), []):
                    rr = roams(c)
                    if not rr:
                        continue
                    d = math.dist((g["x"], g["y"], g["z"]), (c["x"], c["y"], c["z"]))
                    if r < d <= r + rr and prey(g, c, gf) and c["id"] not in creatures:
                        wander.setdefault(c["id"], (c, g, d))
    print(f"\nroaming creatures that may wander into a guard's radius (spawn within roam + radius, no resident): {len(wander)}")
    by = collections.Counter()
    for c, g, d in wander.values():
        by[(guard_town(social_faction(g)), c["name"], c["lvl"])] += 1
    for (town, name, lvl), n in by.most_common(30):
        print(f"   {town:20s} {name} L{lvl} x{n}", "QUEST TARGET" if key(name) in quest else "")
if NOUNS:
    NEUTRAL = 5  # entity.NeutralWildlifeLevel
    neutral = [c for c, g, d in wander.values()
               if c["lvl"] <= NEUTRAL and (aggro.get(key(c["name"])) is not True) and wild_word(c["name"])]
    print(f"\nof those, neutral wildlife (level <= {NEUTRAL}, attacks no one on sight): {len(neutral)}; the rest:")
    rest = collections.Counter(f"{c['name']} L{c['lvl']}" for c, g, d in wander.values() if c not in neutral)
    for k, n in rest.most_common(20):
        print("  ", k, f"x{n}")
