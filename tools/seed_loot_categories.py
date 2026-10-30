#!/usr/bin/env python3
"""The loot categories seeded (handlers/lootcategory.go): presets of rates,
the pools each category draws from, every creature's region and profile, and
each creature's own trash (the user and jib, a GM of SOE's game, 2026-10-30:
rates per tier per kill; "500 more items change nothing about how much
drops").

Presets (lootpreset, lootrate), each a set of rates by profile:

  - **jib** (active): trash by jib's counts (0-6 items a kill at 25, 31, 25,
    12, 5, 1.5 and 0.5%: 1.475 a kill, each between the creature's two
    trash items), gems 20% (88% common, 10% uncommon, 2% rare: Prima's rare
    and ultra rare share the 2% three to one, ours), modifiers 3% (the old
    roll's), uncommon 1 in 30, rare 1 in 240, super rare 1 in 1,200, and a
    global roll 1 in 10,000 on any kill from one server-wide list.
  - **user**: stacks 1 in 2 kills (1-3 of one trash item), gems 5%,
    modifiers 3%, uncommon 15%, rare 3%, super rare 1 in 1,000, ultra rare
    1 in 10,000 and legendary at the rate that gives about five of each on
    the server in ten years.

Profiles: xp (every creature's but these), named (the rare and raid mobs of
EQOA-Data's map) and boss (one with a "boss <name>" loot table): their gear
tiers three times xp's (ours); trash (no creature yet: a GM's choice), trash
and the global roll alone.

Pools (lootpool): the gear of EQOA-Data's tables as tools/seed_camp_loot.py
and tools/seed_region_loot.py hold them, each item in its table's region
(a biome's, marine, rathe mountains, odus; the generic table's every
region's, "*") for its window, in the category tools/loot_tiers.py sorts it
into; the legendaries (any region and level); the global list (the ultra
rares and the legendaries, any region, any level); the jewelers' gems and the modifiers off the craft tables; and each
creature's own two trash items: the trophies (stackable, tradeable, worn by
no one, in no quest, recipe, catch or shop) whose whole creature its name
holds ("fire ant": a fire ant, a deadly fire ant, not an ant soldier; a noun
used as another's adjective, "giant badger", is no giant). A
trophy with no price is given its creatures' median level times what
Sandstorm's priced trophies fetch a level (PRICE_PER_LEVEL, fitted).

The legendary weights: in the global list each legendary's weight gives it
LEGENDARY_COUNT of the list's draws in YEARS (this server's kills times 1 in
10,000: tools/loot_tiers.py's kills), the ultra rares the rest. The user
preset's legendary category is a server-wide list too, the legendaries at one
weight, its rate LEGENDARY_COUNT of each in YEARS of the server's kills: a list
by region and level cannot give each its five at any one rate (Mithril
Flamberge, alone in many creatures' pools, would come eleven times to Aura of
Karana's one).

Creatures (lootcreature): every name that can be attacked, its region the
commonest of its spawns' (a Tunaria quadrant's biome on EQOA-Data's map,
Tunaria's waters marine by --ground, world 1 the Rathe, 2 Odus, 3 Lavastorm, 4
the Plane of Sky, 5 the Secrets); players' pets recorded off the live game
region "-".

The loot tables the categories replace are taken away from the creatures:
the camp and region seeds' bindings to the pooled tables, and the "*" rows
of the craft gems and modifiers (now the gems and modifiers categories). The
bosses' own tables and a GM's stay. Every row written, binding taken away and
price set is logged in loot_category_seed; --undo takes back those still as
written. --report writes the review (both presets an hour by level band).
Report only by default. The server reads all of it at every corpse: no
restart (handlers/lootcategory.go itself needs the restart that brings it).

    python3 -E -s tools/seed_loot_categories.py --db eqoa.db --eqoa-data ../external/EQOA-Data \\
        --ground worlds/ground [--report loot.md] [--apply | --undo]
"""
import argparse
import collections
import datetime
import json
import math
import os
import re
import shutil
import sqlite3
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import gen_camp_teles as camps  # noqa: E402
import loot_tiers as lt  # noqa: E402
import seed_camp_loot as campseed  # noqa: E402
import seed_region_loot as region  # noqa: E402

SOURCE = "category-seed"
TRASH, GEMS, MODS, GLOBAL = "trash", "gems", "modifiers", "global"
UNCOMMON, RARE, SUPER, ULTRA, LEGENDARY = lt.UNCOMMON, lt.RARE, lt.SUPER, lt.ULTRA, lt.LEGENDARY
GEAR_TIERS = (UNCOMMON, RARE, SUPER, ULTRA)
GENERIC = "generic"
POOLED = set(region.BIOMES) | {region.RATHE, region.MARINE, GENERIC} | set(region.ODUS)
POOLED_SOURCES = ("camp-seed", region.SOURCE)
TABLE_REGION = {**{t: t for t in region.BIOMES}, region.RATHE: region.RATHE, region.MARINE: region.MARINE,
                GENERIC: "*", **{t: "odus" for t in region.ODUS}}
