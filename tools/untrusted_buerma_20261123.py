#!/usr/bin/env python3
"""Apprentice Buerma says whose trust "The Ancient Stone Golem" wants (Heirloom petition 2742, 2026-11-23: "Apprentice
Buerma should notify you that you need to raise faction by killing gnolls or bandits when you talk to her").

Quest 100763 asks a standing of 0 with the Black Swan Inn (factionreq). Below it Buerma had nothing to say; the code
(handlers/quests.go untrustedLocked) now has a giver say "I do not trust you yet. Earn the trust of the Black Swan Inn
first." and then the quest's own words on earning it, its new column quests.untrusted (made at a start; made here too if
it is not there yet). This writes those words for 100763, from factionhits: the kills that raise the Black Swan Inn
are the gnolls north of the inn (ruffians, soldiers, spiritists, foragers, the white, red and black paws, fishermen)
and the Darkwood goblins south of it; no bandit raises it. Refused, nothing written, unless the quest is as read here.

Quests are read at a start or a GM panel quest reload: apply in the before.sh of the next keep restart, on each shard,
with the build that reads the column. Dry run by default; --apply writes in one transaction, a gm_audit line, before
and after in untrusted_1123_log; a second --apply writes nothing; --undo puts back what the log holds.

    python3 -E -s tools/untrusted_buerma_20261123.py --db eqoa.db --shard heirloom|live [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "untrusted_1123_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/untrusted_buerma_20261123.py"
DETAIL = f"Heirloom petition 2742: Apprentice Buerma says how to earn the Black Swan Inn's trust: {TOOL}"
QUEST, NAME, GIVER, REQ = 100763, "The Ancient Stone Golem", "Apprentice Buerma", "Black Swan Inn:0"
WORDS = ("Kill the gnolls in the hills north of the inn, or the Darkwood goblins to the south of it: "
         "the Black Swan Inn will trust you for it.")

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



def build(p, shard):
    db = p.db
    if not table(db, "quests") or not table(db, "gm_audit"):
        raise Refused("no quests or gm_audit")
    if "untrusted" not in [r[1] for r in db.execute("PRAGMA table_info(quests)")]:
        db.execute("ALTER TABLE quests ADD COLUMN untrusted TEXT NOT NULL DEFAULT ''")
    q = rows(db, "quests", {"questid": QUEST})
    if len(q) != 1 or q[0]["name"] != NAME or q[0]["giver"] != GIVER or q[0]["factionreq"] != REQ:
        raise Refused(f"quest {QUEST} is not {NAME!r} of {GIVER} asking {REQ}")
    p.change("quests", q[0], {"untrusted": ""}, {"untrusted": WORDS}, f"quest {QUEST} {NAME!r}: its words to the untrusted")


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
            build(p, a.shard)
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
