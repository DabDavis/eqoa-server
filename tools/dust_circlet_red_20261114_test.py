#!/usr/bin/env python3
"""dust_circlet_red_20261114.py on a small database: the report writes nothing; --apply sets Dust's circlet row to
nodes 72 and colour 0xE01818FF (a gm_audit line), his other rows untouched; a second apply finds nothing to do; --undo
puts back exactly what was there. The controls: no circlet row, refused, nothing written; no Dust, nothing (exit 0)."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "dust_circlet_red_20261114.py")
STAFF = 0x1D875ECD


def make(path, circlet=True, dust=True):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY);
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', PRIMARY KEY (charid, merc, slot));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO charaura VALUES (197, 0, 0, 4294967295, 5, 0, 0, '');
    """)
    if dust:
        db.execute("INSERT INTO characters VALUES ('Dust', 197)")
    if circlet:
        db.execute("INSERT INTO charaura VALUES (197, 0, 5, 0, 8, ?, 0, 'n')", (STAFF,))
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


class DustCircletRed(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = state(self.path)
        self.assertIn("report only: 1 rows", run(self.path).stdout)
        self.assertEqual(state(self.path), before)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, _, audits = state(self.path)
        self.assertEqual(rows, [(197, 0, 0, 4294967295, 5, 0, 0, ""), (197, 0, 5, 0xE01818FF, 8, STAFF, 72, "n")])
        self.assertEqual(audits, 1)
        self.assertIn("nothing to do", run(self.path, "--apply").stdout)
        self.assertEqual(run(self.path, "--undo").returncode, 0)
        self.assertEqual(state(self.path)[:2], before[:2], "exactly what was there")

    def test_refused_and_no_dust(self):
        make(self.path, circlet=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), before)
        os.remove(self.path)
        make(self.path, dust=False)
        r = run(self.path, "--apply")
        self.assertEqual((r.returncode, "no Dust on this shard" in r.stdout), (0, True))


if __name__ == "__main__":
    unittest.main()
