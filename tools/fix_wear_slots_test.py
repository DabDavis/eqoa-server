#!/usr/bin/env python3
"""tools/fix_wear_slots.py against a made-up database, harvest and Allakhazam
lists: an item in slot 0 takes Sandstorm's slot, else its Allakhazam list's
(a shield on any list 13, Arms 7 or one wrist 6), else what its name says;
the stand-in icon (on most slot 0 items, on no Sandstorm item) gives way to
its Sandstorm icon, else the icon of Sandstorm's items of its slot; a weapon
is left to fix_dump_weapons.py and a name that says nothing is listed; a
second --apply is refused; --undo gives back the table as it was. No game
data.

    python3 tools/fix_wear_slots_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_wear_slots.py")


class FixWearSlots(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("""CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemicon INTEGER,
            equipslot INTEGER, attacktype INTEGER, weapondamage INTEGER)""")
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?)", [
            (1, "Verdant Bracers", 9, 0, 0, 0),          # Sandstorm's: slot 7, its icon 70
            (2, "Iron Bracers", 9, 0, 0, 0),             # Allakhazam's Arms: 7
            (3, "Amber Bracelet", 9, 0, 0, 0),           # Arms, one wrist: 6
            (4, "Splintered Shield", 9, 0, 0, 0),        # on the Secondary list, a shield: 13
            (5, "Ring of Sorts", 9, 0, 0, 0),            # its name: 8
            (6, "Daran's Sight", 9, 0, 0, 0),            # nothing says
            (7, "Murky Saber", 9, 0, 1, 40),             # a weapon: fix_dump_weapons.py's
            (8, "Old Helm", 55, 1, 0, 0),                # not in slot 0
        ])
        db.commit()
        db.close()
        self.harvest = os.path.join(self.dir.name, "h.json")
        with open(self.harvest, "w") as f:
            json.dump({"merchants": [{"wares": [
                {"name": "Verdant Bracers", "wear_slot": 7, "icon": 70},
                {"name": "Plain Bracers", "wear_slot": 7, "icon": 71},
                {"name": "Plain Bracers II", "wear_slot": 7, "icon": 71},
                {"name": "Plain Bracelet", "wear_slot": 6, "icon": 60},
                {"name": "Plain Ring", "wear_slot": 8, "icon": 80},
                {"name": "Plain Shield", "wear_slot": 13, "icon": 130}]}]}, f)
        self.pages = os.path.join(self.dir.name, "pages")
        os.makedirs(os.path.join(self.pages, "lists"))
        for word, items in (("Arms", [(10, "Iron Bracers"), (11, "Amber Bracelet")]),
                            ("Secondary", [(12, "Splintered Shield")])):
            with open(os.path.join(self.pages, "lists", f"slot-{word}.html"), "w") as f:
                f.write("".join(f"<a href=item.html?eqoaitem={i}>{n}</a><br>" for i, n in items))

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--pages", self.pages,
                            "--prices", self.harvest, *args], capture_output=True, text=True)
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
        out = self.run_tool().stdout
        self.assertIn("slot 0's stand-in icon: 9", out)
        self.assertIn("5 items given a slot", out)
        self.assertIn("left as it is: Daran's Sight (nothing says where it is worn)", out)
        self.run_tool("--apply")
        got = {r[0]: (r[3], r[2]) for r in self.rows()}   # slot, icon
        self.assertEqual(got[1], (7, 70))     # Sandstorm's slot and its own icon
        self.assertEqual(got[2], (7, 71))     # Arms; Sandstorm's bracers' commonest icon
        self.assertEqual(got[3], (6, 60))     # one wrist
        self.assertEqual(got[4], (13, 130))   # a shield, whatever the list
        self.assertEqual(got[5], (8, 80))     # its name
        self.assertEqual(got[6], (0, 9))      # left
        self.assertEqual(got[7], (0, 9))      # a weapon: not this tool's
        self.assertEqual(got[8], (1, 55))     # not in slot 0
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.rows(), before)


if __name__ == "__main__":
    unittest.main()
