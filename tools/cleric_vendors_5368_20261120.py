#!/usr/bin/env python3
"""The cleric's level 51-60 scrolls no one on Live sold, at Live's cleric merchants (Live petition 5368, 2026-11-20:
"lvl 55 or 60 CL heal missing from vender").

Heirloom sells every class's level 50+ spells at its Spell Archivists (2026-11-08, petition 1273), which Live does not
have (tools/shard_sync_20261115.py leaves their stock Heirloom's). Two of the cleric's (classspells) no merchant, loot
table or quest gives on Live, Cure Mystical Plague and Envaric Salve (55); the rest of its 51-60 scrolls drop by design
(its level 51-57 spells off creatures, its level 60 heals Divinity, Influence and Vibrance off Zaoran Plateau). Each
of the two goes last into the stock of Live's six cleric guild merchants (Yornman, Diziria, Marith, Brezzler, Ezzlebip,
Sorendil: 12-17 items each), not the four mixed cleric and druid merchants (Utmar, Jimbra, Gayle, Acquine: 38 items
each; the client's list holds 40). Envaric Salve's value stays (3,500); Cure Mystical Plague's, 0 (never priced),
becomes 4,589, its fellows' (Vital Antidote, Vital Curative).

Refused unless each vendor and record is found by row and name and no vendor stocks any of them. Report only by default;
--apply in one transaction, a gm_audit line a row, logged in cleric_5368_log; --undo exact.

WHEN IT TAKES EFFECT: a merchant's stock and prices are read at each window; the record at the next login.

    tools/cleric_vendors_5368_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "cleric_5368_log"
DETAIL = "petition 5368: the cleric's unsold 51-60 scrolls at Live's cleric merchants: tools/cleric_vendors_5368_20261120.py"
WHO, VIA = "claude", "tool"
# The first apply (2026-11-20 17:55) stocked eight; six are drops by design on Live (Censure Death, Consume
# Affliction and Reclamation off creatures, tables "t303 ... spells"/"uwu 51 spells"; Divinity, Influence and
# Vibrance off Zaoran Plateau, "zp spells"), so it was undone and the two no table or quest gives put back alone
# (the owner's sweep of the same day, tools/live_spell_sweep_20261120.py, found it).
SCROLLS = {8792: "Cure Mystical Plague", 716222: "Envaric Salve"}
PRICES = {8792: (0, 4589)}  # a record's value: (as found, as set)
VENDORS = {1935: "Merchant Yornman", 102305: "Merchant Diziria", 106020: "Merchant Marith", 106181: "Merchant Brezzler",
           106472: "Merchant Ezzlebip", 162430: "Merchant Sorendil"}
LIST_MOST = 40  # the client's merchant list


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))


def plan(db):
    refused = []
    for pid, name in SCROLLS.items():
        r = db.execute("SELECT itemname, itemcost FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
        if r is None or r[0] != name:
            refused.append(f"{pid} is {r and r[0]!r}, not {name!r}")
        elif pid in PRICES and r[1] not in PRICES[pid]:
            refused.append(f"{name} is worth {r[1]}, not {PRICES[pid][0]}")
    for nid, name in VENDORS.items():
        r = db.execute("SELECT npc_name FROM npcs WHERE id = ?", (nid,)).fetchone()
        if r is None or r[0] != name:
            refused.append(f"npcs {nid} is {r and r[0]!r}, not {name!r}")
        else:
            have = db.execute(f"SELECT COUNT(*) FROM npcitem WHERE npcid = ? AND patternid IN ({','.join('?' * len(SCROLLS))})",
                              (nid, *SCROLLS)).fetchone()[0]
            if have:
                refused.append(f"{name} stocks {have} of them already")
            n = db.execute("SELECT COUNT(*) FROM npcitem WHERE npcid = ?", (nid,)).fetchone()[0]
            if n + len(SCROLLS) > LIST_MOST:
                refused.append(f"{name} would list {n + len(SCROLLS)}, past the client's {LIST_MOST}")
    return refused


def undo(db):
    if not table(db, LOG):
        return 0
    rows = db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    for tbl, key, before, after in rows:
        key, after = json.loads(key), json.loads(after)
        if tbl == "npcitem":
            now = db.execute("SELECT npcid, patternid FROM npcitem WHERE itemid = ?", (key["itemid"],)).fetchone()
            if now != (after["npcid"], after["patternid"]):
                raise SystemExit(f"REFUSED: npcitem {key['itemid']} changed since")
        elif db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (key["patternid"],)).fetchone() != (after["itemcost"],):
            raise SystemExit(f"REFUSED: {key['patternid']}'s value changed since")
    for tbl, key, before, after in rows:
        key, before, after = json.loads(key), json.loads(before) if before else None, json.loads(after)
        if tbl == "npcitem":
            db.execute("DELETE FROM npcitem WHERE itemid = ?", (key["itemid"],))
            audit(db, "delete", tbl, key, after, None, "undo: " + DETAIL)
        else:
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (before["itemcost"], key["patternid"]))
            audit(db, "update", tbl, key, after, before, "undo: " + DETAIL)
    db.execute(f"DROP TABLE {LOG}")
    return len(rows)


def apply(db):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT)")
    for nid in VENDORS:
        last = db.execute("SELECT COALESCE(MAX(listnumber), -1) FROM npcitem WHERE npcid = ?", (nid,)).fetchone()[0]
        for pid in SCROLLS:
            last += 1
            row = {"npcid": nid, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": pid, "equiploc": -1,
                   "location": 1, "listnumber": last}
            cur = db.execute(f"INSERT INTO npcitem ({', '.join(row)}) VALUES ({', '.join('?' for _ in row)})", list(row.values()))
            key = {"itemid": cur.lastrowid}
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('npcitem', ?, NULL, ?)", (json.dumps(key), json.dumps(row)))
            audit(db, "insert", "npcitem", key, None, dict(row, itemid=cur.lastrowid), DETAIL)
    for pid, (old, new) in PRICES.items():
        if db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (pid,)).fetchone()[0] != old:
            continue
        db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (new, pid))
        key = {"patternid": pid}
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('itempattern', ?, ?, ?)",
                   (json.dumps(key), json.dumps({"itemcost": old}), json.dumps({"itemcost": new})))
        audit(db, "update", "itempattern", key, {"itemcost": old}, {"itemcost": new}, DETAIL)


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
            n = undo(db)
            db.execute("COMMIT")
            print(f"put back {n}" if n else "nothing to undo")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print("nothing to do (applied: cleric_5368_log holds it)")
            return 0
        refused = plan(db)
        for r in refused:
            print("REFUSED:", r)
        if refused:
            db.execute("ROLLBACK")
            return 1
        print(f"{len(SCROLLS)} scrolls ({', '.join(SCROLLS.values())}) to {', '.join(VENDORS.values())}")
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        apply(db)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
