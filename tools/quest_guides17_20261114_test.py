#!/usr/bin/env python3
"""tools/quest_guides17_20261114.py on a database of its own: each shard's variant written, the other shard's quest left,
a row a GM changed since refusing everything, a second --apply nothing, --undo exact."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

TOOL = os.path.join(os.path.dirname(os.path.abspath(__file__)), "quest_guides17_20261114.py")
PLAN = {"changes": [
    {"section": "mid", "questid": 1, "step": 1, "col": "journal", "why": "t21 L10",
     "variants": [{"shards": "hl", "before": "Speak with Grumgra.", "after": "Speak with Grumgra in a shelter on the shore south-west of Grobb."}]},
    {"section": "mid", "questid": 1, "step": 0, "col": "offer", "why": "t21",
     "variants": [{"shards": "h", "before": "Long words.\nWill you?", "after": "Long words.\nWill you?"},
                  {"shards": "l", "before": "Go.", "after": "Grumgra, on the shore, has something for you."}]},
    {"section": "high", "questid": 2, "step": 2, "col": "journal", "why": "t63",
     "variants": [{"shards": "h", "before": "Return to Brogar.", "after": "Return to Warlord Brogar in Oggok."}]},
]}


def make(path, offer, journal2):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, giver TEXT, offer TEXT, accept TEXT DEFAULT '',
            decline TEXT DEFAULT '', declined TEXT DEFAULT '', started TEXT DEFAULT '');
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, journal TEXT, says TEXT DEFAULT '',
            PRIMARY KEY (questid, step));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
        CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
        INSERT INTO accountinfo VALUES (7, 'dab');
    """)
    db.execute("INSERT INTO quests VALUES (1, 'The Traitor', 'Underlord Solthe', ?, '', '', '', '')", (offer,))
    db.execute("INSERT INTO queststeps VALUES (1, 1, 'talk', 'Grumgra', 'Speak with Grumgra.', '')")
    if journal2 is not None:
        db.execute("INSERT INTO quests VALUES (2, 'Oggok', 'Warlord Brogar', '', '', '', '', '')")
        db.execute("INSERT INTO queststeps VALUES (2, 2, 'talk', 'Warlord Brogar', ?, '')", (journal2,))
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return [list(db.execute(f"SELECT * FROM {t} ORDER BY 1, 2")) for t in ("quests", "queststeps")]
    finally:
        db.close()


class QuestGuides(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.plan = os.path.join(self.dir.name, "plan.json")
        with open(self.plan, "w") as f:
            json.dump(PLAN, f)

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, db, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", db, "--plan", self.plan, *args],
                              capture_output=True, text=True)

    def shard(self, name, offer, journal2):
        path = os.path.join(self.dir.name, name)
        make(path, offer, journal2)
        return path

    def test_each_shard(self):
        h = self.shard("h.db", "Long words.\nWill you?", "Return to Brogar.")
        l = self.shard("l.db", "Go.", None)
        hb, lb = dump(h), dump(l)
        for path, want in ((h, 2), (l, 2)):
            r = self.run_tool(path, "--apply")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertIn(f"applied: {want} rows", r.stdout)
        db = sqlite3.connect(h)
        self.assertEqual(db.execute("SELECT journal FROM queststeps WHERE questid = 2").fetchone()[0], "Return to Warlord Brogar in Oggok.")
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 1").fetchone()[0], "Long words.\nWill you?")
        self.assertEqual(db.execute("SELECT COUNT(*) FROM gm_audit WHERE username = 'dab' AND accountid = 7").fetchone()[0], 2)
        db.close()
        db = sqlite3.connect(l)
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 1").fetchone()[0], "Grumgra, on the shore, has something for you.")
        db.close()
        r = self.run_tool(h, "--apply")
        self.assertIn("applied already", r.stdout)
        for path, before in ((h, hb), (l, lb)):
            r = self.run_tool(path, "--undo")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertEqual(dump(path), before)
        # undone, a fresh apply finds everything to do again; done by hand, nothing
        db = sqlite3.connect(l)
        db.execute("UPDATE quests SET offer = 'Grumgra, on the shore, has something for you.'")
        db.execute("UPDATE queststeps SET journal = 'Speak with Grumgra in a shelter on the shore south-west of Grobb.'")
        db.commit()
        db.close()
        r = self.run_tool(l, "--apply")
        self.assertIn("nothing to do", r.stdout)

    def test_refused_when_changed(self):
        h = self.shard("h.db", "Long words.\nWill you?", "Return to Brogar, a GM's words.")
        before = dump(h)
        r = self.run_tool(h, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("refused, nothing done", r.stdout)
        self.assertEqual(dump(h), before)

    def test_report_only(self):
        h = self.shard("h.db", "Long words.\nWill you?", "Return to Brogar.")
        before = dump(h)
        r = self.run_tool(h)
        self.assertIn("report only: 2 rows", r.stdout)
        self.assertEqual(dump(h), before)
        r = self.run_tool(h, "--section", "high", "--apply")
        self.assertIn("applied: 1 rows", r.stdout)


if __name__ == "__main__":
    unittest.main()
