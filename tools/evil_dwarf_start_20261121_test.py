#!/usr/bin/env python3
"""evil_dwarf_start_20261121.py on a small database: the plan, an apply, a second apply, an exact undo, a row changed
since left by the undo, a database without the Homelands refused, the faction pattern and an existing character's
rows moved."""
import hashlib
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import evil_dwarf_start_20261121 as tool  # noqa: E402

STATS = ", ".join(tool.STATS)
SCHEMA = [
    f"""CREATE TABLE defaultcharacter (defaultCharacterID INTEGER PRIMARY KEY AUTOINCREMENT, tclass INTEGER, race INTEGER,
        humType INTEGER, level INTEGER, tunar INTEGER, bankTunar INTEGER, unusedTP INTEGER, totalTP INTEGER, x REAL, y REAL,
        z REAL, facing REAL, defaultSpeed REAL, world INTEGER, {', '.join(s + ' INTEGER' for s in tool.STATS)})""",
    """CREATE TABLE factionstart (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER NOT NULL DEFAULT -1, class INTEGER NOT NULL
        DEFAULT -1, humtype INTEGER NOT NULL DEFAULT -1, faction TEXT NOT NULL, standing INTEGER NOT NULL, note TEXT NOT NULL
        DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', UNIQUE (race, class, humtype, faction))""",
    "CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL)",
    "CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, race INTEGER, tclass INTEGER, humType INTEGER, level INTEGER)",
    "CREATE TABLE charfactions (serverid INTEGER NOT NULL, faction TEXT NOT NULL, standing INTEGER NOT NULL, PRIMARY KEY (serverid, faction))",
    """CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL, z_coord REAL,
        facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER)""",
    "CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, listnumber INTEGER, patternid INTEGER)",
    """CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
        note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""",
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
    "CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, raceuse INTEGER)",
    "CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER, PRIMARY KEY (serverid, questid))",
]

DATA = {
    "cave_world": 10,
    "start": {"world": 10, "x": 100.0, "y": -5.0, "z": 100.0, "toward": [110.0, 100.0]},
    "camp": {"world": 0, "x": 500.0, "y": 50.0, "z": 500.0, "toward": [520.0, 500.0]},
    "freeport": {"world": 0, "x": 900.0, "y": 50.0, "z": 900.0, "facing": 1.0},
    "people": [
        {"name": "Oathbreaker Test", "like": 1, "level": 50, "role": "talker", "at": [110.0, -5.0, 100.0], "toward": [100.0, 100.0]},
        {"name": "Forgewife Test", "like": 1, "level": 30, "role": "talker", "at": [120.0, -5.0, 100.0], "toward": [100.0, 100.0], "stock": "Seller"},
        {"name": "Spell Archivist", "like": "Nobody", "level": 30, "role": "talker", "at": [130.0, -5.0, 100.0], "optional": True},
        {"name": "test grub", "like": 2, "level": 1, "role": "foe", "at": [200.0, -9.0, 200.0], "facing": 40},
        {"name": "Warden Test", "like": 1, "level": 40, "role": "talker", "world": 0, "at": [510.0, 50.0, 500.0], "toward": [500.0, 500.0]},
    ],
    "peaceful": ["test grub"],
    "hostile": [],
    "lines": {"Oathbreaker Test": ["Hello, %p."]},
    "quests": [
        {"id": 900, "key": "t/L1", "name": "First", "giver": "Oathbreaker Test", "minlevel": 1, "classquest": [1],
         "offer": "o", "accept": "a", "declined": "d", "started": "s",
         "steps": [{"kind": "talk", "target": "Forgewife Test", "journal": "j", "byclass": {"3": {"items": "7"}}},
                   {"kind": "kill", "target": "test grub", "count": 2, "journal": "j"},
                   {"kind": "talk", "target": "Oathbreaker Test", "journal": "j", "xp": 10}]},
    ],
    "trips": [{"id": 901, "key": "t/down", "name": "Down", "giver": "Warden Test", "to": "start",
               "offer": "o", "accept": "a", "declined": "d", "started": "s"}],
    "widen": {"quests": [800, 801]},
    "disable": {"quests": [802]},
    "items": {"raceuse_add": 16, "patterns": [7, 8]},
    "factions": {"like_race": 8, "own_race": 4, "classes": [3, 11], "others_like": [3, 11],
                 "others": ["Citizens of Highbourne"], "home": {"Citizens of Moradhim": 0}},
}


