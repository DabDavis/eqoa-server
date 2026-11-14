#!/usr/bin/env python3
"""dust_circlet_20261114.py on a small database: the report writes nothing; --apply writes Dust's helm-slot row (kind 8,
the Ascendant staff's model) with a gm_audit line, his trim auras untouched; a second apply finds nothing to do; --undo
puts back exactly what was there. The controls: no Dust, nothing (exit 0); no staff record, refused, nothing written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "dust_circlet_20261114.py")
STAFF = 0x1D875ECD


def make(path, dust=True, staff=True):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY, level INTEGER);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, model INTEGER);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, equiploc INTEGER);
        CREATE TABLE charhiddenlooks (serverid INTEGER PRIMARY KEY, locs INTEGER);
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', PRIMARY KEY (charid, merc, slot));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO itempattern VALUES (714043, 'Heirloom Helm', 4);
        INSERT INTO charaura VALUES (197, 0, 0, 4294967295, 5, 0, 0, '');
    """)
    if dust:
        db.executescript("INSERT INTO characters VALUES ('Dust', 197, 60); INSERT INTO charinventory VALUES (1809, 197, 714043, 0);"
                         "INSERT INTO charhiddenlooks VALUES (197, 1);")
    if staff:
        db.execute("INSERT INTO itempattern VALUES (5340, 'Ascendant staff', ?)", (STAFF,))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        return (db.execute("SELECT * FROM charaura ORDER BY charid, slot").fetchall(),
                db.execute("SELECT name FROM sqlite_master ORDER BY name").fetchall(),
                db.execute("SELECT action FROM gm_audit ORDER BY id").fetchall())
    finally:
        db.close()


class DustCirclet(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = state(self.path)
        r = run(self.path)
        self.assertIn("Heirloom Helm (hidden: the circlet shows)", r.stdout)
        self.assertEqual(state(self.path), before)
        self.assertEqual(run(self.path, "--apply").returncode, 0)
        rows, _, audits = state(self.path)
        self.assertIn((197, 0, 5, 0, 8, STAFF, 0, "Dust's monk circlet: the Ascendant staff's ring round his head, the owner 2026-11-14"), rows)
        self.assertIn((197, 0, 0, 4294967295, 5, 0, 0, ""), rows, "his trim aura untouched")
        self.assertEqual(audits, [("insert",)])
        self.assertIn("nothing to do", run(self.path, "--apply").stdout)
        self.assertEqual(run(self.path, "--undo").returncode, 0)
        self.assertEqual(state(self.path)[:2], before[:2])

    def test_no_dust_and_refused(self):
        make(self.path, dust=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual((r.returncode, "no Dust on this shard" in r.stdout), (0, True))
        self.assertEqual(state(self.path), before)
        os.remove(self.path)
        make(self.path, staff=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
