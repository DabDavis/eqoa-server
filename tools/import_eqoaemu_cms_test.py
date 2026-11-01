#!/usr/bin/env python3
"""import_eqoaemu_cms.py on a database and a sheet of its own (no workbook
data here): a rank matched by name, by its stats, by the ability it adds and
by its line keeps our id and takes the sheet's values; a sheet rank of several
of our rows (a row a class), two sheet ranks of one of ours (merged); a racial
master class to Master Class; new ranks (id 900000 + hex, their prerequisites
our ids; a were rank behind its kind's quest, a werebeast's any of the five, a
were-hunter's the brand, all disabled with --weres off; a quest found, a
draft's rank offered behind it and listed); a blank keeps ours; a GM's row overwritten; charcms untouched; --apply's audit
lines, a stale plan, missing spells refused; --undo exact, a row changed since
and a new rank bought since left."""
import json
import os
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import import_eqoaemu_cms as m  # noqa: E402

STATS = ["AC", "STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA", "HP Max", "PWR Max", "HoT", "PoT", "AR", "CR", "LR",
         "FR", "PR", "DR", "STR Max", "STA Max", "AGI Max", "DEX Max", "WIS Max", "INT Max", "CHA Max", "HP Factor",
         "OFF Mod", "DEF Mod", "Speed"]
HEAD = (["CM ID", "Mastery", "Description", "Classification", "Sub-Category", "Race", "Archetype", "Class",
         "Lycanthropy", "Quest", "Level", "Points", "CM Spent Req"]
        + [f"CM Req {i}" for i in range(1, 5)] * 2
        + [f"Ability Req {i} ID" for i in range(1, 7)] + [f"Ability Req {i}" for i in range(1, 7)]
        + [f"Base {s} Req" for s in ("STR", "STA", "DEX", "AGI", "INT", "WIS", "CHA")]
        + [f"=+Spell {i} ID" for i in range(1, 7)] + [f"=+Spell {i}" for i in range(1, 7)]
        + [f"-Spell {i} ID" for i in range(1, 7)] + [f"-Spell {i}" for i in range(1, 7)] + STATS)
ALL_R = "HUM ELF DELF GNO DWF TRL BAR HLF ERU OGR"
ALL_C = "WAR RAN PAL SK MNK BRD RGE DRD SHA CL MAG NEC ENC WIZ ALC"


def wrow(cid, name, cls="General", sub="Stat Max", race=ALL_R, klass=ALL_C, level="10", points="5", spent="",
         desc="", reqs=(), abil=(), base=None, adds=(), removes=(), stats=None, lyc="", quest="", arch="All"):
    r = dict.fromkeys(HEAD, "")
    r.update({"CM ID": cid, "Mastery": name, "Description": desc, "Classification": cls, "Sub-Category": sub,
              "Race": race, "Archetype": arch, "Class": klass, "Lycanthropy": lyc, "Quest": quest, "Level": level,
              "Points": points, "CM Spent Req": spent})
    out = [r[h] for h in HEAD]
    for i, (x, n) in enumerate(reqs):
        out[13 + i], out[17 + i] = x, n
    for i, (x, n) in enumerate(abil):
        out[21 + i], out[27 + i] = x, n
    for k, v in (base or {}).items():
        out[HEAD.index(f"Base {k} Req")] = str(v)
    for i, (x, n) in enumerate(adds):
        out[40 + i], out[46 + i] = x, n
    for i, (x, n) in enumerate(removes):
        out[52 + i], out[58 + i] = x, n
    for k, v in (stats or {}).items():
        out[64 + STATS.index(k)] = str(v)
    return out


