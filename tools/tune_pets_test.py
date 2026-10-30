#!/usr/bin/env python3
"""tools/tune_pets.py against a made-up database at the 01:30 state with tools/tune_balance.py and
tools/retune_balance.py applied, as a server of 2026-10-30 left it (balancelevel's pet_special_hits at 3) and, in a
second case, with the pet lane's templates made (pettemplate, the seeds' numbers): the pet classes' rows move to the
fit's, the spectre's instant to 1.5, the weapon pets' melee and the animation's armour and hit points to the fit's,
nine mastery templates added (each its kind's as this leaves it, fewer hit points) and their summonings given them;
a GM's row is left (and said); a second --apply changes nothing; --undo gives back every table exactly and drops the
log; nothing is done before the re-fit; the log's columns pass the snapshot's name rules. No game data.

    python3 tools/tune_pets_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
FIRST = os.path.join(HERE, "tune_balance.py")
RETUNE = os.path.join(HERE, "retune_balance.py")
TOOL = os.path.join(HERE, "tune_pets.py")
sys.path.insert(0, HERE)
import retune_balance as rb  # noqa: E402
import tune_balance as tb  # noqa: E402
import tune_pets as tp  # noqa: E402
from tune_balance_test import SCHEMA, secret_like  # noqa: E402

KIND = '[{"kind":"taunt","every":9,"first":1,"hits":[[1,84,88]]}]'
PET_TABLES = f"""
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '',
    special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0, special_fx INTEGER NOT NULL DEFAULT 0,
    special_every REAL NOT NULL DEFAULT 0, special_first REAL NOT NULL DEFAULT 0, special_jitter REAL NOT NULL DEFAULT 0,
    special_from INTEGER NOT NULL DEFAULT 0, abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE pettemplate (spell TEXT NOT NULL, weapon INTEGER NOT NULL DEFAULT 0, template TEXT NOT NULL DEFAULT '',
    swing_class INTEGER NOT NULL DEFAULT -1, swing INTEGER NOT NULL DEFAULT -1, round_ms INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (spell, weapon));
INSERT INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, abilities, note, source) VALUES
    ('pet necromancer weapon', 3, 100, 100, 120, '[]', 'the seed', 'seed'),
    ('pet animation', 12, 150, 70, 100, '{KIND}', 'the seed', 'seed'),
    ('pet earth', 0, 130, 80, 90, '[]', 'the seed', 'seed'), ('pet fire', 0, 100, 100, 125, '[]', 'the seed', 'seed'),
    ('pet air', 13, 80, 115, 100, '[]', 'the seed', 'seed'), ('pet water', 9, 100, 100, 100, '[]', 'the seed', 'seed');
INSERT INTO pettemplate (spell, weapon, template, source) VALUES
    ('summon badi', 0, 'pet earth', 'seed'), ('summon efreet', 0, 'pet fire', 'seed'), ('summon djinn', 0, 'pet air', 'seed'),
    ('summon badi elder', 0, 'pet earth', 'seed'), ('summon efreeti lord', 0, 'pet fire', 'seed'),
    ('summon ghilan noble', 0, 'pet water', 'seed'), ('gymon''s animation', 0, 'pet animation', 'seed'),
    ('earthen golem', 0, 'pet animation', 'seed'), ('ferrous golem', 0, 'pet animation', 'seed'),
    ('stone golem', 0, 'pet animation', 'seed'), ('summon ghilan', 0, 'pet water', 'seed');
