#!/usr/bin/env python3
"""Kratas's Deep, the first heroic raid: Kratas in a dungeon's instance, his rising a trigger in each party's copy
(Heirloom petition 3396, Tanker: "Please make all raids, yes all, have a heroic dungeon"; the owner, 2026-11-28: "yes
to the heroic dungeons for all raids", the trigger raids first, a raid-gear set a raid, Heirloom only).

The open world's Kratas rises from the whale bones under the water near Old Man Antiok (trigger "Kratas Rises", fired
by the Kratas quest's last step, 101265 step 4). Its heroic copy is the shore's zone (Tunaria 159, cell (6, 15);
tools/worlds/dungeons.json's "kratas", world 14) with the trigger engine running in each party's instance
(handlers/gmtriggers.go's instance, 2026-11-28):

  - npcs, world 14: a copy of each creature of the shore's box (13050-13900, 30950-31700) of its kinds (FAMILY: the
    spinebacks, griffons, drakes and panthers) or named (a capitalised name), never friendly (npc_type 0x80: Old Man
    Antiok and the rest stay out) nor a quest's seeded row (Captain Soohn and Marrow Maw, quest 100787's: as every
    heroic dungeon leaves its quests' rows out), levels raised as the other heroic dungeons' (trash 60-62, named
    63-64); npctemplate "#<row>" -> its tier; npcloot of the tier's tables. Today: two spinebacks and a black drake.
  - Kratas: a copy of his row as a template OUTSIDE the box (13476, 23.4, 30900), so no instance's opening stands him
    up; level 65, "dungeon boss", the boss tables. The trigger brings a copy of that row to the bones in each party's
    instance.
  - gmtriggers slot 401 "Heroic Kratas Rises", world 14 at the whale bones: an awaitnear start (a player within 60 of
    the bones in a party's copy), its lock (held per instance), a clock of an hour (empty 10 minutes: a wipe lets him
    rise again), Kratas spawned there hostile, his death awaited and told, then the run waits out its copy's life (so
    he rises once in a copy: its creatures never come back, as every heroic dungeon's) and ends with it.
  - itempattern 995145-995156: twelve raid-gear records (ITEMS: a tuned raid boss's drop under a name of the deep's
    own; kept over logout, rent 1, as the others' are), loottables "dungeon kratas boss|named".
  - a Dungeon Warden twin beside the landing (the beach at (13250, 54.12, 31040)), a copy of Blackburrow's.

Needs tools/dungeons_20261112.py applied (the tier templates and the twins): a shard without the dungeons (the Live
Server) is refused, nothing written. The server reads npcs, templates, loot and trigger rows at its start: run it in
a keep restart's before.sh, with the world file that has Kratas's Shore (world 14's recipe of 2026-11-28, 15 zones,
108,507,864 bytes, CRC c1ce7e6f) and its ground in worlds/. Refused when applied already (a second run does nothing)
or a row it would write exists; --undo deletes every row it wrote, refused when one changed since (a GM's edit);
exact. Report only by default; --apply in one transaction, a gm_audit line a row, log heroic_kratas_1128_log.

    python3 -E -s tools/heroic_kratas_20261128.py --db eqoa.db [--apply | --undo] [--recipe tools/worlds/dungeons.json]
"""
import argparse
import importlib.util
import json
import math
import os
import re
import sqlite3
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("dungeons_base", os.path.join(HERE, "dungeons_20261112.py"))
base = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(base)

RUN, LOG = "heroic kratas 20261128", "heroic_kratas_1128_log"
DETAIL = ("Heirloom petition 3396, the owner 2026-11-28: Kratas's Deep, a heroic raid (the trigger in each party's "
          "instance); tools/heroic_kratas_20261128.py")
