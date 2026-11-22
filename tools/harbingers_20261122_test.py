#!/usr/bin/env python3
"""harbingers_20261122.py: its figures against the board's (t288's gems, 684's levels and hp_pct, the films' coloured
weapons), and on a copy of a shard's database (EQOA_REAL_DB names it; skipped without one: no shard's data is in this
repository): the report writes nothing; --apply writes the set with a gm_audit line; a second --apply is refused;
--undo puts every table the tool touches back exactly (row for row, in key order)."""
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
import harbingers_20261122 as hb  # noqa: E402

TABLES = {"itempattern": "patternid", "npcs": "id", "npcspawnonly": "name", "npctemplate": "name", "classtemplates": "name",
          "gmtriggers": "slot", "npcdeath": "name, trigger", "loottables": "id", "npcloot": "npc_name, tablename",
          "quests": "questid", "queststeps": "questid, step", "npcrespawn": "npc_row", "spellpattern": "spellid",
          "spelleffect": "spellid", "spellgates": "spell", "quest_import_quests": "questid"}


def run(*args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        rc = hb.main(list(args))
    return rc, out.getvalue()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return {t: db.execute(f'SELECT * FROM "{t}" ORDER BY {k}').fetchall() for t, k in TABLES.items()}
    finally:
        db.close()


class FiguresTest(unittest.TestCase):
    def test_gems_are_t288s(self):
        self.assertEqual(hb.GEMS["Green"], {"dex": 50, "HPMAX": 300, "PR": 25})
        self.assertEqual(hb.GEMS["Yellow"], {"wis": 50, "HoT": 75, "LR": 25})  # the film's Yellow Bloodplague
        self.assertEqual(hb.GEMS["White"], {"AC": 300, "POWMAX": 250, "HPMAX": 250})  # the film's White Bloodplague
        self.assertEqual(hb.WHITE_DAMAGE, 900)
        self.assertEqual(set(hb.GEMS), {"Black", "Blue", "Green", "Red", "Yellow", "White"})

    def test_weapons_and_classes(self):
        self.assertEqual(set(hb.WEAPONS), {"Obliterator", "Anarchy", "Rapture", "Bloodstaff", "Mourning Star", "Warfire Ring", "Bloodplague"})
        self.assertEqual(hb.WEAPONS["Bloodplague"]["attacktype"], 1)  # 1HS on film, not t288's 1hp
        self.assertEqual(hb.WEAPONS["Bloodstaff"]["attacktype"], 3)   # 1HB on film
        self.assertEqual(hb.WEAPONS["Mourning Star"]["classuse"], 512 + 128 + 256)
        self.assertEqual(hb.WEAPONS["Bloodplague"]["classuse"], 32 + 2)

    def test_levels_and_hp_are_the_boards(self):
        want = {"Khal Viceroy": (63, 7595), "Khal Hound": (63, 316), "Khal Assault Infantry": (62, 1305), "Khal Elite Infantry": (64, 1843),
                "Khal Cavalry": (63, 759), "Khal Dark Templar": (65, 1610), "Khal Berserker": (65, 751), "Khal Warlord": (65, 1073),
                "Khal Conscript": (62, 261), "Khal Garrison": (62, 783)}
        for n, (lv, hp) in want.items():
            self.assertEqual(hb.TEMPLATES[n][:2], (lv, hp), n)

    def test_melee_rises_with_the_top_hit(self):
        class Db:
            def execute(self, *a):
                class R:
                    def fetchone(self):
                        return None
                return R()
        import seed_guide_gaps as sgg
        has = sgg.has
        sgg.has = lambda db, t: False
        try:
            self.assertLess(hb.melee_for(Db(), 63, 1250), hb.melee_for(Db(), 63, 2500))
            self.assertEqual(hb.melee_for(Db(), 68, 3520) > 0, True)
        finally:
            sgg.has = has

    def test_triggers_and_deaths(self):
        trig = hb.triggers()
        self.assertEqual(sorted(t["slot"] for t in trig), sorted(hb.SLOTS.values()))
        names = {t["name"] for t in trig}
        for n, t, c, _ in hb.DEATHS:
            self.assertIn(t, names)
            self.assertTrue(0 < c <= 100)
        rc, out = run("--dump")
        d = json.loads(out)
        self.assertEqual(len(d["triggers"]), len(trig))
        for t in d["triggers"]:
            self.assertLessEqual(len(t["steps"]), 100, t["name"])


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB names a shard's database")
class RealTest(unittest.TestCase):
    def test_apply_again_undo(self):
        d = tempfile.mkdtemp(dir=os.environ.get("TMPDIR"))
        self.addCleanup(shutil.rmtree, d)
        path = os.path.join(d, "eqoa.db")
        shutil.copy(os.environ["EQOA_REAL_DB"], path)
        before = dump(path)
        audit0 = sqlite3.connect(path).execute("SELECT count(*) FROM gm_audit").fetchone()[0]

        rc, out = run("--db", path)
        self.assertEqual(rc, 0, out)
        self.assertEqual(dump(path), before, "the report wrote")

        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(path)
        # every weapon and colour has one canonical record; Hogarth's table names it
        canon = db.execute("SELECT family, colour, count(*) FROM gemsetting WHERE kind = 'weapon' AND canonical = 1 GROUP BY 1, 2").fetchall()
        self.assertEqual(len(canon), 7 * 7)
        self.assertTrue(all(n == 1 for _, _, n in canon))
        self.assertEqual(db.execute("SELECT count(*) FROM gemsetting WHERE kind = 'gem'").fetchone()[0], 6)
        wb = db.execute("""SELECT i.weapondamage, i.sta, i.AC, i.POWMAX, i.HPMAX FROM gemsetting g JOIN itempattern i ON i.patternid = g.patternid
            WHERE g.family = 'bloodplague' AND g.colour = 'white' AND g.canonical = 1""").fetchone()
        self.assertEqual(wb, (900, 25, 300, 250, 250))  # the film's White Bloodplague
        yb = db.execute("""SELECT i.weapondamage, i.sta, i.wis, i.HoT, i.LR, i.str FROM gemsetting g JOIN itempattern i ON i.patternid = g.patternid
            WHERE g.family = 'bloodplague' AND g.colour = 'yellow' AND g.canonical = 1""").fetchone()
        self.assertEqual(yb, (700, 25, 50, 75, 25, 0))
        self.assertEqual(db.execute("SELECT str, HoT FROM itempattern WHERE patternid = 1023").fetchone(), (0, 75))
        # Part II is off
        self.assertEqual(db.execute("""SELECT q.enabled FROM quests q JOIN quest_import_quests i ON i.questid = q.questid
            WHERE i.key = ?""", (hb.PART2_KEY,)).fetchone(), (0,))
        self.assertEqual(db.execute("SELECT count(*) FROM gmtriggers WHERE slot BETWEEN 300 AND 309").fetchone()[0], len(hb.SLOTS))
        self.assertEqual(db.execute("SELECT count(*) FROM npcs WHERE npc_name IN ('Khal Conscript', 'Khal Garrison', 'Khal Captain')").fetchone()[0], 37)
        self.assertEqual(db.execute("SELECT npc_type, npc_level FROM npcs WHERE npc_name = 'Anibael'").fetchone(), (128, 63))
        self.assertGreater(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], audit0)
        db.close()

        rc, out = run("--db", path, "--apply")
        self.assertEqual(rc, 1, out)
        self.assertIn("refused", out)

        rc, out = run("--db", path, "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(dump(path), before, "the undo is not exact")


if __name__ == "__main__":
    unittest.main()
