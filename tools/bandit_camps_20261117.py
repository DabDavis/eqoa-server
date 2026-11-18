#!/usr/bin/env python3
"""Tunaria's bandit camps manned at their tents and campfires, and Bandit Hills' wildlife thickened (the owner,
2026-11-17, owner-approved spawn work: "bandit camps across tunaria lets fix spawns. in tents 2-3 spawns. around
campfires 2-3 spawns."; then "increase wildlife density in bandit hills by 20%" and "the bandit hills should have wisps
as wildlife").

Two parts, each its own --part, logged, applied and undone apart (both by default):

camps (log bandit_camps_20261117_log)
  The tents and campfires are TUNARIA.ESF's zone actors (tools/bandit_camps_20261117.json, every placement of the
  sprites below, world coordinates), each kind identified by rendering it (vanilla's eqoa shot TUNARIA.ESF zonemodel
  <zone> 0x<id>), never by a name:
    tents  68186309 6818630A (striped pavilions), 6818630B 6818630C (open-sided canopy pavilions), A9C8890C (hide
           tent), C4F746EE (green ridge tent), C4F746EF (green dome tent), 88C727DA EE6E8D6C (hide huts)
    fires  BDB6EBB2 (stone fire ring), 2012199E (spit fire with a pot), 8279266E F731ADF7 (fire pit under a smoke rack)
  A camp is a chain of tents and fires (each within CHAIN of the next) with NPC rows within NEAR of them (|dy| < DY); it
  is a BANDIT camp when one or more of those rows have a bandit's name (BANDIT: bandit, brigand, cutthroat,
  highwayman, highway, outlaw) and they are at least half of the rows that are not proper nouns (a captive, a named
  boss). So the goblin and blackscourge camps with a brigand or two, the Nasehir and Taksehir camps with a bandit among
  warriors and clerics, the orcs' thugs and the poachers are not bandit camps; the goblin bandits' are.
  A tent's footprint and roof are its collision (worlds/ground/0.walls, the placement on the actor's spot): a place is
  IN the tent when the tent's own triangles are over it, its floor is the ground's (worlds/ground/0.ground, or a floor
  of the walls just over it) within FLOOR_DY of the tent's foot, nothing of any wall is within CLEAR of it from the
  ankle to the shoulder (BODY_LO-BODY_HI) and the roof is ROOF_OVER over its floor at least, and it is not water, slime, lava or mud (worlds/ground/0.env). A fire's place is on a ring
  round it (its collision's half-width + RING_OUT), on the same tests (and not inside a tent's wall), facing the fire.
  What is there counts: a bandit-named row standing in a tent, or within the ring + RING_SLACK of a fire, is one of its
  2-3; the tool tops a tent up to WANT_TENT (2) and a fire up to WANT_FIRE (3), where they fit SPACING apart from
  everyone (a small tent may take one: said so), and never takes anyone out (a tent or fire with more keeps them). Each new row is a copy (every column but id, as the GM panel's clone_row) of the camp's nearest
  bandit-named row of a common kind (MIN_KIND rows or more in the world, no proper noun, no rare or spawn-only or
  friendly row, no path), so a camp's levels, bodies, gear, loot, class, faction and template (all by name) are its own;
  its npcrespawn row is copied (the camp's respawn), and it gets an npcwander row of radius -1 so it stays in its tent
  or at its fire, facing as placed.

wildlife (log bandit_hills_wildlife_20261117_log)
  Bandit Hills is CLIENT's zone cell c7 r7 (x 14000-16000, z 14000-16000). Its free wildlife: every row there whose
  kind (name) is a generic creature (seed_wildlife's creature_kinds: no players' race body, not friendly, two rows at
  least, no pet's name), not a bandit-named, proper-noun or "'s" (a quest's beasts) name, not on a path, not a quest
  step's NPC, not a water creature (npcmove.water), and not a camp's: the serpent guardians' cave and the tree
  spirits' grove (their kinds), the bandit camps' (within CAMP_CLEAR of a bandit camp's tents and fires). The tool adds
  round(20%) of that count (wisps' copies counted in), WISP_SHARE (30%) of the adds wisps (none live there: copies of
  the nearest wisp kinds of Tunaria within the zone's wildlife levels, their npctemplate "wisp" by name), the rest its
  kinds in proportion to their rows (largest remainder), each a copy of a row of its kind in the zone (its level, so the
  zone's level mix stays), on its open land: a ground floor with no floor OPEN_SKY over it (no cave, no overhang),
  nothing of the walls within CLEAR, dry, out of towns (TOWN_CLEAR) and the camps (CAMP_CLEAR), each the farthest of
  CANDIDATES random places from every NPC and every add before it (the gaps filled first, so the adds spread, never
  clumped), at least WILD_SPACING from anything. Its respawn row copied; it wanders as its kind does.

Dry run by default: a report per camp (tents found, fires found, bandits there, adds) and of the wildlife (counts
before and after by kind); --map DIR draws each camp and the zone. --apply in one transaction, the database's npcs
sequence and every row written logged per part; a second --apply adds nothing (a camp's adds count as what is there; the
wildlife part is done once). --undo deletes exactly the logged rows (npcs, npcrespawn, npcwander), puts the npcs
sequence back and drops the logs: the npcs, npcrespawn and npcwander tables come back byte for byte (--hash prints
their digests). A row changed since the apply is still taken out, and said so.

NPC rows are read at the server's start: apply with the server stopped (a swap's before.sh), on each shard's database
(ids differ between shards; the tool works by names and places alone). Planning takes minutes (Tunaria's walls), and a
before.sh holds the server 12 s at most: so plan ahead on a fresh copy of the shard's database (--write-plan, never on
the live database: it reads every NPC in one transaction), then --apply --plan in the before.sh: it makes each add
again from its template row as it is then (by id, else the nearest row of its name and body), leaves out one whose
spot an NPC has taken since, and needs no ground files. A part already applied (its log has rows) adds nothing.

    python3 -E -s tools/bandit_camps_20261117.py --db eqoa.db --ground worlds/ground [--part camps|wildlife|both]
        [--map DIR] [--write-plan PLAN.json] [--apply | --undo | --hash]
    python3 -E -s tools/bandit_camps_20261117.py --db eqoa.db --plan PLAN.json [--part ...] [--apply]
"""
import argparse
import collections
import hashlib
import json
import math
import os
import random
import re
import sqlite3
import struct
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_wildlife as sw  # noqa: E402
import seed_town_wildlife as stw  # noqa: E402

TOOL = "tools/bandit_camps_20261117.py"
DATA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "bandit_camps_20261117.json")
WORLD = 0
CAMPS_LOG, WILD_LOG = "bandit_camps_20261117_log", "bandit_hills_wildlife_20261117_log"
CAMPS_META, WILD_META = "bandit_camps_20261117_meta", "bandit_hills_wildlife_20261117_meta"
SOURCE_CAMPS = "tools/bandit_camps_20261117.py: camps"
SOURCE_WILD = "tools/bandit_camps_20261117.py: wildlife"

