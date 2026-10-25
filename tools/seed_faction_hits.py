#!/usr/bin/env python3
"""The town rows of factionhits: killing a city's guards and townsfolk lowers
the killer's standing with that city's factions (entity/faction.go).

No capture holds a faction hit, so every number here is this server's own,
and a GM may change any row in the panel. What the tool decides is only WHO
belongs to WHICH city, and it takes that from data, not from this file:

- The cities are EQOA-Data's map towns (Map/zones.json, "towns": a name and
  a map position). The map is the world scaled: its points of interest
  ("pois") that name an NPC found once both there and in npcs fit the scale
  and the offset (least squares, the far misses dropped and fitted again),
  and the fit puts each town in the world.
- Townsfolk are NPCs with a playable race's body (the charactermodel table's
  models), standing together: chained within --link of one another, a chain
  being a city's when one of its NPCs stands within --reach of the town's
  point. Only the attackable ones (npc_type without 0x80) get rows.
- A town's factions are its "Citizens of <town>" (names compared without
  case, spaces or punctuation, one the start of the other: Neriak, Nektulos
  is Citizens of Neriak) and, for the guards (a name with the word Guard),
  the factions that name the town and a guard word (The Freeport Militia,
  Klick`Anon Watch, The Highpass Guard, Defenders of Highpass).
- A name is given rows only when every NPC of that name is the one city's,
  since a row is by name (a #spawn copy shares it): a name seen in two
  cities, or out of town too, is left to the GM and listed.

Rows are written with source "town-seed" and never over a GM's row for the
same name and faction. --undo deletes every town-seed row. Both take a copy
of the database first (<db>.bak-<when>-pre-factionseed). Report only by
default.

The factions table is made and filled by the server (its first start with
factions): start it once first. Restart it after --apply or --undo, or edit
any faction hit in the GM panel, for the running world to read the rows.

    tools/seed_faction_hits.py --eqoa-data ../external/EQOA-Data [--db eqoa.db]
        [--apply | --undo] [--link 100] [--reach 400] [--citizen -25] [--guard -50]
"""
import argparse
import collections
import datetime
import json
import math
import os
import re
import sqlite3
import statistics
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "town-seed"
GUARD_WORDS = {"guard", "guards", "watch", "militia", "defenders", "warden", "wardens"}


def key(name):
    """A name compared without case, spaces or punctuation."""
    return re.sub(r"[^a-z0-9]", "", name.lower())


def poi_name(name):
    """A point of interest's NPC name, without its level: "Guard Sareken (10)"."""
    return re.sub(r"\s*\(\d+(?:\s*-\s*\d+)?\)\s*$", "", name).strip()


def fit_axis(xs, ys):
    n = len(xs)
    mx, my = sum(xs) / n, sum(ys) / n
    sxx = sum((x - mx) ** 2 for x in xs)
    a = sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / sxx
    return a, my - a * mx


def fit_map(pois, npcs_by_name):
    """The map-to-world scale and offset on each axis, from the points of
    interest naming an NPC found once in both, and how well they fit."""
    pairs = []
    for name, ps in pois.items():
        rows = npcs_by_name.get(name, [])
        if len(ps) == 1 and len(rows) == 1:
            pairs.append((ps[0]["x"], ps[0]["y"], rows[0]["x"], rows[0]["z"]))
    if len(pairs) < 10:
        sys.exit(f"only {len(pairs)} map points name a single NPC: too few to place the towns")
    cur = pairs
    while True:
        ax, bx = fit_axis([p[0] for p in cur], [p[2] for p in cur])
        az, bz = fit_axis([p[1] for p in cur], [p[3] for p in cur])
        res = [math.hypot(ax * p[0] + bx - p[2], az * p[1] + bz - p[3]) for p in cur]
        med = statistics.median(res)
        keep = [p for p, r in zip(cur, res) if r <= max(3 * med, 100)]
        if len(keep) == len(cur) or len(keep) < 10:
            return (ax, bx, az, bz), len(pairs), len(cur), med
        cur = keep


