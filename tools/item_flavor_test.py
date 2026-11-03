import os
import re
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import item_flavor as fl  # noqa: E402


def item(name, slot, model=0, level=20, attacktype=0, **stats):
    return {"name": name, "slot": slot, "model": model, "level": level, "attacktype": attacktype, **stats}


class ItemFlavor(unittest.TestCase):
    """tools/item_flavor.py: the same text for the same record; a pair's sentences in the plural; the look's
    material on armour, none on a crown, a ring in an armour slot described as a ring; an owner's name, a role
    no owner; the magic of the largest stat; held and wielded things carried, not worn; the hand-written misc
    lines; every text a capital to a full stop within MAX_LEN. No game data."""

    def test_same_every_time(self):
        i = item("Etheric Scale Tunic", 5, 7, 52, sta=12)
        self.assertEqual(fl.describe(i), fl.describe(dict(i)))

    def test_pairs_are_plural(self):
        d = fl.describe(item("Lion Fur Gloves", 19, 2, 30, agi=9))
        self.assertNotRegex(d, r"\bIt (is|holds|has)\b", d)
        self.assertRegex(d, r"\bThey (are|hold|carry)\b|has been laid upon them", d)

    def test_material_and_kind(self):
        self.assertIn("plate", fl.describe(item("Odd Breastplate Thing", 5, 4, 40)))
        self.assertIn("leather", fl.describe(item("Odd Vest", 5, 2, 40)))
        crown = fl.describe(item("Spider Queen Crown", 1, 1, 30))
        self.assertNotIn("cloth", crown)
        ring = fl.describe(item("Ebony Ring", 11, 1, 30))
        self.assertIn("ring", ring)
        self.assertNotIn("cloth", ring, "a ring in the feet slot is no cloth boot")

    def test_owner_and_role(self):
        self.assertRegex(fl.describe(item("Macklework's Dagger", 12, 0, 20, 5)), r"Macklework")
        d = fl.describe(item("Dancer's Foil", 12, 0, 20, 5))
        self.assertIn("made for a dancer", d)
        self.assertNotIn("property of Dancer", d)

    def test_magic(self):
        d = fl.describe(item("Plain Sword", 12, 0, 20, 1, str=8, sta=2))
        self.assertRegex(d, r"strength enchantment|enchantment of strength", "the largest stat: " + d)
        self.assertIn("wielder", fl.describe(item("Mace", 12, 0, 20, 3, FR=10)))
        self.assertIn("wearer", fl.describe(item("Ring", 8, 0, 20, FR=10)))
        self.assertIn("carry", fl.describe(item("Wand of Insight", 18, 0, 20)))

    def test_misc_and_shape(self):
        self.assertEqual(fl.describe(item("pixie tears", -1)), "A tiny vial of pixie tears.")
        for i in [item("Boots of the Decrepit Wanderer of the Long Roads", 11, 1, 5, str=60, FR=30),
                  item("x", 0), item("Strange Thing", 13, 0, 0), item("Big Hammer", 15, 0, 55, 4, HPMAX=200)]:
            d = fl.describe(i)
            self.assertRegex(d, r"^[A-Z].*\.$", d)
            self.assertLessEqual(len(d), fl.MAX_LEN, d)
            self.assertNotIn("  ", d)
            self.assertNotRegex(d, r"\b(a|an) (a|an)\b", d)


if __name__ == "__main__":
    unittest.main()
