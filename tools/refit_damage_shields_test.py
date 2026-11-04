import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import refit_damage_shields as tool  # noqa: E402


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class RefitDamageShields(unittest.TestCase):
    """tools/refit_damage_shields.py on a made-up balanceclasslevel: each of its rows changed only from the values it
    had, a row it adds added, a row a GM changed left and said, the other rows untouched, a second apply doing nothing,
    an undo putting the table back as it was. No game data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE balanceclasslevel (class INTEGER NOT NULL, level INTEGER NOT NULL, melee_taken_pct REAL NOT NULL, "
                   "pet_hp_pct REAL NOT NULL DEFAULT 100, dealt_pct REAL NOT NULL DEFAULT 100, melee_dealt_pct REAL NOT NULL DEFAULT 100, "
                   "note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (class, level))")
        changed = sorted(k for k, (old, _new) in tool.CHANGES.items() if old is not None)
        self.gm = changed[0]
        rows = [(0, 30, 100, 100, 100)]  # not the tool's: untouched
        for (c, l), (old, _new) in tool.CHANGES.items():
            if old is None:
                continue  # a row the fit adds: none there
            s, p, d = old
            if (c, l) == self.gm:
                s = s + 1  # a GM's change: left
            rows.append((c, l, s, p, d))
        for r in sorted(rows):  # in (class, level) order, as the tunings keep the table
            db.execute("INSERT INTO balanceclasslevel VALUES (?, ?, ?, ?, ?, 100, '', 'seed')", r)
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "refit_damage_shields.py"), "--db", self.path, "--no-backup",
                               *args], capture_output=True, text=True, check=True).stdout

    def rows(self):
        db = sqlite3.connect(self.path)
        out = {(c, l): (s, p, d) for c, l, s, p, d in db.execute(
            "SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM balanceclasslevel")}
        db.close()
        return out

    def test_apply_and_undo(self):
        want = len(tool.CHANGES) - 1
        out = self.tool()
        self.assertIn(f"{want} rows to change, 1 left", out)
        self.assertIn("left as it is: balanceclasslevel %d %d" % self.gm, out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn(f"changed {want} rows", self.tool("--apply"))
        got = self.rows()
        for (c, l), (_old, new) in tool.CHANGES.items():
            if (c, l) != self.gm:
                self.assertEqual(got[(c, l)], tuple(map(float, new)), f"class {c} level {l}")
        self.assertEqual(got[(0, 30)], (100.0, 100.0, 100.0), "a row not the tool's is untouched")
        self.assertIn("0 rows to change", self.tool("--apply"), "a second apply finds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts the table back")


if __name__ == "__main__":
    unittest.main()
