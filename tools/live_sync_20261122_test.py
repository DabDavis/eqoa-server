#!/usr/bin/env python3
"""live_sync_20261122.py on two small shards, both ways: a Heirloom GM's loot row, a new record (its id taken on Live by another
record, so mapped), a placed NPC with its respawn, a reworked quest and a deleted creature go to Live (a); the Christmas
season's item, an heirloom, Live's own fix and an unaudited difference do not (b); a row both shards edited is left for
the owner (c). --apply writes the plan with a gm_audit line a row; a second --apply writes nothing; a new plan on the
synced Live has nothing left in (a); --undo puts Live back exactly. A plan whose row Live changed since leaves that row;
a record id Live gave to something else refuses the whole run. With EQOA_REAL_HEIRLOOM and EQOA_REAL_LIVE (copies),
the same on the real databases, timed."""
import importlib.util
import json
import os
import shutil
import sqlite3
import tempfile
import time
import unittest
from pathlib import Path

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("live_sync", os.path.join(HERE, "live_sync_20261122.py"))
ls = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ls)

SCHEMA = """
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, levelreq INTEGER, itemcost INTEGER,
    itemdesc TEXT);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, source TEXT, name TEXT, giver TEXT, races TEXT, enabled INTEGER);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT, journal TEXT, npcid INTEGER,
    PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT, PRIMARY KEY (questid, step, target));
CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, count INTEGER, done INTEGER,
    PRIMARY KEY (serverid, questid));
CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY);
CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER);
CREATE TABLE familiars (patternid INTEGER PRIMARY KEY);
CREATE TABLE heroes_stock (id INTEGER PRIMARY KEY, patternid INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, npc_level INTEGER, modelid INTEGER, dt_kill DATE, hp_percent INTEGER, npc_id INTEGER);
CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance REAL, rolls INTEGER, note TEXT, source TEXT,
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER,
    minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, world INTEGER, x REAL, z REAL, radius REAL,
    faction TEXT, note TEXT, source TEXT);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER, radius REAL, note TEXT, source TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO accountinfo VALUES (1, 'dab');
INSERT INTO itempattern VALUES (100, 'rusty sword', 12, 1, 10, ''), (200, 'bone chips', -1, 0, 1, ''),
    (300, 'spider silk', -1, 0, 2, '');
INSERT INTO quests VALUES (101000, 'guide', 'Rat Hunt', 'Guard Nolan', '0, 1', 1);
INSERT INTO queststeps VALUES (101000, 1, 'kill', 'rat', '', 'Kill rats.', 0), (101000, 2, 'talk', 'Guard Nolan', '', 'Back.', 0);
INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, modelid, hp_percent, npc_id) VALUES
    (1000, 'rat', 0, 100, 50, 100, 0, 1, 7, 100, 0), (1001, 'Guard Nolan', 0, 200, 50, 200, 0, 30, 8, 100, 0),
    (1002, 'skeleton', 0, 300, 50, 300, 0, 5, 9, 100, 0), (1003, 'spider', 0, 400, 50, 400, 0, 6, 10, 100, 0),
    (1004, 'sphere', 0, 500, 50, 500, 0, 60, 12, 100, 0), (1681, 'Historian Tsali', 0, 24568.8, 52.8, 9142.1, 76, 30, 223572789, 100, 0);
INSERT INTO npcrespawn VALUES (1681, 90, 'named', 'petfix259');
INSERT INTO npcloot VALUES ('skeleton', 'bones', 1, 1, '', 'seed');
INSERT INTO loottables (id, tablename, patternid, weight, minlevel, maxlevel, source) VALUES (1, 'bones', 200, 1, 1, 10, 'seed');
INSERT INTO npcaggro VALUES ('spider', 1, 10, '', 'seed'), ('skeleton', 1, 10, '', 'seed');
"""
LATER = "2026-11-12T10:00:00Z"


def audit(db, user, action, tbl, rk, before, after, detail, at=LATER):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, 1, ?, 'tool', '', ?, ?, ?, ?, ?, ?)",
               (at, user, action, tbl, json.dumps(rk, separators=(",", ":")), json.dumps(before) if before else "",
                json.dumps(after) if after else "", detail))


