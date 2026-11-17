#!/usr/bin/env python3
"""Santa's sleigh and its team by Freeport's Christmas tree (the owner, 2026-11-16: "where dust is standing is what we can
use for santas sleigh"; "i believe there are deer ambient creatures that could be used for santa sleigh").

The sleigh is the sled that stands by the igloo where Dust stood in Tunaria's north (TUNARIA.ESF 0x0B3BA520, zone 22, at
5859.8, 54.1, 6663.8): an NPC "Santa's Sleigh" beside Santa, west of the tree, worn as that art by the clients that draw
decorations (decorart, as the tree is) and, to a PS2, the chest's body (0x3B09103E, the Runed Box's: a chest of
presents where the sleigh stands; the invisible blocker's body stood the sleigh 630 up in the air in vanilla). Its
deck carries a pile of presents and its rim a string of lights (lightstrings on the NPC: they turn with it).

No deer body is on the disc: CHAR.ESF's 582 models hold none, and Tunaria's ambient flocks (0x01CBC6D7, 0x91B2E7A9)
are birds. The team is CHAR.ESF's white winged horse (0x2F8721C0, a body no NPC wears), which a PS2 draws too: Rudolph
in the lead with a red nose (a red glow at his nose), Dasher and Dancer, Prancer and Vixen in pairs behind him,
harnessed to the sleigh by a red strap with lights. Each is friendly (npc_type 128, npcaggro 0), stands
(npcmove), of the Christmas season (npcseason: gone with the tree).

A report by default; --apply writes it with gm_audit lines and keeps the rows in --applied; --undo deletes them. Data
only: any server from christmas3 on (decorart, presents, strings on NPCs); vanilla needs `is_art` (decor-art-still: the
sled otherwise climbs onto its own top). Two of the sleigh's presents name a paper (christmas5's looks): plain before.
New NPCs: a restart (keep restart, before.sh) makes them live.

    christmas_sleigh_20261116.py --db eqoa.db --applied FILE [--apply | --undo]
"""
import argparse, json, math, os, sqlite3, sys, time

SRC, SEASON = "christmas-2026-sleigh", "Christmas"
SLEIGH, SLED, SLED_ZONE, CHEST = "Santa's Sleigh", 0x0B3BA520, 22, 0x3B09103E
STEED = 0x2F8721C0
# The sleigh on the open sand south of the tree (west of it the pond begins), its front (the sled's curled runner,
# its own -x) to the east: facing byte 128 (a half turn: its -x to the world's +x); the team ahead of it, east, facing
# east (byte 64: its ahead, +z, to the world's +x), clear of the presents, Santa, the chest and the townsfolk east.
SLEIGH_AT = (25261.0, 54.1, 15788.0, 128)       # x, y, z, facing byte
FRONT = SLEIGH_AT[0] + 3.7                       # the curled runner's tip, world x
TEAM = [  # name, x, z (world)
    ("Rudolph", 25272.6, 15788.0),
    ("Dasher", 25270.1, 15787.1), ("Dancer", 25270.1, 15788.9),
    ("Prancer", 25267.6, 15787.1), ("Vixen", 25267.6, 15788.9),
]
TEAM_FACING = 64
TEAM_SIZE = 0.65  # the winged horse at its own size stands taller than the sleigh is long
LINES = [
    ("Santa's Sleigh", "The sleigh is loaded to the rails. Not one present more, or the team will never get it off the ground."),
    ("Rudolph", "Rudolph tosses his head, and his red nose glows a little brighter."),
    ("Dasher", "Dasher paws at the sand, eager to be off."),
]
TEMPLATES = [
    # name, colors, pattern, effect, period, spacing, size, bright, cord, cord_width, bulb, bulb_scale, cord_texture, cord_sag, note
    ("Red nose", "FF1010", "cycle", "twinkle", 3.0, 1, 0.55, 2.0, "none", 0.06, "", 0.35, "", 0,
     "Rudolph's nose: one red glow, swelling and dimming."),
    ("Sleigh lights", "FF2A2A, 2AE04A, FFC23A, FFFFFF", "cycle", "chase", 1.2, 0.45, 0.32, 1.1, "1E3F1E", 0.04, "", 0.35, "", 0,
     "A chase of small bulbs along the sleigh's rim and the harness."),
    ("Sleigh harness", "C02020", "cycle", "steady", 1.5, 0.8, 0.01, 0.2, "8A1A1A", 0.09, "", 0.35, "", 0,
     "The team's harness: a red strap from the sleigh to Rudolph (its bulbs a pin-point, unseen)."),
]
PRESENTS = {"boxes": [  # on the sled's deck (its own frame: y 0.68 up, x -2 .. 3 along, z -1.2 .. 1.2 across)
    {"at": [-1.5, 0.68, -0.5], "size": [0.7, 0.6, 0.7], "yaw": 10, "color": "C02020", "ribbon": "FFD040", "bow": True},
    {"at": [-1.4, 0.68, 0.5], "size": [0.6, 0.5, 0.8], "yaw": -15, "color": "1E8C3C", "ribbon": "F0F0F0", "bow": True},
    {"at": [-0.5, 0.68, 0.0], "size": [0.9, 0.8, 0.9], "yaw": 5, "color": "F2E8D0", "ribbon": "C02020", "bow": True, "paper": "0x9EEAF67B", "gain": 2.2},
    {"at": [0.5, 0.68, -0.6], "size": [0.6, 0.6, 0.6], "yaw": 30, "color": "3A6ED8", "ribbon": "E0E0F0", "bow": True},
    {"at": [0.6, 0.68, 0.6], "size": [0.8, 0.5, 0.6], "yaw": -5, "color": "8A3CC0", "ribbon": "FFD040"},
    {"at": [1.5, 0.68, 0.0], "size": [0.8, 0.9, 0.8], "yaw": 20, "color": "F6F0B0", "ribbon": "1E8C3C", "bow": True, "paper": "0x58B41D37", "gain": 2.6},
    {"at": [-0.5, 1.48, 0.05], "size": [0.5, 0.4, 0.5], "yaw": 40, "color": "FFC23A", "ribbon": "C02020", "bow": True},
    {"at": [1.5, 1.58, 0.0], "size": [0.45, 0.4, 0.45], "yaw": -30, "color": "D02060", "ribbon": "F0F0F0", "bow": True},
]}
# the rim (the sled's own frame): along both sides at the deck's edge, and round the curled front
RIM = [[x / 2, 0.75, side * 1.3] for side in (-1, 1) for x in range(-6, 7)]
# Rudolph's nose, in the world: 1.4 ahead of him (east) and 2.1 up (the winged horse rears), checked in the rig's renders
NOSE_AHEAD, NOSE_UP = 1.4, 2.1

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
NAMES = [SLEIGH] + [t[0] for t in TEAM]


