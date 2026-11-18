#!/usr/bin/env python3
"""Crafted products valued below what their merchant-bought ingredients cost (the owner, 2026-11-18: "the crafting gear
can be used to generate positive tunar without negative").

THE LOOP. A merchant sells an item and buys one back at its value (itempattern.itemcost; handlers/merchant.go:
merchantPrice and sellPrice at merchantRate 100, no CHA, faction or merchant bonus; a new product is at full
condition). tools/seed_tradeskills.py and price_zero_items_20261115.py priced a product 3 x its base component's
value + 10 (+ a modifier's worth): with every ingredient bought from a merchant, each craft sold back was profit.

WHAT A CRAFT COSTS (handlers/craft.go attempt, craftrules.go): a success takes EVERY ingredient, the kept-on-failure
ones too (craftNeeds(rec, false)); a failure takes the others (craftNeeds(rec, true)); each attempt wears the tool
10 condition (craftToolWearHP), which a blacksmith mends for value x 10 / maxhp (blacksmith.go repairCost); the roll
is at most 0.98 (craftChanceMax); the cooldown 1 s a session (craftCooldown). So at best a product costs
    C = (sum of all ingredients + sum of the lost-on-failure ones x (1 - 0.98) / 0.98 + tool wear / 0.98) / yield
and at most 3,600 attempts an hour make 3,528 products. An ingredient's cost is its value where a merchant stocks it
(npcitem); a recipe with any ingredient no merchant sells is FARMING (loot goes into it) and is left alone.

THE FIX (by values, crafting untouched): a bought recipe's product is lowered to
    floor(FACTOR x (sum of its ingredients' merchant values, all of them) / yield),  FACTOR 0.75,
never raised: a crafter who vendors what they make gets back three quarters of the ingredients (less with failures
and wear), so crafting for XP stays a tunar sink and never a source. A product several bought recipes make takes the
least. Products that also drop from creatures, or a quest hands over, are listed apart (the owner's to decide: their
drop value falls too), and are lowered unless --keep-loot-products.

Each change is logged in craft_values_1118_log (patternid, name, old, new, recipe, how, gm_audit id) with its own
gm_audit line; --apply a second time does nothing; --undo puts back each value still as written, then drops the log.
Report only by default. Takes effect at once (the server reads a value at each sale; CLIENT's window at the next
login).

THE BUY-BACK (handlers/buyback.go, database/buyback.go) is no loop: an entry is priced what the merchant paid (a sale:
n x sellPrice at that condition) or would have paid (a destroy: merchantWouldPay), it comes back with its condition
and charges, and resold it fetches sellPrice again at the same condition: the same tunar, unless the item's value was
raised in between (a GM's or a tool's change: a one-time difference on what the player already had). Mending before
the resale loses: worn, value x (1 + c) / 2; mended, value x (1 - c) paid then value: net value x c.

    python3 -E -s tools/craft_values_20261118.py --db eqoa.db [--list] [--factor 0.75] [--keep-loot-products] [--apply | --undo]
"""
import argparse
import collections
import json
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import price_zero_items_20261115 as pz  # noqa: E402

LOG = "craft_values_1118_log"
DETAIL = ("the owner (2026-11-18, \"the crafting gear can be used to generate positive tunar without negative\"): a "
          "product bought ingredients make held below their cost; tools/craft_values_20261118.py")
FACTOR = 0.75
CHANCE_MAX = 0.98          # handlers/craftrules.go craftChanceMax
TOOL_WEAR = 10             # craftToolWearHP
ATTEMPTS_PER_HOUR = 3600   # craftCooldown 1 s
TOOL_SLOT0 = 22            # handlers/craftitems.go wearCraftTool0


def tool_tier(levelreq):
    return max(levelreq, 0) // 5 + 1


