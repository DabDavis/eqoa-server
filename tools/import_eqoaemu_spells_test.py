#!/usr/bin/env python3
"""import_eqoaemu_spells.py on a database and a workbook of its own (made up
here: no row of the real workbook): the effects JSON of each kind of row, the
record's columns (updated, made, a shared record's owner), the ranges and
scopes, the areas, the class rows (a were's none), a vendor spell's scroll and
its merchants, the quest and drop spells listed, the line as the server's
reader takes it, a plan through a file, --apply's log, a second --apply
refused, and --undo exact with a row a GM changed since left."""
import collections
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
    eff("0010", "Teleport", Location="Tunaria,100.5,50,200.25", AoERadius=30, Scope="Group", Target_Type="Friendly"),
    eff("0011", "Teleport", Location="Odus,7,8,9", Scope="Group", Target_Type="Friendly"),
    eff("0012", "Teleport", Location="Odus,7,8,9", Scope="Group", Target_Type="Friendly"),
]
SPELLS = [
    spell("0100", "Ember bolt", EffectID1="0001", Level=12, CastTime=3),                        # ours: 101, renamed
    spell("0101", "Kind Mending", EffectID1="0002", Scope="Target", Target_Type="Friendly", CLASS="CL",
          IconBGDColor="Red", Range=1, Duration=24),                                             # ours: 102
    spell("0102", "Shared Name", EffectID1="0003", Level=5, CLASS="NEC"),                       # ours: 103 (its own)
    spell("0103", "Shared Name", EffectID1="0003", Level=30, CLASS="SHA"),                      # 103 split: 900259
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
    spell("0111", "Hearth Portal", EffectID1="0010", Scope="Group", Target_Type="Friendly", ScrollSource="Quest"),
    spell("0112", "Moon Portal", EffectID1="0011", Scope="Group", Target_Type="Friendly", ScrollSource="Quest"),
    spell("0113", "Sun Escape", EffectID1="0012", Scope="Group", Target_Type="Friendly", ScrollSource="Quest"),
    spell("0114", "Pour Cider", EffectID1="0013", Scope="Self", Target_Type="Self", CLASS="MAG", ScrollSource="Quest"),
]
MAP = {"0100": (101, "name"), "0101": (102, "name"), "0102": (103, "name"), "0103": (900259, "split", 103),
       "0111": (900273, "new"), "0112": (900274, "new"), "0113": (900275, "new"), "0114": (900276, "new"),
       "0104": (900260, "new"), "0105": (900261, "new"), "0106": (900262, "new"), "0107": (900263, "new"),
       "0108": (900264, "new"), "0109": (900265, "new"), "010A": (900266, "new"), "010B": (104, "name"),
       "010C": (105, "name"), "010D": (900269, "new"), "010E": (900270, "new"), "010F": (900271, "new"),
       "0110": (106, "name")}
def item(i, name, item_id, level, *effects):
    r = {"SpellID": i, "Name": name, "ItemID": str(item_id), "Level": str(level), "Scope": "Self", "Target Type": "Self"}
    for k, e in enumerate(effects, 1):
        r[f"EffectID{k}"] = e
    return r


