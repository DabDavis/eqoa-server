#!/usr/bin/env python3
"""petpass_bossloot_20261120.py on a small database: the report writes nothing; --apply bars the hand and shield import
copies (both shards) and Ascended records (Heirloom only) from the held drops with a gm_audit line each; a second run
writes nothing; --undo takes back exactly its rows and leaves a GM's own heldbarred row. Controls: a copy worn on the
chest is no held drop and is left; a copy already barred is left; the Ascended part is refused on the Live shard."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "petpass_bossloot_20261120.py")
SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, levelreq INTEGER, model INTEGER);
CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL, made TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '');
CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '');
CREATE TABLE heldbarred (patternid INTEGER PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO accountinfo VALUES (7, 'dab');
INSERT INTO itempattern VALUES (100, 'Pneumatic Staff', 14, 60, 11), (200, 'Pneumatic Staff', 14, 60, 11),
    (101, 'Two Horned Ripper', 15, 20, 12), (201, 'Two Horned Ripper', 15, 20, 12),
    (102, 'Kite', 13, 40, 13), (202, 'Kite', 13, 40, 13),
    (103, 'Vest', 1, 40, 14), (203, 'Vest', 1, 40, 14),
    (104, 'Blade', 12, 60, 15), (204, 'Ascended Blade', 12, 60, 15), (105, 'Robe', 1, 60, 16), (205, 'Ascended Robe', 1, 60, 16);
INSERT INTO itemimported (patternid, base) VALUES (200, 100), (201, 101), (202, 102), (203, 103);
INSERT INTO itemascended (patternid, base) VALUES (204, 104), (205, 105);
INSERT INTO heldbarred VALUES (100, 'a boss''s', 'gm'), (202, 'a GM''s own', 'gm');
"""


class BossLoot(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *args],
                              capture_output=True, text=True)

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def barred(self):
        return self.q("SELECT * FROM heldbarred ORDER BY patternid")

    def test_heirloom_report_apply_again_undo(self):
        was = self.barred()
        r = self.run_tool("--shard", "heirloom")
        self.assertIn("report only: 3 rows to write", r.stdout, r.stdout + r.stderr)
        self.assertEqual(self.barred(), was)
        r = self.run_tool("--shard", "heirloom", "--apply")
        self.assertIn("applied: 3 rows written", r.stdout, r.stdout + r.stderr)
        got = {p: s for p, _, s in self.barred()}
        # the hand copies 200, 201 and the Ascended hand record 204; not the chest's 203 or 205; 100 and 202 as they were
        self.assertEqual(sorted(got), [100, 200, 201, 202, 204])
        self.assertEqual([got[p] for p in (200, 201, 204)], ["petpass1120-bossloot"] * 3)
        self.assertEqual(got[202], "gm")
        self.assertEqual(self.q("SELECT COUNT(*), MIN(accountid), MIN(action) FROM gm_audit"), [(3, 7, "insert")])
        r = self.run_tool("--shard", "heirloom", "--apply")
        self.assertIn("applied: 0 rows written", r.stdout, r.stdout + r.stderr)
        r = self.run_tool("--shard", "heirloom", "--undo")
        self.assertIn("3 rows taken back, 0 left", r.stdout, r.stdout + r.stderr)
        self.assertEqual(self.barred(), was)
        self.assertEqual(self.q("SELECT COUNT(*) FROM sqlite_master WHERE name = 'petpass_bossloot_1120_log'"), [(0,)])
        r = self.run_tool("--shard", "heirloom", "--undo")
        self.assertIn("nothing to undo", r.stdout)

    def test_live_copies_only(self):
        r = self.run_tool("--shard", "live", "--apply")
        self.assertIn("applied: 2 rows written", r.stdout, r.stdout + r.stderr)
        self.assertEqual([p for p, _, _ in self.barred()], [100, 200, 201, 202])
        r = self.run_tool("--shard", "live", "--parts", "ascended")
        self.assertEqual(r.returncode, 2)
        self.assertIn("not for the live shard", r.stdout)

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--shard", "heirloom", "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE heldbarred SET note = 'a GM kept it' WHERE patternid = 201")
        db.commit()
        db.close()
        r = self.run_tool("--shard", "heirloom", "--undo")
        self.assertIn("2 rows taken back, 1 left", r.stdout, r.stdout + r.stderr)
        self.assertEqual([p for p, _, _ in self.barred()], [100, 201, 202])


if __name__ == "__main__":
    unittest.main()
