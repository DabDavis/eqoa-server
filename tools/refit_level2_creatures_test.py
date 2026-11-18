import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import refit_level2_creatures as tool  # noqa: E402


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l and tool.LOG not in l)
    db.close()
    return out


class RefitLevel2Creatures(unittest.TestCase):
    """tools/refit_level2_creatures.py on made-up balance tables: level 2's damage_pct 133 -> 79 by default; with
    --low-shares the healers' and casters' 1-12 shares the lesser of the archetype's and the fit's, a row a GM changed
    left and said, a fit above the archetype share (the cleric of 4's 138) never written, the other rows untouched, a
    second apply doing nothing, an undo putting every value back. No game data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL NOT NULL DEFAULT 100, "
                   "damage_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')")
        db.execute("CREATE TABLE balanceclasslevel (class INTEGER NOT NULL, level INTEGER NOT NULL, melee_taken_pct REAL NOT NULL, "
                   "pet_hp_pct REAL NOT NULL DEFAULT 100, dealt_pct REAL NOT NULL DEFAULT 100, melee_dealt_pct REAL NOT NULL DEFAULT 100, "
                   "note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'seed', PRIMARY KEY (class, level))")
        for l, dmg in ((1, 79), (2, 133), (3, 61)):
            db.execute("INSERT INTO balancelevel VALUES (?, 200, ?, '', 'seed')", (l, dmg))
        for c in range(15):
            for l in range(1, 14):
                db.execute("INSERT INTO balanceclasslevel (class, level, melee_taken_pct) VALUES (?, ?, ?)",
                           (c, l, tool.FLAT.get(c, 85.0)))
        db.execute("UPDATE balanceclasslevel SET melee_taken_pct = 50 WHERE class = 13 AND level = 1")  # a GM's
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "refit_level2_creatures.py"), "--db", self.path,
                               "--no-backup", *args], capture_output=True, text=True, check=True).stdout

    def values(self):
        db = sqlite3.connect(self.path)
        lv = dict(db.execute("SELECT level, damage_pct FROM balancelevel"))
        cl = {(c, l): s for c, l, s in db.execute("SELECT class, level, melee_taken_pct FROM balanceclasslevel")}
        db.close()
        return lv, cl

    def test_default_is_level_2_alone(self):
        out = self.tool()
        self.assertIn("balancelevel 2: damage_pct 133 -> 79", out)
        self.assertIn("1 rows to change, 0 left", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.tool("--apply")
        lv, cl = self.values()
        self.assertEqual(lv, {1: 79.0, 2: 79.0, 3: 61.0})
        self.assertEqual(cl[(8, 4)], 95.0, "no share moves without --low-shares")
        self.assertIn("0 rows to change", self.tool())
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before)

    def test_low_shares_and_undo(self):
        want = 1 + len(tool.shares()) - 1  # the wizard of 1 a GM's
        out = self.tool("--low-shares")
        self.assertIn(f"{want} rows to change, 1 left", out)
        self.assertIn("left as it is: balanceclasslevel 13 1", out)
        self.tool("--low-shares", "--apply")
        lv, cl = self.values()
        self.assertEqual(lv[2], 79.0)
        self.assertEqual([cl[(8, l)] for l in range(1, 13)], [17, 31, 63, 78, 95, 95, 95, 95, 95, 95, 95, 95],
                         "the shaman: the fit under the archetype share, the share where the fit was over it")
        self.assertEqual(cl[(9, 4)], 95.0, "the cleric of 4's fit (138) is over the share: never raised")
        self.assertEqual(cl[(13, 1)], 50.0, "a GM's row is left")
        self.assertEqual(cl[(10, 13)], 115.0, "13 and up untouched")
        for c in range(7):
            self.assertEqual({cl[(c, l)] for l in range(1, 14)}, {85.0}, f"class {c} untouched")
        self.assertIn("0 rows to change", self.tool("--low-shares", "--apply"), "a second apply finds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "the undo puts every value back")


if __name__ == "__main__":
    unittest.main()
