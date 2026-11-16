#!/usr/bin/env python3
"""The Toy Shoppe by Freeport's Christmas tree (the owner, 2026-11-16: "add Toy Shoppe close to the tree by Weapon
Enchanter. The NPC will sell our toys like barrel of monkeys, snowgun, etc. We will make more like Santa's Elf illusion
the gnome with red/green clothing etc."). A server with handlers/tokenshop.go and entity/outfit.go (branch toy-shoppe).

1. The toys, NEW records bound to no one (no itemowner row), so each works for whoever buys it (the server finds the
   Barrel and the Launcher by NAME: handlers/monkeybarrel.go, snowlauncher.go; a record of the name with no binding works
   for whoever holds it). Worn nowhere, CLIENT's usable type (procanim 2: the item menu offers Use, a potion quick slot
   takes it), LORE (one), NO TRADE (trade 1), kept over logout, worth nothing, never spent, any class, race and level:
     - TOYS[0] Barrel of Monkeys (Moose's 715080 is bound to him): a monkey a Use, up to five, all home at the sixth;
     - TOYS[1] Automatic Snowball Launcher (715090-715092 are Zerkziee's, Jibtank's and Dust's): automatic fire on/off;
     - TOYS[2] Santa's Elf, an ILLUSION TOY: its use (table itemeffect: "Self, Unlimited Uses, 30min", body the gnome man
       0xA99C6B15, a woman gets the gnome woman, cooldown COOLDOWN seconds) shows its user as a gnome for 30 minutes, in
       its outfit (table itemoutfit, entity/outfit.go: the Holiday Elf's red and green, tools/christmas_elf_santa_20261116.py's
       ELF_LOOK and DYES: cloth, leather boots, a cap; a PS2 draws each colour as its nearest disc tint, red 8, green 5,
       white 0, no trim); the item used again while it is on takes it off (and vanilla's buff window clicks it off).
   More illusion toys go the same way: a TOYS entry with an "illusion" (body, line, icon, cooldown, outfit).
2. The shop: table tokenshop (database/tokenshop.go), a row a toy for SHOP, priced in Holly Tokens (token '': the
   season's); the prices are TOYS' (an evening of play earns about 18: seasonrewards' note). A GM changes a price or adds
   a ware in the panel (Token shops' stock).
3. The shopkeeper, SHOP: a new npcs row (a copy of Captain Scilond 405's, as the Holiday Elf is) at SHOP_AT, east of the
   tree by its Weapon Enchanter (Heirloom's 190637), north of the presents spilling east, a gnome man (an old toymaker:
   white hair, leather in brown, tint 3), friendly and talked to (npc_type 128), standing (npcmove), never attacking
   (npcaggro), of the Christmas season (npcseason: gone with the tree, back with it). Refused when another NPC's row
   stands within ROOM of the spot, or the tree's is within TREE_CLEAR (its boughs and presents). Where the server has
   table npcdyes (branch christmas4), the shopkeeper is dyed too (vanilla: a red coat with a gold trim, green
   leggings); without it nothing of that is written (run the tool again after it lands: it adds what is missing).

The records' ids are fixed (TOYS' ids, free on both shards on 2026-11-16) so the shards agree; an id taken by another
record refuses everything. Report by default; --apply in one transaction, a gm_audit line a row (as the panel's), logged
in toy_shoppe_1116_log; a second --apply adds only what is missing; --undo puts back exactly what it wrote (a row changed
since is left and listed; a toy's record, its use and its outfit are left while anyone still holds a copy). The server
reads npcs, npcseason and itemoutfit at its start: run it in a keep restart's before.sh, on each shard's database.

    python3 -E -s tools/toy_shoppe_20261116.py --db eqoa.db [--apply | --undo]
"""
import argparse
import datetime
import json
import math
import sqlite3
import sys

LOG = "toy_shoppe_1116_log"
SRC = "toy-shoppe"
DETAIL = "the Toy Shoppe by Freeport's Christmas tree (the owner, 2026-11-16); tools/toy_shoppe_20261116.py"
WHO, VIA = "dab", "tool"
SEASON = "Christmas"
TOKEN = "Holly Token"

