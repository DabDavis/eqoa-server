import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
WIZ, RAN = 1 << 13, 1 << 1


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FixStyle0Looks(unittest.TestCase):
    """tools/fix_style0_looks.py on a made-up database. Armour of style 0 on a cloth icon (the slot's plain one, a
    robe's picture): the decided look first, then its name's measured material, then its name's measured kind
    (a tier, its style the commonest near the level), then its AC against the styled drops near its level (crafted
    records left out), no heavier than its class mask allows; AC 0 cloth; a look other than cloth takes that
    style's icon. Armour on a style's icon takes that style; a name's material reads another kind's icon; a
    shield takes the model its icon's shields carry, a held item the model held items and weapons carry, a weapon
    only weapons'; an icon nothing reads and a GM's record are left; --only limits a run; --replan rolls back;
    --reapply undoes and applies in one go; an undo puts every row back. No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "model INTEGER, itemicon INTEGER, AC INTEGER, levelreq INTEGER, classuse INTEGER, itemdesc TEXT, "
                   "craft INTEGER)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT)")
        rows, pid = [], 1

        def add(n, name, slot, model, icon, ac=10, level=20, desc=""):
            nonlocal pid
            for i in range(n):
                rows.append((pid, f"{name} {i}", slot, model, icon, ac, level, 0, desc, 0))
                pid += 1
        # chests at level 20: cloth AC 40 (icon 51, the plain one), leather 70, chain 100, plate 140
        add(3, "Woven Tunic", 5, 1, 51, 40)
        add(3, "Hide Vest", 5, 2, 52, 70)
        add(3, "Ring Shirt", 5, 3, 53, 100)
        add(8, "Plate Tunic", 5, 4, 54, 140)          # "plate": style 4, 8 of 8
        add(8, "Steel Breastplate", 5, 4, 54, 140)    # "breastplate": the plate tier, 8 of 8 drops
        add(10, "brass breastplate", 5, 5, 55, 300, desc="Armor forged of brass.")   # crafted: no drops
        add(3, "Gale Hauberk", 5, 7, 57, 150, level=40)   # icon 57 is style 7's
        add(3, "Robe", 2, 0, 21)                      # robes on icon 21
        add(8, "Chain Bracers", 7, 3, 73)             # bracers: "chain", style 3 on icon 73
        add(3, "Earring", 3, 0, 33)                   # earrings on icon 33
        add(3, "Kite Shield", 13, 777, 131)           # shields on icon 131 draw model 777
        add(2, "Wand", 12, 888, 181)                  # two weapons and one held item on icon 181 draw 888
        add(1, "Rod", 18, 888, 181)
        rows += [
            (100, "Basher Shirt", 5, 0, 51, 4, 1, 1, "", 0),            # decided: cloth
            (101, "Etheric Plate Tunic", 5, 0, 51, 100, 20, 0, "", 0),  # its name's plate, over its chain AC
            (102, "Plate Charm", 5, 0, 51, 0, 20, 0, "", 0),            # AC 0: cloth
            (103, "Sunfaded Tunic", 5, 0, 21, 45, 20, 0, "", 0),        # a robe's icon, cloth AC: cloth
            (104, "Gale Hauberk of the Gust", 5, 0, 57, 200, 40, 0, "", 0),  # its icon's style: 7
            (105, "Brazen Chain Bracers", 7, 0, 33, 50, 20, 0, "", 0),  # an earring's icon, its name chain
            (106, "Crown of Somebody", 1, 0, 99, 20, 20, 0, "", 0),     # an icon of its own: left
            (107, "GM Tunic", 5, 0, 51, 4, 20, 0, "", 0),               # a GM's: left
            (108, "Tower Shield", 13, 0, 131, 50, 20, 0, "", 0),        # its icon's shields: 777
            (109, "book of thought", 18, 0, 181, 0, 20, 0, "", 0),      # held: held items and weapons: 888
            (110, "Tome", 18, 0, 182, 0, 20, 0, "", 0),                 # no record draws icon 182: counted
            (111, "Stick", 12, 0, 181, 0, 20, 0, "", 0),                # a weapon: only the two weapons: left
            (112, "Mystery Chestguard", 5, 0, 51, 150, 20, 0, "", 0),   # AC plate; crafted 5s don't count: 4
            (113, "Odd Thing", 5, 0, 51, 72, 20, 0, "", 0),             # AC leather
            (114, "Wizard's Thing", 5, 0, 51, 140, 20, WIZ, "", 0),     # plate AC, a wizard's: cloth
            (115, "Ranger's Thing", 5, 0, 51, 140, 20, RAN, "", 0),     # plate AC, a ranger's: chain
            (116, "Odd Breastplate", 5, 0, 51, 40, 20, 0, "", 0),       # cloth AC, its kind's plate: 4
        ]
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", rows)
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

    def test_fix_replan_and_undo(self):
        out = self.tool()
        self.assertIn("13 records change", out)
        self.assertIn("plate style 4 (8/8)", out)
        self.assertIn("breastplate plate (8/8)", out)
        self.assertIn("left: 106 'Crown of Somebody': no rule reads its icon", out)
        self.assertIn("left: 107 'GM Tunic': a GM changed it", out)
        self.assertIn("held 1", out, "the tome is counted")
        self.assertIn("weapon 1", out, "the stick is counted")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 13 records", self.tool("--apply"))
        got = self.looks()
        for pid, look, why in [
                (100, (1, 51), "the user's word: cloth"), (101, (4, 54), "its name's plate, with plate's icon"),
                (102, (1, 51), "AC 0: cloth"), (103, (1, 21), "a robe's picture, cloth AC: cloth"),
                (104, (7, 57), "its icon's style"), (105, (3, 73), "its name's chain, chain's icon"),
                (106, (0, 99), "left"), (107, (0, 51), "a GM's"), (108, (777, 131), "the shields' model"),
                (109, (888, 181), "held items and weapons"), (110, (0, 182), "nothing draws its icon"),
                (111, (0, 181), "a weapon takes only weapons' models"),
                (112, (4, 54), "plate by AC, style 4: the crafted 5s are no drops"),
                (113, (2, 52), "leather by AC, leather's icon"), (114, (1, 51), "a wizard's: cloth at most"),
                (115, (3, 53), "a ranger's: chain at most"), (116, (4, 54), "its kind's plate over its AC")]:
            self.assertEqual(got[pid], look, why)
        self.assertIn("wrote 0 records", self.tool("--apply"), "a second apply finds nothing")
        after = dump(self.path)
        out = self.tool("--replan")
        self.assertIn("against what the log had: 0 records look otherwise", out)
        self.assertEqual(dump(self.path), after, "a re-plan rolls back")
        self.assertIn("wrote 13 records", self.tool("--reapply"))
        self.assertEqual(self.looks(), got, "a re-apply under the same rules gives the same looks")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
