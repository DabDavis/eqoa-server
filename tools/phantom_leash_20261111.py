#!/usr/bin/env python3
"""The West Plateau's phantoms follow twice as far (2026-11-11; petition 1535: "Phantoms camp in West Plateau. Mobs do
not have a long enough leash and go back to spawn location prematurely. should be roughly double what it is
currently"), on the server's code of the same branch (npcaggro's leash column, entity/chase.go's leashLocked: an older
server neither reads it nor has the column).

  npcaggro "phantom": leash 600 (the world's 300, doubled). Every NPC of the name is that camp (21, world 2). The name
    has no row today, so the row is made with aggressive -1: no word on aggro (the world's rules decide as they did with
    no row: the phantoms attack on sight as now, and a row of 1 would also have cut them out of any faction's rules).
    A row there already keeps its aggressive and radius; only its leash is set.

The leash column is added when the database has none (the code's start adds it too: ALTER TABLE ... ADD COLUMN leash
REAL NOT NULL DEFAULT 0, every row 0 = the world's leash, as now); --undo drops it again when this tool added it.
Refused, nothing written, when a row of the name has a leash of its own (not 0) already. Report only by default;
--apply in one transaction, a gm_audit line, log phantom_leash_1111_log; --undo exact (a row changed since left and
said). The rules are read at the start and at a GM panel edit of npcaggro: after a restart onto this code it is live.

    tools/phantom_leash_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "phantom_leash_1111_log"
NAME = "phantom"
LEASH = 600.0
WHO, VIA = "dab", "tool"
SOURCE = "petition 1535"
NOTE = "Petition 1535: the West Plateau's phantoms follow twice the world's leash. tools/phantom_leash_20261111.py"
DETAIL = "phantoms' leash 600 (petition 1535, 2026-11-11): tools/phantom_leash_20261111.py"
COLS = ["npc_name", "aggressive", "radius", "note", "source", "leash"]

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq          INTEGER PRIMARY KEY AUTOINCREMENT,
    npc_name     TEXT NOT NULL,
    before       TEXT,              -- the row before (JSON), NULL: made here
    after        TEXT NOT NULL,     -- the row as written (JSON)
    added_column INTEGER NOT NULL,  -- 1: this tool added npcaggro.leash
    audit        INTEGER NOT NULL DEFAULT 0
)"""


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def has_leash(db):
    return any(r[1] == "leash" for r in db.execute("PRAGMA table_info(npcaggro)"))


def class_key(name):
    """database.NPCClassKey: any case, trimmed, no trailing $."""
    return name.strip().rstrip("$").strip().lower()


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def rows_of_name(db, with_leash):
    cols = COLS if with_leash else COLS[:-1]
    out = []
    for r in db.execute(f"SELECT {', '.join(cols)} FROM npcaggro"):
        row = dict(zip(cols, r))
        if not with_leash:
            row["leash"] = 0.0
        if class_key(row["npc_name"]) == class_key(NAME):
            out.append(row)
    return out


def plan(db):
    """(before, after, lines, refusals): the one row, before None when it is made."""
    if not table(db, "npcaggro"):
        return None, None, [], ["no table npcaggro here"]
    n = db.execute("SELECT count(*), group_concat(DISTINCT world) FROM npcs WHERE npc_name = ?", (NAME,)).fetchone()
    lines = [f"npcs {NAME!r}: {n[0]} (worlds {n[1]})"]
    rows = rows_of_name(db, has_leash(db))
    if len(rows) > 1:
        return None, None, lines, [f"{len(rows)} npcaggro rows of the name: {rows}"]
    if rows:
        before = rows[0]
        if before["leash"] == LEASH:
            return None, None, lines + [f"npcaggro {before['npc_name']!r}: leash {LEASH:g} already"], []
        if before["leash"]:
            return None, None, lines, [f"npcaggro {before['npc_name']!r} has a leash of its own: {before['leash']}"]
        after = dict(before, leash=LEASH)
        lines.append(f"npcaggro {before['npc_name']!r}: leash 0 (300) -> {LEASH:g}, aggressive {before['aggressive']} kept")
        return before, after, lines, []
    after = {"npc_name": NAME, "aggressive": -1, "radius": 0.0, "note": NOTE, "source": SOURCE, "leash": LEASH}
    lines.append(f"npcaggro {NAME!r}: made, aggressive -1 (no word: aggro as now), leash {LEASH:g}")
    return None, after, lines, []


def audit(db, action, key, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, 'npcaggro', ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, go_json({"npc_name": key}),
                      None if before is None else go_json(before), None if after is None else go_json(after), DETAIL))
    return cur.lastrowid


def apply(db, before, after):
    added = 0
    if not has_leash(db):
        db.execute("ALTER TABLE npcaggro ADD COLUMN leash REAL NOT NULL DEFAULT 0")
        added = 1
    if before is None:
        db.execute(f"INSERT INTO npcaggro ({', '.join(COLS)}) VALUES ({', '.join('?' * len(COLS))})", [after[c] for c in COLS])
        a = audit(db, "insert", after["npc_name"], None, after)
    else:
        db.execute("UPDATE npcaggro SET leash = ? WHERE npc_name = ?", (after["leash"], before["npc_name"]))
        a = audit(db, "update", before["npc_name"], before, after)
    db.execute(LOG_SCHEMA)
    db.execute(f"INSERT INTO {LOG} (npc_name, before, after, added_column, audit) VALUES (?, ?, ?, ?, ?)",
               (after["npc_name"], None if before is None else go_json(before), go_json(after), added, a))


def undo(db):
    """(put back, left): the logged row put back as it was when still as written, the column dropped when added here."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    drop_col = False
    for seq, name, before, after, added, _ in db.execute(f"SELECT * FROM {LOG} ORDER BY seq DESC").fetchall():
        after = json.loads(after)
        now = db.execute(f"SELECT {', '.join(COLS)} FROM npcaggro WHERE npc_name = ?", (name,)).fetchone()
        if now is None or dict(zip(COLS, now)) != after:
            print(f"left: npcaggro {name!r} changed since: {now}")
            left += 1
            continue
        if before is None:
            db.execute("DELETE FROM npcaggro WHERE npc_name = ?", (name,))
            audit(db, "delete", name, after, None)
        else:
            b = json.loads(before)
            db.execute("UPDATE npcaggro SET leash = ? WHERE npc_name = ?", (b["leash"], name))
            audit(db, "update", name, after, b)
        back += 1
        drop_col = drop_col or bool(added)
    if left:
        return back, left
    if drop_col:
        if db.execute("SELECT count(*) FROM npcaggro WHERE leash != 0").fetchone()[0]:
            print("the leash column kept: another row has a leash now")
        else:
            db.execute("ALTER TABLE npcaggro DROP COLUMN leash")
    db.execute(f"DROP TABLE {LOG}")
    return back, 0


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
        before, after, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if after is None:
            print("nothing to do")
            return 0
        if not a.apply:
            print("report only: 1 row would be written (--apply writes it)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            apply(db, before, after)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: 1 row, in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
