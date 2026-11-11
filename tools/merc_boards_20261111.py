#!/usr/bin/env python3
"""The Mercenary Recruitment boards in Highpass's bulletin board with papers on it (the owner, 2026-11-11: "change
mercenary liason NPC to a Bulletin Board Model (like in highpass) and it is named Mercenary Recruitment", then "yes put
the merc board in all starter cities in the general area"):

  - the nine liaisons tools/place_merc_liaisons.py stood (npcs 162678-162686: Freeport, Moradhim, Neriak, Halas,
    Tethelin, the Fayspires, Surefall, Oggok, Qeynos) are given the board's body, sprite 0x260EB42F (638497839, a
    zone actor of 23 Tunaria zones, Highpass's among them, sent to every client in patches/serve/ss-art.csf by
    cmd/artbuild: tools/art/ss-art.json) at size 1.0, the name Mercenary Recruitment, and a board's look (no gear,
    no hair: the Announcements boards' zeros). Where each stands is left as it is (the GM panel moved Freeport's,
    Surefall's and Qeynos's against their wells on 2026-11-11);
  - the starter cities with none (where defaultcharacter starts a race: Highbourne's erudites, Rivervale's halflings,
    Klick'Anon's gnomes, Grobb's trolls; the other cities have one) are given one each, a copy of row 162678 as made
    above, on open ground by the city's bank (the bankers everyone visits; the spot read off the server's ground,
    worlds/ground/0.ground: its floor, no wall in the board's 4 x 4 or between it and the bankers, 5 or more from
    every NPC), facing the bankers;
  - npcmove (stands) and npcaggro (never on sight) rows for the new name, as the old name has.

handlers/merc.go hires at an NPC of either name. The server reads npcs, npcmove and npcaggro at its start: run it in a
keep restart's before.sh. A second --apply does nothing; --undo is exact (refused for a row changed since). gm_audit
rows for every change.

    python3 -E -s tools/merc_boards_20261111.py --db eqoa.db [--apply | --undo] [--applied FILE]
"""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

NAME, OLD, SRC = "Mercenary Recruitment", "Mercenary Liaison", "merc boards 20261111"
MODEL, SIZE = 0x260EB42F, 1.0
ROWS = list(range(162678, 162687))
TEMPLATE = 162678
# a board's look: what the Announcements boards have (a sprite holds no gear and grows no hair)
LOOK = ["primary_weapon", "secondary_weapon", "shield", "torso", "forearm", "gloves", "legs", "feet", "head", "hair_color",
        "hair_length", "hair_style", "face"]
CHANGED = ["npc_name", "modelid", "size"] + LOOK
ROOM, CITY = 5.0, 40.0
# The starter cities without a board: the spot by the bank (x, y, z), and the bankers' middle it faces. Read off
# worlds/ground/0.ground on 2026-11-11 (floor at the spot, no wall in a 4 x 4 square at 0.5, 1.5 and 2.5 over it nor
# between it and the bankers; Highbourne's bank is indoors, so its board stands outside the bank's door).
CITIES = [
    ("Highbourne", (4638.0, 54.16, 21384.0), (4638.0, 21402.0)),
    ("Rivervale", (18730.89, 62.34, 10819.37), (18729.6, 10824.2)),
    ("Klick'Anon", (23654.0, 32.66, 6241.03), (23648.5, 6231.5)),
    ("Grobb", (25435.5, 63.75, 31588.2), (25435.5, 31581.7)),
]
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
                "the owner, 2026-11-11: the Mercenary Recruitment boards in Highpass's papered board, one in every "
                "starter city; tools/merc_boards_20261111.py"))


def facing(x, z, tx, tz):
    """The facing byte toward (tx, tz): CLIENT's heading h has its front at (sin h, cos h)."""
    return int(round(math.atan2(tx - x, tz - z) / (2 * math.pi) * 256)) % 256


def want(r):
    """Whether a liaison row is a board as this tool makes it."""
    return r["npc_name"] == NAME and r["modelid"] == MODEL and r["size"] == SIZE and all(r[c] == 0 for c in LOOK)


def is_board(name):
    return (name or "").strip().lower() in (NAME.lower(), OLD.lower())


