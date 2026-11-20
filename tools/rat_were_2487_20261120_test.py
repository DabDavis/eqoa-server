#!/usr/bin/env python3
"""rat_were_2487_20261120.py on a small database holding Debts and Favors as both shards had it on 2026-11-20: the
report writes nothing; --apply makes the twelve steps (the ambush, the bribe or fight), moves a character on the quest
with its step, adds Cogwhisker's npcaggro row, a gm_audit line a row; a second apply writes nothing; --undo puts every
row back exactly. Steps changed since (a GM's) are refused."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "rat_were_2487_20261120.py")
sys.path.insert(0, HERE)
import rat_were_2487_20261120 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', npcid INTEGER NOT NULL DEFAULT 0, items TEXT NOT NULL DEFAULT '',
    tunar INTEGER NOT NULL DEFAULT 0, count INTEGER NOT NULL DEFAULT 1, world INTEGER NOT NULL DEFAULT 0,
    x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 15,
    journal TEXT NOT NULL DEFAULT '', answer TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '',
    waiting TEXT NOT NULL DEFAULT '', notyet TEXT NOT NULL DEFAULT '', reminder TEXT NOT NULL DEFAULT '',
    xp INTEGER NOT NULL DEFAULT 0, rewardtunar INTEGER NOT NULL DEFAULT 0, rewarditems TEXT NOT NULL DEFAULT '',
    factions TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '', chance REAL NOT NULL DEFAULT 0,
    events TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '',
    height REAL NOT NULL DEFAULT 0, anyorder INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
    PRIMARY KEY (questid, step, target));
CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
    count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
    updated TEXT NOT NULL DEFAULT '', picks TEXT NOT NULL DEFAULT '', PRIMARY KEY (serverid, questid));
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', leash REAL NOT NULL DEFAULT 0);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO quests VALUES (?, ?, 'From EQOA-Data.')", (tool.QUEST, tool.QUEST_NAME))
    for i, (kind, target) in enumerate(tool.OLD_STEPS, 1):
        db.execute("INSERT INTO queststeps (questid, step, kind, target, journal, items) VALUES (?, ?, ?, ?, ?, ?)",
                   (tool.QUEST, i, kind, target, f"old {i}", "701536" if i == 1 else ""))
    for sid, step in ((152, 9), (153, 8), (154, 1), (155, 0)):
        db.execute("INSERT INTO charquests (serverid, questid, step, count, done) VALUES (?, ?, ?, 1, ?)",
                   (sid, tool.QUEST, step, 1 if step == 0 else 0))
    for n in ("Guard Wexler", "Guard Wode", "Cogwhisker", "bandit henchman", "a rat"):
        db.execute("INSERT INTO npcs (npc_name) VALUES (?)", (n,))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
           for t in ("quests", "queststeps", "queststeptargets", "charquests", "npcaggro")}
    db.close()
    return out


class RatWere(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        start = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), start, "the report wrote")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        st = db.execute("SELECT step, kind, target, items, journal, choices, events, factions, count FROM queststeps ORDER BY step").fetchall()
        self.assertEqual([(s[1], s[2]) for s in st], [
            ("talk", "shady trafficker"), ("kill", "Guard Wexler"), ("give", "shady trafficker"),
            ("talk", "Shady Swashbuckler"), ("talk", "Rathei Slaerin"), ("talk", "Yindrius"), ("talk", "Shady Swashbuckler"),
            ("talk", "Rolby Rumblebelly"), ("talk", "Shady Swashbuckler"), ("talk", "Cogwhisker"), ("kill", "Cogwhisker"),
            ("talk", "Shady Swashbuckler")])
        self.assertEqual(st[2][3], "701536", "the purse's hand-in kept its items")
        self.assertEqual(st[1][8], 2)
        ch = json.loads(st[9][5])
        self.assertEqual([(o.get("cost", 0), o.get("skip", 0)) for o in ch["options"]], [(1000, 1), (0, 0)])
        self.assertTrue(all(len(o["answer"]) <= 63 for o in ch["options"]))
        self.assertEqual([e["do"] for e in json.loads(st[10][6])], ["attack", "spawn"])
        self.assertEqual(st[10][7], "Topi Di Ombra:+200")
        self.assertEqual(st[11][4], "old 9")
        self.assertEqual(db.execute("SELECT serverid, step, count FROM charquests ORDER BY serverid").fetchall(),
                         [(152, 12, 0), (153, 10, 0), (154, 1, 0), (155, 0, 1)])
        self.assertEqual(db.execute("SELECT target FROM queststeptargets").fetchall(), [("Guard Wode",)])
        self.assertEqual(db.execute("SELECT aggressive FROM npcaggro WHERE npc_name = 'Cogwhisker'").fetchone(), (0,))
        self.assertGreater(db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0], 20)
        db.close()
        r = run(self.db, "--apply")
        self.assertIn("nothing to do", r.stdout)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), start)

    def test_changed_steps_refused(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE queststeps SET target = 'someone' WHERE step = 8")
        db.commit()
        db.close()
        before = dump(self.db)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
