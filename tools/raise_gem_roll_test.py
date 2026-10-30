#!/usr/bin/env python3
"""tools/raise_gem_roll.py against a made-up npcloot: the gems' roll of "*"
goes 3 to 5 and back; the controls: the modifiers' roll and a creature's row
are left, a second --apply is refused, a roll changed since is left by
--undo, and a database without the gems' row is refused. No game data.

    python3 tools/raise_gem_roll_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "raise_gem_roll.py")


class RaiseGemRoll(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("""CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
            rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
            PRIMARY KEY (npc_name, tablename))""")
        db.executemany("INSERT INTO npcloot (npc_name, tablename, chance) VALUES (?, ?, ?)",
                       [("*", "craft gems", 3), ("*", "craft modifiers", 3), ("craft gems", "craft gems", 3)])
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *a, db=None):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", db or self.path, "--no-backup", *a],
                              capture_output=True, text=True)

    def chances(self):
        db = sqlite3.connect(self.path)
        out = dict(((n, t), c) for n, t, c in db.execute("SELECT npc_name, tablename, chance FROM npcloot"))
        db.close()
        return out

    def test_raise_and_undo(self):
        before = self.chances()
        self.assertEqual(self.run_tool("--apply").returncode, 0)
        got = self.chances()
        self.assertEqual(got[("*", "craft gems")], 5)
        self.assertEqual((got[("*", "craft modifiers")], got[("craft gems", "craft gems")]), (3, 3))
        self.assertNotEqual(self.run_tool("--apply").returncode, 0, "a second --apply is refused")
        r = self.run_tool("--undo")
        self.assertIn("back to 3", r.stdout)
        self.assertEqual(self.chances(), before)

    def test_a_roll_changed_since_is_left(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcloot SET chance = 7 WHERE npc_name = '*' AND tablename = 'craft gems'")
        db.commit()
        db.close()
        self.assertIn("changed since", self.run_tool("--undo").stdout)
        self.assertEqual(self.chances()[("*", "craft gems")], 7)

    def test_no_gems_row_is_refused(self):
        empty = os.path.join(self.dir.name, "e.db")
        db = sqlite3.connect(empty)
        db.execute("CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT)")
        db.commit()
        db.close()
        r = self.run_tool(db=empty)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("seed_tradeskills", r.stderr)


if __name__ == "__main__":
    unittest.main()
