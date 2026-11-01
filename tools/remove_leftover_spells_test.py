"""tools/remove_leftover_spells.py on a database of its own: the leftovers out
of their class lists, the books and the quest choice that gives one, the
other spells, options and records left, a second run nothing, --undo the
tables as they were; a spell whose record is another refused (the control)."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "remove_leftover_spells.py")
SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE characters (charName TEXT UNIQUE NOT NULL, serverid INTEGER PRIMARY KEY AUTOINCREMENT, level INTEGER, tclass INTEGER);
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL, addedorder INTEGER NOT NULL,
    onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL, showhide INTEGER NOT NULL);
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, choices TEXT NOT NULL DEFAULT '');
"""
EPIC = {"ask": "Which ability do you take?", "options": [
    {"answer": "Ahrianna's Generous Gift", "says": "It is yours.", "spells": "701097"},
    {"answer": "Eldritch Rune", "says": "It is yours.", "spells": "701132"},
    {"answer": "Jen`Tylis' Lust for Power", "says": "It is yours.", "spells": "701164"}]}


def compact(o):
    return json.dumps(o, separators=(",", ":"), ensure_ascii=False)


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT rowid, * FROM {t} ORDER BY rowid").fetchall()
           for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")}
    out["sqlite_sequence"] = db.execute("SELECT * FROM sqlite_sequence ORDER BY name").fetchall()
    db.close()
    return out


class RemoveLeftovers(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO spellpattern VALUES (?, ?)", [
            (701064, "Nature Form"), (701258, "Wild Nature"), (701107, "Bixie Form"), (700670, "Sacred Spring"),
            (700643, "Savage Healing"), (700654, "Tribal Healing"), (700672, "Spiritform"), (701132, "Eldritch Rune"),
            (900418, "Spirit Form"), (452, "Spirit of the Wolf"), (701097, "Ahrianna's Generous Gift"), (701164, "Jen`Tylis' Lust for Power")])
        db.executemany("INSERT INTO classspells VALUES (?, ?, ?)", [
            (7, 701064, "eqoa-data cm"), (7, 701258, "eqoa-data master Wilding"), (7, 701107, "eqoa-data cm"), (7, 700670, "eqoa-data"),
            (8, 700643, "eqoa-data"), (8, 700654, "eqoa-data"), (8, 700672, "eqoa-data"), (12, 701132, "eqoa-data epic"),
            (7, 900418, "cm (eqoaemu)"), (7, 452, "dump"), (8, 452, "dump"), (12, 701097, "eqoa-data epic")])
        db.executemany("INSERT INTO characters (charName, level, tclass) VALUES (?, ?, ?)", [("Oakleaf", 60, 7), ("Muddy", 55, 8), ("Mindy", 60, 12)])
        db.executemany("INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?, ?, ?, ?, ?, 0, 0)",
                       [(1, 452, 0, 1, 2), (1, 701258, 1, 1, 3), (1, 700670, 2, 0, 0), (2, 700672, 0, 0, 0), (2, 452, 1, 0, 0),
                        (3, 701132, 0, 0, 0), (3, 701097, 1, 0, 0)])
        db.execute("INSERT INTO queststeps VALUES (101122, 1, ?)", (compact(EPIC),))
        db.execute("INSERT INTO queststeps VALUES (101122, 2, '')")
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        return r.stdout

    def test_apply_again_and_undo(self):
        before = dump(self.db)
        out = self.run_tool("--apply")
        self.assertIn("changed 13 rows", out)  # 8 class rows, 4 books, 1 choice
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT class, spellid FROM classspells ORDER BY class, spellid").fetchall(),
                         [(7, 452), (7, 900418), (8, 452), (12, 701097)])
        self.assertEqual(db.execute("SELECT serverid, spellid FROM spells ORDER BY id").fetchall(), [(1, 452), (2, 452), (3, 701097)])
        self.assertEqual(db.execute("SELECT count(*) FROM spellpattern").fetchone()[0], 12, "the records stay")
        want = dict(EPIC, options=[EPIC["options"][0], EPIC["options"][2]])
        self.assertEqual(db.execute("SELECT choices FROM queststeps WHERE step = 1").fetchone()[0], compact(want))
        db.close()
        self.assertIn("changed 0 rows", self.run_tool("--apply"))
        self.assertIn("put back 13 rows, 0 left", self.run_tool("--undo"))
        self.assertEqual(dump(self.db), before)

    def test_a_record_not_the_one_named_is_refused(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE spellpattern SET spellname = 'Eldritch Runes' WHERE spellid = 701132")
        db.commit()
        db.close()
        out = self.run_tool("--apply")
        self.assertIn("refused: 701132", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM classspells WHERE spellid = 701132").fetchone()[0], 1)
        self.assertEqual(db.execute("SELECT choices FROM queststeps WHERE step = 1").fetchone()[0], compact(EPIC))
        db.close()


if __name__ == "__main__":
    unittest.main()
