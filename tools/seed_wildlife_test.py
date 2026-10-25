#!/usr/bin/env python3
"""tools/seed_wildlife.py against a made-up world: a database of a few
NPCs and FrontiersForge sightings, and a ground file of a flat plain with a
hill, a hollow below the land (a lake) and a town on it. No game data.

    python3 tools/seed_wildlife_test.py
"""
import math
import os
import sqlite3
import struct
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_wildlife as sw  # noqa: E402

PLAIN, LAND = 54.15, 54.0
AREA = dict(name="Testmoor", word=0, world=0, x=(10000.0, 12000.0), z=(4000.0, 6000.0), levels=(1, 12), land=LAND)
PC_BODY, ANT, BEETLE, WISP, PET = 0x1111, 0x2222, 0x3333, 0x4444, 0x5555


def height(x, z):
    """The made-up terrain: the plain, a hill round (11000, 5600) and a
    hollow 20 deep round (10500, 5450), beside the town and the ants."""
    h = PLAIN
    d = math.hypot(x - 11000, z - 5600)
    if d < 300:
        h += 60 * (1 - d / 300)
    d = math.hypot(x - 10500, z - 5450)
    if d < 200:
        h -= 20 * (1 - d / 200)
    return h


def write_ground(path, x0=9000.0, z0=3000.0, size=4000.0, step=25.0):
    """A ground file in internal/ground's format: one zone, a strip a row."""
    n = int(size / step) + 1
    strips = []
    for i in range(n - 1):
        s = []
        for j in range(n):
            for x in (x0 + i * step, x0 + (i + 1) * step):
                z = z0 + j * step
                s.append((x, height(x, z), z))
        strips.append(s)
    verts = [v for s in strips for v in s]
    minx, maxx = min(v[0] for v in verts), max(v[0] for v in verts)
    minz, maxz = min(v[2] for v in verts), max(v[2] for v in verts)
    miny = min(v[1] for v in verts)
    off = 12 + 44
    with open(path, "wb") as f:
        f.write(b"EQGR" + struct.pack("<II", 1, 1))
        f.write(struct.pack("<7fqII", minx, minz, maxx, maxz, minx, miny, minz, off, len(strips), len(verts)))
        for s in strips:
            f.write(struct.pack("<H", len(s)))
            for x, y, z in s:
                f.write(struct.pack("<hhh", int(round((x - minx) * 4)) - 32768, int(round((y - miny) * 8)),
                                    int(round((z - minz) * 4)) - 32768))


SCHEMA = [
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, "
    "y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, modelid INTEGER, "
    "size REAL, race TEXT, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER)",
    "CREATE TABLE charactermodel (modelid INTEGER)",
    "CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT)",
    "CREATE TABLE npcs_ff_import (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_level INTEGER, "
    "world INTEGER, x_coord REAL, y_coord REAL, z_coord REAL, pos_suspect INTEGER NOT NULL DEFAULT 0)",
]


def npc(db, name, x, z, model, level=1, ntype=0, world=0, size=1.0, race="0"):
    db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, hp, modelid, size, race, "
               "npc_level, npc_type, npc_id) VALUES (?, 7, ?, ?, ?, 0, ?, 100, ?, ?, ?, ?, ?, 99)",
               (name, x, height(x, z), z, world, model, size, race, level, ntype))


def sight(db, name, level, x, z, y=None, suspect=0):
    db.execute("INSERT INTO npcs_ff_import (npc_name, npc_level, world, x_coord, y_coord, z_coord, pos_suspect) "
               "VALUES (?, ?, 0, ?, ?, ?, ?)", (name, level, x, height(x, z) if y is None else y, z, suspect))


