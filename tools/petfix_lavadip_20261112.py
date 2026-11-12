#!/usr/bin/env python3
"""The evil tank epic's lava dip (petition 1660, Wanuven, 2026-11-12: "the text says to go to where the land dips in
the center of Lavastorm ... the trigger for the quest update was way out east. I think it may be in the wrong spot").

Step 4 of the episode 1 quests (101068 and 101077, a reach step: "Drop the three ores into the lava where the land
dips, in the middle of Lavastorm.") was at (7643, 5144), radius 40, on the hills of the main square's east edge, 140
above the lava: the game names that place "Lavastorm Mountains (E)", and there is no lava within reach. The guides
put it in "Lavastorm (C)" (questmasters id242/id181: "Along the river of lava, the river banks are steep cliffs,
except for ONE area, the banks slope down and meet the river's edge"; EQOA-Data's evil-tank guide: "Lavastorm (C),
close to the edge where the landscape dips slightly downward"). The disc's ground (worlds/ground/3.*) has one such
place by the centre: a bowl north of the middle, between two lava rivers, falling from the plain (55-60) to 33 at the
river's edge (the lava's top is 30.25), its floor at (7200, 4685), in the centre ninth of the game's map cell (x
6667-7333, z 4667-5333, the cell's ninths as UpdateLocation names them: "Lavastorm Mountains (C)"). Every other bank
in the centre is a cliff. The old place was the Megametamilipizon's (a seeded spawn "near animated lava", a guess),
so he moves with it: onto the bowl's slope at (7185, 33.2, 4672), above the lava he climbs out of.

The steps by quest id + step + kind + world + the old place; the creature by name + world + the old place (one
row, any id, as the shards' ids differ). Refused when a row is not as expected. Report only by default; --apply in one
transaction, a gm_audit line a row, log lavadip_1112_log; --undo exact (a row changed since is left and said).
Quests and NPCs are read at the server's start: a restart.

    tools/petfix_lavadip_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "lavadip_1112_log"
DETAIL = ("petition 1660 (Wanuven): the evil tank epic's lava dip moved to Lavastorm's centre bowl, the "
          "Megametamilipizon with it: tools/petfix_lavadip_20261112.py")
WHO, VIA = "claude", "tool"
QUESTS = (101068, 101077)
STEP, KIND, WORLD = 4, "reach", 3
OLD_STEP = (7643.0, 5144.0, 40.0)
NEW_STEP = (7200.0, 4685.0, 45.0)
BOSS = "Megametamilipizon"
OLD_BOSS = (7640.683, 66.2196, 5279.643)
NEW_BOSS = (7185.0, 33.2, 4672.0)
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,      -- queststeps | npcs
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT NOT NULL,      -- the changed columns as they were (JSON)
    after   TEXT NOT NULL,      -- as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), detail))
    return cur.lastrowid


def near(a, b):
    return all(abs(x - y) < 0.01 for x, y in zip(a, b))


def step_now(db, q):
    r = db.execute("SELECT kind, world, x, z, radius FROM queststeps WHERE questid = ? AND step = ?", (q, STEP)).fetchone()
    return None if r is None else {"kind": r[0], "world": r[1], "x": r[2], "z": r[3], "radius": r[4]}


def boss_rows(db):
    return [{"id": r[0], "x_coord": r[1], "y_coord": r[2], "z_coord": r[3]}
            for r in db.execute("SELECT id, x_coord, y_coord, z_coord FROM npcs WHERE npc_name = ? AND world = ?", (BOSS, WORLD))]


def plan(db):
    """(changes [(tbl, key, before, after)], refusals, notes)."""
    changes, refusals, notes = [], [], []
    for t in ("queststeps", "npcs", "gm_audit"):
        if not table(db, t):
            return changes, [f"no {t} table here"], notes
    for q in QUESTS:
        s = step_now(db, q)
        if s is None:
            refusals.append(f"quest {q} has no step {STEP}")
            continue
        if s["kind"] != KIND or s["world"] != WORLD:
            refusals.append(f"quest {q} step {STEP} is {s['kind']} in world {s['world']}, want {KIND} in {WORLD}")
            continue
        here = (s["x"], s["z"], s["radius"])
        if near(here, NEW_STEP):
            notes.append(f"quest {q} step {STEP} is at the bowl already")
        elif near(here, OLD_STEP):
            changes.append(("queststeps", {"questid": q, "step": STEP}, {"x": s["x"], "z": s["z"], "radius": s["radius"]},
                            {"x": NEW_STEP[0], "z": NEW_STEP[1], "radius": NEW_STEP[2]}))
        else:
            refusals.append(f"quest {q} step {STEP} is at {here}, neither the old place {OLD_STEP} nor the bowl {NEW_STEP}")
    rows = boss_rows(db)
    if len(rows) != 1:
        refusals.append(f"{len(rows)} npcs rows named {BOSS} in world {WORLD}, want one")
    else:
        b = rows[0]
        here = (b["x_coord"], b["y_coord"], b["z_coord"])
        if near(here, NEW_BOSS):
            notes.append(f"{BOSS} ({b['id']}) stands at the bowl already")
        elif near(here, OLD_BOSS):
            changes.append(("npcs", {"id": b["id"]}, {"x_coord": here[0], "y_coord": here[1], "z_coord": here[2]},
                            {"x_coord": NEW_BOSS[0], "y_coord": NEW_BOSS[1], "z_coord": NEW_BOSS[2]}))
        else:
            refusals.append(f"{BOSS} ({b['id']}) is at {here}, neither the old place {OLD_BOSS} nor the bowl {NEW_BOSS}")
    return changes, refusals, notes


def where(key):
    return " AND ".join(f"{k} = ?" for k in key), list(key.values())


def current(db, tbl, key, cols):
    w, args = where(key)
    r = db.execute(f"SELECT {', '.join(cols)} FROM {tbl} WHERE {w}", args).fetchone()
    return None if r is None else dict(zip(cols, r))


def write(db, tbl, key, vals):
    w, args = where(key)
    db.execute(f"UPDATE {tbl} SET {', '.join(f'{c} = ?' for c in vals)} WHERE {w}", list(vals.values()) + args)


def apply(db, changes):
    db.execute(LOG_SCHEMA)
    for tbl, key, before, after in changes:
        write(db, tbl, key, after)
        a = audit(db, "update", tbl, key, before, after, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key, sort_keys=True), json.dumps(before, sort_keys=True), json.dumps(after, sort_keys=True), a))


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, tbl, key, before, after in db.execute(f"SELECT seq, tbl, rowkey, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key, before, after = json.loads(key), json.loads(before), json.loads(after)
        now = current(db, tbl, key, list(after))
        if now != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written: {now}")
            continue
        write(db, tbl, key, before)
        audit(db, "update", tbl, key, after, before, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"put back {back}, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        changes, refusals, notes = plan(db)
        for r in refusals:
            print("REFUSED:", r)
        if refusals:
            db.execute("ROLLBACK")
            return 1
        for n in notes:
            print(n)
        for tbl, key, before, after in changes:
            print(f"{tbl} {key}: {before} -> {after}")
        if not changes:
            db.execute("ROLLBACK")
            print("applied already: nothing done")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"report only: {len(changes)} rows to change (--apply to write)")
            return 0
        apply(db, changes)
        db.execute("COMMIT")
        print(f"applied: {len(changes)} rows")
        return 0
    except BaseException:
        if db.in_transaction:
            db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
