#!/usr/bin/env python3
"""fix_eqoaemu_followup2.py on a database the import left with its rules of
before (a merged spell's item none of ours by the line, the workbook's item
made for it; no body, no target's own share): BodiesAndSharesTest's workbook
and database (import_eqoaemu_spells_test.py), a mastery rank moved from our
spell to the new record, books holding one or both, a character and an
auction holding the made item, every name and figure made up. What it plans;
what --apply writes, which leaves every row it touches as a fresh import with
today's rules and the map's row on ours does; a second plan empty; the map's
row decides; an item of ours is never folded; a plan applied later takes the
copies since; --undo exact."""
import contextlib
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_eqoaemu_followup2 as f2  # noqa: E402
import import_eqoaemu_spells as im  # noqa: E402
import import_eqoaemu_spells_test as imt  # noqa: E402

# BodiesAndSharesTest's workbook (not bound here as a class: unittest would
# run its tests again)
BS = {k: getattr(imt.BodiesAndSharesTest, k) for k in ("SPELLS", "EFFECTS", "ITEMS", "ITEM_EFFECTS", "MAP")}
# the import's map: the workbook's spelling of our pill spell a new record;
# the operator's now, it ours
OLD_MAP = dict(BS["MAP"], **{"0230": (900560, "new")})
NEW_MAP = BS["MAP"]
MADE = im.ITEM_BASE + 30110   # the Shiny Potion the old import made for the workbook's name
# the CMs sheet's two ranks DESC_SWAPS names, their own descriptions (made up)
RANKS = [dict(name="Creative Transmutation", desc="Guess A?"), dict(name="Transmutative Mixture", desc="Guess B?")]


@contextlib.contextmanager
def old_rules():
    """The import's rules as it was applied: no use says a body or a target's
    own share, a record the map spells otherwise is read as any other, and a
    use of ours keeps its own time."""
    keep = im.potion_extras, im.settle_item_names, im.MadeItems.give_use, im.MadeItems.ours_time
    plain = im.settle_item_names
    give = im.MadeItems.give_use
    im.potion_extras = lambda objs, thrown, area: {}
    im.settle_item_names = lambda j, old, a, b, line_first=False, folds=None: plain(j, old, a, b)
    im.MadeItems.give_use = lambda self, pid, rows, s, replace=False: give(self, pid, rows, s)
    im.MadeItems.ours_time = lambda self, pid, r, rows, old: None
    try:
        yield
    finally:
        im.potion_extras, im.settle_item_names, im.MadeItems.give_use, im.MadeItems.ours_time = keep


def ours(db):
    """BodiesAndSharesTest's own rows on db (its setUp's), and the tables a
    fold reads."""
    x = db.execute
    rec = "INSERT INTO spellpattern VALUES (?, 60, 0, 0, ?, 2, 100, -1809115846, 7, 0, 3, -1, ?, ?)"
    x(rec, (310, imt.F45, "Shiny Transformative", "Ours."))
    x(rec, (311, imt.F45, "Sour Mixture", "Ours."))
    x("INSERT INTO spelleffect VALUES (310, 'Self, Summon Glimmer Pill', 0, 'gm')")
    x("INSERT INTO spelleffect VALUES (311, 'Self, Summon Sour Potion', 0, 'eqoa-data')")
    x("INSERT INTO classspells VALUES (14, 310, 'eqoa-data'), (14, 311, 'eqoa-data')")
    item = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, "
            "attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, "
            "unk6, craft, itemname, itemdesc, model, color) VALUES (?, 0, 0, 512, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, "
            "50, 32767, 1023, 2, 1, 0, 0, ?, ?, 0, -1)")
    x(item, (60010, "Glimmer Pill", "A pill of ours."))
    x(item, (60011, "Sour Potion", "A potion of ours."))
    x("INSERT INTO itemeffect (patternid, line, name, icon, source) VALUES (60010, "
      "'Self, Increase Maximum HP by 50, Increase Hitpoints by 50, 10min', 'Illusion: Troll', 77, 'gm')")
    x("INSERT INTO cmcatalog VALUES (77, 16384, 'Shiny Rank', 2, '310', '', '')")
    imt.invisible_of_ours(x)
    # the two ranks whose descriptions the workbook swapped, as the masteries import wrote them
    x("ALTER TABLE cmcatalog ADD COLUMN description TEXT NOT NULL DEFAULT ''")
    x("INSERT INTO cmcatalog (id, classes, name, rank, description) VALUES "
      "(78, 16384, 'Creative Transmutation', 2, 'Guess A?'), (79, 16384, 'Transmutative Mixture', 3, 'Guess B?')")
    x("INSERT INTO characters VALUES (81, 14), (82, 14), (83, 14)")
    x("INSERT INTO spells (id, serverid, spellid) VALUES (91, 81, 310), (92, 83, 310)")
    x("CREATE TABLE auction_lots (lot INTEGER PRIMARY KEY AUTOINCREMENT, seller INTEGER, patternid INTEGER, stack INTEGER)")


