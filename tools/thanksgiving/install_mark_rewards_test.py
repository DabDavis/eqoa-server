import json
import sqlite3
import unittest

from install_mark_rewards import PLAN, run


class ThanksgivingRewardsTest(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.addCleanup(self.db.close)
        self.db.executescript("""
            CREATE TABLE gmseasons (name TEXT PRIMARY KEY, active INTEGER);
            CREATE TABLE npcseason (name TEXT PRIMARY KEY, season TEXT, source TEXT);
            CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER);
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, source TEXT);
            CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT,
                itemicon INTEGER, itemcost INTEGER, equipslot INTEGER, attacktype INTEGER,
                weaponproc INTEGER, levelreq INTEGER, trade INTEGER, rent INTEGER, lore INTEGER,
                maxstack INTEGER, raceuse INTEGER, classuse INTEGER);
            CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT, name TEXT, icon INTEGER,
                breaks TEXT, wear INTEGER, area REAL, body INTEGER, target_line TEXT, charges INTEGER,
                cooldown REAL, source TEXT);
            CREATE TABLE tokenshop (id INTEGER PRIMARY KEY, shop TEXT, patternid INTEGER, price INTEGER,
                token TEXT, says TEXT, sort INTEGER, source TEXT, UNIQUE (shop, patternid));
            CREATE TABLE tokenshop_purchases (patternid INTEGER);
            CREATE TABLE charinventory (patternid INTEGER);
            CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT,
                via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
            INSERT INTO npcs VALUES (197603, 'Quartermaster Giblet', 0);
            INSERT INTO npcseason VALUES ('quartermaster giblet', 'Thanksgiving', 'thanksgiving-2026');
            INSERT INTO gmseasons VALUES ('Thanksgiving', 1);
            INSERT INTO quests VALUES (109910, 'thanksgiving-2026');
        """)
        for reward in PLAN["rewards"]:
            self.db.execute("INSERT INTO itempattern (patternid, itemname, itemicon) VALUES (?, ?, 42)",
                            (reward["donor"], reward["donor_name"]))
            self.db.execute("INSERT INTO itemeffect (patternid, icon) VALUES (?, 43)", (reward["donor"],))
        self.db.commit()

    def test_dry_run_apply_retry_and_undo(self):
        self.assertEqual(len(run(self.db)), 6)
        self.assertEqual(self.count("tokenshop"), 0)
        self.assertEqual(self.count("gm_audit"), 0)
        self.assertEqual(len(run(self.db, apply=True)), 6)
        self.assertEqual(self.count("gm_audit"), 6)
        self.assertEqual(run(self.db, apply=True), [])
        rows = self.db.execute("""SELECT p.itemname, p.itemdesc, p.classuse, p.raceuse, e.charges, s.price, s.says,
                              p.itemicon, e.icon, p.maxstack, p.trade, p.lore FROM tokenshop s JOIN itempattern p USING (patternid)
                              JOIN itemeffect e USING (patternid) ORDER BY s.sort""")
        self.assertEqual([tuple(row) for row in rows], [
            (r["name"], r["description"], 32767, 32767, r["charges"], 30, r["says"], 42, 43, 2, 1, 1) for r in PLAN["rewards"]
        ])
        self.assertEqual(self.db.execute("SELECT active FROM gmseasons").fetchone()[0], 1)
        self.assertEqual(len(run(self.db, revert=True)), 6)
        self.assertEqual(self.count("tokenshop"), 0)
        self.assertEqual(self.count("gm_audit"), 12)
        self.assertEqual(run(self.db, revert=True), [])

    def test_collision_leaves_neither_new_item(self):
        with self.db:
            self.db.execute("INSERT INTO itempattern (patternid, itemname) VALUES (1000814, 'Other item')")
        with self.assertRaisesRegex(ValueError, "Existing itempattern"):
            run(self.db, apply=True)
        self.assertEqual(self.count("tokenshop"), 0)
        self.assertEqual(self.count("gm_audit"), 0)
        self.assertIsNone(self.db.execute("SELECT 1 FROM itempattern WHERE patternid=1000813").fetchone())

    def test_upgrades_installed_patterns_in_place_with_existing_copies(self):
        run(self.db, apply=True)
        with self.db:
            self.db.execute("UPDATE itempattern SET maxstack=1, trade=0, lore=0, itemdesc='edited' WHERE patternid IN (1000813,1000814)")
            self.db.execute("INSERT INTO charinventory VALUES (1000814)")
        self.assertEqual(len(run(self.db)), 2)
        self.assertEqual(self.count("gm_audit"), 6)
        self.assertEqual(len(run(self.db, apply=True)), 2)
        self.assertEqual(run(self.db, apply=True), [])
        self.assertEqual(self.count("charinventory"), 1)
        changes = self.db.execute("SELECT tbl, before, after FROM gm_audit WHERE action='update'")
        for table, before, after in changes:
            old, new = json.loads(before), json.loads(after)
            self.assertEqual(table, "itempattern")
            self.assertEqual(old | {"maxstack": 2, "trade": 1, "lore": 1}, new)
            self.assertEqual(new["itemdesc"], "edited")

        with self.db:
            self.db.execute("UPDATE itempattern SET maxstack=1, trade=0, lore=0 WHERE patternid=1000813")
            self.db.execute("UPDATE itempattern SET maxstack=7 WHERE patternid=1000814")
        with self.assertRaisesRegex(ValueError, "Existing itempattern"):
            run(self.db, apply=True)
        self.assertEqual(self.db.execute("SELECT maxstack FROM itempattern WHERE patternid=1000813").fetchone()[0], 1)

    def test_other_shard_is_refused(self):
        with self.db:
            self.db.execute("DELETE FROM npcseason")
        with self.assertRaisesRegex(ValueError, "installed Heirloom Thanksgiving"):
            run(self.db, apply=True)
        self.assertEqual(self.count("gm_audit"), 0)

    def test_undo_refuses_after_purchase(self):
        run(self.db, apply=True)
        with self.db:
            self.db.execute("INSERT INTO tokenshop_purchases VALUES (1000814)")
        with self.assertRaisesRegex(ValueError, "was acquired"):
            run(self.db, revert=True)
        self.assertEqual(self.count("tokenshop"), 2)
        self.assertEqual(self.count("gm_audit"), 6)

    def count(self, table):
        return self.db.execute(f"SELECT count(*) FROM {table}").fetchone()[0]


if __name__ == "__main__":
    unittest.main()
