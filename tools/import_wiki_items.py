#!/usr/bin/env python3
"""The items the EQOA wiki names that this server lacks, made and put where the wiki says they come from (the
user, 2026-11-02: "lets check if the wiki has more items we are missing", then "yes add the wiki items").

The wiki (eqoa.fandom.com, its players' pages) is read through its MediaWiki API (--fetch: every article's
wikitext and categories into the operator's own folder; nothing of it is in this repository, as Allakhazam's
pages are not: tools/fetch_item_pages.py). It names an item in five places:
- its **Item List** page: a text line an item (Classic-era), "NAME - level AC/DMG [type|range] slot HP DUR races
  classes bonus trade found";
- an **item page** (the item categories): "==Statistics==" (classes, races, item level, each stat), the
  ItemTransformer infobox (type, dur, requires, description, location), "==Drop Information==";
- a **set page**'s table: piece, level, slot, armour set (material), attack type, damage, modifiers;
- a **named creature's page**: its "==Loot==" list;
- a **creature type's page** (Undead, Frogloks, Gnolls, Aviaks): the sets and items only that type drops.
An item is missing when no record of ours has its name (case, apostrophes, a backtick, spacing and a plural
aside, and two letters of a typo: the wiki's "Rusty Hachet" is our Rusty Hatchet; a name it words otherwise,
"Short Sword of Ykesha" for our "Short Sword of the Ykesha", is in ALIASES; an Item List line whose slot, level,
classes, AC or damage, attack type and first word are one of ours is that item's Classic name, "Banded Gloves"
for Frontiers' Banded Gauntlets). Only items with a level and a
slot are made (from a Sandstorm record of the same name, else an item page, a set row or an Item List line, in
that order); a name in a loot list alone is listed, not made.

Each record:
- **Sandstorm's** fields where its harvests hold the item (its icon, value, slot, type, damage, range, level,
  masks, durability, stats, description); the wiki's otherwise.
- **The icon and the look (OURS)**: an armour piece's style from its material, read off its page (a model
  picture "ChainMail..." chain 3, "BandedMail..." banded 6; an icon picture "Icon-...-plate" plate 4,
  "-scale" scale 7, "-leather" leather 2, "-cloth" cloth 1) or its set's armour set, else from its classes
  (plate classes alone 4; no caster, monk or druid 3, or 7 when the name says scale; a monk or druid 2; a
  magician, necromancer, enchanter or wizard 1: the alchemist wears leather), and the icon this server's
  pieces of that slot and style carry most. A weapon's model and icon this
  server's weapons of its attack type carry most among those named with its last word, else of its type. A
  jewel, belt, held item or shield the commonest icon (and a shield's model) of this server's items of its slot
  named with its kind's word.
- **Its value**: Sandstorm's, else tools/stock_merchants.py's rule.
- Hit points the line's or 200 a level, durability 50, NO TRADE when the wiki's trade says No.

Where it comes from (OURS where the wiki gives no rate; every row logged):
- a merchant Sandstorm's harvest shows selling it, or the Halas vendor the Item List names ("Halas - Yelda": our
  "Smithy Yelda"), when this server has that merchant: onto its catalog (at most 40 wares);
- a named creature of this server's the wiki says drops it: a loot table of its own ("wiki <name>"), rolled
  once a kill at NAMED_CHANCE percent (npcloot; a creature the wiki calls generic, "bandit", GENERIC_CHANCE);
- a creature type with a level range ("dropped by undead mobs ... (46-50)"): one table a type ("wiki undead",
  "wiki frogloks") with each item for those levels, bound at TYPE_CHANCE percent to every creature name of that
  type spawned within one of the ranges (undead: npcundead's names and the undead words; frogloks: the word),
  an "ULTRA RARE" item weighted 1 against its range's others' 100;
- a region of the loot model ("[[Forest]]"): the region's uncommon pool for 5 levels round its own (lootpool);
- else, unless it is NO TRADE (a quest's reward: listed), the uncommon pool of every region ('*'), as above.
A category pool's size never changes how often its category drops (lootcategory.go), and an npcloot table rolls
beside the categories: nothing any creature drops now is taken away.

Every row made is logged in wiki_item_import_log; --undo deletes each still as this made it, then the log.
Report only by default. The server reads item records, catalogs and loot as it goes: no restart (a player sees
a new item's record when it reaches them).

    tools/import_wiki_items.py --fetch <folder>
    tools/import_wiki_items.py [--db eqoa.db] --wiki <folder> --prices <harvest JSONs or folders> ... [--apply | --undo]
"""
import argparse
import collections
import glob
import html
import json
import math
import os
import re
import sqlite3
import sys
import time
import urllib.parse
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from fix_dump_weapons import by_words  # noqa: E402
from fill_item_values import robust_rule  # noqa: E402
from stock_merchants import Measured, STAT_COLS  # noqa: E402

DEFAULT_DB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "eqoa.db")
API = "https://eqoa.fandom.com/api.php"
UA = "Mozilla/5.0 (X11; Linux x86_64) eqoa-server tools/import_wiki_items.py"

ID_FIRST, ID_LAST = 713000, 713999      # the made records' patternids (checked free at --apply)
NAMED_CHANCE, GENERIC_CHANCE, TYPE_CHANCE = 25, 5, 3
POOL_SPAN = 5
CATALOG_CAP = 40
ULTRA_WEIGHT, WEIGHT = 1, 100

# a name the wiki misspells, as the record is made (wiki key -> name)
NAME_FIXES = {"frozen crudgel": "Frozen Cudgel"}
# the wiki's other wording of an item of ours (wiki key -> our key)
ALIASES = {"short sword of ykesha": "short sword of the ykesha", "robe of the dragonhoard": "robe from the dragonhoard",
           "gloves of the plaguebringer": "gloves of plaguebringer", "steel banded chest": "steel banded chestguard",
           "seraphic planar breastplate": "seraphic planar chestplate", "q.staff of understanding": "quarterstaff of understanding",
           "centaur smithing hammer": "centaur smithy hammer", "essence weaver ring": "essenceweave ring",
           "brgith mossy spear": "bright mossy spear", "dog breath leggings": "dog breath legs",
           "alpine walking staff": "alpine walking stick", "vambracers of toxicity": "bracers of toxicity",
           "packmaster helmet": "packmaster helm", "cizkik's stinger": "cizka's stinger"}
