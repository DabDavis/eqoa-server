#!/usr/bin/env python3
"""recast_group_20261113.py on a small database: the report writes nothing; --apply puts Wolf's Instinct and Hunter's
Instinct in group "ranger instinct" (making the table when there is none); a second apply does nothing; --undo puts
every table back exactly (the table dropped again when the tool made it, kept when the server had). The controls: a
spell already in another group, or a recast not 1800, is refused, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "recast_group_20261113.py")

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER, recast INTEGER);
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, parent INTEGER, add_spells TEXT, remove_spells TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
    ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
INSERT INTO spellpattern VALUES (701068, 'Wolf''s Instinct', 51, 1800), (760110, 'Hunter''s Instinct', 51, 1800);
INSERT INTO cmcatalog VALUES (3371, 0, '701068', ''), (900645, 3371, '760110', '');
"""
SERVER_TABLE = "CREATE TABLE spellrecastgroup (spellid INTEGER PRIMARY KEY, grp TEXT NOT NULL);"


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l]
    db.close()
    return out


class RecastGroup(unittest.TestCase):
    def make(self, extra=""):
        self.dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.dir.cleanup)
        path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(path)
        db.executescript(SCHEMA + extra)
        db.commit()
        db.close()
        return path

    def cycle(self, path):
        before = dump(path)
        r = run(path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(path), before, "the report wrote")
        r = run(path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(path)
        got = db.execute("SELECT spellid, grp FROM spellrecastgroup ORDER BY spellid").fetchall()
        audits = db.execute("SELECT COUNT(*) FROM gm_audit WHERE tbl = 'spellrecastgroup'").fetchone()[0]
        db.close()
        self.assertEqual(got, [(701068, "ranger instinct"), (760110, "ranger instinct")])
        self.assertEqual(audits, 2)
        r = run(path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("applied already", r.stdout)
        r = run(path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(path), before, "undo was not exact")

    def test_no_table_yet(self):
        self.cycle(self.make())

    def test_server_made_the_table(self):
        path = self.make(SERVER_TABLE)
        self.cycle(path)
        db = sqlite3.connect(path)
        self.assertIsNotNone(db.execute("SELECT 1 FROM sqlite_master WHERE name = 'spellrecastgroup'").fetchone())
        db.close()

    def test_refusals(self):
        for extra in (SERVER_TABLE + "INSERT INTO spellrecastgroup VALUES (760110, 'other');",
                      "UPDATE spellpattern SET recast = 60 WHERE spellid = 701068;",
                      "UPDATE cmcatalog SET remove_spells = '701068' WHERE id = 900645;"):
            path = self.make(extra)
            before = dump(path)
            r = run(path, "--apply")
            self.assertEqual(r.returncode, 1, extra + r.stdout + r.stderr)
            self.assertIn("refused", r.stdout)
            self.assertEqual(dump(path), before, "a refusal wrote")


if __name__ == "__main__":
    unittest.main()
