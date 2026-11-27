#!/usr/bin/env python3
"""shard_refs.py and the sync engine's reference translation (2026-11-27, Heirloom petition 3352).

Two small shards where the same spawn templates were minted under different ids on each (Heirloom 163912, Live 162757:
the same froglok watchman, as tools/guk_herbalist_20261111.py made them on each shard), Live edited the trigger that
spawns it, a quest step's events spawn a Rustel Minotaur (Heirloom 190359, Live 191858), the trigger's stop step names the
Trial of Celerity (Live slot 150, Heirloom slot 237; Heirloom's slot 150 is another trigger), and a Live-only trigger
spawns a row Live itself no longer has.
  - the engine as it was (git ea028d0f) copies Live's "#162757", "#191858" and slot 150 to Heirloom verbatim: the
    dangling references of petition 3352 (and a stop of the wrong trigger), and the Live-only trigger's "#162999" too;
  - the engine now writes Heirloom's own rows (#163912, #190359, slot 237) and leaves the Live-only trigger out, listed
    in the plan's "unresolved";
  - shard_refs.py reads, rewrites and normalises references by key only, and --check-refs finds a planted miss.
make_pair() is the fixture apply_sync_refs_test.py (beside apply_sync.py) uses too.
"""
import contextlib
import importlib.util
import io
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import shard_refs as refs  # noqa: E402

OLD = "ea028d0f"   # main before the fix

SCHEMA = """
CREATE TABLE accountinfo (accountid INTEGER PRIMARY KEY, username TEXT);
CREATE TABLE itempattern (patternid INTEGER PRIMARY KEY, itemname TEXT, equipslot INTEGER, levelreq INTEGER, itemcost INTEGER,
    itemdesc TEXT);
CREATE TABLE spellpattern (spellid INTEGER PRIMARY KEY, spellname TEXT, abilitylvl INTEGER);
CREATE TABLE quests (questid INTEGER PRIMARY KEY, source TEXT, name TEXT, giver TEXT, races TEXT, enabled INTEGER,
    events TEXT NOT NULL DEFAULT '');
CREATE TABLE queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT, items TEXT, journal TEXT, npcid INTEGER,
    events TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step));
CREATE TABLE queststeptargets (questid INTEGER, step INTEGER, target TEXT, PRIMARY KEY (questid, step, target));
CREATE TABLE charquests (serverid INTEGER, questid INTEGER, step INTEGER, count INTEGER, done INTEGER,
    PRIMARY KEY (serverid, questid));
CREATE TABLE heirlooms (patternid INTEGER PRIMARY KEY);
CREATE TABLE itemimported (patternid INTEGER PRIMARY KEY, base INTEGER);
CREATE TABLE familiars (patternid INTEGER PRIMARY KEY);
CREATE TABLE heroes_stock (id INTEGER PRIMARY KEY, patternid INTEGER);
CREATE TABLE npcs (id INTEGER PRIMARY KEY AUTOINCREMENT, npc_name TEXT, world INTEGER, x_coord REAL, y_coord REAL,
    z_coord REAL, facing INTEGER, npc_level INTEGER, modelid INTEGER, dt_kill DATE, hp_percent INTEGER, npc_id INTEGER);
CREATE TABLE npcrespawn (npc_row INTEGER PRIMARY KEY, seconds REAL NOT NULL, note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm');
CREATE TABLE npcpets (owner TEXT NOT NULL, pet TEXT NOT NULL, template_row INTEGER NOT NULL, count INTEGER NOT NULL DEFAULT 1,
    level_offset INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (owner, template_row));
CREATE TABLE gmtriggers (slot INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
    scope TEXT NOT NULL DEFAULT 'near', radius REAL NOT NULL DEFAULT 0, world INTEGER NOT NULL DEFAULT -1,
    x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, steps TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '');
CREATE TABLE npcloot (npc_name TEXT, tablename TEXT, chance REAL, rolls INTEGER, note TEXT, source TEXT,
    PRIMARY KEY (npc_name, tablename));
CREATE TABLE loottables (id INTEGER PRIMARY KEY AUTOINCREMENT, tablename TEXT, patternid INTEGER, weight INTEGER,
    minlevel INTEGER, maxlevel INTEGER, source TEXT);
CREATE TABLE npcfaction (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, world INTEGER, x REAL, z REAL, radius REAL,
    faction TEXT, note TEXT, source TEXT);
CREATE TABLE npcaggro (npc_name TEXT PRIMARY KEY, aggressive INTEGER, radius REAL, note TEXT, source TEXT);
CREATE TABLE gm_audit (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, accountid INTEGER NOT NULL,
    username TEXT NOT NULL, via TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, tbl TEXT NOT NULL DEFAULT '',
    rowkey TEXT NOT NULL DEFAULT '', before TEXT, after TEXT, detail TEXT NOT NULL DEFAULT '', reverted_by INTEGER);
INSERT INTO accountinfo VALUES (1, 'dab');
INSERT INTO itempattern VALUES (100, 'rusty sword', 12, 1, 10, ''), (700720, 'herbalist brew', -1, 0, 1, '');
INSERT INTO quests VALUES (100790, 'guide', 'The Horn of Khelmarak', 'Gilthirn', '', 1, '');
INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, modelid, hp_percent, npc_id) VALUES
    (1000, 'rat', 0, 100, 50, 100, 0, 1, 7, 100, 0), (1001, 'froglok herbalist', 0, 24750, 10, 29321, 0, 30, 8, 100, 0);
"""
LATER = "2026-11-21T10:00:00Z"
WATCHMAN = ("froglok watchman", 0, 24726.0, 10.0, 29310.5, 337378828)
RUSTEL = ("Rustel Minotaur", 0, 13047.0, 26505.0, 3318478031)


