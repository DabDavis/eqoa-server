#!/usr/bin/env python3
"""Loot tiers: how often an item drops, a kill, by what kind of item it is
(the user, 2026-10-30, relaying jib, a GM who played SOE's live game:
"uncommon drops 1/100, rares 1/1000, some things like aura of karana, orb of
primordial were so rare only 5 on server after 10 years").

Every item is common, uncommon, rare or legendary, by the first rule that
holds:

  1. legendary by name (LEGENDARY_ITEMS): jib's two, Aura of Karana and Orb of
     Primordial Intellect (with the orb's second record, Primordial Orb of
     Intellect), the orb's twin orb of spiritual fortitude (the same slot,
     level and amounts, wisdom for intelligence), and the two items
     Allakhazam's players of 2004 called one of a kind on their server;
  2. by Allakhazam's players' word (ARCHIVE: its item pages' comments,
     2003-2005, as tools/fetch_item_pages.py keeps them);
  3. not worn (a material, a trophy, food, a potion), or of no level: common;
  4. sold by a merchant here (an npcitem row): common;
  5. worn with no stats (plain gear: its armor class or damage alone, at most
     its slot's usual for its level): common;
  6. stats at least RARE_RATIO times the median of the items of its level
     (within LEVEL_SPAN, every slot): rare;
  7. else (magic): uncommon.

An item's stats are scored in one currency, an attribute point, by the
jewelers' metals (tools/seed_tradeskills.py, Prima's tables): at mithril a
common gem gives 40 of an attribute or a resist, 85 armor class or 125 hit
points or power, so armor class counts 0.47 and hit points and power 0.32 a
point. A point of health or power over time counts 5 and a weapon's proc 20
(ours). Armor and shields count only the armor class above 1.25 times their
slot's plain median at their level, weapons only the damage above 1.25 times
their kind's (40 points for the median's worth more at level 48, in
proportion to level: ours): plain gear varies by about that much.

Rates a kill, an item at its creature: common 1 in 10 (ours: the next step of
jib's ladder), uncommon 1 in 100 and rare 1 in 1,000 (jib's); a legendary
item's its own, LEGENDARY_COUNT over the kills its creatures will take in
YEARS on this server (legendary_rate), so that about five exist after ten
years, as on a live server.

This server's kills: DAILY_PLAYERS (18 accounts logged in during the last day
of the live data of 2026-10-30 01:30 UTC; 33 in its last week) x HOURS_A_DAY
(2, ours) x KILLS_AN_HOUR (20: tools/drop_rates.py's minute and a half a kill
while hunting, and half of a session hunting: ours) x 365 x YEARS: 2,628,000
kills in ten years. A legendary's creatures take the share of those its
spawns are of every spawn that can be attacked (every spawn as often killed
as another: ours). On SOE's server (about 1,000 daily players over its ten
years, 3 hours each: an estimate) the same item came out about 80 times
rarer a kill; it is the server's few players that make five need a higher
rate here, and a GM should lower it if the server grows.

    python3 -E -s tools/loot_tiers.py --db eqoa.db [--tsv tiers.tsv]   # every item's tier and why
"""
import argparse
import collections
import os
import statistics
import sqlite3
import sys

COMMON, UNCOMMON, RARE, LEGENDARY = "common", "uncommon", "rare", "legendary"
TIERS = (COMMON, UNCOMMON, RARE, LEGENDARY)
TIER_RATE = {COMMON: 0.1, UNCOMMON: 0.01, RARE: 0.001}
RARE_RATIO = 2.0   # stats this many times its level's median: rare (the top 11% of the items with stats)
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
    "grimsplitter": (RARE, "Allakhazam's page: called very rare, wanted by many"),
    "blackened iron choppa": (RARE, "Allakhazam's page: called a rare drop"),
    "huntlord's bracers": (RARE, "Allakhazam's page (Storm's Edge): listed among the melee rares"),
    "minstrel's edge": (RARE, "Allakhazam's page (Storm's Edge): listed among the melee rares"),
    "icescale boots": (RARE, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale bracers": (RARE, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale gloves": (RARE, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale helm": (RARE, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale leggings": (RARE, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
    "icescale tunic": (RARE, "Allakhazam's page (Storm's Edge): Icescale listed among the melee rares"),
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


def tiers(db):
    """Every item's tier and why: {patternid: (tier, why)}."""
    every = items(db)
    scores = score(every)
    sold = set()
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'npcitem'").fetchone()[0]:
        sold = {pid for (pid,) in db.execute("SELECT DISTINCT patternid FROM npcitem")}
    out = {}
    for pid, it in every.items():
        name = " ".join((it["itemname"] or "").split()).lower()
        if name in LEGENDARY_ITEMS:
            out[pid] = (LEGENDARY, LEGENDARY_ITEMS[name])
        elif name in ARCHIVE:
            out[pid] = ARCHIVE[name]
        elif pid not in scores and not group(it["equipslot"]):
            out[pid] = (COMMON, "no wear slot: a material, a trophy, food or a potion")
        elif pid not in scores:
            out[pid] = (COMMON, "no level to weigh its stats against")
        elif pid in sold:
            out[pid] = (COMMON, "sold by a merchant here")
        else:
            s, med = scores[pid]
            if s <= 0:
                out[pid] = (COMMON, "no stats: plain gear")
            elif med > 0 and s >= RARE_RATIO * med:
                out[pid] = (RARE, f"stats {s / med:.1f} times its level's median")
            else:
                out[pid] = (UNCOMMON, f"stats ({s / med:.1f} times its level's median)" if med else "stats")
    return out


def kills_in(years=YEARS, daily=DAILY_PLAYERS, hours=HOURS_A_DAY, an_hour=KILLS_AN_HOUR):
    """The server's kills in years."""
    return daily * hours * an_hour * 365 * years


def legendary_rate(carrier_spawns, all_spawns, kills=None):
    """The chance a kill that gives LEGENDARY_COUNT in the kills (default the
    server's in YEARS) when its creatures are carrier_spawns of all_spawns."""
    kills = kills_in() if kills is None else kills
    if carrier_spawns <= 0 or all_spawns <= 0 or kills <= 0:
        return 0.0
    return min(TIER_RATE[RARE], LEGENDARY_COUNT / (kills * carrier_spawns / all_spawns))


def rate(tier, legendary=0.0):
    return legendary if tier == LEGENDARY else TIER_RATE[tier]


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--tsv", help="write every item's tier and why here")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db)
    t = tiers(db)
    names = {pid: n for pid, n in db.execute("SELECT patternid, itemname FROM itempattern")}
    print("items by tier: " + ", ".join(f"{k} {v}" for k, v in collections.Counter(x for x, _ in t.values()).most_common()))
    for pid, (tier, why) in sorted(t.items()):
        if tier == LEGENDARY:
            print(f"  legendary: {names[pid]} ({pid}): {why}")
    print(f"this server's kills in {YEARS} years: {kills_in():,}")
    if a.tsv:
        with open(a.tsv, "w") as f:
            f.write("patternid\titem\ttier\twhy\n")
            for pid, (tier, why) in sorted(t.items()):
                f.write(f"{pid}\t{' '.join((names[pid] or '').split())}\t{tier}\t{why}\n")


if __name__ == "__main__":
    main()
