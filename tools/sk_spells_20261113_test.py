#!/usr/bin/env python3
"""sk_cursed_hand_level_20261113.py and sk_chilling_aura_proc_20261113.py on a database of their own: each report writes
nothing, --apply writes what its doc says, a second run nothing, --undo exact; a row not as found refuses."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
HAND = os.path.join(HERE, "sk_cursed_hand_level_20261113.py")
AURA = os.path.join(HERE, "sk_chilling_aura_proc_20261113.py")
AURA_FX = {"v": 1, "scope": "self", "duration_s": 300, "level": 1, "sheet_type": "Defensive Proc", "effects": [
    {"id": "0725", "type": "Defensive Proc", "kind": "def_proc", "procs": [
        {"id": "0D73", "kind": "hp", "amount": -2, "resist": "cr", "on": "defense", "scope": "target"}]}]}


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT, effects TEXT);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, levelreq INTEGER, itemdesc TEXT);
        CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
        INSERT INTO spellpattern VALUES (1420, 'Chilling Aura', 1), (1422, 'Cursed Hand', 3), (901430, 'Veil of Death', 12);
        INSERT INTO classspells VALUES (3, 1420, 'dump'), (3, 1422, 'dump'), (3, 901430, 'eqoaemu');
        INSERT INTO itempattern VALUES (20691, 'Cursed Hand', 3, 'Scribing ... Level 3 - Offensive Proc');
    """)
    db.execute("INSERT INTO spelleffect VALUES (1422, 'Self, x', ?)", (json.dumps({"v": 1, "level": 3, "effects": []}),))
    db.execute("INSERT INTO spelleffect VALUES (1420, 'Self, Damage Shield Decrease Hitpoints by 2, 5min', ?)", (json.dumps(AURA_FX),))
    db.commit()
    db.close()


def run(tool, path, *args):
    p = subprocess.run([sys.executable, "-E", "-s", tool, "--db", path, *args], capture_output=True, text=True)
    return p.returncode, p.stdout


def dump(path):
    db = sqlite3.connect(path)
    out = [db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("spellpattern", "spelleffect", "itempattern")]
    db.close()
    return out


class Test(unittest.TestCase):
    def test_both(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = dump(p)
            for tool in (HAND, AURA):
                self.assertEqual(run(tool, p)[0], 0)
            self.assertEqual(dump(p), before, "a report wrote")
            for tool in (HAND, AURA):
                code, out = run(tool, p, "--apply")
                self.assertEqual(code, 0, out)
            db = sqlite3.connect(p)
            self.assertEqual(db.execute("SELECT abilitylvl FROM spellpattern WHERE spellid = 1422").fetchone()[0], 4)
            self.assertEqual(db.execute("SELECT abilitylvl FROM spellpattern WHERE spellid = 901430").fetchone()[0], 12)
            self.assertEqual(db.execute("SELECT levelreq, itemdesc FROM itempattern").fetchone(), (4, "Scribing ... Level 4 - Offensive Proc"))
            self.assertEqual(json.loads(db.execute("SELECT effects FROM spelleffect WHERE spellid = 1422").fetchone()[0])["level"], 4)
            line, eff = db.execute("SELECT line, effects FROM spelleffect WHERE spellid = 1420").fetchone()
            e = json.loads(eff)["effects"][0]
            self.assertEqual((line, e["kind"], e["procs"][0]["on"], e["procs"][0]["amount"]),
                             ("Self, Add Offensive Proc: Decrease Hitpoints by 2, 5min", "proc", "offense", -2))
            db.close()
            for tool in (HAND, AURA):
                self.assertIn("applied already", run(tool, p, "--apply")[1])
            for tool in (AURA, HAND):
                self.assertEqual(run(tool, p, "--undo")[0], 0)
            self.assertEqual(dump(p), before, "the undo is not exact")

    def test_refuses(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            db = sqlite3.connect(p)
            db.execute("UPDATE spellpattern SET abilitylvl = 5 WHERE spellid = 1422")
            db.execute("UPDATE spelleffect SET line = 'a GM''s' WHERE spellid = 1420")
            db.commit()
            db.close()
            before = dump(p)
            for tool in (HAND, AURA):
                code, out = run(tool, p, "--apply")
                self.assertNotEqual(code, 0, out)
            self.assertEqual(dump(p), before)


if __name__ == "__main__":
    unittest.main()
