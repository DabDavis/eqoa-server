#!/usr/bin/env python3
"""tools/ranger_start_20261127.py against a made-up database: the western human ranger's row (the glade), the elves'
own ranger row (Fayspires), an eastern human and a dark elf with warrior rows and no ranger's, a race of racedefs with a
ranger row in its homeland, the glade's and Qeynos's factions, the Surefall line and the good-melee epic, four ranger
characters (a dark elf with a moved standing below the floor, an erudite on its start, an elf and a western human) and
the glade's people. No game data. An apply moves or makes every ranger row but the elves' at the glade, raises the
starts and the existing rangers to the floors, opens the line to every race but the elves, the epic to the races it
lacked, and adds the glade's welcome; the elves are untouched; a second apply is refused; a fresh run finds nothing to
do; --undo puts every table back row for row (a row changed since is kept).

    python3 tools/ranger_start_20261127_test.py
"""
import contextlib
import io
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import ranger_start_20261127 as rs  # noqa: E402

STATS = ", ".join(f"{s} INTEGER" for s in rs.STATS)
SCHEMA = [
    f"""CREATE TABLE defaultcharacter (defaultCharacterID INTEGER PRIMARY KEY AUTOINCREMENT, tclass INTEGER NOT NULL,
        race INTEGER NOT NULL, humType INTEGER NOT NULL, level INTEGER NOT NULL, tunar INTEGER NOT NULL,
        bankTunar INTEGER NOT NULL, unusedTP INTEGER NOT NULL, totalTP INTEGER NOT NULL, x REAL NOT NULL, y REAL NOT NULL,
        z REAL, facing REAL NOT NULL, defaultSpeed REAL, world INTEGER, {STATS})""",
    "CREATE TABLE factions (name TEXT PRIMARY KEY, standing INTEGER NOT NULL)",
    """CREATE TABLE factionstart (id INTEGER PRIMARY KEY AUTOINCREMENT, race INTEGER NOT NULL DEFAULT -1,
        class INTEGER NOT NULL DEFAULT -1, humtype INTEGER NOT NULL DEFAULT -1, faction TEXT NOT NULL,
        standing INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
        UNIQUE (race, class, humtype, faction))""",
    "CREATE TABLE racedefs (id INTEGER PRIMARY KEY, name TEXT NOT NULL, wire_race INTEGER NOT NULL, city INTEGER NOT NULL)",
    """CREATE TABLE characters (charName TEXT NOT NULL, serverid INTEGER PRIMARY KEY, tclass INTEGER NOT NULL,
        race INTEGER NOT NULL, humType INTEGER NOT NULL, level INTEGER NOT NULL)""",
    """CREATE TABLE charfactions (serverid INTEGER NOT NULL, faction TEXT NOT NULL, standing INTEGER NOT NULL,
        PRIMARY KEY (serverid, faction))""",
    """CREATE TABLE npcdialog (id INTEGER PRIMARY KEY AUTOINCREMENT, town TEXT NOT NULL DEFAULT '',
        role TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '', mood TEXT NOT NULL DEFAULT '',
        race INTEGER NOT NULL DEFAULT -1, line TEXT NOT NULL, weight INTEGER NOT NULL DEFAULT 1,
        source TEXT NOT NULL DEFAULT 'gm', sex TEXT NOT NULL DEFAULT '', speaker_race INTEGER NOT NULL DEFAULT -1,
        away INTEGER NOT NULL DEFAULT 0, class INTEGER NOT NULL DEFAULT -1, not_race INTEGER NOT NULL DEFAULT -1,
        use TEXT NOT NULL DEFAULT '')""",
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL)",
    """CREATE TABLE npcfaction (id INTEGER PRIMARY KEY, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
        x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL)""",
    """CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
        giver TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1, classes TEXT NOT NULL DEFAULT '',
        races TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')""",
    """CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
        username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
        tbl TEXT NOT NULL DEFAULT '', rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT,
        detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER)""",
]
WEST_WAR = (75, 80, 65, 65, 60, 55, 55)
WEST_RNG = (65, 70, 70, 80, 60, 55, 55)
DELF_WAR = (70, 80, 60, 70, 60, 65, 50)
GLADE = (0, 9005.05, 12749.16)
ELF_SPOT = (0, 18380.2, 7378.4)