WORLD_REGION = {1: region.RATHE, 2: "odus", 3: "lavastorm", 4: "plane of sky", 5: "secrets"}
ANY_LEVEL = (1, 100)
SCROLL_ART = -607649328
JIB_COUNTS = [25, 31, 25, 12, 5, 1.5, 0.5]
GEM_SHARES = {"common": 88, "uncommon": 10, "rare": 1.5, "ultra rare": 0.5}
WEIGHT_SCALE = 100          # a pool row's weight unit: shares in hundredths
GLOBAL_ULTRA_WEIGHT = 100   # an ultra rare's weight in the global list
PROFILE_SCALE = 3           # named and boss: the gear tiers' rates times this (ours)
PRICE_PER_LEVEL_DEFAULT = 3.0

# preset -> (active, note, xp rows [(category, rate, counts, minstack, maxstack)])
PRESETS = {
    "jib": (1, "jib's rates (a GM of SOE's game, 2026-10-30): trash counts, gems 20%, uncommon 1/30, rare 1/240, "
               "super rare 1/1200, a global roll 1/10000 on any kill",
            [(TRASH, 0.0, JIB_COUNTS, 1, 1), (GEMS, 0.20, None, 1, 1), (MODS, 0.03, None, 1, 1),
             (UNCOMMON, 1 / 30, None, 1, 1), (RARE, 1 / 240, None, 1, 1), (SUPER, 1 / 1200, None, 1, 1),
             (GLOBAL, 1 / 10000, None, 1, 1)]),
    "user": (0, "the user's rates (2026-10-30): stacks 1 in 2, uncommon 15%, rare 3%, super rare 1/1000, ultra rare "
                "1/10000, legendary five on the server in ten years",
             [(TRASH, 0.5, None, 1, 3), (GEMS, 0.05, None, 1, 1), (MODS, 0.03, None, 1, 1),
              (UNCOMMON, 0.15, None, 1, 1), (RARE, 0.03, None, 1, 1), (SUPER, 0.001, None, 1, 1),
              (ULTRA, 0.0001, None, 1, 1), (LEGENDARY, None, None, 1, 1)]),
}
ORDER = [TRASH, GEMS, MODS, UNCOMMON, RARE, SUPER, ULTRA, LEGENDARY, GLOBAL]

PART = (r"pelt|hide|fang|claw|tooth|teeth|bone|bones|silk|whisker|whiskers|tail|feather|feathers|venom|scale|scales|"
        r"wing|wings|skull|ichor|shell|fur|meat|talon|egg|eye|leg|legs|antennae|antenna|carapace|chitin|pincer|horn|"
        r"tusk|ear|ears|heart|blood|slime|husk|segment|segments|mandible|cornea|fragment")
PART_RE = re.compile(r"\b(" + PART + r")\b", re.I)
GRADE = set("broken cracked chipped damaged torn ruined fractured smashed shattered shredded pristine intact fine tough "
            "small large huge bent patch clump chunk slice piece bit of the a an mixture brittle misshapen preserved "
            "matted perfect prime venomous flawless".split())

LOG_DDL = """CREATE TABLE IF NOT EXISTS loot_category_seed (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    kind    TEXT NOT NULL,     -- row (a row written) | unbound (an npcloot row taken away) | price (an itemcost set)
    tbl     TEXT NOT NULL,     -- lootpreset | lootrate | lootpool | lootcreature | npcloot | itempattern
    ref     TEXT NOT NULL,     -- the row: lootpool's id, the other tables' primary key as json, the patternid
    old     TEXT NOT NULL,     -- unbound: the npcloot row (json); price: the itemcost before
    new     TEXT NOT NULL,     -- row: the row written (json); price: the itemcost set
    applied TEXT NOT NULL
)"""


GO_SCHEMA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "internal", "database", "lootcategory.go")


def go_schema():
    """The four tables' DDL, read out of the server's own source (lootCategorySchema), so the two never differ."""
    src = open(GO_SCHEMA).read()
    m = re.search(r"const lootCategorySchema = `(.*?)`", src, re.S)
    if not m:
        sys.exit(f"{GO_SCHEMA}: no lootCategorySchema")
    return m.group(1)


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def singular(w):
    return w[:-1] if w.endswith("s") and len(w) > 3 and not w.endswith("ss") else w


def words(name):
    return [singular(w) for w in re.findall(r"[a-z]+", (name or "").lower())]


class World:
    """The creatures: their spawns, regions, profiles; and every attackable spawn's count."""

    def __init__(self, db, data=None, ground=None):
        self.db = db
        zones, rows = camps.load(db, data) if data else ({"zones": [], "towns": [], "pois": []}, [])
        self.zones, self.fitted = zones, (camps.fits(zones, rows) if data else {})
        self.ground, self._envs = ground, {}
        self.pets = set()
        if has_table(db, "npcaggro"):
            self.pets = {campseed.npc_key(n) for (n,) in db.execute(
                "SELECT npc_name FROM npcaggro WHERE note LIKE 'a summoned pet''s name%'")}
        bosses = {n for (n,) in db.execute("SELECT DISTINCT npc_name FROM npcloot WHERE tablename LIKE 'boss %'")}
        # the named creatures: EQOA-Data's map's rare and raid mobs
        named = {camps.clean(p["name"]) for p in zones.get("pois", []) if p.get("category") in ("rare-mob", "raid-mob")}
        self.spawns = collections.defaultdict(list)  # name -> [(world, level)]
        votes = collections.defaultdict(collections.Counter)
        self.all_spawns = 0
        for name, world, x, y, z, lvl, typ in db.execute(
                "SELECT npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type FROM npcs WHERE npc_name IS NOT NULL"):
            k = campseed.npc_key(name)
            if not k or (typ or 0) & 0x80:
                continue
            self.all_spawns += 1
            if "dummy" in k:
                continue
            self.spawns[k].append((world, lvl or 0))
            votes[k][self.region_at(world, x or 0.0, y or 0.0, z or 0.0)] += 1
        self.region = {k: ("-" if k in self.pets else (v.most_common(1)[0][0] or "")) for k, v in votes.items()}
        self.profile = {}
        for k in self.spawns:
            if k in self.pets:
                continue
            if k in bosses:
                self.profile[k] = "boss"
            elif k in named:
                self.profile[k] = "named"
            else:
                self.profile[k] = ""

    def region_at(self, world, x, y, z):
        if world in WORLD_REGION:
            return WORLD_REGION[world]
        if world != 0:
            return None
        if self.ground:
            if world not in self._envs:
                path = os.path.join(self.ground, f"{world}.env")
                self._envs[world] = region.Env(path) if os.path.exists(path) else None
            e = self._envs[world]
            if e is not None and e.flags(x, y + 0.5, z) & 3:
                return region.MARINE
        if 0 in self.fitted:
            a, b, c, d = self.fitted[0][0]
            biome, _ = campseed.biome_at(self.zones, (x - b) / a, (z - d) / c)
            return biome.lower() if biome else None
        return None


