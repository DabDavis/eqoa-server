#!/usr/bin/env python3
"""tools/seed_faction_starts.py against a made-up database and measurement:
the compression (a start every combo shares is no row, a race's one row, a
human city's and a class's where they differ), every measured combo coming
out at its standing by the server's rule, each faction's own standing made
its commonest start (but a GM's, and none with --keep-defaults), a faction
Sandstorm lists added and its junk left out, a GM's row left and said, the
names taken without their ends' spaces, a dry run that writes nothing, a
second --apply and a partial measurement's refused, --undo exact but for a
row a GM changed since, and the table the same as the server's. No game
data: the factions and standings are made up.

    python3 tools/seed_faction_starts_test.py
"""
import contextlib
import io
import os
import re
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_faction_starts as sfs  # noqa: E402

SCHEMA = """
CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL);
CREATE TABLE defaultcharacter (defaultCharacterID INTEGER PRIMARY KEY, tclass INTEGER, race INTEGER, humType INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '');
CREATE TABLE charfactions (serverid INTEGER, faction TEXT, standing INTEGER);
"""
FACTIONS = {"Citizens of Alpha": 1, "Citizens of Beta": -2000, "Guild of Gamma": 0, "Delta Watch ": 5,
            "Wolves": -500, "Edited": 7, "Old Order": 3}
# measured but not in the table: added; junk: left out
NEW, JUNK = "Order of Epsilon", "*Delete"
# (race, class, humtype): two human cities, a dark elf's three classes, a gnome's two
COMBOS = [(0, 0, 1), (0, 9, 1), (0, 0, 2), (0, 9, 2), (0, 1, 2), (2, 0, 0), (2, 3, 0), (2, 11, 0), (3, 13, 0), (3, 11, 0)]


def standing(race, cls, hum, faction):
    """The made-up measurement."""
    if faction == "Citizens of Alpha":  # the dark elves' city: theirs 1000, the west's -1500
        return 1000 if race == 2 else (-1500 if race == 0 and hum == 2 else 1)
    if faction == "Citizens of Beta":  # the west's city; its own priests more
        if race == 0 and hum == 2:
            return 900 if cls == 9 else 500
        return -2000
    if faction == "Guild of Gamma":  # necromancers of every race
        return 100 if cls == 11 else 0
    if faction == "Delta Watch ":  # everyone the same, not the table's
        return 50
    if faction == "Wolves":  # everyone the table's
        return -500
    if faction == "Edited":  # everyone the same, a GM's own standing in the table
        return 70
    if faction == NEW:  # the gnomes' order
        return 600 if race == 3 else 0
    if faction == JUNK:
        return 1
    raise KeyError(faction)


MEASURED = [f for f in FACTIONS if f != "Old Order"] + [NEW, JUNK]  # Old Order is ours alone


def dump(db):
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"):
        out[t] = sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
    return out


class SeedFactionStartsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        db.execute(sfs.SCHEMA)  # the server's, made at its start
        db.executemany("INSERT INTO factions VALUES (?, ?)", FACTIONS.items())
        db.executemany("INSERT INTO defaultcharacter (tclass, race, humType) VALUES (?, ?, ?)",
                       [(c, r, h) for r, c, h in COMBOS])
        db.execute("""INSERT INTO gm_audit (tbl, rowkey) VALUES ('factions', '{"name":"Edited"}')""")
        db.close()
        self.tsv = self.write_tsv("m.tsv", COMBOS)

    def write_tsv(self, name, combos):
        path = os.path.join(self.dir, name)
        with open(path, "w", encoding="utf-8") as f:
            f.write("race\tclass\thumtype\tfaction\tstanding\n")
            for r, c, h in combos:
                for name in MEASURED:
                    written = "Delta Watch" if name == "Delta Watch " else name  # its ends' spaces lost
                    f.write(f"{r}\t{c}\t{h}\t{written}\t{standing(r, c, h, name)}\n")
        return path

    def db(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        self.addCleanup(db.close)
        return db

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = sfs.main(["--db", self.path, *args])
        return code, out.getvalue()

    def test_the_server_rule(self):
        # one faction's rows (entity/factionstart_test's TestStartStandingResolution's cases)
        neriak = [(2, -1, -1, 500), (2, 3, -1, 800), (-1, 3, -1, -50)]
        for combo, want in [((2, 3, 0), 800), ((2, 0, 0), 500), ((0, 3, 1), -50), ((3, 13, 0), 1)]:
            self.assertEqual(sfs.resolve(neriak, 1, combo), want, combo)
        qeynos = [(0, -1, 1, -1000), (0, -1, 2, 400), (0, 9, 2, 900)]
        for combo, want in [((0, 9, 2), 900), ((0, 0, 2), 400), ((0, 9, 1), -1000), ((2, 3, 0), -2000)]:
            self.assertEqual(sfs.resolve(qeynos, -2000, combo), want, combo)
        # the server's combo: a froglok and a human of no city as the east's
        self.assertEqual(sfs.combo_of(10, 3, 0), (0, 3, 1))
        self.assertEqual(sfs.combo_of(0, 9, 0), (0, 9, 1))
        self.assertEqual(sfs.combo_of(2, 0, 1), (2, 0, 0))
        # race and class over race and city over class and city
        tie = [(0, -1, 2, 10), (0, 9, -1, 20), (-1, 9, 2, 30)]
        self.assertEqual(sfs.resolve(tie, 0, (0, 9, 2)), 20)

    def test_the_plan_keeping_the_tables_standings(self):
        db = self.db()
        measured, said = sfs.read_measured(self.tsv, set(FACTIONS))
        self.assertEqual(said["taken without the spaces at its ends"], ["'Delta Watch' as 'Delta Watch '"])
        self.assertEqual(said["junk, left out"], ["'*Delete'"])
        self.assertEqual(said["not in the factions table: added"], ["'Order of Epsilon'"])
        p = sfs.plan(db, measured, keep_defaults=True)
        self.assertEqual(sorted(p["rows"]), sorted([
            # Alpha: the dark elves' one row; the humans' commonest, the west's -1500 (three
            # combos of five), and the east's 1
            (2, -1, -1, "Citizens of Alpha", 1000), (0, -1, -1, "Citizens of Alpha", -1500),
            (0, -1, 1, "Citizens of Alpha", 1),
            # Beta: the humans' tie (500 twice, -2000 twice) goes to the faction's own, -2000,
            # so the west's city and the west's priests
            (0, -1, 2, "Citizens of Beta", 500), (0, 9, 2, "Citizens of Beta", 900),
            # Gamma: each race's necromancers
            (2, 11, -1, "Guild of Gamma", 100), (3, 11, -1, "Guild of Gamma", 100),
            # the new order: the gnomes', at its commonest 0
            (3, -1, -1, NEW, 600),
            # every combo shares Delta Watch's 50, the table 5: a row a race
            (0, -1, -1, "Delta Watch ", 50), (2, -1, -1, "Delta Watch ", 50), (3, -1, -1, "Delta Watch ", 50),
        ]))
        self.assertEqual(p["added"], {NEW: 0})
        self.assertEqual(p["kept_table"], [("Delta Watch ", 50, 5)])
        # every combo shares Edited's 70, a GM's 7 in the table: no row, said
        self.assertEqual(p["shared"], [("Edited", 70, 7)])
        self.assertEqual(p["absent"], ["Old Order"])
        self.assertEqual(p["missing"], [])

    def test_the_own_standings_made_the_commonest(self):
        db = self.db()
        measured, _ = sfs.read_measured(self.tsv, set(FACTIONS))
        p = sfs.plan(db, measured)
        # Delta Watch's own becomes everyone's 50; Beta's commonest is its own -2000; Edited is a GM's
        self.assertEqual(p["new_defaults"], {"Delta Watch ": (5, 50)})
        self.assertEqual(p["gm_defaults"], [("Edited", 70, 7)])
        self.assertEqual(p["shared"], [("Edited", 70, 7)])

    def test_a_gms_row_is_left(self):
        db = self.db()
        db.execute("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (2, -1, -1, 'Citizens of Alpha', 3)")
        measured, _ = sfs.read_measured(self.tsv, set(FACTIONS))
        p = sfs.plan(db, measured)
        self.assertNotIn((2, -1, -1, "Citizens of Alpha", 1000), p["rows"])
        self.assertEqual(p["kept_gm"], [(2, -1, -1, "Citizens of Alpha", 1000)])
        self.assertEqual(sorted(c for c, f, v, got in p["overridden"] if f == "Citizens of Alpha"),
                         [(2, 0, 0), (2, 3, 0), (2, 11, 0)])

    def test_a_partial_measurement_is_refused(self):
        partial = self.write_tsv("partial.tsv", COMBOS[:4])
        with self.assertRaises(SystemExit):
            with contextlib.redirect_stderr(io.StringIO()):
                self.run_tool("--measured", partial, "--apply", "--no-backup")
        db = self.db()
        self.assertEqual(db.execute("SELECT COUNT(*) FROM factionstart").fetchone(), (0,))
        code, out = self.run_tool("--measured", partial, "--apply", "--partial", "--no-backup", "--quiet")
        self.assertEqual(code, 0, out)
        # four human combos of five measured: a row a measured combo, none for the west's ranger
        rows = db.execute("SELECT race, class, humtype FROM factionstart WHERE race = 0").fetchall()
        self.assertTrue(rows)
        self.assertTrue(all(c >= 0 and h >= 0 for _, c, h in rows), rows)
        self.assertNotIn((0, 1, 2), rows)

    def test_apply_then_undo(self):
        db = self.db()
        before = dump(db)
        code, out = self.run_tool("--measured", self.tsv)  # a dry run
        self.assertEqual(code, 0)
        self.assertIn("report only", out)
        self.assertEqual(dump(db), before)
        code, out = self.run_tool("--measured", self.tsv, "--apply", "--no-backup")
        self.assertEqual(code, 0, out)
        written = db.execute("SELECT race, class, humtype, faction, standing, source FROM factionstart").fetchall()
        self.assertEqual(len(written), 8)
        self.assertTrue(all(r[5] == "seed" for r in written))
        own = dict(db.execute("SELECT name, standing FROM factions"))
        self.assertEqual((own["Delta Watch "], own[NEW], own["Edited"]), (50, 0, 7))
        self.assertNotIn(JUNK, own)
        # every measured combo comes out at its measurement, as the server reads the table
        rows = {}
        for r in written:
            rows.setdefault(r[3], []).append((r[0], r[1], r[2], r[4]))
        for r, c, h in COMBOS:
            for name in MEASURED:
                if name in ("Edited", JUNK):
                    continue  # a GM's own standing, left; junk
                self.assertEqual(sfs.resolve(rows.get(name, []), own[name], sfs.combo_of(r, c, h)), standing(r, c, h, name),
                                 (r, c, h, name))
        # a second --apply is refused
        with self.assertRaises(SystemExit):
            with contextlib.redirect_stderr(io.StringIO()):
                self.run_tool("--measured", self.tsv, "--apply", "--no-backup")
        # a GM changes one seeded row, a character moves the new faction: --undo leaves the row and says both
        db.execute("UPDATE factionstart SET standing = 999 WHERE race = 2 AND class = 11")
        db.execute("INSERT INTO charfactions VALUES (5, ?, 650)", (NEW,))
        code, out = self.run_tool("--undo", "--no-backup")
        self.assertEqual(code, 0)
        self.assertIn("now, the seed wrote", out)
        self.assertIn("characters' standings in 'Order of Epsilon'", out)
        left = db.execute("SELECT race, class, faction, standing FROM factionstart").fetchall()
        self.assertEqual(left, [(2, 11, "Guild of Gamma", 999)])
        db.execute("DELETE FROM factionstart")
        db.execute("DELETE FROM charfactions")
        self.assertEqual(dump(db), before)

    def test_the_table_is_the_servers(self):
        here = os.path.dirname(os.path.abspath(__file__))
        with open(os.path.join(here, "..", "internal", "database", "factions.go"), encoding="utf-8") as f:
            go = f.read()
        server = re.search(r"CREATE TABLE IF NOT EXISTS factionstart \((.*?)\);", go, re.S).group(1)
        ours = sqlite3.connect(":memory:")
        ours.execute(sfs.SCHEMA)
        theirs = sqlite3.connect(":memory:")
        theirs.execute(f"CREATE TABLE factionstart ({server})")
        self.assertEqual([r[1:] for r in ours.execute("PRAGMA table_info(factionstart)")],
                         [r[1:] for r in theirs.execute("PRAGMA table_info(factionstart)")])
        ours.close()
        theirs.close()


if __name__ == "__main__":
    unittest.main()