def row(cls, race, hum, x, z, world, stats):
    return (cls, race, hum, 1, 0, 0, 0, 0, x, 58.0, z, 1.0, 0.0, world) + stats


TABLES = ["defaultcharacter", "factionstart", "charfactions", "npcdialog", "quests"]


class RangerStartTest(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "eqoa.db")
        self.applied = os.path.join(self.dir, "applied.json")
        db = sqlite3.connect(self.path)
        for s in SCHEMA:
            db.execute(s)
        cols = ("tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, defaultSpeed, "
                f"world, {', '.join(rs.STATS)}")
        for r in [row(0, 0, 2, 4243.7, 17169.1, 0, WEST_WAR), row(1, 0, 2, GLADE[1], GLADE[2], 0, WEST_RNG),
                  row(1, 1, 0, ELF_SPOT[1], ELF_SPOT[2], 0, (60, 65, 80, 85, 60, 60, 55)),
                  row(0, 1, 0, 18000.0, 7000.0, 0, (70, 75, 75, 70, 60, 60, 55)),
                  row(0, 0, 1, 25486.5, 15547.0, 0, WEST_WAR), row(0, 2, 0, 25220.1, 8626.2, 0, DELF_WAR),
                  row(2, 0, 2, 4300.0, 17100.0, 0, (80, 75, 60, 60, 70, 55, 65)),
                  row(2, 8, 0, 4727.4, 21654.9, 0, (65, 70, 60, 60, 65, 70, 50)),
                  row(0, 11, 0, 6938.5, 3356.6, 10, (95, 95, 55, 55, 55, 50, 50)),
                  row(1, 11, 0, 6938.5, 3356.6, 10, (85, 85, 60, 70, 55, 50, 50))]:
            db.execute(f"INSERT INTO defaultcharacter ({cols}) VALUES ({', '.join('?' * len(r))})", r)
        db.executemany("INSERT INTO factions VALUES (?, ?)", [(f, 1) for f in rs.SET] + [("Citizens of Neriak", 1)])
        db.executemany("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (?, ?, ?, ?, ?)", [
            (0, -1, -1, "Citizens of Surefall Glade", 151), (0, -1, -1, "Citizens of Qeynos", 151),
            (0, -1, -1, "The Qeynos Guard", 0), (0, -1, 1, "Citizens of Surefall Glade", -100),
            (2, -1, -1, "Citizens of Surefall Glade", -2000), (2, -1, -1, "Citizens of Qeynos", -2000),
            (2, -1, -1, "The Qeynos Guard", -2000), (2, -1, -1, "Citizens of Neriak", 151),
            (11, -1, -1, "The Qeynos Guard", -2000), (1, -1, -1, "The Qeynos Guard", -50)])
        db.execute("INSERT INTO racedefs VALUES (11, 'Hill Giant', 9, 0)")
        db.executemany("INSERT INTO characters VALUES (?, ?, ?, ?, ?, ?)", [
            ("Drow", 1, 1, 2, 0, 60), ("Erudo", 2, 1, 8, 0, 20), ("Elfie", 3, 1, 1, 0, 19), ("Wester", 4, 1, 0, 2, 10),
            ("Warrior", 5, 0, 2, 0, 5)])
        db.executemany("INSERT INTO charfactions VALUES (?, ?, ?)", [
            (1, "The Qeynos Guard", -500), (1, "Citizens of Qeynos", 400), (1, "Citizens of Neriak", -50),
            (4, "The Qeynos Guard", -10)])
        db.execute("INSERT INTO npcdialog (town, line) VALUES ('Surefall Glade', 'The glade.')")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
            (1, "Sigmor Fallbourne", 0, 8998.0, 12735.0), (2, "Guard Fellwood", 0, 4400.0, 17300.0),
            (3, "Merchant Lendal", 0, 9010.0, 12751.0)])
        db.executemany("INSERT INTO npcfaction (name, faction) VALUES (?, ?)", [
            ("sigmor fallbourne", "Citizens of Surefall Glade"), ("guard fellwood", "The Qeynos Guard"),
            ("merchant lendal", "Citizens of Qeynos")])
        for i in range(10):
            db.execute("INSERT INTO quests (questid, name, giver, minlevel, classes, races, source) VALUES "
                       "(?, ?, 'Sigmor Fallbourne', ?, '1', '0, 8', ?)",
                       (100697 + i, f"line {i}", i + 1, f"{rs.LINE_SRC}/L{i + 1} (tools/import)"))
        db.execute("INSERT INTO quests (questid, name, giver, minlevel, classes, races, source) VALUES "
                   "(101043, 'The Recruiter', 'Recruiter', 60, '1, 4', '0, 1, 8, 10, 11', ?)", (rs.EPIC_START + " (x)",))
        db.execute("INSERT INTO quests (questid, name, giver, minlevel, classes, races, source) VALUES "
                   "(101044, 'Goo', 'Sensei', 60, '1, 4, 5, 6', '0, 1, 3, 4, 6, 7, 8, 10, 11', ?)", (rs.EPIC_EPS + "1-1",))
        db.execute("INSERT INTO quests (questid, name, giver, minlevel, classes, races, source) VALUES "
                   "(101099, 'Reward', 'Sensei', 60, '1', '', 'EQOA-Data 60/good-melee/reward')")
        db.commit()
        db.close()
        self.before = self.snapshot()

    def tearDown(self):
        for f in os.listdir(self.dir):
            os.remove(os.path.join(self.dir, f))
        os.rmdir(self.dir)

    def snapshot(self):
        db = sqlite3.connect(self.path)
        out = {t: db.execute(f"SELECT * FROM {t} ORDER BY 1, 2").fetchall() for t in TABLES}
        db.close()
        return out

    def run_tool(self, *args, applied=None):
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            code = rs.main(["--db", self.path, "--shard", "heirloom", "--applied", applied or self.applied, *args])
        return code, buf.getvalue()

    def q(self, sql, args=()):
        db = sqlite3.connect(self.path)
        try:
            return db.execute(sql, args).fetchall()
        finally:
            db.close()

    def test_report_writes_nothing(self):
        code, out = self.run_tool()
        self.assertEqual(code, 0, out)
        self.assertIn("OK: every ranger but the elves'", out)
        self.assertEqual(self.snapshot(), self.before)
        self.assertFalse(os.path.exists(self.applied))

    def test_apply_refuse_and_undo(self):
        code, out = self.run_tool("--apply")
        self.assertEqual(code, 0, out)
        rangers = self.q("SELECT race, humType, world, x, z, " + ", ".join(rs.STATS) +
                         " FROM defaultcharacter WHERE tclass = 1 ORDER BY race, humType")
        self.assertEqual([r[:2] for r in rangers], [(0, 1), (0, 2), (1, 0), (2, 0), (8, 0), (11, 0)])
        for r in rangers:
            self.assertEqual(r[2:5], ELF_SPOT if r[0] == 1 else GLADE, r)
        self.assertEqual(rangers[3][5:], tuple(d + w - x for d, w, x in zip(DELF_WAR, WEST_RNG, WEST_WAR)))
        self.assertEqual(rangers[5][5:], (85, 85, 60, 70, 55, 50, 50), "the giant's own stats kept")
        self.assertEqual(self.q("SELECT tclass, world, x, z FROM defaultcharacter WHERE race = 8 ORDER BY tclass"),
                         [(0, 0, 4727.4, 21654.9), (1, *GLADE), (2, 0, 4727.4, 21654.9)],
                         "the erudites' classes without rows keep starting where their paladin's row stands")
        self.assertEqual(self.q("SELECT " + ", ".join(rs.STATS) + " FROM defaultcharacter WHERE race = 8 AND tclass = 0"),
                         [tuple(e + w - p for e, w, p in zip((65, 70, 60, 60, 65, 70, 50), WEST_WAR,
                                                              (80, 75, 60, 60, 70, 55, 65)))])
        st = rs.State(sqlite3.connect(self.path))
        for race, hum in [(0, 1), (2, 0), (8, 0), (11, 0), (rs.FROGLOK, 0)]:
            combo = rs.combo_of(race, rs.RANGER, hum)
            for f, floor in {"Citizens of Surefall Glade": 151, "Citizens of Qeynos": 151, "The Qeynos Guard": 0}.items():
                self.assertGreaterEqual(st.start(f, combo), floor, (race, f))
        self.assertEqual(st.start("The Qeynos Guard", (1, 1, 0)), -50, "the elf ranger's start untouched")
        self.assertEqual(st.start("Citizens of Neriak", (2, 1, 0)), 151, "the race's own standings kept")
        self.assertEqual(st.start("Citizens of Qeynos", (2, 0, 0)), -2000, "the dark elf warrior untouched")
        own = dict(((s, f), v) for s, f, v in self.q("SELECT serverid, faction, standing FROM charfactions"))
        self.assertEqual(own[(1, "The Qeynos Guard")], 0, "a moved standing below the floor raised")
        self.assertEqual(own[(1, "Citizens of Surefall Glade")], 151, "a start below the floor written at it")
        self.assertEqual(own[(1, "Citizens of Qeynos")], 400, "an earned standing above the floor kept")
        self.assertEqual(own[(1, "Citizens of Neriak")], -50, "another faction untouched")
        self.assertEqual(own[(2, "Citizens of Surefall Glade")], 151, "the erudite ranger raised")
        self.assertEqual(own[(2, "Citizens of Qeynos")], 151)
        self.assertNotIn((2, "The Qeynos Guard"), own, "at the floor already: nothing written")
        self.assertFalse([k for k in own if k[0] == 3], "the elf ranger untouched")
        self.assertEqual(own[(4, "The Qeynos Guard")], -10, "the western human ranger untouched")
        self.assertFalse([k for k in own if k[0] == 5], "a warrior untouched")
        line = self.q("SELECT DISTINCT races FROM quests WHERE source LIKE ?", (rs.LINE_SRC + "%",))
        self.assertEqual(line, [("0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11",)], "every race but the elves")
        self.assertEqual(self.q("SELECT races FROM quests WHERE questid = 101043"),
                         [("0, 1, 2, 3, 5, 6, 7, 8, 9, 10, 11",)], "the dwarves keep their own Recruiter")
        self.assertEqual(self.q("SELECT races FROM quests WHERE questid = 101044"),
                         [("0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11",)])
        self.assertEqual(self.q("SELECT races FROM quests WHERE questid = 101099"), [("",)], "every race already")
        lines = self.q("SELECT class, not_race, town FROM npcdialog WHERE source = ?", (rs.SOURCE,))
        self.assertEqual(len(lines), len(rs.LINES))
        self.assertTrue(all(r == (1, 0, "Surefall Glade") for r in lines))
        self.assertEqual(self.q("SELECT COUNT(DISTINCT petition) FROM petition_fix_log"), [(1,)])
        self.assertGreater(self.q("SELECT COUNT(*) FROM gm_audit")[0][0], 0)

        with self.assertRaises(SystemExit):
            self.run_tool("--apply")
        with self.assertRaises(SystemExit):
            self.run_tool("--apply")
        again = os.path.join(self.dir, "again.json")
        code, out = self.run_tool(applied=again)
        self.assertIn("0 changes, 0 rows added", out)
        self.assertEqual(code, 0, out)

        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.snapshot(), self.before)
        self.assertFalse(os.path.exists(self.applied))

    def test_undo_keeps_a_row_changed_since(self):
        self.run_tool("--apply")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charfactions SET standing = 300 WHERE serverid = 2 AND faction = 'Citizens of Qeynos'")
        db.commit()
        db.close()
        code, out = self.run_tool("--undo")
        self.assertIn("kept:", out)
        self.assertEqual(self.q("SELECT standing FROM charfactions WHERE serverid = 2 AND faction = 'Citizens of Qeynos'"),
                         [(300,)])
        self.assertEqual(self.q("SELECT COUNT(*) FROM charfactions WHERE serverid = 2"), [(1,)])

    def test_refused_without_the_line(self):
        db = sqlite3.connect(self.path)
        db.execute("DELETE FROM quests WHERE questid = 100700")
        db.commit()
        db.close()
        before = self.snapshot()
        with self.assertRaises(SystemExit):
            self.run_tool("--apply")
        self.assertEqual(self.snapshot(), before)


if __name__ == "__main__":
    unittest.main()
