#!/usr/bin/env python3
"""moogie_chain_688_20261115.py on a database of its own, made with the server's own table definitions (read out of
internal/database's Go source, classtemplates as an older server made it: no melee_fixed) and the rows the spec found
(Metal Moogie V2 spawn-only with his Moogie Tribute, his template, Healbot's Cog, Root and Halfling Heroism): --dry
writes nothing; --apply writes the 33 rows, takes the next free ids, adds melee_fixed, every row with its gm_audit
line; a second --apply writes nothing; --undo puts every table back exactly, V2's tribute row under its own id. The
controls: a shard with no Metal Moogie V2 is left alone, and a missing Root refuses everything."""
import glob
import json
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "moogie_chain_688_20261115.py")
GO = os.path.join(HERE, "..", "internal", "database")
TABLES = ("itempattern", "itemeffect", "npcs", "npcspawnonly", "npcitem", "classtemplates", "npctemplate", "familiars",
          "spellpattern", "gm_audit")


def schema(name):
    for path in sorted(glob.glob(os.path.join(GO, "*.go"))):
        if path.endswith("_test.go"):
            continue
        with open(path) as f:
            m = re.search(r"CREATE TABLE IF NOT EXISTS " + name + r"\s*\(.*?\n\s*\);", f.read(), re.S)
        if m:
            text = m.group(0)
            if name == "classtemplates":
                text = re.sub(r"\n\s*melee_fixed[^\n]*", "", text)  # an older server's table
            return text
    raise AssertionError(f"no schema of {name} in {GO}")


def dump(db):
    out = {}
    for t in TABLES:
        if t == "gm_audit":
            continue
        cur = db.execute(f"SELECT * FROM {t}")
        cols = [d[0] for d in cur.description if d[0] != "melee_fixed"]
        out[t] = sorted(json.dumps({c: v for c, v in zip([d[0] for d in cur.description], r) if c in cols}, sort_keys=True)
                        for r in cur.fetchall())
    return out


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


class MoogieChainTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        for t in TABLES:
            db.execute(schema(t))
        x = db.execute
        item = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, "
                "weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, "
                "itemdesc, model, color) VALUES (?, 0, 0, ?, 0, -1, 0, 1, 1, 0, 0, 0, 0, ?, 1, 0, ?, ?, ?, ?, 1, 0, 0, ?, '', 0, 0)")
        x(item, (990910, 192051396, 0, 0, 0, 0, 0, "Moogie Tribute"))
        x(item, (715060, 192051396, 1, 80, 32767, 32767, 7, "Healbot's Cog"))
        x("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, modelid, size, npc_level, npc_type, npc_id, race) "
          "VALUES (162677, 'Metal Moogie V2', 0, 25290, 54.094, 15756.2, 100810757, 9.0, 60, 0, 162677, '0')")
        x("INSERT INTO npcspawnonly (name, note, source) VALUES ('metal moogie v2', '', 'moogie1110')")
        x("INSERT INTO npcitem (itemid, npcid, stackleft, patternid, location) VALUES (37090, 162677, 1, 990910, 1)")
        x("INSERT INTO classtemplates (name, class, hp_pct, melee_pct, proc_chance, proc_damage, proc_name, proc_resist, kick_damage, "
          "kick_every, kick_first, abilities) VALUES ('metal moogie v2', 0, 382, 578, 25, 385, 'Static Discharge', 'LR', 578, 6, 6, ?)",
          ('[{"kind":"lifetap","spell_id":905050,"every":30,"first":30,"damage_pct":110}]',))
        sp = ("INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, "
              "eqprequire, spellname, spelldesc) VALUES (?, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, ?, '')")
        x(sp, (460, "Root"))
        x(sp, (901843, "Halfling Heroism"))
        db.close()

    def db(self):
        return sqlite3.connect(self.path)

    def test_dry_apply_again_undo(self):
        before = dump(self.db())
        r = run(self.path, "--dry")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("33 rows would be written", r.stdout)
        self.assertEqual(dump(self.db()), before)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        db = self.db()
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 33)
        ids = dict(db.execute("SELECT itemname, patternid FROM itempattern").fetchall())
        self.assertEqual(ids["Metal Moogie Legs"], 990911, "the next free id after the highest")
        self.assertEqual(db.execute("SELECT maxstack, trade, lore, levelreq FROM itempattern WHERE itemname = 'Snowball'").fetchone(),
                         (99, 0, 0, 1))
        self.assertEqual(db.execute("SELECT procanim, duration, lore FROM itempattern WHERE itemname = 'Moogie Winding Stone'").fetchone(),
                         (7, 80, 1))
        self.assertEqual(db.execute("SELECT patternid FROM npcitem WHERE npcid = 162677").fetchall(), [(ids["Metal Moogie Legs"],)])
        for v, part in (("Metal Moogie V3", "Torso"), ("Metal Moogie V4", "Arms"), ("Metal Moogie V5", "Head")):
            got = db.execute("SELECT p.itemname, n.size, n.id = n.npc_id FROM npcitem i JOIN npcs n ON n.id = i.npcid "
                             "JOIN itempattern p ON p.patternid = i.patternid WHERE n.npc_name = ?", (v,)).fetchall()
            self.assertEqual(got, [("Metal Moogie " + part, 9.0, 1)], v)
        self.assertEqual(db.execute("SELECT size FROM npcs WHERE npc_name = 'Moogie Gearling'").fetchone(), (4.5,))
        self.assertEqual(db.execute("SELECT melee_fixed, hp_pct FROM classtemplates WHERE name = 'moogie gearling'").fetchone(), (25, 35))
        v5 = json.loads(db.execute("SELECT abilities FROM classtemplates WHERE name = 'metal moogie v5'").fetchone()[0])
        self.assertEqual([a.get("name") or a["kind"] for a in v5], ["lifetap", "Arcane Siphon", "summon", "Self Repair"])
        self.assertEqual(db.execute("SELECT count(*) FROM npcspawnonly").fetchone()[0], 5)
        self.assertEqual(db.execute("SELECT name, heal, size FROM familiars").fetchall(), [("Mini Metal Moogie", 0, 2.0)])
        lines = dict(db.execute("SELECT p.itemname, e.line FROM itemeffect e JOIN itempattern p ON p.patternid = e.patternid").fetchall())
        self.assertEqual(lines["Snowball"], "Target, Snowball 0x15C66F59")
        db.close()
        r = run(self.path, "--apply")
        self.assertIn("nothing to write", r.stdout)
        self.assertEqual(self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0], 33)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertNotIn("LEFT", r.stdout)
        self.assertEqual(dump(self.db()), before)

    def test_another_shard_left_alone(self):
        self.db().execute("DELETE FROM npcs").connection.commit()
        before = dump(self.db())
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0)
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.db()), before)

    def test_no_root_refuses(self):
        db = self.db()
        db.execute("DELETE FROM spellpattern WHERE spellid = 460")
        db.commit()
        before = dump(self.db())
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 2)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.db()), before)


if __name__ == "__main__":
    unittest.main()
