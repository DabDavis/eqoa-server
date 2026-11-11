#!/usr/bin/env python3
"""jalraeth_masteries_20261111.py on a small database: the report writes nothing; --apply writes the 14 rows with a
gm_audit line each; a second apply does nothing; --undo puts every table back exactly. The controls: a cmcatalog row
already holding the Jal'Raeth's bit, an id taken, or racedefs 30 not the Jal'Raeth, is refused, writing nothing. And
the three spells' effects are the ones internal/entity/jalraeth_test.go casts."""
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import jalraeth_masteries_20261111 as jk  # noqa: E402

TOOL = os.path.join(HERE, "jalraeth_masteries_20261111.py")
GOTEST = os.path.join(HERE, "..", "internal", "entity", "jalraeth_test.go")

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
    requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '',
    adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '',
    remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'gm', require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '');
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
    effects TEXT NOT NULL DEFAULT '');
CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO spellpattern VALUES (906720, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Wisp''s Raiment', '');
INSERT INTO cmcatalog (id, races, name) VALUES (2145, 2147483648, 'Pale Fire'), (1001, 64, 'Blood of the North');
"""


def make(path, race="Jal'Raeth", extra=""):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA + extra)
    db.execute("INSERT INTO racedefs VALUES (30, ?)", (race,))
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l]
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


class Tool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 14 rows", r.stdout)
        self.assertEqual(dump(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM cmcatalog WHERE races = ?", (1 << 30,)).fetchone()[0], 8)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 14)
        self.assertEqual(db.execute("SELECT add_spells FROM cmcatalog WHERE id = 2167").fetchone()[0], "760170")
        db.close()
        applied = dump(self.db)
        r = run(self.db, "--apply")
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.db), applied)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_refusals(self):
        for extra, race in (("INSERT INTO cmcatalog (id, races, name) VALUES (2199, 1073741824, 'Placeholder');", "Jal'Raeth"),
                            ("INSERT INTO cmcatalog (id, name) VALUES (2163, 'Something');", "Jal'Raeth"),
                            ("", "Wisp")):
            if os.path.exists(self.db):
                os.remove(self.db)
            make(self.db, race, extra)
            before = dump(self.db)
            r = run(self.db, "--apply")
            self.assertEqual(r.returncode, 1, r.stdout)
            self.assertIn("refused", r.stdout)
            self.assertEqual(dump(self.db), before)

    def test_effects_are_the_go_tests(self):
        go = open(GOTEST).read()
        consts = dict(re.findall(r"(\w+)\s*=\s*`([^`]*)`", go))
        for (sp, se), name in zip(jk.SPELLS, ("path", "touch", "med")):
            self.assertEqual(consts[name], se["effects"], sp["spellname"])


if __name__ == "__main__":
    unittest.main()
