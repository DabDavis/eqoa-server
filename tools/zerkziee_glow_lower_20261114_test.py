#!/usr/bin/env python3
"""zerkziee_glow_lower_20261114.py on a small database: the report writes nothing; --apply sets both hand rows'
nodes to 45 (a gm_audit line each), every other column and row as they were; a second apply finds nothing to do;
--undo puts back exactly what was there. The controls: no druid staff glow rows, refused, nothing written; --along 30."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "zerkziee_glow_lower_20261114.py")
STAFF = 0xCBF4135A


def make(path, glow=True):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY);
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', PRIMARY KEY (charid, merc, slot));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO characters VALUES ('Zerkziee', 747), ('Zaeire', 193);
        INSERT INTO charaura VALUES (193, 0, 9, 0, 7, 1607773820, 0, 'Zaeire''s');
    """)
    if glow:
        db.execute("INSERT INTO charaura VALUES (747, 0, 9, 0, 7, ?, 0, 'n'), (747, 0, 10, 0, 7, ?, 0, 'n')", (STAFF, STAFF))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        return (db.execute("SELECT * FROM charaura ORDER BY charid, slot").fetchall(),
                db.execute("SELECT name FROM sqlite_master ORDER BY name").fetchall(),
                db.execute("SELECT count(*) FROM gm_audit").fetchone()[0])
    finally:
        db.close()


class ZerkzieeGlowLower(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = state(self.path)
        self.assertIn("report only: 2 rows", run(self.path).stdout)
        self.assertEqual(state(self.path), before)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, _, audits = state(self.path)
        self.assertEqual(rows, [(193, 0, 9, 0, 7, 1607773820, 0, "Zaeire's"), (747, 0, 9, 0, 7, STAFF, 45, "n"), (747, 0, 10, 0, 7, STAFF, 45, "n")])
        self.assertEqual(audits, 2)
        self.assertIn("nothing to do", run(self.path, "--apply").stdout)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path)[:2], before[:2], "exactly what was there")

    def test_along_and_refused(self):
        make(self.path)
        self.assertEqual(run(self.path, "--along", "30", "--apply").returncode, 0)
        self.assertEqual([r[6] for r in state(self.path)[0] if r[0] == 747], [30, 30])
        os.remove(self.path)
        make(self.path, glow=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
