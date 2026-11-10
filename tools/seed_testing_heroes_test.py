#!/usr/bin/env python3
"""seed_testing_heroes.py on a database of its own, made with the server's own table definitions (read out of
internal/database's Go source) and the rows the tool looks for (the Breastplate of the Anointed 1076, Dabrish by the
well, Merchant Kari, a neighbour on the first spot's far side): the report writes nothing; --apply writes the faction
at the floor, the three new records and the Ascended one by the Ascended records' rule, the two use lines, the five
stock rows at the petition's costs and the vendor on the first free spot, every row with its gm_audit line; a second
apply is refused; --undo puts everything back exactly, the ids too; an undo is refused while a character holds a record
or anything was sold, or a row is not as written (a GM's edit since). The controls: each thing there already, a 1076 that is not
the Breastplate, a Dabrish away from the well and no free spot refuse everything; a server that has the Ascended record
already sells that one."""
import glob
import json
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_testing_heroes as sh  # noqa: E402

TOOL = os.path.join(HERE, "seed_testing_heroes.py")
GO = os.path.join(HERE, "..", "internal", "database")


SOURCES = {}


def schema(name):
    """The server's CREATE TABLE of a table, out of internal/database's Go source."""
    if not SOURCES:
        for path in sorted(glob.glob(os.path.join(GO, "*.go"))):
            if not path.endswith("_test.go"):
                with open(path) as f:
                    SOURCES[path] = f.read()
    for text in SOURCES.values():
        m = re.search(r"CREATE TABLE IF NOT EXISTS \"?" + name + r"\"?\s*\(.*?\n\s*\)[;`]", text, re.S)
        if m:
            return m.group(0).rstrip("`;")
    raise AssertionError(f"no schema of {name} in {GO}")


TABLES = ("factions", "itempattern", "itemeffect", "itemascended", "itemimported", "npcs", "npcmove", "npcaggro", "charinventory",
          "heroes_purchases", "gm_audit")
DUMPED = ("factions", "itempattern", "itemeffect", "itemascended", "npcs", "npcmove", "npcaggro", "sqlite_master", "sqlite_sequence")


def dump(db):
    """Every table the tool touches, the tables there are and the ids handed out, for an exact compare."""
    out = {}
    for t in DUMPED + (("heroes_stock",) if sh.table(db, "heroes_stock") else ()):
        where = " WHERE name NOT IN ('gm_audit', 'heroes_purchases', 'charinventory')" if t == "sqlite_sequence" else ""
        cur = db.execute(f"SELECT * FROM {t}{where}")
        cols = [d[0] for d in cur.description]
        out[t] = sorted(json.dumps(dict(zip(cols, r)), sort_keys=True, default=str) for r in cur.fetchall())
    return out


class SeedTestingHeroesTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        for t in TABLES:
            db.execute(schema(t))
        x = db.execute
        x("INSERT INTO factions (name, standing) VALUES ('Citizens of Freeport', 1)")
        item = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, "
                "weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, "
                "model, color, str, sta, agi, wis, dex, cha, intelligence, HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR, weaponproc, fish) "
                "VALUES (?, 4375, 0, 337212673, 0, 5, 0, 0, 1, 0, 0, 0, 0, 60, 1, 13400, 75, 516, 415, 6, 1, 0, 0, ?, 'Made by Marr.', 4, 4294967295, "
                "0, 30, 0, 0, 0, 0, 0, 75, 0, 0, 0, 414, 0, 0, 0, 0, 0, 30, 0, 0)")
        x(item, (sh.BASE, sh.BASE_NAME))
        x(item, (990165, sh.BASE_NAME))  # an import's copy of it: left alone
        x("INSERT INTO itemimported (patternid, base, made, source) VALUES (990165, ?, '', 'importdamper')", (sh.BASE,))
        x("INSERT INTO sqlite_sequence (name, seq) VALUES ('itempattern', 990983)")
        npc = ("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, npc_id, race, zone, hair_color) "
               "VALUES (?, ?, 0, ?, ?, ?, ?, ?, 1.0, ?, ?, ?, '0', 0, ?)")
        x(npc, (583, sh.TEMPLATE, 25351.5, 54.4, 15721.0, 150, 1893243078, 15, 129, 583, 2))
        x(npc, (162568, sh.BESIDE, 25306.098, 54.12597, 15751.569, 61, 1640644319, 60, 128, 162568, 2))
        x(npc, (162664, "Mithaniel Marr", 25294.8, 54.094, 15756.2, 0, 77, 60, 128, 162664, 0))
        db.close()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def db(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.row_factory = sqlite3.Row
        return db

    def one(self, sql, args=()):
        db = self.db()
        try:
            r = db.execute(sql, args).fetchone()
            return None if r is None else dict(r)
        finally:
            db.close()

    def dump(self):
        db = self.db()
        try:
            return dump(db)
        finally:
            db.close()

    def test_report_apply_and_undo(self):
        before = self.dump()
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(self.dump(), before, "the report wrote")

        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 16 rows", out)
        self.assertEqual(self.one("SELECT standing FROM factions WHERE name = 'Testing Heroes'")["standing"], -2000)

        # the stock: the petition's five costs, cheapest first, each record there
        stock = [dict(r) for r in self.db().execute("SELECT s.patternid, s.cost, s.says, s.source, p.itemname FROM heroes_stock s "
                                                    "JOIN itempattern p ON p.patternid = s.patternid ORDER BY s.cost")]
        self.assertEqual([s["cost"] for s in stock], [25, 100, 250, 500, 1000])
        asc_id = stock[4]["patternid"]
        self.assertEqual([s["patternid"] for s in stock[:4]], [sh.ELIXIR, sh.TOME, sh.WEAKENED, sh.BASE])
        self.assertEqual([s["itemname"] for s in stock], ["Elixir of the Giant Gnome", "Tome of Heroic Training",
                                                          "Weakened Breastplate of the Anointed", "Breastplate of the Anointed",
                                                          "Ascended Breastplate of the Anointed"])
        for s in stock:
            self.assertEqual(s["source"], "testing-heroes")
            # a box holds 217 characters, his name before them
            self.assertLessEqual(len("Testing Heroes Vendor: " + s["says"]), 217, s["says"])
            self.assertLessEqual(len(s["itemname"]) + len(": 1000"), 63)

        # the use items: one a record, never a scroll, the elixir of five charges and the gnome's body, the tome of one
        for pid in (sh.ELIXIR, sh.TOME):
            r = self.one("SELECT * FROM itempattern WHERE patternid = ?", (pid,))
            self.assertEqual((r["equipslot"], r["maxstack"], r["lore"], r["rent"], r["levelreq"]), (-1, 1, 0, 1, 1))
            self.assertNotIn(r["itemicon"], (-607649328, 3687317968))
        e = self.one("SELECT * FROM itemeffect WHERE patternid = ?", (sh.ELIXIR,))
        self.assertEqual((e["line"], e["charges"], e["body"], e["breaks"], e["source"]),
                         ("Self, Body Size 4, 30min", 5, 2845600533, "", "testing-heroes"))
        t = self.one("SELECT * FROM itemeffect WHERE patternid = ?", (sh.TOME,))
        self.assertEqual((t["line"], t["charges"], t["body"]), ("Self, Increase Training Points by 5", 1, 0))

        # the Weakened: the Breastplate's record but its id, name and words
        base = self.one("SELECT * FROM itempattern WHERE patternid = ?", (sh.BASE,))
        weak = self.one("SELECT * FROM itempattern WHERE patternid = ?", (sh.WEAKENED,))
        for c in base:
            if c not in ("patternid", "itemname", "itemdesc"):
                self.assertEqual(weak[c], base[c], c)
        self.assertEqual(weak["itemname"], "Weakened Breastplate of the Anointed")
        self.assertEqual(base["itemname"], "Breastplate of the Anointed")

        # the Ascended: the rule's figures, its itemascended row, an id of the Ascended records'
        up = self.one("SELECT * FROM itempattern WHERE patternid = ?", (asc_id,))
        self.assertTrue(sh.asc.FIRST_ID <= asc_id <= sh.asc.LAST_ID)
        self.assertEqual((up["sta"], up["HPMAX"], up["AC"], up["AR"], up["str"], up["lore"], up["equipslot"]), (33, 83, 455, 33, 0, 1, 5))
        self.assertEqual(self.one("SELECT base FROM itemascended WHERE patternid = ?", (asc_id,))["base"], sh.BASE)

        # the vendor: on the first spot Mithaniel Marr leaves free, Dabrish's facing and height, Kari's look
        v = self.one("SELECT * FROM npcs WHERE npc_name = 'Testing Heroes Vendor'")
        self.assertEqual((v["world"], v["x_coord"], v["z_coord"], v["y_coord"], v["facing"], v["npc_type"], v["npc_level"], v["npc_id"]),
                         (0, 25299.5, 15755.5, 54.12597, 61, 128, 60, v["id"]))
        self.assertEqual((v["modelid"], v["hair_color"]), (1893243078, 2))
        self.assertIn("spot (25299.5, 15755.5): free", out)
        self.assertEqual(self.one("SELECT roam FROM npcmove WHERE name = 'testing heroes vendor'")["roam"], 0)
        self.assertEqual(self.one("SELECT aggressive FROM npcaggro WHERE npc_name = 'Testing Heroes Vendor'")["aggressive"], 0)
        self.assertEqual(self.one("SELECT count(*) n FROM gm_audit WHERE action = 'insert'")["n"], 16)

        # a second apply
        applied = self.dump()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("applied already", out)
        self.assertEqual(self.dump(), applied)

        # an undo while a character holds a record made here, or after a sale: refused, nothing done
        db = self.db()
        db.execute("PRAGMA foreign_keys = OFF")
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (7, 1, 0, 5, ?, -1, 0, 0)", (sh.ELIXIR,))
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn(f"1 of record {sh.ELIXIR} held by characters", out)
        self.assertEqual(self.dump(), applied)
        db.execute("DELETE FROM charinventory")
        db.execute("INSERT INTO heroes_purchases (at, serverid, patternid, cost, standing_before, standing_after) VALUES ('x', 7, ?, 25, -1975, -2000)",
                   (sh.ELIXIR,))
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("1 purchases in heroes_purchases", out)
        db.execute("DELETE FROM heroes_purchases")

        # a row a GM changed since: refused and said, nothing done; put back, the undo is exact
        db.execute("UPDATE heroes_stock SET cost = 30 WHERE patternid = ?", (sh.ELIXIR,))
        changed = self.dump()
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("heroes_stock", out)
        self.assertIn("changed since it was written", out)
        self.assertEqual(self.dump(), changed)
        db.execute("UPDATE heroes_stock SET cost = 25 WHERE patternid = ?", (sh.ELIXIR,))
        db.close()
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.dump(), before, "the undo is not exact")
        code, out = self.run_tool("--undo")
        self.assertEqual((code, out.strip()), (0, "nothing to undo"))

    def refused(self, why):
        before = self.dump()
        for arg in ((), ("--apply",)):
            code, out = self.run_tool(*arg)
            self.assertEqual(code, 1, out)
            self.assertIn("refused, nothing done", out)
            self.assertIn(why, out)
            self.assertEqual(self.dump(), before)

    def test_refusals(self):
        cases = [
            ("INSERT INTO factions (name, standing) VALUES ('testing heroes', 0)", "factions has 'Testing Heroes' already"),
            (f"UPDATE itempattern SET itemname = 'Breastplate of Somebody' WHERE patternid = {sh.BASE}", "not 'Breastplate of the Anointed'"),
            (f"INSERT INTO itemascended (patternid, base) VALUES ({sh.BASE}, 5)", "is a copy (itemascended)"),
            (f"INSERT INTO itemeffect (patternid, line) VALUES ({sh.TOME}, 'Self, Sow, 1min')", f"itemeffect {sh.TOME} is taken"),
            (f"UPDATE itempattern SET patternid = {sh.WEAKENED} WHERE patternid = 990165", f"itempattern {sh.WEAKENED} is taken"),
            ("UPDATE itempattern SET itemname = 'elixir of the giant gnome' WHERE patternid = 990165", "a record named 'Elixir of the Giant Gnome' is there already"),
            ("UPDATE npcs SET npc_name = 'Testing Heroes Vendor' WHERE id = 162664", "a Testing Heroes Vendor stands already"),
            ("UPDATE npcs SET x_coord = 25400 WHERE npc_name = 'Dabrish'", "not within 25 of the well"),
            ("UPDATE npcs SET world = 3 WHERE npc_name = 'Dabrish'", "not within 25 of the well"),
            ("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone) "
             "SELECT 'Dabrish', 0, 25306, 54, 15751, 0, 1, 1.0, 60, 128, '0', 0", "2 rows named Dabrish"),
            ("DELETE FROM npcs WHERE npc_name = 'Merchant Kari'", "no row named Merchant Kari"),
            ("INSERT INTO npcmove (name) VALUES ('testing heroes vendor')", "has an npcmove or npcaggro row already"),
        ]
        for sql, why in cases:
            with self.subTest(why=why):
                self.setUp()
                db = self.db()
                db.execute(sql)
                db.close()
                self.refused(why)

    def test_no_free_spot_and_the_next_spot(self):
        db = self.db()
        npc = ("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone) "
               "VALUES (?, 0, ?, 54, ?, 0, 1, 1.0, 60, 128, '0', 0)")
        x, z = sh.SPOTS[0]
        db.execute(npc, ("Somebody", x + 1, z + 1))
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn(f"spot ({x}, {z}): Somebody stands 1.4 from it", out)
        self.assertIn(f"spot ({sh.SPOTS[1][0]}, {sh.SPOTS[1][1]}): free", out)
        self.assertIn(f"Testing Heroes Vendor at world 0 ({sh.SPOTS[1][0]}, ", out)
        for i, (x, z) in enumerate(sh.SPOTS[1:]):
            db.execute(npc, (f"Somebody {i}", x, z))
        db.close()
        self.refused("no spot by the well is free")

    def test_an_ascended_record_already_there_is_sold(self):
        db = self.db()
        db.execute("INSERT INTO itempattern SELECT 980007, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, "
                   "weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, "
                   "'Ascended ' || itemname, itemdesc, model, color, str, 33, agi, wis, dex, cha, intelligence, 83, POWMAX, PoT, HoT, 455, PR, DR, "
                   "FR, CR, LR, 33, weaponproc, fish FROM itempattern WHERE patternid = ?", (sh.BASE,))
        db.execute("INSERT INTO itemascended (patternid, base, made, source) VALUES (980007, ?, '', 'a quest')", (sh.BASE,))
        db.close()
        before = self.dump()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("record 980007 already: sold as it is", out)
        self.assertIn("applied: 14 rows", out)
        self.assertEqual(self.one("SELECT cost FROM heroes_stock WHERE patternid = 980007")["cost"], 1000)
        self.assertEqual(self.one("SELECT count(*) n FROM itemascended")["n"], 1)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.dump(), before)
        self.assertIsNotNone(self.one("SELECT 1 FROM itempattern WHERE patternid = 980007"), "the server's own Ascended record went with the undo")

    def test_a_server_with_the_stock_table_keeps_it(self):
        db = self.db()
        db.execute(schema("heroes_stock"))
        db.execute("INSERT INTO heroes_stock (patternid, cost, says) VALUES (4242, 7, 'a GM''s')")
        db.close()
        before = self.dump()
        self.assertEqual(self.run_tool("--apply")[0], 0)
        self.assertEqual(self.one("SELECT count(*) n FROM heroes_stock")["n"], 6)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.dump(), before)

    def test_the_stock_schema_is_the_servers(self):
        norm = lambda s: re.sub(r"\s+", " ", s).strip()  # noqa: E731
        self.assertEqual(norm(sh.STOCK_SCHEMA), norm(schema("heroes_stock")))

    def test_the_words_are_the_servers(self):
        """What the server reads by words: the faction's name, the vendor's, the Weakened record's name (its heal's
        key), the box's mark of characters, and the two use lines' clauses."""
        def src(*p):
            with open(os.path.join(HERE, "..", "internal", *p)) as f:
                return f.read()
        self.assertIn(f'const HeroesFaction = "{sh.FACTION}"', src("database", "heroes.go"))
        self.assertIn(f'const HeroesVendorName = "{sh.VENDOR}"', src("handlers", "heroes.go"))
        self.assertIn(f'"{sh.WEAKENED_NAME.lower()}": anointed.weakened()', src("entity", "wornshield.go"))
        self.assertRegex(sh.ELIXIR_LINE.lower().split(", ")[1], r"^(?:body|form)\s+size\s+(\d+(?:\.\d+)?)$")
        self.assertIn(r"^(?:body|form)\s+size\s+(\d+(?:\.\d+)?)$", src("entity", "spelltext.go"))
        self.assertRegex(sh.TOME_LINE.lower().split(", ")[1], r"^(?:increase|raise)\s+(?:unspent\s+)?training\s+points\s+by\s+(\d+)$")
        self.assertIn(r"^(?:increase|raise)\s+(?:unspent\s+)?training\s+points\s+by\s+(\d+)$", src("entity", "spelltext.go"))
        self.assertEqual(sh.FLOOR, -2000)
        self.assertIn("FactionMin = -2000", re.sub(r"\s+", " ", src("entity", "faction.go")))


if __name__ == "__main__":
    unittest.main()
