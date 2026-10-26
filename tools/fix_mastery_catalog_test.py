#!/usr/bin/env python3
"""tools/fix_mastery_catalog.py against made-up mastery lists, seeded by
tools/seed_masteries.py's own functions: a master class's first rank needs
the ranks its notes name (a possessive aside), the same master class in
another class's list takes them (OURS), a rank the list gives no level the
highest level of its abilities or the rank before it's; a GM's row, a value
changed since and a row the seed did not write are left; --apply logged,
--undo exact, a value changed since left at --undo. The walk: every class and
race over the fixed catalogue reaches every rank it is offered (parent and
require_ranks, a spell improved in place kept), with no cycle, and every
master class's prerequisites buyable by its class; the controls: planted
unreachable ranks fail the walk, and a plan that would strand a master class
is refused. No game data.

    python3 tools/fix_mastery_catalog_test.py

With EQOA_DATA (EQOA-Data's checkout) and EQOA_DB (a COPY of a server's
database) set, the same is run over that catalogue too (copied again first).
"""
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_mastery_catalog as fx  # noqa: E402
import seed_masteries as sm  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_mastery_catalog.py")
WAR, PAL, SK = 0, 2, 3
TYPES = ["Tank", "Melee", "Tank", "Tank", "Melee", "Melee", "Melee", "Healer", "Healer", "Healer",
         "Caster", "Caster", "Caster", "Caster", "Caster"]


def rank(name, cost, level=None, add=(), remove=(), need=(), spent=0, stats=None, notes=None):
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
    if add:
        eff["addAbilities"] = [{"name": n, "level": lv} for n, lv in add]
    if remove:
        eff["removeAbilities"] = [{"name": n, "level": lv} for n, lv in remove]
    out = {"name": name, "cmCost": cost, "requirements": req or None, "effects": eff}
    if notes:
        out["notes"] = notes
    return out


def bulwark(stats, notes=None):
    return {"name": "Bulwark", "ranks": [rank("Bulwark 1", 200, 55, spent=500, stats=stats, notes=notes),
                                         rank("Bulwark 2", 75, 60)]}


def vanguard(stats, notes=None):
    return {"name": "Vanguard", "ranks": [rank("Vanguard 1", 200, 55, spent=500, stats=stats, notes=notes),
                                          rank("Vanguard 2", 75, 60)]}


def write_lists(root, war_master_notes=None):
    d = os.path.join(root, "Information", "databases", "cms")
    os.makedirs(d)

    def put(name, data):
        with open(os.path.join(d, name + ".json"), "w") as fh:
            json.dump(data, fh)

    put("general", {"groups": [{"name": "General", "cms": [
        {"name": "Might", "ranks": [rank("Might 1", 60, 10), rank("Might 2", 60, None)]}]}]})
    put("racial", {"groups": [{"name": "Gnome", "cms": [{"name": "Tinkering", "ranks": [rank("Tinkering", 10, 6)]}]}]})
    put("archetype", {"groups": [
        {"name": "Tank", "cms": [
            {"name": "Guard Grace", "ranks": [rank("Guard Grace 1", 60, 20), rank("Guard Grace 2", 60, 30)]}]},
        {"name": "Caster", "cms": [{"name": "Lore", "ranks": [rank("Lore 1", 20, 10)]}]},
    ]})
    for c, f in enumerate(sm.CLASS_FILES):
        groups = [{"name": "Class Masteries", "cms": []}, {"name": "Master Classes", "cms": []}]
        if c == WAR:
            groups[0]["cms"] = [
                {"name": "Iron Will", "ranks": [
                    rank("Iron Will 1", 60, 20, add=[("Battle Cry", 20)], remove=[("Taunt", 1)], need=[("Taunt", 1)]),
                    rank("Iron Will 2", 60, 30, need=[("Battle Cry", 20)]),
                    # an improvement in place: the list adds and removes one spell
                    rank("Iron Will 3", 60, 40, add=[("Battle Cry", 20)], remove=[("Battle Cry", 20)],
                         need=[("Battle Cry", 20)]),
                    rank("Iron Will 4", 60, 50, need=[("Battle Cry", 20)])]},
                {"name": "Warding", "ranks": [
                    rank("Warding 1", 30, None, add=[("Ward", 7), ("Greater Ward", 25)]),
                    rank("Warding 2", 30, None),
                    rank("Warding 3", 30, 35)]},
            ]
            groups[1]["cms"] = [
                bulwark({"STA": 200}, war_master_notes or ["Iron Will 3", "Guard's Grace 2"]),
                vanguard({"STR": 200}, ["Guard's Grace 2"]),
                {"name": "Brute", "ranks": [rank("Brute 1", 200, 55, spent=500, notes=["Iron Will 2", "Missing Thing 2"])]},
                {"name": "Sentinel", "ranks": [rank("Sentinel 1", 200, 55, spent=500, notes=["Iron Will 2"])]},
                {"name": "Champion", "ranks": [rank("Champion 1", 200, 55, spent=500)]},
            ]
        elif c == PAL:
            groups[0]["cms"] = [{"name": "Faith", "ranks": [rank("Faith 1", 100, 20), rank("Faith 2", 100, 30),
                                                            rank("Faith 3", 100, 40)]}]
            # the warrior's two, no notes: Vanguard takes the warrior's (OURS), an
            # archetype rank; Bulwark's name the warrior's own Iron Will, so not
            groups[1]["cms"] = [bulwark({"STA": 200}), vanguard({"STR": 200})]
        elif c == SK:
            groups[0]["cms"] = [{"name": "Gloom", "ranks": [rank("Gloom 1", 100, 20), rank("Gloom 2", 100, 30),
                                                            rank("Gloom 3", 100, 40)]}]
            groups[1]["cms"] = [vanguard({"STR": 150})]  # not the same rank: left without
        put(f, {"class": f.title(), "type": TYPES[c], "groups": groups})


