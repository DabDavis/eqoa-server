#!/usr/bin/env python3
"""snowball_launcher_20261116.py on a small database: the report writes nothing; --apply makes the record (usable,
procanim 2, worn nowhere, LORE, NO TRADE, worth 0, a description of at most 200 plain characters), table itemowner and
the row binding it to Zerkziee, each write with a gm_audit line; --apply --give then puts one in his pack, unequipped at
the end of the list; a third apply finds nothing to do; --undo puts the database back as it was (gm_audit aside), and
leaves the record and its binding while someone still holds a copy. The controls: a Zerkziee of another account, a
taken id, another record of the name, a binding to someone else or a full pack refuse the whole, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "snowball_launcher_20261116.py")
LAUNCHER = 715090

SCHEMA = """
CREATE TABLE characters (charName TEXT UNIQUE NOT NULL, accountid INTEGER NOT NULL, serverid INTEGER PRIMARY KEY,
    tclass INTEGER NOT NULL, race INTEGER NOT NULL, level INTEGER NOT NULL);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL, unk1 INTEGER NOT NULL,
    itemicon INTEGER NOT NULL, unk2 INTEGER NOT NULL, equipslot INTEGER NOT NULL, unk3 INTEGER NOT NULL,
    trade INTEGER NOT NULL, rent INTEGER NOT NULL, unk4 INTEGER NOT NULL, attacktype INTEGER NOT NULL,
    weapondamage INTEGER NOT NULL, unk5 INTEGER NOT NULL, levelreq INTEGER NOT NULL, maxstack INTEGER NOT NULL,
    maxhp INTEGER NOT NULL, duration INTEGER NOT NULL, classuse INTEGER NOT NULL, raceuse INTEGER NOT NULL,
    procanim INTEGER NOT NULL, lore INTEGER NOT NULL, unk6 INTEGER NOT NULL, craft INTEGER NOT NULL,
    itemname TEXT NOT NULL, itemdesc TEXT NOT NULL, model INTEGER NOT NULL, color INTEGER NOT NULL,
    str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER, intelligence INTEGER, HPMAX INTEGER,
    POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER, DR INTEGER, FR INTEGER, CR INTEGER, LR INTEGER,
    AR INTEGER, weaponproc INTEGER, fish INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
    remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL,
    location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


def make(path, account=220, taken=False, named=False, bound=False, full=False):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO characters VALUES ('Zerkziee', ?, 747, 1, 3, 60)", (account,))
    db.execute("INSERT INTO characters VALUES ('Zerkzi', 220, 975, 4, 16, 60)")
    db.execute("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,"
               " weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft,"
               " itemname, itemdesc, model, color) VALUES (2216, 0,0,0,0,5,0,0,1,0,0,0,0,1,1,0,0,32767,1023,0,0,0,0,"
               " 'Tunic', '', 0, 0)")
    db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)"
               " VALUES (747, 1, 0, 0, 2216, 5, 0, 7)")
    if taken:
        db.execute("UPDATE itempattern SET patternid = ? WHERE patternid = 2216", (LAUNCHER,))
    if named:
        db.execute("UPDATE itempattern SET itemname = 'Automatic Snowball Launcher', equipslot = -1 WHERE patternid = 2216")
    if bound:
        db.execute("CREATE TABLE itemowner (patternid INTEGER PRIMARY KEY, serverid INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '')")
        db.execute("INSERT INTO itemowner VALUES (?, 975, '')", (LAUNCHER,))
    for n in range(99 if full else 0):
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)"
                   " VALUES (747, 1, 0, 0, 2216, -1, 0, ?)", (10 + n,))
    db.commit()
    db.close()


def run(path, *args):
    p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def snapshot(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("itempattern", "charinventory", "characters")}
    out["tables"] = db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'sqlite_sequence' ORDER BY 1").fetchall()
    if ("itemowner",) in out["tables"]:
        out["itemowner"] = db.execute("SELECT * FROM itemowner ORDER BY 1").fetchall()
    db.close()
    return out


class SnowballLauncher(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_give_again_undo(self):
        make(self.path)
        before = snapshot(self.path)
        code, out = run(self.path, "--give")
        self.assertEqual(code, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(snapshot(self.path), before)

        code, out = run(self.path, "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        r = db.execute("SELECT itemname, procanim, equipslot, lore, trade, rent, itemcost, maxstack, itemdesc FROM itempattern "
                       "WHERE patternid = ?", (LAUNCHER,)).fetchone()
        self.assertEqual(r[:8], ("Automatic Snowball Launcher", 2, -1, 1, 1, 1, 0, 1))
        self.assertTrue(0 < len(r[8]) <= 200 and r[8].isascii() and "|" not in r[8], r[8])
        self.assertEqual(db.execute("SELECT patternid, serverid FROM itemowner").fetchall(), [(LAUNCHER, 747)])
        self.assertEqual(db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0], 3)  # the table, the record, the binding
        self.assertEqual(db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (LAUNCHER,)).fetchone()[0], 0)
        db.close()

        code, out = run(self.path, "--give", "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT serverid, equiploc, location, listnumber FROM charinventory WHERE patternid = ?",
                                    (LAUNCHER,)).fetchall(), [(747, -1, 0, 8)])
        db.close()

        code, out = run(self.path, "--give", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)

        code, out = run(self.path, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(snapshot(self.path), before)

    def test_undo_leaves_a_record_still_held(self):
        make(self.path)
        run(self.path, "--give", "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charinventory SET serverid = 975 WHERE patternid = ?", (LAUNCHER,))  # moved to another character
        db.commit()
        db.close()
        code, out = run(self.path, "--undo")
        self.assertEqual(code, 1, out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT patternid FROM itempattern WHERE patternid = ?", (LAUNCHER,)).fetchall(), [(LAUNCHER,)])
        self.assertEqual(db.execute("SELECT patternid, serverid FROM itemowner").fetchall(), [(LAUNCHER, 747)])
        db.close()

    def test_refusals_write_nothing(self):
        for i, kw in enumerate(({"account": 98}, {"taken": True}, {"named": True}, {"bound": True}, {"full": True})):
            path = os.path.join(self.dir.name, f"r{i}.db")
            make(path, **kw)
            before = snapshot(path)
            code, out = run(path, "--give", "--apply")
            self.assertEqual(code, 1, (kw, out))
            self.assertIn("REFUSED", out)
            self.assertEqual(snapshot(path), before, kw)


if __name__ == "__main__":
    unittest.main()
