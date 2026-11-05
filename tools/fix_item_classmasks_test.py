import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
LEATHER_LIST = "Bard, Cleric, Druid, Monk, Paladin, Ranger, Rogue, Shadowknight, Shaman, Warrior"


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


def page(name, slot, level, classes):
    """An item page as Allakhazam's are laid out (made up)."""
    return (f"<html><body><table><tr><td class=itemname>Item: {name}</td></tr>"
            f"<tr><td><b>Classes</b>: {classes}</td></tr><tr><td><b>Races</b>: All Races</td></tr>"
            f"<tr><td><b>Slot</b>: {slot}</td></tr><tr><td><b>Item Level</b>: {level}</td></tr>"
            f"<tr><td><b>Submitted by</b>: someone</td></tr></table>Forum Preferences:</body></html>")


class FixItemClassmasks(unittest.TestCase):
    """tools/fix_item_classmasks.py on a made-up database: a Sandstorm record of the same name and slot (its
    level where two differ, none where neither is of it), an Allakhazam page of the same name and slot (its
    classes line first, which fetch_item_pages.read_page drops; the pre-alchemist leather list as the leather
    wearers of today; an empty class line as nothing), the same item
    here (a "(copy)" aside), an armour set's other pieces within three levels, and the rule (jewellery every
    class, armour by its material, a look its AC chose one material lighter or as --ac-armour says, weapons
    by kind; style 0 and a fishing pole left); a GM's record left; a race mask of 0 given every race; held
    copies a new mask refuses listed; --no-rule; an undo puts every row back but one changed since. No game
    data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "levelreq INTEGER, classuse INTEGER, raceuse INTEGER, model INTEGER, attacktype INTEGER, "
                   "itemdesc TEXT)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT)")
        db.execute("CREATE TABLE characters (serverid INTEGER PRIMARY KEY, tclass INTEGER)")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, "
                   "equiploc INTEGER)")
        db.execute("CREATE TABLE loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER)")
        db.execute("CREATE TABLE style0_look_fix_log (seq INTEGER PRIMARY KEY, patternid INTEGER, new_model INTEGER, "
                   "source TEXT)")
        rows = [
            # the same item here
            (1, "Wobble Band", 8, 10, 32767, 1023, 0, 0, ""), (2, "Wobble Band", 8, 12, 0, 1023, 0, 0, ""),
            (3, "Glimmer Bauble (copy)", 3, 5, 0, 1023, 0, 0, ""), (4, "Glimmer Bauble", 3, 5, 2048, 1023, 0, 0, ""),
            # Sandstorm's records: same id; another slot (then Allakhazam); two levels; neither level
            (10, "Quibble Visor", 1, 20, 0, 1023, 4, 0, ""), (11, "Zephyr Cuffs", 7, 15, 0, 1023, 2, 0, ""),
            (12, "Pickle Charm", 4, 30, 0, 1023, 0, 0, ""), (13, "Muddle Charm", 4, 35, 0, 1023, 0, 0, ""),
            # Allakhazam's pages: leather before the alchemist, all classes, an empty line, casters
            (20, "Fuzzle Leggings", 10, 22, 0, 1023, 2, 0, ""), (21, "Squiggle Ring", 8, 18, 0, 1023, 0, 0, ""),
            (22, "Plinky Earring", 3, 9, 0, 1023, 0, 0, ""), (23, "Snorkel Boots", 11, 30, 0, 1023, 4, 0, ""),
            # the Snorkel set: gloves within three levels of the boots, a helm ten off
            (30, "Snorkel Gloves", 19, 31, 0, 1023, 4, 0, ""), (31, "Snorkel Helm", 1, 40, 0, 1023, 4, 0, ""),
            # the rule
            (40, "Pudding Hood", 1, 5, 0, 1023, 1, 0, ""), (41, "Clunk Greaves", 10, 30, 0, 1023, 4, 0, ""),
            (42, "Clank Greaves", 10, 30, 0, 1023, 4, 0, ""), (43, "Wiggle Wrap", 5, 20, 0, 1023, 0, 0, ""),
            (44, "Toffee Hatchet", 12, 5, 0, 1023, 7, 1, ""), (45, "Noodle Pole", 20, 1, 0, 1023, 0, 0, ""),
            # a GM's; a race mask of 0; not worn; one with a mask
            (50, "Kept Ring", 8, 20, 0, 1023, 0, 0, ""), (60, "Raceless Ring", 8, 20, 0, 0, 0, 0, ""),
            (70, "A Scroll", -1, 1, 0, 1023, 0, 0, ""), (80, "Masked Helm", 1, 20, 879, 1023, 3, 0, ""),
        ]
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", rows)
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('itempattern', ?)", (json.dumps({"patternid": 50}),))
        for pid in (30, 31, 42):
            db.execute("INSERT INTO style0_look_fix_log (patternid, new_model, source) VALUES (?, 4, ?)",
                       (pid, "its AC (plate at level 30, style 4), on the slot's plain cloth icon"))
        db.execute("INSERT INTO characters VALUES (1, 13)")      # a wizard wearing the plate greaves
        db.execute("INSERT INTO charinventory VALUES (1, 1, 41, 12)")
        db.execute("INSERT INTO loottables (tablename, patternid) VALUES ('t', 21)")
        db.commit()
        db.close()
        self.before = dump(self.path)
        self.harvest = os.path.join(self.dir, "harvest.json")
        with open(self.harvest, "w") as fh:
            # (id, name, wear slot, level, class mask): the same id; another slot; two levels; neither level
            wares = [(10, "Quibble Visor", 1, 20, 13), (99, "Zephyr Cuffs", 6, 15, 15360),
                     (12, "Pickle Charm", 4, 30, 896), (98, "Pickle Charm", 4, 40, 15360),
                     (13, "Muddle Charm", 4, 30, 896), (97, "Muddle Charm", 4, 40, 15360), (96, "A Scroll", -1, 1, 1)]
            json.dump({"merchants": [{"name": "Somebody", "wares": [
                {"id": i, "name": n, "wear_slot": w, "level_req": lv, "class_mask": m, "race_mask": 1023}
                for i, n, w, lv, m in wares]}]}, fh)
        self.pages = os.path.join(self.dir, "pages")
        os.makedirs(os.path.join(self.pages, "pages"))
        for iid, args in {501: ("Zephyr Cuffs", "Arms", 15, "Monk"), 502: ("Fuzzle Leggings", "Legs", 22, LEATHER_LIST),
                          503: ("Squiggle Ring", "Finger", 18, "All Classes"), 504: ("Plinky Earring", "Ear", 9, ""),
                          505: ("Snorkel Boots", "Feet", 30, "Enchanter, Magician, Necromancer, Wizard"),
                          506: ("Quibble Visor", "Head", 20, "Warrior")}.items():
            with open(os.path.join(self.pages, "pages", f"{iid}.html"), "w") as fh:
                fh.write(page(*args))

    def tearDown(self):
        for root, dirs, files in os.walk(self.dir, topdown=False):
            for f in files:
                os.remove(os.path.join(root, f))
            for d in dirs:
                os.rmdir(os.path.join(root, d))
        os.rmdir(self.dir)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fix_item_classmasks.py"), "--db", self.path,
                               "--harvests", self.harvest, "--pages", self.pages, *args],
                              capture_output=True, text=True, check=True).stdout

    def masks(self):
        db = sqlite3.connect(self.path)
        out = {pid: (c, r) for pid, c, r in db.execute("SELECT patternid, classuse, raceuse FROM itempattern")}
        db.close()
        return out

    def test_fix_and_undo(self):
        out = self.tool("--list")
        self.assertIn("20 wearable records with no class mask: 17 given one, 3 left", out)
        self.assertIn("by source: sandstorm 2, allakhazam 4, same item 2, set 1, rule 8", out)
        self.assertIn("50 'Kept Ring' (slot 8, level 20): a GM changed it", out)
        self.assertIn("43 'Wiggle Wrap' (slot 5, level 20): rule: armour of style 0: no one material", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("held copies the new masks refuse to their holder's class", out)
        self.assertIn("41 'Clunk Greaves': character 1, WIZ, worn", out)
        self.assertIn("wrote 18 values", self.tool("--apply"))
        got = self.masks()
        want = {
            2: 32767, 3: 2048,                      # the same item here, a copy's name aside
            10: 13,                                 # Sandstorm's record before Allakhazam's page (Warrior)
            11: 16,                                 # Sandstorm's is a bracelet: Allakhazam's Arms page
            12: 896,                                # Sandstorm's two records: the one of its level
            13: 32767,                              # neither of its level: the rule (jewellery)
            20: 17407,                              # the leather wearers before the alchemist: with it
            21: 32767, 22: 32767,                   # all classes; an empty line: the rule
            23: 15360, 30: 15360,                   # the casters' boots, and the gloves of their set
            31: 879,                                # the set's other pieces ten levels off: plate by AC, lighter
            40: 32767, 41: 525, 42: 879,            # cloth; plate; plate by its AC, a material lighter
            44: 111, 60: 32767,                     # a one-handed slashing weapon; a ring
        }
        for pid, mask in want.items():
            self.assertEqual(got[pid][0], mask, f"pattern {pid}")
        for pid, why in [(43, "style 0"), (45, "a fishing pole"), (50, "a GM's"), (70, "not worn")]:
            self.assertEqual(got[pid][0], 0, f"left: {why}")
        self.assertEqual(got[60][1], 1023, "a race mask of 0: every race, with its class mask")
        self.assertEqual(got[80][0], 879, "a mask of its own is not touched")
        db = sqlite3.connect(self.path)
        log = dict(db.execute("SELECT source, count(*) FROM item_classmask_fix_log GROUP BY source").fetchall())
        db.close()
        self.assertEqual(log, {"sandstorm": 2, "allakhazam": 4, "same item": 2, "set": 1, "rule": 8, "race": 1})
        self.assertIn("wrote 0 values", self.tool("--apply"), "a second apply finds nothing")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET classuse = 5 WHERE patternid = 21")  # a GM's change since
        db.commit()
        db.close()
        self.assertIn("put back 17; left 1 changed since", self.tool("--undo"))
        self.assertEqual(self.masks()[21][0], 5, "a change since is kept")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET classuse = 0 WHERE patternid = 21")
        db.commit()
        db.close()
        self.assertEqual(dump(self.path), self.before, "undo puts every other row back, and drops its log")

    def test_choices(self):
        self.tool("--apply", "--no-rule")
        got = self.masks()
        for pid in (13, 22, 31, 40, 41, 42, 44, 60):
            self.assertEqual(got[pid][0], 0, f"--no-rule leaves {pid}")
        self.assertEqual(got[60][1], 0, "and its race mask")
        self.assertEqual(got[30][0], 15360, "a set is a record's, not the rule")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before)
        for choice, mask in (("material", 525), ("every", 32767), ("none", 0)):
            self.tool("--apply", "--ac-armour", choice)
            got = self.masks()
            self.assertEqual(got[42][0], mask, f"--ac-armour {choice}")
            self.assertEqual(got[41][0], 525, "a look its AC did not choose keeps its material")
            self.tool("--undo")
            self.assertEqual(dump(self.path), self.before)

    def test_plan_file(self):
        planned = os.path.join(self.dir, "plan.json")
        self.assertIn("plan written", self.tool("--save-plan", planned))
        self.tool("--apply")
        direct = self.masks()
        self.tool("--undo")
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('itempattern', ?)", (json.dumps({"patternid": 44}),))
        db.execute("UPDATE itempattern SET classuse = 3 WHERE patternid = 40")   # changed since the plan
        db.commit()
        db.close()
        out = subprocess.run([sys.executable, os.path.join(HERE, "fix_item_classmasks.py"), "--db", self.path,
                              "--plan", planned, "--apply"], capture_output=True, text=True, check=True).stdout
        self.assertIn("16 changes still as planned", out)
        self.assertIn("2 left: changed since or a GM's", out)
        got = self.masks()
        for pid, (mask, race) in direct.items():
            if pid == 40:
                self.assertEqual(got[pid][0], 3, "changed since the plan: left")
            elif pid == 44:
                self.assertEqual(got[pid][0], 0, "a GM's edit since the plan: left")
            else:
                self.assertEqual(got[pid], (mask, race), f"pattern {pid} as a plan of its own would have it")


if __name__ == "__main__":
    unittest.main()
