#!/usr/bin/env python3
"""Norach's ferry to the Isle of Dread, open to anyone for 10,000 tunar (the owner, 2026-11-11: "lets add Norach to
Cazic Thule Zone on the little island with a palm tree. he will teleport you to Isle of Dread onto the little raft
for 10k"), with the server's code of the same branch (handlers/waysender.go: Norach's trip no longer quest-only,
landing on the fishing raft moored in the cove south of Graumord's pier, (24578, 52.6, 3626), facing the pier).

Norach is already there: the npcs row of the Return Home import stands on the islet with the palm tree in the lake
south of Cazic Thule's temple, (15066.6, 55.05, 31311), on the islet's top (the walls file: 55.04). His trip was
gated by the Isle of Dread's portal quest (EQOA-Data Quests/Portal/isle-of-dread.json), which asks a dreadnaught's
medallion first and tells the player he takes "only those who carry his medallion": that is no longer so, and while
it is in the game it is offered before the trip to every player of its level, again after each refusal. So:

  quests "The Medallion of the Faceless" (import key Portal/isle-of-dread/portal, giver Norach): a draft (enabled
    1 -> 0): offered to no one; the characters who finished it keep the mark; its unlock of the trip bars no one
    while it is a draft (questunlocks is left as it is).
  quests "The Faceless Generals" (Portal/isle-of-dread/trials-1): requires that quest no longer (requires -> ''):
    the isle's chain goes on from the Citizens of Graumord standing it already asks (Cleansing the Isle raises it).
  npcs Norach: nothing when he stands on the islet (within 10 of the spot); none in the world: a row made there, as
    Heirloom's (his body 0x341DA296, level 63, facing 243); one somewhere else: refused, nothing done.

tools/quests/data/portal.json says the same (the medallion quest a draft by its "unresolved", the generals' requires
dropped), so an import writes what this does. A quest a shard does not have (another shard's import) is left out
and said. Report only by default; --apply in one transaction, a gm_audit line a row, log norach_1111_log; --undo
exact (a row changed since is left and said). The quests and NPCs are read at the server's start: run it as the
before.sh of the restart bringing this code.

    tools/norach_20261111.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "norach_1111_log"
DETAIL = "Norach's ferry to the Isle of Dread's raft for 10,000 (the owner, 2026-11-11): tools/norach_20261111.py"
WHO, VIA = "dab", "tool"

MEDALLION_KEY, GENERALS_KEY = "Portal/isle-of-dread/portal", "Portal/isle-of-dread/trials-1"
NORACH = "Norach"
ISLET = (15066.6, 55.0469, 31311.0)  # the islet's top under the palm tree, world 0 (Cazic-Thule, zone 160)
NEAR = 10.0
# Heirloom's row (Return Home's import), for a shard that has none
NORACH_ROW = dict(npc_name=NORACH, zone=576, unk_a=1, unk_b=82, x_coord=ISLET[0], y_coord=ISLET[1], z_coord=ISLET[2],
                  facing=243, world=0, dt_kill=None, hp=1, hp_percent=255, modelid=874357398, size=1.0, unk_d=0, unk_f=0,
                  unk_g=0, primary_weapon=0, secondary_weapon=0, shield=0, unk_h=0, torso=0, forearm=0, gloves=0, legs=0,
                  feet=0, head=0, hair_color=0, hair_length=0, hair_style=0, face=0, robe_style="0", race="255", unk_j=0,
                  npc_level=63, npc_type=384, npc_id=0, unk_k=0, unk_i="15", pd_icon=0, unk_m=0, unk_n=1, unk_o=0)

KEYS = {"quests": ("questid",), "npcs": ("id",)}

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,
    rowkey  TEXT NOT NULL,      -- the row's key (JSON)
    before  TEXT,               -- the row before (JSON), NULL: made here
    after   TEXT NOT NULL,      -- the row as written (JSON)
    audit   INTEGER NOT NULL DEFAULT 0
)"""


def go_json(m):
    """A row as Go's json.Marshal writes a map (keys sorted, <>& escaped), as the panel's audit has it."""
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def dump(row):
    return json.dumps(row, sort_keys=True, default=str)


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row_of(db, tbl, key):
    where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key[c] for c in KEYS[tbl]))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def key_of(tbl, row):
    return {c: row[c] for c in KEYS[tbl]}


def quest_by_key(db, key):
    if not table(db, "quest_import_quests"):
        return None
    r = db.execute("SELECT questid FROM quest_import_quests WHERE key = ?", (key,)).fetchone()
    return None if r is None else row_of(db, "quests", {"questid": r[0]})


