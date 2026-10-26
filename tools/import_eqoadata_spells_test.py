#!/usr/bin/env python3
"""tools/import_eqoadata_spells.py against made-up lists and a made-up
database: a cleric line with one rank on record, a rank worded differently,
a mastery, a name two classes list at one level, a scroll vendor in two
bands. No game data.

    python3 tools/import_eqoadata_spells_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_eqoadata_spells as ie  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL,
    unk2 INTEGER NOT NULL, unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL,
    power INTEGER NOT NULL, iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL,
    recast INTEGER NOT NULL, eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL,
    PRIMARY KEY (class, spellid));
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER, itemicon INTEGER,
    equipslot INTEGER, trade INTEGER, rent INTEGER, levelreq INTEGER, maxstack INTEGER, classuse INTEGER,
    raceuse INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, color INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER, stackleft INTEGER,
    remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER);
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER);
"""
CL, DRD, WAR = 9, 7, 0


def ability(name, line, how="Merchant", icon="Heal"):
    return {"name": name, "description": line, "source": how, "icon": icon}


def lists(dirpath):
    per = {f: [] for f in ie.CLASS_FILES}
    per["cleric"] = [
        {"level": 4, "abilities": [ability("Courage", "Target, Increase Maximum HP by 40, Increase AC by 40",
                                           icon="Defense_Active")]},
        {"level": 12, "abilities": [ability("Center", "Target, Buff MaxHP by 120 and AC by 120, 30min",
                                            icon="Defense_Active"),
                                    ability("Group Center", "Group, Buff MaxHP by 120 and AC by 120, 30min",
                                            how="CM", icon="Defense_Active")]},
        {"level": 20, "abilities": [ability("Healing", "Target, Increase HP per tic by 125 ; "
                                            "Increase 25% based on CHA, 24sec")]},
        {"level": 29, "abilities": [ability("Greater Healing", "Target, Increase HP per tic by 250 ; "
                                            "Increase 25% based on CHA, 24sec"),
                                    ability("Center", "Target, a second Center", how="Quest")]},
        {"level": 30, "abilities": [ability("Resuscitate", "Ressurect Player: restore 30% experience debt",
                                            how="Quest", icon="Wand")]},
        {"level": 60, "abilities": [ability("Celestial Healing", "Target, buff HP per tic by 650 Modified 25% "
                                            "based on CHA", icon=None)]},
    ]
    per["druid"] = [{"level": 29, "abilities": [ability("Greater Healing", "Target, Increase HP per tic by 250 ; "
                                                         "Increase 25% based on CHA, 24sec")]}]
    d = os.path.join(dirpath, "Information", "databases", "spells")
    os.makedirs(d)
    for f, lv in per.items():
        with open(os.path.join(d, f + ".json"), "w") as fh:
            json.dump({"title": f, "list": lv}, fh)
    return dirpath


def database(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(SCHEMA)
    # the record of Courage, and of Healing (level 20), as a dump would hold them
    db.execute("INSERT INTO spellpattern VALUES (237, 4, 0, 0, 16880, 5, 40, 486870879, 1920746744, 1, 3, -1, "
               "'Courage', 'Raises the target''s armor.')")
    db.execute("INSERT INTO spellpattern VALUES (464, 20, 0, 0, 16880, 3, 200, 486870879, -2105187409, 1, 1, -1, "
               "'Healing', 'Heals the target.')")
    db.execute("INSERT INTO classspells VALUES (9, 464, 'dump')")
    # a cleric vendor in two bands, and a scroll of Greater Healing already sold
    scroll = (0, ie.SCROLL_ICON, -1, 0, 1, 1, 9 << 0)
    for pid, cost, lv, mask, name in ((8001, 11, 1, 1 << CL, "Minor Thing"), (8002, 100, 10, 1 << CL, "Small Thing"),
                                      (8003, 300, 20, 1 << CL, "Mid Thing"), (8004, 600, 24, 1 << CL, "Big Thing"),
                                      (8005, 800, 30, 1 << CL, "Bigger Thing"), (8006, 1200, 34, 1 << CL, "Huge Thing"),
                                      (8007, 737, 29, (1 << CL) | (1 << DRD), "Greater Healing")):
        db.execute("INSERT INTO itempattern VALUES (?, ?, ?, -1, 0, 1, ?, 1, ?, 1023, ?, 'x', 0, 0)",
                   (pid, cost, ie.SCROLL_ICON, lv, mask, name))
    for n, pid in enumerate((8001, 8002, 8003)):
        db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                   "listnumber) VALUES (567, 1, 0, 0, ?, -1, 1, ?)", (pid, n))
    for n, pid in enumerate((8004, 8005, 8006, 8007)):
        db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                   "listnumber) VALUES (575, 1, 0, 0, ?, -1, 1, ?)", (pid, n))
    # the counter a character's spell row moves, as a live database has it
    db.execute("INSERT INTO spells (id, serverid, spellid) VALUES (1, 0, 0)")
    db.execute("DELETE FROM spells")
    return db


class ImportTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = database(os.path.join(self.tmp.name, "eqoa.db"))
        self.lists = ie.load_lists(lists(os.path.join(self.tmp.name, "EQOA-Data")))

    def tearDown(self):
        self.db.close()
        self.tmp.cleanup()

    def made(self, p, name):
        return next(m for m in p["spells"] if m["row"]["spellname"] == name)

    def test_plan(self):
        p = ie.plan(self.db, self.lists)
        names = sorted(m["row"]["spellname"] for m in p["spells"])
        # Courage is on record; the mastery and the second "Center" are not made
        self.assertEqual(names, ["Celestial Healing", "Center", "Greater Healing", "Resuscitate"])
        self.assertEqual(p["skipped"]["got by CM"], 1)
        self.assertEqual(p["skipped"]["the same name again in one class"], 1)
        # a rank worded differently takes Courage's numbers, the power by level
        c = self.made(p, "Center")["row"]
        self.assertEqual((c["abilitylvl"], c["scope"], c["casttime"], c["recast"], c["power"], c["icon"],
                          c["iconColor"], c["range"]), (12, 1, 5, 3, 120, 1920746744, 486870879, 16880))
        self.assertTrue(c["spellid"] > ie.ID_BASE)
        # Healing's line: Greater Healing scales from it; one record for the cleric and the druid
        g = self.made(p, "Greater Healing")
        self.assertEqual((g["row"]["casttime"], g["row"]["recast"], g["row"]["power"]), (3, 1, 290))
        self.assertEqual(sorted(set(g["classes"])), [DRD, CL])
        # no icon name: its kind's (a heal); a resurrection is cast at a target
        self.assertEqual(self.made(p, "Celestial Healing")["row"]["icon"], ie.ICONS["Heal"])
        self.assertEqual(self.made(p, "Resuscitate")["row"]["scope"], 1)
        # Courage's record was not the cleric's yet: linked, not made again
        self.assertIn((CL, 237), [(c, s) for c, s, _ in p["links"]])
        # scrolls: Greater Healing's exists; Center's and Celestial Healing's are made, in the bands that hold them
        self.assertEqual(sorted(r["itemname"] for r in p["scrolls"]), ["Celestial Healing", "Center"])
        center = next(r for r in p["scrolls"] if r["itemname"] == "Center")
        self.assertEqual((center["levelreq"], center["classuse"], center["itemcost"]), (12, 1 << CL, 144))
        stock = {(name, npc) for npc, _, name in p["stock"]}
        self.assertEqual(stock, {("Center", 567), ("Celestial Healing", 575)})

    def test_apply_and_undo(self):
        before = "\n".join(self.db.iterdump())
        p = ie.plan(self.db, self.lists)
        ie.apply(self.db, p)
        q = self.db.execute
        self.assertEqual(q("SELECT count(*) FROM spellpattern WHERE spellid > ?", (ie.ID_BASE,)).fetchone()[0], 4)
        self.assertEqual(q("SELECT line FROM spelleffect WHERE spellid = (SELECT spellid FROM spellpattern "
                           "WHERE spellname = 'Center')").fetchone()[0], "Target, Buff MaxHP by 120 and AC by 120, 30min")
        self.assertEqual(q("SELECT count(*) FROM classspells WHERE class = 9").fetchone()[0], 6)
        self.assertEqual(q("SELECT listnumber FROM npcitem WHERE npcid = 567 ORDER BY itemid DESC").fetchone()[0], 3)
        with self.assertRaises(SystemExit):
            ie.apply(self.db, ie.plan(self.db, self.lists))
        # a character holding a made spell stops the undo, unless forced
        sid = q("SELECT spellid FROM spellpattern WHERE spellname = 'Center'").fetchone()[0]
        q("INSERT INTO spells (id, serverid, spellid) VALUES (1, 1, ?)", (sid,))
        with self.assertRaises(SystemExit):
            ie.undo(self.db, False)
        q("DELETE FROM spells")
        ie.undo(self.db, False)
        self.assertEqual("\n".join(self.db.iterdump()), before)


if __name__ == "__main__":
    unittest.main()
