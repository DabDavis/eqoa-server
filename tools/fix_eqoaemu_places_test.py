#!/usr/bin/env python3
"""tools/fix_eqoaemu_places.py against a made-up database and quest file
(no game data: the names, places and ids are made up): a recast set from the
workbook's 3 and one a GM changed left; the gems made (ids of their own),
their uses in the potion reader's words, their gates, loot tables and
bindings, an item of a gem's name already there used and not made again, a
gem whose id another item holds left whole; a quest whose reward is now a
record written again in the game, one whose kill step's creature carries the
NPC bit left a draft; the isle's talked-to NPCs given the NPC bit, one a
quest kills left, its workers' npcaggro row; --apply in one transaction, a
second writing nothing and leaving the database as it was, --undo exact, a
row a GM changed since kept.

    python3 tools/fix_eqoaemu_places_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_eqoaemu_places as fp  # noqa: E402
import import_quests as iq  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, spellname TEXT NOT NULL, recast INTEGER NOT NULL DEFAULT 3,
    abilitylvl INTEGER NOT NULL DEFAULT 1, scope INTEGER NOT NULL DEFAULT 1, iconColor INTEGER NOT NULL DEFAULT 0,
    spelldesc TEXT NOT NULL DEFAULT '');
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL, unk1 INTEGER NOT NULL,
    itemicon INTEGER NOT NULL, unk2 INTEGER NOT NULL, equipslot INTEGER NOT NULL, unk3 INTEGER NOT NULL, trade INTEGER NOT NULL,
    rent INTEGER NOT NULL, unk4 INTEGER NOT NULL, attacktype INTEGER NOT NULL, weapondamage INTEGER NOT NULL, unk5 INTEGER NOT NULL,
    levelreq INTEGER NOT NULL, maxstack INTEGER NOT NULL, maxhp INTEGER NOT NULL, duration INTEGER NOT NULL, classuse INTEGER NOT NULL,
    raceuse INTEGER NOT NULL, procanim INTEGER NOT NULL, lore INTEGER NOT NULL, unk6 INTEGER NOT NULL, craft INTEGER NOT NULL,
    itemname TEXT NOT NULL, itemdesc TEXT NOT NULL, model INTEGER NOT NULL, color INTEGER NOT NULL, str INTEGER, AC INTEGER);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '', icon INTEGER NOT NULL DEFAULT 0,
    breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0, area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'self', place TEXT NOT NULL DEFAULT '',
    world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0,
    radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (npc_name, tablename));
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, modelid INTEGER, npc_type INTEGER);
CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
    patternid INTEGER NOT NULL);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL DEFAULT '', fxdict INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '');
CREATE TABLE eqoaemu_spell_import_log (seq INTEGER PRIMARY KEY, tbl TEXT NOT NULL, key TEXT NOT NULL, action TEXT NOT NULL,
    before TEXT, after TEXT, note TEXT NOT NULL DEFAULT '');
"""

# made up: three spells of ours made the mirror of one the import wrote (6.)
MIRRORS = {"glaring mask": ("soothing mask", 15), "dull mask": ("soothing mask", 5), "odd mask": ("soothing mask", 7)}
SOOTHING = ('{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":300,"stack":1,"level":25,'
            '"min_level":15,"effects":[{"id":"0E30","type":"Offensive Proc","kind":"proc","procs":[{"id":"0E31",'
            '"type":"Modify Hate","kind":"hate","amount":-10,"on":"offense","scope":"target","target_type":"Enemy"}]}]}')

# made up: two spells of ours given effects of ours (7.): one making an item in a maker's frame, one a cure
MAKES = {"chill mixture": ("rime mixture", "Chill Potion", 1), "odd mixture": ("rime mixture", "Chill Potion", 1)}
CURES = {"mending tonic": 3}
RIME = ('{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":46,"min_level":1,'
        '"req_equipment":"Any","sheet_type":"Create Item","effects":[{"id":"0065","type":"Create Item","kind":"create_item",'
        '"item":{"count":3,"name":"Rime Potion","patternid":4780,"sheet_name":"Rime Mix Potion"}}]}')

# made up: two spells that make each other's items, and meals ending as the workbook says (8.)
MADE = {"grub hunt": "Hunted Grub", "fruit hunt": "Hunted Fruit"}
MEALS = {"hunted grub", "hunted fruit", "hunted nut"}


