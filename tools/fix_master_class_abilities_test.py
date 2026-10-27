#!/usr/bin/env python3
"""tools/fix_master_class_abilities.py against made-up mastery lists and a
database seeded the way tools/seed_masteries.py did before it (a master
class's ranks a chain, named "<Class> 2"): after --apply each ability needs
only its master class (but the one improving another's), is named for what it
gives, and the catalog is what a fresh seed writes; a GM's row (gm_audit), a
name changed since, a row the seed did not write and the classes' own ranks
are left; another tool's row is noted; what characters own is kept; --apply
logged, --undo exact, a value changed since left at --undo. No game data.

    python3 tools/fix_master_class_abilities_test.py

With EQOA_DATA (EQOA-Data's checkout) and EQOA_DB (a COPY of a server's
database) set, it is run over that catalog too (copied again first).
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
TOOL = os.path.join(HERE, "fix_master_class_abilities.py")
WAR, ALC = 0, 14
TYPES = ["Tank", "Melee", "Tank", "Tank", "Melee", "Melee", "Melee", "Healer", "Healer", "Healer",
         "Caster", "Caster", "Caster", "Caster", "Caster"]
FIRST = {WAR: 3000 + 200, ALC: 3000 + 300 * ALC + 200}  # a class's first master class's base id


def rank(name, cost, level=None, add=(), remove=(), need=(), spent=0, stats=None, effects=None, notes=None):
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
    out = {"name": name, "cmCost": cost, "requirements": req or None, "effects": eff}
    if notes:
        out["notes"] = notes
    return out


def write_lists(root):
    d = os.path.join(root, "Information", "databases", "cms")
    os.makedirs(d)

    def put(name, data):
        with open(os.path.join(d, name + ".json"), "w") as fh:
            json.dump(data, fh)

    put("general", {"groups": [{"name": "General", "cms": [
        # enough to spend the 500 points a master class needs
        {"name": "Might", "ranks": [rank("Might 1", 300, 10), rank("Might 2", 300, 20)]}]}]})
    put("racial", {"groups": [{"name": "Gnome", "cms": [{"name": "Tinkering", "ranks": [rank("Tinkering", 10, 6)]}]}]})
    put("archetype", {"groups": [{"name": "Tank", "cms": [
        {"name": "Grit", "ranks": [rank("Grit 1", 20, 10), rank("Grit 2", 20, 20)]}]}]})
    for c, f in enumerate(sm.CLASS_FILES):
        groups = [{"name": "Class Masteries", "cms": []}, {"name": "Master Classes", "cms": []}]
        if c == ALC:
            groups[0]["cms"] = [{"name": "Brewing", "ranks": [rank("Brewing 1", 20, 20), rank("Brewing 2", 20, 30)]}]
            groups[1]["cms"] = [
                # the report's: a tag, then two abilities the list writes as a chain
                {"name": "Transmuter", "ranks": [
                    rank("Transmuter 1", 200, 55, spent=500, effects={"INT": 10}),
                    rank("Transmuter 2", 75, 60, add=[("Creative Transmutation", 60)]),
                    rank("Transmuter 3", 75, 60, add=[("Transmutative Mixture", 60)])]},
                {"name": "Metabolist", "ranks": [
                    rank("Metabolist 1", 200, 55, spent=500),
                    rank("Metabolist 2", 75, 60, add=[("Miraculous Mixture", 60)]),
                    rank("Metabolist 3", 75, None, effects={"HP": 200, "HoT": 10})]},  # stats alone: the table's
            ]
        elif c == WAR:
            groups[1]["cms"] = [{"name": "Defender", "ranks": [
                rank("Defender 1", 200, 55, spent=500),
                rank("Defender 2", 75, 60, add=[("Guard", 60)]),
                rank("Defender 3", 0, 60, add=[("Immortal Guard", 60)]),  # MASTER_UPGRADES: after Guard
                rank("Defender 4", 75, 60, effects={"FR": 25})]}]
        put(f, {"class": f.title(), "type": TYPES[c], "groups": groups})


DB_SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER NOT NULL, spellname TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL,
                          PRIMARY KEY (class, spellid));
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, level INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '');
CREATE TABLE other_fill_log (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL);
"""

SPELLS = [(700827, 60, "Transmutive Mixture", ALC), (700802, 60, "Miraculous Mixture", ALC),
          (701155, 60, "Guard", WAR), (700790, 60, "Immortal Guard", WAR)]


def make_db(path, lists_root, old_way=True):
    """A database the seed filled; old_way: as the seed wrote a master class
    before (each rank after the first chained to the one before it, named as
    the list names it)."""
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(DB_SCHEMA)
    for sid, lvl, name, cls in SPELLS:
        db.execute("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, lvl, name))
        db.execute("INSERT INTO classspells VALUES (?, ?, 'eqoa-data')", (cls, sid))
    sm.apply(db, sm.plan(db, sm.load(lists_root)))
    if old_way:
        db.execute("UPDATE cmcatalog SET parent = id - 1, name = mastery || ' ' || rank WHERE category = 4 AND rank > 1")
    return db


def run(db, lists, *args, check=True):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", db, "--eqoa-data", lists, *args],
                          capture_output=True, text=True, check=check)


