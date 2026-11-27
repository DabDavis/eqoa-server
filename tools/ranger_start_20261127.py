#!/usr/bin/env python3
"""Every race's rangers but the elves' start in Surefall Glade, at peace with it and with Qeynos, and get its ranger line
(Heirloom petition 3355, an erudite ranger with no quests after the off-race starters; the owner, 2026-11-27:
"treat them like monks start every race there with proper faction fixed", then "not elf though"). The monk start's
twin (tools/monk_start.py, claude-notes/monk-start-20261117.md), in the petfix pattern.

The glade is the western human ranger's own start row (defaultcharacter race 0, humType 2, class 1: world 0
(9005.1, 56.8, 12749.2), by Sigmor Fallbourne, Spiritmaster Barkwillow 60 away, Coachman Frender 130). Elves (race 1)
keep their own ranger start, standings and line (Fayspires: Tethelin, Lythen Trueshot): no row, standing or quest of
theirs is touched, and race 1 is never added to a quest here.

What is written (one transaction, each row with its gm_audit line and its petition_fix_log line, petition 3355):

starts     defaultcharacter: every ranger row of a race but the western humans' and the elves' (the eastern humans',
           whose row the froglok and a human of no city take too, and each race of racedefs that has rows: their
           homelands' rangers) put at the glade, world, place and facing; a race with no ranger row gets one there
           (the game's races 2-9 and the eastern humans), its stats its own lowest row's of a class the western
           humans have too plus the western ranger's over that class's (a race keeps its bonuses), its level, money
           and training points that row's. A class with no row of its own starts on its race's lowest class's row
           (database.placeOf), so where the ranger's would become it (the erudites': no warrior row, the paladin's the
           lowest) a warrior's row is made where that lowest row stands: every other class starts where it does now
           (checked, class by class). A new character is bound where it starts (database.CreateNew). A race of
           racedefs with no rows takes its wire race's (an elf's wire race keeps the elves' Fayspires row: flagged).
factions   factionstart: each of those races' rangers (and each racedefs race's) starts at least at FLOOR with
           SET (Citizens of Surefall Glade, Citizens of Qeynos, The Qeynos Guard: the factions of every NPC in the
           glade, by its coach and in Qeynos, where the line's Samnar stands): the western human ranger's own start,
           never under 0, or the race's own where higher. A row (race, 1, -1) ((0, 1, 1) for a human of the east)
           where the race's start is lower; the race's other standings are its own.
existing   charfactions: every ranger character but the western humans' and the elves' gets its standing with each
           SET faction raised to the floor where it is below it (its own row raised, or one written where it has none
           and its start as it was is below). Only raised, never lowered; no place touched.
quests     the Surefall Glade ranger line 1-20 (Sigmor Fallbourne, source "EQOA-Data 1-20/surefall-glade/ranger",
           class 1) to every race of the game but the elves (0, 2-10) and every racedefs race by its own id (a
           racedefs race passes a quest by its wire race only when that is listed: handlers.questRaceTakes, and an
           elf's wire race is not); race 1 kept only where it is there already. petfix_3355 put race 8 there before:
           this builds on it.
epic       (unless --no-epic) the good-melee level 60 epic for those rangers: The Recruiter for rangers and monks
           (101043, "EQOA-Data 60/good-melee/start-g3") gains the races it lacks of 2, 3, 5, 6, 7, 9 (the dwarves keep
           their own Recruiter, 109802: tools/dwarf_epic_choice_20261122.py), and the line's episodes
           ("EQOA-Data 60/good-melee/ep*") gain 2, 5, 9 where they lack them. The rangers have no evil-melee
           Recruiter, so this line is theirs. The quests are shared with the monks (101043 is class "1, 4"): a monk of
           those races gains it too.
dialog     npcdialog: the glade's people and guards (town "Surefall Glade") welcome a ranger of any race but the
           humans (class 1, not_race 0; lines an elf may hear as well).

Left as they are: the off-race ranger starters in the cities (source "offrace-20261121 (petition 2540)"): a new ranger
starts in the glade and never meets them, and the existing ones there (one in progress on Heirloom) keep them.

WHEN IT TAKES EFFECT: factionstart, quests and npcfaction are read at the server's start, a character's charfactions
at its login and written over from memory at its logout, npcdialog within 30 s, defaultcharacter at each creation:
run --apply and --undo with the server stopped (a keep restart's before.sh). No new binary: every column it writes is
read by the server since the monk start (npcdialog.class, not_race).

    ranger_start_20261127.py --db eqoa.db --shard heirloom|live [--applied FILE] [--no-epic] [--apply | --undo]
"""
import argparse
import json
import math
import os
import sqlite3
import sys
import time

