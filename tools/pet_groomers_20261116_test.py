#!/usr/bin/env python3
"""pet_groomers_20261116.py on a small database: the report writes nothing; --apply writes the rank for the one gnome
a player (PetLook and OnlyFor his serverid), the drachnid's areas and a groomer beside each dyer that has none (3 units on
from the dyer, away from its outfitter, in the other sex's body), each with a gm_audit line; a second apply finds nothing
to do; --undo takes out exactly what it made. The controls: an elf gets no rank (the rest still written); a
dyer with a groomer beside it gets no second; id 900917 taken by another row refuses the whole, writing nothing."""
import math
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "pet_groomers_20261116.py")


def make(path, race=3, taken=False, groomed=False):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY, race INTEGER, tclass INTEGER, level INTEGER);
        CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
            races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
            name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
            level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
            requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '',
            adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '',
            remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
            source TEXT NOT NULL DEFAULT 'gm', require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '');
        CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
            bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL,
            z_coord REAL, facing INTEGER, world INTEGER, modelid INTEGER, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER, torso INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO cmcatalog (id, category, races, mastery, rank, name, parent, cost, level) VALUES
            (1541, 4, 8, 'Tinker', 1, 'Tinker', 0, 200, 55), (1542, 4, 8, 'Tinker', 2, 'Construct', 1541, 75, 60);
        INSERT INTO spellpattern VALUES (901835, 'Construct'), (901836, 'Construct (Drachnid)'), (901837, 'Construct (Rat)');
        INSERT INTO npcs VALUES (100, 'Heirloom Outfitter', 62, 100, 50, 200, 4, 0, 1893243078, 50, 129, 100, 3),
            (101, 'Equipment Dyer', 62, 104, 50, 200, 4, 0, 1893243078, 50, 128, 101, 3),
            (102, 'Equipment Dyer', 1, 500, 7, 500, 0, 0, 3783858679, 50, 128, 102, 0);
    """)
    db.execute("INSERT INTO characters VALUES ('Zerkziee', 747, ?, 1, 60), ('Zerkzi', 746, 16, 4, 60)", (race,))
    db.execute("INSERT INTO charcms VALUES (747, 1541, 200, 1), (747, 1542, 75, 1)")
    if taken:
        db.execute("INSERT INTO cmcatalog (id, name, source) VALUES (900917, 'Something Else', 'gm')")
    if groomed:
        db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, world) VALUES ('Pet Groomer', 1, 502, 7, 500, 0)")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    try:
        out = {}
        for t in ("cmcatalog", "npcs", "petdyeareas"):
            if db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (t,)).fetchone():
                out[t] = db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall()
        return out
    finally:
        db.close()


class PetGroomers(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = dump(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 4 rows", r.stdout)
        self.assertEqual(dump(self.path).get("cmcatalog"), before["cmcatalog"])
        self.assertEqual(dump(self.path)["npcs"], before["npcs"])
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.path)
        rank = db.execute("SELECT name, parent, cost, level, races, requires, require_spells, effects FROM cmcatalog WHERE id = 900917").fetchone()
        self.assertEqual(rank, ("Construct (Jarek's Creation)", 1542, 0, 60, 8, "Construct@60", "901835|901836|901837",
                                "PetLook:360074664,OnlyFor:747"))
        area = db.execute("SELECT area1, area2, materials FROM petdyeareas WHERE look = ?", (0x15764DA8,)).fetchone()
        self.assertEqual(area[:2], ("Torso", "Spider body"))
        self.assertTrue(area[2])
        g = db.execute("SELECT x_coord, z_coord, y_coord, facing, modelid, npc_type, npc_id = id, zone, torso FROM npcs "
                       "WHERE npc_name = 'Pet Groomer' ORDER BY id").fetchall()
        self.assertEqual(len(g), 2)
        self.assertEqual(g[0][:4], (107.0, 200.0, 50, 4), "3 on from the dyer, away from the outfitter")
        self.assertEqual(g[0][4:], (0x0D537335, 128, 1, 62, 3), "the human woman beside the human man dyer, its zone and gear")
        self.assertAlmostEqual(math.hypot(g[1][0] - 500, g[1][1] - 500), 3.0, places=1)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 4)
        db.close()
        r = run(self.path, "--apply")
        self.assertIn("nothing to do", r.stdout)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        after = dump(self.path)
        self.assertEqual(after["cmcatalog"], before["cmcatalog"])
        self.assertEqual(after["npcs"], before["npcs"])
        self.assertEqual(after["petdyeareas"], [])

    def test_no_gnome_no_rank(self):
        make(self.path, race=1, groomed=True)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("no gnome", r.stdout)
        db = sqlite3.connect(self.path)
        self.assertIsNone(db.execute("SELECT 1 FROM cmcatalog WHERE id = 900917").fetchone())
        self.assertEqual(db.execute("SELECT count(*) FROM npcs WHERE npc_name = 'Pet Groomer'").fetchone()[0], 2,
                         "the one that stood and one more: the groomed dyer gets no second")
        db.close()

    def test_taken_id_refuses(self):
        make(self.path, taken=True)
        before = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.path), before)


if __name__ == "__main__":
    unittest.main()
