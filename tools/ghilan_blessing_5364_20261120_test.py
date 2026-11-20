#!/usr/bin/env python3
"""ghilan_blessing_5364_20261120.py on a small database holding the Ghilan Noble's kit as both shards had it on
2026-11-20: the report writes nothing; --apply gives her blessing on her owner while_out, a gm_audit line; a second
apply writes nothing; --undo puts the row back exactly; her blessing on herself is untouched. A kit changed since (a
GM's) is refused."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "ghilan_blessing_5364_20261120.py")
sys.path.insert(0, HERE)
import ghilan_blessing_5364_20261120 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, name TEXT NOT NULL, abilities TEXT NOT NULL DEFAULT '[]');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""

KIT = [
    {"every": 30.0, "first": 1.0, "kind": "debuff", "spell_id": 906695},
    {"every": 30.0, "first": 1.0, "kind": "heal", "spell_id": 905456, "target": "either", "below": 60},
    tool.OLD,
    {"every": 1800.0, "first": 5.0, "kind": "buff", "spell_id": 902167, "target": "self"},
    {"every": 6.0, "first": 2.0, "kind": "heal", "spell_id": 905495, "target": "group", "below": 100},
]


def make(path, kit=KIT):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO petstats VALUES (701088, 'Ghilan Noble', ?)", (json.dumps(kit, separators=(",", ":")),))
    db.commit()
    db.close()


def dump(path, audit=False):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if audit or "gm_audit" not in l]
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", path, *args], capture_output=True, text=True)


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.db)
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        kit = json.loads(db.execute("SELECT abilities FROM petstats").fetchone()[0])
        self.assertEqual(kit, KIT[:2] + [tool.NEW] + KIT[3:])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 1)
        db.close()
        applied = dump(self.db, audit=True)
        r = run(self.db, "--apply")
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.db, audit=True), applied)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_changed_kit_refused(self):
        make(self.db, KIT[:2] + [dict(tool.OLD, every=600.0)] + KIT[3:])
        before = dump(self.db, audit=True)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.db, audit=True), before)


if __name__ == "__main__":
    unittest.main()