KEY, SOURCE = "kratas", 0
ITEM_FIRST = 995145
WARDEN = base.WARDEN
TWIN_OF = (12586.0, 12760.0)  # Blackburrow's twin (world 14), whose row the new twin copies
BOSS = "Kratas"
BONES = (13475.6, 23.4, 31455.6)          # the open world trigger's place: the whale bones under the water
BOSS_TEMPLATE_AT = (13476.0, 23.4, 30900.0)  # the boss row's own spot in world 14: outside the box
SLOT = 401
TRIGGER = "Heroic Kratas Rises"

FAMILY = r"^(spineback|night griffon|griffon|griffenne|black drake|shadow panther|dread spirit)"
KINDS = r"^$"
ITEMS = [
    ("boss", "Kratas's Abyssal Plate", 6509),            # warrior's chest (Chestplate of the Icy Whirlwind)
    ("boss", "Tunic of the Drowned Deep", 1342),          # rogue's chest (Tunic of Fierce Flames)
    ("boss", "Tidewarden's Breastplate", 6451),           # paladin, cleric, shaman chest (Breastplate of Disbelief)
    ("boss", "Whalebone Bulwark", 6117),                  # paladin, cleric, shaman shield (toxic wyrm tower)
    ("boss", "Kratas's Barbed Fang", 961101),             # Fang of Yhafiran
    ("boss", "Ring of the Sunken Whale", 961106),         # Ring of the Sleeping Wyrm
    ("named", "Tidewalker's Sea Coat", 718006),           # bard's chest (Tunic of Intense Heat)
    ("named", "Deepstalker's Hide Tunic", 718002),        # ranger's chest (Tunic of Snowy Days)
    ("named", "Tidecaller's Tunic", 6476),                # magician's chest (Tunic of the Creator)
    ("named", "Robe of the Drowned", 6477),               # necromancer's chest (Tunic of the decrepit)
    ("named", "Kelp-Woven Girdle", 961104),               # Coil of Yhafiran
    ("named", "Leggings of the Briny Depths", 713096),    # Trousers of the Ancient Dead
]

base.PKS.setdefault("gmtriggers", ["slot"])


def trigger_steps(boss_row):
    x, y, z = BONES
    return [
        {"do": "awaitnear", "start": True, "radius": 60},
        {"do": "lock", "lock": "heroic kratas"},
        {"do": "clock", "seconds": 3600, "empty": 600},
        {"do": "popup", "text": "The water churns over the whale bones. Something vast stirs in the deep."},
        {"do": "spawn", "npc": f"#{boss_row}", "spread": 1, "minutes": 60, "hostile": True, "world": 14,
         "x": x, "y": y, "z": z},
        {"do": "broadcast", "text": "Kratas rises from among the whale bones!"},
        {"do": "awaitdead", "npc": BOSS},
        {"do": "broadcast", "text": "Kratas sinks lifeless among the whale bones. The deep is still."},
        {"do": "end"},
        {"do": "wait", "seconds": 14400},
        {"do": "despawn"},
    ]


def wrote(db, now, tbl, key):
    """Log and audit a row just inserted (key: its primary key's columns)."""
    where = " AND ".join(f"{k} = ?" for k in key)
    after = base.rowd(db, tbl, where, list(key.values()))
    db.execute(f"INSERT INTO {LOG} (run, tbl, pk, after) VALUES (?, ?, ?, ?)",
               (RUN, tbl, json.dumps(key, sort_keys=True), json.dumps(after, sort_keys=True)))
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', 'insert', ?, ?, NULL, ?, ?)""",
               (now, tbl, json.dumps(key), json.dumps(after, sort_keys=True), DETAIL))


def insert(db, now, tbl, vals):
    cols = list(vals)
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [vals[c] for c in cols])
    key = {k: (cur.lastrowid if k == "id" and k not in vals else vals[k]) for k in base.PKS[tbl]}
    wrote(db, now, tbl, key)


def copy_npc(db, cols, row_id):
    db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (row_id,))
    return db.execute("SELECT last_insert_rowid()").fetchone()[0]


