#!/usr/bin/env python3
"""tools/siliskor_quest_20261118.py against a made-up database: the Siliskor quest as the importer and the petitions
left it (ten steps, step 9 raising Pomus and two worshippers), Siliskor's two rows and template, the cave's and the
statue's worshippers, the lead's spawn-only Siliskor and Pomus. No game data.

    python3 tools/siliskor_quest_20261118_test.py
"""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import siliskor_quest_20261118 as sq  # noqa: E402

SCHEMA = """
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
  giver TEXT NOT NULL DEFAULT '', repeatable INTEGER NOT NULL DEFAULT 0, notes TEXT NOT NULL DEFAULT '', offer TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
  target TEXT NOT NULL DEFAULT '', world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0,
  z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 15, journal TEXT NOT NULL DEFAULT '',
  events TEXT NOT NULL DEFAULT '', height REAL NOT NULL DEFAULT 0, PRIMARY KEY (questid, step));
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL, z_coord REAL,
  npc_level INTEGER);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
  special TEXT NOT NULL DEFAULT '', special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0,
  abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
  special_every REAL NOT NULL DEFAULT 0);
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
  weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
  source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
  rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
  PRIMARY KEY (npc_name, tablename));
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, trade INTEGER NOT NULL DEFAULT 0);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
  username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
  rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""

QID = 101266
STEPS = [
    (1, "talk", "Corliss Magemell", 0, 0, 0, ""),
    (2, "reach", "", 14406, 17, 23544, '[{"on":"done","do":"popup","text":"At the dragon altar a voice fills your head."}]'),
    (3, "reach", "", 14334, 23, 23585, '[{"on":"done","do":"popup","text":"Outside, a great dragon eyes you."}]'),
    (4, "talk", "Corliss Magemell", 0, 0, 0, ""),
    (5, "reach", "", 14406, 17, 23544, ""),
    (6, "loot", "mirrored soul", 0, 0, 0, '[{"on":"start","do":"spawn","npc":"mirrored soul","hostile":true}]'),
    (7, "give", "Corliss Magemell", 0, 0, 0, ""),
    (8, "reach", "", 14549, 15, 26778, '[{"on":"done","do":"popup","text":"The ground shakes. Siliskor rises."}]'),
    (9, "kill", "Siliskor", 0, 0, 0, '[{"on":"start","do":"spawn","npc":"Pomus Magemell","hostile":true}]'),
    (10, "talk", "Corliss Magemell", 0, 0, 0, ""),
]
NPCS = [
    (101337, "Siliskor", 14267.4, 54.1, 23459.8, 55), (111881, "Siliskor", 14506.9, 15.6, 26767.7, 55),
    (114081, "mirrored soul", 14406.4, 17.0, 23543.5, 49), (123008, "Pomus Magemell", 14277.4, 54.1, 23460.7, 50),
    (111883, "Pamus Magemell", 14543.3, 15.3, 26773.3, 53), (111882, "dragon worshipper", 14541.7, 15.3, 26791.2, 53),
    (111884, "dragon worshipper", 14550.8, 15.3, 26762.6, 53), (114076, "dragon worshipper", 14308.3, 96.4, 23727.5, 49),
    (101355, "Zaryn Boome", 15219.7, 54.3, 22714.7, 39),
]
ITEMS = [(6125, "shield of king jorat"), (6124, "dragonhide shield"), (4986, "Lythrial's Robe"),
         (5812, "Rycoin's Scimitar"), (5363, "Siliskor's Horn")]


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO quests (questid, name, enabled, giver) VALUES (?, 'Siliskor', 1, 'Zaryn Boome')", (QID,))
    for s, kind, target, x, y, z, ev in STEPS:
        db.execute("INSERT INTO queststeps (questid, step, kind, target, x, y, z, events) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                   (QID, s, kind, target, x, y, z, ev))
    for n in NPCS:
        db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, npc_level) VALUES (?, ?, 0, ?, ?, ?, ?)", n)
    db.execute("INSERT INTO npcspawnonly (name, note) VALUES ('Siliskor', 'the lead'), ('Pomus Magemell', 'questspawn')")
    db.execute("INSERT INTO npctemplate (name, template) VALUES ('siliskor', 'siliskor')")
    db.execute("INSERT INTO classtemplates (name, class, hp_pct, special, special_damage, abilities, note, special_every) "
               "VALUES ('siliskor', 0, 1229, 'Critical Hit', 18.18, ?, 'petition 1240', 10)", (json.dumps([sq.OLD_BLIGHT]),))
    db.execute("INSERT INTO classtemplates (name, class, hp_pct, special, special_damage, note, special_every) "
               "VALUES ('roamer', 0, 100, 'Critical Hit', 7.4, 'roamer', 30)")
    for pid, name in ITEMS:
        db.execute("INSERT INTO itempattern (patternid, itemname, trade) VALUES (?, ?, 0)", (pid, name))
    db.execute("INSERT INTO loottables (tablename, patternid, maxlevel) VALUES ('boss siliskor', 6125, 70)")
    db.execute("INSERT INTO npcloot (npc_name, tablename, chance) VALUES ('siliskor', 'boss siliskor', 100)")
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {}
    for t in ("quests", "queststeps", "npcs", "npcspawnonly", "npctemplate", "classtemplates", "loottables", "npcloot",
              "itempattern"):
        out[t] = db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
    db.close()
    return out


def run(*args):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        code = sq.main(list(args))
    return code, buf.getvalue()


class SiliskorTool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make_db(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *args):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_report_only_writes_nothing(self):
        before = dump(self.db)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(dump(self.db), before)
        self.assertEqual(run("--db", self.db, "--applied")[0], 1)

    def test_apply_then_undo_is_exact(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        ev9 = json.loads(self.q("SELECT events FROM queststeps WHERE step = 9")[0][0])
        self.assertEqual([e["npc"] for e in ev9], ["Siliskor", "Pomus Magemell", "dragon worshipper", "dragon worshipper"])
        self.assertTrue(all(e["group"] and e["minutes"] == sq.LIFE for e in ev9))
        self.assertEqual((ev9[0]["x"], ev9[0]["z"], ev9[0]["hostile"]), (14506.9, 26767.7, True))  # the statue's row
        self.assertNotIn("hostile", ev9[1])
        ev3 = json.loads(self.q("SELECT events FROM queststeps WHERE step = 3")[0][0])
        watch = ev3[-1]
        self.assertEqual((watch["npc"], watch["untouchable"], watch["on"], watch["x"]), ("#111881", True, "done", 14267.4))
        s3 = self.q("SELECT x, y, z, radius, height FROM queststeps WHERE step = 3")[0]
        self.assertEqual(s3, (14306.0, 56.0, 23577.0, 25.0, 10.0))
        self.assertEqual(self.q("SELECT x, y, z, height FROM queststeps WHERE step = 2")[0], (14406.4, 17.0, 23543.5, 10.0))
        so = {r[0] for r in self.q("SELECT name FROM npcspawnonly")}
        self.assertEqual(so, {"Siliskor", "Pomus Magemell", "mirrored soul", "pamus magemell", "#111882", "#111884"})
        lv = dict(self.q("SELECT id, npc_level FROM npcs WHERE id IN (101337, 111881, 114081, 123008)"))
        self.assertEqual(lv, {101337: 57, 111881: 57, 114081: 50, 123008: 55})
        hp, every, ab = self.q("SELECT hp_pct, special_every, abilities FROM classtemplates WHERE name = 'siliskor'")[0]
        self.assertEqual((hp, every), (1914, 0))
        self.assertEqual(json.loads(ab), [sq.BLIGHT, sq.VENOM])
        self.assertEqual(self.q("SELECT hp_pct FROM classtemplates WHERE name = 'pomus magemell'"), [(328,)])
        self.assertEqual(self.q("SELECT trade FROM itempattern WHERE patternid = 6125"), [(1,)])
        self.assertEqual(self.q("SELECT repeatable FROM quests WHERE questid = ?", QID), [(1,)])
        self.assertEqual(sorted(r[0] for r in self.q("SELECT patternid FROM loottables WHERE tablename = ?", sq.DROP_TABLE)),
                         [4986, 5363, 5812, 6124])
        self.assertEqual(self.q("SELECT npc_row, up_minutes, gone_min, gone_max FROM npcrarecycle"), [(101355, 30, 60, 180)])
        self.assertEqual(run("--db", self.db, "--applied")[0], 0)
        code, out = run("--db", self.db, "--apply")
        self.assertIn("applied already", out)

        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before)  # the lead's Siliskor and Pomus rows stay
        self.assertEqual(run("--db", self.db, "--applied")[0], 1)

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.db, "--apply")
        db = sqlite3.connect(self.db, isolation_level=None)
        db.execute("UPDATE classtemplates SET hp_pct = 2000 WHERE name = 'siliskor'")
        db.close()
        code, out = run("--db", self.db, "--undo")
        self.assertIn("changed since", out)
        self.assertEqual(self.q("SELECT hp_pct FROM classtemplates WHERE name = 'siliskor'"), [(2000,)])
        self.assertEqual(self.q("SELECT npc_level FROM npcs WHERE id = 111881"), [(55,)])

    def test_refuses_a_database_not_as_expected(self):
        db = sqlite3.connect(self.db, isolation_level=None)
        db.execute("UPDATE npcs SET npc_level = 57 WHERE id = 111881")
        db.close()
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 1)
        self.assertIn("refused", out)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
