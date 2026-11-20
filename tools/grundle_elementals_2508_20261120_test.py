#!/usr/bin/env python3
"""grundle_elementals_2508_20261120.py: three place rows of no faction, a second apply does nothing, --undo takes them
out; an elemental missing from the cave refuses."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "grundle_elementals_2508_20261120.py")
SCHEMA = """CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', UNIQUE (name, world, x, z, radius));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""
CAVE = [(123690, "Water Elementalkin", 1, 5663.5, 2942.3), (123691, "Earth Elementalkin", 1, 5656.6, 2941.9),
        (123692, "Fire Elementalkin", 1, 5693.5, 2948.8)]


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def rows(p):
    db = sqlite3.connect(p)
    r = db.execute("SELECT name, world, faction FROM npcfaction ORDER BY name").fetchall()
    db.close()
    return r


class Grundle(unittest.TestCase):
    def make(self, d, npcs):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", npcs)
        db.commit()
        db.close()
        return p

    def test_rows(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, CAVE)
            self.assertEqual(run(p, "--apply").returncode, 0)
            self.assertEqual(rows(p), [("Earth Elementalkin", 1, ""), ("Fire Elementalkin", 1, ""), ("Water Elementalkin", 1, "")])
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(rows(p), [])

    def test_missing_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.make(d, CAVE[:2])
            self.assertEqual(run(p, "--apply").returncode, 1)
            self.assertEqual(rows(p), [])


if __name__ == "__main__":
    unittest.main()
