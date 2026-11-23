#!/usr/bin/env python3
"""Tests for petpass_heirloom_1123q.py on a small database holding the rows as read on Heirloom: a dry run writes
nothing; --apply renames Udrik's first tier and puts Dark Ore back to 26-32 once (a second --apply nothing, a report
then says both are already made), each with a gm_audit line; a row not as read refuses the whole run; --undo puts
every row back exactly and drops the log."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_heirloom_1123q as t  # noqa: E402

ROWS = [
    (101181, "Pickclaw Sashes (1-10)", "Udrik Lazow", 1, 30, "EQOA-Data HuntAndGather/udrik-pickclaw-1 (tools/import_quests.py)"),
    (101183, "Pickclaw Sashes (21-30)", "Udrik Lazow", 21, 30, "EQOA-Data HuntAndGather/udrik-pickclaw-3 (tools/import_quests.py)"),
    (101147, "Dark Ore", "Corison", 26, 60, "EQOA-Data HuntAndGather/corison-dark-ore (tools/import_quests.py)"),
]


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
            giver TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
            repeatable INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT '');""")
    db.executemany("INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, repeatable, source) "
                   "VALUES (?, ?, 1, ?, ?, ?, 1, ?)", [(q, n, g, lo, hi, s) for q, n, g, lo, hi, s in ROWS])
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
        code = t.main(list(args))
    return code, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    r = db.execute("SELECT * FROM quests ORDER BY questid").fetchall()
    db.close()
    return r


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, qid):
        db = sqlite3.connect(self.db)
        r = db.execute("SELECT name, minlevel, maxlevel FROM quests WHERE questid = ?", (qid,)).fetchone()
        db.close()
        return r

    def test_dry_run_writes_nothing(self):
        before = dump(self.db)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("2 rows would change", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_once_then_undo(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q(101181), ("Pickclaw Sashes (1-30)", 1, 30))
        self.assertEqual(self.q(101147), ("Dark Ore", 26, 32))
        self.assertEqual(self.q(101183), ("Pickclaw Sashes (21-30)", 21, 30))
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE tbl = 'quests'").fetchone()[0], 2)
        db.close()
        code, out = run("--db", self.db, "--apply", "--no-backup")
        self.assertIn("applied already", out)
        code, out = run("--db", self.db)
        self.assertEqual(out.count("already  2816"), 2, out)
        code, out = run("--db", self.db, "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before)
        db = sqlite3.connect(self.db)
        self.assertFalse(t.has_table(db, t.LOG))
        db.close()

    def test_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE quests SET maxlevel = 40 WHERE questid = 101147")
        db.commit()
        db.close()
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply", "--no-backup")
        self.assertEqual(code, 2, out)
        self.assertIn("not as read", out)
        self.assertEqual(dump(self.db), before)

    def test_wrong_levels_refuse(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE quests SET maxlevel = 10 WHERE questid = 101181")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--apply", "--no-backup")
        self.assertEqual(code, 2, out)
        self.assertIn("want 1-30", out)


if __name__ == "__main__":
    unittest.main()
