#!/usr/bin/env python3
"""scroll_of_returning_20261126.py on a small database: the report writes nothing; --apply makes the record (a clone of
the template scroll, its name, words, value, every class) and stocks it last in the Spell Archivist's list, with a
gm_audit line a row; a second --apply refuses; --undo puts the database back as it was (gm_audit aside) and refuses
while a character holds a copy. The controls: no Spell Archivist in world 0, two of them, and a record already so named
refuse the whole, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "scroll_of_returning_20261126.py")

SCHEMA = """
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
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER NOT NULL DEFAULT 0,
    remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
    remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL,
    location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""

ITEM = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,"
        " weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft,"
        " itemname, itemdesc, model, color) VALUES (?, 1002,0,?,0,-1,0,1,1,0,0,0,0,16,1,0,0,8192,1295,2,0,0,0, ?, 'x', 0, 4294967295)")


def make(path, sellers=1, named=False):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute(ITEM, (8623, -607649328, "Commons Gate"))
    db.execute(ITEM, (716970, 0, "Something Else"))
    for i in range(sellers):
        db.execute("INSERT INTO npcs (id, npc_name, world) VALUES (?, 'Spell Archivist', 0)", (162588 + i,))
    db.execute("INSERT INTO npcs (id, npc_name, world) VALUES (165284, 'Spell Archivist', 10)")
    for i in range(3):
        db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (162588, 1, 8623, -1, 1, ?)", (i,))
    if named:
        db.execute(ITEM, (716999, 0, "scroll of returning"))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("itempattern", "npcitem", "charinventory")}
    out["tables"] = db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY 1").fetchall()
    db.close()
    return out


class ScrollOfReturningTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_report_apply_undo(self):
        make(self.path)
        before = dump(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only", r.stdout)
        self.assertEqual(dump(self.path), before)

        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.path)
        db.row_factory = sqlite3.Row
        rec = db.execute("SELECT * FROM itempattern WHERE itemname = 'Scroll of Returning'").fetchone()
        self.assertEqual(rec["patternid"], 716971, "the first free id from 716970")
        self.assertEqual((rec["itemicon"], rec["equipslot"], rec["procanim"], rec["rent"]), (-607649328, -1, 2, 1), "the template's look")
        self.assertEqual((rec["itemcost"], rec["classuse"], rec["raceuse"], rec["levelreq"], rec["maxstack"], rec["trade"], rec["lore"]),
                         (1000, 32767, 32767, 1, 20, 0, 0))
        self.assertIn("heroic dungeon", rec["itemdesc"])
        stock = db.execute("SELECT npcid, patternid, listnumber FROM npcitem WHERE patternid = 716971").fetchall()
        self.assertEqual([tuple(s) for s in stock], [(162588, 716971, 3)], "last in the world 0 archivist's list")
        self.assertEqual(db.execute("SELECT COUNT(*) FROM gm_audit WHERE action = 'apply'").fetchone()[0], 2)
        db.close()

        r = run(self.path, "--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("holds rows", r.stdout)

        # undo refused while a character holds one
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (7, 1, 0, 0, 716971, -1, 0, 0)")
        db.commit()
        r = run(self.path, "--undo")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("holds", r.stdout + r.stderr)
        db.execute("DELETE FROM charinventory")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before)

    def test_refusals(self):
        for kw, why in (({"sellers": 0}, "0 NPCs named"), ({"sellers": 2}, "2 NPCs named"), ({"named": True}, "there already")):
            with self.subTest(**kw):
                make(self.path, **kw)
                before = dump(self.path)
                r = run(self.path, "--apply")
                self.assertNotEqual(r.returncode, 0)
                self.assertIn(why, r.stdout)
                self.assertEqual(dump(self.path), before)
                os.remove(self.path)


if __name__ == "__main__":
    unittest.main()
