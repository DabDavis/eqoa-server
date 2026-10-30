#!/usr/bin/env python3
"""tools/fit_pet_selfbuffs.py against tools/tune_pets_test.py's made-up database at the state tools/tune_pets.py
leaves, its pets' templates with the notes the server's seeds wrote: the fire, Badi, Badi Elder, Efreet and Efreeti
Lord templates' hit points and the earth and fire templates' and the four mastery templates' notes move to the fit's,
the magician's pet hit points at 34-38 too (three rows added); a second --apply changes nothing; --undo gives back
every table exactly and drops the log; a GM's value is left (and said), and a GM's row among 34-38 leaves all five;
nothing is done before tools/tune_pets.py; the log's columns pass the snapshot's name rules. No game data.

    python3 tools/fit_pet_selfbuffs_test.py
"""
import os
import sqlite3
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fit_pet_selfbuffs.py")
PETS_TOOL = os.path.join(HERE, "tune_pets.py")
sys.path.insert(0, HERE)
import fit_pet_selfbuffs as fp  # noqa: E402
import tune_pets_test as tpt  # noqa: E402
from tune_balance_test import secret_like  # noqa: E402


class FitPetSelfbuffs(unittest.TestCase):
    setUp = tpt.TunePets.setUp
    tearDown = tpt.TunePets.tearDown
    run_tool = tpt.TunePets.run_tool
    retuned = tpt.TunePets.retuned
    tables = tpt.TunePets.tables

    def pets_fitted(self):
        """tools/tune_pets.py's state on a database whose pets' templates the server made: their seeds' notes"""
        self.retuned(pets=True)
        self.run_tool(PETS_TOOL, "--apply")
        c = sqlite3.connect(self.db)
        for name, (old_hp, _, old_note, _) in fp.TEMPLATES.items():
            self.assertEqual(c.execute("SELECT hp_pct FROM classtemplates WHERE name = ?", (name,)).fetchone(), (old_hp,), name)
            c.execute("UPDATE classtemplates SET note = ? WHERE name = ?", (old_note, name))
        c.commit()
        c.close()

    def templates(self):
        c = sqlite3.connect(self.db)
        try:
            return {n: (h, note) for n, h, note in c.execute("SELECT name, hp_pct, note FROM classtemplates")}
        finally:
            c.close()

    def magician(self):
        c = sqlite3.connect(self.db)
        try:
            return {l: (m, p, d) for l, m, p, d in c.execute(
                "SELECT level, melee_taken_pct, pet_hp_pct, dealt_pct FROM balanceclasslevel WHERE class = 10 AND level BETWEEN 30 AND 40")}
        finally:
            c.close()

    def test_nothing_before_the_pets_fit(self):
        self.retuned(pets=True)
        before = self.tables()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("0 rows to change", out)
        self.assertIn("not the pets' fit's", out)
        self.assertEqual(self.tables(), before)

    def test_the_fit(self):
        self.pets_fitted()
        before, was = self.tables(), self.magician()
        self.assertEqual((was[34][1], was[38][1]), (56.0, 100.0))
        self.assertNotIn(36, was)
        self.assertEqual(self.tables(), before)  # report only:
        self.run_tool(TOOL)
        self.assertEqual(self.tables(), before)
        self.run_tool(TOOL, "--apply")
        tm = self.templates()
        for name, (_, new_hp, _, new_note) in fp.TEMPLATES.items():
            self.assertEqual(tm[name], (new_hp, new_note), name)
        self.assertEqual({n: tm[n][0] for n in ("pet fire", "pet badi", "pet badi elder", "pet efreet", "pet efreeti lord", "pet earth")},
                         {"pet fire": 91, "pet badi": 36, "pet badi elder": 32, "pet efreet": 58, "pet efreeti lord": 42, "pet earth": 130})
        self.assertIn("itself in the druid's skin line", tm["pet earth"][1])
        self.assertEqual(tm["pet air"], (80, "the seed"))  # the control: the air pet's buffs are none
        now = self.magician()
        self.assertEqual({l: now[l][1] for l in range(34, 39)}, {34: 41.0, 35: 44.0, 36: 46.0, 37: 53.0, 38: 58.0})
        self.assertEqual({l: (now[l][0], now[l][2]) for l in range(34, 39)}, {l: (100.0, 100.0) for l in range(34, 39)})
        self.assertEqual({l: v for l, v in now.items() if not 34 <= l <= 38}, {l: v for l, v in was.items() if not 34 <= l <= 38})
        c = sqlite3.connect(self.db)
        order = [(cls, l) for cls, l in c.execute("SELECT class, level FROM balanceclasslevel")]
        cols = [r[1] for r in c.execute(f"PRAGMA table_info({fp.LOG})")]
        c.close()
        self.assertEqual(order, sorted(order))  # the rows added in their place, not at the end
        secret = secret_like()
        for col in cols + [fp.LOG]:
            self.assertFalse(secret(col), f"{col} is named like a secret: every database snapshot would be refused")
        self.assertIn("0 rows to change", self.run_tool(TOOL, "--apply"))
        self.run_tool(TOOL, "--undo")
        self.assertEqual(self.tables(), before)

    def test_a_gms_values_are_left(self):
        self.pets_fitted()
        c = sqlite3.connect(self.db)
        c.execute("UPDATE classtemplates SET hp_pct = 77 WHERE name = 'pet badi'")
        c.execute("UPDATE classtemplates SET note = 'mine' WHERE name = 'pet efreet'")
        c.commit()
        c.close()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("left as it is: classtemplates 'pet badi' hp_pct (77, not the fit's 81: a GM's)", out)
        self.assertIn("left as it is: classtemplates 'pet efreet' note", out)
        tm = self.templates()
        self.assertEqual(tm["pet badi"][0], 77)
        self.assertEqual(tm["pet badi"][1], fp.TEMPLATES["pet badi"][3])  # its note still the fit's
        self.assertEqual(tm["pet efreet"], (58, "mine"))
        # a GM's row at 36 (where the line ran) leaves the five as they are
        self.run_tool(TOOL, "--undo")
        c = sqlite3.connect(self.db)
        c.execute("INSERT INTO balanceclasslevel (class, level, melee_taken_pct, pet_hp_pct, note, source, dealt_pct) "
                  "VALUES (10, 36, 100, 70, '', 'gm', 100)")
        c.commit()
        c.close()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("left as it is: balanceclasslevel 10 34-38 (36 (100.0, 70.0, 100.0): not the pets' fit's", out)
        now = self.magician()
        self.assertEqual((now[34][1], now[36][1], now[38][1]), (56.0, 70.0, 100.0))
        self.assertNotIn(35, now)


if __name__ == "__main__":
    unittest.main()
