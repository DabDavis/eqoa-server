#!/usr/bin/env python3
"""mermaid_pets_20261113.py on a small database: the report writes nothing; --apply writes the 25 rows (6 copies' three
rows each, a pettemplate row, 6 ranks), a gm_audit line each; a second apply does nothing; --undo puts every table back
exactly. The copies are the magician's records but their ids, levels, descriptions and sources (the Ascension's pet
level 51). The controls: an id taken, another row with the Mermaid's bit, racedefs 32 not the Mermaid, the kit's Siren
Royal missing, or a magician's record not what it should be, is refused, writing nothing. And the ranks are the ones
internal/entity/mermaidpets_test.go buys."""
import json
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import mermaid_pets_20261113 as mp  # noqa: E402

TOOL = os.path.join(HERE, "mermaid_pets_20261113.py")
GOTEST = os.path.join(HERE, "..", "internal", "entity", "mermaidpets_test.go")

SCHEMA = """
CREATE TABLE cmcatalog (id INTEGER PRIMARY KEY, category INTEGER NOT NULL DEFAULT 0, classes INTEGER NOT NULL DEFAULT 32767,
    races INTEGER NOT NULL DEFAULT 2047, mastery TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL DEFAULT '', parent INTEGER NOT NULL DEFAULT 0, cost INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL DEFAULT 1, spent INTEGER NOT NULL DEFAULT 0, minstats TEXT NOT NULL DEFAULT '',
    requires TEXT NOT NULL DEFAULT '', require_spells TEXT NOT NULL DEFAULT '', effects TEXT NOT NULL DEFAULT '',
    adds TEXT NOT NULL DEFAULT '', add_spells TEXT NOT NULL DEFAULT '', removes TEXT NOT NULL DEFAULT '',
    remove_spells TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'gm', require_ranks TEXT NOT NULL DEFAULT '', require_quests TEXT NOT NULL DEFAULT '');
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER NOT NULL, unk2 INTEGER NOT NULL,
    unk3 INTEGER NOT NULL, range INTEGER NOT NULL, casttime INTEGER NOT NULL, power INTEGER NOT NULL,
    iconColor INTEGER NOT NULL, icon INTEGER NOT NULL, scope INTEGER NOT NULL, recast INTEGER NOT NULL,
    eqprequire INTEGER NOT NULL, spellname TEXT NOT NULL, spelldesc TEXT NOT NULL);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL,
    effects TEXT NOT NULL DEFAULT '');
CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, sheet_pet_id TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
    model INTEGER NOT NULL DEFAULT 0, level INTEGER NOT NULL DEFAULT 0, type TEXT NOT NULL DEFAULT '',
    hp INTEGER NOT NULL DEFAULT 0, power INTEGER NOT NULL DEFAULT 0, ac INTEGER NOT NULL DEFAULT 0,
    abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm', size REAL NOT NULL DEFAULT 0);
CREATE TABLE pettemplate (spell TEXT NOT NULL, weapon INTEGER NOT NULL DEFAULT 0, template TEXT NOT NULL DEFAULT '',
    swing_class INTEGER NOT NULL DEFAULT -1, swing INTEGER NOT NULL DEFAULT -1, round_ms INTEGER NOT NULL DEFAULT 0,
    note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm', PRIMARY KEY (spell, weapon));
CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO cmcatalog (id, category, races, mastery, name, cost, level, spent) VALUES
    (2174, 4, 4294967296, 'Siren Royal', 'Siren Royal', 200, 55, 500), (2171, 1, 4294967296, 'Hydrate', 'Hydrate', 20, 6, 0),
    (2141, 1, 2147483648, 'Will-of-the-Wisp', 'Will-of-the-Wisp', 5, 6, 0), (1, 1, 2047, 'Everyone', 'Everyone', 1, 1, 0);
INSERT INTO pettemplate (spell, template) VALUES ('water elementaling', 'pet water'), ('summon ghilan', 'pet water');
"""

