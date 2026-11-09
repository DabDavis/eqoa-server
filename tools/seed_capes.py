#!/usr/bin/env python3
"""The heirloom capes (the user, 2026-11-08: "lets add a new equip slot for heirloom server only. capes for all. there
are many capes we can borrow from for various styles."; 2026-11-09: "add heirloom capes to the genie and heirloom
vendors, also allow dying"; internal/handlers/capes.go, world.capes).

What it writes (a report by default; --apply writes, --undo takes back exactly what the last run wrote):
- **The records** (itempattern): an "Heirloom" cape for each row of STYLES (ids FIRST_ID + style) and of COLOURS (ids
  FIRST_ID + COLOUR_ID on), of wear slot 28 (the cape's, none of CLIENT's), its style in model (a client that draws
  capes draws style N as the cape it cuts out of the body STYLES names) and its colour in color (0 the art's own; a
  COLOURS row the Nightborne Cloak's pale grey art in a colour of its own; an Armor Dyer's dye of the cape goes over
  either), no stats, level 1, every class and race, NO TRADE, worth 0 (the price and the sale), one to a stack, no wear
  (no hit points, so never broken). Named "Heirloom ..." as the other heirlooms are (an import never damps one:
  database.IsImportedGear). A PS2 is never listed one and, given one, has it as a plain item it cannot wear.
- **Where they are sold**:
  - every Heirloom Outfitter sells the records marked for it (STYLES' and COLOURS' last column: two), after the rest of
    its stock. Its list is the buyer's class's (handlers/buyback.go waresForClass) and the client's holds 40, wares and
    buy-back lines together; a ranger's and a rogue's hold 37 today, so two capes leave each a buy-back line, and the
    run refuses any class's list past 40;
  - the genie's "Heirloom capes" (handlers/genie.go, to a client that draws capes) sells every cape: the npcitem list
    STOCK, no NPC's (handlers/capes.go capeStock).

The records and the lists are read at each request: no restart for the data (the code that sells and draws them
comes with world.capes and a new binary). Logged in cape_seed_log; --undo deletes every row it made (a record a
character holds is kept, and said).

    tools/seed_capes.py [--db eqoa.db] [--list] [--apply | --undo] [--first-id 717000]
"""

import argparse
import datetime
import os
import shutil
import sqlite3

LOG = "cape_seed_log"
FIRST_ID = 717000  # 715000-715042 and 716000-716263 hold the kept items and the plane sets
STOCK = 717000  # handlers/capes.go capeStock: the genie's capes' npcitem list, no NPC's
WEAR_CAPE = 28  # handlers/capes.go wearCape
ALL_CLASSES = 0x7FFF  # WAR 0 .. ALC 14
ALL_RACES = 32767  # every race's bit, as the heirlooms' (raceUseForWire keeps the wire's)
OUTFITTER = "Heirloom Outfitter"
LIST_ROOM = 40  # a merchant list's records on the client (handlers invListCap)

# The styles: (style, the record's name, the body its art is cut out of (CHAR.ESF DictID; the client's table, kept here
# for the record), its icon (ITEMICON.ESF: the robes' icons, as the disc's cloaks have them), its description, whether
# the Heirloom Outfitters sell it). The style is itempattern.model, the number the client draws by: never renumber a
# row a character may hold, drop it. Styles 4 and 5 are one body's cape, without and with its fur mantle.
STYLES = [
    (1, "Heirloom Azure Knight's Cape", 0xD1300BD3, 1188988948,
     "A long cape of blue silk that falls to the heels, as the knights of old wore it.", True),
    (2, "Heirloom Nightborne Cloak", 0xCE1B8083, 1394465037,
     "A sweeping cloak with a high collar lined in red, for those who walk by night. It takes a dye well.", True),
    (3, "Heirloom Violet Mantle", 0xD7540FDA, 1414103605,
     "A long mantle of violet cloth, pleated from the shoulders to the hem.", False),
    (4, "Heirloom Dreadlord's Cape", 0xDC3B3450, 1414103605,
     "A heavy maroon cape with a high, stiff collar, as the lords of the Isle of Dread wear them.", False),
    (5, "Heirloom Dreadlord's Mantle", 0xDC3B3450, -1654522420,
     "The Dreadlord's maroon cape under a mantle of dark fur that rides the shoulders.", False),
]

