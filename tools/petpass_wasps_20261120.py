#!/usr/bin/env python3
"""The wasp hive by Fayspires, as a GM asked in petition 5319 (2026-11-20; Live, approved as submitted): "Was worker
and wasp royal guards have their levels switched. The royal guards should be higher level. The wasp queen should have
a 2 hour repop time, and should be level 20".

The hive (world 0, within 200 of (17740, 7230)): workers of levels 7, 11, 12 and 13 (six, six, one and six), two
royal guards of level 9, the queen of 10. Switched: the royal guards take the workers' top level, 13, and every
worker above the guards' 9 takes it, 9 (the six level 7 workers by the queen stay 7); the queen level 20, respawning
in 7,200 s (npcrespawn, her row's own). A creature's hit points are its level's (entity/balance.go), so nothing else
changes. Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_wasps_log; --undo
exact. npcs rows and npcrespawn are read at the server's start (after the next server update).

    python3 -E -s tools/petpass_wasps_20261120.py --db eqoa.db [--apply | --undo]
"""
import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_wasps_log"
DETAIL = "The wasp hive's levels and queen (a GM's petition 5319): tools/petpass_wasps_20261120.py"
SOURCE = "petpass-1120"
HIVE = (17740.0, 7230.0, 200.0)
GUARD_LEVEL, WORKER_LEVEL, QUEEN_LEVEL, QUEEN_RESPAWN = 13, 9, 20, 7200.0


def changes(db):
    out, lines, refusals = [], [], []
    x, z, r = HIVE
    hive = [row for row in tr.rows(db, "SELECT id, lower(npc_name) AS n, npc_level, x_coord, z_coord FROM npcs "
                                       "WHERE world = 0 AND lower(npc_name) IN ('wasp worker', 'wasp royal guard', "
                                       "'wasp queen')")
            if math.hypot(row["x_coord"] - x, row["z_coord"] - z) <= r]
    queens = [row for row in hive if row["n"] == "wasp queen"]
    if len(queens) != 1:
        refusals.append(f"{len(queens)} wasp queens in the hive, want one")
    for row in hive:
        lvl = row["npc_level"]
        want = {"wasp royal guard": GUARD_LEVEL, "wasp queen": QUEEN_LEVEL}.get(row["n"],
                                                                                WORKER_LEVEL if lvl > WORKER_LEVEL else lvl)
        if want != lvl:
            out.append(("npcs", {"id": row["id"]}, {"npc_level": want}))
            lines.append(f"  {row['n']} row {row['id']}: level {lvl} -> {want}")
    for q in queens:
        have = tr.get(db, "npcrespawn", {"npc_row": q["id"]})
        out.append(("npcrespawn", {"npc_row": q["id"]}, {"npc_row": q["id"], "seconds": QUEEN_RESPAWN,
                                                          "note": "petition 5319 (a GM): the queen respawns in 2 hours",
                                                          "source": SOURCE}))
        lines.append(f"  wasp queen row {q['id']}: respawn {have['seconds'] if have else 'the server default'} -> "
                     f"{QUEEN_RESPAWN:.0f} s")
    lines.insert(0, f"the hive: {len(hive)} rows, {len(out)} to write")
    applied = all(tr.get(db, t, k) is not None and all(tr.get(db, t, k).get(c) == v for c, v in a.items())
                  for t, k, a in out)
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcrespawn"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
