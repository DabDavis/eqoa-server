#!/usr/bin/env python3
"""pet1122_items_20261122.py on a small database: apply, a second apply that writes nothing, undo exact; a row not as
read refuses the whole run; the looks only with --looks; the draughts on Live alone; the panel's requests."""
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import pet1122_items_20261122 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT, tclass INTEGER);
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER, addedorder INTEGER,
    onHotBar INTEGER, whereonBar INTEGER, unk1 INTEGER, showhide INTEGER);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, icon INTEGER, spellname TEXT);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '');
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, classuse INTEGER,
    maxstack INTEGER, procanim INTEGER, HoT INTEGER, PoT INTEGER, itemicon INTEGER, model INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, patternid INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, equiploc INTEGER);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, items TEXT);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY);
CREATE TABLE itemspell (patternid INTEGER PRIMARY KEY);
CREATE TABLE foodeffect (patternid INTEGER PRIMARY KEY);
"""


def j(v):
    return json.dumps(v, separators=(",", ":"))


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO characters VALUES (?, ?, ?)", [(1, "Ranger", 1), (2, "Rogue", 6)])
    db.executemany("INSERT INTO spellpattern VALUES (?, ?, ?, ?)", [
        (700013, 1, 1, "Sneak"), (700021, 3, 1, "Sneak"), (700164, 16, 1, "Stalk"),
        (901098, 1, tool.FORAGE_ICON, "Toxophilite"), (700045, 5, tool.FORAGE_ICON, "Forage Berries"),
        (901503, 60, 0, "Deathcleave"), (701122, 60, 0, "Crusader's Aura"), (700095, 12, 0, "Dirge of Defense")]
        + [(901490 + i, 55, 0, n) for i, n in enumerate(c for c in tool.CLEAVES if c != "Deathcleave")])
    db.executemany("INSERT INTO classspells VALUES (?, ?, 'x')", [(1, 700013), (6, 700021), (1, 700164), (1, 901098)])
    db.executemany("INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) "
                   "VALUES (?, ?, 0, 0, 0, 0, 0)", [(1, 700013), (1, 700164), (1, 901098), (2, 700021)])
    cleave = {"v": 1, "scope": "target", "effects": [{"kind": "drain", "amount": -1}, {"kind": "hp", "amount": -2},
                                                     {"kind": "hp2", "amount": -3}]}
    effects = [(700045, "Self, Summon foraged berries", {"v": 1, "scope": "self", "effects": [
        {"kind": "create_item", "item": {"count": 1, "name": "foraged berries", "patternid": 50101}}]}),
        (701122, "x", {"v": 1, "scope": "self", "stack": 2, "effects": [{"kind": "proc"}]}),
        (700095, "Group, Increase DEX by 10, Increase AC by 50", {"v": 1, "stack": 173, "effects": [
            {"kind": "stat", "stat": "dex", "amount": 10}, {"kind": "stat", "stat": "ac", "amount": 50}]})]
    effects += [(sid, "x", cleave) for (sid,) in db.execute("SELECT spellid FROM spellpattern WHERE spellname LIKE '%cleave'")]
    db.executemany("INSERT INTO spelleffect (spellid, line, effects) VALUES (?, ?, ?)", [(s, l, j(e)) for s, l, e in effects])
    items = [(700002, "Sneak", -1, 2, 1, 2, 0, 0, 1, 0), (8539, "Sneak Scroll", -1, 64, 1, 2, 0, 0, 1, 0),
             (700012, "Stalk", -1, 2, 1, 2, 0, 0, 1, 0),
             (50101, "foraged berries", -1, 32767, 1, 2, 25, 0, tool.BREAD_ICON, 0),
             (700153, "Red Burn Berries", -1, 32767, 1, 2, 10, 0, tool.BREAD_ICON, 0),
             (701198, "Claws of Mayhem", 12, 16, 1, 0, 0, 0, 1, 7), (980118, "Ascended Claws of Mayhem", 12, 16, 1, 0, 0, 0, 1, 7),
             (5356, "Blade of Nature's Purity", 12, 2, 1, 0, 0, 0, 1, 7),
             (700370, "spikeshell crab leg", -1, 32767, 1, 2, 0, 0, 1, 0),
             (700372, "a quest bone", -1, 32767, 1, 2, 0, 0, 1, 0),
             (700371, "a used thing", -1, 32767, 1, 2, 0, 0, 1, 0),
             (1013, "weak herbs", -1, 32767, 999, 2, 0, 0, 1, 0),
             (960201, "Chosen's Baton", 14, 32767, 1, 0, 0, 0, 11, 12), (700997, "Glory of Elddar", 15, 4096, 1, 0, 0, 0, 13, 14),
             (718256, "Katana of Toxicity", 12, 1, 1, 0, 0, 0, 21, 22), (713079, "Katana of Severing", 12, 1, 1, 0, 0, 0, 21, 22),
             (5433, "wakizashi of toxicity", 14, 1, 1, 0, 0, 0, 23, 24),
             (5447, "longbow of toxicity", 16, 1, 1, 0, 0, 0, 30, 31), (1, "Centaur Longbow", 16, 1, 1, 0, 0, 0, 30, 32),
             (960200, "Chosen's Rapier", 14, 1, 1, 0, 0, 0, 33, 34), (2, "Dainty Rapier", 12, 1, 1, 0, 0, 0, 35, 36),
             (4775, "Hammer of Piety", 12, 1023, 1, 0, 0, 0, 15, 16), (4765, "Hammer of Divinity", 12, 1023, 1, 0, 0, 0, 17, 18)]
    items += [(701200 + i, n, 8, 1024, 99, 0, 0, 0, 1, 0) for i, n in enumerate(tool.TROPHIES)]
    items += [(50023 + i, n, -1, 16384, 2000, 2, 0, 0, 1, 0) for i, n in enumerate(tool.DRAUGHTS)]
    db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", items)
    db.executemany("INSERT INTO npcitem (npcid, patternid) VALUES (?, ?)", [(7, 700002), (7, 700012), (7, 8539)])
    db.executemany("INSERT INTO queststeps VALUES (1, ?, 'loot', ?)", [(1, "700370"), (2, "700371*2"), (3, "1013"), (4, "700372")])
    db.execute("INSERT INTO itemeffect VALUES (700371)")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


def snapshot(path):
    db = sqlite3.connect(path)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY 1"):
        if t != "gm_audit":
            out[t] = sorted(map(repr, db.execute(f'SELECT * FROM "{t}"').fetchall()))
    db.close()
    return out


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *a):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        before = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "live")
        self.assertEqual(rc, 0, out)
        self.assertEqual(snapshot(self.db), before, "a dry run wrote")
        rc, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(rc, 0, out)
        # the ranger's Sneak and Stalk gone, the rogue's Sneak and its scroll kept
        self.assertEqual(self.q("SELECT class, spellid FROM classspells ORDER BY 1, 2"), [(1, 901098), (6, 700021)])
        self.assertEqual(self.q("SELECT serverid, spellid FROM spells ORDER BY 1, 2"), [(1, 901098), (2, 700021)])
        self.assertEqual(self.q("SELECT patternid FROM npcitem"), [(8539,)])
        self.assertEqual(self.q("SELECT icon FROM spellpattern WHERE spellid = 901098"), [(tool.BOW_ICON,)])
        eff = json.loads(self.q("SELECT effects FROM spelleffect WHERE spellid = 700045")[0][0])
        self.assertEqual(eff["effects"][0]["item"]["count"], 3)
        self.assertEqual(self.q("SELECT DISTINCT itemicon FROM itempattern WHERE patternid IN (50101, 700153)"),
                         [(tool.BERRIES_ICON,)])
        self.assertEqual(self.q("SELECT DISTINCT equipslot FROM itempattern WHERE maxstack = 99"), [(-1,)])
        # 700370 is held by an open work-board entry (HELD): left
        self.assertEqual(self.q("SELECT patternid, procanim FROM itempattern WHERE patternid IN (700370, 700371, 700372, 1013) "
                                "ORDER BY 1"), [(1013, 2), (700370, 2), (700371, 2), (700372, 0)])
        for (e,) in self.q("SELECT effects FROM spelleffect WHERE spellid IN (SELECT spellid FROM spellpattern "
                           "WHERE spellname LIKE '%cleave')"):
            e = json.loads(e)
            self.assertEqual((e["scope"], e["radius"]), ("target_aoe", tool.CLEAVE_RADIUS))
        self.assertEqual(self.q("SELECT DISTINCT equipslot FROM itempattern WHERE patternid IN (701198, 980118, 5356)"), [(14,)])
        self.assertEqual(json.loads(self.q("SELECT effects FROM spelleffect WHERE spellid = 701122")[0][0])["stack"], 701122)
        dirge = self.q("SELECT effects, line FROM spelleffect WHERE spellid = 700095")[0]
        self.assertEqual(json.loads(dirge[0])["effects"][0]["stat"], "agi")
        self.assertIn("AGI by 10", dirge[1])
        self.assertEqual(self.q("SELECT DISTINCT maxstack FROM itempattern WHERE itemname LIKE '%Draught'"), [(50,)])
        self.assertEqual(self.q("SELECT model FROM itempattern WHERE patternid = 960201"), [(12,)], "a look without --looks")
        self.assertEqual(self.q("SELECT model, itemicon FROM itempattern WHERE patternid = 5433"), [(22, 21)])
        self.assertEqual(self.q("SELECT model, itemicon FROM itempattern WHERE patternid IN (5447, 960200) ORDER BY 1"),
                         [(32, 30), (36, 35)])
        self.assertGreater(self.q("SELECT count(*) FROM gm_audit")[0][0], 20)

        rc, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to write", out)

        rc, out = run("--db", self.db, "--shard", "live", "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(snapshot(self.db), before)

    def test_heirloom_keeps_its_draughts_and_looks_on_ask(self):
        rc, out = run("--db", self.db, "--shard", "heirloom", "--looks", "--apply")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.q("SELECT DISTINCT maxstack FROM itempattern WHERE itemname LIKE '%Draught'"), [(2000,)])
        self.assertEqual(self.q("SELECT model, itemicon FROM itempattern WHERE patternid IN (960201, 4775) ORDER BY 1"),
                         [(14, 13), (18, 17)])

    def test_a_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itempattern SET maxstack = 5 WHERE itemname = 'Nixie Cap'")
        db.commit()
        db.close()
        before = snapshot(self.db)
        rc, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(snapshot(self.db), before)

    def test_panel_requests(self):
        db = sqlite3.connect(self.db)
        p = tool.Plan(db, "heirloom", False)
        tool.build(p)
        reqs = tool.panel_requests(p)
        db.close()
        ops = {op for op, _ in reqs}
        self.assertEqual(ops, {"delete_row", "update_row"})
        up = [b for op, b in reqs if op == "update_row" and b["table"] == "spellpattern"][0]
        self.assertEqual(up, {"table": "spellpattern", "key": {"spellid": 901098}, "shard": "heirloom",
                              "values": {"icon": tool.BOW_ICON}, "expect": {"icon": tool.FORAGE_ICON}})


if __name__ == "__main__":
    unittest.main()
