#!/usr/bin/env python3
"""Creatures Sandstorm shows where this server has none: a census flown over
a world by the headless harvester, each creature it saw that has no namesake
of ours near its place made a row of npcs (the user, 2026-10-31: "we can map
out our missing from sandstorm", after the population tool's guesses for
Rathe's west, cmd/wildspread's made kinds, met their zones' real creatures:
"envar, dead hills have different mobs, undead", "geomancers citadel has the
archon bosses ...").

The census: eqoa-rust-headless's `eqoa harvest` on the user's named
character, `track on` through a route of `goto` steps, a run per zone
(cmd/sandroute -zones -below 12: rows 220 apart, a stop every 50, 12 under
the lowest floor near each leg, where nothing reaches it and it is level
with what walks there: Sandstorm shows the creatures within about 170 of
the client, height counted, and saw twice as many from under Ogre Ruins as
from 60 over them; never under the top of any water, where Sandstorm
drowns a player even under the sea floor), its `<out>.track.jsonl` files: a
`who` line the first time a creature is seen (its body, size, level, class,
race, npc flags, the items it holds, its armour, hair, face and robe) and
its place every half second.

What it decides, and from what:

- A creature is one the census saw with an id under PLAYER_IDS (Sandstorm's
  NPCs' ids; its players' are in the hundreds of millions). Its place is the
  median of the places it was seen at (a wanderer is seen where it walked),
  its facing its last heading.
- It is ours already when a row of npcs of its name (any case, the '$' mark
  dropped) stands within MATCH of its place in its world, each of our rows
  taken once, the nearest first. The rest are missing.
- Only creatures a player fights are made (no NPC bit, 0x80, in its flags),
  and only a kind's: a name two or more of ours or of the census carry, or one
  fix_npc_name_case.py's rules read generic. A named creature is listed, not
  made (a bare row would have none of its words or quests), as is one whose
  name we have elsewhere in the world; a summoned pet (a spell's name: the
  hobgoblin mystics' Feral Spirits) is left out. The townsfolk and merchants
  missing are listed (--with-npcs makes them too, as bare rows: no wares, no
  words).
- A missing creature's row copies one of ours (every column: the server's
  own for the kind, its unknown ones too): of its name, else of its body,
  else the creatures of Return Home's import's usual values; then takes the
  census's own: its name, place, facing, world, body, size, level, flags,
  race, held items, armour, hair, face and robe. npc_id is 0 (no loot rows of
  its own).
- Its name is ours when we have it (any world, our most common spelling),
  else as tools/fix_npc_name_case.py would case it (Sandstorm capitalizes
  every word: "Grass Viper" is "grass viper"; a proper word kept).
- A name this server has no row of takes rows in the tables a name keys:
  npcclass (Sandstorm's class), lootcreature (the world's loot region,
  REGION) and npctemplate (tools/seed_roamers.py's rule: a generic hostile
  warrior the roamer, a wisp the wisp).

--swap makes the world's creatures Sandstorm's (the user, 2026-10-31: "lets
add/swap for Sandstorms Rathe/Odus", swapping creatures): every creature
row of ours in the world is removed but those a quest names or spawns (a
quest's target or giver, an unlock's NPC, a name of npcspawnonly) and those
another table points at by row (wares, a path, a sign); the census is
matched against what is kept, so a kept kind's census creatures near ours
are not made twice. Our townsfolk are never removed. --with-named and
--with-pets make the named creatures and the summoned pets too (a named
one is still not made where a kept row of ours has its name in the world).
Only a row within SIGHT (120, --sight) of the census flight's own path
across and SIGHT_UP (60, --sight-up) over or under it is removed:
Sandstorm shows a client the creatures within about 160 of it sideways but
hardly any more than 90 under it (flights 80 and more over the ground saw
the ground's creatures almost never), so a place the census never came near
(a zone not flown yet, a deep dungeon, a basement under a flight) keeps
what it has.

Every row added is logged in sandstorm_census_seed_w<world> (the table,
the rowid, the census id), and every row removed with the whole row;
--undo deletes the added and puts the removed back, ids and all. A second
--apply in a world is refused while its census is in. Report only by default (--plan writes the
rows it would add as JSON). The server reads npcs at its start: apply with
it stopped (a swap's before.sh), or restart after.

    tools/seed_sandstorm_census.py --track 'zone-*.json.track.jsonl' --world 1
        [--db eqoa.db] [--match 150] [--swap] [--with-npcs] [--with-named] [--with-pets]
        [--plan out.json] [--apply | --undo]
"""
import argparse
import collections
import datetime
import glob
import json
import math
import os
import sqlite3
import statistics
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
PLAYER_IDS = 100_000_000
MATCH = 150.0
SIGHT = 120.0
SIGHT_UP = 60.0
# the harvester's track clock (its "t"): wall-clock milliseconds below 2^30,
# the DRDP clock (eqoa-rust-headless): track_clock puts a UTC time on it
TRACK_CLOCK = 1 << 30
# a creature is seen walking at a pace over WALKING a second; a chase is
# Sandstorm's run state (3) or a pace over CHASE (it walks at 4.92)
WALKING = 0.2
CHASE = 6.0
# a creature watched this long on end without a step stood (Sandstorm's rests
# between legs are 4.1 to 4.6 s)
STILL_WATCH = 20.0
# two census creatures of a name, of two flights, this near each other and
# each where the other flight passed within DEDUPE_COVER across (SIGHT_UP over
# or under), are one spawn seen twice (dedupe)
DEDUPE_LINK = 50.0
DEDUPE_COVER = 100.0
# a row of ours this near a census creature, its name spelled alike, is it
SPELLING_REACH = 5.0
# --move: npcmove rows, logged in MOVE_LOG; a kind no census creature was
# watched long enough of moves as its family (the name less its last word)
# when FAMILY_MIN of those were and FAMILY_SHARE of them one way
MOVE_LOG = "sandstorm_census_move"
FAMILY_MIN = 5
FAMILY_SHARE = 0.75
# kinds whose move is said here, not measured: the Isle of Dread's sea
# creatures, glimpsed 3.6 s each (the user, 2026-10-31, of the census's
# creatures that stood: "same with IoD"); they swim where they spawned
MOVE_SAID = {
    "devourer": (1, "the Isle of Dread's sea, glimpsed only: wanders (the user: \"same with IoD\")"),
    "ray of agony": (1, "the Isle of Dread's sea, glimpsed only: wanders (the user: \"same with IoD\")"),
}
LOG = "sandstorm_census_seed"
META = "sandstorm_census_meta"


