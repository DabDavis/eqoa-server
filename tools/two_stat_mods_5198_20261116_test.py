#!/usr/bin/env python3
"""tools/two_stat_mods_5198_20261116.py on a made database: 115 Enstatite (Str and Int alike) and 115 Takish Ivory
(Dex and Cha alike) armour products."""
import hashlib
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "two_stat_mods_5198_20261116.py")


class Enstatite(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, AC INTEGER, "
                   "str INTEGER, intelligence INTEGER, sta INTEGER, dex INTEGER, cha INTEGER, agi INTEGER)")
        db.execute("CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, level INTEGER, product INTEGER, ingredients TEXT)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
                   "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER)")
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (701574, 'Enstatite', -1)")
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (701573, 'Lazulite', -1)")
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (701575, 'Takish Ivory', -1)")
        ings = lambda mod: json.dumps([{"item": 20300, "count": 1, "keep": False}, {"item": mod, "count": 1, "keep": True}])
        for k in range(115):
            v = 6 + k % 5
            db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, AC, str, intelligence) VALUES (?, ?, 5, 100, ?, ?)",
                       (800000 + k, f"enstatite piece {k}", v, v))
            db.execute("INSERT INTO craftrecipes (skill, level, product, ingredients) VALUES (1, 4, ?, ?)", (800000 + k, ings(701574)))
            db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, AC, dex, cha) VALUES (?, ?, 5, 100, ?, ?)",
                       (810000 + k, f"takish piece {k}", v, v))
            db.execute("INSERT INTO craftrecipes (skill, level, product, ingredients) VALUES (1, 4, ?, ?)", (810000 + k, ings(701575)))
        # another modifier's product: never touched
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, AC, intelligence, cha) VALUES (900000, 'lazulite piece', 5, 100, 6, 6)")
        db.execute("INSERT INTO craftrecipes (skill, level, product, ingredients) VALUES (1, 4, 900000, ?)", (ings(701573),))
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

    def items(self):
        return hashlib.sha256(repr(self.q("SELECT * FROM itempattern ORDER BY patternid")).encode()).hexdigest()

    def test_apply_and_undo(self):
        before = self.items()
        r = self.tool()
        self.assertIn("report only: 230 records", r.stdout, r.stdout + r.stderr)
        self.assertEqual(self.tool("--apply").returncode, 0)
        self.assertEqual(self.q("SELECT str, intelligence, sta FROM itempattern WHERE patternid = 800003"), [(9, None, 9)])
        self.assertEqual(self.q("SELECT dex, cha, agi FROM itempattern WHERE patternid = 810004"), [(10, None, 10)])
        self.assertEqual(self.q("SELECT intelligence, cha FROM itempattern WHERE patternid = 900000"), [(6, 6)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(230,)])
        self.assertIn("nothing to do", self.tool("--apply").stdout)
        self.assertEqual(self.tool("--undo").returncode, 0)
        self.assertEqual(self.items(), before)

    def test_a_record_off_the_pair_writes_nothing(self):
        c = sqlite3.connect(self.db)
        c.execute("UPDATE itempattern SET intelligence = 3 WHERE patternid = 800010")
        c.commit()
        c.close()
        before = self.items()
        r = self.tool("--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("not the grid's pair", r.stdout)
        self.assertEqual(self.items(), before)


if __name__ == "__main__":
    unittest.main()
