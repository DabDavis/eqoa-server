#!/usr/bin/env python3
"""Pike by the Orc Ransackers' shack (Kyros's petition 2060, 2026-11-14: "Pikes for FP Bard 13 (I think?) are located
too far up river from what I remember. I believe (in live and in SS) they were located connable from the Orc Ransacker
house and just barely upstream of the chichans").

Guard Sareken's quest (100067/100087/100097/100107/100117, step 2: three tough pike scales "from the pike in the river
north of Freeport", step 4 the orc ransacker's boots "in the shack north of Freeport") has its pike from
tools/seed_quest_npcs.py's set freeport-pike: 15 rows in six groups along the river at the broken bridge, z 13200 to
13800, 800 up the river from the shack (the Orc Ransackers stand at x 25016-25078, z 14530-14580) and the chichan eel
(25146.8, 14550.4). The river there runs south: its upper reach is the north (smaller z).

The set was placed by the owner's word (2026-11-03: "add pike near freeport for the quests, should be north near the
bridge and river"), so its three groups nearest the bridge itself are left there (142293-142295, 142299-142302, and the
three imported pike at z 12950); the other three groups, eight pike (142290-142292 "by the broken bridge",
142296-142298 "below the bridge", 142303-142304 "the river's south-east reach"), move down the river to just above the
eel: z 14445-14526 in the channel's west half, 6 to 13 deep, 60-130 from the ransackers. Every spot is in the water by
the bake's environment (worlds/ground's 0.env, a point half a unit over the floor flagged water, the water's surface at
51.3-51.6) and its height is the floor there (the bake's 0.ground), as the set's other pike stand; read with
internal/ground off ~/.cache/movecheck/ground on 2026-11-14.

Each row moves only while it is a pike where the seed put it (to 0.01); one not is named and left. A report by default;
--apply in one transaction, a gm_audit line a row, logged in pike_ransacker_1114_log; --undo puts back each row still as
written. NPC positions are read at the start: RESTART after --apply.

    python3 -E -s tools/pike_ransacker_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "pike_ransacker_1114_log"
DETAIL = "petition 2060 (Kyros): pike down the river by the Orc Ransackers' shack, above the chichan eel; tools/pike_ransacker_20261114.py"
WHO, VIA = "dab", "tool"
WORLD = 0
EPS = 0.01
# id: (where the seed put it (x, y, z), where it goes (x, y, z))
MOVES = {
    142290: ((24875.0, 42.063, 13550.0), (25119.0, 44.52, 14445.0)),
    142291: ((24851.584, 44.7298, 13530.599), (25128.0, 41.10, 14456.0)),
    142292: ((24845.408, 43.6158, 13499.951), (25124.0, 44.62, 14468.0)),
    142296: ((24962.0, 43.7951, 13650.0), (25133.0, 42.25, 14478.0)),
    142297: ((25039.928, 50.1952, 13688.681), (25130.0, 45.12, 14492.0)),
    142298: ((25037.872, 47.7698, 13724.394), (25140.0, 38.33, 14502.0)),
    142303: ((25087.0, 43.9114, 13750.0), (25136.0, 42.81, 14515.0)),
    142304: ((25097.964, 42.4839, 13798.026), (25145.0, 39.48, 14526.0)),
}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    npcid  INTEGER NOT NULL,
    before TEXT NOT NULL,     -- x_coord, y_coord, z_coord before (JSON)
    after  TEXT NOT NULL,     -- as written (JSON)
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True)


def audit(db, npcid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'npcs', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, compact({"id": npcid}), compact(before), compact(after), DETAIL))
    return cur.lastrowid


def pos(db, npcid):
    r = db.execute("SELECT npc_name, world, x_coord, y_coord, z_coord FROM npcs WHERE id = ?", (npcid,)).fetchone()
    return None if r is None else (r[0], r[1], {"x_coord": r[2], "y_coord": r[3], "z_coord": r[4]})


def at(p, xyz):
    return all(abs(float(p[k]) - v) <= EPS for k, v in zip(("x_coord", "y_coord", "z_coord"), xyz))


def plan(db):
    """(moves [(id, before, after)], lines, skipped, applied)."""
    for t in ("npcs", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"], False
    moves, lines, skipped, done = [], [], [], 0
    for npcid, (was, to) in MOVES.items():
        p = pos(db, npcid)
        if p is None:
            skipped.append(f"{npcid}: gone")
            continue
        name, world, cur = p
        if name != "pike" or world != WORLD:
            skipped.append(f"{npcid}: now {name!r} in world {world}")
        elif at(cur, to):
            done += 1
        elif not at(cur, was):
            skipped.append(f"{npcid}: moved since the seed (at {cur['x_coord']:.1f}, {cur['z_coord']:.1f})")
        else:
            after = dict(zip(("x_coord", "y_coord", "z_coord"), to))
            moves.append((npcid, cur, after))
            lines.append(f"{npcid} pike: ({cur['x_coord']:.1f}, {cur['y_coord']:.1f}, {cur['z_coord']:.1f}) -> "
                         f"({to[0]:.1f}, {to[1]:.2f}, {to[2]:.1f})")
    return moves, lines, skipped, done == len(MOVES)


def apply(db, moves):
    db.execute(LOG_SCHEMA)
    for npcid, before, after in moves:
        db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ? WHERE id = ?",
                   (after["x_coord"], after["y_coord"], after["z_coord"], npcid))
        a = audit(db, npcid, before, after)
        db.execute(f"INSERT INTO {LOG} (npcid, before, after, audit) VALUES (?, ?, ?, ?)",
                   (npcid, compact(before), compact(after), a))
    return len(moves)


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for npcid, before, after in db.execute(f"SELECT npcid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        was, wrote, p = json.loads(before), json.loads(after), pos(db, npcid)
        if p is None or not at(p[2], (wrote["x_coord"], wrote["y_coord"], wrote["z_coord"])):
            left += 1
            continue
        db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ? WHERE id = ?",
                   (was["x_coord"], was["y_coord"], was["z_coord"], npcid))
        audit(db, npcid, p[2], was)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + (f", {left} changed since (left, the log kept)" if left else ""))
            return 0
        moves, lines, skipped, applied = plan(db)
        if applied:
            db.execute("ROLLBACK")
            print("applied already: nothing to do")
            return 0
        for l in lines:
            print(l)
        for s in skipped:
            print("left: " + s)
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write; RESTART after)")
            return 0
        n = apply(db, moves)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; RESTART the server")
        return 0
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
