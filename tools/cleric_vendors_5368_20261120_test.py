#!/usr/bin/env python3
"""cleric_vendors_5368_20261120.py: the eight scrolls go last into each of the four vendors' lists and Cure Mystical
Plague's value to 4,589; a second apply does nothing; --undo puts everything back; a vendor stocking one is refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "cleric_vendors_5368_20261120.py")
sys.path.insert(0, HERE)
import cleric_vendors_5368_20261120 as tool  # noqa: E402

SCHEMA = """CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemcost INTEGER NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER NOT NULL DEFAULT 0,
    remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def state(p):
    db = sqlite3.connect(p)
    s = (db.execute("SELECT * FROM npcitem ORDER BY itemid").fetchall(), db.execute("SELECT * FROM itempattern ORDER BY 1").fetchall())
    db.close()
    return s


class Cleric(unittest.TestCase):
    def make(self, d, extra=None):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        for pid, name in tool.SCROLLS.items():
            db.execute("INSERT INTO itempattern VALUES (?, ?, ?)", (pid, name, 0 if pid == 8792 else 5000))
        db.executemany("INSERT INTO npcs VALUES (?, ?)", list(tool.VENDORS.items()))
        db.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (1935, 700049, 10)")
        if extra:
            db.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (?, ?, 11)", extra)
        db.commit()
        db.close()
        return p

    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d)
            start = state(p)
            r = run(p, "--apply")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            db = sqlite3.connect(p)
            self.assertEqual(db.execute("SELECT COUNT(*) FROM npcitem").fetchone()[0], 1 + 8 * 6)
            self.assertEqual(db.execute("SELECT MIN(listnumber), MAX(listnumber) FROM npcitem WHERE npcid = 1935 AND patternid <> 700049").fetchone(), (11, 18))
            self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 8792").fetchone(), (4589,))
            self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 716259").fetchone(), (5000,))
            db.close()
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(state(p), start)

    def test_stocked_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, extra=(106020, 716259))
            start = state(p)
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(state(p), start)

    def test_full_list_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d)
            db = sqlite3.connect(p)
            db.executemany("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (162430, ?, ?)", [(9000 + i, i) for i in range(33)])
            db.commit()
            db.close()
            r = run(p, "--apply")
            self.assertEqual(r.returncode, 1)
            self.assertIn("past the client's 40", r.stdout)


if __name__ == "__main__":
    unittest.main()
