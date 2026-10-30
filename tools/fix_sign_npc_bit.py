#!/usr/bin/env python3
"""The NPC bit (0x80: never attacked, never attacks) on every sign and announcement board (the
user, 2026-10-30: "fayspires accouncement attacked and killed me").

Halas's, Tethelin's and Fayspires' Announcements (level 65) and two level 1 signposts stand on the
board's body (0x6A82352C, the one every sign and town board has) with npc_type 0, where the other
41 of that body carry 0x80. Without it a board is an NPC of its town's faction, and a town's people
attack a player whose standing is below -350 at any level (entity/aggro.go): Fayspires' board hit a
level 2 for 704 from 28.5 away. Each row of that body without the bit is given it; logged in
sign_npc_bit_log; --undo puts back each row still as this left it. Report only by default. The
server reads npcs at start: apply with it stopped, or restart it after.

    tools/fix_sign_npc_bit.py [--db eqoa.db] [--apply | --undo]
"""

import argparse
import sqlite3

BOARD_BODY = 0x6A82352C
NPC_BIT = 0x80

LOG_DDL = """CREATE TABLE IF NOT EXISTS sign_npc_bit_log (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    rid INTEGER NOT NULL,
    old_type INTEGER,
    new_type INTEGER
)"""


def plan(db):
    return db.execute("SELECT id, npc_name, npc_level, npc_type FROM npcs WHERE (modelid & 0xFFFFFFFF) = ? "
                      "AND (COALESCE(npc_type, 0) & ?) = 0 ORDER BY id", (BOARD_BODY, NPC_BIT)).fetchall()


def apply(db, todo):
    db.execute(LOG_DDL)
    n = 0
    for rid, _, _, old in todo:
        new = (old or 0) | NPC_BIT
        cur = db.execute("UPDATE npcs SET npc_type = ? WHERE id = ? AND npc_type IS ?", (new, rid, old))
        if cur.rowcount:
            db.execute("INSERT INTO sign_npc_bit_log (rid, old_type, new_type) VALUES (?, ?, ?)", (rid, old, new))
            n += 1
    return n


def undo(db):
    if db.execute("SELECT count(*) FROM sqlite_master WHERE name = 'sign_npc_bit_log'").fetchone()[0] == 0:
        print("nothing logged: nothing to undo")
        return
    back = kept = 0
    for rid, old, new in db.execute("SELECT rid, old_type, new_type FROM sign_npc_bit_log ORDER BY seq DESC").fetchall():
        cur = db.execute("UPDATE npcs SET npc_type = ? WHERE id = ? AND npc_type = ?", (old, rid, new))
        back, kept = (back + 1, kept) if cur.rowcount else (back, kept + 1)
    db.execute("DROP TABLE sign_npc_bit_log")
    print(f"put back {back}; left {kept} changed since")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default="eqoa.db")
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
    todo = plan(db)
    for rid, name, lv, t in todo:
        print(f"{rid} {name!r} level {lv}: npc_type {t} -> {(t or 0) | NPC_BIT}")
    print(f"{len(todo)} signs and boards without the NPC bit")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    n = apply(db, todo)
    db.execute("COMMIT")
    print(f"gave {n} the NPC bit; restart the server for the world to have it")


if __name__ == "__main__":
    main()