class Craft:
    def __init__(self, db):
        self.db = db
        self.pat = {}
        for pid, name, cost, slot, lv, mhp in db.execute(
                "SELECT patternid, itemname, itemcost, equipslot, levelreq, maxhp FROM itempattern"):
            self.pat[pid] = dict(name=name or "", cost=cost or 0, slot=slot, level=lv or 0, maxhp=mhp or 0)
        self.stocked = {r[0] for r in db.execute("SELECT DISTINCT patternid FROM npcitem")} \
            if pz.table(db, "npcitem") else set()
        # the cheapest wear an attempt costs, by skill and recipe level: a stocked tool that fits
        self.tools = collections.defaultdict(list)
        for pid in self.stocked:
            p = self.pat.get(pid)
            if p and TOOL_SLOT0 <= (p["slot"] or 0) < TOOL_SLOT0 + 6:
                wear = 0.0 if p["maxhp"] <= 0 else p["cost"] * TOOL_WEAR / p["maxhp"]
                self.tools[p["slot"] - TOOL_SLOT0].append((tool_tier(p["level"]), wear, pid))
        self.loot = set()
        for t in ("lootpool", "loottables"):
            if pz.table(db, t):
                self.loot |= {r[0] for r in db.execute(f"SELECT DISTINCT patternid FROM {t}")}
        if pz.table(db, "queststeps"):
            for (items,) in db.execute("SELECT items FROM queststeps WHERE kind = 'loot'"):
                self.loot |= pz.item_ids(str(items or "").split(",")[0])
        self.quest_gives = set()
        if pz.table(db, "queststeps"):
            sc = pz.cols(db, "queststeps")
            fl = [c if c in sc else "''" for c in ("rewarditems", "byclass", "choices", "events")]
            for rw, bc, ch, ev in db.execute(f"SELECT {', '.join(fl)} FROM queststeps"):
                self.quest_gives |= pz.item_ids(rw) | pz.json_items(bc) | pz.json_items(ch) | pz.json_items(ev)

    def tool_wear(self, skill, level):
        fit = [w for tier, w, _p in self.tools.get(skill, []) if tier >= level]
        return min(fit) if fit else None

    def recipes(self):
        """Each recipe: (id, name, skill, level, product, yield, [(item, count, keep)], bought?)."""
        out = []
        for rid, name, skill, level, product, yld, ing in self.db.execute(
                "SELECT id, name, skill, level, product, yield, ingredients FROM craftrecipes ORDER BY id"):
            try:
                parts = json.loads(ing or "[]")
            except ValueError:
                parts = []
            comps = [(int(i["item"]), int(i.get("count") or 1), bool(i.get("keep"))) for i in parts
                     if isinstance(i, dict) and isinstance(i.get("item"), int)]
            out.append(dict(id=rid, name=name, skill=skill, level=level, product=product, yld=max(1, yld or 1),
                            comps=comps, bought=bool(comps) and all(c in self.stocked for c, _n, _k in comps)))
        return out

    def economics(self, r):
        """Best-case cost of one product, what it sells for, the margin a product and an hour."""
        allc = sum(self.pat.get(c, {}).get("cost", 0) * n for c, n, _k in r["comps"])
        lost = sum(self.pat.get(c, {}).get("cost", 0) * n for c, n, k in r["comps"] if not k)
        wear = self.tool_wear(r["skill"], r["level"]) or 0.0
        q = CHANCE_MAX
        cost = (allc + lost * (1 - q) / q + wear / q) / r["yld"]
        sale = self.pat.get(r["product"], {}).get("cost", 0)
        margin = sale - cost
        per_hour = margin * ATTEMPTS_PER_HOUR * q * r["yld"]
        lost_only = (lost + wear) / r["yld"]   # the coordinator's reading: kept items not consumed
        return dict(all=allc, lost=lost, wear=wear, cost=cost, sale=sale, margin=margin, per_hour=per_hour,
                    margin_keepfree=sale - lost_only)


def plan(db, factor, keep_loot):
    c = Craft(db)
    rows, farming = [], 0
    target = {}
    for r in c.recipes():
        if not r["bought"]:
            farming += 1
            continue
        e = c.economics(r)
        rows.append((r, e))
        cap = int(factor * e["all"] / r["yld"])
        prev = target.get(r["product"])
        if prev is None or cap < prev[0]:
            target[r["product"]] = (cap, r)
    changes, held = [], []
    for pid, (cap, r) in sorted(target.items()):
        p = c.pat.get(pid)
        if not p or p["cost"] <= cap:
            continue
        also = []
        if pid in c.loot:
            also.append("a creature drops it")
        if pid in c.quest_gives:
            also.append("a quest hands it over")
        if pid in c.stocked:
            also.append("a merchant stocks it")
        how = (f"recipe {r['id']} {r['name']!r}: {factor} x ingredients {c.economics(r)['all']} / yield {r['yld']}"
               + (f" ({'; '.join(also)})" if also else ""))
        item = (pid, p["name"], p["cost"], cap, r["id"], how, bool(also and ("a creature drops it" in also or
                                                                           "a quest hands it over" in also)))
        if item[6] and keep_loot:
            held.append(item)
        else:
            changes.append(item)
    return c, rows, farming, changes, held


