#!/usr/bin/env python3
"""tools/fix_scrolls.py on a made-up database: a scroll's level brought to
its spell's (its description's "Level N -" too), a scroll whose classes'
spells disagree listed, a dead scroll sold or held folded into the scroll
that teaches its spell (a merchant that sells both keeping one) or renamed
to its spell, the unclear ones listed; a second --apply writes nothing and
--undo puts the database back byte for byte. Every name and id here is made
up.

    python3 -m unittest tools/fix_scrolls_test.py
"""
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_scrolls as fs  # noqa: E402
import import_eqoaemu_spells as im  # noqa: E402

SK, DRD, SHA, MAG, WIZ = 3, 7, 8, 10, 13


def make(dir):
    path = os.path.join(dir, "t.db")
    db = sqlite3.connect(path, isolation_level=None)
    db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL DEFAULT 0, "
               "itemicon INTEGER NOT NULL DEFAULT 0, equipslot INTEGER NOT NULL DEFAULT -1, levelreq INTEGER NOT NULL, "
               "maxhp INTEGER NOT NULL DEFAULT 0, classuse INTEGER NOT NULL DEFAULT 32767, itemname TEXT NOT NULL, "
               "itemdesc TEXT NOT NULL DEFAULT '')")
    db.execute("CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT NOT NULL, abilitylvl INTEGER NOT NULL)")
    db.execute("CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'test', "
               "PRIMARY KEY (class, spellid))")
    db.execute("CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL, "
               "listnumber INTEGER NOT NULL DEFAULT 0)")
    db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, "
               "patternid INTEGER NOT NULL, remainHP INTEGER NOT NULL DEFAULT 0)")

    def spell(sid, name, level, *classes):
        db.execute("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, name, level))
        for c in classes:
            db.execute("INSERT INTO classspells (class, spellid) VALUES (?, ?)", (c, sid))

    def scroll(pid, name, level, classes, desc="", cost=0, icon=im.SCROLL_ICON):
        db.execute("INSERT INTO itempattern (patternid, itemcost, itemicon, levelreq, classuse, itemname, itemdesc) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?)", (pid, cost, icon, level, sum(1 << c for c in classes), name, desc))

    def sell(npc, *pids):
        for i, pid in enumerate(pids):
            db.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (?, ?, ?)", (npc, pid, i))

    # the price curve: one sold scroll a level
    for lv, cost in ((4, 24), (8, 97), (12, 160), (20, 369)):
        spell(900 + lv, f"Curve Bolt {lv}", lv, MAG)
        scroll(900 + lv, f"Curve Bolt {lv}", lv, [MAG], cost=cost)
        sell(9900, 900 + lv)

    # 1. a scroll of level 4 whose spell is of level 12, its description saying 4
    spell(1, "Shroud of Ash", 12, SK)
    scroll(101, "Shroud of Ash", 4, [SK], "Imbues your blows with ash. Level 4 - HP drain proc", 24)
    sell(9001, 101)
    # the control: a scroll at its spell's level
    spell(2, "Cinder Ward", 8, SK)
    scroll(102, "Cinder Ward", 8, [SK], "Level 8 - A ward", 97)
    # two classes, two records of different levels: listed, not written
    spell(3, "Mire Grip", 8, DRD)
    spell(4, "Mire Grip", 10, SHA)
    scroll(103, "Mire Grip", 4, [DRD, SHA], "Level 4 - Root")
    # two classes, one record: written
    spell(5, "Bog Clasp", 8, DRD, SHA)
    scroll(104, "Bog Clasp", 4, [DRD, SHA], "The feet stick. Level 4 - Root")

    # 2. a dead scroll sold and held whose spell a scroll teaches: folded
    spell(6, "Glimmer Wisp", 20, WIZ)
    scroll(110, "Glimmer Wisp", 20, [WIZ], "This scroll will allow you to learn Glimmer Wisp.", 369)
    scroll(111, "Summon Glimmer Wisp", 20, [WIZ], "Level 20 - WIZ Pet Summon", 369)
    sell(9002, 110, 111)       # sells both: its dead row goes
    sell(9003, 111)            # sells the dead one alone: its row moves
    db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (7001, 111)")
    # a dead scroll sold whose spell no scroll teaches: renamed, then its level the spell's
    spell(7, "Thornveil", 7, DRD)
    scroll(112, "thornveil scroll", 5, [DRD], "A scroll of Thornveil.")
    sell(9004, 112)
    # a dead scroll sold whose name means no spell exactly: listed with its best candidate
    spell(8, "Stonehide III", 12, DRD)
    scroll(113, "Stonehide 3", 12, [DRD], "Scribing this scroll hardens your skin.")
    sell(9005, 113)
    # a dead scroll for two classes whose spell a scroll teaches one of them only: listed
    spell(9, "Frostling", 20, MAG, WIZ)
    scroll(114, "Frostling", 20, [WIZ], "This scroll will allow you to learn Frostling.")
    scroll(115, "Summon Frostling", 20, [MAG, WIZ], "Level 20 - Pet Summon")
    sell(9006, 115)
    # a dead scroll neither sold nor held: left, listed
    spell(10, "Embercall", 3, DRD)
    scroll(116, "Embercall", 3, [DRD], "A scroll of Embercall.")
    scroll(117, "embercall scroll", 3, [DRD], "A scroll of Embercall.")
    # the scroll's art on a quest item: left
    scroll(118, "Mossy Charm Wrapping", 1, list(range(15)), "Wrapped around a charm by the river.")
    sell(9007, 118)
    db.close()
    return path


def dump(path):
    db = sqlite3.connect(path)
    try:
        return list(db.iterdump())
    finally:
        db.close()


class FixScrollsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = make(self.dir.name)
        self.db = sqlite3.connect(self.path, isolation_level=None)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def item(self, pid):
        return self.db.execute("SELECT itemname, levelreq, itemdesc FROM itempattern WHERE patternid = ?", (pid,)).fetchone()

    def test_plan(self):
        p = fs.plan(self.db)
        levels = {c["patternid"]: c["set"] for c in p["levels"]}
        self.assertEqual(levels[101], {"levelreq": 12, "itemdesc": "Imbues your blows with ash. Level 12 - HP drain proc"})
        self.assertEqual(levels[104], {"levelreq": 8, "itemdesc": "The feet stick. Level 8 - Root"})
        self.assertEqual(levels[112], {"levelreq": 7})   # the renamed one, at its spell's level
        self.assertNotIn(102, levels)                    # at its spell's level already
        self.assertNotIn(103, levels)                    # its classes' spells disagree
        self.assertIn("price stays 24 (the curve's 24 at 4, 160 at 12)", next(c["how"] for c in p["levels"] if c["patternid"] == 101))
        self.assertEqual([(c["patternid"], c["fold"]) for c in p["folds"]], [(111, 110)])
        self.assertEqual([(c["patternid"], c["set"]) for c in p["renames"]], [(112, {"itemname": "Thornveil"})])
        listed = {k: " | ".join(v) for k, v in p["lists"].items()}
        self.assertIn("103 'Mire Grip'", listed["a scroll whose classes' spells are of different levels (listed, not written)"])
        sold_unclear = listed["a dead scroll sold or held, its spell unclear (listed: the user's call)"]
        self.assertIn("113 'Stonehide 3'", sold_unclear)
        self.assertIn("8 'Stonehide III' L12 (its level", sold_unclear)
        self.assertIn("115 'Summon Frostling'",
                      listed["a dead scroll sold or held whose spell a scroll teaches some of its classes only (listed, not written)"])
        self.assertIn("117 'embercall scroll' -> 10 'Embercall' L3: would fold into 116",
                      listed["a dead scroll neither sold nor held whose spell is clear (left)"])
        self.assertIn("118 Mossy Charm Wrapping", listed["the scroll's art on a quest item (left)"])

    def test_apply_again_and_undo(self):
        before = dump(self.path)
        n_ren, n_lv, n_fold, moved, dropped, stale = fs.apply(self.db, fs.plan(self.db), "2026-01-01T00:00:00+00:00")
        self.assertEqual((n_ren, n_lv, n_fold, moved, dropped, stale), (1, 3, 1, 2, 1, 0))
        self.assertEqual(self.item(101), ("Shroud of Ash", 12, "Imbues your blows with ash. Level 12 - HP drain proc"))
        self.assertEqual(self.item(112), ("Thornveil", 7, "A scroll of Thornveil."))
        self.assertIsNone(self.item(111))
        self.assertEqual(self.item(103), ("Mire Grip", 4, "Level 4 - Root"))      # listed, left
        self.assertEqual(self.item(113), ("Stonehide 3", 12, "Scribing this scroll hardens your skin."))
        self.assertEqual(self.db.execute("SELECT npcid, patternid FROM npcitem WHERE npcid IN (9002, 9003) ORDER BY npcid, patternid")
                         .fetchall(), [(9002, 110), (9003, 110)])
        self.assertEqual(self.db.execute("SELECT patternid FROM charinventory WHERE serverid = 7001").fetchall(), [(110,)])
        # by the scribe's own match each written scroll now asks for its spell's level
        spells, own, _ = fs.load(self.db)
        for pid, sid in ((101, 1), (104, 5), (112, 7)):
            name, lv, _ = self.item(pid)
            classes = fs.classes_of(self.db.execute("SELECT classuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()[0])
            got = fs.scribes(spells, own, name, lv, classes)
            self.assertEqual(set(got.values()), {sid}, pid)
            self.assertEqual(lv, next(s[2] for s in spells if s[0] == sid), pid)
        once = dump(self.path)
        self.assertEqual(fs.apply(self.db, fs.plan(self.db), "2026-01-01T00:00:01+00:00"), (0, 0, 0, 0, 0, 0))
        self.assertEqual(dump(self.path), once)
        back, left, run = fs.fb.undo(self.db, fs.LOG)
        self.assertEqual((left, run), ([], "2026-01-01T00:00:00+00:00"))
        self.assertEqual(back, 1 + 3 + 3 + 1)   # renamed, levels, the fold's rows (2 + the dropped one) and record
        self.assertEqual(dump(self.path), before)

    def test_stale_row_left(self):
        p = fs.plan(self.db)
        self.db.execute("UPDATE itempattern SET levelreq = 9 WHERE patternid = 101")  # a GM's change since the plan
        n_ren, n_lv, n_fold, moved, dropped, stale = fs.apply(self.db, p, "2026-01-01T00:00:00+00:00")
        self.assertEqual((n_lv, stale), (2, 1))
        self.assertEqual(self.item(101)[1], 9)


if __name__ == "__main__":
    unittest.main()
