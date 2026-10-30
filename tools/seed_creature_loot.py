#!/usr/bin/env python3
"""A loot table for every creature that has none, by its level and region,
each item at its tier's rate a kill (a proposal of 2026-10-30: only 169 of
the 4,330 creature names that can be attacked had a table, and the user's
GM jib gave the live game's rates: "uncommon drops 1/100, rares 1/1000").

Each creature name gets, for each band of BAND levels it spawns in, a table of
its own, "<name> <lo>-<hi>", holding COMPOSITION (2 common items, 3 uncommon,
1 rare: tools/loot_tiers.py) and every legendary its region holds at its
levels, drawn from what already drops or sells where it lives:

  - its region's loot tables and the generic table (EQOA-Data's tables, as
    tools/seed_camp_loot.py wrote them and tools/seed_region_loot.py bound
    them: a Tunaria quadrant's biome, Tunaria's waters marine, the Rathe
    rathe mountains, Odus its four; Lavastorm, the Plane of Sky and the
    Secrets the generic table alone), each item where its window holds the
    creature's level;
  - with --pages, Allakhazam's item lists by zone (tools/fetch_item_pages.py
    --zones: the items its players found there, 2003-2012): those of the zone
    of EQOA-Data's map nearest the creature, within ZONE_REACH, within
    ITEM_LEVELS of its level (an item found in a zone has no level window);
  - the wares of the merchants within TOWN_REACH, at its level or up to
    ITEM_LEVELS below it (common: they sell there).

A pool's items go round its region's creatures of the band, each slot to the
item of the tier fitting the creature that the fewest have so far (an item
sharing a word with the creature's name first, then one of its zone), so
every item drops from some creature where the pool allows. A legendary goes
to every creature of its region whose level its window holds, at its own rate
(loot_tiers.legendary_rate: five in ten years on this server). The two orbs
of level 60 no table holds go to every creature of levels 55-65, as the
generic table's level 60 items do (ours: no record names their source).

Rates are exact: a table's chance is the sum of its items' rates (one roll)
and each item's weight its rate in units of 1e-7; every row's window is the
band, so its items are in play together at every level of it. A table that
would hold no common or uncommon item is not made (its chance would round up
to 1%).

The legendary items' rows in the other tables (the plains table's Aura of
Karana, the generic table's Grimblade and Mithril Flamberge) are held at
weight 0, so they drop only at their own rate: tools/seed_region_loot.py
binds the plains table to creatures of Aura of Karana's levels, at 0.6% a kill.

--retier-bound: the creatures already bound to the pooled tables (the camp
and region seeds' rows of the biome and generic tables) too: those bindings
taken away and tables of their own given from the same pools, and every item
of those tables that no table then holds added to the creature of its band
that it fits holding the fewest (each still at its tier's rate), so none stops
dropping. The alternative to tools/double_loot_chances.py: do not run both.
Run this after tools/seed_region_loot.py (it binds only names without a
table); tools/double_loot_chances.py leaves its tables alone.

Every row written, binding made, row held and binding taken away is logged in
creature_loot_seed; --undo takes back those still as written. --report writes
what a player farming each band sees an hour; --tsv every table's items.
Report only by default. The server reads npcloot and loottables at every
corpse: no restart.

    python3 -E -s tools/seed_creature_loot.py --db eqoa.db --eqoa-data ../external/EQOA-Data \\
        --ground worlds/ground [--pages ~/.cache/eqoa-item-pages] [--retier-bound] \\
        [--report loot.md] [--tsv loot.tsv] [--apply | --undo]
"""
import argparse
import collections
import datetime
import glob
import html
import json
import math
import os
import re
import shutil
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import gen_camp_teles as camps  # noqa: E402
import loot_tiers as lt  # noqa: E402
import seed_camp_loot as campseed  # noqa: E402
import seed_region_loot as region  # noqa: E402

