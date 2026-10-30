#!/usr/bin/env python3
"""tools/loot_tiers.py against made-up items: each rule's item lands in its
tier and the controls in theirs (an item at its level's median is uncommon,
three times it rare; plain gear, a material and a merchant's ware common; a
second record legendary but never placed; a shield whose armor class sits in an
attribute scored by its armor class, not as the attribute), and the legendary
rate is five over the kills its creatures take. No game data.

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
    item(6, "merchant's ring", 20, 8, str=10),
    item(7, "leather cap", 20, 1, AC=50),
    item(8, "leather cap of plenty", 20, 1, AC=50, str=30),
] + [item(10 + i, f"ring {i}", 20, 8, str=10) for i in range(6)] + [
    item(20, "great ring", 20, 8, str=30),
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
        self.db.execute("INSERT INTO npcitem (npcid, patternid) VALUES (1, 6)")

    def test_each_rule(self):
        t = lt.tiers(self.db)
        self.assertEqual(t[1][0], lt.LEGENDARY)
        self.assertEqual(t[2][0], lt.LEGENDARY)
        self.assertEqual(t[3][0], lt.LEGENDARY)
        self.assertIn("second record", t[3][1])
        self.assertIn("primordial orb of intellect", lt.SECOND_RECORDS)
        self.assertEqual(t[4], lt.ARCHIVE["grimsplitter"])
        self.assertEqual(t[5][0], lt.COMMON, "a material")
        self.assertEqual(t[6], (lt.COMMON, "sold by a merchant here"), "a ware, stats or not")
        self.assertEqual(t[7], (lt.COMMON, "no stats: plain gear"))
        self.assertEqual(t[10][0], lt.UNCOMMON, "at its level's median")
        self.assertEqual(t[20][0], lt.RARE, "three times its level's median")
        self.assertEqual(t[8][0], lt.RARE, "plain armor class and 30 strength: three times the median")

    def test_a_shifted_label_is_read_as_armor_class(self):
        every = lt.items(self.db)
        self.assertEqual((every[21]["AC"], every[21]["agi"]), (200, 0))
        # the control: an armor class above the level's attributes cap stays where the record puts it
        self.assertEqual(every[1]["AC"], 210)
        s = lt.score(every)
        # 200 against the plain shields' 150: 200 - 1.25 x 150 = 12.5 armor class over, 5.9 points (not 200 agility)
        self.assertAlmostEqual(s[21][0], lt.AC_POINT * (200 - lt.PLAIN_SPREAD * 150), places=6)

    def test_legendary_rate(self):
        self.assertEqual(lt.kills_in(), 18 * 2 * 20 * 365 * 10)
        # 5 over the kills of a hundredth of the spawns of a million kills
        self.assertAlmostEqual(lt.legendary_rate(100, 10000, kills=1000000), 5 / 10000)
        self.assertEqual(lt.legendary_rate(1, 10000, kills=1000), lt.TIER_RATE[lt.RARE], "never above a rare's")
        self.assertEqual(lt.legendary_rate(0, 10000), 0.0)


if __name__ == "__main__":
    unittest.main()
