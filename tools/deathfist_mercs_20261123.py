#!/usr/bin/env python3
"""The Deathfist mercenary camps dressed and peopled as the guide tells them (the owner, 2026-11-23: "Deathfist
mercenary camps where a GM stood need populating correctly; they are supposed to have armoured melee and robed
healers"). Every camp of world 0's Deathfist mercenaries (Southwatch Keep's and the eight round it in Deathfist
Horde), on either shard.

The guide (MasterVG782's GameFAQs walkthrough, SOUTHWATCH KEEP, lines 6757-6760): "During the day, you'll run into
Deathfist Mercenaries, which come in three varieties. There are female Ogres (shamans), male Ogres (quad-hitting
melees) and female Trolls (Shadowknights ...)". Our rows carry exactly those three bodies (CHAR.ESF 0xACC00B4F ogre
female, 0x84807490 ogre male, 0x4DE32701 troll female: the player races' own bodies, so each takes the $Armor sets
on text slots 0-4, a robe on 0 and 3, and a helm on its head node; measured: all three CSprites' TSlotLists hold
slots 0-4 as the human male's does), but every row was bare (no armour, no robe, mostly no weapon) and outside the
keep all three were one warrior kind, so no camp had a healer. The keep's own split (the unique-audit of 2026-11-21:
"Deathfist mercenary knight" class 3 and "Deathfist mercenary shaman" class 8, by body) is carried to every camp:

  melee     "Deathfist mercenary" (the male ogres, warriors: quad from 35 with no shield, so none given): banded
            armour (set 5) on all five slots and its helm, tint 3 (brown: the bands' iron gone a rusted bronze,
            a sellsword's kit), their weapon kept where they had one (the scimitar model), else a greataxe
            (ITEM.ESF -335179509, the Rusty/bronze Greataxe's).
  knights   "Deathfist mercenary knight" (the female trolls; the guide's shadow knights: the ones named plain
            "Deathfist mercenary" outside the keep renamed): plate (set 4) and its helm, tint 13 (burgundy, a shadow
            knight's), a longsword (-755168116, the Northwind/Thornsting Longsword's) and a tower shield (612804823,
            the Tower Shield of the Horde's). A shadow knight swings Attack3 from 20 whether it bears a shield or
            not, so the shield changes no round.
  healers   "Deathfist mercenary shaman" (the female ogres; renamed outside the keep): robe 3 (the fur and hide robe,
            the one the camp shamans of these races already wear: the troll baga mystics, Seer Crelga), no armour,
            tint 0 (an NPC row's one tint is laid on every bare slot too: anything but white would dye her skin), a
            staff (1660958085, the Oaken Gnarled/Worn Crook Staff's). They heal: npctemplate 'kin healer' (petition
            5322's: 15% of her hit points, instant, every 15 s, on herself or the worst off of her kin; her kill
            lowers Deathfist Mercenaries most, so every mercenary of the faction is her kin).
  new       Three camps had no female ogre (the 805 camp of three, the 802/803 camp of four, the 948 pair): one
            shaman each, a whole copy of the keep's shaman row (name, body, level 40, race, hair) at a spot on
            checked ground (cmd/campground over worlds/ground: a floor within reach, not roofed, not embedded, no
            wall between it and the camp's nearest creature, dry; 6+ from every NPC of both shards), its zone the
            nearest camp member's, its respawn the keep shaman's (npcrespawn 90 s).
  drops     The renamed keep their faction, class, loot category and kill hits (every table already holds the
            knight's and shaman's rows); the two trash rows lootpool held for "deathfist mercenary" alone are given
            to the knight and the shaman too, so a renamed row drops what it did.

Rows are found by name, world and body; each shard's own (Heirloom and Live differ by one row: the 801/803 camp's
third is a troll on Heirloom, a female ogre on Live). Refused, nothing written, if a shard has fewer rows of a
(name, body) than read on 2026-11-23 (EXPECT) or a body none of them had, if any of them is no longer bare (another
hand dressed it), if the keep's shaman row or a camp is not where it was, or if a new spot is taken. More rows than
read are taken in and said: a GM was placing mercenaries by hand on Heirloom while this was written (three male
ogres at the keep, /place and /clone of row 111056, 04:30 UTC), and a bare copy of a kind is dressed as its kind.
New ids from the shard's max + 1.

Report only by default (every change printed); --apply writes it in one transaction, every row with a gm_audit line
(dab, via "tool") and in deathfist_mercs_1123_log; --undo puts back exactly what it wrote (a row changed since is
left and said) and drops the log. --campground BIN --ground DIR checks the new spots on the ground again first.

WHEN IT TAKES EFFECT: the renames, the new rows, the template and the loot at the next restart. The look columns of
an existing row can go live at once on a running shard through the GM panel's update_row on npcs (it changes the
NPC in the world); this tool writes the database only.

    python3 -E -s tools/deathfist_mercs_20261123.py --db eqoa.db --shard heirloom|live [--apply | --undo]
        [--campground BIN --ground DIR]
"""
import argparse
import json
import math
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tix4_rows as tr  # noqa: E402

