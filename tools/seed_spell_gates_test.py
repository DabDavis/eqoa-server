#!/usr/bin/env python3
"""tools/seed_spell_gates.py against a made-up database and a made-up ground
(flat squares in the server's ground format): a pyramid's top written as it
is, a place on the terrain at the terrain's height, beside an NPC in front of
it, a zone's middle moved off a pinnacle; --apply leaves a GM's row alone,
refuses a second run, --undo deletes what it wrote but a row a GM changed
since, and the table when the run made it; no ground, nothing written. No
game data: places and names made up.

    python3 tools/seed_spell_gates_test.py
"""
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
CREATE TABLE spellgates (spell TEXT PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'self', place TEXT NOT NULL DEFAULT '',
                         world INTEGER NOT NULL DEFAULT 0, x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0,
                         z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
                         source TEXT NOT NULL DEFAULT 'gm');
"""

# a made-up plan: a pyramid, a stone, an NPC, a zone's middle (on a
# pinnacle), and a spell a GM placed already
PLAN = [
    ("Test Gate", sg.SELF, 0, "the Pyramid", ("top", 0, 1000.0, 81.5, 1000.0, "a pyramid"), "Self, Teleport to the Pyramid"),
    ("Test Portal", sg.GROUP, 15, "the Stone", sg.stone(0, 3000.0, 47.0, 3000.0, "a stone"), "Group, Teleport to the Stone"),
    ("Test Circle", sg.GROUP, 0, "the Hermit", sg.beside("Hermit", 2, "by the hermit"), "Group, Teleport to the Hermit"),
    ("Test Escape", sg.SELF, 0, "the Middle", sg.middle(0, 4000, 4000, "C", "Middle"), "Self, Teleport to the Middle"),
    ("Kept Gate", sg.SELF, 0, "a GM's", sg.stone(0, 3000.0, 50.0, 3000.0, "a stone"), "Self"),
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
    c.executemany("INSERT INTO spellpattern (spellname) VALUES (?)", [(n,) for n, *_ in PLAN])
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
    # pinnacle (a 20-wide top at 120) on a plain at 50
    write_ground(os.path.join(ground, "0.ground"), [
        square(1000, 1000, 100, 50), square(3000, 3000, 100, 47), square(5000, 5000, 400, 50), square(5000, 5000, 10, 120)])
    write_ground(os.path.join(ground, "2.ground"), [square(500, 500, 100, 50)])
    return db, ground


def rows(db):
    c = sqlite3.connect(db)
    out = {r[0]: r[1:] for r in c.execute("SELECT spell, kind, place, world, x, y, z, radius, source FROM spellgates")}
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

    def test_apply_then_undo(self):
        # the plan in process (the tool's own GATES are the real 27)
        c = sqlite3.connect(self.db, isolation_level=None)
        planned, _ = sg.plan(c, self.ground)
        before = dump(self.db)
        wrote, kept = sg.apply(c, planned)
        self.assertEqual(sorted(wrote), ["Test Circle", "Test Escape", "Test Gate", "Test Portal"])
        self.assertEqual(kept, ["Kept Gate"])
        r = rows(self.db)
        self.assertEqual(r["kept gate"], ("self", "a GM's", 0, 1.0, 2.0, 3.0, 0.0, "gm"))
        self.assertEqual(r["test gate"][:7], ("self", "the Pyramid", 0, 1000.0, 81.5, 1000.0, 0.0))
        self.assertEqual(r["test gate"][7], "seed")
        with self.assertRaises(SystemExit):
            sg.apply(c, planned)
        # a GM moves one; --undo keeps it and deletes the rest
        c.execute("UPDATE spellgates SET x = 1010 WHERE spell = 'test gate'")
        deleted, changed = sg.undo(c)
        self.assertEqual((deleted, changed), (3, ["test gate"]))
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
        wrote, kept = sg.apply(c, planned)
        self.assertEqual((len(wrote), kept), (5, []))
        self.assertEqual(sg.undo(c), (5, []))
        c.close()
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
