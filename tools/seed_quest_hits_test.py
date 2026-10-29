#!/usr/bin/env python3
"""tools/seed_quest_hits.py against a made-up database: each gate's NPCs by
its place and words (a Cracktooth by Bobble-by-Water, the beasts on Lady
Vox's floors, every mob of the isle but those left out, its siren on a
human's body among them), what is left out (a person in the castle, a pet,
an NPC-bit trader, a fish under the floor, a namesake elsewhere, a row
already there), the rows written as quest-seed, a second --apply refused and
--undo exact but for a row changed since. No game data: the NPCs are made up.

    python3 tools/seed_quest_hits_test.py
"""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_quest_hits as sqh  # noqa: E402

SCHEMA = """
CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL);
CREATE TABLE charactermodel (race INTEGER, sex INTEGER, modelid INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL, modelid INTEGER,
    npc_type INTEGER, world INTEGER);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER);
CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL, faction TEXT NOT NULL,
    delta INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm', UNIQUE (npc_name, faction));
"""
HUMAN, BEAST = 0x22222222, 0x33333333


class SeedQuestHitsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO factions VALUES (?, 0)", [("Feria Hurkert",), ("Citizens of Castle Lightwolf",),
                                                               ("Citizens of Graumord",)])
        db.execute("INSERT INTO charactermodel VALUES (0, 1, ?)", (HUMAN,))
        rows = [
            # by Bobble-by-Water (24641, 11859)
            ("cracktooth orc", 25094, 54, 12429, BEAST, 0, 0),
            ("Biter Cracktooth", 25102, 54, 12433, BEAST, 0, 0),
            ("gnoll pup", 25000, 54, 12400, BEAST, 0, 0),                 # no Cracktooth word
            # Lady Vox's castle (4861, 4397)
            ("Ice goblin warrior", 4870, 70, 4400, BEAST, 0, 0),
            ("Lady Vox", 4861, 114, 4397, BEAST, 0, 0),
            ("Karis McDonogh", 4900, 76, 4420, HUMAN, 0, 0),              # a person
            ("Vigilant Spirit", 4880, 70, 4410, BEAST, 0, 0),             # a pet
            ("Keeper Olga", 4890, 76, 4410, BEAST, 136, 0),               # a trader (the NPC bit)
            ("pike", 4900, -60, 4500, BEAST, 0, 0),                       # under the floor
            ("Efreet", 4850, 91, 4500, BEAST, 64, 0),
            ("Efreet", 9000, 50, 9000, BEAST, 64, 0),                     # a namesake elsewhere
            # the Isle of Dread
            ("dread rot", 24850, 54, 3120, BEAST, 0, 0),
            ("siren", 24565, 54, 3098, HUMAN, 0, 0),                      # a human's body, a mob
            ("Graumord citizen", 24512, 56, 3382, BEAST, 0, 0),
            ("imprisoned soul", 24900, 54, 2600, BEAST, 0, 0),
            ("Kithgrin", 25714, 49, 3229, BEAST, 0, 0),
            ("wicker man", 24835, 54, 3110, BEAST, 0, 0),
        ]
        db.executemany("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, modelid, npc_type, world) "
                       "VALUES (?, ?, ?, ?, ?, ?, ?)", rows)
        db.execute("INSERT INTO npcaggro VALUES ('Vigilant Spirit', 0)")
        db.execute("INSERT INTO factionhits (npc_name, faction, delta) VALUES ('wicker man', 'Citizens of Graumord', 3)")
        db.close()

    def db(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        self.addCleanup(db.close)
        return db

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = sqh.main(["--db", self.path, *args])
        return code, out.getvalue()

    def test_the_plan(self):
        rows, left = sqh.plan(self.db(), 5)
        self.assertEqual(sorted((r[0], r[1], r[2]) for r in rows), [
            ("Biter Cracktooth", "Feria Hurkert", 5), ("Ice goblin warrior", "Citizens of Castle Lightwolf", 5),
            ("Lady Vox", "Citizens of Castle Lightwolf", 5), ("cracktooth orc", "Feria Hurkert", 5),
            ("dread rot", "Citizens of Graumord", 5), ("siren", "Citizens of Graumord", 5)])
        self.assertEqual(left["Citizens of Castle Lightwolf"], ["'Efreet': 1 more elsewhere"])
        self.assertEqual(left["Citizens of Graumord"], ["'wicker man': a row already"])

    def test_apply_then_undo(self):
        db = self.db()
        before = sorted(db.execute("SELECT * FROM factionhits").fetchall())
        code, out = self.run_tool()
        self.assertEqual(code, 0)
        self.assertIn("report only", out)
        self.assertEqual(sorted(db.execute("SELECT * FROM factionhits").fetchall()), before)
        code, out = self.run_tool("--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM factionhits WHERE source = 'quest-seed'").fetchone(), (6,))
        with self.assertRaises(SystemExit):
            with contextlib.redirect_stderr(io.StringIO()):
                self.run_tool("--apply", "--no-backup")
        db.execute("UPDATE factionhits SET delta = 9 WHERE npc_name = 'siren'")
        code, out = self.run_tool("--undo", "--no-backup")
        self.assertEqual(code, 0)
        self.assertIn("changed since", out)
        self.assertEqual(db.execute("SELECT npc_name, delta FROM factionhits WHERE source = 'quest-seed'").fetchall(),
                         [("siren", 9)])
        db.execute("DELETE FROM factionhits WHERE source = 'quest-seed'")
        self.assertEqual(sorted(db.execute("SELECT * FROM factionhits").fetchall()), before)
        self.assertIsNone(db.execute("SELECT name FROM sqlite_master WHERE name = 'quest_hit_seed'").fetchone())


if __name__ == "__main__":
    unittest.main()
