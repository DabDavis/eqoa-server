#!/usr/bin/env python3
"""The instanced raid dungeons' data (handlers/dungeons.go, entity/dungeons.go; claude-notes/dungeon-world-20261112.md;
the owner, 2026-11-12: "lets add a dungeon world. instanced versions of popular dungeons like Guk, Blackburrow,
Runnyeye. they have the same mobs, but will be tuned for level 60 raids and raid gear").

What it writes, every row a gm_audit line and a row of dungeons_1112_log (the undo's record):

  - npcs, world 14 (the dungeons' world, the recipe's "world"): a copy of each creature of each dungeon, where it stands
    on Tunaria (the composed world keeps the disc's coordinates): the world 0 rows inside the dungeon's boxes (the
    recipe's "dungeons" block) of its kinds (FAMILY) or named (a capitalised name), none friendly (npc_type 0x80), none
    a quest's (quest_npc_seed_rows, npcspawnonly), none of SKIP. Its body, gear, size, facing, zone column as the
    original's; npc_id 0; its LEVEL raised to a raid's: trash 60-62 (the dungeon's trash levels spread over them),
    named 63-64 (the dungeon's upper half of its named 64), bosses (BOSSES) 65. These rows are the dungeons' templates:
    the server holds them out of the world and copies them into each instance opened.
  - classtemplates "dungeon trash", "dungeon named", "dungeon boss" (TEMPLATES; class -1: each its own) and npctemplate
    "#<row>" -> its tier, so a GM tunes a tier in one row (or one creature by its own "#row").
  - itempattern 995001-: raid gear of each dungeon (ITEMS): a copy of a raid boss's drop (the bases of the Ascended
    records the GMs' tuned bosses drop: Nagafen, Noroxus, Bertoxxulous, the Quintessence, Yhafiran) with every figure,
    look, slot, classes, level, NO TRADE and Lore of its base, under a name of the dungeon's own and a description.
  - loottables "dungeon <key> boss" and "dungeon <key> named", and npcloot "#<row>" (a dungeon creature rolls its row's
    tables alone, handlers/loot.go's lootNames): a boss two of its boss table and one of the named table; a named 60%
    one of the named table; trash 2% one of the named table.
  - the Dungeon Warden: one beside Dabrish (Freeport; the first free spot round him) and its twin at each dungeon's
    entrance in world 14 (4 to the side of where a player lands, facing it: it stands in every instance, the way
    out), in the Teleporter's look; npcmove stands, npcaggro never.

The server reads npcs, templates and loot rows at its start (loot tables at each corpse): run it in a keep restart's
before.sh with world.dungeons on. Refused when applied already (a second run does nothing) or a row it would write
exists; --undo deletes every row it wrote, refused when one changed since (a GM's edit); exact.

    python3 -E -s tools/dungeons_20261112.py --db eqoa.db [--apply | --undo] [--recipe tools/worlds/dungeons.json]
"""
import argparse
import json
import math
import os
import re
import sqlite3
import sys
import time

RUN = "dungeons 20261112"
LOG = "dungeons_1112_log"
HERE = os.path.dirname(os.path.abspath(__file__))
WARDEN, BESIDE, LOOK = "Dungeon Warden", "Dabrish", "Teleporter"
ROOM, PORTAL_ROOM = 4.0, 6.0
SPOTS = [(-6, 14), (-6, -16), (-10, 14), (-10, -16), (14, 8), (14, -8), (0, 22), (0, -24), (-14, 0), (18, 0)]
ITEM_FIRST = 995001

