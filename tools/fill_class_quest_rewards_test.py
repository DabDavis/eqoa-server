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

SCROLL = fc.SCROLL_ICON

# a few stems of internal/database/snapshot_names.go's secretStems: a log column named like a secret
# makes every database snapshot refuse
SECRET_STEMS = ["passw", "pwd", "secret", "token", "hash", "salt", "key", "ticket", "cookie", "session", "credential",
                "nonce", "oauth", "mail", "addr", "login", "phone", "birth", "device", "install", "private"]


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


def dump(path):
    """The database as SQL, but for sqlite_sequence (AUTOINCREMENT's high-water mark, which a delete
    never lowers: the server's own deletes leave it too)."""
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


# (key, questid, name, giver, [steps: (kind, spells, rewarditems)], notes)
QUESTS = [
    # shadow knight level 3: three cities teach Malice, Neriak and Grobb nothing
    ("1-20/freeport/shadowknight/L3", 100143, "Snake Scales", "Teacher F", [("loot", "", ""), ("give", "1000", "")], ""),
    ("1-20/highbourne/shadowknight/L3", 100279, "Bat Wings", "Teacher H", [("give", "1000", "")], ""),
    ("1-20/oggok/shadowknight/L3", 100519, "Rat Fur", "Teacher O", [("give", "1000", "")], ""),
    ("1-20/neriak-nektulos/shadowknight/L3", 100479, "Proving Yourself", "Sythonis U`Dedne",
     [("loot", "", ""), ("give", "", "")], "The guide names no reward but the experience."),
    ("1-20/grobb/shadowknight/L3", 100163, "Froglok Fishing Nets", "Teacher G", [("give", "", "")], ""),
    # level 5: Highbourne teaches Harm Touch, Neriak's reward is set (the scroll), Freeport's guide
    # says Heavy Arms (an enchanter's spell), Grobb and Oggok nothing
    ("1-20/freeport/shadowknight/L5", 100145, "The Highwaymen", "Teacher F", [("give", "1002", "")], ""),
    ("1-20/highbourne/shadowknight/L5", 100281, "Harm", "Teacher H", [("give", "1001", "")], ""),
    ("1-20/neriak-nektulos/shadowknight/L5", 100481, "The Fire Bats", "Sythonis U`Dedne", [("give", "", "8554")], ""),
    ("1-20/grobb/shadowknight/L5", 100165, "The Casting Stone", "Teacher G", [("give", "", "")], ""),
    ("1-20/oggok/shadowknight/L5", 100521, "The Lizardman Outcast", "Teacher O", [("give", "", "")], ""),
    # paladin level 3: no city names a reward
    ("1-20/fayspire/paladin/L3", 100043, "Slivers", "Teacher P", [("give", "", "")], ""),
    ("1-20/qeynos/paladin/L3", 100579, "Slivers", "Teacher Q", [("give", "", "")], ""),
    # warrior level 5: Taunt, which has no warrior scroll
    ("1-20/freeport/warrior/L5", 100065, "The Highwaymen", "Teacher W", [("give", "1003", "")], ""),
    ("1-20/grobb/warrior/L5", 100185, "The Casting Stone", "Teacher WG", [("give", "", "")], ""),
    # warrior level 20: Freeport kept one path; Halas has nothing: the guides' choice of two
    ("1-20/freeport/warrior/L20", 100070, "The Grimoire", "Teacher W", [("give", "1004", "9000")], ""),
    ("1-20/halas/warrior/L20", 100200, "The River Road", "Teacher WH", [("talk", "", "")], ""),
    # cleric level 10: Freeport's Sturdy Mace; Qeynos names nothing; Highbourne's guide names an item we lack
    ("1-20/freeport/cleric/L10", 100097, "The Mace", "Teacher C", [("give", "", "9001")], ""),
    ("1-20/qeynos/cleric/L10", 100563, "Defender Morion", "Teacher CQ", [("give", "", "")], ""),
    ("1-20/highbourne/cleric/L10", 100237, "Rory's Ring", "Teacher CH", [("give", "", "")],
     'Not on this server, left out of the rewards: the item "Ironsun Morningstar".'),
]

