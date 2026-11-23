#!/usr/bin/env python3
"""The rain chain caught up for groupmates who rode it with the one who did its talks (Live petition 5510, the owner,
2026-11-23: "as long as 1 person in your group is on the quest it should group complete").

A group rode the rain chain's passages together on 2026-11-23 before the code that does a Sky chain's steps for the group
(handlers/skygroup.go): one did "Sterie Glitterlight" (101302) and "The Necrologist of Rain" (101303) to the end, the
others stayed at 101302's first step and never took 101303, though they were carried through every isle with it. Each
groupmate named is given what the group did: 101302 finished (it has no reward), 101303 under way at its last step
(12, the Manifestation of Rain on the Isle of Wonder, whose talk gives its reward as it gave the one who did it), in
101302's journal slot. Refused, nothing written, unless the one named finished 101303 and each groupmate is at 101302
step 1 with no row of 101303.

Quest progress is read at a start (and each online character's is held in memory): apply in the before.sh of the next
keep restart. Dry run by default; --apply writes in one transaction, a gm_audit line a row, before and after in
sky_catchup_1123_log; a second --apply writes nothing; --undo puts back what the log holds.

    python3 -E -s tools/sky_catchup_20261123.py --db eqoa.db --shard live --with <id> --chars <id,id,...> [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys
import time

LOG = "sky_catchup_1123_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/sky_catchup_20261123.py"
DETAIL = f"Live petition 5510: the rain chain caught up for the group that rode it: {TOOL}"
STERIE, NECRO, LAST = 101302, 101303, 12

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rid    INTEGER NOT NULL,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL,
    at     TEXT NOT NULL
)"""


