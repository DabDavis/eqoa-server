#!/usr/bin/env python3
"""shard_sync_20261115.py's NPC matching on two small shards (petitions 2193, 2194, 2198: the 2026-11-15 run doubled
every NPC a GM had moved far, renamed or remade on one shard, and brought back rows a GM had deleted).

A row of the database both shards were cloned from (an id below SHARED_NPC_IDS) is the same NPC on both whatever a GM
did to it since: moved 120 away, renamed, given a new body. So is a row made since under the same id, name and world (the
same tool run on both), and a row the same audited insert made on each shard under different ids. Those are compared
and updated, never copied. A row only one shard has is copied, unless the other shard deleted it: by an audited delete
of that id (whatever name the row had by then), or, for a shared row, with no audit at all (left, listed)."""
import importlib.util
import json
import os
import sqlite3
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("shard_sync", os.path.join(HERE, "shard_sync_20261115.py"))
ss = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ss)

SCHEMA = """
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, levelreq INTEGER);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, source TEXT, name TEXT, giver TEXT);
CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY);
CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER);
CREATE TABLE familiars (patternid INTEGER PRIMARY KEY);
CREATE TABLE heroes_stock (id INTEGER PRIMARY KEY, patternid INTEGER);
CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, PRIMARY KEY (npc_name, tablename));
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, npc_level INTEGER, modelid INTEGER, dt_kill DATE, hp_percent INTEGER, npc_id INTEGER);
CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO accountinfo VALUES (1, 'dab');
"""

# (id, name, world, x, z, level, model): the rows both shards have as they were cloned
SHARED = [
    (122786, "Graumaud Ritualist", 0, 24395.5, 3539.7, 60, 1893243078),   # Heirloom's GM moves it 119 away
    (123092, "cillieane viturille", 5, 5718.0, 3032.0, 56, 1909591809),   # Heirloom renames it and gives it a body
    (118891, "reaver of Graumord", 0, 25100.0, 3600.0, 61, 555),          # left alone: its respawn must stay its own
    (123099, "plague toxicologist", 5, 5522.0, 3080.0, 57, 1478191744),   # Heirloom renames it, then deletes it
    (108167, "sphere of annihilation", 0, 14951.0, 27419.0, 62, 1145480715),  # Heirloom deletes it, no row audit
    (100, "a rat", 0, 100.0, 100.0, 1, 7),
]
SPLIT = ss.SHARED_NPC_IDS if hasattr(ss, "SHARED_NPC_IDS") else 162560


def row(i, name, world, x, z, level, model):
    return dict(id=i, npc_name=name, world=world, x_coord=x, y_coord=50.0, z_coord=z, facing=0, npc_level=level,
                modelid=model, dt_kill=None, hp_percent=100, npc_id=0)


def put(db, r):
    db.execute("INSERT INTO npcs (" + ", ".join(r) + ") VALUES (" + ", ".join("?" for _ in r) + ")", tuple(r.values()))


def audit(db, at, user, action, rk, before, after, detail):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, 1, ?, 'game', '', ?, 'npcs', ?, ?, ?, ?)",
               (at, user, action, json.dumps(rk, separators=(",", ":")), json.dumps(before) if before else None,
                json.dumps(after) if after else None, detail))


