#!/usr/bin/env python3
"""tools/fix_pets_1110.py against a made-up database of the tables it reads and
writes (the server's schemas; the Efreeti Lord's and the Ghilan Noble's rows as
tools/fix_mage_pets_1450.py leaves them): a report writes nothing; --apply
gives the Efreeti Lord its Efreeti's Fire on its owner after its own (1478),
makes Ghilan's Healing Tide with Tide of Glory's look and gives the Ghilan Noble
its group heal (1495), and writes the two npcsocial rows (1493), each row with
its gm_audit line and its log row; --only does the petitions named; a second
--apply has nothing to do; --undo gives back the database as it was but for
gm_audit's lines, and leaves a row changed since; a pet's row not as expected, a
spell id or name taken, an npcsocial row there already or a name no NPC has is
refused with nothing written.

    python3 tools/fix_pets_1110_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_pets_1110.py")
sys.path.insert(0, HERE)
import fix_mage_pets_1450 as base  # noqa: E402
import fix_pets_1110 as fix  # noqa: E402
import import_eqoaemu_pets as pets  # noqa: E402

SCHEMA = """
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
    effects TEXT NOT NULL DEFAULT '');
CREATE TABLE npcsocial (npc_name TEXT PRIMARY KEY, social INTEGER NOT NULL DEFAULT 1, faction TEXT NOT NULL DEFAULT '',
    radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, modelid INTEGER, size REAL);
