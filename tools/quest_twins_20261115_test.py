#!/usr/bin/env python3
"""tools/quest_twins_20261115.py on a made-up database: the two quests made
to exclude each other, a character's unfinished twin taken (the finished one
and a character with both kept), a second --apply a no-op, --undo exact, and a
database whose quests are not the two refused. No game data.

    python3 tools/quest_twins_20261115_test.py
"""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_quests as iq  # noqa: E402
import quest_twins_20261115 as qt  # noqa: E402


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    # an older quests table: no excludes column (the tool makes it)
    db.executescript(iq.QUEST_DDL.replace(", excludes TEXT NOT NULL DEFAULT ''", ""))
    db.executescript("""
    CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
        username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
        rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT);
    CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL,
        digest TEXT NOT NULL, enabled INTEGER NOT NULL);
    """)
    for qid, (name, key) in qt.TWINS.items():
        db.execute("INSERT INTO quests (questid, name, enabled, giver) VALUES (?, ?, 1, 'Argot Bittersteel')", (qid, name))
        db.execute("INSERT INTO queststeps (questid, step, kind, target, rewarditems) VALUES (?, 1, 'give', 'Argot Bittersteel', '8086')", (qid,))
        db.execute("INSERT INTO quest_import_quests VALUES (?, ?, 'x.json', ?, 1)", (key, qid, iq.digest(db, qid)))
    db.executemany("INSERT INTO characters VALUES (?, ?)", [(1, "Cardile"), (2, "Both"), (3, "Fresh")])
    db.executemany("INSERT INTO charquests (serverid, questid, step, done, slot, updated) VALUES (?, ?, ?, ?, ?, ?)", [
        (1, 100868, 0, 1, 83, "2026-11-15T22:35:23Z"), (1, 100770, 1, 0, 86, "2026-11-15T22:36:00Z"),
        (2, 100868, 0, 1, 1, ""), (2, 100770, 0, 1, 2, ""),
        (3, 100770, 2, 0, 5, ""),
    ])
    return db


def rows(db):
    return (db.execute("SELECT * FROM charquests ORDER BY serverid, questid").fetchall(),
            db.execute("SELECT key, digest FROM quest_import_quests ORDER BY key").fetchall())


def run(*argv):
    with contextlib.redirect_stdout(io.StringIO()) as out:
        code = qt.main(list(argv))
    return code, out.getvalue()


class QuestTwins(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "q.db")
        self.db = make_db(self.path)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def test_apply_and_undo(self):
        before = rows(self.db)
        code, out = run("--db", self.path)
        self.assertEqual(code, 0)
        self.assertIn("would take character 1 Cardile's 100770 (step 1)", out)
        self.assertEqual(rows(self.db), before, "a report wrote")
        code, out = run("--db", self.path, "--apply")
        self.assertEqual(code, 0, out)
        ex = dict(self.db.execute("SELECT questid, excludes FROM quests"))
        self.assertEqual(ex, {100868: "100770", 100770: "100868"})
        held = {(s, q) for s, q in self.db.execute("SELECT serverid, questid FROM charquests")}
        self.assertNotIn((1, 100770), held, "Cardile's unfinished twin kept")
        self.assertTrue({(1, 100868), (2, 100868), (2, 100770), (3, 100770)} <= held, held)
        # the import's digests follow, so a re-import knows its own quests
        for qid, (_, key) in qt.TWINS.items():
            dg = self.db.execute("SELECT digest FROM quest_import_quests WHERE key = ?", (key,)).fetchone()[0]
            self.assertEqual(dg, iq.digest(self.db, qid))
        audits = self.db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        self.assertEqual(audits, 3, "two excludes and one row, each audited")
        # a second apply: nothing
        mid = rows(self.db)
        code, out = run("--db", self.path, "--apply")
        self.assertIn("already applied", out)
        self.assertEqual(rows(self.db), mid)
        # undo: exact
        code, out = run("--db", self.path, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(rows(self.db), before)
        self.assertEqual(dict(self.db.execute("SELECT questid, excludes FROM quests")), {100868: "", 100770: ""})

    def test_refused_on_other_quests(self):
        self.db.execute("UPDATE quests SET name = 'Something Else' WHERE questid = 100770")
        code, out = run("--db", self.path, "--apply")
        self.assertEqual(code, 2)
        self.assertIn("refused", out)
        self.assertEqual(self.db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 0)


if __name__ == "__main__":
    unittest.main()
