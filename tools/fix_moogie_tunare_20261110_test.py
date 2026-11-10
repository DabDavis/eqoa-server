#!/usr/bin/env python3
"""fix_moogie_tunare_20261110.py on a database of its own, made with the server's own table definitions (read out of
internal/database's Go source) and rows as the brief found them: the report writes nothing; --apply writes Mr.
Moogie's, Tunare's and Innoruuk's templates, the Servant of Moogie and Metal Moogie V2 (V2's figures 1.1 times Metal
Moogie's and his size 1.2 times), every row with its gm_audit line; a second apply is refused; --undo puts everything
back exactly, the ids the new rows took too; a row a GM changed since is left by the undo and said. The controls: a
template not as found refuses everything, and a server with no Mr. Moogie of level 80 is left alone."""
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
sys.path.insert(0, HERE)
import fix_moogie_tunare_20261110 as fm  # noqa: E402

TOOL = os.path.join(HERE, "fix_moogie_tunare_20261110.py")
GO = os.path.join(HERE, "..", "internal", "database")


def schema(name):
    """The server's CREATE TABLE of a table, out of internal/database's Go source."""
    for path in sorted(glob.glob(os.path.join(GO, "*.go"))):
        if path.endswith("_test.go"):
            continue
        with open(path) as f:
            m = re.search(r"CREATE TABLE IF NOT EXISTS " + name + r"\s*\(.*?\n\s*\);", f.read(), re.S)
        if m:
            return m.group(0)
    raise AssertionError(f"no schema of {name} in {GO}")


TABLES = ("npcs", "npcitem", "classtemplates", "npctemplate", "npcspawnonly", "npcaggro", "petstats", "spellpattern",
          "balancelevel", "gm_audit")


def dump(db):
    """Every table the tool touches, and the ids handed out, for an exact compare."""
    out = {}
    for t in TABLES + ("sqlite_sequence",):
        # the ids of the rows the tool makes (the audit's lines are their own: the undo writes its own)
        cur = db.execute(f"SELECT * FROM {t}" + (" WHERE name IN ('npcs', 'npcitem')" if t == "sqlite_sequence" else ""))
        cols = [d[0] for d in cur.description]
        out[t] = sorted(json.dumps(dict(zip(cols, r)), sort_keys=True, default=str) for r in cur.fetchall())
    out.pop("gm_audit")  # the undo writes its own lines
    return out


class FixMoogieTunareTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        for t in TABLES:
            db.execute(schema(t))
        x = db.execute
        x("INSERT INTO balancelevel (level, hp_pct) VALUES (60, 400), (80, 500)")
        npc = "INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, modelid, size, npc_level, npc_type, npc_id, race) " \
              "VALUES (?, ?, 0, ?, 54, 15756, ?, ?, ?, ?, ?, '0')"
        x(npc, (500, "Mr. Moogie", 25318.8, 1234, 2.0, 80, 128, 500))
        x(npc, (501, "Metal Moogie", 25290.0, 4567, 7.5, 60, 0, 501))
        x(npc, (502, "Innoruuk", 25000.0, 789, 3.0, 70, 0, 502))
        x(npc, (503, "Rallos Zek Unbound", 24000.0, 790, 3.0, 70, 0, 503))
        x("INSERT INTO npcitem (npcid, stackleft, patternid, location) VALUES (501, 1, 9000, 1)")
        x("INSERT INTO npcspawnonly (name, note, source) VALUES ('metal moogie', '', 'metalmoogie1109'), ('innoruuk', '', 'x')")
        tmpl = "INSERT INTO classtemplates (name, class, hp_pct, melee_pct, special, special_damage, special_every, kick_damage, " \
               "kick_every, proc_chance, proc_damage, proc_name, proc_resist, fight_regen, abilities) VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
        x(tmpl, ("mr moogie", 6000, 2500, "Crit", 75.0, 30.0, 4000, 5.0, 0, 0, "", "", 2000, json.dumps(fm.MOOGIE_NOW)))
        x(tmpl, ("tunare", 5000, 500, "", 0.0, 0.0, 0, 0.0, 0, 0, "", "", 0, json.dumps(fm.TUNARE_NOW)))
        inno = [{"kind": "meleekind", "melee_kind": "slash", "taken_pct": 50}, {"kind": "meleekind", "melee_kind": "pierce", "taken_pct": 50}] + \
               [{"kind": "nuke", "spell_id": s, "every": 20} for s in fm.INNO_CASTS]
        x(tmpl, ("innoruuk", 2646, 1200, "Crit", 100.0, 25.0, 0, 0.0, 10, 500, "Hate", "AR", 0, json.dumps(inno)))
        x(tmpl, ("rallos zek unbound", 2646, 1500, "", 0.0, 0.0, 0, 0.0, 0, 0, "", "", 0, "[]"))
        x(tmpl, ("metal moogie", 347, 750, "", 0.0, 0.0, 750, 6.0, 25, 500, "Static Discharge", "LR", 0,
                 json.dumps([{"kind": "lifetap", "spell_id": 9100, "every": 30, "first": 30}])))
        x(tmpl, ("pet swarming death", 100, 100, "", 0.0, 0.0, 0, 0.0, 25, 250, "Swarming Sting", "AR", 0, "[]"))
        x("INSERT INTO npctemplate (name, template) VALUES ('mr moogie', 'mr moogie'), ('tunare', 'tunare'), ('innoruuk', 'innoruuk'), "
          "('rallos zek unbound', 'rallos zek unbound'), ('metal moogie', 'metal moogie')")
        x("INSERT INTO petstats (spellid, name, abilities) VALUES (?, 'Red Swarming Death', ?)", (fm.RSD_SPELL, json.dumps(fm.RSD_ABILITIES)))
        for sid, name in ((fm.LARGESSE, "Largesse of Plague"), (fm.VEIL_SPELL, "Veil of Death")):
            x("INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, "
              "spellname, spelldesc) VALUES (?, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, ?, '')", (sid, name))
        x(schema("spelleffect"))
        x("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Self, Proc', ?, 'test')", (fm.VEIL_SPELL, -205483512))  # 0xF3C09208, kept signed as the tables keep a DictID
        db.close()

    def run_tool(self, *args):
        p = subprocess.run([sys.executable, TOOL, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def db(self):
        db = sqlite3.connect(self.path)
        db.row_factory = sqlite3.Row
        return db

    def template(self, name):
        r = self.db().execute("SELECT * FROM classtemplates WHERE name = ?", (name,)).fetchone()
        return None if r is None else dict(r)

    def test_report_apply_and_undo(self):
        before = dump(self.db())
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("report only", out)
        self.assertIn("Largesse of Plague a proc he always has, at a pet's chance of his level 70: 13%", out)
        hp = fm.npc_max_hp(self.db(), 70, 2646)  # 4,200 at the balance's 450%, then 2,646%: 500,094
        self.assertEqual(hp, 500094)
        self.assertIn(f"Innoruuk's hit points {hp:,} (level 70, hp_pct 2646), Rallos Zek Unbound's {hp:,} (level 70, hp_pct 2646): "
                      f"the same; his veil takes {fm.rnd(hp / 100):,} a hit", out)
        self.assertEqual(dump(self.db()), before, "the report wrote")

        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        moogie = json.loads(self.template("mr moogie")["abilities"])
        self.assertEqual(fm.norm(moogie), fm.norm(fm.MOOGIE_NEW))
        lh = moogie[1]
        self.assertEqual((lh["every"], lh["cooldown"], lh["proc_chance"], lh["proc_damage"], lh["say"]),
                         (900, 900, 100, 350, "Thats a fifteen minute cool down, you'll pay for that"))
        tunare = json.loads(self.template("tunare")["abilities"])
        self.assertEqual(fm.norm(tunare), fm.norm([fm.RENEWAL] + fm.TUNARE_NOW))
        inno = self.template("innoruuk")
        kit = json.loads(inno["abilities"])
        self.assertEqual([a["kind"] for a in kit], ["meleekind", "meleekind", "dot", "nuke", "lifetap", "proc", "veil"])
        self.assertEqual(kit[-1].get("fx"), 0xF3C09208, "Veil of Death's own visual on his veil")
        self.assertEqual((inno["proc_chance"], inno["proc_damage"], inno["proc_name"], inno["proc_resist"]), (25, 250, "Swarming Sting", "AR"))
        self.assertEqual((inno["hp_pct"], inno["special"], inno["special_damage"]), (2646, "Crit", 100.0), "his hit points and crit kept")
        db = self.db()
        servant = dict(db.execute("SELECT * FROM npcs WHERE npc_name = 'Servant of Moogie'").fetchone())
        self.assertEqual((servant["npc_level"], servant["size"], servant["npc_type"], servant["modelid"], servant["npc_id"]),
                         (65, 1.0, 0, 1234, servant["id"]))
        self.assertTrue(db.execute("SELECT 1 FROM npcspawnonly WHERE name = 'servant of moogie'").fetchone())
        self.assertEqual(db.execute("SELECT aggressive FROM npcaggro WHERE npc_name = 'Servant of Moogie'").fetchone()[0], 0)
        v2 = dict(db.execute("SELECT * FROM npcs WHERE npc_name = 'Metal Moogie V2'").fetchone())
        self.assertAlmostEqual(v2["size"], 7.5 * 1.2)
        self.assertEqual((v2["modelid"], v2["npc_level"], v2["npc_id"]), (4567, 60, v2["id"]))
        self.assertEqual([r[0] for r in db.execute("SELECT patternid FROM npcitem WHERE npcid = ?", (v2["id"],))], [9000])
        vt, mt = self.template("metal moogie v2"), self.template("metal moogie")
        for c, want in [("hp_pct", 382), ("melee_pct", 825), ("kick_damage", 825), ("proc_damage", 550)]:
            self.assertEqual(vt[c], want, c)
            self.assertAlmostEqual(vt[c] / mt[c], 1.1, delta=0.01)
        self.assertEqual((vt["proc_chance"], vt["kick_every"], vt["proc_name"]), (25, 6.0, "Static Discharge"))
        va = json.loads(vt["abilities"])
        self.assertEqual(fm.norm(va), fm.norm([{"kind": "lifetap", "spell_id": 9100, "every": 30, "first": 30, "damage_pct": 110}, fm.GEAR_BARRAGE]))
        self.assertEqual(db.execute("SELECT template FROM npctemplate WHERE name = 'metal moogie v2'").fetchone()[0], "metal moogie v2")
        audits = db.execute("SELECT count(*) FROM gm_audit WHERE username = 'dab' AND via = 'tool'").fetchone()[0]
        logged = db.execute(f"SELECT count(*) FROM {fm.LOG} WHERE tbl != 'sqlite_sequence'").fetchone()[0]
        self.assertEqual(audits, logged)
        self.assertEqual(logged, 3 + 3 + 5)  # 3 templates; the servant: npcs, spawn-only, aggro; V2: npcs, spawn-only, item, template, npctemplate

        code, out = self.run_tool("--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("applied already", out)

        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db()), before, "the undo is not exact")
        self.assertFalse(self.db().execute("SELECT count(*) FROM sqlite_master WHERE name = ?", (fm.LOG,)).fetchone()[0])

    def test_undo_leaves_a_row_changed_since(self):
        self.assertEqual(self.run_tool("--apply")[0], 0)
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("UPDATE classtemplates SET note = 'a GM was here' WHERE name = 'tunare'")
        db.close()
        code, out = self.run_tool("--undo")
        self.assertNotEqual(code, 0)
        self.assertIn("left: classtemplates {'name': 'tunare'}", out)
        self.assertEqual(fm.norm(json.loads(self.template("mr moogie")["abilities"])), fm.norm(fm.MOOGIE_NOW))
        self.assertEqual(self.template("tunare")["note"], "a GM was here")

    def test_refusals(self):
        for name, q in [
            ("Mr. Moogie's abilities edited", "UPDATE classtemplates SET abilities = '[]' WHERE name = 'mr moogie'"),
            ("Tunare's abilities edited", "UPDATE classtemplates SET abilities = '[]' WHERE name = 'tunare'"),
            ("Innoruuk's casts not the four", "UPDATE classtemplates SET abilities = '[{\"kind\":\"nuke\",\"spell_id\":905145,\"every\":20}]' WHERE name = 'innoruuk'"),
            ("the Red Swarming Death's kit edited", "UPDATE petstats SET abilities = '[]'"),
            ("its template's proc edited", "UPDATE classtemplates SET proc_damage = 300 WHERE name = 'pet swarming death'"),
            ("Largesse of Plague renamed", "UPDATE spellpattern SET spellname = 'Something' WHERE spellid = 700794"),
            ("a Servant of Moogie there", "INSERT INTO npcs (npc_name, npc_level) VALUES ('Servant of Moogie', 65)"),
            ("V2 there", "INSERT INTO classtemplates (name, abilities) VALUES ('metal moogie v2', '[]')"),
            ("no Metal Moogie", "DELETE FROM npcs WHERE npc_name = 'Metal Moogie'"),
        ]:
            self.setUp()
            db = sqlite3.connect(self.path, isolation_level=None)
            db.execute(q)
            before = dump(db)
            db.close()
            code, out = self.run_tool("--apply")
            self.assertNotEqual(code, 0, name)
            self.assertIn("refused, nothing done", out, name)
            self.assertEqual(dump(self.db()), before, name + ": something was written")

    def test_not_his_shard(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("UPDATE npcs SET npc_name = 'Scarecrow' WHERE npc_name = 'Mr. Moogie'")
        before = dump(db)
        db.close()
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("not his shard", out)
        self.assertEqual(dump(self.db()), before)


if __name__ == "__main__":
    unittest.main()
