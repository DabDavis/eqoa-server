#!/usr/bin/env python3
"""moogie_gm_20261113.py on the raid gods' tool's test database after the tools of 2026-11-10/11 (his nine abilities):
the report writes nothing; --apply gives Mr. Moogie his ward and his hold after the nine, a trigger of one broadcast to
everyone and his death row to it, each with its gm_audit line; a second apply is refused; --undo puts every row back
exactly. The controls: before the last stand (eight abilities) it refuses, writing nothing; with no Mr. Moogie template
it is another shard and does nothing."""
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
import moogie_gm_20261113 as mg  # noqa: E402

TOOL = os.path.join(HERE, "moogie_gm_20261113.py")
VIS_TOOL = os.path.join(HERE, "fix_innoruuk_visuals_20261110.py")
LS_TOOL = os.path.join(HERE, "moogie_laststand_20261111.py")


def dump(path):
    db = sqlite3.connect(path)
    try:
        out = base.dump(db)
        for t in ("gmtriggers", "npcdeath"):
            cur = db.execute(f"SELECT * FROM {t}")
            cols = [d[0] for d in cur.description]
            out[t] = sorted(json.dumps(dict(zip(cols, r)), sort_keys=True) for r in cur.fetchall())
        return out
    finally:
        db.close()


class MoogieGMTest(unittest.TestCase):
    def setUp(self):
        self.v = vis.FixInnoruukVisualsTest("test_apply_and_undo")
        self.v.setUp()
        self.path = self.v.path
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute(base.schema("gmtriggers"))
        db.execute(base.schema("npcdeath"))
        db.execute("INSERT INTO gmtriggers (slot, name, scope, steps) VALUES (12, 'Older', 'near', '[]')")
        db.close()

    def run_tool(self, tool, *args):
        p = subprocess.run([sys.executable, tool, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def test_apply_and_undo(self):
        self.assertEqual(self.run_tool(base.TOOL, "--apply")[0], 0)
        self.assertEqual(self.run_tool(VIS_TOOL, "--apply")[0], 0)
        code, out = self.run_tool(TOOL, "--apply")
        self.assertNotEqual(code, 0, "before the last stand it wrote")
        self.assertIn("refused, nothing done", out)
        self.assertEqual(self.run_tool(LS_TOOL, "--apply")[0], 0)
        before = dump(self.path)
        code, out = self.run_tool(TOOL)
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 3 rows", out)
        self.assertEqual(dump(self.path), before, "the report wrote")
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        got = json.loads(db.execute("SELECT abilities FROM classtemplates WHERE name = 'mr moogie'").fetchone()[0])
        self.assertEqual(fm.norm(got), fm.norm(mg.MOOGIE_NINE + [mg.WARD, mg.HOLD]))
        self.assertEqual(len(got), 11)
        slot, scope, steps = db.execute("SELECT slot, scope, steps FROM gmtriggers WHERE name = ?", (mg.TRIGGER,)).fetchone()
        self.assertEqual((slot, scope), (13, "all"))
        self.assertEqual(json.loads(steps), [{"do": "broadcast",
                                              "text": '"You have won for now mortals. But I do not forgive and I do not forget." - Mr. Moogie'}])
        self.assertEqual(db.execute("SELECT name, trigger, chance FROM npcdeath").fetchall(), [("mr. moogie", mg.TRIGGER, 100.0)])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail = ?", (mg.DETAIL,)).fetchone()[0], 3)
        db.close()
        code, out = self.run_tool(TOOL, "--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("applied already", out)
        code, out = self.run_tool(TOOL, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.path), before, "the undo is not exact")

    def test_not_his_shard(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("DELETE FROM classtemplates WHERE name = 'mr moogie'")
        db.close()
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("not his shard", out)


if __name__ == "__main__":
    unittest.main()
