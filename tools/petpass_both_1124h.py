#!/usr/bin/env python3
"""Heirloom petition 2834 (2026-11-24): a Brellsworn zealot stood in the wall of Blackvein Deep's eastern hall.

The conned zealot (npcs 196187, world 10, the eastern hall's second; its server view's target) spawned at
(19020, -50.95, 9075): the hall's west wall, on placed rock with no terrain floor under it (cmd/campground: stand on
a placed thing, rock 0.5 from it north, west and south). The player's own spot, (19025.7, -51.0, 9072.3), is on the
floor but 3 from the west wall; he offered the room's middle instead, "like the other zealot in the room" (196188 at
(19045, -51.07, 9055)). His later "this position" was not recorded (a reply carries no server view), so the zealot
goes to the hall's open middle south of the pillars, (19045, -51.07, 9077.5): terrain floor at -51.07, not embedded,
no wall within 10.5 in 32 directions (cmd/campground's ground and walls), 22.5 from the other zealot. Facing 58, the
player's (1.4235 rad, entity.FacingByte), as he asked.

The row is found by name, world and its old place, and must read as it did when this was made (or as this writes
it); else refused. Live has no Brellsworn zealot (Blackvein Deep is Heirloom's alone): on Live this says so and writes
nothing (no log table either).

Report only by default; --apply in one transaction, a gm_audit line a row, log petpass_both_1124h_log; --undo
exact. NPC rows are read at the server's next start.

    python3 -E -s tools/petpass_both_1124h.py --db eqoa.db [--apply | --undo]
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "petpass_both_1124h_log"
DETAIL = ("Heirloom petition 2834: the Brellsworn zealot out of the eastern hall's west wall, to the hall's middle "
          "(tools/petpass_both_1124h.py)")
NAME, WORLD = "Brellsworn zealot", 10
OLD = {"x_coord": 19020.0, "y_coord": -50.95, "z_coord": 9075.0, "facing": 220}
NEW = {"x_coord": 19045.0, "y_coord": -51.07, "z_coord": 9077.5, "facing": 58}


def near(r, place):
    return all(abs(r[c] - v) < 0.01 for c, v in place.items())


def changes(db):
    out, lines, refusals = [], [], []
    zealots = tr.rows(db, "SELECT * FROM npcs WHERE npc_name = ? AND world = ?", (NAME, WORLD))
    if not zealots:
        lines.append(f"no {NAME} in world {WORLD} here: nothing to move")
    else:
        found = [r for r in zealots if near(r, OLD) or near(r, NEW)]
        if len(found) != 1:
            refusals.append(f"2834: {len(found)} {NAME} rows at ({OLD['x_coord']}, {OLD['z_coord']}) or the new "
                            "place, expected 1 (a GM's edit since?)")
        else:
            r = found[0]
            out.append(("npcs", {"id": r["id"]}, dict(r, **NEW)))
            lines.append(f"2834: npcs {r['id']} {NAME} ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) facing "
                         f"{r['facing']} -> ({NEW['x_coord']}, {NEW['y_coord']}, {NEW['z_coord']}) facing "
                         f"{NEW['facing']}, the eastern hall's middle")
    if refusals:
        out = []
    applied = bool(out) and all(tr.get(db, t, k) == a for t, k, a in out)
    lines.insert(0, f"{len(out)} rows")
    return out, lines, refusals, applied


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs",), "changes": changes}


def nothing_here(argv):
    """True (and said) when --db has no zealot to move and no log: an --apply there would leave an empty log."""
    import sqlite3
    if "--db" not in argv or argv.index("--db") + 1 >= len(argv):
        return False
    db = sqlite3.connect(f"file:{argv[argv.index('--db') + 1]}?mode=ro", uri=True)
    try:
        if tr.table(db, LOG) or not tr.table(db, "npcs"):
            return False
        n = db.execute("SELECT count(*) FROM npcs WHERE npc_name = ? AND world = ?", (NAME, WORLD)).fetchone()[0]
    finally:
        db.close()
    if n == 0:
        print(f"no {NAME} in world {WORLD} here: nothing to move, nothing written")
    return n == 0


if __name__ == "__main__":
    if nothing_here(sys.argv[1:]):
        sys.exit(0)
    sys.exit(tr.main(SPEC, argv=sys.argv[1:], doc=__doc__))
