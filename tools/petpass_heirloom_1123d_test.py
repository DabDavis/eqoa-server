#!/usr/bin/env python3
"""Tests for petpass_heirloom_1123d.py on a small database holding the rows as read on Heirloom: a dry run writes
nothing; --apply writes each change once (a second --apply nothing, a report then says every change is already
made); Mirin's steps are renumbered with a character past the first moved along; a row not as read refuses the whole
run; --undo puts every row back exactly and drops the log."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_heirloom_1123d as t  # noqa: E402

STEP_COLS = """questid INTEGER, step INTEGER, kind TEXT DEFAULT 'talk', target TEXT DEFAULT '', npcid INTEGER DEFAULT 0,
    items TEXT DEFAULT '', tunar INTEGER DEFAULT 0, count INTEGER DEFAULT 1, world INTEGER DEFAULT 0, x REAL DEFAULT 0,
    y REAL DEFAULT 0, z REAL DEFAULT 0, radius REAL DEFAULT 15, journal TEXT DEFAULT '', answer TEXT DEFAULT '',
    says TEXT DEFAULT '', events TEXT DEFAULT '', chance REAL DEFAULT 0, height REAL DEFAULT 0,
    PRIMARY KEY (questid, step)"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(f"""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, recast INTEGER);
        CREATE TABLE classspells (class INTEGER, spellid INTEGER, PRIMARY KEY (class, spellid));
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, requires TEXT DEFAULT '',
            enabled INTEGER DEFAULT 1);
        CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER);
        CREATE TABLE questmarks (questid INTEGER PRIMARY KEY, note TEXT NOT NULL DEFAULT '');
        CREATE TABLE queststeps ({STEP_COLS});
        CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER DEFAULT 0,
            PRIMARY KEY (serverid, questid));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER, levelreq INTEGER,
            equipslot INTEGER, procanim INTEGER DEFAULT 0);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL,
            y_coord REAL, z_coord REAL, npc_id INTEGER DEFAULT 0, npc_type INTEGER DEFAULT 0);
        CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL, note TEXT DEFAULT '', source TEXT DEFAULT '');
        CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, stackleft INTEGER,
            remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER,
            listnumber INTEGER);
        CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
            scope TEXT NOT NULL DEFAULT 'near', radius REAL NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT -1,
            x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, steps TEXT NOT NULL DEFAULT '',
            notes TEXT NOT NULL DEFAULT '');
    """)
    for name, (sid, old, _) in t.RECASTS.items():
        db.execute("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, name, old))
        db.execute("INSERT INTO classspells VALUES (?, ?)", (t.MONK, sid))
    db.execute("INSERT INTO spellpattern VALUES (?, ?, 90)", (t.ANGUISH_SPELL, t.ANGUISH))
    db.execute("INSERT INTO classspells VALUES (9, ?)", (t.ANGUISH_SPELL,))
    # the level 17 quests: eleven chain heads and Highpass's part 2
    cities = ["fayspire", "freeport", "grobb", "halas", "highbourne", "highpass", "klick-anon", "moradhim",
              "neriak-nektulos", "qeynos", "rivervale"]
    for i, c in enumerate(cities):
        key = f"17/{c}/p1" if c == "highpass" else f"17/{c}/main"
        db.execute("INSERT INTO quests (questid, name, giver) VALUES (?, ?, 'x')", (100747 + i, f"Q {c}"))
        db.execute("INSERT INTO quest_import_quests VALUES (?, ?)", (key, 100747 + i))
    db.execute("INSERT INTO quests (questid, name, giver, requires) VALUES (100799, 'Courier', 'x', '100752')")
    db.execute("INSERT INTO quest_import_quests VALUES ('17/highpass/p2', 100799)")
    # Captain Scilond's Scouts
    db.execute("INSERT INTO quests (questid, name, giver) VALUES (101285, ?, 'Captain Scilond')", (t.SCILOND,))
    for step, npc, _ in t.SCILOND_CON:
        db.execute("INSERT INTO queststeps (questid, step, kind, target, count, events) VALUES (101285, ?, 'loot', ?, 5, ?)",
                   (step, npc, t.events({"on": "start", "do": "spawn", "npc": npc, "hostile": True, "count": 5})))
    # Mirin, a character on each step
    db.execute("INSERT INTO quests (questid, name, giver) VALUES (101353, ?, ?)", (t.MIRIN_Q, t.MIRIN_GIVER))
    db.execute("INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (101353, 1, 'kill', 'Mirin', 'Defeat Mirin.')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, says) VALUES (101353, 2, 'talk', 'Dorandas', 'Yours.')")
    db.executemany("INSERT INTO charquests VALUES (?, 101353, ?, ?)", [(1, 1, 0), (2, 2, 0), (3, 0, 1)])
    # the archivists and the scrolls
    db.execute("INSERT INTO itempattern VALUES (716264, ?, 2048, 46, -1, 0)", (t.SHROUD,))
    db.execute("INSERT INTO itempattern VALUES (716271, ?, 512, 46, -1, 0)", (t.ANGUISH,))
    db.execute("INSERT INTO itempattern VALUES (?, ?, 32767, 1, -1, 0)", (t.EYE, t.EYE_NAME))
    for a in (162588, 165284):
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_id, npc_type) "
                   "VALUES (?, 0, 1, 1, 1, ?, 129)", (t.ARCHIVIST, a))
        db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                   "listnumber) VALUES (?, 1, 0, 0, 716264, -1, 1, 13)", (a,))
    # the creatures
    db.execute("INSERT INTO quests (questid, name, giver) VALUES (100920, ?, 'x')", (t.GREED_Q,))
    db.execute("INSERT INTO quests (questid, name, giver) VALUES (101368, ?, 'x')", (t.LION_Q,))
    knight = {"on": "start", "do": "spawn", "npc": "Siloth knight", "hostile": True}
    db.execute("INSERT INTO queststeps (questid, step, kind, target, events) VALUES (101368, 2, 'kill', 'Siloth knight', ?)",
               (t.events(knight, knight, knight),))
    for name, old, _ in (t.GREED, t.TOBUL, t.LIONELL):
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES (?, 0, ?, ?, ?)", (name, *old))
    tobul = db.execute("SELECT id FROM npcs WHERE npc_name = 'Tobul'").fetchone()[0]
    db.execute("INSERT INTO npcwander VALUES (?, ?, 'disperse', 'disperse')", (tobul, t.TOBUL_WANDER[0]))
    for old, _ in t.WHIPPERS:
        db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES ('mindwhipper', 0, ?, ?, ?)", old)
    db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord) VALUES (?, 0, ?, 79, ?)",
               (t.HIVE[0], *t.HIVE[1]))
    # Goblin Attackers: Honjour
    db.execute("INSERT INTO quests (questid, name, giver) VALUES (100944, ?, ?)", (t.HONJOUR_Q, t.HONJOUR_GIVER))
    db.execute("INSERT INTO queststeps (questid, step, kind, target) VALUES (100944, 2, 'talk', 'Watcher Gabnu')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (100944, 3, 'loot', 'The Captain', "
               "'Kill the goblin and froglok elites and their captain until you have a glowing silver shard.')")
    for rid, name, (x, z) in t.FROGLOKS + [t.CAPTAIN]:
        db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord) VALUES (?, ?, 0, ?, 57.7, ?)",
                   (rid, name, x, z))
    db.execute("INSERT INTO gmtriggers (slot, name) VALUES (315, 'Another')")
    db.commit()
    db.close()


def snapshot(path):
    db = sqlite3.connect(path)
    out = {}
    for tbl in ("gmtriggers", "npcspawnonly", "itempattern", "spellpattern", "questmarks", "queststeps", "charquests", "npcs", "npcwander", "npcitem"):
        out[tbl] = sorted(db.execute(f"SELECT * FROM {tbl}").fetchall(), key=repr)
    db.close()
    return out


def run(*args):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(buf):
        code = t.main(list(args))
    return code, buf.getvalue()


class PetPass1123d(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_dry_apply_again_undo(self):
        before = snapshot(self.path)
        code, out = run("--db", self.path)
        self.assertEqual(code, 0, out)
        self.assertIn("dry run: 36 rows would change", out)
        self.assertEqual(snapshot(self.path), before)

        code, out = run("--db", self.path, "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 36 rows", out)
        # 2771
        got = dict(self.q("SELECT spellname, recast FROM spellpattern WHERE spellid != ?", (t.ANGUISH_SPELL,)))
        self.assertEqual(got, {n: new for n, (_, _, new) in t.RECASTS.items()})
        self.assertEqual(self.q("SELECT procanim FROM itempattern WHERE patternid = ?", (t.EYE,))[0][0], t.GEM_TYPE)
        # Mirin is left to his own part (--mirin)
        self.assertEqual(self.q("SELECT step, kind FROM queststeps WHERE questid = 101353 ORDER BY step"),
                         [(1, "kill"), (2, "talk")])
        # 2773: the eleven heads, not the follow-up
        marked = {r[0] for r in self.q("SELECT questid FROM questmarks")}
        self.assertEqual(marked, set(range(100747, 100758)))
        # 2776
        for step, npc, con in t.SCILOND_CON:
            ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 101285 AND step = ?", (step,))[0][0])
            self.assertEqual((ev[0]["npc"], ev[0]["con"], ev[0]["count"]), (npc, con, 5))
        # 2777 (--mirin): reach, kill, talk; the character on step 2 now on 3, the others as they were
        code, out = run("--db", self.path, "--mirin", "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 4 rows", out)
        j = self.q("SELECT journal FROM queststeps WHERE questid = 101353 AND step = 1")[0][0]
        self.assertEqual(j, "Defeat Mirin.")
        steps = self.q("SELECT step, kind, target, radius FROM queststeps WHERE questid = 101353 ORDER BY step")
        self.assertEqual(steps, [(1, "reach", "", 60.0), (2, "kill", "Mirin", 15.0), (3, "talk", "Dorandas", 15.0)])
        ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 101353 AND step = 1")[0][0])
        self.assertEqual([e["do"] for e in ev], ["hide", "spawn", "say"])
        self.assertEqual(self.q("SELECT serverid, step, done FROM charquests ORDER BY serverid"),
                         [(1, 1, 0), (2, 3, 0), (3, 0, 1)])
        # 2778: one row an archivist
        self.assertEqual(self.q("SELECT npcid, listnumber FROM npcitem WHERE patternid = 716271 ORDER BY npcid"),
                         [(162588, 13), (165284, 13)])
        # the moves
        for name, _, new in (t.GREED, t.TOBUL, t.LIONELL):
            x, y, z = self.q("SELECT x_coord, y_coord, z_coord FROM npcs WHERE npc_name = ?", (name,))[0]
            self.assertAlmostEqual(x, new[0], 3)
            self.assertAlmostEqual(z, new[2], 3)
        self.assertEqual(self.q("SELECT radius FROM npcwander")[0][0], t.TOBUL_WANDER[1])
        self.assertEqual(sorted(self.q("SELECT x_coord, z_coord FROM npcs WHERE npc_name = 'mindwhipper'")),
                         sorted((n[0], n[2]) for _, n in t.WHIPPERS))
        ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 101368 AND step = 2")[0][0])
        self.assertEqual([(e["x"], e["z"]) for e in ev], [(x, z) for x, _, z in t.AMBUSH])
        # 2796: the waves
        self.assertEqual({r[0] for r in self.q("SELECT name FROM npcspawnonly")}, {"#122636", "#122637", "#122638"})
        slot, steps = self.q("SELECT slot, steps FROM gmtriggers WHERE name = ?", (t.TRIGGER,))[0]
        self.assertEqual(slot, 316)
        steps = json.loads(steps)
        spawns = [st for st in steps if st["do"] == "spawn"]
        self.assertEqual([(st["npc"], st["count"]) for st in spawns],
                         [("#122636", 2), ("#122637", 2), ("#122636", 2), ("#122637", 2), ("#122638", 1)])
        self.assertTrue(all(st["hostile"] and (st["x"], st["z"]) == (t.RAMP[0], t.RAMP[2]) for st in spawns))
        self.assertEqual(sum(st["do"] == "awaitdead" for st in steps), 5)
        ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 100944 AND step = 2")[0][0])
        self.assertEqual((ev[0]["do"], ev[0]["trigger"]), ("trigger", t.TRIGGER))
        # every row audited
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 40)

        code, out = run("--db", self.path, "--apply", "--no-backup")
        self.assertIn("applied already", out)
        code, out = run("--db", self.path)
        self.assertEqual(code, 0, out)
        self.assertIn("dry run: 0 rows would change", out)
        code, out = run("--db", self.path, "--mirin")
        self.assertIn("dry run: 0 rows would change", out)

        code, out = run("--db", self.path, "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        code, out = run("--db", self.path, "--mirin", "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(snapshot(self.path), before)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (t.LOG,))[0][0], 0)

    def test_refuses_a_row_not_as_read(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE spellpattern SET recast = 60 WHERE spellname = 'Fiery Punch'")
        db.commit()
        db.close()
        before = snapshot(self.path)
        code, out = run("--db", self.path, "--apply", "--no-backup")
        self.assertEqual(code, 2, out)
        self.assertIn("Fiery Punch", out)
        self.assertEqual(snapshot(self.path), before)

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.path, "--apply", "--no-backup")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcwander SET radius = 99")
        db.commit()
        db.close()
        code, out = run("--db", self.path, "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertIn("left     npcwander", out)
        self.assertEqual(self.q("SELECT radius FROM npcwander")[0][0], 99)


if __name__ == "__main__":
    unittest.main()
