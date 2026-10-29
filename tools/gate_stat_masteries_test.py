#!/usr/bin/env python3
"""tools/gate_stat_masteries.py against a made-up database: every stat-boost
rank gated on its trial's quest (the column added to a catalogue made before
it), a rank whose effects are not its line's left, one a GM gated left, a
rank with no imported quest left, an owner without the quest counted and
nothing taken, a second --apply refused, and --undo giving back the
catalogue as it was but for a value changed since. No game data.

    python3 tools/gate_stat_masteries_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "gate_stat_masteries.py")
sys.path.insert(0, HERE)
import gate_stat_masteries as gate  # noqa: E402

# cmcatalog as a database made before require_quests has it
SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, mastery TEXT NOT NULL DEFAULT '',
    rank INTEGER NOT NULL DEFAULT 1, name TEXT NOT NULL DEFAULT '', level INTEGER NOT NULL DEFAULT 1,
    effects TEXT NOT NULL DEFAULT '', require_ranks TEXT NOT NULL DEFAULT '');
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
    minlevel INTEGER NOT NULL DEFAULT 1);
CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL DEFAULT '',
    digest TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1);
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
    count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, questid));
"""


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    rid, qid = 100, 5000
    for stat, mastery, statname in gate.LINES:
        for n in range(1, gate.RANKS + 1):
            db.execute("INSERT INTO cmcatalog (id, category, mastery, rank, name, level, effects) VALUES (?, 0, ?, ?, ?, ?, ?)",
                       (rid, mastery, n, f"{mastery} {n}", 10 + 10 * n, f"{statname}:{10 * n}"))
            db.execute("INSERT INTO quests (questid, name, enabled, minlevel) VALUES (?, ?, 1, ?)",
                       (qid, f"Trial {stat} {n}", 10 + 10 * n))
            db.execute("INSERT INTO quest_import_quests (key, questid) VALUES (?, ?)",
                       (gate.KEY.format(stat=stat, n=n), qid))
            rid, qid = rid + 1, qid + 1
    # a cap mastery of the same stat: never touched
    db.execute("INSERT INTO cmcatalog (id, category, mastery, rank, name, level, effects) "
               "VALUES (1, 0, 'Brawn', 1, 'Brawn 1', 10, 'STR:1,STRMax:15')")
    # owners of Buldging Muscles 1 (id 100): 7 finished its trial, 8 did not
    db.executemany("INSERT INTO charcms (serverid, cmid) VALUES (?, 100)", [(7,), (8,)])
    db.execute("INSERT INTO charquests (serverid, questid, step, done) VALUES (7, 5000, 0, 1)")
    db.commit()
    db.close()


def run(db, *args, ok=True):
    r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", db, "--no-backup", *args],
                       capture_output=True, text=True)
    if ok and r.returncode != 0:
        raise AssertionError(r.stdout + r.stderr)
    return r


def rows(db, sql):
    c = sqlite3.connect(db)
    try:
        return c.execute(sql).fetchall()
    finally:
        c.close()


class GateStatMasteries(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        out = run(self.db).stdout
        self.assertIn("28 ranks to gate, 0 left", out)
        self.assertIn("characters owning a rank to be gated without its quest finished: 1", out)
        self.assertIn("character 8: Buldging Muscles 1", out)
        self.assertNotIn("character 7", out)
        self.assertFalse(any(r[1] == "require_quests" for r in rows(self.db, "PRAGMA table_info(cmcatalog)")))

    def test_apply_gates_each_rank_on_its_trial_and_undo_puts_it_back(self):
        before = rows(self.db, "SELECT * FROM cmcatalog ORDER BY id")
        run(self.db, "--apply")
        got = dict(rows(self.db, "SELECT name, require_quests FROM cmcatalog"))
        self.assertEqual(got["Buldging Muscles 1"], "5000")
        self.assertEqual(got["Intestinal Fortitude 1"], "5004")
        self.assertEqual(got["Razzle Dazzle 4"], "5027")
        self.assertEqual(got["Brawn 1"], "")
        self.assertEqual(len([v for v in got.values() if v]), 28)
        self.assertEqual(rows(self.db, "SELECT count(*) FROM charcms"), [(2,)])  # nothing taken
        # a second --apply is refused while the log is there
        r = run(self.db, "--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("--undo first", r.stdout + r.stderr)
        # a GM's change since is kept by --undo
        c = sqlite3.connect(self.db)
        c.execute("UPDATE cmcatalog SET require_quests = '5000, 4999' WHERE id = 100")
        c.commit()
        c.close()
        out = run(self.db, "--undo").stdout
        self.assertIn("put back 27 of 28", out)
        self.assertEqual(rows(self.db, "SELECT require_quests FROM cmcatalog WHERE id = 100"), [("5000, 4999",)])
        self.assertEqual(rows(self.db, "SELECT count(*) FROM sqlite_master WHERE name = 'stat_mastery_gate_log'"), [(0,)])
        # the rest as it was, the (empty) column aside
        after = [r[:-1] for r in rows(self.db, "SELECT * FROM cmcatalog ORDER BY id") if r[0] != 100]
        self.assertEqual(after, [r for r in before if r[0] != 100])

    def test_a_rank_unlike_its_line_a_gms_gate_and_a_missing_quest_are_left(self):
        c = sqlite3.connect(self.db)
        c.execute("ALTER TABLE cmcatalog ADD COLUMN require_quests TEXT NOT NULL DEFAULT ''")
        c.execute("UPDATE cmcatalog SET effects = 'STR:15' WHERE name = 'Buldging Muscles 2'")
        c.execute("UPDATE cmcatalog SET require_quests = '9999' WHERE name = 'AGI 3'")
        c.execute("DELETE FROM quest_import_quests WHERE key = 'Misc/stat-boost-quests/cha4'")
        c.execute("UPDATE quests SET enabled = 0 WHERE questid = 5012")  # dex1: a draft, gated all the same
        c.commit()
        c.close()
        out = run(self.db, "--apply").stdout
        self.assertIn("25 ranks to gate, 3 left", out)
        self.assertIn("Buldging Muscles 2: left as it is (its effects are 'STR:15', not STR:20)", out)
        self.assertIn("AGI 3: left as it is (require_quests is '9999', a GM's: left)", out)
        self.assertIn("Razzle Dazzle 4: left as it is (no quest Misc/stat-boost-quests/cha4", out)
        self.assertIn("Unrivaled Grace 1: Trial dex 1 (5012) is a draft", out)
        got = dict(rows(self.db, "SELECT name, require_quests FROM cmcatalog"))
        self.assertEqual((got["Buldging Muscles 2"], got["AGI 3"], got["Razzle Dazzle 4"], got["Unrivaled Grace 1"]),
                         ("", "9999", "", "5012"))


if __name__ == "__main__":
    unittest.main()