SPELLS = [  # spellid, name, level, [(class, source)]
    (900, "Malice", 3, []),            # a twin of 1000 numbered first (Return Home's): scribe.go takes it, at its level
    (1000, "Malice", 3, [(3, "dump")]),
    (1001, "Harm Touch", 5, [(3, "dump")]),
    (1002, "Heavy Arms", 5, [(12, "dump")]),
    (1003, "Taunt", 5, [(0, "dump")]),
    (1004, "Bellow", 20, [(0, "eqoa-data")]),
    (1005, "Pillar of Might", 20, [(0, "eqoa-data")]),
    (1006, "Hero Guard", 3, [(2, "dump")]),
]

ITEMS = [  # patternid, name, icon, slot, level, classuse, raceuse, maxhp, desc
    (8553, "Malice", SCROLL, -1, 3, 8, 1023, 0, "This allows you to scribe Malice into your spell book."),
    (8554, "Harm Touch", SCROLL, -1, 5, 8, 1023, 0, "This allows you to scribe Harm Touch into your spell book."),
    (8564, "Taunt (SK/PAL)", SCROLL, -1, 8, 12, 1023, 0, "A paladin's and shadow knight's taunt."),
    (8539, "Hero Guard Scroll", SCROLL, -1, 3, 4, 1023, 0, "Not a name scribe.go reads as a spell's."),
    (8568, "Bellow", SCROLL, -1, 20, 1, 1023, 0, "Bellow."),
    (8569, "Pillar of Might", SCROLL, -1, 20, 1, 1023, 0, "Pillar of Might."),
    (9000, "ensorcelled greataxe", 11, 15, 20, 1, 1023, 900, "An axe."),
    (9001, "Sturdy Mace", 12, 12, 10, 512, 1023, 700, "A mace."),
]