def make(path, homelands=True):
    db = sqlite3.connect(path)
    for s in SCHEMA:
        db.execute(s)
    db.execute(f"INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, "
               f"defaultSpeed, world, {STATS}) VALUES (0, 4, 0, 1, 0, 0, 0, 0, 1, 1, 1, 0, 0, 0, 80, 85, 60, 65, 65, 50, 50), "
               f"(0, 0, 1, 1, 0, 0, 0, 0, 2, 2, 2, 0, 0, 0, 75, 80, 65, 65, 60, 55, 55), "
               f"(3, 0, 1, 1, 0, 0, 0, 0, 3, 3, 3, 0, 0, 0, 75, 80, 65, 65, 55, 60, 55), "
               f"(11, 0, 1, 1, 0, 0, 0, 0, 4, 4, 4, 0, 0, 0, 65, 75, 55, 65, 60, 80, 55)")
    db.executemany("INSERT INTO factions VALUES (?, ?)", [("Citizens of Neriak", -2000), ("Citizens of Moradhim", 1),
                                                         ("Citizens of Highbourne", 1), ("Citizens of Halas", 1)])
    db.executemany("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (?, ?, ?, ?, ?)", [
        (8, -1, -1, "Citizens of Highbourne", 300), (8, 3, -1, "Citizens of Neriak", 1), (8, 3, -1, "Citizens of Halas", -2000),
        (8, 3, -1, "Citizens of Moradhim", -2000), (8, 11, -1, "Citizens of Neriak", 1), (8, 11, -1, "Citizens of Halas", -2000),
        (3, 11, -1, "Citizens of Highbourne", -300)])
    db.execute("INSERT INTO characters VALUES (50, 'Evilone', 4, 3, 0, 60), (51, 'Goodone', 4, 0, 0, 10)")
    db.executemany("INSERT INTO charfactions VALUES (?, ?, ?)", [(50, "Citizens of Halas", 101), (50, "Other", 5),
                                                                (51, "Citizens of Halas", 101)])
    npcs = [(1, "Template Dwarf", 0, 0, 0, 0, 0, 1, 30, 128), (2, "Template Grub", 0, 0, 0, 0, 0, 2, 20, 0),
            (3, "Seller", 0, 0, 0, 0, 0, 1, 20, 129)]
    if homelands:
        npcs.append((4, "Homeland Elder", 10, 900, 0, 900, 0, 1, 30, 128))
    db.executemany("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, npc_level, npc_type) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", npcs)
    db.execute("INSERT INTO npcitem (npcid, listnumber, patternid) VALUES (3, 0, 7), (3, 1, 8)")
    q = "INSERT INTO quests VALUES (?, ?, ?, 'g', 1, 60, '3', ?, '', 0, '', '', '', '', '', 'src', '', '', '', '', '', '')"
    db.execute(q, (800, "Chain one", 1, "0, 10"))
    db.execute(q, (801, "Every race", 1, ""))
    db.execute(q, (802, "Moradhim starter", 1, "4"))
    db.execute("INSERT INTO itempattern VALUES (7, 'shield', 1023), (8, 'earring', 783)")
    db.commit()
    db.close()


