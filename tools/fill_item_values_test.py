import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))

STATS = ["str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
         "PR", "DR", "FR", "CR", "LR", "AR"]


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


def ware(i, name, slot, level, value, dmg=0, ac=0):
    return {"id": i, "name": name, "wear_slot": slot, "level_req": level, "value": value, "damage": dmg,
            "icon": 1, "max_stack": 1, "desc": "", "no_trade": 0, "lore": 0, "stats": [[14, ac]] if ac else []}


class FillItemValues(unittest.TestCase):
    """tools/fill_item_values.py on a made-up database: gear of level 2 and up worth 0 takes Sandstorm's value
    by name, else the rule fitted on Sandstorm's values (damage for a plain weapon, the nearest measured
    items otherwise); a Sandstorm 0, a NO RENT record, a crafted product, a repeatable quest's reward (a
    reward, a choice, a give event) and a GM's record are left, as are levels 0-1, priced records and what is
    not gear; a non-repeatable quest's reward is valued; an undo puts every row back. No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemcost INTEGER, "
                   "equipslot INTEGER, levelreq INTEGER, maxstack INTEGER, weapondamage INTEGER, itemicon INTEGER, "
                   "itemdesc TEXT, trade INTEGER, lore INTEGER, rent INTEGER, " + ", ".join(f"{s} INTEGER" for s in STATS) + ")")
        db.execute("CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, product INTEGER)")
        db.execute("CREATE TABLE quests (questid INTEGER PRIMARY KEY, repeatable INTEGER, events TEXT)")
        db.execute("CREATE TABLE queststeps (questid INTEGER, step INTEGER, rewarditems TEXT, byclass TEXT, "
                   "choices TEXT, events TEXT)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT)")
        db.execute("CREATE TABLE spelleffect (spellid INTEGER, line TEXT, fxdict INTEGER, source TEXT, effects TEXT)")
        db.executemany("INSERT INTO spelleffect VALUES (?, ?, 0, 'test', ?)", [
            (1, "Self, Summons Fizzy Potion x3 (sow)", ""),
            (2, "Target, Summons a zombie to your aid", ""),
            (3, "", json.dumps({"items": [{"name": "Workbook Charm", "count": 1}]}))])

        def item(pid, name, cost, slot, level, dmg=0, rent=1, ac=0):
            stats = {s: 0 for s in STATS}
            stats["AC"] = ac
            return (pid, name, cost, slot, level, 1, dmg, 1, "", 0, 0, rent, *[stats[s] for s in STATS])
        rows = [
            item(200, "Iron Sword", 0, 12, 5, dmg=100),          # Sandstorm's by name: 185
            item(201, "Rusty Blade", 0, 12, 10, dmg=300),        # the rule: 1.847 x 300
            item(202, "Hide Cap", 0, 1, 10, ac=5),               # the rule: the 3 nearest armour, median 110
            item(203, "Starter Shirt", 0, 5, 1),                 # level 1: left
            item(204, "Conjured Staff", 0, 15, 20, dmg=50, rent=0),  # NO RENT: left
            item(300, "Reward Belt", 0, 9, 20),                  # a repeatable quest's reward: left
            item(301, "Choice Ring", 0, 8, 20),                  # ... its choice: left
            item(302, "Fine Belt", 0, 9, 20),                    # a one-time quest's reward: valued
            item(303, "Event Gloves", 0, 19, 20, ac=5),          # a repeatable quest's give event: left
            item(304, "Crafted Ring", 0, 8, 20),                 # a crafted product: left
            item(305, "GM Robe", 0, 2, 20),                      # a GM's: left
            item(306, "Free Handout", 0, 5, 5),                  # Sandstorm's own 0: left
            item(307, "Priced Cap", 50, 1, 10, ac=5),            # priced: left
            item(308, "Quest Bone", 0, -1, 10),                  # not gear: left
            item(309, "Bait", 0, 21, 10),                        # not gear: left
            item(310, "Fizzy Potion", 0, 16, 10, dmg=20),        # a spell's line summons it: left
            item(311, "Workbook Charm", 0, 4, 10),               # a spell's workbook items name it: left
            item(312, "Misfiled Sword", 0, 17, 40),              # wear slot 17: left
            item(313, "Grand Ring", 0, 8, 40),                   # jewelry: the outlier ring is no measure of it
        ]
        db.executemany(f"INSERT INTO itempattern VALUES ({','.join('?' * (12 + len(STATS)))})", rows)
        db.execute("INSERT INTO craftrecipes (product) VALUES (304)")
        db.executemany("INSERT INTO quests VALUES (?, ?, ?)", [
            (1, 1, json.dumps([{"on": "finish", "do": "give", "items": "303"}])),
            (2, 0, "")])
        db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?, ?)", [
            (1, 1, "300*1", "", json.dumps({"ask": "?", "options": [{"answer": "a", "items": "301"}]}), ""),
            (2, 1, "302", "", "", "")])
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('itempattern', ?)", (json.dumps({"patternid": 305}),))
        db.commit()
        db.close()
        self.before = dump(self.path)
        with open(os.path.join(self.dir, "harvest.json"), "w") as fh:
            json.dump({"merchants": [{"name": "Smith", "wares": [
                ware(1, "Iron Sword", 12, 5, 185, dmg=100), ware(2, "Iron Axe", 12, 9, 370, dmg=200),
                ware(3, "Iron Mace", 12, 3, 92, dmg=50),
                ware(4, "Leather Cap", 1, 10, 100, ac=5), ware(5, "Leather Boots", 11, 10, 110, ac=5),
                ware(6, "Leather Gloves", 19, 10, 120, ac=5), ware(7, "Silk Robe", 2, 30, 3000),
                ware(8, "Free Handout", 5, 5, 0),
                ware(9, "Gold Ring", 8, 40, 3000), ware(10, "Silver Ring", 8, 41, 3200), ware(11, "Ruby Ring", 8, 39, 3100),
                ware(12, "Crafted Ring", 8, 40, 5)]}]}, fh)

    def tearDown(self):
        for f in os.listdir(self.dir):
            os.remove(os.path.join(self.dir, f))
        os.rmdir(self.dir)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fill_item_values.py"), "--db", self.path,
                               "--prices", os.path.join(self.dir, "harvest.json"), *args],
                              capture_output=True, text=True, check=True).stdout

    def test_fill_and_undo(self):
        out = self.tool()
        self.assertIn("5 records valued", out)
        for name, why in [("Conjured Staff", "NO RENT"), ("Reward Belt", "a repeatable quest's reward"),
                          ("Choice Ring", "a repeatable quest's reward"), ("Event Gloves", "a repeatable quest's reward"),
                          ("Crafted Ring", "a crafted product"), ("GM Robe", "a GM changed it"),
                          ("Fizzy Potion", "a spell summons it"), ("Workbook Charm", "a spell summons it"),
                          ("Misfiled Sword", "wear slot 17")]:
            self.assertRegex(out, f"left: \\d+ '{name}': {why}")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 5 values", self.tool("--apply"))
        db = sqlite3.connect(self.path)
        got = dict(db.execute("SELECT patternid, itemcost FROM itempattern"))
        db.close()
        self.assertEqual(got[200], 185, "Sandstorm's value by name")
        self.assertAlmostEqual(got[201], 554, delta=2, msg="a plain weapon: the fitted multiple of its damage")
        self.assertEqual(got[202], 110, "armour: the median of the 3 nearest measured items")
        self.assertGreater(got[302], 0, "a one-time quest's reward is valued")
        self.assertEqual(got[313], 3100, "a rare ring: the median of the three rings near it, not the 5 of a crafted one")
        for pid in (203, 204, 300, 301, 303, 304, 305, 306, 308, 309, 310, 311, 312):
            self.assertEqual(got[pid], 0, f"pattern {pid} is left at 0")
        self.assertEqual(got[307], 50, "a price of its own is kept")
        self.assertIn("wrote 0 values", self.tool("--apply"), "a second apply finds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
