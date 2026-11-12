#!/usr/bin/env python3
"""Move Blackburrow's Dungeon Warden twin to the dungeon's new landing (petition 1808, Poppapump, 2026-11-12: "Think
port isnt putting party inside invis wall, got stuck", filed from world 14 at (13115, 12106)).

Blackburrow's landing was before the burrow's north-east mouth at (13115, 56.8, 12095), 95 inside Tunaria cell (6, 6)'s
north edge, where cmd/worldedges' ridge stands (tools/worlds/dungeons-edges.json: rocks of 0x7FFD2E87 at scale 28-29,
reaching 250 into the cell): the landing was UNDER one of them (its underside 0.5 over the ground there, its top at
370), and the mouth itself is buried by it. The recipe's landing is now before the burrow's south-west mouth, (12590,
62.1, 12760), facing north into it (tools/worlds/dungeons.json; the server reads it from worlds/world14.json at its
start; the world file is unchanged). This moves the warden's twin there with it: npcs row of world 14 named "Dungeon
Warden" at (13119, 12095) -> (12586, 61.2, 12760), 4 to the landing's right, facing it (byte 64).

Refused (nothing done) when the row is not where it was put (a GM moved it) or the spot is taken. One transaction,
gm_audit rows, its record in dungeons_landing_1112_log; --undo puts it back exactly (refused when it changed since).
Undo this before tools/dungeons_20261112.py --undo (that one refuses a row changed since it wrote it).

    python3 -E -s tools/dungeons_landing_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import json
import math
import sqlite3
import sys
import time

RUN = "dungeons landing 20261112"
LOG = "dungeons_landing_1112_log"
NAME, WORLD = "Dungeon Warden", 14
OLD = (13119.0, 12095.0)
NEW = {"x_coord": 12586.0, "y_coord": 61.2, "z_coord": 12760.0, "facing": 64}


def rowd(db, rid):
    cur = db.execute("SELECT * FROM npcs WHERE id = ?", (rid,))
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def audit(db, now, key, before, after):
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', 'update', 'npcs', ?, ?, ?, ?)""",
               (now, json.dumps(key), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True),
                "petition 1808: Blackburrow's landing out from under the edge's rock; tools/dungeons_landing_20261112.py"))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, run TEXT NOT NULL, npc INTEGER NOT NULL,
                   before TEXT NOT NULL, after TEXT NOT NULL, at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
                   undone TEXT NOT NULL DEFAULT '')""")
    db.commit()
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    done = db.execute(f"SELECT seq, npc, before, after FROM {LOG} WHERE run = ? AND undone = ''", (RUN,)).fetchall()
    if a.undo:
        if not done:
            sys.exit("nothing applied: nothing done")
        seq, rid, before, after = done[0]
        cur = rowd(db, rid)
        if cur is None or json.dumps(cur, sort_keys=True) != after:
            sys.exit(f"npcs {rid} changed since (a GM's edit?): nothing done")
        b = json.loads(before)
        with db:
            db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
                       (b["x_coord"], b["y_coord"], b["z_coord"], b["facing"], rid))
            audit(db, now, {"id": rid}, cur, rowd(db, rid))
            db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (now, seq))
        print(f"undone: npcs {rid} back at ({b['x_coord']}, {b['y_coord']}, {b['z_coord']}) facing {b['facing']}")
        return
    if done:
        print("applied already: nothing done")
        return
    rows = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) < 1 AND abs(z_coord - ?) < 1",
                      (NAME, WORLD, OLD[0], OLD[1])).fetchall()
    if len(rows) != 1:
        sys.exit(f"{len(rows)} {NAME} rows of world {WORLD} at {OLD}: nothing done")
    near = db.execute("SELECT id, npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 10 AND abs(z_coord - ?) < 10",
                      (WORLD, NEW["x_coord"], NEW["z_coord"])).fetchall()
    for nid, n, x, z in near:
        if math.hypot(x - NEW["x_coord"], z - NEW["z_coord"]) < 3:
            sys.exit(f"{n} (npcs {nid}) stands at the new spot: nothing done")
    rid = rows[0][0]
    before = rowd(db, rid)
    db.execute("BEGIN")
    db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
               (NEW["x_coord"], NEW["y_coord"], NEW["z_coord"], NEW["facing"], rid))
    after = rowd(db, rid)
    audit(db, now, {"id": rid}, before, after)
    db.execute(f"INSERT INTO {LOG} (run, npc, before, after) VALUES (?, ?, ?, ?)",
               (RUN, rid, json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True)))
    print(f"npcs {rid}: {NAME} from ({before['x_coord']}, {before['y_coord']}, {before['z_coord']}) facing {before['facing']} "
          f"to ({NEW['x_coord']}, {NEW['y_coord']}, {NEW['z_coord']}) facing {NEW['facing']}")
    if a.apply:
        db.commit()
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
