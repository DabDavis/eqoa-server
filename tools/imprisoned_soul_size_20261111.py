#!/usr/bin/env python3
"""The Isle of Dread's oversized imprisoned soul drawn at its siblings' size (2026-11-11; the owner: "imprisoned soul in
Isle of Dread is scaled way too big").

  npcs 118952 "imprisoned soul" (world 0, zone 1125, at 24963, 2694): size 2.9375 -> 1.0.

Why 1.0: the camp's other 15 imprisoned souls (zones 1125/1126, the same body 647210326, level 53) are all 1.0; every
other NPC on that body is 1.0 to 1.25 (Graumord citizens 1.0 x20, glyphed familiars 1.0, animated guards 1.0, the
marionettes 1.0-1.75, Kithgrin 1.25); the soul and ghost bodies elsewhere average 1.0-1.17 and top out at 1.875. The
2.9375 came with jib's Return Home import (prh_import_npcs 18952) and is that row's alone. Its template (iod native)
grows nothing (no enrage).

Refused, nothing written, when the row is not as expected (missing, or a size other than 2.9375 or 1.0). Report only by
default; --apply in one transaction, a gm_audit line, log imprisoned_soul_size_1111_log; --undo exact (a row changed
since left and said). NPCs are read at the start: live after a restart (or the same edit made in the GM panel's npcs).

    tools/imprisoned_soul_size_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "imprisoned_soul_size_1111_log"
ROW_ID = 118952
NAME = "imprisoned soul"
OLD, NEW = 2.9375, 1.0
WHO, VIA = "dab", "tool"
DETAIL = "the IoD imprisoned soul at its siblings' size 1.0 (the owner, 2026-11-11): tools/imprisoned_soul_size_20261111.py"

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    id     INTEGER NOT NULL,
    before REAL NOT NULL,
    after  REAL NOT NULL,
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def audit(db, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'npcs', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, json.dumps({"id": ROW_ID}),
                      json.dumps({"size": before}), json.dumps({"size": after}), DETAIL))
    return cur.lastrowid


def plan(db):
    """(lines, refusals, todo)."""
    r = db.execute("SELECT npc_name, world, size FROM npcs WHERE id = ?", (ROW_ID,)).fetchone()
    if r is None:
        return [], [f"npcs {ROW_ID}: no such row"], False
    name, world, size = r
    if name != NAME or world != 0:
        return [], [f"npcs {ROW_ID}: {name!r} world {world}, want {NAME!r} world 0"], False
    sib = db.execute("SELECT count(*), min(size), max(size) FROM npcs WHERE npc_name = ? AND world = 0 AND id != ?",
                     (NAME, ROW_ID)).fetchone()
    lines = [f"npcs {NAME!r} world 0, the others: {sib[0]}, size {sib[1]}-{sib[2]}"]
    if size == NEW:
        return lines + [f"npcs {ROW_ID}: size {NEW:g} already"], [], False
    if size != OLD:
        return lines, [f"npcs {ROW_ID}: size {size}, want {OLD:g}"], False
    return lines + [f"npcs {ROW_ID}: size {OLD:g} -> {NEW:g}"], [], True


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, rid, before, after, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE npcs SET size = ? WHERE id = ? AND size = ?", (before, rid, after))
        if cur.rowcount != 1:
            print(f"left: npcs {rid} changed since")
            left += 1
            continue
        audit(db, after, before)
        back += 1
    if not left:
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
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        lines, refusals, todo = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not todo:
            print("nothing to do")
            return 0
        if not a.apply:
            print("report only: 1 row would be written (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            cur = db.execute("UPDATE npcs SET size = ? WHERE id = ? AND size = ?", (NEW, ROW_ID, OLD))
            if cur.rowcount != 1:
                raise RuntimeError("the row changed under the update")
            a_id = audit(db, OLD, NEW)
            db.execute(LOG_SCHEMA)
            db.execute(f"INSERT INTO {LOG} (id, before, after, audit) VALUES (?, ?, ?, ?)", (ROW_ID, OLD, NEW, a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: 1 row, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