SHOP = "Toymaker Tinsel"
TEMPLATE = (405, "Captain Scilond")
TREE = "Christmas Tree"
# east of the tree (25270, 15754.7), 15.7 from its middle; 4.1 north-west of its Weapon Enchanter (Heirloom 190637 at
# 25284.76, 15747.96), 5.2 north of the nearest present spilling east (25280.5, 15749.7); facing byte 96, north-east,
# out to the square (the tree at his back)
SHOP_AT = (25282.2, 54.13, 15744.8, 96)
ROOM, TREE_CLEAR = 2.5, 11.0
GNOME_MALE = 0xA99C6B15
SHOP_LOOK = {"modelid": GNOME_MALE, "race": "3", "torso": 2, "forearm": 2, "gloves": 2, "legs": 2, "feet": 2, "head": 0,
             "robe_style": "255", "hair_color": 3, "hair_length": 2, "hair_style": 1, "face": 2, "unk_o": 3, "size": 1.0,
             "primary_weapon": 0, "secondary_weapon": 0, "shield": 0, "npc_type": 128, "npc_level": 30}
RED, GREEN, WHITE, GOLD, BROWN = 0xC81E1EFF, 0x1E8C3CFF, 0xF4F4F4FF, 0xE0B030FF, 0x6B4423FF
SHOP_DYES = {0: RED, 11: GOLD, 1: BROWN, 2: BROWN, 3: GREEN, 4: BROWN}  # npcdyes (christmas4): chardyes' slots

USABLE = 2  # CLIENT's item type of the potions and food (database.ItemTypeUsable)
DESC_MAX = 200
COOLDOWN = 60  # seconds before Santa's Elf may be used again (taking it off spends none)

# the Holiday Elf's look (tools/christmas_elf_santa_20261116.py, branch christmas4): cloth (set 1), leather boots (2),
# a cloth cap; slot: (set, colour) as chardyes numbers them: 0 chest .. 5 helm, 9 robe, 11-16 the trims
ELF_OUTFIT = {0: (1, RED), 11: (0, WHITE), 1: (1, GREEN), 2: (1, WHITE), 3: (1, GREEN), 4: (2, RED), 5: (1, GREEN),
              16: (0, RED)}

TOYS = [
    {"id": 716910, "name": "Barrel of Monkeys", "price": 36, "sort": 2, "icon": -1324486342,
     "desc": "A battered barrel that chatters and squeaks. Use it and a monkey climbs out to follow you, up to five; "
             "use it again with all five out and they scramble home.",
     "says": "A barrel of monkeys! Use it and a monkey climbs out to follow you, up to five of them. Use it again with "
             "all five out and they scramble home."},
    {"id": 716911, "name": "Automatic Snowball Launcher", "price": 54, "sort": 3, "icon": 544383080,
     "desc": "A gnomish contraption of springs, cogs and a bottomless hopper of snow. Use it to switch automatic fire on "
             "or off: it pelts your target. Never runs dry.",
     "says": "My finest work: springs, cogs and a hopper of snow that never runs dry. Use it to switch automatic fire on "
             "or off, and it pelts whoever you target."},
    {"id": 716912, "name": "Santa's Elf", "price": 18, "sort": 1, "icon": -1038043378,  # the Child's Doll's
     "desc": "A little felt elf in red and green. Use it to become one of Santa's gnome elves for 30 minutes; use it "
             "again to be yourself.",
     "says": "Become one of Santa's elves: a gnome in red and green for half an hour. Use it again to be yourself. It "
             "never wears out.",
     "illusion": {"line": "Self, Unlimited Uses, 30min", "name": "Santa's Elf", "icon": 1507275072, "body": GNOME_MALE,
                  "cooldown": COOLDOWN, "outfit": ELF_OUTFIT}},
]

# database/tokenshop.go's schemas, word for word (tools/toy_shoppe_20261116_test.py holds them to it)
TOKENSHOP_DDL = """CREATE TABLE IF NOT EXISTS tokenshop (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	shop      TEXT NOT NULL,
	patternid INTEGER NOT NULL,
	price     INTEGER NOT NULL,
	token     TEXT NOT NULL DEFAULT '',
	says      TEXT NOT NULL DEFAULT '',
	sort      INTEGER NOT NULL DEFAULT 0,
	source    TEXT NOT NULL DEFAULT 'gm',
	UNIQUE (shop, patternid)
)"""
ITEMOUTFIT_DDL = """CREATE TABLE IF NOT EXISTS itemoutfit (
	patternid INTEGER NOT NULL,
	slot      INTEGER NOT NULL,
	look      INTEGER NOT NULL DEFAULT 0,
	color     INTEGER NOT NULL DEFAULT 0,
	note      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT 'gm',
	PRIMARY KEY (patternid, slot)
)"""

LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    seq    INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl    TEXT NOT NULL,
    rowkey TEXT NOT NULL,      -- the key (JSON)
    before TEXT,               -- the row before (JSON), NULL none
    after  TEXT NOT NULL,      -- the row written (JSON)
    audit  INTEGER NOT NULL
)"""


def record(t):
    return {"itemcost": 0, "unk1": 0, "itemicon": t["icon"], "unk2": 0, "equipslot": -1, "unk3": 0, "trade": 1, "rent": 1,
            "unk4": 0, "attacktype": 0, "weapondamage": 0, "unk5": 0, "levelreq": 1, "maxstack": 1, "maxhp": 0,
            "duration": 80, "classuse": 32767, "raceuse": 32767, "procanim": USABLE, "lore": 1, "unk6": 0, "craft": 0,
            "itemname": t["name"], "itemdesc": t["desc"], "model": 0, "color": 0xFFFFFFFF,
            "str": 0, "sta": 0, "agi": 0, "wis": 0, "dex": 0, "cha": 0, "intelligence": 0, "HPMAX": 0, "POWMAX": 0,
            "PoT": 0, "HoT": 0, "AC": 0, "PR": 0, "DR": 0, "FR": 0, "CR": 0, "LR": 0, "AR": 0, "weaponproc": 0, "fish": 0}


def table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def cols(db, tbl):
    return [r[1] for r in db.execute(f"PRAGMA table_info({tbl})")]


def row(db, tbl, key):
    where = " AND ".join(f"{k} = ?" for k in key)
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", tuple(key.values()))
    r = cur.fetchone()
    return None if r is None else dict(zip([d[0] for d in cur.description], r))


def go_json(m):
    return None if m is None else json.dumps(m, separators=(",", ":"), sort_keys=True, default=str)


def audit(db, tbl, key, action, before, after):
    acct = db.execute("SELECT accountid FROM accountinfo WHERE username = ?", (WHO,)).fetchone() if table(db, "accountinfo") else None
    at = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cur = db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) "
                     "VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)",
                     (at, acct[0] if acct else 0, WHO, VIA, action, tbl, go_json(key), go_json(before), go_json(after), DETAIL))
    return cur.lastrowid


def logged(db, tbl, key, before, after, a):
    db.execute(f"INSERT INTO {LOG} (tbl, rowkey, before, after, audit) VALUES (?, ?, ?, ?, ?)",
               (tbl, go_json(key), go_json(before), go_json(after), a))


def insert(db, tbl, want):
    """A new row of want in tbl (a table with an INTEGER PRIMARY KEY id: its id the key; else want's own key)."""
    cur = db.execute(f"INSERT INTO {tbl} ({', '.join(want)}) VALUES ({', '.join('?' * len(want))})", tuple(want.values()))
    return cur.lastrowid


def shop_spot_problems(db):
    x, _, z, _ = SHOP_AT
    out = []
    tree = db.execute("SELECT x_coord, z_coord FROM npcs WHERE npc_name = ? AND world = 0", (TREE,)).fetchall()
    if not tree:
        out.append(f"no {TREE} in world 0 (tools/christmas_season_20261116.py first)")
    for tx, tz in tree:
        if math.hypot(tx - x, tz - z) < TREE_CLEAR:
            out.append(f"the {TREE} at ({tx:.1f}, {tz:.1f}) is {math.hypot(tx - x, tz - z):.1f} from the spot, under {TREE_CLEAR}")
    for nid, name, nx, nz in db.execute("SELECT id, npc_name, x_coord, z_coord FROM npcs WHERE world = 0 AND npc_name <> ? "
                                        "AND abs(x_coord - ?) < 10 AND abs(z_coord - ?) < 10", (SHOP, x, z)):
        if math.hypot(nx - x, nz - z) < ROOM:
            out.append(f"npcs {nid} {name} stands {math.hypot(nx - x, nz - z):.2f} from the spot, under {ROOM}")
    return out


def plan(db):
    """(writes [(tbl, key, want)], lines, refusals); a key of None: a new row of a table keyed by id."""
    writes, lines, refusals = [], [], []
    for t in ("itempattern", "itemeffect", "npcs", "npcseason", "npcmove", "npcaggro", "gm_audit", "charinventory"):
        if not table(db, t):
            refusals.append(f"no {t} table here")
    if refusals:
        return writes, lines, refusals
    if not table(db, "gmseasons") or not db.execute("SELECT 1 FROM gmseasons WHERE name = ?", (SEASON,)).fetchone():
        refusals.append(f"no season {SEASON} in gmseasons (tools/christmas_season_20261116.py first)")
    tok = db.execute("SELECT patternid FROM itempattern WHERE itemname = ? COLLATE NOCASE ORDER BY patternid LIMIT 1", (TOKEN,)).fetchone()
    if not tok:
        refusals.append(f"no {TOKEN} record (tools/christmas_season_20261116.py first)")
    else:
        lines.append(f"  {TOKEN}: record {tok[0]}")

    for t in TOYS:
        if len(t["desc"]) > DESC_MAX or "|" in t["desc"] or not t["desc"].isascii():
            refusals.append(f"{t['name']}'s description is {len(t['desc'])} characters (at most {DESC_MAX}, ASCII, no '|')")
        if len(t["says"]) > 190 or "%" in t["says"]:
            refusals.append(f"{t['name']}'s words are {len(t['says'])} characters (at most 190, no '%')")
        want = record(t)
        cur = row(db, "itempattern", {"patternid": t["id"]})
        if cur is not None and cur["itemname"] != t["name"]:
            refusals.append(f"record {t['id']} is {cur['itemname']!r}, not {t['name']!r}")
            continue
        if cur is None:
            writes.append(("itempattern", {"patternid": t["id"]}, want))
            lines.append(f"  record {t['id']} {t['name']}: new (LORE, NO TRADE, usable, worth nothing)")
        else:
            lines.append(f"  record {t['id']} {t['name']}: there")
        if table(db, "itemowner") and row(db, "itemowner", {"patternid": t["id"]}):
            refusals.append(f"record {t['id']} is bound in itemowner: a shop's copy must be no one's")
        il = t.get("illusion")
        if il:
            ewant = {"line": il["line"], "name": il["name"], "icon": il["icon"], "breaks": "", "wear": 0, "area": 0,
                     "source": SRC, "body": il["body"], "target_line": "", "charges": 0, "cooldown": float(il["cooldown"])}
            ecur = row(db, "itemeffect", {"patternid": t["id"]})
            if ecur is None:
                writes.append(("itemeffect", {"patternid": t["id"]}, ewant))
                lines.append(f"  itemeffect {t['id']}: {il['line']!r}, body {il['body']:#010x}, cooldown {il['cooldown']} s")
            elif ecur["source"] != SRC:
                refusals.append(f"itemeffect {t['id']} is there and not this tool's ({ecur['source']})")
            for slot, (look, color) in sorted(il["outfit"].items()):
                key = {"patternid": t["id"], "slot": slot}
                if not table(db, "itemoutfit") or row(db, "itemoutfit", key) is None:
                    writes.append(("itemoutfit", key, {"look": look, "color": color, "note": f"{t['name']}: the Holiday Elf's look",
                                                       "source": SRC}))
            lines.append(f"  itemoutfit {t['id']}: {len(il['outfit'])} slots")
        skey = {"shop": SHOP, "patternid": t["id"]}
        if not table(db, "tokenshop") or row(db, "tokenshop", skey) is None:
            writes.append(("tokenshop", None, dict(skey, price=t["price"], token="", says=t["says"], sort=t["sort"], source=SRC)))
            lines.append(f"  tokenshop: {SHOP} sells {t['name']} for {t['price']} {TOKEN}s")

    # the shopkeeper
    have = db.execute("SELECT id FROM npcs WHERE npc_name = ?", (SHOP,)).fetchall()
    if have:
        lines.append(f"  {SHOP}: npcs {[h[0] for h in have]} there")
    else:
        tpl = row(db, "npcs", {"id": TEMPLATE[0]})
        if tpl is None or tpl["npc_name"] != TEMPLATE[1]:
            refusals.append(f"npcs {TEMPLATE[0]} is {tpl and tpl['npc_name']!r}, not {TEMPLATE[1]!r}")
        refusals += shop_spot_problems(db)
        x, y, z, f = SHOP_AT
        writes.append(("npcs", None, {"_copy": TEMPLATE[0], "npc_name": SHOP, "zone": 0, "world": 0, "x_coord": x, "y_coord": y,
                                      "z_coord": z, "facing": f, **SHOP_LOOK}))
        lines.append(f"  {SHOP}: a new npcs row at ({x}, {y}, {z}) facing {f}, a gnome man")
    for tbl, key, want in (("npcseason", {"name": SHOP}, {"season": SEASON, "note": "the Toy Shoppe, by the tree", "source": SRC}),
                           ("npcmove", {"name": SHOP.lower()}, {"roam": 0, "radius": 0, "water": "", "note": "stands (the Toy Shoppe, by the Christmas tree)", "source": SRC}),
                           ("npcaggro", {"npc_name": SHOP}, {"aggressive": 0, "radius": 0, "note": "the Toy Shoppe: friendly", "source": SRC})):
        cur = row(db, tbl, key)
        if cur is None:
            writes.append((tbl, key, want))
        elif tbl == "npcseason" and cur["season"] != SEASON:
            refusals.append(f"npcseason {SHOP} is of season {cur['season']!r}, not {SEASON}")
    if table(db, "npcdyes"):
        for slot, color in sorted(SHOP_DYES.items()):
            key = {"name": SHOP, "slot": slot}
            if row(db, "npcdyes", key) is None:
                writes.append(("npcdyes", key, {"color": color, "note": "the Toy Shoppe's toymaker", "source": SRC}))
        lines.append(f"  npcdyes: {SHOP} dyed ({len(SHOP_DYES)} slots)")
    else:
        lines.append("  npcdyes: no table here (branch christmas4 not yet): the shopkeeper in his disc tint alone")
    return writes, lines, refusals


def apply(db, writes):
    db.execute(LOG_DDL)
    db.execute(TOKENSHOP_DDL)
    db.execute(ITEMOUTFIT_DDL)
    for tbl, key, want in writes:
        if tbl == "npcs":
            want = dict(want)
            src = want.pop("_copy")
            c = [k for k in cols(db, "npcs") if k != "id"]
            nid = db.execute(f"INSERT INTO npcs ({', '.join(c)}) SELECT {', '.join(c)} FROM npcs WHERE id = ?", (src,)).lastrowid
            want["npc_id"] = nid
            db.execute(f"UPDATE npcs SET {', '.join(f'{k} = ?' for k in want)} WHERE id = ?", tuple(want.values()) + (nid,))
            key = {"id": nid}
        elif key is None:
            key = {"id": insert(db, tbl, want)}
        else:
            insert(db, tbl, dict(key, **want))
        after = row(db, tbl, key)
        logged(db, tbl, key, None, after, audit(db, tbl, key, "insert", None, after))


def holders(db, pid):
    """How many copies of a record characters hold (pack, worn or bank: charinventory)."""
    return db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (pid,)).fetchone()[0]


