#!/usr/bin/env python3
"""Runnyeye's warren populated (the owner, 2026-11-15: "runnyeye dungeon is not populated yet").

The citadel's halls (world 0, x 16,200-16,460, z 12,000-12,350: a warren seven levels deep, from the gate at y 85 to
the deep tunnels at y 14) stood with one or two goblins of each kind. cmd/dungeonfill planned the fill from every floor
of the game's collision there (its doc): each height band filled with the kinds standing in it, by their counts, at
their levels there, camps of one to three on open floor (a tunnel's middle one at a time), none within 0.8 of a wall,
15 apart (a level's rise counting three times); and King Merdid's royal guards beside him (board entry 766 section 2,
Allakhazam 2003-07: "the king's guards ... in the underground complex"). The plan is tools/worlds/runnyeye_20261115.json
(made on Heirloom's 21:21 UTC pre-swap backup; Live's citadel rows are the same rows, ids and places).

What it writes, each row a gm_audit line and a row of runnyeye_1115_log (the undo's record):

  - npcs, world 0: each planned creature, a copy of its template row (the kind's nearest row: every column but the id,
    the place, the facing, the level, npc_id 0 and dt_kill), so its body, gear, faction, aggro and loot (all bound by
    name) are the kind's.
  - with --dungeon (Heirloom, where the instanced dungeons run, world 14): the same creatures in world 14, Runnyeye's
    instance templates, as tools/dungeons_20261112.py made the first ones: level 60-62 (a world 0 level's world 14
    level as that tool mapped the citadel's trash: the nearest world 0 level its notes name), npctemplate "#<row>" ->
    "dungeon trash", npcloot "#<row>" "dungeon runnyeye named" at 2%. An instance opened after reads them; an open
    one keeps what it has.

Refused when applied already, when a template row is not the plan's (name and place), or when a planned spot has a
creature within 1 already (a GM's since). --undo deletes every row it wrote, refused when one changed since (a GM's
edit), and puts sqlite_sequence back when nothing was added after: exact.

The server reads npcs at its start; on a running server the GM panel's reload_npcs spawns the new rows (and reads
npctemplate); npcloot is read at each corpse.

    python3 -E -s tools/runnyeye_20261115.py --db eqoa.db [--dungeon] [--apply | --undo] [--plan tools/worlds/runnyeye_20261115.json]
"""
import argparse
import json
import os
import re
import sqlite3
import sys
import time

RUN = "runnyeye 20261115"
LOG = "runnyeye_1115_log"
HERE = os.path.dirname(os.path.abspath(__file__))
PLAN = os.path.join(HERE, "worlds", "runnyeye_20261115.json")
DUNGEON_WORLD = 14
WHY = "the owner, 2026-11-15: \"runnyeye dungeon is not populated yet\"; tools/runnyeye_20261115.py"
PKS = {"npcs": ["id"], "npctemplate": ["name"], "npcloot": ["npc_name", "tablename"]}


def now_s():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def rowd(db, tbl, where, args):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def ensure_log(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   pk TEXT NOT NULL, after TEXT, at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
                   undone TEXT NOT NULL DEFAULT '')""")


def wrote(db, now, tbl, key):
    """Log and audit a row just inserted (key: its primary key's columns)."""
    after = rowd(db, tbl, " AND ".join(f"{k} = ?" for k in key), list(key.values()))
    db.execute(f"INSERT INTO {LOG} (run, tbl, pk, after) VALUES (?, ?, ?, ?)",
               (RUN, tbl, json.dumps(key, sort_keys=True), json.dumps(after, sort_keys=True)))
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', 'insert', ?, ?, NULL, ?, ?)""",
               (now, tbl, json.dumps(key), json.dumps(after, sort_keys=True), WHY))


def insert(db, now, tbl, vals):
    cols = list(vals)
    db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [vals[c] for c in cols])
    wrote(db, now, tbl, {k: vals[k] for k in PKS[tbl]})


def copy_row(db, cols, template, world, add, level):
    """A new npcs row: the template's every column but its id, place, facing, level, npc_id and death."""
    db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (template,))
    nid = db.execute("SELECT last_insert_rowid()").fetchone()[0]
    db.execute("""UPDATE npcs SET world = ?, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, npc_level = ?, npc_id = 0,
                  dt_kill = NULL WHERE id = ?""", (world, add["x"], add["y"], add["z"], add["facing"], level, nid))
    return nid


def dungeon_levels(db):
    """(world 0 level, world 14 level) of Runnyeye's instance trash, as tools/dungeons_20261112.py noted them."""
    pairs = []
    for note, lvl in db.execute("""SELECT t.note, n.npc_level FROM npctemplate t JOIN npcs n ON t.name = '#' || n.id
                                   WHERE t.template = 'dungeon trash' AND t.note LIKE 'Runnyeye''s trash:%' AND n.world = ?""",
                                (DUNGEON_WORLD,)):
        m = re.search(r"level (\d+) there", note)
        if m:
            pairs.append((int(m.group(1)), lvl))
    return pairs


