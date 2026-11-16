#!/usr/bin/env python3
"""tools/prima_fill_gaps_20261116.py on a made shard and a made Prima catalog."""
import csv
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "prima_fill_gaps_20261116.py")
STATS = ["levelreq", "maxhp", "weapondamage", "str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX",
         "PoT", "HoT", "AC", "PR", "DR", "FR", "CR", "LR", "AR"]


def make_shard(path, items, extra=()):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT NOT NULL, equipslot INTEGER NOT NULL DEFAULT 0, "
               + ", ".join(f"{c} INTEGER" for c in STATS) + ")")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL, "
               "username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '', "
               "rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER)")
    db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
    db.execute("INSERT INTO accountinfo VALUES (7, 'dab')")
    for pid, name, vals in items:
        cols = ["patternid", "itemname"] + list(vals)
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   (pid, name, *vals.values()))
    for sql in extra:
        db.execute(sql)
    db.commit()
    db.close()


def make_prima(path, rows):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE prima_items (guide_id INTEGER PRIMARY KEY, itemname TEXT NOT NULL, region TEXT NOT NULL, "
               "rarity TEXT NOT NULL, printed_page INTEGER NOT NULL, review_note TEXT, "
               + ", ".join(f"{c} INTEGER" for c in STATS) + ")")
    for gid, name, vals, note in rows:
        cols = ["guide_id", "itemname", "region", "rarity", "printed_page", "review_note"] + list(vals)
        db.execute(f"INSERT INTO prima_items ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   (gid, name, "Forest", "Rare", 400 + gid, note, *vals.values()))
    db.commit()
    db.close()


ITEMS = [
    (1001, "Plain Greaves", {"str": 11, "sta": 0, "AC": None, "maxhp": 0, "levelreq": 30}),   # fill sta, AC; not maxhp
    (1002, "Busy Ring", {"str": 5, "HPMAX": 20}),                                               # str 5 kept (Prima 8), HPMAX kept
    (1003, "Slid Ring", {"str": 100, "sta": 50}),                                               # the dump's slide: left
    (1004, "Twin Name", {}), (1005, "twin  name", {}),                                          # ambiguous here: left
    (1006, "Noted Cap", {}),                                                                    # Prima row has a review_note
    (1007, "Audited Belt", {}),                                                                 # a GM changed it
    (1008, "Logged Bracer", {}),                                                                # a tool log named in gm_audit
    (1009, "Moved Tunic", {}),                                                                  # armor_class_fix_log
    (1010, "Valued Boots", {"dex": 3}),                                                         # item_value_fill_log only: filled
    (1011, "Enstatite Breastplate", {}),                                                        # a gem's name
    (1012, "Fine Shield", {}),                                                                  # a gem's recipe product
    (1013, "Heirloom Blade", {}),
    (1014, "Bound Gloves", {}),                                                                 # itemowner
    (1015, "Boarded Robe", {}),                                                                 # jib's item-record entry, by id
    (1016, "Named Sash", {}),                                                                   # jib's other entry, by name
    (1017, "Counted Cloak", {}),                                                                # its id in a non-item entry: filled
    (714500, "Old Ancestor Ring", {}),                                                          # heirloom range
    (700123, "Quest Reward Cape", {}),                                                          # custom id
    (1018, "Other Shard Hat", {}),                                                              # changed on the other shard
    (1019, "Plain Blade", {"weapondamage": 0, "dex": 15}),                                      # weapon DMG filled
]
PRIMA = [
    (1, "Plain Greaves", {"str": 11, "sta": 10, "AC": 237, "maxhp": 10600, "levelreq": 49}, None),
    (2, "Busy Ring", {"str": 8, "HPMAX": 20, "cha": 4, "FR": -5}, None),
    (3, "Slid Ring", {"HPMAX": 100, "AC": 50}, None),
    (4, "Twin Name", {"wis": 5}, None),
    (5, "Noted Cap", {"wis": 5}, "unparsed equipment fields"),
    (6, "Audited Belt", {"AC": 9}, None),
    (7, "Logged Bracer", {"AC": 9}, None),
    (8, "Moved Tunic", {"AC": 9}, None),
    (9, "Valued Boots", {"dex": 3, "AC": 12}, None),
    (10, "Enstatite Breastplate", {"AC": 9}, None),
    (11, "Fine Shield", {"AC": 9}, None),
    (12, "Heirloom Blade", {"AC": 9}, None),
    (13, "Bound Gloves", {"AC": 9}, None),
    (14, "Boarded Robe", {"AC": 9}, None),
    (15, "Named Sash", {"AC": 9}, None),
    (16, "Counted Cloak", {"AC": 9}, None),
    (17, "Old Ancestor Ring", {"AC": 9}, None),
    (18, "Quest Reward Cape", {"AC": 9}, None),
    (19, "Other Shard Hat", {"AC": 9}, None),
    (20, "Plain Blade", {"weapondamage": 312, "dex": 15}, None),
    (21, "Nowhere Hat", {"AC": 9}, None),
]
EXTRA = [
    "INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey) VALUES "
    "('2026-11-09T01:00:00Z', 3, 'Roninwar', 'web', '', 'update', 'itempattern', '1007')",
    "CREATE TABLE hand_models_1114_log (patternid INTEGER PRIMARY KEY, model INTEGER, how TEXT, at TEXT)",
    "INSERT INTO hand_models_1114_log VALUES (1008, 5, 'icon', '')",
    "INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey) VALUES "
    "('2026-11-14T23:00:00Z', 7, 'dab', 'tool', '', 'update', 'itempattern', 'hand_models_1114_log')",
    "CREATE TABLE armor_class_fix_log (patternid INTEGER, itemname TEXT, kind TEXT, field TEXT, number INTEGER, ac_before INTEGER, applied TEXT)",
    "INSERT INTO armor_class_fix_log VALUES (1009, 'Moved Tunic', 'moved', 'str', 9, 0, '')",
    "CREATE TABLE item_value_fill_log (seq INTEGER PRIMARY KEY, patternid INTEGER, old INTEGER, new INTEGER, source TEXT, applied TEXT)",
    "INSERT INTO item_value_fill_log VALUES (1, 1010, 0, 50, 'rule', ''), (2, 1001, 0, 50, 'rule', '')",
    "INSERT INTO itempattern (patternid, itemname, equipslot) VALUES (701574, 'Enstatite', -1)",
    "CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, product INTEGER, ingredients TEXT)",
    "INSERT INTO craftrecipes VALUES (1, 1012, '[{\"item\": 701574, \"count\": 1}]')",
    "CREATE TABLE itemowner (patternid INTEGER PRIMARY KEY, serverid INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '')",
    "INSERT INTO itemowner VALUES (1014, 99, '')",
    "CREATE TABLE workboard (id INTEGER PRIMARY KEY, title TEXT, touches TEXT, details TEXT, notes TEXT, result TEXT, "
    "created_by TEXT, owner TEXT, updated_by TEXT)",
    "INSERT INTO workboard VALUES (950, 'For dab''s sessions: item records, class lists', 'itempattern', "
    "'HUM ELF (267): 5844, 1015 [AZ 1017]; on 2026-11-11 16:45', '', '', 'jibrish (oauth:Claude)', 'jibrish', '')",
    "INSERT INTO workboard VALUES (951, 'Loot for the bandits', '', 'the named sash drops at 5%; npcclass 1017', '', '', "
    "'jibrish (oauth:Claude)', 'jibrish', '')",
    "INSERT INTO workboard VALUES (952, 'Somebody else''s item records', '', 'Plain Greaves 1001', '', '', 'dab''s sessions', '', '')",
]


class PrimaFill(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        d = self.dir.name
        self.db, self.other, self.prima = (os.path.join(d, n) for n in ("shard.db", "other.db", "prima.sqlite"))
        make_shard(self.db, ITEMS, EXTRA)
        make_shard(self.other, [(5001, "Other Shard Hat", {}), (5002, "Plain Greaves", {})], [
            "INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after) VALUES "
            "('2026-11-12T08:00:00Z', 2, 'jibrish', 'oauth:Codex', '', 'update', 'itempattern', '{\"patternid\": 5001}', '{}', '{}')"])
        make_prima(self.prima, PRIMA)
        self.csv = os.path.join(d, "out.csv")

    def tearDown(self):
        self.dir.cleanup()

    def tool(self, *args, other=True):
        cmd = [sys.executable, "-E", "-s", TOOL, "--db", self.db, "--prima", self.prima, "--csv", self.csv, *args]
        if other:
            cmd += ["--other-db", self.other]
        return subprocess.run(cmd, capture_output=True, text=True)

    def item(self, pid):
        db = sqlite3.connect(self.db)
        db.row_factory = sqlite3.Row
        r = dict(db.execute("SELECT * FROM itempattern WHERE patternid = ?", (pid,)).fetchone())
        db.close()
        return r

    def digest(self):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--hash"], capture_output=True,
                              text=True).stdout.strip()

    def q(self, sql):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql).fetchall()
        finally:
            db.close()

    def changes(self):
        with open(self.csv) as f:
            return {(int(r["patternid"]), r["field"]): (r["before"], int(r["after"])) for r in csv.DictReader(f)}

    def excluded(self):
        with open(self.csv.replace(".csv", "-excluded.csv")) as f:
            return {int(r["patternid"]): r["reasons"] for r in csv.DictReader(f)}

    def test_report_fills_only_gaps(self):
        before = self.digest()
        r = self.tool()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.digest(), before, "report only wrote")
        self.assertEqual(self.changes(), {
            (1001, "sta"): ("0", 10), (1001, "AC"): ("NULL", 237),
            (1002, "cha"): ("NULL", 4),
            (1010, "AC"): ("NULL", 12),
            (1017, "AC"): ("NULL", 9),
            (1019, "weapondamage"): ("0", 312),
        })
        ex = self.excluded()
        self.assertIn("stats out of place", ex[1003])
        self.assertIn("gm_audit Roninwar (web)", ex[1007])
        self.assertIn("gm_audit dab (tool)", ex[1008])
        self.assertIn("armor_class_fix_log", ex[1009])
        self.assertIn("crafting gem by name", ex[1011])
        self.assertIn("crafting gem's product", ex[1012])
        self.assertIn("heirloom by name", ex[1013])
        self.assertIn("bound in itemowner", ex[1014])
        self.assertIn("jib board #950 (id)", ex[1015])
        self.assertIn("jib board #951 (name)", ex[1016])
        self.assertIn("heirloom id range", ex[714500])
        self.assertIn("custom id", ex[700123])
        self.assertIn("other shard: gm_audit jibrish (oauth:Codex)", ex[1018])
        for pid in (1004, 1005, 1006):  # ambiguous, noted: not even candidates
            self.assertNotIn(pid, ex)
        self.assertNotIn(1001, ex)  # a value fill and another author's board entry do not exclude

    def test_without_other_shard(self):
        self.tool(other=False)
        self.assertIn((1018, "AC"), self.changes())

    def test_apply_twice_undo_apply(self):
        start = self.digest()
        r = self.tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        g = self.item(1001)
        self.assertEqual((g["str"], g["sta"], g["AC"], g["maxhp"], g["levelreq"]), (11, 10, 237, 0, 30))
        self.assertEqual(self.item(1002)["str"], 5)
        self.assertEqual(self.item(1002)["FR"], None)  # Prima's negative is not a gap to fill
        self.assertEqual(self.item(1003)["HPMAX"], None)
        self.assertEqual(self.item(1019)["weapondamage"], 312)
        n = self.q("SELECT count(*) FROM gm_audit WHERE tbl = 'itempattern' AND detail LIKE '%prima_fill_gaps%'")[0][0]
        self.assertEqual(n, 5)
        b, a = self.q("SELECT before, after FROM gm_audit WHERE rowkey = '{\"patternid\":1001}'")[0]
        self.assertEqual((json.loads(b)["sta"], json.loads(a)["sta"]), (0, 10))
        applied = self.digest()

        r = self.tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(self.digest(), applied)

        r = self.tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.digest(), start)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'prima_fill_20261116_log'")[0][0], 0)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit WHERE detail LIKE '%prima_fill_gaps%' AND reverted_by IS NOT NULL")[0][0], 5)

        r = self.tool("--apply")  # its own audit lines do not make its records "changed"
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(self.digest(), applied)

    def test_undo_leaves_a_record_changed_since(self):
        self.tool("--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itempattern SET AC = 240 WHERE patternid = 1001")
        db.commit()
        db.close()
        r = self.tool("--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn('{"patternid":1001}: changed since', r.stdout)
        self.assertEqual(self.item(1001)["AC"], 240)
        self.assertEqual(self.item(1002)["cha"], None)
        self.assertEqual(self.q("SELECT count(*) FROM prima_fill_20261116_log")[0][0], 1)


if __name__ == "__main__":
    unittest.main()
