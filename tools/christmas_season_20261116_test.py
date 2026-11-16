#!/usr/bin/env python3
"""christmas_season_20261116.py on a small database with the server's own tables: the report writes nothing; --apply
makes the tree (the treant's row copied, at Dust's spot, friendly, its art the sunk fir), the season (off), the lights'
template and its four strings, the dialog, the trigger at the next free slot past the menu's eight, and gm_audit lines;
a second --apply refuses; --undo takes every row away again, the tables as they were."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "christmas_season_20261116.py")

SCHEMA = """
CREATE TABLE npcs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		npc_name TEXT,
		zone INTEGER,
		unk_a INTEGER,
		unk_b INTEGER,
		x_coord REAL,
		y_coord REAL,
		z_coord REAL,
		facing INTEGER,
		world INTEGER,
		dt_kill DATE,
		hp INTEGER,
		hp_percent INTEGER,
		modelid INTEGER,
		size REAL,
		unk_d INTEGER,
		unk_f INTEGER,
		unk_g INTEGER,
		primary_weapon INTEGER,
		secondary_weapon INTEGER,
		shield INTEGER,
		unk_h INTEGER,
		torso INTEGER,
		forearm INTEGER,
		gloves INTEGER,
		legs INTEGER,
		feet INTEGER,
		head INTEGER,
		hair_color INTEGER,
		hair_length INTEGER,
		hair_style INTEGER,
		face INTEGER,
		robe_style TEXT,
		race TEXT,
		unk_j INTEGER,
		npc_level INTEGER,
		npc_type INTEGER,
		npc_id INTEGER,
		unk_k INTEGER,
		unk_i TEXT,
		pd_icon INTEGER,
		unk_m INTEGER,
		unk_n INTEGER,
		unk_o INTEGER
	);
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
CREATE TABLE IF NOT EXISTS "npcmove" (
			name   TEXT PRIMARY KEY,
			roam   INTEGER NOT NULL DEFAULT -1,
			radius REAL NOT NULL DEFAULT 0,
			water  TEXT NOT NULL DEFAULT '',
			note   TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'gm'
		);
CREATE TABLE npcaggro (
    npc_name   TEXT PRIMARY KEY,
    aggressive INTEGER NOT NULL DEFAULT 1,
    radius     REAL NOT NULL DEFAULT 0,
    note       TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT 'gm'
, leash REAL NOT NULL DEFAULT 0, holding INTEGER NOT NULL DEFAULT 0, see_invis INTEGER NOT NULL DEFAULT 0);
CREATE TABLE npcdialog (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	town   TEXT NOT NULL DEFAULT '',
	role   TEXT NOT NULL DEFAULT '',
	name   TEXT NOT NULL DEFAULT '',
	mood   TEXT NOT NULL DEFAULT '',
	race   INTEGER NOT NULL DEFAULT -1,
	line   TEXT NOT NULL,
	weight INTEGER NOT NULL DEFAULT 1,
	source TEXT NOT NULL DEFAULT 'gm'
, sex TEXT NOT NULL DEFAULT '', speaker_race INTEGER NOT NULL DEFAULT -1, away INTEGER NOT NULL DEFAULT 0);
CREATE TABLE gmtriggers (
		slot    INTEGER PRIMARY KEY,
		name    TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		scope   TEXT NOT NULL DEFAULT 'near',
		radius  REAL NOT NULL DEFAULT 0,
		world   INTEGER NOT NULL DEFAULT -1,
		x       REAL NOT NULL DEFAULT 0,
		y       REAL NOT NULL DEFAULT 0,
		z       REAL NOT NULL DEFAULT 0,
		steps   TEXT NOT NULL DEFAULT '',
		notes   TEXT NOT NULL DEFAULT ''
	);
CREATE TABLE gmseasons (
		name    TEXT PRIMARY KEY COLLATE NOCASE,
		active  INTEGER NOT NULL DEFAULT 0,
		changed TEXT NOT NULL DEFAULT '',
		by      TEXT NOT NULL DEFAULT '',
		note    TEXT NOT NULL DEFAULT ''
	);
CREATE TABLE npcseason (
		name   TEXT PRIMARY KEY,
		season TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);
CREATE TABLE lighttemplates (
		name    TEXT PRIMARY KEY COLLATE NOCASE,
		colors  TEXT NOT NULL DEFAULT 'FFFFFF',
		pattern TEXT NOT NULL DEFAULT 'cycle',
		effect  TEXT NOT NULL DEFAULT 'twinkle',
		period  REAL NOT NULL DEFAULT 1.5,
		spacing REAL NOT NULL DEFAULT 1,
		size    REAL NOT NULL DEFAULT 0.3,
		bright  REAL NOT NULL DEFAULT 1,
		cord    TEXT NOT NULL DEFAULT '1E3F1E',
		cord_width REAL NOT NULL DEFAULT 0.06,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);
