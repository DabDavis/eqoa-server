#!/usr/bin/env python3
"""The familiars' first: Healbot (the GM Jibrish's petition 1445, 2026-11-10: "On the genie heirloom vendor add a familiar
pet people can buy. They can resummon the pet as much as they want, its a 2 second cast time on item use, however they
can only have 1 familiar at a time ... This particular one should share a model with the gnome tinker pet. He should be
called "Healbot". He is immune to all resists, regens 2k health a tick ... when the player who summoned him falls below
75% health, he will cast an instant cast 600 hp heal. He will do this every 6 seconds until the player is above 75%
health ... Pet should have 100k health."; internal/handlers/familiar.go, internal/entity/familiar.go).

What it writes (a report by default; --apply writes, --undo takes back what the last run wrote):
- **the item's record** (itempattern 715060, the kept items' next id after the Rock of Freeport's 715050):
  "Healbot's Cog", worn nowhere, CLIENT's gem type (procanim 7: the item menu's Use and the HUD's gem quick slot, as the
  fang's and the stone's), NO TRADE (trade 1), LORE, worth 0 (as every heirloom), a stack of 1, level 1, every class and
  race, 80 DUR, on the Small Gear Wheel's icon (700368: a gear wheel); the server never spends it;
- **its familiars row**: Healbot, in the Invention's body (the gnome's tinker pet: petstats 901789's, 0xFBABEB00, the
  clockwork gnome Klick'Anon's guard models wear), size 1, 100,000 hit points, 2,000 a regeneration tick, a 2 s cast in
  the Invention's own visual when its spelleffect row has one (else a buff's stand-in), a heal of 600 below 75% of its
  owner's hit points at most every 6 s (a heal's stand-in visual);
- **the Genie's stock**: the item in every Heirloom Outfitter's npcitem list, which is what a genie sells its master
  (handlers/genie.go's shopOf: an outfitter's list), last on it.
Data only; no restart (an item's record reaches a client at its login, the outfitters' lists at each open, the
familiars table within a minute). Logged in familiars_log; --undo deletes the record, its row, its npcitem rows and every
Cog a character holds (the record gone, they would name nothing).

    tools/seed_familiars.py [--db eqoa.db] [--list] [--apply | --undo]
"""

import argparse
import datetime
import os
import sqlite3

LOG = "familiars_log"
ITEM_ID = 715060
NAME = "Healbot's Cog"
DESC = ("A gnomish cog that winds up Healbot, a clockwork familiar who heals you while you are hurt. Use it to call him, "
        "a 2 second cast, as often as you like: one familiar at a time. It never wears out.")
LOOK = 700368       # the Small Gear Wheel: its icon (192051396)
BODY = 4222348032   # 0xFBABEB00: the Invention's (petstats 901789), the gnome's tinker pet
INVENTION = 901789  # its spell, for the cast's visual
FAMILIAR = dict(name="Healbot", model=BODY, size=1.0, hp=100000, regen=2000, cast_ms=2000, heal=600, heal_below=75,
                heal_every_ms=6000, heal_fx=0)
OUTFITTER = "Heirloom Outfitter"


