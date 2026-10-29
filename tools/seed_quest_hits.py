#!/usr/bin/env python3
"""The kill hits the quests' faction gates need: factionhits rows (source
"quest-seed") for the creatures EQOA-Data's guides say raise a faction, so a
gate such as "150 with Feria Hurkert" can be met in the game (entity/faction.go:
a kill changes the killer's standings by the rows for the NPC's name).

Each gate (GATES) names its guide's words, a place and which of its NPCs are
the guide's: every attackable NPC there (no NPC bit; not a players' race's
body, a pet or what a GM made peaceful in npcaggro, a dummy) whose name holds
one of its words, or every one but those it leaves out. A row is by NAME, as
factionhits is (a GM's #spawn copy counts too), so a name is given one only
when every NPC of that name stands in the place; one seen elsewhere too is
left to a GM and said. The number a kill moves is ours (+5): no record says.

- Feria Hurkert (EQOA-Data Quests/23-47/23.json, Hodstock parts two and
  three: 150 to be offered): "Kill Cracktooth Orcs (CONs LB) near Bobble By The
  Water. Return to Feria after getting Feria faction +150." Every Cracktooth by
  name, round Bobble-by-Water.
- Citizens of Castle Lightwolf (Quests/23-47/45.json, Kizden's and Pandrig's
  quests: 150): "Obtain faction by slaying beasts in Lady Vox's Castle,
  Permafrost." Every creature within 400 of Lady Vox, on the castle's floors
  (55 up and more): its goblins, giants, mages, hounds, drakes, mammoths, its
  named ones and Lady Vox.
- Citizens of Graumord (Quests/Portal/isle-of-dread.json, the trials: 1):
  "killing all mobs but sacrifice and citizen of graumord on the isle". Every
  mob of the Isle of Dread (its sirens wear a human's body) but the Graumord
  citizens, the imprisoned souls
  (the guide's sacrifice, ours: the citizens ask that they be freed) and
  Kithgrin, who sets the trials. (The Portal lane's repeatable "Cleansing the
  Isle" stood in for this; it may go.)

Rows are written with source "quest-seed", never over an existing row for the
same name and faction. Every row written is logged in quest_hit_seed; --undo
deletes them, but not one changed since (said), and drops the log; a second
--apply is refused while it is there. Both take a copy of the database first
(<db>.bak-<when>-pre-questhits). Report only by default. The running server
reads factionhits at its start and after any GM panel edit of Faction hits.

    tools/seed_quest_hits.py --db eqoa.db [--apply | --undo] [--delta 5] [--no-backup]
"""
import argparse
import collections
import datetime
import math
import os
import re
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
SOURCE = "quest-seed"
LOG = "quest_hit_seed"

LOG_SCHEMA = f"""CREATE TABLE IF NOT EXISTS {LOG} (
    rowid    INTEGER NOT NULL,  -- the factionhits row written
    npc_name TEXT NOT NULL,
    faction  TEXT NOT NULL,
    delta    INTEGER NOT NULL
)"""

# (faction, guide, world, place, words, leave out): the place a (x, z, radius)
# circle with a floor (y at least), or an (x0, z0, x1, z1) box; words the name
# must hold one of (None: any); leave out by name, lower case; people: a
# players' race's body counts too (the isle's sirens are its mobs)
GATES = [
    dict(faction="Feria Hurkert", world=0, circle=(24641, 11859, 1200), floor=None, words={"cracktooth"}, leave=set(),
         guide="Quests/23-47/23.json: \"Kill Cracktooth Orcs (CONs LB) near Bobble By The Water. Return to Feria after "
               "getting Feria faction +150.\""),
    dict(faction="Citizens of Castle Lightwolf", world=0, circle=(4861, 4397, 400), floor=55, words=None, leave=set(),
         guide="Quests/23-47/45.json: \"Obtain faction by slaying beasts in Lady Vox's Castle, Permafrost.\""),
    dict(faction="Citizens of Graumord", world=0, box=(24100, 2400, 25800, 3650), floor=None, words=None,
         leave={"graumord citizen", "imprisoned soul", "kithgrin"}, people=True,
         guide="Quests/Portal/isle-of-dread.json: \"killing all mobs but sacrifice and citizen of graumord on the isle\""),
]
DUMMY = re.compile(r"^(?:training|regular|sparring) dummy")


def key(name):
    """database.NPCClassKey: lower case, trimmed, no trailing '$'."""
    n = (name or "").strip().lower()
    if n.endswith("$"):
        n = n[:-1]
    return n.strip()


def inside(g, n):
    if n["world"] != g["world"]:
        return False
    if g.get("floor") is not None and n["y"] < g["floor"]:
        return False
    if "circle" in g:
        x, z, r = g["circle"]
        return math.hypot(n["x"] - x, n["z"] - z) <= r
    x0, z0, x1, z1 = g["box"]
    return x0 <= n["x"] <= x1 and z0 <= n["z"] <= z1


