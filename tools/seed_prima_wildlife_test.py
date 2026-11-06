#!/usr/bin/env python3
"""tools/seed_prima_wildlife.py against a made-up world: a database of a few
creatures and a town, a bestiary of invented animals, two zones on EQOA-Data's
grid, a ground file of a plain with a hill and a lake, and an environment
file with the lake's water and a pool of lava. No game data.

    python3 tools/seed_prima_wildlife_test.py
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
import seed_npc_move as mv  # noqa: E402
import seed_prima_wildlife as pw  # noqa: E402
from seed_wildlife_test import height, write_ground  # noqa: E402
from seed_pod import Env  # noqa: E402

PC_BODY, WOLF, GATOR, RAY, PIKE, BAT, BEETLE, CUB, BONEDOG, RHINO = (
    0x1111, 0x2222, 0x3333, 0x4444, 0x5555, 0x6666, 0x7777, 0x8888, 0x9999, 0xAAAA)
WATER_TOP = 50.0                          # the lake's surface, in the hollow round (10500, 5450)
LAKE = (10300.0, 10700.0, 5250.0, 5650.0)
LAVA_POOL = (11690.0, 11710.0, 4290.0, 4310.0)
TOWN = [(10400 + 40 * (i % 4), 4400 + 40 * (i // 4)) for i in range(12)]


def write_env(path):
    """An environment file (seed_pod.Env's format): one zone; its tree tests
    the lake's box under WATER_TOP (water), then the lava pool's under 60
    (lava), else dry."""
    leaf = 0x40000000
    nodes = []

    def box(b, top, flag_leaf, after):
        x0, x1, z0, z1 = b
        start = len(nodes)
        planes = [(1, 0, 0, -x0), (-1, 0, 0, x1), (0, 0, 1, -z0), (0, 0, -1, z1), (0, -1, 0, top)]
        for i, p in enumerate(planes):
            front = start + i + 1 if i < len(planes) - 1 else leaf | flag_leaf
            nodes.append((p, front, after))
        return start

    # leaves: 0 dry, 1 water, 2 lava; the lava box after the lake's, dry after both
    lava_start = 5
    box(LAKE, WATER_TOP, 1, lava_start)
    box(LAVA_POOL, 60.0, 2, leaf | 0)
    out = b"EQEV" + struct.pack("<I", 1)
    out += struct.pack("<III", 0, 1, 0)                    # the world's tree: zone 0 everywhere
    out += struct.pack("<I", 1)
    out += struct.pack("<II", len(nodes), 3)
    for (nx, ny, nz, d), front, back in nodes:
        out += struct.pack("<4f2I", nx, ny, nz, d, front, back)
    out += struct.pack("<3I", 0, 1, 4)
    with open(path, "wb") as f:
        f.write(out)


def write_zones(data_dir):
    """Two zones on EQOA-Data's map, at the points seed_npc_move's fit lays on
    cells (3, 0) and (4, 0): x 10000-12000 and 12000-14000, z 4000-6000."""
    os.makedirs(os.path.join(data_dir, "Map"), exist_ok=True)

    def at(gx, gz):
        return (gx - mv.BX) / mv.AX, (gz - mv.BZ) / mv.AZ

    x1, y1 = at(11000, 5000)
    x2, y2 = at(13000, 5000)
    doc = {"zones": [{"name": "Testmoor", "x": x1, "y": y1, "biomes": ["Forest", "Forest", "Swamp", "Forest"]},
                     {"name": "Eastmoor", "x": x2, "y": y2, "biomes": ["Forest", "Forest", "Forest", "Forest"]},
                     {"name": "Nowhere", "x": 0, "y": 0}],
           "towns": [], "pois": []}
    with open(os.path.join(data_dir, "Map", "zones.json"), "w") as f:
        json.dump(doc, f)


BESTIARY = {"species": [
    {"region": "Forest", "frontiers": False, "name": "Mire Wolf", "prima_levels": [20, 22]},
    {"region": "Swamp", "frontiers": False, "name": "Bog Alligator", "prima_levels": [20, 22]},
    {"region": "Fresh Water", "frontiers": False, "name": "Lake Skate", "prima_levels": [20, 22]},
    {"region": "Fresh Water", "frontiers": False, "name": "Lake Pike", "prima_levels": [20, 22]},
    {"region": "Volcanic", "frontiers": False, "name": "Ember Bat", "prima_levels": [20, 22]},
    {"region": "Forest", "frontiers": False, "name": "Grey Wolf", "prima_levels": [20, 22]},
    {"region": "Forest", "frontiers": False, "name": "Ashen Hound", "prima_levels": [20, 22]},
    {"region": "Jungle", "frontiers": False, "name": "Moor Strider", "prima_levels": [20, 22]},
    {"region": "Wetlands", "frontiers": True, "name": "Fen Rhinoceros", "prima_levels": [20, 22]},
    {"region": "Forest", "frontiers": False, "name": "Testmoor Brigand", "prima_levels": [20, 22]},
]}

SCHEMA = [
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, "
    "y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, modelid INTEGER, "
    "size REAL, race TEXT, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER)",
    "CREATE TABLE charactermodel (modelid INTEGER)",
    "CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT)",
    "CREATE TABLE npcundead (name TEXT PRIMARY KEY, undead INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0, "
    "water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL)",
    "CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', "
    "source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT NOT NULL DEFAULT '', "
    "profile TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')",
    "CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT NOT NULL, region TEXT NOT NULL "
    "DEFAULT '*', npc_name TEXT NOT NULL DEFAULT '', patternid INTEGER NOT NULL, weight INTEGER NOT NULL DEFAULT 1, "
    "minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 100, source TEXT NOT NULL DEFAULT 'gm')",
]


def npc(db, name, x, z, model, level=1, ntype=0, y=None, world=0, size=1.0):
    db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, hp, modelid, size, race, "
               "npc_level, npc_type, npc_id) VALUES (?, 7, ?, ?, ?, 0, ?, 100, ?, ?, '0', ?, ?, 99)",
               (name, x, height(x, z) if y is None else y, z, world, model, size, level, ntype))


def build(path):
    db = sqlite3.connect(path, isolation_level=None)
    for s in SCHEMA:
        db.execute(s)
    db.execute("INSERT INTO charactermodel VALUES (?)", (PC_BODY,))
    db.execute("INSERT INTO spellpattern VALUES (1, 'Summon Ray')")
    # the bodies, far off in world 3: grey wolves (a bestiary animal of 6
    # rows: present), wolf cubs, river alligators, manta rays, cave bats, grey
    # rhinoceroses and a bone dog (undead) of its own
    for i in range(6):
        npc(db, "grey wolf", 2000 + 30 * i, 2000, WOLF, level=21, world=3)
    for i in range(3):
        npc(db, "grey wolf cub", 2000 + 30 * i, 2100, CUB, level=21, world=3)
        npc(db, "river alligator", 2000 + 30 * i, 2200, GATOR, level=21, world=3, size=1.5)
        npc(db, "manta ray", 2000 + 30 * i, 2300, RAY, level=21, world=3)
        npc(db, "cave bat", 2000 + 30 * i, 2400, BAT, level=21, world=3)
        npc(db, "bone dog", 2000 + 30 * i, 2500, BONEDOG, level=21, world=3)
        npc(db, "grey rhinoceros", 2000 + 30 * i, 2600, RHINO, level=21, world=3)
    db.execute("INSERT INTO npcundead VALUES ('bone dog', 1, '', 'seed')")
    # the name-keyed rows a made animal copies from its body
    db.execute("INSERT INTO npcclass VALUES ('grey wolf', 0, 'seed')")
    db.execute("INSERT INTO npctemplate VALUES ('grey wolf', 'roamer', '', 'seed')")
    db.execute("INSERT INTO lootcreature VALUES ('grey wolf', 'forest', '', 'seed')")
    db.execute("INSERT INTO lootpool (category, region, npc_name, patternid) VALUES ('pelt', '*', 'grey wolf', 77)")
    db.execute("INSERT INTO lootpool (category, region, npc_name, patternid) VALUES ('fang', '*', 'grey wolf', 78)")
    db.execute("INSERT INTO lootcreature VALUES ('manta ray', 'marine', '', 'seed')")
    # the town, its people on players' race bodies
    for x, z in TOWN:
        npc(db, f"Townsman {x} {z}", x, z, PC_BODY, level=30)
    # creatures of levels 18-24 over the two zones' land, 150 apart: the levels the animals keep to
    for i in range(20):
        for j in range(13):
            x, z = 10075 + 150 * i, 4075 + 150 * j
            if math.hypot(x - 10500, z - 5450) < 230 or min(math.hypot(x - tx, z - tz) for tx, tz in TOWN) < 200:
                continue
            npc(db, "moss beetle", x, z, BEETLE, level=18 + (i + j) % 7)
    # the lake's pike (a bestiary animal of 3: thin), on its floor
    for x, z in ((10500, 5450), (10450, 5410), (10550, 5490)):
        npc(db, "Lake Pike", x, z, PIKE, level=21)
    return db


class SeedPrimaWildlifeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = tempfile.TemporaryDirectory()
        cls.ground = os.path.join(cls.tmp.name, "ground")
        os.makedirs(cls.ground)
        write_ground(os.path.join(cls.ground, "0.ground"))
        write_env(os.path.join(cls.ground, "0.env"))
        cls.data = os.path.join(cls.tmp.name, "EQOA-Data")
        write_zones(cls.data)
        cls.bestiary_path = os.path.join(cls.tmp.name, "bestiary.json")
        with open(cls.bestiary_path, "w") as f:
            json.dump(BESTIARY, f)
        cls.aliases = dict(pw.ALIASES)
        pw.ALIASES["ashen hound"] = "grey wolf"   # a spelling of a present animal
        pw.READ_ANIMALS.update({"lake skate", "moor strider"})   # animals wildlife.go's words miss

    @classmethod
    def tearDownClass(cls):
        pw.ALIASES.clear()
        pw.ALIASES.update(cls.aliases)
        pw.READ_ANIMALS.difference_update({"lake skate", "moor strider"})
        cls.tmp.cleanup()

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        self.db = build(self.path)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def plan(self, **kw):
        return pw.plan(self.db, self.ground, self.data, pw.read_bestiary(self.bestiary_path), count=6, **kw)

    def rows(self, p, name):
        return [r["row"] for r in p["rows"] if r["row"]["npc_name"].lower() == name]

    def test_the_environment_file_reads(self):
        e = Env(os.path.join(self.ground, "0.env"))
        self.assertEqual(e.flags(10500, height(10500, 5450) + 0.5, 5450), 1, "the lake's floor is under water")
        self.assertEqual(e.flags(11000, height(11000, 4600) + 0.5, 4600), 0, "the plain is dry")
        self.assertEqual(e.flags(11700, height(11700, 4300) + 0.5, 4300), 4, "the pool is lava")

    def test_names_and_families(self):
        self.assertEqual(pw.head("Arctic Wolf Cub"), ("wolf", "canine", {"arctic", "young"}))
        self.assertEqual(pw.head("Snowbat"), ("bat", "bat", {"snow"}))
        self.assertEqual(pw.head("Brazercampus"), ("campus", "seahorse", {"brazer"}))
        self.assertEqual(pw.head("Dire Timberwolf"), ("wolf", "canine", {"dire", "timber"}))
        self.assertEqual(pw.head("Sand Midges"), ("midge", "fly", {"sand"}))
        self.assertIsNone(pw.head("Hill Giant")[1], "a giant is no ant")
        self.assertEqual(pw.unkern("Yo u ng Lion"), "Young Lion")
        self.assertTrue(pw.is_animal("Fen Rhinoceros"))
        self.assertTrue(pw.is_animal("Hydra"), "an animal wildlife.go's words miss, by reading")
        self.assertFalse(pw.is_animal("Testmoor Brigand"))

    def test_read_bestiary(self):
        b = pw.read_bestiary(self.bestiary_path)
        self.assertNotIn("testmoor brigand", b, "not an animal")
        self.assertEqual(b["fen rhinoceros"]["entries"], [dict(part=pw.FRONTIERS, table="Wetlands", lo=20, hi=22,
                                                               page=None)])
        self.assertEqual(b["mire wolf"]["entries"][0]["part"], pw.TUNARIA)

    def test_bodies(self):
        _, npcs, pc, spells, undead, _ = pw.load(self.db)
        kinds = pw.body_kinds(npcs, pc, spells, undead)
        self.assertEqual(pw.body_for("Mire Wolf", 20, 22, set(), [0], kinds)[0], "grey wolf", "grown as grown")
        self.assertEqual(pw.body_for("Mire Wolf Pup", 20, 22, set(), [0], kinds)[0], "grey wolf cub", "young as young")
        self.assertEqual(pw.body_for("Bone Hound", 20, 22, set(), [0], kinds)[0], "bone dog", "undead by its word")
        self.assertEqual(pw.body_for("Ash Hound", 20, 22, set(), [0], kinds)[0], "grey wolf", "not undead without it")
        self.assertIsNone(pw.body_for("Moor Strider", 20, 22, set(), [0], kinds)[0])

    def test_spawns_stand_where_they_should(self):
        p = self.plan()
        g = pw.sw.Ground(os.path.join(self.ground, "0.ground"))
        e = Env(os.path.join(self.ground, "0.env"))
        others = [(r[0], r[1]) for r in self.db.execute("SELECT x_coord, z_coord FROM npcs WHERE world = 0")]
        placed = []
        self.assertTrue(p["rows"])
        for r in p["rows"]:
            row = r["row"]
            x, y, z = row["x_coord"], row["y_coord"], row["z_coord"]
            self.assertTrue(any(abs(f - y) < 1e-3 for f in g.floors(x, z)), "on the terrain")
            self.assertGreaterEqual(min(math.hypot(x - tx, z - tz) for tx, tz in TOWN), pw.TOWN_CLEAR - 1e-6)
            for ox, oz in others + placed:
                self.assertGreaterEqual(math.hypot(x - ox, z - oz), pw.SPACING - 1e-3, row)
            placed.append((x, z))
            self.assertTrue(20 <= row["npc_level"] <= 22, row)
            self.assertEqual((row["npc_id"], row["zone"], row["dt_kill"], row["world"]), (0, 0, None, 0))
            f = e.flags(x, y + 0.5, z)
            name = row["npc_name"].lower()
            if name in ("lake skate", "lake pike"):
                self.assertEqual(f & 1, 1, "a fish in the water")
                self.assertEqual(e.flags(x, y + pw.WATER_DEPTH, z) & 1, 1, "deep enough")
            elif name == "bog alligator":
                self.assertEqual(f & pw.SOAKED, 0, "in the water or out, never slime or lava")
            else:
                self.assertEqual(f, 0, f"{name}: on dry land")
        for name in {r["row"]["npc_name"] for r in p["rows"]}:
            mine = [(r["row"]["x_coord"], r["row"]["z_coord"]) for r in p["rows"] if r["row"]["npc_name"] == name]
            for i, (ax, az) in enumerate(mine):
                for bx, bz in mine[i + 1:]:
                    self.assertGreaterEqual(math.hypot(ax - bx, az - bz), pw.APART - 1e-3, name)

    def test_biome_quarters(self):
        p = self.plan()
        wolves = self.rows(p, "mire wolf")
        self.assertEqual(len(wolves), 6, "three in each of the two zones")
        for w in wolves:
            self.assertEqual(pw.biome_at(pw.read_zones(self.data), 0, w["x_coord"], w["z_coord"]), "Forest")
        gators = self.rows(p, "bog alligator")
        self.assertTrue(gators)
        for a in gators:
            self.assertTrue(a["x_coord"] < 11000 and a["z_coord"] >= 5000, "the swamp is the south-west quarter")

    def test_bodies_names_and_levels(self):
        p = self.plan()
        for name, model in (("mire wolf", WOLF), ("bog alligator", GATOR), ("ember bat", BAT)):
            rows = self.rows(p, name)
            self.assertTrue(rows, name)
            for r in rows:
                self.assertEqual(r["npc_name"], name, "the guide's name, lower case")
                self.assertEqual(r["modelid"], model)
        self.assertEqual({r["size"] for r in self.rows(p, "bog alligator")}, {1.5}, "the body's size")
        pike = self.rows(p, "lake pike")
        self.assertEqual(len(pike), 3, "a thin animal topped up to the count")
        self.assertEqual({(r["npc_name"], r["modelid"]) for r in pike}, {("Lake Pike", PIKE)}, "as its own rows")

    def test_volcanic_by_the_lava(self):
        p = self.plan()
        bats = self.rows(p, "ember bat")
        self.assertTrue(bats)
        for b in bats:
            self.assertLessEqual(math.hypot(b["x_coord"] - 11700, b["z_coord"] - 4300), pw.KIN_REACH + 1e-6)

    def test_left_out(self):
        p = self.plan()
        names = {r["row"]["npc_name"].lower() for r in p["rows"]}
        self.assertEqual(names, {"mire wolf", "bog alligator", "lake skate", "lake pike", "ember bat"})
        why = {name: reason for name, _, reason in p["left"]}
        self.assertIn("no plausible body", why["Moor Strider"])
        self.assertIn("no place", why["Fen Rhinoceros"], "no kin of its table in its worlds")
        self.assertNotIn("Grey Wolf", why, "present: not seeded")
        self.assertNotIn("Ashen Hound", why, "present by its alias")

    def test_name_keyed_rows(self):
        p = self.plan()
        extra = {(x["table"], x["name"]): x["row"] for x in p["extra"]}
        self.assertEqual(extra[("npcclass", "mire wolf")], dict(name="mire wolf", **{"class": 0}, source=pw.SOURCE))
        self.assertEqual(extra[("npctemplate", "mire wolf")]["template"], "roamer")
        self.assertEqual(extra[("lootcreature", "mire wolf")]["region"], "forest")
        self.assertEqual(sum(1 for x in p["extra"] if x["table"] == "lootpool" and x["name"] == "mire wolf"), 2)
        self.assertNotIn(("npcmove", "mire wolf"), extra, "a wolf wanders by its word")
        self.assertEqual(extra[("npcmove", "bog alligator")]["water"], "both")
        self.assertEqual(extra[("npcmove", "lake skate")]["roam"], 1, "a skate is no word of wildlife.go's")
        self.assertEqual(extra[("lootcreature", "lake skate")]["region"], "marine")
        self.assertNotIn(("npcclass", "lake pike"), extra, "a thin animal's name has its rows")

    def test_the_same_plan_twice(self):
        a = [r["row"] for r in self.plan()["rows"]]
        b = [r["row"] for r in self.plan()["rows"]]
        self.assertEqual(a, b)
        c = [r["row"] for r in self.plan(seed=99)["rows"]]
        self.assertNotEqual(a, c)

    def digest(self):
        out = {t: pw.sw.rows_digest(self.db, t) for t in
               ("npcs", "npcclass", "npctemplate", "lootcreature", "lootpool", "npcmove", "npcundead")}
        out["seq"] = self.db.execute("SELECT * FROM sqlite_sequence ORDER BY name").fetchall()
        return out

    def test_apply_and_undo(self):
        before = self.digest()
        p = self.plan()

        class Args:
            seed, count, only, skip = pw.DEFAULT_SEED, 6, "", ""

        pw.apply(self.db, p, Args)
        self.assertEqual(self.db.execute("SELECT count(*) FROM prima_wildlife_seed_npcs").fetchone()[0], len(p["rows"]))
        self.assertEqual(self.db.execute("SELECT count(*) FROM prima_wildlife_seed_rows").fetchone()[0], len(p["extra"]))
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcs WHERE npc_name = 'mire wolf'").fetchone()[0], 6)
        self.assertEqual(self.db.execute("SELECT region FROM lootcreature WHERE npc_name = 'mire wolf'").fetchone()[0],
                         "forest")
        with self.assertRaises(SystemExit):
            pw.apply(self.db, p, Args)
        self.assertEqual(pw.undo(self.db), (len(p["rows"]), len(p["extra"])))
        self.assertEqual(self.digest(), before, "every row and counter as it was")
        self.assertFalse(pw.seeded(self.db))
        with self.assertRaises(SystemExit):
            pw.undo(self.db)

    def test_undo_keeps_a_row_added_after(self):
        p = self.plan()

        class Args:
            seed, count, only, skip = pw.DEFAULT_SEED, 6, "", ""

        pw.apply(self.db, p, Args)
        npc(self.db, "added by a GM", 11800, 5800, BEETLE)
        top = self.db.execute("SELECT max(id) FROM npcs").fetchone()[0]
        pw.undo(self.db)
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcs WHERE npc_name = 'added by a GM'").fetchone()[0], 1)
        self.assertGreaterEqual(pw.seq(self.db, "npcs"), top, "the counter never goes below a row that stays")

    def test_only_and_skip(self):
        p = self.plan(only=["Mire Wolf"])
        self.assertEqual({r["row"]["npc_name"] for r in p["rows"]}, {"mire wolf"})
        p = self.plan(skip=["Mire Wolf"])
        self.assertNotIn("mire wolf", {r["row"]["npc_name"] for r in p["rows"]})
        self.assertIn("--skip", {n: w for n, _, w in p["left"]}["Mire Wolf"])


if __name__ == "__main__":
    unittest.main()
