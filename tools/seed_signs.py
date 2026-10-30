#!/usr/bin/env python3
"""What the signs say (the user, 2026-10-30: "we also need to add signpost
dialog, in cities usually it is the title of the building, on the roads
outside town, they usually give directions to nearest towns in 4 directions
or so").

Every sign post and announcement board stands on one body (SIGN_BODY); what
one says when talked to is its row of table signtext (database/signtext.go,
handlers/signs.go), a GM's to change in the panel. No record of what EQOA's
signs said is known: the Frontiers manual says only that "the signpost will
give you helpful directions", Prima's guide that "a signpost guides you to the
Marketplace" in Qeynos and of a "withered signpost" on the Trade Road; no
capture, harvest, Return Home export, EQOA-Data or FrontiersForge file holds a
sign's words, and CLIENT has none (the server sent them). So the words are
made, each from what is known of where the sign stands:

- a board ("<City> Announcements"): a welcome naming its city (BOARD_WELCOME,
  ours); the server says the news row's words after it (npcid 0, kind news,
  written empty: a GM's, one edit for every board);
- a sign whose own name names its place ("signpost: Midroad"): that place;
- a sign at a building of a city Prima's Frontiers guide maps: the building's
  title as the guide's map legend gives it, when one of the people the legend
  lists there, or the guide's walkthrough cites by its number ("Forgun E`Phet
  9"), stands within BUILDING_REACH of the sign (a place the legend lists by
  its keeper alone is the keeper's shop, by trade: "Smithy"; a merchant's by
  the keeper's name; a titled place within TITLE_SLACK farther is named
  first). Each person is its nearest mapped city's;
- a sign elsewhere in such a city (within CITY_REACH of a mapped place's
  people and inside the outline of them, HULL_MARGIN out, but not beyond the
  city's gate): the city's titled places nearest it in up to four directions;
- any other sign is a road sign: the nearest town each way (north, east,
  south, west: the quarter of the compass the town lies in) within
  TOWN_REACH, from COACH_TOWNS (entity/coachtowns.go's towns, cities and
  outposts, under the names a sign gives them, TOWN_SIGN_NAMES), else the
  region that lies that way: the next zone of CLIENT's grid (ZONES_TUNARIA, the
  WORDS.TXT word of each 2,000-unit cell) that is land, named from the
  operator's own WORDS.TXT (--words). x runs east, z south.

Sources, none kept in this repository: --prima is the guide's text (the PDF
tools/fetch_tradeskill_sources.py fetches, made into layout text with
pdftotext -layout); --words the disc's DATA/WORDS.TXT. Without --prima no
sign is given a building (each is a road sign); without --words a quarter with
no town in reach is left out.

The report lists every sign with what it would say and how that was made.
--save-plan writes that plan as JSON, which --plan reads in place of the
sources (to apply on a machine without them: each row is checked against the
npcs row it names, its name and place, and left out when it no longer
matches). --apply writes a row for each sign that has none (a GM's row is
never touched), source "ours", logged in signtext_seed; run again it adds only
the signs without one. --undo deletes each row still as written, leaves one a
GM changed (said), and drops the log (and the table, when --apply made it and
nothing is left in it). Both copy the database first
(<db>.bak-<when>-pre-signs). A server that reads signtext (handlers/signs.go)
reads it at most 30 s old, so no restart; an older one ignores it. --verbose
lists the guide's places and who stands at each.

    tools/seed_signs.py [--db eqoa.db] [--prima prima-frontiers-layout.txt] [--words WORDS.TXT]
        [--save-plan plan.json | --plan plan.json] [--apply | --undo] [--verbose]
"""
import argparse
import collections
import datetime
import json
import math
import os
import re
import shutil
import sqlite3
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_DB = os.path.join(HERE, "..", "eqoa.db")
SOURCE = "ours"

# The body of every sign post and town board (CHAR.ESF's 0x6A82352C):
# handlers/signs.go's SignBody.
SIGN_BODY = 0x6A82352C

# How near a building's people a sign must stand to be its sign, and how near
# a mapped place's to be in its city (in three dimensions: Neriak's streets
# lie over one another); how far a city sign looks for its places; how far a
# road sign looks for a town.
BUILDING_REACH = 60.0
CITY_REACH = 150.0
CITY_SPAN = 700.0
TOWN_REACH = 9000.0
# How much farther a titled building may stand than a shop listed by its
# keeper alone and still be the one named.
TITLE_SLACK = 15.0
# How far outside the outline of a city's mapped people a sign may stand and
# still be in the city (a sign outside its gate is a road sign).
HULL_MARGIN = 60.0
# How near the city's coach town a person of its map must stand to be taken
# for that person (a name may be a creature's elsewhere).
CITY_RADIUS = 1500.0

