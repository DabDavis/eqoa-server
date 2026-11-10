#!/usr/bin/env python3
"""Those who left a home by its gatekeeper before the home took the cities' template keep their passage.

A home's gatekeeper flies whoever has finished its starter quests (homelands/<race>/flight, repeatable). The
starter quests were two (welcome and the hunt) until 2026-11-10 and are five since (L1..L5: tools/quests/
gen_homelands.py); tools/retire_quests.py takes the old two out, the marks of those who finished them too. One
who had finished them and flown would then be refused at the gate on a visit home, and offered a fledgling's five
quests at any level. This marks L1..L5 finished for each character whose flight is finished, in every home whose
five are imported: it flew under the rules of its day, and its city's own class quests are its to do as before
(a mark sets off no event: nothing is given and no city quest is finished for it).

One who had not flown yet (the old two under way or finished) is left alone: its old lines go with the retiring
and the elder offers it L1, the whole chain with its rewards.

Run it after the import (the five need their ids), before or after the retiring. Each row added is logged in
homes_flown_log; --undo takes them out and drops the log. A character that has a row for one of the five keeps it.
A report by default; --apply writes. The server stopped, or the characters named offline (a character's quests are
read at its login).

    tools/homes_flown.py --db eqoa.db             the report
    tools/homes_flown.py --db eqoa.db --apply     mark them
    tools/homes_flown.py --db eqoa.db --undo      take the marks back
"""
import argparse
import datetime
import re
import sqlite3
import sys

LOG = "homes_flown_log"
LEVELS = ("L1", "L2", "L3", "L4", "L5")


def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def plan(db):
    """[(serverid, questid, key)]: the finished marks to add."""
    if not has_table(db, "quest_import_quests"):
        raise SystemExit("no quest import in this database")
    keys = dict(db.execute("SELECT key, questid FROM quest_import_quests WHERE key LIKE 'homelands/%'"))
    alive = None
    if has_table(db, "characters"):
        alive = {r[0] for r in db.execute("SELECT serverid FROM characters")}
    out = []
    for key in sorted(keys):
        m = re.fullmatch(r"homelands/([^/]+)/flight", key)
        if not m:
            continue
        five = [f"homelands/{m.group(1)}/{lv}" for lv in LEVELS]
        if not all(k in keys for k in five):
            continue  # a home still on its old quests
        for (sid,) in db.execute("SELECT serverid FROM charquests WHERE questid = ? AND done > 0 ORDER BY serverid", (keys[key],)).fetchall():
            if alive is not None and sid not in alive:
                continue
            for k in five:
                if not db.execute("SELECT 1 FROM charquests WHERE serverid = ? AND questid = ?", (sid, keys[k])).fetchone():
                    out.append((sid, keys[k], k))
    return out


def apply(db, rows):
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("BEGIN IMMEDIATE")
    try:
        db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (serverid INTEGER NOT NULL, questid INTEGER NOT NULL, PRIMARY KEY (serverid, questid))")
        for sid, qid, _ in rows:
            db.execute("INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks) VALUES (?, ?, 0, 0, 1, 0, ?, '')",
                       (sid, qid, now))
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?)", (sid, qid))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise


def undo(db):
    if not has_table(db, LOG):
        raise SystemExit("no mark of this tool's here: nothing to undo")
    db.execute("BEGIN IMMEDIATE")
    try:
        n = db.execute(f"DELETE FROM charquests WHERE (serverid, questid) IN (SELECT serverid, questid FROM {LOG})").rowcount
        db.execute(f"DROP TABLE {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return n


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    if a.undo:
        print(f"{a.db}: {undo(db)} marks taken back")
        return 0
    rows = plan(db)
    for sid in sorted({r[0] for r in rows}):
        print(f"  character {sid}: " + ", ".join(k for s, _, k in rows if s == sid))
    if a.apply:
        apply(db, rows)
        print(f"{a.db}: {len(rows)} finished marks added, logged in {LOG}")
    else:
        print(f"{len(rows)} finished marks to add (--apply adds them)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
