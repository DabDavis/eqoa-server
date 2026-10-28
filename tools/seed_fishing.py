#!/usr/bin/env python3
"""Fishing's content (handlers/fishing.go): what a cast brings up, the bait the
game will let a player wear, and someone to sell a pole and bait.

What came up from the water is Allakhazam's EQOA item database as players
filled it in April 2003, the month fishing came in ("Break out your poles!
Gather up some bait! Let's go fishing!", the update of 16 April 2003): its
news posts of that week list the items new to it, and the fishing ones are
plain to see (the archived posts, eqoa-patch-notes-offline.zip): Perch,
Barbel, Walley, Chub, White Bread, Rusty Metal Box, Spiced Jerky, Weeping
Hearts Pendant, Regrets Fish, Rudd, Carp, Catfish, Moldy Bone, Broken
Lantern and Child's Doll on the 16th, the Brittle Fishing Rod among them;
Zander, Lowland Carp, Bream, Nipperfish, Glitterperch, Grayling, Rat Pike,
Fine Tench and Bank Jelly a few days on; then Waterlogged Lute, Leather Carp,
Muddy Jellyfish, Lowland Bream, Golden Perch, Blue Nipper, Blue Zander,
Golden Tench, Fine Grayling, Creek Gambusa, Face Gudgeon, Murray Perch and
Long Gudgeon. EQOA-Data's quest guides add the fish its quests ask for: the
level 41 tunaria bass ("you can fish for it in Haz or Oasis"; "I caught one
in no more than 10 casts") and grass carp, a Longfellow bream ("an oceanic
watering hole on Tunaria or Odus. Hazinak works good"), and the producer's
letter of 17 April 2003 Halas's salmon. The patch notes: "the fish you
catch will adjust seasonally".

None of those records gives a level, a price, a stack or a description, so
every number here is this server's own: a fish is a meal (the grocers'
words, so handlers/consume.go heals by its level for a minute: "you will be
able to catch your own food", the patch notes), the common ones level 1, the
less common 10 and the rare 25, stacks of 50; the junk worth a tunar; the
weights and the skill each needs (characters.fishing, 0 to 100) chosen so a
new angler brings up common fish and junk and a skilled one the rare;
some of the rare in their season only (ours). An item already in itempattern
by name is used as it is.

- Items: each catch that is not in itempattern yet is made, its icon an
  existing item's of the same kind (a fish's the grass carp's), named in the
  report.
- Bait: "can of worms" ("Standard bait for fishing", EQOA-Data) is worn
  nowhere and by no class in Return Home's rows; CLIENT casts only with bait
  worn (wear slot 21), so it becomes wear slot 21 for every class and race.
  The "Fishing Pole" and the bait get a price where they have none.
- fishcatch: the catches, every world and anywhere unless a row says where
  (the quest fish in the Oasis's and Hazinak's 2000-square, the salmon in
  Halas's, the bream there and on Odus), source seed.
- Sellers: every "Fisherman <name>" NPC players talk to (the NPC bit) sells
  a Fishing Pole and a can of worms (the merchant flag, npc_type & 1; a
  catalog in npcitem), as the patch notes' NPCs "near each starting city
  that can walk you through the basic steps of how to begin fishing" might
  have; a fisherman whose catalog is a GM's is left alone.

Every change is recorded in fishing_seed (what it was) and --undo puts it
back; an item a character holds is not deleted (said). Both take a copy of
the database first (<db>.bak-<when>-pre-fishing). Report only by default.
Restart the server after --apply or --undo: the NPCs' flags are read at its
start (the catches at every cast).

    tools/seed_fishing.py [--db eqoa.db] [--apply | --undo]
"""
import argparse
import collections
import datetime
import sqlite3
import sys

DEFAULT_DB = "eqoa.db"

MEAL = "A hearty meal that will increase health regeneration."
EVERY_CLASS, EVERY_RACE = 32767, 1023

# the months a catch keeps to (bit 0 January), ours
SPRING_SUMMER = sum(1 << m for m in range(3, 9))         # April to September
AUTUMN_WINTER = sum(1 << m for m in (0, 1, 2, 9, 10, 11))  # October to March
WINTER = sum(1 << m for m in (0, 1, 10, 11))               # November to February
HIGH_SUMMER = sum(1 << m for m in range(4, 8))            # May to August

