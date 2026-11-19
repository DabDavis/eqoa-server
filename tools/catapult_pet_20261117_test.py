#!/usr/bin/env python3
"""catapult_pet_20261117.py on a small database: the report writes nothing; --apply adds decorart's pet columns, the
rank Construct (Catapult) (PetArt 0xE64B8D78, OnlyFor 747, free, under Construct), the two decorart pet rows
(Tunaria zone 108, Rathe zone 13) and a player's charcms grant, one gm_audit line each, and keeps them in --applied; a
second report finds nothing to do and a second --apply is refused; --undo puts the rows back exactly (the columns
stay). --no-grant leaves charcms alone. The controls: serverid 747 not a player (Live), refused, nothing written; a row
changed since, --undo refused, nothing undone."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "catapult_pet_20261117.py")
SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1, name TEXT NOT NULL DEFAULT '',
    parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0, level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0,
    minstats TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '',
    effects TEXT NOT NULL DEFAULT '', adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '',
    remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'gm', require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '');
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, race INTEGER);
CREATE TABLE decorart (name TEXT PRIMARY KEY, model INTEGER NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT 0,
    zone INTEGER NOT NULL DEFAULT 0, sink REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO cmcatalog (id, category, races, mastery, rank, name, cost, level) VALUES (1541, 4, 8, 'Tinker', 1, 'Tinker', 200, 55);
INSERT INTO cmcatalog (id, category, races, mastery, rank, name, parent, cost, level) VALUES (1542, 4, 8, 'Tinker', 2, 'Construct', 1541, 75, 60);
INSERT INTO cmcatalog (id, category, races, mastery, rank, name, parent, effects) VALUES (900918, 4, 8, 'Tinker', 4, 'Construct (Giant Spider)', 1542, 'PetLook:4191639254,OnlyFor:747');
INSERT INTO characters VALUES (747, 'Zerkziee', 3);
INSERT INTO charcms VALUES (747, 1541, 200, 1), (747, 1542, 75, 2), (747, 900918, 0, 3);
INSERT INTO decorart (name, model, world, zone, sink, source) VALUES ('Christmas Tree', 3300788006, 0, 24, 9.6, 'christmas-2026');
INSERT INTO petstats VALUES (901835, 'Construct'), (901836, 'Construct'), (901837, 'Construct');
"""
QUERIES = ("SELECT * FROM cmcatalog ORDER BY id", "SELECT name, model, world, zone, sink, note, source FROM decorart ORDER BY name",
           "SELECT * FROM charcms ORDER BY cmid")


class CatapultPet(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        self.applied = os.path.join(self.dir.name, "applied.json")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--applied", self.applied, *args],
                              capture_output=True, text=True)

    def q(self, sql):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql).fetchall()
        finally:
            db.close()

    def state(self):
        return tuple(self.q(s) for s in QUERIES)

    def test_report_apply_again_undo(self):
        was = self.state()
        r = self.run_tool()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 4 rows", r.stdout)
        self.assertEqual(self.state(), was)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rank = self.q("SELECT id, mastery, rank, parent, cost, level, races, requires, require_spells, effects, source FROM cmcatalog WHERE name = 'Construct (Catapult)'")
        self.assertEqual(rank, [(900919, "Tinker", 4, 1542, 0, 60, 8, "Construct@60", "901835|901836|901837",
                                 "PetArt:3863711096,OnlyFor:747", "catapult-pet-1117")])
        art = self.q("SELECT name, model, world, zone, kind, shot, shot_scale, scale FROM decorart WHERE kind = 'pet' ORDER BY world")
        self.assertEqual(art, [("Catapult (pet look, Tunaria)", 0xE64B8D78, 0, 108, "pet", 0xBFEE4CCD, 0.3, 0.35),
                               ("Catapult (pet look, Rathe)", 0xE64B8D78, 1, 13, "pet", 0xBFEE4CCD, 0.3, 0.35)])
        self.assertEqual(self.q("SELECT kind, shot, scale FROM decorart WHERE name = 'Christmas Tree'"), [("", 0, 0.0)])
        self.assertEqual(self.q("SELECT cost FROM charcms WHERE serverid = 747 AND cmid = 900919"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 4)
        self.assertEqual(len(json.load(open(self.applied))["rows"]), 4)
        r = self.run_tool()
        self.assertIn("nothing to do", r.stdout)
        r = self.run_tool("--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("applied already", r.stdout)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.state(), was)
        self.assertTrue(os.path.exists(self.applied + ".undone"))

    def test_no_grant(self):
        r = self.run_tool("--apply", "--no-grant")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.q("SELECT count(*) FROM charcms WHERE cmid = 900919")[0][0], 0)
        self.assertEqual(self.q("SELECT count(*) FROM cmcatalog WHERE id = 900919")[0][0], 1)

    def test_refused_on_another_shard(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE characters SET charName = 'Someone' WHERE serverid = 747")
        db.commit()
        db.close()
        was = self.state()
        r = self.run_tool("--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(self.state(), was)
        self.assertFalse(os.path.exists(self.applied))

    def test_undo_refused_after_a_change(self):
        self.assertEqual(self.run_tool("--apply").returncode, 0)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE decorart SET zone = 120 WHERE name = 'Catapult (pet look, Tunaria)'")
        db.commit()
        db.close()
        was = self.state()
        r = self.run_tool("--undo")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("changed since", r.stdout)
        self.assertEqual(self.state(), was)


if __name__ == "__main__":
    unittest.main()
