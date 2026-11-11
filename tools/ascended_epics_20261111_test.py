#!/usr/bin/env python3
"""ascended_epics_20261111.py on a small database: the report writes nothing; --apply makes the epic rewards'
Ascended records, spells, potion and pet, points the epic quests at them (a level 49 pick's weapon only) and writes
a gm_audit line a row; every Ascended spell's figures are its base's a tenth more but what the petition leaves (the
caster's price, speed, a set figure, a mitigation's percent); a second apply does nothing; --undo puts every table
back exactly. The controls: a spell already named "Ascended <name>" that is no copy refuses, writing nothing; a
record a character holds stays at --undo, and is said."""
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import ascended_epics_20261111 as ae  # noqa: E402

TOOL = os.path.join(HERE, "ascended_epics_20261111.py")
SCROLL = -607649328

SCHEMA = """
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER NOT NULL DEFAULT 0,
    itemicon INTEGER NOT NULL DEFAULT 0, equipslot INTEGER NOT NULL DEFAULT 0, trade INTEGER NOT NULL DEFAULT 1,
    rent INTEGER NOT NULL DEFAULT 1, attacktype INTEGER NOT NULL DEFAULT 0, weapondamage INTEGER NOT NULL DEFAULT 0,
    maxstack INTEGER NOT NULL DEFAULT 1, maxhp INTEGER NOT NULL DEFAULT 0, itemname TEXT NOT NULL,
    itemdesc TEXT NOT NULL DEFAULT '', str INTEGER, sta INTEGER, HPMAX INTEGER, POWMAX INTEGER, PoT INTEGER,
    HoT INTEGER, AC INTEGER, FR INTEGER);
CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL, name TEXT NOT NULL DEFAULT '',
    area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm', target_line TEXT NOT NULL DEFAULT '');
CREATE TABLE foodeffect (patternid INTEGER PRIMARY KEY, duration_s INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE, made TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '');
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL DEFAULT 60,
    power INTEGER NOT NULL DEFAULT 100, recast INTEGER NOT NULL DEFAULT 30, spellname TEXT NOT NULL,
    spelldesc TEXT NOT NULL DEFAULT '');
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '');
CREATE TABLE spellarea (spellid INTEGER PRIMARY KEY, centre INTEGER NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0);
CREATE TABLE classspells (class INTEGER NOT NULL, spellid INTEGER NOT NULL, source TEXT NOT NULL,
    PRIMARY KEY (class, spellid));
CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', level INTEGER NOT NULL DEFAULT 0,
    ac INTEGER NOT NULL DEFAULT 0, str INTEGER NOT NULL DEFAULT 0, hp INTEGER NOT NULL DEFAULT 0,
    wpn1_dmg INTEGER NOT NULL DEFAULT 0, abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm', size REAL NOT NULL DEFAULT 0);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    items TEXT NOT NULL DEFAULT '', rewarditems TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '',
    byclass TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE characters (serverid INTEGER PRIMARY KEY, charname TEXT);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER);
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
"""


def js(scope, effects, **top):
    return json.dumps(dict({"v": 1, "scope": scope, "target_type": "Self" if scope == "self" else "Enemy",
                            "stack": 2, "level": 60}, **top, effects=effects))


