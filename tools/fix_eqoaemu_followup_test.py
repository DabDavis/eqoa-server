#!/usr/bin/env python3
"""fix_eqoaemu_followup.py on a database the import left with its rules of
before (no item made for a spell, a long heal ticking): the made-items
fixture of import_eqoaemu_spells_test.py plus a long heal of ours, one spell
of ours the workbook spells otherwise (a new record of it, a mastery rank
moved to it, books holding one or both) and a worn item whose words say NO
RENT, every name and figure made up. What it plans; what --apply writes,
which leaves every row it touches as a fresh import with today's rules (and
the map's row on ours) does; a second plan empty; a GM's row left; --undo
exact."""
import contextlib
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_eqoaemu_followup as fu  # noqa: E402
import import_eqoaemu_spells as im  # noqa: E402
import import_eqoaemu_spells_test as imt  # noqa: E402

EFFECTS = imt.MadeItemsTest.EFFECTS + [
    imt.eff("0221", "Dmg/Heal", HP1=900, Duration=1800, Scope="Target", Target_Type="Friendly", Modifier="Wis",
            Percentage=10),
    imt.eff("0222", "Statistic Change", STR=50, HPMAX=900, Duration=1800, Scope="Target", Target_Type="Friendly"),
    imt.eff("0223", "Create Item", CreatedItemID=30106, QTY=1, Scope="Self", Target_Type="Self"),
]
SPELLS = imt.MadeItemsTest.SPELLS + [
    imt.spell("0220", "Spirit Mantle", CLASS="SHA", Level=51, EffectID1="0221", EffectID2="0222", Scope="Target",
              Target_Type="Friendly", Duration=1800, Learn_Method="CM", ScrollSource=""),     # ours: 305
    imt.made_spell("0224", "Glimmerene Mixture", "ALC", 60, "0223", learn="CM", source=""),   # ours as 303, spelled otherwise
]
ITEM_EFFECTS = imt.MadeItemsTest.ITEM_EFFECTS + [imt.eff("0E18", "Statistic Change", AGI=-20, Duration=24, Scope="Target",
                                          Target_Type="Enemy")]
ITEMS = imt.MadeItemsTest.ITEMS + [imt.info("Glimmerene Mix Potion", 30106, "ALC", 60, "Target", "Enemy", "0E18")]
# the import's map: the new spelling a new record; a fresh import's, it ours
OLD_MAP = dict(imt.MadeItemsTest.MAP, **{"0220": (305, "name"), "0224": (900548, "new")})
NEW_MAP = dict(OLD_MAP, **{"0224": (303, "spelled otherwise")})
TOUCHED = [303, 305, 900512, 900514, 900516, 900518, 900520, 900522, 900524, 900532]


def workbook():
    return im.Workbook({"Spells": SPELLS, "Effects": EFFECTS, "Created Item Info": ITEMS,
                        "Created Item Effects": ITEM_EFFECTS, "Pets": []})


@contextlib.contextmanager
def old_rules():
    """The import's rules as it was applied: a positive heal with a duration
    ticks, and no item is made."""
    keep = im.HEAL_TICKS_UP_TO, im.MadeItems.settle
    im.HEAL_TICKS_UP_TO, im.MadeItems.settle = 10 ** 9, lambda self, s, j, sid=None: None
    try:
        yield
    finally:
        im.HEAL_TICKS_UP_TO, im.MadeItems.settle = keep


def ours(db):
    """What the fixture adds of ours: the long heal's record, the spell the
    workbook spells otherwise with its item and class row, the Transmuter-like
    rank giving it, three alchemists' books, a staff whose words say NO RENT."""
    x = db.execute
    rec = "INSERT INTO spellpattern VALUES (?, ?, 0, 0, ?, 1, ?, -1809115846, 7, ?, 3, -1, ?, ?)"
    x(rec, (303, 60, imt.F45, 153, 0, "Glimmerine Mixture", "SELF Summon Glimmerine Potion"))
    x(rec, (305, 51, imt.F30, 330, 1, "Spirit Mantle", "Imbues a target with spirit."))
    x("INSERT INTO spelleffect VALUES (303, 'SELF Summon Glimmerine Potion', 0, 'eqoa-data')")
    x("INSERT INTO spelleffect VALUES (305, 'Target, Increase Maximum HP by 800, 30min', 0, 'eqoa-data')")
    x("INSERT INTO classspells VALUES (14, 303, 'eqoa-data'), (8, 305, 'eqoa-data')")
    x("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, "
      "weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, "
      "itemdesc, model, color) VALUES (60004, 0, 0, 512, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 3, 0, 50, 32767, 1023, 2, 0, 0, 0, "
      "'Glimmerine Potion', 'A potion.', 0, -1), (60005, 0, 0, 513, 0, 15, 0, 0, 1, 0, 4, 80, 0, 8, 1, 1600, 50, 32767, 1023, "
      "0, 0, 0, 0, 'Spark Staff', 'A summoned staff. NO RENT', 0, -1)")
    x("INSERT INTO cmcatalog VALUES (77, 16384, 'Glimmer Rank', 3, '303', '', '')")
    x("INSERT INTO characters VALUES (81, 14), (82, 14), (83, 14)")
    x("INSERT INTO spells (id, serverid, spellid) VALUES (91, 81, 303), (92, 83, 303)")