def masteries_import(db):
    """The masteries import as it left it: the rank moved to the new record,
    logged; one alchemist given it beside ours, one alone; and the made item
    held, at the auction too."""
    x = db.execute
    x("CREATE TABLE cmcatalog_eqoaemu_log (id INTEGER, op TEXT, before TEXT, after TEXT, audit INTEGER, applied TEXT)")
    x("UPDATE cmcatalog SET add_spells = '900560' WHERE id = 77")
    x("""INSERT INTO cmcatalog_eqoaemu_log VALUES (77, 'update', '{"add_spells": "310"}', '{"add_spells": "900560"}', 0, '')""")
    x("INSERT INTO spells (id, serverid, spellid) VALUES (93, 81, 900560), (94, 82, 900560)")
    x("INSERT INTO charinventory (itemid, serverid, patternid) VALUES (501, 82, ?), (502, 83, 60011)", (MADE,))
    x("INSERT INTO auction_lots (lot, seller, patternid, stack) VALUES (7, 81, ?, 1)", (MADE,))


class Followup2Test(unittest.TestCase):
    def setUp(self):
        d = tempfile.mkdtemp()
        self.wb = im.Workbook({"Spells": BS["SPELLS"], "Effects": BS["EFFECTS"], "Created Item Info": BS["ITEMS"],
                               "Created Item Effects": BS["ITEM_EFFECTS"], "Pets": []})
        self.db = imt.made_db(os.path.join(d, "f.db"))
        ours(self.db)
        with old_rules():
            im.apply(self.db, im.plan(self.db, self.wb, OLD_MAP, weres={}))
        masteries_import(self.db)
        # a fresh import's, for what the follow-up must leave alike
        self.fresh = imt.made_db(os.path.join(d, "fresh.db"))
        ours(self.fresh)
        # a fresh masteries import with DESC_SWAPS writes the descriptions so
        self.fresh.execute("UPDATE cmcatalog SET description = CASE id WHEN 78 THEN 'Guess B?' ELSE 'Guess A?' END "
                           "WHERE id IN (78, 79)")
        im.apply(self.fresh, im.plan(self.fresh, self.wb, NEW_MAP, weres={}))

    def plan(self, smap=NEW_MAP):
        return f2.plan(self.db, self.wb, smap, weres={}, ranks=RANKS)

    def kinds(self, p):
        out = {}
        for c in p["changes"]:
            out.setdefault((c["tbl"], c["action"]), []).append(c["key"])
        return out

    def dump(self, db):
        out = {}
        for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
            out[t] = (db.execute(f"PRAGMA table_info({t})").fetchall(), sorted(map(repr, db.execute(f"SELECT * FROM {t}"))))
        return out

    def test_what_it_plans(self):
        self.assertEqual(self.db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (MADE,)).fetchone(),
                         ("Shiny Potion",), "the old import made the workbook's item")
        p = self.plan()
        self.assertEqual([m["ours"] for m in p["merges"]], [310])
        self.assertEqual(p["folds"], [dict(name="Shiny Mix Potion", found=MADE, line=60010, spellid=310)])
        k = self.kinds(p)
        self.assertEqual(k[("spellpattern", "delete")], [{"spellid": 900560}])
        self.assertEqual(k[("spells", "delete")], [{"id": 93}], "the book holding both keeps ours")
        self.assertEqual(k[("spells", "update")], [{"id": 94}], "the book holding the new alone takes ours")
        self.assertEqual(k[("cmcatalog", "update")], [{"id": 77}, {"id": 78}, {"id": 79}],
                         "the merged rank, then the two descriptions")
        self.assertEqual(k[("itempattern", "update")], [{"patternid": 60010}], "the pill's level, the workbook's")
        self.assertEqual(k[("spellpattern", "update")], [{"spellid": 310}], "ours takes the workbook's record")
        self.assertEqual(sorted(x["spellid"] for x in k[("spelleffect", "update")]), [310])
        self.assertEqual(sorted(x["patternid"] for x in k[("itemeffect", "update")]),
                         [60010, 60011, 60012, im.ITEM_BASE + 30102],
                         "the pill's use, the sour body, the hazy invisibility's time, the cocktail's share")
        desc = {c["key"]["id"]: (c["before"]["description"], c["set"]["description"]) for c in p["changes"]
                if c["tbl"] == "cmcatalog" and "description" in c["set"]}
        self.assertEqual(desc, {78: ("Guess A?", "Guess B?"), 79: ("Guess B?", "Guess A?")})
        self.assertEqual(k[("charinventory", "update")], [{"itemid": 501}], "the copy held, onto the pill")
        self.assertEqual(k[("auction_lots", "update")], [{"lot": 7}])
        self.assertEqual(sorted((t, x["patternid"]) for (t, a), xs in k.items() if a == "delete" and t.startswith("item")
                                for x in xs), [("itemeffect", MADE), ("itempattern", MADE)])
        pill = next(c for c in p["changes"] if c["tbl"] == "itemeffect" and c["key"] == {"patternid": 60010})
        self.assertEqual(pill["set"]["line"], "Self, Increase Hitpoints by 300, Increase STR by 30, Increase Maximum HP by "
                                              "300, 30min")
        self.assertEqual(pill["set"]["body"], 0x44332211)
        fx = next(c for c in p["changes"] if c["tbl"] == "spelleffect" and c["key"] == {"spellid": 310})
        it = [o["item"] for o in json.loads(fx["set"]["effects"])["effects"] if o["kind"] == "create_item"][0]
        self.assertEqual((it["name"], it["patternid"]), ("Glimmer Pill", 60010), "the line's item, not the made one")

    def test_apply_leaves_what_a_fresh_import_does(self):
        done, stale = im.apply(self.db, self.plan(), f2.LOG, f2.META, f2.WHAT)
        self.assertEqual(stale, 0)
        for tbl, where in (("spellpattern", "spellid IN (310, 311)"), ("spelleffect", "spellid IN (310, 311)"),
                           ("itemeffect", "1"), ("classspells", "spellid IN (310, 311, 900560)"),
                           ("itempattern", "patternid IN (60010, 60011, 60012, 60013)"),
                           ("cmcatalog", "id IN (78, 79)")):
            a = sorted(self.db.execute(f"SELECT * FROM {tbl} WHERE {where}"))
            b = sorted(self.fresh.execute(f"SELECT * FROM {tbl} WHERE {where}"))
            if tbl == "spelleffect":
                a = [(r[0], r[1], r[2], json.loads(r[4])) for r in a]
                b = [(r[0], r[1], r[2], json.loads(r[4])) for r in b]
            self.assertEqual(a, b, tbl)
        self.assertIsNone(self.db.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (MADE,)).fetchone())
        self.assertIsNone(self.fresh.execute("SELECT 1 FROM itempattern WHERE patternid = ?", (MADE,)).fetchone(),
                          "a fresh import makes no Shiny Potion")
        self.assertEqual(self.db.execute("SELECT patternid FROM charinventory WHERE itemid = 501").fetchone(), (60010,))
        self.assertEqual(self.db.execute("SELECT patternid FROM auction_lots WHERE lot = 7").fetchone(), (60010,))
        self.assertEqual(self.db.execute("SELECT add_spells FROM cmcatalog WHERE id = 77").fetchone(), ("310",))
        self.assertEqual(sorted(self.db.execute("SELECT id, serverid, spellid FROM spells WHERE spellid IN (310, 900560)")),
                         [(91, 81, 310), (92, 83, 310), (94, 82, 310)])
        self.assertEqual(self.plan()["changes"], [], "a second plan has nothing to do")
        with self.assertRaises(SystemExit):
            im.apply(self.db, self.plan(), f2.LOG, f2.META, f2.WHAT)

    def test_a_gms_description_is_left(self):
        self.db.execute("UPDATE cmcatalog SET description = 'A GM''s words' WHERE id = 78")
        p = self.plan()
        self.assertEqual([c["key"] for c in p["changes"] if c["tbl"] == "cmcatalog" and "description" in c["set"]],
                         [{"id": 79}])
        self.assertEqual(p["lists"]["a rank's description a GM changed since the import (left)"],
                         ["78 Creative Transmutation"])

    def test_the_maps_row_decides(self):
        p = self.plan(OLD_MAP)
        self.assertEqual((p["merges"], p["folds"]), ([], []))
        self.assertEqual(p["lists"]["a rank the masteries import moved to a new record the map does not put on ours "
                                    "(left: two spells)"], ["rank 77: 900560 'Shiny Transmutation' -> 310 'Shiny Transformative'"])
        self.assertNotIn(60010, [c["key"].get("patternid") for c in p["changes"] if c["tbl"] == "itemeffect"],
                         "the pill keeps its use: no merge, no workbook's")

    def test_an_item_of_ours_is_never_folded(self):
        self.db.execute(f"DELETE FROM {im.LOG} WHERE tbl = 'itempattern' AND key = ?", (im.dumps({"patternid": MADE}),))
        p = self.plan()
        self.assertEqual(p["folds"], [])
        self.assertNotIn(("itempattern", "delete"), self.kinds(p))
        self.assertEqual(len(p["lists"]["an item the workbook's name finds that a merged spell's line names otherwise, "
                                        "one of ours (left)"]), 1)

    def test_a_plan_applied_later_takes_the_copies_since(self):
        """A plan made on a copy, applied where another character has since
        made the workbook's item: its fold read there again takes that copy
        too; the control, the plan as made, misses it."""
        p = json.loads(json.dumps(self.plan()))
        self.db.execute("INSERT INTO charinventory (itemid, serverid, patternid) VALUES (503, 81, ?)", (MADE,))
        self.assertNotIn({"itemid": 503}, [c["key"] for c in p["changes"] if c["tbl"] == "charinventory"], "the control")
        q = f2.refresh(self.db, p)
        self.assertIn({"itemid": 503}, [c["key"] for c in q["changes"] if c["tbl"] == "charinventory"])
        self.assertEqual(len(q["changes"]), len(p["changes"]) + 1)
        done, stale = im.apply(self.db, q, f2.LOG, f2.META, f2.WHAT)
        self.assertEqual(stale, 0)
        self.assertEqual(self.db.execute("SELECT patternid FROM charinventory WHERE itemid = 503").fetchone(), (60010,))

    def test_undo_exact(self):
        before = self.dump(self.db)
        im.apply(self.db, self.plan(), f2.LOG, f2.META, f2.WHAT)
        self.assertIn("body", im.columns(self.db, "itemeffect"))
        back, left, _ = im.undo(self.db, False, f2.LOG, f2.META, f2.WHAT)
        self.assertEqual(left, 0)
        self.assertEqual(self.dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
