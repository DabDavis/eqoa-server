#!/usr/bin/env python3
"""The leaf covered jerkin a leather tunic (Heirloom petition 2511, 2026-11-20: "leaf covered jerkin is currently listed
as a misc item in the SFG Druid starting quest line. It was Human Druids first quest reward and is a leather tunic").

8490 (Surefall Glade's druid level 1, quest 100687) was a blank record: no wear slot, class, race, level or figures,
so it filed under Misc and could not be worn. It is given its fellows' figures, the other cities' druid level 1
tunics (Rivervale's stormweave tunic 6543, Tethelin's Evergreen Tunic 700670: chest, No Trade, level 1, item HP 200,
AC 4, the druid's class bit, the home's race), as the human druids': race bit 1; and the leather jerkins' look (model
2 and its icon, as Loam Covered and Peasant's Jerkin). Copies held get their item HP (200: a blank record's were 0,
worn out once it is armour).

Both shards. Refused unless the record is as blank as found. Report only by default; --apply in one transaction, a
gm_audit line a row, logged in leaf_jerkin_2511_log; --undo exact.

WHEN IT TAKES EFFECT: item records go to a client at its login.

    tools/leaf_jerkin_2511_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "leaf_jerkin_2511_log"
DETAIL = "petition 2511: the leaf covered jerkin a druid's leather tunic: tools/leaf_jerkin_2511_20261120.py"
WHO, VIA = "claude", "tool"
PID, NAME = 8490, "leaf covered jerkin"
OLD = {"equipslot": -1, "trade": 1, "levelreq": 0, "maxhp": 0, "classuse": 0, "raceuse": 0, "model": 1,
       "itemicon": -1092141747, "AC": 0}
NEW = {"equipslot": 5, "trade": 0, "levelreq": 1, "maxhp": 200, "classuse": 128, "raceuse": 1, "model": 2,
       "itemicon": 430892660, "AC": 4}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, tbl, go_json(key), go_json(before), go_json(after), detail))


def record(db):
    cols = list(OLD)
    r = db.execute(f"SELECT itemname, {', '.join(cols)} FROM itempattern WHERE patternid = ?", (PID,)).fetchone()
    return None if r is None else (r[0], dict(zip(cols, r[1:])))


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
        r = record(db)
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            if r is None or r[1] != NEW:
                db.execute("ROLLBACK")
                print("REFUSED: the record changed since")
                return 1
            db.execute(f"UPDATE itempattern SET {', '.join(f'{k} = ?' for k in OLD)} WHERE patternid = ?", (*OLD.values(), PID))
            audit(db, "itempattern", {"patternid": PID}, NEW, OLD, "undo: " + DETAIL)
            for (itemid,) in db.execute(f"SELECT itemid FROM {LOG}").fetchall():
                db.execute("UPDATE charinventory SET remainHP = 0 WHERE itemid = ? AND remainHP = 200 AND patternid = ?", (itemid, PID))
                audit(db, "charinventory", {"itemid": itemid}, {"remainHP": 200}, {"remainHP": 0}, "undo: " + DETAIL)
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print("put back")
            return 0
        if r is None or r[0] != NAME:
            db.execute("ROLLBACK")
            print(f"REFUSED: {PID} is {r and r[0]!r}, not {NAME!r}")
            return 1
        if r[1] == NEW:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if r[1] != OLD:
            db.execute("ROLLBACK")
            print(f"REFUSED: the record is {r[1]}, not as blank as found")
            return 1
        held = [i for (i,) in db.execute("SELECT itemid FROM charinventory WHERE patternid = ? AND remainHP = 0", (PID,))]
        print(f"{NAME}: {OLD} -> {NEW}; {len(held)} copies held get item HP 200")
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        db.execute(f"UPDATE itempattern SET {', '.join(f'{k} = ?' for k in NEW)} WHERE patternid = ?", (*NEW.values(), PID))
        audit(db, "itempattern", {"patternid": PID}, OLD, NEW, DETAIL)
        db.execute(f"CREATE TABLE {LOG} (itemid INTEGER PRIMARY KEY)")
        for i in held:
            db.execute("UPDATE charinventory SET remainHP = 200 WHERE itemid = ?", (i,))
            db.execute(f"INSERT INTO {LOG} VALUES (?)", (i,))
            audit(db, "charinventory", {"itemid": i}, {"remainHP": 0}, {"remainHP": 200}, DETAIL)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