def make(d):
    h, l = os.path.join(d, "h.db"), os.path.join(d, "l.db")
    for p in (h, l):
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.execute("CREATE TABLE shard_label(shard TEXT)")
        db.execute("INSERT INTO shard_label VALUES (?)", ("heirloom" if p == h else "live",))
        db.commit()
        db.close()
    db = sqlite3.connect(h)
    db.execute("INSERT INTO heirlooms VALUES (900)")
    db.execute("CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT, source TEXT)")
    db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, modelid, hp_percent, npc_id) "
               "VALUES (1998, 'goblin excavator', 0, 900, 50, 900, 0, 23, 13, 100, 0)")
    audit(db, "gm1", "insert", "npcs", {"id": 1998}, None, {"npc_name": "goblin excavator"}, "/place in the game")
    db.execute("INSERT INTO itempattern VALUES (900, 'Heirloom Blade', 12, 1, 0, '')")
    # (a) a GM's loot: a new record (id 960500, which Live gave to another record) in the bones table
    db.execute("INSERT INTO itempattern VALUES (960500, 'cracked skull', -1, 0, 5, 'From a skeleton.')")
    audit(db, "gm1", "insert", "itempattern", {"patternid": 960500}, None, {"itemname": "cracked skull"}, "")
    db.execute("INSERT INTO loottables (id, tablename, patternid, weight, minlevel, maxlevel, source) VALUES (2, 'bones', 960500, 3, 1, 10, 'gm')")
    audit(db, "gm1", "insert", "loottables", {"id": 2}, None, {"tablename": "bones"}, "")
    # (a) a placed NPC with its respawn
    db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, modelid, hp_percent, npc_id) "
               "VALUES (170000, 'skeleton', 0, 320, 50, 330, 10, 5, 9, 100, 0)")
    audit(db, "gm2", "insert", "npcs", {"id": 170000}, None, {"npc_name": "skeleton", "x_coord": 320.0, "z_coord": 330.0},
          "/clone in the game: skeleton (row 1002), a copy of NPC 1002")
    db.execute("INSERT INTO npcrespawn VALUES (170000, 90, 'named', 'gm')")
    audit(db, "gm2", "insert", "npcrespawn", {"npc_row": 170000}, None, {"seconds": 90}, "")
    # (a) the quest reworked: a third step
    db.execute("UPDATE queststeps SET journal = 'Kill three rats.' WHERE questid = 101000 AND step = 1")
    db.execute("INSERT INTO queststeps VALUES (101000, 3, 'give', 'Guard Nolan', '200', 'Bring bones.', 0)")
    audit(db, "gm1", "update", "queststeps", {"questid": 101000, "step": 1}, {"journal": "Kill rats."}, {"journal": "Kill three rats."}, "")
    # (a) a creature deleted on Heirloom
    db.execute("DELETE FROM npcs WHERE id = 1003")
    audit(db, "gm1", "delete", "npcs", {"id": 1003}, {"npc_name": "spider", "id": 1003}, None, "/delete in the game: spider (row 1003)")
    # (b) the Christmas season's item
    db.execute("INSERT INTO itempattern VALUES (716900, 'Holly Token', -1, 0, 0, 'A Christmas token.')")
    audit(db, "dab", "insert", "itempattern", {"patternid": 716900}, None, {"itemname": "Holly Token"}, "the Christmas season: tools/christmas_20261116.py")
    # (c) both edited the guard's level
    db.execute("UPDATE npcs SET npc_level = 35 WHERE id = 1001")
    audit(db, "gm3", "update", "npcs", {"id": 1001}, {"npc_level": 30}, {"npc_level": 35}, "", at="2026-11-13T10:00:00Z")
    # an unaudited price difference: Heirloom's wins
    db.execute("UPDATE itempattern SET itemcost = 11 WHERE patternid = 100")
    # an unaudited difference a Live tool's log names: Live's wins (below)
    db.execute("UPDATE itempattern SET itemcost = 4 WHERE patternid = 200")
    # the sphere: lost on Heirloom with no audit and no log: restored from Live
    db.execute("DELETE FROM npcs WHERE id = 1004")
    db.commit()
    db.close()
    db = sqlite3.connect(l)
    db.execute("INSERT INTO itempattern VALUES (960500, 'mossy stone', -1, 0, 1, 'Live made this one.')")
    db.execute("UPDATE itempattern SET itemcost = 7 WHERE patternid = 200")
    db.execute("CREATE TABLE bones_5100_log (seq INTEGER PRIMARY KEY, patternid INTEGER, before TEXT, after TEXT)")
    db.execute("INSERT INTO bones_5100_log VALUES (1, 200, '1', '7')")
    # a dangling row on Live: its NPC is gone
    db.execute("INSERT INTO npcrespawn VALUES (1999, 30, 'gone', 'gm')")
    # a flag keyed by an NPC row Live no longer has ('#1998'): Heirloom's 1998 is another NPC; never copied, deleted
    db.execute("CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT, source TEXT)")
    db.execute("INSERT INTO npcspawnonly VALUES ('#1998', 'thinned', 'gm')")
    audit(db, "dab", "insert", "itempattern", {"patternid": 960500}, None, {"itemname": "mossy stone"}, "Live petition 5001")
    # (b) Live's own fix: spider silk's price
    db.execute("UPDATE itempattern SET itemcost = 9 WHERE patternid = 300")
    audit(db, "dab", "update", "itempattern", {"patternid": 300}, {"itemcost": 2}, {"itemcost": 9}, "Live petition 5002")
    db.execute("UPDATE npcs SET npc_level = 32 WHERE id = 1001")
    audit(db, "dab", "update", "npcs", {"id": 1001}, {"npc_level": 30}, {"npc_level": 32}, "Live petition 5003", at="2026-11-12T11:00:00Z")
    db.commit()
    db.close()
    return h, l


