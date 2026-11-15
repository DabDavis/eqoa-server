#!/usr/bin/env python3
"""Sandsharks off Stormhaven (the owner, 2026-11-15: "add sandsharks at stormhaven"), for Abrion Oleken's Tattered
Shark Skins (teamjosh H&G key 50: "The skins drop off sandsharks along the coast of the Stormhaven area"), which
tools/handg_givers_20261115.py had pointed at Freeport's sharks for want of any. Run it on each database, after that
tool.

  - Ten "sandshark" spawns, copies of a mako shark's row (its body, size, npc_type 0) at levels 15-19, in three groups
    in the sea west of Stormhaven, each spot measured on world 0's ground and water files (the sea floor, the water's
    top at 51, the shark ten over the floor as Freeport's swim): north off the oceanic rats' islands, west of the
    town, south toward Wilwalt; each 300 or more from any NPC (the town's people and guards 400 or more). Their rows by name as the mako
    shark's (npcclass, npcmove: in the water; npctemplate: roamer; lootcreature: marine) and a respawn of 390 s.
  - The quest's loot step counts the sandsharks alone (Freeport's mako, bloodfin and white-tipped sharks dropped:
    the page's skins are Stormhaven's), its chance (0.5) as it was.

Every row it changes is found by comparing the tables before and after (tools/seed_guide_gaps.py's machinery, set
"handg-sandsharks-20261115"), so --undo is exact; a gm_audit line a row, one for an undo. Report only by default;
--apply in one transaction; a second --apply does nothing; --undo, again nothing. NPCs are read at the server's start
or by the GM panel's reload_npcs; quests at the start.

    tools/handg_sandsharks_20261115.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import os
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import seed_guide_gaps as sgg  # noqa: E402

SET = "handg-sandsharks-20261115"
WHO, VIA = "dab", "tool"
DETAIL = "Sandsharks off Stormhaven for Tattered Shark Skins (the owner, 2026-11-15): tools/handg_sandsharks_20261115.py"
NAME, LIKE, RESPAWN = "sandshark", "mako shark", 390.0
sgg.TABLES = ("npcs", "npcclass", "npcmove", "npctemplate", "lootcreature", "npcrespawn", "quests", "queststeps",
              "queststeptargets", "sqlite_sequence")
BYNAME = (("npcclass", "name"), ("npcmove", "name"), ("npctemplate", "name"), ("lootcreature", "npc_name"))
# group, x, y, z (the floor + 10; the water's top 51), facing, level
SPAWNS = [
    ("north", 4130.0, 34.1, 18630.0, 40, 15), ("north", 4170.0, 34.1, 18650.0, 200, 16),
    ("north", 4150.0, 34.1, 18680.0, 120, 17), ("north", 4190.0, 34.1, 18620.0, 90, 18),
    ("west", 4230.0, 34.1, 19230.0, 10, 16), ("west", 4270.0, 34.1, 19260.0, 170, 17),
    ("west", 4250.0, 34.1, 19290.0, 230, 19),
    ("south", 4160.0, 35.2, 19770.0, 60, 15), ("south", 4180.0, 36.5, 19790.0, 150, 18),
    ("south", 4200.0, 35.8, 19790.0, 250, 19),
]
GROUPS = {"north": "off the oceanic rats' islands, about 520 west of Guard Opheena",
          "west": "west of the town, about 400 west of Smithy Vorgel (the dark portal 300 east)",
          "south": "toward Wilwalt's shore, about 450 west of the Erudite Courier"}
QUEST, GIVER, OLD = "Tattered Shark Skins", "Abrion Oleken", ("mako shark", "bloodfin shark", "white-tipped shark")
JOURNAL = "Kill the sandsharks off Stormhaven's coast for four tattered shark skins."
NOTE = (" Its creatures since 2026-11-15 (the owner: \"add sandsharks at stormhaven\"): the sandsharks off Stormhaven's coast "
        "(tools/handg_sandsharks_20261115.py), Freeport's sharks no longer.")


def key(n):
    return (n or "").strip().lower()


def cols(db, t):
    return [r[1] for r in db.execute(f'PRAGMA table_info("{t}")')]


def has(db, t):
    return sgg.has(db, t)


def quest(db):
    r = db.execute("SELECT questid FROM quests WHERE name = ? AND lower(trim(giver)) = ?", (QUEST, key(GIVER))).fetchall()
    return r[0][0] if len(r) == 1 else None


def spawned(db):
    return [r for r in db.execute("SELECT id, x_coord, z_coord FROM npcs WHERE lower(trim(npc_name)) = ? AND world = 0", (NAME,))]


def report(db, say):
    q = quest(db)
    say(f"{QUEST}: {'quest ' + str(q) if q else 'NOT HERE (run tools/handg_givers_20261115.py first)'}")
    if q:
        st = db.execute("SELECT target, chance FROM queststeps WHERE questid = ? AND step = 1", (q,)).fetchone()
        also = [r[0] for r in db.execute("SELECT target FROM queststeptargets WHERE questid = ? AND step = 1", (q,))]
        say(f"  its loot step: {st[0]} + {also or 'none'}, chance {st[1]}")
    have = spawned(db)
    say(f"sandsharks here: {len(have)}")
    for g, x, y, z, f, lv in SPAWNS:
        say(f"  {g:5} ({x}, {y}, {z}) L{lv}: {'here' if any(abs(a - x) < 1 and abs(b - z) < 1 for _, a, b in have) else 'to make'}")
    for g, why in GROUPS.items():
        say(f"  {g}: {why}")


def run_on(db, say):
    q = quest(db)
    src = db.execute("SELECT * FROM npcs WHERE lower(trim(npc_name)) = ? AND world = 0 ORDER BY id LIMIT 1", (LIKE,)).fetchone()
    c = cols(db, "npcs")
    row0 = dict(zip(c, src))
    have = spawned(db)
    made = []
    for g, x, y, z, f, lv in SPAWNS:
        if any(abs(a - x) < 1 and abs(b - z) < 1 for _, a, b in have):
            continue
        row = {k: v for k, v in row0.items() if k != "id"}
        row.update(npc_name=NAME, x_coord=x, y_coord=y, z_coord=z, facing=f, npc_level=lv, world=0)
        cur = db.execute(f'INSERT INTO npcs ({",".join(row)}) VALUES ({",".join("?" * len(row))})', list(row.values()))
        nid = cur.lastrowid
        if "npc_id" in c:
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (nid, nid))
        if has(db, "npcrespawn"):
            db.execute("INSERT OR IGNORE INTO npcrespawn (npc_row, seconds, note, source) VALUES (?, ?, ?, ?)",
                       (nid, RESPAWN, f"sandshark off Stormhaven ({g})", SET))
        made.append(nid)
        say(f"  sandshark {nid} {g} ({x}, {y}, {z}) L{lv}")
    for t, nc in BYNAME:
        if not has(db, t) or db.execute(f"SELECT 1 FROM {t} WHERE lower(trim({nc})) = ?", (NAME,)).fetchone():
            continue
        tc = cols(db, t)
        for r in db.execute(f"SELECT * FROM {t} WHERE lower(trim({nc})) = ?", (LIKE,)).fetchall():
            r = dict(zip(tc, r))
            r.pop("id", None)
            r[nc] = NAME
            if "source" in r:
                r["source"] = SET
            db.execute(f'INSERT INTO {t} ({",".join(r)}) VALUES ({",".join("?" * len(r))})', list(r.values()))
    st = db.execute("SELECT target, journal FROM queststeps WHERE questid = ? AND step = 1", (q,)).fetchone()
    if key(st[0]) != NAME:
        db.execute("UPDATE queststeps SET target = ?, journal = ? WHERE questid = ? AND step = 1", (NAME, JOURNAL, q))
        db.execute(f"DELETE FROM queststeptargets WHERE questid = ? AND step = 1 AND lower(target) IN ({','.join('?' * len(OLD))})",
                   (q, *OLD))
        db.execute("UPDATE quests SET notes = notes || ? WHERE questid = ? AND instr(notes, ?) = 0", (NOTE, q, NOTE.strip()))
        say(f"  quest {q}: its loot step counts the sandsharks ({st[0]} and Freeport's sharks dropped)")
    say(f"{len(made)} sandsharks made")


def audit_rows(db, counts_):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for tbl, op, rid, old in db.execute(f"SELECT tbl, op, rid, old FROM {sgg.LOG} WHERE setname = ? ORDER BY seq", (SET,)).fetchall():
        if tbl in ("sqlite_sequence", sgg.LOG) or op == "made":
            continue
        new = db.execute(f'SELECT * FROM "{tbl}" WHERE rowid = ?', (rid,)).fetchone()
        after = json.dumps(dict(zip(cols(db, tbl), new)), separators=(",", ":")) if new else None
        db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                   "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                   (at, acct[0] if acct else 0, WHO, VIA, {"added": "insert", "changed": "update", "gone": "delete"}[op], tbl,
                    json.dumps({"rowid": rid}), old or None, after, DETAIL))


def audit_undo(db, n):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if has(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
               "VALUES (?, ?, ?, ?, '', 'update', ?, ?, NULL, NULL, ?)",
               (at, acct[0] if acct else 0, WHO, VIA, sgg.LOG, json.dumps({"setname": SET}), f"undone, {n} rows put back: " + DETAIL))


def is_in(db):
    return has(db, sgg.LOG) and db.execute(f"SELECT 1 FROM {sgg.LOG} WHERE setname = ?", (SET,)).fetchone() is not None


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    say = print
    try:
        if a.undo:
            if not is_in(db):
                print("nothing to undo")
                return 0
            sgg.undo(db, SET, say, extra=audit_undo)
            print("NPCs: the GM panel's reload_npcs (or a restart); quests: a restart")
            return 0
        report(db, say)
        if is_in(db):
            print(f"applied already ({SET} is in {sgg.LOG}): nothing to do (--undo to take it back)")
            return 0
        if not quest(db) or not db.execute("SELECT 1 FROM npcs WHERE lower(trim(npc_name)) = ? AND world = 0", (LIKE,)).fetchone():
            print(f"refused, nothing done: no {QUEST} quest of {GIVER}, or no {LIKE} to copy")
            return 1
        print()
        sgg.run = lambda db_, spec, say_: run_on(db_, say_)
        sgg.apply(db, {"set": SET}, a.apply, say, extra=audit_rows)
        if a.apply:
            print("NPCs: the GM panel's reload_npcs (or a restart); quests are read at the server's start: the next restart")
        return 0
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