LOG = "petition_fix_log"
HERE = os.path.dirname(os.path.abspath(__file__))
KEYS = {"defaultcharacter": ("defaultCharacterID",), "factionstart": ("id",), "charfactions": ("serverid", "faction"),
        "quests": ("questid",), "npcdialog": ("id",)}
REFUSE = []
PET = 3355
DETAIL = ("ranger_start_20261127.py: every race's rangers but the elves' start in Surefall Glade, at peace with it and "
          "Qeynos, and get its ranger line (petition 3355; the owner 2026-11-27)")
SOURCE = "ranger-start"

RANGER, MONK, WARRIOR = 1, 4, 0
CLASSES = range(15)
ELF, FROGLOK, FIRST_CUSTOM = 1, 10, 11
SET = ["Citizens of Surefall Glade", "Citizens of Qeynos", "The Qeynos Guard"]
FLOOR_MIN = 0
KOS_BELOW = -350  # entity.FactionKOSBelow
# Qeynos (its centre), the glade, its coach: an NPC within these is one a new ranger meets
AREA = [(4400.0, 17200.0, 1000.0), (9005.1, 12749.2, 450.0), (9136.0, 12737.0, 100.0)]
WORLD = 0
LINE_SRC = "EQOA-Data 1-20/surefall-glade/ranger"
EPIC_START = "EQOA-Data 60/good-melee/start-g3"
EPIC_EPS = "EQOA-Data 60/good-melee/ep"
EPIC_START_ADD = [2, 3, 5, 6, 7, 9]
EPIC_EPS_ADD = [2, 5, 9]
STATS = ["defaultStrength", "defaultStamina", "defaultAgility", "defaultDexterity", "defaultWisdom",
         "defaultIntelligence", "defaultCharisma"]
# (role, line, weight): %b brother or sister, %r the listener's race, %p its name (entity.DialogSay)
LINES = [
    ("", "Welcome to the glade, %b. Whatever land you were born to, the forest takes a ranger's measure by the trail, "
         "not the blood.", 3),
    ("", "Sigmor says you have the makings of a fine tracker, %p. The glade is your home for as long as you keep its "
         "trees.", 3),
    ("", "Whether %r or human, it is a ranger's patience the woods ask of you, %b, and you have it. Karana's wind is "
         "at your back.", 3),
    ("", "The glade shelters every ranger who guards it, %p. Walk softly and you will want for nothing here.", 3),
    ("guard", "A ranger of the glade is a friend of its watch, %p. Keep the woods and we will keep you.", 3),
    ("guard", "Sigmor Fallbourne vouches for every ranger trained here, %b. You will have no trouble from us.", 3),
]
RACE_NAMES = ["Human", "Elf", "Dark Elf", "Gnome", "Dwarf", "Troll", "Barbarian", "Halfling", "Erudite", "Ogre",
              "Froglok"]


def dumps(v):
    return json.dumps(v, separators=(",", ":"), ensure_ascii=False)


def same(a, b):
    if isinstance(a, float) or isinstance(b, float):
        try:
            return abs(float(a) - float(b)) < 1e-6
        except (TypeError, ValueError):
            return False
    return a == b


def rows(db, sql, args=()):
    cur = db.execute(sql, args)
    names = [c[0] for c in cur.description]
    return [dict(zip(names, r)) for r in cur.fetchall()]


