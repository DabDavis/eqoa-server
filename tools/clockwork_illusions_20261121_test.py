#!/usr/bin/env python3
"""Tests of tools/clockwork_illusions_20261121.py on a small database of the tables it reads and writes: the sprites
byte-reversed, the portal behind the windmill, an apply that teaches each stage its illusions and moves the windmill
stage, an undo that leaves the database as it was, a refusal when a body is unproven."""
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import clockwork_illusions_20261121 as ci  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
  unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL, iconColor INTEGER NOT NULL,
  icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL, eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL,
  spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
  effects TEXT NOT NULL DEFAULT '');
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL, itemicon INTEGER NOT NULL,
  equipslot INTEGER NOT NULL, levelreq INTEGER NOT NULL, classuse INTEGER NOT NULL, raceuse INTEGER NOT NULL,
  itemname TEXT NOT NULL, itemdesc TEXT NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, x_coord REAL, y_coord REAL, z_coord REAL,
  modelid INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, source TEXT);
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
  target TEXT NOT NULL DEFAULT '', x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0,
  spells TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
  weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
  source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
  rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
  PRIMARY KEY (npc_name, tablename));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT, action TEXT,
  tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""

GOLEM_FX = {"v": 1, "scope": "target", "target_type": "Friendly", "harmful": False, "duration_s": 600, "stack": 4,
            "level": 39, "min_level": 23, "req_equipment": "Any", "sheet_type": "Alter Appearance",
            "arch_type": "Illusion", "effects": [{"id": "02BB", "type": "Alter Appearance", "kind": "illusion",
                                                  "sprite": "0x3C14F3BF"}]}


def spawn(npc, at):
    return {"on": "done", "do": "spawn", "npc": npc, "group": True, "world": 0, "x": at[0], "y": at[1], "z": at[2]}


def make(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO spellpattern VALUES (575, 39, 0, 0, 16544, 5, 120, -1809115846, -1569239993, 1, 1, -1, "
               "'Illusion: Clay Golem', 'Illusion only')")
    db.execute("INSERT INTO spelleffect VALUES (575, 'Target, Gives Form, 10min', 2021015237, 'workbook', ?)",
               (json.dumps(GOLEM_FX),))
    db.execute("INSERT INTO itempattern VALUES (21345, 1311, ?, -1, 39, 4096, 1007, 'Illusion: Clay Golem', 'scroll')",
               (ci.SCROLL_ICON,))
    for _, _, model, wearer, _ in ci.ILLUSIONS:
        db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, modelid) VALUES (?, 1, 2, 3, ?)",
                   (wearer, model - (1 << 32) if model >= 1 << 31 else model))
    for a in ci.ANCHORS:
        db.execute("INSERT INTO npcs (npc_name, x_coord, y_coord, z_coord, modelid) VALUES (?, ?, ?, ?, 1)", (a,) + ci.OLD)
    for n, st in enumerate(ci.STAGES, 1):
        qid = 100 + n
        db.execute("INSERT INTO quests VALUES (?, ?, ?, 'clockwork1120')", (qid, f"The Clockwork Invasion: {st}", ci.GIVER))
        if n == 1:
            db.execute("INSERT INTO queststeps (questid, step, kind, target, x, y, z, events) VALUES (?, 1, 'reach', "
                       "'the windmill', ?, ?, ?, ?)", (qid,) + ci.OLD + (json.dumps(
                           [{"on": "done", "do": "popup", "text": "a portal"}, spawn("Corroded Saboteur", ci.OLD),
                            spawn("deadly corrosion", ci.OLD)]),))
            db.execute("INSERT INTO queststeps (questid, step, kind, target, events) VALUES (?, 2, 'kill', "
                       "'deadly corrosion', ?)", (qid, json.dumps([spawn("Corroded Militant", ci.OLD)])))
        db.execute("INSERT INTO queststeps (questid, step, kind, target) VALUES (?, 4, 'give', ?)", (qid, ci.GIVER))
    db.execute("INSERT INTO npcloot (npc_name, tablename, chance) VALUES ('corroded giant', 'corroded giant', 100)")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f'SELECT * FROM "{t}" ORDER BY 1, 2').fetchall() for (t,) in db.execute(
        "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT IN ('gm_audit', 'sqlite_sequence')")}
    db.close()
    return out


def run(*args):
    with redirect_stdout(io.StringIO()) as f:
        code = ci.main(list(args))
    return code, f.getvalue()


