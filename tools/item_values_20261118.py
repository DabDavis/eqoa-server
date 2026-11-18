#!/usr/bin/env python3
"""Values for the creature drops that sell for nothing (the owner, 2026-11-18: "many items sell for 0 especially for
new players that need tunar").

A merchant pays an item's value (itempattern.itemcost) less its wear and sells a catalog item at the same value
(handlers/merchant.go: sellPrice, merchantPrice, merchantRate 100): a value of 0 sells for nothing. The servers' logs
showed it: the newbie trash of Return Home's starting quests ("tarantula leg tips", "ruined bat wing", "sliver of
snake meat", ...) and the quest drops players keep after a quest ("Bandit Glove", "Crude Deathfist Belt") sold for 0.

tools/price_zero_items_20261115.py priced the zero-value records by kind but KEPT every non-gear record a quest asks
for (its rule "quest"), and those are most of what creatures drop at 0: the starting quests' parts are in the
creatures' trash pool too, and a loot step (queststeps kind 'loot') drops its item off kills. Its "unobtainable" also
missed the loot steps (only quest gives count there). This tool prices exactly those: a record of value 0 that a
CREATURE gives (a loot table, the loot pool, or an enabled quest's loot step).

No real record gives these values: Allakhazam's archived item pages carry no merchant value (only players' asking
prices), EQOA-Data's item files have no value field, and Sandstorm is no reference (the owner, 2026-11-15). So the
values are our own data's, the way the priced trash was made (tools/seed_trash_items.py: PRICE_PER_LEVEL x the
median level of the creatures that drop it):

PRICED (each by its rule; never above what a merchant sells an item of the same name for):
  material  a tradeskill component: price_zero_items_20261115's material rule (the seed's tiers)
  sibling   the median value of the priced trash the SAME creatures drop (lootpool category 'trash', not gear or a
            scroll), held to [FLOOR, SIBLING_CAP x the level rule]: "tarantula leg tips" sells as the tarantula's
            "ruined tarantula fur" and "torn tarantula silk" do
  level     else max(FLOOR, round(PRICE_PER_LEVEL x L)), L the median level of the creatures that drop it (loot
            tables' holders, the pool's creature, a loot step's target and its other targets; a table bound to no
            creature: the middle of its row's level range). FLOOR (5) is the least the seeded trash of level-2
            creatures fetches

KEPT at 0 (each listed with its reason; price_zero_items_20261115's facts):
  copy, gm-zeroed, placeholder, heirloom, familiar, token, conjured (NO RENT, norentvanish, a spell's or an item's
  summon), heroes (bought with standing), merchant-free (a merchant stocks it: at 0 a shop gives it away; priced, a
  shop would charge for it), start-kit (a new character's kit: made again with every character), repeatable (a
  repeatable quest gives it), quest-given (any quest hands it over: a quest given up and taken again hands it
  over again), crafted (a tradeskill product: the crafted rule's, price_zero_items_20261115), gear and scroll (their
  own rules there; what is left at 0 there has no peers, e.g. wear slot 17), no-level (no creature level known).

Across the shards: --write-plan reads each shard's copy (--from heirloom=A.db live=B.db), plans each, and writes ONE
plan whose entries are keyed by the item's id where both shards have that id under the same name, else by its name;
an item both shards price takes the lesser of the two values, so one item sells alike on both. Each entry keeps the
ids it is on in each shard. --apply --plan FILE --shard S writes S's entries, each only where that id still has that
name and value 0, no merchant stocks it now, and held to what a merchant sells the name for now.

Without --plan, --apply plans from --db itself (one shard; for a rig or a test).

Every value written is logged in item_values_1118_log (patternid, name, old, new, rule, how, its gm_audit id) and has
its own gm_audit line (tbl itempattern, rowkey {"patternid":N}, before/after its itemcost), so the GM panel shows and
can revert each. A second --apply finds nothing to do; --undo puts back 0 on each record still at the value written
(one changed since is left and said), audits each, and drops the log.

WHEN IT TAKES EFFECT: at once: the server reads a value at each sale (no restart). CLIENT shows the value off the
record sent at login, so the window shows it at the next login; the server pays it at once.

    python3 -E -s tools/item_values_20261118.py --db eqoa.db [--list]                 # report
    python3 -E -s tools/item_values_20261118.py --write-plan P.json --from heirloom=H.db live=L.db
    python3 -E -s tools/item_values_20261118.py --db eqoa.db --plan P.json --shard heirloom [--list] [--apply | --undo]
"""
import argparse
import collections
import json
import os
import sqlite3
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import price_zero_items_20261115 as pz  # noqa: E402

