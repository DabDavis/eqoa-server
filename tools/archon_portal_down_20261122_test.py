#!/usr/bin/env python3
"""archon_portal_down_20261122.py on a small database: apply, a second apply that writes nothing, undo exact; a row not
as read refuses the whole run; the triggers' steps are the ones the Go test walks (archon_portal_down_test.go)."""
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import archon_portal_down_20261122 as tool  # noqa: E402

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
        (120972, "Archon Aereth", 1, 4699.23, 110.312, 2512.66, 147, 67, 0, 515091180, 1.0, 120972),
        (120974, "Archon Vetash", 1, 4699.55, 106.0, 2342.21, 10, 67, 0, 2856885435, 1.0, 120974),
        (162483, "Portal to Archon Aereth", 1, 4692.0, 66.0, 2512.0, 0, 55, 384, 2012826467, 0.7, 0),
        (162485, "Portal to Archon Vetash", 1, 4708.0, 66.0, 2330.0, 128, 55, 384, 2012826467, 0.7, 0),
        (196277, "Sarek the Betrayer", 1, 4700.0, 110.375, 2500.0, 186, 67, 0, 2577583855, 1.0, 196277)])
    db.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (402, 'Sarek the Betrayer', '[]')")
    db.execute("INSERT INTO npcspawnonly VALUES ('#196277', 'the quest Sarek', 'board 709')")
    db.execute("INSERT INTO npcdeath (name, trigger) VALUES ('granavik', 'Rain 3 Portal Opens')")
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
                             "npc_level, npc_id FROM npcs WHERE npc_name LIKE 'Portal down%' ORDER BY id").fetchall()
        self.assertEqual(portals, [
            (196278, "Portal down from Aereth", 1, 4695.0, 110.375, 2493.0, 0, 384, 2012826467, 0.7, 55, 0),
            (196279, "Portal down from Vetash", 1, 4697.0, 106.0, 2353.0, 128, 384, 2012826467, 0.7, 55, 0)])
        for _, name, *_ in portals:
            self.assertLessEqual(len(name), 23)
        self.assertEqual(sorted(r[0] for r in db.execute("SELECT name FROM npcspawnonly")),
                         ["#196277", "portal down from aereth", "portal down from vetash"])
        trig = {r[1]: r for r in db.execute("SELECT slot, name, scope, radius, world, steps FROM gmtriggers")}
        self.assertEqual((trig["Aereth Portal Down"][0], trig["Vetash Portal Down"][0]), (403, 404))
        deaths = db.execute("SELECT name, trigger, chance FROM npcdeath ORDER BY 1, 2").fetchall()
        self.assertEqual(deaths, [("archon aereth", "Aereth Portal Down", 100.0), ("archon vetash", "Vetash Portal Down", 100.0),
                                  ("granavik", "Rain 3 Portal Opens", 100.0)])
        # the Go test walks these very steps
        with open(os.path.join(HERE, "..", "internal", "handlers", "archon_portal_down_test.go")) as f:
            go = f.read()
        for name in ("Aereth Portal Down", "Vetash Portal Down"):
            steps = trig[name][5]
            self.assertIn(f'"{name}": `{steps}`', go, f"{name}'s steps are not the Go test's")
        audits = db.execute("SELECT count(*) FROM gm_audit WHERE username = 'dab' AND via = 'tool'").fetchone()[0]
        self.assertEqual(audits, 8)
        # Sarek's rows untouched
        self.assertEqual(db.execute("SELECT steps FROM gmtriggers WHERE slot = 402").fetchone()[0], "[]")
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
        db.execute("UPDATE npcs SET x_coord = 4600 WHERE npc_name = 'Archon Vetash'")
        db.commit()
        db.close()
        before = snapshot(self.db, skip=())
        rc, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)
        self.assertEqual(snapshot(self.db, skip=()), before)

    def test_refuses_a_name_taken(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('Portal down from Aereth', 1, 1, 1, 1)")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("not this tool's", out)

    def test_refuses_a_portal_up_changed(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET npc_type = 0 WHERE id = 162483")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--shard", "heirloom")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)

    def test_dump(self):
        rc, out = run("--db", self.db, "--shard", "heirloom", "--dump")
        self.assertEqual(rc, 0)
        d = json.loads(out)
        self.assertEqual(sorted(d["triggers"]), ["Aereth Portal Down", "Vetash Portal Down"])


if __name__ == "__main__":
    unittest.main()
