#!/usr/bin/env python3
"""stacking_t277_20261122.py on a small database: every planned row found by its name moves to its stack type (a
spell's own id for "own"), nothing else in the row changes; a row not as read refuses the whole run, nothing written;
a second apply says applied already; --undo puts every row back exactly, gm_audit a line a row each way."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "stacking_t277_20261122.py")
sys.path.insert(0, HERE)
import stacking_t277_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '');
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def build(path, skip=(), bad=None):
    """Every planned spell at its stack type now (ids from 5000), a decoy of another name, one of a planned name but
    another stack type (Tranquility's kind: not touched)."""
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO accountinfo VALUES (7, 'dab')")
    sid = 5000
    for ex, name, old, new in tool.PLAN:
        if name in skip:
            continue
        eff = {"v": 1, "scope": "target", "stack": old if name != bad else 99, "level": 10,
               "effects": [{"kind": "stat", "stat": "ac", "amount": 5}]}
        db.execute("INSERT INTO spellpattern VALUES (?, ?)", (sid, name))
        db.execute("INSERT INTO spelleffect (spellid, effects) VALUES (?, ?)", (sid, compact(eff)))
        sid += 1
    # a second "Courage" of another stack type (as a self Tranquility beside the enchanter's): not a candidate
    db.execute("INSERT INTO spellpattern VALUES (9001, 'Courage')")
    db.execute("INSERT INTO spelleffect (spellid, effects) VALUES (9001, ?)", (compact({"v": 1, "stack": 2, "effects": []}),))
    db.execute("INSERT INTO spellpattern VALUES (9002, 'Unrelated')")
    db.execute("INSERT INTO spelleffect (spellid, effects) VALUES (9002, ?)", (compact({"v": 1, "stack": 237, "effects": []}),))
    db.commit()
    return db


def snapshot(path):
    db = sqlite3.connect(path)
    try:
        return dict(db.execute("SELECT spellid, effects FROM spelleffect").fetchall())
    finally:
        db.close()


class Stacking(unittest.TestCase):
    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "eqoa.db")
            build(p).close()
            before = snapshot(p)
            r = run(p)
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertIn(f"{len(tool.PLAN)} rows would be written", r.stdout)
            self.assertEqual(snapshot(p), before, "report only wrote")

            r = run(p, "--apply")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            after = snapshot(p)
            db = sqlite3.connect(p)
            for ex, name, old, new in tool.PLAN:
                sid = db.execute("SELECT spellid FROM spellpattern WHERE spellname = ? AND spellid < 9000", (name,)).fetchone()[0]
                eff = json.loads(after[sid])
                self.assertEqual(eff["stack"], sid if new == tool.OWN else new, name)
                was = json.loads(before[sid])
                was["stack"] = eff["stack"]
                self.assertEqual(was, eff, f"{name}: more than the stack changed")
            self.assertEqual(after[9001], before[9001], "the other Courage touched")
            self.assertEqual(after[9002], before[9002], "an unplanned spell touched")
            n = db.execute("SELECT count(*) FROM gm_audit WHERE username = 'dab' AND accountid = 7 AND tbl = 'spelleffect'").fetchone()[0]
            self.assertEqual(n, len(tool.PLAN))
            db.close()

            r = run(p, "--apply")
            self.assertIn("applied already", r.stdout)

            r = run(p, "--undo")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertEqual(snapshot(p), before, "undo not exact")
            db = sqlite3.connect(p)
            self.assertIsNone(db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (tool.LOG,)).fetchone())
            self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 2 * len(tool.PLAN))
            db.close()

    def test_refuses_when_not_as_read(self):
        for kw in ({"bad": "Order"}, {"skip": ("Enrage",)}):
            with tempfile.TemporaryDirectory() as d:
                p = os.path.join(d, "eqoa.db")
                build(p, **kw).close()
                before = snapshot(p)
                r = run(p, "--apply")
                self.assertEqual(r.returncode, 1, r.stdout)
                self.assertIn("refused, nothing done", r.stdout)
                self.assertEqual(snapshot(p), before, f"{kw}: written though refused")

    def test_undo_leaves_a_row_changed_since(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "eqoa.db")
            build(p).close()
            self.assertEqual(run(p, "--apply").returncode, 0)
            db = sqlite3.connect(p)
            sid = db.execute("SELECT spellid FROM spellpattern WHERE spellname = 'Order'").fetchone()[0]
            db.execute("UPDATE spelleffect SET effects = '{}' WHERE spellid = ?", (sid,))
            db.commit()
            db.close()
            r = run(p, "--undo")
            self.assertEqual(r.returncode, 1)
            self.assertIn(f"left: spelleffect {sid} changed since", r.stdout)


if __name__ == "__main__":
    unittest.main()
