#!/usr/bin/env python3
"""tools/seed_npc_charm.py against a made-up world: the pages' uncharmable names and resists and the charm kinds'
names get their npccharm rows (whole names only: "rotwood the hollow" is no hollow), the templates their classtemplates
rows with abilities the server takes (class -1, the kinds, free where the page says so); a GM's row (npccharm or
classtemplates) is left as it is; a second --apply adds nothing; --undo takes out exactly what was written, a row a GM
changed since kept and the template it names with it, and drops the log; npccharm is made when missing. No game data.

    python3 tools/seed_npc_charm_test.py
"""
import importlib.util
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_npc_charm.py")
sys.path.insert(0, HERE)
_spec = importlib.util.spec_from_file_location("seed_npc_charm", TOOL)
M = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(M)

CLASSTEMPLATES = """CREATE TABLE classtemplates (
    name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '',
    special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0, special_fx INTEGER NOT NULL DEFAULT 0,
    abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    special_every REAL NOT NULL DEFAULT 0, special_first REAL NOT NULL DEFAULT 0, special_jitter REAL NOT NULL DEFAULT 0,
    special_from INTEGER NOT NULL DEFAULT 0)"""

NPCS = [("Hollow", 41), ("hollow", 44), ("Rotwood the Hollow", 61), ("Lady Vox", 57), ("Parathior", 52), ("Werehunter", 37),
        ("raging rhinoceros", 50), ("Ambassador Vu'Malik", 45), ("kappa drudge", 42), ("shadow wisp", 43),
        ("Nasehir Wizard", 40), ("plague griffon", 48), ("sand crab", 30), ("Treant", 45)]


def world(path, gm_template=False, gm_charm=False, with_npccharm=True):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_level INTEGER)")
    db.executemany("INSERT INTO npcs (npc_name, npc_level) VALUES (?, ?)", NPCS)
    db.execute(CLASSTEMPLATES)
    if with_npccharm:
        db.execute(M.NPCCHARM_SCHEMA)
    if gm_template:
        db.execute("INSERT INTO classtemplates (name, abilities, source) VALUES ('charm tree', '[]', 'gm')")
    if gm_charm:
        db.execute("INSERT INTO npccharm (name, charmable, source) VALUES ('lady vox', 1, 'gm')")
    db.commit()
    db.close()


def run(path, *args):
    r = subprocess.run([sys.executable, "-I", TOOL, "--db", path, "--no-backup", *args], capture_output=True, text=True)
    if r.returncode != 0:
        raise AssertionError(r.stderr + r.stdout)
    return r.stdout


def rows(path, tbl):
    db = sqlite3.connect(path)
    cur = db.execute(f"SELECT * FROM {tbl} ORDER BY name")
    cols = [d[0] for d in cur.description]
    out = {r[0]: dict(zip(cols, r)) for r in cur.fetchall()}
    db.close()
    return out


