#!/usr/bin/env python3
"""wisp_kit_20261111.py on a small database: the report writes nothing; --apply writes the Wisp's five rows (races
1 << 31) with a gm_audit line each; a second apply does nothing; --undo puts cmcatalog back exactly. The controls: an
id taken, a glamour not named as written, another row on the Wisp's bit and no Wisp in racedefs are refused, writing
nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import wisp_kit_20261111 as wk  # noqa: E402

TOOL = os.path.join(HERE, "wisp_kit_20261111.py")

CMCATALOG = """CREATE TABLE cmcatalog (
    id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
    requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '',
    adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '',
    remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'gm', require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '')"""


def make(path, wisp="Wisp", glamour="Glamour: Elf"):
    db = sqlite3.connect(path)
    db.executescript(f"""
        {CMCATALOG};
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
        CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO racedefs VALUES (30, 'Jal''Raeth'), (31, '{wisp}');
        INSERT INTO cmcatalog (id, category, races, mastery, name) VALUES (1001, 1, 64, 'Blood of the North', 'Blood of the North'),
            (2131, 4, 536870912, 'Stoneguard', 'Stoneguard'), (2201, 2, 2047, 'Protected Faith', 'Protected Faith 1');
    """)
    for name, sid, _ in wk.GLAMOURS:
        db.execute("INSERT INTO spellpattern VALUES (?, ?)", (sid, glamour if sid == 503 else name))
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return [r for r in db.execute("SELECT * FROM cmcatalog ORDER BY id")]
    finally:
        db.close()


class WispKitTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def test_apply_and_undo(self):
        make(self.path)
        before = dump(self.path)
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 5 rows", out)
        self.assertEqual(dump(self.path), before, "the report wrote")
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        for r in wk.ROWS:
            got = wk.row_of(db, r["id"])
            self.assertEqual({k: got[k] for k in r}, r)
            self.assertEqual(got["races"], 1 << 31)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail = ?", (wk.DETAIL,)).fetchone()[0], 5)
        tf = wk.row_of(db, 2142)
        self.assertEqual(tf["add_spells"], "496,503,507,517,524,531,1793")
        self.assertTrue(tf["adds"].startswith("Glamour: Human@1; Glamour: Elf@4"))
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied already", out)
        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.path), before, "the undo")
        db = sqlite3.connect(self.path)
        self.assertFalse(wk.table(db, wk.LOG))
        db.close()

    def refused(self, want):
        before = dump(self.path)
        code, out = self.run_tool("--apply")
        self.assertNotEqual(code, 0, out)
        self.assertIn(want, out)
        self.assertEqual(dump(self.path), before)

    def test_refusals(self):
        make(self.path)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO cmcatalog (id, name, mastery) VALUES (2143, 'Someone', 'Else')")
        db.commit()
        db.close()
        self.refused("cmcatalog 2143 is taken")
        os.remove(self.path)
        make(self.path)
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO cmcatalog (id, category, races, name) VALUES (2300, 1, ?, 'Old Wisp')", (1 << 31,))
        db.commit()
        db.close()
        self.refused("has the Wisp's bit already")
        os.remove(self.path)
        make(self.path, glamour="Illusion: Elf")
        self.refused("not 'Glamour: Elf'")
        os.remove(self.path)
        make(self.path, wisp="Gargoyle")
        self.refused("not the Wisp")


if __name__ == "__main__":
    unittest.main()
