#!/usr/bin/env python3
"""moradhim_quarry_20261122.py on a small database: refused without the evil dwarf start, the report writes nothing, an
apply, a second apply, an exact undo (a character's mark of a trip with it), a row changed since left by the undo, and
the KOS check."""
import hashlib
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import moradhim_quarry_20261122 as tool  # noqa: E402
import evil_dwarf_start_20261121 as ed  # noqa: E402

SCHEMA = [
    """CREATE TABLE factionstart (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER NOT NULL DEFAULT -1, class INTEGER NOT NULL
        DEFAULT -1, humtype INTEGER NOT NULL DEFAULT -1, faction TEXT NOT NULL, standing INTEGER NOT NULL)""",
    "CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL)",
    """CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL, z_coord REAL,
        facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER)""",
    """CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, town TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
        name TEXT NOT NULL DEFAULT '', mood TEXT NOT NULL DEFAULT '', race INTEGER NOT NULL DEFAULT -1, line TEXT NOT NULL,
        weight INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm')""",
    """CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
        x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL)""",
    """CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER, giver TEXT, minlevel INTEGER, maxlevel INTEGER,
        classes TEXT, races TEXT, requires TEXT, repeatable INTEGER, offer TEXT, accept TEXT, decline TEXT, declined TEXT,
        started TEXT, source TEXT, notes TEXT, events TEXT, factionreq TEXT, excludes TEXT, asks TEXT, daygroup TEXT)""",
    """CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, npcid INTEGER, items TEXT, tunar INTEGER,
        count INTEGER, world INTEGER, x REAL, y REAL, z REAL, radius REAL, journal TEXT, answer TEXT, says TEXT, waiting TEXT,
        notyet TEXT, reminder TEXT, xp INTEGER, rewardtunar INTEGER, rewarditems TEXT, factions TEXT, spells TEXT, events TEXT,
        chance REAL, byclass TEXT, choices TEXT, height REAL, anyorder INTEGER, PRIMARY KEY (questid, step))""",
    "CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER, PRIMARY KEY (serverid, questid))",
]


def make(path, deep=True):
    db = sqlite3.connect(path)
    for s in SCHEMA:
        db.execute(s)
    db.executemany("INSERT INTO factions VALUES (?, ?)", [("Citizens of Moradhim", 1), ("Militia", 1)])
    db.executemany("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, npc_level, npc_type) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                   [(1, "Coachman Doba", 0, 15351.6, 54.1, 9267.1, 0, 7, 30, 128),
                    (2, tool.CAMP_WARDEN, 0, 15310.0, 53.84, 9410.0, 230, 2858091591, 40, 128),
                    (3, tool.DEEP_WARDEN, 10, 18715.0, -59.09, 9140.0, 0, 2858091591, 40, 128)])
    db.execute("INSERT INTO npcfaction (name, faction) VALUES ('Coachman Doba', 'Citizens of Moradhim')")
    if deep:
        db.execute(ed.LOG_DDL)
        db.execute(f"INSERT INTO {ed.LOG} (step, tbl, ref, at) VALUES ('people', 'npcs', '{{}}', 'then')")
    db.commit()
    db.close()


def digest(path):
    db = sqlite3.connect(path)
    h = hashlib.sha256()
    for t in ("npcs", "npcdialog", "quests", "queststeps", "charquests", "factionstart"):
        for r in db.execute(f"SELECT * FROM {t} ORDER BY 1, 2"):
            h.update(repr((t, r)).encode())
    db.close()
    return h.hexdigest()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return tool.main(["--db", self.path, "--no-backup", *args])

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_refused_without_the_deep(self):
        os.remove(self.path)
        make(self.path, deep=False)
        before = digest(self.path)
        self.assertEqual(self.run_tool("--apply"), 1)
        self.assertEqual(digest(self.path), before)

    def test_report_writes_nothing(self):
        before = digest(self.path)
        self.assertEqual(self.run_tool(), 0)
        self.assertEqual(digest(self.path), before)

    def test_apply_again_undo(self):
        before = digest(self.path)
        self.assertEqual(self.run_tool("--apply"), 0)
        n = self.q("SELECT npc_name, world, x_coord, z_coord, npc_type, modelid, npc_level, id = npc_id FROM npcs WHERE id > 3")
        self.assertEqual(n, [(tool.WARDEN["name"], 0, 15553.0, 9158.0, 128, 2858091591, 40, 1)])
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog WHERE source = ?", (tool.DIALOG_SOURCE,))[0][0],
                         sum(len(v) for v in tool.LINES.values()))
        got = {r[0]: r[1:] for r in self.q("SELECT questid, giver, classes, races, repeatable FROM quests")}
        self.assertEqual(got[105301], (tool.WARDEN["name"], "3, 11", "4", 1))
        self.assertEqual(got[105303], (tool.DEEP_WARDEN, "3, 11", "4", 1))
        ev = {qid: json.loads(e)[1] for qid, e in self.q("SELECT questid, events FROM queststeps")}
        deep = json.load(open(ed.DATA))
        self.assertEqual((ev[105301]["world"], ev[105301]["x"], ev[105301]["z"]), (10, deep["start"]["x"], deep["start"]["z"]))
        self.assertNotIn("world", ev[105302])  # Freeport, world 0
        self.assertEqual((ev[105303].get("world", 0), ev[105303]["x"], ev[105303]["z"]), (0, tool.QUARRY["x"], tool.QUARRY["z"]))
        audits = self.q("SELECT count(*) FROM gm_audit")[0][0]
        self.assertEqual(audits, self.q(f"SELECT count(*) FROM {tool.LOG}")[0][0])
        applied = digest(self.path)
        self.assertEqual(self.run_tool("--apply"), 0)
        self.assertEqual(digest(self.path), applied)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charquests VALUES (50, 105301, 1, 1)")
        db.commit()
        db.close()
        self.assertEqual(self.run_tool("--undo"), 0)
        self.assertEqual(digest(self.path), before)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (tool.LOG,))[0][0], 0)

    def test_undo_leaves_a_row_moved_since(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET x_coord = 15540 WHERE npc_name = ?", (tool.WARDEN["name"],))
        db.commit()
        db.close()
        self.run_tool("--undo")
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE npc_name = ?", (tool.WARDEN["name"],)), [(15540.0,)])
        self.assertEqual(self.q("SELECT count(*) FROM quests")[0][0], 0)

    def test_check_finds_a_kos_neighbour(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('Militia Guard', 0, 15530, 41, 9170)")
        db.execute("INSERT INTO npcfaction (name, faction) VALUES ('Militia Guard', 'Militia')")
        db.execute("INSERT INTO factionstart (race, class, faction, standing) VALUES (4, 3, 'Militia', -2000)")
        db.commit()
        self.assertIn("class 3: Militia Guard (Militia) by the quarry is KOS to it", tool.check(db))
        self.assertFalse([p for p in tool.check(db) if p.startswith("class 11")])
        db.close()
        self.assertEqual(self.run_tool(), 1)


if __name__ == "__main__":
    unittest.main()