def dump(path):
    c = sqlite3.connect(path)
    out = "\n".join(c.iterdump())
    c.close()
    return out


def masters(path):
    """{id: (name, parent)} of every Master Class rank."""
    c = sqlite3.connect(path)
    out = {i: (n, p) for i, n, p in c.execute("SELECT id, name, parent FROM cmcatalog WHERE category = 4")}
    c.close()
    return out


class FixMasterClassAbilities(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.lists = os.path.join(self.tmp.name, "lists")
        write_lists(self.lists)
        self.path = os.path.join(self.tmp.name, "eqoa.db")
        make_db(self.path, self.lists).close()
        self.tm, self.mb, self.df = FIRST[ALC] + 1, FIRST[ALC] + 11, FIRST[WAR] + 1  # the tags

    def tearDown(self):
        self.tmp.cleanup()

    def db(self):
        return sqlite3.connect(self.path, isolation_level=None)

    def test_the_old_seed_is_a_chain(self):
        # the control the rest stands on: a database seeded before is chained
        m = masters(self.path)
        self.assertEqual(m[self.tm + 2], ("Transmuter 3", self.tm + 1))
        self.assertEqual(m[self.df + 3], ("Defender 4", self.df + 2))

    def test_report_writes_nothing(self):
        before = dump(self.path)
        out = run(self.path, self.lists).stdout
        self.assertIn("report only", out)
        self.assertIn("'Transmuter 2' -> 'Creative Transmutation'", out)
        self.assertEqual(dump(self.path), before)

    def test_apply_is_what_a_fresh_seed_writes(self):
        run(self.path, self.lists, "--apply")
        m = masters(self.path)
        self.assertEqual(m[self.tm], ("Transmuter", 0))
        self.assertEqual(m[self.tm + 1], ("Creative Transmutation", self.tm), "the list's name: no spell of it")
        self.assertEqual(m[self.tm + 2], ("Transmutive Mixture", self.tm), "the spell's name, needing only the tag")
        self.assertEqual(m[self.mb + 2], ("Metabolist's Vigor", self.mb), "stats alone: MASTER_ABILITY_NAMES'")
        self.assertEqual(m[self.df + 2], ("Immortal Guard", self.df + 1), "an upgrade keeps the rank it improves")
        self.assertEqual(m[self.df + 3], ("Defender's Toughness", self.df))
        fresh = os.path.join(self.tmp.name, "fresh.db")
        make_db(fresh, self.lists, old_way=False).close()
        self.assertEqual(masters(self.path), masters(fresh), "the fix and a fresh seed agree")
        db = self.db()
        brewing = dict(db.execute("SELECT id, parent FROM cmcatalog WHERE category != 4 AND rank > 1"))
        db.close()
        self.assertTrue(brewing and all(p == i - 1 for i, p in brewing.items()), "the other categories stay chains")

    def test_what_it_leaves_and_notes(self):
        db = self.db()
        db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('cmcatalog', ?)", (json.dumps({"id": self.mb + 1}),))
        db.execute("UPDATE cmcatalog SET name = 'Ogre Pill' WHERE id = ?", (self.tm + 1,))
        db.execute("UPDATE cmcatalog SET source = 'gm' WHERE id = ?", (self.df + 3,))
        db.execute("INSERT INTO other_fill_log (tbl, rowkey) VALUES ('cmcatalog', ?)", (json.dumps({"id": self.tm + 2}),))
        db.execute("UPDATE cmcatalog SET add_spells = '700802' WHERE id = ?", (self.tm + 2,))  # that tool's change
        db.close()
        out = run(self.path, self.lists, "--apply").stdout
        m = masters(self.path)
        self.assertEqual(m[self.mb + 1], ("Metabolist 2", self.mb), "a GM's row (gm_audit) left whole")
        self.assertEqual(m[self.tm + 1], ("Ogre Pill", self.tm), "a GM's name kept, its parent still the seed's: fixed")
        self.assertEqual(m[self.df + 3], ("Defender 4", self.df + 2), "a row the seed did not write left")
        self.assertEqual(m[self.tm + 2], ("Miraculous Mixture", self.tm), "another tool's row: named for its spell now")
        for what in ("gm_audit", "changed since", "not a row the seed wrote", "other_fill_log"):
            self.assertIn(what, out)
        self.assertEqual(m[self.mb + 2], ("Metabolist's Vigor", self.mb), "the control: the rest are fixed")

    def test_apply_then_undo(self):
        db = self.db()
        db.execute("INSERT INTO characters VALUES (7, 60)")
        db.executemany("INSERT INTO charcms (serverid, cmid, cost) VALUES (7, ?, ?)",
                       [(self.tm, 200), (self.tm + 1, 75)])
        db.close()
        before = dump(self.path)
        out = run(self.path, self.lists, "--apply").stdout
        self.assertIn("changed 10 values", out)
        self.assertIn("characters owning a master class's ability: 1 (1 ranks)", out)
        self.assertIn("0 unreachable after", out)
        self.assertTrue([f for f in os.listdir(self.tmp.name) if "pre-mcabilities" in f], "a copy first")
        db = self.db()
        self.assertEqual(db.execute("SELECT cmid, cost FROM charcms ORDER BY cmid").fetchall(),
                         [(self.tm, 200), (self.tm + 1, 75)], "what characters own is kept")
        self.assertEqual(db.execute("SELECT count(*) FROM mc_ability_fix_log").fetchone()[0], 10)
        db.close()
        self.assertIn("0 values to change", run(self.path, self.lists).stdout, "a second run changes nothing")
        self.assertIn("0 values to change", run(self.path, self.lists, "--apply").stdout, "nor a second --apply")
        out = run(self.path, self.lists, "--undo").stdout
        self.assertIn("put back 10 of 10", out)
        self.assertEqual(dump(self.path), before, "--undo puts the database back as it was")
        self.assertIn("nothing to undo", run(self.path, self.lists, "--undo").stdout)

    def test_undo_leaves_a_value_changed_since(self):
        run(self.path, self.lists, "--apply")
        db = self.db()
        db.execute("UPDATE cmcatalog SET name = 'Ogre Pill' WHERE id = ?", (self.tm + 1,))
        db.close()
        out = run(self.path, self.lists, "--undo").stdout
        self.assertIn("changed since", out)
        m = masters(self.path)
        self.assertEqual(m[self.tm + 1], ("Ogre Pill", self.tm), "a GM's name since kept; its parent was the tag")
        self.assertEqual(m[self.tm + 2], ("Transmuter 3", self.tm + 1), "the control: the rest go back")

    def test_a_later_run_takes_up_its_own_change(self):
        # a value this wrote is its own to change again, the first value
        # before kept for --undo
        before = dump(self.path)
        run(self.path, self.lists, "--apply")
        db = self.db()  # as an earlier version of this would have left it
        db.execute("UPDATE mc_ability_fix_log SET after = 'Old Name' WHERE id = ? AND col = 'name'", (self.tm + 1,))
        db.execute("UPDATE cmcatalog SET name = 'Old Name' WHERE id = ?", (self.tm + 1,))
        db.close()
        out = run(self.path, self.lists, "--apply").stdout
        self.assertNotIn("changed since", out)
        self.assertEqual(masters(self.path)[self.tm + 1][0], "Creative Transmutation")
        run(self.path, self.lists, "--undo")
        self.assertEqual(dump(self.path), before, "--undo goes back to the first value before")

    def test_the_walk_over_the_fixed_catalog(self):
        run(self.path, self.lists, "--apply")
        db = self.db()
        cat, spells_of = fx.catalogue(db), fx.class_spells(db)
        db.close()
        self.assertEqual(fx.walk(cat, spells_of), {}, "every class and race reaches every rank it is offered")
        self.assertEqual(fx.cycles(cat), set())


@unittest.skipUnless(os.environ.get("EQOA_DATA") and os.environ.get("EQOA_DB"),
                     "EQOA_DATA and EQOA_DB (a copy of a server's database) not set")
class RealCatalog(unittest.TestCase):
    def test_fixed_reads_right_and_undoes_exactly(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "eqoa.db")
            shutil.copy2(os.environ["EQOA_DB"], path)
            before = dump(path)
            out = run(path, os.environ["EQOA_DATA"], "--apply").stdout
            self.assertNotIn("refused", out)
            db = sqlite3.connect(path)
            rows = db.execute("SELECT id, mastery, rank, name, parent FROM cmcatalog WHERE category = 4").fetchall()
            tags = {(m, i - r + 1) for i, m, r, _, _ in rows if r == 1}
            for i, m, r, name, parent in rows:
                if r == 1:
                    continue
                with self.subTest(f"{m} {r}"):
                    self.assertNotEqual(name, f"{m} {r}", "named for what it gives")
                    if (m.lower(), r) not in sm.MASTER_UPGRADES:
                        self.assertIn((m, parent), tags, "needs only its master class")
            every = fx.catalogue(db)
            self.assertEqual(fx.walk(every, fx.class_spells(db)), {})
            db.close()
            run(path, os.environ["EQOA_DATA"], "--undo")
            self.assertEqual(dump(path), before, "--undo puts it back as it was")


if __name__ == "__main__":
    unittest.main()
