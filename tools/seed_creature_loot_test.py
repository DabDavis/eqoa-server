#!/usr/bin/env python3
"""tools/seed_creature_loot.py against a made-up world: each creature of a
band gets a table of its own holding its composition, the band's items spread
over its creatures, each item at its tier's rate a kill within the chance's
rounding, a legendary at five over its creatures' share of ten years' kills,
and a merchant's ware near it as a common; the controls get nothing: a player's
pet, a training dummy, a creature bound already (unless --retier-bound, which
takes its pooled binding away). The legendary's row in the generic table is
held at weight 0. --apply then --undo leaves npcloot and loottables as they
were, both ways; a second --apply is refused. No game data.

    python3 tools/seed_creature_loot_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_creature_loot.py")
sys.path.insert(0, HERE)
import loot_tiers as lt  # noqa: E402
import seed_creature_loot as scl  # noqa: E402

COLS = ("patternid", "itemname", "levelreq", "equipslot", "attacktype", "weapondamage", "AC", "HPMAX", "POWMAX", "HoT",
        "PoT", "weaponproc", "str", "sta", "agi", "dex", "wis", "intelligence", "cha", "FR", "LR", "CR", "PR", "DR", "AR",
        "itemicon")
SCHEMA = f"""
CREATE TABLE itempattern ({', '.join(c + (' TEXT' if c == 'itemname' else ' INTEGER') for c in COLS)});
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_id INTEGER, npc_name TEXT, world INTEGER, x_coord REAL,
    y_coord REAL, z_coord REAL, npc_level INTEGER, npc_type INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
