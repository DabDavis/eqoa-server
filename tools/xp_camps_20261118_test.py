#!/usr/bin/env python3
"""tools/xp_camps_20261118.py: a plan's units apply once, a second apply writes nothing, a GM's edit since leaves its
unit, and --undo puts npcs, npcrespawn, npcwander and the npcs sequence back exactly.

    python3 -E -s tools/xp_camps_20261118_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import xp_camps_20261118 as x  # noqa: E402


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL,
            y_coord REAL, z_coord REAL, facing INTEGER, npc_level INTEGER, npc_type INTEGER, modelid INTEGER,
            dt_kill TEXT);
        CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    for i in range(1, 5):
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, npc_type, modelid) "
                   "VALUES ('orc', 0, ?, 50, 100, 0, 10, 0, 7)", (100.0 + i,))
    db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, npc_type, modelid) "
               "VALUES ('orc', 0, 101, 50, 100, 0, 10, 0, 7)")  # 5: a double of 1
    db.execute("INSERT INTO npcrespawn VALUES (5, 90, '', 'gm')")
    db.execute("INSERT INTO npcwander VALUES (2, 30, '', 'gm')")
    db.commit()
    return db


def row(db, i):
    return x.tr.get(db, "npcs", {"id": i})


def units(db):
    r2, r3, r5 = row(db, 2), row(db, 3), row(db, 5)
    new = {k: v for k, v in r2.items() if k != "id"}
    new.update(x_coord=120.0)
    return [
        x.unit("c", "remove", "5 stacked on 1", [["npcs", {"id": 5}, r5, None],
                                                  ["npcrespawn", {"npc_row": 5}, x.tr.get(db, "npcrespawn", {"npc_row": 5}), None]]),
        x.unit("c", "move", "3 in the ground", [["npcs", {"id": 3}, r3, dict(r3, y_coord=55.0)]]),
        x.unit("c", "level", "2 stray", [["npcs", {"id": 2}, r2, dict(r2, npc_level=12)]]),
        x.unit("c", "add", "one more", [["npcs", {"id": "NEW"}, None, new],
                                        ["npcwander", {"npcid": "NEW"}, None, dict(npcid="NEW", radius=30, note="t", source="t")]]),
    ]


def hashes(db):
    return [x.digest(db, t) for t in ("npcs", "npcrespawn", "npcwander")] + \
        [db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()]


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = make(os.path.join(self.dir.name, "t.db"))
        self.db.isolation_level = None

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def test_apply_again_undo(self):
        before = hashes(self.db)
        us = units(self.db)
        count, left = x.apply_units(self.db, us)
        self.assertEqual(count["todo"], 4)
        self.assertEqual(left, [])
        self.assertIsNone(row(self.db, 5))
        self.assertEqual(row(self.db, 3)["y_coord"], 55.0)
        self.assertEqual(row(self.db, 6)["x_coord"], 120.0)
        self.assertEqual(x.tr.get(self.db, "npcwander", {"npcid": 6})["radius"], 30)
        count, _ = x.apply_units(self.db, us)
        self.assertEqual(count["todo"], 0)
        self.assertEqual(count["done"], 4)
        back, left = x.undo(self.db)
        self.assertEqual(left, 0)
        self.assertEqual(hashes(self.db), before)

    def test_changed_since_is_left(self):
        us = units(self.db)
        self.db.execute("UPDATE npcs SET x_coord = 999 WHERE id = 3")
        count, left = x.apply_units(self.db, us)
        self.assertEqual(count["conflict"], 1)
        self.assertEqual(len(left), 1)
        self.assertEqual(row(self.db, 3)["x_coord"], 999)

    def test_verdicts(self):
        r = dict(x=0.0, y=60.0, z=0.0, key="orc")
        self.assertIn("on air", x.ground_verdict(r, dict(knows=True, stand=55.0, env=0))[0])
        self.assertIsNone(x.ground_verdict(dict(r, key="giant bat"), dict(knows=True, stand=55.0, env=0))[0])
        self.assertIsNone(x.ground_verdict(r, dict(knows=True, stand=55.0, env=1))[0])
        what, to = x.ground_verdict(r, dict(knows=True, inground=True, lift=70.0, env=0))
        self.assertIn("in the ground", what)
        self.assertEqual(to, (0.0, 70.0, 0.0))


if __name__ == "__main__":
    unittest.main()
