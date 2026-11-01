#!/usr/bin/env python3
"""tools/fix_broken_records.py against a made-up database and a made-up
EQOA-Data folder (no game data: the names, ids and figures are made up): a
broken record (a PoT or a resist past 500) takes a real record of ours of its
name (a plural s and an apostrophe aside, its wear slot too), else EQOA-Data's
of its name that is none of the template's (its figures, its class; a figure
it lacks its kin's, item HP the weapons' rule), else loses the template (a
weapon the median damage of its attack type's real ones at its level, armour
its slot's median AC); a held copy above the new item HP made whole at it; a
loot row of a stripped record taken out, one of a fixed record kept; through
a plan file, --apply in one transaction, a second writing nothing, --undo
exact, a row a GM changed since kept.

    python3 tools/fix_broken_records_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_broken_records as fb  # noqa: E402

COLS = ("patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL DEFAULT 0, itemicon INTEGER NOT NULL DEFAULT 0, "
        "equipslot INTEGER NOT NULL, trade INTEGER NOT NULL DEFAULT 0, rent INTEGER NOT NULL DEFAULT 1, "
        "attacktype INTEGER NOT NULL DEFAULT 0, weapondamage INTEGER NOT NULL DEFAULT 0, levelreq INTEGER NOT NULL, "
        "maxhp INTEGER NOT NULL DEFAULT 0, duration INTEGER NOT NULL DEFAULT 0, classuse INTEGER NOT NULL DEFAULT 32767, "
        "raceuse INTEGER NOT NULL DEFAULT 1023, lore INTEGER NOT NULL DEFAULT 0, itemname TEXT NOT NULL, "
        "itemdesc TEXT NOT NULL DEFAULT '', model INTEGER NOT NULL DEFAULT 0, " +
        ", ".join(f"{c} INTEGER" for c in fb.STATS))
JUNK = dict(str=13, sta=65, agi=65, wis=76, dex=56, cha=0, intelligence=112, HPMAX=0, POWMAX=12, PoT=987, HoT=0, AC=62,
            PR=23, DR=12, FR=0, CR=0, LR=927, AR=123)


def make(dir):
    path = os.path.join(dir, "t.db")
    db = sqlite3.connect(path, isolation_level=None)
    db.execute(f"CREATE TABLE itempattern ({COLS})")
    db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, "
               "patternid INTEGER NOT NULL, remainHP INTEGER NOT NULL)")
    db.execute("CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL, "
               "weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60, "
               "source TEXT NOT NULL DEFAULT 'gm')")
    db.execute("CREATE TABLE defaultcharinventory (itemid INTEGER PRIMARY KEY, patternid INTEGER NOT NULL)")

    def add(pid, name, slot, level, at=0, dmg=0, hp=0, du=0, **st):
        cols = ["patternid", "itemname", "equipslot", "levelreq", "attacktype", "weapondamage", "maxhp", "duration", *st]
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   [pid, name, slot, level, at, dmg, hp, du, *st.values()])
    # the real ones: a level 40 cloak of ours; level 50 slashing weapons and cloaks for the medians
    add(10, "Mistwoven Cloaks", 2, 40, hp=8000, du=50, agi=5, cha=10, AC=20)
    for i, (dmg, hp) in enumerate([(500, 9000), (520, 9000), (540, 9000), (610, 7000)]):
        add(20 + i, f"Test Blade {i}", 12, 50, at=1, dmg=dmg, hp=hp, du=50)
    for i, ac in enumerate([0, 40, 50, 90]):
        add(30 + i, f"Test Cloak {i}", 2, 50, hp=11000, du=50, AC=ac)
    # the broken: a cloak of a real one's name, a staff EQOA-Data knows, a blade and a cloak of no record
    add(100, "Mistwoven Cloak", 13, 50, hp=18500, du=75, **JUNK)
    add(101, "Staff of Embers", 15, 50, at=4, dmg=999, hp=18500, du=75, **JUNK)
    add(102, "Staff of Ash", 15, 50, at=4, dmg=999, hp=18500, du=75, **JUNK)
    add(103, "Test Sword of Nothing", 12, 50, at=1, dmg=999, hp=18500, du=75, **JUNK)
    add(104, "Test Cloak of Nothing", 2, 50, hp=18500, du=75, **JUNK)
    db.executemany("INSERT INTO charinventory (serverid, patternid, remainHP) VALUES (?, ?, ?)",
                   [(1, 103, 18500), (2, 103, 18500), (1, 100, 3000), (3, 20, 9000)])
    db.executemany("INSERT INTO loottables (tablename, patternid) VALUES (?, ?)", [("warlord", 103), ("warlord", 101), ("other", 20)])
    db.execute("INSERT INTO defaultcharinventory VALUES (1, 103)")
    data = os.path.join(dir, "data", "Information", "databases", "items")
    os.makedirs(data)
    with open(os.path.join(data, "weapons.json"), "w") as f:
        json.dump({"items": [
            # the template's copy under the name, never taken
            {"name": "Staff of Embers", "damage": 999, "itemHp": 18500, "stats": {"pot": 987, "lr": 927}},
            {"name": "Staff of embers", "requirements": {"level": 50, "class": ["DRU"]}, "attackType": "twoHandBlunt",
             "durability": 40, "damage": 800, "stats": {"wis": 30, "pow": 150, "dr": 20}, "noTrade": True, "lore": True},
            {"name": "Staff of Ash", "requirements": {"level": 50, "class": ["SHM", "CLR"]}, "attackType": "twoHandBlunt",
             "stats": {"str": -10, "cha": 25}, "lore": True}]}, f)
    return db, path, os.path.join(dir, "data")


def dump(db):
    return {n: db.execute(f"SELECT rowid, * FROM {n} ORDER BY rowid").fetchall()
            for (n,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name").fetchall()}


class FixBrokenRecords(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db, self.path, self.data = make(self.dir.name)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def rec(self, pid, *cols):
        return self.db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (pid,)).fetchone()

    def test_plan(self):
        p = fb.plan(self.db, self.data)
        by = {c["patternid"]: c for c in p["changes"]}
        self.assertEqual(sorted(by), [100, 101, 102, 103, 104], "the template's records alone")
        # ours of its name (a plural s aside), its wear slot too
        self.assertTrue(by[100]["how"].startswith("ours: 10 Mistwoven Cloaks"))
        self.assertEqual({k: by[100]["set"][k] for k in ("levelreq", "equipslot", "agi", "cha", "AC", "PoT", "LR", "maxhp", "duration")},
                         dict(levelreq=40, equipslot=2, agi=5, cha=10, AC=20, PoT=None, LR=None, maxhp=8000, duration=50),
                         "its kin's figures as its kin holds them (a stat it lacks NULL)")
        self.assertFalse(by[100]["strip"])
        # EQOA-Data's, the template's copy there passed over; its class, lore and no trade
        s = by[101]["set"]
        self.assertEqual((s["weapondamage"], s["wis"], s["POWMAX"], s["DR"], s["PoT"], s["LR"], s["classuse"], s["lore"],
                          s.get("trade", 0), s["duration"]), (800, 30, 150, 20, 0, 0, 1 << 7, 1, 1, 40))
        self.assertEqual(s["maxhp"], 9000, "item HP no record gives: its level's commonest among the real weapons")
        # a figure its record lacks: its kin's (the other record taken here of its level and attack type)
        s = by[102]["set"]
        self.assertEqual((s["weapondamage"], s["duration"], s["str"], s["cha"], s["classuse"]), (800, 40, -10, 25, (1 << 8) | (1 << 9)))
        self.assertIn("102 Staff of Ash: weapondamage its kin's 800; duration its kin's 40; item HP 9000 (the weapons' rule)",
                      p["lists"]["figures EQOA-Data's record lacks (ours)"])
        # no record anywhere: the template lost; the medians (an even count's middle two, a half up)
        s = by[103]["set"]
        self.assertEqual((s["weapondamage"], s["maxhp"], s["duration"], s["PoT"], s["str"], s["AC"]), (530, 9000, 50, 0, 0, 0))
        self.assertTrue(by[103]["strip"])
        s = by[104]["set"]
        self.assertEqual((s["AC"], s["maxhp"], s["PoT"], s["LR"]), (45, 11000, 0, 0))
        self.assertNotIn("equipslot", s, "its slot kept")
        self.assertEqual(p["lists"]["loot rows of a record with no real record (taken out)"], ["1 warlord: 103"])
        self.assertEqual(p["lists"]["loot rows of a fixed record (kept)"], ["2 warlord: 101"])
        self.assertEqual(p["lists"]["the old default inventory's rows (read by no code: left)"], ["103: 1"])

    def test_through_a_plan_again_and_undo(self):
        before = dump(self.db)
        planned = os.path.join(self.dir.name, "plan.json")
        with open(planned, "w") as f:
            json.dump(fb.plan(self.db, self.data), f)
        with open(planned) as f:
            p = json.load(f)
        recs, held, loot, stale = fb.apply(self.db, p, when="2026-11-01T21:00:00+00:00")
        self.assertEqual((recs, held, loot, stale), (5, 2, 1, 0))
        self.assertEqual(self.db.execute("SELECT remainHP FROM charinventory WHERE patternid = 103").fetchall(), [(9000,), (9000,)],
                         "made whole at the new item HP")
        self.assertEqual(self.db.execute("SELECT remainHP FROM charinventory WHERE patternid = 100").fetchone()[0], 3000,
                         "a copy under the new item HP left")
        self.assertEqual(self.db.execute("SELECT tablename, patternid FROM loottables ORDER BY id").fetchall(),
                         [("warlord", 101), ("other", 20)])
        once = dump(self.db)
        # a second run, by the plan or a fresh one, writes nothing
        self.assertEqual(fb.apply(self.db, p, when="2026-11-01T21:00:05+00:00"), (0, 0, 0, 0))
        self.assertEqual(fb.plan(self.db, self.data)["changes"], [], "no record of the template left")
        self.assertEqual(dump(self.db), once)
        back, left, run = fb.undo(self.db)
        self.assertEqual((back, left), (8, []))
        self.assertEqual(dump(self.db), before)

    def test_undo_keeps_a_gms_change(self):
        before = dump(self.db)
        fb.apply(self.db, fb.plan(self.db, self.data), when="2026-11-01T21:00:00+00:00")
        self.db.execute("UPDATE itempattern SET AC = 77 WHERE patternid = 104")
        back, left, _ = fb.undo(self.db)
        self.assertEqual(left, ['itempattern {"patternid":104}'])
        self.assertEqual(self.rec(104, "AC")[0], 77)
        self.assertFalse(fb.has_table(self.db, fb.LOG))
        # every other row as it was; that one as the run and the GM left it
        now = dump(self.db)
        self.assertEqual([r for r in now["itempattern"] if r[0] != 104], [r for r in before["itempattern"] if r[0] != 104])
        self.assertEqual({k: v for k, v in now.items() if k != "itempattern"}, {k: v for k, v in before.items() if k != "itempattern"})
        # a plan made before a GM changed a record leaves that record
        p = fb.plan(self.db, self.data)
        self.db.execute("UPDATE itempattern SET PoT = 900 WHERE patternid = 103")
        recs, _, loot, stale = fb.apply(self.db, p, when="2026-11-01T22:00:00+00:00")
        self.assertEqual((recs, stale), (3, 1), "104 is no record of the template now; 103 changed since the plan")
        self.assertEqual(self.rec(103, "PoT")[0], 900)


if __name__ == "__main__":
    unittest.main()