class ImportCMsTest(unittest.TestCase):
    def setUp(self):
        self.path = os.path.join(tempfile.mkdtemp(), "t.db")
        self.db = sqlite3.connect(self.path, isolation_level=None)
        self.addCleanup(self.db.close)
        x = self.db.execute
        self.db.executescript(m.AUDIT_SCHEMA)
        x("CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT)")
        x("INSERT INTO accountinfo VALUES (7, 'dab')")
        x("""CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0,
            classes INTEGER NOT NULL DEFAULT 32767, races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '',
            rank INTEGER NOT NULL DEFAULT 1, name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0,
            cost INTEGER NOT NULL DEFAULT 0, level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0,
            minstats TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '',
            effects TEXT NOT NULL DEFAULT '', adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '',
            removes TEXT NOT NULL DEFAULT '', remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
            enabled INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm',
            require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '')""")
        x("CREATE TABLE charcms (serverid INTEGER, cmid INTEGER, cost INTEGER, bought INTEGER, PRIMARY KEY (serverid, cmid))")
        x("CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER)")
        x("CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER)")
        x("CREATE TABLE queststeps (questid INTEGER, step INTEGER, spells TEXT NOT NULL DEFAULT '')")
        for sid, name, lvl in [(10, "Flash of Daggers", 6), (11, "Guard", 60), (12, "Dropkick", 54), (13, "Soul Tap", 55)]:
            x("INSERT INTO spellpattern VALUES (?, ?, ?)", (sid, name, lvl))
        x("INSERT INTO quests VALUES (500, 'Dropkick', 1)")
        x("INSERT INTO queststeps VALUES (500, 1, '12')")
        x("INSERT INTO quests VALUES (501, 'Soultap', 0)")
        # the were quests as tools/import_quests.py keys them: a kind's p3 gives its form
        x("CREATE TABLE quest_import_quests (key TEXT PRIMARY KEY, questid INTEGER NOT NULL)")
        for qid, key, name, on in [(601, "Weres/wolfwere/p3", "The Hunt", 1), (602, "Weres/bearwere/p3", "Mirin", 0),
                                   (603, "Weres/ratwere/p3", "Trissen", 1), (604, "Weres/lionwere/p3", "Jensen", 1),
                                   (605, "Weres/gatorwere/p3", "Julian", 0),
                                   (606, "Weres/werehunter/brand", "The Werehunter's Brand", 1)]:
            x("INSERT INTO quests VALUES (?, ?, ?)", (qid, name, on))
            x("INSERT INTO quest_import_quests VALUES (?, ?)", (key, qid))
        src = "eqoa-data cms"
        rows = [
            # id, cat, classes, races, mastery, rank, name, parent, cost, level, effects, adds, add_spells, desc
            (1, 0, 32767, 2047, "Brawn", 1, "Brawn 1", 0, 10, 10, "STR:1,STRMax:15", "", "", "Brawn wins"),
            (2, 0, 32767, 2047, "Brawn", 2, "Brawn 2", 1, 15, 20, "STR:2,STRMax:20", "", "", "Brawn wins"),
            (51, 0, 32767, 2047, "AGI", 1, "AGI 1", 0, 5, 20, "AGI:10", "", "", "AGI boost"),
            (231, 0, 32767, 2047, "Focus", 1, "Focus 1", 0, 5, 10, "PoT:4", "", "", "Focus"),
            (1021, 1, 32767, 64, "Northman's Gait", 1, "Northman's Gait", 0, 5, 6, "MovRate:5", "", "", "MOV RATE boost"),
            (1101, 1, 32767, 4, "Flash of Daggers", 1, "Flash of Daggers", 0, 10, 6, "", "Flash of Daggers@6", "10", "buff"),
            (1641, 1, 32767, 128, "Deputy", 1, "Deputy 1", 0, 200, 55, "HP:500", "", "", "Deputies"),
            (1642, 1, 32767, 128, "Deputy", 2, "Deputy 2", 1641, 75, 60, "", "Protect the Vale@60", "", "Deputies"),
            (3201, 4, 1, 2047, "Defender", 1, "Defender", 0, 200, 55, "AC:300", "", "", "Defenders"),
            (3202, 4, 1, 2047, "Defender", 2, "Guard", 3201, 75, 60, "", "Guard@60", "11", "Defenders"),
            (3221, 4, 1, 2047, "Soldier", 1, "Soldier", 0, 200, 55, "AC:100", "", "", ""),
            (3811, 4, 4, 2047, "Soldier", 1, "Soldier", 0, 200, 55, "AC:100", "", "", ""),
            (4111, 4, 8, 2047, "Death Knight", 1, "Death Knight", 0, 200, 55, "AC:550", "", "", ""),
            (4112, 4, 8, 2047, "Death Knight", 2, "Summon Servant", 4111, 75, 60, "AR:55", "", "", ""),
            (4101, 4, 8, 2047, "Slayer", 1, "Slayer", 0, 200, 55, "Off Mod:4", "", "", ""),
            (9001, 3, 1, 2047, "Old", 1, "Old Thing 1", 0, 9, 9, "STA:9", "", "", "only ours"),
        ]
        for r in rows:
            x("INSERT INTO cmcatalog (id, category, classes, races, mastery, rank, name, parent, cost, level, effects, "
              "adds, add_spells, description, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", r + (src,))
        x("UPDATE cmcatalog SET require_quests = '777' WHERE id = 51")
        # a GM edited Guard in the panel
        x("INSERT INTO gm_audit (at, accountid, username, via, action, tbl, rowkey, before, after) "
          "VALUES ('t', 3, 'jib', 'panel', 'update', 'cmcatalog', '{\"id\":3202}', '{}', '{}')")
        x("INSERT INTO charcms VALUES (42, 1, 10, 0)")   # Brawn 1, paid 10; the sheet's costs 12
        x("INSERT INTO charcms VALUES (42, 3202, 75, 0)")
        self.sheet = m.Sheet([HEAD] + [
            wrow("0001", "Brawn 1", desc="Brawn wins most battles.", points="12", level="10", base={"STR": 100},
                 stats={"STR": 1, "STR Max": 15}),
            wrow("0002", "Brawn 2", desc="NO DESCRIPTION FOUND", points="15", level="20", reqs=[("0001", "Brawn 1")],
                 stats={"STR": 2, "STR Max": 20}),
            wrow("0078", "Avoid Danger 1", sub="Stat Quest", level="20", points="5", quest="QUEST ID",
                 stats={"AGI": 10}),
            wrow("0037", "Focus 1", arch="CASTER MELEE", klass="RAN MNK BRD RGE MAG NEC ENC WIZ ALC", stats={"PoT": 4}),
            wrow("003C", "Focus 1", arch="TANK PRIEST", klass="WAR PAL SK DRD SHA CL", stats={"PoT": 4}),
            wrow("00B4", "Northman Gait", cls="Racial", sub="Barbarian", race="BAR", level="6", points="5",
                 stats={"Speed": 5}),
            wrow("00B8", "Flash of Daggers", cls="Racial", sub="Dark Elf", race="DELF", level="6", points="10",
                 adds=[("06FA", "Flash of Daggers")]),
            wrow("0333", "Deputy", cls="Master Class", sub="Deputy", race="HLF", level="55", points="200", spent="500",
                 stats={"HP Max": 500}),
            wrow("0335", "Protect the Vale", cls="Master Class", sub="Deputy", race="HLF", level="60", points="75",
                 reqs=[("0333", "Deputy")], adds=[("0736", "Protect the Vale")]),
            wrow("03B0", "Defender", cls="Master Class", sub="Defender", klass="WAR", level="55", points="200",
                 spent="500", stats={"AC": 300}),
            wrow("03B1", "Defender's Guard", cls="Master Class", sub="Defender", klass="WAR", level="60", points="75",
                 reqs=[("03B0", "Defender")], adds=[("063B", "Guard")], desc="Defenders are tough to kill."),
            wrow("03C7", "Soldier", cls="Master Class", sub="Soldier", klass="WAR PAL SK", level="55", points="200",
                 spent="500", stats={"AC": 100}, desc="Soldiers."),
            wrow("03A3", "Death Knight", cls="Master Class", sub="Death Knight", klass="SK", level="55", points="200",
                 spent="500", stats={"AC": 300}),
            wrow("03A6", "Slayer", cls="Master Class", sub="Slayer", klass="SK", level="55", points="200",
                 spent="500", stats={"OFF Mod": 4}),
            wrow("03A5", "Undead Servitude", cls="Master Class", sub="Death Knight", klass="SK", level="60",
                 points="75", reqs=[("03A6", "Slayer")], adds=[("05EB", "Undead Servitude")], stats={"AR": 50}),
            wrow("009A", "Heightened Awareness 1", sub="Were Hunter", lyc="Were Hunter", quest="QUEST ID",
                 stats={"STA Max": 10}),
            wrow("00A1", "Bite", sub="Were", lyc="Werebeast", quest="QUEST ID", adds=[("070D", "Bite")]),
            wrow("00A7", "Feral Bear Form 1", sub="Were", lyc="Werebear", quest="QUEST ID"),
            wrow("0356", "Heroic Were-Hunter", cls="Master Class", sub="Heroic Were-Hunter", klass="WAR PAL SK",
                 level="55", points="200", spent="500", reqs=[("009A", "Heightened Awareness 1")], stats={"AC": 200}),
            wrow("02E5", "Ankle Biter", cls="Class", sub="Warrior", klass="WAR", level="55", points="1",
                 quest="QUEST ID", adds=[("0630", "Dropkick")]),
            wrow("02BF", "Soul Tap", cls="Class", sub="Shadowknight", klass="SK", level="55", points="1",
                 quest="QUEST ID", adds=[("05D9", "Soul Tap")]),
            wrow("0003", "Brawn 3", points="20", level="30", reqs=[("0002", "Brawn 2")], stats={"STR": 3}),
        ])
        self.spellmap = {0x06FA: (10, "name"), 0x0736: (900000 + 0x736, "new"), 0x063B: (11, "name"),
                         0x05EB: (900000 + 0x5EB, "new"), 0x0630: (12, "name"), 0x05D9: (13, "name")}

    def plan(self, **opts):
        return m.plan(self.db, m.read_ranks(self.sheet), self.spellmap, {0x0736: 60, 0x05EB: 60}, opts)

    def row(self, cid):
        return m.row_of(self.db, cid)

    def test_matches_and_values(self):
        p = self.plan()
        hows = {oid: how for oid, (_, how) in p["taken"].items()}
        self.assertEqual(hows[1], "name")
        self.assertEqual(hows[51], "its stats, level and cost", "AGI 1 is Avoid Danger 1")
        self.assertEqual(hows[1021], "name", "Northman's Gait is Northman Gait")
        self.assertEqual(hows[1642], "the abilities it adds or needs")
        self.assertEqual(hows[3202], "the abilities it adds or needs")
        self.assertEqual(p["cover"][0x03C7], [3221, 3811], "one sheet rank, a row of ours a class")
        self.assertNotIn(9001, p["taken"])
        ch = {c["id"]: c for c in p["changes"]}
        self.assertEqual(ch[1]["after"]["cost"], 12)
        self.assertEqual(ch[1]["after"]["minstats"], "STR:100")
        self.assertEqual(ch[1]["after"]["description"], "Brawn wins most battles.")
        self.assertNotIn("description", ch.get(2, {"after": {}})["after"], "NO DESCRIPTION FOUND keeps ours")
        self.assertNotIn("effects", ch[1]["after"], "the same stats are left as written")
        self.assertEqual(ch[51]["after"]["name"], "Avoid Danger 1")
        self.assertNotIn("require_quests", ch[51]["after"], "ours names the quest: kept")
        self.assertEqual(ch[1641]["after"]["category"], 4, "a racial master class is Master Class")
        self.assertEqual(ch[1642]["after"]["name"], "Protect the Vale")
        self.assertEqual(ch[1642]["after"]["add_spells"], str(900000 + 0x736))
        self.assertNotIn("adds", ch[1642]["after"], "the same text is left as written")
        self.assertEqual(ch[3202]["after"]["name"], "Defender's Guard")
        self.assertTrue(ch[3202]["gm"], "the GM's row is the sheet's too")
        self.assertNotIn("classes", ch[3811]["after"], "each of our Soldier rows keeps its class")
        self.assertEqual(ch[4112]["after"]["name"], "Undead Servitude")
        self.assertNotIn("require_ranks", ch[4112]["after"], "another master class's rank is the sheet's slip")
        self.assertEqual(p["W"][0x0037]["classes"] | p["W"][0x003C]["classes"], 32767)
        self.assertNotIn(0x003C + 900000, {e["id"] for e in p["inserts"]}, "the second Focus line is ours, merged")
        self.assertNotIn("effects", ch.get(1021, {"after": {}})["after"], "Speed is our MovRate")
        self.assertEqual(p["cost_changed"], [(1, 42, 10, 12)])
        # new ranks
        ins = {e["id"]: e for e in p["inserts"]}
        self.assertEqual(set(ins), {900000 + 0x009A, 900000 + 0x02E5, 900000 + 0x02BF, 900000 + 0x0003,
                                    900000 + 0x03C7, 900000 + 0x00A1, 900000 + 0x00A7, 900000 + 0x0356})
        sk = ins[900000 + 0x03C7]
        self.assertEqual((sk["row"]["classes"], sk["how"]), (8, "classes no rank of ours is for"),
                         "the sheet's Soldier is the shadow knight's too, which has no row of ours")
        b3 = ins[900003]["row"]
        self.assertEqual((b3["parent"], b3["mastery"], b3["rank"], b3["enabled"]), (2, "Brawn", 3, 1))
        gate = {i: (ins[i]["row"]["require_quests"], ins[i]["row"]["enabled"]) for i in ins}
        self.assertEqual(gate[900000 + 0x009A], ("606", 1), "a were-hunter's rank needs the brand")
        self.assertEqual(gate[900000 + 0x0356], ("606", 1), "a Were-Hunter master class too")
        self.assertEqual(gate[900000 + 0x00A7], ("602", 1), "a werebear's needs the bear's form quest, a draft")
        self.assertEqual(gate[900000 + 0x00A1], ("601|602|603|604|605", 1), "a werebeast's any were's")
        self.assertEqual(p["were_gates"]["werebeast (any were)"], "601|602|603|604|605")
        self.assertIn(f"{900000 + 0x00A7} Feral Bear Form 1: 602 Mirin",
                      p["notes"]["new ranks gated on a draft quest (it bars no one until it is in the game)"])
        self.assertEqual(m.parse_groups("601|602, 7"), [[601, 602], [7]])
        off = {e["id"]: e for e in self.plan(weres="off")["inserts"]}
        self.assertEqual(off[900000 + 0x00A1]["row"]["enabled"], 0, "--weres off: written disabled")
        self.assertEqual(ins[900000 + 0x02E5]["row"]["require_quests"], "500", "the quest rewarding Dropkick")
        self.assertEqual(ins[900000 + 0x02E5]["row"]["enabled"], 1)
        self.assertEqual(ins[900000 + 0x02BF]["row"]["require_quests"], "501")
        self.assertEqual(ins[900000 + 0x02BF]["row"]["enabled"], 1, "a draft's rank is offered, gated on it")
        self.assertEqual(self.plan(racial_mc="keep")["changes"][0]["id"], 1)
        keep = {c["id"]: c for c in self.plan(racial_mc="keep")["changes"]}
        self.assertNotIn("category", keep[1641]["after"])

    def test_apply_and_undo(self):
        before = {r[0]: self.row(r[0]) for r in self.db.execute("SELECT id FROM cmcatalog").fetchall()}
        charcms = self.db.execute("SELECT * FROM charcms ORDER BY cmid").fetchall()
        p = json.loads(json.dumps(m.plan_json(self.plan())))  # as a saved plan
        with self.assertRaises(SystemExit):
            m.apply(self.db, p)  # Protect the Vale's and Undead Servitude's spells are not made yet
        done, stale = m.apply(self.db, p, allow_missing=True)
        self.assertEqual(stale, 0)
        self.assertEqual(done, len(p["changes"]) + len(p["inserts"]) + len(p["disables"]))
        self.assertEqual(self.row(1642)["name"], "Protect the Vale")
        self.assertEqual(self.row(1642)["source"], "eqoa-data cms; " + m.SOURCE)
        self.assertEqual(self.row(900003)["source"], m.SOURCE)
        self.assertEqual(self.db.execute("SELECT * FROM charcms ORDER BY cmid").fetchall(), charcms)
        a = self.db.execute("SELECT username, via, action, before, after FROM gm_audit WHERE rowkey = '{\"id\":1642}'").fetchone()
        self.assertEqual(a[:3], ("dab", "tool", "update"))
        self.assertEqual(json.loads(a[3])["name"], "Deputy 2")
        self.assertEqual(json.loads(a[4])["name"], "Protect the Vale")
        self.assertEqual(self.db.execute("SELECT action, before FROM gm_audit WHERE rowkey = '{\"id\":900003}'").fetchone(),
                         ("insert", None))
        self.assertEqual(m.apply(self.db, p, allow_missing=True), (0, done), "the same plan again writes nothing")
        # a GM changes one row after; a character buys a new rank
        self.db.execute("UPDATE cmcatalog SET cost = 99 WHERE id = 1")
        self.db.execute("INSERT INTO charcms VALUES (42, 900003, 20, 0)")
        back, left, why = m.undo(self.db)
        self.assertEqual(left, 2, why)
        self.assertEqual(back, done - 2)
        for cid, r in before.items():
            if cid != 1:
                self.assertEqual(self.row(cid), r, f"{cid} not put back")
        self.assertEqual(self.row(1)["cost"], 99, "the GM's later change was undone")
        self.assertIsNotNone(self.row(900003), "a rank bought since was taken out")
        self.assertIsNone(self.row(900000 + 0x02E5), "a new rank was not taken out")


if __name__ == "__main__":
    unittest.main()
