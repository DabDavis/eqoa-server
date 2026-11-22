#!/usr/bin/env python3
"""Tests for dwarf_epic_choice_20261122.py on a small database shaped as the shards' level 60 chains.

    python3 -E -s tools/dwarf_epic_choice_20261122_test.py
"""
import io
import os
import sqlite3
import sys
import tempfile
import unittest
from contextlib import redirect_stdout

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import dwarf_epic_choice_20261122 as t  # noqa: E402

CUSTOM = list(range(11, 33))
# chain: (first id, recruiters [(classes, disc races)], number of ep quests); the shards' shapes as read
SHAPES = {
    "good-tank": (101034, [((2,), (0, 1, 4, 8)), ((0,), (0, 3, 4, 6, 7))], 3, {0, 2}),
    "evil-tank": (101076, [((0,), (0, 2, 5, 9)), ((3,), (0, 2, 4, 5, 8, 9))], 3, {0, 3}),
    "good-melee": (101044, [((5,), (0, 1)), ((6,), (0, 1, 4, 6, 7)), ((1, 4), (0, 1))], 3, {1, 4, 5, 6}),
    "evil-melee": (101086, [((6,), (0, 2, 3))], 3, {6}),
    "good-healer": (101067, [((9,), (0, 1, 3, 4, 7, 8)), ((8,), (6,)), ((7,), (0, 1, 7))], 3, {7, 8, 9}),
    "evil-healer": (101107, [((9,), (0, 2)), ((8,), (5, 9))], 3, {8, 9}),
    "good-caster": (101057, [((13, 14), (0, 1, 3, 8)), ((12,), (0, 1, 3, 8)), ((10,), (0, 1, 3, 8))], 3,
                    {10, 12, 13, 14}),
    "evil-caster": (101096, [((12,), (0, 2)), ((13, 14), (0, 2)), ((10, 11), (0, 2, 3, 4, 8, 9))], 3,
                    {10, 11, 12, 13, 14}),
}
QCOLS = ("questid", "name", "enabled", "giver", "minlevel", "maxlevel", "classes", "races", "requires", "repeatable",
         "offer", "accept", "decline", "declined", "started", "source", "notes", "events", "factionreq", "excludes",
         "asks", "daygroup")


def races(disc, dwarf=False):
    return t.int_list(list(disc) + ([4] if dwarf else []) + CUSTOM)