def excluded_items(db):
    """What no trash may be: quest items, crafting items, catches, wares."""
    out = set()
    for (items,) in db.execute("SELECT items FROM queststeps WHERE items <> ''") if has_table(db, "queststeps") else []:
        out |= {int(x) for x in re.findall(r"\d+", items)}
    if has_table(db, "quests"):
        for c in [r[1] for r in db.execute("PRAGMA table_info(quests)")]:
            if "reward" in c or "item" in c:
                for (v,) in db.execute(f"SELECT {c} FROM quests"):
                    out |= {int(x) for x in re.findall(r"\d+", str(v or ""))}
    for t, col in (("quest_import_items", "patternid"), ("fishcatch", "itemid"), ("npcitem", "patternid")):
        if has_table(db, t):
            out |= {p for (p,) in db.execute(f"SELECT {col} FROM {t}")}
    out |= {p for (p,) in db.execute("SELECT patternid FROM loottables WHERE tablename LIKE 'craft %'")}
    if has_table(db, "craftrecipes"):
        for ing, prod in db.execute("SELECT ingredients, product FROM craftrecipes"):
            out.add(prod)
            try:
                out |= {int(i.get("item") or i.get("patternid") or 0) for i in json.loads(ing)}
            except (ValueError, TypeError, AttributeError):
                pass
    return out


def creature_phrase(name):
    """A trophy's creature: (its words, its head noun), grades and parts aside."""
    ws = [w for w in words(name) if w not in GRADE and not PART_RE.fullmatch(w)]
    return ws, (ws[-1] if ws else None)


def trophy_fits(phrase, head, creature, nouns, strict):
    """Whether a trophy of that creature phrase drops from a creature of that name."""
    cw = words(creature)
    if head not in cw:
        return False
    i = len(cw) - 1 - cw[::-1].index(head)
    if i + 1 < len(cw) and cw[i + 1] in nouns:  # "giant badger": the giant is the badger's word
        return False
    return not strict or all(w in cw for w in phrase)


def trash_plan(db, w):
    """Each creature's two trash items: ({name: [patternid, ...]}, {patternid: (price before, price)}, trophies)."""
    bad = excluded_items(db)
    trophies = {}
    for pid, name, cost, ms, trade in db.execute(
            "SELECT patternid, itemname, itemcost, maxstack, trade FROM itempattern WHERE equipslot < 1 AND itemicon <> ?",
            (SCROLL_ART,)):
        if PART_RE.search(name or "") and not trade and (ms or 0) > 1 and pid not in bad:
            phrase, head = creature_phrase(name)
            if head:
                trophies[pid] = (name, cost or 0, phrase, head)
    nouns = {h for _, _, _, h in trophies.values()}
    assign, used = {}, collections.Counter()
    for k in sorted(w.spawns):
        if w.region.get(k) == "-":
            continue
        fit = [p for p, (_, _, ph, hd) in trophies.items() if trophy_fits(ph, hd, k, nouns, True)]
        if fit:
            fit.sort(key=lambda p: (used[p], p))
            assign[k] = fit[:2]
            for p in fit[:2]:
                used[p] += 1
    # prices: what Sandstorm's priced trophies fetch a level of their creatures
    levels = collections.defaultdict(list)
    for k, ps in assign.items():
        for p in ps:
            levels[p] += [lv for _, lv in w.spawns[k] if lv > 0]
    ratios = []
    for pid, name, cost in db.execute("SELECT patternid, itemname, itemcost FROM itempattern WHERE itemcost > 0"):
        if PART_RE.search(name or ""):
            phrase, head = creature_phrase(name)
            lv = [l for k in w.spawns if head and trophy_fits(phrase, head, k, nouns, True) for _, l in w.spawns[k] if l > 0]
            if lv:
                ratios.append(cost / statistics.median(lv))
    per_level = statistics.median(ratios) if len(ratios) >= 5 else PRICE_PER_LEVEL_DEFAULT
    prices = {}
    for p in sorted(used):
        name, cost, _, _ = trophies[p]
        if cost <= 0 and levels[p]:
            prices[p] = (cost, max(1, round(per_level * statistics.median(levels[p]))))
    return assign, prices, trophies, per_level, len(ratios)


