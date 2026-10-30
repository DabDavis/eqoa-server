#!/usr/bin/env python3
"""tools/seed_roamers.py against a made-up database: a hostile warrior creature and one of no class take the roamer
template (a generic one, a named one, a boss), a wisp the wisp template, and the townsperson in a player race's body,
the NPC-bit merchant, the caster of class 13, the training dummy and the name a GM gave a template are left (and
said); the roamer row is written with its clock, the wisp row as a copy of the wizard's with both abilities at 28%
(the wizard's left at 35%), a wisp row a GM made kept, the timed columns added when the table lacks them; a second
--apply adds nothing; --undo gives every table back and drops the log; the log's columns pass the snapshot's name
rules. No game data.

    python3 tools/seed_roamers_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_roamers.py")
sys.path.insert(0, HERE)
import seed_roamers as sr  # noqa: E402
from tune_balance_test import secret_like  # noqa: E402

HUMAN = 0x70D898C6
# jib's wizard's abilities as the server writes them (database.TemplateAbilitiesJSON of the seed)
WIZARD_ABILITIES = ('[{"kind":"aoe","every":15,"first":2,"damage_pct":35,"max_recast":10,"max_cast":5},'
                    '{"kind":"nuke","chain":true,"damage_pct":35,"max_recast":10,"max_cast":5}]')
CREATURE = 0x55432169
SCHEMA = f"""
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_level INTEGER, npc_type INTEGER, modelid INTEGER);
CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE charactermodel (sex INTEGER NOT NULL, modelid INTEGER NOT NULL PRIMARY KEY, race INTEGER NOT NULL);
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (npc_name, tablename));
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL DEFAULT 100,
    taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '',
    special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0, special_fx INTEGER NOT NULL DEFAULT 0,
    abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
INSERT INTO charactermodel VALUES (0, {HUMAN}, 0);
INSERT INTO classtemplates (name, class, hp_pct, abilities, note, source) VALUES ('wizard', 13, 85, '{WIZARD_ABILITIES}',
    'jib''s wizard', 'seed');
INSERT INTO npcs (npc_name, npc_level, npc_type, modelid) VALUES
    ('moss snake', 4, 0, {CREATURE}), ('moss snake', 6, 0, {CREATURE}),
    ('Deathfist reaver', 12, 0, {CREATURE}),
    ('Lady Vox', 57, 0, {CREATURE}),
    ('crag wisp', 30, 0, {CREATURE}),
    ('Thal Twisp', 8, 128, {CREATURE}),
    ('Guard Dahvel', 40, 0, {HUMAN}),
    ('Merchant Bob', 20, 128, {CREATURE}),
    ('gnoll shaman', 20, 0, {CREATURE}),
    ('Training Dummy', 20, 0, {CREATURE}),
    ('fire giant', 50, 0, {CREATURE});
