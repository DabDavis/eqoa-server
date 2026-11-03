import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
PH = "Add a real description."


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FillItemDescriptions(unittest.TestCase):
    """tools/fill_item_descriptions.py on a made-up database: the placeholder gives way to the decided text, else
    Sandstorm's record, else EQOA-Data's (not a contributor's note, not a name it describes twice), else
    Allakhazam's page without its "Dropped by"; a real description, a GM's record and an undescribed one are
    left; an undo puts every row back. No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "itemdesc TEXT)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT)")
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?)", [
            (1, "Basher Shirt", 5, PH),               # decided
            (2, "Steel Mace", 12, PH),                # Sandstorm's, before EQOA-Data's
            (3, "Aged Club", 12, PH),                 # EQOA-Data's
            (4, "Good Robe", 2, PH),                  # EQOA-Data's is a note: kept
            (5, "Twice Told", 12, PH),                # EQOA-Data describes it twice: kept
            (6, "Peace Sword", 12, PH),               # Allakhazam's, "Dropped by" left out
            (7, "Real Thing", 12, "Its own words."),  # not the placeholder: untouched
            (8, "GM Mace", 12, PH),                   # a GM's: left
            (9, "Nameless", 12, "add a real description"),   # undescribed: kept
        ])
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('itempattern', ?)", (json.dumps({"patternid": 8}),))
        db.commit()
        db.close()
        self.before = dump(self.path)
        with open(os.path.join(self.dir, "harvest.json"), "w") as fh:
            json.dump({"wares": [{"name": "Steel Mace", "wear_slot": 12, "icon": 1, "desc": "A finely crafted mace."},
                                 {"name": "Aged Club", "wear_slot": 12, "icon": 2, "desc": PH},
                                 {"name": "GM Mace", "wear_slot": 12, "icon": 3, "desc": "A mace."}]}, fh)
        items = os.path.join(self.dir, "eqd", "Information", "databases", "items")
        os.makedirs(items)
        with open(os.path.join(items, "weapons.json"), "w") as fh:
            json.dump({"items": [{"name": "Steel Mace", "description": "Another text."},
                                 {"name": "Aged Club", "description": "A gnarled club."},
                                 {"name": "Good Robe", "description": "Alignment: Good."},
                                 {"name": "Twice Told", "description": "One."},
                                 {"name": "Twice Told", "description": "Two."}]}, fh)
        os.makedirs(os.path.join(self.dir, "pages"))
        with open(os.path.join(self.dir, "pages", "pages.json"), "w") as fh:
            json.dump({"1": {"name": "Peace Sword", "Description": "A long sword. Dropped by Highbourne Guard."}}, fh)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fill_item_descriptions.py"), "--db", self.path,
                               "--prices", os.path.join(self.dir, "harvest.json"),
                               "--eqoa-data", os.path.join(self.dir, "eqd"), "--pages", os.path.join(self.dir, "pages"),
                               *args], capture_output=True, text=True, check=True).stdout

    def descs(self):
        db = sqlite3.connect(self.path)
        out = dict(db.execute("SELECT patternid, itemdesc FROM itempattern"))
        db.close()
        return out

    def test_fill_and_undo(self):
        out = self.tool()
        self.assertIn("4 records change", out)
        self.assertIn("left: 8 'GM Mace': a GM changed it", out)
        self.assertIn("3 records keep the placeholder", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 4 records", self.tool("--apply"))
        got = self.descs()
        self.assertEqual(got[1], "A coarse shirt worn by the Bashers of Grobb.")
        self.assertEqual(got[2], "A finely crafted mace.", "Sandstorm's first")
        self.assertEqual(got[3], "A gnarled club.", "EQOA-Data's")
        self.assertEqual(got[4], PH, "a contributor's note is no description")
        self.assertEqual(got[5], PH, "two descriptions: neither")
        self.assertEqual(got[6], "A long sword.", "Allakhazam's, without the submitter's drop note")
        self.assertEqual(got[7], "Its own words.")
        self.assertEqual(got[8], PH, "a GM's record")
        self.assertIn("wrote 0 records", self.tool("--apply"))
        out = self.tool("--ours", "--apply")
        self.assertIn("wrote 3 records", out, "ours for the three no source describes")
        got = self.descs()
        for pid in (4, 5, 9):
            self.assertNotEqual(got[pid].lower().rstrip("."), PH.lower().rstrip("."))
            self.assertRegex(got[pid], r"^[A-Z].*\.$")
        self.assertEqual(got[8], PH, "a GM's record, ours or not")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
