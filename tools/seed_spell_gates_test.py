#!/usr/bin/env python3
"""tools/seed_spell_gates.py against a made-up database and a made-up ground
(flat squares in the server's ground format): a pyramid's top written as it
is, a place on the terrain at the terrain's height, beside an NPC in front of
it, a zone's middle moved off a pinnacle, a chamber's floor under a ceiling
12.5 over it (not the ceiling), a group's radius its record's own; --apply
leaves a GM's row alone, a later run writes only what is missing (not a row
an earlier run wrote that a GM took out since), --undo takes the last run
back, a row a GM changed since kept, and the table when the first run made
it; no ground, nothing written. No game data: places and names made up.

    python3 tools/seed_spell_gates_test.py
"""
import json
import os
import sqlite3
import struct
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_spell_gates.py")
sys.path.insert(0, HERE)
import seed_spell_gates as sg  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL,
                   facing INTEGER, world INTEGER, npc_type INTEGER, modelid INTEGER);
CREATE TABLE charactermodel (sex INTEGER NOT NULL, modelid INTEGER NOT NULL PRIMARY KEY, race INTEGER NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
                          effects TEXT NOT NULL DEFAULT '');
CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'self', place TEXT NOT NULL DEFAULT '',
                         world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0,
                         z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
                         source TEXT NOT NULL DEFAULT 'gm');
"""

# a made-up plan: a pyramid, a stone, an NPC, a zone's middle (on a
# pinnacle), a chamber under a ceiling, and a spell a GM placed already
PLAN = [
    ("Test Gate", sg.SELF, 0, "the Pyramid", ("top", 0, 1000.0, 81.5, 1000.0, "a pyramid"), "Self, Teleport to the Pyramid"),
    ("Test Portal", sg.GROUP, 15, "the Stone", sg.stone(0, 3000.0, 47.0, 3000.0, "a stone"), "Group, Teleport to the Stone"),
    ("Test Circle", sg.GROUP, 0, "the Hermit", sg.beside("Hermit", 2, "by the hermit"), "Group, Teleport to the Hermit"),
    ("Test Escape", sg.SELF, 0, "the Middle", sg.middle(0, 4000, 4000, "C", "Middle"), "Self, Teleport to the Middle"),
    ("Kept Gate", sg.SELF, 0, "a GM's", sg.stone(0, 3000.0, 50.0, 3000.0, "a stone"), "Self"),
]

# a later run's: a chamber's floor (a group's radius its record's) and a
# group with no radius in its record
LATER = [
    ("Test Chamber", sg.GROUP, sg.OWN, "the Chamber", sg.stone(0, 6000.0, 50.0, 6000.0, "a chamber"), "Group, Teleport to the Chamber"),
    ("Test Bare", sg.GROUP, sg.OWN, "the Stone", sg.stone(0, 3000.0, 47.0, 3000.0, "a stone"), "Group, Teleport to the Stone"),
]


def square(cx, cz, half, y):
    """A flat square of terrain round (cx, cz), as two triangles: (min x, min
    z, max x, max z, y, the strip's vertices)."""
    return (cx - half, cz - half, cx + half, cz + half, y,
            [(cx - half, y, cz - half), (cx + half, y, cz - half), (cx - half, y, cz + half), (cx + half, y, cz + half)])


def write_ground(path, zones):
    """internal/ground's format: "EQGR", version 1, the zone count, each zone's
    entry (bounds, base, the strips' offset, strip and vertex counts), then
    each zone's strip (a vertex count, int16 x z in quarters from the base
    less 32768, y in eighths)."""
    head = b"EQGR" + struct.pack("<II", 1, len(zones))
    entries, bodies = b"", b""
    off = len(head) + 44 * len(zones)
    for minx, minz, maxx, maxz, y, verts in zones:
        body = struct.pack("<H", len(verts))
        for vx, vy, vz in verts:
            body += struct.pack("<hhh", int(round((vx - minx) * 4 - 32768)), int(round((vy - y) * 8)), int(round((vz - minz) * 4 - 32768)))
        entries += struct.pack("<7fqII", minx, minz, maxx, maxz, minx, y, minz, off, 1, len(verts))
        off += len(body)
        bodies += body
    with open(path, "wb") as f:
        f.write(head + entries + bodies)


def make(dir):
    db = os.path.join(dir, "eqoa.db")
    c = sqlite3.connect(db)
    c.executescript(SCHEMA)
    c.executemany("INSERT INTO spellpattern (spellname) VALUES (?)", [(n,) for n, *_ in PLAN + LATER])
    chamber = c.execute("SELECT spellid FROM spellpattern WHERE spellname = 'Test Chamber'").fetchone()[0]
    c.execute("INSERT INTO spelleffect VALUES (?, 'Group, Teleport to a place', 0, 'test', ?)",
              (chamber, '{"v":1,"scope":"group","radius":25,"effects":[{"type":"Teleport","kind":"teleport"}]}'))
    c.executemany("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, facing, world, npc_type, modelid) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", [
        ("Hermit", 500.0, 50.0, 500.0, 0, 2, 128, 1),        # in front of him: +3 z (facing 0)
        ("cave bear", 3000.0, 47.0, 3005.0, 0, 0, 0, 99),      # a creature by the stone
        ("Guard", 3000.0, 47.0, 2990.0, 0, 0, 0, 7),           # a human's body: townsfolk
    ])
    c.execute("INSERT INTO charactermodel VALUES (0, 7, 0)")
    c.execute("INSERT INTO spellgates (spell, kind, place, world, x, y, z, source) VALUES ('kept gate', 'self', 'a GM''s', 0, 1, 2, 3, 'gm')")
    c.commit()
    c.close()
    ground = os.path.join(dir, "ground")
    os.mkdir(ground)
    # Tunaria: the pyramid's terrain at 50, the stone's at 47, the middle's a
    # pinnacle (a 20-wide top at 120) on a plain at 50, and a chamber's floor
    # at 50 under its ceiling at 62.5 (the ground's floors hold both)
    write_ground(os.path.join(ground, "0.ground"), [
        square(1000, 1000, 100, 50), square(3000, 3000, 100, 47), square(5000, 5000, 400, 50), square(5000, 5000, 10, 120),
        square(6000, 6000, 100, 50), square(6000, 6000, 20, 62.5)])
    write_ground(os.path.join(ground, "2.ground"), [square(500, 500, 100, 50)])
    return db, ground


def rows(db):
    c = sqlite3.connect(db)
    out = {r[0]: r[1:] for r in c.execute("SELECT spell, kind, place, world, x, y, z, radius, source FROM spellgates")}
    c.close()
    return out


def log(db):
    c = sqlite3.connect(db)
    out = sorted(c.execute("SELECT spell, row, applied FROM spell_gate_seed"))
    c.close()
    return out


def dump(db):
    c = sqlite3.connect(db)
    out = "\n".join(c.iterdump())
    c.close()
    return out


def run(db, *args):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", db, "--no-backup", *args],
                          capture_output=True, text=True)


