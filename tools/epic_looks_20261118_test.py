#!/usr/bin/env python3
"""epic_looks_20261118.py on a small database: the report writes nothing; --apply gives every record on an epic model that
is no epic, no heirloom and not kept a regular model of its kind (its name's family, a same-named record's model, else
the epic's closest of the kind), the icon only where it was an epic's, and the NPCs' epic hands the epic's closest
regular look, each row with a gm_audit line; epics (quest rewards, Ascended copies, heirlooms' epics, by name, a
"(1H)" variant), heirlooms (table and name) and Twinkle are left; a second --apply finds nothing; --undo puts back
exactly what was there, and a row changed since is left and said."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "epic_looks_20261118.py")
sys.path.insert(0, HERE)
import epic_looks_20261118 as T  # noqa: E402

RANGER_AXE, ROD, GREATHAMMER, SHAMAN_CLUB, STAFF_NEBULA = 0x11666A69, 0x2EDA5089, 0xD8406C85, 0xDF034C5A, 0x7C7DBEA9
AXE_ICON, ROD_ICON = 1081969366, -611086336
s = T.signed


def make(path):
    db = sqlite3.connect(path)
    db.executescript("""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT NOT NULL, itemicon INTEGER NOT NULL,
            equipslot INTEGER NOT NULL, attacktype INTEGER NOT NULL, levelreq INTEGER NOT NULL, model INTEGER NOT NULL,
            color INTEGER NOT NULL DEFAULT -1, weapondamage INTEGER NOT NULL DEFAULT 0, str INTEGER);
        CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, primary_weapon INTEGER, secondary_weapon INTEGER,
            shield INTEGER);
        CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY, class INTEGER NOT NULL, role TEXT NOT NULL,
            epic INTEGER NOT NULL DEFAULT 0);
        CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '',
            source TEXT NOT NULL DEFAULT '');
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '');
        CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, rewarditems TEXT NOT NULL DEFAULT '',
            choices TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
        INSERT INTO quests VALUES (1, 'The Ranger''s Epic Reward', 'EQOA-Data 60/good-x/reward-ranger (tools/import_quests.py)');
        INSERT INTO queststeps VALUES (1, 2, '', '{"options":[{"answer":"Axe","items":"10"}]}');
        INSERT INTO itemascended VALUES (11, 10, '', '');
        INSERT INTO heirlooms VALUES (12, 1, 'main', 10);
    """)
    rows = [
        (10, "Quest Axe of the Ranger", AXE_ICON, 14, 1, 60, s(RANGER_AXE)),        # an epic quest's reward
        (11, "Ascended Quest Axe of the Ranger", AXE_ICON, 14, 1, 60, s(RANGER_AXE)),  # its Ascended copy
        (12, "Heirloom Axe of Innocence", AXE_ICON, 14, 1, 1, s(RANGER_AXE)),       # an heirloom
        (13, "Heirloom Something", AXE_ICON, 14, 1, 1, s(RANGER_AXE)),               # named Heirloom
        (14, "Axe of Nature's Purity", AXE_ICON, 14, 1, 60, s(RANGER_AXE)),          # an epic by name
        (15, "Staff of Nebula (1H)", 1682985694, 12, 3, 49, s(STAFF_NEBULA)),       # an epic's variant
        (16, "Twinkle", 2051918763, 14, 1, 60, s(0xD03942DE)),                       # the owner's look
        (20, "Bronze Bearded Axe", AXE_ICON, 12, 1, 20, s(RANGER_AXE)),             # -> hatchet, icon too
        (21, "Moss Covered Axe", 219574349, 12, 1, 34, s(RANGER_AXE)),              # -> hatchet, its icon kept
        (22, "Urgaunt", 219574349, 15, 2, 46, s(RANGER_AXE)),                       # 2h slash, no name -> greataxe
        (23, "Book of Logic", ROD_ICON, 18, 0, 30, s(ROD)),                         # held book -> tome
        (24, "Wand of Insight", ROD_ICON, 18, 0, 30, s(ROD)),                       # held wand -> crystal wand
        (25, "Brass Partizan", -1279264174, 15, 6, 18, s(GREATHAMMER)),             # -> partizan
        (26, "Minstrel's Edge", 1046403209, 12, 1, 48, s(0xF4DD991A)),             # twin below -> scimitar
        (27, "minstrel's edge", 506409893, 12, 1, 48, s(0x47D981E0)),             # the twin, regular already
        (28, "Pain Crusher", 469503945, 12, 3, 36, s(SHAMAN_CLUB)),                # -> club
    ]
    db.executemany("INSERT INTO itempattern (patternid, itemname, itemicon, equipslot, attacktype, levelreq, model) "
                   "VALUES (?, ?, ?, ?, ?, ?, ?)", rows)
    db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
        (1, "kobold flayer", s(SHAMAN_CLUB), 0, 0),
        (2, "Guitarhero", s(0x8E85E4A6), s(0x9EA9384F), 0),
        (3, "a plain orc", s(0x0A6AF116), 0, 0),
    ])
    db.commit()
    db.close()


def run(path, *args):
    p = subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, *args], capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def snapshot(path):
    db = sqlite3.connect(path)
    out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1").fetchall() for t in ("itempattern", "npcs")}
    db.close()
    return out


def looks(path):
    db = sqlite3.connect(path)
    out = {r[0]: (r[1] & 0xFFFFFFFF, r[2]) for r in db.execute("SELECT patternid, model, itemicon FROM itempattern")}
    db.close()
    return out


class EpicLooks(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "eqoa.db")
        make(self.db)

    def tearDown(self):
        self.dir.cleanup()

    def test_kept_names_petitions_2355_2356(self):
        # the owner, 2026-11-18: Fang of the Downfall and the Legacy Maxillioan Staff keep their epic looks
        for name in ("Fang of the Downfall", "Legacy Maxillioan Staff", "Twinkle"):
            self.assertIn(T.norm(name), T.KEEP_NAMES, name)

    def test_report_writes_nothing(self):
        before = snapshot(self.db)
        code, out = run(self.db)
        self.assertEqual(code, 0, out)
        self.assertIn("items to change: 8", out)
        self.assertIn("NPC rows: 2", out)
        self.assertEqual(before, snapshot(self.db))

    def test_apply_reapply_undo(self):
        before = snapshot(self.db)
        code, out = run(self.db, "--apply")
        self.assertEqual(code, 0, out)
        got = looks(self.db)
        for pid in (10, 11, 12, 13, 14, 15, 16, 27):
            self.assertEqual(got[pid], (before["itempattern"][[r[0] for r in before["itempattern"]].index(pid)][6] & 0xFFFFFFFF,
                                        before["itempattern"][[r[0] for r in before["itempattern"]].index(pid)][2]), pid)
        self.assertEqual(got[20], (T.M["hatchet"], T.MODEL_ICON[T.M["hatchet"]]))
        self.assertEqual(got[21], (T.M["hatchet"], 219574349))
        self.assertEqual(got[22][0], T.M["greataxe"])
        self.assertEqual(got[23], (T.M["tome"], T.MODEL_ICON[T.M["tome"]]))
        self.assertEqual(got[24], (T.M["crystalwand"], T.MODEL_ICON[T.M["crystalwand"]]))
        self.assertEqual(got[25][0], T.M["partizan"])
        self.assertEqual(got[26], (0x47D981E0, 506409893))
        self.assertEqual(got[28][0], T.M["club"])
        db = sqlite3.connect(self.db)
        npcs = {r[0]: tuple((v or 0) & 0xFFFFFFFF for v in r[1:]) for r in
                db.execute("SELECT id, primary_weapon, secondary_weapon, shield FROM npcs")}
        self.assertEqual(npcs[1], (T.M["club"], 0, 0))
        self.assertEqual(npcs[2], (T.M["katana"], T.M["stiletto"], 0))
        self.assertEqual(npcs[3], (0x0A6AF116, 0, 0))
        self.assertEqual(db.execute("SELECT COUNT(*) FROM gm_audit").fetchone()[0], 10)
        weapondamage = db.execute("SELECT weapondamage, color, str FROM itempattern WHERE patternid = 20").fetchone()
        self.assertEqual(weapondamage, (0, -1, None))
        db.close()
        code, out = run(self.db, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("nothing to do", out)
        code, out = run(self.db, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(before, snapshot(self.db))

    def test_undo_leaves_a_row_changed_since(self):
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE itempattern SET model = 123 WHERE patternid = 20")
        db.commit()
        db.close()
        code, out = run(self.db, "--undo")
        self.assertEqual(code, 1, out)
        self.assertIn("changed since", out)
        got = looks(self.db)
        self.assertEqual(got[20][0], 123)
        self.assertEqual(got[21], (RANGER_AXE, 219574349))


if __name__ == "__main__":
    unittest.main()
