#!/usr/bin/env python3
"""tools/lore_dialog_20261121.py on a small database: the rules, each part, --only/--skip, a row not as read skipped,
apply, a second apply, and an exact undo."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lore_dialog_20261121 as tool  # noqa: E402

SCHEMA = """
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
  giver TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1, classes TEXT NOT NULL DEFAULT '',
  races TEXT NOT NULL DEFAULT '', offer TEXT NOT NULL DEFAULT '', accept TEXT NOT NULL DEFAULT '',
  decline TEXT NOT NULL DEFAULT '', declined TEXT NOT NULL DEFAULT '', started TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
  target TEXT NOT NULL DEFAULT '', count INTEGER NOT NULL DEFAULT 1, journal TEXT NOT NULL DEFAULT '',
  answer TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '', waiting TEXT NOT NULL DEFAULT '',
  notyet TEXT NOT NULL DEFAULT '', reminder TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, town TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '', line TEXT NOT NULL, source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE signtext (npcid INTEGER PRIMARY KEY, kind TEXT NOT NULL DEFAULT 'gm', text TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'gm', note TEXT NOT NULL DEFAULT '');
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, modelid INTEGER);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, faction TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT,
  ip TEXT, action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE lore_library_1121_log (seq INTEGER PRIMARY KEY);
