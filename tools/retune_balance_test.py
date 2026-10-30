#!/usr/bin/env python3
"""tools/retune_balance.py against a made-up database at the 01:30 state with tools/tune_balance.py applied: the class
and level rows move to the re-fit's (rows changed, added and taken out), a GM's row is left (and said), a second
--apply changes nothing, --undo gives back the first tuning's tables exactly and drops the log, nothing is done
before the first tuning, the log's columns pass the snapshot's name rules, and the Go seeds (a table made new) are
what the two tools make of the 01:30 table. No game data.

    python3 tools/retune_balance_test.py
"""
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
FIRST = os.path.join(HERE, "tune_balance.py")
TOOL = os.path.join(HERE, "retune_balance.py")
sys.path.insert(0, HERE)
import retune_balance as rb  # noqa: E402
import tune_balance as tb  # noqa: E402
from tune_balance_test import SCHEMA, secret_like  # noqa: E402


class RetuneBalance(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        for level, row in tb.LEVELS_0130.items():
            db.execute(f"INSERT INTO balancelevel (level, {', '.join(tb.LEVEL_COLS)}, source) VALUES (?, {', '.join('?' * 7)}, 'seed')",
                       (level,) + row)
        for diff, (now, _) in tb.CONS.items():
            db.execute("INSERT INTO balancecon (diff, damage_pct, taken_pct, root_resist_pct, root_break_pct, source) VALUES (?, ?, ?, ?, ?, 'seed')",
                       (diff,) + now)
        for cls, now in tb.CLASSES.items():
            db.execute("INSERT INTO balanceclass (class, melee_taken_pct, source) VALUES (?, ?, 'seed')", (cls, now))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, tool, *args):
        r = subprocess.run([sys.executable, "-E", "-s", tool, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        if r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r.stdout

    def tables(self):
        c = sqlite3.connect(self.db)
        try:
            names = [n for (n,) in c.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'sqlite_sequence' ORDER BY name")]
            return {n: c.execute(f"SELECT * FROM {n} ORDER BY 1, 2").fetchall() for n in names}
        finally:
            c.close()

    def class_levels(self):
        c = sqlite3.connect(self.db)
        try:
            out = {}
            for cls, l, m, p in c.execute("SELECT class, level, melee_taken_pct, pet_hp_pct FROM balanceclasslevel"):
                out.setdefault(cls, {})[l] = (m, p)
            return out
        finally:
            c.close()

    def test_nothing_before_the_first_tuning(self):
        out = self.run_tool(TOOL, "--apply")
        self.assertIn("0 rows to change, 1 left", out)
        self.assertNotIn("balanceclasslevel", self.tables())

    def test_apply_and_undo(self):
        self.run_tool(FIRST, "--apply")
        first_state = self.tables()
        want = sum(1 for c in set(tb.CLASSLEVELS) | set(rb.RETUNED)
                   for l in set(tb.CLASSLEVELS.get(c, {})) | set(rb.RETUNED.get(c, {}))
                   if tuple(tb.CLASSLEVELS.get(c, {}).get(l, ())) != tuple(rb.RETUNED.get(c, {}).get(l, ())))
        self.assertGreater(want, 0, "the re-fit changes nothing")
        out = self.run_tool(TOOL)
        self.assertIn(f"{want} rows to change, 0 left", out)
        self.assertEqual(self.tables(), first_state)  # report only
        self.run_tool(TOOL, "--apply")
        self.assertEqual(self.class_levels(), {c: {l: tuple(map(float, v)) for l, v in by.items()} for c, by in rb.RETUNED.items()})
        c = sqlite3.connect(self.db)
        cols = [r[1] for r in c.execute(f"PRAGMA table_info({rb.LOG})")]
        c.close()
        secret = secret_like()
        for col in cols + [rb.LOG]:
            self.assertFalse(secret(col), f"{col} is named like a secret: every database snapshot would be refused")
        self.assertIn("0 rows to change, 0 left", self.run_tool(TOOL, "--apply"))
        self.run_tool(TOOL, "--undo")
        self.assertEqual(self.tables(), first_state)

    def test_a_gms_row_is_left(self):
        self.run_tool(FIRST, "--apply")
        cls, level = next((c, l) for c, by in rb.RETUNED.items() for l, v in by.items()
                          if l in tb.CLASSLEVELS.get(c, {}) and tuple(tb.CLASSLEVELS[c][l]) != tuple(v))
        c = sqlite3.connect(self.db)
        c.execute("UPDATE balanceclasslevel SET melee_taken_pct = 333 WHERE class = ? AND level = ?", (cls, level))
        c.commit()
        c.close()
        out = self.run_tool(TOOL, "--apply")
        self.assertIn(f"left as it is: balanceclasslevel {cls} {level}", out)
        self.assertEqual(self.class_levels()[cls][level][0], 333)

    def test_the_seeds_are_the_two_tunings(self):
        """internal/database/balanceseed.go's class and level rows are what the two tools make of the 01:30 table."""
        self.run_tool(FIRST, "--apply")
        self.run_tool(TOOL, "--apply")
        with open(os.path.join(HERE, "..", "internal", "database", "balanceseed.go")) as f:
            go = f.read()
        cl = {}
        for cls, l, v, pv in re.findall(r"\{Class: (\d+), Level: (\d+), MeleeTakenPct: ([\d.]+), PetHPPct: ([\d.]+)\}", go):
            cl.setdefault(int(cls), {})[int(l)] = (float(v), float(pv))
        self.assertEqual(cl, self.class_levels())


if __name__ == "__main__":
    unittest.main()
