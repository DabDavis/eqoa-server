#!/usr/bin/env python3
"""The Plane of Disease's creatures of the water set on the slime's pools (the owner, 2026-11-14: "the Fish seem to
get out of the water very easy here like the environment boundaries didn't work very good over here"; Skywalker took
75 out by hand on 2026-11-13).

The Plane's slime is a floor: its pools' top is the terrain, flat at -49.94 (the river's at -34.06), and the zone
leaf flagged slime reaches 15-25 units over it and over the banks round it. A GM placing a fish where he stood put it
at his swimming height (CLIENT floats a body at the leaf's top, -36.4), over the slime; others were placed up the
banks. The server (water.go's slime surface rule, branch pod-water) now sets one placed over the pool on it and keeps
a creature of the water to its pool, one spawned up a bank coming down to it; this tool moves the rows themselves:

  over the slime  y set to the floor under it, x and z as they were (only npcmove water creatures: plague eels,
                  piranhas and jellies; the Live Server shard had none to move)
  up a bank       to the nearest point of its pool's flat top (median 11 units off, at most 32)

The plan (tools/pod_slime_spawns_20261114.json) was made on the ground files live uses (world 5's .ground and .env)
from both shards' snapshots of 2026-11-13 19:46/19:48: a row is moved only when it is still the plan's (its id, its
name and its position within 0.01 of the plan's); one deleted or moved since is left and said. Report only by
default; --apply in one transaction, a gm_audit line a row, log pod_slime_spawns_log; --undo exact. npcs rows are
read at the server's start: a restart places them.

    python3 -E -s tools/pod_slime_spawns_20261114.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "pod_slime_spawns_log"
DETAIL = "The Plane of Disease's creatures of the water set on the slime's pools: tools/pod_slime_spawns_20261114.py"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "pod_slime_spawns_20261114.json")


def near(row, p):
    return all(abs(row[c] - v) <= 0.01 for c, v in zip(("x_coord", "y_coord", "z_coord"), p))


def changes(db):
    plan = json.load(open(PLAN))
    out, lines = [], []
    done = todo = gone = moved = 0
    for p in plan:
        r = tr.rows(db, "SELECT id, npc_name, world, x_coord, y_coord, z_coord FROM npcs WHERE id = ?", (p["id"],))
        r = r[0] if r else None
        if r is None or r["npc_name"] != p["name"] or r["world"] != 5:
            gone += 1
            continue
        if near(r, p["to"]):
            done += 1
            continue
        if not near(r, p["from"]):
            moved += 1
            lines.append(f"left: npcs {p['id']} {p['name']} at ({r['x_coord']:.2f}, {r['y_coord']:.2f}, {r['z_coord']:.2f}), "
                         f"not the plan's ({p['from'][0]:.2f}, {p['from'][1]:.2f}, {p['from'][2]:.2f})")
            continue
        x, z = (r["x_coord"], r["z_coord"]) if p["moved"] == 0 else (p["to"][0], p["to"][2])
        out.append(("npcs", {"id": p["id"]}, {"x_coord": x, "y_coord": p["to"][1], "z_coord": z}))
        todo += 1
        lines.append(f"npcs {p['id']} {p['name']} {p['why']}: ({r['x_coord']:.2f}, {r['y_coord']:.2f}, {r['z_coord']:.2f})"
                     f" -> ({x:.2f}, {p['to'][1]:.2f}, {z:.2f}), {p['moved']:.0f} across")
    lines.append(f"{todo} to set on the slime, {done} there already, {moved} moved since the plan (left), "
                 f"{gone} not here (another shard's, or deleted)")
    return out, lines, [], todo == 0 and done > 0


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs",), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
