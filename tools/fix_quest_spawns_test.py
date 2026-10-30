#!/usr/bin/env python3
"""tools/fix_quest_spawns.py against a made-up world holding the five Jarek's
Creation quests as the import wrote them before their content changed (read
from tools/quests/data, so the test follows the content): --apply writes the
content's values, gives the creation its body and marks spawned only the
creatures a quest's spawn alone uses that the quest seed put in the world; the
controls are left standing: one a later step kills with no spawn, one a quest
talks to and gets from, one from another import, one another quest kills with
no spawn, one players talk to, and a GM's row is kept. --undo leaves the
database as it was (its dump identical); a second --apply is refused; a value
changed since is left by --undo. No game data but the content files'.

    python3 tools/fix_quest_spawns_test.py
"""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_quest_spawns.py")
sys.path.insert(0, HERE)
import fix_quest_spawns as fqs  # noqa: E402
import import_quests as iq  # noqa: E402

BEETLE = 0x4140233B


class FixQuestSpawns(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path, isolation_level=None)
        db.executescript("""
            CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL,
                y_coord REAL, z_coord REAL, npc_level INTEGER, npc_type INTEGER, modelid INTEGER, size REAL);
            CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL, file TEXT NOT NULL,
                digest TEXT NOT NULL, enabled INTEGER NOT NULL);
            CREATE TABLE quest_npc_seed_rows (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, source TEXT NOT NULL,
                template_row INTEGER, seedset TEXT NOT NULL DEFAULT '1-20');
            CREATE TABLE prh_import_npcs (src_id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, status TEXT NOT NULL,
                reason TEXT, dst_id INTEGER, fixes TEXT);
            CREATE TABLE npcpaths (npcid INTEGER PRIMARY KEY, world INTEGER NOT NULL DEFAULT 0, points TEXT NOT NULL DEFAULT '[]');
            CREATE TABLE npcspawnonly (name TEXT PRIMARY KEY, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        """)
        iq.ensure_quest_tables(db)
        db.execute("CREATE TABLE IF NOT EXISTS queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, "
                   "target TEXT NOT NULL, PRIMARY KEY (questid, step, target))")
        self.npc = {}

        def npc(name, kind=0, seed=None, prh=None, model=0x70D898C6, size=1.0, level=20):
            cur = db.execute("INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type, modelid, size) "
                             "VALUES (?, 0, ?, 50, 1000, ?, ?, ?, ?)", (name, 1000 + 10 * len(self.npc), level, kind, model, size))
            self.npc[name] = cur.lastrowid
            if seed:
                db.execute("INSERT INTO quest_npc_seed_rows VALUES (?, ?, ?, NULL, 'qd')", (cur.lastrowid, name, seed))
            if prh:
                db.execute("INSERT INTO prh_import_npcs (src_id, npc_name, world, status, dst_id) VALUES (?, ?, 0, ?, ?)",
                           (cur.lastrowid + 9000, name, prh, cur.lastrowid))

        npc("Jarek N`Toxis", kind=128)
        npc("Jarek's Creation", seed="ff", model=BEETLE, size=0.875, level=11)
        npc("fungus cockroach")
        npc("fungus cockroach")
        npc("lone ghost", seed="the guide's place")
        npc("compelled shade", seed="ff")
        npc("Eljin")
        npc("Armis", seed="the guide's place")
        npc("Lure", seed="by Coach Rubgar")
        npc("Bloodletter", prh="imported")
        npc("stone ant", seed="map")
        npc("Rover", kind=128, seed="ff")
        npc("Old One", seed="ff")
        db.execute("INSERT INTO npcspawnonly VALUES ('Old One', 'a GM''s', 'gm')")

        # the five quests as the import wrote them before the content changed
        content = fqs.content(fqs.NERIAK + [fqs.ROGUE], fqs.CONTENT)
        for i, k in enumerate(fqs.NERIAK + [fqs.ROGUE]):
            q, qid = content[k], 100453 + 10 * i
            notes = q.get("notes", "").replace(fqs.NEW_NOTE, fqs.OLD_NOTE)
            db.execute("INSERT INTO quests (questid, name, enabled, giver, notes) VALUES (?, ?, 1, ?, ?)",
                       (qid, q["name"], q["giver"], notes))
            db.execute("INSERT INTO quest_import_quests VALUES (?, ?, ?, '', 1)", (k, qid, fqs.FILES[k]))
            for n, st in enumerate(q["steps"], 1):
                evs, target = fqs.compact(st.get("events", [])), st.get("target", "")
                if n == 4:
                    evs = fqs.OLD_ROGUE_SPAWN if k == fqs.ROGUE else fqs.OLD_SPAWN
                if n == 5 and k != fqs.ROGUE:
                    target = fqs.OLD_TARGET
                db.execute("INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (?, ?, ?, ?, ?, ?)",
                           (qid, n, st["kind"], target, st.get("journal", ""), evs))

        def quest(qid, giver, steps):
            db.execute("INSERT INTO quests (questid, name, enabled, giver) VALUES (?, ?, 1, ?)", (qid, f"Quest {qid}", giver))
            for n, (kind, target, spawns) in enumerate(steps, 1):
                evs = [{"on": "start", "do": "spawn", "npc": s, "hostile": True} for s in spawns]
                db.execute("INSERT INTO queststeps (questid, step, kind, target, journal, events) VALUES (?, ?, ?, ?, ?, ?)",
                           (qid, n, kind, target, f"step {n}", fqs.compact(evs)))

        quest(200, "Giver", [("talk", "Giver", []), ("kill", "lone ghost", ["lone ghost"])])
        quest(201, "Giver", [("loot", "Armis", ["Armis"]), ("loot", "Armis", [])])
        quest(202, "Lure", [("talk", "Lure", []), ("loot", "Lure", ["Lure"])])
        quest(203, "Giver", [("kill", "Bloodletter", ["Bloodletter"])])
        quest(204, "Giver", [("loot", "stone ant", ["stone ant"])])
        quest(205, "Giver", [("loot", "stone ant", [])])
        quest(206, "Giver", [("kill", "Rover", ["Rover"])])
        quest(207, "Giver", [("talk", "Giver", []), ("loot", "Eljin", ["compelled shade"])])
        quest(208, "Giver", [("kill", "Old One", ["Old One"])])
        npc("Pinned One", seed="ff")
        quest(209, "Giver", [("kill", "Pinned One", ["Pinned One"])])
        db.execute("UPDATE queststeps SET npcid = ? WHERE questid = 209", (self.npc["Pinned One"],))
        db.close()

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *a):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.path, "--no-backup", *a],
                              capture_output=True, text=True)

    def dump(self):
        db = sqlite3.connect(self.path)
        out = "\n".join(db.iterdump())
        db.close()
        return out

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        out = db.execute(sql, args).fetchall()
        db.close()
        return out

    def test_a_report_writes_nothing(self):
        before = self.dump()
        r = self.run_tool()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("13 quest values, 1 bodies", r.stdout)
        self.assertIn("3 to mark spawned only, 1 marked already", r.stdout)
        self.assertEqual(self.dump(), before)

    def test_apply_then_undo(self):
        before = self.dump()
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stderr)
        content = fqs.content(fqs.NERIAK + [fqs.ROGUE], fqs.CONTENT)
        for i, k in enumerate(fqs.NERIAK + [fqs.ROGUE]):
            qid, q = 100453 + 10 * i, content[k]
            evs, target = self.q("SELECT events, target FROM queststeps WHERE questid = ? AND step = 4", qid)[0]
            self.assertEqual(evs, fqs.compact(q["steps"][3]["events"]), k)
            self.assertEqual(json.loads(evs), [{"on": "done", "do": "spawn", "npc": "Jarek's Creation", "hostile": True}], k)
            self.assertEqual(self.q("SELECT target FROM queststeps WHERE questid = ? AND step = 5", qid)[0][0],
                             "Jarek's Creation", k)
            self.assertEqual(self.q("SELECT notes FROM quests WHERE questid = ?", qid)[0][0], q.get("notes", ""), k)
        self.assertEqual(self.q("SELECT modelid, size, npc_level FROM npcs WHERE npc_name = ?", "Jarek's Creation"),
                         [(0x15764DA8, 1.0, 11)])
        self.assertEqual(sorted(self.q("SELECT name, source FROM npcspawnonly")),
                         [("Jarek's Creation", "questspawn"), ("Old One", "gm"), ("compelled shade", "questspawn"),
                          ("lone ghost", "questspawn")])
        self.assertNotEqual(self.run_tool("--apply").returncode, 0, "a second --apply is refused")
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("put back 17", r.stdout)
        self.assertEqual(self.dump(), before)

    def test_another_body(self):
        r = self.run_tool("--apply", "--creation-model", "0x1234ABCD", "--creation-size", "0.5")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.q("SELECT modelid, size FROM npcs WHERE npc_name = ?", "Jarek's Creation"), [(0x1234ABCD, 0.5)])

    def test_a_change_since_is_left(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcspawnonly SET note = 'mine now' WHERE name = 'lone ghost'")
        db.execute("UPDATE queststeps SET target = 'a GM''s' WHERE questid = 100453 AND step = 5")
        db.commit()
        db.close()
        r = self.run_tool("--undo")
        self.assertIn("2 changed since", r.stdout)
        self.assertEqual(self.q("SELECT note FROM npcspawnonly WHERE name = 'lone ghost'"), [("mine now",)])
        self.assertEqual(self.q("SELECT target FROM queststeps WHERE questid = 100453 AND step = 5"), [("a GM's",)])
        self.assertEqual(self.q("SELECT target FROM queststeps WHERE questid = 100463 AND step = 5"), [(fqs.OLD_TARGET,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcspawnonly WHERE name = 'compelled shade'"), [(0,)])

    def test_the_table_made_here_goes_with_the_undo(self):
        db = sqlite3.connect(self.path)
        db.execute("DROP TABLE npcspawnonly")
        db.commit()
        db.close()
        before = self.dump()
        self.assertEqual(self.run_tool("--apply").returncode, 0)
        self.assertEqual(self.q("SELECT count(*) FROM npcspawnonly"), [(4,)], "Old One too: its row is gone with the table")
        self.run_tool("--undo")
        self.assertEqual(self.dump(), before)


if __name__ == "__main__":
    unittest.main()
