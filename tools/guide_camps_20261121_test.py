#!/usr/bin/env python3
"""Tests of tools/guide_camps_20261121.py. Without a database: the templates' abilities are kinds the server knows
(fixed figures only on a nuke or a lifetap, as entity/classtemplate.go takes them), the link factions carry no
guard's word (entity/guards.go would make the camp a town's guards), every new place's height is one the ground
gave, the names the templates are given point at templates of this tool or the roamer. With EQOA_REAL_DB (a copy of
a server's database; never the running one's file): the tool's report, --apply, a second --apply refused, --undo
putting every table but gm_audit back as it was, and a second --apply equal to the first."""
import hashlib
import io
import os
import re
import shutil
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import guide_camps_20261121 as gc  # noqa: E402

KINDS = {"nuke", "aoe", "lifetap", "heal", "debuff", "dot", "rampage", "summon", "healshield"}
GUARD_WORDS = re.compile(r"(?i)\b(guard|guards|militia|watch|defenders|wardens|warden)\b")


class TestWithoutADatabase(unittest.TestCase):
    def test_templates(self):
        for name, (cls, abilities, over, note) in gc.TEMPLATES.items():
            self.assertTrue(name.startswith("guide camp"), name)
            self.assertIn(cls, range(15))
            self.assertLessEqual(len(abilities), 12)
            self.assertLessEqual(sum(1 for a in abilities if a["kind"] == "rampage"), 1, name)
            for a in abilities:
                self.assertIn(a["kind"], KINDS, name)
                if "damage" in a:
                    self.assertIn(a["kind"], ("nuke", "lifetap"), name)
                if a["kind"] == "heal":
                    self.assertEqual(a.get("every"), 3600, f"{name}: the guide's heals are once a fight")
                    self.assertEqual(a.get("below"), 50, name)
            self.assertTrue(set(over) <= {"shield_damage"}, name)

    def test_names_point_at_templates(self):
        known = set(gc.TEMPLATES) | {gc.ROAMER}
        for camp, name, t in gc.BY_NAME:
            self.assertIn(t, known)
            self.assertEqual(name, name.lower())
        for r in gc.RARES:
            self.assertIn(r["template"], known)

    def test_links(self):
        for camp, name, faction in gc.LINKS:
            self.assertFalse(GUARD_WORDS.search(faction), faction)

    def test_places(self):
        for camp in (gc.RATMEN, gc.TFG):
            for k, y in camp["y"].items():
                x, z = map(float, k.split(","))
                self.assertTrue(40 < y < 75, (k, y))
        self.assertEqual(len(gc.RATMEN["y"]), 23)
        # both shards' Permafrost camps, 27 a shard: 52 places (two stand where they stand on both)
        self.assertEqual(len(gc.TFG["y"]), 52)
        for (w, sx, sz), (x, y, z) in gc.SAGE_PETS:
            self.assertLess(((x - sx) ** 2 + (z - sz) ** 2) ** 0.5, 4.0, "within companionReach of its sage")

    def test_detail(self):
        self.assertEqual(gc.detail("MOOTS"), "the owner 2026-11-21: camp audit vs MasterVG782's guide: MOOTS")


def table_hashes(path):
    db = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'"):
        if t in ("gm_audit", "gm_audit_facets", "sqlite_sequence"):
            continue
        h = hashlib.sha256()
        for r in db.execute(f'SELECT * FROM "{t}" ORDER BY 1'):
            h.update(repr(r).encode())
        out[t] = h.hexdigest()
    db.close()
    return out


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB names a copy of a server's database")
class TestOnACopy(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        shutil.copyfile(os.environ["EQOA_REAL_DB"], self.db)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def run_tool(self, *args):
        buf = io.StringIO()
        with redirect_stdout(buf):
            rc = gc.main(["--db", self.db, *args])
        return rc, buf.getvalue()

    def test_apply_again_undo(self):
        before = table_hashes(self.db)
        rc, out = self.run_tool()
        self.assertEqual(rc, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(table_hashes(self.db), before, "a report writes nothing")
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        first = table_hashes(self.db)
        db = sqlite3.connect(self.db)
        n = db.execute(f"SELECT count(*) FROM {gc.LOG}").fetchone()[0]
        audit = db.execute("SELECT count(*) FROM gm_audit WHERE detail LIKE 'the owner 2026-11-21: camp audit%' "
                           "AND accountid = 1 AND username = 'dab' AND via = 'tool'").fetchone()[0]
        self.assertEqual(n, audit, "one gm_audit line a row")
        self.assertTrue(db.execute("SELECT 1 FROM npcs WHERE npc_name = 'Froglok Invader'").fetchone())
        self.assertFalse(db.execute("SELECT 1 FROM npcs WHERE npc_name = 'Artic Spiritcaller' AND NOT EXISTS "
                                    "(SELECT 1 FROM npcs WHERE id = 122923)").fetchone(), "Live's spelling mended")
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertIn("applied already", out)
        self.assertEqual(table_hashes(self.db), first)
        rc, out = self.run_tool("--undo")
        self.assertIn(" 0 left", out)
        self.assertEqual(table_hashes(self.db), before, "undo puts every table back")
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        again = table_hashes(self.db)
        # factionhits' ids are its AUTOINCREMENT's, new ones each time (and so the log's keys); the rest the same
        changed = {t for t in again if again[t] != first.get(t)} - {gc.LOG}
        self.assertEqual(changed, {"factionhits"}, "applied again, the same")

    def test_skip_ratmen(self):
        rc, out = self.run_tool("--skip", "ratmen")
        self.assertEqual(rc, 0, out)
        self.assertNotIn("RATMEN (41-50)", out)


if __name__ == "__main__":
    unittest.main()
