#!/usr/bin/env python3
"""tools/fix_missing_effect_lines.py against a made-up database and made-up
lines: the rows it writes (a spell with no row, a row with an empty line a GM
gave a visual, a spell two classes share), the ones it leaves (a line that
exists, another class's spell of a name, another level's), the ones it holds
(a line cast at an enemy on a self record, a record whose description names a
proc, a line the file holds), a self line that costs its caster on a self
record (written), the spells no source covers, and --undo. No game data.

    python3 tools/fix_missing_effect_lines_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_missing_effect_lines.py")

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, scope INTEGER, spellname TEXT, spelldesc TEXT);
CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT, PRIMARY KEY (class, spellid));
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT);
"""

SHA, WIZ, DRD = 8, 13, 7


def expect(target, harmful=False):
    return {"target": target, "harmful": harmful, "seconds": 0, "effects": []}


LINES = {
    "lines": [
        {"classes": [SHA], "name": "Bolt of Test", "level": 4, "line": "Target, Decrease Hitpoints by 10",
         "source": "made up 1", "expect": expect("target", True)},
        {"classes": [SHA], "name": "Ward of Test", "level": 6, "line": "Target, Increase AC by 9, 30min",
         "source": "made up 2", "expect": expect("target")},
        {"classes": [SHA, WIZ], "name": "Veil of Test", "level": 8, "line": "Target, Invis, 5min",
         "source": "made up 3", "ours": "the 5min", "expect": expect("target")},
        {"classes": [SHA], "name": "Curse of Test", "level": 10, "line": "Target, Decrease STR by 5, 1min",
         "source": "made up 4", "expect": expect("target", True)},
        {"classes": [SHA], "name": "Thorns of Test", "level": 12, "line": "Self, Increase AC by 5, 5min",
         "source": "made up 5", "expect": expect("self")},
        {"classes": [SHA], "name": "Written Test", "level": 14, "line": "Self, Increase STR by 2, 5min",
         "source": "made up 6", "expect": expect("self")},
        {"classes": [SHA], "name": "Price of Test", "level": 20, "line": "Self, Decrease Hitpoints by 20, Increase Power by 5",
         "source": "made up 7", "expect": expect("self", True)},
        {"classes": [SHA], "name": "Echo of Test", "level": 22, "line": "Self, Increase HP per tic by 9, 24sec",
         "source": "made up 8", "hold": "a proc's effect, not a spell cast", "expect": expect("self")},
    ],
    "none": [
        {"classes": [SHA], "name": "Unknown Test", "level": 16, "why": "no source gives its numbers"},
    ],
}


def make(path, extra=""):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA + extra)
    db.executemany("INSERT INTO spellpattern VALUES (?, ?, ?, ?, ?)", [
        (1, 4, 1, "Bolt of Test", ""),              # no row: gets one
        (2, 6, 0, "Ward of Test", ""),              # an empty row a GM gave a visual: filled; scope 0 noted
        (3, 8, 1, "Veil of Test", ""),              # shared by two classes: one row
        (4, 10, 0, "Curse of Test", ""),            # harmful on a self record: held
        (5, 12, 0, "Thorns of Test", "Defensive Proc"),  # description names a proc: held
        (6, 14, 0, "Written Test", ""),             # a line exists: left
        (7, 4, 1, "Bolt of Test", ""),              # a druid's of the name: not the shaman's (control)
        (8, 5, 1, "Bolt of Test", ""),              # the shaman's of another level (control)
        (9, 16, 1, "Unknown Test", ""),             # no source covers it
        (10, 18, 1, "Stray Test", ""),              # no line and not in the file
        (11, 20, 0, "Price of Test", ""),           # harmful by its words, but a self line: written
        (12, 22, 0, "Echo of Test", ""),            # the file holds it
    ])
    db.executemany("INSERT INTO classspells VALUES (?, ?, 'test')",
                   [(SHA, 1), (SHA, 2), (SHA, 3), (WIZ, 3), (SHA, 4), (SHA, 5), (SHA, 6), (DRD, 7), (SHA, 8),
                    (SHA, 9), (SHA, 10), (SHA, 11), (SHA, 12)])
    db.executemany("INSERT INTO spelleffect VALUES (?, ?, ?, ?)", [
        (2, "", 3130913559, "visual set by GM someone, from the community's suggestions"),
        (6, "Self, Increase STR by 3, 5min", 0, "eqoa-data shaman"),
    ])
    db.execute("INSERT INTO gm_audit (action, tbl, rowkey, before, after) VALUES ('insert', 'spelleffect', '{\"spellid\":2}', NULL, "
               "'{\"fxdict\":3130913559,\"line\":\"\",\"spellid\":2}')")
    db.commit()
    db.close()


