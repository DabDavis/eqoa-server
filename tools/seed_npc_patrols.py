#!/usr/bin/env python3
"""The cities' patrols as Sandstorm walks them (the user, 2026-10-29: "we need
patrolling npcs through the cities to work correctly. sandstorm is good to go
to each city and watch the patrols").

This server's NPCs walked no route at all: npcpaths (entity/path.go, the GM
panel's path editor) had no row, so every guard and walker stood on its spawn.
eqoa-rust-headless's harvester watched Sandstorm's: in each city a new
character hunted for anyone who walks (its `hunt` step: the 24 creatures
nearest it, followed when one walks, until its route came round) and wrote
every creature in view twice a second (`track on`: --tracks, the
`<combo>.json.track.jsonl` files beside the survey's JSON).

**What Sandstorm sends of a walker.** A walker's place on its channel changes
only now and then, its velocity saying where it goes between (the game
extrapolates). So a route is its stops (the velocity falls to 0 there; the
pause lasts until it moves off again) and its turns (a leg's direction
changes with no stop).

**How a route is read.**
- The stops and turns, near ones (3 apart) taken for one waypoint, make a
  sequence.
- Its shortest repeat makes the route: a loop, or back and forth
  (`pingpong`) when the repeat reads the same both ways. A Freeport piker
  pacing 12 units east and back is two points back and forth.
- A route never seen to repeat (followed out of view, or not long enough) is
  walked back and forth over what was seen. So it never takes a shortcut
  through a wall nobody saw it take.
- The pace is the median of its legs' speeds (database PathSpeed: a number
  of units a second; Sandstorm's pikers 3.12, under the walk's 5).
- The pause is the median of its stops; a point's own where its stops differ
  by 2 s or more.

**A wanderer is no patrol.** Sandstorm's creatures that roam (Highbourne's
Lost Souls, Moradhim's Frosteye goblins and quarry workers) walk at its
wander's pace, 4.87-4.98 (roam.go's 5 is it), in legs that never come round,
with rests of 3 to 30 seconds between; a patrol walks slower (2.62-3.12, Halas'
guards 3.94, Oggok's ogre bouncers and impalers 4.32-4.43) and stops 0.5 or 3
seconds at its points, now and then 30. A walker at
WANDER_PACE never seen to repeat is a wanderer: its name gets an npcmove row
(roam 1: it wanders, as roam.go wanders wildlife; radius the farthest it was
seen from the middle of its walk, 15 to 75), never a path, and no NPC is
made for one; a name with an npcmove row already is left.

Who is left out: an animal by its name (seed_npc_move.py's wildlife_word, the
server's own list), the watcher itself, and every player: an id of
PLAYER_IDS or more (the channels give players and NPCs one kind; Sandstorm's
players' ids are in the hundreds of millions, its NPCs' under 25 million). Wildlife wanders here by
roam.go, not by a path.

Each route goes to this database's NPC of the same name (any case, no "$")
in the city's world whose spawn is nearest the route, within 150; each NPC
once. An NPC with a path a GM gave it is left and said.

**Sandstorm's walkers are mostly its own NPCs** (Fayspires' Hunter Lyraeth,
level 65, and Thalioren Vaerwyn, 60, walk between the guards at its bank;
nothing of either name was in this database). With --add a walker of no NPC
here is made one: the row of this database's NPC nearest its route, in its
world, for every column Sandstorm's channel does not show (its zone and the
rest), and from the track's `who` line its name, body, size, the items it
holds, its armor, hair, face, robe, race, level and class and npc flags
(npc_type: the class in the high byte, the flags' low byte in the low), its
spawn its route's first point, its health the median of this database's NPCs
of its level. It is of the faction of the nearest of its town's people (an
npcfaction row at its spawn), so it is the town's, not a stranger to it.
Sandstorm's armor tints are not kept: this server sends every NPC's as 0.

Rows are logged in npcpaths_seed (the npc, the row before or none, the row
written); --undo puts back each one still as written and drops the log; a
second --apply is refused while the log is there. Report only by default.
Paths are read at the server's start (LoadNPCPaths), or live from the GM
panel: restart after (go-eqoa-server tools/swap_when_empty.sh does it when no
one is connected).

    tools/seed_npc_patrols.py [--db eqoa.db] --tracks <dir or files> ... [--apply | --undo]
"""
import argparse
import collections
import glob
import json
import math
import os
import sqlite3
import statistics
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from seed_npc_move import wildlife_word  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SAME_POINT = 3.0      # stops and turns this near are one waypoint
MOVING = 0.3          # a speed above this is a leg
TURN = math.radians(15)
MATCH = 150.0         # a route to an NPC of its name whose spawn is this near it
OWN_PAUSE = 2.0       # a point's own pause when its stops differ by this
PLAYER_IDS = 100_000_000  # an id this or more is a player's, never an NPC's
WANDER_PACE = (4.8, 5.2)  # Sandstorm's wander (4.87-4.98 measured; roam.go's 5); patrols 2.62-3.94, an ogre's 4.32-4.43
WANDER_RADIUS = (15.0, 75.0)
MOVE_LOG = """CREATE TABLE npcmove_seed (
    name   TEXT PRIMARY KEY,   -- npcmove.name written
    after  TEXT NOT NULL,      -- the row written, JSON
    source TEXT NOT NULL
)"""
LOG_SCHEMA = """CREATE TABLE npcpaths_seed (
    npcid  INTEGER PRIMARY KEY,
    before TEXT,             -- the npcpaths row before, JSON, or NULL for none
    after  TEXT NOT NULL,    -- the row written, JSON
    source TEXT NOT NULL,    -- the track file and the Sandstorm id
    made   TEXT              -- with --add: {"npc": its npcs row as written, "faction": the npcfaction row id}
)"""
COLS = ("world", "mode", "pause", "speed", "points")


