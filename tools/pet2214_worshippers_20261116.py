#!/usr/bin/env python3
"""Petition 2214 (Heirloom, 2026-11-16): "Dragon worshippers keep spawning they should not be doing that".

The two dragon worshippers that stand by Siliskor at the crooked elephant statue in the Elephant Graveyard (npcs 111882
at 14541.7, 26791.2 and 111884 at 14550.8, 26762.6, beside Siliskor 111881 and Pamus Magemell 111883) had no npcrespawn
row, so they came back on the server's default (respawn_after: 15 s): the log has 111882 and 111884 "back in 15s" at
02:44-02:46 UTC, a minute or two after each kill. They are Siliskor's two companions: quest 101266 step 9 raises him
"with Pomus Magemell and two dragon worshippers" (forumotion t177, work board 708: "they finish the tank whose HP he set
to 700"), and the standing pair is the encounter's. They come back with him now: Siliskor's own 12 hours (43,200 s,
petition 1240), an npcrespawn row each. The Dragon Cultists' cave camp of the name (114076-114083) is the area's trash
and is left on the default; so is Pamus Magemell (90 s, petfix259, the owner's named rule). No spawn is added, moved or
removed: making the pair spawn-only (npcspawnonly "#111882", "#111884") would take them out of the world, a GM's call.

Refused, nothing written, when a row is not as expected (a GM's edit since: either npcs row moved, renamed or gone, or
an npcrespawn row there already). Report only by default; --apply in one transaction, a gm_audit line a row, log
pet2214_1116_log; --undo exact (a row changed since is left and said). npcrespawn is read at the server's start, or at
once by the GM panel's live read of the table (gmweb liveRespawn): the next time each comes back uses it.
    tools/pet2214_worshippers_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  the writer (gm_audit, log, exact undo)

LOG = "pet2214_1116_log"
DETAIL = ("petition 2214 (Trulytwo): Siliskor's two dragon worshippers back with him, his 12 h "
          "(tools/pet2214_worshippers_20261116.py)")
SECONDS = 43200.0
SILISKOR = 111881
# npcs row: its place (x, z) within a step of it
ROWS = {111882: (14541.7, 26791.2), 111884: (14550.8, 26762.6)}
NOTE = ("dragon worshipper, Siliskor's companion at the crooked elephant statue (quest 101266 step 9; board 708): "
        "back with him, his 12 hours (petition 2214, Trulytwo: they came back every 15 s)")


def plan(db):
    changes, lines, refusals = [], [], []
    boss = db.execute("SELECT r.seconds FROM npcs n JOIN npcrespawn r ON r.npc_row = n.id WHERE n.id = ? AND n.npc_name = 'Siliskor'",
                      (SILISKOR,)).fetchone()
    if boss is None or boss[0] != SECONDS:
        refusals.append(f"Siliskor {SILISKOR} is not on a {SECONDS:.0f} s respawn here ({boss}): look first")
    for nid, (x, z) in ROWS.items():
        r = db.execute("SELECT npc_name, world, x_coord, z_coord FROM npcs WHERE id = ?", (nid,)).fetchone()
        if r is None or r[0] != "dragon worshipper" or r[1] != 0 or abs(r[2] - x) > 1 or abs(r[3] - z) > 1:
            refusals.append(f"npcs {nid} is not the dragon worshipper at ({x}, {z}): {r}")
            continue
        have = fm.row_of(db, "npcrespawn", {"npc_row": nid})
        if have is not None:
            refusals.append(f"npcs {nid} has an npcrespawn row already: {have}")
            continue
        changes.append(("npcrespawn", None, {"npc_row": nid, "seconds": SECONDS, "note": NOTE, "source": "petition 2214"}))
        lines.append(f"npcrespawn {nid} (dragon worshipper at {x:.0f}, {z:.0f}): default (15 s) -> {SECONDS:.0f} s")
    return changes, lines, refusals


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    fm.LOG, fm.DETAIL = LOG, DETAIL
    fm.LOG_SCHEMA = fm.LOG_SCHEMA.replace("moogie_tunare_1110_log", LOG)
    fm.KEYS["npcrespawn"] = ("npc_row",)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = fm.undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if fm.table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = fm.apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
