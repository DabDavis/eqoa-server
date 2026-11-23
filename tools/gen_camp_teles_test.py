#!/usr/bin/env python3
"""Tests of tools/gen_camp_teles.py's map camps. Without a database: CENSUS_CAMPS' keys, names and kinds are each
their own and none is a #tele camp's, census_camps anchors a camp on its kinds and leaves out one a placed camp of
its kind already is, keep_written keeps every written camp as it was, adds only new camps not of a written camp's
kind within SAME, and writes again one whose anchor's name is gone, and every line of mapcamps.go parses (so
--keep-written keeps them all). With EQOA_REAL_DB (a copy of a server's database; never the running one's file) and
EQOA_DATA (EQOA-Data): --map-out --keep-written over a copy of mapcamps.go writes it as it is in the repository."""
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import gen_camp_teles as g  # noqa: E402

MAPCAMPS = os.path.join(HERE, "..", "internal", "handlers", "mapcamps.go")
CAMPS = os.path.join(HERE, "..", "internal", "handlers", "camps.go")


def row(i, name, x, z, world=0, level=40, y=50.0):
    return (i, name, x, y, z, world, level)


# a fit that is the identity: the map's point is the game's
IDENTITY = {w: ((1.0, 0.0, 1.0, 0.0), 1, 1, 0.0) for w in (0, 1, 2)}
ZONES = {"zones": [{"name": "Test Hills", "x": 0, "y": 0, "biomes": ["x"]}], "pois": [], "towns": []}


class TestCensusList(unittest.TestCase):
    def test_each_its_own(self):
        keys = [c[0] for c in g.CENSUS_CAMPS]
        names = [c[1] for c in g.CENSUS_CAMPS]
        self.assertEqual(len(keys), len(set(keys)))
        self.assertEqual(len(names), len(set(names)))
        tele = set(g.tele_camps(CAMPS)) | {a for spec in g.CAMPS for a in spec[1]}
        for key, name, w, (x, z), pattern, reach in g.CENSUS_CAMPS:
            self.assertRegex(key, r"^[a-z0-9]+$")
            self.assertEqual(key, re.sub(r"[^a-z0-9]", "", name.lower()), key)
            self.assertNotIn(key, tele)
            self.assertIn(w, (0, 1, 2))
            self.assertTrue(re.match(r"^\^\(.+\)\$$", pattern), pattern)
            re.compile(pattern)
            self.assertTrue(60 <= reach <= 700, (key, reach))
            self.assertRegex(name, r"\(.+\)$")

    def test_the_shriek_camp_is_marked(self):
        # the owner, 2026-11-23: a camp of shrieks with no marker: the map's "Shriek" point, placed now its creatures stand
        written = g.written_map_camps(MAPCAMPS)
        near = [k for k, (_, w, x, z, an, cr) in written.items()
                if w == 0 and abs(x - 17601) < 60 and abs(z - 21116) < 60 and "shriek" in cr]
        self.assertTrue(near, "no map camp of shrieks at (17601, 21116)")


class TestCensusCamps(unittest.TestCase):
    def setUp(self):
        self.saved = g.CENSUS_CAMPS

    def tearDown(self):
        g.CENSUS_CAMPS = self.saved

    def test_anchored_on_its_kinds(self):
        g.CENSUS_CAMPS = [("shrieksteststone", "Shrieks (Test Stone)", 0, (1000.0, 1000.0), r"^(shriek)$", 100.0)]
        rows = [row(i, "shriek", 1000 + i, 1000, level=43) for i in range(6)] + [row(99, "griffon", 1001, 1001, level=50)]
        placed, left = g.census_camps(ZONES, rows, IDENTITY, [])
        self.assertEqual(left, [])
        self.assertEqual(len(placed), 1)
        p = placed[0]
        self.assertEqual((p["key"], p["npc"], p["levels"], p["creatures"], p["zone"]), ("shrieksteststone", "shriek", "43", ["shriek"], "Test Hills"))
        self.assertEqual(p["label"], "Shrieks (Test Stone), Test Hills")

    def test_a_placed_camp_of_its_kind_is_that_camp(self):
        g.CENSUS_CAMPS = [("shrieksteststone", "Shrieks (Test Stone)", 0, (1000.0, 1000.0), r"^(shriek)$", 100.0)]
        rows = [row(i, "shriek", 1000 + i, 1000) for i in range(6)]
        taken = [("shriek", 0, 1100.0, 1000.0, re.compile("^shriek$"), "shriek")]
        placed, left = g.census_camps(ZONES, rows, IDENTITY, taken)
        self.assertEqual(placed, [])
        self.assertIn("the camp shriek", left[0][1])

    def test_too_few(self):
        g.CENSUS_CAMPS = [("shrieksteststone", "Shrieks (Test Stone)", 0, (1000.0, 1000.0), r"^(shriek)$", 100.0)]
        placed, left = g.census_camps(ZONES, [row(1, "shriek", 1000, 1000), row(2, "shriek", 1500, 1000)], IDENTITY, [])
        self.assertEqual(placed, [])
        self.assertEqual(len(left), 1)