BANDIT = re.compile(r"bandit|brigand|cutthroat|highwayman|highway|outlaw", re.I)
CHAIN = 60.0          # tents and fires this near one another are one camp
NEAR = 75.0           # a camp's rows: this near one of its tents or fires
DY = 25.0
MIN_KIND = 4          # a kind copied has this many rows in the world at least
WANT_TENT = 2         # "in tents 2-3 spawns": two in a tent (three already there are left), three round a fire
WANT_FIRE = 3         # "around campfires 2-3 spawns"
CLEAR = 1.6           # no wall this near a body (ankle to over the head)
BODY_LO, BODY_HI = 0.5, 3.0  # ankle to shoulder: a tent's roof slopes down over the head
SPACING = 2.6         # bodies this far apart at least
FLOOR_DY = 3.0        # a tent's floor within this of its foot
ROOF_OVER = 3.5       # head room: the tent's own triangles this high over the floor at least
RING_OUT = 3.0        # a fire's ring: its collision's half-width + this
RING_SLACK = 3.0      # a bandit this far past the ring is still at the fire
GRID = 0.75           # a tent's floor sampled this fine
TENT_ROOM = 4.0       # in a tent, bodies this far apart are as good as farther: then the nearer its middle the better

ZONE = dict(name="Bandit Hills", x=(14000.0, 16000.0), z=(14000.0, 16000.0))
GROWTH = 0.20
WISP_SHARE = 0.30
OPEN_SKY = 8.0
TOWN_CLEAR = 120.0
CAMP_CLEAR = 80.0
WILD_SPACING = 25.0
CANDIDATES = 400
EDGE = 15.0
SEED = 1117
# the zone's camp creatures, not wildlife: the serpent guardians' cave camp and the tree spirits' grove (EQOA-Data's
# map camps in Bandit Hills; handlers/camps.go "serpents", mapcamps.go "treespirits")
ZONE_CAMP_KINDS = {"serpent guardian", "serpent king", "tree spirit"}

TABLES = ("npcs", "npcrespawn", "npcwander")
LOG_DDL = """CREATE TABLE IF NOT EXISTS {log} (
    npc_row      INTEGER PRIMARY KEY,  -- the npcs row written
    place        TEXT NOT NULL,        -- the camp and its tent or fire, or the zone
    kind         TEXT NOT NULL,        -- tent, fire, wildlife, wisp
    template_row INTEGER NOT NULL,     -- the namesake row copied
    respawn      INTEGER NOT NULL,     -- 1: an npcrespawn row was written for it
    wander       INTEGER NOT NULL,     -- 1: an npcwander row was written for it
    row          TEXT NOT NULL         -- the npcs row as written (JSON)
)"""
META_DDL = "CREATE TABLE IF NOT EXISTS {meta} (key TEXT PRIMARY KEY, value TEXT NOT NULL)"


# ---- geometry ------------------------------------------------------------------------

def apply_m(m, v):
    x, y, z = v
    return (m[0] * x + m[1] * y + m[2] * z + m[3], m[4] * x + m[5] * y + m[6] * z + m[7],
            m[8] * x + m[9] * y + m[10] * z + m[11])


class Walls:
    """A world's walls file (internal/ground/walls.go): "EQWL", version 1, the buffer and zone counts; each buffer's
    entry (offset int64, strips and vertices uint32, its bounds 6 float32); each zone's (its bounds, offset int64,
    placements uint32); a buffer's strips a uint16 count and float32 x, y, z each; a placement a buffer index and a
    3x4 matrix by rows taking the buffer's space to the world's."""

    def __init__(self, path):
        self.f = open(path, "rb")
        head = self.f.read(16)
        if head[:4] != b"EQWL":
            raise ValueError(f"{path}: not a walls file")
        ver, nb, nz = struct.unpack_from("<III", head, 4)
        if ver != 1:
            raise ValueError(f"{path}: version {ver}, this tool reads 1")
        raw = self.f.read(nb * 40)
        self.bufs = [struct.unpack_from("<qII6f", raw, 40 * i) for i in range(nb)]
        raw = self.f.read(nz * 36)
        self.zones = [struct.unpack_from("<6fqI", raw, 36 * i) for i in range(nz)]
        self.read, self.placed, self.placed_tris = {}, {}, {}

    def buffer(self, i):
        if i not in self.read:
            off, strips, verts = self.bufs[i][:3]
            self.f.seek(off)
            buf = self.f.read(strips * 2 + verts * 12)
            p, tris = 0, []
            for _ in range(strips):
                (n,) = struct.unpack_from("<H", buf, p)
                p += 2
                vs = [struct.unpack_from("<3f", buf, p + 12 * k) for k in range(n)]
                p += 12 * n
                tris.extend((vs[k], vs[k + 1], vs[k + 2]) for k in range(n - 2))
            self.read[i] = tris
        return self.read[i]

    def zone(self, zi):
        if zi not in self.placed:
            e = self.zones[zi]
            self.f.seek(e[6])
            raw = self.f.read(e[7] * 52)
            out = []
            for k in range(e[7]):
                b = struct.unpack_from("<I", raw, 52 * k)[0]
                m = struct.unpack_from("<12f", raw, 52 * k + 4)
                lo, hi = self.bufs[b][3:6], self.bufs[b][6:9]
                cs = [apply_m(m, (x, y, z)) for x in (lo[0], hi[0]) for y in (lo[1], hi[1]) for z in (lo[2], hi[2])]
                out.append((zi, k, b, m, tuple(min(c[i] for c in cs) for i in range(3)),
                            tuple(max(c[i] for c in cs) for i in range(3))))
            self.placed[zi] = out
        return self.placed[zi]

    def near(self, x, z, r):
        """The placements whose bounds come within r of (x, z)."""
        for zi, e in enumerate(self.zones):
            if e[7] == 0 or x + r < e[0] or x - r > e[3] or z + r < e[2] or z - r > e[5]:
                continue
            for p in self.zone(zi):
                lo, hi = p[4], p[5]
                if x + r >= lo[0] and x - r <= hi[0] and z + r >= lo[2] and z - r <= hi[2]:
                    yield p

    def tris(self, p):
        key = (p[0], p[1])
        if key not in self.placed_tris:
            m = p[3]
            self.placed_tris[key] = [tuple(apply_m(m, v) for v in t) for t in self.buffer(p[2])]
        return self.placed_tris[key]


