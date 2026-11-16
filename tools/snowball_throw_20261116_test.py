#!/usr/bin/env python3
"""snowball_throw_20261116.py on a database of its own (the server's own itempattern and gm_audit, read out of
internal/database's Go source) holding the three records as moogie_chain_688_20261115.py wrote them and the Hill
Giant's Throwing Stone: --dry writes nothing; --apply makes the Snowball a usable thrown weapon (type 2, wear 16,
attack 10, reach 30, the stone's model) and both potions usable, each record with its gm_audit line; a second
--apply writes nothing; --undo puts the table back exactly. The controls: a GM's change since is left by both, a
shard without the records is left alone, and two Snowballs refuse everything."""
import glob
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "snowball_throw_20261116.py")
GO = os.path.join(HERE, "..", "internal", "database")
STONE_MODEL = 159183456


def schema(name):
    for path in sorted(glob.glob(os.path.join(GO, "*.go"))):
        if path.endswith("_test.go"):
            continue
        with open(path) as f:
            m = re.search(r"CREATE TABLE IF NOT EXISTS " + name + r"\s*\(.*?\n\s*\);", f.read(), re.S)
        if m:
            return m.group(0)
    raise AssertionError(f"no schema of {name} in {GO}")


def dump(db):
    return sorted(db.execute("SELECT * FROM itempattern").fetchall())


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


ITEM = ("INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, "
        "weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, "
        "itemdesc, model, color) VALUES (?, 0, 0, 192051396, 0, ?, 0, ?, 1, 0, ?, ?, 0, 1, ?, 0, 0, 32767, 32767, ?, 0, 0, 0, ?, "
        "'', ?, 4294967295)")


class SnowballThrowTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
        db.execute(schema("itempattern"))
        db.execute(schema("gm_audit"))
        # id, wear, trade, attack, damage, maxstack, procanim, name, model
        db.execute(ITEM, (1000571, -1, 0, 0, 0, 99, 0, "Snowball", 0))
        db.execute(ITEM, (1000572, -1, 1, 0, 0, 1, 0, "Potion of Moogie's Insight", 0))
        db.execute(ITEM, (1000573, -1, 1, 0, 0, 1, 0, "Deputy's Draught", 0))
        db.execute(ITEM, (6061, 12, 0, 3, 509, 40, 0, "Hill Giant's Throwing Stone", STONE_MODEL))
        db.close()

    def db(self):
        return sqlite3.connect(self.path)

    def test_dry_apply_again_undo(self):
        before = dump(self.db())
        r = run(self.path, "--dry")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("3 records would be changed", r.stdout)
        self.assertEqual(dump(self.db()), before)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
        db = self.db()
        self.assertEqual(db.execute("SELECT procanim, equipslot, attacktype, unk5, model, maxstack, weapondamage, trade "
                                    "FROM itempattern WHERE patternid = 1000571").fetchone(), (2, 16, 10, 30, STONE_MODEL, 99, 0, 0))
        self.assertEqual(db.execute("SELECT procanim, equipslot FROM itempattern WHERE patternid IN (1000572, 1000573)").fetchall(),
                         [(2, -1), (2, -1)])
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 3)
        self.assertEqual(db.execute("SELECT model FROM itempattern WHERE patternid = 6061").fetchone(), (STONE_MODEL,))
        db.close()
        r = run(self.path, "--apply")
        self.assertIn("nothing to write", r.stdout)
        self.assertEqual(self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0], 3)
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertNotIn("LEFT", r.stdout)
        self.assertEqual(dump(self.db()), before)
        self.assertEqual(self.db().execute("SELECT count(*) FROM gm_audit").fetchone()[0], 6)

    def test_a_gms_change_is_left(self):
        db = self.db()
        db.execute("UPDATE itempattern SET unk5 = 45 WHERE patternid = 1000571")  # a GM's reach before the run
        db.commit()
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("unk5 is 45", r.stdout)
        self.assertEqual(self.db().execute("SELECT unk5, equipslot FROM itempattern WHERE patternid = 1000571").fetchone(), (45, 16))
        db = self.db()
        db.execute("UPDATE itempattern SET procanim = 5 WHERE patternid = 1000572")  # and one after it
        db.commit()
        r = run(self.path, "--undo")
        self.assertIn("changed since: LEFT", r.stdout)
        self.assertEqual(self.db().execute("SELECT procanim FROM itempattern WHERE patternid = 1000572").fetchone(), (5,))
        self.assertEqual(self.db().execute("SELECT equipslot, procanim, unk5 FROM itempattern WHERE patternid = 1000571").fetchone(),
                         (-1, 0, 45))

    def test_another_shard_left_alone(self):
        db = self.db()
        db.execute("DELETE FROM itempattern WHERE patternid > 1000000")
        db.commit()
        before = dump(self.db())
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0)
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.db()), before)

    def test_two_snowballs_refuse(self):
        db = self.db()
        db.execute(ITEM, (1000600, -1, 0, 0, 0, 99, 0, "Snowball", 0))
        db.commit()
        before = dump(self.db())
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 2)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.db()), before)


if __name__ == "__main__":
    unittest.main()