# database/signtext.go's signTextSchema, word for word
SCHEMA = """CREATE TABLE IF NOT EXISTS signtext (
	npcid  INTEGER PRIMARY KEY,
	kind   TEXT NOT NULL DEFAULT 'gm',
	text   TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'gm',
	note   TEXT NOT NULL DEFAULT ''
)"""
LOG_SCHEMA = """CREATE TABLE IF NOT EXISTS signtext_seed (
    npcid INTEGER PRIMARY KEY,  -- signtext.npcid written; MADE_TABLE: --apply made signtext
    row   TEXT NOT NULL         -- the row as written, JSON
)"""
MADE_TABLE = -1
COLS = ("kind", "text", "note")

# The news every board says after its own words: one row, npcid 0.
NEWS_ID = 0
NEWS_NOTE = "the news every announcement board says after its own words; empty: none (a GM's)"

# entity/coachtowns.go's coachTowns, word for word (TestSeedSignsKeepsTheCoachTowns).
COACH_TOWNS = [
    ("Halas", 12963, 4689),
    ("Klik'Anon", 23460, 6488),
    ("Neriak", 24929, 9438),
    ("Zentar's Keep", 5496, 6457),
    ("Fayspire", 19473, 7038),
    ("Tethelin", 18421, 7488),
    ("Bobble-by-Water", 24634, 11861),
    ("Castle Lightwolf", 9281, 6504),
    ("Moradhim", 15352, 9262),
    ("Fort Seriak", 21776, 8390),
    ("Rivervale", 18339, 11350),
    ("Wyndhaven", 4715, 13211),
    ("Murnf", 8929, 10516),
    ("Mt. Hatespike", 13764, 11395),
    ("Freeport", 25270, 15710),
    ("Surefall Glade", 9060, 12978),
    ("Qeynos", 4829, 17154),
    ("Honjour", 9804, 14991),
    ("Darvar Manor", 13606, 14766),
    ("Highpass Hold", 16773, 15363),
    ("Muniel's Tea Garden", 23374, 19638),
    ("Highbourne", 4854, 21477),
    ("Forkwatch", 12538, 18774),
    ("Dark Solace", 12665, 21109),
    ("Hazinak (Good)", 24885, 27594),
    ("Hazinak (Evil)", 25130, 27159),
    ("Blackwater", 15147, 22660),
    ("Oasis", 21120, 25167),
    ("Gerntar Mines", 13038, 27506),
    ("Grobb", 25257, 31226),
    ("Oggok", 10980, 27211),
    ("Kerplunk Outpost", 14815, 29246),
]
# The towns are Tunaria's.
TOWN_WORLD = 0
# What a sign calls a town the coach menus name otherwise: the disc's
# spellings, and one Hazinak for its two halves.
TOWN_SIGN_NAMES = {"Klik'Anon": "Klick'Anon", "Fayspire": "Fayspires", "Hazinak (Good)": "Hazinak",
                   "Hazinak (Evil)": "Hazinak"}

# The cities Prima's guide maps, as its maps' titles name them, and the coach
# town each is (its people are looked for round it).
PRIMA_CITIES = {
    "Halas": "Halas", "Neriak": "Neriak", "Moradhim": "Moradhim", "Tethelin": "Tethelin",
    "Fayspires": "Fayspire", "Highbourne": "Highbourne", "Klick’Anon": "Klik'Anon", "Rivervale": "Rivervale",
    "Freeport": "Freeport", "Qeynos": "Qeynos", "Surefall Glade": "Surefall Glade", "Oggok": "Oggok",
    "Grobb": "Grobb",
}

# A board's welcome, by the city its name gives (ours).
BOARD_WELCOME = {
    "Freeport": "Welcome to Freeport, city of opportunity. Keep the peace within these walls: the Militia is watching.",
    "Neriak": "Welcome to Neriak. Serve Innoruuk, and the city will serve you.",
    "Moradhim": "Welcome to Moradhim, the dwarves' mining colony in the hills of the Serpent's Spine.",
    "Halas": "Welcome to Halas. Warm yourself at our fires: the cold of Everfrost forgives no one.",
    "Tethelin": "Welcome to Tethelin. Walk softly among the trees.",
    "Fayspires": "Welcome to the Fayspires, the new home of the elves who fled Takish'Hiz.",
    "Surefall Glade": "Welcome to Surefall Glade. Respect the forest and the creatures in it.",
}
# What a board's name calls its city, where that is not the city's own name
# (Surefall's board says "Surefall Anouncements").
BOARD_CITIES = {"Surefall": "Surefall Glade"}

