#!/usr/bin/env python3
"""tools/seed_faction_hits.py's guard against a re-run (board #1801): the
defaults are Prima's town kill penalty (10, not the first run's -25 / -50),
and --apply writes nothing for a name that already has any factionhits row,
in any case or spacing, whatever its source (the rows petfix 581-587 gave
the towns). No game data: the names are made up.

    python3 tools/seed_faction_hits_test.py
"""
import contextlib
import io
import os
import sqlite3
import sys
import unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import seed_faction_hits as sfh  # noqa: E402


def make_db():
    db = sqlite3.connect(":memory:")
    db.execute("""CREATE TABLE factionhits (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT NOT NULL,
        faction TEXT NOT NULL, delta INTEGER NOT NULL, source TEXT NOT NULL DEFAULT 'gm', UNIQUE (npc_name, faction))""")
    db.executemany("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)", [
        ("Guard Arnold", "Citizens of Testtown", -10, "town-seed, Prima 2003 (petfix581)"),
        ("Guard Arnold", "The Testtown Guard", -10, "prima-2003 petfix581"),
        ("Baker Bea", "Citizens of Testtown", -10, "prima-2003 petfix582"),
    ])
    return db


class Defaults(unittest.TestCase):
    def test_prima_penalty(self):
        self.assertEqual(sfh.PRIMA_PENALTY, -10)
        seen = {}

        def fake_plan(db, eqoa_data, link, reach, citizen, guard):
            seen["citizen"], seen["guard"] = citizen, guard
            return [], [], [], dict(anchors=0, kept=0, median=0.0, scale=(1, 1), offset=(0, 0))

        with mock.patch.object(sfh, "plan", fake_plan), \
                mock.patch.object(sys, "argv", ["seed_faction_hits.py", "--db", ":memory:", "--eqoa-data", "x"]), \
                contextlib.redirect_stdout(io.StringIO()):
            sfh.main()
        self.assertEqual(seen, {"citizen": -10, "guard": -10})


class NewRows(unittest.TestCase):
    def test_names_with_rows_left_alone(self):
        db = make_db()
        rows = [("Guard Arnold", "Citizens of Testtown", -25), ("Guard Arnold", "Defenders of Testtown", -50),
                (" baker bea ", "Citizens of Testtown", -25), ("Weaver Wil", "Citizens of Testtown", -10)]
        new, kept = sfh.new_rows(db, rows)
        self.assertEqual(new, [("Weaver Wil", "Citizens of Testtown", -10)])
        self.assertEqual(kept, [" baker bea ", "Guard Arnold"])


if __name__ == "__main__":
    unittest.main()