def digest(path):
    db = sqlite3.connect(path)
    h = hashlib.sha256()
    for t in ("defaultcharacter", "factionstart", "charfactions", "npcs", "npcitem", "npcaggro", "npcdialog", "quests",
              "queststeps", "itempattern", "charquests"):
        for r in db.execute(f"SELECT * FROM {t} ORDER BY 1, 2"):
            h.update(repr((t, r)).encode())
    db.close()
    return h.hexdigest()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "e.db")
        self.data = os.path.join(self.dir.name, "d.json")
        with open(self.data, "w") as f:
            json.dump(DATA, f)
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return tool.main(["--db", self.path, "--data", self.data, "--no-backup", *args])

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_report_writes_nothing(self):
        before = digest(self.path)
        self.assertEqual(self.run_tool(), 0)
        self.assertEqual(digest(self.path), before)

    def test_apply_again_undo(self):
        before = digest(self.path)
        self.assertEqual(self.run_tool("--apply"), 0)
        # two start rows a class, the deep's and the camp's, the stats the warrior's plus the class's
        self.assertEqual(self.q("SELECT tclass, world, defaultIntelligence FROM defaultcharacter WHERE race = 4 AND tclass IN (3, 11) "
                                "ORDER BY tclass, world"), [(3, 0, 55), (3, 10, 55), (11, 0, 75), (11, 10, 75)])
        # the pattern: the erudite's evil class, the gnome necromancer's Highbourne, Moradhim neutral
        got = dict(((c, f), v) for c, f, v in self.q("SELECT class, faction, standing FROM factionstart WHERE race = 4"))
        self.assertEqual(got[(3, "Citizens of Neriak")], 1)
        self.assertEqual(got[(3, "Citizens of Halas")], -2000)
        self.assertEqual(got[(3, "Citizens of Highbourne")], -300)
        self.assertEqual(got[(3, "Citizens of Moradhim")], 0)
        self.assertEqual(got[(11, "Citizens of Moradhim")], 0)
        # the existing evil dwarf's own row moved by the start's change; the warrior and other factions untouched
        self.assertEqual(self.q("SELECT serverid, faction, standing FROM charfactions ORDER BY 1, 2"),
                         [(50, "Citizens of Halas", -1900), (50, "Other", 5), (51, "Citizens of Halas", 101)])
        self.assertEqual(self.q("SELECT npc_name, world, npc_type FROM npcs WHERE id > 4 ORDER BY id"),
                         [("Oathbreaker Test", 10, 128), ("Forgewife Test", 10, 129), ("test grub", 10, 0), ("Warden Test", 0, 128)])
        self.assertEqual(self.q("SELECT count(*) FROM npcitem")[0][0], 4)
        self.assertEqual(self.q("SELECT aggressive FROM npcaggro WHERE npc_name = 'test grub'"), [(0,)])
        self.assertEqual(self.q("SELECT races FROM quests WHERE questid IN (800, 801) ORDER BY questid"), [("0, 4, 10",), ("",)])
        self.assertEqual(self.q("SELECT enabled FROM quests WHERE questid = 802"), [(0,)])
        self.assertEqual(self.q("SELECT events FROM quests WHERE questid = 900"), [('[{"on":"finish","do":"classquest","level":1}]',)])
        ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 901")[0][0])
        self.assertEqual((ev[1]["do"], ev[1]["world"], ev[1]["x"]), ("teleport", 10, 100.0))
        self.assertEqual(self.q("SELECT raceuse FROM itempattern ORDER BY patternid"), [(1023,), (799,)])
        audits = self.q("SELECT count(*) FROM gm_audit")[0][0]
        self.assertGreater(audits, 0)
        applied = digest(self.path)
        self.assertEqual(self.run_tool("--apply"), 0)
        self.assertEqual(digest(self.path), applied)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], audits)
        # a character's mark of the deep's quest goes with the undo
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charquests VALUES (50, 900, 1, 0)")
        db.commit()
        db.close()
        self.assertEqual(self.run_tool("--undo"), 0)
        self.assertEqual(digest(self.path), before)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (tool.LOG,))[0][0], 0)

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE quests SET races = '0, 10, 4, 5' WHERE questid = 800")
        db.commit()
        db.close()
        self.run_tool("--undo")
        self.assertEqual(self.q("SELECT races FROM quests WHERE questid = 800"), [("0, 10, 4, 5",)])
        self.assertEqual(self.q("SELECT count(*) FROM quests WHERE questid >= 900")[0][0], 0)
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE id > 4")[0][0], 0)

    def test_refused_without_the_homelands(self):
        os.remove(self.path)
        make(self.path, homelands=False)
        before = digest(self.path)
        self.assertEqual(self.run_tool("--apply"), 1)
        self.assertEqual(digest(self.path), before)

    def test_a_name_taken_is_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, modelid, npc_level, npc_type) "
                   "VALUES ('test grub', 0, 1, 1, 1, 2, 3, 0)")
        db.commit()
        db.close()
        before = digest(self.path)
        self.assertEqual(self.run_tool("--apply"), 1)
        self.assertEqual(digest(self.path), before)


if __name__ == "__main__":
    unittest.main()
