#!/usr/bin/env python3
"""tools/fix_armor_class.py against made-up records: a shield with no armor
class and its armor class's number in agility gets it moved; the largest of
two numbers moves when it is twice the next and the next is no armor class's
size; a copy of the armor class in an attribute is taken out. The controls are
left: two numbers either of which could be the armor class, a robe's number (a
robe need not have an armor class), a number too large for any armor class of
its peers, an ordinary attribute on a piece with no armor class, a record with
its armor class right. --apply then --undo leaves itempattern as it was; a
second --apply is refused; an item a GM changed since is left by --undo. No
game data.

    python3 tools/fix_armor_class_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_armor_class.py")
sys.path.insert(0, HERE)
import fix_armor_class as fac  # noqa: E402

COLS = ("patternid", "itemname", "levelreq", "equipslot", "AC") + fac.STATS


def rec(pid, name, level, slot, ac=0, **stats):
    row = dict.fromkeys(COLS, 0)
    row.update(patternid=pid, itemname=name, levelreq=level, equipslot=slot, AC=ac, **stats)
    return row


PEERS = ([rec(1 + i, f"shield {i}", 39 + i % 3, 13, 180 + 8 * i, str=5 + i) for i in range(6)] +
         [rec(10 + i, f"tunic {i}", 39 + i % 3, 5, 80 + 40 * i, sta=5 + i) for i in range(6)] +
         [rec(20 + i, f"helm {i}", 39 + i % 3, 1, 40 + 4 * i, wis=5 + i) for i in range(6)] +
         [rec(30 + i, f"robe {i}", 39 + i % 3, 2, 30 + 4 * i, intelligence=5 + i) for i in range(6)])
CASES = [
    rec(100, "Tundra Tear", 40, 13, str=15, sta=10, agi=210),         # moved: agi
    rec(101, "Incandescent Shield", 40, 13, str=25, sta=90, agi=250),  # moved: the largest (90 is below half 200)
    rec(102, "Enveloped Tunic", 40, 5, sta=150, agi=140),               # undecided: either could be it
    rec(103, "Starfire Robe", 40, 2, agi=70),                           # undecided: a robe
    rec(104, "Dented Cap", 40, 1, str=15),                              # left: an ordinary attribute
    rec(105, "Danlon's Iron Wall", 40, 13, ac=210, agi=210),           # copied: agi out
    rec(106, "Plain Shield", 40, 13, ac=200, agi=15),                   # left: right already
    rec(107, "Odd Tunic", 40, 5, intelligence=900),                     # undecided: no armor class is that size
]


class FixArmorClass(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.execute(f"CREATE TABLE itempattern ({', '.join(c + (' TEXT' if c == 'itemname' else ' INTEGER') for c in COLS)})")
        db.executemany(f"INSERT INTO itempattern VALUES ({', '.join('?' * len(COLS))})",
                       [tuple(r[c] for c in COLS) for r in PEERS + CASES])
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def rows(self):
        db = sqlite3.connect(self.path)
        out = {r[0]: r for r in db.execute(f"SELECT {', '.join(COLS)} FROM itempattern")}
        db.close()
        return out

    def test_plan(self):
        db = sqlite3.connect(self.path)
        moved, copied, undecided = fac.plan(db)
        db.close()
        self.assertEqual({(r[0], r[4], r[5]) for r in moved}, {(100, "agi", 210), (101, "agi", 250)})
        self.assertEqual({(r[0], r[4]) for r in copied}, {(105, "agi")})
        self.assertEqual({r[0] for r in undecided}, {102, 103, 107})
        self.assertIn("robe", [r for r in undecided if r[0] == 103][0][7])

    def test_apply_undo(self):
        run = lambda *a: subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a],
                                        capture_output=True, text=True)
        before = self.rows()
        r = run("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        got = self.rows()
        i = COLS.index
        self.assertEqual((got[100][i("AC")], got[100][i("agi")], got[100][i("str")]), (210, 0, 15), "the rest left")
        self.assertEqual((got[101][i("AC")], got[101][i("agi")], got[101][i("sta")]), (250, 0, 90))
        self.assertEqual((got[105][i("AC")], got[105][i("agi")]), (210, 0))
        for pid in (102, 103, 104, 106, 107) + tuple(range(1, 36)):
            if pid in before:
                self.assertEqual(got[pid], before[pid], f"control {pid} changed")
        self.assertNotEqual(run("--apply").returncode, 0, "a second --apply is refused")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET AC = 222 WHERE patternid = 101")
        db.commit()
        db.close()
        r = run("--undo")
        self.assertIn("put back 2 items; 1 changed since", r.stdout)
        got = self.rows()
        self.assertEqual(got[101][i("AC")], 222)
        got[101] = before[101]
        self.assertEqual(got, before)


if __name__ == "__main__":
    unittest.main()
