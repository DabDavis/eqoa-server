#!/usr/bin/env python3
"""Loot categories: which tier of the loot rolls an item is drawn in
(tools/seed_loot_categories.py; handlers/lootcategory.go rolls each category
on its own, at its own rate a kill).

Gear is sorted by the first rule that holds:

  1. legendary by name (LEGENDARY_ITEMS): jib's two, Aura of Karana and Orb of
     Primordial Intellect (the orb's second record, Primordial Orb of
     Intellect, is never placed), the orb's twin orb of spiritual fortitude
     (the same slot, level and amounts, wisdom for intelligence), and the two
     items Allakhazam's players of 2004 called one of a kind on their server;
  2. by Allakhazam's players' word (ARCHIVE: its item pages' comments,
     2003-2005, as tools/fetch_item_pages.py keeps them): "very rare" ultra
     rare, "a rare drop" super rare (jib's rare, 1 in 1,000 and so);
  3. by its stats against its level's (score / the median score of the items
     of its level within LEVEL_SPAN, every slot): ULTRA_RATIO times or more
     ultra rare, SUPER_RATIO super rare, RARE_RATIO rare (on the 02:05 data of
     the 3,279 items of the pooled tables: 20, 97 and 340, the top 0.6%, 3.6%
     and 14%), anything else uncommon (plain gear, a ware, an item of no wear
     slot that a table holds).

A sale price is not a rule: of the pooled tables' items only those merchants
sell carry one, and most of those prices are ours (tools/stock_merchants.py).
The jewelers' gems and the modifiers keep their rarity as Prima prints it (the
craft tables' weights: 30 common, 9 uncommon, 3 rare, 1 ultra rare).

An item's stats are scored in one currency, an attribute point, by the
jewelers' metals (tools/seed_tradeskills.py, Prima's tables): at mithril a
common gem gives 40 of an attribute or a resist, 85 armor class or 125 hit
points or power, so armor class counts 0.47 and hit points and power 0.32 a
point. A point of health or power over time counts 5 and a weapon's proc 20
(ours). Armor and shields count only the armor class above 1.25 times their
slot's plain median at their level, weapons only the damage above 1.25 times
their kind's (40 points for the median's worth more at level 48, in
proportion to level: ours): plain gear varies by about that much.

This server's kills, for the legendary weights: DAILY_PLAYERS (18 accounts
logged in during the last day of the live data of 2026-10-30 01:30 UTC; 33 in
its last week) x HOURS_A_DAY (2, ours) x KILLS_AN_HOUR (20: tools/drop_rates.py's
minute and a half a kill while hunting, and half of a session hunting: ours)
x 365 x YEARS: 2,628,000 kills in ten years. A GM should lower the legendary
weights if the server grows.

    python3 -E -s tools/loot_tiers.py --db eqoa.db [--tsv categories.tsv]   # every item's category and why
"""
import argparse
import collections
import os
import statistics
import sqlite3
import sys

UNCOMMON, RARE, SUPER, ULTRA, LEGENDARY = "uncommon", "rare", "super rare", "ultra rare", "legendary"
GEAR = (UNCOMMON, RARE, SUPER, ULTRA, LEGENDARY)
RARE_RATIO, SUPER_RATIO, ULTRA_RATIO = 2.0, 3.0, 4.0  # stats this many times its level's median
LEVEL_SPAN = 3     # levels either side an item's peers are of

LEGENDARY_COUNT, YEARS = 5, 10
DAILY_PLAYERS, HOURS_A_DAY, KILLS_AN_HOUR = 18, 2, 20

