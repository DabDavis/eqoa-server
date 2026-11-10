#!/usr/bin/env python3
"""tools/seed_town_dialog.py against a made-up database and lines: the rows
a file's blocks make (an absent field any), the table made when the server
has not made it, a file with a bad role refused, and --undo taking back each
row still as written (a GM's change kept). And the path a database that
holds the earlier lines takes (the live shards do): --apply adds only what is
new and logs it, a second --apply writes nothing, the lore set is written
beside the town set under a log and a source of its own, and its --undo
takes back exactly it, to the byte of the database's dump. No game data;
EQOA_REAL_DB (a server's database, copied) runs the same on the real one.

    python3 tools/seed_town_dialog_test.py
"""
import hashlib
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_town_dialog.py")
sys.path.insert(0, HERE)
import seed_town_dialog as std  # noqa: E402

# npcdialog as the server makes it (database/npcdialog.go), and an older one's later columns
SERVER_TABLE = std.SCHEMA.replace("CREATE TABLE IF NOT EXISTS", "CREATE TABLE")


def dump(path):
    """The whole database as SQL: what "as it was" is held to."""
    c = sqlite3.connect(path)
    try:
        return "\n".join(c.iterdump())
    finally:
        c.close()


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

    def test_a_line_logged_before_the_later_columns_is_the_one_written(self):
        """The live shards' case: the log's rows were written before speaker_race and away (and here before
        sex too), and the file still has the line: nothing is written again, and nothing is taken for gone."""
        with open(self.lines, "w") as f:
            json.dump({"blocks": [{"town": "Freeport", "role": "citizen", "say": ["My husband works the docks."]}]}, f)
        out = self.tool("--apply").stdout
        self.assertIn("in the table already: 1 written before (npcdialog_seed); 0 new", out)
        self.assertIn("nothing new: nothing written", out)
        self.assertNotIn("no longer in the file", out)
        self.assertEqual(self.q("SELECT id, line FROM npcdialog"), [(1, "My husband works the docks.")])
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog_seed"), [(1,)])


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
        self.town = [
            {"role": "guard", "say": ["Move along.", "Halt."]},
            {"town": "Testvale", "mood": "warm", "race": 3, "weight": 2, "say": ["Welcome home, %p."]},
        ]
        self.write({"blocks": self.town})
        self.lore = os.path.join(self.dir.name, "lore.json")
        self.lore_blocks = [
            {"town": "Testvale", "say": [{"line": "The vale was a marsh once, they say.", "note": "VALE-1: the vale's founding"},
                                         {"line": "My grandfather drained the first field.", "note": "VALE-1"}]},
            {"speaker_race": "troll", "away": True, "say": [{"line": "Vale too dry. Troll miss swamp.", "note": "TROLL-1"}]},
            {"race": 1, "role": "guard", "say": ["Your folk sang here before ours built."]},
        ]
        self.write({"blocks": self.lore_blocks}, self.lore)

    def tearDown(self):
        self.dir.cleanup()

    def write(self, d, path=None):
        with open(path or self.lines, "w") as f:
            json.dump(d, f)

    def run_tool(self, *args, ok=True, lines=None):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--lines", lines or self.lines, "--no-backup", *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def run_lore(self, *args, ok=True):
        return self.run_tool("--set", "lore", *args, ok=ok, lines=self.lore)

    def q(self, sql, *args):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql, args).fetchall()
        finally:
            c.close()

    def ex(self, sql, *args):
        c = sqlite3.connect(self.db)
        try:
            c.execute(sql, args)
            c.commit()
        finally:
            c.close()

    def test_apply_and_undo(self):
        out = self.run_tool().stdout
        self.assertIn("3 lines, 1 towns", out)
        self.assertIn("towns with no lines of their own (they say the generic ones): Qeynos", out)
        self.run_tool("--apply")
        rows = [
            ("", "guard", "", "", -1, "Move along.", 1, "ours"),
            ("", "guard", "", "", -1, "Halt.", 1, "ours"),
            ("Testvale", "", "", "warm", 3, "Welcome home, %p.", 2, "ours"),
        ]
        self.assertEqual(self.q("SELECT town, role, name, mood, race, line, weight, source FROM npcdialog ORDER BY id"), rows)
        # a second --apply writes nothing: the lines were written once
        was = dump(self.db)
        again = self.run_tool("--apply").stdout
        self.assertIn("in the table already: 3 written before (npcdialog_seed); 0 new", again)
        self.assertIn("nothing new: nothing written", again)
        self.assertEqual(dump(self.db), was, "a second --apply changed the database")
        self.ex("UPDATE npcdialog SET line = 'Halt, who goes there?' WHERE line = 'Halt.'")  # a GM's change
        self.assertIn("nothing new", self.run_tool("--apply").stdout, "a line a GM changed was written again")
        self.assertIn("took back 2 lines; 1 a GM changed", self.run_tool("--undo").stdout)
        self.assertEqual(self.q("SELECT line FROM npcdialog"), [("Halt, who goes there?",)])
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'npcdialog_seed'"), [(0,)])

    def test_a_bad_role_writes_nothing(self):
        self.write({"blocks": [{"role": "jester", "say": ["Hey nonny."]}]})
        r = self.run_tool("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("role 'jester'", r.stdout)
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'npcdialog'"), [(0,)])

    def test_a_later_line_is_added_alone(self):
        """The file grows after its lines were written: --apply writes the new line and no other, the
        earlier rows keep their ids, and a line a GM took away is not written again."""
        self.run_tool("--apply")
        ids = self.q("SELECT id, line FROM npcdialog ORDER BY id")
        self.ex("DELETE FROM npcdialog WHERE line = 'Move along.'")  # a GM took it away
        self.town[0]["say"].append("Mind the well.")
        self.write({"blocks": self.town})
        out = self.run_tool("--apply").stdout
        self.assertIn("in the table already: 3 written before (npcdialog_seed); 1 new", out)
        self.assertIn("wrote 1 lines (npcdialog 4-4, logged in npcdialog_seed)", out)
        self.assertEqual(self.q("SELECT id, line FROM npcdialog ORDER BY id"), ids[1:] + [(4, "Mind the well.")])
        self.assertEqual(self.q("SELECT id FROM npcdialog_seed ORDER BY id"), [(1,), (2,), (3,), (4,)])
        self.assertIn("took back 3 lines; 0 a GM changed", self.run_tool("--undo").stdout)
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog"), [(0,)])

    def test_the_lore_set_beside_the_town_set(self):
        """The lore set on a database that holds the town set: its lines alone are written, under its own
        source and log; a second --apply writes nothing; --undo takes back exactly it, and the database
        is as it was before, to the byte of its dump, the table's counter included."""
        self.run_tool("--apply")
        self.ex("INSERT INTO npcdialog (name, line, source) VALUES ('Mayor Pip', 'A GM''s line.', 'gm')")
        before = dump(self.db)
        out = self.run_lore("--apply").stdout
        self.assertIn("wrote 4 lines (npcdialog 5-8, logged in npcdialog_lore_seed)", out)
        self.assertEqual(self.q("SELECT id, town, role, race, speaker_race, away, line, source FROM npcdialog WHERE id > 4 ORDER BY id"), [
            (5, "Testvale", "", -1, -1, 0, "The vale was a marsh once, they say.", "lore"),
            (6, "Testvale", "", -1, -1, 0, "My grandfather drained the first field.", "lore"),
            (7, "", "", -1, 5, 1, "Vale too dry. Troll miss swamp.", "lore"),
            (8, "", "guard", 1, -1, 0, "Your folk sang here before ours built.", "lore"),
        ])
        self.assertEqual(self.q("SELECT id FROM npcdialog_lore_seed ORDER BY id"), [(5,), (6,), (7,), (8,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog_seed"), [(3,)], "the town set's log was touched")
        self.assertNotIn("note", self.q("SELECT row FROM npcdialog_lore_seed WHERE id = 5")[0][0], "a note was written")
        applied = dump(self.db)
        again = self.run_lore("--apply").stdout
        self.assertIn("in the table already: 4 written before (npcdialog_lore_seed); 0 new", again)
        self.assertIn("nothing new: nothing written", again)
        self.assertEqual(dump(self.db), applied, "a second --apply changed the database")
        # the town set's own second --apply sees its three, and none of the lore set's
        self.assertIn("in the table already: 3 written before (npcdialog_seed); 0 new", self.run_tool("--apply").stdout)
        self.assertEqual(dump(self.db), applied)
        self.assertIn("took back 4 lines; 0 a GM changed", self.run_lore("--undo").stdout)
        self.assertEqual(dump(self.db), before, "--undo did not leave the database as it was")
        self.assertIn("nothing logged in npcdialog_lore_seed", self.run_lore("--undo").stdout)

    def test_undo_leaves_the_counter_when_something_else_was_written(self):
        """A row written to npcdialog after the set's: --undo takes the set's rows and never the other,
        and the table's counter stays (a number is never given twice while another holds a later one)."""
        self.run_tool("--apply")
        self.run_lore("--apply")
        self.ex("INSERT INTO npcdialog (name, line, source) VALUES ('Mayor Pip', 'A GM''s later line.', 'gm')")
        self.assertIn("took back 4 lines", self.run_lore("--undo").stdout)
        self.assertEqual(self.q("SELECT id, line FROM npcdialog WHERE id > 3"), [(8, "A GM's later line.")])
        self.assertEqual(self.q("SELECT seq FROM sqlite_sequence WHERE name = 'npcdialog'"), [(8,)])

    def test_undo_leaves_a_line_a_gm_changed(self):
        self.run_tool("--apply")
        self.run_lore("--apply")
        self.assertEqual(self.q("SELECT id FROM npcdialog_lore_seed ORDER BY id"), [(4,), (5,), (6,), (7,)])
        self.ex("UPDATE npcdialog SET weight = 3 WHERE id = 5")           # a GM's change: a number
        self.ex("UPDATE npcdialog SET source = 'gm' WHERE id = 6")         # and one taken as the GM's own
        self.ex("DELETE FROM npcdialog WHERE id = 7")                      # and one taken away
        self.assertIn("nothing new: nothing written", self.run_lore("--apply").stdout)
        self.assertIn("took back 1 lines; 2 a GM changed since left as they are", self.run_lore("--undo").stdout)
        self.assertEqual(self.q("SELECT id, weight, source FROM npcdialog WHERE id > 3 ORDER BY id"), [(5, 3, "lore"), (6, 1, "gm")])
        self.assertEqual(self.q("SELECT seq FROM sqlite_sequence WHERE name = 'npcdialog'"), [(7,)], "the counter went back past a kept row")
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog WHERE id <= 3"), [(3,)], "the town set's rows went")

    def test_rows_there_unlogged_are_left_alone(self):
        """A database whose tools' logs were dropped (the open database for a fresh server): the lines are
        in the table and no log says so. --apply writes none of them again and makes no log; a line the
        table lacks is written and logged alone, and --undo takes back only it."""
        self.run_tool("--apply")
        self.ex("DROP TABLE npcdialog_seed")
        was = dump(self.db)
        out = self.run_tool("--apply").stdout
        self.assertIn("in the table already: 0 written before (npcdialog_seed), 3 there unlogged (left alone); 0 new", out)
        self.assertIn("nothing new: nothing written", out)
        self.assertEqual(dump(self.db), was)
        self.town[0]["say"].append("Mind the well.")
        self.write({"blocks": self.town})
        self.assertIn("wrote 1 lines (npcdialog 4-4, logged in npcdialog_seed)", self.run_tool("--apply").stdout)
        self.assertEqual(self.q("SELECT id FROM npcdialog_seed"), [(4,)])
        self.assertIn("took back 1 lines", self.run_tool("--undo").stdout)
        self.assertEqual(dump(self.db), was)

    def test_prune(self):
        """A line the file no longer has: said by the report, left by --apply, taken out by --apply --prune
        while it is still as written (a GM's change kept)."""
        self.run_lore("--apply")
        self.ex("UPDATE npcdialog SET line = 'A GM reworded this one.' WHERE id = 2")
        self.lore_blocks[0]["say"] = []                      # both of the vale's lines leave the file
        self.lore_blocks.append({"say": ["A new common line."]})
        self.write({"blocks": self.lore_blocks}, self.lore)
        out = self.run_lore("--apply").stdout
        self.assertEqual(out.count("logged and no longer in the file"), 2)
        self.assertIn("wrote 1 lines (npcdialog 5-5", out)
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog"), [(5,)], "--apply took a line out without --prune")
        out = self.run_lore("--apply", "--prune").stdout
        self.assertIn("took out 1 lines the file no longer has; 1 a GM changed since left as they are", out)
        self.assertEqual(self.q("SELECT id FROM npcdialog ORDER BY id"), [(2,), (3,), (4,), (5,)])
        self.assertEqual(self.q("SELECT id FROM npcdialog_lore_seed ORDER BY id"), [(3,), (4,), (5,)])
        self.assertIn("nothing new: nothing written", self.run_lore("--apply", "--prune").stdout)

    def test_list_names_who_says_a_line_and_its_note(self):
        out = self.run_lore("--list").stdout
        self.assertIn("[Testvale] The vale was a marsh once, they say.\n    VALE-1: the vale's founding", out)
        self.assertIn("[any town, a troll away from home] Vale too dry. Troll miss swamp.\n    TROLL-1", out)
        self.assertIn("[any town, guard, to an elf] Your folk sang here before ours built.", out)
        self.assertIn("4 lines", out)

    def test_what_a_file_may_not_hold(self):
        bad = {
            "speaker_race 'pixie'": [{"speaker_race": "pixie", "say": ["Hello."]}],
            "away is a speaker_race's": [{"away": True, "say": ["Hello."]}],
            "a '|' or a control character": [{"say": ["Press |x to continue."]}],
            "a '%' that is not": [{"say": ["Prices are up 10% this season."]}],
            "past a box's 217": [{"say": ["x" * 191]}],
            "the same line for the same speakers": [{"role": "guard", "say": ["Halt."]}, {"role": "guard", "say": ["Halt."]}],
            "an empty line": [{"say": [{"line": " ", "note": "N-1"}]}],
        }
        for want, blocks in bad.items():
            self.write({"blocks": blocks}, self.lore)
            r = self.run_lore("--apply", ok=False)
            self.assertNotEqual(r.returncode, 0, want)
            self.assertIn(want, r.stdout)
            self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name LIKE 'npcdialog%'"), [(0,)], want)
        # the same words for other speakers are another line, and every word a line is filled with passes
        self.write({"blocks": [{"role": "guard", "say": ["Halt, %p the %r %c of %t. I am %n."]}, {"role": "citizen", "say": ["Halt, %p the %r %c of %t. I am %n."]}]}, self.lore)
        self.assertIn("wrote 2 lines", self.run_lore("--apply").stdout)


class TheShippedSets(unittest.TestCase):
    """The files the tool ships with have no problem, and the two sets share no line."""

    def test_no_problem_and_no_line_twice(self):
        seen = {}
        for name, (path, log, source) in std.SETS.items():
            rows, problems = std.rows_of(path)
            self.assertEqual(problems, [], name)
            self.assertTrue(rows, name)
            for r in rows:
                j = std.as_json(r)
                self.assertNotIn(j, seen, f"{name} and {seen.get(j)} share {r['line']!r}")
                seen[j] = name
        logs = [log for _, log, _ in std.SETS.values()]
        self.assertEqual(len(set(logs)), len(logs))
        for log in logs:  # the builders of the shards' databases keep a table whose name ends so (their own rule)
            self.assertRegex(log, r"_seed$")


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB names a server's database (a copy is used)")
class OnARealDatabase(unittest.TestCase):
    """A server's database that holds the town set (EQOA_REAL_DB; copied, never written): the town set has
    nothing new for it, the lore set's --apply adds its lines alone, a second adds none, and its --undo
    leaves the copy as the database was, to the byte of its dump."""

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory(dir=os.environ.get("TMPDIR") or None)
        self.db = os.path.join(self.dir.name, "real.db")
        src = sqlite3.connect(f"file:{os.environ['EQOA_REAL_DB']}?mode=ro", uri=True)
        dst = sqlite3.connect(self.db)
        try:
            src.backup(dst)
        finally:
            dst.close()
            src.close()

    def tearDown(self):
        self.dir.cleanup()

    def tool(self, *args):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        if r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r.stdout

    def sha(self):
        """The dump's SHA-256 (sqlite3's own .dump when it is installed: a real database's is long)."""
        h = hashlib.sha256()
        if shutil.which("sqlite3"):
            with subprocess.Popen(["sqlite3", self.db, ".dump"], stdout=subprocess.PIPE) as p:
                for chunk in iter(lambda: p.stdout.read(1 << 20), b""):
                    h.update(chunk)
            self.assertEqual(p.returncode, 0)
        else:
            c = sqlite3.connect(self.db)
            for line in c.iterdump():
                h.update(line.encode() + b"\n")
            c.close()
        return h.hexdigest()

    def q(self, sql):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql).fetchall()
        finally:
            c.close()

    def test_apply_again_and_undo(self):
        if not self.q("SELECT count(*) FROM sqlite_master WHERE name = 'npcdialog_seed'")[0][0]:
            self.skipTest("this database does not hold the town set's log")
        before, rows = self.sha(), self.q("SELECT count(*) FROM npcdialog")[0][0]
        seq = self.q("SELECT seq FROM sqlite_sequence WHERE name = 'npcdialog'")[0][0]
        town = self.tool("--apply")
        self.assertIn("nothing new: nothing written", town)
        self.assertNotIn("no longer in the file", town, "the town set's file and its log on this database differ")
        self.assertEqual(self.sha(), before, "the town set's --apply changed a database that holds it")
        lore, _ = std.rows_of(std.SETS["lore"][0])
        out = self.tool("--set", "lore", "--apply")
        self.assertIn(f"wrote {len(lore)} lines", out)
        self.assertEqual(self.q("SELECT count(*), min(id) FROM npcdialog WHERE source = 'lore'")[0], (len(lore), seq + 1))
        self.assertEqual(self.q("SELECT count(*) FROM npcdialog")[0][0], rows + len(lore))
        applied = self.sha()
        self.assertIn("nothing new: nothing written", self.tool("--set", "lore", "--apply"))
        self.assertEqual(self.sha(), applied, "a second --apply changed the database")
        self.assertIn(f"took back {len(lore)} lines; 0 a GM changed", self.tool("--set", "lore", "--undo"))
        self.assertEqual(self.sha(), before, "--undo did not leave the database as it was")


if __name__ == "__main__":
    unittest.main()
