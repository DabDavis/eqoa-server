#!/usr/bin/env python3
"""Stand the Teleporter (handlers/teleporter.go; the owner, 2026-11-09: "lets add a portal npc named Teleporter. has
destinations of all raids and camps", "put it next to dabrish") beside Dabrish in Freeport: a row of npcs named
Teleporter, talked to and never attacked (npc_type 128), at level 60, in a waysender's look (a copy of Waysender
Ilganis's row: the Erudite body 839650119 in a robe, a staff in hand), on Dabrish's line OUT units to the side his
Armor Dyer stands on (4 off him: the Teleporter beyond it), facing the way he does, at his height; npcmove: stands;
npcaggro: never on sight. Refused (nothing done) where the spot is within 3.5 of another NPC's row, within 6 of the
Summoning Portal's landing (3 in front of Dabrish, summonportal.go) or within 10 of Mr. Moogie, where a Teleporter
stands already, or where Dabrish or the template is not as expected. The server reads npcs at its start: run it in
a keep restart's before.sh (and world.teleporter on). gm_audit rows; --undo exact.

    python3 -E -s tools/place_teleporter.py --db eqoa.db [--apply | --undo] [--applied FILE] [--out 8]
"""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

NAME, SRC = "Teleporter", "teleporter 20261109"
BESIDE, TEMPLATE, BODY = "Dabrish", "Waysender Ilganis", 839650119
LEVEL, TYPE = 60, 128
ROOM, PORTAL_ROOM, MOOGIE_ROOM = 3.5, 6.0, 10.0


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
                "the owner, 2026-11-09: the Teleporter, every raid and camp, beside Dabrish; tools/place_teleporter.py"))


def spot(d, out):
    """OUT along Dabrish's side (-cos h, sin h): his facing byte h, CLIENT's heading (sin h, cos h) his front."""
    h = d["facing"] * 2 * math.pi / 256
    return d["x_coord"] - out * math.cos(h), d["y_coord"], d["z_coord"] + out * math.sin(h)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--applied", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "place_teleporter_applied.json"))
    ap.add_argument("--out", type=float, default=8.0, help="how far to Dabrish's side (the Armor Dyer's is 4)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

    if a.undo:
        if not os.path.exists(a.applied):
            sys.exit("nothing applied: no " + a.applied)
        j = json.load(open(a.applied))
        r = row(db, "npcs", "id = ?", (j["id"],))
        if r is None or r["npc_name"] != NAME:
            sys.exit(f"npcs {j['id']} is {r and r['npc_name']}: nothing done")
        if json.dumps(r, sort_keys=True) != j["row"]:
            sys.exit(f"npcs {j['id']} changed since (a GM's edit): nothing done; undo it in the GM panel")
        with db:
            for tbl, where, args in [("npcs", "id = ?", (j["id"],)), ("npcmove", "name = ? AND source = ?", (NAME.lower(), SRC)),
                                     ("npcaggro", "npc_name = ? AND source = ?", (NAME, SRC))]:
                cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
                cols = [d[0] for d in cur.description]
                for x in cur.fetchall():
                    audit(db, now, "delete", tbl, {"where": where, "args": list(args)}, dict(zip(cols, x)), None)
                db.execute(f"DELETE FROM {tbl} WHERE {where}", args)
            db.execute("UPDATE sqlite_sequence SET seq = (SELECT COALESCE(MAX(id), 0) FROM npcs) WHERE name = 'npcs'")
        os.rename(a.applied, a.applied + ".undone-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()))
        print(f"undone: npcs {j['id']}, its npcmove and npcaggro rows")
        return

    if a.apply and os.path.exists(a.applied):
        sys.exit("applied already: --undo first")
    if db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = ?", (NAME.lower(),)).fetchone()[0]:
        sys.exit("a Teleporter stands already: nothing done")
    if db.execute("SELECT 1 FROM npcmove WHERE name = ?", (NAME.lower(),)).fetchone() or \
            db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = ?", (NAME.lower(),)).fetchone():
        sys.exit("the Teleporter has an npcmove or npcaggro row already: nothing done")
    ds = db.execute("SELECT id FROM npcs WHERE npc_name = ? ORDER BY id", (BESIDE,)).fetchall()
    if len(ds) != 1:
        sys.exit(f"{len(ds)} rows named {BESIDE}: nothing done")
    d = row(db, "npcs", "id = ?", (ds[0][0],))
    t = row(db, "npcs", "npc_name = ? ORDER BY id", (TEMPLATE,))
    if t is None or t["modelid"] != BODY:
        sys.exit(f"{TEMPLATE} is {t and t['modelid']}, not the body {BODY}: nothing done")
    x, y, z = spot(d, a.out)
    w = d["world"]
    near = db.execute("""SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 30 AND abs(z_coord - ?) < 30""",
                      (w, x, z)).fetchall()
    for n, nx, nz in near:
        dist = math.hypot(nx - x, nz - z)
        if dist < ROOM:
            sys.exit(f"{n} stands {dist:.1f} from the spot ({x:.2f}, {z:.2f}): nothing done")
        if n == "Mr. Moogie" and dist < MOOGIE_ROOM:
            sys.exit(f"Mr. Moogie stands {dist:.1f} from the spot: nothing done")
    h = d["facing"] * 2 * math.pi / 256
    px, pz = d["x_coord"] + 3 * math.sin(h), d["z_coord"] + 3 * math.cos(h)
    if math.hypot(px - x, pz - z) < PORTAL_ROOM:
        sys.exit(f"the spot is {math.hypot(px - x, pz - z):.1f} from the Summoning Portal's landing: nothing done")

    cols = [c for c in t if c != "id"]
    nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (t["id"],)).lastrowid
    db.execute("""UPDATE npcs SET npc_name = ?, zone = ?, unk_a = 0, unk_b = 0, world = ?, x_coord = ?, y_coord = ?, z_coord = ?,
                  facing = ?, npc_type = ?, npc_level = ?, npc_id = ?, size = 1.0 WHERE id = ?""",
               (NAME, d["zone"], w, round(x, 3), y, round(z, 3), d["facing"], TYPE, LEVEL, nid, nid))
    made = row(db, "npcs", "id = ?", (nid,))
    audit(db, now, "insert", "npcs", {"id": nid}, None, made)
    db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (the Teleporter)', ?)",
               (NAME.lower(), SRC))
    audit(db, now, "insert", "npcmove", {"name": NAME.lower()}, None, {"roam": 0})
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Teleporter: talks only', ?)",
               (NAME, SRC))
    audit(db, now, "insert", "npcaggro", {"npc_name": NAME}, None, {"aggressive": 0})
    print(f"npcs {nid}: {NAME} at world {w} ({x:.3f}, {y}, {z:.3f}) facing {d['facing']}, {a.out:g} to {BESIDE}'s side "
          f"(npcs {d['id']} at ({d['x_coord']}, {d['z_coord']})), the body of {TEMPLATE} {t['id']}")
    for n, nx, nz in sorted(near, key=lambda r: math.hypot(r[1] - x, r[2] - z))[:6]:
        print(f"  {math.hypot(nx - x, nz - z):5.1f} from {n}")
    if a.apply:
        db.commit()
        json.dump({"id": nid, "row": json.dumps(made, sort_keys=True)}, open(a.applied, "w"))
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
