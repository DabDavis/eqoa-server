#!/usr/bin/env python3
"""tools/fix_npc_name_case.py on a made-up database (no game data but the
names the user and the coordinator gave as examples): a generic name takes
the lowercase spelling another source has ("Moss Snake" is "moss snake", "Tae
Ew Crusader" is "Tae Ew crusader", "Copperhead " is "copperhead"), one
capitalized everywhere is read by its words ("Albino Rat" is "albino rat"),
an individual's keeps its case ("Guard Rick", "Captain Scilond", "Sarek the
Betrayer", "child of Xrag" as written); a retail savestate's spelling wins
("Sign Post" is "sign post") and Sandstorm's capitals do not; pets, towns'
people, the names the server's code keys and the capitals it reads are kept.
--apply renames every column that holds a name, merges two rows that become
one, keeps every join the server makes, and --undo gives back every table as
it was. The control: an apply that leaves factionhits out loses its join.

    python3 tools/fix_npc_name_case_test.py
    EQOA_REAL_DB=<a copy of a server's eqoa.db> python3 tools/fix_npc_name_case_test.py

With EQOA_REAL_DB the round trip runs on that database too (a copy of it).
"""
import json
import os
import re
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "fix_npc_name_case.py")
GO = os.path.join(HERE, "..", "internal")
sys.path.insert(0, HERE)
import fix_npc_name_case as M  # noqa: E402

HUMAN = 0x70D898C6  # a players' race body, as the wire carries it
WOLF = 0x12345678

SCHEMA = """
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, npc_type INTEGER, modelid INTEGER, world INTEGER);
CREATE TABLE prh_import_npcs (src_id INTEGER PRIMARY KEY, npc_name TEXT, status TEXT NOT NULL, dst_id INTEGER);
CREATE TABLE charactermodel (race INTEGER, sex INTEGER, modelid INTEGER);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, spelldesc TEXT);
CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL, faction TEXT NOT NULL,
    delta INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm', UNIQUE (npc_name, faction));
CREATE TABLE npcsocial (npc_name TEXT PRIMARY KEY, social INTEGER NOT NULL DEFAULT 1, faction TEXT NOT NULL DEFAULT '',
    radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1, radius REAL NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcclass (name TEXT PRIMARY KEY, class INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE npcloot (npc_name TEXT NOT NULL, tablename TEXT NOT NULL, chance INTEGER NOT NULL DEFAULT 25,
    rolls INTEGER NOT NULL DEFAULT 1, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (npc_name, tablename));
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', UNIQUE (name, world, x, z, radius));
CREATE TABLE npcfaction_seed (rowid INTEGER NOT NULL, name TEXT NOT NULL, faction TEXT NOT NULL);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', giver TEXT NOT NULL DEFAULT '',
    events TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
    target TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE questunlocks (questid INTEGER NOT NULL, npc TEXT NOT NULL, dest TEXT NOT NULL, PRIMARY KEY (questid, npc, dest));
CREATE TABLE charbind (serverid INTEGER PRIMARY KEY, spiritmaster TEXT NOT NULL DEFAULT '');
CREATE TABLE quest_hit_seed (rowid INTEGER NOT NULL, npc_name TEXT NOT NULL, faction TEXT NOT NULL, delta INTEGER NOT NULL);
CREATE TABLE npcpaths_seed (npcid INTEGER PRIMARY KEY, before TEXT, after TEXT NOT NULL, source TEXT NOT NULL, made TEXT);
"""

