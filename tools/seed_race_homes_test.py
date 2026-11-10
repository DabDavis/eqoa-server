#!/usr/bin/env python3
"""tools/seed_race_homes.py against a made-up database: a race of racedefs with
start rows, two NPCs to copy (a talker of the race's body, a creature of
another's), a merchant's wares, an npcaggro row. No game data. Apply writes the
flavour, moves every start row into the recipe's world facing the point given,
places the people (each a copy of its template but its name, place, facing,
level and role; a body given replaces the copy's and its gear), copies the
wares and makes their seller a merchant (the flag CLIENT opens the window
by), makes the critters passive (a GM's row of one kept for the undo), writes
the people's own words (npcdialog by name, the table made where the server
never ran and dropped again at the undo); a second apply is refused; undo puts
every table back as it was, row for row (the AUTOINCREMENT counters move on,
as any delete leaves them). Lines for no one of the home, and wares from a
seller of nothing, are refused. A name with an npcaggro row already (a GM's)
keeps it; a hostile name gets a rule of aggressive 1; an optional person whose
like or whose wares' seller the database lacks is left out; a foe named as
NPCs of another world are is refused; --format lays the recipe file out again.

    python3 tools/seed_race_homes_test.py
"""
import json
import math
import os
import re
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_race_homes as srh  # noqa: E402

SCHEMA = [
    """CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT NOT NULL, wire_race INTEGER NOT NULL,
        description TEXT NOT NULL DEFAULT '')""",
    """CREATE TABLE defaultcharacter (defaultCharacterID INTEGER PRIMARY KEY AUTOINCREMENT, tclass INTEGER NOT NULL,
        race INTEGER NOT NULL, x REAL NOT NULL, y REAL NOT NULL, z REAL, facing REAL NOT NULL, world INTEGER)""",
    """CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL,
        facing INTEGER, world INTEGER, dt_kill DATE, modelid INTEGER, size REAL, primary_weapon INTEGER, secondary_weapon
        INTEGER, shield INTEGER, torso INTEGER, forearm INTEGER, gloves INTEGER, legs INTEGER, feet INTEGER, head INTEGER,
        race TEXT, npc_level INTEGER, npc_type INTEGER, npc_id INTEGER)""",
    """CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL,
        stackleft INTEGER NOT NULL DEFAULT 0, listnumber INTEGER NOT NULL DEFAULT 0)""",
    """CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL
        DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""",
]

RECIPE = {
    "world": 10, "name": "Test", "zones": [
        {"name": "Nest", "from": "TUNARIA.ESF", "zone": 115, "race": 14, "home": {
            "flavor": "Bird folk of the nest.",
            "start": {"x": 100.0, "y": 50.0, "z": 200.0, "toward": [100.0, 210.0]},
            "npcs": [
                {"name": "Elder", "like": "aviak", "level": 30, "role": "talker", "at": [100.0, 50.0, 210.0], "toward": [100.0, 200.0],
                 "stock": "Merchant"},
                {"name": "scrub rat", "body": "0x9E004757", "level": 1, "role": "foe", "size": 0.8, "at": [150.0, 40.0, 150.0]},
            ],
            "peaceful": ["scrub rat", "Elder"],
            "lines": {"Elder": ["Welcome, %r.", "Fly well."]},
        }},
        {"name": "Elsewhere", "from": "ODUS.ESF", "zone": 7, "race": 16},
    ]}


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    for q in SCHEMA:
        db.execute(q)
    db.execute("INSERT INTO racedefs VALUES (14, 'Aviak', 6, 'Old words.')")
    for c in (0, 7):
        db.execute("INSERT INTO defaultcharacter (tclass, race, x, y, z, facing, world) VALUES (?, 14, 13059.3, 54.4, 4289.6, 1.28, 0)", (c,))
    db.execute("INSERT INTO defaultcharacter (tclass, race, x, y, z, facing, world) VALUES (0, 6, 13059.3, 54.4, 4289.6, 1.28, 0)")
    db.execute("""INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, facing, world, modelid, size, primary_weapon, race, npc_level,
        npc_type, npc_id) VALUES ('aviak', 1, 2, 3, 9, 0, 827449107, 1.0, 1234, '0', 22, 0, 1)""")
    db.execute("""INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, facing, world, modelid, size, primary_weapon, race, npc_level,
        npc_type, npc_id) VALUES ('grave rat', 5, 6, 7, 0, 0, 2650818391, 1.0, 0, '0', 19, 0, 2)""")
    db.execute("""INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, facing, world, modelid, size, race, npc_level, npc_type, npc_id)
        VALUES ('Merchant', 9, 9, 9, 0, 0, 1, 1.0, '0', 20, 128, 3)""")
    for p in (11, 12):
        db.execute("INSERT INTO npcitem (npcid, patternid, listnumber) VALUES (3, ?, ?)", (p, p))
    db.execute("INSERT INTO npcaggro VALUES ('Elder', 1, 30, 'a GM', 'gm')")
    return db