LOG = "item_values_1118_log"
DETAIL = ("the owner (2026-11-18, \"many items sell for 0 especially for new players that need tunar\"): a creature "
          "drop worth 0 priced; tools/item_values_20261118.py")
PRICE_PER_LEVEL = 2.40   # tools/seed_trash_items.py: tunar a level of the creatures that drop a trash item
FLOOR = 5                # the least the seeded trash of level-2 creatures fetches (round(2.40 x 2))
SIBLING_CAP = 3          # a sibling median at most this many times the level rule (a rare among the trash)
SIBLING_MIN = 1          # priced trash siblings needed

KEEP_ORDER = ("copy", "gm-zeroed", "placeholder", "heirloom", "familiar", "token", "conjured", "heroes",
              "merchant-free", "start-kit", "repeatable", "quest-given", "crafted", "gear", "scroll", "no-level")
PRICE_ORDER = ("material", "sibling", "level")


def level_rule(lv):
    return max(FLOOR, int(round(PRICE_PER_LEVEL * lv)))


class Drops:
    """Who drops each record: creature names (normalised) and their levels."""

    def __init__(self, db):
        self.creatures = collections.defaultdict(set)    # pid -> creature keys
        self.levels = collections.defaultdict(list)      # pid -> levels
        self.loot_step = set()                           # pids a loot step drops
        self.pool_trash = set()
        name_lv = collections.defaultdict(list)
        row_name = {}
        if pz.table(db, "npcs"):
            for rid, n, lv in db.execute("SELECT id, npc_name, npc_level FROM npcs"):
                name_lv[pz.norm(n)].append(lv or 0)
                row_name[rid] = pz.norm(n)
        self.med = {n: statistics.median(v) for n, v in name_lv.items()}

        def add(pid, key, fallback=None):
            if key is not None:
                self.creatures[pid].add(key)
            lv = self.med.get(key) if key is not None else None
            if lv is None:
                lv = fallback
            if lv is not None:
                self.levels[pid].append(lv)

        def band(mn, mx):
            return (mn + mx) / 2 if mx and mx < 100 else mn
        if pz.table(db, "lootpool"):
            for cat, npc, pid, mn, mx in db.execute(
                    "SELECT category, npc_name, patternid, minlevel, maxlevel FROM lootpool"):
                if cat == "trash":
                    self.pool_trash.add(pid)
                add(pid, pz.norm(npc) if npc else None, band(mn or 1, mx or 100))
        if pz.table(db, "loottables"):
            holders = collections.defaultdict(list)
            if pz.table(db, "npcloot"):
                for npc, t in db.execute("SELECT npc_name, tablename FROM npcloot"):
                    if npc.startswith("#") and npc[1:].isdigit():
                        npc = row_name.get(int(npc[1:]))
                    else:
                        npc = pz.norm(npc)
                    if npc:
                        holders[t].append(npc)
            for t, pid, mn, mx in db.execute("SELECT tablename, patternid, minlevel, maxlevel FROM loottables"):
                hs = holders.get(t)
                if hs:
                    for h in hs:
                        add(pid, h, None)
                    if not any(h in self.med for h in hs):
                        self.levels[pid].append(band(mn or 1, mx or 60))
                else:
                    add(pid, None, band(mn or 1, mx or 60))
        if pz.table(db, "queststeps"):
            enabled = {r[0] for r in db.execute("SELECT questid FROM quests WHERE enabled <> 0")} \
                if pz.table(db, "quests") and "enabled" in pz.cols(db, "quests") else None
            also = collections.defaultdict(list)
            if pz.table(db, "queststeptargets"):
                for qid, st, t in db.execute("SELECT questid, step, target FROM queststeptargets"):
                    also[(qid, st)].append(t)
            minlv = {r[0]: r[1] for r in db.execute("SELECT questid, minlevel FROM quests")} \
                if pz.table(db, "quests") and "minlevel" in pz.cols(db, "quests") else {}
            sc = pz.cols(db, "queststeps")
            tcol = "target" if "target" in sc else "''"
            for qid, st, items, target in db.execute(
                    f"SELECT questid, step, items, {tcol} FROM queststeps WHERE kind = 'loot'"):
                if enabled is not None and qid not in enabled:
                    continue
                first = [f.strip() for f in str(items or "").split(",") if f.strip()]
                ids = pz.item_ids(first[0]) if first else set()
                for pid in ids:
                    self.loot_step.add(pid)
                    names = [n for n in [target] + also.get((qid, st), []) if n and n.strip() and n.strip() != "*"]
                    got = False
                    for n in names:
                        k = pz.norm(n)
                        self.creatures[pid].add(k)
                        if k in self.med:
                            self.levels[pid].append(self.med[k])
                            got = True
                    if not got and minlv.get(qid):
                        self.levels[pid].append(minlv[qid])

    def drops(self, pid):
        return pid in self.creatures or pid in self.levels