INSERT INTO npcclass VALUES ('moss snake', 0, 'by name'), ('gnoll shaman', 13, 'sandstorm'), ('crag wisp', 10, 'sandstorm');
INSERT INTO npcloot (npc_name, tablename, chance) VALUES ('lady vox', 'boss lady vox', 100);
INSERT INTO npctemplate (name, template) VALUES ('fire giant', 'tank');
"""


class SeedRoamers(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        c = sqlite3.connect(self.db)
        c.executescript(SCHEMA)
        c.commit()
        c.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup", *args], capture_output=True, text=True)
        if r.returncode != 0:
            raise AssertionError(r.stdout + r.stderr)
        return r.stdout

    def tables(self):
        c = sqlite3.connect(self.db)
        try:
            names = [n for (n,) in c.execute("SELECT name FROM sqlite_master WHERE type = 'table' AND name != 'sqlite_sequence' ORDER BY name")]
            return {n: c.execute(f"SELECT * FROM {n} ORDER BY 1").fetchall() for n in names}
        finally:
            c.close()

    def templates(self):
        c = sqlite3.connect(self.db)
        try:
            return dict(c.execute("SELECT name, template FROM npctemplate"))
        finally:
            c.close()

    def test_who_gets_what(self):
        before = self.tables()
        out = self.run_tool("--list")
        self.assertIn("6 rows to add", out)  # the roamer and wisp rows and four npctemplate rows
        self.assertEqual(self.tables(), before)  # report only
        self.assertIn("timed special columns are added", out)
        self.run_tool("--apply")
        self.assertEqual(self.templates(), {"moss snake": "roamer", "deathfist reaver": "roamer", "lady vox": "roamer",
                                            "crag wisp": "wisp", "fire giant": "tank"})
        c = sqlite3.connect(self.db)
        row = c.execute("SELECT class, hp_pct, taken_pct, melee_pct, special, special_chance, special_damage, special_every, "
                        "special_first, special_jitter, special_from FROM classtemplates WHERE name = 'roamer'").fetchone()
        self.assertEqual(row, (0, 100, 100, 100, "Critical Hit", 0, 7.4, 30.0, 15.0, 5.0, 10))
        wisp = c.execute("SELECT class, hp_pct, taken_pct, melee_pct, abilities, source FROM classtemplates WHERE name = 'wisp'").fetchone()
        self.assertEqual(wisp, (13, 85, 100, 100, WIZARD_ABILITIES.replace('"damage_pct":35', '"damage_pct":28'), sr.SOURCE))
        self.assertEqual(c.execute("SELECT abilities, source FROM classtemplates WHERE name = 'wizard'").fetchone(),
                         (WIZARD_ABILITIES, "seed"))  # jib's left as he made it
        notes = dict(c.execute("SELECT name, note FROM npctemplate WHERE source = ?", (sr.SOURCE,)))
        cols = [r[1] for r in c.execute(f"PRAGMA table_info({sr.LOG})")]
        c.close()
        self.assertEqual(notes["lady vox"], "boss, level 57")
        self.assertEqual(notes["moss snake"], "generic, level 6")
        secret = secret_like()
        for col in cols + [sr.LOG]:
            self.assertFalse(secret(col), f"{col} is named like a secret: every database snapshot would be refused")
        self.assertIn("0 rows to add", self.run_tool("--apply"))
        self.run_tool("--undo")
        after = self.tables()
        # the timed columns stay (at 0 they give no template a clock); every other column and row as before
        self.assertEqual(after.keys(), before.keys())
        for t in before:
            if t == "classtemplates":  # the columns added last, at 0
                self.assertEqual([r[:12] for r in after[t]], before[t])
                self.assertEqual({r[12:] for r in after[t]}, {(0.0, 0.0, 0.0, 0)})
            else:
                self.assertEqual(after[t], before[t], t)

    def test_left_and_said(self):
        out = self.run_tool()
        for why in ("a player race's body", "no hostile spawn", "its class is 13", "a training dummy", "npctemplate gives it 'tank' already"):
            self.assertIn(why, out)
        c = sqlite3.connect(self.db)
        verdicts = {v[0]: v for v in sr.classify(c)}
        c.close()
        self.assertEqual(verdicts["thal twisp"][3], None)  # a proper name, the NPC bit
        self.assertEqual(verdicts["deathfist reaver"][2], "generic")
        self.assertEqual(verdicts["lady vox"][2], "boss")

    def test_a_gms_wisp_row_is_kept(self):
        c = sqlite3.connect(self.db)
        c.execute("INSERT INTO classtemplates (name, class, hp_pct, abilities, source) VALUES ('wisp', 13, 70, '[]', 'gm')")
        c.commit()
        c.close()
        self.assertIn("5 rows to add", self.run_tool("--apply"))  # the roamer row and four names, no wisp row
        c = sqlite3.connect(self.db)
        self.assertEqual(c.execute("SELECT hp_pct, source FROM classtemplates WHERE name = 'wisp'").fetchone(), (70, "gm"))
        c.close()
        self.assertEqual(self.templates()["crag wisp"], "wisp")
        self.run_tool("--undo")
        self.assertNotIn("crag wisp", self.templates())

    def test_no_wizard_no_wisp(self):
        c = sqlite3.connect(self.db)
        c.execute("DELETE FROM classtemplates WHERE name = 'wizard'")
        c.commit()
        c.close()
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, "--no-backup"], capture_output=True, text=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("no wizard row", r.stderr + r.stdout)

    def test_a_changed_row_is_kept_by_the_undo(self):
        self.run_tool("--apply")
        c = sqlite3.connect(self.db)
        c.execute("UPDATE npctemplate SET template = 'melee' WHERE name = 'moss snake'")
        c.commit()
        c.close()
        out = self.run_tool("--undo")
        self.assertIn("'moss snake': changed since", out)
        self.assertEqual(self.templates(), {"moss snake": "melee", "fire giant": "tank"})


if __name__ == "__main__":
    unittest.main()