LOG = "deathfist_mercs_1123_log"
DETAIL = "Deathfist mercenary camps dressed and peopled (the owner, 2026-11-23): tools/deathfist_mercs_20261123.py"
SRC = "deathfist-mercs 20261123"

MERC, KNIGHT, SHAMAN = "Deathfist mercenary", "Deathfist mercenary knight", "Deathfist mercenary shaman"
OGRE_M, OGRE_F, TROLL_F = 0x84807490, 0xACC00B4F, 0x4DE32701
BODY_NAME = {OGRE_M: "ogre male", OGRE_F: "ogre female", TROLL_F: "troll female"}
KIND_OF_BODY = {OGRE_M: MERC, TROLL_F: KNIGHT, OGRE_F: SHAMAN}

ARMOR = ("torso", "forearm", "gloves", "legs", "feet", "head")
SCIMITAR = 287843574          # the weapon some mercenaries already hold (Whistling Sabre, Crude Stone Scimitar)
GREATAXE = -335179509         # Rusty Greataxe, bronze greataxe
LONGSWORD = -755168116        # Northwind Longsword, Thornsting Longsword
TOWER_SHIELD = 612804823      # Tower Shield of the Horde
STAFF = 1660958085            # Oaken Gnarled Staff, Worn Crook Staff

LOOK = {
    MERC: dict({c: 5 for c in ARMOR}, unk_o=3, robe_style="255", shield=0, secondary_weapon=0),
    KNIGHT: dict({c: 4 for c in ARMOR}, unk_o=13, robe_style="255", primary_weapon=LONGSWORD, shield=TOWER_SHIELD,
                 secondary_weapon=0),
    SHAMAN: dict({c: 0 for c in ARMOR}, unk_o=0, robe_style="3", primary_weapon=STAFF, shield=0, secondary_weapon=0),
}

HEAL_TEMPLATE = "kin healer"

# rows per (name, body) in world 0, as read 2026-11-23 off each shard's pre-swap backup
EXPECT = {
    "heirloom": {(MERC, OGRE_M): 16, (MERC, TROLL_F): 8, (MERC, OGRE_F): 7, (KNIGHT, TROLL_F): 3, (SHAMAN, OGRE_F): 1},
    "live": {(MERC, OGRE_M): 16, (MERC, TROLL_F): 7, (MERC, OGRE_F): 8, (KNIGHT, TROLL_F): 3, (SHAMAN, OGRE_F): 1},
}

KEEP_SHAMAN = (18231.2, 58.0859, 20871.6)   # Southwatch Keep's shaman, the row the new ones copy
TOL = 3.0
CAMP_REACH = 40.0   # a camp is there when a mercenary stands within this of its new spot
SPOT_CLEAR = 4.0    # and no NPC within this of the spot itself

# The camps with no healer, each with its new shaman's spot (cmd/campground, worlds/ground of 2026-11-10, both shards'
# NPCs 6+ off): (camp, x, y, z, facing)
SPOTS = [
    ("805 camp of three (19221, 21511)", 19223.7, 54.09, 21507.6, 42),
    ("802/803 camp of four (19587, 20541)", 19592.6, 54.09, 20539.6, 238),
    ("948 pair (20432, 22232)", 20437.5, 54.09, 22234.8, 22),
]

SHARD = {"name": None, "campground": None, "ground": None}


def near(r, p, tol=TOL):
    return math.hypot(r["x_coord"] - p[0], r["z_coord"] - p[2]) <= tol and abs(r["y_coord"] - p[1]) <= tol


def body(r):
    return (r["modelid"] or 0) & 0xFFFFFFFF


def bare(r):
    """A row as every one of them stood: no armour, robe, tint, shield or off hand; no weapon or the scimitar."""
    return (all((r[c] or 0) == 0 for c in ARMOR) and str(r["robe_style"]) == "255" and (r["unk_o"] or 0) == 0
            and (r["shield"] or 0) == 0 and (r["secondary_weapon"] or 0) == 0
            and (r["primary_weapon"] or 0) in (0, SCIMITAR))


def look_for(kind, r):
    after = dict(LOOK[kind])
    if kind == MERC:
        after["primary_weapon"] = r["primary_weapon"] or GREATAXE
    return after


