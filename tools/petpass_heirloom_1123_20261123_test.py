#!/usr/bin/env python3
"""Tests for petpass_heirloom_1123_20261123.py on a small database holding the rows as read on both shards: a dry
run writes nothing; --apply writes each change once (and a second --apply nothing); heirloom changes the code's rows
alone; a row not as read refuses the whole run; --undo puts every row back and drops the log."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_heirloom_1123_20261123 as t  # noqa: E402

FM_EFFECTS = ('{"v":1,"scope":"self","effects":[{"id":"05B7","kind":"invulnerable",'
              '"mitigation":{"pct":75,"max_stat":"agi","max_pct":764}}]}')


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, spelldesc TEXT);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT, fxdict INTEGER, source TEXT, effects TEXT);
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, offer TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT, world INTEGER,
            x REAL, z REAL, journal TEXT, PRIMARY KEY (questid, step));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
            z_coord REAL);
        CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0,
            water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
            speed REAL NOT NULL DEFAULT 0);
    """)
    db.execute("INSERT INTO spellpattern VALUES (901185, ?, ?)", ("Forest's Might", t.FM_DESC_OLD))
    db.execute("INSERT INTO spelleffect VALUES (901185, ?, 0, 'x', ?)", (t.FM_LINE_OLD, FM_EFFECTS))
    for i, (name, giver, step, target, _) in enumerate(t.RAT_STEPS):
        db.execute("INSERT INTO quests VALUES (?, ?, ?, '')", (100 + i, name, giver))
        db.execute("INSERT INTO queststeps VALUES (?, ?, 'loot', ?, '1', 0, 0, 0, '')", (100 + i, step, target))
    db.execute("INSERT INTO quests VALUES (200, 'Grave Silk', 'someone', '')")
    db.execute("INSERT INTO queststeps VALUES (200, 1, 'loot', 'grave spider', '7*3', 0, 0, 0, ?)", (t.SILK_OLD,))
    db.execute("INSERT INTO quests VALUES (300, 'Deidre IV', 'Ludvick Undogeem', ?)", (t.DEIDRE_OFFER_OLD,))
    db.execute("INSERT INTO queststeps VALUES (300, 1, 'loot', 'Deidre IV', '729', 0, 0, 0, '')")
    db.execute("INSERT INTO queststeps VALUES (300, 3, 'give', 'Ludvick Undogeem', '729, 730', 0, 0, 0, ?)",
               (t.DEIDRE_J3_OLD,))
    db.execute("INSERT INTO itempattern VALUES (729, ?, 'A cog from the clockwork Deidre IV.')", (t.COG_OLD,))
    db.execute("INSERT INTO itempattern VALUES (735, 'Shadowhand Receipt', ?)", (t.RECEIPT_OLD,))
    db.execute("INSERT INTO npcs VALUES (122506, 'Wandering Priestess', 0, 11155.9, 57.6749, 17228.2)")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
        code = t.main(list(args))
    return code, out.getvalue()


def snap(path):
    db = sqlite3.connect(path)
    out = {}
    for tbl in ("spellpattern", "spelleffect", "quests", "queststeps", "itempattern", "npcs", "npcmove"):
        out[tbl] = db.execute(f"SELECT * FROM {tbl} ORDER BY 1, 2").fetchall()
    db.close()
    return out


class PetpassTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def test_dry_run_writes_nothing(self):
        before = snap(self.path)
        code, out = run("--db", self.path, "--shard", "live")
        self.assertEqual(code, 0, out)
        self.assertIn("dry run: 14 rows", out)
        self.assertEqual(snap(self.path), before)

    def test_apply_live_then_undo(self):
        before = snap(self.path)
        code, out = run("--db", self.path, "--shard", "live", "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        self.assertIn('"all":true', db.execute("SELECT effects FROM spelleffect").fetchone()[0])
        self.assertEqual(db.execute("SELECT itemname FROM itempattern WHERE patternid = 729").fetchone()[0],
                         "Perpetual Motion Device")
        self.assertEqual(db.execute("SELECT x, z FROM queststeps WHERE questid = 101").fetchone(), t.TOWER)
        self.assertEqual(db.execute("SELECT roam, radius FROM npcmove").fetchone(), (1, 25.0))
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 14)
        db.close()
        code, out = run("--db", self.path, "--shard", "live", "--apply", "--no-backup")
        self.assertIn("applied already", out)
        code, out = run("--db", self.path, "--shard", "live", "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        after = snap(self.path)
        self.assertEqual(after, before)

    def test_heirloom_writes_the_codes_rows_alone(self):
        code, out = run("--db", self.path, "--shard", "heirloom", "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 7 rows", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT itemname FROM itempattern WHERE patternid = 729").fetchone()[0], t.COG_OLD)
        db.close()

    def test_a_row_not_as_read_refuses_all(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET itemdesc = 'changed' WHERE patternid = 735")
        db.commit()
        db.close()
        before = snap(self.path)
        code, out = run("--db", self.path, "--shard", "live", "--apply", "--no-backup")
        self.assertEqual(code, 2, out)
        self.assertIn("refused", out)
        self.assertEqual(snap(self.path), before)


if __name__ == "__main__":
    unittest.main()
