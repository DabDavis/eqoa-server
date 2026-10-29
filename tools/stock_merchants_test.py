#!/usr/bin/env python3
"""tools/stock_merchants.py against a made-up database, a made-up Return Home
export and a made-up harvest: Return Home's catalogs reach our merchants
through the import's map (a held pattern dropped, a creature's loot not, a
catalog already there kept); a trade's merchant takes the catalog of its
region's level, a town's second smithy another one, a town's chemists the
three lists; a merchant of no trade the general goods, a jeweler plain
jewelry; the left out (a Keeper, a beast, one Sandstorm shows unflagged, one a
GM stocked by hand) are logged; prices measured by name and by id, Sandstorm's
0 kept, the rule's by damage and by the nearest items, a nonzero price and a
GM's pattern never changed; --apply then --undo exact (and checked by md5), a
second --apply refused, a row changed since left alone. No game data: every
name, item and number is made up.

    python3 tools/stock_merchants_test.py
"""
import contextlib
import io
import json
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import stock_merchants as sm  # noqa: E402

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL,
    world INTEGER, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER NOT NULL DEFAULT 0,
    remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL DEFAULT 0,
    itemicon INTEGER NOT NULL DEFAULT 0, equipslot INTEGER NOT NULL DEFAULT -1, trade INTEGER NOT NULL DEFAULT 0,
    weapondamage INTEGER NOT NULL DEFAULT 0, levelreq INTEGER NOT NULL DEFAULT 0, maxstack INTEGER NOT NULL DEFAULT 1,
    lore INTEGER NOT NULL DEFAULT 0, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL DEFAULT '',
    str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER, intelligence INTEGER,
    HPMAX INTEGER, POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER, DR INTEGER, FR INTEGER,
    CR INTEGER, LR INTEGER, AR INTEGER);
CREATE TABLE prh_import_npcs (src_id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, status TEXT NOT NULL,
    reason TEXT, dst_id INTEGER, fixes TEXT);
CREATE TABLE prh_import_ids (tbl TEXT NOT NULL, src_id INTEGER, dst_id INTEGER NOT NULL, status TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
    before TEXT, after TEXT);
CREATE TABLE grocer_stock (what TEXT NOT NULL, key INTEGER NOT NULL, old INTEGER);
CREATE TABLE eqoadata_spell_import_rows (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, key INTEGER NOT NULL,
    key2 INTEGER, note TEXT);
"""
PRH_SCHEMA = """
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL,
    listnumber INTEGER NOT NULL DEFAULT 0);
"""
SCROLL = sm.SCROLL_ICON


def dump(db):
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
        out[t] = sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
    return out


class World:
    """The made-up world: a town at (1000, 1000) among level 5 beasts, a town
    at (9000, 9000) among level 40 beasts, a town in world 2."""

    def __init__(self, path, prh_path):
        self.db = db = sqlite3.connect(path, isolation_level=None)
        db.executescript(SCHEMA)
        self.prh = sqlite3.connect(prh_path, isolation_level=None)
        self.prh.executescript(PRH_SCHEMA)
        self.p = {}

    def item(self, name, cost=0, slot=-1, level=1, dmg=0, ac=0, icon=0, stack=1, desc="", trade=0, lore=0, st=0, pid=None):
        cur = self.db.execute("""INSERT INTO itempattern (patternid, itemcost, itemicon, equipslot, trade, weapondamage,
            levelreq, maxstack, lore, itemname, itemdesc, AC, str) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
                              (pid, cost, icon, slot, trade, dmg, level, stack, lore, name, desc, ac, st))
        self.p[name] = cur.lastrowid
        return cur.lastrowid

    def npc(self, name, x, z, world=0, level=10, flags=0x81, key=0, rid=None):
        cur = self.db.execute("""INSERT INTO npcs (id, npc_name, x_coord, y_coord, z_coord, world, npc_level, npc_type,
            npc_id) VALUES (?, ?, ?, 0, ?, ?, ?, ?, ?)""", (rid, name, x, z, world, level, flags, key))
        return cur.lastrowid

    def stock(self, key, *names):
        for n, name in enumerate(names):
            self.db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                            "VALUES (?, 1, ?, -1, 1, ?)", (key, self.p[name], n))


