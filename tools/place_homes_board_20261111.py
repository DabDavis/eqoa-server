#!/usr/bin/env python3
"""Stand the Real Estate Listings (handlers/homes.go, entity/homes.go; the owner, 2026-11-11: "add a bulletin board in
freeport near dabrish that says Real Estate Listings"): two rows of npcs named "Real Estate Listings", a bulletin
board talked to and never attacked (npc_type 384, as the Guild Board's), level 1:

  - in Freeport, on open ground near Dabrish: the first free spot of SPOTS round him (each some units in front of or
    behind him along his heading and to a side), facing the way he does; a spot is free when no other NPC's row is
    within ROOM (the Teleporter, the Mercenary Recruitment board, the PvP Events herald, his Armor Dyer stand within
    12 of him) and the Summoning Portal's landing (3 in front of Dabrish, summonportal.go) is 6 or more off;
  - its twin in the yard of the homes' world (world.homes.world, 13), where the recipe's "home" block's "board" says
    (tools/worlds/homesteads.json): it stands in every home (an NPC of the world's own is in every instance), and its
    box there is the way out.

The body is a single parameter, --model (default MODEL, 0x6DE5B2CC: the Guild Board's; the owner wants Highpass's
board with papers on it once its DictID is known), at --size (default 0.3, the Guild Board's). npcmove: stands;
npcaggro: never on sight. Refused (nothing done) where a board stands already or Dabrish or the template is not as
expected. The server reads npcs at its start: run it in a keep restart's before.sh (with world.homes on).
gm_audit rows; --undo exact (refused when a row changed since).

    python3 -E -s tools/place_homes_board_20261111.py --db eqoa.db [--apply | --undo] [--model 0x6DE5B2CC] [--size 0.3]
        [--recipe tools/worlds/homesteads.json] [--applied FILE]
"""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

NAME, SRC = "Real Estate Listings", "homes board 20261111"
BESIDE, TEMPLATE = "Dabrish", "Guild Board"
MODEL, SIZE = 0x6DE5B2CC, 0.3
LEVEL, TYPE = 1, 384
ROOM, PORTAL_ROOM = 4.0, 6.0
HOMES_WORLD = 13
# (front, side): units in front of Dabrish along his heading (negative: behind), and to his right. His Teleporter
# stands 8 to one side, the Mercenary Recruitment board 12 to the other, the PvP Events herald 5 behind.
SPOTS = [(6, 14), (6, -16), (10, 14), (10, -16), (0, 18), (0, -20), (-10, 14), (-10, -16), (14, 0)]
HERE = os.path.dirname(os.path.abspath(__file__))


