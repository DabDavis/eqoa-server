#!/usr/bin/env python3
"""deathfist_mercs_20261123.py on a small database: a report writes nothing; apply dresses every kind by its body,
renames the plain mercenaries by body, adds one copied shaman a healerless camp, her template and the renamed kinds'
trash; a second apply writes nothing; undo is exact. A row dressed since, other counts, a taken spot or another
template refuse the whole run."""
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import deathfist_mercs_20261123 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, size REAL, primary_weapon INTEGER,
    secondary_weapon INTEGER, shield INTEGER, torso INTEGER, forearm INTEGER, gloves INTEGER, legs INTEGER, feet INTEGER,
    head INTEGER, hair_color INTEGER, robe_style TEXT, race TEXT, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER,
    unk_o INTEGER);
CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, abilities TEXT NOT NULL DEFAULT '[]');
CREATE TABLE lootpool (id INTEGER PRIMARY KEY, category TEXT, region TEXT, npc_name TEXT, patternid INTEGER,
    weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY, name TEXT, world INTEGER, x REAL, z REAL, radius REAL, faction TEXT,
    note TEXT, source TEXT);
CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT, profile TEXT, source TEXT);
CREATE TABLE factionhits (id INTEGER PRIMARY KEY, npc_name TEXT, faction TEXT, delta INTEGER, source TEXT);
"""

M, K, S = tool.MERC, tool.KNIGHT, tool.SHAMAN
OM, OF, TF = 0x84807490, 0xACC00B4F, 0x4DE32701


def npc(i, name, zone, x, y, z, body, level=39, primary=0):
    return (i, name, zone, x, y, z, 10, 0, None, body, 1.0, primary, 0, 0, 0, 0, 0, 0, 0, 0, 2, "255",
            "9" if body != TF else "5", level, 0, i, 0)


ROWS = [
    npc(111139, S, 809, 18231.2, 58.0859, 20871.6, OF, 40),       # the keep's shaman
    npc(111140, K, 809, 18226.0, 58.0, 20878.0, TF, 38),          # a keep knight
    npc(111131, M, 809, 18304.0, 54.0, 20887.0, OM),              # a keep mercenary, bare-handed
    npc(111258, M, 801, 19302.0, 79.0, 20813.0, OM, primary=tool.SCIMITAR),  # one holding the scimitar
    npc(111036, M, 801, 19313.0, 52.0, 20706.0, OF),              # a plain mercenary, a female ogre
    npc(111038, M, 801, 19330.0, 52.0, 20692.0, TF),              # a plain mercenary, a female troll
    npc(111077, M, 805, 19211.0, 54.0, 21496.0, TF, 38),          # the 805 camp
    npc(111078, M, 805, 19216.0, 54.0, 21486.0, OM),
    npc(111040, M, 803, 19588.0, 54.0, 20546.0, TF),              # the 802/803 camp
    npc(121605, M, 948, 20435.0, 54.0, 22229.0, TF),              # the 948 pair
    npc(500, "a desert beetle", 801, 19000.0, 54.0, 21000.0, 0x12345678),
]
EXPECT = {(M, OM): 3, (M, OF): 1, (M, TF): 4, (K, TF): 1, (S, OF): 1}


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany(f"INSERT INTO npcs VALUES ({', '.join('?' * 27)})", ROWS)
    db.executemany("INSERT INTO npcrespawn VALUES (?, 90.0, 'named', 'petfix259')", [(r[0],) for r in ROWS[:10]])
    db.execute("INSERT INTO classtemplates (name, class) VALUES ('kin healer', -1)")
    db.executemany("INSERT INTO lootpool VALUES (?, 'trash', '', 'deathfist mercenary', ?, 1, 1, 100, 'trash-seed')",
                   [(4920, 707965), (4921, 985104)])
    for n, c in (("deathfist mercenary", 0), ("deathfist mercenary knight", 3), ("deathfist mercenary shaman", 8)):
        db.execute("INSERT INTO npcclass VALUES (?, ?, 'x')", (n, c))
        db.execute("INSERT INTO npcfaction (name, world, faction) VALUES (?, 0, 'Deathfist Mercenaries')", (n,))
        db.execute("INSERT INTO lootcreature VALUES (?, 'desert', '', 'x')", (n,))
        db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES (?, 'Deathfist Mercenaries', -10, 'x')",
                   (n,))
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


def snapshot(path, skip=("gm_audit",)):
    db = sqlite3.connect(path)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY 1"):
        if t not in skip:
            out[t] = db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall()
    db.close()
    return out


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        make(self.db)
        self.saved = tool.EXPECT
        tool.EXPECT = {"heirloom": EXPECT, "live": {**EXPECT, (M, OF): 2}}

    def tearDown(self):
        tool.EXPECT = self.saved
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def exec(self, sql, args=()):
        db = sqlite3.connect(self.db)
        db.execute(sql, args)
        db.commit()
        db.close()

    def test_report_writes_nothing(self):
        before = snapshot(self.db, skip=())
        rc, out = run("--db", self.db, "--shard", "heirloom")
        self.assertEqual(rc, 0, out)
        self.assertIn("4 'Deathfist mercenary' -> 'Deathfist mercenary knight'", out)
        self.assertIn("1 'Deathfist mercenary' -> 'Deathfist mercenary shaman'", out)
        self.assertEqual(out.count("new 'Deathfist mercenary shaman'"), 3)
        self.assertEqual(snapshot(self.db, skip=()), before)

    def test_apply_again_undo(self):
        before = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 0, out)
        looks = {r[0]: r[1:] for r in self.q(
            "SELECT id, npc_name, torso, forearm, gloves, legs, feet, head, unk_o, robe_style, primary_weapon, shield "
            "FROM npcs WHERE world = 0 AND npc_name LIKE 'Deathfist%'")}
        # the melee by body: banded on the ogres, their scimitar kept or a greataxe; plate, sword and shield on trolls
        self.assertEqual(looks[111131], (M, 5, 5, 5, 5, 5, 5, 3, "255", tool.GREATAXE, 0))
        self.assertEqual(looks[111258], (M, 5, 5, 5, 5, 5, 5, 3, "255", tool.SCIMITAR, 0))
        for i in (111140, 111038, 111077, 111040, 121605):
            self.assertEqual(looks[i], (K, 4, 4, 4, 4, 4, 4, 13, "255", tool.LONGSWORD, tool.TOWER_SHIELD), i)
        # the healers robed, untinted, a staff
        for i in (111139, 111036):
            self.assertEqual(looks[i], (S, 0, 0, 0, 0, 0, 0, 0, "3", tool.STAFF, 0), i)
        # one new shaman a healerless camp: a copy of the keep's, the camp's zone, its respawn
        new = self.q("SELECT id, npc_name, zone, x_coord, z_coord, modelid, npc_level, race, npc_id, robe_style FROM npcs "
                     "WHERE id > 121605 ORDER BY id")
        self.assertEqual([(r[2], r[3], r[4]) for r in new], [(805, 19223.7, 21507.6), (803, 19592.6, 20539.6),
                                                             (948, 20437.5, 22234.8)])
        for r in new:
            self.assertEqual((r[1], r[5], r[6], r[7], r[9]), (S, OF, 40, "9", "3"))
            self.assertEqual(r[0], r[8])
            self.assertEqual(self.q("SELECT seconds FROM npcrespawn WHERE npc_row = ?", (r[0],)), [(90.0,)])
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = 'deathfist mercenary shaman'"),
                         [("kin healer",)])
        self.assertEqual(sorted(self.q("SELECT npc_name, patternid FROM lootpool WHERE id > 4921")),
                         [("deathfist mercenary knight", 707965), ("deathfist mercenary knight", 985104),
                          ("deathfist mercenary shaman", 707965), ("deathfist mercenary shaman", 985104)])
        n = self.q("SELECT count(*) FROM gm_audit")[0][0]
        self.assertEqual(n, 10 + 3 * 2 + 1 + 4)
        self.assertEqual(self.q("SELECT npc_name FROM npcs WHERE id = 500"), [("a desert beetle",)])
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("applied already", out)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], n)
        rc, out = run("--db", self.db, "--shard", "heirloom", "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(snapshot(self.db), before)

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.db, "--shard", "heirloom", "--apply")
        self.exec("UPDATE npcs SET unk_o = 8 WHERE id = 111131")
        rc, out = run("--db", self.db, "--shard", "heirloom", "--undo")
        self.assertEqual(rc, 1)
        self.assertIn("left: npcs {'id': 111131}", out)
        self.assertEqual(self.q("SELECT npc_name FROM npcs WHERE id = 111036"), [("Deathfist mercenary",)])

    def refused(self, *needles, shard="heirloom"):
        before = snapshot(self.db, skip=())
        rc, out = run("--db", self.db, "--shard", shard, "--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("refused, nothing done", out)
        for s in needles:
            self.assertIn(s, out)
        self.assertEqual(snapshot(self.db, skip=()), before)

    def test_a_row_dressed_since_refuses(self):
        self.exec("UPDATE npcs SET torso = 2 WHERE id = 111078")
        self.refused("no longer bare", "111078")

    def test_other_counts_refuse(self):
        self.refused("not as read on 2026-11-23", shard="live")

    def test_a_gms_clone_since_is_dressed_and_said(self):
        self.exec("INSERT INTO npcs (id, npc_name, world, zone, x_coord, y_coord, z_coord, modelid, primary_weapon, "
                  "secondary_weapon, shield, torso, forearm, gloves, legs, feet, head, robe_style, unk_o) "
                  "VALUES (700, 'Deathfist mercenary', 0, 802, 18309.3, 54.1, 20883.9, ?, 0, 0, 0, 0, 0, 0, 0, 0, 0, "
                  "'255', 0)", (OM,))
        rc, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("1 more 'Deathfist mercenary' (ogre male) than read", out)
        self.assertEqual(self.q("SELECT torso, head, unk_o, primary_weapon FROM npcs WHERE id = 700"),
                         [(5, 5, 3, tool.GREATAXE)])

    def test_another_body_refuses(self):
        self.exec("UPDATE npcs SET modelid = ? WHERE id = 111078", (0x70D898C6,))
        self.refused("not as read on 2026-11-23")

    def test_a_taken_spot_refuses(self):
        self.exec("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, modelid) "
                  "VALUES (600, 'a GM''s sign', 0, 19593.0, 54.0, 20540.0, 1)")
        self.refused("is taken: [600]")

    def test_a_camp_moved_refuses(self):
        self.exec("UPDATE npcs SET x_coord = 21000 WHERE id = 121605")
        self.refused("the 948 pair (20432, 22232): no mercenary")

    def test_another_template_refuses(self):
        self.exec("INSERT INTO npctemplate VALUES ('deathfist mercenary shaman', 'shaman', '', 'gm')")
        self.refused("'shaman' already")

    def test_the_keep_shaman_moved_refuses(self):
        self.exec("UPDATE npcs SET x_coord = 18100 WHERE id = 111139")
        self.refused("the keep's shaman")

    def test_shard_is_required(self):
        with self.assertRaises(SystemExit):
            with redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
                tool.main(["--db", self.db])


if __name__ == "__main__":
    unittest.main()
