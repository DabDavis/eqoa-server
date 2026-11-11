#!/usr/bin/env python3
"""norach_20261111.py on a small database of the live schema: the report writes nothing; --apply makes the medallion
quest a draft and the generals' quest require nothing, each with its gm_audit line; a second apply is refused; --undo
puts both back exactly (a dump without gm_audit). Norach on the islet is left; with no Norach a row is made on the
islet (and undone); with Norach elsewhere it refuses, writing nothing; with no portal quests (another shard's import)
there is nothing to write."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "norach_20261111.py")

SCHEMA = """CREATE TABLE quests (
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
, events TEXT NOT NULL DEFAULT '', factionreq TEXT NOT NULL DEFAULT '');
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
CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL, digest TEXT NOT NULL, enabled INTEGER NOT NULL);
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
INSERT INTO accountinfo VALUES (7, 'dab');
"""


class NorachTest(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        self.addCleanup(os.remove, self.path)
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.executescript("""
            INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (101309, 'The Medallion of the Faceless', 1, 'Norach', 50);
            INSERT INTO quests (questid, name, enabled, giver, minlevel, requires, factionreq)
                VALUES (101311, 'The Faceless Generals', 1, 'Graumord citizen', 50, '101309', 'Citizens of Graumord:1');
            INSERT INTO quest_import_quests VALUES ('Portal/isle-of-dread/portal', 101309, 'portal.json', 'x', 1);
            INSERT INTO quest_import_quests VALUES ('Portal/isle-of-dread/trials-1', 101311, 'portal.json', 'y', 1);
            INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, npc_id) VALUES (117982, 'Norach', 0, 15066.6, 55.0469, 31311.0, 117982);
            INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, npc_id) VALUES (117990, 'Someone', 0, 1, 2, 3, 117990);
        """)
        db.commit()
        db.close()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def dump(self):
        db = sqlite3.connect(self.path)
        try:
            return [l for l in db.iterdump() if "gm_audit" not in l]
        finally:
            db.close()

    def test_apply_and_undo(self):
        before = self.dump()
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 2 rows", out)
        self.assertEqual(self.dump(), before)
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT questid, enabled, requires FROM quests ORDER BY questid"),
                         [(101309, 0, ""), (101311, 1, "")])
        self.assertIn("left as he is", out)
        self.assertEqual(self.q("SELECT count(*), min(accountid) FROM gm_audit"), [(2, 7)])
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("applied already", out)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.dump(), before)

    def test_no_norach_made_on_the_islet(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM npcs WHERE id = 117982")
        db.commit()
        db.close()
        before = self.dump()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        rows = self.q("SELECT id, npc_id, world, x_coord, z_coord, modelid, npc_level FROM npcs WHERE npc_name = 'Norach'")
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0][0], rows[0][1])
        self.assertEqual(rows[0][2:], (0, 15066.6, 31311.0, 874357398, 63))
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.dump(), before)

    def test_norach_elsewhere_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET x_coord = 20000 WHERE id = 117982")
        db.commit()
        db.close()
        before = self.dump()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("Norach stands elsewhere", out)
        self.assertEqual(self.dump(), before)

    def test_another_shard_without_the_quests(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM quest_import_quests")
        db.commit()
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(0,)])


if __name__ == "__main__":
    unittest.main()
