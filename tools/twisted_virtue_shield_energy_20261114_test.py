#!/usr/bin/env python3
"""twisted_virtue_shield_energy_20261114.py on a small database: the report writes nothing; --apply writes 714383's
itemaura row (kind 9, 0x2EC56029, 0xC10F0EFF) with a gm_audit line; a second apply finds nothing to do; --undo puts
back exactly what was there. The controls: no 714383, nothing (exit 0); 714383 another item, refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "twisted_virtue_shield_energy_20261114.py")


def make(path, shield="Heirloom Shield of Twisted Virtue"):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, model INTEGER);
        CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
            source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO itemaura VALUES (711000, 255, 5, 0, 0, 'Dust''s trim');
    """)
    if shield:
        db.execute("INSERT INTO itempattern VALUES (714383, ?, 13, 612804823)", (shield,))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        return (db.execute("SELECT * FROM itemaura ORDER BY patternid").fetchall(),
                db.execute("SELECT name FROM sqlite_master ORDER BY name").fetchall(),
                db.execute("SELECT count(*) FROM gm_audit").fetchone()[0])
    finally:
        db.close()


class ShieldEnergy(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = state(self.path)
        self.assertIn("report only", run(self.path).stdout)
        self.assertEqual(state(self.path), before)
        self.assertEqual(run(self.path, "--apply").returncode, 0)
        rows, _, audits = state(self.path)
        self.assertEqual(rows[1][:5], (714383, 0xC10F0EFF, 9, 0x2EC56029, 0))
        self.assertEqual((rows[0], audits), (before[0][0], 1), "Dust's trim untouched")
        self.assertIn("nothing to do", run(self.path, "--apply").stdout)
        self.assertEqual(run(self.path, "--undo").returncode, 0)
        self.assertEqual(state(self.path)[:2], before[:2])

    def test_no_shield_and_refused(self):
        make(self.path, shield=None)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual((r.returncode, "nothing to do" in r.stdout), (0, True))
        self.assertEqual(state(self.path), before)
        os.remove(self.path)
        make(self.path, shield="Rusty Buckler")
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
