#!/usr/bin/env python3
"""The siege of Zentar's Keep's NPCs (the owner, 2026-11-12: "lets add a pvp zone for pvp events. zentaurs keep castle,
it is siege mode, team inside has king with high hp. entrances have a door/wood prop as a mob that doesnt attack with
high hp"; entity/siege.go, handlers/siege.go, claude-notes/siege-world-20261112.md). The siege copies its King and its
gates at each start from NPCs of their names that stand out of the world (npcspawnonly), so a GM can change their look,
level and size in the panel; their hit points are world.siege's (king_hp, gate_hp). This writes those originals in
world 15 (Zentar's Keep, tools/worlds/siege.json), each where the recipe stands its copy:

  King Zentar  a copy of Zentar Reese's row (101079: the keep's lord, a human in plate and a robe), level 60, the NPC
               bit off (a creature to fight), facing south into the courtyard
  Outer Gate   the castle's own portcullis (0xAA243482, the one drawn raised over the gatehouse's outer door), let down
  Inner Gate   into the outer and the inner door, level 60, no gear, the NPC bit off

and the three names in npcspawnonly. Refused (nothing done) where Zentar Reese's row is not as the 2026-11-12 Heirloom
data had it, or an NPC of one of the names (or its npcspawnonly row) is there already. Report only by default; --apply
in one transaction, every row in gm_audit and the log table siege_1112_log; a second --apply does nothing; --undo
deletes exactly what it wrote (a row changed since is left, said) and puts npcs' sequence back. The server reads npcs
and npcspawnonly at its start: apply it with the restart that brings the code (the siege falls back to the recipe's
bodies without it: a plain human King).

    python3 -E -s tools/siege_20261112.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import sqlite3
import sys

LOG = "siege_1112_log"
WHO = "siege"
VIA = "tool"
SRC = "siege1112"
DETAIL = "the siege of Zentar's Keep (the owner, 2026-11-12), its King and gates out of the world: tools/siege_20261112.py"

W = 15  # Zentar's Keep (tools/worlds/siege.json)
REESE = 101079  # Zentar Reese, the keep's lord in world 0
PORTCULLIS = 0xAA243482
GEAR = ("primary_weapon", "secondary_weapon", "shield", "torso", "forearm", "gloves", "legs", "feet", "head")

# name: (place, facing in the table's 256ths, overrides): the recipe's spots (tools/worlds/siege.json's arena)
NEW = {
    "King Zentar": ((5467.5, 54.1, 6460.0), 0, {}),
    "Outer Gate": ((5520.4, 59.5, 6477.25), 64, {"modelid": PORTCULLIS, "size": 1.0, "robe_style": 255, **{g: 0 for g in GEAR}}),
    "Inner Gate": ((5495.6, 59.5, 6477.25), 64, {"modelid": PORTCULLIS, "size": 1.0, "robe_style": 255, **{g: 0 for g in GEAR}}),
}


def table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def row(db, sql, args):
    cur = db.execute(sql, args)
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    s = json.dumps(m, sort_keys=True, separators=(",", ":"), ensure_ascii=False, default=str)
    return s.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")


def audit(db, action, tbl, key, before, after, detail):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key),
                      None if before is None else go_json(before), None if after is None else go_json(after), detail))
    return cur.lastrowid


def plan(db):
    """(rows, refusals): the npcs rows to insert (no id) and the npcspawnonly names."""
    refuse = []
    for t in ("npcs", "npcspawnonly", "gm_audit"):
        if not table(db, t):
            refuse.append(f"no table {t}")
    if refuse:
        return [], refuse
    src = row(db, "SELECT * FROM npcs WHERE id = ?", (REESE,))
    if src is None:
        refuse.append(f"npcs {REESE}: missing")
    else:
        for c, v in (("npc_name", "Zentar Reese"), ("world", 0), ("modelid", 0x70D898C6)):
            if src.get(c) != v:
                refuse.append(f"npcs {REESE}: {c} is {src.get(c)!r}, not {v!r}")
    for name in NEW:
        n = db.execute("SELECT count(*) FROM npcs WHERE lower(trim(npc_name)) = lower(?)", (name,)).fetchone()[0]
        if n:
            refuse.append(f"{n} npcs rows named {name!r} already")
        if row(db, "SELECT * FROM npcspawnonly WHERE name = ?", (name,)) is not None:
            refuse.append(f"npcspawnonly {name!r}: there already")
    if refuse:
        return [], refuse
    rows = []
    for name, (at, facing, over) in NEW.items():
        r = dict(src)
        del r["id"]
        r.update({"npc_name": name, "x_coord": at[0], "y_coord": at[1], "z_coord": at[2], "facing": facing, "world": W,
                  "zone": 0, "npc_id": 0, "npc_level": 60, "npc_type": 0, "dt_kill": None})
        r.update(over)
        rows.append(r)
    return rows, []


LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq INTEGER PRIMARY KEY AUTOINCREMENT, tbl TEXT NOT NULL, rowkey TEXT NOT NULL, after TEXT, audit INTEGER NOT NULL DEFAULT 0)"""


