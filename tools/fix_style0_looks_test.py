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


class FixStyle0Looks(unittest.TestCase):
    """tools/fix_style0_looks.py on a made-up database: armour of style 0 on its slot's plain cloth icon or a
    robe's picture is cloth, unless its name says a measured material (then that style and its icon; not at AC
    0); armour on a style's icon takes that style; a name's material reads another kind's icon; a shield takes
    the model its icon's shields carry, a held item the model held items and weapons carry, a weapon only
    weapons'; an icon nothing reads and a GM's record are left; --only limits a run; an undo puts every row
    back. No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "model INTEGER, itemicon INTEGER, AC INTEGER)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT)")
        rows, pid = [], 1

        def add(n, name, slot, model, icon, ac=10):
            nonlocal pid
            for i in range(n):
                rows.append((pid, f"{name} {i}", slot, model, icon, ac))
                pid += 1
        add(3, "Woven Tunic", 5, 1, 51)          # chest: cloth on icon 51 (the plain one)
        add(8, "Plate Tunic", 5, 4, 54)          # "plate": style 4, 8 of 8, on icon 54
        add(3, "Gale Hauberk", 5, 7, 57)         # icon 57 is style 7's
        add(3, "Robe", 2, 0, 21)                 # robes on icon 21
        add(8, "Chain Bracers", 7, 3, 73)        # bracers: "chain", style 3 on icon 73
        add(3, "Earring", 3, 0, 33)              # earrings on icon 33
        add(3, "Kite Shield", 13, 777, 131)      # shields on icon 131 draw model 777
        add(2, "Wand", 12, 888, 181)             # two weapons and one held item on icon 181 draw model 888
        add(1, "Rod", 18, 888, 181)
        rows += [
            (100, "Basher Shirt", 5, 0, 51, 4),            # the plain icon: cloth
            (101, "Etheric Plate Tunic", 5, 0, 51, 100),   # its name: plate, and plate's icon
            (102, "Plate Charm", 5, 0, 51, 0),             # AC 0: its icon's cloth
            (103, "Sunfaded Tunic", 5, 0, 21, 30),         # a robe's icon: cloth
            (104, "Gale Hauberk of the Gust", 5, 0, 57, 200),  # its icon's style: 7
            (105, "Brazen Chain Bracers", 7, 0, 33, 50),   # an earring's icon, its name chain: 3 and chain's icon
            (106, "Crown of Somebody", 1, 0, 99, 20),      # an icon of its own: left
            (107, "GM Tunic", 5, 0, 51, 4),                # a GM's: left
            (108, "Tower Shield", 13, 0, 131, 50),         # its icon's shields: 777
            (109, "book of thought", 18, 0, 181, 0),       # held: held items and weapons on 181: 888
            (110, "Tome", 18, 0, 182, 0),                  # no record draws icon 182: counted
            (111, "Stick", 12, 0, 181, 0),                 # a weapon: only the two weapons count: left
        ]
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?)", rows)
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('itempattern', ?)", (json.dumps({"patternid": 107}),))
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        for f in os.listdir(self.dir):
            os.remove(os.path.join(self.dir, f))
        os.rmdir(self.dir)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fix_style0_looks.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=True).stdout

    def looks(self):
        db = sqlite3.connect(self.path)
        out = {pid: (m, i) for pid, m, i in db.execute("SELECT patternid, model, itemicon FROM itempattern")}
        db.close()
        return out

    def test_only(self):
        self.assertIn("wrote 1 records", self.tool("--only", "100", "--apply"))
        got = self.looks()
        self.assertEqual(got[100], (1, 51))
        self.assertEqual(got[101], (0, 51), "--only leaves the rest")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before)

    def test_fix_and_undo(self):
        out = self.tool()
        self.assertIn("8 records change", out)
        self.assertIn("plate style 4 (8/8)", out)
        self.assertIn("left: 106 'Crown of Somebody': no rule reads its icon", out)
        self.assertIn("left: 107 'GM Tunic': a GM changed it", out)
        self.assertIn("held 1", out, "the tome is counted")
        self.assertIn("weapon 1", out, "the stick is counted")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 8 records", self.tool("--apply"))
        got = self.looks()
        for pid, look, why in [
                (100, (1, 51), "the plain icon: cloth"), (101, (4, 54), "its name's plate, with plate's icon"),
                (102, (1, 51), "AC 0 keeps to its icon"), (103, (1, 21), "a robe's picture: cloth"),
                (104, (7, 57), "its icon's style"), (105, (3, 73), "its name's chain, chain's icon"),
                (106, (0, 99), "left"), (107, (0, 51), "a GM's"), (108, (777, 131), "the shields' model"),
                (109, (888, 181), "held items and weapons"), (110, (0, 182), "nothing draws its icon"),
                (111, (0, 181), "a weapon takes only weapons' models")]:
            self.assertEqual(got[pid], look, why)
        self.assertIn("wrote 0 records", self.tool("--apply"), "a second apply finds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
