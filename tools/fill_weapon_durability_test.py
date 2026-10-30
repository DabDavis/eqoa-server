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


class FillWeaponDurability(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, "
                   "weapondamage INTEGER, levelreq INTEGER, maxhp INTEGER, duration INTEGER)")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER, remainHP INTEGER)")
        rows = [
            # level 5's own: 800, three of them; level 10 has two (too few): 200 x level
            (1, "a", 12, 50, 5, 800, 50), (2, "b", 12, 50, 5, 800, 50), (3, "c", 15, 50, 5, 800, 50),
            (4, "d", 12, 60, 10, 1500, 50), (5, "e", 12, 60, 10, 1500, 50),
            # blank: a level 5, a level 10, a level 0, and a blank NON-weapon (no damage) left alone
            (10, "blank5", 12, 50, 5, 0, 0), (11, "blank10", 15, 60, 10, 0, 0), (12, "blank0", 12, 10, 0, 0, 0),
            (13, "robe", 2, 0, 5, 0, 0),
            # a weapon with hit points of its own and no duration: not blank, left alone
            (14, "half", 12, 40, 5, 600, 0),
            # a duration of its own and no hit points ("HP 0/0, DUR 55"): given its level's, the duration kept
            (15, "durable", 12, 70, 5, 0, 55),
        ]
        db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?)", rows)
        db.executemany("INSERT INTO charinventory (serverid, patternid, remainHP) VALUES (?, ?, ?)",
                       [(103, 10, 0), (104, 10, 37), (105, 11, 0), (106, 15, 0)])
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fill_weapon_durability.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=True).stdout

    def test_fill_and_undo(self):
        self.assertIn("4 weapons with no hit points (1 with a duration of their own, kept)", self.tool())
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 4 records, 3 held copies made whole", self.tool("--apply"))
        db = sqlite3.connect(self.path)
        got = dict((pid, (hp, du)) for pid, hp, du in db.execute("SELECT patternid, maxhp, duration FROM itempattern"))
        self.assertEqual(got[10], (800, 50), "level 5: its records' own")
        self.assertEqual(got[11], (2000, 50), "level 10 with two records: 200 x level")
        self.assertEqual(got[12], (200, 50), "level 0: at least 200")
        self.assertEqual(got[13], (0, 0), "the control: a blank non-weapon is left")
        self.assertEqual(got[14], (600, 0), "the control: a weapon with hit points of its own is left")
        self.assertEqual(got[15], (800, 55), "a duration of its own is kept, the hit points given")
        held = dict(((s, p), hp) for s, p, hp in db.execute("SELECT serverid, patternid, remainHP FROM charinventory"))
        self.assertEqual(held, {(103, 10): 800, (104, 10): 37, (105, 11): 2000, (106, 15): 800},
                         "only held copies at 0 made whole")
        db.close()
        self.assertIn("wrote 0 records, 0 held copies", self.tool("--apply"), "a second apply adds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")

    def test_undo_reads_the_first_versions_log(self):
        # the log eqoaclassic holds (2026-10-30): itempattern rows of old values 0/0, held copies with no
        # old duration; an undo of it still puts back 0/0 and remainHP 0
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET maxhp = 800, duration = 50 WHERE patternid = 10")
        db.execute("UPDATE charinventory SET remainHP = 800 WHERE serverid = 103")
        db.execute("CREATE TABLE weapon_durability_fix_log (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, "
                   "rid INTEGER NOT NULL, old_maxhp INTEGER, old_duration INTEGER, new_maxhp INTEGER, new_duration INTEGER)")
        db.execute("INSERT INTO weapon_durability_fix_log (tbl, rid, old_maxhp, old_duration, new_maxhp, new_duration) "
                   "VALUES ('itempattern', 10, 0, 0, 800, 50)")
        db.execute("INSERT INTO weapon_durability_fix_log (tbl, rid, old_maxhp, new_maxhp) "
                   "SELECT 'charinventory', itemid, 0, 800 FROM charinventory WHERE serverid = 103")
        db.commit()
        db.close()
        self.assertIn("put back 2; left 0", self.tool("--undo"))
        self.assertEqual(dump(self.path), self.before, "the first version's log undoes")


if __name__ == "__main__":
    unittest.main()
