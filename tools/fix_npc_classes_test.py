#!/usr/bin/env python3
"""tools/fix_npc_classes.py against a made-up database: a few npcs names, an
npcclass with Return Home's and Sandstorm's rows, and a GM's edit in
gm_audit. No game data.

    python3 tools/fix_npc_classes_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_npc_classes as fc  # noqa: E402

TOOL = os.path.join(HERE, "fix_npc_classes.py")


class TheNameSaysTheClass(unittest.TestCase):
    def test_words(self):
        cases = {
            "lord nagafen": 0,           # no caster's word: a warrior
            "crag spider": 0,
            "kappa warrior": 0,
            "orc shaman": 8,
            "goblin witch doctor": 8,    # the longer word at the end wins over "witch"
            "kobold augur": 8,
            "arch bishop vel": 9,        # a title before a name
            "priestess nadya": 9,
            "magus of the stars": 13,    # the head before "of"
            "fire giant magus": 13,
            "kappa defiler": 11,
            "lich overlord": 11,
            "crag wisp": 10,             # Sandstorm's records: wisps are magicians
            "fire elementalkin": 10,
            "chemist aishi": 14,
            "paladin freeland": 2,
            "deathknight halik": 3,
            "druidic cadaver": 0,        # the head is a corpse, the word only says whose
            "mage hunter": 0,
            "corliss magemell": 0,       # not a whole word
            "helda suncharmer": 0,
            "merchant samoul": 0,
            "Goblin Shaman$": 8,         # keyed as NPCClassKey keys it
        }
        for name, want in cases.items():
            got, _ = fc.class_by_name(name)
            self.assertEqual(got, want, f"{name!r}: {got}, want {want}")


def make_db(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
        CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '');
    """)
    names = ["Lord Nagafen", "Orc Shaman", "Orc Shaman", "Crag Wisp", "Merchant Samoul", "Kappa Warrior",
             "Gnoll Seer", "Gnoll Pup", "Sorceress X`lottl", "Guard Findel"]
    db.executemany("INSERT INTO npcs (npc_name) VALUES (?)", [(n,) for n in names])
    db.executemany("INSERT INTO npcclass VALUES (?, ?, ?)", [
        ("lord nagafen", 10, "return home"),     # -> warrior
        ("orc shaman", 11, "return home"),       # -> shaman
        ("merchant samoul", 5, "return home"),   # -> warrior
        ("kappa warrior", 0, "return home"),     # already right: untouched
        ("gnoll seer", 13, "return home"),       # a GM's (gm_audit): untouched
        ("crag wisp", 10, "sandstorm"),          # Sandstorm's: untouched
        ("guard findel", 3, "sandstorm"),
    ])
    db.execute("""INSERT INTO gm_audit (tbl, rowkey) VALUES ('npcclass', '{"name":"gnoll seer"}')""")
    db.commit()
    return db


def dump(db):
    return sorted(db.execute("SELECT name, class, source FROM npcclass"))


def run(*args):
    return subprocess.run([sys.executable, "-I", TOOL, *args], capture_output=True, text=True, check=True).stdout


class ApplyAndUndo(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        self.db = make_db(self.path)
        self.before = dump(self.db)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def test_report_only_writes_nothing(self):
        out = run("--db", self.path)
        self.assertIn("4 names to write", out)
        self.assertEqual(dump(self.db), self.before)

    def test_apply_then_undo(self):
        run("--db", self.path, "--apply", "--no-backup")
        rows = {n: (c, s) for n, c, s in dump(self.db)}
        self.assertEqual(rows["lord nagafen"], (0, "by name"))
        self.assertEqual(rows["orc shaman"], (8, "by name"))
        self.assertEqual(rows["merchant samoul"], (0, "by name"))
        self.assertEqual(rows["sorceress x`lottl"], (13, "by name"), "a caster-named creature with no row gets one")
        self.assertNotIn("gnoll pup", rows, "a warrior's name gets no row: none is a warrior's already")
        self.assertEqual(rows["kappa warrior"], (0, "return home"))
        self.assertEqual(rows["gnoll seer"], (13, "return home"), "a GM's row is left as it is")
        self.assertEqual(rows["crag wisp"], (10, "sandstorm"))
        self.assertEqual(rows["guard findel"], (3, "sandstorm"))

        again = run("--db", self.path, "--apply", "--no-backup")
        self.assertIn("wrote 0 rows", again, "a second run finds nothing")

        run("--db", self.path, "--undo", "--no-backup")
        self.assertEqual(dump(self.db), self.before, "the undo puts every row back exactly")
        self.assertEqual(self.db.execute("SELECT COUNT(*) FROM npcclass_fix_log").fetchone()[0], 0)

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.path, "--apply", "--no-backup")
        self.db.execute("UPDATE npcclass SET class = 1, source = 'gm' WHERE name = 'lord nagafen'")
        self.db.commit()
        out = run("--db", self.path, "--undo", "--no-backup")
        self.assertIn("left: 'lord nagafen'", out)
        rows = {n: (c, s) for n, c, s in dump(self.db)}
        self.assertEqual(rows["lord nagafen"], (1, "gm"), "the GM's change since stays")
        self.assertEqual(rows["orc shaman"], (11, "return home"), "the rest are put back")
        self.assertNotIn("sorceress x`lottl", rows)


if __name__ == "__main__":
    unittest.main()
