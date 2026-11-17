#!/usr/bin/env python3
"""santa_outfits_20261117.py on a small database: the report writes nothing; --apply gives the three names Santa's robe
(his robe style as the robe row's robe), helm dye and trims in the Christmas season; the panel's hat rows are kept, the
Weapon Enchanter's put in the season; a name with no hat gets one in the season; a row it replaces is logged; a second --apply finds nothing to do; --undo puts back exactly what was there. The controls: a second
Weapon Enchanter in the world (the Live shard), the one left away from the tree, and a database without the season
columns: refused, nothing written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "santa_outfits_20261117.py")
SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, robe_style TEXT, x_coord REAL, z_coord REAL);
CREATE TABLE npcdyes (name TEXT NOT NULL, slot INTEGER NOT NULL, color INTEGER NOT NULL, season TEXT NOT NULL DEFAULT '',
    robe INTEGER NOT NULL DEFAULT -1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (name, slot));
CREATE TABLE npctint (name TEXT PRIMARY KEY, color INTEGER NOT NULL DEFAULT 4294967295, glow INTEGER NOT NULL DEFAULT 0,
    aura_kind INTEGER NOT NULL DEFAULT 0, aura_source INTEGER NOT NULL DEFAULT 0, aura_nodes INTEGER NOT NULL DEFAULT 0,
    season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcseason (name TEXT PRIMARY KEY, season TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gmseasons (name TEXT PRIMARY KEY, active INTEGER NOT NULL DEFAULT 0, changed TEXT NOT NULL DEFAULT '',
    by TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO npcs VALUES (190715, 'Santa Claus', '0', 25287.3, 15768.9), (190714, 'Toymaker Tinsel', '255', 25282.2, 15744.8),
    (190713, 'Holiday Elf', '0', 25262.2, 15744.1), (190637, 'Weapon Enchanter', '0', 25284.8, 15748.0),
    (190564, 'Weapon Enchanter', '0', 1000, 1000);
INSERT INTO npcspawnonly VALUES ('#190564', 'held', 'enchanter-tree-only-1117');
INSERT INTO npcseason VALUES ('Toymaker Tinsel', 'Christmas', '', 'toy-shoppe'), ('Holiday Elf', 'Christmas', '', 'christmas-2026'),
    ('Santa Claus', 'Christmas', '', 'christmas-2026');
INSERT INTO npcdyes (name, slot, color, season, source) VALUES ('Santa Claus', 9, 4280295423, 'Christmas', 'christmas-2026'),
    ('Santa Claus', 5, 4280295423, 'Christmas', 'christmas-2026'), ('Santa Claus', 11, 4294967295, 'Christmas', 'christmas-2026'),
    ('Santa Claus', 14, 4294967295, 'Christmas', 'christmas-2026'), ('Santa Claus', 16, 4294967295, 'Christmas', 'christmas-2026'),
    ('Santa Claus', 2, 471604479, 'Christmas', 'christmas-2026'),
    ('Holiday Elf', 5, 512507135, '', 'christmas-2026'), ('Holiday Elf', 9, 4280295423, '', 'gm'), ('Holiday Elf', 4, 3357417215, '', 'christmas-2026'),
    ('Toymaker Tinsel', 11, 3769643263, '', 'toy-shoppe');
INSERT INTO npctint (name, color, aura_kind, aura_source, aura_nodes, source) VALUES ('Santa Claus', 0, 10, 2718273763, 1, 'santa-hat'),
    ('Holiday Elf', 0, 10, 2718273763, 1, 'santa-hat'), ('Toymaker Tinsel', 0, 10, 2718273763, 1, 'santa-hat'),
    ('Weapon Enchanter', 0, 10, 2718273763, 1, 'santa-hat');
"""


def state(path):
    db = sqlite3.connect(path)
    try:
        return tuple(db.execute(q).fetchall() for q in ("SELECT * FROM npcdyes ORDER BY name, slot", "SELECT * FROM npctint ORDER BY name"))
    finally:
        db.close()


