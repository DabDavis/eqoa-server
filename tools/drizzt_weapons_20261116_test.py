#!/usr/bin/env python3
"""drizzt_weapons_20261116.py on a small database: the report writes nothing; --apply gives Twinkle and Icingdeath the
Scimitar of Fury's model and icon (Twinkle plain, Icingdeath blue; every other column as it was) and their itemaura
rows (the druid staff glow, the freezing drip), each with a gm_audit line; a second apply finds nothing to do; --undo
puts back exactly what was there, an aura row a GM had made before included. The controls: two Twinkles, a Twinkle that
is no one-handed slashing weapon, or no record of the Fury's look refuse the whole, writing nothing."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "drizzt_weapons_20261116.py")
FURY, ICON = 0xAB6B7D29, 1086674104


def signed(v):
    return v - (1 << 32) if v >= 1 << 31 else v


def make(path, twins=False, fury=True, twinkle_slot=14, prior_aura=False):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE characters (charName TEXT NOT NULL, accountid INTEGER NOT NULL, serverid INTEGER PRIMARY KEY);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, itemicon INTEGER, model INTEGER,
            color INTEGER, equipslot INTEGER, attacktype INTEGER, weapondamage INTEGER);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, equiploc INTEGER);
        CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
            source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO characters VALUES ('Drizzt', 98, 327);
    """)
    db.execute("INSERT INTO itempattern VALUES (715040, 'Twinkle', 'A slender scimitar.', -1728866781, ?, 1249640447, ?, 1, 801)",
               (signed(0x8E85E4A6), twinkle_slot))
    db.execute("INSERT INTO itempattern VALUES (715041, 'Icingdeath', 'A scimitar.', 445518418, 1205436896, 3606249471, 14, 1, 801)")
    if fury:
        db.execute("INSERT INTO itempattern VALUES (701006, 'Scimitar of Fury', '', ?, ?, 4294967295, 12, 1, 700)", (ICON, signed(FURY)))
    if twins:
        db.execute("INSERT INTO itempattern VALUES (715099, 'Twinkle', '', 1, 1, 0, 14, 1, 1)")
    if prior_aura:
        db.execute("INSERT INTO itemaura VALUES (715041, 4278190335, 2, 0, 0, 'a GM''s red shell')")
    db.executemany("INSERT INTO charinventory VALUES (?, 327, ?, ?)", [(1, 715040, 14), (2, 715041, 15)])
    db.commit()
    db.close()


def run(path, *args):
    p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def snapshot(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("itempattern", "itemaura")}
    db.close()
    return out


class DrizztWeapons(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_report_apply_again_undo(self):
        make(self.db)
        first = snapshot(self.db)
        rc, out = run(self.db)
        self.assertEqual(rc, 0, out)
        self.assertIn("report only: 4 rows", out)
        self.assertEqual(snapshot(self.db), first)
        rc, out = run(self.db, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.db)
        items = {r[0]: r[1:] for r in db.execute("SELECT patternid, itemicon, model, color, itemname, itemdesc, weapondamage FROM itempattern")}
        self.assertEqual(items[715040], (ICON, signed(FURY), 0xFFFFFFFF, "Twinkle", "A slender scimitar.", 801))
        self.assertEqual(items[715041], (ICON, signed(FURY), 0x3060FFFF, "Icingdeath", "A scimitar.", 801))
        auras = {r[0]: r[1:5] for r in db.execute("SELECT patternid, color, kind, source, nodes FROM itemaura")}
        self.assertEqual(auras, {715040: (0, 7, 0xCBF4135A, 45), 715041: (0x3060FFFF, 3, 0x6C151A60, 0)})
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE username = 'dab' AND via = 'tool'").fetchone()[0], 4)
        db.close()
        rc, out = run(self.db, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to do", out)
        rc, out = run(self.db, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(snapshot(self.db), first)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 8)
        self.assertIsNone(db.execute("SELECT 1 FROM sqlite_master WHERE name = 'drizzt_weapons_1116_log'").fetchone())
        db.close()

    def test_a_gms_aura_row_is_put_back(self):
        make(self.db, prior_aura=True)
        first = snapshot(self.db)
        self.assertEqual(run(self.db, "--apply")[0], 0)
        self.assertEqual(run(self.db, "--undo")[0], 0)
        self.assertEqual(snapshot(self.db), first)

    def test_a_row_changed_since_is_left(self):
        make(self.db)
        self.assertEqual(run(self.db, "--apply")[0], 0)
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itemaura SET color = 1 WHERE patternid = 715041")
        db.commit()
        db.close()
        rc, out = run(self.db, "--undo")
        self.assertEqual(rc, 1, out)
        self.assertIn("changed since", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT color FROM itempattern WHERE patternid = 715040").fetchone()[0], 1249640447)
        db.close()

    def test_refusals_write_nothing(self):
        for kw in ({"twins": True}, {"fury": False}, {"twinkle_slot": 15}):
            with self.subTest(**kw):
                if os.path.exists(self.db):
                    os.remove(self.db)
                make(self.db, **kw)
                first = snapshot(self.db)
                rc, out = run(self.db, "--apply")
                self.assertEqual(rc, 1, out)
                self.assertIn("REFUSED", out)
                self.assertEqual(snapshot(self.db), first)


if __name__ == "__main__":
    unittest.main()
