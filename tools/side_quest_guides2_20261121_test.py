#!/usr/bin/env python3
"""side_quest_guides2_20261121.py: every part, a second apply, undo exact, the refusals."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import side_quest_guides2_20261121 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL,
    world INTEGER DEFAULT 0, npc_level INTEGER, npc_type INTEGER DEFAULT 0, npc_id INTEGER DEFAULT 0, modelid INTEGER DEFAULT 1,
    size REAL DEFAULT 1.5);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT DEFAULT '', source TEXT DEFAULT 'gm');
CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, count INTEGER DEFAULT 0, done INTEGER DEFAULT 0,
    PRIMARY KEY (serverid, questid));
CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER, source TEXT DEFAULT 'gm');
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT, note TEXT DEFAULT '', source TEXT DEFAULT 'gm');
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, world INTEGER DEFAULT -1, x REAL DEFAULT 0,
    z REAL DEFAULT 0, radius REAL DEFAULT 0, faction TEXT, note TEXT DEFAULT '', source TEXT DEFAULT 'gm');
CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT, profile TEXT, source TEXT);
CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT, region TEXT, npc_name TEXT, patternid INTEGER,
    weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, faction TEXT, delta INTEGER, source TEXT,
    UNIQUE (npc_name, faction));
CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER DEFAULT -1, radius REAL DEFAULT 0, water TEXT DEFAULT '',
    note TEXT DEFAULT '', source TEXT DEFAULT 'gm', speed REAL DEFAULT 0);
CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL, note TEXT DEFAULT '', source TEXT DEFAULT 'gm');
CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, line TEXT);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, source TEXT, notes TEXT DEFAULT '',
    offer TEXT DEFAULT '', accept TEXT DEFAULT '', decline TEXT DEFAULT '', declined TEXT DEFAULT '',
    started TEXT DEFAULT '', events TEXT DEFAULT '', factionreq TEXT DEFAULT '');
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, npcid INTEGER DEFAULT 0,
    items TEXT DEFAULT '', tunar INTEGER DEFAULT 0, journal TEXT DEFAULT '', answer TEXT DEFAULT '', says TEXT DEFAULT '',
    waiting TEXT DEFAULT '', notyet TEXT DEFAULT '', reminder TEXT DEFAULT '', rewarditems TEXT DEFAULT '',
    events TEXT DEFAULT '', chance REAL DEFAULT 0, byclass TEXT DEFAULT '', count INTEGER DEFAULT 1, world INTEGER DEFAULT 0,
    x REAL DEFAULT 0, y REAL DEFAULT 0, z REAL DEFAULT 0, radius REAL DEFAULT 15, xp INTEGER DEFAULT 0,
    PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT DEFAULT '', levelreq INTEGER,
    classuse INTEGER, equipslot INTEGER DEFAULT 1, AC INTEGER DEFAULT 0, itemicon INTEGER DEFAULT 5, itemcost INTEGER DEFAULT 100,
    maxhp INTEGER DEFAULT 4600, PR INTEGER DEFAULT 10, AR INTEGER DEFAULT 9);
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER);
CREATE TABLE npcheld (npc_name TEXT, hand INTEGER, patternid INTEGER, note TEXT, source TEXT);
CREATE TABLE gmtriggers (id INTEGER PRIMARY KEY, name TEXT, steps TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT, action TEXT,
    tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""

SHAMAN, CLERIC = 1 << 8, 1 << 9


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for old, _ in tool.RENAMES:
        db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, npc_level) VALUES (?, 1, 1, 1, 30)", (old,))
        db.execute("INSERT INTO npcclass (name, class) VALUES (?, 0)", (old.lower(),))
    db.execute("INSERT INTO factionhits (npc_name, faction, delta) VALUES ('Feria Hurkert', 'Feria Hurkert', -10)")
    db.execute("INSERT INTO npcdialog (name, line) VALUES ('Jimbo Hurkert', 'Mother is Feria Hurkert.')")
    # the fort's bandits, the red-haired one at level 25; the Blacktongue camp
    for i, (x, lv) in enumerate([(17653.2, 25), (17700, 22), (17653.0, 22)]):  # rows 15-17
        db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, npc_level, npc_id) VALUES ('arctic bandit', ?, 54, 5593.3, ?, 5)", (x, lv))
    db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, npc_level) VALUES ('Blacktongue orc', 19294, 54, 16930, 40)")
    db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, npc_level) VALUES ('Tsekar Kelriss', 22243, 13, 10718, 31)")
    db.execute("INSERT INTO npcclass (name, class) VALUES ('arctic bandit', 0)")
    db.execute("INSERT INTO lootcreature VALUES ('arctic bandit', 'arctic', 'p', 's')")
    db.execute("INSERT INTO npctemplate (name, template) VALUES ('Blacktongue orc', 'roamer')")
    db.execute("INSERT INTO factionhits (npc_name, faction, delta) VALUES ('Blacktongue orc', 'Blacktongue Orcs', -10)")
    items = [(6345, "Featherweave Coif", 256), (6608, "Featherweave Tunic", 256), (6886, "Featherweave Gloves", 256),
             (6357, "Goldlight Coif", 512), (6147, "Goldlight Shield", 512), (7529, "Goldlight Boots", 512)]
    for pid, name, cu in items:
        db.execute("INSERT INTO itempattern (patternid, itemname, levelreq, classuse, AC) VALUES (?, ?, 23, ?, 40)", (pid, name, cu))
    db.execute("INSERT INTO itempattern (patternid, itemname, itemdesc, levelreq, classuse) VALUES "
               "(700739, 'Redheaded Bandit''s Bandana', '', 23, 32767), (700797, 'Ravager''s Lantern', '', 0, 0), "
               "(700740, 'Runic Recording', '', 1, 32767), "
               "(700775, 'Phoborium''s Horn', 'A horn from Phoborium.', 1, 0), "
               "(701545, 'Songbook', 'The songs of the bard Vedel Revnold.', 1, 0)")
    q = [(100759, "The Redheaded Bandit", "Feria Hurkert", "EQOA-Data 23-47/23/p1"),
         (100760, "The Glowing Shield", "Feria Hurkert", "EQOA-Data 23-47/23/p2"),
         (100761, "The Remains of Manger MacGray", "Feria Hurkert", "EQOA-Data 23-47/23/p3"),
         (100766, "Saerk's Ring", "Saerk", "EQOA-Data 23-47/27/p2"),
         (100776, "The Seal of Deiphobus", "Ambrose Castor", "EQOA-Data 23-47/35/ambrose"),
         (100782, "Poisontongue", "Rigo Dartar", "EQOA-Data 23-47/37/rigo"),
         (101383, "Songs", "Enrique Revnold", "EQOA-Data x")]
    for qid, name, giver, src in q:
        db.execute("INSERT INTO quests (questid, name, giver, source, offer) VALUES (?, ?, ?, ?, ?)",
                   (qid, name, giver, src + " (tools/import_quests.py)", "Speak to Feria Hurkert, and to Vedel Revnold."))
    st = lambda *a: db.execute("INSERT INTO queststeps (questid, step, kind, target, items, chance, journal, byclass, tunar) "
                               "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", a)
    st(100759, 2, "loot", "arctic bandit", "700739", 0.25, "Kill arctic bandits.", "", 0)
    st(100759, 4, "talk", "Drew Hurkert", "", 0, "Speak with Drew Hurkert.", json.dumps({"8": {"items": "6345", "spells": "1"}, "9": {"items": "6357"}}), 0)
    st(100760, 5, "give", "Feria Hurkert", "", 0, "Bring it to Feria Hurkert.", json.dumps({"8": {"items": "6608"}, "9": {"items": "6147"}}), 0)
    st(100761, 5, "give", "frosty chest", "", 0, "Open it.", json.dumps({"8": {"items": "6886"}, "9": {"items": "7529"}}), 0)
    st(100766, 3, "give", "Grithmil Darkstone", "", 0, "Buy a red diamond.", "", 1450)
    st(100776, 1, "loot", "Phoborium", "700775", 1, "Kill Phoborium; bring Phoborium's horn.", "", 0)
    st(100782, 8, "loot", "Blacktongue orc", "700797", 0.25, "Kill Blacktongue orcs.", "", 0)
    st(101383, 1, "talk", "Enrique Revnold", "", 0, "Speak with Enrique Revnold about Vedel Revnold.", "", 0)
    db.execute("UPDATE itempattern SET itemicon = -607649328 WHERE patternid = 700740")
    for n in range(9, 14):  # the Ravager's quest goes on past step 8
        st(100782, n, "talk", "Elmo Hagger" if n == 9 else "Rigo Dartar", "", 0, f"step {n}", "", 0)
    db.execute("INSERT INTO charquests (serverid, questid, step) VALUES (7, 100782, 9), (8, 100782, 3)")
    db.execute("INSERT INTO charquests (serverid, questid, step, done) VALUES (9, 100782, 0, 1)")
    db.commit()
    db.close()


def snapshot(path):
    db = sqlite3.connect(path)
    try:
        out = {}
        for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT IN ('gm_audit', ?)", (tool.LOG,)):
            out[t] = sorted(map(repr, db.execute(f'SELECT rowid, * FROM "{t}"').fetchall()))
        return out
    finally:
        db.close()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def run_tool(self, *a):
        return tool.main(["--db", self.path] + list(a))

    def test_dry_run_writes_nothing(self):
        before = snapshot(self.path)
        self.assertEqual(self.run_tool(), 0)
        self.assertEqual(snapshot(self.path), before)

    def test_apply_again_undo_apply(self):
        before = snapshot(self.path)
        self.assertEqual(self.run_tool("--apply"), 0)
        names = {n for (n,) in self.q("SELECT npc_name FROM npcs")}
        for old, new in tool.RENAMES:
            self.assertIn(new, names)
            self.assertNotIn(old, names)
        self.assertEqual(self.q("SELECT name FROM npcclass WHERE name = 'feria huckert'"), [("feria huckert",)])
        self.assertEqual(self.q("SELECT faction FROM factionhits WHERE npc_name = 'Feria Huckert'"), [("Feria Hurkert",)])
        self.assertEqual(self.q("SELECT line FROM npcdialog"), [("Mother is Feria Huckert.",)])
        self.assertEqual(self.q("SELECT giver FROM quests WHERE questid = 100759"), [("Feria Huckert",)])
        self.assertEqual(self.q("SELECT itemname, itemdesc FROM itempattern WHERE patternid = 700775"),
                         [("Phorriun's Horn", "A horn from Phorriun.")])
        # another man's surname is left (Vedel Revnold)
        self.assertEqual(self.q("SELECT journal FROM queststeps WHERE questid = 101383"),
                         [("Speak with Enrique Reynold about Vedel Revnold.",)])
        self.assertEqual(self.q("SELECT itemdesc FROM itempattern WHERE patternid = 701545"),
                         [("The songs of the bard Vedel Revnold.",)])
        # #975: the red-haired arctic bandit, his row's alone, the note
        rb = self.q("SELECT id, npc_name, npc_level FROM npcs WHERE x_coord = 17653.2")
        self.assertEqual([r[1:] for r in rb], [("arctic bandit", 26)])
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE npc_name = 'Redheaded Bandit'"), [(0,)])
        self.assertEqual(self.q("SELECT seconds FROM npcrespawn WHERE npc_row = ?", (rb[0][0],)), [(600.0,)])
        self.assertEqual(self.q("SELECT target, npcid, chance FROM queststeps WHERE questid = 100759 AND step = 2"),
                         [("arctic bandit", rb[0][0], 1.0)])
        self.assertEqual(self.q("SELECT itemname, equipslot, levelreq, itemcost, maxhp, PR, AR, itemicon FROM itempattern "
                                "WHERE patternid = 700739"), [("Scribbled Note", -1, 0, 0, 0, 0, 0, -607649328)])
        # #931: the Ravager out of the world, raised by step 8, his step 9; the steps and the character on 9 moved
        rv = self.q("SELECT npc_level, x_coord, z_coord, modelid, size, npc_type FROM npcs WHERE npc_name = 'Blacktongue Ravager'")
        self.assertEqual(rv, [(37, 19330.0, 16930.0, 939332601, 1.0, 0)])
        self.assertEqual(self.q("SELECT name FROM npcspawnonly"), [("blacktongue ravager",)])
        steps = self.q("SELECT step, kind, target, chance, events FROM queststeps WHERE questid = 100782 ORDER BY step")
        self.assertEqual([s[:4] for s in steps[:3]], [(8, "reach", "", 0.0), (9, "loot", "Blacktongue Ravager", 1.0),
                                                      (10, "talk", "Elmo Haggar", 0.0)])
        self.assertEqual(json.loads(steps[0][4]), [{"on": "done", "do": "spawn", "npc": "Blacktongue Ravager", "hostile": True}])
        self.assertEqual(steps[-1][0], 14)
        self.assertEqual(self.q("SELECT serverid, step FROM charquests WHERE questid = 100782 ORDER BY serverid"),
                         [(7, 10), (8, 3), (9, 0)])
        # featherlite and the cleric's swap, both on the same steps
        bc = {qid: json.loads(b) for qid, b in self.q("SELECT questid, byclass FROM queststeps WHERE byclass <> ''")}
        fl = dict(self.q("SELECT itemname, patternid FROM itempattern WHERE itemname LIKE 'Featherlite%'"))
        self.assertEqual(sorted(fl.values()), [721000, 721001, 721002])
        self.assertEqual((bc[100759]["8"]["items"], bc[100759]["8"]["spells"]), (str(fl["Featherlite Helm"]), "1"))
        self.assertEqual(bc[100760]["8"]["items"], str(fl["Featherlite Tunic"]))
        self.assertEqual(bc[100761]["8"]["items"], str(fl["Featherlite Gloves"]))
        self.assertEqual((bc[100759]["9"]["items"], bc[100760]["9"]["items"], bc[100761]["9"]["items"]), ("6357", "7529", "6147"))
        self.assertEqual(self.q("SELECT AC, equipslot FROM itempattern WHERE itemname = 'Featherlite Tunic'"), [(40, 1)])
        # Tsekar
        self.assertEqual(self.q("SELECT npc_type FROM npcs WHERE npc_name = 'Tsekar Kelriss'"), [(128,)])
        self.assertEqual(self.q("SELECT target FROM queststeptargets"), [("Tsekar Kelriss",)])
        self.assertEqual(self.q("SELECT tunar, says FROM queststeps WHERE questid = 100766"),
                         [(1500, "A red diamond. Take it to Saerk.")])
        self.assertIn("Tsekar Kelriss in the Green Rift (1,500 tunar)",
                      self.q("SELECT journal FROM queststeps WHERE questid = 100766")[0][0])
        # the bandit's journal went through the renames
        self.assertEqual(self.q("SELECT journal FROM queststeps WHERE questid = 100759 AND step = 2")[0][0][:44],
                         "Kill the red-haired arctic bandit who wander")
        after = snapshot(self.path)
        audits = self.q("SELECT count(*) FROM gm_audit")[0][0]
        self.assertEqual(self.run_tool("--apply"), 0)
        self.assertEqual(snapshot(self.path), after)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], audits)
        self.assertEqual(self.run_tool("--undo"), 0)
        self.assertEqual(snapshot(self.path), before)
        self.assertEqual(self.run_tool("--apply"), 0)
        self.assertEqual(snapshot(self.path), after)

    def test_one_part(self):
        self.assertEqual(self.run_tool("--part", "cleric", "--apply"), 0)
        self.assertEqual(self.q("SELECT npc_name FROM npcs WHERE id = 1"), [("Feria Hurkert",)])
        bc = {qid: json.loads(b)["9"]["items"] for qid, b in self.q("SELECT questid, byclass FROM queststeps WHERE byclass <> ''")}
        self.assertEqual(bc, {100759: "6357", 100760: "7529", 100761: "6147"})

    def refused(self, sql):
        db = sqlite3.connect(self.path)
        db.execute(sql)
        db.commit()
        db.close()
        before = snapshot(self.path)
        self.assertEqual(self.run_tool("--apply"), 2)
        self.assertEqual(snapshot(self.path), before)

    def test_refuses_a_name_taken(self):
        self.refused("INSERT INTO npcfaction (name, faction) VALUES ('razuam marus', 'x')")

    def test_refuses_another_source_of_the_item(self):
        self.refused("INSERT INTO loottables (tablename, patternid, weight) VALUES ('t', 700797, 1)")

    def test_refuses_the_bandit_not_found(self):
        self.refused("UPDATE npcs SET npc_level = 30 WHERE x_coord = 17653.2")

    def test_refuses_a_trigger_naming_an_old_name(self):
        self.refused("INSERT INTO gmtriggers (id, name, steps) VALUES (1, 'x', '[{\"do\":\"spawn\",\"npc\":\"Phoborium\"}]')")

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET npc_level = 50 WHERE npc_name = 'Blacktongue Ravager'")
        db.commit()
        db.close()
        self.run_tool("--undo")
        self.assertEqual(self.q("SELECT npc_level FROM npcs WHERE npc_name = 'Blacktongue Ravager'"), [(50,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE npc_name = 'Feria Hurkert'"), [(1,)])


if __name__ == "__main__":
    unittest.main()
