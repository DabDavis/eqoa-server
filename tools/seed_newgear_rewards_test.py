import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_newgear_rewards as ng  # noqa: E402

COLS = ("patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT, levelreq INTEGER, equipslot INTEGER, "
        "weapondamage INTEGER, classuse INTEGER, model INTEGER, itemicon INTEGER, maxhp INTEGER, rent INTEGER, trade INTEGER")


def dump(path):
    """The database as SQL, but for sqlite_sequence: AUTOINCREMENT's high-water mark is never
    lowered by a delete (the server's own deletes leave it too), and an undo must not lower it while
    the server may be making items."""
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class NewGear(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute(f"CREATE TABLE itempattern ({COLS})")
        db.execute("CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, rewarditems TEXT NOT NULL DEFAULT '')")
        db.execute("CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER)")
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, stackleft INTEGER, "
                   "remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER)")
        bases = {b for _, _, _, b, _, _ in ng.ITEMS} | {l for *_, l, _ in ng.ITEMS if l}
        for b in bases:
            db.execute("INSERT INTO itempattern VALUES (?, ?, '', 5, 12, 50, 8, ?, ?, 300, 1, 0)", (b, f"base {b}", b * 10, b * 100))
        db.execute("ALTER TABLE itempattern ADD COLUMN duration INTEGER NOT NULL DEFAULT 50")
        db.execute("UPDATE itempattern SET maxhp = 0, duration = 0 WHERE patternid = 5703")  # a blank record of the old dump
        for q, *_ in ng.ITEMS:
            db.execute("INSERT INTO queststeps VALUES (?, 1, 'talk', '')", (q,))
            db.execute("INSERT INTO queststeps VALUES (?, 3, 'give', '')", (q,))
        db.execute("INSERT INTO charquests VALUES (103, 100480, 0, 1)")  # Dust: finished, owed
        db.execute("INSERT INTO charquests VALUES (104, 100430, 2, 0)")  # under way: owed nothing
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) "
                   "VALUES (103, 1, 200, 0, 6428, 0, 0, 13)")
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def run_tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "seed_newgear_rewards.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=True).stdout

    def test_apply_and_undo(self):
        self.assertIn("report only", self.run_tool())
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        out = self.run_tool("--apply")
        self.assertIn("wrote 11 items, 11 rewards, 1 given", out)
        db = sqlite3.connect(self.path)
        name, lvl, dmg, cls, model, icon, desc = db.execute(
            "SELECT itemname, levelreq, weapondamage, classuse, model, itemicon, itemdesc FROM itempattern WHERE patternid = 706004").fetchone()
        self.assertEqual((name, lvl, dmg, cls), ("Black Raven Sword", 5, 50, 8), "the hatchet's numbers")
        self.assertEqual((model, icon), (58590, 585900), "the Stone Sword's look")
        self.assertIn("Sythonis U`Dedne", desc)
        self.assertEqual(db.execute("SELECT model FROM itempattern WHERE patternid = 706008").fetchone()[0], 57030, "its own base's look")
        self.assertEqual(db.execute("SELECT maxhp, duration FROM itempattern WHERE patternid = 706004").fetchone(), (800, 50),
                         "a blank base's copy gets hit points and durability")
        self.assertEqual(db.execute("SELECT maxhp, duration FROM itempattern WHERE patternid = 706003").fetchone(), (300, 50),
                         "the control: a base with its own keeps them")
        self.assertEqual(db.execute("SELECT rewarditems FROM queststeps WHERE questid = 100480 AND step = 3").fetchone()[0], "706004")
        self.assertEqual(db.execute("SELECT rewarditems FROM queststeps WHERE questid = 100480 AND step = 1").fetchone()[0], "", "the last step only")
        self.assertEqual(db.execute("SELECT remainHP, location, listnumber FROM charinventory WHERE serverid = 103 AND patternid = 706004").fetchone(),
                         (800, 0, 14), "given as a quest gives: the record's maxhp, the next place")
        self.assertEqual(db.execute("SELECT count(*) FROM charinventory WHERE serverid = 104").fetchone()[0], 0, "a quest under way owes nothing")
        db.close()
        self.assertIn("wrote 0 items, 0 rewards, 0 given", self.run_tool("--apply"), "the control: a second apply adds nothing")
        self.run_tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every byte back")

    def test_a_set_reward_is_left(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE queststeps SET rewarditems = '9999' WHERE questid = 100430 AND step = 3")
        db.commit()
        db.close()
        out = self.run_tool()
        self.assertIn("PROBLEM: quest 100430: step 3 rewards '9999' already: left", out)


if __name__ == "__main__":
    unittest.main()