# the wiki's name of a creature this server spells otherwise (wiki key -> ours)
SOURCE_ALIASES = {"cazic thule": "cazicthule", "bertoxxulous": "bertoxxulous the plaguebringer"}
ITEM_CATS = {"Category:Armor": None, "Category:Weapons": None, "Category:Chestpieces": 5, "Category:Boots": 11,
             "Category:Leggings": 10, "Category:Gauntlets": 19, "Category:Belts": 9, "Category:Robes": 2,
             "Category:Bracers": 7, "Category:Shields": 13, "Category:Helms": 1, "Category:Two-hand": 15,
             "Category:Secondary": 14, "Category:Primary": 12, "Category:Held": 18, "Category:Rings": 8,
             "Category:Necklaces": 4}
SLOT_WORDS = {"head": 1, "helm": 1, "robe": 2, "ear": 3, "earring": 3, "neck": 4, "necklace": 4, "torso": 5, "chest": 5,
              "chestpiece": 5, "forearm": 6, "wrist": 6, "2forearm": 7, "arms": 7, "bracers": 7, "ring": 8, "finger": 8,
              "waist": 9, "belt": 9, "legs": 10, "leggings": 10, "feet": 11, "boots": 11, "primary": 12, "shield": 13,
              "secondary": 14, "2hand": 15, "two-hand": 15, "bow": 16, "crossbow": 16, "thrown": 16, "held": 18,
              "gloves": 19, "hands": 19, "gauntlets": 19}
ATTACK = {"1hs": 1, "2hs": 2, "1hb": 3, "2hb": 4, "1hp": 5, "2hp": 6, "bow": 7, "crossbow": 9, "thrown": 10}
CLASS_ABBR = {"wa": 0, "war": 0, "ra": 1, "ran": 1, "p": 2, "pal": 2, "sk": 3, "mk": 4, "mnk": 4, "b": 5, "brd": 5,
              "rg": 6, "rge": 6, "rog": 6, "d": 7, "drd": 7, "dru": 7, "sh": 8, "sha": 8, "c": 9, "cl": 9, "clr": 9,
              "mg": 10, "ma": 10, "mag": 10, "n": 11, "nec": 11, "e": 12, "enc": 12, "wz": 13, "wiz": 13, "al": 14, "alc": 14}
RACE_ABBR = {"hm": 0, "hum": 0, "e": 1, "elf": 1, "de": 2, "delf": 2, "g": 3, "gno": 3, "dw": 4, "dwf": 4, "t": 5, "trl": 5,
             "b": 6, "bar": 6, "hf": 7, "hlf": 7, "er": 8, "eru": 8, "o": 9, "ogr": 9}
ALL_CLASSES, ALL_RACES = (1 << 15) - 1, (1 << 10) - 1
STAT_NAMES = {"str": "str", "sta": "sta", "agi": "agi", "dex": "dex", "wis": "wis", "int": "intelligence", "cha": "cha",
              "hp": "HPMAX", "hpmax": "HPMAX", "pow": "POWMAX", "power": "POWMAX", "powmax": "POWMAX", "ac": "AC",
              "hot": "HoT", "pot": "PoT", "pr": "PR", "dr": "DR", "fr": "FR", "cr": "CR", "lr": "LR", "ar": "AR"}
PLATE, CHAIN, LEATHER, CLOTH, BANDED, SCALE = 4, 3, 2, 1, 6, 7
CASTERS = {10, 11, 12, 13}         # cloth alone; the alchemist wears leather (17407, the leather sets' classes)
ARMOUR = {1, 5, 7, 10, 11, 19}
# entity/undead.go reUndeadName: a name with no npcundead row is undead when it says so
UNDEAD_WORDS = (r"\b(?:skeletons?|skeletal|zombies?|ghouls?|ghasts?|mumm(?:y|ies|ified)|spectres?|specters?|spectral|"
                r"wraiths?|liches?|undead|revenants?|ghosts?|ghostly|phantoms?|phantasms?|apparitions?|banshees?|wights?|"
                r"unliving|unresting|reanimated|risen|raised|cadavers?|rotting|decaying|bones|eidolons?|souls?|haunts?|dead|"
                r"shades?|restless)\b")

LOG = "wiki_item_import_log"
LOG_DDL = """CREATE TABLE IF NOT EXISTS {log} (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    tbl     TEXT NOT NULL,     -- itempattern | npcitem | loottables | npcloot | lootpool
    rid     TEXT NOT NULL,     -- the row: patternid, itemid, id, or npc_name|tablename
    what    TEXT NOT NULL,     -- the item and why
    applied TEXT NOT NULL DEFAULT (datetime('now'))
)"""


def key(n):
    n = re.sub(r"\[\[File:[^\]]*\]\]", "", html.unescape(n or ""))     # a picture beside a name in a list
    n = re.sub(r"\[\[(?:[^|\]]*\|)?([^\]]*)\]\]", r"\1", n)
    n = re.sub(r"<[^>]+>|'''|''", "", n)
    return " ".join(n.replace("`", "'").replace("’", "'").replace("_", " ").lower().split()).strip(" .,:;*-")


def loose(k):
    k = re.sub(r"[^a-z0-9 ]", "", k.replace("-", " "))
    return " ".join(re.sub(r"s\b", "", w) for w in k.split())


def lev(a, b):
    """The edits between two words, two letters swapped one edit ("Burlpa", "Trage")."""
    d = [[i + j if i * j == 0 else 0 for j in range(len(b) + 1)] for i in range(len(a) + 1)]
    for i in range(1, len(a) + 1):
        for j in range(1, len(b) + 1):
            d[i][j] = min(d[i - 1][j] + 1, d[i][j - 1] + 1, d[i - 1][j - 1] + (a[i - 1] != b[j - 1]))
            if i > 1 and j > 1 and a[i - 1] == b[j - 2] and a[i - 2] == b[j - 1]:
                d[i][j] = min(d[i][j], d[i - 2][j - 2] + 1)
    return d[-1][-1]


