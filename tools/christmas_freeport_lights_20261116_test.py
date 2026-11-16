#!/usr/bin/env python3
"""christmas_freeport_lights_20261116.py on a small database: the report writes nothing; --apply writes one world path a
string (template Christmas lights, of the season), each its fitted points; a second --apply refuses; --undo deletes them."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "christmas_freeport_lights_20261116.py")
SCHEMA = """
CREATE TABLE lighttemplates (name TEXT PRIMARY KEY COLLATE NOCASE, colors TEXT NOT NULL DEFAULT 'FFFFFF', note TEXT NOT NULL DEFAULT '');
CREATE TABLE lightstrings (id INTEGER PRIMARY KEY, template TEXT NOT NULL DEFAULT '', npc TEXT NOT NULL DEFAULT '',
    world INTEGER NOT NULL DEFAULT 0, shape TEXT NOT NULL DEFAULT 'line', params TEXT NOT NULL DEFAULT '{}',
    season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
    via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
    before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


class FreeportLightsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "fl.json")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO lighttemplates (name) VALUES ('Christmas lights')")
        db.execute("INSERT INTO lightstrings (template, npc, shape, params, season, source) VALUES ('Christmas lights', 'Christmas Tree', 'path', '{}', 'Christmas', 'christmas-2026')")
        db.commit()
        db.close()

    def rows(self):
        db = sqlite3.connect(self.db)
        r = db.execute("SELECT * FROM lightstrings ORDER BY id").fetchall()
        db.close()
        return r

    def run_tool(self, *args):
        return subprocess.run([sys.executable, TOOL, "--db", self.db, "--applied", self.applied, *args], capture_output=True, text=True)

    def test_apply_and_undo(self):
        before = self.rows()
        self.assertEqual(self.run_tool().returncode, 0)
        self.assertEqual(self.rows(), before)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        rows = self.rows()[1:]
        n = len(json.load(open(os.path.join(HERE, "christmas_freeport_lights_20261116.json")))["strings"])
        self.assertEqual(len(rows), n)
        for _, tpl, npc, world, shape, params, season, note, src in rows:
            self.assertEqual((tpl, npc, world, shape, season, src), ("Christmas lights", "", 0, "path", "Christmas", "christmas-2026"))
            pts = json.loads(params)["points"]
            self.assertGreater(len(pts), 5)
            self.assertTrue(all(25000 < p[0] < 25600 and 15300 < p[2] < 15900 for p in pts), note)
        self.assertNotEqual(self.run_tool("--apply").returncode, 0)
        self.assertEqual(self.run_tool("--undo").returncode, 0)
        self.assertEqual(self.rows(), before)


if __name__ == "__main__":
    unittest.main()
