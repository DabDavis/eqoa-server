#!/usr/bin/env python3
"""tools/fix_mage_pets_1450.py against a made-up database of the tables it
reads and writes (the server's schemas; the four pets' rows as the tool
expects them, their numbers made up): a report writes nothing; --apply makes
the eight spells with their looks' icons and visuals, gives the four pets
their kits (the Vizier its size, the Efreeti Lord and the Badi Elder their
weapons) and the Badi Elder's template its crit, each row with its gm_audit
line and its log row; a second --apply has nothing to do; --undo gives back
the database as it was but for gm_audit's lines, and leaves a row changed
since; a row not as expected, an id or a name taken, or the template another
pet's too is refused with nothing written.

    python3 tools/fix_mage_pets_1450_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_mage_pets_1450.py")
sys.path.insert(0, HERE)
import fix_mage_pets_1450 as fix  # noqa: E402
import import_eqoaemu_pets as pets  # noqa: E402

SCHEMA = """
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
    effects TEXT NOT NULL DEFAULT '');
CREATE TABLE spellarea (spellid INTEGER PRIMARY KEY, centre INTEGER NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0,
    maxtargets INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '',
    special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0, special_fx INTEGER NOT NULL DEFAULT 0,
    abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    special_every REAL NOT NULL DEFAULT 0, special_first REAL NOT NULL DEFAULT 0, special_jitter REAL NOT NULL DEFAULT 0,
    special_from INTEGER NOT NULL DEFAULT 0, proc_chance INTEGER NOT NULL DEFAULT 0);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, modelid INTEGER, size REAL);
