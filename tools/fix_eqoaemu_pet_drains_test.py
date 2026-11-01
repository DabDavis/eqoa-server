#!/usr/bin/env python3
"""fix_eqoaemu_pet_drains.py on a database and sheets of its own (made-up pets,
spells and ids, none the game's): a pet's drain of the enemy's stats added to
its row as a life tap of them, after its abilities; the drain's record of no
time given its line's first rank's; a pet without one, and a drain there
already, left alone; a second apply writing nothing, and --undo exact."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_eqoaemu_pet_drains as fd  # noqa: E402


def pet(pid, name, *spells):
    row = {"PetID": pid, "PetName": name, "Spell1": "", "Spell2": "", "Spell3": "", "Spell4": ""}
    for i, s in enumerate(spells):
        row[f"Spell{i + 1}"] = s
    return row


def spell(sid, name, eff, duration="0", recast="60"):
    return {"SpellID": sid, "Name": name, "Recast": recast, "Duration": duration, "EffectID1": eff}


def eff(eid, typ, scope, target, **kw):
    row = {"EffectID": eid, "TYPE": typ, "Scope": scope, "Target Type": target, "HP1": "", "Duration": "0", "PetID": ""}
    row.update(kw)
    return row


class PetDrainsTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(os.path.join(tempfile.mkdtemp(), "t.db"), isolation_level=None)
        x = self.db.execute
        x("CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, sheet_pet_id TEXT, name TEXT, abilities TEXT)")
        x("CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT, effects TEXT)")
        hit = [{"every": 15.0, "first": 1.0, "kind": "nuke", "spell_id": 9001}]
        x("INSERT INTO petstats VALUES (100, '0001', 'Test Flesh', ?)", (json.dumps(hit),))
        x("INSERT INTO petstats VALUES (200, '0002', 'Test Death', ?)", (json.dumps(hit),))
        x("INSERT INTO petstats VALUES (300, '0003', 'Test Bones', ?)", (json.dumps(hit),))
        bane = {"v": 1, "scope": "target", "harmful": True, "effects": [{"kind": "drain", "stat": "str", "amount": -30}]}
        x("INSERT INTO spelleffect VALUES (9010, '', ?)", (json.dumps(dict(bane, duration_s=60)),))
        x("INSERT INTO spelleffect VALUES (9011, '', ?)", (json.dumps(bane),))
        self.sheets = {
            "Pets": [pet("0001", "Test Flesh", "0A01", "0A10"), pet("0002", "Test Death", "0A01", "0A11"),
                     pet("0003", "Test Bones", "0A01")],
            "Spells": [spell("0A01", "Test Strike", "0B01"), spell("0A10", "Test Bane 1", "0B10", duration="60"),
                       spell("0A11", "Test Bane 2", "0B11")],
            "Effects": [eff("0B01", "Dmg/Heal", "Target", "Enemy", HP1="-50"),
                        eff("0B10", "Drain", "Target", "Enemy", HP1="0"), eff("0B11", "Drain", "Target", "Enemy", HP1="0")],
        }
        self.smap = {"0A01": 9001, "0A10": 9010, "0A11": 9011}

    def dump(self):
        return [r for r in self.db.execute("SELECT * FROM petstats ORDER BY spellid")] + \
               [r for r in self.db.execute("SELECT * FROM spelleffect ORDER BY spellid")]

    def test_plan_apply_undo(self):
        before = self.dump()
        plan, notes = fd.make_plan(self.db, self.sheets, self.smap)
        self.assertEqual([(r["spellid"], r["add"]["kind"], r["add"]["spell_id"]) for r in plan["rows"]],
                         [(100, "lifetap", 9010), (200, "lifetap", 9011)])
        self.assertEqual([(t["spellid"], t["duration_s"]) for t in plan["times"]], [(9011, 60)])
        with self.db:
            self.db.execute("BEGIN")
            self.assertEqual(fd.apply(self.db, plan), 3)
        abilities = json.loads(self.db.execute("SELECT abilities FROM petstats WHERE spellid = 200").fetchone()[0])
        self.assertEqual([a["kind"] for a in abilities], ["nuke", "lifetap"])
        timed = json.loads(self.db.execute("SELECT effects FROM spelleffect WHERE spellid = 9011").fetchone()[0])
        self.assertEqual(timed["duration_s"], 60)
        self.assertEqual(json.loads(self.db.execute("SELECT abilities FROM petstats WHERE spellid = 300").fetchone()[0]),
                         [{"every": 15.0, "first": 1.0, "kind": "nuke", "spell_id": 9001}], "a pet of no drain")
        with self.db:
            self.db.execute("BEGIN")
            self.assertEqual(fd.apply(self.db, plan), 0, "a second run writes nothing")
        again, _ = fd.make_plan(self.db, self.sheets, self.smap)
        self.assertEqual((again["rows"], again["times"]), ([], []), "nothing left to do")
        with self.db:
            self.db.execute("BEGIN")
            back, left = fd.undo(self.db)
        self.assertEqual((back, left), (3, 0))
        self.assertEqual(self.dump(), before)
        self.assertIsNone(self.db.execute("SELECT name FROM sqlite_master WHERE name = ?", (fd.LOG,)).fetchone())


if __name__ == "__main__":
    unittest.main()
