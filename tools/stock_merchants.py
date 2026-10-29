#!/usr/bin/env python3
"""Merchants that sell nothing get a catalog, and every catalog item a price
(the user, 2026-10-29: "most of the shops aren't populated with items").

A merchant is an NPC with the merchant flag (npc_type & 1): CLIENT opens the
merchant window on it (handlers/merchant.go), and the window shows npcitem's
rows keyed by its npc_id (GetNPCItems). Only a merchant with no catalog is
stocked; a catalog already there (a GM's, Sandstorm's, another tool's) stays.

Who sells what, the first that gives one:

0. Sandstorm's own catalog: a merchant a --prices harvest opened (the 10-21
   harvest's, a faction survey's: every `eqoa harvest` JSON with a merchants
   list) is matched to the nearest NPC of ours of its name in its world within
   MATCH_RADIUS (else one whose name is a letter off, as
   import_sandstorm_harvest.py matches them), and its wares, in Sandstorm's
   order, to our patterns: the ware's own id under its name, else its name,
   else its id at its wear slot and level (a renamed item). A ware this
   database lacks is listed, never made: the catalog is what is here of it.
   One with none of its wares here goes on to the next tier.
1. Return Home's catalog (--prh, jib's export): import_prh_content.py ran
   --no-catalogs, as no price was known; its own map of Return Home's rows to
   ours (prh_import_npcs) and of its patterns (prh_import_ids) gives each of
   our merchants the catalog Return Home holds for it. Return Home's loot
   lists (creatures, no merchant flag here) are not merchants and stay out.
2. A catalog of its trade: the trade is a word of the name (Smithy,
   Weaponsmith: weapons; Armorer, Armorsmith: chain and banded; Tailor,
   Seamster: cloth and leather; Bowyer, Bowsmith; Shieldsmith; Grocer, Chef:
   food and drink; Barkeep, Bartender, Innkeeper: drink; Chemist, Alchemist:
   the alchemist's scrolls; Fisherman: what the pole merchants and
   tacklemasters sell). The catalog is one a merchant of the trade sells here
   (after 1.; a catalog that is not the trade's goods, TRADE_KINDS, is none),
   chosen as a town's (merchants within TOWN_LINK of each other) merchants of
   a trade sell its tiers between them: Sandstorm's Freeport tailors sell
   cloth, rawhide, wool, studded and gossamer. So: of the catalogs within
   TIER_REACH levels of the region's (the median level of the creatures round
   the merchant), one the town sells the fewest of, then the nearest the
   region's level, then one held in the same world, then the nearest holder's;
   past them the rest by the same. A spell vendor's, a grocer's and a
   fisherman's take the one the town lacks whatever its level: a town's three
   chemists sell the alchemist's three lists, as Sandstorm's do.
3. A jeweler (no merchant anywhere sells jewelry): jewelry out of itempattern
   (ear, neck, wrist, finger, waist; tradeable, not lore, a metal's or a gem's
   name), the region's level and below, a few of each kind.
4. General goods, for a merchant of no trade ("Merchant ...", "... shopkeeper",
   a named NPC): the supplies the live game's supply merchants sell in Return
   Home's rows (GENERAL_GOODS).

Left out, listed in the report and logged (what 'left out'): an NPC that is
no shop by its name (LEFT_OUT: Keepers, which EQOA-Data has as quest givers
and Return Home gives no merchant flag; master craftsmen; storytellers;
citizens; guards; class and guild NPCs), a creature (a lowercase name with no
shop word: a flag on a beast is a mistake), and one that a --prices harvest
shows on Sandstorm without the merchant flag.

Prices, for every catalog item whose itemcost is 0 (a nonzero one is never
changed, Sandstorm's, a GM's or another tool's, but for the two exceptions
below):

- Measured: an item of the same name (case, a backtick and spacing aside)
  that Sandstorm prices: a --prices file (an `eqoa harvest` JSON: its
  merchants' wares and its characters' inventories, each with the item's
  value; or a TSV/CSV with a name column and a price or value column), and
  the patterns import_sandstorm_harvest.py wrote (sandstorm_import_rows). A
  name with no measured price falls back to the same id when the harvest
  holds that id at the same wear slot and level (a renamed item). Sandstorm
  sells some items at 0 (a starting quest's handout): those stay 0, logged
  ('free').
- Otherwise ours, by a rule fitted on the measured items, printed with its
  error (leave-one-out over the measured items):
    - a weapon without stats: a x its damage (Sandstorm's vendor weapons are
      1.83 x damage within a tunar or two);
    - anything else: the median of the KNN nearest measured items of its
      kind (weapon, armor and shield, jewelry, ammunition, scroll, meal,
      drink, tool, other), nearness being the level difference over 3, plus
      1 less the share of the names' words they have in common, plus the
      difference of log(1 + AC), log(1 + damage) and log(1 + stat points)
      over 0.35 each, plus 0.5 for another wear slot.
    - a scroll: Sandstorm's scroll curve, the measured scrolls' price at its
      level, the straight line between two levels measured, and above the
      highest the square of the level (as import_eqoadata_spells.py scales
      one; within a tunar of the curve's own quadratic).
  A new harvest refits it: --prices with the harvest's files, after --undo.
  The report also names the catalog items that keep a price of their own
  (not Sandstorm's) ODD times or more off the rule's, for a GM to look at.
- The two exceptions (the coordinator's, 2026-10-29), each narrow and logged
  as any price is (undone with the rest), never on a pattern a GM edited or
  one import_sandstorm_harvest.py wrote:
    - a grocer's price stock_grocers.py set (grocer_stock: ours, 10 tunar a
      level) for an item Sandstorm's merchants sell: Sandstorm's value;
    - a scroll import_eqoadata_spells.py made (eqoadata_spell_import_rows)
      at 1 tunar (it scaled a reference scroll's 0): the scroll curve's.

Every change is logged in merchant_stock (what it was and what it became) and
--undo puts it all back but a row changed since (a GM's edit: left alone and
named); the md5 of the touched tables' rows taken before --apply is checked
after --undo. A second --apply is refused while merchant_stock is there. Both
take a copy of the database first (<db>.bak-<when>-pre-merchants). Report only
by default. Restart the server after --apply or --undo: catalogs and prices
are read as they are needed, but the NPCs' npc_id at its start.

Sandstorm's catalogs come from this tool, not import_sandstorm_harvest.py
--all-merchants: that one makes the items a catalog lacks (and renumbers an
id held by another name) and rewrites every price it meets with the
harvest's, the grocers' too.

    tools/stock_merchants.py --prh prh-export.db [--db eqoa.db]
        [--prices HARVEST.json|HARVEST_DIR|PRICES.tsv ...] [--apply | --undo]

A directory is read for its *.json (a survey's .tsv files are faction and
stop lists, not prices); a .tsv or .csv named is a price list.
"""
import argparse
import collections
import csv
import datetime
import glob
import hashlib
import json
import math
import os
import re
import sqlite3
import statistics
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")

MERCHANT, NPC_BIT = 0x01, 0x80
LIST_CAP = 40            # CLIENT's item list (handlers' invListCap)
SCROLL_ICON = -607649328
TOWN_LINK = 300.0        # merchants this near each other are one town
MATCH_RADIUS = 30.0      # a harvested merchant and our nearest NPC of its name this close are one
NEAR_RADIUS = 30.0       # ... and one whose name is a letter off, this close
REGION_K = 25            # the creatures whose median level is the region's
REGION_RADIUS = 3000.0
REGION_DEFAULT = 20      # a merchant with no creature within the radius
LEVEL_STEP = 5           # levels this close fit alike
TIER_REACH = 25          # a catalog this near the region's level is one of the town's tiers
KNN = 3
ODD = 4                  # a price kept that is this many times off the rule's is named
STOPWORDS = {"of", "the", "and"}
JEWELS_PER_SLOT = 3
JEWELS_AT_LEAST = 8

MEAL = re.compile(r"(?i)\bmeal\b.*\bhealth regeneration\b")
DRINK = re.compile(r"(?i)\bbeverage\b.*\bpower regeneration\b")

