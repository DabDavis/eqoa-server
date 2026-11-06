import math
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))

ITEM_COLS = ("patternid INTEGER PRIMARY KEY, itemcost INTEGER, unk1 INTEGER, itemicon INTEGER, unk2 INTEGER, equipslot INTEGER, "
             "unk3 INTEGER, trade INTEGER, rent INTEGER, unk4 INTEGER, attacktype INTEGER, weapondamage INTEGER, unk5 INTEGER, "
             "levelreq INTEGER, maxstack INTEGER, maxhp INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER, "
             "procanim INTEGER, lore INTEGER, unk6 INTEGER, craft INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, "
             "color INTEGER, str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER, "
             "intelligence INTEGER, HPMAX INTEGER, POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER, "
             "DR INTEGER, FR INTEGER, CR INTEGER, LR INTEGER, AR INTEGER, weaponproc INTEGER, fish INTEGER")


def data(path):
    """Every row of the tables the tool writes or reads, its own tables left out."""
    db = sqlite3.connect(path)
    out = {}
    for t in ("itempattern", "npcs", "npcitem", "charinventory"):
        out[t] = sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
    db.close()
    return out


class SeedHeirlooms(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute(f"CREATE TABLE itempattern ({ITEM_COLS})")
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, "
                   "y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, hp INTEGER, hp_percent INTEGER, modelid INTEGER, "
                   "size REAL, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER)")
        db.execute("CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER "
                   "NOT NULL DEFAULT 0, remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid "
                   "INTEGER NOT NULL, equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber "
                   "INTEGER NOT NULL DEFAULT 0)")
        db.execute("CREATE TABLE quests (questid INTEGER PRIMARY KEY, giver TEXT, source TEXT)")
        db.execute("CREATE TABLE defaultcharacter (tclass INTEGER, race INTEGER, x REAL, y REAL, z REAL, world INTEGER)")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER)")
        item = lambda pid, name, slot, cls, lvl, **kw: db.execute(
            "INSERT INTO itempattern (patternid, itemname, equipslot, classuse, levelreq, rent, trade, itemcost, model, "
            "itemicon, color, AC, weapondamage, attacktype) VALUES (?, ?, ?, ?, ?, 1, 0, 100, ?, ?, -1, ?, ?, ?)",
            (pid, name, slot, cls, lvl, kw.get("model", 4), kw.get("icon", 11), kw.get("ac"), kw.get("dmg", 0), kw.get("type", 0)))
        item(1, "plate helm", 1, 1, 60, ac=90, icon=77)
        item(2, "prop helm", 1, 1, 59, ac=900)
        item(3, "plate chest", 5, 1, 60, ac=200)
        item(5379, "Sword of the Contender", 12, 8, 60, dmg=801, type=1, model=1205436896, icon=445518418)
        for slot in (2, 3, 4, 6, 7, 8, 9, 10, 11, 19):
            item(100 + slot, f"thing {slot}", slot, 32767, 30, ac=5)
        db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, hp, npc_level, npc_type) "
                   "VALUES ('Commander Nothard', 3, 25482.6, 61.4, 15547.0, 191, 0, 100, 15, 128)")
        db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, hp, modelid, npc_type, npc_id) "
                   "VALUES ('Merchant Janisar', 3, 25490, 61, 15560, 10, 0, 100, 777, 129, 2)")
        db.execute("INSERT INTO quests VALUES (100001, 'Commander Nothard', 'EQOA-Data 1-20/freeport/warrior/L1 (tools/import_quests.py)')")
        db.execute("INSERT INTO defaultcharacter VALUES (0, 0, 25486.5, 61.4, 15547.0, 0)")
        db.commit()
        db.close()
        self.before = data(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args, check=True):
        return subprocess.run([sys.executable, os.path.join(HERE, "seed_heirlooms.py"), "--db", self.path, "--no-backup", *args],
                              capture_output=True, text=True, check=check)

    def test_apply_places_the_vendors_and_undo_puts_back(self):
        out = self.tool().stdout
        self.assertIn("records from 714000, 1 outfitters and as many dyers", out)
        self.assertEqual(data(self.path), self.before, "a report writes nothing")
        self.tool("--apply")
        db = sqlite3.connect(self.path)
        war = {r[0]: r[1:] for r in db.execute(
            "SELECT p.itemname, p.equipslot, p.trade, p.itemcost, p.levelreq, p.classuse, p.itemicon, p.model, p.AC "
            "FROM itempattern p JOIN heirlooms h ON h.patternid = p.patternid WHERE h.class = 0")}
        self.assertEqual(war["Heirloom Helm"], (1, 1, 0, 1, 1, 77, 4, None),
                         "the warrior's helm: no trade, free, level 1, a warrior's, the best helm's look (not the prop's), no figures")
        self.assertEqual(war["Heirloom Blade"][:2], (14, 1), "a dual wielder's blade goes in either hand")
        self.assertEqual(war["Heirloom Blade"][5:7], (445518418, 1205436896), "the epic blade's look")
        self.assertIn("Heirloom Chestguard", war)
        outf = db.execute("SELECT id, x_coord, z_coord, facing, npc_type, modelid FROM npcs WHERE npc_name = 'Heirloom Outfitter'").fetchone()
        dyer = db.execute("SELECT x_coord, z_coord, npc_type FROM npcs WHERE npc_name = 'Armor Dyer'").fetchone()
        mid = ((25482.6 + 25486.5) / 2, 15547.0)
        self.assertAlmostEqual(math.hypot(outf[1] - mid[0], outf[2] - mid[1]), 2.0, places=1)
        self.assertAlmostEqual(math.hypot(dyer[0] - mid[0], dyer[1] - mid[1]), 2.0, places=1)
        self.assertEqual((outf[3], outf[4], outf[5], dyer[2]), (191, 0x81, 777, 0x80),
                         "the giver's facing; a merchant in the merchant's body; the dyer an NPC")
        npc_id = db.execute("SELECT npc_id FROM npcs WHERE id = ?", (outf[0],)).fetchone()[0]
        self.assertEqual(npc_id, outf[0], "the outfitter's NPCID (npc_id, its catalog's key) is its own, not the body donor's")
        stock = {r[0] for r in db.execute("SELECT p.itemname FROM npcitem n JOIN itempattern p ON p.patternid = n.patternid "
                                          "WHERE n.npcid = ?", (npc_id,))}
        self.assertEqual(len(stock), len(war), "the outfitter sells the warrior's set")
        db.close()
        again = self.tool("--apply", check=False)
        self.assertNotEqual(again.returncode, 0, "a second apply is refused")
        db = sqlite3.connect(self.path)
        held = db.execute("SELECT patternid FROM itempattern WHERE itemname = 'Heirloom Helm' AND classuse = 1").fetchone()[0]
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (5, ?)", (held,))
        db.commit()
        db.close()
        out = self.tool("--undo").stdout
        self.assertIn(f"kept 1 records characters hold: [{held}]", out)
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory")
        db.execute("DELETE FROM itempattern WHERE patternid = ?", (held,))
        db.commit()
        db.close()
        self.assertEqual(data(self.path), self.before, "undo puts every row back (the held record aside)")


if __name__ == "__main__":
    unittest.main()
