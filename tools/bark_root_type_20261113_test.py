#!/usr/bin/env python3
"""bark_root_type_20261113.py on a small database: the report writes nothing; --apply gives the 23 seeded barks and
roots the pristine kobold ear's type 0 and masks (nothing else of them); a second apply does nothing; --undo puts
every table back exactly. The controls: a bark missing, or another trophy of a usable type, is refused, writing
nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "bark_root_type_20261113.py")
sys.path.insert(0, HERE)
import bark_root_type_20261113 as br  # noqa: E402

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, itemcost INTEGER,
    itemicon INTEGER, equipslot INTEGER, craft INTEGER, maxstack INTEGER, procanim INTEGER, classuse INTEGER,
    raceuse INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, stackleft INTEGER,
    patternid INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
    ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path, skip=("gm_audit",)):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if not any(s in l for s in skip)]
    db.close()
    return out


class BarkRootType(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO itempattern VALUES (1014, 'weak roots', 'A watery concoction.', 0, 824019198, -1, 0, "
                   "1, 2, 32767, 1023)")
        db.execute("INSERT INTO itempattern VALUES (707771, 'pristine kobold ear', 'A pristine kobold ear. A merchant "
                   "will pay for it.', 144, 1656860043, -1, 0, 99, 0, 0, 0)")
        for i, name in enumerate(br.NAMES):
            db.execute("INSERT INTO itempattern VALUES (?, ?, ?, 100, 824019198, -1, 0, 99, 2, 32767, 1023)",
                       (708100 + i, name, f"A {name}. A merchant will pay for it."))
        db.execute("INSERT INTO charinventory (serverid, stackleft, patternid) VALUES (747, 5, ?)",
                   (708100 + br.NAMES.index("pristine kolbok root"),))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        before = dump(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before, "the report wrote")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.path)
        got = db.execute("SELECT DISTINCT procanim, classuse, raceuse, itemicon, maxstack, itemcost FROM itempattern "
                         "WHERE patternid >= 708100").fetchall()
        potion = db.execute("SELECT procanim, classuse, raceuse FROM itempattern WHERE patternid = 1014").fetchone()
        audits = db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0]
        db.close()
        self.assertEqual(got, [(0, 0, 0, 824019198, 99, 100)])
        self.assertEqual(potion, (2, 32767, 1023), "the potion it was copied from moved")
        self.assertEqual(audits, len(br.NAMES))
        applied = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.path), applied)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before)

    def refused(self, sql):
        db = sqlite3.connect(self.path)
        db.execute(sql)
        db.commit()
        db.close()
        before = dump(self.path, skip=())
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("refused", r.stdout)
        self.assertEqual(dump(self.path, skip=()), before)

    def test_missing_refused(self):
        self.refused("DELETE FROM itempattern WHERE itemname = 'sapling bark'")

    def test_unknown_usable_trophy_refused(self):
        self.refused("INSERT INTO itempattern VALUES (709000, 'odd leaf', 'An odd leaf. A merchant will pay for it.', "
                     "5, 1, -1, 0, 99, 2, 32767, 1023)")


if __name__ == "__main__":
    unittest.main()
