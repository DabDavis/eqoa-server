#!/usr/bin/env python3
"""tools/seed_signs.py against a made-up world: a road sign's directions on a
small map of our own (the nearest town each way, a farther one and one out of
reach left out, a quarter with no town named by the region that way, the sea
skipped), a city map legend of our own read in its columns (a building's
title, a keeper's shop by trade, a person the walkthrough cites by number, a
sign in the city but at no building pointed at its places, a sign past the
gate a road sign), the boards and a sign named for its place; then --apply
(a row a sign, the news row, a GM's row untouched, run again only the signs
without one), --undo (each row still as written taken back, a GM's change
kept, the table it made dropped: the dump as before), and a saved plan
applied with a moved sign left out. No game data.

    python3 tools/seed_signs_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_signs.py")
sys.path.insert(0, HERE)
import seed_signs  # noqa: E402

BODY = seed_signs.SIGN_BODY
HUMAN = 0x11223344

NPCS_DDL = """CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER,
    x_coord REAL, y_coord REAL, z_coord REAL, modelid INTEGER, npc_type INTEGER)"""


def npc(i, name, x, z, body=HUMAN, y=50.0, world=0):
    return {"id": i, "name": name, "world": world, "x": x, "y": y, "z": z, "body": body}


class Patched(unittest.TestCase):
    """Runs with the tool's tables swapped for this file's own."""
    PATCH = {}

    def setUp(self):
        self.saved = {k: getattr(seed_signs, k) for k in self.PATCH}
        for k, v in self.PATCH.items():
            setattr(seed_signs, k, v)

    def tearDown(self):
        for k, v in self.saved.items():
            setattr(seed_signs, k, v)


class RoadSigns(Patched):
    """The nearest town in each quarter, within reach; the region where
    none is."""
    # a grid of 4 x 4 zones of 2,000 (rows and columns from 1, as CLIENT's):
    # words 1-16, 99 the sea
    PATCH = {
        "COACH_TOWNS": [
            ("Northton", 10100, 7000),   # north, 3,002
            ("Farnorth", 10000, 4100),   # north, farther
            ("Eastby", 13000, 10500),    # east
            ("Southwick", 9000, 14000),  # south (194 degrees)
            ("Toofar", 2500, 10000),     # west, out of reach
        ],
        "TOWN_REACH": 7000.0,
        "ZONES_TUNARIA": [[1, 2, 3, 4], [5, 6, 7, 8], [99, 99, 11, 12], [13, 14, 15, 16]],
        "TOWN_SIGN_NAMES": {"Eastby": "East-by-the-Sea"},
    }
    WORDS = {1: "Frostfield", 5: "Old Wood", 6: "Mudflat", 7: "Hilltop", 99: "Open Sea", 11: "Crossing", 15: "Moor"}

    def words(self):
        w = [""] * 100
        for k, v in self.WORDS.items():
            w[k] = v
        return w

    def test_the_nearest_town_each_way(self):
        # Farnorth is north too, but farther; Toofar is west, past the reach,
        # and the zones west of the sign are off the grid: west is left out
        sign = npc(1, "signpost", 10000, 10000, BODY)
        text, note = seed_signs.road_text(sign, self.words())
        self.assertEqual(text.split("\n"), ["North: Northton", "East: East-by-the-Sea", "South: Southwick"], note)

    def test_the_region_where_no_town_is(self):
        # the sign stands in row 3 col 3 (x and z 6000-8000: word 11,
        # Crossing); north is row 2 col 3 (7, Hilltop), south row 4 col 3
        # (15, Moor); west is the sea twice, then off the grid: left out
        seed_signs.COACH_TOWNS = [("Eastby", 9500, 7000)]
        sign = npc(1, "signpost", 7000, 7000, BODY)
        text, _ = seed_signs.road_text(sign, self.words())
        self.assertEqual(text.split("\n"), ["North: Hilltop", "East: East-by-the-Sea", "South: Moor"])
        # without the words a quarter with no town says nothing
        text, _ = seed_signs.road_text(sign, None)
        self.assertEqual(text, "East: East-by-the-Sea")

    def test_quarters(self):
        q = seed_signs.quarter_of
        self.assertEqual([q(0), q(44.9), q(45.1), q(134.9), q(135), q(180), q(225), q(225.1), q(315), q(315.1)],
                         ["North", "North", "East", "East", "South", "South", "South", "West", "North", "North"])
        # x runs east and z south
        self.assertEqual(q(seed_signs.bearing(0, 0, 0, -10)), "North")
        self.assertEqual(q(seed_signs.bearing(0, 0, 10, 0)), "East")
        self.assertEqual(q(seed_signs.bearing(0, 0, 0, 10)), "South")


LEGEND = """
                    Hobbits
Map of Testburg
1 Gilded Lantern Inn                 4 West Gate
  Innkeeper Bram                       Guard Tibbet
  Merchant Ostra
                          n
