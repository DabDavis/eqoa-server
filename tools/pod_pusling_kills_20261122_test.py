#!/usr/bin/env python3
"""pod_pusling_kills_20261122.py on a small database: apply, a second apply that writes nothing, undo exact; a name
spawning outside PoD or holding another Pusling hit refuses the whole run."""
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import pod_pusling_kills_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL,
    npc_type INTEGER);
CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL, faction TEXT NOT NULL,
    delta INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm', UNIQUE (npc_name, faction));
CREATE TABLE factionhitareas (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL, faction TEXT NOT NULL,
    delta INTEGER NOT NULL, world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL, z REAL NOT NULL, radius REAL NOT NULL,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO factions VALUES ('The Pusling', -400)")
    for n in tool.ONE + tool.NAMED:
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, z_coord, npc_type) VALUES (?, 5, 4800, 3000, 0)", (n,))
    db.execute("INSERT INTO npcs (npc_name, world, x_coord, z_coord, npc_type) VALUES ('plague wasp', 1, 7000, 9000, 0)")
    db.execute("INSERT INTO npcs (npc_name, world, x_coord, z_coord, npc_type) VALUES ('pusling lacer', 5, 4800, 3000, 0)")
    db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES ('scuzz toad', 'The Pusling', 1, 'x')")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


def snap(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY rowid").fetchall() for t in ("factionhits", "factionhitareas")}
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
        before = snap(self.db)
        rc, out = run("--db", self.db)
        self.assertEqual((rc, snap(self.db)), (0, before), out)
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        hits = dict(db.execute("SELECT npc_name, delta FROM factionhits WHERE faction = 'The Pusling'").fetchall())
        self.assertEqual(len(hits), len(tool.ONE) + len(tool.NAMED) + 1)
        self.assertEqual((hits["scourge rat"], hits["Rawlg the Carnage Hound"], hits["scuzz toad"]), (1, 25, 1))
        self.assertNotIn("pusling lacer", hits)
        self.assertNotIn("plague wasp", hits)
        self.assertEqual(db.execute("SELECT npc_name, delta, world FROM factionhitareas").fetchall(), [("plague wasp", 1, 5)])
        db.close()
        after = snap(self.db)
        rc, out = run("--db", self.db, "--apply")
        self.assertIn("nothing to write", out)
        self.assertEqual(snap(self.db), after)
        rc, out = run("--db", self.db, "--undo")
        self.assertIn(f"{len(tool.ONE) + len(tool.NAMED) + 1} rows deleted", out)
        self.assertEqual(snap(self.db), before)

    def test_rows_put_in_by_the_panel_are_kept(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES ('scourge rat', 'The Pusling', 1, 'gm')")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("scourge rat: The Pusling +1 already there", out)

    def test_refuses_a_name_outside_pod(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, z_coord, npc_type) VALUES ('scourge rat', 0, 100, 100, 0)")
        db.commit()
        db.close()
        before = snap(self.db)
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("outside PoD", out)
        self.assertEqual(snap(self.db), before)

    def test_refuses_another_hit(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO factionhits (npc_name, faction, delta) VALUES ('Grum', 'The Pusling', 5)")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)


if __name__ == "__main__":
    unittest.main()
