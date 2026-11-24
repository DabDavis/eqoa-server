#!/usr/bin/env python3
"""Heirloom petition 2851 (Aeris, 2026-11-24 03:10, Quest): "The orb of light at the top of Saerk tower is in the
wrong spot". Both shards (Live's row is the same).

  The upper orb of light (npcs 108826, on the 305.0 floor under Saerk's) WANDERS: tools/disperse_patrol_20261112.py
  gave it npcwander radius 0 ("a lone creature of a standing kind wanders"), so it roams within the server's 70 of
  its spawn. Her server view has it at (21242.7, 305.0, 13521.4), 14.9 from its spawn (21253.7, 305.188, 13511.2),
  where its step pad (orbs 10) and its talk (lightspheres 8) stay. Its spawn is the authored place
  (~/.cache/petwatch/batch-1110e/petfix213_20261110.py read it there, and orbs 9's landing is put 4.5 from it); the
  ground-floor orb (108825) has no npcwander row and stands. Here the upper orb stands too: radius 0 -> -1 (an
  npcwander row below 0: it stands on its spawn whatever its name's or its kind's rule; NPCWanderRows).

  Not moved: where she stood (21239.9, 13521.4) is beside where the orb had wandered, not a place she asks for, and
  no guide or video gives another. Saerk, 12.8 over it and 2.1 from it across, is no longer taken for it once
  LightSphereNear is 1 (branch pet1124s, petition 2849).

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_heirloom_1124v_log; --undo exact. It takes effect at the server's next
start (npcwander is read then).

    python3 -E -s tools/petpass_heirloom_1124v.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_heirloom_1124v_log"
DETAIL = "Heirloom petition 2851: Saerk's tower's upper orb of light stands on its spawn (tools/petpass_heirloom_1124v.py)"
ORB = ("orb of light", 0, (21253.7, 305.188, 13511.2))
ORB_PAD = "Saerk's Tower: the orb of light on Saerk's floor, down to the ground floor"
WANDERS, STANDS = 0.0, -1.0
NOTE = "petition 2851: the upper orb of light of Saerk's tower stands on its spawn, where its step pad and talk are"


def changes(db):
    out, lines, refusals = [], [], []
    name, world, (x, y, z) = ORB
    npcs = tr.rows(db, """SELECT id FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) < 0.05
        AND abs(y_coord - ?) < 0.05 AND abs(z_coord - ?) < 0.05""", (name, world, x, y, z))
    pads = tr.rows(db, "SELECT id FROM orbs WHERE name = ? AND world = ? AND abs(x - ?) < 0.05 AND abs(z - ?) < 0.05",
                   (ORB_PAD, world, x, z)) if tr.table(db, "orbs") else []
    if len(npcs) != 1:
        refusals.append(f"{len(npcs)} {name} rows at ({x}, {y}, {z}), expected 1")
    if len(pads) != 1:
        refusals.append(f"{len(pads)} orbs rows {ORB_PAD!r} at ({x}, {z}), expected 1")
    if not refusals:
        key = {"npcid": npcs[0]["id"]}
        w = tr.get(db, "npcwander", key)
        if w is None:
            refusals.append(f"npcwander {key['npcid']}: no row (it would stand by its kind already?)")
        elif w["radius"] not in (WANDERS, STANDS):
            refusals.append(f"npcwander {key['npcid']}: radius {w['radius']}, not {WANDERS} (a GM's edit since?)")
        else:
            out.append(("npcwander", key, dict(w, radius=STANDS, note=NOTE, source="petpass-1124v")))
            lines.append(f"2851: npcwander {key['npcid']} ({name} on the 305 floor, orbs pad {pads[0]['id']}) radius "
                         f"{w['radius']} ({w['note']!r}, {w['source']}) -> {STANDS}: stands on its spawn")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs", "npcwander"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
