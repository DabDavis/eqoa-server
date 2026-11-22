#!/usr/bin/env python3
"""heirloom_marks_20261122.py on a small database of its own: the report writes nothing; --apply writes the two
records, the ring's effect, the 90 AC rows and four NPCs with their move and aggro rows, each with a gm_audit line;
a second run has nothing to do; a crowded spot refuses the run (the control: the same run uncrowded writes);
--undo takes every row back exactly. On a copy of a shard's database too when EQOA_REAL_DB names one."""
import json
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import heirloom_marks_20261122 as hm  # noqa: E402

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemcost INTEGER, itemicon INTEGER, equipslot INTEGER,
  trade INTEGER, rent INTEGER, levelreq INTEGER, maxstack INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER,
  procanim INTEGER, lore INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, color INTEGER, AC INTEGER);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '', icon INTEGER NOT NULL DEFAULT 0,
  breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0, area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm',
  body INTEGER NOT NULL DEFAULT 0, target_line TEXT NOT NULL DEFAULT '', charges INTEGER NOT NULL DEFAULT 0, cooldown REAL NOT NULL DEFAULT 0);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL, z_coord REAL,
  facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, size REAL, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER);
CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0, water TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', speed REAL NOT NULL DEFAULT 0);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
  action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER);
INSERT INTO accountinfo VALUES (1, 'dab');
INSERT INTO itempattern (patternid, itemname, equipslot, itemicon) VALUES (966001, 'Ascended Rubicite Breastplate', 5, 1),
  (966020, 'Taken', 5, 1), (716912, 'Santa''s Elf', -1, 2), (1671, 'Tome of Illusion', 18, 1856243648), (500, 'a ring', 8, 777);
INSERT INTO itemeffect (patternid, line, name, icon, cooldown) VALUES (716912, 'Self, Unlimited Uses, 30min', 'Santa''s Elf', 1507275072, 60);
INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, facing, world, modelid, size, npc_level, npc_type) VALUES
  ('Dabrish', 25306.098, 54.126, 15751.569, 61, 0, 1, 1.0, 60, 128),
  ('Rehabilitator Anja', 23431.2, 54.4, 19601.0, 240, 0, 2, 1.0, 50, 128),
  ('Rehabilitator Frejo', 24446.8, 94.72, 17039.4, 116, 0, 3, 1.0, 31, 128),
  ('Teleporter', 25305.9, 54.13, 15757.19, 63, 0, 2012826467, 1.0, 60, 128),
  ('Heirloom Outfitter', 100, 50, 100, 0, 0, 223572789, 1.0, 50, 129);
"""


def fresh():
    d = tempfile.mkdtemp()
    path = os.path.join(d, "t.db")
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.commit()
    db.close()
    return path


def run(path, mode):
    db = sqlite3.connect(path, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        if mode == "undo":
            r = hm.undo(db)
            db.execute("COMMIT")
            return r
        lines, refusals, writes = hm.plan(db)
        if refusals or mode == "dry":
            db.execute("ROLLBACK")
            return lines, refusals, writes
        n = hm.apply(db, writes)
        db.execute("COMMIT")
        return n
    finally:
        db.close()


def snapshot(path):
    db = sqlite3.connect(path)
    try:
        return {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("itempattern", "itemeffect", "npcs", "npcmove", "npcaggro")}
    finally:
        db.close()


class ToolTest(unittest.TestCase):
    def test_apply_again_and_undo(self):
        path = fresh()
        before = snapshot(path)
        lines, refusals, writes = run(path, "dry")
        self.assertEqual(refusals, [])
        self.assertEqual(snapshot(path), before, "the report wrote")
        n = run(path, "apply")
        self.assertEqual(n, len(writes))
        db = sqlite3.connect(path)
        ring = db.execute("SELECT patternid, equipslot, procanim, trade, lore, maxstack, itemicon FROM itempattern WHERE itemname = ?", (hm.RING,)).fetchone()
        tome = db.execute("SELECT patternid, maxstack, itemicon FROM itempattern WHERE itemname = ?", (hm.TOME,)).fetchone()
        # 966020 is taken here: the next free 966xxx (966000); the tome its own 966021
        self.assertEqual(ring, (966000, -1, 2, 1, 1, 1, 777))
        self.assertEqual(tome, (966021, 20, 1856243648))
        eff = db.execute("SELECT line, cooldown, icon FROM itemeffect WHERE patternid = 966000").fetchone()
        self.assertEqual(eff, ("Self, Unlimited Uses, 30min", 60.0, 1507275072))
        self.assertEqual(db.execute("SELECT count(*), sum(ac) FROM heirloomrankac").fetchone()[0], 90)
        self.assertEqual(db.execute("SELECT ac FROM heirloomrankac WHERE class = 0 AND rank = 6").fetchone()[0], 257)
        npcs = db.execute("SELECT npc_name, x_coord, z_coord, facing, npc_type FROM npcs WHERE npc_name IN (?, ?) ORDER BY id", (hm.HERALD, hm.KEEPER)).fetchall()
        self.assertEqual(len(npcs), 4)
        self.assertEqual(npcs[0][0], hm.HERALD)
        self.assertAlmostEqual(npcs[0][1], 25312.098, places=2)
        self.assertEqual(npcs[0][3], 192)  # facing Dabrish, to its west
        self.assertTrue(all(r[4] == 128 for r in npcs))
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], n)
        self.assertIn("marks", [r[1] for r in db.execute("PRAGMA table_info(quests)")])
        db.close()
        lines, refusals, writes = run(path, "dry")
        self.assertEqual((refusals, writes), ([], []), "a second run has something to do")
        back, left = run(path, "undo")
        self.assertEqual((back, left), (n, 0))
        self.assertEqual(snapshot(path), before)

    def test_crowded_spot_refuses(self):
        path = fresh()
        db = sqlite3.connect(path)
        db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world) VALUES ('Squatter', 25312.5, 54, 15751.5, 0)")
        db.commit()
        db.close()
        before = snapshot(path)
        lines, refusals, writes = run(path, "apply")
        self.assertTrue(any("Squatter" in r for r in refusals), refusals)
        self.assertEqual(snapshot(path), before)
        # the control: the same database without the squatter writes
        db = sqlite3.connect(path)
        db.execute("DELETE FROM npcs WHERE npc_name = 'Squatter'")
        db.commit()
        db.close()
        self.assertIsInstance(run(path, "apply"), int)

    def test_real_db(self):
        real = os.environ.get("EQOA_REAL_DB")
        if not real:
            self.skipTest("EQOA_REAL_DB names a copy of a shard's database")
        d = tempfile.mkdtemp()
        path = os.path.join(d, "real.db")
        shutil.copy(real, path)
        before = snapshot(path)
        n = run(path, "apply")
        self.assertIsInstance(n, int, n)
        back, left = run(path, "undo")
        self.assertEqual((back, left), (n, 0))
        self.assertEqual(snapshot(path), before)


if __name__ == "__main__":
    unittest.main()
