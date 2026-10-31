#!/usr/bin/env python3
"""Newbie wildlife spread round a town whose creatures stand in one clump.

F3 20261030-224311 (eqoaclassic): "Tethelin - nowhere near enough low-level
wildlife, especially sparkflies." Round Tethelin the creatures of level 5 or
less are not few (432 within 2500 of the town, Freeport 213) but packed:
three 500-unit squares east of the town hold about a hundred each, and the
ground west, north and south of it almost none, so a new elf met nothing
near the town, and three of Tethelin's first quests want sparkflies' wings.
The user chose more wildlife there (2026-10-31). seed_wildlife.py seeds an
area round FrontiersForge's sightings and allows one seed at a time; this
fills the empty ground round a town instead, and keeps its own log.

What it decides, and from what:

- TOWNS: the town's middle, the ring round it to fill, the kinds it fills
  it with and their shares (the generic creatures of level 5 or less that
  already live round the town; sparkflies the most), and the land's floor.
- The ring is cut into CELL squares. A square gets spawns until it holds
  PER_CELL creatures of level 5 or less, the ones already there counted, so
  the clump gets none and the empty ground its share; each spawn is a kind
  drawn by its share.
- A spawn copies a namesake row (seed_wildlife.template: of the most common
  body, the nearest), so it is drawn, sized and fights as the server's
  others of its kind; its level grows outward from the town (LEVELS by its
  distance), kept to its namesakes' levels.
- Where: on the terrain (seed_wildlife.Ground: the floor nearest the land,
  at most MAX_DROP under it or MAX_RISE over it, never under the land
  itself), never in water, slime, lava or mud (the world's environment
  file, internal/ground/env.go: the game's own BSP and its leaves' flags),
  TOWN_CLEAR from any town's people (seed_wildlife.town_people, as
  entity/towns.go finds them) and SPACING from every other NPC.

Every row is written in one transaction with npc_id 0 (no loot table) and
logged in town_wildlife_seed_npcs; --undo deletes exactly a town's rows,
puts sqlite_sequence back when nothing was added after, and drops the log
when it is empty, so every other row is as it was. A town already seeded is
refused. Positions come from a fixed random seed: the same database and
ground give the same plan. Report only by default. The server reads npcs at
its start: apply it with the server stopped (a swap's before.sh), or
restart after.

    tools/seed_town_wildlife.py --town Tethelin --ground worlds/ground [--db eqoa.db]
        [--apply | --undo] [--seed N] [--verbose]
"""
import argparse
import collections
import datetime
import math
import os
import random
import sqlite3
import struct
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_wildlife as sw  # noqa: E402

DEFAULT_DB = sw.DEFAULT_DB
DEFAULT_GROUND = sw.DEFAULT_GROUND
DEFAULT_SEED = 25

# A town: its middle (the people's, roughly), the ring to fill (from its
# middle), the kinds with their shares, and the land's floor (round
# Tethelin the open ground lies at 54.1, the town's platforms at 69-76).
TOWNS = {
    "Tethelin": dict(world=0, x=18300.0, z=7350.0, ring=(0.0, 1500.0), land=54.0,
                     kinds={"sparkflies": 4, "grass snake": 1, "grass viper": 1, "spiderling": 1, "bat": 1,
                            "rat": 1, "emberflies": 1, "fireflies": 1, "hatchling": 1}),
}

CELL = 250.0
PER_CELL = 2
NEWBIE_LEVEL = 5              # what counts toward PER_CELL: creatures of this level or less
LEVELS = ((500.0, 1, 2), (1000.0, 2, 3), (1e9, 3, 4))   # out to this far: these levels
TOWN_CLEAR = sw.TOWN_CLEAR
SPACING = sw.SPACING
MAX_DROP, MAX_RISE = 12.0, 25.0
TRIES = 60
BAD_ENV = 1 | 2 | 4 | 8       # water, slime, lava, mud (GetEnvFlags)


# ---- the world's environment (internal/ground/env.go) ------------------------------

LEAF = 0x40000000
INDEX = 0x0FFFFFFF


