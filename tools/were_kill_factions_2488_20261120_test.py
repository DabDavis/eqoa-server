#!/usr/bin/env python3
"""were_kill_factions_2488_20261120.py on a small database: the elder and ancient weres of each line get their society's
+10, the plain ones that have it and the named weres nothing; a second apply adds nothing; --undo takes them out exactly."""
import os
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
TOOL = os.path.join(HERE, "were_kill_factions_2488_20261120.py")

SCHEMA = """
CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL, faction TEXT NOT NULL,
    delta INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm', UNIQUE (npc_name, faction));
CREATE TABLE factions (name TEXT PRIMARY KEY, x INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, accountid INTEGER, username TEXT, via TEXT, ip TEXT,
    action TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, detail TEXT);
"""


def run(path, *args):
    return subprocess.run([sys.executable, TOOL, "--db", path, *args], capture_output=True, text=True)


class WereKills(unittest.TestCase):
    def test_apply_undo(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "eqoa.db")
            db = sqlite3.connect(p)
            db.executescript(SCHEMA)
            for f in ("Topi Di Ombra", "Night Reapers", "Sunmanes", "The Bone Crusher", "Wards of Dawn"):
                db.execute("INSERT INTO factions VALUES (?, 1)", (f,))
            db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES ('wererat', 'Topi Di Ombra', 10, 'were-kills')")
            for n in ("wererat", "elder wererat", "ancient ratwere", "elder werewolf", "great bearwere", "Werebear Witchking",
                      "Werehunter", "ancient weregator", "a rat"):
                db.execute("INSERT INTO npcs (npc_name) VALUES (?)", (n,))
            db.commit()
            db.close()
            r = run(p, "--apply")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            db = sqlite3.connect(p)
            got = set(db.execute("SELECT npc_name, faction, delta FROM factionhits WHERE source = 'were-kills'"))
            self.assertEqual(got, {("wererat", "Topi Di Ombra", 10), ("elder wererat", "Topi Di Ombra", 10),
                                   ("ancient ratwere", "Topi Di Ombra", 10), ("elder werewolf", "Night Reapers", 10),
                                   ("great bearwere", "Wards of Dawn", 10), ("ancient weregator", "The Bone Crusher", 10)})
            db.close()
            self.assertIn("nothing to do", run(p, "--apply").stdout)
            r = run(p, "--undo")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            db = sqlite3.connect(p)
            self.assertEqual(db.execute("SELECT COUNT(*) FROM factionhits").fetchone(), (1,))
            db.close()


if __name__ == "__main__":
    unittest.main()
