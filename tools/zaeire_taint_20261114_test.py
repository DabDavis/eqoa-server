#!/usr/bin/env python3
"""zaeire_taint_20261114.py on a small database: the report writes nothing; --apply writes Zaeire's two hands' rows
(kind 7, the sickle's model), one changing a GM's row, each with its gm_audit line; a second apply finds nothing to do;
--undo puts back exactly what was there (the GM's row its values, the made row gone). The controls: another character's
row untouched; two Zaeires, or no sickle record, refuse the whole, writing nothing; a row changed after --apply is left
by --undo and listed."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "zaeire_taint_20261114.py")
SICKLE = 0x5FD4AE7C


def make(path, gm_row=False, twins=False, sickle=True):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, accountid INTEGER NOT NULL, serverid INTEGER PRIMARY KEY, level INTEGER NOT NULL);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, model INTEGER);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, equiploc INTEGER);
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', PRIMARY KEY (charid, merc, slot));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO characters VALUES ('Zaeire', 107, 193, 60), ('Dust', 1, 12, 60);
        INSERT INTO itempattern VALUES (714100, 'Heirloom Dagger of Bloodletting', 3174841800);
        INSERT INTO charinventory VALUES (1, 193, 714100, 14), (2, 193, 714100, 15);
        INSERT INTO charaura VALUES (12, 0, 9, 4280295679, 3, 0, 0, 'Dust''s red sparkles');
    """)
    if sickle:
        db.execute("INSERT INTO itempattern VALUES (990436, 'Sickle of Avhi Escron', ?)", (SICKLE,))
    if twins:
        db.execute("INSERT INTO characters VALUES ('Zaeire', 108, 194, 1)")
    if gm_row:
        db.execute("INSERT INTO charaura VALUES (193, 0, 10, 812711935, 3, 0, 0, 'a GM''s blue sparkles')")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        rows = db.execute("SELECT * FROM charaura ORDER BY charid, slot").fetchall()
        audits = db.execute("SELECT action, tbl, rowkey, before, after FROM gm_audit ORDER BY id").fetchall()
        tables = [r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")]
        return rows, audits, tables
    finally:
        db.close()


class ZaeireTaint(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo_over_a_gms_row(self):
        make(self.path, gm_row=True)
        before = state(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 2 rows", r.stdout)
        self.assertIn("equiploc 14 Heirloom Dagger of Bloodletting (0xBD3C3DC8)", r.stdout)
        self.assertEqual(state(self.path), before, "the report writes nothing")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, audits, _ = state(self.path)
        ours = [x[:7] for x in rows if x[0] == 193]
        self.assertEqual(ours, [(193, 0, 9, 0, 7, SICKLE, 0), (193, 0, 10, 0, 7, SICKLE, 0)])
        self.assertIn((12, 0, 9, 4280295679, 3, 0, 0, "Dust's red sparkles"), rows, "another character's row untouched")
        self.assertEqual([a[0] for a in audits], ["insert", "update"])
        self.assertEqual(json.loads(audits[1][2]), {"charid": 193, "merc": 0, "slot": 10})
        self.assertEqual(json.loads(audits[1][3])["note"], "a GM's blue sparkles")
        self.assertEqual((json.loads(audits[0][4])["kind"], json.loads(audits[0][4])["source"]), (7, SICKLE))
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0)
        self.assertIn("nothing to do", r.stdout, "a second apply")
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, audits, tables = state(self.path)
        self.assertEqual((rows, tables), (before[0], before[2]), "exactly what was there")
        self.assertEqual([a[0] for a in audits[2:]], ["update", "delete"])

    def test_refused_with_two_zaeires_or_no_sickle(self):
        for kw in ({"twins": True}, {"sickle": False}):
            make(self.path, **kw)
            before = state(self.path)
            r = run(self.path, "--apply")
            self.assertEqual(r.returncode, 1, kw)
            self.assertIn("REFUSED", r.stdout)
            self.assertEqual(state(self.path), before, "nothing written")
            os.remove(self.path)

    def test_a_row_changed_since_is_left(self):
        make(self.path)
        run(self.path, "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charaura SET color = 4278190335 WHERE charid = 193 AND slot = 9")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn("193 slot 9: changed since", r.stdout)
        rows, _, _ = state(self.path)
        self.assertEqual([(x[0], x[2]) for x in rows], [(12, 9), (193, 9)], "the changed row left, the other back")


if __name__ == "__main__":
    unittest.main()
