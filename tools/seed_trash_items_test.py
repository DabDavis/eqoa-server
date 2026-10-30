#!/usr/bin/env python3
"""tools/seed_trash_items.py against a made-up world: wolves get a pelt and a
fang named for the wolf at their level's grade and priced at their median
level times 2.40, a skeleton warrior a bone and dust, a guard on a human body
a human belt and ear, Old Red on a wolf's body the wolves' trash; an existing
trophy that already is the item is reused (and priced), a name a quest's item
holds takes another word. The controls: a creature with trash of its own, a
chest, a name with a number and a pet get none. --apply then --undo leaves the
database as it was; a second --apply is refused; an item someone holds is left
by --undo. No game data.

    python3 tools/seed_trash_items_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_trash_items.py")
sys.path.insert(0, HERE)
import seed_trash_items as st  # noqa: E402

COLS = ("patternid", "itemname", "itemcost", "itemicon", "equipslot", "trade", "rent", "levelreq", "maxstack", "maxhp",
        "duration", "classuse", "raceuse", "lore", "craft", "model", "color", "itemdesc")
SCHEMA = f"""
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, {', '.join(c + (' TEXT' if c in ('itemname', 'itemdesc') else ' INTEGER') for c in COLS[1:])});
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_type INTEGER, npc_level INTEGER, modelid INTEGER);
CREATE TABLE charactermodel (sex INTEGER NOT NULL, modelid INTEGER NOT NULL PRIMARY KEY, race INTEGER NOT NULL);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, items TEXT NOT NULL DEFAULT '');
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER,
    minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER);
CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT NOT NULL, region TEXT NOT NULL DEFAULT '*',
    npc_name TEXT NOT NULL DEFAULT '', patternid INTEGER NOT NULL, weight INTEGER NOT NULL DEFAULT 1,
    minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 100, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT NOT NULL DEFAULT '', profile TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
