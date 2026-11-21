#!/usr/bin/env python3
"""tools/tradeskills_f72_20261121.py on a made database: the four tailoring modifiers, their products, Mana Infused
Thread's twins."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "tradeskills_f72_20261121.py")
STATS = ["str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT"]
MODS = {  # name: (id, products, the stats each product has)
    "Marr Blessed Studs": (501, 92, ("sta", "cha")),
    "Takish Ivory Studs": (502, 92, ("dex", "cha")),
    "Tunare Hair Thread": (503, 92, ("agi", "wis")),
    "Mindflow Thread": (504, 65, ("PoT",)),
    "Mana Infused Thread": (505, 65, ("POWMAX",)),
}


class TailoringModifiers(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, equipslot INTEGER, "
                   + ", ".join(f"{s} INTEGER" for s in STATS) + ")")
        db.execute("CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, level INTEGER, product INTEGER, ingredients TEXT)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
                   "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER)")
        pid = 900000
        for name, (mid, n, stats) in MODS.items():
            db.execute("INSERT INTO itempattern (patternid, itemname, itemdesc, equipslot) VALUES (?, ?, 'old words', -1)", (mid, name))
            for k in range(n):
                ings = json.dumps([{"item": 10 + k, "count": 1, "keep": False}, {"item": 20 + k, "count": 1, "keep": False},
                                   {"item": mid, "count": 1, "keep": True}])
                v = 55 + k if name == "Mana Infused Thread" else 5 + k % 6
                cols = ", ".join(stats)
                db.execute(f"INSERT INTO itempattern (patternid, itemname, equipslot, {cols}) VALUES (?, ?, 5, {', '.join('?' * len(stats))})",
                           (pid, f"{name} piece {k}", *([v] * len(stats))))
                db.execute("INSERT INTO craftrecipes (skill, level, product, ingredients) VALUES (2, 6, ?, ?)", (pid, ings))
                pid += 1
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def rows(self):
        db = sqlite3.connect(self.db)
        try:
            return db.execute("SELECT * FROM itempattern ORDER BY patternid").fetchall()
        finally:
            db.close()

    def product(self, name, k):
        db = sqlite3.connect(self.db)
        db.row_factory = sqlite3.Row
        try:
            return dict(db.execute("SELECT * FROM itempattern WHERE itemname = ?", (f"{name} piece {k}",)).fetchone())
        finally:
            db.close()

    def test_report_writes_nothing(self):
        before = self.rows()
        rc, out = self.run_tool()
        self.assertEqual(rc, 0, out)
        self.assertIn("report only: 345 rows", out)
        self.assertEqual(self.rows(), before)

    def test_apply_again_undo(self):
        before = self.rows()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        m = self.product("Marr Blessed Studs", 3)
        self.assertEqual((m["sta"], m["wis"], m["cha"]), (None, 8, 8))
        t = self.product("Takish Ivory Studs", 0)
        self.assertEqual((t["dex"], t["cha"], t["agi"], t["intelligence"]), (None, None, 5, 5))
        u = self.product("Tunare Hair Thread", 1)
        self.assertEqual((u["agi"], u["dex"], u["wis"]), (None, 6, 6))
        f = self.product("Mindflow Thread", 7)
        self.assertEqual((f["PoT"], f["POWMAX"]), (6, 62))  # the twin's 55 + 7
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 345)
        self.assertIn("lending wisdom and charisma", db.execute("SELECT itemdesc FROM itempattern WHERE patternid = 501").fetchone()[0])
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to do", out)
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 0, out)
        self.assertIn("345 rows back", out)
        self.assertEqual(self.rows(), before)

    def test_skip(self):
        rc, out = self.run_tool("--skip", "Mindflow Thread", "--skip", "Takish Ivory Studs", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.product("Mindflow Thread", 0)["POWMAX"], None)
        self.assertEqual(self.product("Takish Ivory Studs", 0)["dex"], 5)
        self.assertEqual(self.product("Tunare Hair Thread", 0)["dex"], 5)
        rc, out = self.run_tool("--skip", "Nothing Such")
        self.assertEqual(rc, 1, out)

    def test_refuses_a_row_not_as_read(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itempattern SET wis = 3 WHERE itemname = 'Marr Blessed Studs piece 4'")
        db.commit()
        db.close()
        before = self.rows()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(self.rows(), before)

    def test_refuses_a_count_not_as_read(self):
        db = sqlite3.connect(self.db)
        db.execute("DELETE FROM craftrecipes WHERE id = (SELECT min(id) FROM craftrecipes)")
        db.commit()
        db.close()
        rc, out = self.run_tool()
        self.assertEqual(rc, 1, out)
        self.assertIn("91 Marr Blessed Studs products here, not 92", out)

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itempattern SET wis = 99 WHERE itemname = 'Marr Blessed Studs piece 0'")
        db.commit()
        db.close()
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 1, out)
        self.assertIn("changed since", out)
        self.assertEqual(self.product("Marr Blessed Studs", 0)["wis"], 99)
        self.assertEqual(self.product("Marr Blessed Studs", 1)["sta"], 6)


if __name__ == "__main__":
    unittest.main()
