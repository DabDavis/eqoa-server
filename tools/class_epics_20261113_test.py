#!/usr/bin/env python3
"""class_epics_20261113.py on a small database: the report writes nothing; --apply puts a source's stats on a record
(a whole block's unnamed stats 0, a DUR raised only, a kept column left), makes a missing record and its Ascended one,
points the reward options at it, makes every Ascended copy its base again by the rule, keeps the owner's proc words, and
writes a gm_audit line a row; a second apply does nothing; --undo puts every table back exactly. Two entries that
disagree on one record write nothing."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import class_epics_20261113 as ce  # noqa: E402

TOOL = os.path.join(HERE, "class_epics_20261113.py")

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL DEFAULT 0,
    itemicon INTEGER NOT NULL DEFAULT 0, equipslot INTEGER NOT NULL DEFAULT 0, rent INTEGER NOT NULL DEFAULT 1,
    attacktype INTEGER NOT NULL DEFAULT 0, weapondamage INTEGER NOT NULL DEFAULT 0, maxstack INTEGER NOT NULL DEFAULT 1,
    duration INTEGER NOT NULL DEFAULT 0, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL DEFAULT '',
    model INTEGER NOT NULL DEFAULT 0, str INTEGER, sta INTEGER, agi INTEGER, dex INTEGER, wis INTEGER,
    intelligence INTEGER, cha INTEGER, HPMAX INTEGER, POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER,
    DR INTEGER, FR INTEGER, CR INTEGER, LR INTEGER, AR INTEGER);
CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '');
CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, target_line TEXT NOT NULL DEFAULT '');
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, items TEXT NOT NULL DEFAULT '',
    rewarditems TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (questid, step));
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER NOT NULL, patternid INTEGER NOT NULL);
"""

# 10 Rod (a level 49 record, two entries: one names STA, a whole block names INT), 11 Shroud (DUR 65 over the
# source's 50), 12 Axe (a kept DMG; the owner's proc words), 13 Staff (the look of 14), 980001 the Rod's Ascended
# record, OFF the rule; quest 1 step 2 gives the Shroud's Ascended record and should give the new Totem's too.
ROWS = [
    (10, "Rod of Hatred", 18, 0, 50, "", 1, 10, 25, 50, 0),
    (11, "Shroud of Blaze", 2, 0, 65, "A robe.", 2, 0, 0, 0, 140),
    (12, "Axe of Innocence", 14, 801, 65, "A flawless axe.\nProcs 250 damage.", 3, 30, 0, 0, 0),
    (13, "Staff of Light", 15, 928, 65, "", 4, 0, 0, 0, 0),
    (14, "Monk Staff", 15, 668, 50, "", 99, 0, 0, 0, 0),
    (980001, "Ascended Rod of Hatred", 18, 0, 50, "", 1, 11, 28, 55, 0),
    (980002, "Ascended Shroud of Blaze", 2, 0, 65, "A robe.", 2, 0, 0, 0, 154),
]

DATA = {
    "items": [
        {"cls": "necromancer", "level": 49, "name": "Rod of Hatred", "source": "questmasters id417", "quote": "q",
         "stats": {"sta": 25, "int": 10}},
        {"cls": "necromancer", "level": 49, "name": "Rod of Hatred", "source": "questmasters id417", "quote": "q",
         "stats": {"sta": 25}},
        {"cls": "necromancer", "level": 60, "name": "Shroud of Blaze", "source": "questmasters id419", "quote": "q",
         "stats": {"sta": 25, "ac": 85}, "whole": True, "dur": 50},
        {"cls": "ranger", "level": 60, "name": "Axe of Innocence", "source": "questmasters id419", "quote": "q",
         "stats": {"str": 30}, "dmg": 928, "keep": {"dmg": "one-handed"}},
        {"cls": "wizard", "level": 60, "name": "Staff of Light", "source": "questmasters id419", "quote": "q",
         "stats": {}, "look_of": "Monk Staff", "dur": 75},
    ],
    "new": [{"name": "Totem of Bones", "prefer_id": 10, "source": "s",
             "row": {"itemname": "Totem of Bones", "equipslot": 18, "duration": 65, "intelligence": 30, "POWMAX": 150,
                     "itemcost": 1, "itemicon": 5, "model": 7, "rent": 1}}],
    "quests": [{"key": "60/evil-caster/reward-necromancer", "step": 2, "source": "s",
                "options": ["Shroud of Blaze", "Totem of Bones"]}],
    "owner": [{"name": "Axe of Innocence", "words": "Procs 250 damage.", "said": "the owner"}],
}


