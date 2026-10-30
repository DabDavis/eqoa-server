#!/usr/bin/env python3
"""tools/fix_death_sacrament.py against a made-up database: the eight Death
Sacrament ranks as tools/seed_masteries.py wrote them (each adding and
removing its base pet), the pets' two records a name (the weapon version
described so, the necromancer's), and owners. Ranks 1-6 swap the base pet
for its weapon version, rank 7 stops taking Swarming Bones, rank 8 is left;
an owner's book is given what its ranks give (the base out, the weapon in,
Swarming Bones back) and a character owning none is not touched; a spell of
the name that is no weapon version, or not the necromancer's, is never
taken for one; a second --apply is refused, and --undo gives the database
back as it was, byte for byte, but for a value changed since. No game data.

    python3 tools/fix_death_sacrament_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_death_sacrament.py")
sys.path.insert(0, HERE)
import fix_death_sacrament as ds  # noqa: E402

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', level INTEGER NOT NULL DEFAULT 1, requires TEXT NOT NULL DEFAULT '',
    require_spells TEXT NOT NULL DEFAULT '', adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '',
    removes TEXT NOT NULL DEFAULT '', remove_spells TEXT NOT NULL DEFAULT '');
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER NOT NULL, spellname TEXT NOT NULL,
    spelldesc TEXT NOT NULL DEFAULT '');
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'dump');
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT NOT NULL);
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL,
    addedorder INTEGER NOT NULL, onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL,
    showhide INTEGER NOT NULL);
"""

# (rank, cmid, level, base id, base name, base level, weapon id or None, weapon level)
RANKS = [
    (1, 6361, 25, 914, "Decaying Flesh", 24, 918, 25),
    (2, 6362, 30, 925, "Rotting Flesh", 29, 932, 30),
    (3, 6363, 35, 936, "Putrid Flesh", 34, 939, 35),
    (4, 6364, 40, 942, "Preserved Flesh", 39, 950, 40),
    (5, 6365, 45, 955, "Howling Death", 44, 956, 44),
    (6, 6366, 50, 963, "Ancient Death", 49, 973, 50),
    (7, 6367, 55, 700736, "Swarming Bones", 55, None, 0),
    (8, 6368, 60, 700823, "Swarming Death", 60, None, 0),
]


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for rank, cmid, level, base, name, blvl, weapon, wlvl in RANKS:
        added = f"{name}@{blvl}" if rank != 7 else "Bone Swarm@55"
        added_ids = str(base) if rank != 7 else ""
        if rank == 5:
            added = "Howling Death@45"  # EQOA-Data's level for it
        db.execute("INSERT INTO cmcatalog (id, mastery, rank, name, level, requires, require_spells, adds, add_spells, "
                   "removes, remove_spells) VALUES (?, 'Death Sacrament', ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                   (cmid, rank, f"Death Sacrament {rank}", level, f"{name}@{blvl}", str(base), added, added_ids,
                    f"{name}@{blvl}", str(base)))
        desc = "Summons a zombie to aid you." if rank < 7 else "Summon Pet"
        db.execute("INSERT INTO spellpattern VALUES (?, ?, ?, ?)", (base, blvl, name, desc))
        db.execute("INSERT INTO classspells (class, spellid) VALUES (11, ?)", (base,))
        if weapon:
            how = "with a weapon" if "Death" in name else "with weapons"
            db.execute("INSERT INTO spellpattern VALUES (?, ?, ?, ?)", (weapon, wlvl, name, f"Summons a pet {how} to aid you."))
            db.execute("INSERT INTO classspells (class, spellid) VALUES (11, ?)", (weapon,))
    # the controls: Rotting Flesh of another class's, described with weapons;
    # a Putrid Flesh of the necromancer's described as no weapon version
    db.execute("INSERT INTO spellpattern VALUES (9925, 30, 'Rotting Flesh', 'Summons a zombie with weapons, a shaman''s.')")
    db.execute("INSERT INTO classspells (class, spellid) VALUES (8, 9925)")
    db.execute("INSERT INTO spellpattern VALUES (9936, 36, 'Putrid Flesh', 'Summons a mummy to aid you, twice.')")
    db.execute("INSERT INTO classspells (class, spellid) VALUES (11, 9936)")
    # character 7 owns ranks 1, 5 and 7 (rank 7 took its Swarming Bones), and
    # holds both of Howling Death's already; character 8 owns none
    db.executemany("INSERT INTO characters VALUES (?, ?)", [(7, "Owner"), (8, "Bystander")])
    db.executemany("INSERT INTO charcms (serverid, cmid) VALUES (7, ?)", [(6361,), (6365,), (6367,)])
    for sid, spellid, order in [(7, 914, 3), (7, 955, 9), (7, 956, 10), (7, 700823, 30), (8, 914, 2), (8, 700736, 5)]:
        db.execute("INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) "
                   "VALUES (?, ?, ?, 0, -1, 0, -1)", (sid, spellid, order))
    db.commit()
    db.close()


