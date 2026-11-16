#!/usr/bin/env python3
"""race_named_bodies_20261116.py on a small database: the report writes nothing; --apply gives the three race-named
rows their race's body and race, a gm_audit line each; a second --apply finds them done; a row a GM changed is refused
and left while the others are written; --undo puts back exactly what was written, a row changed since left."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import race_named_bodies_20261116 as rb  # noqa: E402

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL,
                   modelid INTEGER, race TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
                       username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL,
                       tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT,
                       detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


class RaceNamedBodiesTest(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        for npcid, name, world, x, z, guessed, _, _ in rb.ROWS:
            db.execute("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, NULL)", (npcid, name, world, x, z, guessed))
        db.execute("INSERT INTO npcs VALUES (9, 'troll villager', 0, 1, 1, ?, '5')", (rb.TROLL_MAN,))
        db.close()

    def tearDown(self):
        os.remove(self.path)

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = rb.main(["--db", self.path, *args])
        return code, out.getvalue()

    def rows(self):
        db = sqlite3.connect(self.path)
        try:
            return {r[0]: (r[1] & 0xFFFFFFFF, r[2]) for r in db.execute("SELECT id, modelid, race FROM npcs")}
        finally:
            db.close()

    def audits(self):
        db = sqlite3.connect(self.path)
        try:
            return db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0]
        finally:
            db.close()

    def test_report_apply_again_undo(self):
        before = self.rows()
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(self.rows(), before)
        self.assertEqual(self.audits(), 0)

        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        got = self.rows()
        self.assertEqual(got[3218], (rb.TROLL_MAN, "5"))
        self.assertEqual(got[3224], (rb.DWARF_MAN, "4"))
        self.assertEqual(got[3250], (rb.DARK_ELF_MAN, "2"))
        self.assertEqual(got[9], before[9])
        self.assertEqual(self.audits(), 3)

        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(out.count("already"), 3)
        self.assertEqual(self.audits(), 3)

        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.rows(), before)
        self.assertEqual(self.audits(), 6)

    def test_gm_changed_row_refused_others_written(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("UPDATE npcs SET modelid = 12345 WHERE id = 3224")
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("REFUSED: row 3224", out)
        got = self.rows()
        self.assertEqual(got[3224], (12345, None))
        self.assertEqual(got[3218], (rb.TROLL_MAN, "5"))
        self.assertEqual(self.audits(), 2)

    def test_undo_leaves_row_changed_since(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("UPDATE npcs SET race = '0' WHERE id = 3218")
        db.close()
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("row 3218: changed since", out)
        got = self.rows()
        self.assertEqual(got[3218], (rb.TROLL_MAN, "0"))
        self.assertEqual(got[3224], (rb.GOBLIN_MINER, None))


if __name__ == "__main__":
    unittest.main()
