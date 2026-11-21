#!/usr/bin/env python3
"""tools/guide_loot_20261121.py on a small made database: a dry run writes nothing, --apply makes, wires and logs,
a second --apply writes nothing, --undo puts every table back as it was.

    python3 -E -s tools/guide_loot_20261121_test.py
"""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import guide_loot_20261121 as gl  # noqa: E402

ITEM_COLS = ("patternid,itemcost,unk1,itemicon,unk2,equipslot,unk3,trade,rent,unk4,attacktype,weapondamage,unk5,"
             "levelreq,maxstack,maxhp,duration,classuse,raceuse,procanim,lore,unk6,craft,itemname,itemdesc,model,color,"
             "str,sta,agi,wis,dex,cha,intelligence,HPMAX,POWMAX,PoT,HoT,AC,PR,DR,FR,CR,LR,AR,weaponproc,fish").split(",")
SCHEMA = f"""
CREATE TABLE itempattern ({", ".join(c + (" INTEGER PRIMARY KEY" if c == "patternid" else "") for c in ITEM_COLS)});
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_level INTEGER, world INTEGER, npc_id INTEGER);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY, npcid INTEGER, patternid INTEGER);
CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance INTEGER, rolls INTEGER, note TEXT, source TEXT,
                      PRIMARY KEY (npc_name, tablename));
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER,
                         minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE lootpool (id INTEGER PRIMARY KEY, category TEXT, region TEXT, npc_name TEXT, patternid INTEGER,
                       weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER);
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT, PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT, PRIMARY KEY (questid, step, target));
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
                       ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT, reverted_by INTEGER);
"""


def item(pid, name, slot=-1, lvl=0, stack=999, icon=111):
    r = {c: 0 for c in ITEM_COLS}
    r.update(patternid=pid, itemname=name, itemdesc="", equipslot=slot, levelreq=lvl, maxstack=stack, itemicon=icon,
             itemcost=10, color=4294967295, rent=1)
    return r


def make(path, double=True):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    items = [item(707134, "Bandit Ear"), item(707258, "Crocodile Tooth", icon=222), item(4885, "broken eagle feathers"),
             item(2085, "Rhino Horn Axe of Lightning", slot=14, lvl=54, stack=1),
             item(5365, "Quintashi's Trident", slot=12, lvl=50, stack=1),
             item(1035, "slayer's claw earring", slot=3, lvl=59, stack=1), item(1034, "gold ingot", slot=3, lvl=59, stack=1),
             item(6118, "tunarian shield", slot=13, lvl=58, stack=1), item(6012, "Poisoned Fang", slot=12, lvl=27, stack=1),
             item(5350, "goblin assassin's dirk", slot=12, lvl=60, stack=1), item(701137, "Aktana's soul", stack=1)]
    for r in items:
        db.execute(f"INSERT INTO itempattern ({','.join(r)}) VALUES ({','.join('?' * len(r))})", list(r.values()))
    rows = [(1, "caiman", 19, 0), (2, "young crocodile", 18, 0), (3, "ratman spiritist", 50, 2), (4, "Quintashi", 65, 2),
            (5, "Slayer Demaeog", 64, 2), (6, "Augur Rurrg", 63, 2), (7, "grizz shaman", 59, 2), (8, "Grimrik warrior", 28, 0),
            (9, "Goblin Assassin", 56, 0), (10, "great eagle", 46, 0), (11, "great eagle", 50, 2),
            (12, "Aktana the Flamekissed", 65, 3)]
    if double:
        rows.append((13, "Aktrana the Flamekissed", 65, 3))
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [(i, n, l, w, i) for i, n, l, w in rows])
    db.execute("INSERT INTO quests VALUES (101058, 'Aktana', 1)")
    db.execute("INSERT INTO quests VALUES (101087, 'Aktana', 1)")
    for q in (101058, 101087):
        db.execute("INSERT INTO queststeps VALUES (?, 5, 'loot', 'Aktana the Flamekissed', '701137')", (q,))
    db.commit()
    return db


def digest(db):
    return {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
            for t in ("itempattern", "npcloot", "loottables", "queststeptargets")}


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        gl.main(list(args))
    return out.getvalue()


class GuideLoot(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_cycle(self):
        db = make(self.path)
        before = digest(db)
        out = run("--db", self.path)
        self.assertIn("dry run", out)
        self.assertEqual(digest(db), before)

        out = run("--db", self.path, "--apply")
        self.assertIn("applied:", out)
        # made: the caimans' trash, from 720200, a copy of Bandit Ear by name
        r = db.execute("SELECT patternid, maxstack, trade, lore, equipslot, itemcost FROM itempattern "
                       "WHERE itemname = 'caiman claw'").fetchone()
        self.assertEqual(r[1:5], (999, 0, 0, -1))
        self.assertGreaterEqual(r[0], gl.FIRST_ID)
        self.assertEqual(r[5], round(79 * 0.32))
        # an existing record under the guide's name is wired, never made again
        self.assertEqual(db.execute("SELECT count(*) FROM itempattern WHERE lower(itemname) LIKE 'crocodile tooth'").fetchone()[0], 1)
        # rares and the named
        got = dict(db.execute("SELECT n.npc_name || '|' || t.patternid, n.chance FROM npcloot n JOIN loottables t "
                              "USING (tablename)").fetchall())
        self.assertEqual(got["ratman spiritist|2085"], 1)
        self.assertEqual(got["quintashi|5365"], 100)
        self.assertEqual(got["slayer demaeog|1035"], 100)
        self.assertEqual(got["augur rurrg|1034"], 100)
        self.assertEqual(got["goblin assassin|5350"], 1.5)
        # a name in two worlds is bound by its rows in the camp's world alone
        self.assertNotIn("great eagle|720242", got)
        self.assertTrue(any(k.startswith("#10|") for k in got))
        self.assertFalse(any(k.startswith("#11|") for k in got))
        # the soul's other creature, and a gm_audit line and a log line a row
        self.assertEqual(db.execute("SELECT count(*) FROM queststeptargets").fetchone()[0], 2)
        n_log = db.execute(f"SELECT count(*) FROM {gl.LOG}").fetchone()[0]
        n_audit = db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        self.assertEqual(n_log, n_audit)
        self.assertIn("MasterVG782's GameFAQs guide (camp ", db.execute("SELECT detail FROM gm_audit").fetchone()[0])

        after = digest(db)
        out = run("--db", self.path, "--apply")
        self.assertIn("nothing to write", out)
        self.assertEqual(digest(db), after)

        run("--db", self.path, "--undo")
        self.assertEqual(digest(db), before)

    def test_no_double_no_target(self):
        db = make(self.path, double=False)
        out = run("--db", self.path, "--apply")
        self.assertIn("0 step targets", out)
        self.assertEqual(db.execute("SELECT count(*) FROM queststeptargets").fetchone()[0], 0)


if __name__ == "__main__":
    unittest.main()
