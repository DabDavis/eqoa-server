#!/usr/bin/env python3
"""Tests for petpass_heirloom_1123e.py on a small database holding the rows as read on Heirloom: a dry run writes
nothing; --apply writes each change once (a second --apply nothing, a report then says every change is already
made); --waldorf takes the bearwere line's rows and sets the werehunters' standing; a row not as read refuses the
whole run; --undo puts every row back exactly (deleted rows inserted again) and drops the log."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_heirloom_1123e as t  # noqa: E402

ARCHS = (162588, 165284)


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
        CREATE TABLE classspells (class INTEGER, spellid INTEGER, PRIMARY KEY (class, spellid));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, classuse INTEGER, levelreq INTEGER,
            equipslot INTEGER, itemicon INTEGER DEFAULT 0);
        CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_id INTEGER DEFAULT 0,
            npc_type INTEGER DEFAULT 0);
        CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, stackleft INTEGER,
            remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER,
            listnumber INTEGER);
        CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charname TEXT);
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT);
        CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER);
        CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
            count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
            updated TEXT NOT NULL DEFAULT '', picks TEXT NOT NULL DEFAULT '', taken INTEGER,
            PRIMARY KEY (serverid, questid));
        CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL,
            addedorder INTEGER NOT NULL, onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL,
            showhide INTEGER NOT NULL);
        CREATE TABLE charbuffs (serverid INTEGER NOT NULL, seq INTEGER NOT NULL, spellid INTEGER NOT NULL,
            name TEXT NOT NULL, PRIMARY KEY (serverid, seq));
        CREATE TABLE charfactions (serverid INTEGER NOT NULL, faction TEXT NOT NULL, standing INTEGER NOT NULL,
            PRIMARY KEY (serverid, faction));
    """)
    db.execute("INSERT INTO itempattern VALUES (?, ?, 512, 46, -1, 0)", (t.ANGUISH_SCROLL, t.ANGUISH))
    for name, (pid, bit, sid, cls) in t.SCROLLS.items():
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, 46, -1, 0)", (pid, name, bit))
        db.execute("INSERT INTO spellpattern VALUES (?, ?)", (sid, name))
        db.execute("INSERT INTO classspells VALUES (?, ?)", (cls, sid))
        db.execute("INSERT INTO loottables (tablename, patternid) VALUES (?, ?)", (t.UWU_TABLE, pid))
    for name, (pid, bit, sid, cls) in t.MORE.items():
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, 46, -1, 0)", (pid, name, bit))
        db.execute("INSERT INTO spellpattern VALUES (?, ?)", (sid, name))
        db.execute("INSERT INTO classspells VALUES (?, ?)", (cls, sid))
        db.execute("INSERT INTO loottables (tablename, patternid) VALUES (?, ?)", (t.UWU_TABLE, pid))
    # a second Burning Blade (a sword), not the scroll
    db.execute("INSERT INTO itempattern VALUES (9001, 'Burning Blade', 32767, 40, 14, 0)")
    for a in ARCHS:
        db.execute("INSERT INTO npcs (npc_name, npc_id, npc_type) VALUES (?, ?, 129)", (t.ARCHIVIST, a))
        db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                   "listnumber) VALUES (?, 1, 0, 0, ?, -1, 1, 13)", (a, t.ANGUISH_SCROLL))
    db.execute("INSERT INTO itempattern VALUES (?, ?, 0, 1, -1, ?)", (t.DARK_FROM, t.DARK_FROM_NAME, t.DARK_GEM))
    db.execute("INSERT INTO itempattern VALUES (?, ?, 0, 1, -1, ?)", (t.BLACK_SAPPHIRE, t.BS_NAME, t.GREEN_GEM))
    # Waldorf
    db.execute("INSERT INTO characters VALUES (?, ?)", (t.WALDORF_SID, t.WALDORF))
    for key, (qid, qname) in t.BEAR_QUESTS.items():
        db.execute("INSERT INTO quests VALUES (?, ?)", (qid, qname))
        db.execute("INSERT INTO quest_import_quests VALUES (?, ?)", (key, qid))
        db.execute("INSERT INTO charquests (serverid, questid, step, done, slot, updated, taken) "
                   "VALUES (?, ?, 0, 1, 141, '2026-11-22T20:54:45Z', ?)",
                   (t.WALDORF_SID, qid, 1795450776 if qid == 101353 else None))
    db.execute("INSERT INTO charquests (serverid, questid, step, done) VALUES (?, 999, 2, 0)", (t.WALDORF_SID,))
    db.execute("INSERT INTO spellpattern VALUES (?, ?)", (t.BEARWERE_FORM, t.BEARWERE_NAME))
    db.execute("INSERT INTO spells VALUES (140682, ?, ?, 111, 0, -1, 0, -1)", (t.WALDORF_SID, t.BEARWERE_FORM))
    db.execute("INSERT INTO spells VALUES (140600, ?, 5, 110, 0, -1, 0, -1)", (t.WALDORF_SID,))
    db.execute("INSERT INTO charbuffs VALUES (?, 1, ?, 'Bearwere')", (t.WALDORF_SID, t.BEARWERE_FORM))
    db.execute("INSERT INTO charfactions VALUES (?, ?, ?)", (t.WALDORF_SID, t.WEREHUNTERS, t.WH_OLD))
    db.execute("INSERT INTO charfactions VALUES (?, 'Freeport Militia', 300)", (t.WALDORF_SID,))
    db.commit()
    db.close()


def snapshot(path):
    db = sqlite3.connect(path)
    out = {}
    for tbl in ("itempattern", "npcitem", "charquests", "spells", "charbuffs", "charfactions"):
        out[tbl] = sorted(db.execute(f"SELECT * FROM {tbl}").fetchall(), key=repr)
    db.close()
    return out