LEGENDARY_ITEMS = {
    "aura of karana": "jib (SOE's live game): so rare only about five were on a server after ten years",
    "orb of primordial intellect": "jib (SOE's live game): so rare only about five were on a server after ten years",
    "primordial orb of intellect": "the Orb of Primordial Intellect's second record (the same slot, level and stats)",
    "orb of spiritual fortitude": "the Orb of Primordial Intellect's twin: the same slot, level and amounts, "
                                  "wisdom for intelligence",
    "mithril flamberge": "Allakhazam's page: a player in 2004 called theirs the only one on the server",
    "grimblade": "Allakhazam's page: a player called it ultra rare, selling for over 500,000 tunar",
}
SECOND_RECORDS = {"primordial orb of intellect"}  # a second record of an item (stats alike, "Add a real description."): never placed
ARCHIVE = {  # Allakhazam's item pages, its players' comments of 2003-2005
    "grimsplitter": (ULTRA, "Allakhazam's page: called very rare, wanted by many"),
    "blackened iron choppa": (SUPER, "Allakhazam's page: called a rare drop"),
    "huntlord's bracers": (SUPER, "Allakhazam's page (Storm's Edge): listed among the melee rares"),
    "minstrel's edge": (SUPER, "Allakhazam's page (Storm's Edge): listed among the melee rares"),
    "icescale boots": (SUPER, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale bracers": (SUPER, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale gloves": (SUPER, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale helm": (SUPER, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale leggings": (SUPER, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale tunic": (SUPER, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
}

ATTRS = ("str", "sta", "agi", "dex", "wis", "intelligence", "cha")
RESISTS = ("FR", "LR", "CR", "PR", "DR", "AR")
ARMOR_SLOTS = {1, 2, 5, 7, 10, 11, 19}  # helm, robe, chest, bracers, legs, boots, gloves (claude-notes/dump-weapons)
SHIELD_SLOTS = {13}
JEWEL_SLOTS = {3, 4, 6, 8, 9}           # earring, neck, bracelet, ring, belt
HELD_SLOTS = {18}
WEAPON_SLOTS = {12, 14, 15, 16}         # primary, either hand, two-hand, ranged
AC_POINT, HP_POINT, OVER_TIME, PROC, DOUBLE_DAMAGE = 0.47, 0.32, 5, 20, 40
PLAIN_SPREAD = 1.25  # plain gear's armor class or damage counts only above this times its slot's plain median
ATTR_OVER = 10       # an attribute or resist counts at most its item's level + this

COLUMNS = ("patternid", "itemname", "levelreq", "equipslot", "attacktype", "weapondamage", "AC", "HPMAX", "POWMAX",
           "HoT", "PoT", "weaponproc") + ATTRS + RESISTS


def group(slot):
    """The slot group an item is compared within, None when not worn."""
    if slot in ARMOR_SLOTS:
        return f"armor {slot}"
    if slot in SHIELD_SLOTS:
        return "shield"
    if slot in JEWEL_SLOTS:
        return "jewelry"
    if slot in HELD_SLOTS:
        return "held"
    if slot in WEAPON_SLOTS:
        return "weapon"
    return None


def magic(it):
    """Its stats in attribute points, armor class and damage aside."""
    g = lambda k: max(0, it.get(k) or 0)
    pts = sum(g(k) for k in ATTRS) + sum(g(k) for k in RESISTS)
    pts += HP_POINT * (g("HPMAX") + g("POWMAX")) + OVER_TIME * (g("HoT") + g("PoT"))
    if it.get("weaponproc"):
        pts += PROC
    return pts


def items(db):
    """Every item: {patternid: dict of COLUMNS}, as scored: an armor piece or
    shield with no armor class whose largest attribute is above its level +
    ATTR_OVER has that attribute read as its armor class (EQOA-Data's records
    shift a label: its Tundra Tear, a level 32 shield, "agi": 289 and no armor
    class; 164 of the arctic table's 196 armor pieces and shields), and an
    attribute counts at most its level + ATTR_OVER."""
    have = {r[1] for r in db.execute("PRAGMA table_info(itempattern)")}
    cols = [c if c in have else f"0 AS {c}" for c in COLUMNS]
    out = {}
    for r in db.execute(f"SELECT {', '.join(cols)} FROM itempattern"):
        it = dict(zip(COLUMNS, r))
        lv = it["levelreq"] or 0
        g = group(it["equipslot"])
        top = max(ATTRS, key=lambda k: it[k] or 0)
        if g and (g.startswith("armor") or g == "shield") and not it["AC"] and (it[top] or 0) > lv + ATTR_OVER:
            it["AC"], it[top] = it[top], 0
        for k in ATTRS + RESISTS:
            it[k] = min(it[k] or 0, lv + ATTR_OVER)
        out[r[0]] = it
    return out


def score(every):
    """Each worn item's score and its level's median: {patternid: (score, median)}."""
    worn = {pid: it for pid, it in every.items() if group(it["equipslot"]) and 1 <= (it["levelreq"] or 0) <= 70}
    plain_ac = collections.defaultdict(list)   # (group, level) -> plain items' armor class
    plain_dmg = collections.defaultdict(list)  # (level, attack type) -> plain weapons' damage
    for it in worn.values():
        if magic(it) == 0:
            g, lv = group(it["equipslot"]), it["levelreq"]
            if (it["AC"] or 0) > 0:
                plain_ac[(g, lv)].append(it["AC"])
            if g == "weapon" and (it["weapondamage"] or 0) > 0:
                plain_dmg[(lv, it["attacktype"])].append(it["weapondamage"])

    def usual(table, key):
        """The plain median within LEVEL_SPAN of the key's level (its second part)."""
        a, lv = key
        vals = [v for l in range(lv - LEVEL_SPAN, lv + LEVEL_SPAN + 1) for v in table.get((a, l), ())]
        return statistics.median(vals) if vals else 0
    out = {}
    for pid, it in worn.items():
        g, lv = group(it["equipslot"]), it["levelreq"]
        s = magic(it)
        if g.startswith("armor") or g == "shield":
            base = usual(plain_ac, (g, lv))
            if base:
                s += AC_POINT * max(0.0, (it["AC"] or 0) - PLAIN_SPREAD * base)
        else:
            s += AC_POINT * max(0, it["AC"] or 0)
        if g == "weapon":
            vals = [v for l in range(lv - LEVEL_SPAN, lv + LEVEL_SPAN + 1) for v in plain_dmg.get((l, it["attacktype"]), ())]
            base = statistics.median(vals) if vals else 0
            if base:
                s += DOUBLE_DAMAGE * max(0.0, (it["weapondamage"] or 0) / base - PLAIN_SPREAD) * lv / 48
        out[pid] = s
    scored = collections.defaultdict(list)
    for pid, s in out.items():
        if s > 0:
            scored[worn[pid]["levelreq"]].append(s)
    med = {}
    for lv in range(1, 71):
        near_s = [s for l in range(lv - LEVEL_SPAN, lv + LEVEL_SPAN + 1) for s in scored.get(l, ())]
        med[lv] = statistics.median(near_s) if near_s else 0
    return {pid: (s, med.get(worn[pid]["levelreq"], 0)) for pid, s in out.items()}


CRAFT_TABLES = ("craft gems", "craft modifiers")  # tools/seed_tradeskills.py's, rolled by every creature
CRAFT_RARITY = {30: "common", 9: "uncommon", 3: "rare", 1: "ultra rare"}  # tools/seed_tradeskills.py's LOOT_WEIGHT


def crafted(db):
    """The craft tables' items: {patternid: (table, Prima's rarity)} by their weight there."""
    if not db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'loottables'").fetchone()[0]:
        return {}
    out = {}
    for t, pid, w in db.execute(f"SELECT tablename, patternid, weight FROM loottables WHERE tablename IN "
                                f"({', '.join('?' * len(CRAFT_TABLES))})", CRAFT_TABLES):
        out[pid] = (t, CRAFT_RARITY.get(w, "common"))
    return out


def categories(db):
    """Every item's gear category and why: {patternid: (category, why)}."""
    every = items(db)
    scores = score(every)
    out = {}
    for pid, it in every.items():
        name = " ".join((it["itemname"] or "").split()).lower()
        if name in LEGENDARY_ITEMS:
            out[pid] = (LEGENDARY, LEGENDARY_ITEMS[name])
        elif name in ARCHIVE:
            out[pid] = ARCHIVE[name]
        elif pid not in scores:
            out[pid] = (UNCOMMON, "no stats to weigh: no wear slot or no level")
        else:
            s, med = scores[pid]
            q = s / med if med > 0 else 0
            if q >= ULTRA_RATIO:
                out[pid] = (ULTRA, f"stats {q:.1f} times its level's median")
            elif q >= SUPER_RATIO:
                out[pid] = (SUPER, f"stats {q:.1f} times its level's median")
            elif q >= RARE_RATIO:
                out[pid] = (RARE, f"stats {q:.1f} times its level's median")
            elif s <= 0:
                out[pid] = (UNCOMMON, "no stats: plain gear")
            else:
                out[pid] = (UNCOMMON, f"stats {q:.1f} times its level's median")
    return out


def kills_in(years=YEARS, daily=DAILY_PLAYERS, hours=HOURS_A_DAY, an_hour=KILLS_AN_HOUR):
    """The server's kills in years."""
    return daily * hours * an_hour * 365 * years


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--tsv", help="write every item's category and why here")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db)
    c = categories(db)
    names = {pid: n for pid, n in db.execute("SELECT patternid, itemname FROM itempattern")}
    print("items by category: " + ", ".join(f"{k} {v}" for k, v in collections.Counter(x for x, _ in c.values()).most_common()))
    for pid, (cat, why) in sorted(c.items()):
        if cat == LEGENDARY:
            print(f"  legendary: {names[pid]} ({pid}): {why}")
    print(f"this server's kills in {YEARS} years: {kills_in():,}")
    if a.tsv:
        with open(a.tsv, "w") as f:
            f.write("patternid\titem\tcategory\twhy\n")
            for pid, (cat, why) in sorted(c.items()):
                f.write(f"{pid}\t{' '.join((names[pid] or '').split())}\t{cat}\t{why}\n")


if __name__ == "__main__":
    main()
