#!/usr/bin/env python3
"""Runnyeye's creatures out of its fires and its rock (the owner, 2026-11-20: "runnyeye has messed up mob placements
like fire pits etc"); both shards, the open-world citadel in Tunaria (world 0) and Heirloom's instanced copy (world 14,
tools/dungeons_20261112.py: its rows are templates at the same places).

Found by internal/entity's TestRealSpawnPlaces (spawnplaces_real_test.go) over every npcs row in x 15,900-18,100,
z 11,800-14,100 of worlds 0 and 14 (Tunaria's zone 67 and the gate's strip of 54; world 14's Runnyeye boxes lie
inside it): the floor the server sets each on (internal/ground's Stand), the movement check's ball (Embedded), a floor
over it, the ground over it, the disc's fires (TUNARIA.ESF's zone actors that are a collision sprite with a particle
sprite: the citadel's fire pits 0xBDB6EBB2 3.5 across, braziers 0x991646E4, candle stands 0x47935FF3), the wander's
own leg check in 16 ways and a walk of the floors from it. Of 675 rows on Heirloom and 477 on Live, these:

  in a fire pit's ring (its middle within 2.4 of the pit's, the ring 1.8 out):
    goblin defender  (16410.3, 73.6, 12260.9)   2.2 from the pit at (16412.4, 73.7, 12260.2)   world 0 and 14
    goblin sneak     (16412.9, 73.8, 12258.2)   2.1 from the same pit                          world 0 and 14
    goblin lookout   (16328.9, 85.0, 12046.4)   2.3 from the north gate's pit (16326.9, 85.0, 12045.3)  world 0 and 14
    goblin worg rider(16302.4, 92.0, 11814.1)   2.3 from the pit at (16304.3, 92.0, 11812.9)   world 0 (outside world 14's box)
  standing on a brazier or a candle stand (put there 2026-11-18 by tools/xp_camps_20261118.py: the cutthroat's "in
  something placed" spot freed onto the brazier's top, the excavator's copy on a candle stand's foot):
    goblin cutthroat (16324.5, 51.0, 12211.0)   on the brazier at (16324.2, 50.0, 12211.4), 0.5 off   world 0
                     (16325.0, 50.03, 12211.0)  in it, 0.9 off                                      world 14
    goblin excavator (16291.0, 39.0, 12159.0)   on the candle stand at (16291.5, 38.0, 12159.7)        world 0 and 14
  no leg out (all 16 of the wander's legs refused: pressed against the north gate pit's rim and the gate):
    goblin novice    (16324.5, 85.0, 12044.4)   world 0 and 14
  in something placed (the alcove's rock; the movement check's ball in it):
    goblin prisoner  (16244.724, 35.7812, 12189.493)   world 0
  in the terrain: no floor under it, the terrain 4.9 over it (a steep face the walk does not stand on):
    superior head jailor (17090, 5, 13246)   world 0 (a GM session's move of 2026-11-17)

Each goes to the nearest place by the same checks (spawnplaces_real_test.go's freeSpotLocked): on its own floor's
level, nothing placed in the body, no floor over it, 2 clear of every fire's edge, no hazard, 6 of the 16 legs out (a
creature talked to, which never fights: none asked), a walk from it getting 20 out, no wall between it and where it
stood: 1 to 3.5 off, so every camp stays where it was. The facing is kept. The checker finds none bad after.

Report only by default; --apply in one transaction, a gm_audit line a row, log runnyeye_placements_20261120_log;
--undo exact. A row is found by its name, world and place (within 0.01); one not there and not at its new place
either is refused (a GM moved it: nothing done). A shard without world 14's rows (Live) skips the world-14 lines.
npcs rows are read at the server's start: apply in a keep restart's before.sh.

    python3 -E -s tools/runnyeye_placements_20261120.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "runnyeye_placements_20261120_log"
DETAIL = "Runnyeye's creatures out of its fires and rock (the owner, 2026-11-20): tools/runnyeye_placements_20261120.py"

# (why, name, world, (x, y, z) as found, (x, y, z) to)
MOVES = (
    ("in the fire pit at (16412.4, 12260.2)", "goblin defender", 0, (16410.3, 73.6, 12260.9), (16408.91, 73.74, 12258.82)),
    ("in the fire pit at (16412.4, 12260.2)", "goblin defender", 14, (16410.3, 73.6, 12260.9), (16408.91, 73.74, 12258.82)),
    ("in the fire pit at (16412.4, 12260.2)", "goblin sneak", 0, (16412.9, 73.8, 12258.2), (16410.41, 73.91, 12256.53)),
    ("in the fire pit at (16412.4, 12260.2)", "goblin sneak", 14, (16412.9, 73.8, 12258.2), (16410.41, 73.91, 12256.53)),
    ("in the north gate's fire pit", "goblin lookout", 0, (16328.9, 85.0, 12046.4), (16331.39, 85.03, 12048.07)),
    ("in the north gate's fire pit", "goblin lookout", 14, (16328.9, 85.0, 12046.4), (16331.39, 85.03, 12048.07)),
    ("no leg out by the north gate's fire pit", "goblin novice", 0, (16324.5, 85.0, 12044.4), (16321.27, 85.03, 12043.06)),
    ("no leg out by the north gate's fire pit", "goblin novice", 14, (16324.5, 85.0, 12044.4), (16321.27, 85.03, 12043.06)),
    ("in the fire pit at (16304.3, 11812.9)", "goblin worg rider", 0, (16302.4, 92.0, 11814.1), (16303.55, 92.08, 11816.87)),
    ("on the brazier at (16324.2, 12211.4)", "goblin cutthroat", 0, (16324.5, 51.0, 12211.0), (16326.81, 50.03, 12211.96)),
    ("in the brazier at (16324.2, 12211.4)", "goblin cutthroat", 14, (16325.0, 50.03, 12211.0), (16326.85, 50.03, 12211.77)),
    ("on the candle stand at (16291.5, 12159.7)", "goblin excavator", 0, (16291.0, 39.0, 12159.0), (16290.17, 38.03, 12157.75)),
    ("on the candle stand at (16291.5, 12159.7)", "goblin excavator", 14, (16291.0, 39.0, 12159.0), (16290.17, 38.03, 12157.75)),
    ("in the alcove's rock", "goblin prisoner", 0, (16244.724, 35.7812, 12189.493), (16244.72, 35.78, 12190.49)),
    ("in the terrain, 4.9 under it", "superior head jailor", 0, (17090.0, 5.0, 13246.0), (17092.31, 5.03, 13246.96)),
)

NEAR = 0.01


def at(rows, p):
    return [r for r in rows if all(abs(r[c] - v) < NEAR for c, v in zip(("x_coord", "y_coord", "z_coord"), p))]


def changes(db):
    out, lines, refusals, moved = [], [], [], 0
    has14 = db.execute("SELECT 1 FROM npcs WHERE world = 14 LIMIT 1").fetchone() is not None
    for why, name, world, frm, to in MOVES:
        if world == 14 and not has14:
            lines.append(f"skipped: {name} in world 14 ({why}): no world-14 rows on this shard")
            continue
        rows = tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = ? AND world = ?", (name, world))
        found, there = at(rows, frm), at(rows, to)
        if not found and len(there) == 1:
            moved += 1
            lines.append(f"moved already: {name} row {there[0]['id']} (world {world}) at {to}")
            continue
        if len(found) != 1:
            refusals.append(f"{name} in world {world}: {len(found)} rows at {frm}, want one (a GM moved it?)")
            continue
        r = found[0]
        after = {"x_coord": to[0], "y_coord": to[1], "z_coord": to[2]}
        out.append(("npcs", {"id": r["id"]}, after))
        lines.append(f"{name} row {r['id']} (world {world}), {why}: ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) "
                     f"-> {to}")
    return out, lines, refusals, not out and not refusals and moved > 0


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs",), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
