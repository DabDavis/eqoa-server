#!/usr/bin/env python3
"""owner_pet1122_20261122.py on a small database: a Heirloom-like shard (all three parts) and a Live-like one (the
breaths alone: no homelands); apply, a second apply that writes nothing, undo exact; a row not as read refuses."""
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import owner_pet1122_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemicon INTEGER, equipslot INTEGER,
    levelreq INTEGER, classuse INTEGER, raceuse INTEGER, maxstack INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, byclass TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (questid, step));
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_id INTEGER, npc_type INTEGER, npc_level INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER NOT NULL DEFAULT 0,
    remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0);
CREATE TABLE classtemplates (name TEXT PRIMARY KEY);
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
"""

BYCLASS = '{"0":{"items":"8571"},"1":{"items":"5647"},"10":{"items":"4927"}}'


def make(path, homelands=True):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?, ?)", [
        (701506, "ethereal essence", -607649328, -1, 1, 32767, 1023, 1),
        (701507, "astral breath", -607649328, -1, 13, 64, 2015, 1),
        (701508, "ethereal breath", -607649328, -1, 13, 64, 2015, 1),
        (1805, "arrow", 2094679524, -1, 1, 32767, 1023, 9999),
        (5647, "glade crafted longbow", 1, 14, 4, 2, 1023, 1),
    ])
    db.execute("INSERT INTO classtemplates VALUES ('roamer minor poison')")
    if homelands:
        db.execute("INSERT INTO quests VALUES (101606, 'Arms for the Old Road', 'Pitmaster Grukk')")
        db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?)", [
            (101606, 1, "loot", "cave bat", ""), (101606, 3, "give", "Pitmaster Grukk", BYCLASS)])
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
            (165352, "Grocer Mudgrub", 165352, 129, 25), (165377, "Mother Mossweb", 165377, 0, 5)])
        db.executemany("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                       "VALUES (165352, 1, ?, -1, 1, ?)", [(1932, 0), (1942, 12)])
    db.commit()
    return db


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        code = tool.main(list(args))
    return code, out.getvalue()


def snapshot(db):
    return {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
            for t in ("itempattern", "queststeps", "npcitem", "npctemplate")}


class OwnerPet1122Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_heirloom_apply_again_undo(self):
        db = make(self.path)
        before = snapshot(db)
        code, out = run("--db", self.path, "--shard", "heirloom")
        self.assertEqual(code, 0, out)
        self.assertIn("5 rows would change", out)
        self.assertEqual(snapshot(db), before, "a dry run writes nothing")

        code, out = run("--db", self.path, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 5 rows", out)
        for pid in (701507, 701508):
            self.assertEqual(db.execute("SELECT levelreq, classuse, raceuse FROM itempattern WHERE patternid = ?",
                                        (pid,)).fetchone(), (1, 32767, 1023))
        bc = json.loads(db.execute("SELECT byclass FROM queststeps WHERE questid = 101606 AND step = 3").fetchone()[0])
        self.assertEqual(bc["1"]["items"], "5647, 1805*200")
        self.assertEqual(bc["0"]["items"], "8571", "another class's reward stays")
        self.assertEqual(db.execute("SELECT patternid, listnumber, equiploc, location FROM npcitem WHERE npcid = 165352 "
                                    "ORDER BY listnumber DESC LIMIT 1").fetchone(), (1805, 13, -1, 1))
        self.assertEqual(db.execute("SELECT template FROM npctemplate WHERE name = 'mother mossweb'").fetchone(),
                         ("roamer minor poison",))
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 5)
        applied = snapshot(db)

        code, out = run("--db", self.path, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 0 rows", out)
        self.assertEqual(snapshot(db), applied, "a second apply writes nothing")

        code, out = run("--db", self.path, "--shard", "heirloom", "--undo")
        self.assertEqual(code, 0, out)
        self.assertIn("5 rows put back", out)
        self.assertEqual(snapshot(db), before, "undo is exact")

    def test_live_has_no_homelands(self):
        db = make(self.path, homelands=False)
        code, out = run("--db", self.path, "--shard", "live", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 2 rows", out)
        self.assertIn("no quest 'Arms for the Old Road'", out)
        self.assertIn("no 'mother mossweb'", out)
        self.assertEqual(db.execute("SELECT count(*) FROM itempattern WHERE classuse = 32767 AND levelreq = 1").fetchone()[0], 4)

    def test_not_as_read_refuses_everything(self):
        db = make(self.path)
        db.execute("UPDATE queststeps SET byclass = ? WHERE questid = 101606 AND step = 3",
                   (BYCLASS.replace('"5647"', '"5648"'),))
        db.commit()
        before = snapshot(db)
        code, out = run("--db", self.path, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(snapshot(db), before)

    def test_a_tuned_spider_is_left(self):
        db = make(self.path)
        db.execute("INSERT INTO npctemplate VALUES ('mother mossweb', 'roamer', '', 'gm')")
        db.commit()
        code, out = run("--db", self.path, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("fights by 'roamer'", out)

    def test_undo_leaves_a_row_changed_since(self):
        db = make(self.path)
        run("--db", self.path, "--shard", "heirloom", "--apply")
        db.execute("UPDATE itempattern SET levelreq = 5 WHERE patternid = 701508")
        db.commit()
        code, out = run("--db", self.path, "--shard", "heirloom", "--undo")
        self.assertIn("changed since, left", out)
        self.assertEqual(db.execute("SELECT levelreq FROM itempattern WHERE patternid = 701508").fetchone()[0], 5)
        self.assertEqual(db.execute("SELECT levelreq FROM itempattern WHERE patternid = 701507").fetchone()[0], 13)


if __name__ == "__main__":
    unittest.main()
