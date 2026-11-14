#!/usr/bin/env python3
"""The level 1-20 city quests' words told from the players' walkthroughs (2026-11-14, batch tix5, lane quest-guides).

The owner: "the quest dialog needs to be more informative for the players so they know the next step" and "look at
eqoa forum motion for the 1-20 quests for the original playable races". The eqoa.forumotion.com "Starting City Quests
1-20" sections (one a city: Fayspires, Freeport, Grobb, Halas, Highbourne, Klik'Anon, Moradhim, Neriak, Oggok, Qeynos,
Rivervale, Surefall Glade, Tethelin) and its "Level 17 Side Quests" are players' walkthroughs of about 2010: each step's
NPC, where they stand, what to kill or bring and how to get there. Each was matched to our quests of the city (the 746
EQOA-Data 1-20 imports and the 17s, by giver, name, level and class), and each step's journal line (and, where it was
vague or pointed elsewhere, the NPC's words) given who, where and what of the NEXT step: "Speak with Grumgra in a shelter
on the shore south-west of Grobb." The NPC's spot is where our spawn stands (the guide's words for the landmark), the
journal at most 127 characters (quests.go questLineChars), a dialog line at most one box (217 less the speaker's name),
the last line of an offer or a waiting still its question; tix1114d's waysender hints ("(Waysender Ti`Thox in Neriak
sends you to Grobb, 225 tunar.)") kept as they are. A few data fixes the guides showed (a target or item that was
wrong) ride along. Each change and why: tools/quest_guides_20261114.json, with each shard's text before (the shards'
quests differ: one entry has a variant a shard).

Refused, nothing written, when a row is not as either shard had it (a GM's edit since). Report only by default; --apply
in one transaction, a gm_audit line a row, log quest_guides_log; a second --apply does nothing; --undo exact (a row
changed since is left and said). --city limits it to some cities. The quests are read at the server's start and at a
GM's reload (QuestEngine.Reload): a restart (a keep restart) or the GM panel's quest reload.

    python3 -E -s tools/quest_guides_20261114.py --db eqoa.db [--apply | --undo] [--city grobb,oggok]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "quest_guides_log"
WHO, VIA = "dab", "tool"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "quest_guides_20261114.json")
DETAIL = "the 1-20 city quests' words told from eqoa.forumotion.com's walkthroughs: tools/quest_guides_20261114.py"
QCOLS = {"offer", "accept", "decline", "declined", "started", "giver"}
SCOLS = {"journal", "answer", "says", "waiting", "notyet", "reminder", "target", "items", "count", "chance", "kind",
         "npcid", "tunar"}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    city    TEXT NOT NULL,
    questid INTEGER NOT NULL,
    step    INTEGER NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def where(step):
    return ("quests", "questid = ?") if step == 0 else ("queststeps", "questid = ? AND step = ?")


def keys(qid, step):
    return (qid,) if step == 0 else (qid, step)


def current(db, qid, step, col):
    tbl, w = where(step)
    r = db.execute(f"SELECT {col} FROM {tbl} WHERE {w}", keys(qid, step)).fetchone()
    return None if r is None else (r[0],)


def audit(db, qid, step, col, before, after):
    tbl, _ = where(step)
    rowkey = {"questid": qid} if step == 0 else {"questid": qid, "step": step}
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, tbl, compact(rowkey), compact({col: before}),
                      compact({col: after}), DETAIL))
    return cur.lastrowid


def load(cities, path=PLAN):
    with open(path, encoding="utf-8") as f:
        plan = json.load(f)
    out = []
    for e in plan["changes"]:
        if cities and e["city"] not in cities:
            continue
        col, step = e["col"], e["step"]
        if (step == 0 and col not in QCOLS) or (step != 0 and col not in SCOLS):
            raise SystemExit(f"plan: column {col!r} of quest {e['questid']} step {step} is not one this tool writes")
        out.append(e)
    return plan, out


def plan_changes(db, entries):
    changes, already, refusals, missing = [], 0, [], 0
    for e in entries:
        qid, step, col = e["questid"], e["step"], e["col"]
        now = current(db, qid, step, col)
        if now is None:
            missing += 1  # the other shard's quest or step
            continue
        v = now[0]
        afters = [x["after"] for x in e["variants"]]
        befores = [x["before"] for x in e["variants"]]
        if v in befores:
            after = e["variants"][befores.index(v)]["after"]
            if after != v:
                changes.append((e["city"], qid, step, col, v, after))
        elif v in afters:
            already += 1
        else:
            refusals.append(f"quest {qid} step {step} {col} = {v!r}: not as either shard had it")
    return changes, already, refusals, missing


def write(db, qid, step, col, before, after):
    tbl, w = where(step)
    cur = db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {w} AND {col} IS ?", (after, *keys(qid, step), before))
    return cur.rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, city, qid, step, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if not write(db, qid, step, col, after, before):
            print(f"left: {city} quest {qid} step {step} {col} changed since")
            left += 1
            continue
        audit(db, qid, step, col, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--city", default="", help="comma separated cities (the plan's city keys); default all")
    ap.add_argument("--verbose", action="store_true", help="print every change")
    ap.add_argument("--plan", default=PLAN, help="the plan (default tools/quest_guides_20261114.json)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    cities = {c.strip() for c in a.city.split(",") if c.strip()}
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        plan, entries = load(cities, a.plan)
        known = {e["city"] for e in plan["changes"]}
        if cities - known:
            sys.exit(f"--city: unknown {sorted(cities - known)} (the plan has {sorted(known)})")
        changes, already, refusals, missing = plan_changes(db, entries)
        by = {}
        for c in changes:
            by.setdefault(c[0], [0, 0])[0 if c[3] == "journal" else 1] += 1
        for city in sorted(by):
            print(f"{city}: {by[city][0]} journal lines, {by[city][1]} other columns")
        if a.verbose:
            for city, qid, step, col, before, after in changes:
                print(f"  {city} {qid}/{step} {col}: {before!r} -> {after!r}")
        print(f"{len(changes)} to write, {already} as the plan has them already, {missing} not on this shard")
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for city, qid, step, col, before, after in changes:
                if not write(db, qid, step, col, before, after):
                    raise RuntimeError(f"quest {qid} step {step} {col} changed under the update")
                a_id = audit(db, qid, step, col, before, after)
                db.execute(f"INSERT INTO {LOG} (city, questid, step, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (city, qid, step, col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
