#!/usr/bin/env python3
"""pos_armor_t290_20261121.py: apply, a second apply, --skip, undo, an imported copy left alone, refusals."""
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import pos_armor_t290_20261121 as tool  # noqa: E402

ROWS = [(6765, "bracers of the Traveler", 32, 151, 35), (7162, "Leggings of the Assasin", 64, 201, 35),
        (7413, "Boots of the Mystic", 256, 101, 100), (990890, "Leggings of the Assasin", 64, 201, 35),
        (6760, "Bracers of the Archer", 2, 151, 35)]


def make(path, rows=ROWS):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER, AC INTEGER, POWMAX INTEGER)")
    db.execute("CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER, made TEXT, source TEXT)")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
               "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)")
    db.executemany("INSERT INTO itempattern VALUES (?, ?, ?, ?, ?)", rows)
    db.execute("INSERT INTO itemimported VALUES (990890, 7162, '', '')")
    db.commit()
    db.close()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "i.db")

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql).fetchall()
        finally:
            db.close()

    def figures(self):
        return {p: (ac, pw) for p, ac, pw in self.q("SELECT patternid, AC, POWMAX FROM itempattern")}

    def test_report_writes_nothing(self):
        make(self.path)
        tool.main(["--db", self.path])
        self.assertEqual(self.figures()[6765], (151, 35))
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 0)

    def test_apply_again_undo(self):
        make(self.path)
        tool.main(["--db", self.path, "--apply"])
        f = self.figures()
        self.assertEqual((f[6765], f[7162], f[7413]), ((123, 35), (201, 30), (10, 100)))
        self.assertEqual(f[990890], (201, 35))   # the imported copy follows its base at the server's start
        self.assertEqual(f[6760], (151, 35))
        tool.main(["--db", self.path, "--apply"])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 3)
        tool.main(["--db", self.path, "--undo"])
        self.assertEqual(self.figures(), {r[0]: (r[3], r[4]) for r in ROWS})
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 6)
        self.assertEqual(self.q(f"SELECT count(*) FROM {tool.LOG}")[0][0], 0)

    def test_skip(self):
        make(self.path)
        tool.main(["--db", self.path, "--apply", "--skip", "boots of the mystic"])
        self.assertEqual(self.figures()[7413], (101, 100))
        self.assertEqual(self.figures()[6765], (123, 35))
        with self.assertRaises(SystemExit):
            tool.main(["--db", self.path, "--skip", "no such boots"])

    def test_undo_leaves_a_changed_cell(self):
        make(self.path)
        tool.main(["--db", self.path, "--apply"])
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET AC = 130 WHERE patternid = 6765")
        db.commit()
        db.close()
        tool.main(["--db", self.path, "--undo"])
        self.assertEqual(self.figures()[6765], (130, 35))
        self.assertEqual(self.figures()[7413], (101, 100))

    def test_refuses_an_unexpected_figure(self):
        rows = [r if r[0] != 7413 else (7413, "Boots of the Mystic", 256, 99, 100) for r in ROWS]
        make(self.path, rows)
        with self.assertRaises(SystemExit):
            tool.main(["--db", self.path, "--apply"])
        self.assertEqual(self.figures()[6765], (151, 35))   # nothing done

    def test_refuses_a_wrong_class_or_two_records(self):
        rows = [r if r[0] != 6765 else (6765, "bracers of the Traveler", 34, 151, 35) for r in ROWS]
        make(self.path, rows)
        with self.assertRaises(SystemExit):
            tool.main(["--db", self.path, "--apply"])
        os.remove(self.path)
        make(self.path, ROWS + [(8000, "BOOTS OF THE MYSTIC", 256, 101, 100)])
        with self.assertRaises(SystemExit):
            tool.main(["--db", self.path, "--apply"])


if __name__ == "__main__":
    unittest.main()
