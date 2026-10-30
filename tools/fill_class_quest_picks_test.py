import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fill_class_quest_picks as fp  # noqa: E402
import fill_class_quest_rewards as fc  # noqa: E402
import seed_class_quest_rewards as sc  # noqa: E402

SECRET_STEMS = ["passw", "pwd", "secret", "token", "hash", "salt", "key", "ticket", "cookie", "session", "credential",
                "nonce", "oauth", "mail", "addr", "login", "phone", "birth", "device", "install", "private"]

REVIEW = {
    100184: (8571, "Freeport's militia short sword"),
    100340: (None, "a staff"),
    100056: (8583, "Freeport's fancy slippers"),
}
MADE = [(707200, 100340, 11, "Bal's Staff", 707114, "Carved by Bal Noonlittle from a meat chunk.", "the Staff of Necromancy's numbers")]
QUESTS = [  # key, questid, name, [(kind, rewarditems)]
    ("1-20/grobb/warrior/L4", 100184, "A Warrior Weapon", [("talk", "")]),
    ("1-20/klick-anon/necromancer/L4", 100340, "Bal's Staff", [("loot", ""), ("give", "")]),
    ("1-20/fayspire/wizard/L7", 100056, "The Wasp of Kara", [("give", "8578")]),
    ("1-20/qeynos/warrior/L4", 100550, "A Guard's Sword", [("give", "9999")]),
]
ITEMS = [  # id, name, icon, slot, level, classuse, raceuse, maxhp, duration, desc
    (8571, "militia short sword", 1, 12, 4, 1, 1023, 800, 50, "A sword."),
    (707114, "Staff of Necromancy", 2, 15, 4, 2048, 271, 1000, 50, "Carved by Retainer Alishal."),
    (8583, "fancy slippers", 3, 11, 7, 8192, 271, 800, 50, "Slippers."),
    (8578, "Burning Flare", fc.SCROLL_ICON, -1, 7, 8192, 1023, 0, 0, "A scroll."),
    (9999, "a set reward", 4, 12, 4, 1, 1023, 800, 50, "Set."),
]


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


