#!/usr/bin/env python3
"""Overseer Yurgulch says which of the Plane of Disease's bosses are up (petition 1699, Skywalker, 2026-11-12:
"Overseer Yurgulch can be used to check whether Bertoxxulous, Uzrathixius, or Poxathixius spawns are up and available
to kill"; the wiki's Plane of Disease: Bertoxxulous Quest names him).

Writes table npcoverseer (database/npcoverseer.go; handlers/overseer.go reads it at each talk, 30 s cached, the GM
panel's edits at once): three rows for "Overseer Yurgulch", in his voice (a pusling: short, crude, himself by name, as
Honcho Ulgurglch and Pluxklez speak):

  - Bertoxxulous, by trigger "Bertoxxulous Rises" (pod_quests_20261112.py: the bonescepter at his throne): up, the
    throne ready, or the throne resting with when it answers again;
  - Uzrathixius and Poxathixius, by their own rows (npcrespawn's 2 days): up, or dead and when back.

The rows are by names alone (the NPC ids differ between shards). Refused unless Overseer Yurgulch stands in world 5
(GMs made him on live as a copy of Honcho Oozlgurgle, npcs row 165920 on Heirloom), the three bosses' rows are there,
the trigger is, and no row of npcoverseer is his already. The table is made here when the server has not made it yet
(the same schema); --undo then drops it again.

Each row inserted gets a gm_audit line (insert), and the log table overseer_yurgulch_1112_log the row's id and whether
this tool made the table. --undo deletes each row unchanged since (a row a GM edited is left and named), its gm_audit
line (delete), and the table when this tool made it and it is empty.

The data needs no restart (read at the next talk); the code that reads it (npcoverseer, handlers/overseer.go) needs
the new binary. Run on a binary without it, the rows wait, unread.

    tools/overseer_yurgulch_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "overseer_yurgulch_1112_log"
WHO, VIA = "dab", "tool"
SOURCE = "overseer-yurgulch-1112"
DETAIL = ("Overseer Yurgulch says which Plane of Disease bosses are up (petition 1699, Skywalker): "
          "tools/overseer_yurgulch_20261112.py")
OVERSEER, WORLD, TRIGGER = "Overseer Yurgulch", 5, "Bertoxxulous Rises"

SCHEMA = """
	CREATE TABLE IF NOT EXISTS npcoverseer (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		overseer TEXT NOT NULL,
		boss     TEXT NOT NULL,
		world    INTEGER NOT NULL DEFAULT -1,
		trigger  TEXT NOT NULL DEFAULT '',
		ord      INTEGER NOT NULL DEFAULT 0,
		greeting TEXT NOT NULL DEFAULT '',
		up       TEXT NOT NULL DEFAULT '',
		down     TEXT NOT NULL DEFAULT '',
		ready    TEXT NOT NULL DEFAULT '',
		resting  TEXT NOT NULL DEFAULT '',
		unknown  TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);"""

LOG_SCHEMA = f"""CREATE TABLE {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    rowid_  INTEGER NOT NULL,
    row     TEXT NOT NULL,
    made    INTEGER NOT NULL DEFAULT 0,
    audit   INTEGER NOT NULL DEFAULT 0
)"""

COLS = ["overseer", "boss", "world", "trigger", "ord", "greeting", "up", "down", "ready", "resting", "unknown", "note",
        "source"]

# %b the boss, %t when ("soon", "in about 5 hours", "in about 3 days and 16 hours"), %p the player
ROWS = [
    dict(overseer=OVERSEER, boss="Bertoxxulous", world=-1, trigger=TRIGGER, ord=1,
         greeting="Yurgulch watches the masters!",
         up="Bertoxxulous is up on his throne! Go, go!",
         down="", ready="Bertoxxulous sleeps. Throne is ready: bring the bonescepter!",
         resting="Bertoxxulous gone. Throne rests, ready %t.",
         unknown="Bertoxxulous? Yurgulch not know.",
         note="the wiki's Bertoxxulous Quest: he rises at the throne by the bonescepter (trigger Bertoxxulous Rises), "
              "then the throne rests 3 days 16 hours"),
    dict(overseer=OVERSEER, boss="Uzrathixius", world=-1, trigger="", ord=2, greeting="",
         up="Uzrathixius is up! Kill, kill!", down="Uzrathixius dead. Back %t.", ready="", resting="",
         unknown="Uzrathixius? Yurgulch not know.", note="his own row's respawn (npcrespawn, 2 days)"),
    dict(overseer=OVERSEER, boss="Poxathixius", world=-1, trigger="", ord=3, greeting="",
         up="Poxathixius is up! Kill, kill!", down="Poxathixius dead. Back %t.", ready="", resting="",
         unknown="Poxathixius? Yurgulch not know.", note="his own row's respawn (npcrespawn, 2 days)"),
]
for r in ROWS:
    r["source"] = SOURCE


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False, sort_keys=True)


def audit(db, action, rowid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'npcoverseer', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, compact({"id": rowid}),
                      compact(before) if before else "", compact(after) if after else "", DETAIL))
    return cur.lastrowid


def refusals(db):
    bad = []
    n = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = lower(?) AND world = ?", (OVERSEER, WORLD)).fetchone()[0]
    if n < 1:
        bad.append(f"no {OVERSEER!r} in world {WORLD} (make him first: a copy of Honcho Oozlgurgle)")
    for r in ROWS:
        if not db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = lower(?) AND world = ?", (r["boss"], WORLD)).fetchone():
            bad.append(f"no npcs row of {r['boss']!r} in world {WORLD}")
    if not db.execute("SELECT 1 FROM gmtriggers WHERE name = ?", (TRIGGER,)).fetchone():
        bad.append(f"no trigger {TRIGGER!r} (tools/pod_quests_20261112.py --apply first)")
    if table(db, "npcoverseer"):
        n = db.execute("SELECT count(*) FROM npcoverseer WHERE lower(trim(overseer)) = lower(?)", (OVERSEER,)).fetchone()[0]
        if n:
            bad.append(f"npcoverseer has {n} rows of {OVERSEER!r} already")
    return bad


def apply(db):
    made = not table(db, "npcoverseer")
    db.execute(SCHEMA)
    db.execute(LOG_SCHEMA)
    for r in ROWS:
        cur = db.execute(f"INSERT INTO npcoverseer ({', '.join(COLS)}) VALUES ({', '.join('?' for _ in COLS)})",
                         [r[c] for c in COLS])
        rid = cur.lastrowid
        a = audit(db, "insert", rid, None, r)
        db.execute(f"INSERT INTO {LOG} (rowid_, row, made, audit) VALUES (?, ?, ?, ?)", (rid, compact(r), int(made), a))
        print(f"npcoverseer {rid}: {r['overseer']} -> {r['boss']}")


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    made = False
    for _, rid, row, m, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        made = made or bool(m)
        want = json.loads(row)
        got = db.execute(f"SELECT {', '.join(COLS)} FROM npcoverseer WHERE id = ?", (rid,)).fetchone()
        if got is None or dict(zip(COLS, got)) != want:
            print(f"left: npcoverseer {rid} ({want['boss']}) {'gone' if got is None else 'changed'} since")
            left += 1
            continue
        db.execute("DELETE FROM npcoverseer WHERE id = ?", (rid,))
        audit(db, "delete", rid, want, None)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
        if made and db.execute("SELECT count(*) FROM npcoverseer").fetchone()[0] == 0:
            db.execute("DROP TABLE npcoverseer")
            if table(db, "sqlite_sequence"):
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcoverseer'")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
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
            print(f"deleted {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do")
            return 0
        bad = refusals(db)
        if bad:
            print("refused, nothing done:")
            for b in bad:
                print("  " + b)
            return 1
        for r in ROWS:
            print(f"{r['overseer']} -> {r['boss']}" + (f" (trigger {r['trigger']})" if r["trigger"] else ""))
        if not a.apply:
            print(f"report only: {len(ROWS)} rows of npcoverseer would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(ROWS)} rows; no restart for the rows (read at the next talk), the new binary to read them")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
