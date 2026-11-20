#!/usr/bin/env python3
"""runnyeye_placements_20261120.py on a small database: the report writes nothing; --apply moves every row it names
(found by name, world and place) to its new place, facing and every other column kept, with a gm_audit line each; a
second --apply writes nothing; --undo puts them back exactly; a shard without world 14's rows skips those lines; a row
a GM moved since is refused and nothing is written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "runnyeye_placements_20261120.py")
sys.path.insert(0, HERE)
import runnyeye_placements_20261120 as rp  # noqa: E402


def make(path, worlds=(0, 14)):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL,
            z_coord REAL, facing INTEGER, world INTEGER, npc_level INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    k = 100
    for _, name, world, frm, _ in rp.MOVES:
        if world in worlds:
            db.execute("INSERT INTO npcs VALUES (?, ?, 67, ?, ?, ?, ?, ?, 20)",
                       (k, name.title() if k % 2 else name, frm[0], frm[1], frm[2], k % 256, world))
            k += 1
    # one of the same name elsewhere, never touched
    db.execute("INSERT INTO npcs VALUES (900, 'goblin sneak', 67, 16300, 50, 12200, 7, 0, 20)")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    try:
        return db.execute("SELECT * FROM npcs ORDER BY id").fetchall()
    finally:
        db.close()


def audits(path):
    db = sqlite3.connect(path)
    try:
        return db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
    finally:
        db.close()


class Moves(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn(f"applied: {len(rp.MOVES)} rows", r.stdout)
        self.assertEqual(audits(self.db), len(rp.MOVES))
        after = {row[0]: row for row in dump(self.db)}
        for i, (_, name, world, _, to) in enumerate(rp.MOVES):
            row = after[100 + i]
            self.assertEqual((row[3], row[4], row[5]), to, name)
            self.assertEqual(row[6], before[i][6])  # facing kept
            self.assertEqual(row[7], world)
        self.assertEqual(after[900], before[-1])
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0)
        self.assertIn("applied already", r.stdout)
        self.assertEqual(audits(self.db), len(rp.MOVES))
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_a_shard_without_world_14(self):
        make(self.db, worlds=(0,))
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        n0 = sum(1 for m in rp.MOVES if m[2] == 0)
        self.assertIn(f"applied: {n0} rows", r.stdout)
        self.assertIn("no world-14 rows on this shard", r.stdout)

    def test_a_row_moved_since_is_refused(self):
        make(self.db)
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = x_coord + 5 WHERE id = 101")
        db.commit()
        db.close()
        before = dump(self.db)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertIn("refused", r.stdout)
        self.assertEqual(dump(self.db), before)
        self.assertEqual(audits(self.db), 0)


if __name__ == "__main__":
    unittest.main()
