#!/usr/bin/env python3
"""Prices for the items that sell for nothing (the owner, 2026-11-15: "yes price them all", after petitions for
a Bloodstone jewel and the Golden Coif selling to merchants for 0 tunar).

A merchant pays an item's value (itempattern.itemcost) at the rate 100 less its wear and sells a catalog item at
its value at the same rate (handlers/merchant.go: sellPrice, merchantPrice, merchantRate): what a merchant stocks
sells and buys back at one price, so a value can never be a profit loop on the item itself. The loops are the
items that come for nothing: a spell's or an ability's conjuring, a merchant's giveaway, a quest done again and
again, a new character's kit. Those stay at 0.

Each record of value 0 is classified, the first that holds:

KEPT at 0 (each counted and listed by its reason):
  copy           an Ascended or imported copy (itemascended, itemimported): the server makes each copy its base at
                 every start (database.SyncAscendedCopies, SyncImportedCopies), so it follows its base's new price
  gm-zeroed      a gm_audit line set its value from a price to 0
  placeholder    a test, GM or placeholder record by its name ("Test Helm 3", "Tester Pro", "... testing kit")
  heirloom       an Heirloom (heirlooms, or an "Heirloom " name): the server's own, given again on request
  familiar       a familiar's call item (familiars)
  token          a token by its name ("10 point faction token", "Token of Odus")
  conjured       NO RENT (rent 0), or norentvanish, or what a spell or an item use makes: a spelleffect's
                 create_item (its patternid or its name), "Summon X" in a spelleffect or itemeffect line. A spell
                 scroll (wear slot -1, ScrollArt) of the same name is the scroll, not the conjured item
  heroes         sold by the Testing Heroes for standing (heroes_stock), which is earned again
  merchant-free  a merchant stocks it at 0 (npcitem), or stock_merchants logged it 'free': pricing it would make
                 the shop charge for what it gives away
  start-kit      a new character's kit (startitems, defaultcharinventory)
  repeatable     a repeatable quest's reward (its steps' rewards, class rewards, choices, give events)
  quest          not gear, and a quest asks for it or drops it for a step, or hands it over (a key, a part, a
                 note; a spell scroll it hands over is priced)
  unobtainable   no loot table, loot pool, merchant, quest, recipe, catch or kit gives it and no one holds it
  no-rule        none of the rules below fits (listed)

PRICED (each counted by its rule; the value is never above what a merchant sells an item of the same name for):
  crafted   a tradeskill product (craftrecipes.product): tools/seed_tradeskills.py's own price for its products,
            3 x its base component's value (the bar, ore or swatch: the costliest one used up) + 10, + MOD_WORTH
            of the recipe's rarity when a modifier (a component kept on a failure) is in it. Return Home's live
            records the seed reused as products kept their 0: the Mithril Bloodstone Earring is 3 x 743 + 10 + 50
            = 2,289, as its Pendant is
  gear      wear slot 0-28, not a stack: the median value of the KNN priced records of the same wear slot nearest
            it, nearness being the level difference over 3 plus the differences of log(1 + AC), log(1 + damage)
            and log(1 + stat points) over 0.35 each (tools/stock_merchants.py's measure, without the name). A
            record of level 0 or 1 that has AC, damage or stats is measured without its level (the Golden Coif is
            level 0 with AC 34). Fewer than MIN_PEERS priced records in its slot: no-rule
  scroll    a spell scroll (the scroll's art and a spell's name; a quest's reward scroll too): the median value of
            the priced scrolls of its level (the nearest levels, up to SCROLL_SPAN away, until MIN_PEERS are found)
  material  a tradeskill component (craftrecipes.ingredients): a modifier (kept on a failure) the seed's tier for
            its rarity (a jeweler's gem GEM_PRICE 20 / 60 / 270 / 800, any other MOD_PRICE 20 / 100 / 400 /
            1,500); a component used up the median value of the priced components of the same crafts and
            rarities (else of the same least rarity)
  drop      anything else a creature drops (a loot table or the loot pool): the median value of the priced trash
            (lootpool 'trash', not gear) of its creatures' level band (10 levels), the creatures' median level

The log table price_zero_1115_log has each record written (patternid, name, old 0, new, rule); every record also
gets its own gm_audit line (tbl itempattern, rowkey {"patternid":N}, before/after its itemcost), so the GM panel
shows and can revert each. Report only by default; --apply writes; a second --apply finds nothing to do; --undo
puts back 0 on each record still at the value this wrote (one changed since is left and said), with a gm_audit
line each, and drops the log. Records are matched by patternid within the database: the shards' ids differ.

WHEN IT TAKES EFFECT: the server reads a value at each sale and repair (no restart). CLIENT shows a value off the
record sent at login, so a player sees the new price in the window at the next login; the server pays it at once.
Copies take their base's price at the next start.

    python3 -E -s tools/price_zero_items_20261115.py --db eqoa.db [--list] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import math
import re
import sqlite3
import statistics
import sys

LOG = "price_zero_1115_log"
WHO, VIA = "dab", "tool"
DETAIL = ("the owner (2026-11-15, \"yes price them all\"; petitions for Bloodstone and the Golden Coif): an item "
          "worth 0 priced by its kind; tools/price_zero_items_20261115.py")
SCROLL_ART = -607649328          # database.ScrollArt: a spell scroll's icon
KNN, MIN_PEERS, SCROLL_SPAN = 5, 3, 5
# tools/seed_tradeskills.py's prices by rarity: a product's share for its modifier, a modifier, a jeweler's gem
MOD_WORTH, MOD_PRICE, GEM_PRICE = [50, 200, 600, 2000], [20, 100, 400, 1500], [20, 60, 270, 800]
JEWEL = 3                         # craftrecipes.skill: the jeweler's
GEAR_SLOTS = range(0, 29)
STAT_COLS = ("str", "sta", "agi", "wis", "dex", "cha", "intelligence", "PR", "DR", "FR", "CR", "LR", "AR")

PLACEHOLDER = re.compile(r"\b(?:test|tester|testing|placeholder|debug|dummy|unused|gm)\b", re.I)
TOKEN = re.compile(r"\btokens?\b", re.I)
SUMMON = re.compile(r"\bsumm[ou]ns?\s+(?:item\s*:?\s*)?(?:an?\s+|the\s+)?(.+?)(?:\s+x\s*\d+)?$")
NOT_ITEM = re.compile(r"\b(?:to your aid|to aid|to your defense|pet|creature|familiar|familar|golem|animation)\b")

KEEP_ORDER = ("copy", "gm-zeroed", "placeholder", "heirloom", "familiar", "token", "conjured", "heroes",
              "merchant-free", "start-kit", "repeatable", "quest", "unobtainable", "no-rule")
PRICE_ORDER = ("crafted", "gear", "scroll", "material", "drop")


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def cols(db, name):
    return {r[1] for r in db.execute(f"PRAGMA table_info({name})")}


def norm(name):
    n = " ".join((name or "").replace("`", "'").split()).lower()
    return n[:-1].rstrip() if n.endswith("$") else n


def item_ids(s):
    """The pattern ids of an item list ("5877" or "701280*2, 8154": database.ParseItemCounts)."""
    out = set()
    for f in str(s or "").split(","):
        f = re.split(r"[*x×]", f.strip(), maxsplit=1)[0].strip()
        if f.isdigit():
            out.add(int(f))
    return out


def json_items(blob, give_only=True):
    """Every "items" list in a quest field's JSON (byclass, choices, events); events only where they give."""
    try:
        o = json.loads(blob) if blob else None
    except ValueError:
        return set()
    out = set()

    def walk(v):
        if isinstance(v, dict):
            if isinstance(v.get("items"), (str, int)) and (not give_only or v.get("do", "give") == "give"):
                out.update(item_ids(v["items"]))
            for w in v.values():
                walk(w)
        elif isinstance(v, list):
            for w in v:
                walk(w)
    walk(o)
    return out