def raid_level(pairs, level):
    best = min(abs(a - level) for a, _ in pairs)
    near = sorted(b for a, b in pairs if abs(a - level) == best)
    return near[len(near) // 2]


def undo(db):
    ensure_log(db)
    rows = db.execute(f"SELECT seq, tbl, pk, after FROM {LOG} WHERE run = ? AND undone = '' ORDER BY seq DESC", (RUN,)).fetchall()
    if not rows:
        sys.exit("nothing applied: nothing done")
    for seq, tbl, pk, after in rows:
        if tbl == "meta":
            continue
        key = json.loads(pk)
        cur = rowd(db, tbl, " AND ".join(f"{k} = ?" for k in key), list(key.values()))
        if cur is None or json.dumps(cur, sort_keys=True) != after:
            sys.exit(f"{tbl} {pk} changed since (a GM's edit?): nothing done")
    now = now_s()
    meta = {}
    with db:
        for seq, tbl, pk, after in rows:
            if tbl == "meta":
                meta = json.loads(after)
            else:
                key = json.loads(pk)
                db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{k} = ?" for k in key), list(key.values()))
                db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                              VALUES (?, 1, 'dab', 'tool', '', 'delete', ?, ?, ?, NULL, ?)""",
                           (now, tbl, pk, after, "undo of tools/runnyeye_20261115.py"))
            db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (now, seq))
        seq0 = meta.get("npcs_seq")
        top = db.execute("SELECT COALESCE(MAX(id), 0) FROM npcs").fetchone()[0]
        if seq0 is not None and top <= seq0:
            db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'", (seq0,))
    print(f"undone: {len(rows) - 1} rows")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--plan", default=PLAN)
    ap.add_argument("--dungeon", action="store_true", help="also world 14's Runnyeye instance (Heirloom)")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    if a.undo:
        undo(db)
        return
    ensure_log(db)
    if db.execute(f"SELECT 1 FROM {LOG} WHERE run = ? AND undone = '' LIMIT 1", (RUN,)).fetchone():
        print("applied already: nothing done")
        return
    with open(a.plan) as f:
        plan = json.load(f)
    world = plan["params"]["world"]
    adds = plan["adds"]
    for tid, t in plan["templates"].items():
        r = rowd(db, "npcs", "id = ?", (int(tid),))
        if r is None or r["npc_name"] != t["name"] or r["world"] != world or \
                abs(r["x_coord"] - t["x"]) > 0.5 or abs(r["z_coord"] - t["z"]) > 0.5:
            sys.exit(f"template row {tid} is not the plan's {t['name']} at ({t['x']:.1f}, {t['z']:.1f}): nothing done")
    for ad in adds:
        near = db.execute("""SELECT npc_name FROM npcs WHERE world = ? AND abs(x_coord - ?) < 1 AND abs(z_coord - ?) < 1
                             AND abs(y_coord - ?) < 2""", (world, ad["x"], ad["z"], ad["y"])).fetchone()
        if near:
            sys.exit(f"a {near[0]} stands at the plan's ({ad['x']}, {ad['y']}, {ad['z']}) already: nothing done")
    pairs = []
    if a.dungeon:
        if not db.execute("SELECT 1 FROM classtemplates WHERE name = 'dungeon trash'").fetchone():
            sys.exit("no 'dungeon trash' template (no instanced dungeons on this shard): nothing done")
        pairs = dungeon_levels(db)
        if not pairs:
            sys.exit(f"no Runnyeye instance trash in world {DUNGEON_WORLD} (tools/dungeons_20261112.py's): nothing done")
    seq = db.execute("SELECT seq FROM sqlite_sequence WHERE name = 'npcs'").fetchone()
    cols = [c[1] for c in db.execute("PRAGMA table_info(npcs)") if c[1] != "id"]
    now = now_s()
    counts, raid = {}, {}
    db.execute("BEGIN")
    db.execute(f"INSERT INTO {LOG} (run, tbl, pk, after) VALUES (?, 'meta', '{{}}', ?)",
               (RUN, json.dumps({"npcs_seq": seq[0] if seq else None, "plan": os.path.basename(a.plan), "dungeon": a.dungeon})))
    for ad in adds:
        nid = copy_row(db, cols, ad["template_row"], world, ad, ad["level"])
        wrote(db, now, "npcs", {"id": nid})
        counts[(ad["name"], ad["level"])] = counts.get((ad["name"], ad["level"]), 0) + 1
    if a.dungeon:
        for ad in adds:
            lv = raid_level(pairs, ad["level"])
            nid = copy_row(db, cols, ad["template_row"], DUNGEON_WORLD, ad, lv)
            wrote(db, now, "npcs", {"id": nid})
            insert(db, now, "npctemplate", {"name": f"#{nid}", "template": "dungeon trash",
                                            "note": f"Runnyeye's trash: {ad['name']} (runnyeye 20261115's fill, level {ad['level']} in world 0)",
                                            "source": RUN})
            insert(db, now, "npcloot", {"npc_name": f"#{nid}", "tablename": "dungeon runnyeye named", "chance": 2, "rolls": 1,
                                        "note": f"Runnyeye's trash {ad['name']}", "source": RUN})
            raid[lv] = raid.get(lv, 0) + 1
    n = db.execute(f"SELECT count(*) FROM {LOG} WHERE run = ? AND undone = '' AND tbl != 'meta'", (RUN,)).fetchone()[0]
    for (name, lvl), c in sorted(counts.items()):
        print(f"  {name:22s} L{lvl:<3d} x{c}")
    print(f"world {world}: {len(adds)} creatures added")
    if a.dungeon:
        print(f"world {DUNGEON_WORLD} (Runnyeye's instance): {len(adds)} trash, " +
              ", ".join(f"L{k} x{v}" for k, v in sorted(raid.items())))
    print(f"{n} rows written")
    if a.apply:
        db.commit()
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