"""


class TunePets(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        for level, row in tb.LEVELS_0130.items():
            db.execute(f"INSERT INTO balancelevel (level, {', '.join(tb.LEVEL_COLS)}, source) VALUES (?, {', '.join('?' * 7)}, 'seed')",
                       (level,) + row)
        for diff, (now, _) in tb.CONS.items():
            db.execute("INSERT INTO balancecon (diff, damage_pct, taken_pct, root_resist_pct, root_break_pct, source) VALUES (?, ?, ?, ?, ?, 'seed')",
                       (diff,) + now)
        for cls, now in tb.CLASSES.items():
            db.execute("INSERT INTO balanceclass (class, melee_taken_pct, source) VALUES (?, ?, 'seed')", (cls, now))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, tool, *args):
        r = subprocess.run([sys.executable, "-E", "-s", tool, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        if r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r.stdout

    def retuned(self, pets=False):
        """the state the re-fit leaves, as a server of 2026-10-30 found it (its column at 3), its pets' tables made"""
        self.run_tool(FIRST, "--apply")
        self.run_tool(RETUNE, "--apply")
        c = sqlite3.connect(self.db)
        c.execute("ALTER TABLE balancelevel ADD COLUMN pet_special_hits REAL NOT NULL DEFAULT 3")
        if pets:
            c.executescript(PET_TABLES)
        c.commit()
        c.close()

    def tables(self):
        c = sqlite3.connect(self.db)
        try:
            names = [n for (n,) in c.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'sqlite_sequence' ORDER BY name")]
            return {n: c.execute(f"SELECT * FROM {n}").fetchall() for n in names}  # rowid order: a moved row shows
        finally:
            c.close()

    def pet_rows(self):
        c = sqlite3.connect(self.db)
        try:
            out = {}
            for cls, l, m, p, d in c.execute("SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM balanceclasslevel "
                                             "WHERE class IN (10, 11, 12)"):
                out.setdefault(cls, {})[l] = (m, p, d)
            return out
        finally:
            c.close()

    def check_rows(self):
        self.assertEqual(self.pet_rows(), {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in tp.PETS.items()})
        c = sqlite3.connect(self.db)
        self.assertEqual({v for (v,) in c.execute("SELECT pet_special_hits FROM balancelevel")}, {1.5})
        cols = [r[1] for r in c.execute(f"PRAGMA table_info({tp.LOG})")]
        c.close()
        secret = secret_like()
        for col in cols + [tp.LOG]:
            self.assertFalse(secret(col), f"{col} is named like a secret: every database snapshot would be refused")

    def test_nothing_before_the_retune(self):
        self.run_tool(FIRST, "--apply")
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("0 rows to change", out)
        self.assertIn("tools/retune_balance.py first", out)

    def test_before_the_pets_tables(self):
        self.retuned()
        before = self.tables()
        out = self.run_tool(TOOL)
        self.assertIn("no pettemplate yet", out)
        self.assertEqual(self.tables(), before)  # report only
        self.run_tool(TOOL, "--apply")
        self.check_rows()
        self.assertIn("0 rows to change", self.run_tool(TOOL, "--apply"))
        self.run_tool(TOOL, "--undo")
        self.assertEqual(self.tables(), before)

    def test_with_the_pets_tables(self):
        self.retuned(pets=True)
        before = self.tables()
        self.run_tool(TOOL, "--apply")
        self.check_rows()
        c = sqlite3.connect(self.db)
        tm = {n: (h, t, m, a) for n, h, t, m, a in c.execute("SELECT name, hp_pct, taken_pct, melee_pct, abilities FROM classtemplates")}
        pt = dict(c.execute("SELECT spell, template FROM pettemplate"))
        c.close()
        self.assertEqual(tm["pet necromancer weapon"][2], 100)
        self.assertEqual(tm["pet animation"][:3], (100, 100, 100))
        for name, kind, hp, spell in tp.MASTERY:
            want = (hp,) + tm[kind][1:]  # its kind's as this leaves it (the animation's 100 taken, not 70), fewer hit points
            self.assertEqual(tm[name], want, name)
            self.assertLess(hp, tm[kind][0])
            self.assertEqual(pt[spell], name)
        self.assertEqual((pt["stone golem"], pt["summon ghilan"]), ("pet animation", "pet water"))  # the control
        self.assertIn("0 rows to change", self.run_tool(TOOL, "--apply"))
        self.run_tool(TOOL, "--undo")
        self.assertEqual(self.tables(), before)

    def test_a_gms_rows_are_left(self):
        self.retuned(pets=True)
        cls, level = next((c, l) for c, by in tp.PETS.items() for l in by
                          if l in rb.RETUNED.get(c, {}) and tuple(map(float, rb.RETUNED[c][l])) != tuple(map(float, by[l])))
        c = sqlite3.connect(self.db)
        c.execute("UPDATE balanceclasslevel SET pet_hp_pct = 333 WHERE class = ? AND level = ?", (cls, level))
        c.execute("UPDATE classtemplates SET melee_pct = 110 WHERE name = 'pet necromancer weapon'")
        c.execute("UPDATE balancelevel SET pet_special_hits = 2 WHERE level = 30")
        c.commit()
        c.close()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn(f"left as it is: balanceclasslevel {cls} {level}", out)
        self.assertIn("left as it is: classtemplates 'pet necromancer weapon'", out)
        self.assertIn("left as it is: balancelevel 30", out)
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT pet_hp_pct FROM balanceclasslevel WHERE class = ? AND level = ?", (cls, level)).fetchone()[0], 333)
        self.assertEqual(c.execute("SELECT melee_pct FROM classtemplates WHERE name = 'pet necromancer weapon'").fetchone()[0], 110)
        c.close()


if __name__ == "__main__":
    unittest.main()