def make_db(path, data_path, data=DATA):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for pid, name, slot, dmg, dur, desc, model, sta, intel, powmax, ac in ROWS:
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, weapondamage, duration, itemdesc, model, "
                   "sta, intelligence, POWMAX, AC, str) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",
                   (pid, name, slot, dmg, dur, desc, model, sta, intel, powmax, ac, 30 if pid == 12 else 0))
    db.execute("INSERT INTO itemascended VALUES (980001, 10, '', ''), (980002, 11, '', '')")
    db.execute("INSERT INTO quests VALUES (1, 'The Necromancer''s Epic Reward', "
               "'EQOA-Data 60/evil-caster/reward-necromancer (tools/import_quests.py)')")
    db.execute("INSERT INTO queststeps (questid, step, choices) VALUES (1, 2, ?)",
               (json.dumps({"ask": "Which do you take?", "options": [
                   {"answer": "The Ascended Shroud of Blaze", "says": "Wear it well.", "items": "980002"}]}),))
    db.execute("INSERT INTO charinventory VALUES (1, 7, 980001)")
    db.commit()
    db.close()
    with open(data_path, "w") as f:
        json.dump(data, f)


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
           for t in ("itempattern", "itemascended", "itemeffect", "queststeps")}
    db.close()
    return out


def run(*args):
    return subprocess.run([sys.executable, "-I", "-E", "-s", TOOL, *args], capture_output=True, text=True)


class ClassEpics(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.tmp.name, "eqoa.db")
        self.data = os.path.join(self.tmp.name, "data.json")
        make_db(self.db, self.data)

    def tearDown(self):
        self.tmp.cleanup()

    def item(self, where, args=()):
        db = sqlite3.connect(self.db)
        db.row_factory = sqlite3.Row
        r = db.execute(f"SELECT * FROM itempattern WHERE {where}", args).fetchone()
        db.close()
        return dict(r) if r else None

    def test_report_writes_nothing(self):
        before = dump(self.db)
        r = run("--db", self.db, "--data", self.data)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("Rod of Hatred (10; Ascended 980001): STA 10->25; INT 25->10", r.stdout)
        self.assertEqual(dump(self.db), before)

    def test_apply_second_apply_undo(self):
        before = dump(self.db)
        r = run("--db", self.db, "--data", self.data, "--apply", "--no-backup")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rod = self.item("patternid = 10")
        self.assertEqual((rod["sta"], rod["intelligence"]), (25, 10))
        # a whole block: unnamed stats 0; DUR 65 kept over 50
        sh = self.item("patternid = 11")
        self.assertEqual((sh["sta"], sh["AC"], sh["POWMAX"], sh["intelligence"], sh["duration"]), (25, 85, 0, 0, 65))
        # a kept column; the owner's words untouched
        axe = self.item("patternid = 12")
        self.assertEqual(axe["weapondamage"], 801)
        self.assertTrue(axe["itemdesc"].endswith("Procs 250 damage."))
        self.assertIn("Axe of Innocence (12): present", r.stdout)
        # the look of another record; a DUR raised
        st = self.item("patternid = 13")
        self.assertEqual((st["model"], st["duration"]), (99, 75))
        # the Ascended copies: the rule (a tenth more, half up) from the bases as they are now
        a = self.item("patternid = 980001")
        self.assertEqual((a["sta"], a["intelligence"], a["POWMAX"]), (28, 11, 55))
        a = self.item("patternid = 980002")
        self.assertEqual((a["sta"], a["AC"], a["POWMAX"], a["itemname"]), (28, 94, 0, "Ascended Shroud of Blaze"))
        # the new record (id 10 taken: the first free of 718001-) and its Ascended one, offered
        t = self.item("itemname = 'Totem of Bones'")
        self.assertEqual(t["patternid"], 718001)
        ta = self.item("itemname = 'Ascended Totem of Bones'")
        self.assertEqual((ta["patternid"], ta["intelligence"], ta["POWMAX"]), (980003, 33, 165))
        db = sqlite3.connect(self.db)
        ch = json.loads(db.execute("SELECT choices FROM queststeps WHERE questid = 1 AND step = 2").fetchone()[0])
        self.assertEqual([(o["answer"], o["items"]) for o in ch["options"]],
                         [("The Ascended Shroud of Blaze", "980002"), ("The Ascended Totem of Bones", "980003")])
        self.assertEqual(ch["ask"], "Which do you take?")
        n_audit = db.execute("SELECT count(*) FROM gm_audit WHERE via = 'tool'").fetchone()[0]
        n_log = db.execute(f"SELECT count(*) FROM {ce.LOG}").fetchone()[0]
        db.close()
        self.assertEqual(n_audit, n_log)
        # a second apply does nothing
        after = dump(self.db)
        r = run("--db", self.db, "--data", self.data, "--apply", "--no-backup")
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.db), after)
        # undo: every table back
        r = run("--db", self.db, "--data", self.data, "--undo", "--no-backup")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_disagreement_writes_nothing(self):
        bad = json.loads(json.dumps(DATA))
        bad["items"][1]["stats"] = {"sta": 20}
        make_db(os.path.join(self.tmp.name, "b.db"), self.data, bad)
        path = os.path.join(self.tmp.name, "b.db")
        before = dump(path)
        r = run("--db", path, "--data", self.data, "--apply", "--no-backup")
        self.assertEqual(r.returncode, 2, r.stdout + r.stderr)
        self.assertIn("PROBLEM: Rod of Hatred (10): STA", r.stdout)
        self.assertEqual(dump(path), before)

    def test_rule(self):
        self.assertEqual([ce.figure(v) for v in (0, 4, 5, 15, 801, -50)], [0, 4, 6, 17, 881, -50])


if __name__ == "__main__":
    unittest.main()
