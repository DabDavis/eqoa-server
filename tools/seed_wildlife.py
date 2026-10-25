#!/usr/bin/env python3
"""Wildlife for the zones that have none: FrontiersForge's sightings there,
made into spawns of the creatures this server already knows.

F3 20261025-172939 and 20261025-173551 (eqoaclassic): Klick'Anon's newbie
yard and the Rogue Clockworks had no wildlife (Klick'Anon held three
creatures of level 6 or less within 2500 of the town, where every other
city has 90 to 340). No capture or import covers them; FrontiersForge's
journal (npcs_ff_import, tools/import_ff_npcs.py) saw a creature or two of
each kind there. This tool turns each sighting into a few spawns round it.

What it decides, and from what:

- The areas are CLIENT's zone grid (ZoneName, the decomp's vientity.cpp):
  a 2000-unit cell of a world, named by a WORDS.TXT word. AREAS below holds
  the two the reports name; each has the level band its creatures keep to.
- A sighting is used when its creature is GENERIC: the same name (case
  aside) is a row of npcs at least twice, its body is no players' race
  (charactermodel), it has no 0x80 (not a townsperson, merchant or
  traveller) and it is no pet's body (no spell has its name, and it is no
  enchanter's "animation": entity/pet.go makes pets after those rows). A
  named creature (Blackpaw, Unit I) is never copied.
- The body is the namesake's: every column of one namesake row (the most
  common model's, the nearest such row), so a seeded ant is drawn, sized,
  flagged and fights as the server's other ants. A name with no namesake is
  left out and listed: guessing a body from a name is wrong too often
  (the head noun gives "Blackpaw", a bear, the "gnoll blackpaw"'s body).
- How many: BAND_COUNT by the sighting's level, less the namesakes already
  within the spread of it; each spawn's level the sighting's or one off,
  kept to the band and to its namesakes' range.
- Where: within the spread of the sighting, inside the area's cell, on the
  terrain (the operator's ground files, cmd/groundbake: the floor nearest
  the sighting's own, at most MAX_DROP_FROM_ANCHOR below and MAX_RISE above
  it, and not below the area's land, which keeps them out of the lakes and
  off the mountainsides), at least TOWN_CLEAR from any town's people
  (entity/towns.go's towns, so a wander of the roam radius stays out of
  them), and SPACING from every other NPC. A sighting in a
  camp (CAMP_FOLK NPCs within CAMP_RADIUS: the clockwork village, the
  coldpaw gnolls) gives CAMP_COUNT spawns spread round the camp instead
  (CAMP_SPREAD), so its creatures are met outside it. --skip leaves kinds
  out by name.
- --with-guide-species also seeds the creatures EQOA-Data's Klick'Anon
  quest guides send new players after (bats, rats, snakes, fireflies) where
  FrontiersForge saw that yard's level-2 creatures: no journal recorded
  them there, so they are off unless asked for.

Every row is written in one transaction with npc_id 0 (no loot table) and
logged in wildlife_seed_npcs with its source ('ff-sighting' or
'eqoa-data-guide'); --undo deletes exactly those rows, puts sqlite_sequence
back when nothing was added after, and drops the log, so every other row is
as it was. A second --apply is refused while a seed is in. Positions come
from a fixed random seed: the same database and ground give the same plan.
Report only by default. Run it against a copy, or with the server stopped
(it reads npcs at start), and restart the server after.

    tools/seed_wildlife.py --ground worlds/ground [--db eqoa.db]
        [--apply | --undo] [--with-guide-species] [--skip "coldpaw shaman,..."]
        [--seed N] [--verbose]
"""
import argparse
import collections
import datetime
import hashlib
import math
import os
import random
import sqlite3
import struct
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
DEFAULT_GROUND = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "worlds", "ground")

SRC_FF = "ff-sighting"
SRC_GUIDE = "eqoa-data-guide"

# The areas: a ZoneName cell (2000 units; col = x / 2000, row = z / 2000,
# the word ZONES0[row - 1][col - 1]), the levels its wildlife keeps to, and
# its land's lowest floor: the ground files carry no water (the game's water
# is a zone volume), and round these two the open ground lies at 54.15 with
# lakes and the sea in hollows below it.
AREAS = [
    dict(name="Klick'Anon", word=361, world=0, x=(22000.0, 24000.0), z=(6000.0, 8000.0), levels=(1, 12), land=54.0),
    dict(name="Rogue Clockworks", word=1174, world=0, x=(24000.0, 26000.0), z=(6000.0, 8000.0), levels=(11, 23),
         land=54.0),
]

