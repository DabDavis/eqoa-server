#!/usr/bin/env python3
"""ghilan_noble_2001_20261113.py on a small database holding the Ghilan Noble's row as tools/fix_pets_1110.py left
it and the spells it names: the report writes nothing; --apply makes Ghilan's Frost Curse at the next spell id, puts
it first in her kit and her blessing on herself after her owner's, a gm_audit line a row; a second apply writes
nothing; --undo puts every table back exactly (the id counter too). The controls: her kit changed since (a GM's) and
a spell of the curse's name already there are refused, writing nothing."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "ghilan_noble_2001_20261113.py")
sys.path.insert(0, HERE)
import ghilan_noble_2001_20261113 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
    effects TEXT NOT NULL DEFAULT '');
CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, name TEXT NOT NULL, abilities TEXT NOT NULL DEFAULT '[]',
    note TEXT NOT NULL DEFAULT '');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""


def make(path, kit=None):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for sid, name, ct in ((902167, "Ghilan's Blessing", 3), (905454, "Ghilan's Chill", 2), (906800, "Last Spell", 1)):
        db.execute("INSERT INTO spellpattern VALUES (?, 60, 0, 0, 16880, ?, 0, 439871130, 282007337, 1, 30, -1, ?, '')",
                   (sid, ct, name))
        db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'x', 2654136144, 'test')", (sid,))
    db.execute("INSERT INTO petstats VALUES (701088, 'Ghilan Noble', ?, 'her note.')",
               (json.dumps(tool.BEFORE if kit is None else kit, separators=(",", ":")),))
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

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db, audit=True)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db, audit=True), before, "the report wrote")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        cid, line, fx, eff = db.execute("SELECT p.spellid, e.line, e.fxdict, e.effects FROM spellpattern p JOIN spelleffect e "
                                        "USING (spellid) WHERE p.spellname = ?", (tool.CURSE_NAME,)).fetchone()
        self.assertEqual((cid, line, fx), (906801, "Target, Decrease CR by 35, 1min", 2654136144))
        self.assertEqual(json.loads(eff)["effects"][0], {"type": "Statistic Change", "kind": "stat", "stat": "cr",
                                                         "amount": -35, "resist": "cr"})
        kit = json.loads(db.execute("SELECT abilities FROM petstats WHERE spellid = 701088").fetchone()[0])
        self.assertEqual(kit[0], {"every": 30.0, "first": 1.0, "kind": "debuff", "spell_id": cid})
        self.assertEqual([(a["kind"], a["spell_id"], a.get("target")) for a in kit if a["kind"] == "buff"],
                         [("buff", 902167, "owner"), ("buff", 902167, "self")])
        self.assertEqual(kit[1:4] + kit[5:], tool.BEFORE)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 3)
        db.close()
        applied = dump(self.db, audit=True)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.db, audit=True), applied, "a second apply wrote")
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), [l for l in before if "gm_audit" not in l])

    def test_refusals(self):
        changed = tool.BEFORE[:-1]
        make(self.db, kit=changed)
        before = dump(self.db, audit=True)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.db, audit=True), before)
        os.remove(self.db)
        make(self.db)
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO spellpattern VALUES (NULL, 60, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, ?, '')", (tool.CURSE_NAME,))
        db.commit()
        db.close()
        before = dump(self.db, audit=True)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.db, audit=True), before)


if __name__ == "__main__":
    unittest.main()
