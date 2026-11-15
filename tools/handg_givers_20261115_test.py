#!/usr/bin/env python3
"""tools/handg_givers_20261115.py: it covers every source quest handg_quests_20261115.py could not make, once, and its
rows are whole (journal lengths, faction lines, the givers it makes or copies)."""
import os
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import handg_givers_20261115 as g  # noqa: E402
import handg_quests_20261115 as h  # noqa: E402


class Rules(unittest.TestCase):
    def test_it_covers_what_the_first_tool_could_not(self):
        made = {q["key"] for q in g.QUESTS} | {k for k, *_ in g.SAME}
        self.assertEqual(made, set(h.CANNOT))
        self.assertEqual(len(made), len(g.QUESTS) + len(g.SAME))

    def test_rows_are_whole(self):
        made = {n[0] for n in g.NPCS}
        copied = {"Big Basher Biggum"}
        for q in g.QUESTS:
            self.assertTrue(q["giver"] in made or q["giver"] in copied or q["giver"] == "Sprexy Lionhunter", q["giver"])
            self.assertIn(q.get("give_to", q["giver"]), made | copied | {"Sprexy Lionhunter"})
            for j in (q["loot"], q["give"]):
                self.assertLessEqual(len(j), 127, j)
            for f in [x for x in q["factions"].split(", ") if x]:
                name, _, n = f.rpartition(":")
                self.assertTrue(name and n.startswith("+") and n[1:].isdigit(), f)
            self.assertTrue(0 < q["chance"] <= 1)
        for name, like, w, x, y, z, facing, level, by, why in g.NPCS:
            self.assertTrue(0 <= facing < 256 and level > 0 and why)


if __name__ == "__main__":
    unittest.main()
