#!/usr/bin/env python3
"""Were Forms Without Illusion, a mastery of no cost (the user, 2026-11-08:
"lets add a CM 0 Cost that gives you the same Were Forms, but without the
illusion").

One rank in cmcatalog: "Were Forms Without Illusion", the General list's
(category 0), any class and race, from level 30 (the forms' own level), cost
0, for a were who has turned: require_quests is one group of the five lines'
third quests (the wolfwere's, bearwere's, ratwere's, lionwere's and
gatorwere's p3, by their import keys in quest_import_quests: any one of them
finished), as the Feral form masteries ask each its own. Its effects are
"NoWereIllusion:1", which no stat slot reads: the server keeps its owner in
its own body while a were form is on (internal/entity/wereownbody.go), the
form's stats and time as they were. Removing the rank in the window (cost 0,
nothing lost) shows the beast again.

The rank goes in at the id given (default 900179; refused when another row
holds it) and is written to gm_audit as an insert, so the GM panel lists it
and can take it back. --undo deletes it, with every charcms row of it (each
cost 0: no points to give back), as long as it is the row this wrote (its
source); a second --apply is refused while it is there. Report only by
default. The server reads the catalogue at its start and at a GM's edit:
run this with the server stopped (a keep restart's before.sh), or restart
it after.

    tools/seed_were_own_body.py [--db eqoa.db] [--id 900179] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
DEFAULT_ID = 900179
SOURCE = "tools/seed_were_own_body.py"
LINES = ["wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"]
KEY = "Weres/{line}/p3"
NAME = "Were Forms Without Illusion"
DESCRIPTION = ("Your were forms give you their power without changing your form: you keep your own body. "
               "Remove this mastery to take your beast's shape again.")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def audit(db, action, key, before, after, detail):
    if not has_table(db, "gm_audit"):
        return
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', ?, 'cmcatalog', ?, ?, ?, ?)""",
               (now(), action, json.dumps(key), None if before is None else json.dumps(before),
                None if after is None else json.dumps(after), f"{detail}; {SOURCE}"))


def quests(db):
    """The five lines' third quests, by their import keys; a line missing is reported."""
    found, missing = [], []
    for line in LINES:
        r = db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (KEY.format(line=line),)).fetchone()
        if r is None:
            missing.append(line)
        else:
            found.append(r[0])
    return found, missing


def apply(db, rid, write):
    have = db.execute("SELECT id, name, source FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
    if have is not None:
        if have[2] == SOURCE:
            sys.exit(f"rank {rid} ({have[1]!r}) is in already: --undo first")
        sys.exit(f"rank {rid} is another row ({have[1]!r}, source {have[2]!r}): give --id another")
    found, missing = quests(db)
    if missing:
        sys.exit(f"no third quest imported for {', '.join(missing)}: nothing done")
    row = {"id": rid, "category": 0, "classes": 32767, "races": 2047, "mastery": NAME, "rank": 1, "name": NAME,
           "parent": 0, "cost": 0, "level": 30, "spent": 0, "require_quests": "|".join(str(q) for q in found),
           "effects": "NoWereIllusion:1", "description": DESCRIPTION, "enabled": 1, "source": SOURCE}
    cols = list(row)
    db.execute(f"INSERT INTO cmcatalog ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    audit(db, "insert", {"id": rid}, None, row, "Were Forms Without Illusion, cost 0 (the user's)")
    print(f"rank {rid}: {NAME!r}, cost 0, level 30, any were's third quest ({row['require_quests']})")
    if write:
        db.commit()
        print("applied: restart the server (or run this in a keep restart's before.sh)")
    else:
        db.rollback()
        print("report only: nothing written")


def undo(db, rid, write):
    have = db.execute("SELECT * FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
    if have is None:
        sys.exit(f"no rank {rid}: nothing to undo")
    cols = [d[0] for d in db.execute("SELECT * FROM cmcatalog LIMIT 0").description]
    row = dict(zip(cols, have))
    if row.get("source") != SOURCE:
        sys.exit(f"rank {rid} is not this tool's (source {row.get('source')!r}): left")
    owners = db.execute("SELECT count(*) FROM charcms WHERE cmid = ?", (rid,)).fetchone()[0] if has_table(db, "charcms") else 0
    if owners:
        db.execute("DELETE FROM charcms WHERE cmid = ?", (rid,))
    db.execute("DELETE FROM cmcatalog WHERE id = ?", (rid,))
    audit(db, "delete", {"id": rid}, row, None, f"Were Forms Without Illusion taken out, from {owners} owners (cost 0)")
    print(f"rank {rid} out, and from {owners} characters (cost 0: no points back)")
    if write:
        db.commit()
        print("undone: restart the server")
    else:
        db.rollback()
        print("report only: nothing written")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--id", type=int, default=DEFAULT_ID)
    m = ap.add_mutually_exclusive_group()
    m.add_argument("--apply", action="store_true")
    m.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    if a.undo:
        undo(db, a.id, True)
    else:
        apply(db, a.id, a.apply)


if __name__ == "__main__":
    main()