def run(db, *args):
    r = subprocess.run([sys.executable, "-I", TOOL, "--db", db, "--no-copy", *args], capture_output=True, text=True)
    return r.returncode, r.stdout + r.stderr


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(db.iterdump())
    db.close()
    return out


class DeathSacramentTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def rows(self):
        db = sqlite3.connect(self.db)
        out = {r[0]: r[1:] for r in db.execute("SELECT id, adds, add_spells, removes, remove_spells FROM cmcatalog")}
        db.close()
        return out

    def book(self, sid):
        db = sqlite3.connect(self.db)
        out = {r[0]: r[1] for r in db.execute("SELECT spellid, addedorder FROM spells WHERE serverid = ?", (sid,))}
        db.close()
        return out

    def test_each_rank_swaps_its_pet(self):
        code, out = run(self.db, "--apply")
        self.assertEqual(code, 0, out)
        rows = self.rows()
        for rank, cmid, _, base, name, blvl, weapon, wlvl in RANKS:
            if weapon:
                self.assertEqual(rows[cmid], (f"{name}@{wlvl}", str(weapon), f"{name}@{blvl}", str(base)), f"rank {rank}")
        self.assertEqual(rows[6367], ("Bone Swarm@55", "", "", ""), "rank 7: takes nothing, shows Bone Swarm")
        self.assertEqual(rows[6368], ("Swarming Death@60", "700823", "Swarming Death@60", "700823"), "rank 8: left")
        # the controls: never the shaman's Rotting Flesh, nor the necromancer's
        # second Putrid Flesh of no weapons
        self.assertNotIn("9925", rows[6362][1])
        self.assertNotIn("9936", rows[6363][1])

    def test_owners_get_what_their_ranks_give(self):
        code, out = run(self.db, "--apply")
        self.assertEqual(code, 0, out)
        owner = self.book(7)
        self.assertNotIn(914, owner, "rank 1: Decaying Flesh 914 taken")
        self.assertIn(918, owner, "rank 1: Decaying Flesh 918 given")
        self.assertNotIn(955, owner, "rank 5: Howling Death 955 taken")
        self.assertEqual(owner[956], 10, "rank 5: its 956 kept where it was")
        self.assertIn(700736, owner, "rank 7: Swarming Bones back")
        self.assertEqual(owner[700823], 30, "rank 8 not owned: Swarming Death untouched")
        self.assertEqual(sorted(owner[s] for s in (918, 700736)), [31, 32], "added at the next display indices")
        # the control: a character owning no rank keeps its book
        self.assertEqual(self.book(8), {914: 2, 700736: 5})

    def test_a_second_apply_is_refused_and_undo_is_exact(self):
        before = dump(self.db)
        code, out = run(self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertNotEqual(dump(self.db), before)
        code, out = run(self.db, "--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("--undo first", out)
        code, out = run(self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before, "undo: the database as it was")
        code, out = run(self.db, "--undo")
        self.assertIn("nothing to undo", out)

    def test_undo_leaves_what_changed_since(self):
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE cmcatalog SET add_spells = '12345' WHERE id = 6361")  # a GM's, since
        db.commit()
        db.close()
        code, out = run(self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertIn("rank 6361: changed since, left", out)
        rows = self.rows()
        self.assertEqual(rows[6361][1], "12345", "the GM's value kept")
        self.assertEqual(rows[6362], ("Rotting Flesh@29", "925", "Rotting Flesh@29", "925"), "the rest put back")

    def test_report_writes_nothing(self):
        before = dump(self.db)
        code, out = run(self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("7 ranks to fix", out)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
