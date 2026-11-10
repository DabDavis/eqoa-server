#!/usr/bin/env python3
"""tools/homes_flown.py against a made-up database: a character whose flight is finished gets the five marked
finished in a home that has the five, and only the ones it lacks; one that never flew, one that is deleted and a
home still on its old quests get nothing; a second run adds nothing; --undo leaves the database as it was.

    python3 tools/homes_flown_test.py
"""
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import homes_flown as hf  # noqa: E402

SCHEMA = """
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT);
CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
    count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
    updated TEXT NOT NULL DEFAULT '', picks TEXT NOT NULL DEFAULT '', PRIMARY KEY (serverid, questid));
CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL, digest TEXT NOT NULL,
    enabled INTEGER NOT NULL);
"""


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(SCHEMA)
    keys = {"homelands/aviak/flight": 100, "homelands/kobold/flight": 200, "homelands/kobold/welcome": 201,
            "1-20/halas/warrior/L1": 300}
    keys.update({f"homelands/aviak/L{i}": 100 + i for i in range(1, 6)})
    for k, q in keys.items():
        db.execute("INSERT INTO quest_import_quests VALUES (?, ?, 'f.json', 'd', 1)", (k, q))
    db.executemany("INSERT INTO characters VALUES (?, ?)", [(1, "Flown"), (2, "Home"), (4, "Kobold"), (5, "Half")])
    db.executemany("INSERT INTO charquests (serverid, questid, step, done) VALUES (?, ?, ?, ?)", [
        (1, 100, 0, 1),                  # flew: the five
        (2, 100, 1, 0),                  # at the gate, not flown: nothing
        (3, 100, 0, 1),                  # flew, deleted since: nothing
        (4, 200, 0, 1),                  # flew from a home still on its old quests: nothing
        (5, 100, 0, 2), (5, 101, 0, 1), (5, 102, 2, 0),   # flew; has L1 and an L2 line: the other three
    ])
    return db


def dump(db):
    return "\n".join(db.iterdump())


class Flown(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = make_db(os.path.join(self.dir.name, "t.db"))

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def test_plan(self):
        rows = hf.plan(self.db)
        self.assertEqual([(s, q) for s, q, _ in rows], [(1, 101), (1, 102), (1, 103), (1, 104), (1, 105), (5, 103), (5, 104), (5, 105)])

    def test_apply_again_undo(self):
        before = dump(self.db)
        hf.apply(self.db, hf.plan(self.db))
        got = self.db.execute("SELECT step, count, done FROM charquests WHERE serverid = 1 AND questid = 103").fetchone()
        self.assertEqual(got, (0, 0, 1))
        self.assertEqual(self.db.execute("SELECT step, done FROM charquests WHERE serverid = 5 AND questid = 102").fetchone(), (2, 0))
        self.assertEqual(hf.plan(self.db), [])
        self.assertEqual(hf.undo(self.db), 8)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