def audit(action, tbl, key, before, after):
    db.execute("INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail) VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)",
               (now, action, tbl, json.dumps(key), None if before is None else json.dumps(before), None if after is None else json.dumps(after),
                "the owner: Santa's sleigh and its team; tools/christmas_sleigh_20261116.py"))


if a.undo:
    j = json.load(open(a.applied))
    for nid in j["npcs"]:
        r = db.execute("SELECT * FROM npcs WHERE id = ?", (nid,)).fetchone()
        if r is not None and r["npc_name"] in NAMES:
            db.execute("DELETE FROM npcs WHERE id = ?", (nid,))
            audit("delete", "npcs", {"id": nid}, dict(r), None)
    for sid in j["strings"]:
        db.execute("DELETE FROM lightstrings WHERE id = ? AND source = ?", (sid, SRC))
    for tbl, col in [("decorart", "name"), ("npcseason", "name"), ("npcaggro", "npc_name"), ("lighttemplates", "name")]:
        db.execute(f"DELETE FROM {tbl} WHERE source = ?", (SRC,))
    db.execute("DELETE FROM npcmove WHERE source = ?", (SRC,))
    db.execute("DELETE FROM npcdialog WHERE source = ?", (SRC,))
    audit("delete", "npcs", {"source": SRC}, {"npcs": j["npcs"], "strings": j["strings"]}, None)
    db.commit(); os.rename(a.applied, a.applied + ".undone"); print("undone"); sys.exit()
if a.apply and os.path.exists(a.applied):
    sys.exit("applied already")
for t in TEMPLATES:
    if db.execute("SELECT count(*) FROM lighttemplates WHERE name = ? COLLATE NOCASE", (t[0],)).fetchone()[0]:
        sys.exit(f"a light template {t[0]!r} is here already: nothing done")
for n in NAMES:
    if db.execute("SELECT count(*) FROM npcs WHERE npc_name = ?", (n,)).fetchone()[0]:
        sys.exit(f"an NPC {n!r} is here already: nothing done")
santa = db.execute("SELECT * FROM npcs WHERE npc_name = 'Santa Claus' ORDER BY id LIMIT 1").fetchone()
tree = db.execute("SELECT * FROM npcs WHERE npc_name = 'Christmas Tree' ORDER BY id LIMIT 1").fetchone()
if santa is None or tree is None:
    sys.exit("no Santa Claus or Christmas Tree (christmas3/4's tools first): nothing done")
