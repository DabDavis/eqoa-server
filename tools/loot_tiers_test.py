#!/usr/bin/env python3
"""tools/loot_tiers.py against made-up items: each rule's item lands in its
category and the controls in theirs (an item at its level's median is
uncommon, twice it rare, three times super rare, four times ultra rare; plain
gear and an item of no wear slot uncommon; a second record legendary), and a
shield whose armor class sits in an attribute is scored by its armor class,
not as the attribute. No game data.

    python3 tools/loot_tiers_test.py
"""
import os
import sqlite3
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import loot_tiers as lt  # noqa: E402

COLS = ("patternid", "itemname", "levelreq", "equipslot", "attacktype", "weapondamage", "AC", "HPMAX", "POWMAX", "HoT",
        "PoT", "weaponproc", "str", "sta", "agi", "dex", "wis", "intelligence", "cha", "FR", "LR", "CR", "PR", "DR", "AR")


def item(pid, name, level, slot, **stats):
    row = dict.fromkeys(COLS, 0)
    row.update(patternid=pid, itemname=name, levelreq=level, equipslot=slot, **stats)
    return row


ITEMS = [
    item(1, "Aura of Karana", 26, 13, AC=210),
    item(2, "Orb of Primordial Intellect", 60, 18, intelligence=50, POWMAX=150, PoT=5),
    item(3, "Primordial Orb of Intellect", 60, 18, intelligence=50, POWMAX=150, PoT=5),
    item(4, "Grimsplitter", 24, 12, attacktype=1, weapondamage=300, str=5),
    item(5, "wolf pelt", 0, -1),
    item(7, "leather cap", 20, 1, AC=50),
] + [item(10 + i, f"ring {i}", 20, 8, str=10) for i in range(9)] + [
    item(20, "great ring", 20, 8, str=20),
    item(22, "greater ring", 20, 8, str=30),
    item(23, "greatest ring", 20, 8, str=30, sta=10),  # attributes count up to its level + 10 each
    item(21, "tundra shield", 20, 13, agi=200),  # a shifted label: its armor class in an attribute
] + [item(30 + i, f"plain cap {i}", 20, 1, AC=50) for i in range(4)] + [
    item(40 + i, f"plain shield {i}", 20, 13, AC=150) for i in range(4)]


class LootTiers(unittest.TestCase):
    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.db.execute(f"CREATE TABLE itempattern ({', '.join(c + (' INTEGER' if c != 'itemname' else ' TEXT') for c in COLS)})")
        self.db.executemany(f"INSERT INTO itempattern VALUES ({', '.join('?' * len(COLS))})",
                            [tuple(r[c] for c in COLS) for r in ITEMS])
        self.db.execute("CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY, npcid INTEGER, patternid INTEGER)")
        self.db.execute("CREATE TABLE loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER, weight INTEGER)")
        self.db.executemany("INSERT INTO loottables (tablename, patternid, weight) VALUES (?, ?, ?)",
                            [("craft gems", 50, 30), ("craft gems", 51, 1), ("generic", 50, 1)])
        self.db.executemany(f"INSERT INTO itempattern (patternid, itemname, levelreq, equipslot) VALUES (?, ?, 0, -1)",
                            [(50, "Aquamarine"), (51, "Blue Diamond")])
        self.db.execute("INSERT INTO npcitem (npcid, patternid) VALUES (1, 6)")

    def test_each_rule(self):
        c = lt.categories(self.db)
        self.assertEqual(c[1][0], lt.LEGENDARY)
        self.assertEqual(c[2][0], lt.LEGENDARY)
        self.assertEqual(c[3][0], lt.LEGENDARY)
        self.assertIn("second record", c[3][1])
        self.assertIn("primordial orb of intellect", lt.SECOND_RECORDS)
        self.assertEqual(c[4], lt.ARCHIVE["grimsplitter"])
        self.assertEqual(c[4][0], lt.ULTRA, "Allakhazam's very rare")
        self.assertEqual(c[5][0], lt.UNCOMMON, "no wear slot")
        self.assertEqual(c[7], (lt.UNCOMMON, "no stats: plain gear"))
        self.assertEqual(c[10][0], lt.UNCOMMON, "at its level's median")
        self.assertEqual(c[20][0], lt.RARE, "twice its level's median")
        self.assertEqual(c[22][0], lt.SUPER, "three times")
        self.assertEqual(c[23][0], lt.ULTRA, "four times")

    def test_a_shifted_label_is_read_as_armor_class(self):
        every = lt.items(self.db)
        self.assertEqual((every[21]["AC"], every[21]["agi"]), (200, 0))
        # the control: an armor class above the level's attributes cap stays where the record puts it
        self.assertEqual(every[1]["AC"], 210)
        s = lt.score(every)
        # 200 against the plain shields' 150: 200 - 1.25 x 150 = 12.5 armor class over, 5.9 points (not 200 agility)
        self.assertAlmostEqual(s[21][0], lt.AC_POINT * (200 - lt.PLAIN_SPREAD * 150), places=6)

    def test_kills(self):
        self.assertEqual(lt.kills_in(), 18 * 2 * 20 * 365 * 10)
        self.assertEqual(lt.kills_in(years=1, daily=1, hours=1, an_hour=1), 365)


if __name__ == "__main__":
    unittest.main()