SOURCE = "creature-seed"
BAND = 5
COMPOSITION = ((lt.RARE, 1), (lt.UNCOMMON, 3), (lt.COMMON, 2))
WEIGHT_UNIT = 1e-7          # an item's weight is its rate a kill in these
ZONE_REACH = 600.0          # a creature takes the items of an Allakhazam zone whose centre is this near
TOWN_REACH = 400.0          # and the wares of the merchants this near
ITEM_LEVELS = 5             # an item with no window: its level within this of the creature's
KILLS_AN_HOUR = 40          # tools/drop_rates.py: a kill each minute and a half, hunting
SCROLL_ART = -607649328     # database.ScrollArt: a spell scroll's icon
ORBS_WINDOW = (55, 65)
WORLD_REGION = {1: region.RATHE, 2: "odus", 3: "lavastorm", 4: "plane of sky", 5: "secrets"}
REGION_TABLES = {region.RATHE: (region.RATHE,), "odus": region.ODUS, region.MARINE: (region.MARINE,)}
REGION_TABLES.update({b: (b,) for b in region.BIOMES})
GENERIC = "generic"
POOLED = set(region.BIOMES) | {region.RATHE, region.MARINE, GENERIC} | set(region.ODUS)
POOLED_SOURCES = ("camp-seed", region.SOURCE)
WORDS = re.compile(r"[a-z]{4,}")
COMMON_WORDS = {"with", "from", "that", "this", "ring", "earring", "bracelet", "necklace", "cloak", "robe", "boots",
                "gloves", "helm", "leggings", "tunic", "bracers", "belt", "sword", "shield", "staff", "dagger",
                "axe", "mace", "spear", "bow", "of", "the", "small", "large", "young", "elder", "greater", "lesser"}

LOG_DDL = """CREATE TABLE IF NOT EXISTS creature_loot_seed (
    seq       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind      TEXT NOT NULL,    -- row (a loottables row written) | binding (an npcloot row written)
                                -- | held (a loottables row's weight set 0) | unbound (an npcloot row taken away)
    npc_name  TEXT NOT NULL,
    tablename TEXT NOT NULL,
    lootrow   INTEGER NOT NULL, -- row and held: loottables.id
    old       TEXT NOT NULL,    -- held: the weight; unbound: [chance, rolls, note, source]
    new       TEXT NOT NULL,    -- row: [patternid, weight, lo, hi]; binding: [chance, rolls, note]; held: 0
    applied   TEXT NOT NULL
)"""


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def band_of(level):
    lo = (max(1, level) - 1) // BAND * BAND + 1
    return lo, lo + BAND - 1


def words(s):
    return {w for w in WORDS.findall((s or "").lower()) if w not in COMMON_WORDS}