def plan(db):
    """[(pid, name, rule, value, how)] priced and [(pid, name, reason, how)] kept: the records of value 0 a creature
    drops; plus a Counter of the other records of value 0 (not drops), left to price_zero_items_20261115."""
    pats = pz.load(db)
    f = pz.Facts(db, pats)
    rules = pz.Rules(pats, f)
    d = Drops(db)
    # priced trash: by creature
    trash_by_creature = collections.defaultdict(list)
    for pid in d.pool_trash:
        p = pats.get(pid)
        if not p or p["itemcost"] <= 0 or pz.is_gear(p) or pz.is_scroll(p, f.spells):
            continue
        if pid in f.copies or p["rent"] == 0:
            continue
        for c in d.creatures.get(pid, ()):
            trash_by_creature[c].append((p["itemcost"], pid))
    priced, kept, other = [], [], collections.Counter()
    for pid in sorted(pats):
        p = pats[pid]
        if p["itemcost"] != 0:
            continue
        if not d.drops(pid):
            other["not a drop"] += 1
            continue
        name = p["itemname"] or ""
        n = pz.norm(name)
        gear, scroll = pz.is_gear(p), pz.is_scroll(p, f.spells)

        def keep(reason, how=""):
            kept.append((pid, name, reason, how))
        if pid in f.copies:
            keep("copy", "follows its base at the next start")
        elif pid in f.gm_zeroed:
            keep("gm-zeroed", "a gm_audit line set it to 0")
        elif pz.PLACEHOLDER.search(name):
            keep("placeholder", "its name")
        elif pid in f.heirlooms or name.startswith("Heirloom "):
            keep("heirloom", "the server's own, given again on request")
        elif pid in f.familiars:
            keep("familiar")
        elif pz.TOKEN.search(name) and not gear:
            keep("token", "its name")
        elif p["rent"] == 0 or pid in f.norentvanish or pid in f.summon_ids or (n in f.summon_names and not scroll):
            keep("conjured", "NO RENT" if p["rent"] == 0 else "norentvanish" if pid in f.norentvanish else
                 "a spell's create_item" if pid in f.summon_ids else "a Summon line names it")
        elif pid in f.heroes:
            keep("heroes", "bought with standing")
        elif pid in f.stocked or pid in f.free:
            keep("merchant-free", "a merchant stocks it at 0")
        elif pid in f.start:
            keep("start-kit", "a new character's kit")
        elif pid in f.repeat_gives:
            keep("repeatable", "a repeatable quest gives it")
        elif pid in f.quest_gives:
            keep("quest-given", "a quest hands it over")
        elif pid in f.products:
            keep("crafted", "a tradeskill product: price_zero_items_20261115's crafted rule")
        elif gear:
            keep("gear", f"wear slot {p['equipslot']}: price_zero_items_20261115's gear rule")
        elif scroll:
            keep("scroll", "price_zero_items_20261115's scroll rule")
        else:
            v, rule, how = None, None, ""
            if pid in f.uses:
                rule = "material"
                v, how = rules.material(p)
            if not v:
                lvs = d.levels.get(pid)
                if not lvs:
                    keep("no-level", "no creature level known")
                    continue
                lv = statistics.median(lvs)
                base = level_rule(lv)
                sib = sorted({(c, q) for cr in d.creatures.get(pid, ()) for c, q in trash_by_creature.get(cr, ())
                              if q != pid}, key=lambda x: x[1])
                if len(sib) >= SIBLING_MIN:
                    m = statistics.median(c for c, _q in sib)
                    v = min(max(FLOOR, int(round(m))), SIBLING_CAP * base)
                    rule = "sibling"
                    how = (f"median {m:g} of the same creatures' priced trash ({', '.join(str(q) for _c, q in sib[:6])}"
                           f"{', ...' if len(sib) > 6 else ''}), creatures' level {lv:g}")
                else:
                    v, rule = base, "level"
                    how = f"{PRICE_PER_LEVEL} x creatures' level {lv:g}" + (f", at least {FLOOR}" if base == FLOOR else "")
            sold = f.sold_by_name.get(n)
            if sold is not None and sold < v:
                v, how = sold, f"{how}; held to {sold}, what a merchant sells one of its name for"
            if pid in d.loot_step and pid not in f.loot_ids:
                how += "; a quest's loot step only"
            priced.append((pid, name, rule, int(v), how))
    return priced, kept, other


