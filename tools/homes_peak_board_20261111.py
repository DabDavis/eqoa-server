#!/usr/bin/env python3
"""Move the yard's Real Estate Listings (the board's twin in world 13, tools/place_homes_board_20261111.py) to where
the Homesteads' recipe says it stands now (tools/worlds/homesteads.json's home.board): the owner, 2026-11-11, "for
player homes lets add natural barriers not the tele back to the house. lets make it on a mountain top high and yard
is surrounded by a fence or wall": world 13 is now Tunaria zone 55's crater rim with the cottage carried there, so the
yard's board goes with it. The row is moved in place (its id, body and everything else kept): x, y, z, facing (toward
the door spot), zone. Refused (nothing done) where no single world-13 board stands, or it stands neither at the old
spot (5812, 57.72, 18142) nor at the new one; a second --apply finds it at the new one and does nothing. gm_audit rows;
--undo puts it back exactly (refused when the row changed since). The server reads npcs at its start: run it in a keep
restart's before.sh with the new world files in place.

    python3 -E -s tools/homes_peak_board_20261111.py --db eqoa.db [--apply | --undo] [--recipe tools/worlds/homesteads.json]
        [--applied FILE]
"""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

NAME = "Real Estate Listings"
OLD = (5812.0, 57.72, 18142.0)
HERE = os.path.dirname(os.path.abspath(__file__))


def row(db, where, args):
    cur = db.execute(f"SELECT * FROM npcs WHERE {where}", args)
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def audit(db, now, before, after):
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', 'update', 'npcs', ?, ?, ?, ?)""",
               (now, json.dumps({"id": before["id"]}), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True),
                "the owner, 2026-11-11: the homes on a mountain top; the yard's board moved with the cottage; "
                "tools/homes_peak_board_20261111.py"))


def near(r, p):
    return math.hypot(r["x_coord"] - p[0], r["z_coord"] - p[2]) < 0.5 and abs(r["y_coord"] - p[1]) < 0.5


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--recipe", default=os.path.join(HERE, "worlds", "homesteads.json"))
    ap.add_argument("--applied", default=os.path.join(HERE, "homes_peak_board_applied.json"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    r = json.load(open(a.recipe))
    world, b = r.get("world", 13), r["home"]["board"]
    if "toward" in b:
        deg = math.degrees(math.atan2(b["toward"][0] - b["x"], b["toward"][1] - b["z"]))
    else:
        deg = b.get("facing", 0)
    new = (b["x"], b["y"], b["z"])
    facing = int(round(deg % 360 / 360 * 256)) % 256

    if a.undo:
        if not os.path.exists(a.applied):
            sys.exit("nothing applied: no " + a.applied)
        j = json.load(open(a.applied))
        cur = row(db, "id = ?", (j["id"],))
        if cur is None or json.dumps(cur, sort_keys=True) != j["after"]:
            sys.exit(f"npcs {j['id']} changed since (or is gone): nothing done")
        before = json.loads(j["before"])
        with db:
            sets = ", ".join(f"{k} = ?" for k in before if k != "id")
            db.execute(f"UPDATE npcs SET {sets} WHERE id = ?", [before[k] for k in before if k != "id"] + [j["id"]])
            audit(db, now, cur, row(db, "id = ?", (j["id"],)))
        os.rename(a.applied, a.applied + ".undone-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()))
        print(f"undone: npcs {j['id']} back at ({before['x_coord']}, {before['y_coord']}, {before['z_coord']}) facing {before['facing']}")
        return

    rows = db.execute("SELECT id FROM npcs WHERE lower(trim(npc_name)) = ? AND world = ?", (NAME.lower(), world)).fetchall()
    if len(rows) != 1:
        sys.exit(f"{len(rows)} {NAME} rows in world {world}: nothing done")
    cur = row(db, "id = ?", (rows[0][0],))
    if near(cur, new):
        print(f"npcs {cur['id']} stands at the recipe's board already ({cur['x_coord']}, {cur['y_coord']}, {cur['z_coord']}): nothing to do")
        return
    if not near(cur, OLD):
        sys.exit(f"npcs {cur['id']} stands at ({cur['x_coord']}, {cur['y_coord']}, {cur['z_coord']}), neither the old spot nor the new: nothing done")
    if a.apply and os.path.exists(a.applied):
        sys.exit("applied already: --undo first")
    db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, zone = 0 WHERE id = ?",
               (new[0], new[1], new[2], facing, cur["id"]))
    after = row(db, "id = ?", (cur["id"],))
    audit(db, now, cur, after)
    print(f"npcs {cur['id']}: {NAME} in world {world} from ({cur['x_coord']}, {cur['y_coord']}, {cur['z_coord']}) facing {cur['facing']} "
          f"to ({new[0]}, {new[1]}, {new[2]}) facing {facing}")
    if a.apply:
        db.commit()
        json.dump({"id": cur["id"], "before": json.dumps(cur, sort_keys=True), "after": json.dumps(after, sort_keys=True)}, open(a.applied, "w"))
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