def gear_rows(db, cats):
    """The gear pools: [(category, region, patternid, weight, lo, hi)] and the items placed in each category."""
    craft = set(lt.crafted(db))
    second = {pid for pid, n in db.execute("SELECT patternid, itemname FROM itempattern")
              if " ".join((n or "").split()).lower() in lt.SECOND_RECORDS}
    rows, seen = [], set()
    for t, pid, lo, hi in db.execute(
            f"SELECT tablename, patternid, minlevel, maxlevel FROM loottables WHERE weight > 0 AND tablename IN "
            f"({', '.join('?' * len(POOLED))}) ORDER BY id", sorted(POOLED)):
        if pid in craft or pid in second or pid not in cats:
            continue
        key = (cats[pid][0], TABLE_REGION[t], pid, lo, hi)
        if key not in seen:
            seen.add(key)
            rows.append((cats[pid][0], TABLE_REGION[t], pid, WEIGHT_SCALE, lo, hi))
    rows = [r for r in rows if r[0] != LEGENDARY]
    # the legendaries: one server-wide list, any region and level, each the same weight
    for pid, (c, _) in sorted(cats.items()):
        if c == LEGENDARY and pid not in second:
            rows.append((LEGENDARY, "*", pid, WEIGHT_SCALE, *ANY_LEVEL))
    return rows


def legendary_rate(rows, kills):
    """The user preset's legendary rate: the list's items, each LEGENDARY_COUNT in YEARS of the server's kills."""
    n = len({r[2] for r in rows if r[0] == LEGENDARY})
    return lt.LEGENDARY_COUNT * n / kills if kills and n else 0.0


def global_rows(rows, kills, rate):
    """The global list: the ultra rares and the legendaries, any region and level; each legendary's weight its
    LEGENDARY_COUNT of the list's draws in the years. Returns (rows, the legendary weight, draws in the years)."""
    ultras = sorted({r[2] for r in rows if r[0] == ULTRA})
    legs = sorted({r[2] for r in rows if r[0] == LEGENDARY})
    draws = kills * rate
    share = lt.LEGENDARY_COUNT / draws if draws else 0
    total_ultra = GLOBAL_ULTRA_WEIGHT * len(ultras)
    w_leg = max(1, round(share * total_ultra / max(1e-9, 1 - share * len(legs)))) if legs else 0
    out = [(GLOBAL, "*", p, GLOBAL_ULTRA_WEIGHT, *ANY_LEVEL) for p in ultras]
    out += [(GLOBAL, "*", p, w_leg, *ANY_LEVEL) for p in legs]
    return out, w_leg, draws


def craft_rows(db):
    """The gems' and the modifiers' pools off the craft tables: gems by jib's tier shares, modifiers by their weights."""
    out = []
    gems = db.execute("SELECT patternid, weight, minlevel, maxlevel FROM loottables WHERE tablename = 'craft gems'").fetchall()
    by = collections.Counter(lt.CRAFT_RARITY.get(wt, "common") for _, wt, _, _ in gems)
    for pid, wt, lo, hi in gems:
        r = lt.CRAFT_RARITY.get(wt, "common")
        out.append((GEMS, "*", pid, max(1, round(WEIGHT_SCALE * GEM_SHARES[r] / by[r])), lo, hi))
    for pid, wt, lo, hi in db.execute("SELECT patternid, weight, minlevel, maxlevel FROM loottables WHERE tablename = 'craft modifiers'"):
        out.append((MODS, "*", pid, max(1, wt), lo, hi))
    return out


def plan(db, data, ground):
    w = World(db, data, ground)
    cats = lt.categories(db)
    kills = lt.kills_in()
    rows = gear_rows(db, cats)
    leg_rate = legendary_rate(rows, kills)
    glob, w_leg, draws = global_rows(rows, kills, 1 / 10000)
    rows += glob + craft_rows(db)
    assign, prices, trophies, per_level, n_ratios = trash_plan(db, w)
    for k, ps in sorted(assign.items()):
        rows += [(TRASH, "", p, 1, *ANY_LEVEL, k) for p in ps]
    rates = []
    for preset, (active, note, xp) in PRESETS.items():
        for prof in ("xp", "named", "boss", "trash"):
            for i, (cat, rate, counts, lo, hi) in enumerate(xp):
                if cat == LEGENDARY:
                    rate = leg_rate
                if prof in ("named", "boss") and cat in GEAR_TIERS:
                    rate = min(1.0, rate * PROFILE_SCALE)
                if prof == "trash" and cat not in (TRASH, GLOBAL):
                    continue
                rates.append((preset, prof, cat, rate or 0.0, ",".join(f"{c:g}" for c in counts) if counts else "",
                              lo, hi, ORDER.index(cat)))
    presets = [(p, a, n) for p, (a, n, _) in PRESETS.items()]
    creatures = [(k, w.region.get(k) or "", w.profile.get(k, "")) for k in sorted(w.spawns)]
    unbind = [r for r in db.execute("SELECT npc_name, tablename, chance, rolls, note, source FROM npcloot ORDER BY 1, 2")
              if (r[1] in POOLED and r[5] in POOLED_SOURCES) or (r[0] == "*" and r[1] in lt.CRAFT_TABLES)]
    info = dict(world=w, cats=cats, kills=kills, leg_rate=leg_rate, global_leg_weight=w_leg, global_draws=draws,
                trophies=trophies, trash=assign, per_level=per_level, n_ratios=n_ratios)
    return presets, rates, rows, creatures, unbind, prices, info


