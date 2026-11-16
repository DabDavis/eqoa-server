#!/usr/bin/env python3
"""dust_twinkle_20261116.py on a small database: refused (nothing written) on a charaura without the second-aura
columns, with two Dusts, or with no slot 9 row; the report writes nothing; --apply sets the slot 9 row's second aura
alone (the first, the other slots and another character's row untouched), with its gm_audit line; a second apply finds
nothing to do; --undo puts the second back as it was; a row changed after --apply is left by --undo and listed."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "dust_twinkle_20261116.py")


def make(path, second_cols=True, twins=False, row9=True):
    db = sqlite3.connect(path)
    extra = ", color2 INTEGER NOT NULL DEFAULT 0, kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0, " \
            "nodes2 INTEGER NOT NULL DEFAULT 0" if second_cols else ""
    db.executescript(f"""
        CREATE TABLE characters (charName TEXT NOT NULL, accountid INTEGER NOT NULL, serverid INTEGER PRIMARY KEY, level INTEGER NOT NULL);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, model INTEGER);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, equiploc INTEGER);
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT ''{extra}, PRIMARY KEY (charid, merc, slot));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO characters VALUES ('Dust', 1, 197, 60), ('Zaeire', 107, 193, 60);
        INSERT INTO itempattern VALUES (714382, 'Heirloom Sword of Twisted Virtue', -1997797075);
        INSERT INTO charinventory VALUES (1, 197, 714382, 14);
        INSERT INTO charaura (charid, merc, slot, color, kind, source, nodes) VALUES (197, 0, 0, 4294967295, 5, 0, 0), (193, 0, 9, 0, 7, 1607773820, 0);
    """)
    if row9:
        db.execute("INSERT INTO charaura (charid, merc, slot, color, kind, source, nodes) VALUES (197, 0, 9, 255, 3, 0, 0)")
    if twins:
        db.execute("INSERT INTO characters VALUES ('Dust', 2, 900, 1)")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        rows = db.execute("SELECT * FROM charaura ORDER BY charid, slot").fetchall()
        audits = db.execute("SELECT action, tbl, rowkey FROM gm_audit ORDER BY id").fetchall()
        tables = [r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")]
        return rows, audits, tables
    finally:
        db.close()


class DustTwinkle(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_refusals(self):
        for kw, why in (({"second_cols": False}, "no color2"), ({"twins": True}, "want exactly serverid 197"), ({"row9": False}, "no charaura slot 9")):
            if os.path.exists(self.path):
                os.remove(self.path)
            make(self.path, **kw)
            was = state(self.path)
            r = run(self.path, "--apply")
            self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
            self.assertIn("REFUSED", r.stdout)
            self.assertIn(why, r.stdout)
            self.assertEqual(state(self.path), was)

    def test_apply_undo(self):
        make(self.path)
        was = state(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("report only", r.stdout)
        self.assertIn("Heirloom Sword of Twisted Virtue (714382)", r.stdout)
        self.assertEqual(state(self.path), was)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, audits, tables = state(self.path)
        self.assertIn((197, 0, 9, 255, 3, 0, 0, "", 0, 3, 0x1D875ECD, 100), rows)
        self.assertEqual([x for x in rows if x[:3] != (197, 0, 9)], [x for x in was[0] if x[:3] != (197, 0, 9)])
        self.assertEqual(audits, [("update", "charaura", '{"charid":197,"merc":0,"slot":9}')])
        self.assertIn("dust_twinkle_1116_log", tables)
        r = run(self.path, "--apply")
        self.assertIn("already the twinkle", r.stdout)
        self.assertIn("nothing to do", r.stdout)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rows, audits, tables = state(self.path)
        self.assertEqual(rows, was[0])
        self.assertEqual(len(audits), 2)
        self.assertNotIn("dust_twinkle_1116_log", tables)

    def test_undo_leaves_a_changed_row(self):
        make(self.path)
        run(self.path, "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charaura SET nodes2 = 80 WHERE charid = 197 AND slot = 9")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn("changed since", r.stdout)
        rows, _, tables = state(self.path)
        self.assertIn((197, 0, 9, 255, 3, 0, 0, "", 0, 3, 0x1D875ECD, 80), rows)
        self.assertIn("dust_twinkle_1116_log", tables)


if __name__ == "__main__":
    unittest.main()
