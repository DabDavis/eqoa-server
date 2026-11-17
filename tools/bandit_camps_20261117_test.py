#!/usr/bin/env python3
"""tools/bandit_camps_20261117.py against a made-up Bandit Hills: flat ground, a walls file holding a tent (four walls,
a door, a pyramid roof) and a stone fire ring, a dry environment, and a database with a bandit camp at them, a goblin
camp with one brigand at a second fire, wildlife over the zone and wisps far off. No game data.

    python3 tools/bandit_camps_20261117_test.py
"""
import json
import math
import os
import sqlite3
import struct
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bandit_camps_20261117 as bc  # noqa: E402

LAND = 50.0
TENT = (14500.0, LAND, 14500.0)
FIRE = (14530.0, LAND, 14500.0)
GOBLIN_FIRE = (16300.0, LAND, 13700.0)
BANDIT_BODY, GOBLIN_BODY, WOLF, BOAR, WISP_BODY = 0x1001, 0x1002, 0x2001, 0x2002, 0x3001


def write_ground(path, x0=13500.0, z0=13500.0, size=3000.0, step=50.0):
    n = int(size / step) + 1
    strips = []
    for i in range(n - 1):
        s = []
        for j in range(n):
            for x in (x0 + i * step, x0 + (i + 1) * step):
                s.append((x, LAND, z0 + j * step))
        strips.append(s)
    verts = [v for s in strips for v in s]
    minx, maxx = min(v[0] for v in verts), max(v[0] for v in verts)
    minz, maxz = min(v[2] for v in verts), max(v[2] for v in verts)
    with open(path, "wb") as f:
        f.write(b"EQGR" + struct.pack("<II", 1, 1))
        f.write(struct.pack("<7fqII", minx, minz, maxx, maxz, minx, LAND, minz, 56, len(strips), len(verts)))
        for s in strips:
            f.write(struct.pack("<H", len(s)))
            for x, y, z in s:
                f.write(struct.pack("<hhh", int(round((x - minx) * 4)) - 32768, 0, int(round((z - minz) * 4)) - 32768))


def quad(a, b, h0, h1):
    """A wall from (a) to (b) on the ground, h0 to h1 high: a strip of four."""
    return [(a[0], h0, a[1]), (a[0], h1, a[1]), (b[0], h0, b[1]), (b[0], h1, b[1])]


def tent_buffer():
    strips = []
    corners = [(-8, -8), (8, -8), (8, 8), (-8, 8)]
    for i in range(4):
        a, b = corners[i], corners[(i + 1) % 4]
        if i == 0:  # the north wall, a door in it from -2 to 2
            strips.append(quad(a, (-2, -8), 0, 4))
            strips.append(quad((2, -8), b, 0, 4))
        else:
            strips.append(quad(a, b, 0, 4))
    for i in range(4):
        a, b = corners[i], corners[(i + 1) % 4]
        strips.append([(a[0], 4, a[1]), (b[0], 4, b[1]), (0, 9, 0)])
    return strips


def ring_buffer(r=2.0):
    strips = []
    for k in range(8):
        a, b = 2 * math.pi * k / 8, 2 * math.pi * (k + 1) / 8
        strips.append(quad((r * math.sin(a), r * math.cos(a)), (r * math.sin(b), r * math.cos(b)), 0, 0.6))
    return strips


def write_walls(path, placements):
    """placements: (buffer strips, (x, y, z)), each a buffer of its own, one zone."""
    bufs = [p[0] for p in placements]
    head = 16 + 40 * len(bufs) + 36
    blobs, entries, off = [], [], head
    for strips in bufs:
        vs = [v for s in strips for v in s]
        blob = b"".join(struct.pack("<H", len(s)) + b"".join(struct.pack("<3f", *v) for v in s) for s in strips)
        entries.append(struct.pack("<qII6f", off, len(strips), len(vs), min(v[0] for v in vs), min(v[1] for v in vs),
                                   min(v[2] for v in vs), max(v[0] for v in vs), max(v[1] for v in vs),
                                   max(v[2] for v in vs)))
        blobs.append(blob)
        off += len(blob)
    pl = b""
    for i, (_, (x, y, z)) in enumerate(placements):
        pl += struct.pack("<I12f", i, 1, 0, 0, x, 0, 1, 0, y, 0, 0, 1, z)
    with open(path, "wb") as f:
        f.write(b"EQWL" + struct.pack("<III", 1, len(bufs), 1))
        for e in entries:
            f.write(e)
        f.write(struct.pack("<6fqI", 13000, 0, 13000, 17000, 200, 17000, off, len(placements)))
        for b in blobs:
            f.write(b)
        f.write(pl)


def write_env(path):
    tree = lambda leaves: struct.pack("<II", 0, len(leaves)) + struct.pack(f"<{len(leaves)}I", *leaves)
    with open(path, "wb") as f:
        f.write(b"EQEV" + struct.pack("<I", 1) + tree([0]) + struct.pack("<I", 1) + tree([0]))