class SantaOutfits(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, *args], capture_output=True, text=True)

    def sql(self, q, *args):
        db = sqlite3.connect(self.path)
        try:
            r = db.execute(q, args).fetchall()
            db.commit()
            return r
        finally:
            db.close()

    def test_apply_then_undo(self):
        before = state(self.path)
        r = self.run_tool()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 11 rows", r.stdout)  # 2 names x 5 slots, the Enchanter's hat's season
        self.assertEqual(state(self.path), before)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        for n in ("Toymaker Tinsel", "Weapon Enchanter"):
            got = dict((s, (c, se, rb, src)) for s, c, se, rb, src in self.sql(
                "SELECT slot, color, season, robe, source FROM npcdyes WHERE name = ? AND slot IN (5, 9, 11, 14, 16)", n))
            self.assertEqual(got[9], (4280295423, "Christmas", 0, "santa-outfits"), n)
            self.assertEqual(got[5][0], 4280295423, n)
            for s in (11, 14, 16):
                self.assertEqual(got[s][:3], (4294967295, "Christmas", -1), (n, s))
        # their own other dyes kept; the Elf, in Santa's robe already, untouched
        self.assertEqual(self.sql("SELECT color FROM npcdyes WHERE name = 'Toymaker Tinsel' AND slot = 11"), [(4294967295,)])
        self.assertEqual(self.sql("SELECT slot, color, season, robe FROM npcdyes WHERE name = 'Holiday Elf' ORDER BY slot"),
                         [(4, 3357417215, "", -1), (5, 512507135, "", -1), (9, 4280295423, "", -1)])
        # the hats: the panel's Enchanter's put in the season, its source kept; the Elf's and Tinsel's left alone
        self.assertEqual(self.sql("SELECT color, aura_kind, aura_source, aura_nodes, season, source FROM npctint WHERE name = 'Weapon Enchanter'"),
                         [(0, 10, 2718273763, 1, "Christmas", "santa-hat")])
        self.assertEqual(self.sql("SELECT season, source FROM npctint WHERE name IN ('Holiday Elf', 'Toymaker Tinsel')"),
                         [("", "santa-hat"), ("", "santa-hat")])
        self.assertEqual(self.sql("SELECT count(*) FROM gm_audit"), [(11,)])
        r = self.run_tool("--apply")
        self.assertIn("nothing to do", r.stdout)
        # undo refused once a written row changed, then all back
        self.sql("UPDATE npcdyes SET color = 1 WHERE name = 'Weapon Enchanter' AND slot = 9")
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.sql("UPDATE npcdyes SET color = 4280295423 WHERE name = 'Weapon Enchanter' AND slot = 9")
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path), before)

    def test_an_elf_not_yet_robed_is_dressed(self):
        self.sql("UPDATE npcs SET robe_style = '255' WHERE npc_name = 'Holiday Elf'")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.sql("SELECT color, season, robe FROM npcdyes WHERE name = 'Holiday Elf' AND slot = 9"), [(4280295423, "Christmas", 0)])

    def test_no_hat_row_gets_one_in_the_season(self):
        self.sql("DELETE FROM npctint WHERE name = 'Weapon Enchanter'")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.sql("SELECT color, aura_kind, aura_source, aura_nodes, season, source FROM npctint WHERE name = 'Weapon Enchanter'"),
                         [(0, 10, 2718273763, 1, "Christmas", "santa-outfits")])

    def test_a_second_enchanter_in_the_world_is_refused(self):
        self.sql("DELETE FROM npcspawnonly")
        before = state(self.path)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("2 Weapon Enchanters in the world", r.stdout)
        self.assertEqual(state(self.path), before)

    def test_the_one_left_away_from_the_tree_is_refused(self):
        self.sql("UPDATE npcspawnonly SET name = '#190637'")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("not by the tree", r.stdout)

    def test_an_older_schema_is_refused(self):
        self.sql("ALTER TABLE npcdyes DROP COLUMN robe")
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("santa-outfits", r.stdout)


if __name__ == "__main__":
    unittest.main()
