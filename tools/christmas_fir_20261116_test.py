#!/usr/bin/env python3
"""christmas_fir_20261116.py on a small database: the report writes nothing; --apply puts the Lightwolf fir (0xC4BE0726,
zone 24, sunk 11) in the tree's decorart row and re-fits its four light strings, with gm_audit lines; a second --apply
refuses; --undo puts the rows back as they were."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "christmas_fir_20261116.py")
SCHEMA = """
CREATE TABLE decorart (name TEXT PRIMARY KEY, model INTEGER NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT 0,
    zone INTEGER NOT NULL DEFAULT 0, sink REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE lightstrings (id INTEGER PRIMARY KEY, template TEXT NOT NULL DEFAULT '', npc TEXT NOT NULL DEFAULT '',
    world INTEGER NOT NULL DEFAULT 0, shape TEXT NOT NULL DEFAULT 'line', params TEXT NOT NULL DEFAULT '{}',
    season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
    via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
    before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


def dump(path):
    db = sqlite3.connect(path)
    out = [db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("decorart", "lightstrings")]
    db.close()
    return out


class ChristmasFirTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "fir.json")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO decorart VALUES ('Christmas Tree', 238125619, 0, 32, 13.0, 'the green fir', 'christmas-2026')")
        for i, y in enumerate([0.3, 4.6, 12.5, 18.5], 1):
            p = {"r0": 10.0, "y0": y, "r1": 3.0, "y1": y + 4, "turns": 2.0}
            if i == 4:
                p["top"] = "FFE070"
            db.execute("INSERT INTO lightstrings (id, template, npc, shape, params, season, source) VALUES (?, 'Christmas lights', "
                       "'Christmas Tree', 'spiral', ?, 'Christmas', 'christmas-2026')", (i, json.dumps(p)))
        db.commit()
        db.close()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, TOOL, "--db", self.db, "--applied", self.applied, *args], capture_output=True, text=True)

    def test_apply_and_undo(self):
        before = dump(self.db)
        self.assertEqual(self.run_tool().returncode, 0)
        self.assertEqual(dump(self.db), before, "the report writes nothing")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT model, zone, sink FROM decorart").fetchone(), (0xC4BE0726, 24, 11.0))
        ps = [json.loads(p) for (p,) in db.execute("SELECT params FROM lightstrings ORDER BY id")]
        self.assertEqual([p["y0"] for p in ps], [0.9, 6.6, 14.5, 20.5])
        self.assertEqual(ps[3].get("top"), "FFE070", "the star kept")
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 5)
        db.close()
        self.assertNotEqual(self.run_tool("--apply").returncode, 0, "a second --apply refuses")
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(dump(self.db), before, "undone: the rows as they were")


if __name__ == "__main__":
    unittest.main()
