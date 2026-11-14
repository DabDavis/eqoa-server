#!/usr/bin/env python3
"""The Salt Mines, an eleventh instanced raid in the dungeons' world (the owner, 2026-11-14: "lets add salt mines dungeon
to instanced raids. this is a deep dungeon with drachnids below").

The Siloth's salt mine east of Forkwatch (Tunaria zone 105, cell (7, 9); tools/worlds/dungeons.json's "saltmines"):
its mouth faces east at (14600, 54, 19045), behind the slavers' door guards; its shaft goes down through the Siloth's
floors (y 42, 31, 6, -23/-28: the slavers, knights, ritualists and a necromancer) to the drachnids' nest (y -57 to -73:
lurkers, spinners, gorgers, parasites, stalkers, the drachnid named and Spider Queen Aszec) and the crypt at its foot
(y -88: Mayong Mistmoore among his coffins). tools/dungeons_20261112.py made the first three raids and the three tier
templates, tools/dungeons_more_20261112.py seven more; this adds the Salt Mines as they were made:

  - npcs, world 14: a copy of each creature of the mine, where it stands on Tunaria: world 0's rows inside its box of
    its kinds (FAMILY: the Siloth, the drachnids, the glyphed familiar) or named (a capitalised name), never friendly
    (npc_type 0x80: Slavemaster Hern, Lionell Redrick, the coffins, the Circle of the Spider Queen), a quest's seeded
    row (Collin Ilbeth) or spawn-only; levels raised as the others' (trash 60-62 over the mine's 39-46: the Siloth 60,
    the drachnids 61-62; named 63-64; the bosses 65: BOSSES, the drachnids' queen and the vampire at the foot);
    npctemplate "#<row>" -> its tier, the Siloth knights' -> "dungeon trash shadowknight" where that copy of the trash
    tier stands (tools/petfix_dungeon_casters_20261113.py: a "knight" of npcclass 0 fights as a shadow knight, its
    life tap; the Siloth knights are class 0 as Guk's knights were), else "dungeon trash".
  - itempattern 995121-995132: twelve raid-gear records (ITEMS: a tuned raid boss's drop under a name of the mine's
    own; kept over logout, rent 1, as the others' are since petition 1837), loottables "dungeon saltmines boss|named",
    npcloot "#<row>" (a boss 2 + 1, a named 60%, trash 2%: the others' rates).
  - a Dungeon Warden twin at the entrance (4 to the landing's right, facing it), a copy of Blackburrow's.

Needs tools/dungeons_20261112.py applied (the tier templates and the twins): a shard without the dungeons (the Live
Server) is refused, nothing written. The server reads npcs, templates and loot rows at its start: run it in a keep
restart's before.sh, with the world file that has the Salt Mine (world 14's recipe of 2026-11-14) in worlds/. Refused
when applied already (a second run does nothing) or a row it would write exists; --undo deletes every row it wrote,
refused when one changed since (a GM's edit); exact. Report only by default; --apply in one transaction, a gm_audit
line a row, log dungeons_saltmines_1114_log.

    python3 -E -s tools/dungeons_saltmines_20261114.py --db eqoa.db [--apply | --undo] [--recipe tools/worlds/dungeons.json]
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

RUN, LOG = "dungeons saltmines 20261114", "dungeons_saltmines_1114_log"
DETAIL = "the owner, 2026-11-14: the Salt Mines, an instanced raid; tools/dungeons_saltmines_20261114.py"
KEY, SOURCE = "saltmines", 0
ITEM_FIRST = 995121
WARDEN = base.WARDEN
TWIN_OF = (12586.0, 12760.0)  # Blackburrow's twin (world 14), whose row the new twin copies

FAMILY = r"^(siloth|drachnid|glyphed familiar)"
# capitalised names that are kinds, not persons ("Siloth knight"); "Drachnid Body Snatcher" is a person
KINDS = r"^siloth "
BOSSES = ["Spider Queen Aszec", "Mayong Mistmoore"]
KNIGHTS = re.compile(r"\bknight\b", re.I)
KNIGHT_TIER = "dungeon trash shadowknight"
ITEMS = [
    ("boss", "Aszec's Chitin Breastplate", 6455),       # paladin's chest (Breastplate of the Valiant)
    ("boss", "Fang of the Spider Queen", 961101),       # Fang of Yhafiran
    ("boss", "Mistmoore's Crimson Tunic", 6477),        # necromancer's chest (Tunic of the decrepit)
    ("boss", "Silkspun Coil of Aszec", 961104),         # Coil of Yhafiran
    ("boss", "Signet of Mayong", 961106),               # Ring of the Sleeping Wyrm
    ("boss", "Broodwarden Bulwark", 961105),            # Wyrmscale Bulwark
    ("named", "Breastplate of the Siloth Knight", 6454),  # shadow knight's chest (breastplate of the reaver)
    ("named", "Siloth Slaver's Dirk", 5424),             # dagger of clarity
    ("named", "Ritualist's Salt-Stained Tunic", 6476),   # magician's chest (Tunic of the Creator)
    ("named", "Drachnid Silk Leggings", 713096),         # Trousers of the Ancient Dead
    ("named", "Saltminer's Lantern", 715014),            # Blind Man's Lantern (held)
    ("named", "Maestro's Blade of Rancor", 713079),      # Katana of Severing
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
                       (now, tbl, pk, after, "undo of tools/dungeons_saltmines_20261114.py"))
            db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (now, seq))
        for t in ("npcs", "loottables"):
            if db.execute(f"SELECT MAX(id) FROM {t}").fetchone()[0] is None:
                db.execute("DELETE FROM sqlite_sequence WHERE name = ?", (t,))
            else:
                db.execute(f"UPDATE sqlite_sequence SET seq = (SELECT MAX(id) FROM {t}) WHERE name = ?", (t,))
    print(f"undone: {len(rows)} rows")


def select(db, d):
    """World 0's rows of the mine, each with its tier."""
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
            if r["id"] in seen or r["npc_type"] & 0x80 or n.lower() in spawnonly or f"#{r['id']}" in spawnonly or \
                    r["id"] in seeded:
                continue
            named = n[:1].isupper() and not re.match(KINDS, n, re.I)
            if not (named or re.match(FAMILY, n, re.I)):
                continue
            seen.add(r["id"])
            rows.append((r, "boss" if n in BOSSES else "named" if named else "trash"))
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
    for b in d["boxes"]:
        n = db.execute("SELECT count(*) FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?",
                       (world, min(b[0], b[2]), max(b[0], b[2]), min(b[1], b[3]), max(b[1], b[3]))).fetchone()[0]
        if n:
            sys.exit(f"world {world} has {n} npcs rows in the mine's box {b} already: nothing done")
    twin = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) < 1 AND abs(z_coord - ?) < 1",
                      (WARDEN, world, *TWIN_OF)).fetchone()
    if twin is None:
        sys.exit(f"no {WARDEN} twin at Blackburrow's landing {TWIN_OF} (tools/dungeons_landing_20261112.py first): nothing done")
    knight_tier = KNIGHT_TIER if db.execute("SELECT 1 FROM classtemplates WHERE name = ?", (KNIGHT_TIER,)).fetchone() else None
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
        v["itemdesc"] = f"Raid gear of the {d['name']}' instance, as strong as {b['itemname']}."
        insert(db, now, "itempattern", {c: v[c] for c in icols})
        insert(db, now, "loottables", {"tablename": f"dungeon {KEY} {tier}", "patternid": pid, "weight": 1, "minlevel": 1,
                                       "maxlevel": 100, "source": RUN})
        summary.append(f"  item {pid}: {name} ({tier}; as {b['itemname']} {bid}, slot {b['equipslot']}, classes {b['classuse']})")
        pid += 1
    rows = select(db, d)
    lv = base.raid_levels(rows)
    tiers = {"trash": 0, "named": 0, "boss": 0}
    knights = 0
    for r, tier in rows:
        nid = copy_npc(db, cols, r["id"])
        db.execute("UPDATE npcs SET world = ?, npc_level = ?, npc_id = 0, dt_kill = NULL WHERE id = ?", (world, lv[r["id"]], nid))
        wrote(db, now, "npcs", {"id": nid})
        template = f"dungeon {tier}"
        if tier == "trash" and knight_tier and KNIGHTS.search(r["npc_name"]):
            template = knight_tier
            knights += 1
        insert(db, now, "npctemplate", {"name": f"#{nid}", "template": template,
                                        "note": f"the {d['name']}' {tier}: {r['npc_name']} (world {SOURCE} row {r['id']}, level {r['npc_level']} there)",
                                        "source": RUN})
        for suffix, chance, rolls in base.LOOT[tier]:
            insert(db, now, "npcloot", {"npc_name": f"#{nid}", "tablename": f"dungeon {KEY} {suffix}", "chance": chance,
                                        "rolls": rolls, "note": f"the {d['name']}' {tier} {r['npc_name']}", "source": RUN})
        tiers[tier] += 1
    bosses = sorted({r["npc_name"] for r, t in rows if t == "boss"})
    if bosses != sorted(BOSSES):
        sys.exit(f"{d['name']}: bosses found {bosses}, expected {BOSSES}: nothing done")
    named = sorted({r["npc_name"] for r, t in rows if t == "named"})
    kinds = {}
    for r, t in rows:
        if t == "trash":
            kinds[r["npc_name"]] = kinds.get(r["npc_name"], 0) + 1
    summary.insert(0, f"{d['name']}: {len(rows)} creatures ({tiers['trash']} trash L60-62, {tiers['named']} named L63-64, "
                      f"{tiers['boss']} bosses L65: {', '.join(bosses)}); named: {', '.join(named)}")
    summary.insert(1, "  trash: " + ", ".join(f"{k} {v}" for k, v in sorted(kinds.items())) +
                   (f"; {knights} knights by {knight_tier!r}" if knight_tier else "; no shadow knight tier here: knights by 'dungeon trash'"))
    ex, ey, ez, ef = d["entrance"]
    h = ef * 2 * math.pi / 256
    wx, wz = ex + 4 * math.cos(h), ez - 4 * math.sin(h)
    nid = copy_npc(db, cols, twin[0])
    db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
               (round(wx, 3), ey, round(wz, 3), base.facing_byte(wx, wz, ex, ez), nid))
    wrote(db, now, "npcs", {"id": nid})
    summary.append(f"  npcs {nid}: {WARDEN} at the {d['name']}' entrance ({wx:.2f}, {ey}, {wz:.2f})")
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
