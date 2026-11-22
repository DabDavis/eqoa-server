#!/usr/bin/env python3
"""Kappas drop junk pants, not the level 5 Cloth Pants (Heirloom petition 2620, 2026-11-22: "Kappas dropping a
level 5 cloth pants that hog my inventory space, seems like a placeholder or maybe it should be a stack?"; the owner
chose a stackable junk item).

The guide audit of 2026-11-21 (guide_loot_20261121.py) gave the kappa guards and workers the guide's "cloth pants",
a junk item to sell, as table "guide1121 cloth pants" holding record 7264, the real level 5 Cloth Pants (worn, one a
slot). This makes "Tattered Cloth Pants", a copy of the same audit's junk "small coil of rope" (no slot, a stack of
999, no level) with the pants' icon and value, and points that table at it. Record 7264 is untouched.

Report only by default; --apply writes it in one transaction, a gm_audit line a row, logged in kappa_pants_1122_log;
--undo puts back exactly what it wrote. Refused if a row is not as read.

WHEN IT TAKES EFFECT: loot at each kill; the record at a client's next login.

    python3 tools/kappa_pants_20261122.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "kappa_pants_1122_log"
TABLE = "guide1121 cloth pants"
NAME = "Tattered Cloth Pants"
DESC = "Threadbare pants a kappa wore until they were rags. A merchant might give a few coins for them."
DETAIL = "petition 2620: kappas' junk pants a stack, not the level 5 Cloth Pants: tools/kappa_pants_20261122.py"


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def audit(db, action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now(), action, tbl, json.dumps(key), json.dumps(before) if before is not None else None,
                json.dumps(after) if after is not None else None, DETAIL))


def row(db, sql, args=()):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return dict(zip([c[0] for c in cur.description], r)) if r else None


def plan(db):
    rope = row(db, "SELECT * FROM itempattern WHERE itemname = 'small coil of rope' AND equipslot = -1 AND maxstack > 1")
    pants = row(db, "SELECT * FROM itempattern WHERE patternid = 7264 AND itemname = 'Cloth Pants'")
    lt = row(db, "SELECT * FROM loottables WHERE tablename = ?", (TABLE,))
    if not rope or not pants or not lt:
        sys.exit("refused: the rope, the pants or the table is not as read")
    if lt["patternid"] != 7264:
        sys.exit(f"refused: {TABLE} holds {lt['patternid']}, not 7264")
    if db.execute("SELECT 1 FROM itempattern WHERE itemname = ?", (NAME,)).fetchone():
        sys.exit(f"refused: a record named {NAME} exists")
    new = dict(rope)
    new["patternid"] = max(db.execute("SELECT max(patternid) FROM itempattern WHERE patternid BETWEEN 720000 AND 729999").fetchone()[0] or 720000, 720999) + 1
    new.update(itemname=NAME, itemdesc=DESC, itemicon=pants["itemicon"], itemcost=pants["itemcost"])
    return new, lt


def apply(db):
    new, lt = plan(db)
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, what TEXT, key TEXT, before TEXT)")
    cols = list(new)
    db.execute(f"INSERT INTO itempattern ({','.join(cols)}) VALUES ({','.join('?' * len(cols))})", [new[c] for c in cols])
    audit(db, "insert", "itempattern", {"patternid": new["patternid"]}, None, new)
    db.execute(f"INSERT INTO {LOG} (what, key, before) VALUES ('insert', ?, NULL)", (str(new["patternid"]),))
    db.execute("UPDATE loottables SET patternid = ? WHERE id = ?", (new["patternid"], lt["id"]))
    audit(db, "update", "loottables", {"id": lt["id"]}, {"patternid": 7264}, {"patternid": new["patternid"]})
    db.execute(f"INSERT INTO {LOG} (what, key, before) VALUES ('loot', ?, '7264')", (str(lt["id"]),))
    return new["patternid"]


def undo(db):
    if not db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (LOG,)).fetchone():
        return 0
    n = 0
    for seq, what, key, before in db.execute(f"SELECT seq, what, key, before FROM {LOG} ORDER BY seq DESC").fetchall():
        if what == "loot":
            db.execute("UPDATE loottables SET patternid = ? WHERE id = ?", (int(before), int(key)))
            audit(db, "update", "loottables", {"id": int(key)}, None, {"patternid": int(before)})
        else:
            db.execute("DELETE FROM itempattern WHERE patternid = ?", (int(key),))
            audit(db, "delete", "itempattern", {"patternid": int(key)}, None, None)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        n += 1
    db.execute(f"DROP TABLE {LOG}")
    return n


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            print(f"undo: {undo(db)} rows put back"); db.commit(); return 0
        if db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (LOG,)).fetchone():
            print("already applied (log present)"); return 0
        new, lt = plan(db)
        print(f"would make {NAME} {new['patternid']} and point {TABLE} (row {lt['id']}) at it")
        if a.apply:
            pid = apply(db); db.commit(); print(f"applied: {NAME} {pid}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
