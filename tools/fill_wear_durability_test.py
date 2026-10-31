import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FillWearDurability(unittest.TestCase):
    """tools/fill_wear_durability.py on a made-up database: each slot and level's own maxhp where three
    records carry it, else 200 a level; duration 50 unless a record has its own; held copies at 0 made
    whole; weapons, a shield that hits, the fishing gear and records with hit points left alone; a second
    apply adds nothing and an undo puts every row back. No game data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "weapondamage INTEGER, levelreq INTEGER, maxhp INTEGER, duration INTEGER)")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER, remainHP INTEGER)")
        rows = [
            # necks of level 3: 600, three of them; chests of level 3 have two (too few): 200 a level
            (1, "a", 4, 0, 3, 600, 50), (2, "b", 4, 0, 3, 600, 50), (3, "c", 4, 0, 3, 600, 50),
            (4, "d", 5, 0, 3, 900, 50), (5, "e", 5, 0, 3, 900, 50),
            # blank: a neck of 3, a chest of 3, the Pledging Stone (a neck of 1), a held item of 0
            (10, "neck3", 4, 0, 3, 0, 0), (11, "chest3", 5, 0, 3, 0, 0), (12, "Pledging Stone", 4, 0, 1, 0, 0),
            (13, "held0", 18, 0, 0, 0, 0),
            # a duration of its own: kept, the hit points given
            (14, "durable", 3, 0, 3, 0, 55),
            # the controls: a weapon, a shield that hits (the weapons' tool's), bait, one with hit points
            (20, "sword", 12, 50, 3, 0, 0), (21, "spiked shield", 13, 20, 3, 0, 0), (22, "bait", 21, 0, 1, 0, 0),
            (23, "whole", 4, 0, 3, 500, 0),
        ]
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?)", rows)
        db.executemany("INSERT INTO charinventory (serverid, patternid, remainHP) VALUES (?, ?, ?)",
                       [(103, 12, 0), (104, 12, 37), (105, 11, 0), (106, 20, 0)])
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fill_wear_durability.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=True).stdout

    def test_fill_and_undo(self):
        self.assertIn("5 worn records with no hit points (1 with a duration of their own, kept)", self.tool())
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 5 records, 2 held copies made whole", self.tool("--apply"))
        db = sqlite3.connect(self.path)
        got = dict((pid, (hp, du)) for pid, hp, du in db.execute("SELECT patternid, maxhp, duration FROM itempattern"))
        self.assertEqual(got[10], (600, 50), "a neck of 3: its slot and level's own")
        self.assertEqual(got[11], (600, 50), "a chest of 3 with two records: 200 a level")
        self.assertEqual(got[12], (200, 50), "the Pledging Stone, a neck of 1: 200")
        self.assertEqual(got[13], (200, 50), "level 0: at least 200")
        self.assertEqual(got[14], (600, 55), "a duration of its own is kept")
        for pid, why in [(20, "a weapon"), (21, "a shield that hits"), (22, "bait")]:
            self.assertEqual(got[pid], (0, 0), f"the control: {why} is left")
        self.assertEqual(got[23], (500, 0), "the control: a record with hit points of its own is left")
        held = dict(((s, p), hp) for s, p, hp in db.execute("SELECT serverid, patternid, remainHP FROM charinventory"))
        self.assertEqual(held, {(103, 12): 200, (104, 12): 37, (105, 11): 600, (106, 20): 0},
                         "only the filled records' held copies at 0 made whole")
        db.close()
        self.assertIn("wrote 0 records, 0 held copies", self.tool("--apply"), "a second apply adds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