"""

F_ENTRY = {"giver": "Azlynn", "name": "The Ring", "classes": "", "races": "", "minlevel": 1,
           "live": {"offer": "Ring.", "accept": "Yes", "decline": "No", "declined": "", "started": ""},
           "heirloom": {"offer": "The bronze ring, child.\nWill you fetch it?", "accept": "Yes", "decline": "No",
                        "declined": "Later, then.", "started": "Go."},
           "steps": [{"step": 1, "kind": "give", "target": "Azlynn", "count": 1,
                      "live": {"journal": "Give it.", "answer": "", "says": "Good.", "waiting": "", "notyet": "", "reminder": ""},
                      "heirloom": {"journal": "Give the ring to Azlynn.", "answer": "", "says": "Bright as the day it was made.",
                                   "waiting": "", "notyet": "", "reminder": ""}}]}

DATA = {
    "f": {"entries": [F_ENTRY]},
    "a": {"entries": [{"giver": "Levandius", "name": "Wolfwere Tails", "step": 0, "field": "offer",
                       "before": "Bring me four tails.", "after": "Under Drinal's moon they hunt. Bring me four tails."}],
          "boards": [{"name": "Grobb Announcements", "text": "Grobb. Mind manners."}],
          "lines": [{"town": "Hodstock", "lines": ["Hodstock feeds Freeport."]}, {"town": "Nowhere", "lines": ["x"]}]},
    "b": {"from": "Jurgash", "to": "Jurglash", "npc": "Warlord Jurglash", "tale": "Tales of the Grozmok", "teller": "Kuthrak"},
    "c": {"entries": [{"giver": "Dorandas", "name": "Mirin", "step": 2, "field": "says", "target": "Dorandas",
                       "before": "Welcome to the Wards of the Dawn.", "after": "Welcome to the Wards of Dawn. Heal first.",
                       "drops": []},
                      {"giver": "Nobody", "name": "Nothing", "step": 0, "field": "offer", "before": "a", "after": "b"}]},
    "d": {"entries": [{"giver": "Rathei", "name": "Wings", "step": 0, "field": "offer",
                       "before": "Bring me two wings.", "after": "My potions want wings. Bring me two."}]},
    "e": {"entries": [{"giver": "Rathei", "name": "Wings", "step": 2, "field": "says", "target": "Rathei",
                       "before": "Good.", "after": "Two wings, and not a vein torn. Good."}]},
}


def fixture(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.execute("INSERT INTO accountinfo VALUES (1, 'dab')")
    q = [(1, "Wolfwere Tails", "Levandius", "Bring me four tails."), (2, "Mirin", "Dorandas", "Hear me."),
         (3, "Wings", "Rathei", "Bring me two wings."), (4, "The Ring", "Azlynn", "Ring."),
         (5, "Tales of the Grozmok", "Kuthrak", "Sit.")]
    for qid, name, giver, offer in q:
        db.execute("INSERT INTO quests (questid, name, giver, offer, accept, decline) VALUES (?, ?, ?, ?, 'Yes', 'No')",
                   (qid, name, giver, offer))
    db.execute("INSERT INTO queststeps (questid, step, target, says) VALUES (2, 2, 'Dorandas', 'Welcome to the Wards of the Dawn.')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, says) VALUES (3, 2, 'give', 'Rathei', 'Good.')")
    db.execute("INSERT INTO queststeps (questid, step, kind, target, journal, says) VALUES (4, 1, 'give', 'Azlynn', 'Give it.', 'Good.')")
    db.execute("INSERT INTO queststeps (questid, step, target, says) VALUES (5, 2, 'Kuthrak', 'Warlord Jurgash make Clan Grobb.')")
    db.execute("INSERT INTO npcdialog (town, line, source) VALUES ('Grobb', 'Clan Ykesha follow Warlord Jurgash.', 'lore-1121')")
    db.execute("INSERT INTO signtext (npcid, kind, text, source) VALUES (90, 'place', 'Jurgash and Clan Grobb.', 'lore-1121')")
    db.execute("INSERT INTO npcs (npc_name, modelid) VALUES ('Grobb Announcements', ?)", (tool.SIGN_BODY,))
    db.execute("INSERT INTO npcs (npc_name, modelid) VALUES ('Warlord Jurglash', 1)")
    db.execute("INSERT INTO npcfaction (name, faction) VALUES ('elder fitzgrott', 'Citizens of Hodstock')")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f'SELECT * FROM "{t}" ORDER BY 1, 2').fetchall()
           for t in ("quests", "queststeps", "npcdialog", "signtext")}
    db.close()
    return out


def run(*args):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        code = tool.main(list(args))
    return code, buf.getvalue()


class LoreDialogTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        self.data = os.path.join(self.dir.name, "data")
        os.mkdir(self.data)
        for p, d in DATA.items():
            with open(os.path.join(self.data, f"{p}.json"), "w") as f:
                json.dump(d, f)
        fixture(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_shipped_data_keeps_the_rules(self):
        for p in tool.PARTS:
            path = os.path.join(tool.DATA, f"{p}.json")
            if os.path.exists(path):
                self.assertEqual(tool.check_file(path), [], p)

    def test_rules(self):
        e = {"giver": "Rathei", "name": "x", "step": 0, "field": "offer", "part": "d"}
        self.assertTrue(tool.entry_problems(dict(e, before="Bring me 4 wings.", after="Bring me wings.")))  # a number
        self.assertTrue(tool.entry_problems(dict(e, before="Go to Freeport for wings.", after="Go for wings.")))  # a name
        self.assertTrue(tool.entry_problems(dict(e, before="Bring wings.", after="Bring ‘wings’.")))
        self.assertTrue(tool.entry_problems(dict(e, before="Bring wings.", after="Bring 5% wings.")))
        self.assertTrue(tool.entry_problems(dict(e, before="Bring wings.", after="x" * 200 + ".")))  # the question
        self.assertTrue(tool.entry_problems(dict(e, field="accept", before="Yes", after="Aye")))  # not writable
        self.assertFalse(tool.entry_problems(dict(e, before="Bring wings.", after="My potions want wings. Bring them.")))

    def test_report_only(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data)
        self.assertEqual(code, 0, out)
        self.assertIn("part c: 1 rows to write, 1 skipped", out)
        self.assertEqual(dump(self.db), before)

    def test_apply_again_undo(self):
        before = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertEqual(code, 0, out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT offer, declined, started FROM quests WHERE questid = 4").fetchone(),
                         ("The bronze ring, child.\nWill you fetch it?", "Later, then.", "Go."))
        self.assertEqual(db.execute("SELECT says FROM queststeps WHERE questid = 4").fetchone()[0], "Bright as the day it was made.")
        self.assertIn("Drinal", db.execute("SELECT offer FROM quests WHERE questid = 1").fetchone()[0])
        self.assertEqual(db.execute("SELECT kind, source FROM signtext WHERE text = 'Grobb. Mind manners.'").fetchone(), ("board", tool.SRC))
        self.assertEqual(db.execute("SELECT count(*) FROM npcdialog WHERE town = 'Hodstock'").fetchone()[0], 1)
        self.assertEqual(db.execute("SELECT count(*) FROM npcdialog WHERE line LIKE '%Jurgash%'").fetchone()[0], 0)
        self.assertEqual(db.execute("SELECT text FROM signtext WHERE npcid = 90").fetchone()[0], "Jurglash and Clan Grobb.")
        self.assertEqual(db.execute("SELECT says FROM queststeps WHERE questid = 5").fetchone()[0], "Warlord Jurglash make Clan Grobb.")
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 3").fetchone()[0], "My potions want wings. Bring me two.")
        self.assertGreater(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 10)
        db.close()
        after = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertIn("applied already", out)
        self.assertEqual(dump(self.db), after)
        code, out = run("--db", self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertIn("0 left", out)
        self.assertEqual(dump(self.db), before)

    def test_only_and_skip(self):
        run("--db", self.db, "--data", self.data, "--only", "d", "--apply")
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 1").fetchone()[0], "Bring me four tails.")
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 3").fetchone()[0], "My potions want wings. Bring me two.")
        db.close()
        code, out = run("--db", self.db, "--data", self.data, "--skip", "d,f", "--apply")
        self.assertEqual(code, 0, out)
        code, out = run("--db", self.db, "--undo", "--only", "d")
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 3").fetchone()[0], "Bring me two wings.")
        self.assertIn("Drinal", db.execute("SELECT offer FROM quests WHERE questid = 1").fetchone()[0])
        db.close()

    def test_a_row_not_as_read_is_left(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE quests SET offer = 'A GM wrote this.' WHERE questid = 1")
        db.execute("UPDATE queststeps SET kind = 'talk' WHERE questid = 4")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("is not as read", out)
        self.assertIn("steps are not as read", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 1").fetchone()[0], "A GM wrote this.")
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 4").fetchone()[0], "Ring.")
        db.close()

    def test_undo_leaves_a_row_changed_since(self):
        run("--db", self.db, "--data", self.data, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE quests SET offer = 'Changed.' WHERE questid = 3")
        db.commit()
        db.close()
        code, out = run("--db", self.db, "--undo")
        self.assertIn("changed since", out)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 3").fetchone()[0], "Changed.")
        self.assertEqual(db.execute("SELECT offer FROM quests WHERE questid = 1").fetchone()[0], "Bring me four tails.")
        db.close()

    def test_refused_on_a_bad_text(self):
        bad = dict(DATA["d"])
        bad["entries"] = [dict(DATA["d"]["entries"][0], after="Bring me two wings — now.")]
        with open(os.path.join(self.data, "d.json"), "w") as f:
            json.dump(bad, f)
        before = dump(self.db)
        code, out = run("--db", self.db, "--data", self.data, "--apply")
        self.assertEqual(code, 1)
        self.assertIn("refused", out)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
