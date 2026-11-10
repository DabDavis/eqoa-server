#!/usr/bin/env python3
"""Stand a Mercenary Liaison (handlers/merc.go; the owner, 2026-11-10: "lets add mercenary pets similar to EQ 1", hired
from "a Mercenary Liaison NPC, free", "one beside each city's announcements board and one beside Dabrish") beside every
city's announcements board: a row of npcs named Mercenary Liaison, talked to and never attacked (npc_type 128), level 60,
in a mercenary's look (a copy of the first row named "mercenary" in a human body with a weapon and a shield: plate, a
helm, a sword and a shield), facing the way its board faces. A board is a sign of signtext kind 'board', or with no row
one named "... Announcements" (handlers/signs.go's IsBoard). Freeport's stands on Dabrish's line instead when Dabrish
stands within 12 of its board (he does, beside it), facing as he does: the one beside Dabrish and Freeport's board are
the same NPC. Each stands at the first spot of a few beside its board (or Dabrish) that is 3.5 or more from every other
NPC's row, 6 from the Summoning Portal's landing (3 in front of Dabrish) and 10 from Mr. Moogie; a board with no free
spot, or a liaison within 15 already, gets none. npcmove: stands; npcaggro: never on sight (one row each, by the name).
The server reads npcs at its start: run it in a keep restart's before.sh (and world.mercs on). gm_audit rows; --undo
exact (refused for a row a GM changed since).

    python3 -E -s tools/place_merc_liaisons.py --db eqoa.db [--apply | --undo] [--applied FILE]
"""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

NAME, SRC = "Mercenary Liaison", "merc liaisons 20261110"
TEMPLATE, BODY = "mercenary", 1893243078
LEVEL, TYPE = 60, 128
ROOM, PORTAL_ROOM, MOOGIE_ROOM, NEAR_BOARD, DABRISH_NEAR = 3.5, 6.0, 10.0, 15.0, 12.0
# the spots tried, in order: (side, front) in units of the anchor's own side and front
SPOTS = [(4, 0), (-4, 0), (6, 0), (-6, 0), (8, 0), (-8, 0), (0, 4), (4, 4), (-4, 4), (10, 0), (-10, 0), (-12, 0), (12, 0),
         (0, -4), (14.5, 0), (-14.5, 0)]


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
                "the owner, 2026-11-10: a Mercenary Liaison beside each city's board and Dabrish; tools/place_merc_liaisons.py"))


def spot(a, side, front):
    """side and front along the anchor's facing byte h: CLIENT's heading (sin h, cos h) its front, (-cos h, sin h) its side."""
    h = a["facing"] * 2 * math.pi / 256
    return (a["x_coord"] - side * math.cos(h) + front * math.sin(h), a["y_coord"],
            a["z_coord"] + side * math.sin(h) + front * math.cos(h))


def boards(db):
    """Every board: a sign of signtext kind 'board', or with no row one named "... Announcements"."""
    out = db.execute("""SELECT n.id FROM npcs n JOIN signtext s ON s.npcid = n.id WHERE s.kind = 'board'
                        UNION SELECT n.id FROM npcs n WHERE n.npc_name LIKE '% Announcements'
                          AND NOT EXISTS (SELECT 1 FROM signtext s WHERE s.npcid = n.id)
                        ORDER BY 1""").fetchall()
    return [row(db, "npcs", "id = ?", (i,)) for (i,) in out]