"""
TIDE_LOOK = (16880, 8123, 9123, 0x0B0000AA)  # Tide of Glory's range, icon colour, icon and visual, made up


class FixPets1110Test(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        for s in (pets.PETSTATS_SCHEMA, pets.AUDIT_SCHEMA):
            db.execute(s)
        db.execute("ALTER TABLE petstats ADD COLUMN size REAL NOT NULL DEFAULT 0")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        for sid, name, look in ((fix.FIRE, "Efreeti's Fire", (16528, 1, 2, 3)), (fix.TIDE_LOOK[0], fix.TIDE_LOOK[1], TIDE_LOOK),
                                (905457, "Ghilan's Lay Hands", (16880, 4, 5, 6))):
            db.execute("INSERT INTO spellpattern VALUES (?, 49, 0, 0, ?, 5, 100, ?, ?, 2, 12, -1, ?, '')",
                       (sid, look[0], look[1], look[2], name))
            db.execute("INSERT INTO spelleffect VALUES (?, 'Group, Increase Hitpoints by 550', ?, 'test', '')", (sid, look[3]))
        for sid in (fix.EFREETI, fix.GHILAN):
            _, sheet, name = base.SUMMONS[sid]
            db.execute("INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, abilities, note, source) VALUES "
                       "(?, ?, ?, 70000, 60, ?, 'its row.', 'eqoaemu')", (sid, sheet, name, json.dumps(base.NEW[sid])))
        db.executemany("INSERT INTO npcs (npc_name, modelid, size) VALUES (?, 1, 1.0)",
                       [("ice goblin shaman",), ("Vigilant Spirit",), ("Vigilant Spirit",), ("Tae Ew hierophant",)])
        db.execute("INSERT INTO npcsocial VALUES ('Mummified Guardian', 1, 'Urd of Guk', 20.0, 'petition 1305', 'petition 1305')")
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
        return json.loads(self.db().execute("SELECT abilities FROM petstats WHERE spellid = ?", (sid,)).fetchone()[0])

    def test_a_report_writes_nothing(self):
        was = self.dump()
        p = self.run_tool()
        self.assertIn("6 rows to write, 0 done already", p.stdout)
        self.assertEqual(self.dump(), was)

    def test_apply_writes_the_three_petitions(self):
        self.run_tool("--apply")
        db = self.db()
        # 1478: the Efreeti Lord's own buff kept, the same spell on its owner after it
        k = self.kit(fix.EFREETI)
        self.assertEqual(k[:-1], json.loads(json.dumps(base.NEW[fix.EFREETI])))
        self.assertEqual(k[-1], {"every": 1800.0, "first": 7.0, "kind": "buff", "spell_id": fix.FIRE, "target": "owner"})
        self.assertEqual([a.get("target") for a in k if a["kind"] == "buff"], ["self", "owner"])
        # 1495: the spell with its look's range, icon and visual, and the heal of the group, seventh
        p = db.execute("SELECT spellname, casttime, recast, power, scope, abilitylvl, range, iconColor, icon FROM spellpattern "
                       "WHERE spellid = ?", (fix.TIDE,)).fetchone()
        self.assertEqual(p, ("Ghilan's Healing Tide", 0, 6, 0, 1, 60) + TIDE_LOOK[:3])
        e = db.execute("SELECT line, fxdict, effects FROM spelleffect WHERE spellid = ?", (fix.TIDE,)).fetchone()
        self.assertEqual(e[:2], ("Target, Increase Hitpoints by 150", TIDE_LOOK[3]))
        self.assertEqual([x["amount"] for x in json.loads(e[2])["effects"]], [150])
        g = self.kit(fix.GHILAN)
        self.assertEqual(len(g), 7)
        self.assertEqual(g[:6], json.loads(json.dumps(base.NEW[fix.GHILAN])))
        self.assertEqual(g[6], {"every": 6.0, "first": 2.0, "kind": "heal", "spell_id": fix.TIDE, "target": "group", "below": 100})
        note = db.execute("SELECT note FROM petstats WHERE spellid = ?", (fix.GHILAN,)).fetchone()[0]
        self.assertTrue(note.startswith("its row. Petition 1495"))
        # 1493: the spirit social, its shaman of its faction and helping no one; Guk's row as it was
        rows = db.execute("SELECT npc_name, social, faction, radius, source FROM npcsocial ORDER BY npc_name").fetchall()
        self.assertEqual(rows, [("Mummified Guardian", 1, "Urd of Guk", 20.0, "petition 1305"),
                                ("Vigilant Spirit", 1, fix.FACTION, 20.0, "petition 1493"),
                                ("ice goblin shaman", 0, fix.FACTION, 0.0, "petition 1493")])
        # every row logged with its gm_audit line, the spells' id counter too
        logged = db.execute(f"SELECT tbl, audit FROM {fix.LOG} ORDER BY seq").fetchall()
        self.assertEqual([t for t, _ in logged], ["spellpattern", "spelleffect", "petstats", "petstats", "npcsocial",
                                                  "npcsocial", "sqlite_sequence"])
        audits = db.execute("SELECT id, tbl, action, detail FROM gm_audit ORDER BY id").fetchall()
        self.assertEqual(len(audits), 6)
        self.assertEqual({a for _, a in logged if a}, {r[0] for r in audits})
        self.assertTrue(all("fix_pets_1110" in r[3] for r in audits))
        # a second apply has nothing to do
        p = self.run_tool("--apply")
        self.assertIn("0 rows to write, 5 done already", p.stdout)

    def test_only_does_the_petitions_named(self):
        self.run_tool("--only", "1493", "--apply")
        self.assertEqual(self.kit(fix.EFREETI), json.loads(json.dumps(base.NEW[fix.EFREETI])))
        self.assertEqual(len(self.kit(fix.GHILAN)), 6)
        self.assertEqual(self.db().execute("SELECT count(*) FROM npcsocial").fetchone()[0], 3)
        self.run_tool("--only", "1478", "--apply")
        self.assertEqual(len(self.kit(fix.EFREETI)), 3)
        self.assertIsNone(self.db().execute("SELECT 1 FROM spellpattern WHERE spellid = ?", (fix.TIDE,)).fetchone())
        self.assertNotEqual(self.run_tool("--only", "1484", ok=False).returncode, 0)

    def test_undo_gives_the_database_back(self):
        was = self.dump()
        self.run_tool("--apply")
        self.assertNotEqual(self.dump(), was)
        p = self.run_tool("--undo")
        self.assertIn("put back 7 rows, 0 left", p.stdout)
        self.assertEqual(self.dump(), was)

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--apply")
        self.db().execute("UPDATE npcsocial SET radius = 35 WHERE npc_name = 'Vigilant Spirit'")
        p = self.run_tool("--undo")
        self.assertIn("put back 6 rows, 1 left", p.stdout)
        self.assertEqual(self.db().execute("SELECT radius FROM npcsocial WHERE npc_name = 'Vigilant Spirit'").fetchone()[0], 35)
        self.assertEqual(len(self.kit(fix.GHILAN)), 6)

    def refused(self, sql, words, *args):
        self.db().execute(sql)
        was = self.dump()
        p = self.run_tool("--apply", *args, ok=False)
        self.assertNotEqual(p.returncode, 0)
        self.assertIn("refused, nothing done", p.stderr)
        self.assertIn(words, p.stderr)
        self.assertEqual(self.dump(), was)

    def test_a_pets_row_a_gm_changed_is_refused(self):
        self.refused(f"UPDATE petstats SET abilities = '[]' WHERE spellid = {fix.EFREETI}", "not what tools/fix_mage_pets_1450.py left")

    def test_a_spell_id_taken_is_refused(self):
        self.refused(f"INSERT INTO spellpattern VALUES ({fix.TIDE}, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Another', '')", "is taken")

    def test_a_spell_name_taken_is_refused(self):
        self.refused("INSERT INTO spellpattern VALUES (77, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'ghilan''s healing tide', '')",
                     "is there already")

    def test_a_social_row_there_already_is_refused(self):
        self.refused("INSERT INTO npcsocial VALUES ('vigilant spirit', 0, 'Mine', 0, 'a GM''s', 'gm')", "a GM's, left")

    def test_a_name_no_npc_has_is_refused(self):
        self.refused("DELETE FROM npcs WHERE npc_name = 'ice goblin shaman'", "no NPC is named")


if __name__ == "__main__":
    unittest.main()
