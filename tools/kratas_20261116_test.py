#!/usr/bin/env python3
"""kratas_20261116.py on a copy of a shard's database (EQOA_REAL_DB names it; skipped without one: no shard's data is in
this repository): the report writes nothing; --apply writes the quest, Kratas, his spell, items, recipes, loot, the
bream's box and the trigger with a gm_audit line a row; a second --apply does nothing; --undo puts every table the tool
touches back exactly (row for row, in key order)."""
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
import kratas_20261116 as kt  # noqa: E402

TABLES = {"quests": "questid", "queststeps": "questid, step", "npcs": "id", "npcspawnonly": "name",
          "classtemplates": "name", "spellpattern": "spellid", "spelleffect": "spellid", "itempattern": "patternid",
          "craftrecipes": "id", "loottables": "id", "npcloot": "npc_name, tablename", "fishcatch": "id",
          "gmtriggers": "slot"}


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = kt.main(list(args))
    return rc, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return {t: db.execute(f'SELECT * FROM "{t}" ORDER BY {k}').fetchall() for t, k in TABLES.items()}
    finally:
        db.close()


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB names a shard's database")
class KratasTest(unittest.TestCase):
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
        steps = [r[0] for r in db.execute("SELECT step FROM queststeps WHERE questid = ? ORDER BY step", (kt.QUEST,))]
        self.assertEqual(steps, [1, 2, 3, 4])
        self.assertEqual(db.execute("SELECT items FROM queststeps WHERE questid = ? AND step = 1", (kt.QUEST,)).fetchone()[0],
                         str(kt.JAR))
        self.assertIsNotNone(db.execute("SELECT 1 FROM npcspawnonly WHERE name = 'kratas'").fetchone())
        self.assertEqual(db.execute("SELECT npc_level FROM npcs WHERE id = ?", (kt.KRATAS_ROW,)).fetchone()[0], kt.LEVEL_NEW)
        self.assertEqual(db.execute("SELECT count(*) FROM npcloot WHERE npc_name = 'kratas'").fetchone()[0], 2)
        self.assertEqual(db.execute("SELECT count(*) FROM craftrecipes WHERE id BETWEEN 8001 AND 8005").fetchone()[0], 5)
        self.assertEqual(db.execute("SELECT count(*) FROM gmtriggers WHERE name = ?", (kt.TRIGGER,)).fetchone()[0], 1)
        logged = db.execute(f"SELECT count(*) FROM {kt.LOG}").fetchone()[0]
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