def load(db):
    have = cols(db, "itempattern")
    want = ["patternid", "itemcost", "itemname", "equipslot", "trade", "rent", "levelreq", "maxstack", "itemicon",
            "weapondamage", "AC", "HPMAX", "POWMAX", "PoT", "HoT"] + list(STAT_COLS)
    sel = ", ".join(c if c in have else f"0 AS {c}" for c in want)
    out = {}
    for r in db.execute(f"SELECT {sel} FROM itempattern"):
        p = dict(zip(want, r))
        for k in want[3:]:
            if k != "itemname":
                p[k] = p[k] or 0
        p["stats"] = sum(p[c] for c in STAT_COLS) + (p["HPMAX"] + p["POWMAX"]) / 5 + 5 * (p["PoT"] + p["HoT"])
        out[p["patternid"]] = p
    return out


CLASS_TAG = re.compile(r"\s*\([A-Za-z ,/]+\)\s*$")


def spell_key(name):
    """A spell's or a scroll's name as handlers/scribe.go's spellKey compares them."""
    n = (name or "").strip()
    n = n[:-1].strip() if n.endswith("$") else n
    n = CLASS_TAG.sub("", n).replace("+", "").replace("`", "'")
    return " ".join(n.lower().split())


def is_scroll(p, spells):
    """A spell scroll: unworn, the scroll's art, and its name is a spell's (scribe.go reads a scroll's spell by its
    name). Quest items wear the scroll's art too ("mystical parchment"): those are no scroll."""
    if p["equipslot"] != -1 or p["itemicon"] != SCROLL_ART:
        return False
    k = spell_key(p["itemname"])
    return k in spells or (k.endswith(" scroll") and k[:-7].strip() in spells)


