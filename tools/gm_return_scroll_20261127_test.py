import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from gm_return_scroll_20261127 import DESCRIPTION, NAME, ORIGINAL_ID, TEST_ID
from scroll_of_returning_20261126_test import ITEM, SCHEMA


class TestGMReturnScroll(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.path = Path(temp.name) / "game.db"
        self.db = sqlite3.connect(self.path)
        self.addCleanup(self.db.close)
        self.db.executescript(SCHEMA)
        self.db.execute(ITEM, (ORIGINAL_ID, -607649328, "Scroll of Returning"))
        self.db.execute("INSERT INTO npcitem (patternid, npcid) VALUES (?, 1)", (ORIGINAL_ID,))
        self.db.commit()

    def run_tool(self, *args, code=0):
        script = Path(__file__).with_name("gm_return_scroll_20261127.py")
        result = subprocess.run([sys.executable, "-B", script, "--db", self.path, *args], capture_output=True, text=True)
        self.assertEqual(result.returncode, code, result.stderr)
        return result.stdout + result.stderr

    def test_one_audited_clone_without_a_vendor(self):
        self.assertIn("Dry run passed", self.run_tool())
        self.assertEqual(self.db.execute("SELECT patternid FROM itempattern").fetchall(), [(ORIGINAL_ID,)])
        self.assertIn("audit 1", self.run_tool("--apply"))
        cursor = self.db.execute("SELECT * FROM itempattern ORDER BY patternid")
        columns = [column[0] for column in cursor.description]
        original, installed = (dict(zip(columns, record)) for record in cursor)
        self.assertEqual(installed, dict(original, patternid=TEST_ID, itemname=NAME, itemdesc=DESCRIPTION, trade=1, itemcost=0))
        self.assertEqual(self.db.execute("SELECT patternid FROM npcitem").fetchall(), [(ORIGINAL_ID,)])
        self.assertIn("already exists", self.run_tool("--apply", code=1))
        self.assertIn("audit 2", self.run_tool("--undo"))
        self.assertEqual(self.db.execute("SELECT patternid FROM itempattern").fetchall(), [(ORIGINAL_ID,)])

    def test_refuses_existing_stock_and_protects_held_copies(self):
        self.db.execute("INSERT INTO npcitem (patternid, npcid) VALUES (?, 1)", (TEST_ID,))
        self.db.commit()
        self.assertIn("npcitem references", self.run_tool("--apply", code=1))
        self.db.execute("DELETE FROM npcitem WHERE patternid = ?", (TEST_ID,))
        self.db.commit()
        self.run_tool("--apply")
        self.db.execute("INSERT INTO charinventory VALUES (1, 1, 1, 0, 0, ?, -1, 0, 0)", (TEST_ID,))
        self.db.commit()
        self.assertIn("charinventory references", self.run_tool("--undo", code=1))
        self.assertEqual(self.db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (TEST_ID,)).fetchone(), (NAME,))


if __name__ == "__main__":
    unittest.main()
