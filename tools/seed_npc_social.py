#!/usr/bin/env python3
"""The seed rows of npcsocial: every town's guards come to the help of that
town's guards (entity/socialaggro.go; F3 20261025-215440, jib: "All guards of
a town qualify for this, but not all mobs are social so we will have to
specify as we go").

Who is a town's guard comes from the town rows of factionhits
(tools/seed_faction_hits.py, source "town-seed"), which already settled who
belongs to which city off EQOA-Data's map: a name whose rows lower "Citizens
of <town>" is that town's, and it is a guard when its name holds a guard's
word (Guard, Sentry, Deputy, Sergeant, Captain, Protector, Soldier...; the
list is GUARD_WORDS, and each name is printed so a GM can see who counts).

Each town's guards are written under one faction, the one they help: the
town's guard faction where its guards' kills lower one (The Freeport Militia,
The Qeynos Guard, The Highpass Guard), else "Guards of <town>", a group of its
own, so that guards help guards and not townsfolk (an attacked townsperson is
of its Citizens faction only).

Rows are written with source "seed" and never over a GM's row for the name.
--undo deletes every seed row. Both take a copy of the database first
(<db>.bak-<when>-pre-socialseed). Report only by default. The table is the
server's (database/npcsocial.go); --apply makes it if the server has not yet.
Restart the server after --apply or --undo, or edit any npcsocial row in the
GM panel, for the running world to read them.

    tools/seed_npc_social.py [--db eqoa.db] [--apply | --undo] [--keep NAME ...]
"""
import argparse
import collections
import datetime
import os
import re
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"
FACTION_SEED = "town-seed"

# the table as database/npcsocial.go makes it
SCHEMA = """CREATE TABLE IF NOT EXISTS npcsocial (
    npc_name TEXT PRIMARY KEY,
    social   INTEGER NOT NULL DEFAULT 1,
    faction  TEXT NOT NULL DEFAULT '',
    radius   REAL NOT NULL DEFAULT 0,
    note     TEXT NOT NULL DEFAULT '',
    source   TEXT NOT NULL DEFAULT 'gm'
)"""

# a word of a guard's name (whole words, any case)
GUARD_WORDS = {
    "guard", "guards", "guardsman", "sentry", "sentinel", "watchman", "warden", "deputy", "lieutenant",
    "sergeant", "soldier", "corporal", "protector", "marshal", "marshall", "commander", "captain",
    "militia", "defender", "trooper", "force",
}

# a town guard faction's word, best first: the one a town's guards help
FACTION_WORDS = ["guard", "militia", "watch", "defenders", "wardens", "warden"]

CITIZENS = re.compile(r"^Citizens of (.+)$")


def is_guard(name):
    return bool(GUARD_WORDS & set(re.findall(r"[a-z]+", name.lower())))


def faction_rank(faction):
    words = set(re.findall(r"[a-z]+", faction.lower()))
    for i, w in enumerate(FACTION_WORDS):
        if w in words:
            return i
    return len(FACTION_WORDS)


def plan(db, keep):
    """Every town's guard names and the faction they help: (rows, towns,
    unplaced), rows {name: (faction, town)}."""
    town_of = {}
    guard_factions = collections.defaultdict(set)  # town -> its guards' guard factions
    names = collections.defaultdict(list)          # name -> [(faction, delta)]
    for name, faction, delta in db.execute(
            "SELECT npc_name, faction, delta FROM factionhits WHERE source = ?", (FACTION_SEED,)):
        names[name].append((faction, delta))
        m = CITIZENS.match(faction)
        if m:
            town_of[name] = m.group(1)
    for name, hits in names.items():
        town = town_of.get(name)
        if town is None:
            continue
        for faction, delta in hits:
            if not CITIZENS.match(faction) and delta < 0:
                guard_factions[town].add(faction)

    towns = {}
    for town, fs in guard_factions.items():
        towns[town] = sorted(fs, key=lambda f: (faction_rank(f), f))[0]
    rows = {}
    for name in sorted(names):
        town = town_of.get(name)
        if town is None or not is_guard(name) or name.strip().lower() in keep:
            continue
        rows[name] = (towns.get(town, f"Guards of {town}"), town)

    seeded = set(names)
    unplaced = sorted({n for (n,) in db.execute("SELECT DISTINCT npc_name FROM npcs")
                       if is_guard(n) and n not in seeded})
    return rows, towns, unplaced


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-socialseed"
    n = 2
    while os.path.exists(out):  # never over an earlier copy of the same second
        out = f"{path}.bak-{when}-{n}-pre-socialseed"
        n += 1
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the seed rows")
    ap.add_argument("--undo", action="store_true", help="delete every seed row")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--keep", nargs="*", default=[], help="names to leave out")
    ap.add_argument("--quiet", action="store_true", help="the counts only, not every name")
    args = ap.parse_args()

    db = sqlite3.connect(args.db)
    if args.undo:
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        try:
            n = db.execute("DELETE FROM npcsocial WHERE source = ?", (SOURCE,)).rowcount
        except sqlite3.OperationalError:
            n = 0
        db.commit()
        print(f"deleted {n} seed rows")
        return

    try:
        rows, towns, unplaced = plan(db, {k.strip().lower() for k in args.keep})
    except sqlite3.OperationalError as e:
        sys.exit(f"factionhits: {e} (run tools/seed_faction_hits.py first)")
    if not rows:
        sys.exit("no town guard found: factionhits has no town-seed rows (tools/seed_faction_hits.py)")

    db.execute(SCHEMA)
    have = {n: s for n, s in db.execute("SELECT npc_name, source FROM npcsocial")}
    new = [(name, faction, town) for name, (faction, town) in rows.items() if name not in have]
    per_town = collections.Counter(town for _, town in rows.values())
    print(f"{len(rows)} town guard names in {len(per_town)} towns; {len(new)} not yet in npcsocial "
          f"({sum(1 for n in rows if have.get(n) not in (None, SOURCE))} kept as a GM set them)")
    for town in sorted(per_town):
        faction = towns.get(town, f"Guards of {town}")
        own = "its guard faction" if town in towns else "no guard faction: a group of its own"
        print(f"  {town}: {per_town[town]} names, helping {faction!r} ({own})")
        if not args.quiet:
            for name, (f, t) in rows.items():
                if t == town:
                    mark = "" if name not in have else f"  [already: {have[name]}]"
                    print(f"      {name!r}{mark}")
    if unplaced:
        print(f"{len(unplaced)} guard names are no town's in factionhits (on roads, or in two towns): left to a GM")
        if not args.quiet:
            print("      " + ", ".join(repr(n) for n in unplaced[:40]) + (" ..." if len(unplaced) > 40 else ""))
    if not args.apply:
        print("(report only: --apply writes them)")
        return
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    db.executemany(
        "INSERT OR IGNORE INTO npcsocial (npc_name, social, faction, radius, note, source) VALUES (?, 1, ?, 0, ?, ?)",
        [(name, faction, f"a guard of {town}: comes to the help of its guards", SOURCE) for name, faction, town in new])
    db.commit()
    print(f"wrote {len(new)} rows")


if __name__ == "__main__":
    main()
