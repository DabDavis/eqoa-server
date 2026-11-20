#!/usr/bin/env python3
"""rathe_gate_2507_20261120.py: both rows move 6 east, a second apply does nothing, --undo puts them back; a row
landing elsewhere is refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "rathe_gate_2507_20261120.py")
SCHEMA = """CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT, place TEXT, world INTEGER, x REAL, y REAL, z REAL, radius REAL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def rows(p):
    db = sqlite3.connect(p)
    r = db.execute("SELECT * FROM spellgates ORDER BY 1").fetchall()
    db.close()
    return r


class Gate(unittest.TestCase):
    def make(self, d, x2=6251.0):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO spellgates VALUES ('rathe mountains portal', 'group', 'Cyclops Fortress', 1, 6251.0, 65.2, 2703.0, 0)")
        db.execute("INSERT INTO spellgates VALUES ('circle of rathe', 'group', 'Cyclops Fortress', 1, ?, 65.2, 2703.0, 0)", (x2,))
        db.commit()
        db.close()
        return p

    def test_move(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d)
            start = rows(p)
            self.assertEqual(run(p, "--apply").returncode, 0)
            self.assertEqual({r[4] for r in rows(p)}, {6257.0})
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(rows(p), start)

    def test_elsewhere_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, x2=7000.0)
            start = rows(p)
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(rows(p), start)


if __name__ == "__main__":
    unittest.main()