def audit(db, user, action, tbl, rk, before, after, detail, at=LATER):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, 1, ?, 'tool', '', ?, ?, ?, ?, ?, ?)",
               (at, user, action, tbl, json.dumps(rk, separators=(",", ":")), json.dumps(before) if before else "",
                json.dumps(after) if after else "", detail))


def npc(db, i, name, world, x, y, z, model):
    db.execute("INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, npc_level, modelid, hp_percent, npc_id) "
               "VALUES (?, ?, ?, ?, ?, ?, 0, 20, ?, 100, 0)", (i, name, world, x, y, z, model))


def steps(watchman, stop_slot, brew_text):
    return json.dumps([{"do": "lock", "lock": "guk herbalist"},
                       {"do": "say", "npc": "froglok herbalist", "text": brew_text},
                       {"do": "spawn", "npc": f"#{watchman}", "count": 2, "world": 0, "x": 24726.0, "y": 10.0,
                        "z": 29310.5, "spread": 3, "minutes": 90, "attackable": True},
                       {"do": "give", "items": "700720*2"},
                       {"do": "stop", "slot": stop_slot},
                       {"do": "queststep", "quest": 100790, "step": 4}], separators=(",", ":"))


def make_pair(d):
    """(heirloom db, live db) rehearsal copies (shard_label, no config.yaml)"""
    h, l = os.path.join(d, "h.db"), os.path.join(d, "l.db")
    for p, shard in ((h, "heirloom"), (l, "live")):
        db = sqlite3.connect(p)
        db.executescript(SCHEMA)
        db.execute("CREATE TABLE shard_label(shard TEXT)")
        db.execute("INSERT INTO shard_label VALUES (?)", (shard,))
        db.commit()
        db.close()
    hdb, ldb = sqlite3.connect(h), sqlite3.connect(l)
    # the same tool on each shard: the watchman template under each shard's own id, the Rustel Minotaur too
    npc(hdb, 163912, *WATCHMAN)
    npc(ldb, 162757, *WATCHMAN)
    npc(hdb, 190359, RUSTEL[0], RUSTEL[1], RUSTEL[2], 57.0, RUSTEL[3], RUSTEL[4])
    npc(ldb, 191858, RUSTEL[0], RUSTEL[1], RUSTEL[2], 57.0, RUSTEL[3], RUSTEL[4])
    # the Trial of Celerity: Live slot 150, Heirloom slot 237 (the 11-15 sync gave it a new slot); Heirloom's 150 another
    for db, slot in ((hdb, 237), (ldb, 150)):
        db.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (?, 'The Trial of Celerity', '[]')", (slot,))
    hdb.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (150, 'The Harvest Feast', '[]')")
    ldb.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (237, 'The Harvest Feast', '[]')")
    # trigger 161, Heirloom's rows; Live edited its own (newer, audited): Live's text is copied to Heirloom
    hdb.execute("INSERT INTO gmtriggers (slot, name, radius, steps) VALUES (161, 'The Froglok Herbalist', 80, ?)",
                (steps(163912, 237, "Guard me!"),))
    ldb.execute("INSERT INTO gmtriggers (slot, name, radius, steps) VALUES (161, 'The Froglok Herbalist', 80, ?)",
                (steps(162757, 150, "Now guard me while I brew!"),))
    audit(ldb, "dab", "update", "gmtriggers", {"slot": 161}, {"steps": steps(162757, 150, "Guard me!")},
          {"steps": steps(162757, 150, "Now guard me while I brew!")}, "petition 1617: the herbalist's words")
    # a quest step's events: Live's edit spawns its Rustel Minotaur (the 11-22 sync's quest 100790 step 8)
    for db, rustel, n in ((hdb, 190359, 5), (ldb, 191858, 6)):
        db.execute("INSERT INTO queststeps (questid, step, kind, target, items, journal, npcid, events) "
                   "VALUES (100790, 8, 'kill', 'Khelmarak', '', 'Kill Khelmarak.', 0, ?)",
                   (json.dumps([{"on": "done", "do": "spawn", "npc": f"#{rustel}", "count": n, "minutes": 10}]),))
    audit(ldb, "dab", "update", "queststeps", {"questid": 100790, "step": 8}, {"count": 5}, {"count": 6},
          "petition 2394: Khelmarak's kill spawns six")
    # a Live-only trigger naming a row Live no longer has: nothing on Heirloom to translate it to
    ldb.execute("INSERT INTO gmtriggers (slot, name, steps) VALUES (170, 'The Lost Spawn', ?)",
                (json.dumps([{"do": "spawn", "npc": "#162999"}]),))
    audit(ldb, "dab", "insert", "gmtriggers", {"slot": 170}, None, {"name": "The Lost Spawn"}, "petition 3000: a lost spawn")
    for db in (hdb, ldb):
        db.commit()
        db.close()
    return h, l


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    m = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(m)
    return m


