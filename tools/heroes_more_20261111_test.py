#!/usr/bin/env python3
"""heroes_more_20261111.py on a database of its own, made with the server's own table definitions, after
seed_testing_heroes.py's apply (its rows as they are on Heirloom): the report writes nothing; --apply makes the weakened
Breastplate of the Anointed all/all at half and 350, the three Seraphic Planar Breastplates (weakened all/all at half,
the drop, an Ascended record by the rule) at 2000 / 2250 / 2750 and the Heroic Ceremonial Vestments (all/all, +10 to
the seven stats and six resists) at 4000, each row with its gm_audit line; a second apply writes nothing; --undo puts
every row back exactly, the ids handed out too, and is refused while a made record is held or a ware was sold since, or
while a row is not as written. Controls: a weakened Breastplate not the seed's copy, or an import's copy in a base's id,
refuses everything; a server that has the Ascended Seraphic sells that one."""
import os
import sqlite3
import subprocess
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import heroes_more_20261111 as hm  # noqa: E402
import seed_testing_heroes_test as st  # noqa: E402

TOOL = os.path.join(HERE, "heroes_more_20261111.py")
SEED = os.path.join(HERE, "seed_testing_heroes.py")

ITEM = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, "
        "weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, "
        "model, color, str, sta, agi, wis, dex, cha, intelligence, HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR, weaponproc, fish) "
        "VALUES (?, ?, 0, ?, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, 1, ?, 75, ?, 1023, 0, 0, 0, 0, ?, ?, ?, 48895, "
        "?, 0, ?, 0, 0, ?, ?, ?, ?, 0, 0, ?, 0, 0, 0, 0, 0, 0, 0, 0)")
# (id, cost, icon, slot, level, durability, classes, name, words, model, str, agi, cha, int, HP, power, AC): the drops as Heirloom has them
SERAPHIC = (8527, 6553, 337212673, 5, 55, 16000, 8, "Seraphic Planar Breastplate", "Draws on the planar expanse.", 4, 15, 0, 0, 15, 150, 0, 426)
CV = (5192, 2636, 1394465037, 2, 29, 6400, 32766, "Ceremonial Vestments", "A fine vestments of heavy cloth.", 0, 0, 5, 15, 0, 25, 25, 0)