def dump(db):
    return "\n".join(db.iterdump())


class SeedRaceHomes(unittest.TestCase):
    def test_apply_and_undo(self):
        with tempfile.TemporaryDirectory() as d:
            db = make_db(os.path.join(d, "t.db"))
            before = dump(db)
            steps = srh.plan(db, RECIPE)
            srh.apply(db, steps)
            self.assertEqual(db.execute("SELECT description FROM racedefs WHERE id = 14").fetchone()[0], "Bird folk of the nest.")
            rows = db.execute("SELECT world, x, y, z, facing FROM defaultcharacter WHERE race = 14").fetchall()
            self.assertEqual(len(rows), 2)
            for w, x, y, z, f in rows:
                self.assertEqual((w, x, y, z), (10, 100.0, 50.0, 200.0))
                self.assertAlmostEqual(f, 0.0, places=5)  # toward +z: heading 0
            self.assertEqual(db.execute("SELECT world FROM defaultcharacter WHERE race = 6").fetchone()[0], 0, "another race's row")
            elder = db.execute("SELECT id, npc_name, world, facing, npc_level, npc_type, modelid, primary_weapon, npc_id FROM npcs "
                               "WHERE npc_name = 'Elder'").fetchone()
            self.assertEqual(elder[1:8], ("Elder", 10, 128, 30, 129, 827449107, 1234))  # facing -z: half a turn; a merchant
            self.assertEqual(elder[8], elder[0], "its npc_id is its id")
            rat = db.execute("SELECT world, modelid, primary_weapon, size, npc_level, npc_type FROM npcs WHERE npc_name = 'scrub rat'").fetchone()
            self.assertEqual(rat, (10, 0x9E004757, 0, 0.8, 1, 0))
            self.assertEqual(db.execute("SELECT patternid FROM npcitem WHERE npcid = ? ORDER BY listnumber", (elder[0],)).fetchall(), [(11,), (12,)])
            self.assertEqual(db.execute("SELECT npc_name, aggressive, radius, source FROM npcaggro ORDER BY npc_name").fetchall(),
                             [("Elder", 1, 30.0, "gm"), ("scrub rat", 0, 0.0, "racehomes")],
                             "a rule that was there (a GM's Elder) is kept as it is; the rats' is the homes'")
            self.assertIn("kept as it is", [st for st in steps if st[0] == "skip"][0][4])
            self.assertEqual(db.execute("SELECT name, line, weight, source, town, role, race FROM npcdialog ORDER BY id").fetchall(),
                             [("Elder", "Welcome, %r.", 1, "racehomes", "", "", -1), ("Elder", "Fly well.", 1, "racehomes", "", "", -1)])
            with self.assertRaises(SystemExit):
                srh.apply(db, srh.plan(db, RECIPE))
            srh.undo(db)
            self.assertEqual(self.rows(db), self.rows_of(before, d), "undo is exact, table for table")

    def rows(self, db):
        out = {}
        for t in ("racedefs", "defaultcharacter", "npcs", "npcitem", "npcaggro"):
            out[t] = db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall()
        for t in (srh.LOG, "npcdialog"):
            out[t] = db.execute("SELECT count(*) FROM sqlite_master WHERE name = ?", (t,)).fetchone()[0]
        if out["npcdialog"]:
            out["npcdialog rows"] = db.execute("SELECT * FROM npcdialog ORDER BY 1").fetchall()
        return out

    def test_words_join_a_table_the_server_made(self):
        with tempfile.TemporaryDirectory() as d:
            db = make_db(os.path.join(d, "t.db"))
            db.execute(srh.NPCDIALOG_DDL)
            db.execute("INSERT INTO npcdialog (role, line, source) VALUES ('guard', 'Move along.', 'ours')")
            before = self.rows(db)
            srh.apply(db, srh.plan(db, RECIPE))
            self.assertEqual(db.execute("SELECT count(*) FROM npcdialog").fetchone()[0], 3)
            srh.undo(db)
            self.assertEqual(self.rows(db), before, "the server's table and its own line kept")

    def test_refusals(self):
        with tempfile.TemporaryDirectory() as d:
            db = make_db(os.path.join(d, "t.db"))
            import copy
            r = copy.deepcopy(RECIPE)
            r["zones"][0]["home"]["lines"] = {"Nobody": ["Hello."]}
            with self.assertRaises(SystemExit):
                srh.plan(db, r)
            db.execute("DELETE FROM npcitem")
            with self.assertRaises(SystemExit):
                srh.plan(db, RECIPE)

    def test_a_hostile_foe_an_optional_person_and_a_name_of_its_own(self):
        import copy
        with tempfile.TemporaryDirectory() as d:
            db = make_db(os.path.join(d, "t.db"))
            before = self.rows(db)
            r = copy.deepcopy(RECIPE)
            home = r["zones"][0]["home"]
            home["npcs"] += [
                {"name": "Razortusk", "body": "0x9E004757", "level": 5, "role": "foe", "at": [160.0, 40.0, 160.0]},
                # optional, and here: placed, a merchant of the seller's wares
                {"name": "Spell Archivist", "like": "aviak", "role": "talker", "at": [101.0, 50.0, 205.0], "stock": "Merchant",
                 "optional": True},
                # optional, and this database has no such seller, no such row to copy: left out, and said
                {"name": "Loremaster", "like": "aviak", "role": "talker", "at": [102.0, 50.0, 205.0], "stock": "Spell Archivist",
                 "optional": True},
                {"name": "Ghost", "like": "no such row", "role": "talker", "at": [103.0, 50.0, 205.0], "optional": True},
            ]
            home["hostile"] = ["Razortusk"]
            steps = srh.plan(db, r)
            skipped = [st[4] for st in steps if st[0] == "skip"]
            self.assertEqual(len(skipped), 3, skipped)   # the Loremaster, the Ghost, and the GM's Elder rule kept
            self.assertTrue(any("Loremaster" in x and "no NPC 'Spell Archivist'" in x for x in skipped), skipped)
            self.assertTrue(any("Ghost" in x and "no npcs row like" in x for x in skipped), skipped)
            srh.apply(db, steps)
            self.assertEqual(db.execute("SELECT aggressive, source FROM npcaggro WHERE npc_name = 'Razortusk'").fetchone(), (1, "racehomes"))
            arch = db.execute("SELECT id, npc_type FROM npcs WHERE npc_name = 'Spell Archivist'").fetchone()
            self.assertEqual(arch[1], 129)
            self.assertEqual(db.execute("SELECT count(*) FROM npcitem WHERE npcid = ?", (arch[0],)).fetchone()[0], 2)
            self.assertEqual(db.execute("SELECT count(*) FROM npcs WHERE npc_name IN ('Loremaster', 'Ghost')").fetchone()[0], 0)
            srh.undo(db)
            self.assertEqual(self.rows(db), before, "undo is exact")
            # the controls: not optional, the same lacks are refusals; a rule for no one of the home; both rules for one
            for npc in ({"name": "Loremaster", "like": "aviak", "role": "talker", "at": [1.0, 2.0, 3.0], "stock": "Spell Archivist"},
                        {"name": "Ghost", "like": "no such row", "role": "talker", "at": [1.0, 2.0, 3.0]}):
                bad = copy.deepcopy(RECIPE)
                bad["zones"][0]["home"]["npcs"].append(npc)
                with self.assertRaises(SystemExit):
                    srh.plan(db, bad)
            bad = copy.deepcopy(RECIPE)
            bad["zones"][0]["home"]["hostile"] = ["Nobody"]
            with self.assertRaises(SystemExit):
                srh.plan(db, bad)
            bad = copy.deepcopy(RECIPE)
            bad["zones"][0]["home"]["hostile"] = ["scrub rat"]
            with self.assertRaises(SystemExit):
                srh.plan(db, bad)
            # a foe named as a creature of another world is: refused (npcaggro and a quest's kill go by name)
            bad = copy.deepcopy(RECIPE)
            bad["zones"][0]["home"]["npcs"][1]["name"] = "Grave Rat"
            bad["zones"][0]["home"]["peaceful"] = ["Grave Rat", "Elder"]
            with self.assertRaises(SystemExit) as e:
                srh.plan(db, bad)
            self.assertIn("a name of its own", str(e.exception))

    def test_a_seller_of_the_home_is_no_source_of_wares(self):
        # a second seeding after an undo: the Spell Archivist's wares are the other world's seller's, whatever stands in the home
        with tempfile.TemporaryDirectory() as d:
            db = make_db(os.path.join(d, "t.db"))
            db.execute("""INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, facing, world, modelid, size, race, npc_level, npc_type, npc_id)
                VALUES ('Merchant', 9, 9, 9, 0, 10, 1, 1.0, '0', 20, 129, 4)""")   # a seller of the name in the recipe's world, no wares
            steps = srh.plan(db, RECIPE)
            elder = [st for st in steps if st[0] == "npc" and st[3]["row"]["npc_name"] == "Elder"][0]
            self.assertEqual([it["patternid"] for it in elder[3]["stock"]], [11, 12])

    def test_the_recipe_file_is_laid_out_as_format_writes_it(self):
        text = open(srh.DEFAULT_HOMES).read()
        self.assertEqual(srh.format_recipe(json.loads(text)), text, "tools/seed_race_homes.py --format lays it out again")
        self.assertIn('  "extraFiles": ["homelands-edges.json"],\n', text)

    def rows_of(self, dumped, d):
        db = sqlite3.connect(os.path.join(d, "before.db"))
        db.executescript(dumped)
        return self.rows(db)

    def test_a_race_without_rows_is_refused(self):
        with tempfile.TemporaryDirectory() as d:
            db = make_db(os.path.join(d, "t.db"))
            db.execute("DELETE FROM defaultcharacter WHERE race = 14")
            with self.assertRaises(SystemExit):
                srh.plan(db, RECIPE)

    def test_headings(self):
        self.assertAlmostEqual(srh.heading_to(0, 0, 1, 0), math.pi / 2)
        self.assertEqual(srh.byte_heading(math.pi), 128)
        self.assertEqual(srh.byte_heading(2 * math.pi - 0.001), 0)


