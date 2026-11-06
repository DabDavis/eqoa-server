#!/usr/bin/env python3
"""The heirlooms and their vendors (the user, 2026-11-06: "we also need test armor vendors for starting cities armor
that scales per level, similar to wow heirloom armor. it will be Best in Slot for each class. also give it a healing
effect per hit like the breastplate of the anointed. the weapons that look like the lvl 60 epics that scale also per
class. also give them full set of jewelry, belts", "the test armor npc will be next to the starting q npc, the cost is 0
tunar", "also add a Armor dyer NPC nearby the hierloom npc also"; internal/handlers/heirloom.go and dyer.go).

What it writes (a report by default; --apply writes, --undo takes back exactly what the last run wrote):
- **The records** (itempattern, ids from 714000): for each class an "Heirloom" piece of every role
  (handlers/heirloom.go's roles): helm, chestguard, bracers, gauntlets, greaves, boots and robe; earring, pendant, ring,
  bracelet and belt; a shield for a class that can wear one; the weapons in the look of the class's level 60 epics
  (EPIC_LOOKS: the class's own where it has one, the epic blade or the casters' staff where it has none). A record has
  no figures of its own: the server gives it, on each character's view, the best of its role the class may wear at
  the wearer's level. Its look (icon, model, colour) is the class's best of its slot at 60 by the same measure
  (#template's templateScore), a weapon's its epic's. NO TRADE, worth 0 (the merchant price and the sale), level 1,
  the class's alone, every race.
- **The table heirlooms**: each record's class and role.
- **The vendors**: an "Heirloom Outfitter" (a merchant: npc_type 0x81) beside every class's first quest giver (the
  1-20 guides' L1 quest of each city and class), selling the set of every class that giver starts, and an "Armor Dyer"
  (0x80) on its other side; both on the line from the giver to the class's start spot (defaultcharacter, open floor a
  few steps off), 2 units to each side, facing as the giver faces, in the body of the nearest merchant of the place
  (the outfitter the nearest, the dyer the next).

NPCs are read at the server's start: apply it with the server stopped, the heirloom code's binary to start after.
Logged in heirloom_seed_log; --undo deletes every row it made (a record a character holds is kept, and said).

    tools/seed_heirlooms.py [--db eqoa.db] [--list] [--apply | --undo]
"""

import argparse
import datetime
import math
import os
import re
import shutil
import sqlite3
import sys

LOG = "heirloom_seed_log"
FIRST_ID = 714000
CLASSES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman", "cleric",
           "magician", "necromancer", "enchanter", "wizard", "alchemist"]
DUAL = {0, 1, 4, 5, 6}  # CLIENT's off hand: warrior, ranger, monk, bard, rogue (ChooseEquipSlot, level 20 and up)

# armour and jewellery: role, its wear slot, the record's name ("Heirloom Chestguard" is entity/wornshield.go's heal)
PIECES = [
    ("head", 1, "Heirloom Helm"), ("chest", 5, "Heirloom Chestguard"), ("bracers", 7, "Heirloom Bracers"),
    ("hands", 19, "Heirloom Gauntlets"), ("legs", 10, "Heirloom Greaves"), ("feet", 11, "Heirloom Boots"),
    ("robe", 2, "Heirloom Robe"), ("ear", 3, "Heirloom Earring"), ("neck", 4, "Heirloom Pendant"),
    ("ring", 8, "Heirloom Ring"), ("bracelet", 6, "Heirloom Bracelet"), ("waist", 9, "Heirloom Belt"),
]
SHIELD = ("shield", 13, "Heirloom Shield")