# A shop the guide lists by its keeper alone is named by the keeper's trade,
# the first word of the keeper's name (a longer one first); None: by the
# keeper's own name (a merchant, a spiritmaster: a person, not a shop).
TRADES = [
    ("Pole Merchant", "Fishing Supplies"), ("Smithy", "Smithy"), ("Blacksmith", "Smithy"),
    ("Weaponsmith", "Weaponsmith"), ("Shieldsmith", "Shieldsmith"), ("Forgemaster", "Forge"), ("Armorer", "Armorer"),
    ("Bowyer", "Bowyer"), ("Tailor", "Tailor"), ("Seamster", "Tailor"), ("Seamstress", "Tailor"),
    ("Jeweler", "Jeweler"), ("Grocer", "Grocer"), ("Chemist", "Chemist"), ("Banker", "Bank"),
    ("Coachman", "Stables"), ("Coachwoman", "Stables"), ("Harbormaster", "Docks"), ("Dockmaster", "Docks"),
    ("Tacklemaster", "Fishing Supplies"), ("Fisherman", "Fishing Supplies"), ("Spiritmaster", None),
    ("Waysender", None), ("Merchant", None),
]

# What a sign at a place of the guide's maps says where the guide's legend
# names the keeper and the building has a name of its own, by city and the
# legend's number (the user's corrections).
PLACE_TITLES = {
    ("Neriak", "26"): "Blind Fish Tavern",  # the user, 2026-10-30: "the madam du klot signpost should be Blind Fish Tavern"
}

# The four quarters a sign points to, their bearings (degrees clockwise from
# north) and a step of one zone that way.
QUARTERS = [("North", 0.0, (0, -1)), ("East", 90.0, (1, 0)), ("South", 180.0, (0, 1)), ("West", 270.0, (-1, 0))]

# CLIENT's zone grid of Tunaria (ZoneName, 0x00CC4608; the decomp's
# vientity.cpp sZones0): the WORDS.TXT word naming each 2,000-unit cell, row
# z / 2000 - 1 and column x / 2000 - 1; 641 is none.
ZONE_CELL = 2000.0
ZONES_TUNARIA = [
    [641, 641, 641, 641, 641, 641, 641, 641, 641, 1152, 1153, 1176, 1051],
    [1046, 1057, 1058, 1074, 1075, 364, 1097, 1123, 1124, 1151, 1154, 1175, 1042],
    [1047, 1056, 1059, 1073, 1076, 1096, 1098, 1122, 1125, 1150, 361, 1174, 1045],
    [1047, 1055, 1060, 1072, 1077, 1095, 608, 1121, 1126, 1149, 1155, 1173, 1047],
    [1047, 1054, 1061, 1071, 1078, 1094, 1099, 1120, 1127, 1148, 1156, 1172, 1047],
    [1047, 1053, 1062, 602, 1079, 1093, 1100, 1119, 1128, 1147, 1157, 1171, 1047],
    [1047, 1052, 1063, 1070, 1080, 1092, 1101, 1118, 1129, 1146, 1158, 358, 1047],
    [1047, 357, 1064, 1069, 1081, 1091, 1102, 1117, 1130, 1145, 1159, 1170, 1047],
    [1047, 1050, 1065, 1068, 1082, 1090, 1103, 1116, 1131, 1144, 1160, 1047, 1047],
    [1048, 366, 1066, 1067, 1083, 1089, 1104, 1115, 1132, 1143, 1161, 1047, 1047],
    [1049, 1049, 641, 641, 641, 1088, 1105, 1114, 1133, 1142, 1162, 1047, 1047],
    [641, 641, 641, 641, 641, 1087, 1106, 1113, 1134, 1141, 1163, 1169, 1047],
    [641, 641, 641, 641, 641, 1086, 1107, 1112, 1135, 1140, 1164, 1168, 1047],
    [641, 641, 641, 641, 641, 1085, 1108, 1111, 1136, 1139, 1165, 1167, 1047],
    [641, 641, 641, 641, 641, 1084, 1109, 1110, 1137, 1138, 1166, 363, 1047],
    [641, 641, 641, 641, 641, 1047, 1047, 1047, 1047, 1047, 1047, 1047, 1047],
]
ZONE_NONE = 641
# How many zones a road sign looks along a quarter for land.
ZONE_STEPS = 3
# A zone that is no land a sign points to: the sea, the world's edge.
NOT_LAND = re.compile(r"^(Open Sea|Empty)$|Boundary|Ocean|Dread Sea", re.I)