def make(path, side):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    if side == "H":
        db.execute("INSERT INTO heirlooms VALUES (5)")   # role_of: the shard with heirlooms rows is Heirloom
    for s in SHARED:
        put(db, row(*s))
    db.execute("INSERT INTO npcrespawn VALUES (118891, 3600, 'an hour', 'gm')")
    surgeon = "the Last House's named (the owner, 2026-11-10): tools/pod_wiki_20261110.py"
    feast = "the user: Freeport's Harvest Feast; ~/charmigrate/harvest_feast_20261108.py"
    tool = "the Dracoliche on the Isle of Dread (the owner, 2026-11-14): tools/dracoliche_20261114.py"
    if side == "H":
        b = row(*SHARED[0]); a = dict(b, x_coord=24276.9, z_coord=3544.9)
        db.execute("UPDATE npcs SET x_coord = 24276.9, z_coord = 3544.9 WHERE id = 122786")
        audit(db, "2026-11-14T02:19:12Z", "forestwood", "update", {"id": 122786}, b, a, "/move in the game: Graumaud Ritualist")
        b = row(*SHARED[1]); a = dict(b, npc_name="cyanea virullata", modelid=185486062, x_coord=5751.0)
        db.execute("UPDATE npcs SET npc_name = 'cyanea virullata', modelid = 185486062, x_coord = 5751.0 WHERE id = 123092")
        audit(db, "2026-11-13T08:33:36Z", "funyun", "update", {"id": 123092}, b, a, "")
        b = dict(row(*SHARED[3]), npc_name="last house toxicologist")
        db.execute("DELETE FROM npcs WHERE id = 123099")
        audit(db, "2026-11-12T17:24:15Z", "forestwood", "delete", {"id": 123099}, b, None,
              "/delete in the game: last house toxicologist (row 123099)")
        db.execute("DELETE FROM npcs WHERE id = 108167")
        # the same tool run on both shards, under its own id here, then moved far by a GM
        d = row(168709, "Dracoliche", 0, 25550.0, 3098.0, 70, 3981833002)
        put(db, dict(d, x_coord=25700.0))
        audit(db, "2026-11-14T04:53:02Z", "dracoliche", "insert", {"id": 168709}, None, d, tool)
        audit(db, "2026-11-14T06:10:54Z", "funyun", "update", {"id": 168709}, d, dict(d, x_coord=25700.0), "")
        # the same tool's row, cloned by a GM (a new body) and the first deleted: a clone of a clone is still that row
        m = row(SPLIT + 20, "Master Surgeon", 5, 4590.0, 3632.0, 58, 1893243078)
        put(db, dict(m, id=SPLIT + 40, x_coord=4547.0, z_coord=3725.0, modelid=-1482901058))
        audit(db, "2026-11-10T03:00:00Z", "dab", "insert", {"id": SPLIT + 20}, None, m, surgeon)
        audit(db, "2026-11-13T05:50:00Z", "forestwood", "insert", {"id": SPLIT + 30}, None, dict(m, id=SPLIT + 30),
              f"/clone in the game: Master Surgeon (row {SPLIT + 20}), a copy of NPC {SPLIT + 20}")
        audit(db, "2026-11-13T05:51:00Z", "forestwood", "delete", {"id": SPLIT + 20}, m, None, "/delete in the game")
        audit(db, "2026-11-13T05:52:58Z", "forestwood", "insert", {"id": SPLIT + 40}, None, dict(m, id=SPLIT + 40),
              f"/clone in the game: Master Surgeon (row {SPLIT + 30}), a copy of NPC {SPLIT + 30}")
        audit(db, "2026-11-13T05:53:08Z", "forestwood", "delete", {"id": SPLIT + 30}, dict(m, id=SPLIT + 30), None, "/delete in the game")
        # a feast both shards' tool made; Heirloom's GM took its own away
        f = row(SPLIT + 50, "Harvest Vendor", 0, 25290.0, 15740.0, 1, 77)
        audit(db, "2026-11-08T08:40:00Z", "dab", "insert", {"id": SPLIT + 50}, None, f, feast)
        audit(db, "2026-11-12T08:25:11Z", "funyun", "delete", {"id": SPLIT + 50}, f, None, "")
        # a named NPC an unaudited tool made on each shard under its own id; Heirloom's GM moved it far
        put(db, row(SPLIT + 60, "Noroxus Prime", 5, 4000.0, 3900.0, 60, 9))
        # Heirloom's own new content (an id of its own above the shared range)
        put(db, row(SPLIT + 5, "Equipment Dyer", 0, 18779.0, 10958.0, 50, 2749054946))
        audit(db, "2026-11-12T10:00:00Z", "dab", "insert", {"id": SPLIT + 5}, None,
              row(SPLIT + 5, "Equipment Dyer", 0, 18779.0, 10958.0, 50, 2749054946), "a dyer by the Outfitter")
    else:
        d = row(162773, "Dracoliche", 0, 25550.0, 3098.0, 70, 3981833002)
        put(db, d)
        audit(db, "2026-11-14T04:54:23Z", "dracoliche", "insert", {"id": 162773}, None, d, tool)
        m = row(SPLIT + 3, "Master Surgeon", 5, 4590.0, 3632.0, 58, 1893243078)
        put(db, m)
        audit(db, "2026-11-10T03:01:00Z", "dab", "insert", {"id": SPLIT + 3}, None, m, surgeon)
        f = row(SPLIT + 7, "Harvest Vendor", 0, 25290.0, 15740.0, 1, 77)
        put(db, f)
        audit(db, "2026-11-08T08:41:17Z", "dab", "insert", {"id": SPLIT + 7}, None, f, feast)
        put(db, row(SPLIT + 9, "Noroxus Prime", 5, 4300.0, 3600.0, 60, 9))
        # Live's own row under the id Heirloom's dyer has: another NPC
        put(db, row(SPLIT + 5, "Harvest Scarecrow", 0, 25212.0, 15559.0, 1, 3248582016))
        audit(db, "2026-11-08T08:41:17Z", "dab", "insert", {"id": SPLIT + 5}, None,
              row(SPLIT + 5, "Harvest Scarecrow", 0, 25212.0, 15559.0, 1, 3248582016), "Freeport's Harvest Feast")
    db.commit()
    db.close()


