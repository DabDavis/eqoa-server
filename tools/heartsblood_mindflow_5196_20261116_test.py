#!/usr/bin/env python3
"""tools/heartsblood_mindflow_5196_20261116.py on a made database: 17 pieces x trade levels 6-10, each with its
Heartsblood, Mindflow and single-stat twins' products; a live record (no HoT, higher HP) among them."""
import hashlib
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "heartsblood_mindflow_5196_20261116.py")
STA = {6: 14, 7: 18, 8: 18, 9: 20, 10: 20}
HP = {6: 55, 7: 60, 8: 65, 9: 70, 10: 75}
HOT = {6: 4, 7: 6, 8: 8, 9: 10, 10: 10}


class HeartsbloodArmor(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, levelreq INTEGER, "
                   "AC INTEGER, str INTEGER, sta INTEGER, wis INTEGER, AR INTEGER, HPMAX INTEGER, POWMAX INTEGER, HoT INTEGER, PoT INTEGER)")
        db.execute("CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, level INTEGER, name TEXT, product INTEGER, "
                   "ingredients TEXT)")
        db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
                   "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER)")
        db.execute("CREATE TABLE charinventory (serverid INTEGER, patternid INTEGER)")
        mods = {"heartsblood alloy": 1116, "rhodizite": 4909, "Achroite": 701569, "Titan Alloy": 701570, "mindflow crystals": 1180,
                "Apatite": 701567, "Mana Infused Alloy": 701571}
        for name, pid in mods.items():
            db.execute("INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (?, ?, -1)", (pid, name))
        # a jewel's Heartsblood recipe (another skill): never touched
        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, HoT) VALUES (5, 'Seralite Heartsblood Ring', 8, 3)")
        db.execute("INSERT INTO craftrecipes (skill, level, name, product, ingredients) VALUES (3, 10, 'r', 5, ?)",
                   (json.dumps([{"item": 1, "keep": False}, {"item": 1116, "keep": True}]),))
        pid = 700000
        for piece in range(17):
            mold = 27000 + piece
            for tl in range(6, 11):
                ore = 20300 + tl
                for mod, stats in (("heartsblood alloy", {"HoT": HOT[tl]}), ("rhodizite", {"sta": STA[tl]}),
                                   ("Achroite", {"AR": STA[tl]}), ("Titan Alloy", {"HPMAX": HP[tl]}),
                                   ("mindflow crystals", {"PoT": HOT[tl]}), ("Apatite", {"wis": STA[tl]}),
                                   ("Mana Infused Alloy", {"POWMAX": HP[tl]})):
                    pid += 1
                    if mod == "heartsblood alloy" and piece == 0 and tl == 10:
                        pid = 1042  # the live record: Sta, AR and HP, no HoT
                        db.execute("INSERT INTO itempattern (patternid, itemname, equipslot, levelreq, AC, str, sta, AR, HPMAX, HoT) "
                                   "VALUES (1042, 'Heartsblood Seralite Breastplate', 5, 48, 378, 0, 18, 18, 81, 0)")
                    else:
                        cols = ", ".join(stats)
                        db.execute(f"INSERT INTO itempattern (patternid, itemname, equipslot, levelreq, AC, {cols}) "
                                   f"VALUES (?, ?, 5, ?, 100, {', '.join('?' * len(stats))})",
                                   (pid, f"{mod} piece {piece} tl {tl}", 5 * tl - 2, *stats.values()))
                    ings = [{"item": ore, "count": 1, "keep": False}, {"item": mold, "count": 1, "keep": False},
                            {"item": mods[mod], "count": 1, "keep": True}]
                    db.execute("INSERT INTO craftrecipes (skill, level, name, product, ingredients) VALUES (1, ?, ?, ?, ?)",
                               (tl, f"{mod} {piece} {tl}", pid, json.dumps(ings)))
                    if pid == 1042:
                        pid = 800000
        db.execute("INSERT INTO charinventory VALUES (7, 1042)")
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
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 170 records", r.stdout)
        self.assertEqual(self.items(), before)
        r = self.tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        # a grid product: its twins' Sta, AR, HP, its HoT kept
        self.assertEqual(self.q("SELECT sta, AR, HPMAX, HoT FROM itempattern WHERE itemname = 'heartsblood alloy piece 3 tl 6'"),
                         [(14, 14, 55, 4)])
        self.assertEqual(self.q("SELECT wis, POWMAX, AR, PoT, sta FROM itempattern WHERE itemname = 'mindflow crystals piece 3 tl 9'"),
                         [(20, 70, 20, 10, None)])
        # the live record: nothing lowered (HP 81 over 75), Prima's HoT given
        self.assertEqual(self.q("SELECT AC, sta, AR, HPMAX, HoT FROM itempattern WHERE patternid = 1042"), [(378, 20, 20, 81, 10)])
        self.assertEqual(self.q("SELECT HoT, sta FROM itempattern WHERE patternid = 5"), [(3, None)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(170,)])
        self.assertIn("nothing to do", self.tool("--apply").stdout)
        r = self.tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.items(), before)

    def test_a_missing_twin_writes_nothing(self):
        c = sqlite3.connect(self.db)
        c.execute("DELETE FROM craftrecipes WHERE name = 'Titan Alloy 4 8'")
        c.commit()
        c.close()
        before = self.items()
        r = self.tool("--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("no titan alloy twin", r.stdout)
        self.assertEqual(self.items(), before)


if __name__ == "__main__":
    unittest.main()
