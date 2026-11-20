"""cleric_paths_live_20261120.py on a small database: the report writes nothing; --apply writes each planned column found as
the plan's before, with a gm_audit line and a log row; a second run finds nothing to do; a step changed since is skipped;
--undo puts back exactly what it wrote and leaves a column changed after it."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "cleric_paths_live_20261120.py")
PLAN = json.load(open(os.path.join(HERE, "cleric_paths_live_20261120.json")))


class T(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.db = os.path.join(self.d, "e.db")
        db = sqlite3.connect(self.db)
        db.executescript("""CREATE TABLE queststeps (questid INTEGER, step INTEGER, target TEXT, journal TEXT,
            choices TEXT, spells TEXT, PRIMARY KEY (questid, step));
            CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT,
            reverted_by INTEGER);""")
        for p in PLAN:
            row = {"choices": "{}", "spells": ""}
            for c, ch in p["set"].items():
                row[c] = ch["before"]
            db.execute("INSERT INTO queststeps VALUES (?, ?, ?, ?, ?, ?)",
                       (p["questid"], p["step"], p["target"], p["journal"], row["choices"], row["spells"]))
        db.commit()
        db.close()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, *args], capture_output=True, text=True)

    def q(self, sql, *a):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def snap(self):
        return self.q("SELECT questid, step, choices, spells FROM queststeps ORDER BY questid, step")

    def test_report_apply_again_undo(self):
        before = self.snap()
        self.assertIn("13 to write", self.run_tool().stdout)
        self.assertEqual(self.snap(), before)
        self.assertEqual(self.run_tool("--apply").returncode, 0)
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 13)
        self.assertEqual(self.q("SELECT count(*) FROM queststeps WHERE choices LIKE '%Elemental%'")[0][0], 0)
        self.assertEqual(self.q("SELECT spells FROM queststeps WHERE questid = 100963")[0][0], "")
        self.assertIn("0 to write, 13 already", self.run_tool().stdout)
        self.assertIn("undone: 13", self.run_tool("--undo").stdout)
        self.assertEqual(self.snap(), before)

    def test_changed_rows_are_left(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE queststeps SET spells = '1' WHERE questid = 100963")
        db.commit()
        db.close()
        self.assertIn("12 to write", self.run_tool("--apply").stdout)
        db = sqlite3.connect(self.db)
        db.execute("UPDATE queststeps SET choices = 'a GM edit' WHERE questid = 100020")
        db.commit()
        db.close()
        out = self.run_tool("--undo").stdout
        self.assertIn("undone: 11 column(s), 1 left", out)
        self.assertEqual(self.q("SELECT choices FROM queststeps WHERE questid = 100020")[0][0], "a GM edit")


if __name__ == "__main__":
    unittest.main()
