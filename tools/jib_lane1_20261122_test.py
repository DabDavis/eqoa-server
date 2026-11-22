#!/usr/bin/env python3
"""jib_lane1_20261122.py on a small database: the report writes nothing; --apply makes every change (the two
renames in npcs, npcclass and npcfaction, the porters' npc_type, Take a Shield Break's line, Crusader's Aura's rows,
and on Heirloom the rune flag and the Grinch's life) with a gm_audit line each; a second apply does nothing; --undo
puts every value back exactly. The controls: on Live the rune and the Grinch are not touched; Jasper Darvar's rows
are never touched; a porter of another npc_type, or both a rename's names in npcs, refuse the whole run; a value
changed after the apply is left by the undo."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "jib_lane1_20261122.py")
sys.path.insert(0, HERE)
import jib_lane1_20261122 as tool  # noqa: E402

GRINCH_AB = json.dumps([{"kind": "summon", "below": b, "count": 1, "npc": "Mini Grinch", "share": 50} for b in (75, 50, 25)])
RUNE_EFF = ('{"v":1,"effects":[{"id":"02ED","type":"Invulnerable","kind":"invulnerable","mitigation":{"pct":100,'
            '"max":-3500}}]}')


def make(path, porter_type=0, both=False):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, npc_type INTEGER);
        CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
        CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
            faction TEXT NOT NULL);
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, recast INTEGER, spelldesc TEXT);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL DEFAULT 0,
            source TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '');
        CREATE TABLE classtemplates (name TEXT PRIMARY KEY, abilities TEXT, note TEXT);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    db.executemany("INSERT INTO npcs VALUES (?, ?, 0, ?)", [
        (974, "Coachman D'Larik", 128), (979, "Coachwoman Darva", 384), (2862, "Jasper Darvar", 128),
        (100782, "Rigalig Porter", porter_type), (103274, "Riddletic Porter", 0)])
    if both:
        db.execute("INSERT INTO npcs VALUES (5, 'Coachman Darva', 0, 384)")
    db.executemany("INSERT INTO npcclass VALUES (?, 0, 'x')", [("coachman d'larik",), ("coachwoman darva",), ("jasper darvar",)])
    db.executemany("INSERT INTO npcfaction (name, faction) VALUES (?, 'f')", [("coachman d'larik",), ("coachwoman darva",), ("jasper darvar",)])
    db.executemany("INSERT INTO spellpattern VALUES (?, ?, ?, ?)", [
        (901008, "Take a Shield Break", 30, "Cancels the Sacred Shield and/or Divine Shield abilities"),
        (701122, "Crusader's Aura", 600, "old"), (900616, "Eldrich Rune", 60, "rune")])
    db.executemany("INSERT INTO spelleffect (spellid, line, source, effects) VALUES (?, ?, 'x', ?)", [
        (901008, "Self, Cure ID?", '{"v":1}'), (701122, "old line", '{"v":1,"old":1}'), (900616, "rune", RUNE_EFF)])
    db.execute("INSERT INTO classtemplates VALUES ('the grinch', ?, ?)", (GRINCH_AB, "Grinch. " + tool.GRINCH_OLD_NOTE))
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return [list(db.execute(f"SELECT * FROM {t} ORDER BY 1")) for t in
                ("npcs", "npcclass", "npcfaction", "spellpattern", "spelleffect", "classtemplates")]
    finally:
        db.close()


def run(path, shard, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, "--shard", shard, *args], capture_output=True, text=True)


class JibLane1Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def audits(self):
        db = sqlite3.connect(self.path)
        try:
            return db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        finally:
            db.close()

    def one(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchone()
        finally:
            db.close()

    def test_apply_and_undo_heirloom(self):
        make(self.path)
        before = dump(self.path)
        r = run(self.path, "heirloom")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before)
        r = run(self.path, "heirloom", "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.one("SELECT npc_name FROM npcs WHERE id = 979")[0], "Coachman Darva")
        self.assertEqual(self.one("SELECT npc_name FROM npcs WHERE id = 974")[0], "Coachman D`larik")
        self.assertEqual(self.one("SELECT npc_name FROM npcs WHERE id = 2862")[0], "Jasper Darvar")
        self.assertIsNotNone(self.one("SELECT 1 FROM npcclass WHERE name = 'coachman darva'"))
        self.assertIsNotNone(self.one("SELECT 1 FROM npcfaction WHERE name = 'coachman d`larik'"))
        self.assertEqual(self.one("SELECT count(*) FROM npcs WHERE npc_type = 128 AND npc_name LIKE '%Porter'")[0], 2)
        self.assertEqual(self.one("SELECT line, source, effects FROM spelleffect WHERE spellid = 901008"),
                         (tool.SHIELD_BREAK_LINE, "video-audit", ""))
        self.assertEqual(self.one("SELECT effects, line FROM spelleffect WHERE spellid = 701122"), (tool.AURA_EFFECTS, tool.AURA_LINE))
        self.assertEqual(self.one("SELECT recast, spelldesc FROM spellpattern WHERE spellid = 701122"), (900, tool.AURA_DESC))
        rune = json.loads(self.one("SELECT effects FROM spelleffect WHERE spellid = 900616")[0])
        self.assertEqual(rune["effects"][0]["mitigation"], {"pct": 100, "max": -3500, "rune": True})
        ab, note = self.one("SELECT abilities, note FROM classtemplates")
        self.assertEqual([a.get("life") for a in json.loads(ab)], [90, 90, 90])
        self.assertEqual(note, "Grinch. " + tool.GRINCH_NEW_NOTE)
        n = self.audits()
        self.assertEqual(n, 18)
        r = run(self.path, "heirloom", "--apply")
        self.assertIn("applied already", r.stdout)
        self.assertEqual(self.audits(), n)
        r = run(self.path, "heirloom", "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before)

    def test_live_leaves_rune_and_grinch(self):
        make(self.path)
        r = run(self.path, "live", "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.one("SELECT effects FROM spelleffect WHERE spellid = 900616")[0], RUNE_EFF)
        self.assertEqual(self.one("SELECT abilities FROM classtemplates")[0], GRINCH_AB)
        self.assertEqual(self.audits(), 15)

    def test_refusals_write_nothing(self):
        for kw in ({"porter_type": 7}, {"both": True}):
            make(self.path, **kw)
            before = dump(self.path)
            r = run(self.path, "heirloom", "--apply")
            self.assertEqual(r.returncode, 1, kw)
            self.assertIn("refused", r.stdout)
            self.assertEqual(dump(self.path), before)
            self.assertEqual(self.audits(), 0)
            os.remove(self.path)

    def test_undo_leaves_a_changed_row(self):
        make(self.path)
        run(self.path, "heirloom", "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE spellpattern SET recast = 1200 WHERE spellid = 701122")
        db.commit()
        db.close()
        r = run(self.path, "heirloom", "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn("left", r.stdout)
        self.assertEqual(self.one("SELECT recast FROM spellpattern WHERE spellid = 701122")[0], 1200)
        self.assertEqual(self.one("SELECT npc_name FROM npcs WHERE id = 979")[0], "Coachwoman Darva")


if __name__ == "__main__":
    unittest.main()
