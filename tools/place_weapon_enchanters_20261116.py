#!/usr/bin/env python3
"""Stand a Weapon Enchanter (handlers/enchanter.go; the owner, 2026-11-16: "lets add an weapon enchanter next to
equipment dyer. will stack up to 2 auras on either weapon, tags it with the aura tags") beside every Equipment Dyer
(and any still named Armor Dyer): a row of npcs named "Weapon Enchanter", talked to and never attacked (npc_type 128,
as the dyers), level 50, in an enchanter's look (a copy of Waysender Ilganis's row: the Erudite body 839650119 in a
robe, a staff in hand, TEMPLATE), at the dyer's height, facing the way it does, SIDE units to one of its sides (its
left, else its right, else behind it; then FAR off the same way): the first spot no other NPC's row stands within ROOM of. A dyer with an
enchanter within NEAR already is passed (a second run adds nothing); one with no free spot is listed.

Report by default; --apply in one transaction, a gm_audit insert line per row and place_enchanters_1116_log;
--undo deletes exactly the rows it made (one changed since left and listed). The server reads npcs at its start: run
it in a keep restart's before.sh, on each shard's database.

    python3 -E -s tools/place_weapon_enchanters_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

NAME = "Weapon Enchanter"
DYERS = ("equipment dyer", "armor dyer", "armour dyer")  # handlers.IsArmorDyer
TEMPLATE, BODY = "Waysender Ilganis", 839650119
LEVEL, TYPE = 50, 128
SIDE, FAR, ROOM, NEAR = 2.5, 4.0, 1.8, 6.0
LOG = "place_enchanters_1116_log"
DETAIL = "the owner, 2026-11-16: a Weapon Enchanter beside each Equipment Dyer; tools/place_weapon_enchanters_20261116.py"


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = 'dab'").fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                      "VALUES (?, ?, 'dab', 'tool', '', ?, 'npcs', ?, ?, ?, ?)",
                      (at, acct[0] if acct else 0, action, go_json(key), go_json(before), go_json(after), DETAIL)).lastrowid


def spots(d):
    """The dyer's left, right and back, SIDE off (then FAR off): its facing byte h, CLIENT's heading (sin h, cos h) its
    front."""
    h = d["facing"] * 2 * math.pi / 256
    x, z = d["x_coord"], d["z_coord"]
    out = []
    for s in (SIDE, FAR):
        out += [(x - s * math.cos(h), z + s * math.sin(h)), (x + s * math.cos(h), z - s * math.sin(h)),
                (x - s * math.sin(h), z - s * math.cos(h))]
    return out


def plan(db):
    """(made: [(dyer, x, z)], lines, refusals)."""
    refusals, lines, made = [], [], []
    for t in ("npcs", "gm_audit"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return made, lines, refusals
    tpl = rows(db, "SELECT * FROM npcs WHERE npc_name = ? ORDER BY id LIMIT 1", (TEMPLATE,))
    if not tpl or tpl[0]["modelid"] != BODY:
        refusals.append(f"{TEMPLATE} is {tpl and tpl[0]['modelid']}, not the body {BODY}")
        return made, lines, refusals
    dyers = rows(db, "SELECT * FROM npcs WHERE lower(trim(npc_name)) IN (?, ?, ?) ORDER BY id", DYERS)
    lines.append(f"{len(dyers)} dyers")
    placed = []  # this run's spots, by world
    for d in dyers:
        w = d["world"]
        near = db.execute("SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 20 AND abs(z_coord - ?) < 20",
                          (w, d["x_coord"], d["z_coord"])).fetchall()
        near += [(NAME, x, z) for pw, x, z in placed if pw == w]
        if any(n == NAME and math.hypot(x - d["x_coord"], z - d["z_coord"]) < NEAR for n, x, z in near):
            lines.append(f"  pass  dyer {d['id']} (world {w}, {d['x_coord']:.0f}, {d['z_coord']:.0f}): an enchanter stands by it")
            continue
        for x, z in spots(d):
            if all(math.hypot(nx - x, nz - z) >= ROOM for _, nx, nz in near):
                made.append((d, x, z))
                placed.append((w, x, z))
                break
        else:
            lines.append(f"  NONE  dyer {d['id']} (world {w}, zone {d['zone']}, {d['x_coord']:.0f}, {d['z_coord']:.0f}): no free spot")
    lines.append(f"{len(made)} enchanters to stand")
    return made, lines, refusals


def apply(db, made):
    db.execute(f"CREATE TABLE {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, npcid INTEGER NOT NULL, row TEXT NOT NULL, audit INTEGER NOT NULL)")
    tpl = rows(db, "SELECT * FROM npcs WHERE npc_name = ? ORDER BY id LIMIT 1", (TEMPLATE,))[0]
    cols = [c for c in tpl if c != "id"]
    for d, x, z in made:
        nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (tpl["id"],)).lastrowid
        db.execute("""UPDATE npcs SET npc_name = ?, zone = ?, world = ?, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?,
                      npc_type = ?, npc_level = ?, npc_id = ?, size = 1.0 WHERE id = ?""",
                   (NAME, d["zone"], d["world"], round(x, 3), d["y_coord"], round(z, 3), d["facing"], TYPE, LEVEL, nid, nid))
        r = rows(db, "SELECT * FROM npcs WHERE id = ?", (nid,))[0]
        a = audit(db, "insert", {"id": nid}, None, r)
        db.execute(f"INSERT INTO {LOG} (npcid, row, audit) VALUES (?, ?, ?)", (nid, go_json(r), a))


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    for nid, row in db.execute(f"SELECT npcid, row FROM {LOG} ORDER BY seq DESC").fetchall():
        now = rows(db, "SELECT * FROM npcs WHERE id = ?", (nid,))
        if not now or go_json(now[0]) != row:
            left.append(f"npcs {nid}: changed or gone since, left as it is")
            continue
        db.execute("DELETE FROM npcs WHERE id = ?", (nid,))
        audit(db, "delete", {"id": nid}, now[0], None)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} enchanters taken out" + "".join(f"\n  left: {w}" for w in left) +
                  ("; restart (keep) for the world to lose them" if back else ""))
            return 0 if not left else 1
        made, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not made:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if table(db, LOG):
            db.execute("ROLLBACK")
            print(f"applied before ({LOG} is there): --undo first")
            return 1
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply writes them)")
            return 0
        apply(db, made)
        db.execute("COMMIT")
        print(f"applied: {len(made)} enchanters, in gm_audit and {LOG}; restart (keep) for the world to read them")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
