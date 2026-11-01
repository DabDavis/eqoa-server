#!/usr/bin/env python3
"""import_eqoaemu_spells.py on a database and a workbook of its own (made up
here: no row of the real workbook): the effects JSON of each kind of row, the
record's columns (updated, made, a shared record's owner), the ranges and
scopes, the areas, the class rows (a were's none), a vendor spell's scroll and
its merchants, the quest and drop spells listed, the line as the server's
reader takes it, a plan through a file, --apply's log, a second --apply
refused, and --undo exact with a row a GM changed since left."""
import json
import os
import sqlite3
import struct
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_eqoaemu_spells as im  # noqa: E402

SCROLL = im.SCROLL_ICON
F30, F45, F20 = im.range_bits(30), im.range_bits(4.5), im.range_bits(20)


def eff(i, typ, **kw):
    """An Effects-sheet row: every figure 0 unless given."""
    r = {"EffectID": i, "Name": kw.pop("Name", "x"), "TYPE": typ, "Level": "1", "Scope": kw.pop("Scope", "Target"),
         "Target Type": kw.pop("Target_Type", "Enemy"), "Modifier": "Str", "Percentage": "0", "Level Scaling?": "No",
         "%": "0", "Hate": "0", "Duration": "0", "Resist Basis": "None"}
    for c, _ in im.STAT_COLS:
        r[c] = "0"
    for c in ("HP1", "HP2", "DoT", "POW", "HP Factors", "OFF Mods", "DEF Mods", "SPEED"):
        r[c] = "0"
    r.update({k.replace("_", " "): str(v) for k, v in kw.items()})
    return r


def spell(i, name, **kw):
    r = {"SpellID": i, "Name": name, "Learn Method": "Scroll", "From CM#": "", "ScrollSource": "Vendor",
         "CLASS": "WIZ", "RACE": "HUM ELF", "Graphic": "", "TYPE": "Dmg/Heal", "Level": "10", "Range": "30",
         "CastTime": "2", "Recast": "3", "PowerCost": "40", "IconBGDColor": "Red", "Icon": "", "IconID": "11223344",
         "Scope": "Target", "Target Type": "Enemy", "StackType": "", "ArchType": "", "ReqEQuipment": "Any",
         "Duration": "0", "MinLevelEffect": "1", "EffectID1": "", "EffectID2": "", "EffectID3": "", "EffectID4": ""}
    r.update({k.replace("_", " "): str(v) for k, v in kw.items()})
    return r


