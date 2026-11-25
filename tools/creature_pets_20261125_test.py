#!/usr/bin/env python3
"""tools/creature_pets_20261125.py: placed pets pair with their owners (today's rule, then strays within 25, a name
taken by its share or a caster's word), npcpets rows by owner name (and by row for a creature whose pets differ or have
rules of their own), one row of a look kept as the copy (spawn-only by row), the rest deleted with their rule rows, the
owners' class and caster template set; a second apply writes nothing, a pet named by a path refuses everything, and
--undo puts every table back exactly.

    python3 -E -s tools/creature_pets_20261125_test.py
"""
import contextlib
import io
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import creature_pets_20261125 as cp  # noqa: E402
import tix4_rows as tr  # noqa: E402

TABLES = ("npcs", "npcspawnonly", "npcstands", "npcclass", "npctemplate", "classtemplates", "npcrespawn", "npcwander",
          "campspawns", "npcloot", "npcpaths", "npcpets")


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL,
            y_coord REAL, z_coord REAL, facing INTEGER, npc_level INTEGER, npc_type INTEGER, modelid INTEGER,
            size REAL, dt_kill TEXT);
        CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcstands (npc_row INTEGER PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
        CREATE TABLE npctemplate (name TEXT PRIMARY KEY, template TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1, hp_pct INTEGER NOT NULL
            DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100, special TEXT NOT NULL DEFAULT '', special_every REAL NOT
            NULL DEFAULT 0, abilities TEXT NOT NULL DEFAULT '[]');
        CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcwander (npcid INTEGER PRIMARY KEY, radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE campspawns (npcid INTEGER PRIMARY KEY, camp INTEGER NOT NULL DEFAULT 0, kinds TEXT NOT NULL DEFAULT '');
        CREATE TABLE npcloot (npc_name TEXT NOT NULL, itemid INTEGER NOT NULL, chance REAL NOT NULL DEFAULT 0);
        CREATE TABLE npcpaths (npcid INTEGER PRIMARY KEY, path TEXT NOT NULL DEFAULT '');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO classtemplates (name, class, abilities) VALUES ('shaman', -1, '[{"kind":"nuke"}]');
        INSERT INTO classtemplates (name, class, hp_pct, abilities) VALUES ('necromancer', 11, 90, '[{"kind":"lifetap"}]');
        INSERT INTO classtemplates (name, class, special, special_every) VALUES ('roamer', 0, 'Critical Hit', 30);
        INSERT INTO classtemplates (name, class) VALUES ('chain wizard', 13);
        INSERT INTO npcclass VALUES ('Froglok Shaman', 0, 'seed');
        INSERT INTO npctemplate VALUES ('froglok shaman', 'roamer', '', 'seed');
        INSERT INTO npctemplate VALUES ('ice goblin wizard', 'chain wizard', '', 'seed');
    """)
    spirit, minion = 2118838847, 465275276
    rows = [
        # three froglok shamans: a spirit 1.5 off one, a spirit 6 off another (drift: a stray), none by the third
        (10, "froglok shaman", 100, 100, 20, 0), (11, "Companion Spirit", 101.5, 100, 17, spirit),
        (12, "froglok shaman", 300, 300, 22, 0), (13, "Companion Spirit", 306, 300, 19, spirit),
        (14, "froglok shaman", 500, 500, 21, 0),
        # a froglok shaman whose spirit has loot of its own by its row: its own copy, by its row
        (15, "froglok shaman", 700, 700, 20, 0), (16, "Companion Spirit", 701, 700, 17, spirit),
        # a necromancer and its minion
        (20, "dshinn necromancer", 900, 900, 43, 0), (21, "spectral minion", 901.4, 900, 39, minion),
        # a wizard with a spirit: its template's class wins
        (25, "ice goblin wizard", 1100, 1100, 30, 0), (26, "Vigilant Spirit", 1101, 1100, 24, spirit),
        # ten gnoll warriors, one beside a stray minion: no caster (1 of 10), the minion left a row
        (40, "gnoll warrior", 2000, 2000, 10, 0), (41, "decaying minion", 2001, 2000, 10, minion),
        # a spirit with no one near: left
        (50, "Feral Spirit", 5000, 5000, 12, spirit),
    ] + [(60 + i, "gnoll warrior", 2100 + 20 * i, 2100, 10, 0) for i in range(9)]
    for i, name, x, z, lvl, model in rows:
        db.execute("INSERT INTO npcs VALUES (?, ?, 0, ?, 5, ?, 0, ?, 2, ?, 1.0, NULL)", (i, name, x, z, lvl, model))
    db.execute("INSERT INTO npcrespawn VALUES (13, 90, '', 'gm')")
    db.execute("INSERT INTO npcwander VALUES (13, -1, '', 'gm')")
    db.execute("INSERT INTO npcloot VALUES ('#16', 5, 10)")
    db.commit()
    db.close()


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = tr.main(cp.SPEC, list(args), doc=cp.__doc__)
    return code, out.getvalue()


def digests(path):
    db = sqlite3.connect(path)
    try:
        out = {}
        for t in TABLES:
            if tr.table(db, t):
                out[t] = sorted(db.execute(f"SELECT * FROM {t}").fetchall(), key=repr)
        return out
    finally:
        db.close()


class Test(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        make(self.db)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def q(self, sql, args=()):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        before = digests(self.db)
        code, out = run("--db", self.db)
        self.assertEqual(code, 0, out)
        self.assertEqual(digests(self.db), before, "a dry run writes nothing")
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        pets = {(o, t): (p, n, off) for o, p, t, n, off in
                self.q("SELECT owner, pet, template_row, count, level_offset FROM npcpets")}
        self.assertEqual(pets, {
            ("froglok shaman", 11): ("companion spirit", 1, -3),
            ("#15", 16): ("companion spirit", 1, -3),
            ("dshinn necromancer", 21): ("spectral minion", 1, -4),
            ("ice goblin wizard", 26): ("vigilant spirit", 1, -6),
        })
        left = {r[0] for r in self.q("SELECT id FROM npcs")}
        self.assertNotIn(13, left, "the stray spirit 6 off its shaman goes")
        self.assertTrue({11, 16, 21, 26} <= left, "the copies stay")
        self.assertTrue({41, 50} <= left, "a non-caster's stray minion and a lone spirit stay rows")
        self.assertEqual(self.q("SELECT count(*) FROM npcrespawn")[0][0] + self.q("SELECT count(*) FROM npcwander")[0][0], 0,
                         "the deleted pet's rule rows go with it")
        self.assertEqual({r[0] for r in self.q("SELECT name FROM npcspawnonly")}, {"#11", "#16", "#21", "#26"})
        self.assertEqual(self.q("SELECT class FROM npcclass WHERE name = 'Froglok Shaman'"), [(8,)])
        self.assertEqual(self.q("SELECT class FROM npcclass WHERE name = 'dshinn necromancer'"), [(11,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcclass WHERE name = 'ice goblin wizard'"), [(0,)],
                         "a chain wizard's class wins: no class row written")
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = 'froglok shaman'"), [("shaman",)])
        self.assertEqual(self.q("SELECT template FROM npctemplate WHERE name = 'dshinn necromancer'"), [("necromancer",)])
        self.assertEqual(self.q("SELECT count(*) FROM npctemplate WHERE name = 'gnoll warrior'"), [(0,)])
        applied = digests(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("applied already", out)
        self.assertEqual(digests(self.db), applied)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        after = digests(self.db)
        self.assertEqual(after.pop("npcpets"), [], "npcpets emptied (the server makes the table)")
        self.assertEqual(after, before)
        self.assertEqual(self.q(f"SELECT count(*) FROM sqlite_master WHERE name = '{cp.LOG}'"), [(0,)])

    def test_a_pet_on_a_path_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("INSERT INTO npcpaths VALUES (13, 'x')")
        db.commit()
        db.close()
        before = digests(self.db)
        code, out = run("--db", self.db, "--apply")
        self.assertEqual(code, 1, out)
        self.assertIn("#13", out)
        self.assertIn("npcpaths", out)
        self.assertEqual(digests(self.db), before)

    def test_pairing_rules(self):
        self.assertTrue(cp.pet_rule("Goran's pet")[0])
        self.assertEqual(cp.pet_rule("Goran's pet")[1], "Goran")
        self.assertFalse(cp.pet_rule("village pet")[0])
        self.assertFalse(cp.pet_rule("Blood Thirsty Minion")[0])
        self.assertTrue(cp.pet_rule("elder spirit")[2], "an elder spirit is a pet only beside a shaman")
        self.assertTrue(cp.names_owner("Tal Thir", "Tal'Thir"))
        self.assertEqual(cp.pet_class("kindred spirit"), cp.SHAMAN)
        self.assertEqual(cp.pet_class("fire elementalkin"), cp.MAGICIAN)
        self.assertEqual(cp.pet_class("mummified guardian"), cp.NECROMANCER)
        self.assertIsNone(cp.pet_class("servant of vorin"))


if __name__ == "__main__":
    unittest.main()