class Env:
    """A world's environment file: "EQEV", version 1, the world's tree (node
    and leaf counts, each node a plane nx, ny, nz, d as float32 and its
    front and back as uint32, a leaf one with LEAF set; each leaf a zone
    index, int32), then the zone count and each zone's tree the same, its
    leaves the flags (1 water, 2 slime, 4 lava, 8 mud)."""

    def __init__(self, path):
        with open(path, "rb") as f:
            data = f.read()
        if data[:4] != b"EQEV":
            raise ValueError(f"{path}: not an environment file")
        (ver,) = struct.unpack_from("<I", data, 4)
        if ver != 1:
            raise ValueError(f"{path}: version {ver}, this tool reads 1")
        p = 8

        def tree(p):
            nodes, leaves = struct.unpack_from("<II", data, p)
            p += 8
            ns = [struct.unpack_from("<4fII", data, p + 24 * i) for i in range(nodes)]
            p += 24 * nodes
            ls = list(struct.unpack_from(f"<{leaves}I", data, p))
            p += 4 * leaves
            return (ns, ls), p

        self.tree, p = tree(p)
        (zones,) = struct.unpack_from("<I", data, p)
        p += 4
        self.zones = []
        for _ in range(zones):
            t, p = tree(p)
            self.zones.append(t)

    @staticmethod
    def leaf(t, x, y, z):
        nodes, leaves = t
        at = 0
        if nodes:
            for steps in range(len(nodes) + 1):
                i = at & INDEX
                if i >= len(nodes):
                    return None
                nx, ny, nz, d, front, back = nodes[i]
                at = front if x * nx + y * ny + z * nz + d >= 0 else back
                if at & LEAF:
                    break
            else:
                return None
            at &= INDEX
        return leaves[at] if at < len(leaves) else None

    def flags(self, x, y, z):
        """What (x, y, z) is in: its zone leaf's flags, None outside every zone."""
        zi = self.leaf(self.tree, x, y, z)
        if zi is None or zi >= 0x80000000 or zi >= len(self.zones):
            return None
        return self.leaf(self.zones[zi], x, y, z)


# ---- the plan ----------------------------------------------------------------------

def level_at(d):
    for far, lo, hi in LEVELS:
        if d <= far:
            return lo, hi
    return LEVELS[-1][1], LEVELS[-1][2]


