#!/usr/bin/env python3
"""Tests of tools/clockwork_invasion_20261120.py that need no database: CLIENT's lengths (handlers/quests.go: a box
218, an answer 63, a journal line 127), every class rewarded at each Mysidia stage, the chain's shape."""
import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import clockwork_invasion_20261120 as cw  # noqa: E402


class TestClockworkInvasion(unittest.TestCase):
    def setUp(self):
        self.ids = {n: i for i, n in enumerate(cw.Plan.item_names(), 1)}
        self.qs = cw.quests(self.ids, 999)

    def test_lengths(self):
        for name, giver, lv, rep, offer, accept, decline, declined, started, steps in self.qs:
            for s, lim in ((offer, 218), (declined, 218), (started, 218), (accept, 63), (decline, 63)):
                self.assertLessEqual(len(s), lim, (name, s))
            for st in steps:
                self.assertLessEqual(len(st["journal"]), 127, (name, st["journal"]))
                self.assertLessEqual(len(st.get("says", "")), 218, (name, st.get("says")))

    def test_every_class_rewarded(self):
        for q in self.qs[:5]:
            give = q[9][-1]
            self.assertEqual(give["kind"], "give")
            self.assertEqual(sorted(int(c) for c in json.loads(give["byclass"])), list(range(15)))
            self.assertGreater(give["xp"], 0)

    def test_chain(self):
        self.assertEqual([q[1] for q in self.qs], [cw.GIVER] * 5 + ["Recon Recon"])
        self.assertEqual([q[3] for q in self.qs], [0] * 5 + [1])
        for q in self.qs[:5]:
            loot, give = q[9][-2], q[9][-1]
            self.assertEqual((loot["kind"], loot["items"]), ("loot", give["items"]))

    def test_spawns_are_spawn_only_or_standing(self):
        names = {n.lower() for n, v in cw.NEW_NPCS.items() if v[5]} | {"deadly corrosion", "corroded clockwork", "#999"}
        for q in self.qs:
            for st in q[9]:
                for e in json.loads(st.get("events") or "[]"):
                    if e["do"] == "spawn":
                        self.assertIn(e["npc"].lower(), names)
                        self.assertTrue(e["group"])


if __name__ == "__main__":
    unittest.main()
