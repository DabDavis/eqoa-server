#!/usr/bin/env python3
"""Which creatures wander, as a GM asked in seven petitions (2026-11-20; Live, the GM's requests approved as
submitted), each of one creature: the petition's target, by its name and its spawn (the npcs row whose spawn is
within 1 of the place the server saw), world 0.

  5326  "this decaying skeleton should roam"        (25114.3, 16731.2): an undead's kind stands; its row wanders
  5332  "this mob should roam" (a decaying watchdog) (20628.1, 8476.3)
  5344  "this troll raider should roam"             (24147.4, 26637.7): a players' race's body stands; its row wanders
  5346  "this troll raider should roam"             (24178.3, 26533.0)
  5345  "this troll crusader should roam a short distance" (24149.4, 26574.1): within 20 (ours: the server's 70 is
        the camp's whole hilltop)
  5328  "this madman should not roam"               (25017.4, 16701.8): npcmove has every desert madman wander
  5329  "this madman should not roam"               (25039.6, 16698.0)    (petition 1340); these two stand

npcwander, by the npcs row (entity/movement.go): a radius 0 is the server's roam radius (70), above 0 its own,
below 0 it stands on its spawn until pulled, whatever its name's rule. A row that wanders already, or is missing, is
said. Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_npc_roam_log; --undo
exact. npcwander is read at the server's start (after the next server update), or a GM panel edit of a row.

    python3 -E -s tools/petpass_npc_roam_20261120.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_npc_roam_log"
DETAIL = "Creatures' wander (a GM's petitions 5326-5346): tools/petpass_npc_roam_20261120.py"
SOURCE = "petpass-1120"

# petition, name, spawn x, z, radius
ROWS = (
    (5326, "decaying skeleton", 25114.3, 16731.2, 0.0),
    (5332, "decaying watchdog", 20628.1, 8476.3, 0.0),
    (5344, "troll raider", 24147.4, 26637.7, 0.0),
    (5346, "troll raider", 24178.3, 26533.0, 0.0),
    (5345, "troll crusader", 24149.4, 26574.1, 20.0),
    (5328, "desert madman", 25017.383, 16701.75, -1.0),
    (5329, "desert madman", 25039.637, 16698.047, -1.0),
)


def changes(db):
    out, lines, refusals = [], [], []
    for pet, name, x, z, radius in ROWS:
        found = tr.rows(db, "SELECT id, x_coord, z_coord FROM npcs WHERE lower(npc_name) = ? AND world = 0 AND "
                            "abs(x_coord - ?) < 1 AND abs(z_coord - ?) < 1", (name, x, z))
        if len(found) != 1:
            refusals.append(f"{pet}: {len(found)} rows of {name} at ({x:.1f}, {z:.1f}), want one")
            continue
        rid = found[0]["id"]
        have = tr.get(db, "npcwander", {"npcid": rid})
        what = "stands" if radius < 0 else ("wanders within %g" % radius if radius > 0 else "wanders (the server's radius)")
        note = f"petition {pet} (a GM): it {what}"
        out.append(("npcwander", {"npcid": rid}, {"npcid": rid, "radius": radius, "note": note, "source": SOURCE}))
        lines.append(f"{pet}: {name} row {rid}: {what}" + (f" (its row was radius {have['radius']})" if have else ""))
    applied = bool(out) and all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                                for t, k, a in out)
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcwander", "npcs"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
