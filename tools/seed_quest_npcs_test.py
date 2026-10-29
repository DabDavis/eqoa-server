#!/usr/bin/env python3
"""tools/seed_quest_npcs.py against a made-up world: seed_wildlife_test's
ground (a plain with a hill and a hollow) under a few NPCs, sightings and
map points. No game data.

    python3 tools/seed_quest_npcs_test.py
"""
import json
import math
import os
import sqlite3
import struct
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_quest_npcs as sq  # noqa: E402
import seed_wildlife_test as wt  # noqa: E402

HUMAN_M, HUMAN_F, BEAR, OTHER = 0x0A01, 0x0A02, 0x0B01, 0x0C01

SCHEMA = [
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, "
    "y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, modelid INTEGER, "
    "size REAL, race TEXT, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER, torso INTEGER)",
    "CREATE TABLE charactermodel (sex INTEGER, modelid INTEGER PRIMARY KEY, race INTEGER)",
    "CREATE TABLE npcs_ff_import (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_level INTEGER, "
    "world INTEGER, x_coord REAL, y_coord REAL, z_coord REAL, pos_suspect INTEGER NOT NULL DEFAULT 0)",
]


def write_env(path, x0, top):
    """A water file (seed_pod.py's Env reads it): the one zone is water east
    of x0 up to the height top, dry everywhere else."""
    leaf = 0x40000000
    nodes = [(1.0, 0.0, 0.0, -x0, 1, leaf | 0), (0.0, -1.0, 0.0, top, leaf | 1, leaf | 0)]
    out = b"EQEV" + struct.pack("<I", 1) + struct.pack("<2I", 0, 1) + struct.pack("<I", 0) + struct.pack("<I", 1)
    out += struct.pack("<2I", len(nodes), 2) + b"".join(struct.pack("<4f2I", *n) for n in nodes)
    out += struct.pack("<2I", 0, 1)
    with open(path, "wb") as f:
        f.write(out)


def npc(db, name, x, z, model, level=10, ntype=0, world=0, torso=3):
    db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, hp, modelid, size, race, "
               "npc_level, npc_type, npc_id, torso) VALUES (?, 7, ?, ?, ?, 0, ?, 100, ?, 1.0, '0', ?, ?, 99, ?)",
               (name, x, wt.height(x, z), z, world, model, level, ntype, torso))