class Refused(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def dumps(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def rows(db, tbl, where):
    w = " AND ".join(f'"{c}" = ?' for c in where) or "1"
    cur = db.execute(f'SELECT rowid AS _rowid_, * FROM "{tbl}" WHERE {w} ORDER BY rowid', tuple(where.values()))
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def plain(r):
    return None if r is None else {k: v for k, v in r.items() if k != "_rowid_"}


def same(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return abs(float(a) - float(b)) < 1e-6
    return a == b


def num(v):
    return int(v) if float(v).is_integer() else v


class Plan:
    def __init__(self, db):
        self.db = db
        self.ops = []  # (tbl, rid or None, before or None, after values)
        self.lines = []

    def say(self, s):
        self.lines.append(f"  {s}")

    def add(self, tbl, values, match, what):
        have = rows(self.db, tbl, {c: values[c] for c in match})
        if have:
            if len(have) == 1 and all(same(have[0].get(c), v) for c, v in values.items()):
                self.say(f"{what}: already there")
                return
            raise Refused(f"{what}: {tbl} has {dumps([plain(h) for h in have])[:300]}, not {dumps(values)[:300]}")
        self.ops.append((tbl, None, None, dict(values)))
        self.say(f"{what}: insert {tbl}")

    def change(self, tbl, row, was, now_, what):
        """Set now_'s columns on row, which must read was (or now_ already)."""
        if all(same(row.get(c), v) for c, v in now_.items()):
            self.say(f"{what}: already so")
            return
        for c, v in was.items():
            if not same(row.get(c), v):
                raise Refused(f"{what}: {c} reads {row.get(c)!r}, not {v!r}")
        self.ops.append((tbl, row["_rowid_"], plain(row), dict(now_)))
        self.say(f"{what}: {dumps({c: row.get(c) for c in now_})[:160]} -> {dumps(now_)[:160]}")



def build(p, leader, chars):
    db = p.db
    for t in ("charquests", "queststeps", "gm_audit"):
        if not table(db, t):
            raise Refused(f"no table {t}")
    if not db.execute("SELECT 1 FROM queststeps WHERE questid = ? AND step = ? AND lower(target) = 'manifestation of rain'",
                      (NECRO, LAST)).fetchone():
        raise Refused(f"quest {NECRO} step {LAST} is not the Manifestation of Rain's")
    lead = rows(db, "charquests", {"serverid": leader, "questid": NECRO})
    if len(lead) != 1 or lead[0]["step"] != 0 or lead[0]["done"] < 1:
        raise Refused(f"character {leader} has not finished quest {NECRO}")
    if not chars:
        raise Refused("no groupmates named")
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for c in chars:
        st = rows(db, "charquests", {"serverid": c, "questid": STERIE})
        ne = rows(db, "charquests", {"serverid": c, "questid": NECRO})
        if ne and ne[0]["step"] == LAST and st and st[0]["step"] == 0:
            p.say(f"character {c}: caught up already")
            continue
        if len(st) != 1 or st[0]["step"] != 1 or ne:
            raise Refused(f"character {c}: {STERIE} {[(r['step'], r['done']) for r in st]}, {NECRO} "
                          f"{[(r['step'], r['done']) for r in ne]}: not at {STERIE} step 1 without {NECRO}")
        p.change("charquests", st[0], {"step": 1}, {"step": 0, "count": 0, "done": st[0]["done"] + 1, "updated": now},
                 f"character {c}: {STERIE} finished")
        p.add("charquests", {"serverid": c, "questid": NECRO, "step": LAST, "count": 0, "done": 0, "slot": st[0]["slot"],
                             "updated": now, "picks": "", "taken": int(time.time())}, ("serverid", "questid"),
              f"character {c}: {NECRO} at step {LAST} (the Manifestation of Rain)")


def keyof(db, tbl, row, rid):
    pk = sorted((r[5], r[1]) for r in db.execute(f'PRAGMA table_info("{tbl}")') if r[5])
    return {c: row[c] for _, c in pk} if pk else {"rowid": rid}


def audit(db, action, tbl, key, before, after):
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                      (now(), ACCOUNT, WHO, VIA, action, tbl, dumps(key), None if before is None else dumps(before),
                       None if after is None else dumps(after), DETAIL)).lastrowid


def by_rowid(db, tbl, rid):
    r = rows(db, tbl, {"rowid": rid})
    return plain(r[0]) if r else None


def apply(db, p):
    db.execute(LOG_DDL)
    at = now()
    for tbl, rid, before, new in p.ops:
        if rid is None:
            cs = list(new)
            rid = db.execute(f'INSERT INTO "{tbl}" ({", ".join(chr(34) + c + chr(34) for c in cs)}) '
                             f'VALUES ({", ".join("?" * len(cs))})', [new[c] for c in cs]).lastrowid
            action = "insert"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in new)} WHERE rowid = ?',
                       [*new.values(), rid])
            action = "update"
        after = by_rowid(db, tbl, rid)
        key = keyof(db, tbl, after, rid)
        a = audit(db, action, tbl, key, before, after)
        db.execute(f"INSERT INTO {LOG} (tbl, rid, rowkey, before, after, audit, at) VALUES (?, ?, ?, ?, ?, ?, ?)",
                   (tbl, rid, dumps(key), None if before is None else dumps(before), dumps(after), a, at))
    return len(p.ops)


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for seq, tbl, rid, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rid, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = by_rowid(db, tbl, rid)
        if have is None or dumps(have) != after:
            left.append(f"{tbl} {rowkey}: changed since, left as it is")
            continue
        b = None if before is None else json.loads(before)
        if b is None:
            db.execute(f'DELETE FROM "{tbl}" WHERE rowid = ?', (rid,))
            action = "delete"
        else:
            db.execute(f'UPDATE "{tbl}" SET {", ".join(chr(34) + c + chr(34) + " = ?" for c in b)} WHERE rowid = ?',
                       [*b.values(), rid])
            action = "update"
        audit(db, action, tbl, json.loads(rowkey), have, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    ap.add_argument("--with", dest="leader", type=int, required=True, help="the character who did the steps")
    ap.add_argument("--chars", required=True, help="the groupmates' character ids, comma-separated")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        p = Plan(db)
        try:
            build(p, a.leader, [int(x) for x in a.chars.split(',') if x.strip()])
        except Refused as e:
            db.execute("ROLLBACK")
            print("\n".join(p.lines))
            print(f"REFUSED, nothing written: {e}")
            return 1
        print(f"{a.shard}:")
        print("\n".join(p.lines))
        print(f"totals: {len(p.ops)} rows to write")
        if not a.apply or not p.ops:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)" if not a.apply else "nothing to write: all in place")
            return 0
        n = apply(db, p)
        db.execute("COMMIT")
        print(f"applied: {n} rows, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
