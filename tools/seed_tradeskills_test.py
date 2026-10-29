#!/usr/bin/env python3
"""tools/seed_tradeskills.py on made-up sources in eqoatrade's and the
harvest's shapes (no game data): every grid page's cells become products and
recipes (the ore of the trade level, the piece's mold, the modifier kept on a
failure), named and leveled by Prima's rules; a live crafted record of the
name, level and slot is the product, another item of the name is not; the
suppliers' stand-in goods give way to the craft's wares; the modifiers drop
from every creature; --undo gives back every table as it was. The control: a
magical cell whose AC is not the mundane one's is a problem, and --apply
writes nothing.

    python3 tools/seed_tradeskills_test.py
"""
import hashlib
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_tradeskills.py")
sys.path.insert(0, HERE)
import seed_tradeskills as M  # noqa: E402

ITEM_COLS = ("patternid INTEGER PRIMARY KEY, itemcost INTEGER, unk1 INTEGER, itemicon INTEGER, unk2 INTEGER, equipslot INTEGER, "
             "unk3 INTEGER, trade INTEGER, rent INTEGER, unk4 INTEGER, attacktype INTEGER, weapondamage INTEGER, unk5 INTEGER, "
             "levelreq INTEGER, maxstack INTEGER, maxhp INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER, "
             "procanim INTEGER, lore INTEGER, unk6 INTEGER, craft INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER NOT NULL, "
             "color INTEGER NOT NULL, str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER, "
             "intelligence INTEGER, HPMAX INTEGER, POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER, DR INTEGER, "
             "FR INTEGER, CR INTEGER, LR INTEGER, AR INTEGER, weaponproc INTEGER, fish INTEGER")


def page(title, rows):
    """A grid page: a title, the header, a row a modifier (cells by trade level)."""
    levels = sorted({lvl for cells in rows.values() for lvl in cells})
    tr = [f"<tr><td><b>{title}</b></td></tr>", "<tr><td>Gem</td>" + "".join(f"<td>Level {lvl}</td>" for lvl in levels) + "</tr>"]
    for mod, cells in rows.items():
        tr.append(f"<tr><td>{mod}</td>" + "".join(f"<td>{cells.get(lvl, 'N/C')}</td>" for lvl in levels) + "</tr>")
    return "<html><table>" + "\n".join(tr) + "</table></html>"


def ware(i, name, value, wear=-1, level=1, craft=1, stack=99):
    return {"id": i, "name": name, "value": value, "p08": 0, "icon": 555, "icon2": 0, "wear_slot": wear, "p18": 0, "no_trade": 0,
            "rent": 0, "p24": 0, "attack_type": 0, "damage": 0, "range": 0, "level_req": level, "max_stack": stack, "max_hp": 0,
            "durability": 0, "class_mask": 0, "race_mask": 0, "p4c": 0, "lore": 0, "p58": 0, "p54": craft, "desc": "", "stats": []}