# EQOA-Data's Klick'Anon guides (Quests/1-20/klick-anon): "slay bats",
# "slay rats out past the Coachman", "hunt snakes", "kill the various flies".
GUIDE_SPECIES = {"Klick'Anon": ["bat", "rat", "grass snake", "fireflies"]}

# spawns a sighting becomes, by its level: a newbie yard's creatures in
# number, a camp's fewer
BAND_COUNT = ((5, 10), (10, 8), (99, 4))
# how far round a sighting they go (min, max), by the same bands
BAND_SPREAD = ((5, (30.0, 260.0)), (10, (30.0, 200.0)), (99, (40.0, 220.0)))
CAMP_FOLK = 5                 # this many NPCs this close to a sighting make it a camp...
CAMP_RADIUS = 150.0
CAMP_SPREAD = (160.0, 460.0)  # ...and its creature's seeds go round it, not in it
CAMP_COUNT = 4
TOWN_LINK, TOWN_MIN_FOLK = 60.0, 10   # entity/towns.go
TOWN_CLEAR = 120.0            # its margin (40) and a wander (the roam radius, 75)
SPACING = 20.0
MAX_DROP_FROM_ANCHOR, MAX_RISE = 40.0, 25.0
AREA_INSET = 10.0
TRIES = 200
DEFAULT_SEED = 15


# ---- the ground -------------------------------------------------------------------

