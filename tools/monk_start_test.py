#!/usr/bin/env python3
"""tools/monk_start.py against a made-up database: the western human monk's row (the monastery), a dark elf with a
warrior row and no monk's, a race of racedefs with a monk row in its homeland, the Qeynos factions' starts, three monk
characters (a dark elf with a moved standing below the floor and one above it, a western human) and a Qeynos citizen
by the monastery. No game data. --starts makes the dark elf's monk row (its stats its warrior's plus the western
monk's over the western warrior's) and moves the homeland's, raises their Qeynos starts to the floor; --existing
raises only what is below it (an earned standing above stays, the western human is not touched); --dialog wants a
server that reads the columns; a second apply writes nothing; --undo puts every table back row for row.

    python3 tools/monk_start_test.py
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
import monk_start as ms  # noqa: E402

STATS = ", ".join(f"{s} INTEGER" for s in ms.STATS)
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
        race INTEGER NOT NULL DEFAULT -1, sex TEXT NOT NULL DEFAULT '', line TEXT NOT NULL,
        weight INTEGER NOT NULL DEFAULT 1, source TEXT NOT NULL DEFAULT 'gm',
        speaker_race INTEGER NOT NULL DEFAULT -1, away INTEGER NOT NULL DEFAULT 0)""",
    "CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, z_coord REAL)",
    """CREATE TABLE npcfaction (id INTEGER PRIMARY KEY, name TEXT NOT NULL, world INTEGER NOT NULL DEFAULT -1,
        x REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 0, faction TEXT NOT NULL)""",
]
WEST_WAR = (75, 80, 65, 65, 60, 55, 55)
WEST_MNK = (65, 75, 65, 80, 60, 55, 55)
DELF_WAR = (70, 80, 60, 70, 60, 65, 50)


def row(cls, race, hum, x, z, world, stats):
    return (cls, race, hum, 1, 0, 0, 0, 0, x, 58.0, z, 1.0, 0.0, world) + stats


TABLES = ["defaultcharacter", "factionstart", "charfactions", "npcdialog"]


