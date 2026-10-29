#!/usr/bin/env python3
"""tools/fix_quest_items.py against a made-up database and harvest: a quest
item made as a copy of a higher item takes Sandstorm's record (its level,
classes, races, icon, stats), its id, name and model kept; an item Sandstorm
gives two records is left; an item no quest made is never touched; a second
--apply is refused and --undo gives back the table as it was. No game data.

    python3 tools/fix_quest_items_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_quest_items.py")
sys.path.insert(0, HERE)
import import_sandstorm_harvest as harvest  # noqa: E402

COLS = [c for _, c in harvest.ITEM_FIELDS] + list(harvest.STAT_COLUMNS.values()) + ["model", "color"]


def ware(name, lvl, classes, races, icon, stats):
    w = {k: 0 for k, _ in harvest.ITEM_FIELDS}
    w.update(name=name, desc=f"{name}.", level_req=lvl, class_mask=classes, race_mask=races, icon=icon,
             wear_slot=7, max_stack=1, max_hp=200, durability=50, rent=1, stats=stats, id=99999)
    return w


class FixQuestItems(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute(f"CREATE TABLE itempattern ({', '.join(c + (' INTEGER PRIMARY KEY' if c == 'patternid' else '') for c in COLS)})")
        db.execute("CREATE TABLE quest_import_items (patternid INTEGER PRIMARY KEY, name TEXT NOT NULL, file TEXT NOT NULL)")
        for pid, name, lvl in ((700419, "Bone Bracers", 42), (700420, "Twice Seen", 10), (2850, "Whale Bone Bracers", 42)):
            row = {c: 0 for c in COLS}
            row.update(patternid=pid, itemname=name, itemdesc="", levelreq=lvl, classuse=527, raceuse=1023,
                       itemicon=-343493526, equipslot=7, trade=1, sta=10, model=5, color=7)
            db.execute(f"INSERT INTO itempattern ({', '.join(row)}) VALUES ({', '.join('?' * len(row))})", list(row.values()))
        db.executemany("INSERT INTO quest_import_items VALUES (?, ?, 'q.json')", [(700419, "Bone Bracers"), (700420, "Twice Seen")])
        db.commit()
        db.close()
        self.harvest = os.path.join(self.dir.name, "h.json")
        with open(self.harvest, "w") as f:
            json.dump({"merchants": [{"name": "Merchant", "wares": [
                ware("Bone Bracers", 1, 8, 4, 68842949, [[0, 3]]),
                ware("Twice Seen", 1, 8, 4, 1, []), ware("Twice Seen", 2, 8, 4, 1, []),
                ware("Whale Bone Bracers", 1, 8, 4, 1, [])]}]}, f)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--prices", self.harvest, *args],
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

    def row(self, pid, *cols):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        finally:
            c.close()

    def test_apply_and_undo(self):
        before = self.rows()
        out = self.run_tool().stdout
        self.assertIn("1 quest items to give Sandstorm's record, 1 left", out)
        self.assertIn("left as it is: Twice Seen (Sandstorm has 2 records of that name)", out)
        self.assertEqual(self.rows(), before)
        self.run_tool("--apply")
        self.assertEqual(self.row(700419, "itemname", "levelreq", "classuse", "raceuse", "itemicon", "trade", "str", "sta", "model", "color"),
                         ("Bone Bracers", 1, 8, 4, 68842949, 0, 3, None, 5, 7))
        self.assertEqual(self.row(2850, "levelreq", "classuse"), (42, 527))  # no quest made it: untouched
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        self.run_tool("--undo")
        self.assertEqual(self.rows(), before)


if __name__ == "__main__":
    unittest.main()
