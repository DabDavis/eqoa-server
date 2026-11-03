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
    "CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, spelldesc TEXT)",
    "CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL)",
    "CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT NOT NULL DEFAULT '', profile TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE queststeptargets (target TEXT NOT NULL)",
    "CREATE TABLE npcpaths (npcid INTEGER NOT NULL, seq INTEGER NOT NULL)",
    "CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0, "
    "water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')",
]
BEAR, VIPER, SPIRIT, LORD, MAN = 0x1001, 0x2002, 0x3003, 0x4004, 0x5005


def build(path):
    db = sqlite3.connect(path, isolation_level=None)
    for s in SCHEMA:
        db.execute(s)
    db.execute("INSERT INTO spellpattern (spellid, spellname, spelldesc) VALUES (1, 'Feral Spirit', 'Summons a spirit')")
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
        sc.cased = lambda db, names, rows=None: {n: ((n, False) if n.startswith("Lord ") else (n.lower(), True)) for n in names}

    def tearDown(self):
        sc.cased = self.cased
        self.db.close()
        self.dir.cleanup()

    def dump(self):
        return {t: list(self.db.execute(f"SELECT * FROM {t} ORDER BY rowid"))
                for t in ("npcs", "npcclass", "lootcreature", "npctemplate")}

    def test_the_kind_we_lack_is_made_and_nothing_else(self):
        census = sc.read_census([self.track])
        self.assertNotIn(200_000_001, {i for _, i in census}, "a player")
        adds, removes, report = sc.plan(self.db, census, 1)
        self.assertEqual(removes, [], "nothing removed without --swap")
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
        adds, _, _ = sc.plan(self.db, census, 1)
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
            sc.undo(self.db, 1)
        self.assertEqual(self.dump(), before)
        # the control: the undo is of the census's rows alone
        self.db.execute("INSERT INTO npcclass VALUES ('grizzly bear', 0, 'gm')")
        with self.db:
            sc.apply(self.db, adds, 1, [self.track])
            sc.undo(self.db, 1)
        self.assertEqual([r[0] for r in self.db.execute("SELECT name FROM npcclass")], ["grizzly bear"])