DB_SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER NOT NULL, spellname TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL,
                          PRIMARY KEY (class, spellid));
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, level INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '');
"""


def make_db(path, lists_root):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(DB_SCHEMA)
    for sid, lvl, name in [(500, 1, "Taunt"), (501, 20, "Battle Cry"), (510, 7, "Ward"), (511, 25, "Greater Ward")]:
        db.execute("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, lvl, name))
    for sid in (500, 501, 510, 511):
        db.execute("INSERT INTO classspells VALUES (?, ?, 'dump')", (WAR, sid))
    p = sm.plan(db, sm.load(lists_root))
    sm.apply(db, p)
    return db


def ids_by_name(db):
    out = {}
    for i, c, name in db.execute("SELECT id, classes, name FROM cmcatalog"):
        out[(name, c)] = i
    return out


def run(db, lists, *args, check=True):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", db, "--eqoa-data", lists, *args],
                          capture_output=True, text=True, check=check)


def dump(path):
    c = sqlite3.connect(path)
    out = "\n".join(c.iterdump())
    c.close()
    return out


def every_class_and_race_reaches_all(test, cat, spells_of):
    """The walk over a catalogue: nothing unreachable, no cycle, and each
    master class's prerequisites offered to and bought by its class."""
    bad = fx.walk(cat, spells_of)
    test.assertEqual(fx.describe(bad, cat), [], "ranks a class and race they are offered can never buy")
    test.assertEqual(fx.cycles(cat), set(), "ranks on a cycle")
    for i, r in cat.items():
        if r["category"] != fx.MASTER or not r["require_ranks"]:
            continue
        for need in fx.ids(r["require_ranks"]):
            test.assertIn(need, cat, f"{r['name']} needs {need}, which the catalogue lacks")
            test.assertEqual(cat[need]["classes"] & r["classes"], r["classes"],
                             f"{r['name']} needs {cat[need]['name']}, not offered to its class")


