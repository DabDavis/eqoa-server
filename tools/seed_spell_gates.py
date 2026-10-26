#!/usr/bin/env python3
"""Where the teleport spells send: table spellgates (entity/gates.go).

The wizard's gates, portals and evacuations and the druid's circles (jib's
review, F3 pass 19: 27 spells, wizard 20, druid 7, took their power and did
nothing). Each one's line names its place (EQOA-Data's spell lists, which
spelleffect holds: "Self, Teleports the caster to the Kara Village (s)",
"Group, Set Maximum Power to 0, Teleport to SBC"); this puts each place on
the game's coordinates. The sources, best first:

- THE DISC'S OWN MARKERS, read off the operator's disc (each world's zone
  actors): the wizard's six pyramids, one sprite (DictID 039FC839, a 101 by
  49 by 100 pyramid whose top is 30-31 above its origin) in Tunaria alone,
  in exactly the ninths the lines name: Kara Village (S), Baga Village (SE),
  Sslathis (C), and Druid's Watch (NW), Muniel's Tea Garden (W) and Mu Lin's
  Reach (S). The gates set a player on top ("Baga Gate Spell sends you to
  Baga Village (se) ZONE ... on top of a Pyramid ... safe place", "the
  pyramid you are standing on", "there is a KOS caster at top of Pyramid",
  Allakhazam's comments, archived, 2004-2006); the druid's stones where the
  Frontiers circles' seller stands (Sage Wythin "in Cyclops Fortress (W).
  He's on a hill with a druids stone"; the Barren Coast spell "teleports an
  entire group to that spot, and leaves you within sight of the gargoyle
  fortress"), and the ring of Surefall Glade ("This one ports you to the
  circle in Surefall Glade"; the druid ring where Urisai appears,
  FrontiersForge's sighting).
- The zone and its ninth, as players name a place: CLIENT's own zones
  (ZoneName's grids, transcribed in the decomp's vientity.cpp and
  eqoa-rust-client-vanilla's zones.rs; claude-notes/tunaria-map.md names
  Tunaria's), a world's zones 2,000-unit cells, and the ninth CLIENT's
  UpdateLocation reads (333.34 either side of the cell's middle).
- An NPC who stands there (this server's npcs): the Prophet of Kumaj
  ("ports you to Kerra Isle (SE) right next to the quest npc"), a town's
  coachman or waysender.

Which ninth the Melton, North Ro and Commons pyramids are: Melton's the one
north-west of the Bolash daemons' hill in Druid's Watch (s) ("s/e of the wiz
port-melton", EQOA-Data's Zaoran Plateau guide), North Ro's the one
"straight" west of Muniel's Tea Garden's middle ("MTG (c) - (straight east
of wiz port to north ro)", its Harbingers guide), and the Commons the one no
source names, Mu Lin's Reach (S): ours. Ours too: Kelinar (no pyramid: its
town, where its sorcerers' tower stands), Odus (Arcadin, where Freeport's
waysenders "send you to arcadin/odus"), Saerk Towers (NW) for the Kithicor
Circle (Allakhazam's comment names the ninth, not the spot), and the middles
of Sycamore Joy's Rest and Stonebrunt Mountains (the nearest level, open,
dry spot clear of creatures to each).

Whom each sends: Allakhazam's April 2004 records (archived): every Gate and
Commons Escape "Self Only" (Sandstorm's scrolls too, "Level 39 - Self
Teleport to Nothern Ro"), every Portal "Group" within 15 (the 60-second
ones) or 20 (Barren Coast, Rathe Mountains, Stonebrunt), every circle and
evacuation "Group" within 20 (the server's default); the evacuations go to
their caster's bind point (entity/gates.go's choice: their lines name
Freeport, Melton and Highpass, kept as the rows' places for a GM who makes
them group gates).

With the ground files (--ground worlds/ground: cmd/groundbake's) each
place's height is the terrain's there (a pyramid's top as written) and the
report says where the server's landing puts a player (entity/gates.go's
rules, done again here). Without them nothing is written.

    python3 -E -s tools/seed_spell_gates.py --db eqoa.db --ground worlds/ground            report
    python3 -E -s tools/seed_spell_gates.py --db eqoa.db --ground worlds/ground --apply    write it
    python3 -E -s tools/seed_spell_gates.py --db eqoa.db --undo                              take it back

Rows go in in one transaction, source seed, each logged in spell_gate_seed
with what was written; a spell that has a row already (a GM's) is left
alone and said. --undo deletes the rows a run wrote that no GM changed since
(a changed one is said and kept), drops the log, and the table when the
run made it and it is empty again. A copy of the database
is made first (--no-backup to skip). The server reads the table at its
start and at a GM's edit in the panel: restart it after.
"""
import argparse
import datetime
import json
import math
import os
import sqlite3
import sys

