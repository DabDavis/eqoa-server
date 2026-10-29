#!/usr/bin/env python3
"""tools/fix_weapon_records.py against a made-up database and harvest: a
weapon of no attack type and no model takes Sandstorm's record and the model
most clubs here wear; a potion is left; a second --apply is refused; --undo
gives back the table as it was. No game data.

    python3 tools/fix_weapon_records_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_weapon_records.py")


class FixWeaponRecords(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("""CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER,
            attacktype INTEGER, weapondamage INTEGER, levelreq INTEGER, model INTEGER)""")
        db.execute("CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY)")
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?)", [
            (1825, "Worn Club", 14, 0, 20, 2, 0),
            (5488, "Apprentice Club", 12, 3, 10, 1, 152490910),
            (1902, "oaken gnarled club", 14, 3, 190, 19, 152490910),
            (1986, "Barbed Urchin Club", 14, 3, 378, 36, 77),
            (50005, "Mace Potion", 14, 0, 5, 1, 0),
        ])
        db.execute("INSERT INTO itemeffect VALUES (50005)")
        db.commit()
        db.close()
        self.harvest = os.path.join(self.dir.name, "h.json")
        with open(self.harvest, "w") as f:
            json.dump({"merchants": [{"wares": [
                {"name": "Worn Club", "wear_slot": 14, "attack_type": 3, "damage": 20, "level_req": 2},
                {"name": "Mace Potion", "wear_slot": 14, "attack_type": 3, "damage": 5, "level_req": 1}]}]}, f)

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
        self.assertIn("1 weapons to change", self.run_tool().stdout)
        self.run_tool("--apply")
        got = {r[0]: r for r in self.rows()}
        self.assertEqual(got[1825], (1825, "Worn Club", 14, 3, 20, 2, 152490910))  # the clubs' commonest model
        self.assertEqual(got[50005], before[-1])  # a potion: left
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.rows(), before)


if __name__ == "__main__":
    unittest.main()