def is_gear(p):
    return p["equipslot"] in GEAR_SLOTS and p["maxstack"] <= 1


class Facts:
    """What the database says of where each record comes from and goes."""

    def __init__(self, db, pats):
        self.db = db
        q = lambda sql, t: {r[0] for r in db.execute(sql)} if table(db, t) else set()
        self.copies = q("SELECT patternid FROM itemascended", "itemascended") | q(
            "SELECT patternid FROM itemimported", "itemimported")
        self.heirlooms = q("SELECT patternid FROM heirlooms", "heirlooms")
        self.familiars = q("SELECT patternid FROM familiars", "familiars")
        self.norentvanish = q("SELECT patternid FROM norentvanish", "norentvanish")
        self.heroes = q("SELECT patternid FROM heroes_stock", "heroes_stock")
        self.stocked = q("SELECT DISTINCT patternid FROM npcitem", "npcitem")
        self.free = q("SELECT key FROM merchant_stock WHERE what = 'free'", "merchant_stock")
        self.start = q("SELECT patternid FROM startitems", "startitems") | q(
            "SELECT patternid FROM defaultcharinventory", "defaultcharinventory")
        self.fish = q("SELECT itemid FROM fishcatch", "fishcatch")
        self.held = q("SELECT DISTINCT patternid FROM charinventory", "charinventory")
        self.spells = {spell_key(r[0]) for r in db.execute("SELECT spellname FROM spellpattern")
                       if r[0]} if table(db, "spellpattern") else set()
        self.gm_zeroed = self._gm_zeroed()
        self.summon_ids, self.summon_names = self._summons()
        self.quest_asks, self.quest_gives, self.repeat_gives = self._quests()
        self.products, self.recipes, self.uses = self._crafts()
        self.drop_levels, self.loot_ids = self._loot()
        # a merchant's price by name (an item of the same name sold elsewhere caps a price)
        self.sold_by_name = {}
        if table(db, "npcitem"):
            for (pid,) in db.execute("SELECT DISTINCT patternid FROM npcitem"):
                p = pats.get(pid)
                if p and p["itemcost"] > 0:
                    k = norm(p["itemname"])
                    self.sold_by_name[k] = min(self.sold_by_name.get(k, p["itemcost"]), p["itemcost"])

    def _gm_zeroed(self):
        out = set()
        if not table(self.db, "gm_audit"):
            return out
        for k, b, a in self.db.execute("SELECT rowkey, before, after FROM gm_audit WHERE tbl = 'itempattern'"):
            try:
                pid = int(json.loads(k)["patternid"])
                bc = json.loads(b).get("itemcost") if b else None
                ac = json.loads(a).get("itemcost") if a else None
            except (ValueError, KeyError, TypeError, AttributeError):
                continue
            if ac == 0 and bc:
                out.add(pid)
            elif ac:
                out.discard(pid)
        return out

    def _summons(self):
        ids, names = set(), set()

        def line_names(line):
            for part in re.split(r"[;,\n]", line or ""):
                p = re.sub(r"\([^)]*\)", "", part).strip().lower()
                m = SUMMON.search(p)
                if m and not NOT_ITEM.search(p):
                    names.add(norm(m.group(1)))
        if table(self.db, "spelleffect"):
            has_eff = "effects" in cols(self.db, "spelleffect")
            for line, eff in self.db.execute("SELECT line, " + ("effects" if has_eff else "''") + " FROM spelleffect"):
                line_names(line)
                if not eff or "item" not in eff:
                    continue
                try:
                    o = json.loads(eff)
                except ValueError:
                    continue
                stack = [o]
                while stack:
                    v = stack.pop()
                    if isinstance(v, dict):
                        if v.get("kind") == "create_item" and isinstance(v.get("item"), dict):
                            it = v["item"]
                            if isinstance(it.get("patternid"), int):
                                ids.add(it["patternid"])
                            if it.get("name"):
                                names.add(norm(it["name"]))
                        stack.extend(v.values())
                    elif isinstance(v, list):
                        stack.extend(v)
        if table(self.db, "itemeffect"):
            for (line,) in self.db.execute("SELECT line FROM itemeffect"):
                line_names(line)
        return ids, names

    def _quests(self):
        asks, gives, repeat = set(), set(), set()
        if not table(self.db, "queststeps"):
            return asks, gives, repeat
        rep = {r[0] for r in self.db.execute("SELECT questid FROM quests WHERE repeatable = 1")} \
            if table(self.db, "quests") else set()
        sc = cols(self.db, "queststeps")
        f = [c if c in sc else "''" for c in ("kind", "items", "rewarditems", "byclass", "choices", "events")]
        for qid, kind, items, reward, byclass, choices, events in self.db.execute(
                f"SELECT questid, {', '.join(f)} FROM queststeps"):
            asks |= item_ids(items)
            g = item_ids(reward) | json_items(byclass) | json_items(choices) | json_items(events)
            asks |= json_items(events, give_only=False) - g
            gives |= g
            if qid in rep:
                repeat |= g
        if table(self.db, "quests") and "events" in cols(self.db, "quests"):
            for qid, ev in self.db.execute("SELECT questid, events FROM quests"):
                g = json_items(ev)
                gives |= g
                if qid in rep:
                    repeat |= g
        return asks, gives, repeat

    def _crafts(self):
        products, recipes, uses = set(), collections.defaultdict(list), collections.defaultdict(list)
        self.modifier, self.used_up = collections.defaultdict(set), set()
        if not table(self.db, "craftrecipes"):
            return products, recipes, uses
        for skill, level, rarity, product, yld, ing in self.db.execute(
                "SELECT skill, level, rarity, product, yield, ingredients FROM craftrecipes"):
            products.add(product)
            try:
                parts = json.loads(ing or "[]")
            except ValueError:
                parts = []
            comps = []
            for i in parts:
                if isinstance(i, dict) and isinstance(i.get("item"), int):
                    comps.append((i["item"], int(i.get("count") or 1), bool(i.get("keep"))))
                    uses[i["item"]].append((skill, rarity, level))
                    if i.get("keep"):
                        self.modifier[i["item"]].add((skill, rarity))
                    else:
                        self.used_up.add(i["item"])
            recipes[product].append((max(1, yld or 1), comps, rarity))
        return products, recipes, uses

    def _loot(self):
        """Each dropped record's creatures' levels, and every record any loot gives."""
        levels = collections.defaultdict(list)
        ids = set()
        name_lv = collections.defaultdict(list)
        row_lv = {}
        if table(self.db, "npcs"):
            for rid, n, lv in self.db.execute("SELECT id, npc_name, npc_level FROM npcs"):
                name_lv[norm(n)].append(lv or 0)
                row_lv[rid] = lv or 0
        med = {n: statistics.median(v) for n, v in name_lv.items()}

        def band(mn, mx):
            return (mn + mx) / 2 if mx and mx < 100 else mn
        if table(self.db, "lootpool"):
            for npc, pid, mn, mx in self.db.execute("SELECT npc_name, patternid, minlevel, maxlevel FROM lootpool"):
                ids.add(pid)
                lv = med.get(norm(npc)) if npc else None
                levels[pid].append(lv if lv is not None else band(mn, mx))
        if table(self.db, "loottables"):
            holders = collections.defaultdict(list)
            if table(self.db, "npcloot"):
                for npc, t in self.db.execute("SELECT npc_name, tablename FROM npcloot"):
                    holders[t].append(npc)
            for t, pid, mn, mx in self.db.execute("SELECT tablename, patternid, minlevel, maxlevel FROM loottables"):
                ids.add(pid)
                got = []
                for npc in holders.get(t, []):
                    if npc.startswith("#") and npc[1:].isdigit():
                        lv = row_lv.get(int(npc[1:]))
                    else:
                        lv = med.get(norm(npc))
                    if lv is not None:
                        got.append(lv)
                levels[pid].extend(got or [band(mn, mx)])
        return levels, ids

    def held_only(self, pid):
        return pid in self.held and not (pid in self.loot_ids or pid in self.stocked or pid in self.quest_gives or
                                         pid in self.products or pid in self.fish or pid in self.start or
                                         pid in self.heroes)

    def obtainable(self, pid):
        return (pid in self.loot_ids or pid in self.stocked or pid in self.quest_gives or pid in self.products
                or pid in self.fish or pid in self.start or pid in self.held or pid in self.heroes)


