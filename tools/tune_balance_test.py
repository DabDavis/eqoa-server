#!/usr/bin/env python3
"""tools/tune_balance.py against a made-up database holding the seeds of 2026-10-29: the con rows, con_pct,
the pets' hit points and the class rows take the tuning, the class and level rows are added (the table made),
a GM's row is left (and said), a second --apply changes nothing, --undo gives back every table as it was and
drops the log, no log column is named like a secret (internal/database/snapshot_names.go's rules, read out
of the Go file), and the Go seeds (a table made new) are the values this writes. No game data.

    python3 tools/tune_balance_test.py
"""
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "tune_balance.py")
sys.path.insert(0, HERE)
import tune_balance as tb  # noqa: E402

SCHEMA = """
CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL NOT NULL DEFAULT 100, damage_pct REAL NOT NULL DEFAULT 100,
    con_pct REAL NOT NULL DEFAULT 100, pet_hp_pct REAL NOT NULL DEFAULT 100, pet_damage_pct REAL NOT NULL DEFAULT 100,
    art_cooldown_ms REAL NOT NULL DEFAULT 0, regen_pct REAL NOT NULL DEFAULT 15, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE balancecon (diff INTEGER PRIMARY KEY, damage_pct REAL NOT NULL DEFAULT 100, taken_pct REAL NOT NULL DEFAULT 100,
    root_resist_pct REAL NOT NULL DEFAULT 0, root_break_pct REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE balanceclass (class INTEGER PRIMARY KEY, melee_taken_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
"""

def secret_like():
    """internal/database/snapshot_names.go's SecretLikeName, its lists read out of the Go file (so they cannot drift)."""
    with open(os.path.join(HERE, "..", "internal", "database", "snapshot_names.go")) as f:
        go = f.read()
    body = lambda start: go[go.index(start):go.index("}", go.index(start))]
    stems = re.findall(r'"([^"]+)"', body("var secretStems"))
    words = set(re.findall(r'"([^"]+)": true', body("var secretWords")))
    compound = re.compile("".join(re.findall(r"`([^`]*)`", go[go.index("var ipCompound"):go.index("\n\n", go.index("var ipCompound"))])))
    assert len(stems) > 20 and "key" in stems and "ip" in words, "snapshot_names.go's lists not found"

    def name_words(name):
        out, cur = [], ""
        for i, ch in enumerate(name):
            if ch.isalnum():
                if ch.isupper() and i > 0 and name[i - 1].islower():
                    out.append(cur.lower())
                    cur = ""
                cur += ch
            else:
                if cur:
                    out.append(cur.lower())
                cur = ""
        if cur:
            out.append(cur.lower())
        return [w for w in out if w]

    def check(name):
        low = name.lower()
        return any(s in low for s in stems) or any(w in words or compound.search(w) for w in name_words(name))
    return check


