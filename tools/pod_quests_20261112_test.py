#!/usr/bin/env python3
"""pod_quests_20261112.py on a copy of a real database (EQOA_REAL_DB, a Heirloom snapshot; skipped without one): the
report writes nothing; --apply writes the quests as the wiki has them (one hand-in each, the standings, the levels, the
pet's two quests, the renames, the loot, the cycle's triggers); a second --apply does nothing; --undo puts every row
back exactly (gm_audit and the counters of the logs aside). The control: a quest a GM changed refuses everything."""
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "pod_quests_20261112.py")


def run(db, *args):
    return subprocess.run([sys.executable, TOOL, "--db", db, *args], capture_output=True, text=True)


def dump(path):
    db = sqlite3.connect(path)
    try:
        return [line for line in db.iterdump() if not line.startswith('INSERT INTO "gm_audit"')
                and "sqlite_sequence" not in line]
    finally:
        db.close()


class PodQuests(unittest.TestCase):
    def setUp(self):
        real = os.environ.get("EQOA_REAL_DB")
        if not real or not os.path.exists(real):
            self.skipTest("EQOA_REAL_DB not set")
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "eqoa.db")
        shutil.copy(real, self.db)
        os.chmod(self.db, 0o644)

    def tearDown(self):
        shutil.rmtree(self.dir, ignore_errors=True)

    def q(self, sql, args=()):
        db = sqlite3.connect(self.db)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_apply_again_undo(self):
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("a report: nothing written", r.stdout)
        self.assertEqual(dump(self.db), before)

        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        quests = {n: (lo, req, fr) for n, lo, req, fr in self.q(
            "SELECT name, minlevel, requires, factionreq FROM quests WHERE giver IN ('Yarfsnoogle', 'Witchdoctor Yurplag', 'Honcho Ulgurglch')")}
        self.assertEqual(quests["Yarfsnoogle's Anatomy"], (1, "", "The Pusling:2000"))
        self.assertEqual(quests["The Plague Weapons"], (60, "", "The Pusling:1000"))
        self.assertEqual(quests["The Fuming Portal"][0::2], (51, "The Pusling:1000"))
        steps = self.q("SELECT s.step, s.kind, s.target FROM queststeps s JOIN quests q ON q.questid = s.questid "
                       "WHERE q.name = 'Yarfsnoogle''s Anatomy' ORDER BY s.step")
        self.assertEqual(steps, [(1, "give", "Yarfsnoogle"), (2, "kill", "Grum")])
        self.assertEqual(len(self.q("SELECT 1 FROM quests WHERE giver = 'Yarfsnoogle''s Pet' AND repeatable = 1")), 2)
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE npc_name = 'Rawlg the Carnage Hound' AND world = 5"), [(1,)])
        self.assertEqual(self.q("SELECT count(*) FROM npcs WHERE npc_name LIKE 'Rawgl%'"), [(0,)])
        self.assertEqual(self.q("SELECT chance, rolls FROM npcloot WHERE npc_name = 'rawlg the carnage hound' AND tablename LIKE 'pod quest %'"), [(100, 1)])
        self.assertEqual(self.q("SELECT chance, rolls FROM npcloot WHERE npc_name = 'moth monarch' AND tablename = 'pod quest moth monarch'"), [(75, 2)])
        self.assertEqual(self.q("SELECT trigger, chance FROM npcdeath WHERE name = 'moth larva'"), [("PoD Moth Metamorphosis", 25.0)])
        self.assertEqual(self.q("SELECT 1 FROM npcspawnonly WHERE name = 'moth monarch'"), [(1,)])
        self.assertEqual(self.q("SELECT classuse FROM itempattern WHERE itemname = 'Totem of Avhi' ORDER BY patternid LIMIT 1"), [(31744,)])
        self.assertEqual(self.q("SELECT equipslot, weapondamage, classuse FROM itempattern WHERE itemname = 'Thorn of Deatharn'"), [(14, 720, 16)])
        applied = dump(self.db)

        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.db), applied)

        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_gm_change_refuses(self):
        db = sqlite3.connect(self.db)
        db.execute("UPDATE quests SET offer = offer || ' (a GM)' WHERE name = 'The Plague Weapons' AND giver = 'Witchdoctor Yurplag'")
        db.commit()
        db.close()
        before = dump(self.db)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("changed since its import", r.stdout)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
