#!/usr/bin/env python3
"""toy_shoppe_20261116.py on a small database: the report writes nothing; --apply writes the three toys' records (usable,
worn nowhere, LORE, NO TRADE, worth 0, no binding), Santa's Elf's use and outfit, the shop's stock at its prices and the
shopkeeper (a gnome of the Christmas season, standing, friendly) with a gm_audit line a row; a second --apply finds
nothing to do; --undo puts the database back as it was (gm_audit aside) and leaves a toy while someone holds a copy.
The controls: an id taken by another record, an NPC standing on the spot, a bound record, no Christmas season refuse
the whole, writing nothing. Its schemas are database/tokenshop.go's, word for word."""
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "toy_shoppe_20261116.py")
GO = os.path.join(HERE, "..", "internal", "database", "tokenshop.go")
sys.path.insert(0, HERE)
import toy_shoppe_20261116 as toy  # noqa: E402

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
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
    icon INTEGER NOT NULL DEFAULT 0, breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0,
    area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm', body INTEGER NOT NULL DEFAULT 0,
    target_line TEXT NOT NULL DEFAULT '', charges INTEGER NOT NULL DEFAULT 0, cooldown REAL NOT NULL DEFAULT 0);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
    remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL,
    location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, unk_a INTEGER, unk_b INTEGER,
    x_coord REAL, y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, hp_percent INTEGER,
    modelid INTEGER, size REAL, unk_d INTEGER, unk_f INTEGER, unk_g INTEGER, primary_weapon INTEGER,
    secondary_weapon INTEGER, shield INTEGER, unk_h INTEGER, torso INTEGER, forearm INTEGER, gloves INTEGER, legs INTEGER,
    feet INTEGER, head INTEGER, hair_color INTEGER, hair_length INTEGER, hair_style INTEGER, face INTEGER,
    robe_style TEXT, race TEXT, unk_j INTEGER, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER, unk_k INTEGER,
    unk_i TEXT, pd_icon INTEGER, unk_m INTEGER, unk_n INTEGER, unk_o INTEGER);
CREATE TABLE gmseasons (name TEXT PRIMARY KEY COLLATE NOCASE, active INTEGER NOT NULL DEFAULT 0,
    changed TEXT NOT NULL DEFAULT '', by TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '');
CREATE TABLE npcseason (name TEXT PRIMARY KEY, season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0,
    water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    speed REAL NOT NULL DEFAULT 0);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', leash REAL NOT NULL DEFAULT 0,
    holding INTEGER NOT NULL DEFAULT 0, see_invis INTEGER NOT NULL DEFAULT 0);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""

ITEM = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,"
        " weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft,"
        " itemname, itemdesc, model, color) VALUES (?, 1,0,0,0,-1,0,0,1,0,0,0,0,0,999,0,0,0,0,0,0,0,1, ?, '', 0, 0)")
NPC = ("INSERT INTO npcs (id, npc_name, zone, x_coord, y_coord, z_coord, facing, world, modelid, size, race, npc_type, unk_a, unk_b)"
       " VALUES (?, ?, 0, ?, 54.1, ?, 0, 0, 1, 1.0, '1', 0, 1, 82)")


def make(path, taken=False, crowded=False, bound=False, season=True):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute(ITEM, (716900, "Holly Token"))
    db.execute(NPC, (405, "Captain Scilond", 25291.1, 15768.1))
    db.execute(NPC, (190712, "Christmas Tree", 25270.0, 15754.7))
    db.execute(NPC, (190637, "Weapon Enchanter", 25284.76, 15747.96))
    if season:
        db.execute("INSERT INTO gmseasons (name, active) VALUES ('Christmas', 1)")
    if taken:
        db.execute(ITEM, (716911, "Something Else"))
    if crowded:
        db.execute(NPC, (77, "Busker", 25283.0, 15745.5))
    if bound:
        db.execute("CREATE TABLE itemowner (patternid INTEGER PRIMARY KEY, serverid INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '')")
        db.execute("INSERT INTO itemowner VALUES (716910, 5, '')")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def snapshot(path):
    db = sqlite3.connect(path)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT IN ('gm_audit', 'sqlite_sequence')"):
        out[t] = sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
    db.close()
    return out


