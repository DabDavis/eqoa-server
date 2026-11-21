#!/usr/bin/env python3
"""jib_batch1_20261121.py on a small database: apply, a second apply, undo; a row not as read refuses its part;
--only / --skip; an epic table mirrored from a source with an Ascended record made by the server's rule."""
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jib_batch1_20261121 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, npc_level INTEGER, npc_type INTEGER,
    modelid INTEGER, x_coord REAL, y_coord REAL, z_coord REAL);
CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE factions (name TEXT PRIMARY KEY);
CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL, faction TEXT NOT NULL,
    delta INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm', UNIQUE (npc_name, faction));
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, hp_pct INTEGER, special TEXT, special_damage REAL,
    resist_kind TEXT, resist_pct INTEGER);
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE lootcreature (npc_name TEXT PRIMARY KEY, region TEXT NOT NULL DEFAULT '', profile TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE quests (questid INTEGER PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 0);
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', items TEXT NOT NULL DEFAULT '', journal TEXT NOT NULL DEFAULT '',
    rewarditems TEXT NOT NULL DEFAULT '', xp INTEGER NOT NULL DEFAULT 0, events TEXT NOT NULL DEFAULT '',
    chance REAL NOT NULL DEFAULT 0, byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (questid, step));
CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT, equipslot INTEGER,
    maxstack INTEGER, attacktype INTEGER, itemicon INTEGER, str INTEGER, hpmax INTEGER, itemcost INTEGER, rent INTEGER);
CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '');
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE lootrate (preset TEXT NOT NULL, profile TEXT NOT NULL, category TEXT NOT NULL, rate REAL NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (preset, profile, category));
CREATE TABLE lootpool (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT NOT NULL, npc_name TEXT NOT NULL DEFAULT '',
    patternid INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER);
