#!/usr/bin/env python3
"""santa_hat_20261117.py on a small database: the report writes nothing; --apply makes the Santa Hat (a copy of the
Tattered Cap: every class and race, tradeable, not lore, model 1 red), its itemaura hat (the gnome's cap, Santa's
style), Santa Claus's npctint hat and Toymaker Tinsel's stock row, one gm_audit line each; a second --apply finds
nothing to do; --undo puts back exactly what was there, refused while a hat is held. An npctint row of Santa's kept its
tint. The controls: no Tinsel's shop, refused, nothing written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "santa_hat_20261117.py")
SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, itemicon INTEGER, equipslot INTEGER,
    model INTEGER, color INTEGER, trade INTEGER, lore INTEGER, levelreq INTEGER, classuse INTEGER, raceuse INTEGER, AC INTEGER);
CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
    source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
    color2 INTEGER NOT NULL DEFAULT 0, kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0, nodes2 INTEGER NOT NULL DEFAULT 0);
CREATE TABLE npctint (name TEXT PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, glow INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', aura_kind INTEGER NOT NULL DEFAULT 0,
    aura_source INTEGER NOT NULL DEFAULT 0, aura_nodes INTEGER NOT NULL DEFAULT 0);
CREATE TABLE tokenshop (id INTEGER PRIMARY KEY AUTOINCREMENT, shop TEXT NOT NULL, patternid INTEGER NOT NULL, price INTEGER NOT NULL,
    token TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '', sort INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm',
    UNIQUE (shop, patternid));
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, patternid INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO itempattern VALUES (6428, 'Tattered Cap', 't', 624020665, 1, 1, 4294967295, 0, 0, 1, 32767, 1023, 1);
INSERT INTO itempattern VALUES (716912, 'Santa''s Elf', 'e', 1, 0, 0, 0, 1, 1, 1, 32767, 32767, 0);
"""


def state(path):
    db = sqlite3.connect(path)
    try:
        return tuple(db.execute(q).fetchall() for q in ("SELECT * FROM itempattern ORDER BY 1", "SELECT * FROM itemaura",
                                                         "SELECT * FROM npctint", "SELECT * FROM tokenshop ORDER BY 1"))
    finally:
        db.close()


class SantaHat(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO tokenshop (shop, patternid, price, sort) VALUES ('Toymaker Tinsel', 716912, 54, 1)")
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, *args], capture_output=True, text=True)

    def q(self, sql):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql).fetchall()
        finally:
            db.close()

    def test_apply_undo(self):
        before = state(self.path)
        self.assertIn("report only: 4 rows", self.run_tool().stdout)
        self.assertEqual(state(self.path), before)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.q("SELECT patternid, itemname, model, color, trade, lore, levelreq, classuse, raceuse, AC FROM itempattern WHERE itemname = 'Santa Hat'"),
                         [(716913, "Santa Hat", 1, 0xC01818FF, 0, 0, 1, 32767, 32767, 1)])
        self.assertEqual(self.q("SELECT kind, source, nodes, color FROM itemaura WHERE patternid = 716913"), [(10, 0xA20590E3, 1, 0)])
        self.assertEqual(self.q("SELECT name, aura_kind, aura_source, aura_nodes FROM npctint"), [("Santa Claus", 10, 0xA20590E3, 1)])
        self.assertEqual(self.q("SELECT shop, patternid, price, token, sort FROM tokenshop WHERE patternid = 716913"), [("Toymaker Tinsel", 716913, 27, "", 4)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(4,)])
        self.assertIn("nothing to do", self.run_tool("--apply").stdout)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (5, 716913)")
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertIn("held", r.stdout)
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory")
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path), before)

    def test_santas_tint_kept(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO npctint (name, color, glow) VALUES ('santa claus', 4294901760, 0)")
        db.commit()
        db.close()
        before = state(self.path)
        self.assertEqual(self.run_tool("--apply").returncode, 0)
        self.assertEqual(self.q("SELECT name, color, aura_kind, aura_nodes FROM npctint"), [("santa claus", 4294901760, 10, 1)])
        self.assertEqual(self.run_tool("--undo").returncode, 0)
        self.assertEqual(state(self.path), before)

    def test_no_shop_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM tokenshop")
        db.commit()
        db.close()
        before = state(self.path)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
