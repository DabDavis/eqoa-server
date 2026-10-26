#!/usr/bin/env python3
"""tools/fix_spell_records.py against a made-up database whose rows are built
from the tool's own tables: a record's numbers, a shaman spell's scope (with
its range and frame), a dump's link and the second invisibility unlinked
(books kept, holders named), a duration added; and what it must leave alone
(a record a GM changed, numbers not the expected ones, a line that names
another target, a link of another source, a class with no other
invisibility, a line a GM changed, a line not the expected one, a record two
classes share). --undo exact, and a row changed after the fix left at
--undo. No game data.

    python3 tools/fix_spell_records_test.py
"""
import importlib.util
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_spell_records.py")

spec = importlib.util.spec_from_file_location("fix_spell_records", TOOL)
fsr = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fsr)

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE characters (charName TEXT UNIQUE NOT NULL, serverid INTEGER PRIMARY KEY AUTOINCREMENT, tclass INTEGER NOT NULL);
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, spellid INTEGER NOT NULL,
    addedorder INTEGER NOT NULL, onHotBar INTEGER NOT NULL, whereonBar INTEGER NOT NULL, unk1 INTEGER NOT NULL,
    showhide INTEGER NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '');
"""

PAL, RGE, DRD, SHA, MAG, WIZ, ALC = 2, 6, 7, 8, 10, 13, 14
SELF = (fsr.SELF_RANGE, fsr.SELF_FRAME)


class Fixture:
    """A made-up database, a record at a time."""

    def __init__(self, path):
        self.db = sqlite3.connect(path)
        self.db.executescript(SCHEMA)
        self.next = 900001

    def spell(self, cls, name, lvl, nums=(1, 1, 1), scope=0, rng=SELF, line=None, source="eqoa-data; made", also=()):
        sid = self.next
        self.next += 1
        cast, recast, power = nums
        self.db.execute("INSERT INTO spellpattern VALUES (?, ?, 0, 0, ?, ?, ?, ?, 0, ?, ?, -1, ?, '')",
                        (sid, lvl, rng[0], cast, power, rng[1], scope, recast, name))
        for c in (cls,) + tuple(also):
            self.db.execute("INSERT INTO classspells VALUES (?, ?, ?)", (c, sid, source))
        if line is not None:
            self.db.execute("INSERT INTO spelleffect VALUES (?, ?, 0, 'eqoa-data list (Merchant); made')", (sid, line))
        return sid

    def knows(self, name, cls, *sids):
        cur = self.db.execute("INSERT INTO characters (charName, tclass) VALUES (?, ?)", (name, cls))
        for i, sid in enumerate(sids):
            self.db.execute("INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) "
                            "VALUES (?, ?, ?, 0, -1, 0, -1)", (cur.lastrowid, sid, i))

    def gm(self, tbl, **key):
        self.db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES (?, ?)", (tbl, json.dumps(key)))

    def close(self):
        self.db.commit()
        self.db.close()


def title(name):
    return " ".join(w[:1].upper() + w[1:] for w in name.split())


def make(path):
    """The database, and the spell ids the tests look at by name."""
    f = Fixture(path)
    ids = {}
    # 1. records: two the tool fixes, one a GM changed, one not as expected
    (cls, name, lvl), (now, want, _) = next((k, v) for k, v in fsr.RECORDS.items() if k[0] == PAL)
    ids["record"] = f.spell(cls, title(name), lvl, now, scope=1, rng=(16880, 486870879))
    ids["record_want"] = want
    shaman = [(k, v) for k, v in fsr.RECORDS.items() if k[0] == SHA]
    (cls, name, lvl), (now, want, _) = shaman[0]
    # its record is a self record too: numbers and scope at once
    ids["both"] = f.spell(cls, title(name), lvl, now)
    ids["both_want"] = want
    ids["both_scope"] = fsr.SCOPES[(name, lvl)][0]
    (cls, name, lvl), (now, _, _) = shaman[1]
    ids["gm_record"] = f.spell(cls, title(name), lvl, now)
    f.gm("spellpattern", spellid=ids["gm_record"])
    (cls, name, lvl), (now, _, _) = shaman[2]
    ids["other_nums"] = f.spell(cls, title(name), lvl, (now[0] + 1, now[1], now[2]), scope=1, rng=(16880, 486870879))
    # 2. scopes: a group spell, and one whose line names another target
    group = [(k, v) for k, v in fsr.SCOPES.items() if v[0] == 2]
    target = [(k, v) for k, v in fsr.SCOPES.items() if v[0] == 1 and k not in {(n, l) for (_, n, l) in fsr.RECORDS}]
    (name, lvl), _ = group[0]
    ids["group"] = f.spell(SHA, title(name), lvl)
    (name, lvl), _ = target[0]
    ids["self_line"] = f.spell(SHA, title(name), lvl, line="Self, Increase STR by 5, 1min")
    (name, lvl), _ = target[1]
    ids["shared"] = f.spell(SHA, title(name), lvl, also=(DRD,))
    # 3. links: a dump's (its holder named, its book kept), a link of another
    # source, the second invisibility (a holder of both), a class without the
    # first
    (cls, name, lvl), (source, _, _) = next((k, v) for k, v in fsr.LINKS.items() if k[0] == MAG)
    ids["stray"] = f.spell(fsr.CLASSES.index("WAR"), title(name), lvl, source="vendor")
    f.db.execute("INSERT INTO classspells VALUES (?, ?, ?)", (cls, ids["stray"], source))
    f.knows("Magus", MAG, ids["stray"])
    (cls, name, lvl), (source, _, _) = next((k, v) for k, v in fsr.LINKS.items() if k[0] == DRD)
    ids["vendor_link"] = f.spell(cls, title(name), lvl, source="vendor")
    ids["invis"] = f.spell(SHA, "Invisibility", 8, scope=1, line="Target, invis, 5.5min", source="vendor", also=(WIZ,))
    ids["invis2"] = f.spell(SHA, "Invisibilty", 8, source="eqoa-data", also=(WIZ,))
    f.db.execute("DELETE FROM classspells WHERE class = ? AND spellid = ?", (WIZ, ids["invis"]))
    f.knows("Shammy", SHA, ids["invis"], ids["invis2"])
    # 4. durations: one added, a line a GM changed, a line not the expected one
    durs = list(fsr.DURATIONS.items())
    (cls, name, lvl), (line, time, _) = durs[0]
    ids["dur"] = f.spell(cls, title(name), lvl, line=line)
    ids["dur_want"] = f"{line}, {time}"
    (cls, name, lvl), (line, _, _) = durs[1]
    ids["gm_line"] = f.spell(cls, title(name), lvl, line=line)
    ids["gm_line_text"] = line
    f.gm("spelleffect", spellid=ids["gm_line"])
    (cls, name, lvl), (line, _, _) = durs[2]
    ids["other_line"] = f.spell(cls, title(name), lvl, line=line + ", 10min")
    f.close()
    return ids


def run(db, *args):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", db, *args], capture_output=True, text=True, check=True).stdout


def dump(db):
    c = sqlite3.connect(db)
    out = "\n".join(c.iterdump())
    c.close()
    return out


def record(db, sid):
    c = sqlite3.connect(db)
    r = c.execute("SELECT casttime, recast, power, scope, range, iconColor FROM spellpattern WHERE spellid = ?", (sid,)).fetchone()
    c.close()
    return r


def links(db, sid):
    c = sqlite3.connect(db)
    r = sorted(cls for (cls,) in c.execute("SELECT class FROM classspells WHERE spellid = ?", (sid,)))
    c.close()
    return r


def line(db, sid):
    c = sqlite3.connect(db)
    r = c.execute("SELECT line FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
    c.close()
    return r and r[0]


class FixSpellRecords(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        self.ids = make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        before = dump(self.db)
        out = run(self.db)
        self.assertIn("report only", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_then_undo(self):
        ids, before = self.ids, dump(self.db)
        out = run(self.db, "--apply")
        # 1. records
        self.assertEqual(record(self.db, ids["record"])[:3], ids["record_want"])
        cast, recast, power, scope, rng, frame = record(self.db, ids["both"])
        self.assertEqual((cast, recast, power), ids["both_want"])
        self.assertEqual((scope, rng, frame), (ids["both_scope"], fsr.SCOPE_RANGE[ids["both_scope"]],
                                               fsr.SCOPE_FRAME[ids["both_scope"]]), "numbers and scope at once")
        self.assertEqual(record(self.db, ids["gm_record"])[3], 0, "a record a GM changed is left")
        self.assertIn("a GM changed record", out)
        self.assertNotEqual(record(self.db, ids["other_nums"])[:3], ids["record_want"])
        self.assertIn("not (", out, "numbers not the expected ones are said")
        # 2. scopes
        self.assertEqual(record(self.db, ids["group"])[3:], (2, fsr.SCOPE_RANGE[2], fsr.SCOPE_FRAME[2]))
        self.assertEqual(record(self.db, ids["self_line"])[3], 0, "a line naming another target keeps its scope")
        self.assertIn("its line says otherwise", out)
        self.assertEqual(record(self.db, ids["shared"])[3], 0, "a record another class shares is left")
        self.assertIn("is also DRD's", out)
        # 3. links
        self.assertEqual(links(self.db, ids["stray"]), [fsr.CLASSES.index("WAR")])
        self.assertIn("Magus", out, "a holder of an unlinked spell is named")
        self.assertEqual(links(self.db, ids["vendor_link"]), [DRD], "a link of another source is left")
        self.assertEqual(links(self.db, ids["invis2"]), [WIZ], "the shaman's second invisibility goes; the "
                                                                "wizard, with no first, keeps it")
        self.assertIn("Shammy", out)
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT count(*) FROM spells").fetchone()[0], 3, "no book is changed")
        c.close()
        # 4. durations
        self.assertEqual(line(self.db, ids["dur"]), ids["dur_want"])
        self.assertEqual(line(self.db, ids["gm_line"]), ids["gm_line_text"], "a line a GM changed keeps its words")
        self.assertIn("a GM changed the row of", out)
        self.assertIn("its line is not the one expected", out)
        # a second run changes nothing, and --undo is exact
        self.assertIn("0 rows to change", run(self.db))
        run(self.db, "--undo")
        self.assertEqual(dump(self.db), before, "--undo puts the database back as it was")

    def test_undo_leaves_a_row_changed_since(self):
        run(self.db, "--apply")
        c = sqlite3.connect(self.db)
        c.execute("UPDATE spellpattern SET power = 999 WHERE spellid = ?", (self.ids["record"],))
        c.commit()
        c.close()
        out = run(self.db, "--undo")
        self.assertIn("changed since", out)
        self.assertEqual(record(self.db, self.ids["record"])[2], 999)
        self.assertEqual(record(self.db, self.ids["group"])[3], 0, "the rest is put back")


if __name__ == "__main__":
    unittest.main()