def report(c, rows, farming, changes, held, factor, show_all):
    prof = [(r, e) for r, e in rows if e["margin"] > 0]
    prof_kf = [(r, e) for r, e in rows if e["margin_keepfree"] > 0]
    print(f"{len(rows) + farming} recipes: {len(rows)} made from merchant-bought ingredients alone, {farming} with an "
          f"ingredient no merchant sells (farming: left alone)")
    print(f"bought recipes whose product sells for more than it costs (best case: chance {CHANCE_MAX}, tool wear, "
          f"every ingredient consumed on a success as craft.go does): {len(prof)}"
          f"; if kept-on-failure items were free: {len(prof_kf)}")
    by_skill = collections.Counter(r["skill"] for r, _e in prof)
    print("  by skill (0 weapon, 1 armor, 2 tailoring, 3 jewel): " + ", ".join(f"{k}: {v}" for k, v in sorted(by_skill.items())))
    worst = sorted(prof, key=lambda x: -x[1]["per_hour"])
    print("  worst, tunar an hour at 3,600 attempts:")
    for r, e in worst[:12]:
        print(f"    recipe {r['id']} {r['name']!r} -> {r['product']} x{r['yld']}: costs {e['cost']:.1f}, sells "
              f"{e['sale']}, margin {e['margin']:.1f} ({e['per_hour']:,.0f}/h)")
    if show_all:
        for r, e in sorted(prof, key=lambda x: x[0]["id"]):
            print(f"  profit recipe {r['id']} {r['name']!r} -> {r['product']} x{r['yld']}: ingredients {e['all']} "
                  f"(lost on failure {e['lost']}), wear {e['wear']:.2f}, costs {e['cost']:.1f}, sells {e['sale']}, "
                  f"margin {e['margin']:.1f}, {e['per_hour']:,.0f}/h")
    print(f"fix at factor {factor}: {len(changes)} products lowered"
          + (f", {len(held)} held back (they drop or a quest gives them; --keep-loot-products)" if held else ""))
    alsol = [x for x in changes if "a creature drops it" in x[5] or "a quest hands it over" in x[5]]
    print(f"  of them {len(alsol)} also drop from creatures or come from a quest (their drop value falls too)")
    for x in sorted(changes, key=lambda x: x[3] - x[2])[:10]:
        print(f"    {x[0]} {x[1]!r}: {x[2]} -> {x[3]} ({x[5]})")
    if show_all:
        for x in changes:
            print(f"  lower {x[0]} {x[1]!r}: {x[2]} -> {x[3]} ({x[5]})")
        for x in held:
            print(f"  held {x[0]} {x[1]!r}: {x[2]} (would be {x[3]}; {x[5]})")


LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    patternid INTEGER NOT NULL,
    itemname  TEXT NOT NULL,
    old       INTEGER NOT NULL,
    new       INTEGER NOT NULL,
    recipe    INTEGER NOT NULL,
    how       TEXT NOT NULL,
    audit     INTEGER NOT NULL DEFAULT 0,
    applied   TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def do_apply(db, changes):
    db.execute(LOG_DDL)
    done, ids = 0, []
    for pid, name, old, new, rid, how, _also in changes:
        cur = db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ? AND itemcost = ?", (new, pid, old))
        if cur.rowcount != 1:
            continue
        a = pz.audit(db, pid, old, new, f"{DETAIL}: {how}"[:900])
        db.execute(f"INSERT INTO {LOG} (patternid, itemname, old, new, recipe, how, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (pid, name, old, new, rid, how, a))
        ids.append(a)
        done += 1
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


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--factor", type=float, default=FACTOR)
    ap.add_argument("--keep-loot-products", action="store_true",
                    help="leave products that creatures drop or quests give at their value (listed)")
    ap.add_argument("--list", action="store_true")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    if not 0 < a.factor < 1:
        ap.error("--factor must be above 0 and below 1")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 10000")
    try:
        for t in ("itempattern", "gm_audit", "craftrecipes"):
            if not pz.table(db, t):
                print(f"refused, nothing done: no {t} table here")
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
        if not a.apply:
            report(*plan(db, a.factor, a.keep_loot_products), a.factor, a.list)
            print("report only: --apply to write")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            c, rows, farming, changes, held = plan(db, a.factor, a.keep_loot_products)
            report(c, rows, farming, changes, held, a.factor, a.list)
            done, ids = do_apply(db, changes)
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
