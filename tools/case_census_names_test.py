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


if __name__ == "__main__":
    unittest.main()
