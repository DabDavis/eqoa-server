#!/usr/bin/env python3
"""fix_innoruuk_visuals_20261110.py on the raid gods' tool's test database, after that tool's --apply: the report writes
nothing; --apply gives Innoruuk's Largesse of Plague proc the disease hit's look and pins Veil of Death + on his veil
(no look of its own), and Mr. Moogie his line at Innoruuk's death, each with its gm_audit line; a second apply is
refused; --undo puts both back exactly. The controls: before the raid gods' tool (his abilities not as it wrote them)
it refuses, writing nothing; with no Innoruuk template it is another shard and does nothing."""
import json
import os
import sqlite3
import subprocess
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fix_innoruuk_visuals_20261110 as fv  # noqa: E402
import fix_moogie_tunare_20261110 as fm  # noqa: E402
import fix_moogie_tunare_20261110_test as base  # noqa: E402

TOOL = os.path.join(HERE, "fix_innoruuk_visuals_20261110.py")


class FixInnoruukVisualsTest(unittest.TestCase):
    def setUp(self):
        self.b = base.FixMoogieTunareTest("test_report_apply_and_undo")
        self.b.setUp()
        self.path = self.b.path
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, "
                   "eqprequire, spellname, spelldesc) VALUES (1431, 12, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 'Veil of Death +', '')")
        db.execute("INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (1431, 'Self, Proc, 30min', -205483512, 'test')")
        db.close()

    def run_tool(self, tool, *args):
        p = subprocess.run([sys.executable, tool, "--db", self.path, *args], capture_output=True, text=True)
        return p.returncode, p.stdout + p.stderr

    def abilities(self, name):
        db = sqlite3.connect(self.path)
        try:
            return json.loads(db.execute("SELECT abilities FROM classtemplates WHERE name = ?", (name,)).fetchone()[0])
        finally:
            db.close()

    def test_apply_and_undo(self):
        code, out = self.run_tool(TOOL, "--apply")
        self.assertNotEqual(code, 0, "before the raid gods' tool it wrote")
        self.assertIn("refused, nothing done", out)
        self.assertEqual(self.run_tool(base.TOOL, "--apply")[0], 0)
        before = base.dump(sqlite3.connect(self.path))
        code, out = self.run_tool(TOOL)
        self.assertEqual(code, 0, out)
        self.assertIn("report only: 2 rows", out)
        self.assertIn("Veil of Death + (1431): its line 'Self, Proc, 30min', its visual 0xF3C09208", out)
        self.assertEqual(base.dump(sqlite3.connect(self.path)), before, "the report wrote")
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        kit = self.abilities("innoruuk")
        self.assertEqual(fm.norm(kit[-2:]), fm.norm([fv.LARGESSE_NEW, fv.VEIL_NEW]))
        self.assertEqual([a["kind"] for a in kit[:5]], ["meleekind", "meleekind", "dot", "nuke", "lifetap"], "the rest of his kit kept")
        moogie = self.abilities("mr moogie")
        self.assertEqual(fm.norm(moogie), fm.norm(fm.MOOGIE_NEW + [fv.KILLSAY]))
        db = sqlite3.connect(self.path)
        self.assertEqual(db.execute("SELECT count(*) FROM gm_audit WHERE detail = ?", (fv.DETAIL,)).fetchone()[0], 2)
        db.close()
        code, out = self.run_tool(TOOL, "--apply")
        self.assertNotEqual(code, 0)
        self.assertIn("applied already", out)
        code, out = self.run_tool(TOOL, "--undo")
        self.assertEqual(code, 0, out)
        self.assertEqual(base.dump(sqlite3.connect(self.path)), before, "the undo is not exact")
        self.assertEqual(self.run_tool(base.TOOL, "--undo")[0], 0, "the raid gods' tool's undo after it")

    def test_not_his_shard(self):
        db = sqlite3.connect(self.path, isolation_level=None)
        db.execute("DELETE FROM classtemplates WHERE name = 'innoruuk'")
        db.close()
        code, out = self.run_tool(TOOL, "--apply")
        self.assertEqual(code, 0, out)
        self.assertIn("not his shard", out)


if __name__ == "__main__":
    unittest.main()
