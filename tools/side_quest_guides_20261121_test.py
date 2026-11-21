#!/usr/bin/env python3
"""side_quest_guides_20261121.py: the four rewrites, a second apply, undo exact, the refusals."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import side_quest_guides_20261121 as tool  # noqa: E402

COLS = ("questid", "step", "kind", "target", "npcid", "items", "tunar", "count", "world", "x", "y", "z", "radius",
        "journal", "answer", "says", "waiting", "notyet", "reminder", "xp", "rewardtunar", "rewarditems", "factions",
        "spells", "events", "chance", "byclass", "choices", "height", "anyorder")
ITEMS = {700741: "Tribal Pendant", 700742: "Ice Key", 700743: "Remains of Manger MacGray", 700758: "Tobul's Tea Recipe",
         700759: "Tinsy Tea Leaves", 700760: "Brackish Rocks", 700775: "Phoborium's Horn", 700776: "Caelduiel's Charm",
         700777: "Lelaia's Seal", 700778: "Deiphobus's Skull", 700785: "Dumak's Paper", 700786: "Fender's Stone",
         6357: "Goldlight Coif"}
BYCLASS = '{"9":{"items":"7529"},"2":{"items":"7879"}}'


def st(qid, n, kind, target, **kw):
    r = dict.fromkeys(COLS, "")
    r.update(questid=qid, step=n, kind=kind, target=target, npcid=0, tunar=0, count=1, world=0, x=0.0, y=0.0, z=0.0,
             radius=15.0, xp=0, rewardtunar=0, chance=0.0, height=0.0, anyorder=0, journal=f"j{qid}.{n}", says=f"s{qid}.{n}")
    r.update(kw)
    return r


QUESTS = [
    (100761, "The Remains of Manger MacGray", "Feria Hurkert", "EQOA-Data 23-47/23/p3 (tools/import_quests.py)", [
        st(100761, 1, "give", "Guard Smitty", tunar=1200),
        st(100761, 2, "talk", "Bison"),
        st(100761, 3, "talk", "Feria Hurkert"),
        st(100761, 4, "talk", "ice statue", events='[{"on":"done","do":"give","items":"700742"}]', choices='{"ask":"x"}'),
        st(100761, 5, "give", "frosty chest", items="700742",
           events='[{"on":"done","do":"popup","text":"Inside lie the remains."},{"on":"done","do":"give","items":"700743"}]'),
        st(100761, 6, "give", "Feria Hurkert", items="700743", xp=600000, byclass=BYCLASS),
    ]),
    (100772, "Yismay's Tea", "Teamaster Yismay", "EQOA-Data 23-47/33/teagarden (tools/import_quests.py)", [
        st(100772, 1, "talk", "Killian Helnes"),
        st(100772, 2, "loot", "Tobul", items="700758", chance=1.0),
        st(100772, 3, "give", "Teamaster Yismay", items="700758", says="Captain Hogofa can get the leaves. Take him 1,625 tunar."),
        st(100772, 4, "give", "Captain Hogofa", tunar=1625),
        st(100772, 5, "talk", "Captain Skorn", events='[{"on":"done","do":"give","items":"700759"}]'),
        st(100772, 6, "loot", "Deathfist rider", items="700760", chance=0.3),
    ]),
    (100776, "The Seal of Deiphobus", "Ambrose Castor", "EQOA-Data 23-47/35/ambrose (tools/import_quests.py)", [
        st(100776, 1, "loot", "Phoborium", items="700775", chance=1.0),
        st(100776, 2, "give", "Ambrose Castor", items="700775"),
        st(100776, 3, "talk", "Iulias Maphesti"),
        st(100776, 4, "loot", "Caelduiel the Sacred", items="700776", chance=1.0),
        st(100776, 5, "give", "Lelaia Shavrell", tunar=2325, events='[{"on":"done","do":"give","items":"700777"}]'),
        st(100776, 6, "give", "Iulias Maphesti", items="700776"),
        st(100776, 7, "talk", "Ambrose Castor"),
        st(100776, 8, "reach", ""),
        st(100776, 9, "loot", "Deiphobus", items="700778", chance=1.0),
        st(100776, 10, "give", "Ambrose Castor", items="700778", xp=3768632),
    ]),
    (100779, "Fender's Stone", "Fender Crow", "EQOA-Data 23-47/35/fender (tools/import_quests.py)", [
        st(100779, 1, "loot", "Dumak Olivarn", items="700785", chance=1.0),
        st(100779, 2, "give", "Fender Crow", items="700785"),
        st(100779, 3, "loot", "Grave Robber Laen", items="700786", chance=1.0),
        st(100779, 4, "give", "Fender Crow", items="700786"),
        st(100779, 5, "give", "Smithy Palin", tunar=2250, xp=3768632),
        st(100779, 6, "talk", "Tailor Portia"),
    ]),
]


def make(path):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, source TEXT, "
               "notes TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '')")
    db.execute(f"CREATE TABLE queststeps ({', '.join(COLS)}, PRIMARY KEY (questid, step))")
    db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT)")
    db.execute("CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, count INTEGER DEFAULT 0, "
               "done INTEGER DEFAULT 0, PRIMARY KEY (serverid, questid))")
    db.execute("CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT)")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
               "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)")
    db.executemany("INSERT INTO itempattern VALUES (?, ?)", ITEMS.items())
    for qid, name, giver, source, steps in QUESTS:
        db.execute("INSERT INTO quests VALUES (?, ?, ?, ?, 'From EQOA-Data.', '')", (qid, name, giver, source))
        for s in steps:
            db.execute(f"INSERT INTO queststeps ({', '.join(COLS)}) VALUES ({', '.join('?' for _ in COLS)})",
                       [s[c] for c in COLS])
    for qid, name, req in ((100790, "Ore for the Forge", ""), (100791, "Margun Leafstrider", "100790"),
                           (100792, "The Thulite Anvil", "100790")):
        db.execute("INSERT INTO quests VALUES (?, ?, 'Grimm Brellhand', ?, '', ?)",
                   (qid, name, f"EQOA-Data 23-47/47/p{qid - 100789} (tools/import_quests.py)", req))
        db.execute("INSERT INTO queststeps (questid, step, kind, target) VALUES (?, 1, 'talk', 'Grimm Brellhand')", (qid,))
    db.execute("INSERT INTO charquests VALUES (1, 100761, 0, 0, 1)")  # finished: no matter
    db.execute("INSERT INTO charquests VALUES (2, 100779, 2, 0, 0)")  # before the change: no matter
    db.commit()
    db.close()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def exec(self, sql, args=()):
        db = sqlite3.connect(self.path)
        db.execute(sql, args)
        db.commit()
        db.close()

    def snapshot(self):
        return (self.q("SELECT * FROM queststeps ORDER BY questid, step"),
                self.q("SELECT questid, notes, requires FROM quests"))

    def steps(self, qid):
        return self.q("SELECT step, kind, target, items, tunar, xp, byclass, events, says FROM queststeps "
                      "WHERE questid = ? ORDER BY step", (qid,))

    def test_dry_run_writes_nothing(self):
        before = self.snapshot()
        self.assertEqual(tool.main(["--db", self.path]), 0)
        self.assertEqual(self.snapshot(), before)

    def test_apply_again_undo(self):
        before = self.snapshot()
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 0)
        m = self.steps(100761)
        self.assertEqual(len(m), 5)
        self.assertEqual(m[4][:6], (5, "give", "frosty chest", "700742", 0, 600000))
        self.assertEqual(m[4][6], BYCLASS)
        self.assertEqual(json.loads(m[4][7]), [{"on": "done", "do": "popup", "text": "Inside lie the remains."}])
        y = self.steps(100772)
        self.assertEqual(y[3][1:5], ("talk", "Captain Hogofa", "", 0))
        self.assertEqual(y[4][1:5], ("give", "Captain Skorn", "", 1625))
        self.assertIn("Take 1,625 tunar with you.", y[2][8])
        d = self.steps(100776)
        self.assertEqual([(s[1], s[2], s[3], s[4]) for s in d[3:8]], [
            ("loot", "Caelduiel the Sacred", "700776", 0), ("give", "Iulias Maphesti", "700776", 0),
            ("give", "Lelaia Shavrell", "", 2325), ("show", "Iulias Maphesti", "700777", 0),
            ("talk", "Ambrose Castor", "", 0)])
        self.assertEqual(len(d), 11)
        self.assertEqual(d[10][1:6], ("give", "Ambrose Castor", "700778", 0, 3768632))
        f = self.steps(100779)
        self.assertEqual((f[4][5], f[5][5]), (0, 3768632))
        self.assertEqual(f[4][4], 2250)
        self.assertEqual(self.q("SELECT requires FROM quests WHERE questid = 100792"), [("100790, 100791",)])
        self.assertEqual(self.q("SELECT requires FROM quests WHERE questid = 100791"), [("100790",)])
        notes = dict(self.q("SELECT questid, notes FROM quests WHERE questid NOT IN (100790, 100791)"))
        self.assertTrue(all("side_quest_guides_20261121" in n for n in notes.values()))
        audits = self.q("SELECT count(*) FROM gm_audit")[0][0]
        self.assertEqual(audits, 9)
        after = self.snapshot()
        tool.main(["--db", self.path, "--apply"])  # nothing to do
        self.assertEqual(self.snapshot(), after)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], audits)
        tool.main(["--db", self.path, "--undo"])
        self.assertEqual(self.snapshot(), before)
        tool.main(["--db", self.path, "--apply"])  # and again after undo
        self.assertEqual(self.snapshot(), after)

    def test_refuses_a_character_on_a_changing_step(self):
        self.exec("INSERT INTO charquests VALUES (3, 100761, 6, 0, 0)")
        before = self.snapshot()
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 2)
        self.assertEqual(self.snapshot(), before)

    def test_refuses_a_step_not_as_written(self):
        self.exec("UPDATE queststeps SET tunar = 1500 WHERE questid = 100772 AND step = 4")
        before = self.snapshot()
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 2)
        self.assertEqual(self.snapshot(), before)

    def test_refuses_a_steptarget_on_a_changing_step(self):
        self.exec("INSERT INTO queststeptargets VALUES (100776, 9, 'Deiphobus the Lesser')")
        self.assertEqual(tool.main(["--db", self.path, "--apply"]), 2)

    def test_undo_leaves_a_quest_changed_since(self):
        tool.main(["--db", self.path, "--apply"])
        self.exec("UPDATE queststeps SET says = 'GM words' WHERE questid = 100779 AND step = 6")
        tool.main(["--db", self.path, "--undo"])
        self.assertEqual(self.steps(100779)[5][8], "GM words")
        self.assertEqual(self.steps(100779)[5][5], 3768632)
        self.assertEqual(len(self.steps(100761)), 6)  # the others put back
        self.assertEqual(self.q(f"SELECT questid FROM {tool.LOG}"), [(100779,)])


if __name__ == "__main__":
    unittest.main()
