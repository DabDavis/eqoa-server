#!/usr/bin/env python3
"""race_female_bodies2_20261112.py on a small database: the report writes nothing; --apply gives the goblin and the
gnoll their women's bodies with a gm_audit line each, leaving every other row; a second apply does nothing;
--undo puts racedefs back exactly. The controls: a row not as expected (another name, a female body of a GM's) is
refused, writing nothing; a row changed after the apply is left by the undo and said."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import race_female_bodies2_20261112 as rf  # noqa: E402

TOOL = os.path.join(HERE, "race_female_bodies2_20261112.py")
s32 = rf.s32


def make(path, gnoll_female=0xCF415561, goblin_name="Goblin"):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT NOT NULL, body_male INTEGER NOT NULL DEFAULT 0,
            body_female INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    rows = [(11, "Hill Giant", 0xFFB1EA2C, 0xF34501CB), (12, "Kerran", 0x22176258, 0xB749BF57),
            (13, goblin_name, 0x6C1308E0, 0x6C1308E0), (20, "Gnoll", 0xCF415561, gnoll_female),
            (15, "Lizardman", 0x911220E2, 0x911220E2)]
    db.executemany("INSERT INTO racedefs (id, name, body_male, body_female, note) VALUES (?, ?, ?, ?, 'n')",
                   [(i, n, s32(m), s32(f)) for i, n, m, f in rows])
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return list(db.execute("SELECT * FROM racedefs ORDER BY id"))
    finally:
        db.close()


def audits(path):
    db = sqlite3.connect(path)
    try:
        return db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
    finally:
        db.close()


def run(path, *args):
    r = subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)
    return r.returncode, r.stdout


def female(path, rid):
    db = sqlite3.connect(path)
    try:
        return db.execute("SELECT body_female FROM racedefs WHERE id = ?", (rid,)).fetchone()[0] & 0xFFFFFFFF
    finally:
        db.close()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.path)
        before = dump(self.path)
        rc, out = run(self.path)
        self.assertEqual(rc, 0, out)
        self.assertIn("2 rows would be written", out)
        self.assertEqual(dump(self.path), before, "the report writes nothing")
        rc, out = run(self.path, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(female(self.path, 13), 0xEDB4E0AD)
        self.assertEqual(female(self.path, 20), 0xF5F3A450)
        self.assertEqual(female(self.path, 12), 0xB749BF57, "the kerran left")
        self.assertEqual(female(self.path, 11), 0xF34501CB, "the giantess left")
        self.assertEqual(female(self.path, 15), 0x911220E2, "the lizardman left")
        self.assertEqual(audits(self.path), 2)
        after = dump(self.path)
        rc, out = run(self.path, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("applied already", out)
        self.assertEqual(dump(self.path), after)
        self.assertEqual(audits(self.path), 2)
        rc, out = run(self.path, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(dump(self.path), before, "the undo puts racedefs back exactly")
        self.assertEqual(audits(self.path), 4)
        rc, out = run(self.path, "--undo")
        self.assertIn("nothing to undo", out)

    def test_refused(self):
        make(self.path, gnoll_female=0x12345678)  # a GM's choice
        before = dump(self.path)
        rc, out = run(self.path, "--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("refused", out)
        self.assertEqual(dump(self.path), before)
        os.remove(self.path)
        make(self.path, goblin_name="Hobgoblin")
        rc, out = run(self.path, "--apply")
        self.assertEqual(rc, 1, out)
        self.assertEqual(audits(self.path), 0)

    def test_undo_leaves_a_changed_row(self):
        make(self.path)
        run(self.path, "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE racedefs SET body_female = 7 WHERE id = 20")
        db.commit()
        db.close()
        rc, out = run(self.path, "--undo")
        self.assertEqual(rc, 1, out)
        self.assertIn("left: racedefs 20", out)
        self.assertEqual(female(self.path, 13), 0x6C1308E0)
        self.assertEqual(female(self.path, 20), 7)


if __name__ == "__main__":
    unittest.main()