class World:
    """The creatures, their regions and places, the pooled tables and the
    item records."""

    def __init__(self, db, data=None, ground=None, pages=None):
        self.db = db
        self.tiers = lt.tiers(db)
        self.items = {pid: (n, lv or 0, icon) for pid, n, lv, icon in
                      db.execute("SELECT patternid, itemname, levelreq, itemicon FROM itempattern")}
        self.byname = {}
        for pid, (n, lv, icon) in sorted(self.items.items()):
            k = " ".join((n or "").split()).lower()
            if k and icon != SCROLL_ART:
                self.byname.setdefault(k, pid)
        self.tables = collections.defaultdict(list)  # table -> [(loottables id, patternid, lo, hi)]
        for rid, t, pid, lo, hi in db.execute(
                "SELECT id, tablename, patternid, minlevel, maxlevel FROM loottables WHERE weight > 0"):
            self.tables[t].append((rid, pid, lo, hi))
        self.binds = collections.defaultdict(list)  # name -> [(table, chance, rolls, note, source)]
        for n, t, c, r, note, src in db.execute("SELECT npc_name, tablename, chance, rolls, note, source FROM npcloot"):
            self.binds[n].append((t, c, r, note, src))
        zones, rows = camps.load(db, data) if data else ({"zones": [], "towns": [], "pois": []}, [])
        self.fitted = camps.fits(zones, rows) if data else {}
        self.zones = zones
        self._envs = {}
        self.ground = ground
        self.spawns = collections.defaultdict(list)  # name -> [(world, x, z, level)]
        votes = collections.defaultdict(collections.Counter)
        self.pets = set()
        if has_table(db, "npcaggro"):
            # players' pets recorded beside their owners off the live game (tools/seed_npc_aggro.py)
            self.pets = {campseed.npc_key(n) for (n,) in db.execute(
                "SELECT npc_name FROM npcaggro WHERE note LIKE 'a summoned pet''s name%'")}
        self.all_spawns = 0
        for name, world, x, y, z, lvl, typ in db.execute(
                "SELECT npc_name, world, x_coord, y_coord, z_coord, npc_level, npc_type FROM npcs WHERE npc_name IS NOT NULL"):
            k = campseed.npc_key(name)
            if not k or (typ or 0) & 0x80:
                continue
            self.all_spawns += 1
            if "dummy" in k or k in self.pets:
                continue
            self.spawns[k].append((world, x or 0.0, z or 0.0, lvl or 0))
            votes[k][self.region_at(world, x or 0.0, y or 0.0, z or 0.0)] += 1
        self.region = {k: v.most_common(1)[0][0] for k, v in votes.items()}
        self.zone_items = self.allakhazam(pages) if pages else []
        self.merchants = collections.defaultdict(list)  # world -> [(x, z, [patternid])]
        if has_table(db, "npcitem"):
            wares = collections.defaultdict(list)
            for npcid, pid in db.execute("SELECT npcid, patternid FROM npcitem"):
                wares[npcid].append(pid)
            for npc_id, world, x, z, typ in db.execute("SELECT npc_id, world, x_coord, z_coord, npc_type FROM npcs"):
                if (typ or 0) & 1 and wares.get(npc_id):
                    self.merchants[world].append((x or 0.0, z or 0.0, wares[npc_id]))

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

    def allakhazam(self, pages):
        """[(world, x, z, [patternid])]: the item lists by zone, at the map's
        zone of the same name, in the world whose fit puts a creature
        nearest it."""
        out = []
        names = {}
        for kind in ("zones", "towns", "pois"):
            for e in self.zones.get(kind, []):
                names.setdefault(re.sub(r"[^a-z]", "", e["name"].lower()), (e["x"], e["y"]))
        near_world = collections.defaultdict(list)
        for k, sp in self.spawns.items():
            for w, x, z, _ in sp:
                near_world[w].append((x, z))
        for path in sorted(glob.glob(os.path.join(pages, "lists", "zone-*.html"))):
            raw = re.sub(r"<script.*?</script>", " ", open(path, errors="replace").read(), flags=re.S)
            m = re.search(r"<title>\s*(.*?)\s*::", raw, flags=re.S)
            if not m:
                continue
            key = re.sub(r"[^a-z]", "", html.unescape(m.group(1)).lower())
            spot = names.get(key) or next((v for n, v in names.items() if len(n) > 4 and (n in key or key in n)), None)
            pids = sorted({self.byname[k] for k in
                           (" ".join(html.unescape(n).split()).lower()
                            for _, n in re.findall(r'item\.html\?eqoaitem=(\d+)[^>]*>([^<]+)<', raw))
                           if k in self.byname})
            if not spot or not pids:
                continue
            best = None
            for w, (a, b, c, d) in ((w, f[0]) for w, f in self.fitted.items()):
                gx, gz = a * spot[0] + b, c * spot[1] + d
                dist = min((math.hypot(x - gx, z - gz) for x, z in near_world.get(w, ())), default=math.inf)
                if best is None or dist < best[0]:
                    best = (dist, w, gx, gz)
            if best and best[0] <= ZONE_REACH:
                out.append((best[1], best[2], best[3], pids))
        return out