# the ground and environment readers are seed_wildlife's and seed_pod's (the
# server's own formats)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from seed_wildlife import Ground  # noqa: E402
from seed_pod import Env  # noqa: E402

DEFAULT_DB = "eqoa.db"
DEFAULT_GROUND = "worlds/ground"
SOURCE = "seed"
LOG = "spell_gate_seed"
MADE = "#made the table"  # the log's mark that a run made spellgates

SELF, GROUP, EVAC = "self", "group", "evac"

# a zone's cell, and the middle of each of its ninths as CLIENT's
# UpdateLocation names them (its thirds: 333.34 either side of the middle)
CELL = 2000.0
NINTH = {"NW": (-1, -1), "N": (0, -1), "NE": (1, -1), "W": (-1, 0), "C": (0, 0), "E": (1, 0),
         "SW": (-1, 1), "S": (0, 1), "SE": (1, 1)}
THIRD_MIDDLE = 666.67

# a wizard's pyramid (sprite 039FC839): its walkable top is 30 to 31 above
# its origin (its collision's platform; the creature Sslathis' top holds
# stands 30.0 above it); a player is set half a unit over the higher
PYRAMID_TOP = 31.5

# entity/gates.go's landing, done again for the report
RISE, OPEN, CLEAR, CROWDED, APART, ABOVE = 12.0, 8.0, 40.0, 15.0, 2.5, 6.0
STEPS = (0, 10, 20, 35, 50, 70, 90, 120, 150, 200)
STEP_UP, BODY_ABOVE, WATER = 2.0, 0.5, 1
# a level spot (for a zone's middle): the floor 3 out each way within 2.5,
# and 30 out every eighth of a turn within 15 (no pinnacle's flat top)
LEVEL_OUT, LEVEL_RISE, LEVEL_STEPS = 3.0, 2.5, range(0, 420, 20)
CLIFF_OUT, CLIFF_DROP = 30.0, 15.0


def pyramid(world, x, y, z, where):
    """On top of a wizard's pyramid read off the disc (its origin)."""
    return ("top", world, x, y + PYRAMID_TOP, z, f"the pyramid of {where} (sprite 039FC839 at ({x:.1f}, {y:.1f}, {z:.1f}), on top)")


def stone(world, x, y, z, what):
    """At a marker read off the disc, on the terrain there."""
    return ("spot", world, x, y, z, what)


def beside(name, world, what):
    """In front of this server's NPC of that name."""
    return ("npc", name, world, what)


def middle(world, x0, z0, ninth, zone):
    """The nearest level, open, dry spot clear of creatures to a zone's ninth's
    middle (its cell's corner (x0, z0), CLIENT's ZoneName grid): ours."""
    return ("cell", world, x0, z0, ninth, zone)


KARA = pyramid(0, 21329.2, 54.1, 7650.9, "Kara Village (S)")
BAGA = pyramid(0, 15671.1, 60.5, 11918.1, "Baga Village (SE)")
SSLATHIS = pyramid(0, 22708.7, 59.4, 29311.9, "Sslathis (C)")
MELTON = pyramid(0, 6310.9, 60.5, 18533.9, "Druid's Watch (NW), Melton's")
NORTH_RO = pyramid(0, 22233.2, 62.8, 19020.1, "Muniel's Tea Garden (W), North Ro's")
COMMONS = pyramid(0, 22910.9, 20.7, 13572.5, "Mu Lin's Reach (S), the one no source names: the Commons', ours")
SFG_RING = stone(0, 9282.0, 57.3, 12312.3, "Surefall Glade's druid ring (sprite CAD79993), where Urisai appears")
SBC_HILL = stone(2, 10915.0, 112.0, 9800.0, "South Barren Coast (S)'s hill by the coast, its top, rocks on its north-west shoulder, the "
                "gargoyles' fort (a ring of statue guardians on the mountain to the north-west, sprite CAD79993) in sight: ours, by the words")
