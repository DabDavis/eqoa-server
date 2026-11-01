#!/usr/bin/env python3
"""tools/refit_eqoaemu_balance.py against tools/tune_pets_test.py's made-up database at the state the four tunings
leave (tools/tune_balance.py, retune_balance.py, tune_pets.py, fit_pet_selfbuffs.py, as eqoaclassic had them when the
workbook went in): balanceclasslevel becomes the re-fit's rows exactly (rows changed, added and taken out, dealt_pct
among them) and balancelevel's pet columns 100 at every row, its other columns as they were; a second --apply changes
nothing; --undo gives back every table exactly and drops the log; a GM's row is left (and said); nothing is done
before the four tunings; the log's columns pass the snapshot's name rules; and the Go seeds (a table made new) are
the re-fit's rows. No game data.

    python3 tools/refit_eqoaemu_balance_test.py
"""
import os
import re
import sqlite3
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "refit_eqoaemu_balance.py")
PETS_TOOL = os.path.join(HERE, "tune_pets.py")
PETSELF_TOOL = os.path.join(HERE, "fit_pet_selfbuffs.py")
sys.path.insert(0, HERE)
import fit_pet_selfbuffs_test as fpt  # noqa: E402
import refit_eqoaemu_balance as rf  # noqa: E402
import tune_pets_test as tpt  # noqa: E402
from tune_balance_test import secret_like  # noqa: E402


class RefitEqoaemuBalance(unittest.TestCase):
    setUp = tpt.TunePets.setUp
    tearDown = tpt.TunePets.tearDown
    run_tool = tpt.TunePets.run_tool
    retuned = tpt.TunePets.retuned
    tables = tpt.TunePets.tables
    pets_fitted = fpt.FitPetSelfbuffs.pets_fitted

    def tuned(self):
        """the four tunings applied, the pets' templates the server's"""
        self.pets_fitted()
        self.run_tool(PETSELF_TOOL, "--apply")

    def class_levels(self):
        c = sqlite3.connect(self.db)
        try:
            out = {}
            for cls, l, m, p, d in c.execute("SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM balanceclasslevel"):
                out.setdefault(cls, {})[l] = (m, p, d)
            return out
        finally:
            c.close()

    def levels(self):
        c = sqlite3.connect(self.db)
        try:
            cur = c.execute("SELECT * FROM balancelevel ORDER BY level")
            cols = [d[0] for d in cur.description]
            return {r[0]: dict(zip(cols, r)) for r in cur.fetchall()}
        finally:
            c.close()

    def test_a_refit_to_write(self):
        self.assertTrue(rf.REFIT, "the tool carries no re-fit")
        self.assertEqual(set(rf.REFIT), set(range(15)))
        self.assertTrue(any(v[2] != 100 for by in rf.REFIT.values() for v in by.values()), "no row deals other than all")

    def test_nothing_before_the_tunings(self):
        self.retuned(pets=True)  # the re-fit of 2026-10-30, not the pets' fits after it
        before = self.tables()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("0 rows to change", out)
        self.assertIn("fit_pet_selfbuffs.py first", out)
        self.assertEqual(self.tables(), before)

    def test_the_refit(self):
        self.tuned()
        before, was = self.tables(), self.levels()
        self.assertEqual(self.class_levels(), {c: dict(by) for c, by in rf.before_rows().items()})
        self.run_tool(TOOL)
        self.assertEqual(self.tables(), before)  # report only
        self.run_tool(TOOL, "--apply")
        self.assertEqual(self.class_levels(), {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in rf.REFIT.items()})
        now = self.levels()
        self.assertEqual(set(now), set(was))
        for level, r in now.items():
            self.assertEqual((r["pet_hp_pct"], r["pet_damage_pct"]), rf.PET_LEVEL, level)
            self.assertEqual({k: v for k, v in r.items() if k not in ("pet_hp_pct", "pet_damage_pct")},
                             {k: v for k, v in was[level].items() if k not in ("pet_hp_pct", "pet_damage_pct")}, level)
        c = sqlite3.connect(self.db)
        order = [(cls, l) for cls, l in c.execute("SELECT class, level FROM balanceclasslevel")]
        cols = [r[1] for r in c.execute(f"PRAGMA table_info({rf.LOG})")]
        c.close()
        self.assertEqual(order, sorted(order))  # the rows added in their place, not at the end
        secret = secret_like()
        for col in cols + [rf.LOG]:
            self.assertFalse(secret(col), f"{col} is named like a secret: every database snapshot would be refused")
        self.assertIn("0 rows to change, 0 left", self.run_tool(TOOL, "--apply"))
        self.run_tool(TOOL, "--undo")
        self.assertEqual(self.tables(), before)

    def test_a_gms_rows_are_left(self):
        self.tuned()
        was = rf.before_rows()
        cls, level = next((c, l) for c, by in rf.REFIT.items() for l, v in by.items()
                          if l in was.get(c, {}) and was[c][l] != tuple(map(float, v)))
        c = sqlite3.connect(self.db)
        c.execute("UPDATE balanceclasslevel SET melee_taken_pct = 333 WHERE class = ? AND level = ?", (cls, level))
        c.execute("UPDATE balancelevel SET pet_damage_pct = 222 WHERE level = 30")
        c.commit()
        c.close()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn(f"left as it is: balanceclasslevel {cls} {level}", out)
        self.assertIn("left as it is: balancelevel 30", out)
        self.assertEqual(self.class_levels()[cls][level][0], 333)
        self.assertEqual(self.levels()[30]["pet_damage_pct"], 222)
        self.assertEqual(self.levels()[45]["pet_damage_pct"], rf.PET_LEVEL[1])  # the control: the rest are the re-fit's

    def test_the_seeds_are_the_refit(self):
        """internal/database/balanceseed.go's class and level rows are the re-fit's (with the follow-up's spans over
        them, tools/refit_eqoaemu_followup.py) and its level rows' pet columns the re-fit's, so a table made new is
        what the six tools make of an older one (the earlier tunings' tests follow their chain through this tool)"""
        import refit_eqoaemu_followup as fu
        with open(os.path.join(HERE, "..", "internal", "database", "balanceseed.go")) as f:
            go = f.read()
        cl = {}
        for cls, l, v, pv, dv in re.findall(r"\{Class: (\d+), Level: (\d+), MeleeTakenPct: ([\d.]+), PetHPPct: ([\d.]+)"
                                            r"(?:, DealtPct: ([\d.]+))?\}", go):
            cl.setdefault(int(cls), {})[int(l)] = (float(v), float(pv), float(dv) if dv else 100.0)
        self.assertEqual(cl, fu.whole_rows())
        for c, by in rf.REFIT.items():  # outside the follow-up's spans, this re-fit's
            self.assertEqual({l: v for l, v in cl[c].items() if not fu.in_span(c, l)},
                             {l: tuple(map(float, v)) for l, v in by.items() if not fu.in_span(c, l)}, c)
        pets = re.findall(r"\{Level: (\d+), HPPct: [\d.]+, DamagePct: [\d.]+, ConPct: [\d.]+, PetHPPct: ([\d.]+), "
                          r"PetDamagePct: ([\d.]+)", go)
        self.assertEqual({int(l) for l, _, _ in pets}, set(rf.PET_LEVEL_BEFORE))
        self.assertEqual({(float(h), float(d)) for _, h, d in pets}, {rf.PET_LEVEL})


if __name__ == "__main__":
    unittest.main()
