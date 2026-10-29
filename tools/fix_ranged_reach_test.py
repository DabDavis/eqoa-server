#!/usr/bin/env python3
"""tools/fix_ranged_reach.py against a made-up database and harvest: a
throwing knife Sandstorm sells takes its record (range and stack), a crossbow
that was no ranged weapon takes its slot, attack type and damage, a bow
Sandstorm does not sell takes the range rule's 25, a range already set and a
potion (an itemeffect row) are left, a second --apply is refused and --undo
gives back the table as it was. No game data.

    python3 tools/fix_ranged_reach_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_ranged_reach.py")

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, maxstack INTEGER,
    attacktype INTEGER, weapondamage INTEGER, levelreq INTEGER, unk5 INTEGER);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY);
"""


def ware(name, slot, stack, kind, dmg, lvl, rng):
    return {"name": name, "wear_slot": slot, "max_stack": stack, "attack_type": kind, "damage": dmg,
            "level_req": lvl, "range": rng, "price": 10}


class FixRangedReach(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?, ?)", [
            (1, "rusty throwing knife", 16, 99, 10, 58, 1, 0),
            (2, "Simple War Crossbow", 12, 1, 0, 127, 19, 0),
            (3, "Bow of Somewhere", 16, 1, 7, 400, 30, 0),
            (4, "Ranged Already", 16, 1, 7, 300, 20, 22),
            (5, "Acidic Potion", 16, 999, 10, 0, 4, 20),
            (6, "Rusty Sword", 14, 1, 1, 50, 1, 0),
        ])
        db.execute("INSERT INTO itemeffect VALUES (5)")
        db.commit()
        db.close()
        self.harvest = os.path.join(self.dir.name, "harvest.json")
        with open(self.harvest, "w") as f:
            json.dump({"merchants": [{"name": "Bowyer Test", "wares": [
                ware("rusty throwing knife", 16, 2000, 10, 58, 1, 25),
                ware("Simple War Crossbow", 16, 1, 9, 220, 19, 25),
                ware("Acidic Potion", 16, 2000, 10, 0, 4, 25)]}]}, f)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--prices", self.harvest, *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def rows(self):
        c = sqlite3.connect(self.db)
        try:
            return c.execute("SELECT * FROM itempattern ORDER BY patternid").fetchall()
        finally:
            c.close()

    def test_apply_and_undo(self):
        before = self.rows()
        self.assertIn("3 items to change (2 by Sandstorm's record)", self.run_tool().stdout)
        self.assertEqual(self.rows(), before)
        self.run_tool("--apply")
        got = {r[0]: r for r in self.rows()}
        self.assertEqual(got[1], (1, "rusty throwing knife", 16, 2000, 10, 58, 1, 25))
        self.assertEqual(got[2], (2, "Simple War Crossbow", 16, 1, 9, 220, 19, 25))
        self.assertEqual(got[3][7], 25)                 # not Sandstorm's: the rule's range
        self.assertEqual(got[4], before[3])             # a range already set
        self.assertEqual(got[5], before[4])             # a potion: ours by design
        self.assertEqual(got[6], before[5])             # a sword: no ranged attack type
        r = self.run_tool("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.rows(), before)


if __name__ == "__main__":
    unittest.main()
