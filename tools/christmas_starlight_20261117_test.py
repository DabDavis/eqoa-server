#!/usr/bin/env python3
"""christmas_starlight_20261117.py on christmas_sleigh's small database: the report writes nothing; --apply adds
Starlight (the winged horse at 0.8, friendly, standing, of the season) clear of the tree's presents and of Santa; a
second --apply refuses; --undo puts every table back."""
import os
import sqlite3
import subprocess
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import christmas_sleigh_20261116_test as sl  # noqa: E402

TOOL = os.path.join(HERE, "christmas_starlight_20261117.py")


class StarlightTest(sl.SleighTest):
    def run_tool(self, *args):
        return subprocess.run([sys.executable, TOOL, "--db", self.db, "--applied", self.applied, *args], capture_output=True, text=True)

    def test_apply_and_undo(self):
        before = sl.dump(self.db)
        self.assertEqual(self.run_tool().returncode, 0)
        self.assertEqual(sl.dump(self.db), before, "the report writes nothing")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.db)
        x, z, model, size, kind = db.execute("SELECT x_coord, z_coord, modelid & 4294967295, size, npc_type FROM npcs WHERE npc_name = 'Starlight'").fetchone()
        self.assertEqual((model, size, kind), (0x2F8721C0, 0.8, 128))
        self.assertGreater(((x - 25270) ** 2 + (z - 15754.7) ** 2) ** 0.5, 13, "past the presents")
        self.assertLess(((x - 25270) ** 2 + (z - 15754.7) ** 2) ** 0.5, 18, "by the tree")
        self.assertGreater(((x - 25279) ** 2 + (z - 15769) ** 2) ** 0.5, 6, "clear of Santa")
        for t, col in (("npcseason", "name"), ("npcaggro", "npc_name")):
            self.assertEqual(db.execute(f"SELECT count(*) FROM {t} WHERE {col} = 'Starlight'").fetchone()[0], 1)
        db.close()
        self.assertNotEqual(self.run_tool("--apply").returncode, 0)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(sl.dump(self.db), before, "undone")


if __name__ == "__main__":
    unittest.main(defaultTest="StarlightTest")