cols = [c for c in santa.keys() if c != "id"]
done = {"npcs": [], "strings": []}


def npc_from(base, name, model, x, y, z, facing, size=1.0):
    nid = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (base["id"],)).lastrowid
    db.execute("UPDATE npcs SET npc_name = ?, modelid = ?, x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, npc_type = 128, npc_id = ?, size = ?, "
               "primary_weapon = 0, secondary_weapon = 0, shield = 0, torso = 0, forearm = 0, gloves = 0, legs = 0, feet = 0, head = 0, "
               "robe_style = '255', race = '0' WHERE id = ?", (name, model, x, y, z, facing, nid, size, nid))
    audit("insert", "npcs", {"id": nid}, None, {"npc_name": name, "modelid": f"0x{model:08X}", "at": [x, y, z], "facing": facing})
    done["npcs"].append(nid)
    db.execute("INSERT INTO npcseason (name, season, note, source) VALUES (?, ?, 'by the tree', ?)", (name, SEASON, SRC))
    db.execute("INSERT INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, 'the Christmas season: friendly', ?)", (name, SRC))
    db.execute("INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 0, 0, '', 'stands (by the Christmas tree)', ?)", (name.lower(), SRC))


x, y, z, f = SLEIGH_AT
npc_from(santa, SLEIGH, CHEST, x, y, z, f)
db.execute("INSERT INTO decorart (name, model, world, zone, sink, note, source) VALUES (?, ?, 0, ?, 0, ?, ?)",
           (SLEIGH, SLED, SLED_ZONE, "the sled by the igloo where Dust stood (TUNARIA.ESF 0x0B3BA520, zone 22); a PS2 sees a chest", SRC))
for name, tx, tz in TEAM:
    npc_from(santa, name, STEED, tx, 54.1, tz, TEAM_FACING, TEAM_SIZE)
for t in TEMPLATES:
    # the columns every server has (christmas3 on); the looks' (christmas5) are left at their defaults
    db.execute("""INSERT INTO lighttemplates (name, colors, pattern, effect, period, spacing, size, bright, cord, cord_width, note, source)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""", t[:10] + (t[14], SRC))


def add_string(template, npc, shape, params, note):
    sid = db.execute("INSERT INTO lightstrings (template, npc, world, shape, params, season, note, source) VALUES (?, ?, 0, ?, ?, ?, ?, ?)",
                     (template, npc, shape, json.dumps(params, separators=(",", ":")), SEASON, note, SRC)).lastrowid
    audit("insert", "lightstrings", {"id": sid}, None, {"template": template, "npc": npc, "shape": shape, "note": note})
    done["strings"].append(sid)


add_string("", SLEIGH, "presents", PRESENTS, "the presents piled on Santa's sleigh")
add_string("Sleigh lights", SLEIGH, "path", {"points": RIM[:13]}, "the sleigh's rim, its left side")
add_string("Sleigh lights", SLEIGH, "path", {"points": RIM[13:]}, "the sleigh's rim, its right side")
add_string("Red nose", "", "points", {"points": [[TEAM[0][1] + NOSE_AHEAD, 54.1 + NOSE_UP, TEAM[0][2]]]}, "Rudolph's red nose")
# the harness: from the sleigh's front corners to Rudolph, along each side of the team (world coordinates)
lead = TEAM[0]
for side in (-1, 1):
    p0 = [FRONT, 55.0, z + side * 1.0]
    p1 = [lead[1] - 0.8, 55.3, lead[2] + side * 0.35]
    add_string("Sleigh harness", "", "line", {"from": p0, "to": p1, "sag": 0.15}, f"the harness, the team's {'left' if side < 0 else 'right'} side")
    add_string("Sleigh lights", "", "line", {"from": [p0[0], p0[1] + 0.12, p0[2]], "to": [p1[0], p1[1] + 0.12, p1[2]], "sag": 0.15},
               f"the harness's lights, the team's {'left' if side < 0 else 'right'} side")
for name, line in LINES:
    db.execute("INSERT INTO npcdialog (town, role, name, line, weight, source) VALUES ('', '', ?, ?, 2, ?)", (name, line, SRC))
print(f"{SLEIGH} at ({x}, {z}) and its team of {len(TEAM)} (Rudolph leading), {len(done['strings'])} strings (presents, rim, nose, harness)")
if a.apply:
    json.dump(done, open(a.applied, "w")); db.commit(); print("applied")
else:
    db.rollback()
