#!/usr/bin/env python3
"""tools/fix_tested_spells.py against a made-up database built from its own
tables: a report writes nothing; --apply makes a self spell a target spell
with its page's record, a prose line its page's, Guardian's Aid Holy Aid,
Bountiful Gather Power from Gather Power's page, the masteries' grants and
the evacuations group gates; a GM's value is left and said; a second --apply
has nothing to do; --undo gives back the database as it was but for a value
changed since. No game data.

    python3 tools/fix_tested_spells_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_tested_spells.py")
sys.path.insert(0, HERE)
import fix_tested_spells as fix  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '');
CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'self', place TEXT NOT NULL DEFAULT '',
    world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0,
    radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    sid = 700100
    ids = {}

    def spell(cls, name, lvl, scope, cast, recast, power, line, title=None):
        nonlocal sid
        sid += 1
        rng, frame = (fix.SELF_RANGE, fix.SELF_FRAME) if scope == 0 else (fix.TARGET_RANGE, fix.TARGET_FRAME)
        db.execute("INSERT INTO spellpattern VALUES (?, ?, 0, 0, ?, ?, ?, ?, 77, ?, ?, -1, ?, ?)",
                   (sid, lvl, rng, cast, power, frame, scope, recast, title or name.title(), line))
        db.execute("INSERT INTO spelleffect VALUES (?, ?, 0, 'eqoa-data (made)')", (sid, line))
        db.execute("INSERT INTO classspells VALUES (?, ?, 'eqoa-data')", (cls, sid))
        ids[(cls, name, lvl)] = sid
        return sid

    for (cls, name, lvl), (now, _, _) in fix.RECORDS.items():
        line = fix.LINES.get((cls, name, lvl), ("Target, Decrease Hitpoints by 10",))[0]
        title = fix.NAMES.get((cls, name, lvl), (None,))[0]
        spell(cls, name, lvl, *now, line, title)
    spell(fix.MNK, "mend wounds", 1, 1, 0, 300, 1, "Target, Increase Hitpoints by 45")
    spell(fix.MNK, "bandage wounds", 15, 1, 1, 300, 1, "Target, Increase Hitpoints by 130")
    rid = 6900
    for (mastery, rank), (now, adds, _) in fix.GRANTS.items():
        rid += 1
        db.execute("INSERT INTO cmcatalog (id, mastery, name, adds, add_spells) VALUES (?, ?, ?, ?, ?)",
                   (rid, mastery, rank, adds or "", now))
    for g in fix.GATES:
        db.execute("INSERT INTO spellgates (spell, kind, place, x, z, source) VALUES (?, 'evac', 'There', 1, 2, 'seed')", (g,))
    db.commit()
    db.close()
    return ids


def run(db, *args):
    r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", db, "--no-backup", *args], capture_output=True, text=True)
    if r.returncode != 0:
        raise AssertionError(r.stdout + r.stderr)
    return r.stdout


def dump(db):
    c = sqlite3.connect(db)
    try:
        return [l for l in c.iterdump() if "sqlite_sequence" not in l]
    finally:
        c.close()


def row(db, sql, *args):
    c = sqlite3.connect(db)
    try:
        return c.execute(sql, args).fetchone()
    finally:
        c.close()


class FixTestedSpells(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        self.ids = make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        before = dump(self.db)
        out = run(self.db)
        self.assertIn("31 rows to change, 0 left", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_then_undo(self):
        before = dump(self.db)
        run(self.db, "--apply")
        ember = self.ids[(fix.MNK, "ember strike", 4)]
        self.assertEqual(row(self.db, "SELECT scope, range, iconColor, casttime, recast, power FROM spellpattern WHERE spellid = ?", ember),
                         (1, fix.TARGET_RANGE, fix.TARGET_FRAME, 0, 30, 30))
        tiger = self.ids[(fix.MNK, "pouncing tiger", 49)]
        self.assertEqual(row(self.db, "SELECT line FROM spelleffect WHERE spellid = ?", tiger),
                         ("Target, Decrease Hitpoints by 1224 ; Increase 20% based on STA",))
        aid = self.ids[(fix.PAL, "guardian's aid", 60)]
        self.assertEqual(row(self.db, "SELECT spellname, casttime, power FROM spellpattern WHERE spellid = ?", aid), ("Holy Aid", 5, 780))
        made = row(self.db, "SELECT spellid, casttime, recast, power, scope FROM spellpattern WHERE spellname = 'Bountiful Gather Power'")
        self.assertEqual(made[1:], (2, 90, 1, 0))
        self.assertEqual(row(self.db, "SELECT source FROM classspells WHERE class = ? AND spellid = ?", fix.WIZ, made[0]), ("eqoa-data cm",))
        self.assertEqual(row(self.db, "SELECT add_spells FROM cmcatalog WHERE name = 'Channeler''s Bounty 2'"), (f"{made[0]},700971",))
        mend, bandage = self.ids[(fix.MNK, "mend wounds", 1)], self.ids[(fix.MNK, "bandage wounds", 15)]
        got = row(self.db, "SELECT add_spells, adds FROM cmcatalog WHERE name = 'Heal Other'")
        self.assertEqual(got[0], f"{mend},{bandage},700920,700964,701015,701070")
        self.assertTrue(got[1].startswith("Mend Wounds@60; Bandage Wounds@60;"))
        self.assertEqual(row(self.db, "SELECT add_spells FROM cmcatalog WHERE name = 'Holy Aid'"), (str(aid),))
        self.assertEqual(row(self.db, "SELECT count(*) FROM spellgates WHERE kind = 'group'"), (3,))
        # a second --apply: nothing to do
        self.assertIn("0 rows to change, 0 left", run(self.db, "--apply"))
        # a GM's change since stays through --undo
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spellpattern SET recast = 200 WHERE spellid = ?", (self.ids[(fix.WIZ, "annihilate", 34)],))
        c.commit()
        c.close()
        out = run(self.db, "--undo")
        self.assertIn("put back 30 of 31 rows", out)
        self.assertEqual(row(self.db, "SELECT recast FROM spellpattern WHERE spellid = ?", self.ids[(fix.WIZ, "annihilate", 34)]), (200,))
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spellpattern SET range = ?, iconColor = ?, scope = 0, casttime = 3, recast = 2, power = 408 WHERE spellid = ?",
                  (fix.SELF_RANGE, fix.SELF_FRAME, self.ids[(fix.WIZ, "annihilate", 34)]))
        c.commit()
        c.close()
        self.assertEqual(dump(self.db), before)

    def test_a_gms_values_are_left(self):
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spellpattern SET recast = 200 WHERE spellid = ?", (self.ids[(fix.WIZ, "annihilate", 34)],))
        c.execute("UPDATE spelleffect SET line = 'Target, Decrease Hitpoints by 999' WHERE spellid = ?",
                  (self.ids[(fix.CLR, "poison ward", 30)],))
        c.execute("UPDATE spellgates SET kind = 'self' WHERE spell = 'freeport evacuation'")
        c.commit()
        c.close()
        out = run(self.db, "--apply")
        self.assertIn("left as it is: WIZ L34 annihilate (record", out)
        self.assertIn("left as it is: CL L30 poison ward: line (its line is 'Target, Decrease Hitpoints by 999')", out)
        self.assertIn("left as it is: gate freeport evacuation (kind 'self', source 'seed': a GM's)", out)
        self.assertEqual(row(self.db, "SELECT recast, scope FROM spellpattern WHERE spellid = ?", self.ids[(fix.WIZ, "annihilate", 34)]),
                         (200, 0))


if __name__ == "__main__":
    unittest.main()
