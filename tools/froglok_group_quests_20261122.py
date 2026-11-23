#!/usr/bin/env python3
"""Frogloks take the cross-race group and epic quests a human may (Heirloom petition 2678: a froglok necromancer at 30,
sent by his Freeport trainer to Count Luvincius in Oasis, was offered nothing by the Count or Quatzi).

The froglok is race 10, below the racedefs races: a quest's races are asked of it as 10 itself, not as the human it
passes as (handlers/quests.go, questRaceTakes). The races the importer wrote for EQOA-Data's level 30/40/49 group
quests ("0, 2, 3, 4, 5, 8, 9" for the Count's) and the 60 epics (where 11-32 were added for the racedefs races) name
the human, 0, and never 10, so a froglok, a human of the east (Freeport) in every other list ("0, 10"), could take
none of them.

  - quests: 10 added to the races of each quest whose source is EQOA-Data's "30,40,49/" or "60/" and whose races
    name 0 and not 10. The 1-20 city quests are left: their "0" alone are Qeynos's and Surefall Glade's, the west's.

Quests are read as they go: --apply against a running server's database is the whole deploy (a quest's row change
reloads it when made through the GM panel; this tool's own writes at the next start, or with --api now).

Dry run by default. --apply writes in one transaction, a gm_audit line a row (account 1, dab, via tool), every row's
races before and after in froglok_quests_1122_log; a second --apply writes nothing. --undo puts back what the log
holds (a row changed since is left and said). --api TOKENFILE applies the same changes through the GM panel's
update_row (each with expect: refused if the races moved; audited and revertible there), for a running shard.

    python3 -E -s tools/froglok_group_quests_20261122.py --db eqoa.db [--apply | --undo | --api TOKENFILE [--url U]]
"""
import argparse
import datetime
import json
import sqlite3
import sys
import urllib.error
import urllib.request

LOG = "froglok_quests_1122_log"
WHO, ACCOUNT, VIA = "dab", 1, "tool"
TOOL = "tools/froglok_group_quests_20261122.py"
DETAIL = f"Heirloom petition 2678, frogloks take the group and epic quests a human may: {TOOL}"
FROGLOK, HUMAN = 10, 0
SOURCES = ("EQOA-Data 30,40,49/", "EQOA-Data 60/")

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    questid INTEGER NOT NULL,
    before  TEXT NOT NULL,
    after   TEXT NOT NULL,
    audit   INTEGER NOT NULL,
    at      TEXT NOT NULL
)"""


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def parse(races):
    return [int(r) for r in races.replace(" ", "").split(",") if r != ""]


def fmt(races):
    return ", ".join(str(r) for r in races)


def with_froglok(races):
    """races (as written) with 10 put in its order, or None when it needs none."""
    rs = parse(races)
    if HUMAN not in rs or FROGLOK in rs:
        return None
    return fmt(sorted(rs + [FROGLOK]))


def plan(db):
    out = []
    for qid, name, races, source in db.execute("SELECT questid, name, races, source FROM quests ORDER BY questid"):
        if not source.startswith(SOURCES):
            continue
        new = with_froglok(races)
        if new is not None:
            out.append((qid, name, races, new))
    return out


def audit(db, key, before, after):
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, ?, ?, '', 'update', 'quests', ?, ?, ?, ?)",
                      (now(), ACCOUNT, WHO, VIA, json.dumps(key), json.dumps(before), json.dumps(after), DETAIL)).lastrowid


def apply(db, changes):
    db.execute(LOG_DDL)
    at = now()
    for qid, _, old, new in changes:
        db.execute("UPDATE quests SET races = ? WHERE questid = ? AND races = ?", (new, qid, old))
        a = audit(db, {"questid": qid}, {"races": old}, {"races": new})
        db.execute(f"INSERT INTO {LOG} (questid, before, after, audit, at) VALUES (?, ?, ?, ?, ?)", (qid, old, new, a, at))
    return len(changes)


def undo(db):
    if not db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone():
        return 0, []
    back, left = 0, []
    for seq, qid, before, after in db.execute(f"SELECT seq, questid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        have = db.execute("SELECT races FROM quests WHERE questid = ?", (qid,)).fetchone()
        if have is None or have[0] != after:
            left.append(f"quest {qid}: changed since, left as it is")
            continue
        db.execute("UPDATE quests SET races = ? WHERE questid = ?", (before, qid))
        audit(db, {"questid": qid}, {"races": after}, {"races": before})
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not db.execute(f"SELECT count(*) FROM {LOG}").fetchone()[0]:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def via_api(changes, tokenfile, url):
    tok = open(tokenfile).read().strip()
    bad = 0
    for qid, name, old, new in changes:
        body = {"table": "quests", "key": {"questid": qid}, "expect": {"races": old}, "values": {"races": new}}
        req = urllib.request.Request(f"{url}/api/op/update_row", data=json.dumps(body).encode(),
                                     headers={"Authorization": "Bearer " + tok, "Content-Type": "application/json"})
        try:
            res = json.load(urllib.request.urlopen(req))
            print(f"  {qid} {name}: {res.get('action')} (audit {res.get('audit')})")
        except urllib.error.HTTPError as e:
            bad += 1
            print(f"  {qid} {name}: REFUSED {e.read().decode()[:200]}")
    return bad


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    g.add_argument("--api", metavar="TOKENFILE")
    ap.add_argument("--url", default="https://gm.eqoaclassic.com")
    a = ap.parse_args(argv)
    if a.api:
        db = sqlite3.connect(f"file:{a.db}?mode=ro", uri=True)
        changes = plan(db)
        db.close()
        print(f"{len(changes)} quests through {a.url}:")
        return 1 if via_api(changes, a.api, a.url) else 0
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} quests put back" + "".join(f"\n  left: {w}" for w in left))
            return 0
        changes = plan(db)
        for qid, name, old, new in changes:
            print(f"  {qid} {name}: races {old} -> {new}")
        print(f"totals: {len(changes)} quests")
        if not a.apply or not changes:
            db.execute("ROLLBACK")
            print("dry run: nothing written (--apply to write)" if not a.apply else "nothing to write: all in place")
            return 0
        n = apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {n} quests, a gm_audit line each, logged in {LOG}")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
