#!/usr/bin/env python3
"""christmas_lights3_20261116.py on a small database: the report writes nothing; --apply raises the tree (decorart sink
11 -> SINK), turns its four spirals into fitted paths, adds the fifth garland with the star and the presents, with gm_audit
lines; a second --apply refuses; --undo puts every row back."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "christmas_lights3_20261116.py")
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


class ChristmasLights3Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "l3.json")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO decorart VALUES ('Christmas Tree', 3300788006, 0, 24, 11.0, 'the Lightwolf fir', 'christmas-2026')")
        for i in range(1, 5):
            p = {"r0": 10.0, "y0": i, "r1": 3.0, "y1": i + 4, "turns": 2.0}
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
        self.assertLess(db.execute("SELECT sink FROM decorart").fetchone()[0], 11.0, "raised")
        rows = db.execute("SELECT shape, params FROM lightstrings ORDER BY id").fetchall()
        self.assertEqual([s for s, _ in rows], ["path"] * 5 + ["presents"])
        self.assertTrue(all(len(json.loads(p)["points"]) > 20 for s, p in rows[:5]))
        self.assertIn("star", json.loads(rows[4][1]))
        boxes = json.loads(rows[5][1])["boxes"]
        self.assertTrue(6 <= len(boxes) <= 10)
        self.assertGreaterEqual(len({b["color"] for b in boxes}), 6, "six colours")
        self.assertTrue(any(b["at"][1] > 0 for b in boxes), "some stacked")
        db.close()
        self.assertNotEqual(self.run_tool("--apply").returncode, 0)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(dump(self.db), before, "undone")


if __name__ == "__main__":
    unittest.main()