# Tunaria's zones are 2000 squares (claude-notes/tunaria-map.md); these are
# the ones EQOA-Data's guides fish in
OASIS = (20000, 24000, 22000, 26000)
HAZINAK = (24000, 26000, 26000, 28000)
HALAS = (12000, 4000, 14000, 6000)
TUNARIA, ODUS = 0, 2


class Catch:
    """One thing a cast may bring up: an item by name, made when the database
    has none of the name (kind, level, price, stack, words), and its rows."""

    def __init__(self, name, kind, level=1, price=1, stack=1, words="", donors=(), rows=()):
        self.name, self.kind, self.level, self.price, self.stack = name, kind, level, price, stack
        self.words, self.donors, self.rows = words, donors, rows


def row(weight, skill=0, count=1, world=-1, box=None, months=0, note=""):
    return dict(weight=weight, skill=skill, count=count, world=world, box=box, months=months, note=note)


FISH = ("grass carp", "carp", "fish")


def fish(name, level, price, words, *rows):
    return Catch(name, "fish", level, price, 50, f"{words} {MEAL}", FISH, rows)


def junk(name, words, donors, *rows):
    return Catch(name, "junk", 1, 1, 1, words, donors, rows)


COMMON = "common, a new angler's"
UNCOMMON = "less common: skill 15"
RARE = "rare: skill 40"

