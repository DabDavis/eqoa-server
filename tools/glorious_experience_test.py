import sqlite3
import unittest

from glorious_experience import EFFECT, NAME, TEMPLATE, install


class GloriousExperienceSeedTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.addCleanup(self.db.close)
        self.db.executescript("""
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT,
                itemcost INT, equipslot INT, attacktype INT, levelreq INT, trade INT, lore INT,
                maxstack INT, classuse INT, raceuse INT, itemicon INT, procanim INT);
            CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT, name TEXT, source TEXT,
                breaks TEXT, wear INT, area REAL, body INT, target_line TEXT, cooldown REAL, charges INT, icon INT);
            CREATE TABLE gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail);
            CREATE TABLE npcitem (patternid INT);
            CREATE TABLE loottables (patternid INT);
            CREATE TABLE lootpool (patternid INT);
            CREATE TABLE heroes_stock (patternid INT);
            CREATE TABLE queststeps (kind TEXT, items TEXT);
        """)
        with self.db:
            self.db.execute("INSERT INTO itempattern (patternid,itemname,itemicon,procanim) VALUES (1,?,12,2)", (TEMPLATE,))
            self.db.execute("INSERT INTO itemeffect (patternid,icon,charges) VALUES (1,34,1)")

    def test_preview_apply_and_repeat(self):
        install(self.db, False)
        self.assertEqual(self.db.execute("SELECT count(*) FROM itempattern").fetchone()[0], 1)
        install(self.db, True)
        install(self.db, True)
        row = self.db.execute("SELECT * FROM itempattern WHERE itemname = ?", (NAME,)).fetchone()
        self.assertEqual((row["maxstack"], row["trade"], row["lore"], row["itemicon"], row["procanim"]), (99, 1, 0, 12, 2))
        effect = self.db.execute("SELECT * FROM itemeffect WHERE patternid = ?", (row["patternid"],)).fetchone()
        self.assertEqual((effect["line"], effect["charges"], effect["icon"]), (EFFECT["line"], 0, 34))
        self.assertEqual(self.db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 2)

    def test_existing_quest_drop_rolls_back_item_and_audit(self):
        with self.db:
            self.db.execute("INSERT INTO queststeps VALUES ('loot','12*1, 002x2')")
        with self.assertRaisesRegex(ValueError, "quest drop"):
            install(self.db, True)
        self.assertEqual(self.db.execute("SELECT count(*) FROM itempattern").fetchone()[0], 1)
        self.assertEqual(self.db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 0)

    def test_conflicting_effect_rolls_back_new_item(self):
        with self.db:
            self.db.execute("INSERT INTO itemeffect (patternid,name) VALUES (2,'other effect')")
        with self.assertRaises(sqlite3.IntegrityError):
            install(self.db, True)
        self.assertEqual(self.db.execute("SELECT count(*) FROM itempattern").fetchone()[0], 1)
        self.assertEqual(self.db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 0)


if __name__ == "__main__":
    unittest.main()
