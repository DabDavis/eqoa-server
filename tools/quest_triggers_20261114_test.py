#!/usr/bin/env python3
"""quest_triggers_20261114.py on a small database: the report writes nothing; --apply takes Lore off the quest items,
adds the npcaggro rows where the NPC stands and none is there, and renames the rogues' scroll where the spell is
"Shadow Strike"; a second apply does nothing; --undo puts back exactly what was there. The controls: a GM's aggressive
rule is refused (nothing written); an item renamed since is refused; a shard whose spell keeps "Shadowstrike" keeps
the scroll's name."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "quest_triggers_20261114.py")


def make(path, spell="Shadow Strike", gertrude_rule=None, pelt="shadow cub pelt"):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, lore INTEGER);
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
        CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
            note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', leash REAL NOT NULL DEFAULT 0,
            holding INTEGER NOT NULL DEFAULT 0, see_invis INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO itempattern VALUES (700507, 'triad stone', '', 1);
        INSERT INTO itempattern VALUES (701283, 'Gust Chime', '', 0);
        INSERT INTO itempattern VALUES (8541, 'Shadowstrike', 'This allows you to scribe Shadowstrike into your spell book.', 0);
        INSERT INTO npcs VALUES (117980, 'Gertrude');
        INSERT INTO npcs VALUES (1346, 'ghostly mage');
    """)
    db.execute("INSERT INTO itempattern VALUES (700187, ?, '', 1)", (pelt,))
    db.execute("INSERT INTO spellpattern VALUES (700049, ?)", (spell,))
    if gertrude_rule is not None:
        db.execute("INSERT INTO npcaggro (npc_name, aggressive) VALUES ('Gertrude', ?)", (gertrude_rule,))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        return tuple(db.execute(q).fetchall() for q in (
            "SELECT * FROM itempattern ORDER BY patternid", "SELECT * FROM npcaggro ORDER BY npc_name",
            "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"))
    finally:
        db.close()


class QuestTriggersTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db)
        before = state(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only", r.stdout)
        self.assertEqual(state(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        lore = dict(db.execute("SELECT patternid, lore FROM itempattern"))
        self.assertEqual((lore[700187], lore[700507], lore[701283]), (0, 0, 0))
        self.assertEqual(db.execute("SELECT itemname FROM itempattern WHERE patternid = 8541").fetchone()[0], "Shadow Strike")
        rules = dict(db.execute("SELECT npc_name, aggressive FROM npcaggro"))
        self.assertEqual(rules, {"Gertrude": 0, "ghostly mage": 0})
        self.assertGreater(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 0)
        db.close()
        r = run(self.db, "--apply")
        self.assertIn("applied already", r.stdout)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.db), before)

    def test_refusals_and_skips(self):
        make(self.db, gertrude_rule=1)
        before = state(self.db)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertIn("a GM's rule", r.stdout)
        self.assertEqual(state(self.db), before)

        os.remove(self.db)
        make(self.db, pelt="a pelt renamed")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertEqual(state(self.db), state(self.db))

        os.remove(self.db)
        make(self.db, spell="Shadowstrike")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT itemname FROM itempattern WHERE patternid = 8541").fetchone()[0], "Shadowstrike")
        db.close()


if __name__ == "__main__":
    unittest.main()
