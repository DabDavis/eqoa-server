#!/usr/bin/env python3
"""tools/fix_enchanter_lines.py against a made-up database: the four rows it
fixes (a missing row, an empty one a GM gave a visual, two of the lists'
words), and the rows it must leave alone (another class's spell of a fixed
name, a line a GM wrote, a line that is not the expected one, a row another
source wrote). No game data.

    python3 tools/fix_enchanter_lines_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_enchanter_lines.py")

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, spellname TEXT);
CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT, rowkey TEXT);
"""

ENC, NEC = 12, 11


def make(path, extra=""):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA + extra)
    spells = [
        (516, 12, "Entrancing Charm"),          # no row: gets one
        (510, 8, "Mircyl's Animation"),         # an empty row a GM gave a visual: filled
        (700969, 39, "Anarchic Glyph"),         # the list's 15: 156
        (701218, 60, "Rune of Animation"),      # the list's words: ours
        (900001, 12, "Entrancing Charm"),       # a necromancer's of the name (control)
    ]
    db.executemany("INSERT INTO spellpattern VALUES (?, ?, ?)", spells)
    db.executemany("INSERT INTO classspells VALUES (?, ?, 'dump')",
                   [(ENC, 516), (ENC, 510), (ENC, 700969), (ENC, 701218), (NEC, 900001)])
    db.executemany("INSERT INTO spelleffect VALUES (?, ?, ?, ?)", [
        (510, "", 3130913559, "visual set by GM someone, from the community's suggestions"),
        (700969, "Group, Decrease Hitpoints by 15, 5min", 0, "eqoa-data enchanter (CM); made"),
        (701218, "Target, Weapon proc, 30min", 0, "eqoa-data enchanter (Animator MCA); made"),
    ])
    db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('spelleffect', '{\"spellid\":510}')")
    db.commit()
    db.close()


def run(db, *args):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", db, *args], capture_output=True, text=True, check=True).stdout


def rows(db):
    c = sqlite3.connect(db)
    out = {sid: (line, fx, src) for sid, line, fx, src in c.execute("SELECT spellid, line, fxdict, source FROM spelleffect")}
    c.close()
    return out


def dump(db):
    c = sqlite3.connect(db)
    out = "\n".join(c.iterdump())
    c.close()
    return out


class FixEnchanterLines(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        make(self.db)
        before = dump(self.db)
        out = run(self.db)
        self.assertIn("4 rows to change", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_then_undo(self):
        make(self.db)
        before = dump(self.db)
        run(self.db, "--apply")
        r = rows(self.db)
        self.assertEqual(r[516][0], "Target, Charm ; Increase 25% based on CHA")
        self.assertEqual(r[516][1], 0)
        self.assertEqual(r[510][0], "Summons a pet")
        self.assertEqual(r[510][1], 3130913559, "the GM's visual is kept")
        self.assertTrue(r[510][2].startswith("visual set by GM someone"), "the GM's note is kept")
        self.assertEqual(r[700969][0], "Group, Decrease Hitpoints by 156, 5min")
        self.assertIn("Decrease Hitpoints by 336 ; Increase 10% based on INT", r[701218][0])
        self.assertNotIn(900001, r, "another class's spell of the name is left alone")
        self.assertIn("0 rows to change", run(self.db), "a second run changes nothing")
        run(self.db, "--undo")
        self.assertEqual(dump(self.db), before, "--undo puts the database back as it was")

    def test_left_alone(self):
        # a line a GM wrote, a line not the expected one, a row another source wrote
        make(self.db, """
            INSERT INTO spelleffect VALUES (516, 'Target, Charm ; Increase 50% based on CHA', 0, 'gm');
            INSERT INTO gm_audit (tbl, rowkey) VALUES ('spelleffect', '{"spellid":516}');
        """)
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spelleffect SET line = 'Group, Decrease Hitpoints by 150, 5min' WHERE spellid = 700969")
        c.execute("UPDATE spelleffect SET source = 'another import of its own' WHERE spellid = 701218")
        c.commit()
        c.close()
        out = run(self.db, "--apply")
        r = rows(self.db)
        self.assertEqual(r[516][0], "Target, Charm ; Increase 50% based on CHA")
        self.assertEqual(r[700969][0], "Group, Decrease Hitpoints by 150, 5min")
        self.assertEqual(r[701218][0], "Target, Weapon proc, 30min")
        self.assertIn("another source wrote its row", out)
        self.assertEqual(r[510][0], "Summons a pet", "the one row it may still fill (control)")


if __name__ == "__main__":
    unittest.main()