def titled(name):
    """A wiki name as a record's: an ALL CAPS line's in title case, a page's as it is."""
    n = " ".join(html.unescape(name).replace("_", " ").split())
    if n.isupper():
        n = " ".join(w.capitalize() if w not in ("OF", "THE") else w.lower() for w in n.split())
    return n[0].upper() + n[1:] if n else n


# ---- the wiki ----

def fetch(folder, delay=0.7):
    os.makedirs(folder, exist_ok=True)
    pages, cont = {}, {}
    while True:
        q = {"action": "query", "generator": "allpages", "gapnamespace": 0, "gaplimit": 50, "prop": "revisions|categories",
             "rvprop": "content", "rvslots": "main", "cllimit": "max", "format": "json", **cont}
        with urllib.request.urlopen(urllib.request.Request(API + "?" + urllib.parse.urlencode(q), headers={"User-Agent": UA}),
                                    timeout=60) as r:
            d = json.load(r)
        for p in d.get("query", {}).get("pages", {}).values():
            old = pages.get(p["title"], {})
            txt = p["revisions"][0]["slots"]["main"].get("*", "") if p.get("revisions") else old.get("text")
            pages[p["title"]] = {"text": txt, "cats": sorted(set(old.get("cats", [])) |
                                                           {c["title"] for c in p.get("categories", [])})}
        if "continue" not in d:
            break
        cont = {k: v for k, v in d["continue"].items()}
        time.sleep(delay)
    with open(os.path.join(folder, "allpages.json"), "w") as fh:
        json.dump(pages, fh)
    print(f"{len(pages)} articles into {folder}/allpages.json")


def mask(field, table, all_bits):
    """A classes or races field ("Wa,Ra,P" | "WAR RAN PAL" | "All") as bits, or None when unknown."""
    f = (field or "").strip().lower()
    if not f or "?" in f:
        return None
    if f in ("all", "any"):
        return all_bits
    bits = 0
    for w in re.split(r"[,\s/]+", f):
        if w in table:
            bits |= 1 << table[w]
    return bits or None


def bonus_stats(text):
    """ "Str+6,Sta/Agi+7,Pow+25,FR-15" (an Item List bonus) as {column: value}."""
    out = {}
    for part in re.split(r",", (text or "").replace("_", "+")):
        m = re.match(r"\s*([A-Za-z/]+)\s*([+-])\s*(\d+)", part)
        if not m:
            continue
        v = int(m.group(3)) * (1 if m.group(2) == "+" else -1)
        for n in m.group(1).split("/"):
            col = STAT_NAMES.get(n.strip().lower())
            if col:
                out[col] = out.get(col, 0) + v
    return out


def item_list(text):
    """{key: item} off the Item List page."""
    out = {}
    pat = re.compile(r"^(?P<name>.+?)\s+-?\s*(?P<lvl>\d+)\s+(?P<ac>\d+|\?+)\s+(?:(?P<att>[12]H[SBP])\s+|(?P<rng>\d+)\s+)?"
                     r"(?P<equip>[A-Za-z0-9-]+)\s+(?P<hp>\d+|\?+)\s+(?P<dur>\d+|\?+)\s+(?P<race>\S+)\s+(?P<cls>\S+)\s+"
                     r"(?P<bonus>\S+)\s+(?P<trade>yes|no)\b\s*(?P<found>.*)$", re.I)
    for line in text.splitlines():
        m = pat.match(re.sub(r"'''", "", line).strip())
        if not m:
            continue
        slot = SLOT_WORDS.get(m["equip"].lower())
        if slot is None:
            continue
        at = ATTACK.get((m["att"] or "").lower()) or ATTACK.get(m["equip"].lower()) if (m["att"] or slot == 16) else 0
        if slot == 14 and at in (2, 4, 6):
            slot = 15
        it = dict(name=titled(m["name"]), level=int(m["lvl"]), slot=slot, attack=at or 0,
                  dmg=int(m["ac"]) if at and m["ac"].isdigit() else 0, rng=int(m["rng"]) if m["rng"] else 0,
                  stats=bonus_stats(m["bonus"]), hp=int(m["hp"]) if m["hp"].isdigit() else 0,
                  dur=int(m["dur"]) if m["dur"].isdigit() else 0, races=mask(m["race"], RACE_ABBR, ALL_RACES),
                  classes=mask(m["cls"], CLASS_ABBR, ALL_CLASSES), notrade=m["trade"].lower() == "no",
                  found=m["found"].strip(), src="the wiki's Item List")
        if not at and m["ac"].isdigit() and int(m["ac"]):
            it["stats"]["AC"] = it["stats"].get("AC", 0) + int(m["ac"])
        out[key(m["name"])] = it
    return out


def infobox(text):
    out = {}
    for m in re.finditer(r"^\|\s*(\w+)\s*=\s*(.*)$", text, re.M):
        out.setdefault(m.group(1).lower(), m.group(2).strip())
    return out


