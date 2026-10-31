#!/usr/bin/env python3
"""tools/seed_town_wildlife.py against a made-up world (seed_wildlife_test's
plain, hill and hollow), with a town on it, a square already full of newbies,
and a pond on the plain that the environment file flags as water. No game
data.

    python3 tools/seed_town_wildlife_test.py
"""
import math
import os
import sqlite3
import struct
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_town_wildlife as st  # noqa: E402
import seed_wildlife as sw  # noqa: E402
import seed_wildlife_test as wt  # noqa: E402

SPARK, RAT = 0x7777, 0x8888
POND = (11400.0, 11700.0)     # x: water on the plain, at the land's own height
TOWN = dict(world=0, x=11000.0, z=5000.0, ring=(0.0, 800.0), land=54.0,
            kinds={"sparkflies": 4, "rat": 1, "nobody": 1})
TOWNSFOLK = [(10900 + 40 * (i % 4), 4950 + 40 * (i // 4)) for i in range(12)]
FULL = (11125.0, 4625.0)      # the middle of a square that holds three newbies already


def write_env(path):
    """One zone; its tree: water where POND[0] <= x <= POND[1], dry elsewhere."""
    def tree(nodes, leaves):
        out = struct.pack("<II", len(nodes), len(leaves))
        for n in nodes:
            out += struct.pack("<4fII", *n)
        return out + struct.pack(f"<{len(leaves)}I", *leaves)

    world = tree([], [0])
    # node 0: x - POND[0] >= 0 -> node 1, else leaf 0 (dry);
    # node 1: POND[1] - x >= 0 -> leaf 1 (water), else leaf 0
    zone = tree([(1.0, 0.0, 0.0, -POND[0], 1, st.LEAF | 0), (-1.0, 0.0, 0.0, POND[1], st.LEAF | 1, st.LEAF | 0)],
                [0, 1])
    with open(path, "wb") as f:
        f.write(b"EQEV" + struct.pack("<I", 1) + world + struct.pack("<I", 1) + zone)


def build(path):
    db = sqlite3.connect(path, isolation_level=None)
    for s in wt.SCHEMA:
        db.execute(s)
    db.execute("INSERT INTO charactermodel VALUES (?)", (wt.PC_BODY,))
    # the kinds' namesakes, far off
    for i in range(4):
        wt.npc(db, "sparkflies", 2000 + 30 * i, 2000, SPARK, level=1 + i % 3)
        wt.npc(db, "rat", 2000 + 30 * i, 2100, RAT, level=1 + i)
    for x, z in TOWNSFOLK:
        wt.npc(db, "Townsman", x, z, wt.PC_BODY, level=30)
    # a full square: three newbies in it already
    for i in range(3):
        wt.npc(db, "sparkflies", FULL[0] - 60 + 40 * i, FULL[1], SPARK, level=1)
    return db


def squares(db):
    """Newbies (level 5 or less, of the town's kinds) by square."""
    out = {}
    for x, z in db.execute("SELECT x_coord, z_coord FROM npcs WHERE npc_level <= 5 AND npc_name IN ('sparkflies', 'rat')"):
        k = (math.floor(x / st.CELL), math.floor(z / st.CELL))
        out[k] = out.get(k, 0) + 1
    return out


class SeedTownWildlifeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.gdir = tempfile.TemporaryDirectory()
        cls.ground = cls.gdir.name
        wt.write_ground(os.path.join(cls.ground, "0.ground"))
        write_env(os.path.join(cls.ground, "0.env"))

    @classmethod
    def tearDownClass(cls):
        cls.gdir.cleanup()

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = build(os.path.join(self.tmp.name, "t.db"))
        st.TOWNS["Testvale"] = TOWN
        self.saved = (st.BAD_ENV, st.TOWN_CLEAR)

    def tearDown(self):
        st.BAD_ENV, st.TOWN_CLEAR = self.saved
        st.TOWNS.pop("Testvale", None)
        self.db.close()
        self.tmp.cleanup()

    def plan(self):
        return st.plan(self.db, self.ground, "Testvale", seed=7)

    def test_the_water_is_kept_out(self):
        p = self.plan()
        self.assertGreater(len(p["rows"]), 20)
        wet = [r for r in p["rows"] if POND[0] <= r["row"]["x_coord"] <= POND[1]]
        self.assertEqual(wet, [], "a spawn in the pond")
        # the control: with the environment not read, the pond takes some
        st.BAD_ENV = 0
        wet = [r for r in self.plan()["rows"] if POND[0] <= r["row"]["x_coord"] <= POND[1]]
        self.assertTrue(wet, "the control: the pond took no spawn with the water not looked at")

    def test_the_town_and_the_hollow_are_kept_out(self):
        p = self.plan()
        for r in p["rows"]:
            x, y, z = r["row"]["x_coord"], r["row"]["y_coord"], r["row"]["z_coord"]
            self.assertGreaterEqual(y, TOWN["land"], f"under the land at ({x:.0f}, {z:.0f})")
            self.assertLessEqual(math.hypot(x - TOWN["x"], z - TOWN["z"]), TOWN["ring"][1])
            near = min(math.hypot(x - tx, z - tz) for tx, tz in TOWNSFOLK)
            self.assertGreaterEqual(near, st.TOWN_CLEAR, f"({x:.0f}, {z:.0f}) {near:.0f} from a townsman")
        # the control: no clearance puts one among the town's people
        st.TOWN_CLEAR = 0.0
        near = min(min(math.hypot(r["row"]["x_coord"] - tx, r["row"]["z_coord"] - tz) for tx, tz in TOWNSFOLK)
                   for r in self.plan()["rows"])
        self.assertLess(near, self.saved[1], "the control: a plan with no clearance kept off the town anyway")

    def test_squares_are_filled_to_their_count(self):
        before = squares(self.db)
        p = self.plan()
        self.assertEqual([k for k, _ in p["left"]], ["nobody"])
        full = (math.floor(FULL[0] / st.CELL), math.floor(FULL[1] / st.CELL))
        self.assertEqual(before.get(full), 3)
        self.assertFalse([r for r in p["rows"] if (math.floor(r["row"]["x_coord"] / st.CELL),
                                                   math.floor(r["row"]["z_coord"] / st.CELL)) == full],
                         "a spawn in a square that had three already")
        st.apply(self.db, p, "Testvale", 7)
        after = squares(self.db)
        for k, n in after.items():
            self.assertLessEqual(n, max(st.PER_CELL, before.get(k, 0)), f"square {k}: {n}")
        kinds = {r["kind"] for r in p["rows"]}
        self.assertEqual(kinds, {"sparkflies", "rat"})
        for r in p["rows"]:
            self.assertEqual(r["row"]["modelid"], SPARK if r["kind"] == "sparkflies" else RAT)
            self.assertEqual(r["row"]["npc_id"], 0)

    def test_undo_gives_back_every_row(self):
        digest, seq = sw.rows_digest(self.db), sw.npc_seq(self.db)
        p = self.plan()
        st.apply(self.db, p, "Testvale", 7)
        self.assertEqual(self.db.execute("SELECT count(*) FROM town_wildlife_seed_npcs").fetchone()[0], len(p["rows"]))
        with self.assertRaises(SystemExit):
            st.apply(self.db, p, "Testvale", 7)
        self.assertEqual(st.undo(self.db, "Testvale"), len(p["rows"]))
        self.assertEqual(sw.rows_digest(self.db), digest)
        self.assertEqual(sw.npc_seq(self.db), seq)
        self.assertFalse(st.logged(self.db), "the log tables stayed")
        # the control: a digest after the apply is not the one before
        st.apply(self.db, self.plan(), "Testvale", 7)
        self.assertNotEqual(sw.rows_digest(self.db), digest)


if __name__ == "__main__":
    unittest.main()
