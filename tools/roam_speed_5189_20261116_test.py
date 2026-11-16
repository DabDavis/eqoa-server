#!/usr/bin/env python3
"""tools/roam_speed_5189_20261116.py on a database of its own: report only writes nothing; --apply adds the speed
column to a table without it, updates a name's row (its other columns kept) and inserts a row as its kind for a name
with none, a gm_audit line each; a second run nothing; --undo exact (the rows as they were)."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "roam_speed_5189_20261116.py")


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0,
            water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
        INSERT INTO npcmove VALUES ('alligator', 1, 0, 'both', 'in and out of the water (alligator)', 'seed'),
            ('wolf', 1, 0, '', 'a wolf', 'gm');
    """)
    db.commit()
    db.close()


def run(path, *args, check=True):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True,
                          check=check)


def rows(path):
    db = sqlite3.connect(path)
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcmove)")]
    out = {r[0]: dict(zip(cols, r)) for r in db.execute("SELECT * FROM npcmove")}
    audits = db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
    db.close()
    return out, audits


class Test(unittest.TestCase):
    def test_report_apply_again_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before, _ = rows(p)
            r = run(p)
            self.assertIn("report only", r.stdout)
            self.assertEqual(rows(p), (before, 0))

            r = run(p, "--apply")
            self.assertIn("applied", r.stdout)
            after, audits = rows(p)
            self.assertEqual(after["gharial"]["speed"], 3.0)
            self.assertEqual((after["gharial"]["roam"], after["gharial"]["radius"], after["gharial"]["water"]), (-1, 0, ""))
            al = after["alligator"]
            self.assertEqual((al["speed"], al["roam"], al["water"], al["note"], al["source"]),
                             (2.0, 1, "both", "in and out of the water (alligator)", "seed"))
            self.assertEqual(after["wolf"]["speed"], 0)
            self.assertEqual(audits, len(after) - 1)  # one a croc name, none for the wolf

            r = run(p, "--apply")
            self.assertIn("applied already", r.stdout)
            self.assertEqual(rows(p)[1], audits)

            r = run(p, "--undo")
            self.assertIn("put back", r.stdout)
            back, _ = rows(p)
            self.assertEqual(set(back), {"alligator", "wolf"})
            self.assertEqual({k: {c: v for c, v in r.items() if c != "speed"} for k, r in back.items()}, before)
            self.assertEqual(back["alligator"]["speed"], 0)


if __name__ == "__main__":
    unittest.main()