CATCHES = [
    # the 16th of April: the fish
    fish("Carp", 1, 2, "A carp, fresh from the water.", row(20, note=COMMON)),
    fish("Perch", 1, 2, "A striped perch.", row(20, note=COMMON)),
    fish("Chub", 1, 2, "A plump chub.", row(20, note=COMMON)),
    fish("Rudd", 1, 2, "A red-finned rudd.", row(20, note=COMMON)),
    fish("Catfish", 10, 6, "A whiskered catfish.", row(8, 15, note=UNCOMMON)),
    fish("Barbel", 10, 6, "A bottom-feeding barbel.", row(8, 15, note=UNCOMMON)),
    fish("Walley", 10, 6, "A walley, glassy-eyed.", row(8, 15, note=UNCOMMON)),
    fish("Regrets Fish", 25, 25, "A sorry-looking fish.", row(3, 40, note=RARE)),
    # a few days on, and through the month
    fish("Bream", 1, 2, "A broad bream.", row(20, note=COMMON)),
    fish("Nipperfish", 1, 2, "A small, snapping fish.", row(20, note=COMMON)),
    fish("Zander", 1, 2, "A long-toothed zander.", row(20, note=COMMON)),
    fish("Grayling", 1, 2, "A grayling with a tall fin.", row(20, note=COMMON)),
    fish("Lowland Carp", 10, 6, "A carp of the lowland waters.", row(8, 15, note=UNCOMMON)),
    fish("Leather Carp", 10, 6, "A scaleless leather carp.", row(8, 15, note=UNCOMMON)),
    fish("Lowland Bream", 10, 6, "A bream of the lowland waters.", row(8, 15, note=UNCOMMON)),
    fish("Rat Pike", 10, 6, "A small, sharp-toothed pike.", row(8, 15, note=UNCOMMON)),
    fish("Fine Tench", 10, 6, "A fine, olive tench.", row(8, 15, note=UNCOMMON)),
    fish("Creek Gambusa", 10, 6, "A tiny creek fish.", row(8, 15, note=UNCOMMON)),
    fish("Face Gudgeon", 10, 6, "A gudgeon with an odd face.", row(8, 15, note=UNCOMMON)),
    fish("Long Gudgeon", 10, 6, "A long, thin gudgeon.", row(8, 15, note=UNCOMMON)),
    fish("Murray Perch", 10, 6, "A deep-bodied perch.", row(8, 15, note=UNCOMMON)),
    fish("Golden Perch", 25, 25, "A perch with golden scales.",
         row(3, 40, months=SPRING_SUMMER, note=RARE + ", April to September")),
    fish("Golden Tench", 25, 25, "A tench of gold.", row(3, 40, note=RARE)),
    fish("Fine Grayling", 25, 25, "A fine grayling.",
         row(3, 40, months=AUTUMN_WINTER, note=RARE + ", October to March")),
    fish("Blue Zander", 25, 25, "A zander with a blue sheen.",
         row(3, 40, months=WINTER, note=RARE + ", November to February")),
    fish("Blue Nipper", 25, 25, "A blue snapping fish.", row(3, 40, note=RARE)),
    fish("Glitterperch", 25, 25, "A perch that glitters in the light.",
         row(3, 40, months=HIGH_SUMMER, note=RARE + ", May to August")),
    # the quest fish (EQOA-Data's guides)
    fish("tunaria bass", 41, 15, "A large Tunarian bass.",
         row(30, box=OASIS, world=TUNARIA, note="the Oasis (EQOA-Data: Haz or Oasis)"),
         row(30, box=HAZINAK, world=TUNARIA, note="Hazinak (EQOA-Data: Haz or Oasis)")),
    fish("grass carp", 41, 15, "This grass carp is named for the area the fish is most commonly found.",
         row(20, box=OASIS, world=TUNARIA, note="the Oasis, beside the bass"),
         row(20, box=HAZINAK, world=TUNARIA, note="Hazinak, beside the bass")),
    fish("Longfellow bream", 50, 60, "A long, heavy bream of the open sea.",
         row(4, 50, box=HAZINAK, world=TUNARIA, note="EQOA-Data: an oceanic hole, Hazinak"),
         row(4, 50, world=ODUS, note="EQOA-Data: on Tunaria or Odus")),
    fish("Salmon", 1, 6, "A fat river salmon.",
         row(25, box=HALAS, world=TUNARIA, note="Halas's ice fishing (the producer's letter, 17 April 2003)")),
    # the jellies
    Catch("Bank Jelly", "jelly", 1, 3, 20, "A quivering jelly from the river bank.",
          ("jelly", "stomach", "venom sack"), (row(4, note="ours: common"),)),
    Catch("Muddy Jellyfish", "jelly", 1, 3, 20, "A jellyfish caked in mud.",
          ("jelly", "stomach", "venom sack"), (row(4, note="ours: common"),)),
    # the junk ("Yes, you will still catch that occasional old cruddy sandal")
    junk("Child's Doll", "A sodden child's doll.", ("doll",), row(4, note="junk")),
    junk("Broken Lantern", "A rusted lantern, its glass broken.", ("lantern",), row(4, note="junk")),
    junk("Moldy Bone", "A bone, green with mold.", ("bones", "bone"), row(4, note="junk")),
    junk("Rusty Metal Box", "A small metal box, rusted shut.", ("box", "chest"), row(4, note="junk")),
    junk("Waterlogged Lute", "A lute that will never play again.", ("lute", "flute", "harp", "drum", "box"),
         row(3, note="junk")),
    Catch("Brittle Fishing Rod", "keep", rows=(row(2, 10, note="a rod someone lost"),)),
    # the food and the find
    Catch("White Bread", "meal", 1, 2, 50, "A loaf of white bread, somehow dry. " + MEAL,
          ("white bread", "bread"), (row(3, note="food"),)),
    Catch("Spiced Jerky", "keep", rows=(row(3, note="food"),)),
    Catch("Weeping Hearts Pendant", "keep", rows=(row(1, 30, note="a find: skill 30"),)),
]

BAIT = "can of worms"
POLE = "Fishing Pole"
PRICES = {POLE: 40, BAIT: 2}  # ours
SELLER_PREFIX = "fisherman "
NPC_BIT, MERCHANT = 0x80, 0x01


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-fishing"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def ensure_log(db):
    db.execute("""CREATE TABLE IF NOT EXISTS fishing_seed (
        what TEXT NOT NULL,     -- 'item' | 'column' | 'catch' | 'catalog' | 'npc_type' | 'npc_id'
        key  INTEGER NOT NULL,  -- the itempattern, fishcatch, npcitem or npcs row
        col  TEXT,              -- the column changed ('column')
        old  TEXT               -- what it was
    )""")


def ensure_fishcatch(db):
    # the server makes it at its start (database/fishing.go); the same here,
    # for a database it has not opened since
    db.execute("""CREATE TABLE IF NOT EXISTS fishcatch (
        id INTEGER PRIMARY KEY AUTOINCREMENT, itemid INTEGER NOT NULL,
        count INTEGER NOT NULL DEFAULT 1, weight INTEGER NOT NULL DEFAULT 10,
        world INTEGER NOT NULL DEFAULT -1, minx REAL, minz REAL, maxx REAL, maxz REAL,
        minskill INTEGER NOT NULL DEFAULT 0, months INTEGER NOT NULL DEFAULT 0,
        note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')""")


