#!/usr/bin/env python3
"""The rows that go with lane 2 of a GM's approved code asks (the owner's standing rule: that GM's board asks are approved as
submitted): the new tables' seeds and the rows that waited on the code (corpse tunar, per-merchant prices, shop greetings and
ambient lines, talk trees, welcomes, ...), one board entry a section of `jib_lane2_20261122.json`.

The plan is data, by entry:
  creates      CREATE TABLE IF NOT EXISTS, the same DDL the server runs at its start, so the rows can go in before the binary
  add_columns  a column the server adds at its start (written only when missing)
  inserts      a row by its key: written only when no row has the key (an identical row is "already"; another is left, named)
  updates      a column written only when it is exactly the plan's `before`
Shard-bound rows (item ids differ between shards) carry "shards"; --shard says which shard this database is.

Report only by default; --apply writes everything in one transaction, a gm_audit line a row and a line in
`jib_lane2_1122_log`, so --undo is exact: it deletes an inserted row and restores an updated column only while each is still
what this tool wrote (else it is named and left). A table or column this tool created stays (empty, harmless).
Everything here is read by the server as it goes (no restart), but an entry marked binary_first means nothing until the
binary that reads it runs.

    python3 -E -s tools/jib_lane2_20261122.py --db eqoa.db --shard heirloom|live [--entry N ...] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "jib_lane2_20261122.json")
DETAIL = "Lane 2 of a GM's approved code asks, board entry %s: tools/jib_lane2_20261122.py"
WHO, VIA = "claude", "tool"
LOG = "jib_lane2_1122_log"
SHARDS = ("heirloom", "live")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def ident(name):
    if not name.replace("_", "").isalnum():
        raise ValueError(f"not a plain name: {name!r}")
    return '"' + name + '"'


def has_table(db, t):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone() is not None


def columns(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({ident(t)})")]


def fetch(db, t, where):
    sql = f"SELECT * FROM {ident(t)} WHERE " + " AND ".join(f"{ident(k)} = ?" for k in where)
    cur = db.execute(sql, tuple(where.values()))
    rows = cur.fetchall()
    names = [d[0] for d in cur.description]
    return [dict(zip(names, r)) for r in rows]


class Writer:
    def __init__(self, db):
        self.db = db
        acct = None
        if has_table(db, "accountinfo"):
            acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone()
        self.acct = acct[0] if acct else 0

    def audit(self, entry, action, tbl, key, before, after):
        self.db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                        "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                        (now(), self.acct, WHO, VIA, action, tbl, go_json(key),
                         None if before is None else go_json(before), None if after is None else go_json(after),
                         DETAIL % entry))

    def log(self, entry, kind, tbl, key, before, after):
        self.db.execute(f"INSERT INTO {LOG} (at, entry, kind, tbl, rowkey, before, after) VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (now(), entry, kind, tbl, go_json(key), go_json(before), go_json(after)))


def for_shard(item, shard):
    return shard in item.get("shards", SHARDS)


def plan_work(db, plan, shard, out):
    """Every change still to make, as (entry, kind, payload); prints what is already done or left."""
    todo, done, left = [], 0, 0
    for sec in plan:
        e = sec["entry"]
        for ddl in sec.get("creates", []):
            t = ddl.split("EXISTS", 1)[1].split("(", 1)[0].strip().strip('"')
            if has_table(db, t):
                done += 1
            else:
                todo.append((e, "create", {"table": t, "ddl": ddl}))
        for c in sec.get("add_columns", []):
            created = any(k == "create" and p["table"] == c["table"] for _, k, p in todo)
            if not created and not has_table(db, c["table"]):
                out.append(f"  left: entry {e}: no {c['table']} table for column {c['column']}")
                left += 1
            elif created or c["column"] not in columns(db, c["table"]):
                todo.append((e, "column", c))
            else:
                done += 1
        pending_cols = {(p["table"], p["column"]) for _, k, p in todo if k == "column"}
        pending_tables = {p["table"] for _, k, p in todo if k == "create"}
        for ins in sec.get("inserts", []):
            if not for_shard(ins, shard):
                continue
            t, row = ins["table"], ins["row"]
            key = {k: row[k] for k in ins["key"]}
            if t in pending_tables:
                todo.append((e, "insert", ins))
                continue
            if not has_table(db, t):
                out.append(f"  left: entry {e}: no {t} table for {go_json(key)}")
                left += 1
                continue
            missing = [c for c in row if c not in columns(db, t) and (t, c) not in pending_cols]
            if missing:
                out.append(f"  left: entry {e}: {t} has no column {', '.join(missing)}")
                left += 1
                continue
            have = fetch(db, t, key) if not any((t, k) in pending_cols for k in key) else []
            if not have:
                todo.append((e, "insert", ins))
            elif all(have[0].get(c) == v for c, v in row.items()):
                done += 1
            else:
                out.append(f"  left: entry {e}: {t} {go_json(key)} is here already, not as planned")
                left += 1
        for up in sec.get("updates", []):
            if not for_shard(up, shard):
                continue
            t, where = up["table"], up["where"]
            rows = fetch(db, t, where) if has_table(db, t) else []
            if len(rows) != 1:
                out.append(f"  left: entry {e}: {t} {go_json(where)}: {len(rows)} rows, not one")
                left += 1
                continue
            for col, ch in up["set"].items():
                if col not in rows[0]:
                    out.append(f"  left: entry {e}: {t} has no column {col}")
                    left += 1
                elif rows[0][col] == ch["after"]:
                    done += 1
                elif rows[0][col] == ch["before"]:
                    todo.append((e, "update", {"table": t, "where": where, "col": col, "before": ch["before"],
                                               "after": ch["after"]}))
                else:
                    out.append(f"  left: entry {e}: {t} {go_json(where)} {col} is neither the plan's before nor after")
                    left += 1
    return todo, done, left


def apply(db, w, todo):
    for e, kind, p in todo:
        if kind == "create":
            db.execute(p["ddl"])
            w.log(e, "create", p["table"], {}, None, p["ddl"])
        elif kind == "column":
            db.execute(p["ddl"])
            w.log(e, "column", p["table"], {"column": p["column"]}, None, p["ddl"])
        elif kind == "insert":
            t, row = p["table"], p["row"]
            key = {k: row[k] for k in p["key"]}
            cols = list(row)
            db.execute(f"INSERT INTO {ident(t)} (" + ", ".join(ident(c) for c in cols) + ") VALUES (" +
                       ", ".join("?" for _ in cols) + ")", tuple(row[c] for c in cols))
            after = fetch(db, t, key)[0]
            w.audit(e, "insert", t, key, None, after)
            w.log(e, "insert", t, key, None, row)
        elif kind == "update":
            t, where, col = p["table"], p["where"], p["col"]
            before = fetch(db, t, where)[0]
            db.execute(f"UPDATE {ident(t)} SET {ident(col)} = ? WHERE " + " AND ".join(f"{ident(k)} = ?" for k in where),
                       (p["after"], *where.values()))
            w.audit(e, "update", t, where, before, fetch(db, t, where)[0])
            w.log(e, "update", t, dict(where, _col=col), p["before"], p["after"])


def undo(db, w, entries):
    n = left = 0
    rows = db.execute(f"SELECT id, entry, kind, tbl, rowkey, before, after FROM {LOG} WHERE undone_at IS NULL "
                      "ORDER BY id DESC").fetchall()
    for lid, e, kind, t, rowkey, before, after in rows:
        if entries and e not in entries:
            continue
        key = json.loads(rowkey)
        if kind == "insert":
            row = json.loads(after)
            have = fetch(db, t, key) if has_table(db, t) else []
            if len(have) != 1 or any(have[0].get(c) != v for c, v in row.items()):
                print(f"  left: entry {e}: {t} {rowkey} changed since")
                left += 1
                continue
            db.execute(f"DELETE FROM {ident(t)} WHERE " + " AND ".join(f"{ident(k)} = ?" for k in key), tuple(key.values()))
            w.audit(e, "delete", t, key, have[0], None)
        elif kind == "update":
            col = key.pop("_col")
            have = fetch(db, t, key)
            if len(have) != 1 or have[0].get(col) != json.loads(after):
                print(f"  left: entry {e}: {t} {go_json(key)} {col} changed since")
                left += 1
                continue
            db.execute(f"UPDATE {ident(t)} SET {ident(col)} = ? WHERE " + " AND ".join(f"{ident(k)} = ?" for k in key),
                       (json.loads(before), *key.values()))
            w.audit(e, "update", t, key, have[0], fetch(db, t, key)[0])
        # a created table or column stays: the server makes it at its start anyway
        db.execute(f"UPDATE {LOG} SET undone_at = ? WHERE id = ?", (now(), lid))
        n += 1
    return n, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--shard", required=True, choices=SHARDS)
    ap.add_argument("--entry", type=int, action="append", help="only this board entry (again for more)")
    ap.add_argument("--plan", default=PLAN, help=argparse.SUPPRESS)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    plan = [s for s in json.load(open(a.plan)) if not a.entry or s["entry"] in a.entry]
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if not has_table(db, "gm_audit"):
            db.execute("ROLLBACK")
            print("REFUSED: no gm_audit table here")
            return 1
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT, entry INTEGER, "
                   "kind TEXT, tbl TEXT, rowkey TEXT, before TEXT, after TEXT, undone_at TEXT)")
        w = Writer(db)
        if a.undo:
            n, left = undo(db, w, a.entry)
            db.execute("COMMIT")
            print(f"undone: {n} change(s), {left} left")
            return 0
        out = []
        todo, done, left = plan_work(db, plan, a.shard, out)
        for e, kind, p in todo:
            what = p.get("table")
            if kind == "insert":
                what += " " + go_json({k: p["row"][k] for k in p["key"]})
            elif kind == "update":
                what += f" {go_json(p['where'])} {p['col']}"
            elif kind == "column":
                what += "." + p["column"]
            print(f"  entry {e}: {kind} {what}")
        print("\n".join(out))
        print(f"{len(todo)} to write, {done} already as planned, {left} left")
        if not todo or not a.apply:
            db.execute("ROLLBACK")
            if todo:
                print("(report only; --apply to write)")
            return 0
        apply(db, w, todo)
        db.execute("COMMIT")
        print(f"applied: {len(todo)} change(s), each in {LOG} (and gm_audit for a row)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