# the magician's six, as the live shard keeps them (but their abilities, shortened)
MAGE = [(640, 4, 16544, 4, 25, 10, "Water Elementaling", "0019", "Water Elementaling", 4, 89, 0.0, "a water elementaling"),
        (669, 20, 16544, 6, 390, 30, "Water Elementalkin", "001D", "Water Elementalkin", 19, 766, 0.0, "an elementalkin"),
        (900688, 29, 16544, 6, 726, 30, "Water Elementalkin (29)", "0025", "Water Elementalkin", 26, 1285, 0.0,
         "a Water Elementalkin"),
        (900699, 34, 16544, 8, 857, 45, "Water Elemental (34)", "0029", "Water Elemental", 31, 1707, 0.0, "a Water Elemental"),
        (700557, 47, 16544, 10, 1227, 60, "Summon Ghilan", "0033", "Ghilan", 43, 3002, 2.0, "a Ghilan"),
        (900745, 55, 16544, 10, 1388, 60, "Summon Ghilan (60)", "003A", "Ghilan", 50, 3945, 2.0, "a Ghilan")]


def mage_sql():
    out = []
    for sid, lvl, rng, cast, power, recast, name, sheet, pet, plvl, hp, size, words in MAGE:
        eff = json.dumps({"v": 1, "scope": "self", "target_type": "Self", "harmful": False, "stack": 0, "level": lvl,
                          "min_level": 1, "req_equipment": "Any", "sheet_type": "Summon Pet",
                          "effects": [{"id": "0307", "type": "Summon Pet", "kind": "summon",
                                       "pet": {"sheet_pet_id": sheet, "name": pet}}]}, separators=(",", ":"))
        out.append(f"INSERT INTO spellpattern VALUES ({sid}, {lvl}, 0, 0, {rng}, {cast}, {power}, -1809115846, -862007142, 0, "
                   f"{recast}, -1, '{name}', 'MAG');")
        out.append(f"INSERT INTO spelleffect VALUES ({sid}, 'Summons {words} to aid you', 489318137, 'eqoaemu', '{eff}');")
        out.append(f"INSERT INTO petstats VALUES ({sid}, '{sheet}', '{pet}', 1689387666, {plvl}, 'priest', {hp}, 100, 50, "
                   f"'[{{\"every\":30.0,\"kind\":\"heal\",\"spell_id\":902139}}]', 'workbook', 'eqoaemu', {size});")
    return "\n".join(out)


def make(path, race="Mermaid", extra=""):
    db = sqlite3.connect(path)
    db.executescript(SCHEMA)
    db.executescript(mage_sql())
    db.execute("INSERT INTO racedefs VALUES (32, ?)", (race,))
    db.executescript(extra)
    db.commit()
    db.close()


def dump(path):
    db = sqlite3.connect(path)
    out = [l for l in db.iterdump() if "gm_audit" not in l and "sqlite_sequence" not in l]
    db.close()
    return out


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


