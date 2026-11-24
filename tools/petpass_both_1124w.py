#!/usr/bin/env python3
"""Live petitions 5582 and 5583 (Warsloth, 2026-11-24, Creature or NPC): "mako sharks on land at croc island by bbw"
and "bbw croc island the crocs are out in water mostly". Croc Island by Blackwater (world 0, about (25750, 11050);
not the tame crocodiles' Croc Island at (25470, 17330), tools/petpass_both_1123v.py). Both shards.

  5582: the code's (branch pet1124w): all 37 mako shark spawns are in the water (cmd/campground: env 1, the bed
        26-42 under their 50.1); the sea's water leaf runs on under the island's sand at a swimmer's height, so a
        wander leg went in under it and the ground lifted the shark onto the sand. No row changes here.
  5583: the island's crocodiles are caiman (8 rows, all this island's) and young crocodile (19: 9 here, 10 on the
        shore south-west of Freeport at x 23878-24724, z 17606-17888), npcmove "both" (the seed's: "in and out of
        the water"): a wander's legs end in the sea as readily as on land, and the island is small against their
        roam (70), so in the petition's view 11 of 17 swam off it (at 49.8, the surface). Every one of the 27 spawns
        stands on land (cmd/campground: env 0, on the terrain at 52.7-59.7). Both rows' water word is cleared, as the
        tame crocodile's was (keeps by spawn, entity/water.go's keepsToLocked): they keep to the land round their
        spawns, and a fight still takes one where its enemy is. Roam and pace kept. No spawn moves.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_both_1124w_log; --undo exact. npcmove is read at the server's start.

    python3 -E -s tools/petpass_both_1124w.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124w_log"
DETAIL = ("Live petition 5583: Croc Island (by Blackwater)'s caiman and young crocodiles keep to the land by their "
          "spawns (tools/petpass_both_1124w.py)")
SOURCE = "petpass-1124w"
KINDS = {"caiman": 8, "young crocodile": 19}  # rows each, all on land


def changes(db):
    out, lines, refusals = [], [], []
    for name, want in KINDS.items():
        r = tr.get(db, "npcmove", {"name": name})
        n = tr.rows(db, "SELECT count(*) AS c FROM npcs WHERE npc_name = ?", (name,))[0]["c"]
        after = {"water": "", "note": "Live petition 5583: keeps by its spawn (all on land: Croc Island by "
                                      f"Blackwater's and the shore's); was in and out of the water ({name})",
                 "source": SOURCE}
        if r is None:
            refusals.append(f"no npcmove row for {name}")
        elif r["water"] != "both" and any(r[c] != v for c, v in after.items()):
            refusals.append(f"npcmove {name} is not as expected: {dict(r)}")
        elif n != want:
            refusals.append(f"{n} {name} rows, expected {want} (all checked on land): others would change too")
        else:
            out.append(("npcmove", {"name": name}, dict(r, **after)))
            lines.append(f"5583: npcmove {name} water {r['water']!r} -> '' (by spawn), roam {r['roam']} speed "
                         f"{r['speed']} kept; {n} rows, all spawned on land")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcmove", "npcs"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
