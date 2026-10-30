#!/usr/bin/env python3
"""tools/fix_spell_visuals_sandstorm.py against a made-up database and recordings: a recorded
spell takes Sandstorm's visual (found by its id, by ID_OFFSET, or given a new row); an
unrecorded rank of its line takes it when it lies between two recorded ranks with that
visual, or is another record of a recorded spell; with --whole-lines a line whose recorded
ranks agree gives it to its ranks beyond them too, and a line whose ranks step through
visuals (as Sandstorm's taps do) never does. The controls are left: the same icon and scope
but no line, another scope, another class, a rank word ("Greater") as the only shared word,
a visual a GM set, a spell whose id Sandstorm gives another spell. A second --apply is
refused, --undo gives back the table as it was (the row it made deleted), and a row changed
since the apply is left by --undo. No game data.

    python3 tools/fix_spell_visuals_sandstorm_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_spell_visuals_sandstorm.py")

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, icon INTEGER, scope INTEGER, abilitylvl INTEGER);
CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""
SKIN, HEAL, BARD, TAP = 1920746744, -2105187409, 2151457732, 2036326070
BUFF, MINOR_HEAL, BARB = 0x1DAEB5B6, 0xC2B700FA, 0x725CFDB8
TAP1, TAP2 = 0xAF72B893, 0xAF72B894

SPELLS = [
    # id, name, icon, scope, level, class
    (342, "Woodskin", SKIN, 1, 1, 7),
    (700100, "Barkskin", SKIN, 1, 8, 7),         # between two recorded ranks: takes BUFF
    (353, "Rockskin", SKIN, 1, 12, 7),
    (700267, "Steelskin", SKIN, 1, 24, 7),       # beyond the recorded ranks: --whole-lines only
    (700423, "Diamondskin", SKIN, 1, 39, 7),     # beyond too
    (700110, "Ironskin", SKIN, 1, 6, 7),         # between, but a GM's visual: left
    (700083, "Thistleshield", SKIN, 1, 8, 7),    # below Barbshield, its line's only record: --whole-lines only
    (700090, "Thornward", SKIN, 1, 9, 7),        # same icon and scope, no line: left
    (300357, "Barbshield", SKIN, 1, 16, 7),      # Sandstorm's 357, by ID_OFFSET
    (700357, "Barbshield", SKIN, 1, 15, 7),      # a second of the name: only ID_OFFSET finds 300357; its twin
    (700120, "Brawn", SKIN, 1, 6, 7),            # shares one letter's ending with the skins: left
    (357, "Rapid Strike", 0, 1, 13, 7),          # our 357, another spell: left
    (700834, "Group Woodskin", SKIN, 2, 1, 7),   # another scope: left
    (457, "Minor Healing", HEAL, 1, 1, 7),
    (461, "Light Healing", HEAL, 1, 12, 7),      # between: takes MINOR_HEAL
    (464, "Healing", HEAL, 1, 20, 7),
    (700307, "Greater Healing", HEAL, 1, 29, 7), # beyond: --whole-lines only
    (458, "Minor Curative", HEAL, 1, 4, 7),      # same icon, another line: left
    (700376, "Greater Curative", HEAL, 1, 34, 7),  # shares only a rank's word: left
    (880, "Life Tap", TAP, 1, 3, 11),
    (890, "Life Draw", TAP, 1, 8, 11),           # between two TAP1s: takes TAP1
    (903, "Life Siphon", TAP, 1, 16, 11),
    (915, "Life Spike", TAP, 1, 24, 11),         # between TAP1 and TAP2: left
    (937, "Life Bane", TAP, 1, 34, 11),
    (953, "Life Drain", TAP, 1, 44, 11),         # beyond a line that steps: left, --whole-lines too
    (143, "Chant of Battle", BARD, 2, 3, 5),     # no spelleffect row: a new one
    (800001, "Steelskin", SKIN, 1, 24, 12),      # another class's: left
    (800002, "Bronzeskin", SKIN, 1, 6, 12),      # another class's, at a level between: left
]
ROWS = [(342, "Target, Increase AC by 12", 0x9999, "eqoa-data druid (Merchant)")] + [
    (s, "a line", 0x11111111 if s == 700110 else 0, "eqoa-data") for s, *_ in SPELLS if s not in (342, 143)]
RECORDINGS = """# Sandstorm's visuals (made up)
ssid\tname\tclass\tlevel\tscope\tvisual\tcast_ms\tcasts\tnote
342\tWoodskin\t7\t1\t1\t0x1DAEB5B6\t4000\t1\t
353\tRockskin\t7\t12\t1\t0x1DAEB5B6\t4000\t2\t
357\tBarbshield\t7\t16\t1\t0x725CFDB8\t2000\t1\t
457\tMinor Healing\t7\t1\t1\t0xC2B700FA\t2000\t3\t
464\tHealing\t7\t20\t1\t0xC2B700FA\t3000\t1\t
880\tLife Tap\t11\t3\t1\t0xAF72B893\t2000\t1\t
903\tLife Siphon\t11\t16\t1\t0xAF72B893\t2000\t1\t
937\tLife Bane\t11\t34\t1\t0xAF72B894\t2000\t1\t
143\tChant of Battle\t5\t3\t2\t0x1DAEB5B6\t1000\t1\t
999\tMystery Spell\t7\t1\t0\t0x12345678\t1000\t1\t
"""


def audit(db, sid, before, after, by="gm", reverted_by=None):
    row = lambda fx: None if fx is None else json.dumps({"spellid": sid, "fxdict": fx, "line": "", "source": ""})
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, reverted_by) "
               "VALUES ('t', 1, ?, 'web', '', 'update', 'spelleffect', ?, ?, ?, ?)",
               (by, json.dumps({"spellid": sid}), row(before), row(after), reverted_by))


class FixSpellVisualsSandstorm(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        for sid, name, icon, scope, level, cls in SPELLS:
            db.execute("INSERT INTO spellpattern VALUES (?, ?, ?, ?, ?)", (sid, name, icon, scope, level))
            db.execute("INSERT INTO classspells VALUES (?, ?, 'dump')", (cls, sid))
        db.executemany("INSERT INTO spelleffect VALUES (?, ?, ?, ?)", ROWS)
        audit(db, 700110, 0, 0x11111111)               # a GM set Ironskin's visual
        audit(db, 353, 0, 0x22222222, reverted_by=3)   # a GM's pick on Rockskin, reverted...
        audit(db, 353, 0x22222222, 0, by="claude")     # ...back to the stand-in: not a GM's visual
        db.commit()
        db.close()
        self.visuals = os.path.join(self.dir.name, "visuals.tsv")
        with open(self.visuals, "w") as f:
            f.write(RECORDINGS)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--visuals", self.visuals, *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def fx(self):
        c = sqlite3.connect(self.db)
        try:
            return {sid: fx for sid, fx in c.execute("SELECT spellid, fxdict FROM spelleffect")}
        finally:
            c.close()

    def rows(self):
        c = sqlite3.connect(self.db)
        try:
            return c.execute("SELECT * FROM spelleffect ORDER BY spellid").fetchall()
        finally:
            c.close()

    def test_report_changes_nothing(self):
        before = self.rows()
        out = self.run_tool().stdout
        self.assertEqual(self.rows(), before)
        self.assertIn("13 spells to change: 9 recorded, 4 by their lines", out)
        self.assertIn("700110 Ironskin: 0x11111111", out)               # the GM's, said
        self.assertIn("lines whose recorded ranks differ (1)", out)
        self.assertIn("999 Mystery Spell", out)                         # unmatched, said
        self.assertIn("357 Barbshield -> ours 300357", out)
        self.assertIn("700307 Greater Healing (29): 0xC2B700FA, recorded 1 to 20", out)

    def test_apply_and_undo(self):
        before = self.rows()
        self.run_tool("--apply")
        fx = self.fx()
        self.assertEqual(fx[342], BUFF)                # recorded
        self.assertEqual(fx[353], BUFF)                # recorded: the reverted pick is no GM's visual
        self.assertEqual(fx[700100], BUFF)             # between two recorded ranks
        self.assertEqual(fx[700267], 0)                # beyond them: left
        self.assertEqual(fx[700110], 0x11111111)       # a GM's: left
        self.assertEqual(fx[700423], 0)                # beyond: left
        self.assertEqual(fx[700083], 0)                # below its line's only record: left
        self.assertEqual(fx[700090], 0)                # the same icon and scope, no line
        self.assertEqual(fx[700834], 0)                # another scope
        self.assertEqual(fx[800001], 0)                # another class
        self.assertEqual(fx[800002], 0)                # another class, at a level between
        self.assertEqual(fx[700120], 0)                # a one-letter ending is no line
        self.assertEqual(fx[300357], BARB)             # by ID_OFFSET
        self.assertEqual(fx[700357], BARB)             # its twin
        self.assertEqual(fx[357], 0)                   # Sandstorm's 357 is another spell
        self.assertEqual((fx[457], fx[461], fx[464]), (MINOR_HEAL,) * 3)
        self.assertEqual(fx[700307], 0)                # beyond: left
        self.assertEqual(fx[458], 0)
        self.assertEqual(fx[700376], 0)                # "Greater" is a rank's word, not a line's
        self.assertEqual((fx[880], fx[890], fx[903], fx[937]), (TAP1, TAP1, TAP1, TAP2))
        self.assertEqual((fx[915], fx[953]), (0, 0))   # between visuals that differ, beyond them
        self.assertEqual(fx[143], BUFF)                # a new row
        c = sqlite3.connect(self.db)
        new = c.execute("SELECT line, source FROM spelleffect WHERE spellid = 143").fetchone()
        logged = c.execute("SELECT count(*) FROM spell_visuals_sandstorm_log").fetchone()[0]
        c.close()
        self.assertEqual(new[0], "")
        self.assertEqual(logged, 13)
        r = self.run_tool("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("applied already", r.stderr)
        self.run_tool("--undo")
        self.assertEqual(self.rows(), before)
        c = sqlite3.connect(self.db)
        left = c.execute("SELECT count(*) FROM sqlite_master WHERE name = 'spell_visuals_sandstorm_log'").fetchone()[0]
        c.close()
        self.assertEqual(left, 0)

    def test_whole_lines(self):
        self.run_tool("--apply", "--whole-lines")
        fx = self.fx()
        self.assertEqual(fx[700267], BUFF)             # beyond, in a line that agrees
        self.assertEqual(fx[700307], MINOR_HEAL)
        self.assertEqual(fx[700083], BARB)
        self.assertEqual(fx[700423], BUFF)
        self.assertEqual(fx[700110], 0x11111111)       # a GM's: left still
        self.assertEqual((fx[915], fx[953]), (0, 0))   # a line that steps: never
        self.assertEqual(fx[700376], 0)

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--apply")
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spelleffect SET fxdict = 0x7777 WHERE spellid = 342")  # a GM's, after
        c.commit()
        c.close()
        out = self.run_tool("--undo").stdout
        self.assertIn("1 changed since left as they are", out)
        fx = self.fx()
        self.assertEqual(fx[342], 0x7777)
        self.assertEqual(fx[700100], 0)
        self.assertNotIn(143, fx)

    def test_comparison_tsv(self):
        path = os.path.join(self.dir.name, "cmp.tsv")
        self.run_tool("--tsv", path)
        with open(path) as f:
            lines = f.read().splitlines()
        self.assertEqual(len(lines), 1 + 10)
        woodskin = [l for l in lines if l.startswith("342\t")][0].split("\t")
        self.assertEqual(woodskin[5:], ["0x00009999", "eqoa-data druid (Merchant)", "0x1DAEB5B6", "no"])


if __name__ == "__main__":
    unittest.main()
