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

Who is left out: an animal by its name (seed_npc_move.py's wildlife_word, the
server's own list), and the watcher itself. Wildlife wanders here by
roam.go, not by a path.

Each route goes to this database's NPC of the same name (any case, no "$")
in the city's world whose spawn is nearest the route, within 150; each NPC
once. An NPC with a path a GM gave it is left and said.

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
LOG_SCHEMA = """CREATE TABLE npcpaths_seed (
    npcid  INTEGER PRIMARY KEY,
    before TEXT,             -- the npcpaths row before, JSON, or NULL for none
    after  TEXT NOT NULL,    -- the row written, JSON
    source TEXT NOT NULL     -- the track file and the Sandstorm id
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
    by, me = {}, None
    with open(path) as f:
        for line in f:
            try:
                r = json.loads(line)
            except ValueError:
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
    return by, watcher, world


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
                core.append((u["x"], u["y"], u["z"], 0.0))
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
    # the route is its stops' and turns' shortest repeat, seen once and more
    for p in range(2, len(dedup) // 2 + 1):
        if len(dedup) >= p + 2 and all(dedup[i] == dedup[i + p] for i in range(len(dedup) - p)):
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
    return points, mode, pace, pause, full


def ours(db):
    """This database's NPCs by name key: [(id, x, z, world)]."""
    out = collections.defaultdict(list)
    for i, name, x, z, w in db.execute("SELECT id, npc_name, x_coord, z_coord, world FROM npcs"):
        out[key(name)].append((i, x, z, w))
    return out


def plan(db, files):
    """[(npcid, name, row, source, note)] and what was left [(name, why)]."""
    npcs = ours(db)
    gm = {}
    if has_table(db, "npcpaths"):
        logged = set()
        if has_table(db, "npcpaths_seed"):
            logged = {i for (i,) in db.execute("SELECT npcid FROM npcpaths_seed")}
        gm = {i: True for (i,) in db.execute("SELECT npcid FROM npcpaths") if i not in logged}
    used, out, left = set(), [], []
    for path in files:
        by, watcher, world = read_track(path)
        for sid, (name, samples) in sorted(by.items()):
            if watcher and key(name) == key(watcher):
                continue
            if wildlife_word(name):
                continue
            r = route(samples)
            if r is None:
                continue
            points, mode, pace, pause, full = r
            cands = [c for c in npcs.get(key(name), []) if world is None or c[3] == world]
            best = None
            for c in cands:
                if c[0] in used:
                    continue
                d = min(math.hypot(c[1] - p[0], c[2] - p[2]) for p in points)
                if d <= MATCH and (best is None or d < best[0]):
                    best = (d, c)
            if best is None:
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
            out.append((npc[0], name, row, f"{os.path.basename(path)} {sid}", note))
    return out, left


def track_files(paths):
    files = []
    for p in paths:
        files += sorted(glob.glob(os.path.join(p, "*.track.jsonl"))) if os.path.isdir(p) else [p]
    return files


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--tracks", nargs="*", default=[], help="the watch's track files, or folders of them")
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
        back = kept = 0
        for npcid, before, after in db.execute("SELECT npcid, before, after FROM npcpaths_seed").fetchall():
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
        db.execute("DROP TABLE npcpaths_seed")
        db.execute("COMMIT")
        print(f"put back {back} paths, {kept} changed since left as they are; restart the server")
        return
    if a.apply and has_table(db, "npcpaths_seed"):
        sys.exit("npcpaths_seed is there: applied already (--undo first)")
    files = track_files(a.tracks)
    if not files:
        sys.exit("no track files: give --tracks")
    changes, left = plan(db, files)
    for npcid, name, row, source, note in changes:
        print(f"{npcid:7d} {name}: {note}  [{source}]")
    for name, why in left:
        print(f"left: {name} ({why})")
    print(f"{len(changes)} patrols from {len(files)} track files, {len(left)} left")
    if not a.apply or not changes:
        if not a.apply:
            print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(LOG_SCHEMA)
        for npcid, name, row, source, note in changes:
            before = db.execute(f"SELECT {', '.join(COLS)} FROM npcpaths WHERE npcid = ?", (npcid,)).fetchone()
            db.execute(f"INSERT OR REPLACE INTO npcpaths (npcid, {', '.join(COLS)}) VALUES (?, {', '.join('?' * len(COLS))})",
                       [npcid] + [row[c] for c in COLS])
            db.execute("INSERT INTO npcpaths_seed VALUES (?, ?, ?, ?)",
                       (npcid, json.dumps(dict(zip(COLS, before))) if before else None, json.dumps(row), source))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {len(changes)} paths; restart the server (paths are read at its start)")


if __name__ == "__main__":
    main()
