#!/usr/bin/env python3
"""tools/lore_library_20261121.py on a small database: the texts against a PS2's rules, apply, a second apply, undo
exactly, refusals by name, and a town with no faction skipped."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lore_library_20261121 as lore  # noqa: E402

SCHEMA = """
CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, town TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '', mood TEXT NOT NULL DEFAULT '', race INTEGER NOT NULL DEFAULT -1, line TEXT NOT NULL,
  weight INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm', sex TEXT NOT NULL DEFAULT '',
  speaker_race INTEGER NOT NULL DEFAULT -1, away INTEGER NOT NULL DEFAULT 0, class INTEGER NOT NULL DEFAULT -1,
  not_race INTEGER NOT NULL DEFAULT -1);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL,
  z_coord REAL, facing INTEGER, world INTEGER, modelid INTEGER, size REAL);
CREATE TABLE signtext (npcid INTEGER PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'gm', text TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'gm', note TEXT NOT NULL DEFAULT '');
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
  giver TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
  classes TEXT NOT NULL DEFAULT '', races TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '',
  repeatable INTEGER NOT NULL DEFAULT 0, offer TEXT NOT NULL DEFAULT '', accept TEXT NOT NULL DEFAULT '',
  decline TEXT NOT NULL DEFAULT '', declined TEXT NOT NULL DEFAULT '', started TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
  target TEXT NOT NULL DEFAULT '', journal TEXT NOT NULL DEFAULT '', answer TEXT NOT NULL DEFAULT '',
  says TEXT NOT NULL DEFAULT '', factions TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
  faction TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
  ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
"""

DATA = {
    "lines": [{"town": "Halas", "src": "t389", "lines": ["The bears come to look, not to kill.", "Benno was tall."]},
              {"town": "Nowhere", "src": "t0", "lines": ["A line for a town that is not here."]}],
    "books": [{"near": "Elder Joram", "src": "t389", "pages": [["Blood, I", "Page one."], ["Blood, II", "Page two."]]}],
    "tales": [{"name": "Tales of the North", "giver": "Elder Joram", "faction": "Citizens of Halas", "src": "t389",
               "offer": "Sit by the fire. Will you hear it?", "parts": ["One.", "Two.", "Three."]}],
}


def fixture(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
    db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, modelid, size) "
               "VALUES ('sign post', 0, 25335.0, 54.1, 15700.0, 0, 0, ?, 1.0)", (lore.SIGN_BODY,))
    db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, modelid, size) "
               "VALUES ('Elder Joram', 326, 13460.0, 56.3, 4305.0, 193, 0, 1640644319, 1.0)")
    db.execute("INSERT INTO npcfaction (name, faction) VALUES ('elder joram', 'Citizens of Halas')")
    db.execute("INSERT INTO factions VALUES ('Citizens of Halas', 0)")
    db.execute("INSERT INTO npcdialog (town, line) VALUES ('Halas', 'Benno was tall.')")
    db.execute("INSERT INTO quests (questid, name) VALUES (100500, 'Older')")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f'SELECT * FROM "{t}" ORDER BY 1').fetchall()
           for t in ("npcdialog", "npcs", "signtext", "quests", "queststeps")}
    db.close()
    return out


def run(*args):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        code = lore.main(list(args))
    return code, buf.getvalue()


class LoreLibraryTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        self.data = os.path.join(self.dir.name, "data.json")
        with open(self.data, "w") as f:
            json.dump(DATA, f)
        fixture(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_shipped_texts_keep_the_rules(self):
        data = lore.load()
        self.assertEqual(lore.check_texts(data), [])
        self.assertGreaterEqual(sum(len(b["lines"]) for b in data["lines"]), 75)
        self.assertGreaterEqual(len(data["tales"]), 4)
        self.assertLessEqual(len(data["tales"]), 6)

    def test_rules(self):
        self.assertTrue(lore.problems("a ’quote’", 100, "x"))
        self.assertTrue(lore.problems("a |x run", 100, "x"))
        self.assertTrue(lore.problems("50% off", 100, "x"))
        self.assertTrue(lore.problems("off to Kelethin", 100, "x"))
        self.assertTrue(lore.problems("x" * 191, lore.LINE_MAX, "x"))
        self.assertFalse(lore.problems("The Kerran of the plains.", 100, "x"))

    def test_report_only_writes_nothing(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data)
        self.assertEqual(code, 0, out)
        self.assertIn("Nowhere", out)
        self.assertIn("already said", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_again_undo(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM npcdialog WHERE source = 'lore-1121'").fetchone()[0], 1)
        signs = db.execute("SELECT n.npc_name, n.modelid, n.world, n.x_coord, s.kind, s.text FROM npcs n "
                           "JOIN signtext s ON s.npcid = n.id ORDER BY n.id").fetchall()
        self.assertEqual([s[0] for s in signs], ["Blood, I", "Blood, II"])
        self.assertTrue(all(s[1] == lore.SIGN_BODY and s[2] == 0 and s[4] == "place" for s in signs))
        self.assertEqual([s[3] for s in signs], [13460.0 + lore.SIGN_STEP, 13460.0 + 2 * lore.SIGN_STEP])
        q = db.execute("SELECT questid, giver, enabled, offer FROM quests WHERE name = 'Tales of the North'").fetchone()
        self.assertEqual(q[:3], (100501, "Elder Joram", 1))
        self.assertEqual(q[3], "Sit by the fire.\nWill you hear it?")
        steps = db.execute("SELECT step, kind, target, answer, factions FROM queststeps WHERE questid = ? ORDER BY step",
                           (q[0],)).fetchall()
        self.assertEqual(steps, [(1, "talk", "Elder Joram", "", ""), (2, "talk", "Elder Joram", "Go on.", ""),
                                 (3, "talk", "Elder Joram", "Go on.", "Citizens of Halas:+250")])
        audits = db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        self.assertEqual(audits, 1 + 4 + 1 + 3)
        db.close()
        after = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertIn("applied already", out)
        self.assertEqual(dump(self.db), after)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertIn("9 rows put back, 0 left", out)
        self.assertEqual(dump(self.db), before)

    def test_undo_leaves_a_changed_row(self):
        run("--db", self.db, "--data", self.data, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE signtext SET text = 'a GM wrote this' WHERE text = 'Page one.'")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--undo")
        self.assertIn("changed since", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT text FROM signtext").fetchall(), [("a GM wrote this",)])
        db.close()

    def test_refused_by_name(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO npcs (npc_name, world) VALUES ('Elder Joram', 1)")
        db.commit()
        db.close()
        before = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertEqual(code, 1)
        self.assertIn("2 rows of 'Elder Joram'", out)
        self.assertEqual(dump(self.db), before)

    def test_refused_when_the_faction_or_name_is_wrong(self):
        db = sqlite3.connect(self.db)
        db.execute("DELETE FROM factions")
        db.execute("INSERT INTO quests (questid, name) VALUES (100600, 'Tales of the North')")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertEqual(code, 1)
        self.assertIn("no faction 'Citizens of Halas'", out)
        self.assertIn("is there already", out)


if __name__ == "__main__":
    unittest.main()
