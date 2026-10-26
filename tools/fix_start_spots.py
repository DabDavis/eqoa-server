#!/usr/bin/env python3
"""The new characters' start spots (defaultcharacter) checked against each
class's first quest giver, and the few that are wrong put right.

defaultcharacter holds the live game's spots: Sandstorm's new characters stand
exactly on them (F3 pass 17: a new Freeport warrior on the human-east warrior's
row, a new Surefall ranger on its row), and all but a few stand within 17 of
the NPC that gives the class its first quest, as EQOA-Data's 1-20 guides name
it (Quests/1-20/<city>/<class>.json, the "Level 1" section's "Starting NPC" or
"Quest NPC"). So a spot is kept, and only these are moved, to MOVE_TO in front
of that NPC (the way it faces, CLIENT's heading: the direction (sin h, cos h),
an NPC's facing byte times 2 pi / 256), at its height:

- a spot farther than FAR from its class's first quest giver: the human-east
  cleric's row is a copy of the warrior's (51 from Denouncer Alshea, the
  Freeport cleric's guildmaster), and the gnome necromancer's stands 86 from
  Grear Hosrottle;
- a spot within INSIDE of any NPC, where a new character would start inside it
  (Neriak's rogue on Guildmaster Kriyn, Moradhim's cleric on Spellsmith Huldor).

A moved spot was looked at in the game (vanilla on a private server, F3 pass
17): each on the floor of its quest giver's room, facing it; where it was not
(Moradhim's cleric, on a brazier) SPOT_OVERRIDES holds the spot that was.

Every spot within FAR of its quest giver is turned to face it. Rivervale's
warrior guide is a copy of its rogue guide (Walt Deeppockets) and is not read:
that row stands at Sheriff Hopper's.

The server reads the table at each creation (database/newchar.go), so nothing
needs a restart. Every row changed is logged in start_spot_log with what it
was; --undo puts those values back exactly and empties the log. Report only by
default.

    tools/fix_start_spots.py --guides ../external/EQOA-Data/Quests/1-20 [--db eqoa.db]
        [--apply | --undo]
"""
import argparse
import difflib
import json
import math
import os
import re
import sqlite3
import sys

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
FAR = 30.0      # a start spot farther than this from its quest giver is wrong
INSIDE = 1.0    # a start spot this close to an NPC starts the character inside it
MOVE_TO = 3.0   # a moved spot stands this far in front of the quest giver

CLASSES = ["warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid",
           "shaman", "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"]
RACES = ["Human", "Elf", "Dark Elf", "Gnome", "Dwarf", "Troll", "Barbarian", "Halfling",
         "Erudite", "Ogre", "Froglok"]
# guides that are another class's copied (the same Level 1 text): not read
COPIED_GUIDES = {("rivervale", "warrior")}
# (race, humType, class): a moved spot looked at in the game where MOVE_TO in
# front of the quest giver is not a place to stand. Moradhim's cleric: 3 in
# front of Bishop Gunder is his brazier (the client stood a character on it,
# 1.4 up); 5 in front stands on the floor with the brazier between them.
SPOT_OVERRIDES = {(4, 0, 9): (15445.7, 83.0, 8746.5)}

LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS start_spot_log (
    defaultCharacterID INTEGER PRIMARY KEY,
    x REAL, y REAL, z REAL, facing REAL,
    why TEXT NOT NULL
)"""


def city_of(race, hum, cls):
    """The guides' folder of a race's (and a human's city's) class, as
    EQOA-Data's starting cities list has them (database.HomeCity)."""
    if race == 0:
        if hum == 2:
            return "surefall-glade" if cls in (1, 7) else "qeynos"
        return "freeport"
    if race == 1:
        return "tethelin" if cls in (1, 5, 6, 7) else "fayspire"
    return {2: "neriak-nektulos", 3: "klick-anon", 4: "moradhim", 5: "grobb", 6: "halas",
            7: "rivervale", 8: "highbourne", 9: "oggok"}.get(race)


