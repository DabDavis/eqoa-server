#!/usr/bin/env python3
"""quest_items_unlore_20261121.py on a made database: the dry run reports and writes nothing; --apply takes Lore off
the quest stacks alone (not gear, a single item, a reward alone, an item no quest names, an event's key) with a
gm_audit line each; a second --apply does nothing; --undo puts back what is still as the tool left it."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "quest_items_unlore_20261121.py")
SCHEMA = """CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, lore INTEGER, maxstack INTEGER,
    equipslot INTEGER);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, items TEXT NOT NULL DEFAULT '',
    rewarditems TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);"""
ITEMS = [
    (1, "eye of a madman", 1, 999, -1),       # a loot step's stack: changed
    (2, "oilstone", 1, 20, -1),               # a give step's stack: changed
    (3, "glowing orb", 1, 50, 0),             # a show step's stack, slot 0: changed
    (4, "blood of the desert madman", 0, 1, -1),  # the owner's example: no Lore, no stack: nothing to do
    (5, "Ring of Lore", 1, 5, 7),             # gear: left
    (6, "the one tome", 1, 1, -1),            # a single item: left
    (7, "reward gem", 1, 20, -1),             # a reward alone: left
    (8, "bone chips", 1, 20, -1),             # no quest names it: left
    (9, "Tallon's Key", 1, 999, -1),          # an event's key: left
]
STEPS = [
    (10, 1, "loot", "1*3", ""),
    (10, 2, "give", "1*3, 2, 4", "7"),
    (11, 1, "show", "3", ""),
    (11, 2, "give", "5, 6x1", ""),
    (12, 1, "talk", "8", ""),               # a talk step's items count for nothing
    (13, 1, "show", "9", ""),
]


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def lore(p):
    db = sqlite3.connect(p)
    v = dict(db.execute("SELECT patternid, lore FROM itempattern"))
    db.close()
    return v


def audits(p):
    db = sqlite3.connect(p)
    v = db.execute("SELECT accountid, username, via, ip, action, tbl, rowkey, before, after, detail FROM gm_audit ORDER BY id").fetchall()
    db.close()
    return v


class Unlore(unittest.TestCase):
    def make(self, d):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)", ITEMS)
        db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?)", STEPS)
        db.commit()
        db.close()
        return p

    def test_dry_apply_again_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d)
            before = lore(p)
            r = run(p)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("report only", r.stdout)
            self.assertIn("3 Lore stacks", r.stdout)
            self.assertEqual(lore(p), before)
            self.assertEqual(audits(p), [])

            r = run(p, "--apply")
            self.assertEqual(r.returncode, 0, r.stderr)
            got = lore(p)
            self.assertEqual({k for k in got if got[k] != before[k]}, {1, 2, 3})
            self.assertEqual([got[k] for k in (1, 2, 3)], [0, 0, 0])
            a = audits(p)
            self.assertEqual(len(a), 3)
            for row in a:
                self.assertEqual(row[:6], (1, "dab", "tool", "", "update", "itempattern"))
                self.assertEqual((row[7], row[8]), ('{"lore":1}', '{"lore":0}'))
                self.assertIn("the owner 2026-11-21: quest stacks without lore", row[9])
            self.assertEqual(sorted(row[6] for row in a), ['{"patternid":1}', '{"patternid":2}', '{"patternid":3}'])

            r = run(p, "--apply")
            self.assertIn("nothing to do", r.stdout)
            self.assertEqual(len(audits(p)), 3)

            # one changed by hand since: left by the undo
            db = sqlite3.connect(p)
            db.execute("UPDATE itempattern SET lore = 1 WHERE patternid = 3")
            db.commit()
            db.close()
            r = run(p, "--undo")
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("put back 2, 1 changed since", r.stdout)
            self.assertEqual(lore(p), before)
            self.assertEqual(len(audits(p)), 5)
            self.assertIn("nothing to undo", run(p, "--undo").stdout)


if __name__ == "__main__":
    unittest.main()
