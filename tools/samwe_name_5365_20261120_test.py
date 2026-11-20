#!/usr/bin/env python3
"""samwe_name_5365_20261120.py: the report writes nothing, --apply renames row 106253, a second apply does nothing,
--undo puts it back; a row of another name is refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "samwe_name_5365_20261120.py")
SCHEMA = """CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def name(p):
    db = sqlite3.connect(p)
    n = db.execute("SELECT npc_name FROM npcs WHERE id = 106253").fetchone()[0]
    db.close()
    return n


class Samwe(unittest.TestCase):
    def make(self, d, n):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO npcs VALUES (106253, ?, 0)", (n,))
        db.commit()
        db.close()
        return p

    def test_rename(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, "Merchant Swame")
            run(p)
            self.assertEqual(name(p), "Merchant Swame")
            self.assertEqual(run(p, "--apply").returncode, 0)
            self.assertEqual(name(p), "Merchant Samwe")
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(name(p), "Merchant Swame")

    def test_other_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, "someone")
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(name(p), "someone")


if __name__ == "__main__":
    unittest.main()
