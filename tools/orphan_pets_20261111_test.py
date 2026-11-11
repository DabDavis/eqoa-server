#!/usr/bin/env python3
"""orphan_pets_20261111.py on a database of its own (the server's npcs and gm_audit, read out of internal/database's Go
source), every pet and caster the tool names placed as Heirloom's rows have them: the report writes nothing; --apply
puts the five spirits 1.5 off their casters and removes nothing; a second --apply does nothing; --apply --remove removes
the 30; --undo puts every row back exactly. The controls: a pet not where it was found refuses everything, and a
caster with a pet beside it now refuses its attach."""
import glob
import json
import math
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import orphan_pets_20261111 as op  # noqa: E402

TOOL = os.path.join(HERE, "orphan_pets_20261111.py")
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


def dump(db):
    return sorted(json.dumps(r) for r in db.execute("SELECT * FROM npcs").fetchall())


class OrphanPetsTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        db.execute(schema("npcs"))
        db.execute(schema("gm_audit"))
        ins = "INSERT INTO npcs (id, npc_name, world, zone, x_coord, y_coord, z_coord, npc_level) VALUES (?, ?, ?, 7, ?, 50, ?, 30)"
        for pet, caster in op.ATTACH:
            db.execute(ins, pet[:5])
            db.execute(ins.replace("zone, ", "").replace("7, ", ""), caster[:5])
        for r in op.REMOVE:
            db.execute(ins, r[:5])
        db.execute(ins, (9, "goblin brigand", 0, 19340.0, 12750.0))  # a creature of the camp, kept
        db.close()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def db(self):
        return sqlite3.connect(self.path)

    def test_report_apply_remove_undo(self):
        before = dump(self.db())
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 5 pets attached, 0 removed", out)
        self.assertEqual(dump(self.db()), before, "the report wrote")

        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        db = self.db()
        for (pid, *_), (cid, _, _, cx, cz) in op.ATTACH:
            x, z = db.execute("SELECT x_coord, z_coord FROM npcs WHERE id = ?", (pid,)).fetchone()
            self.assertAlmostEqual(math.hypot(x - cx, z - cz), op.BESIDE, delta=0.02)
        self.assertEqual(db.execute("SELECT count(*) FROM npcs").fetchone()[0], 2 * len(op.ATTACH) + len(op.REMOVE) + 1)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE action = 'update'").fetchone()[0], 5)
        code, out = self.run_tool("--apply")
        self.assertEqual((code, out.strip().splitlines()[-1]), (0, "nothing to do"))

        code, out = self.run_tool("--apply", "--remove")
        self.assertEqual(code, 0, out)
        self.assertIn("0 pets attached, 30 removed", out)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcs").fetchone()[0], 2 * len(op.ATTACH) + 1)

        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db()), before, "the undo is not exact")
        self.assertFalse(self.db().execute("SELECT 1 FROM sqlite_master WHERE name = ?", (op.LOG,)).fetchone())

    def test_refusals(self):
        db = self.db()
        db.execute("UPDATE npcs SET x_coord = x_coord + 5 WHERE id = 112437")
        db.commit()
        before = dump(self.db())
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("#112437 not as found", out)
        self.assertEqual(dump(self.db()), before)
        db.execute("UPDATE npcs SET x_coord = x_coord - 5 WHERE id = 112437")
        db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, z_coord) VALUES (8, 'Companion Spirit', 1, 7429.0, 5544.0)")
        db.commit()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("caster #124600 hobgoblin oracle has a pet beside it now", out)


if __name__ == "__main__":
    unittest.main()
