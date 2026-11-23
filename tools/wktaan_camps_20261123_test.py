#!/usr/bin/env python3
"""tools/wktaan_camps_20261123.py: the camps as read are renamed and topped up once, a second apply writes nothing,
a camp not as read (or the other shard's) is refused with nothing written, and --undo puts npcs, npcrespawn, npcwander
and the npcs sequence back exactly (a row changed since is left and the log kept).

    python3 -E -s tools/wktaan_camps_20261123_test.py
"""
import contextlib
import io
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import wktaan_camps_20261123 as w  # noqa: E402


def make(path, shard="heirloom"):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL,
            y_coord REAL, z_coord REAL, facing INTEGER, npc_level INTEGER, npc_type INTEGER, modelid INTEGER,
            dt_kill TEXT, npc_id INTEGER);
        CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
    """)
    for i, (camp, name, level, x, z) in sorted(w.ROWS.items()):
        if shard == "live" and i in w.LIVE_PLACES:
            x, z = w.LIVE_PLACES[i]
        model = 832107953 if "prophet" in name else 939332601
        db.execute("INSERT INTO npcs VALUES (?, ?, 0, ?, 60, ?, 0, ?, 0, ?, NULL, ?)", (i, name, x, z, level, model, i))
        db.execute("INSERT INTO npcrespawn VALUES (?, 90, 'named', 't')", (i,))
    db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level) VALUES ('emerald drake', 0, 1, 2, 3, 47)")
    db.execute("INSERT INTO npcwander VALUES (107204, 45, '', 't')")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = w.main(list(args))
    return code, out.getvalue()


def digests(path):
    db = w.connect(path, ro=True)
    try:
        return {t: w.digest(db, t) for t in ("npcs", "npcrespawn", "npcwander")}
    finally:
        db.close()


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")

    def tearDown(self):
        shutil.rmtree(self.dir)

    def names(self):
        db = sqlite3.connect(self.db)
        try:
            return dict(db.execute("SELECT npc_name, count(*) FROM npcs WHERE npc_name LIKE '%ktaan%' OR npc_name LIKE 'Deathfist%' GROUP BY 1"))
        finally:
            db.close()

    def test_apply_again_undo(self):
        for shard in ("heirloom", "live"):
            with self.subTest(shard=shard):
                if os.path.exists(self.db):
                    os.remove(self.db)
                make(self.db, shard)
                before = digests(self.db)
                code, out = run("--db", self.db, "--shard", shard)
                self.assertEqual(code, 0, out)
                self.assertIn("44 rows would be written", out)
                self.assertEqual(digests(self.db), before, "a dry run writes nothing")
                code, out = run("--db", self.db, "--shard", shard, "--apply")
                self.assertEqual(code, 0, out)
                self.assertIn("44 rows", out)
                self.assertEqual(self.names(), {"Wktaan prophet": 25, "Wktaan reaver": 26, "Wktaan raider": 9,
                                                "Wktaan crafter": 1, "Warchief Wktaan": 1})
                db = sqlite3.connect(self.db)
                new = db.execute("SELECT id, npc_name, x_coord, y_coord, z_coord, facing, npc_level, modelid FROM npcs "
                                 "WHERE id > 107222 AND npc_name LIKE 'Wktaan%' ORDER BY id").fetchall()
                self.assertEqual(len(new), len(w.ADDS))
                for row, (camp, kind, tid, x, y, z, f) in zip(new, w.ADDS):
                    self.assertEqual(row[1:6], (kind, x, y, z, f))
                    self.assertEqual(row[6], w.ROWS[tid][2], "the copy's level is its template's")
                    self.assertEqual(db.execute("SELECT seconds FROM npcrespawn WHERE npc_row = ?", (row[0],)).fetchone(), (90.0,))
                    self.assertIsNone(db.execute("SELECT 1 FROM npcwander WHERE npcid = ?", (row[0],)).fetchone(), "the adds stand")
                self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 44)
                db.close()
                applied = digests(self.db)
                code, out = run("--db", self.db, "--shard", shard, "--apply")
                self.assertEqual(code, 0, out)
                self.assertIn("nothing written", out)
                self.assertEqual(digests(self.db), applied)
                code, out = run("--db", self.db, "--shard", shard, "--undo")
                self.assertEqual(code, 0, out)
                self.assertEqual(digests(self.db), before)
                db = sqlite3.connect(self.db)
                self.assertIsNone(db.execute("SELECT 1 FROM sqlite_master WHERE name LIKE 'wktaan_camps%'").fetchone())
                db.close()

    def test_refusals(self):
        make(self.db, "live")
        before = digests(self.db)
        code, out = run("--db", self.db, "--shard", "heirloom", "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("refused", out)
        self.assertEqual(digests(self.db), before, "the other shard's places: nothing written")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET npc_name = 'Wktaan reaver' WHERE id = 107153")  # a GM's rename
        db.commit()
        db.close()
        before = digests(self.db)
        code, out = run("--db", self.db, "--shard", "live", "--apply")
        self.assertEqual(code, 2, out)
        self.assertIn("row 107153", out)
        self.assertEqual(digests(self.db), before)

    def test_undo_leaves_a_changed_row(self):
        make(self.db)
        run("--db", self.db, "--shard", "heirloom", "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET npc_level = 41 WHERE id = 107160")  # a GM's edit since
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--shard", "heirloom", "--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("left", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT npc_name, npc_level FROM npcs WHERE id = 107160").fetchone(), ("Wktaan reaver", 41))
        self.assertEqual(db.execute(f"SELECT count(*) FROM {w.LOG}").fetchone()[0], 1, "the log keeps the row left")
        db.close()

    def test_adds_fill_thin_camps(self):
        per = {}
        for camp, *_ in w.ADDS:
            per[camp] = per.get(camp, 0) + 1
        for camp in w.CAMPS:
            have = sum(1 for r in w.ROWS.values() if r[0] == camp)
            self.assertEqual(have + per.get(camp, 0), max(have, w.MIN_CAMP), camp)
            self.assertEqual(len(w.wanted_kinds(camp)), per.get(camp, 0), camp)
        self.assertEqual(sum(1 for r in w.ROWS.values() if r[1] == "Warchief Wktaan"), 1)


if __name__ == "__main__":
    unittest.main()