def free(db, w, x, z, dabrish):
    """Why the spot is no liaison's, "" when it is free."""
    near = db.execute("""SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 20 AND abs(z_coord - ?) < 20""",
                      (w, x, z)).fetchall()
    for n, nx, nz in near:
        d = math.hypot(nx - x, nz - z)
        if d < ROOM:
            return f"{n} stands {d:.1f} from it"
        if n == "Mr. Moogie" and d < MOOGIE_ROOM:
            return f"Mr. Moogie stands {d:.1f} from it"
    if dabrish is not None and dabrish["world"] == w:
        px, _, pz = spot(dabrish, 0, 3)
        if math.hypot(px - x, pz - z) < PORTAL_ROOM:
            return "the Summoning Portal's landing is too near"
    return ""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--applied", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "place_merc_liaisons_applied.json"))
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
        for e in j["npcs"]:
            r = row(db, "npcs", "id = ?", (e["id"],))
            if r is None or r["npc_name"] != NAME:
                sys.exit(f"npcs {e['id']} is {r and r['npc_name']}: nothing done")
            if json.dumps(r, sort_keys=True) != e["row"]:
                sys.exit(f"npcs {e['id']} changed since (a GM's edit): nothing done; undo it in the GM panel")
        with db:
            for e in j["npcs"]:
                r = row(db, "npcs", "id = ?", (e["id"],))
                audit(db, now, "delete", "npcs", {"id": e["id"]}, r, None)
                db.execute("DELETE FROM npcs WHERE id = ?", (e["id"],))
            for tbl, where, args in [("npcmove", "name = ? AND source = ?", (NAME.lower(), SRC)),
                                     ("npcaggro", "npc_name = ? AND source = ?", (NAME, SRC))]:
                if not j.get(tbl):
                    continue
                cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
                cols = [d[0] for d in cur.description]
                for x in cur.fetchall():
                    audit(db, now, "delete", tbl, {"where": where, "args": list(args)}, dict(zip(cols, x)), None)
                db.execute(f"DELETE FROM {tbl} WHERE {where}", args)
            db.execute("UPDATE sqlite_sequence SET seq = (SELECT COALESCE(MAX(id), 0) FROM npcs) WHERE name = 'npcs'")
        os.rename(a.applied, a.applied + ".undone-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()))
        print(f"undone: {len(j['npcs'])} liaisons, their npcmove and npcaggro rows")
        return

    if a.apply and os.path.exists(a.applied):
        sys.exit("applied already: --undo first")
    t = row(db, "npcs", "npc_name = ? AND modelid = ? AND primary_weapon != 0 AND shield != 0 ORDER BY id", (TEMPLATE, BODY))
    if t is None:
        sys.exit(f"no {TEMPLATE!r} row in the human body with a weapon and a shield: nothing done")
    ds = db.execute("SELECT id FROM npcs WHERE npc_name = 'Dabrish' ORDER BY id").fetchall()
    dabrish = row(db, "npcs", "id = ?", (ds[0][0],)) if len(ds) == 1 else None
    liaisons = db.execute("SELECT world, x_coord, z_coord FROM npcs WHERE npc_name = ?", (NAME,)).fetchall()
    made, plan = [], []
    cols = [c for c in t if c != "id"]
    for b in boards(db):
        if any(w == b["world"] and math.hypot(x - b["x_coord"], z - b["z_coord"]) < NEAR_BOARD for w, x, z in liaisons):
            print(f"  {b['npc_name']}: a liaison stands within {NEAR_BOARD:g} already")
            continue
        anchor = b
        if dabrish is not None and dabrish["world"] == b["world"] and \
                math.hypot(dabrish["x_coord"] - b["x_coord"], dabrish["z_coord"] - b["z_coord"]) < DABRISH_NEAR:
            anchor = dabrish  # Freeport's: on Dabrish's line, as the Teleporter (place_teleporter.py)
        at, why = None, []
        for side, front in SPOTS:
            x, y, z = spot(anchor, side, front)
            w = free(db, b["world"], x, z, dabrish)
            if w == "":
                at = (x, y, z)
                break
            why.append(f"({side:g}, {front:g}): {w}")
        if at is None:
            print(f"  {b['npc_name']}: no free spot ({'; '.join(why)})")
            continue
        x, y, z = at
        nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (t["id"],)).lastrowid
        db.execute("""UPDATE npcs SET npc_name = ?, zone = ?, unk_a = 0, unk_b = 0, world = ?, x_coord = ?, y_coord = ?, z_coord = ?,
                      facing = ?, npc_type = ?, npc_level = ?, npc_id = ?, size = 1.0 WHERE id = ?""",
                   (NAME, anchor["zone"], b["world"], round(x, 3), y, round(z, 3), anchor["facing"], TYPE, LEVEL, nid, nid))
        r = row(db, "npcs", "id = ?", (nid,))
        audit(db, now, "insert", "npcs", {"id": nid}, None, r)
        made.append({"id": nid, "row": json.dumps(r, sort_keys=True)})
        liaisons.append((b["world"], x, z))
        by = "Dabrish" if anchor is dabrish else b["npc_name"]
        plan.append(f"  npcs {nid}: beside {by} (world {b['world']} ({x:.2f}, {y:.2f}, {z:.2f}) facing {anchor['facing']}), "
                    f"for {b['npc_name']}")
    extra = {}
    if made and not db.execute("SELECT 1 FROM npcmove WHERE name = ?", (NAME.lower(),)).fetchone():
        db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (the Mercenary Liaisons)', ?)",
                   (NAME.lower(), SRC))
        audit(db, now, "insert", "npcmove", {"name": NAME.lower()}, None, {"roam": 0})
        extra["npcmove"] = True
    if made and not db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = ?", (NAME.lower(),)).fetchone():
        db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Mercenary Liaisons: talk only', ?)",
                   (NAME, SRC))
        audit(db, now, "insert", "npcaggro", {"npc_name": NAME}, None, {"aggressive": 0})
        extra["npcaggro"] = True
    print(f"{len(made)} Mercenary Liaisons in the look of {TEMPLATE} {t['id']}:")
    print("\n".join(plan))
    if a.apply and made:
        db.commit()
        json.dump({"npcs": made, **extra}, open(a.applied, "w"))
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)" if not a.apply else "nothing to apply")


if __name__ == "__main__":
    main()
