#!/usr/bin/env python3
"""construct_quad_20261117.py on a small database: the report writes nothing; --apply writes the 7 rows (the template,
three pettemplate rows, three petstats rows), a gm_audit line each; a second apply does nothing; --undo puts every table
back exactly. --base-only: Construct alone (3 rows). The controls: a petstats row not as the import left it, a
pettemplate row of another's for "construct", Tinker Kick's recast not 12, are refused, writing nothing."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import construct_quad_20261117 as cq  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, recast INTEGER NOT NULL, spellname TEXT NOT NULL);
CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', ar INTEGER NOT NULL DEFAULT 0,
    abilities TEXT NOT NULL DEFAULT '[]');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, abilities TEXT NOT NULL DEFAULT '[]',
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE pettemplate (spell TEXT NOT NULL, weapon INTEGER NOT NULL DEFAULT 0, template TEXT NOT NULL DEFAULT '',
    swing_class INTEGER NOT NULL DEFAULT -1, swing INTEGER NOT NULL DEFAULT -1, round_ms INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (spell, weapon));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO spellpattern VALUES (902052, 12, 'Tinker Kick');
INSERT INTO pettemplate (spell, template) VALUES ('fire elemental', 'pet fire');
INSERT INTO classtemplates (name, class) VALUES ('pet fire', 0);
"""

# as the 2026-11-13 import left them (both shards' rows, read 2026-11-17)
ROWS = [
    (901835, "Construct", 95, '[{"every":12.0,"first":1.0,"kind":"taunt","hits":[[1,0,0]]},{"every":12.0,"first":3.0,"kind":"nuke","spell_id":902052}]'),
    (901836, "Construct", 91, '[{"every":15.0,"first":1.0,"kind":"nuke","spell_id":902053},{"every":6.0,"first":3.0,"kind":"nuke","spell_id":902054}]'),
    (901837, "Construct", 91, '[{"every":6.0,"first":1.0,"kind":"nuke","spell_id":902055},{"every":12.0,"first":3.0,"kind":"nuke","spell_id":902056}]'),
]
TABLES = ("petstats", "classtemplates", "pettemplate")


class ConstructQuad(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO petstats VALUES (?, ?, ?, ?)", ROWS)
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = cq.main(["--db", self.path, *args])
        return code, out.getvalue()

    def dump(self):
        db = sqlite3.connect(self.path)
        try:
            return {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in TABLES}
        finally:
            db.close()

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_apply_and_undo(self):
        was = self.dump()
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("7 rows would be written", out)
        self.assertEqual(self.dump(), was)
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT spell, template, swing_class, swing, round_ms FROM pettemplate WHERE template = 'pet construct' ORDER BY 1"),
                         [("construct", "pet construct", -1, 5, 0), ("construct (drachnid)", "pet construct", -1, 5, 0),
                          ("construct (rat)", "pet construct", -1, 5, 0)])
        self.assertEqual(self.q("SELECT class, hp_pct, taken_pct, melee_pct, abilities FROM classtemplates WHERE name = 'pet construct'"),
                         [(0, 100, 100, 100, "[]")])
        rows = dict((s, (ar, ab)) for s, ar, ab in self.q("SELECT spellid, ar, abilities FROM petstats"))
        self.assertEqual({s: ar for s, (ar, _) in rows.items()}, {901835: 315, 901836: 315, 901837: 315})
        kick = '{"every":12.0,"first":0.0,"kind":"nuke","spell_id":902052}'
        self.assertEqual(rows[901835][1], '[{"every":12.0,"first":1.0,"kind":"taunt","hits":[[1,0,0]]},' + kick + "]")
        for s in (901836, 901837):
            self.assertTrue(rows[s][1].endswith("," + kick + "]"), rows[s][1])
            self.assertEqual(rows[s][1].count("902052"), 1)
        self.assertEqual(self.q("SELECT COUNT(*) FROM gm_audit")[0][0], 7)
        code, out = self.run_tool("--apply")
        self.assertIn("applied already", out)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.dump(), was)
        self.assertEqual(self.q("SELECT COUNT(*) FROM sqlite_master WHERE name = ?", cq.LOG)[0][0], 0)

    def test_base_only(self):
        code, out = self.run_tool("--base-only", "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT spell FROM pettemplate WHERE template = 'pet construct'"), [("construct",)])
        self.assertEqual(self.q("SELECT spellid FROM petstats WHERE ar = 315"), [(901835,)])

    def test_refusals(self):
        for sql in ("UPDATE petstats SET ar = 120 WHERE spellid = 901836",
                    "INSERT INTO pettemplate (spell, template) VALUES ('construct', 'pet earth')",
                    "UPDATE spellpattern SET recast = 6 WHERE spellid = 902052"):
            with self.subTest(sql=sql):
                self.setUp()
                db = sqlite3.connect(self.path)
                db.execute(sql)
                db.commit()
                db.close()
                was = self.dump()
                code, out = self.run_tool("--apply")
                self.assertEqual(code, 1, out)
                self.assertIn("refused", out)
                self.assertEqual(self.dump(), was)
                self.assertEqual(self.q("SELECT COUNT(*) FROM gm_audit")[0][0], 0)


if __name__ == "__main__":
    unittest.main()