def key(name):
    n = (name or "").strip()
    if n.endswith("$"):
        n = n[:-1].rstrip()
    return " ".join(n.replace("`", "'").lower().split())


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def read_track(path):
    """{id: (name, [samples])}, the watcher's own name and the world, off a
    track file (a line cut off at its end skipped)."""
    by, who = {}, {}
    with open(path) as f:
        for line in f:
            try:
                r = json.loads(line)
            except ValueError:
                continue
            if "who" in r:
                who.setdefault(r["who"], r)
                continue
            if "id" not in r:
                continue
            by.setdefault(r["id"], (r["name"], []))[1].append(r)
    world, watcher = None, None
    js = path[: -len(".track.jsonl")] if path.endswith(".track.jsonl") else None
    if js and os.path.exists(js):
        with open(js) as f:
            d = json.load(f)
        watcher = (d.get("made") or {}).get("name") or (d.get("player") or {}).get("name")
        world = d.get("world")
        if world is None and isinstance(d.get("player"), dict):
            world = d["player"].get("world")
    for i in by:
        by[i][1].sort(key=lambda r: r["t"])
    return by, watcher, world, who


def updates(samples):
    """The samples where Sandstorm sent something new: the place or the
    velocity changed."""
    out, last = [], None
    for r in samples:
        state = (round(r["x"], 2), round(r["z"], 2), round(r["v"][0], 2), round(r["v"][2], 2))
        if state != last:
            out.append(r)
            last = state
    return out


def speed(r):
    return math.hypot(r["v"][0], r["v"][2])


