#!/usr/bin/env python3
"""catapult_dye_20261118.py on a small database: the report writes nothing; --apply writes the catapult's petdyeareas
row (wood and stone / iron, materials 1) with a gm_audit line; a second run finds nothing to do; --undo removes it
exactly. The controls: another row for the catapult there, refused; the row changed since, --undo refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "catapult_dye_20261118.py")
SCHEMA = """
CREATE TABLE petdyeareas (look INTEGER PRIMARY KEY, area1 TEXT NOT NULL DEFAULT 'Body', area2 TEXT NOT NULL DEFAULT '',
    materials TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO petdyeareas VALUES (360074664, 'Torso', 'Spider body', '0', 'jarek');
"""
CAT = 0xE64B8D78


class CatapultDye(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        self.applied = os.path.join(self.dir.name, "a.json")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--applied", self.applied, *args], capture_output=True, text=True)

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_report_apply_again_undo(self):
        was = self.q("SELECT * FROM petdyeareas ORDER BY look")
        r = self.run_tool()
        self.assertIn("report only", r.stdout)
        self.assertEqual(self.q("SELECT * FROM petdyeareas ORDER BY look"), was)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.q("SELECT area1, area2, materials FROM petdyeareas WHERE look = ?", CAT), [("Wood and stone", "Iron", "1")])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 1)
        self.assertIn("nothing to do", self.run_tool().stdout)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.q("SELECT * FROM petdyeareas ORDER BY look"), was)

    def test_refused_over_another_row(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO petdyeareas VALUES (?, 'A', 'B', '2', 'gm')", (CAT,))
        db.commit()
        db.close()
        r = self.run_tool("--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(self.q("SELECT area1 FROM petdyeareas WHERE look = ?", CAT), [("A",)])

    def test_undo_refused_after_a_change(self):
        self.assertEqual(self.run_tool("--apply").returncode, 0)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE petdyeareas SET area2 = 'Metal' WHERE look = ?", (CAT,))
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(self.q("SELECT area2 FROM petdyeareas WHERE look = ?", CAT), [("Metal",)])


if __name__ == "__main__":
    unittest.main()
