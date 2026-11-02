import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import refit_low_casters as tool  # noqa: E402


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class RefitLowCasters(unittest.TestCase):
    """tools/refit_low_casters.py on a made-up balanceclasslevel: each of its rows changed only from the values it
    had, a row a GM changed left and said, the other rows untouched, a second apply doing nothing, an undo putting
    every row back. No game data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE balanceclasslevel (class INTEGER NOT NULL, level INTEGER NOT NULL, melee_taken_pct REAL NOT NULL, "
                   "pet_hp_pct REAL NOT NULL DEFAULT 100, dealt_pct REAL NOT NULL DEFAULT 100, note TEXT NOT NULL DEFAULT '', "
                   "source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (class, level))")
        rows = [(c, l, *old) for (c, l), (old, _new) in tool.CHANGES.items()]
        rows.append((10, 6, 26, 159, 100))      # not the tool's: untouched
        rows = [r if (r[0], r[1]) != (14, 9) else (14, 9, 50, 100, 175) for r in rows]   # a GM's change: left
        for c, l, s, p, d in sorted(rows):
            db.execute("INSERT INTO balanceclasslevel VALUES (?, ?, ?, ?, ?, '', 'seed')", (c, l, s, p, d))
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "refit_low_casters.py"), "--db", self.path, "--no-backup", *args],
                              capture_output=True, text=True, check=True).stdout

    def rows(self):
        db = sqlite3.connect(self.path)
        out = {(c, l): (s, p, d) for c, l, s, p, d in db.execute(
            "SELECT class, level, melee_taken_pct, pet_hp_pct, dealt_pct FROM balanceclasslevel")}
        db.close()
        return out

    def test_apply_and_undo(self):
        out = self.tool()
        self.assertIn("8 rows to change, 1 left", out)
        self.assertIn("left as it is: balanceclasslevel 14 9", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("changed 8 rows", self.tool("--apply"))
        got = self.rows()
        for (c, l), (_old, new) in tool.CHANGES.items():
            if (c, l) != (14, 9):
                self.assertEqual(got[(c, l)], tuple(map(float, new)), f"class {c} level {l}")
        self.assertEqual(got[(14, 9)], (50.0, 100.0, 175.0), "a GM's row is left")
        self.assertEqual(got[(10, 6)], (26.0, 159.0, 100.0), "a row not the tool's is untouched")
        self.assertIn("0 rows to change", self.tool("--apply"), "a second apply finds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")


if __name__ == "__main__":
    unittest.main()
