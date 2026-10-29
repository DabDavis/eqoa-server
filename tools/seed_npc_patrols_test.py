#!/usr/bin/env python3
"""tools/seed_npc_patrols.py against made-up tracks in Sandstorm's shape (a
walker's place sent at its stops and as it sets off, its velocity between):
a square walked round twice is a loop of its four corners at its pace and
pause; a pacer is two points back and forth; a walker seen for one leg goes
back and forth over what was seen; a rat, a guard standing and the watcher
are left out; an NPC with a GM's path is left; each route goes to the NPC of
its name nearest it; --undo gives back npcpaths as it was. No game data.

    python3 tools/seed_npc_patrols_test.py
"""
import json
import math
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_npc_patrols.py")


def walk(rows, sid, name, legs, t0=0, pace=4.0, pause=2.0):
    """Rows of a walker going through legs [(x, z)...]: at each point a stop
    (velocity 0) held `pause` s, then a leg at `pace`, sampled every 0.5 s,
    its place sent only at the stops and as it sets off."""
    t = t0
    sent = legs[0]
    for (ax, az), (bx, bz) in zip(legs, legs[1:]):
        for _ in range(int(pause / 0.5)):
            rows.append({"t": int(t * 1000), "id": sid, "name": name, "x": ax, "y": 10, "z": az, "v": [0, 0, 0]})
            t += 0.5
        d = math.hypot(bx - ax, bz - az)
        v = [pace * (bx - ax) / d, 0, pace * (bz - az) / d]
        for _ in range(int(d / pace / 0.5)):
            rows.append({"t": int(t * 1000), "id": sid, "name": name, "x": ax, "y": 10, "z": az, "v": v})
            t += 0.5
        sent = (bx, bz)
    rows.append({"t": int(t * 1000), "id": sid, "name": name, "x": sent[0], "y": 10, "z": sent[1], "v": [0, 0, 0]})
    return t


class SeedNPCPatrols(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        d = self.dir.name
        self.db = os.path.join(d, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, x_coord REAL, z_coord REAL, world INTEGER)")
        db.execute("CREATE TABLE npcpaths (npcid INTEGER PRIMARY KEY, world INTEGER NOT NULL DEFAULT 0, mode TEXT NOT NULL DEFAULT 'loop', "
                   "pause REAL NOT NULL DEFAULT 0, speed TEXT NOT NULL DEFAULT 'walk', points TEXT NOT NULL DEFAULT '[]')")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
            (1, "Guard Rounds", 1000, 1000, 0),
            (2, "Guard Rounds", 3000, 3000, 0),     # another of the name, far off: not it
            (3, "Piker Pace", 1100, 1000, 0),
            (4, "Walker Once", 1200, 1000, 0),
            (5, "Guard Kept", 1300, 1000, 0),       # a GM's path
            (6, "Giant Rat", 1400, 1000, 0),
        ])
        db.execute("INSERT INTO npcpaths VALUES (5, 0, 'loop', 0, 'walk', '[]')")
        db.commit()
        db.close()
        rows = []
        square = [(1000, 1000), (1040, 1000), (1040, 1040), (1000, 1040), (1000, 1000)]
        walk(rows, 11, "Guard Rounds", square + square[1:], pace=4.0, pause=2.0)
        walk(rows, 13, "Piker Pace", [(1100, 1000), (1112, 1000), (1100, 1000), (1112, 1000), (1100, 1000)], pace=3.12, pause=10.0)
        # one leg, then out of view
        rows += [{"t": 0, "id": 14, "name": "Walker Once", "x": 1200, "y": 10, "z": 1000, "v": [5, 0, 0]},
                 {"t": 4000, "id": 14, "name": "Walker Once", "x": 1220, "y": 10, "z": 1000, "v": [5, 0, 0]}]
        walk(rows, 15, "Guard Kept", [(1300, 1000), (1330, 1000), (1300, 1000)])
        walk(rows, 16, "Giant Rat", [(1400, 1000), (1420, 1000), (1400, 1000)])
        rows += [{"t": t, "id": 17, "name": "Guard Still", "x": 1500, "y": 10, "z": 1000, "v": [0, 0, 0]} for t in range(0, 20000, 500)]
        walk(rows, 18, "Pwwatcher", [(1600, 1000), (1650, 1000)])
        self.tracks = os.path.join(d, "tracks")
        os.makedirs(self.tracks)
        with open(os.path.join(self.tracks, "0-0-1.json"), "w") as f:
            json.dump({"made": {"name": "Pwwatcher", "id": 9}}, f)
        with open(os.path.join(self.tracks, "0-0-1.json.track.jsonl"), "w") as f:
            for r in sorted(rows, key=lambda r: r["t"]):
                f.write(json.dumps(r) + "\n")
            f.write('{"t": 99999, "id": 11, "na')  # a line cut off

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--tracks", self.tracks, *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def paths(self):
        c = sqlite3.connect(self.db)
        try:
            return {r[0]: r[1:] for r in c.execute("SELECT npcid, world, mode, pause, speed, points FROM npcpaths")}
        finally:
            c.close()

    def test_apply_and_undo(self):
        before = self.paths()
        out = self.run_tool().stdout
        self.assertIn("3 patrols", out)
        self.assertIn("left: Guard Kept (a GM gave it a path)", out)
        self.run_tool("--apply")
        p = self.paths()
        world, mode, pause, speed, pts = p[1]
        self.assertEqual((mode, speed, pause), ("loop", "4", 2.0))
        self.assertEqual([(q["x"], q["z"]) for q in json.loads(pts)], [(1000, 1000), (1040, 1000), (1040, 1040), (1000, 1040)])
        mode, pause, speed, pts = p[3][1:]
        self.assertEqual((mode, speed, pause), ("pingpong", "3.12", 10.0))
        self.assertEqual(sorted((q["x"], q["z"]) for q in json.loads(pts)), [(1100, 1000), (1112, 1000)])
        mode, pts = p[4][1], p[4][4]
        self.assertEqual((mode, [(q["x"], q["z"]) for q in json.loads(pts)]), ("pingpong", [(1200, 1000), (1220, 1000)]))
        self.assertEqual(p[5], before[5])          # the GM's
        self.assertNotIn(2, p)                       # the far namesake
        self.assertNotIn(6, p)                       # the rat
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.paths(), before)