def check_spots(refusals):
    """The spots on the ground again (--campground BIN --ground DIR)."""
    if not SHARD["campground"]:
        return
    qs = [dict(op="check", k=i, w=0, x=x, y=y, z=z) for i, (_, x, y, z, _) in enumerate(SPOTS)]
    p = subprocess.run([SHARD["campground"], "-ground", SHARD["ground"]], capture_output=True, text=True,
                       input="\n".join(json.dumps(q) for q in qs) + "\n")
    if p.returncode != 0:
        refusals.append(f"campground failed: {p.stderr.strip()[-300:]}")
        return
    ans = {a["k"]: a for a in map(json.loads, p.stdout.splitlines())}
    for i, (camp, x, y, z, _) in enumerate(SPOTS):
        a = ans.get(i) or {}
        st = a.get("stand")
        if st is None or abs(st - y) > 1 or a.get("inground") or a.get("embedded") or a.get("env", 0) & 15:
            refusals.append(f"the {camp}'s spot ({x}, {y}, {z}) is not open ground now: {a}")


def plan(db):
    out, lines, refusals = [], [], []
    shard = SHARD["name"]
    rows = tr.rows(db, "SELECT * FROM npcs WHERE world = 0 AND lower(npc_name) IN (?, ?, ?) ORDER BY id",
                   (MERC.lower(), KNIGHT.lower(), SHAMAN.lower()))
    have = {}
    for r in rows:
        have[(r["npc_name"], body(r))] = have.get((r["npc_name"], body(r)), 0) + 1
    short = {k: c for k, c in EXPECT[shard].items() if have.get(k, 0) < c}
    other = sorted(k for k in have if k not in EXPECT[shard])
    if short or other:
        refusals.append(f"{shard}'s rows per (name, body) are not as read on 2026-11-23: "
                        + ", ".join(f"{n} {BODY_NAME.get(b, hex(b))} {c}" for (n, b), c in sorted(have.items()))
                        + " (expected at least " + ", ".join(f"{n} {BODY_NAME[b]} {c}" for (n, b), c in
                                                              sorted(EXPECT[shard].items())) + ")")
    for (n, b), c in sorted(have.items()):
        if c > EXPECT[shard].get((n, b), c):
            lines.append(f"{c - EXPECT[shard][(n, b)]} more {n!r} ({BODY_NAME[b]}) than read on 2026-11-23 "
                         "(a GM's /place or /clone since): dressed as the rest")
    dressed = [r["id"] for r in rows if not bare(r)]
    if dressed:
        refusals.append(f"rows no longer bare (dressed since 2026-11-23?): {dressed}")
    src = [r for r in rows if r["npc_name"] == SHAMAN and near(r, KEEP_SHAMAN)]
    if len(src) != 1:
        refusals.append(f"the keep's shaman at {KEEP_SHAMAN}: {len(src)} rows, not one")
    for t in ("npcclass", "npcfaction", "lootcreature", "factionhits"):
        for n in (KNIGHT, SHAMAN):
            col = "name" if t in ("npcclass", "npcfaction") else "npc_name"
            if not tr.rows(db, f"SELECT 1 FROM {t} WHERE {col} = ?", (n.lower(),)):
                refusals.append(f"no {t} row for {n.lower()!r}: the renamed would lose it")
    if not tr.rows(db, "SELECT 1 FROM classtemplates WHERE name = ?", (HEAL_TEMPLATE,)):
        refusals.append(f"no classtemplates row {HEAL_TEMPLATE!r}")
    tmpl = tr.get(db, "npctemplate", {"name": SHAMAN.lower()})
    if tmpl is not None and tmpl["template"] != HEAL_TEMPLATE:
        refusals.append(f"npctemplate {SHAMAN.lower()!r} is {tmpl['template']!r} already, not ours")
    check_spots(refusals)
    world0 = tr.rows(db, "SELECT id, x_coord, y_coord, z_coord, zone FROM npcs WHERE world = 0 "
                         "AND x_coord BETWEEN 18000 AND 21000 AND z_coord BETWEEN 20000 AND 22600")
    camps = []
    for camp, x, y, z, face in SPOTS:
        members = [r for r in rows if math.hypot(r["x_coord"] - x, r["z_coord"] - z) <= CAMP_REACH]
        if not any(body(r) != OGRE_F for r in members):
            refusals.append(f"the {camp}: no mercenary within {CAMP_REACH} of ({x}, {z})")
            continue
        if any(body(r) == OGRE_F for r in members):
            refusals.append(f"the {camp} has a female ogre now: no new shaman needed")
        taken = [r["id"] for r in world0 if math.hypot(r["x_coord"] - x, r["z_coord"] - z) < SPOT_CLEAR]
        if taken:
            refusals.append(f"the {camp}'s spot ({x}, {z}) is taken: {taken}")
        camps.append((camp, x, y, z, face, min(members, key=lambda r: math.hypot(r["x_coord"] - x, r["z_coord"] - z))))
    if refusals:
        return out, lines, refusals

    # the renames and the looks
    n = {}
    for r in rows:
        kind = KIND_OF_BODY[body(r)]
        after = look_for(kind, r)
        if r["npc_name"] != kind:
            after["npc_name"] = kind
        out.append(("npcs", {"id": r["id"]}, after))
        key = (r["npc_name"], kind)
        n[key] = n.get(key, 0) + 1
    for (was, kind), c in sorted(n.items()):
        lines.append(f"{c:3d} {was!r} -> {kind!r}" if was != kind else f"{c:3d} {kind!r} dressed")
    # the new shamans
    keep = src[0]
    respawn = tr.get(db, "npcrespawn", {"npc_row": keep["id"]})
    nid = db.execute("SELECT max(id) FROM npcs").fetchone()[0] + 1
    for camp, x, y, z, face, m in camps:
        r = dict(keep)
        r.update(look_for(SHAMAN, r))
        r.update({"id": nid, "npc_id": nid, "x_coord": x, "y_coord": y, "z_coord": z, "facing": face, "zone": m["zone"],
                  "dt_kill": None})
        out.append(("npcs", {"id": nid}, r))
        if respawn is not None:
            out.append(("npcrespawn", {"npc_row": nid}, {"npc_row": nid, "seconds": respawn["seconds"],
                        "note": f"as row {keep['id']}'s (the keep's shaman); {SRC}", "source": SRC}))
        lines.append(f"new {SHAMAN!r} #{nid} at ({x}, {y}, {z}) zone {m['zone']}: the {camp} (a copy of #{keep['id']})")
        nid += 1
    # her heal
    out.append(("npctemplate", {"name": SHAMAN.lower()}, {
        "name": SHAMAN.lower(), "template": HEAL_TEMPLATE,
        "note": "the guide's female ogre shamans heal their camp (the owner, 2026-11-23: 'robed healers')",
        "source": SRC}))
    # the renamed keep their trash
    lid = db.execute("SELECT coalesce(max(id), 0) FROM lootpool").fetchone()[0] + 1
    trash = tr.rows(db, "SELECT * FROM lootpool WHERE npc_name = ? ORDER BY id", (MERC.lower(),))
    for kind in (KNIGHT, SHAMAN):
        mine = {(t["category"], t["region"], t["patternid"]) for t in
                tr.rows(db, "SELECT * FROM lootpool WHERE npc_name = ?", (kind.lower(),))}
        for t in trash:
            if (t["category"], t["region"], t["patternid"]) in mine:
                continue
            row = dict(t, id=lid, npc_name=kind.lower(), source=SRC)
            out.append(("lootpool", {"id": lid}, row))
            lines.append(f"lootpool {kind.lower()!r}: item {t['patternid']} ({t['category']}), as the mercenary's #{t['id']}")
            lid += 1
    return out, lines, refusals