def dist(a, b, use_level):
    d = abs(math.log1p(max(a["AC"], 0)) - math.log1p(max(b["AC"], 0))) / 0.35
    d += abs(math.log1p(max(a["weapondamage"], 0)) - math.log1p(max(b["weapondamage"], 0))) / 0.35
    d += abs(math.log1p(max(a["stats"], 0)) - math.log1p(max(b["stats"], 0))) / 0.35
    if use_level:
        d += abs(a["levelreq"] - b["levelreq"]) / 3
    return d


class Rules:
    def __init__(self, pats, facts):
        self.facts = facts
        ok = lambda p: (p["itemcost"] > 0 and p["patternid"] not in facts.copies and p["rent"] != 0
                        and p["patternid"] not in facts.heirlooms and not PLACEHOLDER.search(p["itemname"] or "")
                        and not (p["itemname"] or "").startswith("Heirloom "))
        self.priced = [p for p in pats.values() if ok(p)]
        self.by_slot = collections.defaultdict(list)
        self.scroll_lv = collections.defaultdict(list)
        for p in self.priced:
            if is_gear(p):
                self.by_slot[p["equipslot"]].append(p)
            elif is_scroll(p, facts.spells):
                self.scroll_lv[p["levelreq"]].append(p["itemcost"])
        # components priced, by (crafts, rarities) and by rarity
        self.mat_key, self.mat_low = collections.defaultdict(list), collections.defaultdict(list)
        for p in self.priced:
            u = facts.uses.get(p["patternid"])
            if u and not is_gear(p):
                self.mat_key[self._mkey(u)].append(p["itemcost"])
                self.mat_low[(self._mkey(u)[0], min(r for _s, r, _l in u))].append(p["itemcost"])
        # the trash of each 10-level band
        self.trash = collections.defaultdict(list)
        if table(facts.db, "lootpool"):
            cat = collections.defaultdict(set)
            for c, pid in facts.db.execute("SELECT category, patternid FROM lootpool"):
                cat[pid].add(c)
            for p in self.priced:
                lv = facts.drop_levels.get(p["patternid"])
                if lv and "trash" in cat.get(p["patternid"], ()) and not is_gear(p) and not is_scroll(p, facts.spells):
                    self.trash[int(statistics.median(lv)) // 10].append(p["itemcost"])

    @staticmethod
    def _mkey(u):
        return (tuple(sorted({s for s, _r, _l in u})), tuple(sorted({r for _s, r, _l in u})))

    def gear(self, p):
        pool = [q for q in self.by_slot.get(p["equipslot"], []) if q["patternid"] != p["patternid"]]
        if len(pool) < MIN_PEERS:
            return None, f"{len(pool)} priced records in wear slot {p['equipslot']}"
        use_level = not (p["levelreq"] <= 1 and (p["AC"] > 0 or p["weapondamage"] > 0 or p["stats"] > 0))
        near = sorted(pool, key=lambda q: (dist(p, q, use_level), q["patternid"]))[:KNN]
        v = int(round(statistics.median(q["itemcost"] for q in near)))
        how = f"slot {p['equipslot']} median of {', '.join(str(q['patternid']) for q in near)}"
        return v, how + ("" if use_level else " (level 0-1 with figures: level not measured)")

    def scroll(self, p):
        lv = p["levelreq"]
        got = []
        for span in range(SCROLL_SPAN + 1):
            got = self.scroll_lv.get(lv, []) + [c for d in range(1, span + 1) for c in
                                                self.scroll_lv.get(lv - d, []) + self.scroll_lv.get(lv + d, [])]
            if len(got) >= MIN_PEERS:
                return int(round(statistics.median(got))), f"scrolls of level {lv}±{span} ({len(got)})"
        return None, f"{len(got)} priced scrolls within {SCROLL_SPAN} levels of {lv}"

    def material(self, p):
        pid = p["patternid"]
        if pid in self.facts.modifier and pid not in self.facts.used_up:
            # a modifier (kept on a failure): tools/seed_tradeskills.py's tier for its rarity, a jeweler's gem's
            # GEM_PRICE, any other's MOD_PRICE
            mods = self.facts.modifier[pid]
            lo = min(r for _s, r in mods)
            lo = min(max(lo, 0), 3)
            if all(sk == JEWEL for sk, _r in mods):
                return GEM_PRICE[lo], f"a jeweler's gem of rarity {lo} (GEM_PRICE)"
            return MOD_PRICE[lo], f"a modifier of rarity {lo} (MOD_PRICE)"
        u = self.facts.uses[pid]
        k = self._mkey(u)
        vals = self.mat_key.get(k, [])
        if len(vals) >= 2:
            return int(round(statistics.median(vals))), f"components of crafts {list(k[0])} rarity {list(k[1])} ({len(vals)})"
        lo = min(r for _s, r, _l in u)
        for crafts in (k[0], None):
            vals = [c for (cr, ra), cs in self.mat_low.items() if ra == lo and (crafts is None or cr == crafts)
                    for c in cs]
            if len(vals) >= (2 if crafts else MIN_PEERS):
                where = f"crafts {list(crafts)}" if crafts else "any craft"
                return int(round(statistics.median(vals))), f"components of {where}, least rarity {lo} ({len(vals)})"
        return None, "no priced component of its kind"

    def drop(self, p):
        lv = self.facts.drop_levels.get(p["patternid"])
        if not lv:
            return None, "no creature level"
        b = int(statistics.median(lv)) // 10
        for d in (0, -1, 1):
            vals = self.trash.get(b + d, [])
            if len(vals) >= MIN_PEERS:
                return int(round(statistics.median(vals))), f"trash of levels {10 * (b + d)}-{10 * (b + d) + 9} ({len(vals)})"
        return None, f"no priced trash near levels {10 * b}-{10 * b + 9}"

    def crafted(self, p, pats):
        """tools/seed_tradeskills.py's price for its products: 3 x the base component's (the bar, ore or swatch: the
        costliest component used up) + 10, + MOD_WORTH of the recipe's rarity when a modifier (the component
        kept on a failure: the gem, the alloy) is in it; the least over its recipes."""
        best = None
        for yld, comps, rarity in self.facts.recipes.get(p["patternid"], []):
            used = [pats[c]["itemcost"] for c, _n, keep in comps if not keep and c in pats]
            if not used or max(used) <= 0:
                continue
            mod = any(keep for _c, _n, keep in comps)
            v = (3 * max(used) + 10 + (MOD_WORTH[min(max(rarity, 0), 3)] if mod else 0)) // yld
            best = v if best is None else min(best, v)
        if best is None:
            return None, "no priced base component"
        return best, "tradeskill seed's product rule (3 x base component + 10 + modifier's worth)"


def classify(db):
    """[(patternid, name, rule, value, how)] priced and [(patternid, name, reason, how)] kept."""
    pats = load(db)
    f = Facts(db, pats)
    rules = Rules(pats, f)
    priced, kept = [], []
    for pid in sorted(pats):
        p = pats[pid]
        if p["itemcost"] != 0:
            continue
        name = p["itemname"] or ""
        n = norm(name)
        gear, scroll = is_gear(p), is_scroll(p, f.spells)

        def keep(reason, how=""):
            kept.append((pid, name, reason, how))
        if pid in f.copies:
            keep("copy", "follows its base at the next start")
        elif pid in f.gm_zeroed:
            keep("gm-zeroed", "a gm_audit line set it to 0")
        elif PLACEHOLDER.search(name):
            keep("placeholder", "its name")
        elif pid in f.heirlooms or name.startswith("Heirloom "):
            keep("heirloom")
        elif pid in f.familiars:
            keep("familiar")
        elif TOKEN.search(name) and not gear:
            keep("token", "its name")
        elif p["rent"] == 0 or pid in f.norentvanish or pid in f.summon_ids or (n in f.summon_names and not scroll):
            keep("conjured", "NO RENT" if p["rent"] == 0 else "norentvanish" if pid in f.norentvanish else
                 "a spell's create_item" if pid in f.summon_ids else "a Summon line names it")
        elif pid in f.heroes:
            keep("heroes", "bought with standing")
        elif pid in f.stocked or pid in f.free:
            keep("merchant-free", "a merchant stocks it at 0")
        elif pid in f.start:
            keep("start-kit")
        elif pid in f.repeat_gives:
            keep("repeatable", "a repeatable quest gives it")
        elif not gear and (pid in f.quest_asks or (pid in f.quest_gives and not scroll)):
            keep("quest", "asked for" if pid in f.quest_asks else "a quest gives it")
        elif not f.obtainable(pid):
            keep("unobtainable", "no source and no one holds it")
        else:
            v = None
            if pid in f.products:
                rule, (v, how) = "crafted", rules.crafted(p, pats)
            if v:
                pass
            elif gear:
                rule, (v, how) = "gear", rules.gear(p)
            elif scroll:
                rule, (v, how) = "scroll", rules.scroll(p)
            elif pid in f.uses:
                rule, (v, how) = "material", rules.material(p)
            elif pid in f.drop_levels:
                rule, (v, how) = "drop", rules.drop(p)
            else:
                rule, v, how = None, None, "not gear, a scroll, a component or a drop"
            if not v:
                keep("no-rule", f"{rule}: {how}" if rule else how)
                continue
            sold = f.sold_by_name.get(n)
            if sold is not None and sold < v:
                v, how = sold, f"{how}; held to {sold}, what a merchant sells one of its name for"
            if f.held_only(pid):
                how += "; only a player's holding shows it can be had"
            priced.append((pid, name, rule, int(v), how))
    return priced, kept


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True)


