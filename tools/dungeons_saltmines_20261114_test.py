#!/usr/bin/env python3
"""dungeons_saltmines_20261114.py on a small database holding the dungeons' tiers, Blackburrow's warden twin, the base
items and a few of the mine's world 0 rows (the repository's recipe's "saltmines" box): the report writes nothing;
--apply copies the mine's own creatures into world 14 (the Siloth and the drachnids as trash, the knights by the shadow
knight tier, the drachnid named, the two bosses at 65), the twelve items and their tables, the loot rows and a warden
twin at the entrance; a second apply does nothing; --undo puts every table back exactly. Controls: friendly, seeded,
wildlife and rows outside the box are left; a shard with no tiers (the Live Server) is refused, writing nothing; a
taken item id is refused."""
import importlib.util
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "dungeons_saltmines_20261114.py")
spec = importlib.util.spec_from_file_location("tool", TOOL)
tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tool)

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, npc_level INTEGER, npc_type INTEGER,
    npc_id INTEGER);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE quest_npc_seed_rows (npc_row INTEGER);
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1);
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT, note TEXT, source TEXT);
CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT,
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, rent INTEGER, equipslot INTEGER,
    classuse INTEGER);
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER,
    minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""
# (name, x, y, z, level, npc_type): the mine's rows and its controls
ROWS = [
    ("Siloth slaver", 14564, 6.0, 19079, 40, 0),
    ("Siloth knight", 14586, 6.0, 19041, 40, 0),
    ("Siloth guard", 14662, 54.1, 19051, 40, 0),
    ("drachnid lurker", 14570, -88.0, 18992, 45, 0),
    ("drachnid parasite", 14528, -88.0, 19035, 39, 0),
    ("Drachnid Body Snatcher", 14474, -73.0, 19004, 47, 0),
    ("Maestro of Rancor", 14481, -60.0, 18966, 51, 0),
    ("Spider Queen Aszec", 14488, -57.0, 19034, 51, 0),
    ("Mayong Mistmoore", 14542, -88.0, 18980, 54, 0),
    ("coffin of Mayong", 14546, -88.0, 18978, 1, 384),      # friendly
    ("Collin Ilbeth", 14554, -28.0, 19023, 51, 0),          # a quest's seeded row
    ("spineback", 14637, 54.0, 19012, 40, 0),               # wildlife on the surface
    ("Siloth hound", 14727, 54.1, 19102, 40, 0),            # outside the box
]


def make(path, tiers=True):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    if tiers:
        for t in list(tool.base.TEMPLATES) + [tool.KNIGHT_TIER]:
            db.execute("INSERT INTO classtemplates (name) VALUES (?)", (t,))
        db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, modelid, npc_level, npc_type, npc_id)"
                   " VALUES (?, 0, ?, 62.1, ?, 0, 14, 1843770060, 60, 128, 0)", (tool.WARDEN, *tool.TWIN_OF))
    for _, _, bid in tool.ITEMS:
        db.execute("INSERT OR IGNORE INTO itempattern VALUES (?, ?, '', 0, 5, 1)", (bid, f"base {bid}"))
    for n, x, y, z, lv, ty in ROWS:
        db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, modelid, npc_level, npc_type, npc_id)"
                   " VALUES (?, 105, ?, ?, ?, 64, 0, 1893243078, ?, ?, 7)", (n, x, y, z, lv, ty))
    seeded = db.execute("SELECT id FROM npcs WHERE npc_name = 'Collin Ilbeth'").fetchone()[0]
    db.execute("INSERT INTO quest_npc_seed_rows VALUES (?)", (seeded,))
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "gm_audit" not in l and "dungeons_saltmines_1114_log" not in l)
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


class SaltMines(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("report only", r.stdout)
        self.assertEqual(dump(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.db)
        got = {n: (lv, t) for n, lv, t in db.execute(
            "SELECT n.npc_name, n.npc_level, t.template FROM npcs n JOIN npctemplate t ON t.name = '#' || n.id WHERE n.world = 14")}
        self.assertEqual(got, {
            "Siloth slaver": (60, "dungeon trash"), "Siloth knight": (60, "dungeon trash shadowknight"),
            "Siloth guard": (60, "dungeon trash"), "drachnid parasite": (60, "dungeon trash"),
            "drachnid lurker": (62, "dungeon trash"), "Drachnid Body Snatcher": (63, "dungeon named"),
            "Maestro of Rancor": (64, "dungeon named"), "Spider Queen Aszec": (65, "dungeon boss"),
            "Mayong Mistmoore": (65, "dungeon boss")})
        # a copy stands where its original does, npc_id 0
        self.assertEqual(db.execute("SELECT x_coord, y_coord, z_coord, npc_id, zone FROM npcs WHERE world = 14 AND npc_name = 'Spider Queen Aszec'").fetchone(),
                         (14488.0, -57.0, 19034.0, 0, 105))
        self.assertEqual(db.execute("SELECT count(*), sum(rent) FROM itempattern WHERE patternid BETWEEN 995121 AND 995132").fetchone(), (12, 12))
        self.assertEqual(db.execute("SELECT tablename, count(*) FROM loottables GROUP BY 1 ORDER BY 1").fetchall(),
                         [("dungeon saltmines boss", 6), ("dungeon saltmines named", 6)])
        boss = db.execute("SELECT '#' || id FROM npcs WHERE world = 14 AND npc_name = 'Mayong Mistmoore'").fetchone()[0]
        self.assertEqual(db.execute("SELECT tablename, chance, rolls FROM npcloot WHERE npc_name = ? ORDER BY 1", (boss,)).fetchall(),
                         [("dungeon saltmines boss", 100, 2), ("dungeon saltmines named", 100, 1)])
        # the twin at the entrance: 4 to the right of (14760, 19045) facing west
        w = db.execute("SELECT x_coord, z_coord, world FROM npcs WHERE npc_name = ? AND x_coord > 14000", (tool.WARDEN,)).fetchall()
        self.assertEqual(len(w), 1)
        self.assertAlmostEqual(w[0][0], 14760, delta=0.01)
        self.assertAlmostEqual(w[0][1], 19049, delta=0.01)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail LIKE '%saltmines%'").fetchone()[0],
                         db.execute("SELECT count(*) FROM dungeons_saltmines_1114_log").fetchone()[0])
        db.close()
        applied = dump(self.db)
        r = run(self.db, "--apply")
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.db), applied)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_no_tiers_refused(self):
        make(self.db, tiers=False)
        before = dump(self.db)
        r = run(self.db, "--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("missing", r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_taken_item_refused(self):
        make(self.db)
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO itempattern VALUES (995125, 'someone else', '', 1, 5, 1)")
        db.commit()
        db.close()
        before = dump(self.db)
        r = run(self.db, "--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("taken", r.stderr)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