def hunt(level, name, pid, sheet):
    return fp.im_dumps({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "stack": 0, "level": level,
                        "effects": [{"id": "0E50", "type": "Create Item", "kind": "create_item",
                                     "item": {"count": 1, "name": name, "patternid": pid, "sheet_name": sheet}}]})

# made up: four lasting debuffs the workbook gave no time (5.)
DURATIONS = {"mire cloud": 30, "ash veil": 30, "bog mark": 30, "quick rot": 30}


def debuff(stat, time=None):
    j = {"v": 1, "scope": "target", "target_type": "Enemy", "harmful": True}
    if time:
        j["duration_s"] = time
    j.update(stack=0, level=6, effects=[{"id": "0001", "type": "Statistic Change", "kind": "stat", "stat": stat, "amount": -5}])
    return fp.im_dumps(j)

# made up: two gems of a tower, their chambers
CHAMBERS = {"upper": ("Upper Hall of the Glass Spire", (100.0, 20.0, 200.0)),
            "roof": ("Roof of the Glass Spire", (100.0, 40.0, 200.0))}
GEMS = [dict(name="Amber Stone", cracked="Cracked Amber Stone", needs=None, drops=["glass warden"], table="spire wardens",
             to="upper", n=1, of="the glass wardens"),
        dict(name="Opal Stone", cracked="Cracked Opal Stone", needs="Cracked Amber Stone", drops=["glass keeper", "lost one"],
             table="spire keepers", to="roof", n=3, of="the glass keepers")]
RECASTS = {"spire step": 3600, "moon circle": 600, "tide gate": 900}
KEYS = ("Test/spire/port", "Test/spire/blocked")
QUESTS = {"city": "test", "quests": [
    {"key": "Test/spire/port", "name": "The Spire Step", "giver": "Old Warden", "classes": [7], "races": [], "minlevel": 50,
     "maxlevel": 60, "requires": [], "offer": "Bring me two glass shards.", "accept": "I will.", "decline": "No.",
     "declined": "Then go.", "started": "Go.",
     "steps": [{"kind": "loot", "target": "glass warden", "items": [{"item": "glass shard", "count": 2}], "count": 2, "chance": 0.5,
                "journal": "Take two glass shards off the wardens."},
               {"kind": "give", "target": "Old Warden", "items": [{"item": "glass shard", "count": 2}], "journal": "Bring them.",
                "says": "Good.", "rewards": {"spells": ["Spire Step"]}}],
     "unresolved": [], "notes": "made up"},
    {"key": "Test/spire/blocked", "name": "The Shut Spire", "giver": "Old Warden", "classes": [7], "races": [], "minlevel": 50,
     "maxlevel": 60, "requires": [], "offer": "Kill the gate warden.", "accept": "I will.", "decline": "No.",
     "declined": "Then go.", "started": "Go.",
     "steps": [{"kind": "kill", "target": "gate warden", "count": 1, "journal": "Kill the gate warden.",
                "rewards": {"spells": ["Spire Step"]}}],
     "unresolved": [], "notes": "made up"}],
    "items": [{"name": "glass shard", "description": "A shard.", "notrade": True, "like": "Old Stone"}]}
TALKED = ("Sky Clerk", "Sky Guard")
BODY = fp.PORTAL_BODY


