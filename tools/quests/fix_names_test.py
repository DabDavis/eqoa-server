#!/usr/bin/env python3
"""tools/quests/fix_names.py against made-up quest files and a made-up
database. No game data.

    python3 tools/quests/fix_names_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import check_quests as cq  # noqa: E402
import fix_names as fx  # noqa: E402

NPCS = [("Counselor Du`Klot", 128), ("Thick Coat Wolves", 0), ("Wolf Pup", 0), ("wolf pups", 0),
        ("Rabid Bat", 0), ("Bat", 0), ("Nobody", 128), ("Town Guard", 128), ("Mayor", 128)]


def step(kind, target, **kw):
    return dict(kind=kind, target=target, journal=f"{kind} {target}", **kw)


def quest(key, giver, steps, **kw):
    return dict(key=key, name=key, giver=giver, steps=steps, **kw)


class Fix(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(prefix="fixnames-")
        self.db = os.path.join(self.dir, "eqoa.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE npcs (npc_name TEXT, npc_type INTEGER)")
        db.execute("CREATE TABLE itempattern (itemname TEXT)")
        db.execute("CREATE TABLE spellpattern (spellname TEXT)")
        db.executemany("INSERT INTO npcs VALUES (?, ?)", NPCS)
        db.execute("INSERT INTO itempattern VALUES ('Pelt')")
        db.commit()
        db.close()
        rw = {"rewards": {"xp": 10}}
        self.quests = [
            # a Teir'dal name with an apostrophe the game spells with a backtick
            quest("1-20/t/a/L1", "Counselor Du'Klot", [step("kill", "thick coat wolf", **rw)],
                  unresolved=["Counselor Du'Klot"],
                  notes="From a guide. Counselor Du'Klot is not in the database. The pelts are the database's Pelt."),
            # two spellings agree with it but for a plural: which one is meant cannot be told
            quest("1-20/t/a/L2", "Mayor", [step("loot", "the wolf pup", items=[{"item": "Pelt"}], chance=1, **rw)],
                  requires=["1-20/t/a/L1"], unresolved=["the wolf pup"]),
            quest("1-20/t/a/L3", "Mayor", [step("loot", "Bat", items=[{"item": "Pelt"}], chance=1, **rw)],
                  requires=["1-20/t/a/L1"],
                  notes="It follows L1: L2 before it is a draft until its names are in the database."),
            quest("1-20/t/b/L1", "Mayor", [step("loot", "Bat", items=[{"item": "Pelt"}, ], chance=1, **rw),
                                           step("kill", "Town Guard"), step("give", "Mayor", items=[{"item": "Gem"}])],
                  unresolved=[], notes="Nobody is not in the database: Mayor stands in."),
            quest("1-20/t/b/L2", "Nobody", [step("talk", "Mayor", **rw)], requires=["1-20/t/b/L1"],
                  unresolved=["Nobody"],
                  notes="Nobody is not in the database. The wolf pups are not in the database either. "
                        "Nobody and Phil Wiggins are not in the database."),
        ]
        self.path = os.path.join(self.dir, "town.json")
        self.write(self.quests)

    def write(self, quests):
        with open(self.path, "w") as f:
            f.write(json.dumps(dict(city="Town", quests=quests), indent=1) + "\n")

    def fixed(self, **kw):
        report, changed = fx.fix([self.path], self.db, **kw)
        if self.path in changed:
            doc = json.loads(changed[self.path])
        else:
            with open(self.path) as f:
                doc = json.load(f)
        return report, {q["key"]: q for q in doc["quests"]}

    def test_a_name_the_game_spells_otherwise_is_respelled_and_found(self):
        _, qs = self.fixed()
        q = qs["1-20/t/a/L1"]
        self.assertEqual(q["giver"], "Counselor Du`Klot")
        self.assertEqual(q["steps"][0]["target"], "Thick Coat Wolves")
        self.assertEqual(q["unresolved"], [])
        self.assertEqual(q["notes"], "From a guide. The pelts are the database's Pelt.")

    def test_two_spellings_that_agree_are_left_alone(self):
        _, qs = self.fixed()
        q = qs["1-20/t/a/L2"]
        self.assertEqual(q["steps"][0]["target"], "the wolf pup")
        self.assertEqual(q["unresolved"], ["the wolf pup"])

    def test_a_chain_waits_while_a_quest_it_skipped_is_a_draft(self):
        _, qs = self.fixed()
        self.assertEqual(qs["1-20/t/a/L3"]["requires"], ["1-20/t/a/L1"])
        self.assertIn("It follows L1", qs["1-20/t/a/L3"]["notes"])

    def test_a_chain_takes_back_its_link_once_the_quest_it_skipped_is_whole(self):
        q2 = self.quests[1]
        q2["steps"][0]["target"] = "Wolf Pup"
        self.quests[4]["notes"] = "Nobody is not in the database."
        self.write(self.quests)
        report, qs = self.fixed()
        self.assertEqual(qs["1-20/t/a/L2"]["unresolved"], [])
        self.assertEqual(qs["1-20/t/a/L3"]["requires"], ["1-20/t/a/L2"])
        self.assertNotIn("notes", {k: v for k, v in qs["1-20/t/a/L3"].items() if v})

    def test_the_unresolved_are_the_checkers_own(self):
        _, qs = self.fixed()
        # the Gem is no item, and the 0x80 guard cannot be killed: both listed as the checker asks
        self.assertEqual(qs["1-20/t/b/L1"]["unresolved"], ["Gem", "Town Guard (attackable)"])
        names = cq.Names(self.db)
        ck = cq.Check(names)
        for q in qs.values():
            ck.quest("town.json", q, set())
        self.assertEqual([e for e in ck.errors if "unresolved" in e], [])

    def test_a_note_that_says_more_is_kept_and_listed(self):
        report, qs = self.fixed()
        self.assertEqual(qs["1-20/t/b/L1"]["notes"], "Nobody is not in the database: Mayor stands in.")
        self.assertTrue(any("word it again" in r and "Mayor stands in" in r for r in report))
        # a name found is dropped from the notes, one still missing anywhere is not
        self.assertEqual(qs["1-20/t/b/L2"]["notes"],
                         "The wolf pups are not in the database either. Nobody and Phil Wiggins are not in the database.")

    def test_a_rename_with_a_key_changes_that_quest_alone(self):
        _, qs = self.fixed(renames=fx.parse_renames(["1-20/t/a/:Bat=Rabid Bat"]))
        self.assertEqual(qs["1-20/t/a/L3"]["steps"][0]["target"], "Rabid Bat")
        self.assertEqual(qs["1-20/t/b/L1"]["steps"][0]["target"], "Bat")
        _, qs = self.fixed(renames=fx.parse_renames(["Bat=Rabid Bat"]))
        self.assertEqual(qs["1-20/t/b/L1"]["steps"][0]["target"], "Rabid Bat")

    def test_a_drop_note_drops_its_sentence(self):
        _, qs = self.fixed(drops=[r"database's Pelt"])
        self.assertEqual(qs["1-20/t/a/L1"]["notes"], "From a guide.")

    def test_nothing_is_written_without_write(self):
        with open(self.path) as f:
            before = f.read()
        fx.fix([self.path], self.db)
        with open(self.path) as f:
            self.assertEqual(f.read(), before)

    def test_a_name_is_told_in_brackets_and_in_the_plural(self):
        self.assertTrue(fx.mentions("The sappers are not in the database (the map calls them Goblin Sappers).", "goblin sapper"))
        self.assertTrue(fx.mentions("Counselor Du'Klot is not in the database.", "Counselor Du`Klot"))
        self.assertFalse(fx.mentions("The Bats are out.", "Bat Out"), "words in order, whole")
        self.assertFalse(fx.mentions("Grant is not in the database.", "Ant"))

    def test_join_mends_the_sentences_round_a_dropped_one(self):
        parts = [("A is the database's B;", False), ("so it is.", True)]
        self.assertEqual(fx.join(parts), "A is the database's B; so it is.")
        self.assertEqual(fx.join([("A is B;", False), ("So it is.", True)]), "A is B. So it is.")
        self.assertEqual(fx.join([("A is B;", False)]), "A is B.")
        self.assertEqual(fx.join([("A is B;", False), ("c is d.", False)]), "A is B; c is d.")


if __name__ == "__main__":
    unittest.main()