class FillClassQuestRewards(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "eqoa.db")
        db = sqlite3.connect(self.path)
        db.executescript("""
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER, classes TEXT, giver TEXT,
                                 notes TEXT NOT NULL DEFAULT '');
            CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, spells TEXT NOT NULL DEFAULT '',
                                     rewarditems TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '',
                                     choices TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '');
            CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER, file TEXT, digest TEXT, enabled INTEGER);
            CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
            CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT, PRIMARY KEY (class, spellid));
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT, itemicon INTEGER,
                                      equipslot INTEGER, levelreq INTEGER, classuse INTEGER, raceuse INTEGER,
                                      maxhp INTEGER, itemdesc TEXT);
            CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL);
            CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, patternid INTEGER);
            CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, tclass INTEGER);
            CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, count INTEGER DEFAULT 0,
                                     done INTEGER, PRIMARY KEY (serverid, questid));
            CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, stackleft INTEGER,
                                        remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER,
                                        location INTEGER, listnumber INTEGER);
            CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER, addedorder INTEGER);
        """)
        for key, qid, name, giver, steps, notes in QUESTS:
            cls = fc.CLASSES.index(key.split("/")[2])
            db.execute("INSERT INTO quests VALUES (?, ?, 1, ?, ?, ?)", (qid, name, str(cls), giver, notes))
            db.execute("INSERT INTO quest_import_quests VALUES (?, ?, ?, '', 1)", (key, qid, key.split("/")[1] + ".json"))
            for i, (kind, sp, ri) in enumerate(steps, 1):
                db.execute("INSERT INTO queststeps (questid, step, kind, spells, rewarditems) VALUES (?, ?, ?, ?, ?)",
                           (qid, i, kind, sp, ri))
        for sid, nm, lv, cls in SPELLS:
            db.execute("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, nm, lv))
            for c, src in cls:
                db.execute("INSERT INTO classspells VALUES (?, ?, ?)", (c, sid, src))
        for it in ITEMS:
            db.execute("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", it)
        db.executemany("INSERT INTO characters VALUES (?, ?, ?)",
                       [(103, "Dust", 3), (104, "Knows", 3), (105, "Busy", 3), (106, "Tester", 0), (107, "Warry", 0)])
        db.executemany("INSERT INTO charquests (serverid, questid, step, done) VALUES (?, ?, ?, ?)", [
            (103, 100479, 0, 1),   # Dust finished Neriak's "Proving Yourself": owed the Malice scroll
            (104, 100163, 0, 1),   # finished Grobb's, and has Malice in his book: owed nothing
            (105, 100521, 1, 0),   # under way: owed nothing
            (106, 100479, 0, 1),   # a warrior a GM marked done on a shadow knight's quest: not given
            (107, 100200, 0, 1),   # finished a level-20 quest that becomes a choice: the pick is his, not given
        ])
        db.execute("INSERT INTO spells (serverid, spellid, addedorder) VALUES (104, 1000, 3)")
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                   "listnumber) VALUES (103, 1, 200, 0, 9001, 0, 0, 15)")
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, os.path.join(HERE, "fill_class_quest_rewards.py"), "--db", self.path, *args],
                           capture_output=True, text=True)
        self.assertEqual(p.returncode, 0, p.stderr)
        return p.stdout

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_report_apply_undo(self):
        out = self.run_tool()
        self.assertIn("report only", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")

        out = self.run_tool("--apply")
        self.assertIn("wrote 6 rewards, 1 given", out)
        rw = lambda qid: self.q("SELECT rewarditems FROM queststeps WHERE questid = ? ORDER BY step DESC LIMIT 1", qid)[0][0]
        # Dust's check case: Neriak's "Proving Yourself" gives the Malice scroll, and Dust has his
        self.assertEqual(rw(100479), "8553")
        self.assertEqual(self.q("SELECT stackleft, remainHP, location, listnumber, equiploc FROM charinventory "
                                "WHERE serverid = 103 AND patternid = 8553"), [(1, 0, 0, 16, -1)],
                         "given as a quest gives: one, at the inventory's next place, unworn")
        self.assertEqual(rw(100163), "8553", "Grobb's too")
        self.assertEqual(self.q("SELECT count(*) FROM charinventory WHERE serverid = 104")[0][0], 0, "Malice in his book: nothing")
        self.assertEqual(self.q("SELECT count(*) FROM charinventory WHERE serverid IN (105, 106, 107)")[0][0], 0,
                         "under way, of another class, or a choice: nothing")
        # level 5: Harm Touch, the most of the cities' own spells; Freeport's Heavy Arms not counted
        self.assertEqual((rw(100165), rw(100521)), ("8554", "8554"))
        self.assertIn("heavy arms (freeport, not a spell of the class's)", out)
        # controls: a set reward, and a quest that teaches another class's spell, are left
        self.assertEqual(rw(100481), "8554", "Neriak's Fire Bats keeps its own reward")
        self.assertEqual(self.q("SELECT spells, rewarditems FROM queststeps WHERE questid = 100145"), [("1002", "")])
        # control: a class with no other city's reward is left
        self.assertEqual((rw(100043), rw(100579)), ("", ""))
        # control: a spell with no scroll the class can scribe is reported, not filled
        self.assertEqual(rw(100185), "")
        self.assertIn("NOT FILLED (no scroll): warrior L5 grobb", out)
        # the level-20 choice of the guides' two paths
        ch = json.loads(self.q("SELECT choices FROM queststeps WHERE questid = 100200")[0][0])
        self.assertEqual(ch, {"ask": "Which do you take?", "options": [
            {"answer": "Bellow", "says": "Then it is yours.", "items": "8568"},
            {"answer": "Pillar of Might", "says": "Then it is yours.", "items": "8569"}]})
        self.assertEqual(rw(100200), "", "the choice is the reward, not the step's own")
        # an item as it is; the control: a guide that names an item this server lacks gets none in its place
        self.assertEqual(rw(100563), "9001")
        self.assertEqual(rw(100237), "")
        self.assertIn("the guide names 'Ironsun Morningstar'", out)
        # the notes say the reward is ours
        notes = self.q("SELECT notes FROM quests WHERE questid = 100479")[0][0]
        self.assertTrue(notes.startswith("The guide names no reward but the experience. The reward is ours: the Malice scroll"), notes)
        # a second run finds nothing more to do
        self.assertIn("0 quests to fill", self.run_tool())

        out = self.run_tool("--undo")
        self.assertIn("left 0 changed since", out)
        self.assertEqual(dump(self.path), self.before, "undo puts every byte back")

    def test_undo_leaves_a_later_change(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE queststeps SET rewarditems = '9001' WHERE questid = 100163")  # a GM's change since
        db.commit()
        db.close()
        self.assertIn("left 1 changed since", self.run_tool("--undo"))
        self.assertEqual(self.q("SELECT rewarditems FROM queststeps WHERE questid = 100163")[0][0], "9001")

    def test_no_items(self):
        self.run_tool("--apply", "--no-items")
        self.assertEqual(self.q("SELECT rewarditems FROM queststeps WHERE questid = 100563")[0][0], "")
        self.assertEqual(self.q("SELECT rewarditems FROM queststeps WHERE questid = 100479 AND step = 2")[0][0], "8553")

    def test_scroll_rules(self):
        db = sqlite3.connect(self.path)
        rec = fc.Records(db)
        self.assertEqual(rec.spell_of_scroll(8553), 900, "scribe.go's scrollSpell: the first record of the name at its level")
        self.assertEqual(rec.scrolls_for("malice", 3, 3)[0], [8553], "a twin at the class's level is the class's spell")
        self.assertEqual(rec.scrolls_for("taunt", 0, 5)[0], [], "another class's scroll is not one")
        self.assertEqual(rec.scrolls_for("malice", 3, 2)[0], [], "nor one above the quest's level")
        self.assertIsNone(rec.spell_of_scroll(8539), "'Hero Guard Scroll' names no spell: scribe.go cannot scribe it")
        db.close()

    def test_log_columns_are_not_named_like_secrets(self):
        self.run_tool("--apply")
        cols = [r[1] for r in self.q(f"PRAGMA table_info({fc.LOG})")]
        self.assertEqual(cols, ["seq", "tbl", "rid", "col", "old", "new"])
        for name in cols + [fc.LOG]:
            for stem in SECRET_STEMS:
                self.assertNotIn(stem, name.lower(), f"{name!r} reads like a secret: every snapshot would refuse")

    def test_write_data_keeps_the_layout(self):
        data = {"city": "neriak-nektulos", "quests": [
            {"key": "1-20/neriak-nektulos/shadowknight/L3", "name": "Proving Yourself", "steps": [
                {"kind": "loot", "target": "grass snake"}, {"kind": "give", "rewards": {"xp": 6900}}],
             "unresolved": [], "notes": "The guide names no reward but the experience."},
            {"key": "1-20/neriak-nektulos/shadowknight/L5", "name": "The Fire Bats", "steps": [
                {"kind": "give", "rewards": {"xp": 36500, "items": ["Harm Touch"]}}], "unresolved": [], "notes": "Set."}],
            "items": []}
        halas = {"city": "halas", "quests": [{"key": "1-20/halas/warrior/L20", "name": "The River Road",
                                              "steps": [{"kind": "talk", "rewards": {"xp": 2814929}}], "notes": ""}]}
        ddir = os.path.join(self.dir, "data")
        os.mkdir(ddir)
        for fn, d in (("neriak-nektulos.json", data), ("halas.json", halas)):
            with open(os.path.join(ddir, fn), "w") as f:
                f.write(json.dumps(d, indent=1) + "\n")
        before = read(os.path.join(ddir, "neriak-nektulos.json"))
        db = sqlite3.connect(self.path)
        _, fills, _ = fc.plan(db)
        db.close()
        fc.write_data(ddir, [f for f in fills if f["file"] in ("neriak-nektulos.json", "halas.json")])
        after = read(os.path.join(ddir, "neriak-nektulos.json"))
        d = json.loads(after)
        self.assertEqual(json.dumps(d, indent=1) + "\n", after, "the file's own layout")
        self.assertEqual(d["quests"][0]["steps"][1]["rewards"], {"xp": 6900, "items": ["Malice"]})
        self.assertIn("The reward is ours: the Malice scroll", d["quests"][0]["notes"])
        self.assertEqual(d["quests"][1], data["quests"][1], "a quest with its own reward is left")
        changed = [l for l in after.splitlines() if l not in before.splitlines()]
        self.assertLessEqual(len(changed), 5, changed)
        h = json.loads(read(os.path.join(ddir, "halas.json")))
        last = h["quests"][0]["steps"][-1]
        self.assertEqual(last["choose"], "Which do you take?")
        self.assertEqual([o["rewards"]["items"] for o in last["choices"]], [["Bellow"], ["Pillar of Might"]])


if __name__ == "__main__":
    unittest.main()