def item_page(title, page):
    """An item page as an item, or None."""
    t, box = page["text"] or "", infobox(page["text"] or "")
    slot = None
    for c in page["cats"]:
        slot = slot or ITEM_CATS.get(c)
    slot = SLOT_WORDS.get((box.get("type") or "").strip().lower(), slot)
    lv = re.search(r"Item Level:\s*(\d+)", t) or re.search(r"Level\s+(\d+)", box.get("requires", ""))
    stats, dmg = {}, 0
    for n, v in re.findall(r"^\*\s*([A-Za-z]+):\s*(-?\d+)", t, re.M):
        if n.upper() == "DMG":
            dmg = int(v)
        elif STAT_NAMES.get(n.lower()):
            stats[STAT_NAMES[n.lower()]] = int(v)
    cl = re.search(r"Classes:\s*([^\n]+)", t)
    rc = re.search(r"Races:\s*([^\n]+)", t)
    desc = box.get("description") or ""
    if not desc:
        first = re.split(r"\n==", re.sub(r"<onlyinclude>.*?</onlyinclude>", "", t, flags=re.S))[0]
        first = re.sub(r"\[\[File:[^\]]*\]\]|\{\{[^}]*\}\}", "", first)
        desc = key(first) and " ".join(re.sub(r"\[\[(?:[^|\]]*\|)?([^\]]*)\]\]", r"\1", first).replace("''", "").split())
    at = 0
    if dmg:
        at = (by_words(title) or (0, False))[0] or weapon_inside(title, slot, mask(cl.group(1) if cl else "", CLASS_ABBR, ALL_CLASSES))
        if slot in (15,) and at in (1, 3, 5):
            at += 1
        slot = slot or (15 if at in (2, 4, 6) else 12)
    if slot is None or not lv:
        return None
    pics = " ".join(re.findall(r"\|\s*(?:image|icon)\s*=\s*([^\n|}]+)", t) + re.findall(r"\[\[File:([^|\]]+)", t)).lower()
    return dict(name=titled(title), level=int(lv.group(1)), slot=slot, attack=at, dmg=dmg, rng=0, stats=stats,
                hp=0, dur=int(box["dur"]) if box.get("dur", "").isdigit() else 0,
                races=mask(rc.group(1) if rc else "", RACE_ABBR, ALL_RACES),
                classes=mask(cl.group(1) if cl else "", CLASS_ABBR, ALL_CLASSES), notrade=False,
                desc=(desc or "")[:250], pics=pics, location=box.get("location", ""), src=f"its wiki page ({title})")


def weapon_inside(name, slot, classes):
    """(OURS) a weapon's attack type when fix_dump_weapons' words miss: a weapon word inside a word ("warsword":
    a sword, slashing), else a caster's two-hander a staff (blunt); 0 when nothing says."""
    n = name.lower()
    for words, at in ((("sword", "blade", "axe", "cutlass", "sabre", "scimitar", "render", "splitter", "cleaver"), 1),
                      (("hammer", "club", "mace", "staff", "maul", "cudgel"), 3), (("spear", "dagger", "lance", "pike", "claw"), 5)):
        if any(w in n for w in words):
            return at + (1 if slot == 15 else 0)
    if slot == 15 and classes and classes & (0b1111 << 10) and not classes & 0b1111111111:
        return 4
    return 0


def set_rows(title, page):
    """{key: item} off a set page's table."""
    out = {}
    t = page["text"] or ""
    mat = re.search(r"\b(plate|chain|scale|leather|cloth|banded)\b", t.split("{|")[0], re.I)
    for row in t.split("|-")[1:]:
        cells = [c.strip() for c in re.split(r"^\|", row, flags=re.M)[1:]]
        if len(cells) < 6 or not cells[0].startswith("[["):
            continue
        name = re.sub(r"\[\[(?:[^|\]]*\|)?([^\]]*)\]\]", r"\1", cells[0]).strip()
        slot = SLOT_WORDS.get(cells[2].lower()) if len(cells) > 2 else None
        if not cells[1].isdigit() or slot is None:
            continue
        stats = {}
        for n, v in re.findall(r"([A-Za-z]+):\s*(-?\d+)", " ".join(cells[6:])):
            if STAT_NAMES.get(n.lower()):
                stats[STAT_NAMES[n.lower()]] = int(v)
        cls = re.search(r"useable by \[\[([A-Za-z]+)", t)
        out[key(name)] = dict(name=titled(name), level=int(cells[1]), slot=slot, attack=0, dmg=0, rng=0, stats=stats,
                              hp=0, dur=0, races=ALL_RACES, classes=mask(cls.group(1)[:3] if cls else "", CLASS_ABBR, ALL_CLASSES),
                              notrade=False, desc="", pics=(cells[3] if len(cells) > 3 else "") + " " + (mat.group(1) if mat else ""),
                              src=f"its set's table ({title})")
    return out


def sources(pages, want):
    """{item key: [(kind, who, low, high, ultra)]}: who drops it, by the wiki."""
    out = collections.defaultdict(list)
    for title, p in pages.items():
        t = p["text"] or ""
        tk = key(title)
        for sec in re.finditer(r"^==+\s*([^=]*?)\s*==+\s*$(.*?)(?=^==|\Z)", t, re.M | re.S):
            head, body = sec.group(1).lower(), sec.group(2)
            if "loot" in head:
                for b in re.finditer(r"^\*+\s*(.+)$", body, re.M):
                    k = key(re.sub(r"\s*\([^)]*\)", "", b.group(1)))
                    if k in want:
                        out[k].append(("named", title, 0, 0, False))
        if tk in want:
            for who in re.findall(r"(?:dropped by|drops from)\s*\[\[([^\]|]+)", t, re.I):
                if who.lower() not in ("undead",):
                    out[tk].append(("named", who, 0, 0, False))
            for kind, lo, hi in re.findall(r"(undead|frogloks?)[^\n]*?\(?(\d+)\s*(?:-|to)\s*(\d+)", t, re.I):
                out[tk].append(("type", "undead" if kind.lower() == "undead" else "frogloks", int(lo), int(hi),
                                "ultra rare" in t.lower()))
            loc = re.search(r"\|\s*location\s*=\s*\[\[([^\]|]+)", t)
            if loc:
                out[tk].append(("place", loc.group(1), 0, 0, False))
    return out


# ---- this server ----

def loot_name(name):
    """A creature's name as the server's loot binds it (database.NPCClassKey: lower case, trimmed, a trailing $ off),
    which key() is not: it reads a backtick as an apostrophe ("Gal`Saris ghoul")."""
    n = (name or "").strip().lower()
    return (n[:-1] if n.endswith("$") else n).strip()


