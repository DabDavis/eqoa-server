#!/usr/bin/env python3
"""The teleport orbs: table orbs (entity/orbs.go, database/orbs.go).

The user, 2026-11-09: "the erudite starts that are in towers the orb wont
teleport them yet to the main highbourne". A data file of tools/orbs/ holds a
place's pads as the disc has them, read off its zone actors (the file says
which world file, zone, room and actor each is, and the banner sprites that
stand behind it), and which pad pairs with which; this writes a row of orbs
for each pad:

- where it stands: the pad's middle as its zone actor places it, its height
  the pad's (the floor it stands on), radius 0 (the server's, 1.5: on the
  pad's top);
- where it sends: beside its pair, LAND_OFF out from the pair's middle on the
  side away from the pair's banners (each pad's banners stand between it and
  its room's wall: the other side is the room, where a body walks off it), on
  the floor there, turned that way (to_facing, a 0x40's heading: 0 faces +z);
  never inside any orb of the file.

With the ground files (--ground worlds/ground: cmd/groundbake's) each landing's
height is the terrain's floor there nearest its pair's own height, within
FLOOR_TOL; a landing with no such floor leaves its pair out (both orbs: one
that sent a player where nothing would send it back is no pair), and is said.
Without the ground files nothing is written.

    python3 -E -s tools/seed_orbs.py --db eqoa.db --ground worlds/ground               report
    python3 -E -s tools/seed_orbs.py --db eqoa.db --ground worlds/ground --apply       write it
    python3 -E -s tools/seed_orbs.py --db eqoa.db --undo                               take the last run back
    --data tools/orbs/highbourne.json   one file (default: every file of tools/orbs/)
    --sets towers                       only the orbs of those sets (default: all)

Rows go in in one transaction, source seed, each logged in orb_seed with what
was written and the run's time; an orb whose name has a row already (a GM's,
an earlier run's) is left alone and said, and so is one an earlier run wrote
that is gone since (a GM took it out). --undo takes the last run back: deletes
the rows it wrote that no GM changed since (a changed one is said and kept)
and its log rows; with none left it drops the log, and the table when the run
made it and it is empty again, so an apply and its undo leave the database as
it was. A copy of the database is made first (--no-backup to skip). The server
reads the table at its start and at a GM's edit in the panel ("Teleport orbs"):
a running server takes a written row at its next restart (or open and save a
row in the panel).
"""
import argparse
import datetime
import glob
import json
import math
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from seed_wildlife import Ground  # noqa: E402  (the server's ground format)

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = "eqoa.db"
DEFAULT_GROUND = "worlds/ground"
DATA_DIR = os.path.join(HERE, "orbs")
SOURCE = "seed"
LOG = "orb_seed"
MADE = "#made the table"  # the log's mark that a run made orbs

# how far from its pair's middle an orb lands a player: out of the pad (3.7
# across) and of the orb's radius (1.5), a step clear of both
LAND_OFF = 3.5
# how near the pair's own height the landing's floor must be
FLOOR_TOL = 0.5
# banners further from a pad than this are another's
BANNER_NEAR = 6.0
# entity/orbs.go's: a body in an orb (database.OrbHolds)
ORB_RADIUS, ORB_BELOW, ORB_ABOVE = 1.5, 1.0, 2.5

# the server's own schema (database/orbs.go), for a database it has not opened yet
SCHEMA = """
	CREATE TABLE IF NOT EXISTS orbs (
		id        INTEGER PRIMARY KEY,
		name      TEXT NOT NULL DEFAULT '',
		world     INTEGER NOT NULL DEFAULT 0,
		x         REAL NOT NULL DEFAULT 0,
		y         REAL NOT NULL DEFAULT 0,
		z         REAL NOT NULL DEFAULT 0,
		radius    REAL NOT NULL DEFAULT 0,
		to_world  INTEGER NOT NULL DEFAULT 0,
		to_x      REAL NOT NULL DEFAULT 0,
		to_y      REAL NOT NULL DEFAULT 0,
		to_z      REAL NOT NULL DEFAULT 0,
		to_facing REAL NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);"""
COLS = ("name", "world", "x", "y", "z", "radius", "to_world", "to_x", "to_y", "to_z", "to_facing", "note", "source")


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def holds(o, x, y, z):
    """database.OrbHolds for an orb of the default radius at o's pad."""
    return math.hypot(x - o["x"], z - o["z"]) <= ORB_RADIUS and o["y"] - ORB_BELOW <= y <= o["y"] + ORB_ABOVE


