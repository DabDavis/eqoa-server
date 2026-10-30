#!/usr/bin/env python3
"""tools/fix_quest_chains.py against a made-up database and quest file: a token several classes
get from quests of their own required as a group (and the controls: an item a merchant sells, one
the quest's own step gives, one a required quest gives, a giver no character of the quest may
take), a give step that hands on an item a later step needs kept (the controls: an item given back,
enough given, an item a merchant sells), a repeatable quest's token kept, the city class quests'
races from defaultcharacter (the froglok with the eastern human), a GM-deleted quest NPC put back
and the panel's delete marked reverted (the controls: a deleted NPC no quest names, one already
put back); --apply then --undo leaves the dump as it was, a second --apply is refused, the import
digests follow, and the quest files take the same changes in their layout. No game data.

    python3 tools/fix_quest_chains_test.py
"""
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
import fix_quest_chains as fq  # noqa: E402
import import_quests as iq  # noqa: E402

SECRET_STEMS = ["passw", "pwd", "secret", "token", "hash", "salt", "key", "ticket", "cookie", "session", "credential",
                "nonce", "oauth", "mail", "addr", "login", "phone", "birth", "device", "install", "private"]

# (key, id, name, giver, classes, races, requires, repeatable, steps: [(kind, target, items, rewards, events)])
QUESTS = [
    ("60/healer/start-a", 10, "The Recruiter", "Recruiter", "9", "", "", 0, [("talk", "Ruddish", "", "500", "")]),
    ("60/healer/start-b", 11, "The Recruiter", "Recruiter", "8", "", "", 0, [("talk", "Ruddish", "", "500", "")]),
    ("60/healer/start-x", 12, "The Recruiter", "Recruiter", "0", "", "", 0, [("talk", "Ruddish", "", "500", "")]),
    ("60/healer/ep1", 13, "Wands", "Ruddish", "8, 9", "", "", 0,
     [("loot", "froglok", "501*2", "", ""), ("give", "Ruddish", "501*2, 500", "", "")]),
    ("60/healer/ep2", 14, "After the wands", "Ruddish", "8, 9", "", "13", 0, [("show", "Ruddish", "500", "", "")]),
    ("misc/buy", 15, "Bought", "Ruddish", "", "", "", 0, [("give", "Ruddish", "502", "", "")]),
    ("misc/own", 16, "Its own", "Ruddish", "", "", "", 0, [("talk", "Ruddish", "", "503", ""), ("give", "Kizdean", "503", "", "")]),
    ("misc/pass", 17, "Passage", "Magus", "", "", "", 1, [("give", "Magus", "504", "", "")]),
    ("misc/magus", 18, "The Magus", "Magus", "", "", "", 0, [("talk", "Magus", "", "504", "")]),
    ("misc/box", 19, "Packages", "Lady", "", "", "", 0,
     [("loot", "Valrean", "505", "", ""), ("give", "Lady", "505", "", ""), ("give", "Nalzik", "505", "", "")]),
    ("misc/back", 20, "Given back", "Lady", "", "", "", 0,
     [("loot", "Valrean", "506", "", ""), ("give", "Lady", "506", "506", ""), ("give", "Nalzik", "506", "", "")]),
    ("misc/gears", 21, "Gears", "Swame", "", "", "", 0,
     [("talk", "Swame", "", "507, 507, 507", ""), ("give", "Zarene", "507", "", ""), ("give", "Zale", "507*2", "", "")]),
    ("misc/bought-twice", 22, "Bought twice", "Lady", "", "", "", 0,
     [("give", "Lady", "502", "", ""), ("give", "Nalzik", "502", "", "")]),
    ("misc/stranger", 23, "The Tomb", "Nothard", "", "", "", 0, [("talk", "Stranger", "", "", "")]),
    ("1-20/freeport/warrior/L1", 24, "Freeport's first", "Guard", "0", "", "", 0, [("talk", "Guard", "", "", "")]),
    ("1-20/tethelin/ranger/L1", 25, "Tethelin's first", "Ranger", "1", "", "", 0, [("talk", "Ranger", "", "", "")]),
    ("1-20/qeynos/warrior/L1", 26, "Qeynos's own", "Guard", "0", "6", "", 0, [("talk", "Guard", "", "", "")]),
    ("misc/war", 27, "No city", "Guard", "0", "", "", 0, [("talk", "Guard", "", "", "")]),
]
ITEMS = [(500, "Ruddish's token", 1), (501, "froglok wand", 2), (502, "bread", 20), (503, "letter", 1),
         (504, "port token", 1), (505, "locked box", 1), (506, "red eye", 99), (507, "gear wheel", 10)]
