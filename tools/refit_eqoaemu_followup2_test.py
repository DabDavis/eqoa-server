#!/usr/bin/env python3
"""tools/refit_eqoaemu_followup2.py against tools/tune_pets_test.py's made-up database at the state the 16:38 re-fit
leaves (the four tunings, then tools/refit_eqoaemu_balance.py and tools/refit_eqoaemu_followup.py, as eqoaclassic had
them when the second round went in): each span's rows become this re-fit's exactly (rows changed, added and taken out),
every row outside the spans and every other table as they were; the apply says "changed N rows" (the deploy reads that
line); a second --apply changes nothing; --undo gives back every table exactly and drops the log; a GM's row is left
(and said); nothing is done before the 16:38 re-fit; a table made new (the seeds) is left as it is; the log's columns
pass the snapshot's name rules; and the Go seeds are the three re-fits' rows. No game data.

    python3 tools/refit_eqoaemu_followup2_test.py
"""
import os
import re
import sqlite3
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "refit_eqoaemu_followup2.py")
REFIT_TOOL = os.path.join(HERE, "refit_eqoaemu_balance.py")
FOLLOWUP_TOOL = os.path.join(HERE, "refit_eqoaemu_followup.py")
PETSELF_TOOL = os.path.join(HERE, "fit_pet_selfbuffs.py")
sys.path.insert(0, HERE)
import fit_pet_selfbuffs_test as fpt  # noqa: E402
import refit_eqoaemu_followup as fu  # noqa: E402
import refit_eqoaemu_followup2 as f2  # noqa: E402
import tune_pets_test as tpt  # noqa: E402
from tune_balance_test import secret_like  # noqa: E402


def as_floats(rows):
    return {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in rows.items()}


