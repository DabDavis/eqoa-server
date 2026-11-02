#!/usr/bin/env python3
"""Values for the gear worth nothing (the user, 2026-11-02: "when I try to sell rares like the Sword of Faith it
sells for 0 tunar", then "also look for others with same issue").

An item sells for its value at the merchant's rate less its wear (CLIENT's GetInvItemSellPrice, the server's
handlers/merchant.go sellPrice) and is mended for its value times its wear (GetInvItemRepairCost,
handlers/blacksmith.go): an item of value 0 sells for nothing and is mended for nothing. Return Home's import
brought every item it had no price for at 0 (tools/import_prh_content.py: "itemcost 0 means price unknown"),
and the tools that copied its records kept it: on eqoaclassic about 6,800 records of gear of level 2 and up,
the Sword of Faith among them.

Sandstorm's records price gear of level 2 and up: 374 of its 377 such records carry a value (the three at 0
are a quest's staff and two thrown potions), where gear of level 0 and 1 is mostly the starting quests'
handouts, at 0 (27 of 35). So each record of wear slot 1-19 (worn, held and wielded) of level 2 and up and
value 0 is given:
1. Sandstorm's value for it (tools/stock_merchants.py's Measured: the harvests' wares and inventories, by
   name, else by Sandstorm's id at the same wear slot and level); a value of 0 there is kept, and listed;
2. else stock_merchants' rule, fitted on those same values: a weapon without stats 1.845 x its damage,
   anything else the median of the 3 nearest measured items of its kind (its error is printed, leave one out),
   without the measured items worth less than a fifth of their kind's within 5 levels (robust_rule): Sandstorm's
   crafted Mithril Diamond ring, bracelet and pendant at 131 beside the earring's 3,614 had priced 93 dropped
   jewels of 42-60 at 131, and cheap vendor goods and bait are no measure of a rare.
Never, each listed:
- a NO RENT record (rent 0): what a spell conjures (entity/balancekit.go simPick), which sold would be tunar
  out of nothing; nor an item a spell summons by name (a spelleffect line's "Summon X", "Summons X x3", or its
  workbook items: the alchemist's thrown potions and cocktails, normal rent), for the same reason (Sandstorm
  values its thrown potions 0);
- a record in wear slot 17, which no record of Sandstorm's uses (two misfiled items: listed);
- a crafted product (craftrecipes.product): the tradeskill seeds price theirs against their components;
- a repeatable quest's reward (its steps' rewarditems, byclass, choices and give events, and the quest's
  own): done again and again it would mint tunar;
- a record a GM edited in the panel (gm_audit).

Run it after tools/fix_belt_robe_slots.py: the rule compares an item with items of its own wear slot first.

Every change is logged in item_value_fill_log; --undo puts back each record still as this left it, then
drops the log. Report only by default. The server reads a value at each sale and repair: no restart. CLIENT
prices the window off the record it got at its login, so a player sees the value at the next login (the
server pays it at once).

    tools/fill_item_values.py [--db eqoa.db] --prices <harvest JSONs or folders> ... [--apply | --undo]
"""
import argparse
import collections
import json
import os
import re
import sqlite3
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from stock_merchants import Measured, Rule, load_patterns, norm  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
GEAR = (1, 19)          # wear slots: head .. hands (handlers/equip.go); 20+ are fishing gear and tools
MIN_LEVEL = 2           # Sandstorm's handouts are levels 0 and 1

LOG_DDL = """CREATE TABLE IF NOT EXISTS item_value_fill_log (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    old       INTEGER NOT NULL,
    new       INTEGER NOT NULL,
    source    TEXT NOT NULL,
    applied   TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def item_ids(s):
    """The pattern ids of a quest's item list ("5877" or "701280*2, 8154": database.ParseItemCounts)."""
    out = set()
    for f in (s or "").split(","):
        f = re.split(r"[*x×]", f.strip(), maxsplit=1)[0].strip()
        if f.isdigit():
            out.add(int(f))
    return out


