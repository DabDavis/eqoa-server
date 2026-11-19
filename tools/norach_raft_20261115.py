#!/usr/bin/env python3
"""Norach's first trip lands on the raft (the owner, 2026-11-15: "players are not always porting onto the raft from
Norrach").

Norach (row 117982, Cazic-Thule's palm-tree islet, world 0) sends a player to the Isle of Dread two ways: his quest's
hand-in, The Medallion of the Faceless (101309, step 1, give: the medallion and 10,000 tunar), whose done event
teleports; and, once it is finished, his waysender trip (handlers/waysender.go wayRoutes "Norach", 10,000 a trip),
which lands on the fishing raft south of Graumord's pier, facing the pier (iodRaft, as the Teleporter's Isle of Dread
does). The hand-in's teleport was the import's (tools/quests/data/portal.json): Graumord itself, (24500, 54.1, 3330),
300 north of the raft. Seen on Heirloom 05:10:28 UTC: "Quests: a player finished "The Medallion of the Faceless"",
then ""The Medallion of the Faceless" teleported a player to (24500.0, 54.1, 3330.0) world=0". Every player's first
trip, the one paid with the quest, missed the raft.

--apply: that step's events made the raft's, (24578, 52.6, 3626) facing 3.9, as iodRaft (a server with raft-port
reads "facing"; an older one lands there keeping the player's heading). Checked as found (the step's events exactly the
import's, or already the raft's: nothing to do), else REFUSED and nothing written. Report by default; one transaction,
a gm_audit line, logged in norach_raft_1115_log; --undo puts back exactly what was there.

The quest engine reads queststeps only at its start (or a panel save that changes a row): run it in a swap's
before.sh, or before a restart. Both shards have the row.

    python3 -E -s tools/norach_raft_20261115.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "norach_raft_1115_log"
DETAIL = "Norach's quest hand-in lands on the raft, as his trips do (the owner, 2026-11-15); tools/norach_raft_20261115.py"
WHO, VIA = "dab", "tool"
QUEST, STEP = 101309, 1
OLD = [{"on": "done", "do": "teleport", "world": 0, "x": 24500.0, "y": 54.1, "z": 3330.0}]
NEW = [{"on": "done", "do": "teleport", "world": 0, "x": 24578.0, "y": 52.6, "z": 3626.0, "facing": 3.9}]

LOG_DDL = f"""CREATE TABLE {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    rowkey TEXT NOT NULL,
    before TEXT,
    after  TEXT,
    audit  INTEGER NOT NULL
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def step_row(db):
    cur = db.execute("SELECT * FROM queststeps WHERE questid = ? AND step = ?", (QUEST, STEP))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'queststeps', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, go_json({"questid": QUEST, "step": STEP}),
                      go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def events_of(r):
    try:
        return json.loads(r["events"] or "[]")
    except ValueError:
        return None


def plan(db):
    """(write: the events to write or None, lines, refusals)."""
    for t in ("quests", "queststeps", "gm_audit"):
        if not table(db, t):
            return None, [], [f"no {t} table here"]
    q = db.execute("SELECT name, giver FROM quests WHERE questid = ?", (QUEST,)).fetchone()
    if q is None:
        return None, [f"no quest {QUEST} on this shard: nothing to do"], []
    if q != ("The Medallion of the Faceless", "Norach"):
        return None, [], [f"quest {QUEST} is {q[0]!r} of {q[1]!r}, want The Medallion of the Faceless of Norach"]
    r = step_row(db)
    if r is None or r["kind"] != "give" or r["target"] != "Norach":
        return None, [], [f"quest {QUEST} step {STEP} is {r and (r['kind'], r['target'])}, want Norach's give"]
    evs = events_of(r)
    if evs == NEW:
        return None, [f"quest {QUEST} step {STEP} already lands on the raft: nothing to do"], []
    if evs != OLD:
        return None, [], [f"quest {QUEST} step {STEP}'s events are {r['events']!r}, not the import's {go_json(OLD)}: look first"]
    return NEW, [f"quest {QUEST} step {STEP}: hand-in teleport (24500, 54.1, 3330) -> the raft (24578, 52.6, 3626) facing 3.9"], []


def apply(db):
    want, lines, refusals = plan(db)
    if refusals or want is None:
        return lines, refusals
    if not table(db, LOG):
        db.execute(LOG_DDL)
    before = step_row(db)
    db.execute("UPDATE queststeps SET events = ? WHERE questid = ? AND step = ?",
               (json.dumps(want, separators=(",", ":")), QUEST, STEP))
    after = step_row(db)
    a = audit(db, before, after)
    db.execute(f"INSERT INTO {LOG} (rowkey, before, after, audit) VALUES (?, ?, ?, ?)",
               (go_json({"questid": QUEST, "step": STEP}), go_json(before), go_json(after), a))
    return lines + ["applied"], []


def undo(db):
    if not table(db, LOG):
        return ["nothing applied here: nothing to undo"], []
    logged = db.execute(f"SELECT seq, before, after FROM {LOG} ORDER BY seq DESC").fetchall()
    if not logged:
        return ["nothing applied here: nothing to undo"], []
    now = step_row(db)
    seq, before, after = logged[0]
    if go_json(now) != after:
        return [], [f"quest {QUEST} step {STEP} changed since the apply: {go_json(now)}"]
    b = json.loads(before)
    db.execute("UPDATE queststeps SET events = ? WHERE questid = ? AND step = ?", (b["events"], QUEST, STEP))
    audit(db, now, step_row(db))
    db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
    return [f"quest {QUEST} step {STEP}: events put back to {b['events']}"], []


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.apply or a.undo:
            with db:
                lines, refusals = (apply if a.apply else undo)(db)
                if refusals:
                    raise RuntimeError("; ".join(refusals))
        else:
            _, lines, refusals = plan(db)
    except RuntimeError as e:
        print(f"REFUSED: {e}", file=sys.stderr)
        return 2
    finally:
        db.close()
    for line in lines:
        print(line)
    for r in refusals:
        print(f"REFUSED: {r}", file=sys.stderr)
    return 2 if refusals else 0


if __name__ == "__main__":
    sys.exit(main())
