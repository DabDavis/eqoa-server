#!/usr/bin/env python3
"""live_sync_20261122.py on two small shards: a Heirloom GM's loot row, a new record (its id taken on Live by another
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
    (1002, 'skeleton', 0, 300, 50, 300, 0, 5, 9, 100, 0), (1003, 'spider', 0, 400, 50, 400, 0, 6, 10, 100, 0);
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
        db.commit()
        db.close()
    db = sqlite3.connect(h)
    db.execute("INSERT INTO heirlooms VALUES (900)")
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
    # an unaudited price difference: (c)
    db.execute("UPDATE itempattern SET itemcost = 11 WHERE patternid = 100")
    db.commit()
    db.close()
    db = sqlite3.connect(l)
    db.execute("INSERT INTO itempattern VALUES (960500, 'mossy stone', -1, 0, 1, 'Live made this one.')")
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
           "npcaggro", "charquests")


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


class LiveSync(unittest.TestCase):
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

    def test_groups(self):
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        rows = self.rows()
        cat = {k: r["cat"] for k, r in rows.items()}
        def find(table, word):
            return [r["cat"] for (t, k), r in rows.items() if t == table and word in k]
        self.assertEqual(find("itempattern", "cracked skull"), ["a"])
        self.assertEqual(find("itempattern", "holly token"), ["b"])
        self.assertEqual(find("itempattern", "#300"), ["b"])          # Live's own fix, the other way
        self.assertEqual(find("itempattern", "#100"), ["c"])          # no own edit on either shard
        self.assertEqual(find("npcs", "Guard Nolan"), ["c"])         # both edited
        self.assertIn("a", find("npcs", "skeleton"))
        self.assertEqual(find("npcs", "spider"), ["a"])               # Heirloom deleted it
        self.assertTrue(all(c == "a" for c in find("queststeps", "101000")))
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

    def test_apply_reapply_replan_undo(self):
        before = snapshot(self.l)
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        done, there, left = quiet(ls.apply_plan, self.l, self.plan)
        self.assertGreater(sum(done.values()), 0)
        self.assertEqual(left, [])
        db = sqlite3.connect(self.l)
        new = db.execute("SELECT patternid FROM itempattern WHERE itemname = 'cracked skull'").fetchone()[0]
        self.assertNotEqual(new, 960500)   # 960500 is Live's own mossy stone
        self.assertEqual(db.execute("SELECT itemname FROM itempattern WHERE patternid = 960500").fetchone()[0], "mossy stone")
        self.assertEqual(db.execute("SELECT COUNT(*) FROM loottables WHERE tablename = 'bones' AND patternid = ?", (new,)).fetchone()[0], 1)
        sk = db.execute("SELECT id FROM npcs WHERE npc_name = 'skeleton' AND x_coord = 320").fetchone()[0]
        self.assertEqual(db.execute("SELECT seconds FROM npcrespawn WHERE npc_row = ?", (sk,)).fetchone()[0], 90)
        self.assertIsNone(db.execute("SELECT 1 FROM npcs WHERE id = 1003").fetchone())
        self.assertEqual(db.execute("SELECT COUNT(*) FROM queststeps WHERE questid = 101000").fetchone()[0], 3)
        self.assertIsNone(db.execute("SELECT 1 FROM itempattern WHERE itemname = 'Holly Token'").fetchone())
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE patternid = 300").fetchone()[0], 9)
        self.assertEqual(db.execute("SELECT npc_level FROM npcs WHERE id = 1001").fetchone()[0], 32)
        n_audit = db.execute("SELECT COUNT(*) FROM gm_audit WHERE detail LIKE 'Live sync 2026-11-22%'").fetchone()[0]
        self.assertEqual(n_audit, sum(done.values()))
        db.close()
        applied = snapshot(self.l)
        done2, there2, left2 = quiet(ls.apply_plan, self.l, self.plan)
        self.assertEqual(sum(done2.values()), 0)
        self.assertEqual(snapshot(self.l), applied)
        plan2 = os.path.join(self.d, "plan2.json")
        quiet(ls.make_plan, self.h, self.l, plan2, None)
        with open(plan2) as f:
            self.assertEqual(json.load(f)["ops"], [])
        n, left3 = quiet(ls.undo, self.l)
        self.assertEqual(left3, [])
        self.assertEqual(snapshot(self.l), before)

    def test_changed_since_plan_and_taken_id(self):
        quiet(ls.make_plan, self.h, self.l, self.plan, self.report)
        db = sqlite3.connect(self.l)
        db.execute("UPDATE queststeps SET journal = 'A Live GM changed it.' WHERE questid = 101000 AND step = 1")
        db.commit()
        db.close()
        done, there, left = quiet(ls.apply_plan, self.l, self.plan)
        self.assertTrue(any((x["unit"] or "").startswith("quest:") for x in left))
        db = sqlite3.connect(self.l)
        self.assertEqual(db.execute("SELECT COUNT(*) FROM queststeps WHERE questid = 101000").fetchone()[0], 2)
        db.close()
        quiet(ls.undo, self.l)
        # a record id the plan inserts, given to another record since: the whole run refused
        with open(self.plan) as f:
            plan = json.load(f)
        new = [o for o in plan["ops"] if o["t"] == "itempattern" and o["before"] is None][0]["pk"]["patternid"]
        db = sqlite3.connect(self.l)
        db.execute("INSERT INTO itempattern VALUES (?, 'something else', -1, 0, 0, '')", (new,))
        db.commit()
        db.close()
        snap = snapshot(self.l)
        with self.assertRaises(ls.Refused):
            quiet(ls.apply_plan, self.l, self.plan)
        self.assertEqual(snapshot(self.l), snap)

    def test_part_way_character_keeps_the_quest(self):
        db = sqlite3.connect(self.l)
        db.execute("INSERT INTO charquests VALUES (7, 101000, 2, 0, 0)")
        db.commit()
        db.close()
        # step 3 added: the shape changes only if a step's kind/target changes; make step 2 a different task on Heirloom
        db = sqlite3.connect(self.h)
        db.execute("UPDATE queststeps SET kind = 'loot', target = 'skeleton' WHERE questid = 101000 AND step = 2")
        db.commit()
        db.close()
        quiet(ls.make_plan, self.h, self.l, self.plan, None)
        done, there, left = quiet(ls.apply_plan, self.l, self.plan)
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
            live = os.path.join(d, "live.db")
            shutil.copy(os.environ["EQOA_REAL_LIVE"], live)
            plan = os.path.join(d, "plan.json")
            ls.make_plan(os.environ["EQOA_REAL_HEIRLOOM"], live, plan, None)
            tables = [t for t in ls.ss.ORDER if ls.ss.has(sqlite3.connect(live), t)]
            before = snapshot(live, tables)
            t0 = time.time()
            done, there, left = ls.apply_plan(live, plan)
            print(f"--apply: {time.time() - t0:.2f} s, {sum(done.values())} rows")
            self.assertGreater(sum(done.values()), 0)
            applied = snapshot(live, tables)
            done2, _, _ = ls.apply_plan(live, plan)
            self.assertEqual(sum(done2.values()), 0)
            self.assertEqual(snapshot(live, tables), applied)
            n, left3 = ls.undo(live)
            self.assertEqual(left3, [])
            self.assertEqual(snapshot(live, tables), before)
        finally:
            shutil.rmtree(d, ignore_errors=True)


if __name__ == "__main__":
    unittest.main()
