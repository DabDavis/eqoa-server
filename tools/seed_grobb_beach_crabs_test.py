import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_grobb_beach_crabs as tool  # noqa: E402


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(db.iterdump())
    db.close()
    return out


class SeedGrobbBeachCrabs(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL, "
                   "z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, size REAL, npc_level INTEGER)")
        db.executemany("INSERT INTO npcs (id, npc_name, x_coord, y_coord, z_coord, facing, world, dt_kill, modelid, size, npc_level) "
                       "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", [
                           (3252, "Grobb Crab", 25686.4, 53.07, 30015.0, 0, 0, None, 152591256, 1.0, 7),
                           (3253, "star crab", 25681.9, 54.13, 30042.6, 0, 0, "2026-10-30 01:00:00", 152591256, 1.0, 7),
                           # namesakes the copies must not come from: another world, and far away
                           (3300, "star crab", 25681.9, 54.13, 30042.6, 0, 1, None, 1, 9.0, 40),
                           (3301, "Grobb Crab", 10000.0, 50.0, 10000.0, 0, 0, None, 2, 9.0, 40),
                           (3302, "orc pawn", 25000.0, 60.0, 30000.0, 5, 0, None, 3, 1.0, 5),
                       ])
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def run_tool(self, *args, check=True):
        return subprocess.run([sys.executable, os.path.join(HERE, "seed_grobb_beach_crabs.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=check)

    def q(self, sql):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql).fetchall()
        finally:
            db.close()

    def test_apply_copies_each_kind_and_undo_puts_back(self):
        n = len(tool.SPAWNS)
        self.assertIn(f"{n} spawns", self.run_tool().stdout)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn(f"wrote {n} spawns", self.run_tool("--apply").stdout)
        rows = self.q("SELECT npc_name, x_coord, y_coord, z_coord, world, dt_kill, modelid, size, npc_level, facing "
                      "FROM npcs WHERE id > 3302 ORDER BY id")
        self.assertEqual(len(rows), n)
        for i, (name, x, y, z, world, dk, model, size, lv, facing) in enumerate(rows):
            self.assertEqual(name, tool.KINDS[i % 2])
            self.assertEqual((x, y, z), tool.SPAWNS[i])
            self.assertEqual((world, dk, model, size, lv), (0, None, 152591256, 1.0, 7),
                             "the beach's own row is copied, never the far or other-world namesake, and dt_kill cleared")
            self.assertTrue(0 <= facing < 256)
        self.assertEqual(len({r[9] for r in rows}), n, "facings spread")
        r = self.run_tool("--apply", check=False)
        self.assertNotEqual(r.returncode, 0, "a second apply is refused")
        self.assertEqual(self.q("SELECT count(*) FROM npcs")[0][0], 5 + n)
        self.assertIn(f"took away {n}; left 0", self.run_tool("--undo").stdout)
        self.assertEqual(dump(self.path), self.before, "undo puts every row back, sqlite_sequence too")

    def test_undo_leaves_a_crab_a_gm_moved(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET x_coord = 1 WHERE id = (SELECT MAX(id) FROM npcs)")
        db.commit()
        db.close()
        self.assertIn(f"took away {len(tool.SPAWNS) - 1}; left 1", self.run_tool("--undo").stdout)

    def test_no_template_no_apply(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM npcs WHERE id = 3252")
        db.commit()
        db.close()
        before = dump(self.path)
        r = self.run_tool("--apply", check=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("no 'Grobb Crab'", r.stdout)
        self.assertEqual(dump(self.path), before, "nothing written")

    def test_places_are_spread_and_on_the_beach(self):
        for i, (x, y, z) in enumerate(tool.SPAWNS):
            self.assertTrue(51.4 <= y <= 56.0, (x, y, z))
            for x2, _, z2 in tool.SPAWNS[i + 1:]:
                self.assertGreaterEqual(((x - x2) ** 2 + (z - z2) ** 2) ** 0.5, 125)


if __name__ == "__main__":
    unittest.main()