def has_table(db, name):
    return db.execute("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (name,)).fetchone()[0] > 0


def harvest_records(paths):
    """{key: (ware, merchant name)} off eqoa harvest JSONs: Sandstorm's records and who sells them."""
    out = {}
    files = []
    for p in paths:
        files += glob.glob(os.path.join(p, "**", "*.json"), recursive=True) if os.path.isdir(p) else [p]
    for f in sorted(files):
        try:
            d = json.load(open(f))
        except (OSError, ValueError):
            continue
        if not isinstance(d, dict):
            continue
        for m in d.get("merchants") or []:
            for w in m.get("wares") or []:
                out.setdefault(key(w["name"]), (w, m.get("name", "")))
        for w in d.get("inventory") or []:
            if isinstance(w, dict) and w.get("name"):
                out.setdefault(key(w["name"]), (w, ""))
    return out


class Ours:
    """What this server holds that the plan reads: names, styles' icons, weapons' looks, merchants, creatures."""

    def __init__(self, db):
        self.names = {}
        rows = db.execute("SELECT patternid, itemname, equipslot, model, itemicon, attacktype, classuse, levelreq, "
                          "coalesce(AC, 0), weapondamage FROM itempattern").fetchall()
        # an item of the same slot, level, classes and AC or damage: the Item List's Classic name of one of ours
        # ("Banded Gloves" is Frontiers' Banded Gauntlets)
        self.twins = {(slot, lv, cls, ac if not dmg else 0, dmg, at, (key(n).split() or [""])[0]): n
                      for _p, n, slot, _m, _i, at, cls, lv, ac, dmg in rows}
        rows = [r[:7] for r in rows]
        for pid, n, *_ in rows:
            self.names.setdefault(key(n), pid)
        self.loose = {loose(k): pid for k, pid in self.names.items()}
        self.squeezed = {loose(k).replace(" ", "") for k in self.names}
        self.by_count = collections.defaultdict(list)
        for k in self.loose:
            self.by_count[len(k.split())].append(k.split())
        by_style = collections.defaultdict(collections.Counter)
        self.weapon = collections.defaultdict(collections.Counter)    # (attack type, last word | '') -> (model, icon)
        self.kind = collections.defaultdict(collections.Counter)      # (slot, kind word | '') -> (model, icon)
        self.masks = collections.defaultdict(collections.Counter)     # (slot, attack type) -> class masks
        for pid, n, slot, model, icon, at, cls in rows:
            words = re.findall(r"[a-z]+", (n or "").lower())
            last = words[-1] if words else ""
            if slot in ARMOUR and model:
                by_style[(slot, model)][icon] += 1
            if at and model and slot in (12, 14, 15, 16):
                self.weapon[(at, last)][(model, icon)] += 1
                self.weapon[(at, "")][(model, icon)] += 1
            if slot in (2, 3, 4, 6, 8, 9, 13, 18):
                for w in set(words):
                    self.kind[(slot, w)][(model, icon)] += 1
                self.kind[(slot, "")][(model, icon)] += 1
            if cls:
                self.masks[(slot, at)][cls] += 1
        self.style_icon = {k: c.most_common(1)[0][0] for k, c in by_style.items()}
        self.merchants = collections.defaultdict(list)                # key name -> npc_ids of merchants
        self.creatures = collections.defaultdict(set)                 # key name -> spawn levels
        self.loot_names = collections.defaultdict(set)                # key name -> the names npcloot binds (NPCClassKey's)
        for name, npc_id, t, lvl in db.execute("SELECT npc_name, npc_id, npc_type, npc_level FROM npcs"):
            self.creatures[key(name)].add(lvl or 0)
            self.loot_names[key(name)].add(loot_name(name))
            if npc_id and (t or 0) & 1:
                self.merchants[key(name)].append(npc_id)
        self.catalog = collections.Counter(n for (n,) in db.execute("SELECT npcid FROM npcitem"))
        tags = dict((key(n), u) for n, u in db.execute("SELECT name, undead FROM npcundead")) if has_table(db, "npcundead") else {}
        self.undead = {n for n in self.creatures if tags.get(n, 1 if re.search(UNDEAD_WORDS, n) else 0)}
        self.regions = {r for (r,) in db.execute("SELECT DISTINCT region FROM lootpool")} if has_table(db, "lootpool") else set()

    def have(self, k):
        """Whether one of ours is this item: its name spacing, case, apostrophes and a plural aside (the wiki's
        "Zombie Back Tunic" is our zombieback tunic), its other wording (ALIASES), or every word the same but
        one a letter off, two on a word of seven letters or more ("Grigand's Chain", "Rusty Hachet")."""
        if k in self.names or loose(k) in self.loose or ALIASES.get(k) in self.names:
            return True
        if loose(k).replace(" ", "") in self.squeezed:
            return True
        words = loose(k).split()
        for n in self.by_count.get(len(words), ()):
            diff = [(a, b) for a, b in zip(words, n) if a != b]
            if len(diff) == 1:
                a, b = diff[0]
                if lev(a, b) <= (2 if min(len(a), len(b)) >= 7 else 1) and min(len(a), len(b)) >= 4:
                    return True
        return False


def style_of(it):
    """An armour piece's style (OURS: its pictures' or set's material, else its classes)."""
    p = (it.get("pics") or "").lower()
    for word, style in (("chainmail", CHAIN), ("bandedmail", BANDED), ("platemail", PLATE), ("plate", PLATE),
                        ("scale", SCALE), ("leather", LEATHER), ("hide", LEATHER), ("cloth", CLOTH), ("silk", CLOTH),
                        ("banded", BANDED), ("chain", CHAIN)):
        if word in p:
            return style, f"its pictures' or set's material ({word})"
    cls = it.get("classes") or ALL_CLASSES
    have = {i for i in range(15) if cls >> i & 1}
    name = it["name"].lower()
    if have & CASTERS:
        return CLOTH, "a caster may wear it"
    if have & {4, 7}:
        return LEATHER, "a monk or a druid may wear it"
    if have <= {0, 2, 3, 9}:
        return PLATE, "plate's classes alone"
    return (SCALE, "chain's classes, named scale") if "scale" in name else (CHAIN, "chain's classes")


def look(it, ours):
    """(model, icon, why) of a made record."""
    slot, name = it["slot"], it["name"].lower()
    words = re.findall(r"[a-z]+", name)
    if slot in ARMOUR:
        style, why = style_of(it)
        icon = ours.style_icon.get((slot, style)) or ours.style_icon.get((slot, CLOTH))
        return style, icon, f"style {style}: {why}"
    if slot in (12, 14, 15, 16):
        for k in ((it["attack"], words[-1] if words else ""), (it["attack"], "")):
            c = ours.weapon.get(k)
            if c:
                (model, icon), _ = c.most_common(1)[0]
                return model, icon, f"the look of this server's type {it['attack']} weapons" + (f" named {k[1]}" if k[1] else "")
        return 0, 0, "no weapon of its type"
    for w in reversed(words):
        c = ours.kind.get((slot, w))
        if c and sum(c.values()) >= 2:
            (model, icon), _ = c.most_common(1)[0]
            return model, icon, f"the commonest of this server's slot {slot} items named {w}"
    c = ours.kind.get((slot, ""))
    (model, icon), _ = c.most_common(1)[0] if c else ((0, 0), 0)
    return model, icon, f"the commonest of this server's slot {slot} items"


def plan(db, pages, harvest, ours):
    """[record dicts] to make and [(name, why)] left."""
    want_all = {}
    il = item_list(pages.get("Item List", {}).get("text") or "")
    for title, p in pages.items():
        cats = set(p["cats"])
        if "Category:Item Sets" in cats:
            for k, it in set_rows(title, p).items():
                want_all.setdefault(k, it)
    for title, p in pages.items():
        if set(ITEM_CATS) & set(p["cats"]) and "Category:Item Sets" not in p["cats"]:
            it = item_page(title, p)
            if it:
                want_all[key(title)] = it
    for k, it in il.items():
        want_all.setdefault(k, it)
    # every name the wiki gives an item, made or not
    named = set(want_all)
    for title, p in pages.items():
        for sec in re.finditer(r"^==+\s*([^=]*?)\s*==+\s*$(.*?)(?=^==|\Z)", p["text"] or "", re.M | re.S):
            if "loot" in sec.group(1).lower():
                for b in re.finditer(r"^\*+\s*(.+)$", sec.group(2), re.M):
                    k = key(re.sub(r"\s*\([^)]*\)", "", b.group(1)))
                    if 2 < len(k) < 50 and not k.endswith(" set") and k not in ours.creatures and not re.search(r"[\d,|=]", k):
                        named.add(k)
    src = sources(pages, named)
    made, left = [], []
    for k in sorted(named):
        if ours.have(k):
            continue
        h = harvest.get(k)
        it = want_all.get(k)
        if h:
            w, merchant = h
            stats = {}
            for idx, val in w.get("stats") or []:
                col = {0: "str", 1: "sta", 2: "agi", 3: "dex", 4: "wis", 5: "intelligence", 6: "cha", 8: "HPMAX",
                       10: "POWMAX", 12: "PoT", 13: "HoT", 14: "AC", 22: "PR", 23: "DR", 24: "FR", 25: "CR", 26: "LR",
                       27: "AR"}.get(idx)
                if col:
                    stats[col] = val
            it = dict(name=w["name"], level=w.get("level_req", 0), slot=w.get("wear_slot", -1), attack=w.get("attack_type", 0),
                      dmg=w.get("damage", 0), rng=w.get("range", 0), stats=stats, hp=w.get("max_hp", 0),
                      dur=w.get("durability", 50), races=w.get("race_mask"), classes=w.get("class_mask"),
                      notrade=bool(w.get("no_trade")), desc=w.get("desc", ""), icon=w.get("icon"), value=w.get("value"),
                      merchant=merchant, src="Sandstorm's record", found=(it or {}).get("found", ""))
        if not it:
            left.append((k, "the wiki gives a name alone: no level or slot"))
            continue
        if it["src"] == "the wiki's Item List":
            twin = ours.twins.get((it["slot"], it["level"], it.get("classes"), it["stats"].get("AC", 0) if not it["dmg"] else 0,
                                   it["dmg"], it["attack"], (k.split() or [""])[0]))
            if twin:
                left.append((k, f"ours by another name, every figure the same: {twin}"))
                continue
        it = dict(it, key=k, sources=src.get(k, []))
        if k in NAME_FIXES:
            it["name"] = NAME_FIXES[k]
        made.append(it)
    return made, left


def placements(it, ours):
    """[(kind, target, chance, weight, low, high)] where a made item comes from, and why."""
    out = []
    if it.get("merchant") and ours.merchants.get(key(it["merchant"])):
        out.append(("catalog", key(it["merchant"]), 0, 0, 0, 0, f"Sandstorm's {it['merchant']} sells it"))
    m = re.match(r"halas - ([a-z' ]+)$", (it.get("found") or "").lower())
    if m:
        vendor = next((n for n in ours.merchants if n.endswith(" " + m.group(1).strip())), None)
        if vendor:
            out.append(("catalog", vendor, 0, 0, 0, 0, f"the Item List's Halas vendor ({m.group(1).strip()})"))
    if out:
        return out[:1]
    lo, hi = max(1, it["level"] - POOL_SPAN), it["level"] + POOL_SPAN
    for kind, who, a, b, ultra in it["sources"]:
        k = SOURCE_ALIASES.get(key(who), key(who))
        if kind == "named" and k in ours.creatures:
            out.append(("npcloot", k, NAMED_CHANCE, WEIGHT, 1, 100, f"the wiki: {who} drops it"))
        elif kind == "type":
            out.append(("type", who, TYPE_CHANCE, ULTRA_WEIGHT if ultra else WEIGHT, a, b,
                        f"the wiki: {who} of {a}-{b} drop it" + (", ultra rare" if ultra else "")))
        elif kind == "place" and k in ours.regions:
            out.append(("pool", k, 0, 1, lo, hi, f"the wiki: found in {who}, the {k} region's pool"))
    f = (it.get("found") or "").lower()
    if not out and " - " in f:
        who = key(f.split(" - ", 1)[1])
        hit = next((n for n in ours.creatures if n == who or n == who.rstrip("s")), None)
        if hit:
            out.append(("npcloot", hit, GENERIC_CHANCE, WEIGHT, 1, 100, f"the Item List: {f.split(' - ', 1)[1].strip()} drops it"))
        elif "assassian d'" in f or "assassin d'" in f:
            out.append(("npcloot", "chief assassin daeld", NAMED_CHANCE, WEIGHT, 1, 100, "the Item List: Chief Assassin D'... (Daeld)"))
    if out:
        seen, once = set(), []
        for r in out:
            if (r[0], r[1]) not in seen:
                seen.add((r[0], r[1]))
                once.append(r)
        return once
    if it.get("notrade"):
        return []
    return [("pool", "*", 0, 1, lo, hi, "no source the wiki names here: every region's uncommon pool")]


def build(made, ours, rule, used_ids):
    """Records, catalog rows, loot rows (each with its reason)."""
    recs, rows = [], []
    nid = ID_FIRST
    for it in made:
        while nid in used_ids:
            nid += 1
        if nid > ID_LAST:
            raise SystemExit("the id block is full")
        used_ids.add(nid)
        model, icon, why = look(it, ours)
        if it.get("icon"):
            icon, why = it["icon"], why + "; Sandstorm's icon"
        cls = it.get("classes")
        if not cls:
            c = ours.masks.get((it["slot"], it["attack"]))
            cls = c.most_common(1)[0][0] if c else ALL_CLASSES
        races = it.get("races") or ALL_RACES
        rng = it.get("rng") or (25 if it["attack"] in (7, 9, 10) else 0)
        hp = it.get("hp") or 200 * max(it["level"], 1)
        stats = {c: it["stats"].get(c) for c in STAT_COLS}
        p = dict(id=nid, name=it["name"], cost=0, slot=it["slot"], level=it["level"], stack=1, dmg=it["dmg"], icon=icon,
                 desc=it.get("desc") or "", trade=1 if it["notrade"] else 0, lore=0, stats={c: v or 0 for c, v in stats.items()})
        value = it.get("value")
        vwhy = "Sandstorm's value"
        if not value:
            value, how = rule.price(p)
            vwhy = f"the rule ({how})"
        # every column the live schema holds NOT NULL, the unknown ones 0 as Return Home's records have them
        rec = dict(patternid=nid, itemcost=int(value or 1), unk1=0, itemicon=int(icon or 0), unk2=0, equipslot=it["slot"],
                   unk3=0, trade=1 if it["notrade"] else 0, rent=1, unk4=0, attacktype=it["attack"], weapondamage=it["dmg"],
                   unk5=rng, levelreq=it["level"], maxstack=1, maxhp=hp, duration=it.get("dur") or 50, classuse=cls,
                   raceuse=races, procanim=0, lore=0, unk6=0, craft=0, itemname=it["name"][:63],
                   itemdesc=(it.get("desc") or "")[:250], model=int(model or 0), color=4294967295,
                   **{c: (v or 0) for c, v in stats.items()})
        recs.append((rec, f"{it['src']}; look: {why}; value: {vwhy}"))
        for kind, target, chance, weight, lo, hi, reason in placements(it, ours):
            rows.append(dict(kind=kind, target=target, chance=chance, weight=weight, low=lo, high=hi, pid=nid,
                             name=it["name"], level=it["level"], why=reason))
    return recs, rows


def write(db, recs, rows, ours, log_table=LOG, kinds=None):
    """Make the records and their rows, logged in log_table. kinds: {type word: names of that type} for the
    "type" rows beside undead and frogloks."""
    db.execute(LOG_DDL.format(log=log_table))

    def log(tbl, rid, what):
        db.execute(f"INSERT INTO {log_table} (tbl, rid, what) VALUES (?, ?, ?)", (tbl, str(rid), what))
    # the AUTOINCREMENT counters the rows below move, for an undo that leaves the database as it found it
    for name, seq in db.execute("SELECT name, seq FROM sqlite_sequence WHERE name IN "
                                "('itempattern', 'npcitem', 'loottables', 'lootpool')").fetchall():
        log("sqlite_sequence", name, str(seq))
    for rec, why in recs:
        cols = ", ".join(rec)
        db.execute(f"INSERT INTO itempattern ({cols}) VALUES ({', '.join('?' * len(rec))})", list(rec.values()))
        log("itempattern", rec["patternid"], f"{rec['itemname']}: {why}")
    tables = set()
    placed = collections.Counter()
    for r in rows:
        if r["kind"] == "catalog":
            for npc_id in ours.merchants[r["target"]]:
                n = db.execute("SELECT count(*), coalesce(max(listnumber), -1) FROM npcitem WHERE npcid = ?", (npc_id,)).fetchone()
                if n[0] >= CATALOG_CAP:
                    continue
                cur = db.execute("INSERT INTO npcitem (npcid, stackleft, remainHP, remaincharge, patternid, equiploc, location, "
                                 "listnumber) VALUES (?, 1, 0, 0, ?, -1, 1, ?)", (npc_id, r["pid"], n[1] + 1))
                log("npcitem", cur.lastrowid, f"{r['name']}: {r['why']}")
                placed["catalog"] += 1
        elif r["kind"] in ("npcloot", "type"):
            table = f"wiki {r['target']}"
            cur = db.execute("INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) "
                             "VALUES (?, ?, ?, ?, ?, 'wiki')", (table, r["pid"], r["weight"], r["low"], r["high"]))
            log("loottables", cur.lastrowid, f"{r['name']}: {r['why']}")
            placed[r["kind"]] += 1
            names = [r["target"]] if r["kind"] == "npcloot" else [
                n for n, lv in ours.creatures.items()
                if (n in ours.undead and r["target"] == "undead") or
                ("froglok" in n and r["target"] == "frogloks") or
                (kinds is not None and n in kinds.get(r["target"], ()))]
            for n in names:
                if r["kind"] == "type" and not any(r["low"] <= l <= r["high"] for l in ours.creatures[n]):
                    continue
                for ln in sorted(ours.loot_names.get(n) or {n}):
                    if (ln, table) in tables:
                        continue
                    tables.add((ln, table))
                    cur = db.execute("INSERT OR IGNORE INTO npcloot (npc_name, tablename, chance, rolls, note, source) "
                                     "VALUES (?, ?, ?, 1, ?, 'wiki')", (ln, table, r["chance"], r["why"][:200]))
                    if cur.rowcount:
                        log("npcloot", f"{ln}|{table}", r["why"])
        elif r["kind"] == "pool":
            cur = db.execute("INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) "
                             "VALUES (?, ?, '', ?, 1, ?, ?, 'wiki')", (r.get("category", "uncommon"), r["target"], r["pid"], r["low"], r["high"]))
            log("lootpool", cur.lastrowid, f"{r['name']}: {r['why']}")
            placed["pool"] += 1
    return placed


def undo(db, log_table=LOG):
    if not has_table(db, log_table):
        print("nothing logged: nothing to undo")
        return
    n = collections.Counter()
    for tbl, rid, what in db.execute(f"SELECT tbl, rid, what FROM {log_table} ORDER BY seq DESC").fetchall():
        if tbl == "sqlite_sequence":
            col = {"itempattern": "patternid", "npcitem": "itemid"}.get(rid, "id")
            top = db.execute(f"SELECT coalesce(max({col}), 0) FROM {rid}").fetchone()[0]
            if top <= int(what):        # nothing made since: the counter as it was
                db.execute("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", (int(what), rid))
            continue
        if tbl == "itempattern":
            used = db.execute("SELECT count(*) FROM charinventory WHERE patternid = ?", (int(rid),)).fetchone()[0]
            if used:
                print(f"  kept pattern {rid}: {used} held by characters")
                continue
            cur = db.execute("DELETE FROM itempattern WHERE patternid = ?", (int(rid),))
        elif tbl == "npcloot":
            name, table = rid.split("|", 1)
            cur = db.execute("DELETE FROM npcloot WHERE npc_name = ? AND tablename = ? AND source = 'wiki'", (name, table))
        else:
            cur = db.execute(f"DELETE FROM {tbl} WHERE {'itemid' if tbl == 'npcitem' else 'id'} = ?", (int(rid),))
        n[tbl] += cur.rowcount
    db.execute(f"DROP TABLE {log_table}")
    print("deleted: " + ", ".join(f"{t} {c}" for t, c in sorted(n.items())))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--fetch", help="read the wiki's articles into this folder and stop")
    ap.add_argument("--wiki", help="the folder --fetch filled")
    ap.add_argument("--prices", nargs="*", default=[], help="eqoa harvest JSON files or folders of them (Sandstorm's records, values)")
    ap.add_argument("--list", action="store_true", help="print every record and row")
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--apply", action="store_true", help="make the records and rows (logged)")
    g.add_argument("--undo", action="store_true", help="delete what --apply made")
    a = ap.parse_args()
    if a.fetch:
        fetch(a.fetch)
        return
    db = sqlite3.connect(a.db, isolation_level=None)
    db.execute("PRAGMA busy_timeout = 10000")
    if a.undo:
        db.execute("BEGIN IMMEDIATE")
        undo(db)
        db.execute("COMMIT")
        return
    if not a.wiki or not a.prices:
        sys.exit("--wiki (the folder --fetch filled) and --prices (Sandstorm's harvests) are both needed")
    if has_table(db, "wiki_item_import_log"):
        sys.exit("wiki_item_import_log is there: --undo first")
    pages = json.load(open(os.path.join(a.wiki, "allpages.json")))
    ours = Ours(db)
    made, left = plan(db, pages, harvest_records(a.prices), ours)
    measured = Measured()
    measured.load(a.prices)
    rule, _dropped = robust_rule(measured)
    used = {pid for (pid,) in db.execute("SELECT patternid FROM itempattern WHERE patternid BETWEEN ? AND ?", (ID_FIRST, ID_LAST))}
    recs, rows = build(made, ours, rule, used)
    print(f"{len(recs)} items to make, {len(left)} named alone (left)")
    by_src = collections.Counter(why.split(";")[0].split(" (")[0] for _r, why in recs)
    print("  from: " + ", ".join(f"{s} {n}" for s, n in by_src.most_common()))
    by_place = collections.Counter(r["kind"] for r in rows)
    unplaced = [rec["itemname"] for rec, _w in recs if not any(r["pid"] == rec["patternid"] for r in rows)]
    print("  placed: " + ", ".join(f"{k} {n}" for k, n in by_place.most_common()) + f"; nowhere (NO TRADE, a quest's): {unplaced}")
    if a.list:
        for rec, why in recs:
            st = {c: rec[c] for c in STAT_COLS if rec.get(c)}
            print(f"  {rec['patternid']} {rec['itemname']!r} L{rec['levelreq']} slot {rec['equipslot']} type {rec['attacktype']} "
                  f"dmg {rec['weapondamage']} style/model {rec['model']} icon {rec['itemicon']} value {rec['itemcost']} "
                  f"classes {rec['classuse']} races {rec['raceuse']} hp {rec['maxhp']} {st} | {why}")
        for r in rows:
            print(f"    -> {r['name']!r}: {r['kind']} {r['target']!r} chance {r['chance']} weight {r['weight']} "
                  f"levels {r['low']}-{r['high']} ({r['why']})")
        for k, why in left:
            print(f"  left: {k!r}: {why}")
    if not a.apply:
        print("report only: --apply to write")
        return
    db.execute("BEGIN IMMEDIATE")
    placed = write(db, recs, rows, ours)
    db.execute("COMMIT")
    print(f"made {len(recs)} items; placed: " + ", ".join(f"{k} {n}" for k, n in placed.most_common()))


if __name__ == "__main__":
    main()
