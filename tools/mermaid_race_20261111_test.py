#!/usr/bin/env python3
"""mermaid_race_20261111.py on a small database: the report writes nothing; --apply writes race 32's racedefs and
racesize rows, a start row a class with the race's and the class's stats, the Erudite's items (plus the heirloom three)
and standings, a gm_audit line each; a second apply does nothing; --undo puts every table back exactly. The controls:
a database without heirloom records, a merfolk body in charactermodel, and a race 32 that is not the tool's are
refused, writing nothing. EQOA_REAL_DB=<copy>: the same on a copy of a real database."""
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import mermaid_race_20261111 as mr  # noqa: E402

TOOL = os.path.join(HERE, "mermaid_race_20261111.py")
TABLES = ("racedefs", "racesize", "defaultcharacter", "startitems", "factionstart")


def make(path, heirlooms=True, model=False, taken=False):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT NOT NULL, short_name TEXT NOT NULL DEFAULT '',
            wire_race INTEGER NOT NULL, bonus_str INTEGER NOT NULL DEFAULT 0, bonus_sta INTEGER NOT NULL DEFAULT 0,
            bonus_agi INTEGER NOT NULL DEFAULT 0, bonus_dex INTEGER NOT NULL DEFAULT 0, bonus_wis INTEGER NOT NULL DEFAULT 0,
            bonus_int INTEGER NOT NULL DEFAULT 0, bonus_cha INTEGER NOT NULL DEFAULT 0, body_male INTEGER NOT NULL DEFAULT 0,
            body_female INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
            description TEXT NOT NULL DEFAULT '', class_mask INTEGER NOT NULL DEFAULT 0, city INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE racesize (race INTEGER PRIMARY KEY, size REAL NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE defaultcharacter (defaultCharacterID INTEGER PRIMARY KEY AUTOINCREMENT, tclass INTEGER NOT NULL,
            race INTEGER NOT NULL, humType INTEGER NOT NULL, level INTEGER NOT NULL, tunar INTEGER NOT NULL,
            bankTunar INTEGER NOT NULL, unusedTP INTEGER NOT NULL, totalTP INTEGER NOT NULL, x REAL NOT NULL, y REAL NOT NULL,
            z REAL, facing REAL NOT NULL, defaultSpeed REAL, world INTEGER, defaultStrength INTEGER, defaultStamina INTEGER,
            defaultAgility INTEGER, defaultDexterity INTEGER, defaultWisdom INTEGER, defaultIntelligence INTEGER,
            defaultCharisma INTEGER);
        CREATE TABLE startitems (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER NOT NULL, class INTEGER NOT NULL,
            humtype INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL, count INTEGER NOT NULL DEFAULT 1,
            note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE factionstart (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER NOT NULL DEFAULT -1,
            class INTEGER NOT NULL DEFAULT -1, humtype INTEGER NOT NULL DEFAULT -1, faction TEXT NOT NULL,
            standing INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
            UNIQUE (race, class, humtype, faction));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, attacktype INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE charactermodel (sex INTEGER NOT NULL, modelid INTEGER NOT NULL PRIMARY KEY, race INTEGER NOT NULL);
        CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY, class INTEGER NOT NULL, role TEXT NOT NULL, epic INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO racedefs (id, name, wire_race) VALUES (30, 'Jal''Raeth', 8);
        INSERT INTO itempattern VALUES (100, 0), (200, 5), (715000, 0), (715010, 0), (715020, 0);
        -- the Erudite's start: a wizard's row (13) and a cleric's (9); every other class takes its first (9)
        INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
            VALUES (13, 8, 0, 1, 0, 0, 0, 0, 4727, 54, 21654, 0, 0), (9, 8, 0, 1, 0, 0, 0, 0, 4700, 54, 21600, 1, 0),
                   (13, 0, 0, 1, 0, 0, 0, 0, 1, 2, 3, 0, 0);
        INSERT INTO startitems (race, class, humtype, patternid, count) VALUES (8, 13, 0, 100, 1), (8, 13, 0, 200, 1), (0, 0, 0, 200, 1);
        INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (8, -1, -1, 'Highbourne', 300), (0, -1, -1, 'Qeynos', 300);
    """)
    if heirlooms:
        db.execute("INSERT INTO heirlooms VALUES (715100, 0, 'tank', 0)")
    if model:
        db.execute("INSERT INTO charactermodel VALUES (1, ?, 9)", (mr.s32(mr.FEMALE),))
    if taken:
        db.execute("INSERT INTO racesize VALUES (32, 1.2, 'a GM', 'gm')")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in TABLES}
    db.close()
    return out


def run(*args):
    return subprocess.run([sys.executable, TOOL, *args], capture_output=True, text=True)


class MermaidRace(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.db = os.path.join(self.dir, "eqoa.db")

    def tearDown(self):
        shutil.rmtree(self.dir)

    def test_apply_once_and_undo_exactly(self):
        make(self.db)
        before = dump(self.db)
        r = run("--db", self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before, "the report wrote")
        r = run("--db", self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        row = db.execute("SELECT name, short_name, wire_race, bonus_agi, bonus_cha, body_male, body_female FROM racedefs WHERE id = 32").fetchone()
        self.assertEqual(row, ("Mermaid", "MER", 8, 15, 10, mr.s32(0xA316EB78), mr.s32(0x74BA7C3D)))
        self.assertEqual(db.execute("SELECT size FROM racesize WHERE race = 32").fetchone(), (0.75,))
        starts = db.execute("SELECT tclass, x, z, defaultAgility, defaultCharisma FROM defaultcharacter WHERE race = 32 ORDER BY tclass").fetchall()
        self.assertEqual(len(starts), 15)
        self.assertEqual(starts[13], (13, 4727, 21654, 60 + 15 + 0, 60 + 10 - 10))  # the wizard's own spot
        self.assertEqual(starts[0][1:3], (4700, 21600))  # a warrior: the Erudite's first (the cleric's)
        wiz = db.execute("SELECT patternid FROM startitems WHERE race = 32 AND class = 13 ORDER BY id").fetchall()
        self.assertEqual([p for (p,) in wiz], [100, 200, 715000, 715010, 715020])
        self.assertEqual(db.execute("SELECT faction, standing FROM factionstart WHERE race = 32").fetchall(), [("Highbourne", 300)])
        n = db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0]
        self.assertEqual(n, db.execute(f"SELECT COUNT(*) FROM {mr.LOG}").fetchone()[0])
        db.close()
        after = dump(self.db)
        r = run("--db", self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.db), after, "a second apply wrote")
        r = run("--db", self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before, "the undo is not exact")

    def test_refusals_write_nothing(self):
        for kw in ({"heirlooms": False}, {"model": True}, {"taken": True}):
            make(self.db, **kw)
            before = dump(self.db)
            r = run("--db", self.db, "--apply")
            self.assertEqual(r.returncode, 1, (kw, r.stdout))
            self.assertIn("refused", r.stdout)
            self.assertEqual(dump(self.db), before, kw)
            os.remove(self.db)

    def test_a_real_database(self):
        real = os.environ.get("EQOA_REAL_DB")
        if not real:
            self.skipTest("EQOA_REAL_DB not set")
        shutil.copy(real, self.db)
        before = dump(self.db)
        self.assertEqual(run("--db", self.db, "--apply").returncode, 0)
        self.assertEqual(len(dump(self.db)["defaultcharacter"]), len(before["defaultcharacter"]) + 15)
        self.assertEqual(run("--db", self.db, "--undo").returncode, 0)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
