#!/usr/bin/env python3
"""christmas_toys_20261117.py on a small database: the report writes nothing; --apply makes the four toys (records
copied from their bases' shapes: tradeable, every class and race), their item auras (lights on the cape, the twinkle and
stripes on the staff, icing on the gingerbread man), the Gingerbread Man's illusion (itemeffect: the clay golem, 60 s)
and its outfit row, and Toymaker Tinsel's four stock rows, a gm_audit line a row; a second --apply finds nothing to do;
--undo puts back exactly what was there, refused while a toy is held. The controls: no Tinsel's shop, or a toy's id
taken by another record, refused, nothing written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "christmas_toys_20261117.py")
COLS = ("patternid INTEGER PRIMARY KEY, itemcost INTEGER, itemicon INTEGER, equipslot INTEGER, trade INTEGER, attacktype INTEGER, "
        "weapondamage INTEGER, levelreq INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER, procanim INTEGER, "
        "lore INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, color INTEGER")
SCHEMA = f"""
CREATE TABLE itempattern ({COLS});
CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
    source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
    color2 INTEGER NOT NULL DEFAULT 0, kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0, nodes2 INTEGER NOT NULL DEFAULT 0);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '', icon INTEGER NOT NULL DEFAULT 0,
    breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0, area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm',
    body INTEGER NOT NULL DEFAULT 0, target_line TEXT NOT NULL DEFAULT '', charges INTEGER NOT NULL DEFAULT 0, cooldown REAL NOT NULL DEFAULT 0);
CREATE TABLE itemoutfit (patternid INTEGER NOT NULL, slot INTEGER NOT NULL, look INTEGER NOT NULL DEFAULT 0, color INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (patternid, slot));
CREATE TABLE tokenshop (id INTEGER PRIMARY KEY AUTOINCREMENT, shop TEXT NOT NULL, patternid INTEGER NOT NULL, price INTEGER NOT NULL,
    token TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '', sort INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm',
    UNIQUE (shop, patternid));
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, patternid INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO itempattern VALUES (717004, 0, 1414103605, 28, 1, 0, 0, 1, 0, 32767, 32767, 0, 0, 'Heirloom Dreadlord''s Cape', 'c', 4, 0);
INSERT INTO itempattern VALUES (5489, 0, -597854559, 15, 1, 4, 12, 1, 100, 31744, 783, 0, 0, 'Apprentice Staff', 's', 40739020, 4294967295);
INSERT INTO itempattern VALUES (716910, 0, -1324486342, -1, 1, 0, 0, 1, 80, 32767, 32767, 2, 1, 'Barrel of Monkeys', 'b', 0, 4294967295);
INSERT INTO tokenshop (shop, patternid, price, sort) VALUES ('Toymaker Tinsel', 716910, 108, 2);
"""
TABLES = ("itempattern", "itemaura", "itemeffect", "itemoutfit", "tokenshop")


def state(path):
    db = sqlite3.connect(path)
    try:
        return tuple(db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in TABLES)
    finally:
        db.close()


class ChristmasToys(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, *args], capture_output=True, text=True)

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        before = state(self.path)
        r = self.run_tool()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 13 rows", r.stdout)
        self.assertEqual(state(self.path), before, "the report wrote")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        recs = {row[0]: row for row in self.q("SELECT patternid, itemname, equipslot, trade, lore, classuse, raceuse, model, color, "
                                              "weapondamage, procanim, itemicon FROM itempattern WHERE patternid BETWEEN 716914 AND 716917")}
        self.assertEqual(sorted(recs), [716914, 716915, 716916, 716917])
        cape, staff, plush, ginger = (recs[i] for i in (716914, 716915, 716916, 716917))
        self.assertEqual(cape[1:9], ("String of Lights", 28, 0, 0, 32767, 32767, 4, 0xE04040FF))
        self.assertEqual(staff[1:10], ("Candy Cane Staff", 15, 0, 0, 32767, 32767, 40739020, 0xC81E1EFF, 12))
        self.assertEqual((plush[1], plush[2], plush[3], plush[4], plush[10], plush[11]), ("Pegasus Plushie", -1, 0, 0, 2, 824019198))
        self.assertEqual((ginger[1], ginger[2], ginger[3], ginger[10]), ("Gingerbread Man", -1, 0, 2))
        auras = {a[0]: a[1:] for a in self.q("SELECT patternid, color, kind, source, nodes, color2, kind2, source2, nodes2 FROM itemaura")}
        self.assertEqual(auras, {716914: (0, 11, 0, 0, 0, 0, 0, 0), 716915: (0, 12, 0, 0, 0, 3, 0x1D875ECD, 100),
                                 716917: (0, 13, 0, 0, 0, 0, 0, 0)})
        self.assertEqual(self.q("SELECT patternid, line, body, cooldown, source FROM itemeffect"),
                         [(716917, "Self, Unlimited Uses, 30min", 0xEC6A0604, 60.0, "christmas-toys")])
        self.assertEqual(self.q("SELECT patternid, slot, look, color FROM itemoutfit"), [(716917, 0, 0, 0)])
        self.assertEqual(self.q("SELECT patternid, price FROM tokenshop WHERE source = 'christmas-toys' ORDER BY sort"),
                         [(716914, 81), (716915, 108), (716916, 54), (716917, 54)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 13)
        applied = state(self.path)
        r = self.run_tool("--apply")
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(state(self.path), applied)
        # held: refused
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, 716916)")
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("are held", r.stdout)
        self.assertEqual(state(self.path), applied)
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory")
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path), before, "undo is not exact")

    def test_refusals(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM tokenshop")
        db.commit()
        db.close()
        before = state(self.path)
        r = self.run_tool("--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), before)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO tokenshop (shop, patternid, price, sort) VALUES ('Toymaker Tinsel', 716910, 108, 2)")
        db.execute("INSERT INTO itempattern (patternid, itemname) VALUES (716916, 'Something Else')")
        db.commit()
        db.close()
        before = state(self.path)
        r = self.run_tool("--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("not free", r.stdout)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
