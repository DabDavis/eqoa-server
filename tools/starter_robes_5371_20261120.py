#!/usr/bin/env python3
"""The Fayspires starter robes free again on Live (Live petition 5371, 2026-11-20: "Elf Alc Lvl 1 quest item to be
purchased from Initiate Firebringer costs 394 tunar. Should be zero?").

Every Fayspires level-1 quest of the alchemist, enchanter and magician (100001, 100021, 100031) asks a new character,
who starts with no tunar, to show a robe bought from Initiate Firebringer. The robes had no value (Heirloom's still have
none, and sell for 0), and tools/price_zero_items_20261115.py priced them as gear on Live (394 each): its "quest"
exemption covers only what is not gear. Their value goes back to 0, as on Heirloom; nothing else changes.

Refused unless each record is at the price that tool gave it (394). Report only by default; --apply in one transaction,
a gm_audit line a record, logged in starter_robes_5371_log; --undo puts back each one still as written.

WHEN IT TAKES EFFECT: item records go to a client at its login; a merchant's price is read at each window.

    tools/starter_robes_5371_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "starter_robes_5371_log"
DETAIL = "petition 5371: the Fayspires starter robes free again, as on Heirloom: tools/starter_robes_5371_20261120.py"
WHO, VIA = "claude", "tool"
ROBES = {700168: "Thick Blue Uniform", 5189: "Blue Student's Robe", 700154: "Red Student's Robe"}
OLD, NEW = 394, 0


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
                now = db.execute("SELECT itemcost FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
                if now is None or now[0] != new:
                    left += 1
                    print(f"  left: {pid}, changed since")
                    continue
                db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (old, pid))
                audit(db, pid, {"itemcost": new}, {"itemcost": old}, "undo: " + DETAIL)
                db.execute(f"DELETE FROM {LOG} WHERE patternid = ?", (pid,))
                back += 1
            if left == 0:
                db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"put back {back}, {left} left")
            return 0 if left == 0 else 1
        todo, refused = [], []
        for pid, name in ROBES.items():
            r = db.execute("SELECT itemname, itemcost FROM itempattern WHERE patternid = ?", (pid,)).fetchone()
            if r is None or r[0] != name:
                refused.append(f"{pid} is {r and r[0]!r}, not {name!r}")
            elif r[1] == OLD:
                todo.append(pid)
            elif r[1] != NEW:
                refused.append(f"{name} costs {r[1]}, not {OLD}")
        for r in refused:
            print("REFUSED:", r)
        if refused:
            db.execute("ROLLBACK")
            return 1
        for pid in todo:
            print(f"  {pid} {ROBES[pid]}: {OLD} -> {NEW}")
        if not todo or not a.apply:
            db.execute("ROLLBACK")
            print("nothing to do" if not todo else "report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (patternid INTEGER PRIMARY KEY, old INTEGER NOT NULL, new INTEGER NOT NULL)")
        for pid in todo:
            db.execute("UPDATE itempattern SET itemcost = ? WHERE patternid = ?", (NEW, pid))
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?)", (pid, OLD, NEW))
            audit(db, pid, {"itemcost": OLD}, {"itemcost": NEW}, DETAIL)
        db.execute("COMMIT")
        print(f"applied {len(todo)}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
