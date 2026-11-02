import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def dump(path):
    db = sqlite3.connect(path)
    out = "\n".join(l for l in db.iterdump() if "sqlite_sequence" not in l)
    db.close()
    return out


class FixWikiQuestRewards(unittest.TestCase):
    """tools/fix_wiki_quest_rewards.py on a made-up database: an item added to a quest's last step beside what it gives,
    one to a step that gives nothing, a magician's place filled in a byclass map; a quest whose magician has a reward
    already left, an item of two records refused; a second apply adds nothing and an undo puts every row back. The
    quest keys and item names are the tool's (it has no other); everything else is made up."""

    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db", dir=os.environ.get("TMPDIR"))
        os.close(fd)
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER, file TEXT, digest TEXT, enabled INTEGER)")
        db.execute("CREATE TABLE queststeps (questid INTEGER, step INTEGER, rewarditems TEXT NOT NULL DEFAULT '', "
                   "byclass TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step))")
        db.execute("CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT)")
        db.executemany("INSERT INTO quest_import_quests VALUES (?, ?, 'x.json', 'd', 1)", [
            ("1-20/neriak-nektulos/warrior/L15", 1), ("1-20/qeynos/bard/L7", 2), ("17/halas/main", 3), ("17/fayspire/main", 4)])
        db.executemany("INSERT INTO queststeps VALUES (?, ?, ?, ?)", [
            (1, 1, "", ""), (1, 2, "50", ""),                                    # the last step gives a scroll: the bracers beside it
            (2, 1, "60", ""), (2, 2, "", ""),                                    # the last step gives nothing
            (3, 1, "", ""), (3, 2, "", json.dumps({"0": {"items": "70"}, "9": {"items": "71"}}, separators=(",", ":"))),
            (4, 1, "", json.dumps({"10": {"items": "72"}}, separators=(",", ":"))),   # the magician has one: left
        ])
        db.executemany("INSERT INTO itempattern VALUES (?, ?)", [
            (100, "Cauldron Bracers"), (101, "Traveling Boots"), (102, "Chromatic Dagger"),
            (103, "Chromatic Gloves"), (104, "chromatic gloves")])                 # two records: refused
        db.commit()
        db.close()
        self.before = dump(self.path)

    def tearDown(self):
        os.remove(self.path)

    def tool(self, *args):
        return subprocess.run([sys.executable, os.path.join(HERE, "fix_wiki_quest_rewards.py"), "--db", self.path, *args],
                              capture_output=True, text=True, check=True).stdout

    def steps(self):
        db = sqlite3.connect(self.path)
        out = {(q, s): (r, b) for q, s, r, b in db.execute("SELECT questid, step, rewarditems, byclass FROM queststeps")}
        db.close()
        return out

    def test_apply_and_undo(self):
        out = self.tool()
        self.assertIn("3 rewards to add, 1 left", out)
        self.assertIn("Chromatic Gloves for class 10: 2 records of that name", out)
        self.assertEqual(dump(self.path), self.before, "a report writes nothing")
        self.assertIn("added 3 rewards", self.tool("--apply"))
        got = self.steps()
        self.assertEqual(got[(1, 2)][0], "50,100", "the bracers beside the scroll, on the last step")
        self.assertEqual(got[(2, 2)][0], "101", "the boots on the last step")
        self.assertEqual(got[(2, 1)][0], "60", "an earlier step untouched")
        self.assertEqual(json.loads(got[(3, 2)][1]), {"0": {"items": "70"}, "9": {"items": "71"}, "10": {"items": "102"}})
        self.assertIn("added 0 rewards", self.tool("--apply"), "a second apply adds nothing")
        self.tool("--undo")
        self.assertEqual(dump(self.path), self.before, "undo puts every row back")

    def test_a_magician_with_a_reward_is_left(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM itempattern WHERE patternid = 104")
        db.commit()
        db.close()
        self.assertIn("left: 17/fayspire/main: Chromatic Gloves for class 10: class 10 has a reward already", self.tool())


if __name__ == "__main__":
    unittest.main()
