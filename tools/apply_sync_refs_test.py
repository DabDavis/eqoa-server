#!/usr/bin/env python3
"""apply_sync.py's reference checks (2026-11-27, Heirloom petition 3352), on the pair of tool/shard_refs_20261127_test.py:
  - the fixed planner's plan: --check refuses while the plan lists an untranslatable row (named); with
    --leave-unresolved it checks, applies (Heirloom's own #163912 / #190359 / slot 237) and --check-refs is clean after;
  - the old planner's plan (git ea028d0f: Live's ids verbatim): --check and --apply refuse, naming each dangling
    reference, and nothing is written;
  - a translated reference whose row is not the one it came from refuses (c); a delete of a row another row names (an
    npcpets template) refuses (b);
  - a minted id a GM took since the plan moves, and its "#<id>" with it, while an item list holding the same number is
    left (key-aware remap).
  python3 -E -s apply_sync_refs_test.py
"""
import copy
import importlib.util
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = HERE
APPLY = os.path.join(HERE, "apply_sync.py")
spec = importlib.util.spec_from_file_location("refs_test", os.path.join(TOOL, "shard_refs_20261127_test.py"))
rt = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rt)

TABLES = ("npcs", "gmtriggers", "queststeps", "quests", "npcpets", "itempattern")


def run(*args):
    r = subprocess.run([sys.executable, "-E", "-s", APPLY, *args], capture_output=True, text=True)
    return r.returncode, r.stdout + r.stderr


def snap(path):
    db = sqlite3.connect(path)
    try:
        return {t: sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr) for t in TABLES}
    finally:
        db.close()


