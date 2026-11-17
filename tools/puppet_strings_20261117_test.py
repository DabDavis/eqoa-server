#!/usr/bin/env python3
"""puppet_strings_20261117.py on a small database: the report writes nothing; --apply makes the Toy Shoppe's Puppet
Strings (the Cazic Thule strings' icon and type, level 1, every class and race, tradeable, not lore, worth 0), its
itemeffect (the charm by colour, No Recharge, 10 charges, 60 s) and Toymaker Tinsel's row, one gm_audit line each,
leaving the Cazic Thule record alone; --give puts one of 10 charges in Zaeire's pack; a second --apply finds nothing to
do; --undo puts back exactly what was there (the --give's copy with it), refused while anyone else holds one. The
controls: no Tinsel's shop, refused; Zaeire of another account, refused; nothing written either way."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "puppet_strings_20261117.py")
SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemcost INTEGER, itemicon INTEGER, equipslot INTEGER,
    trade INTEGER, rent INTEGER, attacktype INTEGER, weapondamage INTEGER, unk5 INTEGER, levelreq INTEGER,
    maxstack INTEGER, maxhp INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER, procanim INTEGER, lore INTEGER,
    craft INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, color INTEGER, wis INTEGER, cha INTEGER, POWMAX INTEGER,
    weaponproc INTEGER, fish INTEGER);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
    icon INTEGER NOT NULL DEFAULT 0, breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0,
    area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm', body INTEGER NOT NULL DEFAULT 0,
    target_line TEXT NOT NULL DEFAULT '', charges INTEGER NOT NULL DEFAULT 0, cooldown REAL NOT NULL DEFAULT 0);
CREATE TABLE tokenshop (id INTEGER PRIMARY KEY AUTOINCREMENT, shop TEXT NOT NULL, patternid INTEGER NOT NULL, price INTEGER NOT NULL,
    token TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '', sort INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm',
    UNIQUE (shop, patternid));
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, accountid INTEGER, level INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
    remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL,
    location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO itempattern VALUES (713085, 4235, -611086336, -1, 0, 1, 0, 0, 0, 60, 1, 12000, 65, 32767, 1023, 7, 1, 0,
    'Puppet Strings', '', 786059401, 4294967295, 28, 28, 100, NULL, NULL);
INSERT INTO itempattern (patternid, itemname, procanim) VALUES (716913, 'Santa Hat', 0);
INSERT INTO itemeffect (patternid, line, charges) VALUES (713085, 'Target, Charm from level 41 up to level 52 ; Increase 25% based on CHA, 3min', 50);
INSERT INTO characters VALUES (193, 'Zaeire', 107, 60);
INSERT INTO characters VALUES (200, 'Other', 5, 10);
INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
    VALUES (193, 1, 0, 0, 716913, -1, 0, 0);
"""


def state(path):
    db = sqlite3.connect(path)
    try:
        return tuple(db.execute(q).fetchall() for q in ("SELECT * FROM itempattern ORDER BY 1", "SELECT * FROM itemeffect ORDER BY 1",
                                                         "SELECT * FROM tokenshop ORDER BY 1", "SELECT * FROM charinventory ORDER BY 1"))
    finally:
        db.close()


class PuppetStrings(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO tokenshop (shop, patternid, price, sort) VALUES ('Toymaker Tinsel', 716913, 27, 4)")
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, *args], capture_output=True, text=True)

    def db(self):
        return sqlite3.connect(self.path)

    def test_report_apply_give_again_undo(self):
        before = state(self.path)
        r = self.run_tool("--give")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 4 rows", r.stdout)
        self.assertEqual(state(self.path), before)

        r = self.run_tool("--give", "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = self.db()
        p = db.execute("SELECT itemname, itemicon, procanim, trade, lore, levelreq, maxstack, itemcost, maxhp, classuse, raceuse, "
                       "wis, cha, POWMAX, equipslot, weaponproc FROM itempattern WHERE patternid = 716914").fetchone()
        self.assertEqual(p, ("Toymaker's Puppet Strings", -611086336, 7, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, -1, 0))
        e = db.execute("SELECT line, charges, cooldown FROM itemeffect WHERE patternid = 716914").fetchone()
        self.assertEqual(e, ("Target, Charm up to light blue, No Recharge, 10min", 10, 60.0))
        self.assertEqual(db.execute("SELECT price, sort FROM tokenshop WHERE patternid = 716914").fetchone(), (108, 5))
        self.assertEqual(db.execute("SELECT remaincharge, location, listnumber, equiploc FROM charinventory "
                                    "WHERE serverid = 193 AND patternid = 716914").fetchall(), [(10, 0, 1, -1)])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 4)
        # the Cazic Thule strings left as they were
        self.assertEqual(db.execute("SELECT * FROM itempattern WHERE patternid = 713085").fetchone(),
                         [r for r in before[0] if r[0] == 713085][0])
        db.close()

        r = self.run_tool("--give", "--apply")
        self.assertIn("nothing to do", r.stdout)

        # another holds a copy: no undo
        db = self.db()
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (200, 1, 0, 10, 716914, -1, 0, 0)")
        db.commit()
        db.close()
        applied = state(self.path)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertIn("held", r.stdout)
        self.assertEqual(state(self.path), applied)
        db = self.db()
        db.execute("DELETE FROM charinventory WHERE serverid = 200")
        db.commit()
        db.close()

        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path), before)

    def test_refusals_write_nothing(self):
        before = state(self.path)
        db = self.db()
        db.execute("UPDATE characters SET accountid = 9 WHERE serverid = 193")
        db.commit()
        db.close()
        r = self.run_tool("--give", "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("not 107's", r.stdout)
        self.assertEqual(state(self.path), before)

        db = self.db()
        db.execute("DELETE FROM tokenshop")
        db.commit()
        db.close()
        before = state(self.path)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no tokenshop rows", r.stdout)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