def ensure_log(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   pk TEXT NOT NULL, after TEXT, at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
                   undone TEXT NOT NULL DEFAULT '')""")


def undo(db):
    if not db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (LOG,)).fetchone():
        sys.exit("nothing applied: nothing done")
    rows = db.execute(f"SELECT seq, tbl, pk, after FROM {LOG} WHERE run = ? AND undone = '' ORDER BY seq DESC", (RUN,)).fetchall()
    if not rows:
        sys.exit("nothing applied: nothing done")
    for seq, tbl, pk, after in rows:
        key = json.loads(pk)
        cur = base.rowd(db, tbl, " AND ".join(f"{k} = ?" for k in key), list(key.values()))
        if cur is None or json.dumps(cur, sort_keys=True) != after:
            sys.exit(f"{tbl} {pk} changed since (a GM's edit?): nothing done")
    now = base.now_s()
    with db:
        for seq, tbl, pk, after in rows:
            key = json.loads(pk)
            db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{k} = ?" for k in key), list(key.values()))
            db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                          VALUES (?, 1, 'dab', 'tool', '', 'delete', ?, ?, ?, NULL, ?)""",
                       (now, tbl, pk, after, "undo of tools/heroic_kratas_20261128.py"))
            db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (now, seq))
        for t in ("npcs", "loottables"):
            if db.execute(f"SELECT MAX(id) FROM {t}").fetchone()[0] is None:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
            else:
                db.execute(f"UPDATE sqlite_sequence SET seq = (SELECT MAX(id) FROM {t}) WHERE name = ?", (t,))
    print(f"undone: {len(rows)} rows")