def build(path):
    db = sqlite3.connect(path, isolation_level=None)
    for s in SCHEMA:
        db.execute(s)
    db.execute("INSERT INTO charactermodel VALUES (?)", (PC_BODY,))
    db.execute("INSERT INTO spellpattern VALUES (1, 'Lesser Golem')")
    # namesakes far away (another part of the world): the bodies to copy
    for i in range(4):
        npc(db, "Ant " if i % 2 else "ant", 2000 + 30 * i, 2000, ANT, level=1, size=0.75, race="6")
    for i in range(4):
        npc(db, "firebeetle", 2000 + 30 * i, 2100, BEETLE, level=2 + i % 2)
    npc(db, "Lone Wisp", 2000, 2200, WISP, level=5)                 # one of a kind
    npc(db, "Watchman Bob", 2000, 2300, PC_BODY, level=30)          # a players' race body
    npc(db, "Lesser Golem", 2000, 2400, PET, level=10)              # a pet's body (a spell's name)
    npc(db, "Lesser Golem", 2030, 2400, PET, level=10)
    npc(db, "Lesser Animation", 2000, 2500, PET, level=10)          # an enchanter's animation
    npc(db, "Lesser Animation", 2030, 2500, PET, level=10)
    # a town of 12 on the plain at (10400, 5000), its people 40 apart
    for i in range(12):
        npc(db, f"Townsman {i}", 10400 + 40 * (i % 4), 5000 + 40 * (i // 4), PC_BODY, level=30)
    # a camp of 6 beetles round (11500, 4400)
    for i in range(6):
        npc(db, "firebeetle", 11500 + 25 * (i % 3), 4400 + 25 * (i // 3), BEETLE, level=3)
    # a field of pebble crabs 50 apart round the ants, past their camp radius
    # (so the ants are no camp): a spawn out there has room only between
    # them, so a plan that ignored the spacing would stand on some
    for i in range(-6, 7):
        for j in range(-6, 7):
            x, z = 10500 + 50 * i + 25, 5250 + 50 * j + 25
            if sw.CAMP_RADIUS + 5 <= math.hypot(x - 10500, z - 5250) <= 300:
                npc(db, "pebble crab", x, z, 0x6666, level=1)
    # the sightings
    sight(db, "Ant", 2, 10500, 5250)          # next to the town and the lake: round it
    sight(db, "firebeetle", 3, 11520, 4410)   # in the beetles' camp
    sight(db, "Lone Wisp", 5, 11000, 4200)
    sight(db, "Watchman Bob", 10, 10450, 5050)
    sight(db, "Lesser Golem", 10, 11200, 4800)
    sight(db, "Lesser Animation", 10, 11300, 4800)
    sight(db, "Nobody Known", 4, 11400, 4800)
    sight(db, "ant", 2, 11900, 5900, suspect=1)
    sight(db, "ant", 2, 12100, 5000)          # just outside the area: its spread reaches in
    return db


class SeedWildlifeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.gdir = tempfile.TemporaryDirectory()
        cls.ground = cls.gdir.name
        write_ground(os.path.join(cls.ground, "0.ground"))

    @classmethod
    def tearDownClass(cls):
        cls.gdir.cleanup()

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        self.db = build(self.path)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def plan(self, **kw):
        return sw.plan(self.db, self.ground, areas=[AREA], **kw)

    def test_ground_matches_the_file(self):
        g = sw.Ground(os.path.join(self.ground, "0.ground"))
        for x, z in ((10500, 5250), (11000, 5600), (10500, 5450), (11950, 5980), (10012.5, 4033.3)):
            self.assertAlmostEqual(g.floors(x, z)[0], height(x, z), delta=0.3, msg=(x, z))

    def test_spawns_stand_on_land_out_of_town_and_apart(self):
        p = self.plan()
        self.assertTrue(p["rows"])
        g = sw.Ground(os.path.join(self.ground, "0.ground"))
        town = [(10400 + 40 * (i % 4), 5000 + 40 * (i // 4)) for i in range(12)]
        others = [(r[0], r[1]) for r in self.db.execute("SELECT x_coord, z_coord FROM npcs")]
        placed = []
        for r in p["rows"]:
            row = r["row"]
            x, y, z = row["x_coord"], row["y_coord"], row["z_coord"]
            self.assertTrue(AREA["x"][0] <= x <= AREA["x"][1] and AREA["z"][0] <= z <= AREA["z"][1], row)
            self.assertIn(round(y, 3), [round(f, 3) for f in g.floors(x, z)], "on the terrain")
            self.assertGreaterEqual(y, LAND, "never in the hollow (a lake)")
            self.assertGreater(math.hypot(x - 10500, z - 5450), 190, "nor in its bowl")
            self.assertGreaterEqual(min(math.hypot(x - tx, z - tz) for tx, tz in town), sw.TOWN_CLEAR - 1e-6)
            for ox, oz in others + placed:
                self.assertGreaterEqual(math.hypot(x - ox, z - oz), sw.SPACING - 1e-6)
            placed.append((x, z))

    def test_the_lake_is_beside_the_ants(self):
        # the land check has something to refuse: the ants' spread reaches
        # into the hollow, so a plan without it would put some there
        g = sw.Ground(os.path.join(self.ground, "0.ground"))
        self.assertLess(g.floors(10500, 5450)[0], LAND)
        self.assertLess(math.hypot(10500 - 10500, 5250 - 5450), sw.band(sw.BAND_SPREAD, 2)[1])

    def test_bodies_names_and_levels_are_the_namesakes(self):
        p = self.plan()
        ants = [r["row"] for r in p["rows"] if r["row"]["npc_name"].lower() == "ant"]
        self.assertEqual(len(ants), sw.band(sw.BAND_COUNT, 2))
        for a in ants:
            self.assertEqual(a["npc_name"], "ant", "the lower-case spelling on a tie, trimmed")
            self.assertEqual(a["modelid"], ANT)
            self.assertEqual((a["size"], a["race"], a["npc_type"]), (0.75, "6", 0))
            self.assertIn(a["npc_level"], (1, 2), "the namesakes' 1, one over, and the sighting's 2")
            self.assertEqual((a["npc_id"], a["zone"], a["dt_kill"], a["world"]), (0, 0, None, 0))

    def test_left_out(self):
        p = self.plan()
        names = {r["row"]["npc_name"].lower() for r in p["rows"]}
        self.assertEqual(names, {"ant", "firebeetle"})
        why = {name.lower(): reason for _, name, _, reason in p["left"]}
        self.assertIn("one of a kind", why["lone wisp"])
        self.assertIn("players' race", why["watchman bob"])
        self.assertIn("pet", why["lesser golem"])
        self.assertIn("pet", why["lesser animation"])
        self.assertIn("no body", why["nobody known"])

    def test_a_camp_is_seeded_round_it(self):
        p = self.plan()
        beetles = [r["row"] for r in p["rows"] if r["row"]["npc_name"] == "firebeetle"]
        self.assertEqual(len(beetles), sw.CAMP_COUNT)
        for b in beetles:
            d = math.hypot(b["x_coord"] - 11520, b["z_coord"] - 4410)
            self.assertTrue(sw.CAMP_SPREAD[0] <= d <= sw.CAMP_SPREAD[1], d)

    def test_skip(self):
        p = self.plan(skip=["firebeetle"])
        self.assertEqual({r["row"]["npc_name"] for r in p["rows"]}, {"ant"})
        self.assertIn("--skip", dict((n.lower(), w) for _, n, _, w in p["left"])["firebeetle"])

    def test_the_same_plan_twice(self):
        a = [r["row"] for r in self.plan()["rows"]]
        b = [r["row"] for r in self.plan()["rows"]]
        self.assertEqual(a, b)
        c = [r["row"] for r in self.plan(seed=99)["rows"]]
        self.assertNotEqual(a, c)

    def test_apply_and_undo(self):
        before = sw.rows_digest(self.db)
        seq = sw.npc_seq(self.db)
        p = self.plan()

        class Args:
            seed, with_guide_species, skip = sw.DEFAULT_SEED, False, ""

        sw.apply(self.db, p, Args)
        self.assertEqual(self.db.execute("SELECT count(*) FROM wildlife_seed_npcs").fetchone()[0], len(p["rows"]))
        self.assertNotEqual(sw.rows_digest(self.db), before)
        with self.assertRaises(SystemExit):
            sw.apply(self.db, p, Args)
        self.assertEqual(sw.undo(self.db), len(p["rows"]))
        self.assertEqual(sw.rows_digest(self.db), before, "every row as it was")
        self.assertEqual(sw.npc_seq(self.db), seq, "AUTOINCREMENT's counter back")
        self.assertFalse(sw.seeded(self.db))
        with self.assertRaises(SystemExit):
            sw.undo(self.db)

    def test_undo_keeps_a_row_added_after(self):
        p = self.plan()

        class Args:
            seed, with_guide_species, skip = sw.DEFAULT_SEED, False, ""

        sw.apply(self.db, p, Args)
        npc(self.db, "added by a GM", 11800, 5800, BEETLE)
        top = self.db.execute("SELECT max(id) FROM npcs").fetchone()[0]
        sw.undo(self.db)
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcs WHERE npc_name = 'added by a GM'").fetchone()[0], 1)
        self.assertGreaterEqual(sw.npc_seq(self.db), top, "the counter never goes below a row that stays")


if __name__ == "__main__":
    unittest.main()
