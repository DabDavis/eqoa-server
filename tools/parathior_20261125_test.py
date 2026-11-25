#!/usr/bin/env python3
"""parathior_20261125.py on a copy of a shard's database (EQOA_REAL_DB names it; skipped without one: no shard's data is
in this repository): the report writes nothing; --apply makes Parathior spawn-only with his own template and AoE, the
trigger and the three quests' firing step, a gm_audit line a row; a second --apply does nothing; --undo puts every table
the tool touches back exactly (row for row, in key order). The trigger's steps are checked offline too."""
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
import parathior_20261125 as pt  # noqa: E402

TABLES = {"quests": "questid", "queststeps": "questid, step", "npcs": "id", "npcspawnonly": "name",
          "npctemplate": "name", "classtemplates": "name", "spellpattern": "spellid", "spelleffect": "spellid",
          "gmtriggers": "slot"}


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = pt.main(list(args))
    return rc, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return {t: db.execute(f'SELECT * FROM "{t}" ORDER BY {k}').fetchall() for t, k in TABLES.items()}
    finally:
        db.close()


class StepsTest(unittest.TestCase):
    def test_steps(self):
        steps = pt.trigger_steps()
        dos = [s["do"] for s in steps]
        self.assertEqual(dos[:2], ["lock", "clock"])
        self.assertEqual([s["quest"] for s in steps if s["do"] == "questset"], list(pt.QUESTS))
        # the questset comes before the wait, the wait before the spawn, the walk after it
        self.assertLess(dos.index("questset"), dos.index("wait"))
        self.assertLess(dos.index("wait"), dos.index("spawn"))
        self.assertEqual(dos.index("walk"), dos.index("spawn") + 1)
        spawn = steps[dos.index("spawn")]
        self.assertEqual(spawn["npc"], pt.NAME)
        self.assertTrue(spawn["hostile"])
        # the descent's start lies within the walk's reach of the run's place, and the clock outlasts wait + stay
        dx, dz = pt.DESCENT_FROM[0] - pt.ROW_PLACE[1], pt.DESCENT_FROM[2] - pt.ROW_PLACE[3]
        self.assertLess((dx * dx + dz * dz) ** 0.5, pt.REACH)
        self.assertGreater(pt.CLOCK_S, pt.WAIT_S + pt.STAY_MIN * 60)
        self.assertEqual(json.loads(pt.fire_events())[0]["trigger"], pt.TRIGGER)


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB names a shard's database")
class ParathiorTest(unittest.TestCase):
    def test_apply_again_undo(self):
        d = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.addCleanup(shutil.rmtree, d)
        path = os.path.join(d, "eqoa.db")
        shutil.copy(os.environ["EQOA_REAL_DB"], path)
        before = dump(path)
        audit0 = sqlite3.connect(path).execute("SELECT count(*) FROM gm_audit").fetchone()[0]

        rc, out = run("--db", path)
        self.assertEqual(rc, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(dump(path), before)

        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        self.assertIsNotNone(db.execute("SELECT 1 FROM npcspawnonly WHERE name = 'parathior'").fetchone())
        tmpl = db.execute("SELECT template FROM npctemplate WHERE name = 'parathior'").fetchone()[0]
        hp, ab = db.execute("SELECT hp_pct, abilities FROM classtemplates WHERE name = ?", (tmpl,)).fetchone()
        if tmpl == pt.KEPT_TEMPLATE:  # a shard's own tuning kept (Live)
            self.assertEqual(hp, [r for r in before["classtemplates"] if r[0] == tmpl][0][2])
        else:
            self.assertEqual(tmpl, "parathior")
            self.assertEqual(hp, pt.HP_PCT)
        self.assertEqual(json.loads(ab)[0]["spell_id"], pt.WRATH)
        eff = json.loads(db.execute("SELECT effects FROM spelleffect WHERE spellid = ?", (pt.WRATH,)).fetchone()[0])
        self.assertEqual(eff["effects"][0]["amount"], -pt.WRATH_DMG)
        for q in pt.QUESTS:
            ev = json.loads(db.execute("SELECT events FROM queststeps WHERE questid = ? AND step = ?",
                                       (q, pt.FIRE_STEP)).fetchone()[0])
            self.assertEqual(ev[0]["trigger"], pt.TRIGGER)
            self.assertNotIn(pt.NOTE_OLD, db.execute("SELECT notes FROM quests WHERE questid = ?", (q,)).fetchone()[0])
        self.assertEqual(db.execute("SELECT count(*) FROM gmtriggers WHERE name = ?", (pt.TRIGGER,)).fetchone()[0], 1)
        logged = db.execute(f"SELECT count(*) FROM {pt.LOG}").fetchone()[0]
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
        self.assertEqual(dump(path), before)


if __name__ == "__main__":
    unittest.main()