BOARD_NAME = re.compile(r"^(?P<city>.+?)\s+Ann?ouncements?\s*$", re.I)
NAMED_SIGN = re.compile(r"^sign\s*post\s*:\s*(?P<place>\S.*?)\s*$", re.I)


# ---- the sources -----------------------------------------------------------------

def read_words(path):
    """WORDS.TXT's words by their number (the first line is 1)."""
    with open(path, "rb") as f:
        text = f.read().decode("utf-8-sig")
    return [""] + text.replace("\r\n", "\n").split("\n")


def zone_word(x, z):
    """The word of the Tunaria zone at (x, z), or ZONE_NONE."""
    row, col = int(math.floor(z / ZONE_CELL)) - 1, int(math.floor(x / ZONE_CELL)) - 1
    if 0 <= row < len(ZONES_TUNARIA) and 0 <= col < len(ZONES_TUNARIA[row]):
        return ZONES_TUNARIA[row][col]
    return ZONE_NONE


LEGEND_ENTRY = re.compile(r"^(\d{1,2}[a-f]?)\s+(\S.*)$")
LEGEND_END = re.compile(r"primagames\.com|Spell Merchants|Merchant Name\s+Description")


def segments(line):
    """A layout line's runs of text, at least two spaces apart: (column, text)."""
    return [(m.start(), m.group(0)) for m in re.finditer(r"\S+(?: \S+)*", line)]


def map_legend(lines, city):
    """The numbered places of the guide's map of a city: [{no, title, people}].

    The legend is laid out in columns (pdftotext -layout keeps them): a place
    is a number and its title, and the people there follow it in its column,
    indented, down the page and over a page break. Map marks (a letter or two)
    and page numbers are skipped; a place the guide lists twice word for word
    (Neriak's 4 and 5) is kept once."""
    start = None
    head = re.compile(r"^\s*Map of " + re.escape(city) + r"\S?\s*$")
    for i, line in enumerate(lines):
        if head.match(line):
            start = i
            break
    if start is None:
        return []
    block, seen_entry = [], False
    for line in lines[start + 1:]:
        if LEGEND_END.search(line) and seen_entry:
            break
        segs = segments(line)
        seen_entry = seen_entry or any(LEGEND_ENTRY.match(t) for _, t in segs)
        block.append(segs)
    starts = sorted({c for segs in block for c, t in segs if LEGEND_ENTRY.match(t)})
    cols = []
    for c in starts:
        if cols and c - cols[-1][-1] <= 3:
            cols[-1].append(c)
        else:
            cols.append([c])
    cols = [min(c) for c in cols]

    def column(c):
        k = None
        for i, s in enumerate(cols):
            if s - 1 <= c:
                k = i
        return k

    entries, current = [], {}
    for segs in block:
        for c, t in segs:
            if (len(t) <= 2 and not t[:1].isdigit()) or re.fullmatch(r"\d{1,3}", t):
                continue
            k = column(c)
            if k is None:
                continue
            m = LEGEND_ENTRY.match(t)
            if m:
                e = {"no": m.group(1), "title": m.group(2).strip(), "people": []}
                entries.append(e)
                current[k] = e
            elif k in current:
                current[k]["people"].append(t.strip())
    out = []
    for e in entries:
        if out and any(o["title"] == e["title"] and o["people"] == e["people"] for o in out):
            continue
        out.append(e)
    return out


def norm(name):
    """A name as the guide and the database are compared: no parentheses,
    every apostrophe the same, lower case."""
    name = re.sub(r"\s*\([^)]*\)", "", name)
    return re.sub(r"[’'`]", "`", name).strip().lower()


CITED = re.compile(r"((?:[A-Z][\w`-]*\s+){0,3}[A-Z][\w`-]*)\s+(\d{1,2}[a-f]?)\s*[.,;)]")


def citations(lines):
    """Every person the guide cites with a map number, as (name, number): its
    walkthrough's "Forgun E`Phet 9 ," and "Speak to Spiritmaster Xoln 32 .",
    and a class's own map listing "5 Master Torlin". A name is every run of
    up to four capitalised words before the number, each tail of it a
    candidate (normalised: norm)."""
    out = []
    for line in lines:
        flat = re.sub(r"[’'`]", "`", line)
        for m in CITED.finditer(flat):
            words = m.group(1).split()
            for k in range(len(words)):
                out.append((norm(" ".join(words[k:])), m.group(2)))
        for _, t in segments(flat):
            m = LEGEND_ENTRY.match(t)
            if m:
                out.append((norm(m.group(2)), m.group(1)))
    return out


