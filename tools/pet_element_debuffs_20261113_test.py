#!/usr/bin/env python3
"""pet_element_debuffs_20261113.py on a small database holding the three master class pets' rows as the shards held
them on 2026-11-13 and their nukes: the report writes nothing; --apply makes the three curses at the next spell ids
(each its element's resist -35 for a minute, a 2 s cast, its nuke's look and icon) and puts each first in its pet's
kit, a gm_audit line a row; a second apply writes nothing; --undo puts every table back exactly (the id counter too).
The controls: a kit changed since (a GM's), a spell of a curse's name already there and a part done are refused,
writing nothing."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "pet_element_debuffs_20261113.py")
sys.path.insert(0, HERE)
import pet_element_debuffs_20261113 as tool  # noqa: E402

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
LOOKS = {905453: (1, 829958967, 486870879, 1338050561), 905450: (0, -901013979, 439871130, 2972655021),
         902156: (1, -1265863830, 439871130, 2712491557)}


def make(path, kits=None, extra=None):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for p in tool.PETS:
        sid, name = p["look"]
        ct, icon, color, fx = LOOKS[sid]
        db.execute("INSERT INTO spellpattern VALUES (?, 60, 0, 0, 16544, ?, 0, ?, ?, 1, 12, -1, ?, '')", (sid, ct, color, icon, name))
        db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'x', ?, 'test')", (sid, fx))
        kit = (kits or {}).get(p["name"], p["kit"])
        db.execute("INSERT INTO petstats VALUES (?, ?, ?, 'its note.')", (p["pet"], p["name"], json.dumps(kit, separators=(",", ":"))))
    db.execute("INSERT INTO spellpattern VALUES (906800, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Last Spell', '')")
    if extra:
        db.execute("INSERT INTO spellpattern VALUES (NULL, 60, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, ?, '')", (extra,))
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

    def fresh(self, **kw):
        if os.path.exists(self.db):
            os.remove(self.db)
        make(self.db, **kw)

    def test_apply_again_undo(self):
        self.fresh()
        before = dump(self.db, audit=True)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db, audit=True), before, "the report wrote")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        want = {"Efreeti Lord": (906801, "Efreeti's Flame Curse", "fr", 905453),
                "Djinni Vizier": (906802, "Djinni's Storm Curse", "lr", 905450),
                "Badi Elder": (906803, "Badi's Blight Curse", "dr", 902156)}
        for p in tool.PETS:
            cid, name, stat, look = want[p["name"]]
            ct, icon, color, fx = LOOKS[look]
            sp = db.execute("SELECT spellid, abilitylvl, range, casttime, power, iconColor, icon, scope, recast FROM spellpattern "
                            "WHERE spellname = ?", (name,)).fetchone()
            self.assertEqual(sp, (cid, 60, 16880, 2, 0, color, icon, 1, 30))
            line, fxd, eff = db.execute("SELECT line, fxdict, effects FROM spelleffect WHERE spellid = ?", (cid,)).fetchone()
            self.assertEqual((line, fxd), (f"Target, Decrease {stat.upper()} by 35, 1min", fx))
            eff = json.loads(eff)
            self.assertEqual((eff["duration_s"], eff["harmful"], eff["effects"]),
                             (60, True, [{"type": "Statistic Change", "kind": "stat", "stat": stat, "amount": -35, "resist": stat}]))
            kit = json.loads(db.execute("SELECT abilities FROM petstats WHERE spellid = ?", (p["pet"],)).fetchone()[0])
            self.assertEqual(kit, [{"every": 30.0, "first": 1.0, "kind": "debuff", "spell_id": cid}] + p["kit"])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 9)
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
        badi = tool.PETS[2]
        for kw in ({"kits": {"Badi Elder": badi["kit"][:1]}}, {"extra": "Djinni's Storm Curse"}):
            with self.subTest(**{k: str(v) for k, v in kw.items()}):
                self.fresh(**kw)
                before = dump(self.db, audit=True)
                r = run(self.db, "--apply")
                self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
                self.assertIn("REFUSED", r.stdout)
                self.assertEqual(dump(self.db, audit=True), before)
        # a part done: the Badi Elder's curse there and in its kit, the other two not
        self.fresh()
        db = sqlite3.connect(self.db)
        cid = db.execute("INSERT INTO spellpattern VALUES (NULL, 60, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, ?, '')", (badi["curse"],)).lastrowid
        db.execute("UPDATE petstats SET abilities = ? WHERE spellid = ?", (json.dumps(tool.after_kit(badi, cid), separators=(",", ":")), badi["pet"]))
        db.commit()
        db.close()
        before = dump(self.db, audit=True)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("some of the three", r.stdout)
        self.assertEqual(dump(self.db, audit=True), before)


if __name__ == "__main__":
    unittest.main()
