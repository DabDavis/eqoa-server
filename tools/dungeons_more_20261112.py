#!/usr/bin/env python3
"""Seven more instanced raids in the dungeons' world (the owner, 2026-11-12: "more instanced raids, same treatment as
Guk/Blackburrow/Runnyeye": Tomb of Kings, Castle Felstar, Collonridge Cemetery, Vox's castle, Toxxulia's lair, Box
Canyon, the Warrens' den). tools/dungeons_20261112.py made the first three and the three tier templates; this adds the
seven as it made those (its functions, imported), each its recipe's dungeon (tools/worlds/dungeons.json's block):

  - npcs, world 14: a copy of each creature of the dungeon, where it stands on its disc world (Tunaria or Odus, by
    SOURCE): the rows inside the dungeon's boxes of its kinds (FAMILY) or named (a capitalised name that is no kind:
    KINDS), never friendly (npc_type 0x80), a quest's seeded row or spawn-only, nor SKIP; levels raised as the first
    three's (trash 60-62, named 63-64, bosses 65: BOSSES); npctemplate "#<row>" -> its tier.
  - itempattern 995037-995120: twelve raid-gear records a dungeon (ITEMS: a tuned raid boss's drop under a name of the
    dungeon's own), loottables "dungeon <key> boss|named", npcloot "#<row>" (a boss 2 + 1, a named 60%, trash 2%).
  - a Dungeon Warden twin at each new entrance (4 to the landing's right, facing it), a copy of Blackburrow's.

Needs tools/dungeons_20261112.py applied (the tier templates). The server reads npcs, templates and loot rows at its
start: run it in a keep restart's before.sh. Refused when applied already (a second run does nothing) or a row it would
write exists; --undo deletes every row it wrote, refused when one changed since (a GM's edit); exact.

    python3 -E -s tools/dungeons_more_20261112.py --db eqoa.db [--apply | --undo] [--recipe tools/worlds/dungeons.json]
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
base.RUN, base.LOG = "dungeons more 20261112", "dungeons_more_1112_log"

ITEM_FIRST = 995037
WARDEN = base.WARDEN
TWIN_OF = (12586.0, 12760.0)  # Blackburrow's twin (world 14), whose row the new twins copy

SOURCE = {"tomb": 0, "felstar": 0, "collonridge": 0, "vox": 0, "toxxulia": 2, "boxcanyon": 0, "warrens": 2}
FAMILY = {
    "tomb": r"^(holciel|hociel|summoned|excavation|fallen treasure|animation|fire elemental|spectral|lich)",
    "felstar": r"^(skeletal|restless|rotting|bone servant|enraged|discarded|flesh fiend|decaying|ghoul|unliving|undead|.*construct)",
    "collonridge": r"^(skeletal|putrid|restless|lesser ghoul|zombie|decaying|dread ?wolf|skeleton|crumbling|maundering|ghostly|unsettled|mourner)",
    "vox": r"^(ice |frost|voxite|icecl|glacial|houndmaster|snowfeather|efreet|frostborn|elite ice|tenacious|war mammoth|.*giant)",
    "toxxulia": r"^(plagued|toxic|swarming)",
    "boxcanyon": r"^(majorkahn|sand giant|inferno drake|torrid drake|fire drake|malign wisp|wretched mummy|defiled mummy|plague griffon"
                 r"|pox griffon|crag spider|singetail|whiptail|calescent|taksehir|cipher)",
    "warrens": r"^(clan kolbok|kolbok|kobold|guardian of|prophet of|minion of krrgar)",
}
# capitalised names that are kinds, not persons
KINDS = r"^(holciel|hociel|excavation team|fire elemental|ice giant|ice goblin|voxite mage|majorkahn giant|taksehir|vigilant spirit" \
        r"|companion spirit|feral spirit|kindred spirit|primal spirit|frenzied spirit|animation)"
# not a dungeon's own though they stand in it: an Odus turtle at the Warrens
SKIP = {"abrosian snapper"}
BOSSES = {"tomb": ["King Polaethius"], "felstar": ["Verghest Felstar"], "collonridge": ["Necromancer Warpheus"],
          "vox": ["Lady Vox"], "toxxulia": ["Toxxulia"], "boxcanyon": ["Skahyir"], "warrens": ["King Bardag"]}
ITEMS = [
    ("tomb", "boss", "Crown of King Polaethius", 6453), ("tomb", "boss", "Regalia of the Tomb", 716001),
    ("tomb", "boss", "Sceptre of the Holciel Court", 5397), ("tomb", "boss", "Signet of Polaethius", 961106),
    ("tomb", "boss", "Shroud of the Buried King", 6477), ("tomb", "boss", "Tomb Warden's Bulwark", 961105),
    ("tomb", "named", "Queen Saersalia's Circlet", 715015), ("tomb", "named", "Lyre-Blade of Nivoel", 5406),
    ("tomb", "named", "Tunic of the Holciel Noble", 6480), ("tomb", "named", "Grave Robber's Leggings", 713096),
    ("tomb", "named", "Usael's Girdle", 715012), ("tomb", "named", "Ring of the Lost Court", 713087),
    ("felstar", "boss", "Felstar's Lordly Plate", 6455), ("felstar", "boss", "Blade of Verghest", 5417),
    ("felstar", "boss", "Felstar Castle Shield", 715013), ("felstar", "boss", "Coil of the Fallen House", 961104),
    ("felstar", "boss", "Robe of the Unquiet Halls", 5072), ("felstar", "boss", "Felstar Ancestral Ring", 961106),
    ("felstar", "named", "Abominus' Bone Cleaver", 713073), ("felstar", "named", "Tunic of Captain Holberg", 6456),
    ("felstar", "named", "Groundskeeper's Lantern", 715014), ("felstar", "named", "Felstar Crypt Leggings", 713096),
    ("felstar", "named", "Tunic of the Bone Servant", 6476), ("felstar", "named", "Ring of the Restless", 713087),
    ("collonridge", "boss", "Warpheus' Death Shroud", 6477), ("collonridge", "boss", "Staff of the Twin Necromancer", 5418),
    ("collonridge", "boss", "Cemetery Warden's Plate", 6454), ("collonridge", "boss", "Grave-Iron Bulwark", 961105),
    ("collonridge", "boss", "Tunic of the Mourner", 6479), ("collonridge", "boss", "Ring of Ancient Death", 961106),
    ("collonridge", "named", "Worpheus' Bone Dagger", 5424), ("collonridge", "named", "Filthmaw's Rotted Mail", 6456),
    ("collonridge", "named", "Bhaal's Burial Girdle", 715012), ("collonridge", "named", "Zarej's Spectral Tunic", 6474),
    ("collonridge", "named", "Collonridge Gravedigger Blade", 713079), ("collonridge", "named", "Eye of the Unsettled", 961103),
    ("vox", "boss", "Lady Vox's Frostscale Plate", 716001), ("vox", "boss", "Icefang of Vox", 961101),
    ("vox", "boss", "Robe of the Frozen Throne", 6475), ("vox", "boss", "Vox's Glacial Bulwark", 715013),
    ("vox", "boss", "Frostscale Coil", 961104), ("vox", "boss", "Ring of the White Dragon", 713087),
    ("vox", "named", "Kraven's Frost Halberd", 713073), ("vox", "named", "Tunic of the Ice Giant Priest", 6478),
    ("vox", "named", "Magnar's Skald Tunic", 6480), ("vox", "named", "Crakill's Frozen Robe", 716005),
    ("vox", "named", "Frostbeard's Leggings", 713096), ("vox", "named", "Snowfeather Talisman", 715015),
    ("toxxulia", "boss", "Toxxulia's Venomscale Plate", 6453), ("toxxulia", "boss", "Fang of Toxxulia", 5424),
    ("toxxulia", "boss", "Plaguehide Tunic", 6472), ("toxxulia", "boss", "Lair Warden's Shield", 961105),
    ("toxxulia", "boss", "Toxic Coil of the Lair", 961104), ("toxxulia", "boss", "Eye of the Plague Mother", 961103),
    ("toxxulia", "named", "Gamboo's Fungal Mace", 5397), ("toxxulia", "named", "Pleglund's Spore Tunic", 6476),
    ("toxxulia", "named", "Swarming Death Sash", 715012), ("toxxulia", "named", "Othmet's Leggings", 713096),
    ("toxxulia", "named", "Plagued Soldier's Blade", 5406), ("toxxulia", "named", "Ring of Toxic Breath", 713087),
    ("boxcanyon", "boss", "Skahyir's Desert Plate", 6455), ("boxcanyon", "boss", "Skahyir's Sand Blade", 1060),
    ("boxcanyon", "boss", "Majorkahn Hide Tunic", 6456), ("boxcanyon", "boss", "Canyon Lord's Bow", 715011),
    ("boxcanyon", "boss", "Taksehir Sunstone Ring", 961106), ("boxcanyon", "boss", "Sandstorm Bulwark", 715013),
    ("boxcanyon", "named", "Pasha's Ceremonial Robe", 5072), ("boxcanyon", "named", "Overlord Frank's Cleaver", 713073),
    ("boxcanyon", "named", "Lantik's Drake Tunic", 6474), ("boxcanyon", "named", "Box Canyon Girdle", 715012),
    ("boxcanyon", "named", "Tunic of the Sand Giant", 6454), ("boxcanyon", "named", "Griffon Plume Talisman", 715014),
    ("warrens", "boss", "King Bardag's Royal Plate", 716001), ("warrens", "boss", "Crown of Clan Kolbok", 6453),
    ("warrens", "boss", "Bardag's Kingsblade", 5417), ("warrens", "boss", "Kolbok Throne Shield", 961105),
    ("warrens", "boss", "Coil of the Warren King", 961104), ("warrens", "boss", "Ring of the Kobold King", 961106),
    ("warrens", "named", "Thulgrr's Shaman Tunic", 6479), ("warrens", "named", "Krrgar's Minion Mace", 5418),
    ("warrens", "named", "Tunic of the Kolbok Elite", 6472), ("warrens", "named", "Warren Hunter's Bow", 715011),
    ("warrens", "named", "Kolbok Jailer's Girdle", 715012), ("warrens", "named", "Prophet's Eye of Rolfron", 961103),
]


def select(db, d):
    """The source world's rows of dungeon d, each with its tier."""
    spawnonly = {r[0].lower() for r in db.execute("SELECT name FROM npcspawnonly")}
    seeded = {r[0] for r in db.execute("SELECT npc_row FROM quest_npc_seed_rows")}
    seen, rows = set(), []
    for b in d["boxes"]:
        x0, z0, x1, z1 = min(b[0], b[2]), min(b[1], b[3]), max(b[0], b[2]), max(b[1], b[3])
        cur = db.execute("""SELECT * FROM npcs WHERE world = ? AND x_coord BETWEEN ? AND ? AND z_coord BETWEEN ? AND ?
                            ORDER BY id""", (SOURCE[d["key"]], x0, x1, z0, z1))
        cols = [c[0] for c in cur.description]
        for r in cur.fetchall():
            r = dict(zip(cols, r))
            n = r["npc_name"]
            if r["id"] in seen or r["npc_type"] & 0x80 or n.lower() in spawnonly or f"#{r['id']}" in spawnonly or \
                    r["id"] in seeded or n.lower() in SKIP:
                continue
            named = n[:1].isupper() and not re.match(KINDS, n, re.I)
            if not (named or re.match(FAMILY[d["key"]], n, re.I)):
                continue
            seen.add(r["id"])
            rows.append((r, "boss" if n in BOSSES[d["key"]] else "named" if named else "trash"))
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
        base.undo(db)
        return
    base.ensure_log(db)
    db.commit()
    if db.execute(f"SELECT 1 FROM {base.LOG} WHERE run = ? AND undone = '' LIMIT 1", (base.RUN,)).fetchone():
        print("applied already: nothing done")
        return
    world, all_dungeons = base.recipe_dungeons(a.recipe)
    dungeons = [d for d in all_dungeons if d["key"] in SOURCE]
    if sorted(d["key"] for d in dungeons) != sorted(SOURCE):
        sys.exit(f"the recipe has {[d['key'] for d in dungeons]} of {sorted(SOURCE)}: nothing done")
    for t in base.TEMPLATES:
        if not db.execute("SELECT 1 FROM classtemplates WHERE name = ?", (t,)).fetchone():
            sys.exit(f"classtemplates {t!r} is missing (tools/dungeons_20261112.py first): nothing done")
    last = ITEM_FIRST + len(ITEMS) - 1
    if db.execute("SELECT count(*) FROM itempattern WHERE patternid BETWEEN ? AND ?", (ITEM_FIRST, last)).fetchone()[0]:
        sys.exit(f"itempattern {ITEM_FIRST}-{last} is taken: nothing done")
    for d in dungeons:
        if db.execute("SELECT count(*) FROM loottables WHERE tablename LIKE ?", (f"dungeon {d['key']} %",)).fetchone()[0]:
            sys.exit(f"loottables 'dungeon {d['key']} ...' exist already: nothing done")
    twin = db.execute("SELECT id FROM npcs WHERE npc_name = ? AND world = ? AND abs(x_coord - ?) < 1 AND abs(z_coord - ?) < 1",
                      (WARDEN, world, *TWIN_OF)).fetchone()
    if twin is None:
        sys.exit(f"no {WARDEN} twin at Blackburrow's landing {TWIN_OF} (tools/dungeons_landing_20261112.py first): nothing done")
    now = base.now_s()
    t0 = time.time()
    cols = [c[1] for c in db.execute("PRAGMA table_info(npcs)") if c[1] != "id"]
    icols = [c[1] for c in db.execute("PRAGMA table_info(itempattern)")]
    summary = []
    db.execute("BEGIN")
    pid = ITEM_FIRST
    for key, tier, name, bid in ITEMS:
        b = base.rowd(db, "itempattern", "patternid = ?", (bid,))
        if b is None or len(name) > 32:
            sys.exit(f"base item {bid} ({name}) missing, or the name over 32: nothing done")
        dname = next(d["name"] for d in dungeons if d["key"] == key)
        v = dict(b)
        v["patternid"], v["itemname"] = pid, name
        v["itemdesc"] = f"Raid gear of {dname}'s instance, as strong as {b['itemname']}."
        base.insert(db, now, "itempattern", {c: v[c] for c in icols})
        base.insert(db, now, "loottables", {"tablename": f"dungeon {key} {tier}", "patternid": pid, "weight": 1, "minlevel": 1,
                                            "maxlevel": 100, "source": base.RUN})
        pid += 1
    for d in dungeons:
        rows = select(db, d)
        lv = base.raid_levels(rows)
        tiers = {"trash": 0, "named": 0, "boss": 0}
        for r, tier in rows:
            db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (r["id"],))
            nid = db.execute("SELECT last_insert_rowid()").fetchone()[0]
            db.execute("UPDATE npcs SET world = ?, npc_level = ?, npc_id = 0, dt_kill = NULL WHERE id = ?", (world, lv[r["id"]], nid))
            base.wrote(db, now, "npcs", {"id": nid})
            base.insert(db, now, "npctemplate", {"name": f"#{nid}", "template": f"dungeon {tier}",
                                                 "note": f"{d['name']}'s {tier}: {r['npc_name']} (world {SOURCE[d['key']]} row {r['id']}, level {r['npc_level']} there)",
                                                 "source": base.RUN})
            for suffix, chance, rolls in base.LOOT[tier]:
                base.insert(db, now, "npcloot", {"npc_name": f"#{nid}", "tablename": f"dungeon {d['key']} {suffix}", "chance": chance,
                                                 "rolls": rolls, "note": f"{d['name']}'s {tier} {r['npc_name']}", "source": base.RUN})
            tiers[tier] += 1
        bosses = sorted({r["npc_name"] for r, t in rows if t == "boss"})
        if bosses != sorted(BOSSES[d["key"]]):
            sys.exit(f"{d['name']}: bosses found {bosses}, expected {BOSSES[d['key']]}: nothing done")
        named = sorted({r["npc_name"] for r, t in rows if t == "named"})
        summary.append(f"{d['name']}: {len(rows)} creatures ({tiers['trash']} trash L60-62, {tiers['named']} named L63-64, "
                       f"{tiers['boss']} boss L65: {', '.join(bosses)}); named: {', '.join(named)}")
        ex, ey, ez, ef = d["entrance"]
        h = ef * 2 * math.pi / 256
        wx, wz = ex + 4 * math.cos(h), ez - 4 * math.sin(h)
        db.execute(f"INSERT INTO npcs ({', '.join(cols)}) SELECT {', '.join(cols)} FROM npcs WHERE id = ?", (twin[0],))
        nid = db.execute("SELECT last_insert_rowid()").fetchone()[0]
        db.execute("UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ?, facing = ? WHERE id = ?",
                   (round(wx, 3), ey, round(wz, 3), base.facing_byte(wx, wz, ex, ez), nid))
        base.wrote(db, now, "npcs", {"id": nid})
        summary.append(f"  npcs {nid}: {WARDEN} at {d['name']}'s entrance ({wx:.2f}, {ey}, {wz:.2f})")
    n = db.execute(f"SELECT count(*) FROM {base.LOG} WHERE run = ? AND undone = ''", (base.RUN,)).fetchone()[0]
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
