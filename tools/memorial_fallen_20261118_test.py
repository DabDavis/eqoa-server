#!/usr/bin/env python3
"""tools/memorial_fallen_20261118.py against a made-up database: the schema's npcs, npcmove, npcaggro, gm_audit and
accountinfo tables, a well's neighbour 15 off. No game data.

    python3 tools/memorial_fallen_20261118_test.py
"""
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import memorial_fallen_20261118 as mf  # noqa: E402

NPC_COLS = ["id INTEGER PRIMARY KEY AUTOINCREMENT", "npc_id INTEGER"] + [
    f"{c} {'TEXT' if c in ('npc_name', 'robe_style', 'race', 'unk_i', 'dt_kill') else 'REAL'}" for c in mf.PIECES[0]]


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.execute(f"CREATE TABLE npcs ({', '.join(NPC_COLS)})")
    db.execute("CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER, radius REAL, water TEXT, note TEXT, source TEXT)")
    db.execute("CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER, radius REAL, note TEXT, source TEXT)")
    db.execute("""CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT,
        via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)""")
    db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
    db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
    db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('Aeric Sparelli', 0, 25323.5, 54.1, 15735.9)")
    return db


class MemorialTool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        make_db(self.path).close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        sys.argv = ["memorial_fallen_20261118.py", "--db", self.path, *args]
        try:
            mf.main()
        except SystemExit as e:
            return e.code
        return 0

    def db(self):
        return sqlite3.connect(self.path)

    def test_apply_idempotent_undo(self):
        self.assertEqual(self.run_tool(), 0)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcs").fetchone()[0], 1, "a report wrote")
        self.assertEqual(self.run_tool("--apply"), 0)
        rows = self.db().execute("SELECT npc_name, modelid, size, npc_type, x_coord, z_coord FROM npcs WHERE id > 1 ORDER BY id").fetchall()
        self.assertEqual(rows, [("Memorial of the Fallen", 1843770060, 1.6, 384, 25318.0, 15722.0),
                                ("Hardcore Leaderboard", 638497839, 1.0, 384, 25314.0, 15719.5)])
        self.assertEqual(self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0], 6)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcmove WHERE roam = 0").fetchone()[0], 2)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcaggro WHERE aggressive = 0").fetchone()[0], 2)
        self.assertEqual(self.run_tool("--apply"), 0)  # nothing to do
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcs").fetchone()[0], 3)
        self.assertEqual(self.run_tool("--undo"), 0)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcs").fetchone()[0], 1)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcmove").fetchone()[0], 0)
        self.assertEqual(self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0], 12)

    def test_refused_where_the_spot_is_taken(self):
        db = self.db()
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('a wanderer', 0, 25318.5, 54.1, 15722.5)")
        db.commit()
        self.assertEqual(self.run_tool("--apply"), 1)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcs").fetchone()[0], 2)


if __name__ == "__main__":
    unittest.main()
