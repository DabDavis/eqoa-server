#!/usr/bin/env python3
"""christmas_elf_santa_20261116.py on a small database: the report writes nothing; --apply makes the Holiday Elf a gnome
and dyes it, adds Santa Claus (an ogre, friendly, of the season, dyed, with its lines); a second --apply refuses; --undo
puts every table back."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "christmas_elf_santa_20261116.py")
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
		, speed REAL NOT NULL DEFAULT 0);
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
CREATE TABLE npcseason (
		name   TEXT PRIMARY KEY,
		season TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);
CREATE TABLE npcdyes (
		name   TEXT NOT NULL,
		slot   INTEGER NOT NULL,
		color  INTEGER NOT NULL,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (name, slot)
	);
"""
TABLES = ("npcs", "npcmove", "npcaggro", "npcdialog", "npcseason", "npcdyes")


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in TABLES}
    db.close()
    return out


class ElfSantaTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "es.json")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        cols = "id, npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, modelid, size, race, robe_style, npc_level, npc_type, npc_id"
        db.execute(f"INSERT INTO npcs ({cols}) VALUES (653, 'Smithy Grol', 0, 0, 0, 1, 1, 1, 0, 0, 100, 255, 2223010960, 1.3, '9', '3', 30, 129, 653)")
        db.execute(f"INSERT INTO npcs ({cols}) VALUES (190713, 'Holiday Elf', 0, 0, 0, 25262.2, 54.09, 15744.1, 64, 0, 100, 255, 3637866488, 1.0, '1', '255', 20, 128, 190713)")
        db.commit()
        db.close()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, TOOL, "--db", self.db, "--applied", self.applied, *args], capture_output=True, text=True)

    def test_apply_and_undo(self):
        before = dump(self.db)
        self.assertEqual(self.run_tool().returncode, 0)
        self.assertEqual(dump(self.db), before, "the report writes nothing")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT modelid & 4294967295, race, torso FROM npcs WHERE npc_name = 'Holiday Elf'").fetchone(), (0xA99C6B15, "3", 1))
        santa = db.execute("SELECT modelid & 4294967295, robe_style, npc_type FROM npcs WHERE npc_name = 'Santa Claus'").fetchone()
        self.assertEqual(santa, (0x84807490, "0", 128))
        self.assertEqual(db.execute("SELECT season FROM npcseason WHERE name = 'Santa Claus'").fetchone(), ("Christmas",))
        self.assertEqual(db.execute("SELECT color FROM npcdyes WHERE name = 'Santa Claus' AND slot = 9").fetchone(), (0xC81E1EFF,))
        self.assertGreaterEqual(db.execute("SELECT count(*) FROM npcdialog WHERE name = 'Santa Claus'").fetchone()[0], 3)
        db.close()
        self.assertNotEqual(self.run_tool("--apply").returncode, 0)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(dump(self.db), before, "undone")


if __name__ == "__main__":
    unittest.main()
