#!/usr/bin/env python3
"""tools/ascend_boss_loot.py on a small database of the server's own tables: a boss whose own table holds gear, a
spell scroll, a gem and an Heirloom; a table it shares with a creature that is not tuned; its own category pool row
and a spawn's item; a quest that takes its ring and rewards a breastplate."""
import contextlib
import io
import json
import os
import re
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ascend_boss_loot as abl  # noqa: E402

# the server's tables (internal/database: sqlite.go, itemeffect.go, loottables.go, lootcategory.go, quests.go)
SCHEMA = """
CREATE TABLE itempattern (
    patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL, unk1 INTEGER NOT NULL, itemicon INTEGER NOT NULL,
    unk2 INTEGER NOT NULL, equipslot INTEGER NOT NULL, unk3 INTEGER NOT NULL, trade INTEGER NOT NULL, rent INTEGER NOT NULL,
    unk4 INTEGER NOT NULL, attacktype INTEGER NOT NULL, weapondamage INTEGER NOT NULL, unk5 INTEGER NOT NULL,
    levelreq INTEGER NOT NULL, maxstack INTEGER NOT NULL, maxhp INTEGER NOT NULL, duration INTEGER NOT NULL,
    classuse INTEGER NOT NULL, raceuse INTEGER NOT NULL, procanim INTEGER NOT NULL, lore INTEGER NOT NULL,
    unk6 INTEGER NOT NULL, craft INTEGER NOT NULL, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL, model INTEGER NOT NULL,
    color INTEGER NOT NULL, str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER,
    intelligence INTEGER, HPMAX INTEGER, POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER, DR INTEGER,
    FR INTEGER, CR INTEGER, LR INTEGER, AR INTEGER, weaponproc INTEGER, fish INTEGER);
CREATE TABLE itemeffect (
    patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '', icon INTEGER NOT NULL DEFAULT 0,
    breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0, area REAL NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'gm', body INTEGER NOT NULL DEFAULT 0, target_line TEXT NOT NULL DEFAULT '',
    charges INTEGER NOT NULL DEFAULT 0, cooldown REAL NOT NULL DEFAULT 0);
CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '');
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE lootpreset (name TEXT PRIMARY KEY, active INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE lootrate (preset TEXT NOT NULL, profile TEXT NOT NULL, category TEXT NOT NULL, rate REAL NOT NULL DEFAULT 0,
    counts TEXT NOT NULL DEFAULT '', minstack INTEGER NOT NULL DEFAULT 1, maxstack INTEGER NOT NULL DEFAULT 1,
    position INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (preset, profile, category));
CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT NOT NULL, region TEXT NOT NULL DEFAULT '*',
    npc_name TEXT NOT NULL DEFAULT '', patternid INTEGER NOT NULL, weight INTEGER NOT NULL DEFAULT 1,
    minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 100, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT NOT NULL DEFAULT '', profile TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, npc_level INTEGER,
    npc_type INTEGER, npc_id INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER NOT NULL DEFAULT 0,
    remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
    remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL,
    location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
    giver TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', items TEXT NOT NULL DEFAULT '', rewarditems TEXT NOT NULL DEFAULT '',
    byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
"""

FANG, RING, SCROLL, GEM, HEIRLOOM, SHIELD, PLATE, TALISMAN = 101, 102, 103, 104, 105, 106, 107, 108


def item(db, pid, name, slot, cost=10, rent=1, icon=5, stack=1, attack=0, dmg=0, maxhp=0, lore=0, trade=0, **stats):
    cols = dict(patternid=pid, itemcost=cost, unk1=0, itemicon=icon, unk2=0, equipslot=slot, unk3=0, trade=trade, rent=rent,
                unk4=0, attacktype=attack, weapondamage=dmg, unk5=0, levelreq=55, maxstack=stack, maxhp=maxhp, duration=0,
                classuse=32767, raceuse=1023, procanim=0, lore=lore, unk6=0, craft=0, itemname=name, itemdesc="A thing.",
                model=402229003, color=3)
    cols.update(stats)
    db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", list(cols.values()))