SCHEMA = [
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, "
    "y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, modelid INTEGER, "
    "size REAL, race TEXT, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER)",
    "CREATE TABLE charactermodel (modelid INTEGER)",
    "CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT)",
    "CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0, "
    "water TEXT NOT NULL DEFAULT '')",
]


def npc(db, name, x, z, model, level=20, y=LAND):
    return db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, hp, modelid, size, "
                      "race, npc_level, npc_type, npc_id) VALUES (?, 7, ?, ?, ?, 0, 0, 100, ?, 1.0, '0', ?, 0, 0)",
                      (name, x, y, z, model, level)).lastrowid


def build(dirpath):
    ground = os.path.join(dirpath, "ground")
    os.makedirs(ground)
    write_ground(os.path.join(ground, "0.ground"))
    write_walls(os.path.join(ground, "0.walls"), [(tent_buffer(), TENT), (ring_buffer(), FIRE),
                                                  (ring_buffer(), GOBLIN_FIRE)])
    write_env(os.path.join(ground, "0.env"))
    data = os.path.join(dirpath, "camps.json")
    with open(data, "w") as f:
        json.dump(dict(tents={"T1": "test tent"}, fires={"F1": "test fire ring"}, zones={"c7r7": "Bandit Hills"},
                       structures=[["tent", "T1", *TENT, 0.0, 1.0, 0], ["fire", "F1", *FIRE, 0.0, 1.0, 0],
                                   ["fire", "F1", *GOBLIN_FIRE, 0.0, 1.0, 0]]), f)
    path = os.path.join(dirpath, "eqoa.db")
    db = sqlite3.connect(path, isolation_level=None)
    for s in SCHEMA:
        db.execute(s)
    db.execute("INSERT INTO charactermodel VALUES (?)", (BANDIT_BODY,))  # bandits are human: a players' race body
    # the bandit camp: one bandit at the fire, two in the open, a captive; more bandits elsewhere (a common kind)
    at_fire = npc(db, "bandit", FIRE[0] + 5, FIRE[2], BANDIT_BODY, 27)
    db.execute("INSERT INTO npcrespawn VALUES (?, 90, '', 'gm')", (at_fire,))
    npc(db, "bandit", 14480, 14540, BANDIT_BODY, 28)
    npc(db, "bandit", 14520, 14460, BANDIT_BODY, 28)
    npc(db, "Allyn Fairfarm", 14505, 14530, BANDIT_BODY, 29)
    for i in range(4):
        npc(db, "bandit", 5000 + 30 * i, 5000, BANDIT_BODY, 30)
    # the goblin camp: one brigand among five gladiators: not a bandit camp
    for i in range(5):
        npc(db, "goblin gladiator", GOBLIN_FIRE[0] + 6 * math.sin(i), GOBLIN_FIRE[2] + 6 * math.cos(i), GOBLIN_BODY)
    npc(db, "goblin brigand", GOBLIN_FIRE[0] - 20, GOBLIN_FIRE[2], GOBLIN_BODY)
    for i in range(4):
        npc(db, "goblin brigand", 6000 + 30 * i, 6000, GOBLIN_BODY)
    # Bandit Hills' wildlife: 20 wolves and 5 boars over the zone; wisps far off with their template
    for i in range(20):
        npc(db, "mist wolf", 14100 + 90 * i, 15800 - 60 * (i % 5), WOLF, 22 + i % 7)
    for i in range(5):
        npc(db, "wild boar", 15000 + 150 * i, 14200, BOAR, 21)
    for i in range(6):
        npc(db, "willowisp", 17500 + 40 * i, 15300, WISP_BODY, 28)
    db.execute("INSERT INTO npctemplate VALUES ('willowisp', 'wisp', '', 'gm')")
    db.close()
    return path, ground, data


def digests(path):
    db = sqlite3.connect(path)
    out = {t: bc.digest(db, t) for t in bc.TABLES}
    out["seq"] = bc.npc_seq(db)
    db.close()
    return out


