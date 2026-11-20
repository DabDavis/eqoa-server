#!/usr/bin/env python3
"""predator_look_2301_20261120.py: the sprite swapped, a second apply does nothing, --undo puts it back; another
spell refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "predator_look_2301_20261120.py")
SCHEMA = """CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, effects TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""
FX = '{"effects":[{"id":"0920","type":"Alter Appearance","kind":"illusion","sprite":"0xF23F517F"},{"id":"0921"}]}'


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def fx(p):
    db = sqlite3.connect(p)
    v = db.execute("SELECT effects FROM spelleffect").fetchone()[0]
    db.close()
    return v


class Wolf(unittest.TestCase):
    def make(self, d, name="Form of the Predator"):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO spellpattern VALUES (700783, ?)", (name,))
        db.execute("INSERT INTO spelleffect VALUES (700783, ?)", (FX,))
        db.commit()
        db.close()
        return p

    def test_swap(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d)
            self.assertEqual(run(p, "--apply").returncode, 0)
            self.assertIn('"sprite":"0x7F513FF2"', fx(p))
            self.assertNotIn("0xF23F517F", fx(p))
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(fx(p), FX)

    def test_other_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, "Something")
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(fx(p), FX)


if __name__ == "__main__":
    unittest.main()