def run(db, lines, *args):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", db, "--lines", lines, *args],
                          capture_output=True, text=True, check=True).stdout


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


class FixMissingEffectLines(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        self.lines = os.path.join(self.dir.name, "lines.json")
        with open(self.lines, "w") as f:
            json.dump(LINES, f)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        make(self.db)
        before = dump(self.db)
        out = run(self.db, self.lines)
        self.assertIn("4 rows to write", out)
        self.assertIn("report only", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_then_undo(self):
        make(self.db)
        before = dump(self.db)
        out = run(self.db, self.lines, "--apply")
        r = rows(self.db)
        self.assertEqual(r[1], ("Target, Decrease Hitpoints by 10", 0, "made up 1, lines19"))
        self.assertEqual(r[2][0], "Target, Increase AC by 9, 30min")
        self.assertEqual(r[2][1], 3130913559, "the GM's visual is kept")
        self.assertTrue(r[2][2].startswith("visual set by GM someone"), "the GM's note is kept")
        self.assertIn("lands on its caster alone", out, "a target line on a self record is said")
        self.assertEqual(r[3], ("Target, Invis, 5min", 0, "made up 3; the 5min (ours), lines19"), "one row for two classes")
        self.assertNotIn(4, r, "a harmful line on a self record is held")
        self.assertIn("would land on its caster", out)
        self.assertNotIn(5, r, "a record whose description names a proc is held")
        self.assertIn("names a proc", out)
        self.assertEqual(r[6][0], "Self, Increase STR by 3, 5min", "a line that exists is never changed")
        self.assertNotIn(7, r, "another class's spell of the name is left alone")
        self.assertNotIn(8, r, "the spell of another level is left alone")
        self.assertEqual(r[11][0], "Self, Decrease Hitpoints by 20, Increase Power by 5", "a self line's own cost is not held")
        self.assertNotIn(12, r, "a line the file holds is never written")
        self.assertIn("held: a proc's effect", out)
        self.assertIn("Unknown Test (L16): no line written: no source gives its numbers", out)
        self.assertIn("Stray Test (L18): no line, and the file does not name it", out)
        self.assertIn("0 rows to write", run(self.db, self.lines), "a second run writes nothing")
        run(self.db, self.lines, "--undo")
        self.assertEqual(dump(self.db), before, "--undo puts the database back as it was")

    def test_held_line_written_once_the_record_is_mended(self):
        make(self.db)
        run(self.db, self.lines, "--apply")
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spellpattern SET scope = 1 WHERE spellid = 4")
        c.commit()
        c.close()
        before = dump(self.db)
        self.assertIn("1 rows to write", run(self.db, self.lines))
        run(self.db, self.lines, "--apply")
        self.assertEqual(rows(self.db)[4][0], "Target, Decrease STR by 5, 1min")
        run(self.db, self.lines, "--undo")
        self.assertNotIn(4, rows(self.db))
        self.assertNotIn(1, rows(self.db), "--undo takes back both runs' rows")

    def test_undo_leaves_what_changed_since(self):
        make(self.db)
        run(self.db, self.lines, "--apply")
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spelleffect SET line = 'Target, Decrease Hitpoints by 12' WHERE spellid = 1")  # a GM's line
        c.execute("UPDATE spelleffect SET fxdict = 305419896 WHERE spellid = 3")                       # a GM's visual
        c.commit()
        c.close()
        out = run(self.db, self.lines, "--undo")
        r = rows(self.db)
        self.assertEqual(r[1][0], "Target, Decrease Hitpoints by 12", "a line changed since is left")
        self.assertEqual(r[3][1], 305419896, "a row given a visual since is left")
        self.assertEqual(r[2][0], "", "a row as this left it is put back")
        self.assertIn("left as it is", out)
        c = sqlite3.connect(self.db)
        logged = [sid for (sid,) in c.execute("SELECT spellid FROM missing_line_log ORDER BY spellid")]
        c.close()
        self.assertEqual(logged, [1, 3], "what was left stays logged")

    def test_undo_after_the_table_was_replaced(self):
        # tools/import_spell_effects.py --apply replaces every row: what this
        # made is gone, and the row it filled is the importer's now
        make(self.db)
        run(self.db, self.lines, "--apply")
        c = sqlite3.connect(self.db)
        c.execute("DELETE FROM spelleffect")
        c.execute("INSERT INTO spelleffect VALUES (2, 'Target, Increase AC by 3, 30min', 0, 'eqoa-data shaman')")
        c.commit()
        c.close()
        out = run(self.db, self.lines, "--undo")
        self.assertIn("3 gone already", out)
        self.assertIn("1 left as they are", out)
        self.assertEqual(rows(self.db), {2: ("Target, Increase AC by 3, 30min", 0, "eqoa-data shaman")}, "the importer's row is left")
        c = sqlite3.connect(self.db)
        logged = [sid for (sid,) in c.execute("SELECT spellid FROM missing_line_log")]
        c.close()
        self.assertEqual(logged, [2], "only what was left stays logged")

    def test_a_line_a_gm_took_away_is_left(self):
        # a GM's delete of a row with a line, a GM's update emptying one, and
        # (control) a GM's delete of a row that had only a visual
        make(self.db, """
            INSERT INTO gm_audit (action, tbl, rowkey, before, after) VALUES ('delete', 'spelleffect', '{"spellid":1}',
                '{"fxdict":0,"line":"Target, Decrease Hitpoints by 9","spellid":1}', NULL);
            INSERT INTO gm_audit (action, tbl, rowkey, before, after) VALUES ('insert', 'spelleffect', '{"spellid":3}',
                NULL, '{"fxdict":305419896,"line":"","spellid":3}');
            INSERT INTO gm_audit (action, tbl, rowkey, before, after) VALUES ('delete', 'spelleffect', '{"spellid":3}',
                '{"fxdict":305419896,"line":"","spellid":3}', NULL);
        """)
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spelleffect SET source = 'a GM emptied it' WHERE spellid = 2")
        c.execute("INSERT INTO gm_audit (action, tbl, rowkey, before, after) VALUES ('update', 'spelleffect', '{\"spellid\":2}', "
                  "'{\"line\":\"Target, Increase AC by 1, 30min\",\"spellid\":2}', '{\"line\":\"\",\"spellid\":2}')")
        c.commit()
        c.close()
        out = run(self.db, self.lines, "--apply")
        r = rows(self.db)
        self.assertNotIn(1, r, "a line a GM deleted stays away")
        self.assertEqual(r[2][0], "", "a line a GM emptied stays empty")
        self.assertEqual(out.count("a GM took its line away"), 2)
        self.assertEqual(r[3], ("Target, Invis, 5min", 0, "made up 3; the 5min (ours), lines19"),
                         "a visual a GM took back is no line taken: written, with no visual")

if __name__ == "__main__":
    unittest.main()
