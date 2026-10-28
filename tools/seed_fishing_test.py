#!/usr/bin/env python3
"""tools/seed_fishing.py against a made-up database: the catches made with an
icon of their kind (and left out when there is none to give), those the
database has used as they are, the bait made wearable, the sellers stocked
(not one without the NPC bit, nor one with a catalog), the rows written,
--undo exact but for an item a character holds, and the fishcatch table the
same as the server's. No game data: the items and icons are made up.

    python3 tools/seed_fishing_test.py
"""
import os
import re
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_fishing as sf  # noqa: E402

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER, unk1 INTEGER,
    itemicon INTEGER, unk2 INTEGER, equipslot INTEGER, unk3 INTEGER, trade INTEGER, rent INTEGER,
    unk4 INTEGER, attacktype INTEGER, weapondamage INTEGER, unk5 INTEGER, levelreq INTEGER,
    maxstack INTEGER, maxhp INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER,
    procanim INTEGER, lore INTEGER, unk6 INTEGER, craft INTEGER, itemname TEXT, itemdesc TEXT,
    model INTEGER, color INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_type INTEGER, npc_id INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER,
    remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER,
    listnumber INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER);
"""


def item(db, name, icon, slot=-1, cost=0, cls=32767, race=1023, stack=1):
    cur = db.execute("""INSERT INTO itempattern (itemcost, itemicon, equipslot, classuse, raceuse, maxstack,
        itemname, itemdesc) VALUES (?, ?, ?, ?, ?, ?, ?, '')""", (cost, icon, slot, cls, race, stack, name))
    return cur.lastrowid


def dump(db):
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"):
        out[t] = sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
    return out


class SeedFishingTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        self.carp_icon, self.doll_icon = 111, 222
        item(db, "grass carp", self.carp_icon, stack=50)  # the fish's donor, a catch itself
        item(db, "a rag doll", self.doll_icon)
        item(db, "rat bones", 333)
        item(db, "locked box", 444)
        item(db, "flax bread", 555)
        item(db, "venom sack", 666)
        item(db, "a doll scroll", -607649328)  # a scroll is no donor
        self.pole = item(db, "Fishing Pole", 777, slot=20)
        self.bait = item(db, "can of worms", 888, slot=-1, cls=0, race=0, stack=99)
        item(db, "Salmon", 999, cls=0, race=0, stack=99)
        db.execute("INSERT INTO npcs VALUES (10, 'Pole Merchant Able', 128, 0)")    # no merchant flag yet
        db.execute("INSERT INTO npcs VALUES (11, 'Tacklemaster Baker', 129, 5011)")
        db.execute("INSERT INTO npcs VALUES (12, 'Pole Merchant Cane', 0, 0)")      # no NPC bit: no window
        db.execute("INSERT INTO npcs VALUES (13, 'Tacklemaster Dory', 129, 5013)")  # a GM's catalog
        db.execute("INSERT INTO npcs VALUES (14, 'Fisherman Eel', 128, 0)")         # sells nothing
        db.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (5013, 1, 0)")
        db.close()

    def db(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        self.addCleanup(db.close)
        return db

    def test_plan_makes_the_missing_with_an_icon_of_their_kind(self):
        db = self.db()
        make, have, rows, cols, sellers, missing = sf.plan(db)
        made = {c.name: d for c, d in make}
        self.assertEqual(made["Carp"][2], self.carp_icon)  # a fish takes the grass carp's
        self.assertEqual(made["Child's Doll"][1], "a rag doll")  # not the scroll
        self.assertEqual(made["Moldy Bone"][1], "rat bones")
        self.assertEqual(made["Waterlogged Lute"][1], "locked box")  # no lute here: a box
        self.assertEqual(made["Bank Jelly"][1], "venom sack")  # not a stomach (the grocers' bread tile)
        self.assertIn("grass carp", have)
        self.assertIn("Salmon", have)
        self.assertNotIn("grass carp", made)
        # "keep" catches the database lacks are left out, not made
        for name in ("Brittle Fishing Rod", "Spiced Jerky", "Weeping Hearts Pendant"):
            self.assertIn(name, missing)
        self.assertNotIn("Carp", missing)
        self.assertEqual(sorted((p, c) for p, c, _, _ in cols),
                         [(self.pole, "itemcost"), (self.bait, "classuse"), (self.bait, "equipslot"),
                          (self.bait, "itemcost"), (self.bait, "raceuse")])
        self.assertEqual([(s[1], s[4]) for s in sellers],
                         [("Pole Merchant Able", "Fishing Pole"), ("Tacklemaster Baker", "can of worms")])

    def test_no_donor_no_item(self):
        db = self.db()
        db.execute("DELETE FROM itempattern WHERE itemname IN ('a rag doll', 'a doll scroll')")
        make, _, rows, _, _, missing = sf.plan(db)
        self.assertNotIn("Child's Doll", [c.name for c, _ in make])
        self.assertIn("Child's Doll", missing)
        self.assertNotIn("Child's Doll", [n for n, _ in rows])

    def test_apply_then_undo(self):
        db = self.db()
        before = dump(db)
        self.assertEqual(sf.main(["--db", self.path, "--apply", "--no-backup"]), 0)
        db = self.db()
        bait = db.execute("SELECT equipslot, classuse, raceuse, itemcost FROM itempattern WHERE patternid = ?",
                          (self.bait,)).fetchone()
        self.assertEqual(bait, (21, 32767, 1023, 2))
        carp = db.execute("""SELECT itemicon, levelreq, maxstack, procanim, itemcost, itemdesc FROM itempattern
            WHERE itemname = 'Carp'""").fetchone()
        self.assertEqual(carp[:5], (self.carp_icon, 1, 50, 2, 2))
        self.assertTrue(re.search(r"\bmeal\b.*\bhealth regeneration\b", carp[5]), carp[5])
        doll = db.execute("SELECT procanim, maxstack FROM itempattern WHERE itemname = \"Child's Doll\"").fetchone()
        self.assertEqual(doll, (0, 1))  # junk is no meal
        # the bass in two boxes of Tunaria, the bream in one and on Odus
        bass = db.execute("""SELECT world, minx, minz, maxx, maxz FROM fishcatch f JOIN itempattern p
            ON p.patternid = f.itemid WHERE p.itemname = 'tunaria bass' ORDER BY minx""").fetchall()
        self.assertEqual(bass, [(0, 20000, 24000, 22000, 26000), (0, 24000, 26000, 26000, 28000)])
        bream = db.execute("""SELECT world, minx, minskill FROM fishcatch f JOIN itempattern p
            ON p.patternid = f.itemid WHERE p.itemname = 'Longfellow bream' ORDER BY world""").fetchall()
        self.assertEqual(bream, [(0, 24000, 50), (2, None, 50)])
        self.assertEqual(db.execute("SELECT count(*) FROM fishcatch WHERE source != 'seed'").fetchone()[0], 0)
        golden = db.execute("""SELECT months FROM fishcatch f JOIN itempattern p ON p.patternid = f.itemid
            WHERE p.itemname = 'Golden Perch'""").fetchone()[0]
        self.assertEqual([m + 1 for m in range(12) if golden >> m & 1], [4, 5, 6, 7, 8, 9])
        sellers = db.execute("SELECT id, npc_type, npc_id FROM npcs ORDER BY id").fetchall()
        self.assertEqual(sellers, [(10, 129, 10), (11, 129, 5011), (12, 0, 0), (13, 129, 5013), (14, 128, 0)])
        stock = db.execute("SELECT npcid, patternid, listnumber FROM npcitem ORDER BY itemid").fetchall()
        self.assertEqual(stock, [(5013, 1, 0), (10, self.pole, 0), (5011, self.bait, 0)])
        # a second --apply is refused
        with self.assertRaises(SystemExit):
            sf.main(["--db", self.path, "--apply", "--no-backup"])
        # undo: as before, but the carp a character took
        carp_id = db.execute("SELECT patternid FROM itempattern WHERE itemname = 'Carp'").fetchone()[0]
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, ?)", (carp_id,))
        self.assertEqual(sf.main(["--db", self.path, "--undo", "--no-backup"]), 0)
        db = self.db()
        after = dump(db)
        kept = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (carp_id,)).fetchone()
        self.assertEqual(kept, ("Carp",))
        db.execute("DELETE FROM charinventory")
        db.execute("DELETE FROM itempattern WHERE patternid = ?", (carp_id,))
        after = dump(db)
        before.pop("charinventory")
        after.pop("charinventory")
        self.assertEqual(after.pop("fishcatch"), [])  # the table stays: the server's
        self.assertEqual(before, after)

    def test_the_table_is_the_servers(self):
        here = os.path.dirname(os.path.abspath(__file__))
        go = open(os.path.join(here, "..", "internal", "database", "fishing.go")).read()
        server = re.search(r"CREATE TABLE IF NOT EXISTS fishcatch \((.*?)\);", go, re.S).group(1)
        db = sqlite3.connect(":memory:")
        sf.ensure_fishcatch(db)
        ours = [r[1:] for r in db.execute("PRAGMA table_info(fishcatch)")]
        db2 = sqlite3.connect(":memory:")
        db2.execute(f"CREATE TABLE fishcatch ({server})")
        theirs = [r[1:] for r in db2.execute("PRAGMA table_info(fishcatch)")]
        self.assertEqual(ours, theirs)


if __name__ == "__main__":
    unittest.main()