# ---- the plan across shards -------------------------------------------------------------------------------------

def make_plan(shards):
    """shards: {label: sqlite3 connection}. Entries keyed by id where every shard that has the item has that id
    under that name, else by name; an item priced on several shards takes the least value."""
    per = {}
    names_by_id = {}
    for label, db in shards.items():
        pr, kp, _o = plan(db)
        per[label] = (pr, kp)
        names_by_id[label] = {r[0]: pz.norm(r[1]) for r in db.execute("SELECT patternid, itemname FROM itempattern")}
    entries = {}
    for label, (pr, _kp) in per.items():
        for pid, name, rule, v, how in pr:
            same = all(names_by_id[o].get(pid) == pz.norm(name) for o in shards)
            key = f"id:{pid}" if same else f"name:{pz.norm(name)}"
            e = entries.setdefault(key, {"key": key, "name": name, "value": v, "rule": rule, "how": how,
                                         "shards": {}})
            e["shards"].setdefault(label, []).append(pid)
            if v < e["value"]:
                e.update(value=v, rule=rule, how=how + f" (the lesser of the shards': {label})")
    out = {"tool": "item_values_20261118", "entries": sorted(entries.values(), key=lambda e: e["key"]),
           "kept": {label: [list(k) for k in kp] for label, (_pr, kp) in per.items()}}
    return out


def plan_for_shard(pl, label):
    return [(pid, e["name"], e["rule"], e["value"], e["how"]) for e in pl["entries"]
            for pid in e["shards"].get(label, [])]


# ---- writing ------------------------------------------------------------------------------------------------------

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    itemname  TEXT NOT NULL,
    old       INTEGER NOT NULL,
    new       INTEGER NOT NULL,
    rule      TEXT NOT NULL,
    how       TEXT NOT NULL,
    audit     INTEGER NOT NULL DEFAULT 0,
    applied   TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def now_checks(db):
    """What a merchant stocks now and its least price by name: a planned value is checked against these at --apply."""
    stocked, sold = set(), {}
    if pz.table(db, "npcitem"):
        for pid, name, cost in db.execute("SELECT DISTINCT n.patternid, p.itemname, p.itemcost FROM npcitem n "
                                          "JOIN itempattern p ON p.patternid = n.patternid"):
            stocked.add(pid)
            if cost and cost > 0:
                k = pz.norm(name)
                sold[k] = min(sold.get(k, cost), cost)
    return stocked, sold