# each dungeon's creatures, by name (a capitalised name is a named whatever its kind)
FAMILY = {
    "guk": r"^(froglok|frogluk|fung|funagl|ghoul|mumm|crypt mummy|swarming bones|.*spirit)",
    "blackburrow": r"^(gnoll|.*spirit)",
    "runnyeye": r"^(goblin|royal guard goblin|surly goblin|filthbat|decaying protector|enthralled|animation|.*spirit)",
}
# not a dungeon's own though they stand in it: Grobb's trolls camped at Guk's gate, a dwarf held in the burrow's pit
SKIP = {"basher lesheg", "basher kwag", "dagel copperbeard"}
# names counted as trash though capitalised (a kind, not a person)
KINDS = {"companion spirit", "feral spirit", "kindred spirit", "mummified guardian", "swarming bones", "animation"}
BOSSES = {"guk": ["froglok king"], "blackburrow": ["Abyssaxis"], "runnyeye": ["King Merdid", "Xorbb"]}
TEMPLATES = {
    # name: (hp_pct, melee_pct, special_damage (a crit's share), resist_pct (every kind: a resist_pct needs its kinds, or
# the server leaves the template unused), abilities, note)
    # A first pull a 4-6 player level 60 raid in raid gear takes (the owner, 2026-11-12: lower the first numbers, which
    # had killed two level 60 GMs in seconds; those deaths were in fact of creatures with NO template, the first rows
    # lacking a resist_kind: naked clones of 1,243 hit points hit for ~2,000). Measured with cmd/balancesim (tier 0.5
    # gear, groups of 2-3): trash 10-12 s for three players at 97-99% hit points left; a named 98 s for two (78% left);
    # the boss won by three in 60% of fights (1.7 dead a fight), never by two. Jibrish tunes from here.
    "dungeon trash": (100, 130, 7.4, 10, [],
                      "The instanced dungeons' creatures (tools/dungeons_20261112.py): levels 60-62, about 15k hit points, "
                      "130% melee, the roamer's crit, 10% resists. A first pull for a 4-6 player L60 raid in raid gear (balancesim: "
                      "three players kill one in 10-12 s); ours: tune freely (this row is every dungeon's trash)."),
    "dungeon named": (250, 200, 10.0, 20, [],
                      "The instanced dungeons' named (tools/dungeons_20261112.py): levels 63-64, about 40k hit points, 200% melee, "
                      "a crit, 20% resists (balancesim: two players win in 98 s). Ours: tune freely."),
    "dungeon boss": (500, 150, 15.0, 30, [{"kind": "aoe", "spell_id": 905023, "every": 30, "first": 15, "damage_pct": 25}],
                     "The instanced dungeons' bosses (tools/dungeons_20261112.py): level 65, about 84k hit points, 150% melee, "
                     "a crit, 30% resists, Pestilent Breath at a quarter every 30 s (balancesim: three players win 60%, two never; "
                     "a raid's fight). Ours: tune freely."),
}
# (dungeon, table, name, base patternid): each a raid boss's drop copied under the dungeon's name
ITEMS = [
    ("guk", "boss", "Royal Breastplate of Guk", 716001),
    ("guk", "boss", "Swampsilk Tunic of Guk", 6479),
    ("guk", "boss", "Froglok King's Scepter", 5397),
    ("guk", "boss", "Ring of the Froglok King", 961106),
    ("guk", "boss", "Mirewalker Robe", 5072),
    ("guk", "boss", "Bog Lord's Bulwark", 961105),
    ("guk", "named", "Ghoulbane", 5417),
    ("guk", "named", "Tunic of the Sunken Halls", 6477),
    ("guk", "named", "Fungal Spore Tunic", 6478),
    ("guk", "named", "Froglok Hide Leggings", 713096),
    ("guk", "named", "Guk Swampwater Belt", 715012),
    ("guk", "named", "Eye of the Sunken Halls", 961103),
    ("blackburrow", "boss", "Abyssal Breastplate", 6455),
    ("blackburrow", "boss", "Abyssaxis' Fang", 961101),
    ("blackburrow", "boss", "Gnollhide Chain of the Pack", 6456),
    ("blackburrow", "boss", "Abyssal Coil", 961104),
    ("blackburrow", "boss", "Tunic of the Pack Lord", 6480),
    ("blackburrow", "boss", "Burrowstone Ring", 713087),
    ("blackburrow", "named", "Splitpaw Halberd", 713073),
    ("blackburrow", "named", "Blackpaw Dagger", 5424),
    ("blackburrow", "named", "Tunic of the Burrow Shaman", 6476),
    ("blackburrow", "named", "Pack Master's Breastplate", 6454),
    ("blackburrow", "named", "Blackburrow Lantern", 715014),
    ("blackburrow", "named", "Gnollskin Tunic", 6472),
    ("runnyeye", "boss", "Breastplate of the Goblin King", 6453),
    ("runnyeye", "boss", "Xorbb's Gaze", 715015),
    ("runnyeye", "boss", "Evil Eye Tunic", 6474),
    ("runnyeye", "boss", "Runnyeye Smasher", 5418),
    ("runnyeye", "boss", "Goblin King's Bow", 715011),
    ("runnyeye", "boss", "Shield of the Warren", 715013),
    ("runnyeye", "named", "Tunic of the Runnyeye Seer", 6475),
    ("runnyeye", "named", "Tunic of the Goblin Mage", 716005),
    ("runnyeye", "named", "Sword of the Warren", 5406),
    ("runnyeye", "named", "Katana of the Evil Eye", 713079),
    ("runnyeye", "named", "Goblin Miner's Belt", 715012),
    ("runnyeye", "named", "Goblin Slayer's Blade", 1060),
]
# npcloot by tier: (table suffix, chance, rolls)
LOOT = {
    "boss": [("boss", 100, 2), ("named", 100, 1)],
    "named": [("named", 60, 1)],
    "trash": [("named", 2, 1)],
}


