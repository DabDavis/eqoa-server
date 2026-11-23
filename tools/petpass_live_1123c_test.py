#!/usr/bin/env python3
"""Tests for petpass_live_1123c.py on a small database holding the camp as read on Live: a dry run writes nothing;
--apply writes each change once (a second --apply nothing), the clerics copies of the nearest paladin; a row not as
read refuses the whole run; --undo puts every table back (the npcs sequence too) and drops the log."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_live_1123c as t  # noqa: E402

TABLES = ("npcs", "classtemplates", "npctemplate", "npcsocial", "npcclass", "quests", "queststeps",
          "queststeptargets", "itempattern", "loottables", "sqlite_sequence")
PALADINS = [(122550, 12500.0, 98.7188, 15300.0, 157), (122551, 12458.3164, 55.5049, 15221.3672, 124),
            (122552, 12473.8438, 66.4746, 15268.3945, 206), (122553, 12437.9424, 57.9426, 15235.2334, 150),
            (196496, 12443.4893, 54.126, 15200.5283, 57), (196497, 12437.374, 54.126, 15199.2422, 182),
            (196498, 12441.0625, 54.126, 15196.4961, 123), (196499, 12457.7754, 56.2368, 15224.5371, 251)]


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL,
            z_coord REAL, facing INTEGER, world INTEGER, hp INTEGER, modelid INTEGER, torso INTEGER, forearm INTEGER,
            gloves INTEGER, legs INTEGER, feet INTEGER, head INTEGER, robe_style TEXT, npc_level INTEGER);
        CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL
            DEFAULT 100, taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100,
            abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL
            DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcsocial (npc_name TEXT PRIMARY KEY, social INTEGER NOT NULL DEFAULT 1, faction TEXT NOT NULL
            DEFAULT '', radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, notes TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT,
            rewarditems TEXT NOT NULL DEFAULT '', chance REAL, journal TEXT, PRIMARY KEY (questid, step));
        CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
            PRIMARY KEY (questid, step, target));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT);
        CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER);
    """)
    for i, x, y, z, f in PALADINS:
        db.execute("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, 0, 1, 1893243078, 0, 0, 0, 0, 0, 0, '0', 58)",
                   (i, t.PALADIN, x, y, z, f))
    db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, world) VALUES ('great spider', 12809, 54, "
               "14000, 0)")
    db.execute("INSERT INTO quests VALUES (?, 'Screaming Souls', ?)",
               (t.QUEST, "The zombie is Urglauh himself; " + t.NOTES_OLD + ". The faction is ours."))
    db.execute("INSERT INTO queststeps VALUES (?, 1, 'loot', ?, ?, '', ?, ?)",
               (t.QUEST, t.PALADIN, f"{t.ITEM}*10", t.CHANCE_OLD, t.JOURNAL_OLD))
    db.execute("INSERT INTO queststeps VALUES (?, 2, 'give', 'Urglauh the Foul', ?, '', 0, 'Feed them.')",
               (t.QUEST, f"{t.ITEM}*10"))
    db.execute("INSERT INTO itempattern VALUES (?, 'Screaming Soul', ?)", (t.ITEM, t.DESC_OLD))
    db.execute("INSERT INTO loottables (tablename, patternid) VALUES ('generic', 5)")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {tb: db.execute(f"SELECT * FROM {tb} ORDER BY 1, 2").fetchall() for tb in TABLES}
    out["sqlite_sequence"] = [r for r in out["sqlite_sequence"] if r[0] != "gm_audit"]  # the audit lines stay
    db.close()
    return out


def run(*argv):
    buf, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(err):
        code = t.main(list(argv))
    return code, buf.getvalue() + err.getvalue()


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "db.sqlite")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_dry_run_writes_nothing(self):
        before = dump(self.path)
        code, out = run("--db", self.path)
        self.assertEqual(code, 0, out)
        self.assertIn("dry run: 23 rows would change", out)
        self.assertEqual(dump(self.path), before)

    def test_apply_then_undo(self):
        before = dump(self.path)
        code, out = run("--db", self.path, "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q(f"SELECT count(*) FROM npcs WHERE npc_name = ? AND torso = 4 AND head = 4 AND "
                                f"robe_style = '255'", (t.PALADIN,)), [(8,)])
        clerics = self.q("SELECT x_coord, z_coord, facing, robe_style, torso, npc_level, modelid FROM npcs WHERE "
                         "npc_name = ? ORDER BY id", (t.CLERIC,))
        self.assertEqual([(x, z) for x, _, z in t.CLERIC_SPOTS], [c[:2] for c in clerics])
        self.assertEqual([c[2] for c in clerics], [182, 251, 150, 206])  # the nearest paladin's facing
        self.assertTrue(all(c[3:] == ("0", 0, 58, 1893243078) for c in clerics))
        self.assertEqual(self.q("SELECT template FROM npctemplate ORDER BY name"),
                         [("rodcet nife cleric",), ("rodcet nife paladin",)])
        self.assertEqual(self.q("SELECT hp_pct, class FROM classtemplates WHERE name = 'rodcet nife paladin'"),
                         [(130, -1)])
        self.assertEqual(self.q("SELECT social, faction FROM npcsocial"), [(0, t.FACTION)] * 2)
        self.assertEqual(self.q("SELECT chance, journal FROM queststeps WHERE step = 1"),
                         [(0.35, t.JOURNAL_NEW)])
        self.assertEqual(self.q("SELECT target FROM queststeptargets"), [(t.CLERIC,)])
        self.assertEqual(self.q("SELECT itemdesc FROM itempattern"), [(t.DESC_NEW,)])
        self.assertIn(t.NOTES_NEW, self.q("SELECT notes FROM quests")[0][0])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(23,)])
        code, out = run("--db", self.path, "--apply")
        self.assertIn("applied already", out)
        code, out = run("--db", self.path, "--undo")
        self.assertEqual(code, 0, out)
        self.assertIn("npcs sequence back", out)
        self.assertEqual(dump(self.path), before)
        self.assertEqual(self.q(f"SELECT name FROM sqlite_master WHERE name = '{t.LOG}'"), [])

    def test_refuses_a_row_not_as_read(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET torso = 3 WHERE id = 122550")
        db.commit()
        db.close()
        before = dump(self.path)
        code, out = run("--db", self.path, "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("refused", out)
        self.assertEqual(dump(self.path), before)

    def test_refuses_another_source_of_the_skull(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO loottables (tablename, patternid) VALUES ('boss x', ?)", (t.ITEM,))
        db.commit()
        db.close()
        code, out = run("--db", self.path, "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("other sources", out)

    def test_refuses_a_missing_paladin(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM npcs WHERE id = 122550")
        db.commit()
        db.close()
        code, out = run("--db", self.path)
        self.assertEqual(code, 2, out)


if __name__ == "__main__":
    unittest.main()
