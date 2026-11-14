#!/usr/bin/env python3
"""tools/master_craft_20261114.py on a database of its own: the chart's 420 recipes, this server's level 60 records
kept, a craft component of a chart name reused, the sellers stocked (a missing one's wares to his level's plate and
chain man), the drops and standing lots, a second run nothing, --undo exact (the AUTOINCREMENT counters too)."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "master_craft_20261114.py")
SCHEMA = """CREATE TABLE itempattern (
		patternid INTEGER PRIMARY KEY AUTOINCREMENT,
		itemcost INTEGER NOT NULL,
		unk1 INTEGER NOT NULL,
		itemicon INTEGER NOT NULL,
		unk2 INTEGER NOT NULL,
		equipslot INTEGER NOT NULL,
		unk3 INTEGER NOT NULL,
		trade INTEGER NOT NULL,
		rent INTEGER NOT NULL,
		unk4 INTEGER NOT NULL,
		attacktype INTEGER NOT NULL,
		weapondamage INTEGER NOT NULL,
		unk5 INTEGER NOT NULL,
		levelreq INTEGER NOT NULL,
		maxstack INTEGER NOT NULL,
		maxhp INTEGER NOT NULL,
		duration INTEGER NOT NULL,
		classuse INTEGER NOT NULL,
		raceuse INTEGER NOT NULL,
		procanim INTEGER NOT NULL,
		lore INTEGER NOT NULL,
		unk6 INTEGER NOT NULL,
		craft INTEGER NOT NULL,
		itemname TEXT NOT NULL,
		itemdesc TEXT NOT NULL,
		model INTEGER NOT NULL,
		color INTEGER NOT NULL,
		str INTEGER,
		sta INTEGER,
		agi INTEGER,
		wis INTEGER,
		dex INTEGER,
		cha INTEGER,
		intelligence INTEGER,
		HPMAX INTEGER,
		POWMAX INTEGER,
		PoT INTEGER,
		HoT INTEGER,
		AC INTEGER,
		PR INTEGER,
		DR INTEGER,
		FR INTEGER,
		CR INTEGER,
		LR INTEGER,
		AR INTEGER,
		weaponproc INTEGER,
		fish INTEGER
	);
CREATE TABLE npcitem (
		itemid INTEGER PRIMARY KEY AUTOINCREMENT,
		npcid INTEGER NOT NULL,
		stackleft INTEGER NOT NULL DEFAULT 0,
		remainHP INTEGER NOT NULL DEFAULT 0,
		remaincharge INTEGER NOT NULL DEFAULT 0,
		patternid INTEGER NOT NULL,
		equiploc INTEGER NOT NULL DEFAULT 0,
		location INTEGER NOT NULL DEFAULT 1,
		listnumber INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (npcid) REFERENCES npcs(id),
		FOREIGN KEY (patternid) REFERENCES itempattern(patternid) ON DELETE CASCADE
	);
CREATE TABLE lootpool (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		category  TEXT NOT NULL,
		region    TEXT NOT NULL DEFAULT '*',
		npc_name  TEXT NOT NULL DEFAULT '',
		patternid INTEGER NOT NULL,
		weight    INTEGER NOT NULL DEFAULT 1,
		minlevel  INTEGER NOT NULL DEFAULT 1,
		maxlevel  INTEGER NOT NULL DEFAULT 100,
		source    TEXT NOT NULL DEFAULT 'gm'
	);
