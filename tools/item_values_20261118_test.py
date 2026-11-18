#!/usr/bin/env python3
"""tools/item_values_20261118.py and tools/craft_values_20261118.py on a made-up database: a creature's zero-value
drop is priced by its siblings or its level, what is free stays 0, the plan joins two shards by id or name, a craft
made from bought ingredients sells below its cost, a second --apply does nothing and --undo gives back every value.
No game data.

    python3 -E -s tools/item_values_20261118_test.py
"""
import contextlib
import io
import json
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import craft_values_20261118 as craft  # noqa: E402
import item_values_20261118 as tool  # noqa: E402
import price_zero_items_20261115 as pz  # noqa: E402

COLS = ["patternid", "itemcost", "itemname", "equipslot", "trade", "rent", "levelreq", "maxstack", "itemicon",
        "weapondamage", "AC", "HPMAX", "POWMAX", "PoT", "HoT", "maxhp"] + list(pz.STAT_COLS)


def item(pid, name, cost=0, slot=-1, level=1, stack=99, rent=1, maxhp=0):
    r = {c: 0 for c in COLS}
    r.update(patternid=pid, itemcost=cost, itemname=name, equipslot=slot, rent=rent, levelreq=level, maxstack=stack,
             itemicon=1, maxhp=maxhp)
    return r


DDL = [
    "CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
    "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER)",
    "CREATE TABLE accountinfo (accountid INTEGER, username TEXT)",
    "CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY)",
    "CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY, npcid INTEGER, patternid INTEGER)",
    "CREATE TABLE startitems (id INTEGER PRIMARY KEY, patternid INTEGER)",
    "CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT)",
    "CREATE TABLE quests (questid INTEGER PRIMARY KEY, enabled INTEGER, repeatable INTEGER, minlevel INTEGER, events TEXT)",
    "CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT, rewarditems TEXT, "
    "byclass TEXT, choices TEXT, events TEXT)",
    "CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT)",
    "CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, level INTEGER, rarity INTEGER, name TEXT, "
    "product INTEGER, yield INTEGER, ingredients TEXT)",
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_level INTEGER)",
    "CREATE TABLE lootpool (id INTEGER PRIMARY KEY, category TEXT, npc_name TEXT, patternid INTEGER, minlevel INTEGER, "
    "maxlevel INTEGER)",
    "CREATE TABLE loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER, minlevel INTEGER, "
    "maxlevel INTEGER)",
    "CREATE TABLE npcloot (npc_name TEXT, tablename TEXT)",
]