# the weapons by class: role, wear slot (a dual wielder's main hand either hand), name, the epic record whose look it
# takes (the level 60 epics: the reward quests' and the classes' own; the warrior, necromancer, enchanter and
# alchemist have none, so the epic blade or the casters' staff)
EPIC_LOOKS = {
    0: [("main", 14, "Heirloom Blade", 5379)],
    1: [("main", 14, "Heirloom Axe", 5353), ("ranged", 16, "Heirloom Longbow", 5359)],
    2: [("main", 12, "Heirloom Sword", 1081)],
    3: [("main", 12, "Heirloom Sword", 5379), ("twohand", 15, "Heirloom Bonecrusher", 5381)],
    4: [("main", 14, "Heirloom Claws", 701197), ("twohand", 15, "Heirloom Staff", 5348)],
    5: [("main", 14, "Heirloom Blade", 5368)],
    6: [("main", 14, "Heirloom Dagger", 5351)],
    7: [("twohand", 15, "Heirloom Staff", 701195), ("main", 12, "Heirloom Scimitar", 5386)],
    8: [("twohand", 15, "Heirloom Staff", 701199), ("main", 12, "Heirloom Hammer", 701200)],
    9: [("twohand", 15, "Heirloom Staff", 701193), ("main", 12, "Heirloom Hammer", 701194)],
    10: [("twohand", 15, "Heirloom Staff", 5392)],
    11: [("twohand", 15, "Heirloom Staff", 5390)],
    12: [("twohand", 15, "Heirloom Staff", 5390)],
    13: [("twohand", 15, "Heirloom Staff", 5390)],
    14: [("twohand", 15, "Heirloom Staff", 5392)],
}
DESC = "Heirloom gear for testing: always the best of its kind your class can wear at your level."
OUTFITTER, DYER = "Heirloom Outfitter", "Armor Dyer"
SIDE, ALONG = 2.0, 0.5  # the vendors 2 units to each side of the giver's line to its start spot, halfway along


def score(r):
    v = lambda c: r.get(c) or 0
    stats = sum(v(c) for c in ("str", "sta", "agi", "dex", "wis", "intelligence", "cha"))
    res = sum(v(c) for c in ("PR", "DR", "FR", "CR", "LR", "AR"))
    return v("AC") + 2 * (stats + v("PoT") + v("HoT")) + (v("HPMAX") + v("POWMAX")) // 2 + res // 4


def unfit(r):
    """handlers/heirloom.go's heirloomUnfit: conjured (NO RENT), broken, a prop or test record, or of no known level."""
    if (r.get("rent") or 0) == 0 or (r.get("weapondamage") or 0) > 1000:
        return True
    name = str(r.get("itemname") or "").lower()
    if name.startswith("prop ") or re.search(r"\btest\b", name) or (r.get("levelreq") or 0) < 1:
        return True
    return any((r.get(c) or 0) > 500 for c in ("PoT", "PR", "DR", "FR", "CR", "LR", "AR"))


def rows_of(db, sql, *args):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def best_look(db, cls, slot):
    """The class's best item of the wear slot at 60 (#template's measure), for its look."""
    items = rows_of(db, "SELECT * FROM itempattern WHERE (classuse & ?) != 0 AND levelreq <= 60 AND equipslot = ? "
                        "AND patternid NOT IN (SELECT patternid FROM heirlooms)", 1 << cls, slot)
    items = [r for r in items if not unfit(r)]
    items.sort(key=lambda r: (-score(r), -(r["levelreq"] or 0), r["patternid"]))
    return items[0] if items else None


def ensure_tables(db):
    db.execute("""CREATE TABLE IF NOT EXISTS heirlooms (patternid INTEGER PRIMARY KEY, class INTEGER NOT NULL,
                  role TEXT NOT NULL)""")
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   rowid_made INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '')""")


def givers(db):
    """Every class's first quest giver: the 1-20 guides' L1 quests, by giver, with the classes each starts."""
    out = {}
    for name, src in db.execute("SELECT giver, source FROM quests WHERE source LIKE '%1-20/%/L1 %'"):
        parts = src.split()[1].split("/")
        if len(parts) != 4 or parts[2] not in CLASSES:
            continue
        out.setdefault(name, set()).add(CLASSES.index(parts[2]))
    return out


def place(db, giver, classes):
    """The giver's row and the vendors' places: halfway to its classes' nearest start spot, 2 to each side."""
    g = rows_of(db, "SELECT * FROM npcs WHERE lower(npc_name) = lower(?) ORDER BY id LIMIT 1", giver)
    if not g:
        return None
    g = g[0]
    gx, gy, gz = g["x_coord"], g["y_coord"], g["z_coord"]
    spots = [s for c in classes for s in db.execute(
        "SELECT x, y, z FROM defaultcharacter WHERE tclass = ? AND world = ?", (c, g["world"]))]
    near = sorted(spots, key=lambda s: (s[0] - gx) ** 2 + (s[2] - gz) ** 2)
    if near and math.hypot(near[0][0] - gx, near[0][2] - gz) <= 30:
        sx, sy, sz = near[0]
    else:
        sx, sy, sz = gx + 4, gy, gz  # no start spot near: 4 units off on x
    dx, dz = sx - gx, sz - gz
    d = math.hypot(dx, dz) or 1.0
    mx, my, mz = gx + ALONG * dx, gy + ALONG * (sy - gy), gz + ALONG * dz
    px, pz = -dz / d * SIDE, dx / d * SIDE
    return g, (mx + px, my, mz + pz), (mx - px, my, mz - pz)


