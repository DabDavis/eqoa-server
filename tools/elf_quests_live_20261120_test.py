#!/usr/bin/env python3
"""elf_quests_live_20261120.py on two small databases (its lists narrowed to them): the report writes nothing; --apply
makes a new item at the source's id, fixes only the listed columns, renames an import name, has the merchant sell the
item, copies a quest whole (a step removed, a step target moved), copies a quest's listed columns, points a quest back
at the original of a stand-in record (one held becomes its original, the record is removed), sets Trapmaw's body,
respawn and wander, moves characters (a loot step to a loot step keeps its count, another move counts from 0) and
gives a character on a give step the item it lacks; a gm_audit line a row; a second --apply writes nothing; --undo
puts every row back as it was (its rowid and the AUTOINCREMENT counters too); a row not as Live had it refuses,
nothing written; the source is never written."""
import hashlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import elf_quests_live_20261120 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, maxhp INTEGER, lore INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, notes TEXT);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT, rewarditems TEXT,
    events TEXT, choices TEXT, byclass TEXT, count INTEGER, tunar INTEGER, journal TEXT, chance REAL,
    PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT, PRIMARY KEY (questid, step, target));
CREATE TABLE quest_import_items (patternid INTEGER PRIMARY KEY, name TEXT, file TEXT);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, stackleft INTEGER, remainHP INTEGER,
    remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, modelid INTEGER);
CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL, note TEXT, source TEXT);
CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER, radius REAL, water TEXT, note TEXT, source TEXT, speed REAL);
CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, count INTEGER, done INTEGER,
    PRIMARY KEY (serverid, questid));
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, stackleft INTEGER,
    remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
    ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
INSERT INTO accountinfo VALUES (1, 'dab');
INSERT INTO npcs VALUES (10, 'Merchant Kelindira', 0), (20, 'Trapmaw', 111);
INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
    VALUES (10, 1, 0, 0, 8316, -1, 1, 0);
"""

# Live before the copy
LIVE = """
INSERT INTO itempattern VALUES (700001, 'Old Name', 'old desc', 0, 0), (700002, 'Two', 'two old', 0, 0),
    (700003, 'Old Three', 'three old', 0, 0), (976001, 'New Three', 'three new', 0, 1), (8316, 'strip', '', 0, 0);
INSERT INTO quest_import_items VALUES (700001, 'Old Name', 'f.json');
INSERT INTO quests VALUES (500001, 'Quest One', 'Giver', 'old notes'), (500002, 'Quest Two', 'Giver', 'n'),
    (500003, 'Quest Three', 'Giver', 'three notes');
INSERT INTO queststeps VALUES
    (500001, 1, 'talk', 'Merchant', '', '', '', '', '', 1, 0, 'speak', 0.0),
    (500001, 2, 'loot', 'rat', '4889', '', '', '', '', 2, 0, 'loot two', 0.5),
    (500001, 3, 'give', 'Giver', '4889', '700002', '', '', '', 1, 0, 'bring', 0.0),
    (500002, 1, 'loot', 'orc', '976001', '', '', '', '', 1, 0, 'key', 1.0),
    (500002, 2, 'give', 'Giver', '976001', '', '', '', '', 1, 0, 'give key', 0.0),
    (500003, 1, 'talk', 'Giver', '', '', '', '', '', 1, 0, 'old journal', 0.0);
INSERT INTO queststeptargets VALUES (500001, 2, 'mouse');
INSERT INTO npcrespawn VALUES (20, 90.0, 'old', 'petfix');
INSERT INTO charquests VALUES (1, 500001, 2, 1, 0), (2, 500001, 3, 0, 0), (3, 500002, 2, 0, 0), (4, 500001, 0, 0, 1);
INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
    VALUES (3, 1, 0, 0, 976001, -1, 0, 0), (2, 1, 0, 0, 4889, -1, 0, 0);
