#!/usr/bin/env python3
"""l17_class_rewards_20261121.py: apply, a second apply, undo, a cell changed since left alone, refusals."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import l17_class_rewards_20261121 as tool  # noqa: E402

ITEMS = [  # patternid, name, levelreq, classuse
    (8115, "Soldier's Pendant", 17, 1), (6697, "Soldier's Tunic", 17, 1), (998119, "Soldier's Tunic", 60, 1),
    (6572, "Chestguard of Malice", 17, 8), (6715, "Tunic of Malice", 17, 8), (990007, "Tunic of Malice", 17, 8),
    (8117, "Spiritrune Amulet", 17, 256), (6701, "Spiritrune Tunic", 17, 256),
    (8028, "Amulet of Radiance", 17, 512), (6717, "Tunic of Radiance", 17, 512),
    (7594, "Slippers of Brilliance", 17, 4096), (5293, "Wand of Brilliance", 17, 4096),
    (6718, "Tunic of the Disciple", 17, 16), (6365, "Helm of the Disciple", 17, 16),
    (7119, "Sash of Insight", 17, 32767), (6899, "Gloves of Insight", 17, 8192),
    (8100, "Pendant of the Shadow", 17, 2048), (6716, "Tunic Of Nature's Ward", 17, 2),
]
P2 = {"0": {"items": "8115"}, "3": {"items": "6572"}, "8": {"items": "8117"}, "9": {"items": "8028"},
      "11": {"items": "8100"}, "12": {"items": "7594"}}
MOR = {"1": {"items": "6716"}, "4": {"items": "6718"}, "13": {"items": "7119"}}


def make(path):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, source TEXT)")
    db.execute("CREATE TABLE queststeps (questid INTEGER, step INTEGER, byclass TEXT NOT NULL DEFAULT '', "
               "PRIMARY KEY (questid, step))")
    db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, levelreq INTEGER, classuse INTEGER)")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
               "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)")
    db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?)", ITEMS)
    db.executemany("INSERT INTO quests VALUES (?, ?, ?)", [
        (100752, "Grave Silk", "EQOA-Data 17/highpass/p1 (tools/import_quests.py)"),
        (100753, "Courier Svex", "EQOA-Data 17/highpass/p2 (tools/import_quests.py)"),
        (100755, "Murien's Brew", "EQOA-Data 17/moradhim/main (tools/import_quests.py)")])
    db.executemany("INSERT INTO queststeps VALUES (?, ?, ?)", [
        (100752, 2, json.dumps({"3": {"items": "7323"}}, separators=(",", ":"))), (100753, 1, ""),
        (100753, 4, json.dumps(P2, sort_keys=True, separators=(",", ":"))),
        (100755, 4, json.dumps(MOR, sort_keys=True, separators=(",", ":")))])
    db.commit()
    db.close()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def cells(self):
        return {(q, s): {k: v["items"] for k, v in json.loads(b).items()}
                for q, s, b in self.q("SELECT questid, step, byclass FROM queststeps WHERE byclass <> ''")}

    def test_apply_again_undo(self):
        before = self.q("SELECT questid, step, byclass FROM queststeps ORDER BY questid, step")
        self.assertEqual(tool.main(["--db", self.path]), 0)
        self.assertEqual(self.q("SELECT questid, step, byclass FROM queststeps ORDER BY questid, step"), before)
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 0)
        c = self.cells()
        self.assertEqual(c[(100753, 4)], {"0": "6697", "3": "6715", "8": "6701", "9": "6717", "11": "8100", "12": "5293"})
        self.assertEqual(c[(100755, 4)], {"1": "6716", "4": "6365", "13": "6899"})
        self.assertEqual(c[(100752, 2)], {"3": "7323"})
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 2)
        self.assertEqual(self.q("SELECT count(*) FROM l17_class_rewards_20261121_log")[0][0], 7)
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 0)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 2)
        tool.main(["--db", self.path, "--undo"])
        self.assertEqual(self.q("SELECT questid, step, byclass FROM queststeps ORDER BY questid, step"), before)
        self.assertEqual(self.q("SELECT count(*) FROM l17_class_rewards_20261121_log")[0][0], 0)

    def test_undo_leaves_a_changed_cell(self):
        tool.main(["--db", self.path, "--apply"])
        bc = json.loads(self.q("SELECT byclass FROM queststeps WHERE questid = 100755")[0][0])
        bc["13"]["items"] = "7313"
        db = sqlite3.connect(self.path)
        db.execute("UPDATE queststeps SET byclass = ? WHERE questid = 100755", (json.dumps(bc),))
        db.commit()
        db.close()
        tool.main(["--db", self.path, "--undo"])
        c = self.cells()
        self.assertEqual(c[(100755, 4)], {"1": "6716", "4": "6718", "13": "7313"})
        self.assertEqual(c[(100753, 4)]["3"], "6572")

    def test_refuses_an_unexpected_cell(self):
        bc = dict(P2, **{"3": {"items": "8100"}})
        db = sqlite3.connect(self.path)
        db.execute("UPDATE queststeps SET byclass = ? WHERE questid = 100753 AND step = 4", (json.dumps(bc),))
        db.commit()
        db.close()
        before = self.q("SELECT questid, step, byclass FROM queststeps ORDER BY questid, step")
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 2)
        self.assertEqual(self.q("SELECT questid, step, byclass FROM queststeps ORDER BY questid, step"), before)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 0)

    def test_refuses_a_missing_record(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM itempattern WHERE itemname = 'Helm of the Disciple'")
        db.commit()
        db.close()
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 2)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 0)


if __name__ == "__main__":
    unittest.main()
