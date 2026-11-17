#!/usr/bin/env python3
"""christmas_sleigh_20261116.py on a small database (christmas_elf_santa's, after its apply, with christmas3's light and
decoration tables): the report writes nothing; --apply adds the sleigh (a chest's body, worn as the sled by vanilla), its
team of five winged horses led by Rudolph, all friendly and of the season, and its strings (presents, rim, nose,
harness); a second --apply refuses; --undo puts every table back."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import christmas_elf_santa_20261116_test as es  # noqa: E402

TOOL = os.path.join(HERE, "christmas_sleigh_20261116.py")
MORE = """
CREATE TABLE lighttemplates (name TEXT PRIMARY KEY COLLATE NOCASE, colors TEXT NOT NULL DEFAULT 'FFFFFF', pattern TEXT NOT NULL DEFAULT 'cycle',
    effect TEXT NOT NULL DEFAULT 'twinkle', period REAL NOT NULL DEFAULT 1.5, spacing REAL NOT NULL DEFAULT 1, size REAL NOT NULL DEFAULT 0.3,
    bright REAL NOT NULL DEFAULT 1, cord TEXT NOT NULL DEFAULT '1E3F1E', cord_width REAL NOT NULL DEFAULT 0.06, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE decorart (name TEXT PRIMARY KEY, model INTEGER NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT 0, zone INTEGER NOT NULL DEFAULT 0,
    sink REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE lightstrings (id INTEGER PRIMARY KEY, template TEXT NOT NULL DEFAULT '', npc TEXT NOT NULL DEFAULT '', world INTEGER NOT NULL DEFAULT 0,
    shape TEXT NOT NULL DEFAULT 'line', params TEXT NOT NULL DEFAULT '{}', season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
"""
TABLES = es.TABLES + ("lighttemplates", "decorart", "lightstrings")


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in TABLES}
    db.close()
    return out


class SleighTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "sl.json")
        db = sqlite3.connect(self.db)
        db.executescript(es.SCHEMA + MORE)
        cols = "id, npc_name, zone, unk_a, unk_b, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, modelid, size, race, robe_style, npc_level, npc_type, npc_id"
        db.execute(f"INSERT INTO npcs ({cols}) VALUES (190712, 'Christmas Tree', 0, 0, 0, 25270, 54.1, 15754.7, 0, 0, 100, 255, 1178150727, 1.0, '0', '255', 1, 128, 190712)")
        db.execute(f"INSERT INTO npcs ({cols}) VALUES (190715, 'Santa Claus', 0, 0, 0, 25279, 54.1, 15769, 32, 0, 100, 255, 2223010960, 1.3, '9', '0', 50, 128, 190715)")
        db.commit()
        db.close()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, TOOL, "--db", self.db, "--applied", self.applied, *args], capture_output=True, text=True)

    def test_apply_and_undo(self):
        before = dump(self.db)
        self.assertEqual(self.run_tool().returncode, 0)
        self.assertEqual(dump(self.db), before, "the report writes nothing")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT modelid & 4294967295, npc_type, facing FROM npcs WHERE npc_name = \"Santa's Sleigh\"").fetchone(), (0x3B09103E, 128, 128))
        self.assertEqual(db.execute("SELECT model, world, zone FROM decorart WHERE name = \"Santa's Sleigh\"").fetchone(), (0x0B3BA520, 0, 22))
        team = db.execute("SELECT npc_name, modelid & 4294967295, size, facing FROM npcs WHERE modelid = ? ORDER BY x_coord DESC", (0x2F8721C0,)).fetchall()
        self.assertEqual([t[0] for t in team][0], "Rudolph", "Rudolph leads, furthest out")
        self.assertEqual(len(team), 5)
        self.assertTrue(all(t[2] == 0.65 and t[3] == 64 for t in team))
        for n in ("Santa's Sleigh", "Rudolph", "Vixen"):
            self.assertEqual(db.execute("SELECT season FROM npcseason WHERE name = ?", (n,)).fetchone(), ("Christmas",))
            self.assertEqual(db.execute("SELECT aggressive FROM npcaggro WHERE npc_name = ?", (n,)).fetchone(), (0,))
        nose = db.execute("SELECT shape, params FROM lightstrings WHERE template = 'Red nose'").fetchone()
        self.assertEqual(nose[0], "points")
        self.assertEqual(json.loads(nose[1])["points"], [[25274.0, 56.2, 15788.0]], "ahead of Rudolph (25272.6), at his head")
        gifts = json.loads(db.execute("SELECT params FROM lightstrings WHERE npc = \"Santa's Sleigh\" AND shape = 'presents'").fetchone()[0])
        self.assertEqual(len(gifts["boxes"]), 8)
        # clear of the tree's presents (a radius of 12) and of Santa
        for x, z in db.execute("SELECT x_coord, z_coord FROM npcs WHERE npc_name IN (\"Santa's Sleigh\", 'Rudolph', 'Dasher', 'Dancer', 'Prancer', 'Vixen')"):
            self.assertGreater(((x - 25270) ** 2 + (z - 15754.7) ** 2) ** 0.5, 14, (x, z))
            self.assertGreater(((x - 25279) ** 2 + (z - 15769) ** 2) ** 0.5, 6, (x, z))
        db.close()
        self.assertNotEqual(self.run_tool("--apply").returncode, 0)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(dump(self.db), before, "undone")


if __name__ == "__main__":
    unittest.main()