def make(dir):
    path = os.path.join(dir, "t.db")
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO spellpattern (spellid, spellname, recast) VALUES (?, ?, ?)",
                   [(901, "Spire Step", 3), (902, "Moon Circle", 45), (903, "Tide Gate", 900)])
    db.execute("INSERT INTO classspells VALUES (7, 901, 'test')")
    item = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, "
            "weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, "
            "itemdesc, model, color) VALUES (?, 0, 0, 7, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)")
    db.execute(item, (500, "Old Stone"))
    # an item of the second gem's cracked name already
    db.execute(item, (777, "Cracked Opal Stone"))
    db.executemany("INSERT INTO npcs (npc_name, world, modelid, npc_type) VALUES (?, ?, ?, ?)", [
        ("Old Warden", 1, 5, 128), ("glass warden", 1, 6, 0), ("glass keeper", 1, 6, 0), ("gate warden", 1, 6, 128),
        ("Sky Clerk", 4, 9, 0), ("Sky Guard", 4, 9, 0), ("essence of mist", 4, BODY, 0), ("essence of mist", 4, BODY, 128),
        ("elemental worker", 4, 8, 0), ("essence of ash", 0, BODY, 0)])
    db.execute("INSERT INTO npcaggro VALUES ('Sky Clerk', 0, 0, 'a seed', 'questnpcs:test')")
    db.execute("INSERT INTO loottables (tablename, patternid) VALUES ('other', 500)")
    qfile = os.path.join(dir, "test.json")
    with open(qfile, "w") as f:
        json.dump(QUESTS, f)
    # the quests imported once, the reward named a spell that was not here (a draft)
    db.execute("UPDATE spellpattern SET spellname = 'Spire Stepp' WHERE spellid = 901")
    iq.write(db, iq.plan(db, iq.load_files([qfile]), routes=None))
    db.execute("UPDATE spellpattern SET spellname = 'Spire Step' WHERE spellid = 901")
    # an item holding the first gem's cracked id
    db.execute(item, (fp.GEM_BASE + 2, "Someone Else's Ring"))
    # a quest step that kills one of the isle's talked-to names
    db.execute("INSERT INTO quests (questid, name, enabled, giver) VALUES (5, 'Guard Hunt', 1, 'Old Warden')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target) VALUES (5, 1, 'kill', 'Sky Guard')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target) VALUES (5, 2, 'talk', 'Sky Clerk')")
    # the debuffs as the import wrote them: Mire Cloud's effects and line both
    # its; Ash Veil's effects a GM's since; Bog Mark's line one the import kept;
    # Quick Rot's time there already
    for sid, name, stat, line, fx, logged in (
            (904, "Mire Cloud", "agi", "Target, Decrease AGI by 5", debuff("agi"), {"line": "Target, Decrease AGI by 5"}),
            (905, "Ash Veil", "ac", "Target, Decrease AC by 5", debuff("ac").replace("-5", "-9"), {"line": "Target, Decrease AC by 5"}),
            (906, "Bog Mark", "str", "Target, Decrease STR by 5 for 2 minutes", debuff("str"), {}),
            (907, "Quick Rot", "sta", "Target, Decrease STA by 5, 30sec", debuff("sta", 30), {})):
        db.execute("INSERT INTO spellpattern (spellid, spellname) VALUES (?, ?)", (sid, name))
        db.execute("INSERT INTO spelleffect (spellid, line, effects) VALUES (?, ?, ?)", (sid, line, fx))
        after = dict(logged, effects=debuff(stat, 30 if sid == 907 else None))
        db.execute("INSERT INTO eqoaemu_spell_import_log (tbl, key, action, after) VALUES ('spelleffect', ?, 'update', ?)",
                   (fp.im_dumps({"spellid": sid}), fp.im_dumps(after)))
    # the mirrors' original as the import wrote it; Glaring Mask as EQOA-Data's import
    # left it, Dull Mask its mirror already, Odd Mask a GM's effects of its own
    taunt = "Group, Enchant warriors weapon with taunt, 5min"
    for sid, name, lv, scope, frame, desc, line, fx in (
            (910, "Soothing Mask", 25, 1, 486870879, "Ours.", "Target, x", SOOTHING),
            (911, "Glaring Mask", 15, 2, 593297002, taunt, taunt, ""),
            (912, "Dull Mask", 20, 2, 593297002, taunt, taunt, ""),
            (913, "Odd Mask", 30, 2, 593297002, taunt, taunt, '{"v":1,"effects":[]}')):
        db.execute("INSERT INTO spellpattern (spellid, spellname, abilitylvl, scope, iconColor, spelldesc) VALUES (?, ?, ?, ?, ?, ?)",
                   (sid, name, lv, scope, frame, desc))
        db.execute("INSERT INTO spelleffect (spellid, line, source, effects) VALUES (?, ?, 'eqoa-data enchanter', ?)",
                   (sid, line, fx))
    db.execute("INSERT INTO eqoaemu_spell_import_log (tbl, key, action, after) VALUES ('spelleffect', ?, 'update', ?)",
               (fp.im_dumps({"spellid": 910}), fp.im_dumps({"line": "Target, x", "effects": SOOTHING})))
    rec = dict(zip(("spellid", "spellname", "abilitylvl", "scope", "iconColor", "spelldesc"),
                   db.execute("SELECT spellid, spellname, abilitylvl, scope, iconColor, spelldesc FROM spellpattern "
                              "WHERE spellid = 912").fetchone()))
    # the maker as the import wrote it, our item it makes, and the spells of ours (Odd Mixture a GM's effects)
    db.execute(item, (4780, "Rime Potion"))
    db.execute(item, (4781, "Chill Potion"))
    for sid, name, lv, scope, line, fx in ((920, "Rime Mixture", 46, 0, "Self, Summon Rime Potion x3", RIME),
                                          (921, "Chill Mixture", 37, 0, "SELF Summon Chill Potion", ""),
                                          (922, "Mending Tonic", 16, 0, "Dispel Effect x3", ""),
                                          (923, "Odd Mixture", 20, 0, "SELF Summon Chill Potion", '{"v":1,"effects":[]}')):
        db.execute("INSERT INTO spellpattern (spellid, spellname, abilitylvl, scope, spelldesc) VALUES (?, ?, ?, ?, ?)",
                   (sid, name, lv, scope, line))
        db.execute("INSERT INTO spelleffect (spellid, line, source, effects) VALUES (?, ?, 'eqoa-data alchemist', ?)",
                   (sid, line, fx))
    db.execute("INSERT INTO eqoaemu_spell_import_log (tbl, key, action, after) VALUES ('spelleffect', ?, 'update', ?)",
               (fp.im_dumps({"spellid": 920}), fp.im_dumps({"line": "Self, Summon Rime Potion x3", "effects": RIME})))
    # the swapped makers as the import wrote them, the meals (Hunted Nut's row a GM's), a grocer's with no row
    db.execute("CREATE TABLE foodeffect (patternid INTEGER PRIMARY KEY, duration_s INTEGER NOT NULL DEFAULT 0, "
               "ends_on_cast INTEGER NOT NULL DEFAULT 0, ends_on_contact INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm')")
    for pid, name in ((4790, "hunted grub"), (4791, "hunted fruit"), (4792, "hunted nut"), (4793, "grocer loaf")):
        db.execute(item, (pid, name))
    for sid, name, lv, fx in ((930, "Grub Hunt", 25, hunt(25, "hunted fruit", 4791, "Hunted Fruit")),
                              (931, "Fruit Hunt", 35, hunt(35, "hunted grub", 4790, "Hunted Grub"))):
        db.execute("INSERT INTO spellpattern (spellid, spellname, abilitylvl, scope) VALUES (?, ?, ?, 0)", (sid, name, lv))
        line = f"Self, Summon {json.loads(fx)['effects'][0]['item']['name']}"
        db.execute("INSERT INTO spelleffect (spellid, line, source, effects) VALUES (?, ?, 'eqoa-data ranger', ?)", (sid, line, fx))
        db.execute("INSERT INTO eqoaemu_spell_import_log (tbl, key, action, after) VALUES ('spelleffect', ?, 'update', ?)",
                   (fp.im_dumps({"spellid": sid}), fp.im_dumps({"line": line, "effects": fx})))
    for pid in (4790, 4791, 4792):
        row = {"patternid": pid, "duration_s": 120, "ends_on_cast": 1, "ends_on_contact": 0, "source": "eqoaemu"}
        db.execute("INSERT INTO foodeffect VALUES (?, 120, 1, 0, 'eqoaemu')", (pid,))
        db.execute("INSERT INTO eqoaemu_spell_import_log (tbl, key, action, after) VALUES ('foodeffect', ?, 'insert', ?)",
                   (fp.im_dumps({"patternid": pid}), fp.im_dumps(row)))
    db.execute("UPDATE foodeffect SET duration_s = 90 WHERE patternid = 4792")   # a GM's
    record, eff = fp.im.mirror_changes(rec, dict(line=taunt, effects="", source="eqoa-data enchanter"), 1, 486870879,
                                       "Soothing Mask", json.loads(SOOTHING), 5)
    db.execute(f"UPDATE spellpattern SET {', '.join(f'{k} = ?' for k in record['set'])} WHERE spellid = 912",
               list(record["set"].values()))
    db.execute(f"UPDATE spelleffect SET {', '.join(f'{k} = ?' for k in eff['set'])} WHERE spellid = 912",
               list(eff["set"].values()))
    return db, path, qfile