RATHE_STONE = stone(1, 6251.0, 65.2, 2703.0, "Cyclops Fortress (W)'s druid stones on their hill (sprite 738C5FD5)")
KUMAJ = beside("Prophet of Kumaj", 2, "Kerra Isle (SE), by the Prophet of Kumaj")
KELINAR = stone(0, 12487.3, 68.2, 25490.2, "Kelinar's town (SW), among its four hums (sprite D9C129A4) by its sorcerers' tower: ours")
ARCADIN = beside("Waysender Yuresh", 2, "Arcadin, by its waysender: ours")
SAERK = middle(0, 20000, 12000, "NW", "Saerk Towers")
SYCAMORE = middle(0, 22000, 22000, "C", "Sycamore Joy's Rest")
STONEBRUNT = middle(2, 8000, 6000, "C", "Stonebrunt Mountains")

# Each gate: its spell (as spellpattern spells it), whom it sends, its group
# radius (0: the server's, 20), its place as a player reads it, where that
# is, and the words that say so (EQOA-Data's line first).
GATES = [
    # ---- the wizard's
    ("Nerius Gate", SELF, 0, "Kara Village", KARA,
     "Self, Teleports the caster to the Kara Village (s); 'Nerius Gate Spell sends you to the Kara Village (s) ZONE'"),
    ("Nerius Portal", GROUP, 15, "Kara Village", KARA,
     "Teleports the Group to the Kara Village (s); Allakhazam: Group, AE 15"),
    ("Baga Gate", SELF, 0, "Baga Village", BAGA,
     "self, Teleports the caster to the Baga Village (se); 'on top of a Pyramid ... safe place'"),
    ("Baga Portal", GROUP, 15, "Baga Village", BAGA,
     "Teleports the Group to the Baga Village (se); Allakhazam: Group, AE 15"),
    ("Sslathis Gate", SELF, 0, "Sslathis", SSLATHIS,
     "Self, Teleports caster to Sslathis (C); 'the same Pyramid ... a KOS caster at top of Pyramid'"),
    ("Melton Gate", SELF, 0, "Melton", MELTON,
     "Self, Teleports to Melton; Druid's Watch (s) is 's/e of the wiz port-melton'"),
    ("Melton Portal", GROUP, 15, "Melton", MELTON,
     "Self, Teleport to Melton (EQOA-Data); Allakhazam: Group, AE 15"),
    ("Melton Evacuation", EVAC, 0, "Melton", MELTON,
     "Group, Set Maximum Power to 0 and Teleport to Melton; Allakhazam: Group, AE 20"),
    ("Freeport Evacuation", EVAC, 0, "Freeport", beside("Coachman Ronks", 0, "Freeport, by its coachman: ours"),
     "Set Maximum Power to 0 Teleport group to FP; Allakhazam: Group, AE 20"),
    ("Highpass Evacuation", EVAC, 0, "Highpass Hold", beside("Coachman Quicksteed", 0, "Highpass Hold, by its coachman: ours"),
     "Group, Set Maximum Power to 0 and teleport to HP; Allakhazam: Group, AE 20"),
    ("Commons Escape", SELF, 0, "the Commons", COMMONS,
     "Self, Teleports ?, Increase Maximum Power by 10 for 12 seconds; Allakhazam: Self Only"),
    ("Commons Gate", SELF, 0, "the Commons", COMMONS,
     "Group, Teleports to Commons? (EQOA-Data); Allakhazam: Self Only; Sandstorm's scroll: Self Teleport to Commons gate"),
    ("North Ro Gate", SELF, 0, "Northern Ro", NORTH_RO,
     "Teleport group to N.Ro (EQOA-Data); Allakhazam: Self Only; 'MTG (c) - (straight east of wiz port to north ro)'"),
    ("North Ro Portal", GROUP, 15, "Northern Ro", NORTH_RO,
     "Group, Teleport to N.Ro; Allakhazam: Group, AE 15"),
    ("Kelinar's Gate", SELF, 0, "Kelinar", KELINAR,
     "Self, Teleports group to Kelinar (EQOA-Data); Allakhazam: Self Only"),
    ("Barren Coast Portal", GROUP, 0, "South Barren Coast", SBC_HILL,
     "Group, Set Maximum Power to 0 and teleport to Barren Coast; Sage Wythin's spot 'on a mountain in South Barren Coast (S) ... It teleports an entire group to that spot, and leaves you within sight of the gargoyle fortess'"),
    ("Rathe Mountains Portal", GROUP, 0, "Cyclops Fortress", RATHE_STONE,
     "Group, Set Maximum Power to 0 and teleport to Rathe; Allakhazam: Group, AE 20"),
    ("Stonebrunt Portal", GROUP, 0, "Stonebrunt Mountains", STONEBRUNT,
     "Group, Set Maximum Power to 0 and teleport to Stonebrunt; Allakhazam: Group, AE 20"),
    ("Odus gate", SELF, 0, "Arcadin", ARCADIN,
     "Self, Teleport to Odus"),
    ("Odus Portal", GROUP, 15, "Arcadin", ARCADIN,
     "Group, Teleport to Odus; Allakhazam: Group, AE 15"),
    # ---- the druid's
    ("Jaggedpine Circle", GROUP, 0, "Surefall Glade", SFG_RING,
     "Group, Set Max Power to 0, Teleport to druid circle in SFG; 'This one ports you to the circle in Surefall Glade'"),
    ("Kithicor Circle", GROUP, 0, "Saerk Towers", SAERK,
     "Grouop, Set Maximum Power to 0 and Teleports to Kithicor Forest; 'This one ports you to Saerk Towers (NW)'"),
    ("Sycamor Circle", GROUP, 0, "Sycamore Joy's Rest", SYCAMORE,
     "Group, Set Maximum Power to 0 and teleports to Sycamor"),
    ("Circle of Barren Coast", GROUP, 0, "South Barren Coast", SBC_HILL,
     "Group, Set Maximum Power to 0, Teleport to SBC; Sage Within 'in (S) portion of zone, on top of one of the hills, at the edge of rock formation, the hill is close to the coast'"),
    ("Circle of Rathe", GROUP, 0, "Cyclops Fortress", RATHE_STONE,
     "Group, Set Maximum Power to 0, Teleport to Rathe; 'Sage Wythin is located in Cyclops Fortress (W). He's on a hill with a druids stone'"),
    ("Circle of Stonebrunt", GROUP, 0, "Stonebrunt Mountains", STONEBRUNT,
     "Group, Set Maximum Power to 0, teleport to SBM"),
    ("Toxxulia Circle", GROUP, 0, "Kerra Isle", KUMAJ,
     "Group, Set Maximum Power to 0, teleport to Toxx; 'This spell ports you to Kerra Isle (SE) right next to the quest npc Prophet of Kumaj'"),
]


