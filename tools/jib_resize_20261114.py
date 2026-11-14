#!/usr/bin/env python3
"""The GM Jibrish's tour of Tunaria's north-west (2026-11-14, filed on the Live Server; the owner: Jib's Live changes
go on Heirloom too): creature sizes, the bearweres' roam, and the import's unmoving rows spread to roam their zone.

A zone is CLIENT's (sZones0: a 2,000-unit cell, x/2000 and z/2000; its name WORDS.TXT's): Permafrost (2,2) x 4000-6000
z 4000-6000, Zentar's Keep (2,3) x 4000-6000 z 6000-8000, Snowblind Plains (3,2) x 6000-8000 z 4000-6000, Unkempt
North (3,3) x 6000-8000 z 6000-8000, Anu Village (4,2) x 8000-10000 z 4000-6000, North Wilderlands (4,3) x 8000-10000 z 6000-8000. "This area" is every row of the creature's name in the zone of the target's spawn (the nearest row of its name): those
creatures are spread evenly over their zones (cmd/wildspread), none a camp across a zone's edge (a row across the edge
is as far from the zone's as the zone's are from one another), so no neighbouring zone's are taken.

  ticket  creature         zone            size
  5036    great worm       Permafrost      x1.75
  5037    Glacier          Permafrost      x2.2
  5038    snow drake       Permafrost      x1.35 (all at the target's new size: "the same size template")
  5039    mammoth          Permafrost      x1.35
  5054    ancient worm     Permafrost      x1.35
  5056    ice giant        Permafrost      x1.2
  5058    ancient mammoth  Permafrost      x1.15
  5059    avalanche worm   Snowblind Pl.   x1.2
  5060    The Unnamed King Anu Village     x1.35 (he alone)
  5041    polar bear       Zentar's Keep   x1.25
  5042    savage bear      Zentar's Keep   x1.15
  5043    black greatbear  Zentar's Keep   x1.75
  5044    green goo        Zentar's Keep   x1.2
  5046    ebon great bear  Unkempt North   x1.75
  5048    snow drake       Unkempt North   x1.35
  5049    jade drake       Unkempt North   x2
  5051    savage bear      N. Wilderlands  x1.35 (the target, at x 7999.0, had wandered off its spawn across the edge)

5047 ("Bearwears in this area need a large roaming distance, about 1/5th of the zone in size as a radius but careful
not to go too far into other zones"): every bearwere of Unkempt North gets its own wander row (npcwander) of radius
400 (a fifth of the zone's 2,000), cut to its spawn's distance from the zone's edge (100 at least), so its roam keeps
inside its zone.

5050, 5052, 5053, 5061-5064 ("a static pile of non roaming NPC's ... They need to roam throughout the zone", "a pack of unroaming
mobs in a row", "unmoving mobs in a row"; 5061-5064 name the treants and bearweres of the line at (7073, 7037)), and the same import's rows found in the zones the GM toured (Unkempt North:
two more lines, at (7540, 6585) and (7086, 7703)): the plan tools/jib_resize_20261114_plan.json
(cmd/jibspread, made from the Live Server's database and the baked ground) puts each creature of them on a spot of its
zone's walkable ground (reached from the pile by steps its wander could walk, no wall, fall or water, not in a town,
100 or more inside the zone's edge, spread farthest first from every other creature) and gives it a wander row of
radius 300, cut to its distance from the zone's edge (100 at least). Mirin (named) stays. A plan row is found by its
id and name, where the plan found it or within 150 of it (Heirloom's disperse tool spread some of these piles a
little, 2026-11-12); one not found is left and said. The bearweres among them take 5047's radius, ebon bears and jade
drakes the sizes.

Report only by default; --apply in one transaction with a gm_audit line a row and the log jib_resize_1114_log; a second
--apply does nothing; --undo exact (a row changed since is left and said). NPC rows and npcwander are read at the
start: Heirloom's reload_npcs, or a restart (the Live Server).

    python3 -E -s tools/jib_resize_20261114.py --db eqoa.db [--apply | --undo]
"""
import json
import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "jib_resize_1114_log"
DETAIL = "the GM Jibrish's tour of Tunaria's north-west, petitions 5036-5064 (sizes, bearwere roam, unmoving rows " \
         "spread to roam their zone): tools/jib_resize_20261114.py"
