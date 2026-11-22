#!/usr/bin/env python3
"""jib_batch2_20261122.py on a small database: apply, a second apply, undo exact; a quest's steps moved with the
characters part-way and a reach step put in; a row not as read refuses its part; --only / --skip."""
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jib_batch2_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, npc_level INTEGER, npc_type INTEGER,
    modelid INTEGER, x_coord REAL, y_coord REAL, z_coord REAL, npc_id INTEGER);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0,
    water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    speed REAL NOT NULL DEFAULT 0);
CREATE TABLE npcundead (name TEXT PRIMARY KEY, undead INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT, classuse INTEGER, raceuse INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1,
    notes TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', items TEXT NOT NULL DEFAULT '', world INTEGER NOT NULL DEFAULT 0,
    x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 15,
    journal TEXT NOT NULL DEFAULT '', factions TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
    PRIMARY KEY (questid, step, target));
CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
    count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, questid));
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO npcs (id, npc_name, world, npc_level, npc_type, modelid, x_coord, y_coord, z_coord, npc_id) "
                   "VALUES (?, ?, 0, ?, 0, 1, 1, 2, 3, 0)",
                   [(3122, "frosteye shaman", 8), (108503, "root spirit", 20), (108500, "wailing spirit", 2),
                    (122481, "aggravated spirit", 44), (113818, "Tobul", 32), (113865, "Troi Respin", 38)])
    db.execute("INSERT INTO itempattern VALUES (8116, 'Spirit Charm', 32767, 1023)")
    db.executemany("INSERT INTO quests (questid, name, minlevel) VALUES (?, ?, 60)",
                   [(q, f"q{q}") for q in tool.ZP_QUESTS] + [(101663, "Weapons for the Liason"), (100786, "The Restless")])
    db.execute("INSERT INTO queststeps (questid, step, kind, target, items, factions) VALUES "
               "(101663, 1, 'give', 'Akessan Liason', '991099*4', 'Akessan Collective:+4')")
    for n, t in tool.ZP_LOOT:
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance) VALUES (?, ?, 50)", (n, t))
    for s in range(1, 9):
        db.execute("INSERT INTO queststeps (questid, step, kind, target) VALUES (100786, ?, ?, ?)",
                   (s, "loot" if s == 5 else "give", "aggravated spirit" if s == 5 else "Broyce Sandstalker"))
    db.execute("INSERT INTO queststeptargets VALUES (100786, 6, 'someone')")
    db.executemany("INSERT INTO charquests (serverid, questid, step, done) VALUES (?, 100786, ?, ?)",
                   [(1, 5, 0), (2, 3, 0), (3, 0, 1)])
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


def snapshot(path):
    db = sqlite3.connect(path)
    s = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
         for t in ("npcs", "npcspawnonly", "npcmove", "npcundead", "npcloot", "loottables", "itempattern", "quests",
                   "queststeps", "queststeptargets", "charquests")}
    db.close()
    return s


class Batch2(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.tmp.name, "t.db")
        make(self.db)

    def tearDown(self):
        self.tmp.cleanup()

    def test_apply_again_undo(self):
        before = snapshot(self.db)
        parts = "898,1777,1011,936"
        rc, out = run("--db", self.db, "--shard", "heirloom", "--only", parts)
        self.assertEqual(rc, 0, out)
        self.assertIn("dry run", out)
        self.assertEqual(snapshot(self.db), before)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--only", parts, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT npc_level FROM npcs WHERE id = 3122").fetchone()[0], 7)
        self.assertEqual(db.execute("SELECT count(*) FROM quests WHERE minlevel = 55").fetchone()[0], 6)
        self.assertEqual(db.execute("SELECT factions FROM queststeps WHERE questid = 101663").fetchone()[0],
                         "Akessan Collective:+12")
        self.assertEqual(db.execute("SELECT min(chance) FROM npcloot WHERE tablename LIKE 'zp %'").fetchone()[0], 100)
        steps = db.execute("SELECT step, kind, target FROM queststeps WHERE questid = 100786 ORDER BY step").fetchall()
        self.assertEqual(len(steps), 9)
        self.assertEqual(steps[4][1], "reach")
        self.assertEqual(steps[5], (6, "loot", "aggravated spirit"))
        self.assertIn('"npc":"aggravated spirit"', db.execute(
            "SELECT events FROM queststeps WHERE questid = 100786 AND step = 5").fetchone()[0])
        self.assertEqual(db.execute("SELECT step FROM queststeptargets").fetchone()[0], 7)
        cq = dict(db.execute("SELECT serverid, step FROM charquests").fetchall())
        self.assertEqual(cq, {1: 6, 2: 3, 3: 0})  # part-way at the kill moves with it; before it and done stay
        self.assertEqual(db.execute("SELECT count(*) FROM npcspawnonly").fetchone()[0], 1)
        self.assertEqual(db.execute("SELECT count(*) FROM loottables WHERE tablename = 'wailing spirit'").fetchone()[0], 1)
        audits = db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        db.close()
        rc, out = run("--db", self.db, "--shard", "heirloom", "--only", parts, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to write", out)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--undo")
        self.assertEqual(rc, 0, out)
        after = snapshot(self.db)
        self.assertEqual(after, before)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 2 * audits)
        self.assertIsNone(db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (tool.LOG,)).fetchone())
        db.close()

    def test_not_as_read_refuses_its_part(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET npc_level = 9 WHERE id = 3122")
        db.execute("INSERT INTO queststeps (questid, step) VALUES (100786, 9)")
        db.commit()
        db.close()
        before = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--only", "898,936,1011", "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("898: REFUSED", out)
        self.assertIn("936: REFUSED", out)
        self.assertEqual(snapshot(self.db), before)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--only", "898,936,1011", "--skip", "898,936", "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT npc_level FROM npcs WHERE id = 108503").fetchone()[0], 21)
        db.close()

    def test_live_only_part_on_heirloom_writes_nothing(self):
        rc, out = run("--db", self.db, "--shard", "heirloom", "--only", "785", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to write", out)


if __name__ == "__main__":
    unittest.main()
