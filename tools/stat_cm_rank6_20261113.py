#!/usr/bin/env python3
"""A rank 6 of each primary stat's mastery line (the GM Jibwiz's petition 1918, 2026-11-13: "Each main stat has a CM line
to increase its cap, example Balance 1, 2, 3, 4, 5 and they increase the amount of CM's they cost, and the amount they
increase the maximum stat by. For every primary stat: cha, wis, int, str, dex, agi, sta add an additional version 6 of it,
scaling the cost, base stat increase, and ...").

The seven General lines (cmcatalog category 0, every class and race; EQOA-Data's CMs with EQOAEmu's workbook), ranks 1-5
the same on every line and both shards:

    rank   id (Brawn)  cost  level  base stat needed  effects
    1      1           10    10     100               +1 stat, cap +15
    2      2           15    20     125               +2,      cap +20
    3      3           20    30     160               +3,      cap +25
    4      4           25    45     200               +4,      cap +30
    5      5           30    60     250               +5,      cap +35
    6 (new)            35    60     310               +6,      cap +40

Rank 6 carries each step on: the cost 5 more, the stat 1 more, the cap 5 more; the base stat needed 60 more (the steps
were 25, 35, 40, 50); the level stays 60, the game's last (rank 5's already). Everything else is rank 5's (name "<Line>
6", its description, classes, races, category, gates), its parent rank 5, its id rank 5's plus one (6, 26, 46, 66, 86, 106,
126: free on both shards), source "petition 1918 rank 6". Lines and stats: Brawn STR, Vigor STA, Nimbleness AGI, Balance
DEX, Insight WIS, Wit INT, Beauty CHA. (Base stat needed counts base, training and what masteries add, entity/cmbuy.go;
the cap is 400 past level 45.)

Refused, nothing written, unless each line's rank 5 is as the table says and its id + 1 is free (or already this tool's
row: applied). Report only by default; --apply writes the seven rows in one transaction, a gm_audit line each, logged in
stat_cm_rank6_1113_log; --undo deletes each row still as written, a row bought by a character left and said. The
catalog is read at the server's start (or a GM panel edit of cmcatalog): RESTART after --apply. Run it on each shard.

    python3 -E -s tools/stat_cm_rank6_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "stat_cm_rank6_1113_log"
DETAIL = "petition 1918 (Jibwiz): rank 6 of each primary stat's mastery line; tools/stat_cm_rank6_20261113.py"
WHO, VIA = "dab", "tool"
SOURCE = "petition 1918 rank 6"
LINES = [("Brawn", "STR", 5), ("Vigor", "STA", 25), ("Nimbleness", "AGI", 45), ("Balance", "DEX", 65), ("Insight", "WIS", 85),
         ("Wit", "INT", 105), ("Beauty", "CHA", 125)]
RANK5 = {"rank": 5, "cost": 30, "level": 60}
RANK6 = {"cost": 35, "level": 60, "min": 310, "stat": 6, "cap": 40}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq   INTEGER PRIMARY KEY AUTOINCREMENT,
    id    INTEGER NOT NULL,
    after TEXT NOT NULL,
    audit INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, cid):
    cur = db.execute("SELECT * FROM cmcatalog WHERE id = ?", (cid,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, cid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'cmcatalog', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"id": cid}),
                      None if before is None else go_json(before), None if after is None else go_json(after), DETAIL))
    return cur.lastrowid


def rank6_of(r5, stat):
    """Rank 6 from its line's rank 5 row (the file's doc)."""
    r = dict(r5)
    r.update(id=r5["id"] + 1, rank=6, name=f"{r5['mastery']} 6", parent=r5["id"], cost=RANK6["cost"], level=RANK6["level"],
             minstats=f"{stat}:{RANK6['min']}", effects=f"{stat}:{RANK6['stat']},{stat}Max:{RANK6['cap']}", source=SOURCE)
    return r


def plan(db):
    """(rows to write, lines, refusals, applied)."""
    for t in ("cmcatalog", "gm_audit"):
        if not table(db, t):
            return [], [], [f"no {t} table here"], False
    rows, lines, refusals, done = [], [], [], 0
    for mastery, stat, rid in LINES:
        r5 = row(db, rid)
        want = f"{stat}:5,{stat}Max:35"
        if r5 is None or r5["mastery"] != mastery or r5["rank"] != 5 or r5["effects"] != want or r5["minstats"] != f"{stat}:250" or \
                r5["cost"] != RANK5["cost"] or r5["level"] != RANK5["level"]:
            refusals.append(f"cmcatalog {rid}: not {mastery} 5 as the petition found it ({stat}:250, {want}, 30 points, level 60): "
                            f"{None if r5 is None else {k: r5[k] for k in ('mastery', 'rank', 'cost', 'level', 'minstats', 'effects')}}")
            continue
        r6 = rank6_of(r5, stat)
        have = row(db, r6["id"])
        if have is not None:
            if have.get("source") == SOURCE:
                done += 1
                continue
            refusals.append(f"cmcatalog {r6['id']}: taken by {have['name']!r} ({have['source']})")
            continue
        if db.execute("SELECT count(*) FROM cmcatalog WHERE lower(mastery) = lower(?) AND rank = 6", (mastery,)).fetchone()[0]:
            refusals.append(f"{mastery} has a rank 6 already")
            continue
        rows.append(r6)
        lines.append(f"cmcatalog {r6['id']} {r6['name']!r}: after {rid}, {r6['cost']} points, level {r6['level']}, "
                     f"{r6['minstats']}, {r6['effects']}")
    if done == len(LINES):
        return [], [], [], True
    if done and not refusals:
        refusals.append(f"{done} of the seven rows are there already: half applied?")
    return rows, lines, refusals, False


def apply(db, rows):
    db.execute(LOG_SCHEMA)
    cols = list(rows[0].keys())
    for r in rows:
        db.execute(f"INSERT INTO cmcatalog ({', '.join(cols)}) VALUES ({', '.join('?' for _ in cols)})", [r[c] for c in cols])
        after = row(db, r["id"])
        a = audit(db, "insert", r["id"], None, after)
        db.execute(f"INSERT INTO {LOG} (id, after, audit) VALUES (?, ?, ?)", (r["id"], go_json(after), a))
    return len(rows)


def undo(db):
    """(deleted, left: [why])."""
    if not table(db, LOG):
        return 0, []
    gone, left = 0, []
    for cid, after in db.execute(f"SELECT id, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = row(db, cid)
        if now is None:
            continue
        if go_json(now) != after:
            left.append(f"cmcatalog {cid}: changed since")
            continue
        if table(db, "charcms") and db.execute("SELECT count(*) FROM charcms WHERE cmid = ?", (cid,)).fetchone()[0]:
            left.append(f"cmcatalog {cid} {now['name']!r}: bought by a character")
            continue
        db.execute("DELETE FROM cmcatalog WHERE id = ?", (cid,))
        audit(db, "delete", cid, now, None)
        gone += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return gone, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            gone, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {gone} rows deleted" + ("".join(f"\n  left: {w}" for w in left)))
            return 0 if not left else 1
        rows, lines, refusals, applied = plan(db)
        if applied:
            db.execute("ROLLBACK")
            print("applied already: nothing to do")
            return 0
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"(report only: {len(rows)} rows; --apply to write, then RESTART)")
            return 0
        n = apply(db, rows)
        db.execute("COMMIT")
        print(f"applied: {n} rows, in gm_audit and {LOG}; RESTART the server")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