def dump(path):
    db = sqlite3.connect(path)
    out = {t: sorted(db.execute(f"SELECT * FROM {t}").fetchall()) for t, in
           db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")}
    db.close()
    return out


class SeedCharm(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_rows(self):
        world(self.db)
        before = dump(self.db)
        out = run(self.db)
        self.assertIn("report only", out)
        self.assertEqual(dump(self.db), before, "a report wrote")
        run(self.db, "--apply")
        charm = rows(self.db, "npccharm")
        self.assertEqual(charm["lady vox"]["charmable"], 0)
        self.assertEqual(charm["parathior"]["charmable"], 0)
        self.assertEqual(charm["werehunter"]["charmable"], 0)
        self.assertEqual(charm["raging rhinoceros"]["charmable"], 0)
        self.assertEqual((charm["ambassador vu'malik"]["charmable"], charm["ambassador vu'malik"]["resist_pct"]), (1, 100))
        self.assertEqual(charm["kappa drudge"]["resist_pct"], 40)
        self.assertEqual(charm["hollow"]["template"], "charm tree")
        self.assertEqual(charm["treant"]["template"], "charm tree")
        self.assertEqual(charm["shadow wisp"]["template"], "charm wisp")
        self.assertEqual(charm["nasehir wizard"]["template"], "charm nasehir wizard")
        self.assertEqual(charm["plague griffon"]["template"], "charm griffon")
        self.assertNotIn("rotwood the hollow", charm, "a name's whole word only")
        self.assertNotIn("sand crab", charm)
        self.assertTrue(all(r["source"] == M.SOURCE for r in charm.values()))
        tm = rows(self.db, "classtemplates")
        for name in ("charm tree", "charm wisp", "charm nasehir wizard", "charm griffon"):
            self.assertIn(name, tm)
            self.assertEqual(tm[name]["class"], -1)
        self.assertNotIn("charm rhino", tm, "no NPC of its kinds: no template")
        tree = json.loads(tm["charm tree"]["abilities"])
        self.assertEqual([a["kind"] for a in tree], ["buff", "buff", "nuke", "nuke", "heal"])
        self.assertTrue(tree[0]["free"] and tree[0]["stat"] == "ds")
        wiz = json.loads(tm["charm nasehir wizard"]["abilities"])
        self.assertTrue(all(a.get("free") for a in wiz), "the Nasehir wizards never run out of power")
        self.assertFalse(any(a.get("free") for a in json.loads(tm["charm wisp"]["abilities"])))
        self.assertGreater(tm["charm griffon"]["special_damage"], 0)
        # every ability within the server's bounds (database.CheckTemplateAbilities)
        for name, r in tm.items():
            ab = json.loads(r["abilities"])
            self.assertLessEqual(len(ab), 8, name)
            for i, a in enumerate(ab):
                self.assertIn(a["kind"], ("nuke", "aoe", "lifetap", "heal", "taunt", "buff", "root"), name)
                self.assertTrue(a.get("chain") or 2 <= a["every"] <= 3600, (name, a))
                self.assertFalse(a.get("chain") and i == 0, name)
        # a second --apply adds nothing
        again = dump(self.db)
        out = run(self.db, "--apply")
        self.assertIn("0 rows to add", out)
        self.assertEqual(dump(self.db), again)

    def test_gm_rows_kept(self):
        world(self.db, gm_template=True, gm_charm=True)
        run(self.db, "--apply")
        self.assertEqual(rows(self.db, "npccharm")["lady vox"]["charmable"], 1, "a GM's row was overwritten")
        self.assertEqual(rows(self.db, "classtemplates")["charm tree"]["source"], "gm")
        self.assertEqual(rows(self.db, "npccharm")["hollow"]["template"], "charm tree", "its names take the GM's template")
        run(self.db, "--undo")
        self.assertEqual(rows(self.db, "classtemplates")["charm tree"]["source"], "gm", "undo took a GM's row")
        self.assertEqual(rows(self.db, "npccharm")["lady vox"]["source"], "gm")

    def test_undo(self):
        world(self.db, with_npccharm=False)
        db = sqlite3.connect(self.db)
        db.execute(M.NPCCHARM_SCHEMA)  # what --undo leaves: the table, empty (as the server makes it)
        db.commit()
        db.close()
        before = dump(self.db)
        world2 = os.path.join(self.dir.name, "fresh.db")
        world(world2, with_npccharm=False)
        run(world2, "--apply")
        self.assertIn("npccharm", dump(world2), "npccharm made when missing")
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npccharm SET resist_pct = 7, source = 'gm' WHERE name = 'kappa drudge'")
        db.execute("UPDATE npccharm SET note = 'a GM likes it' WHERE name = 'shadow wisp'")
        db.commit()
        db.close()
        out = run(self.db, "--undo")
        self.assertIn("changed since", out)
        left = rows(self.db, "npccharm")
        self.assertEqual(set(left), {"kappa drudge", "shadow wisp"}, "only the rows a GM changed are left")
        self.assertIn("charm wisp", rows(self.db, "classtemplates"), "a template a kept row names stays")
        self.assertNotIn("charm tree", rows(self.db, "classtemplates"))
        self.assertNotIn(M.LOG, dump(self.db), "the log dropped")
        # with the GM's rows taken out too, exactly as before
        db = sqlite3.connect(self.db)
        db.execute("DELETE FROM npccharm")
        db.execute("DELETE FROM classtemplates WHERE name = 'charm wisp'")
        db.commit()
        db.close()
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
