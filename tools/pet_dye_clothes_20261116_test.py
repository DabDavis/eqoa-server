#!/usr/bin/env python3
"""pet_dye_clothes_20261116.py on a small database: the report writes nothing; --apply takes the drachnid's row out with
a gm_audit line, another body's row and every petdyes row kept; a second apply finds nothing to do; --undo puts the row
back exactly. The control: a row a GM changed is refused, nothing written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "pet_dye_clothes_20261116.py")
JAREK = 0x15764DA8


def make(path, area1="Torso"):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE petdyeareas (look INTEGER PRIMARY KEY, area1 TEXT NOT NULL DEFAULT 'Body', area2 TEXT NOT NULL DEFAULT '',
            materials TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '');
        CREATE TABLE petdyes (serverid INTEGER NOT NULL, look INTEGER NOT NULL, area1 INTEGER NOT NULL DEFAULT 0,
            area2 INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', PRIMARY KEY (serverid, look));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO petdyeareas VALUES (1, 'Shell', 'Core', '2', 'a GM row');
        INSERT INTO petdyes VALUES (747, 360074664, 3523215615, 8388863, 'the Pet Groomer');
    """)
    db.execute("INSERT INTO petdyeareas VALUES (?, ?, 'Spider body', '0', 'Jarek''s Creation')", (JAREK, area1))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    try:
        return (db.execute("SELECT * FROM petdyeareas ORDER BY look").fetchall(), db.execute("SELECT * FROM petdyes").fetchall())
    finally:
        db.close()


class PetDyeClothes(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = dump(self.path)
        r = run(self.path)
        self.assertEqual((r.returncode, dump(self.path)), (0, before), r.stdout + r.stderr)
        self.assertIn("1 character(s)", r.stdout)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        areas, dyes = dump(self.path)
        self.assertEqual((areas, dyes), ([before[0][0]], before[1]), "the GM's row and the dyes kept")
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT action, tbl FROM gm_audit").fetchall(), [("delete", "petdyeareas")])
        db.close()
        self.assertIn("nothing to do", run(self.path, "--apply").stdout)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before)

    def test_a_changed_row_is_refused(self):
        make(self.path, area1="Plating")
        before = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.path), before)


if __name__ == "__main__":
    unittest.main()