class AddMissing(unittest.TestCase):
    """--add: a walker of no NPC here is made one, its looks off its who line
    and the rest off the NPC nearest its route, of the faction of the nearest
    of its town's people; --undo takes the NPC, its faction row and its path
    away."""

    COLS = ("id INTEGER PRIMARY KEY, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL, z_coord REAL, facing REAL, "
            "world INTEGER, hp INTEGER, hp_percent INTEGER, modelid INTEGER, size REAL, primary_weapon INTEGER, "
            "secondary_weapon INTEGER, shield INTEGER, torso INTEGER, forearm INTEGER, gloves INTEGER, legs INTEGER, "
            "feet INTEGER, head INTEGER, hair_color INTEGER, hair_length INTEGER, hair_style INTEGER, face INTEGER, "
            "robe_style TEXT, race TEXT, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER")

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        d = self.dir.name
        self.db = os.path.join(d, "t.db")
        db = sqlite3.connect(self.db)
        db.execute(f"CREATE TABLE npcs ({self.COLS})")
        db.execute("CREATE TABLE npcpaths (npcid INTEGER PRIMARY KEY, world INTEGER NOT NULL DEFAULT 0, mode TEXT NOT NULL DEFAULT 'loop', "
                   "pause REAL NOT NULL DEFAULT 0, speed TEXT NOT NULL DEFAULT 'walk', points TEXT NOT NULL DEFAULT '[]')")
        db.execute("CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, world INTEGER DEFAULT -1, x REAL DEFAULT 0, "
                   "z REAL DEFAULT 0, radius REAL DEFAULT 0, faction TEXT, note TEXT DEFAULT '', source TEXT DEFAULT 'gm')")
        db.execute("INSERT INTO npcs VALUES (1, 'Guard Bank', 77, 1010, 10, 1000, 0, 0, 3000, 100, 555, 1, 0, 0, 0, "
                   "1, 1, 1, 1, 1, 1, 0, 0, 0, 0, '255', '1', 40, 0, 0)")
        db.execute("INSERT INTO npcs VALUES (2, 'Smith Far', 78, 5000, 10, 5000, 0, 0, 900, 100, 555, 1, 0, 0, 0, "
                   "0, 0, 0, 0, 0, 0, 0, 0, 0, 0, '255', '1', 65, 0, 0)")
        db.execute("INSERT INTO npcfaction (name, faction) VALUES ('guard bank', 'Citizens of Testspire')")
        db.commit()
        db.close()
        rows = [{"t": 0, "who": 21, "name": "Hunter Lyra", "kind": 2, "lvl": 65, "model": 3783858679, "model2": 0, "scale": 1.0,
                 "race": 1, "class": 1, "npc_flags": 0, "items": [111, 0, 222], "items_over": [0, 0, 0], "item_types": [1, 0],
                 "armor": [5, 5, 5, 5, 5, 5], "armor_tint": [3, 3, 3, 3, 3, 3], "hair": [2, 1, 3], "face": 4, "robe": 255, "robe_tint": 0}]
        walk(rows, 21, "Hunter Lyra", [(1000, 1000), (1060, 1000), (1000, 1000), (1060, 1000), (1000, 1000)], pace=4.5, pause=3.0)
        self.tracks = os.path.join(d, "tracks")
        os.makedirs(self.tracks)
        with open(os.path.join(self.tracks, "1-2-0.json.track.jsonl"), "w") as f:
            for r in rows:
                f.write(json.dumps(r) + "\n")

    def tearDown(self):
        self.dir.cleanup()

    def tool(self, *args):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--tracks", self.tracks, "--add", *args],
                           capture_output=True, text=True)
        if r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r.stdout

    def q(self, sql):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql).fetchall()
        finally:
            c.close()

    def test_a_walker_of_no_npc_here_is_made_one(self):
        self.assertIn("1 patrols (1 NPCs to make)", self.tool())
        self.tool("--apply")
        made = self.q("SELECT id, npc_name, zone, x_coord, z_coord, world, hp, modelid, primary_weapon, shield, torso, "
                      "hair_color, hair_style, face, robe_style, race, npc_level, npc_type FROM npcs WHERE npc_name = 'Hunter Lyra'")
        self.assertEqual(len(made), 1)
        i, *rest = made[0]
        # zone off the nearest NPC (Guard Bank's 77); hp the median of level 65 (Smith Far's 900); class 1 in the high byte
        self.assertEqual(rest, ["Hunter Lyra", 77, 1000, 1000, 0, 900, 3783858679, 111, 222, 5, 2, 3, 4, "255", "1", 65, 1 << 8])
        self.assertEqual(self.q(f"SELECT mode, speed FROM npcpaths WHERE npcid = {i}"), [("pingpong", "4.5")])
        self.assertEqual(self.q("SELECT name, faction, source FROM npcfaction WHERE source = 'patrol-seed'"),
                         [("hunter lyra", "Citizens of Testspire", "patrol-seed")])
        self.tool("--undo")
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE npc_name = 'Hunter Lyra'"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcpaths"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcfaction WHERE source = 'patrol-seed'"), [(0,)])


if __name__ == "__main__":
    unittest.main()
