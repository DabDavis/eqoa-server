#!/usr/bin/env python3
"""Roamers on the froglok villages' catwalks, north of Grobb in Tunaria (the owner, 2026-11-20: "the Froglok camps
should have 1-3 Roamers walking the catwalks"). For the Live shard; the same villages are on Heirloom's map (its rows
are its own: the tool finds them by name and place, and refuses where they do not fit).

The villages stand on stilts over the swamp: hut platforms joined by plank walkways about 4.5 wide, 13-14 over the
water (the terrain at 48.2-48.8, the planks at 61.7-62.2), placed floors of the zone's collision (worlds/ground/0.walls),
not the terrain. Three villages have them, each its own common froglok on its planks:
    A  the villagers' village (x 22615-22660, z 30174-30274), one walkway network of 1,100 square units: 2 roamers
    B  the guards' village (x 24990-25241, z 30312-30606), four separate clusters of 1,200, 945, 508 and 340: 3
       roamers, one on each of the three largest
    C  the raiders' village (x 24780-24943, z 30174-30248), two clusters of 936 and 342: 2 roamers
(the warriors' village at x 23525-23665, z 29500-29710, level 23-25, has catwalks too and is left out: not one the
owner named).

Each roamer is a new npcs row copied whole (every column but id, as the GM panel's clone_row) from its village's own
common row nearest its path (froglok villager, froglok guard, froglok raider: its level, body, gear, and its loot,
faction, class and template, which go by name), standing at its path's first point facing the second, with that row's
npcrespawn copied, and an npcpaths row: walked back and forth (pingpong) at the walk's pace, standing END_PAUSE seconds
at either end. Its points were laid with the server's own collision (internal/ground's Stand, Placed and Wall over
the baked 0.ground and 0.walls): every point on the planks; every straight leg between two stays on them, sampled each
0.25, a floor within 0.6 of the last (no step off an edge, no box or post climbed), the deck there 1.25 out to either
side at least, no wall or placed thing across it from the knee to the head; and no NPC standing within 2 of it when it
was laid (a fresh copy of Live, 04:36 UTC 2026-11-20). A creature spawned on a placed floor walks on the floor its
step finds (entity/groundnpc.go: the highest within a step over its height), so it keeps to the planks.

Refused (nothing written) where the plan no longer fits the database: a village's template row is gone or off its
planks, its common row has fewer than MIN_KIND rows on the planks there, or an NPC now stands within CLEAR of a path's
leg at the planks' height (a GM's edit since the plan; named, so the path can be laid again round it).

Dry run by default: the plan, the templates found and what would be written. --apply writes it in one transaction,
a gm_audit line a row, logged in frog_catwalks_20261120_log (+ _meta: the npcs sequence before). A second --apply
writes nothing. --undo deletes exactly the logged rows (npcs, npcrespawn, and the npcpaths rows of those NPCs,
whichever wrote them), puts the npcs sequence back and drops the log: the three tables come back as they were
(--hash prints their digests); a row changed since the apply is still taken out, and said so.

--paths db (the default) writes the npcpaths rows too: right for a restart (a keep swap's before.sh). The running
world reads paths only at its start or from the GM panel's path editor, so for a world that keeps running use
--paths panel --ops FILE: the npcpaths rows are not written, and FILE gets one set_npc_path call a roamer (its id and
points, in the order to send them), each of which writes the row (audited) and gives the NPC its path at once.

    python3 -E -s tools/froglok_catwalks_20261120.py --db eqoa.db [--apply | --undo | --hash]
        [--paths db|panel] [--ops FILE]
"""
import argparse
import datetime
import hashlib
import json
import math
import sqlite3
import sys

TOOL = "tools/froglok_catwalks_20261120.py"
LOG, META = "frog_catwalks_20261120_log", "frog_catwalks_20261120_meta"
WHO, VIA = "claude", "tool"
DETAIL = "froglok catwalk roamers (the owner, 2026-11-20): " + TOOL
WORLD = 0
END_PAUSE = 5.0   # seconds a roamer stands at either end of its path
TEMPLATE_DY = 1.0  # the template row stands on the planks: within this of their height
TEMPLATE_R = 60.0  # and this near its path's first point
KIND_R = 150.0    # a village's common row counted this near the path
MIN_KIND = 3      # rows of the common name on the planks there, at least
CLEAR = 1.5       # no NPC this near a path's leg
CLEAR_DY = 3.0    # at the planks' height (within this)

