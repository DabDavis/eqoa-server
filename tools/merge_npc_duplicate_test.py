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


class MergeNPCDuplicate(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, "
                   "y_coord REAL, z_coord REAL, facing INTEGER, world INTEGER, torso INTEGER, primary_weapon INTEGER)")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", [
            (1630, "Guard Syon", 0, 24974.6, 34.9, 9451.2, 204, 0, 6, 3707576513),
            (2983, "Guard Syon", 0, 25201.2, 36.2, 9442.2, 0, 0, None, None),
            (3000, "Guard Other", 0, 1.0, 2.0, 3.0, 5, 0, 1, 2),
            (100397, "Guard Nosun", 0, 25200.6, 36.0, 9425.3, 63, 0, 6, 1),
        ])
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args, check=True):
        return subprocess.run([sys.executable, os.path.join(HERE, "merge_npc_duplicate.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=check)

    def test_the_dressed_guard_takes_the_naked_ones_place(self):
        self.tool("--keep", "1630", "--at", "2983")
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.tool("--keep", "1630", "--at", "2983", "--apply")
        db = sqlite3.connect(self.path)
        rows = db.execute("SELECT id, x_coord, z_coord, facing, torso, primary_weapon FROM npcs WHERE npc_name = 'Guard Syon'").fetchall()
        db.close()
        self.assertEqual(rows, [(1630, 25201.2, 9442.2, 0, 6, 3707576513)], "one guard, dressed, where the naked one stood")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts both back")

    def test_facing_as_the_guard_beside_him(self):
        self.tool("--keep", "1630", "--at", "2983", "--facing-of", "100397", "--apply")
        db = sqlite3.connect(self.path)
        got = db.execute("SELECT x_coord, z_coord, facing FROM npcs WHERE id = 1630").fetchone()
        db.close()
        self.assertEqual(got, (25201.2, 9442.2, 63), "the naked one's place, Guard Nosun's facing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts both back, the facing too")

    def test_two_names_are_refused(self):
        r = self.tool("--keep", "1630", "--at", "3000", "--apply", check=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(dump(self.path), self.before, "nothing written")


if __name__ == "__main__":
    unittest.main()
