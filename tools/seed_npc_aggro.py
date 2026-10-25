#!/usr/bin/env python3
"""The seed rows of npcaggro: names whose NPCs entity/aggro.go's rule would set
on players but which are to be left peaceful (aggressive 0).

- Players' pets among the NPC rows. Return Home's NPCs (jib's import,
  tools/import_prh_content.py) were recorded off the live game, where a
  player's summoned pet stood beside its owner like any NPC, so pets are in
  npcs as spawns of their own, wherever their owners stood (Companion Spirit
  144 times, Kindred Spirit 138 on eqoaclassic). A pet is named as its spell
  (entity/pet.go), so a name in npcs that is a summoning spell's is a pet's:
  from the server's own spellpattern (a description "Summons ...") and from
  EQOA-Data's class spell lists (Information/databases/spells/*.json: the
  icon "Summoning" or a description "Summons ..."), which hold the shaman's
  spirits that spellpattern lacks. Each name is listed with its count, so a
  name that is also real wildlife can be left out (--keep).
- Measured: Cave Minnow. Sandstorm showed a level-1 player in Neriak a level-2
  one with the white face (pcaps/breath_capture.pcap), which a creature that
  attacks on sight never shows a player in its level range.

Rows are written with source "seed" and never over a GM's row for the name.
--undo deletes every seed row. Both take a copy of the database first
(<db>.bak-<when>-pre-aggroseed). Report only by default. The table is the
server's (database/npcaggro.go); --apply makes it if the server has not yet.
Restart the server after --apply or --undo, or edit any npcaggro row in the GM
panel, for the running world to read them.

    tools/seed_npc_aggro.py --eqoa-data ../external/EQOA-Data [--db eqoa.db]
        [--apply | --undo] [--keep NAME ...]
"""
import argparse
import collections
import datetime
import glob
import json
import os
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"

# the table as database/npcaggro.go makes it
SCHEMA = """CREATE TABLE IF NOT EXISTS npcaggro (
    npc_name   TEXT PRIMARY KEY,
    aggressive INTEGER NOT NULL DEFAULT 1,
    radius     REAL NOT NULL DEFAULT 0,
    note       TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT 'gm'
)"""

MEASURED = {
    "Cave Minnow": "Sandstorm showed a level-1 player in Neriak a level-2 one with the white face "
                   "(pcaps/breath_capture.pcap): it does not attack on sight",
}


def key(name):
    return name.strip().lower()


def spell_names_from_eqoa_data(root):
    """Summoning spells by key -> (spell name, class file), out of EQOA-Data's
    class spell lists."""
    out = {}
    for path in sorted(glob.glob(os.path.join(root, "Information", "databases", "spells", "*.json"))):
        cls = os.path.splitext(os.path.basename(path))[0]
        with open(path, encoding="utf-8") as f:
            data = json.load(f)

        def walk(v):
            if isinstance(v, dict):
                name, icon, desc = v.get("name"), v.get("icon"), v.get("description") or ""
                if isinstance(name, str) and (icon == "Summoning" or "summons" in desc.lower()):
                    out.setdefault(key(name), (name, cls))
                for x in v.values():
                    walk(x)
            elif isinstance(v, list):
                for x in v:
                    walk(x)
        walk(data)
    return out


def spell_names_from_db(db):
    out = {}
    try:
        rows = db.execute("SELECT spellname, spelldesc FROM spellpattern").fetchall()
    except sqlite3.OperationalError:
        return out
    for name, desc in rows:
        if name and "summons" in (desc or "").lower():
            out.setdefault(key(name), (name, "spellpattern"))
    return out


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-aggroseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--eqoa-data", help="the EQOA-Data checkout (Information/databases/spells)")
    ap.add_argument("--apply", action="store_true", help="write the seed rows")
    ap.add_argument("--undo", action="store_true", help="delete every seed row")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--keep", nargs="*", default=[], help="names to leave out (real wildlife)")
    args = ap.parse_args()

    db = sqlite3.connect(args.db)
    if args.undo:
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        try:
            n = db.execute("DELETE FROM npcaggro WHERE source = ?", (SOURCE,)).rowcount
        except sqlite3.OperationalError:
            n = 0
        db.commit()
        print(f"deleted {n} seed rows")
        return

    spells = spell_names_from_db(db)
    if args.eqoa_data:
        for k, v in spell_names_from_eqoa_data(args.eqoa_data).items():
            spells.setdefault(k, v)
    elif not spells:
        sys.exit("no summoning spell known: give --eqoa-data")

    counts = collections.Counter(n for (n,) in db.execute("SELECT npc_name FROM npcs"))
    keep = {key(k) for k in args.keep}
    rows = {}
    for name, n in sorted(counts.items(), key=lambda kv: (-kv[1], kv[0])):
        k = key(name)
        if k in keep:
            continue
        if k in spells:
            spell, where = spells[k]
            rows[name] = (f"a summoned pet's name ({where}: {spell}): a player's pet recorded beside its "
                          f"owner, not wildlife", n)
        elif name in MEASURED:
            rows[name] = (MEASURED[name], n)

    db.execute(SCHEMA)
    have = {n: s for n, s in db.execute("SELECT npc_name, source FROM npcaggro")}
    new = [(name, note) for name, (note, _) in rows.items() if name not in have]
    print(f"{len(spells)} summoning spells known; {len(rows)} names to seed, {len(new)} not yet in npcaggro "
          f"({sum(1 for n in rows if have.get(n) not in (None, SOURCE))} kept as a GM set them):")
    for name, (note, n) in rows.items():
        mark = "" if name not in have else f"  [already: {have[name]}]"
        print(f"  {name!r:28} x{n:<4} {note}{mark}")
    if not args.apply:
        print("(report only: --apply writes them)")
        return
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    db.executemany("INSERT OR IGNORE INTO npcaggro (npc_name, aggressive, radius, note, source) VALUES (?, 0, 0, ?, ?)",
                   [(name, note, SOURCE) for name, note in new])
    db.commit()
    print(f"wrote {len(new)} rows")


if __name__ == "__main__":
    main()
