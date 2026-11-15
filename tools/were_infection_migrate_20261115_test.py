#!/usr/bin/env python3
"""were_infection_migrate_20261115.py on a small database: the report (and --dry) writes nothing; --apply makes a
charinfection row from a marker's timestamp, seven days from now for one with none, an ended expired row (the marker
taken) for one over seven days old, an ended turned row (the marker taken) for a character who turned, keeps the newest
of a character's two markers, each with its charinfection_migration and gm_audit row; a second --apply finds nothing;
--undo puts every marker back as it was and deletes the rows it made, leaving an infection caught since. A record of
the wrong icon refuses --apply."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "were_infection_migrate_20261115.py")
NOW = 1_800_000_000
DAY = 86400


def ts(t):
    import datetime
    return datetime.datetime.fromtimestamp(t, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def make(path, icon=0xCF3C577B):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, source TEXT NOT NULL DEFAULT '');
        CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
            count INTEGER NOT NULL DEFAULT 0, done INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
            updated TEXT NOT NULL DEFAULT '', picks TEXT NOT NULL DEFAULT '', PRIMARY KEY (serverid, questid));
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_level INTEGER, npc_type INTEGER);
        CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT);
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, icon INTEGER);
        CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, fxdict INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO npcs VALUES (1, 'elder wolfwere', 45, 0), (2, 'Ancient Weregator', 55, 0), (3, 'grass snake', 30, 0);
        INSERT INTO npctemplate VALUES ('elder wolfwere', 'roamer'), ('grass snake', 'roamer');
        INSERT INTO spelleffect VALUES (875, 86964495);
    """)
    db.execute("INSERT INTO spellpattern VALUES (875, 'Disease Cloud', ?)", (icon - (1 << 32) if icon >= 1 << 31 else icon,))
    for qid, key in [(101422, "wolfwere/infection"), (101423, "bearwere/infection"), (101424, "ratwere/infection"),
                     (101425, "lionwere/infection"), (101426, "gatorwere/infection"), (101346, "wolfwere/p3"), (101367, "lionwere/p3")]:
        db.execute("INSERT INTO quests VALUES (?, ?)", (qid, f"EQOA-Data Weres/{key} (tools/import_quests.py)"))
    for sid, qid, upd in [(1, 101422, ts(NOW - 2 * DAY)), (2, 101423, ""), (3, 101424, ts(NOW - 9 * DAY)),
                          (4, 101425, ts(NOW - 3600)), (4, 101367, ""), (5, 101426, ts(NOW - 3 * DAY)), (5, 101422, ts(NOW - DAY)),
                          (6, 101346, "")]:
        db.execute("INSERT INTO charquests (serverid, questid, step, done, updated) VALUES (?, ?, 0, 1, ?)", (sid, qid, upd))
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, "--now", str(NOW), *args],
                          capture_output=True, text=True)


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *a):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_migration(self):
        make(self.path)
        before = self.q("SELECT * FROM charquests ORDER BY serverid, questid")
        r = run(self.path, "--dry")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("markers to migrate: 6", r.stdout)
        self.assertIn("weres given their crit (the five lines' names): 2 spawn rows", r.stdout)
        self.assertIn(": OK", r.stdout)
        self.assertEqual(self.q("SELECT name FROM sqlite_master WHERE name = 'charinfection'"), [])
        self.assertEqual(before, self.q("SELECT * FROM charquests ORDER BY serverid, questid"))

        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        rows = {s: (l, a, e, how) for s, l, a, e, how in self.q("SELECT serverid, line, applied_at, expires_at, ended_how FROM charinfection")}
        self.assertEqual(rows[1], ("wolfwere", NOW - 2 * DAY, NOW + 5 * DAY, ""))
        self.assertEqual(rows[2], ("bearwere", NOW, NOW + 7 * DAY, ""))
        self.assertEqual(rows[3], ("ratwere", NOW - 9 * DAY, NOW - 2 * DAY, "expired"))
        self.assertEqual(rows[4][3], "turned")
        self.assertEqual(rows[5], ("wolfwere", NOW - DAY, NOW + 6 * DAY, ""))
        self.assertNotIn(6, rows)  # a turned character with no marker: nothing
        markers = {(s, q) for s, q in self.q("SELECT serverid, questid FROM charquests WHERE questid BETWEEN 101422 AND 101426")}
        self.assertEqual(markers, {(1, 101422), (2, 101423), (5, 101422)})
        self.assertEqual(self.q("SELECT COUNT(*) FROM charinfection_migration")[0][0], 6)
        self.assertEqual(self.q("SELECT COUNT(*) FROM gm_audit WHERE tbl = 'charinfection'")[0][0], 6)

        r = run(self.path, "--apply")
        self.assertIn("markers to migrate: 0", r.stdout)
        self.assertEqual(self.q("SELECT COUNT(*) FROM charinfection")[0][0], 5)

        # an infection caught since by the server: --undo leaves it
        sqlite3.connect(self.path).execute("UPDATE charinfection SET source = 'elder wolfwere', grant_id = '600/0/1' WHERE serverid = 2").connection.commit()
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(before, self.q("SELECT * FROM charquests ORDER BY serverid, questid"))
        self.assertEqual(self.q("SELECT serverid FROM charinfection"), [(2,)])
        self.assertEqual(self.q("SELECT COUNT(*) FROM charinfection_migration")[0][0], 0)

    def test_wrong_icon_refuses(self):
        make(self.path, icon=0x12345678)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 2)
        self.assertIn("MISMATCH", r.stdout)
        self.assertEqual(self.q("SELECT name FROM sqlite_master WHERE name = 'charinfection'"), [])


if __name__ == "__main__":
    unittest.main()
