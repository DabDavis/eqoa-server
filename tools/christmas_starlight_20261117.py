#!/usr/bin/env python3
"""Starlight, a white winged horse by Freeport's Christmas tree (the owner, 2026-11-17: "both garlands and globes on the
tree, also put the pegasus white winged horse by the tree").

One NPC "Starlight": CHAR.ESF's white winged horse (0x2F8721C0, the sleigh team's body; a PS2 draws it too) at size
0.8, at the tree's south edge facing out to the square, clear of the presents, Santa, the elves, Toymaker Tinsel, the
Weapon Enchanter, the well and the sleigh. A copy of Santa Claus's row: friendly (npc_type 128, npcaggro 0), standing
(npcmove), of the Christmas season (npcseason: gone with the tree), a line of its own.

A report by default; --apply writes it with gm_audit lines and keeps the row in --applied; --undo deletes it. Data
only; a new NPC is live after a restart (keep restart, before.sh).

    christmas_starlight_20261117.py --db eqoa.db --applied FILE [--apply | --undo]
"""
import argparse, json, os, sqlite3, sys, time

SRC, SEASON, NAME, STEED = "christmas-2026-starlight", "Christmas", "Starlight", 0x2F8721C0
AT = (25265.0, 54.1, 15770.5)  # the tree (25270, 15754.7)'s south edge, past its presents
FACING = 0                     # out to the square, south (checked in the rig's renders)
SIZE = 0.8
LINE = "Starlight shakes out her wings, and a dusting of snow drifts down from her feathers."

ap = argparse.ArgumentParser()
ap.add_argument("--db", required=True)
ap.add_argument("--applied", required=True)
g = ap.add_mutually_exclusive_group()
g.add_argument("--apply", action="store_true")
g.add_argument("--undo", action="store_true")
a = ap.parse_args()
db = sqlite3.connect(a.db, timeout=30)
db.row_factory = sqlite3.Row
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def audit(action, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, 'npcs', ?, ?, ?, ?)",
               (now, action, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                "the owner: a white winged horse by the tree; tools/christmas_starlight_20261117.py"))


if a.undo:
    j = json.load(open(a.applied))
    r = db.execute("SELECT * FROM npcs WHERE id = ?", (j["npc"],)).fetchone()
    if r is not None and r["npc_name"] == NAME:
        db.execute("DELETE FROM npcs WHERE id = ?", (j["npc"],))
        audit("delete", {"id": j["npc"]}, dict(r), None)
    for tbl in ("npcseason", "npcaggro", "npcmove", "npcdialog"):
        db.execute(f"DELETE FROM {tbl} WHERE source = ?", (SRC,))
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
if db.execute("SELECT count(*) FROM npcs WHERE npc_name = ?", (NAME,)).fetchone()[0]:
    sys.exit(f"an NPC {NAME!r} is here already: nothing done")
santa = db.execute("SELECT * FROM npcs WHERE npc_name = 'Santa Claus' ORDER BY id LIMIT 1").fetchone()
if santa is None:
    sys.exit("no Santa Claus (tools/christmas_elf_santa_20261116.py first): nothing done")
cols = [c for c in santa.keys() if c != "id"]
nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (santa["id"],)).lastrowid
x, y, z = AT
db.execute("UPDATE npcs SET npc_name = ?, modelid = ?, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, npc_type = 128, npc_id = ?, size = ?, "
           "primary_weapon = 0, secondary_weapon = 0, shield = 0, torso = 0, forearm = 0, gloves = 0, legs = 0, feet = 0, head = 0, "
           "robe_style = '255', race = '0' WHERE id = ?", (NAME, STEED, x, y, z, FACING, nid, SIZE, nid))
audit("insert", {"id": nid}, None, {"npc_name": NAME, "modelid": f"0x{STEED:08X}", "at": list(AT), "facing": FACING, "size": SIZE})
db.execute("INSERT INTO npcseason (name, season, note, source) VALUES (?, ?, 'by the tree', ?)", (NAME, SEASON, SRC))
db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Christmas season: friendly', ?)", (NAME, SRC))
db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (by the Christmas tree)', ?)", (NAME.lower(), SRC))
db.execute("INSERT INTO npcdialog (town, role, name, line, weight, source) VALUES ('', '', ?, ?, 2, ?)", (NAME, LINE, SRC))
print(f"{NAME} (npcs {nid}) at ({x}, {z}), size {SIZE}, facing {FACING}")
if a.apply:
    json.dump({"npc": nid}, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
