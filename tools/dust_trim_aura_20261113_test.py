#!/usr/bin/env python3
"""dust_trim_aura_20261113.py on a small database: the report writes nothing; --apply makes itemaura (when the server
has not) and writes the five black trim rows, one changing a GM's row, each with its gm_audit line; a second apply is
refused; --undo puts back exactly what was there (the GM's row its values, the made rows gone, the table dropped when
this made it). The controls: an id that is not an Ornate piece of set 9 refuses the whole, writing nothing; a row
changed after --apply is left by --undo and listed."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "dust_trim_aura_20261113.py")
NAMES = ["Ornate Breastplate", "Ornate Vambraces", "Ornate Gauntlets", "Ornate Greaves", "Ornate Sabatons"]


def make(path, itemaura=False, bad=False):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, model INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    for i, n in enumerate(NAMES):
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?)", (711000 + i, n if not (bad and i == 3) else "Rusty Greaves", 9))
    if itemaura:
        db.executescript("""
            CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
                source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');
            INSERT INTO itemaura VALUES (711001, 4280295679, 2, 0, 0, 'a GM''s red shell'), (424242, 1, 3, 0, 0, 'another');
        """)
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        has = db.execute("SELECT 1 FROM sqlite_master WHERE name = 'itemaura'").fetchone()
        rows = db.execute("SELECT * FROM itemaura ORDER BY patternid").fetchall() if has else None
        audits = db.execute("SELECT action, tbl, rowkey, before, after FROM gm_audit ORDER BY id").fetchall()
        return rows, audits
    finally:
        db.close()


class DustTrimAura(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo_on_a_gms_row(self):
        make(self.path, itemaura=True)
        before = state(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 5 rows", r.stdout)
        self.assertEqual(state(self.path), before, "the report writes nothing")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, audits = state(self.path)
        ours = [x for x in rows if 711000 <= x[0] <= 711004]
        self.assertEqual([(x[0], x[1], x[2], x[3], x[4]) for x in ours], [(711000 + i, 0xFF, 5, 0, 0) for i in range(5)])
        self.assertIn((424242, 1, 3, 0, 0, "another"), rows, "another item's row untouched")
        self.assertEqual([a[0] for a in audits], ["insert", "update", "insert", "insert", "insert"])
        self.assertEqual(json.loads(audits[1][2]), {"patternid": 711001})
        self.assertEqual(json.loads(audits[1][3])["note"], "a GM's red shell")
        self.assertEqual(json.loads(audits[0][4])["kind"], 5)
        self.assertIn("applied already", run(self.path, "--apply").stdout)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, audits = state(self.path)
        self.assertEqual(rows, before[0], "exactly what was there")
        self.assertEqual([a[0] for a in audits[5:]], ["delete", "delete", "delete", "update", "delete"])

    def test_made_table_dropped_on_undo(self):
        make(self.path)
        self.assertEqual(run(self.path, "--apply").returncode, 0)
        rows, _ = state(self.path)
        self.assertEqual(len(rows), 5)
        self.assertEqual(run(self.path, "--undo").returncode, 0)
        self.assertIsNone(state(self.path)[0], "the table this made, dropped")

    def test_refused_when_not_the_ornate_plate(self):
        make(self.path, bad=True)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), (None, []), "nothing written")

    def test_a_row_changed_since_is_left(self):
        make(self.path)
        run(self.path, "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itemaura SET color = 4278190335 WHERE patternid = 711002")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn("711002: changed since", r.stdout)
        rows, _ = state(self.path)
        self.assertEqual([x[0] for x in rows], [711002], "the changed row left, the others back")


if __name__ == "__main__":
    unittest.main()
