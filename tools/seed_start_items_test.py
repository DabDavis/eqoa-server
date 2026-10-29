#!/usr/bin/env python3
"""tools/seed_start_items.py against a made-up database and survey: a combo's
items are found by Sandstorm's id where our record of it has the name, else
by name, and one this database lacks is made from Sandstorm's record; rows
are written per combo with their counts; a second --apply is refused; --undo
takes the rows and the made item back (one a character holds stays). No game
data.

    python3 tools/seed_start_items_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_start_items.py")
sys.path.insert(0, HERE)
import import_sandstorm_harvest as harvest  # noqa: E402

COLS = [c for _, c in harvest.ITEM_FIELDS] + list(harvest.STAT_COLUMNS.values()) + ["model", "color"]


def ware(iid, name, qty, kind=0):
    w = {k: 0 for k, _ in harvest.ITEM_FIELDS}
    w.update(id=iid, name=name, desc="", quantity=qty, stats=[], attack_type=kind, wear_slot=12 if kind else -1,
             max_stack=20, rent=1, level_req=1, class_mask=32767, race_mask=1023, equip_slot=-1)
    return w


class SeedStartItems(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute(f"CREATE TABLE itempattern ({', '.join(c + (' INTEGER PRIMARY KEY' if c == 'patternid' else '') for c in COLS)})")
        db.execute("""CREATE TABLE startitems (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER NOT NULL, class INTEGER NOT NULL,
            humtype INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL, count INTEGER NOT NULL DEFAULT 1,
            note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER)")
        for pid, name in ((1297, "Apprentice Sword"), (5000, "travel cuisine")):  # 5000: its name, another id
            row = {c: 0 for c in COLS}
            row.update(patternid=pid, itemname=name, itemdesc="")
            db.execute(f"INSERT INTO itempattern ({', '.join(row)}) VALUES ({', '.join('?' * len(row))})", list(row.values()))
        db.commit()
        db.close()
        self.survey = os.path.join(self.dir.name, "survey")
        os.makedirs(self.survey)
        for (race, cls, hum), inv in {(2, 3, 0): [ware(8392, "travel cuisine", 20), ware(1297, "Apprentice Sword", 1, 1)],
                                      (10, 0, 0): [ware(26435, "toad snacks", 20)]}.items():
            with open(os.path.join(self.survey, f"{race}-{cls}-{hum}.json"), "w") as f:
                json.dump({"combo": {"race": race, "class": cls, "humtype": hum}, "inventory": inv}, f)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--measured", self.survey, "--no-backup", *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def q(self, sql):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql).fetchall()
        finally:
            c.close()

    def test_apply_and_undo(self):
        self.assertIn("2 combos, 3 rows, 1 items to make", self.run_tool().stdout)
        self.run_tool("--apply")
        self.assertEqual(self.q("SELECT race, class, humtype, patternid, count FROM startitems ORDER BY id"),
                         [(2, 3, 0, 5000, 20), (2, 3, 0, 1297, 1), (10, 0, 0, 26435, 20)])
        self.assertEqual(self.q("SELECT itemname FROM itempattern WHERE patternid = 26435"), [("toad snacks",)])
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.q("SELECT count(*) FROM startitems"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE patternid = 26435"), [(0,)])


if __name__ == "__main__":
    unittest.main()