def mentions(cited, names, numbers):
    """{name: number} for the names given (normalised) the guide cites with
    a number its city's legend has; a name cited by two is left out."""
    found = collections.defaultdict(set)
    for name, no in cited:
        if name in names and no in numbers:
            found[name].add(no)
    return {n: next(iter(s)) for n, s in found.items() if len(s) == 1}


# ---- the world -------------------------------------------------------------------

def load_npcs(db):
    rows = db.execute("SELECT id, npc_name, world, x_coord, y_coord, z_coord, modelid FROM npcs").fetchall()
    out = []
    for i, name, world, x, y, z, model in rows:
        out.append({"id": i, "name": (name or "").strip(), "world": world or 0, "x": x or 0.0, "y": y or 0.0,
                    "z": z or 0.0, "body": (model or 0) & 0xFFFFFFFF})
    return out


def dist3(a, b):
    return math.sqrt((a["x"] - b["x"]) ** 2 + (a["y"] - b["y"]) ** 2 + (a["z"] - b["z"]) ** 2)


def bearing(fx, fz, tx, tz):
    """Degrees clockwise from north (x east, z south)."""
    return math.degrees(math.atan2(tx - fx, -(tz - fz))) % 360.0


def quarter_of(deg):
    for name, centre, _ in QUARTERS:
        d = abs((deg - centre + 180.0) % 360.0 - 180.0)
        if d < 45.0 or (d == 45.0 and name in ("North", "South")):
            return name
    return "North"


def city_places(lines, npcs):
    """Every place of the guide's city maps whose people stand in the
    database near the city: [{city, no, title, keeper, people: [npc]}]."""
    towns = {t[0]: (t[1], t[2]) for t in COACH_TOWNS}
    cited = citations(lines)
    # each person is its nearest mapped city's, within CITY_RADIUS (Tethelin
    # and the Fayspires stand 1,200 apart: each's own map numbers its people)
    home = {}
    for n in npcs:
        if n["world"] != TOWN_WORLD or not n["name"]:
            continue
        best = min(PRIMA_CITIES, key=lambda c: math.hypot(n["x"] - towns[PRIMA_CITIES[c]][0],
                                                            n["z"] - towns[PRIMA_CITIES[c]][1]))
        bx, bz = towns[PRIMA_CITIES[best]]
        if math.hypot(n["x"] - bx, n["z"] - bz) <= CITY_RADIUS:
            home[n["id"]] = best
    out = []
    for city, town in PRIMA_CITIES.items():
        legend = map_legend(lines, city)
        if not legend or town not in towns:
            continue
        near = collections.defaultdict(list)
        for n in npcs:
            if home.get(n["id"]) == city:
                near[norm(n["name"])].append(n)
        numbers = collections.Counter(e["no"] for e in legend)
        unique = {no for no, k in numbers.items() if k == 1}
        cites = mentions(cited, {n for n in near if len(n) > 3}, unique)
        by_no = {e["no"]: e for e in legend}
        for e in legend:
            keeper = trade_of(e["title"]) is not None or norm(e["title"]) in near
            names = [e["title"]] + e["people"] if keeper else list(e["people"])
            names += [n for n, no in cites.items() if no == e["no"] and by_no.get(no) is e]
            people, seen = [], set()
            for name in names:
                # a map mark run into a name ("M Smithy Melliom")
                key = norm(name)
                if key not in near and re.match(r"^[a-z] \S", key):
                    key = key[2:]
                for n in near.get(key, []):
                    if n["id"] not in seen:
                        seen.add(n["id"])
                        people.append(n)
            out.append({"city": city, "no": e["no"], "title": e["title"], "keeper": keeper, "people": people})
    return out


def trade_of(name):
    """The trade a keeper's name begins with: (its word, the shop's name or
    None), or None."""
    for word, shop in TRADES:
        if name == word or name.startswith(word + " "):
            return word, shop
    return None


def place_title(p):
    """What a sign at a place says: the user's word for it (PLACE_TITLES), its
    title, or a keeper's shop by trade (the keeper's own name where the trade
    names no shop, as the database spells it)."""
    if (p["city"], p["no"]) in PLACE_TITLES:
        return PLACE_TITLES[(p["city"], p["no"])]
    if not p["keeper"]:
        return p["title"].replace("’", "'")
    t = trade_of(p["title"])
    if t and t[1]:
        return t[1]
    keeper = [n for n in p["people"] if norm(n["name"]) == norm(p["title"])]
    return keeper[0]["name"] if keeper else p["title"].replace("’", "'")


