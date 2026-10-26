#!/usr/bin/env python3
"""The seed rows of npcundead: which creatures are undead (entity/undead.go),
for the spells that work on undead alone (F3 20261025-215937, -222603,
-220159) and the charms that never take one.

No record of the live game says which creatures were undead, so the seed
takes the two things the server holds:

- the name: whole words of the list entity/undead.go keeps (reUndeadName:
  skeleton, zombie, ghoul, ghast, mummy, spectre, wraith, ghost, phantom,
  revenant, rotting, decaying, bones, soul, dead...). The server already treats
  a name with no row so; a row makes the creature visible and editable in the
  GM panel's Undead creatures.
- the body: every NPC of a name that wears one of the bodies below (npcs.modelid,
  a CHAR.ESF CSprite), which skeletons, zombies, mummies, ghouls and spectres
  wear: a creature drawn as a skeleton is one, whatever its name ("Hexbone",
  "Returned Indigo Pirate", the townsfolk of a haunted village). Each body is
  listed with the undead names that wear it, and was picked by hand from the
  share of its spawns whose names are undead's; bodies that undead share with
  the living (the spirits' with nature spirits, the enchanter's Animation's
  with the spectral knights, the gnoll zombie's with the gnoll reavers) are
  left out, so their creatures go by name alone. A name with "construct" in
  it is never tagged by its body (the discarded constructs wear a zombie's).

Rows are written undead 1 with source "seed" and never over a row already
there (a GM's, or an earlier seed's); --undo deletes every seed row. Both take
a copy of the database first (<db>.bak-<when>-pre-undeadseed). Report only by
default. The table is the server's (database/npcundead.go); --apply makes it
if the server has not yet. Restart the server after --apply or --undo, or
edit any npcundead row in the GM panel, for the running world to read them.

    tools/seed_npc_undead.py [--db eqoa.db] [--apply | --undo] [--list]
"""
import argparse
import collections
import datetime
import os
import re
import sqlite3

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"

# the table as database/npcundead.go makes it
SCHEMA = """CREATE TABLE IF NOT EXISTS npcundead (
    name   TEXT PRIMARY KEY,
    undead INTEGER NOT NULL DEFAULT 1,
    note   TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm'
)"""

# entity/undead.go's reUndeadName, word for word
UNDEAD_NAME = re.compile(
    r"\b(?:skeletons?|skeletal|zombies?|ghouls?|ghasts?|mumm(?:y|ies|ified)|"
    r"spectres?|specters?|spectral|wraiths?|liches?|undead|revenants?|ghosts?|ghostly|phantoms?|phantasms?|"
    r"apparitions?|banshees?|wights?|unliving|unresting|reanimated|risen|raised|cadavers?|rotting|decaying|"
    r"bones|eidolons?|souls?|haunts?|dead|shades?|restless)\b")

NOT_BY_BODY = re.compile(r"\bconstructs?\b")

# The bodies undead wear (npcs.modelid as an unsigned DictID): what it is, and
# what wears it on eqoaclassic (2026-10-25: the share of its spawns with undead
# names, then the other names it was seen on).
UNDEAD_BODIES = {
    0x6AF98A81: ("skeletons", "skeletons (skeleton, decaying skeleton, skeletal guard, undead soldier; 75%), and the haunted "
                "Holciel's and Crethley Manor's townsfolk drawn as skeletons"),
    0x6AF98A82: ("skeletons", "skeletons (skeleton, skeletal blacksmith; 86%)"),
    0x45A8B064: ("skeletons", "skeletons (skeleton, skeletal guard, giant brute skeleton, undead ranger and druid), and the "
                "Hexbones, Charbones and Darkbones (the Hatebone orcs wear orcs' bodies)"),
    0x198BAA64: ("skeletons", "skeletons (putrid and rotbone skeletons, skeletal guardian, undead banker; 70%), the returned "
                "Indigo pirates, the bone servants"),
    0x648DFC11: ("skeletons", "skeletons (skeleton, the icetomb warriors, archers and guardians)"),
    0x6AF98A80: ("skeletal dogs", "skeletal dogs and worgs (98%)"),
    0x4B7CB670: ("skeletal wolves", "skeletal wolves and hounds, crumbling bones (67%), the icebone worgs"),
    0x1BBB898C: ("zombies", "zombies (zombie, decaying, restless and hungry zombies; 89%), the dread, the doomed and the "
                "villagers drawn as zombies"),
    0x1BBB898D: ("zombies", "zombies (zombie, rotting flesh, unresting knights; 70%)"),
    0x1BBB898B: ("undead beasts", "undead beasts (rotting wolf, undead war beast, reanimated beast, raised pet), the dread wolves"),
    0xBB46D88C: ("mummies", "mummies (crypt mummy, mummy, walking dead; 93%)"),
    0xBB46D88D: ("mummies", "mummies (defiled, tainted, dune and wretched mummies; 82%), the necromancer's Putrid Flesh"),
    0xBB46D88E: ("mummies", "mummies (fallen and defiled mummies, mummified minion; 88%)"),
    0x6548134D: ("ghouls", "ghouls (abyssal, deep sea, sunbaked and drowned ghouls, wandering ghast; 100%)"),
    0xEC56B925: ("ghouls", "ghouls and ghasts (lesser, sand and barrow ghouls, greater ghast, haunt; 91%)"),
    0xEC5A64A1: ("ghouls", "ghouls (ghoul captain, ghoul servant, lesser ghoul, al'karad ghoul; 49%), the frighteners"),
    0x6F17325E: ("ghouls", "the ghoul tal and ghoul urd necromancers and wizards, undead minion (97%)"),
    0xEA8B0805: ("ghouls", "the ghoul tal and ghoul urd knights (100%)"),
    0xC1EF4FF4: ("dead riders", "the dead riders (100%)"),
    0xD8C9F052: ("spectres", "spectres (spectre, shade, revenant, spectral servant; 88%), the necromancer's Howling Death"),
    0xD8C9F051: ("spectres", "spectres (spectre, spectral slayer and guardian, undead wizard; 73%)"),
    0x5B1D6121: ("phantoms", "phantoms and lost souls (phantom, lost, forsaken and mirrored souls; 72%), Syndar's manifestations"),
}


