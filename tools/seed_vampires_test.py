#!/usr/bin/env python3
"""tools/seed_vampires.py against a made-up database holding the vampire
quests as tools/import_quests.py imports them (drafts, their abilities not
here): the run makes the nine abilities, the blood, the Sanctuary's gate and
coffin, moves the four NPCs under the rock and imports the four quests again
as quests in the game; a second --apply is refused and --undo gives back
every table as it was, each row at its own rowid (a dump of the database is
the one before; the first version put the quests' steps back at the end of
their table). The controls: an NPC a GM moved since, a Unit of Blood someone
holds, an ability in a book and a quest a GM changed after the run are kept
by --undo (and said), a quest a GM changed before it is left as the GM left
it, and a quest the run made is taken out. No game data.

    python3 tools/seed_vampires_test.py
"""
import hashlib
import json
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import import_quests as iq  # noqa: E402
import seed_vampires as sv  # noqa: E402

SCHEMA = """
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY AUTOINCREMENT, abilitylvl INTEGER, unk2 INTEGER, unk3 INTEGER,
    range INTEGER, casttime INTEGER, power INTEGER, iconColor INTEGER, icon INTEGER, scope INTEGER, recast INTEGER,
    eqprequire INTEGER, spellname TEXT, spelldesc TEXT);
CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL, fxdict INTEGER NOT NULL, source TEXT NOT NULL);
CREATE TABLE spells (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, spellid INTEGER, addedorder INTEGER,
    onHotBar INTEGER, whereonBar INTEGER, unk1 INTEGER, showhide INTEGER);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY AUTOINCREMENT, itemcost INTEGER, itemicon INTEGER, equipslot INTEGER,
    trade INTEGER, rent INTEGER, attacktype INTEGER, weapondamage INTEGER, levelreq INTEGER, maxstack INTEGER, maxhp INTEGER,
    duration INTEGER, classuse INTEGER, raceuse INTEGER, procanim INTEGER, lore INTEGER, craft INTEGER, itemname TEXT,
    itemdesc TEXT, model INTEGER, color INTEGER);
CREATE TABLE charinventory (itemid INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER, patternid INTEGER, stackleft INTEGER,
    location INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, zone INTEGER, x_coord REAL, y_coord REAL, z_coord REAL,
    facing INTEGER, world INTEGER, dt_kill DATE, hp INTEGER, hp_percent INTEGER, modelid INTEGER, size REAL, robe_style TEXT,
    npc_level INTEGER, npc_type INTEGER, npc_id INTEGER, unk_i TEXT);
CREATE TABLE factions (name TEXT);
"""

# the NPCs the quests name, the four where the import first put them, and
# a coffin whose body the Sanctuary's copies
NPCS = [
    ("Greddy Oldman", 0, 25420.0, 55.0, 15532.0, 0, 30, 128, 0x11),
    ("Grindal", 0, 25284.0, 57.0, 15658.0, 0, 35, 128, 0x12),
    ("Hinder", 5, 4300.0, 253.5, 6118.0, 223, 30, 128, 0x13),
    ("Creed", 5, 4318.0, 253.5, 6112.0, 223, 40, 128, 0x13),
    ("Weaver", 5, 4336.0, 253.5, 6118.0, 223, 50, 128, 0x14),
    ("Simmons", 5, 4354.0, 253.5, 6124.0, 223, 60, 128, 0x14),
    ("Desmodus bat", 0, 15000.0, 60.0, 15000.0, 0, 30, 0, 0x15),
    ("coffin", 0, 25516.8, 54.1, 15529.8, 253, 1, 384, 0x16),
]


def snapshot(path):
    """Every table's rows with their rowids, in rowid order, hashed, by name
    (the run's log aside): a row put back elsewhere in its table (a dump
    lists it elsewhere) is a difference."""
    db = sqlite3.connect(path)
    out = {}
    for (t,) in db.execute("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"):
        if t == sv.LOG:
            continue
        rows = list(map(repr, db.execute(f'SELECT rowid, * FROM "{t}" ORDER BY rowid')))
        out[t] = (len(rows), hashlib.sha256("\n".join(rows).encode()).hexdigest())
    db.close()
    return out


def dump(path):
    """The database as sqlite3's .dump has it (Python's iterdump)."""
    db = sqlite3.connect(path)
    out = list(db.iterdump())
    db.close()
    return out


