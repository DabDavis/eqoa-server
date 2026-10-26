#!/usr/bin/env python3
"""tools/fix_master_class_names.py against a made-up catalog: a master class's
first rank renamed bare, and what it must leave alone (the ranks after it, a
class mastery's first rank, a first rank a GM renamed, one bare already);
--undo exact, and a name renamed after the fix left at --undo. No game data.

    python3 tools/fix_master_class_names_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_master_class_names.py")

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL, mastery TEXT NOT NULL,
                        rank INTEGER NOT NULL, name TEXT NOT NULL, cost INTEGER NOT NULL DEFAULT 0);
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, PRIMARY KEY (serverid, cmid));
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO cmcatalog VALUES (?, ?, ?, ?, ?, ?)", [
        (3801, 4, "Binder", 1, "Binder 1", 200),            # renamed: "Binder"
        (3802, 4, "Binder", 2, "Binder 2", 75),             # a later rank: kept
        (3811, 4, "Shaper", 1, "Shaper the First", 200),    # a GM's name: kept
        (3821, 4, "Seer", 1, "Seer", 200),                  # bare already
        (3831, 4, "Ward", 1, "Ward 1", 200),                # renamed: "Ward"
        (3601, 3, "Empathy", 1, "Empathy 1", 20),           # a class mastery: kept
    ])
    db.execute("INSERT INTO charcms VALUES (44, 3801)")
    db.commit()
    db.close()


def run(db, *args):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", db, *args], capture_output=True, text=True, check=True).stdout


def names(db):
    c = sqlite3.connect(db)
    out = dict(c.execute("SELECT id, name FROM cmcatalog"))
    c.close()
    return out


def dump(db):
    c = sqlite3.connect(db)
    out = "\n".join(c.iterdump())
    c.close()
    return out


class FixMasterClassNames(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        before = dump(self.db)
        out = run(self.db)
        self.assertIn("2 names to change", out)
        self.assertIn("'Binder 1' -> 'Binder'", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_then_undo(self):
        before = dump(self.db)
        run(self.db, "--apply")
        n = names(self.db)
        self.assertEqual((n[3801], n[3831]), ("Binder", "Ward"))
        self.assertEqual((n[3802], n[3811], n[3821], n[3601]), ("Binder 2", "Shaper the First", "Seer", "Empathy 1"),
                         "a later rank, a GM's name, a bare one and a class mastery are left as they are")
        self.assertIn("0 names to change", run(self.db), "a second run changes nothing")
        self.assertIn("0 names to change", run(self.db, "--apply"), "nor a second --apply")
        run(self.db, "--undo")
        self.assertEqual(dump(self.db), before, "--undo puts the database back as it was")

    def test_undo_leaves_a_name_renamed_since(self):
        run(self.db, "--apply")
        c = sqlite3.connect(self.db)
        c.execute("UPDATE cmcatalog SET name = 'Ward of Ages' WHERE id = 3831")
        c.commit()
        c.close()
        out = run(self.db, "--undo")
        n = names(self.db)
        self.assertEqual(n[3801], "Binder 1", "the control: one still as the fix left it goes back")
        self.assertEqual(n[3831], "Ward of Ages")
        self.assertIn("renamed since", out)


if __name__ == "__main__":
    unittest.main()
