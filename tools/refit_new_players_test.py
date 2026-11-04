import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import refit_new_players as tool  # noqa: E402


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l and tool.LOG not in l)
    db.close()
    return out


class RefitNewPlayers(unittest.TestCase):
    """tools/refit_new_players.py on made-up balance tables of an older server (no player_melee_pct, no
    melee_dealt_pct): both columns added, each of its rows changed only from the values it had, a row a GM changed
    left and said, the other rows untouched, a second apply doing nothing, an undo putting every value back. No game
    data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL NOT NULL DEFAULT 100, "
                   "damage_pct REAL NOT NULL DEFAULT 100, con_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', "
                   "source TEXT NOT NULL DEFAULT 'gm')")
        db.execute("CREATE TABLE balanceclasslevel (class INTEGER NOT NULL, level INTEGER NOT NULL, melee_taken_pct REAL NOT NULL, "
                   "pet_hp_pct REAL NOT NULL DEFAULT 100, dealt_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', "
                   "source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (class, level))")
        self.gm_level = min(tool.LEVELS)
        for l, (old, _new) in sorted(tool.LEVELS.items()):
            hp, dmg = old[0], old[1]
            if l == self.gm_level:
                hp = hp + 1  # a GM's change: left
            db.execute("INSERT INTO balancelevel VALUES (?, ?, ?, 100, '', 'seed')", (l, hp, dmg))
        db.execute("INSERT INTO balancelevel VALUES (30, 300, 200, 100, '', 'seed')")  # not the tool's: untouched
        self.gm_cell = min(tool.CLASSES)
        for (c, l), (old, _new) in sorted(tool.CLASSES.items()):
            s, p, d = old[0], old[1], old[2]
            if (c, l) == self.gm_cell:
                s = s + 1
            db.execute("INSERT INTO balanceclasslevel VALUES (?, ?, ?, ?, ?, '', 'seed')", (c, l, s, p, d))
        db.execute("INSERT INTO balanceclasslevel VALUES (0, 30, 100, 100, 100, '', 'seed')")
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "refit_new_players.py"), "--db", self.path, "--no-backup",
                               *args], capture_output=True, text=True, check=True).stdout

    def values(self):
        db = sqlite3.connect(self.path)
        lv = {l: (h, d, p) for l, h, d, p in db.execute("SELECT level, hp_pct, damage_pct, player_melee_pct FROM balancelevel")}
        cl = {(c, l): (s, p, d, m) for c, l, s, p, d, m in db.execute(
            "SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct, melee_dealt_pct FROM balanceclasslevel")}
        db.close()
        return lv, cl

    def test_apply_and_undo(self):
        want = len(tool.LEVELS) + len(tool.CLASSES) - 2
        out = self.tool()
        self.assertIn(f"{want} rows to change, 2 left", out)
        self.assertIn(f"left as it is: balancelevel {self.gm_level}", out)
        self.assertIn("left as it is: balanceclasslevel %d %d" % self.gm_cell, out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn(f"changed {want} rows", self.tool("--apply"))
        lv, cl = self.values()
        for l, (old, new) in tool.LEVELS.items():
            if l != self.gm_level:
                self.assertEqual(lv[l], tuple(map(float, new)), f"level {l}")
        self.assertEqual(lv[self.gm_level][2], 100.0, "a GM's level row is left (its new column at 100)")
        for (c, l), (old, new) in tool.CLASSES.items():
            if (c, l) != self.gm_cell:
                self.assertEqual(cl[(c, l)], tuple(map(float, new)), f"class {c} level {l}")
        self.assertEqual(lv[30], (300.0, 200.0, 100.0), "a level row not the tool's is untouched")
        self.assertEqual(cl[(0, 30)], (100.0, 100.0, 100.0, 100.0), "a class row not the tool's is untouched")
        self.assertIn("0 rows to change", self.tool("--apply"), "a second apply finds nothing")
        self.tool("--undo")
        lv, cl = self.values()
        for l, (old, _new) in tool.LEVELS.items():
            if l != self.gm_level:
                self.assertEqual(lv[l], (float(old[0]), float(old[1]), 100.0), f"level {l} back")
        for (c, l), (old, _new) in tool.CLASSES.items():
            if (c, l) != self.gm_cell:
                self.assertEqual(cl[(c, l)], (*map(float, old[:3]), 100.0), f"class {c} level {l} back")
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT count(*) FROM sqlite_master WHERE name = ?", (tool.LOG,)).fetchone()[0], 0,
                         "the log is dropped")
        db.close()


if __name__ == "__main__":
    unittest.main()