NPCS = [(1, "Recruiter", 128), (2, "Ruddish", 128), (3, "Merchant Gale", 129), (4, "Kizdean", 128), (5, "Magus", 128),
        (6, "Lady", 128), (7, "Nalzik", 128), (8, "Nothard", 128), (9, "Guard", 128), (10, "Ranger", 128),
        (11, "froglok", 0), (12, "Valrean", 0), (13, "Swame", 128), (14, "Zarene", 128), (15, "Zale", 128)]
DEFAULTS = [(0, 0, 1), (0, 0, 2), (1, 0, 2), (1, 1, 0), (0, 5, 0), (9, 2, 0)]  # (class, race, humType)


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(db.iterdump())
    db.close()
    return out


def make(path):
    db = sqlite3.connect(path, isolation_level=None)
    iq.ensure_quest_tables(db)
    db.executescript("""
        CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
            PRIMARY KEY (questid, step, target));
        CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL,
            digest TEXT NOT NULL, enabled INTEGER NOT NULL);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT NOT NULL, maxstack INTEGER NOT NULL);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
            z_coord REAL, npc_type INTEGER);
        CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL);
        CREATE TABLE defaultcharacter (defaultCharacterID INTEGER PRIMARY KEY AUTOINCREMENT, tclass INTEGER NOT NULL,
            race INTEGER NOT NULL, humType INTEGER NOT NULL);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    for key, qid, name, giver, cls, races, req, rep, steps in QUESTS:
        db.execute("INSERT INTO quests (questid, name, enabled, giver, classes, races, requires, repeatable) "
                   "VALUES (?, ?, 1, ?, ?, ?, ?, ?)", (qid, name, giver, cls, races, req, rep))
        for i, (kind, target, its, rw, ev) in enumerate(steps, 1):
            db.execute("INSERT INTO queststeps (questid, step, kind, target, items, rewarditems, events, count) "
                       "VALUES (?, ?, ?, ?, ?, ?, ?, ?)", (qid, i, kind, target, its, rw, ev,
                                                          int(its.split("*")[1]) if kind == "loot" and "*" in its else 1))
        f = "cities.json" if key.startswith("1-20/") else "later.json"
        db.execute("INSERT INTO quest_import_quests VALUES (?, ?, ?, ?, 1)", (key, qid, f, iq.digest(db, qid)))
    db.executemany("INSERT INTO itempattern VALUES (?, ?, ?)", ITEMS)
    db.executemany("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, npc_type) VALUES (?, ?, 0, 1, 2, 3, ?)", NPCS)
    db.execute("INSERT INTO npcitem (npcid, patternid) VALUES (3, 502)")  # Merchant Gale sells bread
    db.executemany("INSERT INTO defaultcharacter (tclass, race, humType) VALUES (?, ?, ?)", DEFAULTS)
    stranger = {"id": 40, "npc_name": "Stranger", "world": 0, "x_coord": 25092.0, "y_coord": 75.6, "z_coord": 9479.3, "npc_type": 128}
    cat = dict(stranger, id=41, npc_name="the cat")
    back = dict(stranger, id=42, npc_name="Stranger")
    for i, row in ((1, stranger), (2, cat), (3, back)):
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, reverted_by) "
                   "VALUES ('2026-10-29T15:59:00Z', 8, 'gm', 'web', '', 'delete', 'npcs', ?, ?, ?)",
                   (json.dumps({"id": row["id"]}), json.dumps(row), 99 if i == 3 else None))
    # quest 27 a GM changed since its import: its digest not the import's
    db.execute("UPDATE quests SET notes = 'a GM was here' WHERE questid = 27")
    return db


def quest_file(qs):
    """The quest files as the import would have had them (their layout json.dump(indent=1)'s)."""
    def step(kind, target, its, rw, ev):
        st = {"kind": kind, "target": target}
        if its:
            st["items"] = [{"item": dict((p, n) for p, n, _ in ITEMS)[int(x.split("*")[0])],
                            "count": int(x.split("*")[1]) if "*" in x else 1} for x in its.split(", ")]
        return st
    out = {}
    for key, qid, name, giver, cls, races, req, rep, steps in qs:
        f = "cities.json" if key.startswith("1-20/") else "later.json"
        q = {"key": key, "name": name, "giver": giver, "classes": [int(c) for c in cls.split(", ") if c],
             "races": [int(r) for r in races.split(", ") if r], "requires": [], "steps": [step(*s) for s in steps],
             "notes": "Ünïcode kept."}
        out.setdefault(f, {"city": f[:-5], "quests": []})["quests"].append(q)
    return out


class FixQuestChains(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "eqoa.db")
        self.db = make(self.path)

    def tearDown(self):
        self.db.close()
        shutil.rmtree(self.dir)

    def plan(self):
        return fq.Plan(self.db).build()

    def test_a_token_from_each_classes_own_quest_is_a_group(self):
        p = self.plan()
        req = {r["qid"]: [a["group"] for a in r["add"]] for r in p.requires}
        self.assertEqual(req.get(13), [[10, 11]], "the cleric's and the shaman's recruiters, not the warrior's")
        self.assertNotIn(14, req, "the control: 13, which it requires, gives it now")
        self.assertNotIn(15, req, "the control: a merchant sells it")
        self.assertNotIn(16, req, "the control: its own step gives it")
        self.assertEqual(req.get(17), [[18]], "one giver: one quest")
        self.assertEqual(p.reports, [])

    def test_a_hand_on_is_shown_and_the_controls_are_left(self):
        p = self.plan()
        kept = {(k["qid"], k["step"]) for k in p.keeps}
        self.assertIn((19, 2), kept, "Lady's look at the box Nalzik is to open")
        self.assertIn((17, 1), kept, "a repeatable quest's token only a quest done once gives")
        for q in (20, 21, 22, 13):
            self.assertFalse(any(k[0] == q for k in kept), f"the control: quest {q}")

    def test_the_city_quests_take_the_races_that_start_there(self):
        p = self.plan()
        races = {r["qid"]: r["races"] for r in p.races}
        self.assertEqual(races, {24: [0, 10], 25: [1]}, "an eastern human's warrior and the froglok; a Tethelin elf ranger")
        self.assertEqual(p.table[("qeynos", 0)], [0])
        self.assertEqual(p.table[("surefall-glade", 1)], [0], "a western human ranger's city")
        self.assertEqual(p.table[("grobb", 0)], [5])

    def test_a_deleted_quest_npc_is_put_back(self):
        p = self.plan()
        self.assertEqual([(x["name"], x["audit"], x["quests"]) for x in p.npcs], [("Stranger", 1, [23])],
                         "the controls: the cat names no quest, the third delete is reverted already")

    def test_apply_undo_is_exact_and_the_digests_follow(self):
        before = dump(self.path)
        p = self.plan()
        self.db.execute("BEGIN IMMEDIATE")
        n = fq.apply(self.db, p, now="2026-10-30T09:00:00Z")
        self.db.execute("COMMIT")
        self.assertEqual((n["requires"], n["keeps"], n["races"], n["npcs"]), (2, 2, 2, 1))
        q13 = self.db.execute("SELECT requires FROM quests WHERE questid = 13").fetchone()[0]
        self.assertEqual(q13, "10|11")
        self.assertEqual(self.db.execute("SELECT kind FROM queststeps WHERE questid = 19 AND step = 2").fetchone()[0], "show")
        self.assertEqual(self.db.execute("SELECT races FROM quests WHERE questid = 24").fetchone()[0], "0, 10")
        self.assertEqual(self.db.execute("SELECT npc_name, y_coord FROM npcs WHERE id = 40").fetchone(), ("Stranger", 75.6))
        audit = self.db.execute("SELECT id, username, action, detail FROM gm_audit WHERE id = 4").fetchone()
        self.assertEqual(audit[1:3], ("dab's sessions", "revert"))
        self.assertEqual(self.db.execute("SELECT reverted_by FROM gm_audit WHERE id = 1").fetchone()[0], 4)
        # the import's digests: a quest it still owns follows; one a GM changed is left a GM's
        for key, qid, dg in self.db.execute("SELECT key, questid, digest FROM quest_import_quests").fetchall():
            if qid == 27:
                self.assertNotEqual(iq.digest(self.db, qid), dg, "the GM's quest stays the GM's")
            else:
                self.assertEqual(iq.digest(self.db, qid), dg, key)
        r = subprocess.run([sys.executable, os.path.join(HERE, "fix_quest_chains.py"), "--db", self.path, "--apply"],
                           capture_output=True, text=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("holds an earlier apply", r.stderr)
        self.db.execute("BEGIN IMMEDIATE")
        back, left = fq.undo(self.db)
        self.db.execute("COMMIT")
        self.assertEqual(left, [])
        self.assertEqual(dump(self.path), before, "the database as it was, its counters too")

    def test_undo_leaves_what_changed_since(self):
        p = self.plan()
        self.db.execute("BEGIN IMMEDIATE")
        fq.apply(self.db, p)
        self.db.execute("COMMIT")
        self.db.execute("UPDATE quests SET races = '0' WHERE questid = 24")  # a GM's edit since
        self.db.execute("BEGIN IMMEDIATE")
        back, left = fq.undo(self.db)
        self.db.execute("COMMIT")
        self.assertEqual(self.db.execute("SELECT races FROM quests WHERE questid = 24").fetchone()[0], "0")
        self.assertTrue(any("quests 24 races" in x for x in left), left)
        self.assertEqual(self.db.execute("SELECT races FROM quests WHERE questid = 25").fetchone()[0], "",
                         "the control: one not changed since is put back")

    def test_the_files_take_the_same_changes_in_their_layout(self):
        data = os.path.join(self.dir, "data")
        os.mkdir(data)
        files = quest_file(QUESTS)
        for f, doc in files.items():
            with open(os.path.join(data, f), "w", encoding="utf-8") as fh:
                fh.write(json.dumps(doc, indent=1, ensure_ascii=False) + "\n")
        with open(os.path.join(data, "later.json"), encoding="utf-8") as fh:
            old = fh.read()
        changed, problems = fq.write_data(data, self.plan())
        self.assertEqual(problems, [])
        self.assertEqual(dict(changed), {"later.json": 3, "cities.json": 2}, "13, 17 (both its changes) and 19; 24 and 25")
        with open(os.path.join(data, "later.json"), encoding="utf-8") as fh:
            new = fh.read()
        doc = json.loads(new)
        self.assertEqual(json.dumps(doc, indent=1, ensure_ascii=False) + "\n", new, "dumped as it was read")
        self.assertIn("Ünïcode kept.", new)
        by = {q["key"]: q for q in doc["quests"]}
        self.assertEqual(by["60/healer/ep1"]["requires"], [["60/healer/start-a", "60/healer/start-b"]])
        self.assertEqual(by["misc/pass"]["requires"], ["misc/magus"])
        self.assertEqual(by["misc/box"]["steps"][1]["kind"], "show")
        self.assertEqual(by["misc/back"], json.loads(old)["quests"][[q["key"] for q in json.loads(old)["quests"]].index("misc/back")],
                         "the control: a quest left as it was")
        # the control: a file not as json.dump writes it is left with a problem
        with open(os.path.join(data, "cities.json"), "w", encoding="utf-8") as fh:
            fh.write(json.dumps(files["cities.json"], indent=3))
        changed, problems = fq.write_data(data, self.plan())
        self.assertTrue(any("not laid out" in x for x in problems), problems)

    def test_no_column_is_named_like_a_secret(self):
        self.db.execute(fq.LOG_DDL)
        for (c,) in [(r[1],) for r in self.db.execute(f"PRAGMA table_info({fq.LOG})")]:
            self.assertFalse(any(s in c.lower() for s in SECRET_STEMS), c)


if __name__ == "__main__":
    unittest.main()
