#!/usr/bin/env python3
"""jib742_20261121.py: report only by default, apply, a second apply, undo, a row changed since left alone, a row
already there not written again, an unknown faction skipped."""
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jib742_20261121 as tool  # noqa: E402


def make(path, factions=("Citizens of Baga", "People of Jextalan")):
    db = sqlite3.connect(path)
    db.execute("""CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, town TEXT NOT NULL DEFAULT '',
        role TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', mood TEXT NOT NULL DEFAULT '',
        race INTEGER NOT NULL DEFAULT -1, line TEXT NOT NULL, weight INTEGER NOT NULL DEFAULT 1,
        source TEXT NOT NULL DEFAULT 'gm', sex TEXT NOT NULL DEFAULT '', speaker_race INTEGER NOT NULL DEFAULT -1,
        away INTEGER NOT NULL DEFAULT 0, class INTEGER NOT NULL DEFAULT -1, not_race INTEGER NOT NULL DEFAULT -1)""")
    db.execute("""CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL,
        world INTEGER NOT NULL DEFAULT -1, x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0,
        radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL, note TEXT NOT NULL DEFAULT '',
        source TEXT NOT NULL DEFAULT 'gm', UNIQUE (name, world, x, z, radius))""")
    db.execute("CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER)")
    db.execute("CREATE TABLE npcs (npc_name TEXT)")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
               "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)")
    db.executemany("INSERT INTO factions VALUES (?, -500)", [(f,) for f in factions])
    db.executemany("INSERT INTO npcs VALUES (?)", [("Coachman Lothi",), ("baga shopkeeper",), ("Jextalan shopkeeper",)])
    db.execute("INSERT INTO npcfaction (name, faction) VALUES ('coachman lothi', 'Citizens of Neriak')")
    db.execute("INSERT INTO npcdialog (town, line) VALUES ('Neriak', 'A town line.')")
    db.commit()
    db.close()


def run(*argv):
    with redirect_stdout(io.StringIO()) as out:
        tool.main(list(argv))
    return out.getvalue()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "j.db")

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql).fetchall()
        finally:
            db.close()

    def state(self):
        return (self.q("SELECT name, mood, line, source FROM npcdialog ORDER BY id"),
                self.q("SELECT name, world, faction, note, source FROM npcfaction ORDER BY id"))

    def test_report_apply_again_undo(self):
        make(self.path)
        before = self.state()
        self.assertIn("5 rows to write", run("--db", self.path))
        self.assertEqual(self.state(), before, "report only wrote")
        run("--db", self.path, "--apply")
        dialog, faction = self.state()
        self.assertEqual(len(dialog), 4)
        self.assertIn(("Coachman Zuggug", "hostile", "Gah! You be evil! Go way! Me eat you if you don't!", "video-audit"), dialog)
        self.assertIn(("baga shopkeeper", -1, "Citizens of Baga", "filmed refusal (video audit)", "video-audit"), faction)
        self.assertIn(("jextalan shopkeeper", -1, "People of Jextalan", "filmed refusal (video audit)", "video-audit"), faction)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit WHERE action = 'insert'")[0][0], 5)
        self.assertIn("0 rows to write", run("--db", self.path, "--apply"))
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 5)
        run("--db", self.path, "--undo")
        self.assertEqual(self.state(), before)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit WHERE action = 'delete'")[0][0], 5)
        self.assertEqual(self.q(f"SELECT count(*) FROM sqlite_master WHERE name = '{tool.LOG}'")[0][0], 0)

    def test_undo_leaves_a_changed_row(self):
        make(self.path)
        run("--db", self.path, "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcdialog SET weight = 3 WHERE name = 'Coachman Lothi'")
        db.commit()
        db.close()
        self.assertIn("1 left", run("--db", self.path, "--undo"))
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog WHERE source = 'video-audit'")[0][0], 1)
        self.assertEqual(self.q("SELECT count(*) FROM npcfaction WHERE source = 'video-audit'")[0][0], 0)

    def test_unknown_faction_skipped(self):
        make(self.path, factions=("Citizens of Baga",))
        out = run("--db", self.path, "--apply")
        self.assertIn("is no faction here", out)
        self.assertEqual(self.q("SELECT name FROM npcfaction WHERE source = 'video-audit'"), [("baga shopkeeper",)])


if __name__ == "__main__":
    unittest.main()
