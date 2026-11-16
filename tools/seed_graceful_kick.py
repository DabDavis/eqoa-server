#!/usr/bin/env python3
"""Graceful Kick, a bard's mastery of no cost (the owner, 2026-11-16: "lets
add a 0 cost cm for bards graceful strike to kick animation").

One rank in cmcatalog: "Graceful Kick", the bard's own class list (category
3, classes 1 << 5), any race, from the level the bard gets Graceful Strike at
(its spellpattern record's abilitylvl: 16), cost 0. Its effects are
"GracefulKick:1", which no stat slot reads: the server plays its owner's
Graceful Strike with the Kick art's visual (SPELLFX "Kick", 0x0A286E2E: the
kick, SetAction 43) in place of the art's own swing (internal/entity/
gracefulkick.go); the art's damage and timing are its own. Removing the rank
in the window (cost 0, nothing lost) plays the swing again.

The rank goes in at the id given (default 900916; refused when another row
holds it) and is written to gm_audit as an insert, so the GM panel lists it
and can take it back. Refused when the database has no Graceful Strike.
--undo deletes it, with every charcms row of it (each cost 0: no points to
give back), as long as it is the row this wrote (its source); a second
--apply is refused while it is there. Report only by default. The server
reads the catalogue at its start and at a GM's edit: run this with the
server stopped (a keep restart's before.sh), or restart it after.

    tools/seed_graceful_kick.py [--db eqoa.db] [--id 900916] [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
DEFAULT_ID = 900916
SOURCE = "tools/seed_graceful_kick.py"
BARD = 5
NAME = "Graceful Kick"
ART = "Graceful Strike"
DESCRIPTION = ("Your Graceful Strike is delivered as a kick. It strikes as it always has: only the motion changes. "
               "Remove this mastery to swing your weapon again.")


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


def art_level(db):
    """The lowest level a Graceful Strike record has, None for none."""
    r = db.execute("SELECT min(abilitylvl), count(*) FROM spellpattern WHERE spellname = ?", (ART,)).fetchone()
    return r[0] if r and r[1] else None


def apply(db, rid, write):
    have = db.execute("SELECT id, name, source FROM cmcatalog WHERE id = ?", (rid,)).fetchone()
    if have is not None:
        if have[2] == SOURCE:
            sys.exit(f"rank {rid} ({have[1]!r}) is in already: --undo first")
        sys.exit(f"rank {rid} is another row ({have[1]!r}, source {have[2]!r}): give --id another")
    level = art_level(db)
    if level is None:
        sys.exit(f"no {ART!r} in spellpattern: nothing done")
    level = max(1, int(level))
    row = {"id": rid, "category": 3, "classes": 1 << BARD, "races": 2047, "mastery": NAME, "rank": 1, "name": NAME,
           "parent": 0, "cost": 0, "level": level, "spent": 0, "effects": "GracefulKick:1",
           "description": DESCRIPTION, "enabled": 1, "source": SOURCE}
    cols = list(row)
    db.execute(f"INSERT INTO cmcatalog ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [row[c] for c in cols])
    audit(db, "insert", {"id": rid}, None, row, "Graceful Kick, cost 0 (the owner's)")
    print(f"rank {rid}: {NAME!r}, bard's class list, cost 0, level {level} ({ART}'s)")
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
    audit(db, "delete", {"id": rid}, row, None, f"Graceful Kick taken out, from {owners} owners (cost 0)")
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