CREATE TABLE auction_lots (
		lot         INTEGER PRIMARY KEY AUTOINCREMENT,
		seller      INTEGER NOT NULL,
		seller_name TEXT NOT NULL,
		patternid   INTEGER NOT NULL,
		stack       INTEGER NOT NULL,
		remainhp    INTEGER NOT NULL,
		charges     INTEGER NOT NULL,
		min_bid     INTEGER NOT NULL,
		buyout      INTEGER NOT NULL,
		fee         INTEGER NOT NULL,
		listed_at   INTEGER NOT NULL,
		ends_at     INTEGER NOT NULL,
		state       TEXT NOT NULL DEFAULT 'open',
		high_bidder INTEGER NOT NULL DEFAULT 0,
		high_name   TEXT NOT NULL DEFAULT '',
		high_max    INTEGER NOT NULL DEFAULT 0,
		price       INTEGER NOT NULL DEFAULT 0,
		bids        INTEGER NOT NULL DEFAULT 0,
		closed_at   INTEGER NOT NULL DEFAULT 0,
		holder      INTEGER NOT NULL DEFAULT 0,
		delivered   INTEGER NOT NULL DEFAULT 0,
		item_from   INTEGER NOT NULL DEFAULT 0,
		item_to     INTEGER NOT NULL DEFAULT 0
	, per_item INTEGER NOT NULL DEFAULT 0, demand INTEGER NOT NULL DEFAULT 0, perpetual INTEGER NOT NULL DEFAULT 0);
