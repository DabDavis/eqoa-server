#!/usr/bin/env python3
"""rogue_skeleton_1998_20261113.py on a small database holding Brute's Mask, Remove Illusion (the pink man), Wiles'
scroll sold by Merchant Morian and two Spell Archivists (one listing Brute's Mask), skeleton NPCs and others: the
report writes nothing; --apply makes Skeletal Mask at the next spell id (the skeleton's sprite read the other way
round, Brute's Mask's frame, the pink man), its classspells row, its scroll at the next item id and a row for each
seller (beside Brute's Mask where listed, else last), a gm_audit line a row; a second apply writes nothing; --undo
puts every table back exactly (the id counters too). The controls: another skeleton body most worn, no Merchant
Morian, and a spell of the name already there are refused, writing nothing."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "rogue_skeleton_1998_20261113.py")
sys.path.insert(0, HERE)
import rogue_skeleton_1998_20261113 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
    effects TEXT NOT NULL DEFAULT '');
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL, PRIMARY KEY (class, spellid));
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL, itemicon INTEGER NOT NULL,
    levelreq INTEGER NOT NULL, classuse INTEGER NOT NULL, raceuse INTEGER NOT NULL, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT NOT NULL, npc_type INTEGER NOT NULL, modelid INTEGER NOT NULL);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, stackleft INTEGER NOT NULL DEFAULT 0,
    remainHP INTEGER NOT NULL DEFAULT 0, remaincharge INTEGER NOT NULL DEFAULT 0, patternid INTEGER NOT NULL,
    equiploc INTEGER NOT NULL DEFAULT 0, location INTEGER NOT NULL DEFAULT 1, listnumber INTEGER NOT NULL DEFAULT 0);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""
MORIAN, ARCH_A, ARCH_B = 589, 162588, 165284


def make(path, skeleton=tool.SKELETON, morian=True, named=False):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO spellpattern VALUES (700549, 57, 0, 0, 16544, 5, 80, -1809115846, -1569239993, 0, 1, -1, "
               "'Brute''s Mask', 'Self, Ogre Mask, 10min')")
    db.execute("INSERT INTO spelleffect VALUES (700549, 'Self, Gives Form, Faction Change, 10min', 2021015237, 'test', '{}')")
    db.execute("INSERT INTO classspells VALUES (6, 700549, 'eqoa-data')")
    db.execute("INSERT INTO spellpattern VALUES (1793, 1, 0, 0, 16544, 3, 1, -1809115846, 1507275072, 0, 5, -1, "
               "'Remove Illusion', '')")
    db.execute("INSERT INTO spellpattern VALUES (2844, 1, 0, 0, 16544, 3, 1, -1809115846, -466420384, 0, 5, -1, "
               "'Remove Illusion', '')")
    db.execute("INSERT INTO spellpattern VALUES (906900, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Last Spell', '')")
    if named:
        db.execute(f"INSERT INTO spellpattern VALUES (NULL, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, '{tool.NAME}', '')")
    db.execute("INSERT INTO itempattern VALUES (700123, 3086, -607649328, 60, 64, 1023, 'Wiles', "
               "'This scroll will allow you to learn Wiles.')")
    db.execute("INSERT INTO itempattern VALUES (716224, 3500, -607649328, 57, 64, 1023, 'Brute''s Mask', 'Self, Ogre Mask, 10min')")
    db.execute("INSERT INTO itempattern VALUES (980000, 1, 0, 1, 0, 0, 'Last Item', '')")
    npcs = [(ARCH_A, "Spell Archivist", 129, 1), (ARCH_B, "Spell Archivist", 129, 1), (9, "Spell Archivist", 0, 1)]
    if morian:
        npcs.append((MORIAN, "Merchant Morian", 129, 1))
    npcs += [(100 + i, "skeleton", 0, skeleton) for i in range(3)] + [(200, "skeletal dog", 0, 1794738816),
                                                                     (201, "skeleton", 0, 1687026705)]
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?)", npcs)
    rows = [(MORIAN, 700123, 16), (ARCH_A, 716224, 80), (ARCH_A, 700123, 196), (ARCH_B, 700123, 7), (9, 700123, 3)]
    for npc, pat, ln in rows:
        db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) VALUES (?, 1, ?, -1, 1, ?)",
                   (npc, pat, ln))
    db.commit()
    db.close()


def dump(path, audit=False):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if audit or "gm_audit" not in l]
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, "-I", TOOL, "--db", path, *args], capture_output=True, text=True)


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_sprite_is_the_body_read_the_other_way_round(self):
        self.assertEqual(tool.sprite_of(1794738817), "0x818AF96A")  # CSprite 0x6AF98A81
        self.assertEqual(tool.sprite_of(0x70D898C6), "0xC698D870")  # the human man: Mortal Mask's sprite

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db, audit=True)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db, audit=True), before, "the report wrote")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        sp = db.execute("SELECT * FROM spellpattern WHERE spellname = ?", (tool.NAME,)).fetchone()
        self.assertEqual(sp, (906901, 60, 0, 0, 16544, 5, 80, -1809115846, 1507275072, 0, 1, -1, tool.NAME, tool.DESC))
        line, fx, eff = db.execute("SELECT line, fxdict, effects FROM spelleffect WHERE spellid = 906901").fetchone()
        self.assertEqual((line, fx), ("Self, Gives Form, 10min", 2021015237))
        eff = json.loads(eff)
        self.assertEqual((eff["scope"], eff["duration_s"], eff["stack"], eff["effects"]),
                         ("self", 600, 2, [{"type": "Alter Appearance", "kind": "illusion", "sprite": "0x818AF96A"}]))
        self.assertEqual(db.execute("SELECT source FROM classspells WHERE class = 6 AND spellid = 906901").fetchone(), ("vendor",))
        item = db.execute("SELECT * FROM itempattern WHERE itemname = ?", (tool.NAME,)).fetchone()
        self.assertEqual(item, (980001, 3086, -607649328, 60, 64, 1023, tool.NAME, tool.SCROLL_DESC))
        sold = db.execute("SELECT npcid, listnumber, stackleft, equiploc FROM npcitem WHERE patternid = 980001 ORDER BY npcid").fetchall()
        self.assertEqual(sold, [(MORIAN, 17, 1, -1), (ARCH_A, 80, 1, -1), (ARCH_B, 8, 1, -1)])  # never npc 9: no merchant
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 7)
        db.close()
        applied = dump(self.db, audit=True)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.db, audit=True), applied, "a second apply wrote")
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), [l for l in before if "gm_audit" not in l])

    def test_refusals(self):
        for kw in ({"skeleton": 1168683108}, {"morian": False}, {"named": True}):
            with self.subTest(**kw):
                if os.path.exists(self.db):
                    os.remove(self.db)
                make(self.db, **kw)
                before = dump(self.db, audit=True)
                r = run(self.db, "--apply")
                self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
                self.assertIn("REFUSED", r.stdout)
                self.assertEqual(dump(self.db, audit=True), before)


if __name__ == "__main__":
    unittest.main()