class SeedTradeskills(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        d = self.dir.name
        self.sources = os.path.join(d, "sources")
        grid = os.path.join(self.sources, "eqoatrade", "Recipes", "Armor", "Original")
        for key in M.PIECES:
            os.makedirs(os.path.dirname(os.path.join(grid, key)), exist_ok=True)
            rows = {M.MUNDANE: {1: "15 AC", 4: "85 AC", 10: "290 AC"}, "Sinhalite": {4: "85 AC, 10 Str"},
                    "Heartsblood Alloy": {10: "290 AC, 10 HoT"}}
            if key == "Plate/BP.htm":
                self.bp = rows
            with open(os.path.join(grid, key), "w") as f:
                f.write(page("Plate Breastplates (Level 1 - 10)", rows))
        vend = os.path.join(self.sources, "eqoatrade", "Vendors")
        os.makedirs(vend)
        with open(os.path.join(vend, "Armor.htm"), "w") as f:
            f.write("<p>Testport - Near the Blacksmith<br>Tutorial - Smithy Anvil<br>Hammers - Merchant Hammer<br>"
                    "Molds and Patterns - Merchant Mold<br>Metal Blocks - Mechant Ore (in the yard)</p>"
                    "<p>Nowhere - By the Blacksmith<br>Hammers - ????</p>")
        wares = [ware(20304 + k, f"{m} Ore", 10 + k) for k, m in enumerate(M.METALS)]
        wares += [ware(20400 + k, f"{h} Armorsmith Hammer", 100 * (k + 1), wear=23, level=1 + 5 * k, craft=0, stack=1)
                  for k, h in enumerate(M.HAMMERS)]
        molds = [m for _, _, _, m in M.PIECES.values()] + [m for _, _, _, m in M.EXTRA_SHIELDS.values()]
        wares += [ware(27475 + k, m, 10) for k, m in enumerate(molds)]
        self.harvest = os.path.join(d, "harvest.json")
        with open(self.harvest, "w") as f:
            json.dump({"character": {"world": 0}, "merchants": [{"name": "Merchant Mold", "wares": wares}]}, f)
        with open(os.path.join(d, "not-a-harvest.json"), "w") as f:
            json.dump({"something": "else"}, f)
        self.db = os.path.join(d, "t.db")
        db = sqlite3.connect(self.db)
        db.execute(f"CREATE TABLE itempattern ({ITEM_COLS})")
        # a modifier here already; a live crafted record; a found item of a product's name and another level;
        # Sandstorm's id of a hammer held here by another item
        rows = [(1116, "heartsblood alloy", 1, 0, -1, 0, 50, 0), (1042, "heartsblood seralite breastplate", 0, 48, 5, 378, 1, 525),
                (1920, "Crude Iron Breastplate", 0, 30, 5, 99, 1, 525), (20400, "Forgehammer", 0, 1, 12, 0, 1, 0),
                (500, "rusty smithing hammer", 0, 1, 18, 0, 1, 0)]
        for pid, name, craft, lvl, slot, ac, stack, cls in rows:
            db.execute("INSERT INTO itempattern (patternid, itemname, craft, levelreq, equipslot, AC, maxstack, classuse, model, "
                       "color, itemicon, itemcost) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 77, 5)", (pid, name, craft, lvl, slot, ac, stack, cls))
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_id INTEGER, npc_type INTEGER)")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?)", [(1, "Smithy Anvil", 1, 128), (2, "Merchant Hammer", 2, 129),
                                                                 (3, "Merchant Mold", 0, 128), (4, "Merchant Ore", 4, 129)])
        db.execute("CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, stackleft INTEGER, remainHP INTEGER, "
                   "remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER)")
        db.execute("INSERT INTO npcitem VALUES (10, 2, 1, 0, 0, 500, -1, 1, 0)")   # a stand-in
        db.execute("INSERT INTO npcitem VALUES (11, 4, 1, 0, 0, 20304, -1, 1, 0)")  # Sandstorm's: kept
        db.execute("CREATE TABLE merchant_stock (what TEXT, key INTEGER, old TEXT, new TEXT, note TEXT)")
        db.execute("INSERT INTO merchant_stock VALUES ('catalog', 10, NULL, '2,500,0', 'general goods')")
        db.execute("INSERT INTO merchant_stock VALUES ('catalog', 11, NULL, '4,20304,0', 'Sandstorm''s catalog')")
        db.execute("CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER, "
                   "minlevel INTEGER, maxlevel INTEGER, source TEXT)")
        db.execute("CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT, "
                   "PRIMARY KEY (npc_name, tablename))")
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def run_seed(self, *args, ok=True):
        return self.tool("--sources", self.sources, "--harvests", self.dir.name, *args, ok=ok)

    def q(self, sql):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql).fetchall()
        finally:
            c.close()

    def snapshot(self):
        c = sqlite3.connect(self.db)
        try:
            out = {}
            for (t,) in c.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"):
                out[t] = hashlib.sha256(repr(c.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall()).encode()).hexdigest()
            return out
        finally:
            c.close()

    def test_apply_and_undo(self):
        before = self.snapshot()
        out = self.run_seed().stdout
        self.assertIn("report only", out)
        self.assertIn("'heartsblood seralite breastplate' is here already (1042)", out)
        self.run_seed("--apply")
        # a mundane trade level 1 breastplate: Crude Iron Ore + the breastplate's mold, banded, tan, level 3
        (pid, lvl, slot, model, color, ac, cls, maxhp, dur), = self.q(
            "SELECT patternid, levelreq, equipslot, model, color, AC, classuse, maxhp, duration FROM itempattern "
            "WHERE itemname = 'crude iron breastplate'")
        self.assertEqual((lvl, slot, model, color, ac, cls, maxhp, dur), (3, 5, 5, 11, 15, 525, 650, 51))
        self.assertNotEqual(pid, 1920, "the found item of the name and another level is not the product")
        (comp, level, rarity, ing), = self.q(f"SELECT comp, level, rarity, ingredients FROM craftrecipes WHERE product = {pid}")
        bp_mold = self.q("SELECT patternid FROM itempattern WHERE itemname = 'Plate Breastplate Mold'")[0][0]
        self.assertEqual((comp, level, rarity, json.loads(ing)),
                         (0, 1, 0, [{"item": 20304, "count": 1, "keep": False}, {"item": bp_mold, "count": 1, "keep": False}]))
        # a magical one: the modifier kept on a failure, its stat, scale/plate from trade level 6
        (ac, st), = self.q("SELECT AC, str FROM itempattern WHERE itemname = 'sinhalite brass breastplate'")
        self.assertEqual((ac, st), (85, 10))
        sinh = self.q("SELECT patternid FROM itempattern WHERE itemname = 'Sinhalite'")[0][0]
        ing = json.loads(self.q("SELECT ingredients FROM craftrecipes WHERE name = 'sinhalite brass breastplate'")[0][0])
        self.assertEqual(ing[-1], {"item": sinh, "count": 1, "keep": True})
        self.assertEqual(self.q("SELECT model FROM itempattern WHERE itemname = 'seralite scale coif'"), [(7,)])
        # the live record is the product
        self.assertEqual(self.q("SELECT product, rarity FROM craftrecipes WHERE name = 'heartsblood seralite breastplate'"), [(1042, 3)])
        # the targe and tear by Prima's AC rows; a tear has none below trade level 5
        self.assertEqual(self.q("SELECT AC FROM itempattern WHERE itemname = 'crude iron targe shield'"), [(10,)])
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE itemname = 'crude iron tear shield'"), [(0,)])
        # Sandstorm's ids where free; the hammer whose id is taken here gets another
        self.assertEqual(self.q("SELECT itemname FROM itempattern WHERE patternid = 20313"), [("Seralite Ore",)])
        novice = self.q("SELECT patternid FROM itempattern WHERE itemname = 'Novice Armorsmith Hammer'")[0][0]
        self.assertNotEqual(novice, 20400)
        # the stand-in goods gone, Sandstorm's kept, the craft's wares in: hammers, molds, ores
        self.assertEqual(self.q("SELECT count(*) FROM npcitem WHERE itemid = 10"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcitem WHERE itemid = 11"), [(1,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcitem WHERE npcid = 2"), [(10,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcitem WHERE npcid = 3"), [(17,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcitem WHERE npcid = 4"), [(10,)])
        self.assertEqual(self.q("SELECT npc_id, npc_type FROM npcs WHERE id = 3"), [(3, 129)])
        # the starter, by its scroll alone
        (scroll, disc, ing), = self.q("SELECT scroll, discover, ingredients FROM craftrecipes WHERE name = \"blacksmith's bracers\"")
        self.assertEqual(disc, 0)
        self.assertEqual(self.q(f"SELECT itemname FROM itempattern WHERE patternid = {scroll}"), [("Blacksmith's Bracers Recipe",)])
        self.assertEqual(len(json.loads(ing)), 3)
        # the modifiers' drops
        self.assertEqual(self.q("SELECT count(*) FROM loottables WHERE tablename = 'craft modifiers'"), [(22,)])
        self.assertEqual(self.q("SELECT chance, rolls FROM npcloot WHERE npc_name = '*'"), [(M.LOOT_CHANCE, 1)])
        self.assertNotEqual(self.run_seed("--apply", ok=False).returncode, 0, "a second --apply is refused")
        self.tool("--undo")
        after = self.snapshot()
        after.pop("craftrecipes")  # the engine's table, made empty and left
        self.assertEqual(after, before)

    def test_a_cell_off_the_rule_writes_nothing(self):
        path = os.path.join(self.sources, "eqoatrade", "Recipes", "Armor", "Original", "Plate/BP.htm")
        rows = dict(self.bp)
        rows["Sinhalite"] = {4: "86 AC, 10 Str"}  # not the mundane AC
        with open(path, "w") as f:
            f.write(page("Plate Breastplates (Level 1 - 10)", rows))
        before = self.snapshot()
        r = self.run_seed("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("is not the mundane one's", r.stdout)
        self.assertEqual(self.snapshot(), before)


if __name__ == "__main__":
    unittest.main()
