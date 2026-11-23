#!/usr/bin/env python3
"""froglok_group_quests_20261122.py on a small database: the group and epic quests naming the human gain the froglok,
the city quests and lists without the human do not; a second apply writes nothing; undo exact."""
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import froglok_group_quests_20261122 as tool  # noqa: E402

ROWS = [
    (100939, "Gypsy Blood", "0, 2, 3, 4, 5, 8, 9", "EQOA-Data 30,40,49/group-6-evil/L30-a (tools/import_quests.py)"),
    (101112, "The Spirits", "0, 2, 4, 5, 9, 11, 12, 32", "EQOA-Data 60/evil-healer/ep1-1 (tools/import_quests.py)"),
    (100061, "The Militia Bracer", "0, 10", "EQOA-Data 1-20/freeport/warrior/L1 (tools/import_quests.py)"),
    (100547, "Qeynos warrior", "0", "EQOA-Data 1-20/qeynos/warrior/L1 (tools/import_quests.py)"),
    (100927, "Elves only", "1, 7", "EQOA-Data 30,40,49/group-3-good/L30-a (tools/import_quests.py)"),
    (100928, "Any race", "", "EQOA-Data 30,40,49/group-3-good/L30-b (tools/import_quests.py)"),
]


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript("""CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
            username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, races TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT '');""")
        db.executemany("INSERT INTO quests VALUES (?, ?, ?, ?)", ROWS)
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def races(self):
        db = sqlite3.connect(self.db)
        out = dict(db.execute("SELECT questid, races FROM quests"))
        db.close()
        return out

    def test_apply_again_undo(self):
        before = self.races()
        self.assertEqual(run("--db", self.db)[0], 0)
        self.assertEqual(self.races(), before)
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 0, out)
        after = self.races()
        self.assertEqual(after[100939], "0, 2, 3, 4, 5, 8, 9, 10")
        self.assertEqual(after[101112], "0, 2, 4, 5, 9, 10, 11, 12, 32")
        for q in (100061, 100547, 100927, 100928):
            self.assertEqual(after[q], before[q])
        self.assertIn("nothing to write", run("--db", self.db, "--apply")[1])
        self.assertIn("2 quests put back", run("--db", self.db, "--undo")[1])
        self.assertEqual(self.races(), before)


if __name__ == "__main__":
    unittest.main()
