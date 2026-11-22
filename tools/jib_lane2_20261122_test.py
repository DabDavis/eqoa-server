"""jib_lane2_20261122.py on a small database and a toy plan: the report writes nothing; --apply creates the new table, adds
the column, writes the inserts and updates with a gm_audit line each; a second run finds nothing to do; a row already there
another way and a column that is not the plan's before are named and left; a shard's own rows go only to that shard; --undo
takes back exactly what it wrote and leaves a row changed after it. The real plan is checked for shape."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "jib_lane2_20261122.py")
REAL = os.path.join(HERE, "jib_lane2_20261122.json")

TOY = [
    {"entry": 1, "creates": ["CREATE TABLE IF NOT EXISTS toycoins (npc_name TEXT PRIMARY KEY, coins_min INTEGER NOT NULL, "
                             "coins_max INTEGER NOT NULL, note TEXT, source TEXT)"],
     "inserts": [{"table": "toycoins", "key": ["npc_name"], "row": {"npc_name": "a", "coins_min": 1, "coins_max": 2,
                                                                    "note": "", "source": "t"}},
                 {"table": "toycoins", "key": ["npc_name"], "shards": ["live"],
                  "row": {"npc_name": "liveonly", "coins_min": 3, "coins_max": 3, "note": "", "source": "t"}}]},
    {"entry": 2, "add_columns": [{"table": "npcdialog", "column": "use",
                                  "ddl": "ALTER TABLE npcdialog ADD COLUMN use TEXT NOT NULL DEFAULT ''"}],
     "inserts": [{"table": "npcdialog", "key": ["name", "line"], "row": {"name": "Taruna", "line": "Ah ha!", "use": "ambient"}},
                 {"table": "npcdialog", "key": ["name", "line"], "row": {"name": "Other", "line": "Hi", "use": "shop"}}],
     "updates": [{"table": "quests", "where": {"questid": 7}, "set": {"decline": {"before": "", "after": "No.\nNot again."}}},
                 {"table": "quests", "where": {"questid": 8}, "set": {"decline": {"before": "", "after": "x"}}}]},
]


class T(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.db = os.path.join(self.d, "e.db")
        self.plan = os.path.join(self.d, "plan.json")
        json.dump(TOY, open(self.plan, "w"))
        db = sqlite3.connect(self.db)
        db.executescript("""CREATE TABLE npcdialog (id INTEGER PRIMARY KEY, name TEXT, line TEXT);
            INSERT INTO npcdialog (name, line) VALUES ('Other', 'Hi');
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, decline TEXT);
            INSERT INTO quests VALUES (7, ''), (8, 'changed by a GM');
            CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT,
            reverted_by INTEGER);""")
        db.commit()
        db.close()

    def run_tool(self, *args, shard="heirloom"):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--shard", shard, "--plan", self.plan, *args],
                           capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        return r.stdout

    def q(self, sql, *a):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, a).fetchall()
        finally:
            db.close()

    def test_report_apply_again_undo(self):
        out = self.run_tool()
        self.assertIn("5 to write", out)  # the table, its row, the column, Taruna's row, quest 7
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'toycoins'")[0][0], 0)
        out = self.run_tool("--apply")
        self.assertIn("applied", out)
        self.assertEqual(self.q("SELECT npc_name FROM toycoins"), [("a",)])  # not the live-only row
        self.assertEqual(self.q("SELECT use FROM npcdialog WHERE name = 'Taruna'"), [("ambient",)])
        self.assertEqual(self.q("SELECT decline FROM quests WHERE questid = 7"), [("No.\nNot again.",)])
        self.assertEqual(self.q("SELECT decline FROM quests WHERE questid = 8"), [("changed by a GM",)])
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], 3)
        again = self.run_tool()
        self.assertIn("0 to write", again)
        self.assertIn("npcdialog", again)  # Other's row is here another way (use '' not 'shop'): named, left
        self.assertIn("neither the plan's before nor after", again)
        # a GM changes the inserted row: undo leaves it
        db = sqlite3.connect(self.db)
        db.execute("UPDATE toycoins SET coins_max = 9")
        db.commit()
        db.close()
        out = self.run_tool("--undo")
        self.assertIn("left", out)
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog WHERE name = 'Taruna'")[0][0], 0)
        self.assertEqual(self.q("SELECT decline FROM quests WHERE questid = 7"), [("",)])
        self.assertEqual(self.q("SELECT coins_max FROM toycoins"), [(9,)])

    def test_live_shard_rows(self):
        self.run_tool("--apply", shard="live")
        self.assertEqual(sorted(self.q("SELECT npc_name FROM toycoins")), [("a",), ("liveonly",)])

    def test_entry_filter(self):
        self.run_tool("--apply", "--entry", "2")
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'toycoins'")[0][0], 0)
        self.assertEqual(self.q("SELECT decline FROM quests WHERE questid = 7"), [("No.\nNot again.",)])

    def test_real_plan_shape(self):
        if not os.path.exists(REAL):
            self.skipTest("no plan yet")
        plan = json.load(open(REAL))
        seen = set()
        for sec in plan:
            self.assertNotIn(sec["entry"], seen)
            seen.add(sec["entry"])
            for ddl in sec.get("creates", []):
                self.assertTrue(ddl.startswith("CREATE TABLE IF NOT EXISTS "), ddl)
            for ins in sec.get("inserts", []):
                for k in ins["key"]:
                    self.assertIn(k, ins["row"])
                for s in ins.get("shards", []):
                    self.assertIn(s, ("heirloom", "live"))
            for up in sec.get("updates", []):
                for ch in up["set"].values():
                    self.assertNotEqual(ch["before"], ch["after"])
            text = json.dumps(sec)
            self.assertNotIn("|", text.replace("\\u007c", ""), "a run of | kills a PS2's TextBox")


if __name__ == "__main__":
    unittest.main()
