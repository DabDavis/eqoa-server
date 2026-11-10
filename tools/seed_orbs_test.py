#!/usr/bin/env python3
"""tools/seed_orbs.py against a made-up data file, a made-up ground (flat
squares in the server's ground format) and a made-up database: each orb sends
to beside its pair, on the floor there, facing away from the pair's banners,
never inside an orb; --sets leaves a set out; a landing with no floor leaves
its pair out, both orbs; --apply leaves a GM's row of the name alone; --undo
takes the last run back exactly (the whole database as it was, with the table
the server made and without one), a row a GM changed since kept; no ground,
nothing written; data whose pairs do not point back refused. And the real data
file (tools/orbs/highbourne.json) is whole: its pairs point back, share a
banner, and its banner rule agrees with the disc's own turn of the three pads
that are turned. Places and names made up but for that file.

    python3 tools/seed_orbs_test.py
"""
import json
import math
import os
import sqlite3
import struct
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_orbs.py")
sys.path.insert(0, HERE)
import seed_orbs as so  # noqa: E402

# a tower's pad on a floor at 120 with two banners east of it (the room opens
# west), its hall pad at 60 turned north-west with one banner behind it; and a
# cave pair at 50, one of whose landings (east of the east pad) has no floor
DATA = {
    "world": 0, "file": "TEST.ESF", "zone": 7, "pad": "00000001",
    "orbs": [
        {"key": "tower", "set": "towers", "name": "test tower orb", "room": 1, "actor": 0,
         "x": 1000.0, "y": 120.0, "z": 1000.0, "rot": 0, "banner": "0000000A",
         "banners": [[1002.0, 998.0], [1002.0, 1002.0]], "pair": "hall"},
        {"key": "hall", "set": "towers", "name": "test hall orb", "room": 2, "actor": 0,
         "x": 2000.0, "y": 60.0, "z": 2000.0, "rot": -0.785398, "banner": "0000000A",
         "banners": [[2001.0, 1999.0]], "pair": "tower"},
        {"key": "west", "set": "caves", "name": "test west cave orb", "room": 3, "actor": 0,
         "x": 3000.0, "y": 50.0, "z": 3000.0, "rot": 0, "banner": "0000000B",
         "banners": [[3004.0, 3000.0]], "pair": "east"},
        {"key": "east", "set": "caves", "name": "test east cave orb", "room": 4, "actor": 0,
         "x": 3500.0, "y": 50.0, "z": 3000.0, "rot": 0, "banner": "0000000B",
         "banners": [[3496.0, 3000.0]], "pair": "west"},
    ],
}

SERVER_SCHEMA = so.SCHEMA + """
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
"""


def square(cx, cz, half, y):
    """A flat square of terrain round (cx, cz)."""
    return (cx - half, cz - half, cx + half, cz + half, y,
            [(cx - half, y, cz - half), (cx + half, y, cz - half), (cx - half, y, cz + half), (cx + half, y, cz + half)])


def write_ground(path, zones):
    """internal/ground's format (tools/seed_spell_gates_test.py's writer)."""
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


def dump(db):
    c = sqlite3.connect(db)
    out = "\n".join(c.iterdump())
    c.close()
    return out


def rows(db):
    c = sqlite3.connect(db)
    out = {r[0]: r[1:] for r in c.execute("SELECT name, world, x, y, z, radius, to_world, to_x, to_y, to_z, to_facing, source FROM orbs")}
    c.close()
    return out