TABLES = ("lootpreset", "lootrate", "lootpool", "lootcreature")


def apply(db, presets, rates, rows, creatures, unbind, prices):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    log = "INSERT INTO loot_category_seed (kind, tbl, ref, old, new, applied) VALUES (?, ?, ?, ?, ?, ?)"
    made = [t for t in TABLES if not has_table(db, t)]
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        if made:  # a server without handlers/lootcategory.go yet: its tables, made as it makes them
            for stmt in go_schema().split(";"):
                if stmt.strip():
                    db.execute(stmt)
            for t in made:
                db.execute(log, ("table", t, json.dumps([t]), "", "", when))
        for name, active, note in presets:
            db.execute("INSERT INTO lootpreset (name, active, note, source) VALUES (?, ?, ?, ?)", (name, active, note, SOURCE))
            db.execute(log, ("row", "lootpreset", json.dumps([name]), "", json.dumps([active, note]), when))
        for preset, prof, cat, rate, counts, lo, hi, pos in rates:
            db.execute("INSERT INTO lootrate (preset, profile, category, rate, counts, minstack, maxstack, position, source) "
                       "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", (preset, prof, cat, rate, counts, lo, hi, pos, SOURCE))
            db.execute(log, ("row", "lootrate", json.dumps([preset, prof, cat]), "",
                             json.dumps([rate, counts, lo, hi, pos]), when))
        for r in rows:
            cat, rg, pid, wt, lo, hi = r[:6]
            npc = r[6] if len(r) > 6 else ""
            cur = db.execute("INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) "
                             "VALUES (?, ?, ?, ?, ?, ?, ?, ?)", (cat, rg, npc, pid, wt, lo, hi, SOURCE))
            db.execute(log, ("row", "lootpool", str(cur.lastrowid), "", json.dumps([cat, rg, npc, pid, wt, lo, hi]), when))
        for k, rg, prof in creatures:
            db.execute("INSERT INTO lootcreature (npc_name, region, profile, source) VALUES (?, ?, ?, ?)", (k, rg, prof, SOURCE))
            db.execute(log, ("row", "lootcreature", json.dumps([k]), "", json.dumps([rg, prof]), when))
        for r in unbind:
            db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (r[0], r[1]))
            db.execute(log, ("unbound", "npcloot", json.dumps([r[0], r[1]]), json.dumps(list(r)), "", when))
        for pid, (old, new) in prices.items():
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (new, pid))
            db.execute(log, ("price", "itempattern", str(pid), str(old), str(new), when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "loot_category_seed"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back, kept = 0, []
        drop = []
        for kind, tbl, ref, old, new in db.execute(
                "SELECT kind, tbl, ref, old, new FROM loot_category_seed ORDER BY seq DESC").fetchall():
            if kind == "table":
                drop.append(tbl)
            elif kind == "price":
                if db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (int(ref),)).fetchone() == (int(new),):
                    db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (int(old), int(ref)))
                    back += 1
                else:
                    kept.append(f"itempattern {ref}")
            elif kind == "unbound":
                n, t, c, r, note, src = json.loads(old)
                if db.execute("SELECT 1 FROM npcloot WHERE npc_name = ? AND tablename = ?", (n, t)).fetchone() is None:
                    db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                               (n, t, c, r, note, src))
                    back += 1
                else:
                    kept.append(f"npcloot {n!r} on {t!r}")
            elif tbl == "lootpool":
                cat, rg, npc, pid, wt, lo, hi = json.loads(new)
                cur = db.execute("SELECT category, region, npc_name, patternid, weight, minlevel, maxlevel, source FROM lootpool "
                                 "WHERE id = ?", (int(ref),)).fetchone()
                if cur == (cat, rg, npc, pid, wt, lo, hi, SOURCE):
                    db.execute("DELETE FROM lootpool WHERE id = ?", (int(ref),))
                    back += 1
                elif cur is not None:
                    kept.append(f"lootpool {ref}")
            elif tbl == "lootcreature":
                (k,) = json.loads(ref)
                if db.execute("SELECT region, profile, source FROM lootcreature WHERE npc_name = ?", (k,)).fetchone() == \
                        (*json.loads(new), SOURCE):
                    db.execute("DELETE FROM lootcreature WHERE npc_name = ?", (k,))
                    back += 1
                else:
                    kept.append(f"lootcreature {k!r}")
            elif tbl == "lootrate":
                p, pr, c = json.loads(ref)
                rate, counts, lo, hi, pos = json.loads(new)
                cur = db.execute("SELECT rate, counts, minstack, maxstack, position, source FROM lootrate WHERE preset = ? "
                                 "AND profile = ? AND category = ?", (p, pr, c)).fetchone()
                if cur is not None and abs(cur[0] - rate) < 1e-12 and cur[1:] == (counts, lo, hi, pos, SOURCE):
                    db.execute("DELETE FROM lootrate WHERE preset = ? AND profile = ? AND category = ?", (p, pr, c))
                    back += 1
                elif cur is not None:
                    kept.append(f"lootrate {p}/{pr}/{c}")
            elif tbl == "lootpreset":
                (p,) = json.loads(ref)
                if db.execute("SELECT active, note, source FROM lootpreset WHERE name = ?", (p,)).fetchone() == \
                        (*json.loads(new), SOURCE):
                    db.execute("DELETE FROM lootpreset WHERE name = ?", (p,))
                    back += 1
                else:
                    kept.append(f"lootpreset {p!r}")
        for t in drop:  # a table this made, empty again: gone, with its indexes
            if db.execute(f"SELECT count(*) FROM {t}").fetchone()[0] == 0:
                db.execute(f"DROP TABLE {t}")
                if t == "lootpool":
                    db.execute("DELETE FROM sqlite_sequence WHERE name = 'lootpool'")
            else:
                kept.append(f"table {t} (rows of others)")
        db.execute("DROP TABLE loot_category_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {back} changes" + (f"; {len(kept)} changed since left as they are: " + ", ".join(kept[:20]) if kept else ""))