# A word of the name -> the trade. The first word that names one decides.
TRADES = {
    "smithy": "weapons", "smith": "weapons", "weaponsmith": "weapons",
    "armorer": "armor", "armorsmith": "armor",
    "tailor": "tailor", "seamster": "tailor", "seamstress": "tailor", "seamsteress": "tailor",
    "bowyer": "bows", "bowsmith": "bows", "fletcher": "bows", "hunter": "bows",
    "shieldsmith": "shields", "shieldmaker": "shields",
    "grocer": "grocer", "chef": "grocer", "cook": "grocer", "baker": "grocer", "farmer": "grocer",
    "barkeep": "drink", "bartender": "drink", "innkeeper": "drink", "brewer": "drink",
    "chemist": "alchemist", "alchemist": "alchemist",
    "fisherman": "fishing", "fisher": "fishing",
    "jeweler": "jeweler",
    "merchant": "general", "shopkeeper": "general", "vendor": "general", "vender": "general",
    "trader": "general", "peddler": "general", "junker": "general", "craftsman": "general",
    "carpenter": "general", "stonemason": "general", "potter": "general", "mechanic": "general",
}
# the kinds (kind()) a trade's goods are: a catalog of other things is no
# model for the trade (a smithy's quest handouts, a spell list)
TRADE_KINDS = {
    "weapons": {"weapon"}, "armor": {"armor"}, "tailor": {"armor"}, "bows": {"weapon", "ammo"},
    "shields": {"armor"}, "grocer": {"meal", "drink"}, "drink": {"meal", "drink"},
    "alchemist": {"scroll"}, "fishing": {"tool"}, "jeweler": {"jewelry"},
}
# trades whose town's variety matters more than the region's level, however
# far: a town's spell vendors sell a class's whole list between them, its
# grocers food and drink
VARIETY_FIRST = {"alchemist", "grocer", "drink", "fishing"}
# the trades whose own sellers are the fishing tool's
FISHING_SELLERS = re.compile(r"(?i)^(pole merchant|tacklemaster) ")

# Not shops, by the name (the first match decides; a Return Home catalog
# comes before any of these).
LEFT_OUT = [
    (re.compile(r"(?i)^keeper\b"), "a Keeper: a quest giver (EQOA-Data), no merchant flag in Return Home's rows"),
    (re.compile(r"(?i)^master craftsman\b"), "a master craftsman: a crafting master, no shop"),
    (re.compile(r"(?i)^storyteller\b"), "a storyteller: lore, no shop"),
    (re.compile(r"(?i)\bcitizen\b"), "a citizen: townsfolk, no shop"),
    (re.compile(r"(?i)^guard\b|\bguard$"), "a guard, no shop"),
    (re.compile(r"(?i)^drunk$"), "a drunk, no shop"),
    (re.compile(r"(?i)^(bard|shadowknight|brother|spellsmith|physics)\b|\bmaster$"),
     "a class's or a guild's NPC (trainer or quest), no shop"),
    (re.compile(r"(?i)^costume\b"), "Sandstorm's costume vendor: 'my store won't be ready for awhile'"),
]
SHOP_WORD = re.compile(r"(?i)\b(" + "|".join(sorted(TRADES, key=len, reverse=True)) + r")\b")

# What a merchant of no trade sells: the supplies of Return Home's supply
# merchants (Shohan, Shudan and Grener: leather strip, iron ore, wooden shaft,
# block of ore; Yesam: silk cord, bit of ivory; Mintopp: dice, fishing line,
# fishing worms, bit of charcoal, oilstone; Mallie: oilstone; Landi: lantern
# oil; Charlie: fish liver oil; Lorili Ashgo: yerg oil), by name.
# fishing line and worms (Mintopp's) are left to the fishing trade: worms
# worn nowhere are no bait (seed_fishing.py's can of worms is).
GENERAL_GOODS = ["leather strip", "silk cord", "wooden shaft", "iron ore", "block of ore", "bit of ivory",
                 "bit of charcoal", "oilstone", "lantern oil", "fish liver oil", "yerg oil", "dice"]

JEWEL_SLOTS = (3, 4, 6, 8, 9)
JEWEL_WORDS = re.compile(r"(?i)\b(bronze|brass|copper|iron|steel|silver|gold|golden|electrum|platinum|mithril|"
                         r"jade|amber|pearl|ivory|opal|ruby|sapphire|emerald|diamond|onyx|garnet|topaz|"
                         r"quicksilver|sunsteel|rathesteel)\b")
JEWEL_KINDS = re.compile(r"(?i)\b(ring|band|earring|earrings|stud|hoop|necklace|amulet|choker|chain|charm|"
                         r"pendant|bracelet|bangle|belt|torque|trinket)\b")
# a drop's name, not a jeweler's: someone's, "... of ...", bones
JEWEL_NOT = re.compile(r"(?i)'s\b|\bof\b|\bbone")

ARMOR_SLOTS = {1, 2, 5, 7, 10, 11, 13, 19}  # and shields; 6 (a bracelet) is jewelry
WEAPON_SLOTS = {12, 14, 15, 16}

STAT_COLS = ["str", "sta", "agi", "wis", "dex", "cha", "intelligence", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
             "PR", "DR", "FR", "CR", "LR", "AR"]
# a harvest ware's stat index -> the column (import_sandstorm_harvest.py's STAT_COLUMNS)
STAT_INDEX = {0: "str", 1: "sta", 2: "agi", 3: "dex", 4: "wis", 5: "intelligence", 6: "cha",
              8: "HPMAX", 10: "POWMAX", 12: "PoT", 13: "HoT", 14: "AC",
              22: "PR", 23: "DR", 24: "FR", 25: "CR", 26: "LR", 27: "AR"}

LOG_DDL = """CREATE TABLE IF NOT EXISTS merchant_stock (
    what  TEXT NOT NULL,     -- 'catalog' | 'npc_id' | 'itemcost' | 'free' | 'gm price' | 'left out' | 'seq' | 'md5'
    key   INTEGER NOT NULL,  -- the npcitem row, the npcs row or the pattern (0 for seq and md5)
    old   TEXT,              -- what it was (a catalog row: none)
    new   TEXT,              -- what it became (a catalog row: npcid,patternid,listnumber)
    note  TEXT NOT NULL DEFAULT ''  -- where it came from; why an NPC is left out
)"""


def norm(name):
    """A name as names are compared: no case, no padding or double spaces, a
    backtick the same as an apostrophe, no trailing '$'."""
    n = " ".join((name or "").replace("`", "'").split()).lower()
    return n[:-1].rstrip() if n.endswith("$") else n


def loose(name):
    """A name compared a second time: norm()'s, its apostrophes dropped
    (Sandstorm's "Dark One's Bracers" is Return Home's "Dark Ones Bracers")."""
    return norm(name).replace("'", "")


def words(name):
    return re.findall(r"[a-z']+", (name or "").lower())


def name_words(name):
    return {w for w in re.findall(r"[a-z]+", (name or "").lower()) if len(w) > 2 and w not in STOPWORDS}


def stat_points(p):
    s = p.get("stats", {})
    attrs = sum(s.get(c, 0) or 0 for c in ("str", "sta", "agi", "wis", "dex", "cha", "intelligence"))
    pools = ((s.get("HPMAX", 0) or 0) + (s.get("POWMAX", 0) or 0)) / 2
    res = sum(s.get(c, 0) or 0 for c in ("PR", "DR", "FR", "CR", "LR", "AR"))
    regen = 5 * ((s.get("PoT", 0) or 0) + (s.get("HoT", 0) or 0))
    return max(0, attrs + pools + res + regen)


def kind(p):
    """The kind an item is priced among."""
    slot, stack = p["slot"], p["stack"]
    if p["icon"] == SCROLL_ICON and slot == -1:
        return "scroll"
    if slot == -1 and MEAL.search(p["desc"] or ""):
        return "meal"
    if slot == -1 and DRINK.search(p["desc"] or ""):
        return "drink"
    if slot == 17 or (slot == 16 and stack > 1) or (slot == -1 and stack > 1 and re.search(r"(?i)\b(arrow|bolt)s?$", p["name"])):
        return "ammo"
    if slot in WEAPON_SLOTS:
        return "weapon"
    if slot in ARMOR_SLOTS:
        return "armor"
    if slot in (3, 4, 6, 8, 9, 18):
        return "jewelry"
    if slot >= 20:
        return "tool"
    return "other"


