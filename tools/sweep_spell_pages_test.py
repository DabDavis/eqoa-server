#!/usr/bin/env python3
"""tools/sweep_spell_pages.py against made-up pages and a made-up database:
an imported self spell its page says is cast at a target becomes one with
the importer's target range and frame and the page's numbers; a line of
prose becomes the page's effects; a line with no time takes the page's; a
value an earlier fix chose is left; a record two classes share whose pages
disagree is left; a line whose figures differ is listed and kept; a record
no importer made is never touched; a second --apply has nothing to do and
--undo gives back the database as it was. No game data.

    python3 tools/sweep_spell_pages_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "sweep_spell_pages.py")
sys.path.insert(0, HERE)
import fetch_spell_pages as fetch  # noqa: E402
import sweep_spell_pages as sweep  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE eqoadata_spell_import_rows (seq INTEGER PRIMARY KEY, tbl TEXT, key TEXT, key2 TEXT, note TEXT);
CREATE TABLE spell_fill_log (seq INTEGER PRIMARY KEY, part TEXT, tbl TEXT, rowkey TEXT, action TEXT, before TEXT, after TEXT, note TEXT);
"""
MNK, WIZ, NEC = 4, 13, 11


def page(name, cls, lvl, effect, cast, recast, power, scope, duration):
    """A spell page as Allakhazam's are laid out, as far as the reader reads."""
    fields = [("Effect", effect), ("Class", f"{cls}, level {lvl}"), ("Casting Time", cast), ("Recast", recast),
              ("Power Cost", power), ("Scope", scope), ("Duration", duration), ("Resist", "Fire"), ("AE Radius", "0"),
              ("Hate Rating", "0"), ("Last Import Date", "2004-04-28 13:48:06")]
    rows = "".join(f"<tr><td><b>{k}:</b></td><td>{v}</td></tr>" for k, v in fields)
    return (f"<html><head><title>{name} :: Spells :: EverQuest Online Adventures :: ZAM</title></head><body>"
            f"<table>{rows}</table><a>Report Correction</a></body></html>")


def make_pages(folder):
    os.makedirs(f"{folder}/lists")
    os.makedirs(f"{folder}/pages")
    lists = {"Monk": [(10, "Ember Strike", 4), (11, "Pouncing Tiger", 49)], "Wizard": [(20, "Annihilate", 34),
             (21, "Shared Ward", 20)], "Necromancer": [(30, "Shared Ward", 20), (31, "Plague Rash", 29)]}
    for n, cname in fetch.LISTS.items():
        rows = "".join(f'<tr><td><a href="/spell.html?eqoaspell={i}">{nm}</a></td><td>{lv}</td></tr>'
                       for i, nm, lv in lists.get(cname, []))
        with open(f"{folder}/lists/scl{n}.html", "w") as fh:
            fh.write(f"<h1>Spells by Class: {cname}</h1><table>{rows}</table>")
    for pid, args in {
        10: ("Ember Strike", "Monk", 4, "Decrease Hitpoints by 32 ; Increase 20% based on STR", "0s", "30s", "30", "Single Target", "Instant"),
        11: ("Pouncing Tiger", "Monk", 49, "Decrease Hitpoints by 1224 ; Increase 20% based on STA", "0s", "90s", "368", "Single Target", "Instant"),
        20: ("Annihilate", "Wizard", 34, "Decrease Hitpoints by 1326 ; Increase 30% based on DEX", "2s", "180s", "510", "Single Target", "Instant"),
        21: ("Shared Ward", "Wizard", 20, "Increase AC by 50", "3s", "5s", "60", "Single Target", "30 minutes"),
        30: ("Shared Ward", "Necromancer", 20, "Increase AC by 50", "3s", "6s", "60", "Single Target", "30 minutes"),
        31: ("Plague Rash", "Necromancer", 29, "Decrease HP per tic by 116", "2s", "5s", "126", "Single Target", "18 seconds"),
    }.items():
        with open(f"{folder}/pages/{pid}.html", "w") as fh:
            fh.write(page(*args))