class SwapTest(unittest.TestCase):
    """--swap: our creatures in the world go but those a quest or a table
    keeps, and our townsfolk; the census's creatures take their places."""

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = build(os.path.join(self.dir.name, "t.db"))
        ins = ("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world, modelid, size, npc_level, npc_type, hp, "
               "hp_percent, unk_i, robe_style) VALUES (?, ?, 60, 5000, ?, 7, 1.0, 20, ?, 1, 255, '15', '255')")
        self.db.execute(ins, ("fire beetle", 7000.0, 1, 0))      # no census match: removed
        self.db.execute(ins, ("Quest Wolf", 7500.0, 1, 0))       # a quest's target: kept
        self.db.execute(ins, ("pathed rat", 8000.0, 1, 0))       # walks a path: kept
        self.db.execute(ins, ("Guard Bob", 8500.0, 1, 0x80))     # a townsman: kept
        self.db.execute(ins, ("fire beetle", 7000.0, 2, 0))      # another world: kept
        self.db.execute("INSERT INTO queststeptargets VALUES ('quest wolf')")
        rat = self.db.execute("SELECT id FROM npcs WHERE npc_name = 'pathed rat'").fetchone()[0]
        self.db.execute("INSERT INTO npcpaths VALUES (?, 0)", (rat,))
        self.track = os.path.join(self.dir.name, "run.json.track.jsonl")
        track(self.track)
        self.cased = sc.cased
        sc.cased = lambda db, names, rows=None: {n: ((n, False) if n.startswith("Lord ") else (n.lower(), True)) for n in names}

    def tearDown(self):
        sc.cased = self.cased
        self.db.close()
        self.dir.cleanup()

    def dump(self):
        return {t: list(self.db.execute(f"SELECT * FROM {t} ORDER BY rowid"))
                for t in ("npcs", "npcclass", "lootcreature", "npctemplate", "sqlite_sequence")}

    def test_swap_removes_the_unkept_and_the_census_takes_their_places(self):
        census = sc.read_census([self.track])
        adds, removes, report = sc.plan(self.db, census, 1, swap=True)
        self.assertEqual(sorted((d["npc_name"], d["x_coord"]) for d in removes),
                         [("fire beetle", 7000.0), ("grizzly bear", 5000.0), ("grizzly bear", 9000.0)])
        self.assertEqual(report["matched"], 0, "our bears are gone: the census bear is no longer ours already")
        self.assertEqual(sorted(a["row"]["npc_name"] for a in adds), ["grass viper", "grass viper", "grizzly bear"])
        # the control: without --swap the bear is ours and nothing goes
        adds2, removes2, _ = sc.plan(self.db, census, 1)
        self.assertEqual((len(removes2), sorted(a["row"]["npc_name"] for a in adds2)), (0, ["grass viper", "grass viper"]))

    def test_named_pets_and_townsfolk_are_made_when_asked(self):
        census = sc.read_census([self.track])
        adds, _, _ = sc.plan(self.db, census, 1, swap=True, with_npcs=True, with_named=True, with_pets=True)
        self.assertEqual(sorted(a["row"]["npc_name"] for a in adds),
                         ["Lord Uzzlebeck", "feral spirit", "grass viper", "grass viper", "grizzly bear", "townsman"])
        self.assertNotIn(200_000_001, {a["census_id"] for a in adds}, "never a player")

    def test_swap_removes_only_what_the_flight_could_see(self):
        self.db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world, modelid, size, npc_level, npc_type, hp, "
                        "hp_percent, unk_i, robe_style) VALUES ('fire beetle', 7000, 60, 9000, 1, 7, 1.0, 20, 0, 1, 255, '15', '255')")
        census = sc.read_census([self.track])
        sight = sc.Sight([(float(x), 80.0, 5000.0) for x in range(4500, 9600, 50)], 120, 60)
        _, removes, report = sc.plan(self.db, census, 1, swap=True, sight=sight)
        places = [(d["npc_name"], d["z_coord"]) for d in removes]
        self.assertIn(("fire beetle", 5000.0), places, "under the flight: removed")
        self.assertNotIn(("fire beetle", 9000.0), places, "4,000 off the flight: kept")
        self.assertEqual(report["unseen_kept"], 1)
        # a flight 70 over the rows is out of sight over them: nothing goes
        high = sc.Sight([(float(x), 130.0, 5000.0) for x in range(4500, 9600, 50)], 120, 60)
        _, removes3, _ = sc.plan(self.db, census, 1, swap=True, sight=high)
        self.assertEqual(removes3, [])
        # the control: with no sight given, every unkept row goes
        _, removes2, _ = sc.plan(self.db, census, 1, swap=True)
        self.assertIn(("fire beetle", 9000.0), [(d["npc_name"], d["z_coord"]) for d in removes2])

    def test_an_excluded_name_is_neither_made_nor_matched(self):
        census = sc.read_census([self.track], exclude=["GRASS VIPER"])
        self.assertNotIn("Grass Viper", {c["name"] for c in census.values()})
        adds, _, _ = sc.plan(self.db, census, 1, swap=True)
        self.assertEqual(sorted(a["row"]["npc_name"] for a in adds), ["grizzly bear"])

    def test_compact_keeps_the_census_and_the_flight(self):
        out = os.path.join(self.dir.name, "census.jsonl")
        census = sc.read_census([self.track])
        sc.compact(census, out, {"run": [(1.0, 2.0, 3.0), (100.0, 2.0, 3.0)]})
        self.assertEqual(sc.read_path([out]), [(1.0, 2.0, 3.0), (100.0, 2.0, 3.0)])
        self.assertEqual(sc.read_flights([out]), {"run": [(1.0, 2.0, 3.0), (100.0, 2.0, 3.0)]})
        back = sc.read_census([out])
        self.assertEqual(sorted(back), sorted(census), "the same creatures, epochs and all")
        self.assertEqual({k: c["mv"] for k, c in back.items()}, {k: c["mv"] for k, c in census.items()}, "how they moved")
        a1 = sc.plan(self.db, census, 1)[0]
        a2 = sc.plan(self.db, sc.read_census([out]), 1)[0]
        self.assertEqual([x["row"] for x in a1], [x["row"] for x in a2])

    def test_swap_apply_then_undo_leaves_every_table_as_it_was(self):
        before = self.dump()
        census = sc.read_census([self.track])
        adds, removes, _ = sc.plan(self.db, census, 1, swap=True, with_npcs=True, with_named=True, with_pets=True)
        with self.db:
            sc.apply(self.db, adds, 1, [self.track], removes)
        names = [r[0] for r in self.db.execute("SELECT npc_name FROM npcs WHERE world = 1 ORDER BY id")]
        self.assertNotIn("fire beetle", names)
        for kept in ("Quest Wolf", "pathed rat", "Guard Bob"):
            self.assertIn(kept, names)
        self.assertEqual(self.db.execute("SELECT COUNT(*) FROM npcs WHERE world = 2").fetchone()[0], 1)
        with self.db:
            sc.undo(self.db, 1)
        self.assertEqual(self.dump(), before)


    def test_keep_townsfolk_and_a_kept_name_stay_through_a_swap(self):
        # a city's people of ours are npc_type 0 (a city's guards): a players'
        # race's body keeps one, and so does a name given to --keep
        self.db.execute("CREATE TABLE charactermodel (sex INTEGER NOT NULL, modelid INTEGER NOT NULL PRIMARY KEY, "
                        "race INTEGER NOT NULL)")
        self.db.execute("INSERT INTO charactermodel VALUES (0, ?, 0)", (MAN,))
        ins = ("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world, modelid, size, npc_level, npc_type, hp, "
               "hp_percent, unk_i, robe_style) VALUES (?, ?, 60, 5000, 1, ?, 1.0, 35, 0, 1, 255, '15', '255')")
        self.db.execute(ins, ("Guard Tom", 8600.0, MAN))
        self.db.execute(ins, ("tin box", 8700.0, 7))
        census = sc.read_census([self.track])
        _, removes, _ = sc.plan(self.db, census, 1, swap=True, keep_townsfolk=True, keep=["Tin Box"])
        gone = {d["npc_name"] for d in removes}
        self.assertNotIn("Guard Tom", gone, "a players' race's body: kept")
        self.assertNotIn("tin box", gone, "a kept name, any case: kept")
        self.assertIn("fire beetle", gone, "a creature of ours still goes")
        # the control: without them both go
        _, removes2, _ = sc.plan(self.db, census, 1, swap=True)
        self.assertTrue({"Guard Tom", "tin box"} <= {d["npc_name"] for d in removes2})

    def test_a_tagged_census_applies_and_undoes_beside_another(self):
        before = self.dump()
        census = sc.read_census([self.track])
        adds, removes, _ = sc.plan(self.db, census, 1)
        with self.db:
            sc.apply(self.db, adds, 1, [self.track], removes)
        mid = self.dump()
        # the control: a second untagged census in the world is refused
        with self.assertRaises(SystemExit):
            with self.db:
                sc.apply(self.db, adds, 1, [self.track], removes)
        adds2, removes2, _ = sc.plan(self.db, census, 1, swap=True, keep_townsfolk=True)
        with self.db:
            sc.apply(self.db, adds2, 1, [self.track], removes2, tag="city")
        self.assertTrue(sc.has_table(self.db, "sandstorm_census_seed_w1_city"))
        with self.db:
            sc.undo(self.db, 1, "city")
        self.assertEqual(self.dump(), mid, "the tagged one undone alone")
        with self.db:
            sc.undo(self.db, 1)
        self.assertEqual(self.dump(), before)


