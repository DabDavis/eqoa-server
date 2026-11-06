#!/usr/bin/env python3
"""The starting rations (the user, 2026-11-06: "lets increase players starting rations to scale with level, and doesnt
take too long to restore. Also start them with 99. The heirloom vendor sells more."). The healing itself is the
server's (handlers/consume.go, entity.RationRestore: a tenth of the eater's most health and power each regen tick, a
minute); this writes the data beside it:
- **the ten ration records** (CLIENT's usable type, worn nowhere, named a ration, cuisine or snacks, and some race's
  starting item): a stack of 99 (they were 20, so 99 would have been five records of a 40-record pack) and worth 0
  tunar, so the outfitters give them for nothing and no merchant buys free rations back;
- **startitems**: every ration row 99 (it was 20); a new character starts with one stack of 99;
- **the Heirloom Outfitters sell the rations** of the races whose new characters start beside them (defaultcharacter's
  spots within 60 of the outfitter, their race and class's startitems), Field Rations where none starts near; the
  buyer picks how many, one stack at most.
Data only, no restart: a client reads the records at its next login, the merchants and startitems are read at each
use. Logged in rations_fix_log; --undo puts back exactly what the run changed and deletes what it added.

    tools/fix_rations.py [--db eqoa.db] [--list] [--apply | --undo]
"""

import argparse
import datetime
import json
import math
import os
import re
import sqlite3

LOG = "rations_fix_log"
STACK = 99
NEAR = 60.0
FIELD_RATIONS = 8398
RATION = re.compile(r"(?i)\b(rations?|cuisine|snacks)\b")
MEAL = re.compile(r"(?i)\bmeal\b.*\bhealth regeneration\b")
DRINK = re.compile(r"(?i)\bbeverage\b.*\bpower regeneration\b")


def rows_of(db, sql, *args):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def ensure(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   rowkey INTEGER NOT NULL, action TEXT NOT NULL, old TEXT NOT NULL DEFAULT '')""")


def rations(db):
    """The ration records: handlers/consume.go's isRation, and some race's starting item."""
    out = []
    for r in rows_of(db, "SELECT patternid, itemname, itemdesc, equipslot, procanim, maxstack, itemcost FROM itempattern "
                         "WHERE patternid IN (SELECT DISTINCT patternid FROM startitems)"):
        if r["equipslot"] == -1 and r["procanim"] == 2 and RATION.search(r["itemname"] or "") and \
                not MEAL.search(r["itemdesc"] or "") and not DRINK.search(r["itemdesc"] or ""):
            out.append(r)
    return out


def plan(db):
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("the rations are fixed in this database already: --undo first")
    recs = rations(db)
    ids = {r["patternid"] for r in recs}
    if FIELD_RATIONS not in ids:
        raise SystemExit("no Field Rations among the rations")
    records = [r for r in recs if r["maxstack"] != STACK or r["itemcost"] != 0]
    starts = rows_of(db, f"SELECT id, race, class, humtype, patternid, count FROM startitems WHERE patternid IN "
                         f"({','.join('?' * len(ids))}) AND count != ?", *sorted(ids), STACK)
    # the outfitters and the rations of the races who start beside each
    by_combo = {}
    for race, cls, hum, pid in db.execute(f"SELECT race, class, humtype, patternid FROM startitems WHERE patternid IN "
                                          f"({','.join('?' * len(ids))})", sorted(ids)):
        by_combo.setdefault((race, cls, hum), set()).add(pid)
    spots = rows_of(db, "SELECT race, tclass, humType, x, z, world FROM defaultcharacter")
    sells = []
    for o in rows_of(db, "SELECT id, npc_id, world, x_coord, z_coord FROM npcs WHERE npc_name = 'Heirloom Outfitter' ORDER BY id"):
        near = set()
        for s in spots:
            if s["world"] == o["world"] and math.hypot(s["x"] - o["x_coord"], s["z"] - o["z_coord"]) <= NEAR:
                near |= by_combo.get((s["race"], s["tclass"], s["humType"]), set()) | \
                        by_combo.get((s["race"], s["tclass"], 0), set())
        near = near or {FIELD_RATIONS}
        have = {pid for (pid,) in db.execute("SELECT patternid FROM npcitem WHERE npcid = ?", (o["npc_id"],))}
        last = db.execute("SELECT COALESCE(MAX(listnumber), -1) FROM npcitem WHERE npcid = ?", (o["npc_id"],)).fetchone()[0]
        for pid in sorted(near - have):
            last += 1
            sells.append((o["npc_id"], pid, last))
    return dict(rations=recs, records=records, starts=starts, sells=sells)


def report(p, full):
    names = {r["patternid"]: r["itemname"] for r in p["rations"]}
    print(f"{len(p['rations'])} rations ({', '.join(sorted(names.values()))}); {len(p['records'])} records to a stack of "
          f"{STACK} worth 0; {len(p['starts'])} startitems rows to {STACK}; {len(p['sells'])} outfitter stock rows")
    if full:
        for npc, pid, n in p["sells"]:
            print(f"  outfitter {npc} sells {names.get(pid, pid)} ({pid}) as item {n}")


def apply(db, p):
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    try:
        log = lambda tbl, key, action, old="": db.execute(
            f"INSERT INTO {LOG} (run, tbl, rowkey, action, old) VALUES (?, ?, ?, ?, ?)", (run, tbl, key, action, old))
        for r in p["records"]:
            log("itempattern", r["patternid"], "update", json.dumps({"maxstack": r["maxstack"], "itemcost": r["itemcost"]}))
            db.execute("UPDATE itempattern SET maxstack = ?, itemcost = 0 WHERE patternid = ?", (STACK, r["patternid"]))
        for s in p["starts"]:
            log("startitems", s["id"], "update", json.dumps({"count": s["count"]}))
            db.execute("UPDATE startitems SET count = ? WHERE id = ?", (STACK, s["id"]))
        for npc, pid, n in p["sells"]:
            cur = db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                             "VALUES (?, 1, ?, 0, 1, ?)", (npc, pid, n))
            log("npcitem", cur.lastrowid, "insert")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote it (run {run}): no restart needed")


def undo(db):
    rows = db.execute(f"SELECT tbl, rowkey, action, old FROM {LOG} ORDER BY seq DESC").fetchall()
    if not rows:
        raise SystemExit("nothing to undo")
    keys = {"itempattern": "patternid", "startitems": "id"}
    db.execute("BEGIN IMMEDIATE")
    try:
        for tbl, key, action, old in rows:
            if action == "insert":
                db.execute("DELETE FROM npcitem WHERE itemid = ?", (key,))
            else:
                vals = json.loads(old)
                db.execute(f"UPDATE {tbl} SET {', '.join(f'{k} = ?' for k in vals)} WHERE {keys[tbl]} = ?",
                           list(vals.values()) + [key])
        db.execute(f"DELETE FROM {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {len(rows)} rows")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args()
    if not os.path.isfile(a.db):
        raise SystemExit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=10)
    if (a.apply or a.undo) and not a.no_backup:
        stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        db.execute(f"VACUUM INTO '{a.db}.bak-{stamp}-pre-rations'")
    ensure(db)
    if a.undo:
        undo(db)
        return
    p = plan(db)
    report(p, a.list)
    if a.apply:
        apply(db, p)


if __name__ == "__main__":
    main()
