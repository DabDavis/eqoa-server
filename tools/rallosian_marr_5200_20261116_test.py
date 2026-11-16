#!/usr/bin/env python3
"""tools/rallosian_marr_5200_20261116.py on a made database: the studs, 92 tailoring products (Str and Int alike), the
modifiers pool with Marr Blessed Studs' row."""
import hashlib
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "rallosian_marr_5200_20261116.py")


class RallosianStuds(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, equipslot INTEGER, "
                   "AC INTEGER, str INTEGER, intelligence INTEGER, sta INTEGER, dex INTEGER, cha INTEGER, wis INTEGER, agi INTEGER)")
        db.execute("CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, level INTEGER, product INTEGER, ingredients TEXT)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
                   "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER)")
        db.execute("CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT NOT NULL, region TEXT NOT NULL DEFAULT '*', "
                   "npc_name TEXT NOT NULL DEFAULT '', patternid INTEGER NOT NULL, weight INTEGER NOT NULL DEFAULT 1, "
                   "minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 100, source TEXT NOT NULL DEFAULT 'gm')")
        db.execute("INSERT INTO itempattern (patternid, itemname, itemdesc, equipslot) VALUES (500, 'Rallosian Studs', "
                   "'... lending strength and intelligence.', -1)")
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (501, 'Marr Blessed Studs', -1)")
        db.execute("INSERT INTO lootpool (category, patternid, weight, minlevel, maxlevel) VALUES ('modifiers', 501, 9, 10, 60)")
        ings = json.dumps([{"item": 1, "count": 1, "keep": False}, {"item": 500, "count": 1, "keep": True}])
        for k in range(92):
            v = 5 + k % 6
            db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, AC, str, intelligence) VALUES (?, ?, 5, 10, ?, ?)",
                       (900000 + k, f"Rallosian piece {k}", v, v))
            db.execute("INSERT INTO craftrecipes (skill, level, product, ingredients) VALUES (2, 4, ?, ?)", (900000 + k, ings))
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (502, 'Marr Blessed Alloy', -1)")
        alloy = json.dumps([{"item": 2, "count": 1, "keep": False}, {"item": 502, "count": 1, "keep": True}])
        for k in range(115):
            v = 6 + k % 5
            db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, AC, sta, cha) VALUES (?, ?, 5, 85, ?, ?)",
                       (910000 + k, f"marr blessed piece {k}", v, v))
            db.execute("INSERT INTO craftrecipes (skill, level, product, ingredients) VALUES (1, 4, ?, ?)", (910000 + k, alloy))
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (503, 'Lazulite', -1)")
        laz = json.dumps([{"item": 2, "count": 1, "keep": False}, {"item": 503, "count": 1, "keep": True}])
        for k in range(115):
            v = 6 + k % 5
            db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, AC, agi, wis) VALUES (?, ?, 5, 85, ?, ?)",
                       (920000 + k, f"lazulite piece {k}", v, v))
            db.execute("INSERT INTO craftrecipes (skill, level, product, ingredients) VALUES (1, 4, ?, ?)", (920000 + k, laz))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, *args], capture_output=True, text=True)

    def q(self, sql):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql).fetchall()
        finally:
            c.close()

    def state(self):
        return hashlib.sha256(repr(self.q("SELECT * FROM itempattern ORDER BY patternid") +
                                   self.q("SELECT * FROM lootpool ORDER BY id")).encode()).hexdigest()

    def test_apply_and_undo_without_the_drop(self):
        before = self.state()
        r = self.tool()
        self.assertIn("report only: 323 rows", r.stdout, r.stdout + r.stderr)
        self.assertIn("--with-drop adds row", r.stdout)
        self.assertEqual(self.tool("--apply").returncode, 0)
        self.assertEqual(self.q("SELECT str, intelligence, sta, dex FROM itempattern WHERE patternid = 900003"), [(None, None, 8, 8)])
        self.assertIn("stamina and dexterity", self.q("SELECT itemdesc FROM itempattern WHERE patternid = 500")[0][0])
        self.assertEqual(self.q("SELECT sta, cha, wis FROM itempattern WHERE patternid = 910004"), [(10, None, 10)])
        self.assertEqual(self.q("SELECT agi, wis, sta FROM itempattern WHERE patternid = 920003"), [(9, None, 9)])
        self.assertEqual(self.q("SELECT count(*) FROM lootpool WHERE patternid = 500"), [(0,)], "no drop unless asked")
        self.assertIn("nothing to do", self.tool("--apply").stdout)
        self.assertEqual(self.tool("--undo").returncode, 0)
        self.assertEqual(self.state(), before)

    def test_the_drop_as_marr_blessed_studs(self):
        before = self.state()
        self.assertEqual(self.tool("--with-drop", "--apply").returncode, 0)
        self.assertEqual(self.q("SELECT category, region, weight, minlevel, maxlevel FROM lootpool WHERE patternid = 500"),
                         [("modifiers", "*", 9, 10, 60)])
        self.assertIn("in the 'modifiers' pool already", self.tool("--with-drop").stdout)
        self.assertEqual(self.tool("--undo").returncode, 0)
        self.assertEqual(self.state(), before)


if __name__ == "__main__":
    unittest.main()