def plan(db):
    """(changes, lines, refusals): each change (table, the row before or None, the row after)."""
    changes, lines, refusals = [], [], []

    med = quest_by_key(db, MEDALLION_KEY)
    if med is None:
        lines.append(f"no quest {MEDALLION_KEY!r} here: nothing gates Norach's trip")
    elif med["giver"] != NORACH or med["name"] != "The Medallion of the Faceless":
        refusals.append(f"quest {med['questid']} ({MEDALLION_KEY}) is {med['name']!r} given by {med['giver']!r}, not Norach's medallion")
    elif med["enabled"] == 1:
        changes.append(("quests", med, dict(med, enabled=0)))
        lines.append(f"quests {med['questid']} {med['name']!r}: a draft (offered to no one; its finishers keep the mark)")
    else:
        lines.append(f"quests {med['questid']} {med['name']!r}: a draft already")

    gen = quest_by_key(db, GENERALS_KEY)
    if gen is None:
        lines.append(f"no quest {GENERALS_KEY!r} here")
    elif med is not None and gen["requires"] == str(med["questid"]):
        changes.append(("quests", gen, dict(gen, requires="")))
        lines.append(f"quests {gen['questid']} {gen['name']!r}: requires {gen['requires']} -> nothing "
                     f"(its standing with the Citizens of Graumord, {gen['factionreq']!r}, stays)")
    elif gen["requires"] in ("", None):
        lines.append(f"quests {gen['questid']} {gen['name']!r}: requires nothing already")
    else:
        refusals.append(f"quest {gen['questid']} {gen['name']!r} requires {gen['requires']!r}, not the medallion quest alone")

    rows = db.execute("SELECT id, world, x_coord, y_coord, z_coord FROM npcs WHERE npc_name = ?", (NORACH,)).fetchall()
    on = [r for r in rows if r[1] == 0 and ((r[2] - ISLET[0]) ** 2 + (r[4] - ISLET[2]) ** 2) ** 0.5 <= NEAR]
    if on:
        lines.append(f"npcs {on[0][0]} Norach stands on the islet ({on[0][2]:.1f}, {on[0][3]:.2f}, {on[0][4]:.1f}): left as he is")
    elif rows:
        refusals.append("Norach stands elsewhere: " + "; ".join(f"npcs {r[0]} world {r[1]} ({r[2]:.1f}, {r[3]:.1f}, {r[4]:.1f})" for r in rows))
    else:
        changes.append(("npcs", None, dict(NORACH_ROW)))
        lines.append(f"npcs: Norach made on the islet {ISLET} world 0")
    return changes, lines, refusals


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def seq_of(db):
    if not table(db, "sqlite_sequence"):
        return None
    r = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    return None if r is None else r[0]


def apply(db, changes):
    """The plan written, in the caller's transaction: each row, its gm_audit line and its line in the log."""
    db.execute(LOG_SCHEMA)
    seq_before = seq_of(db)
    for tbl, before, after in changes:
        if before is None:
            cols = list(after)
            cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [after[c] for c in cols])
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (cur.lastrowid, cur.lastrowid))
            key = {"id": cur.lastrowid}
            row = row_of(db, tbl, key)
            a = audit(db, "insert", tbl, key, None, row, DETAIL)
        else:
            key = key_of(tbl, before)
            sets = [c for c in after if after[c] != before.get(c)]
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in sets)} WHERE "
                       + " AND ".join(f"{c} = ?" for c in KEYS[tbl]), [after[c] for c in sets] + [key[c] for c in KEYS[tbl]])
            row = row_of(db, tbl, key)
            a = audit(db, "update", tbl, key, before, row, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
                   (tbl, json.dumps(key), None if before is None else dump(before), dump(row), a))
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after) VALUES ('sqlite_sequence', '{{}}', ?, ?)",
               (json.dumps(seq_before), json.dumps(seq_of(db))))
    return len(changes)


def undo(db):
    """Each logged row still as written put back, the last first, its gm_audit line; then the npcs id sequence, when no
    other row took one since. Returns (back, left)."""
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    seq_row = db.execute(f"SELECT before, after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for seq, tbl, key, before, after in db.execute(
            f"SELECT seq, tbl, rowkey, before, after FROM {LOG} WHERE tbl != 'sqlite_sequence' ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        now = row_of(db, tbl, key)
        if now is None or dump(now) != after:
            left += 1
            print(f"  left: {tbl} {key}, changed since it was written")
            continue
        where = " AND ".join(f"{c} = ?" for c in KEYS[tbl])
        if before is None:
            db.execute(f"DELETE FROM {tbl} WHERE {where}", [key[c] for c in KEYS[tbl]])
            audit(db, "delete", tbl, key, now, None, "undo: " + DETAIL)
        else:
            b = json.loads(before)
            db.execute(f"UPDATE {tbl} SET {', '.join(c + ' = ?' for c in b)} WHERE {where}", [b[c] for c in b] + [key[c] for c in KEYS[tbl]])
            audit(db, "update", tbl, key, now, row_of(db, tbl, key), "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_row is not None:
            was, written = json.loads(seq_row[0]), json.loads(seq_row[1])
            if was is not None and seq_of(db) == written:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (was,))
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
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
            return 1
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
            n = apply(db, changes)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"applied: {n} rows, each in gm_audit and {LOG}")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
