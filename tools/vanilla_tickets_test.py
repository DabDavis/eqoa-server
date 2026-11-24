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


class TransferShortlist(unittest.TestCase):
    def setUp(self):
        self.claim = sample()
        self.parent = {"questid": 11, "name": "Silver and Fur", "giver": "Djivan"}
        self.old = {"kind": "kill", "target": "Lycanthrope Stevenson"}
        self.new = {"kind": "kill", "target": "Lycanthrope Svenson"}
        key = {"questid": 11, "step": 2}
        self.claim["changes"] = [{"table": "queststeps", "key": key, "match": {"kind": "kill"},
                                  "before": {"target": self.old["target"]}, "after": {"target": self.new["target"]}}]
        self.t = self.claim["transfer"] = {
            "case": "C06", "subject": "Silver and Fur", "basis": "named period source", "limits": "kill target",
            "checked_utc": "2026-11-24T01:49:33Z", "builds": {"heirloom": "h", "live": "l"},
            "features": ["silver-and-fur-slick"],
            "quest_parents": [{"live": dict(self.parent), "heirloom": dict(self.parent)}],
            "live_dependencies": [{"table": "quests", "key": {"questid": 11}, "values": dict(self.parent)}],
            "rows": [{"table": "queststeps", "key": key, "live": self.old, "heirloom": self.new}]}

    def candidates(self):
        self.claim["review"] = record(self.claim)
        return vt.transfer_shortlist({"claims": [self.claim]})

    def test_no_release_approval_needed_to_filter_and_noop_and_drift_drop_out(self):
        self.assertEqual(len(self.candidates()), 1)
        self.assertNotIn("approval", self.claim)
        self.old["target"] = "Lycanthrope Svenson"
        self.assertEqual(self.candidates(), [])
        self.old["target"] = "unexpected actor"
        self.assertEqual(self.candidates(), [])

    def test_hold_unknown_scope_and_heirloom_dependency_are_excluded(self):
        self.claim["holds"] = ["active progress"]
        self.assertEqual(self.candidates(), [])
        self.claim["holds"] = []
        for features in ([], ["unknown-feature"], ["silver-and-fur-slick", "mr-moogie"]):
            self.t["features"] = features
            self.assertEqual(self.candidates(), [])
        self.t["features"] = ["silver-and-fur-slick"]
        self.t["live_dependencies"][0]["values"]["steps"] = '[{"do":"sic","target":"Mr. Moogie"}]'
        self.assertEqual(self.candidates(), [])

    def test_reused_parent_id_does_not_establish_shared_content(self):
        self.t["quest_parents"][0]["heirloom"]["name"] = "Poison Stingers"
        self.assertEqual(self.candidates(), [])

    def test_actor_numbers_must_map_to_the_reviewed_live_actor(self):
        source = '[{"do":"spawn","npc":"#12","count":3}]'
        destination = '[{"do":"spawn","npc":"#88","count":3}]'
        self.assertTrue(vt.transfer_value("events", source, destination, {"#12": "#88"}))
        self.assertFalse(vt.transfer_value("events", source, source, {}))
        self.assertFalse(vt.transfer_value("events", source, source, {"#12": "#88"}))
        self.assertFalse(vt.transfer_value("events", source, destination.replace(":3", ":2"), {"#12": "#88"}))

    def test_shared_reward_cannot_carry_heirloom_race_mask(self):
        self.claim["changes"] = [{"table": "itempattern", "key": {"patternid": 42},
                                  "match": {"itemname": "Light Cloth Pants"},
                                  "before": {"raceuse": 256}, "after": {"raceuse": 1023}}]
        self.t["features"] = ["highbourne-cleric-pants"]
        self.t["rows"] = [{"table": "itempattern", "key": {"patternid": 42},
                           "live": {"itemname": "Light Cloth Pants", "raceuse": 256},
                           "heirloom": {"itemname": "Light Cloth Pants", "raceuse": 1023}}]
        self.assertEqual(self.candidates(), [])


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

    def test_bound_path_copy_label_and_stray_heirloom_rows(self):
        self.sql(self.l, "INSERT INTO heirlooms VALUES (91234)")
        self.save()
        self.assertEqual(sum(self.apply()[0].values()), 1)
        cfg = self.root / "config.yaml"
        cfg.write_text('db_path: "l.db" # deployment\ngm:\n  shard_id: "live"\n', encoding="utf-8")
        self.sql(self.l, "DROP TABLE shard_label")
        with contextlib.closing(sqlite3.connect(self.l)) as db:
            self.assertEqual(vt.bound_shard(db, cfg), "live")
        with contextlib.closing(sqlite3.connect(self.h)) as db:
            with self.assertRaisesRegex(vt.ReviewError, "differs"):
                vt.bound_shard(db, cfg)
        # The deployed layout: db_path under server:, shard_id under gm:, other keys between.
        cfg.write_text('server:\n  game_port: 10071\n  db_path: "l.db"\n  nested:\n    db_path: "h.db"\n'
                       'gm:\n  port: 8081\n  shard_id: "live"  # comment\n', encoding="utf-8")
        with contextlib.closing(sqlite3.connect(self.l)) as db:
            self.assertEqual(vt.bound_shard(db, cfg), "live")
        cfg.write_text('server:\n  db_path: "l.db"\ndb_path: "h.db"\ngm:\n  shard_id: "live"\n', encoding="utf-8")
        with self.assertRaisesRegex(vt.ReviewError, "repeated db_path"):
            vt.config_binding(cfg)
        cfg.unlink()
        with contextlib.closing(sqlite3.connect(self.l)) as db:
            with self.assertRaisesRegex(vt.ReviewError, "unbound"):
                vt.bound_shard(db)

    def test_run_selection_in_both_commands_and_legacy_log(self):
        for tool, args in ((ls.__file__, ["--live", self.l]), (ls.ss.__file__, ["--to", self.l])):
            p = subprocess.run([sys.executable, "-B", tool, *args, "--shard", "live", "--undo"],
                               capture_output=True, text=True)
            self.assertNotEqual(p.returncode, 0)
            self.assertIn("--run", p.stderr)
        self.sql(self.l, ls.ss.LOG_DDL)
        row = {"patternid": 100, "itemname": "rusty sword", "equipslot": -1, "levelreq": 0,
               "itemcost": 10, "itemdesc": ""}
        cols = list(row)
        row = dict(zip(cols, self.sql(self.l, "SELECT " + ",".join(cols) + " FROM itempattern WHERE patternid=100")[0]))
        for run in ("old-a", "old-b"):
            self.sql(self.l, f"INSERT INTO {ls.ss.LOG} (run,tbl,pk,before,after,audit) VALUES (?,?,?,?,?,?)",
                     (run, "itempattern", '{"patternid":100}', json.dumps({**row, "itemcost": 5}), json.dumps(row), 0))
        quiet(ls.ss.undo, self.l, "old-a", "live")
        self.assertEqual(self.sql(self.l, f"SELECT run,undone FROM {ls.ss.LOG} ORDER BY run"), [("old-a", 1), ("old-b", 0)])
        bad = subprocess.run([sys.executable, "-B", ls.ss.__file__, "--to", self.l, "--shard", "live",
                              "--undo", "--run", "missing"], capture_output=True, text=True)
        self.assertNotEqual(bad.returncode, 0)
        self.assertTrue(bad.stderr.startswith("REFUSED: unknown run"))

    def test_directional_race_policy_covers_unlisted_items_and_quest_lists(self):
        for target, table, column, before, after in (
                ("heirloom", "itempattern", "raceuse", 1023, 4), ("live", "itempattern", "raceuse", 4, 1023),
                ("heirloom", "quests", "races", "", "2"), ("live", "quests", "races", "2,4", ""),
                ("live", "quests", "races", "2,4", "2,5")):
            with self.subTest(target=target, table=table, before=before, after=after):
                with self.assertRaisesRegex(vt.ReviewError, "protected race"):
                    vt.check_races(target, table, {column: before}, {column: after}, {column})
        self.sql(self.l, "ALTER TABLE itempattern ADD COLUMN raceuse INTEGER DEFAULT 4")
        self.c["changes"][0].update(before={"raceuse": 4}, after={"raceuse": 1023})
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "protected race"):
            self.apply()
        vt.check_races("live", "quests", {"races": ""}, {"races": "2"}, {"races"})
        vt.check_races("heirloom", "itempattern", {"raceuse": 4}, {"raceuse": 1023}, {"raceuse"})

    def test_embedded_ids_require_destination_identities_and_ignore_counts_and_dialog(self):
        key = {"questid": 101000, "step": 1}
        name, giver = self.sql(self.l, "SELECT name,giver FROM quests WHERE questid=101000")[0]
        self.c["checks"] = [{"table": "quests", "key": {"questid": 101000}, "expect": {"name": name, "giver": giver}}]
        self.sql(self.l, "ALTER TABLE queststeps ADD COLUMN rewarditems TEXT DEFAULT ''")
        self.c["changes"] = [{"table": "queststeps", "key": key, "before": {"rewarditems": ""},
                              "after": {"rewarditems": "777777*2"}}]
        self.save()
        with self.assertRaisesRegex(vt.ReviewError, "itempattern 777777"):
            self.apply()
        events = json.dumps([{"do": "give", "items": "100*7,200×2"}, {"do": "cast", "spell": 55},
                             {"do": "spawn", "npc": "#80", "count": 12}, {"do": "say", "text": "300 silver"}])
        self.assertEqual(list(vt.embedded_refs("events", "itext", events)),
                         [("item", 100), ("item", 200), ("spell", 55), ("npc", 80)])
        self.assertEqual(list(vt.embedded_refs("requires", "qtext", "11|12,13")), [("quest", 11), ("quest", 12), ("quest", 13)])
        options = '{"options":[{"items":"100*2","spells":"51,52","cost":1200}]}'
        self.assertEqual(list(vt.embedded_refs("choices", "itext", options)), [("item", 100), ("spell", 51), ("spell", 52)])
        self.assertEqual(list(vt.embedded_refs("ingredients", "itext", '[{"item":100,"count":12,"keep":true}]')),
                         [("item", 100)])
        abilities = '[{"kind":"cast","spell":"Fire","every":20},{"kind":"cast","spell_id":51,"every":30}]'
        self.assertEqual(list(vt.embedded_refs("abilities", "stext", abilities)), [("spell", 51)])

    def test_same_ticket_npc_and_child_resolve_and_protected_vendors_do_not_cross(self):
        with contextlib.closing(sqlite3.connect(self.l)) as db:
            db.row_factory = sqlite3.Row
            npc = dict(db.execute("SELECT * FROM npcs WHERE id=1000").fetchone())
        npc.update(id=81234, npc_name="New actor")
        child = {"npc_row": 81234, "seconds": 16.0, "note": "", "source": "gm"}
        self.c["changes"] = [{"table": "npcrespawn", "key": {"npc_row": 81234}, "before": None, "after": child},
                             {"table": "npcs", "key": {"id": 81234}, "before": None, "after": npc}]
        for protected in ("Equipment Dyer", "Enraged Gertrude", "Spiritbinder Pluto"):
            npc["npc_name"] = protected
            self.save()
            with self.assertRaisesRegex(vt.ReviewError, "protected"):
                self.apply()
        npc["npc_name"] = "Gertrude"  # the original bear and the seasonal actor are distinct
        self.save()
        self.assertEqual(sum(self.apply()[0].values()), 2)
        self.assertEqual(self.sql(self.l, "SELECT seconds FROM npcrespawn WHERE npc_row=81234"), [(16.0,)])

    def test_holiday_and_indirect_vendor_guards(self):
        rules = vt.overlay_rules(self.data)
        with contextlib.closing(sqlite3.connect(self.l)) as db:
            for table, col in (("npctitle", "npcid"), ("npcstands", "npc_row"), ("npcwander", "npcid"),
                               ("npcpaths", "npcid"), ("campspawns", "npcid"), ("gmportals", "exit_npc_id")):
                rule = next(r for r in rules if r["table"] == table and r.get("npc_from") == col and r["target"] == "both")
                self.assertTrue(vt.matches_overlay(db, {col: 123}, rule, {123: "Equipment Dyer"}))
            for ident in (102908, 109910, 109911, 109912):
                self.assertTrue(any(vt.matches_overlay(db, {"questid": ident}, r, {}) for r in rules
                                    if r["table"] == "queststeps" and r["target"] == "live"))

    def test_shared_and_owner_basis_stay_outside_historical_certification(self):
        self.c.update(lane="shared-fix", basis="shared", basis_ref="work:2123/test-grant-id")
        self.c.pop("sources")
        self.save()
        self.assertEqual(vt.assess(self.c), ("reviewed-shared", "approved"))
        self.assertEqual(sum(self.apply()[0].values()), 1)
        self.assertEqual(self.sql(self.l, f"SELECT basis,basis_ref FROM {vt.RELEASES}"),
                         [("shared", "work:2123/test-grant-id")])
        self.c.update(lane="live-policy", basis="owner")
        self.save()
        self.assertEqual(vt.assess(self.c), ("reviewed-policy", "approved"))
        self.c["lane"] = "retail"
        self.save()
        self.assertEqual(vt.assess(self.c), ("needs-basis", "hold"))

    def test_basis_links_and_audit_detail_normalization(self):
        row = {"id": 9, "basis": "shared", "basis_ref": "regression in the grant test", "notes": "private text"}
        data = {"claims": []}
        vt.intake(data, [row], "work")
        self.assertEqual(data["claims"][0]["basis_ref"], "board 9")
        self.assertNotIn("private text", json.dumps(data))
        self.sql(self.l, "CREATE TABLE workboard(id INTEGER PRIMARY KEY,basis TEXT,basis_ref TEXT,version INTEGER)")
        self.sql(self.l, "INSERT INTO workboard VALUES(9,'shared','grant ID test',4)")
        self.c.update(lane="shared-fix", basis="shared", basis_ref="board 9 v4")
        self.save()
        self.sql(self.l, "UPDATE workboard SET basis='estimate',version=5 WHERE id=9")
        with self.assertRaisesRegex(vt.ReviewError, "board basis"):
            self.apply()
        self.assertEqual(ls.ss._detnorm("[basis:owner work 100] Live's grant 990"),
                         ls.ss._detnorm("[basis:shared] Heirloom's grant 991"))
        self.assertEqual(ls.ss._detnorm("[basis:era Prima p. 50] grant 2"), ls.ss._detnorm("grant 3"))

    def test_compact_release_refuses_undo_when_the_canonical_image_is_missing(self):
        self.save()
        self.apply()
        [(run, aid)] = self.sql(self.l, f"SELECT run,audit FROM {ls.LOG}")
        self.sql(self.l, "DELETE FROM gm_audit WHERE id=?", (aid,))
        with self.assertRaisesRegex(vt.ReviewError, "missing audit image"):
            quiet(ls.undo, self.l, run, "live")
        self.assertEqual(self.sql(self.l, "SELECT itemcost FROM itempattern WHERE patternid=100"), [(20,)])

    def test_normal_cli_preview_apply_retry_and_targeted_undo(self):
        self.save()
        cli = [sys.executable, "-B", str(Path(ls.__file__)), "--shard", "live", "--live", self.l]
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
        self.assertEqual(self.sql(self.l, f"SELECT digest,basis FROM {vt.RELEASES} WHERE run=?", (run,)),
                         [(vt.digest(self.c), "era")])
        self.assertEqual(self.sql(self.l, f"SELECT before,after FROM {ls.LOG} WHERE run=?", (run,)), [(None, None)])
        with self.assertRaisesRegex(vt.ReviewError, "--run"):
            quiet(ls.undo, self.l)
        self.assertEqual(quiet(ls.undo, self.l, run, "live")[0], 1)
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
            quiet(ls.undo, self.l, run, "live")
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
