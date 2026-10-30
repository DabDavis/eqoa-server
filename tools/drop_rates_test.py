#!/usr/bin/env python3
"""tools/drop_rates.py against a made-up world: a creature bound to a table at
25% one roll, the table holding four items of equal weight at its level, drops
each at 6.25% a kill (n50 10.7); an item whose level window no bound creature
has is never dropped; the tables of "*" reach every creature; a loot step's
kills are its count over its chance and its camp the spawns round the one
nearest its quest. No game data.

    python3 tools/drop_rates_test.py
"""
import math
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import drop_rates as dr  # noqa: E402
import import_quests as iq  # noqa: E402


class DropRates(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript("""
            CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
                z_coord REAL, npc_level INTEGER, npc_type INTEGER);
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT NOT NULL);
            CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER,
                minlevel INTEGER, maxlevel INTEGER, source TEXT);
            CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT);
        """)
        iq.ensure_quest_tables(db)
        db.executemany("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type) VALUES (?, 0, ?, 0, ?, ?, ?)",
                       [("Giver", 0, 0, 30, 128), ("orc pawn", 100, 0, 10, 0), ("orc pawn", 150, 0, 10, 0),
                        ("orc pawn", 5000, 0, 10, 0), ("rat", 200, 0, 3, 0)])
        db.executemany("INSERT INTO itempattern VALUES (?, ?)", [(i, f"item {i}") for i in range(1, 8)])
        db.executemany("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES (?, ?, 1, ?, ?)",
                       [("plains", 1, 5, 15), ("plains", 2, 5, 15), ("plains", 3, 5, 15), ("plains", 4, 5, 15),
                        ("plains", 5, 40, 50), ("gems", 6, 1, 60)])
        db.executemany("INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES (?, ?, ?, ?)",
                       [("orc pawn", "plains", 25, 1), ("*", "gems", 3, 1)])
        db.execute("INSERT INTO quests (questid, name, enabled, giver) VALUES (1, 'Belts', 1, 'Giver')")
        db.execute("INSERT INTO queststeps (questid, step, kind, target, items, count, chance) VALUES (1, 1, 'loot', 'orc pawn', '7', 2, 0.5)")
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def test_the_chances(self):
        db = sqlite3.connect(self.path)
        tables, binds, levels, names = dr.load(db)
        ev = dr.chances(tables, binds, levels)
        p = max(x[0] for x in ev[1])
        self.assertAlmostEqual(p, 0.0625)
        self.assertAlmostEqual(dr.n_for(p, 0.5), math.log(0.5) / math.log(1 - 0.0625))
        self.assertNotIn(5, ev, "a window no bound creature's level is in: never dropped")
        self.assertEqual({x[1] for x in ev[6]}, {"orc pawn", "rat"}, "the tables of * reach every creature")
        steps = dr.quest_loot(db)
        self.assertEqual(len(steps), 1)
        q, st, c, need, nm, camp, kills, minutes, spawned = steps[0]
        self.assertEqual((c, need, nm, camp, kills), (0.5, 2, ["orc pawn"], 2, 4.0),
                         "the far orc pawn is no part of the camp")
        self.assertAlmostEqual(minutes, 4 * max(dr.KILL_MINUTES / 2, dr.FASTEST_KILL))
        db.close()

    def test_the_report_runs(self):
        r = subprocess.run([sys.executable, "-E", "-s", os.path.join(HERE, "drop_rates.py"), "--db", self.path,
                            "--tsv", os.path.join(self.dir.name, "i.tsv")], capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("plains: 5 items, 1 names bound, 1 never dropped", r.stdout)
        with open(os.path.join(self.dir.name, "i.tsv")) as f:
            self.assertIn("no bound creature of levels 40-50", f.read())


if __name__ == "__main__":
    unittest.main()
