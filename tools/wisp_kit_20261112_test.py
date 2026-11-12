#!/usr/bin/env python3
"""wisp_kit_20261112.py on a small database holding the first kit (tools/wisp_kit_20261111.py's rows, Trickster Flame
as tools/wisp_raiment_20261111.py left it) and one owner of Pale Fire: the report writes nothing; --apply writes the
10 rows, changes the 4 and gives the owner Metaphysical Wound, a gm_audit line each; a second apply does nothing;
--undo puts every table back exactly. The controls: a row of the first kit changed, an id taken, another row with the
Wisp's bit, or racedefs 31 not the Wisp, is refused, writing nothing. And the two spells' effects are the ones
internal/entity/wispkit_test.go casts."""
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import wisp_kit_20261111 as first  # noqa: E402
import wisp_kit_20261112 as wk  # noqa: E402
import wisp_raiment_20261111 as raiment  # noqa: E402

TOOL = os.path.join(HERE, "wisp_kit_20261112.py")
GOTEST = os.path.join(HERE, "..", "internal", "entity", "wispkit_test.go")

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
CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY);
CREATE TABLE charcms (serverid INTEGER NOT NULL, cmid INTEGER NOT NULL, cost INTEGER NOT NULL DEFAULT 0,
    bought INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, cmid));
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL,
    addedorder INTEGER NOT NULL, onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL,
    showhide INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO spellpattern VALUES (906720, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Wisp''s Raiment', '');
INSERT INTO cmcatalog (id, races, name) VALUES (1001, 64, 'Blood of the North');
INSERT INTO characters VALUES ('Wisp', 676), ('Fenora', 743);
INSERT INTO charcms VALUES (676, 2141, 5, 0), (676, 2143, 200, 0), (676, 2145, 75, 0), (743, 2143, 200, 0);
INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (676, 496, 3, 0, -1, 0, -1);
"""


def make(path, race="Wisp", extra=""):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for r in first.ROWS:
        cols = list(r)
        db.execute(f"INSERT INTO cmcatalog ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
    a = raiment.FLAME_AFTER
    db.execute("UPDATE cmcatalog SET adds = ?, add_spells = ?, description = ? WHERE id = 2142",
               (a["adds"], a["add_spells"], a["description"]))
    db.execute("INSERT INTO racedefs VALUES (31, ?)", (race,))
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
        self.assertIn("report only: 10 rows new, 4 changed, 1 book(s)", r.stdout)
        self.assertEqual(dump(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM cmcatalog WHERE races = ?", (1 << 31,)).fetchone()[0], 11)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 15)
        self.assertEqual(db.execute("SELECT name, add_spells, effects FROM cmcatalog WHERE id = 2145").fetchone(),
                         ("Metaphysical Wound", "760174", ""))
        self.assertEqual(db.execute("SELECT effects FROM cmcatalog WHERE id = 2142").fetchone()[0], "")
        self.assertEqual(db.execute("SELECT group_concat(effects, ';') FROM cmcatalog WHERE id BETWEEN 2147 AND 2151")
                         .fetchone()[0], "HP:-25,Pow:50;HP:-50,Pow:100;HP:-75,Pow:150;HP:-100,Pow:200;HP:-125,Pow:250")
        self.assertEqual(db.execute("SELECT serverid, addedorder FROM spells WHERE spellid = 760174").fetchall(), [(676, 4)])
        db.close()
        applied = dump(self.db)
        r = run(self.db, "--apply")
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.db), applied)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_refusals(self):
        for extra, race in (("INSERT INTO cmcatalog (id, races, name) VALUES (2199, 2147483648, 'Placeholder');", "Wisp"),
                            ("INSERT INTO cmcatalog (id, name) VALUES (2148, 'Something');", "Wisp"),
                            ("UPDATE cmcatalog SET effects = 'Pow:400' WHERE id = 2144;", "Wisp"),
                            ("", "Jal'Raeth")):
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
        for (sp, se), name in zip(wk.SPELLS, ("ponder", "wound")):
            self.assertEqual(consts[name], se["effects"], sp["spellname"])
        ranks = dict(re.findall(r'\{(\d{4}), "([^"]*)"\}', go))
        for id, _, after in wk.CHANGES:
            self.assertEqual(ranks[str(id)], after["effects"], id)
        for r in wk.NEW_ROWS:
            self.assertEqual(ranks[str(r["id"])], r["effects"], r["id"])
        self.assertEqual(ranks["2141"], first.ROWS[0]["effects"])


if __name__ == "__main__":
    unittest.main()