def first_quest_giver(guides, city, cls):
    """The NPC the class's first quest starts at, as its guide names it."""
    if city is None or (city, CLASSES[cls]) in COPIED_GUIDES:
        return None
    path = os.path.join(guides, city, CLASSES[cls] + ".json")
    if not os.path.exists(path):
        return None
    with open(path) as fh:
        guide = json.load(fh)["guide"]
    level1 = re.search(r"#\s*Level\s*1\b(.*?)(?=\n#\s*Level|\Z)", guide, re.S)
    if not level1:
        return None
    m = re.search(r"\*\*(?:Starting|Quest) NPC:\*\*\s*([^\n]+)", level1.group(1))
    return m.group(1).strip() if m else None


def norm(name):
    """A name as the guides and the NPC table both spell it: no case, ` for ',
    no ", the Guildmaster", Prof./Lt. in full."""
    n = name.lower().replace("`", "'").replace("\\", "").replace("’", "'").replace('"', "'")
    n = re.sub(r",.*$", "", n)
    n = re.sub(r"\bprof\.", "professor", n)
    n = re.sub(r"\blt\.", "lieutenant", n)
    n = re.sub(r"'+ ?", "'", n)
    return " ".join(n.split())


def same_npc(a, b):
    """Whether two names, normalised (norm), are one NPC's."""
    if a == b:
        return True
    if min(len(a), len(b)) >= 10 and (a.startswith(b) or b.startswith(a)):
        return True  # a name the table cut short ("Werlib Quack")
    words, bwords = a.split(), b.split()
    if len(bwords) == 1 and len(b) >= 4 and (b == words[0] or b == words[-1]):
        return True  # "Kriyn" for "Guildmaster Kriyn", "Deke" for "Deke Gabbins"
    if len(bwords) >= 2 and a.endswith(" " + b):
        return True  # "Field General Oxfist" for "General Oxfist"
    # a spelling of the same name ("Anaebarum"/"Anaebraum", "Heirophant"/"Hierophant",
    # "Felilnthar"/"Fethinar"), the last word alike when it is a long one
    if len(words) != len(bwords) or len(words) < 2 or abs(len(a) - len(b)) > 3:
        return False
    if words[0] != bwords[0] and words[-1] != bwords[-1] and words[0][:2] != bwords[0][:2]:
        return False
    ratio = difflib.SequenceMatcher(None, a, b).ratio()
    return ratio >= 0.88 or (ratio >= 0.8 and words[-1] == bwords[-1] and len(words[-1]) >= 6)


def heading(dx, dz):
    """CLIENT's heading of a direction (VIVect3::HPR: atan2(x, z)) in [0, 2 pi)."""
    return math.atan2(dx, dz) % (2 * math.pi)