class Fixture(unittest.TestCase):
    """The made-up world: its database, ground, map and quest folder."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(prefix="questnpcs-")
        self.ground = os.path.join(self.dir, "ground")
        os.mkdir(self.ground)
        wt.write_ground(os.path.join(self.ground, "0.ground"))
        self.db_path = os.path.join(self.dir, "eqoa.db")
        db = sqlite3.connect(self.db_path)
        for s in SCHEMA:
            db.execute(s)
        db.executemany("INSERT INTO charactermodel VALUES (?, ?, ?)", [(0, HUMAN_M, 0), (1, HUMAN_F, 0)])
        # townsfolk the map's towns are fitted to, a bear to copy, a merchant (0x81) to copy the body of
        npc(db, "Mayor Alpha", 10000, 4000, HUMAN_M, ntype=128)
        npc(db, "Mayor Beta", 11500, 5500, HUMAN_M, ntype=128)
        npc(db, "Mayor Gamma", 10800, 4600, HUMAN_F, ntype=128)
        npc(db, "Merchant Delta", 10300, 4300, HUMAN_M, ntype=0x81, torso=7)
        npc(db, "brown bear", 11200, 4200, BEAR)
        npc(db, "brown bear", 11240, 4260, BEAR)
        npc(db, "Guard Kept", 10600, 4100, HUMAN_M, ntype=128)
        db.executemany("INSERT INTO npcs_ff_import (npc_name, npc_level, world, x_coord, y_coord, z_coord, pos_suspect) "
                       "VALUES (?, ?, 0, ?, ?, ?, ?)",
                       [("Sighted One", 14, 10900.0, wt.height(10900, 4900), 4900.0, 0),
                        ("Suspect One", 9, 18000.0, 24.0, 32000.0, 1)])
        db.commit()
        db.close()
        # the map: x_game = 2 * mx + 1000, z_game = 2 * my + 2000 for the fit's three towns
        self.data = os.path.join(self.dir, "EQOA-Data")
        os.makedirs(os.path.join(self.data, "Map"))
        towns = [dict(name="Mayor Alpha", x=4500, y=1000), dict(name="Mayor Beta", x=5250, y=1750),
                 dict(name="Mayor Gamma", x=4900, y=1300)]
        pois = [dict(name="Map Thing (12)", x=5000, y=1200, category="quest", questGuideIds=["1-20/test/warrior"])]
        with open(os.path.join(self.data, "Map", "zones.json"), "w") as f:
            json.dump(dict(towns=towns, zones=[], pois=pois), f)
        self.quests = os.path.join(self.dir, "quests")
        os.mkdir(self.quests)

    def connect(self, ro=False):
        if ro:
            return sqlite3.connect(f"file:{self.db_path}?mode=ro", uri=True, isolation_level=None)
        return sqlite3.connect(self.db_path, isolation_level=None)

    def plan(self, specs):
        return sq.plan(self.connect(ro=True), self.ground, self.data, self.quests, specs=specs)


class World(Fixture):
    def test_a_sighting_places_a_named_npc_on_it(self):
        p = self.plan([sq.S("Sighted One", sq.FOE, ("kind", "bear"), ("ff",)),
                       sq.S("Suspect One", sq.FOE, ("kind", "bear"), ("ff",))])
        self.assertEqual(len(p["rows"]), 1, p["left"])
        r = p["rows"][0]["row"]
        self.assertEqual((r["npc_name"], r["x_coord"], r["z_coord"], r["npc_level"]), ("Sighted One", 10900.0, 4900.0, 14))
        self.assertAlmostEqual(r["y_coord"], wt.height(10900, 4900), places=1)
        self.assertEqual((r["modelid"], r["npc_type"], r["npc_id"], r["world"]), (BEAR, 0, 0, 0), "a bear's body, a foe")
        # the control: a suspect sighting places nothing
        self.assertEqual(p["left"], [("Suspect One", "no FrontiersForge sighting")])

    def test_a_talker_takes_a_race_body_and_its_look_but_never_its_service(self):
        p = self.plan([sq.S("New Talker", sq.TALKER, ("race", "human", sq.MALE), ("near", "Merchant Delta", 20, 0), level=30)])
        r = p["rows"][0]["row"]
        self.assertEqual(r["modelid"], HUMAN_M)
        self.assertEqual((r["npc_type"], r["npc_id"]), (128, 0), "a talker: 0x80, no merchant's bit, no list")
        d = math.hypot(r["x_coord"] - 10300, r["z_coord"] - 4300)
        self.assertAlmostEqual(d, 20, delta=0.01)
        self.assertEqual(r["npc_level"], 30)
        # the control: a kind never takes a players' body, a race never a creature's
        p = self.plan([sq.S("Bad Bear", sq.FOE, ("race", "human", sq.FEMALE), ("near", "brown bear", 10, 0))])
        self.assertEqual(p["rows"][0]["row"]["modelid"], HUMAN_F)

    def test_a_map_point_lands_by_the_fit(self):
        p = self.plan([sq.S("Map Thing", sq.FOE, ("kind", "bear"), ("poi", "tunaria", "Map Thing"))])
        (a, b, c, d), kept, med = p["fit"]
        self.assertAlmostEqual(a, 2.0, places=3)
        self.assertAlmostEqual(d, 2000.0, delta=1.0)
        r = p["rows"][0]["row"]
        self.assertEqual((round(r["x_coord"]), round(r["z_coord"]), r["npc_level"]), (11000, 4400, 12),
                         "2 x 5000 + 1000, 2 x 1200 + 2000, the point's (12)")

    def test_a_kind_gets_its_spawns_apart_round_the_place(self):
        p = self.plan([sq.S("gnat", sq.FOE, ("kind", "bear"), ("at", 0, 10400.0, 5000.0), count=5, level=3)])
        self.assertEqual(len(p["rows"]), 5)
        pts = [(r["row"]["x_coord"], r["row"]["z_coord"]) for r in p["rows"]]
        for i, a in enumerate(pts):
            self.assertLessEqual(math.hypot(a[0] - 10400, a[1] - 5000), sq.SPREAD[1] + 0.01)
            for b in pts[i + 1:]:
                self.assertGreaterEqual(math.hypot(a[0] - b[0], a[1] - b[1]), sq.SPACING)

    def test_the_water_keeps_land_and_fish_apart_and_a_deck_keeps_its_height(self):
        write_env(os.path.join(self.ground, "0.env"), 11500.0, wt.PLAIN + 2.0)
        db = sqlite3.connect(self.db_path)
        db.execute("INSERT INTO npcs_ff_import (npc_name, npc_level, world, x_coord, y_coord, z_coord) "
                   "VALUES ('On Deck', 5, 0, 11700.0, ?, 5100.0)", (wt.PLAIN + 5.0,))
        db.commit()
        db.close()
        p = self.plan([sq.S("Wader", sq.FOE, ("kind", "bear"), ("at", 0, 11600.0, 5000.0)),
                       sq.S("Swimmer", sq.FOE, ("kind", "bear"), ("at", 0, 11600.0, 5000.0), water=sq.WATER),
                       sq.S("Walker", sq.FOE, ("kind", "bear"), ("at", 0, 11400.0, 5000.0), water=sq.WATER),
                       sq.S("On Deck", sq.TALKER, ("race", "human", sq.MALE), ("ff",))])
        got = {r["name"]: r["row"] for r in p["rows"]}
        self.assertEqual(sorted(got), ["On Deck", "Swimmer"], p["left"])
        self.assertGreaterEqual(got["Swimmer"]["x_coord"], 11500.0)
        self.assertIn(("Wader", "no dry floor round (11600, 5000) in world 0"), p["left"])
        self.assertIn(("Walker", "no floor in the water round (11400, 5000) in world 0"), p["left"])
        # seen above every floor, out of the water: on whatever it stood on
        self.assertEqual((got["On Deck"]["x_coord"], got["On Deck"]["y_coord"]), (11700.0, wt.PLAIN + 5.0))
        # the control: without the water file the wader stands in the pond
        os.remove(os.path.join(self.ground, "0.env"))
        p = self.plan([sq.S("Wader", sq.FOE, ("kind", "bear"), ("at", 0, 11600.0, 5000.0))])
        self.assertEqual(len(p["rows"]), 1, p["left"])

    def test_no_floor_no_npc(self):
        p = self.plan([sq.S("Lost", sq.FOE, ("kind", "bear"), ("at", 0, 30000.0, 30000.0))])
        self.assertEqual(p["rows"], [])
        self.assertTrue(p["left"][0][1].startswith("no floor"), p["left"])

    def test_apply_and_undo_are_exact_and_a_gm_change_is_kept(self):
        old = sq.ATTACKABLE
        sq.ATTACKABLE = ["Guard Kept"]
        try:
            db = self.connect()
            before = _digest(db)
            p = sq.plan(db, self.ground, self.data, self.quests,
                        specs=[sq.S("Sighted One", sq.FOE, ("kind", "bear"), ("ff",), count=3)])
            sq.apply(db, p)
            self.assertEqual(db.execute("SELECT count(*) FROM npcs WHERE npc_name = 'Sighted One'").fetchone()[0], 3)
            self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE npc_name = 'Guard Kept'").fetchone()[0], 0)
            with self.assertRaises(SystemExit):
                sq.apply(db, p)   # one seed at a time
            n, kept = sq.undo(db)
            self.assertEqual((n, kept), (3, []))
            self.assertEqual(_digest(db), before, "undo gives every row back as it was")
            self.assertFalse(sq.seeded(db))
            # a GM's change of the type since the seed is left at undo
            p = sq.plan(db, self.ground, self.data, self.quests, specs=[])
            sq.apply(db, p)
            db.execute("UPDATE npcs SET npc_type = 136 WHERE npc_name = 'Guard Kept'")
            n, kept = sq.undo(db)
            self.assertEqual(len(kept), 1)
            self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE npc_name = 'Guard Kept'").fetchone()[0], 136)
        finally:
            sq.ATTACKABLE = old


def _digest(db):
    return [tuple(r) for r in db.execute("SELECT * FROM npcs ORDER BY id")] + \
        [tuple(r) for r in db.execute("SELECT * FROM sqlite_sequence ORDER BY name")]


class Specs(unittest.TestCase):
    def test_every_spec_is_well_formed(self):
        names = set()
        for s in sq.SPECS:
            # a name may stand in two places (a creature by two towns), an entry not twice
            self.assertNotIn((sq.fold(s["name"]), s["place"]), names, s["name"])
            names.add((sq.fold(s["name"]), s["place"]))
            self.assertIn(s["water"], (sq.LAND, sq.WATER, sq.BOTH))
            self.assertIn(s["role"], (sq.FOE, sq.TALKER))
            kind = s["body"][0]
            self.assertIn(kind, ("race", "kind", "npc"))
            if kind == "race":
                self.assertIn(s["body"][1], sq.RACE)
            if kind == "kind":
                self.assertIn(s["body"][1], sq.KIND)
            self.assertIn(s["place"][0], ("ff", "poi", "near", "at"))
            self.assertGreaterEqual(s["count"], 1)
        # a "near" anchor that is one of the seed's own comes before it
        order = [sq.fold(s["name"]) for s in sq.SPECS]
        for i, s in enumerate(sq.SPECS):
            if s["place"][0] == "near" and sq.fold(s["place"][1]) in order:
                self.assertLess(order.index(sq.fold(s["place"][1])), i, s["name"])



# the pre-sets tool's log (verbatim), as eqoaclassic's 1-20 seed made it
OLD_LOG_DDL = [
    "CREATE TABLE quest_npc_seed_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
    "CREATE TABLE quest_npc_seed_rows (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, "
    "source TEXT NOT NULL, template_row INTEGER)",
    "CREATE TABLE quest_npc_seed_types (npc_row INTEGER PRIMARY KEY, old_type INTEGER NOT NULL, "
    "new_type INTEGER NOT NULL)",
]


class Sets(Fixture):
    """Seed sets: a spec file read as S() writes one, sets in at once and taken
    back each alone, a log from before the sets the set 1-20's, and the
    command line's set rules."""

    def spec_file(self, name, npcs, attackable=()):
        path = os.path.join(self.dir, f"{name}.json")
        with open(path, "w") as f:
            json.dump(dict(set=name, npcs=npcs, attackable=list(attackable)), f)
        return path

    def old_seed(self, db, p):
        """p written as the pre-sets tool wrote a seed: its rows and log."""
        cols = [c for c in p["cols"] if c != "id"]
        for stmt in OLD_LOG_DDL:
            db.execute(stmt)
        seq = sq.npc_seq(db)
        for r in p["rows"]:
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r["row"].get(c) for c in cols])
            db.execute("INSERT INTO quest_npc_seed_rows VALUES (?, ?, ?, ?)", (cur.lastrowid, r["name"], r["source"], r["template"]))
        for t in p["types"]:
            db.execute("UPDATE npcs SET npc_type = ? WHERE id = ?", (t["new"], t["id"]))
            db.execute("INSERT INTO quest_npc_seed_types VALUES (?, ?, ?)", (t["id"], t["old"], t["new"]))
        db.executemany("INSERT INTO quest_npc_seed_meta VALUES (?, ?)",
                       [("rows", str(len(p["rows"]))), ("seq_before", "" if seq is None else str(seq)), ("when", "then")])

    def test_a_spec_file_reads_as_S_writes_one(self):
        good = dict(name="Moor Hermit", role="talker", body=["race", "human", 1], place=["near", "Mayor Alpha", 20, 0],
                    level=30, note="by the mayor")
        path = self.spec_file("qa", [good, dict(name="Bog Bear", role="foe", body=["kind", "bear"], place=["ff", "low"],
                                                count=2, water="both")], attackable=["Guard Kept"])
        sets, errs = sq.load_sets([path])
        self.assertEqual(errs, [])
        self.assertEqual(list(sets), [sq.BUILTIN, "qa"], "the built-in set first")
        self.assertEqual(sets["qa"]["specs"][0],
                         sq.S("Moor Hermit", sq.TALKER, ("race", "human", sq.FEMALE), ("near", "Mayor Alpha", 20, 0), level=30,
                              note="by the mayor"))
        self.assertEqual(sets["qa"]["specs"][1]["place"], ("ff", "low"))
        self.assertEqual(sets["qa"]["attackable"], ["Guard Kept"])
        bad = self.spec_file("qb", [dict(name="X", role="boss", body=["race", "orc", 0], place=["poi", "faydwer", "Y"],
                                         count=0, water="lava"), dict(role="foe")])
        _, errs = sq.load_sets([bad, self.spec_file("qa2", [])])
        text = "\n".join(errs)
        for want in ("role 'boss'", "body ['race', 'orc', 0]", "place ['poi', 'faydwer', 'Y']", "count 0", "water 'lava'", "no name"):
            self.assertIn(want, text)
        dup = os.path.join(self.dir, "again.json")
        with open(dup, "w") as f:
            json.dump(dict(set="qa", npcs=[]), f)
        _, errs = sq.load_sets([path, dup])
        self.assertTrue(any("set 'qa' is" in e for e in errs), errs)

    def test_sets_are_in_together_and_taken_back_alone(self):
        db = self.connect()
        before = _digest(db)
        a = sq.plan(db, self.ground, self.data, self.quests, specs=[sq.S("Sighted One", sq.FOE, ("kind", "bear"), ("ff",), count=2)],
                    attackable=[])
        sq.apply(db, a, "qa")
        mid = _digest(db)
        b = sq.plan(db, self.ground, self.data, self.quests,
                    specs=[sq.S("Hut Keeper", sq.TALKER, ("race", "human", sq.MALE), ("near", "Sighted One", 15, 0))],
                    attackable=["Guard Kept"])
        sq.apply(db, b, "qb")
        self.assertEqual(sq.sets_in(db), {"qa": 2, "qb": 1}, "a set applied while another is in")
        with self.assertRaises(SystemExit):
            sq.apply(db, b, "qb")   # a set once
        self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE npc_name = 'Guard Kept'").fetchone()[0], 0)
        self.assertEqual(sq.undo(db, "qb"), (1, []))
        self.assertEqual(_digest(db), mid, "the later set out: the earlier one as it went in, the counter back")
        sq.apply(db, b, "qb")
        self.assertEqual(sq.undo(db, "qa"), (2, []), "the earlier set out first")
        self.assertEqual(sq.sets_in(db), {"qb": 1})
        sq.undo(db, "qb")
        self.assertFalse(sq.seeded(db), "no set in: no logs")
        self.assertEqual(_digest(db), before, "every row as it was, the counter too")

    def test_a_log_from_before_the_sets_is_the_set_1_20s(self):
        db = self.connect()
        before = _digest(db)
        old = sq.plan(db, self.ground, self.data, self.quests, specs=[sq.S("Sighted One", sq.FOE, ("kind", "bear"), ("ff",))],
                      attackable=["Guard Kept"])
        db.execute("BEGIN")
        self.old_seed(db, old)
        db.execute("COMMIT")
        self.assertEqual(sq.sets_in(db), {sq.BUILTIN: 1}, "read as it is, before any change")
        new = sq.plan(db, self.ground, self.data, self.quests, specs=[sq.S("Map Thing", sq.FOE, ("kind", "bear"),
                                                                           ("poi", "tunaria", "Map Thing"))], attackable=[])
        sq.apply(db, new, "qa")
        self.assertEqual(sq.sets_in(db), {sq.BUILTIN: 1, "qa": 1})
        self.assertEqual(sorted(sq.log_columns(db, "quest_npc_seed_meta")), ["key", "seedset", "value"], "migrated in place")
        self.assertEqual(sq.undo(db, sq.BUILTIN), (1, []), "the old seed undone as its set")
        self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE npc_name = 'Guard Kept'").fetchone()[0], 128)
        sq.undo(db, "qa")
        self.assertEqual(_digest(db), before, "the old seed and the set out: as before both, the counter too")

    def test_a_sets_peaceful_creatures_get_npcaggro_rows_undone_with_it(self):
        db = self.connect()
        db.execute(sq.NPCAGGRO_DDL)
        db.execute("INSERT INTO npcaggro VALUES ('Mayor Beta', 1, 40, 'a GM said so', 'gm')")
        db.execute("INSERT INTO npcaggro VALUES ('Mayor Gamma', 0, 0, 'already', 'seed')")
        before = [tuple(r) for r in db.execute("SELECT * FROM npcaggro ORDER BY npc_name")]
        p = sq.plan(db, self.ground, self.data, self.quests, specs=[], attackable=[],
                    peaceful=["BROWN BEAR", "Mayor Beta", "Mayor Gamma", "Nobody"])
        self.assertEqual([a["name"] for a in p["aggro"]], ["brown bear", "Mayor Beta"],
                         "as npcs spells it; one peaceful already is left as it is")
        self.assertIn(("Nobody", "no NPC of the name to make peaceful"), p["left"])
        sq.apply(db, p, "qa")
        rows = {r[0]: r for r in db.execute("SELECT * FROM npcaggro")}
        self.assertEqual(rows["brown bear"][1:3], (0, 0.0))
        self.assertEqual((rows["Mayor Beta"][1], rows["Mayor Beta"][4]), (0, "questnpcs:qa"))
        self.assertEqual(sq.undo(db, "qa"), (0, []))
        self.assertEqual([tuple(r) for r in db.execute("SELECT * FROM npcaggro ORDER BY npc_name")], before,
                         "the new row gone, the GM's row as it was")
        # a GM's change since is left at the undo, and said
        sq.apply(db, p, "qa")
        db.execute("UPDATE npcaggro SET aggressive = 1 WHERE npc_name = 'brown bear'")
        n, kept = sq.undo(db, "qa")
        self.assertEqual(kept, ["npcaggro 'brown bear'"])
        self.assertEqual(db.execute("SELECT aggressive FROM npcaggro WHERE npc_name = 'brown bear'").fetchone()[0], 1)
        self.assertFalse(sq.seeded(db))
        _, errs = sq.load_sets([self.spec_file("qb", [], attackable=[]) ])
        self.assertEqual(errs, [])
        bad = os.path.join(self.dir, "bad.json")
        with open(bad, "w") as f:
            json.dump(dict(set="qc", npcs=[], peaceful="Mayor Beta"), f)
        _, errs = sq.load_sets([bad])
        self.assertTrue(any("peaceful is not a list of names" in e for e in errs), errs)

    def test_a_later_guides_key_finds_its_map_points(self):
        self.assertEqual(sq.guide_of("1-20/freeport/warrior/L3"), "1-20/freeport/warrior")
        self.assertEqual(sq.guide_of("Portal/zaoran-plateau/portal"), "portal/zaoran-plateau")
        self.assertEqual(sq.guide_of("30,40,49/group-4-evil/p1"), "30-40-49/group-4-evil", "the map's ids: lower case, hyphens")
        with open(os.path.join(self.quests, "portal.json"), "w") as f:
            json.dump(dict(city="portal", quests=[dict(key="Portal/test-isle/p1", giver="Isle Keeper", steps=[])]), f)
        self.assertEqual(sq.guides_of(self.quests)[sq.key("Isle Keeper")], {"portal/test-isle"})
        # two map points of the name: the one of the guide's quest is picked
        with open(os.path.join(self.data, "Map", "zones.json")) as f:
            zones = json.load(f)
        zones["pois"] += [dict(name="Isle Keeper", x=5200, y=1600, questGuideIds=["misc/elsewhere"]),
                          dict(name="Isle Keeper", x=4600, y=1100, questGuideIds=["portal/test-isle"])]
        with open(os.path.join(self.data, "Map", "zones.json"), "w") as f:
            json.dump(zones, f)
        p = self.plan([sq.S("Isle Keeper", sq.TALKER, ("race", "human", sq.MALE), ("poi", "tunaria", "Isle Keeper"))])
        r = p["rows"][0]["row"]
        self.assertEqual((round(r["x_coord"]), round(r["z_coord"])), (10200, 4200), "2 x 4600 + 1000, 2 x 1100 + 2000")

    def run_cli(self, *args):
        import subprocess
        tool = os.path.join(os.path.dirname(os.path.abspath(__file__)), "seed_quest_npcs.py")
        return subprocess.run([sys.executable, "-I", tool, "--db", self.db_path, "--ground", self.ground, "--data", self.data,
                               "--quests", self.quests, "--no-backup", *args], capture_output=True, text=True)

    def test_the_command_line_names_its_sets(self):
        path = self.spec_file("qa", [dict(name="Sighted One", role="foe", body=["kind", "bear"], place=["ff"])])
        out = self.run_cli("--specs", path, "--apply")
        self.assertNotEqual(out.returncode, 0, "an apply names its sets")
        self.assertIn("name the sets to apply", out.stderr)
        out = self.run_cli("--specs", path, "--apply", "--set", "qa")
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertIn("wrote 1 spawns", out.stdout)
        out = self.run_cli("--specs", path)
        self.assertIn("== set qa", out.stdout)
        self.assertIn("in this database already (1 NPCs)", out.stdout, "a report of every set, the one in said")
        self.assertIn("== set 1-20", out.stdout)
        out = self.run_cli("--specs", path, "--undo")
        self.assertEqual(out.returncode, 0, out.stderr)   # one set in: the old --undo undoes it
        self.assertIn("set qa: deleted 1 seeded NPCs", out.stdout)


if __name__ == "__main__":
    unittest.main()