class SeedOrbs(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        d = self.dir.name
        self.data = os.path.join(d, "test.json")
        with open(self.data, "w") as f:
            json.dump(DATA, f)
        self.ground = os.path.join(d, "ground")
        os.mkdir(self.ground)
        # the tower's floor at 120 (and the floor under it at 101: another
        # room), the hall's at 60, the west cave's at 50; the east cave's
        # floor stops 2 east of its pad, where its landing would be is a floor
        # 3 lower (a step down off the room's floor: not the room)
        write_ground(os.path.join(self.ground, "0.ground"), [
            square(1000, 1000, 20, 120), square(1000, 1000, 20, 101), square(2000, 2000, 20, 60),
            square(3000, 3000, 20, 50), (3480, 2980, 3502, 3020, 50, [(3480, 50, 2980), (3502, 50, 2980), (3480, 50, 3020), (3502, 50, 3020)]),
            square(3511, 3000, 8.5, 47)])
        self.db = os.path.join(d, "eqoa.db")
        c = sqlite3.connect(self.db)
        c.executescript(SERVER_SCHEMA)
        c.execute("INSERT INTO npcs VALUES (1, 'somebody')")
        c.commit()
        c.close()

    def tearDown(self):
        self.dir.cleanup()

    def plan(self, sets=None):
        orbs, by = so.read_data([self.data])
        return so.plan(orbs, by, self.ground, sets)

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-I", TOOL, "--db", self.db, "--no-backup", "--data", self.data, *args],
                              capture_output=True, text=True)

    def test_each_sends_beside_its_pair(self):
        planned, missing = self.plan({"towers"})
        self.assertEqual(missing, [])
        got = {r["_key"]: r for r in planned}
        self.assertEqual(sorted(got), ["hall", "tower"])
        # the tower's orb lands the player beside the hall's pad, on the hall's
        # side away from its banner (north-west of a banner to its south-east),
        # facing that way
        t = got["tower"]
        self.assertAlmostEqual(math.hypot(t["to_x"] - 2000, t["to_z"] - 2000), so.LAND_OFF, places=2)
        self.assertAlmostEqual(t["to_y"], 60, places=2)
        h = math.atan2(2000 - 2001, 2000 - 1999)
        self.assertAlmostEqual(t["to_facing"], round(h, 4), places=4)
        self.assertAlmostEqual(t["to_x"], 2000 + so.LAND_OFF * math.sin(h), places=2)
        self.assertEqual((t["x"], t["y"], t["z"], t["radius"], t["source"]), (1000.0, 120.0, 1000.0, 0.0, so.SOURCE))
        # the hall's orb lands it west of the tower's pad (its banners east),
        # on the tower's floor at 120, not the floor 19 under it
        hl = got["hall"]
        self.assertAlmostEqual(hl["to_x"], 1000 - so.LAND_OFF, places=2)
        self.assertAlmostEqual(hl["to_z"], 1000, places=2)
        self.assertAlmostEqual(hl["to_y"], 120, places=2)
        self.assertAlmostEqual(hl["to_facing"], round(-math.pi / 2, 4), places=4)
        for r in planned:
            for o in DATA["orbs"]:
                self.assertFalse(so.holds(dict(o), r["to_x"], r["to_y"], r["to_z"]), f"{r['_key']} lands inside {o['key']}")

    def test_a_landing_with_no_floor_leaves_its_pair_out(self):
        planned, missing = self.plan()
        self.assertEqual(sorted(r["_key"] for r in planned), ["hall", "tower"])
        self.assertEqual(sorted(m.split(":")[0] for m in missing), ["east", "west"], missing)

    def test_apply_then_undo_is_exact(self):
        before = dump(self.db)
        r = self.run_tool("--ground", self.ground, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        got = rows(self.db)
        self.assertEqual(sorted(got), ["test hall orb", "test tower orb"])
        self.assertNotEqual(dump(self.db), before)
        # a later run writes nothing more, and logs nothing: the undo after it
        # takes back the first run
        c = sqlite3.connect(self.db, isolation_level=None)
        planned, _ = self.plan()
        wrote, kept, gone = so.apply(c, planned, when="2999-01-01T00:00:00+00:00")
        self.assertEqual((wrote, sorted(kept), gone), ([], ["test hall orb", "test tower orb"], []))
        c.close()
        self.assertEqual(len(rows(self.db)), 2)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("deleted 2 rows", r.stdout)
        self.assertEqual(dump(self.db), before, "an apply and its undo left the database changed")

    def test_undo_drops_a_table_it_made(self):
        c = sqlite3.connect(self.db)
        c.execute("DROP TABLE orbs")
        c.commit()
        c.close()
        before = dump(self.db)
        self.assertEqual(self.run_tool("--ground", self.ground, "--apply").returncode, 0)
        self.assertEqual(len(rows(self.db)), 2)
        self.assertEqual(self.run_tool("--undo").returncode, 0)
        self.assertEqual(dump(self.db), before)

    def test_a_gms_rows_are_left_alone(self):
        c = sqlite3.connect(self.db)
        c.execute("INSERT INTO orbs (name, world, x, y, z, to_x, to_y, to_z) VALUES ('test hall orb', 0, 1, 2, 3, 9, 9, 9)")
        c.commit()
        c.close()
        r = self.run_tool("--ground", self.ground, "--apply")
        self.assertIn("left alone, there already: test hall orb", r.stdout)
        self.assertEqual(rows(self.db)["test hall orb"][1:4], (1.0, 2.0, 3.0))
        # a GM changes the seeded row: the undo keeps it, and says so
        c = sqlite3.connect(self.db)
        c.execute("UPDATE orbs SET to_facing = 1 WHERE name = 'test tower orb'")
        c.commit()
        c.close()
        r = self.run_tool("--undo")
        self.assertIn("deleted 0 rows; kept, changed since: test tower orb", r.stdout)
        self.assertEqual(sorted(rows(self.db)), ["test hall orb", "test tower orb"])

    def test_no_ground_writes_nothing(self):
        before = dump(self.db)
        r = self.run_tool("--ground", os.path.join(self.dir.name, "nowhere"), "--apply")
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(dump(self.db), before)

    def test_pairs_must_point_back(self):
        bad = json.loads(json.dumps(DATA))
        bad["orbs"][1]["pair"] = "west"
        with open(self.data, "w") as f:
            json.dump(bad, f)
        r = self.run_tool("--ground", self.ground)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("does not pair with it", r.stdout + r.stderr)

    def test_the_real_file_is_whole(self):
        orbs, by = so.read_data([os.path.join(so.DATA_DIR, "highbourne.json")])
        self.assertEqual(len(orbs), 8)
        self.assertEqual(sorted({o["set"] for o in orbs}), ["caves", "towers"])
        for o in orbs:
            p = by[o["pair"]]
            self.assertEqual(p["pair"], o["key"])
            self.assertEqual(p["banner"], o["banner"])
            # where the disc turned a pad, the banner rule's side agrees with
            # its turn (within 0.1): the room's side of it
            if o["rot"] != 0:
                d = (so.open_side(o) - o["rot"] + math.pi) % (2 * math.pi) - math.pi
                self.assertLess(abs(d), 0.1, f"{o['key']}: banners' side {so.open_side(o):.3f}, the pad's turn {o['rot']}")
            # no landing inside any orb of the file
            h = so.open_side(o)
            lx, lz = o["x"] + so.LAND_OFF * math.sin(h), o["z"] + so.LAND_OFF * math.cos(h)
            for q in orbs:
                self.assertFalse(so.holds(q, lx, o["y"], lz), f"{o['key']}'s landing inside {q['key']}")


if __name__ == "__main__":
    unittest.main()
