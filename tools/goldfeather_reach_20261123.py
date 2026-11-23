#!/usr/bin/env python3
"""Goldfeather Eyrie's creatures on the mountain a player cannot climb to (petition 2686, Heirloom, 2026-11-22: "zone
sw has invis wall on the mountain. issue is most the mobs are above that, incuding a lot of ones needed for quest";
he stood at world 10 (14658.3, 145.2, 7624.8), on the mountain's south-west slope).

Goldfeather Eyrie (world 10, the homes' shared ground, tools/worlds/homelands.json; cell 14000-16000 x, 6000-8000 z)
has its L6-L20 creatures where the recipe stood them, several on the mountain's tops. The server's movement check is in
log mode on Heirloom and moved him nowhere (one MOVECHECK line all evening, a jump taken); there is no blocker entity in
the zone and none of the edge rocks (tools/worlds/homelands-edges.json) is near him. The wall is the terrain: the
game's walk (vanilla's crates/vi/src/walk.rs, VIWalkControl::Move) sweeps a 0.6 ball at 1.2 over the feet and stops
at a face it meets, so the walk runs up a slope only to about 47 degrees at a run (1.2 cos a - 0.28 sin a = 0.6) and a
steeper stretch of the snow is a wall that looks like ground.

Measured with vanilla's own walk on the live world10.ESF (its walker, run from his place 12 ways a second at a time,
every 4-unit cell it stood on, 21,129 places): it climbs to y 209 at most; seven creatures stand at 215-255 and six
more on knolls over the slope it runs on. cmd/playerreach's survey (from the outpost at (14500, 66.5, 7400)) at its
default (1:1 plus a step of 1.3 on a grid of 2: about 59 degrees) reaches every one of them, and so missed them; at a
step of 0.6 (about 52 degrees) it parts them off exactly as the walk does, and its places to move them to are places
the walk stood (within 3 across and 3 up of one), but two below the walk's survey (it went no further down than a
second's fall; there the survey's place is the outpost's reach):

    go run ./cmd/playerreach -ground worlds/ground -world 10 -box 14000,6000,16000,8000 -seed 14500,66.5,7400         -stepup 0.6 -npcs NPCS.json   (NPCS: {id, x, y, z} a creature of the cell)

The 13 creatures standing OVER the reached ground (a player comes down from where they stand, never up to it) are
MOVED, camps (rows within 20 of one another) as they stood, to the nearest open reached floor (playerreach's rules:
every floor within 6 reached and within 3 of its height, 5 from every creature that stays, 35 from another moved
camp): moves of 36-278, median 114. Not changed here (a GM's to look at): the 22 creatures of the cell in pockets
walled off from the outpost's ground at that rule (the east snow's owl and rat at 206-207, rats and badgers at the
south edge at 67-145, the rime elementals in their hollow, which no rule reaches).

Rows are found by id, name, world 10 and where they stood (to 0.01); on a shard whose rows differ (another id, the
same name and place) by name and place; a row a GM has moved since, or that is gone, is left alone and said. Report
only by default; --apply writes the moves in one transaction with a gm_audit line, the log goldfeather_reach_1123_log
(every row before and after); a second --apply does nothing; --undo puts back exactly what it wrote (a row changed
since is left and said) and drops the log. tools/worlds/homelands.json's shared npcs stand where this moves them (the
nine of them it holds), so a seeding from it agrees.

WHEN IT TAKES EFFECT: the npcs rows are the spawns, read at the server's start or by the GM panel's reload_npcs; a
creature alive where it stood is there until it dies or the reload.

    python3 -E -s tools/goldfeather_reach_20261123.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "goldfeather_reach_1123_log"
WHO, VIA = "dab", "tool"
DETAIL = ("petition 2686: Goldfeather Eyrie's creatures on the mountain a player cannot climb to, moved to the floor below: "
          "tools/goldfeather_reach_20261123.py")
WORLD = 10
TOL = 0.01

# (row, name, (x, y, z) as surveyed, (x, y, z) to, why): cmd/playerreach's places, camps kept as they stood
MOVES = [
    (165827, 'drift rat', (15272.5, 185.8, 7640.8), (15204, 204.61, 7372),
     "on the mountain at y 186, over every floor a player climbs to (the nearest 267 across at y 205): moved 277 to the floor below (the walk reached it from the petitioner's place)"),
    (165828, 'snowfield badger', (14634.9, 148.9, 7656.0), (14610, 125.15, 7630),
     "on the mountain at y 149, over every floor a player climbs to (the nearest 11 across at y 143): moved 36 to the floor below (the walk reached it from the petitioner's place)"),
    (165831, 'snowfield badger', (15471.8, 225.1, 7007.4), (15358, 200.14, 7008),
     "on the mountain at y 225, over every floor a player climbs to (the nearest 108 across at y 202): moved 114 to the floor below (below the walk's survey (it went no further down than a second's fall), on the floor the outpost's survey reaches)"),
    (165832, 'snowfield badger', (14707.4, 218.9, 7771.7), (14638, 129.95, 7608),
     "on the mountain at y 219, over every floor a player climbs to (the nearest 143 across at y 142): moved 178 to the floor below (the walk reached it from the petitioner's place)"),
    (165833, 'snowfield badger', (14580.9, 215.4, 7812.0), (14486, 181.68, 7830),
     "on the mountain at y 215, over every floor a player climbs to (the nearest 69 across at y 166): moved 97 to the floor below (the walk reached it from the petitioner's place)"),
    (165834, 'snowfield badger', (14615.5, 153.4, 7683.9), (14550, 102.79, 7658),
     "on the mountain at y 153, over every floor a player climbs to (the nearest 16 across at y 142): moved 70 to the floor below (below the walk's survey (it went no further down than a second's fall), on the floor the outpost's survey reaches)"),
    (165835, 'snowfield badger', (15047.0, 237.7, 7472.8), (15046, 195.91, 7402),
     "on the mountain at y 238, over every floor a player climbs to (the nearest 65 across at y 198): moved 71 to the floor below (the walk reached it from the petitioner's place)"),
    (165838, 'rime spider', (15272.7, 183.3, 7648.9), (15204, 204.61, 7380),
     "on the mountain at y 183, over every floor a player climbs to (the nearest 275 across at y 205): moved 278 to the floor below (the walk reached it from the petitioner's place)"),
    (165843, 'rime spider', (15404.4, 255.0, 7428.8), (15278, 194.55, 7304),
     "on the mountain at y 255, over every floor a player climbs to (the nearest 172 across at y 195): moved 178 to the floor below (the walk reached it from the petitioner's place)"),
    (196262, 'drift rat', (14676.8, 167.56, 7673.6), (14674, 139.85, 7606),
     "on the mountain at y 168, over every floor a player climbs to (the nearest 50 across at y 144): moved 68 to the floor below (the walk reached it from the petitioner's place)"),
    (196265, 'drift rat', (14836.5, 223.68, 7633.4), (14700, 128.76, 7558),
     "on the mountain at y 224, over every floor a player climbs to (the nearest 115 across at y 169): moved 156 to the floor below (the walk reached it from the petitioner's place)"),
    (196267, 'drift rat', (14814.7, 166.63, 7789.3), (14664, 117.4, 7566),
     "on the mountain at y 167, over every floor a player climbs to (the nearest 226 across at y 143): moved 269 to the floor below (the walk reached it from the petitioner's place)"),
    (196268, 'drift rat', (14918.3, 215.31, 7460.8), (14910, 186.2, 7402),
     "on the mountain at y 215, over every floor a player climbs to (the nearest 43 across at y 190): moved 59 to the floor below (the walk reached it from the petitioner's place)"),
]
# (row, name, (x, y, z) as surveyed, why)
REMOVES = [
]


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def npc(db, rid):
    r = rows(db, "SELECT * FROM npcs WHERE id = ?", (rid,))
    return r[0] if r else None


def at(r, xyz):
    return all(r[c] is not None and abs(r[c] - v) <= TOL for c, v in zip(("x_coord", "y_coord", "z_coord"), xyz))


def where(r):
    return f"({r['x_coord']:.2f}, {r['y_coord']:.2f}, {r['z_coord']:.2f})"


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def find(db, rid, name, xyz):
    """(row, how): the row by id, or by name and place in world 10; None and why not."""
    r = npc(db, rid)
    if r is not None and r["npc_name"] == name and r["world"] == WORLD:
        return r, ""
    alt = [a for a in rows(db, "SELECT * FROM npcs WHERE world = ? AND npc_name = ? AND abs(x_coord - ?) <= ? AND "
                               "abs(z_coord - ?) <= ?", (WORLD, name, xyz[0], TOL, xyz[2], TOL))]
    if len(alt) == 1:
        return alt[0], f" (row {alt[0]['id']} here)"
    if r is None:
        return None, "no such row"
    return None, f"the row is {r['npc_name']!r} in world {r['world']}, not as surveyed"


def plan(db):
    """(moves, removes, done, left)."""
    moves, removes, done, left = [], [], [], []
    for rid, name, frm, to, why in MOVES:
        r, how = find(db, rid, name, frm)
        if r is None:
            r, how2 = find(db, rid, name, to)
            if r is not None and at(r, to):
                done.append(f"{rid} {name}{how2}: at {where(r)} already")
            else:
                left.append(f"{rid} {name}: {how}")
        elif at(r, to):
            done.append(f"{rid} {name}{how}: at {where(r)} already")
        elif at(r, frm):
            moves.append((r, name, to, why))
        else:
            left.append(f"{rid} {name}{how}: at {where(r)}, moved since the survey (it stood at {frm}): left for a GM")
    for rid, name, frm, why in REMOVES:
        r, how = find(db, rid, name, frm)
        if r is None:
            (done if how == "no such row" else left).append(f"{rid} {name}: {how}")
        elif at(r, frm):
            removes.append((r, name, why))
        else:
            left.append(f"{rid} {name}{how}: at {where(r)}, moved since the survey (it stood at {frm}): left for a GM")
    return moves, removes, done, left


def keyed(db, rid):
    """The rows keyed "#n" by a row: its npctemplate and npcloot (table, key columns, row)."""
    out = []
    if table(db, "npctemplate"):
        out += [("npctemplate", ("name",), r) for r in rows(db, "SELECT * FROM npctemplate WHERE name = ?", (f"#{rid}",))]
    if table(db, "npcloot"):
        out += [("npcloot", ("npc_name", "tablename"), r)
                for r in rows(db, "SELECT * FROM npcloot WHERE npc_name = ?", (f"#{rid}",))]
    return out


def log(db, tbl, keys, before, after, why):
    db.execute(f"INSERT INTO {LOG} (tbl, keycols, before, after, why) VALUES (?, ?, ?, ?, ?)",
               (tbl, json.dumps(list(keys)), json.dumps(before, sort_keys=True) if before is not None else None,
                json.dumps(after, sort_keys=True) if after is not None else None, why))


def audit(db, action, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'npcs', ?, ?, ?, ?)",
               (now, acct[0] if acct else 0, WHO, VIA, action, go_json({"tool": "goldfeather_reach_20261123", "log": LOG}),
                go_json(before), go_json(after), detail))


def insert(db, tbl, r):
    cols = list(r)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])


def current(db, tbl, keys, r):
    got = rows(db, f"SELECT * FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [r[k] for k in keys])
    return got[0] if got else None


def do_apply(db, moves, removes):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (
        seq     INTEGER PRIMARY KEY AUTOINCREMENT,
        tbl     TEXT NOT NULL,
        keycols TEXT NOT NULL,          -- the key columns (JSON list)
        before  TEXT,                   -- the row before (JSON; NULL: none)
        after   TEXT,                   -- the row as written (JSON; NULL: deleted)
        why     TEXT NOT NULL DEFAULT ''
    )""")
    for before, name, to, why in moves:
        cur = db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ? WHERE id = ? AND abs(x_coord - ?) <= ? "
                         "AND abs(y_coord - ?) <= ? AND abs(z_coord - ?) <= ?",
                         (to[0], to[1], to[2], before["id"], before["x_coord"], TOL, before["y_coord"], TOL,
                          before["z_coord"], TOL))
        if cur.rowcount != 1:
            raise RuntimeError(f"{before['id']} changed under the transaction")
        log(db, "npcs", ("id",), before, npc(db, before["id"]), why)
    for before, name, why in removes:
        for tbl, keys, r in keyed(db, before["id"]):
            db.execute(f"DELETE FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [r[k] for k in keys])
            log(db, tbl, keys, r, None, why)
        db.execute("DELETE FROM npcs WHERE id = ?", (before["id"],))
        log(db, "npcs", ("id",), before, None, why)
    audit(db, "update", {"moved": 0, "removed": 0},
          {"moved": [m[0]["id"] for m in moves], "removed": [r[0]["id"] for r in removes]},
          f"{DETAIL}: {len(moves)} moved, {len(removes)} removed")


def do_undo(db):
    if not table(db, LOG):
        print("nothing to undo")
        return 0
    back = kept = 0
    for seq, tbl, keycols, before, after in db.execute(
            f"SELECT seq, tbl, keycols, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        keys = json.loads(keycols)
        was = json.loads(before) if before else None
        wrote = json.loads(after) if after else None
        ref = wrote if wrote is not None else was
        now = current(db, tbl, keys, ref)
        if wrote is not None and (now is None or json.dumps(now, sort_keys=True) != json.dumps(wrote, sort_keys=True)):
            kept += 1
            print(f"  left: {tbl} {[ref[k] for k in keys]} changed since it was written")
            continue
        if wrote is None and now is not None:
            kept += 1
            print(f"  left: {tbl} {[ref[k] for k in keys]} is there again (a row made since)")
            continue
        if wrote is not None:
            db.execute(f"DELETE FROM {tbl} WHERE {' AND '.join(f'{k} = ?' for k in keys)}", [ref[k] for k in keys])
        insert(db, tbl, was)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if kept == 0:
        db.execute(f"DROP TABLE {LOG}")
    audit(db, "update", {}, {"put_back": back, "left": kept}, "undo: " + DETAIL)
    print(f"put back {back} rows, {kept} left (changed since)")
    return 0 if kept == 0 else 1


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        for t in ("npcs", "gm_audit"):
            if not table(db, t):
                print(f"refused, nothing done: no {t} table here")
                return 1
        db.execute("BEGIN IMMEDIATE")
        try:
            if a.undo:
                rc = do_undo(db)
                db.execute("COMMIT")
                return rc
            moves, removes, done, left = plan(db)
            print(f"Goldfeather Eyrie (world {WORLD}): {len(moves)} to move, {len(removes)} to remove, {len(done)} as this tool "
                  f"has them already, {len(left)} left alone")
            for r, name, to, why in moves:
                print(f"  move   {r['id']} {name}: {where(r)} -> ({to[0]}, {to[1]}, {to[2]}): {why}")
            for r, name, why in removes:
                print(f"  remove {r['id']} {name} at {where(r)}: {why}")
            for d in done:
                print("  ok:    " + d)
            for x in left:
                print("  left:  " + x)
            if not moves and not removes:
                print("nothing to do" + (" (applied already)" if table(db, LOG) else ""))
                db.execute("ROLLBACK")
                return 0
            if not a.apply:
                print(f"report only: {len(moves)} would be moved, {len(removes)} removed (--apply)")
                db.execute("ROLLBACK")
                return 0
            do_apply(db, moves, removes)
            db.execute("COMMIT")
            print(f"applied: {len(moves)} moved, {len(removes)} removed, in gm_audit and {LOG}; the GM panel's "
                  "reload_npcs (or a restart) puts them there")
            return 0
        except BaseException:
            if db.in_transaction:
                db.execute("ROLLBACK")
            raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
