#!/usr/bin/env python3
"""Tests of jib_requests_20261121.py on copies of the two shards' databases (JIB_HEIRLOOM, JIB_LIVE: pre-swap backups
with guide_camps_20261121 --skip ratmen applied, as the box was at 07:41 UTC 2026-11-21); skipped without them.

    JIB_HEIRLOOM=heirloom.db JIB_LIVE=live.db python3 -m unittest tools/jib_requests_20261121_test.py
"""
import contextlib
import hashlib
import io
import os
import shutil
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import jib_requests_20261121 as jr  # noqa: E402

HEIRLOOM = os.environ.get("JIB_HEIRLOOM", "")
LIVE = os.environ.get("JIB_LIVE", "")
TABLES = ("petstats", "spellpattern", "spelleffect", "cmcatalog", "itempattern", "quests", "queststeps",
          "queststeptargets", "npcloot", "charquests", "charinventory")


def digest(path):
    db = sqlite3.connect(path)
    out = {}
    for t in TABLES:
        h = hashlib.sha256()
        for r in db.execute(f"SELECT rowid, * FROM {t} ORDER BY rowid"):
            h.update(repr(r).encode())
        out[t] = h.hexdigest()
    db.close()
    return out


def run(*argv):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        rc = jr.main(list(argv))
    return rc, buf.getvalue()


@unittest.skipUnless(HEIRLOOM and LIVE and os.path.exists(HEIRLOOM) and os.path.exists(LIVE),
                     "JIB_HEIRLOOM and JIB_LIVE name copies of the shards' databases")