class HeroesMoreTest(unittest.TestCase):
    """setUp is the seed's test's database; each test applies the seed first, then adds the two drops and their imports."""
    db, one, dump = st.SeedTestingHeroesTest.db, st.SeedTestingHeroesTest.one, st.SeedTestingHeroesTest.dump

    def setUp(self):
        st.SeedTestingHeroesTest.setUp(self)
        code, out = self.run_seed("--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path, isolation_level=None)
        for r in (SERAPHIC, CV):
            db.execute(ITEM, r)
            db.execute(ITEM, (990000 + r[0] % 1000,) + r[1:])  # an import's copy: never the base
            db.execute("INSERT INTO itemimported (patternid, base, made, source) VALUES (?, ?, '', 'importdamper')", (990000 + r[0] % 1000, r[0]))
        db.close()

    def run_seed(self, *args):
        p = subprocess.run([sys.executable, "-E", "-s", SEED, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def item(self, pid):
        return self.one("SELECT * FROM itempattern WHERE patternid = ?", (pid,))

    def stock(self):
        return [dict(r) for r in self.db().execute("SELECT s.patternid, s.cost, s.says, s.source, p.itemname FROM heroes_stock s "
                                                    "JOIN itempattern p ON p.patternid = s.patternid ORDER BY s.cost, s.id")]

    def test_the_words_are_the_servers(self):
        """The weakened records' names are their bases' after the server's prefix, and its percent is the tool's."""
        with open(os.path.join(HERE, "..", "internal", "entity", "wornshield.go")) as f:
            go = f.read()
        self.assertIn(f'const WeakenedPrefix = "{hm.WEAKENED_PREFIX}"', go)
        self.assertIn(f"const WeakenedPercent = {hm.WEAKENED_PERCENT}", go)
        self.assertIn(f'"{hm.SERAPHIC_NAME.lower()}": ', go)
        self.assertEqual(hm.WEAK_SERAPHIC_NAME, hm.WEAKENED_PREFIX + hm.SERAPHIC_NAME)

    def test_more(self):
        before = self.dump()
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 9 steps", out)
        self.assertEqual(self.dump(), before, "the report wrote")

        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 10 rows", out)
        stock = self.stock()
        self.assertEqual([(s["itemname"], s["cost"]) for s in stock], [
            ("Elixir of the Giant Gnome", 25), ("Tome of Heroic Training", 100), ("Weakened Breastplate of the Anointed", 350),
            ("Breastplate of the Anointed", 500), ("Ascended Breastplate of the Anointed", 1000),
            ("Weakened Seraphic Planar Breastplate", 2000), ("Seraphic Planar Breastplate", 2250),
            ("Ascended Seraphic Planar Breastplate", 2750), ("Heroic Ceremonial Vestments", 4000)])
        for s in stock:
            self.assertEqual(s["source"], "testing-heroes")
            self.assertLessEqual(len("Testing Heroes Vendor: " + s["says"]), 217, s["says"])
            self.assertLessEqual(len(s["itemname"]) + len(": 4000"), 63)
            self.assertNotIn("%", s["says"])
        self.assertEqual(stock[6]["patternid"], 8527)

        # the weakened two: all/all, the scaled figures half their base's (half up), the rest the base's
        for pid, base in ((hm.WEAK_BPOA, hm.BPOA), (hm.WEAK_SERAPHIC, hm.SERAPHIC)):
            w, b = self.item(pid), self.item(base)
            self.assertEqual((w["classuse"], w["raceuse"]), (32767, 1023))
            self.assertEqual(w["itemname"], "Weakened " + b["itemname"])
            for c in b:
                if c.lower() in hm.asc.SCALED:
                    self.assertEqual(w[c], (b[c] * 50 + 50) // 100 if b[c] > 0 else b[c], c)
                elif c not in ("patternid", "itemname", "itemdesc", "classuse", "raceuse"):
                    self.assertEqual(w[c], b[c], c)
        self.assertEqual([self.item(hm.WEAK_BPOA)[c] for c in ("sta", "HPMAX", "AC", "AR", "maxhp", "lore")], [15, 38, 207, 15, 13400, 1])
        self.assertEqual([self.item(hm.WEAK_SERAPHIC)[c] for c in ("str", "intelligence", "HPMAX", "AC", "maxhp")], [8, 8, 75, 213, 16000])

        # the Ascended Seraphic: the rule's, its itemascended row
        asc_id = stock[7]["patternid"]
        self.assertTrue(hm.asc.FIRST_ID <= asc_id <= hm.asc.LAST_ID)
        up = self.item(asc_id)
        self.assertEqual([up[c] for c in ("str", "intelligence", "HPMAX", "AC", "classuse", "maxhp")], [17, 17, 165, 469, 8, 16000])
        self.assertEqual(self.one("SELECT base FROM itemascended WHERE patternid = ?", (asc_id,))["base"], 8527)

        # the robe: all/all, 10 more of each stat and resist, its look and the rest its base's
        cv, base = self.item(hm.HERO_CV), self.item(hm.CV)
        self.assertEqual((cv["classuse"], cv["raceuse"], cv["itemname"]), (32767, 1023, "Heroic Ceremonial Vestments"))
        for c in base:
            if c.lower() in hm.STATS + hm.RESISTS:
                self.assertEqual(cv[c], base[c] + 10, c)
            elif c not in ("patternid", "itemname", "itemdesc", "classuse", "raceuse"):
                self.assertEqual(cv[c], base[c], c)
        self.assertEqual(self.one("SELECT count(*) n FROM gm_audit WHERE detail LIKE '%heroes_more_20261111%'")["n"], 10)

        # a second apply writes nothing
        applied = self.dump()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)
        self.assertEqual(self.dump(), applied)

        # an undo while a made record is held, or after a sale of a ware: refused, nothing done
        db = self.db()
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (7, 1, 0, 0, ?, -1, 0, 0)", (hm.HERO_CV,))
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn(f"1 of record {hm.HERO_CV} in charinventory", out)
        db.execute("DELETE FROM charinventory")
        db.execute("INSERT INTO heroes_purchases (at, serverid, patternid, cost, standing_before, standing_after) VALUES ('x', 7, ?, 350, -1500, -1850)",
                   (hm.WEAK_BPOA,))
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("1 purchases of these wares since the apply", out)
        db.execute("DELETE FROM heroes_purchases")
        # a GM's edit since: refused and said
        db.execute("UPDATE heroes_stock SET cost = 3000 WHERE patternid = ?", (hm.HERO_CV,))
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("heroes_stock", out)
        db.execute("UPDATE heroes_stock SET cost = 4000 WHERE patternid = ?", (hm.HERO_CV,))
        db.close()

        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertIn("undone: 10 rows", out)
        self.assertEqual(self.dump(), before)

    def test_refused_where_the_seed_rows_are_not_as_it_wrote(self):
        db = self.db()
        db.execute("UPDATE itempattern SET AC = 1 WHERE patternid = ?", (hm.WEAK_BPOA,))
        db.close()
        before = self.dump()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("is not the seed's copy of 1076 (differs in AC)", out)
        self.assertEqual(self.dump(), before)

    def test_refused_where_a_base_is_a_copy(self):
        db = self.db()
        db.execute("INSERT INTO itemimported (patternid, base, made, source) VALUES (8527, 1, '', 'x')")
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("itempattern 8527 is a copy (itemimported)", out)

    def test_sells_the_ascended_seraphic_the_server_has(self):
        db = self.db()
        db.execute(ITEM, (980500,) + SERAPHIC[1:7] + ("Ascended Seraphic Planar Breastplate",) + SERAPHIC[8:])
        db.execute("INSERT INTO itemascended (patternid, base, made, source) VALUES (980500, 8527, '', 'ascend_boss_loot')")
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("record 980500: sold as it is", out)
        self.assertEqual(self.one("SELECT cost FROM heroes_stock WHERE patternid = 980500")["cost"], 2750)
        self.assertEqual(self.one("SELECT count(*) n FROM itemascended WHERE base = 8527")["n"], 1)


if __name__ == "__main__":
    unittest.main()