class Ground:
    """A world's terrain from its ground file (internal/ground's format):
    "EQGR", version 1, the zone count, each zone's entry (min x, min z,
    max x, max z, base x, y, z as float32, the strips' offset as int64, the
    strip and vertex counts as uint32), then each zone's strips (a uint16
    vertex count, then int16 x, y, z: x and z in quarters of a unit from the
    base, less 32768, y in eighths). Only floors count, as the server's:
    triangles facing up more than sideways."""

    CELL = 8.0

    def __init__(self, path):
        self.f = open(path, "rb")
        head = self.f.read(12)
        if head[:4] != b"EQGR":
            raise ValueError(f"{path}: not a ground file")
        version, count = struct.unpack("<II", head[4:])
        if version != 1:
            raise ValueError(f"{path}: version {version}, this tool reads 1")
        self.entries = []
        for _ in range(count):
            e = struct.unpack("<7fqII", self.f.read(44))
            self.entries.append(e)
        self.zones = {}

    def _zone(self, i):
        if i in self.zones:
            return self.zones[i]
        minx, minz, maxx, maxz, bx, by, bz, off, strips, verts = self.entries[i]
        self.f.seek(off)
        buf = self.f.read(strips * 2 + verts * 6)
        tris = []
        p = 0
        for _ in range(strips):
            (n,) = struct.unpack_from("<H", buf, p)
            p += 2
            vs = []
            for k in range(n):
                qx, qy, qz = struct.unpack_from("<hhh", buf, p + 6 * k)
                vs.append((bx + (qx + 32768) / 4.0, by + qy / 8.0, bz + (qz + 32768) / 4.0))
            p += 6 * n
            for k in range(n - 2):
                t = (vs[k], vs[k + 1], vs[k + 2])
                if floor(t):
                    tris.append(t)
        grid = collections.defaultdict(list)
        c = self.CELL
        for t in tris:
            x0 = int((min(v[0] for v in t) - minx) // c)
            x1 = int((max(v[0] for v in t) - minx) // c)
            z0 = int((min(v[2] for v in t) - minz) // c)
            z1 = int((max(v[2] for v in t) - minz) // c)
            for cx in range(x0, x1 + 1):
                for cz in range(z0, z1 + 1):
                    grid[(cx, cz)].append(t)
        self.zones[i] = (minx, minz, grid)
        return self.zones[i]

    def floors(self, x, z):
        """Every floor's height at (x, z), highest first."""
        out = []
        for i, e in enumerate(self.entries):
            minx, minz, maxx, maxz = e[0], e[1], e[2], e[3]
            if e[9] == 0 or x < minx or x > maxx or z < minz or z > maxz:
                continue
            zminx, zminz, grid = self._zone(i)
            for t in grid.get((int((x - zminx) // self.CELL), int((z - zminz) // self.CELL)), ()):
                h = height_at(t, x, z)
                if h is not None:
                    out.append(h)
        return sorted(set(out), reverse=True)

    def floor_near(self, x, z, y, below, above):
        """The floor at (x, z) nearest y, no more than below under it and
        above over it; None where there is none."""
        best = None
        for h in self.floors(x, z):
            if y - below <= h <= y + above and (best is None or abs(h - y) < abs(best - y)):
                best = h
        return best


def floor(t):
    (ax, ay, az), (bx, by, bz), (cx, cy, cz) = t
    ux, uy, uz = bx - ax, by - ay, bz - az
    vx, vy, vz = cx - ax, cy - ay, cz - az
    nx, ny, nz = uy * vz - uz * vy, uz * vx - ux * vz, ux * vy - uy * vx
    l2 = nx * nx + ny * ny + nz * nz
    return l2 > 1e-8 and ny * ny > 0.25 * l2


def height_at(t, x, z):
    (ax, ay, az), (bx, by, bz), (cx, cy, cz) = t
    d = (bz - cz) * (ax - cx) + (cx - bx) * (az - cz)
    if d == 0:
        return None
    w0 = ((bz - cz) * (x - cx) + (cx - bx) * (z - cz)) / d
    w1 = ((cz - az) * (x - cx) + (ax - cx) * (z - cz)) / d
    w2 = 1 - w0 - w1
    if w0 < -1e-4 or w1 < -1e-4 or w2 < -1e-4:
        return None
    return w0 * ay + w1 * by + w2 * cy


# ---- the plan ---------------------------------------------------------------------

def band(table, level):
    for top, v in table:
        if level <= top:
            return v
    return table[-1][1]


class Grid:
    """Points of a world by cell, for "is anything within r of here"."""

    def __init__(self, cell):
        self.cell = cell
        self.cells = collections.defaultdict(list)

    def add(self, world, x, z, item=None):
        self.cells[(world, int(x // self.cell), int(z // self.cell))].append((x, z, item))

    def near(self, world, x, z, r):
        n = int(math.ceil(r / self.cell))
        cx, cz = int(x // self.cell), int(z // self.cell)
        for dx in range(-n, n + 1):
            for dz in range(-n, n + 1):
                for px, pz, item in self.cells.get((world, cx + dx, cz + dz), ()):
                    if math.hypot(px - x, pz - z) <= r:
                        yield px, pz, item


def town_people(npcs, pc):
    """entity/towns.go's towns: chains of NPCs with a players' race body,
    each within TOWN_LINK of the next, of at least TOWN_MIN_FOLK, by world."""
    folk = [n for n in npcs if n["model"] in pc]
    parent = list(range(len(folk)))

    def find(i):
        while parent[i] != i:
            parent[i] = parent[parent[i]]
            i = parent[i]
        return i

    g = Grid(TOWN_LINK)
    for i, n in enumerate(folk):
        g.add(n["world"], n["x"], n["z"], i)
    for i, n in enumerate(folk):
        for _, _, j in g.near(n["world"], n["x"], n["z"], TOWN_LINK):
            if j > i:
                a, b = find(i), find(j)
                if a != b:
                    parent[a] = b
    size = collections.Counter(find(i) for i in range(len(folk)))
    out = Grid(64.0)
    count = 0
    for i, n in enumerate(folk):
        if size[find(i)] >= TOWN_MIN_FOLK:
            out.add(n["world"], n["x"], n["z"])
            count += 1
    return out, count


def load(db):
    db.row_factory = sqlite3.Row
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    npcs = []
    for r in db.execute("SELECT * FROM npcs ORDER BY id"):
        d = dict(r)
        npcs.append(dict(row=d, id=d["id"], name=d["npc_name"] or "", key=(d["npc_name"] or "").strip().lower(),
                         world=d["world"] or 0, x=d["x_coord"] or 0.0, y=d["y_coord"] or 0.0, z=d["z_coord"] or 0.0,
                         model=(d["modelid"] or 0) & 0xFFFFFFFF, level=d["npc_level"] or 1, type=d["npc_type"] or 0))
    pc = {r[0] & 0xFFFFFFFF for r in db.execute("SELECT modelid FROM charactermodel")}
    spells = {(r[0] or "").strip().lower() for r in db.execute("SELECT spellname FROM spellpattern")}
    ff = [dict(r) for r in db.execute(
        "SELECT id, npc_name, npc_level, world, x_coord, y_coord, z_coord, pos_suspect FROM npcs_ff_import ORDER BY id")]
    db.row_factory = None
    return cols, npcs, pc, spells, ff


def creature_kinds(npcs, pc, spells):
    """name (case aside) -> the rows a seed may copy, for the generic
    creatures; and name -> why a name is no generic creature."""
    by = collections.defaultdict(list)
    for n in npcs:
        if n["key"]:
            by[n["key"]].append(n)
    kinds, why = {}, {}
    for key, rows in by.items():
        usable = [n for n in rows if n["model"] and n["model"] not in pc and not (n["type"] & 0x80)]
        if key in spells or key.split()[-1:] == ["animation"]:
            # a summoned pet's body (entity/pet.go makes a pet after the NPCs
            # of its spell's name, an enchanter's after the "animation"s)
            why[key] = "a pet's body (a spell's name, or an enchanter's animation)"
        elif not usable:
            why[key] = "a players' race body or a friendly NPC (0x80)"
        elif len(usable) < 2:
            why[key] = "one of a kind (a named creature)"
        else:
            kinds[key] = usable
    return kinds, why


def template(rows, world, x, z):
    """The namesake row a seed copies: of the most common body, the nearest."""
    common = collections.Counter(n["model"] for n in rows).most_common(1)[0][0]
    pick = [n for n in rows if n["model"] == common]
    return min(pick, key=lambda n: (n["world"] != world, math.hypot(n["x"] - x, n["z"] - z), n["id"]))


def spelling(rows):
    """A kind's name as most of its rows spell it, trimmed ("Ant " and
    "ant" are both in npcs): the lower-case one on a tie, as the game writes
    a creature that is no one in particular."""
    count = collections.Counter(n["name"].strip() for n in rows)
    return max(count, key=lambda k: (count[k], k[:1].islower(), k))


def plan(db, ground_dir, areas=AREAS, guide=False, seed=DEFAULT_SEED, skip=()):
    cols, npcs, pc, spells, ff = load(db)
    kinds, why = creature_kinds(npcs, pc, spells)
    for key in skip:
        if kinds.pop(key, None) is not None:
            why[key] = "left out by --skip"
    towns, town_count = town_people(npcs, pc)
    taken = Grid(64.0)
    for n in npcs:
        taken.add(n["world"], n["x"], n["z"], n["key"])
    rng = random.Random(seed)
    grounds = {}
    rows, groups, left = [], [], []

    def ground(world):
        if world not in grounds:
            path = os.path.join(ground_dir, f"{world}.ground")
            grounds[world] = Ground(path) if os.path.exists(path) else None
        return grounds[world]

    def place(area, kind_rows, name, level, sx, sy, sz, spread, count, source, ff_id):
        """count spawns of a kind round (sx, sz); the rows planned."""
        world = area["world"]
        g = ground(world)
        if g is None:
            left.append((area["name"], name, level, f"no ground file for world {world}"))
            return 0
        anchor = g.floor_near(sx, sz, sy, 40.0, 40.0)
        if anchor is None:
            left.append((area["name"], name, level, "no ground at the sighting"))
            return 0
        lo_n = min(n["level"] for n in kind_rows)
        hi_n = max(n["level"] for n in kind_rows)
        lo = min(level, max(area["levels"][0], lo_n - 1, 1))
        hi = max(level, min(area["levels"][1], hi_n + 1))
        placed = 0
        for _ in range(count):
            for _ in range(TRIES):
                r = math.sqrt(rng.uniform(spread[0] ** 2, spread[1] ** 2))
                a = rng.uniform(0.0, 2.0 * math.pi)
                x, z = sx + r * math.cos(a), sz + r * math.sin(a)
                if not (area["x"][0] + AREA_INSET <= x <= area["x"][1] - AREA_INSET and
                        area["z"][0] + AREA_INSET <= z <= area["z"][1] - AREA_INSET):
                    continue
                y = g.floor_near(x, z, anchor, MAX_DROP_FROM_ANCHOR, MAX_RISE)
                if y is None or y < area["land"]:
                    continue
                if next(towns.near(world, x, z, TOWN_CLEAR), None) is not None:
                    continue
                if next(taken.near(world, x, z, SPACING), None) is not None:
                    continue
                t = template(kind_rows, world, x, z)
                lv = max(lo, min(hi, level + rng.choice((-1, 0, 0, 1))))
                row = dict(t["row"])
                row.pop("id", None)
                row.update(npc_name=spelling(kind_rows), x_coord=round(x, 3), y_coord=round(y, 4), z_coord=round(z, 3),
                           facing=rng.randrange(256), world=world, npc_level=lv, npc_id=0, dt_kill=None, zone=0)
                rows.append(dict(row=row, area=area["name"], source=source, ff_id=ff_id, template=t["id"]))
                taken.add(world, x, z, t["key"])
                placed += 1
                break
        return placed

    for area in areas:
        world = area["world"]
        sightings = [s for s in ff if s["world"] == world and not s["pos_suspect"]
                     and area["x"][0] <= s["x_coord"] < area["x"][1] and area["z"][0] <= s["z_coord"] < area["z"][1]]
        yard = []
        for s in sorted(sightings, key=lambda s: (s["npc_level"], s["npc_name"].lower(), s["id"])):
            name, level, key = s["npc_name"], s["npc_level"], s["npc_name"].strip().lower()
            if not area["levels"][0] <= level <= area["levels"][1]:
                left.append((area["name"], name, level, "out of the area's levels"))
                continue
            if key not in kinds:
                left.append((area["name"], name, level, why.get(key, "no body: no NPC of that name")))
                continue
            sx, sy, sz = s["x_coord"], s["y_coord"], s["z_coord"]
            folk = sum(1 for _ in taken.near(world, sx, sz, CAMP_RADIUS))
            camp = folk >= CAMP_FOLK
            spread = CAMP_SPREAD if camp else band(BAND_SPREAD, level)
            want = CAMP_COUNT if camp else band(BAND_COUNT, level)
            have = sum(1 for x, z, k in taken.near(world, sx, sz, spread[1])
                       if k == key and math.hypot(x - sx, z - sz) >= (spread[0] if camp else 0.0))
            n = max(0, want - have)
            got = place(area, kinds[key], name, level, sx, sy, sz, spread, n, SRC_FF, s["id"]) if n else 0
            groups.append(dict(area=area["name"], name=spelling(kinds[key]), level=level, at=(sx, sz),
                               camp=camp, have=have, want=n, got=got, source=SRC_FF))
            if level <= 5:
                yard.append(s)
        if guide:
            for key in GUIDE_SPECIES.get(area["name"], ()):
                if key not in kinds:
                    left.append((area["name"], key, 0, why.get(key, "no body: no NPC of that name")))
                    continue
                if not yard:
                    left.append((area["name"], key, 0, "no newbie-yard sighting to put it by"))
                    continue
                share = band(BAND_COUNT, 1) // len(yard)
                for s in yard:
                    level = s["npc_level"]
                    got = place(area, kinds[key], key, level, s["x_coord"], s["y_coord"], s["z_coord"],
                                band(BAND_SPREAD, level), share, SRC_GUIDE, None)
                    groups.append(dict(area=area["name"], name=spelling(kinds[key]), level=level,
                                       at=(s["x_coord"], s["z_coord"]), camp=False, have=0, want=share, got=got,
                                       source=SRC_GUIDE))
    return dict(cols=cols, rows=rows, groups=groups, left=left, towns=town_count)


# ---- writing it, and taking it back out ---------------------------------------------

# one row per npcs row written: its area, where it came from (ff-sighting,
# the npcs_ff_import row it is made from, or eqoa-data-guide, the Klick'Anon
# guides) and the namesake row it copies
LOG_DDL = [
    "CREATE TABLE wildlife_seed_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
    "CREATE TABLE wildlife_seed_npcs (npc_row INTEGER PRIMARY KEY, area TEXT NOT NULL, "
    "source TEXT NOT NULL, ff_id INTEGER, template_row INTEGER NOT NULL)",
]


def seeded(db):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'wildlife_seed_npcs'").fetchone()[0] > 0


def npc_seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return None if r is None else r[0]


def rows_digest(db, table="npcs"):
    """An md5 of a table's rows in key order: what --undo gives back."""
    h = hashlib.md5()
    for r in db.execute(f"SELECT * FROM {table} ORDER BY 1"):
        h.update(repr(tuple(r)).encode())
    return h.hexdigest()


def apply(db, p, args):
    if seeded(db):
        sys.exit("this database already holds a wildlife seed (wildlife_seed_npcs): --undo it first")
    cols = [c for c in p["cols"] if c != "id"]
    db.execute("BEGIN IMMEDIATE")
    try:
        for stmt in LOG_DDL:
            db.execute(stmt)
        meta = dict(when=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                    seq_before="" if npc_seq(db) is None else str(npc_seq(db)), seed=str(args.seed),
                    guide=str(bool(args.with_guide_species)), skip=args.skip, rows=str(len(p["rows"])))
        for r in p["rows"]:
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r["row"].get(c) for c in cols])
            db.execute("INSERT INTO wildlife_seed_npcs VALUES (?, ?, ?, ?, ?)",
                       (cur.lastrowid, r["area"], r["source"], r["ff_id"], r["template"]))
        db.executemany("INSERT INTO wildlife_seed_meta VALUES (?, ?)", sorted(meta.items()))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not seeded(db):
        sys.exit("no wildlife seed to undo here")
    meta = dict(db.execute("SELECT key, value FROM wildlife_seed_meta"))
    db.execute("BEGIN IMMEDIATE")
    try:
        n = db.execute("DELETE FROM npcs WHERE id IN (SELECT npc_row FROM wildlife_seed_npcs)").rowcount
        # AUTOINCREMENT's counter back, unless a row was added after the seed
        before = meta.get("seq_before", "")
        top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
        if before == "":
            if top == 0:
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
        elif top <= int(before):
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (int(before),))
        db.execute("DROP TABLE wildlife_seed_npcs")
        db.execute("DROP TABLE wildlife_seed_meta")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-wildlifeseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(p, verbose=False):
    print(f"towns: {p['towns']} townsfolk in towns (entity/towns.go), kept {TOWN_CLEAR:.0f} off")
    by_area = collections.defaultdict(list)
    for gr in p["groups"]:
        by_area[gr["area"]].append(gr)
    for area in AREAS:
        gs = by_area.get(area["name"], [])
        if not gs:
            continue
        total = sum(gr["got"] for gr in gs)
        print(f"{area['name']} (x {area['x'][0]:.0f}-{area['x'][1]:.0f}, z {area['z'][0]:.0f}-{area['z'][1]:.0f}): "
              f"{total} spawns")
        for gr in gs:
            note = "round its camp" if gr["camp"] else "round the sighting"
            short = f", {gr['want'] - gr['got']} found no place" if gr["got"] < gr["want"] else ""
            had = f", {gr['have']} already there" if gr["have"] else ""
            print(f"    {gr['name']!r} L{gr['level']} at ({gr['at'][0]:.0f}, {gr['at'][1]:.0f}) {note}: "
                  f"{gr['got']}{had}{short} [{gr['source']}]")
    if p["left"]:
        print("left out:")
        for area, name, level, why in p["left"]:
            print(f"    {area}: {name!r} L{level}: {why}")
    if verbose:
        for r in p["rows"]:
            row = r["row"]
            print(f"  {r['area']}: {row['npc_name']!r} L{row['npc_level']} at ({row['x_coord']:.1f}, "
                  f"{row['y_coord']:.2f}, {row['z_coord']:.1f}) facing {row['facing']} copy of {r['template']}")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    ap.add_argument("--apply", action="store_true", help="write the spawns")
    ap.add_argument("--undo", action="store_true", help="delete every spawn a seed wrote")
    ap.add_argument("--with-guide-species", action="store_true",
                    help="also the Klick'Anon guides' bats, rats, snakes and fireflies")
    ap.add_argument("--skip", default="", help="kinds to leave out, by name, comma-separated")
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
        print(f"deleted {undo(db)} seeded spawns; restart the server")
        return
    if seeded(db):
        n = db.execute("SELECT count(*) FROM wildlife_seed_npcs").fetchone()[0]
        print(f"a wildlife seed is in already ({n} spawns): --undo it to plan again")
        return
    skip = [k.strip().lower() for k in args.skip.split(",") if k.strip()]
    p = plan(db, args.ground, guide=args.with_guide_species, seed=args.seed, skip=skip)
    report(p, args.verbose)
    if not args.apply:
        print(f"{len(p['rows'])} spawns (report only: --apply writes them)")
        return
    if not args.no_backup:
        print("backup:", backup(db, args.db))
    apply(db, p, args)
    print(f"wrote {len(p['rows'])} spawns; restart the server to read them")


if __name__ == "__main__":
    main()