class JibRequests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp(dir=os.path.dirname(os.path.abspath(LIVE)))
        self.live = os.path.join(self.dir, "live.db")
        shutil.copy(LIVE, self.live)

    def tearDown(self):
        shutil.rmtree(self.dir)

    def heirloom(self):
        p = os.path.join(self.dir, "heirloom.db")
        shutil.copy(HEIRLOOM, p)
        return p

    def sql(self, path, *stmts):
        db = sqlite3.connect(path)
        for s in stmts:
            db.execute(*s) if isinstance(s, tuple) else db.execute(s)
        db.commit()
        db.close()

    def test_live_apply_again_undo(self):
        chars = [r[0] for r in sqlite3.connect(self.live).execute(
            "SELECT serverid FROM characters ORDER BY serverid LIMIT 5")]
        moves = [(chars[0], 100074, 2, 1), (chars[1], 100078, 5, 0), (chars[2], 100073, 2, 2),
                 (chars[3], 100090, 5, 0), (chars[4], 100079, 8, 0)]
        self.sql(self.live, *[("INSERT OR REPLACE INTO charquests (serverid, questid, step, count, done) "
                               "VALUES (?, ?, ?, ?, 0)", m) for m in moves])
        self.sql(self.live, ("DELETE FROM charinventory WHERE serverid = ? AND patternid = 8357", (chars[1],)))
        before = digest(self.live)
        rc, out = run("--db", self.live, "--shard", "live", "--source", HEIRLOOM, "--apply")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(self.live)
        got = {(s, q): (st, c) for s, q, st, c in db.execute(
            "SELECT serverid, questid, step, count FROM charquests WHERE serverid IN (?, ?, ?, ?, ?)", chars)}
        self.assertEqual(got[(chars[0], 100074)], (1, 1))   # loot worker ant -> loot worker ant: count kept
        self.assertEqual(got[(chars[1], 100078)], (4, 0))
        self.assertEqual(got[(chars[2], 100073)], (2, 0))   # spiderlings now drop 700279
        self.assertEqual(got[(chars[3], 100090)], (6, 0))
        self.assertEqual(got[(chars[4], 100079)], (7, 0))
        self.assertEqual(db.execute("SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = 8357",
                                    (chars[1],)).fetchone()[0], 1)
        self.assertEqual(db.execute("SELECT name, model, level FROM petstats WHERE spellid = 906301").fetchone(),
                         ("Red Swarming Death", 0x6E5E0E2A, 54))
        self.assertEqual(db.execute("SELECT count(*) FROM npcloot WHERE source = ? AND npc_name = 'bandit'",
                                    (jr.LOOT_SRC,)).fetchone()[0], 0)
        self.assertEqual(db.execute("SELECT count(*) FROM npcloot WHERE source = ? AND npc_name = '#192123'",
                                    (jr.LOOT_SRC,)).fetchone()[0], 2)
        self.assertIn("981971", db.execute("SELECT choices FROM queststeps WHERE questid = 100080 AND step = 9").fetchone()[0])
        n = db.execute("SELECT count(*) FROM gm_audit WHERE detail LIKE 'jibrish''s request (board #%'").fetchone()[0]
        db.close()
        self.assertEqual(n, sum(1 for _ in sqlite3.connect(self.live).execute(f"SELECT 1 FROM {jr.LOG}")))
        rc, out = run("--db", self.live, "--shard", "live", "--source", HEIRLOOM, "--apply")
        self.assertEqual(rc, 0, out)
        self.assertIn("nothing to write", out)
        rc, out = run("--db", self.live, "--shard", "live", "--undo")
        self.assertEqual(rc, 0, out)
        self.assertEqual(digest(self.live), before)

    def test_heirloom_200_is_jibrishs_sql(self):
        h = self.heirloom()
        rc, out = run("--db", h, "--shard", "heirloom", "--apply")
        self.assertEqual(rc, 0, out)
        q = ("SELECT npc_name, tablename, chance, rolls, note, source FROM npcloot WHERE source = 'guide-loot-20261121' "
             "AND (npc_name LIKE '#%' OR npc_name IN (" + ",".join("?" * len(jr.LOOT_NAMES)) + "))")
        mine = set(sqlite3.connect(h).execute(q, jr.LOOT_NAMES).fetchall())
        rc, out = run("--db", h, "--shard", "heirloom", "--undo")
        self.assertEqual(rc, 0, out)
        db = sqlite3.connect(h, isolation_level=None)
        db.execute("BEGIN")
        post = jr.SPAWNS["heirloom"]["keep"]
        db.execute("CREATE TEMP TABLE k(id INTEGER PRIMARY KEY, name TEXT)")
        db.executemany("INSERT INTO k VALUES (?, ?)", [(i, n) for n, ids in post.items() for i in ids])
        db.execute("INSERT OR IGNORE INTO npcloot (npc_name, tablename, chance, rolls, note, source) SELECT '#' || k.id, "
                   "l.tablename, l.chance, l.rolls, l.note, l.source FROM npcloot l JOIN k ON k.name = l.npc_name "
                   "WHERE l.source = 'guide-loot-20261121'")
        db.execute("DELETE FROM npcloot WHERE source = 'guide-loot-20261121' AND npc_name IN ("
                   + ",".join("?" * len(jr.LOOT_NAMES)) + ")", jr.LOOT_NAMES)
        theirs = set(db.execute(q, jr.LOOT_NAMES).fetchall())
        db.execute("ROLLBACK")
        self.assertEqual(sum(len(v) for v in post.values()), 210)
        self.assertEqual(mine, theirs)

    def refused(self, *argv):
        before = digest(self.live)
        rc, out = run("--db", self.live, "--shard", "live", "--apply", *argv)
        self.assertEqual(rc, 1, out)
        self.assertIn("REFUSED", out)
        self.assertEqual(digest(self.live), before)
        return out

    def test_refuses_a_quest_not_as_heirloom_had_it(self):
        self.sql(self.live, "UPDATE quests SET offer = offer || '!' WHERE questid = 100093")
        self.assertIn("quest 100093", self.refused("--part", "199", "--source", HEIRLOOM))

    def test_refuses_the_new_items_id_taken(self):
        db = sqlite3.connect(self.live)
        cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)") if r[1] not in ("patternid", "itemname")]
        db.execute(f"INSERT INTO itempattern (patternid, itemname, {', '.join(chr(34) + c + chr(34) for c in cols)}) "
                   f"SELECT 991500, 'something else', {', '.join(chr(34) + c + chr(34) for c in cols)} "
                   "FROM itempattern WHERE patternid = 8357")
        db.commit()
        db.close()
        self.assertIn("991500 is taken", self.refused("--part", "199", "--source", HEIRLOOM))

    def test_refuses_a_ware_not_sold(self):
        self.sql(self.live, "DELETE FROM npcitem WHERE patternid = 8357 AND npcid IN "
                            "(SELECT id FROM npcs WHERE npc_name = 'Merchant Lorili Ashgo')")
        self.assertIn("does not sell 8357", self.refused("--part", "199", "--source", HEIRLOOM))

    def test_refuses_a_pet_column_neither(self):
        self.sql(self.live, "UPDATE petstats SET model = 12345 WHERE spellid = 700823")
        self.assertIn("model is 12345", self.refused("--part", "196"))

    def test_refuses_a_spawn_off_both_lists(self):
        self.sql(self.live, "INSERT INTO npcs (id, npc_name, npc_level, world) VALUES (199999, 'bandit', 25, 0)")
        self.assertIn("199999", self.refused("--part", "200"))


if __name__ == "__main__":
    unittest.main()