def read_data(paths):
    """Every orb of the files, each with its file's world, file and zone; a key
    twice, a pair missing or not pointing back, or banners of two kinds on a
    pair, refused."""
    orbs = []
    for path in paths:
        with open(path) as f:
            d = json.load(f)
        for o in d["orbs"]:
            orbs.append(dict(o, world=d["world"], file=d["file"], zone=d["zone"], pad=d["pad"], data=os.path.basename(path)))
    by = {}
    for o in orbs:
        if o["key"] in by:
            raise SystemExit(f"orb {o['key']} twice")
        by[o["key"]] = o
    for o in orbs:
        p = by.get(o["pair"])
        if p is None or p["pair"] != o["key"]:
            raise SystemExit(f"orb {o['key']}: its pair {o['pair']} is not there or does not pair with it")
        if p["banner"] != o["banner"] or p["world"] != o["world"]:
            raise SystemExit(f"orb {o['key']} and {p['key']}: another banner or world ({o['banner']}, {p['banner']})")
        if not o.get("banners") or any(math.hypot(bx - o["x"], bz - o["z"]) > BANNER_NEAR for bx, bz in o["banners"]):
            raise SystemExit(f"orb {o['key']}: no banner within {BANNER_NEAR} of it")
    return orbs, by


def open_side(o):
    """The heading away from an orb's banners (the room's side of the pad), a
    0x40's: atan2(x, z) of the way from the banners' middle to the pad's."""
    mx = sum(b[0] for b in o["banners"]) / len(o["banners"])
    mz = sum(b[1] for b in o["banners"]) / len(o["banners"])
    return math.atan2(o["x"] - mx, o["z"] - mz)


def floor_near(g, x, y, z):
    """The terrain's floor at (x, z) nearest the height y within FLOOR_TOL,
    None for none."""
    best = None
    for h in g.floors(x, z):
        if abs(h - y) <= FLOOR_TOL and (best is None or abs(h - y) < abs(best - y)):
            best = h
    return best


def plan(orbs, by, ground_dir, sets=None):
    """The rows to write, in the files' order, and what was left out (why).
    ground(world) gives the world's Ground or None."""
    cache = {}

    def ground(world):
        if not ground_dir:
            return None
        if world not in cache:
            p = os.path.join(ground_dir, f"{world}.ground")
            cache[world] = Ground(p) if os.path.exists(p) else None
        return cache[world]

    chosen = [o for o in orbs if not sets or o["set"] in sets]
    land, missing = {}, []
    for o in chosen + [by[o["pair"]] for o in chosen]:
        if o["key"] in land:
            continue
        h = open_side(o)
        x, z = o["x"] + LAND_OFF * math.sin(h), o["z"] + LAND_OFF * math.cos(h)
        g = ground(o["world"])
        land[o["key"]] = (x, floor_near(g, x, o["y"], z) if g is not None else None, z, h)
    rows = []
    for o in chosen:
        p = by[o["pair"]]
        x, y, z, h = land[p["key"]]
        if ground(o["world"]) is not None and y is None:
            missing.append(f"{o['key']}: beside {p['key']}, at ({x:.1f}, {z:.1f}), no floor is within {FLOOR_TOL} of {p['y']:.2f}")
            continue
        if ground(o["world"]) is not None and land[o["key"]][1] is None:
            missing.append(f"{o['key']}: left out with {p['key']}, which could send no one back beside it")
            continue
        inside = [q["key"] for q in orbs if q["world"] == o["world"] and holds(q, x, y, z)]
        if inside:
            missing.append(f"{o['key']}: its landing is inside {', '.join(inside)}")
            continue
        note = (f"The disc's pad (sprite {o['pad']}, {o['file']} zone {o['zone']} room {o['room']} actor {o['actor']}, "
                f"at ({o['x']}, {o['y']}, {o['z']}), heading {o['rot']}), its banner {o['banner']}; its pair {p['key']}'s "
                f"(room {p['room']} actor {p['actor']}) by that banner. Lands {LAND_OFF} off the pair's middle on the side "
                f"away from the pair's banners, on the floor there, facing that way (tools/seed_orbs.py, {o['data']}).")
        rows.append(dict(name=o["name"], world=o["world"], x=o["x"], y=o["y"], z=o["z"], radius=0.0,
                         to_world=p["world"], to_x=round(x, 3), to_y=round(y, 3), to_z=round(z, 3), to_facing=round(h, 4),
                         note=note, source=SOURCE, _key=o["key"], _set=o["set"]))
    for g in cache.values():
        if g is not None:
            g.f.close()
    return rows, missing


def standing(db, rows):
    """Each planned row's standing, by its name: "there" (orbs has a row of the
    name), "gone" (an earlier run wrote it and it is not there now: a GM took it
    out), else "new"."""
    there = set()
    if has_table(db, "orbs"):
        there = {r[0] for r in db.execute("SELECT name FROM orbs")}
    logged = {r[0] for r in db.execute(f"SELECT name FROM {LOG}")} if has_table(db, LOG) else set()
    return {r["name"]: "there" if r["name"] in there else "gone" if r["name"] in logged else "new" for r in rows}


