#!/usr/bin/env python3
"""The Estate's door kept by the server (petition 2212, Hoofbite, Heirloom 2026-11-16: "The Guardian in IOD blocking
entrance to cazic thule house is able to be bypassed via logging and then running forward while relogging").

The Guardian (npcs 118859, Isle of Dread, at 25706.2, 54.1, 3230.7) is the door into Cazic Thule's Estate: his body
is what CLIENT stops a walk at, and a client that logs in standing in it walks through. One questdoors row makes the
server keep it (internal/entity/questdoors.go): a character that steps from the porch into the way in while the
Guardian stands at his post is put back on the porch and told so. Nothing passes him but his absence (Cazic's
Scent's ten minutes, itemhide 1231: "each and every time", t196) and a GM: the row has no quest and no item.

  inner    the way in past him: x 25707.5-25718, y 44-58, z 3227-3235 (the top floor's passage at 54 to the ramp, the
           room at 49 under it, the talisman's landing at 25712, 54.4, 3232 inside it). Measured on the ground file
           live uses (worlds/ground/0.ground): from his middle a walk at 55.3 meets a wall at every z off 3228-3234
           east of x 25707, the house's own surfaces (49 floor, 59 ceiling) run x 25708-25718.
  outer    the porch, his body up to his middle: x 25690-25706.2, y 51-60, z 3215-3250 (Hoofbite's places at the door
           that night, 25702.1/3222.5 and 25705.8/3231.9 at 54.1-55.0, inside it).
  outside  (25700, 54.3, 3231): 6 west of him on the porch's floor (54.25 there), in the open from his middle.

The server reads table questdoors at its start (it makes the table there; this tool makes it too when the server
that reads it has not started yet, as its before.sh). Report only by default; --apply in one transaction, a gm_audit
line, log ct_guardian_door_log; --undo exact.

    python3 -E -s tools/ct_guardian_door_20261116.py --db eqoa.db [--apply | --undo]
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "ct_guardian_door_log"
DETAIL = "The Estate's door kept by the server (petition 2212): tools/ct_guardian_door_20261116.py"
SRC = "ctdoor1116"

GUARDIAN = 118859
AT = (25706.209, 54.126, 3230.661)
INNER = [[25707.5, 44.0, 3227.0, 25718.0, 58.0, 3235.0]]
OUTER = [[25690.0, 51.0, 3215.0, 25706.2, 60.0, 3250.0]]
OUTSIDE = [25700.0, 54.3, 3231.0]
TEXT = "The Guardian bars your way."

# What internal/database/questdoors.go makes at the server's start.
SCHEMA = """CREATE TABLE IF NOT EXISTS questdoors (
		npcid   INTEGER PRIMARY KEY,
		questid INTEGER NOT NULL DEFAULT 0,
		step    INTEGER NOT NULL DEFAULT 0,
		item    INTEGER NOT NULL DEFAULT 0,
		inner   TEXT NOT NULL,
		outer   TEXT NOT NULL,
		outside TEXT NOT NULL,
		text    TEXT NOT NULL DEFAULT '',
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	)"""


def row():
    return {"npcid": GUARDIAN, "questid": 0, "step": 0, "item": 0, "inner": json.dumps(INNER), "outer": json.dumps(OUTER),
            "outside": json.dumps(OUTSIDE), "text": TEXT,
            "note": "the Estate's Guardian keeps his doorway on the server too: a relog in his body walked through "
                    "(petition 2212)", "source": SRC}


def changes(db):
    refusals, lines = [], []
    g = tr.rows(db, "SELECT id, npc_name, world, x_coord, y_coord, z_coord FROM npcs WHERE id = ?", (GUARDIAN,))
    if not g:
        refusals.append(f"no npcs row {GUARDIAN}")
    else:
        g = g[0]
        if g["npc_name"] != "The Guardian" or g["world"] != 0 or \
                any(abs(g[c] - v) > 0.5 for c, v in zip(("x_coord", "y_coord", "z_coord"), AT)):
            refusals.append(f"npcs {GUARDIAN} is {g['npc_name']!r} in world {g['world']} at "
                            f"({g['x_coord']:.1f}, {g['y_coord']:.1f}, {g['z_coord']:.1f}), not the Guardian at the door")
    want = row()
    if tr.table(db, "questdoors"):
        have = tr.get(db, "questdoors", {"npcid": GUARDIAN})
        if have == want:
            return [], lines, [], True
        if have is not None:
            refusals.append(f"questdoors {GUARDIAN} is there and not this tool's: {have}")
    else:
        lines.append("no questdoors table yet: made with the rows (as the server's start makes it)")
    lines.append(f"questdoors {GUARDIAN} The Guardian: inner {INNER}, outer {OUTER}, outside {OUTSIDE}, {TEXT!r}")
    return [("questdoors", {"npcid": GUARDIAN}, want)], lines, refusals, False


def prepare(db):
    if tr.table(db, "questdoors"):
        return []
    db.execute(SCHEMA)
    return ["made table questdoors"]


SPEC = {"log": LOG, "detail": DETAIL, "needs": ("npcs",), "changes": changes, "prepare": prepare}

if __name__ == "__main__":
    sys.exit(tr.main(SPEC, doc=__doc__))
