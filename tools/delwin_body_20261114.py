#!/usr/bin/env python3
"""Delwin Stitchfinger stands (Kyros's petition 2065, 2026-11-14: "Pretty funny since in the quest you kill him anyways,
but Delwin Stitchfinger is dying constantly and/or laying facedown when you go to talk to him the first time for bard
quest (either 13 or 15 I think?)").

Delwin (npcs row 1138, Bobble-by-Water, the bard's level 13 quest Ilenar's Robe: steps 2, 4 and 7 are his) has the
body Sandstorm gives him, CHAR.ESF 0xD7741560: the halfling made for the quest's end ("Oh... I feel faint...", the
fixed chocolates of step 7), one play-once clip (lying, up, down again) that the client copies into every animation
slot, so each idle fidget plays the fall again (claude-notes/f3-pass-20261028.md: it was judged then to be the record;
the petition says it is wrong where the quest starts). This gives him the halfling man's own body, 0xA3DB3FE2, the
body of Fritz Belgor across the room and of the city's other halfling men; his look (armour, hair, face) is left. He
then stands for every step; the faint at step 7 is the quest's words alone.

Found by name, world and place (refused when not exactly one row, or when its body is neither). A report by default;
--apply in one transaction with a gm_audit line, logged in delwin_body_1114_log; --undo puts it back while still as
written. NPCs are read at the start: RESTART after --apply.

    python3 -E -s tools/delwin_body_20261114.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "delwin_body_1114_log"
DETAIL = "petition 2065 (Kyros): Delwin Stitchfinger the halfling man's body, standing; tools/delwin_body_20261114.py"
WHO, VIA = "dab", "tool"
NAME, WORLD, X, Z, NEAR = "Delwin Stitchfinger", 0, 24631.9, 11115.6, 5.0
FAINT, STANDING = 0xD7741560, 0xA3DB3FE2
LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    npcid  INTEGER NOT NULL,
    before INTEGER NOT NULL,  -- modelid before
    after  INTEGER NOT NULL,  -- as written
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def compact(v):
    return json.dumps(v, separators=(",", ":"), sort_keys=True)


def audit(db, npcid, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', 'update', 'npcs', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, compact({"id": npcid}), compact({"modelid": before}),
                      compact({"modelid": after}), DETAIL))
    return cur.lastrowid


def plan(db):
    """(npcid, modelid now, refusals, applied)."""
    for t in ("npcs", "gm_audit"):
        if not table(db, t):
            return None, None, [f"no {t} table here"], False
    rows = db.execute("SELECT id, modelid FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) <= ? "
                      "AND abs(z_coord - ?) <= ?", (NAME, WORLD, X, NEAR, Z, NEAR)).fetchall()
    if len(rows) != 1:
        return None, None, [f"{len(rows)} rows named {NAME!r} at ({X}, {Z}) in world {WORLD}, want one"], False
    npcid, model = rows[0]
    model &= 0xFFFFFFFF
    if model == STANDING:
        return npcid, model, [], True
    if model != FAINT:
        return npcid, model, [f"row {npcid}'s body is {model:#010x}, neither {FAINT:#010x} nor {STANDING:#010x}"], False
    return npcid, model, [], False


def undo(db):
    """(back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for npcid, before, after in db.execute(f"SELECT npcid, before, after FROM {LOG} ORDER BY seq DESC").fetchall():
        now = db.execute("SELECT modelid FROM npcs WHERE id = ?", (npcid,)).fetchone()
        if now is None or now[0] != after:
            left += 1
            continue
        db.execute("UPDATE npcs SET modelid = ? WHERE id = ?", (before, npcid))
        audit(db, npcid, after, before)
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows back" + (f", {left} changed since (left, the log kept)" if left else ""))
            return 0
        npcid, model, refusals, applied = plan(db)
        if applied:
            db.execute("ROLLBACK")
            print("applied already: nothing to do")
            return 0
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED:\n  " + "\n  ".join(refusals))
            return 1
        raw = db.execute("SELECT modelid FROM npcs WHERE id = ?", (npcid,)).fetchone()[0]
        print(f"{npcid} {NAME}: modelid {model:#010x} -> {STANDING:#010x}")
        if not a.apply:
            db.execute("ROLLBACK")
            print("(report only: --apply to write; RESTART after)")
            return 0
        db.execute(LOG_SCHEMA)
        db.execute("UPDATE npcs SET modelid = ? WHERE id = ?", (STANDING, npcid))
        aid = audit(db, npcid, raw, STANDING)
        db.execute(f"INSERT INTO {LOG} (npcid, before, after, audit) VALUES (?, ?, ?, ?)", (npcid, raw, STANDING, aid))
        db.execute("COMMIT")
        print(f"applied: 1 row, in gm_audit and {LOG}; RESTART the server")
        return 0
    except Exception:
        db.execute("ROLLBACK")
        raise


if __name__ == "__main__":
    sys.exit(main())
