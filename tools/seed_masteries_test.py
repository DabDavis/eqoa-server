#!/usr/bin/env python3
"""tools/seed_masteries.py against made-up mastery lists and a made-up
database: ids, chains, categories, who each rank is for, the abilities found
(by name, without punctuation, as a master class's, another class's left
out), the were masteries left out, a GM's row kept, --apply logged and --undo
exact. No game data.

    python3 tools/seed_masteries_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_masteries as sm  # noqa: E402

WAR, SK, ENC, WIZ = 0, 3, 12, 13
TYPES = ["Tank", "Melee", "Tank", "Tank", "Melee", "Melee", "Melee", "Healer", "Healer", "Healer",
         "Caster", "Caster", "Caster", "Caster", "Caster"]

DB_SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER NOT NULL, spellname TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL,
                          PRIMARY KEY (class, spellid));
"""


def rank(name, cost, level=None, stats=None, effects=None, add=(), remove=(), need=(), spent=0):
    req = {}
    if level is not None:
        req["level"] = level
    if spent:
        req["spent"] = spent
    if stats:
        req["stats"] = stats
    if need:
        req["requiredAbilities"] = [{"name": n, "level": lv} for n, lv in need]
    eff = {}
    if effects:
        eff["stats"] = effects
    if add:
        eff["addAbilities"] = [{"name": n, "level": lv} for n, lv in add]
    if remove:
        eff["removeAbilities"] = [{"name": n, "level": lv} for n, lv in remove]
    return {"name": name, "cmCost": cost, "requirements": req, "effects": eff}


