#!/usr/bin/env python3
"""enraged_vox_20261116.py on a copy of a shard's database (EQOA_REAL_DB names it; skipped without one: no shard's data
is in this repository): the report writes nothing; --apply inserts the reach step 14, moves 14-31 to 15-32, moves the
characters on them (two made here: one on the old loot step, one further on), holds her spawn and adds the trigger,
with a gm_audit line a row; a second --apply does nothing; --undo puts every table the tool touches back exactly."""
import contextlib
import io
import json
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import enraged_vox_20261116 as ev  # noqa: E402

TABLES = {"queststeps": "questid, step", "charquests": "serverid, questid", "npcspawnonly": "name",
          "gmtriggers": "slot", "npcs": "id", "npcrespawn": "npc_row"}
ON_LOOT, ON_LATER = 990000001, 990000002


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = ev.main(list(args))
    return rc, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return {t: db.execute(f'SELECT * FROM "{t}" ORDER BY {k}').fetchall() for t, k in TABLES.items()}
    finally:
        db.close()


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB names a shard's database")
class EnragedVoxTest(unittest.TestCase):
    def test_apply_again_undo(self):
        d = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.addCleanup(shutil.rmtree, d)
        path = os.path.join(d, "eqoa.db")
        shutil.copy(os.environ["EQOA_REAL_DB"], path)
        db = sqlite3.connect(path)
        db.execute("INSERT INTO charquests (serverid, questid, step, count) VALUES (?, ?, 14, 0), (?, ?, 20, 0)",
                   (ON_LOOT, ev.QUEST, ON_LATER, ev.QUEST))
        db.commit()
        db.close()
        before = dump(path)
        audit0 = sqlite3.connect(path).execute("SELECT count(*) FROM gm_audit").fetchone()[0]

        rc, out = run("--db", path)
        self.assertEqual(rc, 0, out)
        self.assertIn("report only", out)
        self.assertIn("characters to move: 2", out)
        self.assertEqual(dump(path), before)

        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        steps = [r[0] for r in db.execute("SELECT step FROM queststeps WHERE questid = ? ORDER BY step", (ev.QUEST,))]
        self.assertEqual(steps, list(range(1, ev.LAST + 2)))
        kind, items, events = db.execute("SELECT kind, items, events FROM queststeps WHERE questid = ? AND step = 14",
                                         (ev.QUEST,)).fetchone()
        self.assertEqual((kind, items), ("reach", str(ev.STEW)))
        self.assertEqual(json.loads(events)[0]["trigger"], ev.TRIGGER)
        self.assertEqual(db.execute("SELECT kind, target, items FROM queststeps WHERE questid = ? AND step = 15",
                                    (ev.QUEST,)).fetchone(), ("loot", ev.VOX, str(ev.SCALE)))
        self.assertEqual(db.execute("SELECT kind, target FROM queststeps WHERE questid = ? AND step = 16",
                                    (ev.QUEST,)).fetchone(), ("talk", "Mistress Sanchez"))
        moved = dict(db.execute("SELECT serverid, step FROM charquests WHERE serverid IN (?, ?) AND questid = ?",
                                (ON_LOOT, ON_LATER, ev.QUEST)))
        self.assertEqual(moved, {ON_LOOT: 14, ON_LATER: 21})
        self.assertIsNotNone(db.execute("SELECT 1 FROM npcspawnonly WHERE name = ?", (ev.SPAWNONLY,)).fetchone())
        steps = json.loads(db.execute("SELECT steps FROM gmtriggers WHERE name = ?", (ev.TRIGGER,)).fetchone()[0])
        self.assertEqual([s for s in steps if s["do"] == "spawn"][0]["npc"], ev.VOX)
        logged = db.execute(f"SELECT count(*) FROM {ev.LOG}").fetchone()[0]
        audit = db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        db.close()
        self.assertEqual(audit - audit0, logged)

        applied = dump(path)
        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("applied already", out)
        self.assertEqual(dump(path), applied)

        rc, out = run("--db", path, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertNotIn("changed since", out)
        self.assertEqual(dump(path), before)


if __name__ == "__main__":
    unittest.main()