"""

# made-up bodies, icons and visuals
VIZIER_BODY = 70001
OLD_ROWS = {  # spellid: (sheet pet, name, model, wpn1, wpn2, abilities, size)
    fix.VIZIER: ("003C", "Djinni Vizier", VIZIER_BODY, 330, 0, fix.OLD[fix.VIZIER][0], 0.0),
    fix.EFREETI: ("003D", "Efreeti Lord", 70002, 660, 605, fix.OLD[fix.EFREETI][0], 1.75),
    fix.BADI: ("003B", "Badi Elder", 70003, 495, 440, fix.OLD[fix.BADI][0], 0.0),
    fix.GHILAN: ("003E", "Ghilan Noble", 70004, 385, 0, fix.OLD[fix.GHILAN][0], 2.0),
}


class FixMagePetsTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        for s in (pets.PETSTATS_SCHEMA, pets.PETTEMPLATE_SCHEMA, pets.AUDIT_SCHEMA):
            db.execute(s)
        db.execute("ALTER TABLE petstats ADD COLUMN size REAL NOT NULL DEFAULT 0")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        spells = {sid: name for sid, (name, _, _) in fix.SUMMONS.items()}
        spells.update(fix.KEPT)
        spells.update({look: name for look, name in fix.LOOKS.values()})
        for i, (sid, name) in enumerate(sorted(spells.items())):
            db.execute("INSERT INTO spellpattern VALUES (?, 55, 0, 0, ?, 2, 100, ?, ?, 1, 12, -1, ?, '')",
                       (sid, 16880 + i, 8000 + i, 9000 + i, name))
            db.execute("INSERT INTO spelleffect VALUES (?, 'Target, Decrease Hitpoints by 10', ?, 'test', '')",
                       (sid, 0x0B000000 + i))
        for sid, (sheet, name, model, w1, w2, kit, size) in OLD_ROWS.items():
            db.execute("INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, wpn1_dmg, wpn1_type, wpn2_dmg, "
                       "wpn2_type, abilities, size, note, source) VALUES (?, ?, ?, ?, 60, ?, 'blunt', ?, ?, ?, ?, 'its row.', "
                       "'eqoaemu')", (sid, sheet, name, model, w1, w2, "slash" if w2 else "", json.dumps(kit), size))
        db.execute("INSERT INTO classtemplates (name, class, note, source) VALUES ('pet badi elder', 0, 'a mastery pet.', 'seed')")
        db.execute("INSERT INTO pettemplate (spell, template, note, source) VALUES ('summon badi elder', 'pet badi elder', '', 'seed')")
        db.execute("INSERT INTO pettemplate (spell, template, note, source) VALUES ('summon efreeti lord', 'pet efreeti lord', '', "
                   "'seed')")
        # the Vizier's body: worn at 1.0 within a person's, by a giant outside it
        db.executemany("INSERT INTO npcs (npc_name, modelid, size) VALUES (?, ?, ?)",
                       [("a wind spirit", VIZIER_BODY, 1.0), ("a great wind", VIZIER_BODY, 2.2)])
        db.close()

    def db(self):
        return sqlite3.connect(self.path, isolation_level=None)

    def run_tool(self, *args, ok=True):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, "--no-backup", *args], capture_output=True, text=True)
        if ok:
            self.assertEqual(p.returncode, 0, p.stdout + p.stderr)
        return p

    def dump(self):
        """Every table but gm_audit (its counter too), row for row."""
        db, out = self.db(), {}
        for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'gm_audit' ORDER BY name"):
            q = f"SELECT * FROM {t} ORDER BY rowid" if t != "sqlite_sequence" else \
                "SELECT * FROM sqlite_sequence WHERE name != 'gm_audit' ORDER BY name"
            out[t] = db.execute(q).fetchall()
        return out

    def kit(self, sid):
        return fix.norm(json.loads(self.db().execute("SELECT abilities FROM petstats WHERE spellid = ?", (sid,)).fetchone()[0]))

    def test_a_report_writes_nothing(self):
        was = self.dump()
        p = self.run_tool()
        self.assertIn("21 rows to write", p.stdout)
        self.assertEqual(self.dump(), was)

    def test_apply_writes_the_kits_the_spells_and_the_crit(self):
        self.run_tool("--apply")
        db = self.db()
        for sid in fix.NEW:
            self.assertEqual(self.kit(sid), fix.norm(fix.NEW[sid]), sid)
        row = lambda sid: db.execute("SELECT wpn1_dmg, wpn2_dmg, size, note FROM petstats WHERE spellid = ?", (sid,)).fetchone()  # noqa
        self.assertEqual(row(fix.EFREETI)[:3], (988, 988, 1.75))  # its size left
        self.assertEqual(row(fix.BADI)[:3], (881, 881, 0.0))
        self.assertEqual(row(fix.VIZIER)[2], 1.3)  # 1.3 times its body's 1.0 (the giant's 2.2 not a person's)
        self.assertEqual(row(fix.GHILAN)[2], 2.0)
        self.assertTrue(row(fix.GHILAN)[3].startswith("its row. Petition 1450"))
        # the spells: their records and looks
        for sid, name, cast, recast, line, effects, _desc, look, _ in fix.SPELLS:
            p = db.execute("SELECT spellname, casttime, recast, power, scope, abilitylvl, range, iconColor, icon FROM spellpattern "
                           "WHERE spellid = ?", (sid,)).fetchone()
            lp = db.execute("SELECT range, iconColor, icon FROM spellpattern WHERE spellid = ?", (look,)).fetchone()
            self.assertEqual(p, (name, cast, recast, 0, 1, 60) + lp)
            e = db.execute("SELECT line, fxdict, effects FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
            lfx = db.execute("SELECT fxdict FROM spelleffect WHERE spellid = ?", (look,)).fetchone()[0]
            self.assertEqual((e[0], e[1], json.loads(e[2])), (line, lfx, effects))
        tempest = json.loads(db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (fix.TEMPEST,)).fetchone()[0])
        self.assertEqual((tempest["scope"], tempest["radius"], [x["amount"] for x in tempest["effects"]]),
                         ("target_aoe", 15, [-1700, -1300]))
        t = db.execute("SELECT special, special_damage, special_every, special_first, special_jitter, special_chance, note "
                       "FROM classtemplates WHERE name = 'pet badi elder'").fetchone()
        self.assertEqual(t[:6], ("Critical Hit", 33.3333, 30.0, 3.0, 0.0, 0))
        self.assertEqual(round(60 * t[1]), 2000)
        # each row's gm_audit line and its log row
        audit = db.execute("SELECT action, tbl, rowkey, before, after FROM gm_audit ORDER BY id").fetchall()
        self.assertEqual(len(audit), 21)
        self.assertEqual(sorted({(a, t) for a, t, *_ in audit}),
                         [("insert", "spelleffect"), ("insert", "spellpattern"), ("update", "classtemplates"),
                          ("update", "petstats")])
        for action, tbl, rk, before, after in audit:
            self.assertEqual(before is None, action == "insert")
            self.assertEqual(json.loads(after), fix.row_of(db, tbl, json.loads(rk)))
        log = db.execute(f"SELECT tbl, audit FROM {fix.LOG} ORDER BY seq").fetchall()
        self.assertEqual(len(log), 22)
        self.assertEqual(log[-1], ("sqlite_sequence", 0))
        self.assertEqual(db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'spellpattern'").fetchone()[0], fix.LAYHANDS)

    def test_a_second_apply_has_nothing_to_do(self):
        self.run_tool("--apply")
        was = self.dump()
        p = self.run_tool("--apply")
        self.assertIn("0 rows to write, 16 done already", p.stdout)
        self.assertEqual(self.dump(), was)

    def test_undo_gives_back_the_database(self):
        was = self.dump()
        audit = self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        self.run_tool("--apply")
        p = self.run_tool("--undo")
        self.assertIn("put back 22 rows, 0 left", p.stdout)
        self.assertEqual(self.dump(), was)
        self.assertEqual(self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0], audit + 42)  # 21 lines each way
        self.assertIn("nothing logged", self.run_tool("--undo").stdout)

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--apply")
        db = self.db()
        db.execute("UPDATE petstats SET wpn1_dmg = 900 WHERE spellid = ?", (fix.BADI,))  # a GM's, after
        p = self.run_tool("--undo")
        self.assertIn("put back 21 rows, 1 left", p.stdout)
        self.assertEqual(db.execute("SELECT wpn1_dmg, wpn2_dmg FROM petstats WHERE spellid = ?", (fix.BADI,)).fetchone(), (900, 881))
        self.assertEqual(self.kit(fix.EFREETI), fix.norm(fix.OLD[fix.EFREETI][0]))
        self.assertEqual(db.execute(f"SELECT tbl, rowkey FROM {fix.LOG}").fetchall(), [("petstats", '{"spellid":701086}')])

    def test_the_vizier_with_its_root_on_takes_the_kit_too(self):
        self.db().execute("UPDATE petstats SET abilities = ? WHERE spellid = ?", (json.dumps(fix.OLD[fix.VIZIER][1]), fix.VIZIER))
        self.run_tool("--apply")
        self.assertEqual(self.kit(fix.VIZIER), fix.norm(fix.NEW[fix.VIZIER]))

    def test_a_kit_written_by_the_panel_reads_the_same(self):
        # Go's encoding of the same abilities (its field order, whole numbers) is what this expects
        go = [{"kind": "nuke", "spell_id": 902162, "every": 6, "first": 1}, {"kind": "nuke", "spell_id": 902163, "every": 12, "first": 3},
              {"kind": "buff", "spell_id": 902164, "every": 1800, "first": 5, "target": "self"}]
        self.db().execute("UPDATE petstats SET abilities = ? WHERE spellid = ?", (json.dumps(go), fix.EFREETI))
        self.run_tool("--apply")
        self.assertEqual(self.kit(fix.EFREETI), fix.norm(fix.NEW[fix.EFREETI]))

    def refused(self, why):
        was = self.dump()
        p = self.run_tool("--apply", ok=False)
        self.assertNotEqual(p.returncode, 0)
        self.assertIn("refused, nothing done", p.stderr)
        self.assertIn(why, p.stderr)
        self.assertEqual(self.dump(), was)

    def test_a_row_a_gm_changed_is_refused(self):
        kit = fix.OLD[fix.GHILAN][0][:-1]  # its tide taken off
        self.db().execute("UPDATE petstats SET abilities = ? WHERE spellid = ?", (json.dumps(kit), fix.GHILAN))
        self.refused("Ghilan Noble)'s abilities are")

    def test_weapons_a_gm_changed_are_refused(self):
        self.db().execute("UPDATE petstats SET wpn2_dmg = 700 WHERE spellid = ?", (fix.EFREETI,))
        self.refused("Efreeti Lord)'s weapons are (660, 700)")

    def test_a_vizier_of_its_own_size_is_refused(self):
        self.db().execute("UPDATE petstats SET size = 1.2 WHERE spellid = ?", (fix.VIZIER,))
        self.refused("size is 1.2")

    def test_a_taken_id_is_refused(self):
        self.db().execute("INSERT INTO spellpattern VALUES (?, 1, 0, 0, 0, 0, 0, 0, 0, 1, 1, -1, 'Someone Else''s', '')",
                          (fix.INFERNO,))
        self.refused(f"spell id {fix.INFERNO} is taken")

    def test_a_taken_name_is_refused(self):
        self.db().execute("INSERT INTO spellpattern VALUES (999001, 1, 0, 0, 0, 0, 0, 0, 0, 1, 1, -1, 'vizier''s bolt', '')")
        self.refused("a spell named Vizier's Bolt is there already (999001)")

    def test_a_template_another_pet_shares_is_refused(self):
        self.db().execute("INSERT INTO pettemplate (spell, template) VALUES ('summon badi', 'pet badi elder')")
        self.refused("is the template of ['summon badi'] too")

    def test_a_template_with_a_special_is_refused(self):
        self.db().execute("UPDATE classtemplates SET special_every = 20, special_damage = 5 WHERE name = 'pet badi elder'")
        self.refused("has a special already")

    def test_a_missing_pet_row_is_refused(self):
        self.db().execute("DELETE FROM petstats WHERE spellid = ?", (fix.GHILAN,))
        self.refused("no row for Summon Ghilan Noble")


if __name__ == "__main__":
    unittest.main()
