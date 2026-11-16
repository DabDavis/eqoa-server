import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_weather.py")


def run(*args):
    return subprocess.run([sys.executable, TOOL, *args], capture_output=True, text=True, check=True).stdout


def rows(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in ("weatherprofiles", "weatherzones", "weatherclimates")}
    db.close()
    return out


class SeedWeather(unittest.TestCase):
    """tools/seed_weather.py on an empty database: a report writes nothing; --apply writes 22 profiles, 13 zones and
    3 climates, makes the tables if need be, keeps a GM's row and a second apply writes nothing; --undo deletes the
    seeded rows but one a GM changed since. No game data."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE weatherprofiles (profile INTEGER PRIMARY KEY, climate TEXT NOT NULL DEFAULT 'temperate', "
                   "note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')")
        db.execute("INSERT INTO weatherprofiles (profile, climate, note) VALUES (7, 'desert', 'a GM thinks so')")
        db.commit()
        db.close()

    def tearDown(self):
        for f in os.listdir(os.path.dirname(self.path)):
            if f.startswith(os.path.basename(self.path)):
                os.remove(os.path.join(os.path.dirname(self.path), f))

    def test_report_apply_undo(self):
        out = run("--db", self.path)
        self.assertIn("report only", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM weatherprofiles").fetchone()[0], 1)
        names = {n for (n,) in db.execute("SELECT name FROM sqlite_master")}
        self.assertNotIn("weatherclimates", names)
        db.close()

        out = run("--db", self.path, "--apply")
        self.assertIn("wrote 37 rows", out)
        r = rows(self.path)
        self.assertEqual(len(r["weatherprofiles"]), 22)
        self.assertEqual(len(r["weatherzones"]), 13)
        self.assertEqual(len(r["weatherclimates"]), 3)
        prof = {p: (c, s) for p, c, _, s in r["weatherprofiles"]}
        self.assertEqual(prof[2], ("arctic", "seed"))
        self.assertEqual(prof[11], ("desert", "seed"))
        self.assertEqual(prof[19], ("none", "seed"))
        self.assertEqual(prof[7], ("desert", "gm"))  # a GM's row kept
        self.assertIn((5, 2, 3, "none", "Zaoran Plateau, islands in lava", "seed"), r["weatherzones"])
        cl = {x[0]: x for x in r["weatherclimates"]}
        self.assertEqual(cl["arctic"][1:6], ("snow", 40, 1.0, 6, 12))
        self.assertEqual(cl["arctic"][9], 0.5)
        self.assertEqual(cl["desert"][4:6], (3, 6))
        self.assertTrue(any(f.startswith(os.path.basename(self.path) + ".bak-") for f in os.listdir(os.path.dirname(self.path))))

        self.assertIn("wrote 0 rows", run("--db", self.path, "--apply", "--no-backup"))

        db = sqlite3.connect(self.path)
        db.execute("UPDATE weatherprofiles SET climate = 'temperate', source = 'gm' WHERE profile = 10")
        db.commit()
        db.close()
        out = run("--db", self.path, "--undo", "--no-backup")
        self.assertIn("deleted 36 seeded rows; kept 1", out)
        r = rows(self.path)
        self.assertEqual(sorted(p for p, *_ in r["weatherprofiles"]), [7, 10])
        self.assertEqual(r["weatherzones"], [])
        self.assertEqual(r["weatherclimates"], [])


if __name__ == "__main__":
    unittest.main()