class ToyShoppe(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_schemas_are_the_servers(self):
        with open(GO) as f:
            go = f.read()
        for name, ddl in (("TokenShopSchema", toy.TOKENSHOP_DDL), ("ItemOutfitSchema", toy.ITEMOUTFIT_DDL)):
            m = re.search(name + r" = `(.*?)`", go, re.S)
            self.assertIsNotNone(m, name)
            self.assertEqual(m.group(1), ddl, name)

    def test_apply_then_again_then_undo(self):
        make(self.path)
        before = snapshot(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(snapshot(self.path), before, "the report wrote")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        for t in toy.TOYS:
            (rec,) = self.q("SELECT itemname, equipslot, procanim, lore, trade, itemcost, maxstack, itemdesc FROM itempattern WHERE patternid = ?", t["id"])
            self.assertEqual(rec[:7], (t["name"], -1, 2, 1, 1, 0, 1))
            self.assertLessEqual(len(rec[7]), 200)
            self.assertNotIn("alone", rec[7])
            self.assertEqual(self.q("SELECT price, token, sort FROM tokenshop WHERE shop = ? AND patternid = ?", toy.SHOP, t["id"]),
                             [(t["price"], "", t["sort"])])
        self.assertEqual(self.q("SELECT line, name, body, cooldown FROM itemeffect WHERE patternid = 716912"),
                         [("Self, Unlimited Uses, 30min", "Santa's Elf", 0xA99C6B15, 60.0)])
        outfit = dict((s, (look, c)) for s, look, c in self.q("SELECT slot, look, color FROM itemoutfit WHERE patternid = 716912"))
        self.assertEqual(outfit, toy.ELF_OUTFIT)
        (npc,) = self.q("SELECT npc_name, modelid, race, npc_type, x_coord, z_coord, facing, world, npc_id = id FROM npcs WHERE npc_name = ?", toy.SHOP)
        self.assertEqual(npc, (toy.SHOP, 0xA99C6B15, "3", 128, 25282.2, 15744.8, 96, 0, 1))
        self.assertEqual(self.q("SELECT season FROM npcseason WHERE name = ?", toy.SHOP), [("Christmas",)])
        self.assertEqual(self.q("SELECT roam FROM npcmove WHERE name = ?", toy.SHOP.lower()), [(0,)])
        self.assertEqual(self.q("SELECT aggressive FROM npcaggro WHERE npc_name = ?", toy.SHOP), [(0,)])
        n = self.q(f"SELECT count(*) FROM {toy.LOG}")[0][0]
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit WHERE action = 'insert'")[0][0], n)
        r = run(self.path, "--apply")
        self.assertIn("nothing to do", r.stdout)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        after = snapshot(self.path)
        for t in ("tokenshop", "itemoutfit"):
            self.assertEqual(after.pop(t), [], t)
        self.assertEqual(after, before)

    def test_undo_keeps_a_held_toy(self):
        make(self.path)
        run(self.path, "--apply")
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)"
                   " VALUES (5, 1, 0, 0, 716912, -1, 0, 1)")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertEqual(self.q("SELECT itemname FROM itempattern WHERE patternid = 716912"), [("Santa's Elf",)])
        self.assertEqual(self.q("SELECT count(*) FROM itemoutfit")[0][0], len(toy.ELF_OUTFIT))
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE patternid IN (716910, 716911)")[0][0], 0)
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE npc_name = ?", toy.SHOP)[0][0], 0)

    def test_refusals_write_nothing(self):
        for kw, word in (({"taken": True}, "record 716911"), ({"crowded": True}, "Busker"), ({"bound": True}, "itemowner"),
                         ({"season": False}, "season Christmas")):
            with self.subTest(**kw):
                if os.path.exists(self.path):
                    os.remove(self.path)
                make(self.path, **kw)
                before = snapshot(self.path)
                r = run(self.path, "--apply")
                self.assertEqual(r.returncode, 1, r.stdout)
                self.assertIn("REFUSED", r.stdout)
                self.assertIn(word, r.stdout)
                self.assertEqual(snapshot(self.path), before)


if __name__ == "__main__":
    unittest.main()