class TuneBalance(unittest.TestCase):
    def setUp(self):
        """The balance tables as eqoaclassic had them at 01:30 UTC 2026-10-30, the seeds of 2026-10-29."""
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

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def tables(self):
        c = sqlite3.connect(self.db)
        try:
            names = [n for (n,) in c.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")]
            return {n: c.execute(f"SELECT * FROM {n} ORDER BY 1, 2").fetchall() for n in names}
        finally:
            c.close()

    def test_apply_and_undo(self):
        before = self.tables()
        out = self.run_tool().stdout
        n_class_levels = sum(len(v) for v in tb.CLASSLEVELS.values())
        n_floor = sum(1 for l in tb.FLOOR_LEVELS if l not in tb.LEVELS_0130)
        want = len(tb.LEVELS_0130) + n_floor + (len(tb.CONS) - 1) + sum(1 for v in tb.CLASSES.values() if v != 100) + n_class_levels
        self.assertIn(f"{want} rows to change, 0 left", out)
        self.assertEqual(self.tables(), before)  # report only
        self.run_tool("--apply")
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT con_pct, pet_hp_pct, hp_pct, damage_pct FROM balancelevel WHERE level = 5").fetchone(),
                         (100, 100, 215, round(265 * 11 / 12)))
        # an added row: the line's columns at its level, damage_pct evened out (level 6: an even level, a step below)
        self.assertEqual(c.execute("SELECT hp_pct, damage_pct, pet_hp_pct, con_pct FROM balancelevel WHERE level = 6").fetchone(),
                         (215 + 10 / 3, round((265 - 5 / 3) * 13 / 12), 100, 100))
        self.assertEqual([l for (l,) in c.execute("SELECT level FROM balancelevel WHERE level < 15 ORDER BY level")], list(range(1, 15)))
        self.assertEqual(c.execute("SELECT damage_pct, taken_pct, root_resist_pct, root_break_pct FROM balancecon WHERE diff = 1").fetchone(),
                         (147, 68, 0, 5))
        self.assertEqual(c.execute("SELECT count(*) FROM balanceclass WHERE melee_taken_pct != 100").fetchone()[0], 0)
        self.assertEqual(c.execute("SELECT count(*) FROM balanceclasslevel").fetchone()[0], n_class_levels)
        self.assertEqual(c.execute("SELECT melee_taken_pct, pet_hp_pct FROM balanceclasslevel WHERE class = 3 AND level = 60").fetchone(),
                         tuple(tb.CLASSLEVELS[3][60]))
        cols = [r[1] for r in c.execute(f"PRAGMA table_info({tb.LOG})")]
        c.close()
        secret = secret_like()
        self.assertTrue(secret("rowkey") and secret("last_ip"), "the control: names a snapshot refuses")
        for col in cols + [tb.LOG]:
            self.assertFalse(secret(col), f"{col} is named like a secret: every database snapshot would be refused")
        self.assertIn("0 rows to change, 0 left", self.run_tool("--apply").stdout)
        self.run_tool("--undo")
        after = self.tables()
        after.pop("sqlite_sequence", None)  # the log's AUTOINCREMENT bookkeeping, empty
        self.assertEqual(after, before)  # balanceclasslevel, made by the tool, gone again

    def test_a_gms_row_is_left(self):
        c = sqlite3.connect(self.db)
        c.execute("UPDATE balancecon SET damage_pct = 118 WHERE diff = 1")
        c.commit()
        c.close()
        out = self.run_tool("--apply").stdout
        self.assertIn("left as it is: balancecon 1", out)
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT damage_pct FROM balancecon WHERE diff = 1").fetchone()[0], 118)
        self.assertEqual(c.execute("SELECT damage_pct FROM balancecon WHERE diff = 2").fetchone()[0], tb.CONS[2][1][0])
        c.close()

    def test_the_seeds_are_the_tuning(self):
        """internal/database/balanceseed.go (a table made new) is what this tool makes of the 01:30 table."""
        self.run_tool("--apply")
        c = sqlite3.connect(self.db)
        made = {r[0]: tuple(r[1:]) for r in c.execute(f"SELECT level, {', '.join(tb.LEVEL_COLS)} FROM balancelevel")}
        made_cons = {r[0]: tuple(r[1:]) for r in c.execute("SELECT diff, damage_pct, taken_pct, root_resist_pct, root_break_pct FROM balancecon")}
        made_cl = {}
        for cl, l, v, pv in c.execute("SELECT class, level, melee_taken_pct, pet_hp_pct FROM balanceclasslevel"):
            made_cl.setdefault(cl, {})[l] = (v, pv)
        c.close()
        with open(os.path.join(HERE, "..", "internal", "database", "balanceseed.go")) as f:
            go = f.read()
        num = r"([\d.]+)"
        levels = {int(m[0]): tuple(map(float, m[1:])) for m in re.findall(
            r"\{Level: (\d+), HPPct: " + num + ", DamagePct: " + num + ", ConPct: " + num + ", PetHPPct: " + num +
            ", PetDamagePct: " + num + ", ArtCooldownMS: " + num + ", RegenPct: " + num, go)}
        self.assertEqual(levels, made)
        cons = {int(m[0]): tuple(map(float, m[1:])) for m in re.findall(
            r"\{Diff: (\d+), DamagePct: " + num + ", TakenPct: " + num + ", RootResistPct: " + num + ", RootBreakPct: " + num, go)}
        self.assertEqual(cons, made_cons)
        classes = dict(re.findall(r"\{Class: (\d+), MeleeTakenPct: ([\d.]+)\}", go))
        self.assertEqual(len(classes), 15)
        self.assertEqual(set(classes.values()), {"100"})
        cl = {}
        for cls, l, v, pv in re.findall(r"\{Class: (\d+), Level: (\d+), MeleeTakenPct: ([\d.]+), PetHPPct: ([\d.]+)\}", go):
            cl.setdefault(int(cls), {})[int(l)] = (float(v), float(pv))
        self.assertEqual(cl, made_cl)

if __name__ == "__main__":
    unittest.main()
