#!/usr/bin/env python3
"""moogie_glow_20261113.py on a database with npctint as it stood before the glow: the report writes nothing; --apply
adds the glow column and Mr. Moogie's row (no tint, a soft red glow) with its gm_audit line; a second apply is refused;
--undo takes the row back exactly (the column stays). The controls: a row for him already there is left and refused;
with no Mr. Moogie NPC it is another shard and does nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_moogie_tunare_20261110_test as base  # noqa: E402

TOOL = os.path.join(HERE, "moogie_glow_20261113.py")
OLD_NPCTINT = """CREATE TABLE npctint (name TEXT PRIMARY KEY, color INTEGER NOT NULL DEFAULT 4294967295,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')"""


class MoogieGlowTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        db.execute(base.schema("npcs"))
        db.execute(base.schema("gm_audit"))
        db.execute(OLD_NPCTINT)
        db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, modelid, size, npc_level, npc_type, npc_id, race) "
                   "VALUES (500, 'Mr. Moogie', 0, 1, 2, 3, 4, 2.0, 80, 128, 500, '0')")
        db.execute("INSERT INTO npctint (name, color, source) VALUES ('Yhafiran', 1435259135, 'seed')")
        db.close()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def tints(self):
        db = sqlite3.connect(self.path)
        try:
            cols = [r[1] for r in db.execute("PRAGMA table_info(npctint)")]
            return sorted(db.execute(f"SELECT {', '.join(cols)} FROM npctint").fetchall()), cols
        finally:
            db.close()

    def test_apply_and_undo(self):
        before, _ = self.tints()
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 1 rows", out)
        self.assertEqual(self.tints(), (before, ["name", "color", "note", "source"]), "the report wrote")
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT color, glow FROM npctint WHERE name = 'Mr. Moogie'").fetchone(), (0xFFFFFFFF, 0xFF606099))
        self.assertEqual(db.execute("SELECT glow FROM npctint WHERE name = 'Yhafiran'").fetchone(), (0,))
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE tbl = 'npctint'").fetchone()[0], 1)
        db.close()
        code, out = self.run_tool("--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("applied already", out)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        rows, cols = self.tints()
        self.assertEqual([r[:2] for r in rows], [r[:2] for r in before])
        self.assertIn("glow", cols, "the column stays")

    def test_his_row_already_there(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("INSERT INTO npctint (name, color) VALUES ('mr. moogie ', 4278190335)")
        db.close()
        code, out = self.run_tool("--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("left as it is", out)

    def test_not_his_shard(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("DELETE FROM npcs")
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("not his shard", out)


if __name__ == "__main__":
    unittest.main()
