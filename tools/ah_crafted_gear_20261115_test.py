#!/usr/bin/env python3
"""tools/ah_crafted_gear_20261115.py on a database of its own: refused without the server's column; the modified piece
taken over the plain one, the plain one where the tier has none, a later shape name where the first is missing; No
Trade, Lore and valueless records passed by; priced at value (at least 100); a second run nothing; --undo exact."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "ah_crafted_gear_20261115.py")


def make(path, column=True):
    db = sqlite3.connect(path)
    db.executescript(f"""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemcost INTEGER, maxhp INTEGER,
            levelreq INTEGER, equipslot INTEGER, classuse INTEGER, trade INTEGER, lore INTEGER, rent INTEGER);
        CREATE TABLE craftrecipes (id INTEGER PRIMARY KEY, skill INTEGER, rarity INTEGER, product INTEGER,
            ingredients TEXT);
        CREATE TABLE auction_lots (lot INTEGER PRIMARY KEY AUTOINCREMENT, seller INTEGER, seller_name TEXT,
            patternid INTEGER, stack INTEGER, remainhp INTEGER, charges INTEGER, min_bid INTEGER, buyout INTEGER,
            fee INTEGER, listed_at INTEGER, ends_at INTEGER, state TEXT DEFAULT 'open', per_item INTEGER DEFAULT 0
            {", perpetual INTEGER NOT NULL DEFAULT 0" if column else ""});
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
        INSERT INTO itempattern VALUES
            (1, 'Steel Breastplate', 880, 900, 23, 5, 525, 0, 0, 1),
            (2, 'Titan Steel Breastplate', 1080, 900, 23, 5, 525, 0, 0, 1),
            (3, 'Bronze Breastplate', 394, 900, 13, 5, 525, 0, 0, 1),
            (4, 'Fiery Steel Sabre', 1080, 900, 23, 12, 13, 0, 0, 1),
            (5, 'Steel Sabre', 880, 900, 23, 12, 13, 0, 0, 1),
            (6, 'Silver Amber Ring', 930, 900, 23, 8, 32767, 0, 0, 1),
            (7, 'Silver Bloodstone Ring', 930, 900, 23, 8, 32767, 1, 0, 1),
            (8, 'Steel Amber Ring', 0, 900, 13, 8, 32767, 0, 0, 1),
            (9, 'Infused Ramie Gloves', 1080, 900, 23, 19, 32767, 0, 0, 1),
            (10, 'Infused Suede Gloves', 1080, 900, 23, 19, 17407, 0, 0, 1),
            (11, 'Iron Onyx Earring', 50, 900, 13, 3, 32767, 0, 0, 1),
            (12, 'Titan Bronze Breastplate', 594, 900, 13, 5, 525, 0, 1, 1);
    """)
    db.executemany("INSERT INTO craftrecipes VALUES (?, ?, ?, ?, '[]')", [
        (1, 1, 0, 1), (2, 1, 1, 2), (3, 1, 0, 3), (4, 0, 1, 4), (5, 0, 0, 5), (6, 3, 0, 6), (7, 3, 0, 7),
        (8, 3, 0, 8), (9, 2, 1, 9), (10, 2, 1, 10), (11, 3, 0, 11), (12, 1, 1, 12)])
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

    def test_pick_price_again_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.db")
            make(p)
            before = dump(p)
            self.assertIn("report only", run(p).stdout)
            self.assertEqual(lots(p), {})
            run(p, "--apply")
            got = lots(p)
            # the modified piece over the plain; the plain where only a Lore one is modified; cloth gloves, not
            # leather; No Trade (7) and worth-0 (8) passed by; a cheap piece raised to 100
            want = {"Titan Steel Breastplate": 1080, "Bronze Breastplate": 394, "Fiery Steel Sabre": 1080,
                    "Silver Amber Ring": 930, "Infused Ramie Gloves": 1080, "Iron Onyx Earring": 100}
            self.assertEqual({k: v[0] for k, v in got.items()}, want)
            for v in got.values():
                self.assertEqual(v[0], v[1])
                self.assertEqual(v[2:], (0, "ilovetesters", 1, 1, 4102444800))
            db = sqlite3.connect(p)
            self.assertEqual(db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0], 6)
            db.close()
            self.assertIn("applied already", run(p, "--apply").stdout)
            self.assertIn("put back 6", run(p, "--undo").stdout)
            self.assertEqual(dump(p), before)


if __name__ == "__main__":
    unittest.main()