class NPCMatching(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = tempfile.TemporaryDirectory()
        cls.h, cls.l = os.path.join(cls.tmp.name, "h.db"), os.path.join(cls.tmp.name, "l.db")
        make(cls.h, "H")
        make(cls.l, "L")
        cls.sh = ss.Shards(cls.h, cls.l)
        cls.sh.compare()
        cls.out = cls.sh.classify_all()
        cls.npcs = cls.out["npcs"]

    @classmethod
    def tearDownClass(cls):
        for c in cls.sh.db.values():
            c.close()
        cls.tmp.cleanup()

    def rows_of(self, hid=None, lid=None):
        return [r for r in self.npcs if (hid is not None and hid in r.get("Hid", [])) or (lid is not None and lid in r.get("Lid", []))]

    def test_moved_row_is_paired_by_its_id(self):
        self.assertEqual(self.sh.PAIR["L"].get(122786), 122786)
        rs = self.rows_of(hid=122786, lid=122786)
        self.assertEqual(len(rs), 1, rs)
        self.assertEqual(rs[0]["action"], "update_L_from_H", rs[0])
        self.assertIn("x_coord", rs[0]["changed"])

    def test_renamed_remodelled_shared_row_is_paired_and_updated(self):
        self.assertEqual(self.sh.PAIR["L"].get(123092), 123092)
        rs = self.rows_of(hid=123092, lid=123092)
        self.assertEqual(len(rs), 1, rs)
        self.assertEqual(rs[0]["action"], "update_L_from_H", rs[0])
        self.assertIn("npc_name", rs[0]["changed"])     # a rename is content too, carried to the other shard
        self.assertIn("modelid", rs[0]["changed"])

    def test_untouched_row_keeps_its_side_rows(self):
        self.assertEqual(self.sh.PAIR["L"].get(118891), 118891)
        self.assertFalse(self.rows_of(hid=118891, lid=118891))
        self.assertFalse([r for r in self.out["npcrespawn"] if r["action"] != "leave"])

    def test_same_tool_run_twins_are_paired(self):
        self.assertEqual(self.sh.PAIR["L"].get(162773), 168709)
        rs = self.rows_of(hid=168709, lid=162773)
        self.assertEqual(len(rs), 1, rs)
        self.assertEqual(rs[0]["action"], "update_L_from_H", rs[0])

    def test_clone_of_a_clone_pairs_with_the_other_shards_row(self):
        self.assertEqual(self.sh.PAIR["L"].get(SPLIT + 3), SPLIT + 40)
        rs = self.rows_of(hid=SPLIT + 40, lid=SPLIT + 3)
        self.assertEqual([r["action"] for r in rs], ["update_L_from_H"], rs)

    def test_row_whose_twin_was_deleted_is_not_copied_back(self):
        rs = self.rows_of(lid=SPLIT + 7)
        self.assertEqual(len(rs), 1, rs)
        self.assertEqual(rs[0]["action"], "leave", rs[0])
        self.assertIn("deleted its own row", rs[0]["reason"])

    def test_one_named_npc_on_each_shard_is_paired(self):
        self.assertEqual(self.sh.PAIR["L"].get(SPLIT + 9), SPLIT + 60)
        self.assertFalse([r for r in self.npcs if r["action"].startswith("copy") and "noroxus" in r["key"][0].lower()])

    def test_renamed_then_deleted_row_is_deleted_not_copied_back(self):
        rs = self.rows_of(lid=123099)
        self.assertEqual(len(rs), 1, rs)
        self.assertEqual(rs[0]["action"], "delete_on_L", rs[0])

    def test_shared_row_deleted_without_audit_is_left(self):
        rs = self.rows_of(lid=108167)
        self.assertEqual(len(rs), 1, rs)
        self.assertEqual(rs[0]["action"], "leave", rs[0])
        self.assertNotEqual(rs[0]["action"], "copy_L_to_H")

    def test_same_id_of_another_npc_is_not_paired(self):
        self.assertNotIn(SPLIT + 5, self.sh.PAIR["L"])
        dyer = self.rows_of(hid=SPLIT + 5)
        scarecrow = self.rows_of(lid=SPLIT + 5)
        self.assertEqual([r["action"] for r in dyer], ["copy_H_to_L"])
        self.assertEqual([r["action"] for r in scarecrow], ["copy_L_to_H"])

    def test_nothing_is_copied_but_the_new_rows(self):
        copies = sorted((r["action"], tuple(r.get("Hid") or r.get("Lid"))) for r in self.npcs if r["action"].startswith("copy"))
        self.assertEqual(copies, [("copy_H_to_L", (SPLIT + 5,)), ("copy_L_to_H", (SPLIT + 5,))])


if __name__ == "__main__":
    unittest.main()
