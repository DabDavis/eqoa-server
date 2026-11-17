#!/usr/bin/env python3
"""valkanith_20261117.py on a copy of a shard's database (EQOA_REAL_DB names it; skipped without one: no shard's data is
in this repository): the report writes nothing; --apply (and --apply --t299) writes the quest, the giver, the waves,
Lord Valkanith's template, spell and loot, the rune's use and the trigger with a gm_audit line a row; a second --apply
does nothing; --undo puts every table the tool touches back exactly (row for row, in key order)."""
import contextlib
import io
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import valkanith_20261117 as vt  # noqa: E402

TABLES = {"quests": "questid", "queststeps": "questid, step", "npcs": "id", "npcrespawn": "npc_row",
          "npcundead": "name", "npctemplate": "name", "classtemplates": "name", "spellpattern": "spellid",
          "spelleffect": "spellid", "itempattern": "patternid", "itemeffect": "patternid", "loottables": "id",
          "npcloot": "npc_name, tablename", "gmtriggers": "slot"}


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = vt.main(list(args))
    return rc, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return {t: db.execute(f'SELECT * FROM "{t}" ORDER BY {k}').fetchall() for t, k in TABLES.items()}
    finally:
        db.close()


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB names a shard's database")
class ValkanithTest(unittest.TestCase):
    def once(self, *flags):
        d = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.addCleanup(shutil.rmtree, d)
        path = os.path.join(d, "eqoa.db")
        shutil.copy(os.environ["EQOA_REAL_DB"], path)
        before = dump(path)
        audit0 = sqlite3.connect(path).execute("SELECT count(*) FROM gm_audit").fetchone()[0]

        rc, out = run("--db", path, *flags)
        self.assertEqual(rc, 0, out)
        self.assertIn("report only", out)
        self.assertEqual(dump(path), before)

        rc, out = run("--db", path, "--apply", *flags)
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        steps = [r[0] for r in db.execute("SELECT step FROM queststeps WHERE questid = ? ORDER BY step", (vt.QUEST,))]
        self.assertEqual(steps, [1, 2, 3, 4, 5])
        self.assertEqual(db.execute("SELECT target FROM queststeps WHERE questid = ? AND step = 5", (vt.QUEST,)).fetchone()[0],
                         "Lord Valkanith")
        self.assertEqual(db.execute("SELECT repeatable FROM quests WHERE questid = ?", (vt.QUEST,)).fetchone()[0], 1)
        self.assertEqual([r[0] for r in db.execute("SELECT id FROM npcs WHERE npc_name = ?", (vt.GIVER,))], [vt.GIVER_ROW])
        self.assertEqual(db.execute("SELECT template FROM npctemplate WHERE name = ?", (vt.LORD,)).fetchone()[0], vt.LORD)
        n = db.execute("SELECT count(*) FROM loottables WHERE tablename = ?", (vt.LOOT_TABLE,)).fetchone()[0]
        self.assertEqual(n, 3 if flags else 1)
        self.assertEqual(db.execute("SELECT count(*) FROM npcundead WHERE name IN (?, ?, ?, ?)", vt.UNDEAD).fetchone()[0], 4)
        self.assertEqual(db.execute("SELECT charges FROM itemeffect WHERE patternid = ?", (vt.rune_id(db),)).fetchone()[0], 100)
        self.assertEqual(db.execute("SELECT count(*) FROM gmtriggers WHERE name = ?", (vt.TRIGGER,)).fetchone()[0], 1)
        logged = db.execute(f"SELECT count(*) FROM {vt.LOG}").fetchone()[0]
        audit = db.execute("SELECT count(*) FROM gm_audit").fetchone()[0]
        db.close()
        self.assertEqual(audit - audit0, logged)

        applied = dump(path)
        rc, out = run("--db", path, "--apply", *flags)
        self.assertEqual(rc, 0, out)
        self.assertIn("applied already", out)
        self.assertEqual(dump(path), applied)

        rc, out = run("--db", path, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(dump(path), before)

    def test_apply_again_undo(self):
        self.once()

    def test_apply_again_undo_t299(self):
        self.once("--t299")


if __name__ == "__main__":
    unittest.main()