ZONES = {  # the disc's zones' cells (each world's 2000-unit grid), for the real recipe's check
    ("ODUS.ESF", 7): (2, 1), ("ODUS.ESF", 8): (3, 1), ("ODUS.ESF", 13): (2, 2), ("ODUS.ESF", 14): (3, 2),
    ("ODUS.ESF", 19): (2, 3), ("ODUS.ESF", 22): (5, 3), ("ODUS.ESF", 27): (4, 4), ("ODUS.ESF", 28): (5, 4),
    ("RATHE.ESF", 15): (2, 4),
}


class TheRecipe(unittest.TestCase):
    """tools/worlds/homelands.json and tools/quests/data/homelands.json as written: every home's start, people and
    critters stand in its own zone's cell (where homes touch, a player may walk into a neighbour's, but nothing
    of a home is placed in another's), its peaceful names are its foes', and its gatekeeper flies the player to
    its mainland and binds it there. Zones whose cell this table lacks are held to their notes' "cell c,r"."""

    def test_a_homes_travel_people_are_named_once(self):
        # a coachman is his stable (the coach menus and the ledgers go by his town, his menu by his name), a
        # gatekeeper his flight: no two homes share one's name, and the homes' zone names (the stables' towns) differ
        with open(srh.DEFAULT_HOMES) as f:
            recipe = json.load(f)
        seen, zones = {}, set()
        for z in recipe["zones"]:
            if not z.get("home"):
                continue
            self.assertNotIn(z["name"], zones, f"two homes are named {z['name']}")
            zones.add(z["name"])
            for n in z["home"]["npcs"]:
                if n["name"].startswith(("Coachman ", "Coachwoman ", "Gatekeeper ", "Spiritmaster ")):
                    self.assertIn(seen.setdefault(n["name"], z["name"]), (z["name"],), f"{n['name']} stands in two homes")
            coaches = [n for n in z["home"]["npcs"] if n["name"].startswith(("Coachman ", "Coachwoman "))]
            self.assertLessEqual(len(coaches), 1, f"{z['name']}: one stable a home")
            for n in coaches:
                self.assertNotIn(n["name"], z["home"].get("lines", {}), f"{n['name']} opens his menu: he says no lines")

    def test_homes_stand_in_their_zones(self):
        recipe = json.load(open(srh.DEFAULT_HOMES))
        quests = json.load(open(os.path.join(HERE, "quests", "data", "homelands.json")))["quests"]
        cells = set()
        for z in recipe["zones"]:
            cell = ZONES.get((z["from"], z["zone"]))
            if cell is None:
                m = re.search(r"cell (\d+),\s*(\d+)\)", z.get("note", ""))
                self.assertIsNotNone(m, f"{z['name']}: no cell known")
                cell = (int(m.group(1)), int(m.group(2)))
            self.assertNotIn(cell, cells, f"{z['name']}: two zones in cell {cell}")
            cells.add(cell)
            h = z.get("home")
            if not h:
                continue
            spots = [("start", (h["start"]["x"], h["start"]["z"]))] + [(n["name"], (n["at"][0], n["at"][2])) for n in h["npcs"]]
            for who, (x, zz) in spots:
                self.assertEqual((int(x // 2000), int(zz // 2000)), cell, f"{z['name']}: {who} at ({x}, {zz}) is outside cell {cell}")
            foes = {n["name"] for n in h["npcs"] if n.get("role") == "foe"}
            self.assertTrue(set(h.get("peaceful", [])) <= foes, f"{z['name']}: a peaceful name that is none of its foes")
            self.assertTrue(set(h.get("hostile", [])) <= foes, f"{z['name']}: a hostile name that is none of its foes")
            self.assertFalse(set(h.get("hostile", [])) & set(h.get("peaceful", [])), f"{z['name']}: a name both peaceful and hostile")
            # a home's every creature has a rule: wildlife of level 5 and under attacks no one, and anything else
            # (a body that is no player's, outside the towns) attacks on sight: which it is is the home's to say
            self.assertEqual(foes, set(h.get("peaceful", [])) | set(h.get("hostile", [])),
                             f"{z['name']}: each creature is peaceful or hostile, by name")
            gates = [n["name"] for n in h["npcs"] if n["name"].startswith("Gatekeeper ")]
            self.assertEqual(len(gates), 1, f"{z['name']}: one gatekeeper")
            flights = [q for q in quests if q["giver"] == gates[0] and any(
                e.get("do") == "teleport" and e.get("to") == "mainland" and e.get("bind")
                for st in q["steps"] for e in st.get("events", []))]
            self.assertEqual(len(flights), 1, f"{z['name']}: {gates[0]} gives one flight to the mainland that binds")
            self.assertTrue(flights[0].get("repeatable"), f"{z['name']}: the flight is repeatable")


if __name__ == "__main__":
    unittest.main()