def now_s():
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def rowd(db, tbl, where, args):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


PKS = {"npcs": ["id"], "classtemplates": ["name"], "npctemplate": ["name"], "itempattern": ["patternid"],
       "loottables": ["id"], "npcloot": ["npc_name", "tablename"], "npcmove": ["name"], "npcaggro": ["npc_name"]}


def wrote(db, now, tbl, key):
    """Log and audit a row just inserted (key: its primary key's columns)."""
    where = " AND ".join(f"{k} = ?" for k in key)
    after = rowd(db, tbl, where, list(key.values()))
    db.execute(f"INSERT INTO {LOG} (run, tbl, pk, after) VALUES (?, ?, ?, ?)",
               (RUN, tbl, json.dumps(key, sort_keys=True), json.dumps(after, sort_keys=True)))
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', 'insert', ?, ?, NULL, ?, ?)""",
               (now, tbl, json.dumps(key), json.dumps(after, sort_keys=True),
                "the owner, 2026-11-12: the instanced raid dungeons; tools/dungeons_20261112.py"))
    return after


def insert(db, now, tbl, vals):
    cols = list(vals)
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [vals[c] for c in cols])
    key = {k: (cur.lastrowid if k in ("id",) and k not in vals else vals[k]) for k in PKS[tbl]}
    return wrote(db, now, tbl, key)


def facing_byte(x, z, tx, tz):
    return int(round(math.degrees(math.atan2(tx - x, tz - z)) % 360 / 360 * 256)) % 256


def recipe_dungeons(path):
    j = json.load(open(path))
    if not j.get("dungeons"):
        sys.exit(f"{path} has no dungeons block: nothing done")
    out = []
    for d in j["dungeons"]:
        e = d["entrance"]
        f = e.get("facing")
        fb = facing_byte(e["x"], e["z"], *e["toward"]) if f is None else int(round(f % 360 / 360 * 256)) % 256
        out.append(dict(key=d["key"], name=d["name"], boxes=d["boxes"], entrance=(e["x"], e["y"], e["z"], fb)))
    return j["world"], out


