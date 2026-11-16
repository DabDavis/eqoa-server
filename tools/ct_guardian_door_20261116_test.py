#!/usr/bin/env python3
"""tools/ct_guardian_door_20261116.py: on a database without the table it makes it and the row, a second run is
applied already, --undo leaves the database as it was; a Guardian not at the door refuses; the row's boxes hold the
places petition 2212's night had at the door (internal/entity/questdoors_test.go runs them through the server)."""
import contextlib
import io
import json
import os
import sqlite3
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import ct_guardian_door_20261116 as d  # noqa: E402
import tix4_rows as tr  # noqa: E402


def holds(boxes, p):
    return any(b[0] <= p[0] <= b[3] and b[1] <= p[1] <= b[4] and b[2] <= p[2] <= b[5] for b in boxes)


class Door(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "t.db")
        db = sqlite3.connect(self.path)
        db.execute("CREATE TABLE npcs (id INTEGER PRIMARY KEY, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL, z_coord REAL)")
        db.execute("INSERT INTO npcs VALUES (118859, 'The Guardian', 0, 25706.208984375, 54.1259689331055, 3230.66088867188)")
        db.execute("""CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
            username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
            rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER)""")
        db.commit()
        db.close()

    def run_tool(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            rc = tr.main(d.SPEC, ["--db", self.path, *args])
        return rc, out.getvalue()

    def test_apply_again_undo(self):
        self.assertEqual(self.run_tool()[0], 0)
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.path)
        r = db.execute("SELECT questid, item, inner, outer, outside, text FROM questdoors WHERE npcid = 118859").fetchone()
        self.assertEqual(r[:2], (0, 0))
        self.assertEqual(json.loads(r[4]), d.OUTSIDE)
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertIn("applied already", out)
        rc, out = self.run_tool("--undo")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.path)
        self.assertIsNone(db.execute("SELECT 1 FROM questdoors").fetchone())
        db.close()

    def test_a_guardian_elsewhere_refuses(self):
        db = sqlite3.connect(self.path)
        db.execute("UPDATE npcs SET x_coord = 25000")
        db.commit()
        db.close()
        rc, out = self.run_tool("--apply")
        self.assertEqual(rc, 1)
        self.assertIn("not the Guardian at the door", out)

    def test_the_boxes(self):
        for p in ((25705.8, 55.0, 3231.9), (25705.8, 54.1, 3231.9), (25702.1, 54.1, 3222.5), tuple(d.OUTSIDE)):
            self.assertTrue(holds(d.OUTER, p) and not holds(d.INNER, p), p)
        for p in ((25712.0, 54.4, 3232.0), (25714.5, 48.9, 3228.7), (25711.6, 48.9, 3234.7), (25710.8, 48.5, 3234.7)):
            self.assertTrue(holds(d.INNER, p) and not holds(d.OUTER, p), p)
        # the hall below and the second Guardian's row are neither
        for p in ((25709.0, 36.9, 3207.0), (25716.1, 54.3, 3222.8)):
            self.assertFalse(holds(d.INNER, p) or holds(d.OUTER, p), p)


if __name__ == "__main__":
    unittest.main()
