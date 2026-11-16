#!/usr/bin/env python3
"""hoofbite_look_20261116.py on a small database: the report writes nothing; --apply writes the hat (charaura slot 5,
kind 10) and, with no instanceaura table, skips the weapons; a second --apply after instanceaura appears writes the two
swords' copies alone; a third does nothing; --undo deletes exactly what was written; a GM's helm row or a sword not his
is refused, nothing written."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import hoofbite_look_20261116 as hl  # noqa: E402

SCHEMA = """
CREATE TABLE characters (charName TEXT UNIQUE NOT NULL, serverid INTEGER PRIMARY KEY, level INTEGER, race INTEGER,
                         tclass INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, patternid INTEGER NOT NULL,
                            equiploc INTEGER NOT NULL);
CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
                       color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
                       nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', color2 INTEGER NOT NULL DEFAULT 0,
                       kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0, nodes2 INTEGER NOT NULL DEFAULT 0,
                       PRIMARY KEY (charid, merc, slot));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
                       username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL,
                       tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT,
                       detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""

INSTANCE = """
CREATE TABLE instanceaura (itemid INTEGER PRIMARY KEY, tag TEXT NOT NULL DEFAULT '', color INTEGER NOT NULL DEFAULT 0,
    kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0,
    color2 INTEGER NOT NULL DEFAULT 0, kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0,
    nodes2 INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source_tool TEXT NOT NULL DEFAULT '');
"""


class HoofbiteLookTest(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO characters VALUES ('Hoofbite', 213, 60, 7, 6)")
        db.execute("INSERT INTO characters VALUES ('Dust', 197, 60, 2, 4)")
        for itemid, pid, _, _, _ in hl.SWORDS:
            db.execute("INSERT INTO charinventory VALUES (?, 213, ?, ?)", (itemid, pid, 14 if pid == 960260 else 15))
        db.execute("INSERT INTO charaura (charid, slot, color, kind) VALUES (197, 5, 4278190335, 8)")  # someone else's
        db.close()

    def tearDown(self):
        os.remove(self.path)

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = hl.main(["--db", self.path, *args])
        return code, out.getvalue()

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_report_apply_skip_then_weapons_then_undo(self):
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("SKIPPED", out)
        self.assertEqual(self.q("SELECT COUNT(*) FROM charaura WHERE charid = 213"), [(0,)], "the report writes nothing")
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT slot, color, kind, source, nodes FROM charaura WHERE charid = 213"), [(5, 0, 10, 0xF0FAB933, 0)])
        self.assertEqual(self.q("SELECT tbl, action FROM gm_audit"), [("charaura", "insert")])
        self.assertEqual(self.run_tool("--apply")[1].splitlines()[-1], "nothing to do")
        # the instance-aura server has started: the swords' copies, the hat left
        db = sqlite3.connect(self.path)
        db.executescript(INSTANCE)
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT itemid, tag, color, kind, source FROM instanceaura ORDER BY itemid"),
                         [(14767, "Virtuous", 0x00008CFF, 7, 0x88EC092D), (14768, "Virtuous", 0x4B0082FF, 7, 0x88EC092D)])
        self.assertEqual(self.q("SELECT COUNT(*) FROM gm_audit"), [(3,)])
        self.assertEqual(self.run_tool("--apply")[1].splitlines()[-1], "nothing to do")
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT COUNT(*) FROM charaura WHERE charid = 213"), [(0,)])
        self.assertEqual(self.q("SELECT COUNT(*) FROM instanceaura"), [(0,)])
        self.assertEqual(self.q("SELECT COUNT(*) FROM charaura WHERE charid = 197"), [(1,)], "another's row untouched")
        self.assertEqual(self.q("SELECT COUNT(*) FROM sqlite_master WHERE name = ?", hl.LOG), [(0,)])

    def test_a_gms_helm_row_or_a_sword_not_his_is_refused(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("INSERT INTO charaura (charid, slot, color, kind) VALUES (213, 5, 4278190335, 2)")
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(self.q("SELECT kind FROM charaura WHERE charid = 213"), [(2,)])
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("DELETE FROM charaura WHERE charid = 213")
        db.executescript(INSTANCE)
        db.execute("UPDATE charinventory SET serverid = 197 WHERE itemid = 14768")
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertEqual(self.q("SELECT COUNT(*) FROM charaura WHERE charid = 213"), [(0,)], "nothing written")
        self.assertEqual(self.q("SELECT COUNT(*) FROM instanceaura"), [(0,)])
        self.assertEqual(self.q("SELECT COUNT(*) FROM gm_audit"), [(0,)])


if __name__ == "__main__":
    unittest.main()
