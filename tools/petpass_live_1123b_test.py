#!/usr/bin/env python3
"""Tests for petpass_live_1123b.py on a small database holding the rows as read on Live: a dry run writes nothing;
--apply writes each change once (a second --apply nothing); a row not as read refuses the whole run; --undo puts
every row back (the deleted respawn rows too) and drops the log."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_live_1123b as t  # noqa: E402

TABLES = ("itempattern", "itemeffect", "quests", "queststeps", "npcs", "npcrespawn")


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemicon INTEGER, equipslot INTEGER,
            classuse INTEGER);
        CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
            icon INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, world INTEGER, x REAL, z REAL,
            PRIMARY KEY (questid, step));
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, npc_level INTEGER);
        CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
    """)
    pid = 1
    def item(name, icon, slot=-1, mask=16384):
        nonlocal pid
        pid += 1
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)", (pid, name, icon, slot, mask))
        return pid
    item("Ianant's Chalice", t.GEM)
    item("Klik'Anon Cocktail", -607649328)  # its scroll: not a thrown potion
    for name in t.STRIKE:
        db.execute("INSERT INTO itemeffect (patternid, line) VALUES (?, 'Target, Decrease Hitpoints by 100')",
                   (item(name, t.JUG, 16),))
    for name in t.OVER_TIME:
        db.execute("INSERT INTO itemeffect (patternid, line) VALUES (?, 'Target, Decrease HP per tic by 9')",
                   (item(name, t.JUG, 16),))
    for name in t.BARKS + t.ROOT_NAMES:
        item(name, t.VIAL)
    for name in t.ILLUSION_ITEMS:
        db.execute("INSERT INTO itemeffect VALUES (?, 'Self, Illusion: Elf, Race Faction, 10min', 'Illusion: Elf', 0)",
                   (item(name, -607649328),))
    for i, (_, qname, giver, step, kind, target, _) in enumerate(t.PLACES):
        db.execute("INSERT INTO quests VALUES (?, ?, ?)", (10 + i, qname, giver))
        db.execute("INSERT INTO queststeps VALUES (?, ?, ?, ?, 0, 0, 0)", (10 + i, step, kind, target))
    db.execute("INSERT INTO npcs VALUES (1, 'the lost froglok', 0, 40)")
    for i, name in enumerate(t.GLINALDAL * 2):
        db.execute("INSERT INTO npcs VALUES (?, ?, 0, 35)", (100 + i, name))
        db.execute("INSERT INTO npcrespawn VALUES (?, 90, 'named', 'petfix259')", (100 + i,))
    db.execute("INSERT INTO npcs VALUES (200, 'a named one', 0, 35)")
    db.execute("INSERT INTO npcrespawn VALUES (200, 90, 'named', 'petfix259')")
    db.commit()
    db.close()


def run(*args):
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        code = t.main(list(args))
    return code, out.getvalue() + err.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    got = {n: sorted(db.execute(f"SELECT * FROM {n}").fetchall()) for n in TABLES}
    db.close()
    return got


class PetpassLive1123b(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *a):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_apply_once_and_undo(self):
        was = dump(self.db)
        n = 1 + len(t.STRIKE) + len(t.OVER_TIME) + len(t.BARKS) + len(t.ROOT_NAMES) + 2 + 4 + 1 + 2
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertIn(f"{n} rows would change", out)
        self.assertEqual(dump(self.db), was)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE itemname = ?", "Ianant's Chalice"), [(t.CHALICE,)])
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE itemname = 'Icy Potion'"), [(t.BLUE,)])
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE itemname = 'Septic Potion'"), [(t.VIAL,)])
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE itemname = ? AND equipslot = -1",
                                "Klik'Anon Cocktail"), [(-607649328,)])
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE itemname = 'Fine Treant Bark'"), [(t.BARK,)])
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE itemname = 'Fresh Treant Root'"), [(t.ROOTS,)])
        self.assertEqual(self.q("SELECT world, x, z FROM queststeps WHERE target = 'hobgoblin vandal'"), [(1, 8250.0, 5850.0)])
        self.assertEqual(self.q("SELECT npc_row FROM npcrespawn"), [(200,)])
        self.assertEqual(self.q("SELECT npc_level FROM npcs WHERE id = 1"), [(45,)])
        self.assertEqual(self.q("SELECT DISTINCT icon FROM itemeffect WHERE name = 'Illusion: Elf'"), [(t.ILLUSION_ICON,)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(n,)])
        code, out = run("--db", self.db, "--apply")
        self.assertIn("applied already", out)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), was)
        self.assertEqual(self.q(f"SELECT name FROM sqlite_master WHERE name = '{t.LOG}'"), [])

    def test_a_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET npc_level = 41 WHERE id = 1")
        db.commit()
        db.close()
        was = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 2)
        self.assertIn("refused", out)
        self.assertEqual(dump(self.db), was)


if __name__ == "__main__":
    unittest.main()