def hull(points):
    """The convex hull of (x, z) points, counter-clockwise (Andrew's chain)."""
    pts = sorted(set(points))
    if len(pts) <= 2:
        return pts

    def cross(o, a, b):
        return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0])

    lower, upper = [], []
    for p in pts:
        while len(lower) >= 2 and cross(lower[-2], lower[-1], p) <= 0:
            lower.pop()
        lower.append(p)
    for p in reversed(pts):
        while len(upper) >= 2 and cross(upper[-2], upper[-1], p) <= 0:
            upper.pop()
        upper.append(p)
    return lower[:-1] + upper[:-1]


def inside(poly, x, z, margin):
    """Whether (x, z) is in the convex polygon or within margin of it."""
    if len(poly) < 3:
        return any(math.hypot(px - x, pz - z) <= margin for px, pz in poly)
    n, inner = len(poly), True
    for i in range(n):
        (ax, az), (bx, bz) = poly[i], poly[(i + 1) % n]
        if (bx - ax) * (z - az) - (bz - az) * (x - ax) < 0:
            inner = False
            break
    if inner:
        return True
    for i in range(n):
        (ax, az), (bx, bz) = poly[i], poly[(i + 1) % n]
        dx, dz = bx - ax, bz - az
        t = max(0.0, min(1.0, ((x - ax) * dx + (z - az) * dz) / (dx * dx + dz * dz or 1.0)))
        if math.hypot(ax + dx * t - x, az + dz * t - z) <= margin:
            return True
    return False


# ---- what each sign says ---------------------------------------------------------

def directions(sign, targets, reach):
    """The nearest target in each quarter within reach: {quarter: (name, distance)}."""
    best = {}
    for name, x, z in targets:
        d = math.hypot(x - sign["x"], z - sign["z"])
        if d < 1.0 or d > reach:
            continue
        q = quarter_of(bearing(sign["x"], sign["z"], x, z))
        if q not in best or d < best[q][1]:
            best[q] = (name, d)
    return best


def region_that_way(sign, step, words):
    """The first zone of land along a quarter, other than the sign's own."""
    own = zone_word(sign["x"], sign["z"])
    for k in range(1, ZONE_STEPS + 1):
        w = zone_word(sign["x"] + step[0] * ZONE_CELL * k, sign["z"] + step[1] * ZONE_CELL * k)
        if w == ZONE_NONE or w == own or w >= len(words):
            continue
        name = words[w].strip()
        if name and not NOT_LAND.search(name):
            return name
    return None


def road_text(sign, words):
    """A road sign's words: per quarter the nearest town, else the region."""
    towns = [(TOWN_SIGN_NAMES.get(n, n), x, z) for n, x, z in COACH_TOWNS]
    best = directions(sign, towns, TOWN_REACH) if sign["world"] == TOWN_WORLD else {}
    lines, how = [], []
    for q, _, step in QUARTERS:
        if q in best:
            name, d = best[q]
            lines.append(f"{q}: {name}")
            how.append(f"{q} {name} {d:.0f}")
        elif words and sign["world"] == TOWN_WORLD:
            region = region_that_way(sign, step, words)
            if region:
                lines.append(f"{q}: {region}")
                how.append(f"{q} region {region}")
    return "\n".join(lines), "road: " + ", ".join(how) if how else "road: nothing in reach"


def beyond_gate(sign, city, here, centres):
    """A sign whose nearest titled place of its city is a gate, and which
    stands farther from the city's middle than the gate's people: outside the
    gate, on the road (Freeport's north gate)."""
    titled = [(d, p) for d, p in here if p["city"] == city and not p["keeper"]]
    if not titled or city not in centres or not re.search(r"\bGate\b", titled[0][1]["title"]):
        return False
    gate = min(titled[0][1]["people"], key=lambda n: dist3(sign, n))
    cx, cz = centres[city]
    return math.hypot(sign["x"] - cx, sign["z"] - cz) > math.hypot(gate["x"] - cx, gate["z"] - cz)


