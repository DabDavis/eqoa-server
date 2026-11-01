#!/usr/bin/env python3
"""fill_spell_visuals_eqoaemu.py on a database of its own: the plan (a
stand-in filled, ours replaced, a GM's too, the same left, a name the classes
settle, a visual not on the disc refused, the sheet's typos read as the disc
settles them), the apply's gm_audit lines as the panel writes them, a stale
plan refused, and --undo exact."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fill_spell_visuals_eqoaemu as fv  # noqa: E402

A, B, C, D = 0x1DAEB5B6, 0xA8C2C199, 0xE78024D9, 0x12345678  # D: not on the disc
DISC = {A, B, C}


class FillVisualsTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(os.path.join(tempfile.mkdtemp(), "t.db"), isolation_level=None)
        x = self.db.execute
        x("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        x("INSERT INTO accountinfo VALUES (1, 'dab')")
        x("CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT)")
        x("CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL DEFAULT '', fxdict INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT '')")
        x("CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT)")
        for sid, name in [(1, "Stand In"), (2, "Ours Wrong"), (3, "Same"), (4, "No Row"), (5, "Twice"), (6, "Off Disc"), (7, "Typo"), (8, "GM Set")]:
            x("INSERT INTO spellpattern VALUES (?, ?)", (sid, name))
        x("INSERT INTO spelleffect VALUES (1, 'a line', 0, 'eqoa-data')")
        x("INSERT INTO spelleffect VALUES (2, 'b line', ?, 'eqoa-data; visual recorded')", (B,))
        x("INSERT INTO spelleffect VALUES (3, '', ?, 'sandstorm')", (A,))
        x("INSERT INTO spelleffect VALUES (5, '', 0, '')")
        x("INSERT INTO spelleffect VALUES (6, '', 0, '')")
        x("INSERT INTO spelleffect VALUES (7, '', 0, '')")
        x("INSERT INTO spelleffect VALUES (8, '', ?, 'visual set by GM jib in the game')", (B,))
        x("INSERT INTO classspells VALUES (11, 5, '')")  # Twice is the necromancer's
        self.sheet = [
            {"Name": "Stand In", "CLASS": "WAR", "Graphic": "0x1DAEB5B6"},
            {"Name": "ours wrong", "CLASS": "WAR", "Graphic": "1DAEB5B6"},
            {"Name": "Same", "CLASS": "WAR", "Graphic": "0x1DAEB5B6"},
            {"Name": "No Row", "CLASS": "WAR", "Graphic": "0xA8C2C199"},
            {"Name": "Twice", "CLASS": "WIZ", "Graphic": "0x1DAEB5B6"},
            {"Name": "Twice", "CLASS": "NEC", "Graphic": "0xE78024D9"},
            {"Name": "Off Disc", "CLASS": "WAR", "Graphic": "0x12345678"},
            {"Name": "Typo", "CLASS": "WAR", "Graphic": "0xE78024D9c"},
            {"Name": "GM Set", "CLASS": "WAR", "Graphic": "0xE78024D9"},
        ]

    def fx(self, sid):
        r = self.db.execute("SELECT fxdict, source FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
        return r

    def test_dictid(self):
        self.assertEqual(fv.dictid("0xE78024D9", DISC), C)
        self.assertEqual(fv.dictid("E78024D9c", DISC), C, "a typo's 9th digit")
        self.assertEqual(fv.dictid("0E78024D9", DISC), C, "a typo's leading 0")
        self.assertIsNone(fv.dictid("0x12345678", DISC), "not on the disc")
        self.assertIsNone(fv.dictid("", DISC))

    def test_plan_apply_undo(self):
        entries, why = fv.plan(self.db, self.sheet, DISC)
        got = {e["spellid"]: (e["fx"], e["kind"]) for e in entries}
        self.assertEqual(got, {1: (A, "a stand-in filled"), 2: (A, "ours replaced"), 4: (B, "a stand-in filled"),
                               5: (C, "a stand-in filled"), 7: (C, "a stand-in filled"), 8: (C, "ours replaced")})
        self.assertEqual(why["ours is the sheet's already"], 1)
        self.assertEqual(why["the sheet gives no single visual of the disc"], 1)  # Off Disc

        before = {s: self.fx(s) for s in range(1, 9)}
        done, stale = fv.apply(self.db, entries)
        self.assertEqual((done, stale), (6, 0))
        self.assertEqual(self.fx(2), (A, "eqoa-data; visual recorded; " + fv.SOURCE_NOTE))
        self.assertEqual(self.fx(4), (B, fv.SOURCE_NOTE + " (2020)"))
        self.assertEqual(self.fx(8)[0], C, "a GM's was left")
        # the panel's audit lines: key, the rows before and after, as dab via tool
        a = self.db.execute("SELECT username, via, action, rowkey, before, after FROM gm_audit WHERE rowkey = '{\"spellid\":2}'").fetchone()
        self.assertEqual(a[:4], ("dab", "tool", "update", '{"spellid":2}'))
        self.assertEqual(json.loads(a[4])["fxdict"], B)
        self.assertEqual(json.loads(a[5]), {"fxdict": A, "line": "b line", "source": "eqoa-data; visual recorded; " + fv.SOURCE_NOTE, "spellid": 2})
        ins = self.db.execute("SELECT action, before FROM gm_audit WHERE rowkey = '{\"spellid\":4}'").fetchone()
        self.assertEqual(ins, ("insert", None))

        # the same plan again: every row moved since, nothing written twice
        self.assertEqual(fv.apply(self.db, entries), (0, 6))

        # a GM changes one after; undo leaves it and puts the rest back
        self.db.execute("UPDATE spelleffect SET fxdict = ? WHERE spellid = 1", (B,))
        back, left = fv.undo(self.db)
        self.assertEqual((back, left), (5, 1))
        for s in (2, 3, 5, 6, 7, 8):
            self.assertEqual(self.fx(s), before[s], f"spell {s} not put back")
        self.assertIsNone(self.fx(4), "the row made was not taken out")
        self.assertEqual(self.fx(1)[0], B, "the GM's later change was undone")


if __name__ == "__main__":
    unittest.main()
