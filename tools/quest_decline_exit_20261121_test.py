#!/usr/bin/env python3
"""quest_decline_exit_20261121.py: apply, a second apply, undo, and a row changed since left alone."""
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import quest_decline_exit_20261121 as tool  # noqa: E402


def make(path):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE quests (questid INTEGER PRIMARY KEY, decline TEXT NOT NULL DEFAULT '')")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
               "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)")
    db.executemany("INSERT INTO quests VALUES (?, ?)", [(1, "Exit"), (2, "Not now."), (3, ""), (4, " exit "), (5, "Exit now")])
    db.commit()
    db.close()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def declines(self):
        db = sqlite3.connect(self.path)
        try:
            return dict(db.execute("SELECT questid, decline FROM quests"))
        finally:
            db.close()

    def test_apply_again_undo(self):
        tool.main(["--db", self.path, "--apply"])
        self.assertEqual(self.declines(), {1: "Not now.", 2: "Not now.", 3: "", 4: "Not now.", 5: "Exit now"})
        tool.main(["--db", self.path, "--apply"])
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 2)
        db.close()
        tool.main(["--db", self.path, "--undo"])
        self.assertEqual(self.declines(), {1: "Exit", 2: "Not now.", 3: "", 4: " exit ", 5: "Exit now"})

    def test_undo_leaves_a_changed_row(self):
        tool.main(["--db", self.path, "--apply"])
        db = sqlite3.connect(self.path)
        db.execute("UPDATE quests SET decline = 'Later.' WHERE questid = 1")
        db.commit()
        db.close()
        tool.main(["--db", self.path, "--undo"])
        self.assertEqual(self.declines()[1], "Later.")
        self.assertEqual(self.declines()[4], " exit ")


if __name__ == "__main__":
    unittest.main()
