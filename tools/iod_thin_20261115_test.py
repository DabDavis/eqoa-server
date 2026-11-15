#!/usr/bin/env python3
"""iod_thin_20261115.py on a small database: the report writes nothing; --apply sets the templates' melee_pct by the
shard's warrior share, maps the common names, holds 60% of each name's spawns by npcspawnonly "#row" (a camp keeps its
share, the rest spread), leaves out a spawn-only name, a rare's placeholder and a row off the island; a second --apply
does nothing; --undo puts every row back exactly."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import iod_thin_20261115 as it  # noqa: E402

COLS = ", ".join(f"{k} {'TEXT' if isinstance(v, str) else 'REAL' if isinstance(v, float) else 'INTEGER'}"
                 for k, v in it.TEMPLATE_ROWS["iod native"].items() if k != "name")
SCHEMA = f"""
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, z_coord REAL, world INTEGER);
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, {COLS});
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
                          source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcrare (npc_row INTEGER PRIMARY KEY, rare TEXT NOT NULL, chance REAL NOT NULL DEFAULT 10,
                      note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE balanceclasslevel (class INTEGER, level INTEGER, melee_taken_pct REAL, PRIMARY KEY (class, level));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
                       username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL,
                       tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT,
                       detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""
TABLES = ("npcs", "classtemplates", "npctemplate", "npcspawnonly", "npcrare")


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = it.main(list(args))
    return rc, out.getvalue()


class IoDThinTest(unittest.TestCase):
    def make(self, share, heirloom):
        fd, path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        self.addCleanup(os.remove, path)
        db = sqlite3.connect(path)
        db.executescript(SCHEMA)
        db.execute("INSERT INTO balanceclasslevel VALUES (0, 60, ?)", (share,))
        if heirloom:
            for r in it.TEMPLATE_ROWS.values():
                db.execute(f"INSERT INTO classtemplates ({', '.join(r)}) VALUES ({', '.join('?' * len(r))})", list(r.values()))
            for n, t in it.COMMON.items():
                db.execute("INSERT INTO npctemplate (name, template) VALUES (?, ?)", (n, t))
        else:
            db.execute("INSERT INTO npctemplate (name, template, source) VALUES ('wicker man', 'roamer', 'roamer-seed')")
        # a camp of 10 breath stealers, 10 lone ones across the island, a rare's placeholder, one off the island
        for i in range(10):
            db.execute("INSERT INTO npcs (npc_name, x_coord, z_coord, world) VALUES ('breath stealer', ?, 3000, 0)",
                       (24500 + 5 * i,))
        for i in range(10):
            db.execute("INSERT INTO npcs (npc_name, x_coord, z_coord, world) VALUES ('breath stealer', ?, 2200, 0)",
                       (24100 + 150 * i,))
        db.execute("INSERT INTO npcs (npc_name, x_coord, z_coord, world) VALUES ('breath stealer', 25000, 3500, 0)")
        db.execute("INSERT INTO npcrare (npc_row, rare) VALUES (last_insert_rowid(), 'a rare')")
        db.execute("INSERT INTO npcs (npc_name, x_coord, z_coord, world) VALUES ('breath stealer', 100, 100, 0)")
        db.execute("INSERT INTO npcs (npc_name, x_coord, z_coord, world) VALUES ('Rotting Disease', 25550, 3098, 0)")
        db.execute("INSERT INTO npcspawnonly (name) VALUES ('Rotting Disease')")
        for i in range(5):
            db.execute("INSERT INTO npcs (npc_name, x_coord, z_coord, world) VALUES ('wicker man', ?, 2800, 0)",
                       (24800 + 3 * i,))
        db.execute("INSERT INTO npcs (npc_name, x_coord, z_coord, world) VALUES ('dead rot', 25000, 2500, 0)")
        db.commit()
        db.close()
        return path

    def dump(self, path):
        db = sqlite3.connect(path)
        out = [db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in TABLES]
        db.close()
        return out

    def check(self, share, heirloom, melee):
        path = self.make(share, heirloom)
        start = self.dump(path)
        rc, out = run("--db", path)
        self.assertEqual(rc, 0)
        self.assertEqual(self.dump(path), start, "the report wrote")
        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        self.assertEqual(db.execute("SELECT DISTINCT melee_pct FROM classtemplates").fetchall(), [(melee,)])
        self.assertEqual(db.execute("SELECT template FROM npctemplate WHERE name = 'wicker man'").fetchone()[0],
                         "iod native")
        self.assertEqual(db.execute("SELECT template FROM npctemplate WHERE name = 'flesh crawler'").fetchone()[0],
                         "iod crawler")
        held = {int(r[0][1:]) for r in db.execute("SELECT name FROM npcspawnonly WHERE source = ?", (it.SRC,))}
        names = dict(db.execute("SELECT id, npc_name FROM npcs"))
        by = {}
        for i in held:
            by[names[i]] = by.get(names[i], 0) + 1
        # 20 breath stealers on the island: 8 kept; 5 wicker men: 2 kept; one dead rot: kept
        self.assertEqual(by, {"breath stealer": 12, "wicker man": 3})
        camp = [i for i in range(1, 11) if i not in held]
        self.assertEqual(len(camp), 4, "the camp of 10 keeps its 40%")
        self.assertEqual(len([i for i in range(11, 21) if i not in held]), 4, "the lone ones keep theirs")
        self.assertNotIn(21, held, "a rare's placeholder")
        self.assertNotIn(22, held, "off the island")
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 3 if not heirloom else 2)
        db.close()
        applied = self.dump(path)
        rc, out = run("--db", path, "--apply")
        self.assertIn("nothing to do", out)
        self.assertEqual(self.dump(path), applied, "a second --apply wrote")
        rc, out = run("--db", path, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.dump(path), start, "--undo is not exact")

    def test_heirloom(self):
        self.check(85.0, True, 370)

    def test_live(self):
        self.check(100.0, False, 315)

    def test_other_share_refused(self):
        path = self.make(90.0, True)
        with self.assertRaises(SystemExit):
            run("--db", path, "--apply")


if __name__ == "__main__":
    unittest.main()