def json_items(blob):
    """Every "items" list anywhere in a quest field's JSON (byclass, choices, events)."""
    try:
        o = json.loads(blob) if blob else None
    except ValueError:
        return set()
    out = set()

    def walk(v):
        if isinstance(v, dict):
            if isinstance(v.get("items"), str) and v.get("do", "give") == "give":
                out.update(item_ids(v["items"]))
            for w in v.values():
                walk(w)
        elif isinstance(v, list):
            for w in v:
                walk(w)
    walk(o)
    return out


def repeatable_rewards(db):
    """The items a repeatable quest gives: its steps' rewards, class rewards, choices and give events, and
    the quest's own events."""
    if not has_table(db, "quests"):
        return set()
    out = set()
    cols = {r[1] for r in db.execute("PRAGMA table_info(quests)")}
    if "events" in cols:
        for (ev,) in db.execute("SELECT events FROM quests WHERE repeatable = 1"):
            out |= json_items(ev)
    if has_table(db, "queststeps"):
        for reward, byclass, choices, events in db.execute(
                "SELECT s.rewarditems, s.byclass, s.choices, s.events FROM queststeps s JOIN quests q USING (questid) "
                "WHERE q.repeatable = 1"):
            out |= item_ids(reward) | json_items(byclass) | json_items(choices) | json_items(events)
    return out


def gm_patterns(db):
    out = set()
    if not has_table(db, "gm_audit"):
        return out
    for (k,) in db.execute("SELECT rowkey FROM gm_audit WHERE tbl = 'itempattern'"):
        try:
            out.add(int(json.loads(k)["patternid"]))
        except (ValueError, KeyError, TypeError):
            pass
    return out


SUMMON = re.compile(r"\bsumm[ou]ns?\s+(?:item\s*:?\s*)?(?:an?\s+|the\s+)?(.+?)(?:\s+x\s*\d+)?$")
NOT_ITEM = re.compile(r"\b(?:to your aid|to aid|creature|familiar|familar)\b")
OUTLIER_SHARE, OUTLIER_SPAN, OUTLIER_MIN = 0.2, 5, 3


def summoned_names(db):
    """The names of the items a spell summons (entity/potion.go reSummonItem over a spelleffect line's parts,
    and the workbook's items in its effects JSON), as norm() keys them."""
    out = set()
    if not has_table(db, "spelleffect"):
        return out
    cols = {r[1] for r in db.execute("PRAGMA table_info(spelleffect)")}
    for line, eff in db.execute("SELECT line, " + ("effects" if "effects" in cols else "''") + " FROM spelleffect"):
        for part in re.split(r"[;\n]", line or ""):
            p = re.sub(r"\([^)]*\)", "", part).strip().lower()
            m = SUMMON.search(p)
            if m and not NOT_ITEM.search(p):
                out.add(norm(m.group(1)))
        if eff and "item" in eff.lower():
            try:
                o = json.loads(eff)
            except ValueError:
                continue
            stack = [o]
            while stack:
                v = stack.pop()
                if isinstance(v, dict):
                    for k, x in v.items():
                        if k.lower() in ("items", "item") and isinstance(x, list):
                            out.update(norm(i["name"] if isinstance(i, dict) else i) for i in x
                                       if isinstance(i, (dict, str)) and (i.get("name") if isinstance(i, dict) else i))
                        else:
                            stack.append(x)
                elif isinstance(v, list):
                    stack.extend(v)
    return out


def robust_rule(measured):
    """stock_merchants' Rule without the measured items worth less than OUTLIER_SHARE of the median of their
    kind's within OUTLIER_SPAN levels (at least OUTLIER_MIN of them): no measure of a dropped rare."""
    rule = Rule(measured)
    dropped = []
    for k, pool in rule.pool.items():
        keep = []
        for p in pool:
            band = [q["cost"] for q in pool if q is not p and abs(q["level"] - p["level"]) <= OUTLIER_SPAN]
            if len(band) >= OUTLIER_MIN and p["cost"] < OUTLIER_SHARE * statistics.median(band):
                dropped.append(p["name"])
                continue
            keep.append(p)
        rule.pool[k] = keep
    return rule, dropped


