#!/usr/bin/env python3
"""ability_cm_charts_20261121.py: apply, a second apply, undo, --with-review, --skip, and rows changed since."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ability_cm_charts_20261121 as tool  # noqa: E402

SPELL_COLS = ("abilitylvl", "power", "casttime", "recast", "range")
CM_COLS = ("cost", "level", "minstats", "effects", "adds", "add_spells")


def make(path):
    """A database holding every row the tool names, each field as the tool expects to read it."""
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER DEFAULT 1, "
               "power INTEGER DEFAULT 0, casttime INTEGER DEFAULT 0, recast INTEGER DEFAULT 0, range INTEGER DEFAULT 0)")
    db.execute("CREATE TABLE classspells (class INTEGER, spellid INTEGER, source TEXT, PRIMARY KEY (class, spellid))")
    db.execute("CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, mastery TEXT, rank INTEGER, classes INTEGER, "
               "enabled INTEGER DEFAULT 1, cost INTEGER DEFAULT 0, level INTEGER DEFAULT 1, minstats TEXT DEFAULT '', "
               "effects TEXT DEFAULT '', adds TEXT DEFAULT '', add_spells TEXT DEFAULT '')")
    db.execute("CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, "
               "ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT)")
    ids = {}
    for c in tool.CHANGES:
        if c["key"] == "death-knight-rest":  # the same row as death-knight-ac: its effects are the ac change's new
            db.execute("UPDATE cmcatalog SET minstats = ? WHERE mastery = 'Death Knight'", (c["fields"]["minstats"][0],))
            continue
        if c["kind"] == "spell":
            k = ("s", c["name"])
            if k not in ids:
                ids[k] = 100 + len(ids)
                db.execute("INSERT INTO spellpattern (spellid, spellname) VALUES (?, ?)", (ids[k], c["name"]))
            db.execute("INSERT OR IGNORE INTO classspells VALUES (?, ?, 'test')", (c["cls"], ids[k]))
            for f, (old, new) in c["fields"].items():
                db.execute(f"UPDATE spellpattern SET {f} = ? WHERE spellid = ?", (old, ids[k]))
        else:
            k = ("c", c["name"], c["rank"], c["cls"])
            ids[k] = 1000 + len(ids)
            db.execute("INSERT INTO cmcatalog (id, mastery, rank, classes) VALUES (?, ?, ?, ?)",
                       (ids[k], c["name"], c["rank"], 1 << c["cls"]))
            for f, (old, new) in c["fields"].items():
                db.execute(f"UPDATE cmcatalog SET {f} = ? WHERE id = ?", (old, ids[k]))
    db.commit()
    db.close()


def run(*args):
    with contextlib.redirect_stdout(io.StringIO()) as out:
        tool.main(list(args))
    return out.getvalue()


class T(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "a.db")
        make(self.path)
        self.before = self.snapshot()

    def tearDown(self):
        self.dir.cleanup()

    def snapshot(self):
        db = sqlite3.connect(self.path)
        try:
            return (db.execute(f"SELECT spellid, {', '.join(SPELL_COLS)} FROM spellpattern ORDER BY 1").fetchall(),
                    db.execute(f"SELECT id, {', '.join(CM_COLS)} FROM cmcatalog ORDER BY 1").fetchall())
        finally:
            db.close()

    def value(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchone()[0]
        finally:
            db.close()

    def test_apply_again_undo(self):
        out = run("--db", self.path)
        self.assertEqual(self.snapshot(), self.before, "a dry run writes nothing")
        self.assertIn("todo", out)
        run("--db", self.path, "--apply")
        self.assertEqual(self.value("SELECT abilitylvl FROM spellpattern WHERE spellname = 'Circle of Rathe'"), 50)
        self.assertEqual(self.value("SELECT cost FROM cmcatalog WHERE mastery = 'Primal Scream' AND rank = 3"), 20)
        self.assertEqual(self.value("SELECT add_spells FROM cmcatalog WHERE mastery = 'Breathy Ballad' AND rank = 1"),
                         "700883")
        # the review group is left alone without --with-review
        self.assertEqual(self.value("SELECT abilitylvl FROM spellpattern WHERE spellname = 'Mortal Mask'"), 3)
        n = self.value("SELECT count(*) FROM gm_audit")
        self.assertEqual(n, sum(1 for c in tool.CHANGES if c["group"] == "apply"))
        self.assertIn("applied: 0 rows", run("--db", self.path, "--apply"))
        self.assertEqual(self.value("SELECT count(*) FROM gm_audit"), n)
        run("--db", self.path, "--undo")
        self.assertEqual(self.snapshot(), self.before)

    def test_with_review_and_double_change_undo(self):
        run("--db", self.path, "--apply", "--with-review")
        self.assertEqual(self.value("SELECT abilitylvl FROM spellpattern WHERE spellname = 'Mortal Mask'"), 27)
        self.assertEqual(self.value("SELECT effects FROM cmcatalog WHERE mastery = 'Death Knight'"),
                         "STAMax:50,AGIMax:50,HP:300,AC:550,Def Mod:2,FR:25,LR:25,CR:25")
        run("--db", self.path, "--undo")
        self.assertEqual(self.snapshot(), self.before, "the Death Knight's two changes undo to the first value")

    def test_skip(self):
        run("--db", self.path, "--apply", "--skip", "circle-rathe", "--skip", "hero")
        self.assertEqual(self.value("SELECT abilitylvl FROM spellpattern WHERE spellname = 'Circle of Rathe'"), 60)
        self.assertEqual(self.value("SELECT minstats FROM cmcatalog WHERE mastery = 'Hero'"), "STR:200,STA:250")
        with self.assertRaises(SystemExit):
            run("--db", self.path, "--skip", "no-such-key")

    def test_refuses_a_row_changed_since(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE spellpattern SET abilitylvl = 58 WHERE spellname = 'Binding Grasp'")
        db.commit()
        db.close()
        out = run("--db", self.path, "--apply")
        self.assertIn("refused", out)
        self.assertEqual(self.value("SELECT abilitylvl FROM spellpattern WHERE spellname = 'Binding Grasp'"), 58)

    def test_undo_leaves_a_field_changed_since(self):
        run("--db", self.path, "--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE spellpattern SET abilitylvl = 52 WHERE spellname = 'Mana Flow'")
        db.commit()
        db.close()
        out = run("--db", self.path, "--undo")
        self.assertIn("1 left", out)
        self.assertEqual(self.value("SELECT abilitylvl FROM spellpattern WHERE spellname = 'Mana Flow'"), 52)
        self.assertEqual(self.value("SELECT abilitylvl FROM spellpattern WHERE spellname = 'Circle of Rathe'"), 60)


if __name__ == "__main__":
    unittest.main()