def clip_y(poly, y, keep_above):
    out = []
    n = len(poly)
    for i in range(n):
        a, b = poly[i], poly[(i + 1) % n]
        ina = a[1] >= y if keep_above else a[1] <= y
        inb = b[1] >= y if keep_above else b[1] <= y
        if ina:
            out.append(a)
        if ina != inb:
            t = (y - a[1]) / (b[1] - a[1])
            out.append((a[0] + t * (b[0] - a[0]), y, a[2] + t * (b[2] - a[2])))
    return out


def seg_dist(px, pz, a, b):
    ax, az, bx, bz = a[0], a[2], b[0], b[2]
    dx, dz = bx - ax, bz - az
    l2 = dx * dx + dz * dz
    t = 0.0 if l2 == 0 else max(0.0, min(1.0, ((px - ax) * dx + (pz - az) * dz) / l2))
    return math.hypot(px - ax - t * dx, pz - az - t * dz)


def inside2d(px, pz, poly):
    s = 0
    n = len(poly)
    for i in range(n):
        a, b = poly[i], poly[(i + 1) % n]
        c = (b[0] - a[0]) * (pz - a[2]) - (b[2] - a[2]) * (px - a[0])
        if c != 0:
            if s == 0:
                s = 1 if c > 0 else -1
            elif (c > 0) != (s > 0):
                return False
    return True


def tri_band_dist(px, pz, t, lo, hi):
    """How far (px, pz) lies across the ground from the part of triangle t between heights lo and hi (inf: none)."""
    if max(v[1] for v in t) < lo or min(v[1] for v in t) > hi:
        return math.inf
    poly = clip_y(clip_y(list(t), lo, True), hi, False)
    if len(poly) < 2:
        return math.inf
    if len(poly) >= 3 and inside2d(px, pz, poly):
        return 0.0
    return min(seg_dist(px, pz, poly[i], poly[(i + 1) % len(poly)]) for i in range(len(poly)))


def tri_y(t, x, z):
    return sw.height_at(t, x, z)


class Place:
    """The world's ground, walls and environment, asked about one spot at a time."""

    def __init__(self, ground_dir, world=WORLD):
        self.g = sw.Ground(os.path.join(ground_dir, f"{world}.ground"))
        self.w = Walls(os.path.join(ground_dir, f"{world}.walls"))
        self.env = stw.Env(os.path.join(ground_dir, f"{world}.env"))

    def floor(self, x, z, y, below, above):
        """The floor a body stands on near y: the ground's, or a floor of the walls (a face looking up) within
        `above` over it; None where none."""
        best = self.g.floor_near(x, z, y, below, above)
        for p in self.w.near(x, z, 0.5):
            for t in self.w.tris(p):
                if not sw.floor(t):
                    continue
                h = tri_y(t, x, z)
                if h is not None and y - below <= h <= y + above and (best is None or h > best) and \
                        (best is None or h - best <= 1.5):
                    best = h
        return best

    def clear(self, x, y, z, r=CLEAR):
        """Nothing of any wall within r of a body standing at (x, y, z), from the ankle to over the head."""
        lo, hi = y + BODY_LO, y + BODY_HI
        for p in self.w.near(x, z, r):
            if p[5][1] < lo or p[4][1] > hi:
                continue
            for t in self.w.tris(p):
                if tri_band_dist(x, z, t, lo, hi) < r:
                    return False
        return True

    def dry(self, x, y, z):
        f = self.env.flags(x, y + 1.0, z)
        return f is not None and not (f & stw.BAD_ENV)

    def open_sky(self, x, y, z):
        """No floor of the ground OPEN_SKY or more over (x, y, z): not in a cave, under no overhang."""
        return not any(h > y + OPEN_SKY for h in self.g.floors(x, z))


# ---- reading the database --------------------------------------------------------------

def proper(name):
    words = [w for w in re.split(r"\s+", name.strip()) if w]
    small = {"of", "the", "a", "an"}
    big = [w for w in words if w.lower() not in small]
    return bool(big) and all(w[:1].isupper() for w in big)


