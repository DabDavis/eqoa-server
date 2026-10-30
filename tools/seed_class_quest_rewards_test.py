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
import fill_class_quest_rewards as fc  # noqa: E402
import seed_class_quest_rewards as sc  # noqa: E402

SCROLL = fc.SCROLL_ICON
SECRET_STEMS = ["passw", "pwd", "secret", "token", "hash", "salt", "key", "ticket", "cookie", "session", "credential",
                "nonce", "oauth", "mail", "addr", "login", "phone", "birth", "device", "install", "private"]

# a small world: the tables patched to it
SCROLLS = [
    (707000, 0, 2295, "Taunt", 5),
    (707018, 6, 700021, "Sneak (RGE)", 3),
    (707013, 2, 700188, "Fortify", 20),
    (707014, 2, 700213, "Shining Bastion", 20),
    (707029, 5, 700225, "Sweeping Combat", 20),
]
NAMED = [
    (100360, 707100, 0, "Guard Sword", 8571, None, False, "Forged by Sergeant Grem from an oilstone.", "the warrior's sword"),
    (100640, 707121, 14, "Alchemist's Gloves", 6854, None, True, "Treated by Kelin from tailored gloves.", "its own record"),
]
SET = (100550, 707101, 0, "Guard's Sword", 8571, None, False, "Forged by Lancet Carroll.", "the control: a set reward")
EXISTING = {100573: (5831, "Small Stiletto", "the database's Small Stilletto")}
REVIEW = {100184: (8571, "Freeport's militia short sword")}

# (key, questid, name, [(kind, spells, rewarditems)], notes)
QUESTS = [
    ("1-20/freeport/warrior/L5", 100065, "The Highwaymen", [("give", "2295", "")], ""),
    ("1-20/grobb/warrior/L5", 100185, "The Casting Stone", [("loot", "", ""), ("give", "", "")], "No reward."),
    ("1-20/freeport/rogue/L3", 100133, "Snakes", [("give", "700021", "")], ""),
    ("1-20/tethelin/rogue/L3", 100739, "Tarantula Leg Tips", [("give", "", "")], ""),
    ("1-20/fayspire/paladin/L20", 100050, "Tillian Bracelets", [("talk", "", "")], ""),
    ("1-20/qeynos/paladin/L20", 100586, "The Dark Portals", [("give", "", "")], ""),
    # the bard: Freeport's one path imported; Qeynos's given one path by the first tool (logged)
    ("1-20/freeport/bard/L20", 100090, "The Grimoire", [("give", "", "8369, 8371")], ""),
    ("1-20/qeynos/bard/L20", 100616, "Crethley Manor", [("give", "", "8369, 8371")], "Ours already."),
    ("1-20/klick-anon/warrior/L4", 100360, "Grem's Sword", [("give", "", "")], ""),
    ("1-20/qeynos/alchemist/L4", 100640, "Alchemist's Gloves", [("give", "", "")], ""),
    ("1-20/qeynos/warrior/L4", 100550, "A Guard's Sword", [("give", "", "9999")], ""),
    ("1-20/qeynos/wizard/L10", 100573, "Storm's Pass", [("give", "", "")], ""),
    ("1-20/grobb/warrior/L4", 100184, "A Warrior Weapon", [("talk", "", "")], ""),
    ("1-20/freeport/warrior/L4", 100064, "New Gear", [("give", "", "8571")], ""),
]
SPELLS = [  # id, name, level, [(class, source)]
    (183, "Taunt", 5, []), (2295, "Taunt", 5, [(0, "dump")]),
    (700013, "Sneak", 1, [(1, "eqoa-data")]), (700021, "Sneak", 3, [(6, "eqoa-data")]),
    (700188, "Fortify", 20, [(2, "eqoa-data")]), (700213, "Shining Bastion", 20, [(2, "eqoa-data")]),
    (700203, "Power Dance", 20, [(5, "eqoa-data")]), (700225, "Sweeping Combat", 20, [(5, "eqoa-data")]),
]
ITEMS = [  # patternid, name, icon, slot, level, classuse, raceuse, maxhp, duration, damage, desc
    (8553, "Malice", SCROLL, -1, 3, 8, 1023, 0, 0, 0, "This allows you to scribe Malice into your spell book."),
    (8564, "Taunt (SK/PAL)", SCROLL, -1, 8, 12, 1023, 0, 0, 0, "Taunting."),
    (700002, "Sneak", SCROLL, -1, 1, 2, 3, 0, 0, 0, "The ranger's."),
    (8539, "Sneak Scroll", SCROLL, -1, 3, 64, 991, 0, 0, 0, "This allows you to scribe Sneak Scroll into your spell book."),
    (8369, "Power Dance", SCROLL, -1, 20, 32, 1023, 0, 0, 0, "Power Dance."),
    (8371, "magic foil", 11, 12, 20, 32, 1023, 4000, 50, 200, "A magical foil."),
    (8571, "militia short sword", 12, 12, 4, 1, 1023, 0, 0, 45, "A blank old record."),
    (6854, "Alchemist Gloves", 13, 19, 4, 0, 1023, 0, 0, 0, "Add a real description."),
    (5831, "Small Stilletto", 14, 12, 10, 8192, 1023, 2000, 50, 130, "A stilletto."),
    (9999, "a set reward", 15, 12, 4, 1, 1023, 800, 50, 40, "Set."),
]


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


