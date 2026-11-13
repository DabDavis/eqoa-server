#!/usr/bin/env python3
"""Shared machinery of the 2026-11-13 ticket tools (tools/*_1949_*, *_1956_*, *_1960_*, *_camps_1963_*): a tool says
the rows it writes as (table, key, row after or None for a delete); this reports them, applies them in one
transaction with a gm_audit line each and its own log table, and undoes them exactly (a row changed since is left and
said). Not run on its own.

A tool calls main(spec) with spec a dict:
    log      its log table's name
    detail   the gm_audit lines' detail
    keys     {table: (key columns,)} for every table it writes
    needs    tables that must be there (else refused)
    changes  f(db) -> (out, lines, refusals, applied): out a list of (table, key, after); applied True when the rows
             are already as it writes them (nothing done)
    prepare  optional f(db) run in the apply's transaction before the writes (a table or a column the server would
             make at its start), returning lines to print
"""
import argparse
import datetime
import json
import sqlite3
import sys

WHO, VIA = "dab", "tool"


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def column(db, tbl, col):
    return any(r[1] == col for r in db.execute(f"PRAGMA table_info({tbl})"))


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def get(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    r = rows(db, f"SELECT * FROM {tbl} WHERE {where}", list(key.values()))
    return r[0] if r else None


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def log_schema(log):
    return f"""CREATE TABLE IF NOT EXISTS {log} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL for none
    after   TEXT,               -- the row as written (JSON), NULL for deleted
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def write(db, tbl, key, after):
    before = get(db, tbl, key)
    where = " AND ".join(f"{k} = ?" for k in key)
    if after is None:
        if before is None:
            return None
        db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        action = "delete"
    elif before is None:
        cols = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                   [after[c] for c in cols])
        action = "insert"
    else:
        sets = [c for c in after if before.get(c) != after[c]]
        if not sets:
            return None
        db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in sets)} WHERE {where}",
                   [after[c] for c in sets] + list(key.values()))
        action = "update"
    return action, before, get(db, tbl, key)


def apply(db, spec, out):
    db.execute(log_schema(spec["log"]))
    n = 0
    for tbl, key, after in out:
        r = write(db, tbl, key, after)
        if r is None:
            continue
        action, before, now = r
        a = audit(db, action, tbl, key, before, now, spec["detail"])
        db.execute(f"INSERT INTO {spec['log']} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), None if before is None else json.dumps(before, sort_keys=True),
                    None if now is None else json.dumps(now, sort_keys=True), a))
        n += 1
    return n


def undo(db, spec):
    """(back, left)."""
    log = spec["log"]
    if not table(db, log):
        return 0, 0
    back = left = 0
    for seq, tbl, rowkey, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {log} ORDER BY seq DESC").fetchall():
        key = json.loads(rowkey)
        now = get(db, tbl, key)
        if (now is None) != (after is None) or (now is not None and json.dumps(now, sort_keys=True) != after):
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        was = None if before is None else json.loads(before)
        where = " AND ".join(f"{k} = ?" for k in key)
        if was is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", list(key.values()))
        elif now is None:
            cols = list(was)
            db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [was[c] for c in cols])
        else:
            db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in was)} WHERE {where}",
                       list(was.values()) + list(key.values()))
        audit(db, "undo", tbl, key, now, get(db, tbl, key), "undo: " + spec["detail"])
        db.execute(f"DELETE FROM {log} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {log}")
    return back, left


def main(spec, argv=None, doc=None):
    ap = argparse.ArgumentParser(description=doc, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db, spec)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        for t in list(spec.get("needs", ())) + ["gm_audit"]:
            if not table(db, t):
                print(f"refused, nothing done: no {t} table here")
                return 1
        out, lines, refusals, applied = spec["changes"](db)
        if applied:
            print("applied already (the rows are as this tool writes them): nothing done")
            return 0
        if table(db, spec["log"]):
            print(f"{spec['log']} is there but the rows are not as written: --undo first")
            return 1
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(out)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            if spec.get("prepare"):
                for line in spec["prepare"](db):
                    print(line)
            n = apply(db, spec, out)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, in gm_audit and {spec['log']}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit("a module of the ticket tools: run one of them")