def candidates(w, name, lo, hi):
    """The items a creature of name may hold in band (lo, hi): {patternid:
    (tier, priority)}, priority 0 for its region's and the generic table's,
    1 for its zone's (Allakhazam), 2 for the merchants' wares near it."""
    levels = {lv for _, _, _, lv in w.spawns[name] if lo <= max(1, lv) <= hi}
    reg = w.region.get(name)
    out = {}

    def take(pid, prio, tier=None):
        t = tier or w.tiers.get(pid, (lt.COMMON, ""))[0]
        if pid not in out or out[pid][1] > prio:
            out[pid] = (t, prio)
    for t in REGION_TABLES.get(reg, ()) + (GENERIC,):
        for _, pid, a, b in w.tables.get(t, ()):
            if any(a <= lv <= b for lv in levels):
                take(pid, 0)
    for pid, tier in w.orbs:
        if any(ORBS_WINDOW[0] <= lv <= ORBS_WINDOW[1] for lv in levels):
            take(pid, 0)
    places = [(wd, x, z) for wd, x, z, lv in w.spawns[name] if lo <= max(1, lv) <= hi]
    for wd, zx, zz, pids in w.zone_items:
        if any(wd == pw and math.hypot(x - zx, z - zz) <= ZONE_REACH for pw, x, z in places):
            for pid in pids:
                ilv = w.items[pid][1]
                if any(abs(ilv - lv) <= ITEM_LEVELS for lv in levels):
                    take(pid, 1)
    for pw, x, z in places:
        for mx, mz, wares in w.merchants.get(pw, ()):
            if math.hypot(x - mx, z - mz) <= TOWN_REACH:
                for pid in wares:
                    ilv = w.items.get(pid, ("", 0, 0))[1]
                    if w.items.get(pid, ("", 0, SCROLL_ART))[2] != SCROLL_ART and lt.group(
                            w.equip.get(pid)) and any(lv - ITEM_LEVELS <= ilv <= lv for lv in levels):
                        take(pid, 2, lt.COMMON)
    return out


def plan(w, retier=False, fill=None):
    """The tables to make: [(name, (lo, hi), [(patternid, tier)])], the
    pooled bindings to take away [(name, table, chance, rolls, note, source)],
    and the names left [(name, why)]. fill (retier's default): every item of
    the pools placed somewhere."""
    fill = retier if fill is None else fill
    w.equip = {pid: slot for pid, slot in w.db.execute("SELECT patternid, equipslot FROM itempattern")}
    in_tables = {p for rows in w.tables.values() for _, p, _, _ in rows}
    second = {pid for pid, (n, _, _) in w.items.items() if " ".join((n or "").split()).lower() in lt.SECOND_RECORDS}
    w.orbs = [(pid, t) for pid, (t, why) in w.tiers.items()
              if t == lt.LEGENDARY and pid not in in_tables and pid not in second and w.items[pid][1] >= 50]
    names, unbind = [], []
    for k in sorted(w.spawns):
        bl = w.binds.get(k, [])
        if not bl:
            names.append(k)
        elif retier:
            pooled = [b for b in bl if b[0] in POOLED and b[4] in POOLED_SOURCES]
            if pooled:
                names.append(k)
                unbind += [(k, *b) for b in pooled]
    cells = collections.defaultdict(list)  # (region, band) -> names
    for k in names:
        for b in sorted({band_of(lv) for _, _, _, lv in w.spawns[k]}):
            cells[(w.region.get(k), b)].append(k)
    out, left = [], []
    placed, cands = set(), {}
    for (reg, (lo, hi)), members in sorted(cells.items(), key=lambda kv: (str(kv[0][0]), kv[0][1])):
        used = collections.Counter()
        for k in members:
            cand = cands[(k, lo, hi)] = candidates(w, k, lo, hi)
            kw = words(k)
            got = []
            for tier, n in COMPOSITION:
                pool = [(used[pid], -len(kw & words(w.items[pid][0])), prio, pid) for pid, (t, prio) in cand.items()
                        if t == tier]
                for *_, pid in sorted(pool)[:n]:
                    got.append((pid, tier))
                    used[pid] += 1
                    placed.add(pid)
            got += [(pid, t) for pid, (t, _) in sorted(cand.items()) if t == lt.LEGENDARY]
            if not any(t in (lt.COMMON, lt.UNCOMMON) for _, t in got):
                left.append((k, f"{lo}-{hi}: no common or uncommon item in its pools ({reg or 'no region'})"))
                continue
            out.append((k, (lo, hi), got))
    if fill:
        # an item of the pooled tables no table holds goes to the creature it fits holding the fewest items
        # (the pooled bindings are gone: it would drop nowhere)
        by = {(k, b): got for k, b, got in out}
        for (k, lo, hi), cand in sorted(cands.items()):
            if (k, (lo, hi)) not in by:
                continue
            for pid, (t, prio) in sorted(cand.items()):
                if prio or t == lt.LEGENDARY or pid in placed:
                    continue
                fits = [(len(by[(k2, (lo2, hi2))]), k2, (lo2, hi2)) for (k2, lo2, hi2), c2 in cands.items()
                        if (k2, (lo2, hi2)) in by and pid in c2 and c2[pid][1] == 0 and lo2 == lo]
                _, k2, b2 = min(fits)
                by[(k2, b2)].append((pid, t))
                placed.add(pid)
    return out, unbind, left