def is_applied(db):
    if not tr.table(db, LOG):
        return False
    for tbl, rowkey, after in db.execute(f"SELECT tbl, rowkey, after FROM {LOG}").fetchall():
        now = tr.get(db, tbl, json.loads(rowkey))
        if (now is None) != (after is None) or (now is not None and json.dumps(now, sort_keys=True) != after):
            return False
    return True


def changes(db):
    if is_applied(db):
        return [], [], [], True
    out, lines, refusals = plan(db)
    return out, lines, refusals, False


SPEC = {"log": LOG, "detail": DETAIL,
        "needs": ("npcs", "npcrespawn", "npctemplate", "classtemplates", "lootpool", "npcclass", "npcfaction",
                  "lootcreature", "factionhits"),
        "changes": changes}


def main(argv=None):
    ap = argparse.ArgumentParser(add_help=False)
    ap.add_argument("--shard", required=True, choices=("heirloom", "live"))
    ap.add_argument("--campground")
    ap.add_argument("--ground")
    a, rest = ap.parse_known_args(argv)
    if bool(a.campground) != bool(a.ground):
        print("--campground and --ground go together")
        return 2
    SHARD.update(name=a.shard, campground=a.campground, ground=a.ground)
    return tr.main(SPEC, rest, doc=__doc__)


if __name__ == "__main__":
    sys.exit(main())
