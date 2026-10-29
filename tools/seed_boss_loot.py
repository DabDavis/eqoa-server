#!/usr/bin/env python3
"""Loot tables for the named bosses EQOA-Data's Misc guides give drops for:
rows of loottables and npcloot (database/loottables.go, handlers/loot.go).

EQOA-Data (github.com/Jadiction/EQOA-Data, Quests/Misc/*.json) names what a
rare or named creature drops (Lady Vox's shawl of the winter queen,
Snowgrin's seven, the Undead Drake Lord's drakescale set, ...); its item
lists (Information/databases/items) leave most of them without a source, so
seed_camp_loot.py's biome tables never hold them. So:

  - each boss gets a table of its own, "boss <name>", lower case, each item
    by its name in itempattern (the lowest id of a name, never a spell
    scroll), weight 1, for any level (1-70);
  - npcloot binds the boss's name (as npcclass keys it: lower case) to that
    table at 100% a kill, one roll: one of its items on every corpse.

The guides' drops are the live game's; the 100%, the one roll and the even
weights are ours (no record of the live tables exists). A boss this
database has no NPC of (Terrorantula, Dynmath the Tyrant, ... are the Misc
quests' NPC specs, tools/quests/npcs/qd.json) is bound all the same and
drops once it stands; one no player can attack (the NPC bit 0x80) drops
nothing until a GM makes it attackable; both are listed. An item no
itempattern row names is left out and listed. Rows are written with source
boss-seed, never over a GM's; each is logged in boss_loot_seed, and --undo
deletes those still as written (a row a GM changed is kept, and named).
Report only by default; --apply and --undo copy the database first
(<db>.bak-<when>-pre-bossloot). The server reads both tables at every
corpse: no restart is needed.

    python3 -E -s tools/seed_boss_loot.py --db eqoa.db [--apply | --undo]
"""

import argparse
import datetime
import json
import os
import sqlite3

SOURCE = "boss-seed"
CHANCE, ROLLS = 100, 1
SCROLL_ART = -607649328  # database.ScrollArt: a spell scroll's icon

# boss (as npcs spells it) -> its drops (as EQOA-Data's Misc guides spell them)
BOSSES = {
    "Lady Vox": ["shawl of the winter queen"],
    "Archon Aereth": ["worldbreaker chestguard", "traitorous breastplate", "Grundleskin Guard", "archon's heart"],
    "Archon Vetash": ["worldbreaker chestguard", "traitorous breastplate", "Grundleskin Guard", "archon's heart"],
    "Overlord Xrag": ["Bonecleaver"],
    "Xorbb": ["Charm of the Eye"],
    "Snowgrin": ["Dragoneye", "Crystalline Spear", "dragon tendon cord", "Storm's Edge", "Tobrin's Eyepatch",
                 "White Scaled Gauntlets", "Xave's walking stick"],
    "Terrorantula": ["robe of the terrorbrood"],
    "Dynmath the Tyrant": ["Otherworldy Tower", "Otherworldly Circlet", "Otherworldly Coif", "Otherworldly Helm",
                           "Otherworldly Cap"],
    "Undead Drake Lord": ["Drakescale Boots", "Drakescale Robe", "Drakescale Tunic", "Drakescale Leggings",
                          "drakescale ring"],
    "Siliskor": ["shield of king jorat"],
    "Spider Queen Aszec": ["Bleeding Harpoon of Aszec", "Queen Aszec's robe"],
    "Paladin Lord": ["Earring of Might"],
    "Radavex Planeswalker": ["Nullfield Earring", "Radavex's Robe"],
    "Excavation Leader": ["Sapphyra's Necklace"],
}

SCHEMA = """
CREATE TABLE IF NOT EXISTS loottables (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    tablename TEXT NOT NULL,
    patternid INTEGER NOT NULL,
    weight    INTEGER NOT NULL DEFAULT 1,
    minlevel  INTEGER NOT NULL DEFAULT 1,
    maxlevel  INTEGER NOT NULL DEFAULT 60,
    source    TEXT NOT NULL DEFAULT 'gm'
);
CREATE INDEX IF NOT EXISTS idx_loottables_table ON loottables(tablename);
CREATE TABLE IF NOT EXISTS npcloot (
    npc_name  TEXT NOT NULL,
    tablename TEXT NOT NULL,
    chance    INTEGER NOT NULL DEFAULT 25,
    rolls     INTEGER NOT NULL DEFAULT 1,
    note      TEXT NOT NULL DEFAULT '',
    source    TEXT NOT NULL DEFAULT 'gm',
    PRIMARY KEY (npc_name, tablename)
);
CREATE TABLE IF NOT EXISTS boss_loot_seed (
    kind      TEXT NOT NULL,
    key       TEXT NOT NULL,
    row       TEXT NOT NULL,
    PRIMARY KEY (kind, key)
);
"""


