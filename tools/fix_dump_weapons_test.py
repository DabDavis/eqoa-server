#!/usr/bin/env python3
"""tools/fix_dump_weapons.py against a made-up database, harvest and
Allakhazam folder: a weapon of no attack type takes Sandstorm's record, else
its Allakhazam page's type and slot (a two-hander moved to slot 15), else
what its name or description says, else its icon's commonest type (never the
stand-in's); one with no model takes its kind's; a bow its description's
range; the stand-in icon (on most weapons of no type, on no Sandstorm item)
gives way to the item's Sandstorm icon, else one of its kind, on a typed
weapon too; a potion is left; a second --apply is refused; --undo gives back
the table as it was. No game data.

    python3 tools/fix_dump_weapons_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_dump_weapons.py")
sys.path.insert(0, HERE)
import fix_dump_weapons as fwt  # noqa: E402


class Words(unittest.TestCase):
    def test_the_kinds(self):
        for name, want in (("Goblin Stave", 4), ("Combine Zweihander", 2), ("Steel Battleaxe", 1),
                           ("Bronze Greataxe", 2), ("Rusty Glaive", 2), ("Steel Spear", 5), ("Starry Kriss", 5),
                           ("Iron Mace", 3), ("Blackbolt Crossbow", 9), ("Farseer Bow", 7), ("Bamboozler", None)):
            got = fwt.by_words(name)
            self.assertEqual(got[0] if got else None, want, name)


class StandIns(unittest.TestCase):
    """A stand-in is an icon on 20 weapons or more of three kinds or more that
    no Sandstorm item carries; one kind's icon, or one Sandstorm carries, is not."""

    def test_found(self):
        db = sqlite3.connect(":memory:")
        db.execute("CREATE TABLE itempattern (itemname TEXT, itemicon INTEGER, attacktype INTEGER, "
                   "weapondamage INTEGER, equipslot INTEGER)")
        rows = [(f"Blade {i}", 1, 1, 10, 12) for i in range(8)] + [(f"Mace {i}", 1, 3, 10, 12) for i in range(8)] \
            + [(f"Dagger {i}", 1, 5, 10, 0) for i in range(8)]           # 24 of three kinds: a stand-in
        rows += [(f"Sword {i}", 2, 1, 10, 12) for i in range(30)]          # one kind's icon
        rows += [(f"Axe {i}", 3, 1, 10, 12) for i in range(10)] + [(f"Club {i}", 3, 3, 10, 12) for i in range(10)] \
            + [(f"Spear {i}", 3, 5, 10, 12) for i in range(10)]          # three kinds, but Sandstorm carries it
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)", rows)
        self.assertEqual(fwt.placeholder_icons(db, {3}), {1})
        db.close()


