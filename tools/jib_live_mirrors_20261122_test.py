#!/usr/bin/env python3
"""Tests of tools/jib_live_mirrors_20261122.py on small made-up databases (no game data)."""
import json
import os
import sqlite3
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jib_live_mirrors_20261122 as T  # noqa: E402

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL,
    itemcost INTEGER NOT NULL DEFAULT 0, raceuse INTEGER NOT NULL DEFAULT 1023, maxhp INTEGER NOT NULL DEFAULT 0);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', giver TEXT NOT NULL DEFAULT '',
    decline TEXT NOT NULL DEFAULT '', declined TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', npcid INTEGER NOT NULL DEFAULT 0, items TEXT NOT NULL DEFAULT '',
    choices TEXT NOT NULL DEFAULT '', x REAL NOT NULL DEFAULT 0, PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
    PRIMARY KEY (questid, step, target));
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, zone INTEGER, npc_level INTEGER,
    x_coord REAL, z_coord REAL);
CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL);
CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
    count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, questid));
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL,
    stackleft INTEGER NOT NULL, remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL, location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


def mk():
    db = sqlite3.connect(":memory:", isolation_level=None)
    db.executescript(SCHEMA)
    return db


def haudit(src, tbl, key, before, after, action="update"):
    src.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after) "
                "VALUES ('t', 1, 'jib', 'x', '', ?, ?, ?, ?, ?)",
                (action, tbl, T.dumps(key), None if before is None else T.dumps(before),
                 None if after is None else T.dumps(after)))


class Base(unittest.TestCase):
    def setUp(self):
        T.RENAMES.clear()
        self.db, self.src = mk(), mk()
        for d in (self.db, self.src):
            d.execute("INSERT INTO itempattern VALUES (500, 'old name', 'old text', 10, 1023, 100)")
            d.execute("INSERT INTO quests VALUES (1, 'Old', 'Giver', 'Not now.', '')")
            d.execute("INSERT INTO queststeps (questid, step, kind, target, items) VALUES (1, 1, 'talk', 'Giver', '')")
            d.execute("INSERT INTO queststeps (questid, step, kind, target, items) VALUES (1, 2, 'loot', 'rat', '500')")
            d.execute("INSERT INTO queststeptargets VALUES (1, 2, 'rat'), (1, 2, 'bat')")
            d.execute("INSERT INTO npcs VALUES (10, 'Giver', 0, 1, 5, 1.0, 2.0)")
            d.execute("INSERT INTO npcclass VALUES ('giver', 3, 'seed')")
        # Heirloom's edit, audited: item 500 renamed, the quest rebuilt to one step
        self.src.execute("UPDATE itempattern SET itemname = 'new name', itemdesc = 'new text' WHERE patternid = 500")
        haudit(self.src, "itempattern", {"patternid": 500},
               {"patternid": 500, "itemname": "old name", "itemdesc": "old text"}, {})
        self.src.execute("INSERT INTO itempattern VALUES (900, 'New Item', 'made', 0, 1023, 50)")
        for st in (1, 2):
            b = T.plain(T.full(self.src, "queststeps", (1, st)))
            haudit(self.src, "queststeps", {"questid": 1, "step": st}, b, {}, "update" if st == 1 else "delete")
        self.src.execute("UPDATE queststeps SET kind = 'give', items = '900' WHERE questid = 1 AND step = 1")
        self.src.execute("DELETE FROM queststeps WHERE questid = 1 AND step = 2")
        self.src.execute("DELETE FROM queststeptargets")
        self.src.execute("INSERT INTO queststeptargets VALUES (1, 1, 'giver')")
        self.src.execute("UPDATE quests SET name = 'New', decline = '' WHERE questid = 1")
        self.src.execute("INSERT INTO npcs VALUES (77, 'Copied', 0, 1, 9, 5.0, 6.0)")
        self.db.execute("BEGIN")

    def tearDown(self):
        self.db.close()
        self.src.close()

    def plan(self):
        return T.Plan(self.db, self.src, "x", 1)


class EditTest(Base):
    def test_any_old_is_heirlooms_before(self):
        p = self.plan()
        p.items({500: {"itemname": (T.ANY, T.HL), "itemdesc": (T.ANY, T.HL)}})
        self.assertEqual(p.refusals, [])
        self.assertEqual(p.ops[0][3]["itemname"], "new name")

    def test_any_old_refuses_a_live_value_of_its_own(self):
        self.db.execute("UPDATE itempattern SET itemdesc = 'live only' WHERE patternid = 500")
        p = self.plan()
        p.items({500: {"itemdesc": (T.ANY, T.HL)}})
        self.assertTrue(p.refusals and "neither" in p.refusals[0])

    def test_explicit_old_and_the_froglok_bit(self):
        p = self.plan()
        p.items({500: {"raceuse": (271, 8)}})
        self.assertTrue(p.refusals)
        self.db.execute("UPDATE itempattern SET raceuse = 1295 WHERE patternid = 500")
        p = self.plan()
        p.items({500: {"raceuse": (271, 8)}})
        self.assertEqual(p.refusals, [])

    def test_new_item_taken_refuses(self):
        self.db.execute("INSERT INTO itempattern VALUES (900, 'Other', 'x', 0, 1023, 0)")
        p = self.plan()
        p.new_items({900: 8})
        self.assertTrue(p.refusals and "taken" in p.refusals[0])

    def test_new_item_race_list(self):
        p = self.plan()
        p.new_items({900: 8})
        self.assertEqual(p.ops[0][3]["raceuse"], 8)