class TestClockworkIllusions(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_sprite_is_the_body_byte_reversed(self):
        self.assertEqual(ci.sprite(0x8348419F), "0x9F414883")
        self.assertEqual(ci.sprite(0x06024005), "0x05400206")
        # the root CLAUDE.md's example: the barbarian 0x61CA3EDF is sprite 0xDF3ECA61
        self.assertEqual(ci.sprite(0x61CA3EDF), "0xDF3ECA61")

    def test_portal_is_behind_the_windmill(self):
        # yaw -0.87: its back is toward +x and -z, 7 from its centre, on its terrace
        dx, dz = ci.PORTAL[0] - ci.MILL_AT[0], ci.PORTAL[2] - ci.MILL_AT[2]
        self.assertGreater(dx, 0)
        self.assertLess(dz, 0)
        self.assertAlmostEqual((dx * dx + dz * dz) ** 0.5, ci.BACK, delta=0.1)
        self.assertEqual(ci.PORTAL[1], ci.FLOOR)

    def test_every_stage_and_the_giant(self):
        self.assertEqual(sorted({s for *_, s in ci.ILLUSIONS}), [0, 1, 2, 3, 4, 5])
        self.assertEqual(len({n for n, *_ in ci.ILLUSIONS}), len(ci.ILLUSIONS))
        self.assertTrue(all(len(n) <= 43 for n, *_ in ci.ILLUSIONS))  # the longest spell name the server has
        self.assertTrue(all(lv in ci.PRICE for _, lv, *_ in ci.ILLUSIONS))

    def test_apply_undo_apply(self):
        before = dump(self.db)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before)  # a report writes nothing
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.db)
        spells = dict(db.execute("SELECT spellname, spellid FROM spellpattern WHERE spellid != 575"))
        self.assertEqual(set(spells), {n for n, *_ in ci.ILLUSIONS})
        for name, lv, model, _, stage in ci.ILLUSIONS:
            sp = db.execute("SELECT abilitylvl, scope, power, spelldesc FROM spellpattern WHERE spellid = ?",
                            (spells[name],)).fetchone()
            self.assertEqual(sp, (lv, 1, 120, "Illusion only"))
            fx = json.loads(db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (spells[name],)).fetchone()[0])
            self.assertEqual([e["kind"] for e in fx["effects"]], ["illusion"])
            self.assertEqual(fx["effects"][0]["sprite"], ci.sprite(model))
            self.assertEqual((fx["scope"], fx["stack"], fx["duration_s"], fx["level"]), ("target", 4, 600, lv))
            sc = db.execute("SELECT itemicon, equipslot, classuse, raceuse, levelreq FROM itempattern WHERE itemname = ?",
                            (name,)).fetchall()
            self.assertEqual(sc, [(ci.SCROLL_ICON, -1, ci.ALL_CLASSES, ci.ALL_RACES, lv)])
        for n in range(1, 6):
            got = db.execute("SELECT spells FROM queststeps WHERE questid = ? AND kind = 'give'", (100 + n,)).fetchone()[0]
            want = sorted(spells[name] for name, *_, st in ci.ILLUSIONS if st == n)
            self.assertEqual(sorted(int(x) for x in got.split(",")), want)
        self.assertEqual(db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (ci.GIANT_ILLUSIONS,)).fetchone()[0],
                         len(ci.ILLUSIONS))
        self.assertEqual(db.execute("SELECT x, y, z FROM queststeps WHERE questid = 101 AND step = 1").fetchone(), ci.WINDMILL)
        evs = [e for (t,) in db.execute("SELECT events FROM queststeps WHERE questid = 101 AND events != ''")
               for e in json.loads(t) if e["do"] == "spawn"]
        self.assertEqual(len(evs), 3)
        self.assertTrue(all((e["x"], e["y"], e["z"]) == ci.PORTAL for e in evs))
        for a in ci.ANCHORS:
            self.assertEqual(db.execute("SELECT x_coord, y_coord, z_coord FROM npcs WHERE npc_name = ?", (a,)).fetchone(),
                             ci.PORTAL)
        self.assertGreater(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 20)
        db.close()
        code, out = run("--db", self.db, "--apply")
        self.assertIn("applied already", out)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(dump(self.db), before)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)

    def test_refused_when_a_body_is_unproven(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npcs SET modelid = 5 WHERE npc_name = 'model xiv'")
        db.commit()
        db.close()
        before = dump(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 1)
        self.assertIn("model xiv", out)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
