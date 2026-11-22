#!/usr/bin/env python3
"""sarek_709_20261122.py on a small database: apply, a second apply that writes nothing, undo exact; a row not as the
entry read it refuses the whole run; an Ascended record a character holds is kept by --undo."""
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import sarek_709_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, npc_level INTEGER, npc_type INTEGER, modelid INTEGER, size REAL, npc_id INTEGER);
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '',
    special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0,
    abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    special_every REAL NOT NULL DEFAULT 0, special_first REAL NOT NULL DEFAULT 0, special_jitter REAL NOT NULL DEFAULT 0,
    special_from INTEGER NOT NULL DEFAULT 0, resist_kind TEXT NOT NULL DEFAULT '', resist_pct INTEGER NOT NULL DEFAULT 0);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', giver TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', items TEXT NOT NULL DEFAULT '', journal TEXT NOT NULL DEFAULT '',
    events TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    scope TEXT NOT NULL DEFAULT 'near', radius REAL NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, steps TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '');
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE npccoins (npc_name TEXT PRIMARY KEY, coins_min INTEGER NOT NULL, coins_max INTEGER NOT NULL, note TEXT,
    source TEXT);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemcost INTEGER, itemicon INTEGER, equipslot INTEGER,
    trade INTEGER, rent INTEGER, attacktype INTEGER, weapondamage INTEGER, levelreq INTEGER, maxstack INTEGER,
    maxhp INTEGER, classuse INTEGER, procanim INTEGER, lore INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER,
    str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER, intelligence INTEGER, HPMAX INTEGER,
    POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER, DR INTEGER, FR INTEGER, CR INTEGER, LR INTEGER,
    AR INTEGER, weaponproc INTEGER);
CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '');
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
    icon INTEGER NOT NULL DEFAULT 0, breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0,
    area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm', body INTEGER NOT NULL DEFAULT 0,
    target_line TEXT NOT NULL DEFAULT '', charges INTEGER NOT NULL DEFAULT 0, cooldown REAL NOT NULL DEFAULT 0);
CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL);
CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER);
"""

ROAMER_NOTE = "'roamer' as it was, strong in PR,DR,AR at 50"
ZONE = json.dumps(tool.OLD_ZONE, separators=(",", ":"))


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    npcs = [(120968, "Anvil of Sarek", 1, 4738.44, 132.031, 4651.91, 255, 65, 128, 2882293234, 1.4, 120968),
            (120969, "Archon Seradesh", 1, 4744.93, 144.719, 4811.66, 0, 63, 128, 515091180, 1.0, 120969),
            (120970, "Sarek the Betrayer", 1, 4306.85, 219.25, 2570.09, 186, 67, 0, 2577583855, 1.0, 120970),
            (120972, "Archon Aereth", 1, 4699.2, 110.375, 2512.7, 0, 67, 0, 1, 1.0, 120972),
            (120975, "Archon Seradesh", 1, 4282.56, 219.25, 2586.42, 0, 63, 128, 515091180, 1.0, 120975),
            (122783, "Arios the Battered", 0, 12681.1, 91.4, 18740.7, 0, 50, 128, 1893243078, 1.0, 122783),
            (122825, "terran caretaker", 1, 4793.76, 133.96, 4675.14, 74, 60, 0, 931615265, 1.2, 0),
            (122832, "Arios the Battered", 5, 4944.3, 147.7, 7057.4, 0, 60, 0, 1893243078, 1.0, 122832)]
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", npcs)
    db.executemany("INSERT INTO npctemplate (name, template) VALUES (?, ?)",
                   [("sarek the betrayer", "sarek the betrayer"), ("anvil of sarek", "anvil of sarek")])
    db.execute("INSERT INTO classtemplates (name, class, special, special_damage, special_every, special_first, "
               "special_jitter, special_from, resist_kind, resist_pct, note) VALUES ('sarek the betrayer', 0, "
               "'Critical Hit', 7.4, 30, 15, 5, 10, 'PR,DR,AR', 50, ?)", (ROAMER_NOTE,))
    db.execute("INSERT INTO classtemplates (name, abilities, note) VALUES ('anvil of sarek', ?, 'petition 1956')", (ZONE,))
    db.execute("INSERT INTO quests VALUES (101029, 'Sarek the Betrayer', 'Arios the Battered')")
    steps = [(3, "talk", "Anvil of Sarek", "", "", ""),
             (10, "show", "Anvil of Sarek", "701046", "", json.dumps(tool.OLD10, separators=(",", ":"))),
             (18, "give", "Arios the Battered", "701101", "", ""),
             (19, "kill", "Sarek the Betrayer", "", tool.J19_OLD, json.dumps(tool.OLD19, separators=(",", ":"))),
             (20, "talk", "Archon Seradesh", "", tool.J20_OLD, ""),
             (26, "kill", "Arios the Battered", "", "", "")]
    db.executemany("INSERT INTO queststeps VALUES (101029, ?, ?, ?, ?, ?, ?)", steps)
    db.execute("INSERT INTO gmtriggers (slot, name) VALUES (300, 'another')")
    db.executemany("INSERT INTO balancelevel VALUES (?, ?)", [(60, 400.0), (70, 460.0)])
    gear = [(pid, name, 18 if pid in (5248, 5250) else 2) for pid, name in tool.CLASS_ITEMS + tool.CORPSE_ITEMS[:3]]
    for pid, name, slot in gear:
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack, attacktype, rent, str, sta, HPMAX, "
                   "trade, lore, itemcost, levelreq, classuse) VALUES (?, ?, ?, 1, 0, 1, 10, 15, 125, 0, 0, 3866, 60, 1)",
                   (pid, name, slot))
    db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack, itemicon) VALUES (716230, "
               "'Focus & Desire', -1, 1, -607649328)")
    db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack, attacktype, rent, weapondamage, model) "
               "VALUES (5451, 'Trident of the Merkings', 15, 1, 6, 1, 704, 1200321600)")
    db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack, rent) VALUES "
               "(718019, 'Superior Mana Stone', -1, 1, 0)")
    db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack) VALUES (1203, 'Miraculous Draught', -1, 9999)")
    db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack) VALUES (747437, 'a token', -1, 1)")
    db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack) VALUES (981031, 'Ascended earthen garb', 5, 1)")
    db.execute("INSERT INTO itemascended VALUES (981031, 6518, '', 'earlier')")
    db.execute("INSERT INTO charquests VALUES (1, 101029, 10, 0)")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        code = tool.main(list(args))
    return code, out.getvalue()


def snapshot(path):
    db = sqlite3.connect(path)
    tables = [r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT IN "
                                       "('gm_audit', 'sqlite_sequence') ORDER BY name")]
    out = {t: sorted(map(repr, db.execute(f'SELECT * FROM "{t}"').fetchall())) for t in tables}
    db.close()
    return out


class SarekTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        before = snapshot(self.path)
        code, out = run("--db", self.path, "--shard", "heirloom")
        self.assertEqual(code, 0, out)
        self.assertEqual(snapshot(self.path), before, "a dry run wrote")
        code, out = run("--db", self.path, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 0, out)
        # the random Sarek: big, no crit, the Blight
        hp, every, chance, jitter, first, ab = self.q("SELECT hp_pct, special_every, special_chance, special_jitter, "
                                                      "special_first, abilities FROM classtemplates WHERE name = "
                                                      "'sarek the betrayer'")[0]
        self.assertEqual((hp, every, chance, jitter, first), (4221, 0, 0, 0, 0))
        self.assertEqual(json.loads(ab), [tool.BLIGHT])
        # the quest Sarek: a new row in Aereth's tower, spawn-only by row, its own template, no Blight
        (qid, x, y, z, npc_id), = self.q("SELECT id, x_coord, y_coord, z_coord, npc_id FROM npcs WHERE npc_name = "
                                         "'Sarek the Betrayer' AND id != 120970")
        self.assertEqual((x, y, z, npc_id), (4700.0, 110.375, 2500.0, qid))
        self.assertTrue(self.q("SELECT 1 FROM npcspawnonly WHERE name = ?", f"#{qid}"))
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = ?", f"#{qid}"), [(tool.TPL_QUEST,)])
        self.assertEqual(self.q("SELECT hp_pct, abilities, special_every, resist_kind FROM classtemplates WHERE name = ?",
                                tool.TPL_QUEST), [(1407, "[]", 0, "PR,DR,AR")])
        # the trigger, fired by step 18; step 19's teleport into its room and Seradesh at its done
        (slot, steps), = self.q("SELECT slot, steps FROM gmtriggers WHERE name = 'Sarek the Betrayer'")
        self.assertEqual(slot, 301)
        spawns = [s for s in json.loads(steps) if s["do"] == "spawn"]
        self.assertEqual([s["npc"] for s in spawns], [f"#{qid}"])
        ev = {n: json.loads(e) for n, e in self.q("SELECT step, events FROM queststeps WHERE events != ''")}
        self.assertEqual(ev[18], [{"on": "done", "do": "trigger", "trigger": "Sarek the Betrayer", "text": tool.BUSY}])
        self.assertEqual((ev[19][0]["x"], ev[19][0]["z"]), (4700.0, 2490.0))
        self.assertEqual(ev[19][1]["npc"], "#120975")
        self.assertTrue(self.q("SELECT 1 FROM npcspawnonly WHERE name = '#120975'"))
        self.assertFalse(self.q("SELECT 1 FROM npcspawnonly WHERE name = '#120969'"), "the Anvil's Seradesh stays")
        # the Anvil: the bolt sparing the quester; its fighting row and the Minions at steps 3 and 10
        self.assertEqual(json.loads(self.q("SELECT abilities FROM classtemplates WHERE name = 'anvil of sarek'")[0][0]),
                         tool.NEW_ZONE)
        (frow,), = self.q("SELECT id FROM npcs WHERE npc_name = 'Anvil of Sarek' AND id != 120968")[0:1]
        (mrow, mlevel, mmodel), = self.q("SELECT id, npc_level, modelid FROM npcs WHERE npc_name = 'Minion of Sarek'")
        self.assertEqual((mlevel, mmodel), (63, 931615265))
        for n in (3, 10):
            npcs = [e.get("npc") for e in ev[n] if e["do"] in ("spawn", "hide")]
            self.assertEqual(npcs, ["Anvil of Sarek", f"#{frow}", f"#{mrow}"], n)
        self.assertEqual(ev[10][0]["do"], "popup", "step 10's popup kept first")
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = ?", f"#{frow}"), [(tool.TPL_ANVIL_FIGHT,)])
        self.assertEqual(self.q("SELECT hp_pct, class FROM classtemplates WHERE name = ?", tool.TPL_ANVIL_FIGHT), [(805, -1)])
        self.assertEqual(self.q("SELECT hp_pct FROM classtemplates WHERE name = ?", tool.TPL_MINION), [(190,)])
        # Arios's last fight, by his row alone
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = '#122832'"), [(tool.TPL_ARIOS,)])
        self.assertFalse(self.q("SELECT 1 FROM npctemplate WHERE name IN ('#122783', 'arios the battered')"))
        self.assertEqual(self.q("SELECT hp_pct FROM classtemplates WHERE name = ?", tool.TPL_ARIOS), [(3472,)])
        # the loot: 15 class rows (14 Ascended + the stone), 4 corpse rows (3 Ascended + the scroll)
        cls = [p for p, in self.q("SELECT patternid FROM loottables WHERE tablename = 'boss sarek class'")]
        corpse = [p for p, in self.q("SELECT patternid FROM loottables WHERE tablename = 'boss sarek corpse'")]
        self.assertEqual(len(cls), 15)
        self.assertIn(981031, corpse, "the earthen garb's Ascended record there is used")
        self.assertIn(716230, corpse, "the scroll as it is")
        (trident, tdmg, tcls, tdesc), = self.q("SELECT patternid, weapondamage, classuse, itemdesc FROM itempattern "
                                              "WHERE itemname = 'Earthenguard Trident'")
        self.assertEqual((trident, tdmg, tcls), (747438, 815, 4))
        self.assertIn("Procs 100 power to its wielder", tdesc)
        (heart, hcls, hslot), = self.q("SELECT patternid, classuse, equipslot FROM itempattern WHERE itemname = "
                                       "\"Serpent's Heart\"")
        self.assertEqual((hcls, hslot), (1 << 14, -1))
        self.assertIn(heart, cls)
        self.assertEqual(self.q("SELECT line FROM itemeffect WHERE patternid = ?", heart),
                         [("Self, Summon Miraculous Draught x3, Unlimited Uses",)])
        (atri, admg), = self.q("SELECT patternid, weapondamage FROM itempattern WHERE itemname = 'Ascended Earthenguard Trident'")
        self.assertEqual(admg, 897)
        self.assertIn(atri, cls)
        for pid in cls + corpse:
            if pid not in (heart, 716230):
                self.assertTrue(self.q("SELECT 1 FROM itemascended WHERE patternid = ?", pid), pid)
        self.assertEqual(self.q("SELECT chance, rolls FROM npcloot WHERE npc_name = 'sarek the betrayer'"), [(100, 1)] * 2)
        self.assertEqual(self.q("SELECT coins_min, coins_max FROM npccoins"), [(800, 800)])
        n = self.q("SELECT count(*) FROM gm_audit")[0][0]
        self.assertEqual(n, self.q(f"SELECT count(*) FROM {tool.LOG}")[0][0])

        # a second apply writes nothing
        after = snapshot(self.path)
        code, out = run("--db", self.path, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to write", out)
        self.assertEqual(snapshot(self.path), after)

        # --dump reads what the Go side checks
        code, out = run("--db", self.path, "--shard", "heirloom", "--dump")
        self.assertEqual(code, 0, out)

        # undo: every table as it was, the log gone
        code, out = run("--db", self.path, "--shard", "heirloom", "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(snapshot(self.path), before)
        self.assertGreater(self.q("SELECT count(*) FROM gm_audit")[0][0], n)

    def test_held_ascended_kept(self):
        run("--db", self.path, "--shard", "live", "--apply")
        (pid,), = self.q("SELECT patternid FROM itempattern WHERE itemname = 'Ascended earthshard'")
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, ?)", (pid,))
        db.commit()
        db.close()
        code, out = run("--db", self.path, "--shard", "live", "--undo")
        self.assertEqual(code, 0, out)
        self.assertIn(f"itemascended {pid}: kept, charinventory holds it", out)
        self.assertIn(f"itempattern {pid}: kept, with its record", out)
        self.assertTrue(self.q("SELECT 1 FROM itempattern WHERE patternid = ?", pid))
        self.assertFalse(self.q("SELECT 1 FROM loottables"), "the tables go though a record stays")

    def refused(self, sql, *a):
        db = sqlite3.connect(self.path)
        db.execute(sql, a)
        db.commit()
        db.close()
        before = snapshot(self.path)
        code, out = run("--db", self.path, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(snapshot(self.path), before)
        return out

    def test_refuses_sarek_not_as_read(self):
        self.refused("UPDATE npcs SET npc_level = 66 WHERE id = 120970")

    def test_refuses_another_sarek(self):
        self.refused("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level) VALUES "
                     "('Sarek the Betrayer', 1, 100, 0, 100, 67)")

    def test_refuses_a_sarek_in_the_room_not_its_own(self):
        self.refused("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level) VALUES "
                     "('Sarek the Betrayer', 1, 4700, 110.375, 2500, 67)")

    def test_refuses_anvil_zone_changed(self):
        self.refused("UPDATE classtemplates SET abilities = '[]' WHERE name = 'anvil of sarek'")

    def test_refuses_step_changed(self):
        out = self.refused("UPDATE queststeps SET events = '' WHERE questid = 101029 AND step = 19")
        self.assertIn("queststeps.events", out)

    def test_refuses_random_template_changed(self):
        self.refused("UPDATE classtemplates SET hp_pct = 300 WHERE name = 'sarek the betrayer'")

    def test_refuses_an_arios_template_by_name(self):
        self.refused("INSERT INTO npctemplate (name, template) VALUES ('arios the battered', 'roamer')")

    def test_refuses_a_trident_not_its_own(self):
        self.refused("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack) VALUES "
                     "(5, 'Earthenguard Trident', 15, 1)")


if __name__ == "__main__":
    unittest.main()
