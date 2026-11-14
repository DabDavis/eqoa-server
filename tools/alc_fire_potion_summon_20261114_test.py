#!/usr/bin/env python3
"""tools/alc_fire_potion_summon_20261114.py on a database of its own: the Fire Potion summons as the Cocktail of
Torment does, the base pair left, a second run nothing, --undo exact, a Fire Potion that is not the nuke refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "alc_fire_potion_summon_20261114.py")
NUKE = '{"v":1,"scope":"target_aoe","effects":[{"type":"Dmg/Heal","kind":"hp2","amount":-1980}]}'
SUMMON = ('{"v":1,"scope":"self","effects":[{"kind":"create_item","item":{"count":3,'
          '"name":"Ascended Severupidus\' Cocktail","patternid":980155}}]}')
BASE_NUKE = '{"v":1,"effects":[{"kind":"hp2","amount":-1800}]}'


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, unk2 INTEGER, unk3 INTEGER,
            range INTEGER, casttime INTEGER, power INTEGER, iconColor INTEGER, icon INTEGER, scope INTEGER,
            recast INTEGER, eqprequire INTEGER, spellname TEXT, spelldesc TEXT);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT, fxdict INTEGER, source TEXT, effects TEXT);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
    """)
    db.executemany("INSERT INTO spellpattern VALUES (?, 60, 0, 0, ?, ?, ?, ?, ?, ?, ?, -1, ?, ?)", [
        (906789, 16544, 2, 500, -1, -2, 0, 2, "Ascended Severupidus' Cocktail of Torment", "summon"),
        (906808, 16880, 1, 140, 7, 8, 1, 1, "Ascended Severupidus' Fire Potion", "nuke"),
        (760105, 16880, 1, 140, 7, 8, 1, 1, "Severupidus' Fire Potion", "nuke")])
    db.executemany("INSERT INTO spelleffect VALUES (?, ?, 5, 's', ?)", [
        (906789, "Self, Summon Ascended Severupidus' Cocktail x3", SUMMON),
        (906808, "Target, aoe 1980", NUKE), (760105, "Target, aoe 1800", BASE_NUKE)])
    db.commit()
    db.close()


def run(path, *args, check=True):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True,
                          check=check)


def dump(path):
    db = sqlite3.connect(path)
    out = [list(db.execute("SELECT * FROM spellpattern ORDER BY spellid")),
           list(db.execute("SELECT * FROM spelleffect ORDER BY spellid"))]
    db.close()
    return out


class Test(unittest.TestCase):
    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = dump(p)
            run(p)
            self.assertEqual(dump(p), before, "a report wrote")
            run(p, "--apply")
            db = sqlite3.connect(p)
            sp = db.execute("SELECT scope, casttime, power, icon, spellname FROM spellpattern WHERE spellid = 906808").fetchone()
            self.assertEqual(sp, (0, 2, 500, -2, "Ascended Severupidus' Fire Potion"))
            self.assertEqual(db.execute("SELECT effects FROM spelleffect WHERE spellid = 906808").fetchone()[0], SUMMON)
            self.assertEqual(db.execute("SELECT effects FROM spelleffect WHERE spellid = 760105").fetchone()[0], BASE_NUKE)
            self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 2)
            db.close()
            self.assertIn("applied already", run(p, "--apply").stdout)
            run(p, "--undo")
            self.assertEqual(dump(p), before)

    def test_refuses_other_record(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            db = sqlite3.connect(p)
            db.execute("UPDATE spelleffect SET effects = '{\"effects\":[{\"kind\":\"stat\"}]}' WHERE spellid = 906808")
            db.commit()
            db.close()
            r = run(p, "--apply", check=False)
            self.assertEqual(r.returncode, 1)
            self.assertIn("REFUSED", r.stdout)


if __name__ == "__main__":
    unittest.main()
