#!/usr/bin/env python3
"""tools/fix_single_spawn_steps.py against a made-up world: a loot step that
farms one spawn for four items goes to 1.0; the controls stay: a camp of two
spawns, a step wanting one item, a step at 1.0 already, a draft quest's step,
a step whose creature a spawn event brings. --apply then --undo leaves the
steps as they were; a second --apply is refused; a step changed since is left
by --undo. No game data.

    python3 tools/fix_single_spawn_steps_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_single_spawn_steps.py")
sys.path.insert(0, HERE)
import import_quests as iq  # noqa: E402


class FixSingleSpawnSteps(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript("""
            CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
                z_coord REAL, npc_level INTEGER, npc_type INTEGER);
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT NOT NULL);
        """)
        iq.ensure_quest_tables(db)
        db.executemany("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type) VALUES (?, 0, ?, 0, ?, ?, ?)",
                       [("Giver", 0, 0, 30, 128), ("Drafter", 5000, 5000, 30, 128),
                        ("goblin commander", 100, 0, 15, 0),
                        ("orc pawn", 200, 0, 3, 0), ("orc pawn", 230, 0, 3, 0),
                        ("lone wolf", 300, 0, 5, 0), ("lone bear", 400, 0, 5, 0), ("lone boar", 5100, 5000, 5, 0),
                        ("lone ghost", 600, 0, 9, 0)])
        db.executemany("INSERT INTO itempattern VALUES (?, ?)", [(i, f"item {i}") for i in range(1, 8)])
        db.execute("INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (1, 'Idols', 1, 'Giver', 15)")
        db.execute("INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (2, 'A draft', 0, 'Drafter', 5)")
        s = "INSERT INTO queststeps (questid, step, kind, target, items, count, chance, events) VALUES (?, ?, 'loot', ?, ?, ?, ?, ?)"
        spawn = json.dumps([{"on": "start", "do": "spawn", "npc": "lone ghost", "hostile": True}])
        for row in [(1, 1, "goblin commander", "1", 4, 0.5, ""), (1, 2, "orc pawn", "2", 4, 0.5, ""),
                    (1, 3, "lone wolf", "3", 1, 0.5, ""), (1, 4, "lone bear", "4", 3, 1.0, ""),
                    (1, 5, "lone ghost", "6", 2, 0.5, spawn), (2, 1, "lone boar", "5", 3, 0.5, "")]:
            db.execute(s, row)
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *a):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a], capture_output=True, text=True)

    def chances(self):
        db = sqlite3.connect(self.path)
        out = {(q, n): c for q, n, c in db.execute("SELECT questid, step, chance FROM queststeps")}
        db.close()
        return out

    def test_the_one_spawn_step_and_the_controls(self):
        before = self.chances()
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("1 steps to 1.0", r.stdout)
        got = self.chances()
        self.assertEqual(got[(1, 1)], 1.0)
        for k in [(1, 2), (1, 3), (1, 4), (1, 5), (2, 1)]:
            self.assertEqual(got[k], before[k], f"control {k} changed")
        self.assertNotEqual(self.run_tool("--apply").returncode, 0, "a second --apply is refused")
        r = self.run_tool("--undo")
        self.assertIn("put back 1", r.stdout)
        self.assertEqual(self.chances(), before)

    def test_a_step_changed_since_is_left(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE queststeps SET chance = 0.75 WHERE questid = 1 AND step = 1")
        db.commit()
        db.close()
        self.assertIn("1 changed since", self.run_tool("--undo").stdout)
        self.assertEqual(self.chances()[(1, 1)], 0.75)


if __name__ == "__main__":
    unittest.main()
