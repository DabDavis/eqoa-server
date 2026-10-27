#!/usr/bin/env python3
"""tools/fix_pod_bosses.py against a made-up database: a PoD seed's tables,
the rows it moves as the old plan wrote them (two of one name), a rule, a
GM's edit. No game data (the plans are written out here; the real ones come
from seed_pod.py and the ground files). With EQOA_REAL_DB (a seeded
database) and EQOA_GROUND_DIR (its ground files), the tool itself runs on a
copy of it, apply then undo.

    python3 tools/fix_pod_bosses_test.py
"""
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_pod_bosses as fx  # noqa: E402

TOOL = os.path.join(HERE, "fix_pod_bosses.py")
COLS = ["npc_name", "x_coord", "y_coord", "z_coord", "facing", "world", "dt_kill", "modelid", "size", "race",
        "npc_level", "npc_type", "npc_id", "zone"]
PATRON = "Patron of Bertoxxulous"
BERT = "Bertoxxulous the Plaguebringer"


def npc(name, x, y, z, facing, model, size, race="0", level=65, kind=0):
    return dict(npc_name=name, x_coord=x, y_coord=y, z_coord=z, facing=facing, world=5, dt_kill=None, modelid=model,
                size=size, race=race, npc_level=level, npc_type=kind, npc_id=0, zone=0)


def entry(row, body, body_row, role):
    return dict(row=row, body=body, body_row=body_row, role=role)


def portal(name, x, y, z, facing, role):
    return entry(npc(name, x, y, z, facing, 0x0000D001, 1.0, level=60, kind=384), "portal", 24, role)


def patron(x, y, z, facing):
    return entry(npc(PATRON, x, y, z, facing, 0x0000E001, 1.0, level=59), "zombie", 25, "patron")


OLD = dict(rows=[
    entry(npc("Noroxus", 5620.0, -30.54, 3060.0, 243, 0x0000A001, 2.0), "ghoul", 11, "boss"),
    portal("Bloody Portal", 4872.0, -19.938, 2980.0, 0, "to the twins"),
    portal("Portal of the Twins", 4166.0, -16.438, 2318.0, 64, "back"),
    entry(npc(BERT, 4416.0, -22.188, 2113.0, 192, 0x0000B001, 1.6, level=66), "human", 12, "boss"),
    entry(npc("Poxathixius", 4130.0, -18.188, 2170.0, 209, 0x0000C001, 1.5), "drake", 13, "twin"),
    entry(npc("Uzrathixius", 4190.0, -19.32, 2140.0, 246, 0x0000C001, 1.5), "drake", 13, "twin"),
    patron(4220.0, -16.438, 2200.0, 27),
    patron(4226.0, -16.438, 2200.0, 64),
], rules=[
    ("npcmove", "name", "noroxus", dict(name="noroxus", roam=0, note="PoD", source="pod-seed")),
    ("npcaggro", "npc_name", BERT,
     dict(npc_name=BERT, aggressive=1, radius=0.0, note="PoD: a human's body", source="pod-seed")),
])
NEW = dict(rows=[
    entry(npc("Noroxus", 4155.0, -20.812, 2100.0, 90, 0x0000A002, 1.1, race="6"), "hydra, wearing 0000A002", 21, "boss; arena"),
    portal("Bloody Portal", 5243.0, -25.081, 3175.0, 128, "to the twins"),
    portal("Portal of the Twins", 4872.0, -19.938, 2987.0, 128, "back"),
    entry(npc(BERT, 4842.0, -23.562, 2750.0, 0, 0x0000B002, 1.3, level=66), "ghoul, wearing 0000B002", 11, "boss"),
    entry(npc("Poxathixius", 4937.0, -22.129, 2904.0, 229, 0x0000C002, 1.5, race="6"), "dragon", 23, "twin"),
    entry(npc("Uzrathixius", 4807.0, -23.588, 2904.0, 27, 0x0000C002, 1.5, race="6"), "dragon", 23, "twin"),
    patron(4783.0, -25.664, 2728.0, 27),
    patron(4888.0, -24.005, 2713.0, 64),
    portal("Fuming Portal", 5620.0, -30.54, 3060.0, 54, "to the lair"),
    portal("Portal to the Cauldron", 4226.0, -24.157, 2097.0, 174, "back"),
], rules=[
    ("npcmove", "name", "noroxus", dict(name="noroxus", roam=0, note="PoD", source="pod-seed")),
])
CHANGED_ROWS = ["Noroxus", BERT, "Poxathixius", "Uzrathixius", PATRON, PATRON, "Bloody Portal", "Portal of the Twins"]