def select(db, d):
    """The world 0 rows of the dungeon d, each with its tier."""
    spawnonly = {r[0].lower() for r in db.execute("SELECT name FROM npcspawnonly")}
    seeded = {r[0] for r in db.execute("SELECT npc_row FROM quest_npc_seed_rows")}
    seen, rows = set(), []
    for b in d["boxes"]:
        x0, z0, x1, z1 = min(b[0], b[2]), min(b[1], b[3]), max(b[0], b[2]), max(b[1], b[3])
        cur = db.execute("""SELECT * FROM npcs WHERE world = 0 AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?
                            ORDER BY id""", (x0, x1, z0, z1))
        cols = [c[0] for c in cur.description]
        for r in cur.fetchall():
            r = dict(zip(cols, r))
            n = r["npc_name"]
            if r["id"] in seen or r["npc_type"] & 0x80 or n.lower() in spawnonly or f"#{r['id']}" in spawnonly or \
                    r["id"] in seeded or n.lower() in SKIP:
                continue
            named = n[:1].isupper() and n.lower() not in KINDS
            if not (named or re.match(FAMILY[d["key"]], n, re.I)):
                continue
            seen.add(r["id"])
            boss = n in BOSSES[d["key"]]
            rows.append((r, "boss" if boss else "named" if named else "trash"))
    return rows


def raid_levels(rows):
    """Each row's raid level: trash 60-62 over the dungeon's trash levels, named 63 or 64 (its upper half), bosses 65."""
    trash = [r["npc_level"] for r, t in rows if t == "trash"] or [0]
    lo, hi = min(trash), max(trash)
    named = sorted(r["npc_level"] for r, t in rows if t == "named") or [0]
    mid = named[len(named) // 2]
    out = {}
    for r, t in rows:
        if t == "boss":
            out[r["id"]] = 65
        elif t == "named":
            out[r["id"]] = 64 if r["npc_level"] >= mid else 63
        else:
            out[r["id"]] = 60 + (0 if hi == lo else round(2 * (r["npc_level"] - lo) / (hi - lo)))
    return out


def ensure_log(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY AUTOINCREMENT, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   pk TEXT NOT NULL, after TEXT, at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
                   undone TEXT NOT NULL DEFAULT '')""")


def undo(db):
    ensure_log(db)
    rows = db.execute(f"SELECT seq, tbl, pk, after FROM {LOG} WHERE run = ? AND undone = '' ORDER BY seq DESC", (RUN,)).fetchall()
    if not rows:
        sys.exit("nothing applied: nothing done")
    for seq, tbl, pk, after in rows:
        key = json.loads(pk)
        cur = rowd(db, tbl, " AND ".join(f"{k} = ?" for k in key), list(key.values()))
        if cur is None or json.dumps(cur, sort_keys=True) != after:
            sys.exit(f"{tbl} {pk} changed since (a GM's edit?): nothing done")
    now = now_s()
    with db:
        for seq, tbl, pk, after in rows:
            key = json.loads(pk)
            db.execute(f"DELETE FROM {tbl} WHERE " + " AND ".join(f"{k} = ?" for k in key), list(key.values()))
            db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                          VALUES (?, 1, 'dab', 'tool', '', 'delete', ?, ?, ?, NULL, ?)""",
                       (now, tbl, pk, after, "undo of tools/dungeons_20261112.py"))
            db.execute(f"UPDATE {LOG} SET undone = ? WHERE seq = ?", (now, seq))
        for t in ("npcs", "loottables"):
            db.execute(f"UPDATE sqlite_sequence SET seq = (SELECT COALESCE(MAX(id), 0) FROM {t}) WHERE name = ?", (t,))
    print(f"undone: {len(rows)} rows")