EFFECTS = [
    eff("0001", "Dmg/Heal", HP1=-100, Modifier="DEX", Percentage=15, Resist_Basis="FR", Level=12),
    eff("0002", "Dmg/Heal", HP1=200, Duration=24, Scope="Target", Target_Type="Friendly"),       # a heal a tick
    eff("0003", "Statistic Change", DoT=50, Duration=30),                                         # a DoT written positive
    eff("0004", "Statistic Absolute", SPEED=-50, Duration=36, Resist_Basis="AR"),                 # a root
    eff("0005", "Statistic Change", STR=10, AC=20, Duration=0, Scope="Self", Target_Type="Self"),
    eff("0006", "Offensive Proc", Duration=300, ApplyProc1="0D01", Scope="Target", Target_Type="Friendly"),
    eff("0D01", "Dmg/Heal", HP1=-30, Resist_Basis="AR"),
    eff("0007", "Defensive Proc", Duration=300, ApplyProc1="0D02", Scope="Self", Target_Type="Self"),
    eff("0D02", "Dmg/Heal", HP1=-8, Resist_Basis="Pierce"),
    eff("0008", "Summon Pet", PetID="0001", Scope="Self", Target_Type="Self"),
    eff("0009", "Create Item", CreatedItemID=20001, QTY=3, Scope="Self", Target_Type="Self"),
    eff("000A", "Dmg/Heal", HP1=-60, HP2=-25, AoERadius=15),                                      # target AoE
    eff("000B", "Statistic Change", HPMAX=40, Duration=1800, AoERadius=30, Scope="Group", Target_Type="Friendly"),
    eff("000C", "Ressurect Player", RezDebtRemoval=-20, AoERadius=30, Scope="Group", Target_Type="Friendly"),
    eff("000D", "Alter Appearance", SpriteID="AABBCCDD", Duration=600, Scope="Self", Target_Type="Self"),
    eff("000E", "Statistic Change", STR=50, Duration=600, Scope="Self", Target_Type="Self"),
    eff("000F", "Modify Hate", Hate=300, Modifier="STR", Percentage=10),
]
SPELLS = [
    spell("0100", "Ember Bolt", EffectID1="0001", Level=12, CastTime=3),                        # ours: 101
    spell("0101", "Kind Mending", EffectID1="0002", Scope="Target", Target_Type="Friendly", CLASS="CL",
          IconBGDColor="Red", Range=1, Duration=24),                                             # ours: 102
    spell("0102", "Shared Name", EffectID1="0003", Level=5, CLASS="NEC"),                       # ours: 103 (its own)
    spell("0103", "Shared Name", EffectID1="0003", Level=30, CLASS="SHA"),                      # ours: 103 (left)
    spell("0104", "Gripping Earth", EffectID1="0004", Duration=36),                             # new, vendor
    spell("0105", "Stance of Iron", EffectID1="0005", Scope="Self", Target_Type="Self", Range=30, Duration=300,
          IconBGDColor="Yellow", ReqEQuipment="1HB,1HS,1HP", CLASS="WAR", RACE="HUM BAR"),     # new, vendor
    spell("0106", "Burning Blade", EffectID1="0006", Scope="Target", Target_Type="Friendly", Duration=300,
          Learn_Method="CM", ScrollSource="", CLASS="ENC"),                                     # new, mastery
    spell("0107", "Thorn Skin", EffectID1="0007", Scope="Self", Target_Type="Self", Duration=300,
          ScrollSource="Quest", CLASS="DRD"),                                                   # new, quest
    spell("0108", "Pet Call", EffectID1="0008", Scope="Target", Target_Type="Friendly", ScrollSource="Drop",
          CLASS="MAG"),                                                                         # new, drop; a pet: self
    spell("0109", "Brew Fire", EffectID1="0009", Scope="Self", Target_Type="Self", CLASS="ALC"),  # new, vendor
    spell("010A", "Fire Storm", EffectID1="000A", Scope="TB AoE", Target_Type="Enemy", IconBGDColor="Green"),
    spell("010B", "Group Warding", EffectID1="000B", Scope="Group", Target_Type="Friendly", Range=20,
          Duration=1800, IconBGDColor="Purple", CLASS="CL"),                                    # ours: 104
    spell("010C", "Raise Fallen", EffectID1="000C", Scope="Group", Target_Type="Friendly", Range="", CLASS="CL"),  # ours: 105
    spell("010D", "Bearwere", EffectID1="000D", Scope="Self", Target_Type="Self", CLASS=" ".join(im.CLASSES),
          ScrollSource="Quest"),                                                                # new, a were form
    spell("010E", "Rage of Claws", EffectID1="000E", Scope="Self", Target_Type="Self", CLASS="RAN MNK"),  # a were quest's
    spell("010F", "Pet Strike", EffectID1="0001", CLASS="", Learn_Method="", ScrollSource=""),   # a pet's
    spell("0110", "Sure Taunt", EffectID1="000F", Level=20, Range=20, Recast=6),                 # ours: 106, Sandstorm's
]
MAP = {"0100": (101, "name"), "0101": (102, "name"), "0102": (103, "name"), "0103": (103, "name"),
       "0104": (900260, "new"), "0105": (900261, "new"), "0106": (900262, "new"), "0107": (900263, "new"),
       "0108": (900264, "new"), "0109": (900265, "new"), "010A": (900266, "new"), "010B": (104, "name"),
       "010C": (105, "name"), "010D": (900269, "new"), "010E": (900270, "new"), "010F": (900271, "new"),
       "0110": (106, "name")}
WORKBOOK = {"Spells": SPELLS, "Effects": EFFECTS,
            "Created Item Info": [{"ItemID": "20001", "Name": "Fire Mix Potion"}],
            "Pets": [{"PetID": "0001", "PetName": "Fire Imp"}]}
