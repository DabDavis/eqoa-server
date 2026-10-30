import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_rehabilitation as rh  # noqa: E402

ITEM_COLS = ("patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL, unk1 INTEGER NOT NULL, "
             "itemicon INTEGER NOT NULL, unk2 INTEGER NOT NULL, equipslot INTEGER NOT NULL, unk3 INTEGER NOT NULL, "
             "trade INTEGER NOT NULL, rent INTEGER NOT NULL, unk4 INTEGER NOT NULL, attacktype INTEGER NOT NULL, "
             "weapondamage INTEGER NOT NULL, unk5 INTEGER NOT NULL, levelreq INTEGER NOT NULL, maxstack INTEGER NOT NULL, "
             "maxhp INTEGER NOT NULL, duration INTEGER NOT NULL, classuse INTEGER NOT NULL, raceuse INTEGER NOT NULL, "
             "procanim INTEGER NOT NULL, lore INTEGER NOT NULL, unk6 INTEGER NOT NULL, craft INTEGER NOT NULL, "
             "itemname TEXT NOT NULL, itemdesc TEXT NOT NULL, model INTEGER NOT NULL, color INTEGER NOT NULL, "
             "str INTEGER, AC INTEGER")


def dump(path, sequences=False):
    """The database as SQL, but for sqlite_sequence (AUTOINCREMENT's high-water marks: charinventory's rises with
    the roses given and stays, as any item given in play) unless sequences."""
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if sequences or "sqlite_sequence" not in l)
    db.close()
    return out


def mark(path, table):
    db = sqlite3.connect(path)
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = ?", (table,)).fetchone()
    db.close()
    return None if r is None else r[0]


