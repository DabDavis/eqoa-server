#!/usr/bin/env python3
"""The Holiday Elf as a gnome in red and green, and an ogre Santa Claus by the tree (the owner, 2026-11-16: "make holiday
elfs as gnome model, with red and green dyed clothing, also add ogre santa clause in a santa suit we can make with what
we have").

- Holiday Elf (its npcs row, tools/christmas_season_20261116.py): a gnome man's body (0xA99C6B15, race 3), blond, in cloth
  (set 1) with leather boots and a cloth cap; dyed (table npcdyes, a server with entity/npcdyes.go) a red tunic with a
  white trim, green bracers, white gloves, green leggings, red boots, a green cap with a red band. A PS2 is sent each
  slot's nearest disc tint: red 8, green 5, white 0 (no trim).
- Santa Claus: a new npcs row (a copy of Smithy Grol 653's, the ogre man 0x84807490, race 9) south-east of the tree, its
  presents and the Elf: robe 0 (its bands and panels are the robe's trim) dyed red with a white trim, black leather gloves
  and boots, a red cloth cap with a white band, grey-white hair (colour 3) under it. The ogre faces carry a brown chin beard
  in the face's own texture, which no hair colour or face option makes white (checked: faces 0-5, hair colours 0-7). A PS2:
  red robe (tint 8), dark grey boots and gloves (1), red cap (8). Friendly (npc_type 128), standing, never attacking, of the
  Christmas season (npcseason: gone with the tree). Its own lines (npcdialog).

A report by default; --apply writes it with gm_audit lines and keeps what it changed in --applied; --undo puts it back.
The server reads npcs at its start: a keep restart's before.sh (npcdyes too; or touch NPC dyes in the GM panel).

    christmas_elf_santa_20261116.py --db eqoa.db [--apply | --undo] --applied FILE
"""
import argparse, json, os, sqlite3, sys, time

SRC, SEASON = "christmas-2026", "Christmas"
ELF, SANTA, TREE = "Holiday Elf", "Santa Claus", "Christmas Tree"
SANTA_TEMPLATE = (653, "Smithy Grol")
GNOME_MALE, OGRE_MALE = 0xA99C6B15, 0x84807490
ELF_LOOK = {"modelid": GNOME_MALE, "race": "3", "torso": 1, "forearm": 1, "gloves": 1, "legs": 1, "feet": 2, "head": 1,
            "robe_style": "255", "hair_color": 2, "hair_length": 2, "hair_style": 3, "face": 1, "unk_o": 0, "size": 1.0}
SANTA_LOOK = {"modelid": OGRE_MALE, "race": "9", "torso": 0, "forearm": 0, "gloves": 2, "legs": 0, "feet": 2, "head": 1,
              "robe_style": "0", "hair_color": 3, "hair_length": 1, "hair_style": 1, "face": 0, "unk_o": 0, "size": 1.0}
RED, GREEN, WHITE, BLACK = 0xC81E1EFF, 0x1E8C3CFF, 0xF4F4F4FF, 0x1C1C1CFF
# dye slots as chardyes: 0 chest, 1 bracer, 2 gloves, 3 legs, 4 boots, 5 helm, 9 robe, 11-16 the trims of chest .. helm
DYES = {
    ELF: {0: RED, 11: WHITE, 1: GREEN, 2: WHITE, 3: GREEN, 4: RED, 5: GREEN, 16: RED},
    SANTA: {9: RED, 11: WHITE, 14: WHITE, 2: BLACK, 4: BLACK, 5: RED, 16: WHITE},
}
# by the tree at (25270, 15754.7): south-east of it, 17 from its middle (its presents end at 10.6, those spilling east at
# 13.5), 26 from the Elf (25262.2, 15744.1); facing byte 32, south-east, out to the square
SANTA_AT = (25279.0, 54.1, 15769.0, 32)
LINES = [
    (SANTA, "Ho ho ho! Merry Christmas, %p!"),
    (SANTA, "Have you been naughty or nice this year, %p? Ho ho ho!"),
    (SANTA, "My Holiday Elf has a gift for you every day of the season. Go and see the little fellow by the tree!"),
    (SANTA, "The Holiday Elf needs help delivering gifts and singing for the guards. Lend him a hand, %p, and earn some Holly Tokens."),
    (SANTA, "Holly Tokens? The Weapon Enchanter will gladly take them. Ho ho ho!"),
    (SANTA, "It is a long way from the North to Freeport, but an ogre's legs are long. Ho ho ho!"),
]

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def row(tbl, where, args):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where}", args)
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before, sort_keys=True),
                None if after is None else json.dumps(after, sort_keys=True),
                "the owner: the Holiday Elf a gnome in red and green, an ogre Santa Claus; tools/christmas_elf_santa_20261116.py"))


