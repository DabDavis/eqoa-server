#!/usr/bin/env python3
"""tools/legacy_forms_20261126.py: the two paladin legacy forms (Fervor, Sacred Heart as of April 2004) written as a
spellpattern and spelleffect row each, a class mastery each and their legacyforms rows (the table made when the
database lacks it); a second apply writes nothing; a spell id another spell holds refuses everything; --undo puts every
table back exactly.

    python3 -E -s tools/legacy_forms_20261126_test.py
"""
import contextlib
import io
import json
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import legacy_forms_20261126 as lf  # noqa: E402
import tix4_rows as tr  # noqa: E402

TABLES = ("spellpattern", "spelleffect", "cmcatalog")


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL,
            unk2 INTEGER NOT NULL, unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL,
            power INTEGER NOT NULL, iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL,
            recast INTEGER NOT NULL, eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL,
            source TEXT NOT NULL, effects TEXT NOT NULL DEFAULT '');
        CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0,
            classes INTEGER NOT NULL DEFAULT 32767, races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '',
            rank INTEGER NOT NULL DEFAULT 1, name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0,
            cost INTEGER NOT NULL DEFAULT 0, level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0,
            minstats TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '',
            require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '',
            effects TEXT NOT NULL DEFAULT '', adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '',
            removes TEXT NOT NULL DEFAULT '', remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
            enabled INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO spellpattern VALUES (700032, 4, 0, 0, 16544, 2, 72, -1809115846, -1265863830, 0, 120, -1, 'Fervor',
            'A damage shield that heals you over time when you are hit.');
        INSERT INTO spellpattern VALUES (701066, 51, 0, 0, 16880, 3, 500, 593297002, -213673665, 0, 900, -1,
            'Sacred Heart', 'Self, Proc, Increase Hitpoints by 5 ; Increase 150% based on CHA, 5min');
        INSERT INTO spelleffect VALUES (700032, 'Self, Damage Shield Decrease Hitpoints by 10', 497989046, 'x',
            '{"v":1,"stack":2,"min_level":1,"effects":[]}');
        INSERT INTO spelleffect VALUES (701066, 'Self, Add Defensive Proc', 317317368, 'x',
            '{"v":1,"stack":2,"min_level":30,"effects":[]}');
        INSERT INTO cmcatalog (id, category, classes, mastery, name, cost, level) VALUES (3671, 3, 4, 'Sacred Heart',
            'Sacred Heart', 120, 51);
    """)
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in TABLES}
    out["legacyforms"] = db.execute("SELECT * FROM legacyforms ORDER BY 1, 3").fetchall() if tr.table(db, "legacyforms") else []
    db.close()
    return out


def run(*argv):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        rc = tr.main(lf.SPEC, list(argv))
    return rc, buf.getvalue()


class LegacyFormsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        make(self.db)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def test_apply_writes_the_forms_and_undo_puts_back(self):
        before = dump(self.db)
        rc, out = run("--db", self.db)
        self.assertEqual(rc, 0, out)
        self.assertIn("8 rows would be written", out)
        self.assertEqual(dump(self.db), before, "a report writes nothing")
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        forms = db.execute("SELECT spellid, legacy_spellid, mastery_id, enabled FROM legacyforms ORDER BY 1").fetchall()
        self.assertEqual(forms, [(700032, 761841, 31841, 1), (701066, 761842, 31842, 1)])
        fervor = db.execute("SELECT spellname, abilitylvl, casttime, recast, power, range, icon FROM spellpattern "
                            "WHERE spellid = 761841").fetchone()
        self.assertEqual(fervor, ("Fervor (2004)", 4, 2, 1, 48, 16544, -1265863830), "the 2004 record's figures, today's icon and range")
        heart = db.execute("SELECT spellname, abilitylvl, casttime, recast, power FROM spellpattern WHERE spellid = 761842").fetchone()
        self.assertEqual(heart, ("Sacred Heart (2004)", 51, 0, 3600, 0))
        fx, effects = db.execute("SELECT fxdict, effects FROM spelleffect WHERE spellid = 761842").fetchone()
        self.assertEqual(fx, 317317368, "today's visual")
        e = json.loads(effects)
        self.assertEqual((e["stack"], e["min_level"], e["duration_s"]), (2, 30, 300))
        proc = e["effects"][0]["procs"][0]
        self.assertEqual((e["effects"][0]["kind"], proc["kind"], proc["amount"], proc["scale"], proc["scale_pct"], proc["on"]),
                         ("proc", "hp", 5, "cha", 150, "offense"), "a weapon proc that heals")
        proc = json.loads(db.execute("SELECT effects FROM spelleffect WHERE spellid = 761841").fetchone()[0])["effects"][0]["procs"][0]
        self.assertEqual((proc["amount"], proc["on"], proc["scope"]), (-8, "offense", "target"), "a weapon proc of 8")
        cms = db.execute("SELECT id, category, classes, cost, level, require_spells, adds, removes FROM cmcatalog "
                         "WHERE id > 30000 ORDER BY id").fetchall()
        self.assertEqual(cms, [(31841, 3, 4, 1, 4, "700032", "", ""), (31842, 3, 4, 1, 51, "701066", "", "")],
                         "a paladin's class rank of one point each, changing no book")
        audits = db.execute("SELECT COUNT(*) FROM gm_audit WHERE detail LIKE '[basis:era%'").fetchone()[0]
        self.assertEqual(audits, 8)
        db.close()
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("applied already", out)
        rc, out = run("--db", self.db, "--undo")
        self.assertEqual(rc, 0, out)
        after = dump(self.db)
        self.assertEqual({t: after[t] for t in TABLES}, {t: before[t] for t in TABLES})
        self.assertEqual(after["legacyforms"], [])

    def test_an_id_another_spell_holds_refuses_everything(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO spellpattern VALUES (761842, 9, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Someone Else', '')")
        db.commit()
        db.close()
        before = dump(self.db)
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("another's already", out)
        self.assertEqual(dump(self.db), before)

    def test_the_ability_must_be_the_named_one(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE spellpattern SET spellname = 'Renamed' WHERE spellid = 700032")
        db.commit()
        db.close()
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("is not 'Fervor'", out)


if __name__ == "__main__":
    unittest.main()