def has_table(db, name):
    return db.execute("SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone() is not None


def ints(s):
    return [int(x) for x in (s or "").replace(" ", "").split(",") if x.strip().lstrip("-").isdigit()]


def races_text(rs):
    return ", ".join(str(r) for r in sorted(set(rs)))


# ---- the server's rules (as tools/monk_start.py reads them) ---------------------------------------------------------

def combo_of(race, cls, hum):
    """entity.comboOf: a froglok and a human of no west city as the east's, any other race's city 0."""
    if race == FROGLOK or (race == 0 and hum != 2):
        return (0, cls, 1)
    if race != 0:
        hum = 0
    return (race, cls, hum)


def specificity(r):
    named = [v >= 0 for v in r[:3]]
    return (sum(named), named[0], named[1], named[2])


def resolve(start_rows, base, combo):
    """entity.startsFor: the most specific row (race, class, humtype, standing) holding the combo, else base."""
    best = None
    for r in start_rows:
        if all(v < 0 or v == c for v, c in zip(r[:3], combo)) and (best is None or specificity(r) > specificity(best)):
            best = r
    return best[3] if best else base


def place_of(dc, race, hum, cls):
    """database.placeOf on defaultcharacter rows (own rows; a froglok as an eastern human)."""
    if race == FROGLOK:
        race, hum = 0, 1
    mine = [r for r in dc if r["race"] == race]
    if not mine:
        return None
    mine.sort(key=lambda r: (not (r["humType"] == hum), not (r["tclass"] == cls), r["humType"], r["tclass"]))
    return mine[0]


def place_of_any(st, dc, race, hum, cls):
    """Where a new character of the race, city and class starts on these rows: (world, x, z), a race of racedefs
    with no rows of its own on its wire race's (database.placeOf)."""
    if race >= FIRST_CUSTOM and not any(r["race"] == race for r in dc) and race in st.racedefs:
        wire, city, _ = st.racedefs[race]
        race, hum = wire, (city if wire == 0 else 0)
    p = place_of(dc, race, hum, cls)
    return p and (p["world"], p["x"], p["z"])


def npc_key(name):
    return (name or "").strip().lower().rstrip("$").strip()


class State:
    def __init__(self, db, ours=()):
        self.db = db
        self.dc = rows(db, "SELECT * FROM defaultcharacter ORDER BY race, humType, tclass")
        self.factions = dict(db.execute("SELECT name, standing FROM factions"))
        self.starts, self.starts_old = {}, {}
        for i, race, cls, hum, f, v in db.execute("SELECT id, race, class, humtype, faction, standing FROM factionstart"):
            self.starts.setdefault(f, []).append((race, cls, hum, v, i))
            if i not in ours:
                self.starts_old.setdefault(f, []).append((race, cls, hum, v, i))
        self.racedefs = {}
        if has_table(db, "racedefs"):
            for i, wire, city, name in db.execute("SELECT id, wire_race, city, name FROM racedefs"):
                self.racedefs[i] = (wire, city, name)

    def start(self, f, combo, old=False):
        return resolve((self.starts_old if old else self.starts).get(f, []), self.factions.get(f, 0), combo)

    def glade(self):
        g = [r for r in self.dc if r["race"] == 0 and r["humType"] == 2 and r["tclass"] == RANGER]
        if not g:
            raise SystemExit("no western human ranger row in defaultcharacter: no glade to start at")
        return g[0]

    def elf_row(self):
        return place_of(self.dc, ELF, 0, RANGER)

    def floors(self):
        return {f: max(FLOOR_MIN, self.start(f, (0, RANGER, 2))) for f in SET}

    def start_races(self):
        """(race, humType) of every ranger start that moves: the eastern humans' and every race with rows but the
        western humans and the elves."""
        out = [(0, 1)]
        for race in sorted({r["race"] for r in self.dc if r["race"] not in (0, ELF)}):
            out.append((race, sorted({r["humType"] for r in self.dc if r["race"] == race})[0]))
        return out

    def faction_combos(self):
        """(name, faction combo) of each ranger the floors are for: the start races, and racedefs races without rows."""
        out = [(self.race_name(r, h), combo_of(r, RANGER, h)) for r, h in self.start_races()]
        have = {r["race"] for r in self.dc}
        for rid, (wire, city, name) in sorted(self.racedefs.items()):
            if rid not in have:
                out.append((name, combo_of(rid, RANGER, 0)))
        return out

    def race_name(self, race, hum=0):
        if race == 0:
            return {1: "Human (east)", 2: "Human (west)"}.get(hum, "Human")
        if race < len(RACE_NAMES):
            return RACE_NAMES[race]
        return self.racedefs.get(race, (0, 0, f"race {race}"))[2]


def ranger_chars(db):
    return rows(db, "SELECT serverid, charName AS charname, race, humType, level FROM characters WHERE tclass = ? "
                    "AND race != ? AND NOT (race = 0 AND humType = 2) ORDER BY race, charname", (RANGER, ELF))


def area_factions(db):
    """The factions of the NPCs a new ranger meets (AREA), each with one of its members (npcfaction as the server
    matches it: a place's row, the smallest first, then a world's, then anywhere's)."""
    nf = {}
    for name, w, x, z, r, f in db.execute("SELECT name, world, x, z, radius, faction FROM npcfaction"):
        nf.setdefault(npc_key(name), []).append((w, x, z, r, f))
    out = {}
    for n, w, x, z in db.execute("SELECT npc_name, world, x_coord, z_coord FROM npcs WHERE world = ?", (WORLD,)):
        if not any(math.hypot(x - cx, z - cz) <= cr for cx, cz, cr in AREA):
            continue
        best = None
        for rw, rx, rz, rr, f in nf.get(npc_key(n), []):
            if rw >= 0 and rw != w or rr > 0 and math.hypot(x - rx, z - rz) > rr:
                continue
            rank = rr if rr > 0 else (1e30 if rw >= 0 else 1e38)
            if best is None or rank < best[0]:
                best = (rank, f)
        if best and best[1]:
            out.setdefault(best[1], n)
    return out


# ---- the plan -------------------------------------------------------------------------------------------------------

def plan(db, epic=True):
    """(updates [(tbl, key, {col: new})], inserts [(tbl, row)], report lines)."""
    st = State(db)
    upd, ins, lines = [], [], []
    g = st.glade()
    place = {"world": g["world"], "x": g["x"], "y": g["y"], "z": g["z"], "facing": g["facing"]}
    west = {r["tclass"]: r for r in st.dc if r["race"] == 0 and r["humType"] == 2}
    lines.append(f"The glade: world {g['world']} ({g['x']:.2f}, {g['y']:.2f}, {g['z']:.2f}) facing {g['facing']:.4f} "
                 f"(defaultcharacter {g['defaultCharacterID']}, the western human ranger's row)")
    e = st.elf_row()
    lines.append(f"Elves keep theirs: {e and (e['defaultCharacterID'], e['world'], round(e['x'], 1), round(e['z'], 1))}")

    # starts
    for race, hum in st.start_races():
        own = [r for r in st.dc if r["race"] == race and r["humType"] == hum]
        mine = [r for r in own if r["tclass"] == RANGER]
        name = st.race_name(race, hum)
        if mine:
            for r in mine:
                if all(same(r[k], v) for k, v in place.items()):
                    lines.append(f"  there already: {name} ranger")
                    continue
                upd.append(("defaultcharacter", (r["defaultCharacterID"],), {k: v for k, v in place.items()}))
                lines.append(f"  {name} ranger: row {r['defaultCharacterID']} moved from world {r['world']} "
                             f"({r['x']:.1f}, {r['z']:.1f})")
            continue
        base = next((r for r in sorted(own, key=lambda r: r["tclass"]) if r["tclass"] in west), None)
        if base is None:
            REFUSE.append(f"{name}: no row of a class the western humans have; no ranger row can be made")
            continue
        new = {k: v for k, v in base.items() if k != "defaultCharacterID"}
        new.update(place)
        new["tclass"] = RANGER
        for s in STATS:
            new[s] = base[s] + west[RANGER][s] - west[base["tclass"]][s]
        ins.append(("defaultcharacter", new))
        lines.append(f"  {name} ranger: a row made (it started on its class {base['tclass']} row, world {base['world']} "
                     f"({base['x']:.1f}, {base['z']:.1f})); stats {[new[s] for s in STATS]}")
        low = min(own, key=lambda r: r["tclass"])
        if low["tclass"] > RANGER:
            # a class with no row of its own starts on the race's LOWEST class's row (database.placeOf): the ranger's
            # would become it (the erudites': no warrior row, the paladin's the lowest), and every such class would
            # start in the glade. A warrior's row where that lowest row stands keeps them where they start now.
            if WARRIOR not in west:
                REFUSE.append(f"{name}: no western human warrior row to make its warrior's from")
                continue
            keep = {k: v for k, v in low.items() if k != "defaultCharacterID"}
            keep["tclass"] = WARRIOR
            for s in STATS:
                keep[s] = low[s] + west[WARRIOR][s] - west[low["tclass"]][s]
            ins.append(("defaultcharacter", keep))
            lines.append(f"  {name} warrior: a row made where its class {low['tclass']} row stands (its classes "
                         f"without a row of their own start there, not in the glade); stats {[keep[s] for s in STATS]}")
    for rid, (wire, city, name) in sorted(st.racedefs.items()):
        if not any(r["race"] == rid for r in st.dc) and wire == ELF:
            lines.append(f"  NOTE: {name} (racedefs {rid}) has no rows and an elf's wire race: its rangers keep the "
                         f"elves' start")

    # factions
    floors = st.floors()
    lines.append("Floors (the western human ranger's start, at least 0): " +
                 ", ".join(f"{f} {v}" for f, v in floors.items()))
    for name, combo in st.faction_combos():
        key = (combo[0], RANGER, -1) if combo[0] != 0 else (0, RANGER, 1)
        raised = []
        for f in SET:
            cur = st.start(f, combo)
            if cur >= floors[f]:
                continue
            exact = [r for r in st.starts.get(f, []) if r[:3] == key]
            if exact:
                upd.append(("factionstart", (exact[0][4],), {"standing": floors[f]}))
            else:
                ins.append(("factionstart", {"race": key[0], "class": key[1], "humtype": key[2], "faction": f,
                                             "standing": floors[f], "note": DETAIL, "source": SOURCE}))
            raised.append(f"{f} {cur} -> {floors[f]}")
        if raised:
            lines.append(f"  {name} ranger: " + "; ".join(raised))

    # existing
    own = {(s, f): v for s, f, v in db.execute(
        "SELECT serverid, faction, standing FROM charfactions WHERE faction IN (%s)" % ",".join("?" * len(SET)), SET)}
    every = ranger_chars(db)
    nchars = 0
    lines.append(f"Rangers of other races than the western humans and the elves: {len(every)}")
    for c in every:
        combo = combo_of(c["race"], RANGER, c["humType"])
        now, raised = [], []
        for f in SET:
            if (c["serverid"], f) in own:
                v, mark = own[(c["serverid"], f)], "*"
            else:
                v, mark = st.start(f, combo, old=True), ""
            now.append(f"{f} {v}{mark}")
            if v >= floors[f]:
                continue
            if mark:
                upd.append(("charfactions", (c["serverid"], f), {"standing": floors[f]}))
            else:
                ins.append(("charfactions", {"serverid": c["serverid"], "faction": f, "standing": floors[f]}))
            raised.append(f)
        nchars += bool(raised)
        lines.append(f"  {c['charname']} ({st.race_name(c['race'], c['humType'])}, level {c['level']}): " +
                     ", ".join(now) + (f"  -> raised: {', '.join(raised)}" if raised else ""))
    lines.append(f"  (* its own charfactions row; else its start)  {nchars} characters raised")

    # quests
    line = rows(db, "SELECT questid, name, minlevel, classes, races FROM quests WHERE source LIKE ? AND giver = "
                    "'Sigmor Fallbourne' AND replace(classes, ' ', '') = '1' ORDER BY minlevel, questid", (LINE_SRC + "%",))
    if len(line) != 10:
        REFUSE.append(f"the Surefall ranger line is {len(line)} quests here, not 10: {[q['questid'] for q in line]}")
    want = [0] + list(range(2, FROGLOK + 1)) + sorted(st.racedefs)
    for q in line:
        have = ints(q["races"])
        if not have:
            lines.append(f"  there already: quest {q['questid']} (every race)")
            continue
        new = sorted(set(have) | set(want))
        if new == sorted(set(have)):
            lines.append(f"  there already: quest {q['questid']}")
            continue
        upd.append(("quests", (q["questid"],), {"races": races_text(new)}))
        lines.append(f"  quest {q['questid']} {q['name']!r} (level {q['minlevel']}): races {q['races']!r} -> "
                     f"{races_text(new)!r}")
    if epic:
        for src, add in ((EPIC_START, EPIC_START_ADD), (EPIC_EPS, EPIC_EPS_ADD)):
            eq = rows(db, "SELECT questid, name, classes, races FROM quests WHERE source LIKE ? ORDER BY questid",
                      (src + "%",))
            eq = [q for q in eq if RANGER in ints(q["classes"])]
            if not eq:
                REFUSE.append(f"no ranger quest of {src} here")
            for q in eq:
                have = ints(q["races"])
                if not have:
                    continue
                new = sorted(set(have) | set(add))
                if new == sorted(set(have)):
                    lines.append(f"  there already: epic quest {q['questid']}")
                    continue
                upd.append(("quests", (q["questid"],), {"races": races_text(new)}))
                lines.append(f"  epic quest {q['questid']} {q['name']!r} (classes {q['classes']}): + "
                             f"{sorted(set(new) - set(have))}")

    # dialog
    have = {r[0] for r in db.execute("SELECT line FROM npcdialog WHERE town = 'Surefall Glade' AND class = ?", (RANGER,))}
    for role, text, w in LINES:
        if text in have:
            lines.append(f"  there already: [{role or 'anyone'}] {text[:40]}")
            continue
        ins.append(("npcdialog", {"town": "Surefall Glade", "role": role, "name": "", "mood": "", "race": -1, "sex": "",
                                  "line": text, "weight": w, "source": SOURCE, "speaker_race": -1, "away": 0,
                                  "class": RANGER, "not_race": 0}))
        lines.append(f"  dialog [{role or 'anyone'}] {text}")
    return upd, ins, lines


def check(db, epic=True, before=None):
    """Problems as the database stands: a new ranger (but an elf) not starting at the glade, below a floor or KOS to a
    faction there; an elf's start moved; an existing ranger below a floor or KOS there; a quest of the line or the epic
    refusing one of those races."""
    probs = []
    st = State(db)
    g = st.glade()
    at = (g["world"], g["x"], g["z"])
    area = area_factions(db)
    floors = st.floors()
    for race, hum in st.start_races() + [(FROGLOK, 1), (0, 0)]:
        p = place_of(st.dc, race, hum, RANGER)
        if p is None or (p["world"], p["x"], p["z"]) != at:
            probs.append(f"{st.race_name(race, hum)} ranger starts at {p and (p['world'], p['x'], p['z'])}")
    for rid, (wire, city, name) in st.racedefs.items():
        if not any(r["race"] == rid for r in st.dc) and wire != ELF:
            p = place_of(st.dc, wire, city if wire == 0 else 0, RANGER)
            if p is None or (p["world"], p["x"], p["z"]) != at:
                probs.append(f"{name} ranger (no rows: its wire race's) starts at {p and (p['world'], p['x'], p['z'])}")
    if before is not None:
        races = sorted({r["race"] for r in before} | set(st.racedefs)) + [FROGLOK]
        for race in races:
            for hum in (0, 1, 2):
                for cls in CLASSES:
                    if cls == RANGER:
                        continue
                    a, b = place_of_any(st, before, race, hum, cls), place_of_any(st, st.dc, race, hum, cls)
                    if a != b:
                        probs.append(f"{st.race_name(race, hum)} class {cls} (city {hum}) would start at {b}, not {a}")
    e = place_of(st.dc, ELF, 0, RANGER)
    if e is not None and (e["world"], e["x"], e["z"]) == at:
        probs.append("the elf ranger starts at the glade")
    for name, combo in st.faction_combos():
        for f in SET:
            if st.start(f, combo) < floors[f]:
                probs.append(f"{name} ranger starts at {st.start(f, combo)} with {f}")
        for f in area:
            if st.start(f, combo) < KOS_BELOW:
                probs.append(f"{name} ranger is KOS to {f} ({area[f]})")
    own = {(s, f): v for s, f, v in db.execute("SELECT serverid, faction, standing FROM charfactions")}
    for c in ranger_chars(db):
        combo = combo_of(c["race"], RANGER, c["humType"])
        for f in set(SET) | set(area):
            v = own.get((c["serverid"], f), st.start(f, combo))
            if f in floors and v < floors[f] or v < KOS_BELOW:
                probs.append(f"{c['charname']} stands at {v} with {f}")
    want = [0] + list(range(2, FROGLOK + 1)) + sorted(st.racedefs)
    for qid, races in db.execute("SELECT questid, races FROM quests WHERE source LIKE ? AND giver = 'Sigmor Fallbourne'",
                                 (LINE_SRC + "%",)):
        missing = set(want) - set(ints(races)) if ints(races) else set()
        if missing:
            probs.append(f"quest {qid} refuses races {sorted(missing)}")
    if epic:
        for qid, races, classes, src in db.execute("SELECT questid, races, classes, source FROM quests WHERE source "
                                                   "LIKE ? OR source LIKE ?", (EPIC_START + "%", EPIC_EPS + "%")):
            if RANGER in ints(classes) and ints(races):
                missing = set(EPIC_START_ADD if src.startswith(EPIC_START) else EPIC_EPS_ADD) - set(ints(races))
                if missing:
                    probs.append(f"epic quest {qid} refuses races {sorted(missing)}")
    return probs, area


# ---- the petfix machinery ---------------------------------------------------------------------------------------------

def where(tbl):
    return " AND ".join(f"{c} = ?" for c in KEYS[tbl])


def keyof(tbl, row):
    return tuple(row[c] for c in KEYS[tbl])


def rowkey(tbl, key):
    return dumps(dict(zip(KEYS[tbl], key)))


def key_of_rowkey(tbl, rk):
    d = json.loads(rk)
    return tuple(d[c] for c in KEYS[tbl])


def full_row(db, tbl, key):
    cur = db.execute(f"SELECT * FROM {tbl} WHERE {where(tbl)}", key)
    cols = [d[0] for d in cur.description]
    r = cur.fetchone()
    return None if r is None else dict(zip(cols, r))


def audit(db, tbl, key, before, after):
    action = "insert" if before is None else ("delete" if after is None else "update")
    db.execute("""INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
                  VALUES (?, 1, 'dab', 'tool', '', ?, ?, ?, ?, ?, ?)""",
               (time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), action, tbl, dumps(dict(zip(KEYS[tbl], key))),
                None if before is None else json.dumps(before, sort_keys=True, separators=(",", ":")),
                None if after is None else json.dumps(after, sort_keys=True, separators=(",", ":")),
                DETAIL))


