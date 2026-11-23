#!/usr/bin/env python3
"""Tests for petpass_live_1123e.py on a small database holding the rows as read on Live: a dry run writes nothing
(the petstats column it adds rolled back with it); --apply writes each change once (a second --apply nothing): the
robe, the pets' melee, the epic camp's templates, the con rows below even, the Black Sapphire's icon; a row not as
read refuses the whole run; --undo puts every table back and drops the log."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_live_1123e as t  # noqa: E402

TABLES = ("itempattern", "petstats", "classtemplates", "npctemplate", "balancecon")


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemcost INTEGER, itemicon INTEGER, equipslot INTEGER,
            trade INTEGER, rent INTEGER, levelreq INTEGER, maxstack INTEGER, maxhp INTEGER, duration INTEGER,
            classuse INTEGER, raceuse INTEGER, lore INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, color INTEGER,
            str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER, intelligence INTEGER,
            HPMAX INTEGER, POWMAX INTEGER, AC INTEGER);
        CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', level INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER, hp_pct INTEGER, taken_pct INTEGER,
            melee_pct INTEGER, special TEXT, special_damage REAL, special_every REAL, note TEXT, source TEXT);
        CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT, note TEXT, source TEXT);
        CREATE TABLE balancecon (diff INTEGER PRIMARY KEY, damage_pct REAL, taken_pct REAL, root_resist_pct REAL,
            root_break_pct REAL, note TEXT, source TEXT);
    """)
    db.execute("""INSERT INTO itempattern VALUES (1027, 4292, 1188988948, 2, 1, 1, 55, 1, 33192, 80, 13, 1023, 1,
        'Khal Robe of War', 'Khalimet''s energy surges through the robe.', 0, 65535, 0, 20, 0, 20, 0, 20, 20, 150, 0, 100)""")
    db.execute("INSERT INTO itempattern (patternid, itemname, itemicon) VALUES (?, ?, ?)", (t.ONYX, t.ONYX_NAME, t.DARK_GEM))
    db.execute("INSERT INTO itempattern (patternid, itemname, itemicon) VALUES (?, ?, ?)",
               (t.SAPPHIRE, t.SAPPHIRE_NAME, t.GREEN_GEM))
    for sid, name in t.ENC_PETS.items():
        db.execute("INSERT INTO petstats (spellid, name) VALUES (?, ?)", (sid, name))
    db.execute("INSERT INTO petstats (spellid, name) VALUES (700011, 'Shambling Bones')")
    db.execute("INSERT INTO classtemplates VALUES ('roamer', 0, 100, 100, 100, 'Critical Hit', 7.4, 30, 'basic', 'roamer-seed')")
    for _, _, _, names in t.EPIC_TEMPLATES:
        for n in names:
            db.execute("INSERT INTO npctemplate VALUES (?, 'roamer', 'named, level 52', 'roamer-seed')", (n,))
    db.execute("INSERT INTO npctemplate VALUES ('orc centurion', 'roamer', '', 'roamer-seed')")
    db.execute("INSERT INTO balancecon VALUES (0, 100, 100, 0, 5, 'even', 'seed')")
    db.execute("INSERT INTO balancecon VALUES (1, 147, 100, 0, 5, '', 'seed')")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {tb: sorted(db.execute(f"SELECT * FROM {tb}").fetchall(), key=repr) for tb in TABLES}
    db.close()
    return out


def run(*args):
    buf, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(err):
        code = t.main(list(args))
    return code, buf.getvalue() + err.getvalue()


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_dry_run_writes_nothing(self):
        before = dump(self.db)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("27 rows would change", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_once_then_undo(self):
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT itemname, levelreq, classuse, equipslot, AC, HPMAX, sta, model, color, lore, trade "
                                "FROM itempattern WHERE patternid = ?", (t.NEW_ROBE,)),
                         [(t.NEW_ROBE_NAME, 60, 13, 2, 120, 180, 24, 0, 65535, 1, 1)])
        self.assertEqual(self.q("SELECT count(*) FROM petstats WHERE melee_pct = 200"), [(len(t.ENC_PETS),)])
        self.assertEqual(self.q("SELECT melee_pct FROM petstats WHERE spellid = 700011"), [(100,)])
        self.assertEqual(self.q("SELECT hp_pct, melee_pct, special FROM classtemplates WHERE name = 'epic parathior'"),
                         [(2732, 150, "Critical Hit")])
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = 'tolo grimsteel'"),
                         [("epic the unnamed king's companions",)])
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = 'orc centurion'"), [("roamer",)])
        self.assertEqual(self.q("SELECT diff, damage_pct, root_break_pct FROM balancecon WHERE diff < 0 ORDER BY diff"),
                         [(-5, 50.0, 5.0), (-4, 60.0, 5.0), (-3, 70.0, 5.0), (-2, 80.0, 5.0), (-1, 90.0, 5.0)])
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE patternid = ?", (t.SAPPHIRE,)), [(t.DARK_GEM,)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(27,)])
        after = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied already", out)
        self.assertEqual(dump(self.db), after)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        fresh = os.path.join(self.dir.name, "fresh.db")
        make(fresh)
        db = sqlite3.connect(fresh)
        db.execute("ALTER TABLE petstats ADD COLUMN melee_pct INTEGER NOT NULL DEFAULT 100")  # the column stays
        db.commit()
        db.close()
        self.assertEqual(dump(self.db), dump(fresh))
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (t.LOG,)), [(0,)])

    def test_a_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npctemplate SET template = 'tank' WHERE name = 'parathior'")
        db.commit()
        db.close()
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("template reads 'tank'", out)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
