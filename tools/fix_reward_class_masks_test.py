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
import fix_reward_class_masks as fm  # noqa: E402

SECRET_STEMS = ["passw", "pwd", "secret", "token", "hash", "salt", "key", "ticket", "cookie", "session", "credential",
                "nonce", "oauth", "mail", "addr", "login", "phone", "birth", "device", "install", "private"]

ITEMS = [  # id, name, slot, level, classuse
    (100, "Sturdy Two-Handed Sword", 15, 10, 0),   # a warrior quest's reward: WAR
    (101, "Leggings of Insight", 10, 15, 0),       # two classes' quests: ENC and SK
    (102, "Thulian Circlet", 1, 30, 0),            # control: a quest for every class gives it, and a class quest
    (103, "a masked sword", 12, 4, 1),             # control: a mask of its own
    (104, "a quest note", -1, 0, 0),               # control: not worn
    (105, "Seradwyn's Leggings", 10, 23, 0),       # a reward by class (ranger) alone: --all-rewards
    (106, "Lycaon Slasher", 12, 30, 0),            # a choice's: --all-rewards
    (107, "Prayer Beads", 4, 1, 0),                # a monk's, also in a loot table
    (108, "Rogue's Tunic", 5, 1, 0),               # a rogue's, held by a warrior
]
QUESTS = [  # questid, key, classes, [(step, rewarditems, byclass, choices, events)]
    (1, "1-20/freeport/warrior/L10", "0", [(5, "100", "", "", "")]),
    (2, "1-20/freeport/enchanter/L15", "12", [(7, "101", "", "", "")]),
    (3, "1-20/freeport/shadowknight/L15", "3", [(7, "101, 103*2", "", "", "")]),
    (4, "23-47/30/freeport", "", [(2, "102", "", "", "")]),
    (5, "30,40,49/group-5-evil/L30-a", "0, 8, 13, 14", [(3, "102", "", "", "")]),
    (6, "1-20/qeynos/monk/L1", "4", [(1, "107, 104", "", "", "")]),
    (7, "23-47/23/p1", "", [(4, "", json.dumps({"1": {"items": "105"}}), "", "")]),
    (8, "30,40,49/group-2-good/L30-warrior", "0", [(1, "", "", json.dumps({"options": [{"answer": "x", "items": "106"}]}), "")]),
    (9, "1-20/halas/rogue/L1", "6", [(1, "108", "", "", "")]),
]


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FixRewardClassMasks(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "eqoa.db")
        db = sqlite3.connect(self.path)
        db.executescript("""
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, levelreq INTEGER,
                                      classuse INTEGER);
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER, classes TEXT,
                                 events TEXT NOT NULL DEFAULT '');
            CREATE TABLE queststeps (questid INTEGER, step INTEGER, rewarditems TEXT NOT NULL DEFAULT '',
                                     byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '',
                                     events TEXT NOT NULL DEFAULT '');
            CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER);
            CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
            CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY, npcid INTEGER, patternid INTEGER);
            CREATE TABLE loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER);
            CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, tclass INTEGER);
            CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER);
        """)
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)", ITEMS)
        for qid, key, classes, steps in QUESTS:
            db.execute("INSERT INTO quests (questid, name, enabled, classes) VALUES (?, ?, 1, ?)", (qid, key, classes))
            db.execute("INSERT INTO quest_import_quests VALUES (?, ?)", (key, qid))
            for st, ri, bc, ch, ev in steps:
                db.execute("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?, ?)", (qid, st, ri, bc, ch, ev))
        # a give event on a step of the warrior quest gives the sword too (counted with it)
        db.execute("UPDATE queststeps SET events = ? WHERE questid = 1", (json.dumps([{"on": "done", "do": "give", "items": "100"}]),))
        db.execute("INSERT INTO npcs VALUES (1, 'Tailor McQuaid')")
        db.execute("INSERT INTO npcitem VALUES (1, 1, 108)")
        db.execute("INSERT INTO loottables VALUES (1, 'generic', 107)")
        db.execute("INSERT INTO characters VALUES (50, 'Warry', 0)")
        db.execute("INSERT INTO charinventory VALUES (1, 50, 108)")
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, os.path.join(HERE, "fix_reward_class_masks.py"), "--db", self.path, *args],
                           capture_output=True, text=True)
        self.assertEqual(p.returncode, 0, p.stderr)
        return p.stdout

    def mask(self, pid):
        db = sqlite3.connect(self.path)
        try:
            return db.execute("SELECT classuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()[0]
        finally:
            db.close()

    def test_apply_and_undo(self):
        out = self.run_tool()
        self.assertIn("report only", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("LEFT 102 'Thulian Circlet': a quest for every class gives it", out)
        self.assertIn("HELD by {'warrior': 1} of another class", out)
        self.assertIn("also in loot table generic", out)
        self.assertIn("2 more only a reward by class, a choice or a give event gives (--all-rewards)", out)
        out = self.run_tool("--apply")
        self.assertIn("gave 4 items their quests' class mask", out)
        self.assertEqual(self.mask(100), 1, "a warrior quest's: the warrior's")
        self.assertEqual(self.mask(101), 1 << 12 | 1 << 3, "two classes' quests: both")
        self.assertEqual(self.mask(107), 1 << 4)
        self.assertEqual(self.mask(108), 1 << 6, "held by a warrior: changed, and said")
        self.assertEqual(self.mask(102), 0, "control: a quest for every class gives it")
        self.assertEqual(self.mask(103), 1, "control: a mask of its own")
        self.assertEqual(self.mask(104), 0, "control: not worn")
        self.assertEqual((self.mask(105), self.mask(106)), (0, 0), "control: only by class or a choice, without --all-rewards")
        self.assertIn("0 items to give a class mask", self.run_tool(), "a second run has nothing to do")
        self.run_tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every byte back")

    def test_all_rewards(self):
        self.run_tool("--apply", "--all-rewards")
        self.assertEqual(self.mask(105), 1 << 1, "a reward by class: its class")
        self.assertEqual(self.mask(106), 1, "a choice: the quest's class")
        self.assertEqual(self.mask(102), 0, "control: still a quest for every class's")

    def test_undo_leaves_a_later_change(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET classuse = 3 WHERE patternid = 100")  # a GM's change since
        db.commit()
        db.close()
        self.assertIn("left 1 changed since", self.run_tool("--undo"))
        self.assertEqual(self.mask(100), 3)

    def test_log_columns_are_not_named_like_secrets(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        cols = [r[1] for r in db.execute(f"PRAGMA table_info({fm.LOG})")]
        db.close()
        self.assertEqual(cols, ["seq", "rid", "old", "new"])
        for name in cols + [fm.LOG]:
            for stem in SECRET_STEMS:
                self.assertNotIn(stem, name.lower())


if __name__ == "__main__":
    unittest.main()