def route(samples):
    """A walker's route off its samples: (points [(x, y, z, pause or None)],
    mode, pace, pause, full), or None when it never walked."""
    ups = updates(samples)
    legs = [u for u in ups if speed(u) > MOVING]
    if not legs:
        return None
    span = math.hypot(max(u["x"] for u in ups) - min(u["x"] for u in ups), max(u["z"] for u in ups) - min(u["z"] for u in ups))
    if span < 4:
        return None
    # the stops (where it stood, how long) and the turns, in order
    core = []  # (x, y, z, pause or None)
    for i, u in enumerate(ups):
        if speed(u) <= MOVING:
            nxt = next((v for v in ups[i + 1:] if speed(v) > MOVING), None)
            pause = (nxt["t"] - u["t"]) / 1000 if nxt else None
            core.append((u["x"], u["y"], u["z"], pause))
        elif i > 0 and speed(ups[i - 1]) > MOVING:
            a = math.atan2(ups[i - 1]["v"][2], ups[i - 1]["v"][0])
            b = math.atan2(u["v"][2], u["v"][0])
            if abs((b - a + math.pi) % (2 * math.pi) - math.pi) > TURN:
                core.append((u["x"], u["y"], u["z"], None))  # a turn: no pause of its own
    wps = []  # [x, y, z, [pauses]]

    def waypoint(x, y, z, p):
        for j, w in enumerate(wps):
            if math.hypot(w[0] - x, w[2] - z) < SAME_POINT:
                if p is not None:
                    w[3].append(p)
                return j
        wps.append([x, y, z, [p] if p is not None else []])
        return len(wps) - 1

    seq = [waypoint(*m) for m in core]
    dedup = [s for i, s in enumerate(seq) if i == 0 or s != seq[i - 1]]
    mode, order, full = "pingpong", [], False
    # the route is its stops' and turns' shortest repeat: seen once whole and
    # then again for three of its waypoints (both of a pacer's two)
    for p in range(2, len(dedup)):
        if len(dedup) - p >= min(3, p) and all(dedup[i] == dedup[i + p] for i in range(len(dedup) - p)):
            cycle, full = dedup[:p], True
            mode, order = "loop", cycle
            # back and forth when the cycle reads the same both ways round
            for r in range(p):
                rot = cycle[r:] + cycle[:r]
                half = rot[: p // 2 + 1]
                if p == 2 or (p % 2 == 0 and rot == half + half[-2:0:-1]):
                    mode, order = "pingpong", half
                    break
            break
    if not full:
        # seen in part: where it was first and last seen are ends too, and it
        # walks back and forth over what was seen
        first = waypoint(ups[0]["x"], ups[0]["y"], ups[0]["z"], None)
        last = waypoint(ups[-1]["x"], ups[-1]["y"], ups[-1]["z"], None)
        order = list(dict.fromkeys([first] + dedup + [last]))
    if len(order) < 2:
        return None  # it shuffled on the spot
    pauses = [q for w in wps for q in w[3] if q is not None]
    pause = round(statistics.median(pauses), 1) if pauses else 0.0
    points = []
    for j in order:
        x, y, z, ps = wps[j]
        own = round(statistics.median(ps), 1) if ps else None
        points.append((round(x, 1), round(y, 2), round(z, 1), own if own is not None and abs(own - pause) >= OWN_PAUSE else None))
    pace = round(statistics.median(speed(u) for u in legs), 2)
    mx = statistics.median(u["x"] for u in ups)
    mz = statistics.median(u["z"] for u in ups)
    spread = max(math.hypot(u["x"] - mx, u["z"] - mz) for u in ups)
    return points, mode, pace, pause, full, spread


def ours(db):
    """This database's NPCs by name key: [(id, x, z, world)]."""
    out = collections.defaultdict(list)
    for i, name, x, z, w in db.execute("SELECT id, npc_name, x_coord, z_coord, world FROM npcs"):
        out[key(name)].append((i, x, z, w))
    return out


def town_factions(db):
    """[(x, z, world, faction)]: where each NPC of a town's faction (a
    Citizens of, or a guard faction) spawned."""
    if not has_table(db, "npcfaction"):
        return []
    of = {}
    for name, f in db.execute("SELECT name, faction FROM npcfaction"):
        if f.startswith("Citizens of ") or f in ("The Freeport Militia", "The Qeynos Guard", "The Highpass Guard", "Klick`Anon Watch"):
            of[key(name)] = f
    return [(x, z, w, of[key(n)]) for n, x, z, w in db.execute("SELECT npc_name, x_coord, z_coord, world FROM npcs") if key(n) in of]


def made_npc(db, cols, who, points, world, hp_by_level):
    """A new npcs row for a walker of no NPC here (the module's comment), the
    row of the NPC nearest its route its template."""
    x, y, z = points[0][0], points[0][1], points[0][2]
    q = "SELECT * FROM npcs WHERE world = ? ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?) LIMIT 1"
    t = db.execute(q, (world, x, x, z, z)).fetchone()
    if t is None:
        return None
    row = dict(zip(cols, t))
    row.pop("id", None)
    items = (who.get("items") or [0, 0, 0]) + [0, 0, 0]
    armor = (who.get("armor") or [0] * 6) + [0] * 6
    hair = (who.get("hair") or [0, 0, 0]) + [0, 0, 0]
    lvl = int(who.get("lvl") or row.get("npc_level") or 1)
    row.update({
        "npc_name": who["name"], "x_coord": x, "y_coord": y, "z_coord": z, "facing": 0.0, "world": world,
        "modelid": int(who["model"]), "size": round(float(who.get("scale") or 1.0), 3),
        "primary_weapon": items[0], "secondary_weapon": items[1], "shield": items[2],
        "torso": armor[0], "forearm": armor[1], "gloves": armor[2], "legs": armor[3], "feet": armor[4], "head": armor[5],
        "hair_color": hair[0], "hair_length": hair[1], "hair_style": hair[2], "face": int(who.get("face") or 0),
        "robe_style": str(who.get("robe", 255)), "race": str(who.get("race", 0)), "npc_level": lvl,
        "npc_type": (int(who.get("class") or 0) << 8) | (int(who.get("npc_flags") or 0) & 0xFF), "npc_id": 0,
        "hp": hp_by_level.get(lvl, row.get("hp")), "hp_percent": 100,
    })
    return {c: row[c] for c in cols if c in row}


def plan(db, files, add=False):
    """[(npcid, name, row, source, note, made)] and what was left [(name,
    why)]; made is a new NPC's row (with --add) and its faction, npcid None."""
    npcs = ours(db)
    gm = {}
    if has_table(db, "npcpaths"):
        logged = set()
        if has_table(db, "npcpaths_seed"):
            logged = {i for (i,) in db.execute("SELECT npcid FROM npcpaths_seed")}
        gm = {i: True for (i,) in db.execute("SELECT npcid FROM npcpaths") if i not in logged}
    used, out, left = set(), [], []
    wanders = {}
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    hp_by_level = {}
    if add:
        levels = collections.defaultdict(list)
        for lvl, hp in db.execute("SELECT npc_level, hp FROM npcs WHERE hp > 0"):
            levels[lvl].append(hp)
        hp_by_level = {lvl: int(statistics.median(v)) for lvl, v in levels.items()}
    folk = town_factions(db) if add else []
    for path in files:
        by, watcher, world, who = read_track(path)
        for sid, (name, samples) in sorted(by.items()):
            if sid >= PLAYER_IDS or (watcher and key(name) == key(watcher)):
                continue
            if wildlife_word(name):
                continue
            r = route(samples)
            if r is None:
                continue
            points, mode, pace, pause, full, spread = r
            if WANDER_PACE[0] <= pace <= WANDER_PACE[1] and not full:
                wanders.setdefault(key(name), (name, spread, f"{os.path.basename(path)} {sid}"))
                continue
            cands = [c for c in npcs.get(key(name), []) if world is None or c[3] == world]
            best = None
            for c in cands:
                if c[0] in used:
                    continue
                d = min(math.hypot(c[1] - p[0], c[2] - p[2]) for p in points)
                if d <= MATCH and (best is None or d < best[0]):
                    best = (d, c)
            if best is None:
                w = world if world is not None else nearest_world(db, points)
                if add and sid in who and w is not None:
                    npc = made_npc(db, cols, who[sid], points, w, hp_by_level)
                    if npc is not None:
                        near = [f for f in folk if f[2] == w]
                        fac = min(near, key=lambda f: math.hypot(f[0] - points[0][0], f[1] - points[0][2]))[3] if near else None
                        row = {"world": w, "mode": mode, "pause": pause, "speed": f"{pace:g}",
                               "points": json.dumps([{"x": x, "y": y, "z": z, **({"pause": p} if p is not None else {})} for x, y, z, p in points])}
                        note = (f"NEW: {len(points)} points, {mode}, {pace:g}/s, pause {pause:g}s, level {npc['npc_level']}, "
                                f"of {fac or 'no faction'}" + ("" if full else " (route not seen to repeat)"))
                        out.append((None, name, row, f"{os.path.basename(path)} {sid}", note, {"npc": npc, "faction": fac}))
                        continue
                left.append((name, f"no NPC of the name within {MATCH:.0f} of its route ({len(cands)} of the name)"))
                continue
            npc = best[1]
            if npc[0] in gm:
                left.append((name, "a GM gave it a path"))
                continue
            used.add(npc[0])
            row = {"world": npc[3], "mode": mode, "pause": pause, "speed": f"{pace:g}",
                   "points": json.dumps([{"x": x, "y": y, "z": z, **({"pause": p} if p is not None else {})} for x, y, z, p in points])}
            note = f"{len(points)} points, {mode}, {pace:g}/s, pause {pause:g}s" + ("" if full else " (route not seen to repeat)")
            out.append((npc[0], name, row, f"{os.path.basename(path)} {sid}", note, None))
    moves = []
    have = {key(n) for (n,) in db.execute("SELECT name FROM npcmove")} if has_table(db, "npcmove") else set()
    for k, (name, spread, source) in sorted(wanders.items()):
        if k in have:
            left.append((name, "wanders on Sandstorm; an npcmove row says how it moves already"))
            continue
        radius = round(min(max(spread, WANDER_RADIUS[0]), WANDER_RADIUS[1]))
        moves.append({"name": k, "roam": 1, "radius": radius, "water": "", "note": "wanders as Sandstorm's (tools/seed_npc_patrols.py)",
                      "source": "patrol-seed", "_shown": name, "_from": source})
    return out, left, moves


def nearest_world(db, points):
    """The world of this database's NPC nearest a route (a track file whose
    JSON says none)."""
    x, z = points[0][0], points[0][2]
    r = db.execute("SELECT world FROM npcs ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?) LIMIT 1",
                   (x, x, z, z)).fetchone()
    return r[0] if r else None


def track_files(paths):
    files = []
    for p in paths:
        files += sorted(glob.glob(os.path.join(p, "*.track.jsonl"))) if os.path.isdir(p) else [p]
    return files


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--tracks", nargs="*", default=[], help="the watch's track files, or folders of them")
    ap.add_argument("--add", action="store_true", help="make a walker of no NPC here one (the module's comment)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the paths (logged)")
    g.add_argument("--undo", action="store_true", help="put back what --apply changed")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not has_table(db, "npcpaths_seed"):
            print("nothing logged: nothing to undo")
            return
        db.execute("BEGIN IMMEDIATE")
        back = kept = gone = 0
        cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
        for npcid, before, after, made in db.execute("SELECT npcid, before, after, made FROM npcpaths_seed").fetchall():
            now = db.execute(f"SELECT {', '.join(COLS)} FROM npcpaths WHERE npcid = ?", (npcid,)).fetchone()
            if now is None or dict(zip(COLS, now)) != json.loads(after):
                kept += 1
                continue
            if before is None:
                db.execute("DELETE FROM npcpaths WHERE npcid = ?", (npcid,))
            else:
                b = json.loads(before)
                db.execute(f"UPDATE npcpaths SET {', '.join(c + ' = ?' for c in COLS)} WHERE npcid = ?", [b[c] for c in COLS] + [npcid])
            back += 1
            if made:
                m = json.loads(made)
                row = db.execute("SELECT * FROM npcs WHERE id = ?", (npcid,)).fetchone()
                if row is not None and {k: v for k, v in zip(cols, row) if k != "id"} == m["npc"]:
                    db.execute("DELETE FROM npcs WHERE id = ?", (npcid,))
                    gone += 1
                if m.get("faction_row"):
                    db.execute("DELETE FROM npcfaction WHERE id = ? AND source = 'patrol-seed'", (m["faction_row"],))
        moved = 0
        if has_table(db, "npcmove_seed"):
            for name, after in db.execute("SELECT name, after FROM npcmove_seed").fetchall():
                r = json.loads(after)
                now = db.execute("SELECT name, roam, radius, water, note, source FROM npcmove WHERE name = ?", (name,)).fetchone()
                if now is not None and dict(zip(("name", "roam", "radius", "water", "note", "source"), now)) == r:
                    db.execute("DELETE FROM npcmove WHERE name = ?", (name,))
                    moved += 1
            db.execute("DROP TABLE npcmove_seed")
        db.execute("DROP TABLE npcpaths_seed")
        db.execute("COMMIT")
        print(f"put back {back} paths, took away {gone} NPCs made and {moved} wandering names, {kept} changed since left "
              "as they are; restart the server")
        return
    if a.apply and has_table(db, "npcpaths_seed"):
        sys.exit("npcpaths_seed is there: applied already (--undo first)")
    files = track_files(a.tracks)
    if not files:
        sys.exit("no track files: give --tracks")
    changes, left, moves = plan(db, files, add=a.add)
    for npcid, name, row, source, note, made in changes:
        print(f"{npcid if npcid else 'new':>7} {name}: {note}  [{source}]")
    for m in moves:
        print(f"wander  {m['_shown']}: every NPC of the name wanders, radius {m['radius']}  [{m['_from']}]")
    for name, why in left:
        print(f"left: {name} ({why})")
    made_n = sum(1 for c in changes if c[5])
    print(f"{len(changes)} patrols ({made_n} NPCs to make), {len(moves)} names to wander, from {len(files)} track files, "
          f"{len(left)} left")
    if not a.apply or not (changes or moves):
        if not a.apply:
            print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        db.execute(MOVE_LOG)
        for m in moves:
            row = {k: v for k, v in m.items() if not k.startswith("_")}
            db.execute(f"INSERT INTO npcmove ({', '.join(row)}) VALUES ({', '.join('?' * len(row))})", list(row.values()))
            db.execute("INSERT INTO npcmove_seed VALUES (?, ?, ?)", (row["name"], json.dumps(row, sort_keys=True), m["_from"]))
        for npcid, name, row, source, note, made in changes:
            log_made = None
            if made:
                npc = made["npc"]
                cur = db.execute(f"INSERT INTO npcs ({', '.join(npc)}) VALUES ({', '.join('?' * len(npc))})", list(npc.values()))
                npcid = cur.lastrowid
                fac_row = None
                if made["faction"] and has_table(db, "npcfaction"):
                    c2 = db.execute("INSERT INTO npcfaction (name, world, x, z, radius, faction, note, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                                    (key(name), npc["world"], npc["x_coord"], npc["z_coord"], 10, made["faction"],
                                     "a patrol made as Sandstorm's (tools/seed_npc_patrols.py)", "patrol-seed"))
                    fac_row = c2.lastrowid
                log_made = json.dumps({"npc": npc, "faction_row": fac_row})
            before = db.execute(f"SELECT {', '.join(COLS)} FROM npcpaths WHERE npcid = ?", (npcid,)).fetchone()
            db.execute(f"INSERT OR REPLACE INTO npcpaths (npcid, {', '.join(COLS)}) VALUES (?, {', '.join('?' * len(COLS))})",
                       [npcid] + [row[c] for c in COLS])
            db.execute("INSERT INTO npcpaths_seed VALUES (?, ?, ?, ?, ?)",
                       (npcid, json.dumps(dict(zip(COLS, before))) if before else None, json.dumps(row), source, log_made))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {len(changes)} paths ({made_n} NPCs made) and {len(moves)} wandering names; restart the server "
          "(NPCs, paths and movement are read at its start)")


if __name__ == "__main__":
    main()