def fixture(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    item(db, FANG, "Fang of the Test", 12, cost=4201, rent=0, attack=1, dmg=41, maxhp=200, lore=1, trade=1,
         str=15, AC=4, HPMAX=5, FR=-10, PoT=3)
    item(db, RING, "Ring of the Test", 8, cost=50, wis=14, HoT=5, sta=None)
    item(db, SCROLL, "Big Nuke", -1, icon=abl.SCROLL_ART)
    item(db, GEM, "gleaming gem", -1, stack=20)
    item(db, HEIRLOOM, "Heirloom Blade", 12, attack=1, dmg=60)
    item(db, SHIELD, "Shared Shield", 13, AC=40, maxhp=300)
    item(db, PLATE, "Breastplate of the Test", 5, AC=100, sta=20)
    item(db, TALISMAN, "Talisman of the Test", -1)
    db.execute("INSERT INTO itemeffect (patternid, line, target_line, cooldown) VALUES (?, 'Self, Increase AR by 75, 30min', '', 60)",
               (RING,))
    db.executemany("INSERT INTO npcs (id, npc_name, world, npc_level, npc_type, npc_id) VALUES (?, ?, 3, ?, 0, ?)",
                   [(1, "Big Boss", 65, 9001), (2, "small orc", 10, 0), (3, "Other Boss", 60, 0)])
    db.executemany("INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES (?, ?, ?, ?)",
                   [("big boss", "boss big boss", 100, 2), ("big boss", "shared", 25, 1), ("small orc", "shared", 25, 1),
                    ("*", "mods", 3, 1)])
    db.executemany("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, ?, 1, 70, 'gm')",
                   [("boss big boss", FANG, 2), ("boss big boss", RING, 1), ("boss big boss", SCROLL, 1),
                    ("boss big boss", GEM, 1), ("boss big boss", HEIRLOOM, 1), ("shared", SHIELD, 1), ("shared", GEM, 3),
                    ("mods", GEM, 1)])
    db.execute("INSERT INTO lootpreset (name, active) VALUES ('jib', 1)")
    db.execute("INSERT INTO lootrate (preset, profile, category, rate) VALUES ('jib', 'boss', 'rare', 0.0125)")
    db.execute("INSERT INTO lootcreature (npc_name, region, profile) VALUES ('big boss', 'lavastorm', 'boss')")
    db.executemany("INSERT INTO lootpool (category, region, npc_name, patternid) VALUES (?, ?, ?, ?)",
                   [("rare", "*", "big boss", SHIELD), ("rare", "lavastorm", "", PLATE)])
    db.execute("INSERT INTO npcitem (npcid, stackleft, patternid) VALUES (9001, 1, ?)", (RING,))
    db.execute("INSERT INTO quests (questid, name, enabled, giver) VALUES (500, 'Armour of the Test', 1, 'Smith')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, items, byclass) VALUES (500, 1, 'give', 'Smith', ?, ?)",
               (f"{TALISMAN}, {RING}", json.dumps({"0": {"items": str(PLATE)}, "2": {"items": str(SCROLL)}})))
    db.commit()
    db.close()


def dump(path, skip=("gm_audit", abl.LOG)):
    """Every table's rows (and the counters), for an exact comparison: gm_audit, a history an undo adds to, aside."""
    db = sqlite3.connect(path)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
        if t in skip:
            continue
        rows = db.execute(f'SELECT * FROM "{t}" ORDER BY 1, 2').fetchall()
        out[t] = [r for r in rows if t != "sqlite_sequence" or r[0] not in skip]
    db.close()
    return out


class AscendBossLootTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        fixture(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = abl.main(["--db", self.path, "--no-backup", *args])
        return code, out.getvalue()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def copy_of(self, base):
        if not self.q("SELECT name FROM sqlite_master WHERE name = 'itemascended'"):
            return None
        r = self.q("SELECT patternid FROM itemascended WHERE base = ?", (base,))
        return r[0][0] if r else None

    def test_the_rule(self):
        self.assertEqual([abl.figure(v) for v in (0, 1, 4, 5, 10, 14, 15, 41, 100, -10)], [0, 1, 4, 6, 11, 15, 17, 45, 110, -10])
        self.assertEqual(abl.scaled_line("Self, Increase AR by 75, Decrease Hitpoints By 15, 30min"),
                         "Self, Increase AR by 83, Decrease Hitpoints By 17, 30min")

    def test_report_only_writes_nothing(self):
        before = dump(self.path)
        code, out = self.run_tool("--boss", "Big Boss")
        self.assertEqual(code, 0, out)
        self.assertIn("Ascended Fang of the Test (of 101", out)
        self.assertIn("report only", out)
        self.assertEqual(dump(self.path), before)

    def test_apply_makes_the_records_and_points_the_boss_at_them(self):
        code, out = self.run_tool("--boss", "big boss", "--apply")
        self.assertEqual(code, 0, out)
        fang, ring, shield, plate = (self.copy_of(x) for x in (FANG, RING, SHIELD, PLATE))
        self.assertEqual(sorted([fang, ring, shield, plate]), list(range(abl.FIRST_ID, abl.FIRST_ID + 4)))
        # the figures a tenth more, the rest the base's; never NO RENT
        r = self.q("SELECT itemname, weapondamage, str, sta, AC, HPMAX, FR, PoT, maxhp, itemcost, rent, lore, trade, model, "
                   "equipslot, itemdesc FROM itempattern WHERE patternid = ?", (fang,))[0]
        self.assertEqual(r, ("Ascended Fang of the Test", 45, 17, None, 4, 6, -10, 3, 200, 4201, 1, 1, 1, 402229003, 12, "A thing."))
        self.assertEqual(self.q("SELECT wis, HoT, rent FROM itempattern WHERE patternid = ?", (ring,))[0], (15, 6, 1))
        self.assertEqual(self.q("SELECT line, cooldown FROM itemeffect WHERE patternid = ?", (ring,))[0],
                         ("Self, Increase AR by 83, 30min", 60.0))
        # the boss's own table: gear Ascended, the spell, the gem and the Heirloom as they were
        self.assertEqual(self.q("SELECT patternid, weight FROM loottables WHERE tablename = 'boss big boss' ORDER BY id"),
                         [(fang, 2), (ring, 1), (SCROLL, 1), (GEM, 1), (HEIRLOOM, 1)])
        # the shared table as it was, for the orc; the boss rolls its copy
        self.assertEqual(self.q("SELECT patternid FROM loottables WHERE tablename = 'shared' ORDER BY id"), [(SHIELD,), (GEM,)])
        self.assertEqual(self.q("SELECT patternid, weight FROM loottables WHERE tablename = 'shared ascended' ORDER BY id"),
                         [(shield, 1), (GEM, 3)])
        self.assertEqual(self.q("SELECT npc_name, tablename, chance, rolls FROM npcloot ORDER BY npc_name, tablename"),
                         [("*", "mods", 3, 1), ("big boss", "boss big boss", 100, 2), ("big boss", "shared ascended", 25, 1),
                          ("small orc", "shared", 25, 1)])
        # its own pool row and its spawn's item; the region's pool as it was
        self.assertEqual(self.q("SELECT npc_name, patternid FROM lootpool ORDER BY id"), [("big boss", shield), ("", PLATE)])
        self.assertEqual(self.q("SELECT patternid FROM npcitem"), [(ring,)])
        # the quest that takes the ring: its reward's record made, the quest itself as it was
        self.assertIn('quest 500 "Armour of the Test" step 1 (give Smith): Ring of the Test', out)
        self.assertEqual(self.q("SELECT itemname, AC, sta FROM itempattern WHERE patternid = ?", (plate,))[0],
                         ("Ascended Breastplate of the Test", 110, 22))
        self.assertIsNone(self.copy_of(SCROLL))
        self.assertIsNone(self.copy_of(GEM))
        self.assertIsNone(self.copy_of(HEIRLOOM))
        # every row written is in gm_audit, as the panel lists it
        logged = self.q(f"SELECT count(*) FROM {abl.LOG} WHERE tbl NOT IN ('sqlite_sequence', 'sqlite_master')")[0][0]
        audited = self.q("SELECT count(*) FROM gm_audit WHERE via = 'tool' AND detail LIKE 'petition 1446%'")[0][0]
        self.assertEqual(logged, audited)
        self.assertGreater(audited, 10)
        self.assertEqual(self.q("SELECT before, after FROM gm_audit WHERE tbl = 'npcloot'")[0][1],
                         abl.go_json({"npc_name": "big boss", "tablename": "shared ascended", "chance": 25, "rolls": 1,
                                      "note": "", "source": "gm"}))

    def test_a_second_run_changes_nothing(self):
        self.run_tool("--boss", "Big Boss", "--apply")
        before, audits = dump(self.path), self.q("SELECT count(*) FROM gm_audit")
        code, out = self.run_tool("--boss", "Big Boss", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)
        self.assertIn('quest 500 "Armour of the Test"', out, "the quests are listed again")
        self.assertEqual(dump(self.path), before)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), audits)

    def test_undo_puts_everything_back(self):
        before = dump(self.path)
        self.run_tool("--boss", "Big Boss", "--apply")
        self.assertNotEqual(dump(self.path), before)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.path, skip=("gm_audit",)), before, out)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (abl.LOG,))[0][0], 0)
        undone = self.q("SELECT count(*) FROM gm_audit WHERE detail LIKE 'undo: petition 1446%'")[0][0]
        self.assertGreater(undone, 10)

    def test_undo_keeps_a_record_a_character_holds(self):
        self.run_tool("--boss", "Big Boss", "--apply")
        fang = self.copy_of(FANG)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (7, 1, 200, 0, ?, -1, 0, 0)", (fang,))
        db.commit()
        db.close()
        code, out = self.run_tool("--undo")
        self.assertIn(f"Ascended record {fang} stays: charinventory 1", out)
        self.assertEqual(self.copy_of(FANG), fang, "the record held stays Ascended")
        self.assertIsNone(self.copy_of(RING))
        self.assertEqual(self.q("SELECT patternid FROM loottables WHERE tablename = 'boss big boss' ORDER BY id")[0], (FANG,))
        # once nothing holds it, a later --undo takes it too
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory")
        db.commit()
        db.close()
        self.run_tool("--undo")
        self.assertIsNone(self.copy_of(FANG))
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE patternid >= ?", (abl.FIRST_ID,))[0][0], 0)

    def test_a_later_boss_sharing_the_table_takes_the_same_copy(self):
        self.run_tool("--boss", "Big Boss", "--apply")
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES ('other boss', 'shared', 50, 1)")
        db.commit()
        db.close()
        rows = self.q("SELECT count(*) FROM loottables")[0][0]
        code, out = self.run_tool("--boss", "Other Boss", "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT count(*) FROM loottables")[0][0], rows, "no second copy")
        self.assertEqual(self.q("SELECT tablename, chance FROM npcloot WHERE npc_name = 'other boss'"), [("shared ascended", 50)])
        self.assertEqual(self.q("SELECT tablename FROM npcloot WHERE npc_name = 'small orc'"), [("shared",)])

    def test_undo_of_one_run_leaves_the_others(self):
        self.run_tool("--boss", "Big Boss", "--apply")
        after_first = dump(self.path, skip=("gm_audit",))
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES ('other boss', 'shared', 50, 1)")
        db.commit()
        db.close()
        before_second = dump(self.path)
        code, out = self.run_tool("--boss", "Other Boss", "--apply")
        run = re.search(r"^run (\S+): wrote", out, re.M).group(1)
        self.assertEqual(self.q("SELECT tablename FROM npcloot WHERE npc_name = 'other boss'"), [("shared ascended",)])
        code, out = self.run_tool("--undo", "--run", run)
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.path), before_second, "the second run alone taken back")
        self.assertEqual(self.q("SELECT tablename FROM npcloot WHERE npc_name = 'big boss' ORDER BY tablename"),
                         [("boss big boss",), ("shared ascended",)])
        self.assertNotEqual(after_first, dump(self.path, skip=("gm_audit",)))  # the other boss's binding is the GM's row

    def test_a_table_only_tuned_bosses_roll_changes_in_place(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM npcloot WHERE npc_name = 'small orc'")
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES ('other boss', 'shared', 50, 1)")
        db.commit()
        db.close()
        code, out = self.run_tool("--boss", "Big Boss", "--boss", "Other Boss", "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT patternid FROM loottables WHERE tablename = 'shared' ORDER BY id"),
                         [(self.copy_of(SHIELD),), (GEM,)])
        self.assertEqual(self.q("SELECT count(*) FROM loottables WHERE tablename = 'shared ascended'")[0][0], 0)

    def test_a_quest_item_it_drops_that_a_quest_turns_into_gear_is_named(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO quests (questid, name, enabled, giver) VALUES (501, 'The gem', 1, 'Smith')")
        db.execute("INSERT INTO queststeps (questid, step, kind, target, items, rewarditems) VALUES (501, 1, 'give', 'Smith', ?, ?)",
                   (str(GEM), str(PLATE)))
        db.commit()
        db.close()
        code, out = self.run_tool("--boss", "Big Boss")
        self.assertIn('Not changed: gleaming gem (Big Boss), no gear, drops as it is: quest 501 "The gem" step 1 turns it into '
                      'Breastplate of the Test, given regular', out)

    def test_items_another_creature_shares_stay(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET npc_id = 9001 WHERE npc_name = 'small orc'")  # the orc's corpse holds the same rows
        db.commit()
        db.close()
        code, out = self.run_tool("--boss", "Big Boss", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("WARNING: Big Boss's spawns' items (npcitem, npc_id 9001) are those of small orc too", out)
        self.assertEqual(self.q("SELECT patternid FROM npcitem"), [(RING,)])

    def test_an_unknown_boss_is_refused(self):
        before = dump(self.path)
        code, out = self.run_tool("--boss", "Big Bos", "--apply")
        self.assertEqual(code, 2)
        self.assertIn("PROBLEM: 'Big Bos': no NPC and no loot table of that name here (names like it: Big Boss", out)
        self.assertIn("nothing written", out)
        self.assertEqual(dump(self.path), before)

    def test_a_name_the_client_cannot_show_is_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET itemname = ? WHERE patternid = ?", ("x" * 55, FANG))
        db.commit()
        db.close()
        code, out = self.run_tool("--boss", "Big Boss", "--apply")
        self.assertEqual(code, 2)
        self.assertIn("the client shows 63", out)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'itemascended'")[0][0], 0)

    def test_the_tuned_list(self):
        self.assertIn("Yhafiran", abl.TUNED)
        code, out = self.run_tool("--tuned")
        self.assertEqual(code, 2, "none of them stands in this database")
        self.assertIn("'Lord Nagafen': no NPC", out)


if __name__ == "__main__":
    unittest.main()
