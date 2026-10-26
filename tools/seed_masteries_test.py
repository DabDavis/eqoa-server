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
                rank("Binder 2", 75, 60, add=[("Grasp", 60)])]}]
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
        self.assertEqual(r[enc + 202]["name"], "Binder 2")
        self.assertEqual((r[1]["name"], r[enc + 1]["name"], r[2001]["name"]), ("Might 1", "Empathy 1", "Hardened 1"))
        self.assertEqual(sm.master_class_name(4, 0, "Binder", "Binder the First"), "Binder the First",
                         "a first rank named otherwise is left as the list writes it")

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
