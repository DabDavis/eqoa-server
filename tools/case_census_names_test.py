#!/usr/bin/env python3
"""tools/case_census_names.py over a few made-up rows: the census's kinds
cased as ours are, its named ones, pets and towns' people left, a quest's
target following, and --undo putting back this pass alone. No game data.

    python3 tools/case_census_names_test.py
"""
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import case_census_names as cc  # noqa: E402
import fix_npc_name_case as nc  # noqa: E402

nc.LOG = cc.LOG
nc.LOG_DDL = nc.LOG_DDL.replace("npc_name_case_log", cc.LOG)


def build(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_type INTEGER, modelid INTEGER, world INTEGER)")
    db.execute("CREATE TABLE sandstorm_census_seed_w1 (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, rid INTEGER, census_id INTEGER, "
               "name TEXT, old TEXT)")
    db.execute("CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, spelldesc TEXT)")
    db.execute("INSERT INTO spellpattern VALUES (1, 'Feral Spirit', 'Summons a spirit')")
    db.execute("CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, npcid INTEGER, "
               "events TEXT NOT NULL DEFAULT '')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, npcid) VALUES (7, 1, 'kill', 'Rabid Timberwolf', 0)")
    # ours (not the census's): a mixed name that makes "Tae" and "Ew" proper
    for _ in range(3):
        db.execute("INSERT INTO npcs (npc_name, npc_type, modelid, world) VALUES ('Tae Ew crusader', 0, 5, 1)")
    census = {"Rabid Timberwolf": 3, "Tendora-Buki Spahi": 3, "Gorvak One-Eye": 3, "Feral Spirit": 3, "Tae Ew Weaponmaker": 3,
              "Blackhide Rhinoceros": 4, "Fetid Hydra": 2, "Neskar the Deep": 2, "Lord Zibbit": 1, "Acolyte Satyr": 5}
    for name, n in census.items():
        for _ in range(n):
            cur = db.execute("INSERT INTO npcs (npc_name, npc_type, modelid, world) VALUES (?, 0, 6, 1)", (name,))
            db.execute("INSERT INTO sandstorm_census_seed_w1 (tbl, rid, census_id, name) VALUES ('npcs', ?, 1, ?)", (cur.lastrowid, name))
    return db


class CaseTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = build(os.path.join(self.dir.name, "t.db"))

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def names(self):
        return sorted({n for (n,) in self.db.execute("SELECT npc_name FROM npcs")})

    def test_kinds_cased_ours_and_the_rest_left(self):
        before = self.names()
        p = cc.plan(self.db)
        self.assertEqual(p.renames, {
            "Rabid Timberwolf": "rabid timberwolf",           # its last word no dictionary word: a kind's noun all the same
            "Tendora-Buki Spahi": "Tendora-Buki spahi",       # a tribe's name kept, the role lowercase
            "Tae Ew Weaponmaker": "Tae Ew weaponmaker",       # proper by our own "Tae Ew crusader"
            "Blackhide Rhinoceros": "blackhide rhinoceros",   # two words run together: no name
            "Fetid Hydra": "fetid hydra",                     # twice, its noun known
            "Acolyte Satyr": "acolyte satyr",                 # a title before a kind seen five times is none
        })
        done, problems = nc.apply(self.db, p)
        self.assertEqual(problems, [])
        self.assertEqual(self.db.execute("SELECT target FROM queststeps").fetchone()[0], "rabid timberwolf", "the quest follows")
        for kept in ("Gorvak One-Eye", "Feral Spirit", "Neskar the Deep", "Lord Zibbit", "Tae Ew crusader"):
            self.assertIn(kept, self.names())
        self.assertEqual(nc.undo(self.db)[1], [])
        self.assertEqual(self.names(), before, "--undo puts back this pass alone")
        self.assertEqual(self.db.execute("SELECT target FROM queststeps").fetchone()[0], "Rabid Timberwolf")

    def test_a_tagged_census_is_cased_and_undone_beside_another(self):
        before = self.names()
        done, problems = nc.apply(self.db, cc.plan(self.db))
        self.assertEqual(problems, [])
        mid = self.names()
        # a city's census later, tagged: its kinds, beside a kind of the first census's left as written
        self.db.execute("CREATE TABLE sandstorm_census_seed_w1_city (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, rid INTEGER, "
                        "census_id INTEGER, name TEXT, old TEXT)")
        for name in ["Vorrel Sunbug"] * 3 + ["Quillek Hound"]:
            cur = self.db.execute("INSERT INTO npcs (npc_name, npc_type, modelid, world) VALUES (?, 0, 7, 1)", (name,))
            self.db.execute("INSERT INTO sandstorm_census_seed_w1_city (tbl, rid, census_id, name) VALUES ('npcs', ?, 2, ?)",
                            (cur.lastrowid, name))
        for _ in range(3):  # the first census's: renamed by an untagged pass, never by the city's
            cur = self.db.execute("INSERT INTO npcs (npc_name, npc_type, modelid, world) VALUES ('Murk Gnasher', 0, 6, 1)")
            self.db.execute("INSERT INTO sandstorm_census_seed_w1 (tbl, rid, census_id, name) VALUES ('npcs', ?, 1, 'Murk Gnasher')",
                            (cur.lastrowid,))
        added = self.names()
        self.assertIn("Murk Gnasher", cc.plan(self.db).renames, "the control: an untagged pass takes every census's")
        self.assertTrue(nc.logged(self.db))
        try:
            cc.use_log("city")
            self.assertFalse(nc.logged(self.db), "the city's pass has a log of its own")
            p = cc.plan(self.db, tag="city")
            self.assertEqual(p.renames, {"Vorrel Sunbug": "Vorrel sunbug"},
                             "its names alone: a kind's noun lowercase, a word no dictionary has kept, one made once left")
            done, problems = nc.apply(self.db, p)
            self.assertEqual(problems, [])
            self.assertTrue(nc.has_table(self.db, "npc_name_case_census_log_city"))
            self.assertIn("Murk Gnasher", self.names())
            self.assertEqual(nc.undo(self.db)[1], [])
            self.assertEqual(self.names(), added, "the city's pass undone alone")
        finally:
            cc.use_log()
        self.assertEqual(nc.undo(self.db)[1], [])
        for n in ("Vorrel Sunbug", "Quillek Hound", "Murk Gnasher"):
            self.db.execute("DELETE FROM npcs WHERE npc_name = ?", (n,))
        self.assertEqual(self.names(), before)
        self.assertNotEqual(mid, before)


if __name__ == "__main__":
    unittest.main()