"""

# Heirloom after its entries
SOURCE = """
INSERT INTO itempattern VALUES (700001, 'New Name', 'new desc', 0, 1), (700002, 'Two', 'two new', 0, 0),
    (700003, 'New Three', 'three new', 0, 1), (900001, 'note', 'a note', 0, 1), (8316, 'strip', '', 0, 0);
INSERT INTO quest_import_items VALUES (700001, 'New Name', 'f.json');
INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
    VALUES (10, 1, 0, 0, 700001, -1, 1, 1);
INSERT INTO quests VALUES (500001, 'Quest One!', 'Giver', 'new notes'), (500002, 'Quest Two', 'Giver', 'n'),
    (500003, 'Quest Three', 'Giver', 'new three notes');
INSERT INTO queststeps VALUES
    (500001, 1, 'loot', 'rat', '4889*2', '', '', '', '', 2, 0, 'loot two', 0.5),
    (500001, 2, 'give', 'Giver', '4889*2, 900001', '700002', '', '', '', 1, 0, 'bring', 0.0),
    (500002, 1, 'loot', 'orc', '700003', '', '', '', '', 1, 0, 'key', 1.0),
    (500002, 2, 'give', 'Giver', '700003', '', '', '', '', 1, 0, 'give key', 0.0),
    (500003, 1, 'talk', 'Giver', '', '', '', '', '', 1, 0, 'new journal', 0.0);