def build(dirname):
    """The database, the Return Home export and the harvest the tests share."""
    w = World(os.path.join(dirname, "t.db"), os.path.join(dirname, "prh.db"))
    db = w.db
    # beasts: the regions' levels
    for i in range(30):
        w.npc("sand rat", 1000 + i * 10, 1200, level=5, flags=0)
        w.npc("dire bear", 9000 + i * 10, 9200, level=40, flags=0)
        w.npc("sea beast", 5000 + i * 10, 5200, world=2, level=40, flags=0)
    # items: two low weapon sets, a high one; armor; scrolls; supplies; jewelry
    for name, lvl, dmg in (("Tin Knife", 4, 40), ("Tin Axe", 6, 60), ("Reed Club", 5, 50), ("Reed Staff", 7, 70),
                           ("Gem Sword", 40, 400), ("Gem Axe", 42, 420)):
        w.item(name, slot=14, level=lvl, dmg=dmg)
    w.item("Hide Cap", slot=1, level=5, ac=4)
    w.item("Hide Vest", slot=5, level=5, ac=12)
    for lvl in (1, 10, 20, 30, 40, 50):
        w.item(f"Brew {lvl}", icon=SCROLL, level=lvl, desc="A scroll.")
    w.item("leather strip")
    w.item("silk cord")
    w.item("Copper Ring", slot=8, level=3, ac=0, st=2)
    w.item("Silver Earring", slot=3, level=4, st=3)
    w.item("Ring of Might", slot=8, level=3, st=5)   # a drop's name: no jeweler's
    w.item("Bob's Band", slot=8, level=3, st=5)       # someone's: no jeweler's
    w.item("Gold Necklace", slot=4, level=30, st=20)
    w.item("Spare Stud", slot=3, level=2, trade=1)    # no trade: no jeweler's
    w.item("Priced Thing", cost=77, level=3)          # a price already: never changed
    w.item("GM Thing", level=3)                       # a GM's pattern: its 0 stays
    w.item("Handout Cap", slot=1, level=1, ac=1)      # Sandstorm sells it at 0
    w.item("Old Name Axe", slot=14, level=8, dmg=80, pid=5000)  # Sandstorm's 5000, renamed
    w.item("Held Thing")                              # Return Home's, held by the import
    w.item("Beast Fang")
    # the region's merchants; the low town at 1000,1000
    rh_ours = w.npc("Smithy Able", 1000, 1000)
    rh_imp = w.npc("Merchant Baker", 1010, 1000, rid=100001, key=100001)
    stocked = w.npc("Tailor Kept", 1020, 1000, key=0)
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (stocked,))
    w.stock(stocked, "Hide Cap")
    low1 = w.npc("Smithy Cain", 1030, 1000)
    low2 = w.npc("Smithy Dorn", 1040, 1000)
    high = w.npc("Weaponsmith Eld", 9000, 9000)
    far = w.npc("Smithy Far", 5000, 5000, world=2)
    gold = w.npc("Smithy Gold", 9050, 9000)       # sells the high set already (a model)
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (gold,))
    w.stock(gold, "Gem Sword", "Gem Axe")
    reed = w.npc("Smithy Reed", 1100, 1000)       # sells a low set already
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (reed,))
    w.stock(reed, "Reed Club", "Reed Staff")
    chem_src = []
    for n, lists in enumerate((("Brew 1", "Brew 10"), ("Brew 20", "Brew 30"), ("Brew 40", "Brew 50"))):
        i = w.npc(f"Chemist Src{n}", 9100 + n, 9000)
        db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (i,))
        w.stock(i, *lists)
        chem_src.append(i)
    chems = [w.npc(f"Chemist New{n}", 1000 + n, 1050) for n in range(3)]
    # tailors: three tiers sold elsewhere; three new tailors in the low town
    for n, (lvl, ac) in enumerate(((5, 2), (15, 6), (30, 12), (55, 20))):
        w.item(f"Tier{n} Cap", slot=1, level=lvl, ac=ac)
        w.item(f"Tier{n} Vest", slot=5, level=lvl, ac=ac * 3)
        i = w.npc(f"Tailor Src{n}", 9200 + n, 9000)
        db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (i,))
        w.stock(i, f"Tier{n} Cap", f"Tier{n} Vest")
    tailors = [w.npc(f"Tailor New{n}", 1000 + n, 1150) for n in range(5)]
    general = w.npc("Merchant Plain", 1000, 1100)
    jeweler = w.npc("Jeweler Gem", 1010, 1100)
    keeper = w.npc("Keeper Lore", 1020, 1100)
    beast = w.npc("grizzly bear", 1030, 1100)
    unflagged = w.npc("Merchant Shy", 1040, 1100)   # Sandstorm shows it without the flag
    gmed = w.npc("Merchant Hand", 1050, 1100)       # a GM emptied its catalog
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (gmed,))
    db.execute("INSERT INTO gm_audit (tbl, rowkey, before) VALUES ('npcitem', '{\"itemid\": 9}', ?)",
               (json.dumps({"itemid": 9, "npcid": gmed, "patternid": 1}),))
    db.execute("INSERT INTO gm_audit (tbl, rowkey, after) VALUES ('itempattern', ?, '{}')",
               (json.dumps({"patternid": w.p["GM Thing"]}),))
    # Sandstorm's own: a tailor the harvest opened (three wares here, by id,
    # by name, by a renamed id; one this database lacks), which Return Home
    # stocks too; a smithy none of whose wares are here; a grocer stocked
    # already; a merchant whose namesake here stands too far
    w.item("Cotton Shirt", slot=5, level=5, ac=8, pid=6001)
    w.item("Cotton Cap", slot=1, level=5, ac=2, pid=6100)
    w.item("Old Pants", slot=10, level=5, ac=5, pid=6003)
    sand = w.npc("Tailor Sand", 1000, 2000)       # a town of their own
    moldy = w.npc("Smithy Moldy", 1050, 2000)
    for name, kind, lvl, cost in (("Ale", "beverage that aids in power regeneration", 3, 30),
                                  ("Stew", "meal that will increase health regeneration", 4, 40),
                                  ("Wine", "beverage that aids in power regeneration", 5, 50)):
        w.item(name, cost=cost, level=lvl, desc=f"A refreshing {kind}.")
        db.execute("INSERT INTO grocer_stock VALUES ('itemcost', ?, 0)", (w.p[name],))
    db.execute("INSERT INTO gm_audit (tbl, rowkey, after) VALUES ('itempattern', ?, '{}')",
               (json.dumps({"patternid": w.p["Wine"]}),))
    grocer = w.npc("Grocer Kept", 1100, 2000)
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (grocer,))
    w.stock(grocer, "Ale", "Stew", "Wine")
    farname = w.npc("Merchant Far", 3100, 3000)
    # scrolls import_eqoadata_spells.py made: at 1 (repriced), at 2, at 1 a
    # GM edited; and one at 1 it did not make
    for name, cost, made, gm in (("Made Scroll", 1, True, False), ("Made Two", 2, True, False),
                                 ("Made GM", 1, True, True), ("Other Scroll", 1, False, False)):
        w.item(name, cost=cost, icon=SCROLL, level=20, desc="A scroll.")
        if made:
            db.execute("INSERT INTO eqoadata_spell_import_rows (tbl, key) VALUES ('itempattern', ?)", (w.p[name],))
        if gm:
            db.execute("INSERT INTO gm_audit (tbl, rowkey, after) VALUES ('itempattern', ?, '{}')",
                       (json.dumps({"patternid": w.p[name]}),))
    vendor = w.npc("Merchant Scrolls", 9300, 9000)
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (vendor,))
    w.stock(vendor, "Made Scroll", "Made Two", "Made GM", "Other Scroll")
    loot = w.npc("cave beetle", 1060, 1100, flags=0)
    db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (loot,))
    w.stock(loot, "Beast Fang")                    # its corpse's loot here: no catalog, no price
    # Return Home's export: its rows 1 (ours: Smithy Able), 2 (imported:
    # Merchant Baker), 3 (ours: Tailor Kept, stocked here), 4 (a beetle's loot)
    for src, name, status, dst in ((1, "Smithy Able", "ours", rh_ours), (2, "Merchant Baker", "imported", rh_imp),
                                   (3, "Tailor Kept", "ours", stocked), (4, "cave beetle", "ours", loot),
                                   (5, "Tailor Sand", "ours", sand)):
        db.execute("INSERT INTO prh_import_npcs VALUES (?, ?, 0, ?, '', ?, '')", (src, name, status, dst))
    # Return Home's patterns 501.. -> ours
    for src, name, status in ((501, "Tin Knife", "ours"), (502, "Tin Axe", "ours"), (503, "Held Thing", "held"),
                              (504, "Priced Thing", "ours"), (505, "Hide Vest", "ours"), (506, "Beast Fang", "ours"),
                              (507, "GM Thing", "ours"), (508, "Old Name Axe", "ours"), (509, "Handout Cap", "ours")):
        db.execute("INSERT INTO prh_import_ids VALUES ('itempattern', ?, ?, ?)", (src, w.p[name], status))
    for npcid, pids in ((1, (501, 502, 503)), (2, (504, 507, 508, 509)), (3, (505,)), (4, (506,)), (5, (505,))):
        for n, pid in enumerate(pids):
            w.prh.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (?, ?, ?)", (npcid, pid, n))
    # the harvest: wares and an inventory with Sandstorm's values; what it saw
    harvest = {
        "server": "test", "when": 1, "character": {"name": "Tester", "world": 0},
        "merchants": [
          {"name": "Tailor Sand", "pos": [1010, 0, 2005], "npc_flags": 129, "wares": [
            {"id": 6001, "name": "Cotton Shirt", "value": 19, "wear_slot": 5, "level_req": 5, "stats": [[14, 8]]},
            {"id": 6002, "name": "Cotton Cap", "value": 14, "wear_slot": 1, "level_req": 5, "stats": [[14, 2]]},
            {"id": 6003, "name": "Cotton Pants", "value": 15, "wear_slot": 10, "level_req": 5, "stats": [[14, 5]]},
            {"id": 6004, "name": "Cotton Boots", "value": 15, "wear_slot": 11, "level_req": 5, "stats": [[14, 3]]}]},
          {"name": "Smithy Moldy", "pos": [1050, 0, 2010], "npc_flags": 129, "wares": [
            {"id": 7001, "name": "Knife Mold", "value": 10, "level_req": 1, "stats": []}]},
          {"name": "Grocer Kept", "pos": [1100, 0, 2000], "npc_flags": 129, "wares": [
            {"id": 7101, "name": "Ale", "value": 25, "level_req": 3, "desc": "A refreshing beverage.", "stats": []},
            {"id": 7103, "name": "Wine", "value": 45, "level_req": 5, "desc": "A refreshing beverage.", "stats": []}]},
          {"name": "Merchant Far", "pos": [3000, 0, 3000], "npc_flags": 129, "wares": [
            {"id": 7201, "name": "Far Thing", "value": 5, "level_req": 1, "stats": []}]},
          {"name": "Smithy Elsewhere", "pos": [0, 0, 0], "npc_flags": 129, "wares": [
            {"id": 900, "name": "Tin Knife", "value": 73, "wear_slot": 14, "level_req": 4, "damage": 40, "max_stack": 1, "stats": []},
            {"id": 901, "name": "Reed Club", "value": 91, "wear_slot": 14, "level_req": 5, "damage": 50, "max_stack": 1, "stats": []},
            {"id": 902, "name": "Reed Staff", "value": 128, "wear_slot": 14, "level_req": 7, "damage": 70, "max_stack": 1, "stats": []},
            {"id": 903, "name": "Gem Sword", "value": 730, "wear_slot": 14, "level_req": 40, "damage": 400, "max_stack": 1, "stats": []},
            {"id": 904, "name": "Handout Cap", "value": 0, "wear_slot": 1, "level_req": 1, "stats": [[14, 1]]},
            {"id": 5000, "name": "New Name Axe", "value": 146, "wear_slot": 14, "level_req": 8, "damage": 80, "max_stack": 1, "stats": []},
            {"id": 905, "name": "Hide Cap", "value": 30, "wear_slot": 1, "level_req": 5, "stats": [[14, 4]]},
            {"id": 906, "name": "leather strip", "value": 40, "stats": []},
            {"id": 907, "name": "Brew 1", "value": 11, "icon": SCROLL, "level_req": 1, "stats": []},
        ]}],
        "inventory": [
            {"id": 950, "name": "Plain Band", "value": 100, "wear_slot": 8, "level_req": 3, "stats": [[0, 2]]},
            {"id": 951, "name": "Plain Hoop", "value": 140, "wear_slot": 3, "level_req": 5, "stats": [[0, 3]]},
            {"id": 952, "name": "Fine Chain", "value": 2000, "wear_slot": 4, "level_req": 30, "stats": [[0, 20]]},
            {"id": 955, "name": "Fine Torque", "value": 2200, "wear_slot": 4, "level_req": 31, "stats": [[0, 21]]},
            {"id": 956, "name": "Fine Pendant", "value": 1800, "wear_slot": 4, "level_req": 29, "stats": [[0, 19]]},
            {"id": 957, "name": "Plain Loop", "value": 120, "wear_slot": 8, "level_req": 4, "stats": [[0, 2]]},
            {"id": 953, "name": "Bit of Bark", "value": 12, "level_req": 1, "stats": []},
            {"id": 954, "name": "Brew 10", "value": 160, "icon": SCROLL, "level_req": 10, "stats": []},
        ],
        "seen": [{"name": "Merchant Shy", "npc_flags": 128}],
    }
    with open(os.path.join(dirname, "harvest.json"), "w") as f:
        json.dump(harvest, f)
    w.ids = dict(rh_ours=rh_ours, rh_imp=rh_imp, stocked=stocked, low1=low1, low2=low2, high=high, far=far,
                 chems=chems, chem_src=chem_src, tailors=tailors, general=general, jeweler=jeweler, keeper=keeper, beast=beast,
                 unflagged=unflagged, gmed=gmed, loot=loot, gold=gold, reed=reed, sand=sand, moldy=moldy,
                 grocer=grocer, farname=farname, vendor=vendor)
    db.close()
    w.prh.close()
    return w


class StockMerchantsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.dir, True)
        self.w = build(self.dir)
        self.ids = self.w.ids
        self.path = os.path.join(self.dir, "t.db")
        self.prh = os.path.join(self.dir, "prh.db")
        self.harvest = os.path.join(self.dir, "harvest.json")

    def db(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        self.addCleanup(db.close)
        return db

    def plan(self, prices=None):
        db = self.db()
        prh = sqlite3.connect(f"file:{self.prh}?mode=ro", uri=True)
        self.addCleanup(prh.close)
        patterns = sm.load_patterns(db)
        m = sm.Measured()
        m.load(prices if prices is not None else [self.harvest])
        m.add_db(db, patterns)
        m.fill_items(patterns)
        return sm.plan(db, prh, m, patterns), patterns, m

    def names(self, patterns, items):
        return [patterns[x]["name"] for x in items]

    def test_return_homes_catalogs_through_the_map(self):
        p, pats, _ = self.plan()
        items, src = p.stock[self.ids["rh_ours"]]
        self.assertEqual(self.names(pats, items), ["Tin Knife", "Tin Axe"])  # the held pattern dropped
        self.assertEqual(src, "Return Home's catalog")
        self.assertEqual(self.names(pats, p.stock[self.ids["rh_imp"]][0]),
                         ["Priced Thing", "GM Thing", "Old Name Axe", "Handout Cap"])
        self.assertNotIn(self.ids["stocked"], p.stock)   # a catalog already: kept
        self.assertNotIn(self.ids["loot"], p.stock)      # a beetle's loot: no merchant
        self.assertEqual(p.prh_report[("ours", "already stocked")], 1)
        self.assertEqual(p.prh_report[("ours", "not a merchant here (loot)")], 1)
        self.assertEqual(p.prh_report[("ours", "stocked from it")], 1)
        self.assertEqual(p.prh_report[("imported", "stocked from it")], 1)

    def test_sandstorms_own_catalog(self):
        p, pats, _ = self.plan()
        items, src = p.stock[self.ids["sand"]]
        # Sandstorm's order; by id, by name, by a renamed id; the lacking one listed, not made
        self.assertEqual(self.names(pats, items), ["Cotton Shirt", "Cotton Cap", "Old Pants"])
        self.assertEqual(src, "Sandstorm's catalog (3 of 4 wares here)")
        got, missing, renamed, near = p.sandstorm[self.ids["sand"]]
        self.assertEqual(missing, ["Cotton Boots"])
        self.assertEqual(renamed, ["Cotton Pants = our Old Pants"])
        self.assertFalse(near)
        self.assertEqual(p.prh_report[("ours", "Sandstorm's own catalog instead")], 1)
        # none of the smithy's wares here: the next tier, its trade's
        self.assertEqual(p.sandstorm[self.ids["moldy"]][0], [])
        self.assertTrue(p.stock[self.ids["moldy"]][1].startswith("weapons:"))
        # a merchant with a catalog keeps it; a namesake 100 away is no match
        self.assertNotIn(self.ids["grocer"], p.stock)
        self.assertEqual([m["name"] for m in p.misses], ["Merchant Far", "Smithy Elsewhere"])
        self.assertEqual(p.stock[self.ids["farname"]][1], "general goods")
        # the prices: Sandstorm's, by name and by the renamed id
        price = {pats[x]["name"]: v for x, (v, _) in p.prices.items()}
        self.assertEqual((price["Cotton Shirt"], price["Cotton Cap"], price["Old Pants"]), (19, 14, 15))

    def test_the_grocers_exception(self):
        p, pats, _ = self.plan()
        got = {pats[x]["name"]: (old, new) for x, (old, new, _) in p.reprice.items()}
        # a grocer's price of ours that Sandstorm sells otherwise: Sandstorm's
        self.assertEqual(got, {"Ale": (30, 25)})
        # not: a grocer's item Sandstorm sells not (Stew) or a GM's (Wine)
        for name in ("Stew", "Wine", "Made Scroll"):
            self.assertNotIn(name, got)

    def test_a_trades_catalog_by_level_and_town(self):
        p, pats, _ = self.plan()
        a = self.names(pats, p.stock[self.ids["low1"]][0])
        b = self.names(pats, p.stock[self.ids["low2"]][0])
        # the low town's levels (5): the low sets, one each (Smithy Reed's,
        # and Smithy Able's from Return Home), never the level 40 one
        self.assertEqual(sorted([a, b]), sorted([["Reed Club", "Reed Staff"], ["Tin Knife", "Tin Axe"]]))
        # the high town's (40): the high set; world 2 (40, no holder there) too
        self.assertEqual(self.names(pats, p.stock[self.ids["high"]][0]), ["Gem Sword", "Gem Axe"])
        self.assertEqual(self.names(pats, p.stock[self.ids["far"]][0]), ["Gem Sword", "Gem Axe"])
        self.assertIn("world 0", p.stock[self.ids["far"]][1])

    def test_a_towns_tailors_sell_its_tiers(self):
        p, pats, _ = self.plan()
        got = [self.names(pats, p.stock[i][0])[0] for i in self.ids["tailors"]]
        # the low town (5): the four sets within reach it lacks, the nearest
        # the level first (Tailor Sand's and Tier0's both 5: the nearer
        # holder's first); then one it has (its own Tailor Kept's, the
        # nearest holder), before the far one (55)
        self.assertEqual(got, ["Cotton Shirt", "Tier0 Cap", "Tier1 Cap", "Tier2 Cap", "Hide Cap"])

    def test_a_towns_chemists_sell_the_three_lists(self):
        p, pats, _ = self.plan()
        lists = sorted(tuple(self.names(pats, p.stock[i][0])) for i in self.ids["chems"])
        self.assertEqual(lists, [("Brew 1", "Brew 10"), ("Brew 20", "Brew 30"), ("Brew 40", "Brew 50")])

    def test_general_goods_and_jewelry(self):
        p, pats, _ = self.plan()
        self.assertEqual(self.names(pats, p.stock[self.ids["general"]][0]), ["leather strip", "silk cord"])
        jew = self.names(pats, p.stock[self.ids["jeweler"]][0])
        self.assertEqual(sorted(jew), ["Copper Ring", "Gold Necklace", "Silver Earring"])

    def test_left_out(self):
        p, _, _ = self.plan()
        self.assertIn("Keeper", p.left_out[self.ids["keeper"]])
        self.assertIn("creature", p.left_out[self.ids["beast"]])
        self.assertIn("Sandstorm", p.left_out[self.ids["unflagged"]])
        self.assertIn("GM", p.left_out[self.ids["gmed"]])
        for k in ("keeper", "beast", "unflagged", "gmed"):
            self.assertNotIn(self.ids[k], p.stock)
        # without the harvest Sandstorm's flags are unknown: a plain merchant
        p2, _, _ = self.plan(prices=[os.path.join(self.dir, "prices.tsv")] if self.write_tsv() else None)
        self.assertNotIn(self.ids["unflagged"], p2.left_out)

    def write_tsv(self):
        with open(os.path.join(self.dir, "prices.tsv"), "w") as f:
            f.write("name\tprice\nTin Knife\t70\nGem Sword\t700\nleather strip\t44\n")
        return True

    def test_prices(self):
        p, pats, m = self.plan()
        price = {pats[x]["name"]: v for x, (v, _) in p.prices.items()}
        how = {pats[x]["name"]: h for x, (_, h) in p.prices.items()}
        self.assertEqual(price["Tin Knife"], 73)            # measured, by name
        self.assertEqual(how["Tin Knife"], "measured")
        self.assertEqual(price["Old Name Axe"], 146)        # measured by id: same slot and level
        self.assertTrue(how["Old Name Axe"].startswith("measured by id"))
        self.assertEqual(price["leather strip"], 40)
        self.assertNotIn("Priced Thing", price)            # a price already
        self.assertNotIn("Beast Fang", price)              # a creature's loot: not a merchant's
        self.assertNotIn("GM Thing", price)                # a GM's pattern
        self.assertIn(pats_id(pats, "GM Thing"), p.gm_priced)
        self.assertIn(pats_id(pats, "Handout Cap"), p.free)  # Sandstorm's 0
        # ours: a weapon without stats by damage, fitted on the measured ones (1.82..1.83 a point)
        a = m and p.rule.a
        self.assertAlmostEqual(a, 1.8255, places=2)
        self.assertEqual(price["Tin Axe"], round(a * 60))
        self.assertEqual(how["Tin Axe"], "ours: damage")
        # ours: jewelry by the nearest measured jewelry
        self.assertEqual(how["Copper Ring"], "ours: nearest")
        self.assertLess(price["Copper Ring"], price["Gold Necklace"])
        self.assertEqual(p.unpriced, [])
        errs = p.rule.errors()
        self.assertIn("weapon", errs)

    def test_a_price_list(self):
        self.write_tsv()
        p, pats, _ = self.plan(prices=[os.path.join(self.dir, "prices.tsv")])
        price = {pats[x]["name"]: v for x, (v, _) in p.prices.items()}
        self.assertEqual(price["Tin Knife"], 70)
        self.assertEqual(price["leather strip"], 44)

    def test_apply_then_undo(self):
        db = self.db()
        before = dump(db)
        argv = ["--db", self.path, "--prh", self.prh, "--prices", self.harvest, "--no-backup"]
        self.assertEqual(quiet(argv + ["--apply"]), 0)
        db = self.db()
        # the catalogs, by the key the server reads (npc_id; set where it was 0)
        key = db.execute("SELECT npc_id FROM npcs WHERE id = ?", (self.ids["rh_ours"],)).fetchone()[0]
        self.assertEqual(key, self.ids["rh_ours"])
        rows = db.execute("""SELECT p.itemname, p.itemcost, i.listnumber FROM npcitem i JOIN itempattern p USING(patternid)
            WHERE i.npcid = ? ORDER BY i.listnumber""", (key,)).fetchall()
        self.assertEqual([r[0] for r in rows], ["Tin Knife", "Tin Axe"])
        self.assertTrue(all(r[1] > 0 for r in rows))
        # every merchant stocked but the left out; every catalog item priced but Sandstorm's 0 and the GM's
        left = {k for (k,) in db.execute("SELECT key FROM merchant_stock WHERE what = 'left out'")}
        empty = [i for (i,) in db.execute("""SELECT id FROM npcs n WHERE npc_type & 1 AND NOT EXISTS
            (SELECT 1 FROM npcitem i WHERE i.npcid = n.npc_id AND n.npc_id != 0)""")]
        self.assertEqual(sorted(empty), sorted(left))
        free = {k for (k,) in db.execute("SELECT key FROM merchant_stock WHERE what IN ('free', 'gm price')")}
        zero = {k for (k,) in db.execute("""SELECT DISTINCT p.patternid FROM npcitem i JOIN itempattern p USING(patternid)
            JOIN npcs n ON n.npc_id = i.npcid WHERE n.npc_type & 1 AND p.itemcost <= 0""")}
        self.assertEqual(zero, free)
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE itemname = 'Priced Thing'").fetchone()[0], 77)
        cost = dict(db.execute("SELECT itemname, itemcost FROM itempattern WHERE itemname IN "
                               "('Ale', 'Stew', 'Wine', 'Made Scroll', 'Made Two', 'Made GM', 'Other Scroll')"))
        self.assertEqual(cost, {"Ale": 25, "Stew": 40, "Wine": 50, "Made Scroll": 1, "Made Two": 2, "Made GM": 1,
                                "Other Scroll": 1})
        # a second --apply is refused
        with self.assertRaises(SystemExit):
            quiet(argv + ["--apply"])
        # undo: exactly as before, the md5 check with it
        self.assertEqual(quiet(["--db", self.path, "--undo", "--no-backup"]), 0)
        self.assertEqual(dump(self.db()), before)

    def test_undo_leaves_a_row_changed_since(self):
        argv = ["--db", self.path, "--prh", self.prh, "--prices", self.harvest, "--no-backup"]
        quiet(argv + ["--apply"])
        db = self.db()
        db.execute("UPDATE itempattern SET itemcost = 999 WHERE itemname = 'Tin Axe'")   # a GM's price
        row = db.execute("SELECT itemid FROM npcitem WHERE npcid = ? AND listnumber = 0", (self.ids["rh_ours"],)).fetchone()[0]
        db.execute("UPDATE npcitem SET listnumber = 7 WHERE itemid = ?", (row,))         # a GM's reorder
        db.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (?, ?, 9)",  # a GM's new row
                   (self.ids["rh_ours"], self.w.p["Gem Axe"]))
        n, kept, before, after = sm.undo(db)
        self.assertEqual(db.execute("SELECT itemcost FROM itempattern WHERE itemname = 'Tin Axe'").fetchone()[0], 999)
        self.assertEqual(db.execute("SELECT listnumber FROM npcitem WHERE itemid = ?", (row,)).fetchone()[0], 7)
        self.assertEqual(len(kept), 2)
        self.assertNotEqual(before["npcitem"], after["npcitem"])
        self.assertIsNone(db.execute("SELECT name FROM sqlite_master WHERE name = 'merchant_stock'").fetchone())

    def test_no_measured_prices_refused(self):
        db = self.db()
        with self.assertRaises(SystemExit):
            quiet(["--db", self.path, "--prh", self.prh, "--no-backup"])
        self.assertIsNone(db.execute("SELECT name FROM sqlite_master WHERE name = 'merchant_stock'").fetchone())

    def test_names_and_kinds(self):
        self.assertEqual(sm.norm("  Grocer  G`Zar$ "), "grocer g'zar")
        self.assertEqual(sm.trade_of("Pole Merchant Ann"), "fishing")
        self.assertEqual(sm.trade_of("Crostell the Jeweler"), "jeweler")
        self.assertEqual(sm.trade_of("baga shopkeeper"), "general")
        self.assertEqual(sm.trade_of("Seamster Hal"), "tailor")
        self.assertIsNone(sm.trade_of("Bob Smithers"))
        item = dict(name="x", slot=-1, stack=1, icon=SCROLL, desc="", dmg=0, level=1, stats={})
        self.assertEqual(sm.kind(item), "scroll")
        self.assertEqual(sm.kind(dict(item, icon=0, slot=6)), "jewelry")
        self.assertEqual(sm.kind(dict(item, icon=0, slot=13)), "armor")
        self.assertEqual(sm.kind(dict(item, icon=0, name="arrow", stack=99)), "ammo")


def quiet(argv):
    """sm.main(argv), its report kept out of the test's output."""
    with contextlib.redirect_stdout(io.StringIO()):
        return sm.main(argv)


def pats_id(pats, name):
    return next(x for x, p in pats.items() if p["name"] == name)


if __name__ == "__main__":
    unittest.main()
