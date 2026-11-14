#!/usr/bin/env python3
"""zerkziee_glow_20261114.py on a small database: the report writes nothing; --apply writes both hands' rows (kind 7,
the staff's model), his own copy of Heroic Ykesha in the Triad Scimitar's look (every other column as the record's),
moves his row and his gear set onto it, each with a gm_audit line; another character's Heroic Ykesha keeps the record;
a second apply finds nothing to do; --undo puts back exactly what was there. The controls: no Heroic Ykesha, the glow
alone; two Zerkziees, or no staff record, refuse the whole, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "zerkziee_glow_20261114.py")
STAFF, TRIAD = 0xCBF4135A, 0xD03942DE


def signed(v):
    return v - (1 << 32) if v >= 1 << 31 else v


def make(path, twins=False, staff=True, scimitar=True):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, accountid INTEGER NOT NULL, serverid INTEGER PRIMARY KEY, level INTEGER, tclass INTEGER);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, itemicon INTEGER, model INTEGER, weapondamage INTEGER);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER);
        CREATE TABLE gearsets (serverid INTEGER NOT NULL, slot INTEGER NOT NULL, name TEXT NOT NULL, mainitem INTEGER NOT NULL,
            mainpattern INTEGER NOT NULL, offitem INTEGER NOT NULL, offpattern INTEGER NOT NULL, PRIMARY KEY (serverid, slot));
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', PRIMARY KEY (charid, merc, slot));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO characters VALUES ('Zerkziee', 220, 747, 60, 1), ('Zerkzi', 220, 746, 60, 4);
        INSERT INTO itempattern VALUES (996023, 'Heroic Ykesha', 'Procs 160 damage.', 445518418, 1205436896, 740),
            (980081, 'Ascended Axe of Nature''s Innocence', '', 1, 291924585, 700), (714384, 'Heirloom Shield', '', 1, 1, 0);
        INSERT INTO charinventory VALUES (24200, 747, 980081, 15, 0), (99, 746, 996023, 14, 0);
        INSERT INTO gearsets VALUES (747, 1, 'sword', 24420, 996023, 24200, 980081), (747, 0, 'bow', 26598, 995118, -1, -1);
    """)
    db.execute("INSERT INTO itempattern VALUES (990328, 'Triad Scimitar', '', 2051918763, ?, 300)", (signed(TRIAD),))
    if staff:
        db.execute("INSERT INTO itempattern VALUES (701008, 'Oak Staff of Eilerawdl', '', 1, ?, 300)", (signed(STAFF),))
    if scimitar:
        db.execute("INSERT INTO charinventory VALUES (24420, 747, 996023, 14, 0)")
    if twins:
        db.execute("INSERT INTO characters VALUES ('Zerkziee', 1, 800, 1, 1)")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        out = tuple(db.execute(q).fetchall() for q in (
            "SELECT * FROM charaura ORDER BY charid, slot", "SELECT * FROM itempattern ORDER BY patternid",
            "SELECT * FROM charinventory ORDER BY itemid", "SELECT * FROM gearsets ORDER BY serverid, slot",
            "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"))
        return out, db.execute("SELECT action, tbl FROM gm_audit ORDER BY id").fetchall()
    finally:
        db.close()


class ZerkzieeGlow(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_and_undo(self):
        make(self.path)
        before = state(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 5 rows", r.stdout)
        self.assertEqual(state(self.path), before, "the report writes nothing")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        (aura, items, inv, sets, _), audits = state(self.path)
        self.assertEqual([a[:7] for a in aura], [(747, 0, 9, 0, 7, STAFF, 0), (747, 0, 10, 0, 7, STAFF, 0)])
        copy = [i for i in items if i[0] == 714385]
        self.assertEqual(copy, [(714385, "Heroic Ykesha", "Procs 160 damage.", 2051918763, signed(TRIAD), 740)], "the record's all but its look")
        self.assertIn((24420, 747, 714385, 14, 0), inv)
        self.assertIn((99, 746, 996023, 14, 0), inv, "another character's Heroic Ykesha keeps the record")
        self.assertIn((747, 1, "sword", 24420, 714385, 24200, 980081), sets)
        self.assertIn((747, 0, "bow", 26598, 995118, -1, -1), sets)
        self.assertEqual(audits, [("insert", "charaura"), ("insert", "charaura"), ("insert", "itempattern"),
                                  ("update", "charinventory"), ("update", "gearsets")])
        r = run(self.path, "--apply")
        self.assertIn("nothing to do", r.stdout, "a second apply")
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path)[0], before[0], "exactly what was there")

    def test_no_scimitar_the_glow_alone(self):
        make(self.path, scimitar=False)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("no Heroic Ykesha here", r.stdout)
        (aura, items, _, _, _), _ = state(self.path)
        self.assertEqual(len(aura), 2)
        self.assertFalse([i for i in items if i[0] == 714385])

    def test_refused(self):
        for kw in ({"twins": True}, {"staff": False}):
            make(self.path, **kw)
            before = state(self.path)
            r = run(self.path, "--apply")
            self.assertEqual(r.returncode, 1, kw)
            self.assertIn("REFUSED", r.stdout)
            self.assertEqual(state(self.path), before, "nothing written")
            os.remove(self.path)


if __name__ == "__main__":
    unittest.main()