def audit(db, pid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'itempattern', ?, ?, ?, ?)",
                     (now, acct[0] if acct else 0, WHO, VIA, go_json({"patternid": pid}),
                      go_json({"itemcost": before}), go_json({"itemcost": after}), detail))
    return cur.lastrowid


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


def do_apply(db, priced):
    db.execute(LOG_DDL)
    done, ids = 0, []
    for pid, name, rule, v, how in priced:
        cur = db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ? AND itemcost = 0", (v, pid))
        if cur.rowcount != 1:
            continue
        a = audit(db, pid, 0, v, f"{DETAIL}: {rule}, {how}"[:900])
        db.execute(f"INSERT INTO {LOG} (patternid, itemname, old, new, rule, how, audit) VALUES (?, ?, 0, ?, ?, ?, ?)",
                   (pid, name, v, rule, how, a))
        ids.append(a)
        done += 1
    return done, ids


def do_undo(db):
    if not table(db, LOG):
        print("nothing to undo: no log")
        return 0
    back = kept = 0
    ids = []
    for seq, pid, old, new in db.execute(f"SELECT seq, patternid, old, new FROM {LOG} ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ? AND itemcost = ?", (old, pid, new))
        if cur.rowcount != 1:
            kept += 1
            print(f"  left: {pid} is no longer at {new}")
            continue
        ids.append(audit(db, pid, new, old, "undo: " + DETAIL))
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if kept == 0:
        db.execute(f"DROP TABLE {LOG}")
    print(f"put back {back}; left {kept} changed since" + (f"; gm_audit {min(ids)}-{max(ids)}" if ids else ""))
    return 0 if kept == 0 else 1


