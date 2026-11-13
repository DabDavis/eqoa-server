#!/usr/bin/env python3
"""The Toxxulia cave's thorns as the GM Jibwiz asked in petition 1960 (2026-11-13): "apply them to within 1 feet or
inside of the thorns. Have them do 500 damage on a 6 second cool down per individual player. If 5 people walk into the
thorn, all of them should take 500 damage and if they remain in it for 6 seconds THEN MOVE they take 500 again. However
if they stay still it should not continuously damage them."

Over tools/seed_hazards.py's 144 rows (hazards, source vines: 1000 every 2 s, each a capsule holding 80% of its vine's
mesh across, radius 1 to 2.51), matched by their note (each its own zone actor):

  damage    1000 -> 500
  every_ms  2000 -> 6000 (each player's own clock: entity/hazards.go's Player.hazardNext, so five in it all take it)
  on_move   0 -> 1 (after a hit, a player standing still is not hit again until it moves: entity/hazards.go)
  capsule   cmd/hazardscan read again with -share 1.0 (the whole mesh across, no cap) and the radius 1 more: 3.01 to
            7.52, its ends and radius tools/thorns_1960_20261113.json's (the scan's own, so the thorns themselves and a
            foot round them)

hazards.on_move is made here when the table has none (as the server makes it at its start). Refused, nothing written,
unless every vines row is as tools/seed_hazards.py wrote it (damage 1000, every 2000) or as this tool writes it. Report
only by default; --apply in one transaction, a gm_audit line a row, log thorns_1960_log; --undo exact (on_move stays as
a column). The server reads table hazards at its start and at a GM panel edit of a hazards row (a running server takes
the rows by opening and saving any row in the panel's Hazards). Needs the server with on_move (an older one hits every
2 s whatever the column says, at 500 every 6 s).

    python3 -E -s tools/thorns_1960_20261113.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import tix4_rows as tr  # noqa: E402

LOG = "thorns_1960_log"
DETAIL = "The Toxxulia thorns: 500 every 6 s a player, only once moved, a foot round them (the GM Jibwiz's petition " \
         "1960): tools/thorns_1960_20261113.py"
DATA = os.path.join(HERE, "thorns_1960_20261113.json")
SOURCE = "vines"
NOW = {"damage": 500, "every_ms": 6000, "on_move": 1}
WAS = {"damage": 1000, "every_ms": 2000}
GEOM = ("x1", "y1", "z1", "x2", "y2", "z2", "radius")


def changes(db):
    out, lines, refusals = [], [], []
    shapes = {r["note"]: r for r in json.load(open(DATA))}
    has_move = tr.column(db, "hazards", "on_move")
    rows = tr.rows(db, "SELECT * FROM hazards WHERE source = ? ORDER BY id", (SOURCE,))
    if len(rows) != len(shapes):
        refusals.append(f"{len(rows)} hazards rows of source {SOURCE}, the file has {len(shapes)}")
    done = 0
    for r in rows:
        s = shapes.get(r["note"])
        if s is None:
            refusals.append(f"hazards {r['id']}: its note is not the file's: {r['note']!r}")
            continue
        want = {**NOW, **{k: s[k] for k in GEOM}}
        if has_move and all(abs(r[k] - v) < 1e-6 if isinstance(v, float) else r[k] == v for k, v in want.items()):
            done += 1
            continue
        if any(r[k] != v for k, v in WAS.items()):
            refusals.append(f"hazards {r['id']}: damage {r['damage']} every {r['every_ms']}, not 1000 every 2000")
            continue
        out.append(("hazards", {"id": r["id"]}, {**r, **want}))
    if rows and done == len(rows):
        return [], lines, [], True
    lines.append(f"hazards: {len(out)} of {len(rows)} vines rows to 500 every 6 s, only once moved, the whole mesh and "
                 f"a foot round it (radius {min((o[2]['radius'] for o in out), default=0):.2f} to "
                 f"{max((o[2]['radius'] for o in out), default=0):.2f})")
    if not has_move:
        lines.append("hazards.on_move: no such column yet; made at --apply (as the server makes it)")
    return out, lines, refusals, False


def prepare(db):
    if tr.column(db, "hazards", "on_move"):
        return []
    db.execute("ALTER TABLE hazards ADD COLUMN on_move INTEGER NOT NULL DEFAULT 0")
    return ["hazards.on_move made"]


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("hazards",), "changes": changes, "prepare": prepare}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
