#!/usr/bin/env python3
"""starter_robes_5371_20261120.py: the three robes at 394 go to 0, another record is left, a second apply does nothing,
--undo puts the prices back; a robe at another price is refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "starter_robes_5371_20261120.py")
SCHEMA = """CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemcost INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""
ROWS = [(700168, "Thick Blue Uniform", 394), (5189, "Blue Student's Robe", 394), (700154, "Red Student's Robe", 394), (1, "x", 394)]


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def costs(p):
    db = sqlite3.connect(p)
    c = dict(db.execute("SELECT patternid, itemcost FROM itempattern"))
    db.close()
    return c


class Robes(unittest.TestCase):
    def make(self, d, rows):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?)", rows)
        db.commit()
        db.close()
        return p

    def test_free(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, ROWS)
            start = costs(p)
            self.assertEqual(run(p, "--apply").returncode, 0)
            self.assertEqual(costs(p), {700168: 0, 5189: 0, 700154: 0, 1: 394})
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(costs(p), start)

    def test_other_price_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, ROWS[:2] + [(700154, "Red Student's Robe", 500)])
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(costs(p)[5189], 394)


if __name__ == "__main__":
    unittest.main()
