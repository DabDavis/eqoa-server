#!/usr/bin/env python3
"""Goad at the knights' and warriors' vendors on Live (Live petition 5370, 2026-11-20: "Pallys Missing a 55 Abil off
vendor ... there is only 1 55 to be bought from vendor", then "Missing Goad").

Goad (8704, warrior, paladin and shadow knight, level 55) is sold on Heirloom by the Spell Archivists alone, and Live has
none, so no one on Live sold it. It is put in the stock of the Live vendors who sell those classes' other level-55
abilities: Merchant Gable (the paladins'; Zealous Defender), Merchant Harriston and Merchant Terk (the warriors'; Ankle
Biter), Merchant Scurver (the shadow knights'; Draw Strength, Paincleave), last in each list. Its value, 0 (never priced:
no merchant sold it), becomes 3,086, Minotaur Form's (the other ability these three classes share at that level) and
the paladins' level-55 price.

Refused unless the four vendors are found by row and name and none stocks Goad. Report only by default; --apply in one
transaction, a gm_audit line a row, logged in goad_5370_log; --undo exact.

WHEN IT TAKES EFFECT: a merchant's stock and prices are read at each window; the record at the next login.

    tools/goad_vendors_5370_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "goad_5370_log"
DETAIL = "petition 5370: Goad at the knights' and warriors' vendors: tools/goad_vendors_5370_20261120.py"
WHO, VIA = "claude", "tool"
GOAD, GOAD_NAME, PRICE = 8704, "Goad", 3086
VENDORS = {105836: "Merchant Gable", 579: "Merchant Harriston", 101694: "Merchant Terk", 599: "Merchant Scurver"}


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
    g = db.execute("SELECT itemname, itemcost FROM itempattern WHERE patternid = ?", (GOAD,)).fetchone()
    if g is None or g[0] != GOAD_NAME:
        refused.append(f"{GOAD} is {g and g[0]!r}, not {GOAD_NAME!r}")
    for nid, name in VENDORS.items():
        r = db.execute("SELECT npc_name FROM npcs WHERE id = ?", (nid,)).fetchone()
        if r is None or r[0] != name:
            refused.append(f"npcs {nid} is {r and r[0]!r}, not {name!r}")
        elif db.execute("SELECT COUNT(*) FROM npcitem WHERE npcid = ? AND patternid = ?", (nid, GOAD)).fetchone()[0]:
            refused.append(f"{name} stocks Goad already")
    return g, refused


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
            rows = db.execute(f"SELECT tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
            for tbl, key, before, after in rows:
                key, before, after = json.loads(key), json.loads(before) if before else None, json.loads(after)
                if tbl == "npcitem":
                    now = db.execute("SELECT npcid, patternid FROM npcitem WHERE itemid = ?", (key["itemid"],)).fetchone()
                    if now != (after["npcid"], after["patternid"]):
                        raise SystemExit(f"REFUSED: npcitem {key['itemid']} changed since")
                else:
                    now = db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (GOAD,)).fetchone()
                    if now != (after["itemcost"],):
                        raise SystemExit("REFUSED: Goad's value changed since")
            for tbl, key, before, after in rows:
                key, before, after = json.loads(key), json.loads(before) if before else None, json.loads(after)
                if tbl == "npcitem":
                    db.execute("DELETE FROM npcitem WHERE itemid = ?", (key["itemid"],))
                    audit(db, "delete", tbl, key, after, None, "undo: " + DETAIL)
                else:
                    db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (before["itemcost"], GOAD))
                    audit(db, "update", tbl, key, after, before, "undo: " + DETAIL)
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"put back {len(rows)}")
            return 0
        goad, refused = plan(db)
        for r in refused:
            print("REFUSED:", r)
        if refused:
            db.execute("ROLLBACK")
            done = any("stocks Goad already" in r for r in refused) and table(db, LOG)
            print("nothing to do" if done else "")
            return 0 if done else 1
        print(f"Goad ({GOAD}) to {', '.join(VENDORS.values())}; its value {goad[1]} -> {PRICE}")
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT)")
        for nid in VENDORS:
            last = db.execute("SELECT COALESCE(MAX(listnumber), -1) FROM npcitem WHERE npcid = ?", (nid,)).fetchone()[0]
            row = {"npcid": nid, "stackleft": 1, "remainHP": 0, "remaincharge": 0, "patternid": GOAD, "equiploc": -1,
                   "location": 1, "listnumber": last + 1}
            cur = db.execute(f"INSERT INTO npcitem ({', '.join(row)}) VALUES ({', '.join('?' for _ in row)})", list(row.values()))
            key = {"itemid": cur.lastrowid}
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('npcitem', ?, NULL, ?)", (json.dumps(key), json.dumps(row)))
            audit(db, "insert", "npcitem", key, None, dict(row, itemid=cur.lastrowid), DETAIL)
        if goad[1] != PRICE:
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (PRICE, GOAD))
            key = {"patternid": GOAD}
            db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('itempattern', ?, ?, ?)",
                       (json.dumps(key), json.dumps({"itemcost": goad[1]}), json.dumps({"itemcost": PRICE})))
            audit(db, "update", "itempattern", key, {"itemcost": goad[1]}, {"itemcost": PRICE}, DETAIL)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