class FixMasteryCatalog(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.lists = os.path.join(self.tmp.name, "lists")
        write_lists(self.lists)
        self.path = os.path.join(self.tmp.name, "eqoa.db")
        db = make_db(self.path, self.lists)
        self.ids = ids_by_name(db)
        db.close()

    def tearDown(self):
        self.tmp.cleanup()

    def id(self, name, cls=WAR):
        return self.ids[(name, 1 << cls)]

    def db(self):
        return sqlite3.connect(self.path, isolation_level=None)

    def plan(self):
        db = self.db()
        try:
            return fx.plan(db, sm.load(self.lists))
        finally:
            db.close()

    def test_the_plan(self):
        changes, left, problems = self.plan()
        got = {(rid, col): after for rid, col, _, after, _ in changes}
        iw2, iw3, gg2 = self.id("Iron Will 2"), self.id("Iron Will 3"), self.ids[("Guard Grace 2", 13)]
        self.assertEqual(got[(self.id("Bulwark"), "require_ranks")], f"{iw3},{gg2}",
                         "the notes' ranks, the possessive aside ('Guard's Grace 2' is 'Guard Grace 2')")
        self.assertEqual(got[(self.id("Vanguard", PAL), "require_ranks")], str(gg2),
                         "the same master class in the paladin's list, which names none, takes the warrior's")
        why = {rid: w for rid, col, _, _, w in changes}
        self.assertIn("OURS", why[self.id("Vanguard", PAL)])
        self.assertNotIn((self.id("Bulwark", PAL), "require_ranks"), got,
                         "the warrior's notes name its own Iron Will: not the paladin's to take")
        self.assertTrue(any("Bulwark (PAL)" in p and "not taking" in p for p in problems), problems)
        self.assertNotIn((self.id("Vanguard", SK), "require_ranks"), got, "the knight's Vanguard is not the same rank")
        self.assertNotIn((self.id("Champion"), "require_ranks"), got, "a master class whose notes name nothing")
        self.assertEqual(got[(self.id("Brute"), "require_ranks")], str(iw2), "the note found; the other said")
        self.assertTrue(any("Missing Thing 2" in p for p in problems), problems)
        self.assertEqual(got[(self.id("Warding 1"), "level")], 25, "the highest level of an ability it adds")
        self.assertEqual(got[(self.id("Warding 2"), "level")], 25, "the rank before it's")
        self.assertNotIn((self.ids[("Might 2", sm.ALL_CLASSES)], "level"), got,
                         "the rank before it's, as the seed has it: no change")
        self.assertEqual(left, [])

    def test_what_it_leaves(self):
        db = self.db()
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('cmcatalog', ?)", (json.dumps({"id": self.id("Vanguard", PAL)}),))
        db.execute("UPDATE cmcatalog SET require_ranks = ? WHERE id = ?", (str(self.id("Iron Will 1")), self.id("Sentinel")))
        db.execute("UPDATE cmcatalog SET source = 'gm' WHERE id = ?", (self.id("Warding 1"),))
        db.close()
        changes, left, _ = self.plan()
        changed = {rid for rid, *_ in changes}
        why = {rid: w for rid, _, w in left}
        self.assertIn("gm_audit", why.get(self.id("Vanguard", PAL), ""))
        self.assertIn("changed since", why.get(self.id("Sentinel"), ""))
        self.assertIn("not a row the seed wrote", why.get(self.id("Warding 1"), ""))
        self.assertFalse(changed & {self.id("Vanguard", PAL), self.id("Sentinel"), self.id("Warding 1")})
        self.assertIn(self.id("Bulwark"), changed, "the control: the warrior's Bulwark is still changed")

    def test_apply_then_undo(self):
        # two owners a corrected requirement catches: a level 20 holding
        # Warding 1 (now 25), and one holding Bulwark without its two ranks
        db = self.db()
        db.execute("INSERT INTO characters VALUES (7, 20)")
        db.execute("INSERT INTO charcms (serverid, cmid) VALUES (7, ?)", (self.id("Warding 1"),))
        db.execute("INSERT INTO charcms (serverid, cmid) VALUES (8, ?)", (self.id("Bulwark"),))
        db.close()
        before = dump(self.path)
        out = run(self.path, self.lists).stdout
        self.assertIn("report only", out)
        self.assertIn("characters whose ranks a corrected requirement would not let them buy now: 2", out)
        self.assertEqual(dump(self.path), before, "a report writes nothing")

        out = run(self.path, self.lists, "--apply").stdout
        self.assertIn("changed 7 values", out)
        db = self.db()
        got = dict(db.execute("SELECT id, require_ranks FROM cmcatalog WHERE require_ranks != ''"))
        self.assertEqual(got[self.id("Bulwark")], f"{self.id('Iron Will 3')},{self.ids[('Guard Grace 2', 13)]}")
        self.assertEqual(db.execute("SELECT level FROM cmcatalog WHERE id = ?", (self.id("Warding 1"),)).fetchone()[0], 25)
        self.assertEqual(db.execute("SELECT count(*) FROM charcms").fetchone()[0], 2, "what characters own is kept")
        self.assertEqual(db.execute("SELECT count(*) FROM cmcatalog WHERE enabled = 0").fetchone()[0], 0)
        db.close()
        self.assertTrue([f for f in os.listdir(self.tmp.name) if "pre-cmfix" in f], "a copy first")

        self.assertIn("0 values to change", run(self.path, self.lists).stdout, "a second run changes nothing")
        self.assertIn("0 values to change", run(self.path, self.lists, "--apply").stdout, "nor a second --apply")
        out = run(self.path, self.lists, "--undo").stdout
        self.assertIn("put back 7 of 7", out)
        self.assertEqual(dump(self.path), before, "--undo puts the database back as it was")
        self.assertIn("nothing to undo", run(self.path, self.lists, "--undo").stdout)

    def test_undo_leaves_a_value_changed_since(self):
        run(self.path, self.lists, "--apply")
        db = self.db()
        db.execute("UPDATE cmcatalog SET level = 30 WHERE id = ?", (self.id("Warding 1"),))
        db.close()
        out = run(self.path, self.lists, "--undo").stdout
        self.assertIn("changed since", out)
        db = self.db()
        lv = dict(db.execute("SELECT id, level FROM cmcatalog"))
        rr = dict(db.execute("SELECT id, require_ranks FROM cmcatalog"))
        db.close()
        self.assertEqual(lv[self.id("Warding 1")], 30, "a GM's value since is kept")
        self.assertEqual((lv[self.id("Warding 2")], rr[self.id("Bulwark")]), (1, ""), "the control: the rest go back")

    def test_walk_every_class_and_race_over_the_fixed_catalogue(self):
        run(self.path, self.lists, "--apply")
        db = self.db()
        every_class_and_race_reaches_all(self, fx.catalogue(db), fx.class_spells(db))
        db.close()

    def test_walk_controls(self):
        run(self.path, self.lists, "--apply")
        db = self.db()
        cat, spells_of = fx.catalogue(db), fx.class_spells(db)
        db.close()
        self.assertEqual(fx.walk(cat, spells_of), {}, "the control: the fixed catalogue walks clean")

        def planted(**rows):
            out = {i: dict(r) for i, r in cat.items()}
            for i, (base, change) in enumerate(rows.items(), start=90001):
                r = dict(cat[self.id(base) if not isinstance(base, int) else base])
                r.update(id=i, parent=0, require_ranks="", require_spells="", add_spells="", remove_spells="")
                r.update(change)
                out[i] = r
            return out

        faith1 = self.ids[("Faith 1", 1 << PAL)]
        cases = {
            "a rank its class is never offered": planted(**{"Iron Will 1": dict(require_ranks=str(faith1))}),
            "a level past 60": planted(**{"Iron Will 1": dict(level=61)}),
            "a spell the class never holds": planted(**{"Iron Will 1": dict(require_spells="999")}),
            "a spell a rank it needs takes out": planted(**{"Iron Will 2": dict(parent=self.id("Iron Will 1"),
                                                                               require_spells="500")}),
            "another master class": planted(**{"Champion": dict(require_ranks=str(self.id("Bulwark")))}),
        }
        for what, c in cases.items():
            with self.subTest(what):
                self.assertIn(90001, fx.walk(c, spells_of), f"{what}: the planted rank must fail the walk")
        loop = planted(**{"Iron Will 1": dict(require_ranks="90002"), "Iron Will 2": dict(parent=90001)})
        self.assertEqual(fx.cycles(loop), {90001, 90002})
        self.assertIn(90001, fx.walk(loop, spells_of))
        with self.assertRaises(AssertionError):
            every_class_and_race_reaches_all(self, cases["a level past 60"], spells_of)

    def test_a_plan_that_strands_a_rank_is_refused(self):
        lists = os.path.join(self.tmp.name, "racial-lists")
        write_lists(lists, war_master_notes=["Tinkering"])  # a gnome's own: no other race could buy Bulwark
        before = dump(self.path)
        p = run(self.path, lists, "--apply", check=False)
        self.assertNotEqual(p.returncode, 0)
        self.assertIn("refused", p.stdout + p.stderr)
        self.assertIn("Bulwark", p.stdout)
        self.assertEqual(dump(self.path), before, "nothing written")


@unittest.skipUnless(os.environ.get("EQOA_DATA") and os.environ.get("EQOA_DB"),
                     "EQOA_DATA and EQOA_DB (a copy of a server's database) not set")
class RealCatalogue(unittest.TestCase):
    def test_fixed_walks_clean_and_undoes_exactly(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "eqoa.db")
            shutil.copy2(os.environ["EQOA_DB"], path)
            before = dump(path)
            out = run(path, os.environ["EQOA_DATA"], "--apply").stdout
            self.assertNotIn("refused", out)
            db = sqlite3.connect(path)
            self.assertEqual(db.execute("SELECT count(*) FROM cmcatalog WHERE enabled = 0").fetchone()[0], 0)
            every_class_and_race_reaches_all(self, fx.catalogue(db), fx.class_spells(db))
            db.close()
            run(path, os.environ["EQOA_DATA"], "--undo")
            self.assertEqual(dump(path), before, "--undo puts it back as it was")


if __name__ == "__main__":
    unittest.main()
