#!/usr/bin/env python3
"""Stand the Bug Finder Leaderboard against Freeport's well (handlers/bugboard.go; the owner, 2026-11-15: "lets add a
bulletin board in freeport by the well like the others with Bug Finder Leaderboard. the same list from the website
with out a GM and I.").

One npcs row named "Bug Finder Leaderboard": Highpass's papered bulletin board (modelid 638497839, 0x260EB42F, size 1,
the boards' look: no gear, robe_style 255), a board talked to and never attacked (npc_type 384, level 1, as the Real
Estate Listings), in Freeport (world 0) on the well's SOUTH-EAST side, (25301.084, 54.13, 15754.484) facing 32 (away
from the well). The well's centre is (25296.7, 15750.1), the centre of its four boards on the Heirloom server: Real
Estate Listings west, PvP Events east, Heroic Dungeons south, each 6.2 off it, and the Mercenary Recruitment board on
the north side; so the leaderboard stands 6.2 off it too, between Heroic Dungeons and PvP Events (4.7 from each; the
nearest NPC is the Testing Heroes Vendor, 4.1 off). The other diagonals are taken: north-west 2.7 from the Mercenary
Recruitment board, north-east 2.9 from the Live server's Feastmaster Holloway, south-west under the GM triggers'
Metal Moogie (sizes 7.5 and 9). On the Live server, which has none of the other boards, the same spot (its nearest
NPC is the Freeport Announcements board, 7.6 off). Its name plate is the server's (entity/boardplate.go: a papered
board shows its name); its art reaches every client in the patch's ss-art.csf.

Plus, as the other boards: npcmove (stands) and npcaggro (talks only) rows for the name.

A report by default (--dry is the same); --apply writes it in one transaction with a gm_audit line a change and keeps
what it did in table bugboard_1115_log; --undo takes back each change still as this tool left it. Idempotent: a
database with the board as this tool stands it is left alone. Refused, nothing written, where another row is named
so, an NPC row of world 0 stands within ROOM of the spot, or npcmove/npcaggro has a row of the name.

The server reads npcs at its start: a RESTART (a keep restart: before.sh), with the server with bugboard's code.

    python3 -E -s tools/bugboard_20261115.py --db eqoa.db [--dry | --apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

NAME, SRC = "Bug Finder Leaderboard", "bugboard 20261115"
LOG = "bugboard_1115_log"
DETAIL = "the Bug Finder Leaderboard against Freeport's well (the owner, 2026-11-15): tools/bugboard_20261115.py"
WHO, VIA = "dab", "tool"
WELL_X, WELL_Z, OFF = 25296.7, 15750.1, 6.2
FACING = 32  # CLIENT's heading byte: 0 is +z, 64 +x; 32 is south-east, away from the well
X = round(WELL_X + OFF * math.sin(FACING * 2 * math.pi / 256), 3)
Z = round(WELL_Z + OFF * math.cos(FACING * 2 * math.pi / 256), 3)
Y = 54.13
ROOM = 3.0
BOARD = {  # the Real Estate Listings' row (Heirloom npcs 164531), its place and name aside
    "npc_name": NAME, "zone": 0, "unk_a": 0, "unk_b": 0, "x_coord": X, "y_coord": Y, "z_coord": Z, "facing": FACING,
    "world": 0, "dt_kill": None, "hp": 100, "hp_percent": 255, "modelid": 638497839, "size": 1.0, "unk_d": 0,
    "unk_f": 0, "unk_g": 0, "primary_weapon": 0, "secondary_weapon": 0, "shield": 0, "unk_h": 0, "torso": 0,
    "forearm": 0, "gloves": 0, "legs": 0, "feet": 0, "head": 0, "hair_color": 0, "hair_length": 0, "hair_style": 0,
    "face": 0, "robe_style": "255", "race": "0", "unk_j": 0, "npc_level": 1, "npc_type": 384, "unk_k": 0, "unk_i": "",
    "pd_icon": 0, "unk_m": 0, "unk_n": 0, "unk_o": 0,
}
MOVE = {"name": NAME.lower(), "roam": 0, "radius": 0.0, "water": "", "note": f"stands (the {NAME} board)", "source": SRC}
AGGRO = {"npc_name": NAME, "aggressive": 0, "radius": 0.0, "note": f"the {NAME} board: talks only", "source": SRC}
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


def standing(db):
    """The board as this tool stands it, or None."""
    rows = db.execute("SELECT id FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (NAME,)).fetchall()
    if len(rows) != 1:
        return None
    r = row(db, "npcs", rows[0][0])
    want = dict(BOARD, npc_id=r["id"])
    if any(r[k] != v for k, v in want.items()):
        return None
    m, a = row(db, "npcmove", NAME.lower()), row(db, "npcaggro", NAME)
    if m is None or a is None or m["roam"] != 0 or a["aggressive"] != 0:
        return None
    return r


def plan(db):
    """(lines, refusals, done)."""
    for t in ("npcs", "npcmove", "npcaggro", "gm_audit"):
        if not table(db, t):
            return [], [f"no {t} table here"], False
    if (r := standing(db)) is not None:
        return [f"npcs {r['id']}: {NAME} stands at ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) facing {r['facing']} already"], [], True
    refusals = []
    n = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (NAME,)).fetchone()[0]
    if n:
        refusals.append(f"{n} npcs rows named {NAME} already (not as this tool stands it): look first")
    for t, k in (("npcmove", NAME.lower()), ("npcaggro", NAME)):
        if db.execute(f"SELECT 1 FROM {t} WHERE lower({KEYS[t]}) = lower(?)", (k,)).fetchone():
            refusals.append(f"{t} has a row for {NAME} already")
    near = db.execute("""SELECT id, npc_name, x_coord, z_coord FROM npcs WHERE world = 0
                         AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?""",
                      (X - 15, X + 15, Z - 15, Z + 15)).fetchall()
    near = sorted(((math.hypot(x - X, z - Z), i, nm) for i, nm, x, z in near if x is not None and z is not None))
    for d, i, nm in near:
        if d < ROOM and nm != NAME:
            refusals.append(f"npcs {i} {nm!r} stands {d:.1f} from the spot")
    lines = [f"npcs (new): {NAME!r} in world 0 at ({X}, {Y}, {Z}) facing {FACING}: the papered board "
             f"(modelid {BOARD['modelid']}, size {BOARD['size']}), npc_type {BOARD['npc_type']}, level {BOARD['npc_level']}",
             "  nearest: " + ", ".join(f"{nm} {d:.1f}" for d, _, nm in near[:5]),
             f"npcmove {NAME.lower()!r}: roam 0 (stands)",
             f"npcaggro {NAME!r}: aggressive 0 (talks only)"]
    return lines, refusals, False


def log(db, t, key, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES (?, ?, ?, ?)", (t, json.dumps(key), json.dumps(after), a))


def apply(db):
    db.execute(LOG_SCHEMA)
    cols = list(BOARD)
    nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [BOARD[c] for c in cols]).lastrowid
    db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
    after = row(db, "npcs", nid)
    log(db, "npcs", nid, after, audit(db, "insert", "npcs", nid, None, after))
    for t, vals in (("npcmove", MOVE), ("npcaggro", AGGRO)):
        db.execute(f"INSERT INTO {t} ({', '.join(vals)}) VALUES ({', '.join('?' * len(vals))})", list(vals.values()))
        key = vals[KEYS[t]]
        after = row(db, t, key)
        log(db, t, key, after, audit(db, "insert", t, key, None, after))
    return nid


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
        nid = apply(db)
        db.execute("COMMIT")
        print(f"applied: npcs {nid}")
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    main()
