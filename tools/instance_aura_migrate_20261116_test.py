#!/usr/bin/env python3
"""instance_aura_migrate_20261116.py on a small database: refused without instanceaura; the report writes nothing;
--apply moves each hand row onto the copy held there (a merc's too, its key -id), tags it, deletes the charaura row,
keeps a hand that holds nothing and a copy with a row already, carries a charaura-two second, stacks Dust's twinkle
where his row has none, tags a single holder's record-aura copies and lists a shared record; a second apply finds
nothing; --undo puts every row back exactly; a row changed since is left by --undo and listed."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "instance_aura_migrate_20261116.py")
sys.path.insert(0, HERE)
import instance_aura_migrate_20261116 as tool  # noqa: E402


def make(path, second_cols=False, instance=True):
    db = sqlite3.connect(path)
    extra = ", color2 INTEGER NOT NULL DEFAULT 0, kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0, " \
            "nodes2 INTEGER NOT NULL DEFAULT 0" if second_cols else ""
    db.executescript(f"""
        CREATE TABLE characters (charName TEXT NOT NULL, accountid INTEGER NOT NULL, serverid INTEGER PRIMARY KEY);
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER);
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER);
        CREATE TABLE charmercitem (id INTEGER PRIMARY KEY, serverid INTEGER, slot INTEGER, patternid INTEGER, equiploc INTEGER);
        CREATE TABLE charaura (charid INTEGER NOT NULL, merc INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0, source INTEGER NOT NULL DEFAULT 0,
            nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT ''{extra}, PRIMARY KEY (charid, merc, slot));
        CREATE TABLE itemaura (patternid INTEGER PRIMARY KEY, color INTEGER NOT NULL DEFAULT 0, kind INTEGER NOT NULL DEFAULT 0,
            source INTEGER NOT NULL DEFAULT 0, nodes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '',
            color2 INTEGER NOT NULL DEFAULT 0, kind2 INTEGER NOT NULL DEFAULT 0, source2 INTEGER NOT NULL DEFAULT 0, nodes2 INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO characters VALUES ('Dust', 1, 197), ('Zaeire', 2, 193), ('Drizzt', 3, 327), ('Other', 4, 400), ('Jibwiz', 5, 791);
        INSERT INTO itempattern VALUES (714382, 'Heirloom Sword of Twisted Virtue', 14), (714100, 'Heirloom Dagger', 14),
            (715040, 'Twinkle', 14), (9, 'Shared Blade', 14), (711000, 'Ornate Breastplate', 5), (30, 'Staff', 14);
        INSERT INTO charinventory VALUES (1, 197, 714382, 14, 0), (2, 193, 714100, 14, 0), (3, 327, 715040, 14, 0),
            (4, 197, 9, -1, 0), (5, 400, 9, -1, 0), (6, 197, 711000, 5, 0), (7, 791, 30, 14, 0), (8, 193, 714100, 15, 1);
        INSERT INTO charmercitem VALUES (11, 197, 1, 714100, 16);
        INSERT INTO charaura (charid, merc, slot, color, kind, source, nodes) VALUES
            (197, 0, 9, 255, 3, 0, 0), (193, 0, 9, 0, 7, 1607773820, 0), (193, 0, 10, 0, 7, 1607773820, 0),
            (197, 1, 9, 0, 3, 1813322336, 0), (791, 0, 9, 0, 3, 1813322336, 0), (197, 0, 0, 255, 5, 0, 0);
        INSERT INTO itemaura (patternid, color, kind, source, kind2, source2, nodes2) VALUES
            (715040, 1856569343, 7, 2297170221, 3, 495410893, 100), (9, 0, 3, 0, 0, 0, 0), (711000, 255, 5, 0, 0, 0, 0);
    """)
    if instance:
        db.execute(tool.INSTANCE_DDL)
        db.execute("INSERT INTO instanceaura (itemid, tag) VALUES (7, 'GM tag')")  # Jibwiz's staff: a row already
    if second_cols:
        db.execute("UPDATE charaura SET kind2 = 3, source2 = 1, nodes2 = 50 WHERE charid = 193 AND slot = 9")
    db.commit()
    db.close()


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    try:
        out = {}
        for t in ("charaura", "instanceaura", "charinventory", "itemaura"):
            try:
                out[t] = db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall()
            except sqlite3.OperationalError:
                out[t] = None
        return out
    finally:
        db.close()


class Migrate(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_refused_without_the_table(self):
        make(self.db, instance=False)
        before = dump(self.db)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout)
        self.assertIn("REFUSED", r.stdout)
        self.assertEqual(dump(self.db), before)

    def test_report_apply_undo(self):
        make(self.db)
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before, "the report wrote")
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("KEPT  Zaeire (193) off hand: holds nothing", r.stdout)  # her off hand's copy is in the bank
        self.assertIn("has an instanceaura row already", r.stdout)
        self.assertIn("SHARED Shared Blade (9)", r.stdout)
        db = sqlite3.connect(self.db)
        inst = {r[0]: r for r in db.execute("SELECT itemid, tag, color, kind, source, nodes, color2, kind2, source2, nodes2, source_tool FROM instanceaura")}
        aura = {(r[0], r[1], r[2]) for r in db.execute("SELECT charid, merc, slot FROM charaura")}
        audits = db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0]
        db.close()
        self.assertEqual(inst[1][1:10], ("Shadowed, Twinkling", 255, 3, 0, 0, 0, 3, 0x1D875ECD, 100))  # Dust's twinkle stacked
        self.assertEqual(inst[2][1:5], ("Tainted", 0, 7, 1607773820))
        self.assertEqual(inst[-11][1:5], ("Freezing", 0, 3, 1813322336))  # Dust's merc's copy
        self.assertEqual(inst[3][1:4], ("Virtuous, Twinkling", 0, 0))  # Twinkle: a tag alone
        self.assertEqual(inst[7][1], "GM tag")  # untouched
        self.assertNotIn(4, inst)  # the shared record: nothing
        self.assertNotIn(6, inst)  # an armour piece's record aura: not a held item's
        self.assertEqual(aura, {(193, 0, 10), (791, 0, 9), (197, 0, 0)})
        self.assertEqual(audits, 3 * 2 + 1)  # three moved, one tagged
        self.assertIn("nothing to do", run(self.db).stdout)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        after = dump(self.db)
        self.assertEqual(after["charaura"], before["charaura"])
        self.assertEqual(after["instanceaura"], before["instanceaura"])

    def test_second_columns_carried(self):
        make(self.db, second_cols=True)
        self.assertEqual(run(self.db, "--apply").returncode, 0)
        db = sqlite3.connect(self.db)
        r = db.execute("SELECT tag, kind2, source2, nodes2 FROM instanceaura WHERE itemid = 2").fetchone()
        db.close()
        self.assertEqual(r, ("Tainted, Enchanted", 3, 1, 50))

    def test_undo_leaves_a_changed_row(self):
        make(self.db)
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE instanceaura SET tag = 'Changed' WHERE itemid = 1")
        db.commit()
        db.close()
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn("instanceaura 1: changed since", r.stdout)

    def test_tags(self):
        self.assertEqual(tool.tag_of((0x000000FF, 3, 0), None), "Shadowed")
        self.assertEqual(tool.tag_of((0, 7, 0xCBF4135A), None), "Druid Glow")
        self.assertEqual(tool.tag_of((0x3060FFFF, 7, 0x88EC092D), (0, 3, 0x6C151A60)), "Virtuous, Freezing")
        self.assertEqual(tool.tag_of((0, 9, 0x2EC56029), None), "Energized")
        self.assertEqual(tool.tag_of((0, 5, 0), None), "")


if __name__ == "__main__":
    unittest.main()
