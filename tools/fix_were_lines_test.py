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
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, modelid INTEGER, size REAL, "
                   "npc_level INTEGER NOT NULL DEFAULT 1, npc_type INTEGER NOT NULL DEFAULT 0)")
        db.execute("CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL, faction TEXT NOT NULL, "
                   "delta INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm', UNIQUE (npc_name, faction))")
        db.execute("CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')")
        db.execute("CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL, "
                   "target TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step))")
        db.execute("CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL, "
                   "PRIMARY KEY (questid, step, target))")
        db.execute("CREATE TABLE quest_npc_seed_rows (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, source TEXT NOT NULL, "
                   "template_row INTEGER, seedset TEXT NOT NULL DEFAULT '1-20')")
        db.execute("CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, "
                   "source TEXT NOT NULL, effects TEXT NOT NULL DEFAULT '')")
        db.executemany("INSERT INTO npcs (id, npc_name, modelid, size) VALUES (?, ?, ?, ?)", [
            (122735, "gatorwere", ALLIGATOR, 1.0),
            (122736, "gatorwere", ALLIGATOR, 1.0),
            (130000, "gatorwere", ALLIGATOR, 1.0),        # a GM's: no seed wrote it
            (130001, "elder weregator", WEREGATOR, 1.1),  # Sandstorm's, as it is
            (130002, "raging alligator", ALLIGATOR, 1.0),  # the body's own creature
        ])
        db.executemany("INSERT INTO npcs (id, npc_name, modelid, size, npc_level, npc_type) VALUES (?, ?, 1, 1.0, ?, ?)", [
            (140000, "Zaren", 44, 0),
            (140001, "Levandius", 60, 0),
            (140002, "Dorandas", 60, 0),
            (140003, "Edward Duskrender", 60, 0),
            (140004, "Corindas", 50, 0x80),          # an NPC already
            (140005, "Yindrius", 10, 0x0300),        # a class byte above: kept
            (140006, "The Bone Crusher", 40, 0),
            (140007, "Levandius the Younger", 30, 0),  # not a giver
        ])
        db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES ('wererat', 'Topi Di Ombra', -5, 'gm')")
        db.executemany("INSERT INTO quests VALUES (?, ?, ?)", [
            (101348, "Wolfwere Tails", "EQOA-Data Weres/wolfwere/p2 (tools/import_quests.py)"),
            (101355, "Bearwere Tails", "EQOA-Data Weres/bearwere/p2 (tools/import_quests.py)"),
            (101362, "Ratwere Tails", "EQOA-Data Weres/ratwere/p2 (tools/import_quests.py)")])
        db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?)", [
            (101348, 1, "loot", "wolfwere"), (101348, 2, "give", "Levandius"),
            (101355, 1, "loot", "bearwere"), (101355, 2, "give", "Dorandas"),
            (101362, 1, "loot", "wererat")])
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
        self.assertIn("2 gatorweres given the weregator's body (4 values), 1 left; the Wolfwere form: 2 values; Zaren: 1; "
                      "givers given the NPC bit: 5; kill hits: 6 (1 left); tail creatures: 2", out)
        self.assertIn("npcs 130000 'gatorwere': on the alligator's body but no seed's row: left (a GM's)", out)
        self.assertIn("factionhits wererat / Topi Di Ombra: -5 (gm): a row of its own is there: left", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("wrote 20 values", self.tool("--apply").stdout)
        db = sqlite3.connect(self.path)
        bodies = {r[0]: r[1:] for r in db.execute("SELECT id, modelid, size FROM npcs WHERE id < 140000")}
        line, effects = db.execute("SELECT line, effects FROM spelleffect WHERE spellid = 901824").fetchone()
        people = {r[0]: r[1:] for r in db.execute("SELECT id, npc_level, npc_type FROM npcs WHERE id >= 140000")}
        hits = {(r[0], r[1]): (r[2], r[3]) for r in db.execute("SELECT npc_name, faction, delta, source FROM factionhits")}
        tails = sorted(db.execute("SELECT questid, step, target FROM queststeptargets").fetchall())
        db.close()
        self.assertEqual(bodies, {122735: (WEREGATOR, 1.1), 122736: (WEREGATOR, 1.1), 130000: (ALLIGATOR, 1.0),
                                  130001: (WEREGATOR, 1.1), 130002: (ALLIGATOR, 1.0)},
                         "the seeded gatorweres; the GM's, Sandstorm's and the alligator as they were")
        self.assertEqual(people, {140000: (35, 0), 140001: (60, 0x80), 140002: (60, 0x80), 140003: (60, 0x80),
                                  140004: (50, 0x80), 140005: (10, 0x0380), 140006: (40, 0x80), 140007: (30, 0)},
                         "Zaren at 35; the givers with the NPC bit, a class byte kept; another name as it was")
        self.assertEqual(hits, {("wererat", "Topi Di Ombra"): (-5, "gm"),
                                ("wolfwere", "Night Reapers"): (10, "were-kills"), ("werewolf", "Night Reapers"): (10, "were-kills"),
                                ("bearwere", "Wards of Dawn"): (10, "were-kills"), ("werebear", "Wards of Dawn"): (10, "were-kills"),
                                ("lionwere", "Sunmanes"): (10, "were-kills"),
                                ("gatorwere", "The Bone Crusher"): (10, "were-kills")},
                         "a kill of each line's were +10 to its society; the GM's row kept")
        self.assertEqual(tails, [(101348, 1, "werewolf"), (101355, 1, "werebear")], "the other spellings on the tail steps")
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
