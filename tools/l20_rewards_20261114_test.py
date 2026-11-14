#!/usr/bin/env python3
"""tools/l20_rewards_20261114.py on a database of its own: a one-path quest made a pick with weapons and its finisher
given the weapon and the other path's scroll, a pick given its weapons and its picker the weapon, the new items made,
a second --apply nothing, --undo exact, a quest changed since refusing everything."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import l20_rewards_20261114 as tool  # noqa: E402

TOOL = tool.__file__
COLS = ("classuse", "raceuse", "attacktype", "equipslot", "model", "itemicon", "weapondamage", "unk2", "unk3", "unk4",
        "unk5", "agi", "wis", "sta", "cha", "intelligence", "POWMAX", "lore", "trade", "maxhp")


def make(path, step_spells="1436"):
    db = sqlite3.connect(path)
    db.executescript(f"""
        CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, itemdesc TEXT,
            {", ".join(c + " INTEGER DEFAULT 0" for c in COLS)});
        CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT);
        CREATE TABLE quests (questid INTEGER PRIMARY KEY, source TEXT);
        CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, spells TEXT DEFAULT '', rewarditems TEXT DEFAULT '',
            choices TEXT DEFAULT '', PRIMARY KEY (questid, step));
        CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charName TEXT);
        CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER DEFAULT 0, done INTEGER, picks TEXT DEFAULT '',
            PRIMARY KEY (serverid, questid));
        CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, stackleft INTEGER,
            remainHP INTEGER, remaincharge INTEGER, patternid INTEGER, equiploc INTEGER, location INTEGER, listnumber INTEGER);
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
            action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
    """)
    for pid, name in tool.WEAPONS.items():
        db.execute("INSERT INTO itempattern (patternid, itemname, itemdesc, equipslot, maxhp) VALUES (?, ?, 'x', 12, 2000)",
                   (pid, name))
    for name, cls in (("Shadow Tower", 8), ("Pillar of Might", 1)):
        db.execute("INSERT INTO itempattern (patternid, itemname, itemdesc, classuse) VALUES (?, ?, 'scroll', ?)",
                   (9000 + cls, name, cls))
    spells = {v: k for k, v in vars(tool).items() if isinstance(v, int) and k.isupper() and k not in ("INV_CAP",)}
    for sid in {s for (_, paths) in tool.QUESTS.values() for s, _ in paths}:
        db.execute("INSERT INTO spellpattern VALUES (?, ?)", (sid, spells.get(sid, str(sid))))
    db.execute("UPDATE spellpattern SET spellname = 'Bloodwasp' WHERE spellid = 1436")
    db.execute("UPDATE spellpattern SET spellname = 'Shadow Tower' WHERE spellid = 700212")
    db.execute("UPDATE spellpattern SET spellname = 'Bellow' WHERE spellid = 1588")
    db.execute("UPDATE spellpattern SET spellname = 'Pillar of Might' WHERE spellid = 1534")
    for qid, ((given, item), _) in tool.QUESTS.items():
        city = "freeport"
        db.execute("INSERT INTO quests VALUES (?, ?)", (qid, f"EQOA-Data 1-20/{city}/{'shadowknight' if qid == 100150 else 'x'}/L20"))
        db.execute("INSERT INTO queststeps (questid, step, kind, spells, rewarditems) VALUES (?, 9, 'give', ?, ?)",
                   (qid, step_spells if qid == 100150 else str(given), "" if item is None else str(item)))
    db.execute("INSERT INTO quests VALUES (100556, 'EQOA-Data 1-20/qeynos/warrior/L20')")
    db.execute("INSERT INTO queststeps (questid, step, kind, choices) VALUES (100556, 12, 'talk', ?)",
               ('{"ask":"Which do you take?","options":[{"answer":"Bellow","says":"Then it is yours.","items":"8568"},'
                '{"answer":"Pillar of Might","says":"Then it is yours.","items":"8569"}]}',))
    db.executemany("INSERT INTO characters VALUES (?, ?)", ((110, "Smoke"), (5, "Picker")))
    db.execute("INSERT INTO charquests (serverid, questid, done) VALUES (110, 100150, 1)")
    db.execute("INSERT INTO charquests (serverid, questid, done, picks) VALUES (5, 100556, 1, '12:1')")
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    try:
        return [list(db.execute(f"SELECT * FROM {t} ORDER BY 1")) for t in ("itempattern", "queststeps", "charinventory")]
    finally:
        db.close()


class L20Rewards(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def run_tool(self, *args):
        return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", self.db, *args], capture_output=True, text=True)

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        ch = db.execute("SELECT choices, spells FROM queststeps WHERE questid = 100150").fetchone()
        self.assertIn('"items":"5501","spells":"700212"', ch[0])
        self.assertEqual(ch[1], "")
        self.assertIn("8568, 8575", db.execute("SELECT choices FROM queststeps WHERE questid = 100556").fetchone()[0])
        given = sorted(db.execute("SELECT serverid, patternid FROM charinventory").fetchall())
        self.assertEqual(given, [(5, 8576), (110, 8562), (110, 9008)])
        self.assertEqual(db.execute("SELECT COUNT(*) FROM itempattern WHERE itemname IN "
                                    "('magic greatclub', 'magic spear', 'alchemical tome')").fetchone()[0], 3)
        db.close()
        self.assertIn("applied already", self.run_tool("--apply").stdout)
        r = self.run_tool("--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_refused(self):
        make(self.db, step_spells="9")
        before = dump(self.db)
        r = self.run_tool("--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("refused, nothing done", r.stdout)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
