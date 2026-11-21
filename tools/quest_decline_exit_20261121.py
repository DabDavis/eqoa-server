#!/usr/bin/env python3
"""A quest's decline that reads "Exit" becomes "Not now." (Live petition 5409, 2026-11-21: "quest dialog has a
duplicate exit option when initially speaking to him").

Every question box (0x0034) a client draws carries its own close row under the server's answers: CLIENT's
VIWndResponse::Build adds it, and vanilla's dialog.rs does the same, as WORDS.TXT word 161, which reads "Exit". A quest
whose decline (quests.decline, offerChoices in handlers/quests.go) is the word "Exit" was so shown with two "Exit" rows,
the server's (declines the quest, so the giver offers his next one) and the client's (closes the box, answered 255).
Its decline becomes "Not now.", the words of 1,409 of Live's enabled quests; an empty decline is left as it is (empty sends
no decline answer at all, and a declined quest no longer stands aside for the giver's next).

Report only by default; --apply writes, a gm_audit line a row, the old words kept in quest_decline_exit_20261121_log;
--undo puts back each row still as this tool left it. A second --apply finds nothing to do.

WHEN IT TAKES EFFECT: the quests are read again at a GM panel quest edit or reload, else at the server's next start.

    tools/quest_decline_exit_20261121.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "quest_decline_exit_20261121_log"
DETAIL = "petition 5409: a quest's decline \"Exit\" doubled the client's own Exit row: tools/quest_decline_exit_20261121.py"
ACCOUNT, WHO, VIA = 1, "dab", "tool"
OLD, NEW = "Exit", "Not now."


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, qid, before, after):
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', 'quests', ?, ?, ?, ?)",
               (at, ACCOUNT, WHO, VIA, go_json({"questid": qid}), go_json(before), go_json(after), DETAIL))


def targets(db):
    """The quests whose decline is the word Exit, any case, spaces round it."""
    return [(qid, d) for qid, d in db.execute("SELECT questid, decline FROM quests ORDER BY questid")
            if (d or "").strip().lower() == OLD.lower()]


def apply(db):
    db.execute(f"CREATE TABLE IF NOT EXISTS {LOG} (questid INTEGER PRIMARY KEY, old TEXT NOT NULL, new TEXT NOT NULL, at TEXT NOT NULL)")
    done = 0
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for qid, d in targets(db):
        db.execute("UPDATE quests SET decline = ? WHERE questid = ?", (NEW, qid))
        db.execute(f"INSERT OR REPLACE INTO {LOG} (questid, old, new, at) VALUES (?, ?, ?, ?)", (qid, d, NEW, at))
        audit(db, qid, {"decline": d}, {"decline": NEW})
        done += 1
    return done


def undo(db):
    if db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (LOG,)).fetchone() is None:
        return 0, 0
    back, left = 0, 0
    for qid, old, new in db.execute(f"SELECT questid, old, new FROM {LOG}").fetchall():
        row = db.execute("SELECT decline FROM quests WHERE questid = ?", (qid,)).fetchone()
        if row is None or row[0] != new:
            left += 1
            print(f"  quest {qid}: changed since ({row[0] if row else 'gone'!r}), left")
            continue
        db.execute("UPDATE quests SET decline = ? WHERE questid = ?", (old, qid))
        audit(db, qid, {"decline": new}, {"decline": old})
        db.execute(f"DELETE FROM {LOG} WHERE questid = ?", (qid,))
        back += 1
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, timeout=5)
    try:
        if a.undo:
            back, left = undo(db)
            db.commit()
            print(f"undo: {back} declines put back, {left} left as changed since")
            return 0
        t = targets(db)
        print(f"{len(t)} quests decline with \"{OLD}\": {', '.join(str(q) for q, _ in t[:20])}{' ...' if len(t) > 20 else ''}")
        if a.apply:
            n = apply(db)
            db.commit()
            print(f"applied: {n} declines now \"{NEW}\"")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
