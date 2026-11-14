#!/usr/bin/env python3
"""The level 17-60, path, epic, raid, misc, were and hunt-and-gather quests' words told from a second walkthrough
(2026-11-14, batch tix5, lane quest-guides-17).

The owner: "the quest dialog needs to be more informative for the players so they know the next step". Build224
(tools/quest_guides_20261114.py) did the 1-20 city quests from the eqoa.forumotion guides; this is the second source the
owner sent, Jarrod Garripoli's EQOA Frontiers FAQ (GameFAQs 48152, about 2005-2007): its Level 17 (corrections to
build224 only), 23-27, 33-37, 43-47, Path (30/40), Level 49 Epic, 53-59, Level 60 Epic, Raid, Miscellaneous,
Lycanthrope/Werehunter and Hunt & Gather sections. Each FAQ quest was matched to ours (by giver, name, level, class)
and each step's journal line (and, where vague or pointing elsewhere, the NPC's words) given who, where and what of the
NEXT step, the spot being where our spawn stands (the FAQ's landmark where it agrees), the journal at most 127
characters (quests.go questLineChars), a dialog line one box, the last line of an offer or a waiting still its
question; tix1114d's waysender hints kept as they are, build224's lines kept unless the FAQ shows them wrong. A few
data fixes the FAQ showed (a target, an item, a count) ride along. Each change and why (faq line numbers):
tools/quest_guides17_20261114.json, with each shard's text before (the shards' quests differ: an entry has a variant
a shard).

Refused, nothing written, when a row is not as either shard had it (a GM's edit since). Report only by default; --apply
in one transaction, a gm_audit line a row, log quest_guides17_log; a second --apply does nothing; --undo exact (a row
changed since is left and said). --section limits it to some sections. The quests are read at the server's start and
at a GM's reload (QuestEngine.Reload): a restart (a keep restart) or the GM panel's quest reload.

    python3 -E -s tools/quest_guides17_20261114.py --db eqoa.db [--apply | --undo] [--section mid,high]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

LOG = "quest_guides17_log"
WHO, VIA = "dab", "tool"
PLAN = os.path.join(os.path.dirname(os.path.abspath(__file__)), "quest_guides17_20261114.json")
DETAIL = "the 17-60 quests' words told from the GameFAQs EQOA Frontiers FAQ (48152): tools/quest_guides17_20261114.py"
QCOLS = {"offer", "accept", "decline", "declined", "started", "giver"}
SCOLS = {"journal", "answer", "says", "waiting", "notyet", "reminder", "target", "items", "count", "chance", "kind",
         "npcid", "tunar"}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    section TEXT NOT NULL,
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


def load(sections, path=PLAN):
    with open(path, encoding="utf-8") as f:
        plan = json.load(f)
    out = []
    for e in plan["changes"]:
        if sections and e["section"] not in sections:
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
                changes.append((e["section"], qid, step, col, v, after))
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
    for _, sec, qid, step, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if not write(db, qid, step, col, after, before):
            print(f"left: {sec} quest {qid} step {step} {col} changed since")
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
    ap.add_argument("--section", default="", help="comma separated sections (the plan's section keys); default all")
    ap.add_argument("--verbose", action="store_true", help="print every change")
    ap.add_argument("--plan", default=PLAN, help="the plan (default tools/quest_guides17_20261114.json)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    sections = {c.strip() for c in a.section.split(",") if c.strip()}
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
        plan, entries = load(sections, a.plan)
        known = {e["section"] for e in plan["changes"]}
        if sections - known:
            sys.exit(f"--section: unknown {sorted(sections - known)} (the plan has {sorted(known)})")
        changes, already, refusals, missing = plan_changes(db, entries)
        by = {}
        for c in changes:
            by.setdefault(c[0], [0, 0])[0 if c[3] == "journal" else 1] += 1
        for sec in sorted(by):
            print(f"{sec}: {by[sec][0]} journal lines, {by[sec][1]} other columns")
        if a.verbose:
            for sec, qid, step, col, before, after in changes:
                print(f"  {sec} {qid}/{step} {col}: {before!r} -> {after!r}")
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
            for sec, qid, step, col, before, after in changes:
                if not write(db, qid, step, col, before, after):
                    raise RuntimeError(f"quest {qid} step {step} {col} changed under the update")
                a_id = audit(db, qid, step, col, before, after)
                db.execute(f"INSERT INTO {LOG} (section, questid, step, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (sec, qid, step, col, json.dumps(before), json.dumps(after), a_id))
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
