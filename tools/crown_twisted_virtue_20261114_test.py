#!/usr/bin/env python3
"""crown_twisted_virtue_20261114.py on a small database: the report writes nothing; --apply makes the crown (a copy of
the Heirloom Helm: model 0, its own name, icon and colour, every other column the helm's), its heirlooms row, its
itemaura circlet and one in Dust's pack, his own circlet row kept; a second apply finds nothing to do; --finish is
refused until he wears it, then deletes his circlet row and shows his helm slot; --undo puts back exactly what was there.
The controls: no Dust, nothing (exit 0); no Heirloom Helm, refused, nothing written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "crown_twisted_virtue_20261114.py")


def make(path, dust=True, helm=True):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, itemicon INTEGER, equipslot INTEGER,
            model INTEGER, color INTEGER, trade INTEGER, maxhp INTEGER);
        CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY, class INTEGER NOT NULL, role TEXT NOT NULL, epic INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
            source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
            remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL,
            location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', PRIMARY KEY (charid, merc, slot));
        CREATE TABLE charhiddenlooks (serverid INTEGER PRIMARY KEY, locs INTEGER NOT NULL);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO itempattern VALUES (714386, 'Heirloom Shield of Lightning Bolts', '', 1, 13, 1, 1, 1, 1);
        INSERT INTO charinventory VALUES (1809, 197, 1, 9200, 0, 714043, 0, 0, 0);
        INSERT INTO charaura VALUES (197, 0, 5, 3759675647, 8, 495410893, 72, 'n');
        INSERT INTO charhiddenlooks VALUES (197, 1);
    """)
    if dust:
        db.execute("INSERT INTO characters VALUES ('Dust', 197)")
    if helm:
        db.executescript("INSERT INTO itempattern VALUES (714043, 'Heirloom Helm', 'h', -1651410546, 1, 4, 4294967295, 1, 9200);"
                         "INSERT INTO heirlooms VALUES (714043, 3, 'head', 0);")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def state(path):
    db = sqlite3.connect(path)
    try:
        return tuple(db.execute(q).fetchall() for q in (
            "SELECT * FROM itempattern ORDER BY patternid", "SELECT * FROM heirlooms ORDER BY patternid",
            "SELECT * FROM itemaura ORDER BY patternid", "SELECT * FROM charinventory ORDER BY itemid",
            "SELECT * FROM charaura", "SELECT * FROM charhiddenlooks", "SELECT name FROM sqlite_master ORDER BY name"))
    finally:
        db.close()


class CrownOfTwistedVirtue(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_finish_undo(self):
        make(self.path)
        before = state(self.path)
        self.assertIn("report only: 4 rows", run(self.path).stdout)
        self.assertEqual(state(self.path), before)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        items, heir, aura, inv, own, hidden, _ = state(self.path)
        self.assertIn((714387, "Heirloom Crown of Twisted Virtue", items[-1][2], 1396793324, 1, 0, 3238989567, 1, 9200), items)
        self.assertIn((714387, 3, "head", 0), heir)
        self.assertEqual(aura[-1][:5], (714387, 0xE01818FF, 8, 0x1D875ECD, 72))
        self.assertEqual([r[5:] for r in inv if r[5] == 714387], [(714387, -1, 0, 1)], "in his pack, the next list number")
        self.assertEqual(own, before[4], "his own circlet kept")
        self.assertIn("nothing to do", run(self.path, "--apply").stdout)
        r = run(self.path, "--finish")
        self.assertEqual(r.returncode, 1)
        self.assertIn("does not wear the crown", r.stdout)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charinventory SET equiploc = -1 WHERE itemid = 1809")
        db.execute("UPDATE charinventory SET equiploc = 0 WHERE patternid = 714387")
        db.commit()
        db.close()
        self.assertEqual(run(self.path, "--finish").returncode, 0)
        _, _, _, _, own, hidden, _ = state(self.path)
        self.assertEqual((own, hidden), ([], [(197, 0)]))
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charinventory SET equiploc = -1 WHERE patternid = 714387")
        db.execute("UPDATE charinventory SET equiploc = 0 WHERE itemid = 1809")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path), before, "exactly what was there")

    def test_no_dust_and_refused(self):
        make(self.path, dust=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual((r.returncode, "no Dust on this shard" in r.stdout), (0, True))
        self.assertEqual(state(self.path), before)
        os.remove(self.path)
        make(self.path, helm=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
