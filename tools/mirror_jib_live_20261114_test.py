#!/usr/bin/env python3
"""mirror_jib_live_20261114.py on two small databases: the report writes nothing; --apply makes Jibrish's placement
on Heirloom as a copy of Heirloom's own source row (its model) with what differs on Live from Live's source (spot,
level 24), its wander, npc_mirror and an inbox line, each row a gm_audit line; his deleted placement and another GM's
are left; a second apply finds nothing to do; --undo deletes exactly what was made; Live is never written."""
import hashlib
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "mirror_jib_live_20261114.py")

SCHEMA = """
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT UNIQUE NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL,
    facing INTEGER, world INTEGER, modelid INTEGER, npc_level INTEGER, npc_id INTEGER);
CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO accountinfo VALUES (1, 'dab'), (10, 'jibrish'), (11, 'other');
"""


def run(*args):
    p = subprocess.run([sys.executable, "-E", "-s", TOOL] + list(args), capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def dump(path):
    db = sqlite3.connect(path)
    out = [r for t in ("npcs", "npcwander") for r in db.execute(f"SELECT '{t}', * FROM {t} ORDER BY 2")]
    db.close()
    return out


class MirrorJibLive(unittest.TestCase):
    def setUp(self):
        d = tempfile.mkdtemp()
        self.live, self.hl = os.path.join(d, "live.db"), os.path.join(d, "hl.db")
        db = sqlite3.connect(self.live)
        db.executescript(SCHEMA + """
            INSERT INTO npcs VALUES (107931, 'crocodile', 24821.4, 8, 29131, 118, 0, 40080030, 37, 107931),
                (162787, 'crocodile', 25362.6, 69.9, 17326.0, 173, 0, 40080030, 24, 107931),
                (162790, 'crocodile', 1, 2, 3, 0, 0, 40080030, 37, 107931);
            INSERT INTO npcwander VALUES (162787, 300, 'a GM''s roamer', 'gm');
            INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, detail) VALUES
                ('t', 10, 'jibrish', 'game', '', 'insert', 'npcs', '{"id":162787}', '/place in the game: crocodile (row 107931), a copy of NPC 107931'),
                ('t', 10, 'jibrish', 'game', '', 'insert', 'npcs', '{"id":162788}', '/place in the game: crocodile (row 107931), a copy of NPC 107931'),
                ('t', 11, 'other', 'game', '', 'insert', 'npcs', '{"id":162790}', '/clone in the game: crocodile (row 107931), a copy of NPC 107931');
        """)
        db.close()
        db = sqlite3.connect(self.hl)
        db.executescript(SCHEMA + """
            INSERT INTO npcs VALUES (107931, 'crocodile', 24821.4, 8, 29131, 118, 0, 777, 37, 107931),
                (168972, 'filler', 0, 0, 0, 0, 0, 1, 1, 1);
        """)
        db.close()
        with open(self.live, "rb") as f:
            self.live_md5 = hashlib.md5(f.read()).hexdigest()

    def test_apply_again_undo(self):
        before = dump(self.hl)
        code, out = run("--live", self.live, "--heirloom", self.hl)
        self.assertEqual(code, 0, out)
        self.assertIn("1 rows to make", out)
        self.assertIn("Live row 162788", out)
        self.assertIn("deleted on Live since", out)
        self.assertNotIn("162790", out)
        self.assertEqual(dump(self.hl), before)

        code, out = run("--live", self.live, "--heirloom", self.hl, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("Live row 162787 -> Heirloom row 168973", out)
        db = sqlite3.connect(self.hl)
        r = db.execute("SELECT npc_name, x_coord, z_coord, facing, modelid, npc_level FROM npcs WHERE id = 168973").fetchone()
        self.assertEqual(r, ("crocodile", 25362.6, 17326.0, 173, 777, 24))  # Heirloom's model, Live's spot and level
        self.assertEqual(db.execute("SELECT radius FROM npcwander WHERE npcid = 168973").fetchone(), (300,))
        self.assertEqual(db.execute("SELECT from_id, to_id FROM npc_mirror").fetchall(), [(162787, 168973)])
        self.assertEqual(db.execute("SELECT npc_row, gone FROM npc_mirror_inbox").fetchall(), [(168973, 0)])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail LIKE 'mirrored from Live: jibrish''s /place%'").fetchone(), (2,))
        db.close()

        code, out = run("--live", self.live, "--heirloom", self.hl, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)

        code, out = run("--live", self.live, "--heirloom", self.hl, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.hl), before)
        db = sqlite3.connect(self.hl)
        self.assertEqual(db.execute("SELECT count(*) FROM npc_mirror").fetchone(), (0,))
        self.assertEqual(db.execute("SELECT npc_row, gone FROM npc_mirror_inbox ORDER BY id DESC LIMIT 1").fetchone(), (168973, 1))
        db.close()
        with open(self.live, "rb") as f:
            self.assertEqual(hashlib.md5(f.read()).hexdigest(), self.live_md5)


if __name__ == "__main__":
    unittest.main()