class RefitEqoaemuFollowup2(unittest.TestCase):
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

    def followed(self):
        """the 10:20 re-fit and the 16:38 re-fit applied over the four tunings"""
        self.tuned()
        self.run_tool(REFIT_TOOL, "--apply")
        self.run_tool(FOLLOWUP_TOOL, "--apply")

    def class_levels(self):
        c = sqlite3.connect(self.db)
        try:
            out = {}
            for cls, l, m, p, d in c.execute("SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM balanceclasslevel"):
                out.setdefault(cls, {})[l] = (m, p, d)
            return out
        finally:
            c.close()

    def test_a_refit_to_write(self):
        self.assertTrue(f2.FOLLOWUP, "the tool carries no re-fit")
        self.assertEqual(set(f2.FOLLOWUP), set(f2.SPAN))
        was = fu.whole_rows()
        for c, spans in f2.SPAN.items():
            levels = set(f2.FOLLOWUP[c])
            self.assertTrue(all(f2.in_span(c, l) for l in levels), f"class {c}: a row outside its spans {spans}")
            for lo, hi in spans:
                self.assertTrue(1 <= lo <= hi <= 60, (c, lo, hi))
                self.assertIn(lo, levels, f"class {c}: no row at its span's first level {lo}")
                self.assertIn(hi, levels, f"class {c}: no row at its span's last level {hi}")
                # the levels just outside a span read a row of the 16:38 re-fit's, as they did
                self.assertTrue(lo == 1 or lo - 1 in was[c], f"class {c}: no row of the 16:38 re-fit's at {lo - 1}")
                self.assertTrue(hi == 60 or hi + 1 in was[c], f"class {c}: no row of the 16:38 re-fit's at {hi + 1}")
            for l, v in f2.FOLLOWUP[c].items():
                self.assertEqual(len(v), 3)
                self.assertTrue(all(x > 0 for x in v), (c, l, v))
        self.assertNotEqual(as_floats(f2.FOLLOWUP), f2.before_rows(), "the re-fit changes nothing")

    def test_nothing_before_the_followup(self):
        """at the 10:20 re-fit's state (the 16:38 one not applied) nothing is done, and it says which tool comes first"""
        self.tuned()
        self.run_tool(REFIT_TOOL, "--apply")
        before = self.tables()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("0 rows to change", out)
        self.assertIn("refit_eqoaemu_followup.py first", out)
        self.assertNotRegex(out, re.compile(r"^changed [1-9][0-9]* rows", re.M))
        self.assertEqual(self.tables(), before)

    def test_the_refit(self):
        self.followed()
        before = self.tables()
        self.assertEqual(self.class_levels(), fu.whole_rows())
        self.run_tool(TOOL)
        self.assertEqual(self.tables(), before)  # report only
        out = self.run_tool(TOOL, "--apply")
        n = sum(1 for c, by in f2.after_rows().items() for l in set(by) | set(f2.before_rows().get(c, {}))
                if f2.before_rows().get(c, {}).get(l) != by.get(l))
        self.assertGreater(n, 0)
        # the deploy's check of the apply's output (deploy-eqfix2.sh greps "^changed [1-9][0-9]* rows")
        self.assertRegex(out, re.compile(rf"^changed {n} rows", re.M))
        now = self.class_levels()
        self.assertEqual(now, f2.whole_rows())
        for c, by in fu.whole_rows().items():  # outside the spans: the 16:38 re-fit's, every row
            self.assertEqual({l: v for l, v in now[c].items() if not f2.in_span(c, l)},
                             {l: v for l, v in by.items() if not f2.in_span(c, l)}, c)
        after = self.tables()
        for name in before:  # only balanceclasslevel moved (and the log came)
            if name != "balanceclasslevel":
                self.assertEqual(after[name], before[name], name)
        self.assertEqual(set(after) - set(before), {f2.LOG})
        c = sqlite3.connect(self.db)
        order = [(cls, l) for cls, l in c.execute("SELECT class, level FROM balanceclasslevel")]
        cols = [r[1] for r in c.execute(f"PRAGMA table_info({f2.LOG})")]
        logged = c.execute(f"SELECT COUNT(*) FROM {f2.LOG}").fetchone()[0]
        c.close()
        self.assertEqual(logged, n)
        self.assertEqual(order, sorted(order))  # the rows added in their place, not at the end
        secret = secret_like()
        for col in cols + [f2.LOG]:
            self.assertFalse(secret(col), f"{col} is named like a secret: every database snapshot would be refused")
        again = self.run_tool(TOOL, "--apply")
        self.assertIn("0 rows to change, 0 left", again)
        self.assertNotRegex(again, re.compile(r"^changed", re.M))
        self.run_tool(TOOL, "--undo")
        self.assertEqual(self.tables(), before)

    def test_a_gms_rows_are_left(self):
        self.followed()
        was, want = f2.before_rows(), f2.after_rows()
        cls, level = next((c, l) for c, by in want.items() for l, v in by.items() if l in was[c] and was[c][l] != v)
        c = sqlite3.connect(self.db)
        c.execute("UPDATE balanceclasslevel SET melee_taken_pct = 333 WHERE class = ? AND level = ?", (cls, level))
        c.commit()
        c.close()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn(f"left as it is: balanceclasslevel {cls} {level}", out)
        now = self.class_levels()
        self.assertEqual(now[cls][level][0], 333)
        others = [(c, l) for c, by in want.items() for l in by if (c, l) != (cls, level)]
        self.assertTrue(others)
        for c, l in others:  # the control: the rest are this re-fit's
            self.assertEqual(now[c][l], want[c][l], (c, l))

    def test_a_table_made_new(self):
        """a table made new holds the seeds, the three re-fits' rows: nothing to do, and not refused"""
        self.tuned()
        c = sqlite3.connect(self.db)
        c.execute("DELETE FROM balanceclasslevel")
        for cls, by in f2.whole_rows().items():
            for l, (m, p, d) in by.items():
                c.execute("INSERT INTO balanceclasslevel (class, level, melee_taken_pct, pet_hp_pct, dealt_pct, source) "
                          "VALUES (?, ?, ?, ?, ?, 'seed')", (cls, l, m, p, d))
        c.commit()
        c.close()
        before = self.tables()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("0 rows to change, 0 left", out)
        self.assertEqual(self.tables(), before)

    def test_the_seeds_are_the_refit(self):
        """internal/database/balanceseed.go's class and level rows are the three re-fits' (a table made new is what the
        tools make of an older one; the earlier tunings' tests follow their chain through this tool)"""
        with open(os.path.join(HERE, "..", "internal", "database", "balanceseed.go")) as f:
            go = f.read()
        cl = {}
        for cls, l, v, pv, dv in re.findall(r"\{Class: (\d+), Level: (\d+), MeleeTakenPct: ([\d.]+), PetHPPct: ([\d.]+)"
                                            r"(?:, DealtPct: ([\d.]+))?\}", go):
            cl.setdefault(int(cls), {})[int(l)] = (float(v), float(pv), float(dv) if dv else 100.0)
        self.assertEqual(cl, f2.whole_rows())


if __name__ == "__main__":
    unittest.main()