def bodies(db, g):
    """The two nearest merchants of the giver's world (npc_type's merchant bit), for the vendors' bodies."""
    ms = rows_of(db, "SELECT * FROM npcs WHERE world = ? AND (npc_type & 1) != 0 AND npc_name NOT IN (?, ?) "
                     "ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?) LIMIT 2",
                 g["world"], OUTFITTER, DYER, g["x_coord"], g["x_coord"], g["z_coord"], g["z_coord"])
    while len(ms) < 2:
        ms.append(g)
    return ms[0], ms[1]


def plan(db):
    ensure_tables(db)
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("the heirlooms are in this database already: --undo first")
    taken = db.execute("SELECT COUNT(*) FROM itempattern WHERE patternid >= ? AND patternid < ?",
                       (FIRST_ID, FIRST_ID + 1000)).fetchone()[0]
    if taken:
        raise SystemExit(f"{taken} records already hold ids {FIRST_ID}-{FIRST_ID + 999}")
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    records, sets, nid = [], {}, FIRST_ID
    for cls in range(len(CLASSES)):
        roles = [(role, slot, name, None) for role, slot, name in PIECES]
        if best_look(db, cls, 13):
            roles.append(SHIELD + (None,))
        roles += [(role, slot, name, look) for role, slot, name, look in EPIC_LOOKS[cls]]
        for role, slot, name, look in roles:
            donor = (rows_of(db, "SELECT * FROM itempattern WHERE patternid = ?", look) or [None])[0] if look else best_look(db, cls, slot)
            if donor is None:
                donor = (rows_of(db, "SELECT * FROM itempattern WHERE equipslot = ? ORDER BY patternid LIMIT 1", slot) or [None])[0]
            if donor is None:
                continue
            r = dict(donor)
            for c in ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
                      "PR", "DR", "FR", "CR", "LR", "AR"):
                if c in r:
                    r[c] = None
            r.update(patternid=nid, itemname=name, itemdesc=DESC, itemcost=0, trade=1, rent=1, levelreq=1, maxstack=1,
                     classuse=1 << cls, raceuse=32767, lore=0, craft=0, equipslot=slot, weapondamage=0)
            for c in ("weaponproc", "fish"):
                if c in r:
                    r[c] = 0
            records.append(({c: r.get(c) for c in cols}, cls, role, donor["patternid"], donor["itemname"]))
            sets.setdefault(cls, []).append(nid)
            nid += 1
    vendors = []
    for giver, classes in sorted(givers(db).items()):
        p = place(db, giver, classes)
        if p is None:
            continue
        g, at_out, at_dye = p
        b_out, b_dye = bodies(db, g)
        stock = [pid for c in sorted(classes) for pid in sets.get(c, [])]
        vendors.append(dict(giver=giver, classes=sorted(classes), g=g, out=(b_out, at_out), dye=(b_dye, at_dye), stock=stock))
    return dict(records=records, vendors=vendors)


def npc_row(cols, body, giver, name, at, npc_type):
    r = {c: body.get(c) for c in cols if c != "id"}
    r.update(npc_name=name, x_coord=round(at[0], 2), y_coord=round(at[1], 2), z_coord=round(at[2], 2),
             facing=giver["facing"], world=giver["world"], zone=giver["zone"], npc_type=npc_type,
             npc_level=max(giver.get("npc_level") or 1, 1), hp=giver.get("hp") or 100, hp_percent=255)
    return r