if a.undo:
    j = json.load(open(a.applied))
    elf = j["elf"]
    sets = ", ".join(f"{k} = ?" for k in ELF_LOOK)
    db.execute(f"UPDATE npcs SET {sets} WHERE id = ?", [elf[k] for k in ELF_LOOK] + [elf["id"]])
    audit("update", "npcs", {"id": elf["id"]}, None, {k: elf[k] for k in ELF_LOOK})
    r = row("npcs", "id = ?", (j["santa"],))
    if r is not None and r["npc_name"] == SANTA:
        db.execute("DELETE FROM npcs WHERE id = ?", (j["santa"],))
        audit("delete", "npcs", {"id": j["santa"]}, r, None)
    for tbl, where, args in [("npcdyes", "source = ? AND name IN (?, ?)", (SRC, ELF, SANTA)), ("npcmove", "source = ? AND name = ?", (SRC, SANTA.lower())),
                             ("npcaggro", "source = ? AND npc_name = ?", (SRC, SANTA)), ("npcseason", "source = ? AND name = ?", (SRC, SANTA)),
                             ("npcdialog", "source = ? AND name = ?", (SRC, SANTA))]:
        n = db.execute(f"DELETE FROM {tbl} WHERE {where}", args).rowcount
        audit("delete", tbl, {"where": where, "args": list(args)}, {"rows": n}, None)
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()

if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
have = {r[0] for r in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
if "npcdyes" not in have:
    sys.exit("table npcdyes is not here: start the server with npcdyes (christmas4) on this database once first; nothing done")
elves = db.execute("SELECT id FROM npcs WHERE npc_name = ?", (ELF,)).fetchall()
if len(elves) != 1:
    sys.exit(f"{len(elves)} Holiday Elf rows, not one: nothing done")
for q, args, what in [("SELECT count(*) FROM npcs WHERE npc_name = ?", (SANTA,), "a Santa Claus row"),
                      ("SELECT count(*) FROM npcdyes WHERE name IN (?, ?)", (ELF, SANTA), "a dye on the Elf or Santa"),
                      ("SELECT count(*) FROM npcseason WHERE name = ?", (SANTA,), "Santa's season row")]:
    if db.execute(q, args).fetchone()[0]:
        sys.exit(f"{what} is here already: nothing done")
t = row("npcs", "id = ?", (SANTA_TEMPLATE[0],))
if t is None or t["npc_name"] != SANTA_TEMPLATE[1] or (t["modelid"] & 0xFFFFFFFF) != OGRE_MALE:
    sys.exit(f"npcs {SANTA_TEMPLATE[0]} is {t and t['npc_name']}, not the ogre {SANTA_TEMPLATE[1]}: nothing done")
eid = elves[0][0]
was = row("npcs", "id = ?", (eid,))
j = {"elf": {"id": eid, **{k: was[k] for k in ELF_LOOK}}}
sets = ", ".join(f"{k} = ?" for k in ELF_LOOK)
db.execute(f"UPDATE npcs SET {sets} WHERE id = ?", list(ELF_LOOK.values()) + [eid])
audit("update", "npcs", {"id": eid}, {k: was[k] for k in ELF_LOOK}, ELF_LOOK)
cols = [c for c in t if c != "id"]
sid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (SANTA_TEMPLATE[0],)).lastrowid
x, y, z, fb = SANTA_AT
db.execute("UPDATE npcs SET npc_name = ?, zone = 0, unk_a = 0, unk_b = 0, world = 0, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, "
           "npc_type = 128, npc_level = 50, npc_id = ?, primary_weapon = 0, secondary_weapon = 0, shield = 0 WHERE id = ?", (SANTA, x, y, z, fb, sid, sid))
sets = ", ".join(f"{k} = ?" for k in SANTA_LOOK)
db.execute(f"UPDATE npcs SET {sets} WHERE id = ?", list(SANTA_LOOK.values()) + [sid])
audit("insert", "npcs", {"id": sid}, None, row("npcs", "id = ?", (sid,)))
j["santa"] = sid
for name, dyes in DYES.items():
    for slot, c in dyes.items():
        db.execute("INSERT INTO npcdyes (name, slot, color, note, source) VALUES (?, ?, ?, ?, ?)", (name, slot, c, "Christmas", SRC))
    audit("insert", "npcdyes", {"name": name}, None, {str(k): f"{v:08X}" for k, v in dyes.items()})
db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (by the Christmas tree)', ?)", (SANTA.lower(), SRC))
db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Christmas season: friendly', ?)", (SANTA, SRC))
db.execute("INSERT INTO npcseason (name, season, note, source) VALUES (?, ?, 'by the tree', ?)", (SANTA, SEASON, SRC))
for name, line in LINES:
    db.execute("INSERT INTO npcdialog (town, role, name, line, weight, source) VALUES ('', '', ?, ?, 2, ?)", (name, line, SRC))
audit("insert", "npcmove", {"name": SANTA}, None, {"npcmove": "stands", "npcaggro": "friendly", "npcseason": SEASON, "npcdialog": len(LINES)})
print(f"{ELF} npcs {eid}: a gnome in red and green ({len(DYES[ELF])} dyes); {SANTA} npcs {sid} at ({x}, {z}): an ogre in a red "
      f"robe with a white trim ({len(DYES[SANTA])} dyes), {len(LINES)} lines")
if a.apply:
    json.dump(j, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
