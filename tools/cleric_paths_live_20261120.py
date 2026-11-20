#!/usr/bin/env python3
"""Board #1686's cleric path quests on a shard that never had them (the owner, 2026-11-20: "lets sync heirloom and live if one
is more up to date").

A GM rewrote the cleric's path rewards on one shard on 2026-11-18 (board #1686: the era's two paths, heal and resist; the
"Elemental" line gone from the level 20, 30 and 40 quests; the level 49 quests no longer giving every cleric First Aid as the
step's own spell), one audited row edit at a time, and the other shard kept the old rows. This tool makes the same edits there,
from `cleric_paths_live_20261120.json`: the GM's audit lines' before and after, one entry a quest step, matched by questid,
step, target and journal. The level 49 rows' choices were already the same on both shards (a later edit), so only their step
spell is in the plan. Item ids differ between shards: the plan's choices only drop options (and one option's spell), so every
item id in an `after` is one the `before` already held.

Each step is written only when its column is EXACTLY the plan's `before` (else it is named and left); a second run finds
nothing to do. Report only by default; --apply writes every change in one transaction with a gm_audit line each and a row in
`cleric_paths_1120_log`; --undo puts back every logged row whose column is still what this tool wrote. Quests are read at the
server's start or a GM panel quest save: run it in a keep restart's before.sh, or save any quest in the panel after.

    python3 -E -s tools/cleric_paths_live_20261120.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "cleric_paths_live_20261120.json")
DETAIL = "Board #1686's cleric path rewards mirrored from the other shard (the owner, 2026-11-20): tools/cleric_paths_live_20261120.py"
WHO, VIA = "claude", "tool"
LOG = "cleric_paths_1120_log"


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def step_row(db, q, s):
    cur = db.execute("SELECT * FROM queststeps WHERE questid = ? AND step = ?", (q, s))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if db.execute("SELECT 1 FROM sqlite_master WHERE name = 'accountinfo'").fetchone() else None
    key = go_json({"questid": before["questid"], "step": before["step"]})
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'queststeps', ?, ?, ?, ?)",
               (now(), acct[0] if acct else 0, WHO, VIA, key, go_json(before), go_json(after), DETAIL))


def set_col(db, q, s, col, value):
    assert col in ("choices", "spells")
    before = step_row(db, q, s)
    db.execute(f"UPDATE queststeps SET {col} = ? WHERE questid = ? AND step = ?", (value, q, s))
    audit(db, before, step_row(db, q, s))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    plan = json.load(open(PLAN))
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        for t in ("queststeps", "gm_audit"):
            if not db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone():
                db.execute("ROLLBACK")
                print(f"REFUSED: no {t} table here")
                return 1
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, questid INTEGER, "
                   "step INTEGER, col TEXT, before TEXT, after TEXT, undone_at TEXT)")
        if a.undo:
            n = left = 0
            for lid, q, s, col, before, after in db.execute(
                    f"SELECT id, questid, step, col, before, after FROM {LOG} WHERE undone_at IS NULL ORDER BY id DESC").fetchall():
                r = step_row(db, q, s)
                if r is None or r[col] != after:
                    print(f"  left: quest {q} step {s} {col} changed since")
                    left += 1
                    continue
                set_col(db, q, s, col, before)
                db.execute(f"UPDATE {LOG} SET undone_at = ? WHERE id = ?", (now(), lid))
                n += 1
            db.execute("COMMIT")
            print(f"undone: {n} column(s), {left} left")
            return 0
        todo, skipped, done = [], 0, 0
        for p in plan:
            r = step_row(db, p["questid"], p["step"])
            if r is None or r["target"] != p["target"] or r["journal"] != p["journal"]:
                print(f"  skipped: quest {p['questid']} step {p['step']} ({p['target']}) is not here as planned")
                skipped += 1
                continue
            for col, ch in p["set"].items():
                if r[col] == ch["after"]:
                    done += 1
                elif r[col] == ch["before"]:
                    todo.append((p, col, ch))
                    print(f"  quest {p['questid']} step {p['step']} ({p['target']}): {col}")
                else:
                    print(f"  skipped: quest {p['questid']} step {p['step']} {col} is neither the plan's before nor after")
                    skipped += 1
        print(f"{len(todo)} to write, {done} already as planned, {skipped} skipped")
        if not todo or not a.apply:
            db.execute("ROLLBACK")
            if todo:
                print("(report only; --apply to write)")
            return 0
        for p, col, ch in todo:
            set_col(db, p["questid"], p["step"], col, ch["after"])
            db.execute(f"INSERT INTO {LOG} (at, questid, step, col, before, after) VALUES (?, ?, ?, ?, ?, ?)",
                       (now(), p["questid"], p["step"], col, ch["before"], ch["after"]))
        db.execute("COMMIT")
        print(f"applied: {len(todo)} column(s), each in gm_audit and {LOG}; read at the server's start or a panel quest save")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
