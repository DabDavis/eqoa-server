#!/usr/bin/env python3
"""quest_fixes_20261122.py on a small database: apply, a second apply that writes nothing, undo exact; a row not as
read refuses the whole run."""
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import quest_fixes_20261122 as tool  # noqa: E402

TRIG = [{"do": "walk", "npc": "Jarek N`Toxis", "x": 1, "y": 2, "z": 3}, {"do": "hide", "npc": "Jarek N`Toxis", "minutes": 5}]


def run(*args):
    out = io.StringIO()
    with redirect_stdout(out):
        rc = tool.main(list(args))
    return rc, out.getvalue()


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.executescript("""CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER,
            username TEXT, via TEXT, ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
            CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, offer TEXT);
            CREATE TABLE queststeps (questid INTEGER, step INTEGER, target TEXT, rewarditems TEXT DEFAULT '',
                events TEXT DEFAULT '', journal TEXT DEFAULT '', x REAL DEFAULT 0, z REAL DEFAULT 0, PRIMARY KEY (questid, step));
            CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT, steps TEXT);""")
        db.executemany("INSERT INTO quests VALUES (?, ?, ?, ?)", [
            (101303, "The Necrologist of Rain", "Singe", ""), (101285, "Captain Scilond's Scouts", "Scilond", ""),
            (100463, "Jarek's Creation", "J`Narus", ""), (103740, "The Stone Way Back", "Waysender Hrumgar", tool.HRUMGAR_OLD), (100748, "The Scribe's Inks", "Scribe", "")])
        db.executemany("INSERT INTO queststeps (questid, step, target) VALUES (?, ?, ?)", [
            (101303, 12, "manifestation of rain"), (101285, 22, "deadly corrosion"), (100463, 3, "fungus cockroach"), (100748, 2, "Talya Montar")])
        db.execute("UPDATE queststeps SET journal = ? WHERE questid = 100748", (tool.STEPS[-1][4],))
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL, z_coord REAL)")
        db.execute("INSERT INTO npcs VALUES (162789, 'Talya Montar', 0, 16889.75390625, 207.68852, 15236.33203125)")
        db.execute("INSERT INTO gmtriggers VALUES (143, ?, ?)", (tool.TRIGGER, json.dumps(TRIG)))
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def snap(self):
        db = sqlite3.connect(self.db)
        out = [db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in ("quests", "queststeps", "gmtriggers", "npcs")]
        db.close()
        return out

    def test_apply_again_undo(self):
        before = self.snap()
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT rewarditems FROM queststeps WHERE questid = 101303").fetchone()[0], "701464")
        self.assertEqual(db.execute("SELECT x, z FROM queststeps WHERE questid = 100463").fetchone(), (24303.0, 9580.0))
        steps = json.loads(db.execute("SELECT steps FROM gmtriggers").fetchone()[0])
        self.assertEqual((steps[0]["pace"], steps[1]["minutes"]), (4.0, 1))
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 103740").fetchone()[0], tool.HRUMGAR_NEW)
        self.assertEqual(db.execute("SELECT x_coord, y_coord, z_coord FROM npcs").fetchone(), (16896.5, 225.25, 15238.97))
        db.close()
        after = self.snap()
        self.assertIn("nothing to write", run("--db", self.db, "--apply")[1])
        self.assertEqual(self.snap(), after)
        self.assertIn("10 changes put back", run("--db", self.db, "--undo")[1])
        self.assertEqual(self.snap(), before)

    def test_refuses_a_row_not_as_read(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE queststeps SET events = 'x' WHERE questid = 101285")
        db.commit()
        db.close()
        before = self.snap()
        rc, out = run("--db", self.db, "--apply")
        self.assertEqual(rc, 1)
        self.assertEqual(self.snap(), before)


if __name__ == "__main__":
    unittest.main()