def make(path, shift=0):
    """A shard; shift moves the ids of two records (another shard's numbering)."""
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE itempattern (" + ", ".join(
        f"{c} {'TEXT' if c == 'itemname' else 'INTEGER'}" for c in COLS) + ")")
    for d in DDL:
        db.execute(d)
    rows = [
        # priced trash of the tarantula (level 2) and a lone creature's nothing
        item(10, "tarantula fur", 7), item(11, "tarantula silk", 9),
        # zero drops
        item(20, "tarantula leg tips"),                      # sibling: median 8 of 7, 9
        item(21, "wolf tail"),                               # level: 2.4 x 30 = 72
        item(22, "bat wing"),                                # level 1: the floor, 5
        item(23 + shift, "Bandit Glove"),                    # a quest's loot step only: level 20 -> 48
        item(24, "Heirloom Thing", slot=-1),                 # kept: heirloom
        item(25, "conjured rock", rent=0),                   # kept: NO RENT
        item(26, "beads"),                                   # kept: a merchant stocks it at 0
        item(27, "quest note"),                              # kept: a quest hands it over
        item(28, "fetid bone"),                              # kept: a repeatable quest gives it
        item(29, "Rusty Sword", slot=12, stack=1),           # kept: gear (its own rule)
        item(30 + shift, "rat ear"),                         # loot, no level: a table's band 1-60 -> 30.5 -> 73
        item(31, "nothing drops this"),                      # not a drop: not this tool's
        # crafting: bar 100 + gem 50 (kept on failure), all stocked; tool 100 of 1000 hp
        item(40, "iron bar", 100), item(41, "gem", 50), item(42, "Iron Helm", 400, slot=1, stack=1),
        item(43, "Novice Hammer", 100, slot=23, level=1, stack=1, maxhp=1000),
        item(44, "dragon scale", 500), item(45, "Dragon Helm", 2000, slot=1, stack=1),   # farming: scale is loot
    ]
    for r in rows:
        db.execute(f"INSERT INTO itempattern ({', '.join(COLS)}) VALUES ({', '.join('?' * len(COLS))})",
                   [r[c] for c in COLS])
    db.executemany("INSERT INTO npcs (npc_name, npc_level) VALUES (?, ?)",
                   [("tarantula", 2), ("wolf", 30), ("bat", 1), ("bandit", 20)])
    db.executemany("INSERT INTO lootpool (category, npc_name, patternid, minlevel, maxlevel) VALUES (?, ?, ?, 1, 100)",
                   [("trash", "tarantula", 10), ("trash", "tarantula", 11), ("trash", "tarantula", 20),
                    ("trash", "wolf", 21), ("trash", "bat", 22), ("trash", "bat", 24), ("trash", "bat", 25),
                    ("trash", "bat", 26), ("trash", "bat", 27), ("trash", "bat", 28), ("trash", "wolf", 29),
                    ])
    db.execute("INSERT INTO loottables (tablename, patternid, minlevel, maxlevel) VALUES ('lonely', ?, 1, 60)",
               (30 + shift,))
    db.execute("INSERT INTO heirlooms VALUES (24)")
    db.executemany("INSERT INTO npcitem (npcid, patternid) VALUES (1, ?)", [(26,), (40,), (41,), (43,)])
    db.executemany("INSERT INTO quests VALUES (?, ?, ?, ?, '')", [(1, 1, 0, 20), (2, 1, 1, 1), (3, 1, 0, 1)])
    db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?, ?, '', '', '')", [
        (1, 1, "loot", "bandit", str(23 + shift), ""),
        (2, 1, "give", "x", "", "28"),
        (3, 1, "talk", "x", "", "27"),
    ])
    db.executemany("INSERT INTO craftrecipes VALUES (?, 1, 1, 0, ?, ?, 1, ?)", [
        (1, "Iron Helm", 42, json.dumps([{"item": 40, "count": 1, "keep": False},
                                        {"item": 41, "count": 1, "keep": True}])),
        (2, "Dragon Helm", 45, json.dumps([{"item": 44, "count": 1, "keep": False}])),
    ])
    db.commit()
    return db


def costs(path):
    db = sqlite3.connect(path)
    try:
        return dict(db.execute("SELECT patternid, itemcost FROM itempattern"))
    finally:
        db.close()


def quiet(fn, *a):
    with contextlib.redirect_stdout(io.StringIO()) as out:
        rc = fn(*a)
    return rc, out.getvalue()