def undo(db, now, path):
    if not os.path.exists(path):
        sys.exit("nothing applied: no " + path)
    j = json.load(open(path))
    for u in j["updated"]:
        r = row(db, "npcs", "id = ?", (u["id"],))
        if r is None or json.dumps(r, sort_keys=True) != u["after"]:
            sys.exit(f"npcs {u['id']} changed since it was applied (a GM's edit): nothing done")
    for k in j["inserted"]:
        r = row(db, "npcs", "id = ?", (k["id"],))
        if r is None or json.dumps(r, sort_keys=True) != k["row"]:
            sys.exit(f"npcs {k['id']} changed since it was applied (a GM's edit): nothing done")
    for tbl, key, val in j["side"]:
        r = row(db, tbl, f"{key} = ?", (val["k"],))
        if r is None or json.dumps(r, sort_keys=True) != val["row"]:
            sys.exit(f"{tbl} {val['k']} changed since it was applied: nothing done")
    with db:
        for u in j["updated"]:
            before = json.loads(u["before"])
            r = row(db, "npcs", "id = ?", (u["id"],))
            db.execute(f"UPDATE npcs SET {', '.join(c + ' = ?' for c in CHANGED)} WHERE id = ?",
                       [before[c] for c in CHANGED] + [u["id"]])
            audit(db, now, "update", "npcs", {"id": u["id"]}, r, row(db, "npcs", "id = ?", (u["id"],)))
        for k in j["inserted"]:
            audit(db, now, "delete", "npcs", {"id": k["id"]}, row(db, "npcs", "id = ?", (k["id"],)), None)
            db.execute("DELETE FROM npcs WHERE id = ?", (k["id"],))
        for tbl, key, val in j["side"]:
            audit(db, now, "delete", tbl, {key: val["k"]}, row(db, tbl, f"{key} = ?", (val["k"],)), None)
            db.execute(f"DELETE FROM {tbl} WHERE {key} = ?", (val["k"],))
        db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (j["seq"],))
    os.rename(path, path + ".undone-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()))
    print(f"undone: {len(j['updated'])} boards back to their rows, npcs {', '.join(str(k['id']) for k in j['inserted']) or 'none'} "
          f"removed, {len(j['side'])} npcmove/npcaggro rows removed")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--applied", default=os.path.join(HERE, "merc_boards_20261111_applied.json"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    if a.undo:
        undo(db, now, a.applied)
        return

    rows = {i: row(db, "npcs", "id = ?", (i,)) for i in ROWS}
    for i, r in rows.items():
        if r is None or not is_board(r["npc_name"]):
            sys.exit(f"npcs {i} is {r and r['npc_name']!r}, not a Mercenary Liaison: nothing done")
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()[0]
    rec = {"updated": [], "inserted": [], "side": [], "seq": seq}
    for i in ROWS:
        r = rows[i]
        if want(r):
            print(f"npcs {i}: a board already at ({r['x_coord']:.2f}, {r['y_coord']:.2f}, {r['z_coord']:.2f}) facing {r['facing']}")
            continue
        db.execute(f"UPDATE npcs SET npc_name = ?, modelid = ?, size = ?, {', '.join(c + ' = 0' for c in LOOK)} WHERE id = ?",
                   (NAME, MODEL, SIZE, i))
        after = row(db, "npcs", "id = ?", (i,))
        audit(db, now, "update", "npcs", {"id": i}, r, after)
        rec["updated"].append({"id": i, "before": json.dumps(r, sort_keys=True), "after": json.dumps(after, sort_keys=True)})
        print(f"npcs {i}: {r['npc_name']} -> {NAME}, body {r['modelid']:#010x} -> {MODEL:#010x} at {SIZE}, "
              f"stays at world {r['world']} ({r['x_coord']:.2f}, {r['y_coord']:.2f}, {r['z_coord']:.2f}) facing {r['facing']}")

    t = row(db, "npcs", "id = ?", (TEMPLATE,))
    cols = [c for c in t if c != "id"]
    for city, (x, y, z), (tx, tz) in CITIES:
        near = db.execute("SELECT id, npc_name, x_coord, z_coord FROM npcs WHERE world = 0 AND abs(x_coord - ?) < ? AND abs(z_coord - ?) < ?",
                          (x, CITY, z, CITY)).fetchall()
        have = [n for n in near if is_board(n[1]) and math.hypot(n[2] - x, n[3] - z) < CITY]
        if have:
            print(f"{city}: npcs {have[0][0]} ({have[0][1]}) stands {math.hypot(have[0][2] - x, have[0][3] - z):.1f} from the spot already")
            continue
        for n in near:
            if math.hypot(n[2] - x, n[3] - z) < ROOM:
                sys.exit(f"{city}: {n[1]} (npcs {n[0]}) stands {math.hypot(n[2] - x, n[3] - z):.1f} from the spot: nothing done")
        f = facing(x, z, tx, tz)
        nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (TEMPLATE,)).lastrowid
        db.execute("""UPDATE npcs SET zone = 0, unk_a = 0, unk_b = 0, world = 0, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?,
                      npc_id = ? WHERE id = ?""", (x, y, z, f, nid, nid))
        made = row(db, "npcs", "id = ?", (nid,))
        audit(db, now, "insert", "npcs", {"id": nid}, None, made)
        rec["inserted"].append({"id": nid, "city": city, "row": json.dumps(made, sort_keys=True)})
        print(f"npcs {nid}: {NAME} in {city} at world 0 ({x}, {y}, {z}) facing {f} (toward the bankers), a copy of {TEMPLATE}")

    for tbl, key, k, sql, args in [
            ("npcmove", "name", NAME.lower(), "INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', ?, ?)",
             (NAME.lower(), "stands (the Mercenary Recruitment boards)", SRC)),
            ("npcaggro", "npc_name", NAME, "INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, ?, ?)",
             (NAME, "the Mercenary Recruitment boards: talk only", SRC))]:
        if row(db, tbl, f"{key} = ? COLLATE NOCASE", (k,)) is not None:
            continue
        db.execute(sql, args)
        made = row(db, tbl, f"{key} = ?", (k,))
        audit(db, now, "insert", tbl, {key: k}, None, made)
        rec["side"].append([tbl, key, {"k": k, "row": json.dumps(made, sort_keys=True)}])
        print(f"{tbl}: {k} {'stands' if tbl == 'npcmove' else 'never attacks on sight'}")

    if not (rec["updated"] or rec["inserted"] or rec["side"]):
        db.rollback()
        print("nothing to do: every board is as this tool makes it")
        return
    if a.apply:
        if os.path.exists(a.applied):
            db.rollback()
            sys.exit(f"{a.applied} records an earlier apply and the boards are not as it left them: --undo first, nothing done")
        db.commit()
        json.dump(rec, open(a.applied, "w"), indent=1)
        print(f"applied (record: {a.applied})")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
