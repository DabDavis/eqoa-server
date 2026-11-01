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
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, spellname TEXT NOT NULL, recast INTEGER NOT NULL DEFAULT 3);
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
"""

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
        return fp.plan(self.db, recasts=RECASTS, gems=GEMS, chambers=CHAMBERS, portal=self.qfile, keys=KEYS, talked_to=TALKED)

    def by(self, p, tbl, action):
        return [c for c in p["changes"] if c["tbl"] == tbl and c["action"] == action]

    def test_plan(self):
        p = self.plan()
        # recasts: from the workbook's 3; a GM's 45 left; one as it is to be
        self.assertEqual([(c["ref"], c["set"]) for c in self.by(p, "spellpattern", "update")],
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