class Rehabilitation(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute(f"CREATE TABLE itempattern ({ITEM_COLS})")
        db.execute("INSERT INTO itempattern VALUES (1001, 0, 0, 759960290, 0, -1, 0, 0, 1, 0, 0, 0, 0, 55, 1, 0, 0, "
                   "32767, 1023, 0, 1, 0, 0, 'rose of renewal', 'A magical and extremely rare type of rose.', 0, "
                   "4294967295, 0, 0)")
        db.execute("INSERT INTO itempattern VALUES (1081, 0, 0, 5, 0, 12, 0, 1, 1, 0, 1, 500, 0, 60, 1, 900, 50, "
                   "4, 1023, 0, 0, 0, 0, 'Sword of Virtue', '', 7, 0, NULL, NULL)")
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, npc_type INTEGER)")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?)", [(2205, "Rehabilitator Anja", 128),
                                                             (101943, "Rehabilitator Frejo", 0),
                                                             (7, "a gnoll", 0)])
        db.execute("CREATE TABLE quests (questid INTEGER PRIMARY KEY, source TEXT NOT NULL DEFAULT '')")
        db.execute("CREATE TABLE queststeps (questid INTEGER, step INTEGER, rewarditems TEXT NOT NULL DEFAULT '')")
        db.executemany("INSERT INTO quests VALUES (?, ?)", [
            (101114, "EQOA-Data 60/good-tank/reward-warrior (tools/import_quests.py)"),
            (101115, "EQOA-Data 60/good-tank/reward-paladin (tools/import_quests.py)"),
            (101107, "EQOA-Data 60/good-tank/ep3-2 (tools/import_quests.py)"),
            (100100, "EQOA-Data 1-20/freeport/warrior/L20 (tools/import_quests.py)")])
        db.executemany("INSERT INTO queststeps VALUES (?, ?, ?)", [
            (101114, 1, ""), (101114, 2, ""), (101115, 1, ""), (101115, 2, "1081"),
            (101107, 7, ""), (100100, 9, "")])
        db.execute("CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, done INTEGER)")
        db.executemany("INSERT INTO charquests VALUES (?, ?, ?, ?)", [
            (31, 101114, 0, 1),   # finished the warrior's epic: owed a rose
            (32, 101115, 0, 1),   # finished the paladin's, holds a rose already: owed none
            (33, 101114, 2, 0),   # under way: owed none
            (34, 101107, 0, 1)])  # finished an episode, not the epic: owed none
        db.execute("CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, "
                   "stackleft INTEGER, remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, "
                   "location INTEGER, listnumber INTEGER)")
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, "
                   "location, listnumber) VALUES (31, 1, 0, 0, 1081, -1, 0, 4)")
        db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, "
                   "location, listnumber) VALUES (32, 1, 0, 0, 709300, -1, 1, 0)")  # a rose in the bank
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def run_tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "seed_rehabilitation.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=True).stdout

    def test_apply_and_undo(self):
        self.assertIn("report only", self.run_tool())
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        db = sqlite3.connect(self.path)
        out = self.run_tool("--apply")
        self.assertIn("wrote 1 items, 1 npcs, 2 rewards, 1 given", out)
        rose = db.execute("SELECT itemname, itemicon, itemcost, trade, lore, maxstack, equipslot, model, levelreq "
                          "FROM itempattern WHERE patternid = 709300").fetchone()
        self.assertEqual(rose, ("Rose of Rehabilitation", 759960290, 10_000_000, 1, 0, 1, -1, 0, 1),
                         "rose of renewal's look, worth ten million, no trade, one a record")
        self.assertIn("Rehabilitator Frejo", db.execute("SELECT itemdesc FROM itempattern WHERE patternid = 709300").fetchone()[0])
        self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE id = 101943").fetchone()[0], 128, "Frejo talks")
        self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE id = 2205").fetchone()[0], 128, "Anja as she was")
        self.assertEqual(db.execute("SELECT npc_type FROM npcs WHERE id = 7").fetchone()[0], 0, "the control: a gnoll fights")
        steps = dict(((q, s), r) for q, s, r in db.execute("SELECT questid, step, rewarditems FROM queststeps"))
        self.assertEqual(steps[(101114, 2)], "709300", "the epic's last step")
        self.assertEqual(steps[(101114, 1)], "", "not the step before")
        self.assertEqual(steps[(101115, 2)], "1081, 709300", "beside the epic's own reward")
        self.assertEqual(steps[(101107, 7)], "", "the control: an episode, not the epic")
        self.assertEqual(steps[(100100, 9)], "", "the control: a level 20 quest")
        given = db.execute("SELECT serverid, location, equiploc, listnumber FROM charinventory WHERE patternid = 709300 "
                           "AND serverid != 32").fetchall()
        self.assertEqual(given, [(31, 0, -1, 5)], "the warrior's finisher, at the inventory's next place; not the one "
                         "under way, nor one who finished an episode")
        self.assertEqual(db.execute("SELECT count(*) FROM charinventory WHERE serverid = 32").fetchone()[0], 1,
                         "the control: one who holds a rose (banked) is owed none")
        db.close()

        out = self.run_tool("--apply")
        self.assertIn("wrote 0 items, 0 npcs, 0 rewards, 0 given", out, "a second run finds it done")
        self.run_tool("--undo")
        self.assertEqual(dump(self.path), self.before, "the undo puts back exactly what was there")

    def fill(self, sid, location, upto):
        db = sqlite3.connect(self.path)
        while db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND location = ?",
                         (sid, location)).fetchone()[0] < upto:
            db.execute("INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, "
                       "location, listnumber) VALUES (?, 1, 0, 0, 1081, -1, ?, (SELECT COALESCE(MAX(listnumber), -1) + 1 "
                       "FROM charinventory WHERE serverid = ? AND location = ?))", (sid, location, sid, location))
        db.commit()
        db.close()

    def roses(self, sid):
        db = sqlite3.connect(self.path)
        out = db.execute("SELECT location, listnumber FROM charinventory WHERE serverid = ? AND patternid = ?",
                         (sid, rh.ROSE_ID)).fetchall()
        db.close()
        return out

    def test_a_full_pack_banks_the_rose(self):
        """A finisher whose pack holds its 40 records is given the rose in its bank (the control is
        test_apply_and_undo's, given in the pack); with the bank full too, none, said so, and a later run gives it
        once there is room."""
        self.fill(31, 0, rh.LIST_CAP)
        self.assertIn("in its bank: its pack is full", self.run_tool("--apply"))
        self.assertEqual(self.roses(31), [(1, 0)])
        self.run_tool("--undo")

        self.fill(31, 1, rh.LIST_CAP)
        out = self.run_tool("--apply")
        self.assertIn("character 31: owed a rose, but its pack and bank are full: given none", out)
        self.assertIn("0 given", out)
        self.assertEqual(self.roses(31), [])
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory WHERE itemid = (SELECT max(itemid) FROM charinventory WHERE serverid = 31 "
                   "AND location = 0)")
        db.commit()
        db.close()
        self.assertIn("1 given", self.run_tool("--apply"), "room made, a later run gives it")
        self.assertEqual(len(self.roses(31)), 1)

    def test_the_item_mark_goes_back(self):
        """The rose's id raises itempattern's AUTOINCREMENT mark (1081 here) to 709300; an undo puts it back, so the
        round trip is the file as it was, marks and all. The control: an item made after the apply keeps the mark
        it raised, and the next item made takes a new id."""
        before = dump(self.path, sequences=True)
        self.run_tool("--apply")
        self.assertEqual(mark(self.path, "itempattern"), rh.ROSE_ID)
        self.run_tool("--undo")
        self.assertEqual(mark(self.path, "itempattern"), 1081)
        self.assertEqual(dump(self.path, sequences=True).replace(
            "INSERT INTO \"sqlite_sequence\" VALUES('charinventory',3);",
            "INSERT INTO \"sqlite_sequence\" VALUES('charinventory',2);"), before, "the round trip, marks and all")

        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, "
                   "attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, "
                   "lore, unk6, craft, itemname, itemdesc, model, color) VALUES (0, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, "
                   "1, 0, 0, 32767, 1023, 0, 0, 0, 0, 'a GM''s new item', '', 0, 0)")
        made = db.execute("SELECT max(patternid) FROM itempattern").fetchone()[0]
        db.commit()
        db.close()
        self.assertEqual(made, rh.ROSE_ID + 1)
        self.run_tool("--undo")
        self.assertEqual(mark(self.path, "itempattern"), rh.ROSE_ID + 1, "an item made since keeps its mark")

    def test_a_rose_kept_keeps_the_mark(self):
        """A rose a GM changed since stays at an undo, and so does the mark above it."""
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE itempattern SET itemcost = 5000000 WHERE patternid = ?", (rh.ROSE_ID,))
        db.commit()
        db.close()
        self.run_tool("--undo")
        self.assertEqual(mark(self.path, "itempattern"), rh.ROSE_ID)

    def test_no_second_rose_after_one_is_used(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory WHERE patternid = 709300 AND serverid = 31")  # used at a rehabilitator
        db.commit()
        out = self.run_tool("--apply")
        self.assertIn("0 given", out, "a rose this gave and the character used is not given again")
        db.close()

    def test_a_rose_used_at_a_rehabilitator_counts(self):
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE charresets (serverid INTEGER, kind TEXT, used INTEGER, paid INTEGER, bonus INTEGER, "
                   "updated TEXT)")
        db.execute("INSERT INTO charresets VALUES (31, 'path', 2, 1, 0, '')")  # the quest's rose, used for a path
        db.commit()
        db.close()
        self.assertNotIn("character 31", self.run_tool(), "a rose used is a rose had")

    def test_holder_of_a_rose_owed_none(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM charinventory WHERE serverid = 32")
        db.commit()
        db.close()
        self.assertIn("character 32: given a rose", self.run_tool(), "the control: without its rose it is owed one")

    def test_write_data(self):
        folder = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        try:
            shutil.copy(os.path.join(HERE, "quests", "data", "60.json"), folder)
            path = os.path.join(folder, "60.json")
            before = json.load(open(path, encoding="utf-8"))
            self.assertIn("the rose added to", self.run_tool("--write-data", folder))
            after = json.load(open(path, encoding="utf-8"))
            changed = 0
            for b, a in zip(before["quests"], after["quests"]):
                if b == a:
                    continue
                changed += 1
                self.assertRegex(a["key"], r"^60/[^/]+/reward-[a-z]+$")
                self.assertEqual(a["steps"][:-1], b["steps"][:-1], "only the last step")
                self.assertEqual(a["steps"][-1]["rewards"]["items"][-1], "Rose of Rehabilitation")
            epics = [q for q in after["quests"] if rh.EPIC_FILE_KEY.match(q["key"])]
            self.assertEqual(len(epics), 23, "the 23 epics' reward quests")
            for q in epics:
                self.assertEqual(q["steps"][-1]["rewards"]["items"].count("Rose of Rehabilitation"), 1, q["key"])
            # this repository's file names the rose already: a copy of it changes nothing
            self.assertEqual(changed, 0, "the committed 60.json already names the rose")
            self.run_tool("--write-data", folder)
            self.assertEqual(json.load(open(path, encoding="utf-8")), after, "a second run changes nothing")
        finally:
            shutil.rmtree(folder)


if __name__ == "__main__":
    unittest.main()