def plan_with(tooldir, h, l, out):
    """the H/L plan live_sync_20261122.py (in tooldir) makes of the pair"""
    sys.path.insert(0, tooldir)
    try:
        for m in ("shard_refs", "vanilla_tickets"):
            sys.modules.pop(m, None)
        ls = load(f"live_sync_{abs(hash(tooldir))}", os.path.join(tooldir, "live_sync_20261122.py"))
        with contextlib.redirect_stdout(io.StringIO()):
            plan, report = ls.make_plan(h, l, out, None)
        return plan
    finally:
        sys.path.remove(tooldir)
        for m in ("shard_refs", "vanilla_tickets"):
            sys.modules.pop(m, None)


def op(plan, side, t, **pk):
    for o in plan[side]["ops"]:
        if o["t"] == t and all(o["pk"].get(k) == v for k, v in pk.items()):
            return o
    return None


def old_tools(d):
    """the tools as they were (git OLD) in d, or None when git cannot show them"""
    os.makedirs(d, exist_ok=True)
    repos = [HERE, os.path.expanduser(os.environ.get("EQOA_SERVER_REPO", "~/claude-eqoa/go-eqoa-server"))]
    for f in ("live_sync_20261122.py", "shard_sync_20261115.py", "vanilla_tickets.py"):
        for repo in repos:
            r = subprocess.run(["git", "-C", repo, "show", f"{OLD}:tools/{f}"], capture_output=True)
            if not r.returncode:
                break
        else:
            return None
        with open(os.path.join(d, f), "wb") as fh:
            fh.write(r.stdout)
    return d


