#!/usr/bin/env python3
"""armory_items_20261121.py: apply, a second apply, undo, suspects, --skip, copies, and rows not as read."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import armory_items_20261121 as tool  # noqa: E402

PLAN = {"entries": [
    {"name": "Etheric Ring", "fields": {"AC": {"guide": 15, "read": [10]}, "FR": {"guide": 15, "read": [0]},
                                        "AR": {"guide": 250, "read": [25], "suspect": "ten times ours"}}},
    {"name": "Robe of Tests", "fields": {"HoT": {"guide": 3, "read": [0, 1]}}},
    {"name": "Changed Belt", "fields": {"str": {"guide": 25, "read": [20]}, "AC": {"guide": 25, "read": [20]}}},
    {"name": "Skipped Cap", "fields": {"AC": {"guide": 21, "read": [20]}}},
    {"name": "Epic Shroud", "fields": {"AC": {"guide": 140, "read": [85], "prior": "heirloom 2026-11-11 tool"}}},
]}


def make(path):
    db = sqlite3.connect(path)
    cols = ", ".join(f"{c} INTEGER" for c in tool.COLUMNS)
    db.execute(f"CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT NOT NULL, {cols})")
    db.execute("CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT, source TEXT)")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
               "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)")
    for pid, name, vals in [(1, "etheric ring", {"AC": 10, "FR": None, "AR": 25}),
                            (2, " Etheric ring", {"AC": 15, "FR": 0, "AR": 25}),
                            (990001, "etheric ring", {"AC": 10, "FR": 0, "AR": 25}),
                            (3, "Robe of Tests", {"HoT": 1}),
                            (4, "Changed Belt", {"str": 22, "AC": 20}),
                            (5, "Skipped Cap", {"AC": 20}),
                            (6, "Ascended Etheric Ring", {"AC": 11}),
                            (7, "Epic Shroud", {"AC": 85})]:
        db.execute("INSERT INTO itempattern (patternid, itemname) VALUES (?, ?)", (pid, name))
        for c, v in vals.items():
            db.execute(f"UPDATE itempattern SET {c} = ? WHERE patternid = ?", (v, pid))
    db.execute("INSERT INTO itemimported VALUES (990001, 1, '', '')")
    db.commit()
    db.close()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "i.db")
        self.plan = os.path.join(self.dir.name, "plan.json")
        with open(self.plan, "w") as f:
            json.dump(PLAN, f)
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return tool.main(["--db", self.path, "--plan", self.plan, *args])

    def row(self, pid, *cols):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        finally:
            db.close()

    def audits(self):
        db = sqlite3.connect(self.path)
        try:
            return db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        finally:
            db.close()

    def test_report_only_writes_nothing(self):
        self.run_tool()
        self.assertEqual(self.row(1, "AC", "FR"), (10, None))
        self.assertEqual(self.audits(), 0)

    def test_apply_again_undo(self):
        self.run_tool("--apply", "--skip", "skipped cap")
        self.assertEqual(self.row(1, "AC", "FR", "AR"), (15, 15, 25))  # the suspect AR left out
        self.assertEqual(self.row(2, "AC", "FR", "AR"), (15, 15, 25))
        self.assertEqual(self.row(990001, "AC"), (10,))  # an imported copy: left to the server's start
        self.assertEqual(self.row(6, "AC"), (11,))  # another name
        self.assertEqual(self.row(3, "HoT"), (3,))
        self.assertEqual(self.row(4, "str", "AC"), (22, 20))  # str not as read: the record refused whole
        self.assertEqual(self.row(5, "AC"), (20,))  # skipped
        self.assertEqual(self.row(7, "AC"), (85,))  # set on purpose before: left out
        self.assertEqual(self.audits(), 3)
        self.run_tool("--apply", "--skip", "Skipped Cap")
        self.assertEqual(self.audits(), 3)
        self.run_tool("--undo")
        self.assertEqual(self.row(1, "AC", "FR"), (10, None))
        self.assertEqual(self.row(2, "AC", "FR"), (15, 0))
        self.assertEqual(self.row(3, "HoT"), (1,))
        self.assertEqual(self.audits(), 6)

    def test_with_suspect(self):
        self.run_tool("--apply", "--with-suspect", "--with-prior")
        self.assertEqual(self.row(7, "AC"), (140,))
        self.assertEqual(self.row(1, "AR"), (250,))
        self.assertEqual(self.row(5, "AC"), (21,))

    def test_undo_leaves_a_changed_figure(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET AC = 40 WHERE patternid = 1")
        db.commit()
        db.close()
        self.run_tool("--undo")
        self.assertEqual(self.row(1, "AC", "FR"), (40, None))


if __name__ == "__main__":
    unittest.main()
