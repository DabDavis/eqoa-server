#!/usr/bin/env python3
"""tools/faction_stone_5089_20261115.py on a database of its own: refused without the server's itemfactions table;
the stone's record and one row per faction of the level 17 quests' NPCs (and their city's guards) at 700; a second
run nothing; --undo exact."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "faction_stone_5089_20261115.py")
COLS = ("patternid itemcost unk1 itemicon unk2 equipslot unk3 trade rent unk4 attacktype weapondamage unk5 levelreq "
        "maxstack maxhp duration classuse raceuse procanim lore unk6 craft itemname itemdesc model color").split()


def make(path, table=True):
    db = sqlite3.connect(path)
    db.executescript(f"""
        CREATE TABLE itempattern ({", ".join(COLS)});
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT);
        CREATE TABLE npcfaction (id INTEGER PRIMARY KEY, name TEXT, world INTEGER, faction TEXT);
        CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
        {"CREATE TABLE itemfactions (patternid INTEGER NOT NULL, faction TEXT NOT NULL, standing INTEGER NOT NULL, "
         "note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (patternid, faction));"
         if table else ""}
        INSERT INTO factions VALUES ('Citizens of Freeport', 1), ('The Freeport Militia', 1),
            ('Citizens of Neriak', -2000), ('Citizens of Oggok', -2000);
        INSERT INTO npcfaction (name, world, faction) VALUES ('Scribe Cathassa', -1, 'Citizens of Freeport'),
            ('Tanadril N`Thael', -1, 'Citizens of Neriak'), ('Wandering Priestess', -1, 'Citizens of Oggok');
    """)
    for q in range(100747, 100759):
        db.execute("INSERT INTO quests VALUES (?, 'q', ?)", (q, "Scribe Cathassa" if q == 100748 else "Nobody"))
    db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?)", [
        (100756, 2, "loot", "Wandering Priestess"), (100756, 3, "give", "tanadril n`thael")])
    db.commit()
    db.close()


def run(path, *args, check=True):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True,
                          check=check)


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l and "sqlite_sequence" not in l]
    db.close()
    return out


class Test(unittest.TestCase):
    def test_refused_without_the_table(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p, table=False)
            r = run(p, "--apply", check=False)
            self.assertEqual(r.returncode, 1)
            self.assertIn("no itemfactions table", r.stdout)

    def test_apply_again_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = dump(p)
            run(p, "--apply")
            db = sqlite3.connect(p)
            rows = dict(db.execute("SELECT faction, standing FROM itemfactions WHERE patternid = 715100"))
            stone = db.execute("SELECT itemname, equipslot, trade, lore, procanim FROM itempattern").fetchall()
            db.close()
            # a looted creature's faction is not a giver's: Oggok stays out
            self.assertEqual(rows, {"Citizens of Freeport": 700, "The Freeport Militia": 700, "Citizens of Neriak": 700})
            self.assertEqual(stone, [("Stone of Seventeen Banners", -1, 1, 1, 7)])
            self.assertIn("applied already", run(p, "--apply").stdout)
            self.assertIn("put back 4", run(p, "--undo").stdout)
            self.assertEqual(dump(p), before)


if __name__ == "__main__":
    unittest.main()