def undo(db):
    if not table(db, LOG):
        return 0, []
    back, left = 0, []
    kept = {t["id"] for t in TOYS if holders(db, t["id"]) > 0}
    for seq, tbl, key, after in db.execute(f"SELECT seq, tbl, rowkey, after FROM {LOG} ORDER BY seq DESC").fetchall():
        key = json.loads(key)
        if tbl in ("itempattern", "itemeffect", "itemoutfit") and key.get("patternid") in kept:
            left.append(f"{tbl} {key}: a copy is still held, left as it is")
            continue
        now = row(db, tbl, key)
        if now is None:
            db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
            continue
        if go_json(now) != after:
            left.append(f"{tbl} {key}: changed since, left as it is")
            continue
        where = " AND ".join(f"{k} = ?" for k in key)
        db.execute(f"DELETE FROM {tbl} WHERE {where}", tuple(key.values()))
        audit(db, tbl, key, "delete", now, None)
        db.execute(f"DELETE FROM {LOG} WHERE seq = ?", (seq,))
        back += 1
    if not left:
        db.execute(f"DROP TABLE {LOG}")
    return back, left


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", required=True)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None, timeout=30)
    db.execute("BEGIN IMMEDIATE")
    try:
        if a.undo:
            back, left = undo(db)
            db.execute("COMMIT")
            print(f"undone: {back} rows taken out" + "".join(f"\n  left: {w}" for w in left) +
                  ("; restart (keep) for the world to lose them" if back else ""))
            return 0 if not left else 1
        writes, lines, refusals = plan(db)
        for line in lines:
            print(line)
        if refusals:
            db.execute("ROLLBACK")
            print("REFUSED, nothing written:\n  " + "\n  ".join(refusals))
            return 1
        if not writes:
            db.execute("ROLLBACK")
            print("nothing to do")
            return 0
        if not a.apply:
            db.execute("ROLLBACK")
            print(f"{len(writes)} rows to write (report only: --apply writes them)")
            return 0
        apply(db, writes)
        db.execute("COMMIT")
        print(f"applied: {len(writes)} rows, in gm_audit and {LOG}; restart (keep) for the world to read them")
        return 0
    except BaseException:
        db.execute("ROLLBACK")
        raise
    finally:
        db.close()


if __name__ == "__main__":
    sys.exit(main())