def make_db(path):
    db = sqlite3.connect(path, isolation_level=None)
    db.executescript(f"""
        CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, {', '.join(COLS)});
        CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER NOT NULL DEFAULT 1,
            radius REAL NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE npcmove (name TEXT PRIMARY KEY, roam INTEGER NOT NULL DEFAULT -1, radius REAL NOT NULL DEFAULT 0,
            water TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm');
        CREATE TABLE gm_audit (id INTEGER PRIMARY KEY, tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '');
        CREATE TABLE pod_seed_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
        CREATE TABLE pod_seed_npcs (npc_row INTEGER PRIMARY KEY, name TEXT NOT NULL, body_row INTEGER NOT NULL,
            role TEXT NOT NULL);
        CREATE TABLE pod_seed_rules (tbl TEXT NOT NULL, keycol TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY (tbl, name));
    """)
    put = lambda r: db.execute(f"INSERT INTO npcs ({', '.join(COLS)}) VALUES ({', '.join('?' * len(COLS))})",
                               [r[c] for c in COLS]).lastrowid
    put(npc("a GM's rat", 1.0, 2.0, 3.0, 0, 7, 1.0))
    for e in OLD["rows"]:
        db.execute("INSERT INTO pod_seed_npcs VALUES (?, ?, ?, ?)", (put(e["row"]), e["row"]["npc_name"], e["body_row"], e["role"]))
    # rows written after the seed: a new id must not reuse theirs, nor the undo forget them
    put(npc("a quest's creature", 9.0, 9.0, 9.0, 0, 8, 1.0))
    # a rule another tool wrote between the seed's two (its rowid sits between)
    db.execute("INSERT INTO npcaggro VALUES ('Cave Minnow', 0, 0, 'peaceful', 'aggro-seed')")
    for tbl, keycol, key, vals in OLD["rules"]:
        names = list(vals)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [vals[n] for n in names])
        db.execute("INSERT INTO pod_seed_rules VALUES (?, ?, ?)", (tbl, keycol, key))
    db.execute("INSERT INTO npcaggro VALUES ('Orc Pawn', 1, 0, '', 'gm')")
    db.executemany("INSERT INTO pod_seed_meta VALUES (?, ?)", [("rows", "8"), ("rules", "2"), ("seq_before", "1")])
    return db


def dump(db):
    return "\n".join(db.iterdump())


def boss(db, name):
    return db.execute("SELECT modelid, size, x_coord, z_coord, facing, race FROM npcs WHERE npc_name = ?", (name,)).fetchone()


