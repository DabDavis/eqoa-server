#!/usr/bin/env python3
"""tools/hold_legendary_loot.py against made-up tables: a legendary item's
rows in the pooled tables go to weight 0 and come back; the controls stay: an
uncommon item's row, a legendary's row in a boss's table. A second --apply is
refused; a row a GM changed since is left by --undo. No game data.

    python3 tools/hold_legendary_loot_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "hold_legendary_loot.py")


class HoldLegendaryLoot(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, levelreq INTEGER, equipslot INTEGER, str INTEGER)")
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)",
                       [(1, "Aura of Karana", 26, 13, 0), (2, "ring of plenty", 26, 8, 10), (3, "Grimblade", 26, 12, 0)])
        db.execute("""CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
            weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
            source TEXT NOT NULL DEFAULT 'gm')""")
        db.executemany("INSERT INTO loottables (tablename, patternid) VALUES (?, ?)",
                       [("plains", 1), ("plains", 2), ("generic", 3), ("boss snowgrin", 3)])
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *a):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a], capture_output=True, text=True)

    def weights(self):
        db = sqlite3.connect(self.path)
        out = {(t, p): w for t, p, w in db.execute("SELECT tablename, patternid, weight FROM loottables")}
        db.close()
        return out

    def test_hold_and_undo(self):
        before = self.weights()
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        got = self.weights()
        self.assertEqual((got[("plains", 1)], got[("generic", 3)]), (0, 0))
        self.assertEqual((got[("plains", 2)], got[("boss snowgrin", 3)]), (1, 1), "an uncommon, and a boss's table")
        self.assertNotEqual(self.run_tool("--apply").returncode, 0, "a second --apply is refused")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE loottables SET weight = 5 WHERE tablename = 'generic'")
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertIn("put back 1 rows; 1 changed since", r.stdout)
        got = self.weights()
        self.assertEqual(got[("generic", 3)], 5)
        got[("generic", 3)] = 1
        self.assertEqual(got, before)


if __name__ == "__main__":
    unittest.main()
