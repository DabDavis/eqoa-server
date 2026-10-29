#!/usr/bin/env python3
"""tools/seed_npc_factions.py against a made-up database: a town found from
a start spot, its people under its Citizens faction (its guards its guard
faction, a house's by its name, a town family's by its name), the creatures
among them by the hull of its people (the ghoul servant between two of its
rows), wildlife only right among them (not the forest's overhead), what is
left out and why, a namesake out of town making a place's row, the impact
and the start check (an --apply refused while a new character would be
attacked at its start), --undo exact but for a row a GM changed since, the
rule an NPC's faction is read by the server's, and the table the server's.
No game data: the NPCs and places are made up.

    python3 tools/seed_npc_factions_test.py
"""
import contextlib
import io
import os
import re
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_npc_factions as snf  # noqa: E402

SCHEMA = """
CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL);
CREATE TABLE charactermodel (race INTEGER, sex INTEGER, modelid INTEGER);
CREATE TABLE defaultcharacter (defaultCharacterID INTEGER PRIMARY KEY, tclass INTEGER, race INTEGER, humType INTEGER,
    x REAL, y REAL, z REAL, world INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL, modelid INTEGER,
    npc_type INTEGER, world INTEGER, npc_level INTEGER);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER, radius REAL, note TEXT, source TEXT);
CREATE TABLE npcundead (name TEXT PRIMARY KEY, undead INTEGER);
CREATE TABLE factionstart (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER, class INTEGER, humtype INTEGER,
    faction TEXT, standing INTEGER, note TEXT DEFAULT '', source TEXT DEFAULT 'gm');
"""
ELF, HUMAN, GHOUL, CAT, GOBLIN = 0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x55555555
FACTIONS = {"Citizens of Neriak": 1, "Citizens of Qeynos": -2000, "The Qeynos Guard": -2000, "House Do`Vexis": 1,
            "Pickclaw Goblins": -500, "The Stoneguard": 1, "Undead Frogloks": -500}


class SeedNPCFactionsTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        db.execute(snf.SCHEMA)  # the server's
        db.executemany("INSERT INTO factions VALUES (?, ?)", FACTIONS.items())
        db.executemany("INSERT INTO charactermodel VALUES (?, 0, ?)", [(2, ELF), (0, HUMAN)])
        # a dark elf shadow knight starts in "Neriak" at (1100, 0, 1040); a western human warrior in "Qeynos"
        db.execute("INSERT INTO defaultcharacter (tclass, race, humType, x, y, z, world) VALUES (3, 2, 0, 1100, 0, 1040, 0)")
        db.execute("INSERT INTO defaultcharacter (tclass, race, humType, x, y, z, world) VALUES (0, 0, 2, 5100, 0, 5010, 0)")
        rows, i = [], 100

        def npc(name, x, z, model, y=0.0, typ=0, world=0):
            nonlocal i
            i += 1
            rows.append((i, name, x, y, z, model, typ, world, 10))
            return i
        # Neriak: two rows of dark elves, 120 apart (z 1000 and 1120), 20 apart along x
        for j in range(8):
            npc(f"Elf {j}", 1000 + 20 * j, 1000, ELF)
            npc(f"Elf {j + 8}", 1000 + 20 * j, 1120, ELF)
        npc("Guard Ix", 1060, 1000, ELF)
        npc("Merchant Ox", 1080, 1120, ELF, typ=0x81)
        npc("Dianax Do`Vexis", 1100, 1000, ELF)
        self.ghoul = npc("ghoul servant", 1070, 1060, GHOUL)                 # 60 from either row: inside their hull
        npc("City Cat", 1041, 1000, CAT)                                    # wildlife, among them
        npc("Spider", 1050, 1005, CAT, y=55)                                # wildlife of the forest overhead
        npc("Grass Snake", 1180, 970, CAT)                                  # wildlife at the edge, outside the hull
        npc("Pickclaw Warrior", 1090, 1110, GOBLIN)                         # a kind's own
        npc("Training Dummy Lv20", 1010, 1110, GHOUL)
        npc("Kindred Spirit", 1030, 1010, CAT)                              # a pet (npcaggro)
        npc("Chest", 1020, 1115, GHOUL)
        npc("undead servant", 1120, 1010, GHOUL)                            # no Undead Froglok
        npc("Signpost", 1000, 1050, GHOUL, typ=0x80)                         # the NPC bit: no creature row
        # Qeynos: a row of humans
        for j in range(10):
            npc(f"Citizen {j}", 5000 + 20 * j, 5000, HUMAN)
        npc("Guard Tilman", 5050, 5005, HUMAN)
        npc("Lerda Stoneguard", 5150, 5000, HUMAN)
        npc("decaying zombie", 5060, 5020, GHOUL)                           # the town's
        npc("decaying zombie", 9000, 9000, GHOUL)                           # the wild's: a place's row
        npc("mummy", 5080, 5020, GHOUL)
        npc("mummy", 5082, 5021, GHOUL, y=80)                               # a namesake inside the place: left
        # a gnoll camp far out
        for j in range(3):
            npc("gnoll", 20000 + 10 * j, 20000, GOBLIN)
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", rows)
        db.execute("INSERT INTO npcaggro VALUES ('Kindred Spirit', 0, 0, 'a pet', 'seed')")
        db.close()

    def db(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        self.addCleanup(db.close)
        return db

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = snf.main(["--db", self.path, *args])
        return code, out.getvalue()

    def test_the_plan(self):
        p = snf.plan(self.db(), None)
        got = {(k, w, f) for k, w, x, z, r, f, t, ms in p["rows"]}
        want = {
            ("guard ix", -1, "Citizens of Neriak"),          # Neriak has no guard faction
            ("merchant ox", -1, "Citizens of Neriak"),       # the NPC bit too: its face
            ("dianax do`vexis", -1, "House Do`Vexis"),
            ("ghoul servant", -1, "Citizens of Neriak"),     # inside the hull, 60 off
            ("city cat", -1, "Citizens of Neriak"),
            ("undead servant", -1, "Citizens of Neriak"),
            ("guard tilman", -1, "The Qeynos Guard"),
            ("lerda stoneguard", -1, "The Stoneguard"),
            ("decaying zombie", 0, "Citizens of Qeynos"),    # a place: the wild's zombie is not the town's
        }
        for j in range(16):
            want.add((f"elf {j}", -1, "Citizens of Neriak"))
        for j in range(10):
            want.add((f"citizen {j}", -1, "Citizens of Qeynos"))
        self.assertEqual(got, want)
        place = next(r for r in p["rows"] if r[0] == "decaying zombie")
        self.assertEqual((place[2], place[3], place[4]), (5060.0, 5020.0, 5.0))
        left = {why: sorted(names) for why, names in p["left"].items()}
        self.assertEqual(left["of a kind's own faction"], ["Pickclaw Warrior (Neriak, Nektulos)"])
        self.assertEqual(left["a dummy"], ["Training Dummy Lv20 (Neriak, Nektulos)"])
        self.assertEqual(left["a pet or made peaceful (npcaggro)"], ["Kindred Spirit (Neriak, Nektulos)"])
        self.assertEqual(left["an object"], ["Chest (Neriak, Nektulos)"])
        self.assertEqual(left["namesakes inside its place"], ["mummy (Qeynos: 1 more)"])
        names = {k for k, *_ in p["rows"]}
        for k in ("spider", "grass snake", "gnoll", "signpost"):
            self.assertNotIn(k, names)

    def test_the_impact_and_the_start_check(self):
        db = self.db()
        p = snf.plan(db, None)
        table, groups, unsafe = snf.impact(p, snf.Starts(db))
        # the factions' own: Qeynos -2000 sets its own people on a new western human at its start
        self.assertEqual(groups, ["Human west", "Dark Elf"])
        self.assertEqual(table["Qeynos"]["Human west"], (12, 12))  # 10 citizens, the guard, the zombie: not Lerda (1)
        self.assertEqual([(c, t) for c, t, _ in unsafe], [((0, 0, 2), "Qeynos")])
        with self.assertRaises(SystemExit):
            with contextlib.redirect_stderr(io.StringIO()):
                self.run_tool("--apply", "--no-backup")
        self.assertEqual(db.execute("SELECT COUNT(*) FROM npcfaction").fetchone(), (0,))
        # the measured starts in: a western human starts at 500 with its city, a dark elf at -2000
        db.execute("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (0, -1, 2, 'Citizens of Qeynos', 500)")
        db.execute("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (0, -1, 2, 'The Qeynos Guard', 500)")
        db.execute("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (2, -1, -1, 'Citizens of Neriak', 900)")
        table, groups, unsafe = snf.impact(p, snf.Starts(db))
        self.assertEqual(unsafe, [])
        self.assertEqual(table["Qeynos"]["Human west"], (0, 0))
        self.assertEqual(table["Qeynos"]["Dark Elf"], (12, 12))  # the dark elf in Qeynos
        self.assertEqual(table["Neriak, Nektulos"]["Dark Elf"], (0, 0))
        # with a measurement the table is the measured combos' alone
        tsv = os.path.join(self.dir, "m.tsv")
        with open(tsv, "w", encoding="utf-8") as f:
            f.write("2\t3\t0\tCitizens of Qeynos\t-100\n")
        measured, _ = snf.sfs.read_measured(tsv, set(FACTIONS))
        table, _, _ = snf.impact(p, snf.Starts(db, measured))
        self.assertEqual(table["Qeynos"]["Dark Elf"], (1, 1))      # the guard's faction alone is still -2000
        self.assertIsNone(table["Qeynos"]["Human west"])          # not measured

    def test_apply_then_undo(self):
        db = self.db()
        db.execute("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (0, -1, 2, 'Citizens of Qeynos', 500)")
        db.execute("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (0, -1, 2, 'The Qeynos Guard', 500)")
        db.execute("INSERT INTO npcfaction (name, faction) VALUES ('citizen 3', 'Citizens of Neriak')")  # a GM's
        before = sorted(db.execute("SELECT * FROM npcfaction").fetchall())
        code, out = self.run_tool("--quiet")
        self.assertEqual(code, 0)
        self.assertIn("report only", out)
        self.assertIn("a GM's npcfaction row", out)
        code, out = self.run_tool("--apply", "--no-backup", "--quiet")
        self.assertEqual(code, 0, out)
        seeded = db.execute("SELECT name, source FROM npcfaction WHERE source = 'seed'").fetchall()
        self.assertEqual(len(seeded), 34)
        self.assertNotIn(("citizen 3", "seed"), seeded)
        with self.assertRaises(SystemExit):
            with contextlib.redirect_stderr(io.StringIO()):
                self.run_tool("--apply", "--no-backup")
        db.execute("UPDATE npcfaction SET faction = 'The Stoneguard' WHERE name = 'citizen 4'")
        code, out = self.run_tool("--undo", "--no-backup")
        self.assertEqual(code, 0)
        self.assertIn("changed since", out)
        left = sorted(db.execute("SELECT name, faction, source FROM npcfaction").fetchall())
        self.assertEqual(left, [("citizen 3", "Citizens of Neriak", "gm"), ("citizen 4", "The Stoneguard", "seed")])
        db.execute("DELETE FROM npcfaction WHERE name = 'citizen 4'")
        self.assertEqual(sorted(db.execute("SELECT * FROM npcfaction").fetchall()), before)
        self.assertIsNone(db.execute("SELECT name FROM sqlite_master WHERE name = 'npcfaction_seed'").fetchone())

    def test_the_server_rule(self):
        # entity/factionstart_test's TestNPCFactionPlaces
        rows = {"decaying zombie": [(-1, 0, 0, 0, "Undead of Paineel"), (0, 1000, 1000, 50, "Citizens of Neriak"),
                                    (0, 1005, 1000, 10, "House J`Narus"), (2, 0, 0, 0, "Necrological Society"),
                                    (0, 3000, 3000, 20, "")]}
        for world, x, z, want in [(0, 1006, 1000, "House J`Narus"), (0, 1030, 1000, "Citizens of Neriak"),
                                  (0, 5000, 5000, "Undead of Paineel"), (2, 1006, 1000, "Necrological Society"),
                                  (0, 3010, 3000, "")]:
            n = dict(key="decaying zombie", world=world, x=x, z=z)
            self.assertEqual(snf.npc_faction(rows, n), want, (world, x, z))

    def test_the_table_is_the_servers(self):
        here = os.path.dirname(os.path.abspath(__file__))
        with open(os.path.join(here, "..", "internal", "database", "factions.go"), encoding="utf-8") as f:
            go = f.read()
        server = re.search(r"CREATE TABLE IF NOT EXISTS npcfaction \((.*?)\);", go, re.S).group(1)
        ours = sqlite3.connect(":memory:")
        ours.execute(snf.SCHEMA)
        theirs = sqlite3.connect(":memory:")
        theirs.execute(f"CREATE TABLE npcfaction ({server})")
        self.assertEqual([r[1:] for r in ours.execute("PRAGMA table_info(npcfaction)")],
                         [r[1:] for r in theirs.execute("PRAGMA table_info(npcfaction)")])
        ours.close()
        theirs.close()


if __name__ == "__main__":
    unittest.main()
