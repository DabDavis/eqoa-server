#!/usr/bin/env python3
"""tools/seed_sandstorm_census.py against a made-up census: a track file of a
creature of ours, two of a kind we lack, a pet, a named one, a townsman and a
player, over a database of a few rows. No game data.

    python3 tools/seed_sandstorm_census_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_sandstorm_census as sc  # noqa: E402

SCHEMA = [
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, unk_a INTEGER, unk_b INTEGER, "
    "x_coord REAL, y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, hp_percent INTEGER, "
    "modelid INTEGER, size REAL, unk_d INTEGER, unk_f INTEGER, unk_g INTEGER, primary_weapon INTEGER, "
    "secondary_weapon INTEGER, shield INTEGER, unk_h INTEGER, torso INTEGER, forearm INTEGER, gloves INTEGER, legs INTEGER, "
    "feet INTEGER, head INTEGER, hair_color INTEGER, hair_length INTEGER, hair_style INTEGER, face INTEGER, robe_style TEXT, "
    "race TEXT, unk_j INTEGER, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER, unk_k INTEGER, unk_i TEXT, "
    "pd_icon INTEGER, unk_m INTEGER, unk_n INTEGER, unk_o INTEGER)",
    "CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT)",
    "CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL)",
    "CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT NOT NULL DEFAULT '', profile TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
]
BEAR, VIPER, SPIRIT, LORD, MAN = 0x1001, 0x2002, 0x3003, 0x4004, 0x5005


def build(path):
    db = sqlite3.connect(path, isolation_level=None)
    for s in SCHEMA:
        db.execute(s)
    db.execute("INSERT INTO spellpattern VALUES (1, 'Feral Spirit')")
    # ours: a bear where the census sees one, another far off
    for x in (5000.0, 9000.0):
        db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world, modelid, size, npc_level, npc_type, hp, "
                   "hp_percent, unk_i, robe_style) VALUES ('grizzly bear', ?, 60, 5000, 1, ?, 1.0, 20, 0, 1, 255, '15', '255')",
                   (x, BEAR))
    return db


def track(path):
    lines = []

    def who(i, name, model, flags=0, lvl=20, cls=0, scale=1.0):
        lines.append(dict(t=1, who=i, name=name, kind=130, lvl=lvl, model=model, model2=0, scale=scale, race=0, **{"class": cls},
                          npc_flags=flags, items=[7, 0, 0], items_over=[0, 0, 0], item_types=[1, 0], armor=[1, 2, 3, 4, 5, 6],
                          armor_tint=[0] * 6, hair=[1, 2, 3], face=4, robe=255, robe_tint=0))

    def at(t, i, name, x, z, lvl=20, h=1.5708):
        lines.append(dict(t=t, id=i, name=name, lvl=lvl, model=0, x=x, y=61.0, z=z, h=h, v=[0, 0, 0], st=0, wf=0))

    who(10, "Grizzly Bear", BEAR)
    who(11, "Grass Viper", VIPER, lvl=12, scale=0.75)
    who(12, "Grass Viper", VIPER, lvl=13, scale=0.75)
    who(13, "Feral Spirit", SPIRIT)
    who(14, "Lord Uzzlebeck", LORD, lvl=40)
    who(15, "Townsman", MAN, flags=0x80)
    who(200_000_001, "Somebody", MAN)
    for t in (1, 2, 3):
        at(t, 10, "Grizzly Bear", 5040.0 + t, 5010.0)
        at(t, 11, "Grass Viper", 6000.0 + 2 * t, 6000.0, lvl=12)
        at(t, 12, "Grass Viper", 6300.0, 6000.0 - t, lvl=13)
        at(t, 13, "Feral Spirit", 6000.0, 6100.0)
        at(t, 14, "Lord Uzzlebeck", 7000.0, 7000.0, lvl=40)
        at(t, 15, "Townsman", 8000.0, 8000.0)
        at(t, 200_000_001, "Somebody", 5000.0, 5000.0)
    with open(path, "w") as f:
        for r in lines:
            f.write(json.dumps(r) + "\n")
        f.write('{"t": 4, "id": 11, "na')  # a line still being written


class CensusTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = build(os.path.join(self.dir.name, "t.db"))
        self.track = os.path.join(self.dir.name, "run.json.track.jsonl")
        track(self.track)
        # the casing rules want a whole database's evidence: here, a kind's
        # name lower case, a lord's someone's
        self.cased = sc.cased
        sc.cased = lambda db, names: {n: ((n, False) if n.startswith("Lord ") else (n.lower(), True)) for n in names}

    def tearDown(self):
        sc.cased = self.cased
        self.db.close()
        self.dir.cleanup()

    def dump(self):
        return {t: list(self.db.execute(f"SELECT * FROM {t} ORDER BY rowid"))
                for t in ("npcs", "npcclass", "lootcreature", "npctemplate")}

    def test_the_kind_we_lack_is_made_and_nothing_else(self):
        census = sc.read_census([self.track])
        self.assertNotIn(200_000_001, census, "a player")
        adds, report = sc.plan(self.db, census, 1)
        self.assertEqual(report["matched"], 1, "the bear within MATCH of ours")
        names = sorted(a["row"]["npc_name"] for a in adds)
        self.assertEqual(names, ["grass viper", "grass viper"])
        self.assertEqual(report["pets"], [("Feral Spirit", 1)])
        self.assertEqual([n for n, _, _ in report["named"]], ["Lord Uzzlebeck"])
        self.assertEqual(report["people"], [("Townsman", 1)])
        v = min(adds, key=lambda a: a["census_id"])["row"]
        self.assertEqual((v["modelid"], v["size"], v["npc_level"], v["primary_weapon"], v["torso"], v["head"], v["hair_style"],
                          v["face"], v["world"], v["npc_id"]), (VIPER, 0.75, 12, 7, 1, 6, 3, 4, 1, 0))
        self.assertAlmostEqual(v["x_coord"], 6004.0)
        self.assertEqual(v["facing"], 64, "a quarter turn")
        self.assertEqual(v["unk_i"], "15", "the usual values for what the census does not see")

    def test_apply_then_undo_leaves_every_table_as_it_was(self):
        before = self.dump()
        census = sc.read_census([self.track])
        adds, _ = sc.plan(self.db, census, 1)
        with self.db:
            sc.apply(self.db, adds, 1, [self.track])
        after = self.dump()
        self.assertEqual(len(after["npcs"]), len(before["npcs"]) + 2)
        self.assertEqual([r[:2] for r in after["npcclass"]], [("grass viper", 0)])
        self.assertEqual([r[:2] for r in after["lootcreature"]], [("grass viper", "rathe mountains")])
        self.assertEqual([r[:2] for r in after["npctemplate"]], [("grass viper", "roamer")])
        with self.assertRaises(SystemExit):
            sc.apply(self.db, adds, 1, [self.track])
        with self.db:
            sc.undo(self.db)
        self.assertEqual(self.dump(), before)
        # the control: the undo is of the census's rows alone
        self.db.execute("INSERT INTO npcclass VALUES ('grizzly bear', 0, 'gm')")
        with self.db:
            sc.apply(self.db, adds, 1, [self.track])
            sc.undo(self.db)
        self.assertEqual([r[0] for r in self.db.execute("SELECT name FROM npcclass")], ["grizzly bear"])


if __name__ == "__main__":
    unittest.main()