ITEM_EFFECTS = [
    eff("0E01", "Statistic Change", PoT=8, Duration=120, Scope="Self", Target_Type="Self", DisspellOnCast=1,
        DisspellOnContact=1),
    eff("0E02", "Statistic Change", PoT=30, Duration=120, Scope="Self", Target_Type="Self"),
    eff("0E03", "Statistic Change", PoT=10, HP1=10, Duration=180, Scope="Self", Target_Type="Self"),
    eff("0E04", "Statistic Change", HP1=6, Duration=120, Scope="Self", Target_Type="Self", DisspellOnCast=1),
    eff("0E05", "Statistic Change", HoT=20, Duration=54, Scope="Self", Target_Type="Self"),
    eff("0E06", "Statistic Change", HP1=20, Duration=0, Scope="Self", Target_Type="Self"),
]
ITEMS = [
    {"ItemID": "20001", "Name": "Fire Mix Potion"},
    item("0901", "Well Water Effect", 30001, 5, "0E01"),          # ours: a drink, PoT 5
    item("0902", "Sweet Cider effect", 30002, 20, "0E02"),        # made as Well Water is
    item("0903", "Holy Fish Effect", 30003, 25, "0E03"),          # made, a meal with both
    item("0904", "Wild Berries", 30004, 5, "0E04"),               # ours: a meal, HoT 2
    item("0905", "Calm Concoction Effect", 30005, 30, "0E05"),    # a heal now and over time: a potion
    item("0906", "Calm Concoction Effect", 30005, 30, "0E06"),
]
WORKBOOK = {"Spells": SPELLS, "Effects": EFFECTS + [eff("0013", "Create Item", CreatedItemID=30002, QTY=3, Scope="Self",
                                                         Target_Type="Self")],
            "Created Item Info": ITEMS, "Created Item Effects": ITEM_EFFECTS,
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
             color INTEGER NOT NULL, str INTEGER, HoT INTEGER, PoT INTEGER)""")
        x("""CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
             icon INTEGER NOT NULL DEFAULT 0, breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0,
             area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm')""")
        x("""CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL,
             stackleft INTEGER NOT NULL DEFAULT 0, remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0,
             patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1,
             listnumber INTEGER NOT NULL DEFAULT 0)""")
        x("CREATE TABLE sandstorm_import_rows (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, key INTEGER, action TEXT, old TEXT)")
        x("CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER)")
        x("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER)")
        x("CREATE TABLE queststeps (questid INTEGER, step INTEGER, rewarditems TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step))")
        x("CREATE TABLE quests (questid INTEGER PRIMARY KEY, classes TEXT NOT NULL DEFAULT '')")
        x("CREATE TABLE characters (serverid INTEGER PRIMARY KEY, tclass INTEGER NOT NULL)")
        x("""CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, classes INTEGER NOT NULL, name TEXT NOT NULL DEFAULT '',
             rank INTEGER NOT NULL DEFAULT 1, add_spells TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '',
             remove_spells TEXT NOT NULL DEFAULT '')""")
        x("""CREATE TABLE charbuffs (serverid INTEGER NOT NULL, seq INTEGER NOT NULL, spellid INTEGER NOT NULL,
             caster INTEGER NOT NULL, PRIMARY KEY (serverid, seq))""")
        # the split: 103 is the necromancer's (5) and the shaman's (8) today; the shaman's row splits off
        x("INSERT INTO characters VALUES (21, 8), (22, 11)")
        x("INSERT INTO spells (id, serverid, spellid) VALUES (31, 21, 103), (32, 22, 103)")
        x("INSERT INTO charbuffs VALUES (21, 0, 103, 21), (22, 0, 103, 22)")
        x("INSERT INTO cmcatalog VALUES (41, 256, 'Shaman Thing', 1, '103, 777', '', ''), "
          "(42, 256, 'Shaman Other', 2, '', '103', '103'), (43, 2304, 'Both', 1, '103', '', '')")
        x("INSERT INTO quests VALUES (51, '8'), (52, ''), (53, '8, 11')")
        x("INSERT INTO queststeps (questid, step, spells, byclass) VALUES (51, 1, '103', ''), "
          "(52, 1, '', '{\"8\":{\"spells\":\"103\"},\"11\":{\"spells\":\"103\"}}'), (53, 1, '103', '')")
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
        x("INSERT INTO classspells VALUES (8, 103, 'vendor')")
        x("INSERT INTO classspells VALUES (13, 101, 'dump')")
        # a wizard's scroll vendor (npc 50: levels 1-20, three scrolls), one full (npc 60), our potion
        item = ("INSERT INTO itempattern VALUES (?, ?, 0, ?, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, 1, 0, 0, ?, 1023, 2, 0, 0, 0, ?, '', 0, 0, "
                "NULL, NULL, NULL)")
        for pid, lv, cost in ((1, 1, 10), (2, 12, 160), (3, 20, 370)):
            x(item, (pid, cost, SCROLL, -1, lv, 1 << 13, f"Scroll {pid}"))
            x("INSERT INTO sandstorm_import_rows (tbl, key, action) VALUES ('itempattern', ?, 'inserted')", (pid,))
            x("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (50, 1, ?, -1, 1, ?)", (pid, pid))
            x("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (60, 1, ?, -1, 1, ?)", (pid, pid))
        for k in range(37):
            x("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (60, 1, 1, -1, 1, ?)", (10 + k,))
        x(item, (7, 5, 999, 0, 1, 1 << 14, "Fire Potion"))
        food = ("INSERT INTO itempattern VALUES (?, 0, 0, ?, 0, -1, 0, 1, 0, 0, 0, 0, 0, ?, 50, 0, 50, 32767, 1023, 2, 0, 0, "
                "0, ?, ?, 0, 0, NULL, ?, ?)")
        x(food, (8, 4242, 5, "Well Water", "A refreshing beverage that increases power regeneration.", None, 5))
        x(food, (9, 4343, 5, "wild berries", "Berries of the wild, a meal that will increase health regeneration.", 2, None))
        x(food, (10, 4444, 25, "Weak Draught", "A draught.", None, None))
        x("INSERT INTO itemeffect (patternid, line, source) VALUES (10, 'Self, Increase Power by 10', 'potions')")
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
                         {"count": 3, "name": "Fire Potion", "patternid": 7, "sheet_name": "Fire Mix Potion"},
                         "an item ours by our name for it: under ours, which the server's summon finds it by")
        aoe = self.effects(p, 900266)
        self.assertEqual((aoe["scope"], aoe["radius"]), ("target_aoe", 15))
        self.assertEqual([o["kind"] for o in aoe["effects"]], ["hp", "hp2"])

    def test_records(self):
        p = self.plan()
        up = {e["spellid"]: e for e in p["spells"] if e["action"] == "update"}
        self.assertEqual(up[101]["set"], {"abilitylvl": 12, "casttime": 3, "spellname": "Ember bolt",
                                          "icon": struct.unpack("<i", struct.pack(">I", 0x11223344))[0]},
                         "the workbook's on all: its icon and spelling too (its frame is ours already)")
        self.assertEqual(up[102]["set"].get("range"), F45, "the workbook's 1 on a target spell is touch")
        self.assertNotIn("abilitylvl", up.get(103, {}).get("set", {}), "the record's own row is its level's")
        self.assertNotIn("several rows on one record: not applied (the map's to settle)", p["lists"])
        self.assertEqual(up[104]["set"].get("range"), F20)
        self.assertNotIn("scope", up.get(105, {}).get("set", {}), "a rez keeps its scope")
        self.assertNotIn("range", up.get(105, {}).get("set", {}), "no range given: ours")
        self.assertEqual(up[106]["set"], {"casttime": 2, "power": 40, "range": F20,
                                          "icon": struct.unpack("<i", struct.pack(">I", 0x11223344))[0]},
                         "a Sandstorm record takes the workbook's too")
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

    def test_split(self):
        p = self.plan()
        new = {e["spellid"]: e["row"] for e in p["spells"] if e["action"] == "insert"}
        self.assertEqual((new[900259]["spellname"], new[900259]["abilitylvl"]), ("Shared Name", 30))
        self.assertEqual(p["moves"], [{"cls": 8, "spellid": 103, "source": "vendor"}], "the shaman's row moves")
        self.assertIn({"cls": 8, "spellid": 900259, "source": "vendor"}, p["links"])
        mig = {(m["tbl"], json.dumps(m["key"], sort_keys=True)): (m["before"], m["set"]) for m in p["migrations"]}
        self.assertEqual(mig[("spells", '{"id": 31}')], ({"spellid": 103}, {"spellid": 900259}), "the shaman's book")
        self.assertNotIn(("spells", '{"id": 32}'), mig, "the necromancer's book keeps 103")
        self.assertEqual(mig[("charbuffs", '{"seq": 0, "serverid": 21}')][1], {"spellid": 900259})
        self.assertEqual(mig[("cmcatalog", '{"id": 41}')], ({"add_spells": "103, 777"}, {"add_spells": "900259, 777"}))
        self.assertEqual(mig[("cmcatalog", '{"id": 42}')][1], {"require_spells": "900259", "remove_spells": "900259"})
        self.assertNotIn(("cmcatalog", '{"id": 43}'), mig, "a mastery of both classes is left")
        self.assertIn("43 Both 1 (add_spells): 900259 Shared Name from 103",
                      p["lists"]["a mastery of a split record's classes and others: left naming the shared record"])
        self.assertEqual(mig[("queststeps", '{"questid": 51, "step": 1}')][1], {"spells": "900259"})
        self.assertEqual(json.loads(mig[("queststeps", '{"questid": 52, "step": 1}')][1]["byclass"]),
                         {"8": {"spells": "900259"}, "11": {"spells": "103"}}, "byclass: the shaman's entry alone")
        self.assertNotIn(("queststeps", '{"questid": 53, "step": 1}'), mig, "a quest of both classes is left")

    def test_split_map(self):
        d = os.path.dirname(self.path)
        src, out = os.path.join(d, "map.tsv"), os.path.join(d, "split.tsv")
        with open(src, "w") as f:
            f.write("# a map\nsheet_id\tspellid\thow\tname\tclasses\tlearn\tsource\n"
                    "0102\t103\tname\tShared Name\tNEC\tScroll\tVendor\n0103\t103\tname\tShared Name\tSHA\tScroll\tVendor\n")
        done = im.split_map(self.db, self.wb, src, out)
        self.assertEqual([(h, frm, to) for h, _, _, frm, to in done], [("0103", 103, 900259)])
        m = im.read_map(out)
        self.assertEqual(m["0103"], (900259, "split", 103))
        self.assertEqual(m["0102"], (103, "name", None))
        self.assertEqual(im.split_map(self.db, self.wb, out, out + "2"), [], "a split map splits nothing again")

    def test_gates(self):
        p = self.plan()
        self.assertEqual(p["gates"], [{"spell": "hearth portal", "kind": "group", "place": "Hearth", "world": 0,
                                       "x": 100.5, "y": 50.0, "z": 200.2, "radius": 30.0,
                                       "note": "EQOAEmu's workbook (2020): Location Tunaria,100.5,50,200.25",
                                       "source": "eqoaemu"}])
        self.assertEqual(len(p["lists"]["a new teleport whose Location differently named teleports share "
                                        "(the workbook's stand-in: no row)"]), 2)

    def test_foods(self):
        p = self.plan()
        f = p["foods"]
        self.assertEqual(sorted((x["patternid"], x["set"]) for x in f["updates"]),
                         [(8, {"PoT": 8}), (9, {"HoT": 6})], "ours take the workbook's figures")
        made = {x["patternid"]: x["row"] for x in f["items"]}
        self.assertEqual(sorted(made), [940002, 940003, 940005])
        cider, fish, calm = made[940002], made[940003], made[940005]
        self.assertEqual((cider["itemname"], cider["PoT"], cider["itemicon"], cider["levelreq"]), ("Sweet Cider", 30, 4242, 20),
                         "a drink as our drink, its own figure and level")
        self.assertEqual((fish["HoT"], fish["PoT"], fish["itemicon"]), (10, 10, 4343), "a meal with both, as our meal")
        self.assertRegex(fish["itemdesc"], r"meal.*health regeneration")
        self.assertEqual((calm["itemicon"], calm["HoT"], calm["PoT"]), (4444, None, None), "a potion as our potion")
        self.assertEqual(f["itemeffects"], [{"patternid": 940005, "line": "Self, Increase Hitpoints by 20, Increase HP per tic by 20, 54sec",
                                             "name": "", "icon": 0, "breaks": "", "wear": 0, "area": 0.0, "source": "eqoaemu"}])
        fe = {x["patternid"]: (x["duration_s"], x["ends_on_cast"], x["ends_on_contact"]) for x in f["foodeffects"]}
        self.assertEqual(fe, {8: (120, 1, 1), 940002: (120, 0, 0), 940003: (180, 0, 0), 9: (120, 1, 0)})
        pour = self.effects(p, 900276)["effects"][0]
        self.assertEqual(pour["item"]["patternid"], 940002, "the summoning names the made item")
        self.assertEqual(self.line(p, 900276), "Self, Summon Sweet Cider x3")

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
        self.assertEqual(sorted(scrolls), [900259, 900260, 900261, 900265, 900266])
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
        self.assertEqual(self.line(p, 900265), "Self, Summon Fire Potion x3", "our item's own name")
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
                         (struct.unpack("<i", struct.pack(">I", 0x11223344))[0], "Ember bolt"), "the workbook's icon and name")
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcitem WHERE patternid >= 900000").fetchone()[0],
                         len(p["stock"]))
        with self.assertRaises(SystemExit):
            im.apply(self.db, p2)
        # a GM edits a row the import wrote; undo leaves that one
        self.db.execute("UPDATE spellpattern SET power = 999 WHERE spellid = 900260")
        back, left, leftover = im.undo(self.db)
        self.assertEqual((left, leftover), (1, ['spellpattern {"spellid":900260}']))
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
        self.assertEqual(self.db.execute("SELECT count(*) FROM spells WHERE spellid >= 900000").fetchone()[0], 0)
        self.assertEqual(self.db.execute("SELECT spellid FROM spells WHERE id = 31").fetchone()[0], 103,
                         "the shaman's book back on the shared record")

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
        self.assertEqual(im.scroll_spell([(1, "Anthem of Hope +", 20), (9, "Anthem of Hope", 20)], "Anthem of Hope", 20), 9,
                         "scribe.go's exact name before its key")
        self.assertEqual(im.scroll_spell([(1, "Anthem of Hope +", 20)], "Anthem of Hope", 20), 1, "the key when no exact")
        self.assertEqual(im.scroll_spell([(3, "Hide", 1), (8, "Hide", 6)], "Hide", 1, {8}.__contains__), 8,
                         "the class's record before the level's")
        self.assertTrue(im.rename_safe("Glimmering blades", "Glimmering Blades", self.db), "case alone")
        self.assertFalse(im.rename_safe("Kast Spark", "Kast's Spark", self.db), "another spelling")
        self.db.execute("CREATE TABLE spellgates (spell TEXT PRIMARY KEY)")
        self.db.execute("INSERT INTO spellgates VALUES ('mai`adra''s gate')")
        self.assertFalse(im.rename_safe("Mai`adra's Gate", "Mai'adra's Gate", self.db), "a gate keyed by the backtick")
        self.assertTrue(im.rename_safe("Mai`adra's Hope", "Mai'adra's Hope", self.db), "a backtick no key holds")