class QuestTest(Base):
    def test_copy_whole_and_targets(self):
        p = self.plan()
        p.new_items({900: None})
        p.quests([1])
        self.assertEqual(p.refusals, [])
        T.apply(self.db, p)
        self.assertEqual(self.db.execute("SELECT step, kind, items FROM queststeps WHERE questid = 1").fetchall(),
                         [(1, "give", "900")])
        self.assertEqual(self.db.execute("SELECT step, target FROM queststeptargets").fetchall(), [(1, "giver")])
        self.assertEqual(self.db.execute("SELECT name, decline FROM quests").fetchone(), ("New", ""))

    def test_live_steps_not_as_heirloom_had_them_refuse(self):
        self.db.execute("UPDATE queststeps SET items = '123' WHERE questid = 1 AND step = 2")
        p = self.plan()
        p.quests([1])
        self.assertTrue(p.refusals and "not as Heirloom had it before" in p.refusals[0])

    def test_override(self):
        self.src.execute("UPDATE queststeps SET choices = 'a 997854' WHERE questid = 1 AND step = 1")
        p = self.plan()
        p.quests([1], {(1, 1): {"choices": T.TOME, "npcid": 42}})
        T.apply(self.db, p)
        self.assertEqual(self.db.execute("SELECT choices, npcid FROM queststeps WHERE questid = 1").fetchone(),
                         ("a 981971", 42))


class CharTest(Base):
    def test_move_give_and_ask(self):
        self.db.execute("INSERT INTO charquests (serverid, questid, step, count) VALUES (5, 1, 2, 3), (6, 1, 1, 0)")
        p = self.plan()
        p.new_items({900: None})
        p.quests([1])
        p.chars([1], moves={1: {2: (1, [900])}}, keep=[(6, 1, 1)])
        self.assertEqual(p.refusals, [])
        T.apply(self.db, p)
        self.assertEqual(self.db.execute("SELECT step, count FROM charquests WHERE serverid = 5").fetchone(), (1, 0))
        self.assertEqual(self.db.execute("SELECT serverid, patternid, stackleft FROM charinventory").fetchall(),
                         [(5, 900, 1)])
        p = self.plan()
        p.chars([1], ask=(1,))
        self.assertTrue(p.refusals and "ask jibrish" in p.refusals[0])

    def test_kept_character_moved_on_refuses(self):
        self.db.execute("INSERT INTO charquests (serverid, questid, step) VALUES (6, 1, 2)")
        p = self.plan()
        p.chars([1], keep=[(6, 1, 1)])
        self.assertTrue(p.refusals and "read step 1" in p.refusals[0])


class ApplyUndoTest(Base):
    def test_round_trip(self):
        before = {t: self.db.execute(f"SELECT rowid, * FROM {t} ORDER BY rowid").fetchall()
                  for t in ("itempattern", "quests", "queststeps", "queststeptargets", "npcs", "npcclass")}
        p = self.plan()
        p.new_items({900: None})
        p.items({500: {"itemname": (T.ANY, T.HL)}})
        p.rename_npc(10, "Giver", "Taker")
        p.rename_key("npcclass", "giver", "taker")
        nid = p.copy_new("npcs", (77,))
        p.quests([1])
        self.assertEqual(p.refusals, [])
        n = T.apply(self.db, p)
        self.assertEqual(nid, 11)  # Live's own next id, not Heirloom's 77
        self.assertEqual(self.db.execute("SELECT count(*) FROM gm_audit WHERE username = 'dab'").fetchone()[0], n)
        self.assertEqual(self.db.execute("SELECT name FROM npcclass").fetchall(), [("taker",)])
        self.assertEqual(T.logged(self.db), {"x"})
        back, left = T.undo(self.db)
        self.assertEqual((back, left), (n, []))
        for t, rs in before.items():
            self.assertEqual(self.db.execute(f"SELECT rowid, * FROM {t} ORDER BY rowid").fetchall(), rs, t)
        self.assertFalse(T.table(self.db, T.LOG))
        self.assertEqual(self.db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()[0], 10)

    def test_undo_leaves_a_row_changed_since(self):
        p = self.plan()
        p.items({500: {"itemname": (T.ANY, T.HL)}})
        T.apply(self.db, p)
        self.db.execute("UPDATE itempattern SET itemname = 'a GM' WHERE patternid = 500")
        back, left = T.undo(self.db)
        self.assertEqual(back, 0)
        self.assertTrue(left)


class SETest(unittest.TestCase):
    def setUp(self):
        self.db = mk()
        self.db.execute("BEGIN")
        for tn, _, _ in T.SE_TABLES:
            self.db.execute("INSERT INTO loottables (tablename, patternid) VALUES (?, 1)", (tn,))
        for kind, ids in T.SE_IDS.items():
            for i in ids:
                self.db.execute("INSERT INTO npcs VALUES (?, ?, 3, 30, 50, 0, 0)", (i, kind))

    def tearDown(self):
        self.db.close()

    def test_binds_198(self):
        p = T.Plan(self.db, self.db, "257", 0)
        T.e257(p)
        self.assertEqual(p.refusals, [])
        self.assertEqual(len(p.ops), 198)
        self.assertEqual({o[3]["source"] for o in p.ops}, {T.SE_SOURCE})

    def test_stops(self):
        self.db.execute("INSERT INTO npcloot (npc_name, tablename) VALUES ('kobold augur', 'se rare')")
        self.db.execute("INSERT INTO npcloot (npc_name, tablename) VALUES ('#121098', 'other')")
        self.db.execute("UPDATE npcs SET npc_name = 'fire goblin' WHERE id = 120244")
        p = T.Plan(self.db, self.db, "257", 0)
        T.e257(p)
        text = "\n".join(p.refusals)
        self.assertIn("name binding", text)
        self.assertIn("#121098", text)
        self.assertIn("120244", text)


if __name__ == "__main__":
    unittest.main()
