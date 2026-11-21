#!/usr/bin/env python3
"""tools/tradeskills_f72b_20261121.py on a made database: master-craft cloth and plate recipes, the jewel gems and
their loot rows and lots, the l60 gems table and its bosses."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "tradeskills_f72b_20261121.py")
TABLES = ["itempattern", "craftrecipes", "charrecipes", "charcraftskills", "loottables", "lootpool", "auction_lots", "npcloot"]


class F72b(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript("""
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemcost INTEGER, equipslot INTEGER);
            CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, comp INTEGER, level INTEGER, rarity INTEGER,
                name TEXT, product INTEGER, ingredients TEXT, difficulty INTEGER, xp INTEGER, source TEXT);
            CREATE TABLE charrecipes (charid INTEGER, recipe INTEGER, ord INTEGER, PRIMARY KEY (charid, recipe));
            CREATE TABLE charcraftskills (charid INTEGER, skill INTEGER, level INTEGER, rank INTEGER, xp INTEGER,
                PRIMARY KEY (charid, skill));
            CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER,
                minlevel INTEGER, maxlevel INTEGER, source TEXT);
            CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT, region TEXT, npc_name TEXT,
                patternid INTEGER, weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT);
            CREATE TABLE auction_lots (lot INTEGER PRIMARY KEY, patternid INTEGER, min_bid INTEGER, buyout INTEGER,
                state TEXT, bids INTEGER, high_bidder INTEGER, perpetual INTEGER);
            CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT,
                PRIMARY KEY (npc_name, tablename));
            CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
            CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
                ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
        """)
        pid = [1000]

        def item(name, cost=10, slot=-1):
            pid[0] += 1
            db.execute("INSERT INTO itempattern VALUES (?, ?, ?, ?)", (pid[0], name, cost, slot))
            return pid[0]

        rid = [0]

        def recipe(skill, comp, level, rarity, ings, product, source, diff=None, x=None):
            rid[0] += 1
            db.execute("INSERT INTO craftrecipes VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                       (rid[0], skill, comp, level, rarity, f"r{rid[0]}", product, json.dumps(ings),
                        diff if diff is not None else 10 * level + 2 * rarity,
                        x if x is not None else 25 * level * (1 + rarity), source))
            return rid[0]

        # 1: 196 cloth/leather master recipes (7 a pattern, 28 patterns: a level each of 7 classes stands in), 10 plate ones
        pats = ["Tweed", "Vineweb Silk", "Red Lined", "Embroidered", "Pigskin", "Shaded Leather", "Trimmed Leather", "Superior"]
        raws = ["Silky Thread", "Vineweb Silk Thread", "Blood Drenched Thread", "Silklined Swatch",
                "Intact Fur Hide", "Double Layered Hide", "Flakey Hide", "Brown Split Hide"]
        raw_ids = [item(r) for r in raws]
        gem = item("Some Modifier")
        self.cloth = []
        n = 0
        while n < 196:
            k = n % 8
            p = item(f"{pats[k]} Piece{n} Pattern")
            ings = [{"item": p, "count": 1, "keep": False}, {"item": raw_ids[k], "count": 1, "keep": False},
                    {"item": gem, "count": 1, "keep": True}]
            self.cloth.append(recipe(1, 10, 7 + (k % 4), 1, ings, item(f"prod{n}", 5, 5), "master-craft-1114"))
            n += 1
        plate = item("Bronzed Tunic Pattern")
        spring = item("Cobalt Spring")
        self.plate = recipe(1, 10, 7, 1, [{"item": plate, "count": 1, "keep": False}, {"item": spring, "count": 1, "keep": False}],
                            item("plate prod", 5, 5), "master-craft-1114")
        # 2: the gems, a bar, the rare peers and their rows, the two ultra gems' 24 recipes each
        bar = item("Silver Bar", 290)
        for name in ["Ruby", "Diamond", "Emerald", "Sapphire"]:
            g = item(name, 270)
            db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('craft gems', ?, 3, 20, 60)", (g,))
            db.execute("INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel) "
                       "VALUES ('gems', '*', '', ?, 38, 20, 60)", (g,))
        self.gems = {}
        for i, name in enumerate(["Fire Emerald", "Star Ruby"]):
            g = item(name, 800)
            self.gems[name] = g
            db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('craft gems', ?, 1, 30, 60)", (g,))
            db.execute("INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel) "
                       "VALUES ('gems', '*', '', ?, 12, 30, 60)", (g,))
            db.execute("INSERT INTO auction_lots VALUES (?, ?, 800, 800, 'open', 0, 0, 1)", (100 + i, g))
            for t in range(5, 11):
                for m in range(4):
                    mold = item(f"mold{m}")
                    recipe(3, t - 1, t, 3, [{"item": bar, "count": 1, "keep": False}, {"item": mold, "count": 1, "keep": False},
                                            {"item": g, "count": 1, "keep": True}],
                           item(f"{name} piece {t} {m}", 3 * 290 + 10 + 2000, 3), "tradeskill-seed")
        # 3: the table and its four, the bosses (Tal'Thir not on this shard)
        bs = item("Black Sapphire", 800)
        db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('l60 gems', ?, 1, 1, 70)", (bs,))
        for n in ["king cepheus", "queen lacerta", "saftemoot", "haliai high priestess"]:
            db.execute("INSERT INTO npcloot VALUES (?, 'l60 gems', 25, 1, '', 'board')", (n,))
        for n in ["Shiran Harpy Queen", "Jal'Raeth Patriarch", "Jal'Raeth Bishop", "High Priest Vorin", "Overlord Xrag",
                  "Overseer Durekar", "Overseer Durekar"]:
            db.execute("INSERT INTO npcs (npc_name) VALUES (?)", (n,))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def q(self, sql, *a):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def snapshot(self):
        return {t: self.q(f"SELECT * FROM {t} ORDER BY 1, 2") for t in TABLES}

    def test_report_writes_nothing(self):
        before = self.snapshot()
        rc, out = self.run_tool()
        self.assertEqual(rc, 0, out)
        self.assertIn("report only: 306 rows", out)  # 196 + 50 + 48 + 2 + 2 + 2 + 6
        self.assertIn("Tal'Thir: not on this shard", out)
        self.assertEqual(self.snapshot(), before)

    def test_apply_again_undo(self):
        before = self.snapshot()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT skill, comp, count(*) FROM craftrecipes WHERE source = 'master-craft-1114' GROUP BY 1, 2"),
                         [(1, 10, 1), (2, 18, 196)])
        fe = self.gems["Fire Emerald"]
        self.assertEqual(self.q("SELECT itemcost FROM itempattern WHERE patternid = ?", fe), [(270,)])
        self.assertEqual(self.q("SELECT DISTINCT rarity, difficulty, xp FROM craftrecipes WHERE skill = 3 AND level = 5"), [(2, 54, 375)])
        self.assertEqual(self.q("SELECT DISTINCT itemcost FROM itempattern WHERE itemname LIKE 'Star Ruby piece%'"), [(3 * 290 + 10 + 600,)])
        self.assertEqual(self.q("SELECT weight, minlevel FROM loottables WHERE patternid = ?", fe), [(3, 20)])
        self.assertEqual(self.q("SELECT weight, minlevel FROM lootpool WHERE patternid = ?", fe), [(38, 20)])
        self.assertEqual(self.q("SELECT min_bid, buyout FROM auction_lots WHERE patternid = ?", fe), [(500, 500)])
        self.assertEqual(self.q("SELECT chance, rolls FROM npcloot WHERE npc_name = 'overseer durekar'"), [(25, 1)])
        self.assertEqual(self.q("SELECT count(*) FROM npcloot WHERE tablename = 'l60 gems'"), [(10,)])
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to do", out)
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 0, out)
        self.assertIn("306 rows back", out)
        self.assertEqual(self.snapshot(), before)

    def test_knower_refused_then_raised(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO charrecipes VALUES (7, ?, 3)", (self.cloth[3],))  # a level 10 recipe (k = 3)
        db.execute("INSERT INTO charcraftskills VALUES (7, 2, 4, 5, 120)")
        db.commit()
        db.close()
        rc, out = self.run_tool()
        self.assertEqual(rc, 1, out)
        self.assertIn("character 7 knows a level 10 moved recipe with Tailoring 4", out)
        before = self.snapshot()
        rc, out = self.run_tool("--raise-tailoring", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT level, rank, xp FROM charcraftskills WHERE charid = 7 AND skill = 2"), [(10, 0, 0)])
        self.assertEqual(self.q("SELECT recipe, ord FROM charrecipes WHERE charid = 7"), [(self.cloth[3], 3)])
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.snapshot(), before)

    def test_knower_with_no_tailoring_row_gets_one_and_undo_removes_it(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO charrecipes VALUES (8, ?, 1)", (self.cloth[0],))  # level 7
        db.commit()
        db.close()
        before = self.snapshot()
        rc, out = self.run_tool("--raise-tailoring", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT level FROM charcraftskills WHERE charid = 8 AND skill = 2"), [(7,)])
        self.run_tool("--undo")
        self.assertEqual(self.snapshot(), before)

    def test_skip_and_refusals(self):
        rc, out = self.run_tool("--skip", "move", "--skip", "drops", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT count(*) FROM craftrecipes WHERE skill = 2"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcloot"), [(4,)])
        self.run_tool("--undo")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itempattern SET itemcost = 700 WHERE itemname = 'Star Ruby piece 6 2'")
        db.commit()
        db.close()
        before = self.snapshot()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("value 700", out)
        self.assertEqual(self.snapshot(), before)
        rc, out = self.run_tool("--skip", "nothing")
        self.assertEqual(rc, 1, out)

    def test_undo_leaves_changed_rows(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcloot SET chance = 50 WHERE npc_name = 'overlord xrag'")
        db.commit()
        db.close()
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 1, out)
        self.assertIn("changed since", out)
        self.assertEqual(self.q("SELECT chance FROM npcloot WHERE npc_name = 'overlord xrag'"), [(50,)])


if __name__ == "__main__":
    unittest.main()