def plan(db, measured):
    """[(patternid, name, value, source)] to write and [(patternid, name, why)] left."""
    patterns = load_patterns(db)
    measured.add_db(db, patterns)
    measured.fill_items(patterns)
    rule, _dropped = robust_rule(measured)
    no_rent = {pid for (pid,) in db.execute("SELECT patternid FROM itempattern WHERE rent = 0")}
    summoned = summoned_names(db)
    crafted = {pid for (pid,) in db.execute("SELECT product FROM craftrecipes")} if has_table(db, "craftrecipes") else set()
    repeat = repeatable_rewards(db)
    gm = gm_patterns(db)
    todo, left = [], []
    for pid in sorted(patterns):
        p = patterns[pid]
        if p["cost"] != 0 or not (GEAR[0] <= p["slot"] <= GEAR[1]) or p["level"] < MIN_LEVEL:
            continue
        why = ("NO RENT (a spell's)" if pid in no_rent else "a spell summons it" if norm(p["name"]) in summoned else
               "wear slot 17 (misfiled)" if p["slot"] == 17 else "a crafted product" if pid in crafted else
               "a repeatable quest's reward" if pid in repeat else "a GM changed it" if pid in gm else None)
        if why:
            left.append((pid, p["name"], why))
            continue
        v, src = measured.price_of(p)
        if v == 0:
            left.append((pid, p["name"], "Sandstorm's own value is 0"))
            continue
        if v is None:
            v, how = rule.price(p)
            src = f"the rule ({how})" if v else None
        if not v:
            left.append((pid, p["name"], "nothing to price it by"))
            continue
        todo.append((pid, p["name"], int(v), f"Sandstorm's value ({src})" if src.startswith("measured") else src))
    return todo, left, rule


def apply(db, todo):
    db.execute(LOG_DDL)
    done = 0
    for pid, _name, v, src in todo:
        cur = db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ? AND itemcost = 0", (v, pid))
        if cur.rowcount:
            db.execute("INSERT INTO item_value_fill_log (patternid, old, new, source) VALUES (?, 0, ?, ?)", (pid, v, src))
            done += 1
    return done


def undo(db):
    if not has_table(db, "item_value_fill_log"):
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for pid, old, new in db.execute("SELECT patternid, old, new FROM item_value_fill_log ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ? AND itemcost = ?", (old, pid, new))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE item_value_fill_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them (Sandstorm's values)")
    ap.add_argument("--list", action="store_true", help="print every value written")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the values (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    if not a.prices:
        sys.exit("--prices: Sandstorm's harvests are what the values are measured and fitted on")
    measured = Measured()
    files = measured.load(a.prices)
    todo, left, rule = plan(db, measured)
    print(f"{len(measured.by_name)} measured values from {len(files)} files")
    for k, (n, med, p90) in sorted(rule.errors().items()):
        print(f"  the rule's error on {k}: median {med:.0%}, 90th percentile {p90:.0%} ({n} items, leave one out)")
    by_src = collections.Counter(src.split(" (")[0] for *_x, src in todo)
    print(f"{len(todo)} records valued: " + ", ".join(f"{s}: {n}" for s, n in by_src.most_common()))
    if todo:
        vals = [v for _p, _n, v, _s in todo]
        print(f"  values: median {statistics.median(vals):.0f}, least {min(vals)}, most {max(vals)}")
    for pid, name, v, src in todo:
        if a.list:
            print(f"  {pid} {name!r}: {v} ({src})")
    why = collections.Counter(w for *_x, w in left)
    print(f"{len(left)} left: " + ", ".join(f"{w}: {n}" for w, n in why.most_common()))
    for pid, name, w in left:
        if a.list or w != "Sandstorm's own value is 0":
            print(f"  left: {pid} {name!r}: {w}")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    done = apply(db, todo)
    db.execute("COMMIT")
    print(f"wrote {done} values")


if __name__ == "__main__":
    main()
