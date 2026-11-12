#!/usr/bin/env python3
"""disperse_patrol_20261112.py on a database of its own (the server's npcs and gm_audit, read out of
internal/database's Go source) and a plan of its own: the report writes nothing; --apply --world 0 moves world 0's
rows and writes its wander rows (making table npcwander) and touches world 2 not at all; a second apply does nothing;
world 2 applies by itself; --undo --world 0 puts world 0 back and leaves world 2 as applied; undoing both leaves the
database's dump exactly as it was. The controls: a row a GM moved since the plan is skipped (never moved) and the rest
of its world still applies; a row moved after the apply is left by the undo and named."""
import glob
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import json
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "disperse_patrol_20261112.py")
GO = os.path.join(HERE, "..", "internal", "database")


def schema(name):
    for path in sorted(glob.glob(os.path.join(GO, "*.go"))):
        if path.endswith("_test.go"):
            continue
        with open(path) as f:
            m = re.search(r"CREATE TABLE IF NOT EXISTS " + name + r"\s*\(.*?\n\s*\);", f.read(), re.S)
        if m:
            return m.group(0)
    raise AssertionError(f"no schema of {name} in {GO}")


def pos(x, y, z, f):
    return {"X": x, "Y": y, "Z": z, "Facing": f}


PLAN = {
    "moves": [
        {"id": 1, "world": 0, "name": "wolfwere", "camp": 1, "why": "gm", "before": pos(100, 50, 100, 10), "after": pos(120.5, 51.25, 90.125, 77)},
        {"id": 2, "world": 0, "name": "wolfwere", "camp": 1, "why": "gm", "before": pos(100.5, 50, 100, 10), "after": pos(80, 49.5, 110, 200)},
        {"id": 3, "world": 0, "name": "treant", "camp": 2, "why": "rows", "before": pos(300, 50, 300, 0), "after": pos(310, 50, 290, 5)},
        {"id": 5, "world": 2, "name": "grizz warrior", "camp": 3, "why": "stacked", "before": pos(900, 60, 900, 0), "after": pos(910, 61, 905, 9)},
    ],
    "wander": [
        {"id": 1, "world": 0, "name": "wolfwere", "radius": 110, "note": "GM petition 1734 (Jibrish): spread out and roam", "x": 100, "z": 100},
        {"id": 4, "world": 0, "name": "orc pawn", "radius": 0, "note": "a lone creature of a standing kind wanders", "x": 500, "z": 500},
        {"id": 6, "world": 2, "name": "hill giant", "radius": 300, "note": "a far roamer roams its ground", "x": 1, "z": 1},
    ],
    "camps": [], "left": {},
}


class DisperseTest(unittest.TestCase):
    def setUp(self):
        d = tempfile.mkdtemp()
        self.path = os.path.join(d, "t.db")
        self.plan = os.path.join(d, "plan.json")
        with open(self.plan, "w") as f:
            json.dump(PLAN, f)
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        db.execute(schema("npcs"))
        db.execute(schema("gm_audit"))
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action) VALUES ('x', 1, 'dab', 'web', '', 'update')")
        rows = [(1, "wolfwere", 0, 100, 50, 100, 10), (2, "wolfwere", 0, 100.5, 50, 100, 10), (3, "treant", 0, 300, 50, 300, 0),
                (4, "orc pawn", 0, 500, 50, 500, 3), (5, "grizz warrior", 2, 900, 60, 900, 0), (6, "hill giant", 2, 1, 2, 1, 0)]
        for r in rows:
            db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing) VALUES (?, ?, ?, ?, ?, ?, ?)", r)
        db.close()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, "--plan", self.plan, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def dump(self):
        db = sqlite3.connect(self.path)
        out = "\n".join(db.iterdump())
        db.close()
        return out

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        r = db.execute(sql, a).fetchall()
        db.close()
        return r

    def test_world_by_world_apply_again_undo_exact(self):
        before = self.dump()
        rc, out = self.run_tool()
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.dump(), before, "the report wrote")
        rc, out = self.run_tool("--world", "0", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("moved 3 rows, wrote 2 wander rows", out)
        self.assertEqual(self.q("SELECT x_coord, y_coord, z_coord, facing FROM npcs WHERE id = 1"), [(120.5, 51.25, 90.125, 77)])
        self.assertEqual(self.q("SELECT x_coord, facing FROM npcs WHERE id = 5"), [(900, 0)], "world 2 touched")
        self.assertEqual(self.q("SELECT npcid, radius, source FROM npcwander ORDER BY npcid"), [(1, 110, "disperse"), (4, 0, "disperse")])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit WHERE detail LIKE '%disperse_patrol%'"), [(5,)])
        mid = self.dump()
        rc, out = self.run_tool("--world", "0", "--apply")
        self.assertIn("nothing to do", out)
        self.assertEqual(self.dump(), mid, "a second apply changed something")
        rc, out = self.run_tool("--world", "2", "--apply")
        self.assertIn("moved 1 rows, wrote 1 wander rows", out)
        rc, out = self.run_tool("--world", "0", "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT x_coord, facing FROM npcs WHERE id = 1"), [(100, 10)])
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE id = 5"), [(910,)], "world 2's undone by world 0's undo")
        self.assertEqual(self.q("SELECT npcid FROM npcwander"), [(6,)])
        rc, out = self.run_tool("--world", "2", "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.dump(), before, "the undo is not exact")

    def test_a_row_moved_since_is_skipped_and_left(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("UPDATE npcs SET x_coord = 105 WHERE id = 2")  # a GM moved it after the plan
        db.close()
        rc, out = self.run_tool("--world", "0", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("skipped: npcs 2 'wolfwere': moved since the plan", out)
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE id = 2"), [(105,)])
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE id = 3"), [(310,)], "the rest of the world not applied")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("UPDATE npcs SET x_coord = 333 WHERE id = 3")  # moved again after the apply
        db.close()
        rc, out = self.run_tool("--world", "0", "--undo")
        self.assertEqual(rc, 1, out)
        self.assertIn("left: npcs 3: moved since", out)
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE id = 3"), [(333,)])
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE id = 1"), [(100,)])


if __name__ == "__main__":
    unittest.main()
