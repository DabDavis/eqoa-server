#!/usr/bin/env python3
"""Tests for petpass_heirloom_1123c.py on a small database holding the rows as read on Heirloom: a dry run writes
nothing; --apply writes each change once (a second --apply nothing); the orbs' talk lands where their pads do; a row
not as read refuses the whole run; --undo puts every row back and drops the log."""
import contextlib
import io
import math
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_heirloom_1123c as t  # noqa: E402

TABLES = ("itempattern", "quests", "queststeps", "npcs", "npcwander", "orbs", "lightspheres")
ORB_TO = {9: (21250.0, 305.0, 13513.8), 10: (21245.8, 245.0, 13516.7)}


def make(path):
    db = sqlite3.connect(path)
    cols = ", ".join(f"{c} {'TEXT' if c == 'itemdesc' else 'INTEGER'}" for c in t.SIGNET_COLS)
    db.executescript(f"""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, {cols});
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT DEFAULT '',
            PRIMARY KEY (questid, step));
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL,
            y_coord REAL, z_coord REAL);
        CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL
            DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE orbs (id INTEGER PRIMARY KEY, name TEXT, world INTEGER, to_x REAL, to_y REAL, to_z REAL);
        CREATE TABLE lightspheres (id INTEGER PRIMARY KEY AUTOINCREMENT, world INTEGER NOT NULL DEFAULT 0,
            x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, line TEXT NOT NULL DEFAULT '',
            answer TEXT NOT NULL DEFAULT 'Touch the light.', to_world INTEGER NOT NULL DEFAULT 0,
            to_x REAL NOT NULL DEFAULT 0, to_y REAL NOT NULL DEFAULT 0, to_z REAL NOT NULL DEFAULT 0,
            to_facing INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm', note TEXT NOT NULL DEFAULT '');
    """)
    names = list(t.SIGNET_COLS)
    db.execute(f"INSERT INTO itempattern (patternid, itemname, {', '.join(names)}) VALUES (700744, ?, "
               f"{', '.join('?' * len(names))})", [t.SIGNET] + [t.SIGNET_COLS[c][0] for c in names])
    db.execute("INSERT INTO itempattern (patternid, itemname, itemicon, equipslot) VALUES (8078, ?, ?, 4)",
               (t.NECK_ICON_REF, t.NECK_ICON))
    db.execute("INSERT INTO quests VALUES (100762, ?, ?)", t.TEMBY)
    db.execute("INSERT INTO queststeps VALUES (100762, 3, 'give', 'Dristan Blane', '700744')")
    db.execute("INSERT INTO quests VALUES (100765, ?, ?)", t.GOBLIN_POISON)
    db.execute("INSERT INTO quests VALUES (100531, 'The Lizardman Outcast', 'Oligan')")
    db.execute("INSERT INTO npcs VALUES (108824, 'Commander Dursley', 0, ?, ?, ?)", t.DURSLEY_OLD)
    db.execute("INSERT INTO npcs VALUES (?, 'goblin fanatic', 0, ?, ?, ?)", (t.FANATIC_ID,) + t.FANATIC_OLD)
    for i, ((x, y, z), orb_id, _) in enumerate(t.SPHERES):
        db.execute("INSERT INTO npcs VALUES (?, 'orb of light', 0, ?, ?, ?)", (108825 + i, x, y, z))
        db.execute("INSERT INTO orbs VALUES (?, ?, 0, ?, ?, ?)", (orb_id, f"Saerk's Tower: orb {orb_id}") + ORB_TO[orb_id])
    for npc_id, old, _ in t.EAGLES:
        db.execute("INSERT INTO npcs VALUES (?, 'young eagle', 1, ?, ?, ?)", (npc_id,) + old)
    for npc_id, old, _ in t.OUTCASTS + [(t.OUTCAST_ROOF[0], t.OUTCAST_ROOF[1], None)]:
        db.execute("INSERT INTO npcs VALUES (?, 'lizardman outcast', 1, ?, ?, ?)", (npc_id,) + old)
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
        code = t.main(list(args) + ["--no-backup"])
    return code, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    got = {tb: sorted(db.execute(f"SELECT * FROM {tb}").fetchall(), key=repr) for tb in TABLES}
    db.close()
    return got


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_dry_run_writes_nothing(self):
        before = dump(self.db)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("dry run: 15 rows would change", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_once_and_undo(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 15 rows", out)
        self.assertEqual(self.q("SELECT equipslot, itemicon, levelreq, sta, dex, wis FROM itempattern WHERE patternid = 700744"),
                         [(4, t.NECK_ICON, 25, 10, 10, 10)])
        self.assertEqual(self.q("SELECT kind FROM queststeps WHERE questid = 100762"), [("show",)])
        self.assertEqual(self.q("SELECT x_coord, y_coord, z_coord FROM npcs WHERE id = 108824"), [t.DURSLEY_NEW])
        self.assertEqual(self.q("SELECT radius FROM npcwander ORDER BY npcid"), [(30.0,), (30.0,)])
        # handlers/lightsphere.go lands the player inFrontOf(to_x, to_z): bindReach along to_facing
        for (x, _, z), orb_id, _ in t.SPHERES:
            tx, ty, tz, h = self.q("SELECT to_x, to_y, to_z, to_facing FROM lightspheres WHERE x = ? AND z = ?", (x, z))[0]
            a = h / 256 * 2 * math.pi
            lx, lz = tx + 3 * math.sin(a), tz + 3 * math.cos(a)
            want = ORB_TO[orb_id]
            self.assertLess(abs(lx - want[0]) + abs(lz - want[2]), 0.01, (orb_id, lx, lz))
            self.assertEqual(ty, want[1])
        self.assertEqual(self.q(f"SELECT count(*) FROM {t.LOG}"), [(15,)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(15,)])
        code, out = run("--db", self.db, "--apply")
        self.assertIn("applied already", out)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (t.LOG,)), [(0,)])

    def test_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = 1 WHERE id = 108824")
        db.commit()
        db.close()
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("refused", out)
        self.assertEqual(dump(self.db), before)

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = 1 WHERE id = 108824")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--undo")
        self.assertIn("left     npcs", out)
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE id = 108824"), [(1.0,)])


if __name__ == "__main__":
    unittest.main()
