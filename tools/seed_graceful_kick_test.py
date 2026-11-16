#!/usr/bin/env python3
"""tools/seed_graceful_kick.py against a made-up database: the rank written in
the bard's class list at Graceful Strike's level, a report writing nothing, a
second --apply refused, an id another row holds refused, a database without
the art refused, and --undo taking the rank and its owners' rows out, but
never a row the tool did not write. No game data.

    python3 tools/seed_graceful_kick_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_graceful_kick.py")

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
    requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '', require_ranks TEXT NOT NULL DEFAULT '',
    require_quests TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '', adds TEXT NOT NULL DEFAULT '',
    add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '', remove_spells TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER NOT NULL, spellname TEXT NOT NULL);
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT, action TEXT,
    tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""


def make(path, art=True):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    if art:
        db.execute("INSERT INTO spellpattern VALUES (11, 16, 'Graceful Strike')")
    db.execute("INSERT INTO spellpattern VALUES (12, 1, 'Kick')")
    db.execute("INSERT INTO cmcatalog (id, mastery, name, cost, source) VALUES (55, 'Breathy Ballad', 'Breathy Ballad 1', 50, 'x')")
    db.commit()
    db.close()


def run(db, *args):
    return subprocess.run([sys.executable, TOOL, "--db", db, *args], capture_output=True, text=True)


def rank(db, rid=900916):
    c = sqlite3.connect(db)
    c.row_factory = sqlite3.Row
    r = c.execute("SELECT * FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
    c.close()
    return r


class SeedGracefulKick(unittest.TestCase):
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
        self.assertEqual((got["name"], got["category"], got["classes"], got["races"], got["cost"], got["level"], got["effects"]),
                         ("Graceful Kick", 3, 32, 2047, 0, 16, "GracefulKick:1"))
        self.assertNotEqual(run(self.db, "--apply").returncode, 0, "a second --apply went in")
        c = sqlite3.connect(self.db)
        c.execute("INSERT INTO charcms (serverid, cmid, cost) VALUES (1, 900916, 0), (2, 900916, 0), (2, 55, 50)")
        c.commit()
        self.assertEqual(c.execute("SELECT count(*) FROM gm_audit WHERE tbl = 'cmcatalog' AND action = 'insert'").fetchone()[0], 1)
        c.close()
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIsNone(rank(self.db))
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT serverid, cmid FROM charcms").fetchall(), [(2, 55)], "undo took another rank")
        self.assertEqual(c.execute("SELECT count(*) FROM gm_audit WHERE action = 'delete'").fetchone()[0], 1)
        c.close()

    def test_refusals(self):
        make(self.db, art=False)
        r = run(self.db, "--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("Graceful Strike", r.stderr)
        self.assertIsNone(rank(self.db), "no art and still the rank")
        os.remove(self.db)
        make(self.db)
        self.assertNotEqual(run(self.db, "--apply", "--id", "55").returncode, 0, "it wrote over another rank")
        self.assertEqual(rank(self.db, 55)["name"], "Breathy Ballad 1")
        self.assertNotEqual(run(self.db, "--undo", "--id", "55").returncode, 0, "undo took a rank it did not write")
        self.assertIsNotNone(rank(self.db, 55))


if __name__ == "__main__":
    unittest.main()
