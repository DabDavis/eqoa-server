import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FixItemLooks(unittest.TestCase):
    """tools/fix_item_looks.py on a made-up database: the woolen helm and gloves take the cloth style and its
    icon; an icon Sandstorm's gear record gives otherwise is taken, and a style 0 armour piece takes the style
    that icon is; a style whose icon agrees with it is not overturned by another style's icon; a Sandstorm
    record that is not gear, a GM's record and the plain style 0 pieces are left; an undo puts every row back.
    No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "model INTEGER, itemicon INTEGER)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT)")
        rows = []
        pid = 1
        # the styles' icons: helms 11 (cloth), 12 (leather), 14 (plate); gloves 21, 22; chests 53 (chain), 57
        for slot, style, icon, n in [(1, 1, 11, 3), (1, 2, 12, 3), (1, 4, 14, 3), (19, 1, 21, 3), (19, 2, 22, 3),
                                     (5, 3, 53, 3), (5, 7, 57, 3)]:
            for i in range(n):
                rows.append((pid, f"piece {slot}-{style}-{i}", slot, style, icon))
                pid += 1
        rows += [
            (100, "woolen helm", 1, 2, 12), (101, "Woolen Gloves", 19, 2, 22),
            (102, "Antique Plate Helm", 1, 0, 11),      # Sandstorm: icon 14 (plate's): style 4
            (103, "Fancy Ring", 8, 0, 81),              # Sandstorm: icon 82
            (104, "Chain Coat", 5, 7, 57),              # Sandstorm: icon 53 (chain's): its style 7 and icon agree: left
            (105, "Spell Thing", 8, 0, 1),              # Sandstorm's is a scroll (not gear): left
            (106, "GM Helm", 1, 0, 11),                 # a GM's: left
            (107, "Plain Hood", 1, 0, 11),              # style 0 on the cloth icon, nothing measured: counted only
        ]
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)", rows)
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('itempattern', ?)", (json.dumps({"patternid": 106}),))
        db.commit()
        db.close()
        self.before = dump(self.path)
        with open(os.path.join(self.dir, "harvest.json"), "w") as fh:
            json.dump({"merchants": [{"name": "Armorer", "wares": [
                {"id": 1, "name": "Antique Plate Helm", "wear_slot": 1, "icon": 14},
                {"id": 2, "name": "Fancy Ring", "wear_slot": 8, "icon": 82},
                {"id": 3, "name": "Chain Coat", "wear_slot": 5, "icon": 53},
                {"id": 4, "name": "Spell Thing", "wear_slot": -1, "icon": 999},
                {"id": 5, "name": "GM Helm", "wear_slot": 1, "icon": 14},
                {"id": 6, "name": "Woolen Helm", "wear_slot": 1, "icon": 12}]}]}, fh)

    def tearDown(self):
        for f in os.listdir(self.dir):
            os.remove(os.path.join(self.dir, f))
        os.rmdir(self.dir)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fix_item_looks.py"), "--db", self.path,
                               "--prices", os.path.join(self.dir, "harvest.json"), *args],
                              capture_output=True, text=True, check=True).stdout

    def looks(self):
        db = sqlite3.connect(self.path)
        out = {pid: (m, i) for pid, m, i in db.execute("SELECT patternid, model, itemicon FROM itempattern")}
        db.close()
        return out

    def test_fix_and_undo(self):
        out = self.tool()
        self.assertIn("4 records change", out)
        self.assertIn("left: 'Chain Coat': Sandstorm's icon is style 3's, its style 7 and its icon agree", out)
        self.assertIn("left: 'GM Helm': a GM changed it", out)
        self.assertIn("slot 1: 1 (1 on the cloth icon)", out, "the plain hood is counted")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 4 records", self.tool("--apply"))
        got = self.looks()
        self.assertEqual(got[100], (1, 11), "the woolen helm: cloth, though Sandstorm's icon is leather's")
        self.assertEqual(got[101], (1, 21), "the woolen gloves: cloth")
        self.assertEqual(got[102], (4, 14), "Sandstorm's icon, and the style it is")
        self.assertEqual(got[103], (0, 82), "a ring takes the icon, no style")
        for pid, look, why in [(104, (7, 57), "a style its icon agrees with"), (105, (0, 1), "Sandstorm's is no gear"),
                               (106, (0, 11), "a GM's"), (107, (0, 11), "nothing measured")]:
            self.assertEqual(got[pid], look, f"left: {why}")
        self.assertIn("wrote 0 records", self.tool("--apply"), "a second apply finds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