def pattern_from_row(r):
    """itempattern columns (as load_patterns selects them) -> an item dict."""
    (pid, name, cost, slot, level, stack, dmg, icon, desc, trade, lore, *stats) = r
    return dict(id=pid, name=name, cost=cost or 0, slot=slot if slot is not None else -1, level=level or 0,
                stack=stack or 0, dmg=dmg or 0, icon=icon, desc=desc or "", trade=trade or 0, lore=lore or 0,
                stats={c: v or 0 for c, v in zip(STAT_COLS, stats)})


def load_patterns(db):
    cols = ", ".join(STAT_COLS)
    return {r[0]: pattern_from_row(r) for r in db.execute(
        f"SELECT patternid, itemname, itemcost, equipslot, levelreq, maxstack, weapondamage, itemicon, itemdesc, "
        f"trade, lore, {cols} FROM itempattern")}


def ware_item(w):
    """A harvest ware (or inventory item) as an item dict (its own fields)."""
    stats = {c: 0 for c in STAT_COLS}
    for idx, val in w.get("stats") or []:
        col = STAT_INDEX.get(idx)
        if col:
            stats[col] = val
    return dict(id=w["id"], name=w["name"], cost=w["value"], slot=w.get("wear_slot", -1), level=w.get("level_req", 0),
                stack=w.get("max_stack", 1), dmg=w.get("damage", 0), icon=w.get("icon"), desc=w.get("desc") or "",
                trade=w.get("no_trade", 0), lore=w.get("lore", 0), stats=stats)


# ---- measured prices ----

class Measured:
    """What Sandstorm is known to charge: values by name, items with their
    fields (the rule's data), and what the harvests saw of the NPCs."""

    def __init__(self):
        self.by_name = {}          # norm name -> value
        self.items = {}            # norm name -> item dict (fields + cost = the value)
        self.by_id = {}            # harvest id -> item dict
        self.conflicts = collections.defaultdict(set)
        self.sources = collections.Counter()
        self.seen = {}             # (world, norm name) -> npc flags, as Sandstorm shows them
        self.opened = {}           # (world, norm name) -> wares Sandstorm sells there (count)
        self.merchants = []        # the harvests' merchants: name, world, pos, wares, when
        self.sold = set()          # norm names of what Sandstorm's merchants sell
        self.by_loose = None       # loose name -> value, made when first asked

    def add(self, item, source):
        n = norm(item["name"])
        if not n:
            return
        v = int(item["cost"])
        if n in self.by_name and self.by_name[n] != v:
            self.conflicts[n] |= {self.by_name[n], v}
            return
        self.by_name[n] = v
        self.items.setdefault(n, item)
        if item.get("id") is not None:
            self.by_id.setdefault(item["id"], item)
        self.sources[source] += 1

    def load(self, paths):
        files = []
        for p in paths:
            files += sorted(glob.glob(os.path.join(p, "*.json"))) if os.path.isdir(p) else [p]
        if paths and not files:
            sys.exit(f"--prices: no files in {', '.join(paths)}")
        for f in files:
            if f.endswith(".json"):
                self.load_json(f)
            else:
                self.load_table(f)
        return files

    def load_json(self, path):
        with open(path) as fh:
            d = json.load(fh)
        if isinstance(d, dict) and ("merchants" in d or "inventory" in d):
            world = (d.get("character") or {}).get("world", 0)
            for m in d.get("merchants") or []:
                wares = m.get("wares")
                if wares is not None:
                    self.opened[(world, norm(m["name"]))] = len(wares)
                    self.merchants.append(dict(name=m["name"], world=world, pos=m.get("pos") or [0, 0, 0],
                                               wares=[ware_item(w) for w in wares], when=d.get("when", 0)))
                for w in wares or []:
                    self.add(ware_item(w), "harvest ware")
                    self.sold.add(norm(w["name"]))
            for w in d.get("inventory") or []:
                self.add(ware_item(w), "harvest inventory")
            for s in d.get("seen") or []:
                self.seen[(world, norm(s["name"]))] = s.get("npc_flags", 0)
            return
        rows = d if isinstance(d, list) else [dict(name=k, value=v) for k, v in d.items()]
        for r in rows:
            v = r.get("value", r.get("price"))
            if r.get("name") and v is not None:
                self.by_name.setdefault(norm(r["name"]), int(v))
                self.sources["price list"] += 1

    def load_table(self, path):
        with open(path, newline="") as fh:
            sample = fh.read(4096)
            fh.seek(0)
            dialect = csv.excel_tab if "\t" in sample.split("\n", 1)[0] else csv.excel
            for r in csv.DictReader(fh, dialect=dialect):
                r = {(k or "").strip().lower(): v for k, v in r.items()}
                v = r.get("price") or r.get("value")
                if r.get("name") and v not in (None, ""):
                    try:
                        val = int(float(v))
                    except ValueError:
                        continue
                    n = norm(r["name"])
                    if n in self.by_name and self.by_name[n] != val:
                        self.conflicts[n] |= {self.by_name[n], val}
                        continue
                    self.by_name[n] = val
                    self.sources["price list"] += 1

    def add_db(self, db, patterns):
        """The patterns the Sandstorm harvest import wrote: Sandstorm's own
        values and fields."""
        if not db.execute("SELECT 1 FROM sqlite_master WHERE name = 'sandstorm_import_rows'").fetchone():
            return
        for (pid,) in db.execute("SELECT DISTINCT key FROM sandstorm_import_rows WHERE tbl = 'itempattern' "
                                 "AND action IN ('inserted', 'updated')"):
            p = patterns.get(pid)
            if p and p["cost"] > 0:
                self.add(dict(p), "Sandstorm import")

    def fill_items(self, patterns):
        """Names priced by a plain list carry no fields: take ours."""
        ours = collections.defaultdict(list)
        for p in patterns.values():
            ours[norm(p["name"])].append(p)
        for n, v in self.by_name.items():
            if n not in self.items and ours.get(n):
                self.items[n] = dict(min(ours[n], key=lambda p: p["id"]), cost=v)

    def price_of(self, p):
        """Sandstorm's value for our pattern p, and how it was matched."""
        n = norm(p["name"])
        if n in self.by_name:
            return self.by_name[n], "measured"
        if self.by_loose is None:
            self.by_loose = {}
            for k, v in self.by_name.items():
                self.by_loose.setdefault(k.replace("'", ""), v)
        if loose(p["name"]) in self.by_loose:
            return self.by_loose[loose(p["name"])], "measured"
        h = self.by_id.get(p["id"])
        if h is not None and h["slot"] == p["slot"] and h["level"] == p["level"]:
            return int(h["cost"]), f"measured by id ({h['name']})"
        return None, None


# ---- the rule ----