def lines_to(path, lines):
    with open(path, "w") as f:
        for r in lines:
            f.write(json.dumps(r) + "\n")


def looks(i, name, model, t=1, flags=0, lvl=20):
    return dict(t=t, who=i, name=name, kind=130, lvl=lvl, model=model, model2=0, scale=1.0, race=0, **{"class": 0},
                npc_flags=flags, items=[0, 0, 0], items_over=[0, 0, 0], item_types=[0, 0], armor=[0] * 6,
                armor_tint=[0] * 6, hair=[0, 0, 0], face=0, robe=255, robe_tint=0)


def seen(t, i, name, x, z, pace=0.0, st=0):
    return dict(t=t, id=i, name=name, lvl=20, model=0, x=x, y=61.0, z=z, h=0.0, v=[pace, 0, 0], st=st, wf=0)


def flight(t0, x0, x1, z, step=10):
    """A flight's self lines along z from x0 to x1."""
    return [dict(t=t0 + k, self=[float(x), 80.0, float(z)]) for k, x in enumerate(range(int(x0), int(x1) + 1, step))]


class MoreTest(unittest.TestCase):
    """Restarts, two flights over one place, a spelling of our name on its
    spot, the casing's counts and --move."""

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = build(os.path.join(self.dir.name, "t.db"))
        self.cased = sc.cased
        sc.cased = lambda db, names, rows=None: {n: (n.lower(), True) for n in names}

    def tearDown(self):
        sc.cased = self.cased
        self.db.close()
        self.dir.cleanup()

    def path(self, name):
        return os.path.join(self.dir.name, name)

    def test_a_restart_makes_an_id_two_creatures(self):
        # id 20000005 a viper before the restart (t 100) and a beetle after it (t 900)
        lines_to(self.path("a.json.track.jsonl"), [looks(20000005, "Grass Viper", VIPER, t=100),
                                                   seen(100, 20000005, "Grass Viper", 6000.0, 6000.0)])
        lines_to(self.path("b.json.track.jsonl"), [looks(20000005, "Fire Beetle", BEAR, t=900),
                                                   seen(900, 20000005, "Fire Beetle", 9000.0, 9000.0)])
        files = [self.path("a.json.track.jsonl"), self.path("b.json.track.jsonl")]
        two = sc.read_census(files, restarts=[500])
        self.assertEqual(sorted(c["name"] for c in two.values()), ["Fire Beetle", "Grass Viper"])
        self.assertEqual(sorted(two), [(0, 20000005), (1, 20000005)])
        # the control: without the restart the two are one, its place between them
        one = sc.read_census(files)
        self.assertEqual(len(one), 1)

    def test_a_spawn_seen_again_under_a_new_id_on_its_spot_is_one(self):
        # one walk: Sandstorm gave its hall master a new id between two
        # looks, on the same spot; two vipers side by side in view at once are two
        lines_to(self.path("w.json.track.jsonl"),
                 [looks(30, "Hall Master", MAN, t=10), seen(10, 30, "Hall Master", 4211.0, 17207.0),
                  seen(11, 30, "Hall Master", 4211.0, 17207.0),
                  looks(31, "Hall Master", MAN, t=500), seen(500, 31, "Hall Master", 4211.5, 17207.0),
                  looks(40, "Grass Viper", VIPER, t=10), seen(10, 40, "Grass Viper", 6000.0, 6000.0),
                  looks(41, "Grass Viper", VIPER, t=10), seen(10, 41, "Grass Viper", 6002.0, 6000.0)])
        census = sc.read_census([self.path("w.json.track.jsonl")])
        drop = sc.respawns(census)
        self.assertEqual(drop, {(0, 31)}, "the later look of the master, seen less: dropped")
        # the control: the vipers were in view at one moment, so both stay
        self.assertNotIn((0, 40), drop)
        self.assertNotIn((0, 41), drop)

    def test_a_spawn_two_flights_saw_is_made_once(self):
        # flight a (t 100) and flight b (t 5000) both pass over z 6000; each sees the viper spawn
        # there under its own id; a sees a second viper 30 off that b never came near
        a = flight(100, 5800, 6200, 6000) + [looks(20000001, "Grass Viper", VIPER, t=100), seen(110, 20000001, "Grass Viper", 6000.0, 6010.0),
                                             looks(20000002, "Grass Viper", VIPER, t=100), seen(110, 20000002, "Grass Viper", 6000.0, 6040.0)]
        b = flight(5000, 5800, 6200, 6000) + [looks(20000101, "Grass Viper", VIPER, t=5000), seen(5010, 20000101, "Grass Viper", 6003.0, 6012.0)]
        lines_to(self.path("a.json.track.jsonl"), a)
        lines_to(self.path("b.json.track.jsonl"), b)
        files = [self.path("a.json.track.jsonl"), self.path("b.json.track.jsonl")]
        census = sc.read_census(files)
        drop, by = sc.dedupe(census, sc.read_flights(files))
        self.assertEqual(drop, {(0, 20000101)}, "b's viper is a's, seen again; a saw both of the pair")
        self.assertEqual(dict(by), {"b": 1})
        # the control: one flight seeing two of a kind on one spot keeps both
        lines_to(self.path("c.json.track.jsonl"), flight(100, 5800, 6200, 6000) + [
            looks(1, "Grass Viper", VIPER), seen(110, 1, "Grass Viper", 6000.0, 6010.0),
            looks(2, "Grass Viper", VIPER), seen(110, 2, "Grass Viper", 6001.0, 6011.0)])
        c = [self.path("c.json.track.jsonl")]
        self.assertEqual(sc.dedupe(sc.read_census(c), sc.read_flights(c))[0], set())
        # and a flight that never passed near the other's creature does not drop it
        far = flight(5000, 9800, 10200, 9000) + [looks(20000101, "Grass Viper", VIPER, t=5000),
                                                  seen(5010, 20000101, "Grass Viper", 6003.0, 6012.0)]
        lines_to(self.path("d.json.track.jsonl"), far)
        files = [self.path("a.json.track.jsonl"), self.path("d.json.track.jsonl")]
        self.assertEqual(sc.dedupe(sc.read_census(files), sc.read_flights(files))[0], set())

    def test_our_row_spelled_otherwise_on_its_spot_is_ours(self):
        ins = ("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world, modelid, size, npc_level, npc_type, hp, "
               "hp_percent, unk_i, robe_style) VALUES (?, ?, 60, ?, 1, 7, 1.0, 15, 0, 1, 255, '15', '255')")
        self.db.execute(ins, ("orge thug", 8861.0, 5548.0))
        lines_to(self.path("a.json.track.jsonl"), [looks(5, "Ogre Thug", BEAR), seen(1, 5, "Ogre Thug", 8862.0, 5549.0),
                                                   looks(6, "Ogre Thug", BEAR), seen(1, 6, "Ogre Thug", 8962.0, 5549.0)])
        census = sc.read_census([self.path("a.json.track.jsonl")])
        adds, _, report = sc.plan(self.db, census, 1)
        self.assertEqual(report["matched"], 1, "the thug on our orge thug's spot is ours")
        self.assertEqual([round(a["row"]["x_coord"]) for a in adds], [8962], "the one 100 off is made")
        self.assertTrue(sc.spelled_alike("seamstress kella", "seamsteress kella"))
        self.assertFalse(sc.spelled_alike("kerran priest", "kerran soldier"))
        self.assertFalse(sc.spelled_alike("rat", "bat"), "short names are not guessed at")

    def test_the_casing_counts_the_census(self):
        sc.cased = self.cased  # the real rules, over this database's evidence
        rows = {"toxic mist": [3, 0, 0]}
        out = sc.cased(self.db, ["Toxic Mist", "Ogre Thug", "ogre thug"], rows)
        self.assertEqual(out["Toxic Mist"][0], "toxic mist", "seen three times: a kind, lowercase")
        self.assertEqual(out["Ogre Thug"][0], out["ogre thug"][0], "one spelling a name")
        self.assertEqual(out["Ogre Thug"][0], "ogre thug")
        # the control: once, an unknown head keeps Sandstorm's capitals
        self.assertEqual(sc.cased(self.db, ["Toxic Mist"], {"toxic mist": [1, 0, 0]})["Toxic Mist"][0], "Toxic Mist")
        # one word seen three times is a kind's, once someone's
        self.assertEqual(sc.cased(self.db, ["Devourer"], {"devourer": [6, 0, 0]})["Devourer"][0], "devourer")
        self.assertEqual(sc.cased(self.db, ["Naris"], {"naris": [1, 0, 0]})["Naris"][0], "Naris")

    def test_move_rows_from_how_sandstorm_moved_them(self):
        lines = flight(1, 0, 100, 0)
        def watch(i, name, model, pace, t0=1, secs=25, flags=0, st=0):
            lines.append(looks(i, name, model, t=t0, flags=flags))
            lines.extend(seen(t0 + 1000 * s, i, name, 50.0, 50.0, pace=(pace if s % 4 == 1 else 0.0), st=st) for s in range(secs))
        watch(1, "Tae Ew Soldier", 0x7001, 4.9)       # walked: not wildlife, so a row
        watch(2, "Tae Ew Soldier", 0x7001, 4.9)
        watch(3, "Kappa Guard", 0x7002, 0.0)          # stood: as its kind (it stands): no row
        watch(17, "Ankextet Guard", 0x7009, 4.9)      # one walked, one stood: as its kind, no row
        watch(18, "Ankextet Guard", 0x7009, 0.0)
        watch(4, "Grass Viper", VIPER, 0.0)           # wildlife that stood, twice: a row to stand
        watch(5, "Grass Viper", VIPER, 0.0)
        watch(6, "Tiger", 0x7003, 0.0)                # wildlife that stood once: left to wander
        watch(7, "Town Crier", 0x7004, 4.9, flags=0x80)  # a person: never a row
        watch(8, "Ankexfen Knight", 0x7005, 4.9)      # the Ankexfen family: five walk...
        for i in range(9, 13):
            watch(i, "Ankexfen Priest", 0x7005, 4.9)
        watch(13, "Ankexfen Scout", 0x7005, 4.9, secs=2)  # ...and a scout glimpsed: walks as they do
        watch(14, "Imprisoned Soul", 0x7006, 12.0, st=3)  # a chase: no evidence either way
        watch(15, "Devourer", 0x7007, 0.0, secs=3)    # said outright (MOVE_SAID)
        watch(16, "Fire Beetle", 0x7008, 4.9)         # walked, but npcmove has a row: kept
        self.db.execute("INSERT INTO npcmove (name, roam, source) VALUES ('fire beetle', 0, 'gm')")
        lines_to(self.path("m.json.track.jsonl"), lines)
        census = sc.read_census([self.path("m.json.track.jsonl")])
        rules = sc.move_rules(self.db, census)
        self.assertEqual({k: r for k, (r, _) in rules.items()},
                         {"tae ew soldier": 1, "grass viper": 0, "ankexfen knight": 1, "ankexfen priest": 1, "ankexfen scout": 1,
                          "devourer": 1})
        before = list(self.db.execute("SELECT * FROM npcmove ORDER BY rowid"))
        with self.db:
            sc.apply_move(self.db, rules)
        self.assertEqual(self.db.execute("SELECT roam, source FROM npcmove WHERE name = 'tae ew soldier'").fetchone(),
                         (1, sc.SOURCE))
        with self.assertRaises(SystemExit):
            sc.apply_move(self.db, rules)
        with self.db:
            self.assertEqual(sc.undo_move(self.db), len(rules))
        self.assertEqual(list(self.db.execute("SELECT * FROM npcmove ORDER BY rowid")), before)

    def test_a_tagged_census_moves_and_undoes_beside_another(self):
        def census_of(name, kinds):
            lines = flight(1, 0, 100, 0)
            for i, (kind, model, pace) in enumerate(kinds, 1):
                lines.append(looks(i, kind, model))
                lines.extend(seen(1 + 1000 * s, i, kind, 50.0, 50.0, pace=(pace if s % 4 == 1 else 0.0)) for s in range(25))
            lines_to(self.path(name), lines)
            return sc.read_census([self.path(name)])
        rows = lambda: list(self.db.execute("SELECT * FROM npcmove ORDER BY rowid"))
        before = rows()
        first = sc.move_rules(self.db, census_of("w.json.track.jsonl", [("Quillek Runner", 0x7101, 4.9)] * 2))
        with self.db:
            sc.apply_move(self.db, first)
        mid = rows()
        # a city's census later: a kind of its own, and the first's kind again (npcmove has it: left alone)
        city = sc.move_rules(self.db, census_of("c.json.track.jsonl", [("Zorvan Drudge", 0x7102, 4.9)] * 2
                                                + [("Quillek Runner", 0x7101, 0.0)] * 2))
        self.assertEqual({k: r for k, (r, _) in city.items()}, {"zorvan drudge": 1})
        with self.assertRaises(SystemExit):
            sc.apply_move(self.db, city)  # the control: untagged, the first's log refuses it
        with self.db:
            sc.apply_move(self.db, city, "city")
        self.assertTrue(sc.has_table(self.db, "sandstorm_census_move_city"))
        with self.assertRaises(SystemExit):
            sc.apply_move(self.db, city, "city")
        with self.db:
            self.assertEqual(sc.undo_move(self.db, "city"), 1)
        self.assertEqual(rows(), mid, "the tagged census's movement undone alone")
        with self.db:
            self.assertEqual(sc.undo_move(self.db), 1)
        self.assertEqual(rows(), before)


if __name__ == "__main__":
    unittest.main()