"""


def item(pid, name, level, slot, **stats):
    row = dict.fromkeys(COLS, 0)
    row.update(patternid=pid, itemname=name, levelreq=level, equipslot=slot, **stats)
    return row


ITEMS = ([item(1 + i, f"plain cap {i}", 23, 1, AC=50) for i in range(3)] +          # common
         [item(10 + i, f"ring {i}", 23, 8, str=10) for i in range(4)] +              # uncommon
         [item(20, "great ring", 23, 8, str=40), item(21, "Grimblade", 26, 12, attacktype=1, weapondamage=300),
          item(22, "town boots", 22, 11, AC=40)] +                                   # rare, legendary, a ware
         [item(30 + i, f"filler ring {i}", 23, 8, str=10) for i in range(4)] +      # the level's median
         [item(50 + i, f"band ring {i}", 23, 8, str=10) for i in range(8)] +        # more uncommons than slots
         [item(60, "Aquamarine", 0, -1)])                                            # a gem every creature rolls
LOOT = [("rathe mountains", p, 18, 28) for p in (1, 10, 11, 12, 20)] + [("generic", 13, 18, 28), ("generic", 21, 21, 31)] + \
       [("rathe mountains", 50 + i, 18, 28) for i in range(8)] + [("generic", 60, 18, 28), ("craft gems", 60, 1, 60)]
NPCS = [  # npc_id, name, world, x, z, level, type
    (1, "hill troll", 1, 0, 0, 23, 0), (2, "rock golem", 1, 5000, 0, 24, 0), (3, "hill troll", 1, 60, 0, 22, 0),
    (4, "primal spirit", 1, 70, 0, 23, 0), (5, "training dummy", 1, 80, 0, 23, 0), (6, "cave bear", 1, 90, 0, 23, 0),
    (7, "Smith Rolf", 1, 10, 0, 30, 1),  # a merchant, beside the hill troll
]


class SeedCreatureLoot(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.executemany(f"INSERT INTO itempattern VALUES ({', '.join('?' * len(COLS))})",
                       [tuple(r[c] for c in COLS) for r in ITEMS])
        db.executemany("INSERT INTO npcs (npc_id, npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type) "
                       "VALUES (?, ?, ?, ?, 0, ?, ?, ?)", NPCS)
        db.execute("INSERT INTO npcitem (npcid, patternid) VALUES (7, 22)")
        db.execute("INSERT INTO npcaggro (npc_name, aggressive, note, source) VALUES ('primal spirit', 0, "
                   "'a summoned pet''s name (spellpattern): a player''s pet recorded beside its owner', 'seed')")
        db.executemany("INSERT INTO loottables (tablename, patternid, minlevel, maxlevel) VALUES (?, ?, ?, ?)", LOOT)
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, source) VALUES ('cave bear', 'generic', 15, 1, 'camp-seed')")
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, source) VALUES ('*', 'craft gems', 5, 1, 'tradeskill-seed')")
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def plan(self, retier=False):
        db = sqlite3.connect(self.path)
        try:
            w = scl.World(db)
            tables, unbind, left = scl.plan(w, retier)
            leg = scl.rates(w, tables)
            return w, scl.rows_of(tables, leg), unbind, leg, scl.held(w)
        finally:
            db.close()

    def test_tables_rates_and_controls(self):
        w, made, unbind, leg, hold = self.plan()
        by = {k: (table, chance, items) for k, table, chance, items in made}
        self.assertEqual(set(by), {"hill troll", "rock golem"}, "no pet, dummy or bound creature")
        self.assertEqual(by["hill troll"][0], "hill troll 21-25")
        tiers = {pid: (lt.LEGENDARY if pid in leg else w.tiers[pid][0]) for *_, items in made for pid, *_ in items}
        # one common drops in the band; the hill troll's second is the ware of the merchant beside it
        want = {"hill troll": (1, 3, 2, 1), "rock golem": (1, 3, 1, 1)}
        for k, (table, chance, items) in by.items():
            count = {t: sum(1 for pid, *_ in items if tiers[pid] == t) for t in lt.TIERS}
            self.assertEqual((count[lt.RARE], count[lt.UNCOMMON], count[lt.COMMON], count[lt.LEGENDARY]), want[k], k)
            self.assertEqual(chance, round(100 * sum(r for *_, r in items)))
            total = sum(i[1] for i in items)
            for pid, weight, lo, hi, r in items:
                self.assertEqual((lo, hi), (21, 25), "the band's window")
                p = chance / 100 * weight / total
                self.assertAlmostEqual(p / r, 1, delta=0.03, msg=f"{k}: {w.items[pid][0]}")
        # the gem in the generic table is every creature's already (the craft gems' "*" roll): no one's own
        self.assertFalse(any(pid == 60 for *_, items in made for pid, *_ in items))
        # the ware beside the hill troll is its common, not the golem's (out of reach)
        self.assertIn(22, [pid for pid, *_ in by["hill troll"][2]])
        self.assertNotIn(22, [pid for pid, *_ in by["rock golem"][2]])
        # twelve uncommons over the two: three each, six apart
        unc = [{pid for pid, *_ in by[k][2] if tiers[pid] == lt.UNCOMMON} for k in by]
        self.assertEqual(len(unc[0] | unc[1]), 6)
        # Grimblade: five over the kills of its creatures' share (3 of the 7 spawns that can be attacked: the
        # pet, the dummy and the merchant count)
        r, n = leg[21]
        self.assertEqual(n, 3)
        self.assertAlmostEqual(r / (lt.LEGENDARY_COUNT / (lt.kills_in() * 3 / 7)), 1, places=9)
        self.assertEqual([(t, pid) for _, t, pid in hold], [("generic", 21)])

    def test_retier_places_every_pooled_item(self):
        w, made, unbind, leg, hold = self.plan(retier=True)
        self.assertEqual([(k, t) for k, t, *_ in unbind], [("cave bear", "generic")])
        held_by = {pid for *_, items in made for pid, *_ in items}
        # every item of the pools but what every creature rolls already (the gem)
        pooled = {p for t, p, _, _ in LOOT if not t.startswith("craft")} - {p for t, p, _, _ in LOOT if t.startswith("craft")}
        self.assertEqual(pooled - held_by, set(), "with the pooled bindings gone, every item of the pools is placed")
        # the control: without --retier-bound the twelve uncommons are not all placed (the pooled tables hold them)
        w, made, *_ = self.plan(retier=False)
        held_by = {pid for *_, items in made for pid, *_ in items}
        self.assertTrue(pooled - held_by)

    def test_apply_undo_both_ways(self):
        def state():
            db = sqlite3.connect(self.path)
            out = (db.execute("SELECT * FROM npcloot ORDER BY 1, 2").fetchall(),
                   db.execute("SELECT * FROM loottables ORDER BY id").fetchall())
            db.close()
            return out

        run = lambda *a: subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a],
                                        capture_output=True, text=True)
        before = state()
        r = run("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT weight FROM loottables WHERE tablename = 'generic' AND patternid = 21").fetchone(), (0,))
        self.assertEqual(db.execute("SELECT count(*) FROM npcloot WHERE npc_name = 'cave bear'").fetchone(), (1,))
        db.close()
        self.assertNotEqual(run("--apply").returncode, 0, "a second --apply is refused")
        r = run("--undo")
        self.assertIn("took back", r.stdout)
        self.assertNotIn("changed since", r.stdout)
        self.assertEqual(state(), before)
        # --retier-bound: the cave bear's pooled binding goes, and comes back
        r = run("--retier-bound", "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT tablename FROM npcloot WHERE npc_name = 'cave bear'").fetchall(), [("cave bear 21-25",)])
        db.close()
        r = run("--undo")
        self.assertNotIn("changed since", r.stdout)
        self.assertEqual(state(), before)


if __name__ == "__main__":
    unittest.main()
