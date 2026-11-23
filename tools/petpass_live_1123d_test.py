#!/usr/bin/env python3
"""Tests for petpass_live_1123d.py on a small database holding the rows as read on Live (the data file's old values):
a dry run writes nothing; --apply writes each change once (a second --apply nothing): the Eye a gem, the four quests'
words; a row not as read refuses the whole run; --undo puts every table back and drops the log."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_live_1123d as t  # noqa: E402

TABLES = ("itempattern", "quests", "queststeps", "queststeptargets")
STEP_COLS = ("kind", "target", "npcid", "items", "tunar", "count", "world", "x", "y", "z", "radius", "journal",
             "answer", "says", "waiting", "notyet", "reminder", "xp", "rewardtunar", "rewarditems", "factions",
             "spells", "events", "chance", "byclass", "choices", "height", "anyorder", "exactbase")


def data():
    with open(t.DATA, encoding="utf-8") as f:
        return json.load(f)


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, maxstack INTEGER, procanim INTEGER);
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, offer TEXT, accept TEXT, decline TEXT,
            declined TEXT, started TEXT);
        CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
            target TEXT NOT NULL DEFAULT '', npcid INTEGER NOT NULL DEFAULT 0, items TEXT NOT NULL DEFAULT '',
            tunar INTEGER NOT NULL DEFAULT 0, count INTEGER NOT NULL DEFAULT 1, world INTEGER NOT NULL DEFAULT 0,
            x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0,
            radius REAL NOT NULL DEFAULT 15, journal TEXT NOT NULL DEFAULT '', answer TEXT NOT NULL DEFAULT '',
            says TEXT NOT NULL DEFAULT '', waiting TEXT NOT NULL DEFAULT '', notyet TEXT NOT NULL DEFAULT '',
            reminder TEXT NOT NULL DEFAULT '', xp INTEGER NOT NULL DEFAULT 0, rewardtunar INTEGER NOT NULL DEFAULT 0,
            rewarditems TEXT NOT NULL DEFAULT '', factions TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '',
            events TEXT NOT NULL DEFAULT '', chance REAL NOT NULL DEFAULT 0, byclass TEXT NOT NULL DEFAULT '',
            choices TEXT NOT NULL DEFAULT '', height REAL NOT NULL DEFAULT 0, anyorder INTEGER NOT NULL DEFAULT 0,
            exactbase INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (questid, step));
        CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
            PRIMARY KEY (questid, step, target));
    """)
    db.execute("INSERT INTO itempattern VALUES (?, ?, 1, 0)", (t.EYE, t.EYE_NAME))
    for q in data()["quests"]:
        o = q["quest_old"]
        db.execute("INSERT INTO quests VALUES (?, ?, ?, ?, ?, ?, ?)", (q["questid"], o["name"], o["offer"],
                   o["accept"], o["decline"], o["declined"], o["started"]))
        for st in q["steps"]:
            if st["old"] is None:
                continue
            cols = ["questid", "step"] + list(st["old"])
            db.execute(f"INSERT INTO queststeps ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [q["questid"], st["step"]] + list(st["old"].values()))
        for tg in q["targets_old"]:
            db.execute("INSERT INTO queststeptargets VALUES (?, ?, ?)", (q["questid"], tg["step"], tg["target"]))
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {tb: sorted(db.execute(f"SELECT * FROM {tb}").fetchall(), key=repr) for tb in TABLES}
    db.close()
    return out


def run(*args):
    buf, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(err):
        code = t.main(list(args))
    return code, buf.getvalue() + err.getvalue()


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_dry_run_writes_nothing(self):
        before = dump(self.db)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("rows would change", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_once_then_undo(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT procanim FROM itempattern WHERE patternid = ?", (t.EYE,)), [(t.GEM_TYPE,)])
        for q in data()["quests"]:
            for st in q["steps"]:
                got = self.q("SELECT says, journal FROM queststeps WHERE questid = ? AND step = ?",
                             (q["questid"], st["step"]))
                self.assertEqual(got, [(st["new"]["says"], st["new"]["journal"])], (q["questid"], st["step"]))
        self.assertEqual(self.q("SELECT name FROM quests WHERE questid = 103523"), [("Further Training",)])
        self.assertTrue(self.q("SELECT count(*) FROM gm_audit")[0][0] >= 15)
        after = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied already", out)
        self.assertEqual(dump(self.db), after)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (t.LOG,)), [(0,)])

    def test_a_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itempattern SET procanim = 2 WHERE patternid = ?", (t.EYE,))
        db.commit()
        db.close()
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("procanim reads 2", out)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
