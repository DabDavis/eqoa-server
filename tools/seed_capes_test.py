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
    """tools/seed_capes.py on a made-up database: a cape a style (wear slot 28, the style in model, colour 0, every
    class and race, NO TRADE, worth 0), one Cape Tailor by the Heirloom Outfitters of each place (two outfitters 6 apart
    are one place, a third far off another), beyond the first one away from its dyer, in a merchant's body neither it
    nor its dyer wears, selling every cape under its own NPC id; a report writes nothing, a second apply is refused, an
    undo puts every row back but a record a character holds; --colours adds the style 2 cloaks in their colours, which
    the default leaves out. No game data."""

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
        db.execute("""CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER,
            x_coord REAL, y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER,
            hp_percent INTEGER, modelid INTEGER, size REAL, race TEXT, npc_level INTEGER, npc_type INTEGER,
            npc_id INTEGER)""")
        db.execute("""CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL,
            stackleft INTEGER NOT NULL DEFAULT 0, remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL
            DEFAULT 0, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL
            DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0)""")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER, equiploc INTEGER)")
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, model) VALUES (5000, 'Iron Helm', 1, 4)")
        npcs = [
            # a place of two outfitters (6 apart), each with its dyer 4 off; a third outfitter far off
            ("Heirloom Outfitter", 10, 100.0, 5.0, 100.0, 64, 0, 111, 0x81),
            ("Armor Dyer", 10, 100.0, 5.0, 96.0, 64, 0, 222, 0x80),
            ("Heirloom Outfitter", 10, 106.0, 5.0, 100.0, 64, 0, 111, 0x81),
            ("Armor Dyer", 10, 106.0, 5.0, 96.0, 64, 0, 222, 0x80),
            ("Heirloom Outfitter", 20, 900.0, 7.0, 900.0, 0, 0, 333, 0x81),
            ("Armor Dyer", 20, 904.0, 7.0, 900.0, 0, 0, 444, 0x80),
            # merchants: one in the outfitter's body (passed over), one in another
            ("Merchant Same", 10, 110.0, 5.0, 110.0, 0, 0, 111, 0x81),
            ("Merchant Other", 10, 120.0, 5.0, 120.0, 0, 0, 555, 0x81),
            ("Merchant Far", 20, 950.0, 7.0, 950.0, 0, 0, 666, 0x81),
            # the control: a guard, no merchant
            ("Guard", 10, 101.0, 5.0, 101.0, 0, 0, 777, 0x00),
        ]
        db.executemany("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, modelid, npc_type, "
                       "npc_level, hp, hp_percent, size) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 50, 1, 255, 1.0)", npcs)
        db.execute("UPDATE npcs SET npc_id = id")
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
        self.assertIn("5 capes at 717001-717005 (wear slot 28), 2 tailors", out)
        self.assertIn("(--colours, off: also the Crimson Cloak", out, "the report names what --colours adds")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.tool("--apply")
        db = sqlite3.connect(self.path)
        capes = db.execute("SELECT patternid, equipslot, model, color, classuse, raceuse, trade, itemcost, levelreq, "
                           "maxstack, maxhp, str FROM itempattern WHERE patternid > 5000 ORDER BY patternid").fetchall()
        self.assertEqual([c[0] for c in capes], list(range(717001, 717006)), "five styles, no colours by default")
        for pid, slot, model, color, classes, races, trade, cost, level, stack, hp, stat in capes:
            self.assertEqual((slot, model, color), (28, pid - 717000, 0), "the cape's slot, its style, the art's colours")
            self.assertEqual((classes, races, trade, cost, level, stack, hp, stat), (0x7FFF, 32767, 1, 0, 1, 1, 0, None))
        tailors = db.execute("SELECT id, npc_id, world, zone, x_coord, z_coord, facing, modelid, npc_type FROM npcs "
                             "WHERE npc_name = 'Cape Tailor' ORDER BY id").fetchall()
        self.assertEqual(len(tailors), 2, "one tailor a place")
        near, far = tailors
        self.assertEqual(near[1], near[0], "its own NPC id, its catalog's key")
        self.assertEqual((near[2], near[3], round(near[4], 2), round(near[5], 2), near[6], near[7], near[8]),
                         (0, 10, 100.0, 102.0, 64, 555, 0x81),
                         "beyond the first outfitter away from its dyer, as it faces, in a body neither wears")
        self.assertEqual((round(far[4], 2), round(far[5], 2), far[7]), (898.0, 900.0, 666))
        for t in tailors:
            sold = [p for (p,) in db.execute("SELECT patternid FROM npcitem WHERE npcid = ? ORDER BY listnumber", (t[0],))]
            self.assertEqual(sold, list(range(717001, 717006)), "every cape on every tailor")
        self.assertEqual(db.execute("SELECT COUNT(*) FROM npcitem WHERE npcid NOT IN (SELECT id FROM npcs WHERE "
                                    "npc_name = 'Cape Tailor')").fetchone()[0], 0, "the control: no one else sells them")
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

    def test_colours(self):
        self.tool("--colours", "--apply")
        db = sqlite3.connect(self.path)
        got = db.execute("SELECT patternid, itemname, model, color FROM itempattern WHERE patternid > 717005 "
                         "ORDER BY patternid").fetchall()
        self.assertEqual(got, [(717020, "Crimson Cloak", 2, 0xB01818FF), (717021, "Forest Cloak", 2, 0x2E7D32FF),
                               (717022, "Royal Cloak", 2, 0x283593FF), (717023, "Gilded Cloak", 2, 0xC9A227FF)])
        self.assertEqual(db.execute("SELECT COUNT(*) FROM itempattern WHERE patternid BETWEEN 717001 AND 717005 "
                                    "AND color = 0").fetchone()[0], 5, "the styles keep the art's own colours")
        for (t,) in db.execute("SELECT id FROM npcs WHERE npc_name = 'Cape Tailor'").fetchall():
            self.assertEqual(db.execute("SELECT COUNT(*) FROM npcitem WHERE npcid = ?", (t,)).fetchone()[0], 9)
        db.close()
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