class ApplyAndUndo(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.db = make_db(os.path.join(self.dir.name, "eqoa.db"))
        self.before = dump(self.db)

    def tearDown(self):
        self.db.close()
        self.dir.cleanup()

    def test_the_work(self):
        w = fx.work(self.db, OLD, NEW)
        self.assertEqual([c["name"] for c in w["change"]], CHANGED_ROWS)
        nox, bert = w["change"][0], w["change"][1]
        self.assertEqual(set(nox["after"]), {"x_coord", "y_coord", "z_coord", "facing", "modelid", "size", "race"},
                         "only the columns the plan changes")
        self.assertEqual(set(bert["after"]), {"x_coord", "y_coord", "z_coord", "facing", "modelid", "size"})
        patrons = [c for c in w["change"] if c["name"] == PATRON]
        self.assertEqual([(c["before"]["x_coord"], c["after"]["x_coord"]) for c in patrons], [(4220.0, 4783.0), (4226.0, 4888.0)],
                         "a name's rows meet the plan's in order")
        self.assertEqual(set(patrons[0]["after"]), {"x_coord", "y_coord", "z_coord"})
        self.assertEqual([r["row"]["npc_name"] for r in w["add"]], ["Fuming Portal", "Portal to the Cauldron"])
        self.assertEqual([t["key"] for t in w["take_out"]], [BERT])
        self.assertEqual((w["left"], w["done"], w["put_in"]), ([], [], []))
        self.assertEqual(dump(self.db), self.before, "working it out writes nothing")

    def test_apply_then_undo(self):
        fx.apply(self.db, fx.work(self.db, OLD, NEW))
        self.assertEqual(boss(self.db, "Noroxus"), (0xA002, 1.1, 4155.0, 2100.0, 90, "6"))
        self.assertEqual(boss(self.db, BERT), (0xB002, 1.3, 4842.0, 2750.0, 0, "0"))
        self.assertEqual(boss(self.db, "Uzrathixius")[:4], (0xC002, 1.5, 4807.0, 2904.0))
        self.assertEqual(boss(self.db, "Bloody Portal")[2:5], (5243.0, 3175.0, 128))
        self.assertEqual(boss(self.db, "Portal of the Twins")[2:5], (4872.0, 2987.0, 128))
        self.assertEqual(self.db.execute(f"SELECT x_coord, z_coord FROM npcs WHERE npc_name = '{PATRON}' ORDER BY id").fetchall(),
                         [(4783.0, 2728.0), (4888.0, 2713.0)])
        portals = self.db.execute("SELECT n.id, p.body_row FROM npcs n JOIN pod_seed_npcs p ON p.npc_row = n.id "
                                  "WHERE n.npc_name IN ('Fuming Portal', 'Portal to the Cauldron') ORDER BY n.id").fetchall()
        self.assertEqual([p[0] for p in portals], [11, 12], "the portals take new ids, past the rows added since the seed")
        self.assertEqual(self.db.execute("SELECT body_row FROM pod_seed_npcs WHERE name = 'Noroxus'").fetchone()[0], 21,
                         "the seed's log names the row the look is now copied from")
        self.assertIsNone(self.db.execute(f"SELECT * FROM npcaggro WHERE npc_name = '{BERT}'").fetchone())
        self.assertIsNone(self.db.execute("SELECT * FROM pod_seed_rules WHERE tbl = 'npcaggro'").fetchone())
        self.assertEqual(self.db.execute("SELECT count(*) FROM npcaggro").fetchone()[0], 2, "others' rules stay")
        self.assertEqual(dict(self.db.execute("SELECT key, value FROM pod_seed_meta WHERE key != 'seq_before'")),
                         {"rows": "10", "rules": "1"})

        w = fx.work(self.db, OLD, NEW)
        self.assertEqual((w["change"], w["add"], w["take_out"]), ([], [], []), "a second look finds nothing to do")
        self.assertEqual([n for n, _ in w["done"]], [*CHANGED_ROWS, *fx.ADDED],
                         "the rows as the plan has them, the portals it added seed rows")
        self.assertEqual(w["left"], [])
        with self.assertRaises(SystemExit):
            fx.apply(self.db, w)

        back, left = fx.undo(self.db)
        self.assertEqual(left, [])
        self.assertEqual(dump(self.db), self.before, "the undo puts back every row, rule rowid, count and the sequence")

    def test_rows_changed_since_the_seed_are_left(self):
        self.db.execute("UPDATE npcs SET size = 3.0 WHERE npc_name = 'Poxathixius'")
        uz = self.db.execute("SELECT id FROM npcs WHERE npc_name = 'Uzrathixius'").fetchone()[0]
        self.db.execute("INSERT INTO gm_audit (tbl, rowkey) VALUES ('npcs', ?)", (f'{{"id":{uz}}}',))
        second = self.db.execute(f"SELECT max(id) FROM npcs WHERE npc_name = '{PATRON}'").fetchone()[0]
        self.db.execute("UPDATE npcs SET facing = 99 WHERE id = ?", (second,))
        self.db.execute(f"UPDATE npcaggro SET radius = 40 WHERE npc_name = '{BERT}'")
        self.db.execute("INSERT INTO npcs (npc_name, world) VALUES ('Fuming Portal', 5)")
        w = fx.work(self.db, OLD, NEW)
        left = {(n, r): why for n, r, why in w["left"]}
        self.assertIn("size", left[("Poxathixius", uz - 1)])
        self.assertIn("gm_audit", left[("Uzrathixius", uz)])
        self.assertIn("facing", left[(PATRON, second)], "one row of a name left, the other moved")
        rule = self.db.execute(f"SELECT rowid FROM npcaggro WHERE npc_name = '{BERT}'").fetchone()[0]
        self.assertIn("changed since", left[(f"npcaggro {BERT}", rule)])
        self.assertIn("already", left[("Fuming Portal", None)])
        self.assertEqual([c["name"] for c in w["change"]], ["Noroxus", BERT, PATRON, "Bloody Portal", "Portal of the Twins"])
        self.assertEqual([r["row"]["npc_name"] for r in w["add"]], ["Portal to the Cauldron"])
        self.assertEqual(w["take_out"], [])
        before = dump(self.db)
        fx.apply(self.db, w)
        self.assertEqual(boss(self.db, "Poxathixius")[:3], (0xC001, 3.0, 4130.0), "the GM's row is as the GM left it")
        self.assertEqual(self.db.execute("SELECT x_coord, facing FROM npcs WHERE id = ?", (second,)).fetchone(), (4226.0, 99))
        fx.undo(self.db)
        self.assertEqual(dump(self.db), before)

    def test_undo_leaves_what_changed_since(self):
        fx.apply(self.db, fx.work(self.db, OLD, NEW))
        self.db.execute("UPDATE npcs SET size = 1.4 WHERE npc_name = 'Noroxus'")
        self.db.execute("UPDATE npcs SET facing = 10 WHERE npc_name = 'Fuming Portal'")
        self.db.execute(f"INSERT INTO npcaggro VALUES ('{BERT}', 1, 15, 'a GM''s', 'gm')")
        back, left = fx.undo(self.db)
        said = "\n".join(left)
        for what in ("npcs row 2 (Noroxus)", "pod_seed_npcs row 2 (Noroxus): its NPC changed since", "npcs row 11 (Fuming Portal)",
                     "pod_seed_npcs row 11 (Fuming Portal): its NPC changed since", f"npcaggro '{BERT}'", "the npcs sequence"):
            self.assertIn(what, said)
        self.assertEqual(len(left), 6, left)
        self.assertEqual(boss(self.db, "Noroxus")[:2], (0xA002, 1.4), "the change since stays")
        self.assertEqual(self.db.execute("SELECT body_row FROM pod_seed_npcs WHERE npc_row = 2").fetchone()[0], 21,
                         "and its seed log row with it")
        self.assertEqual(boss(self.db, BERT)[:3], (0xB001, 1.6, 4416.0), "the rest are put back")
        self.assertEqual(boss(self.db, "Bloody Portal")[2:4], (4872.0, 2980.0))
        self.assertIsNotNone(self.db.execute("SELECT * FROM npcs WHERE npc_name = 'Fuming Portal'").fetchone())
        self.assertIsNotNone(self.db.execute("SELECT * FROM pod_seed_npcs WHERE name = 'Fuming Portal'").fetchone(),
                             "a portal a GM changed stays a seed row")
        self.assertIsNone(self.db.execute("SELECT * FROM npcs WHERE npc_name = 'Portal to the Cauldron'").fetchone())
        self.assertEqual(self.db.execute(f"SELECT note FROM npcaggro WHERE npc_name = '{BERT}'").fetchone(), ("a GM's",))
        self.assertEqual(self.db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()[0], 12,
                         "a row it added still stands: the sequence keeps its count")
        self.assertFalse(fx.has_table(self.db, fx.LOG), "the log goes either way")


@unittest.skipUnless(os.environ.get("EQOA_REAL_DB") and os.environ.get("EQOA_GROUND_DIR"),
                     "EQOA_REAL_DB (a seeded database) and EQOA_GROUND_DIR (its ground files) not set")
class OnARealDatabase(unittest.TestCase):
    def test_apply_then_undo_is_exact(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "eqoa.db")
            shutil.copy(os.environ["EQOA_REAL_DB"], path)
            run = lambda *a: subprocess.run([sys.executable, "-I", TOOL, "--db", path, "--ground", os.environ["EQOA_GROUND_DIR"],
                                             "--no-backup", *a], capture_output=True, text=True, check=True).stdout
            db = sqlite3.connect(path)
            before = dump(db)
            out = run()
            self.assertIn("(report only", out)
            self.assertIn(" 0 left;", out, "every row the tool moves is still the seed's")
            self.assertEqual(dump(db), before)
            run("--apply")
            self.assertEqual(db.execute("SELECT count(*) FROM npcs WHERE npc_name IN ('Fuming Portal', 'Portal to the Cauldron')")
                             .fetchone()[0], 2)
            run("--undo")
            self.assertEqual(dump(db), before)
            db.close()


if __name__ == "__main__":
    unittest.main()