def log(db, tbl, key, col, before, after):
    return db.execute(f"INSERT INTO {LOG} (petition, tbl, rowkey, col, before, after) VALUES (?, ?, ?, ?, ?, ?)",
                      (PET, tbl, rowkey(tbl, key), col, before, after)).lastrowid


LOG_DDL = f"""CREATE TABLE IF NOT EXISTS {LOG} (
        seq      INTEGER PRIMARY KEY AUTOINCREMENT,
        petition INTEGER NOT NULL,
        tbl      TEXT NOT NULL,
        rowkey   INTEGER NOT NULL,
        col      TEXT NOT NULL,
        before   TEXT,
        after    TEXT,
        at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
        undone   TEXT NOT NULL DEFAULT ''
    )"""


def undo(db, applied):
    if not os.path.exists(applied):
        sys.exit("nothing applied: no " + applied)
    with open(applied) as fh:
        seqs = json.load(fh)
    n = kept = 0
    with db:
        for seq in sorted(seqs, reverse=True):
            row = db.execute(f"SELECT tbl, rowkey, col, before, after, undone FROM {LOG} WHERE seq = ?", (seq,)).fetchone()
            if row is None or row[5]:
                continue
            tbl, rk, col, before, after, _ = row
            key = key_of_rowkey(tbl, rk)
            b0 = full_row(db, tbl, key)
            if col == "+row":
                if b0 is None or json.dumps(b0, sort_keys=True) != after:
                    print(f"  kept: {tbl} {key} (changed since)")
                    kept += 1
                    continue
                db.execute(f"DELETE FROM {tbl} WHERE {where(tbl)}", key)
                audit(db, tbl, key, b0, None)
            else:
                if b0 is None or str(b0[col]) != after:
                    print(f"  kept: {tbl} {key} {col} (changed since: {b0 and b0[col]!r})")
                    kept += 1
                    continue
                kind = db.execute(f"SELECT typeof({col}) FROM {tbl} WHERE {where(tbl)}", key).fetchone()[0]
                val = before
                if before is not None and kind == "integer":
                    val = int(before)
                elif before is not None and kind == "real":
                    val = float(before)
                db.execute(f"UPDATE {tbl} SET {col} = ? WHERE {where(tbl)}", (val, *key))
                audit(db, tbl, key, b0, full_row(db, tbl, key))
            db.execute(f"UPDATE {LOG} SET undone = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE seq = ?", (seq,))
            n += 1
    os.rename(applied, applied + ".undone-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()))
    print(f"undone: {n}" + (f"; {kept} kept as they are now" if kept else ""))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default="eqoa.db")
    ap.add_argument("--shard", choices=["heirloom", "live"], required=True)
    ap.add_argument("--applied", help="the applied file (default: beside this script, by shard)")
    ap.add_argument("--no-epic", action="store_true", help="leave the level 60 epic's races as they are")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true")
    g.add_argument("--undo", action="store_true")
    a = ap.parse_args(argv)
    REFUSE.clear()
    applied = a.applied or os.path.join(HERE, f"ranger_start_20261127_{a.shard}_applied.json")
    if not os.path.exists(a.db):
        sys.exit(f"no database at {a.db}")
    db = sqlite3.connect(a.db, timeout=10)
    try:
        if a.undo:
            undo(db, applied)
            return 0
        if os.path.exists(applied):
            sys.exit(f"this run is applied already ({applied}): --undo first")
        epic = not a.no_epic
        before = State(db).dc
        upd, ins, lines = plan(db, epic)
        for ln in lines:
            print(ln)
        if REFUSE:
            for r in REFUSE:
                print("refused:", r)
            sys.exit("not as expected here: nothing done")
        changes = []
        for tbl, key, cols in upd:
            r = full_row(db, tbl, key)
            for c, v in cols.items():
                if not same(r[c], v):
                    changes.append((tbl, key, c, r[c], v))
        print(f"{len(changes)} changes, {len(ins)} rows added ({a.shard})")
        if not a.apply:
            mem = sqlite3.connect(":memory:")
            db.backup(mem)
            write(mem, changes, ins)
            probs, area = check(mem, epic, before)
            mem.close()
            print(f"== check, as it would be (the area's factions: {', '.join(area)})")
            for p in probs:
                print("PROBLEM:", p)
            print("OK: every ranger but the elves' at the glade, at the floors, KOS to no one there, its quests open"
                  if not probs else "")
            print("report only: --apply writes it")
            return 1 if probs else 0
        with db:
            seqs = write(db, changes, ins)
            probs, _ = check(db, epic, before)
            if probs:
                for p in probs:
                    print("PROBLEM:", p)
                raise SystemExit("not written: the check after the write failed (rolled back)")
        with open(applied, "w") as fh:
            json.dump(seqs, fh)
        print(f"applied: {len(seqs)} log rows kept in {applied}; restart the server for it (a keep restart's before.sh)")
        return 0
    finally:
        db.close()


def write(db, changes, ins):
    db.execute(LOG_DDL)
    seqs, done = [], {}
    for tbl, key, c, b, n in changes:
        if (tbl, key) not in done:
            done[(tbl, key)] = full_row(db, tbl, key)
        if db.execute(f"UPDATE {tbl} SET {c} = ? WHERE {where(tbl)} AND {c} IS ?", (n, *key, b)).rowcount != 1:
            raise RuntimeError(f"{tbl} {key} {c}: changed under us")
        seqs.append(log(db, tbl, key, c, None if b is None else str(b), str(n)))
    for (tbl, key), before in done.items():
        audit(db, tbl, key, before, full_row(db, tbl, key))
    for tbl, row in ins:
        cols = list(row)
        cur = db.execute(f"INSERT INTO {tbl} ({', '.join(cols)}) VALUES ({', '.join('?' * len(cols))})",
                         [row[c] for c in cols])
        key = keyof(tbl, row) if all(c in row for c in KEYS[tbl]) else (cur.lastrowid,)
        after = full_row(db, tbl, key)
        seqs.append(log(db, tbl, key, "+row", None, json.dumps(after, sort_keys=True)))
        audit(db, tbl, key, None, after)
    return seqs


if __name__ == "__main__":
    sys.exit(main())