def make_db(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    rows = [  # (id, class, name, level, scope, cast, recast, power, line)
        (700001, MNK, "Ember Strike", 4, 0, 3, 2, 48, "Decrease Hitpoints by 32 ; Increase 20% based on STR"),
        (700002, MNK, "Pouncing Tiger", 49, 1, 2, 3, 184, "A furious martial attack that injures the target"),
        (700003, WIZ, "Annihilate", 34, 0, 3, 2, 408, "Damage by 1999, Modified 30% based on DEX"),
        (700004, WIZ, "Shared Ward", 20, 1, 2, 3, 50, "Target, Increase AC by 50"),
        (700005, NEC, "Plague Rash", 29, 0, 2, 5, 156, "Decrease HP per tic by 116"),
        (1500, MNK, "Ember Strike", 4, 0, 3, 2, 48, "Decrease Hitpoints by 32"),  # not an import's
    ]
    for sid, cls, name, lvl, scope, cast, recast, power, line in rows:
        rng, frame = sweep.SCOPE_RANGE[scope], sweep.SCOPE_FRAME[scope]
        db.execute("INSERT INTO spellpattern VALUES (?, ?, 0, 0, ?, ?, ?, ?, 7, ?, ?, -1, ?, ?)",
                   (sid, lvl, rng, cast, power, frame, scope, recast, name, line))
        db.execute("INSERT INTO spelleffect VALUES (?, ?, 0, 'eqoa-data (made)')", (sid, line))
        db.execute("INSERT INTO classspells VALUES (?, ?, 'eqoa-data')", (cls, sid))
        if sid >= 700001:
            db.execute("INSERT INTO eqoadata_spell_import_rows (tbl, key, note) VALUES ('spellpattern', ?, '')", (str(sid),))
    db.execute("INSERT INTO classspells VALUES (?, 700004, 'eqoa-data')", (NEC,))  # Shared Ward: two classes
    # an earlier fix chose Plague Rash's power
    before = {"spellid": 700005, "power": 100}
    db.execute("INSERT INTO spell_fill_log (part, tbl, rowkey, action, before, after, note) VALUES "
               "('spells', 'spellpattern', ?, 'update', ?, ?, '')",
               (json.dumps({"spellid": 700005}), json.dumps(before), json.dumps({"spellid": 700005, "power": 156})))
    db.commit()
    db.close()


def run(db, folder, *args):
    r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", db, "--pages", folder, "--no-backup", "--list", *args],
                       capture_output=True, text=True)
    if r.returncode != 0:
        raise AssertionError(r.stdout + r.stderr)
    return r.stdout


def dump(db):
    c = sqlite3.connect(db)
    try:
        return [l for l in c.iterdump() if "sqlite_sequence" not in l]
    finally:
        c.close()


def row(db, sql, *args):
    c = sqlite3.connect(db)
    try:
        return c.execute(sql, args).fetchone()
    finally:
        c.close()