def masteries_import(db):
    """The masteries import as it left the fixture: the rank moved to the new
    record, logged; one alchemist given it beside ours, one given it alone."""
    x = db.execute
    x("CREATE TABLE cmcatalog_eqoaemu_log (id INTEGER, op TEXT, before TEXT, after TEXT, audit INTEGER, applied TEXT)")
    x("UPDATE cmcatalog SET add_spells = '900548' WHERE id = 77")
    x("""INSERT INTO cmcatalog_eqoaemu_log VALUES (77, 'update', '{"add_spells": "303"}', '{"add_spells": "900548"}', 0, '')""")
    x("INSERT INTO spells (id, serverid, spellid) VALUES (93, 81, 900548), (94, 82, 900548)")


class FollowupTest(unittest.TestCase):
    def setUp(self):
        d = tempfile.mkdtemp()
        self.path = os.path.join(d, "f.db")
        self.db = imt.made_db(self.path)
        ours(self.db)
        self.wb = workbook()
        with old_rules():
            im.apply(self.db, im.plan(self.db, self.wb, OLD_MAP, weres={}))
        masteries_import(self.db)
        # a fresh import's, for what the follow-up must leave alike
        self.fresh = imt.made_db(os.path.join(d, "fresh.db"))
        ours(self.fresh)
        im.apply(self.fresh, im.plan(self.fresh, self.wb, NEW_MAP, weres={}))

    def plan(self):
        return fu.plan(self.db, self.wb, OLD_MAP, weres={})

    def dump(self, db):
        out = {}
        for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
            out[t] = (db.execute(f"PRAGMA table_info({t})").fetchall(), sorted(map(repr, db.execute(f"SELECT * FROM {t}"))))
        return out

    def test_what_it_plans(self):
        p = self.plan()
        self.assertEqual(p["touched"], TOUCHED)
        self.assertEqual([m["ours"] for m in p["merges"]], [303])
        kinds = {}
        for c in p["changes"]:
            kinds.setdefault((c["tbl"], c["action"]), []).append(c["key"])
        self.assertEqual(len(kinds[("itempattern", "insert")]), 7, "the cure, splash, teleport, staff, dagger, pie, brew")
        self.assertIn({"patternid": 60005}, kinds[("itempattern", "update")], "the staff its words call NO RENT")
        self.assertIn({"patternid": 60004}, kinds[("itempattern", "update")], "our potion thrown, as the workbook's use")
        self.assertEqual(sorted(k["spellid"] for k in kinds[("spelleffect", "update")]), TOUCHED)
        self.assertEqual(kinds[("spellpattern", "delete")], [{"spellid": 900548}])
        self.assertEqual(kinds[("spells", "delete")], [{"id": 93}], "the book holding both keeps ours")
        self.assertEqual(kinds[("spells", "update")], [{"id": 94}], "the book holding the new alone takes ours")
        self.assertEqual(kinds[("cmcatalog", "update")], [{"id": 77}])
        heal = next(c for c in p["changes"] if c["tbl"] == "spelleffect" and c["key"] == {"spellid": 305})
        self.assertEqual(json.loads(heal["before"]["effects"])["effects"][0]["kind"], "hp_tick", "the import's tick")
        self.assertEqual(json.loads(heal["set"]["effects"])["effects"][0]["kind"], "hp", "a heal once")
        self.assertTrue(heal["set"]["line"].startswith("Target, Increase Hitpoints by 900 ; Increase 10% based on WIS"))

    def test_apply_leaves_what_a_fresh_import_does(self):
        done, stale = im.apply(self.db, self.plan(), fu.LOG, fu.META, fu.WHAT)
        self.assertEqual(stale, 0)
        for sid in TOUCHED:
            a = self.db.execute("SELECT line, effects, source FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
            b = self.fresh.execute("SELECT line, effects, source FROM spelleffect WHERE spellid = ?", (sid,)).fetchone()
            self.assertEqual((a[0], json.loads(a[1]), a[2]), (b[0], json.loads(b[1]), b[2]), sid)
            self.assertEqual(self.db.execute("SELECT * FROM spellpattern WHERE spellid = ?", (sid,)).fetchone(),
                             self.fresh.execute("SELECT * FROM spellpattern WHERE spellid = ?", (sid,)).fetchone(), sid)
        for tbl in ("itemeffect", "foodeffect"):
            self.assertEqual(sorted(self.db.execute(f"SELECT * FROM {tbl}")), sorted(self.fresh.execute(f"SELECT * FROM {tbl}")), tbl)
        made = "SELECT * FROM itempattern WHERE patternid >= 900000 AND NOT (itemicon = ? AND equipslot = -1) OR patternid = 60004"
        self.assertEqual(sorted(self.db.execute(made, (imt.SCROLL,))), sorted(self.fresh.execute(made, (imt.SCROLL,))))
        self.assertIsNone(self.db.execute("SELECT 1 FROM spellpattern WHERE spellid = 900548").fetchone())
        self.assertEqual(self.db.execute("SELECT add_spells FROM cmcatalog WHERE id = 77").fetchone(), ("303",))
        self.assertEqual(sorted(self.db.execute("SELECT id, serverid, spellid FROM spells WHERE spellid IN (303, 900548)")),
                         [(91, 81, 303), (92, 83, 303), (94, 82, 303)])
        self.assertEqual(self.db.execute("SELECT class, source FROM classspells WHERE spellid IN (303, 900548)").fetchall(),
                         [(14, "eqoa-data")], "ours keeps its class row, the new one's goes")
        self.assertEqual(self.db.execute("SELECT rent FROM itempattern WHERE patternid = 60005").fetchone(), (0,))
        self.assertEqual(self.plan()["changes"], [], "a second plan has nothing to do")
        with self.assertRaises(SystemExit):
            im.apply(self.db, self.plan(), fu.LOG, fu.META, fu.WHAT)

    def test_a_gms_row_is_left(self):
        self.db.execute("UPDATE spelleffect SET line = 'Self, a GM''s words' WHERE spellid = 900514")
        p = self.plan()
        self.assertNotIn({"spellid": 900514}, [c["key"] for c in p["changes"] if c["tbl"] == "spelleffect"])
        self.assertEqual(p["lists"]["a spell's effects row a GM changed since the import (left)"], ["900514"])

    def test_undo_exact(self):
        before = self.dump(self.db)
        im.apply(self.db, self.plan(), fu.LOG, fu.META, fu.WHAT)
        back, left, _ = im.undo(self.db, False, fu.LOG, fu.META, fu.WHAT)
        self.assertEqual(left, 0)
        self.assertEqual(self.dump(self.db), before)

    def test_undo_refused_while_a_made_item_is_held(self):
        im.apply(self.db, self.plan(), fu.LOG, fu.META, fu.WHAT)
        self.db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (81, 940101)")
        with self.assertRaises(SystemExit):
            im.undo(self.db, False, fu.LOG, fu.META, fu.WHAT)
        back, left, _ = im.undo(self.db, True, fu.LOG, fu.META, fu.WHAT)
        self.assertEqual(left, 0)
        self.assertEqual(self.db.execute("SELECT count(*) FROM charinventory WHERE patternid = 940101").fetchone(), (0,))

    def test_a_plan_applied_later_takes_the_books_since(self):
        """A plan made on a copy, applied where a fourth alchemist has since
        bought the rank: its merges are read there again, so the new book row
        names ours too; the control, the plan as made, misses it."""
        p = json.loads(json.dumps(self.plan()))
        self.db.execute("INSERT INTO characters VALUES (84, 14)")
        self.db.execute("INSERT INTO spells (id, serverid, spellid) VALUES (95, 84, 900548)")
        self.assertNotIn({"id": 95}, [c["key"] for c in p["changes"] if c["tbl"] == "spells"], "the control")
        q = fu.refresh_merges(self.db, p)
        self.assertIn({"id": 95}, [c["key"] for c in q["changes"] if c["tbl"] == "spells"])
        self.assertEqual(len(q["changes"]), len(p["changes"]) + 1)
        im.apply(self.db, q, fu.LOG, fu.META, fu.WHAT)
        self.assertEqual(self.db.execute("SELECT spellid FROM spells WHERE id = 95").fetchone(), (303,))

    def test_edits(self):
        self.assertEqual(fu.edits("glimmerine mixture", "glimmerene mixture"), 1)
        self.assertEqual(fu.edits("transmute", "transmutate"), 2)
        self.assertGreater(fu.edits("create shiny", "creative shine"), 2)


if __name__ == "__main__":
    unittest.main()
