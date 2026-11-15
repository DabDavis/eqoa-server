#!/usr/bin/env python3
"""runnyeye_20261115.py on a small database: the report writes nothing; --apply copies each planned creature from its
template row (body, gear and every other column kept; place, facing, level, npc_id, dt_kill its own), audited; a second
apply does nothing; --dungeon also writes world 14's instance copies at the levels the dungeons tool's notes map, each
with its tier and loot rows; --undo puts every table back exactly. Refused: a template row that is not the plan's, a
spot already taken, --dungeon on a shard with no instanced dungeons (the Live Server)."""
import hashlib
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "runnyeye_20261115.py")

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, torso INTEGER, npc_level INTEGER,
    npc_type INTEGER, npc_id INTEGER);
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1);
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""
# world 0's citadel rows (the templates) and world 14's instance trash with the dungeons tool's notes
ROWS = [
    (1, "goblin miner", 7, 16334.0, 23.0, 12308.0, 10, 0, 0x96371804, 5, 20, 0, 1467),
    (2, "goblin veteran", 7, 16326.0, 36.0, 12226.0, 20, 0, 0x96371804, 6, 24, 0, 1485),
    (3, "goblin miner", 0, 16334.0, 23.0, 12308.0, 10, 14, 0x96371804, 5, 60, 0, 0),
    (4, "goblin veteran", 0, 16326.0, 36.0, 12226.0, 20, 14, 0x96371804, 6, 61, 0, 0),
    (5, "goblin champion", 0, 16345.0, 38.0, 12159.0, 20, 14, 0x96371804, 6, 62, 0, 0),
]
NOTES = [("#3", "Runnyeye's trash: goblin miner (world 0 row 1467, level 20 there)"),
         ("#4", "Runnyeye's trash: goblin veteran (world 0 row 1485, level 24 there)"),
         ("#5", "Runnyeye's trash: goblin champion (world 0 row 1442, level 27 there)")]
PLAN = {"params": {"world": 0},
        "templates": {"1": {"name": "goblin miner", "x": 16334.0, "y": 23.0, "z": 12308.0},
                      "2": {"name": "goblin veteran", "x": 16326.0, "y": 36.0, "z": 12226.0}},
        "adds": [{"name": "goblin miner", "template_row": 1, "level": 21, "x": 16350.5, "y": 23.03, "z": 12300.0, "facing": 77},
                 {"name": "goblin veteran", "template_row": 2, "level": 26, "x": 16310.0, "y": 36.03, "z": 12230.0, "facing": 5},
                 {"name": "goblin veteran", "template_row": 2, "level": 24, "x": 16314.0, "y": 36.03, "z": 12232.0, "facing": 200}]}


def digest(path):
    db = sqlite3.connect(path)
    h = hashlib.sha256()
    for t, o in (("npcs", "id"), ("npctemplate", "name"), ("npcloot", "npc_name, tablename"),
                 ("(SELECT * FROM sqlite_sequence WHERE name = 'npcs')", "name")):
        for r in db.execute(f"SELECT * FROM {t} ORDER BY {o}"):
            h.update(repr(r).encode())
    db.close()
    return h.hexdigest()


class RunnyeyeTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.db = os.path.join(self.dir, "eqoa.db")
        self.plan = os.path.join(self.dir, "plan.json")
        with open(self.plan, "w") as f:
            json.dump(PLAN, f)
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?)", ROWS)
        db.execute("INSERT INTO classtemplates (name) VALUES ('dungeon trash')")
        db.executemany("INSERT INTO npctemplate (name, template, note) VALUES (?, 'dungeon trash', ?)", NOTES)
        db.commit()
        db.close()

    def run_tool(self, *args, ok=True):
        p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--plan", self.plan, *args],
                           capture_output=True, text=True)
        if ok:
            self.assertEqual(p.returncode, 0, p.stderr + p.stdout)
        else:
            self.assertNotEqual(p.returncode, 0, p.stdout)
        return p.stdout + p.stderr

    def q(self, sql, *args):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_report_apply_undo(self):
        before = digest(self.db)
        self.assertIn("report only", self.run_tool("--dungeon"))
        self.assertEqual(digest(self.db), before)
        out = self.run_tool("--dungeon", "--apply")
        self.assertIn("world 0: 3 creatures added", out)
        w0 = self.q("SELECT npc_name, zone, x_coord, y_coord, z_coord, facing, modelid, torso, npc_level, npc_id FROM npcs "
                    "WHERE world = 0 AND id > 5 ORDER BY id")
        self.assertEqual(w0, [("goblin miner", 7, 16350.5, 23.03, 12300.0, 77, 0x96371804, 5, 21, 0),
                              ("goblin veteran", 7, 16310.0, 36.03, 12230.0, 5, 0x96371804, 6, 26, 0),
                              ("goblin veteran", 7, 16314.0, 36.03, 12232.0, 200, 0x96371804, 6, 24, 0)])
        w14 = self.q("SELECT id, npc_name, npc_level FROM npcs WHERE world = 14 AND id > 5 ORDER BY id")
        # level 21 -> nearest noted 20 -> 60; 26 -> nearest 27 -> 62; 24 -> 61
        self.assertEqual([(n, l) for _, n, l in w14], [("goblin miner", 60), ("goblin veteran", 62), ("goblin veteran", 61)])
        for nid, _, _ in w14:
            self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = ?", f"#{nid}"), [("dungeon trash",)])
            self.assertEqual(self.q("SELECT tablename, chance, rolls FROM npcloot WHERE npc_name = ?", f"#{nid}"),
                             [("dungeon runnyeye named", 2, 1)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit WHERE action = 'insert'"), [(12,)])
        self.assertIn("applied already", self.run_tool("--dungeon", "--apply"))
        self.run_tool("--undo")
        self.assertEqual(digest(self.db), before)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit WHERE action = 'delete'"), [(12,)])
        # and again after an undo
        self.run_tool("--apply")
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE world = 14"), [(3,)])
        self.run_tool("--undo")
        self.assertEqual(digest(self.db), before)

    def test_undo_refused_after_a_gm_edit(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET npc_level = 30 WHERE id = 6")
        db.commit()
        db.close()
        self.assertIn("changed since", self.run_tool("--undo", ok=False))
        self.assertEqual(self.q("SELECT count(*) FROM npcs"), [(8,)])

    def test_refusals(self):
        before = digest(self.db)
        db = sqlite3.connect(self.db)
        db.execute("DELETE FROM classtemplates")
        db.commit()
        db.close()
        self.assertIn("no 'dungeon trash'", self.run_tool("--dungeon", "--apply", ok=False))
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = 16000 WHERE id = 2")
        db.commit()
        db.close()
        self.assertIn("is not the plan's", self.run_tool("--apply", ok=False))
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = 16326 WHERE id = 2")
        db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world) VALUES ('goblin sneak', 16350.2, 23, 12300.3, 0)")
        db.commit()
        db.close()
        self.assertIn("stands at the plan's", self.run_tool("--apply", ok=False))
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE id > 5"), [(1,)])

    def test_plan_in_repository(self):
        """The repository's plan: every add a template it names, in the citadel, at its kinds' levels."""
        with open(os.path.join(HERE, "worlds", "runnyeye_20261115.json")) as f:
            plan = json.load(f)
        self.assertEqual(plan["params"]["world"], 0)
        self.assertGreaterEqual(len(plan["adds"]), 80)
        for a in plan["adds"]:
            t = plan["templates"][str(a["template_row"])]
            self.assertEqual(t["name"], a["name"])
            self.assertTrue(16200 <= a["x"] <= 16460 and 12000 <= a["z"] <= 12350, a)
            self.assertTrue(14 <= a["level"] <= 28, a)
            self.assertFalse(a["name"][:1].isupper(), a)


if __name__ == "__main__":
    unittest.main()
