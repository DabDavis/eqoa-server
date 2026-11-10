import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l and "cape_seed_log" not in l
                    and "rowid_made INTEGER" not in l)
    db.close()
    return out


class SeedCapes(unittest.TestCase):
    """tools/seed_capes.py on a made-up database: an heirloom cape a style and a colour (wear slot 28, the style in
    model, the colour in color, named "Heirloom ...", every class and race, NO TRADE, worth 0), the two marked for the
    outfitters after the rest of every Heirloom Outfitter's stock, every one on the genie's list (npcitem 717000, no
    NPC's), nothing on any other NPC; a report writes nothing, a second apply is refused, and so is a plan that would put
    a class's outfitter list past the client's 40; an undo puts every row back but a record a character holds. No game
    data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("""CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemcost INTEGER, unk1 INTEGER,
            itemicon INTEGER, unk2 INTEGER, equipslot INTEGER, unk3 INTEGER, trade INTEGER, rent INTEGER, unk4 INTEGER,
            attacktype INTEGER, weapondamage INTEGER, unk5 INTEGER, levelreq INTEGER, maxstack INTEGER, maxhp INTEGER,
            duration INTEGER, classuse INTEGER, raceuse INTEGER, procanim INTEGER, lore INTEGER, unk6 INTEGER,
            craft INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, color INTEGER, str INTEGER, AC INTEGER,
            weaponproc INTEGER, fish INTEGER)""")
        db.execute("""CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER,
            npc_type INTEGER, npc_id INTEGER)""")
        db.execute("""CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL,
            stackleft INTEGER NOT NULL DEFAULT 0, remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL
            DEFAULT 0, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL
            DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0)""")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER, equiploc INTEGER)")
        # a ranger's (class 1) helm and an all-class ration, on two outfitters; a merchant's wares
        db.executemany("INSERT INTO itempattern (patternid, itemname, equipslot, classuse) VALUES (?, ?, ?, ?)",
                       [(5000, "Heirloom Helm", 1, 2), (5001, "Field Rations", -1, 32767), (5002, "Iron Pot", -1, 32767)])
        db.executemany("INSERT INTO npcs (npc_name, world, npc_type) VALUES (?, 0, ?)",
                       [("Heirloom Outfitter", 0x81), ("Heirloom Outfitter", 0x81), ("Merchant Pots", 0x81)])
        db.execute("UPDATE npcs SET npc_id = id")
        for npcid in (1, 2):
            db.executemany("INSERT INTO npcitem (npcid, stackleft, patternid, listnumber) VALUES (?, 1, ?, ?)",
                           [(npcid, 5000, 0), (npcid, 5001, 1)])
        db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, listnumber) VALUES (3, 1, 5002, 0)")
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args, check=True):
        return subprocess.run([sys.executable, os.path.join(HERE, "seed_capes.py"), "--db", self.path, "--no-backup", *args],
                              capture_output=True, text=True, check=check)

    def test_seed_and_undo(self):
        out = self.tool().stdout
        self.assertIn("9 heirloom capes at 717001-717023 (wear slot 28): 2 on each of the 2 Heirloom Outfitters", out)
        self.assertIn("717003: 'Heirloom Violet Mantle', style 3", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.tool("--apply")
        db = sqlite3.connect(self.path)
        capes = db.execute("SELECT patternid, itemname, equipslot, model, color, classuse, raceuse, trade, itemcost, "
                           "levelreq, maxstack, maxhp, str FROM itempattern WHERE equipslot = 28 ORDER BY patternid").fetchall()
        self.assertEqual([c[0] for c in capes], [717001, 717002, 717003, 717004, 717005, 717020, 717021, 717022, 717023])
        for pid, name, slot, model, color, classes, races, trade, cost, level, stack, hp, stat in capes:
            self.assertTrue(name.startswith("Heirloom "), name)
            self.assertEqual((classes, races, trade, cost, level, stack, hp, stat), (0x7FFF, 32767, 1, 0, 1, 1, 0, None))
            if pid < 717020:
                self.assertEqual((model, color), (pid - 717000, 0), "a style: its own art's colours")
        self.assertEqual([(c[3], c[4]) for c in capes[5:]],
                         [(2, 0xB01818FF), (2, 0x2E7D32FF), (2, 0x283593FF), (2, 0xC9A227FF)], "the colours: style 2")
        for npcid in (1, 2):
            sold = [p for (p,) in db.execute("SELECT patternid FROM npcitem WHERE npcid = ? ORDER BY listnumber", (npcid,))]
            self.assertEqual(sold, [5000, 5001, 717001, 717002], "two capes after an outfitter's own stock, in order")
        genie = [p for (p,) in db.execute("SELECT patternid FROM npcitem WHERE npcid = 717000 ORDER BY listnumber")]
        self.assertEqual(genie, [c[0] for c in capes], "every cape on the genie's list")
        self.assertEqual([p for (p,) in db.execute("SELECT patternid FROM npcitem WHERE npcid = 3")], [5002],
                         "the control: another merchant's wares as they were")
        db.execute("INSERT INTO charinventory (serverid, patternid, equiploc) VALUES (7, 717002, 22)")
        db.commit()
        db.close()
        refused = self.tool("--apply", check=False)
        self.assertNotEqual(refused.returncode, 0)
        self.assertIn("--undo first", refused.stderr)
        self.assertIn("kept 1 records characters hold: [717002]", self.tool("--undo").stdout)
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory")
        db.execute("DELETE FROM itempattern WHERE patternid = 717002")
        db.commit()
        db.close()
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")

    def test_refuses_a_list_past_40(self):
        db = sqlite3.connect(self.path)
        db.executemany("INSERT INTO itempattern (patternid, itemname, equipslot, classuse) VALUES (?, ?, 14, 2)",
                       [(6000 + i, f"Blade {i}") for i in range(37)])
        db.executemany("INSERT INTO npcitem (npcid, stackleft, patternid, listnumber) VALUES (1, 1, ?, ?)",
                       [(6000 + i, 10 + i) for i in range(37)])
        db.commit()
        db.close()
        before = dump(self.path)
        refused = self.tool("--apply", check=False)  # the ranger's 39 + 2
        self.assertNotEqual(refused.returncode, 0)
        self.assertIn("past an outfitter list's 40", refused.stderr)
        self.assertEqual(dump(self.path), before, "a refused plan writes nothing")


if __name__ == "__main__":
    unittest.main()