SOURCE = "jibresize"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "jib_resize_20261114_plan.json")
ZONE = 2000.0
ZONES = {(2, 2): "Permafrost", (2, 3): "Zentar's Keep", (3, 2): "Snowblind Plains", (3, 3): "Unkempt North",
         (4, 2): "Anu Village", (4, 3): "North Wilderlands"}

# (ticket, name, target x, target z, factor, one size for all)
SIZES = [
    (5036, "great worm", 5668.7, 5460.8, 1.75, False),
    (5037, "Glacier", 5589.8, 5522.6, 2.2, False),
    (5038, "snow drake", 5541.7, 5639.8, 1.35, True),
    (5039, "mammoth", 5238.8, 5952.2, 1.35, False),
    (5054, "ancient worm", 4427.1, 5391.0, 1.35, False),
    (5056, "ice giant", 4471.9, 5178.0, 1.2, False),
    (5058, "ancient mammoth", 4302.6, 4863.0, 1.15, False),
    (5059, "avalanche worm", 6915.6, 4368.6, 1.2, False),
    (5060, "The Unnamed King", 8916.6, 4113.9, 1.35, False),
    (5041, "polar bear", 4875.6, 6258.1, 1.25, False),
    (5042, "savage bear", 5324.4, 6858.0, 1.15, False),
    (5043, "black greatbear", 5284.4, 7014.2, 1.75, False),
    (5044, "green goo", 5556.4, 7278.2, 1.2, False),
    (5046, "ebon great bear", 6299.3, 7299.0, 1.75, False),
    (5048, "snow drake", 6741.1, 7375.4, 1.35, True),
    (5049, "jade drake", 6524.6, 7599.4, 2.0, False),
    (5051, "savage bear", 7999.0, 7399.8, 1.35, False),
]
BEARWERE = (5047, "bearwere", 6630.0, 7230.0, 400.0)
ROAM_LEAST = 100.0
NEAR_PLAN = 150.0  # a plan row this near where the plan found it is its


def cell(x, z):
    return int(math.floor(x / ZONE)), int(math.floor(z / ZONE))


def edge(x, z):
    cx, cz = cell(x, z)
    return min(x - cx * ZONE, (cx + 1) * ZONE - x, z - cz * ZONE, (cz + 1) * ZONE - z)


def applied_already(db):
    """True when the log is there and every row it wrote is still as written; None when there is no log."""
    if not tr.table(db, LOG):
        return None
    for tbl, rowkey, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG}").fetchall():
        now = tr.get(db, tbl, json.loads(rowkey))
        if (now is None) != (after is None) or (now is not None and json.dumps(now, sort_keys=True) != after):
            return False
    return True