# key, village, the common row's name, the planks' height, the points (x, y, z) in the order walked: laid with the
# server's ground (internal/ground) as the docstring says
ROAMERS = [
    ("A1", "A", "froglok villager", 62.2, [
        (22650.25, 62.19, 30190.00),
        (22651.25, 62.19, 30192.00),
        (22651.25, 62.19, 30212.50),
        (22647.75, 62.19, 30219.50),
        (22646.75, 62.19, 30220.50),
        (22630.25, 62.09, 30222.00),
    ]),
    ("A2", "A", "froglok villager", 62.2, [
        (22619.25, 62.19, 30257.00),
        (22620.75, 62.19, 30252.50),
        (22620.25, 62.19, 30227.50),
        (22618.75, 62.19, 30226.00),
        (22618.75, 62.19, 30222.00),
    ]),
    ("B1", "B", "froglok guard", 61.9, [
        (25185.25, 61.89, 30377.00),
        (25185.75, 61.94, 30360.50),
        (25189.75, 61.94, 30357.00),
        (25194.25, 61.89, 30355.50),
        (25214.25, 61.94, 30354.00),
        (25218.25, 61.94, 30350.00),
        (25219.25, 61.94, 30350.00),
        (25221.25, 61.94, 30352.00),
        (25222.25, 61.94, 30355.00),
        (25223.75, 61.94, 30381.00),
        (25224.75, 61.94, 30384.50),
        (25226.75, 61.94, 30386.50),
        (25228.25, 61.94, 30390.00),
    ]),
    ("B2", "B", "froglok guard", 61.9, [
        (25105.25, 61.94, 30547.00),
        (25099.25, 61.89, 30559.50),
        (25099.25, 61.94, 30576.00),
        (25098.25, 61.94, 30579.00),
        (25094.75, 61.89, 30582.00),
        (25077.75, 61.89, 30582.00),
        (25073.25, 61.94, 30584.00),
        (25069.25, 61.94, 30587.00),
        (25069.25, 61.89, 30603.00),
    ]),
    ("B3", "B", "froglok guard", 61.9, [
        (25008.00, 61.89, 30493.00),
        (25020.00, 61.94, 30506.50),
        (25020.00, 61.94, 30508.00),
        (25018.00, 61.94, 30515.50),
        (25004.00, 61.94, 30529.50),
        (24994.50, 61.94, 30533.50),
    ]),
    ("C1", "C", "froglok raider", 61.9, [
        (24781.75, 61.89, 30188.00),
        (24799.75, 61.94, 30189.00),
        (24806.25, 61.89, 30200.00),
        (24806.25, 61.89, 30213.00),
        (24806.75, 61.94, 30217.50),
        (24808.25, 61.94, 30218.50),
        (24812.25, 61.89, 30219.50),
        (24832.25, 61.94, 30221.00),
        (24833.25, 61.94, 30222.00),
        (24836.25, 61.94, 30224.00),
        (24837.75, 61.94, 30224.00),
        (24842.75, 61.94, 30221.00),
        (24843.25, 61.71, 30221.00),
    ]),
    ("C2", "C", "froglok raider", 61.9, [
        (24914.25, 61.89, 30238.00),
        (24937.75, 61.94, 30239.00),
        (24938.75, 61.94, 30240.00),
        (24939.75, 61.94, 30242.00),
        (24940.25, 61.94, 30245.50),
    ]),
]

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    npc_row      INTEGER PRIMARY KEY,  -- the npcs row written
    roamer       TEXT NOT NULL,        -- its key in the plan (A1, B2...)
    template_row INTEGER NOT NULL,     -- the row copied
    respawn      INTEGER NOT NULL,     -- 1: an npcrespawn row was written for it
    path         INTEGER NOT NULL,     -- 1: its npcpaths row was written by the tool (0: by the GM panel)
    row          TEXT NOT NULL,        -- the npcs row as written (JSON)
    path_json    TEXT NOT NULL         -- its path (the npcpaths columns, JSON)
)"""
META_DDL = f"CREATE TABLE IF NOT EXISTS {META} (key TEXT PRIMARY KEY, value TEXT NOT NULL)"


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail=DETAIL):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                None if before is None else go_json(before), None if after is None else go_json(after), detail))


def seg_dist(px, pz, a, b):
    dx, dz = b[0] - a[0], b[2] - a[2]
    l2 = dx * dx + dz * dz
    t = 0.0 if l2 == 0 else max(0.0, min(1.0, ((px - a[0]) * dx + (pz - a[2]) * dz) / l2))
    return math.hypot(px - a[0] - t * dx, pz - a[2] - t * dz)


def facing_to(x, z, tx, tz):
    """The facing byte of a look from (x, z) toward (tx, tz) (CLIENT's heading atan2(dx, dz), as the GM tools'
    facing)."""
    h = math.atan2(tx - x, tz - z)
    if h < 0:
        h += 2 * math.pi
    return int(round(h / (2 * math.pi) * 256)) % 256


def path_points(pts):
    out = []
    for i, (x, y, z) in enumerate(pts):
        p = {"x": x, "y": y, "z": z}
        if i in (0, len(pts) - 1):
            p["pause"] = END_PAUSE
        out.append(p)
    return out


def npc_seq(db):
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return None if r is None else r[0]


def plan(db):
    """The roamers as the database has them now: each its template row and its new row and path; and the
    refusals."""
    db.row_factory = sqlite3.Row
    rows = [dict(r) for r in db.execute(
        "SELECT * FROM npcs WHERE world = ? AND x_coord BETWEEN 22000 AND 26000 AND z_coord BETWEEN 29800 AND 30800",
        (WORLD,))]
    db.row_factory = None
    pathed = {r[0] for r in db.execute("SELECT npcid FROM npcpaths")} if table(db, "npcpaths") else set()
    respawn = {r[0]: r for r in db.execute("SELECT npc_row, seconds, note, source FROM npcrespawn")} \
        if table(db, "npcrespawn") else {}
    out, refusals = [], []
    for key, village, name, deck, pts in ROAMERS:
        first = pts[0]
        on = [r for r in rows if r["npc_name"] == name and abs((r["y_coord"] or 0) - deck) < TEMPLATE_DY and
              math.hypot(r["x_coord"] - first[0], r["z_coord"] - first[2]) <= KIND_R]
        if len(on) < MIN_KIND:
            refusals.append(f"{key}: {len(on)} {name} rows on the planks within {KIND_R:.0f} of "
                            f"({first[0]:.0f}, {first[2]:.0f}), want {MIN_KIND}: the village is not as planned")
            continue
        cands = sorted((r for r in on if r["id"] not in pathed and
                        math.hypot(r["x_coord"] - first[0], r["z_coord"] - first[2]) <= TEMPLATE_R),
                       key=lambda r: (math.hypot(r["x_coord"] - first[0], r["z_coord"] - first[2]), r["id"]))
        if not cands:
            refusals.append(f"{key}: no {name} row on the planks within {TEMPLATE_R:.0f} of its path's start")
            continue
        t = cands[0]
        for r in rows:
            if abs((r["y_coord"] or 0) - deck) >= CLEAR_DY:
                continue
            d = min(seg_dist(r["x_coord"], r["z_coord"], pts[i], pts[i + 1]) for i in range(len(pts) - 1))
            if d < CLEAR:
                refusals.append(f"{key}: row {r['id']} ({r['npc_name']}) stands {d:.2f} from its path at "
                                f"({r['x_coord']:.1f}, {r['y_coord']:.1f}, {r['z_coord']:.1f}): lay the path again")
        new = dict(t)
        new.pop("id")
        new.update(x_coord=first[0], y_coord=first[1], z_coord=first[2],
                   facing=facing_to(first[0], first[2], pts[1][0], pts[1][2]), dt_kill=None)
        path = {"world": WORLD, "mode": "pingpong", "pause": 0.0, "speed": "walk", "points": path_points(pts)}
        out.append(dict(key=key, village=village, template=t, row=new, path=path, respawn=respawn.get(t["id"]),
                        length=sum(math.hypot(pts[i + 1][0] - pts[i][0], pts[i + 1][2] - pts[i][2])
                                   for i in range(len(pts) - 1))))
    return out, refusals


def applied(db):
    return table(db, LOG) and db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0] > 0


def apply(db, roamers, by_db):
    db.execute(LOG_DDL)
    db.execute(META_DDL)
    db.execute(f"INSERT OR REPLACE INTO {META} VALUES ('seq_before', ?)", (json.dumps(npc_seq(db)),))
    cols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    for a in roamers:
        row = {k: v for k, v in a["row"].items() if k in cols}
        names = list(row)
        cur = db.execute(f"INSERT INTO npcs ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})",
                         [row[k] for k in names])
        rid = cur.lastrowid
        a["id"] = rid
        written = dict(row, id=rid)
        audit(db, "insert", "npcs", {"id": rid}, None, written,
              f"{DETAIL}: roamer {a['key']}, a copy of row {a['template']['id']} ({a['template']['npc_name']})")
        resp = a["respawn"]
        if resp is not None:
            r = {"npc_row": rid, "seconds": resp[1], "note": f"as row {a['template']['id']}'s (its village's); {TOOL}",
                 "source": resp[3] or "tool"}
            db.execute("INSERT INTO npcrespawn (npc_row, seconds, note, source) VALUES (?, ?, ?, ?)",
                       (r["npc_row"], r["seconds"], r["note"], r["source"]))
            audit(db, "insert", "npcrespawn", {"npc_row": rid}, None, r)
        p = a["path"]
        pj = json.dumps(p["points"], separators=(",", ":"))
        if by_db:
            db.execute("INSERT INTO npcpaths (npcid, world, mode, pause, speed, points) VALUES (?, ?, ?, ?, ?, ?)",
                       (rid, p["world"], p["mode"], p["pause"], p["speed"], pj))
            audit(db, "insert", "npcpaths", {"npcid": rid}, None,
                  {"npcid": rid, "world": p["world"], "mode": p["mode"], "pause": p["pause"], "speed": p["speed"],
                   "points": pj})
        db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (rid, a["key"], a["template"]["id"], 1 if resp is not None else 0, 1 if by_db else 0,
                    json.dumps(written, sort_keys=True, default=str), json.dumps(p, sort_keys=True)))


def undo(db):
    if not table(db, LOG):
        return 0, []
    changed = []
    logged = list(db.execute(f"SELECT npc_row, respawn, row FROM {LOG} ORDER BY npc_row"))
    db.row_factory = sqlite3.Row
    for rid, _, written in logged:
        r = db.execute("SELECT * FROM npcs WHERE id = ?", (rid,)).fetchone()
        if r is not None and json.dumps(dict(r), sort_keys=True, default=str) != written:
            changed.append(rid)
    db.row_factory = None
    for rid, resp, _ in logged:
        cur = db.execute("SELECT * FROM npcs WHERE id = ?", (rid,))
        r = cur.fetchone()
        if r is not None:
            db.execute("DELETE FROM npcs WHERE id = ?", (rid,))
            audit(db, "delete", "npcs", {"id": rid}, dict(zip([d[0] for d in cur.description], r)), None,
                  "undo: " + DETAIL)
        if resp:
            cur = db.execute("SELECT * FROM npcrespawn WHERE npc_row = ?", (rid,))
            r = cur.fetchone()
            if r is not None:
                db.execute("DELETE FROM npcrespawn WHERE npc_row = ?", (rid,))
                audit(db, "delete", "npcrespawn", {"npc_row": rid}, dict(zip([d[0] for d in cur.description], r)),
                      None, "undo: " + DETAIL)
        # its path, whoever wrote it (the tool, or the GM panel's set_npc_path with --paths panel)
        cur = db.execute("SELECT * FROM npcpaths WHERE npcid = ?", (rid,))
        r = cur.fetchone()
        if r is not None:
            db.execute("DELETE FROM npcpaths WHERE npcid = ?", (rid,))
            audit(db, "delete", "npcpaths", {"npcid": rid}, dict(zip([d[0] for d in cur.description], r)), None,
                  "undo: " + DETAIL)
    seq = json.loads(db.execute(f"SELECT value FROM {META} WHERE key = 'seq_before'").fetchone()[0])
    top = db.execute("SELECT max(id) FROM npcs").fetchone()[0] or 0
    if seq is None:
        if top == 0:
            db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
    elif top <= seq:
        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (seq,))
    db.execute(f"DROP TABLE {LOG}")
    db.execute(f"DROP TABLE {META}")
    return len(logged), changed


def digests(db):
    out = {}
    for t, k in (("npcs", "id"), ("npcrespawn", "npc_row"), ("npcpaths", "npcid")):
        h = hashlib.sha256()
        for r in db.execute(f"SELECT * FROM {t} ORDER BY {k}"):
            h.update(repr(r).encode())
        out[t] = h.hexdigest()[:16]
    out["npcs_seq"] = npc_seq(db)
    return out


def ops_of(roamers):
    return [{"op": "set_npc_path", "args": {
        "id": a["id"], "mode": a["path"]["mode"], "pause": a["path"]["pause"], "speed": a["path"]["speed"],
        "points": a["path"]["points"]}, "npc": a["row"]["npc_name"], "roamer": a["key"]} for a in roamers]


def report(roamers):
    for a in roamers:
        t, p = a["template"], a["path"]["points"]
        resp = f"respawn {a['respawn'][1]:g} s" if a["respawn"] else "no respawn row"
        print(f"  {a['key']} village {a['village']}: {t['npc_name']} L{t['npc_level']}, a copy of row {t['id']} at "
              f"({t['x_coord']:.1f}, {t['y_coord']:.1f}, {t['z_coord']:.1f}); {resp}; pingpong at the walk, "
              f"{len(p)} points, {a['length']:.1f} long, {END_PAUSE:g} s at either end")
        print("      " + " ".join(f"({q['x']:.2f}, {q['y']:.2f}, {q['z']:.2f})" for q in p))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--hash", action="store_true")
    ap.add_argument("--paths", choices=("db", "panel"), default="db")
    ap.add_argument("--ops", help="with --paths panel: write the set_npc_path calls here (JSON)")
    args = ap.parse_args()
    if args.paths == "panel" and args.apply and not args.ops:
        ap.error("--paths panel needs --ops FILE")
    db = sqlite3.connect(args.db)
    db.isolation_level = None
    if args.hash:
        print(json.dumps(digests(db), sort_keys=True))
        return 0
    if args.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            n, changed = undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"undone: {n} roamer(s) taken out" + (f"; changed since the apply (still taken out): {changed}"
                                                    if changed else ""))
        return 0
    if applied(db):
        print(f"already applied ({LOG} has rows): nothing to do; --undo first to lay them again")
        return 0
    roamers, refusals = plan(db)
    print(f"froglok catwalk roamers: {len(roamers)} of {len(ROAMERS)} planned")
    report(roamers)
    if refusals:
        print("REFUSED, nothing written:")
        for r in refusals:
            print("  " + r)
        return 2
    if not args.apply:
        print("dry run: --apply to write")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        apply(db, roamers, args.paths == "db")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    ids = ", ".join(f"{a['key']}={a['id']}" for a in roamers)
    print(f"applied: {len(roamers)} npcs rows ({ids}), {sum(1 for a in roamers if a['respawn'])} npcrespawn rows, "
          + (f"{len(roamers)} npcpaths rows" if args.paths == "db" else "no npcpaths rows (the panel's set_npc_path)"))
    if args.ops:
        with open(args.ops, "w") as f:
            json.dump(ops_of(roamers), f, indent=1)
        print(f"set_npc_path calls: {args.ops}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
