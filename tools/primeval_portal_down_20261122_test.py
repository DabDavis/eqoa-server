#!/usr/bin/env python3
"""primeval_portal_down_20261122.py on a small database: apply, a second apply that writes nothing, undo exact; a row not
as read refuses the whole run; the triggers' steps are the ones the Go test walks (primeval_portal_down_test.go)."""
import io
import json
import os
import re
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import primeval_portal_down_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, dt_kill DATE, npc_level INTEGER, npc_type INTEGER, modelid INTEGER, size REAL,
    npc_id INTEGER);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    scope TEXT NOT NULL DEFAULT 'near', radius REAL NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, steps TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '');
CREATE TABLE npcdeath (name TEXT NOT NULL, trigger TEXT NOT NULL, chance REAL NOT NULL DEFAULT 100,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (name, trigger));
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?)", [
        (162487, "Earth Primeval", 1, 4700.0, 85.3, 2511.0, 128, 67, 0, 931615265, 1.6, 0),
        (162488, "Fire Primeval", 1, 4700.0, 85.5, 2344.0, 0, 67, 0, 2319540801, 1.6, 0),
        (162484, "Earth Primeval Portal", 1, 4701.22, 67.4, 2524.94, 133, 55, 384, 2012826467, 0.7, 0),
        (162486, "Fire Primeval Portal", 1, 4698.61, 67.37, 2321.74, 2, 55, 384, 2012826467, 0.7, 0),
        (196282, "Portal down from Aereth", 1, 4695.0, 110.375, 2493.0, 0, 55, 384, 2012826467, 0.7, 0)])
    db.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (311, 'Aereth Portal Down', '[]')")
    db.execute("INSERT INTO npcspawnonly VALUES ('portal down from aereth', 'the Archon portal', 'petition 2670')")
    db.execute("INSERT INTO npcdeath (name, trigger) VALUES ('archon aereth', 'Aereth Portal Down')")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


def snapshot(path, skip=("gm_audit",)):
    db = sqlite3.connect(path)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY 1"):
        if t not in skip:
            out[t] = db.execute(f'SELECT * FROM "{t}" ORDER BY rowid').fetchall()
    db.close()
    return out


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        before = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "heirloom")
        self.assertEqual(rc, 0, out)
        self.assertEqual(snapshot(self.db), before, "a dry run wrote")
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        portals = db.execute("SELECT id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_type, modelid, size, "
                             "npc_level, npc_id FROM npcs WHERE npc_name LIKE 'Portal down%' AND id > 196282 ORDER BY id").fetchall()
        self.assertEqual(portals, [
            (196283, "Portal down from Earth", 1, 4695.0, 83.0, 2493.0, 0, 384, 2012826467, 0.7, 55, 0),
            (196284, "Portal down from Fire", 1, 4697.0, 84.0, 2357.0, 128, 384, 2012826467, 0.7, 55, 0)])
        for _, name, *_ in portals:
            self.assertLessEqual(len(name), 23)
        self.assertEqual(sorted(r[0] for r in db.execute("SELECT name FROM npcspawnonly")),
                         ["portal down from aereth", "portal down from earth", "portal down from fire"])
        trig = {r[1]: r for r in db.execute("SELECT slot, name, scope, radius, world, steps FROM gmtriggers")}
        self.assertEqual((trig["Earth Portal Down"][0], trig["Fire Portal Down"][0]), (312, 313))
        deaths = db.execute("SELECT name, trigger, chance FROM npcdeath ORDER BY 1, 2").fetchall()
        self.assertEqual(deaths, [("archon aereth", "Aereth Portal Down", 100.0), ("earth primeval", "Earth Portal Down", 100.0),
                                  ("fire primeval", "Fire Portal Down", 100.0)])
        # the Go test walks these very steps
        with open(os.path.join(HERE, "..", "internal", "handlers", "primeval_portal_down_test.go")) as f:
            go = f.read()
        for name in ("Earth Portal Down", "Fire Portal Down"):
            steps = trig[name][5]
            self.assertRegex(go, re.escape(f'"{name}":') + r"\s+" + re.escape(f"`{steps}`"),
                             f"{name}'s steps are not the Go test's")  # gofmt aligns the map's values
        audits = db.execute("SELECT count(*) FROM gm_audit WHERE username = 'dab' AND via = 'tool'").fetchone()[0]
        self.assertEqual(audits, 8)
        # the Archon's rows untouched
        self.assertEqual(db.execute("SELECT steps FROM gmtriggers WHERE slot = 311").fetchone()[0], "[]")
        db.close()

        after = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to write", out)
        self.assertEqual(snapshot(self.db), after)

        rc, out = run("--db", self.db, "--shard", "heirloom", "--undo")
        self.assertEqual(rc, 0, out)
        self.assertIn("8 rows put back", out)
        self.assertEqual(snapshot(self.db), before)

    def test_refuses_when_not_as_read(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = 4600 WHERE npc_name = 'Fire Primeval'")
        db.commit()
        db.close()
        before = snapshot(self.db, skip=())
        rc, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)
        self.assertEqual(snapshot(self.db, skip=()), before)

    def test_refuses_a_name_taken(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('Portal down from Fire', 1, 1, 1, 1)")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("not this tool's", out)

    def test_refuses_a_portal_up_changed(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET npc_type = 0 WHERE id = 162484")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--shard", "heirloom")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)

    def test_dump(self):
        rc, out = run("--db", self.db, "--shard", "heirloom", "--dump")
        self.assertEqual(rc, 0)
        d = json.loads(out)
        self.assertEqual(sorted(d["triggers"]), ["Earth Portal Down", "Fire Portal Down"])


if __name__ == "__main__":
    unittest.main()
