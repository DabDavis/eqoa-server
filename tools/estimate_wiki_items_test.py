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
BARD = 32


def page(text, *cats):
    return {"text": text, "cats": list(cats)}


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class EstimateWikiItems(unittest.TestCase):
    """tools/estimate_wiki_items.py on a made-up wiki and database: a level 70 boss's "(Bard only)" vest made for the
    bard at 60, drawn as a bard's chain, its figures a chain vest's (never a freak record's, a cloth or a plate one's),
    dropped every kill; a name that says no slot a held item; a clan's ring at its sibling's level, in its creature
    type's table bound to that type's creatures of its levels; a lost boss's charm in every region's rare pool; a
    piece with no item of ours near its level left; one of ours under a typo not made; a second apply refused; an
    undo deletes all of it. No game data."""

    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        # the server's own schema: every column but the stats NOT NULL, no defaults
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, " + ", ".join(
            f"{c} {'TEXT' if c in ('itemname', 'itemdesc') else 'INTEGER'} NOT NULL" for c in NOT_NULL) + ", " +
            ", ".join(f"{c} INTEGER" for c in STATS) + ")")

        def item(pid, name, slot, level, classes, model=0, icon=0, **stats):
            row = dict.fromkeys(NOT_NULL, 0)
            row.update(itemcost=100, itemicon=icon, equipslot=slot, rent=1, levelreq=level, maxstack=1, maxhp=200 * level,
                       duration=50, classuse=classes, raceuse=1023, itemname=name, itemdesc="", model=model, **stats)
            db.execute(f"INSERT INTO itempattern (patternid, {', '.join(row)}) VALUES (?, {', '.join('?' * len(row))})",
                       (pid, *row.values()))
        # a bard's chest pieces: three chain (style 3), a freak record, a cloth one and a plate one
        item(1, "Linked Vest", 5, 58, BARD, model=3, icon=53, AC=80, dex=10)
        item(2, "Chain Vest", 5, 59, BARD, model=3, icon=53, AC=90, dex=12)
        item(3, "Mail Vest", 5, 60, BARD, model=3, icon=53, AC=100, dex=14)
        item(4, "Freak Vest", 5, 60, BARD, model=3, icon=53, AC=900, cha=300)
        item(5, "Silk Vest", 5, 60, BARD, model=1, icon=51, AC=105, dex=15)
        item(6, "Iron Breastplate", 5, 60, BARD, model=4, icon=54, AC=400, str=30)
        item(10, "Gold Ring", 8, 34, 32767, icon=81, wis=5)
        item(11, "Silver Ring", 8, 35, 32767, icon=81, wis=7)
        item(12, "Copper Ring", 8, 36, 32767, icon=81, wis=9)
        item(13, "Clanmark Boots", 11, 35, 32767, model=1, icon=111, AC=20)
        item(20, "Old Orb", 18, 40, 32767, icon=181, POWMAX=40)
        item(21, "New Orb", 18, 41, 32767, icon=181, POWMAX=45)
        item(22, "Fine Orb", 18, 42, 32767, icon=181, POWMAX=50)
        item(30, "Cave Charm", 4, 20, 32767, icon=41, AC=5)
        item(31, "Bone Amulet", 4, 19, 32767, icon=41, AC=4)
        item(32, "Jade Amulet", 4, 21, 32767, icon=41, AC=6)
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_id INTEGER, npc_type INTEGER, npc_level INTEGER)")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
            (1, "Great Wyrm", 0, 0, 70), (2, "Cave Boss", 0, 0, 45), (3, "gnoll pup", 0, 0, 34), (4, "gnoll pup", 0, 0, 36),
            (5, "gnoll elder", 0, 0, 50)])
        db.execute("CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, stackleft INTEGER, remainHP INTEGER, "
                   "remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER)")
        db.execute("CREATE TABLE npcundead (name TEXT PRIMARY KEY, undead INTEGER, note TEXT, source TEXT)")
        db.execute("CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER, "
                   "minlevel INTEGER, maxlevel INTEGER, source TEXT)")
        db.execute("CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT, "
                   "PRIMARY KEY (npc_name, tablename))")
        db.execute("CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT, region TEXT, npc_name TEXT, "
                   "patternid INTEGER, weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT)")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER)")
        db.commit()
        db.close()
        self.before = dump(self.path)
        os.makedirs(os.path.join(self.dir, "wiki"))
        pages = {
            "Great Wyrm": page("==Loot==\n*[[Vest of Song]] (Bard only)\n*[[Wyrm Orb Thing]]\n*[[Coper Ring]]\n", "Category:Named Mobs"),
            "Gnolls": page("Dogs.\n==Specific loot==\n*Clanmark Ring\n", "Category:Mob Types"),
            "Lost Boss": page("==Loot==\n*[[Lost Charm]]\n", "Category:Named Mobs"),
            "Cave Boss": page("==Loot==\n*[[Old Cave Charm]]\n", "Category:Named Mobs"),
        }
        with open(os.path.join(self.dir, "wiki", "allpages.json"), "w") as fh:
            json.dump(pages, fh)
        os.makedirs(os.path.join(self.dir, "harvest"))
        with open(os.path.join(self.dir, "harvest", "h.json"), "w") as fh:
            json.dump({"merchants": [{"name": "Trader", "wares": [
                {"id": 1, "name": "Gold Ring", "value": 1000, "icon": 81, "wear_slot": 8, "level_req": 34, "stats": [[4, 5]]},
                {"id": 2, "name": "Mail Vest", "value": 3000, "icon": 53, "wear_slot": 5, "level_req": 60, "stats": [[14, 100]]},
                {"id": 3, "name": "Fine Orb", "value": 2000, "icon": 181, "wear_slot": 18, "level_req": 42, "stats": [[10, 50]]},
                {"id": 4, "name": "Cave Charm", "value": 500, "icon": 41, "wear_slot": 4, "level_req": 20, "stats": [[14, 5]]}]}]}, fh)

    def tearDown(self):
        for root, dirs, files in os.walk(self.dir, topdown=False):
            for f in files:
                os.remove(os.path.join(root, f))
            for d in dirs:
                os.rmdir(os.path.join(root, d))
        os.rmdir(self.dir)

    def tool(self, *args, check=True):
        r = subprocess.run([sys.executable, os.path.join(HERE, "estimate_wiki_items.py"), "--db", self.path,
                            "--wiki", os.path.join(self.dir, "wiki"), "--prices", os.path.join(self.dir, "harvest"), *args],
                           capture_output=True, text=True, check=check)
        return r.stdout + r.stderr

    def test_estimate_apply_and_undo(self):
        out = self.tool("--list")
        self.assertIn("4 items to make with estimated figures, 1 left", out)
        self.assertIn("left: 'old cave charm': no item of ours to take figures from (slot 4, type 0, level 45)", out)
        self.assertNotIn("Coper Ring", out, "our Copper Ring under a typo is not made")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("made 4 items", self.tool("--apply"))
        self.assertIn("is there: --undo first", self.tool("--apply", check=False), "a second apply is refused")
        db = sqlite3.connect(self.path)
        rec = {r[0]: r[1:] for r in db.execute(
            "SELECT itemname, equipslot, levelreq, classuse, model, AC, cha, itemdesc, itemcost FROM itempattern WHERE patternid >= 713000")}
        self.assertEqual(set(rec), {"Vest of Song", "Wyrm Orb Thing", "Clanmark Ring", "Lost Charm"})
        slot, level, classes, model, ac, cha, desc, cost = rec["Vest of Song"]
        self.assertEqual((slot, level, classes, model), (5, 60, BARD, 3), "a chest piece, the boss's 70 held to 60, a bard's chain")
        self.assertIn(ac, (90, 100), "a chain vest's figures above the middle: never the freak's, the cloth's or the plate's")
        self.assertEqual((cha, desc), (0, ""), "the description a player reads is left empty")
        self.assertGreater(cost, 0)
        self.assertEqual(rec["Wyrm Orb Thing"][:2], (18, 60), "a name that says no slot of its own word: held, at its boss's level")
        self.assertEqual(rec["Clanmark Ring"][:2], (8, 35), "a ring at its clan's sibling's level (Clanmark Boots)")
        self.assertEqual(rec["Lost Charm"][:2], (4, 20), "a charm at its sibling's level (Cave Charm)")
        loot = set(db.execute("SELECT npc_name, tablename, chance FROM npcloot"))
        self.assertIn(("great wyrm", "wiki great wyrm", 100), loot, "a boss of 50 and up: every kill")
        self.assertEqual({n for n, t, c in loot if t == "wiki gnolls"}, {"gnoll pup"}, "the gnolls within 3 of the ring's 35")
        rows = sorted(db.execute("SELECT tablename, minlevel, maxlevel FROM loottables"))
        self.assertEqual(rows, [("wiki gnolls", 32, 38), ("wiki great wyrm", 1, 100), ("wiki great wyrm", 1, 100)])
        pool = list(db.execute("SELECT category, region, minlevel, maxlevel FROM lootpool WHERE source = 'wiki'"))
        self.assertEqual(pool, [("rare", "*", 15, 25)], "Lost Boss is in no world: every region's rare pool round its 20")
        self.assertEqual(db.execute("SELECT count(*) FROM wiki_item_estimate_log WHERE tbl = 'itempattern' "
                                    "AND what LIKE '%an estimate%'").fetchone()[0], 4, "the log says each is an estimate")
        db.close()
        self.assertIn("deleted:", self.tool("--undo"))
        self.assertEqual(dump(self.path), self.before, "undo deletes every row it made")


if __name__ == "__main__":
    unittest.main()