def plan_signs(npcs, prima_lines, words, verbose=False):
    """Every sign: {npcid, name, world, x, z, kind, text, note}."""
    places = city_places(prima_lines, npcs) if prima_lines else []
    if verbose:
        for p in places:
            who = ", ".join(sorted({n["name"] for n in p["people"]}))
            print(f"  map of {p['city']} {p['no']} {p['title']!r}{' (keeper)' if p['keeper'] else ''}: {who or '-'}")
    # each city's outline, the hull of its mapped people, and its middle
    outlines, centres = {}, {}
    for c in {p["city"] for p in places}:
        pts = [(n["x"], n["z"]) for p in places if p["city"] == c for n in p["people"]]
        outlines[c] = hull(pts)
        if pts:
            centres[c] = (sum(x for x, _ in pts) / len(pts), sum(z for _, z in pts) / len(pts))
    signs = [n for n in npcs if n["body"] == SIGN_BODY]
    plan = []
    for s in sorted(signs, key=lambda n: n["id"]):
        row = {"npcid": s["id"], "name": s["name"], "world": s["world"], "x": round(s["x"], 1), "z": round(s["z"], 1)}
        m = BOARD_NAME.match(s["name"])
        if m:
            city = m.group("city").strip()
            city = BOARD_CITIES.get(city, city)
            welcome = BOARD_WELCOME.get(city, f"Welcome to {city}.")
            row.update(kind="board", text=welcome, note=f"ours: a welcome naming {city}; the news row follows it")
            plan.append(row)
            continue
        m = NAMED_SIGN.match(s["name"])
        if m:
            row.update(kind="place", text=m.group("place"), note="its own name")
            plan.append(row)
            continue
        here = [(min(dist3(s, n) for n in p["people"]), p) for p in places if p["people"] and s["world"] == TOWN_WORLD]
        here.sort(key=lambda t: (t[0], t[1]["city"], t[1]["no"]))
        at = [(d, p) for d, p in here if d <= BUILDING_REACH]
        if at:
            d, p = at[0]
            if p["keeper"]:
                titled = [(dd, pp) for dd, pp in at if not pp["keeper"] and dd <= d + TITLE_SLACK]
                if titled:
                    d, p = titled[0]
            who = min(p["people"], key=lambda n: dist3(s, n))
            row.update(kind="building", text=place_title(p),
                       note=f"Prima's map of {p['city']}, {p['no']} {p['title']}: {who['name']} {d:.0f} away")
            plan.append(row)
            continue
        city = here[0][1]["city"] if here and here[0][0] <= CITY_REACH else None
        if city and inside(outlines[city], s["x"], s["z"], HULL_MARGIN) and not beyond_gate(s, city, here, centres):
            targets = []
            for p in places:
                if p["city"] == city and not p["keeper"] and p["people"]:
                    n = min(p["people"], key=lambda n: dist3(s, n))
                    targets.append((p["title"], n["x"], n["z"]))
            best = directions(s, targets, CITY_SPAN)
            if best:
                lines = [f"{q}: {best[q][0]}" for q, _, _ in QUARTERS if q in best]
                how = ", ".join(f"{q} {best[q][0]} {best[q][1]:.0f}" for q, _, _ in QUARTERS if q in best)
                row.update(kind="city", text="\n".join(lines), note=f"in {city}, Prima's places: {how}")
                plan.append(row)
                continue
        text, note = road_text(s, words)
        row.update(kind="road", text=text, note=note)
        plan.append(row)
    return plan


# ---- writing ---------------------------------------------------------------------

def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def backup(path):
    when = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    dst = f"{path}.bak-{when}-pre-signs"
    shutil.copy2(path, dst)
    return dst


def still_there(npcs_by_id, row):
    """A planned row's sign, still the same npcs row: its name and place."""
    n = npcs_by_id.get(row["npcid"])
    return (n is not None and n["body"] == SIGN_BODY and n["name"] == row["name"] and n["world"] == row["world"]
            and abs(n["x"] - row["x"]) <= 1.0 and abs(n["z"] - row["z"]) <= 1.0)


def apply(db, plan):
    """Write each planned row whose sign has none, and the news row; logged.
    Returns (written, kept): kept are the signs that already had a row."""
    db.execute("BEGIN IMMEDIATE")
    try:
        made = not has_table(db, "signtext")
        db.execute(SCHEMA)
        db.execute(LOG_SCHEMA)
        if made:
            db.execute("INSERT OR REPLACE INTO signtext_seed VALUES (?, '{}')", (MADE_TABLE,))
        rows = [dict(r) for r in plan]
        rows.append({"npcid": NEWS_ID, "kind": "news", "text": "", "note": NEWS_NOTE})
        written = kept = 0
        for r in rows:
            if db.execute("SELECT 1 FROM signtext WHERE npcid = ?", (r["npcid"],)).fetchone():
                kept += 1
                continue
            db.execute("INSERT INTO signtext (npcid, kind, text, source, note) VALUES (?, ?, ?, ?, ?)",
                       (r["npcid"], r["kind"], r["text"], SOURCE, r["note"]))
            db.execute("INSERT OR REPLACE INTO signtext_seed VALUES (?, ?)",
                       (r["npcid"], json.dumps({c: r[c] for c in COLS}, sort_keys=True)))
            written += 1
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    return written, kept


