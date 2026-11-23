#!/usr/bin/env python3
"""Tests for side_quest_items_35_45_20261123.py on a small database holding the eleven quests' last steps and the
picked records: a dry run writes nothing; --apply gives each class its item on each last step (the ability quests'
spells kept) and a finisher its item as one approved grant; a second --apply writes nothing; a class's cell already
giving an item refuses the run; --undo puts every row back."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import side_quest_items_35_45_20261123 as t  # noqa: E402


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, byclass TEXT NOT NULL DEFAULT '',
            PRIMARY KEY (questid, step));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER, levelreq INTEGER);
        CREATE TABLE charquests (serverid INTEGER, questid INTEGER, done INTEGER, PRIMARY KEY (serverid, questid));
        CREATE TABLE characters (serverid INTEGER PRIMARY KEY, accountid INTEGER, charName TEXT, tclass INTEGER);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER);
        CREATE TABLE chartransfers (id INTEGER PRIMARY KEY AUTOINCREMENT, accountid INTEGER, serverid INTEGER,
            charname TEXT, from_server TEXT, from_name TEXT, from_class TEXT, notes TEXT, status TEXT, filed TEXT,
            updated TEXT, plan TEXT, decided TEXT, decided_by TEXT);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT,
            via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
    """)
    masks = {}
    for cells in t.PICKS.values():
        for c, (pid, name) in cells.items():
            masks.setdefault((pid, name), 0)
            masks[(pid, name)] |= 1 << c
    for (pid, name), m in masks.items():
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, 40)", (pid, name, m))
    for qid, name, kind, npc in t.QUESTS:
        db.execute("INSERT INTO quests VALUES (?, ?)", (qid, name))
        db.execute("INSERT INTO queststeps VALUES (?, 1, 'talk', 'someone', '')", (qid,))
        bc = json.dumps({"3": {"spells": "1565"}}) if qid == 100787 else ""
        db.execute("INSERT INTO queststeps VALUES (?, 2, ?, ?, ?)", (qid, kind, npc, bc))
    db.execute("INSERT INTO characters VALUES (61, 9, 'Tester', 3)")
    db.execute("INSERT INTO charquests VALUES (61, 100787, 1)")
    db.execute("INSERT INTO charquests VALUES (61, 100788, 0)")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = t.main(list(args))
    return code, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    got = {n: sorted(db.execute(f"SELECT * FROM {n}").fetchall()) for n in ("queststeps", "chartransfers")}
    db.close()
    return got


class SideQuestItems(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_reapply_undo(self):
        was = dump(self.db)
        code, out = run("--db", self.db, "--shard", "live")
        self.assertEqual(code, 0, out)
        self.assertIn("12 rows to write", out)
        self.assertEqual(dump(self.db), was)
        code, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.db)
        for qid, _, _, _ in t.QUESTS:
            bc = json.loads(db.execute("SELECT byclass FROM queststeps WHERE questid = ? AND step = 2", (qid,)).fetchone()[0])
            for c in range(15):
                self.assertEqual(bc[str(c)]["items"], str(t.PICKS[qid][c][0]))
        bc = json.loads(db.execute("SELECT byclass FROM queststeps WHERE questid = 100787 AND step = 2").fetchone()[0])
        self.assertEqual(bc["3"]["spells"], "1565")
        self.assertEqual(db.execute("SELECT byclass FROM queststeps WHERE step = 1 AND byclass != ''").fetchall(), [])
        plan = json.loads(db.execute("SELECT plan FROM chartransfers WHERE serverid = 61 AND status = 'approved'").fetchone()[0])
        self.assertEqual(plan["items"], [{"pattern": t.PICKS[100787][3][0], "count": 1}])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 12)
        db.close()
        code, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertIn("nothing to write", out)
        code, out = run("--db", self.db, "--shard", "live", "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), was)

    def test_a_cell_with_an_item_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE queststeps SET byclass = ? WHERE questid = 100782 AND step = 2", (json.dumps({"0": {"items": "1"}}),))
        db.commit()
        db.close()
        was = dump(self.db)
        code, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(code, 1)
        self.assertIn("REFUSED", out)
        self.assertEqual(dump(self.db), was)


if __name__ == "__main__":
    unittest.main()
