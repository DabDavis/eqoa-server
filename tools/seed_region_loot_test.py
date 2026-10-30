#!/usr/bin/env python3
"""tools/seed_region_loot.py against a made-up world: the Rathe's creature of
a kind gets the rathe mountains table, Odus's the four Odus tables at
chances by their sizes, a creature of the water the marine table, and a
creature of a swamp quadrant the swamp table at the level its bound
creatures never reached; the controls get nothing: a named creature, a
town's person, a player's pet, a training dummy, a creature of another
quadrant, one on dry land, a level no item of the table holds. A dead item's window is stretched to
the nearest bound level within reach, one with none near is left; --apply then
--undo leaves npcloot and loottables as they were, and a second --apply is
refused. No game data.

    python3 tools/seed_region_loot_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_region_loot.py")
sys.path.insert(0, HERE)
import seed_region_loot as rl  # noqa: E402

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, npc_level INTEGER, npc_type INTEGER);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
"""

# EQOA-Data's map: two anchors fit the game to the map at a tenth, two zones (swamp west, forest east)
ZONES = {"zones": [{"name": "Bog", "x": 100, "y": 100, "biomes": ["Swamp"] * 4},
                   {"name": "Wood", "x": 300, "y": 100, "biomes": ["Forest"] * 4}],
         "towns": [], "pois": [{"name": "Anchor One", "x": 100, "y": 100, "category": "other"},
                               {"name": "Anchor Two", "x": 300, "y": 300, "category": "other"},
                               {"name": "Anchor Three", "x": 10, "y": 800, "category": "other"},
                               {"name": "Anchor Four", "x": 50, "y": 900, "category": "other"}]}

NPCS = [  # name, world, x, z, level, npc_type
    ("Anchor One", 0, 1000, 1000, 1, 128), ("Anchor Two", 0, 3000, 3000, 1, 128),
    ("Anchor Three", 2, 100, 8000, 1, 128), ("Anchor Four", 2, 500, 9000, 1, 128),
    ("hill troll", 1, 500, 500, 20, 0), ("Old Red", 1, 520, 500, 20, 0), ("training dummy", 1, 530, 500, 20, 0),
    ("sentry", 1, 540, 500, 20, 0),  # a town's person (a name-wide npcfaction row)
    ("primal spirit", 1, 550, 500, 20, 0),  # a player's pet recorded off the live game (an npcaggro seed row)
    ("goblin", 1, 560, 500, 28, 0), ("goblin", 1, 570, 500, 43, 0),
    ("kerran warrior", 2, 500, 500, 45, 0),
    ("bog beast", 0, 900, 1000, 10, 0), ("swamp rat", 0, 950, 1000, 40, 0), ("forest rat", 0, 3050, 1000, 40, 0),
    ("shark", 0, 5000, 5000, 20, 0), ("crab", 0, 5100, 5000, 20, 0),
]
LOOT = [  # table, patternid, lo, hi
    ("rathe mountains", 1, 15, 25), ("rathe mountains", 2, 31, 41), ("rathe mountains", 3, 56, 66),
    ("rathe mountains", 4, 25, 30),
    ("red desert", 10, 40, 50), ("red desert", 11, 41, 51), ("deep jungle", 12, 40, 50), ("wetlands", 13, 44, 54),
    ("wastelands", 14, 43, 53),
    ("swamp", 20, 5, 15), ("swamp", 21, 35, 45), ("marine", 30, 15, 25), ("forest", 40, 35, 45),
]


class FakeEnv:
    """A world's water: everything east of x 4,900 and south of z 4,900 is wet for the shark, dry land for the
    crab (x past 5,050)."""

    def __init__(self, path):
        pass

    def flags(self, x, y, z):
        return 1 if 4900 < x < 5050 and z > 4900 else 0