WERES = {im.quest_fold("Rage of Claws"): ("Rage of Claws", "Weres/wolfwere/L40-melee")}


class ImportTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        self.db = sqlite3.connect(self.path, isolation_level=None)
        x = self.db.execute
        x("""CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL,
             unk2 INTEGER NOT NULL, unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL,
             power INTEGER NOT NULL, iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL,
             recast INTEGER NOT NULL, eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL)""")
        x("CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL)")
        x("CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid))")
        x("""CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL,
             unk1 INTEGER NOT NULL, itemicon INTEGER NOT NULL, unk2 INTEGER NOT NULL, equipslot INTEGER NOT NULL,
             unk3 INTEGER NOT NULL, trade INTEGER NOT NULL, rent INTEGER NOT NULL, unk4 INTEGER NOT NULL,
             attacktype INTEGER NOT NULL, weapondamage INTEGER NOT NULL, unk5 INTEGER NOT NULL, levelreq INTEGER NOT NULL,
             maxstack INTEGER NOT NULL, maxhp INTEGER NOT NULL, duration INTEGER NOT NULL, classuse INTEGER NOT NULL,
             raceuse INTEGER NOT NULL, procanim INTEGER NOT NULL, lore INTEGER NOT NULL, unk6 INTEGER NOT NULL,
             craft INTEGER NOT NULL, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL, model INTEGER NOT NULL,
             color INTEGER NOT NULL, str INTEGER)""")
        x("""CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL,
             stackleft INTEGER NOT NULL DEFAULT 0, remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0,
             patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1,
             listnumber INTEGER NOT NULL DEFAULT 0)""")
        x("CREATE TABLE sandstorm_import_rows (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, key INTEGER, action TEXT, old TEXT)")
        x("CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER)")
        x("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER)")
        x("CREATE TABLE queststeps (questid INTEGER, step INTEGER, rewarditems TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '')")
        rec = "INSERT INTO spellpattern VALUES (?, ?, 0, 0, ?, ?, ?, ?, ?, ?, ?, -1, ?, ?)"
        x(rec, (101, 10, F30, 2, 40, 486870879, 111, 1, 3, "Ember Bolt", "Single target fire spell."))
        x(rec, (102, 4, F30, 3, 20, 486870879, 222, 1, 1, "Kind Mending", "Heals the target."))
        x(rec, (103, 5, F30, 3, 20, 486870879, 333, 1, 1, "Shared Name", "Ours."))
        x(rec, (104, 20, F30, 3, 60, 593297002, 444, 2, 5, "Group Warding", "Wards the group."))
        x(rec, (105, 20, F30, 5, 100, 486870879, 555, 1, 1, "Raise Fallen", "A rez."))
        x(rec, (106, 20, im.range_bits(15), 0, 30, 486870879, 666, 1, 6, "Sure Taunt", "Taunts."))
        x("INSERT INTO sandstorm_import_rows (tbl, key, action) VALUES ('spellpattern', 106, 'inserted')")
        x("INSERT INTO spelleffect VALUES (101, 'Target, Decrease Hitpoints by 90', 1234, 'eqoa-data wizard')")
        x("INSERT INTO spelleffect VALUES (104, 'Group, Increase Maximum HP by 30, 30min', 0, 'eqoa-data cleric')")
        x("INSERT INTO classspells VALUES (11, 103, 'dump')")
        x("INSERT INTO classspells VALUES (13, 101, 'dump')")
        # a wizard's scroll vendor (npc 50: levels 1-20, three scrolls), one full (npc 60), our potion
        item = ("INSERT INTO itempattern VALUES (?, ?, 0, ?, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, 1, 0, 0, ?, 1023, 2, 0, 0, 0, ?, '', 0, 0, NULL)")
        for pid, lv, cost in ((1, 1, 10), (2, 12, 160), (3, 20, 370)):
            x(item, (pid, cost, SCROLL, -1, lv, 1 << 13, f"Scroll {pid}"))
            x("INSERT INTO sandstorm_import_rows (tbl, key, action) VALUES ('itempattern', ?, 'inserted')", (pid,))
            x("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (50, 1, ?, -1, 1, ?)", (pid, pid))
            x("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (60, 1, ?, -1, 1, ?)", (pid, pid))
        for k in range(37):
            x("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (60, 1, 1, -1, 1, ?)", (10 + k,))
        x(item, (7, 5, 999, 0, 1, 1 << 14, "Fire Potion"))
        # the warrior's and the alchemist's vendors
        x(item, (4, 10, SCROLL, -1, 1, 1, "War Scroll A"))
        x(item, (5, 20, SCROLL, -1, 4, 1, "War Scroll B"))
        x(item, (6, 40, SCROLL, -1, 8, 1, "War Scroll C"))
        for pid in (4, 5, 6):
            x("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (70, 1, ?, -1, 1, ?)", (pid, pid))
        self.wb = im.Workbook(WORKBOOK)

    def plan(self, **kw):
        return im.plan(self.db, self.wb, MAP, weres=WERES, **kw)

    def dump(self):
        out = {}
        for (t,) in self.db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
            out[t] = (self.db.execute(f"PRAGMA table_info({t})").fetchall(),
                      sorted(map(repr, self.db.execute(f"SELECT * FROM {t}").fetchall())))
        return out

    def by_id(self, p, key, sid):
        return [e for e in p[key] if e["spellid"] == sid]

    def effects(self, p, sid):
        e = self.by_id(p, "effects", sid)[0]
        return json.loads((e.get("row") or e.get("set"))["effects"])

    def line(self, p, sid):
        e = self.by_id(p, "effects", sid)[0]
        return (e.get("row") or e.get("set"))["line"]

    def test_effects_json(self):
        p = self.plan()
        j = self.effects(p, 101)
        self.assertEqual((j["v"], j["scope"], j["harmful"], j["level"]), (1, "target", True, 12))
        self.assertEqual(j["effects"], [{"id": "0001", "type": "Dmg/Heal", "kind": "hp", "amount": -100,
                                         "scale": "dex", "scale_pct": 15, "resist": "fr"}])
        heal = self.effects(p, 102)["effects"][0]
        self.assertEqual((heal["kind"], heal["amount"]), ("hp_tick", 200), "a heal with a duration ticks")
        dot = self.effects(p, 103)["effects"][0]
        self.assertEqual((dot["kind"], dot["amount"], dot["sheet_dot"]), ("hp_tick", -50, 50))
        root = self.effects(p, 900260)["effects"][0]
        self.assertEqual((root["kind"], root["amount"], root["absolute"], root["resist"]), ("speed", -50, True, "ar"))
        stance = self.effects(p, 900261)["effects"]
        self.assertEqual([(o["kind"], o["stat"], o["amount"]) for o in stance], [("stat", "str", 10), ("stat", "ac", 20)])
        self.assertTrue(all("duration_s" not in o for o in stance), "a row's 0 is its spell's time")
        proc = self.effects(p, 900262)["effects"][0]
        self.assertEqual(proc["kind"], "proc")
        self.assertEqual(proc["procs"], [{"id": "0D01", "type": "Dmg/Heal", "kind": "hp", "amount": -30, "resist": "ar",
                                          "on": "offense", "scope": "target", "target_type": "Enemy"}])
        self.assertEqual(self.effects(p, 900263)["effects"][0]["procs"][0]["on"], "defense")
        self.assertEqual(self.effects(p, 900264)["effects"][0]["pet"], {"sheet_pet_id": "0001", "name": "Fire Imp"})
        self.assertEqual(self.effects(p, 900265)["effects"][0]["item"],
                         {"count": 3, "name": "Fire Mix Potion", "patternid": 7}, "an item ours by our name for it")
        aoe = self.effects(p, 900266)
        self.assertEqual((aoe["scope"], aoe["radius"]), ("target_aoe", 15))
        self.assertEqual([o["kind"] for o in aoe["effects"]], ["hp", "hp2"])

    def test_records(self):
        p = self.plan()
        up = {e["spellid"]: e for e in p["spells"] if e["action"] == "update"}
        self.assertEqual(up[101]["set"], {"abilitylvl": 12, "casttime": 3})
        self.assertEqual(up[102]["set"].get("range"), F45, "the workbook's 1 on a target spell is touch")
        self.assertNotIn("abilitylvl", up.get(103, {}).get("set", {}), "the record's own row is its level's")
        self.assertIn("0103 Shared Name SHA L30 -> 103", " ".join(p["lists"]["several rows on one record: not applied (the map's to settle)"]))
        self.assertEqual(up[104]["set"].get("range"), F20)
        self.assertNotIn("scope", up.get(105, {}).get("set", {}), "a rez keeps its scope")
        self.assertNotIn("range", up.get(105, {}).get("set", {}), "no range given: ours")
        self.assertEqual(up[106]["set"], {"casttime": 2, "power": 40, "range": F20}, "a Sandstorm record takes the workbook's too")
        new = {e["spellid"]: e["row"] for e in p["spells"] if e["action"] == "insert"}
        r = new[900261]
        self.assertEqual((r["scope"], r["range"], r["iconColor"], r["eqprequire"]), (0, F45, -1809115846, 21))
        self.assertEqual(r["icon"], struct.unpack("<i", struct.pack(">I", 0x11223344))[0])
        self.assertEqual(new[900264]["scope"], 0, "a pet is made beside its caster")
        self.assertEqual(new[900266]["iconColor"], im.FRAMES["Green"])
        self.assertEqual(new[900269]["spelldesc"], "Assume the form of the bearwere.")
        self.assertTrue(new[900270]["spelldesc"].startswith("Werewolf melee ability."))
        self.assertTrue(im.is_were_ability(new[900270]["spellname"], new[900270]["spelldesc"]))
        self.assertIn("chance to deal additional", new[900262]["spelldesc"], "a weapon proc says so")
        for sid, r in new.items():
            self.assertNotRegex(r["spelldesc"].lower(), r"\bproc\b", sid)

    def test_areas_and_classes(self):
        p = self.plan()
        areas = {a["spellid"]: a["row"] if a["action"] == "insert" else a["set"] for a in p["areas"]}
        self.assertEqual((areas[900266]["centre"], areas[900266]["radius"]), (1, 15.0))
        self.assertEqual((areas[104]["centre"], areas[104]["radius"]), (2, 30.0))
        self.assertNotIn(105, areas, "a rez picks its own")
        links = {(x["cls"], x["spellid"]): x["source"] for x in p["links"]}
        self.assertEqual(links[(13, 900260)], "eqoaemu")
        self.assertEqual(links[(12, 900262)], "cm (eqoaemu)")
        self.assertFalse([k for k in links if k[1] in (900269, 900270, 900271)], "a were's and a pet's are no class's")
        self.assertFalse([k for k in links if k[1] < 900000], "ours keep their rows")

    def test_scrolls(self):
        p = self.plan()
        scrolls = {x["patternid"]: x["row"] for x in p["scrolls"]}
        self.assertEqual(sorted(scrolls), [900260, 900261, 900265, 900266])
        s = scrolls[900260]
        self.assertEqual((s["itemicon"], s["equipslot"], s["levelreq"], s["classuse"], s["raceuse"]),
                         (SCROLL, -1, 10, 1 << 13, 3))
        self.assertEqual(s["itemcost"], round(10 + (160 - 10) * 9 / 11), "on the scroll curve, between levels 1 and 12")
        stock = {(x["npcid"], x["patternid"]) for x in p["stock"]}
        self.assertIn((50, 900260), stock)
        self.assertNotIn((60, 900260), stock, "a full list takes no more")
        self.assertIn((70, 900261), stock)
        self.assertIn("900263 Thorn Skin (DRD L10)", p["lists"]["a new quest spell no quest gives (Scroll + Quest)"])
        self.assertIn("900264 Pet Call (MAG L10)", p["lists"]["a new drop spell (Scroll + Drop; no loot changed)"])

    def test_lines(self):
        p = self.plan()
        self.assertEqual(self.line(p, 101), "Target, Decrease Hitpoints by 100 ; Increase 15% based on DEX")
        self.assertEqual(self.line(p, 102), "Target, Increase HP per tic by 200, 24sec")
        self.assertEqual(self.line(p, 900260), "Target, Root, 36sec")
        self.assertEqual(self.line(p, 900262), "Target, Decrease Hitpoints by 30, 5min", "a weapon proc's hit, its desc says proc")
        self.assertEqual(self.line(p, 900263), "Self, Damage Shield Decrease Hitpoints by 8, 5min")
        self.assertEqual(self.line(p, 900264), "Summons a Fire Imp to aid you")
        self.assertEqual(self.line(p, 900265), "Self, Summon Fire Mix Potion x3")
        self.assertEqual(self.line(p, 900266), "Target, Decrease Hitpoints by 60 aoe Decrease Hitpoints by 25")
        self.assertEqual(self.line(p, 104), "Group, Increase Maximum HP by 40, Radius 30, 30min")
        self.assertEqual(self.line(p, 106), "Target, Increase Hate by 300 ; Increase 10% based on STR")

    def test_apply_undo(self):
        before = self.dump()
        p = self.plan()
        path = os.path.join(os.path.dirname(self.path), "plan.json")
        with open(path, "w") as f:
            json.dump(p, f)
        with open(path) as f:
            p2 = json.load(f)
        done, stale = im.apply(self.db, p2)
        self.assertEqual(stale, 0)
        self.assertGreater(done, 20)
        self.assertIn("effects", im.columns(self.db, "spelleffect"))
        r = self.db.execute("SELECT fxdict, source FROM spelleffect WHERE spellid = 101").fetchone()
        self.assertEqual(r, (1234, "eqoa-data wizard; effects from EQOAEmu's workbook"), "the visual left alone")
        self.assertEqual(self.db.execute("SELECT icon, spellname FROM spellpattern WHERE spellid = 101").fetchone(),
                         (111, "Ember Bolt"), "our icon and name kept")
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcitem WHERE patternid >= 900000").fetchone()[0],
                         len(p["stock"]))
        with self.assertRaises(SystemExit):
            im.apply(self.db, p2)
        # a GM edits a row the import wrote; undo leaves that one
        self.db.execute("UPDATE spellpattern SET power = 999 WHERE spellid = 900260")
        back, left, leftover = im.undo(self.db)
        self.assertEqual((left, leftover), (1, ["spellpattern 900260"]))
        self.db.execute("DELETE FROM spellpattern WHERE spellid = 900260")
        self.db.execute("UPDATE sqlite_sequence SET seq = (SELECT max(spellid) FROM spellpattern) WHERE name = 'spellpattern'")
        self.db.execute("ALTER TABLE spelleffect DROP COLUMN effects")
        self.assertEqual(self.dump(), before, "all but the GM's row put back")

    def test_undo_exact(self):
        before = self.dump()
        im.apply(self.db, self.plan())
        back, left, _ = im.undo(self.db)
        self.assertEqual(left, 0)
        self.assertEqual(self.dump(), before)

    def test_undo_refused_while_held(self):
        im.apply(self.db, self.plan())
        self.db.execute("INSERT INTO spells (serverid, spellid) VALUES (7, 900260)")
        with self.assertRaises(SystemExit):
            im.undo(self.db)
        back, left, _ = im.undo(self.db, force=True)
        self.assertEqual(left, 0)
        self.assertEqual(self.db.execute("SELECT count(*) FROM spells").fetchone()[0], 0)

    def test_helpers(self):
        self.assertEqual(im.range_of(im.range_bits(4.5)), 4.5)
        self.assertEqual(im.equip_mask("Any"), -1)
        self.assertEqual(im.equip_mask("None"), 0)
        self.assertEqual(im.equip_mask("1HB,1HS,1HP, 2HB,2HS,2HP"), 63)
        self.assertEqual(im.equip_mask("Range"), 64)
        self.assertEqual(im.spell_key("Anthem of Hope +"), im.spell_key("Anthem of Hope"))
        self.assertEqual(im.quest_fold("Hunter's Scorn"), im.quest_fold("Hunters Scorn"))
        self.assertTrue(im.is_were_ability("Lionwere", ""))
        self.assertFalse(im.is_were_ability("Lion's Nobility", "Heals you."))
        self.assertEqual(im.scroll_spell([(1, "Anthem of Hope +", 20), (9, "Anthem of Hope", 20)], "Anthem of Hope", 20), 1,
                         "scribe.go's key: the first of the level")


if __name__ == "__main__":
    unittest.main()