# ---- where -------------------------------------------------------------------------

def has_table(db, t):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (t,)).fetchone()[0] > 0


def floor_at(g, x, y, z):
    """The server's Ground.Floor: the highest terrain at (x, z) at most
    STEP_UP above y; None for none."""
    for h in g.floors(x, z):
        if h <= y + STEP_UP:
            return h
    return None


class Near:
    """A world's NPCs as the landing counts them: every body (by where it
    spawned), and the creatures that attack on sight (entity/aggro.go's
    hostileNatureLocked without its towns: no NPC bit, a body no player race
    wears, not a dummy)."""

    def __init__(self, db, world):
        races = {r[0] for r in db.execute("SELECT modelid FROM charactermodel")} if has_table(db, "charactermodel") else set()
        self.bodies, self.hostile = [], []
        for x, z, t, n, m in db.execute(
                "SELECT x_coord, z_coord, npc_type, npc_name, modelid FROM npcs WHERE world = ?", (world,)):
            self.bodies.append((x, z))
            if races and not (t or 0) & 0x80 and m and m not in races and "dummy" not in (n or "").lower():
                self.hostile.append((x, z))

    def room(self, x, z):
        return min([CLEAR] + [math.hypot(cx - x, cz - z) for cx, cz in self.hostile
                              if abs(cx - x) <= CLEAR and abs(cz - z) <= CLEAR])

    def apart(self, x, z):
        return all(math.hypot(cx - x, cz - z) >= APART for cx, cz in self.bodies
                   if abs(cx - x) <= APART and abs(cz - z) <= APART)


