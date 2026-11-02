import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
STATS = ["str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
         "PR", "DR", "FR", "CR", "LR", "AR", "weaponproc", "fish"]
NOT_NULL = ["itemcost", "unk1", "itemicon", "unk2", "equipslot", "unk3", "trade", "rent", "unk4", "attacktype", "weapondamage",
            "unk5", "levelreq", "maxstack", "maxhp", "duration", "classuse", "raceuse", "procanim", "lore", "unk6", "craft",
            "itemname", "itemdesc", "model", "color"]

ITEM_LIST = """'''ITEM LIST'''
'''Slashing Weapons'''
item Level Dam Att Equip HP Dur Race Class bonus Trade Found
Test Glaive - 20 232 2HP 2Hand 4000 50 All Wa,Ra,P,Sk none yes Halas - Yelda
'''Ranged Weapons'''
item Level Dam Rng Equip HP Dur Race Class bonus Trade Found
Plain Shortbow - 12 139 25 Bow 2400 50 All Wa,Ra none Yes Halas - Nobody
'''HEAD Armor'''
item level AC Equip HP Dur Race Class Bonus Trade Found
QUEST CAP - 15 42 Head ???? 50 De Wa Str+4,Dex_10 No level 15 Quest (once)
Mystery Cap - 18 10 Head ???? 50 All Mg,N,E,Wz Sta/Int+7,Pow+25 Yes ?????
Iron Cap - 25 75 Head 5000 50 All Wa,P,Sk,C none Yes Halas - Yelda
'''Rings'''
item level AC Equip HP Dur Race Class Bonus Trade Found
Bandit Band - 26 0 Ring ???? 50 All All LR/FR+11 Yes Jared's Blight - Bandit
Rusty Hachet - 3 30 Ring 600 50 All All none Yes Halas - Yelda
"""