class ItemValues(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.a = os.path.join(self.dir, "a.db")
        self.b = os.path.join(self.dir, "b.db")
        make(self.a).close()
        make(self.b, shift=1000).close()

    def tearDown(self):
        shutil.rmtree(self.dir)

    def test_rules(self):
        db = sqlite3.connect(self.a)
        priced, kept, other = tool.plan(db)
        db.close()
        got = {p: (r, v) for p, _n, r, v, _h in priced}
        self.assertEqual(got[20], ("sibling", 8))
        self.assertEqual(got[21], ("level", 72))
        self.assertEqual(got[22], ("level", tool.FLOOR))
        self.assertEqual(got[23], ("level", 48))
        self.assertEqual(got[30], ("level", 73))
        why = {p: r for p, _n, r, _h in kept}
        self.assertEqual(why, {24: "heirloom", 25: "conjured", 26: "merchant-free", 27: "quest-given",
                               28: "repeatable", 29: "gear"})
        self.assertNotIn(31, got)
        self.assertNotIn(31, why)

    def test_plan_ids_and_names(self):
        plan = os.path.join(self.dir, "p.json")
        rc, _ = quiet(tool.main, ["--write-plan", plan, "--from", f"a={self.a}", f"b={self.b}"])
        self.assertEqual(rc, 0)
        with open(plan) as fh:
            pl = json.load(fh)
        keys = {e["key"]: e for e in pl["entries"]}
        self.assertIn("id:20", keys)
        self.assertEqual(keys["name:bandit glove"]["shards"], {"a": [23], "b": [1023]})
        for shard, path in (("a", self.a), ("b", self.b)):
            before = costs(path)
            rc, _ = quiet(tool.main, ["--db", path, "--plan", plan, "--shard", shard, "--apply"])
            self.assertEqual(rc, 0)
            after = costs(path)
            glove = 23 if shard == "a" else 1023
            self.assertEqual(after[glove], 48)
            self.assertEqual(after[20], 8)
            self.assertEqual(after[26], 0)
            rc, out = quiet(tool.main, ["--db", path, "--plan", plan, "--shard", shard, "--apply"])
            self.assertIn("nothing to do", out)
            self.assertEqual(costs(path), after)
            g = sqlite3.connect(path)
            n = g.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
            g.close()
            self.assertEqual(n, sum(1 for k in after if after[k] != before[k]))
            rc, _ = quiet(tool.main, ["--db", path, "--undo"])
            self.assertEqual(rc, 0)
            self.assertEqual(costs(path), before)

    def test_apply_checks_now(self):
        """A record a merchant has stocked since the plan stays 0; one changed since is left."""
        db = sqlite3.connect(self.a)
        priced, _k, _o = tool.plan(db)
        db.execute("INSERT INTO npcitem (npcid, patternid) VALUES (2, 21)")
        db.execute("UPDATE itempattern SET itemcost = 3 WHERE patternid = 22")
        db.commit()
        db.close()
        db = tool.connect(self.a)
        with contextlib.redirect_stdout(io.StringIO()):
            tool.do_apply(db, priced)
        db.close()
        c = costs(self.a)
        self.assertEqual(c[21], 0)
        self.assertEqual(c[22], 3)
        self.assertEqual(c[20], 8)


class CraftValues(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.a = os.path.join(self.dir, "a.db")
        make(self.a).close()

    def tearDown(self):
        shutil.rmtree(self.dir)

    def test_economics_and_fix(self):
        db = sqlite3.connect(self.a)
        c, rows, farming, changes, _held = craft.plan(db, 0.75, False)
        db.close()
        self.assertEqual(farming, 1)                       # the dragon helm: a scale no merchant sells
        (r, e), = rows
        self.assertEqual(e["all"], 150)
        self.assertAlmostEqual(e["wear"], 1.0)
        self.assertAlmostEqual(e["cost"], (150 + 100 * 0.02 / 0.98 + 1 / 0.98))
        self.assertGreater(e["margin"], 0)
        self.assertEqual([(x[0], x[2], x[3]) for x in changes], [(42, 400, 112)])
        before = costs(self.a)
        rc, _ = quiet(craft.main, ["--db", self.a, "--apply"])
        self.assertEqual(rc, 0)
        self.assertEqual(costs(self.a)[42], 112)
        self.assertEqual(costs(self.a)[45], 2000)          # farming kept
        rc, out = quiet(craft.main, ["--db", self.a, "--apply"])
        self.assertIn("nothing to do", out)
        rc, out = quiet(craft.main, ["--db", self.a])
        self.assertIn("as craft.go does): 0", out)
        rc, _ = quiet(craft.main, ["--db", self.a, "--undo"])
        self.assertEqual(costs(self.a), before)

    def test_factor_bounds(self):
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            craft.main(["--db", self.a, "--factor", "1.2"])


if __name__ == "__main__":
    unittest.main()
