#!/usr/bin/env python3
"""account_bank_20261114.py on a small database: the report writes nothing; --apply merges each account's banks (a kept
row back to the bank, one list numbered across its characters, the tunar summed into accountbank, every character's
bankTunar left), with a gm_audit line an account; a second --apply finds nothing to do; --undo puts every row and every
column back exactly when nothing happened between, and after play puts back what is still in the bank, leaves a
withdrawn row with its withdrawer, gives a deleted character's banked rows to the account's first character and the
tunar the bank gained to it. The controls: an account with no bank is not touched; accountbank already holding tunar
with no log is refused, nothing written."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "account_bank_20261114.py")


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
        CREATE TABLE characters (charName TEXT, accountid INTEGER, serverid INTEGER PRIMARY KEY AUTOINCREMENT,
            tunar INTEGER NOT NULL DEFAULT 0, bankTunar INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, stackleft INTEGER,
            remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO accountinfo VALUES (1, 'dab'), (2, 'other'), (3, 'empty');
        INSERT INTO characters (charName, accountid, serverid, tunar, bankTunar) VALUES
            ('Ash', 1, 10, 5, 100), ('Birch', 1, 11, 0, 250), ('Cedar', 2, 20, 9, 0), ('Elm', 3, 30, 1, 0);
        -- Ash: two bank rows and one kept from the bank; Birch: two; Cedar: one; Elm: none (a pack row)
        INSERT INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) VALUES
            (1, 10, 1, 50, 0, 7, -1, 1, 0), (2, 10, 3, 100, 0, 8, -1, 1, 1), (3, 10, 1, 100, 2, 9, -1, 3, 0),
            (4, 11, 1, 100, 0, 7, -1, 1, 0), (5, 11, 4, 100, 0, 8, -1, 1, 1),
            (6, 20, 1, 100, 0, 7, -1, 1, 0),
            (7, 30, 1, 100, 0, 7, -1, 0, 0),
            (8, 10, 1, 100, 0, 7, 3, 0, 0);
    """)
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def snap(path):
    db = sqlite3.connect(path)
    try:
        return (db.execute("SELECT * FROM charinventory ORDER BY itemid").fetchall(),
                db.execute("SELECT serverid, tunar, bankTunar FROM characters ORDER BY serverid").fetchall())
    finally:
        db.close()


def q(path, sql, *args):
    db = sqlite3.connect(path)
    try:
        return db.execute(sql, args).fetchall()
    finally:
        db.close()


class AccountBank(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        before = snap(self.path)
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("2 accounts with a bank: 6 bank records", r.stdout)
        self.assertEqual(snap(self.path), before)
        self.assertEqual(q(self.path, "SELECT name FROM sqlite_master WHERE name LIKE 'accountbank%'"), [])

    def test_apply_merges_and_undo_restores(self):
        before = snap(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        # one list: Ash's bank, Ash's kept, then Birch's
        self.assertEqual(q(self.path, "SELECT itemid, serverid, location, listnumber FROM charinventory WHERE serverid IN (10, 11) "
                                      "AND location != 0 ORDER BY listnumber"),
                         [(1, 10, 1, 0), (2, 10, 1, 1), (3, 10, 1, 2), (4, 11, 1, 3), (5, 11, 1, 4)])
        self.assertEqual(q(self.path, "SELECT accountid, tunar FROM accountbank ORDER BY accountid"), [(1, 350), (2, 0)])
        # the characters' own columns, and every other row, as they were
        self.assertEqual(snap(self.path)[1], before[1])
        self.assertEqual([r for r in snap(self.path)[0] if r[1] not in (10, 11) or r[7] == 0],
                         [r for r in before[0] if r[1] not in (10, 11) or r[7] == 0])
        self.assertEqual(q(self.path, "SELECT action, tbl FROM gm_audit"), [("update", "accountbank"), ("update", "accountbank")])
        again = run(self.path, "--apply")
        self.assertEqual(again.returncode, 0)
        self.assertIn("nothing to do", again.stdout)
        u = run(self.path, "--undo")
        self.assertEqual(u.returncode, 0, u.stdout + u.stderr)
        self.assertEqual(snap(self.path), before)
        self.assertEqual(q(self.path, "SELECT COUNT(*) FROM accountbank"), [(0,)])
        self.assertEqual(q(self.path, "SELECT name FROM sqlite_master WHERE name LIKE 'account_bank_1114%'"), [])

    def test_undo_after_play(self):
        self.assertEqual(run(self.path, "--apply").returncode, 0)
        db = sqlite3.connect(self.path)
        # Birch withdraws Ash's row 1; Ash banks 50 more tunar; a new character of account 1 is made, banks a row and
        # is deleted (a holder); Cedar's account banks its first tunar
        db.executescript("""
            UPDATE charinventory SET serverid = 11, location = 0, listnumber = 5 WHERE itemid = 1;
            UPDATE accountbank SET tunar = 400 WHERE accountid = 1;
            INSERT INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
                VALUES (9, 12, 1, 100, 0, 7, -1, 1, 5);
            INSERT INTO accountbank_holders (serverid, accountid, deleted_at) VALUES (12, 1, 1);
            UPDATE accountbank SET tunar = 70 WHERE accountid = 2;
        """)
        db.commit()
        db.close()
        u = run(self.path, "--undo")
        self.assertEqual(u.returncode, 0, u.stdout + u.stderr)
        rows = {r[0]: r for r in q(self.path, "SELECT itemid, serverid, location, listnumber FROM charinventory")}
        self.assertEqual(rows[1], (1, 11, 0, 5), "a withdrawn row stays its withdrawer's")
        self.assertEqual(rows[3], (3, 10, 3, 0), "a kept row back to its keeping")
        self.assertEqual(rows[4], (4, 11, 1, 0))
        self.assertEqual(rows[9][1:3], (10, 1), "a deleted character's banked row to the account's first character")
        self.assertEqual(q(self.path, "SELECT serverid, bankTunar FROM characters ORDER BY serverid"),
                         [(10, 150), (11, 250), (20, 70), (30, 0)])
        self.assertEqual(q(self.path, "SELECT COUNT(*) FROM accountbank"), [(0,)])
        self.assertEqual(q(self.path, "SELECT COUNT(*) FROM accountbank_holders"), [(0,)])

    def test_refused_when_accountbank_holds_tunar_unlogged(self):
        db = sqlite3.connect(self.path)
        db.executescript("CREATE TABLE accountbank (accountid INTEGER PRIMARY KEY, tunar INTEGER NOT NULL DEFAULT 0);"
                         "INSERT INTO accountbank VALUES (1, 5);")
        db.commit()
        db.close()
        before = snap(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(snap(self.path), before)
        self.assertEqual(q(self.path, "SELECT COUNT(*) FROM gm_audit"), [(0,)])


if __name__ == "__main__":
    unittest.main()
