#!/usr/bin/env python3
"""jib_lane3_20261122.py on a small database: both parts' plans, an apply, a second apply, an exact undo, a row
changed since refused by the undo, and the controls (a plain trade-0 record held is never moved; a Super SoW row that
reads otherwise is left; a shard without heroic Guk or a look record left alone)."""
import hashlib
import io
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "jib_lane3_20261122.py")

ITEM_COLS = ("patternid INTEGER PRIMARY KEY, itemcost INTEGER, unk1 INTEGER, itemicon INTEGER, unk2 INTEGER, equipslot INTEGER, "
             "unk3 INTEGER, trade INTEGER, rent INTEGER, unk4 INTEGER, attacktype INTEGER, weapondamage INTEGER, unk5 INTEGER, "
             "levelreq INTEGER, maxstack INTEGER, maxhp INTEGER, duration INTEGER, classuse INTEGER, raceuse INTEGER, "
             "procanim INTEGER, lore INTEGER, unk6 INTEGER, craft INTEGER, itemname TEXT, itemdesc TEXT, model INTEGER, "
             "color INTEGER, str INTEGER, sta INTEGER, agi INTEGER, wis INTEGER, dex INTEGER, cha INTEGER, intelligence INTEGER, "
             "HPMAX INTEGER, POWMAX INTEGER, PoT INTEGER, HoT INTEGER, AC INTEGER, PR INTEGER, DR INTEGER, FR INTEGER, CR INTEGER, "
             "LR INTEGER, AR INTEGER, weaponproc INTEGER, fish INTEGER")
SCHEMA = [
    f"CREATE TABLE itempattern ({ITEM_COLS})",
    """CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
        icon INTEGER NOT NULL DEFAULT 0, breaks TEXT NOT NULL DEFAULT '', wear INTEGER NOT NULL DEFAULT 0, area REAL NOT NULL DEFAULT 0,
        source TEXT NOT NULL DEFAULT 'gm', body INTEGER NOT NULL DEFAULT 0, target_line TEXT NOT NULL DEFAULT '',
        charges INTEGER NOT NULL DEFAULT 0, cooldown REAL NOT NULL DEFAULT 0)""",
    "CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')",
    "CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')",
    "CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER)",
    """CREATE TABLE auction_lots (lot INTEGER PRIMARY KEY AUTOINCREMENT, patternid INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'open',
        delivered INTEGER NOT NULL DEFAULT 0)""",
    "CREATE TABLE charbuyback (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER)",
    "CREATE TABLE charmercitem (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER)",
    "CREATE TABLE mail_attachments (id INTEGER PRIMARY KEY AUTOINCREMENT, patternid INTEGER NOT NULL DEFAULT 0, itemname TEXT NOT NULL DEFAULT '')",
    """CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25, rolls INTEGER NOT NULL DEFAULT 1,
        note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (npc_name, tablename))""",
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, y_coord REAL)",
    """CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL, username TEXT NOT NULL,
        via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '',
        before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER)""",
]


def item(db, pid, name, trade=0, **kw):
    v = {"patternid": pid, "itemname": name, "trade": trade, "itemcost": 10, "itemicon": 5, "equipslot": 5, "rent": 1,
         "maxstack": 1, "levelreq": 10, "classuse": 32767, "raceuse": 1023, "model": 7, "color": 1, "str": 9, "lore": 0}
    v.update(kw)
    db.execute(f"INSERT INTO itempattern ({', '.join(v)}) VALUES ({', '.join('?' for _ in v)})", list(v.values()))


