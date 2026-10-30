#!/usr/bin/env python3
"""tools/seed_loot_categories.py against a made-up world: the presets (jib
active with his trash counts, the user's with a legendary rate of five each
in ten years), the gear pools by category and region (the generic table's
every region's), the gems by jib's shares, each creature's own trash (a
trophy whose whole creature its name holds), the global list's legendary
weight, the creatures' regions and profiles, and the bindings taken away.
The controls: a giant badger gets no giant's trophy, a quest trophy and a
no-trade one are no trash, a pet rolls nothing, a boss's own table and a
GM's binding stay. --apply then --undo leaves the database as it was (the four
tables made and dropped again); a second --apply is refused. No game data.

    python3 tools/seed_loot_categories_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_loot_categories.py")
sys.path.insert(0, HERE)
import loot_tiers as lt  # noqa: E402
import seed_loot_categories as slc  # noqa: E402

COLS = ("patternid", "itemname", "levelreq", "equipslot", "attacktype", "weapondamage", "AC", "HPMAX", "POWMAX", "HoT",
        "PoT", "weaponproc", "str", "sta", "agi", "dex", "wis", "intelligence", "cha", "FR", "LR", "CR", "PR", "DR", "AR",
        "itemicon", "itemcost", "maxstack", "trade")
SCHEMA = f"""
CREATE TABLE itempattern ({', '.join(c + (' TEXT' if c == 'itemname' else ' INTEGER') for c in COLS)});
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_id INTEGER, npc_name TEXT, world INTEGER, x_coord REAL,
    y_coord REAL, z_coord REAL, npc_level INTEGER, npc_type INTEGER);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE queststeps (questid INTEGER, step INTEGER, items TEXT NOT NULL DEFAULT '');
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
"""


def item(pid, name, level, slot, **kw):
    row = dict.fromkeys(COLS, 0)
    row.update(patternid=pid, itemname=name, levelreq=level, equipslot=slot, maxstack=1)
    row.update(kw)
    return row


ITEMS = ([item(1 + i, f"ring {i}", 20, 8, str=10) for i in range(9)] +
         [item(20, "great ring", 20, 8, str=20), item(21, "greater ring", 20, 8, str=30),
          item(22, "greatest ring", 20, 8, str=30, sta=10), item(23, "Aura of Karana", 26, 13, AC=210),
          item(24, "generic ring", 20, 8, str=10)] +
         [item(30, "Aquamarine", 0, -1, maxstack=50), item(31, "Blue Diamond", 0, -1, maxstack=50),
          item(32, "Pyrite", 0, -1, maxstack=50), item(33, "Jade", 0, -1, maxstack=50)] +
         [item(40, "clump of rat fur", 0, -1, maxstack=99), item(41, "bent rat whiskers", 0, -1, maxstack=99, itemcost=12),
          item(42, "frost giant eye", 0, -1, maxstack=99), item(38, "rat tail tip", 0, -1, maxstack=99),
          item(39, "torn rat ear", 0, -1, maxstack=99, trade=1)])
LOOT = ([("plains", p, 15, 25) for p in (1, 2, 3, 20, 21, 22, 23)] + [("generic", 24, 15, 25)] +
        [("craft gems", 30, 1, 60), ("craft gems", 32, 1, 60), ("craft gems", 33, 10, 60)] +
        [("craft gems", 31, 30, 60)] + [("boss old red", 1, 1, 60)])
WEIGHTS = {30: 30, 32: 30, 33: 9, 31: 1}
NPCS = [  # npc_id, name, world, level, type
    (1, "rat", 1, 20, 0), (2, "rat", 1, 22, 0), (3, "giant badger", 1, 20, 0), (4, "frost giant", 2, 50, 0),
    (5, "Old Red", 1, 20, 0), (6, "Deputy Allen", 1, 20, 0), (7, "primal spirit", 1, 20, 0), (8, "Guard Rolf", 1, 20, 0),
]


class SeedLootCategories(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.executemany(f"INSERT INTO itempattern VALUES ({', '.join('?' * len(COLS))})", [tuple(r[c] for c in COLS) for r in ITEMS])
        db.executemany("INSERT INTO npcs (npc_id, npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type) "
                       "VALUES (?, ?, ?, 0, 0, 0, ?, ?)", NPCS)
        db.execute("INSERT INTO npcfaction (name, faction) VALUES ('guard rolf', 'Citizens')")
        db.execute("INSERT INTO npcaggro (npc_name, aggressive, note, source) VALUES ('primal spirit', 0, "
                   "'a summoned pet''s name (spellpattern): a player''s pet', 'seed')")
        db.execute("INSERT INTO queststeps (questid, step, items) VALUES (1, 1, '38')")  # the rat tail tip is a quest's
        db.executemany("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES (?, ?, ?, ?, ?)",
                       [(t, p, WEIGHTS.get(p, 1) if t == "craft gems" else 1, lo, hi) for t, p, lo, hi in LOOT])
        db.executemany("INSERT INTO npcloot (npc_name, tablename, chance, rolls, source) VALUES (?, ?, ?, 1, ?)",
                       [("rat", "plains", 25, "camp-seed"), ("rat", "generic", 15, "camp-seed"),
                        ("giant badger", "rathe mountains", 25, "region-seed"), ("*", "craft gems", 5, "tradeskill-seed"),
                        ("old red", "boss old red", 100, "boss-seed"), ("deputy allen", "plains", 10, "gm")])
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def plan(self):
        db = sqlite3.connect(self.path)
        try:
            return slc.plan(db, None, None)
        finally:
            db.close()

    def test_plan(self):
        presets, rates, rows, creatures, unbind, prices, info = self.plan()
        self.assertEqual(sorted((p, a) for p, a, _ in presets), [("jib", 1), ("user", 0)])
        r = {(p, prof, c): (rate, counts) for p, prof, c, rate, counts, *_ in rates}
        self.assertEqual(r[("jib", "xp", "trash")], (0.0, "25,31,25,12,5,1.5,0.5"))
        self.assertAlmostEqual(r[("jib", "xp", "uncommon")][0], 1 / 30)
        self.assertAlmostEqual(r[("jib", "named", "rare")][0], 3 / 240)
        self.assertNotIn(("jib", "trash", "uncommon"), r)
        self.assertIn(("jib", "trash", "global"), r)
        # the user's legendaries: five of each in ten years of the server's kills
        self.assertAlmostEqual(r[("user", "xp", "legendary")][0], 5 * 1 / lt.kills_in())
        pools = {(x[0], x[1], x[6] if len(x) > 6 else "", x[2]) for x in rows}
        self.assertIn(("uncommon", "plains", "", 1), pools)
        self.assertIn(("rare", "plains", "", 20), pools)
        self.assertIn(("super rare", "plains", "", 21), pools)
        self.assertIn(("ultra rare", "plains", "", 22), pools)
        self.assertIn(("uncommon", "*", "", 24), pools, "the generic table's: every region's")
        self.assertIn(("legendary", "*", "", 23), pools)
        self.assertIn(("global", "*", "", 22), pools)
        # the gems by jib's shares: two commons 44 each, the uncommon 10, the ultra rare 0.5
        gw = {x[2]: x[3] for x in rows if x[0] == "gems"}
        self.assertEqual(gw, {30: 4400, 32: 4400, 33: 1000, 31: 50})
        # the trash: the rats' two (whiskers, fur; the quest's tail and the no-trade ear never), the frost giant's eye,
        # nothing for the giant badger (its giant is a word of the badger's)
        trash = {}
        for x in rows:
            if x[0] == "trash":
                trash.setdefault(x[6], set()).add(x[2])
        self.assertEqual(trash, {"rat": {40, 41}, "frost giant": {42}})
        self.assertEqual(prices.get(40, (0, 0))[0], 0)
        self.assertNotIn(41, prices, "a trophy with a price keeps it")
        # the global list: the legendary's weight its five of the draws in ten years
        draws = lt.kills_in() / 10000
        share = 5 / draws
        self.assertEqual(info["global_leg_weight"], max(1, round(share * 100 / (1 - share))))
        cr = {k: (rg, prof) for k, rg, prof in creatures}
        self.assertEqual(cr["rat"], ("rathe mountains", ""))
        self.assertEqual(cr["frost giant"], ("odus", ""))
        self.assertEqual(cr["old red"][1], "boss")
        self.assertEqual(cr["deputy allen"][1], "named")
        self.assertEqual(cr["guard rolf"][1], "", "a town's person is no named creature")
        self.assertEqual(cr["primal spirit"][0], "-")
        gone = {(n, t) for n, t, *_ in unbind}
        self.assertEqual(gone, {("rat", "plains"), ("rat", "generic"), ("giant badger", "rathe mountains"), ("*", "craft gems")})

    def test_apply_undo(self):
        def state():
            db = sqlite3.connect(self.path)
            out = {}
            for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name <> 'sqlite_sequence' ORDER BY name"):
                out[t] = sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
            out["schema"] = sorted(db.execute("SELECT type, name FROM sqlite_master WHERE name <> 'sqlite_sequence'").fetchall())
            db.close()
            return out

        run = lambda *a: subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a],
                                        capture_output=True, text=True)
        before = state()
        r = run("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT count(*) FROM npcloot WHERE source = 'boss-seed'").fetchone(), (1,))
        self.assertEqual(db.execute("SELECT count(*) FROM npcloot WHERE source = 'gm'").fetchone(), (1,))
        self.assertEqual(db.execute("SELECT name FROM lootpreset WHERE active = 1").fetchall(), [("jib",)])
        self.assertGreater(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 40").fetchone()[0], 0)
        db.close()
        self.assertNotEqual(run("--apply").returncode, 0, "a second --apply is refused")
        r = run("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertNotIn("changed since", r.stdout)
        self.assertEqual(state(), before)


if __name__ == "__main__":
    unittest.main()