class Translation(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp(prefix="refs1127-", dir=os.environ.get("TMPDIR"))
        self.h, self.l = make_pair(self.d)

    def tearDown(self):
        shutil.rmtree(self.d, ignore_errors=True)

    def test_old_engine_copies_the_other_shards_ids(self):
        old = old_tools(os.path.join(self.d, "old"))
        if old is None:
            self.skipTest(f"git cannot show {OLD}")
        plan = plan_with(old, self.h, self.l, os.path.join(self.d, "old.json"))
        o = op(plan, "H", "gmtriggers", slot=161)
        self.assertIsNotNone(o)
        s = json.loads(o["after"]["steps"])
        self.assertEqual(s[2]["npc"], "#162757")          # Live's row: no such row on Heirloom (petition 3352)
        self.assertEqual(s[4]["slot"], 150)               # Live's slot: Heirloom's 150 is the Harvest Feast
        q = op(plan, "H", "queststeps", questid=100790, step=8)
        self.assertEqual(json.loads(q["after"]["events"])[0]["npc"], "#191858")
        lost = [o for o in plan["H"]["ops"] if o["t"] == "gmtriggers" and (o["after"] or {}).get("name") == "The Lost Spawn"]
        self.assertEqual(len(lost), 1)
        self.assertIn("#162999", lost[0]["after"]["steps"])
        # and so the shard holds references to no row
        db = sqlite3.connect(self.h)
        ids = refs.existing(db)
        self.assertNotIn(162757, ids["npc"])
        self.assertNotIn(191858, ids["npc"])
        db.close()

    def test_new_engine_translates(self):
        plan = plan_with(HERE, self.h, self.l, os.path.join(self.d, "new.json"))
        o = op(plan, "H", "gmtriggers", slot=161)
        self.assertIsNotNone(o, "Live's edit of trigger 161 is still copied")
        s = json.loads(o["after"]["steps"])
        self.assertEqual(s[1]["text"], "Now guard me while I brew!")
        self.assertEqual(s[2]["npc"], "#163912")           # Heirloom's own watchman
        self.assertEqual(s[4]["slot"], 237)                # Heirloom's Trial of Celerity
        self.assertEqual(s[3]["items"], "700720*2")
        self.assertEqual(s[5]["quest"], 100790)
        # only the references changed, the text kept as Live wrote it
        live = steps(162757, 150, "Now guard me while I brew!")
        self.assertEqual(o["after"]["steps"], live.replace('"#162757"', '"#163912"').replace('"slot":150', '"slot":237'))
        q = op(plan, "H", "queststeps", questid=100790, step=8)
        self.assertEqual(json.loads(q["after"]["events"])[0]["npc"], "#190359")
        self.assertEqual(json.loads(q["after"]["events"])[0]["count"], 6)
        # the plan says where each translated reference came from
        self.assertEqual(plan["H"]["refs"]["npc"]["163912"]["src"], 162757)
        self.assertEqual(plan["H"]["refs"]["npc"]["163912"]["name"], "froglok watchman")
        self.assertEqual(plan["H"]["refs"]["slot"]["237"], {"src": 150, "name": "the trial of celerity"})
        # the untranslatable one is not written: listed
        self.assertFalse([o for o in plan["H"]["ops"] if (o["after"] or {}).get("name") == "The Lost Spawn"])
        un = plan["H"]["unresolved"]
        self.assertEqual(len(un), 1)
        self.assertEqual(un[0]["table"], "gmtriggers")
        self.assertIn("npc 162999", un[0]["why"])
        # every reference the H half writes names a row Heirloom has (or one the half inserts)
        db = sqlite3.connect(self.h)
        ids = refs.existing(db)
        db.close()
        for o in plan["H"]["ops"]:
            for c, v in (o["after"] or {}).items():
                for kind, i, where in refs.refs_in(o["t"], c, v):
                    self.assertIn(i, ids[kind], f"{o['t']} {o['pk']} {where}")

    def test_same_trigger_by_its_rows_is_no_difference(self):
        # Heirloom and Live hold the same trigger, each naming its own rows: nothing to copy
        for p, w, slot in ((self.h, 163912, 237), (self.l, 162757, 150)):
            db = sqlite3.connect(p)
            db.execute("UPDATE gmtriggers SET steps = ? WHERE slot = 161", (steps(w, slot, "Same."),))
            db.execute("DELETE FROM gm_audit WHERE tbl = 'gmtriggers' AND rowkey = '{\"slot\":161}'")
            db.commit()
            db.close()
        plan = plan_with(HERE, self.h, self.l, os.path.join(self.d, "same.json"))
        self.assertIsNone(op(plan, "H", "gmtriggers", slot=161))
        self.assertFalse([o for o in plan["L"]["ops"] if o["t"] == "gmtriggers" and o["pk"].get("slot") == 161])


class Registry(unittest.TestCase):
    def test_refs_by_key_only(self):
        v = json.dumps([{"do": "spawn", "npc": "#163912", "x": 163912, "count": 163912},
                        {"do": "say", "text": "row \"#5\" and quest 7", "npc": "froglok"},
                        {"do": "give", "items": "716105*2, 700720"}, {"do": "stop", "slot": 113},
                        {"onfail": [{"do": "questset", "quest": 101306, "step": 11}]}])
        self.assertEqual([(k, i) for k, i, _ in refs.refs_in("gmtriggers", "steps", v)],
                         [("npc", 163912), ("item", 716105), ("item", 700720), ("slot", 113), ("quest", 101306)])
        # a stop slot is a trigger only in gmtriggers.steps
        self.assertNotIn("slot", [k for k, _, _ in refs.refs_in("queststeps", "events", v)])
        m = {("npc", 163912): 1, ("item", 716105): 2, ("item", 700720): 3, ("slot", 113): 4, ("quest", 101306): 5}
        out = refs.rewrite("gmtriggers", "steps", v, lambda k, i: m[(k, i)])
        self.assertEqual(json.loads(out)[0], {"do": "spawn", "npc": "#1", "x": 163912, "count": 163912})
        self.assertEqual(json.loads(out)[1]["text"], "row \"#5\" and quest 7")
        self.assertEqual(json.loads(out)[2]["items"], "2*2, 3")
        self.assertEqual(json.loads(out)[3]["slot"], 4)
        self.assertEqual(json.loads(out)[4]["onfail"][0], {"do": "questset", "quest": 5, "step": 11})
        self.assertEqual(refs.rewrite("npcloot", "npc_name", "#163912", lambda k, i: 9), "#9")
        self.assertEqual(refs.rewrite("quests", "requires", "100793|100794", lambda k, i: i + 1), "100794|100795")
        self.assertEqual(refs.rewrite("queststeps", "items", "701447, 701450*2", lambda k, i: i + 1), "701448, 701451*2")
        with self.assertRaises(refs.Untranslatable):
            refs.rewrite("gmtriggers", "steps", v, lambda k, i: (_ for _ in ()).throw(refs.Untranslatable("no")))

    def test_check_refs_finds_a_planted_miss(self):
        d = tempfile.mkdtemp(prefix="refs1127-", dir=os.environ.get("TMPDIR"))
        try:
            h, _ = make_pair(d)
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf):
                self.assertEqual(refs.main(["--check-refs", "--db", h]), 0, buf.getvalue())
            self.assertIn("REFS OK", buf.getvalue())
            db = sqlite3.connect(h)
            db.execute("UPDATE gmtriggers SET steps = ? WHERE slot = 161", (steps(162757, 237, "x"),))
            db.execute("INSERT INTO npcpets VALUES ('gnoll seer', 'kindred spirit', 163739, 1, 0, '', 'gm')")
            db.commit()
            db.close()
            buf = io.StringIO()
            with contextlib.redirect_stdout(buf):
                self.assertEqual(refs.main(["--check-refs", "--db", h, "--json", os.path.join(d, "m.json")]), 1)
            out = buf.getvalue()
            self.assertIn("REFS FAIL", out)
            self.assertIn("gmtriggers {\"slot\":161} 'The Froglok Herbalist' steps:[2].npc: npc 162757", out)
            self.assertIn("npcpets", out)
            with open(os.path.join(d, "m.json")) as f:
                ms = json.load(f)
            self.assertEqual(sorted((m["table"], m["id"]) for m in ms), [("gmtriggers", 162757), ("npcpets", 163739)])
        finally:
            shutil.rmtree(d, ignore_errors=True)


if __name__ == "__main__":
    unittest.main()
