#!/usr/bin/env python3
"""Guk's stray mummy (Waldorf's ticket 2010, 2026-11-13: "I've killed 306 of 307. Is the last npc the teleport?", then
"You actually have/put a mummy at a random camp outside the dungeon near the water").

The 307th of Guk's instance is world 14 row 166250, a "mummy" (level 60) at (24979.3, 59.0, 28791.3): a copy that
tools/dungeons_20261112.py made of world 0 row 116153, a level 18 Innothule swamp mummy, because its Guk name rule
("^(...|mumm|...)", meant for the Mummified Guardians and crypt mummies) took it, and its spot lies in Guk's one box
(24580-25100, 28700-29480). It stands on the swamp's surface above Guk's lower halls: in world 0's ground there is no
floor at that x, z but the swamp's at 59.0 (the halls around it are at -11 and -12), so no one inside the instance
reaches it, and every instance counted it.

--apply removes the row and what the dungeons tool gave it (its npctemplate "#166250", "dungeon trash", and its two
npcloot rows under "#166250"), in one transaction, a gm_audit line a row, logged in guk_stray_mummy_1113_log; a second
--apply does nothing. --undo puts every row back exactly (refused for a row whose key is taken again). Refused, nothing
written, when the row is not as found (its name, world, place) or its dependants are not as the dungeons tool wrote them.
NPCs are read at the start: live after a restart (an instance open now keeps its copy until it closes).

    tools/guk_stray_mummy_20261113.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "guk_stray_mummy_1113_log"
WHO, VIA = "dab", "tool"
DETAIL = "Guk's stray mummy, outside the dungeon (Waldorf's ticket 2010): tools/guk_stray_mummy_20261113.py"
ROW, NAME, WORLD, X, Z = 166250, "mummy", 14, 24979.275, 28791.262
KEY = f"#{ROW}"
FOUND = 0.05
# (table, key columns, where, its rows' count as the dungeons tool left them)
DEPENDANTS = [
    ("npctemplate", ["name"], "name = ?", 1),
    ("npcloot", ["npc_name", "tablename"], "npc_name = ?", 2),
]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,   -- the row's key (JSON)
    before TEXT NOT NULL,   -- the whole row deleted (JSON)
    audit  INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def cols(db, t):
    return [r[1] for r in db.execute(f"PRAGMA table_info({t})")]


def rows(db, t, where, arg):
    c = cols(db, t)
    return [dict(zip(c, r)) for r in db.execute(f"SELECT {', '.join(c)} FROM {t} WHERE {where}", (arg,)).fetchall()]


def plan(db):
    """(deletes [(table, keycols, row)], refusals); nothing when done."""
    if table(db, LOG) and db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        return [], []
    found = rows(db, "npcs", "id = ?", ROW)
    r = found[0] if found else None
    if r is None or (r["npc_name"] or "").strip() != NAME or r["world"] != WORLD or \
            abs(r["x_coord"] - X) > FOUND or abs(r["z_coord"] - Z) > FOUND:
        return [], [f"npcs #{ROW} not as found ({NAME} w{WORLD} {X},{Z}): "
                    f"{r and (r['npc_name'], r['world'], r['x_coord'], r['z_coord'])}"]
    deletes, refusals = [("npcs", ["id"], r)], []
    for t, keys, where, want in DEPENDANTS:
        got = rows(db, t, where, KEY) if table(db, t) else []
        if len(got) != want:
            refusals.append(f"{t}: {len(got)} rows under {KEY}, want {want}: {got}")
        deletes += [(t, keys, d) for d in got]
    return deletes, refusals


def audit(db, action, t, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, t, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), DETAIL))
    return cur.lastrowid


def apply(db, deletes):
    db.execute(LOG_SCHEMA)
    for t, keys, r in deletes:
        key = {k: r[k] for k in keys}
        db.execute(f"DELETE FROM {t} WHERE {' AND '.join(k + ' = ?' for k in keys)}", [r[k] for k in keys])
        a = audit(db, "delete", t, key, r, None)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, audit) VALUES (?, ?, ?, ?)", (t, go_json(key), go_json(r), a))


def undo(db):
    """(put back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    for seq, t, rowkey, before, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        key, b = json.loads(rowkey), json.loads(before)
        if db.execute(f"SELECT 1 FROM {t} WHERE {' AND '.join(k + ' = ?' for k in key)}", list(key.values())).fetchone():
            print(f"left: {t} {key} is taken again")
            left += 1
            continue
        c = [k for k in cols(db, t) if k in b]
        db.execute(f"INSERT INTO {t} ({', '.join(c)}) VALUES ({', '.join('?' * len(c))})", [b[k] for k in c])
        audit(db, "insert", t, key, None, b)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
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
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        try:
            back, left = undo(db)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"put back {back} rows, {left} left (taken again)" if back or left else "nothing to undo")
        return 0 if left == 0 else 1
    deletes, refusals = plan(db)
    if refusals:
        print("refused, nothing done:")
        for r in refusals:
            print("  " + r)
        return 1
    if not deletes:
        print("done already: nothing to do")
        return 0
    for t, keys, r in deletes:
        print(f"remove: {t} {go_json({k: r[k] for k in keys})}")
    if not a.apply:
        print(f"report only: {len(deletes)} rows would be removed (--apply removes them)")
        return 0
    db.execute("BEGIN IMMEDIATE")
    try:
        apply(db, deletes)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied: {len(deletes)} rows removed, in gm_audit and {LOG}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
