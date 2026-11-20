#!/usr/bin/env python3
"""Merchant Samwe by her own name on Live (Live petition 5365, 2026-11-20: "samwe is not progressing quest automatically,
nor is she selling the quest items for most quests involving her").

Live's row 106253 stood as "Merchant Swame", the Return Home import's misspelling (prh_import_npcs); Heirloom's row and
all ten quests' steps (Bal's Staff, Rin's Clockworks, The Windmills of Rivervale, the Klik'Anon staffs and gears) say
"Merchant Samwe", the name EQOA-Data gives, so no step was done at her and none of her hand-overs came. The row is
renamed; nothing else of it changes. Refused unless row 106253 is "Merchant Swame" (or Samwe already: done).
Report only by default; --apply a gm_audit line, logged in samwe_5365_log; --undo exact.

WHEN IT TAKES EFFECT: NPCs are read at the server's start.

    tools/samwe_name_5365_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "samwe_5365_log"
DETAIL = "petition 5365: Merchant Swame renamed Merchant Samwe, as her quests name her: tools/samwe_name_5365_20261120.py"
WHO, VIA = "claude", "tool"
ROW, OLD, NEW = 106253, "Merchant Swame", "Merchant Samwe"


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'npcs', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, go_json({"id": ROW}), go_json(before), go_json(after), detail))


def row(db):
    cur = db.execute("SELECT * FROM npcs WHERE id = ?", (ROW,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


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
        r = row(db)
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            before, after = (json.loads(x) for x in db.execute(f"SELECT before, after FROM {LOG}").fetchone())
            if r != after:
                db.execute("ROLLBACK")
                print("REFUSED: row 106253 changed since it was written")
                return 1
            db.execute("UPDATE npcs SET npc_name = ? WHERE id = ?", (before["npc_name"], ROW))
            audit(db, after, before, "undo: " + DETAIL)
            db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print("put back")
            return 0
        if r is None or r["npc_name"] not in (OLD, NEW):
            db.execute("ROLLBACK")
            print(f"REFUSED: row {ROW} is {r and r['npc_name']!r}, not {OLD!r}")
            return 1
        if r["npc_name"] == NEW:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        print(f"row {ROW}: {OLD!r} -> {NEW!r}")
        if not a.apply:
            db.execute("ROLLBACK")
            print("report only (--apply to write)")
            return 0
        db.execute("UPDATE npcs SET npc_name = ? WHERE id = ?", (NEW, ROW))
        after = row(db)
        db.execute(f"CREATE TABLE {LOG} (before TEXT NOT NULL, after TEXT NOT NULL)")
        db.execute(f"INSERT INTO {LOG} VALUES (?, ?)", (json.dumps(r), json.dumps(after)))
        audit(db, r, after, DETAIL)
        db.execute("COMMIT")
        print("applied")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
