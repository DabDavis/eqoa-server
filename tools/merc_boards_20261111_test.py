#!/usr/bin/env python3
"""merc_boards_20261111.py on a small database: the report writes nothing; --apply makes the nine liaisons boards
where they stand (a GM's move kept), adds the four starter cities' boards and the new name's npcmove and npcaggro rows
with gm_audit lines; a second apply does nothing; --undo puts every table back exactly. The controls: a liaison row
missing or renamed, an NPC on a city's spot, and a row changed after the apply (undo refused) write nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import merc_boards_20261111 as mb  # noqa: E402

TOOL = os.path.join(HERE, "merc_boards_20261111.py")

SCHEMA = """
CREATE TABLE npcs ( id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, unk_a INTEGER, unk_b INTEGER,
 x_coord REAL, y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, hp_percent INTEGER,
 modelid INTEGER, size REAL, unk_d INTEGER, unk_f INTEGER, unk_g INTEGER, primary_weapon INTEGER, secondary_weapon INTEGER,
 shield INTEGER, unk_h INTEGER, torso INTEGER, forearm INTEGER, gloves INTEGER, legs INTEGER, feet INTEGER, head INTEGER,
 hair_color INTEGER, hair_length INTEGER, hair_style INTEGER, face INTEGER, robe_style TEXT, race TEXT, unk_j INTEGER,
 npc_level INTEGER, npc_type INTEGER, npc_id INTEGER, unk_k INTEGER, unk_i TEXT, pd_icon INTEGER, unk_m INTEGER,
 unk_n INTEGER, unk_o INTEGER );
CREATE TABLE npcmove ( name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0,
 water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm' );
CREATE TABLE npcaggro ( npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
 note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm' , leash REAL NOT NULL DEFAULT 0,
 holding INTEGER NOT NULL DEFAULT 0);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
 username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
 rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


def make(path, rename=None, missing=None, blocker=None):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    spots = [(25306.98, 54.13, 15739.6, 61, 0), (15300.8, 99.0, 8828.6, 1, 0), (24952.6, 31.9, 9389.5, 0, 0),
             (13214.6, 54.12, 4382.29, 0, 0), (18387.8, 54.92, 7525.84, 0, 0), (19310.3, 54.12, 6727.3, 0, 0),
             (9209.19, 57.66, 12733.1, 128, 0), (8950.5, 54.12, 7225.55, 176, 1), (4610.28, 57.66, 17186.87, 10, 0)]
    for i, (x, y, z, f, w) in zip(mb.ROWS, spots):
        if i == missing:
            continue
        name = rename if i == mb.ROWS[0] and rename else "Mercenary Liaison"
        db.execute("""INSERT INTO npcs (id, npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp,
                      hp_percent, modelid, size, primary_weapon, secondary_weapon, shield, torso, forearm, gloves, legs, feet,
                      head, hair_color, hair_length, hair_style, face, robe_style, race, npc_level, npc_type, npc_id)
                      VALUES (?, ?, 0, 0, 0, ?, ?, ?, ?, ?, 100, 255, 1893243078, 1.0, 4102560063, 0, 876411255, 6, 6, 6, 6,
                      6, 7, 0, 0, 1, 0, '255', '0', 60, 128, ?)""", (i, name, x, y, z, f, w, i))
    db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing) VALUES (163624, 'Banker Kelshan', 0, 4632.2, 54.8, 21399.7, 193)")
    if blocker:
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, facing) VALUES ('a rat', 0, ?, 54.0, ?, 0)", blocker)
    db.execute("INSERT INTO npcmove VALUES ('mercenary liaison', 0, 0, '', 'stands', 'merc liaisons 20261110')")
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, note, source) VALUES ('Mercenary Liaison', 0, 'talk only', 'merc liaisons 20261110')")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    try:
        out = [l for l in db.iterdump() if "gm_audit" not in l]
        return out
    finally:
        db.close()


class MercBoardsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        self.applied = os.path.join(self.dir.name, "applied.json")

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--applied", self.applied, *args],
                           capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        make(self.path)
        # a GM moved Freeport's against its well (the panel, 2026-11-11): the move is kept
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET x_coord = 25299.5, z_coord = 15755.0, facing = 200 WHERE id = 162678")
        db.commit()
        db.close()
        before = dump(self.path)
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.path), before, "the report wrote")
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        boards = self.q("SELECT id, npc_name, modelid, size, primary_weapon, shield, head, hair_style FROM npcs WHERE id BETWEEN 162678 AND 162686")
        self.assertEqual(len(boards), 9)
        for b in boards:
            self.assertEqual(b[1:], ("Mercenary Recruitment", 638497839, 1.0, 0, 0, 0, 0), b)
        self.assertEqual(self.q("SELECT x_coord, z_coord, facing FROM npcs WHERE id = 162678"), [(25299.5, 15755.0, 200)])
        new = self.q("SELECT id, npc_id, world, x_coord, y_coord, z_coord, facing, modelid, npc_type, npc_level FROM npcs WHERE id > 163624 ORDER BY id")
        self.assertEqual([r[3:6] for r in new], [c[1] for c in mb.CITIES])
        for r in new:
            self.assertEqual((r[1], r[2], r[7], r[8], r[9]), (r[0], 0, 638497839, 128, 60), r)
        self.assertEqual(new[0][6], 0, "Highbourne's faces its bankers to the north (+z)")
        self.assertEqual(self.q("SELECT roam FROM npcmove WHERE name = 'mercenary recruitment'"), [(0,)])
        self.assertEqual(self.q("SELECT aggressive FROM npcaggro WHERE npc_name = 'Mercenary Recruitment'"), [(0,)])
        audits = self.q("SELECT count(*) FROM gm_audit")[0][0]
        self.assertEqual(audits, 9 + 4 + 2)
        applied = dump(self.path)

        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)
        self.assertEqual(dump(self.path), applied)

        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.path), before)

    def test_refusals(self):
        for kw, want in [({"missing": 162683}, "162683"), ({"rename": "Mercenary Quimbly"}, "not a Mercenary Liaison"),
                         ({"blocker": (23655.0, 6242.0)}, "Klick'Anon: a rat")]:
            with self.subTest(**{k: str(v) for k, v in kw.items()}):
                if os.path.exists(self.path):
                    os.remove(self.path)
                make(self.path, **kw)
                before = dump(self.path)
                code, out = self.run_tool("--apply")
                self.assertNotEqual(code, 0, out)
                self.assertIn(want, out)
                self.assertEqual(dump(self.path), before)
                self.assertFalse(os.path.exists(self.applied))

    def test_undo_refused_after_an_edit(self):
        make(self.path)
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET x_coord = x_coord + 1 WHERE id = 162680")
        db.commit()
        db.close()
        edited = dump(self.path)
        code, out = self.run_tool("--undo")
        self.assertNotEqual(code, 0, out)
        self.assertIn("162680 changed", out)
        self.assertEqual(dump(self.path), edited)


if __name__ == "__main__":
    unittest.main()
