#!/usr/bin/env python3
"""tools/import_eqoadata_mastery_spells.py against made-up lists and a
made-up database (the leveled import's test schema): a mastery, an epic, a
master class's ability, a bare MCA, a mastery the server already holds a
record of, a name a class has both by a level and as a mastery, and a
leveled spell it must leave alone. No game data.

    python3 tools/import_eqoadata_mastery_spells_test.py
"""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_eqoadata_mastery_spells as im  # noqa: E402
import import_eqoadata_spells as ie  # noqa: E402
from import_eqoadata_spells_test import SCHEMA  # noqa: E402

CL, NEC, WAR = 9, 11, 0


def ability(name, line, how, icon="Heal"):
    return {"name": name, "description": line, "source": how, "icon": icon}


def lists(dirpath):
    per = {f: [] for f in ie.CLASS_FILES}
    per["cleric"] = [
        {"level": 10, "abilities": [ability("Minor Ward", "Target, Increase AC by 10, 30min", "Merchant")]},
        {"level": 29, "abilities": [ability("Group Bravery", "Group, Increase Maximum HP by 290, 30min", "CM",
                                            icon="Defense_Active"),
                                    ability("Tide of Life", "Group, Increase Hitpoints by 250", "CM")]},
        {"level": 60, "abilities": [ability("Holy Sanctuary", "Level 60 Epic Ward path. Power cost: 500, Cast: 3, "
                                            "Recast: 4. 60 epic: 85+10%wis to all resists (target)", "60 Epic"),
                                    ability("Faithful Healing", "Target, Increase HP by 900", "Faithful MCA")]},
    ]
    per["necromancer"] = [
        {"level": 60, "abilities": [ability("Lich", "Self, Increase Power per tic by 40", "Lich MCA",
                                            icon="Skeletal_Arm"),
                                    ability("Revert Lich", "Self, Removes the Lich form", "MCA", icon=None)]},
        # a name the class has by a level and as a mastery: by the level
        {"level": 30, "abilities": [ability("Dark Pact", "Target, Increase Hitpoints by 25", "Merchant")]},
        {"level": 31, "abilities": [ability("Dark Pact", "Target, Increase Hitpoints by 25", "CM")]},
    ]
    per["warrior"] = [{"level": 60, "abilities": [ability("Guard", "Self, Increase AC by 100", "Gaurdian MCA",
                                                          icon="Defense_Active")]}]
    d = os.path.join(dirpath, "Information", "databases", "spells")
    os.makedirs(d)
    for f, lv in per.items():
        with open(os.path.join(d, f + ".json"), "w") as fh:
            json.dump({"title": f, "list": lv}, fh)
    return dirpath


