#!/usr/bin/env python3
"""tools/seed_were_own_body.py against a made-up database: the rank written
with the five lines' third quests as one group, a report writing nothing, a
second --apply refused, an id another row holds refused, a line with no
imported quest refusing the whole, and --undo taking the rank and its owners'
rows out, but never a row the tool did not write. No game data.

    python3 tools/seed_were_own_body_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_were_own_body.py")

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
    requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '', require_ranks TEXT NOT NULL DEFAULT '',
    require_quests TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '', adds TEXT NOT NULL DEFAULT '',
    add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '', remove_spells TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL DEFAULT '',
    digest TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1);
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT, action TEXT,
    tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""
LINES = ["wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"]


def make(path, lines=LINES):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for i, line in enumerate(lines):
        db.execute("INSERT INTO quest_import_quests (key, questid) VALUES (?, ?)", (f"Weres/{line}/p3", 7000 + i))
        db.execute("INSERT INTO quest_import_quests (key, questid) VALUES (?, ?)", (f"Weres/{line}/p2", 7100 + i))
    db.execute("INSERT INTO cmcatalog (id, mastery, name, cost, source) VALUES (55, 'Feral Rat Form', 'Feral Rat Form 1', 50, 'x')")
    db.commit()
    db.close()


def run(db, *args):
    return subprocess.run([sys.executable, TOOL, "--db", db, *args], capture_output=True, text=True)


def rank(db, rid=900179):
    c = sqlite3.connect(db)
    c.row_factory = sqlite3.Row
    r = c.execute("SELECT * FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
    c.close()
    return r


class SeedWereOwnBody(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIsNone(rank(self.db), "a report wrote the rank")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        got = rank(self.db)
        self.assertEqual((got["cost"], got["level"], got["effects"], got["require_quests"], got["classes"], got["races"]),
                         (0, 30, "NoWereIllusion:1", "7000|7001|7002|7003|7004", 32767, 2047))
        self.assertNotEqual(run(self.db, "--apply").returncode, 0, "a second --apply went in")
        c = sqlite3.connect(self.db)
        c.execute("INSERT INTO charcms (serverid, cmid, cost) VALUES (1, 900179, 0), (2, 900179, 0), (2, 55, 50)")
        c.commit()
        self.assertEqual(c.execute("SELECT count(*) FROM gm_audit WHERE tbl = 'cmcatalog' AND action = 'insert'").fetchone()[0], 1)
        c.close()
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIsNone(rank(self.db))
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT serverid, cmid FROM charcms").fetchall(), [(2, 55)], "undo took another rank")
        c.close()

    def test_refusals(self):
        make(self.db, LINES[:4])
        r = run(self.db, "--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("gatorwere", r.stderr)
        self.assertIsNone(rank(self.db), "a missing line still wrote the rank")
        os.remove(self.db)
        make(self.db)
        self.assertNotEqual(run(self.db, "--apply", "--id", "55").returncode, 0, "it wrote over another rank")
        self.assertEqual(rank(self.db, 55)["name"], "Feral Rat Form 1")
        self.assertNotEqual(run(self.db, "--undo", "--id", "55").returncode, 0, "undo took a rank it did not write")
        self.assertIsNotNone(rank(self.db, 55))


if __name__ == "__main__":
    unittest.main()
