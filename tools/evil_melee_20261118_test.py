#!/usr/bin/env python3
"""tools/evil_melee_20261118.py against a made-up database: the four epic-sode 3 quests as the importer left them
(Boldunder: reach, loot, give; Ahriaana: talk, loot, give), the bosses', the ghost's and Magi'Kot's rows, the summoning
trigger, the templates, the Gem of the Sky and the Sky Portal's gate, and characters part way. No game data.

    python3 tools/evil_melee_20261118_test.py
"""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import evil_melee_20261118 as em  # noqa: E402

SCHEMA = """
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', giver TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
  target TEXT NOT NULL DEFAULT '', npcid INTEGER NOT NULL DEFAULT 0, items TEXT NOT NULL DEFAULT '',
  count INTEGER NOT NULL DEFAULT 1, world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0,
  y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 15,
  journal TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '', xp INTEGER NOT NULL DEFAULT 0,
  events TEXT NOT NULL DEFAULT '', chance REAL NOT NULL DEFAULT 0, height REAL NOT NULL DEFAULT 0,
  PRIMARY KEY (questid, step));
CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
  done INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, questid));
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, world INTEGER, x_coord REAL,
  y_coord REAL, z_coord REAL, facing INTEGER, modelid INTEGER, npc_type INTEGER, npc_id INTEGER);
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
  melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '', special_damage REAL NOT NULL DEFAULT 0,
  special_every REAL NOT NULL DEFAULT 0, special_first REAL NOT NULL DEFAULT 0, resist_kind TEXT NOT NULL DEFAULT '',
  resist_pct INTEGER NOT NULL DEFAULT 0, abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', leash REAL NOT NULL DEFAULT 0);
CREATE TABLE npccharm (name TEXT PRIMARY KEY, charmable INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', world INTEGER NOT NULL DEFAULT -1,
  x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, steps TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '');
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemcost INTEGER, itemicon INTEGER, trade INTEGER, rent INTEGER,
  maxstack INTEGER, classuse INTEGER, raceuse INTEGER, procanim INTEGER, lore INTEGER, itemname TEXT, itemdesc TEXT);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT, place TEXT, world INTEGER, x REAL, y REAL, z REAL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
  username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
  rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""

TRIGGER_STEPS = json.dumps([
    {"do": "lock", "lock": "ahriaana"}, {"do": "popup", "text": "Ahriaana, come forth!"},
    {"do": "spawn", "npc": "#122689", "world": 4, "x": 4135.7, "y": 180.1, "z": 5192.1, "spread": 1, "attackable": True,
     "minutes": 120},
    {"do": "clock", "seconds": 7200, "empty": 900}, {"do": "awaitdead"}, {"do": "wait", "seconds": 300}, {"do": "end"},
    {"do": "despawn"}])


def build(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for qid, name, giver in ((101062, "Boldunder", "Magi'Kot"), (101063, "Ahriaana the Ultimate", "Magi'Kot"),
                             (101091, "Boldunder", "Rashiroz the Rogue"),
                             (101092, "Ahriaana the Ultimate", "Rashiroz the Rogue")):
        db.execute("INSERT INTO quests VALUES (?, ?, ?, 'imported.')", (qid, name, giver))
        if name == "Boldunder":
            steps = [(1, "reach", "", 2, '[{"on":"done","do":"popup","text":"rumbling"}]'),
                     (2, "loot", "Boldunder", 0, ""), (3, "give", giver, 0, "")]
        else:
            last = giver if giver != "Magi'Kot" else "ghost of Magi'Kot"
            steps = [(1, "talk", "Magi'Kot", 0, '[{"on":"done","do":"teleport","world":4,"x":4180,"y":181,"z":5214}]'),
                     (2, "loot", "Ahriaana the Ultimate", 0, ""), (3, "give", last, 0, "")]
        for s, kind, target, world, ev in steps:
            db.execute("INSERT INTO queststeps (questid, step, kind, target, world, events) VALUES (?, ?, ?, ?, ?, ?)",
                       (qid, s, kind, target, world, ev))
    db.executemany("INSERT INTO charquests VALUES (?, ?, ?, ?)",
                   [(1, 101063, 2, 0), (2, 101063, 0, 1), (3, 101091, 3, 0), (4, 101092, 1, 0)])
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", [
        (116314, "Magi'Kot", 1223, 0, 24537.8, 52.0, 24783.3, 151, 1893243078, 128, 116314),
        (119499, "Sartheret", 4, 4, 4839.56, 250.0, 5543.79, 0, 1, 128, 0),
        (122688, "Boldunder", 0, 2, 11157.9, 41.1, 5423.9, 135, 3966371332, 0, 0),
        (122689, "Ahriaana the Ultimate", 0, 4, 4135.72, 180.06, 5192.09, 179, 1689387666, 0, 0),
        (122690, "ghost of Magi'Kot", 0, 4, 4148.16, 180.06, 5227.88, 145, 1893243078, 128, 0)])
    db.execute("UPDATE sqlite_sequence SET seq = 190869 WHERE name = 'npcs'")
    db.execute("INSERT INTO npcspawnonly VALUES ('Ahriaana the Ultimate', 'the summoning', 'epic-audit 1130')")
    db.execute("INSERT INTO npctemplate VALUES ('boldunder', 'roamer', 'named', 'roamer-seed')")
    db.execute("INSERT INTO npctemplate VALUES ('ahriaana the ultimate', 'ahriaana the ultimate', 'raid boss', 'x')")
    db.execute("INSERT INTO classtemplates (name, class, special, special_damage, special_every, note) VALUES "
               "('roamer', 0, 'Critical Hit', 7.4, 30, 'roamer')")
    db.execute("INSERT INTO classtemplates (name, class, hp_pct, melee_pct, abilities, note) VALUES "
               "('ahriaana the ultimate', 0, 8688, 3430, ?, 'Ahriaana.')", (json.dumps(em.OLD_AHRI_ABILITIES),))
    db.execute("INSERT INTO gmtriggers VALUES (172, 'Ahriaana the Ultimate', 4, 4135.7, 180.1, 5192.1, ?, 'summoning')",
               (TRIGGER_STEPS,))
    db.execute("INSERT INTO itempattern VALUES (960196, 146, 481511195, 1, 1, 1, 32767, 32767, 7, 0, 'Gem of the Sky', "
               "'From Talon.')")
    db.execute("INSERT INTO itemeffect VALUES (960196, 'Self, Teleport to Sky Portal, Unlimited Uses', 'Gem of the Sky', 'x')")
    db.execute("INSERT INTO spellgates VALUES ('sky portal', 'group', 'the Plane of Sky', 4, 4834.8, 250.1, 5524.7)")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l]
    db.close()
    return out


def run(*args):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        rc = em.main(list(args))
    return rc, buf.getvalue()


class EvilMeleeTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        build(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_report_writes_nothing(self):
        before = dump(self.path)
        rc, out = run("--db", self.path)
        self.assertEqual(rc, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(dump(self.path), before)

    def test_apply_again_and_undo(self):
        before = dump(self.path)
        rc, out = run("--db", self.path, "--apply")
        self.assertEqual(rc, 0, out)
        # Boldunder: five steps, the third raising him in the bowl, one for a group
        kinds = self.q("SELECT step, kind FROM queststeps WHERE questid = 101091 ORDER BY step")
        self.assertEqual(kinds, [(1, "reach"), (2, "reach"), (3, "reach"), (4, "loot"), (5, "give")])
        ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 101091 AND step = 3")[0][0])
        spawn = [e for e in ev if e["do"] == "spawn"][0]
        self.assertTrue(spawn["group"] and spawn["hostile"])
        self.assertEqual((spawn["x"], spawn["z"]), (em.BOWL[0], em.BOWL[2]))
        self.assertEqual(self.q("SELECT x_coord, z_coord FROM npcs WHERE id = 122688")[0], (em.BOWL[0], em.BOWL[2]))
        self.assertTrue(self.q("SELECT 1 FROM npcspawnonly WHERE name = 'Boldunder'"))
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = 'boldunder'")[0][0], "boldunder")
        # Ahriaana: the gems and the port, Magi'Kot on the isle (a new row), the trigger moved
        sky = self.q("SELECT id, world FROM npcs WHERE npc_name = 'Magi''Kot' AND world = 4")
        self.assertEqual(len(sky), 1)
        self.assertEqual(sky[0][0], 190870)
        pins = self.q("SELECT step, kind, npcid FROM queststeps WHERE questid = 101092 ORDER BY step")
        self.assertEqual(pins, [(1, "talk", 116314), (2, "talk", 190870), (3, "loot", 0), (4, "give", 0)])
        ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 101063 AND step = 1")[0][0])
        self.assertEqual(ev[0], {"on": "done", "do": "give", "items": f"{em.GEM_ID}*20", "unless_held": True})
        self.assertEqual([e["do"] for e in json.loads(self.q(
            "SELECT events FROM queststeps WHERE questid = 101063 AND step = 2")[0][0])], ["trigger"])
        t = self.q("SELECT x, z, steps FROM gmtriggers WHERE slot = 172")[0]
        self.assertEqual((t[0], t[1]), (em.SUMMON[0], em.SUMMON[2]))
        self.assertEqual(json.loads(t[2])[2]["x"], em.SUMMON[0])
        ab = json.loads(self.q("SELECT abilities FROM classtemplates WHERE name = 'ahriaana the ultimate'")[0][0])
        self.assertEqual(ab[-1]["kind"], "portfoe")
        gem = self.q("SELECT trade, maxstack, procanim, itemname FROM itempattern WHERE patternid = ?", em.GEM_ID)[0]
        self.assertEqual(gem, (0, 20, 7, "Magi'Kot's summoning gem"))
        self.assertEqual(self.q("SELECT line FROM itemeffect WHERE patternid = ?", em.GEM_ID)[0][0],
                         "Self, Teleport to Sky Portal")
        # characters moved with their steps; a finished one and one at step 1 left
        self.assertEqual(self.q("SELECT serverid, questid, step FROM charquests ORDER BY serverid"),
                         [(1, 101063, 3), (2, 101063, 0), (3, 101091, 5), (4, 101092, 1)])
        self.assertTrue(self.q("SELECT count(*) FROM gm_audit")[0][0] > 20)

        applied = dump(self.path)
        rc, out = run("--db", self.path, "--apply")
        self.assertEqual(rc, 0)
        self.assertIn("applied already", out)
        self.assertEqual(dump(self.path), applied)

        rc, out = run("--db", self.path, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(dump(self.path), before)

    def test_refusals_write_nothing(self):
        for sql in ("INSERT INTO npcspawnonly VALUES ('Boldunder', '', 'gm')",
                    "UPDATE queststeps SET kind = 'kill' WHERE questid = 101091 AND step = 2",
                    "INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('Magi''Kot', 4, 4800, 250, 5500)",
                    "UPDATE gmtriggers SET steps = '[]' WHERE slot = 172",
                    "INSERT INTO itempattern (patternid, itemname) VALUES (716990, 'something')",
                    "UPDATE classtemplates SET abilities = '[]' WHERE name = 'ahriaana the ultimate'"):
            with self.subTest(sql=sql):
                if os.path.exists(self.path + ".r"):
                    os.remove(self.path + ".r")
                build(self.path + ".r")
                db = sqlite3.connect(self.path + ".r")
                db.execute(sql)
                db.commit()
                db.close()
                before = dump(self.path + ".r")
                rc, out = run("--db", self.path + ".r", "--apply")
                self.assertEqual(rc, 1, out)
                self.assertIn("refused", out)
                self.assertEqual(dump(self.path + ".r"), before)
                os.remove(self.path + ".r")


if __name__ == "__main__":
    unittest.main()
