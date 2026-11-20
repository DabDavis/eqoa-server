#!/usr/bin/env python3
"""monk_races_2501_20261120.py: a monk-only human-only record becomes every race's; a record of another class mask or a
race mask already wider is left; a second apply does nothing; --undo puts the masks back."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "monk_races_2501_20261120.py")
SCHEMA = """CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER NOT NULL, raceuse INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def masks(p):
    db = sqlite3.connect(p)
    m = dict(db.execute("SELECT patternid, raceuse FROM itempattern"))
    db.close()
    return m


class Monk(unittest.TestCase):
    def test_masks(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "eqoa.db")
            db = sqlite3.connect(p)
            db.executescript(SCHEMA)
            db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?)", [
                (1, "Mirewalker Robe", 16, 1), (2, "a warrior's thing", 1, 1), (3, "Monk wide", 16, 1023), (4, "Monk+war", 17, 1)])
            db.commit()
            db.close()
            start = masks(p)
            self.assertEqual(run(p, "--apply").returncode, 0)
            self.assertEqual(masks(p), {1: 32767, 2: 1, 3: 1023, 4: 1})
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(masks(p), start)


if __name__ == "__main__":
    unittest.main()
