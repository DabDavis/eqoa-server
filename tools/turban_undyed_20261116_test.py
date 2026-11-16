#!/usr/bin/env python3
"""turban_undyed_20261116.py on a small database: the report writes nothing; --apply sets the turban's colour white,
one gm_audit line; a second --apply finds it white; --undo puts the crimson back. The controls: a colour a GM changed,
refused; no turban, refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "turban_undyed_20261116.py")
SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, color INTEGER, model INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


class TurbanUndyed(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO itempattern VALUES (714388, 'Turban of Twisted Virtue', 3238989567, 1)")
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, *args], capture_output=True, text=True)

    def q(self, sql):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql).fetchall()
        finally:
            db.close()

    def test_apply_undo(self):
        self.assertIn("report only", self.run_tool().stdout)
        self.assertEqual(self.q("SELECT color FROM itempattern"), [(3238989567,)])
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.q("SELECT color FROM itempattern"), [(4294967295,)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(1,)])
        self.assertIn("nothing to do", self.run_tool("--apply").stdout)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.q("SELECT color FROM itempattern"), [(3238989567,)])
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'turban_undyed_1116_log'"), [(0,)])

    def test_a_gms_colour_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET color = 255")
        db.commit()
        db.close()
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertEqual(self.q("SELECT color FROM itempattern"), [(255,)])

    def test_no_turban_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM itempattern")
        db.commit()
        db.close()
        self.assertEqual(self.run_tool("--apply").returncode, 1)


if __name__ == "__main__":
    unittest.main()
