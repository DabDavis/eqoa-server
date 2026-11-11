#!/usr/bin/env python3
"""cleave_cone_20261111.py on a small database: the report writes nothing; --apply writes Cleave's cone row (adding
spellarea.cone to a table made before it) with its gm_audit line; a second apply is refused; --undo puts spellarea's
rows back exactly. The controls: a spellarea row for Cleave already there, or a spell 700534 not named Cleave, is
refused, writing nothing."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import cleave_cone_20261111 as cc  # noqa: E402

TOOL = os.path.join(HERE, "cleave_cone_20261111.py")


def make(path, name="Cleave", cone=False):
    db = sqlite3.connect(path)
    db.executescript(f"""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, scope INTEGER);
        CREATE TABLE spellarea (spellid INTEGER PRIMARY KEY, centre INTEGER NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0,
            maxtargets INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT ''{", cone REAL NOT NULL DEFAULT 0" if cone else ""});
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO spellpattern VALUES (700534, '{name}', 1), (760139, 'Warchief''s Cleave', 0), (700145, 'Fire Storm', 1);
        INSERT INTO spellarea (spellid, centre, radius, maxtargets, note) VALUES (700145, 1, 25, 0, 'TB AoE');
    """)
    db.commit()
    db.close()


def rows(path):
    db = sqlite3.connect(path)
    try:
        return [r[:5] for r in db.execute("SELECT * FROM spellarea ORDER BY spellid")]
    finally:
        db.close()


class CleaveConeTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def test_apply_and_undo(self):
        for cone in (False, True):
            with self.subTest(cone_column=cone):
                if os.path.exists(self.path):
                    os.remove(self.path)
                make(self.path, cone=cone)
                before = rows(self.path)
                code, out = self.run_tool()
                self.assertEqual(code, 0, out)
                self.assertIn("report only", out)
                self.assertEqual(rows(self.path), before, "the report wrote")
                code, out = self.run_tool("--apply")
                self.assertEqual(code, 0, out)
                db = sqlite3.connect(self.path)
                got = cc.row_of(db, 700534)
                self.assertEqual({k: got[k] for k in cc.ROW}, cc.ROW)
                self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail = ?", (cc.DETAIL,)).fetchone()[0], 1)
                db.close()
                code, out = self.run_tool("--apply")
                self.assertNotEqual(code, 0)
                self.assertIn("applied already", out)
                code, out = self.run_tool("--undo")
                self.assertEqual(code, 0, out)
                self.assertEqual(rows(self.path), before, "the undo")
                db = sqlite3.connect(self.path)
                self.assertFalse(cc.table(db, cc.LOG))
                self.assertTrue(cc.has_column(db, "spellarea", "cone"), "the undo took the column the server reads")
                db.close()

    def test_refusals(self):
        make(self.path, cone=True)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO spellarea (spellid, centre) VALUES (700534, 2)")
        db.commit()
        db.close()
        before = rows(self.path)
        code, out = self.run_tool("--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("has a row already", out)
        self.assertEqual(rows(self.path), before)
        os.remove(self.path)
        make(self.path, name="Bash")
        code, out = self.run_tool("--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("not 'Cleave'", out)
        db = sqlite3.connect(self.path)
        self.assertFalse(cc.has_column(db, "spellarea", "cone"), "a refusal added the column")
        db.close()


if __name__ == "__main__":
    unittest.main()
