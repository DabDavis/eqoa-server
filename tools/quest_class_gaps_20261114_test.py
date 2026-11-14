#!/usr/bin/env python3
"""quest_class_gaps_20261114.py on a small database: the report writes nothing; --apply fills a guide-silent class
with an unused piece of its own level's set (not one another quest of the level gives it, nor a second record of a
piece it holds), fills a guide-named class with the guide's item when that item is its and of the level, leaves a
class whose set has none left, widens a gate only for a class that then has a reward, and leaves rewards already given;
a second apply finds nothing to do; --undo puts back exactly what was there. A step whose rewards are shared items is
not filled. A quest of another name is refused, nothing written."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "quest_class_gaps_20261114.py")

WAR, MAG, NEC, WIZ, ALC = 0, 10, 11, 13, 14
PLAN = {"quests": [
    {"questid": 1, "name": "One", "level": 17, "widen": [ALC, WIZ],
     "steps": [{"step": 2, "fill": {str(MAG): {"set": True}, str(NEC): {"set": True}, str(WIZ): {"set": True},
                                    str(ALC): {"set": True}, str(WAR): {"guide": ["Soldier's Belt"]}}}]},
    {"questid": 3, "name": "Shared", "level": 50, "widen": [], "steps": [{"step": 1, "fill": {str(MAG): {"set": True}}}]},
]}


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, classes TEXT NOT NULL DEFAULT '',
            minlevel INTEGER, notes TEXT NOT NULL DEFAULT '');
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, byclass TEXT NOT NULL DEFAULT '',
            rewarditems TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER, levelreq INTEGER,
            equipslot INTEGER);
        CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, tclass INTEGER);
        CREATE TABLE charquests (serverid INTEGER, questid INTEGER, done INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        -- the magician's set: the gloves another level 17 quest gives, the circlet free
        INSERT INTO itempattern VALUES (101, 'Chromatic Gloves', 1024, 17, 19);
        INSERT INTO itempattern VALUES (102, 'Chromatic Circlet', 1024, 17, 1);
        INSERT INTO itempattern VALUES (103, 'chromatic tabard', 1024, 17, 2);
        -- the necromancer's: a ring given, its second record the only other piece
        INSERT INTO itempattern VALUES (201, 'Ring of Shadow', 2048, 17, 8);
        INSERT INTO itempattern VALUES (202, 'Ring of the Shadow', 2048, 17, 8);
        INSERT INTO itempattern VALUES (203, 'Robe of Shadow', 2048, 18, 2);
        -- the alchemist's: two pieces free
        INSERT INTO itempattern VALUES (301, 'Blue Aura Helm', 16384, 17, 1);
        INSERT INTO itempattern VALUES (302, 'Blue Aura Tunic', 16384, 17, 5);
        -- the warrior's guide item and its level 60 namesake; the wizard's only piece, given
        INSERT INTO itempattern VALUES (401, 'Soldier''s Belt', 32767, 60, 9);
        INSERT INTO itempattern VALUES (402, 'Soldier''s Gloves', 1, 17, 19);
        INSERT INTO itempattern VALUES (403, 'Soldier''s Tunic', 1, 17, 5);
        INSERT INTO itempattern VALUES (501, 'Robe of Insight', 8192, 17, 2);
        INSERT INTO itempattern VALUES (900, 'Shared Staff', 15360, 50, 18);
        INSERT INTO quests VALUES (1, 'One', '0, 10, 11, 13', 17, 'n.');
        INSERT INTO quests VALUES (2, 'Two', '', 17, '');
        INSERT INTO quests VALUES (3, 'Shared', '', 50, '');
        INSERT INTO queststeps VALUES (1, 2, '{"3":{"items":"999"},"12":{}}', '');
        INSERT INTO queststeps VALUES (2, 4, '{"10":{"items":"101"},"11":{"items":"201"},"13":{"items":"501"},"0":{"items":"402"}}', '');
        INSERT INTO queststeps VALUES (3, 1, '{"11":{"items":"900"}}', '');
        INSERT INTO itempattern VALUES (999, 'Gloves of Malice', 8, 17, 19);
        INSERT INTO characters VALUES (7, 'Mage', 10);
        INSERT INTO charquests VALUES (7, 1, 1);
    """)
    db.commit()
    db.close()


def run(db, plan, *args):
    p = subprocess.run([sys.executable, TOOL, "--db", db, "--plan", plan, *args], capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def snapshot(db):
    c = sqlite3.connect(db)
    out = (c.execute("SELECT * FROM quests ORDER BY questid").fetchall(),
           c.execute("SELECT * FROM queststeps ORDER BY questid, step").fetchall())
    c.close()
    return out


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        self.plan = os.path.join(self.dir.name, "plan.json")
        make(self.db)
        with open(self.plan, "w") as f:
            json.dump(PLAN, f)

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        before = snapshot(self.db)
        rc, out = run(self.db, self.plan)
        self.assertEqual(rc, 0, out)
        self.assertIn("would be written", out)
        self.assertEqual(snapshot(self.db), before)

        rc, out = run(self.db, self.plan, "--apply")
        self.assertEqual(rc, 0, out)
        c = sqlite3.connect(self.db)
        bc = json.loads(c.execute("SELECT byclass FROM queststeps WHERE questid = 1 AND step = 2").fetchone()[0])
        self.assertEqual(bc[str(MAG)], {"items": "102"})        # the circlet: the gloves are Two's, the tabard lowercase
        self.assertNotIn(str(NEC), bc)                         # Ring of the Shadow is the ring it has: none left
        self.assertEqual(bc[str(ALC)], {"items": "301"})
        self.assertNotIn(str(WIZ), bc)                         # its one piece is Two's
        self.assertEqual(bc[str(WAR)], {"items": "403"})       # the guide's belt is level 60: its set's free piece
        self.assertEqual(bc["3"], {"items": "999"})            # given already: kept
        self.assertEqual(bc["12"], {})
        cl = c.execute("SELECT classes, notes FROM quests WHERE questid = 1").fetchone()
        self.assertEqual(cl[0], "0, 10, 11, 13, 14")           # the alchemist has a reward now, the wizard none
        self.assertIn("quest_class_gaps_20261114.py", cl[1])
        self.assertEqual(c.execute("SELECT byclass FROM queststeps WHERE questid = 3").fetchone()[0], '{"11":{"items":"900"}}')
        n = c.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        self.assertEqual(n, 3)                                 # the step, the gate, the notes
        c.close()
        self.assertIn("finished before by {'MAG': 1}", out)

        rc, out = run(self.db, self.plan, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to do", out)

        rc, out = run(self.db, self.plan, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(snapshot(self.db), before)
        c = sqlite3.connect(self.db)
        self.assertIsNone(c.execute("SELECT 1 FROM sqlite_master WHERE name = 'quest_class_gaps_1114_log'").fetchone())
        c.close()

    def test_refused_other_name(self):
        c = sqlite3.connect(self.db)
        c.execute("UPDATE quests SET name = 'Other' WHERE questid = 1")
        c.commit()
        c.close()
        before = snapshot(self.db)
        rc, out = run(self.db, self.plan, "--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("refused", out)
        self.assertEqual(snapshot(self.db), before)


if __name__ == "__main__":
    unittest.main()
