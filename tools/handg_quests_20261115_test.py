#!/usr/bin/env python3
"""tools/handg_quests_20261115.py: its rules cover the source's 77 quests once each, and on a small database a quest
we have is put right, one we lack is made, a second --apply does nothing and --undo puts every row back."""
import io
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import handg_quests_20261115 as h  # noqa: E402
import import_quests  # noqa: E402

TOOL = os.path.join(HERE, "handg_quests_20261115.py")


def small_db(path):
    db = sqlite3.connect(path)
    db.executescript(import_quests.QUEST_DDL)
    db.executescript("""
        ALTER TABLE queststeps ADD COLUMN height REAL NOT NULL DEFAULT 0;
        CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
            PRIMARY KEY (questid, step, target));
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemname TEXT NOT NULL, itemdesc TEXT NOT NULL,
            itemicon INTEGER NOT NULL DEFAULT 0, trade INTEGER NOT NULL DEFAULT 0, maxstack INTEGER NOT NULL DEFAULT 1);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_level INTEGER, world INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        CREATE TABLE charinventory (serverid INTEGER, patternid INTEGER);
    """)
    for n in ("Chef Lukkif", "fire basilisk", "Corison", "cave crawler"):
        db.execute("INSERT INTO npcs (npc_name, npc_level, world) VALUES (?, 50, 0)", (n,))
    for n in ("Broken Leopard Claw", "Dark Ore"):
        db.execute("INSERT INTO itempattern (itemname, itemdesc, itemicon, trade, maxstack) VALUES (?, '', 7, 1, 999)", (n,))
    db.execute("INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, repeatable, notes) "
               "VALUES (101147, 'Dark Ore', 1, 'Corison', 26, 32, 1, 'ours')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, items, count, chance) VALUES (101147, 1, 'loot', 'cave crawler', '2*4', 4, 0.5)")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, items, count, xp) VALUES (101147, 2, 'give', 'Corison', '2*4', 1, 560000)")
    db.commit()
    return db


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY rowid").fetchall()
           for t in ("quests", "queststeps", "queststeptargets", "itempattern")}
    out["seq"] = db.execute("SELECT * FROM sqlite_sequence WHERE name = 'itempattern'").fetchall()
    db.close()
    return out


def tool(path, *args):
    r = subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)
    if r.returncode:
        raise AssertionError(r.stdout + r.stderr)
    return r.stdout


class Rules(unittest.TestCase):
    def test_every_source_quest_has_one_rule(self):
        src, copies, items = h.source()
        self.assertEqual(len(src), 77)
        for k in src:
            self.assertEqual((k in h.PRESENT) + (k in h.CANNOT), 1, k)
        for k in list(h.NEW) + list(h.COPIES):
            self.assertIn(k, h.PRESENT)
        self.assertEqual(sorted(copies), sorted(h.COPIES))
        for c in copies.values():
            for st in c["steps"]:
                for n, _ in st["items"]:
                    self.assertIn(n, items)

    def test_a_sash_is_worth_half_a_weapon_four_to_a_quest(self):
        sash, sword, totem = h.PRESENT[85]
        self.assertEqual((sash.xp, sash.tunar, sash.levels), (40000, 400, (1, 30)))
        self.assertEqual((sword.xp, sword.tunar, totem.tunar), (20000, 200, 200))


class RoundTrip(unittest.TestCase):
    def test_apply_reapply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "t.db")
            small_db(path).close()
            before = dump(path)
            rep = tool(path)
            self.assertIn("MISSING   key 76", rep)
            self.assertIn("tunar 0 -> 630 (fix)", rep)
            self.assertEqual(dump(path), before, "a report writes nothing")
            tool(path, "--apply")
            db = sqlite3.connect(path)
            self.assertEqual(db.execute("SELECT rewardtunar FROM queststeps WHERE questid = 101147 AND step = 2").fetchone()[0], 630)
            self.assertAlmostEqual(db.execute("SELECT chance FROM queststeps WHERE questid = 101147 AND step = 1").fetchone()[0], 0.067)
            q = db.execute("SELECT questid, giver, minlevel, maxlevel, repeatable FROM quests WHERE name = 'Basilisk Claws'").fetchone()
            self.assertEqual(q[1:], ("Chef Lukkif", 50, 60, 1))
            st = db.execute("SELECT kind, target, items, chance FROM queststeps WHERE questid = ? ORDER BY step", (q[0],)).fetchall()
            claw = db.execute("SELECT patternid, trade, maxstack, itemicon FROM itempattern WHERE itemname = 'Basilisk Claw'").fetchone()
            self.assertEqual(st, [("loot", "fire basilisk", f"{claw[0]}*4", 0.5), ("give", "Chef Lukkif", f"{claw[0]}*4", 0.0)])
            self.assertEqual(claw[1:], (1, 99, 7))  # NO TRADE, 99, the leopard claw's icon
            self.assertIsNone(db.execute("SELECT 1 FROM quests WHERE name = 'Flavored Mushrooms'").fetchone(), "no fungal folk here")
            self.assertGreater(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 0)
            db.close()
            applied = dump(path)
            self.assertIn("nothing to do", tool(path, "--apply"))
            self.assertEqual(dump(path), applied)
            tool(path, "--undo")
            self.assertEqual(dump(path), before)
            self.assertIn("nothing to undo", tool(path, "--undo"))


if __name__ == "__main__":
    unittest.main()