def chains(points, link):
    """Groups of points chained within link of one another (indices)."""
    parent = list(range(len(points)))

    def find(i):
        while parent[i] != i:
            parent[i] = parent[parent[i]]
            i = parent[i]
        return i

    grid = collections.defaultdict(list)
    for i, p in enumerate(points):
        grid[(int(p["x"] // link), int(p["z"] // link))].append(i)
    for i, p in enumerate(points):
        gx, gz = int(p["x"] // link), int(p["z"] // link)
        for dx in (-1, 0, 1):
            for dz in (-1, 0, 1):
                for j in grid.get((gx + dx, gz + dz), ()):
                    if j > i and math.hypot(p["x"] - points[j]["x"], p["z"] - points[j]["z"]) <= link:
                        a, b = find(i), find(j)
                        if a != b:
                            parent[a] = b
    out = collections.defaultdict(list)
    for i in range(len(points)):
        out[find(i)].append(i)
    return list(out.values())


def citizens_of(town, factions):
    """The town's "Citizens of" faction and the key its other factions are
    matched by, or (None, None)."""
    t = key(town)
    best = None
    for f in factions:
        if not f.startswith("Citizens of "):
            continue
        c = key(f[len("Citizens of "):])
        if c == t or (len(c) >= 5 and len(t) >= 5 and (t.startswith(c) or c.startswith(t))):
            if best is None or abs(len(c) - len(t)) < abs(len(key(best[len("Citizens of "):])) - len(t)):
                best = f
    if best is None:
        return None, None
    return best, key(best[len("Citizens of "):])


def guard_factions(town_key, factions):
    out = []
    for f in factions:
        if f.startswith("Citizens of ") or town_key not in key(f):
            continue
        if GUARD_WORDS & set(re.findall(r"[a-z]+", f.lower())):
            out.append(f)
    return sorted(out)


def is_guard(name):
    return "guard" in re.findall(r"[a-z]+", name.lower())


def plan(db, eqoa_data, link, reach, citizen, guard):
    factions = [r[0] for r in db.execute("SELECT name FROM factions")]
    if not factions:
        sys.exit("the factions table is empty or missing: start the server once (it makes and fills it)")
    models = {r[0] & 0xFFFFFFFF for r in db.execute("SELECT modelid FROM charactermodel")}
    npcs = [dict(id=r[0], name=r[1], x=r[2], z=r[3], model=r[4] & 0xFFFFFFFF, type=r[5] or 0, world=r[6])
            for r in db.execute("SELECT id, npc_name, x_coord, z_coord, modelid, npc_type, world FROM npcs")]
    by_name_all = collections.defaultdict(list)
    for n in npcs:
        by_name_all[n["name"]].append(n)
    tunaria = [n for n in npcs if n["world"] == 0]
    by_name = collections.defaultdict(list)
    for n in tunaria:
        by_name[n["name"].strip()].append(n)

    zones = json.load(open(os.path.join(eqoa_data, "Map", "zones.json")))
    pois = collections.defaultdict(list)
    for p in zones["pois"]:
        pois[poi_name(p["name"])].append(p)
    (ax, bx, az, bz), anchors, kept, med = fit_map(pois, by_name)

    folk = [n for n in tunaria if n["model"] in models]
    groups = chains(folk, link)
    towns = []
    for t in zones["towns"]:
        tx, tz = ax * t["x"] + bx, az * t["y"] + bz
        members = [folk[i] for g in groups if any(math.hypot(folk[i]["x"] - tx, folk[i]["z"] - tz) <= reach for i in g)
                   for i in g]
        cit, ck = citizens_of(t["name"], factions)
        towns.append(dict(name=t["name"], x=tx, z=tz, members=members, citizens=cit,
                          guards=guard_factions(ck, factions) if ck else []))

    # a name's cities: only a name all of whose NPCs are one city's gets rows
    city_of = collections.defaultdict(set)
    ids_of = collections.defaultdict(set)
    for t in towns:
        if not t["citizens"]:
            continue
        for n in t["members"]:
            if not (n["type"] & 0x80):
                city_of[n["name"]].add(t["citizens"])
                ids_of[n["name"]].add(n["id"])
    rows, skipped = [], []
    for name, cities in sorted(city_of.items()):
        everywhere = {n["id"] for n in by_name_all[name]}
        if len(cities) > 1:
            skipped.append((name, "in " + " and ".join(sorted(cities))))
            continue
        if everywhere - ids_of[name]:
            skipped.append((name, f"{len(everywhere - ids_of[name])} more out of town"))
            continue
        cit = next(iter(cities))
        town = next(t for t in towns if t["citizens"] == cit)
        rows.append((name, cit, citizen))
        if is_guard(name):
            for g in town["guards"]:
                rows.append((name, g, guard))
    fit = dict(anchors=anchors, kept=kept, median=med, scale=(ax, az), offset=(bx, bz))
    return towns, rows, skipped, fit


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-factionseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="the EQOA-Data checkout (Map/zones.json)")
    ap.add_argument("--apply", action="store_true", help="write the town rows")
    ap.add_argument("--undo", action="store_true", help="delete every town-seed row")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--link", type=float, default=100, help="townsfolk this near one another are one town")
    ap.add_argument("--reach", type=float, default=400, help="a town's NPCs: a chain within this of its map point")
    ap.add_argument("--citizen", type=int, default=-25, help="a townsperson's kill on its Citizens faction")
    ap.add_argument("--guard", type=int, default=-50, help="a guard's kill on each of its town's guard factions")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    if a.undo:
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        n = db.execute("DELETE FROM factionhits WHERE source = ?", (SOURCE,)).rowcount
        print(f"deleted {n} town-seed rows")
        return
    if not a.eqoa_data:
        ap.error("--eqoa-data is needed")
    towns, rows, skipped, fit = plan(db, a.eqoa_data, a.link, a.reach, a.citizen, a.guard)
    print(f"map fit: {fit['kept']} of {fit['anchors']} named points, median miss {fit['median']:.1f}, "
          f"scale {fit['scale'][0]:.4f} {fit['scale'][1]:.4f}, offset {fit['offset'][0]:.1f} {fit['offset'][1]:.1f}")
    per = collections.Counter(r[1] for r in rows)
    for t in towns:
        att = sum(1 for n in t["members"] if not (n["type"] & 0x80))
        if not t["citizens"]:
            print(f"  {t['name']}: no Citizens faction ({att} attackable townsfolk left alone)")
            continue
        g = ", ".join(f"{x} ({per[x]})" for x in t["guards"]) or "none"
        print(f"  {t['name']}: {att} attackable townsfolk; {t['citizens']} ({per[t['citizens']]} names); guards: {g}")
    print(f"{len(rows)} rows on {len({r[0] for r in rows})} names; {len(skipped)} names left to the GM:")
    for name, why in skipped:
        print(f"    {name!r}: {why}")
    if not a.apply:
        print("(report only: --apply writes them)")
        return
    have = {(r[0], r[1]) for r in db.execute("SELECT npc_name, faction FROM factionhits")}
    new = [r for r in rows if (r[0], r[1]) not in have]
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    db.execute("BEGIN")
    db.executemany("INSERT OR IGNORE INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)",
                   [(n, f, d, SOURCE) for n, f, d in new])
    db.execute("COMMIT")
    print(f"wrote {len(new)} rows ({len(rows) - len(new)} were there already); restart the server to read them")


if __name__ == "__main__":
    main()