def plan(db, ground_dir, town_name, seed=DEFAULT_SEED):
    town = TOWNS[town_name]
    world = town["world"]
    cols, npcs, pc, spells, _ = sw.load(db)
    kinds, why = sw.creature_kinds(npcs, pc, spells)
    towns, town_count = sw.town_people(npcs, pc)
    gpath = os.path.join(ground_dir, f"{world}.ground")
    epath = os.path.join(ground_dir, f"{world}.env")
    if not os.path.exists(gpath):
        sys.exit(f"no ground file {gpath}")
    if not os.path.exists(epath):
        sys.exit(f"no environment file {epath}: the water could not be kept out")
    g, env = sw.Ground(gpath), Env(epath)
    taken = sw.Grid(64.0)
    for n in npcs:
        taken.add(n["world"], n["x"], n["z"], n["key"])
    left = [(k, why.get(k, "no body: no NPC of that name")) for k in town["kinds"] if k not in kinds]
    shares = [(k, w) for k, w in sorted(town["kinds"].items()) if k in kinds]
    total = sum(w for _, w in shares)
    cx, cz, r0, r1 = town["x"], town["z"], town["ring"][0], town["ring"][1]
    # the newbies already in each square
    have = collections.Counter()
    for n in npcs:
        if n["world"] == world and n["level"] <= NEWBIE_LEVEL and n["key"] in kinds:
            have[(math.floor(n["x"] / CELL), math.floor(n["z"] / CELL))] += 1
    rng = random.Random(seed)
    rows, cells, why_not = [], [], collections.Counter()
    i0, i1 = math.floor((cx - r1) / CELL), math.floor((cx + r1) / CELL)
    j0, j1 = math.floor((cz - r1) / CELL), math.floor((cz + r1) / CELL)
    for i in range(i0, i1 + 1):
        for j in range(j0, j1 + 1):
            mx, mz = (i + 0.5) * CELL, (j + 0.5) * CELL
            d = math.hypot(mx - cx, mz - cz)
            if not r0 <= d <= r1:
                continue
            want = max(0, PER_CELL - have[(i, j)])
            got = 0
            for _ in range(want):
                for _ in range(TRIES):
                    x, z = rng.uniform(i * CELL, (i + 1) * CELL), rng.uniform(j * CELL, (j + 1) * CELL)
                    if not r0 <= math.hypot(x - cx, z - cz) <= r1:
                        why_not["off the ring"] += 1
                        continue
                    y = g.floor_near(x, z, town["land"], MAX_DROP, MAX_RISE)
                    if y is None or y < town["land"]:
                        why_not["no land"] += 1
                        continue
                    f = env.flags(x, y + 1.0, z)
                    if f is None or f & BAD_ENV:
                        why_not["water or no zone"] += 1
                        continue
                    if next(towns.near(world, x, z, TOWN_CLEAR), None) is not None:
                        why_not["the town"] += 1
                        continue
                    if next(taken.near(world, x, z, SPACING), None) is not None:
                        why_not["another NPC"] += 1
                        continue
                    pick = rng.uniform(0, total)
                    for key, w in shares:
                        pick -= w
                        if pick <= 0:
                            break
                    rows_k = kinds[key]
                    t = sw.template(rows_k, world, x, z)
                    lo, hi = level_at(math.hypot(x - cx, z - cz))
                    near = [n["level"] for n in rows_k if n["world"] == world]
                    lo, hi = max(lo, min(near)), min(hi, max(near))
                    lv = rng.randint(min(lo, hi), max(lo, hi))
                    row = dict(t["row"])
                    row.pop("id", None)
                    row.update(npc_name=sw.spelling(rows_k), x_coord=round(x, 3), y_coord=round(y, 4),
                               z_coord=round(z, 3), facing=rng.randrange(256), world=world, npc_level=lv,
                               npc_id=0, dt_kill=None, zone=0)
                    rows.append(dict(row=row, town=town_name, template=t["id"], kind=key))
                    taken.add(world, x, z, key)
                    got += 1
                    break
            cells.append(dict(cell=(i, j), d=d, had=have[(i, j)], want=want, got=got))
    return dict(cols=cols, rows=rows, cells=cells, left=left, why_not=why_not, towns=town_count)


# ---- writing it, and taking it back out ---------------------------------------------

LOG_DDL = [
    "CREATE TABLE IF NOT EXISTS town_wildlife_seed_meta (town TEXT NOT NULL, key TEXT NOT NULL, "
    "value TEXT NOT NULL, PRIMARY KEY (town, key))",
    "CREATE TABLE IF NOT EXISTS town_wildlife_seed_npcs (npc_row INTEGER PRIMARY KEY, town TEXT NOT NULL, "
    "kind TEXT NOT NULL, template_row INTEGER NOT NULL)",
]


def logged(db):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'town_wildlife_seed_npcs'").fetchone()[0] > 0


def seeded(db, town):
    return logged(db) and db.execute("SELECT count(*) FROM town_wildlife_seed_meta WHERE town = ?",
                                     (town,)).fetchone()[0] > 0