def plan(db, guides):
    npcs = db.execute("SELECT id, npc_name, x_coord, y_coord, z_coord, COALESCE(facing, 0), world FROM npcs").fetchall()
    rows = db.execute("""SELECT defaultCharacterID, race, humType, tclass, x, y, COALESCE(z, 0), facing,
        COALESCE(world, 0) FROM defaultcharacter ORDER BY race, humType, tclass""").fetchall()
    by_world = {}
    for n in npcs:
        by_world.setdefault(n[6], []).append(n + (norm(n[1]),))
    changes = []
    for (rid, race, hum, cls, x, y, z, facing, world) in rows:
        label = f"{RACES[race] if 0 <= race < len(RACES) else race} {'east ' if race == 0 and hum == 1 else 'west ' if race == 0 and hum == 2 else ''}{CLASSES[cls] if 0 <= cls < 15 else cls}"
        city = city_of(race, hum, cls)
        giver = first_quest_giver(guides, city, cls)
        same_world = by_world.get(world, [])
        key = norm(giver) if giver else None
        cands = [n for n in same_world if key and same_npc(n[7], key)]
        npc = min(cands, key=lambda n: math.dist((x, z), (n[2], n[4]))) if cands else None
        inside = [n for n in same_world if math.dist((x, z), (n[2], n[4])) < INSIDE]
        why = None
        if npc is not None and math.dist((x, z), (npc[2], npc[4])) > FAR:
            why = f"{math.dist((x, z), (npc[2], npc[4])):.1f} from {npc[1].strip()}, its first quest giver"
        elif inside:
            why = f"inside {inside[0][1].strip()}"
        new = None
        if why and (race, hum, cls) in SPOT_OVERRIDES and npc is not None:
            nx, ny, nz = SPOT_OVERRIDES[(race, hum, cls)]
            new = (nx, ny, nz, heading(npc[2] - nx, npc[4] - nz))
        elif why and npc is not None:
            h = npc[5] * 2 * math.pi / 256
            nx, nz = npc[2] + MOVE_TO * math.sin(h), npc[4] + MOVE_TO * math.cos(h)
            new = (nx, npc[3], nz, (h + math.pi) % (2 * math.pi))
        elif why and inside:
            n = inside[0]
            h = n[5] * 2 * math.pi / 256
            nx, nz = n[2] + MOVE_TO * math.sin(h), n[4] + MOVE_TO * math.cos(h)
            new = (nx, n[3], nz, (h + math.pi) % (2 * math.pi))
        elif npc is not None:
            face = heading(npc[2] - x, npc[4] - z)
            if abs(face - facing) > 1e-4:
                new = (x, y, z, face)
                why = f"faces {npc[1].strip()}, its first quest giver, {math.dist((x, z), (npc[2], npc[4])):.1f} off"
        changes.append((rid, label, city, giver, npc, (x, y, z, facing), new, why))
    return changes


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--guides", help="EQOA-Data's Quests/1-20 directory")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write the changes (logged in start_spot_log)")
    g.add_argument("--undo", action="store_true", help="put every logged row back as it was")
    a = ap.parse_args()
    db = sqlite3.connect(a.db, isolation_level=None)

    if a.undo:
        db.execute(LOG_SCHEMA)
        logged = db.execute("SELECT defaultCharacterID, x, y, z, facing FROM start_spot_log").fetchall()
        db.execute("BEGIN IMMEDIATE")
        try:
            for rid, x, y, z, facing in logged:
                db.execute("UPDATE defaultcharacter SET x = ?, y = ?, z = ?, facing = ? WHERE defaultCharacterID = ?",
                           (x, y, z, facing, rid))
            db.execute("DELETE FROM start_spot_log")
            db.execute("COMMIT")
        except BaseException:
            db.execute("ROLLBACK")
            raise
        print(f"{len(logged)} start spots put back as they were")
        return

    if not a.guides or not os.path.isdir(a.guides):
        sys.exit("--guides (EQOA-Data's Quests/1-20) is required")
    changes = plan(db, a.guides)
    moved = turned = 0
    for (rid, label, city, giver, npc, old, new, why) in changes:
        if new is None:
            note = "kept" if npc is not None else f"kept (no first quest giver found{': ' + giver if giver else ''})"
            print(f"  {rid:3d} {label:24s} {note}")
            continue
        if (new[0], new[2]) != (old[0], old[2]):
            moved += 1
            print(f"  {rid:3d} {label:24s} MOVED ({old[0]:.1f}, {old[1]:.1f}, {old[2]:.1f}) -> "
                  f"({new[0]:.1f}, {new[1]:.1f}, {new[2]:.1f}) facing {new[3]:.3f}: {why}")
        else:
            turned += 1
            print(f"  {rid:3d} {label:24s} facing {old[3]:.3f} -> {new[3]:.3f}: {why}")
    print(f"{len(changes)} start spots: {moved} moved, {turned} turned to their quest giver")
    if not a.apply:
        print("(report only: --apply to write it)")
        return
    db.execute(LOG_SCHEMA)
    db.execute("BEGIN IMMEDIATE")
    try:
        for (rid, label, city, giver, npc, old, new, why) in changes:
            if new is None:
                continue
            # the first value logged is the one --undo restores
            db.execute("INSERT OR IGNORE INTO start_spot_log (defaultCharacterID, x, y, z, facing, why) VALUES (?, ?, ?, ?, ?, ?)",
                       (rid, old[0], old[1], old[2], old[3], why))
            db.execute("UPDATE defaultcharacter SET x = ?, y = ?, z = ?, facing = ? WHERE defaultCharacterID = ?",
                       (new[0], new[1], new[2], new[3], rid))
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"applied to {a.db}")


if __name__ == "__main__":
    main()