def open_dry(g, env, x, fy, z):
    top = floor_at(g, x, fy + 1000, z)
    if top is not None and top > fy + OPEN:
        return False
    return env is None or not env.flags(x, fy + BODY_ABOVE, z) & WATER


def landing(g, env, x, y, z, near):
    """entity/gates.go's landing: (x, y, z, how far from the place, room), the
    place as written when it stands well above the terrain."""
    base = floor_at(g, x, y + RISE, z)
    if base is not None and base < y - ABOVE:
        return x, y, z, 0.0, CLEAR
    if base is None:
        base = floor_at(g, x, 1e6, z)
    if base is None:
        return x, y, z, 0.0, 0.0
    best = None
    for d in STEPS:
        for k in range(1 if d == 0 else 16):
            h = k * math.pi / 8
            px, pz = x + d * math.cos(h), z + d * math.sin(h)
            fy = floor_at(g, px, base + RISE, pz)
            if fy is None or abs(fy - base) > RISE or not near.apart(px, pz) or not open_dry(g, env, px, fy, pz):
                continue
            r = near.room(px, pz)
            if r >= CLEAR:
                return px, fy, pz, d, r
            if best is None or r > best[4]:
                best = (px, fy, pz, d, r)
    if best is not None and best[4] >= CROWDED:
        return best
    return x, base, z, 0.0, 0.0


def level_spot(g, env, x, z, near):
    """The nearest level, open, dry spot clear of creatures to (x, z): its
    topmost floor, the floor LEVEL_OUT off each way within LEVEL_RISE of it,
    and CLIFF_OUT off every eighth of a turn within CLIFF_DROP (no cliff
    edge, no pinnacle's top)."""
    for d in LEVEL_STEPS:
        for k in range(1 if d == 0 else 16):
            h = k * math.pi / 8
            px, pz = x + d * math.cos(h), z + d * math.sin(h)
            fy = floor_at(g, px, 1e6, pz)
            if fy is None or not open_dry(g, env, px, fy, pz) or near.room(px, pz) < CLEAR or not near.apart(px, pz):
                continue
            ok = True
            for ox, oz in ((LEVEL_OUT, 0), (-LEVEL_OUT, 0), (0, LEVEL_OUT), (0, -LEVEL_OUT)):
                f2 = floor_at(g, px + ox, fy + LEVEL_RISE, pz + oz)
                if f2 is None or abs(f2 - fy) > LEVEL_RISE:
                    ok = False
                    break
            for j in range(8 if ok else 0):
                hh = j * math.pi / 4
                f2 = floor_at(g, px + CLIFF_OUT * math.cos(hh), fy + CLIFF_DROP, pz + CLIFF_OUT * math.sin(hh))
                if f2 is None or abs(f2 - fy) > CLIFF_DROP:
                    ok = False
                    break
            if ok:
                return px, fy, pz, d
    return None


