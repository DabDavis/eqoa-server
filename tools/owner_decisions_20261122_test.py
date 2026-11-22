#!/usr/bin/env python3
"""Tests of tools/owner_decisions_20261122.py on a made database (no server's)."""
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import owner_decisions_20261122 as t  # noqa: E402


def make(path, chances, items):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, chance REAL, "
               "PRIMARY KEY (questid, step))")
    db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER, "
               "raceuse INTEGER)")
    for i, (kind, c) in enumerate(chances):
        db.execute("INSERT INTO queststeps VALUES (?, 1, ?, ?)", (100 + i, kind, c))
    for i, (cu, ru) in enumerate(items):
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, ?)", (1000 + i, f"item {i}", cu, ru))
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        code = t.main(list(args))
    return code, out.getvalue()


class Rules(unittest.TestCase):
    def test_raised(self):
        self.assertEqual(t.raised(0.5), 0.75)
        self.assertEqual(t.raised(0.2), 0.6)
        self.assertEqual(t.raised(0.067), 0.5335)
        self.assertEqual(t.raised(0.999999), 1.0)

    def test_froglok(self):
        every = t.ALL_CLASSES
        # ranger only: human and elf can be one, so 3 is every race the class allows
        self.assertTrue(t.froglok_gains(1 << 1, 3, every))
        # ranger, but the list leaves out the elf: a restriction, left
        self.assertFalse(t.froglok_gains(1 << 1, 1, every))
        # every game race, none, the froglok already: left
        self.assertFalse(t.froglok_gains(1 << 1, 1023, every))
        self.assertFalse(t.froglok_gains(1 << 1, 0, every))
        self.assertFalse(t.froglok_gains(1 << 1, 3 | 1024, every))
        # paladin (dwarf, elf, human, erudite, ... ): races_for names them all
        pal = t.races_for(1 << 2)
        self.assertTrue(t.froglok_gains(1 << 2, pal, every))
        self.assertFalse(t.froglok_gains(1 << 2, pal & ~(1 << 4), every))
        # Live: the froglok is an east human (0x7E69): no ranger, no paladin; a shadow knight
        self.assertFalse(t.froglok_gains(1 << 1, 3, 0x7E69))
        self.assertFalse(t.froglok_gains(1 << 2, pal, 0x7E69))
        self.assertTrue(t.froglok_gains(1 << 3, t.races_for(1 << 3), 0x7E69))


class Tool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        self.saved = t.EXPECT
        t.EXPECT = {"heirloom": {"chances": {0.5: 2, 0.2: 1}, "items": 1},
                    "live": {"chances": {0.5: 2, 0.2: 1}, "items": 0}}
        self.slack = t.SLACK
        t.SLACK = 0
        make(self.db, [("loot", 0.5), ("loot", 0.5), ("loot", 0.2), ("loot", 0.0), ("loot", 1.0), ("kill", 0.5)],
             [(1 << 1, 3), (1 << 1, 1), (1 << 1, 1023), (0x7FFF, 32767)])

    def tearDown(self):
        t.EXPECT, t.SLACK = self.saved, self.slack
        self.dir.cleanup()

    def rows(self):
        db = sqlite3.connect(self.db)
        try:
            return (db.execute("SELECT questid, kind, chance FROM queststeps ORDER BY questid").fetchall(),
                    db.execute("SELECT patternid, raceuse FROM itempattern ORDER BY patternid").fetchall())
        finally:
            db.close()

    def test_report_writes_nothing(self):
        before = self.rows()
        code, out = run("--db", self.db, "--shard", "heirloom")
        self.assertEqual(code, 0, out)
        self.assertIn("3 loot steps raised", out)
        self.assertIn("1 items gain bit 1024", out)
        self.assertEqual(self.rows(), before)

    def test_apply_and_undo(self):
        before = self.rows()
        code, out = run("--db", self.db, "--shard", "heirloom", "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        steps, items = self.rows()
        self.assertEqual([c for _, _, c in steps], [0.75, 0.75, 0.6, 0.0, 1.0, 0.5])
        self.assertEqual([r for _, r in items], [3 | 1024, 1, 1023, 32767])
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE via = 'tool'").fetchone()[0], 4)
        db.close()
        code, out = run("--db", self.db, "--shard", "heirloom", "--apply", "--no-backup")
        self.assertIn("applied already", out)
        code, out = run("--db", self.db, "--shard", "heirloom", "--undo", "--no-backup")
        self.assertIn("4 rows put back, 0 left", out)
        self.assertEqual(self.rows(), before)

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.db, "--shard", "heirloom", "--apply", "--no-backup")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE queststeps SET chance = 0.9 WHERE questid = 100")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--shard", "heirloom", "--undo", "--no-backup")
        self.assertIn("3 rows put back, 1 left", out)
        steps, _ = self.rows()
        self.assertEqual(steps[0][2], 0.9)

    def test_refused_when_not_as_read(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO queststeps VALUES (200, 1, 'loot', 0.42)")
        db.commit()
        db.close()
        before = self.rows()
        code, out = run("--db", self.db, "--shard", "heirloom", "--apply", "--no-backup")
        self.assertEqual(code, 2)
        self.assertIn("REFUSED", out)
        self.assertEqual(self.rows(), before)

    def test_live_froglok_mask(self):
        code, out = run("--db", self.db, "--shard", "live")
        self.assertEqual(code, 0, out)
        self.assertIn("0 items gain bit 1024", out)


if __name__ == "__main__":
    unittest.main()
