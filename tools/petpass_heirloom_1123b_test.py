#!/usr/bin/env python3
"""Tests for petpass_heirloom_1123b.py on a small database holding the rows as read on Heirloom: a dry run writes
nothing; --apply writes each change once (a second --apply nothing); a row not as read refuses the whole run;
--undo puts every row back and drops the log."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import petpass_heirloom_1123b as t  # noqa: E402

TABLES = ("itempattern", "quests", "queststeps", "npcs", "npcfaction", "npcundead", "npcwander", "factions")


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemicon INTEGER, model INTEGER);
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, minlevel INTEGER,
            factionreq TEXT NOT NULL DEFAULT '', untrusted TEXT NOT NULL DEFAULT '');
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, npcid INTEGER DEFAULT 0,
            x REAL DEFAULT 0, y REAL DEFAULT 0, z REAL DEFAULT 0, journal TEXT DEFAULT '', says TEXT DEFAULT '',
            events TEXT DEFAULT '', PRIMARY KEY (questid, step));
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL,
            y_coord REAL, z_coord REAL);
        CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL
            DEFAULT -1, x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0,
            faction TEXT NOT NULL, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcundead (name TEXT PRIMARY KEY, undead INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL
            DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL
            DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER);
    """)
    pid = 1
    for _, name, cols in t.ITEMS:
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, ?)",
                   (pid, name, cols["itemicon"][0], cols["model"][0] if "model" in cols else 0))
        pid += 1
    for icon, ref in t.LOOK_REFS.items():
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, ?)", (pid, ref, icon, t.MODEL_REFS.get(icon, 0)))
        pid += 1
    for model, ref in t.MODEL_REFS.items():
        db.execute("UPDATE itempattern SET model = ? WHERE itemname = ?", (model, ref))
    db.execute("INSERT INTO npcs VALUES (103840, 'Abominus', 0, 20648.2, 40, 8497.86)")
    for i, giver in enumerate(t.HIGHPASS_GIVERS):
        db.execute("INSERT INTO quests (questid, name, giver, minlevel, factionreq) VALUES (?, ?, ?, 35, ?)",
                   (100775 + i, f"quest {i}", giver, "Citizens of Highpass:150"))
    db.execute("INSERT INTO npcs VALUES (191039, 'Recon Wecon', 0, ?, 57.7, ?)", t.RECON_OLD)
    db.execute("INSERT INTO quests (questid, name, giver, minlevel) VALUES (102914, "
               "'The Clockwork Invasion: Klockizar Riddik', 'Elder Mysidia', 28)")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, says) VALUES (102914, 1, 'talk', 'Recon Wecon', ?)",
               (t.RIDDIK_S1_OLD,))
    mx, my, mz = t.MOUNTAIN
    ev = [{"on": "done", "do": "spawn", "npc": n, "hostile": True, "group": True, "minutes": 30, "world": 0,
           "x": mx, "y": my, "z": mz} for n in ("Klockizar Riddik", "#191043")]
    db.execute("INSERT INTO queststeps (questid, step, kind, target, x, y, z, journal, events) VALUES "
               "(102914, 2, 'reach', 'the mountain top', ?, ?, ?, ?, ?)",
               (mx, my, mz, t.RIDDIK_S2_JOURNAL_OLD, json.dumps(ev, separators=(",", ":"))))
    db.execute("INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (102914, 3, 'loot', "
               "'Klockizar Riddik', ?)", (t.RIDDIK_S3_JOURNAL_OLD,))
    for f in (t.PIRATE_FACTION, "Feria Hurkert", "Citizens of Hodstock"):
        db.execute("INSERT INTO factions VALUES (?, -500)", (f,))
    for i, name in enumerate(t.PIRATES):
        db.execute("INSERT INTO npcs VALUES (?, ?, 0, ?, 54.125, ?)", (108406 + i, name.title(), 25420.0 + i, 6560.0))
    for name, (ox, oz), _ in t.TENTS:
        db.execute("UPDATE npcs SET x_coord = ?, z_coord = ? WHERE npc_name = ?", (ox, oz, name))
    db.execute("INSERT INTO npcfaction (id, name, faction) VALUES (2962, 'feria huckert', 'Citizens of Hodstock')")
    db.execute("INSERT INTO npcs VALUES (?, 'arctic bandit', 0, ?, 54.125, ?)", (t.BANDIT_ID,) + t.BANDIT_OLD)
    db.execute("INSERT INTO quests (questid, name, giver, minlevel) VALUES (100759, 'The Redheaded Bandit', 'x', 23)")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, npcid) VALUES (100759, 2, 'loot', "
               "'arctic bandit', ?)", (t.BANDIT_ID,))
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
        self.assertIn("dry run: 36 rows would change", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_once_and_undo(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied: 36 rows", out)
        self.assertEqual(self.q("SELECT itemicon FROM itempattern WHERE itemname = 'Wolf''s Eye Agate'"),
                         [(t.GREEN_GEM,)])
        self.assertEqual(self.q("SELECT itemicon, model FROM itempattern WHERE itemname = 'Dimunitive Shortblade'"),
                         [(t.SHORT_SWORD_ICON, t.SHORT_SWORD_MODEL)])
        self.assertEqual(self.q("SELECT undead FROM npcundead WHERE name = 'abominus'"), [(1,)])
        self.assertEqual(self.q("SELECT faction FROM npcfaction WHERE name = 'feria huckert'"), [("Feria Hurkert",)])
        self.assertEqual(self.q("SELECT count(*) FROM npcfaction WHERE faction = ?", (t.PIRATE_FACTION,)), [(9,)])
        ev = json.loads(self.q("SELECT events FROM queststeps WHERE questid = 102914 AND step = 2")[0][0])
        self.assertEqual([(e["x"], e["z"]) for e in ev], [(t.HILL[0], t.HILL[2])] * 2)
        self.assertEqual(self.q("SELECT radius FROM npcwander WHERE npcid = ?", (t.BANDIT_ID,)), [(60.0,)])
        self.assertEqual(self.q(f"SELECT count(*) FROM {t.LOG}"), [(36,)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit"), [(36,)])
        code, out = run("--db", self.db, "--apply")
        self.assertIn("applied already", out)
        code, out = run("--db", self.db)  # every row already as written
        self.assertIn("dry run: 0 rows would change", out)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = ?", (t.LOG,)), [(0,)])

    def test_row_not_as_read_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcfaction SET faction = 'Citizens of Freeport' WHERE name = 'feria huckert'")
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
        db.execute("UPDATE npcs SET x_coord = 1 WHERE npc_name = 'Recon Wecon'")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--undo")
        self.assertIn("left     npcs", out)
        self.assertEqual(self.q("SELECT x_coord FROM npcs WHERE npc_name = 'Recon Wecon'"), [(1.0,)])


if __name__ == "__main__":
    unittest.main()
