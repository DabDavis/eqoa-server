#!/usr/bin/env python3
"""Tests for gnoll_5524_20261123.py on a small database holding quest 100780 as read on both shards: a report
writes nothing; --apply makes step 5 the tunnel's reach (a popup and the gnoll's 10-minute spawn), moves steps 5-9 to
6-10 and characters past step 5 one step on (one done, on step 5 or before it left), and makes the gnoll spawn-only;
a second --apply writes nothing; a quest not as read is refused; --undo puts it all back."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import gnoll_5524_20261123 as t  # noqa: E402


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
            target TEXT NOT NULL DEFAULT '', items TEXT NOT NULL DEFAULT '', world INTEGER NOT NULL DEFAULT 0,
            x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0,
            radius REAL NOT NULL DEFAULT 15, journal TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '',
            PRIMARY KEY (questid, step));
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
            z_coord REAL);
        CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER DEFAULT 0,
            PRIMARY KEY (serverid, questid));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT,
            via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
    """)
    for step, kind, target in t.AS_READ:
        db.execute("INSERT INTO queststeps (questid, step, kind, target) VALUES (?, ?, ?, ?)", (t.QUEST, step, kind, target))
    db.execute("INSERT INTO npcs VALUES (103714, ?, 0, 12949.2, -24.0, 12537.0)", (t.NAME,))
    db.executemany("INSERT INTO charquests VALUES (?, ?, ?, ?)",
                   [(1, t.QUEST, 4, 0), (2, t.QUEST, 5, 0), (3, t.QUEST, 6, 0), (4, t.QUEST, 9, 1), (5, t.QUEST, 8, 0)])
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = t.main(list(args))
    return code, out.getvalue()


def snap(path):
    db = sqlite3.connect(path)
    out = {tbl: db.execute(f"SELECT * FROM {tbl} ORDER BY 1, 2").fetchall()
           for tbl in ("queststeps", "npcs", "npcspawnonly", "charquests")}
    db.close()
    return out


class GnollTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        before = snap(self.path)
        code, out = run("--db", self.path)
        self.assertEqual(code, 0, out)
        self.assertEqual(snap(self.path), before)

    def test_apply_then_undo(self):
        before = snap(self.path)
        code, out = run("--db", self.path, "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        steps = db.execute("SELECT step, kind, target FROM queststeps ORDER BY step").fetchall()
        self.assertEqual(steps, t.APPLIED)
        self.assertEqual(steps[5], (6, "loot", t.NAME))
        x, z, ev = db.execute("SELECT x, z, events FROM queststeps WHERE step = 5").fetchone()
        self.assertEqual((x, z), (12949.2, 12537.0))
        ev = json.loads(ev)
        self.assertEqual([e["do"] for e in ev], ["popup", "spawn"])
        self.assertEqual((ev[1]["npc"], ev[1]["hostile"], ev[1]["minutes"]), (t.NAME, True, 10))
        self.assertEqual(db.execute("SELECT serverid, step, done FROM charquests ORDER BY 1").fetchall(),
                         [(1, 4, 0), (2, 5, 0), (3, 7, 0), (4, 9, 1), (5, 9, 0)])
        self.assertEqual(db.execute("SELECT name FROM npcspawnonly").fetchall(), [(t.NAME,)])
        db.close()
        code, out = run("--db", self.path, "--apply")
        self.assertIn("already applied", out)
        code, out = run("--db", self.path, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(snap(self.path), before)

    def test_a_quest_not_as_read_is_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE queststeps SET target = 'someone' WHERE step = 3")
        db.commit()
        db.close()
        before = snap(self.path)
        code, out = run("--db", self.path, "--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(snap(self.path), before)


if __name__ == "__main__":
    unittest.main()