def database(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(SCHEMA)
    # the leveled import's record of Minor Ward, and a record of Tide of Life a
    # dump gave the server (no class row of it), and Dark Pact on a leveled row
    db.execute("INSERT INTO spellpattern VALUES (700001, 10, 0, 0, 16880, 3, 30, 486870879, 1920746744, 1, 3, -1, "
               "'Minor Ward', 'Target, Increase AC by 10, 30min')")
    db.execute("INSERT INTO classspells VALUES (9, 700001, 'eqoa-data')")
    db.execute("INSERT INTO spellpattern VALUES (2223, 29, 0, 0, 16880, 4, 300, 593297002, -2105187409, 2, 6, -1, "
               "'Tide of Life', 'Heals your group.')")
    db.execute("INSERT INTO spellpattern VALUES (900, 30, 0, 0, 16880, 2, 40, 486870879, -2105187409, 1, 2, -1, "
               "'Dark Pact', 'A pact.')")
    db.execute("INSERT INTO classspells VALUES (11, 900, 'eqoa-data')")
    db.execute("INSERT INTO spells (id, serverid, spellid) VALUES (1, 0, 0)")
    db.execute("DELETE FROM spells")
    return db


class MasteryImportTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.tmp.name, "eqoa.db")
        self.db = database(self.path)
        self.lists = ie.load_lists(lists(os.path.join(self.tmp.name, "EQOA-Data")))

    def tearDown(self):
        self.db.close()
        self.tmp.cleanup()

    def test_kind_source(self):
        for how, want in (("CM", "eqoa-data cm"), ("60 Epic", "eqoa-data epic"), ("MCA", "eqoa-data mca"),
                          ("Lich MCA", "eqoa-data master Lich"), ("Gaurdian MCA", "eqoa-data master Guardian"),
                          ("Storm caller MCA", "eqoa-data master Storm Caller"),
                          ("Death Knight MCA", "eqoa-data master Death Knight"),
                          ("Buglar MCA", "eqoa-data master Burglar"),
                          ("Merchant", None), ("Quest", None), ("Mob Drop", None), ("", None)):
            self.assertEqual(im.kind_source(how), want, how)

    def test_plan(self):
        p = im.plan(self.db, self.lists)
        made = {m["row"]["spellname"]: m for m in p["spells"]}
        # the masteries, the epic and the master classes' are made; Minor Ward (leveled, held),
        # Tide of Life (held) and Dark Pact (held, and the class's by a level) are not
        self.assertEqual(sorted(made), ["Faithful Healing", "Group Bravery", "Guard", "Holy Sanctuary", "Lich",
                                        "Revert Lich"])
        self.assertTrue(all(m["row"]["spellid"] > 700001 for m in made.values()))
        links = {(c, s): src for c, s, src, _ in p["links"]}
        sid = {n: m["row"]["spellid"] for n, m in made.items()}
        self.assertEqual(links[(CL, sid["Group Bravery"])], "eqoa-data cm")
        self.assertEqual(links[(CL, sid["Holy Sanctuary"])], "eqoa-data epic")
        self.assertEqual(links[(CL, sid["Faithful Healing"])], "eqoa-data master Faithful")
        self.assertEqual(links[(NEC, sid["Lich"])], "eqoa-data master Lich")
        self.assertEqual(links[(NEC, sid["Revert Lich"])], "eqoa-data mca")
        self.assertEqual(links[(WAR, sid["Guard"])], "eqoa-data master Guardian")
        # a record held of a mastery goes to its class by the mastery's row
        self.assertEqual(links[(CL, 2223)], "eqoa-data cm")
        # the control: nothing for a spell the class has by a level
        self.assertNotIn((NEC, 900), links)
        self.assertNotIn((CL, 700001), links)
        # the epic's own numbers, said in its line
        r = made["Holy Sanctuary"]["row"]
        self.assertEqual((r["casttime"], r["recast"], r["power"]), (3, 4, 500))
        # a line for each made record, and no scroll
        self.assertEqual(sorted(e[0] for e in p["effects"]), sorted(sid.values()))
        self.assertEqual(p["retags"], [])

    def test_retag(self):
        # the class-spells importer's plain row of a mastery (Tide of Life for the cleric)
        self.db.execute("INSERT INTO classspells VALUES (9, 2223, 'eqoa-data')")
        p = im.plan(self.db, self.lists)
        self.assertEqual(p["retags"], [])  # off by default
        self.assertEqual(p["retag_candidates"], 1)
        p = im.plan(self.db, self.lists, retag=True)
        self.assertEqual([(c, s, o, n) for c, s, o, n, _ in p["retags"]], [(CL, 2223, "eqoa-data", "eqoa-data cm")])

    def test_apply_and_undo_exactly(self):
        self.db.execute("INSERT INTO classspells VALUES (9, 2223, 'eqoa-data')")
        before = "\n".join(self.db.iterdump())
        p = im.plan(self.db, self.lists, retag=True)
        im.apply(self.db, p)
        got = dict(self.db.execute("SELECT spellid, source FROM classspells WHERE class = 9").fetchall())
        self.assertEqual(got[2223], "eqoa-data cm")
        self.assertEqual(self.db.execute("SELECT count(*) FROM spellpattern WHERE spellid > 700001").fetchone()[0], 6)
        with self.assertRaises(SystemExit):  # a second apply is refused
            im.apply(self.db, im.plan(self.db, self.lists))
        im.undo(self.db, False)
        self.assertEqual("\n".join(self.db.iterdump()), before)

    def test_undo_refused_while_held(self):
        p = im.plan(self.db, self.lists)
        im.apply(self.db, p)
        lich = next(m["row"]["spellid"] for m in p["spells"] if m["row"]["spellname"] == "Lich")
        self.db.execute("INSERT INTO spells (serverid, spellid) VALUES (5, ?)", (lich,))
        with self.assertRaises(SystemExit):
            im.undo(self.db, False)
        im.undo(self.db, True)
        self.assertEqual(self.db.execute("SELECT count(*) FROM spells").fetchone()[0], 0)


if __name__ == "__main__":
    unittest.main()
