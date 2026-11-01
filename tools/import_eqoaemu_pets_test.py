#!/usr/bin/env python3
"""import_eqoaemu_pets.py on a database and a workbook of its own (made-up
pets, spells and ids, none the game's): a pet joined to its summoning spell
through the Effects sheet's PetID, its body and weapons byte-reversed, its
numbers, stances and weapon kinds read, its spells made abilities by what
they do (a nuke, an area nuke on its target, a taunt of hate alone, a heal of
either, a lasting heal a buff on the owner, a drain of the enemy's stats a
life tap of them), a pettemplate
row for a magician's pet by its element and none over one there; the apply's
gm_audit lines, a GM's petstats row replaced (the workbook wins on all) and
put back by --undo, the same plan again writing nothing, and --undo exact; and the schema the same as the server's."""
import json
import os
import re
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_eqoaemu_pets as ip  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))


def pet(pid, name, sprite, **kw):
    row = {"PetID": pid, "SpellName": name, "PetName": name, "Sprite": sprite, "Settings": "Passive, Defensive, Neutral, Aggressive",
           "Pet Level": "10", "Type": "Melee", "Element": "Fire", "WeaponSpriteID1": "", "WeaponSpriteID2": "",
           "AC": "300", "Base STR": "40", "Base STA": "41", "Base AGI": "42", "Base DEX": "43", "Base WIS": "44",
           "Base INT": "45", "Base CHA": "46", "Base HP": "500", "Base POW": "600", "HoT": "11", "PoT": "12",
           "AR": "1", "CR": "2", "LR": "3", "FR": "4", "PR": "5", "DR": "6", "STR Max": "400", "STA Max": "400",
           "AGI Max": "400", "DEX Max": "400", "WIS Max": "400", "INT Max": "400", "CHA Max": "400",
           "HP Factor": "20", "OFF Mod": "1", "DEF Mod": "2", "WPN1 DMG": "120", "WPN1 Type": "Slash",
           "WPN2 DMG": "0", "WPN2 Type": "", "Spell1": "", "Spell2": "", "Spell3": "", "Spell4": ""}
    row.update(kw)
    return row


def eff(eid, typ, scope, target, **kw):
    row = {"EffectID": eid, "TYPE": typ, "Scope": scope, "Target Type": target, "HP1": "", "Duration": "0", "PetID": ""}
    row.update(kw)
    return row


class ImportPetsTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(os.path.join(tempfile.mkdtemp(), "t.db"), isolation_level=None)
        x = self.db.execute
        x("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        x("INSERT INTO accountinfo VALUES (1, 'dab')")
        x("CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, spellname TEXT)")
        x("INSERT INTO spellpattern VALUES (10, 8, 'Fiery Thing')")      # pet A's summoning, ours
        x("INSERT INTO spellpattern VALUES (20, 30, 'Summon Stone')")    # pet B's, ours: has a pettemplate row
        x("INSERT INTO spellpattern VALUES (950, 12, 'Pet Strike')")     # a pet spell already here
        x("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, modelid INTEGER, primary_weapon INTEGER, secondary_weapon INTEGER)")
        x("INSERT INTO npcs VALUES (1, 'a fiery thing', ?, 0, 0)", (0x44332211,))
        x("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, model INTEGER)")
        x("INSERT INTO itempattern VALUES (1, ?)", (0xDDCCBBAA,))
        x("CREATE TABLE classtemplates (name TEXT PRIMARY KEY)")
        for n in ("pet fire", "pet earth", "pet water", "pet air", "pet necromancer", "pet animation"):
            x("INSERT INTO classtemplates VALUES (?)", (n,))
        x(ip.PETTEMPLATE_SCHEMA)
        x("INSERT INTO pettemplate (spell, weapon, template, source) VALUES ('summon stone', 0, 'pet earth', 'seed')")
        self.sheets = {
            "Pets": [
                pet("0001", "Fiery Thing", "11223344", Spell1="0901", Spell2="0902", Spell3="0903",
                    WeaponSpriteID1="AABBCCDD", **{"WPN2 DMG": "110", "WPN2 Type": "Pierce"}),
                pet("0002", "Summon Stone", "55667788", Type="Tank", Element="Earth", Settings="Passive",
                    Spell1="0904", Spell2="0905", Spell3="0906"),
                pet("0003", "Brand New", "99AABBCC", Type="Non-Combat", Element="Earth", **{"Base HP": "x"}),
            ],
            "Spells": [
                {"SpellID": "0010", "Name": "Fiery Thing", "CLASS": "MAG", "Level": "8", "EffectID1": "0A01"},
                {"SpellID": "0011", "Name": "Summon Stone", "CLASS": "MAG", "Level": "30", "EffectID1": "0A02"},
                {"SpellID": "0012", "Name": "Brand New", "CLASS": "MAG", "Level": "40", "EffectID1": "0A03"},
                {"SpellID": "0901", "Name": "Pet Strike", "Recast": "6", "EffectID1": "0B01"},
                {"SpellID": "0902", "Name": "Pet Burst", "Recast": "12", "EffectID1": "0B02"},
                {"SpellID": "0903", "Name": "Pet Hex", "Recast": "60", "EffectID1": "0B03"},
                {"SpellID": "0904", "Name": "Pet Taunt", "Recast": "1", "EffectID1": "0B04"},
                {"SpellID": "0905", "Name": "Pet Mend", "Recast": "30", "EffectID1": "0B05"},
                {"SpellID": "0906", "Name": "Pet Ward", "Recast": "1800", "EffectID1": "0B06"},
            ],
            "Effects": [
                eff("0A01", "Summon Pet", "Self", "Self", PetID="0001"),
                eff("0A02", "Summon Pet", "Self", "Self", PetID="0002"),
                eff("0A03", "Summon Pet", "Self", "Self", PetID="0003"),
                eff("0B01", "Dmg/Heal", "Target", "Enemy", HP1="-50"),
                eff("0B02", "Dmg/Heal", "TB AoE", "Enemy", HP1="-80", HP2="-20"),
                eff("0B03", "Drain", "Target", "Enemy", STR="-10", AC="-30"),
                eff("0B04", "Modify Hate", "Target", "Enemy", Hate="100"),
                eff("0B05", "Dmg/Heal", "Target", "Owner/Self", HP1="200"),
                eff("0B06", "Dmg/Heal", "Target", "Owner", HPMAX="120", Duration="600"),
            ],
        }
        self.smap = {"0010": 10, "0011": 20, "0012": 900018, "0901": 950, "0902": 902306, "0903": 902307,
                     "0904": 902308, "0905": 902309, "0906": 902310}

    def plan(self):
        return ip.plan(self.db, self.sheets, self.smap, models={0x44332211, 0x88776655})

    def test_plan(self):
        rows, pts, notes, counts = self.plan()
        by = {r["sheet_pet_id"]: r for r in rows}
        a, b, c = by["0001"], by["0002"], by["0003"]
        self.assertEqual((a["spellid"], b["spellid"], c["spellid"]), (10, 20, 900018))
        self.assertEqual((a["model"], a["primary_model"], a["secondary_model"]), (0x44332211, 0xDDCCBBAA, 0))
        self.assertEqual((a["level"], a["type"], a["ac"], a["hp"], a["power"], a["hot"], a["pot"]), (10, "melee", 300, 500, 600, 11, 12))
        self.assertEqual((a["str"], a["intelligence"], a["ar"], a["dr"], a["str_max"], a["hp_factor"], a["def_mod"]), (40, 45, 1, 6, 400, 20, 2))
        self.assertEqual((a["wpn1_dmg"], a["wpn1_type"], a["wpn2_dmg"], a["wpn2_type"]), (120, "slash", 110, "pierce"))
        self.assertEqual((a["stances"], b["stances"], c["type"]), ("", "passive", "non-combat"))
        self.assertEqual(json.loads(a["abilities"]), [
            {"every": 6.0, "first": 1.0, "kind": "nuke", "spell_id": 950},
            {"every": 12.0, "first": 3.0, "kind": "nuke", "spell_id": 902306},
            # a drain of the enemy's stats a life tap of them (2026-11-01: a creature's stats count)
            {"every": 60.0, "first": 5.0, "kind": "lifetap", "spell_id": 902307}])
        self.assertNotIn("a debuff", a["note"])
        self.assertNotIn("an area spell", a["note"])  # its area lands round its target too (pettemplate.go)
        self.assertEqual(json.loads(b["abilities"]), [
            {"every": 2.0, "first": 1.0, "kind": "taunt", "hits": [[1, 0, 0]]},
            {"every": 30.0, "first": 3.0, "kind": "heal", "spell_id": 902309, "target": "either", "below": 60},
            # the magician's pets buff themselves (the user, 2026-11-01), the sheet's "Owner" notwithstanding
            {"every": 1800.0, "first": 5.0, "kind": "buff", "spell_id": 902310, "target": "self"}])
        self.assertEqual(c["hp"], 0, "a cell not read is 0")
        self.assertTrue(any("0003" in n and "Base HP" in n for n in notes))
        self.assertEqual(counts["bodies on the disc"], 2)
        self.assertEqual(counts["bodies NOT on the disc"], 1)
        self.assertEqual(counts["summoning spells not in the database yet (the spell import makes them)"], 1)
        self.assertEqual(counts["abilities pinning a spell in the database"], 1)
        # a magician's pets by their element; Summon Stone's own row kept
        self.assertEqual([(p["spell"], p["template"], p["swing_class"]) for p in pts],
                         [("fiery thing", "pet fire", -1), ("brand new", "pet earth", -1)])

    def test_roots_buffs_and_fixes(self):
        """A root at the enemy a root; a wizard's pet's buff its owner's, a magician's its own; a spell the
        operator's fixes name in error cast as the right one."""
        sheets = {
            "Pets": [pet("0011", "Test Vizier", "11223344", Spell1="0951", Spell2="0952"),
                     pet("0012", "Test Familiar", "11223344", Type="Non-Combat", Spell1="0952"),
                     pet("0013", "Test Noble", "11223344", Spell1="0953")],
            "Spells": [
                {"SpellID": "0021", "Name": "Test Vizier", "CLASS": "MAG", "Level": "55", "EffectID1": "0A11"},
                {"SpellID": "0022", "Name": "Test Familiar", "CLASS": "WIZ", "Level": "8", "EffectID1": "0A12"},
                {"SpellID": "0023", "Name": "Test Noble", "CLASS": "MAG", "Level": "55", "EffectID1": "0A13"},
                {"SpellID": "0951", "Name": "Test Root", "Recast": "30", "EffectID1": "0B51"},
                {"SpellID": "0952", "Name": "Test Grace", "Recast": "1800", "EffectID1": "0B52"},
                {"SpellID": "0953", "Name": "Test Slip", "Recast": "15", "EffectID1": "0B53"},
                {"SpellID": "0954", "Name": "Test Water", "Recast": "12", "EffectID1": "0B54"},
            ],
            "Effects": [
                eff("0A11", "Summon Pet", "Self", "Self", PetID="0011"),
                eff("0A12", "Summon Pet", "Self", "Self", PetID="0012"),
                eff("0A13", "Summon Pet", "Self", "Self", PetID="0013"),
                eff("0B51", "Statistic Absolute", "TB AoE", "Enemy", SPEED="-50", Duration="10"),
                eff("0B52", "Statistic Change", "Owner", "Friendly", HPMAX="250", Duration="1800"),
                eff("0B53", "Dmg/Heal", "Target", "Enemy", HP1="-10"),
                eff("0B54", "Dmg/Heal", "Target", "Enemy", HP1="-440"),
            ],
        }
        smap = {"0021": 30, "0022": 31, "0023": 32, "0951": 9701, "0952": 9702, "0953": 9703, "0954": 9704}
        rows, _, _, counts = ip.plan(self.db, sheets, smap, fixes={("0013", "0953"): "0954"})
        by = {r["sheet_pet_id"]: json.loads(r["abilities"]) for r in rows}
        self.assertEqual([(a["kind"], a.get("spell_id"), a.get("target")) for a in by["0011"]],
                         [("root", 9701, None), ("buff", 9702, "self")])
        self.assertEqual([(a["kind"], a.get("spell_id"), a.get("target")) for a in by["0012"]], [("buff", 9702, "owner")])
        self.assertEqual([(a["kind"], a.get("spell_id")) for a in by["0013"]], [("nuke", 9704)])
        self.assertEqual(counts["pet spells put right by the fixes file"], 1)
        with tempfile.NamedTemporaryFile("w", suffix=".tsv", delete=False) as f:
            f.write("# PetID\tsheet SpellID\tright SpellID\tnote\n0013\t0953\t0954\ta test\n")
        self.assertEqual(ip.read_pet_fixes(f.name), {("0013", "0953"): "0954"})

    def table(self, tbl, keycol):
        cur = self.db.execute(f"SELECT * FROM {tbl} ORDER BY {keycol}")
        return [tuple(r) for r in cur.fetchall()]

    def test_apply_undo(self):
        rows, pts, _, _ = self.plan()
        self.db.execute(ip.PETSTATS_SCHEMA)
        # a GM's row for Summon Stone's spell is there already: the workbook wins on all
        self.db.execute("INSERT INTO petstats (spellid, name, hp, source) VALUES (20, 'a GM pet', 77, 'gm')")
        before = (self.table("petstats", "spellid"), self.table("pettemplate", "spell"))
        written, same, replaced = ip.apply(self.db, rows, pts)
        self.assertEqual((written, same, replaced), (5, 0, 1))
        self.assertEqual(self.db.execute("SELECT name, hp, source FROM petstats WHERE spellid = 20").fetchone(),
                         ("Summon Stone", 500, "eqoaemu"))
        got = self.db.execute("SELECT name, model, wpn1_type, source FROM petstats WHERE spellid = 10").fetchone()
        self.assertEqual(got, ("Fiery Thing", 0x44332211, "slash", "eqoaemu"))
        a = self.db.execute("SELECT username, via, action, tbl, rowkey, before FROM gm_audit WHERE rowkey = '{\"spellid\":10}'").fetchone()
        self.assertEqual(a, ("dab", "tool", "insert", "petstats", '{"spellid":10}', None))
        # the same plan again writes nothing
        self.assertEqual(ip.apply(self.db, rows, pts), (0, 5, 0))
        # a GM edits one of ours after: undo leaves it, puts the rest back
        self.db.execute("UPDATE petstats SET hp = 1 WHERE spellid = 900018")
        back, left = ip.undo(self.db)
        self.assertEqual((back, left), (4, 1))  # the GM's row for spell 20 back
        self.assertEqual(self.db.execute("SELECT hp FROM petstats WHERE spellid = 900018").fetchone(), (1,))
        self.db.execute("DELETE FROM petstats WHERE spellid = 900018")
        self.assertEqual((self.table("petstats", "spellid"), self.table("pettemplate", "spell")), before)

    def test_schema_is_the_servers(self):
        with open(os.path.join(HERE, "..", "internal", "database", "petstats.go"), encoding="utf-8") as f:
            go = f.read()
        cols = lambda s: re.findall(r"^\s+(\w+)\s+(INTEGER|TEXT)", s, re.M)
        self.assertEqual(cols(go.split("petStatsSchema = `")[1].split("`")[0]), cols(ip.PETSTATS_SCHEMA))

    def test_rev32(self):
        self.assertEqual(ip.rev32("A1B2C3D4"), 0xD4C3B2A1)
        self.assertEqual(ip.rev32("0x11223344"), 0x44332211)
        self.assertEqual(ip.rev32(""), 0)
        self.assertEqual(ip.rev32("123"), 0)


if __name__ == "__main__":
    unittest.main()
