#!/usr/bin/env python3
"""fix_eqoaemu_pet_abilities.py on a database and sheets of its own (made-up pets,
spells and ids, none the game's): each pet row still as the import wrote it takes
the importer's abilities now (a drain of the enemy's stats a life tap, a root at
the enemy a root, a magician's pet's buff on itself, a spell the operator's fixes
name in error the right one), a row a GM changed since left; the drains' records
of no time given the first rank's; every petstats change with its gm_audit line;
a second apply writing nothing, and --undo exact."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_eqoaemu_pet_abilities as fa  # noqa: E402
import import_eqoaemu_pets as ip  # noqa: E402


def pet(pid, name, *spells):
    row = {"PetID": pid, "SpellName": name, "PetName": name, "Sprite": "11223344", "Settings": "Defensive",
           "Pet Level": "30", "Type": "Melee", "Element": "", "WeaponSpriteID1": "", "WeaponSpriteID2": ""}
    for i in range(4):
        row[f"Spell{i + 1}"] = spells[i] if i < len(spells) else ""
    return row


def eff(eid, typ, scope, target, **kw):
    row = {"EffectID": eid, "TYPE": typ, "Scope": scope, "Target Type": target, "HP1": "", "Duration": "0", "PetID": ""}
    row.update(kw)
    return row


class PetAbilitiesTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(os.path.join(tempfile.mkdtemp(), "t.db"), isolation_level=None)
        x = self.db.execute
        x("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        x("INSERT INTO accountinfo VALUES (1, 'dab')")
        x("CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, spellname TEXT)")
        x("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, modelid INTEGER, primary_weapon INTEGER, secondary_weapon INTEGER)")
        x("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, model INTEGER)")
        x("CREATE TABLE classtemplates (name TEXT PRIMARY KEY)")
        x(ip.PETTEMPLATE_SCHEMA)
        x(ip.PETSTATS_SCHEMA)
        x(ip.AUDIT_SCHEMA)
        x(ip.LOG_SCHEMA)
        x("CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT, effects TEXT)")
        bane = {"v": 1, "scope": "target", "harmful": True, "effects": [{"kind": "drain", "stat": "str", "amount": -30}]}
        x("INSERT INTO spelleffect VALUES (9010, '', ?)", (json.dumps(dict(bane, duration_s=60)),))
        x("INSERT INTO spelleffect VALUES (9011, '', ?)", (json.dumps(bane),))
        self.sheets = {
            "Pets": [pet("0001", "Test Flesh", "0A01", "0A10"), pet("0002", "Test Death", "0A01", "0A11"),
                     pet("0003", "Test Vizier", "0A20", "0A21"), pet("0004", "Test Noble", "0A22"),
                     pet("0005", "Test Edited", "0A01", "0A10")],
            "Spells": [
                {"SpellID": "0011", "Name": "Test Flesh", "CLASS": "NEC", "EffectID1": "0C01"},
                {"SpellID": "0012", "Name": "Test Death", "CLASS": "NEC", "EffectID1": "0C02"},
                {"SpellID": "0013", "Name": "Test Vizier", "CLASS": "MAG", "EffectID1": "0C03"},
                {"SpellID": "0014", "Name": "Test Noble", "CLASS": "MAG", "EffectID1": "0C04"},
                {"SpellID": "0015", "Name": "Test Edited", "CLASS": "NEC", "EffectID1": "0C05"},
                {"SpellID": "0A01", "Name": "Test Strike", "Recast": "15", "EffectID1": "0B01"},
                {"SpellID": "0A10", "Name": "Test Bane 1", "Recast": "60", "Duration": "60", "EffectID1": "0B10"},
                {"SpellID": "0A11", "Name": "Test Bane 2", "Recast": "60", "EffectID1": "0B11"},
                {"SpellID": "0A20", "Name": "Test Root", "Recast": "30", "EffectID1": "0B20"},
                {"SpellID": "0A21", "Name": "Test Grace", "Recast": "1800", "EffectID1": "0B21"},
                {"SpellID": "0A22", "Name": "Test Slip", "Recast": "15", "EffectID1": "0B22"},
                {"SpellID": "0A23", "Name": "Test Water", "Recast": "12", "EffectID1": "0B23"},
            ],
            "Effects": [eff(f"0C0{i}", "Summon Pet", "Self", "Self", PetID=f"000{i}") for i in range(1, 6)] + [
                eff("0B01", "Dmg/Heal", "Target", "Enemy", HP1="-50"),
                eff("0B10", "Drain", "Target", "Enemy", HP1="0"), eff("0B11", "Drain", "Target", "Enemy", HP1="0"),
                eff("0B20", "Statistic Absolute", "TB AoE", "Enemy", SPEED="-50", Duration="10"),
                eff("0B21", "Statistic Change", "Owner", "Friendly", HPMAX="250", Duration="1800"),
                eff("0B22", "Dmg/Heal", "Target", "Enemy", HP1="-10"),
                eff("0B23", "Dmg/Heal", "Target", "Enemy", HP1="-440"),
            ],
        }
        self.smap = {"0011": 101, "0012": 102, "0013": 103, "0014": 104, "0015": 105, "0A01": 9001, "0A10": 9010,
                     "0A11": 9011, "0A20": 9020, "0A21": 9021, "0A22": 9022, "0A23": 9023}
        self.fixes = {("0004", "0A22"): "0A23"}
        # the pet import as it was: abilities by the old rules, each row logged as written
        old = {101: [{"every": 15.0, "first": 1.0, "kind": "nuke", "spell_id": 9001}],
               102: [{"every": 15.0, "first": 1.0, "kind": "nuke", "spell_id": 9001}],
               103: [{"every": 1800.0, "first": 1.0, "kind": "buff", "spell_id": 9021, "target": "owner"}],
               104: [{"every": 15.0, "first": 1.0, "kind": "nuke", "spell_id": 9022}],
               105: [{"every": 15.0, "first": 1.0, "kind": "nuke", "spell_id": 9001}]}
        for sid, ab in old.items():
            x("INSERT INTO petstats (spellid, sheet_pet_id, name, abilities, source) VALUES (?, ?, ?, ?, 'eqoaemu')",
              (sid, f"000{sid - 100}", f"pet {sid}", json.dumps(ab, separators=(",", ":"))))
            row = ip.row_of(self.db, "petstats", {"spellid": sid})
            x(f"INSERT INTO {ip.LOG} (tbl, rowkey, before, after, audit) VALUES ('petstats', ?, NULL, ?, 0)",
              (ip.go_json({"spellid": sid}), ip.go_json(row)))
        # a GM changed one since
        x("UPDATE petstats SET abilities = '[]' WHERE spellid = 105")

    def tables(self):
        return [tuple(r) for r in self.db.execute("SELECT spellid, abilities FROM petstats ORDER BY spellid")] + \
               [tuple(r) for r in self.db.execute("SELECT * FROM spelleffect ORDER BY spellid")]

    def test_plan_apply_undo(self):
        before = self.tables()
        plan, notes = fa.make_plan(self.db, self.sheets, self.smap, self.fixes)
        got = {r["spellid"]: [(a["kind"], a.get("spell_id"), a.get("target")) for a in json.loads(r["after"])] for r in plan["rows"]}
        self.assertEqual(got[101], [("nuke", 9001, None), ("lifetap", 9010, None)])
        self.assertEqual(got[102], [("nuke", 9001, None), ("lifetap", 9011, None)])
        self.assertEqual(got[103], [("root", 9020, None), ("buff", 9021, "self")])
        self.assertEqual(got[104], [("nuke", 9023, None)])
        self.assertNotIn(105, got, "a row a GM changed since is left")
        self.assertTrue(any("105" in n and "a GM's" in n for n in notes))
        self.assertEqual([(t["spellid"], t["duration_s"]) for t in plan["times"]], [(9011, 60)])
        self.db.execute("BEGIN")
        self.assertEqual(fa.apply(self.db, plan), 5)
        self.db.execute("COMMIT")
        self.assertEqual(self.db.execute("SELECT count(*) FROM gm_audit WHERE tbl = 'petstats'").fetchone(), (4,))
        self.db.execute("BEGIN")
        self.assertEqual(fa.apply(self.db, plan), 0, "a second run writes nothing")
        self.db.execute("COMMIT")
        again, _ = fa.make_plan(self.db, self.sheets, self.smap, self.fixes)
        self.assertEqual((again["rows"], again["times"]), ([], []))
        self.db.execute("BEGIN")
        self.assertEqual(fa.undo(self.db), (5, 0))
        self.db.execute("COMMIT")
        self.assertEqual(self.tables(), before)
        self.assertIsNone(self.db.execute("SELECT name FROM sqlite_master WHERE name = ?", (fa.LOG,)).fetchone())


if __name__ == "__main__":
    unittest.main()
