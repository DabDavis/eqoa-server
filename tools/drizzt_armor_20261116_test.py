#!/usr/bin/env python3
"""drizzt_armor_20261116.py on a small database: the report writes nothing; --apply makes the six scale mail records
(model 7) and the six ornate plate records (model 9), each ranger only, NO TRADE, LORE, with its own description of
at most 200 plain characters, table itemowner and a row binding each to a player, each write with a gm_audit
line; --apply --give then puts one of each in his pack, unequipped at the end of the list; a third apply finds nothing
to do; --undo puts the database back as it was (gm_audit aside), and leaves a record someone still holds. The
controls: a player who is no level 60 dark elf ranger, a taken id, a pack with no room, or a piece worse than what he
wears refuse the whole, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "drizzt_armor_20261116.py")
IDS = list(range(715043, 715049)) + list(range(715051, 715057))
SCALE_IDS, PLATE_IDS = IDS[:6], IDS[6:]

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


def item(db, pid, name, slot, agi=0, intel=0):
    db.execute("INSERT INTO itempattern VALUES (?, 0,0,0,0, ?, 0,0,1,0,0,0,0, 48,1,10000,50,32767,1023,0,0,0,0, ?, '', 1, 0,"
               " 15, 15, ?, 0, 10, 0, ?, 0, 0, 0, 0, 0, 0,0,0,0,0,0, 0, 0)", (pid, slot, name, agi, intel))


def make(path, cls=1, taken=False, full=False, strong_tunic=False):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO characters VALUES ('Drizzt', 98, 327, ?, 2, 60)", (cls,))
    db.execute("INSERT INTO characters VALUES ('Destiny', 98, 417, 1, 2, 60)")
    item(db, 2216, "Gravemold Tunic", 5, agi=95 if strong_tunic else 75, intel=80)
    db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)"
               " VALUES (327, 1, 10600, 0, 2216, 5, 0, 61)")
    if taken:
        item(db, 715045, "Someone's Bracers", 7)
    for n in range(90 if full else 0):  # 91 of 100: no room for 12
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)"
                   " VALUES (327, 1, 0, 0, 2216, -1, 0, ?)", (70 + n,))
    db.commit()
    db.close()


def run(path, *args):
    p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def snapshot(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("itempattern", "charinventory", "characters")}
    out["tables"] = db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'sqlite_sequence' ORDER BY 1").fetchall()
    db.close()
    return out


class DrizztArmor(unittest.TestCase):
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
        rows = db.execute("SELECT patternid, model, classuse, trade, rent, lore, levelreq, equipslot, itemdesc FROM itempattern "
                          "WHERE patternid BETWEEN 715043 AND 715056 ORDER BY 1").fetchall()
        self.assertEqual([r[0] for r in rows], IDS)
        self.assertEqual({r[1:7] for r in rows if r[0] in SCALE_IDS}, {(7, 2, 1, 1, 1, 60)})
        self.assertEqual({r[1:7] for r in rows if r[0] in PLATE_IDS}, {(9, 2, 1, 1, 1, 60)})
        for ids in (SCALE_IDS, PLATE_IDS):
            self.assertEqual(sorted(r[7] for r in rows if r[0] in ids), [1, 5, 7, 10, 11, 19])
        descs = [r[8] for r in rows]
        self.assertEqual(len(set(descs)), 12)
        self.assertTrue(all(0 < len(d) <= 200 and d.isascii() and "|" not in d for d in descs), descs)
        self.assertEqual(db.execute("SELECT patternid, serverid FROM itemowner ORDER BY 1").fetchall(), [(i, 327) for i in IDS])
        self.assertEqual(db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0], 25)
        for chest in (715044, 715052):
            tunic = db.execute("SELECT str, sta, agi, dex, intelligence FROM itempattern WHERE patternid = ?", (chest,)).fetchone()
            self.assertTrue(all(a >= b for a, b in zip(tunic, (15, 15, 75, 10, 80))), tunic)
        db.close()

        code, out = run(self.path, "--give", "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        given = db.execute("SELECT patternid, equiploc, location, listnumber FROM charinventory WHERE serverid = 327 "
                           "AND patternid BETWEEN 715043 AND 715056 ORDER BY listnumber").fetchall()
        self.assertEqual(given, [(pid, -1, 0, 62 + n) for n, pid in enumerate(IDS)])
        self.assertEqual(db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = 2216").fetchone()[0], 1)  # nothing taken
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
        db.execute("UPDATE charinventory SET serverid = 417 WHERE patternid = 715046")  # the account's other ranger took the gloves
        db.commit()
        db.close()
        code, out = run(self.path, "--undo")
        self.assertEqual(code, 1, out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN 715043 AND 715056").fetchall(), [(715046,)])
        self.assertEqual(db.execute("SELECT patternid, serverid FROM itemowner").fetchall(), [(715046, 327)])
        db.close()

    def test_refusals_write_nothing(self):
        for i, kw in enumerate(({"cls": 6}, {"taken": True}, {"full": True}, {"strong_tunic": True})):
            path = os.path.join(self.dir.name, f"r{i}.db")
            make(path, **kw)
            before = snapshot(path)
            code, out = run(path, "--give", "--apply")
            self.assertEqual(code, 1, (kw, out))
            self.assertIn("REFUSED", out)
            self.assertEqual(snapshot(path), before, kw)


if __name__ == "__main__":
    unittest.main()