CONTENT = ("itempattern", "quests", "queststeps", "queststeptargets", "npcs", "npcrespawn", "npcloot", "loottables",
           "npcaggro", "npcfaction", "charquests", "npcspawnonly")


def snapshot(path, tables=CONTENT):
    db = sqlite3.connect(path)
    try:
        out = {t: sorted(db.execute(f"SELECT rowid, * FROM {t}").fetchall(), key=repr) for t in tables}
        out["seq"] = sorted(r for r in db.execute("SELECT name, seq FROM sqlite_sequence") if r[0] not in ("gm_audit", ls.LOG))
        return out
    finally:
        db.close()


def quiet(f, *a):
    import contextlib
    import io
    with contextlib.redirect_stdout(io.StringIO()):
        return f(*a)


def replay_fixture(path, plan_path):
    """Exercise the historical row writer independently of ticket admission."""
    with open(plan_path) as f:
        plan = json.load(f)
    return ls._replay(path, lambda db, side: plan[side])


def undo_fixture(path, shard, after_seq=0):
    db = sqlite3.connect(path)
    try:
        runs = db.execute(f"SELECT DISTINCT run FROM {ls.LOG} WHERE undone=0 AND seq>?", (after_seq,)).fetchall()
    finally:
        db.close()
    if not runs:
        return {}, []
    [(run,)] = runs
    return ls.undo(path, run, shard)


def real_test_input(source, destination, shard, use_disposable):
    """Only labelled offline copies may enter the historical bulk stress test."""
    path = os.path.realpath(source)
    db = ls.ss.readonly(path)
    try:
        cfg = Path(path).parent / "config.yaml"
        if cfg.is_file() and ls.tickets.config_binding(cfg)[1] == Path(path):
            raise ValueError("configured server database cannot be a stress-test input")
        if not ls.ss.has(db, "shard_label") or ls.tickets.bound_shard(db) != shard:
            raise ValueError(f"expected a labelled disposable {shard} copy")
        if use_disposable:
            return path
        copied = sqlite3.connect(destination)
        try:
            db.backup(copied)
        finally:
            copied.close()
        return destination
    finally:
        db.close()


