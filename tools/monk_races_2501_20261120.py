#!/usr/bin/env python3
"""Monk-only items and monk scrolls any race's (Heirloom petition 2501, 2026-11-20: "Mirewalker Robe currently Human/monk
only. Should be All/Monk since all classes can be Monk on this server").

Every record whose class mask is the monk's alone (16) still carried the human-only race mask (1), as the original game
had it when only humans were monks: here every race may be a monk (the monk start, 2026-11-17), so a non-human monk could
wear none of its own gear or scribe its own scrolls. Each such record's race mask becomes every race's (32767, the mask
8,549 records already carry); nothing else changes. Records of other class masks are untouched.

Both shards, each by its own records. Report only by default; --apply in one transaction, a gm_audit line a record,
logged in monk_races_2501_log; a second --apply finds nothing; --undo puts back each record still as written.

WHEN IT TAKES EFFECT: item records go to a client at its login.

    tools/monk_races_2501_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "monk_races_2501_log"
DETAIL = "petition 2501: monk-only records any race's: tools/monk_races_2501_20261120.py"
WHO, VIA = "claude", "tool"
MONK, HUMAN, ALL = 16, 1, 32767


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, pid, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'itempattern', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, go_json({"patternid": pid}), go_json(before), go_json(after), detail))


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
            back = left = 0
            for pid, old, new in db.execute(f"SELECT patternid, old, new FROM {LOG}").fetchall():
                now = db.execute("SELECT raceuse FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
                if now is None or now[0] != new:
                    left += 1
                    print(f"  left: {pid}, changed since")
                    continue
                db.execute("UPDATE itempattern SET raceuse = ? WHERE patternid = ?", (old, pid))
                audit(db, pid, {"raceuse": new}, {"raceuse": old}, "undo: " + DETAIL)
                db.execute(f"DELETE FROM {LOG} WHERE patternid = ?", (pid,))
                back += 1
            if left == 0:
                db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"put back {back}, {left} left")
            return 0 if left == 0 else 1
        todo = db.execute("SELECT patternid, itemname FROM itempattern WHERE classuse = ? AND raceuse = ? ORDER BY itemname",
                          (MONK, HUMAN)).fetchall()
        for pid, name in todo:
            print(f"  {pid} {name}")
        if not todo or not a.apply:
            db.execute("ROLLBACK")
            print("nothing to do" if not todo else f"{len(todo)} records: report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (patternid INTEGER PRIMARY KEY, old INTEGER NOT NULL, new INTEGER NOT NULL)")
        for pid, _ in todo:
            db.execute("UPDATE itempattern SET raceuse = ? WHERE patternid = ?", (ALL, pid))
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?)", (pid, HUMAN, ALL))
            audit(db, pid, {"raceuse": HUMAN}, {"raceuse": ALL}, DETAIL)
        db.execute("COMMIT")
        print(f"applied {len(todo)}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