def plan(db, delta):
    """The rows (npc_name, faction, delta, gate) and, by gate, the names left
    and why."""
    models = {(m or 0) & 0xFFFFFFFF for (m,) in db.execute("SELECT modelid FROM charactermodel")}
    try:
        peaceful = {n for (n,) in db.execute("SELECT npc_name FROM npcaggro WHERE aggressive = 0")}
    except sqlite3.OperationalError:
        peaceful = set()
    factions = {f for (f,) in db.execute("SELECT name FROM factions")}
    have = {(n, f) for n, f in db.execute("SELECT npc_name, faction FROM factionhits")}
    npcs = [dict(name=r[0] or "", x=r[1] or 0, y=r[2] or 0, z=r[3] or 0, model=(r[4] or 0) & 0xFFFFFFFF, type=r[5] or 0,
                 world=r[6] or 0)
            for r in db.execute("""SELECT npc_name, x_coord, y_coord, z_coord, modelid, npc_type, world FROM npcs""")]
    by_name = collections.defaultdict(list)
    for n in npcs:
        by_name[n["name"]].append(n)
    rows, left = [], collections.defaultdict(list)
    for g in GATES:
        if g["faction"] not in factions:
            left[g["faction"]].append(f"no faction {g['faction']!r} in the factions table")
            continue
        names = set()
        for n in npcs:
            if not inside(g, n) or n["type"] & 0x80:
                continue
            k = key(n["name"])
            if not k or k in g["leave"] or DUMMY.match(k):
                continue
            if g["words"] is not None and not g["words"] & set(re.findall(r"[a-z]+", k)):
                continue
            if (n["model"] in models and not g.get("people")) or n["name"] in peaceful:
                continue  # a person (but the isle's sirens are its mobs) or a pet
            names.add(n["name"])
        for name in sorted(names):
            out = [o for o in by_name[name] if not inside(g, o)]
            if out:
                left[g["faction"]].append(f"{name!r}: {len(out)} more elsewhere")
                continue
            if (name, g["faction"]) in have:
                left[g["faction"]].append(f"{name!r}: a row already")
                continue
            rows.append((name, g["faction"], delta, g, len(by_name[name])))
    return rows, left


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-questhits"
    n = 2
    while os.path.exists(out):
        out = f"{path}.bak-{when}-{n}-pre-questhits"
        n += 1
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def apply(db, rows):
    db.execute("BEGIN")
    db.execute(LOG_SCHEMA)
    for name, faction, delta, _, _ in rows:
        cur = db.execute("INSERT INTO factionhits (npc_name, faction, delta, source) VALUES (?, ?, ?, ?)",
                         (name, faction, delta, SOURCE))
        db.execute(f"INSERT INTO {LOG} (rowid, npc_name, faction, delta) VALUES (?, ?, ?, ?)",
                   (cur.lastrowid, name, faction, delta))
    db.execute("COMMIT")


def undo(db):
    done, kept = 0, []
    db.execute("BEGIN")
    for rowid, name, faction, delta in list(db.execute(f"SELECT rowid, npc_name, faction, delta FROM {LOG}")):
        r = db.execute("SELECT npc_name, faction, delta FROM factionhits WHERE id = ?", (rowid,)).fetchone()
        if r is None:
            continue
        if r != (name, faction, delta):
            kept.append(f"factionhits row {rowid}: {r} now, the seed wrote {(name, faction, delta)}")
            continue
        db.execute("DELETE FROM factionhits WHERE id = ?", (rowid,))
        done += 1
    db.execute(f"DROP TABLE {LOG}")
    db.execute("COMMIT")
    return done, kept


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--apply", action="store_true", help="write the rows")
    ap.add_argument("--undo", action="store_true", help="delete what --apply wrote")
    ap.add_argument("--delta", type=int, default=5, help="what a kill adds (ours)")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    a = ap.parse_args(argv)
    db = sqlite3.connect(a.db, isolation_level=None)
    try:
        return run(ap, a, db)
    finally:
        db.close()


def run(ap, a, db):
    db.execute("PRAGMA busy_timeout = 5000")
    logged = db.execute("SELECT name FROM sqlite_master WHERE name = ?", (LOG,)).fetchone()
    if a.undo:
        if not logged:
            print(f"{LOG} is not there: nothing to delete")
            return 0
        if not a.no_backup:
            print("backup:", backup(db, a.db))
        done, kept = undo(db)
        print(f"deleted {done} rows")
        for k in kept:
            print("  left (changed since):", k)
        print("restart the server, or edit a Faction hits row in the GM panel, for the world to read it")
        return 0
    if a.apply and logged:
        ap.error(f"{LOG} is there: this database was seeded already (--undo first)")
    rows, left = plan(db, a.delta)
    for g in GATES:
        mine = [r for r in rows if r[3] is g]
        print(f"{g['faction']}: {len(mine)} names, {sum(r[4] for r in mine)} NPCs, {a.delta:+d} a kill ({g['guide']})")
        for name, _, _, _, n in mine:
            print(f"    {name!r} x{n}")
        for why in left.get(g["faction"], []):
            print(f"    left: {why}")
    if not a.apply:
        print("(report only: --apply writes them)")
        return 0
    if not a.no_backup:
        print("backup:", backup(db, a.db))
    apply(db, rows)
    print(f"wrote {len(rows)} rows; restart the server, or edit a Faction hits row in the GM panel, for the world to "
          f"read them")
    return 0


if __name__ == "__main__":
    sys.exit(main())