def warden_spot(db, d):
    h = d["facing"] * 2 * math.pi / 256
    fx, fz, rx, rz = math.sin(h), math.cos(h), math.cos(h), -math.sin(h)
    px, pz = d["x_coord"] + 3 * fx, d["z_coord"] + 3 * fz
    for front, side in SPOTS:
        x, z = d["x_coord"] + front * fx + side * rx, d["z_coord"] + front * fz + side * rz
        near = db.execute("SELECT npc_name, x_coord, z_coord FROM npcs WHERE world = ? AND abs(x_coord - ?) < 10 AND abs(z_coord - ?) < 10",
                          (d["world"], x, z)).fetchall()
        if any(math.hypot(nx - x, nz - z) < ROOM for _, nx, nz in near) or math.hypot(px - x, pz - z) < PORTAL_ROOM:
            continue
        return x, d["y_coord"], z, front, side
    sys.exit("no spot round Dabrish is free: nothing done")


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
    ensure_log(db)
    if db.execute(f"SELECT 1 FROM {LOG} WHERE run = ? AND undone = '' LIMIT 1", (RUN,)).fetchone():
        print("applied already: nothing done")
        return
    world, dungeons = recipe_dungeons(a.recipe)
    if db.execute("SELECT count(*) FROM npcs WHERE world = ?", (world,)).fetchone()[0]:
        sys.exit(f"world {world} has npcs rows already: nothing done")
    if db.execute("SELECT count(*) FROM npcs WHERE lower(npc_name) = ?", (WARDEN.lower(),)).fetchone()[0]:
        sys.exit("a Dungeon Warden stands already: nothing done")
    for t in TEMPLATES:
        if db.execute("SELECT 1 FROM classtemplates WHERE name = ?", (t,)).fetchone():
            sys.exit(f"classtemplates {t!r} exists already: nothing done")
    if db.execute("SELECT count(*) FROM itempattern WHERE patternid BETWEEN ? AND ?", (ITEM_FIRST, ITEM_FIRST + 998)).fetchone()[0]:
        sys.exit(f"itempattern {ITEM_FIRST}- is taken: nothing done")
    if db.execute("SELECT count(*) FROM loottables WHERE tablename LIKE 'dungeon %'").fetchone()[0]:
        sys.exit("loottables 'dungeon ...' exist already: nothing done")
    for n in (WARDEN.lower(),):
        if db.execute("SELECT 1 FROM npcmove WHERE name = ?", (n,)).fetchone() or \
                db.execute("SELECT 1 FROM npcaggro WHERE lower(npc_name) = ?", (n,)).fetchone():
            sys.exit("the warden has an npcmove or npcaggro row already: nothing done")
    ds = db.execute("SELECT id FROM npcs WHERE npc_name = ? ORDER BY id", (BESIDE,)).fetchall()
    look = rowd(db, "npcs", "npc_name = ? AND world = 0 ORDER BY id", (LOOK,))
    if len(ds) != 1 or look is None:
        sys.exit(f"{len(ds)} rows named {BESIDE}, the {LOOK} {'found' if look else 'missing'}: nothing done")
    dab = rowd(db, "npcs", "id = ?", (ds[0][0],))
    now = now_s()
    t0 = time.time()
    cols = [c[1] for c in db.execute("PRAGMA table_info(npcs)") if c[1] != "id"]
    summary = []
    db.execute("BEGIN")
    # the templates
    for name, (hp, melee, crit, resist, abilities, note) in TEMPLATES.items():
        insert(db, now, "classtemplates", {"name": name, "class": -1, "hp_pct": hp, "taken_pct": 100, "melee_pct": melee,
                                           "special": "Critical Hit", "special_chance": 0, "special_damage": crit,
                                           "special_every": 30.0, "special_first": 0.0, "special_jitter": 5.0,
                                           "resist_pct": resist, "resist_kind": "FR,CR,LR,AR,PR,DR", "abilities": json.dumps(abilities), "note": note,
                                           "source": RUN})
    # the items and their tables
    pid = ITEM_FIRST
    icols = [c[1] for c in db.execute("PRAGMA table_info(itempattern)")]
    for key, tier, name, base in ITEMS:
        b = rowd(db, "itempattern", "patternid = ?", (base,))
        if b is None:
            sys.exit(f"base item {base} ({name}) is missing: nothing done")
        if len(name) > 32:
            sys.exit(f"{name!r} is longer than 32: nothing done")
        dname = next(d["name"] for d in dungeons if d["key"] == key)
        v = dict(b)
        v["patternid"], v["itemname"] = pid, name
        v["rent"] = 1  # kept over logout: some bases are No Rent (petition 1837, the Guk Swampwater Belt)
        v["itemdesc"] = f"Raid gear of {dname}'s instance, as strong as {b['itemname']}."
        insert(db, now, "itempattern", {c: v[c] for c in icols})
        insert(db, now, "loottables", {"tablename": f"dungeon {key} {tier}", "patternid": pid, "weight": 1, "minlevel": 1,
                                       "maxlevel": 100, "source": RUN})
        pid += 1
    # the creatures
    for d in dungeons:
        rows = select(db, d)
        lv = raid_levels(rows)
        tiers = {"trash": 0, "named": 0, "boss": 0}
        for r, tier in rows:
            db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (r["id"],))
            nid = db.execute("SELECT last_insert_rowid()").fetchone()[0]
            db.execute("UPDATE npcs SET world = ?, npc_level = ?, npc_id = 0, dt_kill = NULL WHERE id = ?", (world, lv[r["id"]], nid))
            wrote(db, now, "npcs", {"id": nid})
            insert(db, now, "npctemplate", {"name": f"#{nid}", "template": f"dungeon {tier}",
                                            "note": f"{d['name']}'s {tier}: {r['npc_name']} (world 0 row {r['id']}, level {r['npc_level']} there)",
                                            "source": RUN})
            for suffix, chance, rolls in LOOT[tier]:
                insert(db, now, "npcloot", {"npc_name": f"#{nid}", "tablename": f"dungeon {d['key']} {suffix}", "chance": chance,
                                            "rolls": rolls, "note": f"{d['name']}'s {tier} {r['npc_name']}", "source": RUN})
            tiers[tier] += 1
        bosses = sorted({r["npc_name"] for r, t in rows if t == "boss"})
        if sorted(bosses) != sorted(BOSSES[d["key"]]):
            sys.exit(f"{d['name']}: bosses found {bosses}, expected {BOSSES[d['key']]}: nothing done")
        summary.append(f"{d['name']}: {len(rows)} creatures ({tiers['trash']} trash L60-62, {tiers['named']} named L63-64, "
                       f"{tiers['boss']} bosses L65: {', '.join(bosses)})")
    # the wardens: beside Dabrish, and at each entrance (4 to the right of the landing, facing it)
    x, y, z, front, side = warden_spot(db, dab)
    spots = [(dab["world"], x, y, z, dab["facing"], dab["zone"], "Freeport, beside Dabrish")]
    for d in dungeons:
        ex, ey, ez, ef = d["entrance"]
        h = ef * 2 * math.pi / 256
        wx, wz = ex + 4 * math.cos(h), ez - 4 * math.sin(h)
        spots.append((world, wx, ey, wz, facing_byte(wx, wz, ex, ez), 0, f"{d['name']}'s entrance"))
    for (w, px, py, pz, pf, zone, where) in spots:
        db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (look["id"],))
        nid = db.execute("SELECT last_insert_rowid()").fetchone()[0]
        db.execute("""UPDATE npcs SET npc_name = ?, zone = ?, world = ?, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?,
                      npc_id = 0 WHERE id = ?""", (WARDEN, zone, w, round(px, 3), py, round(pz, 3), pf, nid))
        wrote(db, now, "npcs", {"id": nid})
        summary.append(f"npcs {nid}: {WARDEN} at {where}, world {w} ({px:.2f}, {py}, {pz:.2f}) facing {pf}")
    insert(db, now, "npcmove", {"name": WARDEN.lower(), "roam": 0, "radius": 0, "water": "",
                                "note": "stands (the Dungeon Warden)", "source": RUN})
    insert(db, now, "npcaggro", {"npc_name": WARDEN, "aggressive": 0, "radius": 0,
                                 "note": "the Dungeon Warden: talks only", "source": RUN})
    n = db.execute(f"SELECT count(*) FROM {LOG} WHERE run = ? AND undone = ''", (RUN,)).fetchone()[0]
    print("\n".join(summary))
    print(f"{len(ITEMS)} items {ITEM_FIRST}-{pid - 1}, {len(TEMPLATES)} templates; {n} rows in {time.time() - t0:.1f} s")
    if a.apply:
        db.commit()
        print("applied")
    else:
        db.rollback()
        print("(report only: --apply writes it)")


if __name__ == "__main__":
    main()