class Rule:
    """Ours: a price for an item Sandstorm's prices do not name, fitted on the
    ones they do."""

    def __init__(self, measured):
        self.pool = collections.defaultdict(list)
        for it in measured.items.values():
            if it["cost"] > 0:
                self.pool[kind(it)].append(it)
        plain = [it for it in self.pool["weapon"] if it["dmg"] > 0 and stat_points(it) == 0]
        # relative least squares for v = a * dmg: a = sum(d/v) / sum(d^2/v^2)
        num = sum(it["dmg"] / it["cost"] for it in plain)
        den = sum((it["dmg"] / it["cost"]) ** 2 for it in plain)
        self.a = num / den if den else 1.83
        self.plain = plain
        self.curve = self.scroll_curve(self.pool["scroll"])

    @staticmethod
    def scroll_curve(scrolls, exclude=None):
        """Sandstorm's scroll prices by level: the median of the measured
        scrolls of each level (every one of a level costs the same but for a
        stray), level by level."""
        by_level = collections.defaultdict(list)
        for it in scrolls:
            if it is not exclude and it["level"] > 0:
                by_level[it["level"]].append(it["cost"])
        return sorted((lv, statistics.median(v)) for lv, v in by_level.items())

    @staticmethod
    def on_curve(curve, level):
        """A scroll's price at a level: the curve's at a level measured, the
        straight line between the two measured either side of it, the square
        of the level above the highest (as import_eqoadata_spells.py scales a
        scroll's price, and within a tunar of the curve's own quadratic) and
        the lowest's below it."""
        if not curve:
            return None
        if level <= curve[0][0]:
            return curve[0][1]
        if level >= curve[-1][0]:
            return curve[-1][1] * (level / curve[-1][0]) ** 2
        for (l0, v0), (l1, v1) in zip(curve, curve[1:]):
            if l0 <= level <= l1:
                return v0 + (v1 - v0) * (level - l0) / (l1 - l0)
        return None

    @staticmethod
    def distance(x, p):
        d = abs(p["level"] - x["level"]) / 3
        a, b = name_words(x["name"]), name_words(p["name"])
        d += 1 - (len(a & b) / len(a | b) if a | b else 0)
        d += abs(math.log1p(p["stats"].get("AC", 0)) - math.log1p(x["stats"].get("AC", 0))) / 0.35
        d += abs(math.log1p(p["dmg"]) - math.log1p(x["dmg"])) / 0.35
        d += abs(math.log1p(stat_points(p)) - math.log1p(stat_points(x))) / 0.35
        if p["slot"] != x["slot"]:
            d += 0.5
        return d

    def nearest(self, x, exclude=None):
        k = kind(x)
        pool = [p for p in self.pool.get(k, []) if p is not exclude]
        if not pool:  # a kind with nothing measured: every measured item
            pool = [p for ps in self.pool.values() for p in ps if p is not exclude]
        if not pool:
            return None
        near = sorted(pool, key=lambda p: (self.distance(x, p), p["cost"]))[:KNN]
        return statistics.median(p["cost"] for p in near)

    def price(self, x, exclude=None):
        """The rule's price and the part of it that gave it."""
        if kind(x) == "weapon" and x["dmg"] > 0 and stat_points(x) == 0:
            return max(1, round(self.a * x["dmg"])), "damage"
        if kind(x) == "scroll" and self.curve:
            return max(1, round(self.on_curve(self.curve, x["level"]))), "the scroll curve"
        v = self.nearest(x, exclude)
        if v is None:
            return None, None
        return max(1, round(v)), "nearest"

    def errors(self):
        """Leave one out: each measured item priced by the others, by kind."""
        out = {}
        for k, items in sorted(self.pool.items()):
            errs = []
            for it in items:
                if k == "weapon" and it["dmg"] > 0 and stat_points(it) == 0:
                    others = [p for p in self.plain if p is not it]
                    num = sum(p["dmg"] / p["cost"] for p in others)
                    den = sum((p["dmg"] / p["cost"]) ** 2 for p in others)
                    v = (num / den if den else self.a) * it["dmg"]
                elif k == "scroll" and it["level"] > 0:
                    v = self.on_curve(self.scroll_curve(items, exclude=it), it["level"])
                else:
                    v = self.nearest(it, exclude=it)
                if v:
                    errs.append(abs(v - it["cost"]) / it["cost"])
            if errs:
                errs.sort()
                out[k] = (len(errs), errs[len(errs) // 2], errs[int(0.9 * (len(errs) - 1))])
        return out


# ---- the NPCs ----

def trade_of(name):
    """The trade a name says: the fishing tool's sellers', else the first word
    of it that names one."""
    if FISHING_SELLERS.match(name or ""):
        return "fishing"
    for w in words(name):
        t = TRADES.get(w.strip("'"))
        if t:
            return t
    return None


def left_out_reason(npc, measured):
    name = npc["name"] or ""
    flags = measured.seen.get((npc["world"], norm(name)))
    if flags is not None and not flags & MERCHANT:
        return f"Sandstorm shows it without the merchant flag (flags {flags})"
    for pat, why in LEFT_OUT:
        if pat.search(name):
            return why
    if name[:1].islower() and not SHOP_WORD.search(name):
        return "a creature (a lowercase name, no shop word): the flag is a mistake"
    return None


class Grid:
    """NPCs by world and cell, for the nearest ones."""

    def __init__(self, cell=500.0):
        self.cell = cell
        self.rows = collections.defaultdict(list)

    def add(self, n):
        self.rows[(n["world"], int(n["x"] // self.cell), int(n["z"] // self.cell))].append(n)

    def near(self, world, x, z, radius):
        cx, cz, r = int(x // self.cell), int(z // self.cell), int(radius // self.cell) + 1
        for i in range(cx - r, cx + r + 1):
            for j in range(cz - r, cz + r + 1):
                for n in self.rows.get((world, i, j), ()):
                    d = math.hypot(n["x"] - x, n["z"] - z)
                    if d <= radius:
                        yield d, n


def load_npcs(db):
    npcs = [dict(id=i, name=n or "", type=t or 0, npc_id=k or 0, world=w or 0, x=x or 0.0, y=y or 0.0, z=z or 0.0,
                 level=lv or 0)
            for i, n, t, k, w, x, y, z, lv in db.execute(
                "SELECT id, npc_name, npc_type, npc_id, world, x_coord, y_coord, z_coord, npc_level FROM npcs ORDER BY id")]
    for n in npcs:
        n["key"] = n["npc_id"] or n["id"]
    return npcs


def region_levels(npcs, merchants):
    """Each merchant's region level: the median level of the REGION_K nearest
    creatures (no NPC bit, a lowercase name) within REGION_RADIUS."""
    grid = Grid()
    for n in npcs:
        if not n["type"] & NPC_BIT and n["name"][:1].islower() and n["level"] > 0:
            grid.add(n)
    out = {}
    for m in merchants:
        near = sorted(grid.near(m["world"], m["x"], m["z"], REGION_RADIUS), key=lambda t: t[0])[:REGION_K]
        out[m["id"]] = int(statistics.median(n["level"] for _, n in near)) if near else REGION_DEFAULT
    return out


def towns(merchants):
    """Merchants linked within TOWN_LINK, by world: row id -> town number."""
    parent = {m["id"]: m["id"] for m in merchants}

    def find(i):
        while parent[i] != i:
            parent[i] = parent[parent[i]]
            i = parent[i]
        return i

    grid = Grid(TOWN_LINK)
    for m in merchants:
        grid.add(m)
    for m in merchants:
        for _, n in grid.near(m["world"], m["x"], m["z"], TOWN_LINK):
            a, b = find(m["id"]), find(n["id"])
            if a != b:
                parent[a] = b
    return {m["id"]: find(m["id"]) for m in merchants}


# ---- Return Home's catalogs ----

def prh_catalogs(db, prh, patterns):
    """Return Home's catalogs as ours: our npcs row -> pattern ids in its
    order, through the import's map; and what became of each source catalog."""
    if not db.execute("SELECT 1 FROM sqlite_master WHERE name = 'prh_import_npcs'").fetchone():
        sys.exit("no prh_import_npcs here: import_prh_content.py has not run on this database")
    to_ours = {s: (st, d) for s, st, d in db.execute("SELECT src_id, status, dst_id FROM prh_import_npcs")}
    pats = {s: d for s, d, st in db.execute(
        "SELECT src_id, dst_id, status FROM prh_import_ids WHERE tbl = 'itempattern'") if st != "held"}
    src = collections.defaultdict(list)
    for npcid, pid in prh.execute("SELECT npcid, patternid FROM npcitem ORDER BY npcid, listnumber, itemid"):
        src[npcid].append(pid)
    out, fate = {}, {}
    for sid in sorted(src):
        st, dst = to_ours.get(sid, ("unknown", None))
        items = []
        for pid in src[sid]:
            d = pats.get(pid)
            if d is not None and d in patterns and d not in items:
                items.append(d)
        fate[sid] = (st, dst, len(src[sid]), len(items))
        if dst is None or not items or dst in out:
            continue
        out[dst] = items[:LIST_CAP]
    return out, fate


# ---- Sandstorm's own catalogs ----

def one_letter_off(a, b):
    """Whether two names differ by one letter added, dropped or changed
    (import_sandstorm_harvest.py's: Return Home's "Merchant Liliac" is
    Sandstorm's "Merchant Lilac")."""
    if a == b or abs(len(a) - len(b)) > 1:
        return False
    if len(a) > len(b):
        a, b = b, a
    i = 0
    while i < len(a) and a[i] == b[i]:
        i += 1
    return a[i + (len(a) == len(b)):] == b[i + 1:]


def harvest_on_npcs(npcs, measured):
    """The harvests' merchants on our NPCs, as import_sandstorm_harvest.py
    matches them: the same world, the same name within MATCH_RADIUS, else a
    name a letter off within NEAR_RADIUS; of two harvests of one merchant the
    latest; of two merchants on one NPC the nearer. npcs row -> (the
    merchant, matched by a near name)."""
    latest = {}
    for m in sorted(measured.merchants, key=lambda m: m["when"]):
        latest[(m["world"], norm(m["name"]), round(m["pos"][0]), round(m["pos"][2]))] = m
    grid = Grid(NEAR_RADIUS)
    for n in npcs:
        grid.add(n)
    out, misses = {}, []
    for m in latest.values():
        x, y, z = (list(m["pos"]) + [0, 0, 0])[:3]
        best = near = None
        for _, n in grid.near(m["world"], x, z, NEAR_RADIUS):
            d = math.dist((x, y, z), (n["x"], n["y"], n["z"]))
            if norm(n["name"]) == norm(m["name"]):
                if d <= MATCH_RADIUS and (best is None or d < best[0]):
                    best = (d, n)
            elif one_letter_off(norm(n["name"]), norm(m["name"])) and d <= NEAR_RADIUS:
                if near is None or d < near[0]:
                    near = (d, n)
        hit = best or near
        if hit is None:
            if m["wares"]:
                misses.append(m)
            continue
        d, n = hit
        if n["id"] not in out or d < out[n["id"]][0]:
            out[n["id"]] = (d, m, best is None)
    return {i: (m, by_near) for i, (_, m, by_near) in out.items()}, misses


def ware_pattern(w, patterns, by_name):
    """Our pattern of a Sandstorm ware, and how it was matched: its own id
    under its name; else its name (the one at its wear slot and level first,
    then the lowest id); else its id at its wear slot and level (a renamed
    item, as prices are matched). None for an item this database lacks."""
    n = norm(w["name"])
    p = patterns.get(w["id"])
    if p is not None and norm(p["name"]) == n:
        return p["id"], "id"
    cands = by_name.get(n) or by_name.get(("loose", loose(w["name"])))
    if cands:
        same = [c for c in cands if c["slot"] == w["slot"] and c["level"] == w["level"]]
        return min(same or cands, key=lambda c: c["id"])["id"], "name"
    if p is not None and p["slot"] == w["slot"] and p["level"] == w["level"]:
        return p["id"], f"id ({p['name']})"
    return None, None


def sandstorm_catalog(m, patterns, by_name):
    """A harvested merchant's wares as our patterns, in its order: the pattern
    ids, the wares this database lacks, the ones matched by a renamed id."""
    items, missing, renamed = [], [], []
    for w in m["wares"]:
        pid, how = ware_pattern(w, patterns, by_name)
        if pid is None:
            missing.append(w["name"])
        elif pid not in items:
            items.append(pid)
            if how.startswith("id ("):
                renamed.append(f"{w['name']} = our {patterns[pid]['name']}")
    return items[:LIST_CAP], missing, renamed


# ---- the plan ----

class Plan:
    def __init__(self):
        self.stock = {}        # npcs row id -> (pattern ids, source)
        self.left_out = {}     # npcs row id -> reason
        self.keyed = []        # npcs rows whose npc_id is set to their id
        self.prices = {}       # pattern -> (price, how)
        self.free = {}         # pattern -> how (Sandstorm sells it at 0)
        self.unpriced = []
        self.gm_priced = []    # patterns a GM edited: their 0 left alone
        self.reprice = {}      # pattern -> (old, new, why): the two exceptions to "never a nonzero price"
        self.sandstorm = {}    # npcs row -> (items, missing, renamed, by a near name) of Sandstorm's own
        self.misses = []       # harvested merchants with wares on no NPC here
        self.prh_report = {}


def trade_goods(t, items, patterns, measured):
    """Whether a catalog is its trade's goods: at least half its items of the
    trade's kinds (TRADE_KINDS), and not half of them Sandstorm's free
    handouts."""
    kinds = TRADE_KINDS.get(t)
    if kinds is None:
        return False
    mine = sum(kind(patterns[x]) in kinds for x in items)
    free = sum(patterns[x]["cost"] == 0 and measured.by_name.get(norm(patterns[x]["name"])) == 0 for x in items)
    return mine * 2 >= len(items) and free * 2 < len(items)


def gm_edits(db):
    """The catalogs (npcitem's npcid) and the patterns a GM edited in the
    panel (gm_audit's before and after rows): this tool leaves both alone."""
    keys, pats = set(), set()
    if not db.execute("SELECT 1 FROM sqlite_master WHERE name = 'gm_audit'").fetchone():
        return keys, pats
    for tbl, rowkey, before, after in db.execute(
            "SELECT tbl, rowkey, before, after FROM gm_audit WHERE tbl IN ('npcitem', 'itempattern')"):
        for blob in (rowkey, before, after):
            try:
                row = json.loads(blob) if blob else {}
            except ValueError:
                continue
            if not isinstance(row, dict):
                continue
            if tbl == "npcitem" and row.get("npcid") is not None:
                keys.add(int(row["npcid"]))
            if tbl == "itempattern" and row.get("patternid") is not None:
                pats.add(int(row["patternid"]))
    return keys, pats


def catalog_level(items, patterns):
    lv = [patterns[p]["level"] for p in items if patterns[p]["level"] > 0]
    return statistics.median(lv) if lv else 0


def plan(db, prh, measured, patterns):
    p = Plan()
    npcs = load_npcs(db)
    by_id = {n["id"]: n for n in npcs}
    catalogs = collections.defaultdict(list)
    for key, pid in db.execute("SELECT npcid, patternid FROM npcitem ORDER BY npcid, listnumber, itemid"):
        if pid in patterns:  # as GetNPCItems' join with itempattern reads them
            catalogs[key].append(pid)
    merchants = [n for n in npcs if n["type"] & MERCHANT]
    empty = [n for n in merchants if not (n["npc_id"] and catalogs.get(n["npc_id"]))]
    region = region_levels(npcs, merchants)
    town = towns(merchants)

    # 1. Return Home's catalogs
    rh, fate = prh_catalogs(db, prh, patterns)
    empty_ids = {n["id"] for n in empty}
    gm_keys, gm_patterns = gm_edits(db)
    by_name = collections.defaultdict(list)
    for pat in patterns.values():
        by_name[norm(pat["name"])].append(pat)
        by_name[("loose", loose(pat["name"]))].append(pat)
    harvested, p.misses = harvest_on_npcs(npcs, measured)
    for n in empty:
        h = harvested.get(n["id"])
        if h is not None and h[0]["wares"] and n["key"] not in gm_keys:
            items, missing, renamed = sandstorm_catalog(h[0], patterns, by_name)
            p.sandstorm[n["id"]] = (items, missing, renamed, h[1])
    for n in empty:
        ss = p.sandstorm.get(n["id"])
        if n["key"] in gm_keys:
            p.left_out[n["id"]] = "a GM edited its catalog in the panel (gm_audit): left as it is"
        elif ss and ss[0]:
            whole = "whole" if not ss[1] else f"{len(ss[0])} of {len(ss[0]) + len(ss[1])} wares here"
            p.stock[n["id"]] = (ss[0], f"Sandstorm's catalog ({whole})")
        elif n["id"] in rh:
            p.stock[n["id"]] = (rh[n["id"]], "Return Home's catalog")
    tally = collections.Counter()
    for sid, (st, dst, n_src, n_ok) in fate.items():
        n = by_id.get(dst)
        if n is None:
            tally[(st, "no NPC here")] += 1
        elif not n["type"] & MERCHANT:
            tally[(st, "not a merchant here (loot)")] += 1
        elif dst not in empty_ids:
            tally[(st, "already stocked")] += 1
        elif not n_ok:
            tally[(st, "none of its items here")] += 1
        elif rh.get(dst) is None:
            tally[(st, "a second copy of one NPC")] += 1
        elif not p.stock.get(dst, ("", ""))[1].startswith("Return Home"):
            tally[(st, "Sandstorm's own catalog instead")] += 1
        else:
            tally[(st, "stocked from it")] += 1
    p.prh_report = tally
    p.harvested = harvested

    # who is left out, and each one's trade
    trade = {}
    for n in empty:
        if n["id"] in p.stock or n["id"] in p.left_out:
            continue
        why = left_out_reason(n, measured)
        if why:
            p.left_out[n["id"]] = why
            continue
        trade[n["id"]] = trade_of(n["name"]) or "general"

    # 2. the trades' catalogs: what merchants of each sell (theirs or step 1's)
    held = {}  # npcs row -> pattern ids
    for n in merchants:
        if n["npc_id"] and catalogs.get(n["npc_id"]):
            held[n["id"]] = catalogs[n["npc_id"]]
    for i, (items, _) in p.stock.items():
        held[i] = items
    pools = collections.defaultdict(dict)  # trade -> tuple(items) -> holders
    for i, items in held.items():
        n = by_id[i]
        t = trade_of(n["name"])
        if t in (None, "general"):
            continue
        if not trade_goods(t, items, patterns, measured):
            continue  # a spell list, a quest's handouts, odds and ends: no model for the trade
        pools[t].setdefault(tuple(items), []).append(n)
    if pools.get("fishing"):
        # a fisherman sells what the two sellers sell between them
        union = []
        for items in pools["fishing"]:
            union += [x for x in items if x not in union]
        pools["fishing"] = {tuple(union[:LIST_CAP]): [h for hs in pools["fishing"].values() for h in hs]}
    sets = {t: sorted(ps.items(), key=lambda kv: (catalog_level(kv[0], patterns), kv[0])) for t, ps in pools.items()}
    town_has = collections.defaultdict(collections.Counter)  # (town, trade) -> catalogs sold there
    for i, items in held.items():
        town_has[(town[i], trade_of(by_id[i]["name"]))][tuple(items)] += 1

    def choose(n, t):
        best = None
        for idx, (items, holders) in enumerate(sets.get(t, [])):
            off = abs(catalog_level(items, patterns) - region[n["id"]])
            fit = int(off // LEVEL_STEP)
            same = [h for h in holders if h["world"] == n["world"]]
            dist = min((math.hypot(h["x"] - n["x"], h["z"] - n["z"]) for h in same), default=math.inf)
            has = town_has[(town[n["id"]], t)][items]
            # a town's merchants of a trade sell its tiers between them (Sandstorm's
            # Freeport tailors: cloth, rawhide, wool, studded, gossamer): one the
            # town lacks first, of those within TIER_REACH of the region's level;
            # a spell vendor's or a grocer's whatever the level
            key = (has, fit) if t in VARIETY_FIRST else (off > TIER_REACH, has, fit)
            key += (not same, dist, idx)
            if best is None or key < best[0]:
                best = (key, items, holders, same, dist)
        return best

    # 3. jewelers, and 4. general goods
    goods = general_goods(patterns, measured)
    order = sorted(trade, key=lambda i: (by_id[i]["world"], town[i], by_id[i]["name"], i))
    for i in order:
        n, t = by_id[i], trade[i]
        if t == "jeweler" and not sets.get("jeweler"):
            items = jewelry(patterns, region[i])
            if items:
                p.stock[i] = (items, f"jewelry of the region's level ({region[i]})")
                continue
            t = "general"
        if t == "general":
            if goods:
                p.stock[i] = (goods, "general goods")
            else:
                p.left_out[i] = "no trade, and none of the general goods is here"
            continue
        best = choose(n, t)
        if best is None:
            if goods:
                p.stock[i] = (goods, f"general goods (no merchant of trade {t} sells anything here)")
            else:
                p.left_out[i] = f"no merchant of trade {t} sells anything here"
            continue
        _, items, holders, same, dist = best
        h = min(same, key=lambda h: math.hypot(h["x"] - n["x"], h["z"] - n["z"])) if same else holders[0]
        where = f"{dist:.0f} away" if same else f"world {h['world']}"
        p.stock[i] = (list(items), f"{t}: {h['name']}'s catalog ({where}; items level "
                                    f"{catalog_level(items, patterns):g}, region {region[i]})")
        town_has[(town[i], t)][items] += 1

    for i in p.stock:
        if not by_id[i]["npc_id"]:
            p.keyed.append(i)

    # prices: every catalog item, theirs and ours, whose itemcost is 0
    rule = Rule(measured)
    p.rule = rule
    # (a merchant's: a creature's rows are its corpse's loot, and its value
    # is what a merchant pays for it, left as it is)
    wanted = {x for n in merchants if n["npc_id"] for x in catalogs.get(n["npc_id"], ())}
    wanted |= {x for items, _ in p.stock.values() for x in items}
    p.odd = []
    ours_set = set_by(db)
    for x in sorted(wanted):
        pat = patterns.get(x)
        if pat is None:
            continue  # a row naming no pattern: GetNPCItems' join drops it too
        why = exception(x, pat, ours_set, gm_patterns, measured, rule)
        if why:
            p.reprice[x] = why
            continue
        if pat["cost"] > 0:
            # kept, but named when it is far from what the rule would say
            if norm(pat["name"]) not in measured.by_name:
                est, _ = rule.price(pat)
                if est and (pat["cost"] * ODD < est or pat["cost"] > est * ODD):
                    p.odd.append((x, est))
            continue
        if x in gm_patterns:
            p.gm_priced.append(x)
            continue
        v, how = measured.price_of(pat)
        if v is not None:
            if v > 0:
                p.prices[x] = (v, how)
            else:
                p.free[x] = how
            continue
        v, how = rule.price(pat)
        if v is None:
            p.unpriced.append(x)
        else:
            p.prices[x] = (v, "ours: " + how)
    p.by_id, p.region, p.town, p.empty = by_id, region, town, empty
    return p


def set_by(db):
    """Which tool set a pattern's price, of the two whose prices the
    exceptions below may change, and of Sandstorm's import, whose never:
    pattern -> 'grocers' | 'eqoa-data' | 'sandstorm' (the last wins)."""
    out = {}
    tables = {r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type = 'table'")}
    if "grocer_stock" in tables:
        for (k,) in db.execute("SELECT DISTINCT key FROM grocer_stock WHERE what = 'itemcost'"):
            out[k] = "grocers"
    if "eqoadata_spell_import_rows" in tables:
        for (k,) in db.execute("SELECT DISTINCT key FROM eqoadata_spell_import_rows WHERE tbl = 'itempattern'"):
            out[k] = "eqoa-data"
    if "sandstorm_import_rows" in tables:
        for (k,) in db.execute("SELECT DISTINCT key FROM sandstorm_import_rows WHERE tbl = 'itempattern'"):
            out[k] = "sandstorm"
    return out


def exception(x, pat, ours_set, gm_patterns, measured, rule):
    """The two exceptions to "never a nonzero price", each narrow and logged:
    (old, new, why), or None.

    - A grocer's price of stock_grocers.py's (ours: 10 tunar a level) for an
      item Sandstorm's merchants sell: Sandstorm's value.
    - A scroll import_eqoadata_spells.py made at 1 tunar (its price is a
      reference scroll's scaled by the square of the level, and its reference
      had 0): Sandstorm's scroll price at its level (the scroll curve).

    Never one a GM edited, nor one Sandstorm's import wrote."""
    if x in gm_patterns or pat["cost"] <= 0:
        return None
    src = ours_set.get(x)
    if src == "grocers":
        n = norm(pat["name"])
        v = measured.by_name.get(n)
        if n in measured.sold and v and v != pat["cost"]:
            return (pat["cost"], v, f"stock_grocers.py's price (ours); Sandstorm sells it at {v}")
    if src == "eqoa-data" and pat["cost"] == 1 and pat["icon"] == SCROLL_ICON and pat["slot"] == -1 and rule.curve:
        v = max(1, round(rule.on_curve(rule.curve, pat["level"])))
        if v != 1:
            return (1, v, f"import_eqoadata_spells.py made it at 1 (a 0 reference); Sandstorm's scroll "
                          f"price at level {pat['level']}")
    return None


def general_goods(patterns, measured):
    """GENERAL_GOODS as our patterns: of each name, a priced one if any, else
    the lowest id; in GENERAL_GOODS' order."""
    by = collections.defaultdict(list)
    for p in patterns.values():
        by[norm(p["name"])].append(p)
    out = []
    for name in GENERAL_GOODS:
        cands = by.get(name)
        if cands:
            out.append(min(cands, key=lambda p: (p["cost"] <= 0 and norm(p["name"]) not in measured.by_name, p["id"]))["id"])
    return out


def jewelry(patterns, level):
    """A jeweler's catalog: plain jewelry (a metal's or a gem's name and a
    jewelry word; tradeable, not lore; no one's, nothing "of" anything, no
    bones: those are drops) of the region's level and below (higher until
    JEWELS_AT_LEAST are), up to JEWELS_PER_SLOT of each wear slot, the
    nearest the level first."""
    seen, cands = set(), []
    for p in sorted(patterns.values(), key=lambda p: p["id"]):
        n = norm(p["name"])
        if (p["slot"] in JEWEL_SLOTS and p["trade"] == 0 and p["lore"] == 0 and p["level"] >= 1
                and JEWEL_WORDS.search(n) and JEWEL_KINDS.search(n) and not JEWEL_NOT.search(n) and n not in seen):
            seen.add(n)
            cands.append(p)
    top = max(level, 5)
    while sum(p["level"] <= top for p in cands) < JEWELS_AT_LEAST and top < 60:
        top += LEVEL_STEP
    out = []
    for slot in JEWEL_SLOTS:
        mine = sorted((p for p in cands if p["slot"] == slot and p["level"] <= top),
                      key=lambda p: (top - p["level"], p["id"]))
        out += [p["id"] for p in mine[:JEWELS_PER_SLOT]]
    return sorted(out, key=lambda x: (patterns[x]["level"], patterns[x]["slot"], x))[:LIST_CAP]


# ---- report, apply, undo ----

def report(db, p, patterns, measured, files):
    by_id = p.by_id
    print(f"merchants: {sum(1 for n in by_id.values() if n['type'] & MERCHANT)}, "
          f"{len(p.empty)} with nothing to sell")
    print("Return Home's catalogs (source NPC: the import's status here, what became of it):")
    for (st, what), c in sorted(p.prh_report.items()):
        print(f"  {st:9s} {c:4d}  {what}")
    src = collections.Counter(s.split(":")[0].split(" (")[0] for _, s in p.stock.values())
    print(f"to stock: {len(p.stock)} merchants, {sum(len(i) for i, _ in p.stock.values())} catalog rows:",
          dict(src.most_common()))
    per = collections.defaultdict(lambda: [0, 0, [], 0])
    for i, (items, s) in p.stock.items():
        t = trade_of(by_id[i]["name"]) or "general"
        r = per[t]
        r[0] += 1
        r[1] += len(items)
        r[2] += [p.reprice[x][1] if x in p.reprice else p.prices.get(x, (patterns[x]["cost"], ""))[0] for x in items]
        r[3] += s.startswith("Return Home")
    print("by trade: merchants, rows, of them Return Home's, prices (min / median / max):")
    for t, (n, rows, prices, rh) in sorted(per.items(), key=lambda kv: -kv[1][0]):
        pr = [v for v in prices if v is not None]
        stat = f"{min(pr)} / {statistics.median(pr):g} / {max(pr)}" if pr else "-"
        print(f"  {t:10s} {n:4d} merchants {rows:5d} rows  {rh:4d} Return Home's  {stat}")
    for i, (items, s) in sorted(p.stock.items(), key=lambda kv: (by_id[kv[0]]["world"], by_id[kv[0]]["name"])):
        n = by_id[i]
        print(f"    {n['name']} (row {i}, world {n['world']}): {len(items)} items, {s}")
    if measured.merchants:
        ss = p.sandstorm
        whole = sorted((i for i, v in ss.items() if v[0] and not v[1]), key=lambda i: by_id[i]["name"])
        part = sorted((i for i, v in ss.items() if v[0] and v[1]), key=lambda i: by_id[i]["name"])
        none = sorted((i for i, v in ss.items() if not v[0]), key=lambda i: by_id[i]["name"])
        kept = sorted(by_id[i]["name"] for i, (m, _) in p.harvested.items()
                      if i not in ss and m["wares"] and by_id[i]["type"] & MERCHANT)
        print(f"Sandstorm's own catalogs (the --prices harvests' merchants on our NPCs): {len(ss)} of the empty "
              f"merchants have one: {len(whole)} whole, {len(part)} partial, {len(none)} with none of their wares "
              f"here (left to the next tier); {len(kept)} more are on merchants that sell something already (kept)")
        if whole:
            print(f"  whole: {', '.join(by_id[i]['name'] for i in whole)}")
        for i in part + none:
            items, missing, _, _ = ss[i]
            print(f"  {by_id[i]['name']} (row {i}): {len(items)} of {len(items) + len(missing)} here; "
                  f"this database lacks {', '.join(missing)}")
        renamed = [r for v in ss.values() for r in v[2]]
        if renamed:
            print(f"  wares matched by a renamed id: {', '.join(sorted(set(renamed)))}")
        near = [(by_id[i]["name"], p.harvested[i][0]["name"]) for i, (m, by_near) in p.harvested.items() if by_near]
        if near:
            print(f"  matched by a name a letter off: {', '.join(f'{theirs} is our {ours}' for ours, theirs in sorted(near))}")
        if p.misses:
            print(f"  {len(p.misses)} harvested merchants with wares are on no NPC here (the nearest of the name "
                  f"within {MATCH_RADIUS:g}): {', '.join(sorted(m['name'] + ' at ' + str([round(v) for v in m['pos']]) + ' world ' + str(m['world']) for m in p.misses))}")
    print(f"left out: {len(p.left_out)}")
    why = collections.Counter(p.left_out.values())
    for w, c in why.most_common():
        names = sorted(by_id[i]["name"] for i, r in p.left_out.items() if r == w)
        print(f"  {c:4d}  {w}: {', '.join(names[:30])}{' ...' if len(names) > 30 else ''}")
    if measured.opened:
        none = [i for i in p.stock if measured.opened.get((by_id[i]["world"], norm(by_id[i]["name"]))) == 0]
        if none:
            print(f"{len(none)} sell nothing on Sandstorm either (opened by a harvest): "
                  f"{', '.join(sorted(by_id[i]['name'] for i in none))}")
    if not files:
        print("warning: no --prices: only what the Sandstorm import wrote is measured, and the rule fits on that "
              "alone (the harvest's raw files price far more)")
    print(f"measured prices: {len(measured.by_name)} names {dict(measured.sources)}"
          f"{' from ' + str(len(files)) + ' files' if files else ''}")
    if measured.conflicts:
        print(f"  {len(measured.conflicts)} names with two values (the first kept): "
              f"{', '.join(f'{n} {sorted(v)}' for n, v in sorted(measured.conflicts.items())[:10])}")
    groc = sorted((x, a, b) for x, (a, b, w) in p.reprice.items() if w.startswith("stock_grocers"))
    scro = sorted((x, a, b) for x, (a, b, w) in p.reprice.items() if w.startswith("import_eqoadata"))
    print(f"grocers' prices (stock_grocers.py's, ours) of items Sandstorm sells, set to Sandstorm's: {len(groc)}"
          + (": " + ", ".join(f"{patterns[x]['name']} {a} -> {b}" for x, a, b in groc) if groc else ""))
    print(f"scrolls import_eqoadata_spells.py left at 1, set on Sandstorm's scroll curve: {len(scro)}"
          + (": " + ", ".join(f"{patterns[x]['name']} (L{patterns[x]['level']}) -> {b}" for x, a, b in scro) if scro else ""))
    how = collections.Counter(h.split(" (")[0] for _, h in p.prices.values())
    print(f"prices to set: {len(p.prices)} {dict(how)}; Sandstorm's 0 kept: {len(p.free)}; "
          f"no price found: {len(p.unpriced)}")
    rule = p.rule
    print(f"the rule: a weapon without stats {rule.a:.4f} x damage ({len(rule.plain)} measured); "
          f"anything else the median of the {KNN} nearest measured items of its kind "
          f"({', '.join(f'{k} {len(v)}' for k, v in sorted(rule.pool.items()))})")
    for k, (n, med, p90) in rule.errors().items():
        print(f"  leave one out, {k:8s}: {n:4d} items, median error {100 * med:5.1f}%, 90th percentile {100 * p90:5.1f}%")
    ours = [(x, v, h) for x, (v, h) in p.prices.items() if h.startswith("ours")]
    for x, v, h in sorted(ours, key=lambda t: (kind(patterns[t[0]]), patterns[t[0]]["level"]))[:60]:
        pat = patterns[x]
        print(f"    ours: {pat['name']} (L{pat['level']}, {kind(pat)}, slot {pat['slot']}): {v} by {h[6:]}")
    if len(ours) > 60:
        print(f"    ... and {len(ours) - 60} more of ours")
    if p.unpriced:
        print(f"  no price: {', '.join(patterns[x]['name'] for x in p.unpriced)}")
    if p.gm_priced:
        print(f"  a GM's pattern, its 0 left alone: {', '.join(patterns[x]['name'] for x in p.gm_priced)}")
    if p.free:
        print(f"  Sandstorm's 0 kept: {', '.join(sorted(patterns[x]['name'] for x in p.free))}")
    if p.odd:
        odd = [f"{patterns[x]['name']} {patterns[x]['cost']} (the rule {e})" for x, e in sorted(p.odd)]
        print(f"{len(odd)} catalog items keep a price of theirs (not Sandstorm's) {ODD} times or more off the rule's: "
              f"{', '.join(odd[:15])}{' ...' if len(odd) > 15 else ''}")
    costs = collections.defaultdict(set)
    for pat in patterns.values():
        if pat["cost"] > 0:
            costs[norm(pat["name"])].add(pat["cost"])
    repriced = {norm(patterns[x]["name"]) for x in p.reprice}
    differ = [(n, v, min(costs[n])) for n, v in measured.by_name.items()
              if v and len(costs.get(n, ())) == 1 and v not in costs[n] and n not in repriced]
    if differ:
        print(f"{len(differ)} items priced here (not Sandstorm's, kept) that Sandstorm prices otherwise: "
              f"{', '.join(f'{n} {c} (Sandstorm {v})' for n, v, c in sorted(differ)[:12])}"
              f"{' ...' if len(differ) > 12 else ''}")


def table_md5(db, table):
    h = hashlib.md5()
    for r in db.execute(f"SELECT * FROM {table} ORDER BY rowid"):
        h.update(repr(r).encode())
    return h.hexdigest()


TOUCHED = ("npcs", "npcitem", "itempattern")


def seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcitem'").fetchone()
    return r[0] if r else None


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-merchants"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def apply(db, p):
    db.execute(LOG_DDL)
    db.execute("BEGIN IMMEDIATE")
    try:
        log = [("md5", 0, json.dumps({t: table_md5(db, t) for t in TOUCHED}), None, "the touched tables before")]
        log.append(("seq", 0, None if seq(db) is None else str(seq(db)), None, "npcitem's AUTOINCREMENT"))
        for i in p.keyed:
            old = db.execute("SELECT npc_id FROM npcs WHERE id = ?", (i,)).fetchone()[0]
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (i, i))
            log.append(("npc_id", i, None if old is None else str(old), str(i), "keyed to its own row"))
        rows = 0
        for i, (items, src) in p.stock.items():
            key = p.by_id[i]["npc_id"] or i
            for n, x in enumerate(items):
                cur = db.execute("""INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid,
                    equiploc, location, listnumber) VALUES (?, 1, 0, 0, ?, -1, 1, ?)""", (key, x, n))
                log.append(("catalog", cur.lastrowid, None, f"{key},{x},{n}", src))
                rows += 1
        for x, (v, how) in p.prices.items():
            old = db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (x,)).fetchone()[0]
            if old:
                continue  # priced since the plan: never a nonzero one
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (v, x))
            log.append(("itemcost", x, str(old), str(v), how))
        for x, (was, v, why) in p.reprice.items():
            old = db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (x,)).fetchone()[0]
            if old != was:
                continue  # changed since the plan: left as it is
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (v, x))
            log.append(("itemcost", x, str(old), str(v), why))
        for x, how in p.free.items():
            log.append(("free", x, "0", "0", f"Sandstorm sells it at 0 ({how})"))
        for x in p.gm_priced:
            log.append(("gm price", x, "0", "0", "a GM edited the pattern in the panel: its 0 left alone"))
        for i, why in p.left_out.items():
            log.append(("left out", i, None, None, why))
        db.executemany("INSERT INTO merchant_stock (what, key, old, new, note) VALUES (?, ?, ?, ?, ?)", log)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return rows


def undo(db):
    rows = list(db.execute("SELECT rowid, what, key, old, new, note FROM merchant_stock ORDER BY rowid DESC"))
    n, kept = collections.Counter(), []
    before = None
    db.execute("BEGIN IMMEDIATE")
    try:
        for _, what, key, old, new, note in rows:
            if what == "md5":
                before = json.loads(old)
            elif what == "seq":
                if old is None:
                    db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcitem'")
                else:
                    db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcitem'", (int(old),))
            elif what == "catalog":
                npcid, pid, listn = (int(v) for v in new.split(","))
                r = db.execute("SELECT npcid, patternid, listnumber FROM npcitem WHERE itemid = ?", (key,)).fetchone()
                if r is None:
                    kept.append(f"catalog row {key} (gone already)")
                    continue
                if r != (npcid, pid, listn):
                    kept.append(f"catalog row {key} (changed since: {r})")
                    continue
                db.execute("DELETE FROM npcitem WHERE itemid = ?", (key,))
                n[what] += 1
            elif what == "npc_id":
                (cur,) = db.execute("SELECT npc_id FROM npcs WHERE id = ?", (key,)).fetchone() or (None,)
                if cur is None or str(cur) != new:
                    kept.append(f"npcs row {key}'s npc_id (now {cur})")
                    continue
                db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (None if old is None else int(old), key))
                n[what] += 1
            elif what == "itemcost":
                (cur,) = db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (key,)).fetchone() or (None,)
                if cur is None or str(cur) != new:
                    kept.append(f"pattern {key}'s price (now {cur})")
                    continue
                db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (int(old), key))
                n[what] += 1
        db.execute("DROP TABLE merchant_stock")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    after = {t: table_md5(db, t) for t in TOUCHED}
    return n, kept, before, after


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prh", help="jib's Return Home export (its npcitem), opened read-only")
    ap.add_argument("--prices", nargs="+", default=[], metavar="FILE",
                    help="Sandstorm's prices: eqoa harvest JSON files or directories of them, or a TSV/CSV of name, price")
    ap.add_argument("--apply", action="store_true", help="write the catalogs and prices")
    ap.add_argument("--undo", action="store_true", help="put back everything a run wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    stocked = db.execute("SELECT name FROM sqlite_master WHERE name = 'merchant_stock'").fetchone()
    if a.undo:
        if not stocked:
            print("merchant_stock is not there: nothing to put back")
            return 0
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        n, kept, before, after = undo(db)
        print("put back:", dict(n) or "nothing")
        if kept:
            print(f"left alone, changed since: {len(kept)}: {'; '.join(kept[:20])}{' ...' if len(kept) > 20 else ''}")
        if before:
            same = [t for t in TOUCHED if before.get(t) == after[t]]
            print(f"md5 of {', '.join(TOUCHED)} as before --apply: "
                  f"{'all equal' if len(same) == len(TOUCHED) else 'equal for ' + (', '.join(same) or 'none') + ' (something else changed the others since)'}")
        print("restart the server")
        return 0
    if not a.prh:
        ap.error("--prh is needed")
    if a.apply and stocked:
        ap.error("merchant_stock is there: this database was stocked already (--undo first)")
    prh = sqlite3.connect(f"file:{os.path.abspath(a.prh)}?mode=ro&immutable=1", uri=True)
    patterns = load_patterns(db)
    measured = Measured()
    files = measured.load(a.prices)
    measured.add_db(db, patterns)
    measured.fill_items(patterns)
    if not measured.items:
        ap.error("no measured prices: give --prices (an eqoa harvest), or import one first")
    p = plan(db, prh, measured, patterns)
    report(db, p, patterns, measured, files)
    if not a.apply:
        print("(report only: --apply writes them)")
        return 0
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    rows = apply(db, p)
    print(f"wrote {rows} catalog rows for {len(p.stock)} merchants, {len(p.prices)} prices, "
          f"{len(p.reprice)} prices of the two exceptions and {len(p.keyed)} npc_id keys; restart the server")
    return 0


if __name__ == "__main__":
    sys.exit(main())