def apply(db, rows, when=None):
    """Writes the rows standing "new" in one transaction, each logged under
    this run's time; the rest are left alone. Returns (wrote, kept, gone)."""
    if any(r["to_y"] is None for r in rows):
        sys.exit("not written: a landing has no height (--ground worlds/ground)")
    when = when or datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    wrote, kept, gone = [], [], []
    db.execute("BEGIN IMMEDIATE")
    try:
        if has_table(db, LOG):
            last = db.execute(f"SELECT max(applied) FROM {LOG}").fetchone()[0]
            if last is not None and when <= last:
                raise SystemExit(f"the last run was applied at {last}, this one at {when}: run it again in a second")
        made = not has_table(db, "orbs")
        if made and has_table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG} WHERE name = ?", (MADE,)).fetchone()[0]:
            raise SystemExit(f"{LOG} says a run made orbs, and the table is gone: look before writing")
        db.execute(SCHEMA)
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (name TEXT NOT NULL, id INTEGER NOT NULL, row TEXT NOT NULL, applied TEXT NOT NULL)")
        if made:
            db.execute(f"INSERT INTO {LOG} VALUES (?, 0, '[]', ?)", (MADE, when))
        how = standing(db, rows)
        for r in rows:
            if how[r["name"]] == "there":
                kept.append(r["name"])
                continue
            if how[r["name"]] == "gone":
                gone.append(r["name"])
                continue
            vals = [r[c] for c in COLS]
            cur = db.execute(f"INSERT INTO orbs ({', '.join(COLS)}) VALUES ({', '.join('?' * len(COLS))})", vals)
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?, ?)", (r["name"], cur.lastrowid, json.dumps(vals), when))
            wrote.append(r["name"])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return wrote, kept, gone


def undo(db):
    """Takes the last run back (the latest time in the log): deletes the rows it
    wrote that no GM changed since (a changed one is said and kept) and its log
    rows; with none left, drops the log, and orbs when the run made it and it is
    empty again. Returns (deleted, changed, the run's time)."""
    if not has_table(db, LOG):
        sys.exit("no run to undo here")
    deleted, changed = 0, []
    db.execute("BEGIN IMMEDIATE")
    try:
        last = db.execute(f"SELECT max(applied) FROM {LOG}").fetchone()[0]
        made = False
        for name, rid, row in db.execute(f"SELECT name, id, row FROM {LOG} WHERE applied = ? ORDER BY id", (last,)).fetchall():
            if name == MADE:
                made = True
                continue
            now = db.execute(f"SELECT {', '.join(COLS)} FROM orbs WHERE id = ?", (rid,)).fetchone()
            if now is None:
                continue
            if list(now) != json.loads(row):
                changed.append(name)
                continue
            deleted += db.execute("DELETE FROM orbs WHERE id = ?", (rid,)).rowcount
        db.execute(f"DELETE FROM {LOG} WHERE applied = ?", (last,))
        if db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] == 0:
            db.execute(f"DROP TABLE {LOG}")
        if made and db.execute("SELECT count(*) FROM orbs").fetchone()[0] == 0:
            db.execute("DROP TABLE orbs")  # the run made it
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return deleted, changed, last


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-orbs"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(rows, missing, how=None):
    how = how or {}
    print(f"{len(rows)} orbs; {sum(v == 'new' for v in how.values())} to write")
    for r in rows:
        y = "?" if r["to_y"] is None else f"{r['to_y']:.2f}"
        print(f"  [{r['_set']}] {r['name']}\n      at ({r['x']:.1f}, {r['y']:.2f}, {r['z']:.1f}) world {r['world']}: "
              f"lands ({r['to_x']:.1f}, {y}, {r['to_z']:.1f}) world {r['to_world']}, facing {r['to_facing']:.2f}"
              + {"there": " [a row of the name there: left alone]",
                 "gone": " [an earlier run's, taken out since: left alone]"}.get(how.get(r["name"]), ""))
    for m in missing:
        print("  left out:", m)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    ap.add_argument("--data", action="append", help="a data file (default: every file of tools/orbs/)")
    ap.add_argument("--sets", help="only the orbs of these sets, comma-separated (default: all)")
    ap.add_argument("--apply", action="store_true", help="write the rows")
    ap.add_argument("--undo", action="store_true", help="delete the rows the last run wrote that no GM changed")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    args = ap.parse_args()
    db = sqlite3.connect(args.db, isolation_level=None)
    if args.undo:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        deleted, changed, run = undo(db)
        print(f"the run of {run}: deleted {deleted} rows" + (f"; kept, changed since: {', '.join(changed)}" if changed else ""))
        return
    paths = args.data or sorted(glob.glob(os.path.join(DATA_DIR, "*.json")))
    orbs, by = read_data(paths)
    sets = {s.strip() for s in args.sets.split(",")} if args.sets else None
    ground = args.ground if os.path.isdir(args.ground) else None
    if ground is None:
        print(f"(no ground files at {args.ground}: no landing's height, nothing can be written)")
    rows, missing = plan(orbs, by, ground, sets)
    report(rows, missing, standing(db, rows))
    if args.apply:
        if ground is None:
            sys.exit("not written: no ground files (--ground worlds/ground)")
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        wrote, kept, gone = apply(db, rows)
        print(f"wrote {len(wrote)} rows" + (f"; left alone, there already: {', '.join(kept)}" if kept else "")
              + (f"; left alone, taken out since: {', '.join(gone)}" if gone else ""))


if __name__ == "__main__":
    main()