def make(path, clash=False):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executemany("INSERT INTO itempattern (patternid, itemname, equipslot, attacktype, weapondamage, str, sta, HPMAX, "
                   "AC, FR, maxstack, itemicon, PoT) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", [
                       (718029, "Rogue's Epic Dagger", 14, 5, 801, 30, None, 150, 0, 0, 1, 1, None),
                       (1342, "Tunic of Fierce Flames", 5, 0, 0, 30, 4, 200, 420, 45, 1, 2, None),
                       (5996, "Heartsbane", 14, 5, 585, 0, 0, 0, 0, 0, 1, 3, None),
                       (700995, "Blacktongue", 14, 5, 577, 0, 0, 0, 0, 0, 1, 4, None),
                       (709300, "Rose of Rehabilitation", -1, 0, 0, None, None, None, None, None, 1, 5, None),
                       (923625, "Severupidus' Cocktail", 16, 10, 0, None, None, None, None, None, 9999, 6, None),
                       (705317, "Fine Red Wine", -1, 0, 0, None, None, None, None, None, 50, 7, 200),
                       (701222, "Severupidus' Cocktail of Torment", -1, 0, 0, None, None, None, None, None, 1, SCROLL, None)])
    db.execute("INSERT INTO itemeffect (patternid, line, area) VALUES (923625, 'Target, Decrease Hitpoints by 1350 to 1400', 10)")
    db.execute("INSERT INTO foodeffect VALUES (705317, 120, 'eqoaemu')")
    spells = [
        (701227, "Sothimus Critical Wounding", "Target, Root, Decrease Hitpoints by 1600 ; Increase 30% based on STR, 18sec",
         js("target", [{"kind": "speed", "amount": -30, "absolute": True},
                       {"kind": "hp", "amount": -1600, "scale": "str", "scale_pct": 30, "hate": 200}], harmful=True, duration_s=18)),
        (701118, "Celedonicus' Grand Finale", "Self, Increase Power by 5000, Decrease Power by 5000, PB AE",
         js("self", [{"kind": "power", "amount": 5000, "scope": "group"}, {"kind": "power", "amount": -5000}], radius=30)),
        (701240, "Talimus's Diamond Barrier", "Self, Mitigate 70% of melee damage up to 12500, Root, 30sec",
         js("self", [{"kind": "invulnerable", "mitigation": {"pct": 70, "max": -12500}},
                     {"kind": "speed", "amount": -50, "absolute": True}], duration_s=30)),
        (701150, "Graal's Final Strike", "Self, Decrease Hitpoints by 400 ; Increase 10% based on STR, 5min",
         js("self", [{"kind": "proc", "procs": [{"kind": "hp", "amount": -400, "scale": "str", "scale_pct": 10,
                                                  "on": "offense", "scope": "target"}]}], duration_s=300)),
        (701145, "Gelledyn's Perfect Vision", "Self, Increase WIS by 340, Set Maximum Power to 1625, 5min",
         js("self", [{"kind": "stat", "stat": "wis", "amount": 340}, {"kind": "stat_set", "stat": "maxpower", "amount": 1625}])),
        (701222, "Severupidus' Cocktail of Torment", "Self, Summon Severupidus' Cocktail x3",
         js("self", [{"kind": "create_item", "item": {"count": 3, "name": "Severupidus' Cocktail", "patternid": 923625}}])),
        (701139, "Fine Red Wine", "Self, Summon Fine Red Wine x3",
         js("self", [{"kind": "create_item", "item": {"count": 3, "name": "Fine Red Wine"}}])),
        (701259, "Yalgima's Fearsome Companion", "Summons a cat",
         js("self", [{"kind": "summon", "pet": {"sheet_pet_id": "0062", "name": "Fearsome Companion"}}])),
        (902253, "Kitteh Bite", "Target, Decrease Hitpoints by 450 ; Increase 25% based on STR",
         js("target", [{"kind": "hp", "amount": -450, "scale": "str", "scale_pct": 25}], harmful=True)),
        (700576, "Bulwark", "Target, Increase AC by 1000, Decrease AC to self by 500, 18sec", ""),
        (700595, "Elude", "Target, Decrease Hate by 555", js("target", [{"kind": "hate", "amount": -555}])),
    ]
    for sid, name, line, eff in spells:
        db.execute("INSERT INTO spellpattern (spellid, spellname, spelldesc) VALUES (?, ?, ?)", (sid, name, line))
        db.execute("INSERT INTO spelleffect (spellid, line, effects, source) VALUES (?, ?, ?, 'workbook')", (sid, line, eff))
    if clash:
        db.execute("INSERT INTO spellpattern (spellid, spellname) VALUES (905000, 'Ascended Bulwark')")
    db.execute("INSERT INTO spellarea VALUES (701118, 1, 30)")
    db.executemany("INSERT INTO classspells VALUES (?, ?, ?)", [(6, 701227, "eqoa-data epic"), (5, 701118, "eqoa-data epic"),
                                                                (6, 700595, "eqoa-data")])
    db.execute("INSERT INTO petstats (spellid, name, level, ac, str, hp, wpn1_dmg, abilities) VALUES "
               "(701259, 'Fearsome Companion', 54, 1620, 536, 4423, 648, ?)",
               (json.dumps([{"every": 12.0, "kind": "nuke", "spell_id": 902253}, {"every": 12.0, "kind": "taunt"}]),))
    q = [(101117, "The Rogue's Epic Reward", "EQOA-Data 60/good-melee/reward-rogue (tools/import_quests.py)"),
         (100975, "The Rogue's Reward, Level 49", "EQOA-Data 30,40,49/group-2-good/L49-rogue (tools/import_quests.py)"),
         (100900, "The Rogue's Reward, Level 40", "EQOA-Data 30,40,49/group-2-good/L40-rogue (tools/import_quests.py)")]
    db.executemany("INSERT INTO quests VALUES (?, ?, ?)", q)

    def opts(*o):
        return json.dumps({"ask": "Which?", "options": [dict(answer=a, says="Yes.", **kw) for a, kw in o]})
    db.executemany("INSERT INTO queststeps (questid, step, rewarditems, choices) VALUES (?, ?, ?, ?)", [
        (101117, 1, "", opts(("Sothimus Critical wounding", {"spells": "701227"}), ("Celedonicus' Grand Finale", {"spells": "701118"}),
                             ("Talimus's Diamond Barrier", {"spells": "701240"}), ("Graal's Final Strike", {"spells": "701150"}),
                             ("Gelledyn's Perfect Vision", {"spells": "701145"}),
                             ("Severupidus' Cocktail of Torment", {"spells": "701222"}), ("Fine Red Wine", {"spells": "701139"}),
                             ("Yalgima's Fearsome Companion", {"spells": "701259"}), ("Bulwark", {"spells": "700576"}))),
        (101117, 2, "1342, 709300", opts(("The Rogue's Epic Daggers", {"items": "718029*2"}),
                                         ("The Tunic of Fierce Flames", {"items": "1342"}))),
        (100975, 1, "", opts(("Elude and the Blacktongue", {"items": "700995", "spells": "700595"}),
                             ("The Heartsbane", {"items": "5996"}))),
        (100900, 1, "5996", opts(("Elude", {"spells": "700595"}))),
    ])
    db.execute("INSERT INTO characters VALUES (1, 'Holder')")
    db.commit()
    db.close()


