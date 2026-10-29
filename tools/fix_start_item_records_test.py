#!/usr/bin/env python3
"""tools/fix_start_item_records.py against a made-up database and survey: a
ration of item type 0 takes Sandstorm's type 2, masks and level, and a sword
Sandstorm's class mask, each keeping its model; an item a GM changed is left;
a second --apply is refused; --undo gives back the table as it was. No game
data.

    python3 tools/fix_start_item_records_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_start_item_records.py")
sys.path.insert(0, HERE)
import import_sandstorm_harvest as harvest  # noqa: E402

COLS = [c for _, c in harvest.ITEM_FIELDS] + list(harvest.STAT_COLUMNS.values()) + ["model", "color"]


def ware(iid, name, **kw):
    w = {k: 0 for k, _ in harvest.ITEM_FIELDS}
    w.update(id=iid, name=name, desc="", quantity=1, stats=[], wear_slot=-1, max_stack=20, rent=1, level_req=1,
             class_mask=32767, race_mask=1023)
    w.update(kw)
    return w


class FixStartItemRecords(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute(f"CREATE TABLE itempattern ({', '.join(c + (' INTEGER PRIMARY KEY' if c == 'patternid' else '') for c in COLS)})")
        db.execute("CREATE TABLE gm_audit (tbl TEXT, rowkey TEXT)")
        for pid, name, extra in ((8398, "field rations", {"itemdesc": "Simple field rations.", "maxstack": 20}),
                                 (1297, "Apprentice Sword", {"equipslot": 12, "attacktype": 1, "weapondamage": 10,
                                                             "classuse": 14, "raceuse": 823, "model": 77}),
                                 (5489, "Apprentice Staff", {"equipslot": 15, "classuse": 1})):
            row = {c: 0 for c in COLS}
            row.update(patternid=pid, itemname=name, itemdesc="", rent=1)
            row.update(extra)
            db.execute(f"INSERT INTO itempattern ({', '.join(row)}) VALUES ({', '.join('?' * len(row))})", list(row.values()))
        db.execute("""INSERT INTO gm_audit VALUES ('itempattern', '{"patternid": 5489}')""")
        db.commit()
        db.close()
        self.survey = os.path.join(self.dir.name, "survey")
        os.makedirs(self.survey)
        inv = [ware(8398, "Field Rations", desc="The basic lunch of a farmer.", p4c=2, quantity=20),
               ware(1297, "Apprentice Sword", wear_slot=12, attack_type=1, damage=10, max_stack=1, class_mask=15),
               ware(5489, "Apprentice Staff", wear_slot=15, attack_type=4, damage=10, max_stack=1)]
        with open(os.path.join(self.survey, "0-0-1.json"), "w") as f:
            json.dump({"combo": {"race": 0, "class": 0, "humtype": 1}, "inventory": inv}, f)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--measured", self.survey, *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def rows(self):
        c = sqlite3.connect(self.db)
        try:
            return c.execute("SELECT * FROM itempattern ORDER BY patternid").fetchall()
        finally:
            c.close()

    def get(self, pid, cols):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        finally:
            c.close()

    def test_apply_and_undo(self):
        before = self.rows()
        out = self.run_tool().stdout
        self.assertIn("2 starting items to give Sandstorm's record, 1 left", out)
        self.assertIn("left as it is: Apprentice Staff (a GM changed it)", out)
        self.run_tool("--apply")
        self.assertEqual(self.get(8398, ["itemname", "procanim", "classuse", "raceuse", "levelreq", "itemdesc"]),
                         ("Field Rations", 2, 32767, 1023, 1, "The basic lunch of a farmer."))
        self.assertEqual(self.get(1297, ["classuse", "raceuse", "model"]), (15, 1023, 77))  # its model kept
        self.assertEqual(self.get(5489, ["classuse"]), (1,))
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.rows(), before)


if __name__ == "__main__":
    unittest.main()