class ApplyRefs(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.d = tempfile.mkdtemp(prefix="applyrefs1127-", dir=os.environ.get("TMPDIR"))
        os.makedirs(os.path.join(cls.d, "src"))
        h, l = rt.make_pair(os.path.join(cls.d, "src"))
        cls.src = (h, l)
        cls.new = os.path.join(cls.d, "new.json")
        rt.plan_with(TOOL, h, l, cls.new)
        old = rt.old_tools(os.path.join(cls.d, "old"))
        cls.old = None
        if old:
            cls.old = os.path.join(cls.d, "old.json")
            rt.plan_with(old, h, l, cls.old)

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.d, ignore_errors=True)

    def fresh(self):
        d = tempfile.mkdtemp(dir=self.d)
        p = os.path.join(d, "h.db")
        shutil.copy(self.src[0], p)
        return p

    def plan(self, edit=None):
        with open(self.new) as f:
            p = json.load(f)
        if edit:
            edit(p)
        path = os.path.join(tempfile.mkdtemp(dir=self.d), "p.json")
        with open(path, "w") as f:
            json.dump(p, f)
        return path

    def test_fixed_plan_applies_and_checks_clean(self):
        h = self.fresh()
        before = snap(h)
        rc, out = run("--db", h, "--shard", "heirloom", "--plan", self.new, "--check", "--rehearsal")
        self.assertNotEqual(rc, 0)
        self.assertIn("UNRESOLVED (not in the plan) gmtriggers", out)
        self.assertIn("npc 162999", out)
        self.assertIn("--leave-unresolved", out)
        rc, out = run("--db", h, "--shard", "heirloom", "--plan", self.new, "--check", "--rehearsal", "--leave-unresolved")
        self.assertEqual(rc, 0, out)
        self.assertIn("references resolve", out)
        self.assertEqual(snap(h), before)
        rc, out = run("--db", h, "--shard", "heirloom", "--plan", self.new, "--apply", "--rehearsal", "--leave-unresolved")
        self.assertEqual(rc, 0, out)
        self.assertIn("references checked", out)
        db = sqlite3.connect(h)
        s = json.loads(db.execute("SELECT steps FROM gmtriggers WHERE slot = 161").fetchone()[0])
        ev = json.loads(db.execute("SELECT events FROM queststeps WHERE questid = 100790 AND step = 8").fetchone()[0])
        db.close()
        self.assertEqual((s[1]["text"], s[2]["npc"], s[4]["slot"]), ("Now guard me while I brew!", "#163912", 237))
        self.assertEqual((ev[0]["npc"], ev[0]["count"]), ("#190359", 6))
        rc, out = run("--check-refs", "--db", h)
        self.assertEqual(rc, 0, out)
        self.assertIn("REFS OK", out)

    def test_old_plan_refuses_and_writes_nothing(self):
        if not self.old:
            self.skipTest("git cannot show the old tools")
        h = self.fresh()
        before = snap(h)
        for mode in ("--check", "--apply"):
            rc, out = run("--db", h, "--shard", "heirloom", "--plan", self.old, mode, "--rehearsal")
            self.assertNotEqual(rc, 0, out)
            self.assertIn("REFUSED (nothing written)", out)
            self.assertIn('(a) gmtriggers {"slot":161} steps:[2].npc: npc 162757 names no npcs row', out)
            self.assertIn('(a) queststeps {"questid":100790,"step":8} events:[0].npc: npc 191858', out)
            self.assertIn("npc 162999", out)          # the Lost Spawn, copied verbatim by the old planner
            self.assertEqual(snap(h), before)
        # and what the old plan would have left, had nothing checked it, is what --check-refs reports
        rc, out = run("--check-refs", "--db", h)
        self.assertEqual(rc, 0, out)

    def test_wrong_row_refuses(self):
        def edit(p):
            p["H"]["refs"]["npc"]["163912"]["name"] = "froglok guardsman"
        h = self.fresh()
        before = snap(h)
        rc, out = run("--db", h, "--shard", "heirloom", "--plan", self.plan(edit), "--apply", "--rehearsal", "--leave-unresolved")
        self.assertNotEqual(rc, 0, out)
        self.assertIn("(c) gmtriggers {\"slot\":161} steps:[2].npc: npcs 163912 is 'froglok watchman'", out)
        self.assertEqual(snap(h), before)

    def test_delete_of_a_named_row_refuses(self):
        h = self.fresh()
        db = sqlite3.connect(h)
        db.execute("INSERT INTO npcpets VALUES ('rustel shaman', 'rustel minotaur', 190359, 1, 0, '', 'gm')")
        row = db.execute("SELECT * FROM npcs WHERE id = 190359").fetchone()
        cols = [d[1] for d in db.execute("PRAGMA table_info(npcs)")]
        db.commit()
        db.close()

        def edit(p):
            p["H"]["ops"].append({"t": "npcs", "pk": {"id": 190359}, "before": dict(zip(cols, row)), "after": None,
                                  "why": "a delete from the other shard", "unit": None})
            # (and the quest step that spawns it is left out of this plan)
            p["H"]["ops"] = [o for o in p["H"]["ops"] if o["t"] != "queststeps"]
        before = snap(h)
        for mode in ("--check", "--apply"):
            rc, out = run("--db", h, "--shard", "heirloom", "--plan", self.plan(edit), mode, "--rehearsal", "--leave-unresolved")
            self.assertNotEqual(rc, 0, out)
            self.assertIn('(b) npcpets {"owner":"rustel shaman","template_row":190359} template_row: npc 190359', out)
            self.assertIn("(b) queststeps", out)       # the step's events, as Heirloom holds them, spawn it too
            self.assertEqual(snap(h), before)

    def test_remap_moves_the_reference_by_key(self):
        h = self.fresh()
        db = sqlite3.connect(h)
        db.execute("INSERT INTO itempattern VALUES (200000, 'a number like the id', -1, 0, 1, '')")
        cols = [d[1] for d in db.execute("PRAGMA table_info(npcs)")]
        db.commit()
        db.close()
        new_npc = dict(zip(cols, [200000, "froglok bouncer", 0, 24726.0, 10.0, 29310.5, 0, 20, 337378828, None, 100, 0]))

        def edit(p):
            p["H"]["ops"].insert(0, {"t": "npcs", "pk": {"id": 200000}, "before": None, "after": new_npc, "why": "a new template",
                                     "unit": None})
            for o in p["H"]["ops"]:
                if o["t"] == "gmtriggers" and o["pk"].get("slot") == 161:
                    s = json.loads(o["after"]["steps"])
                    s.append({"do": "spawn", "npc": "#200000"})
                    s.append({"do": "give", "items": "200000*1"})
                    o["after"]["steps"] = json.dumps(s, separators=(",", ":"))
            p["H"]["refs"]["npc"]["200000"] = {"src": 199000, "name": "froglok bouncer", "world": 0, "x": 24726.0,
                                               "z": 29310.5, "modelid": 337378828}
        plan = self.plan(edit)
        # a GM places another NPC, which takes the id the plan chose
        db = sqlite3.connect(h)
        db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, modelid, hp_percent, npc_id) "
                   "VALUES (200000, 'a GM''s rat', 0, 1, 1, 1, 0, 1, 7, 100, 0)")
        db.commit()
        db.close()
        rc, out = run("--db", h, "--shard", "heirloom", "--plan", plan, "--apply", "--rehearsal", "--leave-unresolved")
        self.assertEqual(rc, 0, out)
        self.assertIn("REMAP npcs 200000 -> 200001", out)
        db = sqlite3.connect(h)
        s = json.loads(db.execute("SELECT steps FROM gmtriggers WHERE slot = 161").fetchone()[0])
        self.assertEqual(db.execute("SELECT npc_name FROM npcs WHERE id = 200001").fetchone()[0], "froglok bouncer")
        db.close()
        self.assertEqual(s[-2], {"do": "spawn", "npc": "#200001"})
        self.assertEqual(s[-1], {"do": "give", "items": "200000*1"})    # an item, not the NPC: left
        rc, out = run("--check-refs", "--db", h)
        self.assertEqual(rc, 0, out)


if __name__ == "__main__":
    unittest.main()