def dump(path, skip=("gm_audit",)):
    db = sqlite3.connect(path)
    out = {}
    for (t, sql) in db.execute("SELECT name, sql FROM sqlite_master WHERE type = 'table' ORDER BY name"):
        if t in skip:
            continue
        rows = db.execute(f'SELECT * FROM "{t}"').fetchall()
        if t == "sqlite_sequence":
            rows = [r for r in rows if r[0] not in skip]
        out[t] = (sql, sorted(map(repr, rows)))
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, "-E", "-s", TOOL, "--db", path, "--no-backup", *args],
                          capture_output=True, text=True)


def walk(a, b, path, bad):
    """Every number of a against b's: a tenth more, or kept where the petition leaves it."""
    if isinstance(b, dict):
        for k in b:
            if k == "patternid" and path[-1:] == ["item"]:
                continue  # the made item's id: its Ascended copy's
            walk(a.get(k), b[k], path + [str(k)], bad)
    elif isinstance(b, list):
        for i, (x, y) in enumerate(zip(a, b)):
            walk(x, y, path + [str(i)], bad)
    elif isinstance(b, (int, float)) and not isinstance(b, bool):
        if a != b and a != ae.up(b):
            bad.append(f"{'.'.join(path)}: {a} against {b}")


class Tool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        make(self.path)

    def tearDown(self):
        self.dir.cleanup()

    def q(self, sql, *args):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def eff(self, base):
        r = self.q("SELECT e.effects, e.line, p.spellname, p.spelldesc FROM spellascended a JOIN spelleffect e ON "
                   "e.spellid = a.spellid JOIN spellpattern p ON p.spellid = a.spellid WHERE a.base = ?", base)
        self.assertTrue(r, f"no Ascended copy of {base}")
        return (json.loads(r[0][0]) if r[0][0] else None), r[0][1], r[0][2], r[0][3]

    def test_report_apply_again_undo(self):
        before = dump(self.path)
        level40 = self.q("SELECT rewarditems, choices FROM queststeps WHERE questid = 100900")[0]
        r = run(self.path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before, "the report wrote something")
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        # gear: the server's rule, in itemascended
        rows = dict(self.q("SELECT i.base, p.itemname FROM itemascended i JOIN itempattern p USING (patternid)"))
        self.assertEqual(rows, {718029: "Ascended Rogue's Epic Dagger", 1342: "Ascended Tunic of Fierce Flames",
                                5996: "Ascended Heartsbane", 700995: "Ascended Blacktongue"})
        t = self.q("SELECT str, sta, HPMAX, AC, FR, rent FROM itempattern WHERE itemname = 'Ascended Tunic of Fierce Flames'")[0]
        self.assertEqual(t, (33, 4, 220, 462, 50, 1))
        # the quests: the 60 epic's spells and gear, the 49's gear alone, the 40's nothing
        s1 = json.loads(self.q("SELECT choices FROM queststeps WHERE questid = 101117 AND step = 1")[0][0])["options"]
        self.assertEqual(s1[0]["answer"], "Ascended Sothimus Critical wounding")
        base_of = dict(self.q("SELECT spellid, base FROM spellascended"))
        self.assertEqual(base_of[int(s1[0]["spells"])], 701227)
        s2 = self.q("SELECT rewarditems, choices FROM queststeps WHERE questid = 101117 AND step = 2")[0]
        tunic = self.q("SELECT patternid FROM itemascended WHERE base = 1342")[0][0]
        dagger = self.q("SELECT patternid FROM itemascended WHERE base = 718029")[0][0]
        self.assertEqual(s2[0], f"{tunic}, 709300")
        o2 = json.loads(s2[1])["options"]
        self.assertEqual((o2[0]["answer"], o2[0]["items"]), ("The Ascended Rogue's Epic Daggers", f"{dagger}*2"))
        o49 = json.loads(self.q("SELECT choices FROM queststeps WHERE questid = 100975")[0][0])["options"]
        self.assertEqual(o49[0]["answer"], "Elude and the Ascended Blacktongue")
        self.assertEqual(o49[0]["spells"], "700595", "a level 49 path spell was made Ascended")
        self.assertEqual(self.q("SELECT rewarditems, choices FROM queststeps WHERE questid = 100900")[0], level40,
                         "a level 40 path reward changed")
        self.assertNotIn(700595, base_of.values())
        # the spells: a tenth more, but what the petition leaves
        for base, sid in [(b, s) for s, b in base_of.items()]:
            new, line, name, desc = self.eff(base)
            old = self.q("SELECT effects FROM spelleffect WHERE spellid = ?", base)[0][0]
            if new is not None:
                bad = []
                walk(new, json.loads(old), [name], bad)
                self.assertEqual(bad, [], name)
            self.assertTrue(desc.startswith("Ascended (+10%): ") and len(desc) <= 127, desc)
        e, line, _, _ = self.eff(701227)
        self.assertEqual(e["effects"][0], {"kind": "speed", "amount": -30, "absolute": True})
        self.assertEqual((e["effects"][1]["amount"], e["effects"][1]["scale_pct"], e["effects"][1]["hate"]), (-1760, 33, 220))
        self.assertEqual(line, "Target, Root, Decrease Hitpoints by 1760 ; Increase 33% based on STR, 18sec")
        e, line, _, _ = self.eff(701118)
        self.assertEqual([x["amount"] for x in e["effects"]], [5500, -5000], "the bard's price changed")
        self.assertEqual(line, "Self, Increase Power by 5500, Decrease Power by 5000, PB AE")
        e, line, _, _ = self.eff(701240)
        self.assertEqual(e["effects"][0]["mitigation"], {"pct": 70, "max": -13750})
        e, line, _, _ = self.eff(701150)
        self.assertEqual((e["effects"][0]["procs"][0]["amount"], e["effects"][0]["procs"][0]["scale_pct"]), (-440, 11))
        self.assertEqual(line, "Self, Decrease Hitpoints by 440 ; Increase 11% based on STR, 5min")
        e, _, _, _ = self.eff(701145)
        self.assertEqual([x["amount"] for x in e["effects"]], [374, 1625])
        e, line, _, _ = self.eff(700576)  # no JSON: the line is the spell
        self.assertIsNone(e)
        self.assertEqual(line, "Target, Increase AC by 1100, Decrease AC to self by 500, 18sec")
        # what they make: Ascended copies, their use a tenth more
        e, line, _, _ = self.eff(701222)
        cocktail = self.q("SELECT p.patternid, e.line, e.area FROM itempattern p JOIN itemeffect e USING (patternid) "
                          "WHERE p.itemname = 'Ascended Severupidus'' Cocktail'")[0]
        self.assertEqual(e["effects"][0]["item"], {"count": 3, "name": "Ascended Severupidus' Cocktail", "patternid": cocktail[0]})
        self.assertEqual(cocktail[1:], ("Target, Decrease Hitpoints by 1485 to 1540", 10.0))
        self.assertEqual(line, "Self, Summon Ascended Severupidus' Cocktail x3")
        wine = self.q("SELECT p.patternid, p.PoT, f.duration_s FROM itempattern p JOIN foodeffect f USING (patternid) "
                      "WHERE p.itemname = 'Ascended Fine Red Wine'")[0]
        self.assertEqual(wine[1:], (220, 120))
        self.assertEqual(self.q("SELECT count(*) FROM itemascended WHERE patternid IN (?, ?)", cocktail[0], wine[0])[0][0], 0)
        # the pet: a tenth more, its ability's spell Ascended
        cat = self.q("SELECT spellid FROM spellascended WHERE base = 701259")[0][0]
        bite = self.q("SELECT spellid FROM spellascended WHERE base = 902253")[0][0]
        pet = self.q("SELECT name, level, ac, str, hp, wpn1_dmg, abilities FROM petstats WHERE spellid = ?", cat)[0]
        self.assertEqual(pet[:6], ("Fearsome Companion", 54, 1782, 590, 4865, 713))
        self.assertEqual(json.loads(pet[6])[0]["spell_id"], bite)
        # who it is for: the base's classes, never a class grant's
        self.assertEqual(sorted(self.q("SELECT class, source FROM classspells WHERE spellid IN "
                                       "(SELECT spellid FROM spellascended)")),
                         [(5, "eqoa-data epic"), (6, "eqoa-data epic")])
        self.assertEqual(self.q("SELECT count(*) FROM spellarea WHERE spellid = (SELECT spellid FROM spellascended WHERE base = 701118)")[0][0], 1)
        # a gm_audit line a row
        logged = self.q(f"SELECT count(*) FROM {ae.LOG} WHERE tbl NOT LIKE 'sqlite_%'")[0][0]
        self.assertEqual(self.q("SELECT count(*) FROM gm_audit")[0][0], logged)
        once = dump(self.path)
        r = run(self.path, "--apply")
        self.assertIn("nothing to do", r.stdout)
        self.assertEqual(dump(self.path), once, "a second apply wrote something")
        r = run(self.path, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.path), before, "--undo did not put every table back")

    def test_a_name_taken_refuses(self):
        os.remove(self.path)
        make(self.path, clash=True)
        before = dump(self.path)
        r = run(self.path, "--apply")
        self.assertEqual(r.returncode, 2, r.stdout + r.stderr)
        self.assertIn("named 'Ascended Bulwark' already", r.stdout)
        self.assertIn("nothing written", r.stdout)
        self.assertEqual(dump(self.path), before)

    def test_a_held_record_stays(self):
        run(self.path, "--apply")
        dagger = self.q("SELECT patternid FROM itemascended WHERE base = 718029")[0][0]
        db = sqlite3.connect(self.path)
        db.execute("INSERT INTO charinventory (serverid, patternid) VALUES (1, ?)", (dagger,))
        db.commit()
        db.close()
        r = run(self.path, "--undo")
        self.assertIn(f"itempattern {dagger} stays: charinventory 1", r.stdout)
        self.assertEqual(self.q("SELECT itemname FROM itempattern WHERE patternid = ?", dagger)[0][0], "Ascended Rogue's Epic Dagger")
        self.assertEqual(json.loads(self.q("SELECT choices FROM queststeps WHERE questid = 101117 AND step = 2")[0][0])
                         ["options"][0]["items"], "718029*2")


if __name__ == "__main__":
    unittest.main()