def dump(db):
    out = {}
    for name, sql in db.execute("SELECT name, sql FROM sqlite_master WHERE type = 'table' ORDER BY name").fetchall():
        out[name] = (sql, db.execute(f"SELECT rowid, * FROM {name} ORDER BY rowid").fetchall())
    return out


class FixPlaces(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db, self.path, self.qfile = make(self.dir.name)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def plan(self):
        return fp.plan(self.db, recasts=RECASTS, gems=GEMS, chambers=CHAMBERS, portal=self.qfile, keys=KEYS, talked_to=TALKED,
                       durations=DURATIONS, mirrors=MIRRORS, makes=MAKES, cures=CURES, made=MADE, meals=MEALS)

    def by(self, p, tbl, action):
        return [c for c in p["changes"] if c["tbl"] == tbl and c["action"] == action]

    def test_plan(self):
        p = self.plan()
        # recasts: from the workbook's 3; a GM's 45 left; one as it is to be
        self.assertEqual([(c["ref"], c["set"]) for c in self.by(p, "spellpattern", "update") if "recast" in c["set"]],
                         [({"spellid": 901}, {"recast": 3600})])
        self.assertIn("902 Moon Circle 45", p["said"]["a recast not the workbook's 3 (left: a GM's figure?)"])
        self.assertIn("903 Tide Gate 900", p["said"]["recasts as they are to be"])
        # the first gem left out whole: another item holds its cracked one's id; the
        # second's cracked one there already (used), the second made
        made = {c["row"]["itemname"]: c["row"] for c in self.by(p, "itempattern", "insert")}
        self.assertEqual(sorted(made), ["Opal Stone"])
        self.assertIn(f"{fp.GEM_BASE + 2} Someone Else's Ring for Cracked Amber Stone",
                      p["said"]["a gem's id taken by another item (the gem left out: its items, use, gate and drop)"])
        self.assertIn("777 Cracked Opal Stone", p["said"]["a gem's item there already (left as it is)"])
        opal = made["Opal Stone"]
        self.assertEqual((opal["patternid"], opal["trade"], opal["rent"], opal["lore"], opal["maxstack"], opal["procanim"]),
                         (fp.GEM_BASE + 3, 0, 0, 1, 1, 2), "a gem: its own id, trades, no rent, lore, single, used")
        uses = self.by(p, "itemeffect", "insert")
        self.assertEqual([(c["row"]["patternid"], c["row"]["line"]) for c in uses],
                         [(fp.GEM_BASE + 3, "Self, Reagent Cracked Amber Stone x1, Teleport to the Roof of the Glass Spire, "
                                            "Summon Cracked Opal Stone x1")])
        gates = self.by(p, "spellgates", "insert")
        self.assertEqual([(g["row"]["spell"], g["row"]["kind"], g["row"]["place"], g["row"]["world"], g["row"]["y"]) for g in gates],
                         [("opal stone", "self", "Roof of the Glass Spire", 1, 40.0)])
        self.assertEqual([(c["row"]["tablename"], c["row"]["patternid"]) for c in self.by(p, "loottables", "insert")],
                         [("spire keepers", fp.GEM_BASE + 3)])
        self.assertEqual(sorted((c["row"]["npc_name"], c["row"]["chance"]) for c in self.by(p, "npcloot", "insert")),
                         [("glass keeper", fp.GEM_CHANCE), ("lost one", fp.GEM_CHANCE)])
        self.assertIn("lost one (Opal Stone)", p["said"]["a gem's creature not in npcs (bound all the same)"])
        # the quests: the reward a record now, in the game; the kill of an NPC-bit creature, a draft
        lines = p["said"]["the quests (written again by import_quests.py's plan and write)"]
        self.assertIn("Test/spire/port: a draft -> in the game", lines)
        self.assertTrue(any(s.startswith("Test/spire/blocked: a draft -> a draft: every 'gate warden' carries the NPC bit")
                            for s in lines), lines)
        # the isle: the talked-to NPC and the world 4 essence in the portal body given the bit; the one a quest kills left
        bits = {c["ref"]["id"]: c["set"]["npc_type"] for c in self.by(p, "npcs", "update")}
        names = dict(self.db.execute("SELECT id, npc_name FROM npcs").fetchall())
        self.assertEqual(sorted(names[i] for i in bits), ["Sky Clerk", "essence of mist"])
        self.assertEqual(set(bits.values()), {128})
        self.assertIn("Sky Guard: quest 5 step 1 kill", p["said"]["an isle NPC a quest kills (left without the NPC bit)"])
        self.assertEqual([c["row"]["aggressive"] for c in self.by(p, "npcaggro", "insert")], [0])
        # the debuffs' times: the import's effects take duration_s after "harmful", the import's line its time
        times = {c["ref"]["spellid"]: c for c in self.by(p, "spelleffect", "update") if c["ref"]["spellid"] in range(904, 908)}
        self.assertEqual(sorted(times), [904, 906])
        self.assertEqual(times[904]["set"], {"effects": debuff("agi", 30), "line": "Target, Decrease AGI by 5, 30sec"})
        self.assertEqual(times[906]["set"], {"effects": debuff("str", 30)}, "a line the import did not write is kept")
        self.assertIn("906 Bog Mark: Target, Decrease STR by 5 for 2 minutes",
                      p["said"]["a duration's line not the import's, kept (a look: does it say a time?)"])
        self.assertTrue(p["said"]["a duration's effects not as the import wrote them (left: a GM's?)"][0].startswith("905 Ash Veil"))
        self.assertIn("907 Quick Rot 30 s", p["said"]["durations as they are to be"])
        # the mirrors: Glaring Mask a target spell with Soothing Mask's effects, its proc's hate raised by 15
        rec = next(c for c in self.by(p, "spellpattern", "update") if c["ref"] == {"spellid": 911})
        self.assertEqual(rec["set"], {"scope": 1, "iconColor": 486870879, "spelldesc": fp.im.MIRROR_DESC})
        fx = next(c for c in self.by(p, "spelleffect", "update") if c["ref"] == {"spellid": 911})
        j = json.loads(fx["set"]["effects"])
        self.assertEqual((j["level"], j["min_level"], j["effects"][0]["procs"][0]["amount"]), (15, 5, 15))
        self.assertNotIn("id", j["effects"][0], "the original's ids left out")
        self.assertEqual(fx["set"]["line"], "Target, Add Offensive Proc: Increase Hate by 15 when it hits, 5min")
        self.assertEqual(p["said"]["mirrors as they are to be"], ["912 Dull Mask"])
        self.assertEqual(p["said"]["a mirror's spell with effects of its own or no effects row (left: a GM's?)"],
                         ["913 Odd Mask"])
        self.assertNotIn({"spellid": 913}, [c["ref"] for c in p["changes"]])
        # the spells of ours given effects of ours: Chill Mixture makes one Chill Potion in Rime Mixture's frame,
        # Mending Tonic a self cure of up to three; Odd Mixture's own effects left
        fx = {c["ref"]["spellid"]: c["set"] for c in self.by(p, "spelleffect", "update") if c["ref"]["spellid"] in range(920, 924)}
        self.assertEqual(sorted(fx), [921, 922])
        chill = json.loads(fx[921]["effects"])
        self.assertEqual((chill["level"], chill["effects"]), (37, [{"type": "Create Item", "kind": "create_item",
                                                                     "item": {"count": 1, "name": "Chill Potion", "patternid": 4781}}]))
        self.assertEqual(fx[921]["line"], "Self, Summon Chill Potion")
        self.assertEqual(json.loads(fx[922]["effects"])["effects"],
                         [{"type": "Remove Ability", "kind": "cure", "removes": "", "cure_max": 3}])
        self.assertEqual(fx[922]["line"], "Self, Cure up to 3 harmful effects")
        self.assertEqual(p["said"]["a spell of ours with effects of its own or no effects row (left: a GM's?)"],
                         ["923 Odd Mixture"])
        # the errata: each hunt makes its own item, its line saying so; the meals end as a meal does, a GM's row left
        hunts = {c["ref"]["spellid"]: c["set"] for c in self.by(p, "spelleffect", "update") if c["ref"]["spellid"] in (930, 931)}
        self.assertEqual({k: (json.loads(v["effects"])["effects"][0]["item"]["patternid"], v["line"]) for k, v in hunts.items()},
                         {930: (4790, "Self, Summon hunted grub"), 931: (4791, "Self, Summon hunted fruit")})
        self.assertEqual(json.loads(hunts[930]["effects"]), json.loads(hunt(25, "hunted grub", 4790, "Hunted Grub")),
                         "the item part the import wrote for its item, on the other spell")
        meals = {c["ref"]["patternid"]: c["set"] for c in self.by(p, "foodeffect", "update")}
        self.assertEqual(meals, {4790: {"ends_on_cast": 0, "ends_on_contact": 1}, 4791: {"ends_on_cast": 0, "ends_on_contact": 1}})
        self.assertEqual(p["said"]["a meal's foodeffect row not as the import wrote it (left: a GM's?)"], ["4792 hunted nut"])
        self.assertNotIn(4793, meals, "a grocer's meal with no row: the minute")

    def test_apply_again_and_undo(self):
        before = dump(self.db)
        p = self.plan()
        wrote, stale, q = fp.apply(self.db, p, when="2026-11-01T20:00:00+00:00")
        self.assertEqual(stale, 0)
        self.assertEqual(wrote, len(p["changes"]) + 2, "every change and the two quests")
        self.assertEqual(self.db.execute("SELECT recast FROM spellpattern WHERE spellid = 901").fetchone()[0], 3600)
        qid = dict(self.db.execute("SELECT key, questid FROM quest_import_quests").fetchall())
        self.assertEqual(self.db.execute("SELECT enabled FROM quests WHERE questid = ?", (qid["Test/spire/port"],)).fetchone()[0], 1)
        self.assertEqual(self.db.execute("SELECT enabled FROM quests WHERE questid = ?", (qid["Test/spire/blocked"],)).fetchone()[0], 0)
        self.assertEqual(self.db.execute("SELECT spells FROM queststeps WHERE questid = ? AND step = 2",
                                         (qid["Test/spire/port"],)).fetchone()[0], "901")
        self.assertEqual(self.db.execute("SELECT line, effects FROM spelleffect WHERE spellid = 904").fetchone(),
                         ("Target, Decrease AGI by 5, 30sec", debuff("agi", 30)))
        once = dump(self.db)
        # a second run writes nothing and leaves every table as it was
        wrote, stale, _ = fp.apply(self.db, self.plan(), when="2026-11-01T20:00:05+00:00")
        self.assertEqual((wrote, stale), (0, 0))
        self.assertEqual(dump(self.db), once)
        # nor in the same second as the last
        with self.assertRaises(SystemExit):
            fp.apply(self.db, self.plan(), when="2026-11-01T20:00:00+00:00")
        # --undo takes it all back, exact
        back, left, run = fp.undo(self.db)
        self.assertEqual((left, run), ([], "2026-11-01T20:00:00+00:00"))
        self.assertEqual(dump(self.db), before)

    def test_undo_keeps_a_gms_change(self):
        before = dump(self.db)
        fp.apply(self.db, self.plan(), when="2026-11-01T20:00:00+00:00")
        self.db.execute("UPDATE npcloot SET chance = 50 WHERE npc_name = 'glass keeper'")
        self.db.execute("UPDATE npcs SET npc_type = 384 WHERE npc_name = 'Sky Clerk'")
        back, left, _ = fp.undo(self.db)
        self.assertEqual(sorted(left), ['npcloot {"npc_name":"glass keeper","tablename":"spire keepers"}', 'npcs {"id":5}'])
        self.assertEqual(self.db.execute("SELECT chance FROM npcloot WHERE npc_name = 'glass keeper'").fetchone()[0], 50)
        self.assertFalse(fp.has_table(self.db, fp.LOG), "the log goes with its last run")
        # the rest as it was
        self.db.execute("DELETE FROM npcloot WHERE npc_name = 'glass keeper'")
        self.db.execute("UPDATE npcs SET npc_type = 0 WHERE npc_name = 'Sky Clerk'")
        self.assertEqual(dump(self.db), before)

    def test_the_gems_lines_read_as_the_reader_reads_them(self):
        # the shape the server's potion reader takes (entity/potion.go,
        # spelltext.go): no comma in a place, the reagent's and the
        # summoning's words, the place after "Teleport to the"
        for g in fp.GEMS:
            line = fp.gem_line(g)
            place = fp.CHAMBERS[g["to"]][0]
            self.assertNotIn(",", place)
            self.assertNotIn("pet", line.lower(), "a line with the word reads as a pet's summoning")
            self.assertIn(f"Teleport to the {place}, Summon {g['cracked']} x1", line)
            if g["needs"]:
                self.assertTrue(line.startswith(f"Self, Reagent {g['needs']} x1, "))
        self.assertEqual([g["needs"] for g in fp.GEMS[1:]], [g["cracked"] for g in fp.GEMS[:-1]],
                         "each gem needs the cracked gem the one before leaves")


if __name__ == "__main__":
    unittest.main()
