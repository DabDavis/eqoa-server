#!/usr/bin/env python3
"""tools/seed_town_dialog.py against a made-up database and lines: the rows
a file's blocks make (an absent field any), the table made when the server
has not made it, a second --apply refused, a file with a bad role refused,
and --undo taking back each row still as written (a GM's change kept). No
game data.

    python3 tools/seed_town_dialog_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_town_dialog.py")


class SeedTownDialogSex(unittest.TestCase):
    """The sex column: a seed logged before it (the table made without it) is
    taken back whole, a block's sex is written, and a sex that is neither
    "male" nor "female" is refused."""

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE npcfaction (id INTEGER PRIMARY KEY, name TEXT, faction TEXT)")
        db.execute("""CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, town TEXT NOT NULL DEFAULT '',
            role TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', mood TEXT NOT NULL DEFAULT '',
            race INTEGER NOT NULL DEFAULT -1, line TEXT NOT NULL, weight INTEGER NOT NULL DEFAULT 1,
            source TEXT NOT NULL DEFAULT 'gm')""")
        db.execute("CREATE TABLE npcdialog_seed (id INTEGER PRIMARY KEY, row TEXT NOT NULL)")
        old = {"town": "Freeport", "role": "citizen", "name": "", "mood": "", "race": -1,
               "line": "My husband works the docks.", "weight": 1}
        db.execute("INSERT INTO npcdialog (id, town, role, line, source) VALUES (1, 'Freeport', 'citizen', ?, 'ours')", (old["line"],))
        db.execute("INSERT INTO npcdialog_seed VALUES (1, ?)", (json.dumps(old, sort_keys=True),))
        db.commit()
        db.close()
        self.lines = os.path.join(self.dir.name, "lines.json")

    def tearDown(self):
        self.dir.cleanup()

    def tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--lines", self.lines, "--no-backup", *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def q(self, sql):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql).fetchall()
        finally:
            c.close()

    def test_undo_then_apply_with_sex(self):
        self.assertIn("took back 1 lines", self.tool("--undo").stdout)
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog"), [(0,)])
        with open(self.lines, "w") as f:
            json.dump({"blocks": [{"town": "Freeport", "role": "citizen", "sex": "female", "say": ["My husband works the docks."]},
                                  {"town": "Freeport", "role": "citizen", "sex": "male", "say": ["My wife works the docks."]}]}, f)
        self.tool("--apply")
        self.assertEqual(self.q("SELECT sex, line FROM npcdialog ORDER BY id"),
                         [("female", "My husband works the docks."), ("male", "My wife works the docks.")])

    def test_a_bad_sex_is_refused(self):
        with open(self.lines, "w") as f:
            json.dump({"blocks": [{"sex": "lady", "say": ["Hello."]}]}, f)
        self.tool("--undo")
        r = self.tool("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("sex 'lady'", r.stdout)


class SeedTownDialog(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute("CREATE TABLE npcfaction (id INTEGER PRIMARY KEY, name TEXT, faction TEXT)")
        db.executemany("INSERT INTO npcfaction (name, faction) VALUES (?, ?)",
                       [("a", "Citizens of Testvale"), ("b", "The Qeynos Guard")])
        db.commit()
        db.close()
        self.lines = os.path.join(self.dir.name, "lines.json")
        self.write({"blocks": [
            {"role": "guard", "say": ["Move along.", "Halt."]},
            {"town": "Testvale", "mood": "warm", "race": 3, "weight": 2, "say": ["Welcome home, %p."]},
        ]})

    def tearDown(self):
        self.dir.cleanup()

    def write(self, d):
        with open(self.lines, "w") as f:
            json.dump(d, f)

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--lines", self.lines, "--no-backup", *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def q(self, sql):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql).fetchall()
        finally:
            c.close()

    def test_apply_and_undo(self):
        out = self.run_tool().stdout
        self.assertIn("3 lines, 1 towns", out)
        self.assertIn("towns with no lines of their own (they say the generic ones): Qeynos", out)
        self.run_tool("--apply")
        self.assertEqual(self.q("SELECT town, role, name, mood, race, line, weight, source FROM npcdialog ORDER BY id"), [
            ("", "guard", "", "", -1, "Move along.", 1, "ours"),
            ("", "guard", "", "", -1, "Halt.", 1, "ours"),
            ("Testvale", "", "", "warm", 3, "Welcome home, %p.", 2, "ours"),
        ])
        self.assertNotEqual(self.run_tool("--apply", ok=False).returncode, 0)
        c = sqlite3.connect(self.db)
        c.execute("UPDATE npcdialog SET line = 'Halt, who goes there?' WHERE line = 'Halt.'")  # a GM's change
        c.commit()
        c.close()
        self.assertIn("took back 2 lines; 1 a GM changed", self.run_tool("--undo").stdout)
        self.assertEqual(self.q("SELECT line FROM npcdialog"), [("Halt, who goes there?",)])

    def test_a_bad_role_writes_nothing(self):
        self.write({"blocks": [{"role": "jester", "say": ["Hey nonny."]}]})
        r = self.run_tool("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("role 'jester'", r.stdout)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'npcdialog'"), [(0,)])


if __name__ == "__main__":
    unittest.main()