class ShardSync(unittest.TestCase):
    def test_reuse_only_explicit_offline_copies(self):
        unused = os.path.join(self.d, "extra.db")
        self.assertEqual(real_test_input(self.l, unused, "live", True), os.path.realpath(self.l))
        self.assertFalse(os.path.exists(unused))
        with self.assertRaisesRegex(ValueError, "labelled"):
            real_test_input(self.h, unused, "live", True)
        Path(self.d, "config.yaml").write_text("db_path: l.db\ngm:\n  shard_id: live\n", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "configured server"):
            real_test_input(self.l, unused, "live", True)

    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.h, self.l = make(self.d)
        self.plan = os.path.join(self.d, "plan.json")
        self.report = os.path.join(self.d, "report.json")

    def tearDown(self):
        shutil.rmtree(self.d, ignore_errors=True)

    def rows(self):
        with open(self.report) as f:
            rep = json.load(f)
        return {(r["table"], json.dumps(r["key"]) + " " + str(r["name"]).lower()): r for r in rep["rows"]}

    def find(self, rows, table, word):
        return sorted((r["cat"], r["dir"]) for (t, k), r in rows.items() if t == table and word in k)

    def test_gm_return_scroll_stays_on_heirloom(self):
        from gm_return_scroll_20261127 import NAME, TEST_ID

        with sqlite3.connect(self.h) as db:
            db.execute("INSERT INTO itempattern VALUES (?, ?, -1, 1, 0, '')", (TEST_ID, NAME))
            audit(db, "dab", "insert", "itempattern", {"patternid": TEST_ID}, None, {"itemname": NAME}, "GM test scroll")
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        self.assertEqual(self.find(self.rows(), "itempattern", NAME.lower()), [("b", "-")])

    def test_groups(self):
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        rows = self.rows()
        f = lambda t, w: self.find(rows, t, w)
        self.assertEqual(f("itempattern", "cracked skull"), [("HL", "to Live")])
        self.assertEqual(f("itempattern", "holly token"), [("b", "-")])
        self.assertEqual(f("itempattern", "#300"), [("LH", "to Heirloom")])         # Live's own fix
        self.assertEqual(f("itempattern", "#100"), [("R-noaudit", "to Live")])      # Heirloom's value wins
        self.assertEqual(f("itempattern", "#200"), [("R-noaudit", "to Heirloom")])  # a Live tool's log names it
        self.assertEqual(f("npcs", "Guard Nolan"), [("R-both", "to Live")])         # Heirloom's edit is the later
        self.assertIn(("HL", "to Live"), f("npcs", "skeleton"))
        self.assertEqual(f("npcs", "spider"), [("HL", "to Live")])                   # Heirloom deleted it
        self.assertEqual(f("npcs", "sphere"), [("R-deletes", "to Heirloom")])        # lost with no audit: restored
        self.assertEqual(f("npcrespawn", "missing 1999"), [("R-dangling", "to Live")])
        self.assertEqual(f("npcspawnonly", "#1998"), [("R-dangling", "to Live")])
        self.assertTrue(all(c == ("HL", "to Live") for c in f("queststeps", "101000")))
        self.assertTrue(any(r["cat"] == "b" for (t, k), r in rows.items() if "heirloom blade" in k))

    def test_same_id_other_copy_is_not_copied(self):
        # each shard minted its own ids after the split: Live's 162611 is Heirloom's 162621 (another copy, elsewhere)
        for path, rows in ((self.h, ((162611, 500), (162621, 600))), (self.l, ((162601, 500), (162611, 600)))):
            db = sqlite3.connect(path)
            for i, x in rows:
                db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, modelid, "
                           "hp_percent, npc_id) VALUES (?, 'Blood Thirsty Minion', 5, ?, 50, ?, 0, 50, 11, 100, 0)", (i, x, x))
            db.commit()
            db.close()
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        self.assertEqual([k for k in self.rows() if "minion" in k[1]], [])

    def test_apply_both_reapply_replan_undo(self):
        before = {"H": snapshot(self.h), "L": snapshot(self.l)}
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        done = {}
        for s, p in (("L", self.l), ("H", self.h)):
            d, there, left = quiet(replay_fixture, p, self.plan)
            self.assertGreater(sum(d.values()), 0)
            self.assertEqual(left, [])
            done[s] = d
        db = sqlite3.connect(self.l)
        new = db.execute("SELECT patternid FROM itempattern WHERE itemname = 'cracked skull'").fetchone()[0]
        self.assertNotEqual(new, 960500)   # 960500 is Live's own mossy stone
        self.assertEqual(db.execute("SELECT itemname FROM itempattern WHERE patternid = 960500").fetchone()[0], "mossy stone")
        self.assertEqual(db.execute("SELECT COUNT(*) FROM loottables WHERE tablename = 'bones' AND patternid = ?", (new,)).fetchone()[0], 1)
        sk = db.execute("SELECT id FROM npcs WHERE npc_name = 'skeleton' AND x_coord = 320").fetchone()[0]
        self.assertEqual(db.execute("SELECT seconds FROM npcrespawn WHERE npc_row = ?", (sk,)).fetchone()[0], 90)
        self.assertIsNone(db.execute("SELECT 1 FROM npcs WHERE id = 1003").fetchone())
        self.assertIsNone(db.execute("SELECT 1 FROM npcrespawn WHERE npc_row = 1999").fetchone())
        self.assertIsNone(db.execute("SELECT 1 FROM npcspawnonly WHERE name = '#1998'").fetchone())
        self.assertEqual(db.execute("SELECT COUNT(*) FROM queststeps WHERE questid = 101000").fetchone()[0], 3)
        self.assertIsNone(db.execute("SELECT 1 FROM itempattern WHERE itemname = 'Holly Token'").fetchone())
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 100").fetchone()[0], 11)
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 200").fetchone()[0], 7)
        self.assertEqual(db.execute("SELECT npc_level FROM npcs WHERE id = 1001").fetchone()[0], 35)
        n_audit = db.execute("SELECT COUNT(*) FROM gm_audit WHERE detail LIKE 'Shard sync 2026-11-22%'").fetchone()[0]
        self.assertEqual(n_audit, sum(done["L"].values()))
        db.close()
        db = sqlite3.connect(self.h)
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 300").fetchone()[0], 9)
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 200").fetchone()[0], 7)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM npcs WHERE npc_name = 'sphere'").fetchone()[0], 1)
        self.assertIsNone(db.execute("SELECT 1 FROM npcspawnonly").fetchone())   # Live's dangling flag never reaches 1998
        # Live's own record, at an id of Heirloom's own (960500 is the cracked skull there)
        self.assertNotEqual(db.execute("SELECT patternid FROM itempattern WHERE itemname = 'mossy stone'").fetchone()[0], 960500)
        db.close()
        for p in (self.h, self.l):   # Tsali on the hill north of Freeport, both shards, with his respawn and faction
            db = sqlite3.connect(p)
            t = db.execute("SELECT id, npc_level, modelid, y_coord FROM npcs WHERE npc_name = 'Historian Tsali' AND x_coord = 25492").fetchone()
            self.assertEqual(t[1:], (30, 223572789, 77.88))
            self.assertEqual(db.execute("SELECT seconds FROM npcrespawn WHERE npc_row = ?", (t[0],)).fetchone()[0], 90)
            self.assertEqual(db.execute("SELECT faction FROM npcfaction WHERE x = 25492").fetchone()[0], "Citizens of Freeport")
            db.close()
        applied = {"H": snapshot(self.h), "L": snapshot(self.l)}
        for p in (self.l, self.h):
            d2, _, _ = quiet(replay_fixture, p, self.plan)
            self.assertEqual(sum(d2.values()), 0)
        self.assertEqual({"H": snapshot(self.h), "L": snapshot(self.l)}, applied)
        plan2 = os.path.join(self.d, "plan2.json")
        quiet(ls.make_plan, self.h, self.l, plan2, None)
        with open(plan2) as f:
            p2 = json.load(f)
        self.assertEqual((p2["H"]["ops"], p2["L"]["ops"]), ([], []))
        for s, p in (("H", self.h), ("L", self.l)):
            n, left3 = quiet(undo_fixture, p, {"H": "heirloom", "L": "live"}[s])
            self.assertEqual(left3, [])
            self.assertEqual(snapshot(p), before[s])

    def test_changed_since_plan_and_taken_id(self):
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        db = sqlite3.connect(self.l)
        db.execute("UPDATE queststeps SET journal = 'A Live GM changed it.' WHERE questid = 101000 AND step = 1")
        db.commit()
        db.close()
        done, there, left = quiet(replay_fixture, self.l, self.plan)
        self.assertTrue(any((x["unit"] or "").startswith("quest:") for x in left))
        db = sqlite3.connect(self.l)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM queststeps WHERE questid = 101000").fetchone()[0], 2)
        db.close()
        quiet(undo_fixture, self.l, "live")
        # a record id the plan inserts, given to another record since: the whole run refused
        with open(self.plan) as f:
            plan = json.load(f)
        new = [o for o in plan["L"]["ops"] if o["t"] == "itempattern" and o["before"] is None][0]["pk"]["patternid"]
        db = sqlite3.connect(self.l)
        db.execute("INSERT INTO itempattern VALUES (?, 'something else', -1, 0, 0, '')", (new,))
        db.commit()
        db.close()
        snap = snapshot(self.l)
        with self.assertRaises(ls.Refused):
            quiet(replay_fixture, self.l, self.plan)
        self.assertEqual(snapshot(self.l), snap)

    def test_part_way_character_keeps_the_quest(self):
        db = sqlite3.connect(self.l)
        db.execute("INSERT INTO charquests VALUES (7, 101000, 2, 0, 0)")
        db.commit()
        db.close()
        # step 2 a different task on Heirloom: the quest's shape changes
        db = sqlite3.connect(self.h)
        db.execute("UPDATE queststeps SET kind = 'loot', target = 'skeleton' WHERE questid = 101000 AND step = 2")
        db.commit()
        db.close()
        quiet(ls.make_plan, self.h, self.l, self.plan, None)
        done, there, left = quiet(replay_fixture, self.l, self.plan)
        self.assertTrue(any("part-way" in x["why"] for x in left))
        db = sqlite3.connect(self.l)
        self.assertEqual(db.execute("SELECT step FROM charquests WHERE serverid = 7").fetchone()[0], 2)
        self.assertEqual(db.execute("SELECT kind FROM queststeps WHERE questid = 101000 AND step = 2").fetchone()[0], "talk")
        db.close()


