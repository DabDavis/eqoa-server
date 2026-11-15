#!/usr/bin/env python3
"""tools/norach_raft_20261115.py on a made database: the report writes nothing; --apply lands the hand-in on the raft,
audited and logged; a second run finds nothing to do; --undo puts back exactly what was there; a step not as the
import wrote it is refused and left alone.

    python3 -E -s tools/norach_raft_20261115_test.py
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
import norach_raft_20261115 as tool  # noqa: E402


def make(path, events):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, tunar INTEGER, events TEXT,
                                 PRIMARY KEY (questid, step));
        CREATE TABLE accountinfo (accountid INTEGER, username TEXT);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
                               action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
        INSERT INTO accountinfo VALUES (7, 'dab');
        INSERT INTO quests VALUES (101309, 'The Medallion of the Faceless', 'Norach');
    """)
    db.execute("INSERT INTO queststeps VALUES (101309, 1, 'give', 'Norach', 10000, ?)", (events,))
    db.commit()
    db.close()


def run(*argv):
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        code = tool.main(list(argv))
    return code, out.getvalue() + err.getvalue()


def events(path):
    db = sqlite3.connect(path)
    try:
        return db.execute("SELECT events FROM queststeps WHERE questid = 101309 AND step = 1").fetchone()[0]
    finally:
        db.close()


OLD = '[{"on":"done","do":"teleport","world":0,"x":24500.0,"y":54.1,"z":3330.0}]'


class NorachRaft(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db, OLD)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("-> the raft", out)
        self.assertEqual(events(self.db), OLD, "the report wrote")
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(json.loads(events(self.db)),
                         [{"on": "done", "do": "teleport", "world": 0, "x": 24578.0, "y": 52.6, "z": 3626.0, "facing": 3.9}])
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*), max(accountid) FROM gm_audit WHERE tbl = 'queststeps'").fetchone(), (1, 7))
        self.assertEqual(db.execute(f"SELECT count(*) FROM {tool.LOG}").fetchone()[0], 1)
        db.close()
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(events(self.db), OLD)
        code, out = run("--db", self.db, "--undo")
        self.assertIn("nothing to undo", out)

    def test_changed_step_refused(self):
        other = '[{"on":"done","do":"teleport","world":0,"x":1.0,"y":2.0,"z":3.0}]'
        make(self.db, other)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(events(self.db), other)


if __name__ == "__main__":
    unittest.main()
