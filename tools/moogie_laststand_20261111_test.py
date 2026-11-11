#!/usr/bin/env python3
"""moogie_laststand_20261111.py on the raid gods' tool's test database, after that tool's and the Innoruuk visuals tool's
--apply: the report writes nothing; --apply gives Mr. Moogie his last stand after his eight abilities, with its gm_audit
line; a second apply is refused; --undo puts the row back exactly. The controls: before those tools (his abilities not
their eight) it refuses, writing nothing; with no Mr. Moogie template it is another shard and does nothing."""
import json
import os
import sqlite3
import subprocess
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_innoruuk_visuals_20261110_test as vis  # noqa: E402
import fix_moogie_tunare_20261110 as fm  # noqa: E402
import fix_moogie_tunare_20261110_test as base  # noqa: E402
import moogie_laststand_20261111 as ml  # noqa: E402

TOOL = os.path.join(HERE, "moogie_laststand_20261111.py")
VIS_TOOL = os.path.join(HERE, "fix_innoruuk_visuals_20261110.py")


class MoogieLastStandTest(unittest.TestCase):
    def setUp(self):
        self.v = vis.FixInnoruukVisualsTest("test_apply_and_undo")
        self.v.setUp()
        self.path = self.v.path

    def run_tool(self, tool, *args):
        p = subprocess.run([sys.executable, tool, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def moogie(self):
        db = sqlite3.connect(self.path)
        try:
            return json.loads(db.execute("SELECT abilities FROM classtemplates WHERE name = 'mr moogie'").fetchone()[0])
        finally:
            db.close()

    def test_apply_and_undo(self):
        code, out = self.run_tool(TOOL, "--apply")
        self.assertNotEqual(code, 0, "before the tools of 2026-11-10 it wrote")
        self.assertIn("refused, nothing done", out)
        self.assertEqual(self.run_tool(base.TOOL, "--apply")[0], 0)
        self.assertEqual(self.run_tool(VIS_TOOL, "--apply")[0], 0)
        before = base.dump(sqlite3.connect(self.path))
        code, out = self.run_tool(TOOL)
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 1 rows", out)
        self.assertEqual(base.dump(sqlite3.connect(self.path)), before, "the report wrote")
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(fm.norm(self.moogie()), fm.norm(ml.MOOGIE_NOW + [ml.LASTSTAND]))
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail = ?", (ml.DETAIL,)).fetchone()[0], 1)
        db.close()
        code, out = self.run_tool(TOOL, "--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("applied already", out)
        code, out = self.run_tool(TOOL, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(base.dump(sqlite3.connect(self.path)), before, "the undo is not exact")

    def test_not_his_shard(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("DELETE FROM classtemplates WHERE name = 'mr moogie'")
        db.close()
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("not his shard", out)


if __name__ == "__main__":
    unittest.main()