def undo(db):
    if not has_table(db, "signtext_seed"):
        print("nothing logged: nothing to undo")
        return
    db.execute("BEGIN IMMEDIATE")
    try:
        gone = kept = 0
        made = False
        for npcid, row in db.execute("SELECT npcid, row FROM signtext_seed ORDER BY npcid").fetchall():
            if npcid == MADE_TABLE:
                made = True
                continue
            r = json.loads(row)
            now = db.execute(f"SELECT {', '.join(COLS)}, source FROM signtext WHERE npcid = ?", (npcid,)).fetchone()
            if now is None:
                continue
            if dict(zip(COLS, now[:-1])) != r or now[-1] != SOURCE:
                kept += 1
                continue
            db.execute("DELETE FROM signtext WHERE npcid = ?", (npcid,))
            gone += 1
        db.execute("DROP TABLE signtext_seed")
        # a table --apply made goes again when nothing is left in it (the
        # server makes it at its start)
        if made and db.execute("SELECT count(*) FROM signtext").fetchone()[0] == 0:
            db.execute("DROP TABLE signtext")
        db.execute("COMMIT")
    except BaseException:
        db.execute("ROLLBACK")
        raise
    print(f"took back {gone} rows; {kept} a GM changed since left as they are")


def report(plan, sources):
    kinds = collections.Counter(r["kind"] for r in plan)
    for r in plan:
        text = r["text"].replace("\n", " / ") or "(nothing)"
        print(f"{r['npcid']:>7} {r['name']!r} w{r['world']} ({r['x']:.0f}, {r['z']:.0f}) {r['kind']}: {text}")
        print(f"          {r['note']}")
    print(f"{len(plan)} signs: " + ", ".join(f"{k} {n}" for k, n in sorted(kinds.items())) + f" ({sources}); "
          f"and the news row (npcid {NEWS_ID}, empty: {NEWS_NOTE})")


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--prima", help="Prima's Frontiers guide as layout text (pdftotext -layout)")
    ap.add_argument("--words", help="the disc's DATA/WORDS.TXT")
    ap.add_argument("--save-plan", help="write the plan to this file (JSON)")
    ap.add_argument("--plan", help="read a saved plan in place of the sources")
    ap.add_argument("--no-backup", action="store_true", help="no copy of the database first (tests)")
    ap.add_argument("--verbose", action="store_true", help="list the guide's places and who stands at each")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="write a row for each sign without one (logged)")
    g.add_argument("--undo", action="store_true", help="take back what --apply wrote")
    a = ap.parse_args()
    if not os.path.exists(a.db):
        sys.exit(f"{a.db}: no such database")
    db = sqlite3.connect(a.db, timeout=30, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 30000")
    if a.undo:
        if not a.no_backup and has_table(db, "signtext_seed"):
            print(f"copy: {backup(a.db)}")
        undo(db)
        return
    npcs = load_npcs(db)
    if a.plan:
        if a.prima or a.words:
            sys.exit("--plan is the sources' plan already: give it alone")
        with open(a.plan) as f:
            saved = json.load(f)
        by_id = {n["id"]: n for n in npcs}
        plan = [r for r in saved if still_there(by_id, r)]
        for r in saved:
            if not still_there(by_id, r):
                print(f"left out: {r['npcid']} {r['name']!r}: no longer that sign there")
        sources = f"from {a.plan}"
    else:
        lines = None
        if a.prima:
            with open(a.prima, encoding="utf-8") as f:
                lines = f.read().split("\n")
        words = read_words(a.words) if a.words else None
        plan = plan_signs(npcs, lines, words, a.verbose)
        sources = ", ".join(s for s in ("Prima's maps" if lines else "", "WORDS.TXT's zones" if words else "") if s) \
            or "no sources: every sign a road sign to the towns"
    report(plan, sources)
    if a.save_plan:
        with open(a.save_plan, "w") as f:
            json.dump(plan, f, indent=1, sort_keys=True)
        print(f"plan: {a.save_plan}")
    if not a.apply:
        print("report only: --apply to write")
        return
    if not a.no_backup:
        print(f"copy: {backup(a.db)}")
    written, kept = apply(db, plan)
    print(f"wrote {written} rows (the news row among them when it was not there); {kept} signs had one and keep it; "
          f"the server says them within 30 s, no restart")


if __name__ == "__main__":
    main()
