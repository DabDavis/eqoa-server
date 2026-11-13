#!/usr/bin/env python3
"""dungeon_casters_2004_20261113.py on a small database holding the dungeons' tiers as the 1900 tool left them: the
report writes nothing; --apply gives each tier a damage over time and a heal of its share for the shamans and druids
(the book's heal kept for paladins and clerics) and the necromancer copy the damage over time, a gm_audit line each; a
second apply does nothing; --undo puts every table back exactly. Controls: a tier not as the 1900 tool left it is
refused, writing nothing; a shard with no tiers is left alone."""
import importlib.util
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "dungeon_casters_2004_20261113.py")
spec = importlib.util.spec_from_file_location("tool", TOOL)
tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tool)

SCHEMA = """
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, abilities TEXT NOT NULL DEFAULT '[]',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""


def make(path, tiers=True, trash=None):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    if tiers:
        for name, (before, _) in tool.ROWS.items():
            a = trash if name == "dungeon trash" and trash is not None else before
            db.execute("INSERT INTO classtemplates (name, abilities) VALUES (?, ?)", (name, json.dumps(a)))
        db.execute("INSERT INTO classtemplates (name, class, abilities) VALUES ('dungeon trash shadowknight', 3, '[]')")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = sorted(db.execute("SELECT * FROM classtemplates").fetchall())
    tables = sorted(r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'"))
    db.close()
    return out, tables


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


class DungeonCasters2004(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = dump(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 4 rows", r.stdout)
        self.assertEqual(dump(self.path), before)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.path)
        got = {n: json.loads(a) for n, a in db.execute("SELECT name, abilities FROM classtemplates")}
        kinds = lambda n: [(a["kind"], a.get("classes")) for a in got[n]]  # noqa: E731
        self.assertEqual(kinds("dungeon trash")[2:], [("heal", [2, 9]), ("dot", [7, 8, 11]), ("heal", [7, 8])])
        self.assertEqual(got["dungeon trash"][4]["heal_pct"], 10)
        self.assertEqual(kinds("dungeon boss")[0][0], "aoe")
        self.assertEqual([k for k, _ in kinds("dungeon trash necromancer")], ["nuke", "lifetap", "dot"])
        self.assertEqual(got["dungeon trash shadowknight"], [])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 4)
        db.close()
        applied = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.path), applied)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before)

    def test_refuses_a_changed_tier(self):
        make(self.path, trash=[{"kind": "nuke", "every": 9}])
        before = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.path), before)

    def test_no_tiers(self):
        make(self.path, tiers=False)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("without the dungeons", r.stdout)


if __name__ == "__main__":
    unittest.main()
