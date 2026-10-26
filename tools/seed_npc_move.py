#!/usr/bin/env python3
"""The seed rows of npcmove: how the creatures of a name move (entity/movement.go,
water.go; F3 20261026-021730, -020316, jib's asks).

No record of the live game says which creatures stood or swam, so the seed goes
by whole words of the creatures' names, and only for creatures (NPCs without the
NPC bit 0x80 whose bodies are not the players' races, charactermodel: those are
the townsfolk, who never wander):

- roam 0, standing on their spawns until pulled: the Hexbones (jib: "hexbones in
  zentaurs keep do NOT roam, they are static spawn until agrod"; they stand in
  camps of four to eight, Zentar's Keep among them), every creature named a guard,
  guardian, bodyguard, lookout, watchman, sentry, sentinel, warden or gatekeeper
  (a camp's or a keep's guard: "Lionguard brigand" is not one), and the chests.
- water "both", in and out of the water: caiman, crocodiles, alligators, crabs,
  turtles, tortoises, frogs, toads, water rats and water moccasins (jib: "Caiman,
  crocodiles, crabs can roam both").
- water "water", keeping to it: sharks, eels, piranhas, fish, anglers, barracudas,
  squid, octopi, jellyfish, minnows, rays, krakens, sea snakes, water wolves and
  water lions. A creature that spawned in the water keeps to it with no row; these
  rows only make it so for one recorded at the surface, and show it to GMs.

Rows are written with source "seed" and never over a row already there (a GM's,
or an earlier seed's); --undo deletes every seed row. Both take a copy of the
database first (<db>.bak-<when>-pre-moveseed). Report only by default. The table
is the server's (database/npcmove.go); --apply makes it if the server has not yet.
Restart the server after --apply or --undo, or edit any npcmove row in the GM
panel, for the running world to read them.

    tools/seed_npc_move.py [--db eqoa.db] [--apply | --undo] [--list]
"""
import argparse
import collections
import datetime
import os
import re
import sqlite3

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "seed"

# the table as database/npcmove.go makes it
SCHEMA = """CREATE TABLE IF NOT EXISTS npcmove (
    name   TEXT PRIMARY KEY,
    roam   INTEGER NOT NULL DEFAULT 1,
    radius REAL NOT NULL DEFAULT 0,
    water  TEXT NOT NULL DEFAULT '',
    note   TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm'
)"""

STAND = re.compile(
    r"\b(?:hexbones?|guards?|guardians?|bodyguards?|lookouts?|watchm[ae]n|sentr(?:y|ies)|sentinels?|"
    r"wardens?|gatekeepers?|chests?)\b")
BOTH = re.compile(
    r"\b(?:caimans?|crocodiles?|alligators?|crabs?|turtles?|tortoises?|frogs?|toads?|water rats?|"
    r"water moccasins?)\b")
WATER = re.compile(
    r"\b(?:sharks?|eels?|piranhas?|fish|fishes|stonefish|sawfish|bonefish|swordfish|anglers?|barracudas?|"
    r"squids?|octopus(?:es)?|octopi|jellyfish|minnows?|stingrays?|rays?|krakens?|sea snakes?|"
    r"water wol(?:f|ves)|water lions?)\b")


def key(name):
    """The name as database.NPCClassKey keys it: lower case, trimmed, no
    trailing '$'."""
    n = name.strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-moveseed"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def seed_rows(db):
    """name key -> (roam, water, note, spawns): what the seed writes, and why."""
    try:
        players = {(m or 0) & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel")}
    except sqlite3.OperationalError:
        players = set()
    spawns = collections.Counter()
    creature = collections.defaultdict(bool)
    for name, model, typ in db.execute("SELECT npc_name, modelid, npc_type FROM npcs"):
        if not name or not key(name):
            continue
        k = key(name)
        spawns[k] += 1
        if not ((typ or 0) & 0x80) and model and (model & 0xFFFFFFFF) not in players:
            creature[k] = True
    rows = {}
    for k in sorted(spawns, key=lambda n: (-spawns[n], n)):
        if not creature[k]:
            continue
        roam, water, why = 1, "", []
        m = STAND.search(k)
        if m:
            roam = 0
            why.append(f"stands ({m.group(0)})")
        m = WATER.search(k)
        if m:
            water = "water"
            why.append(f"keeps to the water ({m.group(0)})")
        else:
            m = BOTH.search(k)
            if m:
                water = "both"
                why.append(f"in and out of the water ({m.group(0)})")
        if why:
            rows[k] = (roam, water, "; ".join(why), spawns[k])
    return rows


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the seed rows")
    ap.add_argument("--undo", action="store_true", help="delete every seed row")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    ap.add_argument("--list", action="store_true", help="list every name, not only the counts")
    args = ap.parse_args()

    db = sqlite3.connect(args.db)
    if args.undo:
        if not args.no_backup:
            print("copy:", backup(db, args.db))
        try:
            n = db.execute("DELETE FROM npcmove WHERE source = ?", (SOURCE,)).rowcount
        except sqlite3.OperationalError:
            n = 0
        db.commit()
        print(f"deleted {n} seed rows")
        return

    rows = seed_rows(db)
    db.execute(SCHEMA)
    have = {n: s for n, s in db.execute("SELECT name, source FROM npcmove")}
    new = [(k, v) for k, v in rows.items() if k not in have]
    stand = [k for k, v in rows.items() if v[0] == 0]
    both = [k for k, v in rows.items() if v[1] == "both"]
    water = [k for k, v in rows.items() if v[1] == "water"]
    print(f"{len(rows)} creature names: {len(stand)} stand ({sum(rows[k][3] for k in stand)} spawns), "
          f"{len(both)} go in and out of the water ({sum(rows[k][3] for k in both)}), {len(water)} keep to it "
          f"({sum(rows[k][3] for k in water)}); {len(new)} not yet in npcmove "
          f"({sum(1 for k in rows if have.get(k) not in (None, SOURCE))} left as a GM set them)")
    for title, names in (("stand", stand), ("both", both), ("water", water)):
        shown = names if args.list else names[:25]
        print(f"  {title}: " + ", ".join(f"{k} x{rows[k][3]}" for k in shown) +
              ("" if len(shown) == len(names) else f", ... ({len(names) - len(shown)} more: --list)"))
    if not args.apply:
        print("(report only: --apply writes them, --list lists every name)")
        return
    if not args.no_backup:
        print("copy:", backup(db, args.db))
    db.executemany("INSERT OR IGNORE INTO npcmove (name, roam, radius, water, note, source) VALUES (?, ?, 0, ?, ?, ?)",
                   [(k, v[0], v[1], v[2], SOURCE) for k, v in new])
    db.commit()
    print(f"wrote {len(new)} rows")


if __name__ == "__main__":
    main()