# (name as our rows spell it, rows, npc_type, body)
OURS = [
    ("Moss Snake", 9, 0, WOLF), ("moss snake", 16, 0, WOLF),
    ("Tae Ew Crusader", 59, 0, WOLF), ("Tae Ew crusader", 78, 0, WOLF),
    ("Copperhead ", 1, 0, WOLF), (" Majorkahn giant", 1, 0, WOLF),
    ("child of Xrag", 4, 0, WOLF), ("Deathfist reaver", 5, 0, HUMAN),
    ("Guard Rick", 1, 128, HUMAN), ("Guard Jahn", 1, 128, HUMAN), ("Guard Telan", 1, 128, HUMAN),
    ("Guard  Bajie", 1, 128, HUMAN),
    ("Captain Scilond", 1, 128, HUMAN), ("Captain Norgam", 1, 128, HUMAN),
    ("Sarek the Betrayer", 1, 0, HUMAN), ("Albino Rat", 2, 0, WOLF), ("giant rat", 3, 0, WOLF),
    ("Sign Post", 1, 128, WOLF), ("Fire Elemental", 2, 0, WOLF), ("Bloody Portal", 1, 128, WOLF),
    ("Old Villager", 1, 128, HUMAN), ("Frostbite", 1, 0, WOLF),
    ("Grass Viper", 3, 0, WOLF), ("grass viper", 5, 0, WOLF),
    ("Breath Stealer", 1, 0, WOLF), ("breath stealer", 2, 0, WOLF),
    ("Guard Mason", 1, 128, HUMAN), ("guard mason", 1, 0, HUMAN),
]
# Return Home's names (lowercase where SOE wrote them so)
RETURNHOME = ["moss snake", "Tae Ew crusader", "copperhead", "Majorkahn giant", "child of Xrag", "Deathfist reaver",
              "grass viper", "giant rat", "breath stealer", "Deathfist pawn", "kappa drudge", "Tae Ew devastator"]
WORDS = ["rick", "snake", "rat", "albino", "guard", "captain", "child", "crusader", "giant", "copperhead", "frostbite",
         "betrayer", "villager", "old", "sign", "post", "viper", "grass", "breath", "stealer", "mason", "fire", "elemental",
         "bloody", "portal", "moss", "pawn", "drudge", "reaver"]


def make_db(path):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    for name, n, t, body in OURS:
        for _ in range(n):
            db.execute("INSERT INTO npcs (npc_name, npc_type, modelid, world) VALUES (?, ?, ?, 0)", (name, t, body))
    for i, n in enumerate(RETURNHOME):
        db.execute("INSERT INTO prh_import_npcs VALUES (?, ?, 'ours', NULL)", (i + 1, n))
    db.execute("INSERT INTO charactermodel VALUES (0, 0, ?)", (HUMAN,))
    db.execute("INSERT INTO spellpattern VALUES (1, 'Fire Elemental', 'Summons a fire elemental to fight for you.')")
    # factionhits on two spellings: one row each, and the pair on one faction
    db.executemany("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)", [
        ("Moss Snake", "Snakes", -5, "gm"), ("Tae Ew Crusader", "Tae Ew", -10, "quest-seed"),
        ("Breath Stealer", "Citizens of Graumord", 5, "quest-seed"), ("breath stealer", "Citizens of Graumord", 5, "quest-seed"),
        ("Guard  Bajie", "The Freeport Militia", -50, "town-seed")])
    db.executemany("INSERT INTO quest_hit_seed VALUES (?, ?, ?, ?)", [
        (2, "Tae Ew Crusader", "Tae Ew", -10), (3, "Breath Stealer", "Citizens of Graumord", 5),
        (4, "breath stealer", "Citizens of Graumord", 5)])
    db.executemany("INSERT INTO npcsocial (npc_name, social, faction) VALUES (?, 1, ?)", [
        ("Tae Ew Crusader", "Tae Ew"), ("Guard  Bajie", "The Freeport Militia")])
    db.executemany("INSERT INTO npcaggro (npc_name, aggressive, note, source) VALUES (?, ?, ?, ?)", [
        ("Moss Snake", 1, "", "gm"), ("Fire Elemental", 0, "a summoned pet's name (magician: Fire Elemental)", "seed")])
    db.executemany("INSERT INTO npcclass VALUES (?, ?, 'return home')", [
        ("moss snake", 0), ("tae ew crusader", 9), ("guard  bajie", 0), ("guard rick", 0)])
    db.execute("INSERT INTO npcloot VALUES ('moss snake', 'snakes', 25, 1, '', 'camp-seed')")
    db.execute("INSERT INTO npcfaction (name, faction, source) VALUES ('guard  bajie', 'The Freeport Militia', 'seed')")
    db.execute("INSERT INTO npcfaction_seed VALUES (1, 'guard  bajie', 'The Freeport Militia')")
    db.execute("INSERT INTO quests VALUES (1, 'Snakes', 'Tae Ew Crusader', '')")
    db.execute("INSERT INTO queststeps VALUES (1, 1, 'kill', 'Moss Snake', ?)",
               (json.dumps([{"on": "start", "do": "spawn", "npc": "Moss Snake", "hostile": True}], separators=(",", ":")),))
    db.execute("INSERT INTO queststeps VALUES (1, 2, 'talk', 'Guard Rick', '')")
    db.execute("INSERT INTO questunlocks VALUES (1, 'Copperhead ', 'Freeport')")
    db.execute("INSERT INTO charbind VALUES (7, 'Guard  Bajie')")
    db.execute("INSERT INTO charbind VALUES (8, 'Freeport')")
    made = db.execute("SELECT id FROM npcs WHERE npc_name = 'Albino Rat' LIMIT 1").fetchone()[0]
    row = dict(zip([c[1] for c in db.execute("PRAGMA table_info(npcs)")][1:],
                   db.execute("SELECT * FROM npcs WHERE id = ?", (made,)).fetchone()[1:]))
    db.execute("INSERT INTO npcpaths_seed VALUES (?, NULL, '{}', 'track', ?)", (made, json.dumps({"npc": row})))
    db.commit()
    return db