def make(path, guk=True, sow_line="Self, Sow, 30min", sash_look=True):
    db = sqlite3.connect(path)
    for s in SCHEMA:
        db.execute(s)
    item(db, 100, "Test Blade")                      # a base
    item(db, 980001, "Ascended Test Blade")          # its Ascended copy, trade 0, held
    item(db, 980002, "Ascended Test Ring", trade=1)  # already NO TRADE: not T
    item(db, 980003, "Ascended Test Cap")            # Ascended, trade 0, held by no one
    item(db, 985115, "Ascended Test Scroll")         # named only, trade 0, in a lot
    item(db, 991241, "Ascended Test Blade (Imported)")  # an import of an Ascended record, in a buyback
    item(db, 200, "Plain Held Thing")                # the control: trade 0, not Ascended, held
    item(db, 8852, "Look Guise", itemicon=11)
    item(db, 705311, "Look Stone", itemicon=12)
    item(db, 8737, "Look Knife", itemicon=13, model=14)
    if sash_look:
        item(db, 960012, "Look Sash", itemicon=15)
    db.executemany("INSERT INTO itemascended (patternid, base) VALUES (?, ?)", [(980001, 100), (980002, 101), (980003, 102)])
    db.execute("INSERT INTO itemimported (patternid, base) VALUES (991241, 980001)")
    db.execute("INSERT INTO itemeffect (patternid, line, name) VALUES (980001, 'Self, Increase STR by 11, 1min', 'Test Use')")
    db.execute("INSERT INTO itemeffect (patternid, line, name, icon, cooldown) VALUES (705311, 'x', 'Mana Stone', 77, 0)")
    if sow_line is not None:
        db.execute("INSERT INTO itemeffect (patternid, line, name, icon, breaks, charges) VALUES (990988, ?, 'Super SoW', 1, 'attack', 99)",
                   (sow_line,))
    db.executemany("INSERT INTO charinventory (serverid, patternid) VALUES (?, ?)", [(1, 980001), (2, 980001), (1, 200), (3, 100)])
    db.execute("INSERT INTO auction_lots (patternid, state, delivered) VALUES (985115, 'open', 0), (980001, 'sold', 1)")
    db.execute("INSERT INTO charbuyback (serverid, patternid) VALUES (4, 991241)")
    db.execute("INSERT INTO mail_attachments (patternid, itemname) VALUES (980001, 'Ascended Test Blade')")
    if guk:
        db.executemany("INSERT INTO npcs VALUES (?, ?, 14, ?)",
                       [(166246, "froglok king", -33.0), (165977, "Trub", 40.0), (166029, "Vahk'Kiv", -9.0),
                        (170001, "a froglok", 50.0), (170002, "a froglok", 10.0), (170003, "an era named", 10.0)])
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance) VALUES ('#166246', 'heroic guk fungal spore tunic', 2.222)")
        db.executemany("INSERT INTO npcloot (npc_name, tablename, chance) VALUES (?, 'dungeon guk named', ?)",
                       [("#165977", 5), ("#166029", 5), ("#170001", 1), ("#170002", 1.111), ("#170003", 5), ("#166246", 40)])
    db.commit()
    db.close()


def run(path, part, *args):
    r = subprocess.run([sys.executable, "-I", TOOL, "--db", path, "--part", part, *args], capture_output=True, text=True)
    return r.returncode, r.stdout + r.stderr


def digest(path):
    db = sqlite3.connect(path)
    h = hashlib.sha256()
    for t in ("itempattern", "itemeffect", "charinventory", "auction_lots", "charbuyback", "mail_attachments", "npcloot"):
        for r in db.execute(f"SELECT * FROM {t} ORDER BY 1, 2"):
            h.update(repr((t, r)).encode())
    for t in ("itemlegacy", "itemspeed"):
        if db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (t,)).fetchone():
            for r in db.execute(f"SELECT * FROM {t} ORDER BY 1"):
                h.update(repr((t, r)).encode())
    db.close()
    return h.hexdigest()


