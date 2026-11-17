#!/usr/bin/env python3
"""christmas_decor_looks_20261116.py on a small database (christmas3's tree: its rings of lights and its presents, and a
Freeport wall string): the report writes nothing; --apply adds the assets, the four templates, the garlands between the
rings and the wall's globes, and wraps every other present; --globes turns the tree's lights to globes; a second
--apply refuses; --undo puts every table back."""
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
import christmas_sleigh_20261116_test as sl  # noqa: E402

TOOL = os.path.join(HERE, "christmas_decor_looks_20261116.py")
TABLES = ("lighttemplates", "lightstrings")
WALL = "the low wall by Freeport's coach, its segment at (25308.5, 15727.4)"


def ring(y, r):
    import math
    return [[round(r * math.cos(t / 12 * 2 * math.pi), 3), y, round(r * math.sin(t / 12 * 2 * math.pi), 3)] for t in range(12)]


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in TABLES}
    names = {r[1] for r in db.execute("PRAGMA table_info(lighttemplates)")}
    db.close()
    return out, names


class LooksTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "lk.json")
        db = sqlite3.connect(self.db)
        db.executescript(es.SCHEMA + sl.MORE)
        db.execute("INSERT INTO lighttemplates (name, colors, source) VALUES ('Christmas lights', 'FF2A2A, 2AE04A', 'christmas-2026')")
        for y, r in ((1.8, 10), (9.7, 8), (17.2, 6)):
            db.execute("INSERT INTO lightstrings (template, npc, shape, params, season, source) VALUES ('Christmas lights', 'Christmas Tree', 'path', ?, 'Christmas', 'christmas-2026')",
                       (json.dumps({"points": ring(y, r)}),))
        boxes = [{"at": [i, 0, 3], "size": [0.6, 0.5, 0.6], "color": "C02020", "ribbon": "FFD040"} for i in range(6)]
        db.execute("INSERT INTO lightstrings (template, npc, shape, params, season, note, source) VALUES ('', 'Christmas Tree', 'presents', ?, 'Christmas', 'presents', 'christmas-2026')",
                   (json.dumps({"boxes": boxes}),))
        db.execute("INSERT INTO lightstrings (template, npc, shape, params, season, note, source) VALUES ('Christmas lights', '', 'path', ?, 'Christmas', ?, 'christmas-2026')",
                   (json.dumps({"points": [[25308.5, 55, 15722], [25308.5, 55, 15732]]}), WALL))
        db.commit()
        db.close()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, TOOL, "--db", self.db, "--applied", self.applied, *args], capture_output=True, text=True)

    def test_apply_and_undo(self):
        before, _ = dump(self.db)
        self.assertEqual(self.run_tool().returncode, 0)
        r = self.run_tool("--apply", "--garland", "both", "--globes")
        self.assertEqual(r.returncode, 0, r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT world, zone, ids FROM decorassets WHERE name = 'Magic globe'").fetchone(), (0, 108, "0xF8E6C045"))
        self.assertEqual(db.execute("SELECT bulb, size FROM lighttemplates WHERE name = 'Christmas globes'").fetchone()[0], "0xF8E6C045")
        self.assertEqual(db.execute("SELECT cord_texture, size FROM lighttemplates WHERE name = 'Gold tinsel'").fetchone(), ("0x9196DAC4", 0))
        garlands = db.execute("SELECT template, params FROM lightstrings WHERE source = 'christmas-2026-looks' AND npc = 'Christmas Tree'").fetchall()
        self.assertEqual([g[0] for g in garlands], ["Gold tinsel", "Red rope garland"], "one between each two rings")
        ys = [p[1] for p in json.loads(garlands[0][1])["points"]]
        self.assertTrue(all(5 < y < 6.6 for y in ys), "halfway between rings 1.8 and 9.7")
        self.assertEqual(db.execute("SELECT count(*) FROM lightstrings WHERE template = 'Christmas globes'").fetchone()[0], 4, "the wall and the tree's three")
        boxes = json.loads(db.execute("SELECT params FROM lightstrings WHERE shape = 'presents'").fetchone()[0])["boxes"]
        self.assertEqual([("paper" in b) for b in boxes], [False, True] * 3)
        self.assertNotEqual(boxes[1]["color"], "C02020", "a paper's tint made lighter")
        db.close()
        self.assertNotEqual(self.run_tool("--apply").returncode, 0)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        after, cols = dump(self.db)
        # the template table keeps the looks' columns (the server makes them too); every row is as it was
        self.assertEqual({t: [row[:len(before[t][0])] if t == "lighttemplates" else row for row in after[t]] for t in TABLES}, before)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM decorassets").fetchone()[0], 0)


if __name__ == "__main__":
    unittest.main()