def page(text, *cats):
    return {"text": text, "cats": list(cats)}


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class ImportWikiItems(unittest.TestCase):
    """tools/import_wiki_items.py on a made-up wiki and database: Item List lines, an item page, a set row and
    a Sandstorm record made; an item of ours under the wiki's spacing, typo or Classic name not made; a name
    in a loot list alone left; each placed where the wiki says (a Halas vendor's or Sandstorm's merchant's
    catalog, a named creature's table, a creature type's table bound to that type's creatures of its levels,
    a region's pool, every region's pool) or nowhere (NO TRADE); an undo deletes all of it. No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        # the server's own schema: every column but the stats NOT NULL, no defaults
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, " + ", ".join(
            f"{c} {'TEXT' if c in ('itemname', 'itemdesc') else 'INTEGER'} NOT NULL" for c in NOT_NULL) + ", " +
            ", ".join(f"{c} INTEGER" for c in STATS) + ")")
        ours = [  # patternid, name, slot, model, icon, type, dmg, level, classes, AC
            (1, "Cloth Cap", 1, 1, 11, 0, 0, 5, 32767, 2), (2, "Silk Cap", 1, 1, 11, 0, 0, 10, 32767, 4),
            (3, "Leather Cap", 1, 2, 12, 0, 0, 10, 17407, 8), (4, "Hide Cap", 1, 2, 12, 0, 0, 12, 17407, 9),
            (5, "Plate Helm", 1, 4, 14, 0, 0, 30, 525, 40), (6, "Steel Helm", 1, 4, 14, 0, 0, 35, 525, 44),
            (7, "Plate Breastplate", 5, 4, 54, 0, 0, 30, 525, 100), (8, "Steel Breastplate", 5, 4, 54, 0, 0, 35, 525, 110),
            (9, "Iron Spear", 15, 900, 91, 6, 200, 18, 15, 0), (10, "Oak Spear", 15, 900, 91, 6, 210, 19, 15, 0),
            (11, "Gold Ring", 8, 0, 81, 0, 0, 20, 32767, 0), (12, "Silver Ring", 8, 0, 81, 0, 0, 22, 32767, 0),
            (13, "Long Bow", 16, 700, 71, 7, 150, 14, 3, 0), (14, "Short Bow", 16, 700, 71, 7, 120, 10, 3, 0),
            (20, "zombieback tunic", 5, 2, 52, 0, 0, 30, 16, 110), (21, "Rusty Hatchet", 8, 0, 81, 0, 0, 3, 32767, 0),
            (22, "Iron Helm", 1, 4, 14, 0, 0, 25, 525, 75),
            (23, "Iron Cleaver", 12, 800, 61, 1, 100, 20, 13, 0), (24, "Steel Cleaver", 12, 800, 61, 1, 150, 25, 13, 0),
        ]
        for pid, name, slot, model, icon, at, dmg, lv, cls, ac in ours:
            row = dict.fromkeys(NOT_NULL, 0)
            row.update(itemcost=100, itemicon=icon, equipslot=slot, rent=1, attacktype=at, weapondamage=dmg, levelreq=lv,
                       maxstack=1, classuse=cls, raceuse=1023, itemname=name, itemdesc="", model=model)
            db.execute(f"INSERT INTO itempattern (patternid, {', '.join(row)}, AC) VALUES (?, {', '.join('?' * len(row))}, ?)",
                       (pid, *row.values(), ac))
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_id INTEGER, npc_type INTEGER, npc_level INTEGER)")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
            (1, "Smithy Yelda", 501, 129, 50), (2, "Bowyer Derlask", 502, 129, 50), (3, "Brute Ogg", 0, 0, 30),
            (4, "bandit", 0, 0, 25), (5, "decaying skeleton", 0, 0, 18), (6, "decaying skeleton", 0, 0, 30),
            (7, "rotting corpse", 0, 0, 17), (8, "living spirit", 0, 0, 18), (9, "a soul eater", 0, 0, 18)])
        db.execute("CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, stackleft INTEGER, remainHP INTEGER, "
                   "remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER)")
        db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (501, 1, 0, 0, 9, -1, 1, 0)")
        db.execute("CREATE TABLE npcundead (name TEXT PRIMARY KEY, undead INTEGER, note TEXT, source TEXT)")
        db.executemany("INSERT INTO npcundead VALUES (?, ?, '', 'test')", [("rotting corpse", 1), ("a soul eater", 0)])
        db.execute("CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER, "
                   "minlevel INTEGER, maxlevel INTEGER, source TEXT)")
        db.execute("CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT, "
                   "PRIMARY KEY (npc_name, tablename))")
        db.execute("CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT, region TEXT, npc_name TEXT, "
                   "patternid INTEGER, weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT)")
        db.execute("INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) "
                   "VALUES ('uncommon', 'forest', '', 11, 1, 1, 60, 'seed')")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER)")
        db.commit()
        db.close()
        self.before = dump(self.path)
        os.makedirs(os.path.join(self.dir, "wiki"))
        pages = {
            "Item List": page(ITEM_LIST),
            "Ogg's Cleaver": page("A cleaver.\n==Statistics==\nClasses: WAR PAL SK\n\nRaces: ALL\n\nItem Level: 28\n*STR: 9\n"
                                  "*DMG: 300\n==Drop Information==\n*[[Brute Ogg]]\n", "Category:Weapons", "Category:Primary"),
            "Boneplate": page("Plate.\n==Statistics==\nClasses: WAR PAL SK CL\n\nRaces: ALL\n\nItem Level: 18\n*AC: 60\n"
                              "It is dropped by [[Undead|undead]] mobs in the appropriate level range (16-20).",
                              "Category:Armor", "Category:Chestpieces"),
            "Grimplate": page("Plate.\n==Statistics==\nClasses: WAR PAL SK CL\n\nRaces: ALL\n\nItem Level: 18\n*AC: 90\n"
                              "It drops from undead mobs within the 16 to 20 level range, with an ULTRA RARE frequency.",
                              "Category:Armor", "Category:Chestpieces"),
            "Woodsword": page("<onlyinclude>{{ItemTransformer\n|location=[[Forest|Forest]]\n}}</onlyinclude>\n==Statistics==\n"
                              "Classes: WAR RAN\n\nRaces: ALL\n\nItem Level: 22\n*DMG: 250\n", "Category:Weapons", "Category:Primary"),
            "Scale set": page("The Scale set is a scale quality armor set. It is useable by [[Rogue|rogues]].\n{| class=x\n|-\n"
                              "!Name !!Level\n|-\n|[[Scale Coif]]\n|40\n|Head\n|Scale\n|N/A\n|0\n|\n|AGI: 10 |\n|None\n",
                              "Category:Item Sets"),
            "Brute Ogg": page("==Loot==\n*[[Ogg's Cleaver]]\n*Ogg's Mystery Charm\n", "Category:Named Mobs"),
        }
        with open(os.path.join(self.dir, "wiki", "allpages.json"), "w") as fh:
            json.dump(pages, fh)
        os.makedirs(os.path.join(self.dir, "harvest"))
        with open(os.path.join(self.dir, "harvest", "h.json"), "w") as fh:
            json.dump({"merchants": [{"name": "Bowyer Derlask", "wares": [
                {"id": 1, "name": "Plain Shortbow", "value": 254, "icon": 444, "wear_slot": 16, "attack_type": 7, "damage": 139,
                 "range": 25, "level_req": 12, "max_stack": 1, "max_hp": 2400, "durability": 50, "class_mask": 111,
                 "race_mask": 1023, "no_trade": 0, "lore": 0, "desc": "A plain bow.", "stats": []},
                {"id": 2, "name": "Gold Ring", "value": 3000, "icon": 81, "wear_slot": 8, "level_req": 20, "stats": []},
                {"id": 3, "name": "Leather Cap", "value": 120, "icon": 12, "wear_slot": 1, "level_req": 10, "stats": [[14, 8]]},
                {"id": 4, "name": "Plate Helm", "value": 900, "icon": 14, "wear_slot": 1, "level_req": 30, "stats": [[14, 40]]},
                {"id": 5, "name": "Iron Spear", "value": 370, "icon": 91, "wear_slot": 15, "attack_type": 6, "damage": 200,
                 "level_req": 18, "stats": []}]}]}, fh)

    def tearDown(self):
        for root, dirs, files in os.walk(self.dir, topdown=False):
            for f in files:
                os.remove(os.path.join(root, f))
            for d in dirs:
                os.rmdir(os.path.join(root, d))
        os.rmdir(self.dir)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "import_wiki_items.py"), "--db", self.path,
                               "--wiki", os.path.join(self.dir, "wiki"), "--prices", os.path.join(self.dir, "harvest"), *args],
                              capture_output=True, text=True, check=True).stdout

    def test_import_and_undo(self):
        out = self.tool("--list")
        self.assertIn("10 items to make", out)
        self.assertIn("left: \"ogg's mystery charm\": the wiki gives a name alone", out)
        self.assertIn("left: 'iron cap': ours by another name, every figure the same: Iron Helm", out)
        self.assertNotIn("Rusty Hachet", out.split("left:")[0], "a typo of ours is ours")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("made 10 items", self.tool("--apply"))
        db = sqlite3.connect(self.path)
        rec = {r[0]: r for r in db.execute("SELECT itemname, equipslot, attacktype, weapondamage, levelreq, classuse, raceuse, "
                                           "trade, itemcost, itemicon, model, unk5, maxhp, AC, str, dex FROM itempattern WHERE patternid >= 713000")}
        self.assertEqual(rec["Test Glaive"][1:6], (15, 6, 232, 20, 15), "a two-handed piercer, Wa Ra P Sk")
        self.assertEqual(rec["Test Glaive"][10], 900, "its look: our type 6 weapons'")
        self.assertEqual(rec["Plain Shortbow"][8:12], (254, 444, 700, 25), "Sandstorm's value and icon, our bows' model, range 25")
        self.assertEqual(rec["Quest Cap"][5:8], (1, 4, 1), "a warrior's, a dark elf's, NO TRADE")
        self.assertEqual((rec["Quest Cap"][13], rec["Quest Cap"][14], rec["Quest Cap"][15]), (42, 4, 10), "AC and the bonus (Dex_10)")
        self.assertEqual((rec["Mystery Cap"][10], rec["Mystery Cap"][9]), (1, 11), "a caster's cap: cloth, the cloth caps' icon")
        self.assertEqual((rec["Boneplate"][10], rec["Boneplate"][9]), (4, 54), "plate classes alone: plate, the plate chests' icon")
        self.assertEqual((rec["Ogg's Cleaver"][2], rec["Ogg's Cleaver"][10], rec["Ogg's Cleaver"][9]), (1, 800, 61),
                         "a cleaver slashes, with our cleavers' look and icon")
        self.assertEqual(rec["Scale Coif"][10], 7, "its set's material: scale")
        self.assertGreater(rec["Bandit Band"][8], 0, "valued by the rule")
        cat = {r for r in db.execute("SELECT npcid, patternid FROM npcitem WHERE itemid > 1")}
        ids = {r[0]: r[1] for r in db.execute("SELECT itemname, patternid FROM itempattern")}
        self.assertEqual(cat, {(501, ids["Test Glaive"]), (502, ids["Plain Shortbow"])}, "the Halas vendor's and Sandstorm's merchant's catalogs")
        loot = set(db.execute("SELECT npc_name, tablename, chance FROM npcloot"))
        self.assertEqual(loot, {("brute ogg", "wiki brute ogg", 25), ("bandit", "wiki bandit", 5),
                                ("decaying skeleton", "wiki undead", 3), ("rotting corpse", "wiki undead", 3)},
                         "the named one, the generic one, the undead of 16-20 (by name or npcundead; a 0 row keeps one living; "
                         "'living spirit' is no undead word)")
        tables = {(t, p): (w, lo, hi) for t, p, w, lo, hi in db.execute("SELECT tablename, patternid, weight, minlevel, maxlevel FROM loottables")}
        self.assertEqual(tables[("wiki undead", ids["Boneplate"])], (100, 16, 20))
        self.assertEqual(tables[("wiki undead", ids["Grimplate"])], (1, 16, 20), "ultra rare: weight 1")
        pool = {(r, p): (lo, hi) for r, p, lo, hi in db.execute("SELECT region, patternid, minlevel, maxlevel FROM lootpool WHERE source = 'wiki'")}
        self.assertEqual(pool, {("forest", ids["Woodsword"]): (17, 27), ("*", ids["Mystery Cap"]): (13, 23), ("*", ids["Scale Coif"]): (35, 45)},
                         "a region's pool, else every region's; NO TRADE nowhere")
        db.close()
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo deletes every row it made")


if __name__ == "__main__":
    unittest.main()
