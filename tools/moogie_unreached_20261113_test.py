#!/usr/bin/env python3
"""moogie_unreached_20261113.py after moogie_gm_20261113.py (his eleven abilities): the report writes nothing; --apply
gives Mr. Moogie his unreached moment after the eleven, with its gm_audit line; a second apply is refused; --undo puts
the row back exactly. The control: before moogie_gm (nine abilities) it refuses, writing nothing."""
import json
import os
import sqlite3
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_moogie_tunare_20261110 as fm  # noqa: E402
import fix_moogie_tunare_20261110_test as base  # noqa: E402
import moogie_gm_20261113_test as gmt  # noqa: E402
import moogie_unreached_20261113 as mu  # noqa: E402

TOOL = os.path.join(HERE, "moogie_unreached_20261113.py")


class MoogieUnreachedTest(gmt.MoogieGMTest):
    def test_apply_and_undo(self):
        for t in (base.TOOL, gmt.VIS_TOOL, gmt.LS_TOOL):
            self.assertEqual(self.run_tool(t, "--apply")[0], 0)
        code, out = self.run_tool(TOOL, "--apply")
        self.assertNotEqual(code, 0, "before moogie_gm it wrote")
        self.assertIn("refused, nothing done", out)
        self.assertEqual(self.run_tool(gmt.TOOL, "--apply")[0], 0)
        before = gmt.dump(self.path)
        code, out = self.run_tool(TOOL)
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 1 rows", out)
        self.assertEqual(gmt.dump(self.path), before, "the report wrote")
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        got = json.loads(db.execute("SELECT abilities FROM classtemplates WHERE name = 'mr moogie'").fetchone()[0])
        self.assertEqual(len(got), 12)
        self.assertEqual(fm.norm(got[-1:]), fm.norm([mu.UNREACHED]))
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail = ?", (mu.DETAIL,)).fetchone()[0], 1)
        db.close()
        self.assertNotEqual(self.run_tool(TOOL, "--apply")[0], 0)
        code, out = self.run_tool(TOOL, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(gmt.dump(self.path), before, "the undo is not exact")

    def test_not_his_shard(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("DELETE FROM classtemplates WHERE name = 'mr moogie'")
        db.close()
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("not his shard", out)


if __name__ == "__main__":
    unittest.main()
