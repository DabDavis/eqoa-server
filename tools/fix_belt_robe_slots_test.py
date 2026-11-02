import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))

BELT_ICON, ROBE_ICON, BAR_ICON = 100, 200, 300


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FixBeltRobeSlots(unittest.TestCase):
    """tools/fix_belt_robe_slots.py on a made-up database: the icon families come from the names in the two
    slots; a belt in the robe slot and a robe in the belt slot move, an icon of neither family goes by its
    description, a GM's record and one a measured record contradicts are left, an item outside the two slots
    moves only on a measured record, worn copies follow their records (a swap included), and an undo puts
    every row back. No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "itemicon INTEGER, itemdesc TEXT)")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, "
                   "patternid INTEGER, equiploc INTEGER)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT)")
        rows = [
            # the belts' icon: named belts in both slots
            (1, "Rope Belt", 9, BELT_ICON, ""), (2, "Leather Sash", 9, BELT_ICON, ""),
            (3, "Girdle of the Ox", 2, BELT_ICON, ""), (4, "Bowline Belt", 2, BELT_ICON, ""),
            # the robes' icon
            (5, "Plain Robe", 2, ROBE_ICON, ""), (6, "Silk Gown", 2, ROBE_ICON, ""),
            (7, "Robe of the Belt", 9, ROBE_ICON, ""),
            # an icon of neither family: by the description, else left
            (8, "Ingot of Strength", 2, BAR_ICON, "This belt has an ingot on it."),
            (9, "Shiny Trinket", 2, BAR_ICON, "Something to wear."),
            # a GM's belt in the robe slot: left
            (10, "Guarded Belt", 2, BELT_ICON, ""),
            # a robe-icon item in the belt slot that a Sandstorm record calls a belt: left
            (11, "Odd Sash", 9, ROBE_ICON, ""),
            # a belt in the chest slot: moves only on Allakhazam's Waist list
            (12, "Chest Belt", 5, BELT_ICON, ""),
            # controls: a scarf with the belts' icon in the neck slot, a chest piece of the robes' icon
            (13, "Red Scarf", 4, BELT_ICON, ""), (14, "Silk Tunic", 5, ROBE_ICON, ""),
            # worn: a second belt-in-robe and robe-in-belt pair, for the copies
            (15, "Worn Belt", 2, BELT_ICON, ""), (16, "Worn Robe", 9, ROBE_ICON, ""),
        ]
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)", rows)
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('itempattern', ?)", (json.dumps({"patternid": 10}),))
        db.executemany("INSERT INTO charinventory (serverid, patternid, equiploc) VALUES (?, ?, ?)", [
            (1, 15, 21),           # a belt in the robe slot, nothing at 11: to 11
            (2, 15, 21), (2, 1, 11),   # ... with a belt already at 11: into the pack
            (3, 15, 21), (3, 16, 11),  # a swapped pair: each to its own slot
            (4, 4, -1),            # carried: untouched
            (5, 5, 21),            # a real robe worn: untouched
        ])
        db.commit()
        db.close()
        self.before = dump(self.path)
        os.makedirs(os.path.join(self.dir, "pages", "lists"))
        with open(os.path.join(self.dir, "pages", "lists", "slot-Waist.html"), "w") as fh:
            fh.write("<a href=item.html?eqoaitem=12>Chest Belt</a> <a href=item.html?eqoaitem=4>Bowline Belt</a>")
        with open(os.path.join(self.dir, "pages", "lists", "slot-Robe.html"), "w") as fh:
            fh.write("<a href=item.html?eqoaitem=7>Robe of the Belt</a>")
        with open(os.path.join(self.dir, "harvest.json"), "w") as fh:
            json.dump({"merchants": [{"name": "Tailor", "wares": [
                {"id": 900, "name": "Odd Sash", "wear_slot": 9, "icon": 555, "value": 1},
                {"id": 901, "name": "Plain Robe", "wear_slot": 2, "icon": ROBE_ICON, "value": 1}]}]}, fh)

    def tearDown(self):
        for root, dirs, files in os.walk(self.dir, topdown=False):
            for f in files:
                os.remove(os.path.join(root, f))
            for d in dirs:
                os.rmdir(os.path.join(root, d))
        os.rmdir(self.dir)

    def tool(self, *args, measured=True):
        extra = ["--prices", os.path.join(self.dir, "harvest.json"), "--pages", os.path.join(self.dir, "pages")] if measured else []
        return subprocess.run([sys.executable, os.path.join(HERE, "fix_belt_robe_slots.py"), "--db", self.path, *extra, *args],
                              capture_output=True, text=True, check=True).stdout

    def slots(self):
        db = sqlite3.connect(self.path)
        out = dict(db.execute("SELECT patternid, equipslot FROM itempattern"))
        db.close()
        return out

    def test_without_measured_records(self):
        out = self.tool(measured=False)
        self.assertIn("icon families: 1 belts', 1 robes'", out)
        self.assertIn("7 records move", out, "3, 4, 7, 8, 11 (nothing measured contradicts its icon), 15, 16; "
                                             "not 12 (outside the two slots, nothing measured)")
        self.assertIn("left: 'Guarded Belt': a GM changed it", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")

    def test_fix_and_undo(self):
        out = self.tool()
        self.assertIn("Allakhazam: 2 agree with the rule, 0 disagree", out)
        self.assertIn("left: 'Shiny Trinket'", out)
        self.assertIn("left: 'Guarded Belt': a GM changed it", out)
        self.assertIn("left: 'Odd Sash': Sandstorm's record says slot 9, its icon 2", out)
        self.assertIn("wrote 7 records, 4 worn copies moved", self.tool("--apply"))
        got = self.slots()
        for pid, slot, why in [(3, 9, "a belt in the robe slot"), (4, 9, "the Bowline Belt"), (7, 2, "a robe in the belt slot"),
                               (8, 9, "its description's belt"), (12, 9, "Allakhazam's Waist list"),
                               (15, 9, "worn belt"), (16, 2, "worn robe")]:
            self.assertEqual(got[pid], slot, why)
        for pid, slot, why in [(9, 2, "nothing says"), (10, 2, "a GM's"), (11, 9, "Sandstorm contradicts its icon"),
                               (13, 4, "a scarf with the belts' icon, nothing measured"), (14, 5, "a tunic with the robes' icon"),
                               (1, 9, "a belt already"), (5, 2, "a robe already")]:
            self.assertEqual(got[pid], slot, f"left: {why}")
        db = sqlite3.connect(self.path)
        worn = sorted(db.execute("SELECT serverid, patternid, equiploc FROM charinventory"))
        db.close()
        self.assertEqual(worn, [(1, 15, 11), (2, 1, 11), (2, 15, -1), (3, 15, 11), (3, 16, 21), (4, 4, -1), (5, 5, 21)],
                         "worn copies follow: to the belt slot, into the pack when it is taken, a swapped pair each home")
        self.assertIn("wrote 0 records, 0 worn copies", self.tool("--apply"), "a second apply finds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
