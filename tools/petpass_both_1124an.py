#!/usr/bin/env python3
"""Heirloom petition 2878 (Arcaneamigo, a level 15 gnome wizard, 2026-11-24).

"i didnt have to go to alliaster's tome stone to get pop up, it gave me pop up and item by just entering the zone"

Psova's Robe (100375, the wizard's) step 4, and its three siblings Psova's Bracers (100315, cleric), Psova's Slippers
(100325, enchanter) and Psova's Helmet (100365, warrior), all step 4: a reach step, "Allister's tombstone", whose
journal sends the player north-east outside the Tomb of Elders in Collonridge Cemetery (Prima pp.210-215) to dig up
Allister's book. The rows stood at (22960, 54, 8640) radius 30, the cemetery's east side by the final thoughts' roam
(the importer's 22850, 8660 r90 guess moved, never onto the stone): the first stretch of the cemetery a player from
Klick'Anon walks into, 370 east of the tomb. He did the step at 06:34:10 coming in.

The Tomb of Elders is the cemetery's mausoleum (TUNARIA.ESF zone 44, sprite 0x931430CF at 22589, 8812; x 22568-22617,
z 8785-8840; the four Elders stand in it, y 84). North-east outside it (north is smaller z) stand the graves
0xF4486170 at (22639-22648, 8775-8784) and the headstone 0x22E938A4 at (22661.8, 54.1, 8750.3). The step now stands
at (22650, 54.1, 8765) radius 20: those stones and no further, the tomb's own walls and the crypt under the ground
east (y 42, z 8696-8740) outside it. Radius 20 is one of the engine's usual named-place sizes (30 reach steps at 20).

Other reach steps checked (enabled quests, both shards): none at 0,0,0 (the engine treats no point as anywhere only
for the Thanksgiving gate). The wide ones (60-250) are the target-less travel steps ("reach the Dead Hills" and the
like, a zone or region on purpose); of the 15 named-place steps, only these four were off their place.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log petpass_both_1124an_log; --undo exact. Quests are read at the next restart.
Who did the step early keeps what it gave.

    python3 -E -s tools/petpass_both_1124an.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124an_log"
DETAIL = ("[basis:shared petition 2878] Heirloom petition 2878: Allister's tombstone (Psova's quests step 4) moved "
          "from the cemetery's east side to north-east outside the Tomb of Elders (tools/petpass_both_1124an.py)")

QUESTS = {100315: "Psova's Bracers", 100325: "Psova's Slippers", 100365: "Psova's Helmet", 100375: "Psova's Robe"}
STEP, TARGET = 4, "Allister's tombstone"
OLD = {"world": 0, "x": 22960.0, "y": 54.0, "z": 8640.0, "radius": 30.0}
NEW = {"world": 0, "x": 22650.0, "y": 54.1, "z": 8765.0, "radius": 20.0}


def changes(db):
    out, lines, refusals = [], [], []
    for qid, name in sorted(QUESTS.items()):
        q = tr.get(db, "quests", {"questid": qid})
        s = tr.get(db, "queststeps", {"questid": qid, "step": STEP})
        if q is None or q["name"] != name or s is None or s["kind"] != "reach" or s["target"] != TARGET:
            refusals.append(f"{qid} step {STEP}: not {name!r}'s reach step at {TARGET!r}")
            continue
        now = {c: s[c] for c in OLD}
        if now != OLD and now != NEW:
            refusals.append(f"{qid} step {STEP}: place {now}, expected {OLD}")
            continue
        out.append(("queststeps", {"questid": qid, "step": STEP}, dict(NEW)))
        lines.append(f"queststeps {qid} {name} step {STEP} {TARGET}: ({now['x']}, {now['z']}) r{now['radius']} -> "
                     f"({NEW['x']}, {NEW['z']}) r{NEW['radius']}")
    if refusals:
        out = []
    applied = bool(out) and all(
        all(tr.get(db, t, k).get(c) == v for c, v in a.items()) for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("quests", "queststeps"), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