def by_name(db, name):
    r = db.execute("""SELECT patternid FROM itempattern WHERE lower(itemname) = lower(?)
        ORDER BY patternid LIMIT 1""", (name,)).fetchone()
    return r[0] if r else None


def donor(db, words):
    """The first unworn item (not a spell scroll) whose name holds one of the
    words, in their order: its id, name and icon."""
    for w in words:
        r = db.execute("""SELECT patternid, itemname, itemicon FROM itempattern
            WHERE lower(itemname) LIKE ? AND equipslot = -1 AND itemicon != -607649328
            ORDER BY lower(itemname) = ? DESC, patternid LIMIT 1""", (f"%{w}%", w)).fetchone()
        if r:
            return r
    return None


def plan(db):
    """What --apply would do: the items to make (with their icon's donor),
    the catches' rows, the bait's and the pole's columns, the sellers."""
    make, have, rows, missing = [], {}, [], []
    for c in CATCHES:
        pid = by_name(db, c.name)
        if pid is None:
            if c.kind == "keep":
                missing.append(c.name)
                continue
            d = donor(db, c.donors)
            if d is None:
                missing.append(c.name)
                continue
            make.append((c, d))
        else:
            have[c.name] = pid
        for r in c.rows:
            rows.append((c.name, r))
    cols = []
    bait = db.execute("SELECT patternid, equipslot, classuse, raceuse, itemcost FROM itempattern WHERE lower(itemname) = ?",
                      (BAIT,)).fetchall()
    for pid, slot, cls, race, cost in bait:
        if slot != 21:
            cols.append((pid, "equipslot", slot, 21))
        if not cls:
            cols.append((pid, "classuse", cls, EVERY_CLASS))
        if not race:
            cols.append((pid, "raceuse", race, EVERY_RACE))
        if not cost:
            cols.append((pid, "itemcost", cost, PRICES[BAIT]))
    for pid, cost in db.execute("SELECT patternid, itemcost FROM itempattern WHERE lower(itemname) = lower(?)", (POLE,)):
        if not cost:
            cols.append((pid, "itemcost", cost, PRICES[POLE]))
    sellers = []
    if by_name(db, POLE) is not None and bait:
        for i, name, typ, npc_id in db.execute("SELECT id, npc_name, npc_type, npc_id FROM npcs ORDER BY id"):
            if not (name or "").lower().startswith(SELLER_PREFIX) or not (typ or 0) & NPC_BIT:
                continue
            key = npc_id or i
            if db.execute("SELECT 1 FROM npcitem WHERE npcid = ? LIMIT 1", (key,)).fetchone():
                continue  # a catalog already: a GM's, or this tool's
            sellers.append((i, name, typ or 0, npc_id or 0))
    return make, have, rows, cols, sellers, missing


