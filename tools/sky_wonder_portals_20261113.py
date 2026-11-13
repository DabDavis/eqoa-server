#!/usr/bin/env python3
"""The Isle of Wonder's portals (2026-11-13; the owner: "lets look at Plane of Sky teleporters make sure they work").

The Plane of Sky's main isle and the Isle of Wonder above it are not joined by any walk (world 4's ground and walls
files: 552 apart, 83 up, no floor between), and nothing sent a player from one to the other: the main isle's
"Island of Wonder" (npcs row 119498, beside Sartheret, an essence's body: the retail import's portal) and the Isle of
Wonder's own astral essence (row 119523) said nothing. So the Marks of the Majestic and of Desolation (the snow and
desolation chains' starts and their ways in, waysender.go) and the Manifestations stood where only a chain's last
essence (skypass.go) or the Heirloom server's Teleporter set anyone down. The guide (EQOA-Data
Quests/Portal/plane-of-sky.json): "port up to the main PoS isle ... then pass through to the Isle of Wonder", "From
the Isle of Wonder port, facing Sarthet", and back, "Then go back to the main isle on PoS and find the Snow Essence
Portal".

Two GM portals (table gmportals, handlers/gmportal.go: "Do you wish to travel to <place>?" Yes or No, a waysender's
trip; no fee, no level, the group not moved), each on its one row, so the main isle's other astral essence (the
twins' quest, guide gaps sky-twins) is left as it is; a quest at the Isle of Wonder's (the twins' multicoloured
lights, offered only to one holding them) still comes first:

  - "Island of Wonder" (row 119498) -> "the Isle of Wonder": where every chain's last essence and the Teleporter's
    Plane of Sky land (skyIsleOfWonder, petition 1605's spot), facing as they do;
  - the Isle of Wonder's "astral essence" (row 119523) -> "the main isle": where the Sky Portal's trip lands (the
    Endless Tower's way up, beside Sartheret, 29 from the Island of Wonder), facing Sartheret.

Both landings are on a floor of world 4 at their height, out of the water, and walk to everyone they are for
(handlers/skytele_real_test.go). The server reads switched-on portals again every 30 s: live within a minute of
--apply, no restart. Refused, nothing written, unless each row stands where it is named, has no portal of its own
already and the set is not in. Report only by default; --apply in one transaction, a gm_audit line a row, log
sky_wonder_portals_1113_log; --undo exact (a row changed since is left and said).

    tools/sky_wonder_portals_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fix_moogie_tunare_20261110 as fm  # noqa: E402  gm_audit's writer (dab, tool) and Go's JSON

LOG = "sky_wonder_portals_1113_log"
DETAIL = "the Isle of Wonder's portals (the owner, 2026-11-13: Plane of Sky teleporters): tools/sky_wonder_portals_20261113.py"
REACH = 5  # how near its spot the row must stand

# (row, name, its spot (x, z), the place the question names, the landing (x, y, z, facing))
PORTALS = [
    (119498, "Island of Wonder", (4823.95, 5497.42), "the Isle of Wonder", (5000.09, 333.54, 4998.23, 3.068)),
    (119523, "astral essence", (4999.87, 4999.70), "the main isle", (4834.8, 250.1, 5524.7, 0.25)),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    id     INTEGER NOT NULL,   -- the gmportals row made
    after  TEXT NOT NULL,      -- the row as written (JSON)
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def row(db, id_):
    cur = db.execute("SELECT * FROM gmportals WHERE id = ?", (id_,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def plan(db):
    """(rows, lines, refusals): the gmportals rows to make."""
    rows, lines, refusals = [], [], []
    if not fm.table(db, "gmportals"):
        return rows, lines, ["no table gmportals: a server older than /markportal"]
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (fm.WHO,)).fetchone() if fm.table(db, "accountinfo") else None
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for npc_id, name, (sx, sz), place, (x, y, z, facing) in PORTALS:
        r = db.execute("SELECT npc_name, world, x_coord, y_coord, z_coord FROM npcs WHERE id = ?", (npc_id,)).fetchone()
        if r is None:
            refusals.append(f"npcs row {npc_id} ({name}) is not there")
            continue
        if r[0].strip().lower() != name.lower() or r[1] != 4 or math.hypot(r[2] - sx, r[4] - sz) > REACH:
            refusals.append(f"npcs row {npc_id} is {r[0]!r} in world {r[1]} at ({r[2]:.1f}, {r[4]:.1f}), not {name!r} at ({sx}, {sz})")
            continue
        had = db.execute("SELECT id, place, enabled FROM gmportals WHERE npc_id = ?", (npc_id,)).fetchall()
        if had:
            refusals.append(f"{name} (row {npc_id}) has a portal already: {had}")
            continue
        rows.append({"npc_id": npc_id, "npc_name": r[0], "entry_world": 4, "entry_x": r[2], "entry_y": r[3], "entry_z": r[4],
                     "place": place, "world": 4, "x": x, "y": y, "z": z, "facing": facing,
                     "exit_by": "stand", "exit_name": "", "exit_npc_id": 0, "fee": 0, "min_level": 0, "enabled": 1,
                     "petition": 0, "accountid": acct[0] if acct else 0, "charname": "", "created": now})
        lines.append(f"{name} (row {npc_id}, world 4 at ({r[2]:.1f}, {r[3]:.1f}, {r[4]:.1f})): "
                     f"\"Do you wish to travel to {place}?\" Yes: world 4 ({x}, {y}, {z}) facing {facing}, no fee, no level")
    return rows, lines, refusals


def apply(db, rows):
    db.execute(LOG_SCHEMA)
    for after in rows:
        cols = list(after)
        cur = db.execute(f"INSERT INTO gmportals ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
        id_ = cur.lastrowid
        now = row(db, id_)
        a = fm.audit(db, "insert", "gmportals", {"id": id_}, None, now, DETAIL)
        db.execute(f"INSERT INTO {LOG} (id, after, audit) VALUES (?, ?, ?)", (id_, json.dumps(now, sort_keys=True, default=str), a))
    return len(rows)


def undo(db):
    """Each portal still as written deleted, the last first, its gm_audit line. Returns (back, left)."""
    if not fm.table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, id_, after in db.execute(f"SELECT seq, id, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row(db, id_)
        if now is None or json.dumps(now, sort_keys=True, default=str) != after:
            left += 1
            print(f"  left: gmportals {id_}, changed since it was written")
            continue
        db.execute("DELETE FROM gmportals WHERE id = ?", (id_,))
        fm.audit(db, "delete", "gmportals", {"id": id_}, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"deleted {back} portals, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if fm.table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 1
        rows, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(rows)} portals would be made, switched on (--apply makes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            n = apply(db, rows)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} portals switched on, each in gm_audit and {LOG}; live within 30 s, no restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