CREATE TABLE lightstrings (
		id       INTEGER PRIMARY KEY,
		template TEXT NOT NULL DEFAULT '',
		npc      TEXT NOT NULL DEFAULT '',
		world    INTEGER NOT NULL DEFAULT 0,
		shape    TEXT NOT NULL DEFAULT 'line',
		params   TEXT NOT NULL DEFAULT '{}',
		season   TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);
CREATE TABLE decorart (
		name   TEXT PRIMARY KEY,
		model  INTEGER NOT NULL DEFAULT 0,
		world  INTEGER NOT NULL DEFAULT 0,
		zone   INTEGER NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	, sink REAL NOT NULL DEFAULT 0);
CREATE TABLE itempattern (
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
CREATE TABLE quests (
	questid    INTEGER PRIMARY KEY,
	name       TEXT NOT NULL DEFAULT '',
	enabled    INTEGER NOT NULL DEFAULT 0,
	giver      TEXT NOT NULL DEFAULT '',
	minlevel   INTEGER NOT NULL DEFAULT 1,
	maxlevel   INTEGER NOT NULL DEFAULT 60,
	classes    TEXT NOT NULL DEFAULT '',
	races      TEXT NOT NULL DEFAULT '',
	requires   TEXT NOT NULL DEFAULT '',
	repeatable INTEGER NOT NULL DEFAULT 0,
	offer      TEXT NOT NULL DEFAULT '',
	accept     TEXT NOT NULL DEFAULT '',
	decline    TEXT NOT NULL DEFAULT '',
	declined   TEXT NOT NULL DEFAULT '',
	started    TEXT NOT NULL DEFAULT '',
	source     TEXT NOT NULL DEFAULT '',
	notes      TEXT NOT NULL DEFAULT ''
, events TEXT NOT NULL DEFAULT '', factionreq TEXT NOT NULL DEFAULT '', excludes TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (
	questid     INTEGER NOT NULL,
	step        INTEGER NOT NULL,
	kind        TEXT NOT NULL DEFAULT 'talk',
	target      TEXT NOT NULL DEFAULT '',
	npcid       INTEGER NOT NULL DEFAULT 0,
	items       TEXT NOT NULL DEFAULT '',
	tunar       INTEGER NOT NULL DEFAULT 0,
	count       INTEGER NOT NULL DEFAULT 1,
	world       INTEGER NOT NULL DEFAULT 0,
	x           REAL NOT NULL DEFAULT 0,
	y           REAL NOT NULL DEFAULT 0,
	z           REAL NOT NULL DEFAULT 0,
	radius      REAL NOT NULL DEFAULT 15,
	journal     TEXT NOT NULL DEFAULT '',
	answer      TEXT NOT NULL DEFAULT '',
	says        TEXT NOT NULL DEFAULT '',
	waiting     TEXT NOT NULL DEFAULT '',
	notyet      TEXT NOT NULL DEFAULT '',
	reminder    TEXT NOT NULL DEFAULT '',
	xp          INTEGER NOT NULL DEFAULT 0,
	rewardtunar INTEGER NOT NULL DEFAULT 0,
	rewarditems TEXT NOT NULL DEFAULT '',
	factions    TEXT NOT NULL DEFAULT '',
	spells      TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '', chance REAL NOT NULL DEFAULT 0, byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '', height REAL NOT NULL DEFAULT 0,
	PRIMARY KEY (questid, step)
);
CREATE TABLE seasonrewards (
		season     TEXT PRIMARY KEY COLLATE NOCASE,
		item       INTEGER NOT NULL DEFAULT 0,
		drop_pct   REAL NOT NULL DEFAULT 0,
		named_pct  REAL NOT NULL DEFAULT 0,
		gift_npc   TEXT NOT NULL DEFAULT '',
		gift_count INTEGER NOT NULL DEFAULT 0,
		hourly     INTEGER NOT NULL DEFAULT 0,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm'
	);
CREATE TABLE seasongifts (
		season   TEXT NOT NULL,
		serverid INTEGER NOT NULL,
		day      TEXT NOT NULL,
		PRIMARY KEY (season, serverid)
	);
CREATE TABLE seasonplay (
		season   TEXT NOT NULL,
		serverid INTEGER NOT NULL,
		seconds  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (season, serverid)
	);
"""

TABLES = ("npcs", "npcmove", "npcaggro", "npcdialog", "gmtriggers", "gmseasons", "npcseason", "lighttemplates", "lightstrings", "decorart",
          "itempattern", "quests", "queststeps", "seasonrewards", "seasongifts", "seasonplay")


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in TABLES}
    db.close()
    return out


class ChristmasSeasonTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "applied.json")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO npcs (id, npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, modelid, size, "
                   "npc_level, npc_type, npc_id) VALUES (113208, 'Oakhollow', 94, 1, 82, 7279.3, 80.6, 7553.0, 104, 0, 1, 255, 1178150727, 1.0, 44, 0, 113208)")
        db.execute("INSERT INTO npcs (id, npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, modelid, size, "
                   "npc_level, npc_type, npc_id) VALUES (405, 'Captain Scilond', 0, 0, 0, 25271.4, 54.3, 15755.7, 0, 0, 100, 255, -657100808, 1.0, 60, 128, 405)")
        cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
        vals = {c: 0 for c in cols}
        vals.update(patternid=700545, itemicon=1342617881, trade=1, rent=1, maxstack=999, itemname="Defaced Coin", itemdesc="A defaced coin.")
        db.execute(f"INSERT INTO itempattern ({','.join(cols)}) VALUES ({','.join('?' * len(cols))})", [vals[c] for c in cols])
        db.execute("INSERT INTO seasongifts (season, serverid, day) VALUES ('christmas', 5, '2026-12-01')")
        for slot in range(1, 12):
            db.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (?, ?, '[]')", (slot, f"t{slot}"))
        db.commit()
        db.close()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, TOOL, "--db", self.db, "--applied", self.applied, *args], capture_output=True, text=True)

    def test_apply_and_undo(self):
        before = dump(self.db)
        r = self.run_tool()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(dump(self.db), before, "the report writes nothing")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.db)
        tree = db.execute("SELECT x_coord, y_coord, z_coord, npc_type, npc_level, modelid, size, world FROM npcs WHERE npc_name = 'Christmas Tree'").fetchall()
        self.assertEqual(len(tree), 1)
        x, y, z, typ, lvl, model, size, world = tree[0]
        self.assertEqual((round(x, 1), round(y, 3), round(z, 1), typ, lvl, model & 0xFFFFFFFF, world), (25303.8, 54.094, 15774.6, 128, 1, 0x46392747, 0))
        self.assertEqual(db.execute("SELECT name, active FROM gmseasons").fetchall(), [("Christmas", 0)], "the season starts off")
        self.assertEqual(db.execute("SELECT model, zone, sink FROM decorart").fetchall(), [(0x0E318233, 32, 13.0)])
        self.assertEqual(db.execute("SELECT count(*) FROM lightstrings WHERE npc = 'Christmas Tree' AND season = 'Christmas'").fetchone()[0], 4)
        slot, steps = db.execute("SELECT slot, steps FROM gmtriggers WHERE name = 'Christmas Season'").fetchone()
        self.assertEqual(slot, 12, "the next free slot past the menu's eight")
        self.assertEqual(json.loads(steps)[0]["do"], "season")
        self.assertGreater(db.execute("SELECT count(*) FROM gm_audit WHERE detail LIKE '%christmas_season%'").fetchone()[0], 8)
        tok = db.execute("SELECT itemname, maxstack, trade, itemicon FROM itempattern WHERE patternid = 716900").fetchone()
        self.assertEqual(tok, ("Holly Token", 999, 0, 140265582), "stackable, tradeable")
        self.assertEqual(db.execute("SELECT item, drop_pct, named_pct, gift_npc, gift_count, hourly FROM seasonrewards").fetchall(),
                         [(716900, 4.0, 25.0, "Holiday Elf", 3, 1)])
        self.assertEqual(db.execute("SELECT questid, repeatable, giver FROM quests ORDER BY 1").fetchall(),
                         [(102901, 2, "Holiday Elf"), (102902, 2, "Holiday Elf")])
        self.assertEqual(db.execute("SELECT rewarditems FROM queststeps WHERE rewarditems != ''").fetchall(), [("716900*3",), ("716900*3",)])
        self.assertEqual(db.execute("SELECT name, season FROM npcseason ORDER BY 1").fetchall(), [("Christmas Tree", "Christmas"), ("Holiday Elf", "Christmas")])
        db.close()
        r = self.run_tool("--apply")
        self.assertNotEqual(r.returncode, 0, "a second --apply refuses")
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        after = dump(self.db)
        self.assertEqual(after.pop("seasongifts"), [], "undone: the season's gifts forgotten")
        before.pop("seasongifts")
        self.assertEqual(after, before, "undone: the tables as they were")


if __name__ == "__main__":
    unittest.main()
