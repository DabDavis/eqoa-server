#!/usr/bin/env python3
"""goad_vendors_5370_20261120.py: Goad goes last into the four vendors' lists and its value to 3,086; a second apply
does nothing; --undo takes the rows out and puts the value back; a vendor of another name is refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "goad_vendors_5370_20261120.py")
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
    s = (db.execute("SELECT * FROM npcitem ORDER BY itemid").fetchall(), db.execute("SELECT * FROM itempattern").fetchall())
    db.close()
    return s


class Goad(unittest.TestCase):
    def make(self, d, gable="Merchant Gable"):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO itempattern VALUES (8704, 'Goad', 0)")
        db.executemany("INSERT INTO npcs VALUES (?, ?)", [(105836, gable), (579, "Merchant Harriston"), (101694, "Merchant Terk"), (599, "Merchant Scurver")])
        db.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (105836, 700124, 14)")
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
            self.assertEqual(db.execute("SELECT npcid, listnumber FROM npcitem WHERE patternid = 8704 ORDER BY npcid").fetchall(),
                             [(579, 0), (599, 0), (101694, 0), (105836, 15)])
            self.assertEqual(db.execute("SELECT itemcost FROM itempattern").fetchone(), (3086,))
            db.close()
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(state(p), start)

    def test_other_vendor_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, gable="someone")
            start = state(p)
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(state(p), start)


if __name__ == "__main__":
    unittest.main()