class TestKeepWritten(unittest.TestCase):
    def written(self):
        line = '\t"grizz": {Name: "Grizz", NPC: "grizz slayer", X: 100.0, Y: 1.0, Z: 100.0, World: 2, Creatures: []string{"grizz slayer", "grizz mage"}},'
        line2 = '\t"undead": {Name: "Undead", NPC: "skeleton", X: 5000.0, Y: 1.0, Z: 5000.0, World: 0, Creatures: []string{"skeleton"}},'
        d = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, d)
        path = os.path.join(d, "mapcamps.go")
        open(path, "w").write("package handlers\n\nvar mapCamps = map[string]mapCamp{\n" + line + "\n" + line2 + "\n}\n")
        return g.written_map_camps(path), line

    def placed(self, key, npc, x, z, world=2, creatures=None):
        return {"key": key, "npc": npc, "x": x, "z": z, "world": world, "creatures": creatures or [npc]}

    def test_keeps_and_adds(self):
        written, line = self.written()
        rows = [row(1, "grizz slayer", 110, 100, world=2), row(2, "skeleton", 5010, 5000)]
        placed = [self.placed("grizz", "grizz slayer", 400, 400),            # moved: kept as written
                  self.placed("grizznorth", "grizz slayer", 150, 150),       # the written grizz
                  self.placed("grizzsouth", "grizz warrior", 900, 900),      # new
                  self.placed("shriek", "shriek", 120, 120, world=0)]        # new
        lines, added, dup, redone = g.keep_written(written, placed, rows)
        self.assertEqual(lines["grizz"], line)
        self.assertEqual(sorted(p["key"] for p in added), ["grizzsouth", "shriek"])
        self.assertEqual([(p["key"], k) for p, k in dup], [("grizznorth", "grizz")])
        self.assertEqual(redone, [])
        self.assertIsNone(lines["shriek"])
        text = g.render_map([dict(p, name=p["key"], map="", zone="", label="", y=0, levels="", guide="", mx=0, my=0)
                             for p in placed], lines)
        self.assertIn(line, text)
        self.assertNotIn('"grizznorth"', text)

    def test_renamed_anchor_written_again(self):
        written, _ = self.written()
        rows = [row(1, "grizz slayer", 110, 100, world=2), row(2, "decaying skeleton", 5010, 5000)]
        p = self.placed("undead", "decaying skeleton", 5010, 5000, world=0)
        lines, added, dup, redone = g.keep_written(written, [p], rows)
        self.assertIsNone(lines["undead"])
        self.assertEqual(redone, [p])


class TestTheFile(unittest.TestCase):
    def test_every_line_parses(self):
        text = open(MAPCAMPS).read()
        entries = re.findall(r'^\t"\w+":', text, re.M)
        self.assertEqual(len(g.written_map_camps(MAPCAMPS)), len(entries))

    @unittest.skipUnless(os.environ.get("EQOA_REAL_DB") and os.environ.get("EQOA_DATA"), "EQOA_REAL_DB and EQOA_DATA")
    def test_regenerates_as_written(self):
        d = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, d)
        out = os.path.join(d, "mapcamps.go")
        shutil.copy(MAPCAMPS, out)
        subprocess.run([sys.executable, "-E", "-s", os.path.join(HERE, "gen_camp_teles.py"), "--db", os.environ["EQOA_REAL_DB"],
                        "--eqoa-data", os.environ["EQOA_DATA"], "--map-out", out, "--keep-written"],
                       check=True, capture_output=True)
        self.assertEqual(open(out).read(), open(MAPCAMPS).read())


if __name__ == "__main__":
    unittest.main()