CREATE TABLE gm_audit (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	at          TEXT NOT NULL,
	accountid   INTEGER NOT NULL,
	username    TEXT NOT NULL,
	via         TEXT NOT NULL,
	ip          TEXT NOT NULL,
	action      TEXT NOT NULL,
	tbl         TEXT NOT NULL DEFAULT '',
	rowkey      TEXT NOT NULL DEFAULT '',
	before      TEXT,
	after       TEXT,
	detail      TEXT NOT NULL DEFAULT '',
	reverted_by INTEGER
);
CREATE TABLE craftrecipes (
    id          INTEGER PRIMARY KEY,
    skill       INTEGER NOT NULL,
    comp        INTEGER NOT NULL,
    level       INTEGER NOT NULL,
    rarity      INTEGER NOT NULL,
    name        TEXT NOT NULL,
    product     INTEGER NOT NULL,
    yield       INTEGER NOT NULL DEFAULT 1,
    ingredients TEXT NOT NULL,
    difficulty  INTEGER NOT NULL,
    xp          INTEGER NOT NULL,
    discover    INTEGER NOT NULL DEFAULT 1,
    scroll      INTEGER NOT NULL DEFAULT 0,
    description TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT 'gm'
);
"""
SELLERS = ["Farns", "Truen", "Sams", "Gibbs", "Prat", "Yarney", "Yeatts", "Johns", "Fritts", "Glarn", "Watts", "Higgy"]
COLS = ("patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, weapondamage, unk5, "
        "levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, "
        "color")


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA + """
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, npc_type INTEGER,
            npc_id INTEGER);
    """)
    item = lambda pid, name, slot, level, craft: db.execute(
        f"INSERT INTO itempattern ({COLS}) VALUES (?, 10, 0, 1, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, 1, 0, 0, 4, 1023, 0, 0, 0, "
        "?, ?, '', 4, 0)", (pid, slot, level, craft, name))
    item(1093, "champion's helm", 1, 60, 0)
    item(996087, "Magnesium Alloy", -1, 0, 1)
    item(8306, "brass ring", 8, 1, 0)
    item(5000, "Gilded Gloves", 19, 26, 0)
    for i, who in enumerate(SELLERS):
        db.execute("INSERT INTO npcs VALUES (?, ?, 2, ?, ?)", (100 + i, "Master Craftsman " + who, 0 if i % 2 else 129,
                                                                0 if i % 3 else 100 + i))
    db.execute("INSERT INTO npcs VALUES (200, 'Master Craftsman Farns', 0, 129, 200)")  # another world's: not his
    db.execute("INSERT INTO npcitem (npcid, patternid) VALUES (100, 5000)")
    db.execute("INSERT INTO lootpool (category, patternid, weight, minlevel, maxlevel, source) "
               "VALUES ('modifiers', 996087, 9, 10, 60, 'tradeskill-seed')")
    db.commit()
    db.close()


def run(path, *args, check=True):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True,
                          check=check)


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l]
    db.close()
    return out


def q(path, sql, args=()):
    db = sqlite3.connect(path)
    try:
        return db.execute(sql, args).fetchall()
    finally:
        db.close()


class Test(unittest.TestCase):
    def test_apply_again_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = dump(p)
            out = run(p).stdout
            self.assertIn("420 recipes", out)
            self.assertIn("Master Craftsman Rhodes (Arcadin 30) is no NPC here: Master Craftsman Farns", out)
            self.assertIn("Magnesium Alloy (996087)", out)
            self.assertIn("report only", out)
            self.assertEqual(q(p, "SELECT COUNT(*) FROM craftrecipes")[0][0], 0)
            out = run(p, "--apply").stdout
            self.assertIn("applied:", out)
            self.assertEqual(q(p, "SELECT COUNT(*), MIN(level), MAX(level) FROM craftrecipes WHERE skill = 1 AND comp = 10")[0],
                             (420, 7, 10))
            # the paladin's level 60 helm is this server's record, untouched
            (ings,), = q(p, "SELECT ingredients FROM craftrecipes WHERE product = 1093")
            names = [q(p, "SELECT itemname FROM itempattern WHERE patternid = ?", (i["item"],))[0][0]
                     for i in json.loads(ings)]
            self.assertEqual(names, ["Elaborate Plate Helm Pattern", "Platinum Ore Chunk", "Nickel Rod"])
            self.assertEqual([i["keep"] for i in json.loads(ings)], [False, False, True])
            self.assertEqual(q(p, "SELECT itemcost, classuse FROM itempattern WHERE patternid = 1093")[0], (10, 4))
            # the shadowknight's 60 modifier is the weaponsmiths' alloy; the bard's a new Brass Ring
            (sk,), = q(p, "SELECT ingredients FROM craftrecipes WHERE name = 'soulripper''s tunic'")
            self.assertEqual(json.loads(sk)[2]["item"], 996087)
            self.assertEqual(q(p, "SELECT COUNT(*) FROM itempattern WHERE itemname = 'Brass Ring' AND craft = 1")[0][0], 1)
            # a new product: the class's only, its slot, level, stats
            row = q(p, "SELECT classuse, equipslot, levelreq, model, AC, str, sta FROM itempattern "
                       "WHERE itemname = 'Gilded Tunic'")
            self.assertEqual(row, [(1 << 2, 5, 30, 4, 216, 20, 10)])
            self.assertEqual(q(p, "SELECT COUNT(*) FROM itempattern WHERE itemname = 'Gilded Gloves'")[0][0], 2)
            # the rogue's level 30: pattern and metal alone, common
            (ings, rar), = q(p, "SELECT ingredients, rarity FROM craftrecipes WHERE name = 'leaf chain tunic'")
            self.assertEqual((len(json.loads(ings)), rar), (2, 0))
            # the sellers: Farns (100) his and Rhodes' (15 + 16 wares) beside what he sold; merchants flagged
            self.assertEqual(q(p, "SELECT COUNT(*) FROM npcitem WHERE npcid = 100")[0][0], 1 + 31)
            self.assertEqual(q(p, "SELECT COUNT(*) FROM npcitem WHERE npcid = 200")[0][0], 0)
            self.assertEqual(q(p, "SELECT COUNT(*) FROM npcs WHERE id BETWEEN 100 AND 111 AND (npc_type & 1) = 1 "
                                  "AND npc_id = id")[0][0], 12)
            # drops: 58 new modifiers (the alloy's own row kept)
            self.assertEqual(q(p, "SELECT COUNT(*) FROM lootpool WHERE source = 'master-craft-1114'")[0][0], 58)
            self.assertEqual(q(p, "SELECT weight, minlevel FROM lootpool WHERE patternid = 996087")[0], (9, 10))
            # standing lots at ah_materials' prices
            lot = lambda n: q(p, "SELECT l.buyout FROM auction_lots l JOIN itempattern i USING (patternid) "
                                 "WHERE i.itemname = ? AND l.perpetual = 1", (n,))[0][0]
            self.assertEqual(lot("Bronzed Tunic Pattern"), 600)
            self.assertEqual(lot("Gilded Rocks"), 100)
            self.assertEqual(lot("Nickel Rod"), 500)
            self.assertEqual(lot("Cobalt Spring"), 300)
            self.assertIn("applied already", run(p, "--apply").stdout)
            self.assertIn("put back", run(p, "--undo").stdout)
            self.assertEqual(dump(p), before)


if __name__ == "__main__":
    unittest.main()