def select(db, d):
    """World 0's rows of the shore's box but Kratas, each with its tier."""
    spawnonly = {r[0].lower() for r in db.execute("SELECT name FROM npcspawnonly")}
    seeded = {r[0] for r in db.execute("SELECT npc_row FROM quest_npc_seed_rows")}
    seen, rows = set(), []
    for b in d["boxes"]:
        x0, z0, x1, z1 = min(b[0], b[2]), min(b[1], b[3]), max(b[0], b[2]), max(b[1], b[3])
        cur = db.execute("""SELECT * FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?
                            ORDER BY id""", (SOURCE, x0, x1, z0, z1))
        cols = [c[0] for c in cur.description]
        for r in cur.fetchall():
            r = dict(zip(cols, r))
            n = r["npc_name"]
            if n == BOSS or r["id"] in seen or r["npc_type"] & 0x80 or f"#{r['id']}" in spawnonly or r["id"] in seeded:
                continue
            if n.lower() in spawnonly and n != BOSS:
                continue
            named = n[:1].isupper() and not re.match(KINDS, n, re.I)
            if not (named or re.match(FAMILY, n, re.I)):
                continue
            seen.add(r["id"])
            rows.append((r, "named" if named else "trash"))
    return rows


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--recipe", default=os.path.join(HERE, "worlds", "dungeons.json"))
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, timeout=30)
    if a.undo:
        undo(db)
        return
    for t in base.TEMPLATES:
        if not db.execute("SELECT 1 FROM classtemplates WHERE name = ?", (t,)).fetchone():
            sys.exit(f"classtemplates {t!r} is missing (no dungeons on this shard; tools/dungeons_20261112.py first): nothing done")
    if db.execute("SELECT 1 FROM sqlite_master WHERE name = ?", (LOG,)).fetchone() and \
            db.execute(f"SELECT 1 FROM {LOG} WHERE run = ? AND undone = '' LIMIT 1", (RUN,)).fetchone():
        print("applied already: nothing done")
        return
    world, dungeons = base.recipe_dungeons(a.recipe)
    d = next((d for d in dungeons if d["key"] == KEY), None)
    if d is None:
        sys.exit(f"the recipe {a.recipe} has no dungeon {KEY!r}: nothing done")
    last = ITEM_FIRST + len(ITEMS) - 1
    if db.execute("SELECT count(*) FROM itempattern WHERE patternid BETWEEN ? AND ?", (ITEM_FIRST, last)).fetchone()[0]:
        sys.exit(f"itempattern {ITEM_FIRST}-{last} is taken: nothing done")
    if db.execute("SELECT count(*) FROM loottables WHERE tablename LIKE ?", (f"dungeon {KEY} %",)).fetchone()[0]:
        sys.exit(f"loottables 'dungeon {KEY} ...' exist already: nothing done")
    if db.execute("SELECT 1 FROM gmtriggers WHERE slot = ? OR name = ?", (SLOT, TRIGGER)).fetchone():
        sys.exit(f"gmtriggers slot {SLOT} or {TRIGGER!r} is taken: nothing done")
    for b in d["boxes"]:
        n = db.execute("SELECT count(*) FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?",
                       (world, min(b[0], b[2]), max(b[0], b[2]), min(b[1], b[3]), max(b[1], b[3]))).fetchone()[0]
        if n:
            sys.exit(f"world {world} has {n} npcs rows in the shore's box {b} already: nothing done")
    bx = BOSS_TEMPLATE_AT
    if any(min(b[0], b[2]) <= bx[0] <= max(b[0], b[2]) and min(b[1], b[3]) <= bx[2] <= max(b[1], b[3]) for b in d["boxes"]):
        sys.exit(f"the boss template's spot {bx} is inside the box (every instance would stand him up): nothing done")
    boss = db.execute("SELECT id, npc_level FROM npcs WHERE world = ? AND npc_name = ?", (SOURCE, BOSS)).fetchall()
    if len(boss) != 1:
        sys.exit(f"world {SOURCE} has {len(boss)} rows named {BOSS!r}, want 1: nothing done")
    twin = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) < 1 AND abs(z_coord - ?) < 1",
                      (WARDEN, world, *TWIN_OF)).fetchone()
    if twin is None:
        sys.exit(f"no {WARDEN} twin at Blackburrow's landing {TWIN_OF} (tools/dungeons_landing_20261112.py first): nothing done")
    ensure_log(db)
    db.commit()
    now = base.now_s()
    t0 = time.time()
    cols = [c[1] for c in db.execute("PRAGMA table_info(npcs)") if c[1] != "id"]
    icols = [c[1] for c in db.execute("PRAGMA table_info(itempattern)")]
    summary = []
    db.execute("BEGIN")
    pid = ITEM_FIRST
    for tier, name, bid in ITEMS:
        b = base.rowd(db, "itempattern", "patternid = ?", (bid,))
        if b is None or len(name) > 32:
            sys.exit(f"base item {bid} ({name}) missing, or the name over 32: nothing done")
        v = dict(b)
        v["patternid"], v["itemname"], v["rent"] = pid, name, 1
        v["itemdesc"] = f"Raid gear of {d['name']}'s instance, as strong as {b['itemname']}."
        insert(db, now, "itempattern", {c: v[c] for c in icols})
        insert(db, now, "loottables", {"tablename": f"dungeon {KEY} {tier}", "patternid": pid, "weight": 1, "minlevel": 1,
                                       "maxlevel": 100, "source": RUN})
        summary.append(f"  item {pid}: {name} ({tier}; as {b['itemname']} {bid}, slot {b['equipslot']}, classes {b['classuse']})")
        pid += 1
    rows = select(db, d)
    lv = base.raid_levels(rows)
    tiers = {"trash": 0, "named": 0}
    for r, tier in rows:
        nid = copy_npc(db, cols, r["id"])
        db.execute("UPDATE npcs SET world = ?, npc_level = ?, npc_id = 0, dt_kill = NULL WHERE id = ?", (world, lv[r["id"]], nid))
        wrote(db, now, "npcs", {"id": nid})
        insert(db, now, "npctemplate", {"name": f"#{nid}", "template": f"dungeon {tier}",
                                        "note": f"{d['name']}'s {tier}: {r['npc_name']} (world {SOURCE} row {r['id']}, level {r['npc_level']} there)",
                                        "source": RUN})
        for suffix, chance, rolls in base.LOOT[tier]:
            insert(db, now, "npcloot", {"npc_name": f"#{nid}", "tablename": f"dungeon {KEY} {suffix}", "chance": chance,
                                        "rolls": rolls, "note": f"{d['name']}'s {tier} {r['npc_name']}", "source": RUN})
        tiers[tier] += 1
    # the boss: a template the trigger copies, standing outside the box (no instance's opening stands him up)
    boss_id, boss_level = boss[0]
    bid_ = copy_npc(db, cols, boss_id)
    db.execute("""UPDATE npcs SET world = ?, npc_level = 65, npc_id = 0, dt_kill = NULL, x_coord = ?, y_coord = ?, z_coord = ?
                  WHERE id = ?""", (world, bx[0], bx[1], bx[2], bid_))
    wrote(db, now, "npcs", {"id": bid_})
    insert(db, now, "npctemplate", {"name": f"#{bid_}", "template": "dungeon boss",
                                    "note": f"{d['name']}'s boss: {BOSS} (world {SOURCE} row {boss_id}, level {boss_level} there), raised by trigger {SLOT}",
                                    "source": RUN})
    for suffix, chance, rolls in base.LOOT["boss"]:
        insert(db, now, "npcloot", {"npc_name": f"#{bid_}", "tablename": f"dungeon {KEY} {suffix}", "chance": chance,
                                    "rolls": rolls, "note": f"{d['name']}'s boss {BOSS}", "source": RUN})
    # the trigger: Kratas rises at the bones in each party's copy
    insert(db, now, "gmtriggers", {"slot": SLOT, "name": TRIGGER, "enabled": 1, "scope": "near", "radius": 600,
                                   "world": world, "x": BONES[0], "y": BONES[1], "z": BONES[2],
                                   "steps": json.dumps(trigger_steps(bid_), separators=(",", ":")),
                                   "notes": ("Heirloom petition 3396 (the owner, 2026-11-28): Kratas's Deep, a heroic raid. A party "
                                             "near the whale bones in its own copy of the dungeon raises Kratas there (row "
                                             f"#{bid_}, the boss template outside the box); once dead he rises no more in that "
                                             "copy (the run waits out its life), a wipe (10 minutes empty) lets him rise again. "
                                             "tools/heroic_kratas_20261128.py")})
    named = sorted({r["npc_name"] for r, t in rows if t == "named"})
    kinds = {}
    for r, t in rows:
        if t == "trash":
            kinds[r["npc_name"]] = kinds.get(r["npc_name"], 0) + 1
    summary.insert(0, f"{d['name']}: {len(rows)} creatures ({tiers['trash']} trash L60-62, {tiers['named']} named L63-64) and "
                      f"{BOSS} (row {bid_}, L65, outside the box at {bx}); named: {', '.join(named)}")
    summary.insert(1, "  trash: " + ", ".join(f"{k} {v}" for k, v in sorted(kinds.items())))
    summary.append(f"  gmtriggers {SLOT}: {TRIGGER}, world {world} at the bones {BONES}")
    ex, ey, ez, ef = d["entrance"]
    h = ef * 2 * math.pi / 256
    wx, wz = ex + 4 * math.cos(h), ez - 4 * math.sin(h)
    nid = copy_npc(db, cols, twin[0])
    db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
               (round(wx, 3), ey, round(wz, 3), base.facing_byte(wx, wz, ex, ez), nid))
    wrote(db, now, "npcs", {"id": nid})
    summary.append(f"  npcs {nid}: {WARDEN} at {d['name']}'s entrance ({wx:.2f}, {ey}, {wz:.2f})")
    n = db.execute(f"SELECT count(*) FROM {LOG} WHERE run = ? AND undone = ''", (RUN,)).fetchone()[0]
    print("\n".join(summary))
    print(f"{len(ITEMS)} items {ITEM_FIRST}-{pid - 1}; {n} rows in {time.time() - t0:.1f} s")
    if a.apply:
        db.commit()
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