def anchor_of(db, a, ground):
    """A gate's place (world, x, y, z, what says so), or None with why. ground
    is ground(world) -> (Ground, Env, Near) or None."""
    kind = a[0]
    if kind == "top":
        _, world, x, y, z, what = a
        return (world, x, y, z, what), ""
    if kind in ("spot", "npc"):
        if kind == "spot":
            _, world, x, y, z, what = a
        else:
            _, name, world, what = a
            r = db.execute("SELECT x_coord, y_coord, z_coord, facing FROM npcs WHERE npc_name = ? AND world = ? ORDER BY id LIMIT 1",
                           (name, world)).fetchone()
            if r is None:
                return None, f"no NPC {name} here"
            # in front of it (handlers' inFrontOf: 3 along its heading)
            h = (r[3] or 0) / 256.0 * 2 * math.pi
            x, y, z = r[0] + 3 * math.sin(h), r[1], r[2] + 3 * math.cos(h)
        gr = ground(world)
        if gr is not None:
            fy = floor_at(gr[0], x, y + RISE, z)
            if fy is None or fy < y - RISE:
                fy = floor_at(gr[0], x, 1e6, z)
            if fy is None:
                return None, f"no terrain at {what}"
            y = fy
        return (world, x, y, z, what), ""
    if kind == "cell":
        _, world, x0, z0, ninth, zone = a
        dx, dz = NINTH[ninth]
        x, z = x0 + CELL / 2 + dx * THIRD_MIDDLE, z0 + CELL / 2 + dz * THIRD_MIDDLE
        gr = ground(world)
        what = f"{zone} ({ninth}), CLIENT's cell ({x0:.0f}, {z0:.0f})"
        if gr is None:
            return (world, x, None, z, what), ""
        found = level_spot(gr[0], gr[1], x, z, gr[2])
        if found is None:
            return None, f"no level spot near {what}'s middle"
        lx, ly, lz, d = found
        return (world, lx, ly, lz, f"{what}: the nearest level, open, dry spot clear of creatures to its middle, {d:.0f} off: ours"), ""
    raise ValueError(a)


def plan(db, ground_dir):
    cache = {}

    def ground(world):
        if not ground_dir:
            return None
        if world not in cache:
            gp = os.path.join(ground_dir, f"{world}.ground")
            ep = os.path.join(ground_dir, f"{world}.env")
            cache[world] = (Ground(gp), Env(ep) if os.path.exists(ep) else None, Near(db, world)) if os.path.exists(gp) else None
        return cache[world]

    rows, missing = [], []
    for spell, kind, radius, place, a, why in GATES:
        at, err = anchor_of(db, a, ground)
        if at is None:
            missing.append(f"{spell}: {err}")
            continue
        world, x, y, z, src = at
        land = None
        gr = ground(world)
        if gr is not None and y is not None:
            land = landing(gr[0], gr[1], x, y, z, gr[2])
        exists = db.execute("SELECT count(*) FROM spellpattern WHERE lower(trim(spellname)) = ?",
                            (spell.lower(),)).fetchone()[0] if has_table(db, "spellpattern") else 0
        rows.append(dict(spell=spell.lower(), kind=kind, place=place, world=world, x=round(x, 1),
                         y=None if y is None else round(y, 1), z=round(z, 1), radius=float(radius),
                         note=f"{why}. Place: {src}", source=SOURCE, _land=land, _records=exists, _name=spell))
    for gr in cache.values():
        if gr is not None:
            gr[0].f.close()
    return rows, missing


# ---- the table ---------------------------------------------------------------------

SCHEMA = """CREATE TABLE IF NOT EXISTS spellgates (
    spell  TEXT PRIMARY KEY,
    kind   TEXT NOT NULL DEFAULT 'self',
    place  TEXT NOT NULL DEFAULT '',
    world  INTEGER NOT NULL DEFAULT 0,
    x      REAL NOT NULL DEFAULT 0,
    y      REAL NOT NULL DEFAULT 0,
    z      REAL NOT NULL DEFAULT 0,
    radius REAL NOT NULL DEFAULT 0,
    note   TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'gm'
)"""
COLS = ("spell", "kind", "place", "world", "x", "y", "z", "radius", "note", "source")


