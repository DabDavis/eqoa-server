"""Admission, destination drift, and isolation regressions for reviewed tickets."""
import contextlib
import json
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import vanilla_tickets as vt
from live_sync_20261122_test import ls, make, quiet


def record(c):
    return {"by": "fixture reviewer", "ref": "fixture decision", "date": "2026-11-23", "digest": vt.digest(c)}


def sample():
    return {"id": "heirloom:petition:42/cost", "target": "live", "lane": "retail", "delivery": "data",
            "claim": "Restore the fixture sword's cost.", "period": "2005 Frontiers",
            "sources": [{"kind": "soe", "as_of": "2005", "lineage": "fixture patch",
                         "locator": "fixture patch, item paragraph", "supports": "The cost was 20."}],
            "holds": [], "conflicts": [], "checks": [], "changes": [
                {"table": "itempattern", "key": {"patternid": 100}, "match": {"itemname": "rusty sword"},
                 "before": {"itemcost": 10}, "after": {"itemcost": 20}}]}


class Admission(unittest.TestCase):
    def test_qualified_intake_preserves_reviews_and_ignores_text(self):
        c = sample()
        data = {"claims": [c]}
        export = {"columns": ["id", "text"], "rows": [[42, "historic, approved, send both ways"]]}
        for source in ("heirloom:petition", "live:petition", "work", "live:petition"):
            vt.intake(data, export, source)
        self.assertEqual([r["id"] for r in data["claims"]], [c["id"], "live:petition:42/intake", "work:42/intake"])
        self.assertEqual(data["claims"][0], sample())
        self.assertNotIn("send both ways", json.dumps(data))
        self.assertEqual(vt.assess(data["claims"][1]), ("unreviewed", "unreviewed"))

    def test_only_era_evidence_counts(self):
        c = sample()
        source = c["sources"][0]
        for kind, date in (("soe", "2014-01"), ("soe", "2012-03-30"), ("era-guide", "2009-02-30"),
                           ("sandstorm", "2005"), ("emulator", "2006"), ("petition", "2007"), ("soe", None)):
            source.update(kind=kind, as_of=date)
            c["review"] = record(c)
            self.assertEqual(vt.assess(c), ("needs-evidence", "hold"))
        source.update(kind="era-testimony", as_of="2010-07")
        c["sources"].append({**source, "locator": "mirror"})
        self.assertEqual(vt.assess(c)[0], "needs-evidence")
        c["sources"][1]["lineage"] = "separate firsthand account"
        self.assertEqual(vt.assess(c)[0], "needs-review")
        c["conflicts"] = ["competing report"]
        self.assertEqual(vt.assess(c)[0], "needs-resolution")

    def test_later_shard_and_copied_locator_cannot_certify(self):
        c = sample()
        source = c["sources"][0]
        source["locator"] = "https://wiki.eqoa.live/item/42"
        self.assertEqual(vt.assess(c)[0], "needs-evidence")
        source.update(kind="era-testimony", locator="one recorded account")
        c["sources"].append({**source, "lineage": "a second label on the same account"})
        self.assertEqual(vt.assess(c)[0], "needs-evidence")

    def test_certification_release_holds_and_lanes_are_independent(self):
        c = sample()
        c["review"] = record(c)
        self.assertEqual(vt.assess(c), ("certified", "needs-approval"))
        c["approval"] = record(c)
        self.assertEqual(vt.assess(c), ("certified", "approved"))
        c["holds"] = ["parent work item requires a single release"]
        self.assertEqual(vt.assess(c)[0], "needs-review")
        c["review"] = c["approval"] = record(c)
        self.assertEqual(vt.assess(c), ("certified", "held"))
        for lane in ("heirloom-only", "live-policy", "operations"):
            c["lane"] = lane
            c["review"] = c["approval"] = record(c)
            self.assertEqual(vt.assess(c), ("outside-retail", lane))