def apply(db, make, have, rows, cols, sellers):
    ensure_log(db)
    ensure_fishcatch(db)
    log = []
    db.execute("BEGIN")
    ids = dict(have)
    for c, (_, _, icon) in make:
        cls, race, kind = EVERY_CLASS, EVERY_RACE, 2 if c.kind in ("fish", "meal") else 0
        cur = db.execute("""INSERT INTO itempattern
            (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
             weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
             procanim, lore, unk6, craft, itemname, itemdesc, model, color)
            VALUES (?, 0, ?, 0, -1, 0, 0, 1, 0, 0, 0, 0, ?, ?, 0, 50, ?, ?, ?, 0, 0, 0, ?, ?, 0, 4294967295)""",
                         (c.price, icon, c.level, c.stack, cls, race, kind, c.name, c.words))
        ids[c.name] = cur.lastrowid
        log.append(("item", cur.lastrowid, None, None))
    for pid, col, old, new in cols:
        db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ?", (new, pid))
        log.append(("column", pid, col, None if old is None else str(old)))
    for name, r in rows:
        box = r["box"] or (None, None, None, None)
        cur = db.execute("""INSERT INTO fishcatch (itemid, count, weight, world, minx, minz, maxx, maxz,
            minskill, months, note, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'seed')""",
                         (ids[name], r["count"], r["weight"], r["world"], *box, r["skill"], r["months"], r["note"]))
        log.append(("catch", cur.lastrowid, None, None))
    pole, bait = by_name(db, POLE), by_name(db, BAIT)
    for i, _, typ, npc_id in sellers:
        if not typ & MERCHANT:
            db.execute("UPDATE npcs SET npc_type = ? WHERE id = ?", (typ | MERCHANT, i))
            log.append(("npc_type", i, None, str(typ)))
        if not npc_id:
            db.execute("UPDATE npcs SET npc_id = ? WHERE id = ?", (i, i))
            log.append(("npc_id", i, None, str(npc_id)))
        key = npc_id or i
        for n, p in enumerate((pole, bait)):
            cur = db.execute("""INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid,
                equiploc, location, listnumber) VALUES (?, 1, 0, 0, ?, -1, 1, ?)""", (key, p, n))
            log.append(("catalog", cur.lastrowid, None, None))
    db.executemany("INSERT INTO fishing_seed (what, key, col, old) VALUES (?, ?, ?, ?)", log)
    db.execute("COMMIT")
    return collections.Counter(w for w, *_ in log)


def undo(db):
    ensure_log(db)
    log = list(db.execute("SELECT what, key, col, old FROM fishing_seed ORDER BY rowid DESC"))
    n, kept = collections.Counter(), []
    db.execute("BEGIN")
    for what, key, col, old in log:
        if what == "item":
            if db.execute("SELECT 1 FROM charinventory WHERE patternid = ? LIMIT 1", (key,)).fetchone():
                name = db.execute("SELECT itemname FROM itempattern WHERE patternid = ?", (key,)).fetchone()
                kept.append(name[0] if name else str(key))
                continue
            db.execute("DELETE FROM itempattern WHERE patternid = ?", (key,))
        elif what == "column":
            db.execute(f"UPDATE itempattern SET {col} = ? WHERE patternid = ?", (old, key))
        elif what == "catch":
            db.execute("DELETE FROM fishcatch WHERE id = ?", (key,))
        elif what == "catalog":
            db.execute("DELETE FROM npcitem WHERE itemid = ?", (key,))
        elif what in ("npc_type", "npc_id"):
            db.execute(f"UPDATE npcs SET {what} = ? WHERE id = ?", (int(old), key))
        n[what] += 1
    db.execute("DROP TABLE fishing_seed")
    db.execute("COMMIT")
    return n, kept


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="make the items, rows and sellers")
    ap.add_argument("--undo", action="store_true", help="put back everything a run wrote")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 5000")
    seeded = db.execute("SELECT name FROM sqlite_master WHERE name = 'fishing_seed'").fetchone()
    if a.undo:
        if not seeded:
            print("fishing_seed is not there: nothing to put back")
            return 0
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        n, kept = undo(db)
        print("put back:", dict(n) or "nothing")
        if kept:
            print(f"kept {len(kept)} items a character holds: {', '.join(kept)}")
        print("restart the server")
        return 0
    if a.apply and seeded:
        ap.error("fishing_seed is there: this database was seeded already (--undo first)")
    make, have, rows, cols, sellers, missing = plan(db)
    for c, (pid, name, _) in make:
        print(f"make {c.name!r} ({c.kind}, level {c.level}, {c.price} tunar, stack {c.stack}), the icon of {name!r} ({pid})")
    for name, pid in sorted(have.items()):
        print(f"have {name!r} ({pid})")
    if missing:
        print(f"left out (no such item, or no icon to give it): {', '.join(missing)}")
    print(f"{len(rows)} fishcatch rows")
    for pid, col, old, new in cols:
        print(f"item {pid}: {col} {old} -> {new}")
    for i, name, typ, _ in sellers:
        print(f"seller: {name} (row {i}{'' if typ & MERCHANT else ', the merchant flag added'})")
    if not a.apply:
        print("(report only: --apply writes them)")
        return 0
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    n = apply(db, make, have, rows, cols, sellers)
    print("wrote:", dict(n), "- restart the server")
    return 0


if __name__ == "__main__":
    sys.exit(main())