class LineItemsTest(unittest.TestCase):
    """Every made or taken item under the name the server finds it by: an item
    of ours by the workbook's name takes ours (a Fizzing Mix Potion is our
    Fizzing Potion), one it names none of ours of, or not at all, takes our line's
    (a cocktail's Grobb Cocktail, Feed's Unit of Blood, the vampires'
    reagents): the server reads the effects alone, so without it a summon
    makes nothing and a reagent is free."""

    NAMES = {709302: "Unit of Blood", 7: "Fire Potion", 50002: "Fizzing Potion", 50025: "Weak Brew",
             61: "Grobb Cocktail", 50104: "foraged herb"}

    def settle(self, j, line):
        ours = {im.fold(n): pid for pid, n in self.NAMES.items()}
        return im.settle_item_names(j, line, ours, self.NAMES)

    def test_named_as_the_line_names_it(self):
        feed = {"effects": [{"kind": "hp", "amount": -1}, {"kind": "create_item", "item": {"count": 1, "sheet_item_id": ""}}]}
        self.assertEqual(self.settle(feed, "Target, Drain HP by 40 to 60 give self, Summon Unit of Blood x1"),
                         ([], ["makes Unit of Blood x1"]))
        self.assertEqual(feed["effects"][1]["item"], {"count": 1, "name": "Unit of Blood", "patternid": 709302})
        gaze = {"effects": [{"kind": "hate", "amount": -500, "consume": {"count": 10, "sheet_item_id": ""}}]}
        self.assertEqual(self.settle(gaze, "Target, Set Aggro to 0, Reagent Unit of Blood x10"), ([], ["takes Unit of Blood x10"]))
        self.assertEqual(gaze["effects"][0]["consume"]["patternid"], 709302)
        # the workbook's count stays (Vitality: our line's 5, its 6)
        vit = {"effects": [{"kind": "stat", "consume": {"count": 6, "sheet_item_id": ""}}]}
        self.settle(vit, "Self, Increase HP by 1000, 60sec, Reagent Unit of Blood x5")
        self.assertEqual(vit["effects"][0]["consume"]["count"], 6)
        # an item the workbook names that is none of ours: our line's, the workbook's name kept
        cocktail = {"effects": [{"kind": "create_item", "item": {"count": 3, "name": "Dark Fire Potion"}}]}
        self.assertEqual(self.settle(cocktail, "SELF Summon Grobb Cocktail"),
                         ([], ["makes Grobb Cocktail x3 (the workbook's 'Dark Fire Potion', none of ours)"]))
        self.assertEqual(cocktail["effects"][0]["item"], {"count": 3, "name": "Grobb Cocktail", "patternid": 61,
                                                          "sheet_name": "Dark Fire Potion"})
        # the line's words as the server's reader takes them
        for line in ("Self, Summon Item: foraged herb", "Summons Foraged Herb X3", "SELF Summon foraged herb (a meal)"):
            j = {"effects": [{"kind": "create_item", "item": {"count": 1, "name": "Wild Roots"}}]}
            self.assertEqual(self.settle(j, line)[1], ["makes foraged herb x1 (the workbook's 'Wild Roots', none of ours)"], line)

    def test_ours_by_the_workbooks_name(self):
        acid = {"effects": [{"kind": "create_item", "item": {"count": 6, "name": "Fizzing Mix Potion", "patternid": 50002}}]}
        self.assertEqual(self.settle(acid, "Summon Fizzing Potion X3"), (["'Fizzing Mix Potion' -> 'Fizzing Potion'"], []))
        self.assertEqual(acid["effects"][0]["item"], {"count": 6, "name": "Fizzing Potion", "patternid": 50002,
                                                      "sheet_name": "Fizzing Mix Potion"})
        # matched here though the plan's item_ref had not (a consume, a name by a variant)
        dr = {"effects": [{"kind": "create_item", "item": {"count": 1, "name": "Weak Brew Potion"}}]}
        self.assertEqual(self.settle(dr, "")[0], ["'Weak Brew Potion' -> 'Weak Brew'"])
        # already ours by our own name: as it is
        fire = {"effects": [{"kind": "create_item", "item": {"count": 1, "name": "Fire Potion", "patternid": 7}}]}
        self.assertEqual(self.settle(fire, "SELF Summon Grobb Cocktail"), ([], []))
        self.assertNotIn("sheet_name", fire["effects"][0]["item"])

    def test_left_unnamed(self):
        for line in ("Self, Creates an item x6",                                    # no name in the line
                     "Self, Summon Ruby Shard x2",                                  # none of ours
                     "Self, Summon Unit of Blood, Summon Fire Potion",              # two: which is which
                     "Group, Summon Player", "Self, Summons a pet", "Summons a zombie to your aid"):
            j = {"effects": [{"kind": "create_item", "item": {"count": 1, "sheet_item_id": ""}}]}
            self.assertEqual(self.settle(j, line), ([], []), line)
            self.assertNotIn("name", j["effects"][0]["item"], line)
        # two made items none of ours: which is which, so neither
        two = {"effects": [{"kind": "create_item", "item": {"count": 1, "name": "A"}},
                           {"kind": "create_item", "item": {"count": 1, "name": "B"}}]}
        self.assertEqual(self.settle(two, "SELF Summon Grobb Cocktail"), ([], []))

    def test_gate_twins(self):
        self.assertTrue(im.one_letter_apart("mossvole circle", "mossvale circle"))
        self.assertTrue(im.one_letter_apart("thornwood circle", "thornwod circle"))
        self.assertFalse(im.one_letter_apart("odus gate", "odus gates"))              # too short to tell
        self.assertFalse(im.one_letter_apart("nerius portal", "melton portal"))
        db = sqlite3.connect(":memory:")
        db.execute("""CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'self',
                      place TEXT NOT NULL DEFAULT '', world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0,
                      y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0,
                      note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")
        db.execute("INSERT INTO spellgates VALUES ('mossvale circle', 'group', 'Saerk Towers', 0, 20333.3, 80.6, "
                   "12333.3, 0, 'seeded', 'seed')")
        lists, counts = collections.defaultdict(list), collections.Counter()
        out = im.plan_gates(db, [dict(spellid=900434, name="Mossvole Circle", location="", group=True, radius=0),
                                 dict(spellid=900429, name="Circle of Dead Hills", location="", group=True, radius=0)],
                            lists, counts)
        self.assertEqual([(g["spell"], g["place"], g["x"], g["source"]) for g in out],
                         [("mossvole circle", "Saerk Towers", 20333.3, "eqoaemu")])
        self.assertEqual(lists["a new teleport with no place in its Location (no spellgates row)"],
                         ["900429 Circle of Dead Hills: none"])


class IconTest(unittest.TestCase):
    """icon_of: the workbook's IconID byte-reversed, unless only the id as
    written is an icon on the disc; neither: none."""

    def test_the_way_round_the_disc_has(self):
        signed = lambda v: struct.unpack("<i", struct.pack("<I", v))[0]  # noqa: E731
        disc = {0x7C84475E, 0x5F49041D}
        self.assertEqual(im.icon_of("0x1D04495F", disc), signed(0x5F49041D), "reversed, as the workbook mostly writes it")
        self.assertEqual(im.icon_of("0x7C84475E", disc), signed(0x7C84475E), "written our way round: taken as written")
        self.assertIsNone(im.icon_of("0x12345678", disc), "no icon either way round")
        self.assertEqual(im.icon_of("0x12345678"), signed(0x78563412), "no disc read: reversed, as before")
        self.assertIsNone(im.icon_of("12 34", disc))


if __name__ == "__main__":
    unittest.main()