class FillClassQuestPicks(unittest.TestCase):
    def setUp(self):
        self.saved = (sc.REVIEW, fp.MADE)
        sc.REVIEW, fp.MADE = dict(REVIEW), list(MADE)
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "eqoa.db")
        db = sqlite3.connect(self.path)
        db.executescript("""
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER, classes TEXT, giver TEXT,
                                 notes TEXT NOT NULL DEFAULT '');
            CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, spells TEXT NOT NULL DEFAULT '',
                                     rewarditems TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '',
                                     choices TEXT NOT NULL DEFAULT '');
            CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER, file TEXT, digest TEXT, enabled INTEGER);
            CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
            CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT);
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT, itemicon INTEGER,
                                      equipslot INTEGER, levelreq INTEGER, classuse INTEGER, raceuse INTEGER, maxhp INTEGER,
                                      duration INTEGER, weapondamage INTEGER DEFAULT 0, itemdesc TEXT);
            CREATE TABLE defaultcharacter (tclass INTEGER, race INTEGER, world INTEGER, x REAL, z REAL);
            CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, tclass INTEGER);
            CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER);
            CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, stackleft INTEGER,
                                        remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER,
                                        location INTEGER, listnumber INTEGER);
        """)
        for key, qid, name, steps in QUESTS:
            cls = fc.CLASSES.index(key.split("/")[2])
            db.execute("INSERT INTO quests VALUES (?, ?, 1, ?, 'Giver', 'The guide names no reward.')", (qid, name, str(cls)))
            db.execute("INSERT INTO quest_import_quests VALUES (?, ?, ?, '', 1)", (key, qid, key.split("/")[1] + ".json"))
            for i, (kind, ri) in enumerate(steps, 1):
                db.execute("INSERT INTO queststeps (questid, step, kind, rewarditems) VALUES (?, ?, ?, ?)", (qid, i, kind, ri))
        db.execute("INSERT INTO spellpattern VALUES (700052, 'Burning Flare', 7)")
        db.execute("INSERT INTO classspells VALUES (13, 700052, 'eqoa-data')")
        for it in ITEMS:
            db.execute("INSERT INTO itempattern (patternid, itemname, itemicon, equipslot, levelreq, classuse, raceuse, maxhp, "
                       "duration, itemdesc) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", it)
        # the races that start as each class: a gnome necromancer, an elf wizard, a troll and a human warrior
        db.executemany("INSERT INTO defaultcharacter VALUES (?, ?, 0, 0, 0)", [(11, 3), (13, 1), (0, 5), (0, 0)])
        db.executemany("INSERT INTO characters VALUES (?, ?, ?)", [(301, "Necro", 11), (302, "Warry", 0), (303, "Held", 0)])
        db.executemany("INSERT INTO charquests VALUES (?, ?, 0, 1)", [(301, 100340), (302, 100184), (303, 100184)])
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                   "listnumber) VALUES (303, 1, 800, 0, 8571, -1, 0, 0)")  # holds the pick already
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        sc.REVIEW, fp.MADE = self.saved
        shutil.rmtree(self.dir)

    def plan(self, records_only=False):
        db = sqlite3.connect(self.path, isolation_level=None)
        p = fp.Plan(db)
        p.build(records_only=records_only)
        return db, p

    def apply(self, records_only=False):
        db, p = self.plan(records_only)
        self.assertEqual(p.problems, [])
        db.execute("BEGIN IMMEDIATE")
        n = fp.apply(db, p)
        db.execute("COMMIT")
        db.close()
        return n

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def rw(self, qid):
        return self.q("SELECT rewarditems FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1", qid)[0][0]

    def test_apply_and_undo(self):
        n = self.apply()
        self.assertEqual((n["made"], n["quests"], n["given"]), (1, 3, 2))
        self.assertEqual(self.rw(100184), "8571")
        self.assertEqual(self.rw(100340), "707200")
        self.assertEqual(self.rw(100056), "8578, 8583", "beside the scroll the first fill gave")
        self.assertEqual(self.q("SELECT itemname, itemdesc, classuse, raceuse, maxhp FROM itempattern WHERE patternid = 707200")[0],
                         ("Bal's Staff", "Carved by Bal Noonlittle from a meat chunk.", 2048, 271, 1000), "the made staff, its class's")
        self.assertEqual(self.q("SELECT serverid, patternid FROM charinventory WHERE serverid IN (301, 302) ORDER BY serverid"),
                         [(301, 707200), (302, 8571)])
        self.assertEqual(self.q("SELECT count(*) FROM charinventory WHERE serverid = 303")[0][0], 1, "control: held already")
        self.assertEqual(self.rw(100550), "9999", "control: a quest the review does not name keeps its own")
        self.assertIn("The reward is ours: militia short sword", self.q("SELECT notes FROM quests WHERE questid = 100184")[0][0])
        db, p = self.plan()
        self.assertEqual((p.made, p.fills, p.problems), ([], [], []), "a second plan has nothing to do")
        db.close()
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("BEGIN IMMEDIATE")
        fp.undo(db)
        db.execute("COMMIT")
        db.close()
        self.assertEqual(dump(self.path), self.before, "undo puts every byte back")

    def test_a_set_reward_is_a_problem(self):
        sc.REVIEW[100550] = (8571, "Freeport's")
        db, p = self.plan()
        self.assertIn("quest 100550: its last step gives an item already: left", p.problems)
        db.close()

    def test_a_pick_not_of_the_class_is_a_problem(self):
        sc.REVIEW[100184] = (8583, "slippers for a warrior")
        db, p = self.plan()
        self.assertTrue(any("is not the warrior's" in x for x in p.problems), p.problems)
        db.close()

    def test_a_race_missing_is_a_problem(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO defaultcharacter VALUES (11, 9, 0, 0, 0)")  # an ogre necromancer: 271 lacks ogres
        db.commit()
        db.close()
        db, p = self.plan()
        self.assertTrue(any("misses races 512" in x for x in p.problems), p.problems)
        db.close()

    def test_records_only(self):
        n = self.apply(records_only=True)
        self.assertEqual((n["made"], n["quests"]), (1, 0))

    def test_every_review_quest_is_picked(self):
        sc.REVIEW[100999] = (None, "no record")
        with self.assertRaises(SystemExit):
            fp.picks()

    def test_log_columns_are_not_named_like_secrets(self):
        self.apply()
        cols = [r[1] for r in self.q(f"PRAGMA table_info({fp.LOG})")]
        self.assertEqual(cols, ["seq", "tbl", "rid", "col", "old", "new"])
        for name in cols + [fp.LOG]:
            for stem in SECRET_STEMS:
                self.assertNotIn(stem, name.lower())

    def test_write_data(self):
        quest = {"key": "1-20/fayspire/wizard/L7", "name": "The Wasp of Kara",
                 "steps": [{"kind": "give", "rewards": {"xp": 157474, "items": ["Burning Flare"]}}], "notes": "Ours."}
        ddir = os.path.join(self.dir, "data")
        os.mkdir(ddir)
        with open(os.path.join(ddir, "fayspire.json"), "w") as f:
            f.write(json.dumps({"city": "fayspire", "quests": [quest], "items": []}, indent=1) + "\n")
        db, p = self.plan()
        fp.write_data(ddir, type("P", (), {"fills": [x for x in p.fills if x["key"] == quest["key"]], "rec": p.rec})())
        text = read(os.path.join(ddir, "fayspire.json"))
        d = json.loads(text)
        self.assertEqual(json.dumps(d, indent=1) + "\n", text, "the file's own layout")
        self.assertEqual(d["quests"][0]["steps"][-1]["rewards"]["items"], ["Burning Flare", "fancy slippers"])
        db.close()

    def test_cli_report(self):
        out = subprocess.run([sys.executable, os.path.join(HERE, "fill_class_quest_picks.py"), "--db", self.path],
                             capture_output=True, text=True)
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertIn("report only", out.stdout)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")


if __name__ == "__main__":
    unittest.main()
