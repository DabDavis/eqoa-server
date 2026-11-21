#!/usr/bin/env python3
"""sunder_melee_20261121.py on a small database: the report writes nothing; --apply sets Sunder's req_equipment to the
melee codes (nothing else in its JSON) with a gm_audit line; a second apply does nothing; --undo puts the row back
exactly. The controls: Cleave is never touched, a spell 700700 not named Sunder or a req_equipment other than "Any"
is refused, writing nothing, and a row changed after the apply is left by the undo."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "sunder_melee_20261121.py")
MELEE = "1HS,2HS,1HB,2HB,1HP,2HP"
SUNDER = ('{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":54,"min_level":1,'
          '"req_equipment":"%s","sheet_type":"Dmg/Heal","effects":[{"id":"0519","type":"Dmg/Heal","kind":"hp",'
          '"amount":-750,"scale":"str","scale_pct":30,"resist":"blunt"}]}')
CLEAVE = SUNDER.replace("700", "600") % MELEE


def make(path, name="Sunder", req="Any"):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '',
            fxdict INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    db.execute("INSERT INTO spellpattern VALUES (700700, ?), (700534, 'Cleave')", (name,))
    db.execute("INSERT INTO spelleffect (spellid, line, effects) VALUES (700700, 'Target, Decrease Hitpoints by 750', ?), "
               "(700534, 'Target, Decrease Hitpoints by 600', ?)", (SUNDER % req, CLEAVE))
    db.commit()
    db.close()


def rows(path):
    db = sqlite3.connect(path)
    try:
        return list(db.execute("SELECT spellid, line, effects, fxdict FROM spelleffect ORDER BY spellid"))
    finally:
        db.close()


def audits(path):
    db = sqlite3.connect(path)
    try:
        return db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
    finally:
        db.close()


class SunderMeleeTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, "-I", TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def test_apply_and_undo(self):
        make(self.path)
        before = rows(self.path)
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertEqual(rows(self.path), before, "the report wrote")
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        after = rows(self.path)
        self.assertEqual(after[0], before[0], "Cleave was touched")
        got, was = json.loads(after[1][2]), json.loads(before[1][2])
        self.assertEqual(got["req_equipment"], MELEE)
        was["req_equipment"] = MELEE
        self.assertEqual(got, was, "more than req_equipment changed")
        self.assertEqual(after[1][1], before[1][1])
        self.assertEqual(audits(self.path), 1)
        code, out = self.run_tool("--apply")
        self.assertEqual((code, rows(self.path)), (0, after), out)
        self.assertIn("applied already", out)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(rows(self.path), before)
        self.assertEqual(audits(self.path), 2)

    def test_refusals(self):
        for name, req in (("Smite", "Any"), ("Sunder", "1HB,2HB")):
            with self.subTest(name=name, req=req):
                if os.path.exists(self.path):
                    os.remove(self.path)
                make(self.path, name, req)
                before = rows(self.path)
                code, out = self.run_tool("--apply")
                self.assertEqual(code, 1, out)
                self.assertIn("refused", out)
                self.assertEqual(rows(self.path), before)
                self.assertEqual(audits(self.path), 0)

    def test_undo_leaves_a_row_changed_since(self):
        make(self.path)
        self.assertEqual(self.run_tool("--apply")[0], 0)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE spelleffect SET effects = ? WHERE spellid = 700700", (SUNDER % "1HS",))
        db.commit()
        db.close()
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("left", out)
        self.assertIn('"req_equipment":"1HS"', rows(self.path)[1][2])


if __name__ == "__main__":
    unittest.main()
