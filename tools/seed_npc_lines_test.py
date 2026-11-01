#!/usr/bin/env python3
"""seed_npc_lines.py against a database of its own: the file's lines written
once, an edit synced (a new line added, a dropped one taken out), a GM's
change kept, and --undo exact; the shipped file has no problem."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_npc_lines as snl  # noqa: E402


def lines_file(d, blocks):
    p = os.path.join(d, "lines.json")
    with open(p, "w", encoding="utf-8") as f:
        json.dump({"blocks": blocks}, f)
    return p


class SeedNPCLinesTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = sqlite3.connect(os.path.join(self.dir, "t.db"), isolation_level=None)
        self.db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT)")
        self.db.execute("INSERT INTO npcs (npc_name) VALUES ('Valrean V`Ghen')")
        self.db.execute(snl.SCHEMA)
        self.db.execute("INSERT INTO npcdialog (town, line, weight, source) VALUES ('Neriak', 'A town line.', 1, 'ours')")

    def rows(self, blocks):
        rows, problems = snl.rows_of(lines_file(self.dir, blocks))
        self.assertEqual(problems, [])
        return rows

    def own(self):
        return sorted(r[0] for r in self.db.execute("SELECT line FROM npcdialog WHERE name <> ''"))

    def test_written_once_synced_and_undone(self):
        b = [{"name": "Valrean V`Ghen", "say": ["One.", "Two."]}, {"name": "Valrean V`Ghen", "mood": "wary", "say": ["Three."]}]
        self.assertEqual(snl.apply(self.db, self.rows(b)), (3, 0, 0))
        self.assertEqual(snl.apply(self.db, self.rows(b)), (0, 0, 0), "a second apply wrote again")
        self.assertEqual(self.own(), ["One.", "Three.", "Two."])
        mood = self.db.execute("SELECT mood, race, weight, source FROM npcdialog WHERE line = 'Three.'").fetchone()
        self.assertEqual(mood, ("wary", -1, 1, "ours"))

        # an edit: "Two." dropped, "Four." added; a GM changed "One." meanwhile
        self.db.execute("UPDATE npcdialog SET line = 'One, a GM''s.' WHERE line = 'One.'")
        b = [{"name": "Valrean V`Ghen", "say": ["One.", "Four."]}, {"name": "Valrean V`Ghen", "mood": "wary", "say": ["Three."]}]
        self.assertEqual(snl.apply(self.db, self.rows(b)), (1, 1, 0))
        self.assertEqual(self.own(), ["Four.", "One, a GM's.", "Three."], "the GM's change was written over, or the sync missed")

        gone, kept = snl.undo(self.db)
        self.assertEqual((gone, kept), (2, 1))
        self.assertEqual(self.own(), ["One, a GM's."], "undo took the GM's line or left one of ours")
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcdialog WHERE name = ''").fetchone()[0], 1, "the town line went")
        self.assertFalse(snl.has_table(self.db, "npcdialog_own_seed"))

    def test_race_blocks_and_an_old_log(self):
        b = [{"speaker_race": "ogre", "away": True, "say": ["Oggok far."]},
             {"speaker_race": "troll", "role": "guard", "say": ["Troll watch."]}]
        rows = self.rows(b)
        got = sorted((r["speaker_race"], r["away"], r["role"], r["name"]) for r in rows)
        self.assertEqual(got, [(5, 0, "guard", ""), (9, 1, "", "")])
        self.assertEqual(snl.apply(self.db, rows), (2, 0, 0))
        self.assertEqual(self.db.execute("SELECT speaker_race, away FROM npcdialog WHERE line = 'Oggok far.'").fetchone(), (9, 1))
        # a name's line logged before speaker_race and away existed is still the one written
        cur = self.db.execute("INSERT INTO npcdialog (name, line, weight, source) VALUES ('Valrean V`Ghen', 'Old one.', 1, 'ours')")
        old = {"town": "", "role": "", "name": "Valrean V`Ghen", "mood": "", "race": -1, "sex": "", "line": "Old one.", "weight": 1}
        self.db.execute("INSERT INTO npcdialog_own_seed VALUES (?, ?)", (cur.lastrowid, snl.json.dumps(old, sort_keys=True)))
        b.append({"name": "Valrean V`Ghen", "say": ["Old one."]})
        self.assertEqual(snl.apply(self.db, self.rows(b)), (0, 0, 0), "the old log's row was written again or taken out")
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcdialog WHERE line = 'Old one.'").fetchone()[0], 1)

    def test_problems(self):
        _, problems = snl.rows_of(lines_file(self.dir, [{"name": "", "say": ["x"]}, {"name": "A", "mood": "angry", "say": ["y", " "]},
                                                         {"name": "B", "say": ["z" * 300]}]))
        self.assertEqual(len(problems), 4, problems)
        _, problems = snl.rows_of(lines_file(self.dir, [{"name": "A", "speaker_race": "ogre", "say": ["x"]},
                                                         {"speaker_race": "wookie", "say": ["x"]},
                                                         {"speaker_race": "troll", "role": "pirate", "say": ["x"]},
                                                         {"name": "B", "away": True, "say": ["x"]}]))
        self.assertEqual(len(problems), 4, problems)

    def test_the_shipped_file(self):
        rows, problems = snl.rows_of(snl.DEFAULT_LINES)
        self.assertEqual(problems, [])
        self.assertTrue(any(r["name"] == "Valrean V`Ghen" for r in rows))
        self.assertTrue(any(r["mood"] == "" for r in rows), "no line for a neutral listener")
        for race in range(10):
            self.assertTrue(any(r["speaker_race"] == race and r["mood"] == "" for r in rows), f"race {race} has no voice of its own")
        for race in (5, 9):  # trolls and ogres say no common line: every role needs theirs
            roles = {r["role"] for r in rows if r["speaker_race"] == race and not r["away"]}
            self.assertTrue({"", "guard", "trader"} <= roles, f"race {race}'s roles: {roles}")


if __name__ == "__main__":
    unittest.main()