class SeedSpellGates(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db, self.ground = make(self.dir.name)
        self.saved = sg.GATES
        sg.GATES = PLAN

    def tearDown(self):
        sg.GATES = self.saved
        self.dir.cleanup()

    def plan(self):
        c = sqlite3.connect(self.db)
        try:
            return sg.plan(c, self.ground)
        finally:
            c.close()

    def test_places(self):
        planned, missing = self.plan()
        self.assertEqual(missing, [])
        by = {r["spell"]: r for r in planned}
        # a pyramid's top as written, and the server lands there as written
        p = by["test gate"]
        self.assertEqual((p["x"], p["y"], p["z"]), (1000.0, 81.5, 1000.0))
        self.assertEqual(p["_land"][:3], (1000.0, 81.5, 1000.0))
        # a stone on its terrain; the landing 40 from the bear (not the guard)
        s = by["test portal"]
        self.assertEqual((s["y"], s["radius"], s["kind"]), (47.0, 15.0, "group"))
        lx, ly, lz, d, room = s["_land"]
        self.assertGreaterEqual(((lx - 3000) ** 2 + (lz - 3005) ** 2) ** 0.5, sg.CLEAR)
        # in front of the hermit
        h = by["test circle"]
        self.assertEqual((h["world"], h["x"], h["z"], h["y"]), (2, 500.0, 503.0, 50.0))
        # a zone's middle off its pinnacle: level ground, not the 120 top
        m = by["test escape"]
        self.assertEqual(m["y"], 50.0)
        self.assertGreater(((m["x"] - 5000) ** 2 + (m["z"] - 5000) ** 2) ** 0.5, 10)

    def test_no_ground_writes_nothing(self):
        before = dump(self.db)
        out = subprocess.run([sys.executable, "-I", TOOL, "--db", self.db, "--ground", os.path.join(self.dir.name, "none"), "--apply", "--no-backup"],
                             capture_output=True, text=True)
        self.assertNotEqual(out.returncode, 0)
        self.assertEqual(dump(self.db), before)

    def test_a_chamber_and_its_records_radius(self):
        sg.GATES = LATER
        planned, missing = self.plan()
        self.assertEqual(missing, [])
        by = {r["spell"]: r for r in planned}
        # the chamber's floor, not its ceiling 12.5 over it; under a roof, the
        # server's walls decide where it lands (room None)
        ch = by["test chamber"]
        self.assertEqual((ch["x"], ch["y"], ch["z"], ch["radius"]), (6000.0, 50.0, 6000.0, 25.0))
        self.assertIsNone(ch["_land"][4])
        # the control: no radius in the record, the server's (0)
        self.assertEqual(by["test bare"]["radius"], 0.0)

    def test_apply_then_undo(self):
        # the plan in process (the tool's own GATES are the real ones)
        c = sqlite3.connect(self.db, isolation_level=None)
        planned, _ = sg.plan(c, self.ground)
        before = dump(self.db)
        wrote, kept, gone = sg.apply(c, planned, when="2026-10-26T21:24:04+00:00")
        self.assertEqual(sorted(wrote), ["Test Circle", "Test Escape", "Test Gate", "Test Portal"])
        self.assertEqual((kept, gone), (["Kept Gate"], []))
        r = rows(self.db)
        self.assertEqual(r["kept gate"], ("self", "a GM's", 0, 1.0, 2.0, 3.0, 0.0, "gm"))
        self.assertEqual(r["test gate"][:7], ("self", "the Pyramid", 0, 1000.0, 81.5, 1000.0, 0.0))
        self.assertEqual(r["test gate"][7], "seed")
        first, first_rows, first_log = dump(self.db), rows(self.db), log(self.db)
        # a later run with nothing new writes nothing, and the log is as it was
        self.assertEqual(sg.apply(c, planned, when="2026-10-27T00:00:00+00:00"), ([], [n for n, *_ in PLAN], []))
        self.assertEqual(dump(self.db), first)
        # nor in the same second as the last run (the time is the run's key)
        with self.assertRaises(SystemExit):
            sg.apply(c, planned, when="2026-10-26T21:24:04+00:00")
        # a GM takes one out: a later run with two more gates writes those two
        # and leaves the one taken out alone
        c.execute("DELETE FROM spellgates WHERE spell = 'test portal'")
        sg.GATES = PLAN + LATER
        later, _ = sg.plan(c, self.ground)
        wrote, kept, gone = sg.apply(c, later, when="2026-11-01T15:00:00+00:00")
        self.assertEqual((sorted(wrote), gone), (["Test Bare", "Test Chamber"], ["Test Portal"]))
        self.assertEqual(sorted(kept), ["Kept Gate", "Test Circle", "Test Escape", "Test Gate"])
        self.assertEqual(rows(self.db)["test chamber"][:8], ("group", "the Chamber", 0, 6000.0, 50.0, 6000.0, 25.0, "seed"))
        # --undo takes the last run back alone: the rows and the log as the
        # first run left them, less the GM's take-out
        self.assertEqual(sg.undo(c), (2, [], "2026-11-01T15:00:00+00:00"))
        self.assertEqual(rows(self.db), {k: v for k, v in first_rows.items() if k != "test portal"})
        self.assertEqual(log(self.db), first_log)
        c.execute("INSERT INTO spellgates VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                  json.loads(c.execute("SELECT row FROM spell_gate_seed WHERE spell = 'test portal'").fetchone()[0]))
        # a GM moves one; --undo of the first run keeps it and deletes the rest
        c.execute("UPDATE spellgates SET x = 1010 WHERE spell = 'test gate'")
        self.assertEqual(sg.undo(c), (3, ["test gate"], "2026-10-26T21:24:04+00:00"))
        self.assertEqual(sorted(rows(self.db)), ["kept gate", "test gate"])
        c.execute("DELETE FROM spellgates WHERE spell = 'test gate'")
        c.close()
        self.assertEqual(dump(self.db), before)

    def test_undo_drops_a_table_it_made(self):
        # a database the new server never opened: no spellgates table
        c = sqlite3.connect(self.db, isolation_level=None)
        c.execute("DROP TABLE spellgates")
        before = dump(self.db)
        planned, _ = sg.plan(c, self.ground)
        wrote, kept, gone = sg.apply(c, planned, when="2026-10-26T21:24:04+00:00")
        self.assertEqual((len(wrote), kept, gone), (5, [], []))
        # a later run's rows go back first; the table only with the first run's
        sg.GATES = LATER
        wrote, _, _ = sg.apply(c, sg.plan(c, self.ground)[0], when="2026-11-01T15:00:00+00:00")
        self.assertEqual(len(wrote), 2)
        self.assertEqual(sg.undo(c)[:2], (2, []))
        self.assertEqual(sg.undo(c)[:2], (5, []))
        c.close()
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