def report(p, full):
    by = {}
    for rec, cls, role, did, dname in p["records"]:
        by.setdefault(cls, []).append(f"{rec['itemname']} ({role}, slot {rec['equipslot']}, look of {dname} {did})")
    print(f"{len(p['records'])} records from {FIRST_ID}, {len(p['vendors'])} outfitters and as many dyers")
    for cls in sorted(by):
        print(f"  {CLASSES[cls]}: {len(by[cls])}" + (": " + "; ".join(by[cls]) if full else ""))
    if full:
        for v in p["vendors"]:
            print(f"  beside {v['giver']} ({', '.join(CLASSES[c] for c in v['classes'])}): outfitter at "
                  f"{tuple(round(x, 1) for x in v['out'][1])} as {v['out'][0]['npc_name']!r}, dyer at "
                  f"{tuple(round(x, 1) for x in v['dye'][1])} as {v['dye'][0]['npc_name']!r}, {len(v['stock'])} items")


def apply(db, p):
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    ncols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    db.execute("BEGIN IMMEDIATE")
    try:
        log = lambda tbl, rid, note="": db.execute(f"INSERT INTO {LOG} (run, tbl, rowid_made, note) VALUES (?, ?, ?, ?)",
                                                   (run, tbl, rid, note))
        for rec, cls, role, did, dname in p["records"]:
            cols = list(rec)
            db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [rec[c] for c in cols])
            log("itempattern", rec["patternid"], f"{CLASSES[cls]} {role}, look of {dname} {did}")
            db.execute("INSERT INTO heirlooms (patternid, class, role) VALUES (?, ?, ?)", (rec["patternid"], cls, role))
            log("heirlooms", rec["patternid"])
        for v in p["vendors"]:
            for (body, at), name, typ in ((v["out"], OUTFITTER, 0x81), (v["dye"], DYER, 0x80)):
                r = npc_row(ncols, body, v["g"], name, at, typ)
                cols = list(r)
                cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                                 [r[c] for c in cols])
                log("npcs", cur.lastrowid, f"{name} beside {v['giver']}")
                if name == OUTFITTER:
                    for i, pid in enumerate(v["stock"]):
                        c2 = db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                                        "VALUES (?, 1, ?, 0, 1, ?)", (cur.lastrowid, pid, i))
                        log("npcitem", c2.lastrowid)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {len(p['records'])} records and {2 * len(p['vendors'])} NPCs (run {run}); restart the server")


def undo(db):
    ensure_tables(db)
    rows = db.execute(f"SELECT tbl, rowid_made FROM {LOG} ORDER BY seq DESC").fetchall()
    if not rows:
        raise SystemExit("nothing to undo")
    db.execute("BEGIN IMMEDIATE")
    kept = []
    try:
        for tbl, rid in rows:
            if tbl == "npcitem":
                db.execute("DELETE FROM npcitem WHERE itemid = ?", (rid,))
            elif tbl == "npcs":
                db.execute("DELETE FROM npcs WHERE id = ?", (rid,))
            elif tbl == "heirlooms":
                db.execute("DELETE FROM heirlooms WHERE patternid = ?", (rid,))
            elif tbl == "itempattern":
                if db.execute("SELECT COUNT(*) FROM charinventory WHERE patternid = ?", (rid,)).fetchone()[0]:
                    kept.append(rid)
                    continue
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (rid,))
        db.execute(f"DELETE FROM {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {len(rows)} rows" + (f"; kept {len(kept)} records characters hold: {kept}" if kept else ""))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--list", action="store_true", help="every record and vendor")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args()
    if not os.path.isfile(a.db):
        raise SystemExit(f"no database at {a.db}")
    if (a.apply or a.undo) and not a.no_backup:
        stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        shutil.copy2(a.db, f"{a.db}.bak-{stamp}-pre-heirlooms")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=5)
    if a.undo:
        undo(db)
        return
    p = plan(db)
    report(p, a.list)
    if a.apply:
        apply(db, p)


if __name__ == "__main__":
    main()
