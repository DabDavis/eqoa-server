#!/usr/bin/env python3
"""The loot tables' dead items brought to life: the regional tables no
creature rolled bound to their regions' creatures, and a table's items no
bound creature's level reaches given the creatures of its biome at that level
(the user, 2026-10-30, of tools/drop_rates.py's report: 1,186 items of
loottables never dropped, 888 in tables bound to no creature and 298 in
windows no bound creature's level reached).

tools/seed_camp_loot.py wrote EQOA-Data's biome tables and bound only the 49
XP camps' creatures (a camp's zone quadrant's biome on EQOA-Data's map, and
"generic"). So:

  - **rathe mountains** (295 items, levels 5-60): the Rathe's creatures
    (world 1), the region the table is named for.
  - **marine** (206, levels 9-49: coral, brine, sharks' and eels' teeth):
    Tunaria's creatures of the water (most of their spawns standing in water
    or slime, the zone leaves' flags of --ground's <world>.env, as the server
    has them), and those of the quadrants EQOA-Data's map calls Marine.
  - **red desert, deep jungle, wetlands, wastelands** (383 items): no zone of
    the map carries these biomes, and Odus's zones carry none. The items'
    levels (all but one 30-60, the deep jungle's 41-60) are Odus's creatures'
    (world 2: 2,385 spawns, 2,124 of them 40-69) and not Lavastorm's (none
    below 50), and their names are Odus's (Kerran Claw Ring, Abysmal Plate
    Helm, Barren Ring, Rhino Horn Earring, Pirate's Scimitar: its kerrans,
    Abyssmal Sea, Barren Coast, rhinoceroses and indigo pirates), so all four
    go to Odus's creatures, sharing a biome's chance by their sizes (a kill
    there rolls about as often as a camp's biome, each item about as often as
    another's; at least 1: red desert 10, wetlands 8, deep jungle 7,
    wastelands 1). Inferred: no record names their zones.
  - **every other biome table** (arctic, desert, forest, mountain, plains,
    swamp): an item whose level window no creature bound to the table reaches
    is reached by binding the creatures of that biome (the commonest biome of
    their spawns' quadrants) whose levels its window holds.
  - a window still dead after that is stretched to the nearest level a bound
    creature has, within STRETCH (the Rathe has no creature of a kind at
    levels 29-42: its 23 items there reach 28 or 43).
  - **khal viceroy, khal warlord**: named creatures this server has no spawn
    of; left, listed.

A creature is a name (npcloot binds names as NPCClassKey keys them) of a kind,
spelled in lower case (the rename of NPC names: a capital is a person's or a
named creature's: "Deputy Allen", "Old Red"), that can be attacked (a spawn
without the NPC bit), is no town's person (no name-wide npcfaction row) and no
training dummy; its region is the commonest of its spawns'. A binding is
written only where the table has an item for one of the name's spawn levels,
never over a row already there (a GM's, the camp seed's). Chances:
BIOME_CHANCE (the camp seed's biome chance) but Odus's; one roll. Ours, as the
camp seed's are. Run tools/double_loot_chances.py after this one to double
these too, and undo in the other order.

On the live data of 2026-10-30 01:30 UTC: 948 bindings for 508 names and 23
windows; 1,186 dead items to 4 (the Khal Viceroy's and Warlord's).

Every binding written and window changed is logged in region_loot_seed;
--undo takes back those still as written. Report only by default. The server
reads npcloot and loottables at every corpse: no restart.

    python3 -E -s tools/seed_region_loot.py --db eqoa.db --eqoa-data ../external/EQOA-Data \
        --ground worlds/ground [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import os
import shutil
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import gen_camp_teles as camps  # noqa: E402
import seed_camp_loot as campseed  # noqa: E402
from seed_pod import Env  # noqa: E402

SOURCE = "region-seed"
BIOME_CHANCE = 25
ROLLS = 1
STRETCH = 10  # levels a dead item's window may be stretched to reach a bound creature
RATHE = "rathe mountains"
MARINE = "marine"
ODUS = ("red desert", "deep jungle", "wetlands", "wastelands")
NAMED = ("khal viceroy", "khal warlord")
BIOMES = ("arctic", "desert", "forest", "mountain", "plains", "swamp")
WORLD_RATHE, WORLD_ODUS = 1, 2

LOG_DDL = """CREATE TABLE IF NOT EXISTS region_loot_seed (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind      TEXT NOT NULL,   -- binding (an npcloot row written) | window (a loottables row's levels)
    npc_name  TEXT NOT NULL,   -- binding: the name; window: ''
    tablename TEXT NOT NULL,
    lootrow   INTEGER NOT NULL, -- window: loottables.id; binding: 0
    old       TEXT NOT NULL,   -- window: [minlevel, maxlevel] before; binding: ''
    new       TEXT NOT NULL,   -- binding: [chance, rolls, note]; window: [minlevel, maxlevel]
    applied   TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def creatures(db, data, ground=None):
    """The creatures of a kind, {name: (region, Counter of levels)}, region
    the commonest of its spawns' (a world's for the Rathe and Odus, a map
    biome's for Tunaria, None elsewhere); and every name that can be attacked,
    {name: Counter of levels} (what a bound table reaches)."""
    people = set()
    if has_table(db, "npcfaction"):
        people = {campseed.npc_key(n) for (n,) in db.execute("SELECT name FROM npcfaction WHERE radius = 0")}
    zones, rows = camps.load(db, data) if data else ({"zones": [], "towns": [], "pois": []}, [])
    fitted = camps.fits(zones, rows) if data else {}
    envs = {}

    def wet(world, x, y, z):
        """Whether a spawn stands in water or slime (the zone leaves' flags,
        half a unit above its feet, as the server has it); None without the
        world's .env."""
        if not ground:
            return None
        if world not in envs:
            path = os.path.join(ground, f"{world}.env")
            envs[world] = Env(path) if os.path.exists(path) else None
        e = envs[world]
        return None if e is None else e.flags(x, y + 0.5, z) & 3 != 0
    votes = collections.defaultdict(collections.Counter)
    levels = collections.defaultdict(collections.Counter)
    every = collections.defaultdict(collections.Counter)
    for name, world, x, y, z, lvl, typ in db.execute(
            "SELECT npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type FROM npcs WHERE npc_name IS NOT NULL"):
        k = campseed.npc_key(name)
        if not k or (typ or 0) & 0x80:
            continue
        every[k][lvl or 0] += 1
        # a creature of a kind is spelled in lower case (tools/fix_npc_name_case.py: "crow", "fierce
        # shark"); a capital is a person's or a named creature's ("Deputy Allen", "Old Red")
        if k in people or "dummy" in k or name.strip() != name.strip().lower():
            continue
        if world == WORLD_RATHE:
            region = RATHE
        elif world == WORLD_ODUS:
            region = "odus"
        elif world == 0 and wet(world, x or 0.0, y or 0.0, z or 0.0):
            region = MARINE  # a creature of Tunaria's waters
        elif world == 0 and 0 in fitted:
            (a, b, c, d) = fitted[0][0]
            biome, _ = campseed.biome_at(zones, (x - b) / a, (z - d) / c)
            region = biome.lower() if biome else None
        else:
            region = None
        votes[k][region] += 1
        levels[k][lvl or 0] += 1
    return {k: (v.most_common(1)[0][0], levels[k]) for k, v in votes.items()}, every


def windows(db):
    """Each table's items: {table: [(loottables id, lo, hi)]} (weight above 0)."""
    out = collections.defaultdict(list)
    for t, rid, lo, hi in db.execute("SELECT tablename, id, minlevel, maxlevel FROM loottables WHERE weight > 0"):
        out[t].append((rid, lo, hi))
    return out


def covers(items, levels):
    """Whether any of the levels falls in any item's window."""
    return any(lo <= lv <= hi for _, lo, hi in items for lv in levels)


def plan(db, data, ground=None):
    """The bindings to write [(name, table, chance, note)], and what is left
    [(what, why)]."""
    names, every = creatures(db, data, ground)
    tables = windows(db)
    bound = collections.defaultdict(set)  # table -> names bound
    have = set()
    for n, t in db.execute("SELECT npc_name, tablename FROM npcloot"):
        bound[t].add(n)
        have.add((n, t))
    out, left = [], []

    def bind(name, table, chance, note):
        if (name, table) not in have:
            out.append((name, table, chance, note))
            have.add((name, table))

    # Odus's four tables share a biome's chance by their sizes: a kill there rolls about as often as a camp's
    # biome, and each item about as often as another's (a table of five at a quarter would give each of its
    # items, one a level, five times the rest's)
    sizes = {t: len(tables[t]) for t in ODUS}
    odus_chance = {t: max(1, round(BIOME_CHANCE * sizes[t] / max(1, sum(sizes.values())))) for t in ODUS}
    for name, (region, lv) in sorted(names.items(), key=lambda kv: kv[0]):
        if region == RATHE and covers(tables[RATHE], lv):
            bind(name, RATHE, BIOME_CHANCE, "the Rathe's creature (world 1): the region its table is named for")
        elif region == MARINE and covers(tables[MARINE], lv):
            bind(name, MARINE, BIOME_CHANCE, "a creature of Tunaria's waters (or of its Marine quadrants on EQOA-Data's map)")
        elif region == "odus":
            for t in ODUS:
                if covers(tables[t], lv):
                    bind(name, t, odus_chance[t], "Odus's creature (world 2): the table's items' levels and names are Odus's (inferred)")
    # the biome tables' dead windows: the biome's creatures at those levels
    for t in BIOMES:
        reach = [lv for n in bound[t] for lv in every.get(n, ())]
        dead = [it for it in tables[t] if not any(it[1] <= lv <= it[2] for lv in reach)]
        if not dead:
            continue
        for name, (region, lv) in sorted(names.items()):
            if region == t and covers(dead, lv):
                bind(name, t, BIOME_CHANCE, f"a creature of EQOA-Data's {t} quadrants, at levels the table's bound creatures never reached")
    for t in NAMED:
        if t in tables and t not in every:
            left.append((t, f"no creature named {t!r} spawns here: its {len(tables[t])} items stay unbound"))
    return out, left, every, tables


def stretches(db, every, tables, rows):
    """The windows of the items no bound creature reaches once rows are
    written, stretched to the nearest level a bound creature has, within
    STRETCH of the window: [(loottables id, table, (lo, hi) before, after)];
    and the items left [(table, count, why)]."""
    bound = collections.defaultdict(set)
    for n, t in list(db.execute("SELECT npc_name, tablename FROM npcloot")) + [(n, t) for n, t, _, _ in rows]:
        bound[t].add(n)
    out, left = [], collections.Counter()
    for rid, t, lo, hi in db.execute("SELECT id, tablename, minlevel, maxlevel FROM loottables WHERE weight > 0 ORDER BY id"):
        if t in NAMED or t.startswith("boss ") or "*" in bound[t]:
            continue
        reach = sorted({lv for n in bound[t] for lv in every.get(n, ())})
        if not reach or any(lo <= lv <= hi for lv in reach):
            if not reach:
                left[t] += 1
            continue
        above = [lv for lv in reach if lv > hi and lv - hi <= STRETCH]
        below = [lv for lv in reach if lv < lo and lo - lv <= STRETCH]
        if above and (not below or above[0] - hi <= lo - below[-1]):
            out.append((rid, t, (lo, hi), (lo, above[0])))
        elif below:
            out.append((rid, t, (lo, hi), (below[-1], hi)))
        else:
            left[t] += 1
    return out, left


def dead_items(db, every, tables, extra=()):
    """The table items no bound creature's level reaches: {table: count},
    every the attackable names' levels, the extra bindings [(name, table)]
    counted as written (a table bound to "*" reaches every creature)."""
    bound = collections.defaultdict(set)
    for n, t in list(db.execute("SELECT npc_name, tablename FROM npcloot")) + list(extra):
        bound[t].add(n)
    any_level = {lv for n in every for lv in every[n]}
    out = collections.Counter()
    for t, items in tables.items():
        reach = any_level if "*" in bound[t] else {lv for n in bound[t] for lv in every.get(n, ())}
        for rid, lo, hi in items:
            if not any(lo <= lv <= hi for lv in reach):
                out[t] += 1
    return out


def windows_after(tables, wins):
    """The tables' items with the stretched windows."""
    by = {rid: new for rid, _, _, new in wins}
    out = {}
    for t, items in tables.items():
        out[t] = [(rid, *by.get(rid, (lo, hi))) for rid, lo, hi in items]
    return out


def apply(db, rows, wins=()):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for name, table, chance, note in rows:
            db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                       (name, table, chance, ROLLS, note, SOURCE))
            db.execute("INSERT INTO region_loot_seed (kind, npc_name, tablename, lootrow, old, new, applied) "
                       "VALUES ('binding', ?, ?, 0, '', ?, ?)", (name, table, json.dumps([chance, ROLLS, note]), when))
        for rid, table, old, new in wins:
            db.execute("UPDATE loottables SET minlevel = ?, maxlevel = ? WHERE id = ?", (new[0], new[1], rid))
            db.execute("INSERT INTO region_loot_seed (kind, npc_name, tablename, lootrow, old, new, applied) "
                       "VALUES ('window', '', ?, ?, ?, ?, ?)", (table, rid, json.dumps(list(old)), json.dumps(list(new)), when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "region_loot_seed"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back, kept = 0, []
        for kind, name, table, rid, old, new in db.execute(
                "SELECT kind, npc_name, tablename, lootrow, old, new FROM region_loot_seed ORDER BY seq DESC").fetchall():
            new = json.loads(new)
            if kind == "window":
                cur = db.execute("SELECT minlevel, maxlevel FROM loottables WHERE id = ?", (rid,)).fetchone()
                if cur is not None and list(cur) == new:
                    lo, hi = json.loads(old)
                    db.execute("UPDATE loottables SET minlevel = ?, maxlevel = ? WHERE id = ?", (lo, hi, rid))
                    back += 1
                elif cur is not None:
                    kept.append(f"loottables row {rid}")
                continue
            cur = db.execute("SELECT chance, rolls, note, source FROM npcloot WHERE npc_name = ? AND tablename = ?",
                             (name, table)).fetchone()
            if cur is None:
                continue
            if list(cur) == new + [SOURCE]:
                db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table))
                back += 1
            else:
                kept.append(f"{name!r} on {table!r}")
        db.execute("DROP TABLE region_loot_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {back} changes" + (f"; {len(kept)} changed since left as they are: " + ", ".join(kept[:20]) if kept else ""))


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-regionloot"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout, for its map (Tunaria's biomes)")
    ap.add_argument("--ground", help="the server's ground files (worlds/ground: <world>.env), for Tunaria's waters")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the bindings (logged)")
    g.add_argument("--undo", action="store_true", help="delete the bindings written, as written")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "region_loot_seed"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "region_loot_seed"):
        sys.exit("region_loot_seed is there: applied already (--undo first)")
    if not a.eqoa_data:
        sys.exit("give --eqoa-data (the map's biomes)")
    rows, left, every, tables = plan(db, a.eqoa_data, a.ground)
    wins, still = stretches(db, every, tables, rows)
    before = dead_items(db, every, tables)
    stretched = windows_after(tables, wins)
    after = dead_items(db, every, stretched, [(n, t) for n, t, _, _ in rows])
    per = collections.Counter(t for _, t, _, _ in rows)
    print(f"bindings to write: {len(rows)} ({len({n for n, *_ in rows})} creature names): " +
          ", ".join(f"{t} {n}" for t, n in sorted(per.items())))
    print("items no bound creature reaches, by table, now -> after: " +
          ", ".join(f"{t} {before[t]} -> {after[t]}" for t in sorted(set(before) | set(after)) if before[t] or after[t]))
    print(f"  in all {sum(before.values())} -> {sum(after.values())}")
    print(f"windows stretched to reach a bound creature (at most {STRETCH} levels): {len(wins)} (" +
          ", ".join(f"{t} {n}" for t, n in sorted(collections.Counter(t for _, t, _, _ in wins).items())) + ")")
    for what, why in left:
        print(f"left: {why}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not rows and not wins:
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, rows, wins)
    print(f"wrote {len(rows)} bindings, {len(wins)} windows")


if __name__ == "__main__":
    main()