# The colours: (the record's name, its colour 0xRRGGBBAA, its icon, its description, whether the outfitters sell it),
# each the art of COLOUR_STYLE (whose texture is a pale grey, so a colour reads true), at ids FIRST_ID + COLOUR_ID on.
# The icons are the robes' nearest in colour (no robe icon is blue).
COLOUR_STYLE = 2
COLOURS = [
    ("Heirloom Crimson Cloak", 0xB01818FF, 1414103605, "A sweeping cloak with a high collar, dyed a deep crimson.", False),
    ("Heirloom Forest Cloak", 0x2E7D32FF, -1184067867, "A sweeping cloak with a high collar, dyed the green of the deep forest.", False),
    ("Heirloom Royal Cloak", 0x283593FF, 1188988948, "A sweeping cloak with a high collar, dyed a royal blue.", False),
    ("Heirloom Gilded Cloak", 0xC9A227FF, -1669429287, "A sweeping cloak with a high collar, dyed the gold of old coin.", False),
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


def class_lists(db, npcid):
    """An outfitter's list size for each class, as the buyer's class's list counts it (classuse)."""
    uses = [u or 0 for (u,) in db.execute("SELECT p.classuse FROM npcitem i JOIN itempattern p ON p.patternid = i.patternid "
                                          "WHERE i.npcid = ?", (npcid,))]
    return [sum(1 for u in uses if u & (1 << c)) for c in range(15)]


def plan(db, first_id):
    ensure_tables(db)
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("the capes are in this database already: --undo first")
    taken = db.execute("SELECT COUNT(*) FROM itempattern WHERE patternid >= ? AND patternid < ?",
                       (first_id, first_id + 100)).fetchone()[0]
    if taken:
        raise SystemExit(f"{taken} records already hold ids {first_id}-{first_id + 99}: --first-id another block")
    if db.execute("SELECT COUNT(*) FROM npcs WHERE npc_id = ?", (STOCK,)).fetchone()[0] or \
            db.execute("SELECT COUNT(*) FROM npcitem WHERE npcid = ?", (STOCK,)).fetchone()[0]:
        raise SystemExit(f"npcitem list {STOCK} (the genie's capes) is an NPC's or holds rows already")
    styles = [s[0] for s in STYLES]
    if len(set(styles)) != len(styles) or not all(1 <= s < COLOUR_ID for s in styles):
        raise SystemExit(f"the styles must be distinct and 1-{COLOUR_ID - 1}: {styles}")
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    records = [(record(cols, first_id + st, st, name, icon, desc), dict_id, sold)
               for st, name, dict_id, icon, desc, sold in STYLES]
    art = next(d for st, _, d, _, _, _ in STYLES if st == COLOUR_STYLE)
    records += [(record(cols, first_id + COLOUR_ID + i, COLOUR_STYLE, name, icon, desc, color), art, sold)
                for i, (name, color, icon, desc, sold) in enumerate(COLOURS)]
    sold = [rec for rec, _, s in records if s]
    outfitters = rows_of(db, "SELECT id, npc_id FROM npcs WHERE npc_name = ? ORDER BY id", OUTFITTER)
    worst = [0] * 15
    for o in outfitters:
        before = class_lists(db, o["npc_id"])
        worst = [max(w, b + len(sold)) for w, b in zip(worst, before)]
    over = [c for c in range(15) if worst[c] > LIST_ROOM]
    if over:
        raise SystemExit(f"{len(sold)} capes would put class(es) {over} past an outfitter list's {LIST_ROOM} "
                         f"({[worst[c] for c in over]}): mark fewer for the outfitters")
    return dict(records=records, sold=sold, outfitters=outfitters, worst=worst, first_id=first_id)


def report(p, full):
    print(f"{len(p['records'])} heirloom capes at {p['first_id'] + 1}-{p['records'][-1][0]['patternid']} (wear slot "
          f"{WEAR_CAPE}): {len(p['sold'])} on each of the {len(p['outfitters'])} {OUTFITTER}s, every one on the genie's "
          f"capes (npcitem list {STOCK})")
    for rec, dict_id, sold in p["records"]:
        tint = f", colour {rec['color']:08X}" if rec["color"] else ""
        where = "the outfitters and the genie" if sold else "the genie"
        print(f"  {rec['patternid']}: {rec['itemname']!r}, style {rec['model']} (the art of CHAR.ESF {dict_id:#010x})"
              f"{tint}, icon {rec['itemicon']}: {where}")
    print("  the fullest outfitter list a class has with them (of 40, the buy-back lines after): " +
          ", ".join(f"{c}:{n}" for c, n in enumerate(p["worst"])))
    if full:
        for o in p["outfitters"]:
            print(f"  outfitter {o['id']} (catalog {o['npc_id']})")


def apply(db, p):
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    try:
        log = lambda tbl, rid, note="": db.execute(f"INSERT INTO {LOG} (run, tbl, rowid_made, note) VALUES (?, ?, ?, ?)",
                                                   (run, tbl, rid, note))
        for rec, dict_id, _ in p["records"]:
            cols = list(rec)
            db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                       [rec[c] for c in cols])
            log("itempattern", rec["patternid"], f"style {rec['model']}, art of {dict_id:#010x}")
        for o in p["outfitters"]:
            last = db.execute("SELECT COALESCE(MAX(listnumber), -1) FROM npcitem WHERE npcid = ?",
                              (o["npc_id"],)).fetchone()[0]
            for i, rec in enumerate(p["sold"]):
                c = db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                               "VALUES (?, 1, ?, 0, 1, ?)", (o["npc_id"], rec["patternid"], last + 1 + i))
                log("npcitem", c.lastrowid, f"{OUTFITTER} {o['id']}")
        for i, (rec, _, _) in enumerate(p["records"]):
            c = db.execute("INSERT INTO npcitem (npcid, stackleft, patternid, equiploc, location, listnumber) "
                           "VALUES (?, 1, ?, 0, 1, ?)", (STOCK, rec["patternid"], i))
            log("npcitem", c.lastrowid, "the genie's capes")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote {len(p['records'])} capes, {len(p['sold'])} on each of {len(p['outfitters'])} outfitters and all on "
          f"the genie's list (run {run}); read at each request, shown with world.capes on")


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
            elif tbl == "npcs":  # the Cape Tailors of the first version of this tool (2026-11-08)
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
    ap.add_argument("--list", action="store_true", help="every record and outfitter")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--first-id", type=int, default=FIRST_ID, help="the block of 100 ids the records take")
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
    p = plan(db, a.first_id)
    report(p, a.list)
    if a.apply:
        apply(db, p)


if __name__ == "__main__":
    main()
