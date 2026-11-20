#!/usr/bin/env python3
"""Three creatures' places and sizes (petitions of 2026-11-20 and 2026-11-18; both shards, rows found by name):

  2462 (Heirloom)  "Da Snapper for lvl 40 Troll SK quest is awful small." He is an alligator's body (0x3E0EBFE1, its
                   own scale 0.8) at size 1.0, a man's height at the shoulder; the quest's boss is drawn at 2.5 (ours:
                   twice a man's length, the crocosaurus's 12 being a raid's).
  2464 (Heirloom)  "In ZP, Arios the Battered is in the wall I believe." A GM moved him on Heirloom after the petition
                   (2026-11-20 04:10 UTC, /move: (4930.0, 147.6, 7050.0) -> (4944.3, 147.7, 7057.4), facing 161),
                   clear of the room's pillars by the ground's collision; Live's row still stands at the old place and
                   is moved to the same. A row there already is left.
  2319 (Heirloom)  "Agent of Honor / Lord Pickclaw is in unattackable position on mountain". He stood at
                   (16884.9, 100.2, 12190.7), on the rim of the sunken canyon north-east of Runnyeye's tree, 94 above
                   its floor and over a cliff, where the questers stand; moved down onto the canyon's floor at
                   (16895.0, 6.0, 12240.0) (internal/ground: a floor to stand on, nothing placed in the body, no wall
                   between it and the reporter's spot 20 off). Work board entry 1037 (his raising by a trigger) names
                   the old place: its builder should take the new one.

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_npc_places_log; --undo exact.
npcs rows are read at the server's start (after the next server update).

    python3 -E -s tools/petpass_npc_places_20261120.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_npc_places_log"
DETAIL = "Da Snapper's size, Arios the Battered's and Lord Pickclaw's places (petitions 2462, 2464, 2319): " \
         "tools/petpass_npc_places_20261120.py"

# name, world, from (x, z) within 1 (None: any row of the name), what to write
CHANGES = (
    (2462, "da snapper", 0, None, {"size": 2.5}),
    (2464, "arios the battered", 5, (4930.0, 7050.0), {"x_coord": 4944.26220703125, "y_coord": 147.6572265625,
                                                       "z_coord": 7057.42236328125, "facing": 161}),
    (2319, "lord pickclaw", 0, (16884.895, 12190.709), {"x_coord": 16895.0, "y_coord": 5.9566536, "z_coord": 12240.0}),
)


def changes(db):
    out, lines, refusals = [], [], []
    for pet, name, world, at, after in CHANGES:
        rows = tr.rows(db, "SELECT * FROM npcs WHERE lower(npc_name) = ? AND world = ?", (name, world))
        if at is not None:
            moved = [r for r in rows if all(abs(r[c] - after[c]) < 0.01 for c in ("x_coord", "z_coord"))]
            rows = [r for r in rows if abs(r["x_coord"] - at[0]) < 1 and abs(r["z_coord"] - at[1]) < 1]
            if not rows and moved:
                lines.append(f"{pet}: {name} row {moved[0]['id']} stands at the new place already")
                continue
        if len(rows) != 1:
            refusals.append(f"{pet}: {len(rows)} rows of {name} in world {world}" + (f" at {at}" if at else "") +
                            ", want one")
            continue
        r = rows[0]
        out.append(("npcs", {"id": r["id"]}, after))
        lines.append(f"{pet}: {name} row {r['id']}: " + ", ".join(f"{c} {r[c]} -> {v}" for c, v in after.items()))
    applied = all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                  for t, k, a in out)
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs",), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
