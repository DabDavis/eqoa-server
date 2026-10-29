#!/usr/bin/env python3
"""tools/quests/check_quests.py's checks of the later guides' fields
(factions_required, repeatable, unlocks, rewards.byclass) against a made-up
quest and a made-up database. No game data.

    python3 tools/quests/check_quests_test.py
"""
import copy
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import check_quests as cq  # noqa: E402

ROUTES = [("Waysender Ti`Thox", "Freeport"), ("Waysender Ti`Thox", "Grobb")]

GOOD = {
    "key": "Misc/testmoor-writ/p1", "name": "The Writ", "giver": "Captain Brindle", "classes": [0], "minlevel": 20,
    "factions_required": [["Testmoor Watch", 100], ["Lockjaw", -2000, -100]],
    "repeatable": True,
    "unlocks": [{"npc": "Waysender Ti`Thox", "to": "the grobb"}],
    "steps": [{"kind": "talk", "target": "Captain Brindle", "journal": "Talk.",
               "rewards": {"xp": 5, "factions": [["Testmoor Watch", 10]],
                           "byclass": {"0": {"items": ["Old Spear"], "spells": ["Swipe"], "xp": 50, "tunar": 3}}}}],
    "unresolved": [],
}


class Later(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        path = os.path.join(self.dir.name, "q.db")
        db = sqlite3.connect(path)
        db.executescript("""
            CREATE TABLE npcs (npc_name TEXT, npc_type INTEGER);
            CREATE TABLE itempattern (itemname TEXT);
            CREATE TABLE spellpattern (spellname TEXT);
            CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER);
            INSERT INTO npcs VALUES ('Captain Brindle', 128), ('Waysender Ti`Thox', 128);
            INSERT INTO itempattern VALUES ('Old Spear');
            INSERT INTO spellpattern VALUES ('Swipe');
            INSERT INTO factions VALUES ('Testmoor Watch', 0), ('Lockjaw', 0);
        """)
        db.commit()
        db.close()
        self.names = cq.Names(path)

    def tearDown(self):
        self.dir.cleanup()

    def check(self, q, names=True):
        ck = cq.Check(self.names if names else None, ROUTES)
        ck.quest("t.json", q, set())
        return ck

    def test_a_whole_quest_with_them_is_clean(self):
        ck = self.check(copy.deepcopy(GOOD))
        self.assertEqual((ck.errors, ck.warnings, ck.misses[GOOD["key"]]), ([], [], []))

    def test_their_shapes_are_checked(self):
        q = copy.deepcopy(GOOD)
        q["key"] = "Misc/the writ/p1"
        q["repeatable"] = 1
        q["factions_required"] = [["Testmoor Watch"], ["Lockjaw", 5, 1], ["Lockjaw", 2500], "Lockjaw"]
        q["unlocks"] = [{"npc": "Waysender Ti`Thox"}]
        q["steps"][0]["rewards"]["byclass"] = {"15": {}, "wizard": {"xp": 1}, "0": {"items": "Old Spear", "level": 3}}
        errs = "\n".join(self.check(q).errors)
        for want in ("the key has a space", "repeatable 1", "factions_required ['Testmoor Watch']",
                     "factions_required ['Lockjaw', 5, 1]", "factions_required ['Lockjaw', 2500]",
                     "factions_required 'Lockjaw'", "is not {\"npc\", \"to\"}", "'15' is no class id",
                     "'wizard' is no class id", "items is not a list of names", "gives ['level']"):
            self.assertIn(want, errs)

    def test_names_they_use_are_resolved(self):
        q = copy.deepcopy(GOOD)
        q["factions_required"][0][0] = "Moor Hermits"
        q["steps"][0]["rewards"]["factions"] = [["Bog Folk", 5]]
        q["steps"][0]["rewards"]["byclass"]["0"] = {"items": ["Gold Spear"], "spells": ["Smite"]}
        ck = self.check(q)
        self.assertEqual(sorted(ck.misses[q["key"]]), ["Bog Folk", "Gold Spear", "Moor Hermits", "Smite"])
        self.assertEqual(len(ck.errors), 4, "each missing name is an error until unresolved lists it")
        q["unresolved"] = ["Moor Hermits", "Bog Folk", "Gold Spear", "Smite"]
        self.assertEqual(self.check(q).errors, [], "the control: listed, no error")

    def test_a_choice_is_checked(self):
        q = copy.deepcopy(GOOD)
        q["steps"][0]["choices"] = [{"answer": "The spear.", "rewards": {"items": ["Old Spear"]}},
                                    {"answer": "The swipe.", "says": "Yours.", "rewards": {"spells": ["Swipe"], "xp": 5}},
                                    {"answer": "No.", "stay": True}]
        q["steps"][0]["choose"] = "Which?"
        ck = self.check(q)
        self.assertEqual((ck.errors, ck.misses[q["key"]]), ([], []), "the control")
        q["steps"][0]["choices"][0]["rewards"]["items"] = ["Gold Spear"]
        q["steps"][0]["choices"].append({"answer": "x" * 64, "level": 3, "rewards": {"items": "Old Spear"}})
        q["steps"][0]["choices"].append({"says": "nothing to answer"})
        ck = self.check(q)
        errs = "\n".join(ck.errors)
        for want in ("option 4: the answer is longer than 63", "has ['level']", "option 5: no answer", "'gold spear' does not resolve"):
            self.assertIn(want, errs)
        q["steps"][0]["kind"], q["steps"][0]["count"] = "kill", 1
        q["steps"][0]["choices"] = [{"answer": "a"}] * 7
        errs = "\n".join(self.check(q).errors)
        self.assertIn("a kill step asks no choice", errs)
        self.assertIn("7 choices: the client shows 6", errs)

    def test_an_unlock_to_no_route_or_no_npc_is_a_warning(self):
        q = copy.deepcopy(GOOD)
        q["unlocks"] = [{"npc": "Waysender Ti`Thox", "to": "Testmoor Isle"}, {"npc": "Ferryman Olaf", "to": "the Isle"}]
        ck = self.check(q)
        warned = "\n".join(ck.warnings)
        self.assertIn("'Testmoor Isle', which is no route of waysender.go yet (its routes: Freeport, Grobb)", warned)
        self.assertIn("no NPC is named 'Ferryman Olaf'", warned)
        self.assertIn("'the Isle', which is no route of waysender.go yet (it has no routes)", warned)
        self.assertEqual((ck.errors, ck.misses[q["key"]]), ([], []), "an unlock is no name the quest needs to be played")
        self.assertIn("no route", "\n".join(self.check(q, names=False).warnings), "routes checked without a database too")


if __name__ == "__main__":
    unittest.main()