class SeedRegionLoot(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        self.data = os.path.join(self.dir.name, "data")
        os.makedirs(os.path.join(self.data, "Map"))
        with open(os.path.join(self.data, "Map", "zones.json"), "w") as f:
            json.dump(ZONES, f)
        self.ground = os.path.join(self.dir.name, "ground")
        os.makedirs(self.ground)
        open(os.path.join(self.ground, "0.env"), "w").close()
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type) "
                       "VALUES (?, ?, ?, 0, ?, ?, ?)", NPCS)
        db.execute("INSERT INTO npcfaction (name, faction) VALUES ('sentry', 'Citizens of the Rathe')")
        db.execute("INSERT INTO npcaggro (npc_name, aggressive, note, source) VALUES ('Primal Spirit', 0, "
                   "'a summoned pet''s name (spellpattern: Primal Spirit): a player''s pet recorded beside its owner', 'seed')")
        db.executemany("INSERT INTO loottables (tablename, patternid, minlevel, maxlevel) VALUES (?, ?, ?, ?)", LOOT)
        # the swamp table already bound to a level 10 creature: its level 40 item is dead
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, source) VALUES ('bog beast', 'swamp', 25, 1, 'camp-seed')")
        db.commit()
        db.close()
        rl.Env = FakeEnv

    def tearDown(self):
        self.dir.cleanup()

    def plan(self):
        db = sqlite3.connect(self.path)
        try:
            rows, left, every, tables = rl.plan(db, self.data, self.ground)
            wins, still = rl.stretches(db, every, tables, rows)
        finally:
            db.close()
        return {(n, t): c for n, t, c, _ in rows}, wins, still, left

    def test_each_region_and_the_controls(self):
        got, wins, still, left = self.plan()
        self.assertEqual(got.get(("hill troll", "rathe mountains")), 25)
        for name in ("old red", "training dummy", "sentry", "primal spirit"):
            self.assertFalse(any(n == name for n, _ in got), f"{name} was bound: {got}")
        # Odus's four tables share 25 by their sizes (2, 1, 1, 1 items: 10, 5, 5, 5)
        self.assertEqual({t: c for (n, t), c in got.items() if n == "kerran warrior"},
                         {"red desert": 10, "deep jungle": 5, "wetlands": 5, "wastelands": 5})
        # the swamp's dead level 40 item: the swamp quadrant's level 40 creature, not the forest's
        self.assertEqual(got.get(("swamp rat", "swamp")), 25)
        self.assertNotIn(("forest rat", "swamp"), got)
        self.assertNotIn(("bog beast", "swamp"), got, "a binding there already is never written again")
        # the water: the shark in it, not the crab on the shore
        self.assertEqual(got.get(("shark", "marine")), 25)
        self.assertFalse(any(n == "crab" for n, _ in got))

    def test_dead_windows_stretch_to_the_nearest_bound_level(self):
        got, wins, still, left = self.plan()
        by = {rid: (t, old, new) for rid, t, old, new in wins}
        # the Rathe's 31-41 item: goblins (bound by their level 28) at 28 and 43, 3 below and 2 above: up to 43
        self.assertIn(("rathe mountains", (31, 41), (31, 43)), by.values())
        # the control: 56-66 has no Rathe creature within ten levels (43 is 13 below): left
        self.assertFalse(any(old == (56, 66) for t, old, new in by.values()))
        self.assertEqual(still.get("rathe mountains"), 1)

    def test_apply_then_undo_is_exact(self):
        def state():
            db = sqlite3.connect(self.path)
            out = (db.execute("SELECT * FROM npcloot ORDER BY 1, 2").fetchall(),
                   db.execute("SELECT * FROM loottables ORDER BY id").fetchall())
            db.close()
            return out

        before = state()
        run = lambda *a: subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a],
                                        capture_output=True, text=True)
        # the tool as a program reads no fake water: give it no ground (the shark is then left)
        r = run("--eqoa-data", self.data, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("wrote", r.stdout)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT chance FROM npcloot WHERE npc_name = 'hill troll'").fetchone(), (25,))
        db.close()
        r = run("--eqoa-data", self.data, "--apply")
        self.assertNotEqual(r.returncode, 0, "a second --apply is refused")
        r = run("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("took back", r.stdout)
        self.assertNotIn("changed since", r.stdout)
        self.assertEqual(state(), before)


if __name__ == "__main__":
    unittest.main()