def npc_key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing $."""
    n = name.strip().lower()
    return n[:-1].strip() if n.endswith("$") else n


def plan(db):
    pattern = {}
    for pid, name, icon, slot in db.execute("SELECT patternid, itemname, itemicon, equipslot FROM itempattern ORDER BY patternid"):
        if not name or (icon == SCROLL_ART and slot == -1):
            continue
        pattern.setdefault(name.strip().lower(), pid)
    standing = {}
    for name, t in db.execute("SELECT npc_name, npc_type FROM npcs WHERE npc_name IS NOT NULL"):
        k = npc_key(name)
        standing[k] = standing.get(k, False) or (t or 0) & 0x80 == 0
    entries, bindings, missing, absent, unattackable = [], [], [], [], []
    for boss, items in BOSSES.items():
        key, table = npc_key(boss), "boss " + npc_key(boss)
        seen = set()
        for it in items:
            pid = pattern.get(it.strip().lower())
            if pid is None:
                missing.append(f"{boss}: {it}")
                continue
            if pid not in seen:
                seen.add(pid)
                entries.append((table, pid, 1, 1, 70))
        if not seen:
            continue
        bindings.append((key, table, CHANCE, ROLLS, "EQOA-Data's Misc guide: its named drops"))
        if key not in standing:
            absent.append(boss)
        elif not standing[key]:
            unattackable.append(boss)
    return entries, bindings, missing, absent, unattackable


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out, n = f"{path}.bak-{when}-pre-bossloot", 2
    while os.path.exists(out):
        out, n = f"{path}.bak-{when}-{n}-pre-bossloot", n + 1
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def undo(db):
    kept, gone = [], 0
    for kind, key, row in db.execute("SELECT kind, key, row FROM boss_loot_seed").fetchall():
        want = json.loads(row)
        if kind == "entry":
            cur = db.execute("SELECT tablename, patternid, weight, minlevel, maxlevel, source FROM loottables WHERE id = ?", (int(key),)).fetchone()
            if cur is not None and list(cur) != want:
                kept.append(f"loottables row {key}")
                continue
            if cur is not None:
                db.execute("DELETE FROM loottables WHERE id = ?", (int(key),))
                gone += 1
        else:
            name, table = json.loads(key)
            cur = db.execute("SELECT npc_name, tablename, chance, rolls, note, source FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table)).fetchone()
            if cur is not None and list(cur) != want:
                kept.append(f"npcloot {name!r} on {table!r}")
                continue
            if cur is not None:
                db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ?", (name, table))
                gone += 1
        db.execute("DELETE FROM boss_loot_seed WHERE kind = ? AND key = ?", (kind, key))
    if not kept:
        db.execute("DROP TABLE boss_loot_seed")
    db.commit()
    print(f"deleted {gone} seed rows" + (f"; kept {len(kept)} a GM changed: " + ", ".join(kept) if kept else ""))


def apply(db, entries, bindings):
    have = {(t, p) for t, p in db.execute("SELECT tablename, patternid FROM loottables")}
    wrote = 0
    for t, p, w, lo, hi in entries:
        if (t, p) in have:
            continue
        cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES (?, ?, ?, ?, ?, ?)",
                         (t, p, w, lo, hi, SOURCE))
        db.execute("INSERT INTO boss_loot_seed (kind, key, row) VALUES ('entry', ?, ?)",
                   (str(cur.lastrowid), json.dumps([t, p, w, lo, hi, SOURCE])))
        wrote += 1
    bound = {(n, t) for n, t in db.execute("SELECT npc_name, tablename FROM npcloot")}
    tied = 0
    for name, t, chance, rolls, note in bindings:
        if (name, t) in bound:
            continue
        db.execute("INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES (?, ?, ?, ?, ?, ?)",
                   (name, t, chance, rolls, note, SOURCE))
        db.execute("INSERT INTO boss_loot_seed (kind, key, row) VALUES ('binding', ?, ?)",
                   (json.dumps([name, t]), json.dumps([name, t, chance, rolls, note, SOURCE])))
        tied += 1
    db.commit()
    print(f"wrote {wrote} loottables rows and {tied} npcloot rows")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", required=True)
    ap.add_argument("--apply", action="store_true", help="write the rows")
    ap.add_argument("--undo", action="store_true", help="delete the rows written, as written")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    args = ap.parse_args()

    db = sqlite3.connect(args.db)
    if args.undo:
        try:
            db.execute("SELECT 1 FROM boss_loot_seed LIMIT 1")
        except sqlite3.OperationalError:
            print("nothing to undo: no boss_loot_seed table")
            return
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        undo(db)
        return

    entries, bindings, missing, absent, unattackable = plan(db)
    print(f"{len(entries)} items in {len(bindings)} boss tables")
    for b in bindings:
        print(f"  {b[0]!r} -> {b[1]!r} {b[2]}%, {sum(1 for e in entries if e[0] == b[1])} items")
    if missing:
        print("  not in itempattern: " + "; ".join(missing))
    if absent:
        print("  no NPC of the name (drops once one stands): " + ", ".join(absent))
    if unattackable:
        print("  no player can attack it (npc_type 0x80): " + ", ".join(unattackable))
    if not args.apply:
        print("(report only: --apply writes them)")
        return
    db.executescript(SCHEMA)
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    apply(db, entries, bindings)


if __name__ == "__main__":
    main()
