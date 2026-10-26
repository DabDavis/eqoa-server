#!/usr/bin/env python3
"""tools/seed_camp_loot.py's pieces against made-up data: the table names, the
level window, the biome of a point (the quadrant order), and --apply / --undo
on a small database, a GM's change kept. No game data.

    python3 -E -s tools/seed_camp_loot_test.py
"""
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_camp_loot as scl  # noqa: E402


class SeedCampLootTest(unittest.TestCase):
    def test_table_names(self):
        self.assertEqual(scl.table_of("Desert Loot Table"), "desert")
        self.assertEqual(scl.table_of("Rathe Mountains Loot Table"), "rathe mountains")
        self.assertEqual(scl.table_of("the Khal Warlord's"), "khal warlord")
        self.assertEqual(scl.table_of("Khal Viceroy"), "khal viceroy")

    def test_level_window(self):
        self.assertEqual(scl.window(20), (15, 25))
        self.assertEqual(scl.window(3), (1, 8))
        self.assertEqual(scl.window(55), (50, 70))  # the raid creatures above 60 too
        self.assertEqual(scl.window(None), (1, 70))

    def test_biome_quadrants(self):
        # a zone's biomes are NW, NE, SW, SE; the map's y grows southward
        zones = {"zones": [{"name": "Testmoor", "x": 1000, "y": 1000, "biomes": ["Arctic", "Forest", "Desert", "Swamp"]},
                           {"name": "Odus", "x": 5000, "y": 5000}],
                 "towns": []}
        self.assertEqual(scl.biome_at(zones, 990, 990)[0], "Arctic")
        self.assertEqual(scl.biome_at(zones, 1010, 990)[0], "Forest")
        self.assertEqual(scl.biome_at(zones, 990, 1010)[0], "Desert")
        self.assertEqual(scl.biome_at(zones, 1010, 1010)[0], "Swamp")
        self.assertEqual(scl.biome_at(zones, 5010, 5010), (None, "Odus"))  # no biomes: none

    def test_apply_then_undo(self):
        d = tempfile.mkdtemp()
        db = sqlite3.connect(os.path.join(d, "t.db"))
        db.executescript(scl.SCHEMA)
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, source) VALUES ('orc pawn', 'plains', 50, 1, 'gm')")
        db.commit()
        entries = [("plains", 11, 1, 1, 8), ("plains", 12, 1, 5, 15), ("generic", 11, 1, 1, 8)]
        bindings = [("orc pawn", "plains", 25, 1, "XP camp x: plains"), ("orc pawn", "generic", 15, 1, "XP camp x: generic"),
                    ("gnoll", "generic", 15, 1, "XP camp y: generic")]
        scl.apply(db, entries, bindings)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM loottables WHERE source = 'camp-seed'").fetchone()[0], 3)
        # the GM's row for orc pawn on plains is kept as it was
        self.assertEqual(db.execute("SELECT chance, source FROM npcloot WHERE npc_name = 'orc pawn' AND tablename = 'plains'").fetchone(), (50, "gm"))
        scl.apply(db, entries, bindings)  # again: nothing new
        self.assertEqual(db.execute("SELECT COUNT(*) FROM loottables").fetchone()[0], 3)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM npcloot").fetchone()[0], 3)
        # a GM changes one seeded row: undo keeps it, deletes the rest
        db.execute("UPDATE npcloot SET chance = 40 WHERE npc_name = 'gnoll'")
        db.commit()
        scl.undo(db)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM loottables").fetchone()[0], 0)
        self.assertEqual(sorted(db.execute("SELECT npc_name, tablename, chance FROM npcloot").fetchall()),
                         [("gnoll", "generic", 40), ("orc pawn", "plains", 50)])


if __name__ == "__main__":
    unittest.main()