def apply(db, p, town, seed):
    if seeded(db, town):
        sys.exit(f"{town} is seeded already (town_wildlife_seed_npcs): --undo it first")
    cols = [c for c in p["cols"] if c != "id"]
    db.execute("BEGIN IMMEDIATE")
    try:
        for stmt in LOG_DDL:
            db.execute(stmt)
        seq = sw.npc_seq(db)
        meta = dict(when=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                    seq_before="" if seq is None else str(seq), seed=str(seed), rows=str(len(p["rows"])))
        for r in p["rows"]:
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r["row"].get(c) for c in cols])
            db.execute("INSERT INTO town_wildlife_seed_npcs VALUES (?, ?, ?, ?)",
                       (cur.lastrowid, town, r["kind"], r["template"]))
        db.executemany("INSERT INTO town_wildlife_seed_meta VALUES (?, ?, ?)",
                       [(town, k, v) for k, v in sorted(meta.items())])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db, town):
    if not seeded(db, town):
        sys.exit(f"no wildlife seed for {town} to undo here")
    meta = dict(db.execute("SELECT key, value FROM town_wildlife_seed_meta WHERE town = ?", (town,)))
    db.execute("BEGIN IMMEDIATE")
    try:
        n = db.execute("DELETE FROM npcs WHERE id IN (SELECT npc_row FROM town_wildlife_seed_npcs WHERE town = ?)",
                       (town,)).rowcount
        db.execute("DELETE FROM town_wildlife_seed_npcs WHERE town = ?", (town,))
        db.execute("DELETE FROM town_wildlife_seed_meta WHERE town = ?", (town,))
        # AUTOINCREMENT's counter back, unless a row was added after the seed
        before = meta.get("seq_before", "")
        top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
        if before == "":
            if top == 0:
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
        elif top <= int(before):
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (int(before),))
        if db.execute("SELECT count(*) FROM town_wildlife_seed_meta").fetchone()[0] == 0:
            db.execute("DROP TABLE town_wildlife_seed_npcs")
            db.execute("DROP TABLE town_wildlife_seed_meta")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-townwildlife"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(p, town, verbose=False):
    t = TOWNS[town]
    print(f"{town} (world {t['world']}, round ({t['x']:.0f}, {t['z']:.0f}) to {t['ring'][1]:.0f}): "
          f"{len(p['cells'])} squares of {CELL:.0f}, {sum(c['had'] for c in p['cells'])} newbie creatures in them, "
          f"{len(p['rows'])} spawns")
    filled = [c for c in p["cells"] if c["want"]]
    short = [c for c in filled if c["got"] < c["want"]]
    print(f"    {len(filled)} squares under {PER_CELL}; {len(short)} found less room than they wanted "
          f"({sum(c['want'] - c['got'] for c in short)} spawns: no land, water, the town or other NPCs)")
    for k, n in sorted(collections.Counter(r["kind"] for r in p["rows"]).items(), key=lambda kv: -kv[1]):
        lv = collections.Counter(r["row"]["npc_level"] for r in p["rows"] if r["kind"] == k)
        print(f"    {k}: {n} (levels {dict(sorted(lv.items()))})")
    if p["why_not"]:
        print("    places tried and refused: " + ", ".join(f"{k} {v}" for k, v in p["why_not"].most_common()))
    for k, why in p["left"]:
        print(f"    left out: {k!r}: {why}")
    if verbose:
        for r in p["rows"]:
            row = r["row"]
            print(f"  {row['npc_name']!r} L{row['npc_level']} at ({row['x_coord']:.1f}, {row['y_coord']:.2f}, "
                  f"{row['z_coord']:.1f}) copy of {r['template']}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    ap.add_argument("--town", default="Tethelin", choices=sorted(TOWNS))
    ap.add_argument("--apply", action="store_true", help="write the spawns")
    ap.add_argument("--undo", action="store_true", help="delete every spawn the town's seed wrote")
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
        print(f"deleted {undo(db, args.town)} seeded spawns round {args.town}; restart the server")
        return
    if seeded(db, args.town):
        n = db.execute("SELECT count(*) FROM town_wildlife_seed_npcs WHERE town = ?", (args.town,)).fetchone()[0]
        print(f"{args.town} is seeded already ({n} spawns): --undo it to plan again")
        return
    p = plan(db, args.ground, args.town, seed=args.seed)
    report(p, args.town, args.verbose)
    if not args.apply:
        print(f"{len(p['rows'])} spawns (report only: --apply writes them)")
        return
    if not args.no_backup:
        print("backup:", backup(db, args.db))
    apply(db, p, args.town, args.seed)
    print(f"wrote {len(p['rows'])} spawns round {args.town}; restart the server to read them")


if __name__ == "__main__":
    main()
