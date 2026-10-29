#!/usr/bin/env python3
"""tools/import_quests.py against a made-up database and content file
(tools/quests/testdata/sample.json): names resolved as a player reads them, a
quest item made from its like, a quest with a missing creature a draft and the
quest after it too, a run again a no-op, a GM's change kept, and --undo exact;
and the later guides' fields (tools/quests/testdata/later.json): a faction
requirement, repeatable, the trips a quest unlocks and rewards by class. No
game data.

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
LATER = os.path.join(HERE, "quests", "testdata", "later.json")
# the routes the unlocks are matched to, as waysender.go's would be
ROUTES = [("Waysender Ti`Thox", "Freeport"), ("Waysender Ti`Thox", "Grobb"), ("Waysender Abodai", "Neriak")]

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
        cols = [c for c in iq.columns(db, t) if c not in ("events", "chance", "factionreq", "byclass")]
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

    def test_a_gms_mended_draft_frees_the_quests_after_it(self):
        iq.write(self.db, iq.plan(self.db, self.files))
        ghost, _ = self.quest("1-20/testmoor/warrior/L3")
        self.db.execute("UPDATE queststeps SET target = 'Field Boar' WHERE questid = ?", (ghost["questid"],))
        self.db.execute("UPDATE quests SET enabled = 1 WHERE questid = ?", (ghost["questid"],))
        written, kept, _ = iq.write(self.db, iq.plan(self.db, self.files))
        self.assertEqual(kept, ["1-20/testmoor/warrior/L3"])
        self.assertEqual(self.quest("1-20/testmoor/warrior/L5")[0]["enabled"], 1, "the GM's whole quest frees the one after it")
        # the control: the GM turns it off again, and the one after is a draft at the next run
        self.db.execute("UPDATE quests SET enabled = 0 WHERE questid = ?", (ghost["questid"],))
        iq.write(self.db, iq.plan(self.db, self.files))
        self.assertEqual(self.quest("1-20/testmoor/warrior/L5")[0]["enabled"], 0)

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



def old_digest(db, qid):
    """An import's digest as it was before the later columns and the unlocks
    (its columns verbatim), which quest_import_quests rows of an earlier
    import hold."""
    qcols = [c for c in iq.QUEST_COLS if c not in iq.LATER_COLS]
    scols = [c for c in iq.STEP_COLS if c not in iq.LATER_COLS]
    h = hashlib.md5()
    for r in db.execute(f"SELECT {', '.join(qcols)} FROM quests WHERE questid = ?", (qid,)):
        h.update(repr(tuple(r)).encode())
    for r in db.execute(f"SELECT {', '.join(scols)} FROM queststeps WHERE questid = ? ORDER BY step", (qid,)):
        h.update(repr(tuple(r)).encode())
    return h.hexdigest()


class LaterFields(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        self.db = make_db(self.path)
        self.db.execute("INSERT INTO npcs (npc_name, world) VALUES ('Waysender Ti`Thox', 1)")
        self.files = iq.load_files([SAMPLE, LATER])

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def quest(self, key):
        return ImportQuests.quest(self, key)

    def test_the_later_fields_resolve(self):
        p = iq.plan(self.db, self.files, routes=ROUTES)
        iq.write(self.db, p)
        q, (st,) = self.quest("Misc/testmoor-writ/p1")
        self.assertEqual((q["enabled"], q["repeatable"], q["factionreq"]), (1, 1, "Testmoor Watch:100..500"),
                         "the faction as the database spells it, its range")
        self.assertEqual(json.loads(st["byclass"]), {"0": {"xp": 50, "items": "2", "spells": "2"}, "4": {"tunar": 10, "spells": "1"}},
                         "each class's Swipe its own (the warrior's 2, the monk's 1), the item by its id")
        trips = self.db.execute("SELECT npc, dest FROM questunlocks WHERE questid = ? ORDER BY dest", (q["questid"],)).fetchall()
        self.assertEqual(trips, [("Waysender Ti`Thox", "Grobb"), ("Waysender Ti`Thox", "Testmoor Isle")],
                         "the route's own label where one is matched; one naming no route written as given")
        warned = "\n".join(p["warnings"])
        self.assertIn("'Testmoor Isle', which is no route", warned)
        self.assertNotIn("'the grobb', which is no route", warned, "the control: the grobb is Grobb")
        self.assertEqual(q["source"], "EQOA-Data Misc/testmoor-writ/p1 (tools/import_quests.py)", "the new category's key")

    def test_a_choice_resolves_as_a_steps_rewards(self):
        iq.write(self.db, iq.plan(self.db, self.files, routes=ROUTES))
        q, (st,) = self.quest("60/testmoor-epic/p1")
        self.assertEqual(q["enabled"], 1)
        self.assertEqual(json.loads(st["choices"]), {"ask": "Which will you take?", "options": [
            {"answer": "The spear.", "items": "2"}, {"answer": "The swipe.", "xp": 5, "spells": "2"},
            {"answer": "Neither.", "says": "Then think on it.", "stay": True}]}, "the warrior's Swipe, the spear by its id")
        # an option's missing item makes a draft, as a step's reward does
        files = json.loads(json.dumps(self.files))
        files[1][1]["quests"][2]["steps"][0]["choices"][0]["rewards"]["items"] = ["Golden Spear"]
        p = iq.plan(self.db, files, routes=ROUTES)
        epic = [x for x in p["quests"] if x["key"] == "60/testmoor-epic/p1"][0]
        self.assertIn("item 'Golden Spear' (step 1 option 1)", epic["why"])

    def test_a_faction_this_database_lacks_makes_a_draft(self):
        iq.write(self.db, iq.plan(self.db, self.files, routes=ROUTES))
        q, _ = self.quest("Misc/testmoor-writ/p2")
        self.assertEqual(q["enabled"], 0)
        self.assertIn("faction 'Moor Hermits' (required)", q["notes"])
        self.db.execute("INSERT INTO factions VALUES ('Moor Hermits', 0)")
        p = iq.plan(self.db, self.files, routes=ROUTES)
        lost = [x for x in p["quests"] if x["key"] == "Misc/testmoor-writ/p2"][0]
        self.assertEqual(lost["why"], ["the file's unresolved 'Moor Hermits'"],
                         "the faction now here: only the file's own list keeps it out (fix_names.py mends that)")

    def test_unlocks_go_with_undo_and_a_run_again_is_a_no_op(self):
        before = state(self.db)
        iq.write(self.db, iq.plan(self.db, self.files, routes=ROUTES))
        once = state(self.db)
        trips = self.db.execute("SELECT * FROM questunlocks ORDER BY dest").fetchall()
        written, kept, _ = iq.write(self.db, iq.plan(self.db, self.files, routes=ROUTES))
        self.assertEqual((written, kept), (6, []))
        self.assertEqual((state(self.db), self.db.execute("SELECT * FROM questunlocks ORDER BY dest").fetchall()), (once, trips))
        # a GM's unlock row on an imported quest is a GM's change: left as it is
        qid = self.quest("Misc/testmoor-writ/p1")[0]["questid"]
        self.db.execute("INSERT INTO questunlocks VALUES (?, 'Waysender Abodai', 'Neriak')", (qid,))
        _, kept, _ = iq.write(self.db, iq.plan(self.db, self.files, routes=ROUTES))
        self.assertEqual(kept, ["Misc/testmoor-writ/p1"])
        n = iq.undo(self.db)
        self.assertEqual(n["trips"], 3)
        self.assertEqual(self.db.execute("SELECT count(*) FROM questunlocks").fetchone()[0], 0)
        self.assertEqual(state(self.db), before, "undo leaves the database as it was")

    def test_an_earlier_imports_digest_still_holds(self):
        iq.write(self.db, iq.plan(self.db, self.files, routes=ROUTES))
        plain = self.quest("1-20/testmoor/warrior/L1")[0]["questid"]
        later = self.quest("Misc/testmoor-writ/p1")[0]["questid"]
        self.assertEqual(iq.digest(self.db, plain), old_digest(self.db, plain),
                         "a quest with none of the later fields digests as an earlier import logged it")
        self.assertNotEqual(iq.digest(self.db, later), old_digest(self.db, later),
                            "the control: a quest with them digests them too")

    def test_the_servers_routes_are_read(self):
        rs = iq.travel.travel_routes()
        self.assertIsNotNone(rs, "waysender.go beside the tools")
        self.assertIn(("Necrologist Zizclank", "the Plane of Disease"), rs)
        with open(iq.travel.WAYSENDER) as f:
            src = f.read()
        block = src[src.index("var wayRoutes"):src.index("\n}\n", src.index("var wayRoutes"))]
        self.assertEqual(len(rs), block.count("{To:"), "every route of the map, none past it")
        self.assertEqual(iq.travel.route_of("waysender windfoot", "Klik`Anon", rs), ("Waysender Windfoot", "Klik'Anon"))
        self.assertIsNone(iq.travel.route_of("Waysender Windfoot", "Klick'Anon", rs), "a letter is another place")


if __name__ == "__main__":
    unittest.main()