BANDS = [(1, 10), (11, 20), (21, 30), (31, 40), (41, 50), (51, 60), (61, 70)]
PACES = (40, 60)  # kills an hour: tools/drop_rates.py's minute and a half a kill, and jib's 60


def report(db, path, rates, rows, prices, info):
    """The review: both presets an hour by level band, at 40 and 60 kills an hour."""
    w, kills = info["world"], info["kills"]
    names = {pid: " ".join((n or "").split()) for pid, n in db.execute("SELECT patternid, itemname FROM itempattern")}
    cost = {pid: c or 0 for pid, c in db.execute("SELECT patternid, itemcost FROM itempattern")}
    cost.update({p: new for p, (old, new) in prices.items()})
    rate = {(p, c): (r, cnt) for p, prof, c, r, cnt, lo, hi, pos in rates if prof == "xp"}

    def per_kill(preset, cat):
        r, cnt = rate.get((preset, cat), (0.0, ""))
        if cnt:
            ch = [float(x) for x in cnt.split(",")]
            return sum(i * c for i, c in enumerate(ch)) / sum(ch)
        if cat == TRASH:
            lo, hi = [(lo, hi) for p, prof, c, r2, cn, lo, hi, pos in rates if p == preset and prof == "xp" and c == cat][0]
            return r * (lo + hi) / 2
        return r

    pool = collections.defaultdict(list)  # (category, region) -> [(pid, lo, hi)]
    for r in rows:
        if len(r) == 6:
            pool[(r[0], r[1])].append((r[2], r[4], r[5]))

    def pool_at(cat, reg, lv):
        return [p for key in ((cat, reg), (cat, "*")) for p, lo, hi in pool.get(key, ()) if lo <= lv <= hi]

    # what players earn now: a corpse's coins (handlers/corpsecoins.go: its level to twice it) and what its
    # loot tables give, sold at its value (merchant.go: merchantRate 100)
    binds = collections.defaultdict(list)
    for n, t, c, r in db.execute("SELECT npc_name, tablename, chance, rolls FROM npcloot"):
        binds[n].append((t, c, r))
    tables = collections.defaultdict(list)
    for t, pid, wt, lo, hi in db.execute("SELECT tablename, patternid, weight, minlevel, maxlevel FROM loottables WHERE weight > 0"):
        tables[t].append((pid, wt, lo, hi))

    def sold_now(k, lv):
        v = 0.0
        for t, c, r in binds.get(k, []) + binds.get("*", []):
            valid = [(p, wt) for p, wt, lo, hi in tables.get(t, ()) if lo <= lv <= hi]
            total = sum(wt for _, wt in valid)
            for p, wt in valid:
                v += r * c / 100 * wt / total * cost.get(p, 0)
        return v

    L = ["# Loot by category: the review", ""]
    L.append("Built by `tools/seed_loot_categories.py` on the live data of 2026-10-30 02:05 UTC with "
             "tools/seed_region_loot.py, tools/fix_single_spawn_steps.py and tools/fix_armor_class.py applied: what "
             "--apply would write. Nothing is applied anywhere live.")
    L.append("")
    L.append("## The rolls, a kill")
    L.append("")
    L.append("Each category rolls on its own at every corpse, the item drawn fairly (by weight) from the creature's "
             "pool for that category, its level band and region: a pool's size changes which item comes, never how "
             "often its category drops. One set of rolls a corpse, whoever loots it; no pity timers; a kill that "
             "gives no experience rolls the same. Every number is a row of Loot rates in the GM panel.")
    L.append("")
    L.append("| category | jib (active) | user |")
    L.append("|---|---|---|")
    for cat in ORDER:
        cells = []
        for preset in ("jib", "user"):
            r, cnt = rate.get((preset, cat), (None, ""))
            if r is None:
                cells.append("none")
            elif cnt:
                cells.append(f"counts {cnt} (% for 0, 1, 2 ... items: {per_kill(preset, cat):.3f} a kill)")
            elif cat == TRASH:
                cells.append(f"{r:g} a kill, a stack of 1-3")
            else:
                cells.append(f"1 in {1 / r:,.0f} ({r:.4%})" if r else "0")
        L.append(f"| {cat} | {cells[0]} | {cells[1]} |")
    L.append("")
    L.append(f"Profiles: named and boss creatures roll the gear tiers at {PROFILE_SCALE} times these (ours: jib "
             "gave no numbers for them); the trash profile (no creature yet) rolls trash and the global roll alone.")
    L.append("")
    cats = info["cats"]
    placed = collections.Counter()
    whys = collections.defaultdict(collections.Counter)
    for r in rows:
        if len(r) == 6 and r[0] in GEAR_TIERS + (LEGENDARY,):
            placed[r[0]] += 0
    seen = set()
    for r in rows:
        if len(r) == 6 and r[0] in GEAR_TIERS + (LEGENDARY,) and (r[0], r[2]) not in seen:
            seen.add((r[0], r[2]))
            placed[r[0]] += 1
            y = cats[r[2]][1]
            whys[r[0]][re.sub(r"^stats [\d.]+ times", "stats N times", y)] += 1
    L.append("## Which item is which")
    L.append("")
    L.append("Gear by its stats against the items of its level (tools/loot_tiers.py): at least 4 times the median "
             "ultra rare, 3 super rare, 2 rare, else uncommon; Allakhazam's players' \"rare drop\" super rare and "
             "\"very rare\" ultra rare; the legendaries by name. A sale price is no rule: the pooled tables' items "
             "carry one only where a merchant sells them, most of those ours.")
    L.append("")
    L.append("| category | items | why |")
    L.append("|---|---:|---|")
    for c in GEAR_TIERS + (LEGENDARY,):
        for y, n in whys[c].most_common():
            L.append(f"| {c} | {n} | {y} |")
    L.append("")
    L.append(f"**The global list** (jib's 1 in 10,000 on any kill, one server-wide list): the "
             f"{sum(1 for r in rows if r[0] == GLOBAL)} ultra rares and legendaries. This server's kills in "
             f"{lt.YEARS} years ({lt.DAILY_PLAYERS} daily players x {lt.HOURS_A_DAY} hours x {lt.KILLS_AN_HOUR} kills "
             f"an hour, tools/loot_tiers.py) are {kills:,}, so the list is drawn about {info['global_draws']:.0f} "
             f"times; each legendary's weight ({info['global_leg_weight']} against an ultra rare's "
             f"{GLOBAL_ULTRA_WEIGHT}) gives it {lt.LEGENDARY_COUNT} of them, about five on the server in ten years.")
    L.append("")
    legs = sorted({r[2] for r in rows if r[0] == LEGENDARY})
    L.append(f"**The user preset's legendaries** ({', '.join(names[p] for p in legs)}) are one server-wide list too, "
             f"each the same weight, rolled at {info['leg_rate']:.3g} a kill (1 in {1 / info['leg_rate']:,.0f}) on "
             "any kill: five of each in ten years. A list by region and level cannot give each five: Mithril "
             "Flamberge alone in many creatures' pools would come eleven times to Aura of Karana's one at any "
             "single rate.")
    L.append("")
    tro = info["trophies"]
    trash = info["trash"]
    L.append("## Trash")
    L.append("")
    L.append(f"Each creature's own two trophies (stackable, tradeable, in no quest, recipe, catch or shop) whose "
             f"creature its name holds: {len(trash)} of {len(w.spawns)} creature names, "
             f"{sum(len(w.spawns[k]) for k in trash):,} of {sum(len(v) for v in w.spawns.values()):,} spawns, from "
             f"{len({p for ps in trash.values() for p in ps})} trophies. **The database holds no trash for the other "
             "creatures**: the live game's (pelts, bones, ears ...) were never recorded; they need items of their own.")
    L.append(f"Prices: {len(prices)} trophies had none, given their creatures' median level times "
             f"{info['per_level']:.2f} tunar (Sandstorm's priced trophies, {info['n_ratios']} of them, against their "
             "creatures' levels here): ")
    L.append("")
    for p, (old, new) in sorted(prices.items(), key=lambda kv: kv[1][1]):
        who = sorted(k for k, ps in trash.items() if p in ps)
        L.append(f"- {names[p]}: {new} tunar ({len(who)} creatures: {', '.join(who[:4])}{' ...' if len(who) > 4 else ''})")
    L.append("")
    L.append("## An hour of farming, by level band")
    L.append("")
    L.append("A creature of the band (the median of the band's creatures, by spawns), its xp profile, at 40 and 60 "
             "kills an hour. Trash: for a creature that has its own (the share of the band's spawns that do in "
             "brackets). Now: its corpse's coins and its loot tables' items sold at their value, for the creatures "
             "with tables now (most have none: coins alone).")
    L.append("")
    for kph in PACES:
        L.append(f"### {kph} kills an hour")
        L.append("")
        L.append("| band | now: tunar an hour (coins + drops sold) | trash an hour, jib / user | trash tunar an hour, jib / user | "
                 "gems + modifiers, jib / user | uncommon, jib / user | rare, jib / user | super rare in 100 h, jib / user | "
                 "ultra rare in 100 h (user) | legendary in 100 h (user) | global in 100 h (jib) |")
        L.append("|---|---|---|---|---|---|---|---|---|---|---|")
        for lo, hi in BANDS:
            ks = [(k, lv) for k, sp in w.spawns.items() if w.region.get(k) != "-" for _, lv in sp if lo <= lv <= hi]
            if not ks:
                continue
            coins = statistics.median(1.5 * lv for _, lv in ks) * kph
            drops = [sold_now(k, lv) * kph for k, lv in ks if binds.get(k)]
            have = [(k, lv) for k, lv in ks if k in trash]
            price = statistics.median(statistics.mean(cost.get(p, 0) for p in trash[k]) for k, _ in have) if have else 0
            tr = [per_kill(p, TRASH) * kph for p in ("jib", "user")]
            gm = [(per_kill(p, GEMS) + per_kill(p, MODS)) * kph for p in ("jib", "user")]
            un = [per_kill(p, UNCOMMON) * kph for p in ("jib", "user")]
            ra = [per_kill(p, RARE) * kph for p in ("jib", "user")]
            su = [per_kill(p, SUPER) * kph * 100 for p in ("jib", "user")]
            ul = per_kill("user", ULTRA) * kph * 100
            with_leg = [(k, lv) for k, lv in ks if pool_at(LEGENDARY, w.region.get(k), lv)]
            lg = per_kill("user", LEGENDARY) * kph * 100
            gl = per_kill("jib", GLOBAL) * kph * 100
            share = len(have) / len(ks)
            L.append(f"| {lo}-{hi} | {coins:,.0f} + {statistics.median(drops) if drops else 0:,.0f} | "
                     f"{tr[0]:.0f} / {tr[1]:.0f} ({share:.0%}) | {tr[0] * price:,.0f} / {tr[1] * price:,.0f} | "
                     f"{gm[0]:.1f} / {gm[1]:.1f} | {un[0]:.1f} / {un[1]:.1f} | {ra[0]:.2f} / {ra[1]:.2f} | "
                     f"{su[0]:.1f} / {su[1]:.1f} | {ul:.2f} | "
                     f"{(f'{lg:.3f} ({len(with_leg) / len(ks):.0%} of spawns)') if with_leg else 'none'} | {gl:.2f} |")
        L.append("")
    L.append("## Examples by region")
    L.append("")
    shown = 0
    for lo, hi in BANDS:
        regions = set()
        for k in sorted(w.spawns, key=lambda k: -len(w.spawns[k])):
            reg = w.region.get(k)
            lvs = [lv for _, lv in w.spawns[k] if lo <= lv <= hi]
            if not lvs or reg in regions or reg == "-" or len(regions) >= 2:
                continue
            regions.add(reg)
            lv = int(statistics.median(lvs))
            parts = []
            for c in GEAR_TIERS:
                p = pool_at(c, reg, lv)
                if p:
                    eg = ", ".join(names[x] for x in sorted(set(p))[:2])
                    parts.append(f"{c} {len(set(p))} ({eg} ...)")
            tr = ", ".join(f"{names[p]} {cost.get(p, 0)}t" for p in trash.get(k, [])) or "none"
            L.append(f"- **{lo}-{hi}, {reg or 'no region'}: {k}** (level {lv}, {len(w.spawns[k])} spawns): trash {tr}; " +
                     "; ".join(parts))
            shown += 1
    L.append("")
    with open(path, "w") as f:
        f.write("\n".join(L) + "\n")


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-lootcategories"
    shutil.copy2(path, dst)
    return dst


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout (Map/zones.json): Tunaria's biomes")
    ap.add_argument("--ground", help="the server's ground files (worlds/ground: <world>.env), for Tunaria's waters")
    ap.add_argument("--report", help="write the review (Markdown) here")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write it all (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "loot_category_seed"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "loot_category_seed"):
        sys.exit("loot_category_seed is there: applied already (--undo first)")
    for t in TABLES:
        if has_table(db, t) and db.execute(f"SELECT count(*) FROM {t} WHERE source = ?", (SOURCE,)).fetchone()[0]:
            sys.exit(f"{t} holds rows of source {SOURCE} with no log: take them out first")
    presets, rates, rows, creatures, unbind, prices, info = plan(db, a.eqoa_data, a.ground)
    by = collections.Counter(r[0] for r in rows)
    print("pools: " + ", ".join(f"{c} {by[c]}" for c in ORDER if by[c]) + f" rows; trash for {len(info['trash'])} of "
          f"{len(creatures)} creatures from {len({p for ps in info['trash'].values() for p in ps})} trophies, "
          f"{len(prices)} priced at {info['per_level']:.2f} tunar a level ({info['n_ratios']} of Sandstorm's priced trophies)")
    print(f"rates: {len(rates)} rows in {len(presets)} presets; creatures: {len(creatures)} "
          f"({collections.Counter(c[2] or 'xp' for c in creatures if c[1] != '-')}, {sum(1 for c in creatures if c[1] == '-')} none)")
    print(f"loot table bindings taken away: {len(unbind)} ({collections.Counter(r[1] for r in unbind).most_common(4)} ...)")
    names = {pid: n for pid, n in db.execute("SELECT patternid, itemname FROM itempattern")}
    print(f"global list: {sum(1 for r in rows if r[0] == GLOBAL)} items, {info['global_draws']:.0f} draws in "
          f"{lt.YEARS} years; each legendary's weight {info['global_leg_weight']} (an ultra rare's {GLOBAL_ULTRA_WEIGHT})")
    legs = sorted({r[2] for r in rows if r[0] == LEGENDARY})
    print(f"user preset's legendary rate {info['leg_rate']:.3g} a kill (1 in {1 / info['leg_rate']:,.0f}), "
          f"{lt.LEGENDARY_COUNT} each in {lt.YEARS} years: " + ", ".join(names[p] for p in legs))
    if a.report:
        report(db, a.report, rates, rows, prices, info)
        print(f"report: {a.report}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, presets, rates, rows, creatures, unbind, prices)
    print(f"wrote {len(presets)} presets, {len(rates)} rates, {len(rows)} pool rows, {len(creatures)} creatures; "
          f"took away {len(unbind)} bindings; priced {len(prices)} trophies")


if __name__ == "__main__":
    main()
