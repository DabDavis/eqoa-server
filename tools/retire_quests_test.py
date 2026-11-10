#!/usr/bin/env python3
"""tools/retire_quests.py against a made-up database: the quests an import logged for a file whose keys the file no
longer has are taken out with their steps, their trips, their steps' other creatures, the characters' progress and
their lines of the import's log; the file's other quests, another file's and a GM's own are left; --undo puts every
row back; a quest another still requires, or a mastery rank asks for, refuses the run. No game data.

    python3 tools/retire_quests_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import retire_quests as rq  # noqa: E402

SCHEMA = """
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
    giver TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', journal TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE questunlocks (questid INTEGER NOT NULL, npc TEXT NOT NULL, dest TEXT NOT NULL, PRIMARY KEY (questid, npc, dest));
CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL);
CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
    count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
    updated TEXT NOT NULL DEFAULT '', picks TEXT NOT NULL DEFAULT '', PRIMARY KEY (serverid, questid));
CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL, digest TEXT NOT NULL,
    enabled INTEGER NOT NULL);
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, name TEXT NOT NULL, require_quests TEXT NOT NULL DEFAULT '');
"""


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(SCHEMA)
    # homelands.json's: welcome and hunt (dropped by the file), flight (kept, it requires the two today) and L1 (new)
    for qid, key, name, req, file in ((101, "homelands/aviak/welcome", "Welcome", "", "homelands.json"),
                                      (102, "homelands/aviak/rats", "Rats", "101", "homelands.json"),
                                      (103, "homelands/aviak/flight", "Flight", "104", "homelands.json"),
                                      (104, "homelands/aviak/L1", "Wingband", "", "homelands.json"),
                                      (201, "1-20/halas/warrior/L1", "Tunic", "", "halas.json")):
        db.execute("INSERT INTO quests VALUES (?, ?, 1, 'Elder', ?, ?)", (qid, name, req, f"EQOA-Data {key} (tools/import_quests.py)"))
        db.execute("INSERT INTO queststeps VALUES (?, 1, 'talk', 'Elder', 'Talk.')", (qid,))
        db.execute("INSERT INTO quest_import_quests VALUES (?, ?, ?, 'd', 1)", (key, qid, file))
    db.execute("INSERT INTO quests VALUES (900, 'A GM''s own', 1, 'Elder', '', 'gm')")
    db.execute("INSERT INTO queststeps VALUES (102, 2, 'kill', 'scrub rat', 'Kill.')")
    db.execute("INSERT INTO queststeptargets VALUES (102, 2, 'dire rat')")
    db.execute("INSERT INTO questunlocks VALUES (101, 'Waysender X', 'Somewhere')")
    db.executemany("INSERT INTO charquests (serverid, questid, step, done) VALUES (?, ?, ?, ?)",
                   [(7, 101, 0, 1), (7, 102, 2, 0), (8, 101, 0, 1), (8, 201, 0, 1), (7, 104, 1, 0)])
    return db


def dump(db):
    return "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)


class RetireQuests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = make_db(os.path.join(self.dir.name, "q.db"))
        self.file = os.path.join(self.dir.name, "homelands.json")
        with open(self.file, "w") as f:
            json.dump({"city": "homelands", "quests": [{"key": "homelands/aviak/L1"}, {"key": "homelands/aviak/flight"}]}, f)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def count(self, table, where="1"):
        return self.db.execute(f"SELECT count(*) FROM {table} WHERE {where}").fetchone()[0]

    def test_the_dropped_quests_go_and_come_back(self):
        before = dump(self.db)
        gone, problems = rq.plan(self.db, [self.file])
        self.assertEqual(([(k, q) for k, q, _ in gone], problems), ([("homelands/aviak/welcome", 101), ("homelands/aviak/rats", 102)], []))
        n = rq.apply(self.db, gone)
        self.assertEqual(n, {"queststeps": 3, "questunlocks": 1, "queststeptargets": 1, "charquests": 3, "quests": 2,
                             "quest_import_quests": 2})
        self.assertEqual([r[0] for r in self.db.execute("SELECT questid FROM quests ORDER BY questid")], [103, 104, 201, 900],
                         "the file's kept quests, another file's and a GM's own stay")
        self.assertEqual(self.db.execute("SELECT serverid, questid FROM charquests ORDER BY 1, 2").fetchall(), [(7, 104), (8, 201)])
        self.assertEqual(self.count("quest_import_quests"), 3)
        # a second run finds nothing more
        self.assertEqual(rq.plan(self.db, [self.file]), ([], []))
        rq.undo(self.db)
        self.assertEqual(dump(self.db), before, "undo is exact, row for row")
        with self.assertRaises(SystemExit):
            rq.undo(self.db)

    def test_two_retirings_are_one_undo(self):
        before = dump(self.db)
        gone, _ = rq.plan(self.db, [self.file])
        rq.apply(self.db, gone[:1])
        gone, _ = rq.plan(self.db, [self.file])
        self.assertEqual([q for _, q, _ in gone], [102])
        rq.apply(self.db, gone)
        rq.undo(self.db)
        self.assertEqual(dump(self.db), before)

    def test_a_quest_still_required_or_asked_for_refuses(self):
        self.db.execute("UPDATE quests SET requires = '101, 104' WHERE questid = 103")   # the kept flight requires welcome
        gone, problems = rq.plan(self.db, [self.file])
        self.assertEqual(len(gone), 2)
        self.assertIn("quest 103 'Flight' requires [101]", "\n".join(problems))
        self.assertEqual(rq.main(["--db", os.path.join(self.dir.name, "q.db"), "--apply", self.file]), 1)
        self.assertEqual(self.count("quests"), 6, "nothing written")
        self.db.execute("UPDATE quests SET requires = '104' WHERE questid = 103")
        self.db.execute("INSERT INTO cmcatalog VALUES (1, 'Bulging Muscles 1', '102|201')")
        _, problems = rq.plan(self.db, [self.file])
        self.assertIn("mastery rank 1 'Bulging Muscles 1' asks for [102]", "\n".join(problems))
        # a group among the dropped quests themselves (rats requires welcome) is no problem: both go
        self.db.execute("DELETE FROM cmcatalog")
        self.assertEqual(rq.plan(self.db, [self.file])[1], [])

    def test_a_file_the_import_never_logged(self):
        other = os.path.join(self.dir.name, "elsewhere.json")
        with open(other, "w") as f:
            json.dump({"city": "x", "quests": []}, f)
        gone, problems = rq.plan(self.db, [other])
        self.assertEqual(gone, [])
        self.assertIn("logged no quest", problems[0])


if __name__ == "__main__":
    unittest.main()
