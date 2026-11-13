#!/usr/bin/env python3
"""tools/ascended_natureblade_20261113.py on a database of its own: the copy to 60, the base and the others left, a
second run nothing, --undo exact."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "ascended_natureblade_20261113.py")
EFF = {"v": 1, "scope": "self", "level": 49, "effects": [{"kind": "stat", "stat": "int", "amount": 165}]}


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT, effects TEXT);
        CREATE TABLE spellascended (spellid INTEGER PRIMARY KEY, base INTEGER, made TEXT, source TEXT);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
        INSERT INTO spellpattern VALUES (700623, 'Natureblade', 49), (906732, 'Ascended Natureblade', 49),
            (700622, 'Mortal Wound', 49), (906731, 'Ascended Mortal Wound', 49);
        INSERT INTO spellascended VALUES (906732, 700623, '', ''), (906731, 700622, '', '');
    """)
    for sid in (700623, 906732, 906731):
        db.execute("INSERT INTO spelleffect VALUES (?, '', ?)", (sid, json.dumps(EFF, separators=(",", ":"))))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True, check=True).stdout


def levels(path):
    db = sqlite3.connect(path)
    out = {r[0]: (r[1], json.loads(r[2])["level"]) for r in db.execute(
        "SELECT p.spellid, p.abilitylvl, e.effects FROM spellpattern p JOIN spelleffect e USING (spellid)")}
    db.close()
    return out


class Test(unittest.TestCase):
    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = levels(p)
            run(p)
            self.assertEqual(levels(p), before, "a report wrote")
            run(p, "--apply")
            got = levels(p)
            self.assertEqual(got[906732], (60, 60))
            self.assertEqual(got[700623], (49, 49), "the base")
            self.assertEqual(got[906731], (49, 49), "another copy without --also-broken")
            self.assertIn("applied already", run(p, "--apply"))
            run(p, "--undo")
            self.assertEqual(levels(p), before)
            run(p, "--also-broken", "--apply")
            self.assertEqual(levels(p)[906731], (60, 60))


if __name__ == "__main__":
    unittest.main()