class SeedClassQuestRewards(unittest.TestCase):
    def setUp(self):
        self.saved = (sc.SCROLLS, sc.NAMED, sc.EXISTING, sc.REVIEW)
        sc.SCROLLS, sc.NAMED, sc.EXISTING, sc.REVIEW = SCROLLS, NAMED, EXISTING, REVIEW
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "eqoa.db")
        db = sqlite3.connect(self.path)
        db.executescript("""
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER, classes TEXT, giver TEXT,
                                 notes TEXT NOT NULL DEFAULT '');
            CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT NOT NULL DEFAULT '',
                                     items TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '',
                                     rewarditems TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '',
                                     choices TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '');
            CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER, file TEXT, digest TEXT, enabled INTEGER);
            CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
            CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT, PRIMARY KEY (class, spellid));
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT, itemicon INTEGER,
                                      equipslot INTEGER, levelreq INTEGER, classuse INTEGER, raceuse INTEGER, maxhp INTEGER,
                                      duration INTEGER, weapondamage INTEGER, attacktype INTEGER DEFAULT 0, model INTEGER DEFAULT 0,
                                      itemdesc TEXT);
            CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL);
            CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, patternid INTEGER);
            CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, tclass INTEGER);
            CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER, PRIMARY KEY (serverid, questid));
            CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, stackleft INTEGER,
                                        remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER,
                                        location INTEGER, listnumber INTEGER);
            CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER, addedorder INTEGER);
        """)
        for key, qid, name, steps, notes in QUESTS:
            cls = fc.CLASSES.index(key.split("/")[2])
            db.execute("INSERT INTO quests VALUES (?, ?, 1, ?, 'Giver', ?)", (qid, name, str(cls), notes))
            db.execute("INSERT INTO quest_import_quests VALUES (?, ?, ?, '', 1)", (key, qid, key.split("/")[1] + ".json"))
            for i, (kind, sp, ri) in enumerate(steps, 1):
                db.execute("INSERT INTO queststeps (questid, step, kind, spells, rewarditems) VALUES (?, ?, ?, ?, ?)",
                           (qid, i, kind, sp, ri))
        for sid, nm, lv, cls in SPELLS:
            db.execute("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, nm, lv))
            for c, src in cls:
                db.execute("INSERT INTO classspells VALUES (?, ?, ?)", (c, sid, src))
        for pid, nm, icon, slot, lv, cu, ru, hp, dur, dmg, desc in ITEMS:
            db.execute("INSERT INTO itempattern (patternid, itemname, itemicon, equipslot, levelreq, classuse, raceuse, maxhp, "
                       "duration, weapondamage, itemdesc) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                       (pid, nm, icon, slot, lv, cu, ru, hp, dur, dmg, desc))
        # the first tool's log: it gave Qeynos's bard its one path (Freeport's it did not)
        db.execute(f"CREATE TABLE {fc.LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, rid TEXT, col TEXT, old TEXT, new TEXT)")
        db.execute(f"INSERT INTO {fc.LOG} (tbl, rid, col, old, new) VALUES ('queststeps', '100616/1', 'rewarditems', ?, ?)",
                   (json.dumps(""), json.dumps("8369, 8371")))
        db.executemany("INSERT INTO characters VALUES (?, ?, ?)",
                       [(201, "Grobbo", 0), (202, "Knows", 0), (203, "Busy", 0), (204, "Pally", 2), (205, "Sly", 6)])
        db.executemany("INSERT INTO charquests VALUES (?, ?, ?, ?)", [
            (201, 100185, 0, 1),   # finished Grobb's warrior L5: owed the new Taunt scroll
            (202, 100185, 0, 1),   # finished, and knows Taunt: owed nothing
            (203, 100185, 1, 0),   # under way: owed nothing
            (204, 100586, 0, 1),   # finished a level-20 quest that becomes a choice: his to pick
            (205, 100739, 0, 1),   # finished Tethelin's rogue L3: owed Sneak (RGE)
        ])
        db.execute("INSERT INTO spells (serverid, spellid, addedorder) VALUES (202, 2295, 4)")
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        sc.SCROLLS, sc.NAMED, sc.EXISTING, sc.REVIEW = self.saved
        shutil.rmtree(self.dir)

    def connect(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        return db

    def plan(self, records_only=False):
        db = self.connect()
        p = sc.Plan(db)
        p.build(records_only=records_only)
        return db, p

    def apply(self, records_only=False):
        db, p = self.plan(records_only)
        self.assertEqual(p.problems, [])
        db.execute("BEGIN IMMEDIATE")
        n = sc.apply(db, p)
        db.execute("COMMIT")
        db.close()
        return n, p

    def undo(self):
        db = self.connect()
        db.execute("BEGIN IMMEDIATE")
        sc.undo(db)
        db.execute("COMMIT")
        db.close()

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def rw(self, qid):
        return self.q("SELECT rewarditems, choices FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1", qid)[0]

    def test_apply_and_undo(self):
        n, p = self.apply()
        self.assertEqual((n["scrolls"], n["items"], n["given"], n["choice"]), (5, 2, 2, 1))
        # the new scrolls: Malice's record, the spell's name and line, the class, the level
        r = self.q("SELECT itemname, itemdesc, itemicon, equipslot, levelreq, classuse, raceuse FROM itempattern WHERE patternid = 707000")[0]
        self.assertEqual(r, ("Taunt", "This allows you to scribe Taunt into your spell book.", SCROLL, -1, 5, 1, 1023))
        self.assertEqual(self.q("SELECT itemname FROM itempattern WHERE patternid = 707018")[0][0], "Sneak (RGE)",
                         "another item has the spell's name: the class's tag")
        # the quests filled: a scroll each, the level-20 choices
        self.assertEqual(self.rw(100185), ("707000", ""))
        self.assertEqual(self.rw(100739), ("707018", ""))
        ch = json.loads(self.rw(100586)[1])
        self.assertEqual([o["items"] for o in ch["options"]], ["707013", "707014"])
        # the bard's one path from the first tool moved into the choice, its foil kept
        rew, ch = self.rw(100616)
        self.assertEqual(rew, "8371")
        self.assertEqual([o["items"] for o in json.loads(ch)["options"]], ["8369", "707029"])
        # control: Freeport's one path, the import's (not the first tool's), is left
        self.assertEqual(self.rw(100090), ("8369, 8371", ""))
        # the guides' items: a copy under the guide's name, its line, blank durability filled, the class set
        self.assertEqual(self.q("SELECT itemname, itemdesc, levelreq, classuse, maxhp, duration, weapondamage FROM itempattern "
                                "WHERE patternid = 707100")[0],
                         ("Guard Sword", "Forged by Sergeant Grem from an oilstone.", 4, 1, 800, 50, 45))
        self.assertEqual(self.q("SELECT classuse, maxhp, duration FROM itempattern WHERE patternid = 707121")[0], (16384, 800, 50))
        self.assertEqual(self.rw(100360), ("707100", ""))
        self.assertEqual(self.rw(100573), ("5831", ""), "a guide's item under another spelling: given as it is")
        # control: a quest NAMED does not name keeps its own reward
        self.assertEqual(self.rw(100550), ("9999", ""))
        # the finishers
        self.assertEqual(self.q("SELECT serverid, patternid FROM charinventory ORDER BY serverid"), [(201, 707000), (205, 707018)])
        # the notes say the reward is ours
        self.assertIn("The reward is ours: the Taunt scroll, made for it", self.q("SELECT notes FROM quests WHERE questid = 100185")[0][0])
        # a second plan finds nothing more to do
        db, p2 = self.plan()
        self.assertEqual((p2.made, p2.fills, p2.problems), ([], [], []))
        db.close()
        self.undo()
        self.assertEqual(dump(self.path), self.before, "undo puts every byte back")

    def test_undo_keeps_a_held_record(self):
        self.apply()
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (209, 1, 0, 0, 707100, -1, 0, 0)")  # given by the quest engine since
        db.commit()
        db.close()
        self.undo()
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE patternid = 707100")[0][0], 1, "someone holds it: kept")
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE patternid = 707000")[0][0], 0)

    def test_a_set_reward_is_a_problem(self):
        sc.NAMED = NAMED + [SET]
        db, p = self.plan()
        self.assertIn("quest 100550: its last step gives an item already: left", p.problems)
        self.assertNotIn(100550, [f["qid"] for f in p.fills])
        db.close()

    def test_records_only(self):
        n, _ = self.apply(records_only=True)
        self.assertEqual((n["scrolls"], n["items"], n["quests"]), (5, 2, 0))
        self.assertEqual(self.rw(100185), ("", ""))

    def test_refuses_an_id_another_record_holds(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO itempattern (patternid, itemname, itemicon, equipslot, levelreq, classuse, raceuse, maxhp, duration, "
                   "weapondamage, itemdesc) VALUES (707000, 'Someone Else', 0, 12, 1, 1, 1023, 0, 0, 0, '')")
        db.commit()
        db.close()
        db, p = self.plan()
        self.assertTrue(any("707000 is 'Someone Else'" in x for x in p.problems), p.problems)
        db.close()

    def test_scroll_twin_and_unscribable(self):
        db, p = self.plan()
        self.assertEqual(p.rec.spell_of_scroll(707000), 183, "scribe.go's first Taunt of level 5: Return Home's number")
        self.assertIsNone(p.rec.spell_of_scroll(8539), "'Sneak Scroll' names no spell")
        db.close()

    def test_log_columns_are_not_named_like_secrets(self):
        self.apply()
        cols = [r[1] for r in self.q(f"PRAGMA table_info({sc.LOG})")]
        self.assertEqual(cols, ["seq", "tbl", "rid", "col", "old", "new"])
        for name in cols + [sc.LOG]:
            for stem in SECRET_STEMS:
                self.assertNotIn(stem, name.lower())

    def test_write_data_and_review(self):
        bard = {"key": "1-20/qeynos/bard/L20", "name": "Crethley Manor",
                "steps": [{"kind": "give", "rewards": {"xp": 2814929, "items": ["Power Dance", "magic foil"]}}], "notes": "Ours."}
        wiz = {"key": "1-20/qeynos/wizard/L10", "name": "Storm's Pass", "steps": [{"kind": "give", "rewards": {"xp": 556753}}],
               "notes": ""}
        ddir = os.path.join(self.dir, "data")
        os.mkdir(ddir)
        with open(os.path.join(ddir, "qeynos.json"), "w") as f:
            f.write(json.dumps({"city": "qeynos", "quests": [bard, wiz], "items": []}, indent=1) + "\n")
        db, p = self.plan()
        keys = {bard["key"], wiz["key"]}
        sc.write_data(ddir, type("P", (), {"fills": [x for x in p.fills if x["key"] in keys], "rec": p.rec})())
        text = read(os.path.join(ddir, "qeynos.json"))
        d = json.loads(text)
        self.assertEqual(json.dumps(d, indent=1) + "\n", text, "the file's own layout")
        last = d["quests"][0]["steps"][-1]
        self.assertEqual(last["rewards"]["items"], ["magic foil"], "the moved path's scroll out of the step's own")
        self.assertEqual([o["rewards"]["items"] for o in last["choices"]], [["Power Dance"], ["Sweeping Combat"]])
        self.assertEqual(d["quests"][1]["steps"][-1]["rewards"]["items"], ["Small Stilletto"])
        out = os.path.join(self.dir, "review.md")
        self.assertEqual(sc.item_review(db, p, out), 1)
        self.assertIn("militia short sword", read(out))
        db.close()

    def test_cli_report(self):
        out = subprocess.run([sys.executable, os.path.join(HERE, "seed_class_quest_rewards.py"), "--db", self.path],
                             capture_output=True, text=True)
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertIn("report only", out.stdout)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")


if __name__ == "__main__":
    unittest.main()
