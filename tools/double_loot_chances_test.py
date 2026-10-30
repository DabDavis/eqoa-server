#!/usr/bin/env python3
"""tools/double_loot_chances.py against a made-up npcloot: every creature's
row doubled, at most 100; the controls left: the rows of "*" (every
creature's), a row at 100 already, a row of no chance. --apply then --undo
leaves npcloot as it was; a second --apply is refused; a row a GM changed
since is left by --undo. No game data.

    python3 tools/double_loot_chances_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "double_loot_chances.py")


class DoubleLootChances(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("""CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
            rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
            PRIMARY KEY (npc_name, tablename))""")
        db.executemany("INSERT INTO npcloot (npc_name, tablename, chance) VALUES (?, ?, ?)",
                       [("orc pawn", "plains", 25), ("orc pawn", "generic", 15), ("ogre", "mountain", 60),
                        ("lady vox", "boss lady vox", 100), ("*", "craft gems", 3), ("*", "craft modifiers", 3)])
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *a):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a], capture_output=True, text=True)

    def rows(self):
        db = sqlite3.connect(self.path)
        out = dict(((n, t), c) for n, t, c in db.execute("SELECT npc_name, tablename, chance FROM npcloot"))
        db.close()
        return out

    def test_double_and_undo(self):
        before = self.rows()
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        got = self.rows()
        self.assertEqual(got[("orc pawn", "plains")], 50)
        self.assertEqual(got[("orc pawn", "generic")], 30)
        self.assertEqual(got[("ogre", "mountain")], 100, "doubled, at most 100")
        # the controls
        self.assertEqual(got[("lady vox", "boss lady vox")], 100)
        self.assertEqual((got[("*", "craft gems")], got[("*", "craft modifiers")]), (3, 3), "every creature's tables are no creature's")
        self.assertNotEqual(self.run_tool("--apply").returncode, 0, "a second --apply is refused")
        # a GM's change since is left by the undo
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcloot SET chance = 40 WHERE npc_name = 'orc pawn' AND tablename = 'generic'")
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertIn("1 changed since", r.stdout)
        got = self.rows()
        self.assertEqual(got[("orc pawn", "generic")], 40)
        got[("orc pawn", "generic")] = 15
        self.assertEqual(got, before)


if __name__ == "__main__":
    unittest.main()