def tables(world):
    """A world's log and meta tables: each world's census is applied and
    undone on its own."""
    return f"{LOG}_w{world}", f"{META}_w{world}"
SOURCE = "sandstorm-census"
REGION = {0: "tunaria", 1: "rathe mountains", 2: "odus"}

# Return Home's imported creatures' usual values for the columns the census
# does not see (a basilisk's row, 2026-10-31)
DEFAULTS = dict(hp=1, hp_percent=255, unk_a=1, unk_b=82, unk_d=0, unk_f=0, unk_g=0, unk_h=0, unk_j=0, unk_k=0,
                unk_i="15", pd_icon=0, unk_m=0, unk_n=1, unk_o=0, zone=0, dt_kill=None)


def key(name):
    return (name or "").strip().lower().rstrip("$").strip()


def flight_of(path):
    """A track file's flight: its name without the harvester's endings."""
    b = os.path.basename(path)
    for end in (".json.track.jsonl", ".track.jsonl", ".jsonl"):
        if b.endswith(end):
            return b[: -len(end)]
    return b


def track_clock(when):
    """A UTC time ("2026-10-31T13:46:52Z") on the tracks' clock."""
    d = datetime.datetime.fromisoformat(when.strip().replace("Z", "+00:00"))
    if d.tzinfo is None:
        d = d.replace(tzinfo=datetime.timezone.utc)
    return int(d.timestamp() * 1000) % TRACK_CLOCK


def read_census(paths, exclude=(), restarts=()):
    """(epoch, id) -> {looks, name, flight, places [(t, x, y, z, h)], mv} for
    every creature seen, but those whose name (any case) is in `exclude`: what
    Sandstorm put in for an event, not its world (--exclude).

    Sandstorm gives a creature a new id each time it spawns, from a counter a
    restart of its server starts again (at 20,000,000; 2026-10-31 it restarted
    at 13:46:52 and 14:53:45 UTC), so an id names one creature only between
    two restarts: `restarts` (on the tracks' clock, track_clock) split the
    census into epochs, a track file's by when it begins (a restart ends every
    session). flight: the track file a creature was first seen in. mv: how it
    moved while watched, out of the velocity Sandstorm sends with a creature's
    place: walked (a step at a walk, out of a chase), chase (it ran at
    someone), watch (its longest watch on end, s)."""
    exclude = {key(n) for n in exclude}
    restarts = sorted(restarts)
    seen = {}
    motion = collections.defaultdict(list)  # (epoch, id) -> [(t, pace, state)]
    for path in paths:
        fl, epoch = flight_of(path), None
        with open(path) as f:
            for line in f:
                try:
                    r = json.loads(line)
                except ValueError:
                    continue  # a line the harvester was still writing
                if epoch is None and "t" in r:
                    epoch = sum(1 for x in restarts if r["t"] >= x)
                ep = r.get("epoch", epoch or 0)
                if "who" in r:
                    if r["who"] < PLAYER_IDS:
                        c = seen.setdefault((ep, r["who"]), {"places": [], "flight": r.get("flight", fl)})
                        c.setdefault("looks", {k: v for k, v in r.items() if k not in ("epoch", "flight")})
                elif "id" in r and r["id"] < PLAYER_IDS:
                    k = (ep, r["id"])
                    c = seen.setdefault(k, {"places": [], "flight": r.get("flight", fl)})
                    c["name"] = r["name"]
                    c["places"].append((r["t"], r["x"], r["y"], r["z"], r.get("h", 0.0)))
                    if "mv" in r:
                        c["mv"] = r["mv"]
                    elif "v" in r:
                        v = r["v"] or [0, 0, 0]
                        motion[k].append((r["t"], math.hypot(v[0], v[2]), r.get("st", 0)))
    for k, ms in motion.items():
        ms.sort()
        watch, start = 0.0, ms[0][0]
        for a, b in zip(ms, ms[1:]):
            if b[0] - a[0] > 2000:
                start = b[0]
            watch = max(watch, (b[0] - start) / 1000)
        seen[k]["mv"] = dict(walked=any(WALKING < p <= CHASE and st != 3 for _, p, st in ms),
                             chase=any(p > CHASE or st == 3 for _, p, st in ms), watch=round(watch, 1))
    # a nameless entity (a spawner, a marker) is no creature to make
    return {k: c for k, c in seen.items()
            if c.get("places") and c.get("looks") and key(c.get("name")) and key(c.get("name")) not in exclude}


