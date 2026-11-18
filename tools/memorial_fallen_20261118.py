#!/usr/bin/env python3
"""Stand the Memorial of the Fallen and the Hardcore Leaderboard in Freeport (handlers/memorial.go; the owner,
2026-11-18: "add a memorial to deceased hardcore players in freeport with timestamps and names", "next to deceased
memorial for hardcore players also have Living Hardcore Leaderboard").

Two npcs rows, talked to and never attacked (npc_type 384, level 1, as the boards), in Freeport (world 0) on the plaza
between the well (25296.7, 15750.1) and the east sign post (25335.3, 15699.7), on open ground 15 off the nearest NPC
(Aeric Sparelli) on both shards (read off both shards' databases of 2026-11-15):

  "Memorial of the Fallen": the disc's headstone with the skull (modelid 1843770060, 0x6DE5B2CC: a CSprite of
  CHAR.ESF, the Guild Board's and the Duel Tournament Board's body, seen in a render), at size 1.6 (2.8 tall), at
  (25318.0, 54.13, 15722.0) facing 230 (toward the well). Its name plate is the server's (entity/boardplate.go
  names it a board). A talk lists the shard's fallen Hardcore characters, the newest first, a page a talk.
  "Hardcore Leaderboard": Highpass's papered bulletin board (modelid 638497839, 0x260EB42F, size 1, the boards'
  look), 4.7 west of it at (25314.0, 54.13, 15719.5) facing 230. A talk ranks the shard's living Hardcore characters.

Plus, as the boards: npcmove (stands) and npcaggro (talks only) rows for each name.

A report by default (--dry is the same); --apply writes it in one transaction with a gm_audit line a change and keeps
what it did in table memorial_1118_log; --undo takes back each change still as this tool left it. Idempotent: a
database with both as this tool stands them is left alone. Refused, nothing written, where another row has either
name, an NPC row of world 0 stands within ROOM of either spot, or npcmove/npcaggro has a row of either name.

The server reads npcs at its start: a RESTART (a keep restart: before.sh), with the server with memorial's code.

    python3 -E -s tools/memorial_fallen_20261118.py --db eqoa.db [--dry | --apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

SRC = "memorial 20261118"
LOG = "memorial_1118_log"
DETAIL = "the Memorial of the Fallen and the Hardcore Leaderboard in Freeport (the owner, 2026-11-18): tools/memorial_fallen_20261118.py"
WHO, VIA = "dab", "tool"
Y = 54.13
FACING = 230  # CLIENT's heading byte: 0 is +z, 64 +x; 230 faces the well (north-west)
ROOM = 3.0


def npc_row(name, x, z, model, size):
    """The Real Estate Listings' row (Heirloom npcs 164531), its place, name, body and size aside."""
    return {
        "npc_name": name, "zone": 0, "unk_a": 0, "unk_b": 0, "x_coord": x, "y_coord": Y, "z_coord": z, "facing": FACING,
        "world": 0, "dt_kill": None, "hp": 100, "hp_percent": 255, "modelid": model, "size": size, "unk_d": 0,
        "unk_f": 0, "unk_g": 0, "primary_weapon": 0, "secondary_weapon": 0, "shield": 0, "unk_h": 0, "torso": 0,
        "forearm": 0, "gloves": 0, "legs": 0, "feet": 0, "head": 0, "hair_color": 0, "hair_length": 0, "hair_style": 0,
        "face": 0, "robe_style": "255", "race": "0", "unk_j": 0, "npc_level": 1, "npc_type": 384, "unk_k": 0, "unk_i": "",
        "pd_icon": 0, "unk_m": 0, "unk_n": 0, "unk_o": 0,
    }


PIECES = [
    npc_row("Memorial of the Fallen", 25318.0, 15722.0, 1843770060, 1.6),
    npc_row("Hardcore Leaderboard", 25314.0, 15719.5, 638497839, 1.0),
]
KEYS = {"npcs": "id", "npcmove": "name", "npcaggro": "npc_name"}
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,
    after  TEXT NOT NULL,      -- the row as this tool left it (JSON); every change is an insert
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def row(db, t, key):
    cur = db.execute(f"SELECT * FROM {t} WHERE {KEYS[t]} = ?", (key,))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, action, t, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, t, go_json({KEYS[t]: key}),
                      go_json(before) if before is not None else "", go_json(after) if after is not None else "", DETAIL))
    return cur.lastrowid


def move_row(name):
    return {"name": name.lower(), "roam": 0, "radius": 0.0, "water": "", "note": f"stands (the {name})", "source": SRC}


def aggro_row(name):
    return {"npc_name": name, "aggressive": 0, "radius": 0.0, "note": f"the {name}: talks only", "source": SRC}


