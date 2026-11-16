#!/usr/bin/env python3
"""flame_lick_2217_20261116.py on a database of its own (the server's own spellpattern and gm_audit, read out of
internal/database's Go source) holding Flame Lick as the import wrote it: --dry writes nothing; --apply makes it a 0 s
cast with a 6 s recast with its gm_audit line; a second --apply writes nothing; --undo puts the table back exactly. The
controls: a GM's change since is left by both, a shard without the record is left alone, two records refuse."""
import glob
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "flame_lick_2217_20261116.py")
GO = os.path.join(HERE, "..", "internal", "database")


def schema(name):
    for path in sorted(glob.glob(os.path.join(GO, "*.go"))):
        if path.endswith("_test.go"):
            continue
        with open(path) as f:
            m = re.search(r"CREATE TABLE IF NOT EXISTS " + name + r"\s*\(.*?\n\s*\);", f.read(), re.S)
        if m:
            return m.group(0)
    raise AssertionError(f"no schema of {name} in {GO}")


def dump(db):
    return sorted(db.execute("SELECT * FROM spellpattern").fetchall())


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


SPELL = ("INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, "
         "eqprequire, spellname, spelldesc) VALUES (?, 60, 0, 0, 16880, ?, 325, 0, 0, 1, ?, -1, ?, '')")


class FlameLickTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        db.execute(schema("spellpattern"))
        db.execute(schema("gm_audit"))
        db.execute(SPELL, (701140, 2, 15, "Flame Lick"))
        db.execute(SPELL, (701141, 3, 20, "Flaming Fury"))
        db.close()

    def db(self):
        return sqlite3.connect(self.path)

    def test_apply_undo(self):
        before = dump(self.db())
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("casttime 2 -> 0, recast 15 -> 6", r.stdout)
        self.assertEqual(dump(self.db()), before, "--dry wrote")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = self.db()
        self.assertEqual(db.execute("SELECT casttime, recast, power FROM spellpattern WHERE spellid = 701140").fetchone(), (0, 6, 325))
        self.assertEqual(db.execute("SELECT casttime, recast FROM spellpattern WHERE spellid = 701141").fetchone(), (3, 20))
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE tbl = 'spellpattern'").fetchone()[0], 1)
        r = run(self.path, "--apply")
        self.assertIn("nothing to write", r.stdout)
        self.assertEqual(self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0], 1)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(dump(self.db()), before)

    def test_gm_change_left(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("UPDATE spellpattern SET recast = 10 WHERE spellid = 701140")
        r = run(self.path, "--apply")
        self.assertIn("LEFT", r.stdout)
        db = self.db()
        self.assertEqual(db.execute("SELECT casttime, recast FROM spellpattern WHERE spellid = 701140").fetchone(), (0, 10))
        sqlite3.connect(self.path, isolation_level=None).execute("UPDATE spellpattern SET casttime = 1 WHERE spellid = 701140")
        r = run(self.path, "--undo")
        self.assertIn("changed since: LEFT", r.stdout)
        self.assertEqual(self.db().execute("SELECT casttime FROM spellpattern WHERE spellid = 701140").fetchone()[0], 1)

    def test_no_record(self):
        sqlite3.connect(self.path, isolation_level=None).execute("DELETE FROM spellpattern WHERE spellid = 701140")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("nothing to do", r.stdout)

    def test_two_refused(self):
        sqlite3.connect(self.path, isolation_level=None).execute(SPELL, (701999, 2, 15, "Flame Lick"))
        before = dump(self.db())
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 2)
        self.assertEqual(dump(self.db()), before)


if __name__ == "__main__":
    unittest.main()
