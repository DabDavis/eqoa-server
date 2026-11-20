#!/usr/bin/env python3
"""leaf_jerkin_2511_20261120.py: the blank record becomes the druid's leather tunic and held copies get item HP 200;
a second apply does nothing; --undo puts both back; a changed record refuses."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "leaf_jerkin_2511_20261120.py")
sys.path.insert(0, HERE)
import leaf_jerkin_2511_20261120 as tool  # noqa: E402

COLS = list(tool.OLD)
SCHEMA = f"""CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, {', '.join(c + ' INTEGER' for c in COLS)});
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, remainHP INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def state(p):
    db = sqlite3.connect(p)
    s = (db.execute("SELECT * FROM itempattern").fetchall(), db.execute("SELECT * FROM charinventory ORDER BY 1").fetchall())
    db.close()
    return s


class Jerkin(unittest.TestCase):
    def make(self, d, rec):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.execute(f"INSERT INTO itempattern VALUES (8490, 'leaf covered jerkin', {', '.join('?' * len(COLS))})", list(rec.values()))
        db.executemany("INSERT INTO charinventory VALUES (?, ?, 8490, 0)", [(1, 1399), (2, 1827)])
        db.commit()
        db.close()
        return p

    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, tool.OLD)
            start = state(p)
            self.assertEqual(run(p, "--apply").returncode, 0)
            rec, inv = state(p)
            self.assertEqual(rec[0][2:], tuple(tool.NEW.values()))
            self.assertEqual([r[3] for r in inv], [200, 200])
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(state(p), start)

    def test_changed_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, dict(tool.OLD, AC=9))
            start = state(p)
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(state(p), start)


if __name__ == "__main__":
    unittest.main()