def report(priced, kept, show_all):
    total = len(priced) + len(kept)
    print(f"{total} records at value 0: {len(priced)} to price, {len(kept)} kept at 0")
    kc = collections.Counter(r for _p, _n, r, _h in kept)
    print("kept at 0, by reason:")
    for r in KEEP_ORDER:
        if kc[r]:
            sample = [f"{p} {n!r}" for p, n, rr, _h in kept if rr == r][:4]
            print(f"  {r:14} {kc[r]:5}   e.g. {'; '.join(sample)}")
    pc = collections.Counter(r for _p, _n, r, _v, _h in priced)
    print("priced, by rule:")
    for r in PRICE_ORDER:
        if pc[r]:
            vals = [v for _p, _n, rr, v, _h in priced if rr == r]
            sample = [f"{p} {n!r} {v}" for p, n, rr, v, _h in priced if rr == r][:4]
            print(f"  {r:9} {pc[r]:5}   values median {statistics.median(vals):.0f}, {min(vals)}-{max(vals)}; "
                  f"e.g. {'; '.join(sample)}")
    capped = [x for x in priced if "held to" in x[4]]
    if capped:
        print(f"  {len(capped)} held to a merchant's price")
    held = [x for x in priced if "only a player's holding" in x[4]]
    if held:
        print(f"  {len(held)} known only from a player's holding (a GM's gift, a transfer): "
              + "; ".join(f"{p} {n!r} {v}" for p, n, _r, v, _h in held[:8]))
    for pid, name, rule, v, how in priced:
        if show_all:
            print(f"  price {pid} {name!r}: {v} ({rule}: {how})")
    for pid, name, r, how in kept:
        if show_all or r == "no-rule":
            print(f"  keep {pid} {name!r}: {r}" + (f" ({how})" if how else ""))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--list", action="store_true", help="print every record, priced and kept")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the prices (logged, audited)")
    g.add_argument("--undo", action="store_true", help="put back what --apply wrote")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 10000")
    try:
        if not table(db, "itempattern") or not table(db, "gm_audit"):
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
        if not a.apply:
            priced, kept = classify(db)
            report(priced, kept, a.list)
            print("report only: --apply to write")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            priced, kept = classify(db)
            report(priced, kept, a.list)
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