class Test(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db, self.ground, data = build(self.tmp.name)
        self.old_data = bc.DATA
        bc.DATA = data

    def tearDown(self):
        bc.DATA = self.old_data
        self.tmp.cleanup()

    def run_tool(self, *extra):
        return bc.main(["--db", self.db, "--ground", self.ground, *extra])

    def plan(self):
        db = sqlite3.connect(self.db)
        info = bc.load(db)
        info["templates_wisp"] = {"willowisp"}
        place = bc.Place(self.ground)
        cp = bc.plan_camps(info, place)
        wp = bc.plan_wildlife(info, place, cp)
        db.close()
        return cp, wp, place

    def test_camps(self):
        cp, _, place = self.plan()
        self.assertEqual(len(cp["bandit_camps"]), 1, "the goblin camp with one brigand is no bandit camp")
        c = cp["bandit_camps"][0]
        tent, fire = c["tents"][0], c["fires"][0]
        self.assertEqual(len(tent["have"]), 0)
        self.assertEqual(len(tent["adds"]), bc.WANT_TENT)
        self.assertEqual(len(fire["have"]), 1, "the bandit at the fire counts")
        self.assertEqual(len(fire["adds"]), bc.WANT_FIRE - 1)
        p = tent["s"]["placement"]
        for a in tent["adds"]:
            r = a["row"]
            self.assertEqual(r["npc_name"], "bandit")
            self.assertLess(abs(r["x_coord"] - TENT[0]), 8 - bc.CLEAR + 1e-6, "inside the tent's walls")
            self.assertLess(abs(r["z_coord"] - TENT[2]), 8 - bc.CLEAR + 1e-6)
            self.assertTrue(bc.under_roof(place, p, r["x_coord"], r["z_coord"], r["y_coord"]))
            self.assertAlmostEqual(r["y_coord"], LAND, places=3, msg="on the floor")
        for a in fire["adds"]:
            r = a["row"]
            d = math.hypot(r["x_coord"] - FIRE[0], r["z_coord"] - FIRE[2])
            self.assertGreaterEqual(d, 2 + bc.RING_OUT - 1e-6)
            self.assertLessEqual(d, 2 + bc.RING_OUT + 2 + 1e-6)
            self.assertEqual(r["facing"], bc.facing_to(r["x_coord"], r["z_coord"], FIRE[0], FIRE[2]), "faces the fire")
            # a level and body the camp's: copied from its nearest bandit
            self.assertIn(r["npc_level"], (27, 28))
        spots = [(a["row"]["x_coord"], a["row"]["z_coord"]) for a in c["adds"]] + [(FIRE[0] + 5, FIRE[2])]
        for i in range(len(spots)):
            for j in range(i + 1, len(spots)):
                self.assertGreaterEqual(math.dist(spots[i], spots[j]), bc.SPACING - 1e-6)

    def test_facing(self):
        self.assertEqual(bc.facing_to(0, 0, 0, 10), 0)        # +z: heading 0
        self.assertEqual(bc.facing_to(0, 0, 10, 0), 64)       # +x: pi/2
        self.assertEqual(bc.facing_to(0, 0, 0, -10), 128)
        self.assertEqual(bc.facing_to(0, 0, -10, 0), 192)

    def test_wildlife(self):
        _, wp, _ = self.plan()
        self.assertEqual(len(wp["base"]), 25)
        self.assertEqual(wp["total"], 5)
        kinds = [a["row"]["npc_name"] for a in wp["rows"]]
        self.assertEqual(len(kinds), 5)
        self.assertEqual(kinds.count("willowisp"), round(bc.WISP_SHARE * 5))
        self.assertEqual(kinds.count("mist wolf") + kinds.count("wild boar"), 5 - kinds.count("willowisp"))
        for a in wp["rows"]:
            r = a["row"]
            self.assertTrue(bc.ZONE["x"][0] <= r["x_coord"] < bc.ZONE["x"][1])
            self.assertTrue(bc.ZONE["z"][0] <= r["z_coord"] < bc.ZONE["z"][1])
            self.assertGreaterEqual(a["gap"], bc.WILD_SPACING)
            self.assertGreater(math.hypot(r["x_coord"] - TENT[0], r["z_coord"] - TENT[2]), bc.CAMP_CLEAR)

    def test_apply_again_undo(self):
        before = digests(self.db)
        self.run_tool("--apply")
        db = sqlite3.connect(self.db)
        camps = db.execute(f"SELECT count(*) FROM {bc.CAMPS_LOG}").fetchone()[0]
        wild = db.execute(f"SELECT count(*) FROM {bc.WILD_LOG}").fetchone()[0]
        self.assertEqual(camps, bc.WANT_TENT + bc.WANT_FIRE - 1)
        self.assertEqual(wild, 5)
        # the camp's respawn copied where its template had one, and every camp add stays put
        self.assertEqual(db.execute(f"SELECT count(*) FROM npcwander WHERE radius = -1 AND npcid IN "
                                    f"(SELECT npc_row FROM {bc.CAMPS_LOG})").fetchone()[0], camps)
        self.assertEqual(db.execute(f"SELECT count(*) FROM npcwander WHERE npcid IN "
                                    f"(SELECT npc_row FROM {bc.WILD_LOG})").fetchone()[0], 0)
        resp = db.execute(f"SELECT count(*) FROM {bc.CAMPS_LOG} WHERE respawn = 1").fetchone()[0]
        self.assertEqual(resp, db.execute("SELECT count(*) FROM npcrespawn").fetchone()[0] - 1)
        db.close()
        mid = digests(self.db)
        self.run_tool("--apply")
        self.assertEqual(digests(self.db), mid, "a second apply writes nothing")
        self.run_tool("--undo")
        self.assertEqual(digests(self.db), before, "undo puts every table back")
        db = sqlite3.connect(self.db)
        self.assertIsNone(db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (bc.CAMPS_LOG,)).fetchone())
        db.close()

    def test_parts_apart(self):
        before = digests(self.db)
        self.run_tool("--apply", "--part", "camps")
        after_camps = digests(self.db)
        self.run_tool("--apply", "--part", "wildlife")
        self.run_tool("--undo", "--part", "wildlife")
        self.assertEqual(digests(self.db), after_camps)
        self.run_tool("--undo", "--part", "camps")
        self.assertEqual(digests(self.db), before)


if __name__ == "__main__":
    unittest.main()