def standing_one(db, piece):
    """The piece as this tool stands it, or None."""
    name = piece["npc_name"]
    rows = db.execute("SELECT id FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (name,)).fetchall()
    if len(rows) != 1:
        return None
    r = row(db, "npcs", rows[0][0])
    want = dict(piece, npc_id=r["id"])
    if any(r[k] != v for k, v in want.items()):
        return None
    m, a = row(db, "npcmove", name.lower()), row(db, "npcaggro", name)
    if m is None or a is None or m["roam"] != 0 or a["aggressive"] != 0:
        return None
    return r


def plan(db):
    """(lines, refusals, done)."""
    for t in ("npcs", "npcmove", "npcaggro", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    stood = [standing_one(db, p) for p in PIECES]
    if all(r is not None for r in stood):
        return [f"npcs {r['id']}: {r['npc_name']} stands at ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) already" for r in stood], [], True
    refusals, lines = [], []
    names = {p["npc_name"] for p in PIECES}
    for p in PIECES:
        name, X, Z = p["npc_name"], p["x_coord"], p["z_coord"]
        n = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (name,)).fetchone()[0]
        if n:
            refusals.append(f"{n} npcs rows named {name} already (not as this tool stands them): look first, or --undo")
        for t, k in (("npcmove", name.lower()), ("npcaggro", name)):
            if db.execute(f"SELECT 1 FROM {t} WHERE lower({KEYS[t]}) = lower(?)", (k,)).fetchone():
                refusals.append(f"{t} has a row for {name} already")
        near = db.execute("""SELECT id, npc_name, x_coord, z_coord FROM npcs WHERE world = 0
                             AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?""",
                          (X - 15, X + 15, Z - 15, Z + 15)).fetchall()
        near = sorted(((math.hypot(x - X, z - Z), i, nm) for i, nm, x, z in near if x is not None and z is not None))
        for d, i, nm in near:
            if d < ROOM and nm not in names:
                refusals.append(f"npcs {i} {nm!r} stands {d:.1f} from {name}'s spot")
        lines += [f"npcs (new): {name!r} in world 0 at ({X}, {Y}, {Z}) facing {FACING}: modelid {p['modelid']}, size {p['size']}, "
                  f"npc_type {p['npc_type']}, level {p['npc_level']}",
                  "  nearest: " + ", ".join(f"{nm} {d:.1f}" for d, _, nm in near[:5]),
                  f"npcmove {name.lower()!r}: roam 0 (stands)", f"npcaggro {name!r}: aggressive 0 (talks only)"]
    return lines, refusals, False


def log(db, t, key, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES (?, ?, ?, ?)", (t, json.dumps(key), json.dumps(after), a))


def apply(db):
    db.execute(LOG_SCHEMA)
    ids = []
    for piece in PIECES:
        cols = list(piece)
        nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [piece[c] for c in cols]).lastrowid
        db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
        after = row(db, "npcs", nid)
        log(db, "npcs", nid, after, audit(db, "insert", "npcs", nid, None, after))
        for t, vals in (("npcmove", move_row(piece["npc_name"])), ("npcaggro", aggro_row(piece["npc_name"]))):
            db.execute(f"INSERT INTO {t} ({', '.join(vals)}) VALUES ({', '.join('?' * len(vals))})", list(vals.values()))
            key = vals[KEYS[t]]
            after = row(db, t, key)
            log(db, t, key, after, audit(db, "insert", t, key, None, after))
        ids.append(nid)
    return ids


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, after in db.execute(f"SELECT seq, tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row(db, tbl, key)
        if now is None:
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
            continue
        if json.loads(go_json(now)) != json.loads(go_json(json.loads(after))):
            left += 1
            print(f"left: {tbl} {key!r} changed since")
            continue
        db.execute(f"DELETE FROM {tbl} WHERE {KEYS[tbl]} = ?", (key,))
        audit(db, "delete", tbl, key, now, None)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--dry", action="store_true", help="report only (the default)")
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            print(f"undo: {back} rows taken back, {left} left as changed since")
            db.execute("COMMIT")
            return
        lines, refusals, done = plan(db)
        for l in lines:
            print(l)
        if refusals:
            for r in refusals:
                print("REFUSED:", r)
            db.execute("ROLLBACK")
            sys.exit(1)
        if done:
            print("nothing to do")
            db.execute("ROLLBACK")
            return
        if not a.apply:
            print("(report only: --apply writes it)")
            db.execute("ROLLBACK")
            return
        ids = apply(db)
        db.execute("COMMIT")
        print(f"applied: npcs {ids}")
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