def table_exists(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def load(db):
    db.row_factory = sqlite3.Row
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    npcs = []
    for r in db.execute("SELECT * FROM npcs WHERE world = ? ORDER BY id", (WORLD,)):
        d = dict(r)
        npcs.append(dict(row=d, id=d["id"], name=(d["npc_name"] or "").strip(), key=(d["npc_name"] or "").strip().lower(),
                         x=d["x_coord"] or 0.0, y=d["y_coord"] or 0.0, z=d["z_coord"] or 0.0,
                         model=(d["modelid"] or 0) & 0xFFFFFFFF, level=d["npc_level"] or 1, type=d["npc_type"] or 0))
    db.row_factory = None

    def ids(sql):
        try:
            return {r[0] for r in db.execute(sql)}
        except sqlite3.OperationalError:
            return set()

    def names(sql):
        try:
            return {(r[0] or "").strip().lower() for r in db.execute(sql)}
        except sqlite3.OperationalError:
            return set()

    info = dict(
        cols=cols, npcs=npcs,
        pc={r[0] & 0xFFFFFFFF for r in db.execute("SELECT modelid FROM charactermodel")},
        spells=names("SELECT spellname FROM spellpattern"),
        paths=ids("SELECT npcid FROM npcpaths"),
        quest=ids("SELECT npcid FROM queststeps"),
        rare=ids("SELECT npc_row FROM npcrare"),
        spawnonly=names("SELECT name FROM npcspawnonly"),
        water=names("SELECT name FROM npcmove WHERE water != ''"),
        respawn={r[0]: r for r in db.execute("SELECT npc_row, seconds, note, source FROM npcrespawn")},
        count=collections.Counter(n["key"] for n in npcs),
    )
    return info


def new_row(t, x, y, z, facing):
    row = dict(t["row"])
    row.pop("id", None)
    row.update(x_coord=round(x, 3), y_coord=round(y, 4), z_coord=round(z, 3), facing=facing % 256, dt_kill=None)
    return row


def facing_to(x, z, tx, tz):
    """The facing byte of a look from (x, z) toward (tx, tz): CLIENT's heading atan2(dx, dz) (entity/npcmove.go
    heading), as entity.FacingByte rounds it."""
    h = math.atan2(tx - x, tz - z)
    if h < 0:
        h += 2 * math.pi
    return int(round(h / (2 * math.pi) * 256)) % 256


class Grid:
    def __init__(self, cell=16.0):
        self.cell, self.cells = cell, collections.defaultdict(list)

    def add(self, x, y, z, item=None):
        self.cells[(int(x // self.cell), int(z // self.cell))].append((x, y, z, item))

    def near(self, x, z, r):
        n = int(math.ceil(r / self.cell))
        cx, cz = int(x // self.cell), int(z // self.cell)
        for dx in range(-n, n + 1):
            for dz in range(-n, n + 1):
                for p in self.cells.get((cx + dx, cz + dz), ()):
                    if math.hypot(p[0] - x, p[2] - z) <= r:
                        yield p


# ---- part 1: the camps ------------------------------------------------------------------

def structures():
    with open(DATA) as f:
        d = json.load(f)
    out = []
    for kind, sprite, x, y, z, yaw, scale, ez in d["structures"]:
        out.append(dict(kind=kind, sprite=sprite, x=x, y=y, z=z, yaw=yaw, scale=scale,
                        what=(d["tents"] if kind == "tent" else d["fires"])[sprite]))
    return out, d["zones"]


def zone_name(zones, x, z):
    return zones.get(f"c{int(x // 2000)}r{int(z // 2000)}", f"cell c{int(x // 2000)} r{int(z // 2000)}")


def find_camps(info, sts, zones):
    """Chains of tents and fires; each with its rows and whether it is a bandit camp."""
    parent = list(range(len(sts)))

    def find(i):
        while parent[i] != i:
            parent[i] = parent[parent[i]]
            i = parent[i]
        return i

    for i, a in enumerate(sts):
        for j in range(i + 1, len(sts)):
            b = sts[j]
            if abs(b["x"] - a["x"]) > CHAIN:
                break
            if math.hypot(a["x"] - b["x"], a["z"] - b["z"]) <= CHAIN and abs(a["y"] - b["y"]) < DY:
                ra, rb = find(i), find(j)
                if ra != rb:
                    parent[ra] = rb
    groups = collections.defaultdict(list)
    for i, s in enumerate(sts):
        groups[find(i)].append(s)
    grid = Grid(64.0)
    for n in info["npcs"]:
        grid.add(n["x"], n["y"], n["z"], n)
    camps = []
    for members in groups.values():
        rows = {}
        for s in members:
            for _, _, _, n in grid.near(s["x"], s["z"], NEAR):
                if abs(n["y"] - s["y"]) < DY:
                    rows[n["id"]] = n
        rows = sorted(rows.values(), key=lambda n: n["id"])
        bandits = [n for n in rows if BANDIT.search(n["name"])]
        common = [n for n in rows if not proper(n["name"])]
        cx = sum(s["x"] for s in members) / len(members)
        cz = sum(s["z"] for s in members) / len(members)
        is_bandit = len(bandits) >= 1 and 2 * len(bandits) >= len(common)
        kinds = collections.Counter(n["name"] for n in bandits)
        top = kinds.most_common(1)[0][0] if kinds else ""
        camps.append(dict(structures=sorted(members, key=lambda s: (s["kind"] != "tent", s["x"], s["z"])),
                          rows=rows, bandits=bandits, bandit=is_bandit, x=cx, z=cz,
                          y=sum(s["y"] for s in members) / len(members),
                          zone=zone_name(zones, cx, cz), kinds=kinds,
                          name=f"{top or 'camp'} camp, {zone_name(zones, cx, cz)} ({cx:.0f}, {cz:.0f})"))
    camps.sort(key=lambda c: (c["x"], c["z"]))
    return camps


def copyable(info, n):
    return (BANDIT.search(n["name"]) and not proper(n["name"]) and info["count"][n["key"]] >= MIN_KIND
            and n["id"] not in info["rare"] and n["key"] not in info["spawnonly"] and not (n["type"] & 0x80)
            and n["id"] not in info["paths"] and n["model"])


def placement_of(place, s):
    """The walls placement of a tent or fire actor: a placement at its spot (the matrix's translation)."""
    best = None
    for p in place.w.near(s["x"], s["z"], 1.0):
        m = p[3]
        d = math.hypot(m[3] - s["x"], m[11] - s["z"])
        if d < 1.5 and abs(m[7] - s["y"]) < 2.0 and (best is None or d < best[0]):
            best = (d, p)
    return None if best is None else best[1]


def under_roof(place, p, x, z, floor_y):
    for t in place.w.tris(p):
        h = tri_y(t, x, z)
        if h is not None and h >= floor_y + ROOF_OVER:
            return True
    return False


def plan_camps(info, place):
    sts, zones = structures()
    camps = find_camps(info, sts, zones)
    taken = Grid(16.0)
    for n in info["npcs"]:
        taken.add(n["x"], n["y"], n["z"], n["id"])
    tent_walls = []  # every bandit camp tent's placement: a fire's ring must not cut into one
    out = []
    for c in camps:
        if not c["bandit"]:
            continue
        c["tents"], c["fires"] = [], []
        for s in c["structures"]:
            s["placement"] = placement_of(place, s)
            if s["kind"] == "tent" and s["placement"] is not None:
                tent_walls.append(s["placement"])
        templates = [n for n in c["bandits"] if copyable(info, n)]
        c["templates"] = templates
        c["adds"] = []
        counted = set()
        # tents first (a bandit in a tent is the tent's), then the fires
        for s in sorted(c["structures"], key=lambda s: s["kind"] != "tent"):
            entry = dict(s=s, have=[], adds=[], why="")
            (c["tents"] if s["kind"] == "tent" else c["fires"]).append(entry)
            p = s["placement"]
            if p is None:
                entry["why"] = "no collision placed at the actor"
                continue
            lo, hi = p[4], p[5]
            if s["kind"] == "tent":
                for n in c["bandits"]:
                    if n["id"] in counted:
                        continue
                    if lo[0] <= n["x"] <= hi[0] and lo[2] <= n["z"] <= hi[2] and abs(n["y"] - s["y"]) < FLOOR_DY + 2 \
                            and under_roof(place, p, n["x"], n["z"], s["y"]):
                        entry["have"].append(n)
                        counted.add(n["id"])
                spots = tent_spots(place, s, p)
                cx, cz = (lo[0] + hi[0]) / 2, (lo[2] + hi[2]) / 2
                look = lambda x, z, cx=cx, cz=cz, s=s: (facing_to(x, z, cx, cz) if math.hypot(x - cx, z - cz) > 1.5
                                                       else int(round(s["yaw"] / (2 * math.pi) * 256)) % 256)
            else:
                half = max(hi[0] - lo[0], hi[2] - lo[2]) / 2
                ring = half * 1.0 + RING_OUT
                for n in c["bandits"]:
                    if n["id"] in counted:
                        continue
                    if math.hypot(n["x"] - s["x"], n["z"] - s["z"]) <= ring + RING_SLACK and abs(n["y"] - s["y"]) < 6:
                        entry["have"].append(n)
                        counted.add(n["id"])
                spots = fire_spots(place, s, ring, tent_walls + [q["s"]["placement"] for q in c["tents"]
                                                                 if q["s"]["placement"] is not None])
                look = lambda x, z, s=s: facing_to(x, z, s["x"], s["z"])
                entry["ring"] = ring
            want = max(0, (WANT_TENT if s["kind"] == "tent" else WANT_FIRE) - len(entry["have"]))
            if want and not templates:
                entry["why"] = "no common bandit-named row in the camp to copy"
                continue
            center = ((lo[0] + hi[0]) / 2, (lo[2] + hi[2]) / 2) if s["kind"] == "tent" else None
            chosen = pick_spots(spots, want, taken, center)
            if len(entry["have"]) + len(chosen) < 2 and want:
                entry["why"] = f"room for {len(chosen)} more only"
            for (x, y, z) in chosen:
                t = min(templates, key=lambda n: (math.hypot(n["x"] - x, n["z"] - z), n["id"]))
                row = new_row(t, x, y, z, look(x, z))
                add = dict(row=row, template=t, kind=s["kind"],
                           place=f"{c['name']}: {s['kind']} {s['sprite']} at ({s['x']:.0f}, {s['y']:.0f}, {s['z']:.0f})")
                entry["adds"].append(add)
                c["adds"].append(add)
                taken.add(x, y, z, None)
        out.append(c)
    return dict(camps=camps, bandit_camps=out, rows=[a for c in out for a in c["adds"]])


def tent_spots(place, s, p):
    lo, hi = p[4], p[5]
    spots = []
    nx, nz = int((hi[0] - lo[0]) / GRID), int((hi[2] - lo[2]) / GRID)
    for i in range(1, nx):
        for j in range(1, nz):
            x, z = lo[0] + i * GRID, lo[2] + j * GRID
            y = place.floor(x, z, s["y"], FLOOR_DY, FLOOR_DY)
            if y is None or not under_roof(place, p, x, z, y):
                continue
            if not place.clear(x, y, z) or not place.dry(x, y, z):
                continue
            spots.append((x, y, z))
    return spots


def fire_spots(place, s, ring, tents):
    spots = []
    for k in range(36):
        a = 2 * math.pi * k / 36
        for r in (ring, ring + 1.0, ring + 2.0):
            x, z = s["x"] + r * math.sin(a), s["z"] + r * math.cos(a)
            y = place.floor(x, z, s["y"], FLOOR_DY, FLOOR_DY)
            if y is None or not place.clear(x, y, z) or not place.dry(x, y, z):
                continue
            # not under a tent's roof: that is the tent's, not the fire's
            if any(q[4][0] <= x <= q[5][0] and q[4][2] <= z <= q[5][2] and under_roof(place, q, x, z, y) for q in tents):
                continue
            spots.append((x, y, z))
            break
    return spots


def pick_spots(spots, want, taken, center=None):
    """Up to `want` spots SPACING apart from every body near and every spot picked. Round a fire (center None) each is
    the spot farthest from the rest, so they spread round it; in a tent, the spot nearest the tent's middle among
    those TENT_ROOM or more from the rest (a tent's edge is under its eaves, its middle where people sit)."""
    chosen = []
    for _ in range(want):
        best, bk = None, None
        for (x, y, z) in spots:
            d = min([math.hypot(px - x, pz - z) for px, py, pz, _ in taken.near(x, z, 12.0) if abs(py - y) < 8] +
                    [math.hypot(cx - x, cz - z) for cx, _, cz in chosen] + [99.0])
            if d < SPACING:
                continue
            if center is None:
                k = (-d, x, z)
            else:
                k = (-min(d, TENT_ROOM), math.hypot(x - center[0], z - center[1]), x, z)
            if bk is None or k < bk:
                best, bk = (x, y, z), k
        if best is None:
            break
        chosen.append(best)
    return chosen


# ---- part 2: Bandit Hills' wildlife --------------------------------------------------------

def in_zone(n):
    return ZONE["x"][0] <= n["x"] < ZONE["x"][1] and ZONE["z"][0] <= n["z"] < ZONE["z"][1]


def plan_wildlife(info, place, camps_plan, ours=()):
    """ours: the npcs rows a wildlife apply wrote (counted as wildlife, never copied)."""
    npcs = info["npcs"]
    full = [dict(n, world=WORLD) for n in npcs]
    kinds, why = sw.creature_kinds(full, info["pc"], info["spells"])
    camp_pts = [(s["x"], s["z"]) for c in camps_plan["bandit_camps"] for s in c["structures"]]

    def wild(n):
        k = n["key"]
        if k not in kinds or BANDIT.search(k) or proper(n["name"]) or "'s" in k or k in ZONE_CAMP_KINDS:
            return False
        if n["id"] in info["paths"] or n["id"] in info["quest"] or k in info["water"]:
            return False
        if any(math.hypot(n["x"] - x, n["z"] - z) <= CAMP_CLEAR for x, z in camp_pts):
            return False
        return True

    ours = set(ours)
    zone_rows = [n for n in npcs if in_zone(n)]
    wildlife = [n for n in zone_rows if wild(n) or n["id"] in ours]
    base = [n for n in wildlife if n["id"] not in ours]
    total = int(round(GROWTH * len(base)))
    done = len(wildlife) - len(base)
    todo = max(0, total - done)
    res = dict(base=base, wildlife=wildlife, total=total, done=done, rows=[], wisp_kinds=[], left=[])
    by_kind = collections.defaultdict(list)
    for n in base:
        by_kind[n["key"]].append(n)
    levels = sorted(n["level"] for n in base)
    lo_lv, hi_lv = (levels[0], levels[-1]) if levels else (1, 60)
    # wisps: the wisp kinds of Tunaria (a wisp template by name) within the zone's levels, nearest first
    cx, cz = sum(ZONE["x"]) / 2, sum(ZONE["z"]) / 2
    wisp_kinds = []
    for k, rows in kinds.items():
        if "wisp" not in k or proper(rows[0]["name"]) or k not in info["templates_wisp"]:
            continue
        rows = [n for n in rows if lo_lv <= n["level"] <= hi_lv]
        if not rows:
            continue
        d = min(math.hypot(n["x"] - cx, n["z"] - cz) for n in rows)
        wisp_kinds.append((d, k, rows))
    wisp_kinds.sort()
    wisp_kinds = wisp_kinds[:2]
    res["wisp_kinds"] = [(k, d, sorted({n["level"] for n in rows}), len(rows)) for d, k, rows in wisp_kinds]
    have_wisps = sum(1 for n in wildlife if "wisp" in n["key"])
    n_wisp = max(0, int(round(WISP_SHARE * total)) - have_wisps) if wisp_kinds else 0
    n_wisp = min(n_wisp, todo)
    n_rest = todo - n_wisp
    # the rest by the kinds' shares (largest remainder)
    shares = []
    for k, rows in by_kind.items():
        q = n_rest * len(rows) / max(1, len(base))
        shares.append([k, int(q), q - int(q)])
    short = n_rest - sum(s[1] for s in shares)
    for s in sorted(shares, key=lambda s: (-s[2], s[0]))[:short]:
        s[1] += 1
    order = []
    for k, cnt, _ in sorted(shares, key=lambda s: s[0]):
        order += [("wildlife", k)] * cnt
    for i in range(n_wisp):
        order.append(("wisp", wisp_kinds[i % len(wisp_kinds)][1]))
    rng = random.Random(SEED)
    rng.shuffle(order)
    # where: the farthest of CANDIDATES random open-land places from everything
    taken = Grid(64.0)
    for n in npcs:
        taken.add(n["x"], n["y"], n["z"], n["id"])
    towns, _ = sw.town_people(full, info["pc"])
    wisp_rows = {k: rows for _, k, rows in wisp_kinds}
    land = sorted(n["y"] for n in base)
    for kind, key in order:
        best, bd = None, -1.0
        tries = 0
        while tries < CANDIDATES * 6 and (best is None or tries < CANDIDATES):
            tries += 1
            x = rng.uniform(ZONE["x"][0] + EDGE, ZONE["x"][1] - EDGE)
            z = rng.uniform(ZONE["z"][0] + EDGE, ZONE["z"][1] - EDGE)
            fl = place.g.floors(x, z)
            if not fl:
                continue
            y = fl[0]
            if land and not (land[0] - 15 <= y <= land[-1] + 15):
                continue
            if not place.open_sky(x, y, z) or not place.dry(x, y, z) or not place.clear(x, y, z, 2.5):
                continue
            if any(math.hypot(x - px, z - pz) <= CAMP_CLEAR for px, pz in camp_pts):
                continue
            if next(towns.near(WORLD, x, z, TOWN_CLEAR), None) is not None:
                continue
            near = [math.hypot(px - x, pz - z) for px, py, pz, _ in taken.near(x, z, 400.0)]
            d = min(near) if near else 400.0
            if d < WILD_SPACING:
                continue
            if d > bd:
                best, bd = (x, y, z), d
        if best is None:
            res["left"].append((kind, key, "no open place left"))
            continue
        x, y, z = best
        pool = wisp_rows[key] if kind == "wisp" else by_kind[key]
        t = rng.choice(sorted(pool, key=lambda n: n["id"]))
        row = new_row(t, x, y, z, rng.randrange(256))
        res["rows"].append(dict(row=row, template=t, kind=kind, place=f"{ZONE['name']}: open land", gap=bd))
        taken.add(x, y, z, None)
    return res


# ---- writing, and taking it back out ---------------------------------------------------------

def digest(db, table):
    h = hashlib.md5()
    if table_exists(db, table):
        for r in db.execute(f"SELECT * FROM {table} ORDER BY 1"):
            h.update(repr(tuple(r)).encode())
    return h.hexdigest()


def npc_seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return None if r is None else r[0]


def logged(db, log):
    if not table_exists(db, log):
        return []
    return [r[0] for r in db.execute(f"SELECT npc_row FROM {log} ORDER BY npc_row")]


def write(db, rows, log, meta, cols, info, wander):
    db.execute(LOG_DDL.format(log=log))
    db.execute(META_DDL.format(meta=meta))
    if db.execute(f"SELECT count(*) FROM {meta} WHERE key = 'seq_before'").fetchone()[0] == 0:
        before = npc_seq(db)
        db.execute(f"INSERT INTO {meta} VALUES ('seq_before', ?)", (json.dumps(before),))
    has_wander = table_exists(db, "npcwander")
    for a in rows:
        row = {k: v for k, v in a["row"].items() if k in cols}
        names = list(row)
        cur = db.execute(f"INSERT INTO npcs ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})",
                         [row[k] for k in names])
        rid = cur.lastrowid
        a["id"] = rid
        resp = info["respawn"].get(a["template"]["id"])
        if resp is not None:
            db.execute("INSERT INTO npcrespawn (npc_row, seconds, note, source) VALUES (?, ?, ?, ?)",
                       (rid, resp[1], f"as row {a['template']['id']}'s (its camp's); {TOOL}", resp[3] or "tool"))
        w = 0
        if wander and has_wander:
            db.execute("INSERT INTO npcwander (npcid, radius, note, source) VALUES (?, -1, ?, ?)",
                       (rid, "stays in its tent or at its fire, facing as placed", TOOL))
            w = 1
        db.execute(f"INSERT INTO {log} VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (rid, a["place"], a["kind"], a["template"]["id"], 1 if resp is not None else 0, w,
                    json.dumps(dict(row, id=rid), sort_keys=True, default=str)))


def undo(db, log, meta):
    if not table_exists(db, log):
        return 0, []
    changed = []
    rows = list(db.execute(f"SELECT npc_row, respawn, wander, row FROM {log} ORDER BY npc_row"))
    db.row_factory = sqlite3.Row
    for rid, resp, w, written in rows:
        r = db.execute("SELECT * FROM npcs WHERE id = ?", (rid,)).fetchone()
        if r is not None and json.dumps(dict(r), sort_keys=True, default=str) != written:
            changed.append(rid)
    db.row_factory = None
    for rid, resp, w, _ in rows:
        db.execute("DELETE FROM npcs WHERE id = ?", (rid,))
        if resp:
            db.execute("DELETE FROM npcrespawn WHERE npc_row = ?", (rid,))
        if w:
            db.execute("DELETE FROM npcwander WHERE npcid = ?", (rid,))
    seq = json.loads(db.execute(f"SELECT value FROM {meta} WHERE key = 'seq_before'").fetchone()[0])
    # the sequence back as it was, unless something else has been written past our rows since
    top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
    if seq is None:
        if top == 0:
            db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
    elif top <= seq:
        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (seq,))
    db.execute(f"DROP TABLE {log}")
    db.execute(f"DROP TABLE {meta}")
    return len(rows), changed


# ---- reporting ------------------------------------------------------------------------------

def report_camps(p, verbose=False):
    bc = p["bandit_camps"]
    print(f"== camps: {len(bc)} bandit camps of {len(p['camps'])} tent-and-fire camps in Tunaria; "
          f"{len(p['rows'])} adds")
    for c in bc:
        nt, nf = len(c["tents"]), len(c["fires"])
        print(f"-- {c['name']}: {nt} tents, {nf} fires; bandit-named rows there {len(c['bandits'])} "
              f"({', '.join(f'{k} x{v}' for k, v in c['kinds'].most_common())}); adds {len(c['adds'])}")
        for e in c["tents"] + c["fires"]:
            s = e["s"]
            got = ", ".join(f"{a['row']['npc_name']} L{a['row']['npc_level']}" for a in e["adds"]) or "-"
            extra = f" ({e['why']})" if e["why"] else ""
            print(f"     {s['kind']:4} {s['sprite']} {s['what']:<32} ({s['x']:.0f}, {s['y']:.0f}, {s['z']:.0f}) "
                  f"has {len(e['have'])}, adds {len(e['adds'])}: {got}{extra}")
    if verbose:
        others = [c for c in p["camps"] if not c["bandit"] and c["bandits"]]
        print(f"   not bandit camps (bandit-named rows a minority or one): " +
              "; ".join(f"{c['name']} ({len(c['bandits'])}/{len(c['rows'])})" for c in others))


def report_wild(w):
    before = collections.Counter(n["name"] for n in w["wildlife"])
    adds = collections.Counter(a["row"]["npc_name"] for a in w["rows"])
    print(f"== wildlife, {ZONE['name']}: {len(w['base'])} free wildlife rows; +{int(GROWTH * 100)}% = {w['total']} "
          f"(written before: {w['done']}); adds now {len(w['rows'])}")
    for k, d, lv, n in w["wisp_kinds"]:
        print(f"   wisps from: {k} (levels {lv}, {n} rows in those levels, nearest {d:.0f} off the zone's middle)")
    names = sorted(set(before) | set(adds), key=lambda k: (-(before[k] + adds[k]), k))
    for k in names:
        lv = sorted({a["row"]["npc_level"] for a in w["rows"] if a["row"]["npc_name"] == k})
        print(f"   {k:<22} {before[k]:>4} -> {before[k] + adds[k]:>4}" + (f"  (+{adds[k]}, levels {lv})" if adds[k] else ""))
    if w["rows"]:
        gaps = sorted(a["gap"] for a in w["rows"])
        print(f"   each add's nearest NPC: {gaps[0]:.0f} to {gaps[-1]:.0f} (median {gaps[len(gaps) // 2]:.0f})")
    for kind, key, why in w["left"]:
        print(f"   left: {kind} {key}: {why}")


def draw(p, w, place, out):
    try:
        from PIL import Image, ImageDraw
    except ImportError:
        print("no PIL: no maps")
        return
    os.makedirs(out, exist_ok=True)
    for i, c in enumerate(p["bandit_camps"]):
        xs = [s["x"] for s in c["structures"]] + [a["row"]["x_coord"] for a in c["adds"]]
        zs = [s["z"] for s in c["structures"]] + [a["row"]["z_coord"] for a in c["adds"]]
        x0, x1, z0, z1 = min(xs) - 25, max(xs) + 25, min(zs) - 25, max(zs) + 25
        k = 900 / max(x1 - x0, z1 - z0)
        im = Image.new("RGB", (int((x1 - x0) * k) + 1, int((z1 - z0) * k) + 1), (30, 34, 44))
        d = ImageDraw.Draw(im)
        P = lambda x, z: ((x - x0) * k, (z - z0) * k)  # +z is south: down the page, north up
        seen = set()
        for s in c["structures"]:
            for q in place.w.near(s["x"], s["z"], 25):
                if (q[0], q[1]) in seen:
                    continue
                seen.add((q[0], q[1]))
                mine = [t["placement"][:2] for t in c["structures"] if t.get("placement") is not None]
                col = (200, 170, 90) if q[:2] in mine else (90, 90, 100)
                for t in place.w.tris(q):
                    d.polygon([P(v[0], v[2]) for v in t], outline=col)
        for s in c["structures"]:
            x, z = P(s["x"], s["z"])
            col = (255, 120, 40) if s["kind"] == "fire" else (230, 210, 120)
            d.ellipse((x - 5, z - 5, x + 5, z + 5), outline=col, width=2)
        for n in c["rows"]:
            x, z = P(n["x"], n["z"])
            col = (220, 60, 60) if BANDIT.search(n["name"]) else (150, 150, 150)
            d.ellipse((x - 4, z - 4, x + 4, z + 4), fill=col)
        for a in c["adds"]:
            r = a["row"]
            x, z = P(r["x_coord"], r["z_coord"])
            d.ellipse((x - 4, z - 4, x + 4, z + 4), fill=(60, 220, 90))
            h = r["facing"] / 256 * 2 * math.pi
            d.line((x, z, x + 12 * math.sin(h), z + 12 * math.cos(h)), fill=(60, 220, 90), width=2)
        d.text((6, 6), c["name"] + "  red: bandits there, green: adds (facing), orange ring: fire, yellow: tent",
               fill=(255, 255, 255))
        im.save(os.path.join(out, f"camp{i:02d}.png"))
    if w is not None:
        x0, z0, S = ZONE["x"][0], ZONE["z"][0], 2000.0
        k = 900 / S
        im = Image.new("RGB", (900, 900), (30, 34, 44))
        d = ImageDraw.Draw(im)
        P = lambda x, z: ((x - x0) * k, (z - z0) * k)
        for n in w["wildlife"]:
            x, z = P(n["x"], n["z"])
            d.ellipse((x - 3, z - 3, x + 3, z + 3), fill=(150, 150, 150))
        for a in w["rows"]:
            x, z = P(a["row"]["x_coord"], a["row"]["z_coord"])
            col = (120, 200, 255) if a["kind"] == "wisp" else (60, 220, 90)
            d.ellipse((x - 4, z - 4, x + 4, z + 4), fill=col)
        for c in p["bandit_camps"]:
            for s in c["structures"]:
                if in_zone(s):
                    x, z = P(s["x"], s["z"])
                    d.rectangle((x - 2, z - 2, x + 2, z + 2), outline=(255, 120, 40))
        d.text((6, 6), "Bandit Hills (north up): grey wildlife, green adds, blue wisps, orange the bandit camps",
               fill=(255, 255, 255))
        im.save(os.path.join(out, "bandit_hills_wildlife.png"))


# ---- a plan made ahead, applied fast ------------------------------------------------------------

def plan_doc(cp, wp, db_path):
    """The adds as a plan file: each its template row (id, name, body) and where it stands."""
    def entry(a):
        r, t = a["row"], a["template"]
        return dict(template=t["id"], name=t["name"], model=t["model"], kind=a["kind"], place=a["place"],
                    x=r["x_coord"], y=r["y_coord"], z=r["z_coord"], facing=r["facing"])
    parts = {"camps": [entry(a) for a in cp["rows"]]}
    if wp is not None:
        parts["wildlife"] = [entry(a) for a in wp["rows"]]
    return dict(tool=TOOL, made_from=os.path.abspath(db_path), parts=parts)


def load_lean(db, doc):
    """What a plan's apply needs, quickly: every world row's id, name, body and place; the full rows of the plan's
    names alone; the respawn rows."""
    names = sorted({e["name"].strip().lower() for p in doc["parts"].values() for e in p})
    npcs = []
    for i, name, x, y, z, model in db.execute("SELECT id, npc_name, x_coord, y_coord, z_coord, modelid FROM npcs "
                                              "WHERE world = ?", (WORLD,)):
        npcs.append(dict(id=i, name=(name or "").strip(), key=(name or "").strip().lower(), x=x or 0.0, y=y or 0.0,
                         z=z or 0.0, model=(model or 0) & 0xFFFFFFFF))
    full = {}
    db.row_factory = sqlite3.Row
    for k in range(0, len(names), 200):
        chunk = names[k:k + 200]
        q = ("SELECT * FROM npcs WHERE world = ? AND lower(trim(npc_name)) IN (%s)" % ",".join("?" * len(chunk)))
        for r in db.execute(q, [WORLD] + chunk):
            full[r["id"]] = dict(r)
    db.row_factory = None
    for n in npcs:
        if n["id"] in full:
            n["row"] = full[n["id"]]
    return dict(cols=[r[1] for r in db.execute("PRAGMA table_info(npcs)")], npcs=npcs,
                respawn={r[0]: r for r in db.execute("SELECT npc_row, seconds, note, source FROM npcrespawn")})


def rows_from_plan(db, info, doc, part):
    """The planned adds of a part made rows again from their templates as they are now: a template gone, renamed or
    given another body is replaced by the nearest row of its name and body in the world (else the add is left out);
    an add whose spot an NPC has taken since (within 1.5) is left out. Returns (rows, left)."""
    by_id = {n["id"]: n for n in info["npcs"]}
    by_name = collections.defaultdict(list)
    for n in info["npcs"]:
        if "row" in n:
            by_name[(n["key"], n["model"])].append(n)
    taken = Grid(16.0)
    for n in info["npcs"]:
        taken.add(n["x"], n["y"], n["z"], n["id"])
    rows, left = [], []
    for e in doc["parts"].get(part, []):
        t = by_id.get(e["template"])
        if t is None or t["key"] != e["name"].strip().lower() or t["model"] != e["model"]:
            pool = by_name.get((e["name"].strip().lower(), e["model"]), [])
            t = min(pool, key=lambda n: (math.hypot(n["x"] - e["x"], n["z"] - e["z"]), n["id"])) if pool else None
        if t is None:
            left.append((e["place"], e["name"], "no row of its name and body left to copy"))
            continue
        if any(abs(py - e["y"]) < 8 for _, py, _, _ in taken.near(e["x"], e["z"], 1.5)):
            left.append((e["place"], e["name"], "its spot taken since the plan"))
            continue
        rows.append(dict(row=new_row(t, e["x"], e["y"], e["z"], e["facing"]), template=t, kind=e["kind"],
                         place=e["place"]))
        taken.add(e["x"], e["y"], e["z"], None)
    return rows, left


# ---- main ---------------------------------------------------------------------------------------

def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--ground", default="worlds/ground", help="the ground files' folder (0.ground, 0.walls, 0.env)")
    ap.add_argument("--part", choices=("camps", "wildlife", "both"), default="both")
    ap.add_argument("--map", help="draw each camp and the zone into this folder")
    ap.add_argument("--verbose", action="store_true")
    ap.add_argument("--write-plan", help="write the adds planned to this file (for a later --apply --plan)")
    ap.add_argument("--plan", help="apply (or show) the adds of this plan file instead of planning now: seconds, "
                                   "not minutes (a before.sh's hold is 12 s); make it with --write-plan on a fresh "
                                   "copy of this shard's database")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--hash", action="store_true")
    a = ap.parse_args(argv)
    if not os.path.exists(a.db):
        sys.exit(f"no database {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA foreign_keys = OFF")
    parts = ("camps", "wildlife") if a.part == "both" else (a.part,)
    if a.hash:
        for t in TABLES:
            print(t, digest(db, t))
        return 0
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        for part in reversed(parts):
            log, meta = (CAMPS_LOG, CAMPS_META) if part == "camps" else (WILD_LOG, WILD_META)
            n, changed = undo(db, log, meta)
            print(f"undo {part}: {n} rows taken out" + (f"; changed since the apply: {changed}" if changed else ""))
        db.execute("COMMIT")
        for t in TABLES:
            print(t, digest(db, t))
        return 0
    db.execute("BEGIN IMMEDIATE" if a.apply else "BEGIN")
    if a.plan:
        with open(a.plan) as f:
            doc = json.load(f)
        if doc.get("tool") != TOOL:
            sys.exit(f"{a.plan}: not a plan of {TOOL}")
        info = load_lean(db, doc)
        todo = {}
        for part in parts:
            log = CAMPS_LOG if part == "camps" else WILD_LOG
            if logged(db, log):
                print(f"{part}: applied already ({len(logged(db, log))} rows logged): nothing more")
                todo[part] = []
                continue
            rows, left = rows_from_plan(db, info, doc, part)
            todo[part] = rows
            print(f"{part}: {len(rows)} adds from {a.plan} (planned {len(doc['parts'].get(part, []))})")
            for place_, name, why in left:
                print(f"   left: {name} at {place_}: {why}")
            for (k, n), c in collections.Counter((r["kind"], r["row"]["npc_name"]) for r in rows).most_common():
                print(f"   {k:8} {n:<24} +{c}")
        if not a.apply:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)")
            return 0
        for part in parts:
            log, meta = (CAMPS_LOG, CAMPS_META) if part == "camps" else (WILD_LOG, WILD_META)
            write(db, todo[part], log, meta, info["cols"], info, wander=(part == "camps"))
            print(f"applied {part}: {len(todo[part])} rows")
        db.execute("COMMIT")
        return 0
    info = load(db)
    info["templates_wisp"] = {(r[0] or "").strip().lower() for r in
                              db.execute("SELECT name FROM npctemplate WHERE template = 'wisp'")} \
        if table_exists(db, "npctemplate") else set()
    place = Place(a.ground)
    cp = plan_camps(info, place)
    report_camps(cp, a.verbose)
    wp = None
    if "wildlife" in parts:
        wp = plan_wildlife(info, place, cp, logged(db, WILD_LOG))
        if logged(db, WILD_LOG):
            wp["rows"] = []  # the wildlife part is done once
        report_wild(wp)
    if a.map:
        draw(cp, wp, place, a.map)
    if a.write_plan:
        with open(a.write_plan, "w") as f:
            json.dump(plan_doc(cp, wp, a.db), f, indent=1)
        print(f"plan written: {a.write_plan}")
    if not a.apply:
        db.execute("ROLLBACK")
        print("dry run: nothing written (--apply to write)")
        return 0
    if "camps" in parts:
        write(db, cp["rows"], CAMPS_LOG, CAMPS_META, info["cols"], info, wander=True)
        print(f"applied camps: {len(cp['rows'])} rows")
    if wp is not None:
        write(db, wp["rows"], WILD_LOG, WILD_META, info["cols"], info, wander=False)
        print(f"applied wildlife: {len(wp['rows'])} rows")
    db.execute("COMMIT")
    return 0


if __name__ == "__main__":
    sys.exit(main())
