#!/usr/bin/env python3
"""tools/fix_quest_targets.py against a made-up world: a loot step whose
journal names ravens and magpies, and that counts only the ravens, gets the
magpies near its giver; the controls get nothing: a creature its journal
names as a place ("near the bandit fort") or as company ("with the gnoll
champions"), a creature said with its own word before it ("blood flies"), one
of the right name too far off or too high, and a step whose creature is one
known one. A quest whose first step wants items nothing gives gets them on
taking it; one whose give step wants roots nothing drops gets a loot step
before it, the steps after moving down with the characters at them. --apply
then --undo leaves the quest tables as they were, a second --apply is
refused, and a name lowered since (another tool's) is still taken back. No
game data.

    python3 tools/fix_quest_targets_test.py
"""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_quest_targets.py")
sys.path.insert(0, HERE)
import fix_quest_targets as fqt  # noqa: E402
import import_quests as iq  # noqa: E402

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, npc_level INTEGER, npc_type INTEGER);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT NOT NULL);
CREATE TABLE npcitem (itemid INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, patternid INTEGER NOT NULL);
CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL, digest TEXT NOT NULL,
    enabled INTEGER NOT NULL);
"""

# the giver at (1000, 1000) in world 0; a creature's spawns, (name, x, z, level, npc_type)
NPCS = [
    ("Sythonis", 1000, 1000, 50, 128), ("Merchant Rokar", 1050, 1000, 15, 129),
    ("raven", 1700, 1000, 2, 0), ("raven", 1750, 1000, 2, 0),
    ("Magpie ", 1300, 1000, 1, 0), ("magpie", 1350, 1000, 2, 0), ("Magpie ", 1400, 1050, 3, 0),
    ("crow", 1200, 1000, 3, 0),
    # the controls' creatures
    ("bandit", 1100, 1100, 3, 0), ("orc", 1150, 1150, 3, 0), ("orc", 1160, 1150, 3, 0),
    ("gnoll champion", 1100, 1200, 4, 0), ("Vithara", 1120, 1200, 4, 0),
    ("bloodflies", 1200, 1200, 3, 0), ("bloodflies", 1220, 1200, 3, 0), ("swamp flies", 1210, 1210, 3, 0),
    ("rat", 9000, 9000, 2, 0), ("large rat", 1100, 1300, 2, 0), ("giant rat", 1110, 1300, 30, 0),
    # the item fixes' NPCs
    ("Sundro", 3000, 3000, 40, 128), ("Jebdo", 3100, 3000, 40, 128),
    ("Agent", 5000, 5000, 60, 128), ("Assistant", 5010, 5000, 60, 128), ("orc sapper", 5500, 5000, 48, 0),
]
ITEMS = [(10, "chipped raven beak"), (11, "a loot"), (12, "rat fur"), (20, "A Useless Sword"), (21, "Note from Sundro"),
         (22, "bitter roots"), (23, "Legend")]


class FixQuestTargets(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript(SCHEMA)
        iq.ensure_quest_tables(db)
        db.executemany("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type) VALUES (?, 0, ?, 0, ?, ?, ?)",
                       NPCS)
        db.executemany("INSERT INTO itempattern VALUES (?, ?)", ITEMS)
        q = "INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (?, ?, 1, ?, ?)"
        s = "INSERT INTO queststeps (questid, step, kind, target, items, count, chance, journal) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
        db.execute(q, (1, "New Gear", "Sythonis", 4))
        db.execute(s, (1, 1, "talk", "Merchant Rokar", "", 1, 0, "Speak with Merchant Rokar."))
        db.execute(s, (1, 2, "loot", "raven", "10", 1, 0.5, "Take the chipped raven beak from the ravens and magpies outside the city."))
        db.execute(s, (1, 3, "give", "Sythonis", "10", 1, 0, "Bring the chipped raven beak to Sythonis."))
        # the controls, one a step
        db.execute(q, (2, "Controls", "Sythonis", 4))
        for n, (target, journal) in enumerate([
                ("orc", "Kill the orcs near the bandit fort for a loot."),
                ("orc", "Kill the orcs with the gnoll champions for a loot."),
                ("bloodflies", "Take a loot from the blood flies round the city."),
                ("large rat", "Kill rats outside the city for rat fur."),
                ("crow", "Kill the crow by the gate for a loot."),
                ("Vithara", "Slay Vithara and the gnoll champions for a loot.")], start=1):
            db.execute(s, (2, n, "loot", target, "11" if target != "large rat" else "12", 1, 0.5, journal))
        # the item fixes' quests, by their import keys
        db.execute(q, (3, "The Legend of Altivo", "Sundro", 41))
        db.execute(s, (3, 1, "give", "Jebdo", "20, 21", 1, 0, "Find Jebdo."))
        db.execute(s, (3, 2, "give", "Sundro", "23", 1, 0, "Bring the legend to Sundro."))
        db.execute("UPDATE queststeps SET events = ? WHERE questid = 3 AND step = 1", ('[{"on":"done","do":"give","items":"23"}]',))
        db.execute(q, (4, "The Agent of Growth", "Agent", 60))
        db.execute(s, (4, 1, "talk", "Assistant", "", 1, 0, "Speak with the assistant."))
        db.execute(s, (4, 2, "give", "Assistant", "22*3", 1, 0, "Bring three bitter roots."))
        db.execute(s, (4, 3, "talk", "Agent", "", 1, 0, "Return to the Agent."))
        db.executemany("INSERT INTO quest_import_quests VALUES (?, ?, 'f.json', '', 1)",
                       [("Misc/zentars-keep-pet/p1", 3), ("Portal/zaoran-plateau/growth", 4)])
        # characters on the quests: one at the give step that moves down, one before it
        db.executemany("INSERT INTO charquests (serverid, questid, step) VALUES (?, ?, ?)", [(7, 4, 2), (8, 4, 1), (7, 1, 2)])
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def db(self):
        return sqlite3.connect(self.path, isolation_level=None)

    def run_tool(self, *args, ok=True):
        r = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *args],
                           capture_output=True, text=True)
        if ok and r.returncode != 0:
            self.fail(f"{args}: exit {r.returncode}\n{r.stdout}\n{r.stderr}")
        return r

    def state(self):
        db = self.db()
        out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in ("quests", "queststeps", "charquests")}
        db.close()
        return out

    def test_the_journals_creatures_near_the_quest_and_none_of_the_controls(self):
        db = self.db()
        _, _, rows, changes, undecided, gaps, fixes, _ = fqt.plan(db)
        by = {(r["q"]["questid"], r["st"]["step"]): r for r in rows}
        added = {(r["q"]["questid"], r["st"]["step"]): add for r, target, add, why in changes}
        self.assertEqual(added.get((1, 2)), ["magpie"], "the ravens' step takes the magpies its journal names")
        self.assertIn("journal", by[(1, 2)]["flags"])
        for step in range(1, 7):
            self.assertNotIn((2, step), added, f"control step {step}: {by[(2, step)]['st']['journal']!r} added "
                                                f"{added.get((2, step))}")
        # the rats: the line's "rats" are named, but the only rat stands 9,000 off (and a giant rat is 30)
        self.assertEqual(by[(2, 4)]["sources"], [])
        # Vithara is one known creature: her company is not where her loot drops (the line names the
        # champions as her killers' too: they are a source, and still not added)
        self.assertTrue(by[(2, 6)]["named"])
        self.assertEqual(by[(2, 6)]["sources"], ["gnoll champion"])
        # the orcs' step is no one creature's: the champions are left out by the line's "with", the swamp
        # flies by "blood flies" being a kind of their own
        self.assertFalse(by[(2, 2)]["named"])
        self.assertEqual((by[(2, 1)]["sources"], by[(2, 2)]["sources"], by[(2, 3)]["sources"]), ([], [], []))
        db.close()

    def test_the_spelling_written_is_the_npcs_own_trimmed(self):
        db = self.db()
        w = fqt.World(db)
        self.assertEqual(w.spell("magpie"), "Magpie", "two 'Magpie ' and one 'magpie': the commoner, trimmed")
        db.close()

    def test_items_nothing_gives(self):
        db = self.db()
        _, _, _, _, _, gaps, fixes, left = fqt.plan(db)
        wanted = sorted((q["questid"], pid) for q, st, pid, name, what in gaps)
        self.assertEqual(wanted, [(3, 20), (3, 21), (4, 22)], "the legend (23) is given by step 1's done: no gap")
        self.assertEqual(len(fixes), 2, left)
        db.close()

    def test_apply_then_undo_is_exact(self):
        before = self.state()
        self.run_tool("--apply")
        db = self.db()
        self.assertEqual(db.execute("SELECT target FROM queststeptargets WHERE questid = 1 AND step = 2").fetchall(), [("Magpie",)])
        self.assertEqual(db.execute("SELECT events FROM quests WHERE questid = 3").fetchone()[0],
                         '[{"on":"accept","do":"give","items":"20, 21"}]')
        steps = db.execute("SELECT step, kind, target, items, count, chance FROM queststeps WHERE questid = 4 ORDER BY step").fetchall()
        self.assertEqual(steps, [(1, "talk", "Assistant", "", 1, 0.0), (2, "loot", "orc sapper", "22*3", 3, 0.5),
                                 (3, "give", "Assistant", "22*3", 1, 0.0), (4, "talk", "Agent", "", 1, 0.0)])
        self.assertEqual(db.execute("SELECT serverid, step FROM charquests WHERE questid = 4 ORDER BY serverid").fetchall(),
                         [(7, 3), (8, 1)], "the character at the give step moves down with it; the one before it stays")
        db.close()
        r = self.run_tool("--apply", ok=False)
        self.assertNotEqual(r.returncode, 0, "a second --apply is refused")
        # another tool lowers the name since: still taken back
        db = self.db()
        db.execute("UPDATE queststeptargets SET target = 'magpie'")
        db.close()
        r = self.run_tool("--undo")
        self.assertIn("0 changed since", r.stdout)
        self.assertEqual(self.state(), before, "undo leaves the quest tables as they were")
        db = self.db()
        self.assertEqual(db.execute("SELECT count(*) FROM queststeptargets").fetchone()[0], 0)
        self.assertFalse(fqt.has_table(db, "quest_targets_fix_log"))
        db.close()

    def test_a_row_changed_since_is_left(self):
        self.run_tool("--apply")
        db = self.db()
        db.execute("UPDATE quests SET events = '' WHERE questid = 3")  # a GM's change since
        db.close()
        r = self.run_tool("--undo")
        self.assertIn("1 changed since left as they are", r.stdout)


if __name__ == "__main__":
    unittest.main()