INSERT INTO queststeptargets VALUES (500001, 1, 'mouse');
INSERT INTO npcrespawn VALUES (20, 120.0, 'Trapmaw: 120 s', 'petfix');
INSERT INTO npcmove VALUES ('trapmaw', 1, 120.0, '', 'wanders', 'board', 0.0);
UPDATE npcs SET modelid = 222 WHERE id = 20;
INSERT INTO gm_audit (id, at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES
    (100, 't', 9, 'gm', 'panel', '', 'update', 'itempattern', '{"patternid":700001}',
     '{"patternid":700001,"itemname":"Old Name","itemdesc":"old desc","maxhp":0,"lore":0}', '{}', ''),
    (101, 't', 9, 'gm', 'panel', '', 'update', 'itempattern', '{"patternid":700002}',
     '{"patternid":700002,"itemname":"Two","itemdesc":"two old","maxhp":0,"lore":0}', '{}', ''),
    (102, 't', 9, 'gm', 'panel', '', 'update', 'itempattern', '{"patternid":700003}',
     '{"patternid":700003,"itemname":"Old Three","itemdesc":"three old","maxhp":0,"lore":0}', '{}', ''),
    (103, 't', 9, 'gm', 'panel', '', 'update', 'quest_import_items', '{"patternid":700001}',
     '{"patternid":700001,"name":"Old Name","file":"f.json"}', '{}', ''),
    (104, 't', 9, 'gm', 'panel', '', 'update', 'npcs', '{"id":20}', '{"id":20,"npc_name":"Trapmaw","modelid":111}', '{}', ''),
    (105, 't', 9, 'gm', 'panel', '', 'update', 'npcrespawn', '{"npc_row":20}', '{"npc_row":20,"seconds":90}', '{}', '');
"""


def tables_digest(path):
    db = sqlite3.connect(path)
    h = hashlib.sha256()
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT IN ('gm_audit') ORDER BY name"):
        for r in db.execute(f"SELECT rowid, * FROM \"{t}\" ORDER BY rowid" if t != "sqlite_sequence" else
                            "SELECT * FROM sqlite_sequence WHERE name != 'gm_audit' ORDER BY name"):
            h.update(repr((t,) + tuple((type(v).__name__, v) for v in r)).encode())
    db.close()
    return h.hexdigest()


class ElfQuestsLive(unittest.TestCase):
    def setUp(self):
        d = tempfile.mkdtemp()
        self.live, self.src = os.path.join(d, "live.db"), os.path.join(d, "src.db")
        for path, data in ((self.live, LIVE), (self.src, SOURCE)):
            db = sqlite3.connect(path)
            db.executescript(SCHEMA + data)
            db.close()
        self.saved = {k: getattr(tool, k) for k in ("NEW_ITEMS", "FIXES", "IMPORT_NAMES", "STAND_INS", "WHOLE_QUESTS",
                                                    "COLUMN_COPIES", "LIVE_BEFORE", "STEP_MOVES", "GIVES",
                                                    "SOURCE_AUDITS", "MERCHANT_ITEM")}
        tool.NEW_ITEMS = [900001]
        tool.FIXES = {700001: "itemname itemdesc lore", 700002: "itemdesc", 700003: "itemname itemdesc lore"}
        tool.IMPORT_NAMES = (700001,)
        tool.STAND_INS = {976001: 700003}
        tool.WHOLE_QUESTS = [500001, 500002]
        tool.COLUMN_COPIES = {500003: (("notes",), {1: ("journal",)})}
        tool.STEP_MOVES = {500001: {1: 1, 2: 1, 3: 2}}
        tool.GIVES = {(500001, 2): 900001}
        tool.SOURCE_AUDITS = ((100, 200),)
        tool.MERCHANT_ITEM = 700001
        db = sqlite3.connect(self.live)
        tool.LIVE_BEFORE = {q: tool.quest_fingerprint(db, q) for q in (500001, 500002, 500003)}
        db.close()
        self.src_digest = tables_digest(self.src)

    def tearDown(self):
        for k, v in self.saved.items():
            setattr(tool, k, v)

    def run_tool(self, *args):
        out = io.StringIO()
        with redirect_stdout(out):
            rc = tool.main(["--db", self.live, "--source", self.src] + list(args))
        return rc, out.getvalue()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.live)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        before = tables_digest(self.live)
        rc, out = self.run_tool()
        self.assertEqual(rc, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(tables_digest(self.live), before)

        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT itemname, lore FROM itempattern WHERE patternid = 900001"), [("note", 1)])
        self.assertEqual(self.q("SELECT itemname, itemdesc, lore FROM itempattern WHERE patternid = 700001"),
                         [("New Name", "new desc", 1)])
        self.assertEqual(self.q("SELECT name FROM quest_import_items WHERE patternid = 700001"), [("New Name",)])
        self.assertEqual(self.q("SELECT patternid, listnumber, location FROM npcitem WHERE npcid = 10 ORDER BY listnumber"),
                         [(8316, 0, 1), (700001, 1, 1)])
        self.assertEqual(self.q("SELECT step, kind, items FROM queststeps WHERE questid = 500001 ORDER BY step"),
                         [(1, "loot", "4889*2"), (2, "give", "4889*2, 900001")])
        self.assertEqual(self.q("SELECT step, target FROM queststeptargets WHERE questid = 500001"), [(1, "mouse")])
        self.assertEqual(self.q("SELECT name, notes FROM quests WHERE questid = 500001"), [("Quest One!", "new notes")])
        self.assertEqual(self.q("SELECT name, notes FROM quests WHERE questid = 500003"),
                         [("Quest Three", "new three notes")])
        self.assertEqual(self.q("SELECT journal FROM queststeps WHERE questid = 500003"), [("new journal",)])
        self.assertEqual(self.q("SELECT items FROM queststeps WHERE questid = 500002 ORDER BY step"),
                         [("700003",), ("700003",)])
        # the stand-in: held becomes its original, the record removed
        self.assertEqual(self.q("SELECT patternid FROM charinventory WHERE serverid = 3"), [(700003,)])
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE patternid = 976001"), [(0,)])
        # Trapmaw
        self.assertEqual(self.q("SELECT modelid FROM npcs WHERE id = 20"), [(222,)])
        self.assertEqual(self.q("SELECT seconds, note FROM npcrespawn WHERE npc_row = 20"), [(120.0, "Trapmaw: 120 s")])
        self.assertEqual(self.q("SELECT name, roam, radius FROM npcmove"), [("trapmaw", 1, 120.0)])
        # characters: loot -> loot keeps its count; give -> give from 0 and the item given; finished left
        self.assertEqual(self.q("SELECT serverid, step, count FROM charquests WHERE questid = 500001 ORDER BY serverid"),
                         [(1, 1, 1), (2, 2, 0), (4, 0, 0)])
        self.assertEqual(self.q("SELECT patternid FROM charinventory WHERE serverid = 2 ORDER BY itemid"),
                         [(4889,), (900001,)])
        self.assertEqual(self.q("SELECT count(*) FROM charinventory WHERE serverid = 1 AND patternid = 900001"), [(0,)])
        logged = self.q(f"SELECT count(*) FROM {tool.LOG}")[0][0]
        audited = self.q("SELECT count(*) FROM gm_audit WHERE detail LIKE ?", ("%" + tool.TOOL + "%",))[0][0]
        self.assertEqual(logged, audited)
        self.assertGreater(logged, 15)
        for (b, a) in self.q("SELECT before, after FROM gm_audit WHERE detail LIKE ?", ("%" + tool.TOOL + "%",)):
            for side in (b, a):
                self.assertNotIn("_rowid_", side or "")

        applied = tables_digest(self.live)
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to do", out)
        self.assertEqual(tables_digest(self.live), applied)

        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(tables_digest(self.live), before)
        self.assertEqual(self.q(f"SELECT count(*) FROM sqlite_master WHERE name LIKE '{tool.LOG}%'"), [(0,)])
        self.assertEqual(tables_digest(self.src), self.src_digest)

    def test_refuses_an_item_not_as_before(self):
        db = sqlite3.connect(self.live)
        db.execute("UPDATE itempattern SET itemdesc = 'a GM wrote this' WHERE patternid = 700002")
        db.commit()
        db.close()
        before = tables_digest(self.live)
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)
        self.assertIn("700002", out)
        self.assertEqual(tables_digest(self.live), before)

    def test_refuses_a_quest_not_as_before(self):
        db = sqlite3.connect(self.live)
        db.execute("UPDATE queststeps SET journal = 'edited' WHERE questid = 500001 AND step = 1")
        db.commit()
        db.close()
        before = tables_digest(self.live)
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 1)
        self.assertIn("quest 500001", out)
        self.assertEqual(tables_digest(self.live), before)

    def test_refuses_a_taken_id(self):
        db = sqlite3.connect(self.live)
        db.execute("INSERT INTO itempattern VALUES (900001, 'something else', '', 0, 0)")
        db.commit()
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 1)
        self.assertIn("itempattern 900001: taken", out)

    def test_refuses_a_step_outside_the_moves(self):
        db = sqlite3.connect(self.live)
        db.execute("INSERT INTO queststeps VALUES (500001, 4, 'talk', 'Giver', '', '', '', '', '', 1, 0, 'x', 0.0)")
        db.execute("INSERT INTO charquests VALUES (5, 500001, 4, 0, 0)")
        db.commit()
        db.close()
        tool.LIVE_BEFORE[500001] = tool.quest_fingerprint(sqlite3.connect(self.live), 500001)
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 1)
        self.assertIn("character 5 on 500001 step 4", out)

    def test_a_held_item_is_not_given_again(self):
        db = sqlite3.connect(self.live)
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, "
                   "location, listnumber) VALUES (2, 1, 0, 0, 900001, -1, 1, 0)")
        db.commit()
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("holds 900001 already", out)
        self.assertEqual(self.q("SELECT count(*) FROM charinventory WHERE serverid = 2 AND patternid = 900001"), [(1,)])

    def test_the_lists_are_the_entries(self):
        self.assertEqual(len(self.saved["NEW_ITEMS"]), 24)
        self.assertEqual(len(self.saved["FIXES"]), 33)
        self.assertEqual(len(self.saved["WHOLE_QUESTS"]), 35)
        self.assertEqual(set(self.saved["LIVE_BEFORE"]),
                         {100712, 100713, 100716, 100721, 100059, 100060, 100335, 100336} | set(range(100722, 100747)))
        for (q, s), item in self.saved["GIVES"].items():
            self.assertIn(q, set(self.saved["WHOLE_QUESTS"]) | set(self.saved["COLUMN_COPIES"]))


if __name__ == "__main__":
    unittest.main()