def do_apply(db, priced):
    db.execute(LOG_DDL)
    stocked, sold = now_checks(db)
    done, ids, skipped = 0, [], []
    for pid, name, rule, v, how in priced:
        row = db.execute("SELECT itemname, itemcost FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if not row or pz.norm(row[0]) != pz.norm(name):
            skipped.append((pid, name, "not this record here"))
            continue
        if row[1] != 0:
            continue
        if pid in stocked:
            skipped.append((pid, name, "a merchant stocks it now"))
            continue
        cap = sold.get(pz.norm(name))
        if cap is not None and cap < v:
            v, how = cap, f"{how}; held to {cap}, what a merchant sells one of its name for"
        cur = db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ? AND itemcost = 0", (v, pid))
        if cur.rowcount != 1:
            continue
        a = pz.audit(db, pid, 0, v, f"{DETAIL}: {rule}, {how}"[:900])
        db.execute(f"INSERT INTO {LOG} (patternid, itemname, old, new, rule, how, audit) VALUES (?, ?, 0, ?, ?, ?, ?)",
                   (pid, name, v, rule, how, a))
        ids.append(a)
        done += 1
    for pid, name, why in skipped:
        print(f"  skipped {pid} {name!r}: {why}")
    return done, ids


def do_undo(db):
    if not pz.table(db, LOG):
        print("nothing to undo: no log")
        return 0
    back = left = 0
    ids = []
    for seq, pid, old, new in db.execute(f"SELECT seq, patternid, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ? AND itemcost = ?", (old, pid, new))
        if cur.rowcount != 1:
            left += 1
            print(f"  left: {pid} is no longer at {new}")
            continue
        ids.append(pz.audit(db, pid, new, old, "undo: " + DETAIL))
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    print(f"put back {back}; left {left} changed since" + (f"; gm_audit {min(ids)}-{max(ids)}" if ids else ""))
    return 0 if left == 0 else 1


def report(priced, kept, other, show_all):
    print(f"{len(priced) + len(kept)} records of value 0 that creatures drop: {len(priced)} to price, "
          f"{len(kept)} kept at 0; {sum(other.values())} other records of value 0 are no drop (not this tool's)")
    kc = collections.Counter(r for _p, _n, r, _h in kept)
    print("kept at 0, by reason:")
    for r in KEEP_ORDER:
        if kc[r]:
            sample = [f"{p} {n!r}" for p, n, rr, _h in kept if rr == r][:5]
            print(f"  {r:13} {kc[r]:5}   e.g. {'; '.join(sample)}")
    pc = collections.Counter(r for _p, _n, r, _v, _h in priced)
    print("priced, by rule:")
    for r in PRICE_ORDER:
        if pc[r]:
            vals = [v for _p, _n, rr, v, _h in priced if rr == r]
            sample = [f"{p} {n!r} {v}" for p, n, rr, v, _h in priced if rr == r][:4]
            print(f"  {r:9} {pc[r]:5}   values median {statistics.median(vals):g}, {min(vals)}-{max(vals)}; "
                  f"e.g. {'; '.join(sample)}")
    qs = [x for x in priced if "loot step only" in x[4]]
    print(f"  of them {len(qs)} dropped by a quest's loot step only; "
          f"{sum(1 for x in priced if 'held to' in x[4])} held to a merchant's price")
    if show_all:
        for pid, name, rule, v, how in priced:
            print(f"  price {pid} {name!r}: {v} ({rule}: {how})")
        for pid, name, r, how in kept:
            print(f"  keep {pid} {name!r}: {r}" + (f" ({how})" if how else ""))


def connect(path, ro=False):
    if ro:
        db = sqlite3.connect(f"file:{path}?mode=ro", uri=True, timeout=30)
    else:
        db = sqlite3.connect(path, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 10000")
    return db


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db")
    ap.add_argument("--plan", help="a plan written by --write-plan (else plan from --db itself)")
    ap.add_argument("--shard", help="with --plan: which shard's entries (heirloom, live)")
    ap.add_argument("--write-plan", metavar="FILE")
    ap.add_argument("--from", dest="sources", nargs="+", metavar="LABEL=DB", help="with --write-plan: shard copies")
    ap.add_argument("--list", action="store_true", help="print every record, priced and kept")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the values (logged, audited)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args(argv)

    if a.write_plan:
        if not a.sources:
            ap.error("--write-plan needs --from LABEL=DB ...")
        shards = {}
        for s in a.sources:
            label, _, path = s.partition("=")
            shards[label] = connect(path, ro=True)
        pl = make_plan(shards)
        for label, db in shards.items():
            pr = plan_for_shard(pl, label)
            kept = [tuple(k) for k in pl["kept"][label]]
            print(f"== {label}")
            report(pr, kept, collections.Counter(), a.list)
            db.close()
        diff = [e for e in pl["entries"] if len(e["shards"]) > 1 and "lesser of the shards" in e["how"]]
        print(f"plan: {len(pl['entries'])} entries ({sum(1 for e in pl['entries'] if e['key'].startswith('id:'))} "
              f"by id, the rest by name); {len(diff)} took the lesser of two shards' values")
        with open(a.write_plan, "w") as fh:
            json.dump(pl, fh, indent=1, sort_keys=True)
            fh.write("\n")
        print(f"wrote {a.write_plan}")
        return 0

    if not a.db:
        ap.error("--db is needed")
    db = connect(a.db)
    try:
        if not pz.table(db, "itempattern") or not pz.table(db, "gm_audit"):
            print("refused, nothing done: no itempattern or gm_audit table here")
            return 1
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                rc = do_undo(db)
                db.execute("COMMIT")
                return rc
            except BaseException:
                db.execute("ROLLBACK")
                raise

        def priced_now():
            if a.plan:
                if not a.shard:
                    ap.error("--plan needs --shard")
                with open(a.plan) as fh:
                    pl = json.load(fh)
                if a.shard not in pl["kept"]:
                    ap.error(f"the plan has no shard {a.shard!r}: {', '.join(pl['kept'])}")
                return plan_for_shard(pl, a.shard), [tuple(k) for k in pl["kept"][a.shard]], collections.Counter()
            return plan(db)
        if not a.apply:
            report(*priced_now(), a.list)
            print("report only: --apply to write")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            priced, kept, other = priced_now()
            report(priced, kept, other, a.list)
            done, ids = do_apply(db, priced)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {done} values written, log {LOG}" + (f", gm_audit {min(ids)}-{max(ids)}" if ids else
                                                               ", nothing to do"))
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