2 Smithy Hollan                      5 Mossy Chapel
  Smithy Pell                          Brother Eamon
3 Merchant Quill                     6 Old Tower
  Merchant Dunn                        Watchman Hale
7d Tannery
  K Tanner Oslo
                                                 primagames.com   12
Spell Merchants
Speak to Tobin Rask 1 , who keeps the cellar.
"""


class CitySigns(Patched):
    """A city the guide maps: its places by the people standing there."""
    PATCH = {
        "COACH_TOWNS": [("Testburg", 5000, 5000), ("Otherton", 9000, 5000)],
        "PRIMA_CITIES": {"Testburg": "Testburg"},
        "BOARD_WELCOME": {},
    }

    def setUp(self):
        super().setUp()
        self.lines = LEGEND.split("\n")
        self.people = [
            npc(10, "Innkeeper Bram", 5000, 5000), npc(11, "Merchant Ostra", 5010, 5005),
            npc(12, "Smithy Hollan", 5100, 5000), npc(13, "Smithy Pell", 5105, 5004),
            npc(14, "Guard Tibbet", 4800, 5000), npc(15, "Brother Eamon", 4990, 5130),
            npc(16, "Merchant Quill", 5050, 5150), npc(17, "Tobin Rask", 5005, 4990),
            npc(19, "Watchman Hale", 4800, 5300), npc(20, "Tanner Oslo", 5040, 5060),
            npc(18, "Innkeeper Bram", 9000, 9000),  # a namesake of no city's
        ]

    def plan(self, *signs):
        return {r["npcid"]: r for r in seed_signs.plan_signs(self.people + list(signs), self.lines, None)}

    def test_the_legend_in_columns(self):
        legend = seed_signs.map_legend(self.lines, "Testburg")
        self.assertEqual([(e["no"], e["title"], e["people"]) for e in legend], [
            ("1", "Gilded Lantern Inn", ["Innkeeper Bram", "Merchant Ostra"]),
            ("4", "West Gate", ["Guard Tibbet"]),
            ("2", "Smithy Hollan", ["Smithy Pell"]),
            ("5", "Mossy Chapel", ["Brother Eamon"]),
            ("3", "Merchant Quill", ["Merchant Dunn"]),
            ("6", "Old Tower", ["Watchman Hale"]),
            ("7d", "Tannery", ["K Tanner Oslo"]),
        ])
        self.assertEqual(seed_signs.map_legend(self.lines, "Nowhere"), [])

    def test_a_building_and_a_shop(self):
        p = self.plan(npc(100, "signpost", 5020, 5020, BODY), npc(101, "signpost", 5120, 5010, BODY),
                      npc(102, "signpost", 5060, 5170, BODY))
        self.assertEqual((p[100]["kind"], p[100]["text"]), ("building", "Gilded Lantern Inn"))
        self.assertEqual((p[101]["kind"], p[101]["text"]), ("building", "Smithy"))
        # a merchant's place is its keeper's, by name
        self.assertEqual((p[102]["kind"], p[102]["text"]), ("building", "Merchant Quill"))

    def test_the_users_word_for_a_place(self):
        # a keeper's place the user named (Neriak's 26, Madam Du`Klot's: the Blind Fish Tavern)
        seed_signs.PLACE_TITLES[("Testburg", "3")] = "The Quill and Ink"
        try:
            p = self.plan(npc(102, "signpost", 5060, 5170, BODY))
        finally:
            del seed_signs.PLACE_TITLES[("Testburg", "3")]
        self.assertEqual((p[102]["kind"], p[102]["text"]), ("building", "The Quill and Ink"))
        self.assertEqual(seed_signs.PLACE_TITLES[("Neriak", "26")], "Blind Fish Tavern")

    def test_the_walkthrough_places_a_person(self):
        places = {p["no"]: p for p in seed_signs.city_places(self.lines, self.people)}
        self.assertIn("Tobin Rask", [n["name"] for n in places["1"]["people"]])
        self.assertNotIn(18, [n["id"] for n in places["1"]["people"]])
        # a map mark run into a name is no part of it
        self.assertEqual([n["name"] for n in places["7d"]["people"]], ["Tanner Oslo"])

    def test_in_the_city_and_past_its_gate(self):
        p = self.plan(npc(103, "signpost", 4920, 5040, BODY), npc(104, "signpost", 4700, 5000, BODY),
                      npc(108, "signpost", 4760, 5070, BODY), npc(109, "signpost", 4850, 5080, BODY))
        # among the places, at none of them: its places each way
        self.assertEqual(p[103]["kind"], "city")
        self.assertEqual(p[103]["text"].split("\n"), ["East: Gilded Lantern Inn", "South: Mossy Chapel", "West: West Gate"])
        # far outside the city's outline: the road
        self.assertEqual(p[104]["kind"], "road")
        self.assertIn("East: Testburg", p[104]["text"])
        # just outside the outline, its nearest place the west gate and
        # farther from the middle than the gate's guard: the road; the control,
        # just inside the gate, nearer the middle than the guard: the city
        self.assertEqual(p[108]["kind"], "road")
        self.assertEqual(p[109]["kind"], "city")

    def test_boards_and_named_signs(self):
        p = self.plan(npc(105, "Testburg Announcements", 5000, 5050, BODY),
                      npc(106, "signpost: Old Mill Road", 5000, 5040, BODY),
                      npc(107, "Testburg Anouncements", 5000, 5060, BODY))
        self.assertEqual((p[105]["kind"], p[105]["text"]), ("board", "Welcome to Testburg."))
        self.assertEqual((p[107]["kind"], p[107]["text"]), ("board", "Welcome to Testburg."))
        self.assertEqual((p[106]["kind"], p[106]["text"]), ("place", "Old Mill Road"))

    def test_without_the_guide_every_sign_is_a_road_sign(self):
        p = {r["npcid"]: r for r in seed_signs.plan_signs(self.people + [npc(100, "signpost", 5020, 5020, BODY)], None, None)}
        self.assertEqual(p[100]["kind"], "road")


class ApplyAndUndo(unittest.TestCase):
    """--apply, --undo and a saved plan, on a made-up database."""

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.db)
        db.execute(NPCS_DDL)
        db.executemany("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, modelid, npc_type) "
                       "VALUES (?, ?, 0, ?, 50, ?, ?, 384)",
                       [(1, "signpost", 12000.0, 12000.0, BODY), (2, "Testburg Announcements", 5000.0, 5000.0, BODY),
                        (3, "a rat", 12010.0, 12000.0, HUMAN)])
        db.commit()
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r

    def q(self, sql, *args):
        c = sqlite3.connect(self.db)
        try:
            return c.execute(sql, args).fetchall()
        finally:
            c.close()

    def dump(self):
        c = sqlite3.connect(self.db)
        try:
            return "\n".join(c.iterdump())
        finally:
            c.close()

    def test_apply_then_undo(self):
        before = self.dump()
        out = self.tool().stdout
        self.assertIn("report only", out)
        self.assertEqual(self.dump(), before, "a report wrote")
        self.tool("--apply")
        rows = {r[0]: r[1:] for r in self.q("SELECT npcid, kind, source FROM signtext")}
        self.assertEqual(rows, {0: ("news", "ours"), 1: ("road", "ours"), 2: ("board", "ours")})
        self.assertEqual(self.q("SELECT text FROM signtext WHERE npcid = 0"), [("",)])
        self.tool("--undo")
        self.assertEqual(self.dump(), before)

    def test_a_gms_row_is_kept(self):
        db = sqlite3.connect(self.db)
        db.executescript(seed_signs.SCHEMA + ";")
        db.execute("INSERT INTO signtext (npcid, kind, text, source) VALUES (1, 'gm', 'Beware of the rats.', 'gm')")
        db.commit()
        db.close()
        self.tool("--apply")
        self.assertEqual(self.q("SELECT kind, text, source FROM signtext WHERE npcid = 1"), [("gm", "Beware of the rats.", "gm")])
        # a sign added later is given a row by --apply again, and nothing else changes
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, modelid, npc_type) "
                   "VALUES (4, 'signpost', 0, 20000, 50, 20000, ?, 384)", (BODY,))
        db.execute("UPDATE signtext SET text = 'Welcome, friend.' WHERE npcid = 2")
        db.commit()
        db.close()
        self.tool("--apply")
        self.assertEqual(sorted(r[0] for r in self.q("SELECT npcid FROM signtext")), [0, 1, 2, 4])
        self.assertEqual(self.q("SELECT text FROM signtext WHERE npcid = 2"), [("Welcome, friend.",)])
        out = self.tool("--undo").stdout
        self.assertIn("took back 2 rows; 1 a GM changed", out)
        self.assertEqual(sorted(r[0] for r in self.q("SELECT npcid FROM signtext")), [1, 2])
        self.assertEqual(self.q("SELECT count(*) FROM sqlite_master WHERE name = 'signtext_seed'"), [(0,)])

    def test_a_saved_plan(self):
        plan = os.path.join(self.dir.name, "plan.json")
        self.tool("--save-plan", plan)
        with open(plan) as f:
            saved = json.load(f)
        self.assertEqual(sorted(r["npcid"] for r in saved), [1, 2])
        # the board moved since: left out
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET x_coord = 5100 WHERE id = 2")
        db.commit()
        db.close()
        out = self.tool("--plan", plan, "--apply").stdout
        self.assertIn("left out: 2 'Testburg Announcements'", out)
        self.assertEqual(sorted(r[0] for r in self.q("SELECT npcid FROM signtext")), [0, 1])
        self.assertEqual(self.tool("--plan", plan, "--words", plan, ok=False).returncode != 0, True)


if __name__ == "__main__":
    unittest.main()
