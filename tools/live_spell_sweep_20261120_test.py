#!/usr/bin/env python3
"""live_spell_sweep_20261120.py on a small Live: an Archivist scroll no merchant, loot table or quest gives goes to its
class's guild merchant, priced by its level's stocked scrolls when worth 0; a dropped one and a stocked one do not; a
merchant past 40 gets none; a second apply does nothing; --undo puts everything back; a database with Archivists
(Heirloom) refuses."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "live_spell_sweep_20261120.py")
SCHEMA = """CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER, levelreq INTEGER,
    itemcost INTEGER, equipslot INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER NOT NULL DEFAULT 0,
    remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0);
CREATE TABLE loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, rewarditems TEXT, byclass TEXT, choices TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);"""


def run(p, *a):
    return subprocess.run([sys.executable, TOOL, "--db", p, *a], capture_output=True, text=True)


def state(p):
    db = sqlite3.connect(p)
    s = (db.execute("SELECT npcid, patternid, listnumber FROM npcitem ORDER BY itemid").fetchall(),
         db.execute("SELECT patternid, itemcost FROM itempattern ORDER BY 1").fetchall())
    db.close()
    return s


class Sweep(unittest.TestCase):
    def make(self, d, full=False, archivist=False):
        p = os.path.join(d, "eqoa.db")
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, -1)", [
            (100, "Warrior 45", 1, 45, 900), (101, "Wizard 45", 8192, 45, 900),       # the guild merchants' own
            (200, "Supernova", 8192, 60, 0), (201, "Call to Victory", 1, 55, 0),      # unsold, worth 0
            (202, "Boot", 1, 51, 0),                                                  # unsold but dropped
            (203, "Stocked 55", 1, 55, 2593), (204, "Stocked 60", 8192, 60, 3086)])   # stocked, the prices
        db.executemany("INSERT INTO npcs VALUES (?, ?)", [(1, "Merchant Harriston"), (2, "Keeper Ulhan"), (3, "a vendor")]
                       + ([(4, "Spell Archivist")] if archivist else []))
        db.executemany("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (?, ?, ?)",
                       [(1, 100, 0), (2, 101, 0), (3, 203, 0), (3, 204, 1)] + ([(2, 9000 + i, i + 1) for i in range(39)] if full else []))
        db.execute("INSERT INTO loottables (tablename, patternid) VALUES ('t303 51-54 spells', 202)")
        db.commit()
        db.close()
        arch = os.path.join(d, "arch.txt")
        open(arch, "w").write("200\n201\n202\n203\n")
        return p, arch

    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p, arch = self.make(d)
            start = state(p)
            r = run(p, "--archivist", arch, "--apply")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            inv, items = state(p)
            self.assertIn((1, 201, 1), inv)
            self.assertIn((2, 200, 1), inv)
            self.assertFalse(any(row[1] == 202 for row in inv), "a dropped scroll was stocked")
            self.assertEqual(dict(items)[201], 2593)
            self.assertEqual(dict(items)[200], 3086)
            self.assertIn("nothing to do", run(p, "--archivist", arch, "--apply").stdout)
            self.assertEqual(run(p, "--undo").returncode, 0)
            self.assertEqual(state(p), start)

    def test_full_list_gets_none(self):
        with tempfile.TemporaryDirectory() as d:
            p, arch = self.make(d, full=True)
            r = run(p, "--archivist", arch, "--apply")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertIn("would pass 40", r.stdout)
            self.assertFalse(any(row[0] == 2 and row[1] == 200 for row in state(p)[0]))

    def test_heirloom_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p, arch = self.make(d, archivist=True)
            start = state(p)
            self.assertEqual(run(p, "--archivist", arch, "--apply").returncode, 1)
            self.assertEqual(state(p), start)


if __name__ == "__main__":
    unittest.main()