def apply(db, rows):
    db.execute(LOG_SCHEMA)
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after) VALUES ('sqlite_sequence', '{{}}', ?)",
               (json.dumps(None if seq is None else seq[0]),))
    ids = {}
    for r in rows:
        cols = list(r)
        cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [r[c] for c in cols])
        nid = cur.lastrowid
        db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
        ids[r["npc_name"]] = nid
        now = row(db, "SELECT * FROM npcs WHERE id = ?", (nid,))
        a = audit(db, "insert", "npcs", {"id": nid}, None, now, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES ('npcs', ?, ?, ?)",
                   (json.dumps({"id": nid}), json.dumps(now, sort_keys=True, default=str), a))
        so = {"name": r["npc_name"], "note": "the siege of Zentar's Keep: copied at each start (entity/siege.go)", "source": SRC}
        db.execute("INSERT INTO npcspawnonly (name, note, source) VALUES (?, ?, ?)", (so["name"], so["note"], so["source"]))
        a = audit(db, "insert", "npcspawnonly", {"name": so["name"]}, None, so, DETAIL)
        db.execute(f"INSERT INTO {LOG} (tbl, rowkey, after, audit) VALUES ('npcspawnonly', ?, ?, ?)",
                   (json.dumps({"name": so["name"]}), json.dumps(so, sort_keys=True), a))
    return ids


def undo(db):
    if not table(db, LOG):
        return 0, 0
    back = left = 0
    seq_was = db.execute(f"SELECT after FROM {LOG} WHERE tbl = 'sqlite_sequence'").fetchone()
    for seq, tbl, key, after in db.execute(
            f"SELECT seq, tbl, rowkey, after FROM {LOG} WHERE tbl != 'sqlite_sequence' ORDER BY seq DESC").fetchall():
        k = json.loads(key)
        if tbl == "npcs":
            now = row(db, "SELECT * FROM npcs WHERE id = ?", (k["id"],))
            same = now is not None and json.dumps(now, sort_keys=True, default=str) == after
        else:
            now = row(db, "SELECT * FROM npcspawnonly WHERE name = ?", (k["name"],))
            same = now is not None and json.dumps(now, sort_keys=True) == after
        if not same:
            left += 1
            print(f"  left: {tbl} {k}, changed since it was written")
            continue
        if tbl == "npcs":
            db.execute("DELETE FROM npcs WHERE id = ?", (k["id"],))
        else:
            db.execute("DELETE FROM npcspawnonly WHERE name = ?", (k["name"],))
        audit(db, "delete", tbl, k, now, None, "undo: " + DETAIL)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if left == 0:
        if seq_was is not None:
            was = json.loads(seq_was[0])
            if was is None:
                db.execute("DELETE FROM sqlite_sequence WHERE name = 'npcs'")
            else:
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (was,))
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
            print(f"applied already ({LOG} is there): nothing to do (--undo first to write it again)")
            return 0
        rows, refusals = plan(db)
        for r in rows:
            print(f"  npcs insert: {r['npc_name']!r} world {r['world']} at ({r['x_coord']}, {r['y_coord']}, {r['z_coord']}) "
                  f"facing {r['facing']}, body {r['modelid'] & 0xFFFFFFFF:08X}, level {r['npc_level']}, npc_type {r['npc_type']}; npcspawnonly")
        if refusals:
            print("refused, nothing done:")
            for r in refusals:
                print("  " + r)
            return 1
        if not a.apply:
            print(f"report only: {len(rows)} npcs rows and {len(rows)} npcspawnonly rows would be written (--apply writes them)")
            return 0
        db.execute("BEGIN IMMEDIATE")
        try:
            ids = apply(db, rows)
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print("applied, each row in gm_audit and " + LOG + ": " + ", ".join(f"{n} npcs {i}" for n, i in ids.items()))
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
