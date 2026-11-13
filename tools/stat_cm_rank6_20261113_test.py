#!/usr/bin/env python3
"""stat_cm_rank6_20261113.py on a catalog of the seven lines' ranks 1-5: the report writes nothing, --apply adds the seven
rank 6 rows after rank 5, a second apply nothing, --undo exact; a rank 5 changed by a GM refuses everything."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "stat_cm_rank6_20261113.py")
sys.path.insert(0, HERE)
import stat_cm_rank6_20261113 as t6  # noqa: E402


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
            races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
            name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
            level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
            requires TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
            enabled INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm', require_quests TEXT NOT NULL DEFAULT '');
        CREATE TABLE charcms (serverid INTEGER, cmid INTEGER, cost INTEGER, bought INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
    """)
    for mastery, stat, r5 in t6.LINES:
        for k, (cost, level, need) in enumerate([(10, 10, 100), (15, 20, 125), (20, 30, 160), (25, 45, 200), (30, 60, 250)], 1):
            cid = r5 - 5 + k
            db.execute("INSERT INTO cmcatalog (id, mastery, rank, name, parent, cost, level, minstats, effects, description, require_quests) "
                       "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                       (cid, mastery, k, f"{mastery} {k}", 0 if k == 1 else cid - 1, cost, level, f"{stat}:{need}",
                        f"{stat}:{k},{stat}Max:{10 + 5 * k}", f"{mastery} desc", "42" if k == 1 else ""))
    db.commit()
    db.close()


def run(path, *args):
    p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)
    return p.returncode, p.stdout


def dump(path):
    db = sqlite3.connect(path)
    out = db.execute("SELECT * FROM cmcatalog ORDER BY id").fetchall()
    db.close()
    return out


class Test(unittest.TestCase):
    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = dump(p)
            self.assertEqual(run(p)[0], 0)
            self.assertEqual(dump(p), before)
            code, out = run(p, "--apply")
            self.assertEqual(code, 0, out)
            db = sqlite3.connect(p)
            got = db.execute("SELECT id, name, parent, cost, level, minstats, effects FROM cmcatalog WHERE rank = 6 ORDER BY id").fetchall()
            db.close()
            self.assertEqual(len(got), 7)
            self.assertEqual(got[3], (66, "Balance 6", 65, 35, 60, "DEX:310", "DEX:6,DEXMax:40"))
            self.assertIn("applied already", run(p, "--apply")[1])
            self.assertEqual(run(p, "--undo")[0], 0)
            self.assertEqual(dump(p), before)

    def test_refuses_a_changed_rank5(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            db = sqlite3.connect(p)
            db.execute("UPDATE cmcatalog SET cost = 31 WHERE id = 125")
            db.commit()
            db.close()
            before = dump(p)
            code, out = run(p, "--apply")
            self.assertNotEqual(code, 0)
            self.assertIn("REFUSED", out)
            self.assertEqual(dump(p), before)


if __name__ == "__main__":
    unittest.main()
