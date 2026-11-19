#!/usr/bin/env python3
"""dust_turban_20261116.py on a small database: the report writes nothing; --give is refused before --apply; --apply
makes the turban (a copy of the Heirloom Helm: lore, model 1, its own name, icon and colour, every other column the
helm's), its heirlooms row, its itemaura hat (kind 10 of the efreeti lord) and its itemowner row (a GM); a second
--apply finds nothing to do; --give puts one in a GM's pack, a second finds he holds it; --undo puts back exactly what
was there, all or nothing: refused while he wears it. The controls: no such character, nothing (exit 0); no Heirloom Helm, refused,
nothing written; the free id's itemowner already someone's, refused."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "dust_turban_20261116.py")


def make(path, dust=True, helm=True):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, itemicon INTEGER, equipslot INTEGER,
            model INTEGER, color INTEGER, trade INTEGER, lore INTEGER, maxhp INTEGER);
        CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY, class INTEGER NOT NULL, role TEXT NOT NULL, epic INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
            source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
            color2 INTEGER NOT NULL DEFAULT 0, kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0,
            nodes2 INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE itemowner (patternid INTEGER PRIMARY KEY, serverid INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '');
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER NOT NULL,
            remainHP INTEGER NOT NULL, remaincharge INTEGER NOT NULL, patternid INTEGER NOT NULL, equiploc INTEGER NOT NULL,
            location INTEGER NOT NULL, listnumber INTEGER NOT NULL);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO itempattern VALUES (714387, 'Heirloom Crown of Twisted Virtue', 'c', 1396793324, 1, 0, 3238989567, 1, 0, 9200);
        INSERT INTO charinventory VALUES (27176, 197, 1, 9200, 0, 714387, 0, 0, 3);
    """)
    if dust:
        db.execute("INSERT INTO characters VALUES ('Dust', 197)")
    if helm:
        db.executescript("INSERT INTO itempattern VALUES (714043, 'Heirloom Helm', 'h', -1651410546, 1, 4, 4294967295, 1, 0, 9200);"
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
            "SELECT * FROM itemaura ORDER BY patternid", "SELECT * FROM itemowner ORDER BY patternid",
            "SELECT * FROM charinventory ORDER BY itemid", "SELECT name FROM sqlite_master ORDER BY name"))
    finally:
        db.close()


def q(path, sql):
    db = sqlite3.connect(path)
    try:
        return db.execute(sql).fetchall()
    finally:
        db.close()


class TurbanOfTwistedVirtue(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_give_undo(self):
        make(self.path)
        before = state(self.path)
        self.assertIn("report only: 4 rows", run(self.path).stdout)
        self.assertEqual(state(self.path), before)
        r = run(self.path, "--give")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("--apply first", r.stdout)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        rec = q(self.path, "SELECT patternid, itemname, itemicon, equipslot, model, color, trade, lore, maxhp FROM itempattern WHERE patternid = 714388")
        self.assertEqual(rec, [(714388, "Turban of Twisted Virtue", 864498561, 1, 1, 3238989567, 1, 1, 9200)])
        self.assertEqual(q(self.path, "SELECT * FROM heirlooms WHERE patternid = 714388"), [(714388, 3, "head", 0)])
        self.assertEqual(q(self.path, "SELECT color, kind, source, nodes FROM itemaura WHERE patternid = 714388"), [(0, 10, 0xAA489CBB, 0)])
        self.assertEqual(q(self.path, "SELECT serverid FROM itemowner WHERE patternid = 714388"), [(197,)])
        self.assertEqual(q(self.path, "SELECT count(*) FROM charinventory WHERE patternid = 714388"), [(0,)], "--apply gives nothing")
        self.assertIn("nothing to do", run(self.path, "--apply").stdout)
        r = run(self.path, "--give")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(q(self.path, "SELECT serverid, patternid, equiploc, location, listnumber, remainHP FROM charinventory WHERE patternid = 714388"),
                         [(197, 714388, -1, 0, 4, 9200)])
        self.assertIn("holds the turban already", run(self.path, "--give").stdout)
        self.assertEqual(q(self.path, "SELECT count(*) FROM gm_audit"), [(5,)])
        # worn: --undo leaves it (and the record it needs)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charinventory SET equiploc = 0 WHERE patternid = 714388")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("nothing undone", r.stdout)
        self.assertEqual(q(self.path, "SELECT count(*) FROM itemaura WHERE patternid = 714388"), [(1,)], "all or nothing")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charinventory SET equiploc = -1 WHERE patternid = 714388")
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(state(self.path)[:5], before[:5])
        self.assertNotIn(("dust_turban_1116_log",), state(self.path)[5])

    def test_no_dust_nothing(self):
        make(self.path, dust=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("no Dust", r.stdout)
        self.assertEqual(state(self.path), before)

    def test_no_helm_refused(self):
        make(self.path, helm=False)
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(state(self.path), before)

    def test_free_ids_owner_taken_refused(self):
        make(self.path)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO itemowner VALUES (714388, 327, 'someone')")
        db.commit()
        db.close()
        before = state(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertEqual(state(self.path), before)


if __name__ == "__main__":
    unittest.main()