def dump(db):
    return "\n".join(db.iterdump())


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


class Classification(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        self.db = make_db(self.path)
        self.words = os.path.join(self.dir.name, "words")
        with open(self.words, "w") as f:
            f.write("\n".join(WORDS + ["Rick", "Snake"]) + "\n")
        # Sandstorm's recording: capitalized throughout, as its 2,094 of 2,103 names are
        self.harvest = os.path.join(self.dir.name, "harvest.json")
        with open(self.harvest, "w") as f:
            json.dump({"server": "eqoa.live", "seen": [{"id": 1001, "name": "Grass Viper"}, {"id": 1002, "name": "Moss Snake"},
                                                       {"id": 500000000, "name": "Aplayer"}]}, f)
        self.ev = M.load_evidence(self.db, [self.harvest], self.words)
        self.plan = M.plan(self.db, self.ev)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def new(self, name):
        return self.plan.renames.get(name, name)

    def test_generic_names_take_the_lowercase_spelling(self):
        self.assertEqual(self.new("Moss Snake"), "moss snake")
        self.assertEqual(self.new("Tae Ew Crusader"), "Tae Ew crusader")
        self.assertEqual(self.new("Copperhead "), "copperhead")
        self.assertEqual(self.new(" Majorkahn giant"), "Majorkahn giant")
        self.assertEqual(self.new("Grass Viper"), "grass viper", "Sandstorm's capitals are no evidence")
        self.assertEqual(self.new("moss snake"), "moss snake")

    def test_a_name_capitalized_everywhere_is_read_by_its_words(self):
        self.assertEqual(self.new("Albino Rat"), "albino rat")
        self.assertEqual(self.plan.decisions["albino rat"].kind, "generic")

    def test_individuals_keep_their_case(self):
        for name in ("child of Xrag", "Guard Rick", "Captain Scilond", "Sarek the Betrayer", "Deathfist reaver"):
            self.assertNotIn(name, self.plan.renames, name)
        self.assertEqual(self.new("Guard  Bajie"), "Guard Bajie", "a doubled space goes, the case stays")

    def test_the_rule_on_its_own(self):
        self.assertEqual(self.ev.rule("Guard Rick")[0], "individual", "a title before a dictionary word")
        self.assertEqual(self.ev.rule("Sarek the Betrayer")[0], "individual")
        self.assertEqual(self.ev.rule("The Storyteller")[0], "individual")
        self.assertEqual(self.ev.rule("Albino Rat")[0], "generic")
        self.assertEqual(self.ev.rule("Frostbite")[0], "ambiguous", "a head no name has lowercase")
        self.assertEqual(self.ev.rule("Frostbite", rows=3)[0], "generic", "spawned three times: a kind")
        self.assertEqual(self.ev.rule("Training Dummy Lv50")[0], "individual")
        self.assertTrue(self.ev.proper("Xrag") and self.ev.proper("Tae") and not self.ev.proper("crusader"))

    def test_retail_pets_towns_places_and_code(self):
        self.assertEqual(self.new("Sign Post"), "sign post", "the retail memory's spelling")
        self.assertEqual(self.plan.decisions["fire elemental"].kind, "pet")
        self.assertEqual(self.plan.decisions["old villager"].kind, "town")
        self.assertEqual(self.plan.decisions["bloody portal"].kind, "code")
        self.assertEqual(self.plan.decisions["frostbite"].kind, "ambiguous")
        for name in ("Fire Elemental", "Old Villager", "Bloody Portal", "Frostbite"):
            self.assertNotIn(name, self.plan.renames, name)
        # "guard mason" elsewhere would take "Guard Mason"'s guard title away (entity/dialog.go)
        self.assertEqual(self.plan.decisions["guard mason"].kind, "code")
        self.assertNotIn("Guard Mason", self.plan.renames)

    def test_two_lowercase_bearing_spellings_go_by_their_words(self):
        ev = M.Evidence({"a": {"Deathfist pawn": 2, "deathfist pawn": 4, "Deathfist reaver": 30, "Deathfist slayer": 9}})
        self.assertEqual(ev.pick({"Deathfist pawn": 2, "deathfist pawn": 4}), "Deathfist pawn")

    def test_sandstorm_names_leave_out_players(self):
        names = M.harvest_names([self.harvest])
        self.assertIn("Grass Viper", names)
        self.assertNotIn("Aplayer", names)

    def test_the_names_the_code_keys_are_kept(self):
        """Every name handlers/waysender.go's routes, entity/world.go's warp
        labels and handlers/chat.go's #tele places key as written is in
        CODE_KEYED or keeps its capitals by ROLE_PREFIXES."""
        keyed = set()
        src = read(os.path.join(GO, "handlers", "waysender.go"))
        block = src[src.index("var wayRoutes"):src.index("\n}\n", src.index("var wayRoutes"))]
        keyed |= set(re.findall(r'^\t"([^"]+)":', block, flags=re.M))
        src = read(os.path.join(GO, "entity", "world.go"))
        block = src[src.index("var warpLocationNames"):src.index("\n}\n", src.index("var warpLocationNames"))]
        keyed |= set(re.findall(r'^\t"([^"]+)":', block, flags=re.M))
        src = read(os.path.join(GO, "handlers", "chat.go"))
        block = src[src.index("var teleDests"):src.index("\n}\n", src.index("var teleDests"))]
        keyed |= set(re.findall(r'NPC: "([^"]+)"', block))
        src = read(os.path.join(GO, "entity", "coachtowns.go"))
        block = src[src.index("var coachLocationNames"):src.index("\n}\n", src.index("var coachLocationNames"))]
        keyed |= set(re.findall(r'^\t"([^"]+)":', block, flags=re.M))
        self.assertGreater(len(keyed), 40)
        missing = sorted(n for n in keyed if n not in M.CODE_KEYED and not n.startswith(M.ROLE_PREFIXES))
        self.assertEqual(missing, [], "names the code keys as written, neither in CODE_KEYED nor of a role prefix")


class ApplyAndUndo(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        self.db = make_db(self.path)
        self.words = os.path.join(self.dir.name, "words")
        with open(self.words, "w") as f:
            f.write("\n".join(WORDS) + "\n")
        self.before_values = value_joins(self.db)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def plan(self):
        return M.plan(self.db, M.load_evidence(self.db, [], self.words))

    def test_apply_renames_every_reference_and_undo_is_exact(self):
        before = dump(self.db)
        joins = M.name_joins(self.db)
        done, problems = M.apply(self.db, self.plan())
        self.assertEqual(problems, [])
        q = lambda sql, *a: self.db.execute(sql, a).fetchall()  # noqa: E731
        self.assertEqual(q("SELECT count(*) FROM npcs WHERE npc_name = 'moss snake'"), [(25,)])
        self.assertEqual(q("SELECT count(*) FROM npcs WHERE npc_name = 'Tae Ew crusader'"), [(137,)])
        self.assertEqual(q("SELECT npc_name FROM factionhits ORDER BY id"),
                         [("moss snake",), ("Tae Ew crusader",), ("breath stealer",), ("Guard Bajie",)],
                         "renamed, and the two spellings' rows on one faction merged")
        self.assertEqual(q("SELECT npc_name FROM npcsocial ORDER BY npc_name"), [("Guard Bajie",), ("Tae Ew crusader",)])
        self.assertEqual(q("SELECT giver FROM quests"), [("Tae Ew crusader",)])
        self.assertEqual(q("SELECT target FROM queststeps ORDER BY step"), [("moss snake",), ("Guard Rick",)])
        self.assertEqual(json.loads(q("SELECT events FROM queststeps WHERE step = 1")[0][0])[0]["npc"], "moss snake")
        self.assertEqual(q("SELECT npc FROM questunlocks"), [("copperhead",)])
        self.assertEqual(q("SELECT spiritmaster FROM charbind ORDER BY serverid"), [("Guard Bajie",), ("Freeport",)])
        self.assertEqual(q("SELECT name FROM npcclass WHERE name LIKE 'guard%bajie'"), [("guard bajie",)])
        self.assertEqual(q("SELECT name FROM npcfaction"), [("guard bajie",)])
        self.assertEqual(q("SELECT name FROM npcfaction_seed"), [("guard bajie",)], "seed_npc_factions' undo follows")
        self.assertEqual(q("SELECT npc_name FROM quest_hit_seed ORDER BY rowid"),
                         [("Tae Ew crusader",), ("Breath Stealer",), ("breath stealer",)],
                         "seed_quest_hits' undo follows its renamed row; the merged row's log is left")
        made = json.loads(q("SELECT made FROM npcpaths_seed")[0][0])["npc"]["npc_name"]
        self.assertEqual(made, "albino rat", "seed_npc_patrols' undo follows its made NPC")
        self.assertEqual(q("SELECT npc_name FROM npcaggro ORDER BY npc_name"), [("Fire Elemental",), ("Moss Snake",)],
                         "a folded table is left: the server keys it by NPCClassKey")
        # every join the server makes still reaches every NPC it reached
        after = M.name_joins(self.db)
        folded = {t for t, c, how in M.JOINS if how != "exact"}
        for k in joins:
            if k[0] in folded:
                self.assertEqual(len(after[k]), len(joins[k]), k)
                self.assertEqual(after[k], joins[k], k)
        self.assertEqual(self.before_values - value_joins(self.db), set(), "an NPC lost what an exact table gave it")
        self.assertGreater(len(value_joins(self.db) - self.before_values), 0, "the spellings' rows now reach every spelling")
        # a second apply is refused, and --undo gives back every table as it was
        with self.assertRaises(SystemExit):
            M.apply(self.db, self.plan())
        back, kept = M.undo(self.db)
        self.assertEqual(kept, [])
        self.assertEqual(dump(self.db), before)

    def test_control_an_apply_without_factionhits_loses_its_join(self):
        before = value_joins(self.db)
        M.apply(self.db, self.plan(), skip={"factionhits"})
        lost = before - value_joins(self.db)
        self.assertTrue(any(t == "factionhits" for t, *_ in lost), "the control's join did not drop")
        M.undo(self.db)

    def test_two_rows_that_disagree_are_a_problem_and_nothing_is_written(self):
        self.db.execute("UPDATE factionhits SET delta = 7 WHERE npc_name = 'breath stealer'")
        self.db.commit()
        before = dump(self.db)
        done, problems = M.apply(self.db, self.plan())
        self.assertEqual(len(problems), 1, problems)
        self.assertEqual(dump(self.db), before)

    def test_undo_leaves_a_value_changed_since(self):
        M.apply(self.db, self.plan())
        self.db.execute("UPDATE quests SET giver = 'Someone Else'")
        self.db.commit()
        back, kept = M.undo(self.db)
        self.assertEqual(len(kept), 1, kept)
        self.assertEqual(self.db.execute("SELECT giver FROM quests").fetchone(), ("Someone Else",))

    def test_the_change_list_applies_as_written(self):
        p = self.plan()
        out = os.path.join(self.dir.name, "out")
        changes, _, _ = M.write_lists(p, M.load_evidence(self.db, [], self.words), out)
        q = M.plan_from_tsv(self.db, changes)
        self.assertEqual(q.renames, p.renames)
        self.assertEqual(q.targets, p.targets)

    def test_the_command_reports_applies_and_undoes(self):
        before = dump(self.db)
        run = lambda *a: subprocess.run([sys.executable, "-I", TOOL, "--db", self.path, "--dict", self.words,  # noqa: E731
                                         "--no-backup", *a], capture_output=True, text=True, check=True).stdout
        out = run()
        self.assertIn("report only", out)
        self.assertEqual(dump(self.db), before)
        self.assertIn("restart the server", run("--apply"))
        self.assertNotEqual(dump(self.db), before)
        run("--undo")
        self.assertEqual(dump(self.db), before)


def value_joins(db):
    """What an NPC row gets from the tables the server keys by exact name:
    (table, npcs row, the row's values but its name, id and bookkeeping)."""
    out = set()
    for tbl, col, how in M.JOINS:
        if how != "exact" or not M.has_table(db, tbl):
            continue
        cur = db.execute(f"SELECT * FROM {tbl}")
        cols = [d[0] for d in cur.description]
        rows = cur.fetchall()
        names = {}
        for rid, n in db.execute("SELECT id, npc_name FROM npcs"):
            names.setdefault(n, []).append(rid)
        for r in rows:
            v = dict(zip(cols, r))
            rest = tuple(sorted((k, x) for k, x in v.items() if k not in (col, "id", "source", "note")))
            for npc in names.get(v[col], ()):
                out.add((tbl, npc, rest))
    return out


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB"), "EQOA_REAL_DB (a copy of a server's eqoa.db) not set")
class OnARealDatabase(unittest.TestCase):
    """The round trip on a real database's names: every join the server
    makes keeps what it reached, the quests import_quests.py logged still
    digest as logged, --undo is exact; the control loses factionhits' join."""

    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "eqoa.db")
        shutil.copyfile(os.environ["EQOA_REAL_DB"], self.path)  # not its mode: a snapshot may be read-only
        self.db = sqlite3.connect(self.path, isolation_level=None)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def test_apply_keeps_every_join_and_undo_is_exact(self):
        from import_quests import digest
        before = dump(self.db)
        joins, values = M.name_joins(self.db), value_joins(self.db)
        logged = {qid: dg for qid, dg in self.db.execute("SELECT questid, digest FROM quest_import_quests")} \
            if M.has_table(self.db, "quest_import_quests") else {}
        clean = {qid for qid, dg in logged.items() if digest(self.db, qid) == dg}
        p = M.plan(self.db, M.load_evidence(self.db))
        self.assertGreater(len(p.renames), 100)
        done, problems = M.apply(self.db, p)
        self.assertEqual(problems, [])
        after, after_values = M.name_joins(self.db), value_joins(self.db)
        for tbl, col, how in M.JOINS:
            if how != "exact" and (tbl, col) in joins:
                self.assertEqual(after[(tbl, col)], joins[(tbl, col)], tbl)
        self.assertEqual(values - after_values, set(), "an NPC lost what an exact table gave it")
        for qid in clean:
            dg = self.db.execute("SELECT digest FROM quest_import_quests WHERE questid = ?", (qid,)).fetchone()[0]
            self.assertEqual(digest(self.db, qid), dg, f"quest {qid} no longer digests as logged")
        back, kept = M.undo(self.db)
        self.assertEqual(kept, [])
        self.assertEqual(dump(self.db), before)
        # the control: factionhits left out, an NPC loses its hits
        M.apply(self.db, M.plan(self.db, M.load_evidence(self.db)), skip={"factionhits"})
        self.assertTrue(any(t == "factionhits" for t, *_ in values - value_joins(self.db)))
        M.undo(self.db)
        self.assertEqual(dump(self.db), before)


if __name__ == "__main__":
    unittest.main()