"""
WOLF, HUMAN, BONEY = 111, 222, 333
TROPHIES = [  # name, cost, icon, slot, trade, maxstack
    ("clump of rat fur", 10, 501, -1, 0, 99), ("bat fang", 7, 502, -1, 0, 99), ("old skull", 0, 503, -1, 0, 99),
    ("pixie dust", 5, 504, -1, 0, 99), ("raider belt", 9, 505, -1, 0, 99), ("crusty goblin eye", 0, 506, -1, 0, 99),
    ("skeleton bone", 0, 503, -1, 0, 99),     # already the trash: reused and priced
    ("chipped wolf fang", 0, 502, -1, 0, 99),  # a quest's: the wolves' low fang takes another word
]
NPCS = [  # name, type, level, model
    ("dire wolf", 0, 5, WOLF), ("dire wolf", 0, 7, WOLF), ("timber wolf", 0, 25, WOLF), ("Old Red", 0, 6, WOLF),
    ("skeleton warrior", 0, 20, BONEY), ("Guard Bob", 0, 35, HUMAN), ("rat", 0, 3, 444), ("coin chest", 0, 10, 555),
    ("re pc fire 1", 0, 1, 666), ("primal spirit", 0, 20, 777),
]


class SeedTrashItems(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        for name, cost, icon, slot, trade, ms in TROPHIES:
            db.execute("INSERT INTO itempattern (itemname, itemcost, itemicon, equipslot, trade, rent, levelreq, maxstack, maxhp, "
                       "duration, classuse, raceuse, lore, craft, model, color, itemdesc) "
                       "VALUES (?, ?, ?, ?, ?, 1, 0, ?, 0, 0, 0, 0, 0, 0, 0, -1, 'a trophy')", (name, cost, icon, slot, trade, ms))
        self.quest = db.execute("SELECT patternid FROM itempattern WHERE itemname = 'chipped wolf fang'").fetchone()[0]
        db.execute("INSERT INTO queststeps (questid, step, items) VALUES (1, 1, ?)", (str(self.quest),))
        db.executemany("INSERT INTO npcs (npc_name, npc_type, npc_level, modelid) VALUES (?, ?, ?, ?)", NPCS)
        db.execute("INSERT INTO charactermodel VALUES (0, ?, 0)", (HUMAN,))
        for k in ("dire wolf", "timber wolf", "old red", "skeleton warrior", "guard bob", "rat", "coin chest", "re pc fire 1"):
            db.execute("INSERT INTO lootcreature (npc_name, region) VALUES (?, 'plains')", (k,))
        db.execute("INSERT INTO lootcreature (npc_name, region) VALUES ('primal spirit', '-')")
        fur = db.execute("SELECT patternid FROM itempattern WHERE itemname = 'clump of rat fur'").fetchone()[0]
        db.execute("INSERT INTO lootpool (category, npc_name, patternid) VALUES ('trash', 'rat', ?)", (fur,))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def plan(self):
        db = sqlite3.connect(self.path)
        try:
            p = st.Plan(db)
            items, pool = p.build()
            return p, items, pool
        finally:
            db.close()

    def test_plan(self):
        p, items, pool = self.plan()
        self.assertEqual(set(p.kinds), {"dire wolf", "timber wolf", "old red", "skeleton warrior", "guard bob"},
                         "no rat (trash of its own), chest, numbered name or pet")
        got = {}
        for k, key in pool:
            got.setdefault(k, []).append(items[key]["name"])
        self.assertEqual(got["dire wolf"], ["torn wolf pelt", "worn wolf fang"], "the quest's chipped wolf fang is not repeated")
        self.assertEqual(got["timber wolf"], ["wolf pelt", "wolf fang"])
        self.assertEqual(got["old red"], ["torn wolf pelt", "worn wolf fang"], "its body's: a wolf")
        self.assertEqual(got["skeleton warrior"], ["skeleton bone", "pile of skeleton dust"])
        self.assertEqual(got["guard bob"], ["sturdy human belt", "intact human ear"], "a role on a human body")
        # the price: the median level of the creatures that drop it (5, 7, 6 for the low wolf pelt) x 2.40
        self.assertEqual(items["torn wolf pelt"]["price"], round(6 * 2.40))
        self.assertEqual(items["wolf pelt"]["price"], round(25 * 2.40))
        bone = items["skeleton bone"]
        self.assertIsNotNone(bone["pid"], "the existing skeleton bone is reused")
        self.assertEqual((bone["old"], bone["price"]), (0, round(20 * 2.40)))
        self.assertIsNone(items["torn wolf pelt"]["pid"], "made")

    def test_apply_undo(self):
        def state():
            db = sqlite3.connect(self.path)
            out = {t: sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
                   for t in ("itempattern", "lootpool", "charinventory")}
            out["schema"] = sorted(db.execute("SELECT name FROM sqlite_master WHERE name <> 'sqlite_sequence'").fetchall())
            db.close()
            return out

        run = lambda *a: subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a],
                                        capture_output=True, text=True)
        before = state()
        r = run("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.path)
        made = db.execute("SELECT itemicon, maxstack, equipslot, trade, itemcost, color FROM itempattern WHERE itemname = 'torn wolf pelt'").fetchone()
        self.assertEqual(made, (501, 99, -1, 0, 14, -1), "a fur trophy's icon and looks, stacking to 99, for vendors")
        self.assertEqual(db.execute("SELECT itemicon FROM itempattern WHERE itemname = 'intact human ear'").fetchone(), (506,),
                         "an ear wears an eye trophy's icon")
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE itemname = 'skeleton bone'").fetchone(), (48,))
        self.assertEqual(db.execute("SELECT count(*) FROM lootpool WHERE source = 'trash-seed'").fetchone(), (10,))
        db.close()
        self.assertNotEqual(run("--apply").returncode, 0, "a second --apply is refused")
        r = run("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertNotIn("left as they are", r.stdout)
        self.assertEqual(state(), before)

    def test_an_item_someone_holds_is_left(self):
        run = lambda *a: subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a],
                                        capture_output=True, text=True)
        run("--apply")
        db = sqlite3.connect(self.path)
        pid = db.execute("SELECT patternid FROM itempattern WHERE itemname = 'wolf pelt'").fetchone()[0]
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, ?)", (pid,))
        db.commit()
        db.close()
        r = run("--undo")
        self.assertIn("'wolf pelt'", r.stdout)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT count(*) FROM itempattern WHERE itemname = 'wolf pelt'").fetchone(), (1,))
        db.close()


if __name__ == "__main__":
    unittest.main()
