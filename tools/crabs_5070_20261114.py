#!/usr/bin/env python3
"""Petition 5070 (Innoruuk, Live, 2026-11-14): "spikeshell crabs not on the coast where they should be east of neriak"
(he stood at world 0 (25620.42, 9219.1)). The necromancer guide (eqoa.forumotion.com t57, level 10, Jarek's
Creation): "kill a Spikeshell Crab (cons white to yellow), located just on the other side of Neriak, southeast of the
city ... You will find the crabs between a red spire and the shore. You need the SHELL". Three quests take a part of
one: 100463 step 2 (the shell), 100483 step 2 (the claw), 100353 step 2 (the leg), each a loot step of chance 0.5.

The four spikeshell crabs (npcs 2469-2472, levels 10-11, the same rows on both shards) stood 450-750 south of him, on
the shore past the darkclaw crabs (z 9654-9962). The spires are terrain: two rock pinnacles south-east of the city
(world 0's ground file: floors to 235 round (25150, 9370) and (25220, 9545), the beach at 54.2), the sea's edge
(the .env water) at x ~25680 there. The camp, between them and the sea on the beach, eight crabs 32 or more apart,
each on open sand (no placed thing within 10, the floor level within 3), 15 to 45 from the water, at least 25 from the
beach's other creatures (rats, flies, spiderlings, the darkclaw crabs): the four rows moved there and four more, copies
of 2470 (level 11) and 2469 (level 10) as the four's levels go, each with the others' 90 s respawn (npcrespawn). Crabs
roam in and out of the water (npcmove "spikeshell crab"). The three steps' journal lines say where.

Refused, nothing written, when a row is not as expected (a GM's edit since). Report only by default; --apply in one
transaction, a gm_audit line a row, log crabs5070_log; a second --apply does nothing; --undo exact (a row changed
since is left and said). npcs and npcrespawn are read at the server's start (quests at start and a GM's reload): a
restart (a keep restart).

    python3 -E -s tools/crabs_5070_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "crabs5070_log"
WHO, VIA = "dab", "tool"
DETAIL = ("petition 5070 (Innoruuk): a camp of spikeshell crabs on the shore south-east of Neriak, between the spires and "
          "the sea: tools/crabs_5070_20261114.py")
Y = 54.3
# the four rows: (x, y, z) before, (x, z) after
MOVES = ((2469, (25478.4, 53.3, 9962.5), (25645.0, 9270.0)),
         (2470, (25646.3, 54.4, 9803.5), (25655.0, 9310.0)),
         (2471, (25668.8, 54.5, 9736.1), (25630.0, 9330.0)),
         (2472, (25632.8, 54.3, 9653.8), (25615.0, 9400.0)))
# the new ones: the row copied, (x, z), facing byte
NEW = ((2470, (25640.0, 9420.0), 60), (2469, (25640.0, 9455.0), 150),
       (2470, (25610.0, 9470.0), 210), (2469, (25640.0, 9490.0), 20))
RESPAWN = 90.0
JOURNALS = {
    (100463, 2): ("Take the spikeshell crab shell from the spikeshell crabs south-east of Neriak, by the shore.",
                  "Take the spikeshell crab shell from the crabs on the beach south-east of Neriak, between "
                  "the rock spires and the sea."),
    (100483, 2): ("Take the spikeshell crab claw from the spikeshell crabs south-east of Neriak, by the shore.",
                  "Take the spikeshell crab claw from the crabs on the beach south-east of Neriak, between "
                  "the rock spires and the sea."),
    (100353, 2): ("Collect Spikeshell Crab Leg from spikeshell crabs on the shore south-east of Neriak.",
                  "Take a spikeshell crab leg from the crabs on the beach south-east of Neriak, between the "
                  "rock spires and the sea."),
}
ROW = "*row*"

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    keycol  TEXT NOT NULL,
    keyval  TEXT NOT NULL,
    col     TEXT NOT NULL,
    before  TEXT,
    after   TEXT,
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def rowdict(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def near(a, b, eps=0.05):
    return a is not None and abs(a - b) < eps


def audit(db, tbl, keycol, keyval, col, before, after):
    action = "update" if col != ROW else ("delete" if after is None else "insert")
    if col == ROW:
        before, after = ({} if before is None else before), ({} if after is None else after)
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    rowkey = dict(zip(keycol.split(","), (int(v) for v in str(keyval).split(","))))
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, compact(rowkey),
                      compact(before if col == ROW else {col: before}), compact(after if col == ROW else {col: after}), DETAIL))
    return cur.lastrowid


def plan(db):
    changes, lines, refusals = [], [], []
    rows = {}
    for rid, was, to in MOVES:
        r = rowdict(db, "SELECT * FROM npcs WHERE id = ?", (rid,))
        if r is None or (r["npc_name"] or "").lower() != "spikeshell crab" or r["world"] != 0:
            refusals.append(f"npcs {rid}: {None if r is None else (r['npc_name'], r['world'])}, want a spikeshell crab of world 0")
            continue
        rows[rid] = r
        now = (r["x_coord"], r["y_coord"], r["z_coord"])
        want = (to[0], Y, to[1])
        if all(near(a, b) for a, b in zip(now, want)):
            lines.append(f"npcs {rid} at the camp already")
            continue
        if not all(near(a, b) for a, b in zip(now, was)):
            refusals.append(f"npcs {rid} stands at {now}, not {was} (moved since)")
            continue
        for col, old, new in zip(("x_coord", "y_coord", "z_coord"), now, want):
            if old != new:
                changes.append(("npcs", "id", rid, col, old, new))
        lines.append(f"npcs {rid} (spikeshell crab, level {r['npc_level']}) {was} -> {want}")
        if db.execute("SELECT seconds FROM npcrespawn WHERE npc_row = ?", (rid,)).fetchone() != (RESPAWN,):
            refusals.append(f"npcrespawn {rid}: not {RESPAWN} s")
    for (qid, step), (before, after) in JOURNALS.items():
        r = db.execute("SELECT journal, kind, lower(target) FROM queststeps WHERE questid = ? AND step = ?", (qid, step)).fetchone()
        if r is None:
            refusals.append(f"queststeps {qid}/{step}: not there")
        elif r[1:] != ("loot", "spikeshell crab"):
            refusals.append(f"queststeps {qid}/{step}: {r[1:]}, want a loot of spikeshell crab")
        elif r[0] == after:
            lines.append(f"queststeps {qid}/{step} journal as wanted already")
        elif r[0] != before:
            refusals.append(f"queststeps {qid}/{step} journal {r[0]!r}, not {before!r}")
        else:
            changes.append(("queststeps", "questid,step", f"{qid},{step}", "journal", before, after))
            lines.append(f"queststeps {qid}/{step} journal -> {after!r}")
    if refusals:
        return changes, lines, refusals
    placed = [n for n in NEW if db.execute("SELECT 1 FROM npcs WHERE lower(npc_name) = 'spikeshell crab' AND world = 0 AND "
                                           "abs(x_coord - ?) < 0.5 AND abs(z_coord - ?) < 0.5", n[1]).fetchone()]
    if len(placed) == len(NEW):
        lines.append("the four new crabs are there already")
    elif placed:
        refusals.append(f"{len(placed)} of the four new crabs are there already: {placed}")
    else:
        nid = db.execute("SELECT MAX(id) FROM npcs").fetchone()[0] + 1
        if table(db, "sqlite_sequence"):
            seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
            if seq and seq[0] >= nid:
                nid = seq[0] + 1
        for src, (x, z), facing in NEW:
            row = dict(rows[src], id=nid, npc_id=nid, x_coord=x, y_coord=Y, z_coord=z, facing=facing, dt_kill=None)
            changes.append(("npcs", "id", nid, ROW, None, row))
            changes.append(("npcrespawn", "npc_row", nid, ROW, None,
                            {"npc_row": nid, "seconds": RESPAWN, "source": "crabs5070",
                             "note": "petition 5070: a spikeshell crab of the camp south-east of Neriak, the others' 90 s"}))
            lines.append(f"npcs {nid} a spikeshell crab (a copy of {src}, level {rows[src]['npc_level']}) at ({x}, {Y}, {z})")
            nid += 1
    return changes, lines, refusals


def write(db, tbl, keycol, keyval, col, before, after):
    cols, vals = keycol.split(","), [int(v) for v in str(keyval).split(",")]
    where = " AND ".join(f"{c} = ?" for c in cols)
    if col == ROW:
        now = rowdict(db, f"SELECT * FROM {tbl} WHERE {where}", vals)
        if after is None:
            if now != before:
                return False
            return db.execute(f"DELETE FROM {tbl} WHERE {where}", vals).rowcount == 1
        if now is not None:
            return False
        names = list(after)
        db.execute(f"INSERT INTO {tbl} ({', '.join(names)}) VALUES ({', '.join('?' * len(names))})", [after[n] for n in names])
        return True
    return db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {where} AND {col} IS ?", (after, *vals, before)).rowcount == 1


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for _, tbl, keycol, keyval, col, before, after, _a in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        before, after = json.loads(before), json.loads(after)
        if not write(db, tbl, keycol, keyval, col, after, before):
            print(f"left: {tbl} {keyval} {col} changed since")
            left += 1
            continue
        audit(db, tbl, keycol, keyval, col, after, before)
        back += 1
    if not left:
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
            print(f"applied already ({LOG} is there): --undo first")
            return 0
        changes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not changes:
            print("nothing to do")
            return 0
        if not a.apply:
            print(f"report only: {len(changes)} rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            db.execute(LOG_SCHEMA)
            for tbl, keycol, keyval, col, before, after in changes:
                if not write(db, tbl, keycol, keyval, col, before, after):
                    raise RuntimeError(f"{tbl} {keyval} changed under the update")
                a_id = audit(db, tbl, keycol, keyval, col, before, after)
                db.execute(f"INSERT INTO {LOG} (tbl, keycol, keyval, col, before, after, audit) VALUES (?, ?, ?, ?, ?, ?, ?)",
                           (tbl, keycol, str(keyval), col, json.dumps(before), json.dumps(after), a_id))
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {len(changes)} rows, log {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
