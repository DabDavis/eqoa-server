#!/usr/bin/env python3
"""mermaid_kit_20261112.py on a small database: the report writes nothing; --apply writes the 14 rows (4 spells' two
rows each, 6 ranks), a gm_audit line each; a second apply does nothing; --undo puts every table back exactly. The
controls: an id taken, another row with the Mermaid's bit, a spell of one of the names, or racedefs 32 not the Mermaid,
is refused, writing nothing. And the spells' effects and the ranks' are the ones internal/entity/mermaidkit_test.go
casts and sums."""
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import mermaid_kit_20261112 as mk  # noqa: E402

TOOL = os.path.join(HERE, "mermaid_kit_20261112.py")
GOTEST = os.path.join(HERE, "..", "internal", "entity", "mermaidkit_test.go")

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
    requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '',
    adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '',
    remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'gm', require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '');
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
    effects TEXT NOT NULL DEFAULT '');
CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO spellpattern VALUES (901784, 6, 0, 0, 16880, 0, 30, 486870879, 829958967, 1, 10, -1, 'Cantrip', '');
INSERT INTO cmcatalog (id, races, name) VALUES (1001, 64, 'Blood of the North'), (2141, 2147483648, 'Will-of-the-Wisp'),
    (1, 2047, 'Everyone');
"""


def make(path, race="Mermaid", extra=""):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO racedefs VALUES (32, ?)", (race,))
    db.executescript(extra)
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l and "sqlite_sequence" not in l]
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


class Tool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 14 rows new", r.stdout)
        self.assertEqual(dump(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM cmcatalog WHERE races = ?", (1 << 32,)).fetchone()[0], 6)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 14)
        self.assertEqual(db.execute("SELECT description FROM cmcatalog WHERE id = 2173").fetchone()[0], "You are moist.")
        self.assertEqual(db.execute("SELECT spent, category FROM cmcatalog WHERE id = 2174").fetchone(), (500, 4))
        self.assertEqual(db.execute("SELECT group_concat(add_spells, ';') FROM cmcatalog WHERE id BETWEEN 2171 AND 2176")
                         .fetchone()[0], "760175;760176;;;760177;760178")
        self.assertEqual(db.execute("SELECT count(*) FROM cmcatalog WHERE id BETWEEN 2177 AND 2182").fetchone()[0], 0,
                         "Aquatic Birthright and Ascension wait for the owner")
        db.close()
        applied = dump(self.db)
        r = run(self.db, "--apply")
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.db), applied)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_refusals(self):
        for extra, race in (("INSERT INTO cmcatalog (id, races, name) VALUES (2199, 4294967296, 'Placeholder');", "Mermaid"),
                            ("INSERT INTO cmcatalog (id, name) VALUES (2173, 'Something');", "Mermaid"),
                            ("INSERT INTO spellpattern VALUES (5, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Splash', '');", "Mermaid"),
                            ("INSERT INTO spelleffect VALUES (760177, 'x', 0, 'gm', '');", "Mermaid"),
                            ("", "Siren")):
            if os.path.exists(self.db):
                os.remove(self.db)
            make(self.db, race, extra)
            before = dump(self.db)
            r = run(self.db, "--apply")
            self.assertEqual(r.returncode, 1, r.stdout)
            self.assertIn("refused", r.stdout)
            self.assertEqual(dump(self.db), before)

    def test_effects_are_the_go_tests(self):
        go = open(GOTEST).read()
        consts = dict(re.findall(r"(\w+)\s*=\s*`([^`]*)`", go))
        for (sp, se), name in zip(mk.SPELLS, ("hydrate", "splash", "call", "kiss")):
            self.assertEqual(consts[name], se["effects"], sp["spellname"])
        ranks = dict(re.findall(r'\{(\d{4}), "([^"]*)"\}', go))
        for r in mk.ROWS:
            self.assertEqual(ranks[str(r["id"])], r["effects"], r["id"])

    def test_splash_is_cantrip(self):
        """Splash's pattern is the gnome's Cantrip's but its name, and its hit Cantrip's 5 and 300% of the level."""
        splash = mk.SPELLS[1][0]
        cantrip = {"abilitylvl": 6, "range": 16880, "casttime": 0, "power": 30, "iconColor": 486870879,
                   "icon": 829958967, "scope": 1, "recast": 10}
        self.assertEqual({k: splash[k] for k in cantrip}, cantrip)
        self.assertIn('"amount":-5,', mk.SPELLS[1][1]["effects"])
        self.assertIn('"level_pct":300', mk.SPELLS[1][1]["effects"])


if __name__ == "__main__":
    unittest.main()