def row(db, tbl, where, args):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def audit(db, now, action, tbl, key, before, after):
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)""",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True),
                None if after is None else json.dumps(after, sort_keys=True),
                "the owner, 2026-11-11: the Real Estate Listings of the players' homes; tools/place_homes_board_20261111.py"))


def spot(d, front, side):
    """front in front of Dabrish (his front is CLIENT's heading (sin h, cos h), h his facing byte) and side to his right."""
    h = d["facing"] * 2 * math.pi / 256
    fx, fz = math.sin(h), math.cos(h)
    rx, rz = math.cos(h), -math.sin(h)
    return d["x_coord"] + front * fx + side * rx, d["y_coord"], d["z_coord"] + front * fz + side * rz


def taken(db, world, x, z, d=None):
    """Why the spot is taken, or None."""
    near = db.execute("SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 30 AND abs(z_coord - ?) < 30",
                      (world, x, z)).fetchall()
    for n, nx, nz in near:
        dist = math.hypot(nx - x, nz - z)
        if dist < ROOM:
            return f"{n} stands {dist:.1f} from it"
    if d is not None:
        h = d["facing"] * 2 * math.pi / 256
        px, pz = d["x_coord"] + 3 * math.sin(h), d["z_coord"] + 3 * math.cos(h)
        if math.hypot(px - x, pz - z) < PORTAL_ROOM:
            return f"it is {math.hypot(px - x, pz - z):.1f} from the Summoning Portal's landing"
    return None


def yard_board(path):
    """The yard's board's spot (x, y, z, facing byte) from the recipe's home block."""
    j = json.load(open(path))
    b = (j.get("home") or {}).get("board")
    if not b:
        sys.exit(f"{path} has no home.board: nothing done")
    facing = b.get("facing")
    if facing is None and b.get("toward"):
        tx, tz = b["toward"]
        facing = math.degrees(math.atan2(tx - b["x"], tz - b["z"]))
    byte = int(round((facing or 0) % 360 / 360 * 256)) % 256
    return b["x"], b["y"], b["z"], byte, j.get("world", HOMES_WORLD)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--applied", default=os.path.join(HERE, "place_homes_board_applied.json"))
    ap.add_argument("--recipe", default=os.path.join(HERE, "worlds", "homesteads.json"))
    ap.add_argument("--model", default=hex(MODEL), help="the board's body, a DictID (default the Guild Board's)")
    ap.add_argument("--size", type=float, default=SIZE)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    model = int(a.model, 0) & 0xFFFFFFFF
    db = sqlite3.connect(a.db, timeout=30)
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

    if a.undo:
        if not os.path.exists(a.applied):
            sys.exit("nothing applied: no " + a.applied)
        j = json.load(open(a.applied))
        for k in j["rows"]:
            r = row(db, "npcs", "id = ?", (k["id"],))
            if r is None or r["npc_name"] != NAME:
                sys.exit(f"npcs {k['id']} is {r and r['npc_name']}: nothing done")
            if json.dumps(r, sort_keys=True) != k["row"]:
                sys.exit(f"npcs {k['id']} changed since (a GM's edit): nothing done; undo it in the GM panel")
        with db:
            dels = [("npcs", "id = ?", (k["id"],)) for k in j["rows"]] + [
                ("npcmove", "name = ? AND source = ?", (NAME.lower(), SRC)), ("npcaggro", "npc_name = ? AND source = ?", (NAME, SRC))]
            for tbl, where, args in dels:
                cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
                cols = [d[0] for d in cur.description]
                for x in cur.fetchall():
                    audit(db, now, "delete", tbl, {"where": where, "args": list(args)}, dict(zip(cols, x)), None)
                db.execute(f"DELETE FROM {tbl} WHERE {where}", args)
            db.execute("UPDATE sqlite_sequence SET seq = (SELECT COALESCE(MAX(id), 0) FROM npcs) WHERE name = 'npcs'")
        os.rename(a.applied, a.applied + ".undone-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()))
        print(f"undone: npcs {', '.join(str(k['id']) for k in j['rows'])}, their npcmove and npcaggro rows")
        return

    if a.apply and os.path.exists(a.applied):
        sys.exit("applied already: --undo first")
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (NAME.lower(),)).fetchone()[0]:
        sys.exit("a Real Estate Listings board stands already: nothing done")
    if db.execute("SELECT 1 FROM npcmove WHERE name = ?", (NAME.lower(),)).fetchone() or \
            db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = ?", (NAME.lower(),)).fetchone():
        sys.exit("the board has an npcmove or npcaggro row already: nothing done")
    ds = db.execute("SELECT id FROM npcs WHERE npc_name = ? ORDER BY id", (BESIDE,)).fetchall()
    if len(ds) != 1:
        sys.exit(f"{len(ds)} rows named {BESIDE}: nothing done")
    d = row(db, "npcs", "id = ?", (ds[0][0],))
    t = row(db, "npcs", "npc_name = ? AND world = 0 ORDER BY id", (TEMPLATE,))
    if t is None:
        sys.exit(f"no {TEMPLATE} row to copy: nothing done")
    yx, yy, yz, yf, hw = yard_board(a.recipe)
    chosen = None
    for front, side in SPOTS:
        x, y, z = spot(d, front, side)
        why = taken(db, d["world"], x, z, d)
        print(f"  {front:+g} in front, {side:+g} to the right: ({x:.2f}, {z:.2f}): {why or 'free'}")
        if why is None:
            chosen = (x, y, z, front, side)
            break
    if chosen is None:
        sys.exit("no spot round Dabrish is free: nothing done")
    if (why := taken(db, hw, yx, yz)) is not None:
        sys.exit(f"the yard's board spot is taken: {why}: nothing done")
    x, y, z, front, side = chosen
    cols = [c for c in t if c != "id"]
    made_rows = []
    for (w, px, py, pz, pf, zone) in [(d["world"], x, y, z, d["facing"], d["zone"]), (hw, yx, yy, yz, yf, 0)]:
        nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (t["id"],)).lastrowid
        db.execute("""UPDATE npcs SET npc_name = ?, zone = ?, unk_a = 0, unk_b = 0, world = ?, x_coord = ?, y_coord = ?, z_coord = ?,
                      facing = ?, npc_type = ?, npc_level = ?, npc_id = ?, modelid = ?, size = ? WHERE id = ?""",
                   (NAME, zone, w, round(px, 3), py, round(pz, 3), pf, TYPE, LEVEL, nid, model,
                    a.size, nid))
        made = row(db, "npcs", "id = ?", (nid,))
        audit(db, now, "insert", "npcs", {"id": nid}, None, made)
        made_rows.append(made)
        print(f"npcs {nid}: {NAME} at world {w} ({px:.3f}, {py}, {pz:.3f}) facing {pf}, body {model:#010x} at {a.size}")
    db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (the Real Estate Listings board)', ?)",
               (NAME.lower(), SRC))
    audit(db, now, "insert", "npcmove", {"name": NAME.lower()}, None, {"roam": 0})
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Real Estate Listings board: talks only', ?)",
               (NAME, SRC))
    audit(db, now, "insert", "npcaggro", {"npc_name": NAME}, None, {"aggressive": 0})
    print(f"Freeport's: {front:+g} in front of and {side:+g} to the right of {BESIDE} (npcs {d['id']} at ({d['x_coord']}, {d['z_coord']})), "
          f"a copy of {TEMPLATE} {t['id']}")
    if a.apply:
        db.commit()
        json.dump({"rows": [{"id": m["id"], "row": json.dumps(m, sort_keys=True)} for m in made_rows]}, open(a.applied, "w"))
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
