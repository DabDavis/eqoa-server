#!/usr/bin/env python3
"""The capes and their tailors (the user, 2026-11-08: "lets add a new equip slot for heirloom server only. capes for
all. there are many capes we can borrow from for various styles."; internal/handlers/capes.go, world.capes).

What it writes (a report by default; --apply writes, --undo takes back exactly what the last run wrote):
- **The records** (itempattern, ids FIRST_ID + style): one cape for each row of STYLES, of wear slot 28 (the cape's,
  none of CLIENT's), its style in model (a client that draws capes draws style N as the cape it cuts out of the body
  STYLES names) and colour 0 (the art's own colours; a record's colour 0xRRGGBBAA tints it), no stats, level 1, every
  class and race, NO TRADE, worth 0 (the price and the sale), one to a stack, no wear (no hit points, so never
  broken). A PS2 is sent one as a plain item it cannot wear. With --colours (off unless asked), COLOURS too (ids
  FIRST_ID + 20 on): the Nightborne Cloak's art (style 2, whose texture is a pale grey, so a tint reads true) in a
  colour of its own each.
- **The tailors**: a "Cape Tailor" (a merchant: npc_type 0x81, as the outfitters) beside the Heirloom Outfitters of
  each place (outfitters within CLUSTER of each other are one place: up to four class givers stand together, each with
  its outfitter and dyer), beside the first of them: PAST units beyond it on the line from its Armor Dyer (else along
  x), or round it a quarter turn at a time to the first spot no NPC stands within CLEAR of; facing as it faces, in its
  world and zone, in the body of the nearest merchant of the place whose body is neither its outfitter's nor its
  dyer's; selling every cape (npcitem).

Why not the outfitters' own lists: an outfitter's list is the buyer's class's (handlers/buyback.go waresForClass) and
the client holds 40 records; a ranger's and a rogue's hold 37 today and a druid's 35, so six capes would cut the last
three, three and one of theirs (handlers/buyback.go waresFor keeps the first 40).

NPCs are read at the server's start: apply it with the server stopped, or in a keep swap's before.sh
(tools/swap_keep.sh), the capes code's binary starting after with world.capes on. The records alone are read at each
request. Logged in cape_seed_log; --undo deletes every row it made (a record a character holds is kept, and said).

    tools/seed_capes.py [--db eqoa.db] [--list] [--colours] [--apply | --undo] [--first-id 717000]
"""

import argparse
import datetime
import math
import os
import shutil
import sqlite3

LOG = "cape_seed_log"
FIRST_ID = 717000  # 715000-715042 and 716000-716263 hold the kept items and the plane sets
WEAR_CAPE = 28  # handlers/capes.go wearCape
ALL_CLASSES = 0x7FFF  # WAR 0 .. ALC 14
ALL_RACES = 32767  # every race's bit, as the heirlooms' (raceUseForWire keeps the wire's)
OUTFITTER, DYER, TAILOR = "Heirloom Outfitter", "Armor Dyer", "Cape Tailor"
PAST = 2.0  # the tailor this far beyond the outfitter, away from the dyer
DYER_NEAR = 8.0  # a dyer within this of an outfitter is its own (the seed put them 4 apart)
CLUSTER = 20.0  # outfitters this close are one place, with one tailor
CLEAR = 1.2  # no NPC this close to a tailor's spot

# The styles: (style, the record's name, the body its art is cut out of (CHAR.ESF DictID; the client's table, kept here
# for the record), its icon (ITEMICON.ESF: the robes' icons, as the disc's cloaks have them), its description). The
# style is itempattern.model, the number the client draws by: never renumber a row a character may hold, drop it.
# Styles 4 and 5 are one body's cape, without and with its fur mantle over the shoulders.
STYLES = [
    (1, "Azure Knight's Cape", 0xD1300BD3, 1188988948,
     "A long cape of blue silk that falls to the heels, as the knights of old wore it."),
    (2, "Nightborne Cloak", 0xCE1B8083, 1394465037,
     "A sweeping black cloak with a high collar lined in red, for those who walk by night."),
    (3, "Violet Mantle", 0xD7540FDA, 1414103605,
     "A long mantle of violet cloth, pleated from the shoulders to the hem."),
    (4, "Dreadlord's Cape", 0xDC3B3450, 1414103605,
     "A heavy maroon cape with a high, stiff collar, as the lords of the Isle of Dread wear them."),
    (5, "Dreadlord's Mantle", 0xDC3B3450, -1654522420,
     "The Dreadlord's maroon cape under a mantle of dark fur that rides the shoulders."),
]

