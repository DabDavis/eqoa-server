#!/usr/bin/env python3
"""sorcerer_fury_20261113.py on a small database: the report writes nothing; --apply uncrosses Flame and Icy Mastery
and gives an owner of one rank alone the spell it paid for in its book's place (an owner of both, and a deleted
character's leftover rows, left); a second apply does nothing; --undo puts every table back exactly. The control:
a rank whose abilities are neither as imported nor as fixed is refused, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "sorcerer_fury_20261113.py")

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1, name TEXT NOT NULL DEFAULT '',
    parent INTEGER NOT NULL DEFAULT 0, adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '');
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL,
    addedorder INTEGER NOT NULL, onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL,
    showhide INTEGER NOT NULL);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT NOT NULL);
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
    ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
INSERT INTO spellpattern VALUES (701141, 'Flaming Fury'), (701159, 'Icy Fury'), (701242, 'Thunderous Fury');
INSERT INTO cmcatalog VALUES (7111, 4, 8192, 'Sorcerer', 1, 'Sorcerer', 0, '', ''),
    (7112, 4, 8192, 'Sorcerer', 2, 'Flame Mastery', 7111, 'Icy Fury@60', '701159'),
    (7113, 4, 8192, 'Sorcerer', 3, 'Icy Mastery', 7111, 'Flaming Fury@60', '701141'),
    (7114, 4, 8192, 'Sorcerer', 4, 'Lightning Mastery', 7111, 'Thunderous Fury@60', '701242');
INSERT INTO characters VALUES (1, 'Flamer'), (2, 'Icer'), (3, 'Both');
INSERT INTO charcms VALUES (1, 7111, 200, 1), (1, 7112, 50, 2), (1, 7114, 50, 3), (2, 7111, 200, 4), (2, 7113, 50, 5),
    (3, 7112, 50, 6), (3, 7113, 50, 7), (9, 7112, 50, 8);
INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES
    (1, 701159, 120, 1, 1, 0, -1), (1, 701242, 121, 1, 0, 0, -1), (2, 701141, 115, 1, 2, 0, -1),
    (3, 701141, 10, 0, -1, 0, -1), (3, 701159, 11, 0, -1, 0, -1), (9, 701159, 5, 0, -1, 0, -1);
"""


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path, skip=("gm_audit",)):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if not any(s in l for s in skip)]
    db.close()
    return out


def book(path, sid):
    db = sqlite3.connect(path)
    r = db.execute("SELECT spellid, addedorder, onHotBar, whereonBar FROM spells WHERE serverid = ? ORDER BY addedorder",
                   (sid,)).fetchall()
    db.close()
    return r


class SorcererFury(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
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
        cat = dict((i, (a, s)) for i, a, s in db.execute("SELECT id, adds, add_spells FROM cmcatalog"))
        audits = db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0]
        db.close()
        self.assertEqual(cat[7112], ("Flaming Fury@60", "701141"))
        self.assertEqual(cat[7113], ("Icy Fury@60", "701159"))
        self.assertEqual(cat[7114], ("Thunderous Fury@60", "701242"))
        self.assertEqual(book(self.path, 1), [(701141, 120, 1, 1), (701242, 121, 1, 0)])
        self.assertEqual(book(self.path, 2), [(701159, 115, 1, 2)])
        self.assertEqual(book(self.path, 3), [(701141, 10, 0, -1), (701159, 11, 0, -1)])
        self.assertEqual(book(self.path, 9), [(701159, 5, 0, -1)], "a deleted character's leftover rows moved")
        self.assertEqual(audits, 4)
        applied = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.path), applied)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path, skip=("gm_audit", "sorcerer_fury_1113_log")), before)

    def test_refused(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE cmcatalog SET adds = 'Something@60', add_spells = '1' WHERE id = 7113")
        db.commit()
        db.close()
        before = dump(self.path, skip=())
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("refused", r.stdout)
        self.assertEqual(dump(self.path, skip=()), before)


if __name__ == "__main__":
    unittest.main()
