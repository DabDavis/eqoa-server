#!/usr/bin/env python3
"""The Rathe gates land beside the druid stones, not in them (Heirloom petition 2507, 2026-11-20: "Wizards Rathe Mountain
port teleports you inside a tree").

The wizard's Rathe Mountains Portal and the druid's Circle of Rathe land at (6251.0, 65.2, 2703.0) in world 1, Cyclops
Fortress's druid stones on their hill. Asked of the server's own collision (ground.Embedded, the move check's ball, on
the baked world 1), that spot is inside something placed; a ring 4 out is clear but for one side, and 6 out clear all
round. Both land 6 east, on the floor there: (6257.0, 65.2, 2703.0). Nothing else of either row changes.

Both shards. Refused unless each row lands at the old spot (or the new: done). Report only by default; --apply in one
transaction, a gm_audit line a row, logged in rathe_gate_2507_log; --undo exact.

WHEN IT TAKES EFFECT: spellgates is read at the server's start (and at a GM panel edit of it).

    tools/rathe_gate_2507_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "rathe_gate_2507_log"
DETAIL = "petition 2507: the Rathe gates land beside the druid stones: tools/rathe_gate_2507_20261120.py"
WHO, VIA = "claude", "tool"
SPELLS = ("rathe mountains portal", "circle of rathe")
OLD, NEW = (1, 6251.0, 65.2, 2703.0), (1, 6257.0, 65.2, 2703.0)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, spell, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'spellgates', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, go_json({"spell": spell}), go_json(before), go_json(after), detail))


def place(db, spell):
    r = db.execute("SELECT world, x, y, z FROM spellgates WHERE spell = ?", (spell,)).fetchone()
    return None if r is None else (r[0], round(r[1], 1), round(r[2], 1), round(r[3], 1))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        keys = ("world", "x", "y", "z")
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            rows = db.execute(f"SELECT spell, before, after FROM {LOG}").fetchall()
            for spell, before, after in rows:
                if list(place(db, spell) or ()) != json.loads(after):
                    db.execute("ROLLBACK")
                    print(f"REFUSED: {spell} changed since")
                    return 1
            for spell, before, after in rows:
                b = json.loads(before)
                db.execute("UPDATE spellgates SET world = ?, x = ?, y = ?, z = ? WHERE spell = ?", (*b, spell))
                audit(db, spell, dict(zip(keys, json.loads(after))), dict(zip(keys, b)), "undo: " + DETAIL)
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"put back {len(rows)}")
            return 0
        todo = []
        for spell in SPELLS:
            p = place(db, spell)
            if p == NEW:
                continue
            if p != OLD:
                db.execute("ROLLBACK")
                print(f"REFUSED: {spell} lands at {p}, not {OLD}")
                return 1
            todo.append(spell)
        if not todo:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        for s in todo:
            print(f"  {s}: {OLD} -> {NEW}")
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (spell TEXT PRIMARY KEY, before TEXT NOT NULL, after TEXT NOT NULL)")
        for s in todo:
            db.execute("UPDATE spellgates SET world = ?, x = ?, y = ?, z = ? WHERE spell = ?", (*NEW, s))
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?)", (s, json.dumps(OLD), json.dumps(NEW)))
            audit(db, s, dict(zip(keys, OLD)), dict(zip(keys, NEW)), DETAIL)
        db.execute("COMMIT")
        print(f"applied {len(todo)}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