class Lane3(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_before_moves_the_held_ascended_copies_to_legacy(self):
        make(self.path)
        start = digest(self.path)
        code, out = run(self.path, "before")
        self.assertEqual(code, 0, out)
        self.assertEqual(digest(self.path), start, "the report wrote")
        code, out = run(self.path, "before", "--apply")
        self.assertEqual(code, 0, out)
        legacy = dict(self.q("SELECT record, patternid FROM itemlegacy"))
        self.assertEqual(sorted(legacy), [980001, 985115, 991241])  # held; 980003 held by no one; 980002 NO TRADE already
        blade = legacy[980001]
        self.assertEqual(self.q("SELECT itemname, trade, str, model FROM itempattern WHERE patternid = ?", blade),
                         [("Legacy Ascended Test Blade", 0, 9, 7)])
        self.assertEqual(self.q("SELECT line, name FROM itemeffect WHERE patternid = ?", blade),
                         [("Self, Increase STR by 11, 1min", "Test Use")])
        self.assertEqual(self.q("SELECT patternid FROM charinventory ORDER BY itemid"), [(blade,), (blade,), (200,), (100,)])
        self.assertEqual(self.q("SELECT patternid FROM auction_lots ORDER BY lot"), [(legacy[985115],), (980001,)])  # a sold lot stays
        self.assertEqual(self.q("SELECT patternid FROM charbuyback"), [(legacy[991241],)])
        self.assertEqual(self.q("SELECT patternid, itemname FROM mail_attachments"), [(blade, "Legacy Ascended Test Blade")])
        # T's unlisted records NO TRADE; the listed ones are the server's to do at its start; the base and the control alone
        trade = dict(self.q("SELECT patternid, trade FROM itempattern"))
        self.assertEqual((trade[985115], trade[991241], trade[980001], trade[980003], trade[100], trade[200]), (1, 1, 0, 0, 0, 0))
        self.assertGreater(self.q("SELECT count(*) FROM gm_audit")[0][0], 0)
        applied = digest(self.path)
        code, out = run(self.path, "before", "--apply")
        self.assertIn("0 rows", out)
        self.assertEqual(digest(self.path), applied, "a second apply wrote")
        code, out = run(self.path, "before", "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(digest(self.path), start, "the undo is not exact")

    def test_after_sets_sow_the_guk_items_and_the_tunic(self):
        make(self.path)
        start = digest(self.path)
        code, out = run(self.path, "after", "--apply")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.q("SELECT line, name, charges FROM itemeffect WHERE patternid = 990988"),
                         [("Target, Increase Movement Speed by 260, 30min", "Super SoW", 99)])
        rows = self.q("SELECT patternid, itemname, equipslot, trade, lore, levelreq, str, itemicon FROM itempattern WHERE patternid > 968000 "
                      "AND patternid < 969000 ORDER BY patternid")
        self.assertEqual(rows, [(968055, "Guise of the Believer", -1, 1, 1, 60, 0, 11), (968058, "Powerstone", -1, 0, 0, 60, 0, 12),
                                (968059, "Bag of Open Kind Eye", 16, 0, 1, 60, 0, 13),
                                (968062, "Flowing Black Satin Sash", 9, 0, 0, 60, 25, 15)])
        self.assertEqual(self.q("SELECT attacktype, weapondamage, unk5, dex, model FROM itempattern WHERE patternid = 968059"),
                         [(10, 761, 25, 20, 14)])
        self.assertEqual(self.q("SELECT line, icon, cooldown FROM itemeffect WHERE patternid = 968058"),
                         [("Self, Decrease Hitpoints by 300, Increase Power by 100, Unlimited Uses", 77, 6.0)])
        self.assertEqual(self.q("SELECT patternid, pct FROM itemspeed"), [(968062, 7.0)])
        chances = dict(self.q("SELECT npc_name, chance FROM npcloot WHERE tablename = 'heroic guk fungal spore tunic'"))
        self.assertEqual(chances, {"#166246": 2.778, "#165977": 0.5, "#166029": 0.556, "#170001": 0.05, "#170002": 0.056})
        applied = digest(self.path)
        code, out = run(self.path, "after", "--apply")
        self.assertIn("0 rows", out)
        self.assertEqual(digest(self.path), applied)
        # a row changed since: the undo refuses and writes nothing
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itemspeed SET pct = 8 WHERE patternid = 968062")
        db.commit()
        db.close()
        changed = digest(self.path)
        code, out = run(self.path, "after", "--undo")
        self.assertNotEqual(code, 0)
        self.assertIn("changed since", out)
        self.assertEqual(digest(self.path), changed)
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itemspeed SET pct = 7 WHERE patternid = 968062")
        db.commit()
        db.close()
        code, out = run(self.path, "after", "--undo")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.path)
        db.execute("DROP TABLE itemspeed")  # made by this tool (the server makes it too); empty after the undo
        db.commit()
        db.close()
        self.assertEqual(digest(self.path), start, "the undo is not exact")

    def test_the_controls(self):
        make(self.path, guk=False, sow_line="Self, Something Else, 30min", sash_look=False)
        code, out = run(self.path, "after", "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("not 'Self, Sow, 30min'", out)
        self.assertIn("no heroic Guk", out)
        self.assertIn("look record 960012 missing", out)
        self.assertEqual(self.q("SELECT line FROM itemeffect WHERE patternid = 990988"), [("Self, Something Else, 30min",)])
        self.assertEqual(self.q("SELECT count(*) FROM npcloot"), [(0,)])
        self.assertEqual(self.q("SELECT count(*) FROM itempattern WHERE patternid = 968062"), [(0,)])


if __name__ == "__main__":
    unittest.main()