def ensure(db):
    db.execute(f"""CREATE TABLE IF NOT EXISTS {LOG} (seq INTEGER PRIMARY KEY, run TEXT NOT NULL, tbl TEXT NOT NULL,
                   rowkey INTEGER NOT NULL, action TEXT NOT NULL)""")
    db.execute("""CREATE TABLE IF NOT EXISTS familiars (patternid INTEGER PRIMARY KEY, name TEXT NOT NULL,
                  model INTEGER NOT NULL DEFAULT 0, size REAL NOT NULL DEFAULT 1, hp INTEGER NOT NULL DEFAULT 100000,
                  regen INTEGER NOT NULL DEFAULT 0, cast_ms INTEGER NOT NULL DEFAULT 2000, cast_fx INTEGER NOT NULL DEFAULT 0,
                  heal INTEGER NOT NULL DEFAULT 0, heal_below INTEGER NOT NULL DEFAULT 75,
                  heal_every_ms INTEGER NOT NULL DEFAULT 6000, heal_fx INTEGER NOT NULL DEFAULT 0,
                  note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")


def plan(db):
    if db.execute(f"SELECT COUNT(*) FROM {LOG}").fetchone()[0]:
        raise SystemExit("Healbot is in this database already: --undo first")
    if db.execute("SELECT COUNT(*) FROM itempattern WHERE patternid = ? OR itemname = ?", (ITEM_ID, NAME)).fetchone()[0]:
        raise SystemExit(f"a record {ITEM_ID} or named {NAME!r} exists")
    if db.execute("SELECT COUNT(*) FROM familiars WHERE patternid = ?", (ITEM_ID,)).fetchone()[0]:
        raise SystemExit(f"a familiars row for {ITEM_ID} exists")
    cols = [r[1] for r in db.execute("PRAGMA table_info(itempattern)")]
    look = db.execute(f"SELECT {', '.join(cols)} FROM itempattern WHERE patternid = ?", (LOOK,)).fetchone()
    if look is None:
        raise SystemExit(f"no record {LOOK} for the cog's look")
    r = dict(zip(cols, look))
    for c in ("str", "sta", "agi", "dex", "wis", "intelligence", "cha", "HPMAX", "POWMAX", "PoT", "HoT", "AC",
              "PR", "DR", "FR", "CR", "LR", "AR", "weaponproc", "fish"):
        if c in r:
            r[c] = 0
    r.update(patternid=ITEM_ID, itemname=NAME, itemdesc=DESC, itemcost=0, equipslot=-1, trade=1, rent=1, levelreq=1,
             maxstack=1, maxhp=0, duration=80, classuse=32767, raceuse=32767, procanim=7, lore=1, craft=0, attacktype=0,
             weapondamage=0, unk5=0, model=0)
    fx = db.execute("SELECT fxdict FROM spelleffect WHERE spellid = ?", (INVENTION,)).fetchone()
    fam = dict(FAMILIAR, cast_fx=(fx[0] if fx else 0))
    outfitters = db.execute("""SELECT n.npc_id, COALESCE(MAX(i.listnumber), -1) FROM npcs n LEFT JOIN npcitem i ON i.npcid = n.npc_id
                               WHERE n.npc_name = ? GROUP BY n.npc_id ORDER BY n.npc_id""", (OUTFITTER,)).fetchall()
    return dict(record={c: r.get(c) for c in cols}, familiar=fam, outfitters=outfitters)


def report(p, full):
    f = p["familiar"]
    print(f"the record {ITEM_ID} {NAME!r} (icon {p['record']['itemicon']}); {f['name']}: body {f['model']:#x} size {f['size']}, "
          f"{f['hp']} HP, {f['regen']} a tick, a {f['cast_ms']} ms cast (visual {f['cast_fx']:#x}), heals {f['heal']} below "
          f"{f['heal_below']}% every {f['heal_every_ms']} ms; in {len(p['outfitters'])} {OUTFITTER}s' stock")
    if full:
        for npc, last in p["outfitters"]:
            print(f"  npcitem list {npc}: at {last + 1}")


def apply(db, p):
    run = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
    db.execute("BEGIN IMMEDIATE")
    try:
        log = lambda tbl, key, action: db.execute(f"INSERT INTO {LOG} (run, tbl, rowkey, action) VALUES (?, ?, ?, ?)",
                                                  (run, tbl, key, action))
        rec = p["record"]
        cols = list(rec)
        db.execute(f"INSERT INTO itempattern ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})", [rec[c] for c in cols])
        log("itempattern", ITEM_ID, "insert")
        f = p["familiar"]
        db.execute("""INSERT INTO familiars (patternid, name, model, size, hp, regen, cast_ms, cast_fx, heal, heal_below,
                      heal_every_ms, heal_fx, note, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'petition 1445')""",
                   (ITEM_ID, f["name"], f["model"], f["size"], f["hp"], f["regen"], f["cast_ms"], f["cast_fx"], f["heal"],
                    f["heal_below"], f["heal_every_ms"], f["heal_fx"], "the GM Jibrish's petition 1445: Healbot"))
        log("familiars", ITEM_ID, "insert")
        for npc, last in p["outfitters"]:
            cur = db.execute("""INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
                                VALUES (?, 0, 0, 0, ?, 0, 1, ?)""", (npc, ITEM_ID, last + 1))
            log("npcitem", cur.lastrowid, "insert")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"wrote it (run {run}): no restart needed")


def undo(db):
    rows = db.execute(f"SELECT tbl, rowkey FROM {LOG} ORDER BY seq DESC").fetchall()
    if not rows:
        raise SystemExit("nothing to undo")
    db.execute("BEGIN IMMEDIATE")
    try:
        held = db.execute("DELETE FROM charinventory WHERE patternid = ?", (ITEM_ID,)).rowcount
        for tbl, key in rows:
            if tbl == "npcitem":
                db.execute("DELETE FROM npcitem WHERE itemid = ?", (key,))
            elif tbl == "familiars":
                db.execute("DELETE FROM familiars WHERE patternid = ?", (key,))
            elif tbl == "itempattern":
                db.execute("DELETE FROM itempattern WHERE patternid = ?", (key,))
        db.execute(f"DELETE FROM {LOG}")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {len(rows)} rows and {held} cogs characters held")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--list", action="store_true")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--undo", action="store_true")
    ap.add_argument("--no-backup", action="store_true")
    a = ap.parse_args()
    if not os.path.isfile(a.db):
        raise SystemExit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, isolation_level=None, timeout=10)
    if (a.apply or a.undo) and not a.no_backup:
        stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
        db.execute(f"VACUUM INTO '{a.db}.bak-{stamp}-pre-familiars'")
    ensure(db)
    if a.undo:
        undo(db)
        return
    p = plan(db)
    report(p, a.list)
    if a.apply:
        apply(db, p)


if __name__ == "__main__":
    main()
