#!/usr/bin/env python3
"""Stand the PvP Events herald (handlers/ctf.go, entity/ctf.go; the owner, 2026-11-10: "players will join from PvP
Events NPC next to Dabrish") beside Dabrish in Freeport: a row of npcs named "PvP Events", talked to and never
attacked (npc_type 128), at level 60, in a war captain's look (a copy of Captain Maehler's row: the human body in
plate, a sword and a shield), facing the way Dabrish does, at his height, on the first free spot of SPOTS round him
(each a step back or to a side of his own heading, never in front, where the Summoning Portal lands its arrivals);
npcmove: stands; npcaggro: never on sight. A spot is free when no other NPC's row is within 3.5 of it, the Summoning
Portal's landing (3 in front of Dabrish, summonportal.go) is 6 or more off and Mr. Moogie 10 or more. Refused
(nothing done) where a herald stands already, where no spot is free, or where Dabrish or the template is not as
expected. The server reads npcs at its start: run it in a keep restart's before.sh (with world.ctf on).
gm_audit rows; --undo exact.

    python3 -E -s tools/place_ctf_herald.py --db eqoa.db [--apply | --undo] [--applied FILE]
"""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

NAME, SRC = "PvP Events", "ctf herald 20261110"
BESIDE, TEMPLATE, BODY = "Dabrish", "Captain Maehler", 0x70D898C6
LEVEL, TYPE = 60, 128
ROOM, PORTAL_ROOM, MOOGIE_ROOM = 3.5, 6.0, 10.0
# (back, side): units behind Dabrish along his heading, and to his right (his Armor Dyer stands 4 to his left, the
# Teleporter 8)
SPOTS = [(5, 0), (5, 4), (5, -4), (0, 6), (0, 8), (8, 0), (8, 4), (8, -4), (0, 10)]


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
                "the owner, 2026-11-10: the PvP Events herald of capture the flag, beside Dabrish; tools/place_ctf_herald.py"))


def spot(d, back, side):
    """back behind Dabrish (his front is CLIENT's heading (sin h, cos h), h his facing byte) and side to his right."""
    h = d["facing"] * 2 * math.pi / 256
    fx, fz = math.sin(h), math.cos(h)
    rx, rz = math.cos(h), -math.sin(h)
    return d["x_coord"] - back * fx + side * rx, d["y_coord"], d["z_coord"] - back * fz + side * rz


def free(db, d, x, z):
    """Why the spot is taken, or None."""
    near = db.execute("SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 30 AND abs(z_coord - ?) < 30",
                      (d["world"], x, z)).fetchall()
    for n, nx, nz in near:
        dist = math.hypot(nx - x, nz - z)
        if dist < ROOM:
            return f"{n} stands {dist:.1f} from it"
        if n == "Mr. Moogie" and dist < MOOGIE_ROOM:
            return f"Mr. Moogie stands {dist:.1f} from it"
    h = d["facing"] * 2 * math.pi / 256
    px, pz = d["x_coord"] + 3 * math.sin(h), d["z_coord"] + 3 * math.cos(h)
    if math.hypot(px - x, pz - z) < PORTAL_ROOM:
        return f"it is {math.hypot(px - x, pz - z):.1f} from the Summoning Portal's landing"
    return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--applied", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "place_ctf_herald_applied.json"))
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
        sys.exit("a PvP Events herald stands already: nothing done")
    if db.execute("SELECT 1 FROM npcmove WHERE name = ?", (NAME.lower(),)).fetchone() or \
            db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = ?", (NAME.lower(),)).fetchone():
        sys.exit("the herald has an npcmove or npcaggro row already: nothing done")
    ds = db.execute("SELECT id FROM npcs WHERE npc_name = ? ORDER BY id", (BESIDE,)).fetchall()
    if len(ds) != 1:
        sys.exit(f"{len(ds)} rows named {BESIDE}: nothing done")
    d = row(db, "npcs", "id = ?", (ds[0][0],))
    t = row(db, "npcs", "npc_name = ? ORDER BY id", (TEMPLATE,))
    if t is None or t["modelid"] & 0xFFFFFFFF != BODY:
        sys.exit(f"{TEMPLATE} is {t and t['modelid']}, not the body {BODY:#x}: nothing done")
    chosen = None
    for back, side in SPOTS:
        x, y, z = spot(d, back, side)
        why = free(db, d, x, z)
        print(f"  {back:g} back, {side:+g} to the right: ({x:.2f}, {z:.2f}): {why or 'free'}")
        if why is None:
            chosen = (x, y, z, back, side)
            break
    if chosen is None:
        sys.exit("no spot round Dabrish is free: nothing done")
    x, y, z, back, side = chosen
    w = d["world"]
    cols = [c for c in t if c != "id"]
    nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (t["id"],)).lastrowid
    db.execute("""UPDATE npcs SET npc_name = ?, zone = ?, unk_a = 0, unk_b = 0, world = ?, x_coord = ?, y_coord = ?, z_coord = ?,
                  facing = ?, npc_type = ?, npc_level = ?, npc_id = ?, size = 1.0 WHERE id = ?""",
               (NAME, d["zone"], w, round(x, 3), y, round(z, 3), d["facing"], TYPE, LEVEL, nid, nid))
    made = row(db, "npcs", "id = ?", (nid,))
    audit(db, now, "insert", "npcs", {"id": nid}, None, made)
    db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (the PvP Events herald)', ?)",
               (NAME.lower(), SRC))
    audit(db, now, "insert", "npcmove", {"name": NAME.lower()}, None, {"roam": 0})
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the PvP Events herald: talks only', ?)",
               (NAME, SRC))
    audit(db, now, "insert", "npcaggro", {"npc_name": NAME}, None, {"aggressive": 0})
    print(f"npcs {nid}: {NAME} at world {w} ({x:.3f}, {y}, {z:.3f}) facing {d['facing']}, {back:g} behind and {side:+g} to the right "
          f"of {BESIDE} (npcs {d['id']} at ({d['x_coord']}, {d['z_coord']})), the look of {TEMPLATE} {t['id']}")
    if a.apply:
        db.commit()
        json.dump({"id": nid, "row": json.dumps(made, sort_keys=True)}, open(a.applied, "w"))
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