def make_db(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
          giver TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
          classes TEXT NOT NULL DEFAULT '', races TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '',
          repeatable INTEGER NOT NULL DEFAULT 0, offer TEXT NOT NULL DEFAULT '', accept TEXT NOT NULL DEFAULT '',
          decline TEXT NOT NULL DEFAULT '', declined TEXT NOT NULL DEFAULT '', started TEXT NOT NULL DEFAULT '',
          source TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '',
          factionreq TEXT NOT NULL DEFAULT '', excludes TEXT NOT NULL DEFAULT '', asks TEXT NOT NULL DEFAULT '',
          daygroup TEXT NOT NULL DEFAULT '');
        CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
          target TEXT NOT NULL DEFAULT '', journal TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
        CREATE TABLE queststeptargets (questid INTEGER NOT NULL, step INTEGER NOT NULL, target TEXT NOT NULL,
          PRIMARY KEY (questid, step, target));
        CREATE TABLE charquests (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, step INTEGER NOT NULL DEFAULT 1,
          done INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (serverid, questid));
        CREATE TABLE factionstart (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER NOT NULL DEFAULT -1,
          class INTEGER NOT NULL DEFAULT -1, humtype INTEGER NOT NULL DEFAULT -1, faction TEXT NOT NULL, standing INTEGER NOT NULL);
        CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL);
        CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
          faction TEXT NOT NULL);
    """)
    db.execute("INSERT INTO factions VALUES ('Citizens of Neriak', -2000)")
    db.execute("INSERT INTO factionstart (race, class, faction, standing) VALUES (4, 3, 'Citizens of Neriak', 1)")
    db.execute("INSERT INTO npcfaction (name, faction) VALUES ('Tilvyis S`Lynx', 'Citizens of Neriak')")

    def quest(**kw):
        cols = list(kw)
        db.execute(f"INSERT INTO quests ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [kw[c] for c in cols])

    rid = 102000
    for chain, (first, recruiters, n, classes) in SHAPES.items():
        fname, ccls, disc, last_name = t.CHAINS[chain]
        giver = "Tilvyis S`Lynx" if chain == "evil-tank" else f"Giver {chain}"
        starts = []
        for i, (cls, rr) in enumerate(recruiters):
            sid = first - len(recruiters) + i
            starts.append(sid)
            quest(questid=sid, name="The Recruiter", giver="Recruiter", minlevel=60, classes=t.int_list(cls),
                  races=races(rr), source=f"EQOA-Data 60/{chain}/start-g{i + 1} (tools/import_quests.py)")
            db.execute("INSERT INTO queststeps (questid, step, target, journal) VALUES (?, 1, ?, 'Find the giver.')",
                       (sid, giver))
        dwarf_ep = chain in ("good-tank", "evil-tank", "good-melee", "good-healer")
        for k in range(n):
            qid = first + k
            key = "ep1-1" if k == 0 else f"ep2-{k}"
            name = fname if k == 0 else (last_name if k == n - 1 else f"{chain} part {k}")
            quest(questid=qid, name=name, giver=giver, minlevel=60, classes=ccls, races=races(t.CHAINS[chain][2], dwarf_ep),
                  requires="|".join(map(str, starts)) if k == 0 else str(qid - 1),
                  source=f"EQOA-Data 60/{chain}/{key} (tools/import_quests.py)")
            db.execute("INSERT INTO queststeps (questid, step, target) VALUES (?, 1, ?)", (qid, giver))
        for c in sorted(classes):
            quest(questid=rid, name=f"The {t.CLASS_NAMES[c].capitalize()}'s Epic Reward", giver="Ediopolus",
                  classes=str(c), requires=str(first + n - 1),
                  source=f"EQOA-Data 60/{chain}/reward-{t.CLASS_NAMES[c]} (tools/import_quests.py)")
            db.execute("INSERT INTO queststeps (questid, step, target) VALUES (?, 1, 'Ediopolus')", (rid,))
            rid += 1
    db.commit()
    return db


def may_take(db, qid, race, cls, done):
    """handlers.mayTakeAtLocked's class, race, requires and excludes rules (custom races aside)."""
    q = t.rows(db, "SELECT * FROM quests WHERE questid = ?", (qid,))[0]
    cs, rs = t.ints(q["classes"]), t.ints(q["races"])
    if cs and cls not in cs or rs and race not in rs:
        return False
    for group in [g for g in q["requires"].replace(" ", "").split(",") if g]:
        if not any(int(x) in done for x in group.split("|")):
            return False
    return not any(x in done for x in t.ints(q["excludes"]) if x != qid) and qid not in done


def run_chain(db, chain, race, cls, done):
    """Takes the chain from a recruiter to its reward as far as the rules let; True when the reward is taken."""
    starts = [r[0] for r in db.execute("SELECT questid FROM quests WHERE source LIKE ? AND name = 'The Recruiter' "
                                       "ORDER BY questid", (f"%60/{chain}/start-%",))]
    took = next((s for s in starts if may_take(db, s, race, cls, done)), None)
    if took is None:
        return False
    done.add(took)
    eps = [r[0] for r in db.execute("SELECT questid FROM quests WHERE source LIKE ? ORDER BY questid",
                                    (f"EQOA-Data 60/{chain}/ep%",))]
    for q in eps:
        if not may_take(db, q, race, cls, done):
            return False
        done.add(q)
    rewards = [r[0] for r in db.execute("SELECT questid FROM quests WHERE source LIKE '%/reward-%' AND classes = ?",
                                        (str(cls),))]
    got = [r for r in rewards if may_take(db, r, race, cls, done)]
    done.update(got)
    return bool(got)


def quiet(fn, *a):
    with redirect_stdout(io.StringIO()) as out:
        r = fn(*a)
    return r, out.getvalue()


class DwarfEpicChoiceTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        self.db = make_db(self.path)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def apply(self):
        code, out = quiet(t.main, ["--db", self.path, "--shard", "heirloom", "--apply", "--no-backup"])
        self.assertEqual(code, 0, out)
        return out

    def snapshot(self):
        return (self.db.execute("SELECT * FROM quests ORDER BY questid").fetchall(),
                self.db.execute("SELECT * FROM queststeps ORDER BY questid, step").fetchall())

    def test_every_dwarf_class_runs_either_side_never_both(self):
        self.apply()
        for arch, classes in t.ARCHETYPES.items():
            for cls in classes:
                for side, other in (("good", "evil"), ("evil", "good")):
                    done = set()
                    self.assertTrue(run_chain(self.db, f"{side}-{arch}", 4, cls, done), (arch, t.ABBR[cls], side))
                    self.assertFalse(run_chain(self.db, f"{other}-{arch}", 4, cls, done),
                                     f"a dwarf {t.ABBR[cls]} ran both {arch} chains")

    def test_other_races_unchanged(self):
        before = {}
        for arch, classes in t.ARCHETYPES.items():
            for cls in classes:
                for side in t.SIDES:
                    before[(arch, cls, side)] = run_chain(self.db, f"{side}-{arch}", 0, cls, set())
        self.apply()
        for (arch, cls, side), ran in before.items():
            self.assertEqual(run_chain(self.db, f"{side}-{arch}", 0, cls, set()), ran, (arch, t.ABBR[cls], side))
        # a human warrior still runs both tank chains, as he could
        done = set()
        self.assertTrue(run_chain(self.db, "good-tank", 0, 0, done))
        self.assertTrue(run_chain(self.db, "evil-tank", 0, 0, done))
        # a human shadow knight never reaches the good tank chain
        self.assertFalse(run_chain(self.db, "good-tank", 0, 3, set()))

    def test_dwarf_who_took_its_side_cannot_take_the_new_recruiter(self):
        self.apply()
        done = {101032, 101034}  # a dwarf paladin on the good chain already
        n = self.db.execute("SELECT questid FROM quests WHERE source LIKE '%60/evil-tank/start-dwarf%'").fetchone()[0]
        self.assertFalse(may_take(self.db, n, 4, 2, done))

    def test_undo_restores_and_apply_once(self):
        snap = self.snapshot()
        self.apply()
        self.assertNotEqual(self.snapshot(), snap)
        after = self.snapshot()
        code, out = quiet(t.main, ["--db", self.path, "--shard", "heirloom", "--apply", "--no-backup"])
        self.assertIn("applied already", out)
        self.assertEqual(self.snapshot(), after)
        n_audit = self.db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        self.assertGreater(n_audit, 0)
        code, out = quiet(t.main, ["--db", self.path, "--shard", "heirloom", "--undo", "--no-backup"])
        self.assertEqual(code, 0, out)
        self.assertEqual(self.snapshot(), snap)
        self.assertFalse(t.has_table(self.db, t.LOG))
        self.apply()
        self.assertEqual(self.snapshot(), after)

    def test_undo_leaves_a_held_recruiter_and_a_changed_row(self):
        self.apply()
        n = self.db.execute("SELECT questid FROM quests WHERE source LIKE '%60/good-tank/start-dwarf%'").fetchone()[0]
        self.db.execute("INSERT INTO charquests (serverid, questid, done) VALUES (7, ?, 1)", (n,))
        self.db.execute("UPDATE quests SET classes = '0' WHERE questid = 101035")
        self.db.commit()
        code, out = quiet(t.main, ["--db", self.path, "--shard", "heirloom", "--undo", "--no-backup"])
        self.assertIn("characters hold quest", out)
        self.assertIn("changed since", out)
        self.assertIsNotNone(self.db.execute("SELECT 1 FROM quests WHERE questid = ?", (n,)).fetchone())
        self.assertTrue(t.has_table(self.db, t.LOG))

    def test_refuses_when_not_as_read(self):
        snap = self.snapshot()
        self.db.execute("UPDATE quests SET classes = '0, 2, 9' WHERE questid = 101035")
        self.db.commit()
        code, out = quiet(t.main, ["--db", self.path, "--shard", "heirloom", "--apply", "--no-backup"])
        self.assertEqual(code, 2)
        self.assertIn("REFUSED", out)
        self.db.execute("UPDATE quests SET classes = '0, 2' WHERE questid = 101035")
        self.db.execute("UPDATE quests SET name = 'Something Else' WHERE questid = 101076")
        self.db.commit()
        code, out = quiet(t.main, ["--db", self.path, "--shard", "heirloom", "--apply", "--no-backup"])
        self.assertEqual(code, 2)
        self.db.execute(f"UPDATE quests SET name = 'Wands and Branches' WHERE questid = 101076")
        self.db.commit()
        self.assertEqual(self.snapshot(), snap)

    def test_gate_reported(self):
        out = self.apply()
        self.assertIn("tilvyis s`lynx is Citizens of Neriak, KOS to a new dwarf WAR/PAL", out)


if __name__ == "__main__":
    unittest.main()
