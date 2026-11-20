#!/usr/bin/env python3
"""Grundle's cave's elementals are creatures (Heirloom petition 2508, 2026-11-20: "The elementals in Grundle's cave
should all be aggressive").

The cave's Earth, Fire and Water Elementalkin (world 1, about (5672, 2945)) stand alone, no one's pet, but the seed's
npcaggro rows for the magicians' pets' names ("a player's pet recorded beside its owner") kept them peaceful. Since
this petition a GM's npcfaction place row naming a pet's name (entity/aggro.go aggroRuleLocked) says the ones of that
name standing there are creatures, their nature deciding: a row for each, world 1, within 60 of (5672, 2945), of no
faction. Elementalkin anywhere else (a player's pet, a recorded one) are as they were.

Both shards (the cave is the same). Refused unless each of the three stands there and no npcfaction row names it.
Report only by default; --apply in one transaction, a gm_audit line a row, logged in grundle_2508_log; --undo exact.

WHEN IT TAKES EFFECT: npcfaction is read at the server's start (and a GM's edit of the factions); the rule needs the
binary that brings it.

    tools/grundle_elementals_2508_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "grundle_2508_log"
DETAIL = "petition 2508: Grundle's cave's elementals are creatures, not pets: tools/grundle_elementals_2508_20261120.py"
WHO, VIA = "claude", "tool"
NAMES = ("Earth Elementalkin", "Fire Elementalkin", "Water Elementalkin")
WORLD, X, Z, R = 1, 5672.0, 2945.0, 60.0
NOTE = "petition 2508: Grundle's cave's elementals are creatures (no faction): the seed's pet-name rule does not hold here"


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'npcfaction', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, go_json(key), go_json(before), go_json(after), detail))


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
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            rows = db.execute(f"SELECT id, row FROM {LOG}").fetchall()
            for rid, row in rows:
                r = json.loads(row)
                now = db.execute("SELECT name, world, x, z, radius, faction FROM npcfaction WHERE id = ?", (rid,)).fetchone()
                if now != (r["name"], r["world"], r["x"], r["z"], r["radius"], r["faction"]):
                    db.execute("ROLLBACK")
                    print(f"REFUSED: npcfaction {rid} changed since")
                    return 1
            for rid, row in rows:
                db.execute("DELETE FROM npcfaction WHERE id = ?", (rid,))
                audit(db, "delete", {"id": rid}, dict(json.loads(row), id=rid), None, "undo: " + DETAIL)
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"took out {len(rows)}")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print("nothing to do (applied: grundle_2508_log holds it)")
            return 0
        refused = []
        for n in NAMES:
            here = db.execute("SELECT COUNT(*) FROM npcs WHERE npc_name = ? AND world = ? AND (x_coord - ?) * (x_coord - ?) + "
                              "(z_coord - ?) * (z_coord - ?) <= ? * ?", (n, WORLD, X, X, Z, Z, R, R)).fetchone()[0]
            if here == 0:
                refused.append(f"no {n} stands within {R:g} of ({X:g}, {Z:g}) in world {WORLD}")
            if db.execute("SELECT COUNT(*) FROM npcfaction WHERE name = ? COLLATE NOCASE", (n,)).fetchone()[0]:
                refused.append(f"npcfaction names {n} already")
        for r in refused:
            print("REFUSED:", r)
        if refused:
            db.execute("ROLLBACK")
            return 1
        print(f"npcfaction place rows (no faction) for {', '.join(NAMES)}: world {WORLD}, within {R:g} of ({X:g}, {Z:g})")
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE {LOG} (id INTEGER PRIMARY KEY, row TEXT NOT NULL)")
        for n in NAMES:
            row = {"name": n, "world": WORLD, "x": X, "z": Z, "radius": R, "faction": "", "note": NOTE, "source": "gm"}
            cur = db.execute("INSERT INTO npcfaction (name, world, x, z, radius, faction, note, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                             tuple(row.values()))
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?)", (cur.lastrowid, json.dumps(row)))
            audit(db, "insert", {"id": cur.lastrowid}, None, dict(row, id=cur.lastrowid), DETAIL)
        db.execute("COMMIT")
        print(f"applied {len(NAMES)}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
