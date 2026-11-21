#!/usr/bin/env python3
"""tools/guide_quests_20261121.py on two small made databases (one shaped as the shard with the misspelt rows, one as
the shard without): a dry run writes nothing, --apply writes each fix only where its problem is and logs it, a second
--apply writes nothing, --undo puts every table back as it was.

    python3 -E -s tools/guide_quests_20261121_test.py
"""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import guide_quests_20261121 as gq  # noqa: E402

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL, npc_type INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, races TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, npcid INTEGER DEFAULT 0, xp INTEGER DEFAULT 0,
                         spells TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '',
                         events TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT, PRIMARY KEY (questid, step, target));
CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT, enabled INTEGER DEFAULT 1, steps TEXT NOT NULL DEFAULT '');
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
CREATE TABLE signtext (npcid INTEGER PRIMARY KEY, kind TEXT, text TEXT DEFAULT '', source TEXT DEFAULT '', note TEXT DEFAULT '');
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER, radius REAL, note TEXT, source TEXT, leash REAL,
                       holding INTEGER, see_invis INTEGER, holds INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
                       ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
"""


def make(path, misspelt):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    rigo = "Rigo Darter" if misspelt else "Rigo Dartar"
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?)", [
        (105170, rigo, 0, 21077.0, 25268.0, 0),
        (101488, "Artic Spiritcaller", 0, 6000.0, 6000.0, 0),
        (120969, "Archon Seradesh", 1, 4745.0, 4812.0, 0),
        (123005, "Lord Pickclaw", 0, 16895.0, 12240.0, 0),
        (191269 if misspelt else 163618, "Bloodletter", 0, 25672.0, 3312.0, 0),
        (163616, "Trial of Celerity", 0, 25633.0, 3167.0, 128),
    ] + ([] if misspelt else [(122923, "Arctic Spiritcaller", 0, 6989.0, 5897.0, 0)]))
    db.executemany("INSERT INTO quests VALUES (?, ?, ?, ?)", [
        (100782, "Poisontongue", "", "EQOA-Data 23-47/37/rigo"),
        (100896, "Bardif's Butler", "0, 1, 3, 7, 8", "EQOA-Data 30,40,49/group-3-good/L30-a"),
        (101227, "Sundro's Enchantment", "", ""), (101120, "The Wizard's Epic Reward", "", ""),
        (101312, "The Trials of Cazic Thule", "", ""),
    ])
    # the shard with the problem names the other shard's id of the spell; the other names its own
    mine, theirs = (906614, 906733) if misspelt else (906733, 906733)
    db.execute("INSERT INTO spellpattern VALUES (?, 'Ascended Towering Mind')", (mine,))
    db.executemany("INSERT INTO queststeps (questid, step, kind, target, npcid, xp, choices) VALUES (?, ?, ?, ?, ?, ?, ?)", [
        (100782, 13, "talk", "Rigo Dartar", 0, 2000000, ""),
        (101227, 1, "loot", "Arctic Spiritcaller", 0, 0, ""),
        (101120, 1, "talk", "Ediopolus", 0, 0,
         json.dumps({"ask": "Which?", "options": [{"answer": "Ascended Towering Mind", "spells": str(theirs)}]},
                    separators=(",", ":"))),
        (101312, 4, "talk", "Trial of Celerity", 163616, 0, ""),
    ])
    db.execute("INSERT INTO signtext VALUES (163616, 'quest', '', 'petition', '')")
    db.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (237, 'The Trial of Celerity', ?)",
               (json.dumps([{"do": "spawn", "npc": "#163618"}], separators=(",", ":")),))
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = gq.main(list(args))
    return rc, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT IN ('gm_audit', 'sqlite_sequence')"):
        out[t] = sorted(db.execute(f'SELECT * FROM "{t}"').fetchall(), key=repr)
    db.close()
    return out


class GuideQuestsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.dir.cleanup)

    def db(self, misspelt):
        p = os.path.join(self.dir.name, f"{'a' if misspelt else 'b'}.db")
        make(p, misspelt)
        return p

    def test_dry_run_writes_nothing(self):
        p = self.db(True)
        before = dump(p)
        rc, out = run("--db", p)
        self.assertEqual(rc, 0)
        self.assertIn("dry run", out)
        self.assertEqual(dump(p), before)

    def test_apply_where_the_problems_are(self):
        p = self.db(True)
        rc, out = run("--db", p, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(p)
        self.assertEqual(db.execute("SELECT npc_name FROM npcs WHERE id = 105170").fetchone()[0], "Rigo Dartar")
        self.assertEqual(db.execute("SELECT target FROM queststeptargets WHERE questid = 101227").fetchone()[0],
                         "Artic Spiritcaller")
        self.assertIn("#191269", db.execute("SELECT steps FROM gmtriggers WHERE slot = 237").fetchone()[0])
        self.assertIn('"spells":"906614"', db.execute("SELECT choices FROM queststeps WHERE questid = 101120").fetchone()[0])
        self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE id = 120969").fetchone()[0], 128)
        self.assertEqual(db.execute("SELECT aggressive FROM npcaggro WHERE npc_name = 'Lord Pickclaw'").fetchone()[0], 0)
        self.assertEqual(db.execute("SELECT races FROM quests WHERE questid = 100896").fetchone()[0], "")
        self.assertEqual(db.execute("SELECT xp FROM queststeps WHERE questid = 100782 AND step = 13").fetchone()[0], 4100000)
        n = db.execute(f"SELECT count(*) FROM {gq.LOG}").fetchone()[0]
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail LIKE '%MasterVG782%'").fetchone()[0], n)
        db.close()
        rc, out = run("--db", p, "--apply")
        self.assertIn("nothing to write", out)

    def test_the_other_shard_takes_only_its_own(self):
        p = self.db(False)
        rc, out = run("--db", p, "--apply")
        db = sqlite3.connect(p)
        self.assertEqual(db.execute("SELECT count(*) FROM queststeptargets").fetchone()[0], 0)  # its Arctic row stands
        self.assertIn("#163618", db.execute("SELECT steps FROM gmtriggers").fetchone()[0])     # its row is there
        self.assertIn('"spells":"906733"', db.execute("SELECT choices FROM queststeps WHERE questid = 101120").fetchone()[0])
        db.close()

    def test_undo_is_exact(self):
        for misspelt in (True, False):
            p = self.db(misspelt)
            before = dump(p)
            run("--db", p, "--apply")
            self.assertNotEqual(dump(p), before)
            rc, out = run("--db", p, "--undo")
            self.assertIn("0 left", out)
            after = dump(p)
            after.pop(gq.LOG, None)
            self.assertEqual(after, before)

    def test_a_row_not_as_written_is_left(self):
        p = self.db(True)
        db = sqlite3.connect(p)
        db.execute("UPDATE npcs SET x_coord = 1 WHERE id = 105170")
        db.commit()
        db.close()
        rc, out = run("--db", p)
        self.assertIn("row 105170", out)
        self.assertNotIn("rename NPC row 105170", out)


if __name__ == "__main__":
    unittest.main()