def run(*args):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(buf):
        code = t.main(list(args))
    return code, buf.getvalue()


class PetPass1123e(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_dry_apply_again_undo(self):
        before = snapshot(self.path)
        code, out = run("--db", self.path)
        self.assertEqual(code, 0, out)
        self.assertIn("dry run: 19 rows would change", out)  # 9 scrolls x 2 archivists + the icon
        self.assertEqual(snapshot(self.path), before)

        code, out = run("--db", self.path, "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 19 rows", out)
        for a in ARCHS:
            got = self.q("SELECT patternid, listnumber, location FROM npcitem WHERE npcid = ? ORDER BY patternid", (a,))
            want = sorted([(t.ANGUISH_SCROLL, 13, 1)] + [(p, 13, 1) for p, _, _, _ in t.SCROLLS.values()])
            self.assertEqual(got, want)
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE patternid = ?", (t.BLACK_SAPPHIRE,))[0][0],
                         t.DARK_GEM)
        # Uwu's drops left as they were
        self.assertEqual(self.q("SELECT count(*) FROM loottables WHERE tablename = ?", (t.UWU_TABLE,))[0][0], 11)
        self.assertEqual(self.q("SELECT count(*) FROM npcitem WHERE patternid IN (8702, 8664)")[0][0], 0)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 19)
        # Waldorf untouched by the main part
        self.assertEqual(snapshot(self.path)["charquests"], before["charquests"])

        code, out = run("--db", self.path, "--apply", "--no-backup")
        self.assertIn("applied already", out)
        code, out = run("--db", self.path)
        self.assertIn("dry run: 0 rows would change", out)

        code, out = run("--db", self.path, "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(snapshot(self.path), before)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (t.LOG,))[0][0], 0)

    def test_waldorf_apply_again_undo(self):
        before = snapshot(self.path)
        code, out = run("--db", self.path, "--waldorf")
        self.assertEqual(code, 0, out)
        self.assertIn("dry run: 6 rows would change", out)  # 3 quests, the form, its effect, the standing
        code, out = run("--db", self.path, "--waldorf", "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT questid FROM charquests"), [(999,)])
        self.assertEqual(self.q("SELECT spellid FROM spells"), [(5,)])
        self.assertEqual(self.q("SELECT count(*) FROM charbuffs")[0][0], 0)
        self.assertEqual(dict(self.q("SELECT faction, standing FROM charfactions")),
                         {t.WEREHUNTERS: 0, "Freeport Militia": 300})
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 6)
        # the scrolls are the other part's
        self.assertEqual(self.q("SELECT count(*) FROM npcitem")[0][0], 2)

        code, out = run("--db", self.path, "--waldorf", "--apply", "--no-backup")
        self.assertIn("applied already", out)
        code, out = run("--db", self.path, "--waldorf")
        self.assertIn("dry run: 0 rows would change", out)

        code, out = run("--db", self.path, "--waldorf", "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(snapshot(self.path), before)
        self.assertEqual(self.q("SELECT taken FROM charquests WHERE questid = 101353")[0][0], 1795450776)

    def test_more_after_the_first_each_undone_alone(self):
        before = snapshot(self.path)
        run("--db", self.path, "--apply", "--no-backup")
        mid = snapshot(self.path)
        code, out = run("--db", self.path, "--more")
        self.assertIn("dry run: 4 rows would change", out)  # 2 scrolls x 2 archivists
        code, out = run("--db", self.path, "--more", "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        for a in ARCHS:
            got = {r[0] for r in self.q("SELECT patternid FROM npcitem WHERE npcid = ? AND listnumber = 13", (a,))}
            self.assertTrue({8702, 8664} <= got)
        code, out = run("--db", self.path, "--more", "--apply", "--no-backup")
        self.assertIn("applied already", out)
        code, out = run("--db", self.path, "--more")
        self.assertIn("dry run: 0 rows would change", out)
        code, out = run("--db", self.path, "--more", "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertEqual(snapshot(self.path), mid)
        run("--db", self.path, "--undo", "--no-backup")
        self.assertEqual(snapshot(self.path), before)

    def test_refuses_a_row_not_as_read(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET itemicon = 5 WHERE patternid = ?", (t.BLACK_SAPPHIRE,))
        db.commit()
        db.close()
        before = snapshot(self.path)
        code, out = run("--db", self.path, "--apply", "--no-backup")
        self.assertEqual(code, 2, out)
        self.assertIn(t.BS_NAME, out)
        self.assertEqual(snapshot(self.path), before)

    def test_refuses_a_standing_not_as_read(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charfactions SET standing = -400 WHERE faction = ?", (t.WEREHUNTERS,))
        db.commit()
        db.close()
        code, out = run("--db", self.path, "--waldorf", "--apply", "--no-backup")
        self.assertEqual(code, 2, out)
        self.assertEqual(len(self.q("SELECT * FROM charquests")), 4)

    def test_undo_leaves_a_row_holding_the_key(self):
        run("--db", self.path, "--waldorf", "--apply", "--no-backup")
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charquests (serverid, questid, step) VALUES (?, 101354, 1)", (t.WALDORF_SID,))
        db.commit()
        db.close()
        code, out = run("--db", self.path, "--waldorf", "--undo", "--no-backup")
        self.assertEqual(code, 0, out)
        self.assertIn("left     charquests", out)
        self.assertEqual(self.q("SELECT step, done FROM charquests WHERE questid = 101354")[0], (1, 0))


if __name__ == "__main__":
    unittest.main()
