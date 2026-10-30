#!/usr/bin/env python3
"""tools/fix_balance_spells.py against a made-up database: Vampiric Bond
Effect takes its recast, the three wrong infusions their record's scope and
all six the pact wording, a GM's changed row is left (and said), a second
--apply changes nothing and --undo gives back every table as it was. No game
data.

    python3 tools/fix_balance_spells_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_balance_spells.py")
sys.path.insert(0, HERE)
import fix_balance_spells as fbs  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, range INTEGER, casttime INTEGER,
    power INTEGER, iconColor INTEGER, scope INTEGER, recast INTEGER, spellname TEXT, spelldesc TEXT);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
"""


class FixBalanceSpells(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        rows = [(1, fbs.SK, "Vampiric Bond Effect", 46, 1, 0, "Target, Drain HP by 96 ; Increase 5% based on INT")]
        sid = 10
        for (cls, name, lvl), (scope, line, _, _, _) in fbs.INFUSIONS.items():
            rows.append((sid, cls, name.title(), lvl, scope, 0, line))
            sid += 1
        rows.append((99, fbs.SK, "Siphon Life", 16, 1, 2, "Target, Drain HP by 29 and gives to self"))
        for sid, cls, name, lvl, scope, recast, line in rows:
            rng, frame = (fbs.TARGET_RANGE, fbs.TARGET_FRAME) if scope else (fbs.SELF_RANGE, fbs.SELF_FRAME)
            db.execute("INSERT INTO spellpattern VALUES (?, ?, ?, 2, 0, ?, ?, ?, ?, ?)",
                       (sid, lvl, rng, frame, scope, recast, name, line))
            db.execute("INSERT INTO spelleffect VALUES (?, ?, 0, 'eqoa-data')", (sid, line))
            db.execute("INSERT INTO classspells VALUES (?, ?, 'eqoa-data')", (cls, sid))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def tables(self):
        c = sqlite3.connect(self.db)
        try:
            return {t: c.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
                    for t in ("spellpattern", "spelleffect", "classspells")}
        finally:
            c.close()

    def test_apply_and_undo(self):
        before = self.tables()
        self.assertIn("13 rows to change, 0 left", self.run_tool().stdout)
        self.assertEqual(self.tables(), before)  # report only
        self.run_tool("--apply")
        c = sqlite3.connect(self.db)
        vbe = c.execute("SELECT recast FROM spellpattern WHERE spellid = 1").fetchone()[0]
        self.assertEqual(vbe, fbs.VBE_RECAST)
        for sid, scope, rng, frame in c.execute("SELECT spellid, scope, range, iconColor FROM spellpattern WHERE spellid BETWEEN 10 AND 15"):
            self.assertEqual((scope, rng, frame), (0, fbs.SELF_RANGE, fbs.SELF_FRAME), sid)
        lines = [l for (l,) in c.execute("SELECT line FROM spelleffect WHERE spellid BETWEEN 10 AND 15 ORDER BY spellid")]
        self.assertEqual(lines[4], "Self, Decrease Hitpoints by 1000 to Self, Increase Power per tic by 600, 18sec")
        self.assertTrue(all(l.startswith("Self, Decrease Hitpoints by ") and " to Self, " in l for l in lines))
        self.assertEqual(c.execute("SELECT recast, line FROM spellpattern JOIN spelleffect USING (spellid) WHERE spellid = 99").fetchone(),
                         (2, "Target, Drain HP by 29 and gives to self"))  # not the tool's
        c.close()
        self.assertIn("0 rows to change", self.run_tool("--apply").stdout)
        self.run_tool("--undo")
        self.assertEqual(self.tables(), before)

    def test_a_gms_change_is_left(self):
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spellpattern SET recast = 12 WHERE spellid = 1")
        c.commit()
        c.close()
        out = self.run_tool("--apply").stdout
        self.assertIn("left as it is: SK L46 vampiric bond effect (record 1 has recast 12, not 0: a GM's)", out)
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT recast FROM spellpattern WHERE spellid = 1").fetchone()[0], 12)
        c.close()


if __name__ == "__main__":
    unittest.main()