def rates(w, tables):
    """Each legendary's rate: {patternid: (rate, carrier spawns)}."""
    carriers = collections.Counter()
    for k, (lo, hi), got in tables:
        n = sum(1 for _, _, _, lv in w.spawns[k] if lo <= max(1, lv) <= hi)
        for pid, t in got:
            if t == lt.LEGENDARY:
                carriers[pid] += n
    return {pid: (lt.legendary_rate(n, w.all_spawns), n) for pid, n in carriers.items()}


def rows_of(tables, leg):
    """[(name, table, chance, [(patternid, weight, lo, hi, rate)])]."""
    out = []
    for k, (lo, hi), got in tables:
        items = []
        for pid, t in got:
            r = leg[pid][0] if t == lt.LEGENDARY else lt.TIER_RATE[t]
            if r <= 0:
                continue
            items.append((pid, max(1, round(r / WEIGHT_UNIT)), lo, hi, r))
        total = sum(i[4] for i in items)
        chance = max(1, min(100, round(100 * total)))
        out.append((k, f"{k} {lo}-{hi}", chance, items))
    return out


def held(w):
    """The legendary items' rows in the other tables: [(loottables id, table, patternid)]."""
    leg = {pid for pid, (t, _) in w.tiers.items() if t == lt.LEGENDARY}
    return [(rid, t, pid) for t, rows in w.tables.items() for rid, pid, _, _ in rows
            if pid in leg and not t.startswith("boss ")]


def apply(db, made, unbind, hold):
    when = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    log = ("INSERT INTO creature_loot_seed (kind, npc_name, tablename, lootrow, old, new, applied) "
           "VALUES (?, ?, ?, ?, ?, ?, ?)")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_DDL)
        for k, table, chance, items in made:
            for pid, weight, lo, hi, _ in items:
                cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                                 "VALUES (?, ?, ?, ?, ?, ?)", (table, pid, weight, lo, hi, SOURCE))
                db.execute(log, ("row", k, table, cur.lastrowid, "", json.dumps([pid, weight, lo, hi]), when))
            note = "a tiered table: common 1 in 10, uncommon 1 in 100, rare 1 in 1,000 a kill (tools/loot_tiers.py)"
            db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, 1, ?, ?)",
                       (k, table, chance, note, SOURCE))
            db.execute(log, ("binding", k, table, 0, "", json.dumps([chance, 1, note]), when))
        for k, t, c, r, note, src in unbind:
            db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (k, t))
            db.execute(log, ("unbound", k, t, 0, json.dumps([c, r, note, src]), "", when))
        for rid, t, pid in hold:
            (old,) = db.execute("SELECT weight FROM loottables WHERE id = ?", (rid,)).fetchone()
            db.execute("UPDATE loottables SET weight = 0 WHERE id = ?", (rid,))
            db.execute(log, ("held", "", t, rid, json.dumps(old), "0", when))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, "creature_loot_seed"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        back, kept = 0, []
        for kind, k, t, rid, old, new in db.execute(
                "SELECT kind, npc_name, tablename, lootrow, old, new FROM creature_loot_seed ORDER BY seq DESC").fetchall():
            if kind == "row":
                pid, weight, lo, hi = json.loads(new)
                cur = db.execute("SELECT tablename, patternid, weight, minlevel, maxlevel, source FROM loottables WHERE id = ?",
                                 (rid,)).fetchone()
                if cur == (t, pid, weight, lo, hi, SOURCE):
                    db.execute("DELETE FROM loottables WHERE id = ?", (rid,))
                    back += 1
                elif cur is not None:
                    kept.append(f"loottables row {rid}")
            elif kind == "binding":
                chance, rolls, note = json.loads(new)
                cur = db.execute("SELECT chance, rolls, note, source FROM npcloot WHERE npc_name = ? AND tablename = ?",
                                 (k, t)).fetchone()
                if cur == (chance, rolls, note, SOURCE):
                    db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (k, t))
                    back += 1
                elif cur is not None:
                    kept.append(f"{k!r} on {t!r}")
            elif kind == "unbound":
                c, r, note, src = json.loads(old)
                if db.execute("SELECT 1 FROM npcloot WHERE npc_name = ? AND tablename = ?", (k, t)).fetchone() is None:
                    db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                               (k, t, c, r, note, src))
                    back += 1
                else:
                    kept.append(f"{k!r} on {t!r} (bound again since)")
            elif kind == "held":
                cur = db.execute("SELECT weight FROM loottables WHERE id = ?", (rid,)).fetchone()
                if cur is not None and cur[0] == 0:
                    db.execute("UPDATE loottables SET weight = ? WHERE id = ?", (json.loads(old), rid))
                    back += 1
                elif cur is not None:
                    kept.append(f"loottables row {rid}")
        db.execute("DROP TABLE creature_loot_seed")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {back} changes" + (f"; {len(kept)} changed since left as they are: " + ", ".join(kept[:20]) if kept else ""))


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-creatureloot"
    shutil.copy2(path, dst)
    return dst


