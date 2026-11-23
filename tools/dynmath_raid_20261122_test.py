#!/usr/bin/env python3
"""dynmath_raid_20261122.py on a small database: apply, a second apply that writes nothing, undo exact; a row not as
read refuses the whole run; the trigger's steps are the ones the Go test walks (dynmath_raid_test.go)."""
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
import dynmath_raid_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, dt_kill DATE, npc_level INTEGER, npc_type INTEGER, modelid INTEGER, size REAL,
    npc_id INTEGER, race INTEGER);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    scope TEXT NOT NULL DEFAULT 'near', radius REAL NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, steps TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '',
    special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0, special_fx INTEGER NOT NULL DEFAULT 0,
    abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    special_every REAL NOT NULL DEFAULT 0, special_first REAL NOT NULL DEFAULT 0, special_jitter REAL NOT NULL DEFAULT 0,
    special_from INTEGER NOT NULL DEFAULT 0);
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcundead (name TEXT PRIMARY KEY, undead INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT DEFAULT '',
    events TEXT DEFAULT '', PRIMARY KEY (questid, step));
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO npcs VALUES (122985, 'Dynmath the Tyrant', 0, 4765.387, 58.9529, 14028.429, 0, NULL, 60, 0, "
               "3965346723, 1.0, 0, 12)")
    db.execute("INSERT INTO npcs VALUES (122984, 'Etheneric Portal', 0, 4767.05, 59.38, 14043.34, 0, NULL, 1, 384, "
               "2012826467, 1.0, 0, 0)")
    db.execute("INSERT INTO npcspawnonly VALUES ('Dynmath the Tyrant', 'questspawn', 'questspawn')")
    db.execute("INSERT INTO npctemplate VALUES ('dynmath the tyrant', 'roamer', 'boss, level 60', 'roamer-seed')")
    db.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (312, 'Other', '[]')")
    db.executemany("INSERT INTO quests VALUES (?, ?)", [(101257, "Dynmath the Tyrant"), (101258, "Another Totem of Dynmath")])
    db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?, ?)", [
        (101257, 1, "give", "Etheneric Portal", "701360", ""),
        (101257, 2, "kill", "Dynmath the Tyrant", "", tool.OLD_KILL_EVENTS),
        (101257, 3, "talk", "Historian Ilena", "", ""),
        (101258, 1, "give", "Historian Ilena", "", '[{"on":"done","do":"give","items":"701360"}]'),
        (101258, 2, "give", "Etheneric Portal", "701360", ""),
        (101258, 3, "kill", "Dynmath the Tyrant", "", tool.OLD_KILL_EVENTS)])
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


def snapshot(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f'SELECT * FROM "{t}" ORDER BY rowid').fetchall()
           for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' "
                                  "AND name != 'gm_audit' ORDER BY 1")}
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
        self.assertEqual((rc, snapshot(self.db)), (0, before), out)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT modelid, size FROM npcs WHERE id = 122985").fetchone(), (0x001D9EDF, 2.5))
        self.assertEqual(db.execute("SELECT id, modelid, size, npc_level FROM npcs WHERE npc_name = 'minion of Dynmath'")
                         .fetchall(), [(122986, 0x1BBB898C, 1.1, 57)])
        self.assertEqual(db.execute("SELECT template FROM npctemplate WHERE name = 'dynmath the tyrant'").fetchone()[0],
                         "dynmath the tyrant raid")
        self.assertEqual(db.execute("SELECT hp_pct, class FROM classtemplates WHERE name = 'dynmath the tyrant raid'")
                         .fetchone(), (3000, 11))
        self.assertEqual(sorted(r[0] for r in db.execute("SELECT name FROM npcundead")),
                         ["dynmath the tyrant", "minion of dynmath"])
        slot, steps = db.execute("SELECT slot, steps FROM gmtriggers WHERE name = 'Dynmath the Tyrant'").fetchone()
        self.assertEqual(slot, 313)
        ev = dict(((q, s), e) for q, s, e in db.execute("SELECT questid, step, events FROM queststeps"))
        self.assertEqual(ev[(101257, 1)], tool.HANDIN_EVENTS)
        self.assertEqual(ev[(101258, 2)], tool.HANDIN_EVENTS)
        self.assertEqual((ev[(101257, 2)], ev[(101258, 3)]), ("", ""))
        self.assertEqual(json.loads(ev[(101258, 1)])[0]["do"], "give")
        with open(os.path.join(HERE, "..", "internal", "handlers", "dynmath_raid_test.go")) as f:
            go = f.read()
        self.assertIn(f"const dynmathSteps = `{steps}`", go, "the trigger's steps are not the Go test's")
        abil = db.execute("SELECT abilities FROM classtemplates WHERE name = 'dynmath the tyrant raid'").fetchone()[0]
        self.assertIn(f"const dynmathAbilities = `{abil}`", go, "his abilities are not the Go test's")
        self.assertTrue(any(a["kind"] == "portnear" and a.get("top") for a in json.loads(abil)))
        db.close()
        after = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertIn("nothing to write", out)
        self.assertEqual(snapshot(self.db), after)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--undo")
        self.assertIn("14 rows put back", out)
        self.assertEqual(snapshot(self.db), before)

    def test_refuses_when_not_as_read(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE queststeps SET events = 'x' WHERE questid = 101258 AND step = 3")
        db.commit()
        db.close()
        before = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)
        self.assertEqual(snapshot(self.db), before)

    def test_refuses_a_name_taken(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('minion of Dynmath', 0, 1, 1, 1)")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("not this tool's", out)


if __name__ == "__main__":
    unittest.main()