class FixWeaponTypes(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("""CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT,
            itemicon INTEGER, equipslot INTEGER, attacktype INTEGER, weapondamage INTEGER, model INTEGER, unk5 INTEGER,
            levelreq INTEGER DEFAULT 10)""")
        db.execute("CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY)")
        db.executemany("INSERT INTO itempattern (patternid, itemname, itemdesc, itemicon, equipslot, attacktype, "
                       "weapondamage, model, unk5) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", [
            # typed weapons: the kinds' models and the icons' types
            (1, "Iron Mace", "", 50, 12, 3, 20, 300, 0),
            (2, "Oaken Staff", "", 60, 15, 4, 30, 400, 0),
            (3, "Long Sword", "", 70, 12, 1, 25, 500, 0),
            # of no attack type
            (10, "Sold Club", "", 50, 12, 0, 20, 0, 0),            # Sandstorm sells it
            (11, "Ahlspiess of Virtue", "", 99, 12, 0, 262, 0, 0),  # Allakhazam: Two Hand Piercing
            (12, "Goblin Stave", "", 99, 12, 0, 40, 0, 0),          # its name: a two-hand staff
            (13, "Bamboozler", "This tiny dagger packs a big punch.", 99, 12, 0, 40, 0, 0),
            (14, "Ouch", "Add a real description.", 70, 12, 0, 40, 0, 0),   # its icon: a sword's
            (15, "Greatbow of Doom", "A great bow.\nRange: 45", 99, 12, 0, 40, 0, 0),
            (16, "Mace Potion", "", 50, 12, 0, 5, 0, 0),            # a potion's
            (17, "Crafted Shortbow", "", 99, 16, 7, 30, 0, 25),     # typed, the stand-in icon
            (18, "Runic Broadsword", "", 99, 12, 0, 0, 0, 0),       # no damage either: a sword by its name
            (19, "Ransacker's Thing", "", 99, 12, 0, 0, 0, 0),      # no damage, nothing says a weapon
            (23, "Murky Saber", "", 98, 0, 1, 40, 0, 0),            # typed, in no slot, on slot 0's stand-in
            (24, "Spiked Band", "", 98, 0, 0, 12, 0, 0),            # in slot 0 with damage, named as armour
        ])
        # three more swords near level 20, for the broadsword's damage
        db.executemany("INSERT INTO itempattern VALUES (?, ?, '', 71, 12, 1, ?, 500, 0, ?)",
                       [(20, "Iron Sword", 100, 19), (21, "Steel Sword", 120, 21), (22, "Fine Sword", 110, 20)])
        db.execute("UPDATE itempattern SET levelreq = 20 WHERE patternid = 18")
        db.execute("INSERT INTO itemeffect VALUES (16)")
        db.commit()
        db.close()
        self.harvest = os.path.join(self.dir.name, "h.json")
        with open(self.harvest, "w") as f:
            json.dump({"merchants": [{"wares": [
                {"name": "Sold Club", "attack_type": 3, "wear_slot": 14, "damage": 20},
                {"name": "Crafted Shortbow", "attack_type": 7, "wear_slot": 16, "damage": 30, "icon": 444}]}]}, f)
        self.pages = os.path.join(self.dir.name, "pages")
        os.makedirs(self.pages)
        with open(os.path.join(self.pages, "lists.json"), "w") as f:
            json.dump({"ahlspiess of virtue": [[1610, "Primary"]]}, f)
        with open(os.path.join(self.pages, "pages.json"), "w") as f:
            json.dump({"1610": {"name": "Ahlspiess of Virtue", "Slot": "Primary", "Weapon Type": "Two Hand Piercing"}}, f)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--pages", self.pages,
                            "--prices", self.harvest, "--placeholder", "99", "--placeholder", "98", *args],
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
        out = self.run_tool().stdout
        self.assertIn("the dump's stand-in icons: 98, 99", out)
        self.assertIn("9 weapons to change: 7 given a type", out)
        self.assertIn("1 out of slot 0", out)
        self.assertIn("Spiked Band: in slot 0 and named as armour", out)
        self.assertIn("Ransacker's Thing: no damage and nothing says it is a weapon", out)
        self.run_tool("--apply")
        got = {r[0]: (r[4], r[5], r[7], r[8], r[3]) for r in self.rows()}   # slot, type, model, range, icon
        self.assertEqual(got[10], (14, 3, 300, 0, 50))   # Sandstorm's record; the blunt weapons' model
        self.assertEqual(got[11], (15, 6, 0, 0, 99))     # Allakhazam's two-hander; nothing of its type here
        self.assertEqual(got[12], (15, 4, 400, 0, 60))   # a stave: a two-hand staff, the staves' model and icon
        self.assertEqual(got[13][:2], (12, 5))           # the description's dagger
        self.assertEqual(got[14], (12, 1, 500, 0, 70))   # its icon's: a sword's
        self.assertEqual(got[15][:2], (16, 7))           # a bow, slot 16
        self.assertEqual(got[15][3:], (45, 444))         # its description's range; Sandstorm's bows' icon
        self.assertEqual(got[16], (12, 0, 0, 0, 50))     # a potion: left
        self.assertEqual(got[17], (16, 7, 0, 25, 444))   # typed: its own Sandstorm icon
        self.assertEqual(got[18], (12, 1, 500, 0, 71))   # a sword: the swords' model and icon
        dmg = {r[0]: r[6] for r in self.rows()}
        self.assertEqual(dmg[18], 110)                   # the median of the swords within 2 levels
        self.assertEqual(got[19], (12, 0, 0, 0, 99))     # left
        self.assertEqual(got[23], (12, 1, 500, 0, 71))   # its type's hand slot; the swords' model and icon
        self.assertEqual(got[24], (0, 0, 0, 0, 98))      # fix_wear_slots.py's
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.rows(), before)


if __name__ == "__main__":
    unittest.main()
