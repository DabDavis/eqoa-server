#!/usr/bin/env python3
"""tools/import_quests.py against a made-up database and content file
(tools/quests/testdata/sample.json): names resolved as a player reads them, a
quest item made from its like, a quest with a missing creature a draft and the
quest after it too, a run again a no-op, a GM's change kept, and --undo exact.
No game data.

    python3 tools/import_quests_test.py
"""
import hashlib
import json
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_quests as iq  # noqa: E402

SAMPLE = os.path.join(HERE, "quests", "testdata", "sample.json")

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL, itemicon INTEGER NOT NULL,
    trade INTEGER NOT NULL, maxstack INTEGER NOT NULL, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL, model INTEGER NOT NULL);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, spellname TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
    patternid INTEGER NOT NULL);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL);
"""


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO npcs (npc_name, world) VALUES (?, 0)",
                   [("Captain Brindle",), ("Field Boar",), ("Field Boar",), ("field boar",)])
    db.executemany("INSERT INTO itempattern (itemcost, itemicon, trade, maxstack, itemname, itemdesc, model) VALUES (?, ?, 0, ?, ?, ?, ?)",
                   [(5, 111, 20, "broken fang", "A fang.", 7), (30, 222, 1, "Old Spear", "A spear.", 9)])
    db.executemany("INSERT INTO spellpattern (spellname) VALUES (?)", [("Swipe",), ("Swipe",)])  # 1 a monk's, 2 a warrior's
    db.executemany("INSERT INTO classspells VALUES (?, ?, 'test')", [(4, 1), (0, 2)])
    db.execute("INSERT INTO spelleffect VALUES (2, '', ?, 'test')", (0x1DAEB5B6,))
    db.execute("INSERT INTO factions VALUES ('Testmoor Watch', 0)")
    return db


def state(db):
    """Every table's rows (the quest tables' columns the server would have
    anyway left out) and the item counter."""
    h = hashlib.md5()
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"):
        cols = [c for c in iq.columns(db, t) if c not in ("events", "chance")]
        for r in db.execute(f"SELECT {', '.join(cols)} FROM {t} ORDER BY 1"):
            h.update(repr((t, r)).encode())
    h.update(repr(db.execute("SELECT * FROM sqlite_sequence WHERE name = 'itempattern'").fetchall()).encode())
    return h.hexdigest()


class ImportQuests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        self.db = make_db(self.path)
        self.files = iq.load_files([SAMPLE])

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def quest(self, key):
        qid = self.db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (key,)).fetchone()[0]
        q = dict(zip(iq.QUEST_COLS, self.db.execute(f"SELECT {', '.join(iq.QUEST_COLS)} FROM quests WHERE questid = ?", (qid,)).fetchone()))
        steps = [dict(zip(iq.STEP_COLS, r)) for r in
                 self.db.execute(f"SELECT {', '.join(iq.STEP_COLS)} FROM queststeps WHERE questid = ? ORDER BY step", (qid,))]
        return q, steps

    def test_a_whole_quest_resolves(self):
        p = iq.plan(self.db, self.files)
        self.assertEqual([m["name"] for m in p["made"]], ["Boar's Tusk"])
        iq.write(self.db, p)
        tusk = self.db.execute("SELECT patternid, trade, itemicon, itemdesc FROM itempattern WHERE itemname = ?", ("Boar's Tusk",)).fetchone()
        self.assertEqual(tusk[1:], (1, 111, "A tusk from a field boar."), "made from its like, NO TRADE, its own words")
        q, steps = self.quest("1-20/testmoor/warrior/L1")
        self.assertEqual((q["enabled"], q["giver"], q["classes"], q["questid"]), (1, "Captain Brindle", "0", iq.FIRST_ID))
        self.assertEqual(json.loads(q["events"]), [{"on": "accept", "do": "popup", "text": "Welcome to Testmoor, %p."}])
        loot, give = steps
        self.assertEqual((loot["kind"], loot["target"], loot["items"], loot["count"], loot["chance"]),
                         ("loot", "Field Boar", f"{tusk[0]}*2", 2, 0.5), "the creature as this database spells it most")
        evs = json.loads(loot["events"])
        self.assertEqual(evs[0], {"on": "start", "do": "give", "items": "2"})
        self.assertEqual(evs[1], {"on": "done", "do": "visual", "fx": "0x1DAEB5B6"}, "a spell's visual by its name")
        self.assertEqual((give["items"], give["xp"], give["rewarditems"], give["factions"]),
                         (f"{tusk[0]}*2", 430, "2", "Testmoor Watch:+10"), "'boar's tusk' is the Boar's Tusk")
        self.assertEqual(give["spells"], "2", "the warrior's Swipe, not the monk's (the control: spell 1)")

    def test_a_missing_creature_makes_drafts_down_the_chain(self):
        iq.write(self.db, iq.plan(self.db, self.files))
        ghost, _ = self.quest("1-20/testmoor/warrior/L3")
        after, _ = self.quest("1-20/testmoor/warrior/L5")
        first, _ = self.quest("1-20/testmoor/warrior/L1")
        self.assertEqual((ghost["enabled"], after["enabled"], first["enabled"]), (0, 0, 1))
        self.assertIn("creature 'Moor Ghost'", ghost["notes"])
        self.assertIn("a draft", after["notes"])
        self.assertEqual(ghost["requires"], str(first["questid"]), "the chain by ids")

    def test_again_is_a_no_op_and_keeps_a_gms_change_and_undo_is_exact(self):
        before = state(self.db)
        written, kept, made = iq.write(self.db, iq.plan(self.db, self.files))
        self.assertEqual((written, kept, made), (3, [], 1))
        once = state(self.db)
        written, kept, made = iq.write(self.db, iq.plan(self.db, self.files))
        self.assertEqual((written, kept, made), (3, [], 0))
        self.assertEqual(state(self.db), once, "a run again changes nothing")
        q, _ = self.quest("1-20/testmoor/warrior/L1")
        self.db.execute("UPDATE quests SET offer = 'A GM wrote this.' WHERE questid = ?", (q["questid"],))
        written, kept, _ = iq.write(self.db, iq.plan(self.db, self.files))
        self.assertEqual(kept, ["1-20/testmoor/warrior/L1"])
        self.assertEqual(self.quest("1-20/testmoor/warrior/L1")[0]["offer"], "A GM wrote this.")
        # a character holding the made item and on the quest: undo takes both
        tusk = self.db.execute("SELECT patternid FROM itempattern WHERE itemname = ?", ("Boar's Tusk",)).fetchone()[0]
        self.db.execute("INSERT INTO charinventory (serverid, stackleft, patternid) VALUES (7, 2, ?)", (tusk,))
        self.db.execute("INSERT INTO charquests (serverid, questid, step) VALUES (7, ?, 1)", (q["questid"],))
        n = iq.undo(self.db)
        self.assertEqual((n["quests"], n["items"], n["progress"], n["held"]), (3, 1, 1, 1))
        self.assertEqual(state(self.db), before, "undo leaves the database as it was")


if __name__ == "__main__":
    unittest.main()
