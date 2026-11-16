#!/usr/bin/env python3
"""place_weapon_enchanters_20261116.py on a small database: refused without the template's body; the report writes
nothing; --apply stands one enchanter in the template's look beside each dyer (by its side, at its height and facing),
one for two dyers close together, none where every spot is taken (listed); a second run adds nothing; --undo takes out
exactly those; a row changed since is left and listed."""
import math
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "place_weapon_enchanters_20261116.py")


def make(path, body=839650119):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL,
            z_coord REAL, facing INTEGER, world INTEGER, modelid INTEGER, size REAL, race INTEGER, robe_style INTEGER,
            primary_weapon INTEGER, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    db.execute("INSERT INTO npcs VALUES (1, 'Waysender Ilganis', 5, 0, 0, 0, 0, 0, ?, 1.0, 8, 0, -1379336229, 35, 384, 1)", (body,))
    db.execute("INSERT INTO npcs VALUES (2, 'Equipment Dyer', 62, 100, 50, 100, 0, 0, 1, 1.0, 0, 255, 0, 50, 128, 2)")
    db.execute("INSERT INTO npcs VALUES (3, 'Armor Dyer', 62, 101, 50, 101, 64, 0, 1, 1.0, 0, 255, 0, 50, 128, 3)")  # by the first
    db.execute("INSERT INTO npcs VALUES (4, 'Equipment Dyer', 1, 500, 60, 500, 0, 1, 1, 1.0, 0, 255, 0, 50, 128, 4)")
    # a crowd round the third: every spot taken
    k = 10
    for s in (2.5, 4.0):
        for dx, dz in ((-s, 0), (s, 0), (0, -s)):
            db.execute("INSERT INTO npcs (id, npc_name, x_coord, y_coord, z_coord, facing, world) VALUES (?, 'Guard', ?, 60, ?, 0, 1)",
                       (k, 500 + dx, 500 + dz))
            k += 1
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def ench(path):
    db = sqlite3.connect(path)
    try:
        return db.execute("SELECT id, x_coord, y_coord, z_coord, facing, modelid, robe_style, npc_type, npc_level, npc_id, zone "
                          "FROM npcs WHERE npc_name = 'Weapon Enchanter'").fetchall()
    finally:
        db.close()


class Place(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_refused_without_the_body(self):
        make(self.db, body=1)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertEqual(ench(self.db), [])

    def test_apply_and_undo(self):
        make(self.db)
        self.assertEqual(run(self.db).returncode, 0)
        self.assertEqual(ench(self.db), [])
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("NONE  dyer 4", r.stdout)
        e = ench(self.db)
        self.assertEqual(len(e), 1, e)
        nid, x, y, z, facing, model, robe, typ, lvl, npcid, zone = e[0]
        self.assertAlmostEqual(math.hypot(x - 100, z - 100), 2.5, places=2)
        self.assertEqual((y, facing, model, robe, typ, lvl, npcid, zone), (50, 0, 839650119, 0, 128, 50, nid, 62))
        self.assertIn("nothing to do", run(self.db).stdout)
        self.assertEqual(run(self.db, "--undo").returncode, 0)
        self.assertEqual(ench(self.db), [])

    def test_undo_leaves_a_changed_row(self):
        make(self.db)
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = x_coord + 1 WHERE npc_name = 'Weapon Enchanter'")
        db.commit()
        db.close()
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn("changed or gone since", r.stdout)


if __name__ == "__main__":
    unittest.main()