def hour(items, chance):
    """Expected items an hour of a table, by tier, at KILLS_AN_HOUR."""
    total = sum(i[1] for i in items)
    out = collections.Counter()
    for pid, weight, lo, hi, r, tier in items:
        out[tier] += KILLS_AN_HOUR * chance / 100 * weight / total
    return out


def report(w, made, leg, left, unbind, hold, retier, path):
    names = {pid: n for pid, (n, _, _) in w.items.items()}
    every = {t: c for t, c, *_ in w.binds.get("*", [])}
    tiers = w.tiers
    L = []
    L.append("# A loot table for every creature: the proposal\n")
    L.append("Built by `tools/seed_creature_loot.py`" + (" --retier-bound" if retier else "") +
             " on a copy of the live data of 2026-10-30 01:30 UTC with tools/seed_region_loot.py, "
             "tools/raise_gem_roll.py and tools/fix_single_spawn_steps.py applied. Nothing is applied anywhere live.\n")
    n_names = len({k for k, *_ in made})
    L.append(f"- creature names given tables: **{n_names}**, tables **{len(made)}**, rows "
             f"**{sum(len(i) for *_, i in made)}**" + (f"; bindings to the pooled tables taken away: {len(unbind)}" if unbind else ""))
    L.append(f"- names left without one: {len({k for k, _ in left})} (no common or uncommon item at their level and place)")
    L.append(f"- legendary rows held at weight 0 in the other tables: {len(hold)} "
             f"({', '.join(sorted({f'{names[p]} ({t})' for _, t, p in hold}))})")
    L.append("")
    L.append("## Tiers and rates\n")
    L.append("Per item, per kill of a creature holding it (tools/loot_tiers.py): **common 1 in 10** (ours: the next "
             "step of jib's ladder), **uncommon 1 in 100**, **rare 1 in 1,000** (jib's), **legendary** its own rate, "
             "so about five exist on this server after ten years.\n")
    L.append(f"Each creature holds, for each band of {BAND} levels it spawns in: " +
             ", ".join(f"{n} {t}" for t, n in reversed(COMPOSITION)) + ", and every legendary of its region and level. "
             "Every creature also rolls the tables every creature does: " +
             ", ".join(f"{t} {c}%" for t, c in sorted(every.items())) + ".\n")
    why = collections.defaultdict(collections.Counter)
    eg = collections.defaultdict(list)
    pool_items = {pid for *_, items in made for pid, *_ in items}
    for pid in sorted(pool_items, key=lambda p: (-w.items[p][1], names[p])):
        t, y = tiers[pid]
        y = re.sub(r"^stats \(?\d+(\.\d+)? times its level's median\)?$",
                   f"stats {'at least' if t == lt.RARE else 'under'} {lt.RARE_RATIO:g} times its level's median", y)
        why[t][y] += 1
        if len(eg[(t, y)]) < 3:
            eg[(t, y)].append(f"{' '.join(names[pid].split())} (L{w.items[pid][1]})")
    L.append("Items placed, by tier and why (every item's tier and why: `tools/loot_tiers.py --tsv`):\n")
    L.append("| tier | items | why | for example |")
    L.append("|---|---:|---|---|")
    for t in lt.TIERS:
        for y, n in why[t].most_common():
            L.append(f"| {t} | {n} | {y} | {', '.join(eg[(t, y)])} |")
    L.append("")
    L.append("## The legendary items\n")
    k10 = lt.kills_in()
    L.append(f"This server's kills in {lt.YEARS} years: {lt.DAILY_PLAYERS} daily players (accounts logged in during the "
             f"data's last day) x {lt.HOURS_A_DAY} hours x {lt.KILLS_AN_HOUR} kills an hour x 365 x {lt.YEARS} = "
             f"**{k10:,}**. An item's creatures take the share of them their spawns are of every spawn that can be "
             f"attacked ({w.all_spawns:,}); its rate is {lt.LEGENDARY_COUNT} over that.\n")
    L.append("| item | why legendary | creatures' spawns | share of kills | rate a kill | about 1 in | in 10 years |")
    L.append("|---|---|---:|---:|---:|---:|---:|")
    for pid, (r, n) in sorted(leg.items(), key=lambda kv: names[kv[0]]):
        share = n / w.all_spawns
        L.append(f"| {names[pid]} | {tiers[pid][1]} | {n} | {share:.2%} | {r:.2e} | {1 / r:,.0f} | "
                 f"{r * k10 * share:.1f} |")
    for pid, (t, y) in sorted(tiers.items()):
        if t == lt.LEGENDARY and pid not in leg:
            not_placed = ("a second record of the same item: not placed" if " ".join(names[pid].split()).lower()
                          in lt.SECOND_RECORDS else "not placed: no creature of its region at its level")
            L.append(f"| {names[pid]} | {y} | 0 | | {not_placed} | | |")
    L.append("")
    L.append(f"## What a player farming a band sees an hour ({KILLS_AN_HOUR} kills)\n")
    L.append("Items an hour from a creature's own tables, by tier (the band's median creature), then the tables every "
             f"creature rolls. One given uncommon item comes about every {1 / (KILLS_AN_HOUR * lt.TIER_RATE[lt.UNCOMMON]):.1f} "
             f"hours of farming a creature that holds it, one given rare every {1 / (KILLS_AN_HOUR * lt.TIER_RATE[lt.RARE]):.0f}.\n")
    byband = collections.defaultdict(list)
    for k, table, chance, items in made:
        lo, hi = items[0][2], items[0][3]
        h = hour([(pid, wt, a, b, r, tiers[pid][0] if pid not in leg else lt.LEGENDARY) for pid, wt, a, b, r in items], chance)
        byband[(lo, hi)].append((k, table, chance, items, h))
    L.append("| band | tables | commons an hour | uncommons an hour | a rare, hours for one | a legendary, hours for one "
             "(the tables holding one) | gems and modifiers an hour |")
    L.append("|---|---:|---:|---:|---:|---:|---:|")
    star = KILLS_AN_HOUR * sum(every.values()) / 100

    def med(xs):
        xs = sorted(xs)
        return xs[len(xs) // 2] if xs else 0

    def hours(x):
        return f"{1 / x:,.0f}" if x > 0 else "none"
    for b in sorted(byband):
        hs = [h for *_, h in byband[b]]
        legs = [h[lt.LEGENDARY] for h in hs if h[lt.LEGENDARY] > 0]
        L.append(f"| {b[0]}-{b[1]} | {len(hs)} | {med([h[lt.COMMON] for h in hs]):.1f} | {med([h[lt.UNCOMMON] for h in hs]):.2f} | "
                 f"{hours(med([h[lt.RARE] for h in hs]))} | {hours(med(legs)) + f' ({len(legs)} tables)' if legs else 'none'} | "
                 f"{star:.1f} |")
    L.append("")
    L.append("## Examples by band and region\n")
    for b in sorted(byband):
        seen = set()
        shown = 0
        for k, table, chance, items, h in sorted(byband[b], key=lambda x: (-len(w.spawns[x[0]]), x[0])):
            reg = w.region.get(k) or "no region"
            if reg in seen or shown >= 3:
                continue
            seen.add(reg)
            shown += 1
            parts = []
            for pid, wt, a, bb, r in sorted(items, key=lambda i: -i[4]):
                p = chance / 100 * wt / sum(i[1] for i in items)
                t = lt.LEGENDARY if pid in leg else tiers[pid][0]
                parts.append(f"{names[pid]} ({t}, 1 in {1 / p:,.0f})")
            L.append(f"- **{b[0]}-{b[1]}, {reg}: {k}** ({len(w.spawns[k])} spawns, table {chance}%): " + "; ".join(parts))
    L.append("")
    if left:
        L.append("## Left without a table\n")
        for k, y in left[:60]:
            L.append(f"- {k}: {y}")
        if len(left) > 60:
            L.append(f"- and {len(left) - 60} more")
    with open(path, "w") as f:
        f.write("\n".join(L) + "\n")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--eqoa-data", help="EQOA-Data's checkout (Map/zones.json): Tunaria's biomes and Allakhazam's zones")
    ap.add_argument("--ground", help="the server's ground files (worlds/ground: <world>.env), for Tunaria's waters")
    ap.add_argument("--pages", help="tools/fetch_item_pages.py's OUT (lists/zone-*.html): Allakhazam's item lists by zone")
    ap.add_argument("--retier-bound", action="store_true",
                    help="the creatures bound to the pooled tables too (the alternative to double_loot_chances.py)")
    ap.add_argument("--report", help="write the proposal's report (Markdown) here")
    ap.add_argument("--tsv", help="write every table's items here")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the tables (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "creature_loot_seed"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    if a.apply and has_table(db, "creature_loot_seed"):
        sys.exit("creature_loot_seed is there: applied already (--undo first)")
    w = World(db, a.eqoa_data, a.ground, a.pages)
    tables, unbind, left = plan(w, a.retier_bound)
    leg = rates(w, tables)
    made = rows_of(tables, leg)
    hold = held(w)
    print(f"tables to make: {len(made)} for {len({k for k, *_ in made})} names, {sum(len(i) for *_, i in made)} rows; "
          f"names left: {len({k for k, _ in left})}; pooled bindings taken away: {len(unbind)}; legendary rows held: {len(hold)}")
    c = collections.Counter(t for *_, got in tables for _, t in got)
    print("items placed by tier: " + ", ".join(f"{t} {c[t]}" for t in lt.TIERS))
    names = {pid: n for pid, (n, _, _) in w.items.items()}
    for pid, (r, n) in sorted(leg.items(), key=lambda kv: names[kv[0]]):
        print(f"  legendary {names[pid]}: {n} spawns hold it, {r:.2e} a kill (1 in {1 / r:,.0f})")
    if a.report:
        report(w, made, leg, left, unbind, hold, a.retier_bound, a.report)
        print(f"report: {a.report}")
    if a.tsv:
        with open(a.tsv, "w") as f:
            f.write("creature\ttable\tchance\titem\ttier\trate\n")
            for k, table, chance, items in made:
                total = sum(i[1] for i in items)
                for pid, wt, lo, hi, r in items:
                    t = lt.LEGENDARY if pid in leg else w.tiers[pid][0]
                    f.write(f"{k}\t{table}\t{chance}\t{' '.join(names[pid].split())}\t{t}\t{chance / 100 * wt / total:.6g}\n")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not made and not hold:
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    apply(db, made, unbind, hold)
    print(f"wrote {len(made)} tables, {len(unbind)} pooled bindings taken away, {len(hold)} legendary rows held")


if __name__ == "__main__":
    main()
