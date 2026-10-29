#!/usr/bin/env python3
"""tools/seed_boss_loot.py against a made-up database: a boss's table and
binding written once (a second apply writes nothing), a row of the same table
a GM wrote left alone, a drop no itempattern row names left out, a spell
scroll of a drop's name never taken for it, and --undo giving back the tables
as they were, but for a row a GM changed since, which stays. No game data.

    python3 tools/seed_boss_loot_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_boss_loot.py")
SCROLL_ART = -607649328

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT NOT NULL, itemicon INTEGER NOT NULL, equipslot INTEGER NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_type INTEGER);
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?)", [
        (10, "Shawl of the Winter Queen", 5, 3),     # Lady Vox's, any case
        (11, "shawl of the winter queen", 5, 3),     # a later namesake: the lowest id wins
        (20, "Bonecleaver", SCROLL_ART, -1),         # a scroll of the name: never the drop
        (21, "Bonecleaver", 7, 12),                  # Overlord Xrag's
        (30, "Nullfield Earring", 9, 4),             # Radavex's, whom no one can attack
    ])
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?)", [
        (1, "Lady Vox", 0), (2, "Overlord Xrag", 0), (3, "Radavex Planeswalker", 0x180),
    ])
    db.commit()
    db.close()


def run(db, *args):
    r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", db, "--no-backup", *args],
                       capture_output=True, text=True)
    if r.returncode != 0:
        raise AssertionError(r.stdout + r.stderr)
    return r.stdout


def rows(db, sql):
    c = sqlite3.connect(db)
    try:
        return c.execute(sql).fetchall()
    finally:
        c.close()


class SeedBossLoot(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.tmp.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.tmp.cleanup()

    def test_report_writes_nothing(self):
        out = run(self.db)
        self.assertIn("report only", out)
        self.assertEqual(rows(self.db, "SELECT name FROM sqlite_master WHERE name IN ('loottables', 'npcloot')"), [])

    def test_apply_once(self):
        out = run(self.db, "--apply")
        self.assertIn("not in itempattern", out)  # every other boss's drops
        self.assertIn("no player can attack it (npc_type 0x80): Radavex Planeswalker", out)
        self.assertEqual(rows(self.db, "SELECT tablename, patternid, minlevel, maxlevel, source FROM loottables WHERE tablename IN ('boss lady vox', 'boss overlord xrag') ORDER BY tablename"),
                         [("boss lady vox", 10, 1, 70, "boss-seed"), ("boss overlord xrag", 21, 1, 70, "boss-seed")])
        self.assertEqual(rows(self.db, "SELECT npc_name, chance, rolls FROM npcloot WHERE npc_name = 'lady vox'"), [("lady vox", 100, 1)])
        n = rows(self.db, "SELECT COUNT(*) FROM loottables")[0][0]
        self.assertIn("wrote 0 loottables rows and 0 npcloot rows", run(self.db, "--apply"))
        self.assertEqual(rows(self.db, "SELECT COUNT(*) FROM loottables")[0][0], n)

    def test_undo_keeps_a_gm_change(self):
        run(self.db, "--apply")
        c = sqlite3.connect(self.db)
        c.execute("INSERT INTO loottables (tablename, patternid, source) VALUES ('boss lady vox', 11, 'gm')")
        c.execute("UPDATE npcloot SET chance = 40 WHERE npc_name = 'overlord xrag'")
        c.commit()
        c.close()
        out = run(self.db, "--undo")
        self.assertIn("kept 1 a GM changed", out)
        self.assertEqual(rows(self.db, "SELECT tablename, patternid, source FROM loottables"), [("boss lady vox", 11, "gm")])
        self.assertEqual(rows(self.db, "SELECT npc_name, chance FROM npcloot"), [("overlord xrag", 40)])

    def test_undo_whole(self):
        run(self.db, "--apply")
        run(self.db, "--undo")
        self.assertEqual(rows(self.db, "SELECT COUNT(*) FROM loottables")[0][0], 0)
        self.assertEqual(rows(self.db, "SELECT COUNT(*) FROM npcloot")[0][0], 0)
        self.assertEqual(rows(self.db, "SELECT name FROM sqlite_master WHERE name = 'boss_loot_seed'"), [])
        self.assertIn("nothing to undo", run(self.db, "--undo"))


if __name__ == "__main__":
    unittest.main()
