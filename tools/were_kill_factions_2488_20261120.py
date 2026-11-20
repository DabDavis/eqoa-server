#!/usr/bin/env python3
"""Every were's kill raises its line's society (Heirloom petition 2488, 2026-11-20: "I can't progress to the tails portion
of the quest as I can't get the faction needed. Neither the Ratwere/wererats in The Warens, Geomancer Citadel, or South
Barren Coast give faction on kill").

tools/fix_were_lines.py gave the lines' plain weres +10 a kill with their society (factionhits, source were-kills: the
user's "Add +10 a kill"), but not the elder, ancient, great and vicious weres of the same lines, which are the ones where
the guide sends a player to be infected (the rat line's Warrens, Geomancer's Citadel, South Barren Coast). Every were
NPC name of a line with no factionhits row of its society gets the same +10: the rat line Topi Di Ombra, the wolf line
Night Reapers, the lion line Sunmanes, the gator line The Bone Crusher, the bear line Wards of Dawn. Named weres (the
Werebear Witchking, the Werehunter) are not of the pattern.

Both shards, each by its own names. Report only by default; --apply in one transaction, a gm_audit line a row, logged in
were_kills_2488_log; a second --apply finds nothing to add; --undo takes out each row still as written (one changed
since is left and said).

WHEN IT TAKES EFFECT: factionhits is read at the server's start.

    tools/were_kill_factions_2488_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import re
import sqlite3
import sys

LOG = "were_kills_2488_log"
DETAIL = "petition 2488: every were's kill raises its line's society by 10: tools/were_kill_factions_2488_20261120.py"
WHO, VIA = "claude", "tool"
SOURCE, DELTA = "were-kills", 10
LINES = {"rat": "Topi Di Ombra", "wolf": "Night Reapers", "lion": "Sunmanes", "gator": "The Bone Crusher", "bear": "Wards of Dawn"}
NAME = re.compile(r"^(?:(?:elder|ancient|great|vicious|feral|dire) )?(?:were(rat|wolf|lion|gator|bear)|(rat|wolf|lion|gator|bear)were)$")


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'factionhits', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, go_json(key), go_json(before), go_json(after), detail))


def missing(db):
    """[(npc_name, faction)] a were of a line lacking its society's row, by name."""
    have = {(n.lower(), f) for n, f in db.execute("SELECT npc_name, faction FROM factionhits")}
    factions = {f for (f,) in db.execute("SELECT name FROM factions")}
    out = []
    for (name,) in db.execute("SELECT DISTINCT npc_name FROM npcs WHERE npc_name LIKE '%were%' ORDER BY npc_name"):
        m = NAME.match(name or "")
        if not m:
            continue
        faction = LINES[m.group(1) or m.group(2)]
        if faction in factions and (name.lower(), faction) not in have:
            out.append((name, faction))
    return out


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
        for t in ("factionhits", "factions", "npcs", "gm_audit"):
            if not table(db, t):
                print(f"REFUSED: no {t} table here")
                db.execute("ROLLBACK")
                return 1
        if a.undo:
            if not table(db, LOG):
                db.execute("ROLLBACK")
                print("nothing to undo")
                return 0
            back = left = 0
            for seq, rid, row in db.execute(f"SELECT seq, id, row FROM {LOG} ORDER BY seq DESC").fetchall():
                row = json.loads(row)
                now = db.execute("SELECT npc_name, faction, delta, source FROM factionhits WHERE id = ?", (rid,)).fetchone()
                if now != (row["npc_name"], row["faction"], row["delta"], row["source"]):
                    left += 1
                    print(f"  left: factionhits {rid} ({row['npc_name']}), changed since")
                    continue
                db.execute("DELETE FROM factionhits WHERE id = ?", (rid,))
                audit(db, "delete", {"id": rid}, dict(row, id=rid), None, "undo: " + DETAIL)
                db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
                back += 1
            if left == 0:
                db.execute(f"DROP TABLE {LOG}")
            db.execute("COMMIT")
            print(f"took out {back}, {left} left")
            return 0 if left == 0 else 1
        todo = missing(db)
        for n, f in todo:
            print(f"  {n}: {f} {DELTA:+d} a kill")
        if not todo or not a.apply:
            db.execute("ROLLBACK")
            print("nothing to do" if not todo else f"{len(todo)} rows: report only (--apply to write)")
            return 0
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, id INTEGER NOT NULL, row TEXT NOT NULL)")
        for n, f in todo:
            row = {"npc_name": n, "faction": f, "delta": DELTA, "source": SOURCE}
            cur = db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)", (n, f, DELTA, SOURCE))
            db.execute(f"INSERT INTO {LOG} (id, row) VALUES (?, ?)", (cur.lastrowid, json.dumps(row, sort_keys=True)))
            audit(db, "insert", {"id": cur.lastrowid}, None, dict(row, id=cur.lastrowid), DETAIL)
        db.execute("COMMIT")
        print(f"applied {len(todo)}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
