#!/usr/bin/env python3
"""petfix_dungeon_casters_20261113.py on a small database holding the dungeons tool's three tiers and world 14's rows
of Guk (a wizard, a shaman, two knights, a misspelt necromancer, a warrior) bound to them by "#n": the report writes
nothing; --apply gives each tier the three casts, adds the shadow knight's and the necromancer's copies of the trash
tier (their class's casts alone) and points the knights' and the necromancer's rows at them, a gm_audit line each; a
second apply refuses; --undo puts every table back exactly. The controls: a tier's abilities not the dungeons tool's,
or a knight on another template, are refused, writing nothing; a shard with no tiers is left alone."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "petfix_dungeon_casters_20261113.py")

SCHEMA = """
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    melee_pct INTEGER NOT NULL DEFAULT 100, abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm', resist_pct INTEGER NOT NULL DEFAULT 0);
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""
BOSS = '[{"kind": "aoe", "spell_id": 905023, "every": 30, "first": 15, "damage_pct": 25}]'
ROWS = [(1, "froglok tal wizard"), (2, "froglok tal shaman"), (3, "froglok vis knight"), (4, "ghoul tal knight"),
        (5, "ghoul tal necromaner"), (6, "froglok dar warrior"), (7, "froglok king")]


def make(path, tiers=True, boss=BOSS):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    if tiers:
        db.execute("INSERT INTO classtemplates (name, melee_pct, abilities, note, resist_pct) VALUES ('dungeon trash', 130, '[]', 'trash', 10)")
        db.execute("INSERT INTO classtemplates (name, melee_pct, abilities, note, resist_pct) VALUES ('dungeon named', 200, '[]', 'named', 20)")
        db.execute("INSERT INTO classtemplates (name, melee_pct, abilities, note, resist_pct) VALUES ('dungeon boss', 150, ?, 'boss', 30)", (boss,))
    for i, n in ROWS:
        db.execute("INSERT INTO npcs VALUES (?, ?, 14)", (i, n))
        db.execute("INSERT INTO npctemplate (name, template) VALUES (?, ?)", (f"#{i}", "dungeon boss" if i == 7 else "dungeon trash"))
    db.execute("INSERT INTO npcs VALUES (100, 'froglok vis knight', 0)")  # its open world self, no "#n"
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {t: sorted(db.execute(f"SELECT * FROM {t}").fetchall()) for t in ("classtemplates", "npctemplate", "npcs")}
    out["tables"] = sorted(r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'"))
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


class DungeonCasters(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = dump(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only", r.stdout)
        self.assertEqual(dump(self.path), before)

        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.path)
        tmpl = {n: (c, json.loads(a), mp) for n, c, a, mp in db.execute("SELECT name, class, abilities, melee_pct FROM classtemplates")}
        kinds = lambda n: [a["kind"] for a in tmpl[n][1]]  # noqa: E731
        self.assertEqual(kinds("dungeon trash"), ["nuke", "lifetap", "heal"])
        self.assertEqual(kinds("dungeon named"), ["nuke", "lifetap", "heal"])
        self.assertEqual(kinds("dungeon boss"), ["aoe", "nuke", "lifetap", "heal"])
        self.assertNotIn(0, tmpl["dungeon trash"][1][0]["classes"])  # a warrior casts no nuke
        self.assertEqual((tmpl["dungeon trash shadowknight"][0], kinds("dungeon trash shadowknight")), (3, ["lifetap"]))
        self.assertEqual((tmpl["dungeon trash necromancer"][0], kinds("dungeon trash necromancer")), (11, ["nuke", "lifetap"]))
        self.assertEqual(tmpl["dungeon trash shadowknight"][2], 130)  # the trash tier's figures
        bind = dict(db.execute("SELECT name, template FROM npctemplate"))
        self.assertEqual(bind, {"#1": "dungeon trash", "#2": "dungeon trash", "#3": "dungeon trash shadowknight",
                                "#4": "dungeon trash shadowknight", "#5": "dungeon trash necromancer", "#6": "dungeon trash",
                                "#7": "dungeon boss"})
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 8)
        db.close()

        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("applied already", r.stdout)

        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before)

    def test_refusals(self):
        make(self.path, boss='[{"kind": "aoe", "spell_id": 1, "every": 30}]')
        before = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.path), before)

        os.remove(self.path)
        make(self.path)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npctemplate SET template = 'roamer' WHERE name = '#4'")
        db.commit()
        db.close()
        before = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("ghoul tal knight", r.stdout)
        self.assertEqual(dump(self.path), before)

    def test_no_dungeons(self):
        make(self.path, tiers=False)
        before = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("no dungeon tiers", r.stdout)
        self.assertEqual(dump(self.path), before)


if __name__ == "__main__":
    unittest.main()