def read_flights(paths, step=10.0):
    """flight -> its places (its `self` lines), one every `step`."""
    out = collections.defaultdict(list)
    for path in paths:
        fl = flight_of(path)
        with open(path) as f:
            for line in f:
                if '"self"' not in line:
                    continue
                try:
                    r = json.loads(line)
                except ValueError:
                    continue
                p = r.get("self")
                if not p or len(p) != 3:
                    continue
                pts = out[r.get("flight", fl)]
                if not pts or math.dist(pts[-1], p) >= step:
                    pts.append(tuple(p))
    return dict(out)


def dedupe(census, flights, link=DEDUPE_LINK, cover=DEDUPE_COVER, up=SIGHT_UP):
    """(drop, by_flight): the census creatures that are another's spawn seen
    again, and how many each flight loses. Sandstorm gives a creature a new id
    each time it spawns, and spawns them as a player comes (its id counter
    runs on through a census: a flight's ids are its own hour's), so a place
    two flights saw, a zone flown again or the edge two zones share, has its
    creatures twice under two ids (the census of 2026-10-31: Rathe 922 of
    5,760, Oggok flown three times; Odus 704 of 6,616). Two creatures of a
    name from two flights, within `link` of each other, each where the other's
    flight passed within `cover` across and `up` over or under (it would have
    been seen), are joined; of a joined group the flight that saw the most is
    kept (a tie: the one that passed nearest), the others' dropped. A flight
    never meets itself: two of a name it saw are two creatures."""
    grids = {}
    for fl, pts in flights.items():
        g = collections.defaultdict(list)
        for p in pts:
            g[(int(p[0] // 100), int(p[2] // 100))].append(p)
        grids[fl] = g
    places = {k: place(c) for k, c in census.items()}
    reach = int(cover // 100) + 1
    near_cache = {}

    def near(fl, k):
        """How near flight fl passed creature k (across, within up), inf for never."""
        if (fl, k) not in near_cache:
            x, y, z, _ = places[k]
            best, g = math.inf, grids.get(fl, {})
            cx, cz = int(x // 100), int(z // 100)
            for i in range(cx - reach, cx + reach + 1):
                for j in range(cz - reach, cz + reach + 1):
                    for p in g.get((i, j), ()):
                        if abs(p[1] - y) <= up:
                            best = min(best, math.hypot(p[0] - x, p[2] - z))
            near_cache[(fl, k)] = best
        return near_cache[(fl, k)]

    cells = collections.defaultdict(list)
    for k, c in census.items():
        x, _, z, _ = places[k]
        cells[(key(c["name"]), int(x // link), int(z // link))].append(k)
    parent = {k: k for k in census}

    def find(k):
        while parent[k] != k:
            parent[k] = parent[parent[k]]
            k = parent[k]
        return k

    for k, c in census.items():
        x, _, z, _ = places[k]
        nk = key(c["name"])
        for di in (-1, 0, 1):
            for dj in (-1, 0, 1):
                for o in cells.get((nk, int(x // link) + di, int(z // link) + dj), ()):
                    oc = census[o]
                    if o <= k or oc["flight"] == c["flight"]:
                        continue
                    ox, _, oz, _ = places[o]
                    if math.hypot(ox - x, oz - z) <= link and near(oc["flight"], k) <= cover and near(c["flight"], o) <= cover:
                        a, b = find(k), find(o)
                        if a != b:
                            parent[a] = b
    groups = collections.defaultdict(list)
    for k in census:
        groups[find(k)].append(k)
    drop, by_flight = set(), collections.Counter()
    for g in groups.values():
        of = collections.defaultdict(list)
        for k in g:
            of[census[k]["flight"]].append(k)
        if len(of) < 2:
            continue
        keep = min(of, key=lambda fl: (-len(of[fl]), statistics.mean(min(near(fl, k), 1e9) for k in g), fl))
        for fl, ks in of.items():
            if fl != keep:
                drop.update(ks)
                by_flight[fl] += len(ks)
    return drop, by_flight


def read_path(paths, step=20.0):
    """The census flight's own places (its `self` lines), one every `step`."""
    out = []
    for path in paths:
        with open(path) as f:
            for line in f:
                if '"self"' not in line:
                    continue
                try:
                    p = json.loads(line).get("self")
                except ValueError:
                    continue
                if p and len(p) == 3 and (not out or math.dist(out[-1], p) >= step):
                    out.append(tuple(p))
    return out


class Sight:
    """Whether a place lies within `reach` across and `up` over or under some
    place of the census flight's path."""

    def __init__(self, path, reach=SIGHT, up=SIGHT_UP):
        self.reach, self.up, self.cells = reach, up, collections.defaultdict(list)
        for p in path:
            self.cells[(int(p[0] // 100), int(p[2] // 100))].append(p)

    def saw(self, x, y, z):
        cx, cz, n = int(x // 100), int(z // 100), int(self.reach // 100) + 1
        for i in range(cx - n, cx + n + 1):
            for j in range(cz - n, cz + n + 1):
                for p in self.cells.get((i, j), ()):
                    if abs(p[1] - y) <= self.up and math.hypot(p[0] - x, p[2] - z) <= self.reach:
                        return True
        return False


def compact(census, path, flights=None):
    """The census as two lines a creature, its first `who` line and one place
    line at its place (place: the median, the height near it, the last
    heading; its epoch, its flight and how it moved, mv), which read_census
    reads back to the same census for plan, dedupe and --move, and each
    flight's path (read_flights') as `self` lines with their flight: the
    track files are tens of megabytes, this a few."""
    with open(path, "w") as f:
        for fl in sorted(flights or {}):
            for p in flights[fl]:
                f.write(json.dumps(dict(self=list(p), flight=fl)) + "\n")
        for k in sorted(census):
            ep, i = k
            c = census[k]
            x, y, z, h = place(c)
            last = c["places"][-1]
            f.write(json.dumps(dict(c["looks"], epoch=ep, flight=c["flight"])) + "\n")
            line = dict(t=last[0], id=i, name=c["name"], lvl=c["looks"].get("lvl"), model=c["looks"].get("model"),
                        x=x, y=y, z=z, h=h, epoch=ep, flight=c["flight"])
            if "mv" in c:
                line["mv"] = c["mv"]
            f.write(json.dumps(line) + "\n")


def place(c):
    """A creature's place: the median of where it was seen; its last heading."""
    ps = c["places"]
    x = statistics.median(p[1] for p in ps)
    z = statistics.median(p[3] for p in ps)
    # the height of the sighting nearest that place
    near = min(ps, key=lambda p: math.hypot(p[1] - x, p[3] - z))
    return x, near[2], z, ps[-1][4]


def facing_byte(h):
    return int(round((h % (2 * math.pi)) / (2 * math.pi) * 256)) % 256


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def spellings(db):
    """key -> our most common spelling of the name, any world."""
    c = collections.defaultdict(collections.Counter)
    for (n,) in db.execute("SELECT npc_name FROM npcs WHERE npc_name IS NOT NULL"):
        c[key(n)][n.strip()] += 1
    return {k: v.most_common(1)[0][0] for k, v in c.items()}


def cased(db, names, rows=None):
    """The spelling tools/fix_npc_name_case.py's rules give each new name
    (Sandstorm's, as the census has it: it capitalizes every word, and spells
    some names two ways, "Ogre Thug" and "ogre thug"), one for every spelling
    of a name, and whether those rules read it a kind's (generic). rows: fold
    key -> [creatures of the name, of them a town's peaceful person, of them
    on a players' race body], as that tool's town_rows counts a name's rows,
    so a name the census saw three times or more is a kind whatever its words
    (its KIND_ROWS: "Toxic Mist" is "toxic mist") and a town's people keep
    their capitals. The rules are what our style is (2026-10-30: a generic
    name lowercase but for its proper words, "Ankextet soldier"; an
    individual's, a pet's, a town's people's and a place's as written)."""
    try:
        import fix_npc_name_case as nc
        ev = nc.load_evidence(db)
    except Exception as e:  # no evidence: every name someone's, made only when two or more carry it
        print(f"casing: {e}: Sandstorm's spellings kept", file=sys.stderr)
        return {n: (n, False) for n in names}
    spellings = collections.defaultdict(collections.Counter)
    for n in names:
        spellings[nc.fold(n)][n] += 1
    for k, ns in spellings.items():
        ev.variants[k].update(ns)
    out = {}
    for k, ns in spellings.items():
        d = nc.decide(ev, k, (rows or {}).get(k))
        target = d.target if d is not None and d.target else ns.most_common(1)[0][0]
        if d is not None and d.kind == "individual" and len(k.split()) == 1 and (rows or {}).get(k, [0])[0] >= nc.KIND_ROWS:
            # one word the census saw three times or more: a kind's ("Devourer", "Necrapede"), not someone's
            target, d = k, nc.Decision(k, "generic", k, f"one word, seen {(rows or {})[k][0]} times: a kind")
        for n in ns:
            out[n] = (target, d is not None and d.kind == "generic")
    return out


def spelled_alike(a, b, most=2):
    """Two names two letters or fewer apart (an edit distance): a spelling of
    one name, as "orge thug" is of "ogre thug"."""
    if a == b or abs(len(a) - len(b)) > most or min(len(a), len(b)) < 6:
        return a == b
    prev = list(range(len(b) + 1))
    for i, ca in enumerate(a, 1):
        cur = [i]
        for j, cb in enumerate(b, 1):
            cur.append(min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (ca != cb)))
        prev = cur
    return prev[-1] <= most


def player_bodies(db):
    """The players' race bodies (charactermodel): a town's people wear them."""
    if not has_table(db, "charactermodel"):
        return set()
    return {m & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel") if m is not None}


def pet_names(db):
    """A summoned pet's names: a spell's (entity/pet.go makes a pet after the
    NPCs of its spell's name) and an enchanter's animations."""
    out = set()
    if has_table(db, "spellpattern"):
        out = {key(n) for (n,) in db.execute("SELECT spellname FROM spellpattern WHERE spellname IS NOT NULL")}
    return out


# what keeps a creature row of ours through --swap: another table pointing at
# its row, or a quest naming it
KEEP_IDS = (("npcitem", "npcid"), ("npcpaths", "npcid"), ("queststeps", "npcid"), ("signtext", "npcid"))
KEEP_NAMES = (("queststeps", "target"), ("queststeptargets", "target"), ("quests", "giver"),
              ("questunlocks", "npc"), ("npcspawnonly", "name"))


def kept(db):
    """(ids, names): the rows --swap keeps by row and by name."""
    ids, names = set(), set()
    for t, c in KEEP_IDS:
        if has_table(db, t):
            ids |= {r[0] for r in db.execute(f"SELECT {c} FROM {t} WHERE {c} IS NOT NULL")}
    for t, c in KEEP_NAMES:
        if has_table(db, t):
            names |= {key(r[0]) for r in db.execute(f"SELECT {c} FROM {t} WHERE {c} IS NOT NULL")}
    return ids, names


def plan(db, census, world, match=MATCH, with_npcs=False, swap=False, with_named=False, with_pets=False, sight=None):
    """(adds, removes, report): the rows to add, the rows of ours to remove
    (--swap: only those `sight`, a Sight, saw; all with none) and what was
    found."""
    keep_ids, keep_names = kept(db) if swap else (set(), set())
    unseen = []
    db.row_factory = sqlite3.Row
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    ours = collections.defaultdict(list)
    by_name = collections.defaultdict(list)
    by_model = collections.defaultdict(list)
    removes = []
    for r in db.execute("SELECT * FROM npcs"):
        d = dict(r)
        k = key(d["npc_name"])
        if d["world"] == world and swap and not (d["npc_type"] or 0) & 0x80 and d["id"] not in keep_ids and k not in keep_names:
            if sight is None or sight.saw(d["x_coord"], d["y_coord"], d["z_coord"]):
                removes.append(d)
            else:
                unseen.append(d)
                ours[k].append(d)
        elif d["world"] == world:
            ours[k].append(d)
        if k:
            by_name[k].append(d)
        if d["modelid"]:
            by_model[d["modelid"] & 0xFFFFFFFF].append(d)
    db.row_factory = None
    ours_spelling = spellings(db)
    # each census creature to its nearest namesake of ours within match, one each
    pairs = []
    for i, c in census.items():
        x, _, z, _ = place(c)
        for d in ours.get(key(c["name"]), ()):
            dist = math.hypot(d["x_coord"] - x, d["z_coord"] - z)
            if dist <= match:
                pairs.append((dist, i, d["id"]))
    pairs.sort()
    matched, used = {}, set()
    for dist, i, rid in pairs:
        if i in matched or rid in used:
            continue
        matched[i] = (rid, dist)
        used.add(rid)
    # a row of ours on the very spot under a spelling of the name (the
    # quests' "orge thug" is Sandstorm's "ogre thug", "Seamsteress Kella" its
    # "Seamstress Kella"): one creature, ours
    near = collections.defaultdict(list)
    for k in ours:
        for d in ours[k]:
            near[(int(d["x_coord"] // 10), int(d["z_coord"] // 10))].append(d)
    for i, c in census.items():
        if i in matched:
            continue
        x, _, z, _ = place(c)
        best = None
        for di in (-1, 0, 1):
            for dj in (-1, 0, 1):
                for d in near.get((int(x // 10) + di, int(z // 10) + dj), ()):
                    dist = math.hypot(d["x_coord"] - x, d["z_coord"] - z)
                    if d["id"] not in used and dist <= SPELLING_REACH and spelled_alike(key(c["name"]), key(d["npc_name"])) \
                            and (best is None or dist < best[0]):
                        best = (dist, d["id"])
        if best:
            matched[i] = (best[1], best[0])
            used.add(best[1])
    pets = pet_names(db)
    is_pet = lambda i: key(census[i]["name"]) in pets or key(census[i]["name"]).endswith("animation")
    missing = [i for i in census if i not in matched and (with_pets or not is_pet(i))]
    pets_seen = collections.Counter(census[i]["name"] for i in census if is_pet(i) and i not in matched)
    creatures = [i for i in missing if not census[i]["looks"].get("npc_flags", 0) & 0x80]
    people = [i for i in missing if census[i]["looks"].get("npc_flags", 0) & 0x80]
    new_names = sorted({census[i]["name"].strip() for i in missing if key(census[i]["name"]) not in ours_spelling})
    bodies = player_bodies(db)
    name_rows = collections.defaultdict(lambda: [0, 0, 0])  # as fix_npc_name_case.town_rows counts a name's rows
    for c in census.values():
        r, body = name_rows[" ".join(c["name"].split()).lower()], (c["looks"].get("model") or 0) & 0xFFFFFFFF in bodies
        r[0] += 1
        r[1] += bool(c["looks"].get("npc_flags", 0) & 0x80 and body)
        r[2] += body
    casing = cased(db, new_names, name_rows)
    seen_count = collections.Counter(key(census[i]["name"]) for i in census)
    in_world = collections.Counter(key(d["npc_name"]) for k in ours for d in ours[k])

    def generic(i):
        # a kind's: two or more of ours or of the census, or the casing rules' generic
        k = key(census[i]["name"])
        if in_world[k] >= 2 or len(by_name.get(k, ())) >= 2 or seen_count[k] >= 2:
            return True
        return k not in ours_spelling and casing.get(census[i]["name"].strip(), ("", False))[1]

    named = [i for i in creatures if not generic(i)]
    elsewhere = [i for i in named if in_world[key(census[i]["name"])] > 0]
    make = [i for i in creatures if generic(i)] + (people if with_npcs else []) + \
        ([i for i in named if i not in elsewhere] if with_named else [])
    adds = []
    for i in sorted(make):
        c, lk = census[i], census[i]["looks"]
        x, y, z, h = place(c)
        k = key(c["name"])
        tmpl = None
        if by_name.get(k):
            tmpl = min(by_name[k], key=lambda d: (d["world"] != world, d["id"]))
        elif by_model.get(lk["model"]):
            tmpl = min(by_model[lk["model"]], key=lambda d: (d["world"] != world, d["id"]))
        row = {c2: None for c2 in cols}
        row.update(DEFAULTS)
        if tmpl:
            row.update(tmpl)
        row.pop("id", None)
        name = ours_spelling.get(k) or casing.get(c["name"].strip(), (c["name"].strip(), True))[0]
        items = (lk.get("items") or [0, 0, 0]) + [0, 0, 0]
        armor = (lk.get("armor") or [0] * 6) + [0] * 6
        hair = (lk.get("hair") or [0, 0, 0]) + [0, 0, 0]
        row.update(npc_name=name, x_coord=round(x, 3), y_coord=round(y, 4), z_coord=round(z, 3),
                   facing=facing_byte(h), world=world, modelid=lk["model"], size=lk.get("scale", 1.0),
                   npc_level=lk.get("lvl", 1), npc_type=lk.get("npc_flags", 0), race=str(lk.get("race", 0)),
                   primary_weapon=items[0], secondary_weapon=items[1], shield=items[2],
                   torso=armor[0], forearm=armor[1], gloves=armor[2], legs=armor[3], feet=armor[4], head=armor[5],
                   hair_color=hair[0], hair_length=hair[1], hair_style=hair[2], face=lk.get("face", 0),
                   robe_style=str(lk.get("robe", 255)), npc_id=0, dt_kill=None)
        adds.append(dict(row={c2: row.get(c2) for c2 in cols if c2 != "id"}, census_id=i[1], new_name=k not in ours_spelling,
                         sand_class=lk.get("class", 0), template_row=tmpl["id"] if tmpl else None))
    report = dict(seen=len(census), matched=len(matched), missing_creatures=len(creatures), missing_people=len(people),
                  adds=len(adds), removes=len(removes), unseen_kept=len(unseen),
                  removed=collections.Counter(d["npc_name"] for d in removes).most_common(),
                  with_npcs=with_npcs, with_named=with_named, with_pets=with_pets,
                  new_names=sorted({a["row"]["npc_name"] for a in adds if a["new_name"]}),
                  by_name=collections.Counter(a["row"]["npc_name"] for a in adds).most_common(),
                  people=sorted(collections.Counter(census[i]["name"] for i in people).items()),
                  pets=sorted(pets_seen.items()),
                  named=sorted((census[i]["name"], census[i]["looks"].get("lvl"), [round(v) for v in place(census[i])[:3]])
                               for i in named if i not in elsewhere),
                  elsewhere=sorted((census[i]["name"], [round(v) for v in place(census[i])[:3]]) for i in elsewhere))
    return adds, removes, report


def roamer_template(name, level, cls):
    """tools/seed_roamers.py's verdict for a new name: the wisp for a wisp, the
    roamer for a generic hostile warrior, else none."""
    if "wisp" in key(name).split():
        return "wisp"
    if cls == 0:
        return "roamer"
    return None


def apply(db, adds, world, files, removes=()):
    LOG, META = tables(world)
    if has_table(db, META):
        sys.exit(f"a census is applied already in world {world} ({META}): --undo it first")
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rid INTEGER NOT NULL, "
               f"census_id INTEGER, name TEXT NOT NULL, old TEXT)")
    db.execute(f"CREATE TABLE {META} (k TEXT PRIMARY KEY, v TEXT NOT NULL)")
    # --swap's removals first, each kept whole for --undo
    for d in removes:
        db.execute("DELETE FROM npcs WHERE id = ?", (d["id"],))
        db.execute(f"INSERT INTO {LOG} (tbl, rid, census_id, name, old) VALUES ('npcs removed', ?, NULL, ?, ?)",
                   (d["id"], d["npc_name"] or "", json.dumps(d)))
    done_names = set()
    for a in adds:
        r = a["row"]
        cols = list(r)
        cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
        db.execute(f"INSERT INTO {LOG} (tbl, rid, census_id, name) VALUES ('npcs', ?, ?, ?)", (cur.lastrowid, a["census_id"], r["npc_name"]))
        name = r["npc_name"]
        if a["new_name"] and name not in done_names:
            done_names.add(name)
            for tbl, sql, args in (
                ("npcclass", "INSERT INTO npcclass (name, class, source) VALUES (?, ?, ?)", (name, a["sand_class"], SOURCE)),
                ("lootcreature", "INSERT INTO lootcreature (npc_name, region, profile, source) VALUES (?, ?, '', ?)",
                 (name, REGION.get(world, ""), SOURCE)),
            ):
                if has_table(db, tbl) and db.execute(f"SELECT 1 FROM {tbl} WHERE lower(trim({'name' if tbl == 'npcclass' else 'npc_name'})) = ?",
                                                      (key(name),)).fetchone() is None:
                    cur = db.execute(sql, args)
                    db.execute(f"INSERT INTO {LOG} (tbl, rid, census_id, name) VALUES (?, ?, ?, ?)", (tbl, cur.lastrowid, a["census_id"], name))
            t = roamer_template(name, r["npc_level"], a["sand_class"]) if not r["npc_type"] & 0x80 else None
            if t and has_table(db, "npctemplate") and db.execute("SELECT 1 FROM npctemplate WHERE lower(trim(name)) = ?",
                                                                 (key(name),)).fetchone() is None:
                cur = db.execute("INSERT INTO npctemplate (name, template, note, source) VALUES (?, ?, ?, ?)",
                                 (name, t, f"generic, level {r['npc_level']}", SOURCE))
                db.execute(f"INSERT INTO {LOG} (tbl, rid, census_id, name) VALUES ('npctemplate', ?, ?, ?)", (cur.lastrowid, a["census_id"], name))
    for k, v in dict(applied=datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                     world=str(world), files=json.dumps(files), seq_before=str(seq[0] if seq else ""),
                     added=str(len(adds)), removed=str(len(removes))).items():
        db.execute(f"INSERT INTO {META} VALUES (?, ?)", (k, v))


def undo(db, world):
    LOG, META = tables(world)
    if not has_table(db, LOG):
        sys.exit(f"no census is applied in world {world}")
    meta = dict(db.execute(f"SELECT k, v FROM {META}").fetchall()) if has_table(db, META) else {}
    n = collections.Counter()
    for tbl, rid, old in db.execute(f"SELECT tbl, rid, old FROM {LOG} ORDER BY seq DESC").fetchall():
        if tbl == "npcs removed":
            d = json.loads(old)
            cols = list(d)
            n[tbl] += db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                 [d[c] for c in cols]).rowcount
            continue
        n[tbl] += db.execute(f"DELETE FROM {tbl} WHERE rowid = ?", (rid,)).rowcount
    seq = meta.get("seq_before", "")
    if seq:
        top = db.execute("SELECT MAX(id) FROM npcs").fetchone()[0] or 0
        if top <= int(seq):
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (int(seq),))
    db.execute(f"DROP TABLE {LOG}")
    db.execute(f"DROP TABLE IF EXISTS {META}")
    return n


def move_rules(db, census, said=MOVE_SAID):
    """name key -> (roam, note): how the census's kinds move on Sandstorm,
    where the server would move them otherwise by their kind (entity's
    movement: wildlife wanders, anything else stands; tools/seed_npc_move.py
    keeps its lists). Sandstorm walks most of its creatures, wildlife or not
    (82% of Odus's and 86% of Rathe's watched 20 s or more), and stands its
    sentries, its keeps' and crypts' creatures and its casters: a kind walks
    when as many of its creatures were seen walking (a step at a walk, out of
    a chase) as were watched STILL_WATCH on end without one, and stands
    otherwise; a kind none of whose creatures was watched long enough moves as
    its family (its name less its last word, "Ankexfen" of "Ankexfen knight")
    when FAMILY_MIN of the family's were and FAMILY_SHARE of them one way;
    `said` says some outright. Wildlife is made to stand only when two or more
    of its kind stood and none walked. A name npcmove has a row of, a town's
    people (the NPC bit, a players' race body) and a kind nothing is known of
    are left to the server."""
    import seed_npc_move as mv
    undead = {}
    if has_table(db, "npcundead"):
        undead = {key(n): bool(u) for n, u in db.execute("SELECT name, undead FROM npcundead")}
    rows = {}
    if has_table(db, "npcmove"):
        rows = {key(n) for (n,) in db.execute("SELECT name FROM npcmove")}
    bodies = player_bodies(db)
    seen = collections.defaultdict(lambda: [0, 0])  # key -> [walked, stood]
    kinds = set()
    for c in census.values():
        lk, k = c["looks"], key(c["name"])
        if lk.get("npc_flags", 0) & 0x80 or (lk.get("model") or 0) & 0xFFFFFFFF in bodies:
            continue
        kinds.add(k)
        m = c.get("mv")
        if not m:
            continue
        if m["walked"]:
            seen[k][0] += 1
        elif not m["chase"] and m["watch"] >= STILL_WATCH:
            seen[k][1] += 1
    family = collections.defaultdict(lambda: [0, 0])
    for k, (w, st) in seen.items():
        if len(k.split()) > 1:
            f = " ".join(k.split()[:-1])
            family[f][0] += w
            family[f][1] += st
    out = {}
    for k in sorted(kinds):
        if k in rows:
            continue
        w, st = seen.get(k, (0, 0))
        if k in said:
            roam, why = said[k]
        elif w or st:
            if w == st:
                continue  # as many stood as walked: as its kind
            roam = 1 if w > st else 0
            why = f"Sandstorm's census: {w} of {w + st} watched {'walking' if roam else 'walking; the rest stood'}"
        else:
            f = " ".join(k.split()[:-1])
            fw, fs = family.get(f, (0, 0)) if f else (0, 0)
            if fw + fs < FAMILY_MIN or max(fw, fs) < FAMILY_SHARE * (fw + fs):
                continue
            roam = 1 if fw > fs else 0
            why = f"Sandstorm's census: none watched long enough; the {f} kinds {fw} of {fw + fs} walking"
        wild = bool(mv.wildlife_word(k)) and not mv.is_undead(k, undead)
        if (roam == 1) == wild:
            continue  # the server moves it so by its kind already
        if roam == 0 and (st < 2 or w > 0) and k not in said:
            continue  # wildlife stands only on plain evidence
        out[k] = (roam, why)
    return out


def apply_move(db, rules):
    """Writes move_rules' rows into npcmove (source sandstorm-census), logged
    in MOVE_LOG for --move --undo; a second is refused while the log is there."""
    if has_table(db, MOVE_LOG):
        sys.exit(f"the census's movement is applied already ({MOVE_LOG}): --move --undo it first")
    if not has_table(db, "npcmove"):
        sys.exit("no table npcmove (the server makes it at its start)")
    db.execute(f"CREATE TABLE {MOVE_LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, rid INTEGER NOT NULL, name TEXT NOT NULL)")
    for k, (roam, note) in sorted(rules.items()):
        cur = db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, ?, 0, '', ?, ?)",
                         (k, roam, note, SOURCE))
        db.execute(f"INSERT INTO {MOVE_LOG} (rid, name) VALUES (?, ?)", (cur.lastrowid, k))


def undo_move(db):
    """Takes --move's rows out of npcmove again (a row a GM has since made
    another source's is left) and drops the log."""
    if not has_table(db, MOVE_LOG):
        sys.exit(f"the census's movement is not applied ({MOVE_LOG})")
    n = 0
    for rid, name in db.execute(f"SELECT rid, name FROM {MOVE_LOG}").fetchall():
        n += db.execute("DELETE FROM npcmove WHERE rowid = ? AND name = ? AND source = ?", (rid, name, SOURCE)).rowcount
    db.execute(f"DROP TABLE {MOVE_LOG}")
    return n


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--track", action="append", default=[], help="the census's track files (a glob; again for more)")
    ap.add_argument("--world", type=int, default=1)
    ap.add_argument("--match", type=float, default=MATCH)
    ap.add_argument("--with-npcs", action="store_true", help="make the missing townsfolk and merchants too")
    ap.add_argument("--with-named", action="store_true", help="make the named creatures too")
    ap.add_argument("--with-pets", action="store_true", help="make the summoned pets too")
    ap.add_argument("--swap", action="store_true", help="remove our creatures in the world but those a quest or a table keeps")
    ap.add_argument("--sight", type=float, default=SIGHT, help="--swap removes only a row this near the census flight's path, across")
    ap.add_argument("--sight-up", type=float, default=SIGHT_UP, help="... and this near it over or under")
    ap.add_argument("--exclude", action="append", default=[],
                    help="a name never made nor matched (an event's spawn); again for more, or @file with a name a line")
    ap.add_argument("--restart", action="append", default=[],
                    help="when Sandstorm restarted during the census (UTC, 2026-10-31T13:46:52Z; again for more, or a comma "
                         "list): an id names one creature only between two")
    ap.add_argument("--no-dedupe", action="store_true", help="keep a spawn two flights saw twice (dedupe)")
    ap.add_argument("--plan", help="write the rows it would add, as JSON")
    ap.add_argument("--compact", help="write the census as two lines a creature (read back as a track file)")
    ap.add_argument("--move", action="store_true",
                    help="npcmove rows for the census's kinds as Sandstorm moves them (move_rules; every world's census at once)")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=5)
    if a.undo:
        with db:
            if a.move:
                print(f"undone: {undo_move(db)} npcmove rows")
                return
            n = undo(db, a.world)
        print(f"undone: {dict(n)}")
        return
    files = sorted({p for g in a.track for p in glob.glob(g)})
    if not files:
        sys.exit("no track files (--track)")
    exclude = []
    for e in a.exclude:
        if e.startswith("@"):
            exclude += [line.strip() for line in open(e[1:]) if line.strip() and not line.startswith("#")]
        else:
            exclude.append(e)
    restarts = [track_clock(t) for r in a.restart for t in r.split(",") if t.strip()]
    census = read_census(files, exclude, restarts)
    flights = read_flights(files)
    if a.compact:
        compact(census, a.compact, flights)
        print(f"compact: {len(census)} creatures and {sum(map(len, flights.values()))} places of {len(flights)} flights "
              f"to {a.compact}")
    if not a.no_dedupe:
        drop, by_flight = dedupe(census, flights)
        census = {k: c for k, c in census.items() if k not in drop}
        print(f"dedupe: {len(drop)} creatures another flight saw too, dropped"
              + (" (" + ", ".join(f"{fl} {n}" for fl, n in by_flight.most_common()) + ")" if drop else ""))
    if a.move:
        rules = move_rules(db, census)
        walk = sorted(k for k, (r, _) in rules.items() if r)
        stand = sorted(k for k, (r, _) in rules.items() if not r)
        print(f"move: {len(walk)} kinds walk ({sum(1 for c in census.values() if key(c['name']) in walk)} of the census's "
              f"creatures), {len(stand)} stand, where the server's kinds say otherwise")
        for k in walk + stand:
            print(f"  {'walks ' if rules[k][0] else 'stands'}  {k}: {rules[k][1]}")
        if a.apply:
            with db:
                apply_move(db, rules)
            print(f"applied: {len(rules)} npcmove rows ({MOVE_LOG})")
        return
    flight = [p for fl in sorted(flights) for p in flights[fl]]
    if a.swap and not flight:
        sys.exit("--swap needs the census flight's path (the tracks' self lines)")
    sight = Sight(flight, a.sight, a.sight_up) if a.swap else None
    adds, removes, report = plan(db, census, a.world, a.match, a.with_npcs, a.swap, a.with_named, a.with_pets, sight)
    print(f"{len(files)} track files: {report['seen']} creatures seen, {report['matched']} ours already, "
          f"{report['missing_creatures']} creatures and {report['missing_people']} townsfolk missing; {report['adds']} to add"
          + (f", {report['removes']} of ours to remove ({report['unseen_kept']} more out of the flight's sight kept)" if a.swap else ""))
    print("new names:", ", ".join(report["new_names"]) or "none")
    for name, n in report["by_name"]:
        print(f"  {n:4d}  {name}")
    if a.swap:
        print("ours removed:", ", ".join(f"{n} x{c}" for n, c in report["removed"]) or "none")
    if report["people"]:
        print(f"townsfolk missing ({'made' if a.with_npcs else 'not made without --with-npcs'}):",
              ", ".join(f"{n} x{c}" for n, c in report["people"]))
    if report["pets"]:
        print(f"pets seen (a summoning spell's name: {'made' if a.with_pets else 'not made without --with-pets'}):",
              ", ".join(f"{n} x{c}" for n, c in report["pets"]))
    if report["named"]:
        print(f"named creatures we have none of ({'made' if a.with_named else 'not made without --with-named'}):",
              "; ".join(f"{n} L{l} at {p}" for n, l, p in report["named"]))
    if report["elsewhere"]:
        print("named creatures we have elsewhere in the world (not made):", "; ".join(f"{n} at {p}" for n, p in report["elsewhere"]))
    if a.plan:
        with open(a.plan, "w") as f:
            json.dump(dict(report=report, adds=adds, removes=removes), f, indent=1, default=str)
    if a.apply:
        with db:
            apply(db, adds, a.world, files, removes)
        print(f"applied: {len(adds)} rows added, {len(removes)} removed ({tables(a.world)[0]})")


if __name__ == "__main__":
    main()