class MonkStartTest(unittest.TestCase):
    def setUp(self):
        fd, self.path = tempfile.mkstemp(suffix=".db")
        os.close(fd)
        db = sqlite3.connect(self.path)
        for s in SCHEMA:
            db.execute(s)
        cols = ("tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, defaultSpeed, "
                f"world, {', '.join(ms.STATS)}")
        for r in [row(0, 0, 2, 4243.7, 17169.1, 0, WEST_WAR), row(4, 0, 2, 5593.7, 17626.6, 0, WEST_MNK),
                  row(0, 0, 1, 25486.5, 15547.0, 0, WEST_WAR), row(0, 2, 0, 25220.1, 8626.2, 0, DELF_WAR),
                  row(0, 11, 0, 6938.5, 3356.6, 10, (95, 95, 55, 55, 55, 50, 50)),
                  row(4, 11, 0, 6938.5, 3356.6, 10, (85, 90, 55, 70, 55, 50, 50))]:
            db.execute(f"INSERT INTO defaultcharacter ({cols}) VALUES ({', '.join('?' * len(r))})", r)
        db.executemany("INSERT INTO factions VALUES (?, ?)", [(f, 1) for f in ms.QEYNOS] + [("Citizens of Neriak", 1)])
        db.executemany("INSERT INTO factionstart (race, class, humtype, faction, standing) VALUES (?, ?, ?, ?, ?)", [
            (0, -1, -1, "Citizens of Qeynos", 151), (0, -1, -1, "The Qeynos Guard", 0),
            (0, -1, -1, "Qeynos Troupe", 151), (0, -1, -1, "Order of the Silent Fist", 0),
            (0, -1, 1, "Citizens of Qeynos", 1), (0, -1, 1, "Order of the Silent Fist", -100),
            (2, -1, -1, "Citizens of Qeynos", -2000), (2, -1, -1, "The Qeynos Guard", -2000),
            (2, -1, -1, "Citizens of Neriak", 151), (11, -1, -1, "The Qeynos Guard", -2000)])
        db.execute("INSERT INTO racedefs VALUES (11, 'Hill Giant', 9, 0)")
        db.executemany("INSERT INTO characters VALUES (?, ?, ?, ?, ?, ?)", [
            ("Deus", 1, 4, 2, 0, 60), ("Seraph", 2, 4, 2, 0, 60), ("Wester", 3, 4, 0, 2, 10), ("Warrior", 4, 0, 2, 0, 5)])
        db.executemany("INSERT INTO charfactions VALUES (?, ?, ?)", [
            (1, "The Qeynos Guard", -500), (2, "Citizens of Qeynos", 400), (2, "Citizens of Neriak", -50),
            (3, "The Qeynos Guard", -10)])
        db.execute("INSERT INTO npcdialog (town, line) VALUES ('Qeynos', 'Qeynos.')")
        db.executemany("INSERT INTO npcs VALUES (?, ?, ?, ?, ?)", [
            (1, "Mu Nan", 0, 5628.0, 17629.0), (2, "Guard Fellwood", 0, 4400.0, 17300.0)])
        db.executemany("INSERT INTO npcfaction (name, faction) VALUES (?, ?)", [
            ("mu nan", "Citizens of Qeynos"), ("guard fellwood", "The Qeynos Guard")])
        db.commit()
        db.close()
        self.before = self.snapshot()
        fd, self.binary = tempfile.mkstemp()
        os.write(fd, b"...SELECT id, town, role, name, mood, race, sex, line, weight, speaker_race, " + ms.BINARY_MARK)
        os.close(fd)

    def tearDown(self):
        os.remove(self.path)
        os.remove(self.binary)

    def snapshot(self):
        db = sqlite3.connect(self.path)
        out = {}
        for t in TABLES:
            cols = [r[1] for r in db.execute(f"PRAGMA table_info({t})") if r[1] not in ("class", "not_race")]
            out[t] = db.execute(f"SELECT {', '.join(cols)} FROM {t} ORDER BY 1, 2").fetchall()
        db.close()
        return out

    def run_tool(self, *args):
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            code = ms.main(["--db", self.path, "--no-backup", *args])
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
        self.assertIn("OK: every monk at the monastery", out)
        self.assertEqual(self.snapshot(), self.before)

    def test_dialog_wants_a_server_that_reads_it(self):
        with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
            ms.main(["--db", self.path, "--no-backup", "--apply", "--dialog"])
        fd, old = tempfile.mkstemp()
        os.write(fd, b"speaker_race, away FROM npcdialog")
        os.close(fd)
        try:
            with self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
                ms.main(["--db", self.path, "--no-backup", "--apply", "--dialog", "--server-binary", old])
        finally:
            os.remove(old)
        self.assertEqual(self.snapshot(), self.before)

    def test_apply_and_undo(self):
        code, out = self.run_tool("--apply", "--starts", "--existing", "--dialog", "--server-binary", self.binary)
        self.assertEqual(code, 0, out)
        here = (0, 5593.7, 17626.6)
        monks = self.q("SELECT race, humType, world, x, z, " + ", ".join(ms.STATS) +
                       " FROM defaultcharacter WHERE tclass = 4 ORDER BY race, humType")
        self.assertEqual([m[:2] for m in monks], [(0, 1), (0, 2), (2, 0), (11, 0)])
        for m in monks:
            self.assertEqual(m[2:5], here, m)
        delf = monks[2][5:]
        self.assertEqual(delf, tuple(d + w - x for d, w, x in zip(DELF_WAR, WEST_MNK, WEST_WAR)))
        self.assertEqual(monks[3][5:], (85, 90, 55, 70, 55, 50, 50), "the giant's own stats kept")
        st = ms.State(sqlite3.connect(self.path))
        for race, hum in [(0, 1), (2, 0), (11, 0), (ms.RACE_FROGLOK, 0)]:
            combo = ms.combo_of(race, 4, hum)
            for f, floor in {"Citizens of Qeynos": 151, "The Qeynos Guard": 0, "Qeynos Troupe": 151,
                             "Order of the Silent Fist": 0}.items():
                self.assertGreaterEqual(st.start(f, combo), floor, (race, f))
        self.assertEqual(st.start("Citizens of Neriak", (2, 4, 0)), 151, "the race's own standings kept")
        self.assertEqual(st.start("Citizens of Qeynos", (2, 0, 0)), -2000, "the dark elf warrior untouched")
        own = dict(((s, f), v) for s, f, v in self.q("SELECT serverid, faction, standing FROM charfactions"))
        self.assertEqual(own[(1, "The Qeynos Guard")], 0, "a moved standing below the floor raised")
        self.assertEqual(own[(1, "Citizens of Qeynos")], 151, "a start below the floor written at it")
        self.assertEqual(own[(2, "Citizens of Qeynos")], 400, "an earned standing above the floor kept")
        self.assertEqual(own[(2, "Citizens of Neriak")], -50, "another faction untouched")
        self.assertEqual(own[(3, "The Qeynos Guard")], -10, "the western human monk untouched")
        self.assertNotIn((4, "Citizens of Qeynos"), own, "a dark elf warrior untouched")
        lines = self.q("SELECT class, not_race, town FROM npcdialog WHERE source = ?", (ms.SOURCE,))
        self.assertEqual(len(lines), len(ms.LINES))
        self.assertTrue(all(r == (4, 0, "Qeynos") for r in lines))
        audits = self.q("SELECT COUNT(*) FROM gm_audit")[0][0]
        self.assertEqual(audits, self.q(f"SELECT COUNT(*) FROM {ms.LOG}")[0][0])

        code, out = self.run_tool("--apply", "--starts", "--existing", "--dialog", "--server-binary", self.binary)
        self.assertIn("wrote 0 rows", out)

        code, out = self.run_tool("--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(self.snapshot(), self.before)
        self.assertEqual(self.q(f"SELECT name FROM sqlite_master WHERE name = '{ms.LOG}'"), [])

    def test_undo_leaves_a_row_changed_since(self):
        self.run_tool("--apply", "--existing")
        db = sqlite3.connect(self.path)
        db.execute("UPDATE charfactions SET standing = 300 WHERE serverid = 1 AND faction = 'Citizens of Qeynos'")
        db.commit()
        db.close()
        code, out = self.run_tool("--undo", "--step", "existing")
        self.assertIn("left:", out)
        self.assertEqual(self.q("SELECT standing FROM charfactions WHERE serverid = 1 AND faction = 'Citizens of Qeynos'"),
                         [(300,)])
        self.assertEqual(self.q("SELECT standing FROM charfactions WHERE serverid = 1 AND faction = 'The Qeynos Guard'"),
                         [(-500,)])


if __name__ == "__main__":
    unittest.main()