def apply(db, rows):
    if has_table(db, LOG):
        sys.exit(f"this database holds a run already ({LOG}): --undo it first")
    if any(r["y"] is None for r in rows):
        sys.exit("not written: a place has no height (--ground worlds/ground)")
    wrote, kept = [], []
    db.execute("BEGIN IMMEDIATE")
    try:
        made = not has_table(db, "spellgates")
        db.execute(SCHEMA)  # the server makes it too; a database it has not opened yet
        db.execute(f"CREATE TABLE {LOG} (spell TEXT PRIMARY KEY, row TEXT NOT NULL, applied TEXT NOT NULL)")
        when = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds")
        if made:
            db.execute(f"INSERT INTO {LOG} VALUES (?, '[]', ?)", (MADE, when))
        for r in rows:
            if db.execute("SELECT count(*) FROM spellgates WHERE lower(trim(spell)) = ?", (r["spell"],)).fetchone()[0]:
                kept.append(r["_name"])
                continue
            vals = [r[c] for c in COLS]
            db.execute(f"INSERT INTO spellgates ({', '.join(COLS)}) VALUES ({', '.join('?' * len(COLS))})", vals)
            db.execute(f"INSERT INTO {LOG} VALUES (?, ?, ?)", (r["spell"], json.dumps(vals), when))
            wrote.append(r["_name"])
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return wrote, kept


def undo(db):
    if not has_table(db, LOG):
        sys.exit("no run to undo here")
    deleted, changed = 0, []
    db.execute("BEGIN IMMEDIATE")
    try:
        made = False
        for spell, row in db.execute(f"SELECT spell, row FROM {LOG} ORDER BY spell").fetchall():
            if spell == MADE:
                made = True
                continue
            now = db.execute(f"SELECT {', '.join(COLS)} FROM spellgates WHERE spell = ?", (spell,)).fetchone()
            if now is None:
                continue
            if list(now) != json.loads(row):
                changed.append(spell)
                continue
            deleted += db.execute("DELETE FROM spellgates WHERE spell = ?", (spell,)).rowcount
        db.execute(f"DROP TABLE {LOG}")
        if made and db.execute("SELECT count(*) FROM spellgates").fetchone()[0] == 0:
            db.execute("DROP TABLE spellgates")  # this run made it
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return deleted, changed


def backup(db, path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = f"{path}.bak-{when}-pre-spellgates"
    dst = sqlite3.connect(out)
    db.backup(dst)
    dst.close()
    return out


def report(rows, missing):
    worlds = {0: "Tunaria", 1: "Rathe", 2: "Odus"}
    print(f"{len(rows)} gates: {sum(r['kind'] == SELF for r in rows)} self, {sum(r['kind'] == GROUP for r in rows)} group, "
          f"{sum(r['kind'] == EVAC for r in rows)} evacuations")
    for r in rows:
        y = "?" if r["y"] is None else f"{r['y']:.1f}"
        line = f"  {r['_name']:24} {r['kind']:5} {r['place']:28} {worlds.get(r['world'], r['world'])} ({r['x']:.0f}, {y}, {r['z']:.0f})"
        if r["_land"] is not None:
            lx, ly, lz, d, room = r["_land"]
            line += f" lands {d:.0f} off at ({lx:.0f}, {ly:.1f}, {lz:.0f})" + ("" if room >= CLEAR else f", {room:.0f} from a creature")
        if not r["_records"]:
            line += " [no spellpattern record of the name]"
        print(line)
    for m in missing:
        print("  left out:", m)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--ground", default=DEFAULT_GROUND, help="the ground files' folder (worlds/ground)")
    ap.add_argument("--apply", action="store_true", help="write the rows")
    ap.add_argument("--undo", action="store_true", help="delete the rows a run wrote that no GM changed")
    ap.add_argument("--no-backup", action="store_true", help="write without a copy first")
    args = ap.parse_args()
    db = sqlite3.connect(args.db, isolation_level=None)
    if args.undo:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        deleted, changed = undo(db)
        print(f"deleted {deleted} rows" + (f"; kept, changed since: {', '.join(changed)}" if changed else ""))
        return
    ground = args.ground if os.path.isdir(args.ground) else None
    if ground is None:
        print(f"(no ground files at {args.ground}: no heights, nothing can be written)")
    rows, missing = plan(db, ground)
    report(rows, missing)
    if args.apply:
        if not args.no_backup:
            print("backup:", backup(db, args.db))
        wrote, kept = apply(db, rows)
        print(f"wrote {len(wrote)} rows" + (f"; left alone, a row there already: {', '.join(kept)}" if kept else "")
              + "; restart the server")
    else:
        print("(report only: --apply to write it)")


if __name__ == "__main__":
    main()
