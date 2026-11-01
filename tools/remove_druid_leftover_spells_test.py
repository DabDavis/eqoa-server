"""tools/remove_druid_leftover_spells.py on a database of its own: the four
rows out of the druid's list and the books, the other spells and the records
left, a second run nothing, --undo the tables as they were; a spell whose
record is another refused (the control)."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "remove_druid_leftover_spells.py")
SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE characters (charName TEXT UNIQUE NOT NULL, serverid INTEGER PRIMARY KEY AUTOINCREMENT, level INTEGER, tclass INTEGER);
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL, addedorder INTEGER NOT NULL,
    onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL, showhide INTEGER NOT NULL);
"""


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT rowid, * FROM {t} ORDER BY rowid").fetchall()
           for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")}
    out["sqlite_sequence"] = db.execute("SELECT * FROM sqlite_sequence ORDER BY name").fetchall()
    db.close()
    return out


class RemoveDruidLeftovers(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        db = sqlite3.connect(self.db)
        db.executescript(SCHEMA)
        db.executemany("INSERT INTO spellpattern VALUES (?, ?)", [(701064, "Nature Form"), (701258, "Wild Nature"),
                       (701107, "Bixie Form"), (700670, "Sacred Spring"), (900418, "Spirit Form"), (452, "Spirit of the Wolf")])
        db.executemany("INSERT INTO classspells VALUES (7, ?, ?)", [(701064, "eqoa-data cm"), (701258, "eqoa-data master Wilding"),
                       (701107, "eqoa-data cm"), (700670, "eqoa-data"), (900418, "cm (eqoaemu)"), (452, "dump")])
        db.execute("INSERT INTO classspells VALUES (8, 452, 'dump')")
        db.executemany("INSERT INTO characters (charName, level, tclass) VALUES (?, ?, 7)", [("Oakleaf", 60), ("Fern", 52)])
        db.executemany("INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?, ?, ?, ?, ?, 0, 0)",
                       [(1, 452, 0, 1, 2, ), (1, 701258, 1, 1, 3), (1, 700670, 2, 0, 0), (2, 701064, 0, 0, 0), (2, 900418, 1, 0, 0)])
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        return r.stdout

    def test_apply_again_and_undo(self):
        before = dump(self.db)
        out = self.run_tool("--apply")
        self.assertIn("removed 7 rows", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT spellid FROM classspells WHERE class = 7 ORDER BY spellid").fetchall(), [(452,), (900418,)])
        self.assertEqual(db.execute("SELECT serverid, spellid FROM spells ORDER BY id").fetchall(), [(1, 452), (2, 900418)])
        self.assertEqual(db.execute("SELECT count(*) FROM spellpattern").fetchone()[0], 6, "the records stay")
        self.assertEqual(db.execute("SELECT count(*) FROM classspells WHERE class = 8").fetchone()[0], 1)
        db.close()
        self.assertIn("removed 0 rows", self.run_tool("--apply"))
        self.assertIn("put back 7 rows, 0 left", self.run_tool("--undo"))
        self.assertEqual(dump(self.db), before)

    def test_a_record_not_the_one_named_is_refused(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE spellpattern SET spellname = 'Bixie Wings' WHERE spellid = 701107")
        db.commit()
        db.close()
        out = self.run_tool("--apply")
        self.assertIn("refused: 701107", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM classspells WHERE spellid = 701107").fetchone()[0], 1)
        db.close()


if __name__ == "__main__":
    unittest.main()