class Tool(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = os.path.join(self.dir.name, "t.db")

    def tearDown(self):
        self.dir.cleanup()

    def test_apply_again_undo(self):
        make(self.db)
        before = dump(self.db)
        r = run(self.db)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("report only: 25 rows new", r.stdout)
        self.assertEqual(dump(self.db), before)
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit").fetchone()[0], 25)
        self.assertEqual(db.execute("SELECT group_concat(id || ':' || cost || ':' || level, ' ') FROM cmcatalog "
                                    "WHERE id BETWEEN 2177 AND 2182").fetchone()[0],
                         "2177:15:15 2178:15:25 2179:15:35 2180:15:45 2181:15:55 2182:50:60")
        self.assertEqual(db.execute("SELECT count(*) FROM cmcatalog WHERE id BETWEEN 2177 AND 2182 AND races = ? "
                                    "AND classes = 32767", (1 << 32,)).fetchone()[0], 6)
        # the copies: the magician's records but id, level, description and source; the Ascension's pet 51
        for new, src in ((760180, 640), (760184, 700557), (760179, 900745)):
            sp_new = db.execute("SELECT range, casttime, power, recast, icon, iconColor, scope, spellname FROM spellpattern "
                                "WHERE spellid = ?", (new,)).fetchone()
            sp_src = db.execute("SELECT range, casttime, power, recast, icon, iconColor, scope, spellname FROM spellpattern "
                                "WHERE spellid = ?", (src,)).fetchone()
            self.assertEqual(sp_new[:7], sp_src[:7], new)
            ps_new = db.execute("SELECT sheet_pet_id, name, model, level, type, hp, abilities, size FROM petstats "
                                "WHERE spellid = ?", (new,)).fetchone()
            ps_src = db.execute("SELECT sheet_pet_id, name, model, level, type, hp, abilities, size FROM petstats "
                                "WHERE spellid = ?", (src,)).fetchone()
            want = list(ps_src)
            if new == 760179:
                want[3] = 51
            self.assertEqual(list(ps_new), want, new)
        self.assertEqual(db.execute("SELECT spellname, abilitylvl FROM spellpattern WHERE spellid = 760179").fetchone(),
                         ("Aquatic Ascension", 60))
        self.assertEqual(db.execute("SELECT abilitylvl FROM spellpattern WHERE spellid = 760182").fetchone()[0], 35)
        self.assertEqual(json.loads(db.execute("SELECT effects FROM spelleffect WHERE spellid = 760183").fetchone()[0])["level"],
                         45)
        self.assertEqual(db.execute("SELECT template FROM pettemplate WHERE spell = 'aquatic ascension'").fetchone()[0],
                         "pet water")
        db.close()
        applied = dump(self.db)
        r = run(self.db, "--apply")
        self.assertIn("applied already", r.stdout)
        self.assertEqual(dump(self.db), applied)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertEqual(dump(self.db), before)

    def test_refusals(self):
        for extra, race in (("INSERT INTO cmcatalog (id, races, name) VALUES (2199, 4294967296, 'Placeholder');", "Mermaid"),
                            ("INSERT INTO cmcatalog (id, name) VALUES (2180, 'Something');", "Mermaid"),
                            ("INSERT INTO spellpattern VALUES (760181, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'X', '');", "Mermaid"),
                            ("INSERT INTO petstats (spellid, name) VALUES (760179, 'x');", "Mermaid"),
                            ("INSERT INTO pettemplate (spell, template) VALUES ('aquatic ascension', 'pet fire');", "Mermaid"),
                            ("INSERT INTO spellpattern VALUES (5, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Aquatic Ascension', '');",
                             "Mermaid"),
                            ("DELETE FROM cmcatalog WHERE id = 2174;", "Mermaid"),
                            ("UPDATE spellpattern SET spellname = 'Water Elementaling!' WHERE spellid = 640;", "Mermaid"),
                            ("UPDATE petstats SET sheet_pet_id = '0099' WHERE spellid = 900745;", "Mermaid"),
                            ("", "Siren")):
            if os.path.exists(self.db):
                os.remove(self.db)
            make(self.db, race, extra)
            before = dump(self.db)
            r = run(self.db, "--apply")
            self.assertEqual(r.returncode, 1, extra + r.stdout)
            self.assertIn("refused", r.stdout)
            self.assertEqual(dump(self.db), before)

    def test_undo_leaves_a_changed_row(self):
        make(self.db)
        run(self.db, "--apply")
        db = sqlite3.connect(self.db)
        db.execute("UPDATE petstats SET hp = 4000 WHERE spellid = 760179")
        db.commit()
        db.close()
        r = run(self.db, "--apply")
        self.assertEqual(r.returncode, 1)
        self.assertIn("--undo first", r.stdout)
        r = run(self.db, "--undo")
        self.assertEqual(r.returncode, 1)
        self.assertIn("put back 24, 1 left", r.stdout)
        db = sqlite3.connect(self.db)
        self.assertEqual(db.execute("SELECT hp FROM petstats WHERE spellid = 760179").fetchone()[0], 4000)
        db.close()

    def test_ranks_are_the_go_tests(self):
        go = open(GOTEST).read()
        table = re.search(r"const mermaidPetRanks = `([^`]*)`", go).group(1).splitlines()
        db_path = os.path.join(self.dir.name, "g.db")
        make(db_path)
        db = sqlite3.connect(db_path)
        rows, refusals = mp.build(db)
        db.close()
        self.assertEqual(refusals, [])
        mine = ["|".join(str(r[c]) for c in ("id", "category", "mastery", "rank", "name", "parent", "cost", "level", "adds",
                                             "add_spells", "removes", "remove_spells", "require_ranks"))
                for t, r in rows if t == "cmcatalog"]
        self.assertEqual(table[1:], mine)


if __name__ == "__main__":
    unittest.main()