class SweepSpellPages(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.pages = os.path.join(self.dir.name, "pages")
        make_pages(self.pages)
        self.db = os.path.join(self.dir.name, "t.db")
        make_db(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_report_writes_nothing(self):
        before = dump(self.db)
        out = run(self.db, self.pages)
        self.assertIn("5 imported records: 5 on a page", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_then_undo(self):
        before = dump(self.db)
        out = run(self.db, self.pages, "--apply")
        # a self spell its page casts at a target
        self.assertEqual(row(self.db, "SELECT scope, range, iconColor, casttime, recast, power FROM spellpattern WHERE spellid = 700001"),
                         (1, sweep.SCOPE_RANGE[1], sweep.SCOPE_FRAME[1], 0, 30, 30))
        # a line of prose: the page's effects, its word and no time (Instant)
        self.assertEqual(row(self.db, "SELECT line FROM spelleffect WHERE spellid = 700002"),
                         ("Target, Decrease Hitpoints by 1224 ; Increase 20% based on STA",))
        self.assertEqual(row(self.db, "SELECT casttime, recast, power, spelldesc FROM spellpattern WHERE spellid = 700002"),
                         (0, 90, 368, "Target, Decrease Hitpoints by 1224 ; Increase 20% based on STA"))
        # a tick with no time: its page's
        self.assertEqual(row(self.db, "SELECT line FROM spelleffect WHERE spellid = 700005"), ("Decrease HP per tic by 116, 18sec",))
        # a value an earlier fix chose is left; the rest of the record is the page's
        self.assertEqual(row(self.db, "SELECT power, scope FROM spellpattern WHERE spellid = 700005"), (156, 1))
        self.assertIn("power 156 (set by spell_fill_log), its page's 126", out)
        # two classes' pages disagree: left
        self.assertEqual(row(self.db, "SELECT recast, power FROM spellpattern WHERE spellid = 700004"), (3, 50))
        self.assertIn("its classes' pages disagree", out)
        # the figures differ: listed and kept, the record still the page's
        self.assertEqual(row(self.db, "SELECT line FROM spelleffect WHERE spellid = 700003"),
                         ("Damage by 1999, Modified 30% based on DEX",))
        self.assertIn("figures differ", out)
        self.assertEqual(row(self.db, "SELECT scope, recast FROM spellpattern WHERE spellid = 700003"), (1, 180))
        # no importer's record: untouched
        self.assertEqual(row(self.db, "SELECT scope, recast, power FROM spellpattern WHERE spellid = 1500"), (0, 2, 48))
        # a second --apply: nothing to do
        self.assertIn("0 rows to change", run(self.db, self.pages, "--apply"))
        out = run(self.db, self.pages, "--undo")
        self.assertIn("put back", out)
        self.assertEqual(dump(self.db), before)


class FetchSpellPagesReaders(unittest.TestCase):
    """tools/fetch_spell_pages.py's readers, on the layout of Allakhazam's
    pages, with no fetching."""

    def test_a_page_and_a_list(self):
        rec = fetch.read_page(page("Pouncing Tiger", "Monk", 49, "Decrease Hitpoints by 1224 ; Increase 20% based on STA | Decrease AC by 5",
                                   "0s", "90s", "368", "Single Target", "Instant"))
        self.assertEqual((rec["name"], rec["class"], rec["level"], rec["Scope"], rec["Recast"], rec["Power Cost"]),
                         ("Pouncing Tiger", "Monk", 49, "Single Target", "90s", "368"))
        self.assertEqual(rec["effects"], ["Decrease Hitpoints by 1224 ; Increase 20% based on STA", "Decrease AC by 5"])
        self.assertIsNone(fetch.read_page("<html><title>Nothing :: Spells :: EverQuest</title></html>"))
        lst = fetch.read_list('<a href="/spell.html?eqoaspell=851">Pouncing Tiger</a></td><td>49</td>'
                              '<a href="/spell.html?eqoaspell=850">Swooping Eagle</a></td><td>39</td>')
        self.assertEqual(lst, [[851, "Pouncing Tiger", 49], [850, "Swooping Eagle", 39]])

    def test_the_capture_read(self):
        caps = [["20060506173019", "http://eqoa.allakhazam.com:80/spell.html?eqoaspell=1654&amp;mid=1"],
                ["20120206075204", "http://eqoa.allakhazam.com/spell.html?eqoaspell=1654"],
                ["20060428223211", "http://eqoa.allakhazam.com:80/spell.html?eqoaspell=1654"]]
        self.assertEqual(fetch.best(caps)[0], "20060428223211")  # the plain page, nearest 2006
        self.assertEqual(fetch.best(caps[:1])[0], "20060506173019")  # a comment view when nothing else
        self.assertIsNone(fetch.best([]))

    def test_seconds_and_words(self):
        self.assertEqual([sweep.seconds(x) for x in ("90s", "30 minutes", "24 seconds", "Instant")], [90, 1800, 24, None])
        self.assertEqual([sweep.duration_words(x) for x in (24, 1800, 450, 90)], ["24sec", "30min", "7.5min", "1.5min"])


if __name__ == "__main__":
    unittest.main()
