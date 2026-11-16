#!/usr/bin/env python3
"""cazic_loot_iod_bosses_20261116.py on a small database: the report writes nothing; --apply makes the three records,
the three tables and bindings, unbinds the wiki table and sets the two templates; a second --apply does nothing;
--undo puts every row back exactly; an Ascended copy in the wiki table (work board 1536 applied first) is carried
over; a table not as expected is refused whole."""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import cazic_loot_iod_bosses_20261116 as cl  # noqa: E402

ITEM_COLS = ("patternid", "itemcost", "itemicon", "equipslot", "trade", "rent", "levelreq", "classuse", "lore",
             "itemname", "itemdesc", "model", "weapondamage") + cl.STATS
SCHEMA = f"""
CREATE TABLE itempattern ({", ".join(f'"{c}"' + (" INTEGER PRIMARY KEY" if c == "patternid" else "") for c in ITEM_COLS)});
CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT, source TEXT);
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT NOT NULL, patternid INTEGER NOT NULL,
                         weight INTEGER NOT NULL DEFAULT 1, minlevel INTEGER NOT NULL DEFAULT 1,
                         maxlevel INTEGER NOT NULL DEFAULT 60, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
                      rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
                      PRIMARY KEY (npc_name, tablename));
CREATE TABLE classtemplates (name TEXT PRIMARY KEY, hp_pct INTEGER, taken_pct INTEGER, note TEXT NOT NULL DEFAULT '');
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_level INTEGER);
CREATE TABLE balancelevel (level INTEGER PRIMARY KEY, hp_pct REAL);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY, serverid INTEGER, patternid INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
                       username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL,
                       tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT,
                       detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""
TABLES = ("itempattern", "loottables", "npcloot", "classtemplates")


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = cl.main(list(args))
    return rc, out.getvalue()


def item(db, pid, name):
    r = {c: 0 for c in ITEM_COLS}
    r.update(patternid=pid, itemname=name, itemdesc="", levelreq=60, rent=1, classuse=32767, str=63)
    db.execute(f'INSERT INTO itempattern ({", ".join(chr(34) + c + chr(34) for c in r)}) VALUES ({", ".join("?" * len(r))})',
               list(r.values()))


class CazicLootTest(unittest.TestCase):
    def make(self):
        fd, path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        self.addCleanup(os.remove, path)
        db = sqlite3.connect(path)
        db.executescript(SCHEMA)
        ids = list(cl.WIKI_BASES) + list(cl.OTHER_CLASS) + cl.MISC + [s for s, *_ in cl.MADE.values()]
        for pid in ids:
            item(db, pid, f"item {pid}")
        for pid in cl.WIKI_BASES:
            db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                       "VALUES (?, ?, 100, 1, 100, 'wiki')", (cl.WIKI, pid))
        db.execute("INSERT INTO npcloot VALUES (?, ?, 100, 1, 'the wiki', 'wiki')", (cl.NPC, cl.WIKI))
        db.execute("INSERT INTO classtemplates VALUES ('iod impale', 655, 100, 'x')")
        db.execute("INSERT INTO classtemplates VALUES ('iod contagion', 819, 100, 'y')")
        db.execute("INSERT INTO npcs VALUES (1, 'Impale', 55), (2, 'Contagion', 55), (3, 'Contagion', 63)")
        db.executemany("INSERT INTO balancelevel VALUES (?, ?)", [(50, 340.0), (60, 400.0), (70, 460.0)])
        db.commit()
        db.close()
        return path

    def dump(self, path):
        db = sqlite3.connect(path)
        try:
            return {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in TABLES}
        finally:
            db.close()

    def test_report_apply_again_undo(self):
        path = self.make()
        before = self.dump(path)
        rc, out = run("--db", path)
        self.assertEqual(rc, 0, out)
        self.assertIn("impale L55: 79,976 HP, taken_pct 100 -> 120,024 HP", out)
        self.assertIn("contagion L55: 100,000 HP, taken_pct 100 -> 150,061 HP, taken_pct 80", out)
        self.assertEqual(self.dump(path), before)

        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        bind = db.execute("SELECT tablename, chance, rolls FROM npcloot WHERE npc_name = ? ORDER BY 1", (cl.NPC,)).fetchall()
        self.assertEqual(bind, [(cl.T_CLASS, 100, 2), (cl.T_MISC, 50, 1), (cl.T_PUPPET, 10, 1)])
        cls = dict(db.execute("SELECT patternid, weight FROM loottables WHERE tablename = ?", (cl.T_CLASS,)).fetchall())
        self.assertEqual(len(cls), 14)
        self.assertEqual(sum(cls.values()), 26)
        self.assertIn(713056, cls)  # the Bracelet of Doubt stays
        self.assertNotIn(cl.PUPPET, cls)
        self.assertEqual(db.execute("SELECT patternid FROM loottables WHERE tablename = ?", (cl.T_PUPPET,)).fetchall(),
                         [(cl.PUPPET,)])
        self.assertEqual(db.execute("SELECT COUNT(*) FROM loottables WHERE tablename = ?", (cl.WIKI,)).fetchone()[0], 11)
        helm = db.execute("SELECT itemname, classuse, AC, cha, intelligence, HPMAX, str FROM itempattern "
                          "WHERE patternid = 713099").fetchone()
        self.assertEqual(helm, ("Helm of Doubt", 4, 124, 20, -50, 80, 0))
        self.assertEqual(db.execute("SELECT hp_pct, taken_pct FROM classtemplates ORDER BY name").fetchall(),
                         [(1229, 80), (983, 100)])
        self.assertEqual(db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0], 30)
        db.close()

        rc, out = run("--db", path, "--apply")
        self.assertIn("applied already", out)
        rc, out = run("--db", path, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertIn("30 rows put back, 0 left", out)
        self.assertEqual(self.dump(path), before)

    def test_ascended_carried_over(self):
        path = self.make()
        db = sqlite3.connect(path)
        item(db, 981203, "Ascended Bracelet of Doubt")
        db.execute("INSERT INTO itemascended VALUES (981203, 713056, '', 'board 1536')")
        db.execute("UPDATE loottables SET patternid = 981203 WHERE patternid = 713056")
        db.commit()
        db.close()
        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        cls = {r[0] for r in db.execute("SELECT patternid FROM loottables WHERE tablename = ?", (cl.T_CLASS,))}
        db.close()
        self.assertIn(981203, cls)
        self.assertNotIn(713056, cls)

    def test_refused(self):
        path = self.make()
        db = sqlite3.connect(path)
        db.execute("DELETE FROM loottables WHERE patternid = 713056")
        db.execute("UPDATE classtemplates SET hp_pct = 700 WHERE name = 'iod impale'")
        db.commit()
        db.close()
        before = self.dump(path)
        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 1)
        self.assertIn("refused", out)
        self.assertIn("hp_pct 700", out)
        self.assertEqual(self.dump(path), before)


if __name__ == "__main__":
    unittest.main()