# The colours (--colours): (the record's name, its colour 0xRRGGBBAA, its icon, its description), each the art of
# COLOUR_STYLE, at ids FIRST_ID + 20 on. The icons are the robes' nearest in colour (no robe icon is blue).
COLOUR_STYLE = 2
COLOURS = [
    ("Crimson Cloak", 0xB01818FF, 1414103605, "A sweeping cloak with a high collar, dyed a deep crimson."),
    ("Forest Cloak", 0x2E7D32FF, -1184067867, "A sweeping cloak with a high collar, dyed the green of the deep forest."),
    ("Royal Cloak", 0x283593FF, 1188988948, "A sweeping cloak with a high collar, dyed a royal blue."),
    ("Gilded Cloak", 0xC9A227FF, -1669429287, "A sweeping cloak with a high collar, dyed the gold of old coin."),
]
COLOUR_ID = 20  # the colours' ids: FIRST_ID + COLOUR_ID on


def rows_of(db, sql, *args):
    cur = db.execute(sql, args)
    cols = [d[0] for d in cur.description]
    return [dict(zip(cols, r)) for r in cur.fetchall()]


def ensure_tables(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   rowid_made INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '')""")


def record(cols, pid, style, name, icon, desc, color=0):
    r = dict(patternid=pid, itemcost=0, unk1=0, itemicon=icon, unk2=0, equipslot=WEAR_CAPE, unk3=0,
             trade=1, rent=1, unk4=0, attacktype=0, weapondamage=0, unk5=0, levelreq=1, maxstack=1, maxhp=0,
             duration=0, classuse=ALL_CLASSES, raceuse=ALL_RACES, procanim=0, lore=0, unk6=0, craft=0, itemname=name,
             itemdesc=desc, model=style, color=color, weaponproc=0, fish=0)
    return {c: r.get(c) for c in cols}  # the stats NULL


def tailor_place(db, o, taken):
    """The tailor's place beside outfitter o: PAST beyond it on the line from its dyer (else along x), or that turned a
    quarter at a time, the first spot no NPC (nor a tailor placed before, taken) stands within CLEAR of; the first
    spot when none is clear."""
    dyers = rows_of(db, "SELECT x_coord, z_coord FROM npcs WHERE npc_name = ? AND world = ? "
                        "ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?) LIMIT 1",
                    DYER, o["world"], o["x_coord"], o["x_coord"], o["z_coord"], o["z_coord"])
    ux, uz = 1.0, 0.0
    if dyers:
        ex, ez = o["x_coord"] - dyers[0]["x_coord"], o["z_coord"] - dyers[0]["z_coord"]
        d = math.hypot(ex, ez)
        if 0 < d <= DYER_NEAR:
            ux, uz = ex / d, ez / d
    spots = [(o["x_coord"] + rx * PAST, o["z_coord"] + rz * PAST) for rx, rz in ((ux, uz), (-uz, ux), (uz, -ux), (-ux, -uz))]
    for x, z in spots:
        near = db.execute("SELECT COUNT(*) FROM npcs WHERE world = ? AND (x_coord - ?) * (x_coord - ?) + "
                          "(z_coord - ?) * (z_coord - ?) < ?", (o["world"], x, x, z, z, CLEAR * CLEAR)).fetchone()[0]
        if near == 0 and all(w != o["world"] or math.hypot(x - tx, z - tz) >= CLEAR for w, tx, tz in taken):
            return x, o["y_coord"], z
    x, z = spots[0]
    return x, o["y_coord"], z


def places(outfitters):
    """The outfitters by place: each group those within CLUSTER of another of it, in one world, by id."""
    groups, seen = [], set()
    for o in outfitters:
        if o["id"] in seen:
            continue
        group, todo = [], [o]
        seen.add(o["id"])
        while todo:
            a = todo.pop()
            group.append(a)
            for b in outfitters:
                if b["id"] not in seen and b["world"] == a["world"] and \
                        math.hypot(b["x_coord"] - a["x_coord"], b["z_coord"] - a["z_coord"]) < CLUSTER:
                    seen.add(b["id"])
                    todo.append(b)
        groups.append(sorted(group, key=lambda r: r["id"]))
    return groups


def body(db, o):
    """The nearest merchant of the outfitter's world in a body neither it nor its dyer wears, else in one it does not
    wear (a town whose merchants have two bodies, Halas's barbarians' and trolls'), else the outfitter."""
    dyer = rows_of(db, "SELECT modelid FROM npcs WHERE npc_name = ? AND world = ? "
                       "ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?) LIMIT 1",
                   DYER, o["world"], o["x_coord"], o["x_coord"], o["z_coord"], o["z_coord"])
    worn = {o["modelid"]} | {d["modelid"] for d in dyer}
    ms = rows_of(db, "SELECT * FROM npcs WHERE world = ? AND (npc_type & 1) != 0 AND npc_name NOT IN (?, ?, ?) "
                     "AND modelid IS NOT NULL AND modelid != 0 "
                     "ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?) LIMIT 40",
                 o["world"], OUTFITTER, DYER, TAILOR, o["x_coord"], o["x_coord"], o["z_coord"], o["z_coord"])
    return next((m for m in ms if m["modelid"] not in worn), next((m for m in ms if m["modelid"] != o["modelid"]), o))


def plan(db, first_id, colours=False):
    ensure_tables(db)
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("the capes are in this database already: --undo first")
    taken = db.execute("SELECT COUNT(*) FROM itempattern WHERE patternid >= ? AND patternid < ?",
                       (first_id, first_id + 100)).fetchone()[0]
    if taken:
        raise SystemExit(f"{taken} records already hold ids {first_id}-{first_id + 99}: --first-id another block")
    styles = [s[0] for s in STYLES]
    if len(set(styles)) != len(styles) or not all(1 <= s < COLOUR_ID for s in styles):
        raise SystemExit(f"the styles must be distinct and 1-{COLOUR_ID - 1}: {styles}")
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    records = [(record(cols, first_id + style, style, name, icon, desc), dict_id)
               for style, name, dict_id, icon, desc in STYLES]
    if colours:
        art = next(d for st, _, d, _, _ in STYLES if st == COLOUR_STYLE)
        records += [(record(cols, first_id + COLOUR_ID + i, COLOUR_STYLE, name, icon, desc, color), art)
                    for i, (name, color, icon, desc) in enumerate(COLOURS)]
    tailors, taken = [], []
    for group in places(rows_of(db, "SELECT * FROM npcs WHERE npc_name = ? ORDER BY id", OUTFITTER)):
        o = group[0]
        at = tailor_place(db, o, taken)
        taken.append((o["world"], at[0], at[2]))
        tailors.append(dict(outfitter=o, place=len(group), at=at, body=body(db, o)))
    return dict(records=records, tailors=tailors, first_id=first_id, colours=colours)


def npc_row(cols, body, o, at):
    r = {c: body.get(c) for c in cols if c != "id"}
    r.update(npc_name=TAILOR, x_coord=round(at[0], 2), y_coord=round(at[1], 2), z_coord=round(at[2], 2),
             facing=o["facing"], world=o["world"], zone=o["zone"], npc_type=0x81, npc_level=o.get("npc_level") or 1,
             hp=o.get("hp") or 100, hp_percent=255, dt_kill=None)
    return r


def report(p, full):
    print(f"{len(p['records'])} capes at {p['first_id'] + 1}-{p['records'][-1][0]['patternid']} (wear slot {WEAR_CAPE}), "
          f"{len(p['tailors'])} tailors, one by the {OUTFITTER}s of each place "
          f"({sum(t['place'] for t in p['tailors'])} outfitters)")
    for rec, dict_id in p["records"]:
        tint = f", colour {rec['color']:08X}" if rec["color"] else ""
        print(f"  {rec['patternid']}: {rec['itemname']!r}, style {rec['model']} (the art of CHAR.ESF {dict_id:#010x})"
              f"{tint}, icon {rec['itemicon']}")
    if not p["colours"]:
        print(f"  (--colours, off: also the {', '.join(n for n, _, _, _ in COLOURS)}, the style {COLOUR_STYLE} art "
              f"in a colour each, ids {p['first_id'] + COLOUR_ID}-{p['first_id'] + COLOUR_ID + len(COLOURS) - 1})")
    if full:
        for t in p["tailors"]:
            o = t["outfitter"]
            print(f"  beside the outfitter {o['id']} (world {o['world']}, {o['x_coord']:.1f}, {o['z_coord']:.1f}; "
                  f"{t['place']} at the place): "
                  f"at {tuple(round(x, 1) for x in t['at'])} in {t['body']['npc_name']!r}'s body "
                  f"({t['body']['modelid'] & 0xFFFFFFFF:#010x})")


def apply(db, p):
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    ncols = [r[1] for r in db.execute("PRAGMA table_info(npcs)")]
    db.execute("BEGIN IMMEDIATE")
    try:
        log = lambda tbl, rid, note="": db.execute(f"INSERT INTO {LOG} (run, tbl, rowid_made, note) VALUES (?, ?, ?, ?)",
                                                   (run, tbl, rid, note))
        for rec, dict_id in p["records"]:
            cols = list(rec)
            db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [rec[c] for c in cols])
            log("itempattern", rec["patternid"], f"style {rec['model']}, art of {dict_id:#010x}")
        for t in p["tailors"]:
            r = npc_row(ncols, t["body"], t["outfitter"], t["at"])
            cols = list(r)
            cur = db.execute(f"INSERT INTO npcs ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                             [r[c] for c in cols])
            # npc_id is the server's NPCID, the key of a merchant's catalog (npcitem.npcid): the row's own, never the
            # body donor's (as tools/seed_heirlooms.py)
            if "npc_id" in ncols:
                db.execute("UPDATE npcs SET npc_id = id WHERE id = ?", (cur.lastrowid,))
            log("npcs", cur.lastrowid, f"{TAILOR} beside outfitter {t['outfitter']['id']}")
            for i, (rec, _) in enumerate(p["records"]):
                c2 = db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                                "VALUES (?, 1, ?, 0, 1, ?)", (cur.lastrowid, rec["patternid"], i))
                log("npcitem", c2.lastrowid)
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {len(p['records'])} capes and {len(p['tailors'])} tailors (run {run}); "
          "restart the server (NPCs are read at its start), with world.capes on")


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
    ap.add_argument("--list", action="store_true", help="every record and tailor")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--first-id", type=int, default=FIRST_ID, help="the block of 100 ids the records take")
    ap.add_argument("--colours", action="store_true", help=f"also the colour variants of style {COLOUR_STYLE} (COLOURS)")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args()
    if not os.path.isfile(a.db):
        raise SystemExit(f"no database at {a.db}")
    if (a.apply or a.undo) and not a.no_backup:
        stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        shutil.copy2(a.db, f"{a.db}.bak-{stamp}-pre-capes")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=5)
    if a.undo:
        undo(db)
        return
    p = plan(db, a.first_id, a.colours)
    report(p, a.list)
    if a.apply:
        apply(db, p)


if __name__ == "__main__":
    main()
