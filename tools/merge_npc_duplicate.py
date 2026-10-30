#!/usr/bin/env python3
"""One NPC where two of the same name stood: the row kept takes the other's place, and the other is
deleted (the user, 2026-10-30: "guard Syon in neriak has a duplicate, one is naked the other towards
the coach, should be where the naked one is": Guard Syon 1630, dressed, by the coach, and 2983, no
gear, where he belongs).

    tools/merge_npc_duplicate.py --keep 1630 --at 2983 [--db eqoa.db] [--apply]
    tools/merge_npc_duplicate.py --undo [--db eqoa.db]

--keep is the row that stays (its body, gear, level and the rest), --at the row whose place
(x, y, z, facing, world, zone) it takes and which is then deleted. Both must be the same name, case
aside. Both rows are logged whole in npc_merge_log; --undo puts the deleted row back with its own id
and the kept one where it stood, each only if the kept one is still where this left it. Report only
by default. The server reads npcs at start: apply with it stopped, or restart it after.
"""

import argparse
import json
import sqlite3
import sys

PLACE = ("x_coord", "y_coord", "z_coord", "facing", "world", "zone")

LOG_DDL = """CREATE TABLE IF NOT EXISTS npc_merge_log (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    kept INTEGER NOT NULL,
    gone INTEGER NOT NULL,
    kept_before TEXT NOT NULL,
    gone_row TEXT NOT NULL
)"""


def row(db, rid):
    cur = db.execute("SELECT * FROM npcs WHERE id = ?", (rid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([c[0] for c in cur.description], r))


def plan(db, keep, at):
    k, a = row(db, keep), row(db, at)
    if k is None or a is None:
        sys.exit(f"refused: npcs {keep if k is None else at} is not here")
    if k["npc_name"].strip().lower() != a["npc_name"].strip().lower():
        sys.exit(f"refused: {k['npc_name']!r} and {a['npc_name']!r} are not one NPC's name")
    return k, a


def apply(db, k, a):
    db.execute(LOG_DDL)
    db.execute("INSERT INTO npc_merge_log (kept, gone, kept_before, gone_row) VALUES (?, ?, ?, ?)",
               (k["id"], a["id"], json.dumps(k), json.dumps(a)))
    db.execute(f"UPDATE npcs SET {', '.join(f'{c} = ?' for c in PLACE)} WHERE id = ?",
               [a[c] for c in PLACE] + [k["id"]])
    db.execute("DELETE FROM npcs WHERE id = ?", (a["id"],))


def undo(db):
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'npc_merge_log'").fetchone()[0] == 0:
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for kid, gid, kb, gr in db.execute("SELECT kept, gone, kept_before, gone_row FROM npc_merge_log ORDER BY seq DESC").fetchall():
        kb, gr = json.loads(kb), json.loads(gr)
        now = row(db, kid)
        if now is None or any(now[c] != gr[c] for c in PLACE) or row(db, gid) is not None:
            kept += 1
            continue
        db.execute(f"UPDATE npcs SET {', '.join(f'{c} = ?' for c in PLACE)} WHERE id = ?", [kb[c] for c in PLACE] + [kid])
        cols = list(gr)
        db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [gr[c] for c in cols])
        back += 1
    db.execute("DROP TABLE npc_merge_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--keep", type=int)
    ap.add_argument("--at", type=int)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    if a.keep is None or a.at is None:
        sys.exit("--keep and --at")
    k, at = plan(db, a.keep, a.at)
    print(f"{k['npc_name']!r}: {k['id']} (kept) from ({k['x_coord']:.1f}, {k['z_coord']:.1f}) to "
          f"{at['id']}'s ({at['x_coord']:.1f}, {at['z_coord']:.1f}); {at['id']} deleted")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    apply(db, k, at)
    db.execute("COMMIT")
    print("merged; restart the server for the world to have it")


if __name__ == "__main__":
    main()