@unittest.skipUnless(os.environ.get("EQOA_REAL_HEIRLOOM") and os.environ.get("EQOA_REAL_LIVE"), "real shard copies not named")
class RealShards(unittest.TestCase):
    def test_real(self):
        d = tempfile.mkdtemp(dir=os.environ.get("EQOA_REAL_TMP") or None)
        try:
            reuse = os.environ.get("EQOA_REAL_USE_DISPOSABLE") == "1"
            p = {side: real_test_input(os.environ["EQOA_REAL_" + name.upper()], os.path.join(d, name + ".db"), name, reuse)
                 for side, name in (("H", "heirloom"), ("L", "live"))}
            plan = os.path.join(d, "plan.json")
            ls.make_plan(p["H"], p["L"], plan, None)
            tables, start_seq = {}, {}
            for s, path in p.items():
                db = ls.ss.readonly(path)
                try:
                    tables[s] = [t for t in ls.ss.ORDER if ls.ss.has(db, t)]
                    start_seq[s] = db.execute(f"SELECT coalesce(max(seq),0) FROM {ls.LOG}").fetchone()[0] if ls.ss.has(db, ls.LOG) else 0
                finally:
                    db.close()
            before = {s: snapshot(p[s], tables[s]) for s in "HL"}
            for s in "LH":
                t0 = time.time()
                done, there, left = replay_fixture(p[s], plan)
                print(f"--apply {s}: {time.time() - t0:.2f} s, {sum(done.values())} rows")
            for s in "LH":
                done2, _, _ = replay_fixture(p[s], plan)
                self.assertEqual(sum(done2.values()), 0)
            for s in "HL":
                n, left3 = undo_fixture(p[s], {"H": "heirloom", "L": "live"}[s], start_seq[s])
                self.assertEqual(left3, [])
                self.assertEqual(snapshot(p[s], tables[s]), before[s])
        finally:
            shutil.rmtree(d, ignore_errors=True)


if __name__ == "__main__":
    unittest.main()
