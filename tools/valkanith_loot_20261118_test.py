#!/usr/bin/env python3
"""valkanith_loot_20261118.py on a small database: the report writes nothing; --apply makes the records, the spells,
the pet sheet, the itemspell rows, the Ascended copies (the server's rule) and both tables, renames the Bone Wing
Sentinel and gives its held copies their charges; a second --apply does nothing; --undo puts every row back exactly
(the id counters too); a made name already there is refused whole. (TestRealValkanithLoot runs it on a shard's
copy.)"""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import valkanith_loot_20261118 as vl  # noqa: E402

ITEM_COLS = ("itemcost", "itemicon", "equipslot", "trade", "rent", "levelreq", "classuse", "raceuse", "lore", "maxstack",
             "attacktype", "maxhp", "duration", "itemname", "itemdesc", "model", "weapondamage") + vl.STAT_COLS
SCHEMA = f"""
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, {", ".join(f'"{c}"' for c in ITEM_COLS)});
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
                         source TEXT NOT NULL DEFAULT 'gm', charges INTEGER NOT NULL DEFAULT 0);
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
                         weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1,
                         maxlevel INTEGER NOT NULL DEFAULT 60, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
                      rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
                      PRIMARY KEY (npc_name, tablename));
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER, casttime INTEGER, power INTEGER,
                           scope INTEGER, recast INTEGER, spellname TEXT, spelldesc TEXT);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL,
                          source TEXT NOT NULL, effects TEXT NOT NULL DEFAULT '');
CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, sheet_pet_id TEXT, name TEXT, model INTEGER, primary_model INTEGER,
                       secondary_model INTEGER, level INTEGER, hp INTEGER, abilities TEXT, keep_template INTEGER,
                       note TEXT, source TEXT, size REAL);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, remaincharge INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
                       username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL,
                       tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT,
                       detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""
SIBLINGS = ["Circlet of Loathing", "Dazzling Mace", "Terra Cloth Tunic", "Etheric cloth gloves", "Etheric Cloth pants",
            "etheric cloth boots", "badlands helm", "Etheric plate helm", "etheric breastplate", "etheric plate bracers",
            "etheric plate gauntlets", "etheric plate greaves", "Etheric Plate Boots"]


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(out):
        rc = vl.main(list(args))
    return rc, out.getvalue()


def item(db, name, **kw):
    r = {c: 0 for c in ITEM_COLS}
    r.update(itemname=name, itemdesc="", rent=1, classuse=32767, raceuse=1023, maxstack=1, equipslot=1)
    r.update(kw)
    cur = db.execute(f'INSERT INTO itempattern ({", ".join(chr(34) + c + chr(34) for c in r)}) '
                     f'VALUES ({", ".join("?" * len(r))})', list(r.values()))
    return cur.lastrowid


def digest(db):
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'gm_audit' ORDER BY name"):
        out[t] = [r for r in db.execute(f'SELECT * FROM "{t}" ORDER BY 1').fetchall() if r[0] != "gm_audit"]
    return out


class ValkanithLootTest(unittest.TestCase):
    def make(self):
        fd, path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        self.addCleanup(os.remove, path)
        db = sqlite3.connect(path)
        db.executescript(SCHEMA)
        for s in SIBLINGS:
            item(db, s, AC=10)
        self.shield = item(db, "positronian energy shield", equipslot=13, AC=139)
        self.phyl = item(db, "phylactery of valkanith", equipslot=4, PoT=25)
        self.brace = item(db, "Thexian bracelet", equipslot=6, AC=22)
        self.rune = item(db, "Bone Wing Sentinel", equipslot=-1, trade=1)
        db.execute("INSERT INTO itemeffect (patternid, line, name, charges, source) VALUES (?, 'Self, x', 'Thexian Bonewing', "
                   "100, 'valkanith1117')", (self.rune,))
        for pid in (self.rune, self.phyl, self.shield):
            db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                       "VALUES ('boss lord valkanith', ?, 1, 1, 70, 'valkanith1117')", (pid,))
        db.execute("INSERT INTO npcloot VALUES ('lord valkanith', 'boss lord valkanith', 100, 1, 'x', 'valkanith1117')")
        for sid, name in ((vl.SUMMON_FROM, "Summon Servant"), (vl.BUFF_FROM, "Undead Servitude")):
            db.execute("INSERT INTO spellpattern VALUES (?, 60, 2, 500, 0, 3, ?, '')", (sid, name))
            db.execute("INSERT INTO spelleffect VALUES (?, 'x', 7, 'x', '')", (sid,))
        db.execute("INSERT INTO petstats VALUES (700766, '006E', 'Arch Familiar', 11, 0, 0, 55, 4540, '[]', 0, '', 'x', 0)")
        db.execute("INSERT INTO petstats VALUES (701236, '0064', 'Undead Servant', 1794738819, 0, 0, 45, 3203, '[]', 0, "
                   "'', 'x', 0)")
        db.execute("INSERT INTO charinventory VALUES (1, 661, ?, 0)", (self.rune,))
        db.execute("INSERT INTO charinventory VALUES (2, 662, ?, 57)", (self.rune,))
        db.commit()
        db.close()
        return path

    def test_apply_again_undo(self):
        path = self.make()
        db = sqlite3.connect(path)
        before = digest(db)
        db.close()
        rc, out = run("--db", path, "--no-backup")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        self.assertEqual(digest(db), before, "the report wrote")
        db.close()

        rc, out = run("--db", path, "--apply", "--no-backup")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        names = sorted(r[0] for r in db.execute("SELECT p.itemname FROM loottables l JOIN itempattern p USING (patternid) "
                                                "WHERE tablename = 'boss lord valkanith'"))
        self.assertEqual(names, sorted(["Ascended Crown of Tal'thex", "Ascended positronian energy shield",
                                        "Ascended Positronian Mace", "Ascended phylactery of valkanith",
                                        "Rune of the Bonewing", "Rune of Defilement"]))
        # the Ascended copies: the server's rule (a tenth more, half up)
        ac, pot = db.execute("SELECT AC, PoT FROM itempattern WHERE itemname = 'Ascended positronian energy shield'").fetchone()[0], \
            db.execute("SELECT PoT FROM itempattern WHERE itemname = 'Ascended phylactery of valkanith'").fetchone()[0]
        self.assertEqual((ac, pot), (153, 28))
        self.assertEqual(db.execute("SELECT AC, HoT, lore FROM itempattern WHERE itemname = 'Ascended Crown of Tal''thex'")
                         .fetchone(), (30, 28, 1))
        self.assertEqual(db.execute("SELECT weapondamage, equipslot FROM itempattern WHERE itemname = "
                                    "'Ascended Positronian Mace'").fetchone(), (715, 14))
        self.assertEqual(db.execute("SELECT count(*) FROM itemascended").fetchone()[0], 4)
        wave = db.execute("SELECT count(*) FROM loottables WHERE tablename = 'tal''thex armour'").fetchone()[0]
        self.assertEqual(wave, 13)
        self.assertEqual(sorted(r[0] for r in db.execute("SELECT npc_name FROM npcloot WHERE tablename = 'tal''thex armour'")),
                         sorted(vl.WAVES))
        self.assertEqual(db.execute("SELECT itemname, lore FROM itempattern WHERE patternid = ?", (self.rune,)).fetchone(),
                         ("Rune of the Bonewing", 1))
        self.assertEqual(db.execute("SELECT spellid FROM itemspell WHERE patternid = ?", (self.rune,)).fetchone()[0],
                         vl.SUMMON)
        self.assertEqual([r[0] for r in db.execute("SELECT remaincharge FROM charinventory ORDER BY itemid")], [100, 57])
        self.assertEqual(db.execute("SELECT classuse FROM itempattern WHERE patternid = ?", (self.brace,)).fetchone()[0],
                         vl.CASTER)
        model, name, ab = db.execute("SELECT model, name, abilities FROM petstats WHERE spellid = ?", (vl.SUMMON,)).fetchone()
        self.assertEqual((model, name), (1794738819, "Thexian Bonewing"))
        self.assertIn('"while_out":true', ab)
        self.assertEqual(db.execute("SELECT count(*) FROM spellpattern WHERE spellid IN (?, ?, ?)",
                                    (vl.SUMMON, vl.BUFF, vl.DEFILE)).fetchone()[0], 3)
        db.close()

        rc, out = run("--db", path, "--apply", "--no-backup")
        self.assertIn("nothing to do", out)
        rc, out = run("--db", path, "--undo", "--no-backup")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        after = digest(db)
        after.pop("itemascended", None)  # made empty by the tool (the server makes it at its start too)
        after.pop("itemspell", None)
        self.assertEqual(db.execute("SELECT count(*) FROM itemascended").fetchone()[0], 0)
        self.assertEqual(db.execute("SELECT count(*) FROM itemspell").fetchone()[0], 0)
        self.assertEqual(after, before)
        db.close()

    def test_refused_whole(self):
        path = self.make()
        db = sqlite3.connect(path)
        item(db, "Crown of Tal'thex")
        db.commit()
        before = digest(db)
        db.close()
        rc, out = run("--db", path, "--apply", "--no-backup")
        self.assertEqual(rc, 1)
        self.assertIn("exists already", out)
        db = sqlite3.connect(path)
        self.assertEqual(digest(db), before)
        db.close()


if __name__ == "__main__":
    unittest.main()
