import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
ALLIGATOR, WEREGATOR = 1041154017, 740132644

# the Wolfwere form's record as the workbook import wrote it: INT -16 twice
WOLF_EFFECTS = json.dumps({"v": 1, "scope": "self", "effects": [
    {"id": "0950", "type": "Alter Appearance", "kind": "illusion", "sprite": "0x23A3EDDB"},
    {"id": "0951", "type": "Statistic Change", "kind": "stat", "stat": "str", "amount": 45},
    {"id": "0951", "type": "Statistic Change", "kind": "stat", "stat": "sta", "amount": 45},
    {"id": "0951", "type": "Statistic Change", "kind": "stat", "stat": "int", "amount": -16},
    {"id": "0952", "type": "Statistic Change", "kind": "stat", "stat": "int", "amount": -16, "resist": "ar"}]},
    separators=(",", ":"))
WOLF_LINE = "Self, Gives Form, Increase STR by 45, Increase STA by 45, Decrease INT by 16, Decrease INT by 16"


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FixWereLines(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, modelid INTEGER, size REAL)")
        db.execute("CREATE TABLE quest_npc_seed_rows (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, source TEXT NOT NULL, "
                   "template_row INTEGER, seedset TEXT NOT NULL DEFAULT '1-20')")
        db.execute("CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, "
                   "source TEXT NOT NULL, effects TEXT NOT NULL DEFAULT '')")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?)", [
            (122735, "gatorwere", ALLIGATOR, 1.0),
            (122736, "gatorwere", ALLIGATOR, 1.0),
            (130000, "gatorwere", ALLIGATOR, 1.0),        # a GM's: no seed wrote it
            (130001, "elder weregator", WEREGATOR, 1.1),  # Sandstorm's, as it is
            (130002, "raging alligator", ALLIGATOR, 1.0),  # the body's own creature
        ])
        db.executemany("INSERT INTO quest_npc_seed_rows VALUES (?, 'gatorwere', 'by The Bone Crusher', 116170, 'qc')",
                       [(122735,), (122736,)])
        db.execute("INSERT INTO spelleffect VALUES (901824, ?, 235077704, 'EQOAEmu''s workbook (2020); made', ?)",
                   (WOLF_LINE, WOLF_EFFECTS))
        db.execute("INSERT INTO spelleffect VALUES (901804, 'Self, Gives Form, Increase STR by 45', 1, 'x', '')")
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args, check=True):
        return subprocess.run([sys.executable, os.path.join(HERE, "fix_were_lines.py"), "--db", self.path, "--no-backup", *args],
                              capture_output=True, text=True, check=check)

    def test_the_gatorweres_and_the_wolfwere_form_and_undo_puts_back(self):
        out = self.tool().stdout
        self.assertIn("2 gatorweres given the weregator's body (4 values), 1 left; the Wolfwere form: 2 values", out)
        self.assertIn("npcs 130000 'gatorwere': on the alligator's body but no seed's row: left (a GM's)", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 6 values", self.tool("--apply").stdout)
        db = sqlite3.connect(self.path)
        bodies = {r[0]: r[1:] for r in db.execute("SELECT id, modelid, size FROM npcs")}
        line, effects = db.execute("SELECT line, effects FROM spelleffect WHERE spellid = 901824").fetchone()
        db.close()
        self.assertEqual(bodies, {122735: (WEREGATOR, 1.1), 122736: (WEREGATOR, 1.1), 130000: (ALLIGATOR, 1.0),
                                  130001: (WEREGATOR, 1.1), 130002: (ALLIGATOR, 1.0)},
                         "the seeded gatorweres; the GM's, Sandstorm's and the alligator as they were")
        self.assertEqual(line, "Self, Gives Form, Increase STR by 45, Increase STA by 45, Decrease INT by 16")
        ints = [e for e in json.loads(effects)["effects"] if e.get("stat") == "int"]
        self.assertEqual(ints, [{"id": "0952", "type": "Statistic Change", "kind": "stat", "stat": "int", "amount": -16,
                                 "resist": "ar"}], "INT -16 once, the resistable one")
        again = self.tool("--apply", check=False)
        self.assertNotEqual(again.returncode, 0, "a second apply is refused")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every value back")

    def test_undo_leaves_a_value_changed_since(self):
        self.tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET modelid = 5 WHERE id = 122736")   # a GM's body since
        db.commit()
        db.close()
        out = self.tool("--undo").stdout
        self.assertIn("left 1 changed since: npcs 122736 modelid", out)
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT modelid, size FROM npcs WHERE id = 122736").fetchone(), (5, 1.0))
        self.assertEqual(db.execute("SELECT modelid, size FROM npcs WHERE id = 122735").fetchone(), (ALLIGATOR, 1.0))
        db.close()


if __name__ == "__main__":
    unittest.main()