def write_lists(root):
    d = os.path.join(root, "Information", "databases", "cms")
    os.makedirs(d)

    def put(name, data):
        with open(os.path.join(d, name + ".json"), "w") as fh:
            json.dump(data, fh)

    put("general", {"groups": [{"name": "General", "cms": [
        {"name": "Might", "description": "Strength.", "ranks": [
            rank("Might 1", 10, 10, {"STR": 100}, {"STR": 1, "STRMax": 15}),
            rank("Might 2", 15, None, {"STR": 125}, {"STR": 2, "STRMax": 20})]},
        {"name": "Mending", "ranks": [
            rank("Mending 1", 5, 20, add=[("Bandage", 1)]),
            # a shadow knight's spell in a general rank: left out
            rank("Mending 2", 9, 30, add=[("Dark Touch", 30)], remove=[("Bandage", 1)], need=[("Bandage", 1)])]},
    ]}]})
    put("racial", {"groups": [
        {"name": "Gnome", "cms": [{"name": "Tinkering", "ranks": [rank("Tinkering", 10, 6)]}]},
        {"name": ["Human Western", "Human Eastern"], "cms": [{"name": "Drill", "ranks": [rank("Drill", 5, 8)]}]},
    ]})
    put("archetype", {"groups": [
        {"name": "Tank", "cms": [
            {"name": "Hardened", "ranks": [rank("Hardened 1", 20, 10, effects={"STA": 10})]},
            {"name": "Heroic Were-Hunter", "ranks": [rank("Heroic Were-Hunter 1", 50, 60)]}]},
        {"name": "Caster", "cms": [{"name": "Clarity", "ranks": [rank("Clarity 1", 20, 10, effects={"PoT": 2})]}]},
    ]})
    for c, f in enumerate(sm.CLASS_FILES):
        groups = [{"name": "Class Masteries", "cms": []}, {"name": "Master Classes", "cms": []}]
        if c == ENC:
            groups[0]["cms"] = [{"name": "Empathy", "description": "Breezes.", "ranks": [
                rank("Empathy 1", 20, 25, add=[("Empathetic Breeze", 25)]),
                rank("Empathy 2", 25, 35, add=[("Mind Rune", 35)], remove=[("Empathetic Breeze", 25)],
                     need=[("Empathetic Breeze", 25)])]}]
            groups[1]["cms"] = [{"name": "Binder", "ranks": [
                rank("Binder 1", 200, 55, {"INT": 150, "CHA": 225}, {"CHA": 20, "CHAMax": 50, "Pow": 500}, spent=500),
                rank("Binder 2", 75, 60, add=[("Grasp", 60)]),  # found as the spell "Grasp Binder"
                rank("Binder 3", 75, 60, add=[("Chant", 60)]),  # no spell of the name
                rank("Binder 4", 75, 60, effects={"CHAMax": 25}),  # stats alone
                # it needs and replaces Binder 2's Grasp: it improves it
                rank("Binder 5", 0, 60, add=[("Tight Grasp", 60)], remove=[("Grasp", 60)], need=[("Grasp", 60)]),
                rank("Binder 6", 1, 60, add=[("Rune", 60)]),
                rank("Binder 7", 50, 60, add=[("Bolt", 60), ("Ward", 60)])]}]  # several, in no table
        put(f, {"class": f.title(), "type": TYPES[c], "groups": groups})


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(DB_SCHEMA)
    for sid, lvl, name in [(1, 1, "Bandage"), (2, 30, "Dark Touch"), (3, 25, "Empathic Breeze"),
                           (4, 35, "Mind-Rune"), (5, 60, "Grasp Binder"), (6, 25, "Empathic Breeze")]:
        db.execute("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, lvl, name))
    for cls, sid, src in [(SK, 2, "dump"), (ENC, 3, "eqoa-data cm"), (WIZ, 6, "eqoa-data"),
                          (ENC, 4, "eqoa-data cm"), (ENC, 5, "eqoa-data master Binder")]:
        db.execute("INSERT INTO classspells VALUES (?, ?, ?)", (cls, sid, src))
    return db


class SeedTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        write_lists(self.tmp.name)
        self.path = os.path.join(self.tmp.name, "eqoa.db")
        self.db = make_db(self.path)
        self.data = sm.load(self.tmp.name)

    def tearDown(self):
        self.db.close()
        self.tmp.cleanup()

    def rows(self):
        return {r["id"]: r for r in sm.plan(self.db, self.data)["rows"]}

    def test_ids_chains_and_who(self):
        r = self.rows()
        self.assertEqual((r[1]["name"], r[1]["parent"], r[1]["category"]), ("Might 1", 0, 0))
        self.assertEqual((r[2]["parent"], r[2]["level"]), (1, 10), "a rank without a level takes the one before it")
        self.assertEqual((r[1]["classes"], r[1]["races"]), (32767, 2047))
        self.assertEqual((r[1001]["races"], r[1101]["races"]), (1 << 3, 1 << 0), "gnome; both human groups are race 0")
        tanks = (1 << 0) | (1 << 2) | (1 << 3)
        self.assertEqual((r[2001]["category"], r[2001]["classes"]), (2, tanks))
        self.assertEqual(r[2201]["classes"], sum(1 << c for c in (10, 11, 12, 13, 14)))
        enc = 3000 + 300 * ENC
        self.assertEqual((r[enc + 1]["category"], r[enc + 2]["parent"]), (3, enc + 1))
        self.assertEqual((r[enc + 201]["category"], r[enc + 201]["classes"], r[enc + 201]["spent"]), (4, 1 << ENC, 500))
        self.assertEqual(r[enc + 201]["minstats"], "INT:150,CHA:225")
        self.assertEqual(r[enc + 201]["effects"], "CHA:20,CHAMax:50,Pow:500")
        self.assertEqual(r[1]["description"], "Strength.")
        self.assertNotIn(2011, r, "the were-hunter mastery is left out")

    def test_a_master_classs_first_rank_is_named_bare(self):
        # F3 20261026-135728: "Binder 1" is the class itself, "Binder"; the
        # controls: its second rank, and the first ranks of the other
        # categories, keep their numbers
        r = self.rows()
        enc = 3000 + 300 * ENC
        self.assertEqual((r[enc + 201]["name"], r[enc + 201]["mastery"]), ("Binder", "Binder"))
        self.assertEqual((r[1]["name"], r[enc + 1]["name"], r[2001]["name"]), ("Might 1", "Empathy 1", "Hardened 1"))
        self.assertEqual(sm.master_class_name(4, 0, "Binder", "Binder the First"), "Binder the First",
                         "a first rank named otherwise is left as the list writes it")

    def test_a_master_classs_abilities_need_only_the_class(self):
        # F3 20261026-222613, jib: "Tag *unlocks* the 2, but isn't a singular
        # master class purchase chain": each rank after the first has the
        # master class itself as its parent, but one improving an earlier
        # rank's ability (it needs or replaces it, or MASTER_UPGRADES says so)
        # keeps that rank. The controls: a class mastery and a general one
        # are chains still.
        enc = 3000 + 300 * ENC + 200
        with mock.patch.dict(sm.MASTER_UPGRADES, {("binder", 6): (3, "test: Rune improves Chant")}):
            r = self.rows()
        self.assertEqual([r[enc + i]["parent"] for i in range(1, 8)],
                         [0, enc + 1, enc + 1, enc + 1, enc + 2, enc + 3, enc + 1])
        self.assertEqual([bool(r[enc + i]["kept"]) for i in range(2, 8)], [False, False, False, True, True, False])
        self.assertIn("Grasp", r[enc + 5]["kept"])
        empathy = 3000 + 300 * ENC
        self.assertEqual((r[empathy + 2]["parent"], r[2]["parent"]), (empathy + 1, 1), "the controls: chains")
        with mock.patch.dict(sm.MASTER_UPGRADES, {("binder", 6): (6, "after itself")}):
            with self.assertRaises(SystemExit):
                self.rows()

    def test_a_master_classs_abilities_named_for_what_they_give(self):
        # jib: "named what they are ... and not "Transmuter 2"": the one
        # ability a rank adds, as the server's spell is named, as the list
        # names it when no spell is found, else MASTER_ABILITY_NAMES; a rank
        # adding several and in no table keeps its name (the control)
        enc = 3000 + 300 * ENC + 200
        with mock.patch.dict(sm.MASTER_ABILITY_NAMES, {("binder", 4): ("Binder's Ward", "test")}):
            r = self.rows()
        self.assertEqual([r[enc + i]["name"] for i in range(1, 8)],
                         ["Binder", "Grasp Binder", "Chant", "Binder's Ward", "Tight Grasp", "Rune", "Binder 7"])
        self.assertEqual(r[enc + 2]["add_spells"], "5", "named for the spell it was found as")
        self.assertEqual(r[enc + 4]["named"], "test")
        self.assertEqual(r[enc + 3]["mastery"], "Binder", "the mastery is still the master class")
        self.assertEqual(self.rows()[enc + 4]["name"], "Binder 4", "the control: in no table, stats alone keep it")
        # read off a row as cmcatalog keeps it: a spell given since names it
        row = dict(r[enc + 3], add_spells="7")
        self.assertEqual(sm.ability_rank_name(row, {7: "Chant of Binding$"})[0], "Chant of Binding")
        self.assertEqual(sm.ability_rank_name(dict(row, add_spells="0"), {})[0], "Chant")

    def test_abilities_found(self):
        r = self.rows()
        enc = 3000 + 300 * ENC
        # a close spelling, the enchanter's own record (not the wizard's of the same name)
        self.assertEqual((r[enc + 1]["adds"], r[enc + 1]["add_spells"]), ("Empathic Breeze@25", "3"))
        # without punctuation; and what it needs and replaces
        self.assertEqual(r[enc + 2]["add_spells"], "4")
        self.assertEqual((r[enc + 2]["remove_spells"], r[enc + 2]["require_spells"]), ("3", "3"))
        # a master class's ability as the spell lists write it, with the master's name
        self.assertEqual(r[enc + 202]["add_spells"], "5")
        # a general rank: the shared Bandage found, the shadow knight's spell left out (shown, not given)
        self.assertEqual(r[11]["add_spells"], "1")
        self.assertEqual((r[12]["adds"], r[12]["add_spells"]), ("Dark Touch@30", ""))
        self.assertEqual(r[12]["require_spells"], "1")

    def test_apply_and_undo_exact(self):
        sm.apply(self.db, sm.plan(self.db, self.data))  # makes the tables
        self.db.execute("DELETE FROM cmcatalog")
        self.db.execute("DROP TABLE cmcatalog_seed")
        self.db.execute("INSERT INTO cmcatalog (id, name, cost, source) VALUES (1, 'A GM''s rank', 3, 'gm')")
        before = self.db.execute("SELECT * FROM cmcatalog ORDER BY id").fetchall()
        p = sm.plan(self.db, self.data)
        self.assertEqual([k["id"] for k in p["kept"]], [1])
        sm.apply(self.db, p)
        self.assertEqual(self.db.execute("SELECT name, source FROM cmcatalog WHERE id = 1").fetchone(),
                         ("A GM's rank", "gm"), "a GM's row is never written over")
        n = self.db.execute("SELECT count(*) FROM cmcatalog WHERE source = ?", (sm.SOURCE,)).fetchone()[0]
        self.assertEqual(n, len(p["write"]))
        self.assertEqual(self.db.execute("SELECT count(*) FROM cmcatalog_seed").fetchone()[0], n)
        # a character owns a seeded rank: refused without --force
        self.db.execute("INSERT INTO charcms (serverid, cmid, cost) VALUES (7, 2, 15)")
        with self.assertRaises(SystemExit):
            sm.undo(self.db, False)
        sm.undo(self.db, True)
        self.assertEqual(self.db.execute("SELECT * FROM cmcatalog ORDER BY id").fetchall(), before)
        self.assertEqual(self.db.execute("SELECT count(*) FROM charcms").fetchone()[0], 1, "--force leaves charcms")

    def test_schema_is_the_servers(self):
        here = os.path.dirname(os.path.abspath(__file__))
        with open(os.path.join(here, "..", "internal", "database", "cmcatalog.go")) as fh:
            go = fh.read()
        start = go.index("const cmCatalogSchema = `") + len("const cmCatalogSchema = `")
        server = go[start:go.index("`", start)]
        self.assertEqual(" ".join(server.split()), " ".join(sm.SCHEMA.split()),
                         "the tool's tables must be database/cmcatalog.go's")


if __name__ == "__main__":
    unittest.main()
