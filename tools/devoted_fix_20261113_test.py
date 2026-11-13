#!/usr/bin/env python3
"""devoted_fix_20261113.py on a small database holding tools/jalraeth_masteries_20261111.py's rows: the report writes
nothing; --apply makes Sacred Scripture one rank (2165 gone, an owner of both keeping what it paid on 2164) and Path
of the Devout raw stats (its ability out of every book, a kept buff of it gone), a gm_audit line a row; a second
apply does nothing; --undo puts every table back exactly. The controls: rows not as the 2026-11-11 tool wrote them,
or an owner of 2165 without 2164, are refused, writing nothing. And the effects it writes are the ones
internal/entity/devoted_test.go parses."""
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import devoted_fix_20261113 as df  # noqa: E402
import jalraeth_masteries_20261111_test as jt  # noqa: E402

TOOL = os.path.join(HERE, "devoted_fix_20261113.py")
KIT = os.path.join(HERE, "jalraeth_masteries_20261111.py")
GOTEST = os.path.join(HERE, "..", "internal", "entity", "devoted_test.go")

MORE = """
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL,
    addedorder INTEGER NOT NULL, onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL,
    showhide INTEGER NOT NULL);
CREATE TABLE charbuffs (serverid INTEGER NOT NULL, seq INTEGER NOT NULL, spellid INTEGER NOT NULL, name TEXT NOT NULL,
    icon INTEGER NOT NULL, caster INTEGER NOT NULL, left_ms INTEGER NOT NULL, tick_ms INTEGER NOT NULL,
    hp_tick INTEGER NOT NULL, power_tick INTEGER NOT NULL, mods TEXT NOT NULL, flags INTEGER NOT NULL,
    PRIMARY KEY (serverid, seq));
"""

OWNERS = """
INSERT INTO charcms VALUES (722, 2166, 200, 1), (722, 2167, 75, 2), (722, 2168, 75, 3),
    (800, 2164, 10, 4), (800, 2165, 15, 5), (801, 2164, 10, 6);
INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES
    (722, 760170, 66, 1, 1, 0, -1), (722, 760171, 67, 0, -1, 0, -1), (722, 760172, 63, 0, -1, 0, -1);
INSERT INTO charbuffs VALUES (722, 0, 760170, 'Path of the Devout', 1, 722, 30000, 0, 0, 0, '{}', 0),
    (722, 1, 760172, 'Guided Meditation', 1, 722, 900000, 0, 0, 0, '{}', 0);
"""


def make(path, extra=""):
    jt.make(path)
    r = subprocess.run([sys.executable, KIT, "--db", path, "--apply"], capture_output=True, text=True)
    assert r.returncode == 0, r.stdout + r.stderr
    db = sqlite3.connect(path)
    db.executescript(MORE + OWNERS + extra)
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


def q(path, sql, args=()):
    db = sqlite3.connect(path)
    try:
        return db.execute(sql, args).fetchall()
    finally:
        db.close()


class Tool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db)
        before = jt.dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 7 rows", r.stdout)
        self.assertEqual(jt.dump(self.db), before)

        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("applied: 7 rows", r.stdout)
        self.assertEqual(q(self.db, "SELECT name, cost, level, effects FROM cmcatalog WHERE id = 2164"),
                         [("Sacred Scripture", 25, 6, "WIS:10,WISMax:20")])
        self.assertEqual(q(self.db, "SELECT COUNT(*) FROM cmcatalog WHERE id = 2165"), [(0,)])
        self.assertEqual(q(self.db, "SELECT effects, adds, add_spells, removes, remove_spells FROM cmcatalog WHERE id = 2167"),
                         [("HP:500,AC:250,Off Mod:2,Def Mod:2,STA:25,WIS:25", "", "", "", "")])
        # an owner of both keeps what it paid, on the one rank; one of 2164 alone keeps it as it was
        self.assertEqual(q(self.db, "SELECT serverid, cmid, cost FROM charcms WHERE cmid IN (2164, 2165) ORDER BY 1, 2"),
                         [(800, 2164, 25), (801, 2164, 10)])
        # Path of the Devout out of the book and its kept buff gone; the other abilities and buffs stay
        self.assertEqual(q(self.db, "SELECT spellid FROM spells WHERE serverid = 722 ORDER BY spellid"),
                         [(760171,), (760172,)])
        self.assertEqual(q(self.db, "SELECT spellid FROM charbuffs WHERE serverid = 722"), [(760172,)])
        self.assertEqual(q(self.db, "SELECT COUNT(*) FROM gm_audit WHERE detail LIKE '%devoted_fix_20261113%'"), [(7,)])

        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("applied already", r.stdout)

        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("put back 7, 0 left", r.stdout)
        # the same rows (a row of a table keyed by two columns comes back under a new rowid: its dump line moves)
        self.assertEqual(sorted(jt.dump(self.db)), sorted(before))
        self.assertEqual(q(self.db, "SELECT COUNT(*) FROM sqlite_master WHERE name = ?", (df.LOG,)), [(0,)])

    def test_undo_leaves_a_row_changed_since(self):
        make(self.db)
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE cmcatalog SET cost = 30 WHERE id = 2164")
        db.commit()
        db.close()
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("put back 6, 1 left", r.stdout)
        self.assertEqual(q(self.db, "SELECT cost FROM cmcatalog WHERE id = 2164"), [(30,)])

    def test_refused_when_not_as_written(self):
        make(self.db, "UPDATE cmcatalog SET effects = 'WIS:12' WHERE id = 2164;")
        before = jt.dump(self.db)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("cmcatalog 2164 is not as", r.stdout)
        self.assertEqual(jt.dump(self.db), before)

    def test_refused_owner_of_2165_without_2164(self):
        make(self.db, "INSERT INTO charcms VALUES (802, 2165, 15, 7);")
        before = jt.dump(self.db)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("character 802 owns 2165 but not 2164", r.stdout)
        self.assertEqual(jt.dump(self.db), before)

    def test_effects_are_the_go_tests(self):
        src = open(GOTEST).read()
        for name, value in (("sacredScriptureEffects", df.NOW[df.SCRIPTURE]["effects"]),
                            ("pathOfTheDevoutEffects", df.NOW[df.PATH_RANK]["effects"])):
            m = re.search(name + r'\s*=\s*"([^"]*)"', src)
            self.assertIsNotNone(m, name)
            self.assertEqual(m.group(1), value, name)


if __name__ == "__main__":
    unittest.main()
