#!/usr/bin/env python3
"""Heirloom petition 2861 (Lonewulff, 2026-11-24, Creature or NPC): "flying".

His target was an ancient sawtusk (level 50, world 0) at its spawn (14817.0, 51.3, 26760.5), 102.5 away, on the
plateau's edge above the valley floor the Plane of Disease creatures stand on (flesh trampler, bone hunter, 15.3).
It is one of a camp of seven on that plateau: npcs 156163 (the old one, level 44, (14817.0, 51.26, 26748.5)) and six
seeded round it in a 12-unit grid, Heirloom 166515-166520, Live 190485, 190553, 190836, 191397, 191480, 191831 (the
same places on both shards). The ground under them (cmd/campground, the server's own floors): the plateau's floor
runs to z ~26753 (50.2 at x 14817, z 26752), then the cliff falls to the valley, a steep face with no floor anywhere
from z ~26754 to ~26768 (21.2 by 26772, 15.3 by 26800). The grid's south row, z 26760.5 (x 14793, 14817, 14841), was
set out past the edge at the plateau's height: three sawtusks stood in the air over the cliff face, 7 units out.
There is no floor under them at any height, so no height alone fixes them: each is brought back onto the plateau,
keeping x, facing (206) and the camp's shape, onto the floor there (checked: no roof, not embedded, no wall between it
and 156163):

    (14817.032, 51.3, 26760.487) -> (14817.032, 53.01, 26742.487)  between 166520 and 156163
    (14841.032, 51.0, 26760.487) -> (14841.032, 51.25, 26748.487)  the middle row's end
    (14793.032, 51.0, 26760.487) -> (14793.032, 49.47, 26748.487)  the middle row's other end

The other four (156163, 51.26 on 51.25; 166516/190836 51.3 on 51.25; 166517/191831 51.0 on 51.05; 166520/191397 54.2
on 54.19) stand on their floors, as does the camp's lone ancient sawtusk 146747 (62.80 on 62.80).

Each row is found by name, world and its old place (or its new one), and must be the only one there; else refused.

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_both_1124ab_log; --undo
exact. NPC rows are read at the server's next start, or at once by the GM panel's reload_npcs.

    python3 -E -s tools/petpass_both_1124ab.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124ab_log"
DETAIL = ("Heirloom petition 2861: three ancient sawtusks set out in the air past the plateau's edge above the Plane "
          "of Disease valley brought back onto the plateau (tools/petpass_both_1124ab.py)")
NAME, WORLD, FACING = "ancient sawtusk", 0, 206
# (old (x, y, z), new (x, y, z))
MOVES = [
    ((14817.032, 51.3, 26760.487), (14817.032, 53.01, 26742.487)),
    ((14841.032, 51.0, 26760.487), (14841.032, 51.25, 26748.487)),
    ((14793.032, 51.0, 26760.487), (14793.032, 49.47, 26748.487)),
]


def place(p):
    return {"x_coord": p[0], "y_coord": p[1], "z_coord": p[2]}


def near(r, p):
    return all(abs(r[c] - v) < 0.01 for c, v in place(p).items())


def changes(db):
    out, lines, refusals = [], [], []
    rows = tr.rows(db, "SELECT * FROM npcs WHERE npc_name = ? AND world = ?", (NAME, WORLD))
    for old, new in MOVES:
        found = [r for r in rows if near(r, old) or near(r, new)]
        if len(found) != 1:
            refusals.append(f"2861: {len(found)} {NAME} rows at {old} or {new}, expected 1 (a GM's edit since?)")
            continue
        r = found[0]
        if r["facing"] != FACING:
            refusals.append(f"2861: npcs {r['id']} faces {r['facing']}, expected {FACING}")
            continue
        out.append(("npcs", {"id": r["id"]}, dict(r, **place(new))))
        lines.append(f"2861: npcs {r['id']} {NAME} ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) -> {new}, "
                     f"the plateau's floor")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs",), "changes": changes}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
