#!/usr/bin/env python3
"""tools/seed_npc_tint.py against a made-up world: the report writes nothing; --apply makes npctint where it is
missing and gives Yhafiran the seed's tint, whether or not an NPC of the name stands yet; a second --apply writes
nothing; --undo puts the database back as it was, dump for dump (the table it made dropped, the log dropped); a GM's
row for the name, in any case, is left as it is, and a seed row a GM changed since is left by --apply and --undo; a
tint of the tool's changed since its last --apply is written over its own row and undone back through it. No game
data.

    python3 tools/seed_npc_tint_test.py
"""
import importlib.util
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "seed_npc_tint.py")
_spec = importlib.util.spec_from_file_location("seed_npc_tint", TOOL)
M = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(M)

TINT = M.TINTS[0][1]


def world(path, npctint=False, rows=()):
    db = sqlite3.connect(path)
    db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, modelid INTEGER)")
    db.executemany("INSERT INTO npcs (npc_name, modelid) VALUES (?, ?)",
                   [("Poxathixius", 0xB5C8A651), ("Uzrathixius", 0xB5C8A651), ("Yhafiran", 0xB5C8A651), ("a moss snake", 1)])
    if npctint:
        db.execute(M.SCHEMA)
        db.executemany("INSERT INTO npctint (name, color, note, source) VALUES (?, ?, ?, ?)", rows)
    db.commit()
    db.close()


def run(path, *args):
    r = subprocess.run([sys.executable, "-I", TOOL, "--db", path, "--no-backup", *args], capture_output=True, text=True)
    if r.returncode != 0:
        raise AssertionError(r.stderr + r.stdout)
    return r.stdout


def dump(path):
    db = sqlite3.connect(path)
    out = list(db.iterdump())
    db.close()
    return out


def tints(path):
    db = sqlite3.connect(path)
    out = {r[0]: r[1:] for r in db.execute("SELECT name, color, source FROM npctint")}
    db.close()
    return out


class SeedTint(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_is_idempotent_and_undo_exact(self):
        world(self.db)
        before = dump(self.db)
        out = run(self.db)
        self.assertIn("to write: Yhafiran 0x2A7333FF", out)
        self.assertIn("1 NPC row(s) of the name", out)
        self.assertEqual(dump(self.db), before, "the report wrote something")
        run(self.db, "--apply")
        self.assertEqual(tints(self.db), {"Yhafiran": (TINT, "seed")})
        once = dump(self.db)
        self.assertIn("nothing to write", run(self.db, "--apply"))
        self.assertEqual(dump(self.db), once, "a second --apply wrote something")
        run(self.db, "--undo")
        self.assertEqual(dump(self.db), before, "--undo is not exact")

    def test_a_table_the_server_made_stays(self):
        world(self.db, npctint=True)
        before = dump(self.db)
        run(self.db, "--apply")
        self.assertEqual(tints(self.db), {"Yhafiran": (TINT, "seed")})
        run(self.db, "--undo")
        self.assertEqual(dump(self.db), before, "the server's empty table was dropped or changed")

    def test_a_gms_row_is_left(self):
        world(self.db, npctint=True, rows=[("yhafiran", 0x112233FF, "a GM's", "gm")])
        before = dump(self.db)
        out = run(self.db, "--apply")
        self.assertIn("left: Yhafiran", out)
        self.assertIn("nothing to write", out)
        self.assertEqual(dump(self.db), before)
        # the control: no row of the name, and one is written
        world(os.path.join(self.dir.name, "other.db"), npctint=True, rows=[("Uzrathixius", 0x112233FF, "", "gm")])
        run(os.path.join(self.dir.name, "other.db"), "--apply")
        self.assertEqual(tints(os.path.join(self.dir.name, "other.db"))["Yhafiran"], (TINT, "seed"))

    def test_a_row_changed_since_is_left(self):
        world(self.db, npctint=True)
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE npctint SET color = ? WHERE name = 'Yhafiran'", (0x445566FF,))
        db.commit()
        db.close()
        self.assertIn("a row a GM set or changed", run(self.db, "--apply"))
        out = run(self.db, "--undo")
        self.assertIn("changed since, left as it is", out)
        self.assertEqual(tints(self.db), {"Yhafiran": (0x445566FF, "seed")})

    def test_a_new_tint_of_the_tools_goes_over_its_own(self):
        world(self.db)
        before = dump(self.db)
        run(self.db, "--apply")
        old = M.TINTS
        try:
            M.TINTS = [(old[0][0], 0x4D804DFF, "another")]
            db = sqlite3.connect(self.db, isolation_level=None)
            writes, left = M.work(db)
            self.assertEqual(([(w[0], w[2]) for w in writes], left), ([("Yhafiran", 0x4D804DFF)], []))
            M.apply(db, writes)
            db.close()
            self.assertEqual(tints(self.db), {"Yhafiran": (0x4D804DFF, "seed")})
        finally:
            M.TINTS = old
        run(self.db, "--undo")
        self.assertEqual(dump(self.db), before, "--undo through two writes is not exact")

    def test_key(self):
        self.assertEqual(M.key("  Lady Vox$ "), "lady vox")
        self.assertEqual(M.key("Yhafiran"), "yhafiran")


if __name__ == "__main__":
    unittest.main()
