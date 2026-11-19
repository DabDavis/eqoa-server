#!/usr/bin/env python3
"""The catapult's two dye areas (the owner, 2026-11-18: "the catapult pet needs two tone color regions for pet groomer").

A player's construct as Tunaria's catapult prop (0xE64B8D78, tools/catapult_pet_20261117.py) is dyed by the Pet Groomer
in the art his client draws it in (server branch catapult-pet: entity/decorlights.go petDrawnLook, petdyes.go). The prop
has no clothing by the client's rule, so one petdyeareas row gives it two areas by its four materials (palette indices,
read off the disc by vanilla's petart.rs disc test):
  material 0  0x10A8243D  brown planks: the frame, the arm's wooden core, the axle hubs
  material 1  0x6140DAAF  rusted iron: the arm's straps, the spikes, the fittings and the crossbar
  material 2  0xC16FFA97  green-painted boards: the deck's sides
  material 3  0xD0793459  grey stone: the wheels
Area 1 "Wood and stone" (0, 2, 3), area 2 "Iron" (1): materials "1". Vanilla dyes it as any pet's two areas by that mask
(petdye::apply). A PS2 never sees the art, so its dyes stay its body's (the giant spider's) as before.

Checked as found, else REFUSED and nothing written: no petdyeareas row for the catapult other than this tool's. Report
by default; --apply with a gm_audit line, what was there kept in --applied (JSON); --undo puts it back exactly, refused
if the row changed since. Live: at the server's start, or the panel's "Pet dye areas" edit / "Reload every NPC".

    python3 -E -s tools/catapult_dye_20261118.py --db eqoa.db --applied FILE [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

DETAIL = "The catapult's two dye areas, wood and stone / iron (the owner, 2026-11-18): tools/catapult_dye_20261118.py"
WHO, VIA = "claude", "tool"
CATAPULT = 0xE64B8D78
WANT = {"look": CATAPULT, "area1": "Wood and stone", "area2": "Iron", "materials": "1",
        "note": "Tunaria's catapult prop (Zerkziee's construct): materials 0 planks, 2 green boards, 3 stone wheels; 1 iron (the owner 2026-11-18)"}


def row(db):
    cur = db.execute("SELECT * FROM petdyeareas WHERE look = ?", (CATAPULT,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() \
        if db.execute("SELECT 1 FROM sqlite_master WHERE name = 'accountinfo'").fetchone() else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', ?, 'petdyeareas', ?, ?, ?, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"look": CATAPULT}), go_json(before), go_json(after), DETAIL))


def put(db, want):
    before = row(db)
    if want is None:
        db.execute("DELETE FROM petdyeareas WHERE look = ?", (CATAPULT,))
        action = "delete"
    elif before is None:
        db.execute("INSERT INTO petdyeareas (look, area1, area2, materials, note) VALUES (?, ?, ?, ?, ?)",
                   (want["look"], want["area1"], want["area2"], want["materials"], want["note"]))
        action = "insert"
    else:
        db.execute("UPDATE petdyeareas SET area1 = ?, area2 = ?, materials = ?, note = ? WHERE look = ?",
                   (want["area1"], want["area2"], want["materials"], want["note"], CATAPULT))
        action = "update"
    after = row(db)
    audit(db, action, before, after)
    return before, after


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--applied", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        for t in ("petdyeareas", "gm_audit"):
            if not db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone():
                db.execute("ROLLBACK")
                print(f"REFUSED: no {t} table here")
                return 1
        if a.undo:
            if not os.path.exists(a.applied):
                db.execute("ROLLBACK")
                print(f"nothing applied ({a.applied} not there)")
                return 1
            done = json.load(open(a.applied))
            if go_json(row(db)) != go_json(done["after"]):
                db.execute("ROLLBACK")
                print("REFUSED, nothing undone: the row changed since")
                return 1
            put(db, done["before"])
            db.execute("COMMIT")
            os.rename(a.applied, a.applied + ".undone")
            print("undone: the catapult's areas as they were")
            return 0
        have = row(db)
        if have is not None and all(have.get(k) == v for k, v in WANT.items()):
            db.execute("ROLLBACK")
            print("nothing to do: the catapult's areas are there")
            return 0
        if have is not None:
            db.execute("ROLLBACK")
            print(f"REFUSED, nothing written: a petdyeareas row for the catapult is there already: {have}")
            return 1
        print(f"petdyeareas {CATAPULT:#010X}: area 1 {WANT['area1']!r}, area 2 {WANT['area2']!r} (materials {WANT['materials']})")
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: 1 row; --apply to write)")
            return 0
        if os.path.exists(a.applied):
            db.execute("ROLLBACK")
            print(f"applied already ({a.applied}); --undo first")
            return 1
        before, after = put(db, WANT)
        db.execute("COMMIT")
        with open(a.applied, "w") as f:
            json.dump({"before": before, "after": after}, f, indent=1)
        print(f"applied: 1 row, in gm_audit and {a.applied}; read at the server's start (or the panel's Pet dye areas / Reload every NPC)")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
