#!/usr/bin/env python3
"""wisp_raiment_20261111.py on a small database: the report writes nothing; --apply writes the spell, its record,
Trickster Flame's adds and the owners' books (not a book that has it, nor a character without the mastery), a
gm_audit line each; a second apply does nothing; --undo puts every table back exactly. The controls: Trickster Flame
not as the wisp kit wrote it, and the spell id taken, are refused, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import wisp_kit_20261111 as wk  # noqa: E402
import wisp_raiment_20261111 as wr  # noqa: E402
from wisp_kit_20261111_test import CMCATALOG  # noqa: E402

TOOL = os.path.join(HERE, "wisp_raiment_20261111.py")


def make(path, flame=True, taken=False):
    db = sqlite3.connect(path)
    db.executescript(f"""
        {CMCATALOG};
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
            unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
            iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
            eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
            effects TEXT NOT NULL DEFAULT '');
        CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER, addedorder INTEGER,
            onHotBar INTEGER, whereonBar INTEGER, unk1 INTEGER, showhide INTEGER);
        CREATE TABLE charcms (serverid INTEGER, cmid INTEGER, cost INTEGER, bought INTEGER);
        CREATE TABLE characters (charName TEXT, serverid INTEGER PRIMARY KEY);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO spellpattern VALUES (496, 1, 0, 0, 16544, 5, 20, -1809115846, 1507275072, 1, 1, -1, 'Glamour: Human', 'CM'),
            (1793, 1, 0, 0, 16528, 3, 1, -1809115846, 1507275072, 0, 5, -1, 'Remove Illusion', 'removes');
        INSERT INTO spelleffect VALUES (496, 'Target, Gives Form, 60min', 2021015237, 'x', '');
        INSERT INTO characters VALUES ('Wisp', 676), ('Flicker', 677), ('Human', 5);
        INSERT INTO charcms VALUES (676, 2142, 10, 1), (677, 2142, 10, 1), (5, 1001, 5, 1);
        INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES
            (676, 496, 0, 0, -1, 0, -1), (676, 1793, 7, 0, -1, 0, -1), (677, 496, 0, 0, -1, 0, -1), (5, 1793, 3, 0, -1, 0, -1);
    """)
    for r in wk.ROWS:
        if r["id"] == wr.FLAME and not flame:
            r = dict(r, adds="Glamour: Human@1")
        cols = list(r)
        db.execute(f"INSERT INTO cmcatalog ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
    if taken:
        db.execute("INSERT INTO spelleffect VALUES (?, 'Someone else', 0, 'x', '')", (wr.SPELL,))
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return [list(db.execute(f"SELECT * FROM {t} ORDER BY 1, 2")) for t in ("cmcatalog", "spellpattern", "spelleffect", "spells")]
    finally:
        db.close()


class WispRaimentTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def test_apply_and_undo(self):
        make(self.path)
        before = dump(self.path)
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("2 owner(s)", out)
        self.assertEqual(dump(self.path), before, "the report wrote")
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT spellname, scope, icon FROM spellpattern WHERE spellid = ?", (wr.SPELL,)).fetchone(),
                         ("Wisp's Raiment", 0, 1507275072))
        self.assertEqual(db.execute("SELECT fxdict FROM spelleffect WHERE spellid = ?", (wr.SPELL,)).fetchone()[0], 2021015237)
        flame = wk.row_of(db, wr.FLAME)
        self.assertTrue(flame["add_spells"].endswith(",906720") and flame["adds"].endswith("; Wisp's Raiment@1"))
        self.assertEqual(db.execute("SELECT serverid, addedorder FROM spells WHERE spellid = ? ORDER BY serverid", (wr.SPELL,)).fetchall(),
                         [(676, 8), (677, 1)], "each owner's book, after its last; the human's not")
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail = ?", (wr.DETAIL,)).fetchone()[0], 5)
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied already", out)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.path), before, "the undo")

    def refused(self, want):
        before = dump(self.path)
        code, out = self.run_tool("--apply")
        self.assertNotEqual(code, 0, out)
        self.assertIn(want, out)
        self.assertEqual(dump(self.path), before)

    def test_refusals(self):
        make(self.path, flame=False)
        self.refused("is not Trickster Flame as")
        os.remove(self.path)
        make(self.path, taken=True)
        self.refused("spelleffect 906720 is taken")


if __name__ == "__main__":
    unittest.main()