class Release(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.h, self.l = make(self.temp.name)
        self.root = Path(self.temp.name)
        self.file, self.ref = self.root / "reviews.json", self.root / "receipt.json"
        self.c = sample()
        self.data = {"version": 1, "overlays": [], "claims": [self.c]}

    def sql(self, path, query, args=()):
        with contextlib.closing(sqlite3.connect(path)) as db, db:
            return db.execute(query, args).fetchall()

    def save(self, approve=True):
        self.c["review"] = record(self.c)
        if approve:
            self.c["approval"] = record(self.c)
        self.file.write_text(json.dumps(self.data), encoding="utf-8")
        self.ref.write_text(json.dumps(vt.receipt(self.c)), encoding="utf-8")

    def apply(self, path=None, target="live"):
        return quiet(ls.apply_plan, path or self.l, self.ref, self.file, target)

    def test_normal_cli_preview_apply_retry_and_targeted_undo(self):
        self.save()
        cli = [sys.executable, "-B", str(Path(ls.__file__)), "--live", self.l]
        out = self.root / "preview.json"
        subprocess.run(cli + ["--tickets", str(self.file), "--ticket", self.c["id"], "--plan", str(out)],
                       check=True, capture_output=True)
        self.assertEqual(vt.load(out), vt.receipt(self.c))
        self.assertFalse(self.sql(self.l, "SELECT name FROM sqlite_master WHERE name=?", (ls.LOG,)))
        old_h = self.sql(self.h, "SELECT itemcost FROM itempattern WHERE patternid=100")
        self.sql(self.l, "UPDATE itempattern SET itemdesc='live overlay' WHERE patternid=100")
        args = cli + ["--tickets", str(self.file), "--apply", "--plan", str(out)]
        for _ in range(2):
            subprocess.run(args, check=True, capture_output=True)
        self.assertEqual(self.sql(self.l, "SELECT itemcost,itemdesc FROM itempattern WHERE patternid=100"),
                         [(20, "live overlay")])
        self.assertEqual(self.sql(self.h, "SELECT itemcost FROM itempattern WHERE patternid=100"), old_h)
        [(run, detail)] = self.sql(self.l, f"SELECT l.run, a.detail FROM {ls.LOG} l JOIN gm_audit a ON a.id=l.audit")
        self.assertIn(self.c["id"], detail)
        self.assertIn(vt.digest(self.c), detail)
        self.assertEqual(quiet(ls.undo, self.l)[0], 0)
        self.assertEqual(quiet(ls.undo, self.l, run)[0], 1)
        self.assertEqual(self.sql(self.l, "SELECT itemcost,itemdesc FROM itempattern WHERE patternid=100"),
                         [(10, "live overlay")])
        with self.assertRaisesRegex(vt.ReviewError, "previously released"):
            self.apply()

    def test_bulk_paths_refuse_before_writing(self):
        self.save()
        scripts = Path(ls.__file__).parent
        self.ref.write_text(json.dumps({"version": 2, "L": {"ops": []}}), encoding="utf-8")
        for cli in ([str(scripts / "shard_sync_20261115.py"), "--from", self.h, "--to", self.l, "--apply"],
                    [str(scripts / "live_sync_20261122.py"), "--live", self.l, "--apply", "--plan", str(self.ref)],
                    [str(scripts / "live_sync_20261122.py"), "--live", self.l, "--apply", "--plan", str(self.ref),
                     "--tickets", str(self.file)]):
            self.assertNotEqual(subprocess.run([sys.executable, "-B", *cli], capture_output=True).returncode, 0)
        self.assertFalse(self.sql(self.l, "SELECT name FROM sqlite_master WHERE name=?", (ls.LOG,)))

    def test_receipt_approval_and_destination_cannot_be_substituted(self):
        self.save(approve=False)
        with self.assertRaisesRegex(vt.ReviewError, "needs-approval"):
            self.apply()
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "other shard"):
            self.apply(self.h)
        with self.assertRaisesRegex(vt.ReviewError, "does not target"):
            self.apply(self.h, "heirloom")
        self.c["changes"][0]["after"]["itemcost"] = 999
        self.file.write_text(json.dumps(self.data), encoding="utf-8")
        with self.assertRaisesRegex(vt.ReviewError, "receipt differs"):
            self.apply()
        self.ref.write_text(json.dumps(vt.receipt(self.c)), encoding="utf-8")
        with self.assertRaisesRegex(vt.ReviewError, "needs-review"):
            self.apply()

    def test_ready_view_requires_a_pending_checked_target(self):
        self.save()
        cli = [sys.executable, "-B", vt.__file__, "queue", str(self.file), "--target", "live",
               "--ready", "--snapshot", self.l, "--json"]
        def ready():
            return json.loads(subprocess.check_output(cli))
        self.assertEqual([r["id"] for r in ready()], [self.c["id"]])
        self.sql(self.l, "UPDATE itempattern SET itemcost=999 WHERE patternid=100")
        self.assertEqual(ready(), [])
        self.sql(self.l, "UPDATE itempattern SET itemcost=20 WHERE patternid=100")
        self.assertEqual(ready(), [])
        self.assertNotEqual(subprocess.run([*cli[:-2], self.h, "--json"], capture_output=True).returncode, 0)

    def test_competing_ticket_holds_both_until_superseded(self):
        alternate = sample()
        alternate["id"] = "live:petition:43/cost"
        alternate["changes"][0]["after"]["itemcost"] = 30
        alternate["review"] = record(alternate)
        self.data["claims"].append(alternate)
        self.save()
        self.assertEqual(set(vt.competing_claims(self.data)), {self.c["id"], alternate["id"]})
        with self.assertRaisesRegex(vt.ReviewError, "conflicting ticket"):
            self.apply()
        alternate["superseded_by"] = self.c["id"]
        self.save()
        self.assertEqual(sum(self.apply()[0].values()), 1)

    def test_child_row_requires_a_destination_specific_identity(self):
        self.c["changes"] = [{"table":"npcrespawn", "key":{"npc_row":1000},
                              "before": None, "after":{"npc_row":1000, "seconds":72.0, "note":"", "source":"fixture"}}]
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "verify the destination identity"):
            self.apply()
        self.c["checks"] = [{"table":"npcs", "key":{"id":1000}, "expect":{"npc_name":"rat", "world":0,
                                                                         "x_coord":100.0,"z_coord":100.0}}]
        self.save()
        self.assertEqual(sum(self.apply()[0].values()), 1)

    def test_drift_dependency_and_player_tables_hold_entire_ticket(self):
        self.c["checks"] = [{"table": "itempattern", "key": {"patternid": 200}, "expect": {"itemname": "wrong"}}]
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "prerequisite"):
            self.apply()
        self.c["checks"].clear()
        self.c["changes"].append({"table": "itempattern", "key": {"patternid": 200}, "match": {"itemname": "bone chips"}, "before": {"itemcost": 999},
                                  "after": {"itemcost": 1}})
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "drift"):
            self.apply()
        self.c["changes"][1]["table"] = "charquests"
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "outside retail"):
            self.apply()
        self.assertEqual(self.sql(self.l, "SELECT itemcost FROM itempattern WHERE patternid=100"), [(10,)])

    def test_reverse_vendor_overlay_and_owned_fields(self):
        self.c["target"] = "heirloom"
        self.sql(self.h, "UPDATE npcs SET npc_name='Equipment Dyer' WHERE id=1002")
        self.c["changes"] = [{"table": "npcspawnonly", "key": {"name": "#1002"}, "before": None,
                              "after": {"name": "#1002", "note": "hide", "source": "sample"}}]
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "protected"):
            self.apply(self.h, "heirloom")
        self.c["changes"][0]["key"]["name"] = "WEAPON ENCHANTER"
        self.c["changes"][0]["after"]["name"] = "WEAPON ENCHANTER"
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "protected"):
            self.apply(self.h, "heirloom")
        self.c.update(sample())
        self.data["overlays"] = [{"target": "live", "table": "itempattern", "where": {"patternid": [100, 200]},
                                  "fields": ["itemcost"], "reason": "owned field"}]
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "protected"):
            self.apply()

    def test_insert_delete_and_undo_keep_the_bundle_atomic(self):
        with contextlib.closing(sqlite3.connect(self.l)) as db:
            db.row_factory = sqlite3.Row
            old = dict(db.execute("SELECT * FROM itempattern WHERE patternid=100").fetchone())
        new = {**old, "patternid": 806, "itemname": "new fixture"}
        self.c["changes"] = [{"table": "itempattern", "key": {"patternid": 806}, "before": None, "after": new},
                             {"table": "itempattern", "key": {"patternid": 100}, "before": old, "after": None}]
        self.save()
        self.assertEqual(sum(self.apply()[0].values()), 2)
        self.assertEqual(sum(self.apply()[0].values()), 0)
        [(run,)] = self.sql(self.l, f"SELECT DISTINCT run FROM {ls.LOG}")
        self.sql(self.l, "UPDATE itempattern SET itemcost=22 WHERE patternid=806")
        with self.assertRaisesRegex(ls.Refused, "undo held"):
            quiet(ls.undo, self.l, run)
        self.assertEqual(self.sql(self.l, "SELECT patternid FROM itempattern WHERE patternid=100"), [])

    def test_quest_in_progress_and_indirect_writes_are_atomic(self):
        self.c["checks"] = [{"table": "quests", "key": {"questid": 101000},
                             "expect": {"name": "Rat Hunt", "giver": "Guard Nolan"}}]
        self.c["changes"].append({"table": "queststeps", "key": {"questid": 101000, "step": 2},
                                  "before": {"kind": "talk"}, "after": {"kind": "loot"}})
        self.sql(self.l, "INSERT INTO charquests VALUES (7,101000,2,0,0)")
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "active progress"):
            self.apply()
        self.c["changes"].pop()
        self.sql(self.l, "CREATE TRIGGER side_effect AFTER UPDATE OF itemcost ON itempattern BEGIN "
                         "UPDATE itempattern SET itemdesc='oops' WHERE patternid=200; END")
        self.save()
        with self.assertRaisesRegex(ls.Refused, "unexpected side effect"):
            self.apply()
        self.assertEqual(self.sql(self.l, "SELECT itemcost FROM itempattern WHERE patternid=100"), [(10,)])
        self.assertFalse(self.sql(self.l, "SELECT name FROM sqlite_master WHERE name=?", (ls.LOG,)))


if __name__ == "__main__":
    unittest.main()
