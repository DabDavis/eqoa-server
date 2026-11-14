#!/usr/bin/env python3
"""Croc Island's crocodiles moved as the GM Jibcrotwo stood (2026-11-14, batch tix5, lane crocs), and the Staff of the
Jade Forest's stats (the GM Kobe). Both shards; Live's 5029 (Innoruuk: "Crocs at croc island by fp don't give space for
your characters to stand, constantly attacked") is the same ground, and these moves are its answer.

A petition names its creature by the entity id the client was shown (a session's own), not its row: each is the row of
that name its roam (70 from its spawn, npcmove's tame crocodile/crocodile roam) can have brought to the target's spot,
the one assignment of the island's twelve tame crocodile rows that gives every petition a row within 70 (the least
total distance, 256): 2087 2536 (53 off), 2090 105043 (39), 2091 2539 (36), 2092 2535 (33), 2093 2534 (41), 2094
105157 (54); of the two crocodile rows, 2088 1038 (59), 2089 3216 (45) (the other way 50/55: the two land four apart,
both standing, facing within 4 degrees: either way is the same on the ground). Both shards hold the same rows.

  2087: "This croc should not roam this far on this side of the island, move it in the pond": npcs 2536's spawn to the
        island's pond (world 0's .env water leaves: the pool east of the hut, x 25406-25490, z 17300-17370, its floor
        54.09, the surface 62), at its middle (25448, 17338), the spot farthest from its shore (28.4); its roam an
        npcwander row of radius 20 round it, so its every leg ends in the water.
  2088: npcs 1038 (crocodile) to (25382.4, 69.95, 17326.24), facing byte 42 (1.031 rad), and it does not roam: an
        npcwander row of radius -1 (entity/movement.go: a row's own no-roam).
  2089: npcs 3216 (crocodile) to (25378.31, 69.95, 17332.77), facing byte 45 (1.104 rad), no roam the same way.
  2090: npcs 105043 (tame crocodile) spawn to (25438.52, 68.25, 17395.17); its roam as before (none of its own: its
        name's 70), round the new spawn.
  2091: npcs 2539 spawn to (25470.02, 67.82, 17382.75); 2092: npcs 2535 to (25493.41, 68.25, 17286.81); 2093: npcs
        2534 to (25450.78, 67.39, 17289.53); 2094: npcs 105157 to (25417.38, 63.4, 17374.4); each its roam as it was,
        its facing kept.
  2086 (Kobe): "item Staff of the Jade Forest should have stats WIS 15 CHA 50 Power 70": every itempattern record of
        that name (Heirloom 31006 and 990138, the Nagafen drop's copy; Live 31006): wis 0 -> 15, cha 0 -> 50, POWMAX
        0 -> 70. It had no other stat (every stat and resist column 0, no itemeffect, no weapon proc): nothing else.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log crocs_1114_log; a second --apply does nothing; --undo exact (a row changed
since is left and said). --only limits it to those petitions. npcs, npcwander and items are read at the server's start:
a restart, with the npcwander code (a negative radius stands) in the same branch.

    python3 -E -s tools/crocs_staff_20261114.py --db eqoa.db [--apply | --undo] [--only 2087,2088,...]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "crocs_1114_log"
WHO, VIA = "dab", "tool"

# petition: (npcs row, its name, (x, y, z) before (its spawn), (x, y, z) after, facing after or None, wander or None)
MOVES = {
    2087: (2536, "tame crocodile", (25331.7, 68.5, 17353.4), (25448.0, 54.09, 17338.0), None, 20.0),
    2088: (1038, "crocodile", (25367.8, 69.9, 17327.7), (25382.4, 69.95, 17326.24), 42, -1.0),
    2089: (3216, "crocodile", (25357.053, 68.25098, 17328.258), (25378.31, 69.95, 17332.77), 45, -1.0),
    2090: (105043, "tame crocodile", (25388.8, 67.9766, 17395.7), (25438.52, 68.25, 17395.17), None, None),
    2091: (2539, "tame crocodile", (25536.4, 74.9, 17408.9), (25470.02, 67.82, 17382.75), None, None),
    2092: (2535, "tame crocodile", (25491.8, 63.1, 17330.5), (25493.41, 68.25, 17286.81), None, None),
    2093: (2534, "tame crocodile", (25398.9, 58.4, 17263.3), (25450.78, 67.39, 17289.53), None, None),
    2094: (105157, "tame crocodile", (25329.8, 63.8672, 17303.0), (25417.38, 63.4, 17374.4), None, None),
}
STAFF = "Staff of the Jade Forest"
STAFF_STATS = {"wis": 15, "cha": 50, "POWMAX": 70}

DETAILS = {
    2087: "petition 2087 (Jibcrotwo): Croc Island's tame crocodile spawns in the pond and keeps to it",
    2088: "petition 2088 (Jibcrotwo): Croc Island's crocodile moved into the hut, facing east by south, not roaming",
    2089: "petition 2089 (Jibcrotwo): Croc Island's crocodile moved into the hut, facing east by south, not roaming",
    2090: "petition 2090 (Jibcrotwo): Croc Island's tame crocodile spawns and roams where the GM stood",
    2091: "petition 2091 (Jibcrotwo): Croc Island's tame crocodile spawns where the GM stood, its roam kept",
    2092: "petition 2092 (Jibcrotwo): Croc Island's tame crocodile spawns where the GM stood, its roam kept",
    2093: "petition 2093 (Jibcrotwo): Croc Island's tame crocodile spawns where the GM stood, its roam kept",
    2094: "petition 2094 (Jibcrotwo): Croc Island's tame crocodile spawns where the GM stood, its roam kept",
    2086: "petition 2086 (Kobe): the Staff of the Jade Forest WIS 15, CHA 50, Power 70",
}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket  INTEGER NOT NULL,
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    keyval  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""

ROW = "*row*"  # a change of a whole row: before None an insert, after None a delete (the row as a dict)


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def rowdict(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def audit(db, ticket, tbl, keycol, keyval, col, before, after):
    action = "update" if col != ROW else ("delete" if after is None else "insert")
    if col == ROW:
        before, after = ({} if before is None else before), ({} if after is None else after)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact({keycol: int(keyval)}),
                      compact(before if col == ROW else {col: before}), compact(after if col == ROW else {col: after}),
                      DETAILS[ticket] + ": tools/crocs_staff_20261114.py"))
    return cur.lastrowid


class Plan:
    def __init__(self, db):
        self.db, self.changes, self.lines, self.refusals = db, [], [], []

    def column(self, ticket, tbl, keycol, key, col, want, after, label):
        row = rowdict(self.db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,))
        if row is None:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) is not there")
            return
        v = row[col]
        if v not in want:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) {col} = {v!r}, want {want!r}")
            return
        self.changes.append((ticket, tbl, keycol, key, col, v, after))
        self.lines.append(f"{ticket}: {tbl} {key} ({label}) {col} {v!r} -> {after!r}")

    def insert(self, ticket, tbl, keycol, row, label):
        self.changes.append((ticket, tbl, keycol, row[keycol], ROW, None, row))
        self.lines.append(f"{ticket}: {tbl} {row[keycol]} ({label}) added")

    def delete(self, ticket, tbl, keycol, key, label, check):
        row = rowdict(self.db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (key,))
        if row is None:
            self.refusals.append(f"{ticket}: {tbl} {key} ({label}) is not there")
            return
        bad = check(row)
        if bad:
            self.refusals.append(f"{ticket}: {tbl} {key}: {bad}")
            return
        self.changes.append((ticket, tbl, keycol, key, ROW, row, None))
        self.lines.append(f"{ticket}: {tbl} {key} ({label}) deleted")


def plan(db, only):
    p = Plan(db)
    for t, (rid, name, was, to, facing, wander) in MOVES.items():
        if t not in only:
            continue
        r = rowdict(db, "SELECT * FROM npcs WHERE id = ?", (rid,))
        if r is None or r["npc_name"] != name or r["world"] != 0:
            p.refusals.append(f"{t}: npcs {rid} is not a {name} in world 0")
            continue
        here = (r["x_coord"], r["y_coord"], r["z_coord"])
        if any(abs(a - b) > 0.01 for a, b in zip(here, was)):
            p.refusals.append(f"{t}: npcs {rid} ({name}) spawns at {here}, not {was} (moved since?)")
            continue
        for tbl, col in (("npcpaths", "npcid"), ("campspawns", "npcid"), ("npcwander", "npcid")):
            if table(db, tbl) and col in [c[1] for c in db.execute(f"PRAGMA table_info({tbl})")] and \
                    db.execute(f"SELECT 1 FROM {tbl} WHERE {col} = ?", (rid,)).fetchone():
                p.refusals.append(f"{t}: {tbl} holds a row of npcs {rid}: not foreseen")
        for col, v in zip(("x_coord", "y_coord", "z_coord"), to):
            p.column(t, "npcs", "id", rid, col, (r[col],), v, name)
        if facing is not None:
            p.column(t, "npcs", "id", rid, "facing", (r["facing"],), facing, name)
        if wander is not None:
            note = {20.0: "petition 2087: keeps to Croc Island's pond", -1.0: f"petition {t}: does not roam"}[wander]
            p.insert(t, "npcwander", "npcid", {"npcid": rid, "radius": wander, "note": note, "source": "crocs_1114"},
                     f"{name}'s own wander {wander}")
    if 2086 in only:
        recs = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? ORDER BY patternid", (STAFF,)).fetchall()
        if not recs:
            p.lines.append(f"2086: no {STAFF} on this shard: skipped")
        for (pid,) in recs:
            r = rowdict(db, "SELECT * FROM itempattern WHERE patternid = ?", (pid,))
            for col, v in STAFF_STATS.items():
                p.column(2086, "itempattern", "patternid", pid, col, (0,), v, STAFF)
            if table(db, "itemeffect") and db.execute("SELECT 1 FROM itemeffect WHERE patternid = ?", (pid,)).fetchone():
                p.lines.append(f"2086: {pid} has an itemeffect row: left as it is")
    return p


def write(db, tbl, keycol, keyval, col, before, after):
    if col == ROW:
        now = rowdict(db, f"SELECT * FROM {tbl} WHERE {keycol} = ?", (keyval,))
        if after is None:  # delete: the row must be as it was read
            if now != before:
                return False
            return db.execute(f"DELETE FROM {tbl} WHERE {keycol} = ?", (keyval,)).rowcount == 1
        if now is not None:  # insert: nothing there
            return False
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [after[n] for n in names])
        return True
    return db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {keycol} = ? AND {col} IS ?", (after, keyval, before)).rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, ticket, tbl, keycol, keyval, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        keyval = int(keyval)
        if not write(db, tbl, keycol, keyval, col, after, before):
            print(f"left: {ticket} {tbl} {keyval} {col} changed since")
            left += 1
            continue
        audit(db, ticket, tbl, keycol, keyval, col, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    ap.add_argument("--only", default=",".join(str(t) for t in DETAILS))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    only = {int(t) for t in a.only.split(",") if t.strip()}
    if not only <= set(DETAILS):
        sys.exit(f"--only: unknown petitions {sorted(only - set(DETAILS))}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    try:
        if a.undo:
            db.execute("BEGIN IMMEDIATE")
            try:
                back, left = undo(db)
                db.execute("COMMIT")
            except BaseException:
                db.execute("ROLLBACK")
                raise
            print(f"put back {back} rows, {left} left (changed since)" if back or left else "nothing to undo")
            return 0 if left == 0 else 1
        if table(db, LOG):
            print(f"applied already ({LOG} is there): nothing to do (--undo first to redo)")
            return 0
        p = plan(db, only)
        for line in p.lines:
            print(line)
        if p.refusals:
            print("refused, nothing done:")
            for r in p.refusals:
                print("  " + r)
            return 1
        if not p.changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(p.changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for ticket, tbl, keycol, keyval, col, before, after in p.changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, ticket, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (ticket, tbl, keycol, keyval, col, before, after, audit) "
                           "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                           (ticket, tbl, keycol, str(keyval), col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(p.changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