"""


def make(path, live=True):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO npcs (id, npc_name, world, npc_level, npc_type, modelid, x_coord, y_coord, z_coord) "
                   "VALUES (?, ?, ?, ?, ?, 0, 1, 2, 3)",
                   [(2392, "Slagbreath", 0, 40, 128), (120970, "Sarek the Betrayer", 1, 67, 0),
                    (192338, "High Guard Ianant", 0, 37, 0), (2040, "Nasehir Sandcaster", 0, 15, 128),
                    (122699, "Teledic", 0, 62, 0)])
    db.execute("INSERT INTO npcrespawn VALUES (120970, 90, 'petfix', 'petfix259')")
    db.execute("INSERT INTO factions VALUES ('Sarek the Betrayer')")
    db.execute("INSERT INTO lootcreature VALUES ('Bordag', 'odus', '', 'census'), ('Dooga', '', '', 'census')")
    db.execute("INSERT INTO quests VALUES (101073, 1)")
    db.execute("INSERT INTO queststeps (questid, step, xp) VALUES (101073, 3, 73000000)")
    db.executemany("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack, attacktype, itemicon, str, hpmax, "
                   "itemcost, rent) VALUES (?, ?, 7, 1, 0, 5, ?, 100, 50, 0)",
                   [(7418, "ensorceling boots", 10), (719124, "Silent Chain Boots", 15)])
    db.execute("INSERT INTO itemeffect VALUES (7418, 'Increase STR by 10', 'gm')")
    if live:
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance) VALUES ('teledic', 'epic boss armour', 5)")
    db.commit()
    db.close()


def make_source(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO itempattern (patternid, itemname, equipslot, maxstack, attacktype, itemicon, str, hpmax, "
                   "itemcost, rent) VALUES (?, ?, 7, 1, 0, 5, 10, 100, 50, ?)",
                   [(7418, "ensorceling boots", 0), (719004, "Ascended ensorceling boots", 1),
                    (719200, "Silent Chain Boots", 0), (719152, "Ascended Silent Chain Boots", 1)])
    db.execute("INSERT INTO itemascended VALUES (719004, 7418, '', 'epic-audit'), (719152, 719200, '', 'epic-audit')")
    db.executemany("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, 1, 1, 100, 'epic-audit 1130')",
                   [("epic boss armour", 719004), ("epic boss armour", 719152)])
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "live.db")
        self.src = os.path.join(self.dir.name, "heirloom.db")
        make(self.path)
        make_source(self.src)
        self.only = "1764,709,1718,1785,1832,992"

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        before = self.q("SELECT * FROM npcs ORDER BY id"), self.q("SELECT * FROM npcrespawn"), self.q("SELECT * FROM lootcreature ORDER BY rowid")
        rc, out = run("--db", self.path, "--shard", "live", "--only", self.only, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT npc_type FROM npcs WHERE id = 2392"), [(0,)])
        self.assertEqual(self.q("SELECT seconds, source FROM npcrespawn WHERE npc_row = 120970"), [(259200, "board 1820")])
        self.assertEqual(self.q("SELECT delta FROM factionhits WHERE npc_name = 'Sarek the Betrayer'"), [(-505,)])
        self.assertEqual(self.q("SELECT npc_level FROM npcs WHERE id = 192338"), [(38,)])
        self.assertEqual(self.q("SELECT xp FROM queststeps WHERE questid = 101073"), [(100000000,)])
        self.assertEqual(self.q("SELECT npc_name FROM lootcreature ORDER BY rowid"), [("bordag",), ("dooga",)])
        self.assertEqual(self.q("SELECT npc_name, npc_level FROM npcs WHERE id = 2040"), [("Nasehir sandcaster", 14)])
        audits = self.q("SELECT count(*) FROM gm_audit")[0][0]
        rc, out = run("--db", self.path, "--shard", "live", "--only", self.only, "--apply")
        self.assertIn("nothing to write", out)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], audits)
        rc, out = run("--db", self.path, "--shard", "live", "--undo")
        self.assertEqual(rc, 0, out)
        after = self.q("SELECT * FROM npcs ORDER BY id"), self.q("SELECT * FROM npcrespawn"), self.q("SELECT * FROM lootcreature ORDER BY rowid")
        self.assertEqual(before, after)
        self.assertEqual(self.q("SELECT count(*) FROM factionhits"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", tool.LOG), [(0,)])

    def test_heirloom_keeps_its_respawn(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcrespawn SET seconds = 82800 WHERE npc_row = 120970")
        db.commit()
        db.close()
        rc, out = run("--db", self.path, "--shard", "heirloom", "--only", "709", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT seconds FROM npcrespawn"), [(82800,)])

    def test_a_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE queststeps SET xp = 5 WHERE questid = 101073")
        db.commit()
        db.close()
        rc, out = run("--db", self.path, "--shard", "live", "--only", self.only, "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("REFUSED", out)
        self.assertEqual(self.q("SELECT npc_type FROM npcs WHERE id = 2392"), [(128,)])  # nothing written
        rc, out = run("--db", self.path, "--shard", "live", "--only", self.only, "--skip", "1785", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT npc_type FROM npcs WHERE id = 2392"), [(0,)])

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.path, "--shard", "live", "--only", "1764,1785", "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET npc_type = 129 WHERE id = 2392")
        db.commit()
        db.close()
        rc, out = run("--db", self.path, "--shard", "live", "--undo")
        self.assertIn("changed since", out)
        self.assertEqual(self.q("SELECT npc_type FROM npcs WHERE id = 2392"), [(129,)])
        self.assertEqual(self.q("SELECT xp FROM queststeps"), [(73000000,)])

    def test_epic_mirror_makes_ascended(self):
        rc, out = run("--db", self.path, "--shard", "live", "--source", self.src, "--only", "677", "--apply")
        self.assertEqual(rc, 0, out)
        made = self.q("SELECT patternid, itemname, str, rent FROM itempattern WHERE patternid >= 980001 ORDER BY patternid")
        self.assertEqual(made, [(980001, "Ascended ensorceling boots", 11, 1), (980002, "Ascended Silent Chain Boots", 17, 1)])
        self.assertEqual(self.q("SELECT base FROM itemascended ORDER BY patternid"), [(7418,), (719124,)])
        self.assertEqual(self.q("SELECT line FROM itemeffect WHERE patternid = 980001"), [("Increase STR by 11",)])
        self.assertEqual(self.q("SELECT patternid FROM loottables WHERE tablename = 'epic boss armour' ORDER BY id"),
                         [(980001,), (980002,)])
        rc, out = run("--db", self.path, "--shard", "live", "--source", self.src, "--only", "677", "--apply")
        self.assertIn("nothing to write", out)
        # a looted Ascended record stays at an undo
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, 980002)")
        db.commit()
        db.close()
        rc, out = run("--db", self.path, "--shard", "live", "--undo")
        self.assertEqual(self.q("SELECT count(*) FROM loottables"), [(0,)])
        self.assertEqual(self.q("SELECT patternid FROM itempattern WHERE patternid >= 980001"), [(980002,)])
        self.assertEqual(self.q("SELECT patternid FROM itemascended"), [(980002,)])

    def test_epic_mirror_refuses_unbound(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM npcloot")
        db.commit()
        db.close()
        rc, out = run("--db", self.path, "--shard", "live", "--source", self.src, "--only", "677")
        self.assertEqual(rc, 1)
        self.assertIn("not on this shard", out)

    def test_lower_twin_refuses_1832(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO lootcreature VALUES ('bordag', '', '', 'x')")
        db.commit()
        db.close()
        rc, out = run("--db", self.path, "--shard", "live", "--only", "1832")
        self.assertEqual(rc, 1)
        self.assertIn("twins", out)

    def test_1599_heirloom_keeps_himlar(self):
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO npcs (id, npc_name, world, npc_level) VALUES (190044, 'elder spirit', 2, 50), "
                   "(190045, 'elder spirit', 2, 50)")
        db.execute("INSERT INTO loottables (tablename, patternid) VALUES ('rare spirit makers robe', 5209)")
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance) VALUES ('himlar', 'rare spirit makers robe', 1)")
        db.execute("INSERT INTO lootrate VALUES ('jib', 'xp', 'spirit makers robe', 0.001, 'workboard 1598')")
        db.execute("INSERT INTO lootpool VALUES (12158, 'spirit makers robe', 'elder spirit', 5209, 'workboard 1598')")
        db.commit()
        db.close()
        rc, out = run("--db", self.path, "--shard", "live", "--only", "1599", "--apply")
        self.assertIn("Heirloom only", out)
        rc, out = run("--db", self.path, "--shard", "heirloom", "--only", "1599", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT npc_name, chance FROM npcloot WHERE tablename = 'rare spirit makers robe' "
                                "ORDER BY npc_name"), [("#190044", 0.1), ("#190045", 0.1), ("himlar", 1)])
        self.assertEqual(self.q("SELECT count(*) FROM lootrate"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM lootpool"), [(0,)])
        run("--db", self.path, "--shard", "heirloom", "--undo", "--only", "1599")
        self.assertEqual(self.q("SELECT count(*) FROM lootrate"), [(1,)])
        self.assertEqual(self.q("SELECT id FROM lootpool"), [(12158,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcloot WHERE npc_name LIKE '#%'"), [(0,)])

    def test_source_needed(self):
        with self.assertRaises(SystemExit):
            with redirect_stdout(io.StringIO()), open(os.devnull, "w") as dn:
                old, sys.stderr = sys.stderr, dn
                try:
                    tool.main(["--db", self.path, "--shard", "live", "--only", "670"])
                finally:
                    sys.stderr = old


if __name__ == "__main__":
    unittest.main()