def changes(db):
    lines, refusals = [], []
    done = applied_already(db)
    if done:
        return [], lines, [], True
    if done is False:
        return [], lines, [f"{LOG} is there but its rows changed since: --undo first"], False
    plan = json.load(open(PLAN))
    npc = {}  # id -> the row's new columns
    wander = {}  # id -> (radius, note)
    where = {}  # id -> (x, z) after the moves

    # the spread
    for m in plan["moves"]:
        r = db.execute("SELECT npc_name, world, x_coord, y_coord, z_coord FROM npcs WHERE id = ?", (m["id"],)).fetchone()
        bx, by, bz = m["before"]
        if r is None or r[0] != m["name"] or r[1] != 0:
            lines.append(f"  {m['ticket']}: npcs {m['id']} {m['name']}: not on this shard, left")
            continue
        d = math.hypot(r[2] - bx, r[4] - bz)
        if d > NEAR_PLAN:
            lines.append(f"  {m['ticket']}: npcs {m['id']} {m['name']}: at ({r[2]:.1f}, {r[4]:.1f}), {d:.0f} from where "
                         f"the plan found it, left")
            continue
        ax, ay, az, af = m["after"]
        npc[m["id"]] = {"x_coord": ax, "y_coord": ay, "z_coord": az, "facing": af}
        wander[m["id"]] = (m["radius"], f"petition {m['ticket']}: roams {ZONES[cell(ax, az)]} (spread from "
                                        f"({bx:.0f}, {bz:.0f}))")
        where[m["id"]] = (ax, az)
    by_ticket = {}
    for m in plan["moves"]:
        if m["id"] in npc:
            by_ticket[m["ticket"]] = by_ticket.get(m["ticket"], 0) + 1
    for t, n in by_ticket.items():
        lines.append(f"{t}: {n} rows spread over {ZONES[(3, 3)]}, each roaming its ground")

    def rows_of(name, zc):
        out = []
        for rid, x, z, size in db.execute("SELECT id, x_coord, z_coord, size FROM npcs WHERE world = 0 AND npc_name = ?",
                                          (name,)):
            x, z = where.get(rid, (x, z))
            if cell(x, z) == zc:
                out.append((rid, x, z, size))
        return out

    def target_zone(name, tx, tz):
        """The zone of the target's spawn: the nearest row of its name (a creature wanders off its spawn, and 5051's
        target, at x 7999.0, was North Wilderlands' savage bear 65 from its spawn across the edge)."""
        best = None
        for rid, x, z in db.execute("SELECT id, x_coord, z_coord FROM npcs WHERE world = 0 AND npc_name = ?", (name,)):
            x, z = where.get(rid, (x, z))
            d = math.hypot(x - tx, z - tz)
            if best is None or d < best[0]:
                best = (d, cell(x, z))
        return best[1] if best else None

    # the sizes
    for ticket, name, tx, tz, f, same in SIZES:
        zc = target_zone(name, tx, tz)
        rs = rows_of(name, zc) if zc else []
        if not rs:
            refusals.append(f"{ticket}: no {name!r} near ({tx}, {tz})")
            continue
        tgt = min(rs, key=lambda r: math.hypot(r[1] - tx, r[2] - tz))
        sizes = {}
        for rid, x, z, size in rs:
            sizes[rid] = round((tgt[3] if same else size) * f, 4)
            npc.setdefault(rid, {})["size"] = sizes[rid]
        olds = sorted({r[3] for r in rs})
        news = sorted(set(sizes.values()))
        lines.append(f"{ticket}: {len(rs)} {name} of {ZONES[zc]} (the target's spawn npcs {tgt[0]}, "
                     f"{math.hypot(tgt[1] - tx, tgt[2] - tz):.0f} from it), size {'/'.join(f'{s:g}' for s in olds)} -> "
                     f"{'/'.join(f'{s:g}' for s in news)}: {', '.join(str(r[0]) for r in sorted(rs))}")

    # 5047: the bearweres' roam
    ticket, name, tx, tz, most = BEARWERE
    zc = cell(tx, tz)
    rs = rows_of(name, zc)
    radii = []
    for rid, x, z, _ in rs:
        rad = float(round(max(ROAM_LEAST, min(most, edge(x, z)))))
        wander[rid] = (rad, f"petition {ticket}: a bearwere's wide roam, a fifth of the zone, inside {ZONES[zc]}")
        radii.append(rad)
    lines.append(f"{ticket}: {len(rs)} {name} of {ZONES[zc]} roam radius {'/'.join(f'{r:g}' for r in sorted(set(radii)))}: "
                 f"{', '.join(str(r[0]) for r in sorted(rs))}")

    out = []
    for rid in sorted(npc):
        cur = tr.get(db, "npcs", {"id": rid})
        if any(cur[k] != v for k, v in npc[rid].items()):
            out.append(("npcs", {"id": rid}, npc[rid]))
    for rid in sorted(wander):
        rad, note = wander[rid]
        out.append(("npcwander", {"npcid": rid}, {"npcid": rid, "radius": rad, "note": note, "source": SOURCE}))
    if not tr.table(db, "npcwander"):
        refusals.append("no npcwander table (a server older than npcwander)")
    return out, lines, refusals, False


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcwander"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