def key(name):
    """The name as database.NPCClassKey keys it: lower case, trimmed, no
    trailing '$'."""
    n = name.strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-undeadseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def seed_rows(db):
    """name key -> (note, spawns): what the seed tags, and why."""
    spawns = collections.Counter()
    bodies = collections.defaultdict(set)
    for name, model in db.execute("SELECT npc_name, modelid FROM npcs"):
        if not name or not key(name):
            continue
        k = key(name)
        spawns[k] += 1
        bodies[k].add((model or 0) & 0xFFFFFFFF)
    rows = {}
    for k in sorted(spawns, key=lambda n: (-spawns[n], n)):
        m = UNDEAD_NAME.search(k)
        if m:
            rows[k] = (f"its name ({m.group(0)})", spawns[k])
            continue
        if NOT_BY_BODY.search(k):
            continue
        worn = sorted(b for b in bodies[k] if b in UNDEAD_BODIES)
        if worn:
            b = worn[0]
            rows[k] = (f"its body 0x{b:08X}, {UNDEAD_BODIES[b][0]}'", spawns[k])
    return rows


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the seed rows")
    ap.add_argument("--undo", action="store_true", help="delete every seed row")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--list", action="store_true", help="list every name, not only the counts and the bodies' ones")
    args = ap.parse_args()

    db = sqlite3.connect(args.db)
    if args.undo:
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        try:
            n = db.execute("DELETE FROM npcundead WHERE source = ?", (SOURCE,)).rowcount
        except sqlite3.OperationalError:
            n = 0
        db.commit()
        print(f"deleted {n} seed rows")
        return

    rows = seed_rows(db)
    db.execute(SCHEMA)
    have = {n: s for n, s in db.execute("SELECT name, source FROM npcundead")}
    new = [(k, note) for k, (note, _) in rows.items() if k not in have]
    by_name = {k: v for k, v in rows.items() if v[0].startswith("its name")}
    by_body = {k: v for k, v in rows.items() if not v[0].startswith("its name")}
    print(f"{len(rows)} names to tag undead ({sum(n for _, n in rows.values())} spawns): {len(by_name)} by their "
          f"names, {len(by_body)} by their bodies alone; {len(new)} not yet in npcundead "
          f"({sum(1 for k in rows if have.get(k) not in (None, SOURCE))} left as a GM set them)")
    print("by body alone:")
    for b, (kind, worn_by) in UNDEAD_BODIES.items():
        names = [(k, n) for k, (note, n) in by_body.items() if note.startswith(f"its body 0x{b:08X}")]
        if names:
            print(f"  0x{b:08X} {kind}: {worn_by}")
            print("      " + ", ".join(f"{k} x{n}" + ("" if k not in have else f" [already: {have[k]}]") for k, n in names))
    if args.list:
        print("by name:")
        for k, (note, n) in by_name.items():
            mark = "" if k not in have else f"  [already: {have[k]}]"
            print(f"  {k!r:34} x{n:<4} {note}{mark}")
    if not args.apply:
        print("(report only: --apply writes them, --list lists the names too)")
        return
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    db.executemany("INSERT OR IGNORE INTO npcundead (name, undead, note, source) VALUES (?, 1, ?, ?)",
                   [(k, note, SOURCE) for k, note in new])
    db.commit()
    print(f"wrote {len(new)} rows")


if __name__ == "__main__":
    main()
