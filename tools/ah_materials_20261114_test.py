#!/usr/bin/env python3
"""tools/ah_materials_20261114.py on a database of its own: refused without the server's column, the materials put up
as common or rare at their prices (raised to an item's value), the left-out parts not, a second run nothing, --undo
exact."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "ah_materials_20261114.py")


def make(path, column=True):
    db = sqlite3.connect(path)
    db.executescript(f"""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemcost INTEGER, maxstack INTEGER,
            maxhp INTEGER, trade INTEGER, lore INTEGER, rent INTEGER);
        CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, rarity INTEGER, product INTEGER, ingredients TEXT);
        CREATE TABLE auction_lots (lot INTEGER PRIMARY KEY AUTOINCREMENT, seller INTEGER, seller_name TEXT,
            patternid INTEGER, stack INTEGER, remainhp INTEGER, charges INTEGER, min_bid INTEGER, buyout INTEGER,
            fee INTEGER, listed_at INTEGER, ends_at INTEGER, state TEXT DEFAULT 'open', per_item INTEGER DEFAULT 0
            {", perpetual INTEGER NOT NULL DEFAULT 0" if column else ""});
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
        INSERT INTO itempattern VALUES
            (1, 'Iron Bar', 14, 999, 0, 0, 0, 1), (2, 'Seralite Ore', 743, 999, 0, 0, 0, 1),
            (3, 'Achroite', 20, 50, 0, 0, 0, 1), (4, 'Titan Alloy', 100, 50, 0, 0, 0, 1),
            (5, 'Ruby', 270, 50, 0, 0, 0, 1), (6, 'Star Ruby', 800, 50, 0, 0, 0, 1),
            (7, 'Blacksmith''s Emblem', 5, 20, 0, 0, 0, 1), (8, 'iron ore', 47, 999, 0, 0, 0, 1),
            (9, 'Bound Gem', 20, 50, 0, 1, 0, 1), (10, 'Plate Helm Mold', 10, 999, 0, 0, 0, 1),
            (100, 'plate helm', 50, 1, 900, 0, 0, 1), (101, 'ruby ring', 50, 1, 900, 0, 0, 1);
    """)
    ing = lambda *xs: json.dumps([{"item": i, "count": 1, "keep": k} for i, k in xs])
    db.executemany("INSERT INTO craftrecipes VALUES (?, ?, ?, ?)", [
        (1, 0, 100, ing((1, False), (10, False), (3, True))),
        (2, 1, 100, ing((2, False), (10, False), (4, True))),
        (3, 2, 101, ing((1, False), (5, True), (9, True))),
        (4, 3, 101, ing((1, False), (6, True))),
        (5, 0, 100, ing((8, False), (7, False), (10, False))),
    ])
    db.commit()
    db.close()


def run(path, *args, check=True):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True,
                          check=check)


def lots(path):
    db = sqlite3.connect(path)
    out = {r[0]: r[1:] for r in db.execute(
        "SELECT p.itemname, l.min_bid, l.buyout, l.seller, l.seller_name, l.stack, l.perpetual, l.ends_at "
        "FROM auction_lots l JOIN itempattern p USING (patternid)")}
    db.close()
    return out


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l and "sqlite_sequence" not in l]
    db.close()
    return out


class Test(unittest.TestCase):
    def test_refused_without_the_column(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p, column=False)
            r = run(p, "--apply", check=False)
            self.assertEqual(r.returncode, 1)
            self.assertIn("no perpetual column", r.stdout)
            db = sqlite3.connect(p)
            self.assertEqual(db.execute("SELECT COUNT(*) FROM auction_lots").fetchone()[0], 0)
            db.close()

    def test_apply_again_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = dump(p)
            out = run(p, "--apply").stdout
            self.assertIn("7 materials: 5 common (100), 2 rare (500)", out)
            got = lots(p)
            want = {"Iron Bar": 100, "Seralite Ore": 743, "Achroite": 100, "Titan Alloy": 100, "Ruby": 500,
                    "Star Ruby": 800, "Plate Helm Mold": 100}
            self.assertEqual({k: v[0] for k, v in got.items()}, want)
            for v in got.values():
                self.assertEqual(v[0], v[1])
                self.assertEqual(v[2:], (0, "ilovetesters", 1, 1, 4102444800))
            self.assertIn("applied already", run(p, "--apply").stdout)
            self.assertIn("put back 7", run(p, "--undo").stdout)
            self.assertEqual(dump(p), before)


if __name__ == "__main__":
    unittest.main()