class SeedVampires(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.path = os.path.join(self.dir.name, "t.db")
        db = sqlite3.connect(self.path)
        db.executescript(SCHEMA)
        for name, world, x, y, z, facing, level, kind, model in NPCS:
            db.execute("INSERT INTO npcs (npc_name, zone, x_coord, y_coord, z_coord, facing, world, hp, hp_percent, modelid, size, "
                       "robe_style, npc_level, npc_type, npc_id, unk_i) VALUES (?, 0, ?, ?, ?, ?, ?, 100, 255, ?, 1.4, '255', ?, ?, 0, '')",
                       (name, x, y, z, facing, world, model, level, kind))
        db.execute("INSERT INTO itempattern (itemname, itemdesc, itemicon, trade, rent, maxstack, duration, color) "
                   "VALUES ('Randerith''s blood', 'This is the blood of a vampire.', 1778128471, 1, 1, 1, 50, 4294967295)")
        db.execute("INSERT INTO spellpattern (spellid, spellname, abilitylvl) VALUES (100, 'Minor Healing', 1)")
        db.execute("INSERT INTO factions VALUES ('Werehunters of Tunaria')")
        db.commit()
        db.close()
        # the quests as the import wrote them before: drafts, their abilities not here
        self.db = sqlite3.connect(self.path, isolation_level=None)
        p = iq.plan(self.db, iq.load_files([sv.WERES]), routes=None)
        iq.write(self.db, p)
        self.ids = sv.quest_ids(self.db)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def one(self, q, *args):
        return self.db.execute(q, args).fetchone()

    def apply(self):
        return sv.apply(self.db)

    def test_the_quests_were_drafts(self):
        self.assertEqual(sorted(self.ids), sorted(sv.QUEST_KEYS))
        for k, qid in self.ids.items():
            self.assertEqual(self.one("SELECT enabled FROM quests WHERE questid = ?", qid)[0], 0, k)

    def test_apply_then_undo(self):
        before = snapshot(self.path)
        said, written, kept, drafts = self.apply()
        self.assertEqual((said, written, kept, drafts), ([], 4, [], []))
        # the abilities, Sandstorm's two by their own ids, and their lines
        for sid, name, level, cast, recast, scope, rng, back, desc, line in sv.ABILITIES:
            got = self.one("SELECT spellname, abilitylvl, casttime, recast, scope, range, iconColor, icon, power FROM spellpattern "
                           "WHERE spellid = ?", sid)
            self.assertEqual(got, (name, level, cast, recast, scope, rng, back, sv.FRONT, 0), name)
            self.assertEqual(self.one("SELECT line, source FROM spelleffect WHERE spellid = ?", sid), (line, sv.SOURCE))
        self.assertEqual(self.one("SELECT spellname, casttime, recast FROM spellpattern WHERE spellid = 1823"), ("Sanctuary", 3, 3600))
        self.assertEqual(self.one("SELECT spellname, scope, recast FROM spellpattern WHERE spellid = 1807"), ("Feed", 1, 5))
        # the blood: 30 a stack, no trade, no rent
        self.assertEqual(self.one("SELECT maxstack, trade, rent, itemicon, equipslot FROM itempattern WHERE itemname = ?", sv.ITEM),
                         (30, 1, 0, sv.ITEM_ICON, -1))
        # the Sanctuary's place, and its people under the rock round it
        self.assertEqual(self.one("SELECT kind, place, world, x, y, z FROM spellgates WHERE spell = 'sanctuary'"),
                         ("self", "The Sanctuary") + sv.LANDING)
        for name, (x, y, z, f) in sv.NPC_SPOTS.items():
            self.assertEqual(self.one("SELECT x_coord, y_coord, z_coord, facing FROM npcs WHERE npc_name = ?", name), (x, y, z, f))
        coffin = self.one("SELECT world, x_coord, y_coord, z_coord, modelid, npc_type, npc_level FROM npcs WHERE npc_name = ?", sv.COFFIN)
        self.assertEqual(coffin, (5,) + sv.COFFIN_SPOT[:3] + (0x16, 384, 1))  # the coffin's body
        # the quests in the game: the bats' infection, the port to the landing, the abilities taught
        p1 = self.ids["Weres/vampire/p1"]
        for qid in self.ids.values():
            self.assertEqual(self.one("SELECT enabled FROM quests WHERE questid = ?", qid)[0], 1)
        self.assertEqual(json.loads(self.one("SELECT events FROM queststeps WHERE questid = ? AND step = 1", p1)[0]),
                         [{"on": "done", "do": "vampire"}])
        port = json.loads(self.one("SELECT events FROM queststeps WHERE questid = ? AND step = 2", p1)[0])
        self.assertEqual([(e["do"], e["world"], e["x"], e["y"], e["z"]) for e in port], [("teleport",) + sv.LANDING])
        self.assertEqual(self.one("SELECT spells FROM queststeps WHERE questid = ? AND step = 3", p1)[0], "1807, 1823")
        self.assertEqual(self.one("SELECT spells FROM queststeps WHERE questid = ?", self.ids["Weres/vampire/L60"])[0],
                         "760005, 760006, 760007")
        # once only
        with self.assertRaises(SystemExit):
            self.apply()
        # and back, every table as it was
        done, kept = sv.undo(self.db)
        self.assertEqual(kept, [])
        self.assertEqual(snapshot(self.path), before)

    def test_undo_keeps_what_changed_since(self):
        before = snapshot(self.path)
        self.apply()
        creed = self.one("SELECT id FROM npcs WHERE npc_name = 'Creed'")[0]
        blood = self.one("SELECT patternid FROM itempattern WHERE itemname = ?", sv.ITEM)[0]
        self.db.execute("UPDATE npcs SET x_coord = 4330 WHERE id = ?", (creed,))  # a GM's
        self.db.execute("INSERT INTO charinventory (serverid, patternid, stackleft, location) VALUES (7, ?, 3, 0)", (blood,))
        self.db.execute("INSERT INTO spells (serverid, spellid, addedorder) VALUES (7, 1807, 1)")
        done, kept = sv.undo(self.db)
        self.assertEqual(sorted(k.split(":")[0] for k in kept),
                         sorted([f"NPC {creed} (Creed)", f"item {blood}", "spelleffect 1807", "spellpattern 1807"]))
        self.assertEqual(self.one("SELECT x_coord FROM npcs WHERE id = ?", creed)[0], 4330)
        self.assertEqual(self.one("SELECT x_coord FROM npcs WHERE npc_name = 'Hinder'")[0], 4300.0)  # the control: back
        self.assertIsNone(self.one("SELECT spellid FROM spellpattern WHERE spellid = 1823"))  # in no book: gone
        after = snapshot(self.path)
        for t in ("quests", "queststeps", "quest_import_quests", "spellgates"):  # spellgates: the run made it, and dropped it
            self.assertEqual(after.get(t), before.get(t), t)

    def test_undo_puts_the_quests_rows_back_where_they_stood(self):
        """The re-import rewrites each quest's steps (deleted, then written
        at the end of their table); --undo puts the logged rows back at
        their own rowids, so the steps, the import's marks and its meta
        stand as before, and a dump of the database is the one before."""
        p1 = self.ids["Weres/vampire/p1"]

        def steps():
            return self.db.execute("SELECT rowid, * FROM queststeps WHERE questid = ? ORDER BY rowid", (p1,)).fetchall()

        def marks():
            return self.db.execute("SELECT rowid, * FROM quest_import_quests ORDER BY rowid").fetchall()

        before, marked, dumped = steps(), marks(), dump(self.path)
        self.apply()
        moved = steps()
        self.assertEqual(len(moved), 3)
        self.assertNotEqual([r[0] for r in moved], [r[0] for r in before], "the import wrote the steps at new rowids")
        sv.undo(self.db)
        self.assertEqual(steps(), before)
        self.assertEqual(marks(), marked)
        self.assertEqual(dump(self.path), dumped)

    def test_undo_takes_out_a_quest_the_run_made(self):
        """A vampire quest the database did not hold (never imported) is
        imported new by the run and taken out again by --undo."""
        k = "Weres/vampire/L60"
        qid = self.ids[k]
        for t in ("quests", "queststeps"):
            self.db.execute(f"DELETE FROM {t} WHERE questid = ?", (qid,))
        self.db.execute("DELETE FROM quest_import_quests WHERE key = ?", (k,))
        before = snapshot(self.path)
        self.apply()
        made = sv.quest_ids(self.db)[k]
        self.assertEqual(self.one("SELECT enabled FROM quests WHERE questid = ?", made)[0], 1)
        done, kept = sv.undo(self.db)
        self.assertIn(f"quest {k} taken out (the run made it)", done)
        self.assertEqual(snapshot(self.path), before)

    def test_undo_leaves_a_quest_a_gm_changed_after_the_run(self):
        """A quest a GM changes after the run stays as the GM left it (said);
        the others go back. The control is the others."""
        before = snapshot(self.path)
        self.apply()
        qid = self.ids["Weres/vampire/L40"]
        self.db.execute("UPDATE queststeps SET says = 'A GM wrote this.' WHERE questid = ?", (qid,))
        done, kept = sv.undo(self.db)
        self.assertEqual(kept, ["quest Weres/vampire/L40: changed since, left"])
        self.assertEqual(self.one("SELECT says, enabled FROM queststeps JOIN quests USING (questid) WHERE questid = ?", qid),
                         ("A GM wrote this.", 1))
        for k in ("Weres/vampire/p1", "Weres/vampire/L50", "Weres/vampire/L60"):
            self.assertEqual(self.one("SELECT enabled FROM quests WHERE questid = ?", self.ids[k])[0], 0, k)
        self.assertNotEqual(snapshot(self.path)["queststeps"], before["queststeps"])

    def test_a_quest_a_gm_changed_is_left(self):
        qid = self.ids["Weres/vampire/L50"]
        self.db.execute("UPDATE quests SET offer = 'A GM wrote this.' WHERE questid = ?", (qid,))
        said, written, kept, drafts = self.apply()
        self.assertEqual((written, kept), (3, ["Weres/vampire/L50"]))
        self.assertEqual(self.one("SELECT offer, enabled FROM quests WHERE questid = ?", qid), ("A GM wrote this.", 0))
        self.assertEqual(self.one("SELECT enabled FROM quests WHERE questid = ?", self.ids["Weres/vampire/L40"])[0], 1)  # the control

    def test_the_report_changes_nothing(self):
        before = snapshot(self.path)
        lines = sv.plan(self.db)
        self.assertEqual(sum(1 for what, _ in lines if what == "ability"), 9)
        self.assertEqual(snapshot(self.path), before)


if __name__ == "__main__":
    unittest.main()
