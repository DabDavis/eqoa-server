#!/usr/bin/env python3
"""boxcanyon_reach_20261115.py on a small database: the report writes nothing; --apply moves the rows of MOVES and
removes those of REMOVES with their npctemplate and npcloot "#n" rows, one gm_audit line; a second --apply does
nothing; --undo puts every row back exactly. A row found by name and place (another shard's id) is moved too; a row a
GM moved since is left alone."""
import os
import sqlite3
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
import sys  # noqa: E402

sys.path.insert(0, HERE)
import boxcanyon_reach_20261115 as bc  # noqa: E402

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL,
                   world INTEGER, npc_level INTEGER);
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
                          source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
                      rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
                      PRIMARY KEY (npc_name, tablename));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
                       username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL,
                       tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT,
                       detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


def dump(path):
    db = sqlite3.connect(path)
    out = [db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in ("npcs", "npctemplate", "npcloot")]
    db.close()
    return out


class BoxCanyonReachTest(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        self.move = bc.MOVES[0]       # by its id
        self.moved_other = bc.MOVES[1]  # another shard's id: found by name and place
        self.gm_moved = bc.MOVES[2]   # a GM moved it since
        self.remove = bc.REMOVES[0]
        rid, name, frm = self.move[:3]
        db.execute("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, 14, 61)", (rid, name, *frm))
        rid, name, frm = self.moved_other[:3]
        db.execute("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, 14, 61)", (rid + 500000, name, *frm))
        rid, name, frm = self.gm_moved[:3]
        db.execute("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, 14, 61)", (rid, name, frm[0] + 5, frm[1], frm[2]))
        rid, name, frm = self.remove[:3]
        db.execute("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, 14, 61)", (rid, name, *frm))
        db.execute("INSERT INTO npctemplate (name, template) VALUES (?, 'dungeon trash')", (f"#{rid}",))
        db.execute("INSERT INTO npcloot (npc_name, tablename) VALUES (?, 'dungeon boxcanyon trash')", (f"#{rid}",))
        db.execute("INSERT INTO npcs VALUES (1, 'a bystander', 1, 2, 3, 0, 1)")
        db.close()

    def tearDown(self):
        os.unlink(self.path)

    def test_apply_and_undo(self):
        before = dump(self.path)
        self.assertEqual(bc.main(["--db", self.path]), 0)
        self.assertEqual(dump(self.path), before, "the report wrote")
        self.assertEqual(bc.main(["--db", self.path, "--apply"]), 0)
        db = sqlite3.connect(self.path)
        to = self.move[3]
        self.assertEqual(db.execute("SELECT x_coord, y_coord, z_coord FROM npcs WHERE id = ?", (self.move[0],)).fetchone(),
                         to)
        self.assertEqual(db.execute("SELECT x_coord, y_coord, z_coord FROM npcs WHERE id = ?",
                                    (self.moved_other[0] + 500000,)).fetchone(), self.moved_other[3])
        self.assertNotEqual(db.execute("SELECT x_coord FROM npcs WHERE id = ?", (self.gm_moved[0],)).fetchone()[0],
                            self.gm_moved[3][0], "a GM's move was overwritten")
        rid = self.remove[0]
        self.assertIsNone(db.execute("SELECT 1 FROM npcs WHERE id = ?", (rid,)).fetchone())
        self.assertIsNone(db.execute("SELECT 1 FROM npctemplate WHERE name = ?", (f"#{rid}",)).fetchone())
        self.assertIsNone(db.execute("SELECT 1 FROM npcloot WHERE npc_name = ?", (f"#{rid}",)).fetchone())
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 1)
        db.close()
        after = dump(self.path)
        self.assertEqual(bc.main(["--db", self.path, "--apply"]), 0)
        self.assertEqual(dump(self.path), after, "a second apply wrote")
        self.assertEqual(bc.main(["--db", self.path, "--undo"]), 0)
        self.assertEqual(dump(self.path), before, "the undo is not exact")


if __name__ == "__main__":
    unittest.main()
